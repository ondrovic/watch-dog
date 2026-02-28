# Feature Specification: Container Health Monitor (Watch-Dog)

**Feature Branch**: `001-container-health-monitor`  
**Created**: 2025-02-28  
**Status**: Draft  
**Input**: User description: "Standalone Docker container to monitor health of parent/child containers, replace autoheal; depends_on labels only; implemented in Go; push to Docker Hub; 100% dynamic; multiple children per parent; restart parent when unhealthy then wait for health then restart dependents; prevent child unusable state like swarm without swarm."

## Clarifications

### Session 2025-02-28

- Q: Where should the container image be built and hosted? → A: GitHub Container Registry (GHCR); image is built and published via GitHub (e.g. GitHub Actions on repo push), so the image is hosted directly from the same GitHub repository.
- Note: The original input quoted "push to Docker Hub"; the decision above uses GitHub Container Registry (GHCR) instead.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Monitor and Restart Unhealthy Parent (Priority: P1)

As a user running a stack of containers where some depend on others (e.g., VPN as parent, torrent/client as child), I want a single monitoring container to watch parent health and restart the parent when it becomes unhealthy, so that I can replace autoheal and get reliable recovery without manual intervention.

**Why this priority**: Core value is detecting parent unhealthiness and triggering restart; without this, dependent containers remain in an unusable state.

**Independent Test**: Deploy the monitor with one parent container that has a healthcheck; simulate unhealthy (e.g., kill healthcheck or force unhealthy state); verify the monitor restarts the parent. Delivers immediate value as a drop-in replacement for autoheal for single-parent setups.

**Acceptance Scenarios**:

1. **Given** a parent container with a healthcheck is running, **When** the parent becomes unhealthy, **Then** the monitor restarts the parent container.
2. **Given** the monitor is running and connected to the same Docker host, **When** no containers are unhealthy, **Then** the monitor takes no restart action.
3. **Given** a parent container is restarted by the monitor, **When** the parent becomes healthy again, **Then** the monitor considers that parent recovered and does not restart it again for that same unhealthy event.

---

### User Story 2 - Restart Dependents After Parent Is Healthy (Priority: P2)

As a user with child containers that depend on a parent (via depends_on labels), I want the monitor to restart those child containers only after the parent has been restarted and has become healthy again, so that children do not end up in an unusable state and reconnect correctly to the parent.

**Why this priority**: Ensures correct recovery order (parent first, then dependents) and avoids the situation where children run while the parent is still coming up.

**Independent Test**: Deploy one parent and one or more children with depends_on label pointing to the parent; make parent unhealthy; verify monitor restarts parent, waits until parent is healthy, then restarts each dependent. Delivers swarm-like ordering without swarm.

**Acceptance Scenarios**:

1. **Given** a parent is unhealthy and has dependents (containers with depends_on label naming that parent), **When** the monitor restarts the parent, **Then** the monitor waits until the parent is healthy before restarting any dependent container.
2. **Given** the parent has become healthy after restart, **When** the monitor proceeds to restart dependents, **Then** each dependent that declares depends_on for that parent is restarted.
3. **Given** multiple dependent containers for the same parent, **When** the monitor restarts dependents, **Then** all such dependents are restarted (order of dependent restarts is not specified).

---

### User Story 3 - Dynamic Parent/Child Discovery via Labels (Priority: P1)

As a user, I want the monitor to discover parent/child relationships entirely from container labels (depends_on), so that I do not have to configure container names or dependency lists inside the monitor and any stack using the same label convention is supported.

**Why this priority**: "100% dynamic" and "no matter the parent/child relationship" require zero hardcoding; label-based discovery is the mechanism.

**Independent Test**: Add a new container to the same host with a depends_on label pointing to an existing container; do not change monitor configuration; make the parent unhealthy and verify the new child is restarted after the parent is healthy. Delivers drop-in compatibility with existing compose stacks that use depends_on labels.

**Acceptance Scenarios**:

1. **Given** containers on the same Docker host use a label that indicates dependency (e.g., depends_on listing parent name(s)), **When** the monitor runs, **Then** it discovers parent/child relationships only from those labels.
2. **Given** a container has multiple parents listed in its depends_on label, **When** any of those parents become unhealthy and are restarted, **Then** after each such parent is healthy, the monitor restarts that dependent as needed (per parent recovery).
3. **Given** one parent and multiple children (each with depends_on naming that parent), **When** the parent becomes unhealthy, **Then** the monitor restarts the parent, waits for health, then restarts all of those children.

---

### User Story 4 - Deliverable as Standalone Container Publishable to Registry (Priority: P2)

As a user, I want the monitor delivered as a single Docker image that I can run as a standalone container and optionally pull from a public registry, so that I can add it to any compose file without building from source.

**Why this priority**: Replace autoheal implies drop-in deployment; image publishable to GitHub Container Registry (GHCR) supports reuse and distribution from the same repo.

**Independent Test**: Build the monitor as one image (e.g. via GitHub Actions); run it in a compose stack with no host mounts of source code; verify it monitors and restarts as in P1/P2. Delivers the "standalone docker container" and "build and host on GitHub" outcome.

**Acceptance Scenarios**:

1. **Given** the monitor is built as a single image, **When** run with access to the Docker socket (or equivalent), **Then** it discovers and monitors containers on that host.
2. **Given** the image is published to a registry, **When** a user runs the image with appropriate access to the Docker host, **Then** the same behavior as when built locally is achieved.

---

### Edge Cases

- What happens when a parent never becomes healthy after restart (e.g., persistent failure)? The monitor should not indefinitely restart dependents; it should only restart dependents once the parent has reached a healthy state. If the parent remains unhealthy, the monitor may retry restarting the parent according to policy but must not restart dependents until the parent is healthy.
- How does the system handle a dependent that is already restarting or stopped when the monitor tries to restart it? The monitor should trigger restart for that container; Docker restart is idempotent for running containers and starts stopped ones.
- What happens when the monitor starts and some parents are already unhealthy? The monitor should treat current state as an unhealthy event and apply the same logic: restart parent, wait for health, then restart dependents.
- What happens when depends_on labels reference a container that does not exist or is not on the same host? The monitor should not fail; it should ignore or skip non-existent parents when resolving dependencies and only act on containers that exist and are being monitored.
- What happens when multiple parents of the same dependent become unhealthy in a short window? Each parent is restarted and waited on until healthy; the dependent may be restarted multiple times (once per parent recovery). This is acceptable to keep behavior simple and correct.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST run as a single container that has access to the Docker host (e.g., socket or API) to list containers, inspect labels, and restart containers.
- **FR-002**: The system MUST discover parent/child relationships only from container labels (e.g., a label such as depends_on whose value indicates which container(s) the current container depends on); no static list of container names or dependencies may be required in configuration.
- **FR-003**: The system MUST detect when a monitored parent container becomes unhealthy (using the container’s health status from the runtime).
- **FR-004**: When a parent container becomes unhealthy, the system MUST restart that parent and MUST wait until that parent is reported healthy before restarting any container that declares a dependency on that parent via the depends_on label.
- **FR-005**: The system MUST support one parent with multiple dependent containers; each dependent that lists that parent in depends_on MUST be restarted after the parent is healthy.
- **FR-006**: The system MUST support multiple parents (a container may depend on more than one parent); when a parent becomes unhealthy, the system restarts that parent, waits for it to be healthy, then restarts all containers that list that parent in depends_on.
- **FR-007**: The system MUST NOT require configuration that enumerates container names or dependency graphs; discovery MUST be fully dynamic from labels and runtime state.
- **FR-008**: The system MUST be deliverable as a single container image suitable for deployment in any compose stack and publishable to a container registry; the image MUST be built and hosted via GitHub (e.g., GitHub Container Registry, GHCR) so that pushing the repo triggers build and publish from the same repository.
- **FR-009**: The system MUST NOT modify any files or configuration outside of the project’s own repository (build and runtime behavior must be self-contained).

### Key Entities

- **Container**: A runtime entity on the Docker host; has a name, health status, and labels. May be a parent (depended on by others) or a dependent (depends on others via depends_on label), or both.
- **Parent/child relationship**: Expressed by a label on the dependent container (e.g., depends_on) whose value references one or more parent container names. Discovered dynamically; no static config.
- **Health status**: The reported health state of a container (e.g., healthy, unhealthy, starting, none). Used to decide when to restart a parent and when to proceed to restart dependents.

**Terminology**: In this spec, *child* and *dependent* are used interchangeably (a container that lists another in its depends_on label).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Parent/child relationships are 100% dynamic: no manual configuration of container names or dependency lists is required; any stack that uses the agreed depends_on label convention is supported without code or config changes.
- **SC-002**: The system correctly supports multiple children per parent: when one parent becomes unhealthy, after that parent is restarted and healthy, all containers that declare dependency on that parent are restarted.
- **SC-003**: Recovery order is always enforced: when a parent is unhealthy, the parent is restarted first; dependents are restarted only after the parent is reported healthy, so that child containers do not remain in an unusable state due to a down or starting parent.
- **SC-004**: Behavior is equivalent to swarm-style dependency and health recovery (parent then dependents, health-gated) without requiring Docker Swarm; users achieve reliable child recovery by running the monitor container alongside their existing compose stack.
- **SC-005**: The monitor is delivered as a single container image that can be run standalone and published from GitHub (e.g. GitHub Container Registry); users can add it to a compose file and use it as a drop-in replacement for autoheal for the depends_on/health use case.
- **SC-006**: No changes are made to files or systems outside the project repository; the solution is self-contained within the project.

## Assumptions

- Containers use a healthcheck so that "unhealthy" is detectable by the runtime; containers without healthchecks are out of scope for health-based restart (or are treated as always healthy for the purpose of "wait for parent healthy").
- The depends_on label format (e.g., single parent name or comma-separated list) is agreed and documented; the monitor parses it to resolve parent names.
- The monitor runs with sufficient privileges to list containers, inspect them, and restart them on the same host (e.g., Docker socket bind or Docker API access).
- Implementation will be in Go and the image will be built and published via GitHub (e.g. GitHub Container Registry, GHCR) as a project constraint, so that the repo push drives build and hosting in one place; the specification itself remains technology-agnostic for success criteria.
- Only depends_on-based dependency and health-based restart are in scope; port checks, torrent tracker checks, and similar features are explicitly out of scope.
- "Do not modify any file outside of this project" means the monitor does not edit the user’s compose files or host files; it only interacts with the container runtime API.
