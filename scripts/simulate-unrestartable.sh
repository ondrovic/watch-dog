#!/usr/bin/env bash
# simulate-unrestartable.sh — Print or run Docker commands to simulate unrestartable
# states (no such container, marked for removal, dependency missing) for verifying
# watch-dog bounded retries and recovery-after-recreate. See
# specs/005-fix-recovery-stale-container/simulate-failures.md.
#
# Usage:
#   ./scripts/simulate-unrestartable.sh [parent_name]     # print commands only
#   ./scripts/simulate-unrestartable.sh --run [parent_name]  # run "no such container" steps (destructive)

set -euo pipefail
PARENT_NAME="${1:-vpn}"
RUN_MODE=false
if [[ "${1:-}" == "--run" ]]; then
	RUN_MODE=true
	PARENT_NAME="${2:-vpn}"
fi

echo "=== 1. No such container ==="
echo "# Remove the parent so it no longer exists; watch-dog will attempt recovery and then skip."
if [[ "$RUN_MODE" == true ]]; then
	docker rm -f "$PARENT_NAME" 2>/dev/null || true
	echo "Removed container $PARENT_NAME. Run watch-dog and check logs for one failure then skip messages."
else
	echo "  docker rm -f \"$PARENT_NAME\""
	echo "  # Then run watch-dog; expect one (or bounded) failure log, then skip messages."
	echo "# Recreate to verify recovery:"
	echo "  docker compose up -d $PARENT_NAME"
fi

echo ""
echo "=== 2. Marked for removal ==="
echo "# Stop and remove the container; daemon may report 'marked for removal' on start/restart."
echo "  docker stop $PARENT_NAME"
echo "  docker rm $PARENT_NAME"
echo "  # Trigger recovery; expect 'marked_for_removal' and skip thereafter."

echo ""
echo "=== 3. Dependency missing ==="
echo "# Use a parent (e.g. vpn) and dependent (e.g. dler). Remove the parent, then trigger"
echo "# recovery for the dependent; daemon may return 'joining network namespace ... No such container'."
echo "  docker rm -f <parent_name>   # e.g. vpn"
echo "  # Make dependent unhealthy or stop it; watch-dog will try to restart dependent and see dependency_missing."

echo ""
echo "=== 4. Proactive restart (parent replaced) ==="
echo "# Run an updater (watchtower, wud, ouroboros) and replace only the parent."
echo "# Watch-dog should log 'parent has new ID, proactively restarting dependents' and restart dependents."
