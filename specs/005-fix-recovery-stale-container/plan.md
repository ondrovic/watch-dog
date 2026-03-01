# Implementation Plan: Fix Recovery When Containers Are Gone or Unrestartable (Compose SDK)

**Branch**: `005-fix-recovery-stale-container` | **Date**: 2026-03-01 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `/specs/005-fix-recovery-stale-container/spec.md`

**Note**: This plan updates auto-recreate (FR-008) to use the Docker Compose Go SDK instead of shelling out to `docker` / `docker-compose`, eliminating the "unknown shorthand flag: 'f'" error when the Docker binary has no Compose plugin.

## Summary

- **Primary requirement**: When a parent is marked unrestartable with reason **container_gone** or **marked_for_removal**, optionally trigger recreation of that service so the operator does not have to run compose by hand (FR-008).
- **Technical approach**: Replace the current exec-based implementation (`docker compose -f <path> up -d <service>` / `docker-compose` fallback) with the **Docker Compose Go SDK**. The equivalent of `compose up -d <service>` runs in-process; no `docker` or `docker-compose` binary is required. Same behavior: load project from compose file, bring up one service (and its dependencies), detach; resolve container name to service name via existing discovery; log trigger/success/failure.

## Technical Context

**Language/Version**: Go 1.21+ (go.mod 1.25; compatible 1.21+)  
**Primary Dependencies**: Docker Engine API (github.com/docker/docker/client), log/slog, gopkg.in/yaml.v3 (compose parsing). **New for FR-008**: Docker Compose Go SDK (github.com/docker/compose/v2) and github.com/docker/cli for the CLI layer the SDK uses to talk to the daemon; pin docker/cli to v28.5.2+incompatible if using Compose v5 to avoid moby/moby vs docker/docker conflicts.  
**Storage**: N/A (in-memory unrestartable set and last-known parent ID map only; discovery and state from Docker API and compose file).  
**Testing**: go test; contract-driven; extend tests when touching contracts.  
**Target Platform**: Linux with Docker daemon (socket or DOCKER_HOST).  
**Project Type**: CLI (single binary in cmd/watch-dog).  
**Performance Goals**: Bounded retries (SC-001); auto-recreate runs in background with timeout.  
**Constraints**: Single binary, no persistent config; no modification of files outside the project.  
**Scale/Scope**: In-memory set bounded (e.g. max 100 IDs, pruning); one compose project path per process.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Contract-first behavior | PASS | FR-008 behavior defined in contracts/recovery-unrestartable-behavior.md; plan updates contract to SDK-based auto-recreate. |
| II. Spec-branch workflow | PASS | Active branch `005-fix-recovery-stale-container`; spec dir resolved. |
| III. Recovery order | PASS | Parent-first, then dependents; auto-recreate triggers after parent marked unrestartable, does not change recovery order. |
| IV. Observability | PASS | Structured logging (INFO trigger/success, ERROR on failure); same semantics as current auto-recreate. |
| V. Simplicity and scope | PASS | SDK replaces exec; single in-process path; no new user-facing config (WATCHDOG_AUTO_RECREATE unchanged). |
| Stack / Technologies | PASS | Add Compose SDK to Active Technologies; single binary, internal/ layout unchanged. |

## Project Structure

### Documentation (this feature)

```text
specs/005-fix-recovery-stale-container/
├── plan.md              # This file
├── research.md           # Phase 0 (includes Compose SDK decision)
├── data-model.md        # Phase 1 (includes auto-recreate via SDK)
├── quickstart.md        # Phase 1 (updated for SDK-based auto-recreate)
├── contracts/           # recovery-unrestartable-behavior.md (FR-008 updated)
└── tasks.md             # Phase 2 (/speckit.tasks)
```

### Source Code (repository root)

```text
cmd/watch-dog/
└── main.go              # Replace exec-based auto-recreate with Compose SDK init + Up()

internal/
├── docker/              # Existing; no change for auto-recreate
├── discovery/           # Existing; ContainerNameToServiceName unchanged
└── recovery/            # Existing; unrestartable set, last-known parent ID
```

**Structure Decision**: Single binary in cmd/watch-dog; Compose SDK usage is confined to main.go when auto-recreate is enabled (create compose service at startup, call LoadProject + Up in OnParentContainerGone callback).

## Implementation Details (FR-008: Auto-recreate via Compose SDK)

1. **Dependencies**: Add `github.com/docker/compose/v2` and `github.com/docker/cli`; resolve version constraints with existing `github.com/docker/docker` (go mod tidy; pin docker/cli v28.5.2+incompatible if needed for v5).
2. **Compose service**: Create once at startup when `autoRecreate && composePath != ""` using `compose.NewComposeService(dockerCLI)` where `dockerCLI` is from `command.NewDockerCli()` and `Initialize(flags.ClientOptions{})` (uses DOCKER_HOST / env). On init failure: log warning and disable auto-recreate.
3. **Project load**: In OnParentContainerGone callback: `LoadProject(ctx, api.ProjectLoadOptions{ConfigPaths: []string{composePath}, WorkingDir: filepath.Dir(composePath), ProjectName: os.Getenv("COMPOSE_PROJECT_NAME")})` then `Up(ctx, project, api.UpOptions{Create: api.CreateOptions{Services: []string{serviceName}, Recreate: "force"}, Start: api.StartOptions{Services: []string{serviceName}}})`. Run in goroutine with existing timeout; same logging (INFO trigger/success, ERROR on failure).
4. **Service name**: Keep existing resolution: parent container name → compose service name via `discovery.ContainerNameToServiceName(composePath)` (e.g. vpn → gluetun).
5. **Contract**: Update contracts/recovery-unrestartable-behavior.md FR-008 to state that the monitor performs the equivalent of `docker compose -f <composePath> up -d <service_name>` using the **Docker Compose Go SDK** (no docker or docker-compose binary required); remove "retry with docker-compose" wording.

## Complexity Tracking

No constitution violations requiring justification.
