# Quickstart: Healthcheck and Log Configuration (002)

**Branch**: `002-docker-healthcheck` | **Date**: 2026-02-28

This quickstart adds **container healthcheck** and **configurable logging** (LOG_LEVEL, LOG_FORMAT) to the watch-dog container. Prerequisites and basic run options are the same as [001 quickstart](../001-container-health-monitor/quickstart.md); below focuses on what’s new in 002.

## Healthcheck

The watch-dog image includes a **HEALTHCHECK** that runs a minimal Docker API check (e.g. `docker info`) inside the container. With the Docker socket mounted, the check verifies the monitor can talk to the daemon.

- **Default (image)**: The Dockerfile defines interval 15s, start period 20s, timeout 10s, retries 2. No configuration needed for basic use.
- **Override in Compose**: Use .env variables so the healthcheck block is driven by your config (no hardcoded values). See [contracts/env-logging-healthcheck.md](./contracts/env-logging-healthcheck.md).

Example `.env` (reference values):

```bash
# Healthcheck (used by compose healthcheck: block)
DOCKER_HEALTHCHECK_INTERVAL=15s
DOCKER_HEALTHCHECK_RETRIES=2
DOCKER_HEALTHCHECK_START_PERIOD=20s
DOCKER_HEALTHCHECK_TIMEOUT=10s
```

Compose snippet using them:

```yaml
watch-dog:
  image: ghcr.io/<owner>/watch-dog:latest
  healthcheck:
    test: ["CMD", "docker", "info"]
    interval: ${DOCKER_HEALTHCHECK_INTERVAL:-15s}
    start_period: ${DOCKER_HEALTHCHECK_START_PERIOD:-20s}
    timeout: ${DOCKER_HEALTHCHECK_TIMEOUT:-10s}
    retries: ${DOCKER_HEALTHCHECK_RETRIES:-2}
  # ... volumes, environment, etc.
```

After starting, `docker ps` (or your UI) will show a health status (healthy / starting / unhealthy) for the watch-dog container.

## Logging: LOG_LEVEL and LOG_FORMAT

Set these in the container environment (e.g. Compose `environment` or `.env`) to control what is logged and how it looks.

### LOG_LEVEL

Controls which messages are output (only messages at or above this level).

| Value  | Effect |
|--------|--------|
| DEBUG  | All messages (debug, info, warn, error). |
| INFO   | Info, warn, error (default). |
| WARN   | Warn and error only. |
| ERROR  | Error only. |

Example:

```yaml
environment:
  - LOG_LEVEL=INFO
```

Or for verbose troubleshooting:

```yaml
environment:
  - LOG_LEVEL=DEBUG
```

### LOG_FORMAT

Controls the shape of each log line.

| Value     | Example line |
|----------|--------------|
| compact  | `[INFO] watch-dog started parents=[vpn dler ...]` |
| timestamp| `2026-02-28T18:04:26Z [INFO] parent needs recovery parent=vpn reason=stop` |
| json     | `{"time":"2026-02-28T18:04:26Z","level":"INFO","msg":"parent needs recovery","parent":"vpn"}` |

Default when unset: **timestamp**.

Example:

```yaml
environment:
  - LOG_FORMAT=timestamp
```

Combined with LOG_LEVEL:

```yaml
environment:
  - LOG_LEVEL=INFO
  - LOG_FORMAT=compact
```

## Full Compose example (002)

```yaml
services:
  watch-dog:
    image: ghcr.io/<owner>/watch-dog:latest
    environment:
      WATCHDOG_COMPOSE_PATH: /app/docker-compose.yml
      LOG_LEVEL: ${LOG_LEVEL:-INFO}
      LOG_FORMAT: ${LOG_FORMAT:-timestamp}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - .:/app:ro
    healthcheck:
      test: ["CMD", "docker", "info"]
      interval: ${DOCKER_HEALTHCHECK_INTERVAL:-15s}
      start_period: ${DOCKER_HEALTHCHECK_START_PERIOD:-20s}
      timeout: ${DOCKER_HEALTHCHECK_TIMEOUT:-10s}
      retries: ${DOCKER_HEALTHCHECK_RETRIES:-2}
    restart: unless-stopped

  # ... your other services (vpn, dependents, etc.)
```

Optional `.env`:

```bash
# Healthcheck
DOCKER_HEALTHCHECK_INTERVAL=15s
DOCKER_HEALTHCHECK_RETRIES=2
DOCKER_HEALTHCHECK_START_PERIOD=20s
DOCKER_HEALTHCHECK_TIMEOUT=10s

# Logging
LOG_LEVEL=INFO
LOG_FORMAT=timestamp
```

## Verify

- **Healthcheck**: `docker ps` shows a health column; watch-dog should become healthy shortly after start. If you stop the process inside the container, it should turn unhealthy after the configured retries/timeout.
- **Logging**: `docker logs watch-dog` shows lines in the chosen format; changing LOG_LEVEL (e.g. to WARN) reduces output; changing LOG_FORMAT (e.g. to compact) changes the line shape. See README for an example line for each LOG_LEVEL and LOG_FORMAT value.
