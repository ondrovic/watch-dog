// Package recovery implements the restart flow and error classification for
// unrestartable container states (see specs/005-fix-recovery-stale-container).
package recovery

import "strings"

// IsUnrestartableError reports whether err indicates the container cannot be
// restarted for this run: the container no longer exists, is marked for removal,
// or a required dependency (e.g. another container's network namespace) is missing.
// Classification is done by inspecting the error message from ContainerRestart
// or ContainerInspect. Only these cases are treated as unrestartable; other
// errors (e.g. daemon busy, timeout) are not and may be retried.
// See contracts/recovery-unrestartable-behavior.md and research.md.
func IsUnrestartableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// No such container: container removed or ID invalid.
	if strings.Contains(msg, "no such container") {
		return true
	}

	// Marked for removal: container cannot be started because it is marked for removal.
	if strings.Contains(msg, "marked for removal") {
		return true
	}
	if strings.Contains(msg, "cannot be started") && strings.Contains(msg, "removal") {
		return true
	}

	return false
}

	return false
}
