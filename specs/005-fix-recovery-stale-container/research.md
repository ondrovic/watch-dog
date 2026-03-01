# Research: 005-fix-recovery-stale-container

**Branch**: `005-fix-recovery-stale-container` | **Date**: 2026-03-01

## 1. Classifying Docker API errors as "unrestartable"

**Decision**: Classify by inspecting the Go `error` returned from `ContainerRestart` and `ContainerInspect`. The Docker Engine API returns errors as standard Go `error`; the daemon encodes the condition in the error message. Use **string matching** on `err.Error()` for the known cases:

- **No such container**: message contains "No such container" (container removed or ID invalid).
- **Marked for removal**: message contains "marked for removal" or "cannot be started" in the context of removal (container is being or has been removed).
- **Dependency missing (e.g. network namespace)**: message contains "joining network namespace" and "No such container" (another container’s network namespace is referenced but that container is gone).

**Rationale**: The Docker Go client does not expose typed error codes for these cases; the API returns `errdefs.ErrNotFound` and similar for some, but message-based classification is reliable and avoids depending on internal errdefs. We treat any match as "unrestartable for this container ID" and do not retry that ID.

**Alternatives considered**: Use `errors.Is(err, errdefs.ErrNotFound)` for "no such container" (possible but not all daemon paths use it; message is more consistent across Docker versions). Typed errors per daemon (rejected: not in client contract).

---

## 2. Where to record and check "unrestartable"

**Decision**: Maintain an **in-memory set of container IDs** for which restart (or inspect-after-restart) has failed with an unrestartable error. Store and check this inside **recovery** (e.g. on `recovery.Flow`), so the same Flow instance used by events and polling shares the set. Before calling the Docker restart in `RunFullSequence` (and before restarting dependents), if the container ID is in the set, **skip** the sequence and log once at INFO that recovery is skipped for that parent/dependent because it is unrestartable.

**Rationale**: Flow is already shared by the event loop and polling in main; a single set prevents retrying the same bad ID from either path. Keying by ID (not name) ensures that when a new container is created for the same service name, it has a new ID and is not in the set, so recovery runs for the new instance (FR-004, SC-003).

**Alternatives considered**: Key by service name (rejected: would block recovery for the new ID until we clear the name, adding complexity). Store in main and pass into Flow (acceptable but Flow already has the error; keeping the set next to the code that adds to it is simpler).

---

## 3. When to add an ID to the unrestartable set

**Decision**: Add the container ID to the set **as soon as** a restart (or inspect during wait-for-healthy) fails with a classified unrestartable error. Do not add on other errors (e.g. temporary daemon busy). For **dependent** restart failures, add the **dependent’s** container ID (we look up ID by name from discovery/list so we have it when we call Restart). That way we stop retrying that specific container instance; when the dependent is recreated with a new ID, it will not be in the set.

**Rationale**: Spec requires bounded retries (FR-001–FR-003); adding on first unrestartable failure achieves that. One failure is enough to classify the container as unrestartable for the rest of its lifetime (that ID).

**Alternatives considered**: Add only after N failures (rejected: spec says limit retries, not continue N times). Exponential back-off without a set (rejected: we need to skip by ID so that the same ID is not retried on every poll/event).

---

## 4. Bounding or pruning the unrestartable set

**Decision**: **Cap the set size** (e.g. max 100 IDs) and/or **prune** IDs that no longer appear in `ListContainers` (all). When adding would exceed the cap, remove the oldest entry (FIFO) or one not seen in the last list. Pruning: when we have a fresh container list (e.g. in polling or after event refresh), remove any unrestartable ID that is not in the current list so we don’t grow indefinitely on long-lived stacks with many replacements.

**Rationale**: Prevents unbounded memory growth; spec does not require infinite history. Pruning is consistent with "re-discovery": if an ID is no longer in the list, we don’t need to remember it for skip logic (we’ll never see that ID again as input).

**Alternatives considered**: Unbounded set (rejected: could grow over time). TTL per ID (acceptable but pruning on list is simpler and keeps set relevant to current state).

---

## 5. Logging and operator visibility

**Decision**: (1) On **first** unrestartable failure for a container: log at ERROR with message that recovery failed and the reason (container gone / marked for removal / dependency missing), and that the monitor will not retry this container ID. (2) When **skipping** recovery because the ID is already unrestartable: log at INFO (or DEBUG to reduce noise) that recovery is skipped for parent/dependent X because the container is unrestartable (optional: "retries limited" or "will retry when a new instance appears"). (3) Use structured attributes: parent/dependent name, container ID (short), reason.

**Rationale**: Meets FR-005 and SC-002: operators see why recovery failed and that retries are limited; they can tell "recovery attempted and failed (retries limited)" vs "recovery in progress."

**Alternatives considered**: Log every skip at WARN (rejected: could spam if polling runs often). ERROR only on first failure, DEBUG for skips (acceptable; INFO for first skip after failure gives a clear "we’re not retrying" signal).

---

## 6. Dependent restart and Inspect failure

**Decision**: (1) **Dependent restart**: When `Restart(ctx, dependentID)` fails with an unrestartable error, add the **dependent’s** ID to the set and log; do not retry that dependent ID. The dependent may be restarted again when it appears with a new ID (e.g. after updater replaces it). (2) **WaitUntilHealthy Inspect failure**: If `Inspect(ctx, parentID)` during wait-for-healthy returns an unrestartable error (e.g. no such container), add parent ID to the set, log, and return (do not restart dependents). Same as parent restart failure: we will not retry that parent ID.

**Rationale**: Aligns with FR-003 (dependency missing) and FR-006 (other containers still monitored). Dependent and parent are treated symmetrically: one unrestartable ID does not block others.

**Alternatives considered**: Only track parent IDs (rejected: dependent restart can also fail with "joining network namespace ... No such container"; we must bound retries for that dependent too).

---

## 7. Detecting "parent has new ID" and proactive dependent restart (FR-007)

**Decision**: Maintain a **last-known container ID per parent name** (e.g. map[string]string, keyed by parent name). After each discovery (event path and polling), we have the current container list and thus current ID per parent name. For each parent: (1) Compare current ID to last-known ID for that parent name. (2) If they differ (including first run where last-known is empty → treat as "new") and the parent is **healthy** (from Inspect or from list/event), run the **dependent-restart sequence only** for that parent (do not restart the parent; reuse RestartDependents with the same cooldown as normal recovery). (3) Update last-known ID for that parent to the current ID. Store this map in the same place as the unrestartable set (e.g. on Flow or in main) so event and polling paths share it. Use the same DependentRestartCooldown so we do not restart the same dependent repeatedly when multiple parents get new IDs in quick succession.

**Rationale**: FR-007 and SC-005 require proactive restart of dependents when parent has new ID and is healthy (e.g. after updater replaces parent). Comparing current ID to last-known is the minimal way to detect "parent was replaced"; running only the dependent-restart step (no parent restart) matches the spec. Reusing RestartDependents keeps one code path and ensures cooldown applies.

**Alternatives considered**: Trigger only when parent was previously in unrestartable set (rejected: spec says "when the monitor observes a parent has a new ID and is healthy", not only after a failure). Separate "proactive" cooldown (rejected: clarification said same cooldown as normal recovery).
