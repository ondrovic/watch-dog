# Feature Specification: No Dependent Restarts on Initial Stack Start

**Feature Branch**: `004-child-deps-initial-restart`  
**Created**: 2026-02-28  
**Status**: Draft  
**Input**: User description: "fix child dependencies restarts on initial stack start"

## Clarifications

### Session 2026-02-28

- Q: Should the spec explicitly require that when the stack is brought up with `docker compose up` (monitor and parents/dependents start together), the monitor must not trigger any dependent restarts? → A: Yes — call out explicitly: add "docker compose up" as a primary scenario; require that when the stack is brought up with docker compose up, the monitor MUST NOT trigger any dependent restarts.
- Q: Should the spec state that the intended deployment may include the monitor as a dependent of a parent in the compose file (e.g. depends_on), and the fix must allow that without cascade? → A: Yes: the spec should state that the intended deployment may include the monitor as a dependent in compose, and the fix MUST allow that without cascade; operators should be able to use depends_on for the monitor.
- Q: Should the spec add an explicit "no restart loop" requirement? → A: Explicit: add a requirement that the system MUST NOT enter a cascade restart failure; make it testable (e.g. compose up does not lead to sustained or repeated restart loops).
- Q: When the monitor starts and sees a parent still starting (not yet healthy) at first discovery, treat as initial or as recovery? → A: Initial: during initial discovery, if a parent is not yet healthy, do NOT trigger recovery or dependent restarts; only observe. Recovery only after initial discovery is complete and the monitor later sees the parent unhealthy.
- Q: Should the spec add a testable definition of "initial discovery"? → A: B with additional wait time: add a testable definition (e.g. first full discovery cycle or until all monitored parents observed at least once) plus an additional wait time before initial discovery is considered complete.
- Q: How should the initial discovery wait time be configured? → A: Configurable via an environment variable; stacks vary (e.g. 120 seconds to 5 minutes or more), so operators must be able to set the wait time per deployment.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - No Dependent Restarts When Stack Is Brought Up or Already Healthy (Priority: P1)

When the operator brings the stack up with **docker compose up** (monitor and parents/dependents start together) or starts the monitor while the stack is already running (e.g. after a host reboot), the operator expects the monitor to discover containers and their health without triggering any dependent restarts. Parents that are already running and healthy—including when they became healthy as part of the same compose up—must not cause their dependents to be restarted.

**Why this priority**: Avoiding unnecessary restarts on initial start is the core fix; it prevents disruption and matches operator expectation that "start the monitor" does not mean "restart my stack."

**Independent Test**: (1) Run **docker compose up** so the full stack (including the monitor) starts together; confirm no dependent containers are restarted and no cascade occurs. (2) Alternatively: start the stack, wait until all are healthy, then start the monitor; confirm no dependent restarts.

**Acceptance Scenarios**:

1. **Given** the stack is brought up with **docker compose up** (monitor and parents/dependents start together), **When** all containers have started and the monitor completes initial discovery, **Then** no dependent container is restarted; only observation (and any periodic checks) occurs, and no cascade restart failure occurs.
2. **Given** the stack is already running and the parent and all dependents are healthy, **When** the monitor starts and completes initial discovery, **Then** no dependent container is restarted.
3. **Given** either scenario above, **When** the operator inspects logs or container start times after the monitor has been running, **Then** there is no evidence that dependents were restarted at or shortly after monitor startup.

---

### User Story 2 - Dependent Restarts Only After Parent Recovery (Priority: P2)

When the monitor has previously started and is observing the stack, the operator expects dependent restarts to occur only when the monitor has just recovered a parent (e.g. restarted it after it was stopped or unhealthy). If the parent was already healthy at monitor startup, dependents must not be restarted at startup; they are restarted only in response to a later parent recovery.

**Why this priority**: This distinguishes "initial healthy state" from "parent just recovered," which is required for correct behavior on both initial start and ongoing operation.

**Independent Test**: Start monitor with stack healthy (no restarts). Then stop the parent; verify the monitor restarts the parent and then restarts dependents. Compare with initial start where no restarts occur.

**Acceptance Scenarios**:

1. **Given** the monitor is running and the parent becomes stopped or unhealthy, **When** the monitor restarts the parent and it becomes healthy, **Then** the monitor restarts dependents (per existing behavior).
2. **Given** the monitor has just started and the parent is already healthy, **When** the monitor completes discovery, **Then** the monitor does not initiate dependent restarts.

---

### Edge Cases

- What happens when the monitor starts while the parent is already unhealthy or stopped? During **initial discovery**, the system does NOT trigger recovery or dependent restarts (see FR-004); it only observes. After initial discovery is complete, if the parent is then (or later) unhealthy or stopped, the monitor treats that as a recovery scenario: restart the parent, wait for healthy, then restart dependents.
- What happens when a parent is still starting (not yet healthy) when the monitor performs its first discovery? The system treats this as initial discovery: do NOT restart the parent or dependents; only observe. This avoids restarts during the normal compose-up window.
- What happens when the monitor restarts (e.g. itself is a dependent and gets restarted)? After it comes back up, it should not trigger dependent restarts for parents that are already healthy; only actual parent recovery events should trigger dependent restarts.
- What happens when discovery completes at different times for different parents? The rule remains: do not restart dependents for a parent that was already healthy when first observed; only restart dependents after the monitor has performed a recovery (restart) of that parent.
- What happens when the monitor is a dependent of a parent (e.g. `depends_on` in compose) and the stack is brought up with **docker compose up**? The monitor starts after (or with) the parent; it MUST NOT trigger dependent restarts for that parent on initial discovery, so that the monitor’s own restart or other dependents’ restarts do not cause a cascade.
- How does the system avoid a cascade restart failure? By not triggering dependent restarts on initial discovery (so no restarts → no re-observation → no further restarts); the explicit requirement (FR-007) is that the system must not enter sustained or repeated restart loops.

## Assumptions

- "Initial stack start" includes both: (1) the stack is brought up with **docker compose up** (monitor and parents/dependents start together), and (2) the monitor process starts later while the stack is already running. **Initial discovery** is defined as the first full discovery cycle (or until all monitored parents have been observed at least once) plus a configurable additional wait time (e.g. via environment variable); during this phase, parents may be healthy, still starting (not yet healthy), or stopped, and the system does not trigger recovery or dependent restarts—only after initial discovery is complete does an unhealthy or stopped parent trigger recovery. Stack readiness varies (e.g. 120 seconds to 5 minutes or more), so the wait time must be operator-configurable.
- A "recovery" is an event where the monitor restarts a parent because it was stopped or unhealthy; only after such a recovery should dependents be restarted.
- Existing behavior for "restart parent then restart dependents one at a time" (from prior feature) remains unchanged when a recovery actually occurs.
- The monitor can distinguish "parent was just restarted by me" from "parent was already running when I started."
- The intended deployment may include the monitor as a **dependent of a parent** in the compose file (e.g. `depends_on` on the parent). The fix MUST allow this without causing a cascade; operators must be able to use `depends_on` for the monitor and run **docker compose up** without entering a restart loop.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When the stack is brought up with **docker compose up** (monitor and parents/dependents start together), the system MUST NOT restart any dependent container as a result of initial discovery; only observation and periodic checks may occur.
- **FR-001b**: On any initial startup (including after compose up), the system MUST NOT restart any dependent container solely because a parent is already running and healthy when discovery completes. (Refines FR-001 for the general "monitor starts with stack already up" case.)
- **FR-002**: The system MUST restart dependents of a parent only when the monitor has just completed recovery of that parent (i.e. the monitor restarted the parent after it was stopped or unhealthy).
- **FR-003**: When the monitor starts and discovers a parent that is already healthy, the system MUST treat that as observation-only and MUST NOT trigger the "restart dependents" sequence for that parent.
- **FR-004**: During **initial discovery** (first full discovery cycle or equivalent after monitor start), the system MUST NOT trigger recovery or dependent restarts even if a parent is not yet healthy (e.g. still starting); the system MUST only observe. Only after initial discovery is complete MAY the system treat a parent that is then stopped or unhealthy as a recovery scenario (restart parent, then dependents).
- **FR-004b**: When the monitor has completed initial discovery and subsequently discovers a parent that is stopped or unhealthy, the system MUST treat that as a recovery scenario and MUST restart the parent and then its dependents (consistent with existing recovery behavior).
- **FR-005**: The system MUST allow operators to confirm that no dependent restarts occurred at monitor startup when the stack was already healthy (e.g. via logs or observable container start times such as `docker inspect` restart count or container start timestamp).
- **FR-006**: The system MUST support the monitor being declared as a dependent of a parent in the compose file (e.g. `depends_on`); when the stack is brought up with **docker compose up** in that configuration, the system MUST NOT cause a cascade restart failure, so operators can use `depends_on` for the monitor without removing it. (FR-006 is the compose-up + monitor-as-dependent case of the general no-cascade requirement in FR-007.)
- **FR-007**: The system MUST NOT enter a **cascade restart failure**: restarts MUST NOT repeatedly trigger further restarts in a sustained or unbounded loop. In particular, bringing the stack up with **docker compose up** (with or without the monitor as a dependent) MUST NOT lead to sustained or repeated restart loops; the stack must reach a stable state after startup.
- **FR-008**: The system MUST treat **initial discovery** as a well-defined phase: it consists of (1) the first full discovery cycle after monitor start (or until all monitored parents have been observed at least once), and (2) an **additional wait time** after that, before initial discovery is considered complete. Until that phase has elapsed, the system MUST NOT trigger recovery or dependent restarts (see FR-004). The wait time allows containers that are still starting (e.g. during compose up) to reach a healthy state without triggering recovery.
- **FR-009**: The initial discovery wait time MUST be **configurable by the operator** (e.g. via an environment variable), because stack readiness varies (e.g. one stack may need 120 seconds, another 5 minutes or more). Operators must be able to set the wait duration per deployment without code changes.

### Key Entities

- **Parent container**: A monitored container; when it is recovered (restarted by the monitor), its dependents are restarted afterward.
- **Dependent (child) container**: A container that depends on a parent; restarts only after the monitor has recovered that parent, not on initial discovery.
- **Initial discovery**: A well-defined phase after monitor start during which no recovery or dependent restarts are triggered. It consists of: (1) the first full discovery cycle (or until all monitored parents have been observed at least once), and (2) an **additional wait time** after that (configurable by the operator, e.g. via an environment variable). Only when both are complete is "initial discovery" considered finished; the wait time gives containers that are still starting (e.g. during compose up) time to become healthy. Stacks vary (e.g. 120 seconds to 5 minutes or more), so the wait time must be configurable per deployment.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: When the stack is brought up with **docker compose up** or when the monitor starts with the stack already running and healthy, zero dependent containers are restarted as a result of startup or initial discovery; no cascade restart failure occurs.
- **SC-002**: Dependent restarts occur only after a parent recovery event (monitor restarted the parent); operators can reproduce this by starting the monitor with stack healthy (no restarts) and then stopping the parent (restarts occur).
- **SC-003**: Operators can start the monitor against an already-healthy stack and verify via logs or container lifecycle that no unnecessary dependent restarts occurred at startup.
- **SC-004**: Operators can declare the monitor as a dependent of a parent in the compose file (e.g. `depends_on`) and run **docker compose up** without cascade restart failure; no need to remove `depends_on` from the monitor to achieve a stable stack.
- **SC-005**: After **docker compose up** (or monitor startup with stack already up), the stack reaches a stable state: no sustained or repeated restart loop occurs; this can be verified by observing that container restarts do not repeatedly re-trigger (e.g. restarts settle within a bounded number of cycles or do not occur at all on initial discovery).
- **SC-006**: The initial discovery phase (first discovery cycle plus configurable wait time) is defined in a testable way; the wait time is configurable by the operator (e.g. via an environment variable) so that different stacks (e.g. 120 seconds vs 5 minutes readiness) can be accommodated without code changes, and operators or tests can verify that no recovery or dependent restarts occur until that phase has completed.
