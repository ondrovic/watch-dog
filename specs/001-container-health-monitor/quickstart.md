# Quickstart: Container Health Monitor (Watch-Dog)

**Branch**: `001-container-health-monitor` | **Date**: 2025-02-28

## Prerequisites

- Docker (or compatible runtime) on the host.
- Containers that should be monitored must have a **healthcheck** so that “unhealthy” can be detected.
- Dependencies must be declared with **root-level depends_on** in your Docker Compose file (short or long form). See [contracts/depends-on-label.md](./contracts/depends-on-label.md).
- The monitor must have access to the compose file and `WATCHDOG_COMPOSE_PATH` or `COMPOSE_FILE` set so it can read root-level `depends_on`.

## Run the monitor

### Option A: Pre-built image (GHCR)

After the image is published to GitHub Container Registry:

```bash
docker run -d \
  --name watch-dog \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/<owner>/watch-dog:latest
```

Replace `<owner>` with the GitHub org or username (e.g. `ghcr.io/<owner>/watch-dog:latest`). When built via GitHub Actions, the image is `ghcr.io/<repository_owner>/watch-dog:latest`.

### Option B: Build from source

From the repository root (Go 1.21+):

```bash
go build -o watch-dog ./cmd/watch-dog
# Run with Docker socket access (Linux)
./watch-dog
# Or with DOCKER_HOST set if using remote daemon
DOCKER_HOST=tcp://... ./watch-dog
```

Or build the Docker image and run:

```bash
docker build -t watch-dog .
docker run -d --name watch-dog -v /var/run/docker.sock:/var/run/docker.sock watch-dog
```

## Compose example

Add the watch-dog service and use **root-level depends_on** for dependents. Mount the compose file and set `WATCHDOG_COMPOSE_PATH` (or `COMPOSE_FILE`):

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
    # ... define healthcheck ...

  torrent:
    image: my-torrent-image
    depends_on:
      vpn:
        condition: service_healthy
        restart: true
    # ...
```

Short form is also supported:

```yaml
  torrent:
    image: my-torrent-image
    depends_on:
      - vpn
```

## Verify

- Make a parent container unhealthy (e.g. break its healthcheck or kill the healthcheck process).
- Check logs of the watch-dog container: you should see that it detected the unhealthy parent, restarted it, waited for healthy, then restarted dependents.
- Containers without a healthcheck are not triggered as “unhealthy”; only services that appear as dependents in the compose file’s root-level `depends_on` are restarted after their parent is healthy.

## Configuration

- **WATCHDOG_COMPOSE_PATH** — Path to the compose file for root-level `depends_on` discovery (e.g. `/app/docker-compose.yml`). Required for discovery.
- **COMPOSE_FILE** — Alternative; if set, the first path (colon-separated list) is used as the compose file.
- Health-wait timeout (default e.g. 5 minutes).
- Poll interval if using polling fallback (default e.g. 60s).
- Log format (e.g. JSON).
