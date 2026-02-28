# Research: Correct Handling of Dependent (Child) Containers (003-fix-children-handling)

**Branch**: `003-fix-children-handling` | **Date**: 2026-02-28

## 1. Sequential dependent restart order

**Decision**: Restart dependents in a **deterministic, stable order** (e.g. sort by container name). Implement **one restart at a time**: start the next dependent’s restart only after the previous restart call has returned (success or error). The current code already does one call per dependent in a loop; the fix is to **guarantee order** (so logs and behavior are reproducible) and to **defer self-restart to last**.

**Rationale**: Spec FR-002 and SC-002 require “one at a time” and that B starts only after A’s attempt has finished. Go map iteration in `BuildServiceParentToDependents` is non-deterministic, so the dependent list order is currently undefined. Sorting by name gives a stable, reproducible order. Keeping the existing sequential loop (no goroutines for restarts) ensures one-at-a-time and avoids context cancellation from concurrent work.

**Alternatives considered**:
- Preserve compose file order: would require parsing order from YAML (e.g. ordered map or document order). Possible but more complex; sorting by name is simpler and sufficient.
- Parallel restarts: would contradict “one at a time” and increase risk of overload and context cancellation; rejected.

---

## 2. Self-restart: avoid canceling in-flight work

**Decision**: When the monitor’s **own container name** appears in the dependent list for the parent just recovered, **restart all other dependents first**, then restart the monitor’s container **last**. Do not restart self in the middle of the list. This way, when the process is terminated by Docker (restart of the watch-dog container), all other dependent restarts have already completed and no in-flight restart or discovery call is canceled.

**Rationale**: Logs show “restart dependent dependent=watch-dog … context canceled” and “refresh discovery … context canceled” because restarting the watch-dog container stops the process and cancels the request context. By restarting self last, we ensure no other dependent’s restart or discovery refresh is in progress when the process exits. No separate “skip self” policy is required: “self last” satisfies the spec (complete all others, then optionally self).

**Alternatives considered**:
- Skip self entirely: would leave the monitor’s container not restarted when the stack is recovered; operators might expect consistency (all dependents restarted). Self last is preferable so the monitor is still restarted but after others.
- Separate context for “restart others” that is not canceled when self restarts: possible in theory but the process still exits when the container restarts, so any in-flight call will see connection/context failure; reordering is simpler and sufficient.

---

## 3. Identifying the monitor’s container name (“self”)

**Decision**: Use an **optional environment variable** (e.g. `WATCHDOG_CONTAINER_NAME` or `HOSTNAME`) to supply the current container’s name. If set, the recovery flow uses it to reorder dependents so this name is last. If unset, all dependents are restarted in the same deterministic order (no “self last” behavior). In Docker Compose, the service name or container name can be passed via `environment: CONTAINER_NAME: watch-dog` or by using `container_name: watch-dog` and passing it explicitly (e.g. `environment: WATCHDOG_CONTAINER_NAME: watch-dog`).

**Rationale**: The process does not have a built-in way to know its container name from the Docker API without listing/inspecting all containers and matching by PID or similar, which is heavier and daemon-specific. Environment variable is simple, explicit, and works in all runtimes. Using `HOSTNAME` is a possibility when the container hostname equals the container name (often true when `container_name` is set), but a dedicated env (e.g. `WATCHDOG_CONTAINER_NAME`) is clearer and avoids relying on hostname semantics.

**Alternatives considered**:
- Infer from Docker API (e.g. find container where PID matches): possible but more complex and not always reliable; env is simpler.
- Always use `HOSTNAME`: acceptable as a fallback when it matches container name; can be documented as optional fallback if we add it later.

---

## 4. Deterministic order in discovery

**Decision**: When building the list of dependents for a parent (in discovery or in recovery), **sort the list of dependent names** before use so that (1) order is stable across runs and (2) after “self last” reordering, the final order is still deterministic. Apply sorting in the recovery layer when preparing the list for RestartDependents (or in discovery when returning GetDependents), so all consumers see the same order.

**Rationale**: Spec FR-006 and SC-002 require a defined order for sequential restart. BuildServiceParentToDependents currently appends in map iteration order (random in Go). Sorting by name is the minimal change to get a defined order; “self last” is then applied on top of that sorted list.

**Alternatives considered**:
- Sort in BuildServiceParentToDependents / BuildParentToDependentsFromCompose: keeps order in one place but discovery would need to know “self” to put it last; “self last” is recovery semantics, so sorting in discovery and reordering in recovery is cleaner.
- No sorting: would leave order undefined and make logs and tests flaky; rejected.
