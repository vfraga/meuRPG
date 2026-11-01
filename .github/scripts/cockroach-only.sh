#!/usr/bin/env bash
# The backend tests that need CockroachDB itself (backend/cockroach-only-tests.txt), for the PostgreSQL runs and their checks.
# Run it from the repository root.
#
#   cockroach-only.sh skip-regex          print the `go test -skip` regex that leaves them out
#   cockroach-only.sh run-regex           print the `go test -run` regex that selects only them
#   cockroach-only.sh packages            print the packages that hold them, as `go test` takes them
#   cockroach-only.sh show                print the list, for a job log
#   cockroach-only.sh check-names         fail unless every entry is a top-level test of its package and its name is
#                                         unique in the backend (-skip matches names in every package)
#   cockroach-only.sh check-list          run the listed tests on the PostgreSQL MEURPG_TEST_DATABASE_URL names and fail
#                                         unless every one of them fails (a test that passes there does not need
#                                         CockroachDB, and leaves the list)
set -euo pipefail

self="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
root=$(cd "$(dirname "$0")/../.." && pwd)
list="$root/backend/cockroach-only-tests.txt"
module=github.com/PuraFome/meuRPG/backend

# entries prints "<package> <test>" for each entry of the list.
entries() {
  awk '!/^[[:space:]]*(#|$)/ { print $1, $2 }' "$list"
}

join_names() {
  entries | awk '{ print $2 }' | paste -sd'|' -
}

case "${1:-}" in
  skip-regex)
    echo "^($(join_names))\$"
    ;;
  run-regex)
    echo "^($(join_names))\$"
    ;;
  packages)
    entries | awk -v m="$module" '{ print m "/" $1 }' | sort -u
    ;;
  show)
    awk '!/^[[:space:]]*(#|$)/' "$list"
    ;;
  check-names)
    status=0
    names=$(entries | awk '{ print $2 }')
    dups=$(echo "$names" | sort | uniq -d)
    if [ -n "$dups" ]; then
      echo "::error file=backend/cockroach-only-tests.txt::listed twice: $dups" >&2
      status=1
    fi
    while read -r pkg name; do
      dir="$root/backend/$pkg"
      if ! grep -qE "^func $name\(t \*testing\.T\)" "$dir"/*_test.go 2>/dev/null; then
        echo "::error file=backend/cockroach-only-tests.txt::$pkg has no top-level test $name" >&2
        status=1
      fi
      # -skip and -run match a name in every package: another package's test of the same name would be left out too.
      others=$(grep -rlE "^func $name\(t \*testing\.T\)" --include='*_test.go' "$root/backend" | xargs -r -n1 dirname | grep -vx "$dir" || true)
      if [ -n "$others" ]; then
        echo "::error file=backend/cockroach-only-tests.txt::$name is also a test in: $others" >&2
        status=1
      fi
    done < <(entries)
    exit "$status"
    ;;
  check-list)
    : "${MEURPG_TEST_DATABASE_URL:?set MEURPG_TEST_DATABASE_URL to a PostgreSQL}"
    out=$(mktemp)
    trap 'rm -f "$out"' EXIT
    mapfile -t packages < <("$self" packages)
    # The tests are expected to fail: the exit code of go test is not the answer, the per-test results are.
    (cd "$root/backend" && MEURPG_REQUIRE_DB=1 go test -json -count=1 -timeout 20m -run "$("$self" run-regex)" "${packages[@]}" > "$out") || true
    status=0
    while read -r pkg name; do
      result=$(jq -r --arg p "$module/$pkg" --arg t "$name" 'select(.Package == $p and .Test == $t and (.Action == "pass" or .Action == "fail" or .Action == "skip")) | .Action' "$out" | tail -n1)
      case "$result" in
        fail) echo "ok: $pkg $name fails on PostgreSQL, as listed" ;;
        pass) echo "::error file=backend/cockroach-only-tests.txt::$name passes on PostgreSQL: it no longer needs CockroachDB, so remove it from the list" >&2; status=1 ;;
        *) echo "::error file=backend/cockroach-only-tests.txt::$name did not run on PostgreSQL (result: ${result:-none})" >&2; status=1 ;;
      esac
    done < <(entries)
    exit "$status"
    ;;
  *)
    sed -n '2,13p' "$0" >&2
    exit 2
    ;;
esac
