# Tasks: Fix Recovery When Containers Are Gone or Unrestartable

**Input**: Design documents from `/specs/005-fix-recovery-stale-container/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Optional; plan requests extending recovery tests for error classification and skip behavior. One unit test phase for the classifier is included.

**Organization**: Tasks are grouped by user story so each story can be implemented and validated independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story (US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Repository layout per plan: `cmd/watch-dog/main.go`, `internal/docker/`, `internal/discovery/`, `internal/recovery/`.
- Recovery package (this feature): `internal/recovery/restart.go` (Flow, RunFullSequence, RestartDependents), `internal/recovery/errors.go` (error classification), `internal/recovery/unrestartable.go` (unrestartable set type). No flow.go; Flow remains in restart.go.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm project structure; no new project init (existing Go binary).

- [x] T001 Verify project structure per plan (cmd/watch-dog, internal/docker, internal/discovery, internal/recovery) and that no new dependencies are required

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Error classification and unrestartable set type that all user stories depend on.

**Critical**: No user story work can begin until this phase is complete.

- [x] T002 [P] Add function to classify Docker API errors as unrestartable in internal/recovery/errors.go (No such container, marked for removal, joining network namespace + No such container) per research.md and contracts/recovery-unrestartable-behavior.md; include package and exported function docstrings (e.g. IsUnrestartableError) per Go conventions
- [x] T003 Add in-memory unrestartable set type (bounded size, e.g. max 100 IDs) with thread-safe add/contains and optional prune in internal/recovery/unrestartable.go per data-model.md; include package and exported type/method docstrings per Go conventions
- [x] T004 Add unrestartable set field to recovery.Flow and wire check-before-run (skip when ID in set) and add-on-failure in internal/recovery/restart.go; add or update Flow and method docstrings for new behavior

**Checkpoint**: Foundation ready; US1 implementation can begin.

---

## Phase 3: User Story 1 - No Repeated Failed Recovery Attempts (Priority: P1) — MVP

**Goal**: When restart or inspect fails with an unrestartable error, the monitor records that container ID and does not retry the same ID on subsequent triggers; retries are bounded.

**Independent Test**: Remove or mark-for-removal a monitored parent; run the monitor; confirm at most one (or a small bounded number of) recovery failure log lines for that container, then skip messages instead of repeated identical errors.

### Optional: Tests for User Story 1

- [x] T005 [P] [US1] Add unit tests for unrestartable error classification (no such container, marked for removal, dependency missing) in internal/recovery/errors_test.go

### Implementation for User Story 1

- [x] T006 [US1] At start of RunFullSequence, if parentID is in the unrestartable set then skip sequence and log recovery skipped for parent (INFO or DEBUG) in internal/recovery/restart.go
- [x] T007 [US1] On Restart(parentID) failure in RunFullSequence, if error is unrestartable then add parentID to set and log failure with reason (container gone / marked for removal / dependency missing); return without wait or dependents in internal/recovery/restart.go
- [x] T008 [US1] In WaitUntilHealthy, on Inspect(parentID) error classify as unrestartable; if so add parentID to set, log, and return false in internal/recovery/restart.go
- [x] T009 [US1] Pass nameToID (or equivalent) into RunFullSequence and RestartDependents so dependent container ID is available when restarting dependents; in main.go ensure nameToID from ListContainers is passed when calling tryRecoverParent/flow.RunFullSequence
- [x] T010 [US1] In RestartDependents, before Restart(dependent) check if dependent ID is in unrestartable set and skip that dependent with log; on Restart failure classify and if unrestartable add dependent ID to set and log in internal/recovery/restart.go

**Checkpoint**: User Story 1 is complete; repeated failed recovery for the same container ID is bounded. FR-006 (continue monitoring others) is satisfied by skip-and-continue in T006 and T010; no separate task.

---

## Phase 4: User Story 2 - Recovery Succeeds When Container Can Be Brought Back (Priority: P2)

**Goal**: After backing off, when the same logical service appears with a new container ID (e.g. recreated by updater), the monitor resumes normal recovery for that new ID; when only the parent is replaced by an updater (new parent ID, healthy), the monitor proactively restarts that parent's dependents so the child comes back online (FR-007, SC-005, issue #5).

**Independent Test**: (1) Cause unrestartable failure and confirm skip; recreate the container (new ID); on next discovery/poll confirm the monitor runs recovery for the new ID. (2) Let an updater replace only the parent; confirm within one poll or next discovery the monitor proactively restarts dependents and the child comes back online.

### Implementation for User Story 2

- [x] T011 [US2] Implement set cap: when adding would exceed max size (e.g. 100), remove oldest or an entry not in the current container list in internal/recovery/unrestartable.go; document in type/method docstrings
- [x] T012 [US2] Add pruning: when a fresh container list is available, remove from unrestartable set any ID not in that list; add Prune method (or equivalent) with docstring in internal/recovery/unrestartable.go and call from main after ListContainers in cmd/watch-dog/main.go
- [x] T020 [US2] Add last-known parent ID map (parent name → container ID) in main.go; after each discovery (event and polling) for each parent if current ID != last-known and last-known was set and parent is healthy (Inspect), call flow.RestartDependents for that parent and log proactive restart (parent has new ID, restarting dependents), then set last-known to current ID; on first discovery only populate last-known without restarting per research §7 and contracts in cmd/watch-dog/main.go
- [x] T021 [US2] Ensure RestartDependents can be invoked without prior RestartParent (for proactive case) and document in internal/recovery/restart.go; same DependentRestartCooldown applies per FR-007

**Checkpoint**: New instances (new ID) are not blocked; set stays bounded; when only parent is replaced, dependents are proactively restarted (SC-005).

---

## Phase 5: User Story 3 - Clear Operator Visibility When Recovery Cannot Succeed (Priority: P3)

**Goal**: Operators see why recovery failed and that retries are limited; logs support troubleshooting (FR-005, SC-002).

**Independent Test**: Trigger unrestartable failure; confirm logs show recovery attempted, identifiable reason (container gone / marked for removal / dependency missing), and that monitor is not retrying indefinitely (skip or back-off message).

### Implementation for User Story 3

- [x] T013 [US3] Ensure first unrestartable failure log includes structured attributes: parent or dependent name, container id_short, reason (container_gone / marked_for_removal / dependency_missing) in internal/recovery/restart.go and internal/docker/log.go if needed
- [x] T014 [US3] Ensure skip log includes parent/dependent name and reason (e.g. "recovery skipped, container unrestartable") at INFO or DEBUG in internal/recovery/restart.go

**Checkpoint**: Operators can distinguish "recovery failed, retries limited" from "recovery in progress."

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Validation, docs, docstrings for new APIs, and a way to manually simulate failure states so the system can be verified to recover correctly.

- [x] T015 Run quickstart.md verification steps (bounded retries, recovery after recreate, other containers still monitored) and fix any gaps
- [x] T016 [P] Update README or operator docs if recovery behavior (unrestartable, no config) should be mentioned per specs/005-fix-recovery-stale-container/quickstart.md
- [x] T017 Add documentation with step-by-step instructions to manually simulate each unrestartable state (no such container, marked for removal, dependency missing) and how to verify watch-dog bounded retries and recovery after recreate in specs/005-fix-recovery-stale-container/simulate-failures.md
- [x] T018 [P] Add an optional script at scripts/simulate-unrestartable.sh that prints or runs Docker commands to simulate each failure state (no such container, marked for removal, dependency missing) for use during verification; optionally add a Makefile target at repo root that invokes the script
- [x] T019 Add or update Go docstrings for all new or modified exported symbols in internal/recovery (errors.go, unrestartable.go, restart.go): packages, types, and functions/methods per Go conventions; ensure no new public API is missing documentation

---

## Phase 7: User Story 4 - Auto-recreate when container gone (Priority: P2)

**Goal**: When the monitor marks a **parent** as unrestartable with reason **container_gone** (no such container), optionally trigger recreation of that service via `docker compose up -d <service_name>` so the operator does not have to run compose by hand. Recovery then proceeds when discovery sees the new container ID.

**Context**: Per discussion—currently the monitor stops retrying the gone container ID and only recovers when something external (operator or updater) recreates the service. This story adds an opt-in behavior: when a parent is container_gone, the monitor can run compose up for that service name so the service comes back without manual intervention.

**Independent Test**: (1) Enable WATCHDOG_AUTO_RECREATE. (2) Remove a monitored parent (e.g. `docker rm -f vpn`). (3) Trigger recovery (event or poll). (4) Verify one failure log (container_gone), then a log that auto-recreate was triggered, then on next discovery the new container is seen and recovery runs for the new ID (or the new container is already healthy).

### Implementation for User Story 4

- [ ] T022 [US4] Add optional callback `OnParentContainerGone func(parentName string)` to `recovery.Flow` in internal/recovery/restart.go; when adding parent ID to unrestartable set with reason **container_gone** (in RunFullSequence after Restart failure), invoke the callback with parentName if non-nil; document in Flow struct and method docstrings
- [ ] T023 [US4] In cmd/watch-dog/main.go read optional env `WATCHDOG_AUTO_RECREATE` (e.g. `true`/`1` to enable); when enabled, set `Flow.OnParentContainerGone` to a function that runs `docker compose -f <composePath> up -d <parentName>` using `discovery.ComposePathFromEnv()` for compose path, and `exec.Command` (or equivalent) with working directory set appropriately (e.g. compose file directory or current dir); skip if compose path is empty
- [ ] T024 [US4] Log at INFO when auto-recreate is triggered: parent name, compose path, and that the monitor will re-discover on next cycle in cmd/watch-dog/main.go (or where the callback is implemented)
- [ ] T025 [US4] Ensure auto-recreate runs only for **parent** container_gone (not for dependents, and not for marked_for_removal or dependency_missing); callback is invoked only from the parent-restart-failure path with reason container_gone in internal/recovery/restart.go
- [ ] T026 [US4] Update specs/005-fix-recovery-stale-container/contracts/recovery-unrestartable-behavior.md (or add a short section) describing optional auto-recreate when container_gone and WATCHDOG_AUTO_RECREATE is set; update quickstart.md and README Configuration table with WATCHDOG_AUTO_RECREATE

**Checkpoint**: With WATCHDOG_AUTO_RECREATE enabled, removing a parent triggers one failure log, then compose up for that service, then recovery of the new container on next discovery.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies.
- **Phase 2 (Foundational)**: Depends on Phase 1. Blocks all user stories.
- **Phase 3 (US1)**: Depends on Phase 2. Delivers MVP (bounded retries).
- **Phase 4 (US2)**: Depends on Phase 3. Ensures new IDs are not blocked, set is bounded, and proactive dependent restart when parent has new ID (FR-007).
- **Phase 5 (US3)**: Depends on Phase 3 (logging touches same code); can overlap with Phase 4.
- **Phase 6 (Polish)**: Depends on Phase 3–5. Includes docstrings (T019), manual simulation guide (T017), and optional script (T018) so failure states can be reproduced and recovery verified.
- **Phase 7 (US4)**: Depends on Phase 3 (unrestartable set and container_gone path). Adds opt-in auto-recreate when parent is container_gone.

### User Story Dependencies

- **US1 (P1)**: After Foundational only. No dependency on US2/US3.
- **US2 (P2)**: Builds on US1 (set cap/prune; re-discovery already uses new IDs from US1 design).
- **US3 (P3)**: Builds on US1 (same logging points); can be done with US1 or after.
- **US4 (P2)**: Builds on US1 (needs unrestartable add with reason container_gone and parent name in recovery path).

### Within Each User Story

- US1: T006–T010 are sequential (skip check → parent failure → inspect failure → nameToID wiring → dependent skip/failure). T005 can run in parallel with T002–T004.
- US2: T011 then T012 (cap then prune); T020 then T021 (last-known map + proactive restart loop in main; RestartDependents doc/behavior). T020–T021 can follow T012.
- US3: T013 and T014 can be parallel (different log sites).
- US4: T022 first (callback in Flow); then T023–T024 (main callback impl + log); T025 is a guard in restart.go; T026 can run in parallel (docs).

### Parallel Opportunities

- T002 and T003 can run in parallel (errors.go vs set type).
- T005 (tests) can run in parallel with T006–T010 once T002 is done.
- T013 and T014 can run in parallel.
- T016, T017, T018, T019 can run in parallel with T015 (docs, simulation assets, docstrings).
- T026 can run in parallel with T023–T025 (docs vs code).

---

## Parallel Example: User Story 1

```bash
# After T002–T004 done:
# Option A: Implement then test
T006 → T007 → T008 → T009 → T010
T005 (in parallel with T006–T010 if desired)

# Option B: Test first (optional)
T005 (unit test for classifier) → then T006–T010
```

---

## Parallel Example: User Story 2

```bash
# Set cap and prune first, then proactive restart:
T011 → T012 → T020 → T021
# T020 (last-known map + proactive loop in main) and T021 (RestartDependents doc) can overlap if T021 is doc-only.
```

---

## Parallel Example: User Story 4

```bash
# Callback in Flow first, then main wiring and docs:
T022 → T023 → T024 → T025
T026 (docs) in parallel with T023–T025
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (T001).
2. Complete Phase 2 (T002–T004).
3. Complete Phase 3 (T005 optional, T006–T010).
4. **Stop and validate**: Run quickstart “Verify bounded retries”; confirm no endless loop.
5. Deploy/demo if ready.

### Incremental Delivery

1. Phase 1 + 2 → foundation ready.
2. Phase 3 (US1) → validate bounded retries (MVP).
3. Phase 4 (US2) → validate recovery after recreate, set bound, and proactive restart when only parent is replaced (quickstart "Verify proactive restart when only parent is replaced").
4. Phase 5 (US3) → validate operator visibility.
5. Phase 6 → quickstart, docs, and manual simulation guide so failure states can be reproduced and recovery verified.
6. Phase 7 (US4) → optional auto-recreate when parent is container_gone (WATCHDOG_AUTO_RECREATE).

### Task Count Summary

| Phase            | Task IDs   | Count |
|------------------|------------|-------|
| Phase 1 Setup    | T001       | 1     |
| Phase 2 Foundational | T002–T004 | 3  |
| Phase 3 US1      | T005–T010  | 6     |
| Phase 4 US2      | T011–T012, T020–T021 | 4     |
| Phase 5 US3      | T013–T014  | 2     |
| Phase 6 Polish   | T015–T019  | 5     |
| Phase 7 US4      | T022–T026  | 5     |
| **Total**        |            | **26**|

- **Per user story**: US1: 6 tasks (1 optional test). US2: 4 (set cap/prune + proactive restart per FR-007). US3: 2. Polish: 5. US4: 5 (auto-recreate when container_gone).
- **Suggested MVP scope**: Phase 1 + Phase 2 + Phase 3 (User Story 1) = T001–T010.
- **Manual simulation**: Use T017 (simulate-failures.md) and optionally T018 (script at scripts/simulate-unrestartable.sh, optionally Makefile target) to reproduce no-such-container, marked-for-removal, and dependency-missing states and verify the system properly recovers (bounded retries, skip logs, recovery after recreate).

---

## Notes

- All tasks use the required format: `- [ ] Txxx [P?] [US?] Description with file path`.
- [P] used where tasks touch different files and have no ordering requirement.
- [US1]/[US2]/[US3] used for Phase 3–5 implementation tasks only.
- **Manual simulation (T017–T018)**: Provides a repeatable way to simulate the failure states (no such container, marked for removal, dependency missing) so operators and developers can verify that the system properly recovers (bounded retries, skip logs, recovery after recreate) without relying on an external updater.
- **Docstrings (T019)**: All new or modified exported symbols in internal/recovery (errors.go, unrestartable.go, restart.go) must have Go docstrings; T002, T003, T004, T011, T012 call out docstrings for their respective files; T019 is a final pass to ensure nothing is missing.
- **Proactive restart (T020–T021)**: Implements FR-007 / SC-005; when parent has new ID and is healthy (e.g. after updater replace), monitor proactively restarts that parent's dependents so the child comes back online (GitHub issue #5). Same cooldown as normal recovery.
- **Auto-recreate (T022–T026, US4)**: When a parent is marked container_gone and WATCHDOG_AUTO_RECREATE is enabled, the monitor runs `docker compose up -d <parentName>` so the service is recreated without the operator running compose by hand; recovery then runs for the new container on next discovery.
- Commit after each task or logical group; do not run git commit from the agent.
