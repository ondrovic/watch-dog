# Quickstart: No Dependent Restarts on Initial Stack Start (004)

**Branch**: `004-child-deps-initial-restart` | **Date**: 2026-02-28

This feature prevents the monitor from restarting parents or dependents during an **initial discovery** phase after startup. That phase = first discovery cycle + a **configurable wait time** (env var). Use it so `docker compose up` (including when watch-dog is a dependent via `depends_on`) does not cause cascade restarts.

## Prerequisites

- Same as [001 quickstart](../001-container-health-monitor/quickstart.md): Docker, healthchecks, root-level `depends_on`, compose path set.
- Optional but recommended when watch-dog is a dependent: `WATCHDOG_CONTAINER_NAME` (see [003 quickstart](../003-fix-children-handling/quickstart.md)).

## Configure initial discovery wait

Set the wait time so your stack has time to become ready before the monitor may run recovery. Stacks vary (e.g. 120 seconds to 5 minutes).

```yaml
services:
  watch-dog:
    environment:
      WATCHDOG_COMPOSE_PATH: /app/docker-compose.yml
      WATCHDOG_INITIAL_DISCOVERY_WAIT: 120s   # e.g. 2 minutes for your stack; default 60s
```

Examples: `30s`, `2m`, `5m`. If unset, default is **60s**. Invalid values fall back to 60s with a warning in logs.

## Verify no restarts on compose up

1. **Setup**: Compose with a parent (e.g. VPN) and dependents (e.g. watch-dog, other apps). Set `WATCHDOG_INITIAL_DISCOVERY_WAIT` to at least the time your stack needs to become healthy (e.g. `120s` or `5m`).

2. **Bring stack up**: Run `docker compose up -d` (or `docker compose up`) so the full stack, including watch-dog, starts together.

3. **Check logs**:
   - You should see a line that initial discovery has started and the wait duration (e.g. `initial discovery started, wait=2m0s`).
   - After that duration, a line that initial discovery is complete and recovery is enabled.
   - **No** "parent needs recovery" or "restart dependent" during the initial discovery phase; no cascade of restarts.

4. **Check containers**: After the wait has elapsed, dependent container start times (or restart counts) should not show restarts caused by the monitor during startup.

## Verify recovery still works after phase

1. **Setup**: Same stack; wait until initial discovery is complete (see logs).

2. **Trigger**: Stop the parent (e.g. `docker stop vpn`).

3. **Expected**: Monitor runs full recovery (restart parent → wait healthy → restart dependents one at a time, per 003). Logs show "parent needs recovery" and "restarted dependent" as before.

## Verify no cascade after phase

1. **Setup**: After initial discovery is complete (see logs), trigger **one** recovery (e.g. `docker stop <parent>` once).
2. **Expected**: Recovery runs once; the stack reaches a stable state. No sustained or repeated restart loop (no restarts that repeatedly re-trigger). You can confirm by checking that container restarts settle (e.g. one recovery cycle, then idle).

## Optional: watch-dog as dependent (depends_on)

You can keep watch-dog as a dependent of the parent in compose. With this feature and an appropriate `WATCHDOG_INITIAL_DISCOVERY_WAIT`, `docker compose up` should not cause a cascade; the monitor will not restart itself or other dependents during initial discovery.

```yaml
services:
  watch-dog:
    container_name: watch-dog
    image: ghcr.io/<owner>/watch-dog:latest
    environment:
      WATCHDOG_COMPOSE_PATH: /app/docker-compose.yml
      WATCHDOG_CONTAINER_NAME: watch-dog
      WATCHDOG_INITIAL_DISCOVERY_WAIT: 120s
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - .:/app:ro
    restart: unless-stopped
    depends_on:
      - gluetun
      - qbittorrent
```

If your stack takes longer to be ready (e.g. 5 minutes), set `WATCHDOG_INITIAL_DISCOVERY_WAIT=5m`.
