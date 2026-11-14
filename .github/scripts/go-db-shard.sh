#!/usr/bin/env bash
# Splits the backend's Go tests into the groups the `go-db` CI job runs in
# parallel, each against its own CockroachDB. Run it from backend/.
#
#   go-db-shard.sh <group>        print the group's packages, one per line
#   go-db-shard.sh --run <group>  print the group's `-run` regex (empty unless
#                                 the group is a part of a split package: play
#                                 or leaktest)
#   go-db-shard.sh --check        fail unless the groups together are exactly
#                                 `go list ./...` (nothing skipped, nothing
#                                 twice) and the parts of each split package
#                                 together are exactly that package's tests
#   go-db-shard.sh --verify-scan  fail unless the tests read from each split
#                                 package's source are exactly the tests
#                                 `go test -list` prints (compiles the test
#                                 binaries: about a minute)
#
# The heavy packages are named by hand, from the per-package times of the
# 10/10/2026 run (play 920 s, leaktest 630 s, rules/dungeon 598 s and rules
# 476 s, maps/images 471 s, campaigns 309 s, campaignpackage 253 s, identity
# 157 s, with the groups' packages sharing four vCPUs). The group "rest" is
# everything not named here, so a NEW package lands in "rest" on its own and
# is never skipped. If a group gets too slow, move a package up.
#
# The play package alone took 920 s, so it is split by test: play-1 to play-6
# each run the tests whose name hashes (cksum) to their part, with
# `-run '^(TestA|TestB)$'`. The leaktest package is split the same way, in
# leak-1 to leak-4: every test builds its own world, so no test needs another
# to run first. The rules package (pure, CPU-bound under -race and coverage)
# is split in rules-1 to rules-3: whole, it reached the 25-minute -timeout. A package is split by adding its prefix to split_prefixes, its
# directory to split_dir (when it is not the prefix), its number of parts to
# split_parts, and its parts to case_packages' pattern. The hash is stable:
# adding a test never moves the others, and a NEW test lands in a part by
# itself. A test too heavy to share a part is pinned to one of its own (pinned_tests). The tests are read from the source, so a group needs no compile to
# get its `-run`; --check proves each test is in
# exactly one part, and --verify-scan that the source reading finds exactly the
# tests `go test -list` prints (Test, Fuzz and Example).
set -euo pipefail

module=github.com/PuraFome/meuRPG/backend

case_packages() {
  case "$1" in
    play-[1-9]|leak-[1-9]|rules-[1-9]) echo "$module/internal/$(split_dir "${1%-*}")" ;;
    maps) echo "$module/internal/maps" ;;
    characters) echo "$module/internal/characters $module/migrations $module/internal/progression" ;;
    # The CPU-bound pure packages, kept off the runners of the database ones: rules (split
    # above, in rules-1 to rules-3) and dungeon with puzzle.
    dungeon) echo "$module/internal/rules/dungeon $module/internal/rules/puzzle" ;;
    rest) echo "" ;;
    *) echo "unknown group: $1" >&2; return 1 ;;
  esac
}

# play-2 to play-6 name the same package as play-1, leak-2 to leak-4 the same as leak-1,
# and rules-2 and rules-3 the same as rules-1: only the first part of a split
# package counts here.
named_groups="play-1 maps characters rules-1 dungeon leak-1"

# The split packages, by the prefix of their groups (play-N, leak-N, rules-N): the
# directory under internal/ that case_packages names, and the number of parts.
split_prefixes="play leak rules"
split_dir() { case "$1" in leak) echo leaktest ;; *) echo "$1" ;; esac; }
split_parts() {
  case "$1" in
    play) echo 6 ;;
    leak) echo 4 ;;
    rules) echo 3 ;;
    *) echo "not a split package: $1" >&2; return 1 ;;
  esac
}

all_packages() { go list ./... | sort; }

named_packages() { for g in $named_groups; do case_packages "$g" | tr ' ' '\n'; done | sort; }

# A split package's tests (Test*, Fuzz*, Example*), sorted, one per line, read
# from the top-level `func` lines of its _test.go files (TestMain is not a test).
split_tests() {
  local prefix=$1 listed="SPLIT_TESTS_$1"
  # --check reads once and passes the list on through the shell variable
  # SPLIT_TESTS_<prefix>.
  if [ -n "${!listed:-}" ]; then echo "${!listed}"; return; fi
  grep -hE '^func (Test|Fuzz|Example)([A-Z0-9_][A-Za-z0-9_]*)?\(' "internal/$(split_dir "$prefix")"/*_test.go \
    | sed -E 's/^func ([A-Za-z0-9_]+)\(.*/\1/' | grep -vx TestMain | sort -u
}

# The same list, from `go test -list`, which compiles the test binary and runs no test.
split_tests_compiled() {
  go test -list '.*' "$module/internal/$(split_dir "$1")" | grep -E '^(Test|Fuzz|Example)[A-Za-z0-9_]*$' | sort
}

# The tests of a split package that run alone in a part of their own: "<test> <part>" lines. Pin a test that takes over about a
# third of the 25-minute -timeout on CockroachDB (hashing it would leave it in a part with a fifth of the package's other tests, and
# it alone fills the part's budget); any other test is hashed over the parts that are not pinned.
pinned_tests() {
  case "$1" in
    leak) echo "TestAHiddenWriteMovesThePlayersNumberAndNothingElse 4" ;;
    *) ;;
  esac
}

# The parts of package $1 that no test is pinned to, one number per line: the tests that are not pinned are hashed over these.
hashed_parts() {
  local prefix=$1 n pname ppart
  local -A taken=()
  while read -r pname ppart; do
    if [ -n "$pname" ]; then taken[$ppart]=1; fi
  done < <(pinned_tests "$prefix")
  for n in $(seq 1 "$(split_parts "$prefix")"); do
    if [ -z "${taken[$n]:-}" ]; then echo "$n"; fi
  done
}

# Part N (1-based) of the tests on stdin, for the split package $2: a pinned test is in its part and in no other; any other test
# is in the (cksum mod number of hashed parts)-th of the parts that are not pinned.
part_of() {
  local n=$1 prefix=$2 name sum pname ppart
  local -A pin=()
  local -a hashed
  while read -r pname ppart; do
    if [ -n "$pname" ]; then pin[$pname]=$ppart; fi
  done < <(pinned_tests "$prefix")
  mapfile -t hashed < <(hashed_parts "$prefix")
  while IFS= read -r name; do
    if [ -n "${pin[$name]:-}" ]; then
      if [ "${pin[$name]}" -eq "$n" ]; then echo "$name"; fi
      continue
    fi
    sum=$(printf '%s' "$name" | cksum | cut -d' ' -f1)
    if [ "${hashed[$((sum % ${#hashed[@]}))]}" -eq "$n" ]; then echo "$name"; fi
  done
}

# The `-run` regex of group <prefix>-<n>.
run_regex() {
  local prefix=$1 n=$2 tests
  tests=$(split_tests "$prefix" | part_of "$n" "$prefix" | paste -sd'|' -)
  if [ -z "$tests" ]; then
    echo "::error::$prefix part $n has no tests" >&2
    return 1
  fi
  echo "^($tests)\$"
}

if [ "${1:-}" = "--run" ]; then
  group=${2:?usage: go-db-shard.sh --run <group>}
  case "$group" in
    play-[1-9]|leak-[1-9]|rules-[1-9])
      # A part number past the package's parts has no tests: fail, do not run nothing.
      if [ "${group#*-}" -gt "$(split_parts "${group%-*}")" ]; then
        echo "::error::unknown group: $group" >&2
        exit 1
      fi
      run_regex "${group%-*}" "${group#*-}" ;;
    *) case_packages "$group" >/dev/null; echo "" ;;
  esac
  exit 0
fi

if [ "${1:-}" = "--verify-scan" ]; then
  for prefix in $split_prefixes; do
    scanned=$(split_tests "$prefix")
    compiled=$(split_tests_compiled "$prefix")
    if [ -z "$scanned" ] || [ "$scanned" != "$compiled" ]; then
      echo "::error::the $prefix tests read from the source are not the tests go test -list prints; fix split_tests in go-db-shard.sh" >&2
      diff <(echo "$compiled") <(echo "$scanned") >&2 || true
      exit 1
    fi
    echo "OK: $(echo "$scanned" | wc -l | tr -d ' ') $prefix tests, the source reading and go test -list agree."
  done
  exit 0
fi

if [ "${1:-}" = "--check" ]; then
  all=$(all_packages)
  named=$(named_packages)
  # A named package that no longer exists (renamed, deleted) would silently
  # drop out of its group and run in "rest" instead: say so.
  missing=$(comm -13 <(echo "$all") <(echo "$named"))
  if [ -n "$missing" ]; then
    echo "::error::go-db-shard.sh names packages that do not exist:" >&2
    echo "$missing" >&2
    exit 1
  fi
  dup=$(echo "$named" | uniq -d)
  if [ -n "$dup" ]; then
    echo "::error::go-db-shard.sh puts a package in two groups:" >&2
    echo "$dup" >&2
    exit 1
  fi
  union=$( { echo "$named"; "$0" rest; } | sort)
  if [ "$union" != "$all" ]; then
    echo "::error::the go-db groups are not exactly 'go list ./...'" >&2
    diff <(echo "$all") <(echo "$union") >&2 || true
    exit 1
  fi
  summary=""
  for prefix in $split_prefixes; do
    parts_count=$(split_parts "$prefix")
    # Every part runs the split package, and nothing else.
    for i in $(seq 1 "$parts_count"); do
      if [ "$(case_packages "$prefix-$i")" != "$(case_packages "$prefix-1")" ]; then
        echo "::error::$prefix-$i must name only the $prefix package" >&2
        exit 1
      fi
    done
    # The parts together are exactly the package's tests, each once.
    tests=$(split_tests "$prefix")
    # A pinned test exists (a renamed or deleted one would silently fall back to a hash), is pinned once, and to a part of the package;
    # the tests that are not pinned have a part left to be hashed over.
    pinned=$(pinned_tests "$prefix")
    while read -r pinned_test pinned_part; do
      [ -z "$pinned_test" ] && continue
      if ! echo "$tests" | grep -qx "$pinned_test"; then
        echo "::error::$prefix pins $pinned_test, which is not a test of the $prefix package" >&2
        exit 1
      fi
      if [ "$pinned_part" -lt 1 ] || [ "$pinned_part" -gt "$parts_count" ]; then
        echo "::error::$prefix pins $pinned_test to part $pinned_part, but has parts 1 to $parts_count" >&2
        exit 1
      fi
    done <<<"$pinned"
    if [ -n "$pinned" ] && [ "$(echo "$pinned" | awk '{print $1}' | sort | uniq -d)" != "" ]; then
      echo "::error::$prefix pins a test twice" >&2
      exit 1
    fi
    if [ -z "$(hashed_parts "$prefix")" ]; then
      echo "::error::every part of $prefix is pinned: no part is left for the other tests" >&2
      exit 1
    fi
    # A shell variable, not an exported one: Linux refuses to start any program
    # whose environment holds a string over 128 KiB (MAX_ARG_STRLEN), and a
    # split package's list can be longer than that.
    printf -v "SPLIT_TESTS_$prefix" '%s' "$tests"
    parts=$(for i in $(seq 1 "$parts_count"); do echo "$tests" | part_of "$i" "$prefix"; done | sort)
    if [ -z "$tests" ] || [ "$parts" != "$tests" ]; then
      echo "::error::the $prefix parts are not exactly the $prefix package's tests" >&2
      diff <(echo "$tests") <(echo "$parts") >&2 || true
      exit 1
    fi
    for i in $(seq 1 "$parts_count"); do run_regex "$prefix" "$i" >/dev/null; done
    summary="$summary; $(echo "$tests" | wc -l | tr -d ' ') $prefix tests, every one in exactly one of $parts_count parts"
    if [ -n "$pinned" ]; then
      sizes=$(for i in $(seq 1 "$parts_count"); do echo "$tests" | part_of "$i" "$prefix" | wc -l | tr -d ' '; done | paste -sd, -)
      summary="$summary ($prefix parts hold $sizes tests; pinned: $(echo "$pinned" | awk '{printf "%s%s to part %s", sep, $1, $2; sep=", "}'))"
    fi
  done
  echo "OK: $(echo "$all" | wc -l | tr -d ' ') packages, every one in exactly one group$summary."
  exit 0
fi

group=${1:?usage: go-db-shard.sh <group>|--run <group>|--check|--verify-scan}
if [ "$group" = rest ]; then
  comm -23 <(all_packages) <(named_packages)
else
  case_packages "$group" | tr ' ' '\n'
fi
