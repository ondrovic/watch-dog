# watch-dog

A standalone Docker container that monitors parent/child container health and restarts unhealthy parents, then (after they become healthy) their dependents. It replaces [autoheal](https://github.com/willfarrell/docker-autoheal) for Compose stacks that use root-level `depends_on` and healthchecks.

## Features

- **Compose-native**: Discovers parent/child relationships from the compose file’s **root-level `depends_on`** (short or long form); no custom labels.
- **Correct order**: Restarts the parent first, waits until it is healthy, then restarts dependents (swarm-like behavior without Swarm).
- **Multi-parent mitigation**: Containers with multiple `depends_on` parents are restarted at most once per cooldown window (default 90s) when several parents recover in quick succession, avoiding redundant restarts.
- **Event-driven**: Uses Docker `health_status` events; optional 60s polling fallback for robustness.
- **Startup reconciliation**: On start, treats already-unhealthy parents and runs the full recovery sequence.
- **Bounded retries when containers are gone**: If a restart fails because the container no longer exists, is marked for removal, or a dependency (e.g. network namespace) is missing, the monitor does not retry that container ID indefinitely; it logs the failure and skips that ID until re-discovery yields a new instance. No extra configuration. When only a parent is replaced by an updater (e.g. watchtower), the monitor proactively restarts that parent’s dependents so the child comes back online.

## Using in Docker Compose

Pull the image from GitHub Container Registry and add a `watch-dog` service to your stack. The monitor needs the Docker socket and access to your compose file so it can read root-level `depends_on`.

Replace `<owner>` with your GitHub org or username (e.g. the repository owner when using GitHub Actions to build).

```yaml
services:
  watch-dog:
    image: ghcr.io/<owner>/watch-dog:latest
    environment:
      WATCHDOG_COMPOSE_PATH: /app/docker-compose.yml
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - .:/app:ro
    restart: unless-stopped

  vpn:
    image: my-vpn-image
    healthcheck: { ... }

  torrent:
    image: my-torrent-image
    depends_on:
      vpn:
        condition: service_healthy
        restart: true
```

Short form `depends_on` is also supported:

```yaml
  torrent:
    depends_on:
      - vpn
```

**Required**: Mount the compose file (e.g. `.:/app:ro`) and set `WATCHDOG_COMPOSE_PATH` (or `COMPOSE_FILE`) to the path **inside the container** (e.g. `/app/docker-compose.yml`). Without this, the monitor will not discover any parents.

## Healthcheck

The watch-dog image includes a **HEALTHCHECK** that runs a minimal Docker API check (e.g. `docker info`) inside the container. With the Docker socket mounted, the check verifies the monitor can talk to the daemon. The container reports a health status (healthy / starting / unhealthy) in `docker ps` and in Compose.

- **How health is determined**: The healthcheck runs the check command at a defined interval. Success (exit 0) means the container is healthy. After consecutive failures (e.g. socket unavailable, process not responding), the container becomes unhealthy. During the start period, failures do not count toward retries.
- **Transient failures**: If the Docker socket or daemon is temporarily unavailable, the healthcheck may report unhealthy or starting until the next successful run. Under heavy load, the check runs within its timeout so it does not block the container; repeated failures lead to unhealthy.

**Override in Compose**: Use .env variables so the healthcheck block is driven by your config (no hardcoded values). Reference values:

| Variable | Description | Example |
|----------|-------------|---------|
| `DOCKER_HEALTHCHECK_INTERVAL` | Time between health checks (duration with unit) | `15s` |
| `DOCKER_HEALTHCHECK_RETRIES` | Consecutive failures before unhealthy | `2` |
| `DOCKER_HEALTHCHECK_START_PERIOD` | Grace period before failures count (duration with unit) | `20s` |
| `DOCKER_HEALTHCHECK_TIMEOUT` | Max time for one check (duration with unit) | `10s` |

Compose example using variable substitution:

```yaml
watch-dog:
  image: ghcr.io/<owner>/watch-dog:latest
  healthcheck:
    test: ["CMD", "docker", "info"]
    interval: ${DOCKER_HEALTHCHECK_INTERVAL:-15s}
    start_period: ${DOCKER_HEALTHCHECK_START_PERIOD:-20s}
    timeout: ${DOCKER_HEALTHCHECK_TIMEOUT:-10s}
    retries: ${DOCKER_HEALTHCHECK_RETRIES:-2}
  # ... environment, volumes, etc.
```

Use a `.env` file or export these variables so Compose can substitute them (e.g. `DOCKER_HEALTHCHECK_INTERVAL=15s`, `DOCKER_HEALTHCHECK_RETRIES=2`, `DOCKER_HEALTHCHECK_START_PERIOD=20s`, `DOCKER_HEALTHCHECK_TIMEOUT=10s`).

## How to use

### Configuration

| Variable | Description |
|----------|-------------|
| `WATCHDOG_COMPOSE_PATH` | Path inside the container to the compose file (e.g. `/app/docker-compose.yml`). |
| `COMPOSE_FILE` | Alternative; if set, the first path in a colon-separated list is used. |
| `WATCHDOG_CONTAINER_NAME` | Optional. When the monitor is a dependent of a recovered parent (e.g. in `depends_on`), set this to the monitor’s container name (e.g. `watch-dog`). The monitor will restart **all other** dependents first, then itself last, so in-flight restarts are not canceled. If unset, dependents are restarted in deterministic (e.g. alphabetical) order with no special handling for the monitor. |
| `WATCHDOG_DEPENDENT_RESTART_COOLDOWN` | Optional. When a container has multiple parents (e.g. `depends_on: [qbittorrent, prowlarr]`), the monitor skips restarting it again if it was already restarted within this duration. Default: `90s`. Set to `0` to disable (restart after every parent recovery). Invalid values fall back to 90s with a warning. See [recovery-behavior](specs/001-container-health-monitor/contracts/recovery-behavior.md). |
| `WATCHDOG_INITIAL_DISCOVERY_WAIT` | Optional. Duration to wait after the first discovery cycle before the monitor may run recovery (e.g. `30s`, `2m`, `5m`). Default: `60s`. Use when bringing the stack up with `docker compose up` so the monitor does not restart dependents during initial startup; set to at least how long your stack needs to become ready (e.g. `120s` or `5m`). Invalid or non-positive values fall back to 60s with a warning in logs. |
| `WATCHDOG_AUTO_RECREATE` | Optional. When a parent is marked unrestartable with reason **container_gone** or **marked_for_removal**, run `docker compose up -d <service_name>` so the service comes back without manual intervention. The monitor resolves container name to service name from the compose file when `container_name` is set. Set to `true`, `1`, or `yes` to enable. Requires `WATCHDOG_COMPOSE_PATH` (or `COMPOSE_FILE`). If unset or compose path empty, auto-recreate is disabled. See [recovery-unrestartable-behavior](specs/005-fix-recovery-stale-container/contracts/recovery-unrestartable-behavior.md). |

#### Logging: LOG_LEVEL and LOG_FORMAT

Set these in the container environment to control what is logged and how it looks.

**LOG_LEVEL** — only messages at this level or higher are output (default: `INFO`):

| Value | Effect |
|-------|--------|
| `DEBUG` | All messages (debug, info, warn, error). Use for troubleshooting. |
| `INFO` | Info, warn, error (default). Normal operation. |
| `WARN` | Warn and error only. |
| `ERROR` | Error only. |

**LOG_FORMAT** — shape of each log line (default: `timestamp`):

| Value | Example line |
|-------|--------------|
| `compact` | `[INFO] watch-dog started parents=[vpn dler ...]` |
| `timestamp` | `2026-02-28T18:04:26Z [INFO] parent needs recovery parent=vpn reason=stop` |
| `json` | `{"time":"2026-02-28T18:04:26Z","level":"INFO","msg":"parent needs recovery","parent":"vpn"}` |

Compose example:

```yaml
environment:
  - LOG_LEVEL=INFO
  - LOG_FORMAT=timestamp
```

Or with defaults from .env: `LOG_LEVEL: ${LOG_LEVEL:-INFO}`, `LOG_FORMAT: ${LOG_FORMAT:-timestamp}`.

Your compose file must declare dependencies with **root-level `depends_on`** per service (short list or long form with `condition` / `restart`). See [contracts/depends-on-label.md](specs/001-container-health-monitor/contracts/depends-on-label.md) for the exact format.

Full run options and examples: [quickstart](specs/001-container-health-monitor/quickstart.md).

### Verification

1. Start your stack (including watch-dog) with the compose path and socket mounted.
2. Make a parent container unhealthy (e.g. break its healthcheck or kill the healthcheck process).
3. Check watch-dog logs: `docker logs watch-dog` (or your service name). You should see detection of the unhealthy parent, restart of the parent, then restart of dependents after the parent is healthy.

## Debugging / troubleshooting

### View logs

```bash
docker logs watch-dog
```

Or with Compose: `docker compose logs watch-dog`. Logs are emitted to stdout (e.g. key=value or JSON).

### Common issues

- **No parents discovered**  
  - Ensure `WATCHDOG_COMPOSE_PATH` or `COMPOSE_FILE` is set and points to the compose file path **inside the container**.  
  - Ensure the compose file is mounted (e.g. `.:/app:ro`) and the path is correct (e.g. `/app/docker-compose.yml`).

- **Compose file not readable**  
  - Check that the volume mount is read-only or read-write and the file exists at the given path inside the container (e.g. `docker exec watch-dog cat /app/docker-compose.yml`).

- **Containers not recognized (no `com.docker.compose.service`)**  
  - The monitor maps compose **service names** to containers using the `com.docker.compose.service` label set by Docker Compose. Containers not started by Compose (e.g. plain `docker run`) will not be linked to services and are ignored for discovery. Run your stack with `docker compose up` (or equivalent) so labels are applied.

For more detail on `depends_on` format and matching, see [contracts/depends-on-label.md](specs/001-container-health-monitor/contracts/depends-on-label.md) and [quickstart](specs/001-container-health-monitor/quickstart.md).

## Build and run (from source)

```bash
go build -o watch-dog ./cmd/watch-dog
./watch-dog   # needs access to Docker socket; set WATCHDOG_COMPOSE_PATH for discovery
```

Or build the image locally:

```bash
docker build -t watch-dog .
docker run -d --name watch-dog \
  -e WATCHDOG_COMPOSE_PATH=/app/docker-compose.yml \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd):/app:ro \
  watch-dog
```

## License

MIT. See [LICENSE](LICENSE).
