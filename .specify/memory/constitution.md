# watch-dog Constitution

## Core Principles

### I. Contract-first behavior
Observable behavior is defined in `specs/<feature>/contracts/` (e.g. recovery-behavior, depends_on, healthcheck). Code and docs must align with contracts; contract changes require spec/plan updates.

### II. Spec-branch workflow
Feature work happens on spec branches (e.g. `001-container-health-monitor`, `003-fix-children-handling`). The active spec directory is resolved from the current branch (or `SPECIFY_FEATURE`). Do not assume a branch; verify or prompt if unclear.

### III. Recovery order
Parent-first, then dependents: restart parent → wait until healthy → restart dependents (one at a time per spec 003). No restart of dependents until parent is healthy; behavior matches contracts/recovery-behavior.md.

### IV. Observability
Structured logging via log/slog; LOG_LEVEL and LOG_FORMAT from env (see contracts/env-logging-healthcheck). All significant actions (discovery, recovery, errors) are logged so operators can verify behavior.

### V. Simplicity and scope
Single binary, no persistent config; discovery is fully from the compose file and runtime. No features outside the spec (e.g. no port checks, no editing user files). YAGNI.

## Workflow Rules

### Spec branch
- Before making changes or running spec-related commands, confirm the repository is on the correct spec/feature branch (e.g. `001-container-health-monitor` for that spec).
- Use the current branch (or `SPECIFY_FEATURE`) to resolve the active spec directory under `specs/`. Do not assume a branch; verify or prompt if unclear.

### No automated commits
- Never run `git commit` or create commits. The agent may stage files (`git add`) or suggest commit messages, but committing is left to the user.

## Technology and Constraints

- **Stack**: Go 1.21+, Docker Engine API (github.com/docker/docker/client), standard library, log/slog. See `.cursor/rules/specify-rules.mdc` for current "Active Technologies". Canonical layout: `cmd/watch-dog/` (single binary), `internal/` (docker, discovery, recovery); specify-rules may show generic `src/`, `tests/` — the repo uses `cmd/` and `internal/` and test files alongside packages (e.g. `internal/discovery/labels_test.go`).
- **Deliverable**: Single container image; build and publish via GitHub (e.g. GHCR). No modification of files outside the project repository.

## Development and Quality

- **Contracts**: New or changed behavior must be reflected in the appropriate spec's `contracts/`. Implementation and README/quickstart must match contracts.
- **Testing**: Contract-driven; integration tests for recovery behavior and discovery when added. Existing unit test: `internal/discovery/labels_test.go`; extend tests when touching contracts (per tasks.md and speckit commands).

## Governance

This constitution supersedes ad-hoc practices for watch-dog. PRs and reviews should verify compliance with contracts and recovery order. Amendments: document change, update this file and version/date; keep ratification/last-amended footer.

**Version**: 1.0.0 | **Ratified**: 2026-02-28 | **Last Amended**: 2026-02-28
