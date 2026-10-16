#!/usr/bin/env bash
# The Go backend's complexity totals may only go down. Measures every
# hand-written, non-test Go file of backend/ and compares the functions over
# each limit with the committed baseline, .github/go-complexity-baseline.txt.
#
#   go-complexity.sh           compare with the baseline (CI runs this)
#   go-complexity.sh --update  rewrite the baseline from the code as it is
#
# Three lists, with the limits of backend/.golangci.yml:
#   cyclomatic       gocyclo v0.6.0: complexity above 15 (higher is worse)
#   cognitive        gocognit v1.2.0: complexity above 20 (higher is worse)
#   maintainability  golangci-lint v2.14.0's maintidx: index below 20 (lower
#                    is worse)
#
# Why it exists: golangci-lint's ratchet judges only the lines a change
# touches, and these linters report on the `func` line, so a change to the
# body of an old function never trips them. This script judges the bodies.
#
# It fails when a function enters a list or its score gets worse, even if the
# count stays the same, and names each one. It also fails when a function got
# better or left a list, and asks for --update, so every improvement is locked
# in. A function is named by its package directory and its name as the tools
# print it ("internal/play (*Service).takeBack"), never by its line, which
# moves with every edit: moving a function to another file of the same package
# changes nothing. Generated files (Go's "// Code generated ... DO NOT EDIT."
# line: backend/gen and the sqlc packages) are left out, as golangci-lint
# leaves them out.
#
# Bash 3.2 compatible (macOS's /bin/bash): no associative arrays, no mapfile.
set -euo pipefail

# The limits: keep them equal to gocyclo.min-complexity and
# gocognit.min-complexity in backend/.golangci.yml. maintidx's (under: 20)
# is read from that file by golangci-lint itself.
cyclo_over=15
cognit_over=20

# Every tool pinned by its module version. gocognit v1.2.0 is the version the
# baseline was measured with; golangci-lint v2.14.0 bundles v1.2.1, whose
# scoring code (gocognit.go) is the same. Keep golangci_version equal to the
# Makefile's and the golangci-lint-action's in .github/workflows/backend.yml.
gocyclo_pkg=github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0
gocognit_pkg=github.com/uudashr/gocognit/cmd/gocognit@v1.2.0
golangci_version=2.14.0

update=false
case "${1:-}" in
  "") ;;
  --update) update=true ;;
  *) echo "usage: go-complexity.sh [--update]" >&2; exit 2 ;;
esac

repo=$(cd "$(dirname "$0")/../.." && pwd)
baseline="$repo/.github/go-complexity-baseline.txt"
cd "$repo/backend"

command -v jq >/dev/null || { echo "::error::go-complexity.sh needs jq" >&2; exit 2; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# The hand-written, non-test Go files, relative to backend/. grep -L lists
# the files WITHOUT the generated line; its exit status only says whether it
# listed any (and differs between grep versions), so the list is what counts.
find . -name testdata -prune -o -name '*.go' ! -name '*_test.go' -print | sed 's|^\./||' | LC_ALL=C sort > "$tmp/all"
xargs grep -L -E '^// Code generated .* DO NOT EDIT\.$' < "$tmp/all" > "$tmp/files" || true
if [ ! -s "$tmp/files" ]; then
  echo "::error::go-complexity.sh found no hand-written Go file under backend/" >&2
  exit 2
fi
files=()
while IFS= read -r f; do files+=("$f"); done < "$tmp/files"

# Without -over, gocyclo and gocognit list every function and exit 0, so a
# non-zero exit is a real failure; the limits are applied below. Each output
# line is "<score> <package> <function> <file:line:column>".
go run "$gocyclo_pkg" "${files[@]}" > "$tmp/gocyclo"
go run "$gocognit_pkg" "${files[@]}" > "$tmp/gocognit"

# maintidx only runs inside golangci-lint: its own command is built on a 2021
# golang.org/x/tools that cannot load today's packages. CI's lint step puts
# the pinned binary on PATH; anywhere else without it, `go run` builds it (a
# minute or two the first time, as for `make lint`).
if command -v golangci-lint >/dev/null && golangci-lint version 2>/dev/null | grep -q "version $golangci_version "; then
  golangci=(golangci-lint)
else
  golangci=(go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$golangci_version")
fi
"${golangci[@]}" run --enable-only=maintidx --tests=false --issues-exit-code=0 \
  --max-issues-per-linter=0 --max-same-issues=0 --show-stats=false \
  --output.json.path="$tmp/maintidx.json" ./...

# One line per function over a limit: "<metric>\t<score>\t<function>\t<file:line>".
# maintidx prints the bare name; the receiver comes from the `func` line it
# quotes, written the way gocyclo writes it ("(*Service).Name").
{
  awk -v metric=cyclomatic -v over="$cyclo_over" 'BEGIN { OFS = "\t" }
    $1 > over { f = $3; for (i = 4; i < NF; i++) f = f " " $i; pos = $NF; sub(/:[0-9]+$/, "", pos); print metric, $1, f, pos }' "$tmp/gocyclo"
  awk -v metric=cognitive -v over="$cognit_over" 'BEGIN { OFS = "\t" }
    $1 > over { f = $3; for (i = 4; i < NF; i++) f = f " " $i; pos = $NF; sub(/:[0-9]+$/, "", pos); print metric, $1, f, pos }' "$tmp/gocognit"
  jq -r '.Issues[]? | select(.FromLinter == "maintidx")
    | ((.Text | capture("Function name: (?<name>[^,]+),.* Maintainability Index: (?<mi>-?[0-9]+)"))
       // error("go-complexity.sh cannot read this maintidx message: \(.Text)")) as $m
    | [(.SourceLines[0] // "") | match("^func \\((?:\\w+ )?(\\*?\\w+)") | .captures[0].string] as $recv
    | ["maintainability", $m.mi, ((if ($recv | length) > 0 then "(" + $recv[0] + ")." else "" end) + $m.name), "\(.Pos.Filename):\(.Pos.Line)"]
    | @tsv' "$tmp/maintidx.json"
} > "$tmp/raw"

# The comparison key is "<metric> <package directory> <function>". Two
# functions with the same key (two init functions in a package) get their
# file's name added, "init@a.go", so neither hides the other. Writes the list
# ("<key> <score>") and, for the messages, where each function is.
: > "$tmp/current"
: > "$tmp/where"
awk -F'\t' -v list="$tmp/current" -v where="$tmp/where" '
  {
    n++; metric[n] = $1; score[n] = $2; fn[n] = $3; pos[n] = $4
    file = $4; sub(/:[0-9]+$/, "", file); dir = file
    if (!sub(/\/[^\/]*$/, "", dir)) dir = "."
    d[n] = dir; f[n] = file; count[$1 " " dir " " $3]++
  }
  END {
    for (i = 1; i <= n; i++) {
      name = fn[i]
      if (count[metric[i] " " d[i] " " name] > 1) { base = f[i]; sub(/.*\//, "", base); name = name "@" base }
      key = metric[i] " " d[i] " " name
      print key " " score[i] > list
      print key "\t" pos[i] > where
    }
  }' "$tmp/raw"
LC_ALL=C sort -o "$tmp/current" "$tmp/current"

summary() {
  for m in cyclomatic cognitive maintainability; do
    now=$(grep -c "^$m " "$tmp/current" || true)
    was=$(grep -c "^$m " "$1" || true)
    echo "  $m: $now functions (baseline $was)"
  done
}

if $update; then
  {
    echo "# The Go functions over the complexity limits, one per line:"
    echo "# <metric> <package directory> <function> <score>. Written by"
    echo "# .github/scripts/go-complexity.sh --update; never edited by hand."
    echo "# cyclomatic (gocyclo): complexity above 15, higher is worse."
    echo "# cognitive (gocognit): complexity above 20, higher is worse."
    echo "# maintainability (maintidx): index below 20, lower is worse."
    echo "# The list may only shrink: CI fails when a function enters it or gets"
    echo "# worse, and when one gets better until this file is written again."
    echo "# See CONTRIBUTING.md, \"Complexity totals\"."
    cat "$tmp/current"
  } > "$tmp/new"
  if [ -f "$baseline" ]; then
    grep -v -e '^#' -e '^$' "$baseline" > "$tmp/old" || true
    echo "Changes to the baseline (- removed, + added):"
    diff "$tmp/old" "$tmp/current" > "$tmp/diff" || true
    if grep -q '^[<>]' "$tmp/diff"; then
      grep '^[<>]' "$tmp/diff" | sed 's/^</  -/; s/^>/  +/'
    else
      echo "  none"
    fi
  else
    : > "$tmp/old"
  fi
  mv "$tmp/new" "$baseline"
  echo "Wrote ${baseline#"$repo"/}:"
  summary "$tmp/old"
  exit 0
fi

if [ ! -f "$baseline" ]; then
  echo "::error::${baseline#"$repo"/} is missing: run .github/scripts/go-complexity.sh --update" >&2
  exit 1
fi
grep -v -e '^#' -e '^$' "$baseline" > "$tmp/base" || true

# Worse: a function that is new to a list, or whose score got worse. Better:
# a function that left a list (split, simplified, renamed or deleted), or
# whose score got better. A renamed function is both: it leaves under its old
# name and enters under the new one, and --update records the rename. Files
# are told apart by name, not ARGIND, which only GNU awk has.
: > "$tmp/worse"
: > "$tmp/better"
awk -v worse="$tmp/worse" -v better="$tmp/better" -v wherefile="$tmp/where" -v basefile="$tmp/base" '
  function keyof(line) { sub(/ [^ ]+$/, "", line); return line }
  function scoreof(line,  parts, n) { n = split(line, parts, " "); return parts[n] + 0 }
  function isworse(k, now, was) { return (k ~ /^maintainability /) ? now < was : now > was }
  FILENAME == wherefile { split($0, w, "\t"); at[w[1]] = w[2]; next }
  FILENAME == basefile { was[keyof($0)] = scoreof($0); next }
  {
    k = keyof($0); now = scoreof($0); seen[k] = 1
    if (!(k in was)) print k "\t" now "\tnew\t" at[k] > worse
    else if (isworse(k, now, was[k])) print k "\t" now "\twas " was[k] "\t" at[k] > worse
    else if (now != was[k]) print k "\t" now "\twas " was[k] > better
  }
  END { for (k in was) if (!(k in seen)) print k "\t-\tleft the list" > better }
' "$tmp/where" "$tmp/base" "$tmp/current"

echo "Functions over the complexity limits (cyclomatic > $cyclo_over, cognitive > $cognit_over, maintainability < 20):"
summary "$tmp/base"

status=0
if [ -s "$tmp/worse" ]; then
  status=1
  echo
  echo "Worse than the baseline: split or simplify these functions (or, if it really cannot"
  echo "be helped, run .github/scripts/go-complexity.sh --update and explain it in the PR):"
  LC_ALL=C sort "$tmp/worse" | while IFS=$'\t' read -r key now why at; do
    echo "  $key $now ($why; $at)"
    # An annotation on the function's line, in the pull request's diff.
    echo "::error file=backend/${at%:*},line=${at##*:}::complexity totals: $key is $now ($why), worse than the baseline"
  done
fi
if [ -s "$tmp/better" ]; then
  status=1
  echo
  echo "Better than the baseline: lock it in with .github/scripts/go-complexity.sh --update"
  echo "and commit .github/go-complexity-baseline.txt with this change:"
  LC_ALL=C sort "$tmp/better" | while IFS=$'\t' read -r key now why; do
    echo "  $key $now ($why)"
  done
  echo "::error::the complexity totals went down: run .github/scripts/go-complexity.sh --update and commit the baseline"
fi
if [ "$status" -eq 0 ]; then
  echo "OK: no function entered a list or changed its score."
fi
exit "$status"
