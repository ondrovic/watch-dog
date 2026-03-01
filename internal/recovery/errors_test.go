package recovery

import (
	"errors"
	"testing"
)

func TestIsUnrestartableError_noSuchContainer(t *testing.T) {
	err := errors.New("Error response from daemon: No such container: abc123")
	if !IsUnrestartableError(err) {
		t.Error("expected no such container to be unrestartable")
	}
}

func TestIsUnrestartableError_markedForRemoval(t *testing.T) {
	err := errors.New("container is marked for removal and cannot be started")
	if !IsUnrestartableError(err) {
		t.Error("expected marked for removal to be unrestartable")
	}
}

func TestIsUnrestartableError_cannotBeStartedRemoval(t *testing.T) {
	// Exercises the branch that checks for "cannot be started" and "removal" (or "pending removal").
	err := errors.New("container cannot be started; pending removal")
	if !IsUnrestartableError(err) {
		t.Error("expected cannot be started + removal to be unrestartable")
	}
}

func TestIsUnrestartableError_dependencyMissing(t *testing.T) {
	err := errors.New("joining network namespace of container xyz: No such container: xyz")
	if !IsUnrestartableError(err) {
		t.Error("expected dependency missing (joining network namespace + No such container) to be unrestartable")
	}
}

func TestIsUnrestartableError_nil(t *testing.T) {
	if IsUnrestartableError(nil) {
		t.Error("nil should not be unrestartable")
	}
}

func TestIsUnrestartableError_otherError(t *testing.T) {
	err := errors.New("connection refused")
	if IsUnrestartableError(err) {
		t.Error("connection refused should not be unrestartable")
	}
}
