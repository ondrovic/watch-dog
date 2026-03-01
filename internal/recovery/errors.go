// Package recovery implements the restart flow and error classification for
// unrestartable container states (see specs/005-fix-recovery-stale-container).
package recovery

import "strings"

// ClassifyUnrestartable classifies err into a reason and whether the container
// is unrestartable. Reason is one of "dependency_missing", "marked_for_removal",
// "container_gone" when unrestartable is true; otherwise reason is "".
// Used by recovery to decide retries and to report reason for logging and
// OnParentContainerGone (container_gone and marked_for_removal only).
// See contracts/recovery-unrestartable-behavior.md and research.md.
func ClassifyUnrestartable(err error) (reason string, unrestartable bool) {
	if err == nil {
		return "", false
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "joining network namespace") && strings.Contains(s, "no such container"):
		return "dependency_missing", true
	case strings.Contains(s, "marked for removal") || (strings.Contains(s, "cannot be started") && strings.Contains(s, "removal")):
		return "marked_for_removal", true
	case strings.Contains(s, "no such container"):
		return "container_gone", true
	default:
		return "", false
	}
}

// IsUnrestartableError reports whether err indicates the container cannot be
// restarted for this run: the container no longer exists, is marked for removal,
// or a required dependency (e.g. another container's network namespace) is missing.
// Classification is done by inspecting the error message from ContainerRestart
// or ContainerInspect. Only these cases are treated as unrestartable; other
// errors (e.g. daemon busy, timeout) are not and may be retried.
func IsUnrestartableError(err error) bool {
	_, ok := ClassifyUnrestartable(err)
	return ok
}
