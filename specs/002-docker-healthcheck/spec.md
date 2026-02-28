# Feature Specification: Add Healthcheck to Watch-Dog Docker Container

**Feature Branch**: `002-docker-healthcheck`  
**Created**: 2026-02-28  
**Status**: Draft  
**Input**: User description: "add healthcheck to docker container"

## Clarifications

### Session 2026-02-28

- Q: How should liveness be determined given watch-dog has no HTTP server (e.g. in a compose stack with only socket and compose path)? → A: Docker API check – run a minimal Docker command inside the container; success means process is up and socket works.
- Q: What healthcheck interval, start period, timeout, and retries should be used? → A: Interval 15s, retries 2, start period 20s, timeout 10s (user .env reference values).
- Q: Where should the healthcheck be defined (Dockerfile vs docs only)? → A: Both – same parameters in Dockerfile and documented in README/compose example so users can copy or override.
- Q: Should healthcheck interval and related parameters be hardcoded or sourced from .env? → A: Pull from .env; use canonical variable names INTERVAL_IN_SECS, RETRIES, START_PERIOD_IN_SECS, TIMEOUT_IN_SECS (reference values: 15s, 2, 20s, 10s). Documented configuration must not hardcode values.
- Q: Should improved log display be part of this feature (002) or a separate feature? → A: Include in this feature – same release delivers both healthcheck and improved log format.
- Q: Should log level be configurable? → A: Yes, via LOG_LEVEL environment variable; output only messages at or above the configured level (e.g. DEBUG, INFO, WARN, ERROR). Default INFO.
- Q: Which log output format styles should be supported and how are they selected? → A: Log format is configurable (e.g. via LOG_FORMAT); at least two documented styles: "compact" ([Level] message) and "timestamp" (timestamp [Level] message). A third style "json" is recommended for machine parsing. Custom names are defined and documented; default when unset is "timestamp" (or as specified in plan).
- Q: Where must LOG_LEVEL and LOG_FORMAT be documented with examples? → A: The README MUST provide examples for all supported LOG_LEVEL values (DEBUG, INFO, WARN, ERROR) and all supported LOG_FORMAT values (compact, timestamp, and json if supported)—e.g. example env/compose snippets or example log output for each configuration so users can copy and troubleshoot.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Container Reports Health Status (Priority: P1)

As a user or orchestrator running the watch-dog container, I want the container to expose a health status (e.g. in `docker ps` or compose health status) so that I can see at a glance whether the monitor is running and functioning, and so that restart policies or other monitors can react if watch-dog itself becomes unhealthy.

**Why this priority**: Without a healthcheck, the container shows as "running" even if the process has hung or exited; adding a healthcheck makes watch-dog observable and allows automated recovery of the monitor itself.

**Independent Test**: Run the watch-dog image with the healthcheck defined; run `docker ps` (or equivalent) and confirm the container shows a health status (healthy/starting/unhealthy). Stop or kill the watch-dog process inside the container and confirm the container eventually reports unhealthy. Delivers immediate value for operators and orchestrators.

**Acceptance Scenarios**:

1. **Given** the watch-dog container is running and the monitor process is functioning, **When** the healthcheck runs, **Then** the container is reported as healthy (or starting during initial interval).
2. **Given** the watch-dog container is running but the monitor process has stopped or is not responding, **When** the healthcheck runs, **Then** the container is reported as unhealthy after the configured retries/interval.
3. **Given** the container is started, **When** viewed via `docker ps` or compose health output, **Then** a health status column or indicator is present (healthy / unhealthy / starting, or equivalent).

---

### User Story 2 - Readable Log Output (Priority: P2)

As a user or operator viewing watch-dog container logs (e.g. `docker logs watch-dog` or a log UI), I want log lines to be human-readable at a glance (e.g. level and message prominent, consistent with other containers in the stack) so that I can quickly see what the monitor is doing without parsing key=value style output.

**Why this priority**: Improves day-to-day operability alongside the healthcheck; both are observability improvements for the same release.

**Independent Test**: Run the container and trigger a few events (startup, parent recovery); view logs and confirm each line is easy to read (level and message clear without parsing key=value).

**Acceptance Scenarios**:

1. **Given** the watch-dog container is running, **When** logs are viewed (stdout), **Then** each log line uses a human-readable format (level and message clearly visible, not raw key=value only).
2. **Given** an event such as "parent needs recovery" or "restarted dependent", **When** logs are viewed, **Then** the message and relevant attributes (e.g. parent, dependent) are readable without parsing dense key=value strings.
3. **Given** LOG_LEVEL is set (e.g. DEBUG, INFO, WARN, ERROR), **When** the container runs, **Then** only log messages at or above that level are output (e.g. LOG_LEVEL=WARN shows only WARN and ERROR).
4. **Given** LOG_FORMAT is set to a supported style (e.g. compact, timestamp, json), **When** logs are viewed, **Then** each line follows that style (compact: [Level] message; timestamp: timestamp [Level] message; json: one JSON object per line).

---

### Edge Cases

- What happens when the Docker socket or required resources are temporarily unavailable during a health check? The healthcheck should reflect actual monitor liveness; transient resource issues may result in a temporary unhealthy or starting state until the next successful check.
- How does the system behave when the container is under heavy load? The healthcheck should complete within a defined timeout so it does not block or overload the container; repeated failures lead to unhealthy.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The watch-dog Docker image MUST define a healthcheck (e.g. in the Dockerfile) so that the container reports a health status by default; the same parameters MUST be documented in the README and (as applicable) in a compose example so users can copy or override.
- **FR-002**: The healthcheck MUST report healthy when the monitor process is running and able to communicate with the Docker API (e.g. by running a minimal Docker command such as listing containers or querying the daemon); no HTTP endpoint is required.
- **FR-003**: The healthcheck MUST report unhealthy when the monitor process is not running or is not responding to the check within a defined timeout.
- **FR-004**: The healthcheck MUST run at a defined interval with a bounded start period, timeout, and retries. Parameters MUST be configurable via .env using the canonical variable names INTERVAL_IN_SECS, RETRIES, START_PERIOD_IN_SECS, TIMEOUT_IN_SECS; documented configuration (e.g. compose example) MUST use these variables rather than hardcoded values. Reference values for documentation: 15s, 2, 20s, 10s respectively.
- **FR-005**: The healthcheck definition and parameters MUST be documented in the README (and in a compose example where relevant) using the .env variable names (INTERVAL_IN_SECS, RETRIES, START_PERIOD_IN_SECS, TIMEOUT_IN_SECS) so that values are not hardcoded; users and operators know how health is determined, how to interpret healthy/unhealthy, and how to override via .env if needed. *(FR-004 defines the healthcheck behavior and .env parameter names; FR-005 defines the documentation and compose-example obligation.)*
- **FR-006**: Log output to stdout MUST support configurable format styles so that operators can choose readability vs machine parsing. At least two styles MUST be supported and documented: **compact** (`[Level] message`, e.g. `[INFO] watch-dog started parents=[vpn dler ...]`) and **timestamp** (`timestamp [Level] message`, e.g. `2026-02-28T18:04:26Z [INFO] parent needs recovery parent=vpn`). Format MUST be selectable via an environment variable (e.g. LOG_FORMAT); allowed values and default MUST be documented. The README MUST include examples for every supported LOG_FORMAT value (e.g. compose/env snippet or example log line per style).
- **FR-007**: Log level MUST be configurable via the LOG_LEVEL environment variable; only messages at or above the configured level are output. Supported levels MUST include at least DEBUG, INFO, WARN, ERROR; default MUST be INFO when LOG_LEVEL is unset. Behavior and allowed values MUST be documented. The README MUST include examples for every supported LOG_LEVEL value (e.g. compose/env snippet or short description of what each level shows).
- **FR-008**: A third log format style **json** (one JSON object per line) MAY be supported for machine parsing and log aggregators; if supported, it MUST be documented alongside compact and timestamp with an example in the README.

### Key Entities

- **Healthcheck**: The mechanism by which the container reports liveness to the runtime; uses a minimal Docker API check (no HTTP server). Parameters are sourced from .env via INTERVAL_IN_SECS, RETRIES, START_PERIOD_IN_SECS, TIMEOUT_IN_SECS (no hardcoded values in user-facing config). Reference values: 15s, 2, 20s, 10s; status is observable and actionable.
- **Log level (LOG_LEVEL)**: Environment variable controlling log verbosity; only messages at or above the configured level (DEBUG, INFO, WARN, ERROR) are output. Default INFO.
- **Log format (LOG_FORMAT)**: Environment variable selecting output style. Supported styles: **compact** – `[Level] message`; **timestamp** – `timestamp [Level] message`; **json** (optional) – one JSON object per line. Custom names are defined and documented; default when unset to be specified in plan (e.g. timestamp).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operators can see a health status (healthy / unhealthy / starting) for the watch-dog container when listing or inspecting the container (e.g. within one healthcheck interval after start).
- **SC-002**: When the monitor process is stopped or unresponsive, the container transitions to unhealthy within the configured retries (default 2 consecutive failures) and timeout (default 10s per check).
- **SC-003**: The healthcheck completes within its configured timeout (default 10s) on each run so that it does not cause resource or scheduling issues under normal operation.
- **SC-004**: Documentation describes how health is determined and what healthy/unhealthy means, so that users can troubleshoot and integrate with orchestrators or other monitors.
- **SC-005**: When viewing container logs, operators can read each line without parsing raw key=value; level and message are clearly visible. Operators can set LOG_FORMAT to choose compact or timestamp (and optionally json) style.
- **SC-006**: Operators can set LOG_LEVEL (e.g. DEBUG, INFO, WARN, ERROR) to control which messages appear; when unset, default level (INFO) applies and only INFO and above are shown.
- **SC-007**: The README lists the supported LOG_FORMAT values (compact, timestamp, and if implemented json) with an example line or configuration snippet for each style, and lists the supported LOG_LEVEL values (DEBUG, INFO, WARN, ERROR) with an example or description for each, so users can copy configurations and troubleshoot.
