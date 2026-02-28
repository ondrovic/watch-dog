# Quickstart: Dependent Restart Order and Self-Restart (003)

**Branch**: `003-fix-children-handling` | **Date**: 2026-02-28

This feature refines how watch-dog restarts **dependents** after a parent recovers: **one at a time** in a **deterministic order**, with the **monitor’s container restarted last** when it is in the dependent list (so in-flight work is not canceled).

## Prerequisites

- Same as [001 quickstart](../001-container-health-monitor/quickstart.md): Docker, healthchecks, root-level `depends_on`, compose path set.
- For “self last” behavior: set the monitor’s container name (e.g. `WATCHDOG_CONTAINER_NAME=watch-dog`) in the watch-dog service environment so the monitor can put itself last in the dependent list.

## Verify sequential dependent restart

1. **Setup**: Run a stack where a parent (e.g. `vpn` or `dler`) has **multiple** dependents (e.g. `ouroboros`, `tv`, `movies`, `watch-dog`). Ensure `WATCHDOG_COMPOSE_PATH` (or `COMPOSE_FILE`) is set and the compose file uses root-level `depends_on`.

2. **Trigger recovery**: Stop the parent container (e.g. `docker stop vpn`). Wait for the monitor to restart the parent and then restart dependents.

3. **Check logs**:
   - You should see one “restarted dependent” (or “restart dependent … error”) line **per** dependent, in a **stable order** (e.g. alphabetical by name), with **no** “context canceled” for “restart dependent” or “refresh discovery” when the monitor is among dependents.
   - If `WATCHDOG_CONTAINER_NAME` is set to the monitor’s container name, the **last** dependent restarted should be that container (e.g. `watch-dog`).

4. **Reproduce**: Repeat with the same stack; the order of “restarted dependent” lines should be the same across runs.

## Verify self-restart last (no context canceled)

1. **Setup**: Compose must list **watch-dog** as a dependent of the parent (e.g. `depends_on: [gluetun, qbittorrent, ..., watch-dog]`). Set `WATCHDOG_CONTAINER_NAME=watch-dog` (or the actual container name) in the watch-dog service.

2. **Trigger**: Stop the parent (e.g. `docker stop vpn`).

3. **Expected**:
   - Parent restarts and becomes healthy.
   - All dependents **except** watch-dog are restarted first; logs show “restarted dependent” for each.
   - **Then** watch-dog is restarted (last); the process exits and a new watch-dog process starts.
   - **No** “context canceled” errors for “restart dependent”, “refresh discovery”, or “docker events” **before** the final “restarted dependent” for watch-dog.

## Optional: Compose snippet for self name

```yaml
services:
  watch-dog:
    container_name: watch-dog
    image: ghcr.io/<owner>/watch-dog:latest
    environment:
      WATCHDOG_COMPOSE_PATH: /app/docker-compose.yml
      WATCHDOG_CONTAINER_NAME: watch-dog   # so monitor restarts itself last
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - .:/app:ro
    restart: unless-stopped
    depends_on:
      - gluetun
      - qbittorrent
      # ...
```

When `WATCHDOG_CONTAINER_NAME` is omitted, the monitor still restarts all dependents one at a time in a deterministic order; it just does not reorder itself to last (so if it appears in the middle of the list, restarting it may still cancel in-flight work—setting the env is recommended when watch-dog is a dependent).
