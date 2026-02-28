# Contract: Recovery behavior

**Consumer**: Operators and integration tests; defines the observable behavior of the watch-dog.

---

## Trigger

- The monitor acts when it observes that a **parent** container has become **unhealthy**.
- “Parent” here means: a container that is referenced by at least one other container’s `depends_on` label (see [depends-on-label.md](./depends-on-label.md)).
- Observation is via Docker `health_status` events and/or periodic inspection of container health (polling fallback).

---

## Sequence (guaranteed order)

1. **Restart parent**: The monitor restarts the container that became unhealthy (single restart call).
2. **Wait for healthy**: The monitor waits until that container’s health status is reported as `healthy`, or until a configured timeout (e.g. 5 minutes). It does **not** restart any dependents until this step succeeds (parent healthy or timeout without restarting dependents).
3. **Restart dependents**: After the parent is healthy, the monitor restarts every container that lists this parent in its `depends_on` label. Order among multiple dependents is not specified.

---

## Idempotency and errors

- **Restart**: Restart is idempotent (Docker restart on already-running or stopped container is valid). A dependent that is already restarting or stopped may still be sent a restart.
- **Missing/mismatched names**: If `depends_on` references a container that does not exist on the host, that dependency is ignored. The monitor does not fail or restart dependents for non-existent parents.
- **Multiple parents**: If container C lists parents A and B, and A becomes unhealthy, the monitor runs the sequence for A (restart A → wait healthy → restart C). If B later becomes unhealthy, it runs the same for B (restart B → wait healthy → restart C). C may be restarted more than once in a short window; this is acceptable.

---

## Startup behavior

- If at monitor startup some parents are already unhealthy, the monitor treats that as an unhealthy event and applies the same sequence: restart parent → wait healthy → restart dependents.

---

## No persistent config

- The monitor does **not** require a list of container names or a static dependency graph. Discovery is entirely from container labels and runtime state (100% dynamic per spec).
