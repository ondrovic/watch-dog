# watch-dog

A standalone Docker container that monitors parent/child container health and restarts unhealthy parents, then (after they become healthy) their dependents. It replaces [autoheal](https://github.com/willfarrell/docker-autoheal) for Compose stacks that use root-level `depends_on` and healthchecks.

## Features

- **Compose-native**: Discovers parent/child relationships from the compose file’s **root-level `depends_on`** (short or long form); no custom labels.
- **Correct order**: Restarts the parent first, waits until it is healthy, then restarts dependents (swarm-like behavior without Swarm).
- **Event-driven**: Uses Docker `health_status` events; optional 60s polling fallback for robustness.
- **Startup reconciliation**: On start, treats already-unhealthy parents and runs the full recovery sequence.

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

## How to use

### Configuration

| Variable | Description |
|----------|-------------|
| `WATCHDOG_COMPOSE_PATH` | Path inside the container to the compose file (e.g. `/app/docker-compose.yml`). |
| `COMPOSE_FILE` | Alternative; if set, the first path in a colon-separated list is used. |

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

## Spec and plan

- [Feature spec](specs/001-container-health-monitor/spec.md)
- [Implementation plan](specs/001-container-health-monitor/plan.md)
- [Tasks](specs/001-container-health-monitor/tasks.md)

## License

MIT. See [LICENSE](LICENSE).
