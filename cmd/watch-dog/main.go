// Package main is the watch-dog entrypoint: it monitors container health via Docker
// events, discovers parent/dependent relationships from the compose file, and runs
// recovery (restart parent, wait until healthy, then restart dependents).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"watch-dog/internal/discovery"
	"watch-dog/internal/docker"
	"watch-dog/internal/recovery"
)

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
	runStartupReconciliation(ctx, cli, &parentToDeps, flow, selfName)

	healthCh := make(chan docker.HealthEvent, 8)
	cli.SubscribeHealthStatus(ctx, healthCh)

	// Optional polling fallback (e.g. every 60s) to catch missed events
	go runPollingFallback(ctx, cli, flow, selfName)

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
			docker.LogInfo("parent needs recovery", "parent", ev.ContainerName, "id", ev.ContainerID, "reason", ev.Status)
			flow.RunFullSequence(ctx, ev.ContainerID, ev.ContainerName, &parentToDeps, selfName)
		}
	}
}

// runStartupReconciliation finds parents that are already unhealthy or stopped and runs full recovery.
func runStartupReconciliation(ctx context.Context, cli *docker.Client, m *discovery.ParentToDependents, flow *recovery.Flow, selfName string) {
	containers, err := cli.ListContainers(ctx, true)
	if err != nil {
		docker.LogError("startup list containers", "error", err)
		return
	}
	nameToID := make(map[string]string)
	nameToState := make(map[string]string)
	for _, c := range containers {
		nameToID[c.Name] = c.ID
		nameToState[c.Name] = c.State
	}
	for parentName := range *m {
		id, ok := nameToID[parentName]
		if !ok {
			continue
		}
		state := nameToState[parentName]
		if state != "running" {
			docker.LogInfo("startup: parent not running", "parent", parentName, "state", state)
			flow.RunFullSequence(ctx, id, parentName, m, selfName)
			continue
		}
		health, _, err := cli.Inspect(ctx, id)
		if err != nil || health != "unhealthy" {
			continue
		}
		docker.LogInfo("startup: parent already unhealthy", "parent", parentName)
		flow.RunFullSequence(ctx, id, parentName, m, selfName)
	}
}

const pollInterval = 60 * time.Second

// runPollingFallback periodically rechecks parent health and triggers recovery if unhealthy.
func runPollingFallback(ctx context.Context, cli *docker.Client, flow *recovery.Flow, selfName string) {
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
			nameToID := make(map[string]string)
			nameToState := make(map[string]string)
			for _, c := range containers {
				nameToID[c.Name] = c.ID
				nameToState[c.Name] = c.State
			}
			for parentName := range parentToDeps {
				id, ok := nameToID[parentName]
				if !ok {
					continue
				}
				state := nameToState[parentName]
				if state != "running" {
					docker.LogInfo("polling: parent not running", "parent", parentName, "state", state)
					flow.RunFullSequence(ctx, id, parentName, &parentToDeps, selfName)
					continue
				}
				health, _, err := cli.Inspect(ctx, id)
				if err != nil {
					docker.LogDebug("polling: inspect failed", "parent", parentName, "error", err)
					continue
				}
				if health == "unhealthy" {
					docker.LogInfo("polling: unhealthy parent", "parent", parentName)
					flow.RunFullSequence(ctx, id, parentName, &parentToDeps, selfName)
				}
			}
		}
	}
}
