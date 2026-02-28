# Data Model: Correct Handling of Dependent (Child) Containers (003)

**Branch**: `003-fix-children-handling` | **Date**: 2026-02-28

No new persistent storage. This feature refines **runtime behavior** and **in-memory ordering** of the existing parent→dependents model.

---

## Runtime entities (unchanged from 001)

### Container (runtime view)

| Attribute      | Source                | Notes |
|----------------|-----------------------|--------|
| ID             | Docker API            | Container ID. |
| Name           | Docker API            | Container name; used for depends_on matching and for “self” when reordering. |
| State          | Docker API            | running, exited, etc. |
| Health.Status  | Docker API (inspect)  | `healthy`, `unhealthy`, `starting`, or absent. |
| Labels         | Docker API            | Used for compose service mapping (`com.docker.compose.service`). |

### Parent → Dependents (compose-based)

- **Key**: Parent container name (from compose service name mapped to container name).
- **Value**: **Ordered** list of dependent container names (see below).
- **Built from**: Compose file root-level `depends_on` via `BuildParentToDependentsFromCompose`; order is made deterministic (e.g. sort by name) and optionally “self last” when the monitor’s container name is known.

---

## Ordering rules (new / clarified)

### Dependent list order

1. **Base order**: Dependents for a parent are ordered **deterministically** (e.g. lexicographic by container name) over the compose-derived list—**not** by literal compose file declaration order—so that restarts are reproducible and “one at a time” has a defined sequence.
2. **Self last**: If the monitor’s container name (e.g. from `WATCHDOG_CONTAINER_NAME`) is present in the dependent list, it is moved to **last** in the list before any restart is performed. All other dependents are restarted first; the monitor’s container is restarted last so that in-flight operations are not canceled.

### Recovery sequence (state flow)

1. **Idle** → **Parent unhealthy**: Unchanged (event or poll).
2. **Parent unhealthy** → **Restarting parent**: Unchanged.
3. **Restarting parent** → **Waiting for healthy**: Unchanged.
4. **Healthy** → **Restarting dependents**: For each dependent in the **ordered** list (one at a time): call restart, wait for call to return (success or error), then proceed to next. No concurrent restart calls. If a restart fails (non-canceled error), log and continue with the next.
5. **Restarting dependents** → **Idle**: When all dependents in the list have been given a restart attempt, return to idle.

No new state is stored; “self” container name is read from environment at start (or when building the list) and used only to reorder the list for that recovery run.

---

## Validation

- **Self name**: If `WATCHDOG_CONTAINER_NAME` (or chosen env) is set, it must match a container name that can appear in the dependent list (no structural validation beyond use when reordering).
- **Order**: The list passed to RestartDependents must be deterministic and, when self is in the list, self must be last.
