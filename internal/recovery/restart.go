// Package recovery implements the restart flow: restart parent, wait until healthy,
// then restart dependents.
package recovery

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"watch-dog/internal/discovery"
	"watch-dog/internal/docker"
	"watch-dog/internal/util"
)

const defaultWaitHealthyTimeout = 5 * time.Minute

// dockerClient is the subset of Docker API used by Flow (for testing with fakes).
type dockerClient interface {
	Restart(ctx context.Context, containerID string) error
	Inspect(ctx context.Context, containerID string) (health string, labels map[string]string, err error)
}

// Flow runs the full recovery sequence: restart parent, wait until healthy, restart dependents.
// When a restart or inspect fails with an unrestartable error, that container ID is added to
// Unrestartable and the sequence is skipped on subsequent triggers until re-discovery yields
// a new ID for the same service (see specs/005-fix-recovery-stale-container).
type Flow struct {
	// Client is the Docker client used for restart and inspect.
	Client dockerClient
	// DependentRestartCooldown is the minimum time between restarts of the same dependent (0 = disabled).
	// When multiple parents of the same dependent recover in quick succession, the dependent is restarted at most once per this window.
	DependentRestartCooldown time.Duration
	// Unrestartable holds container IDs for which restart (or inspect during wait-for-healthy) has failed with an unrestartable error.
	// If nil, unrestartable tracking is disabled.
	Unrestartable *Set
	// OnParentContainerGone, if non-nil, is called when a parent is added to Unrestartable with reason container_gone or marked_for_removal.
	// Used for optional auto-recreate (e.g. run docker compose up -d <serviceName>) so the operator does not have to run compose by hand (FR-008).
	OnParentContainerGone func(parentName string)

	mu                   sync.Mutex
	lastDependentRestart map[string]time.Time
}

// RestartParent restarts the container by ID or name (idempotent).
func (f *Flow) RestartParent(ctx context.Context, containerID string) error {
	return f.Client.Restart(ctx, containerID)
}

// WaitUntilHealthy polls the container's health status until "healthy" or timeout.
// If timeout is reached, returns false (caller must not restart dependents).
// If Inspect returns an unrestartable error, the container ID is added to the unrestartable set and false is returned.
// parentName is the service name (e.g. for logging and OnParentContainerGone); only used when the container is the parent.
func (f *Flow) WaitUntilHealthy(ctx context.Context, containerID, parentName string, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = defaultWaitHealthyTimeout
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		health, _, err := f.Client.Inspect(ctx, containerID)
		if err != nil {
			f.handleUnrestartableError(err, containerID, HandleUnrestartableOptions{
				AddToSet:   true,
				ParentName: parentName,
				LogUnrestartable: func(reason string, err error) {
					docker.LogErrorRecovery("recovery: inspect failed, container unrestartable (will not retry this ID)", "container", containerID, "id_short", util.ShortID(containerID), "reason", reason, "error", err)
				},
				LogOther: func(err error) {
					docker.LogErrorRecovery("recovery: inspect after restart failed", "container", containerID, "error", err)
				},
			})
			return false
		}
		if health == "healthy" {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// HandleUnrestartableOptions configures handleUnrestartableError. All fields optional.
type HandleUnrestartableOptions struct {
	// AddToSet: if true and err is classified unrestartable, containerID is added to f.Unrestartable.
	AddToSet bool
	// ParentName: if non-empty and reason is container_gone or marked_for_removal, OnParentContainerGone(parentName) is called.
	ParentName string
	// LogUnrestartable is called when err is classified unrestartable (after Add and callback).
	LogUnrestartable func(reason string, err error)
	// LogOther is called when err is not classified unrestartable.
	LogOther func(err error)
}

// handleUnrestartableError classifies err with ClassifyUnrestartable. If unrestartable and f.Unrestartable is set and opts.AddToSet,
// adds containerID to the set, optionally invokes OnParentContainerGone for container_gone/marked_for_removal, then calls LogUnrestartable.
// Otherwise calls LogOther. Returns true if the error was classified unrestartable and handled.
func (f *Flow) handleUnrestartableError(err error, containerID string, opts HandleUnrestartableOptions) bool {
	reasonStr, ok := ClassifyUnrestartable(err)
	if !ok {
		if opts.LogOther != nil {
			opts.LogOther(err)
		}
		return false
	}
	if f.Unrestartable != nil && opts.AddToSet {
		f.Unrestartable.Add(containerID, nil)
	}
	if (reasonStr == "container_gone" || reasonStr == "marked_for_removal") && opts.ParentName != "" && f.OnParentContainerGone != nil {
		f.OnParentContainerGone(opts.ParentName)
	}
	if opts.LogUnrestartable != nil {
		opts.LogUnrestartable(reasonStr, err)
	}
	return true
}

// shouldRestartDependent reports whether the dependent name may be restarted under cooldown,
// and if so updates the last-restart timestamp. Caller must hold no locks.
func (f *Flow) shouldRestartDependent(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastDependentRestart == nil {
		f.lastDependentRestart = make(map[string]time.Time)
	}
	last := f.lastDependentRestart[name]
	if !last.IsZero() && time.Since(last) < f.DependentRestartCooldown {
		return false
	}
	f.lastDependentRestart[name] = time.Now()
	return true
}

// clearDependentCooldown removes the dependent from the cooldown map so a subsequent restart is allowed (e.g. after a failed restart).
func (f *Flow) clearDependentCooldown(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastDependentRestart != nil {
		delete(f.lastDependentRestart, name)
	}
}

// RestartDependents restarts all containers that list parentName in depends_on,
// one at a time in deterministic order (sorted by name). If selfName is non-empty
// and present in the list, it is restarted last so in-flight operations are not canceled.
// If DependentRestartCooldown is set, a dependent that was restarted within that window is skipped (at most one restart per dependent per cooldown).
// If a dependent's container ID is in the unrestartable set, that dependent is skipped. On Restart failure with an unrestartable error, the dependent's ID is added to the set.
// discovery may be nil; then no dependents are restarted. nameToID maps container name to ID (for unrestartable check and logging); may be nil.
// When nameToID is nil, depID is set to the dependent's container name (Docker API accepts name for Restart); unrestartable check and add are skipped for dependents so the set remains ID-only and behavior is consistent with other code paths.
// RestartDependents can be invoked without prior RestartParent (e.g. proactive restart when parent has new ID and is already healthy); same cooldown applies.
func (f *Flow) RestartDependents(ctx context.Context, parentName string, discovery *discovery.ParentToDependents, nameToID map[string]string, selfName string) {
	if discovery == nil {
		docker.LogDebug("no discovery available, skipping restart of dependents", "parentName", parentName)
		return
	}
	deps := discovery.GetDependents(parentName)
	if len(deps) == 0 {
		return
	}
	ordered := slices.Clone(deps)
	slices.Sort(ordered)
	if selfName != "" {
		for i, name := range ordered {
			if name == selfName {
				copy(ordered[i:], ordered[i+1:])
				ordered[len(ordered)-1] = name
				break
			}
		}
	}
	if nameToID == nil && f.Unrestartable != nil {
		docker.LogInfoRecovery(fmt.Sprintf("recovery: nameToID absent, skipping unrestartable check for dependents (parent %s)", parentName), "parent", parentName)
	}
	for _, name := range ordered {
		var depID string
		if nameToID != nil {
			id, ok := nameToID[name]
			if !ok {
				docker.LogWarnRecovery(fmt.Sprintf("recovery: no ID mapping for dependent %q (parent %s), skipping", name, parentName), "dependent", name, "parent", parentName)
				continue
			}
			depID = id
		} else {
			// Fallback: use name so Restart works (Docker accepts name). Unrestartable set is not checked or updated when nameToID is nil so keys stay ID-only.
			// TODO if we ever need to check/add unrestartable when nameToID is nil: either resolve name→ID (e.g. caller-provided lookup) or document that set keys accept names as well / normalize keys globally.
			depID = name
		}
		if nameToID != nil && f.Unrestartable != nil && f.Unrestartable.Contains(depID) {
			docker.LogInfoRecovery(fmt.Sprintf("recovery: skipping dependent %q (parent %s), container unrestartable", name, parentName), "dependent", name, "parent", parentName, "id_short", util.ShortID(depID))
			continue
		}
		if f.DependentRestartCooldown > 0 && !f.shouldRestartDependent(name) {
			docker.LogDebug("skip dependent restart, within cooldown", "dependent", name, "parent", parentName)
			continue
		}
		if err := f.Client.Restart(ctx, depID); err != nil {
			f.handleUnrestartableError(err, depID, HandleUnrestartableOptions{
				AddToSet: nameToID != nil,
				LogUnrestartable: func(reason string, err error) {
					if nameToID != nil {
						docker.LogErrorRecovery(fmt.Sprintf("recovery: failed to restart dependent %q (%s), will not retry this container ID", name, reason), "dependent", name, "parent", parentName, "id_short", util.ShortID(depID), "reason", reason, "error", err)
					} else {
						docker.LogErrorRecovery(fmt.Sprintf("recovery: failed to restart dependent %q (%s), nameToID absent so not recording as unrestartable", name, reason), "dependent", name, "parent", parentName, "reason", reason, "error", err)
					}
				},
				LogOther: func(err error) {
					docker.LogErrorRecovery(fmt.Sprintf("recovery: failed to restart dependent %q (parent %s)", name, parentName), "dependent", name, "parent", parentName, "error", err)
				},
			})
			if f.DependentRestartCooldown > 0 {
				f.clearDependentCooldown(name)
			}
		} else {
			docker.LogInfoRecovery(fmt.Sprintf("recovery: restarted dependent %q (parent %s)", name, parentName), "dependent", name, "parent", parentName)
		}
	}
}

// RunFullSequence restarts the parent, waits until healthy, then restarts dependents.
// If the parent ID is already in the unrestartable set, the sequence is skipped and a skip log is emitted.
// If restart or inspect fails with an unrestartable error, that ID is added to the set and the sequence stops (no dependents).
// If wait-for-healthy times out, dependents are not restarted.
// reason describes why recovery was triggered (e.g. "stop", "unhealthy"); used for logging.
// selfName is optional; when set and present in the dependent list, that container is restarted last.
// nameToID maps container name to ID for dependents; used to check unrestartable set and for logging (may be nil).
func (f *Flow) RunFullSequence(ctx context.Context, parentID, parentName, reason string, discovery *discovery.ParentToDependents, nameToID map[string]string, selfName string) {
	if reason == "" {
		reason = "unknown"
	}
	if f.Unrestartable != nil && f.Unrestartable.Contains(parentID) {
		docker.LogInfoRecovery(fmt.Sprintf("recovery: skipping parent %q, container unrestartable (will retry when new instance appears)", parentName), "parent", parentName, "id_short", util.ShortID(parentID))
		return
	}
	docker.LogInfoRecovery(fmt.Sprintf("recovery: starting recovery sequence for parent %q (reason: %s)", parentName, reason), "parent", parentName, "reason", reason)
	if err := f.RestartParent(ctx, parentID); err != nil {
		f.handleUnrestartableError(err, parentID, HandleUnrestartableOptions{
			AddToSet:   true,
			ParentName: parentName,
			LogUnrestartable: func(reason string, err error) {
				docker.LogErrorRecovery(fmt.Sprintf("recovery: failed to restart parent %q (%s), will not retry this container ID", parentName, reason), "parent", parentName, "id_short", util.ShortID(parentID), "reason", reason, "error", err)
			},
			LogOther: func(err error) {
				docker.LogErrorRecovery(fmt.Sprintf("recovery: failed to restart parent %q", parentName), "parent", parentName, "error", err)
			},
		})
		return
	}
	docker.LogInfoRecovery(fmt.Sprintf("recovery: restarted parent %q, waiting for healthy", parentName), "parent", parentName)
	if !f.WaitUntilHealthy(ctx, parentID, parentName, defaultWaitHealthyTimeout) {
		docker.LogWarnRecovery(fmt.Sprintf("recovery: parent %q did not become healthy in time; not restarting dependents", parentName), "parent", parentName)
		return
	}
	f.RestartDependents(ctx, parentName, discovery, nameToID, selfName)
}
