---
description: "Task list for Container Health Monitor (Watch-Dog) implementation"
---

# Tasks: Container Health Monitor (Watch-Dog)

**Input**: Design documents from `/specs/001-container-health-monitor/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Not explicitly requested in the feature specification; no test tasks included. Add contract or unit tests in a later pass if desired.

**Organization**: Tasks are grouped by user story so each story can be implemented and validated independently.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US4)
- Include exact file paths in descriptions

## Path Conventions

- Single Go module at repository root (per plan.md)
- `cmd/watch-dog/`, `internal/docker/`, `internal/discovery/`, `internal/recovery/`, `tests/`, `.github/workflows/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and directory structure per plan.md

- [x] T001 Create project structure: cmd/watch-dog/, internal/docker/, internal/discovery/, internal/recovery/, tests/contract/, tests/unit/
- [x] T002 Initialize Go module and add Docker client dependency (go.mod at repo root): `go mod init` and `go get github.com/docker/docker/client`
- [x] T003 [P] Add a proper .gitignore at repository root configured for this project: Go (vendor/, *.exe, watch-dog binary, *.test, coverage.out, go.work), IDE (.idea/, .vscode/, *.swp, *.swo), OS (.DS_Store, Thumbs.db); ensure paths and binary name align with plan.md (single binary watch-dog)
- [x] T004 [P] Add a proper LICENSE file at repository root: choose a license appropriate for the project (e.g. MIT, Apache-2.0); include copyright year and copyright holder; use standard license text from the license’s official source (no modifications that affect legal meaning)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Docker client wrapper and event subscription that all user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T005 Implement Docker client wrapper in internal/docker/client.go: NewClient (from env DOCKER_HOST), ListContainers (running, with labels), Inspect (health status, labels), Restart(containerID)
- [x] T006 [P] Add structured logging to stdout (e.g. key=value or JSON) in a small internal package or internal/docker/client.go for Docker operations and errors
- [x] T007 Implement health_status event subscription in internal/docker/events.go: subscribe to Docker events with filter event=health_status, expose channel or callback for unhealthy container name/ID

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 3 - Dynamic Parent/Child Discovery via Labels (Priority: P1)

**Goal**: Discover parent/child relationships only from container labels (depends_on); no static config. Required so US1/US2 know which containers are parents and which are dependents.

**Independent Test**: Add a container with depends_on label pointing to an existing container; run monitor; verify it discovers the relationship without any config change.

- [x] T008 [P] [US3] Implement depends_on label parsing in internal/discovery/labels.go: parse comma-separated value, trim spaces, return slice of parent names (per contracts/depends-on-label.md)
- [x] T009 [US3] Implement parent→dependents map builder in internal/discovery/labels.go: accept a Docker client interface from the caller (e.g. from internal/docker); list running containers with label depends_on, parse each label, build map parent name → list of dependent container names; ignore names that do not match any container on host
- [x] T010 [US3] Expose function in internal/discovery/labels.go to get dependent container names for a given parent name (used by recovery)

**Checkpoint**: Discovery is 100% dynamic from labels; US1 and US2 can use it to identify parents and dependents

---

## Phase 4: User Story 1 - Monitor and Restart Unhealthy Parent (Priority: P1) 🎯 MVP

**Goal**: Detect when a parent container becomes unhealthy and restart that parent. Delivers drop-in replacement for autoheal for single-parent setups.

**Independent Test**: Deploy monitor with one parent container that has a healthcheck; make parent unhealthy; verify monitor restarts the parent.

- [x] T011 [US1] Implement RestartParent in internal/recovery/restart.go: accept container name or ID, call Docker client Restart (idempotent)
- [x] T012 [US1] In cmd/watch-dog/main.go: on each health_status=unhealthy event from internal/docker/events.go, resolve container name and use discovery to check if it is a parent (appears in any depends_on); if not a parent, ignore; only invoke recovery for parents
- [x] T013 [US1] On unhealthy parent in cmd/watch-dog/main.go: call RestartParent in internal/recovery/restart.go; add logging for detected unhealthy and restart triggered
- [x] T014 [US1] Wire main loop in cmd/watch-dog/main.go: create Docker client, discovery, recovery; start event subscription; handle each unhealthy event (filter to parents, restart parent)

**Checkpoint**: User Story 1 complete — unhealthy parent is restarted; no dependents yet

---

## Phase 5: User Story 2 - Restart Dependents After Parent Is Healthy (Priority: P2)

**Goal**: After restarting a parent, wait until it is healthy then restart all containers that list that parent in depends_on. Enforces recovery order per contracts/recovery-behavior.md.

**Independent Test**: One parent, one or more children with depends_on label; make parent unhealthy; verify monitor restarts parent, waits for healthy, then restarts each dependent.

- [x] T015 [US2] Implement WaitUntilHealthy in internal/recovery/restart.go: after restart, poll container inspect for State.Health.Status until "healthy" or timeout (e.g. 5 minutes); if timeout, do not restart dependents (per spec)
- [x] T016 [US2] Implement RestartDependents in internal/recovery/restart.go: accept parent name, get dependent names from discovery, call Restart for each; log each restart
- [x] T017 [US2] Update recovery flow in internal/recovery/restart.go: after RestartParent call WaitUntilHealthy then RestartDependents; wire from main/event handler so full sequence runs on unhealthy parent

**Checkpoint**: User Stories 1 and 2 complete — parent then dependents recovery order enforced

---

## Phase 6: User Story 4 - Deliverable as Standalone Container Publishable to Registry (Priority: P2)

**Goal**: Single Docker image buildable and publishable to GitHub Container Registry so users can run the image without building from source.

**Independent Test**: Build image (e.g. docker build), run with Docker socket; verify same behavior as local binary. Push to GHCR via workflow; pull and run from registry.

- [x] T018 [US4] Add Dockerfile at repository root: multi-stage build (build Go binary, then minimal runtime image e.g. scratch or distroless); CMD runs watch-dog binary; no host mounts required at build time
- [x] T019 [US4] Add .github/workflows/build-push.yml: trigger on push to main (and optionally tags); checkout repo; login to ghcr.io; build and push image (e.g. ghcr.io/<owner>/watch-dog:latest); use docker/build-push-action with registry ghcr.io
- [x] T020 [US4] Update specs/001-container-health-monitor/quickstart.md with final image name (ghcr.io/<owner>/watch-dog), run example (docker run -v /var/run/docker.sock:/var/run/docker.sock), and compose snippet; do not duplicate project overview here (that is T024)
- [x] T033 [US4] Ensure multi-platform image in .github/workflows/build-push.yml: set `platforms: linux/amd64,linux/arm64` on docker/build-push-action so the image has manifests for Intel/AMD and Apple Silicon (arm64); avoids "no matching manifest for linux/arm64" when pulling on macOS Silicon

**Checkpoint**: Image builds and pushes to GHCR on repo push; users can pull and run on amd64 and arm64 (including macOS Silicon)

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Startup behavior, robustness, and documentation

- [x] T021 [P] Add startup reconciliation in cmd/watch-dog/main.go: on start, list containers and inspect health; for any container that is a parent (per discovery) and already unhealthy, run full recovery sequence (restart parent → wait healthy → restart dependents) per contracts/recovery-behavior.md
- [x] T022 [P] Add optional polling fallback in internal/docker/events.go: periodic poll (e.g. every 60s) of container health for discovered parents; if unhealthy and not already handling, trigger same recovery flow (covers missed events after monitor or daemon restart)
- [x] T023 Run quickstart.md validation: build image, run with socket, verify discovery and restart behavior against a small compose stack
- [x] T024 [P] Update README or docs at repo root with project description, link to spec and quickstart (quickstart content is in T020), and run instructions

---

## Phase 8: Refactor — Use root-level depends_on from Compose (US3)

**Purpose**: Replace label-based discovery with root-level `depends_on` from Docker Compose so the monitor works with native compose syntax (no custom labels).

**Goal**: Discovery reads compose file(s), parses `depends_on` in both supported formats, maps service names to running container names (via compose labels), and builds parent→dependents. Recovery behavior unchanged.

**Supported compose formats**:
- Short form: `depends_on: [container_name, container_name_2, ...]`
- Long form: `depends_on: { container_name: { condition: service_healthy, restart: true }, ... }`

**Independent Test**: Compose stack with root-level depends_on (short or long form); run monitor with COMPOSE_FILE or compose path set; make a parent unhealthy; verify monitor restarts parent then dependents.

- [x] T025 Add compose file path configuration: env COMPOSE_FILE or WATCHDOG_COMPOSE_PATH (default or empty = no compose mode); document in internal/discovery or cmd/watch-dog/main.go; add YAML dependency (e.g. gopkg.in/yaml.v3) to go.mod for parsing compose
- [x] T026 [P] [US3] Implement compose depends_on parsing in internal/discovery/compose.go: parse root-level `depends_on` supporting (1) short form list of service names, (2) long form map of service name to object with optional condition/restart; output per-service list of parent service names (who this service depends on)
- [x] T027 [US3] In internal/discovery/compose.go: map compose service names to running container names using Docker API—use labels `com.docker.compose.service` and `com.docker.compose.project` on containers to resolve service→container; build ParentToDependents (parent container name → dependent container names) from parsed depends_on; ignore services with no running container
- [x] T028 [US3] Refactor discovery in internal/discovery: when compose file path is set, use compose-based BuildParentToDependents (from T026–T027); remove or deprecate label-based discovery (labels depends_on) so root-level depends_on is the single source; keep ParentToDependents type and GetDependents/IsParent for recovery
- [x] T029 Update specs/001-container-health-monitor/contracts/depends-on-label.md (or add contracts/compose-depends-on.md) and quickstart.md: document root-level depends_on (short and long form), compose file path env, and that compose-based discovery replaces label-based

**Checkpoint**: Monitor uses only root-level depends_on from compose; works with both YAML formats and compose service→container mapping.

---

## Phase 9: README — Docker Compose, usage, debugging

**Purpose**: Expand the repository README so users can run watch-dog from Docker Compose (pulling from GitHub), understand how to use it, and debug issues.

**Goal**: README at repo root includes: using the image from GitHub Container Registry in docker-compose, project-relevant info, how to use (config, verification), and debugging/troubleshooting.

**Independent Test**: A reader can copy a compose snippet from the README to add watch-dog to their stack; follow "How to use" to configure and verify; use "Debugging" to diagnose common problems.

- [x] T030 [P] In README.md at repository root: add section "Using in Docker Compose" with example pulling the image from GitHub Container Registry (ghcr.io/<owner>/watch-dog:latest); include required Docker socket volume, WATCHDOG_COMPOSE_PATH (or COMPOSE_FILE), and mounting the compose file so the monitor can read root-level depends_on; note replacing <owner> with GitHub org/username
- [x] T031 [P] In README.md add or expand "How to use" section: configuration (WATCHDOG_COMPOSE_PATH, COMPOSE_FILE), requirement for root-level depends_on in the compose file (short and long form), link to specs/001-container-health-monitor/quickstart.md and contracts/depends-on-label.md; verification steps (make parent unhealthy, check logs)
- [x] T032 [P] In README.md add "Debugging / troubleshooting" section: how to view logs (docker logs watch-dog), common issues (no parents discovered when compose path unset or file not mounted, compose file not readable, containers not started by Compose so no com.docker.compose.service label), and pointer to quickstart and contracts for details

**Checkpoint**: README is self-contained for Compose-from-GitHub usage, usage, and debugging.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — blocks all user stories
- **Phase 3 (US3 Discovery)**: Depends on Phase 2 — provides parent/dependents for US1/US2
- **Phase 4 (US1 Restart Parent)**: Depends on Phase 2 and Phase 3
- **Phase 5 (US2 Restart Dependents)**: Depends on Phase 4 (recovery flow extended)
- **Phase 6 (US4 Image & GHCR)**: Depends on Phase 5 (working binary to package)
- **Phase 7 (Polish)**: Depends on Phase 6
- **Phase 8 (Refactor depends_on)**: Depends on Phase 7; refactors Phase 3 discovery to use compose root-level depends_on
- **Phase 9 (README)**: Depends on Phase 8; documentation only

### User Story Dependencies

- **US3 (P1)**: After Foundation — no other story dependency; discovery must be done before US1/US2
- **US1 (P1)**: After Foundation and US3 — needs discovery to know “parent”
- **US2 (P2)**: After US1 — extends recovery flow with wait-healthy and restart dependents
- **US4 (P2)**: After US2 — packages the binary into image and CI

### Within Each User Story

- US3: Parse labels → build map → expose get-dependents
- US1: RestartParent → filter unhealthy to parents → wire event loop
- US2: WaitUntilHealthy → RestartDependents → integrate into recovery flow
- US4: Dockerfile → workflow → docs

### Parallel Opportunities

- T003, T004 [P] in Phase 1 can run with T001/T002
- T006 [P] in Phase 2 can run with T005/T007
- T008 [P] in Phase 3 can run first; T009/T010 sequential
- T018, T020 [US4] and T021, T022, T024 [P] in Polish can be parallelized where different files
- T026 [P] in Phase 8 can start first; T027–T029 sequential
- T030, T031, T032 [P] in Phase 9 can run in parallel (all README.md)

---

## Parallel Example: User Story 3

```bash
# T008 can start as soon as Phase 2 is done (single file internal/discovery/labels.go)
# T009 depends on T008 (same package); T010 depends on T009
```

## Parallel Example: Phase 7

```bash
# T021 (startup reconciliation), T022 (polling fallback), T024 (README) are [P] — different files
# T023 (quickstart validation) runs after binary/image is stable
```

## Parallel Example: Phase 8 (Refactor)

```bash
# T025 (config + YAML dep) first; T026 [P] (compose parsing) can run in parallel once T025 is done
# T027 depends on T026; T028 depends on T027; T029 (contracts/docs) can run after T028
```

## Parallel Example: Phase 9 (README)

```bash
# T030, T031, T032 are [P] — all edit README.md; can be done in one pass or split by section
```

---

## Implementation Strategy

### MVP First (User Story 1 + US3)

1. Complete Phase 1: Setup  
2. Complete Phase 2: Foundational  
3. Complete Phase 3: US3 Discovery  
4. Complete Phase 4: US1 Monitor and Restart Parent  
5. **STOP and VALIDATE**: Run monitor with one parent with healthcheck; make it unhealthy; confirm restart  
6. Then add US2 (dependents) and US4 (image/CI)

### Incremental Delivery

1. Setup + Foundation → Foundation ready  
2. US3 → Discovery working, 100% dynamic  
3. US1 → MVP: unhealthy parent restarted  
4. US2 → Full recovery order: parent then dependents  
5. US4 → Image and GHCR; drop-in deployable  
6. Polish → Startup reconciliation, polling fallback, docs  
7. Phase 8 → Discovery uses root-level depends_on from compose (short + long form)
8. Phase 9 → README: Docker Compose from GitHub, how to use, debugging

### Task Count Summary

| Phase              | Task IDs   | Count |
|--------------------|------------|-------|
| Phase 1 Setup      | T001–T004  | 4     |
| Phase 2 Foundation | T005–T007  | 3     |
| Phase 3 US3        | T008–T010  | 3     |
| Phase 4 US1        | T011–T014  | 4     |
| Phase 5 US2        | T015–T017  | 3     |
| Phase 6 US4        | T018–T020, T033 | 4     |
| Phase 7 Polish     | T021–T024  | 4     |
| Phase 8 Refactor   | T025–T029  | 5     |
| Phase 9 README     | T030–T032  | 3     |
| **Total**          |            | **33**|

---

## Notes

- [P] tasks use different files and have no ordering dependency within their phase where noted.
- [USn] maps each task to the user story for traceability.
- Spec does not require tests; add tests in a follow-up if desired.
- User may commit after each task or logical group; stop at checkpoints to validate story independently (agents do not run git commit per project constitution).
