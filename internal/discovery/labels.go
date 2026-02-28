package discovery

import (
	"context"
	"os"
	"strings"

	"watch-dog/internal/docker"
)

// ComposePathFromEnv returns the path to the compose file for root-level depends_on discovery.
// Checks WATCHDOG_COMPOSE_PATH first, then COMPOSE_FILE (first path if colon-separated).
// Empty means do not use compose-based discovery.
func ComposePathFromEnv() string {
	if p := os.Getenv("WATCHDOG_COMPOSE_PATH"); p != "" {
		return p
	}
	if p := os.Getenv("COMPOSE_FILE"); p != "" {
		if idx := strings.Index(p, ":"); idx > 0 {
			return p[:idx]
		}
		return p
	}
	return ""
}

// ParentToDependents maps parent container name -> list of dependent container names.
type ParentToDependents map[string][]string

// BuildParentToDependents uses root-level depends_on from the compose file when
// WATCHDOG_COMPOSE_PATH or COMPOSE_FILE is set; otherwise returns an empty map.
// Discovery is 100% from compose (no label-based depends_on).
func BuildParentToDependents(ctx context.Context, client *docker.Client) (ParentToDependents, error) {
	path := ComposePathFromEnv()
	return BuildParentToDependentsFromCompose(ctx, client, path)
}

// GetDependents returns dependent container names for a parent (empty if none or unknown).
func (m ParentToDependents) GetDependents(parentName string) []string {
	return m[parentName]
}

// IsParent returns true if the given container name is a parent (has at least one dependent).
func (m ParentToDependents) IsParent(containerName string) bool {
	return len(m[containerName]) > 0
}

// ParentNames returns the list of parent container names (for logging).
func (m ParentToDependents) ParentNames() []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	return names
}
