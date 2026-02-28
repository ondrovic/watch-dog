// Package main is the watch-dog entrypoint: it monitors container health via Docker
// events, discovers parent/dependent relationships from the compose file, and runs
// recovery (restart parent, wait until healthy, then restart dependents).
package main

import (
	"context"
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

func init() {
	const defaultCooldown = 2 * time.Minute
	s := os.Getenv("RECOVERY_COOLDOWN")
	if s == "" {
		recoveryCooldown = defaultCooldown
		return
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		reason := "must be positive"
		if err != nil {
			reason = err.Error()
		}
		docker.LogWarn("invalid RECOVERY_COOLDOWN, using default 2m", "value", s, "error", reason)
		recoveryCooldown = defaultCooldown
		return
	}
	recoveryCooldown = d
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
// discovery from the compose file, subscribes to health-status events, runs startup
// reconciliation and a polling fallback, and executes recovery (restart parent then
// dependents) when a parent becomes unhealthy.
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

	flow := &recovery.Flow{Client: cli}
	cooldown := &recoveryCooldownState{}
	selfName := os.Getenv("WATCHDOG_CONTAINER_NAME")
	if selfName == "" {
		docker.LogWarn("WATCHDOG_CONTAINER_NAME not set: self-last-restart behavior disabled")
	}

	// Log startup and discovered parents so there is always visible output (e.g. docker logs watch-dog).
	parentNames := parentToDeps.ParentNames()
	if len(parentNames) == 0 {
		docker.LogWarn("no parents discovered; set WATCHDOG_COMPOSE_PATH and mount the compose file", "path", discovery.ComposePathFromEnv())
	} else {
		docker.LogInfo("watch-dog started", "parents", parentNames)
	}

	// Startup reconciliation: treat already-unhealthy or stopped parents (per contracts/recovery-behavior.md).
	runStartupReconciliation(ctx, cli, &parentToDeps, flow, cooldown, selfName)

	healthCh := make(chan docker.HealthEvent, 8)
	cli.SubscribeHealthStatus(ctx, healthCh)

	// Optional polling fallback (e.g. every 60s) to catch missed events
	go runPollingFallback(ctx, cli, flow, cooldown, selfName)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-healthCh:
			if !ok {
				return
			}
			parentToDeps, err = discovery.BuildParentToDependents(ctx, cli)
			if err != nil {
				docker.LogError("refresh discovery", "error", err)
				continue
			}
			if !parentToDeps.IsParent(ev.ContainerName) {
				continue
			}
			func() {
				if !cooldown.StartRecovery(ev.ContainerName) {
					docker.LogDebug("skipping recovery, in cooldown or already in flight", "parent", ev.ContainerName, "id", ev.ContainerID)
					return
				}
				defer cooldown.EndRecovery(ev.ContainerName)
				docker.LogInfo("parent needs recovery", "parent", ev.ContainerName, "id", ev.ContainerID, "reason", ev.Status)
				flow.RunFullSequence(ctx, ev.ContainerID, ev.ContainerName, &parentToDeps, selfName)
			}()
		}
	}
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
			docker.LogInfo("startup: parent not running", "parent", parentName, "state", state)
			func() {
				if !cooldown.StartRecovery(parentName) {
					docker.LogDebug("startup: skipping recovery, in cooldown or in flight", "parent", parentName, "id", id)
					return
				}
				defer cooldown.EndRecovery(parentName)
				flow.RunFullSequence(ctx, id, parentName, m, selfName)
			}()
			continue
		}
		health, _, err := cli.Inspect(ctx, id)
		if err != nil || health != "unhealthy" {
			continue
		}
		docker.LogInfo("startup: parent already unhealthy", "parent", parentName)
		func() {
			if !cooldown.StartRecovery(parentName) {
				docker.LogDebug("startup: skipping recovery, in cooldown or in flight", "parent", parentName, "id", id)
				return
			}
			defer cooldown.EndRecovery(parentName)
			flow.RunFullSequence(ctx, id, parentName, m, selfName)
		}()
	}
}

const pollInterval = 60 * time.Second

// runPollingFallback periodically rechecks parent health and triggers recovery if unhealthy.
func runPollingFallback(ctx context.Context, cli *docker.Client, flow *recovery.Flow, cooldown *recoveryCooldownState, selfName string) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
					docker.LogInfo("polling: parent not running", "parent", parentName, "state", state)
					if cooldown.StartRecovery(parentName) {
						defer cooldown.EndRecovery(parentName)
						flow.RunFullSequence(ctx, id, parentName, &parentToDeps, selfName)
					} else {
						docker.LogDebug("polling: skipping recovery, in cooldown or in flight", "parent", parentName, "id", id)
					}
					continue
				}
				health, _, err := cli.Inspect(ctx, id)
				if err != nil {
					docker.LogDebug("polling: inspect failed", "parent", parentName, "error", err)
					continue
				}
				if health == "unhealthy" {
					docker.LogInfo("polling: unhealthy parent", "parent", parentName)
					if cooldown.StartRecovery(parentName) {
						defer cooldown.EndRecovery(parentName)
						flow.RunFullSequence(ctx, id, parentName, &parentToDeps, selfName)
					} else {
						docker.LogDebug("polling: skipping recovery, in cooldown or in flight", "parent", parentName, "id", id)
					}
				}
			}
		}
	}
}
