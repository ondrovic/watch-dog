package recovery

import (
	"context"
	"time"

	"watch-dog/internal/discovery"
	"watch-dog/internal/docker"
)

const defaultWaitHealthyTimeout = 5 * time.Minute

// Flow runs the full recovery sequence: restart parent, wait until healthy, restart dependents.
type Flow struct {
	Client *docker.Client
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
			docker.LogError("inspect after restart", "container", containerID, "error", err)
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

// RestartDependents restarts all containers that list parentName in depends_on.
// discovery may be nil; then no dependents are restarted.
func (f *Flow) RestartDependents(ctx context.Context, parentName string, discovery *discovery.ParentToDependents) {
	if discovery == nil {
		return
	}
	deps := discovery.GetDependents(parentName)
	for _, name := range deps {
		if err := f.Client.Restart(ctx, name); err != nil {
			docker.LogError("restart dependent", "dependent", name, "error", err)
		} else {
			docker.LogInfo("restarted dependent", "dependent", name, "parent", parentName)
		}
	}
}

// RunFullSequence restarts the parent, waits until healthy, then restarts dependents.
// If wait-for-healthy times out, dependents are not restarted.
func (f *Flow) RunFullSequence(ctx context.Context, parentID, parentName string, discovery *discovery.ParentToDependents) {
	if err := f.RestartParent(ctx, parentID); err != nil {
		docker.LogError("restart parent", "parent", parentName, "error", err)
		return
	}
	docker.LogInfo("restarted parent, waiting for healthy", "parent", parentName)
	if !f.WaitUntilHealthy(ctx, parentID, defaultWaitHealthyTimeout) {
		docker.LogWarn("parent did not become healthy in time; not restarting dependents", "parent", parentName)
		return
	}
	f.RestartDependents(ctx, parentName, discovery)
}
