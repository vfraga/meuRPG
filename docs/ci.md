# CI

All code enters through a pull request with green CI; the workflows are in `.github/workflows/`.
This page says what each one checks, which gates only move one way, and how dependencies are pinned and updated.
How to run the same checks locally is in [CONTRIBUTING.md](../CONTRIBUTING.md); the tests themselves are in [Testing](testing.md).

## Workflows

| Workflow | Runs on | Jobs |
| --- | --- | --- |
| `backend` | PRs (all of a train) and pushes to `main` touching `backend/`, `proto/`, `web/`, the `Makefile`, `.node-version` or the Go CI files; nightly; manual | `changes`, `proto`, `go`, `govulncheck`, `go-db` (nine groups plus a gate), `go-coverage`, `report` |
| `web` | `web/`, `.node-version`, the workflow; every pull request of a train | `test and build`, `coverage` |
| `e2e` | `backend/`, `proto/`, `web/`, `e2e/`, `deploy/local/`, the `Makefile`, `.node-version`, the workflow; never for a Markdown-only change, except in a train; nightly; manual | `plan`, `lint and typecheck`, `images`, `database`, `playwright` shards, `report` |
| `codeql` | PRs (all of a train) touching `backend/`, `web/src/`, `e2e/` or workflows; every push to `main`; weekly | Go (built from `backend/`), JavaScript/TypeScript and GitHub Actions, with the `security-extended` queries; findings go to the Security tab |
| `osv-scan` | Every pull request of a train; changes to a lockfile (`backend/go.mod`, `backend/go.sum`, the `package-lock.json` of `web/`, `e2e/` and `e2e/lint/`), `osv-scanner.toml` or the workflow; pushes to `main`; nightly; manual | `osv-scanner (lockfiles)`, `report` |
| `workflows-lint` | Changes under `.github/` or to `e2e/lint/package-lock.json`; every pull request of a train | `actionlint` and `zizmor`, at versions pinned by number and sha256; the unit tests of the CI scripts (`node --test`) |
| `repeat-changed` | PRs touching `e2e/`, `backend/` or its own files | `targets`, `images`, `playwright`, `go`: [the repeat of the changed tests](#repeat-of-the-changed-tests) |
| `nightly-hunt` | Nightly, only in `PuraFome/meuRPG`; manual | `plan`, `image`, `playwright` shards, `go` groups, `report`: [the nightly hunt](#the-nightly-hunt) |
| `flake-check` | Manual only | `plan`, `images`, `playwright` or `go` runners, `summary`: [flake-check](#flake-check) |

- A new run of the same PR cancels the previous one. A push to `main` never cancels a run in progress, but GitHub keeps only one waiting run per workflow, so in a burst of merges an intermediate commit may get no run of its own. Deploy only a commit whose own run passed.
- Every job has a `timeout-minutes`, so a stuck test cannot burn the six-hour default.
- **When a run outside a pull request fails** (`main` or the nightly), the `report` job of `backend`, `e2e` and `osv-scan` opens a GitHub issue labelled `ci-red`, or comments on the one it opened: each job finds its own issue by its exact title (`CI vermelho: <workflow> na main`) and never writes into another one.
  All of them, and the nightly hunt, run `.github/scripts/ci-red-report.sh` (its tests: `ci-red-report.test.mjs`).
  It runs only in `PuraFome/meuRPG`: a fork has no issues to report to.

### What runs when

The `Runs on` column above is what each workflow judges. Whether it runs for an event is decided in the jobs, not by a `paths` filter on the pull request, so that a train can run everything:

| Where and what | Runs |
| --- | --- |
| `PuraFome/meuRPG`, any trigger | Everything the workflow lists: pushes to `main`, the schedules, pull requests, manual runs |
| Any other repository (the fork `vfraga/meuRPG`) | Only `pull_request` and `workflow_dispatch`. A push to the fork (a train branch pushed there for `flake-check`, or a sync of `main`) and a schedule start nothing: every job is skipped by a job-level `if` |
| A pull request from a `refactor/train-*` branch of the same repository | The full gate, whatever it changes (a docs-only train included): `e2e` runs every `@a11y` shard, `backend` every `go-db` group and the Go jobs, and `web`, `workflows-lint`, `osv-scan` and `codeql` run |
| Any other pull request | The workflows whose files it changes (the `Runs on` column), and within `backend` and `e2e` the Go jobs and the `@a11y` shards only when it touches what they judge. The fork's draft pull requests, the preflights, rely on this |
| Nightly and manual runs | Everything, with no path selection |

A branch named `refactor/train-*` in someone else's fork is not a train: the rule compares the head repository of the pull request with the base one, and that pull request is judged by its files like any other.

The `changes`, `plan` and `select` jobs make the choice.
The rule lives in one expression, `DIFF`, in the `env` of each of those jobs (`backend`, `codeql`, `e2e`, `osv-scan`, `web` and `workflows-lint`): it is `true` only for a pull request that is not a train.
It is copied because a local composite action needs a checkout first; `select-workflows.test.mjs` compares the six copies exactly.
When `DIFF` is `false` (a push to `main`, the nightly, a manual run, a train) a step answers that everything is selected, with no checkout.
When it is `true` the job checks out the whole history and calls the composite action `.github/actions/select`, which runs `.github/scripts/gate-select.sh` (its tests: `gate-select.test.mjs`) to match the changed files.
A pull request that touches no file of a workflow still starts it, and its jobs are skipped.
`nightly-hunt` runs only in `PuraFome/meuRPG`. `flake-check` is manual only. `repeat-changed` keeps a `paths` filter, train or not: with no test changed there is nothing to repeat. All three are described below.

## Backend

| Job | Checks |
| --- | --- |
| `proto` | `buf lint`, `buf format`, `buf breaking` against `main`, and that `buf generate` and `make sqlc` leave the committed generated code unchanged |
| `go (lint)` | `golangci-lint` as a ratchet, the [complexity limits](#complexity-limits) and the [`//nolint` cap](#the-nolint-cap). It runs no `go test`: `go-db` runs every package |
| `govulncheck (called vulnerabilities)` | `govulncheck ./...` (version pinned): fails on a known vulnerability in a module or in the Go standard library that the code actually calls. See [Vulnerability scans](#vulnerability-scans) |
| `go-db <group>` | `go test -race -covermode=atomic -coverpkg=./...` for one group of packages against its own in-memory database, with `MEURPG_REQUIRE_DB=1`: [PostgreSQL or CockroachDB, by the event](#which-database-the-tests-run-on). Each package has a 25-minute `-timeout` inside the job's 30 |
| `go-db cockroach-only list` | Checks the [list of the tests that need CockroachDB](#which-database-the-tests-run-on): every listed test exists and, on PostgreSQL, fails |
| `go (integration tests against CockroachDB)` | Waits for every group and for the list check; the stable check for a branch protection rule. The name is stable for a branch protection rule; a preflight runs it on PostgreSQL |
| `go (coverage)` | Merges the groups' profiles and enforces the [coverage floor](#coverage-floors) |

**The groups** are defined in `.github/scripts/go-db-shard.sh`: `play-1` to `play-3`, `maps`, `characters`, `pure`, `leak`, `camp` and `rest`.
The heavy packages are named by hand and `rest` holds everything else, so a new package lands in `rest` and is never skipped.
The `play` package is split by test: each part runs the tests whose name hashes (`cksum`) to it, so a new test lands in a part by itself and adding one never moves the others.
`go-db-shard.sh --check` (run by every group) fails unless the groups together are exactly `go list ./...` and the play parts together are exactly the tests `go test -list` prints.
If a group nears its timeout, move a package between groups in that script.
The architecture test (`internal/archtest`) runs in `rest`.

### Which database the tests run on

The backend tests run on CockroachDB, the production database, wherever a result decides a merge.
A preflight (a pull request that is not a train) runs them on PostgreSQL 17, so the sessions get fast feedback.

| Run | Database |
| --- | --- |
| A pull request from a `refactor/train-*` branch of the same repository | CockroachDB |
| A push to `main`, the nightly run, a manual run | CockroachDB |
| Any other pull request (a preflight) | PostgreSQL 17: `default_transaction_isolation=serializable`, `fsync`, `synchronous_commit` and `full_page_writes` off, data in memory |
| `repeat-changed` | The same as the `go-db` jobs of that pull request |
| `flake-check` | CockroachDB: it is dispatched on a train branch |

The composite action `.github/actions/test-database` is the one place that decides it, from the event and the head ref, and starts the database.
The workflows read its `database` and `url` outputs and never test the event themselves.
It applies the train rule of the [table above](#what-runs-when): a branch named like a train in someone else's fork is not one.

**The tests that need CockroachDB itself** are listed in `backend/cockroach-only-tests.txt`, one per line with the reason.
A PostgreSQL run leaves them out by name (`go test -skip`) and prints the list at the start of the step; the CockroachDB runs run every test.
Two checks keep the list true (`.github/scripts/cockroach-only.sh`):

- `check-names` runs in every group: each entry is a top-level test of its package, and no other package has a test with that name.
- `check-list` runs in the job `go-db cockroach-only list` on PostgreSQL: every listed test must fail there.
  A test that passes on PostgreSQL does not need CockroachDB, and leaves the list.

A test that needs CockroachDB and is not on the list fails in the PostgreSQL run: add it with its reason, or write it so it runs on both databases.
`repeat-changed` leaves the listed tests out of its repeats on PostgreSQL.

## Go linters and the ratchet

New findings fail the build; old ones do not.
`backend/.golangci.yml` enables more linters than the existing code satisfies, so CI judges only the lines a change adds: on a pull request, the diff against the merge base (`--new-from-merge-base=origin/<base>`); on a push to `main`, the pushed commits (`--new-from-rev=<before>`).
The job checks out the whole history for that.
Fixing old findings is welcome and never required.

- `make lint` runs `buf lint`, `buf format` and `golangci-lint` (the version in the `Makefile`, which must equal the `golangci-lint-action` in `backend.yml` and `golangci_version` in `.github/scripts/go-complexity.sh`) as the same ratchet, against `origin/main` (`git fetch origin main` first; `LINT_BASE=<ref>` changes the base).
- `make lint-all` runs them on the whole backend with every finding, to pick what to clean up next.
- The linters, their settings and the reason for each one are in `backend/.golangci.yml`. Highlights: `sloglint` (a fixed message per call site, `snake_case` keys; see [Architecture](architecture/logs.md#logs)), `nolintlint` (every `//nolint` names its linter, says why and still suppresses something), `gocyclo` and `gocognit` at 15, `maintidx` at 20, `mnd` (a bare number becomes a named `const` with a comment) and `goconst`.
- `wrapcheck`, `gochecknoglobals`, `funlen`, `nilnil` and `exhaustive` are off on purpose. `wrapcheck` is off because authorization errors are `*connect.Error` and stay unwrapped for `rpclog`'s type switch.

### Complexity limits

No hand-written Go function of `backend/` may pass a complexity limit: cyclomatic complexity above 15 (gocyclo), cognitive complexity above 15 (gocognit) or a maintainability index below 20 (golangci-lint's `maintidx`).
The ratchet above judges a function only when its `func` line changes, so `.github/scripts/go-complexity.sh` measures every hand-written, non-test Go file (generated files are left out) and fails on any function over a limit, naming it with its file and line.
It needs `jq` and runs in seconds.

- A function over a limit is split by what it does: a check, a read, a write, the reply. Freeze its behaviour first with a test that pins its answers, errors and writes (see [Testing](testing.md)), then split it.
- A `//nolint` does not help: gocyclo and gocognit run outside golangci-lint.
- The limits are the same as `gocyclo`, `gocognit` and `maintidx` in `backend/.golangci.yml`; change both together.

### The `//nolint` cap

A `//nolint` hides a finding, and `nolintlint` only makes each one name its linter and say why. `.github/scripts/nolint-cap.sh` counts them: every `//nolint` directive in the hand-written Go files of `backend/` (tests included; generated files and `testdata/` left out) may not pass the number in `.github/nolint-cap`, and the `go` job fails when it does.
It needs only `grep`, and runs in a second.

- A lower count always passes. The step then prints a notice; `.github/scripts/nolint-cap.sh --lower` writes the cap down to the count (it never raises it), so the number follows the work that removes suppressions.
- To go up, edit `.github/nolint-cap` in the same PR and say in the description why the suppression cannot be avoided: the reviewer sees the number change. Fixing the finding is the default.
- The count is the whole backend's, so lower the number in the PR that removes suppressions: otherwise the room they leave could be used by a new one.

### Coverage floors

`.github/scripts/go-coverage.sh` merges the groups' profiles (a block is covered if any group ran it), prints the total and a per-package table in the job summary, and fails if a package in `FLOOR_PACKAGES` is under `FLOOR_PERCENT` (90).
The floor packages are the pure rules packages: `rules`, `combat`, `grid`, `vision`, `dungeon`, `encounter` and `puzzle`.
Other packages are reported without a floor.
Each group uploads its profile as the artifact `go-cover-<group>`.
Locally:

```bash
cd backend
go test -covermode=atomic -coverpkg=./... -coverprofile=c.out ./internal/rules/...
../.github/scripts/go-coverage.sh c.out
```

The web floors are `coverageThresholds` in `web/angular.json` (lines, statements, branches and functions).
The `web / coverage` job is blocking: raise the floors when the numbers rise, never lower them to get green.
The report goes to the job summary and the `web-coverage` artifact.

## Web and e2e linters

The web and e2e linters use the same ratchet: new violations fail, old ones are a recorded backlog.
ESLint's bulk suppressions list the old violations as a count per file and rule in `web/eslint-suppressions.json` and `e2e/eslint-suppressions.json`, so CI fails only when a file gets a violation it did not have.

- Never grow a suppressions file. Fix the new violation; if it is really unavoidable, write `// eslint-disable-next-line <rule> -- why` on that line and explain it in the PR.
- When you touch a suppressed spot, fix it, run `npx eslint --prune-suppressions` and commit the smaller file.
- `eslint --suppress-rule <rule>` is only for turning on a new rule: enable it, record what exists, fix it over time.
- `complexity` and `max-lines` have no exceptions: the `web` job fails when `web/eslint-suppressions.json` lists either rule or a file disables one inline. Split the function, or split the file by what it does (pure builders next to the component, a plain class for shared state).
- `web/eslint.config.js`: typescript-eslint with the type-aware promise rules and no `any`; angular-eslint with the template and accessibility rules, OnPush, standalone components and `inject()`; `complexity` 15 and `max-depth` 4; no app file over 400 lines of code (`max-lines`, the Angular style guide's limit; blank lines and comments do not count, specs are exempt); no magic numbers in app code (-1, 0, 1, 2, the radix 10, 100 and 1000, default values, enum members, readonly fields and array indexes are allowed; specs are exempt); a `setTimeout(` in a `*.spec.ts` is forbidden (see [Testing](testing.md#web-unit-tests)); and no `.replace(/ /g, ' ')` (a space replaced by itself: write a non-breaking space as ` `, which CodeQL also flags as `js/identity-replacement`).
- **Layers and imports (dependency-cruiser).** `cd web && npm run depcruise` applies `web/.dependency-cruiser.cjs` to `web/src/app` (specs and `src/gen` are not cruised): no import cycles; `core/` imports neither `pages/` nor `shell/`; `shared/` does not import `pages/`; a page (a direct child folder of `pages/`) does not import another page (its own sub-folders are fine). Type-only imports count. `web/.dependency-cruiser-known-violations.json` is empty and must stay empty: move what two layers or pages share down to `core/` or `shared/`. dependency-cruiser is a dev dependency used only by this check.
- **Formatting.** `cd web && npm run format:check` (Prettier, `web/.prettierrc`) checks TypeScript, SCSS and JSON. Angular templates are left out, because Prettier's HTML printer changes whitespace between elements and text, which changes what renders when CSS makes an element inline; format templates by hand. `.git-blame-ignore-revs` lists the one formatting commit (`git config blame.ignoreRevsFile .git-blame-ignore-revs`, once, locally).
- **e2e.** `cd e2e && npm run lint:install && npm run lint` (see [Testing](testing.md#end-to-end-tests-playwright)). `expect-expect` is off in `e2e/`: the tests assert through helpers. `meurpg/rpc-revision-wait` is an error with no baseline (a helper that changes the combat is followed by a wait for the page to show it; see [Testing](testing.md#end-to-end-tests-playwright)), and the job runs `npm run lint:test`, which proves the rule on a bad and a fixed fixture. `test.skip` and `test.fixme` are errors (`playwright/no-skipped-test` with `disallowFixme`), and the `e2e / lint and typecheck` job runs `.github/scripts/e2e-no-retries.sh`, which fails unless `retries` is set once, to 0, in `e2e/playwright.config.ts` and nowhere else (no `--retries`, no `test.describe.configure({ retries })`).

### The web bundle budget

`web/angular.json` fails the production build when the initial chunks pass 500 kB (the "Initial total" line, raw size) and warns from 490 kB; a component's styles warn from 4 kB and fail from 8 kB.
What a first screen needs stays in the initial chunks (Angular, the router, protobuf-es and Connect, the shell); everything else is a lazy route, a `@defer` or a dynamic `import()`.
To see what grew, `cd web && npx ng build --stats-json` writes `dist/web/stats.json` (esbuild's metafile): follow the `imports` of `src/main.ts` to the module.
A new import in `app.ts`, `app.config.ts`, `app.routes.ts` or `shell/` is the usual cause, in particular a generated `*_pb` file or `@angular/cdk/overlay`.

## Web

`web / test and build` runs, in `web/`: `npm ci --ignore-scripts`, `npm run lint`, `npm run format:check`, `npm run depcruise`, `npm test` and `npm run build`.
`web / coverage` runs `ng test --coverage` against the floors in its own job so it cannot cancel or skip the build.

## End to end

The `e2e` workflow runs the Playwright suite against the local stack.

- `plan` chooses the shards: three plain shards (`--grep-invert @a11y`) always, and six accessibility shards (`--grep @a11y`) on `main`, nightly, manual runs, and pull requests that touch `web/`, `e2e/`, `deploy/local/`, the workflow, the split script or its reporter.
- `lint and typecheck` needs no Docker, so a mistake fails in about a minute.
- `images` builds the production image and the devidp image once (with the GitHub Actions layer cache) and hands both over as an artifact; each shard runs `docker load`, so a broken Dockerfile fails before any shard.
- `database` migrates an empty CockroachDB once, beside `images`: it builds `migrate` from the checkout, runs it against the in-memory database of `compose.ci.yaml`, and backs the database up (`BACKUP DATABASE`, to the folder `compose.ci-db.yaml` mounts) as the artifact `e2e-database`.
  The backup holds the state right after the migrations, seed rows included.
  It runs beside `images`, not inside it, so neither waits for the other.
  It builds `migrate` itself because the one in the image exists only after `images` has built it, which would put the whole migration after the image build.
- Every shard but the one the plan marks `fresh` (plain 1/3) restores that backup (`RESTORE DATABASE`) instead of running the migrations; `migrate` then finds every migration applied and changes nothing.
  The `fresh` shard starts from an empty database and runs every migration, so a broken migration still fails `e2e`.
  It waits for `database` but runs even when that job failed; the shards that restore then fail at the download of `e2e-database`.
  The other workflows that start the stack (`repeat-changed`, `flake-check`, `nightly-hunt`) migrate from an empty database.
- Each shard runs a list of tests balanced by measured time, not Playwright's own `--shard`, which counts tests.
  `e2e-shards.mjs list <n>/<count> <grep flags>` (in `.github/scripts`) splits the tests the flags select, and the shard runs its list with `--test-list`.
  The split uses the durations in `e2e/timings.json`, so the shards of a group finish together.
  The counts (`PLAIN_SHARDS`, `A11Y_SHARDS`) and the fresh shard (`FRESH_SHARD`) are the workflow's `env`: the `plan` matrix and the check in `lint and typecheck` read the same values.
  A serial group (`test.describe.configure({ mode: 'serial' })`, in a describe or at the top of a file) runs whole in one shard, so its tests need not be independent.
  `e2e-list-reporter.mjs` finds the groups.
  `check` fails when a group is in two lists, when the flags select only part of a group (a tag on one of its tests), and when it finds no group at all.
  The fresh shard (`--fresh-shard=<n>`) gets less test time, because its migrations and the devidp check make its setup slower.
  Its extra setup time is the entry `(extra setup of the fresh shard)` of `e2e/timings.json`.
  The durations are summed over a shard's workers, so the entry is the extra wall-clock seconds times the workers of the run (Playwright's default is half the CPUs).
  A test the file does not know runs too, in the shard with the least time, and the step prints a notice with how many are missing.
  The `setup` project runs ahead of every shard, so it is in one list and weighs nothing.
  `lint and typecheck` runs `e2e-shards.mjs check`, which fails unless every test is in exactly one list, read back through Playwright's `--test-list`.
  A change to the split script or the reporter runs all nine shards.
  `nightly-hunt` keeps Playwright's own `--shard`.
- Refresh `e2e/timings.json` when the notice counts missing tests, or when the shards stop finishing together.
  Every shard of a green run uploads its timing as the artifact `e2e-timing-<n>`: the Playwright report and `e2e-setup.json`, how long the shard's setup took and how many workers the run uses.
  The upload never turns a shard red when its tests passed.
  `timings` refuses a run without the setup file of the fresh shard or of the others, and a run in which the fresh shard stalled during setup: use another run.
  Download the artifacts of a run of all nine shards and run:

  ```bash
  gh run download <run id> --pattern 'e2e-timing-*' --dir /tmp/e2e-timing
  node .github/scripts/e2e-shards.mjs timings /tmp/e2e-timing/*/*.json > e2e/timings.json
  ```

- Each shard starts the stack in the background (the composite action `start-e2e-stack`) while it installs Node, the e2e dependencies and Chromium, which the stack does not need.
  The stack starts from `compose.yaml`, `compose.ci.yaml` (it keeps the database in memory and lets it BACKUP and RESTORE to a folder), `compose.ci-images.yaml` (it points at the loaded images) and, when the database is restored, `compose.ci-db.yaml` (it mounts the folder of the backup).
  The action returns that list as `compose-file` and the folder as `database-dir`; a later step that runs `docker compose` sets them as `COMPOSE_FILE` and `E2E_DB_DIR`, so it never uses fewer files than the start did.
  The last step of the action waits for it, and a stack that fails to start fails the job with its log.
  One shard then checks that the production image does not contain the devidp, and every shard runs the tests on Chromium.
  A failed shard shows the stack logs and keeps them (`stack-logs/stack.log`, with timestamps) with its Playwright report in the artifact `playwright-report-<n>`.

## Vulnerability scans

Two scans look for known vulnerabilities, and they answer different questions.

- **`govulncheck`** (a job of `backend`, so it runs with the other Go jobs: on pull requests that touch Go, on pushes to `main` and nightly) reports only the vulnerabilities in code the backend **calls**, in a module or in the standard library of the Go version in `go.mod`. Any finding fails the job. The fix is to bump the module (or the Go version).
- **`osv-scan`** (its own workflow) reads the lockfiles (`backend/go.mod`, `web/package-lock.json`, `e2e/package-lock.json`, `e2e/lint/package-lock.json`) and lists every vulnerable **version** installed, called or not, dev-only or not. Any finding fails the job. It reads `go.mod` because OSV-Scanner does not support `go.sum`. The nightly run exists because a vulnerability is published against a version we already have, with no change on our side. OSV-Scanner is a release binary pinned by version and sha256, like the tools of `workflows-lint`.

A finding is fixed by a dependency bump. When the parent pins the package exactly, or the parent's range has no fixed version, `overrides` in `web/package.json` or `e2e/lint/package.json` force a fixed one (the ones there now are listed below); remove an override once the parent ships the fix, and regenerate the lockfile with `npm install`, never by hand. An override that crosses a major version is kept only while lint, format, depcruise, tests and the build still pass with it. Only a finding with no installable fix goes in `osv-scanner.toml`, with its `id`, a `reason` and an `ignoreUntil` date, so the finding returns for a decision when the date passes. Nothing is accepted today.

`web/package.json` overrides: `vite` and `undici` under `@angular/build`; `@modelcontextprotocol/sdk` and `zod` under `@angular/cli`; `undici` under `jsdom`; `minimatch` `^11.0.1` under `eslint`, `@eslint/config-array`, `@typescript-eslint/typescript-estree`, `glob`, `ignore-walk` and `@tufjs/models`; `glob` `^14.0.2` under `@npmcli/package-json` and `cacache`.

`e2e/lint/package.json` overrides: `minimatch` `^11.0.1` under `eslint`, `@eslint/config-array` and `@typescript-eslint/typescript-estree`.

## Pinning and dependencies

- **Every GitHub action is pinned by commit SHA**, not by tag: whoever controls an action can move a tag to malicious code, but cannot change a SHA. The `# vX.Y.Z` comment is for humans.
- **Every job's machine is pinned to an Ubuntu version** (`runs-on: ubuntu-26.04`), never `ubuntu-latest`. Changing it is done in every job at once, tested in a PR first.
- **Dockerfiles pin their images by digest**, and must match `backend/go.mod` (Go) and `.node-version` (Node).
- **npm installs never run third-party install scripts** (`npm ci --ignore-scripts`).
- **Workflow permissions** are `contents: read`; a job that needs more (`report`) asks for it by name.

Dependabot (`.github/dependabot.yml`) opens weekly PRs for the actions, the Go modules, the npm packages of `web/`, `e2e/` and `e2e/lint/`, the Docker base images and the CockroachDB image of `compose.yaml`.
Each new version waits 7 days (`cooldown`), so a compromised release can be pulled before it reaches us; a security update skips the wait.
Minor and patch updates arrive together in one PR per group.
Majors that need a deliberate step are ignored by Dependabot and done by hand:

- a major of Angular, TypeScript, vitest or angular-eslint in `web/`, which comes with `ng update` and its migrations (the builder accepts one major at a time);
- a major of `@types/node`, which follows the Node version the code runs on;
- a new Node or Go minor in the images, which changes together with `.node-version` and `go.mod`;
- a new CockroachDB release line: only "Regular" lines (26.2, 26.4...), never "Innovation" ones, which have no long-term support. It changes `compose.yaml`, `compose.ci.yaml`, `COCKROACH_IMAGE` in `backend.yml`, the `Makefile` and the Homebrew formula together.

A CockroachDB patch PR only changes `compose.yaml`: change `COCKROACH_IMAGE` and the `image` of `deploy/local/compose.ci.yaml` in the same PR, because Dependabot ignores the copy in `compose.ci.yaml` (it has another name there).
Those two pull the same digest from Cockroach Labs' registry on Google Cloud, and the e2e image build pulls every Docker Hub image through Google's mirror (`mirror.gcr.io`), because GitHub's runners share Docker Hub's anonymous pull limit.

## Reading a failure

Never rerun first. Read the annotations, the logs, the Playwright report and the trace, decide whether the application, the test or the infrastructure failed, fix it, and rerun only as confirmation.
Before merging, read CodeQL's comments on the PR and the open code-scanning alerts: a green check ignores warnings.

## Flaky tests

A red test is a bug in the test or in the app until the evidence says which. It is never noise, and nothing here lets a flaky test through: no retries, no quarantine (`test.skip`, `test.fixme`, a skipped Go test), and no "the train did not touch that test".

### When a flaky test stops holding a train

A flaky test that fails a train's gate holds that train until the test is **acknowledged**. It is acknowledged when all four are true:

1. **Its artifacts were read.** For Playwright, `error-context.md`, the trace (`npx playwright trace actions <trace.zip>`, then `npx playwright trace action <trace.zip> <n>` for the step that failed) and the screenshots; for Go, the log and the stack of the panic.
2. **A `flake-check` run shows it fails without the train, and that the train does not make it fail more often.** The run compares the head (with the train) and the base (without it) on the same test, many times; see [flake-check](#flake-check).
3. **It has an owner:** a person or a branch that is fixing it, named on the issue.
4. **Its fix goes in the next train.**

Until all four are true the train waits. When they are, the train may merge with the failure on the record, and the failure is a `ci-red` issue with its owner, not a rerun.

### Banned fixes

The train review looks for all of these; the lint catches the skips (`test.skip`, `test.fixme`) and the retries check catches retries:

- retries (`retries` in the config, `--retries`, a rerun of a job until it is green, gotestsum's `--rerun-fails`);
- `test.fixme`, `test.skip`, or a skipped Go test (`t.Skip` to dodge a failure);
- a longer timeout;
- a fixed sleep or `waitForTimeout`;
- a looser assertion.

The fix is in the test (wait for the condition that matters, own the state it needs) or in the app (a page that kept a stale state, a request canceled early, an answer that arrived after the click). When it is the app, the fix carries a unit test that fails without it. How to reproduce a flake and what counts as fixed are in [Testing](testing.md#reproducing-a-flaky-test).

### flake-check

`flake-check` answers two questions about one test: does it fail without the train, and does the train make it fail more often? It is dispatched by hand and runs on the fork (`vfraga/meuRPG`), so it never takes the gate's runners. The fork holds a train branch only once it is pushed there: run `git push fork refactor/train-<n>` before the dispatch.

Dispatch it from the fork's Actions tab (workflow `flake-check`, "Run workflow") or with `gh workflow run flake-check.yml --repo vfraga/meuRPG --ref <train branch> -f kind=playwright -f test='<title>'`. `--ref` names the train branch: it is the head, and the run's caches are scoped to that branch. The base is always the repository's default branch (`main`). There are no ref inputs: the run never checks out a ref it was given as text, so a dispatch cannot run code from a ref of its choosing in the default branch's context.

| Input | Meaning |
| --- | --- |
| `kind` | `playwright` or `go` |
| `test` | Playwright: the `--grep` (a title, read as a regular expression: escape `(`, `.`, `+`, `?`). Go: the `-run` regular expression |
| `package` | Go only: the package relative to `backend/`, for example `./internal/play` |
| `runs` | Runs per side, default 30. Rounded up to a multiple of `runners` |
| `runners` | Runners per side, default 5 (each does `runs / runners` repeats) |

Each side builds from its own ref (Playwright builds that ref's images and stack; Go starts CockroachDB as the `go-db` jobs do and runs with `-race`), on runners of its own: a matrix of side by runner. The last job writes, to the job summary and to the `flake-check` artifact (`flake-check.json`), the passes and failures of each side, the failure rate, and the first error line of each failure (its signature, with ids, addresses and times taken out, so one flake counts as one signature).

Read it like this:

- **The head fails more often than the base:** the summary says to dispatch again with `runs=100`. A few failures in 30 runs can be chance; a rate that holds at 100 is the train's doing. Find what the train changed.
- **The head does not fail more often, and the base fails:** the flake is there without the train. That is evidence for point 2 of the rule above; the owner and the fix go on the issue.
- **Neither fails:** it did not reproduce. Read its artifacts again and add diagnostics that will explain its next occurrence; never guess a fix.
- **"Incomplete":** a runner died before it reported (the summary job then fails). Read the failed jobs and dispatch again.
- **"The side ran no test":** the test does not exist on that ref, or the title matches nothing there.

`flake-check` runs refs that are not `main`'s, so it writes no cache (no `cache:` on `setup-node` or `setup-go`, no cache save, no `cache-to` on the image builds): a ref under test cannot poison a cache that `main`'s runs restore.

The traces, the HTML report and the stack logs (`stack-logs/stack.log`, saved on every run) are kept as the artifacts `flake-traces-<side>-<runner>` (Playwright) and the log as `flake-log-<side>-<runner>` (Go).

### Repeat of the changed tests

The `repeat-changed` job runs next to the gate on every pull request that touches `e2e/` or `backend/`. A new or edited test that is flaky fails here, before it is merged.

`.github/scripts/flake-targets.mjs` maps the diff against the base to the tests to repeat:

- the Playwright `test(...)` blocks and the Go `func TestX` whose body changed (a change outside any test maps to nothing, except one in a `test.beforeEach`, `test.beforeAll`, `test.afterEach` or `test.afterAll`, which maps to a sample of up to 5 of the tests the hook reaches);
- for a changed helper, a sample of up to 5 of the tests that call it, directly or through other helpers: an exported function of an `e2e/tests/*-support.ts`, or a function or method of `backend/internal/leaktest`.

`.github/scripts/repeat-changed.sh` runs them within a time budget (`BUDGET_SECONDS`, 900 in the workflow, so the job ends about when the gate does): Playwright with `--grep` (the title, escaped) on its spec file and `--repeat-each=10`, once in each colour scheme (`E2E_COLOR_SCHEME=light|dark`, which the page and the contexts made with no `colorScheme` option follow; a context made with `newSignedInContext` follows it unless the test names a scheme of its own (an option left `undefined` counts as not named), and a test that names its theme in its title is covered by that title); Go with `-run`, `-count=10` and `-race`. Each test or package is a chunk that stops at its first failure. All the Playwright chunks share one stack and one client address, and `/auth/login` allows that address 40 requests at once (20 sign-ins) and one more every 3 seconds. Each `playwright test` spends 6 of them in the global setup, and a test that signs in itself spends 2 per sign-in, per run: ten runs of a test with three of them would be 66 requests and get a 429. So a chunk whose sign-ins do not fit runs its repeats in batches. `flake-targets.mjs` reads the specs with the TypeScript compiler API and gives a test a `signins` count of at least 3 when its body, a hook (`test.beforeEach`, `beforeAll`, `afterEach` or `afterAll`) that runs for it (one in its own `test.describe`, or outside every group) or a helper that calls `signIn` (through up to three levels) signs in, or it opens `/auth/login` itself; a sign-in passed by reference (`xs.map(loginAs)`) is not seen. The batches only work because the workflow gives each one a fresh bucket: it recreates the `api` container (the limit lives in that container's memory) before any run that would not fit what is left of the 40 requests, counted as if nothing refilled. The traces and the report of a failed chunk are kept in `e2e/kept/` for the artifact. A failure fails the job. Running out of time passes, and the job summary lists what was cut off and what did not run.

The tests of a serial `test.describe` share state, such as the one package upload of an account.
A describe is serial with `test.describe.serial(...)`, or with `test.describe.configure({ mode: 'serial' })` in it, in a group around it or outside every group.
`flake-targets.mjs` marks the target of such a test `serial`, and `repeat-changed.sh` runs its copies one at a time.
The sign-in batches and the time budget apply as for any other target.

The summary of `targets` lists the tests it found and why (`changed`, `calls <helper>` or `reached by a changed hook`). The scripts have unit tests, run by `workflows-lint` (`node --test .github/scripts/*.test.mjs`).

### The nightly hunt

`nightly-hunt` runs in `PuraFome/meuRPG` only, at 04:30 in São Paulo, and looks for the test that fails only in another order, more than once, or under the race detector:

- **Playwright:** the whole suite (the nine shards of `e2e`) with `--shuffle <seed>`. The seed is printed in the `plan` job and in the summary, and goes in the issue. To replay an order, dispatch the workflow with that seed (`seed` input) or run `cd e2e && npx playwright test <the shard's flags> --shuffle <seed>` against a local stack.
- **Go:** the `go-db` groups with `-shuffle=on -count=3 -race`, through `gotestsum --jsonfile` (pinned: `GOTESTSUM_VERSION` in the workflow, updated by hand), never with `--rerun-fails`. Go prints the seed it chose for each package; the issue names the command that runs the package again in that order. The JSON of each group is kept as the artifact `nightly-go-json-<group>` for 30 days: it also holds the time of every test.
- **A failure** opens, or comments on, a `ci-red` issue (the same way the `report` jobs of `backend` and `e2e` do) with the test, its signature and the seed or command to replay it. The rule above applies: a failure found here is a bug, and its fix is the next train's.
