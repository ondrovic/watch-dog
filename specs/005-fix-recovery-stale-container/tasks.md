# Tasks: Fix Recovery When Containers Are Gone or Unrestartable

**Input**: Design documents from `/specs/005-fix-recovery-stale-container/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Optional; plan requests extending recovery tests for error classification and skip behavior. One unit test phase for the classifier is included.

**Organization**: Tasks are grouped by user story so each story can be implemented and validated independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story (e.g. US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

- Repository layout per plan: `cmd/watch-dog/main.go`, `internal/docker/`, `internal/discovery/`, `internal/recovery/`.
- Recovery package (this feature): `internal/recovery/restart.go` (Flow, RunFullSequence, RestartDependents), `internal/recovery/errors.go` (error classification), `internal/recovery/unrestartable.go` (unrestartable set type).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm project structure and add dependencies required for US4 (Compose SDK).

- [x] T001 Verify project structure per plan (cmd/watch-dog, internal/docker, internal/discovery, internal/recovery)
- [ ] T002 [P] Add Docker Compose Go SDK and docker/cli to go.mod: add github.com/docker/compose/v2 and github.com/docker/cli; run go mod tidy; pin github.com/docker/cli to v28.5.2+incompatible if needed (e.g. when using Compose v5) for compatibility with existing github.com/docker/docker per plan.md

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Error classification and unrestartable set type that all user stories depend on.

**Critical**: No user story work can begin until this phase is complete.

- [x] T003 [P] Add function to classify Docker API errors as unrestartable in internal/recovery/errors.go (No such container, marked for removal, joining network namespace + No such container) per research.md and contracts/recovery-unrestartable-behavior.md; include package and exported function docstrings (e.g. IsUnrestartableError) per Go conventions
- [x] T004 Add in-memory unrestartable set type (bounded size, e.g. max 100 IDs) with thread-safe add/contains and optional prune in internal/recovery/unrestartable.go per data-model.md; include package and exported type/method docstrings per Go conventions
- [x] T005 Add unrestartable set field to recovery.Flow and wire check-before-run (skip when ID in set) and add-on-failure in internal/recovery/restart.go; add or update Flow and method docstrings for new behavior

**Checkpoint**: Foundation ready; US1 implementation can begin.

---

## Phase 3: User Story 1 - No Repeated Failed Recovery Attempts (Priority: P1) — MVP

**Goal**: When restart or inspect fails with an unrestartable error, the monitor records that container ID and does not retry the same ID on subsequent triggers; retries are bounded.

**Independent Test**: Remove or mark-for-removal a monitored parent; run the monitor; confirm at most one (or a small bounded number of) recovery failure log lines for that container, then skip messages instead of repeated identical errors.

### Optional: Tests for User Story 1

- [x] T006 [P] [US1] Add unit tests for unrestartable error classification (no such container, marked for removal, dependency missing) in internal/recovery/errors_test.go

### Implementation for User Story 1

- [x] T007 [US1] At start of RunFullSequence, if parentID is in the unrestartable set then skip sequence and log recovery skipped for parent (INFO or DEBUG) in internal/recovery/restart.go
- [x] T008 [US1] On Restart(parentID) failure in RunFullSequence, if error is unrestartable then add parentID to set and log failure with reason (container gone / marked for removal / dependency missing); return without wait or dependents in internal/recovery/restart.go
- [x] T009 [US1] In WaitUntilHealthy, on Inspect(parentID) error classify as unrestartable; if so add parentID to set, log, and return false in internal/recovery/restart.go
- [x] T010 [US1] Pass nameToID (or equivalent) into RunFullSequence and RestartDependents so dependent container ID is available when restarting dependents; in main.go ensure nameToID from ListContainers is passed when calling tryRecoverParent/flow.RunFullSequence
- [x] T011 [US1] In RestartDependents, before Restart(dependent) check if dependent ID is in unrestartable set and skip that dependent with log; on Restart failure classify and if unrestartable add dependent ID to set and log in internal/recovery/restart.go

**Checkpoint**: User Story 1 is complete; repeated failed recovery for the same container ID is bounded. FR-006 (continue monitoring others) is satisfied by skip-and-continue; no separate task.

---

## Phase 4: User Story 2 - Recovery Succeeds When Container Can Be Brought Back (Priority: P2)

**Goal**: After backing off, when the same logical service appears with a new container ID (e.g. recreated by updater), the monitor resumes normal recovery for that new ID; when only the parent is replaced by an updater (new parent ID, healthy), the monitor proactively restarts that parent's dependents so the child comes back online (FR-007, SC-005, issue #5).

**Independent Test**: (1) Cause unrestartable failure and confirm skip; recreate the container (new ID); on next discovery/poll confirm the monitor runs recovery for the new ID. (2) Let an updater replace only the parent; confirm within one poll or next discovery the monitor proactively restarts dependents and the child comes back online.

### Implementation for User Story 2

- [x] T012 [US2] Implement set cap: when adding would exceed max size (e.g. 100), remove oldest or an entry not in the current container list in internal/recovery/unrestartable.go; document in type/method docstrings
- [x] T013 [US2] Add pruning: when a fresh container list is available, remove from unrestartable set any ID not in that list; add Prune method (or equivalent) with docstring in internal/recovery/unrestartable.go and call from main after ListContainers in cmd/watch-dog/main.go
- [x] T014 [US2] Add last-known parent ID map (parent name → container ID) in main.go; after each discovery (event and polling) for each parent if current ID != last-known and last-known was set and parent is healthy (Inspect), call flow.RestartDependents for that parent and log proactive restart (parent has new ID, restarting dependents), then set last-known to current ID; on first discovery only populate last-known without restarting per research §7 and contracts in cmd/watch-dog/main.go
- [x] T015 [US2] Ensure RestartDependents can be invoked without prior RestartParent (for proactive case) and document in internal/recovery/restart.go; same DependentRestartCooldown applies per FR-007

**Checkpoint**: New instances (new ID) are not blocked; set stays bounded; when only parent is replaced, dependents are proactively restarted (SC-005).

---

## Phase 5: User Story 3 - Clear Operator Visibility When Recovery Cannot Succeed (Priority: P3)

**Goal**: Operators see why recovery failed and that retries are limited; logs support troubleshooting (FR-005, SC-002).

**Independent Test**: Trigger unrestartable failure; confirm logs show recovery attempted, identifiable reason (container gone / marked for removal / dependency missing), and that monitor is not retrying indefinitely (skip or back-off message).

### Implementation for User Story 3

- [x] T016 [US3] Ensure first unrestartable failure log includes structured attributes: parent or dependent name, container id_short, reason (container_gone / marked_for_removal / dependency_missing) in internal/recovery/restart.go and internal/docker/log.go if needed
- [x] T017 [US3] Ensure skip log includes parent/dependent name and reason (e.g. "recovery skipped, container unrestartable") at INFO or DEBUG in internal/recovery/restart.go

**Checkpoint**: Operators can distinguish "recovery failed, retries limited" from "recovery in progress."

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Validation, docs, docstrings for new APIs, and a way to manually simulate failure states so the system can be verified to recover correctly.

- [x] T018 Run quickstart.md verification steps (bounded retries, recovery after recreate, other containers still monitored) and fix any gaps
- [x] T019 [P] Update README or operator docs if recovery behavior (unrestartable, no config) should be mentioned per specs/005-fix-recovery-stale-container/quickstart.md
- [x] T020 Add documentation with step-by-step instructions to manually simulate each unrestartable state (no such container, marked for removal, dependency missing) and how to verify watch-dog bounded retries and recovery after recreate in specs/005-fix-recovery-stale-container/simulate-failures.md
- [x] T021 [P] Add an optional script at scripts/simulate-unrestartable.sh that prints or runs Docker commands to simulate each failure state (no such container, marked for removal, dependency missing) for use during verification; optionally add a Makefile target at repo root that invokes the script
- [x] T022 Add or update Go docstrings for all new or modified exported symbols in internal/recovery (errors.go, unrestartable.go, restart.go): packages, types, and functions/methods per Go conventions; ensure no new public API is missing documentation

---

## Phase 7: User Story 4 - Auto-recreate when parent is gone or marked for removal (Priority: P2, optional)

**Goal**: When the monitor marks a **parent** as unrestartable with reason **container_gone** or **marked_for_removal**, optionally trigger recreation of that service via the **Docker Compose Go SDK** (equivalent of `docker compose up -d <service_name>`) so the operator does not have to run compose by hand. No `docker` or `docker-compose` binary is required. Recovery then proceeds when discovery sees the new container ID.

**Independent Test**: (1) Enable WATCHDOG_AUTO_RECREATE. (2) Remove a monitored parent (e.g. `docker rm -f <container_name>`) or cause marked_for_removal. (3) Trigger recovery (event or poll). (4) Verify one failure log (container_gone or marked_for_removal), then a log that auto-recreate was triggered, then on next discovery the new container is seen and recovery runs for the new ID (or the new container is already healthy).

### Implementation for User Story 4

- [x] T023 [US4] Add optional callback `OnParentContainerGone func(parentName string)` to `recovery.Flow` in internal/recovery/restart.go; when adding parent ID to unrestartable set with reason **container_gone** or **marked_for_removal**, invoke the callback with parentName if non-nil; document in Flow struct and method docstrings
- [ ] T024 [US4] Replace exec-based auto-recreate with Docker Compose Go SDK in cmd/watch-dog/main.go: when autoRecreate && composePath != "" create Compose API service at startup via compose.NewComposeService(dockerCLI) with dockerCLI from command.NewDockerCli() and Initialize(flags.ClientOptions{}); on init failure log warning and disable auto-recreate; in OnParentContainerGone resolve parent container name to service name via discovery.ContainerNameToServiceName(composePath); call SDK LoadProject(ctx, api.ProjectLoadOptions{ConfigPaths: []string{composePath}, WorkingDir: filepath.Dir(composePath), ProjectName: os.Getenv("COMPOSE_PROJECT_NAME")}); call SDK Up(ctx, project, api.UpOptions{Create: api.CreateOptions{Services: []string{serviceName}, Recreate: "force"}, Start: api.StartOptions{Services: []string{serviceName}}}); run in goroutine with existing timeout; log INFO on trigger/success, ERROR on failure; remove all exec-based docker compose / docker-compose invocation and fallback
- [x] T025 [US4] Log at INFO when auto-recreate is triggered: parent name, service name (if resolved), compose path, and that the monitor will re-discover on next cycle in cmd/watch-dog/main.go
- [x] T026 [US4] Ensure auto-recreate runs only for **parent** and only for **container_gone** or **marked_for_removal** (not for dependents, not for dependency_missing); callback is invoked only from the parent-restart-failure path with those reasons in internal/recovery/restart.go
- [x] T027 [US4] Verify specs/005-fix-recovery-stale-container/contracts/recovery-unrestartable-behavior.md and quickstart.md describe SDK-based auto-recreate (no docker or docker-compose binary required); update README Configuration table with WATCHDOG_AUTO_RECREATE if not already present

**Checkpoint**: With WATCHDOG_AUTO_RECREATE enabled, removing a parent or marked_for_removal triggers one failure log, then Compose SDK Up for that service, then recovery of the new container on next discovery.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies. T002 (Compose SDK deps) required for Phase 7 only.
- **Phase 2 (Foundational)**: Depends on Phase 1. Blocks all user stories.
- **Phase 3 (US1)**: Depends on Phase 2. Delivers MVP (bounded retries).
- **Phase 4 (US2)**: Depends on Phase 3. New IDs not blocked, set bounded, proactive dependent restart (FR-007).
- **Phase 5 (US3)**: Depends on Phase 3 (logging touches same code); can overlap with Phase 4.
- **Phase 6 (Polish)**: Depends on Phase 3–5.
- **Phase 7 (US4)**: Depends on Phase 3 (unrestartable set and callback path) and Phase 1 T002 (Compose SDK deps).

### User Story Dependencies

- **US1 (P1)**: After Foundational only. No dependency on US2/US3.
- **US2 (P2)**: Builds on US1 (set cap/prune; re-discovery already uses new IDs).
- **US3 (P3)**: Builds on US1 (same logging points); can be done with US1 or after.
- **US4 (P2)**: Builds on US1 (callback and reasons container_gone/marked_for_removal) and Phase 1 T002 (Compose SDK).

### Parallel Opportunities

- T003 and T004 can run in parallel (errors.go vs unrestartable.go).
- T006 (tests) can run in parallel with T007–T011 once T003 is done.
- T016 and T017 can run in parallel.
- T019, T020, T021, T022 can run in parallel with T018.
- T027 can run in parallel with T024 (docs vs code).

---

## Parallel Example: User Story 1

```bash
# After T003–T005 done:
T006 (unit test) → then T007 → T008 → T009 → T010 → T011
# Or T006 in parallel with T007–T011
```

---

## Parallel Example: User Story 4

```bash
# After T002 (deps) and T023 (callback) done:
T024 (Replace exec with Compose SDK in cmd/watch-dog/main.go)
T027 (Verify contract/quickstart) in parallel with T024
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (T001; T002 optional until US4).
2. Complete Phase 2 (T003–T005).
3. Complete Phase 3 (T006 optional, T007–T011).
4. **Stop and validate**: Run quickstart "Verify bounded retries"; confirm no endless loop.
5. Deploy/demo if ready.

### Incremental Delivery

1. Phase 1 + 2 → foundation ready.
2. Phase 3 (US1) → validate bounded retries (MVP).
3. Phase 4 (US2) → validate recovery after recreate, set bound, proactive restart.
4. Phase 5 (US3) → validate operator visibility.
5. Phase 6 → quickstart, docs, simulation guide.
6. Phase 7 (US4) → optional auto-recreate via Compose SDK (T002 + T024–T027).

### Task Count Summary

| Phase                  | Task IDs        | Count |
|------------------------|-----------------|-------|
| Phase 1 Setup          | T001–T002       | 2     |
| Phase 2 Foundational   | T003–T005       | 3     |
| Phase 3 US1            | T006–T011       | 6     |
| Phase 4 US2            | T012–T015       | 4     |
| Phase 5 US3            | T016–T017       | 2     |
| Phase 6 Polish         | T018–T022       | 5     |
| Phase 7 US4            | T023–T027       | 5     |
| **Total**              |                 | **27**|

- **Per user story**: US1: 6 tasks (1 optional test). US2: 4. US3: 2. US4: 5 (callback + SDK implementation + logging + scope + contract/quickstart).
- **Suggested MVP scope**: Phase 1 + Phase 2 + Phase 3 (User Story 1) = T001, T003–T011.
- **Remaining work**: T002 (Add Compose SDK deps), T024 (Replace exec-based auto-recreate with Compose SDK in cmd/watch-dog/main.go).

---

## Notes

- All tasks use the required format: `- [ ] Txxx [P?] [US?] Description with file path`.
- [P] used where tasks touch different files and have no ordering requirement.
- **Auto-recreate (US4)**: Contract and quickstart already state SDK-based auto-recreate (no docker/docker-compose binary). T024 is the implementation task: replace exec in main.go with Compose SDK init + LoadProject + Up in OnParentContainerGone.
- Commit after each task or logical group; do not run git commit from the agent.
