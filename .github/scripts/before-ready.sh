#!/usr/bin/env bash
# Runs the Go tests that a branch's own change makes stale: those that compare with a
# golden, a hash, a frozen table or a stored fixture, and the inventory tests that list
# the service methods, the error reasons, the authorization matrices and the event kinds.
# A session runs it before it writes `ready:` (docs/testing.md#before-ready):
#
#   make before-ready
#
# The tests, their packages and the command that regenerates each file are in one place,
# backend/internal/archtest/before_ready.txt. On a failure the script prints that command
# next to the failed test.
#
# By default it runs only the lines of the list that the change can affect: the working tree
# against the merge base with origin/main, and the packages and inputs each line reads
# (backend/cmd/beforeready). A change that reaches none of them (web, docs) runs nothing and
# exits 0. When the change cannot be compared, every line runs.
#
# Environment:
#   MEURPG_TEST_DATABASE_URL  required: the tests that need a database fail without it
#   BEFORE_READY_JOBS         packages tested at once (default 4)
#   BEFORE_READY_LIST         another list file, to try a few tests (same format). Every line of it
#                             runs, whatever the change, and the output says so
#   BEFORE_READY_ALL          any value but empty or 0: run every line of the list, whatever the change
#   BEFORE_READY_BASE         a revision to compare the working tree with, instead of the merge
#                             base with origin/main (HEAD: only what is not committed yet)
#   BEFORE_READY_ONLY         a Go (RE2) regex: test only the packages whose directory
#                             matches it (a rerun of what failed, e.g. 'internal/rules$'). It
#                             narrows the selected lines, or every line with BEFORE_READY_ALL set.
#                             An invalid one, or one that matches no package, exits 2
#
# Bash 3.2 compatible (macOS's /bin/bash): no associative arrays, no mapfile, no wait -n.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
default_list="$root/backend/internal/archtest/before_ready.txt"
list="${BEFORE_READY_LIST:-$default_list}"
list_shown="${list#"$root"/}"

# One package: the mode xargs calls, so the packages run side by side and each keeps its
# own output. $2 is the package, $3 the directory of the logs (which holds the tool, built
# by the parent, that reads the lines to run from BEFORE_READY_LIST).
if [ "${1:-}" = "--one" ]; then
  out="$3/$(echo "$2" | tr '/' '_').out"
  echo $$ >"$out.pid"
  cd "$root/backend"
  regex="$("$3/beforeready" -list "$BEFORE_READY_LIST" -regex-of "$2")"
  if go test -count=1 -timeout 45m -run "$regex" "./$2" >"$out" 2>&1; then
    echo 0 >"$out.status"
  else
    echo 1 >"$out.status"
  fi
  exit 0
fi

[ -r "$list" ] || { echo "before-ready: cannot read $list" >&2; exit 2; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
started="$(date +%s)"

# The tool reads the list, chooses the lines to run (see the header) and answers the rest of
# the script: the packages, the -run regex of each and the regenerate command of a failure. A
# BEFORE_READY_ALL that is set (not empty, not 0) takes every line, and so does a custom
# BEFORE_READY_LIST (the change is compared on the project's own list). BEFORE_READY_ONLY is
# applied there, as an RE2 regex: an invalid one, or one that matches no package, exits 2. The
# children of --one read the chosen list through BEFORE_READY_LIST.
flags=""
case "${BEFORE_READY_ALL:-}" in "" | 0) ;; *) flags="-all" ;; esac
[ -z "${BEFORE_READY_LIST:-}" ] || flags="$flags -custom"
# go build starts in backend/, so a relative list is made absolute here.
case "$list" in /*) ;; *) list="$PWD/$list" ;; esac
tool="$tmp/beforeready"
(cd "$root/backend" && go build -o "$tool" ./cmd/beforeready) || { echo "before-ready: cannot build backend/cmd/beforeready" >&2; exit 2; }
# shellcheck disable=SC2086 # $flags are words on purpose
(cd "$root/backend" && "$tool" -list "$list" -out "$tmp/selected.txt" -only "${BEFORE_READY_ONLY:-}" -base "${BEFORE_READY_BASE:-}" $flags) || exit $?
list="$tmp/selected.txt"
export BEFORE_READY_LIST="$list"

packages="$("$tool" -list "$list" -packages)"
[ -n "$packages" ] || exit 0
count="$(echo "$packages" | wc -l | tr -d ' ')"

if [ -z "${MEURPG_TEST_DATABASE_URL:-}" ]; then
  url="${TEST_DATABASE_URL:-postgresql://root@localhost:5433/postgres?sslmode=disable}"
  cat >&2 <<EOF
before-ready: MEURPG_TEST_DATABASE_URL is not set. Most of these tests need the test
database, and without it they would skip and pass for nothing. Start it and point the tests
at it:

  make db-test-start
  export MEURPG_TEST_DATABASE_URL='$url'
  make before-ready

(docs/testing.md#integration-tests; any PostgreSQL or CockroachDB works the same way.)
EOF
  exit 2
fi
# A test that needs the database fails instead of skipping when it is missing.
export MEURPG_REQUIRE_DB=1

echo "before-ready: running the golden, frozen, fixture and inventory tests of $count packages ($list_shown)"
# A child that dies (killed, out of memory, no backend/ to enter) makes xargs exit non-zero: the
# summary runs whatever it returns, and the package of that child has no result, which the
# summary reports as FAILED.
xargs_status=0
echo "$packages" | xargs -P "${BEFORE_READY_JOBS:-4}" -I{} bash "${BASH_SOURCE[0]}" --one {} "$tmp" || xargs_status=$?
if [ "$xargs_status" -ne 0 ]; then
  echo "before-ready: a test run ended abnormally (xargs exit $xargs_status): waiting for the others to end" >&2
  # xargs gives up at once when a child is killed, while the others still run: the summary
  # (and the removal of the logs) waits for every child that has started and left no result.
  for pkg in $packages; do
    out="$tmp/$(echo "$pkg" | tr '/' '_').out"
    pid="$(cat "$out.pid" 2>/dev/null || true)"
    while [ -n "$pid" ] && [ ! -e "$out.status" ] && kill -0 "$pid" 2>/dev/null; do sleep 1; done
  done
fi

failed=0
for pkg in $packages; do
  out="$tmp/$(echo "$pkg" | tr '/' '_').out"
  status="$(cat "$out.status" 2>/dev/null || echo missing)"
  if [ "$status" = missing ]; then
    failed=1
    echo
    echo "=== FAILED: $pkg: the test run left no result (the script was interrupted, or go could not start): run it again."
    [ ! -s "$out" ] || { echo "--- the output it left:"; cat "$out"; }
    continue
  fi
  if [ "$status" = 0 ]; then
    grep -E '^(ok|\?)' "$out" || true
    continue
  fi
  failed=1
  echo
  echo "=== FAILED: $pkg"
  cat "$out"
  names="$(grep -E '^--- FAIL: ' "$out" | sed 's/^--- FAIL: \([^ ]*\).*/\1/' | sort -u || true)"
  if [ -z "$names" ]; then
    echo "--- $pkg failed without a failed test (it did not build, panicked or timed out): read the output above."
    continue
  fi
  for name in $names; do
    cmd="$("$tool" -list "$list" -regen-of "$pkg" -test "$name" 2>/dev/null || true)"
    echo "--- $pkg $name"
    if [ -n "$cmd" ]; then
      echo "    regenerate, only if the change is intended: $cmd"
    else
      echo "    backend/internal/archtest/before_ready.txt has no regenerate command for it"
    fi
  done
done

seconds=$(($(date +%s) - started))
if [ "$failed" -ne 0 ]; then
  cat <<EOF

before-ready: FAILED after ${seconds}s. A failure here means the branch changed what the test pins.
If the change is intended, run the command printed under the test, then:
  - READ the diff of every file it rewrote (git diff) before you commit it: a regenerated
    file that nobody read pins whatever the code does, a bug included.
  - Run a NEW golden test with -count=5, because a Go map's order is random:
    cd backend && go test ./internal/<package> -run '^<Test>\$' -count=5
  - Run make before-ready again.
If the change is not intended, fix the code.
EOF
  exit 1
fi
echo "before-ready: ok in ${seconds}s"
