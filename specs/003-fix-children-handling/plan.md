# Implementation Plan: Correct Handling of Dependent (Child) Containers

**Branch**: `003-fix-children-handling` | **Date**: 2026-02-28 | **Spec**: [spec.md](./spec.md)  
**Input**: Feature specification from `/specs/003-fix-children-handling/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

When a parent container (e.g. VPN or dler) is restarted and becomes healthy, the monitor must restart its dependents **one at a time** in a **deterministic order**, and when the monitor is itself a dependent it must **restart itself last** (or skip self) so that in-flight operations (other restarts, discovery, event stream) are not canceled by the process exiting. Current behavior restarts dependents in a simple loop but uses a single process context; restarting the watch-dog container cancels that context and causes "context canceled" for remaining restarts and discovery. The fix: (1) enforce a stable order for dependents (e.g. sort by name); (2) when the current process’s container name is in the dependent list, restart all other dependents first, then restart self so no work is in-flight when the process stops.

## Technical Context

**Language/Version**: Go 1.25 (go.mod); compatible with Go 1.21+  
**Primary Dependencies**: github.com/docker/docker/client (Docker Engine API), gopkg.in/yaml.v3 (compose parsing), standard library, log/slog  
**Storage**: N/A (no persistent storage; discovery and state from Docker API and compose file)  
**Testing**: Go standard testing (e.g. `go test ./...`); integration tests via Docker socket where needed  
**Target Platform**: Linux (Docker host); Docker socket access required  
**Project Type**: CLI / daemon (single binary, long-running monitor)  
**Performance Goals**: Recovery completes within minutes (parent restart + wait healthy + N dependent restarts); no hard throughput target  
**Constraints**: Must not cancel in-flight Docker API calls when the monitor restarts itself; sequential dependent restarts to avoid overload and cancellation  
**Scale/Scope**: Typical stack 5–20 containers; dependents per parent usually 1–10

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The repository constitution (`.specify/memory/constitution.md`) is a template with placeholder principles and no project-specific gates. No violations are defined. **Proceed with Phase 0 and Phase 1.**

## Project Structure

### Documentation (this feature)

```text
specs/003-fix-children-handling/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/watch-dog/
└── main.go              # Entrypoint; discovery, events, recovery flow

internal/
├── discovery/
│   ├── compose.go       # Compose parse, BuildServiceParentToDependents, BuildParentToDependentsFromCompose
│   └── labels.go        # BuildParentToDependents, ParentToDependents, ComposePathFromEnv
├── docker/
│   ├── client.go        # ListContainers, Inspect, Restart
│   ├── events.go        # Health event subscription
│   └── log.go           # Structured logging
└── recovery/
    └── restart.go       # Flow: RestartParent, WaitUntilHealthy, RestartDependents, RunFullSequence

tests/                   # As needed (unit/integration)
```

**Structure Decision**: Single Go module; `cmd/watch-dog` for the binary; `internal/` for discovery, Docker client, and recovery flow. Changes for this feature are in `internal/recovery/restart.go` (sequential order, self last), `internal/discovery/compose.go` (deterministic dependent order if needed), and `cmd/watch-dog/main.go` (pass self container name into recovery).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

Not applicable; no violations.
