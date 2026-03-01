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
	mu               sync.Mutex
	restarts         []string
	inspect          map[string]string // containerID -> health to return
	nextRestartErr   error             // if set, Restart returns it once and clears it
	nextInspectErr   error             // if set, Inspect returns it once and clears it
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
	defer c.mu.Unlock()
	if c.nextInspectErr != nil {
		err = c.nextInspectErr
		c.nextInspectErr = nil
		return "", nil, err
	}
	h := c.inspect[containerID]
	if h == "" {
		h = "healthy"
	}
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

func TestRestartDependents_nameToID_skipsUnrestartableRestartsOthers(t *testing.T) {
	ctx := context.Background()
	const idUnrestartable = "id-unrestartable"
	const idRestartable = "id-restartable"
	fake := &fakeClient{inspect: make(map[string]string)}
	unrestartable := NewSet(0)
	unrestartable.Add(idUnrestartable, nil)
	flow := &Flow{
		Client:                   fake,
		DependentRestartCooldown: 0,
		Unrestartable:            unrestartable,
	}
	parentToDeps := discovery.ParentToDependents{
		"parent1": {"dep-unrestartable", "dep-restartable"},
	}
	nameToID := map[string]string{
		"dep-unrestartable": idUnrestartable,
		"dep-restartable":   idRestartable,
	}

	flow.RestartDependents(ctx, "parent1", &parentToDeps, nameToID, "")
	got := fake.getRestarts()
	if len(got) != 1 || got[0] != idRestartable {
		t.Errorf("with nameToID: got restarts %v, want [%s] (unrestartable skipped)", got, idRestartable)
	}
}

func TestRestartDependents_nameToID_failedRestartClearsCooldown(t *testing.T) {
	ctx := context.Background()
	const depID = "id-dep-a"
	fake := &fakeClient{inspect: make(map[string]string), nextRestartErr: errors.New("fake restart failure")}
	flow := &Flow{
		Client:                   fake,
		DependentRestartCooldown: 2 * time.Second,
	}
	parentToDeps := discovery.ParentToDependents{
		"parent1": {"dep-a"},
	}
	nameToID := map[string]string{"dep-a": depID}

	flow.RestartDependents(ctx, "parent1", &parentToDeps, nameToID, "")
	if got := fake.getRestarts(); len(got) != 0 {
		t.Fatalf("after first RestartDependents (Restart failed) with nameToID: got %d restarts %v, want 0", len(got), got)
	}

	flow.RestartDependents(ctx, "parent1", &parentToDeps, nameToID, "")
	got := fake.getRestarts()
	if len(got) != 1 || got[0] != depID {
		t.Errorf("after second RestartDependents with nameToID: got %v, want [%s] (cooldown cleared)", got, depID)
	}
}

// TestRunFullSequence_onParentContainerGone_calledForContainerGoneAndMarkedForRemoval verifies
// that OnParentContainerGone is invoked when RestartParent fails with container_gone or marked_for_removal,
// and not invoked for dependency_missing.
func TestRunFullSequence_onParentContainerGone_calledForContainerGoneAndMarkedForRemoval(t *testing.T) {
	ctx := context.Background()
	parentToDeps := discovery.ParentToDependents{"vpn": {"dler"}}
	nameToID := map[string]string{"dler": "dep-id"}

	t.Run("container_gone", func(t *testing.T) {
		fake := &fakeClient{inspect: make(map[string]string), nextRestartErr: errors.New("Error response from daemon: No such container: abc123")}
		var goneName string
		flow := &Flow{
			Client:                   fake,
			Unrestartable:            NewSet(0),
			OnParentContainerGone:   func(parentName string) { goneName = parentName },
		}
		flow.RunFullSequence(ctx, "parent-id", "vpn", "die", &parentToDeps, nameToID, "")
		if goneName != "vpn" {
			t.Errorf("OnParentContainerGone called with %q, want %q", goneName, "vpn")
		}
	})

	t.Run("marked_for_removal", func(t *testing.T) {
		fake := &fakeClient{inspect: make(map[string]string), nextRestartErr: errors.New("container is marked for removal and cannot be started")}
		var goneName string
		flow := &Flow{
			Client:                 fake,
			Unrestartable:          NewSet(0),
			OnParentContainerGone: func(parentName string) { goneName = parentName },
		}
		flow.RunFullSequence(ctx, "parent-id", "vpn", "die", &parentToDeps, nameToID, "")
		if goneName != "vpn" {
			t.Errorf("OnParentContainerGone called with %q, want %q", goneName, "vpn")
		}
	})

	t.Run("dependency_missing_not_called", func(t *testing.T) {
		fake := &fakeClient{inspect: make(map[string]string), nextRestartErr: errors.New("joining network namespace of container: No such container: xyz")}
		var goneName string
		flow := &Flow{
			Client:                 fake,
			Unrestartable:          NewSet(0),
			OnParentContainerGone: func(parentName string) { goneName = parentName },
		}
		flow.RunFullSequence(ctx, "parent-id", "vpn", "die", &parentToDeps, nameToID, "")
		if goneName != "" {
			t.Errorf("OnParentContainerGone should not be called for dependency_missing, got %q", goneName)
		}
	})

	t.Run("inspect_unrestartable", func(t *testing.T) {
		// Restart succeeds; WaitUntilHealthy's first Inspect returns unrestartable error → OnParentContainerGone called.
		fake := &fakeClient{inspect: make(map[string]string), nextInspectErr: errors.New("Error response from daemon: No such container: parent-id")}
		var goneName string
		flow := &Flow{
			Client:                 fake,
			Unrestartable:          NewSet(0),
			OnParentContainerGone: func(parentName string) { goneName = parentName },
		}
		flow.RunFullSequence(ctx, "parent-id", "vpn", "die", &parentToDeps, nameToID, "")
		if goneName != "vpn" {
			t.Errorf("OnParentContainerGone called with %q, want %q (WaitUntilHealthy/Inspect path)", goneName, "vpn")
		}
	})
}
