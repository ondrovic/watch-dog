# Research: 004-child-deps-initial-restart

**Branch**: `004-child-deps-initial-restart` | **Date**: 2026-02-28

## 1. Environment variable for initial discovery wait time

**Decision**: Use `WATCHDOG_INITIAL_DISCOVERY_WAIT` with the same duration format as `RECOVERY_COOLDOWN` (Go `time.ParseDuration`: e.g. `30s`, `2m`, `5m`).

**Rationale**: Aligns with existing env vars (`RECOVERY_COOLDOWN`, `WATCHDOG_CONTAINER_NAME`); duration format is familiar to operators and supports 120s–5min range from spec.

**Alternatives considered**: Separate numeric seconds + unit (rejected: more parsing, no benefit over ParseDuration); config file (rejected: constitution says no persistent config).

---

## 2. Default wait duration

**Decision**: Default `WATCHDOG_INITIAL_DISCOVERY_WAIT` to **60 seconds** when unset or invalid.

**Rationale**: Gives most small stacks time to become healthy after compose up without delaying recovery for already-stable stacks. Spec says stacks vary (120s–5min); operators with slower stacks set the env var. 60s is a safe middle ground and avoids zero (which would make "initial discovery" effectively first cycle only).

**Alternatives considered**: 0 (rejected: no wait can still cause restarts during compose up); 120s (acceptable but longer for fast stacks); 30s (acceptable, 60s chosen for slightly more safety).

---

## 3. Definition of "first discovery cycle"

**Decision**: "First full discovery cycle" = the first successful call to `discovery.BuildParentToDependents` at monitor startup. No extra polling for "all parents observed"; that first call already returns the full parent→dependents map. Initial discovery phase = **that first discovery completed** + **wait time**.

**Rationale**: Code already performs one discovery at startup before any recovery; reusing it keeps the model simple. "Until all monitored parents have been observed at least once" is satisfied by that single successful call (we have the full set of parents from the compose file at that point).

**Alternatives considered**: Multiple discovery cycles until stable (rejected: adds complexity and delay without spec requirement); time-only phase (rejected: spec requires discovery + wait).

---

## 4. Gating recovery during initial discovery

**Decision**: (1) Do not call `runStartupReconciliation` until after the initial discovery phase has elapsed. (2) After starting the health-event subscription and polling goroutine, ignore recovery triggers (do not run `flow.RunFullSequence`) until the phase has elapsed; then run startup reconciliation once and allow event-driven and polling-driven recovery as today.

**Rationale**: Ensures no recovery or dependent restarts happen during the phase. Single place to gate: a "phase complete" flag or timestamp checked before any recovery path (startup, events, polling).

**Alternatives considered**: Run startup reconciliation but skip only "restart dependents" (rejected: spec says no recovery at all during initial discovery). Defer only startup reconciliation and allow events (rejected: events can fire during compose up and trigger recovery).

---

## 5. Logging and observability

**Decision**: Log at INFO: (1) at startup, that initial discovery phase has started and its wait duration; (2) when the wait has elapsed, that initial discovery is complete and normal recovery is enabled. Use existing slog and LOG_LEVEL/LOG_FORMAT so operators can verify behavior (FR-005, SC-006).

**Rationale**: Constitution IV requires structured logging for significant actions; phase boundaries are significant for debugging cascade issues.

**Alternatives considered**: DEBUG-only (rejected: operators need to confirm phase without changing LOG_LEVEL).
