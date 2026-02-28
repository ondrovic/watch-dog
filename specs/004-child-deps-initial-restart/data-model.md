# Data Model: No Dependent Restarts on Initial Stack Start (004)

**Branch**: `004-child-deps-initial-restart` | **Date**: 2026-02-28

No new persistent storage. This feature adds a **runtime phase** (initial discovery) and **configuration** (wait duration from environment). The existing parent→dependents model (001, 003) is unchanged.

---

## Runtime phase: Initial discovery

| Concept | Description |
|--------|-------------|
| **Initial discovery** | A well-defined phase after monitor process start during which **no** recovery or dependent restarts are triggered. |
| **Phase start** | When the first full discovery cycle completes (first successful `BuildParentToDependents` at startup). |
| **Phase end** | Phase start + **configurable wait time** (see Config). Only after phase end does the monitor run startup reconciliation or react to health events with recovery. |
| **State** | In-memory only: "initial discovery in progress" until (phase start + wait duration) has elapsed; then "initial discovery complete". No persistence. |

---

## Config (from environment)

| Source | Attribute | Type | Default | Notes |
|--------|-----------|------|---------|--------|
| `WATCHDOG_INITIAL_DISCOVERY_WAIT` | Initial discovery wait duration | duration (Go ParseDuration) | 60s | Time to wait after first discovery before enabling recovery. Stacks vary (e.g. 120s–5min); operators set per deployment. See [contracts/env-initial-discovery.md](./contracts/env-initial-discovery.md). |

Other env (e.g. `RECOVERY_COOLDOWN`, `WATCHDOG_CONTAINER_NAME`, `LOG_LEVEL`, `LOG_FORMAT`) unchanged; see existing contracts.

---

## Validation

- **Wait duration**: If set, must parse as a positive duration (e.g. `30s`, `2m`, `5m`). Invalid or non-positive → use default 60s and log warning.
- **Phase boundary**: Recovery (startup reconciliation, event-driven, polling) must not run until phase end; implementation must gate all recovery entry points on "initial discovery complete".

---

## Relationship to existing model

- **Parent → Dependents**: Unchanged (003); discovery still builds the map at startup and on refresh.
- **Recovery sequence**: Unchanged when recovery is allowed (003: restart parent → wait healthy → restart dependents one at a time, self last).
- **Cooldown / in-flight**: Unchanged; applies only when a recovery is actually started (after initial discovery).
