# Contract: Dependent Restart Order and Self-Restart (003)

**Consumer**: Operators and integration tests. Extends [recovery-behavior](../001-container-health-monitor/contracts/recovery-behavior.md) for the “Restart dependents” step.

---

## Scope

When a parent has been restarted and is healthy, the monitor restarts each dependent container. This contract defines **order** and **self-restart** behavior so that all dependents are restarted reliably and in-flight work is not canceled.

---

## Order of dependents

- Dependents are restarted **one at a time** (sequentially): the next dependent’s restart is **started** only after the previous restart **call has returned** (success or error).
- The **order** of the dependent list is **deterministic** (e.g. sorted by container name) so that logs and behavior are reproducible.
- No parallel restart calls for dependents of the same parent.

---

## Self-restart (monitor is a dependent)

- When the monitor’s **own container name** is in the dependent list for the parent just recovered, the monitor **restarts all other dependents first**, then restarts **its own container last**.
- This ensures that when the process is stopped (because the watch-dog container is restarted), no other dependent’s restart or discovery/event operation is still in progress (no “context canceled” for those operations).
- The monitor’s container name is supplied by configuration (e.g. environment variable). If not set, no “self last” reordering is applied; all dependents are restarted in the same deterministic order.

---

## Errors and logging

- Each dependent restart attempt is **logged** (start and outcome: success or error).
- If a dependent restart **fails** (e.g. API error, not “context canceled”), the monitor **continues** with the next dependent in order; it does not cancel the remaining sequence.
- “Context canceled” should not occur for other dependents when the monitor restarts itself, because self is restarted last.
