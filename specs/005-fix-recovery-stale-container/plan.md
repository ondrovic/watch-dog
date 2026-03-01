# Implementation Plan: Fix Recovery When Containers Are Gone or Unrestartable

**Branch**: `005-fix-recovery-stale-container` | **Date**: 2026-03-01 | **Spec**: [spec.md](./spec.md)  
**Input**: Feature specification from `/specs/005-fix-recovery-stale-container/spec.md`

## Summary

When recovery (restart parent or dependent) fails because the container no longer exists, is marked for removal, or a dependency (e.g. network namespace container) is missing, the monitor currently retries on every subsequent trigger (event or poll) with the same container ID, causing unbounded identical errors. The fix: (1) **Classify** Docker API restart/inspect errors as "unrestartable" (no such container, marked for removal, dependency missing). (2) **Track** container IDs for which restart has failed with an unrestartable error. (3) **Skip** running the full recovery sequence for those IDs until re-discovery yields a different ID for the same logical service (name). (4) **Log** clearly when recovery is skipped due to unrestartable state and when the failure is first detected. (5) **Proactive dependent restart (FR-007)**: When the monitor observes that a parent has a **new** container ID and is healthy (e.g. after re-discovery, such as when an updater replaced the parent), it **proactively restarts all dependents** of that parent so dependents that were failing due to the missing (old) parent's network can re-bind to the new parent; the same dependent restart cooldown as in normal recovery applies. Re-discovery is already in place; to implement (5) the monitor tracks last-known container ID per parent name and, after each discovery, compares current ID to last-known—if different and parent is healthy, run the dependent-restart sequence for that parent (without restarting the parent itself). Addresses [GitHub issue #5](https://github.com/ondrovic/watch-dog/issues/5) (child/dependent when parent is auto-updated).

## Technical Context

**Language/Version**: Go 1.21+ (go.mod may specify 1.25; compatible 1.21+)  
**Primary Dependencies**: Docker Engine API (github.com/docker/docker/client), standard library, log/slog, gopkg.in/yaml.v3 (compose parsing)  
**Storage**: N/A (in-memory unrestartable set and last-known parent ID map only; discovery and state from Docker API and compose file)  
**Testing**: Go test; contract-driven; unit tests alongside packages; extend recovery tests for error classification, skip behavior, and proactive restart  
**Target Platform**: Linux (Docker host); container image (e.g. GHCR)  
**Project Type**: CLI / single binary (containerized monitor)  
**Performance Goals**: Bounded recovery-failure log volume (SC-001); no endless retry loops; proactive restart within one poll or next discovery (SC-005)  
**Constraints**: Single binary; no persistent config; discovery from compose + runtime; observability via slog (LOG_LEVEL, LOG_FORMAT from env)  
**Scale/Scope**: One monitor per stack; typical stacks with a small number of parents and dependents; unrestartable set size bounded (e.g. cap or prune by ID no longer in list)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|--------|
| I. Contract-first behavior | PASS | New/updated contracts for recovery when restart fails (unrestartable) and for proactive dependent restart (FR-007); contracts/ in this feature. |
| II. Spec-branch workflow | PASS | Work on branch `005-fix-recovery-stale-container`; spec dir `specs/005-fix-recovery-stale-container`. |
| III. Recovery order | PASS | Unchanged: parent first, then dependents one at a time. Unrestartable handling only skips or bounds retries. Proactive restart runs dependents only (parent already healthy); same order as normal dependent restart. |
| IV. Observability | PASS | Log unrestartable detection, skip (with reason), identifiable failure reasons, and proactive restart (parent has new ID, restarting dependents) per FR-005. |
| V. Simplicity and scope | PASS | In-memory set and last-known-ID map; error classification by message; no new external deps; no features outside spec. |
| No automated commits | PASS | Agent may stage only; no git commit. |
| Stack / layout | PASS | cmd/watch-dog, internal/ (docker, discovery, recovery); existing layout. |

## Project Structure

### Documentation (this feature)

```text
specs/005-fix-recovery-stale-container/
├── plan.md              # This file
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/           # Phase 1 (recovery unrestartable + proactive dependent restart)
└── tasks.md             # Phase 2 (/speckit.tasks - not created by plan)
```

### Source Code (repository root)

```text
cmd/watch-dog/
└── main.go              # Pass nameToID into tryRecoverParent/Flow.RunFullSequence; call prune after ListContainers; after discovery, when parent has new ID and healthy call flow.RestartDependents for that parent (proactive restart per FR-007)

internal/
├── docker/              # Client (Restart, Inspect); unchanged except possibly log helpers
├── discovery/            # Unchanged
├── recovery/
│   ├── restart.go       # Flow, RunFullSequence, WaitUntilHealthy, RestartDependents; integrates unrestartable set (skip, add on failure); RestartDependents is invoked from main for proactive case (parent has new ID and healthy) without prior RestartParent
│   ├── errors.go        # Error classification: IsUnrestartableError (no such container, marked for removal, dependency missing)
│   ├── unrestartable.go # Unrestartable set type (bounded, thread-safe add/contains/prune); Flow holds an instance
│   └── *_test.go        # Tests alongside packages
└── ...*_test.go         # Tests alongside packages
```

**Structure Decision**: Unrestartable set type and its methods live in `internal/recovery/unrestartable.go`. Error classification in `internal/recovery/errors.go`. Flow stays in `internal/recovery/restart.go` and holds the unrestartable set; skip and add-on-failure logic run from RunFullSequence and RestartDependents. For FR-007: track **last-known container ID per parent name** (e.g. in main or on Flow); after each discovery (event path and polling), for each parent compare current ID from the container list to last-known—if different and parent is healthy, call a method that restarts only the dependents of that parent (reusing RestartDependents with same cooldown), then update last-known to current ID. Callers (main.go) share the same Flow instance. All new exported types and functions must have Go docstrings.

## Complexity Tracking

No violations; table left empty.
