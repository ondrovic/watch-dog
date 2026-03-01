# Feature Specification: Fix Recovery When Containers Are Gone or Unrestartable

**Feature Branch**: `005-fix-recovery-stale-container`  
**Created**: 2026-03-01  
**Status**: Draft  
**Input**: User description: "Fix recovery when containers no longer exist, are marked for removal, or when a dependent's network container is missing; avoid repeated retries that always fail with the same error."

## Clarifications

### Session 2026-03-01

- Q: When an updater replaces the parent (new parent ID, healthy), should the monitor proactively restart dependents of that parent so dependents that were failing due to missing network can re-bind? → A: Yes — proactive: when the monitor observes a parent has a new ID and is healthy (e.g. after re-discovery), it proactively restarts all dependents of that parent.
- Q: When the monitor proactively restarts dependents (parent has new ID and healthy), should it apply the same dependent restart cooldown as in normal recovery? → A: Yes — apply the same cooldown (e.g. at most one restart per dependent per cooldown window) to proactive restarts.
- Q: Should the spec explicitly name updaters (watchtower, wud, ouroboros) for issue #5 traceability and operator clarity? → A: Yes — name them in Context or Assumptions.

## Context / Observed Cause

This problem often occurs when containers are updated by an external updater (e.g. watchtower, wud, ouroboros): the updater pulls new images, creates new container instances, and stops or removes the old ones. The monitor continues to track the old container identities; when it sees them as stopped or unhealthy it attempts recovery by restarting those IDs. Those IDs refer to containers that no longer exist or are marked for removal, so restart fails. The monitor then retries indefinitely with the same stale ID. In one observed case, Ouroboros updated captcha, dler, and vpn in sequence; watch-dog then failed to restart the old captcha (no such container), old dler and vpn (marked for removal), and dler again (vpn’s network namespace container gone), with repeated identical errors. Documenting this context ensures the fix covers the “container replaced by an updater” scenario.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - No Repeated Failed Recovery Attempts (Priority: P1)

When the monitor attempts to recover a container and the recovery action fails because the container no longer exists, is marked for removal, or is in a state where it cannot be restarted, the operator expects the monitor to stop retrying the same failing action for that container. The monitor must not repeatedly attempt the same failed recovery (e.g. every poll cycle) with no path to success, filling logs and consuming resources.

**Why this priority**: Endless retry loops with identical errors provide no value and make it harder to see real issues; operators need the monitor to recognize unrecoverable states and back off or re-evaluate.

**Independent Test**: Cause a monitored container to be removed (or marked for removal) so that restart fails; run the monitor and verify that it does not log the same recovery failure repeatedly in a tight loop (e.g. many times per minute). After a bounded number of attempts or after detecting the failure type, retries for that container must cease or be significantly reduced.

**Acceptance Scenarios**:

1. **Given** a monitored container has been removed so that restart is impossible, **When** the monitor attempts recovery, **Then** after the failure is detected the monitor does not repeatedly retry the same restart in a tight loop; it either stops retrying for that container or backs off.
2. **Given** a monitored container is marked for removal and cannot be started, **When** the monitor attempts recovery, **Then** the monitor does not repeatedly attempt restart with the same error; it treats the failure as unrecoverable for that container identity and does not spam retries.
3. **Given** the monitor has stopped (or backed off) retrying for a container that could not be restarted, **When** the operator inspects logs, **Then** the number of identical failure messages for that container is bounded, not unbounded over time.

---

### User Story 2 - Recovery Succeeds When Container Can Be Brought Back (Priority: P2)

When a monitored container has been stopped or removed but can still be brought back (e.g. by starting a new instance of the same service), the operator expects the monitor to eventually recover the service so that the monitored workload is running again. If the monitor stops retrying a failed recovery, it must still be possible for the same logical service to be recovered later—for example after the operator or the system has recreated the container, or after the monitor re-discovers the current set of containers.

**Why this priority**: Stopping retry loops (P1) must not prevent legitimate recovery when the situation changes (e.g. container recreated, dependency fixed).

**Independent Test**: (1) Remove or stop a monitored container so that restart fails; confirm the monitor backs off. (2) Recreate the container (or fix the blocking condition). (3) Verify that the monitor can again successfully recover that service (e.g. on a later event or discovery cycle) so the workload is running.

**Acceptance Scenarios**:

1. **Given** the monitor has stopped retrying recovery for a container that no longer existed, **When** a new instance of the same logical service appears (e.g. recreated by the operator or by the runtime), **Then** the monitor can treat it as the monitored service and resume normal monitoring and recovery behavior.
2. **Given** recovery failed because a dependent's network dependency (another container) was missing, **When** that dependency is later available again, **Then** the monitor can successfully recover the dependent (or the operator has a clear path to get the service back without being blocked by endless retries).
3. **Given** an external updater has replaced a parent (new parent container ID, parent is healthy), **When** the monitor re-discovers and observes that parent has a new ID and is healthy, **Then** the monitor proactively restarts all dependents of that parent so dependents that were failing due to the missing (old) parent's network can re-bind to the new parent and the child/dependent comes back online.

---

### User Story 3 - Clear Operator Visibility When Recovery Cannot Succeed (Priority: P3)

When recovery fails in a way that indicates the container cannot be restarted (e.g. container gone, marked for removal, or dependency missing), the operator expects to see a clear indication that recovery was attempted and failed for a known reason, and that retries have been limited or stopped—rather than an endless stream of identical errors.

**Why this priority**: Operators need to understand why a service is not recovering and that the monitor has not given up arbitrarily; visibility supports troubleshooting and manual intervention.

**Independent Test**: Trigger an unrecoverable failure (e.g. container removed); confirm logs indicate recovery was attempted, the failure reason is identifiable, and that repeated identical errors are not the only signal (e.g. a summary or back-off message appears).

**Acceptance Scenarios**:

1. **Given** recovery fails because the container no longer exists (or similar unrecoverable condition), **When** the monitor limits or stops retries, **Then** the operator can see from logs that recovery was attempted, why it failed, and that the monitor is not continuing to retry indefinitely.
2. **Given** recovery fails because a dependency (e.g. network namespace container) is missing, **When** the monitor logs the failure, **Then** the operator can identify the missing dependency and the affected service so they can fix the underlying issue.

---

### User Story 4 - Optional auto-recreate when parent is gone (Priority: P2, optional)

When the monitor marks a **parent** as unrestartable with reason container_gone (no such container) or marked_for_removal, the operator may prefer the monitor to trigger recreation of that service via the compose file (equivalent to `docker compose up -d <service_name>`). When this option is enabled (e.g. WATCHDOG_AUTO_RECREATE), the monitor brings up that parent's **service name** (resolved from the compose file when `container_name` is set); recovery then proceeds when discovery sees the new container ID. Implementation uses the Docker Compose Go SDK (in-process); no docker or docker-compose binary is required.

**Acceptance**: With the option enabled, remove a monitored parent (e.g. `docker rm -f <container_name>`); verify one failure log (container_gone or marked_for_removal), then a log that auto-recreate was triggered, then on next discovery the new container is seen and recovery runs for the new ID (or the new container is already healthy).

---

### Edge Cases

- What happens when an external updater (e.g. watchtower, wud, ouroboros) replaces several monitored containers in sequence? Each replaced container may be seen as stopped or unhealthy; the monitor must not retry restart for the old (stale) IDs indefinitely. After the updater has created new instances, the monitor should be able to pick up and monitor those new instances so the service is considered recovered.
- What happens when multiple monitored containers fail recovery for different reasons (e.g. one gone, one marked for removal)? Each is handled so that no single container causes unbounded retries, and the monitor continues to monitor and recover others where possible.
- What happens when a dependent's recovery fails because its parent (or network peer) container is missing? The monitor must not enter an endless retry loop for the dependent; it may record the failure and optionally retry after the parent is available again, but repeated identical failures must be bounded.
- What happens when the container is recreated with a new identity (e.g. new ID) after the monitor has backed off? The monitor must be able to discover and monitor the new instance so that recovery behavior is restored for that service.
- What happens when the runtime reports "container is marked for removal"? The monitor treats this as an unrestartable state for that container identity and does not repeatedly attempt restart.
- What happens when only the parent is replaced by an updater (new parent ID, parent healthy) and dependents are still the old containers that had been failing with "dependency missing"? The monitor, when it observes a parent has a new ID and is healthy (e.g. after re-discovery), proactively restarts all dependents of that parent so they can re-bind to the new parent and the dependent (child) comes back online without requiring the dependent to be recreated.

## Assumptions

- "Container no longer exists," "marked for removal," and "cannot be started" (or equivalent runtime responses) are treated as unrecoverable for that specific container identity; the monitor may re-discover containers by service name or composition so that the same logical service can be monitored again after recreation.
- A common trigger is an external container updater (e.g. watchtower, wud, ouroboros) that replaces containers (new image → new container → old container stopped/removed); the monitor may run alongside such an updater and must handle stale identities without endless retries. This feature addresses the scenario described in GitHub issue #5 (child/dependent container when parent is auto-updated).
- Dependency order (e.g. parent before dependent, or network container before container that joins it) may require the monitor to recover or re-discover in order; the fix does not require implementing full orchestration, but must avoid endless retries when a dependency is missing.
- Operators may recreate containers manually or via composition or via an updater; the monitor's role is to observe, attempt recovery, and avoid retry loops. Optionally (when configured), the monitor MAY trigger recreation of a parent service via compose when that parent is marked container_gone or marked_for_removal so the operator does not have to run compose by hand.
- Logging and back-off behavior must be observable so that operators can confirm retries are bounded and understand failure reasons.
- The poll interval (referred to in success criteria) is implementation-defined (e.g. from code or environment); the spec does not mandate a specific value.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When a recovery attempt fails because the container no longer exists (or equivalent "not found" condition), the system MUST NOT repeatedly retry the same restart for that container identity in an unbounded way; it MUST limit retries or stop retrying and MAY re-discover containers so the same service can be monitored again.
- **FR-002**: When a recovery attempt fails because the container is marked for removal (or equivalent "cannot be started" condition), the system MUST NOT repeatedly retry the same restart for that container identity; it MUST treat the state as unrestartable for that identity and MUST NOT spam retries.
- **FR-003**: When a recovery attempt fails because a required dependency (e.g. another container's network namespace) is missing, the system MUST NOT repeatedly retry the same failing recovery in an unbounded way; it MUST limit retries or back off so that the same error is not logged in a tight loop.
- **FR-004**: The system MUST allow the same logical service to be recovered later (e.g. after containers are re-discovered or recreated), so that stopping retries for one container identity does not permanently prevent recovery of that service. Measurable outcome: SC-003.
- **FR-005**: The system MUST provide operators with identifiable failure reasons when recovery fails (e.g. container gone, marked for removal, dependency missing), so that logs support troubleshooting and manual intervention.
- **FR-006**: When the system limits or stops retries for a failed recovery, it MUST continue to monitor other containers and MUST still attempt recovery for them when appropriate; one unrestartable container must not block monitoring of the rest.
- **FR-007**: When the monitor observes that a parent has a new container ID and is healthy (e.g. after re-discovery, such as when an updater replaced the parent), the system MUST proactively restart all dependents of that parent so dependents that were failing due to the missing (old) parent's network can re-bind to the new parent and the child/dependent comes back online. The same dependent restart cooldown as in normal recovery (e.g. at most one restart per dependent per cooldown window) MUST apply to these proactive restarts.
- **FR-008** (optional): When configured (e.g. via WATCHDOG_AUTO_RECREATE), when a **parent** is marked unrestartable with reason container_gone or marked_for_removal, the system MAY trigger recreation of that service via the compose file (equivalent to `docker compose up -d <service_name>`) so the operator does not have to run compose by hand; recovery then proceeds when discovery sees the new container ID. The monitor resolves container name to service name from the compose file when `container_name` is set. Implementation uses the Docker Compose Go SDK; no docker or docker-compose binary is required. This applies only to parent container_gone or marked_for_removal, not to dependents or to dependency_missing.

### Key Entities

- **Monitored container (identity)**: A specific container instance the monitor is trying to recover; when that instance is gone or unrestartable, recovery for that identity fails and retries must be bounded.
- **Logical service**: The workload identified by name or composition (e.g. service "vpn", "dler"); may have a new container identity after recreation, and the monitor should be able to track and recover it again after re-discovery.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For any single container that cannot be restarted (gone, marked for removal, or dependency missing), the number of identical recovery-failure log lines for that container over a 10-minute window is bounded—concretely, at most one failure log plus skip logs per container per 10 minutes, not a growing stream.
- **SC-002**: Operators can distinguish "recovery attempted and failed (retries limited)" from "recovery in progress" using logs or observable behavior, so they know when to intervene manually.
- **SC-003** (satisfies FR-004): After the monitor has stopped retrying a failed recovery, if the same logical service is recreated or becomes available again, the monitor can resume normal monitoring and recovery for that service within one poll interval or on the next Docker event, whichever comes first.
- **SC-004**: When one or more containers are unrestartable, the monitor continues to observe and recover other monitored containers; overall recovery success rate for other containers is not degraded by the presence of unrestartable ones.
- **SC-005**: When a parent is replaced by an updater (new parent ID, parent healthy) and dependents had been failing due to the old parent's network being missing, the monitor proactively restarts those dependents within one poll interval or on the next discovery after observing the new parent, so the dependent (child) comes back online without requiring the dependent to be recreated.
