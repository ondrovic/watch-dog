# Data Model: Fix Recovery When Containers Are Gone or Unrestartable (005)

**Branch**: `005-fix-recovery-stale-container` | **Date**: 2026-03-01

No new persistent storage. This feature adds an **in-memory unrestartable set** and **error classification** used during recovery. The existing parent→dependents and recovery sequence (001, 003, 004) are unchanged except for the new skip and add-on-failure behavior.

---

## Unrestartable set

| Concept | Description |
|--------|-------------|
| **Unrestartable set** | Set of container IDs for which a restart (or inspect during wait-for-healthy) has failed with an **unrestartable** error. Stored in memory on the recovery Flow (or equivalent); shared by event-driven and polling recovery. |
| **Unrestartable error** | A Docker API error that indicates the container cannot be restarted: (1) "No such container", (2) "marked for removal" / "cannot be started" in removal context, (3) "joining network namespace" and "No such container" (dependency missing). Classified by inspecting the error message. |
| **Add to set** | When `Restart` or `Inspect` fails and the error is classified as unrestartable, the **container ID** used in that call is added to the set. For parent: the parent ID. For dependent: the dependent’s container ID (resolved when restarting dependents). |
| **Check before run** | Before running the full recovery sequence for a parent (or before restarting a dependent), if the container ID is in the set, the monitor **skips** that restart and logs that recovery is skipped because the container is unrestartable. |
| **Re-discovery** | Discovery (event and polling) already refreshes the container list and parent→dependents. When a container is recreated (e.g. by an updater), it has a **new** ID. That new ID is not in the unrestartable set, so the monitor will run recovery for it as normal. No explicit "clear" of the set by name is required. |

---

## Bounding the set

| Rule | Description |
|------|-------------|
| **Cap** | The set size is bounded (e.g. max 100 entries). When adding would exceed the cap, remove an existing entry (e.g. oldest or one not in the latest container list). |
| **Pruning** | Optionally, when a fresh container list is available (e.g. from polling or after event discovery), remove from the set any ID that does **not** appear in the current list. That keeps the set from growing indefinitely and only stores IDs that might still be passed to recovery (e.g. from stale events). |

---

## Validation

- **Classification**: Only errors that match the documented unrestartable patterns are added to the set. Other errors (e.g. temporary daemon failure) do not add to the set and may be retried.
- **Skip**: Recovery sequence (parent restart, wait healthy, dependent restarts) is skipped for a container ID that is in the set; no Docker restart call is made for that ID.
- **Visibility**: First failure for an ID is logged with reason; skip is logged so operators see that retries are limited.

---

## Relationship to existing model

- **Parent → Dependents**: Unchanged (003); discovery still builds the map; dependent restart order unchanged.
- **Recovery sequence**: Unchanged when the container ID is **not** in the unrestartable set. When it is, the sequence is skipped (no restart, no wait, no dependents for that parent; or that dependent is skipped).
- **Cooldown / in-flight**: Unchanged; cooldown is per parent **name**; unrestartable is per **ID**. So after a new container (new ID) for the same name appears, cooldown still applies by name, but recovery is allowed because the new ID is not in the set.
- **Initial discovery (004)**: Unchanged; unrestartable check happens inside recovery when we are about to run the sequence; initial discovery still gates whether recovery runs at all.

---

## Last-known parent ID (FR-007 proactive restart)

| Concept | Description |
|--------|-------------|
| **Last-known parent ID** | A map (parent name → container ID) storing the last container ID seen for each parent. Updated after each discovery when we have a current container list (event path and polling). Used to detect "parent has new ID" (current ID ≠ last-known). |
| **Detection** | After discovery: for each parent name, current ID = from container list; if current ID != last-known ID for that parent and the parent is healthy, trigger proactive restart of dependents (run RestartDependents only, with same cooldown); then set last-known[parentName] = current ID. On first run, last-known may be empty so current ID is treated as "new" (optional: skip proactive restart on first discovery to avoid restarting all dependents at startup, or allow per spec—spec says "when the monitor observes a parent has a new ID and is healthy"). |
| **Proactive restart** | Restart dependents of that parent only (no parent restart). Reuse existing RestartDependents logic; same DependentRestartCooldown applies. |
