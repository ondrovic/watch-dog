# Contract: Environment Variable — Initial Discovery Wait

**Feature**: 004-child-deps-initial-restart | **Date**: 2026-02-28

This contract defines the environment variable used to configure the **initial discovery wait time**. The watch-dog process reads it at startup. Stacks vary in how long they take to become ready (e.g. 120 seconds to 5 minutes); operators set this per deployment.

## Variable

| Variable | Allowed values | Default | Description |
|----------|----------------|---------|-------------|
| WATCHDOG_INITIAL_DISCOVERY_WAIT | Positive duration (Go `time.ParseDuration`) | 60s | Duration to wait after the first discovery cycle before the monitor may run recovery or restart dependents. Examples: `30s`, `2m`, `5m`. |

- When unset or invalid (non-positive or parse error), the default **60s** is used and a warning is logged.
- README and quickstart MUST document this variable with an example (e.g. `WATCHDOG_INITIAL_DISCOVERY_WAIT=120s` or `5m` for slow stacks).

## Relationship to other env

- Logging: `LOG_LEVEL`, `LOG_FORMAT` — see [002 env-logging-healthcheck](../002-docker-healthcheck/contracts/env-logging-healthcheck.md).
- Recovery: `RECOVERY_COOLDOWN` — applies only after initial discovery is complete.
- Self-name: `WATCHDOG_CONTAINER_NAME` — unchanged; used when ordering dependents (003).

## Stability

- Changing the default (e.g. from 60s to 120s) is a behavioral change and should be versioned/release-noted.
- Adding new allowed formats for duration is backward-compatible (ParseDuration already accepts a wide set).
