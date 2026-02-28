# Implementation Plan: No Dependent Restarts on Initial Stack Start

**Branch**: `004-child-deps-initial-restart` | **Date**: 2026-02-28 | **Spec**: [spec.md](./spec.md)  
**Input**: Feature specification from `/specs/004-child-deps-initial-restart/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Prevent the monitor from triggering recovery or dependent restarts during an **initial discovery** phase after startup. Initial discovery is defined as: (1) first full discovery cycle (or until all monitored parents have been observed at least once), and (2) a configurable **additional wait time** (environment variable). During this phase the monitor only observes; after the phase completes, existing behavior applies (startup reconciliation for already-unhealthy/stopped parents, and event-driven recovery). This fixes cascade restart failures when the stack is brought up with `docker compose up` (including when the monitor is a dependent via `depends_on`) and allows operators to set the wait time per deployment (e.g. 120s to 5 minutes).

## Technical Context

**Language/Version**: Go 1.21+ (go.mod may specify 1.25; compatible 1.21+)  
**Primary Dependencies**: Docker Engine API (github.com/docker/docker/client), standard library, log/slog, gopkg.in/yaml.v3 (compose parsing)  
**Storage**: N/A (discovery and state from Docker API and compose file)  
**Testing**: Go test; contract-driven; unit tests alongside packages (e.g. internal/discovery/labels_test.go); integration tests for recovery when added  
**Target Platform**: Linux (Docker host); container image (e.g. GHCR)  
**Project Type**: CLI / single binary (containerized monitor)  
**Performance Goals**: No cascade restarts; stack reaches stable state after compose up; recovery latency acceptable for operator use (e.g. a single recovery run completes within minutes)  
**Constraints**: Single binary; no persistent config; discovery from compose + runtime; observability via slog (LOG_LEVEL, LOG_FORMAT from env)  
**Scale/Scope**: One monitor per stack; typical stacks with a small number of parents and dependents

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|--------|
| I. Contract-first behavior | PASS | New/updated contracts for initial-discovery behavior and env var (contracts/ in this feature). |
| II. Spec-branch workflow | PASS | Work on branch `004-child-deps-initial-restart`; spec dir `specs/004-child-deps-initial-restart`. |
| III. Recovery order | PASS | Unchanged: parent first, then dependents one at a time (spec 003). Initial discovery only defers when recovery runs. |
| IV. Observability | PASS | Log initial discovery phase start/end and wait time; structured logging per existing contracts. |
| V. Simplicity and scope | PASS | Single new env var; no persistent config; no features outside spec. |
| No automated commits | PASS | Agent may stage only; no git commit. |
| Stack / layout | PASS | cmd/watch-dog, internal/ (docker, discovery, recovery); env vars per contracts. |

## Project Structure

### Documentation (this feature)

```text
specs/004-child-deps-initial-restart/
├── plan.md              # This file
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/           # Phase 1 (initial-discovery env + behavior)
└── tasks.md             # Phase 2 (/speckit.tasks - not created by plan)
```

### Source Code (repository root)

```text
cmd/watch-dog/
└── main.go              # Entrypoint: add initial discovery phase; gate startup reconciliation and event handling on phase completion

internal/
├── docker/              # Client, events, list/inspect
├── discovery/           # BuildParentToDependents, compose path from env
├── recovery/            # Flow.RunFullSequence (parent → healthy → dependents one at a time)
└── ...*_test.go         # Tests alongside packages
```

**Structure Decision**: Existing layout is used. Changes are confined to `cmd/watch-dog/main.go` (initial discovery phase, env var, gating of `runStartupReconciliation` and event-loop recovery until phase complete) and optionally a small internal helper or config struct for the phase; new contract(s) under `specs/004-child-deps-initial-restart/contracts/`. No new top-level packages required.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations; table left empty.
