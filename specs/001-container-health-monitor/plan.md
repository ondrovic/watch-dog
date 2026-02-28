# Implementation Plan: Container Health Monitor (Watch-Dog)

**Branch**: `001-container-health-monitor` | **Date**: 2025-02-28 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-container-health-monitor/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Build a standalone Docker container (Go) that monitors container health on the same host, discovers parent/child relationships from the `depends_on` label, and restarts unhealthy parents then—after they become healthy—restarts their dependents. Delivered as a single image built and published via GitHub (GHCR). No static config; 100% dynamic from labels and runtime state.

## Technical Context

**Language/Version**: Go 1.21+  
**Primary Dependencies**: Docker Engine API (e.g. github.com/docker/docker/client), standard library  
**Storage**: N/A (no persistent storage; in-memory state only)  
**Testing**: Go testing (stdlib), optional testify; contract tests for label format and restart behavior  
**Target Platform**: Linux (Docker host); container runs as single process with Docker socket or DOCKER_HOST  
**Project Type**: CLI/daemon (long-running process in container)  
**Performance Goals**: React to health_status events within seconds; optional polling fallback on same order (e.g. 60s). Reaction time is satisfied by the event-driven subscription (see tasks T007, T022); no separate verification task is required.  
**Constraints**: Single binary, minimal image (e.g. scratch or distroless); read-only except Docker API; no files modified outside project.  
**Scale/Scope**: Single host; tens of containers; one monitor instance per host.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- Constitution file (`.specify/memory/constitution.md`) is still a template and not yet ratified for this project.
- **No project-specific gates applied.** Proceeding with plan; when the constitution is ratified, re-run this check against the listed principles.

## Project Structure

### Documentation (this feature)

```text
specs/001-container-health-monitor/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
└── watch-dog/
    └── main.go          # Entrypoint; wire Docker client, event loop, restart logic

internal/
├── docker/
│   ├── client.go        # Docker client wrapper (list, inspect, restart)
│   └── events.go       # Subscribe to health_status events (and optional poll)
├── discovery/
│   └── labels.go        # Parse depends_on label; build parent→dependents map
└── recovery/
    └── restart.go       # Restart parent → wait healthy → restart dependents

tests/
├── contract/            # Label format contract; expected restart sequence
├── integration/         # Optional: real Docker socket tests (CI or manual)
└── unit/                # Mock client tests for discovery and recovery logic

.github/
└── workflows/
    └── build-push.yml   # Build image and push to GHCR on push/tag
```

**Structure Decision**: Single Go module at repo root. `cmd/watch-dog` is the only binary. `internal/` keeps Docker, discovery, and recovery logic private. Contracts document the `depends_on` label format and restart behavior for consumers and tests. CI builds and pushes to GitHub Container Registry.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| (none) | — | — |
