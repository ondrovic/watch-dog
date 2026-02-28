# Contract: Watch-Dog Container Healthcheck Behavior

**Feature**: 002-docker-healthcheck | **Date**: 2026-02-28

## Purpose

The watch-dog image defines a HEALTHCHECK so that the container reports a health status (healthy / starting / unhealthy) to the Docker daemon. This contract describes how the healthcheck is implemented and how it should behave.

## Implementation

- **Check command**: A minimal Docker API check run inside the container (e.g. `docker info` or `docker ps -q`). The Docker socket must be mounted; the check succeeds if the command exits 0.
- **Location**: HEALTHCHECK in the Dockerfile with default interval, start_period, timeout, retries. Compose can override via `healthcheck:` using env vars (INTERVAL_IN_SECS, RETRIES, START_PERIOD_IN_SECS, TIMEOUT_IN_SECS).
- **Runtime image**: Must have Docker CLI available (e.g. `docker` binary) so the healthcheck command can run; socket is already mounted by the user.

## Expected behavior

- **Healthy**: The check command exits 0 (Docker API reachable). Container state is reported as healthy after start_period and successful checks.
- **Unhealthy**: The check command exits non-zero (e.g. socket missing, daemon unreachable, or watch-dog process not running so no docker context). After retries consecutive failures, container is unhealthy.
- **Starting**: During start_period, failures do not count toward retries; container may show as "starting" until the first success or until start_period ends and retries are exhausted.

## Consumer expectations

- Operators and orchestrators can rely on `docker ps` / compose health status to see whether the watch-dog container is functioning.
- Restart policies (e.g. on-failure) or parent monitors can react to unhealthy status.
- Documentation (README, quickstart) describes the healthcheck and how to override parameters via .env in Compose.
