# Tasks: Correct Handling of Dependent (Child) Containers

**Input**: Design documents from `/specs/003-fix-children-handling/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Not explicitly requested in the spec; verification is via quickstart.md manual steps.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Verify project and feature artifacts before implementation

- [x] T001 Verify project structure and feature specs: ensure specs/003-fix-children-handling/ contains plan.md, spec.md, research.md, data-model.md, quickstart.md and contracts/ (dependent-restart-order.md)
- [x] T002 [P] Verify Go build from repo root: `go build -o watch-dog ./cmd/watch-dog` per plan.md

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add self-container-name support and recovery API so user story implementation can use it

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T003 Add optional self container name to recovery flow: extend RunFullSequence and RestartDependents in internal/recovery/restart.go to accept an optional selfName string (e.g. RunFullSequence(ctx, parentID, parentName, discovery, selfName), RestartDependents(ctx, parentName, discovery, selfName))
- [x] T004 Read WATCHDOG_CONTAINER_NAME from environment in cmd/watch-dog/main.go and pass to flow.RunFullSequence for both event-driven recovery and startup reconciliation and polling fallback

**Checkpoint**: Foundation ready – RestartDependents can receive self name; main passes it

---

## Phase 3: User Story 1 – Parent Recovery Completes All Dependent Restarts (Priority: P1) 🎯 MVP

**Goal**: After a parent recovers, every dependent receives a restart attempt; no skip due to context canceled; each attempt is logged.

**Independent Test**: Stop a parent (e.g. VPN); confirm parent restarts and becomes healthy, then every dependent is restarted and logs show each attempt (no "context canceled" causing skips).

### Implementation for User Story 1

- [x] T005 [US1] In RestartDependents in internal/recovery/restart.go: build ordered list from discovery.GetDependents(parentName) by sorting names deterministically (e.g. sort.Strings), then if selfName is non-empty and in the list move it to last; iterate one-by-one and call Client.Restart for each; on error log and continue to next
- [x] T006 [US1] In RestartDependents in internal/recovery/restart.go: log each dependent restart attempt (success: "restarted dependent" with dependent/parent; failure: existing "restart dependent" error log) so operators can verify all dependents were processed (FR-007)

**Checkpoint**: User Story 1 complete – all dependents get a restart attempt in order with logging

---

## Phase 4: User Story 2 – Dependent Restarts Are Sequential (One at a Time) (Priority: P2)

**Goal**: Dependents are restarted one at a time in a deterministic order; no parallel restart calls.

**Independent Test**: Trigger parent recovery with multiple dependents; logs show restarts in a stable order (e.g. alphabetical), one after another.

### Implementation for User Story 2

- [x] T007 [US2] Ensure RestartDependents in internal/recovery/restart.go uses a single sequential loop (no goroutines) and that the dependent list is sorted before iteration so order is deterministic per specs/003-fix-children-handling/contracts/dependent-restart-order.md *(Satisfied by T005; treat as verification/contract check.)*

**Checkpoint**: User Story 2 complete – sequential, deterministic order is enforced and observable

---

## Phase 5: User Story 3 – Monitor Restart Does Not Cancel In-Flight Recovery (Priority: P3)

**Goal**: When the monitor is in the dependent list, it restarts itself last so no in-flight restart or discovery is canceled.

**Independent Test**: Compose with watch-dog as dependent of parent; set WATCHDOG_CONTAINER_NAME=watch-dog; stop parent; after recovery, other dependents restart first, then watch-dog; no "context canceled" for restart dependent / refresh discovery / docker events before the last restart.

### Implementation for User Story 3

- [ ] T008 [US3] Ensure RestartDependents in internal/recovery/restart.go moves self (when selfName is set and present in the list) to last position before the restart loop so the monitor’s container is restarted last per specs/003-fix-children-handling/contracts/dependent-restart-order.md *(Satisfied by T005; treat as verification/contract check.)*

**Checkpoint**: User Story 3 complete – self is last when in dependent list (T005/T007/T008 are satisfied by the same implementation; T008 is verification/documentation of self-last)

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Validation and documentation

- [ ] T009 [P] Run quickstart.md validation: follow specs/003-fix-children-handling/quickstart.md to verify sequential dependent restart and self-restart-last behavior (manual or script)
- [x] T010 Update README or docs: document WATCHDOG_CONTAINER_NAME (optional) for self-restart-last behavior when watch-dog is a dependent, per specs/003-fix-children-handling/quickstart.md

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies – can start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 – BLOCKS all user stories
- **Phase 3 (US1)**: Depends on Phase 2 – delivers “all dependents restarted” + ordering + logging
- **Phase 4 (US2)**: Depends on Phase 3 – same code; verification that order is sequential and deterministic
- **Phase 5 (US3)**: Depends on Phase 3 – same code; verification that self is last
- **Phase 6 (Polish)**: Depends on Phase 3–5 complete

### User Story Dependencies

- **US1 (P1)**: After Phase 2; no dependency on US2/US3
- **US2 (P2)**: Implemented in same RestartDependents changes as US1; Phase 4 verifies
- **US3 (P3)**: Implemented in same RestartDependents changes as US1 (self last); Phase 5 verifies

### Within Implementation

- T003 (recovery API) before T004 (main passes env)
- T005 (order + loop + self last) delivers US1; T006 (logging) can be implemented in the same change as T005—log each attempt on success and on error
- T007 and T008 are satisfied by the same RestartDependents implementation; they are verification/contract alignment tasks

### Parallel Opportunities

- T001 and T002 can run in parallel (Phase 1)
- T009 and T010 can run in parallel (Phase 6)

---

## Parallel Example: Phase 1

```bash
# Verify specs and build in parallel:
Task T001: "Verify project structure and feature specs in specs/003-fix-children-handling/"
Task T002: "Verify Go build: go build -o watch-dog ./cmd/watch-dog"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (T003, T004)
3. Complete Phase 3: User Story 1 (T005, T006)
4. **STOP and VALIDATE**: Run quickstart “Verify sequential dependent restart”; confirm all dependents get a restart attempt and logs show each
5. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → recovery accepts self name, main passes it
2. Phase 3 (US1) → all dependents restarted in order with logging (MVP)
3. Phase 4–5 (US2, US3) → confirm sequential order and self-last in code and contract
4. Phase 6 → quickstart validation and README

### Single-Developer Flow

Implementation is small: T005/T006 (and effectively T007/T008) are one cohesive change in internal/recovery/restart.go (sort, self last, loop, log). T003/T004 are small API and main changes. Recommended: do T001–T004, then T005–T006 in one pass in restart.go, then T007–T008 as code review/contract check, then T009–T010.

---

## Notes

- [P] tasks = different files or independent checks
- [Story] label maps task to spec user story for traceability
- No separate test tasks; verification via quickstart.md
- All tasks include file paths or spec references
- Format: `- [ ] Tnnn [P?] [USn?] Description with path`
