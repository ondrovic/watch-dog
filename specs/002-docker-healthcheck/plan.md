# Implementation Plan: Add Healthcheck and Observability to Watch-Dog Container

**Branch**: `002-docker-healthcheck` | **Date**: 2026-02-28 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-docker-healthcheck/spec.md`

## Summary

Add a Docker healthcheck to the watch-dog image (Docker API–based liveness check), make healthcheck parameters configurable via .env (INTERVAL_IN_SECS, RETRIES, START_PERIOD_IN_SECS, TIMEOUT_IN_SECS), and improve log output: configurable LOG_LEVEL (DEBUG, INFO, WARN, ERROR) and LOG_FORMAT (compact, timestamp, optional json) with human-readable styles. Document all options and provide README examples for every LOG_LEVEL and LOG_FORMAT configuration.

## Technical Context

**Language/Version**: Go 1.21+ (existing)  
**Primary Dependencies**: Docker Engine API (github.com/docker/docker/client), standard library, log/slog (existing)  
**Storage**: N/A  
**Testing**: Go testing (stdlib); extend existing tests for log level/format and healthcheck behavior  
**Target Platform**: Linux (Docker host); container image Alpine 3.19 (existing)  
**Project Type**: CLI/daemon (long-running process in container)  
**Performance Goals**: Healthcheck completes within configured timeout (e.g. 10s); log formatting adds minimal overhead (no blocking I/O; handler selection at startup only)  
**Constraints**: Single binary; runtime image may add Docker CLI for HEALTHCHECK unless alternative (e.g. wget to localhost) is chosen; no breaking changes to existing env vars  
**Scale/Scope**: Same as 001 (single host; tens of containers; one monitor instance)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- Constitution file (`.specify/memory/constitution.md`) is still a template and not yet ratified for this project.
- **No project-specific gates applied.** Proceeding with plan; when the constitution is ratified, re-run this check against the listed principles.

## Project Structure

### Documentation (this feature)

```text
specs/002-docker-healthcheck/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output (healthcheck + log options)
├── contracts/           # Phase 1 output (env vars, healthcheck contract)
└── tasks.md             # Phase 2 output (/speckit.tasks – not created by plan)
```

### Source Code (repository root)

Existing layout from 001; this feature adds or changes:

```text
cmd/watch-dog/
  main.go                # Wire LOG_LEVEL / LOG_FORMAT at startup (read env, set slog)

internal/
  docker/
    client.go            # (unchanged)
    events.go            # (unchanged)
    log.go               # EXTEND: slog handler from LOG_LEVEL + LOG_FORMAT; custom formats (compact, timestamp, json)
  discovery/             # (unchanged)
  recovery/              # (unchanged)

Dockerfile               # ADD: HEALTHCHECK using Docker API check; optionally install docker CLI in runtime stage
README.md                # ADD: Healthcheck section with .env vars; LOG_LEVEL + LOG_FORMAT table with examples for each value
```

**Structure Decision**: No new packages. Extend `internal/docker/log.go` for configurable level and format; extend `Dockerfile` and `README.md` as above. Compose example in README (and quickstart) uses env var substitution for healthcheck and logging.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| (none) | — | — |
