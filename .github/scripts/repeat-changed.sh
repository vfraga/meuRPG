#!/usr/bin/env bash
# Runs the tests flake-targets.mjs found, REPEATS times each, inside a time budget (docs/ci.md, "Repeat of the changed tests").
#
#   repeat-changed.sh playwright <targets.json>   from the e2e/ directory, in both colour schemes
#   repeat-changed.sh go <targets.json>           from the backend/ directory, with -race
#
# Each target (one Playwright test, one Go package) is a chunk. A chunk stops at its first failure (--max-failures=1,
# -failfast), so a chunk that is cut off by the budget had no failure up to then: running out of time passes, and the
# job summary says what did not run. A failure fails the script, after the other chunks have run. The summary goes to
# $GITHUB_STEP_SUMMARY (stdout without it).
set -uo pipefail

kind="${1:?playwright or go}"
targets="${2:?the targets.json from flake-targets.mjs}"
repeats="${REPEATS:-10}"
budget="${BUDGET_SECONDS:-900}"
# The limit of /auth/login per client address (backend/internal/identity/login.go, loginClientBurst) and what the
# global setup of every `playwright test` spends of it (3 sign-ins of 2 requests).
LOGIN_BURST=40
SETUP_REQUESTS=6
summary="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
deadline=$(( $(date +%s) + budget ))

# One chunk per line, as compact JSON (a regex keeps its backslashes that way): {label, field, regex, file, signins, serial}.
# For Playwright `field` is the colour scheme and `file` the spec, relative to e2e/; for Go `field` is the package.
case "$kind" in
  playwright)
    mapfile -t chunks < <(jq -c '.playwright[] | . as $t | ("light", "dark") | {label: ($t.title + " (" + . + ")"), field: ., regex: $t.grep, signins: ($t.signins // 0), serial: ($t.serial // false), file: ($t.file | sub("^e2e/"; ""))}' "$targets")
    ;;
  go)
    mapfile -t chunks < <(jq -c '.go | group_by(.pkg)[] | {label: (.[0].pkg + ": " + (map(.test) | join(", "))), field: .[0].pkg, regex: ("^(" + (map(.test) | join("|")) + ")$"), file: ""}' "$targets")
    ;;
  *)
    echo "unknown kind: $kind" >&2
    exit 2
    ;;
esac

passed=(); notrun=(); cut=(); failed=()
left=${#chunks[@]}
spent=0 # login requests since the stack started or was reset
per_run=0
for chunk in "${chunks[@]}"; do
  label=$(jq -r .label <<<"$chunk")
  field=$(jq -r .field <<<"$chunk")
  regex=$(jq -r .regex <<<"$chunk")
  file=$(jq -r .file <<<"$chunk")
  signins=$(jq -r .signins <<<"$chunk")
  serial=$(jq -r '.serial // false' <<<"$chunk")
  now=$(date +%s)
  remaining=$(( deadline - now ))
  if [ "$remaining" -lt "${MIN_SECONDS:-20}" ]; then
    notrun+=("$label")
    left=$(( left - 1 ))
    continue
  fi
  # A fair share: twice the average of what is left, so one slow chunk cannot eat the others' time.
  limit=$(( remaining * 2 / (left > 0 ? left : 1) ))
  [ "$limit" -gt "$remaining" ] && limit=$remaining
  left=$(( left - 1 ))

  # /auth/login lets one address 40 requests at once (20 sign-ins) and refills one every 3 s; a sign-in is 2 requests and
  # the global setup makes 3 of them in every run of `playwright test`. A test that signs in itself, repeated 10 times,
  # can spend the bucket (login.spec.ts "cada pessoa tem uma conta": 3 sign-ins a run, 60 requests in 21 s, a 429 on
  # the 7th run). So a chunk whose sign-ins would not fit runs in batches of repeats, and RESET_STACK_CMD (the workflow
  # recreates the api container: an empty rate-limit bucket) comes before a batch that would not fit what is left.
  batch=$repeats
  if [ "$kind" = playwright ]; then
    per_run=$(( signins * 2 ))
    if [ "$per_run" -gt 0 ] && [ $(( SETUP_REQUESTS + per_run * repeats )) -gt "$LOGIN_BURST" ]; then
      batch=$(( (LOGIN_BURST - SETUP_REQUESTS) / per_run ))
      [ "$batch" -lt 1 ] && batch=1
    fi
  fi
  chunk_deadline=$(( $(date +%s) + limit ))
  todo=$repeats
  code=0
  echo "::group::${label}"
  while [ "$todo" -gt 0 ]; do
    n=$(( todo < batch ? todo : batch ))
    todo=$(( todo - n ))
    left_s=$(( chunk_deadline - $(date +%s) ))
    if [ "$left_s" -lt 1 ]; then code=124; break; fi
    if [ "$kind" = playwright ]; then
      # The requests this batch could make, counted as if nothing refilled: reset only when they would not fit.
      cost=$(( SETUP_REQUESTS + per_run * n ))
      if [ $(( spent + cost )) -gt "$LOGIN_BURST" ] && [ -n "${RESET_STACK_CMD:-}" ]; then
        echo "reset the stack (a fresh login rate-limit bucket)"
        bash -c "$RESET_STACK_CMD" || echo "::warning::the reset failed; the login rate limit may be spent"
        spent=0
        left_s=$(( chunk_deadline - $(date +%s) ))
        if [ "$left_s" -lt 1 ]; then code=124; break; fi
      fi
      spent=$(( spent + cost ))
    fi
    started=$(date +%s)
    if [ "$kind" = playwright ]; then
      # A serial describe shares state between its tests (one account, one upload): its copies must not run in parallel.
      workers=()
      [ "$serial" = true ] && workers=(--workers=1)
      E2E_COLOR_SCHEME="$field" timeout --kill-after=15 "$left_s" \
        npx playwright test "$file" --grep "$regex" --repeat-each="$n" --max-failures=1 ${workers[@]+"${workers[@]}"}
    else
      # GO_SKIP (a -skip regex): on PostgreSQL, the tests that need CockroachDB itself are not repeated.
      skip=()
      [ -n "${GO_SKIP:-}" ] && skip=(-skip "$GO_SKIP")
      timeout --kill-after=15 "$left_s" \
        go test -race -count="$n" -failfast -timeout 30m -run "$regex" ${skip[@]+"${skip[@]}"} "$field"
    fi
    code=$?
    # Only a timeout is "cut off": 124, or 137 once the --kill-after had to kill the run at its time limit. A 137
    # (SIGKILL) before the limit is something else, usually the kernel's out-of-memory kill: a failure.
    if [ "$code" -eq 137 ] && [ $(( $(date +%s) - started )) -ge "$left_s" ]; then code=124; fi
    [ "$code" -ne 0 ] && break
  done
  echo "::endgroup::"
  case "$code" in
    0) passed+=("$label") ;;
    124) cut+=("$label") ;;
    *)
      failed+=("$label")
      # The next chunk's run empties test-results/ and playwright-report/: keep this one's traces and report.
      if [ "$kind" = playwright ]; then
        keep="kept/${#failed[@]}"
        mkdir -p "$keep" && cp -r test-results playwright-report "$keep"/ 2>/dev/null || true
      fi
      ;;
  esac
done

{
  echo "## Repeat of the changed tests (${kind}, ${repeats} runs each, budget ${budget} s)"
  echo
  echo "- Passed: ${#passed[@]}"
  echo "- Failed: ${#failed[@]}"
  echo "- Cut off by the time budget, no failure before it: ${#cut[@]}"
  echo "- Not run, the budget was spent: ${#notrun[@]}"
  for l in "${failed[@]}"; do echo "  - FAILED: ${l}"; done
  for l in "${cut[@]}"; do echo "  - cut off: ${l}"; done
  for l in "${notrun[@]}"; do echo "  - not run: ${l}"; done
} >>"$summary"

if [ "${#failed[@]}" -gt 0 ]; then
  echo "::error::${#failed[@]} changed test(s) failed when repeated (docs/ci.md, \"Flaky tests\"): a flaky test is a bug in the test or the app." >&2
  exit 1
fi
exit 0
