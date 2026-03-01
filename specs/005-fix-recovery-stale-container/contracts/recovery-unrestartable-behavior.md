# Contract: Recovery When Restart Fails (Unrestartable)

**Feature**: 005-fix-recovery-stale-container | **Date**: 2026-03-01  
**Consumer**: Operators and integration tests; extends recovery behavior so the monitor does not retry the same failed container ID indefinitely.

**Relationship**: This contract extends [001 recovery-behavior](../../001-container-health-monitor/contracts/recovery-behavior.md). When a restart (or inspect during wait-for-healthy) fails with an **unrestartable** error, the behavior below applies in addition to the normal sequence.

---

## Unrestartable errors

The monitor treats the following Docker API error conditions as **unrestartable** for the container ID that was used in the call:

1. **No such container**: The error message indicates the container does not exist (e.g. "No such container").
2. **Marked for removal**: The error message indicates the container cannot be started because it is marked for removal (e.g. "container is marked for removal and cannot be started").
3. **Dependency missing**: The error message indicates that a required dependency (e.g. another container’s network namespace) is missing (e.g. "joining network namespace of container: No such container: ...").

Classification is done by inspecting the error message returned from `ContainerRestart` or `ContainerInspect`. Only these cases are treated as unrestartable; other errors (e.g. daemon busy, timeout) are not and may be retried.

---

## Behavior when restart fails with an unrestartable error

- **Parent restart failed**: If `Restart(parentID)` fails with an unrestartable error, the monitor records that container ID as unrestartable. It does **not** wait for healthy or restart dependents for this run. It does **not** retry the same parent ID on subsequent triggers (event or polling) until that ID is no longer used (e.g. re-discovery yields a new ID for the same service name).
- **Inspect during wait-for-healthy failed**: If `Inspect(parentID)` during wait-for-healthy fails with an unrestartable error, the monitor records that parent ID as unrestartable, does not restart dependents for this run, and does not retry that parent ID as above.
- **Dependent restart failed**: If `Restart(dependentID)` fails with an unrestartable error, the monitor records that **dependent** container ID as unrestartable. It does not retry restarting that dependent ID on subsequent recovery runs; other dependents are still restarted. When the same logical service (name) appears with a new container ID (e.g. after an updater replaces it), that new ID is not in the unrestartable set, so the monitor may restart it as usual when its parent is recovered.

---

## Skip when ID is already unrestartable

- Before running the full recovery sequence for a parent, if the parent’s container ID is already in the unrestartable set, the monitor **skips** the sequence: it does not call Restart, does not wait for healthy, and does not restart dependents for that parent on this trigger. It logs that recovery is skipped for that parent because the container is unrestartable (so operators see that retries are limited).
- Before restarting a dependent, if that dependent’s container ID is in the unrestartable set, the monitor **skips** restarting that dependent for this run and logs accordingly. Other dependents are still restarted.

---

## Re-discovery and new instances

- The monitor does **not** persist the unrestartable set; it is in-memory only. Discovery (event and polling) already refreshes the container list and parent→dependents. When a container is recreated (e.g. by an external updater), it has a **new** container ID. The monitor will receive that new ID on the next discovery (e.g. from the container list). That new ID is not in the unrestartable set, so the monitor will run recovery for it as normal. No explicit "clear by name" is required.
- The unrestartable set is bounded (e.g. max size and/or pruning of IDs that no longer appear in the container list) so it does not grow indefinitely.

---

## Observability

- **First failure**: When a restart or inspect fails with an unrestartable error, the monitor logs at ERROR (or INFO) that recovery failed for that container, the reason (e.g. container gone, marked for removal, dependency missing), and that it will not retry this container ID.
- **Skip**: When recovery is skipped because the container ID is already unrestartable, the monitor logs at INFO or DEBUG so operators can see that retries are limited and that the monitor will retry when a new instance (new ID) appears.
- Structured attributes (parent/dependent name, container ID short, reason) are used so logs are queryable.

---

## Proactive dependent restart when parent has new ID (FR-007)

When an external updater (e.g. watchtower, wud, ouroboros) replaces only the **parent** (new parent container ID, parent is healthy), dependents may still be the old containers that had been failing with "dependency missing" (e.g. joining network namespace of old parent). The monitor **proactively restarts all dependents** of that parent so they can re-bind to the new parent and the child/dependent comes back online (addresses [GitHub issue #5](https://github.com/ondrovic/watch-dog/issues/5)).

- **Trigger**: When the monitor observes that a parent has a **new** container ID and is healthy (e.g. after re-discovery). Detection: compare current container ID for that parent name (from the container list) to the last-known ID stored for that parent; if different and the parent is healthy, trigger proactive restart.
- **Action**: Restart **dependents only** (do not restart the parent). Use the same order and logic as normal recovery (RestartDependents); the same **dependent restart cooldown** (e.g. WATCHDOG_DEPENDENT_RESTART_COOLDOWN) applies, so at most one restart per dependent per cooldown window.
- **Update state**: After triggering proactive restart for a parent, update the last-known ID for that parent to the current ID so we do not repeatedly trigger on every discovery.
- **Observability**: Log that proactive restart is running for parent X (parent has new ID, restarting dependents) so operators can verify the behavior (SC-005).

---

## Optional auto-recreate when parent is container_gone or marked_for_removal (FR-008)

When a **parent** is marked unrestartable with reason **container_gone** (no such container) or **marked_for_removal**, the monitor may optionally trigger recreation of that service so the operator does not have to run compose by hand.

- **Enable**: Set `WATCHDOG_AUTO_RECREATE` to `true`, `1`, or `yes` (case-insensitive). When enabled and a compose path is available (`WATCHDOG_COMPOSE_PATH` or `COMPOSE_FILE`), the monitor sets an internal callback that runs when a parent is added to the unrestartable set with reason **container_gone** or **marked_for_removal**.
- **Action**: The callback runs `docker compose -f <composePath> up -d <service_name>` with the working directory set to the compose file’s directory (or current directory when appropriate). If that fails because the Compose V2 plugin is not available (e.g. "unknown shorthand flag" or "compose is not a docker command"), the monitor may retry with `docker-compose -f <composePath> up -d <service_name>` when the standalone binary is in PATH, so auto-recreate works in both Compose V1 and V2 environments. The monitor resolves the parent's **container name** (e.g. `vpn`) to the compose **service name** (e.g. `gluetun`) using the compose file's `container_name` when set, so the correct service is recreated. The monitor does not wait for the command to finish; it continues and will re-discover on the next cycle.
- **Scope**: The callback is invoked **only** when the failed container is the **parent** and the reason is **container_gone** or **marked_for_removal**. It is **not** invoked for dependents or for **dependency_missing**.
- **Observability**: When auto-recreate is triggered, the monitor logs at INFO the parent name, service name (if resolved), compose path, and that it will re-discover on the next cycle. On success it logs whether `docker compose` or `docker-compose` was used. If the compose up command fails, an error is logged (and, if both variants were tried, that both were attempted).
