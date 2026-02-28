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

func TestComposePathFromEnv_singlePath(t *testing.T) {
	t.Setenv("WATCHDOG_COMPOSE_PATH", "")
	t.Setenv("COMPOSE_FILE", "docker-compose.yml")

	got := ComposePathFromEnv()
	if got != "docker-compose.yml" {
		t.Errorf("ComposePathFromEnv() with COMPOSE_FILE=\"docker-compose.yml\" = %q, want \"docker-compose.yml\"", got)
	}
}

func TestComposePathFromEnv_midColon(t *testing.T) {
	t.Setenv("WATCHDOG_COMPOSE_PATH", "")
	t.Setenv("COMPOSE_FILE", "first.yml:second.yml")

	got := ComposePathFromEnv()
	if got != "first.yml" {
		t.Errorf("ComposePathFromEnv() with COMPOSE_FILE=\"first.yml:second.yml\" = %q, want \"first.yml\"", got)
	}
}

func TestComposePathFromEnv_watchdogPrecedence(t *testing.T) {
	t.Setenv("WATCHDOG_COMPOSE_PATH", "/path/compose.yml")
	t.Setenv("COMPOSE_FILE", "other.yml")

	got := ComposePathFromEnv()
	if got != "/path/compose.yml" {
		t.Errorf("ComposePathFromEnv() with WATCHDOG_COMPOSE_PATH set = %q, want \"/path/compose.yml\"", got)
	}
}
