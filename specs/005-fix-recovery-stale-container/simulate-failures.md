# Simulating Unrestartable States for Verification

This guide describes how to manually trigger each unrestartable error condition so you can verify watch-dog’s bounded retries and recovery-after-recreate behavior. See [quickstart.md](./quickstart.md) for expected outcomes.

## Prerequisites

- A running stack with watch-dog and at least one parent (e.g. VPN) discovered from the compose file.
- Docker CLI access to the same host (socket or DOCKER_HOST).

---

## 1. No such container

**Goal**: Restart fails because the container no longer exists.

**Steps**:

1. Note the parent container name and ID, e.g. `vpn` and `abc123...`.
2. Stop and remove the container:  
   `docker rm -f <container_name_or_id>`
3. Run watch-dog (or leave it running). It will attempt recovery (e.g. on the next event or poll).
4. **Expected**: One (or a small bounded number of) recovery failure log lines for that container ID, with a message that the container is unrestartable (e.g. "container_gone") and that the monitor will not retry this ID. On subsequent triggers you should see skip messages, not repeated identical errors.

**Verify recovery after recreate**:

5. Recreate the same service so a new container exists:  
   `docker compose up -d vpn` (or equivalent).
6. **Expected**: On the next discovery, watch-dog sees the new container ID and runs normal recovery for that service if needed. No need to restart the monitor.

---

## 2. Marked for removal

**Goal**: Restart fails because the container is marked for removal and cannot be started.

**Steps**:

1. Stop the container:  
   `docker stop <container_name_or_id>`
2. Remove the container (this marks it for removal; the daemon may not delete it immediately):  
   `docker rm <container_name_or_id>`  
   If the container is still present, the daemon may report "marked for removal" when you try to start/restart it.
3. Trigger recovery (e.g. by having watch-dog poll or by an event). Alternatively, run:  
   `docker start <container_name_or_id>`  
   and observe the daemon error; watch-dog will see a similar error when it tries to restart.
4. **Expected**: Watch-dog logs that recovery failed (reason e.g. "marked_for_removal") and will not retry this container ID. Subsequent triggers show skip messages.

**Verify recovery after recreate**:

5. Recreate the service (new container):  
   `docker compose up -d <service>`
6. **Expected**: Watch-dog picks up the new ID and resumes normal behavior.

---

## 3. Dependency missing (e.g. network namespace)

**Goal**: A dependent’s restart fails because its network dependency (another container) is missing.

**Steps**:

1. Use a stack with a parent (e.g. `vpn`) and a dependent (e.g. `dler`) that uses the parent’s network.
2. Remove or stop the **parent** so it no longer exists (or is gone from the network namespace).
3. Trigger recovery for the **dependent** (e.g. make the dependent unhealthy or stop it). When watch-dog tries to restart the dependent, the daemon may return an error like “joining network namespace of container … No such container”.
4. **Expected**: Watch-dog logs that recovery failed for the dependent (reason e.g. "dependency_missing") and will not retry that dependent’s container ID. Other dependents and parents are still handled.

**Verify recovery after dependency is back**:

5. Recreate the parent:  
   `docker compose up -d vpn`
6. Once the parent is healthy, watch-dog may proactively restart dependents (if the parent has a new ID). Otherwise, trigger recovery for the dependent again; the new parent’s network is available, so restart should succeed.

---

## 4. Proactive restart when only parent is replaced (FR-007)

**Goal**: Parent is replaced by an updater (new container ID); dependents that were failing due to “dependency missing” come back after watch-dog proactively restarts them.

**Steps**:

1. Run a stack with a parent (e.g. `vpn`) and a dependent (e.g. `dler`). Run watch-dog and an updater (e.g. watchtower, wud, ouroboros).
2. Let the updater replace **only the parent** (new image → new container ID for the parent; dependent is unchanged).
3. **Expected**: Within one poll interval or on the next discovery after the new parent is healthy, watch-dog logs “parent has new ID, proactively restarting dependents” and restarts the dependent(s). The dependent comes back online without being recreated.

---

## Optional script

See `scripts/simulate-unrestartable.sh` (if present) for Docker commands that print or run steps for each scenario. You can also add a Makefile target that invokes it (e.g. `make simulate-unrestartable`).
