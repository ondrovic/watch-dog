# Data Model: Container Health Monitor

**Branch**: `001-container-health-monitor` | **Date**: 2025-02-28

No persistent storage is used; all state is derived from the Docker API at runtime.

---

## Runtime entities (from Docker API)

### Container (runtime view)

| Attribute      | Source                | Notes |
|----------------|-----------------------|--------|
| ID             | Docker API            | Container ID (e.g. short or full). |
| Name           | Docker API            | Container name (used in depends_on matching). |
| State          | Docker API            | running, exited, etc. |
| Health.Status  | Docker API (inspect)  | `healthy`, `unhealthy`, `starting`, or absent (no healthcheck). |
| Labels         | Docker API            | Key-value; we read `depends_on` only. |

**Validation**: We only act on containers that exist on the same host and are listed/inspectable. Containers without a healthcheck are treated as “always healthy” for the purpose of “wait for parent healthy” (per spec assumptions).

---

## Derived structures (in-memory)

### Parent → Dependents map

- **Key**: Parent container name (string).
- **Value**: Set or list of container names that have `depends_on` containing that parent.
- **Built from**: List all running containers with label `depends_on`; parse label value (comma-separated parent names); for each parent name, add the current container to that parent’s dependents.
- **Updated**: On each evaluation (event or poll), rebuild from current container list and labels; no caching across cycles required for correctness.

### depends_on label value

- **Format**: Comma-separated list of parent container names.
- **Examples**: `"vpn"`, `"vpn,db"`.
- **Parsing**: Split by comma, trim spaces, drop empty; each segment is one parent name. Matching to running containers is by exact name (case-sensitive).
- **Validation**: If a name in `depends_on` does not match any container on the host, it is ignored (per spec: do not fail; skip non-existent parents).

---

## State transitions (recovery flow)

1. **Idle** → **Parent unhealthy**: Monitor observes health_status=unhealthy (or poll sees `State.Health.Status == "unhealthy"`).
2. **Parent unhealthy** → **Restarting parent**: Monitor calls restart for that container.
3. **Restarting parent** → **Waiting for healthy**: Monitor polls that container’s health until status is `healthy` or timeout (e.g. 5 minutes).
4. **Healthy** → **Restarting dependents**: For each container that lists this parent in `depends_on`, call restart.
5. **Restarting dependents** → **Idle**: When all such dependents have been sent restart, return to idle for that parent.

No persistent state is stored; each unhealthy event is processed independently. If the same parent becomes unhealthy again later, the flow runs again.
