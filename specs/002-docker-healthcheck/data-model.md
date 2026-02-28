# Data Model: Docker Healthcheck and Log Observability (002)

**Branch**: `002-docker-healthcheck` | **Date**: 2026-02-28

This feature does not introduce new persistent data; it adds **configuration entities** consumed at startup from the environment and **runtime behavior** (healthcheck, log output). The following describe the logical model for configuration and behavior.

## Entities

### Healthcheck configuration (runtime / compose)

- **Source**: Environment or compose `healthcheck:` block; .env variables INTERVAL_IN_SECS, RETRIES, START_PERIOD_IN_SECS, TIMEOUT_IN_SECS.
- **Attributes**:
  - **interval**: Duration between checks (e.g. 15s). Canonical env: INTERVAL_IN_SECS.
  - **start_period**: Grace period before failures count (e.g. 20s). Canonical env: START_PERIOD_IN_SECS.
  - **timeout**: Max time for one check (e.g. 10s). Canonical env: TIMEOUT_IN_SECS.
  - **retries**: Consecutive failures before unhealthy (e.g. 2). Canonical env: RETRIES.
- **Validation**: Values must be valid for the Docker healthcheck schema (positive durations / integers). Invalid or unset use image defaults (see contracts).
- **Relationship**: Applied to the watch-dog container at run time; Dockerfile provides default HEALTHCHECK; compose overrides via env substitution.

### Log level (LOG_LEVEL)

- **Source**: Environment variable LOG_LEVEL.
- **Attributes**:
  - **value**: One of DEBUG, INFO, WARN, ERROR (case-insensitive).
- **Default**: INFO when unset or invalid.
- **Behavior**: Only log records with level >= configured level are output.
- **Relationship**: Read once at process start; controls slog handler level.

### Log format (LOG_FORMAT)

- **Source**: Environment variable LOG_FORMAT.
- **Attributes**:
  - **value**: One of compact, timestamp, json (case-insensitive).
- **Default**: timestamp when unset or invalid.
- **Behavior**:
  - **compact**: Line format `[LEVEL] message [key=value...]`.
  - **timestamp**: Line format `timestamp [LEVEL] message [key=value...]` (RFC3339).
  - **json**: One JSON object per line (slog JSON handler).
- **Relationship**: Read once at process start; selects slog handler (or custom handler) for stdout.

## State transitions

- **Process startup**: Read LOG_LEVEL and LOG_FORMAT; configure slog; then run main loop. No state machine for logging.
- **Healthcheck**: Docker daemon runs the healthcheck command periodically; success/failure drives container state (healthy / starting / unhealthy). No application-level state.

## Validation rules (from spec)

- LOG_LEVEL: If not in {DEBUG, INFO, WARN, ERROR}, treat as INFO.
- LOG_FORMAT: If not in {compact, timestamp, json}, treat as timestamp.
- Healthcheck parameters (compose): If a value is missing from .env, compose may use empty or fail; document required .env keys in README and quickstart.
