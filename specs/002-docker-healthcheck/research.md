# Research: Docker Healthcheck and Log Observability (002-docker-healthcheck)

**Branch**: `002-docker-healthcheck` | **Date**: 2026-02-28

## 1. Healthcheck: Docker API check inside the container

**Decision**: Add the **Docker CLI** to the runtime image (Alpine) and use a HEALTHCHECK that runs `docker info` (or `docker ps -q`) with the socket already mounted. Use default intervals in the Dockerfile (interval 15s, start_period 20s, timeout 10s, retries 2); compose example uses variable substitution from .env (DOCKER_HEALTHCHECK_INTERVAL, DOCKER_HEALTHCHECK_RETRIES, DOCKER_HEALTHCHECK_START_PERIOD, DOCKER_HEALTHCHECK_TIMEOUT) so users override without rebuilding.

**Rationale**: Spec requires a minimal Docker API check (no HTTP server). The monitor already has the socket mounted; running `docker info` from inside the container proves the process can talk to the daemon. Alpine can install `docker-cli` via `apk add docker-cli`. Alternative of a small Go healthcheck binary or `wget` to a local endpoint would require either exposing HTTP (rejected) or a second binary (more complexity).

**Alternatives considered**:
- **Process-only check** (e.g. `pgrep watch-dog`): Does not verify Docker API; spec requires Docker API check.
- **Separate health binary in image**: Adds build and maintenance; Docker CLI is one `apk add` and well-understood.
- **No Dockerfile HEALTHCHECK, only compose**: Spec requires both; image should be self-contained with defaults.

---

## 2. LOG_LEVEL and slog in Go

**Decision**: Read **LOG_LEVEL** from the environment at startup (in `internal/docker/log.go` or `main`); map to `slog.Level` (Debug, Info, Warn, Error). Set `slog.HandlerOptions{Level: level}`. Default to `slog.LevelInfo` when unset or invalid. Use case-insensitive comparison for values (DEBUG, debug, Debug all map to LevelDebug).

**Rationale**: `log/slog` supports level filtering natively; no custom filtering needed. Single place (init or main) reads env and configures the default handler. Document allowed values in README.

**Alternatives considered**:
- Custom level names: Spec asks for DEBUG, INFO, WARN, ERROR; standard names reduce documentation and match slog.
- Reading LOG_LEVEL per log call: Unnecessary; one-time init is sufficient.

---

## 3. LOG_FORMAT: compact, timestamp, json

**Decision**: Implement **custom slog handlers** (or wrap TextHandler/JSONHandler) to produce: (1) **compact** – `[LEVEL] message key=value...`; (2) **timestamp** – `2006-01-02T15:04:05Z07:00 [LEVEL] message key=value...`; (3) **json** – `slog.NewJSONHandler(os.Stdout, ...)` (one JSON object per line). Read **LOG_FORMAT** from env at startup; default to **timestamp** when unset or invalid. Handler selection in the same init as LOG_LEVEL.

**Rationale**: Spec defines custom format names (compact, timestamp) and optional json. slog’s TextHandler is key=value and was reported hard to read; custom handlers allow exact spec wording. Default timestamp gives ordering and familiarity from other containers.

**Alternatives considered**:
- Only slog TextHandler with different options: Would not yield `[LEVEL]` or leading timestamp in the requested form without a custom handler.
- External log library: slog is stdlib and already in use; custom handler is minimal code.

---

## 4. Default for LOG_FORMAT when unset

**Decision**: Default **LOG_FORMAT** to **timestamp** when the variable is unset or not one of the supported values (compact, timestamp, json). Document in README and in contracts.

**Rationale**: Spec allows plan to set default; timestamp is the most informative for operators and matches the user’s “timestamp [Level] message” example.

---

## 5. README examples for all LOG_LEVEL and LOG_FORMAT values

**Decision**: Add a **Configuration** (or **Environment**) section in README that includes: (1) a table of **LOG_LEVEL** values (DEBUG, INFO, WARN, ERROR) with a one-line description or example env snippet for each; (2) a table of **LOG_FORMAT** values (compact, timestamp, json if implemented) with an example log line for each style. Compose example in README and quickstart uses `environment: - LOG_LEVEL=INFO - LOG_FORMAT=timestamp` (or equivalent) so users can copy and change values.

**Rationale**: Spec (FR-006, FR-007, SC-007) requires README examples for every supported value so users can copy and troubleshoot. Tables plus one compose snippet satisfy this.

---

## 6. Healthcheck parameters in Dockerfile vs compose

**Decision**: **Dockerfile**: Use a single HEALTHCHECK instruction with literal defaults (e.g. `--interval=15s --start-period=20s --timeout=10s --retries=2` and `CMD docker info` or `CMD docker ps -q`). **Compose example**: Use `healthcheck:` with variable substitution: `interval: ${DOCKER_HEALTHCHECK_INTERVAL:-15s}`, `start_period: ${DOCKER_HEALTHCHECK_START_PERIOD:-20s}`, `timeout: ${DOCKER_HEALTHCHECK_TIMEOUT:-10s}`, `retries: ${DOCKER_HEALTHCHECK_RETRIES:-2}`, so when users run with a `.env` file the healthcheck parameters come from .env. Document in README that the image has built-in defaults and compose allows overrides via .env.

**Rationale**: Dockerfile HEALTHCHECK does not support env substitution at runtime; values are fixed at build time. Compose does support `${VAR}` for healthcheck; so image ships with sensible defaults and compose users get .env-driven config without rebuilding.
