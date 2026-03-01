# Contract: Root-level depends_on (Compose)

**Consumer**: Docker Compose stacks. The watch-dog discovers parent/child relationships from the compose file’s root-level `depends_on` and restarts dependents when a parent becomes unhealthy.

Discovery is **only** from the compose file; the previous label-based `depends_on` contract is no longer used.

---

## Compose file path

- **Environment**: Set one of:
  - `WATCHDOG_COMPOSE_PATH` — path to a single compose file (e.g. `/app/docker-compose.yml`).
  - `COMPOSE_FILE` — path(s); if multiple (colon-separated), the first path is used.
- **When unset**: No compose-based discovery; the monitor runs but will not treat any container as a parent (no restarts).

- **Optional**: `WATCHDOG_DEPENDENT_RESTART_COOLDOWN` — duration (e.g. `90s`) for the dependent restart cooldown. When a container has multiple parents, the monitor skips restarting it again if it was already restarted within this window (at most one restart per dependent per cooldown). Default is 90s if unset or invalid.

---

## depends_on format (root-level per service)

Supported at `services.<name>.depends_on` in the compose file.

### Short form (list)

- **Syntax**: List of parent **service** names.
- **Example**:
  ```yaml
  services:
    app:
      depends_on:
        - db
        - redis
  ```

### Long form (map)

- **Syntax**: Map of parent service name to optional object with `condition`, `restart`, etc. Keys are parent names; values can be empty or contain `condition: service_healthy`, `restart: true`, etc.
- **Example**:
  ```yaml
  services:
    app:
      depends_on:
        db:
          condition: service_healthy
          restart: true
        redis:
          condition: service_started
  ```

Both forms are supported; the monitor uses the same parent→dependents logic for either.

---

## Matching rules

- Parent/dependent are identified by **compose service names** in the compose file.
- At runtime, service names are mapped to **container names** using the `com.docker.compose.service` label on running containers (set by Docker Compose).
- Services with no running container on the host are ignored; the monitor does not fail.
- Matching is case-sensitive.

---

## Compose example

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

Or short form:

```yaml
  torrent:
    image: my-torrent-image
    depends_on:
      - vpn
```

The monitor reads the compose file, builds parent→dependents from `depends_on`, and maps service names to containers via compose labels.
