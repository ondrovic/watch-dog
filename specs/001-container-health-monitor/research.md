# Research: Container Health Monitor (001-container-health-monitor)

**Branch**: `001-container-health-monitor` | **Date**: 2025-02-28

## 1. Docker API from Go

**Decision**: Use the official Docker Engine API client for Go: `github.com/docker/docker/client` with `github.com/docker/docker/api/types` (and related types). Communicate with the daemon via `DOCKER_HOST` (default unix socket) or `DOCKER_API_VERSION` if needed.

**Rationale**: Official client is maintained, supports listing containers, inspecting (labels, health status), restart, and event stream. No need to shell out to `docker` CLI; single binary and simpler error handling.

**Alternatives considered**:
- Shell out to `docker` / `docker inspect`: works but brittle, requires docker binary in image, harder to test.
- Third-party Docker SDKs: redundant when official client is sufficient.

---

## 2. Health detection: events vs polling

**Decision**: Prefer **Docker events** with filter `event=health_status` as the primary trigger. Optionally add a **polling fallback** (e.g. every 60s) to reconcile state if events are missed (e.g. monitor restarted, or daemon restart).

**Rationale**: Spec and example (`watchdog.sh`) use `docker events --filter event=health_status`; event-driven avoids busy polling. Polling fallback improves robustness when event stream has gaps.

**Alternatives considered**:
- Events only: simpler but risk missing transitions if monitor or daemon restarts.
- Polling only: higher latency and unnecessary load; rejected.

---

## 3. depends_on label format

**Decision**: Support a **comma-separated list** of parent container names in the label `depends_on`. Container names are trimmed; empty segments ignored. Example: `depends_on: "vpn"` or `depends_on: "vpn,db"`. Matching is by container **name** (as seen by `docker ps` / API), case-sensitive.

**Rationale**: Aligns with example script (`index .Config.Labels "depends_on"` and substring check for parent). Comma-separated allows multiple parents (FR-006) without new syntax.

**Alternatives considered**:
- JSON array in label: more precise but harder to set in compose (requires escaping).
- Single parent only: would not satisfy FR-006 (multiple parents).

---

## 4. Wait-for-health after parent restart

**Decision**: After restarting a parent, **poll** that container’s inspect (e.g. `State.Health.Status`) until it becomes `healthy` or a **timeout** (e.g. 5 minutes) is reached. Only then restart dependents. If timeout, do not restart dependents (per spec: must not restart dependents until parent is healthy).

**Rationale**: Docker API does not push “became healthy” as a single event with guaranteed ordering; polling inspect is reliable. Timeout prevents indefinite wait when a parent never recovers.

**Alternatives considered**:
- Rely only on next `health_status: healthy` event: possible but requires correlating event to the restart we triggered; polling is simpler and deterministic.
- No timeout: risk of hanging forever; spec says dependents must not restart until parent healthy, so timeout is required.

---

## 5. Retry policy for unhealthy parent

**Decision**: **No automatic retry loop** for restarting the same parent repeatedly. One restart per unhealthy transition; if the parent stays unhealthy, the next `health_status` event (or polling cycle) can trigger another restart. Avoids thundering herd and infinite restart loops; optional config (e.g. max restarts per container per hour) can be added later if needed.

**Rationale**: Spec says “the monitor may retry restarting the parent according to policy”; keeping policy minimal (one restart per observed unhealthy) is safe and simple. Events or polling will naturally retrigger if the parent stays unhealthy.

---

## 6. Image build and GitHub Container Registry (GHCR)

**Decision**: Use **GitHub Actions** to build the Go binary (single stage or multi-stage Dockerfile), build the image, and push to **GHCR** (`ghcr.io/<owner>/watch-dog` or similar). Trigger on push to main (and optionally tags). Use `docker/login-action` and `docker/build-push-action` with `registry: ghcr.io`.

**Rationale**: Spec and clarifications require image built and hosted via GitHub; GHCR is the standard for GitHub-hosted images and works with same repo and permissions.

**Alternatives considered**:
- Docker Hub: user requested GHCR.
- External CI (e.g. Jenkins): adds dependency; GitHub Actions keeps everything in repo.

---

## 7. Observability

**Decision**: **Structured logs** (e.g. JSON or key=value) to stdout: log when an unhealthy parent is detected, when restart is triggered, when waiting for health, when dependents are restarted, and on errors. No metrics server or tracing in scope unless added later.

**Rationale**: Container-native apps log to stdout; structured logs allow parsing in any orchestrator. Spec does not mandate metrics; logging is sufficient for MVP.
