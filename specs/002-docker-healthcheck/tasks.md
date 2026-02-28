---

description: "Task list for 002-docker-healthcheck feature implementation"
---

# Tasks: Add Healthcheck and Observability to Watch-Dog Container

**Input**: Design documents from `specs/002-docker-healthcheck/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Plan requests extending existing tests for log level/format and healthcheck behavior; optional test task included in Polish phase.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story (US1, US2)
- Include exact file paths in descriptions

## Path Conventions

- **Project**: `cmd/watch-dog/`, `internal/docker/`, repository root for Dockerfile and README.md

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Verify project and feature artifacts are in place before implementation.

- [x] T001 Verify project structure and feature specs: ensure specs/002-docker-healthcheck/ contains plan.md, spec.md, data-model.md, research.md, quickstart.md and contracts/ (healthcheck-behavior.md, env-logging-healthcheck.md)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Runtime image must have Docker CLI so the healthcheck command can run; no user story work depends on anything else shared.

**⚠️ CRITICAL**: US1 requires Docker CLI in the image before HEALTHCHECK can be added.

- [x] T002 Add Docker CLI to the runtime image in Dockerfile (e.g. `apk add docker-cli` in the runtime stage so `docker info` is available for HEALTHCHECK)

**Checkpoint**: Foundation ready – US1 and US2 implementation can proceed (US1 after T002; US2 can start after Phase 1)

---

## Phase 3: User Story 1 – Container Reports Health Status (Priority: P1) 🎯 MVP

**Goal**: Container exposes health status (healthy/starting/unhealthy) via `docker ps` or compose; HEALTHCHECK runs a minimal Docker API check; parameters configurable via .env in Compose.

**Independent Test**: Run the watch-dog image with the healthcheck; run `docker ps` and confirm health column shows healthy/starting/unhealthy. Stop the watch-dog process inside the container and confirm the container becomes unhealthy after configured retries/timeout.

### Implementation for User Story 1

- [x] T003 [P] [US1] Add HEALTHCHECK instruction to Dockerfile with defaults `--interval=15s --start-period=20s --timeout=10s --retries=2` and test command `docker info` as the HEALTHCHECK CMD (e.g. `HEALTHCHECK ... CMD docker info`)
- [x] T004 [P] [US1] Document healthcheck in README.md: new section describing how health is determined, .env variable names (DOCKER_HEALTHCHECK_INTERVAL, DOCKER_HEALTHCHECK_RETRIES, DOCKER_HEALTHCHECK_START_PERIOD, DOCKER_HEALTHCHECK_TIMEOUT) with reference values (15s, 2, 20s, 10s), transient failure and timeout behavior (e.g. socket unavailable, heavy load) for operator troubleshooting, and a compose example using variable substitution (no hardcoded values in the example)

**Checkpoint**: User Story 1 is done; container reports health status and README explains healthcheck and .env overrides.

---

## Phase 4: User Story 2 – Readable Log Output (Priority: P2)

**Goal**: Log lines are human-readable; LOG_LEVEL and LOG_FORMAT configurable via env; README has an example for every LOG_LEVEL and LOG_FORMAT value.

**Independent Test**: Run the container and trigger events (startup, parent recovery); view logs and confirm format. Set LOG_LEVEL (e.g. WARN) and LOG_FORMAT (e.g. compact) and confirm output matches.

### Implementation for User Story 2

- [x] T005 [US2] Implement LOG_LEVEL and LOG_FORMAT in internal/docker/log.go: read LOG_LEVEL (DEBUG, INFO, WARN, ERROR; default INFO) and LOG_FORMAT (compact, timestamp, json; default timestamp); map to slog level and custom handlers (compact: `[Level] message`; timestamp: RFC3339 + `[Level] message`; json: slog JSONHandler); set slog default handler at startup (case-insensitive; invalid/unset use defaults)
- [x] T006 [US2] Wire logging at startup in cmd/watch-dog/main.go: call docker logging init (e.g. InitLogging or equivalent) before creating Docker client so LOG_LEVEL and LOG_FORMAT from env are applied from first log line
- [x] T007 [P] [US2] Document LOG_LEVEL and LOG_FORMAT in README.md: table of LOG_LEVEL values (DEBUG, INFO, WARN, ERROR) with short description or example snippet for each; table of LOG_FORMAT values (compact, timestamp, json) with example log line for each; include compose/env snippet showing LOG_LEVEL and LOG_FORMAT (per FR-006, FR-007, SC-007)

**Checkpoint**: User Story 2 is done; logs are readable and configurable; README has examples for every level and format.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Validation and optional test extensions. If the repo has no `*_test.go` files yet, T009 may be deferred until a test package is introduced.

- [x] T008 [P] Run quickstart.md validation: follow specs/002-docker-healthcheck/quickstart.md to verify healthcheck and LOG_LEVEL/LOG_FORMAT behavior (manual or script)
- [x] T009 [P] Extend existing tests for log level/format and healthcheck behavior when test packages exist (e.g. internal/docker or cmd/watch-dog); if no `*_test.go` files exist yet, defer until tests are added, per plan.md testing goal

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies – can start immediately
- **Foundational (Phase 2)**: Depends on Setup – T002 blocks US1 (HEALTHCHECK needs docker CLI)
- **User Story 1 (Phase 3)**: Depends on Phase 2 (T002)
- **User Story 2 (Phase 4)**: Depends on Phase 1 only – no dependency on US1
- **Polish (Phase 5)**: Depends on completion of Phase 3 and Phase 4

### User Story Dependencies

- **User Story 1 (P1)**: After T002 – no dependency on US2
- **User Story 2 (P2)**: After Phase 1 – can be implemented in parallel with US1 once T002 is done

### Within Each User Story

- US1: T003 (Dockerfile) and T004 (README) can run in parallel
- US2: T005 (log.go) then T006 (main.go); T007 (README) can run in parallel with T005/T006 if different owner

### Parallel Opportunities

- After T002: T003 and T004 can run in parallel (Dockerfile vs README); T005 can run in parallel with T003/T004 (different files)
- T007 (README logging section) can run in parallel with T005/T006
- T008 and T009 (Polish) can run in parallel

---

## Parallel Example: User Story 1

```bash
# After T002, launch US1 tasks in parallel:
Task T003: "Add HEALTHCHECK instruction to Dockerfile ..."
Task T004: "Document healthcheck in README.md ..."
```

## Parallel Example: User Story 2

```bash
# T005 must complete before T006 (main wires logging). T007 can run in parallel with T005/T006:
Task T005: "Implement LOG_LEVEL and LOG_FORMAT in internal/docker/log.go ..."
Task T007: "Document LOG_LEVEL and LOG_FORMAT in README.md ..."
# Then:
Task T006: "Wire logging at startup in cmd/watch-dog/main.go ..."
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (T002 – Docker CLI in image)
3. Complete Phase 3: User Story 1 (HEALTHCHECK + README)
4. **STOP and VALIDATE**: Run container, `docker ps` for health status; stop process and confirm unhealthy
5. Deploy or demo

### Incremental Delivery

1. Setup + Foundational → image has Docker CLI
2. Add User Story 1 → healthcheck and docs → validate → MVP
3. Add User Story 2 → configurable logging and README examples → validate
4. Polish → quickstart validation and optional test extensions

### Parallel Team Strategy

- After T002: One developer can do US1 (T003, T004) while another does US2 (T005, T006, T007)
- README work (T004, T007) can be split or done by whoever owns the feature

---

## Notes

- [P] = different files or no dependency on incomplete work
- [US1]/[US2] map tasks to user stories for traceability
- Each user story is independently testable per spec.md
- Commit after each task or logical group
- Format validation: All tasks use checklist format `- [ ] [ID] [P?] [Story?] Description` with file paths where applicable
