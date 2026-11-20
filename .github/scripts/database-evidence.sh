#!/usr/bin/env bash
# Prints the state of the test database of a `go-db` job. After a job that passed (`brief`) it prints one group: the
# container's state and memory, the runner's memory and how full CockroachDB's store is, so a run that passed shows how
# much room it had. After a step failed it also prints the end of the database's log, the runner's disk and the kernel's
# memory kills, so a test that could not reach the database explains itself in the job's log (docs/ci.md, "When the test
# database stops").
# Usage: database-evidence.sh <cockroach|postgres> [brief]   (the container name start-cockroach.sh or start-postgres.sh gave)
# It never fails: every command is best effort.
set -uo pipefail

name=${1:?the container name: cockroach or postgres}
brief=${2:-}

echo "::group::The test database container ($name)"
docker inspect --format 'status={{.State.Status}} oom_killed={{.State.OOMKilled}} exit_code={{.State.ExitCode}} started={{.State.StartedAt}} finished={{.State.FinishedAt}}' "$name" 2>&1 || true
docker stats --no-stream --format 'memory={{.MemUsage}} cpu={{.CPUPerc}}' "$name" 2>&1 || true
free -m 2>&1 || true
if [ "$name" = cockroach ]; then
  # The store is in memory with a fixed size (start-cockroach.sh). An in-memory store reports no used space, so its
  # fill is read from the bytes it holds: `total` counts every version, the ones dropped test databases still hold
  # until the garbage collection takes them included, and `live` only what a read sees. CockroachDB answers the store's
  # status only to a session that allows its internal tables, which is fine for a throwaway test node.
  docker exec cockroach cockroach sql --insecure --host=localhost:26257 --format=tsv \
    -e "SET allow_unsafe_internals = true" \
    -e "SELECT store_id, (capacity/1048576)::INT AS store_capacity_mib, ((metrics->>'totalbytes')::FLOAT/1048576)::INT AS store_total_mib, ((metrics->>'livebytes')::FLOAT/1048576)::INT AS store_live_mib, range_count FROM crdb_internal.kv_store_status" 2>&1 || true
fi
echo "::endgroup::"
if [ "$brief" = brief ]; then exit 0; fi

echo "::group::The last lines of the database's log"
docker logs --tail 200 "$name" 2>&1 || true
echo "::endgroup::"

echo "::group::The runner's disk"
df -h / /dev/shm 2>&1 || true
echo "::endgroup::"

echo "::group::Processes the kernel killed for memory"
sudo dmesg --ctime 2>/dev/null | grep -i -E 'out of memory|oom-kill|killed process' | tail -n 20 || echo "none in the kernel log"
echo "::endgroup::"
