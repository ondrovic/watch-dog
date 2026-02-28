# Feature Specification: Correct Handling of Dependent (Child) Containers

**Feature Branch**: `003-fix-children-handling`  
**Created**: 2026-02-28  
**Status**: Draft  
**Input**: User description: "I don't believe it's handling children correctly here are some logs, I tested by stopping the vpn container. … There are dependent children on the qtorrent (dler) that should have restarted 1 at a time."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Parent Recovery Completes All Dependent Restarts (Priority: P1)

When a monitored parent container (e.g. VPN or torrent client) is stopped or becomes unhealthy, the operator expects the monitor to restart the parent, wait until it is healthy, then restart each of its dependents so that the stack returns to a fully running state. No dependent restart should be skipped or fail due to the monitor’s own operations being canceled.

**Why this priority**: This is the core value: after a parent recovers, all dependents are reliably restarted.

**Independent Test**: Stop a parent container (e.g. VPN); confirm the parent is restarted and becomes healthy, then confirm every dependent is restarted and no "context canceled" or equivalent errors cause restarts to be skipped.

**Acceptance Scenarios**:

1. **Given** a parent is monitored and has multiple dependents, **When** the parent is stopped, **Then** the monitor restarts the parent, waits for it to be healthy, then restarts each dependent so that all dependents are running.
2. **Given** a parent has just been restarted and is healthy, **When** the monitor restarts dependents, **Then** each dependent restart is attempted and completes (or fails with a clear, non-canceled error) so that the operator can see which, if any, failed.

---

### User Story 2 - Dependent Restarts Are Sequential (One at a Time) (Priority: P2)

When restarting dependents after a parent recovers, the operator expects dependents to be restarted one at a time (sequentially), not all at once. This avoids overloading the runtime and reduces the chance of operations being canceled or failing due to concurrent restarts and lifecycle changes.

**Why this priority**: User explicitly requested "1 at a time"; sequential behavior is required for predictable recovery.

**Independent Test**: Trigger parent recovery with several dependents; verify from logs or behavior that dependent restarts happen in a defined order, one after another, not in parallel.

**Acceptance Scenarios**:

1. **Given** a parent has N dependents (N ≥ 2), **When** the monitor restarts dependents after the parent is healthy, **Then** restarts are performed one at a time (next dependent starts only after the previous restart attempt has completed or clearly failed).
2. **Given** the same setup, **When** dependent restarts are in progress, **Then** the monitor does not cancel in-flight restart operations for other dependents (e.g. no "context canceled" for restart or discovery due to restarting another dependent or self).

---

### User Story 3 - Monitor Restart Does Not Cancel In-Flight Recovery (Priority: P3)

When the monitor is itself a dependent of a recovered parent, the operator expects the monitor to complete restarting all other dependents (or to hand off cleanly) before any action that would stop the current process. Recovery of the rest of the stack should not be left incomplete because the monitor’s process was stopped mid-recovery.

**Why this priority**: Logs show "restart dependent dependent=watch-dog … context canceled" and subsequent "refresh discovery … context canceled", indicating the monitor’s own restart cancels ongoing work; fixing this prevents half-done recovery.

**Independent Test**: Use a compose where the monitor is a dependent of a parent (e.g. VPN or dler); stop the parent, then verify that after the parent is healthy, other dependents are restarted and only then (if applicable) the monitor’s own restart is considered, without in-flight restarts or discovery being canceled.

**Acceptance Scenarios**:

1. **Given** the monitor is in the dependent list of a parent that just recovered, **When** the monitor performs dependent restarts, **Then** it does not cancel its own in-flight operations (e.g. restart, discovery) so that other dependents are restarted before any step that would terminate the process.
2. **Given** the same setup, **When** the operator inspects logs after parent recovery, **Then** there are no "context canceled" errors for "restart dependent", "refresh discovery", or "docker events" caused by the monitor restarting itself while work is in progress.

---

### Edge Cases

- What happens when a dependent fails to restart (e.g. image missing, config error)? The monitor should log the failure, continue with the next dependent (if any), and not cancel remaining restarts.
- What happens when the same container is a dependent of more than one parent? The system should restart it once per recovery (or define a single canonical order) so it is not restarted multiple times concurrently.
- What happens when the compose file defines no dependents for a parent? Only the parent is restarted; no dependent step runs.
- What happens when discovery or event stream is temporarily unavailable during recovery? For this feature, no new retry or reconnect logic is required; the system completes the **current** restart sequence (the in-flight RestartDependents loop) without canceling it due to discovery/event errors. If discovery fails when building the dependent list, existing behavior (e.g. skip or use cached list) applies; no additional guarantee is specified here.

## Assumptions

- "One at a time" means one dependent restart at a time (sequential), not parallel batches.
- Dependency and order of dependents are defined by the compose file (e.g. root-level depends_on); the monitor uses this to determine which containers are dependents and in what order to restart them.
- The monitor may be listed as a dependent of a parent; in that case, the desired behavior is to complete other dependents’ restarts before any step that would stop the current process (or to skip self-restart and document behavior).
- A "context canceled" in logs indicates that an operation was aborted because the process or request context was canceled (e.g. shutdown or self-restart), not necessarily a user action.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When a parent needs recovery, the system MUST restart the parent and wait until it is healthy before starting any dependent restarts.
- **FR-002**: The system MUST restart dependents of a recovered parent one at a time (sequentially): each dependent’s restart MUST be started only after the previous dependent’s restart attempt has completed or clearly failed.
- **FR-003**: The system MUST NOT cancel in-flight restart or discovery operations for other dependents when performing a dependent restart (including when the monitor is in the dependent list).
- **FR-004**: The system MUST complete or clearly fail every dependent restart attempt before performing any action that would terminate the monitor process (e.g. self-restart); it MUST NOT leave restarts or discovery in a "context canceled" state due to self-restart.
- **FR-005**: When a dependent restart fails (non-canceled error), the system MUST log the failure and MUST continue with the next dependent in order.
- **FR-006**: The system MUST determine dependent containers from the compose file (e.g. root-level depends_on) and use a **deterministic** order (e.g. sort by container name) for sequential restart so that order is reproducible and not literal compose declaration order.
- **FR-007**: The system MUST record or log each dependent restart attempt (start and outcome) so operators can verify that all dependents were processed.

### Key Entities

- **Parent container**: A monitored container whose health (or run state) is watched; when it is stopped or unhealthy, the system restarts it and then its dependents.
- **Dependent (child) container**: A container that depends on a parent (e.g. via depends_on in compose); when the parent is recovered, the system restarts dependents in a defined order.
- **Recovery sequence**: The ordered steps: detect parent need → restart parent → wait healthy → restart dependents one at a time.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After a single parent is stopped or becomes unhealthy, 100% of its dependents (as defined by the compose file) receive a restart attempt once the parent is healthy; no dependent is skipped due to "context canceled" or equivalent.
- **SC-002**: Dependent restarts occur one at a time: for every two dependents A and B, the start of B’s restart occurs only after A’s restart attempt has finished (success or logged failure).
- **SC-003**: When the monitor is a dependent of the recovered parent, zero "context canceled" errors occur for "restart dependent", "refresh discovery", or "docker events" caused by the monitor stopping itself before completing other dependents’ restarts.
- **SC-004**: Operators can reproduce recovery by stopping a parent (e.g. VPN or torrent container) and confirm via logs that all dependents are restarted in order with no canceled operations.
