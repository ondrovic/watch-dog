# Quickstart: Fix Recovery When Containers Are Gone or Unrestartable (005)

**Branch**: `005-fix-recovery-stale-container` | **Date**: 2026-03-01

This feature stops the monitor from retrying the same failed recovery indefinitely when the container no longer exists, is marked for removal, or a dependency (e.g. network namespace) is missing. It ensures bounded retries, allows recovery to resume when the same service is recreated with a new container ID, and **proactively restarts dependents** when only the parent is replaced by an updater (e.g. watchtower, wud, ouroboros) so the child/dependent comes back online—see [GitHub issue #5](https://github.com/ondrovic/watch-dog/issues/5).

## Prerequisites

- Same as [001 quickstart](../001-container-health-monitor/quickstart.md): Docker, healthchecks, compose path set.
- Optional: an external container updater (e.g. watchtower, wud, ouroboros) that replaces containers; useful to verify the "updater replaces container" and "parent replaced → child comes back" (issue #5) scenarios.

## What changes for operators

- **Config**: Optional env `WATCHDOG_AUTO_RECREATE` (`true`/`1`/`yes`): when a parent is marked unrestartable with reason **container_gone** or **marked_for_removal**, the monitor can run `docker compose up -d <service_name>` (resolving container name to service name from the compose file when `container_name` is set) so the service comes back without manual intervention; the monitor re-discovers on the next cycle. Requires a compose path. If unset or disabled, behavior is as before (no auto-recreate).
- **Logs**: When a restart fails because the container is gone, marked for removal, or a dependency is missing, you will see a clear error that recovery failed and that the monitor will not retry this container ID. When the monitor skips recovery because the container is already known unrestartable, you will see a skip message (INFO or DEBUG) so you know retries are limited.
- **Recovery after replace**: When the same service is recreated (e.g. new container ID after an updater run), the monitor will pick it up on the next discovery and run recovery for it as normal.
- **Proactive dependent restart**: When only the **parent** is replaced by an updater (new parent ID, parent healthy), the monitor will proactively restart that parent's dependents within one poll interval or next discovery so the dependent (child) comes back online without needing to recreate the dependent (SC-005).

## Verify bounded retries (no endless loop)

1. **Setup**: Run the monitor with at least one parent (e.g. VPN). Ensure the parent is monitored (discovery and events or polling active).

2. **Trigger**: Remove the parent container so it no longer exists (e.g. `docker rm -f <parent>`), or put it in a state where restart fails (e.g. container marked for removal). Alternatively, use an updater that replaces the parent so the old container is removed or marked for removal.

3. **Expected**:
   - The monitor attempts recovery (restart) at most once (or a small bounded number of times) for that container **ID**.
   - After the failure is detected and the ID is marked unrestartable, you see a log that recovery failed and will not be retried for this container ID.
   - On subsequent events or polling, you do **not** see the same error repeated in a tight loop (e.g. many times per minute). Log volume for that container is bounded: at most one failure log plus skip messages per container per 10 minutes (see SC-001).

## Verify recovery resumes when service is recreated

1. **Setup**: After you have triggered the scenario above (container gone or unrestartable), recreate the **same logical service** (same compose service name) so a **new** container with a new ID exists (e.g. `docker compose up -d` for that service, or let your updater create a new container).

2. **Expected**:
   - Within one poll interval (e.g. 60s) or on the next Docker event, the monitor sees the new container ID for that service name (see SC-003).
   - If that parent is unhealthy or stopped, the monitor runs the full recovery sequence for the **new** ID (restart parent → wait healthy → restart dependents). No need to restart the monitor; re-discovery picks up the new ID.

## Verify other containers still monitored

1. **Setup**: Multiple parents (e.g. vpn, dler, captcha). Cause one parent’s container to be unrestartable (e.g. remove it or mark for removal).

2. **Expected**:
   - The monitor stops retrying the unrestartable container ID and logs accordingly.
   - Other parents are still monitored; if another parent becomes unhealthy, the monitor runs recovery for that parent as before. One unrestartable container does not block recovery for others.

## Verify proactive restart when only parent is replaced (child comes back)

1. **Setup**: Compose with a parent (e.g. vpn) and at least one dependent (e.g. dler) that uses the parent's network. Run the monitor and an updater (e.g. watchtower, wud, ouroboros).
2. **Trigger**: Let the updater replace **only the parent** (vpn gets new image and new container ID; dependent dler is not updated and may have been failing with "joining network namespace ... No such container" for the old vpn).
3. **Expected**: Within one poll interval or on the next discovery after the new parent is healthy, the monitor detects that the parent has a new ID and is healthy, then **proactively restarts the dependents** of that parent. The dependent (child) comes back online without requiring the dependent to be recreated (SC-005). Logs indicate proactive restart (e.g. parent has new ID, restarting dependents).

## Optional: auto-recreate when parent is gone (WATCHDOG_AUTO_RECREATE)

1. **Setup**: Set `WATCHDOG_AUTO_RECREATE=true` (or `1`/`yes`) and ensure `WATCHDOG_COMPOSE_PATH` (or `COMPOSE_FILE`) is set. Run the monitor with at least one parent.
2. **Trigger**: Remove the parent container (e.g. `docker rm -f <container_name>`). Trigger recovery (event or poll). The daemon may report container_gone or marked_for_removal; both trigger auto-recreate.
3. **Expected**: One failure log (container_gone or marked_for_removal), then an INFO log that auto-recreate was triggered (parent name, service name if resolved, compose path, re-discover on next cycle). On the next discovery the new container is seen and recovery runs for the new ID (or the new container is already healthy).

## Optional: run with an updater (e.g. watchtower, wud, ouroboros)

If you use an updater to update containers:

1. Let the updater replace one or more monitored parents (e.g. vpn, dler).
2. **Expected**: You may see one or a few recovery attempts for the **old** container IDs that fail (no such container or marked for removal). After that, the monitor does not spam the same error. When the updater has created new containers, the monitor’s next discovery uses the new IDs; if only the parent was replaced, the monitor proactively restarts dependents so the child comes back. Recovery works for the new instances as needed.
