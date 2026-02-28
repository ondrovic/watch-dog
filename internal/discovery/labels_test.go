package discovery

import (
	"testing"
)

func TestComposePathFromEnv_leadingColon(t *testing.T) {
	// Ensure WATCHDOG_COMPOSE_PATH does not override
	t.Setenv("WATCHDOG_COMPOSE_PATH", "")
	t.Setenv("COMPOSE_FILE", ":second.yml")

	got := ComposePathFromEnv()
	if got != "" {
		t.Errorf("ComposePathFromEnv() with COMPOSE_FILE=\":second.yml\" = %q, want \"\"", got)
	}
}
