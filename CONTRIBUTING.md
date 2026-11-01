# Contributing to MeuRPG

All code enters through a pull request to `PuraFome/meuRPG` with green CI; nothing goes straight to `main`.
Until the MVP, the author squash-merges a PR as soon as CI is green, without waiting for a review; the PR is still opened to record what changed.

## Tools

Go (version in `backend/go.mod`), Node (`.node-version`), Docker, `buf`, `make`, and optionally `sqlc`, `golangci-lint`, `goose`, `govulncheck` and `osv-scanner` (`make sqlc` and `make lint` run the pinned versions by themselves).
On a Mac they all install with Homebrew.
The pinned versions and where each one lives are in [docs/development.md](docs/development.md#tools).

## Local environment

`make up` starts CockroachDB, the devidp (a development OIDC provider) and the backend, which also serves the app, at `http://localhost:8080`, with sign-in working (use Chrome; click a test user).
`make down` stops it; `make logs` follows it.

- Native CockroachDB instead of Docker's: `make db-native-start`, then `make up LOCAL_DB=native`.
- No Docker at all: `make up LOCAL_STACK=native`.
- Test databases, sign-in with a real provider, gallery images, AI-generated images and abuse-limit variables: [docs/development.md](docs/development.md).

## Everyday commands

`make help` lists every target.

| Command | What it does |
| --- | --- |
| `make proto` | Generates the Go and TypeScript code from `proto/` (`backend/gen`, `web/src/gen`). Commit the result |
| `make sqlc` | Generates the Go code of the SQL queries (`backend/internal/<module>/<module>db`) from each module's `queries.sql`. Commit the result |
| `make migrate` | Applies the goose migrations to the local database |
| `make test` | `go test -race ./...` in `backend/`. Integration tests need `MEURPG_TEST_DATABASE_URL`, see [docs/testing.md](docs/testing.md#integration-tests) |
| `make before-ready` | The golden, frozen, fixture and inventory tests a change makes stale, against the test database (`MEURPG_TEST_DATABASE_URL`): run it before you write `ready:`. It runs only the lines of the list that the change can affect (`make before-ready ALL=1` runs all of them; `time make before-ready` measures a run). It prints the command that regenerates a failed file; a test that turns a second train round red is added to `backend/internal/archtest/before_ready.txt`. See [docs/testing.md](docs/testing.md#before-ready) |
| `make db-test-start` / `make db-test-stop` | The test PostgreSQL on port 5433, where the backend tests run in a preflight |
| `make db-test-cockroach-start` / `make db-test-cockroach-stop` | An in-memory test CockroachDB on port 26258: the tests that [need it](docs/testing.md#integration-tests) and the ones a train runs |
| `make test-postgres` | `go test -race` on the test PostgreSQL, leaving out the tests that need CockroachDB. `make test` runs every test, on the database `MEURPG_TEST_DATABASE_URL` names |
| `make lint` | `buf lint`, `buf format` and `golangci-lint` on what your branch added since `origin/main` (run `git fetch origin main` first) |
| `make lint-all` | The same linters on the whole backend, to pick what to clean up |
| `make web-install`, `make web-test`, `make web-build` | Install, unit-test and build the Angular app in `web/` |
| `cd web && npm run lint` / `npm run format:check` / `npm run depcruise` | The web linters, as in CI |
| `make e2e` | Starts the local stack and runs the Playwright tests in `e2e/` |
| `.github/scripts/go-complexity.sh` | The complexity limits, as CI checks them: no Go function over cyclomatic 15, cognitive 15 or maintainability 20 |
| `cd backend && govulncheck ./...` | The known vulnerabilities the Go code calls, as the `govulncheck` job runs them (`brew install govulncheck`; CI pins its own version) |
| `osv-scanner scan source --config osv-scanner.toml -L backend/go.mod -L web/package-lock.json -L e2e/package-lock.json -L e2e/lint/package-lock.json` | The lockfile scan of the `osv-scan` workflow (`brew install osv-scanner`) |
| `.github/scripts/nolint-cap.sh` | The `//nolint` cap: no more directives in `backend/` than the number in `.github/nolint-cap` (`--lower` writes the cap down after you remove some) |

The Go and web linters are ratchets: they judge what your change adds, and the backlog may only shrink.
The complexity limits are not a ratchet: every Go function and every web file must meet them.
Read [docs/ci.md](docs/ci.md) before adding a `//nolint` or an ESLint suppression.
The initial web bundle is a build gate (500 kB), see [docs/ci.md](docs/ci.md#the-web-bundle-budget).

## Code rules that bite

- **Transactions:** inside `db.InTx`, use only `s.queries.WithTx(tx)`. A read through the pool takes a second connection while the transaction holds the first and can lock up the pool; the test pools have one connection and fail such a test with the stack. A cross-module call that reads receives the `tx`. See [Architecture](docs/architecture/transactions.md#transactions-and-the-connection-pool).
- **Migrations only add:** production is live, and the migration runs before the new revision gets traffic. A new table, column, index or row; never a rename, a drop or a new `NOT NULL` without a default on something running code reads. To remove something, release the code that stops using it first and drop it in a later release. Use SQL that both PostgreSQL (sqlc's parser) and CockroachDB accept: an index in its own migration with `CREATE INDEX IF NOT EXISTS`; every migration must run twice without error. A new session event kind is a row in `session_event_kinds`. Details in [docs/data/README.md](docs/data/README.md).
- **Rules content:** a change under `backend/internal/rules/srd51/effects/` raises the revision in `effects/revision.json`; the SRD snapshot in `data/` is generated, never edited. See [docs/rules-content.md](docs/rules-content.md).
- **Screens** follow the [design](docs/design/README.md) (the `--mr-*` tokens and the common pieces, never a hand-written colour) and enter `e2e/tests/a11y.spec.ts`. A screen change carries before and after screenshots at 390, 768 and 1280 px, in both themes.
- **Error reasons:** a new reason the server can send is added to `proto/error-reasons.json` with its Portuguese sentence in the web; two tests fail until both exist. See [API](docs/architecture/api.md#error-reasons).
- **Secrets** (the OIDC client secret, `GEMINI_API_KEY`, session cookies) never go to the repository, a test, a log or an issue.
- The app's UI text is in Portuguese; code, comments, commits and docs are in English.

## Flaky tests

A red test is a bug in the test or in the app, never noise: there are no retries, no `skip` or `fixme`, no longer timeout, no fixed sleep and no looser assertion, and "the train did not touch that test" is not a reason to merge. A flaky test stops holding a train only once it is acknowledged: its artifacts were read, a `flake-check` run shows it fails without the train, it has an owner, and its fix goes in the next train. How to reproduce one, what counts as fixed and how to dispatch `flake-check` are in [docs/ci.md](docs/ci.md#flaky-tests) and [docs/testing.md](docs/testing.md#reproducing-a-flaky-test).

## Branches, commits and pull requests

1. Branch from `main`: `feat/`, `fix/`, `docs/` or `chore/` and a short name. Without write access, work in a fork.
2. Write the code, the tests and the documentation in the same PR.
3. Commit in English with [Conventional Commits](https://www.conventionalcommits.org/), for example `feat(characters): lock sheet on first session`.
4. Open the PR citing the story (`MR-006`) and the rule (`RN-01`) it fulfils, and the issue it closes (`Closes #N`), or `Refs #N` for an issue it only advances, with the [PR template](.github/pull_request_template.md).
5. CI must be green. Every PR is squash-merged, with an English subject.
   A train is one PR from `refactor/train-N`, built from `main` out of the member branches.
   The member branches keep their own history in their closed preflight PRs on the fork.
6. Never force-push a branch that others build on.

What CI checks, and how failures are read, is in [docs/ci.md](docs/ci.md).

## The backlog

The backlog is the repository's issues and the "MeuRPG" board (https://github.com/users/vfraga/projects/1).

- An epic is an issue labelled `epic`; its tasks are its sub-issues.
- The labels are `area: rules`, `area: play`, `area: characters`, `area: maps`, `area: web`, `ops` and `code-health`, next to GitHub's own `bug`, `enhancement` and `documentation`.
- The milestones are "Sessão zero" and "Antes de outra mesa".
- The board's Status is A fazer, Fazendo, Bloqueado, Em revisão and Feito.

## Keeping the docs current

A change is not done until the docs that say what the code cannot say the same thing as the code.
Update them in the same PR:

| If the PR changes... | Update |
| --- | --- |
| A product rule, a story or its priority, a term | `docs/product/rules.md`, `stories.md`, `glossary.md` **and** their PT-BR twins under `docs/pt-BR/produto/` (same structure, same ids) |
| Personal data, a log, a cookie, an image or a vendor | `docs/privacy.md` and `docs/pt-BR/privacidade.md`, and the privacy checklist in the PR |
| Deploy, a secret, an alert or a cost | `docs/operations/README.md` |
| The system's structure: a module, an external service, infrastructure | `docs/architecture/README.md`, and an ADR if it is hard to undo |
| A table or column | The migration and `docs/data/README.md` |
| Colour, font, spacing or a visual component | `docs/design/README.md` and the visual system in Claude Design |
| A command, a tool or the CI | This file, `docs/development.md`, `docs/testing.md` or `docs/ci.md` |
| A project milestone | `docs/roadmap.md` |
| A dependency, font, icon or third-party content | A licence compatible with Apache 2.0; `NOTICE`, the licence in `third_party/licenses/` and the "Créditos" page when the licence asks for attribution |

Implementation detail lives in the code, its comments and its tests: API messages are documented in the `.proto` comments, and the reference is generated from them.
Do not copy it into a doc.

How to write a doc:

- Write every doc as if for the first time: the current state, with no history ("now", "no longer", "was changed"), no dates and no PR numbers. When and who decided goes to `docs/archive/decisions.md`.
- The first sentence of a section is the answer. Short sentences, one per line. English is canonical; the readers' docs (`README.md`, `docs/product/*.md`, `docs/privacy.md`) have PT-BR versions updated in the same PR, and the engineering docs are English only.
- One subject per file, with links instead of copies; when something moves, fix the links.
- Flows and relations are Mermaid diagrams (render-check them); compared items are tables.
- Every number has a unit and a source. What nobody knows is asked of the product owner, never guessed.
- A doc says what the system does and why. Measured timings and counts that rot do not belong in it; the command that measures does.

A new story follows the template in `docs/product/stories.md`; every acceptance criterion becomes an automatic test, and the story is done when they pass.
[docs/README.md](docs/README.md) indexes the documentation.

## Licence

MeuRPG is distributed under the [Apache License 2.0](LICENSE); every PR enters under it (section 5), so only submit code you wrote or that has a compatible licence.
Code under an incompatible licence (for example CC BY-NC) is never copied: if an idea is worth it, one person writes a specification of the behaviour in our own words and another implements from it alone (clean room).
Third-party content keeps its own licence, listed in [NOTICE](NOTICE): SRD 5.1 (CC BY 4.0), the 5e-database data (MIT), the fonts (OFL 1.1) and the icons (Apache 2.0).

## Topic docs

| Document | Answers |
| --- | --- |
| [docs/development.md](docs/development.md) | The local environment in full, sign-in, images, the variables |
| [docs/testing.md](docs/testing.md) | The test layers, how to run them, the database rules, leak tests, Playwright |
| [docs/ci.md](docs/ci.md) | The CI jobs, the ratchets and gates, pinning and Dependabot |
| [docs/performance.md](docs/performance.md) | The memory and time budgets in code and how to measure |
| [docs/rules-content.md](docs/rules-content.md) | The SRD snapshot, the effects, names and texts, and how to change them |
| [docs/architecture/README.md](docs/architecture/README.md) | How the system is built |
