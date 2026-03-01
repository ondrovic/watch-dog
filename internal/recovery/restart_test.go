package recovery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"watch-dog/internal/discovery"
)

// fakeClient records Restart and Inspect calls for tests.
type fakeClient struct {
	mu              sync.Mutex
	restarts        []string
	inspect         map[string]string // containerID -> health to return
	nextRestartErr  error              // if set, Restart returns it once and clears it
}

func (c *fakeClient) Restart(ctx context.Context, containerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nextRestartErr != nil {
		err := c.nextRestartErr
		c.nextRestartErr = nil
		return err
	}
	c.restarts = append(c.restarts, containerID)
	return nil
}

func (c *fakeClient) Inspect(ctx context.Context, containerID string) (health string, labels map[string]string, err error) {
	c.mu.Lock()
	h := c.inspect[containerID]
	if h == "" {
		h = "healthy"
	}
	c.mu.Unlock()
	return h, nil, nil
}

func (c *fakeClient) getRestarts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.restarts...)
}

func TestRestartDependents_dependentCooldownSkipsSecondRestart(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{inspect: make(map[string]string)}
	flow := &Flow{
		Client:                   fake,
		DependentRestartCooldown: 2 * time.Second,
	}
	parentToDeps := discovery.ParentToDependents{
		"parent1": {"dep-a", "dep-b"},
		"parent2": {"dep-a", "dep-b"},
	}

	// First RestartDependents (parent1 recovered): should restart dep-a and dep-b.
	flow.RestartDependents(ctx, "parent1", &parentToDeps, nil, "")
	got := fake.getRestarts()
	if len(got) != 2 {
		t.Fatalf("after first RestartDependents: got %d restarts %v, want 2", len(got), got)
	}

	// Second RestartDependents (parent2 recovered) within cooldown: should skip both.
	flow.RestartDependents(ctx, "parent2", &parentToDeps, nil, "")
	got = fake.getRestarts()
	if len(got) != 2 {
		t.Errorf("after second RestartDependents within cooldown: got %d restarts %v, want still 2 (skipped)", len(got), got)
	}
}

func TestRestartDependents_dependentCooldownAllowsRestartAfterWindow(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{inspect: make(map[string]string)}
	flow := &Flow{
		Client:                   fake,
		DependentRestartCooldown: 20 * time.Millisecond,
	}
	parentToDeps := discovery.ParentToDependents{
		"parent1": {"dep-a"},
	}

	flow.RestartDependents(ctx, "parent1", &parentToDeps, nil, "")
	if n := len(fake.getRestarts()); n != 1 {
		t.Fatalf("first call: got %d restarts, want 1", n)
	}

	time.Sleep(50 * time.Millisecond)

	flow.RestartDependents(ctx, "parent1", &parentToDeps, nil, "")
	got := fake.getRestarts()
	if len(got) != 2 {
		t.Errorf("after cooldown elapsed: got %d restarts %v, want 2", len(got), got)
	}
}

func TestRestartDependents_failedRestartClearsCooldown(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{inspect: make(map[string]string), nextRestartErr: errors.New("fake restart failure")}
	flow := &Flow{
		Client:                   fake,
		DependentRestartCooldown: 2 * time.Second,
	}
	parentToDeps := discovery.ParentToDependents{
		"parent1": {"dep-a"},
	}

	flow.RestartDependents(ctx, "parent1", &parentToDeps, nil, "")
	if got := fake.getRestarts(); len(got) != 0 {
		t.Fatalf("after first RestartDependents (Restart failed): got %d restarts %v, want 0", len(got), got)
	}

	flow.RestartDependents(ctx, "parent1", &parentToDeps, nil, "")
	got := fake.getRestarts()
	if len(got) != 1 {
		t.Errorf("after second RestartDependents: got %d restarts %v, want 1 (cooldown was cleared)", len(got), got)
	}
}

func TestRestartDependents_cooldownDisabledRestartsEveryTime(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{inspect: make(map[string]string)}
	flow := &Flow{
		Client:                   fake,
		DependentRestartCooldown: 0,
	}
	parentToDeps := discovery.ParentToDependents{
		"parent1": {"dep-a"},
		"parent2": {"dep-a"},
	}

	flow.RestartDependents(ctx, "parent1", &parentToDeps, nil, "")
	flow.RestartDependents(ctx, "parent2", &parentToDeps, nil, "")
	got := fake.getRestarts()
	if len(got) != 2 {
		t.Errorf("cooldown disabled: got %d restarts %v, want 2 (dep-a twice)", len(got), got)
	}
}
