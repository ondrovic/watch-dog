# Tasks: No Dependent Restarts on Initial Stack Start

**Input**: Design documents from `/specs/004-child-deps-initial-restart/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Spec does not request new test tasks; verification is via quickstart.md. Contract alignment is ensured by implementation tasks.

**Organization**: Tasks are grouped by user story so each story can be implemented and verified independently.

**Docstrings**: When adding new items (env vars, types, functions, or behavior), keep package and function docstrings updated in the same change; see T011 and the Notes section.

**Cascade verification**: Ensure we do not get into a cascade failure of restarts after initial discovery has completed (spec FR-007, SC-005). The design may already prevent this (startup reconciliation once at phase end; event/polling only on new unhealthy state; cooldown/in-flight). We must **verify** this explicitly; see T013 and T008.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story (US1, US2) for phase 3+
- Include exact file paths in descriptions

## Path Conventions

- **This feature**: All implementation in `cmd/watch-dog/main.go` at repository root. Contracts and docs under `specs/004-child-deps-initial-restart/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm project structure per plan; no new packages.

- [x] T001 Verify project structure per plan (specs/004-child-deps-initial-restart/plan.md § Project Structure): `cmd/watch-dog/`, `internal/` (docker, discovery, recovery) exist and no new top-level packages required for this feature

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Initial discovery wait config and phase state. MUST be complete before user story implementation.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T002 Parse `WATCHDOG_INITIAL_DISCOVERY_WAIT` from env in `cmd/watch-dog/main.go` (init or early main): use `time.ParseDuration`, default 60s when unset or invalid; log warning on invalid value per specs/004-child-deps-initial-restart/contracts/env-initial-discovery.md; add or update docstring for the init/package describing the new env var
- [x] T003 Add initial discovery phase state in `cmd/watch-dog/main.go`: compute phase end time as (first discovery completion time + wait duration) and expose a function or check `isInitialDiscoveryComplete()` so recovery paths can gate on it per specs/004-child-deps-initial-restart/data-model.md; add docstrings for any new types or functions

**Checkpoint**: Foundation ready — initial discovery wait duration and phase-complete check available

---

## Phase 3: User Story 1 — No Dependent Restarts When Stack Brought Up or Already Healthy (Priority: P1) — MVP

**Goal**: During initial discovery (first discovery + wait), the monitor does not run recovery or restart any dependents; operators can bring the stack up with `docker compose up` without cascade restarts.

**Independent Test**: Run `docker compose up` with full stack (including monitor); confirm no dependent restarts and no cascade; logs show "initial discovery started" and after wait "initial discovery complete, recovery enabled".

### Implementation for User Story 1

- [x] T004 [US1] Log at INFO "initial discovery started" with wait duration when monitor starts in `cmd/watch-dog/main.go` per specs/004-child-deps-initial-restart/research.md and contracts/initial-discovery-behavior.md
- [x] T005 [US1] Defer `runStartupReconciliation` until initial discovery phase has elapsed; run it exactly once when phase ends (e.g. after wait) in `cmd/watch-dog/main.go` per research.md §4
- [x] T006 [US1] In health event handler and in `runPollingFallback`, skip calling `flow.RunFullSequence` until `isInitialDiscoveryComplete()` in `cmd/watch-dog/main.go`
- [x] T007 [US1] When initial discovery phase elapses, log at INFO "initial discovery complete, recovery enabled" in `cmd/watch-dog/main.go` per contracts/initial-discovery-behavior.md

**Checkpoint**: User Story 1 complete — no recovery during initial discovery; compose up does not cause cascade

---

## Phase 4: User Story 2 — Dependent Restarts Only After Parent Recovery (Priority: P2)

**Goal**: After initial discovery phase completes, the monitor runs startup reconciliation once and reacts to health events and polling with recovery (restart parent then dependents per 003). No restarts during the phase; restarts only after phase and only when the monitor has recovered a parent.

**Independent Test**: Start monitor with stack healthy; wait for "initial discovery complete" in logs; then stop the parent — verify monitor restarts parent and then dependents. Compare: during initial discovery, no restarts.

### Implementation for User Story 2

- [x] T008 [US2] Ensure `runStartupReconciliation` is invoked exactly once when initial discovery phase completes (not at startup) and event/polling recovery only run when phase is complete in `cmd/watch-dog/main.go`; ensure post-phase logic cannot cause a cascade (e.g. startup reconciliation runs once, not in a loop; cooldown/in-flight prevent duplicate recovery runs); add brief comment referencing specs/004-child-deps-initial-restart/contracts/initial-discovery-behavior.md
- [x] T009 [US2] Confirm specs/004-child-deps-initial-restart/quickstart.md section "Verify recovery still works after phase" accurately describes post-initial-discovery behavior; update if needed

**Checkpoint**: User Stories 1 and 2 both satisfied — no restarts on initial start; recovery works after phase

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, docstrings, and validation.

- [x] T010 [P] Add `WATCHDOG_INITIAL_DISCOVERY_WAIT` to README (description, example values e.g. 120s, 5m) per specs/004-child-deps-initial-restart/contracts/env-initial-discovery.md
- [x] T011 Update package and function docstrings in `cmd/watch-dog/main.go` for all new or changed items: initial discovery wait env var, phase state / phase end time, `isInitialDiscoveryComplete()` (or equivalent), and main flow (initial discovery phase vs recovery); ensure docstrings reflect current behavior so future readers see accurate docs
- [ ] T012 Run quickstart validation per specs/004-child-deps-initial-restart/quickstart.md: compose up, verify no restarts during phase; after phase, stop parent and verify recovery *(manual: run locally with your stack)*
- [x] T013 Verify no cascade failure after initial discovery: after phase completes, trigger one recovery (e.g. stop parent once); confirm recovery runs once and stack reaches stable state with no sustained or repeated restart loop per spec FR-007 and SC-005; add or update a "No cascade after phase" verification step in specs/004-child-deps-initial-restart/quickstart.md if not already covered
- [x] T014 [P] Verify spec FR-005 in specs/004-child-deps-initial-restart/spec.md includes an unambiguous example for "observable container start times" (e.g. docker inspect restart count or container start timestamp); add the example only if missing so verification is testable

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — BLOCKS all user stories
- **Phase 3 (US1)**: Depends on Phase 2 — implements no-restart-during-initial-discovery
- **Phase 4 (US2)**: Depends on Phase 3 — ensures post-phase recovery behavior and docs
- **Phase 5 (Polish)**: Depends on Phase 4 — README, docstrings, quickstart validation, and spec clarification (FR-005)

### User Story Dependencies

- **User Story 1 (P1)**: After Foundational; no dependency on US2. Delivers: initial discovery phase, gating, logging.
- **User Story 2 (P2)**: After US1; verifies and documents that recovery runs only after phase. Depends on same main.go changes (startup reconciliation at phase end, event/polling gating).

### Within Each User Story

- T004 → T005, T006, T007 (all use phase state from T002–T003)
- T005 and T006 can be implemented in either order (same file, different call sites)
- T008 and T009 can be done in parallel (code comment vs quickstart doc)

### Parallel Opportunities

- Phase 5: T010 (README), T011 (docstrings), T012 (quickstart run), T013 (cascade verification), T014 (verify FR-005 clarification in spec) — T010, T011, and T014 can run in parallel; T012 and T013 are manual validation (T013 can follow T012). FR-005 already contains the example; T014 is verify-only and can be marked complete after check.
- Phase 4: T008 (code) and T009 (quickstart doc) are independent.

---

## Parallel Example: User Story 1

```text
# After T002 and T003 are done, US1 implementation order:
T004 first (logging at start)
Then T005 (defer startup reconciliation until phase end)
Then T006 (gate event loop and polling)
Then T007 (log when phase ends)

# T005 and T006 touch different code paths (startup vs event/polling) but same file — implement sequentially to avoid merge conflicts.
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup  
2. Complete Phase 2: Foundational (T002, T003)  
3. Complete Phase 3: User Story 1 (T004–T007)  
4. **STOP and VALIDATE**: Run `docker compose up`, confirm no restarts during initial discovery and logs show phase start/end  
5. Demo: compose up without cascade; optional deploy

### Incremental Delivery

1. Setup + Foundational → phase state and env ready  
2. Add US1 → validate no restarts on initial start (MVP)  
3. Add US2 → validate recovery after phase and update quickstart if needed  
4. Polish → README and quickstart validation  

### Single-File Strategy

All implementation is in `cmd/watch-dog/main.go`. Recommended order: T002 → T003 → T004 → T005 → T006 → T007 → T008; then T009 (quickstart), T010 (README), T011 (docstrings), T012 (run quickstart), T013 (verify no cascade after phase), T014 (spec FR-005). If a small internal helper is added for phase state, keep it in main.go or document in plan; no new packages.

---

## Notes

- **Docstrings**: Keep docstrings updated when adding new items. When introducing a new env var, type, function, or behavior, add or update the corresponding package/function docstring in the same change. T002 and T003 call this out; T011 is a dedicated pass to ensure `cmd/watch-dog/main.go` docstrings reflect the full initial-discovery behavior.
- **Cascade after phase**: We must verify that after initial discovery completes we do not get into a cascade of restarts (FR-007, SC-005). T008 ensures the implementation avoids it (startup reconciliation once; no loop). T013 is explicit verification: one recovery after phase, then confirm stack settles with no sustained restart loop; add quickstart step if needed.
- **Optional helper**: Plan allows "optionally a small internal helper or config struct"; tasks keep all logic in `cmd/watch-dog/main.go`. If a helper is added later, keep it in main.go or document in plan; no new top-level packages.
- [P] tasks: T010 (README), T011 (docstrings), and T014 (spec FR-005) can be done in parallel with T012 (quickstart run) or T013 (cascade verification).
- [Story] label maps tasks to US1 or US2 for traceability.
- Each user story is independently testable via quickstart steps.
- No new test files required by spec; extend tests when touching contracts if project practice requires it.
- Commit after each task or logical group (e.g. after T002+T003, after T004–T007).
