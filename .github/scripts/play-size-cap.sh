#!/usr/bin/env bash
# The Go code of backend/internal/play may not grow past the number committed in .github/play-size-cap:
# the package is four times the next one, and the count only goes down.
#
#   play-size-cap.sh           check the count against the cap (CI runs this)
#   play-size-cap.sh --lower   write the cap down to the current count, when it is lower
#
# It counts the code lines of every non-test .go file in backend/internal/play itself: the lines that are
# not blank and not only a comment (// or /* */). The subdirectories (link, live, playdb, voice) are not
# counted: moving pure logic into a subpackage is the way down. Every non-test .go file counts, whatever its
# name, its build tags or a "Code generated" header (the package has none: sqlc writes into playdb/), so
# moving code to another file of the package changes nothing. A _test.go file is out: it is not in the binary.
#
# Lowering the cap is always fine: when the count is under it, the check passes and says so, and `--lower`
# (or an edit of the file) records it. Raising it is an edit of .github/play-size-cap that the reviewer sees.
#
# Bash 3.2 compatible (macOS's /bin/bash): no associative arrays, no mapfile.
set -euo pipefail

mode=check
case "${1:-}" in
  "") ;;
  --lower) mode=lower ;;
  *)
    echo "usage: play-size-cap.sh [--lower]" >&2
    exit 2
    ;;
esac

repo=$(cd "$(dirname "$0")/../.." && pwd)
cap_file="$repo/.github/play-size-cap"
cd "$repo/backend/internal/play"

if [ ! -f "$cap_file" ]; then
  echo "::error::$cap_file is missing: it holds the number of code lines the play package may have" >&2
  exit 2
fi
cap=$(tr -d '[:space:]' < "$cap_file")
case "$cap" in
  '' | *[!0-9]*)
    echo "::error::.github/play-size-cap must hold one whole number, found: $cap" >&2
    exit 2
    ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

find . -maxdepth 1 -name '*.go' ! -name '*_test.go' -print | sed 's|^\./||' | LC_ALL=C sort > "$tmp/files"
if [ ! -s "$tmp/files" ]; then
  echo "::error::play-size-cap.sh found no Go file in backend/internal/play" >&2
  exit 2
fi

# One line per file: "<code lines> <file>". A line inside a block comment, a blank line and a line that is
# only a comment are not code; the text after the end of a block comment on the same line is.
: > "$tmp/counts"
while IFS= read -r f; do
  awk -v f="$f" '
    { line = $0; sub(/^[ \t]+/, "", line) }
    inblock {
      i = index(line, "*/")
      if (i > 0) {
        inblock = 0
        rest = substr(line, i + 2)
        if (rest ~ /[^ \t]/ && rest !~ /^[ \t]*\/\//) n++
      }
      next
    }
    line == "" { next }
    line ~ /^\/\// { next }
    line ~ /^\/\*/ {
      i = index(line, "*/")
      if (i == 0) { inblock = 1; next }
      rest = substr(line, i + 2)
      if (rest ~ /[^ \t]/ && rest !~ /^[ \t]*\/\//) n++
      next
    }
    { n++ }
    END { print n + 0, f }
  ' "$f" >> "$tmp/counts"
done < "$tmp/files"
count=$(awk '{ s += $1 } END { print s + 0 }' "$tmp/counts")

echo "code lines in backend/internal/play: $count (cap $cap)"

if [ "$count" -gt "$cap" ]; then
  echo "::error file=.github/play-size-cap::backend/internal/play has $count code lines, over the cap of $cap. Move pure logic to a subpackage in this PR instead of raising the cap; if the growth cannot be avoided, raise .github/play-size-cap in this PR and say why."
  echo "The biggest files (the ones you grew are in your diff):"
  LC_ALL=C sort -rn "$tmp/counts" | head -10
  exit 1
fi

if [ "$count" -lt "$cap" ]; then
  if [ "$mode" = lower ]; then
    echo "$count" > "$cap_file"
    echo "Lowered .github/play-size-cap from $cap to $count."
  else
    echo "::notice file=.github/play-size-cap::The count ($count) is under the cap ($cap): lower .github/play-size-cap to $count (play-size-cap.sh --lower)."
  fi
fi
