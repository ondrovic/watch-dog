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
)

const defaultWaitHealthyTimeout = 5 * time.Minute

// dockerClient is the subset of Docker API used by Flow (for testing with fakes).
type dockerClient interface {
	Restart(ctx context.Context, containerID string) error
	Inspect(ctx context.Context, containerID string) (health string, labels map[string]string, err error)
}

// Flow runs the full recovery sequence: restart parent, wait until healthy, restart dependents.
type Flow struct {
	// Client is the Docker client used for restart and inspect.
	Client dockerClient
	// DependentRestartCooldown is the minimum time between restarts of the same dependent (0 = disabled).
	// When multiple parents of the same dependent recover in quick succession, the dependent is restarted at most once per this window.
	DependentRestartCooldown time.Duration

	mu                   sync.Mutex
	lastDependentRestart map[string]time.Time
}

// RestartParent restarts the container by ID or name (idempotent).
func (f *Flow) RestartParent(ctx context.Context, containerID string) error {
	return f.Client.Restart(ctx, containerID)
}

// WaitUntilHealthy polls the container's health status until "healthy" or timeout.
// If timeout is reached, returns false (caller must not restart dependents).
func (f *Flow) WaitUntilHealthy(ctx context.Context, containerID string, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = defaultWaitHealthyTimeout
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		health, _, err := f.Client.Inspect(ctx, containerID)
		if err != nil {
			docker.LogErrorRecovery(fmt.Sprintf("recovery: inspect after restart failed (container %s)", containerID), "container", containerID, "error", err)
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
// discovery may be nil; then no dependents are restarted.
func (f *Flow) RestartDependents(ctx context.Context, parentName string, discovery *discovery.ParentToDependents, selfName string) {
	if discovery == nil {
		docker.LogDebug("no discovery available, skipping restart of dependents", "parentName", parentName)
		return
	}
	deps := discovery.GetDependents(parentName)
	if len(deps) == 0 {
		return
	}
	ordered := slices.Clone(deps)
	// Deterministic order: sort by name.
	slices.Sort(ordered)
	// If self is in the list, move it to last so we don't cancel in-flight restarts.
	if selfName != "" {
		for i, name := range ordered {
			if name == selfName {
				copy(ordered[i:], ordered[i+1:])
				ordered[len(ordered)-1] = name
				break
			}
		}
	}
	for _, name := range ordered {
		if f.DependentRestartCooldown > 0 && !f.shouldRestartDependent(name) {
			docker.LogDebug("skip dependent restart, within cooldown", "dependent", name, "parent", parentName)
			continue
		}
		if err := f.Client.Restart(ctx, name); err != nil {
			docker.LogErrorRecovery(fmt.Sprintf("recovery: failed to restart dependent %q (parent %s)", name, parentName), "dependent", name, "parent", parentName, "error", err)
			if f.DependentRestartCooldown > 0 {
				f.clearDependentCooldown(name)
			}
		} else {
			docker.LogInfoRecovery(fmt.Sprintf("recovery: restarted dependent %q (parent %s)", name, parentName), "dependent", name, "parent", parentName)
		}
	}
}

// RunFullSequence restarts the parent, waits until healthy, then restarts dependents.
// If wait-for-healthy times out, dependents are not restarted.
// reason describes why recovery was triggered (e.g. "stop", "unhealthy"); used for logging.
// selfName is optional; when set and present in the dependent list, that container is restarted last.
func (f *Flow) RunFullSequence(ctx context.Context, parentID, parentName, reason string, discovery *discovery.ParentToDependents, selfName string) {
	if reason == "" {
		reason = "unknown"
	}
	docker.LogInfoRecovery(fmt.Sprintf("recovery: starting recovery sequence for parent %q (reason: %s)", parentName, reason), "parent", parentName, "reason", reason)
	if err := f.RestartParent(ctx, parentID); err != nil {
		docker.LogErrorRecovery(fmt.Sprintf("recovery: failed to restart parent %q", parentName), "parent", parentName, "error", err)
		return
	}
	docker.LogInfoRecovery(fmt.Sprintf("recovery: restarted parent %q, waiting for healthy", parentName), "parent", parentName)
	if !f.WaitUntilHealthy(ctx, parentID, defaultWaitHealthyTimeout) {
		docker.LogWarnRecovery(fmt.Sprintf("recovery: parent %q did not become healthy in time; not restarting dependents", parentName), "parent", parentName)
		return
	}
	f.RestartDependents(ctx, parentName, discovery, selfName)
}
