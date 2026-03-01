// Package main is the watch-dog entrypoint: it monitors container health via Docker
// events, discovers parent/dependent relationships from the compose file, and runs
// recovery (restart parent, wait until healthy, then restart dependents).
// During an initial discovery phase after startup (first discovery + WATCHDOG_INITIAL_DISCOVERY_WAIT),
// no recovery or dependent restarts run; see specs/004-child-deps-initial-restart/contracts/initial-discovery-behavior.md.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"watch-dog/internal/discovery"
	"watch-dog/internal/docker"
	"watch-dog/internal/recovery"
)

var recoveryCooldown time.Duration
var initialDiscoveryWait time.Duration
var dependentRestartCooldown time.Duration

// initialDiscoveryPhaseEnd is set after first discovery; recovery is gated until time.Now() > initialDiscoveryPhaseEnd.
var initialDiscoveryPhaseEnd time.Time

func init() {
	const defaultCooldown = 2 * time.Minute
	s := os.Getenv("RECOVERY_COOLDOWN")
	if s == "" {
		recoveryCooldown = defaultCooldown
	} else {
		d, err := time.ParseDuration(s)
		if err != nil || d <= 0 {
			reason := "must be positive"
			if err != nil {
				reason = err.Error()
			}
			docker.LogWarn("invalid RECOVERY_COOLDOWN, using default 2m", "value", s, "error", reason)
			d = defaultCooldown
		}
		recoveryCooldown = d
	}

	const defaultInitialDiscoveryWait = 60 * time.Second
	ws := os.Getenv("WATCHDOG_INITIAL_DISCOVERY_WAIT")
	if ws == "" {
		initialDiscoveryWait = defaultInitialDiscoveryWait
	} else {
		d, err := time.ParseDuration(ws)
		if err != nil || d <= 0 {
			reason := "must be positive"
			if err != nil {
				reason = err.Error()
			}
			docker.LogWarn("invalid WATCHDOG_INITIAL_DISCOVERY_WAIT, using default 60s", "value", ws, "error", reason)
			d = defaultInitialDiscoveryWait
		}
		initialDiscoveryWait = d
	}

	const defaultDependentRestartCooldown = 90 * time.Second
	ds := os.Getenv("WATCHDOG_DEPENDENT_RESTART_COOLDOWN")
	if ds == "" {
		dependentRestartCooldown = defaultDependentRestartCooldown
	} else {
		// Zero is permitted: 0 disables the cooldown period, so dependents may restart on every eligible recovery.
		d, err := time.ParseDuration(ds)
		if err != nil || d < 0 {
			reason := "must be non-negative"
			if err != nil {
				reason = err.Error()
			}
			docker.LogWarn("invalid WATCHDOG_DEPENDENT_RESTART_COOLDOWN, using default 90s", "value", ds, "error", reason)
			d = defaultDependentRestartCooldown
		}
		dependentRestartCooldown = d
	}
}

// isInitialDiscoveryComplete returns true after the initial discovery phase (first discovery + wait) has elapsed.
// Until then, recovery and runStartupReconciliation must not run; see contracts/initial-discovery-behavior.md.
func isInitialDiscoveryComplete() bool {
	if initialDiscoveryPhaseEnd.IsZero() {
		return false
	}
	return !time.Now().Before(initialDiscoveryPhaseEnd)
}

// recoveryCooldownState tracks last recovery time and in-flight recovery per parent
// to avoid re-running recovery on duplicate events (stop + die) or overlapping runs.
type recoveryCooldownState struct {
	mu       sync.Mutex
	last     map[string]time.Time
	inFlight map[string]bool
}

// StartRecovery checks cooldown and in-flight for parentName. If allowed, marks the parent
// in-flight and updates last recovery time, then returns true. Caller must call EndRecovery
// when recovery finishes (e.g. defer after StartRecovery returns true).
func (s *recoveryCooldownState) StartRecovery(parentName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		s.last = make(map[string]time.Time)
	}
	if s.inFlight == nil {
		s.inFlight = make(map[string]bool)
	}
	if s.inFlight[parentName] {
		return false
	}
	if t, ok := s.last[parentName]; ok && time.Since(t) < recoveryCooldown {
		return false
	}
	s.inFlight[parentName] = true
	s.last[parentName] = time.Now()
	return true
}

// EndRecovery clears the in-flight mark for parentName. Call when recovery for that parent finishes.
func (s *recoveryCooldownState) EndRecovery(parentName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight != nil {
		delete(s.inFlight, parentName)
	}
}

// main initializes logging from env, creates the Docker client, builds parent-to-dependents
// discovery from the compose file, and runs an initial discovery phase (no recovery until
// phase end). After the phase, it runs startup reconciliation once and subscribes to
// health-status events and polling, executing recovery when a parent becomes unhealthy.
// See contracts/initial-discovery-behavior.md.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cli, err := docker.NewClient(ctx)
	if err != nil {
		docker.LogError("create docker client", "error", err)
		os.Exit(1)
	}
	defer cli.Close()

	parentToDeps, err := discovery.BuildParentToDependents(ctx, cli)
	if err != nil {
		docker.LogError("build discovery", "error", err)
		os.Exit(1)
	}

	// Initial discovery phase: no recovery until first discovery + wait has elapsed (specs/004-child-deps-initial-restart).
	initialDiscoveryPhaseEnd = time.Now().Add(initialDiscoveryWait)
	docker.LogInfo("initial discovery started", "wait", initialDiscoveryWait.String())

	flow := &recovery.Flow{
		Client:                   cli,
		DependentRestartCooldown: dependentRestartCooldown,
	}
	cooldown := &recoveryCooldownState{}
	selfName := os.Getenv("WATCHDOG_CONTAINER_NAME")
	if selfName == "" {
		docker.LogWarn("WATCHDOG_CONTAINER_NAME not set: self-last-restart behavior disabled")
	}

	parentNames := parentToDeps.ParentNames()
	if len(parentNames) == 0 {
		docker.LogWarn("no parents discovered; set WATCHDOG_COMPOSE_PATH and mount the compose file", "path", discovery.ComposePathFromEnv())
	} else {
		docker.LogInfo("watch-dog started", "parents", parentNames)
	}

	// Run startup reconciliation exactly once when initial discovery phase ends (not at startup).
	// Post-phase: no cascade—reconciliation runs once; cooldown/in-flight prevent duplicate runs (contracts/initial-discovery-behavior.md).
	go func() {
		d := initialDiscoveryPhaseEnd.Sub(time.Now())
		if d <= 0 {
			d = 0
		}
		select {
		case <-ctx.Done():
			docker.LogInfo("shutdown during initial discovery wait, skipping startup reconciliation")
			return
		case <-time.After(d):
			// wait completed; proceed
		}
		docker.LogInfo("initial discovery complete, recovery enabled")
		var built discovery.ParentToDependents
		var lastErr error
		backoff := 2 * time.Second
		for attempt := 0; attempt < 5; attempt++ {
			if ctx.Err() != nil {
				return
			}
			var buildErr error
			built, buildErr = discovery.BuildParentToDependents(ctx, cli)
			if buildErr == nil {
				runStartupReconciliation(ctx, cli, &built, flow, cooldown, selfName)
				return
			}
			lastErr = buildErr
			docker.LogError("startup reconciliation: build discovery", "error", buildErr, "attempt", attempt+1)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
			}
		}
		docker.LogError("startup reconciliation: gave up after retries, skipping runStartupReconciliation", "error", lastErr)
	}()

	healthCh := make(chan docker.HealthEvent, 8)
	cli.SubscribeHealthStatus(ctx, healthCh)

	go runPollingFallback(ctx, cli, flow, cooldown, selfName)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-healthCh:
			if !ok {
				return
			}
			if !isInitialDiscoveryComplete() {
				continue
			}
			parentToDeps, err = discovery.BuildParentToDependents(ctx, cli)
			if err != nil {
				docker.LogError("refresh discovery", "error", err)
				continue
			}
			if !parentToDeps.IsParent(ev.ContainerName) {
				continue
			}
			tryRecoverParent(ctx, ev.ContainerID, ev.ContainerName, ev.Status, shortID(ev.ContainerID), "event", flow, cooldown, &parentToDeps, selfName)
		}
	}
}

// shortID returns the first 12 characters of a container ID.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// tryRecoverParent runs recovery for a parent if cooldown allows: StartRecovery, then defer EndRecovery, then RunFullSequence.
// reason describes why recovery was triggered (e.g. "stop", "unhealthy"). idShort is the short container ID for logging.
// trigger is "event", "startup", or "polling". INFO recovery log is emitted only when recovery actually runs (after cooldown check).
func tryRecoverParent(ctx context.Context, parentID, parentName, reason, idShort, trigger string, flow *recovery.Flow, cooldown *recoveryCooldownState, parentToDeps *discovery.ParentToDependents, selfName string) {
	if !cooldown.StartRecovery(parentName) {
		docker.LogDebug("skipping recovery, in cooldown or in flight", "parent", parentName, "id", parentID)
		return
	}
	defer cooldown.EndRecovery(parentName)
	docker.LogInfoRecovery(fmt.Sprintf("recovery: attempting recovery for parent %q (reason: %s, trigger: %s)", parentName, reason, trigger), "parent", parentName, "reason", reason, "id_short", idShort, "trigger", trigger)
	flow.RunFullSequence(ctx, parentID, parentName, reason, parentToDeps, selfName)
}

// buildContainerMaps builds name→ID and name→state maps from the given containers.
func buildContainerMaps(containers []docker.ContainerInfo) (nameToID, nameToState map[string]string) {
	nameToID = make(map[string]string)
	nameToState = make(map[string]string)
	for _, c := range containers {
		nameToID[c.Name] = c.ID
		nameToState[c.Name] = c.State
	}
	return nameToID, nameToState
}

// runStartupReconciliation finds parents that are already unhealthy or stopped and runs full recovery.
func runStartupReconciliation(ctx context.Context, cli *docker.Client, m *discovery.ParentToDependents, flow *recovery.Flow, cooldown *recoveryCooldownState, selfName string) {
	containers, err := cli.ListContainers(ctx, true)
	if err != nil {
		docker.LogError("startup list containers", "error", err)
		return
	}
	nameToID, nameToState := buildContainerMaps(containers)
	for parentName := range *m {
		id, ok := nameToID[parentName]
		if !ok {
			continue
		}
		state := nameToState[parentName]
		if state != "running" {
			tryRecoverParent(ctx, id, parentName, state, shortID(id), "startup", flow, cooldown, m, selfName)
			continue
		}
		health, _, err := cli.Inspect(ctx, id)
		if err != nil || health != "unhealthy" {
			continue
		}
		tryRecoverParent(ctx, id, parentName, "unhealthy", shortID(id), "startup", flow, cooldown, m, selfName)
	}
}

const pollInterval = 60 * time.Second

// runPollingFallback periodically rechecks parent health and triggers recovery if unhealthy.
// Recovery runs only after initial discovery phase is complete; see isInitialDiscoveryComplete().
func runPollingFallback(ctx context.Context, cli *docker.Client, flow *recovery.Flow, cooldown *recoveryCooldownState, selfName string) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !isInitialDiscoveryComplete() {
				continue
			}
			parentToDeps, err := discovery.BuildParentToDependents(ctx, cli)
			if err != nil {
				continue
			}
			containers, err := cli.ListContainers(ctx, true)
			if err != nil {
				continue
			}
			nameToID, nameToState := buildContainerMaps(containers)
			for parentName := range parentToDeps {
				id, ok := nameToID[parentName]
				if !ok {
					continue
				}
				state := nameToState[parentName]
				if state != "running" {
					tryRecoverParent(ctx, id, parentName, state, shortID(id), "polling", flow, cooldown, &parentToDeps, selfName)
					continue
				}
				health, _, err := cli.Inspect(ctx, id)
				if err != nil {
					docker.LogDebug("polling: inspect failed", "parent", parentName, "error", err)
					continue
				}
				if health == "unhealthy" {
					tryRecoverParent(ctx, id, parentName, "unhealthy", shortID(id), "polling", flow, cooldown, &parentToDeps, selfName)
				}
			}
		}
	}
}
