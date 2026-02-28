# Contract: Environment Variables (Logging and Healthcheck)

**Feature**: 002-docker-healthcheck | **Date**: 2026-02-28

This contract defines the environment variables used for log configuration and for healthcheck parameter substitution in Docker Compose. The watch-dog process reads logging variables at startup; Compose uses healthcheck variables for the `healthcheck:` block.

## Logging

| Variable    | Allowed values              | Default   | Description |
|------------|-----------------------------|-----------|-------------|
| LOG_LEVEL  | DEBUG, INFO, WARN, ERROR    | INFO      | Only messages at this level or higher are output. Case-insensitive. |
| LOG_FORMAT | compact, timestamp, json    | timestamp | Output style: compact = `[LEVEL] message`; timestamp = `RFC3339 [LEVEL] message`; json = one JSON object per line. Case-insensitive. |

- When a value is unset or invalid, the default is used.
- README MUST document each value with an example (snippet or example log line).

## Healthcheck (Compose)

Used in the compose `healthcheck:` block with variable substitution (e.g. `interval: ${INTERVAL_IN_SECS}`). Not read by the watch-dog process; Compose injects them when evaluating the compose file.

| Variable               | Example | Description |
|------------------------|---------|-------------|
| INTERVAL_IN_SECS       | 15s     | Time between health checks. |
| START_PERIOD_IN_SECS   | 20s     | Grace period before failures count. |
| TIMEOUT_IN_SECS        | 10s     | Max time for one check. |
| RETRIES                | 2       | Consecutive failures before unhealthy. |

- The Dockerfile HEALTHCHECK uses fixed defaults (same numeric values); Compose overrides when these variables are set in .env or environment.
- README and quickstart MUST show a compose example that uses these variables (no hardcoded values in the example).

## Stability

- Adding new LOG_FORMAT or LOG_LEVEL values is backward-compatible.
- Changing defaults (e.g. LOG_FORMAT from timestamp to compact) is a breaking change and must be versioned.
