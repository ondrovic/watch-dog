# Contract: Initial Discovery Behavior

**Feature**: 004-child-deps-initial-restart | **Date**: 2026-02-28  
**Consumer**: Operators and integration tests; defines when the monitor does **not** run recovery.

---

## Initial discovery phase

- **Definition**: The period from monitor process start until **first discovery cycle has completed** plus **WATCHDOG_INITIAL_DISCOVERY_WAIT** (see [env-initial-discovery.md](./env-initial-discovery.md)).
- **First discovery cycle**: The first successful call that builds the parent→dependents map from the compose file at startup (e.g. `BuildParentToDependents`). No recovery or dependent restarts run before this plus the wait time have elapsed.

---

## Behavior during initial discovery

- The monitor MUST **NOT** run recovery (restart parent, wait healthy, restart dependents) during the initial discovery phase.
- The monitor MUST **NOT** restart any dependent container during this phase, including when a parent is already unhealthy or stopped when first observed.
- The monitor MAY perform discovery, list containers, subscribe to Docker events, and log; it MUST NOT invoke restart or run the full recovery sequence for any parent until the phase has ended.

---

## Behavior after initial discovery

- Once the initial discovery phase has ended:
  - **Startup reconciliation**: The monitor runs once the same logic as today: find parents that are already unhealthy or stopped and run the full recovery sequence (restart parent → wait healthy → restart dependents per 003).
  - **Event-driven and polling**: Health-status events and polling that detect unhealthy or stopped parents trigger recovery as today.
- Recovery order and dependent restart order remain as in [001 recovery-behavior](../001-container-health-monitor/contracts/recovery-behavior.md) and [003 dependent-restart-order](../003-fix-children-handling/contracts/dependent-restart-order.md).

---

## Observability

- At startup, the monitor MUST log (INFO) that the initial discovery phase has started and the configured wait duration (e.g. from env or default).
- When the wait has elapsed, the monitor MUST log (INFO) that initial discovery is complete and normal recovery is enabled.
- Operators use these logs to verify that no recovery ran during the phase (e.g. after `docker compose up`).

---

## Relationship to recovery-behavior.md (001)

- [recovery-behavior.md](../001-container-health-monitor/contracts/recovery-behavior.md) "Startup behavior" stated: "If at monitor startup some parents are already unhealthy, the monitor treats that as an unhealthy event and applies the same sequence." This feature **overrides** that for the initial discovery phase: during that phase, the monitor does **not** treat already-unhealthy or stopped parents as a trigger for recovery. After the phase, startup reconciliation applies as before (treat already-unhealthy/stopped parents and run the sequence).
