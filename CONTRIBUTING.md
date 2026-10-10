# Contributing to MeuRPG

All code enters through a PR to `PuraFome/meuRPG`, with green CI. Nothing goes straight to `main`. Until the MVP, the author of a PR squash-merges it as soon as CI is green, without waiting for a review; the PR is still opened to record what changed.

## Local environment

Tools: Go 1.27, buf, sqlc 1.31.1, goose, golangci-lint 2.14.0, Docker and Node 22. On a Mac, all of them install with Homebrew. sqlc and golangci-lint are optional: `make sqlc` and `make lint` run the right version by themselves. Node comes from the `.node-version` file (CI's only source) and Go from `backend/go.mod`; the Dockerfiles pin their images by digest and must match both (a comment in `backend/Dockerfile` says so).

| Command | What it does |
| --- | --- |
| `make up` | Starts CockroachDB (a single node), the devidp (the development OIDC provider) and the backend with Docker Compose (`deploy/local/compose.yaml`); serves the app at `http://localhost:8080`, server and API on the same origin, with sign-in working (see [Local sign-in with the devidp](#local-sign-in-with-the-devidp)) and the gallery images on a volume (see [Gallery images](#gallery-images)). The log is at `LOG_LEVEL=debug` (every request, RPC, stream and game event, with a `request_id`); `LOG_LEVEL=info make up` lowers it (see [Architecture](docs/architecture.md#logs)). |
| `make up LOCAL_STACK=native` | The same environment **without Docker**: the devidp and the API run as processes on the Mac, against the native CockroachDB, on the same ports and with the same sign-in. `make down`, `make logs` and `make e2e` accept the same `LOCAL_STACK=native`. See [Everything native](#everything-native-mac-optional). |
| `make run` | Runs the backend straight in the terminal, pointing at the `make up` database. |
| `make db-native-start` / `make db-native-stop` | Start and stop a CockroachDB running directly on the Mac, outside Docker, for `LOCAL_DB=native`. See [Native CockroachDB](#native-cockroachdb-mac-optional). |
| `make db-test-start` / `make db-test-stop` | Start and stop the **test** CockroachDB, also directly on the Mac, on port 26258 (with `TEST_DB_PORT=26259`, a second one next to it), with the database **in memory** (at most 2 GiB, gone on shutdown). It is the best place for the integration tests: schema changes, which the test databases are made of, take a quarter of the time, and the development database (26257) does not fill up with test databases. See [Integration test databases](#integration-test-databases). |
| `make proto` | Generates the Go **and** the TypeScript code from the `.proto` files (`backend/gen` and `web/src/gen`). Installs the `web/` dependencies by itself if they are missing. |
| `make sqlc` | Generates the Go code of the SQL queries (`backend/internal/<module>/<module>db`) with sqlc 1.31.1. See [Queries with sqlc](#queries-with-sqlc). |
| `make lint` | Runs `buf lint`, `buf format` and `golangci-lint` at CI's pinned version (2.14.0, through `go run`; the first run builds the linter, about two minutes) **as a ratchet**: only what your branch added since `origin/main` (the merge base) is reported, exactly what CI judges on a pull request. Run `git fetch origin main` first so the base is fresh; `LINT_BASE=<ref>` changes the base. When you change the linter version, change the `golangci-lint-action` in `.github/workflows/backend.yml` too. See [Go linters and the ratchet](#go-linters-and-the-ratchet). |
| `make lint-all` | The same linters on the whole backend, including the backlog the ratchet lets through. Use it to pick what to clean up next. |
| `make test` | Runs `go test -race` over the whole backend. |
| `MEURPG_TEST_DATABASE_URL='postgresql://root@localhost:26258/defaultdb?sslmode=disable' make test` | Runs the integration tests (see the list below) against the test CockroachDB (`make db-test-start`); port 26257 also works, the one from `make up` or `make db-native-start`. Without the variable, they are skipped. |
| `make migrate` | Applies the goose migrations to the local database. |
| `make e2e` | Starts the local environment (like `make up`), runs the Playwright tests in `e2e/` against it and shows where the report is. The environment stays up; `make down` takes it down. See [End-to-end tests](#end-to-end-tests-playwright). |
| `make down` | Takes the local environment down (`docker compose down`). |
| `make elk-up` / `make elk-down` / `make elk-logs` | Starts, stops and shows the logs of the ELK stack (Elasticsearch, Kibana and Filebeat, `deploy/elk/compose.yaml`) that keeps the API's logs: it reads the containers of `make up` and the `~/.meurpg/run*/api.log` files of `LOCAL_STACK=native`. The first run creates `deploy/elk/.env` with random passwords. Kibana is at `http://localhost:5601`; from the terminal, `deploy/elk/logs.sh errors`, `request <id>`, `tail` and more (`logs.sh help`). See [Operations](docs/operations.md#logs-in-elk). |
| `make web-install` | Installs the `web/` dependencies: `npm ci --ignore-scripts` (never runs third-party install scripts). To add or update a dependency, use `npm install` with Corepack enabled (`corepack enable`, once): `web/package.json` pins `npm@11.20.0` because the stock `npm` (10.x) hangs resolving the peer-dependency graph of Vitest 4.1; `npm ci` does not have this problem and works with either. |
| `make web-test` | Runs the Angular tests (`cd web && npm test`). The test files are not isolated from one another (the Angular builder runs Vitest with `isolate: false`), so the hooks of `web/src/test-setup.ts` (registered for every spec file by `web/src/test-hooks.ts`, which `web/vitest-base.config.ts` lists as a setup file) reset the TestBed, undo every `vi.stubGlobal` at the end of each test, give each test a fresh `scrollIntoView` and `window.scrollTo` (jsdom has none, and the components call them) and restore the real timers (a fake clock must not leak into another file); and `web/src/test-providers.ts` turns off the Material animations: no test depends on file order or waits real time. |
| `cd web && npm run lint` | ESLint (`web/eslint.config.js`): typescript-eslint with the type-aware promise rules (`no-floating-promises`, `no-misused-promises`, `await-thenable`) and no `any`; angular-eslint with the template and accessibility rules, OnPush, standalone components and `inject()`; `complexity` 15, `max-depth` 4 and no magic numbers in app code. A ratchet: see [Web and e2e linters](#web-and-e2e-linters). |
| `cd web && npx eslint --prune-suppressions` | Removes from `web/eslint-suppressions.json` the entries you fixed. Run it after fixing a suppressed spot, and commit the smaller file. |
| `cd web && npm run format` / `npm run format:check` | Prettier (`web/.prettierrc`, `web/.prettierignore`) writes or checks the TypeScript, SCSS and JSON; CI runs `format:check`. The Angular templates (`.html` files and the inline templates of `.ts` files) are left out: Prettier's HTML printer adds and removes whitespace between elements and text, which changes what renders when CSS makes an element inline. Format them by hand, as before. The one formatting commit is in `.git-blame-ignore-revs`, so `git blame` skips it (`git config blame.ignoreRevsFile .git-blame-ignore-revs` once, locally; GitHub reads it by itself). |
| `cd web && npx ng test --watch=false --coverage` | The unit tests with coverage, the same 616 spec files as the plain run (`test-setup.spec.ts` included); the report is in `web/coverage/web/` (`lcov-report/index.html`, `coverage-summary.json`). Vitest does not run again, for each spec file, the setup files the Angular builder bundles when `--coverage` is on (they stay cached for the worker), so hooks registered there reach only the first file of each worker: the next ones keep the previous file's TestBed, fake clock and stubs, and their `fixture.whenStable()` and timer waits run into the 5 s test timeout. That is why the hooks are registered by `web/src/test-hooks.ts`, which Vite loads itself (`web/vitest-base.config.ts`) and Vitest runs again for every file, and which calls the bundled `web/src/test-setup.ts` through a global. |
| `cd e2e && npm run lint:install && npm run lint` | ESLint for the Playwright tests: `no-floating-promises` and `await-thenable` (a missing `await` is the classic Playwright flake) and `eslint-plugin-playwright`. `e2e/lint/` is its own small package because typescript-eslint needs TypeScript 5.x and `e2e/` type-checks with TypeScript 7. |
| `make web-build` | Builds Angular for production (`cd web && npm run build`). |
| `cd web && npm start` | Starts Angular alone, in dev mode, with `proxy.conf.json` forwarding the API routes (`/meurpg.*`, `/auth`, `/images`, `/uploads`, `/healthz`, `/readyz`) to `localhost:8080`. |
| `WEB_DIR=../web/dist/web/browser PORT=8090 go run -C backend ./cmd/api` | Starts just the API the way it runs in production, serving the Angular build, with the real cache headers and CSP, without Docker or a database. Run `cd web && npm run build` first. Without `DATABASE_URL`, sign-in is off and `IdentityService` answers `unavailable`, but the public screen and `SystemService.GetServerInfo` work normally. Useful to check the CSP in the browser without starting Docker; port 8090 does not clash with `make up`. |

The integration tests (run with `MEURPG_TEST_DATABASE_URL`) cover: migrations, transactions, sign-in, campaigns, invites, characters, game sessions, the live session with its stream, the gallery, maps, RP scenes, the images of an RP scene, clues and players' notes, the stage and NPC portraits, combat, combat highlights, XP, guided level-up, the editor's preview of the sheet being made (the same checks as the save, nothing written, the hit points the effects add), scene options, the session summary, the SRD creatures, the bestiary and the NPC made from a creature, the map layers, doors, traps and what they do in play, treasures, the carried light, per-player fog of war, circle movement, jumping and cover in combat, the character's creatures and what the sheet can summon, Wild Shape and the familiar's eyes, rests and hit dice by die size, the class resources (Lay on Hands, Flexible Casting, Metamagic and Bardic Inspiration with its held roll), and the table's classes and subclasses (the 20-level table defaults, the effect menu, the field of each refusal, multiclass at creation, level-up, and combat with the one-third caster, and the "A classe mudou" sentences). They also cover the table rules outside combat (level-up HP, the ways to make ability scores and the stored 4d6, the fog of new maps and the change of XP mode), grid calibration, the map of a generated dungeon (the preview, the room list, the scene in the room and "Redesenhar"), and the puzzles: the four tables, simultaneous rolls, "Ao resolver", the riddle, the sequence (played step by step) and the cipher, hints by skill check (app dice and physical dice), split information and "Ao errar" (the trap, the attempts and the limits, counted exactly with simultaneous answers). Further: the table content (the master's writes, the revision and the live cache, "A classe mudou" and the archived entry as a new choice); AI-generated images with the fake generator, and the ones made from a map (the players' view: the union of what the characters see now, the hidden NPC and the secret door as a wall; the textured map completed and cropped back; and "Usar como imagem do mapa"); combat without a grid, theatre of the mind (the mode, movement by number, the opportunity attack the master offers, and what does not exist without a map); the players' spell list, the "Outro" background and the table's spell in combat; the "Opções para os jogadores" switches (what is off never reaches a player, the new choice refused, the revision and the cache, the `content_changed` hint and simultaneous switches); monsters in combat, "Pôr no combate" (names, average or rolled HP, each one's initiative, what the player reads, the critical, XP by CR and theatre of the mind); the table rules inside combat (the critical: doubled dice or the maximum plus a roll, for weapon, spell, creature and NPC, with app dice and physical dice, read in the transaction of each roll; and death saves that only the owner and the master see, on the combatant, the log, the stream, the highlights and the summary); the treasure generator (the party level, who can ask, "Pôr no mapa" with the hidden point, the idempotency key and what the player never receives, and gold that becomes XP in a gold campaign); and casting outside combat (rituals, long casts the master concludes, concentration, Mage Armor, rests and reach), and the encounter builder (the maths against the party, the seed generator, "Trocar", the encounter saved on a battle point and "Começar este combate", with the monsters entering at the start of combat, also in theatre of the mind); and the RN-10 leak tests, `internal/leaktest` (a campaign with one hidden thing of each kind, every read and every stream event asked by every person, the images and the fog tiles, and the guard that fails for a new procedure without a classification; see [Architecture](docs/architecture.md#leak-tests-rn-10)).

### Native CockroachDB (Mac, optional)

On a Mac, every container runs inside a Linux virtual machine (Docker Desktop's or Rancher Desktop's), and CockroachDB is by far the heaviest part of the environment: the VM can ask for more than 10 GB of memory, and the integration tests get slow. Running the database directly on the Mac fixes that. It is optional: the default, and what CI uses, is still the database in Docker.

1. Install the same version CI and `compose.yaml` use (26.2.7), from Cockroach Labs' official tap. Homebrew asks to trust the formula first:

   ```bash
   brew tap cockroachdb/tap
   brew trust --formula cockroachdb/tap/cockroach@26.2
   brew trust --formula cockroachdb/tap/cockroach   # the link loads this one too
   brew install cockroachdb/tap/cockroach@26.2
   brew link cockroachdb/tap/cockroach@26.2
   ```

   Do not use `brew services start`: it makes the database start with the Mac. `make` starts and stops it when you need it.
2. `make db-native-start` starts the database at `localhost:26257` (the console is at `http://localhost:8081`), with the data in `~/.meurpg/cockroach`, outside the repository (all worktrees use the same database), and creates the `meurpg` database. If it is already up, it does nothing. `make db-native-stop` stops it.
3. With `LOCAL_DB=native`, `make up`, `make e2e`, `make down` and `make logs` use this database: `deploy/local/compose.native-db.yaml` leaves the CockroachDB container out and points the API and the migrations at `host.docker.internal:26257`. You can pin it in the terminal with `export LOCAL_DB=native`, or pass it to each command: `make up LOCAL_DB=native`.
4. The integration tests can use this database (`MEURPG_TEST_DATABASE_URL='postgresql://root@localhost:26257/defaultdb?sslmode=disable' make test`), but the better place for them is the test CockroachDB, also native, in memory, on port 26258: `make db-test-start` and `MEURPG_TEST_DATABASE_URL='postgresql://root@localhost:26258/defaultdb?sslmode=disable' make test` (see [Integration test databases](#integration-test-databases)). Measured on Vinicius's machine, with the native database on disk, the `campaigns` tests dropped from 729 s to 99 s, and the `play` tests from 407 s to 60 s, compared with the database in Docker.

The container database and the native one use the same port: stop one before starting the other (`make down` takes the container's down). Their data is separate. With the database outside Docker, you can lower the VM's memory in the Docker Desktop or Rancher Desktop settings.

### Everything native (Mac, optional)

With `LOCAL_STACK=native`, the local environment runs **without Docker**: the devidp and the API are processes on the Mac, against the native CockroachDB (`make db-native-start`), and the Docker VM can stay off. On a Mac, that VM reserves several GB of memory; without it, there is memory left for the in-memory test databases (see [below](#integration-test-databases)). CI still uses Docker, and `LOCAL_STACK=docker` is still the default.

```bash
make db-native-start
make up LOCAL_STACK=native     # builds, migrates, starts the devidp and the API; the app at http://localhost:8080
make logs LOCAL_STACK=native   # the logs of both processes
make e2e LOCAL_STACK=native    # starts (if needed) and runs the Playwright tests
make down LOCAL_STACK=native   # stops both processes
```

- **What `make up` does** (`deploy/local/native.sh`): builds the API, `migrate` and the devidp, rebuilds the Angular app only if some file in `web/` changed since the last build, applies the migrations to the `meurpg` database and starts the devidp and the API in the background, with the same variables as `compose.yaml`. Each process writes its PID and log to `~/.meurpg/run/`, and `make down` stops exactly those PIDs. The gallery images live in `~/.meurpg/images`, separate from the Docker volume.
- **Nothing is open on the local network.** The native API and devidp listen only on `127.0.0.1` (`native.sh` sets `LISTEN_HOST=127.0.0.1`; in production the variable is empty and the server listens on every interface, as Cloud Run requires), and `compose.yaml` publishes every port on `127.0.0.1`: the local CockroachDB has no password.
- **The issuer is the same, `http://idp.localhost:9090`.** The macOS resolver sends any `*.localhost` name to the computer itself (RFC 6761), and Go uses that resolver by default on the Mac. Go's own resolver (`GODEBUG=netdns=go`) and Linux do not do this; on them, use Docker.
- **Ports 8080 and 9090 must be free.** Rancher Desktop keeps forwarding a container's ports even after `make down`, until it is closed: close it, or start the native environment on other ports.
- **A second environment, next to the first:** `API_PORT=8180 IDP_PORT=9190 DB_NAME=meurpg_2 deploy/local/native.sh up` starts another one, with other ports, another database (created if it does not exist), another PID and log folder (`~/.meurpg/run-8180`) and another images folder. The Playwright tests go to it with `E2E_BASE_URL=http://localhost:8180 E2E_IDP_ORIGIN=http://idp.localhost:9190`; since the database is different, the test accounts of the two environments do not mix. To stop it: `API_PORT=8180 deploy/local/native.sh down`.

### Integration test databases

You can have **more than one in-memory test database** at the same time, each on its own port (`make db-test-start TEST_DB_PORT=26259`; the console is on the port after the first one's, 8083), each with its own memory (up to 2 GiB). Two runs of the integration tests, each pointing `MEURPG_TEST_DATABASE_URL` at one, never wait for each other. Each costs about 2.5 GB of memory on the Mac; with the Docker VM off ([Everything native](#everything-native-mac-optional)), two fit on a 16 GB Mac.

The integration tests **reuse databases**: each package creates a few test databases and, between one test and the next, only empties them (`backend/internal/platform/dbtest`). Creating a database with all the tables takes about 5 seconds in CockroachDB (each table is a schema change), and dropping it another 1.5; emptying it with `DELETE` takes about 10 milliseconds. Measured on 04/10/2026: the `progression` tests dropped from 88 to 20 seconds.

- The migrations run once, on a template database, `meurpg_tpl_<hash of the migrations>`. A new or changed migration generates another one. The templates of old migrations stay on the test server, empty, and can be dropped (`DROP DATABASE meurpg_tpl_... CASCADE`).
- The tables a migration fills (a seeded table such as `session_event_kinds`) are found in the migrated template, not listed anywhere: a test database is created with their rows, and emptying one leaves them.
- A test that needs a database takes a free one from the package (or creates one, copying the tables from the template) and returns it when it finishes; the next test deletes all the rows before using it (child tables first). Parallel tests never share a database: the package creates as many as run at the same time.
- Every package that uses a database has a `TestMain` that calls `dbtest.Main(m)`: at the end of the run, it drops the databases the package created. A new package with integration tests needs it, or the databases are left behind.
- If a run is interrupted midway, its databases stay on the server (`meurpg_<package>_test_...`) and can be dropped by hand. With `make db-test-start`, stopping the test database deletes everything.
- The migrations themselves are still tested from scratch by the `migrations` package.
- Every test pool has **one** connection (`MaxConns = 1`) and an `Acquire` tracker. A test where a read goes through the pool from inside a `db.InTx` fails at once, with the stack, and the `Acquire` is cancelled. A wait of more than 90 s for a connection (the case the stack does not see) brings down the whole package's test binary, with the stack of whoever is waiting. So each package's suite is also the proof that no write takes a second connection.
  - A test that races transactions against each other asks for a bigger pool at the start: `dbtest.PoolSize(t, n)`, with `n` at least the number of runners. With one connection they would run one at a time, and the test would pass proving nothing. It works with `t.Parallel`. It must come before the harness is built (a later call fails the test, since it would change nothing). The runners wait for each other at a start line, `start := dbtest.NewBarrier(n)` before the goroutines and `start.Wait()` first thing in each, so that they really begin together instead of one finishing before the next is scheduled.
  - A test that needs to hold a transaction open while the service works uses `dbtest.SideConnection(t, pool).Begin(...)` (not `db.InTx` around calls to the service: the tracker would flag them).
  - `MEURPG_TEST_POOL_MAX_CONNS=8` applies to the whole process, and serves to list all the violations at once. In production the pool opens at most 10 connections (see [Operations](docs/operations.md#the-connection-pool)).
- In CI, the `go-db` job's CockroachDB also keeps its data in memory and turns off what only serves a long-lived database (automatic statistics, empty range merging, job history), like `make db-test-start`. The packages run at the same time against it, each with up to 25 minutes (`go test -timeout 25m`, inside the job's 30).

## Sign-in with an OIDC provider

Master sign-in works with any OpenID Connect provider: Google in production and, on your machine, a local OIDC provider (or a real provider with a test client). The backend only needs these variables:

| Variable | Required | What it is |
| --- | --- | --- |
| `OIDC_ISSUER` | Yes, to turn sign-in on | The provider's issuer, equal to the `issuer` field of its `/.well-known/openid-configuration`. Must be `https`; `http` is only valid on `localhost`, on a `*.localhost` name or on a loopback IP. |
| `OIDC_CLIENT_ID` | Yes | MeuRPG's client ID at the provider. |
| `OIDC_CLIENT_SECRET` | Yes | The client secret. It is a secret: it never goes to the repository, an issue or the log (the backend shows `[REDACTED]`). |
| `OIDC_REDIRECT_URL` | Yes | The backend's public URL plus `/auth/callback`. On your machine, `http://localhost:8080/auth/callback`. Register it at the provider exactly the same. |
| `OIDC_CA_FILE` | No | A PEM file with the certificate of a local provider with a self-signed certificate. |
| `OIDC_MAX_AGE` | No | A duration, such as `1h`. If set, it goes as `max_age`: the provider asks for the password again when the last sign-in with it is older than that. Use it only with a provider that documents `max_age`: `1h` with the devidp (what `compose.yaml` uses); **do not set it with Google**, which does not document `max_age`. What guarantees re-authentication at least every 30 days (NIST SP 800-63B-4, AAL1) is our server-side session, which never lasts more than 30 days, not `max_age`. |

One separate variable applies to the sign-in session itself: `SESSION_IDLE_TIMEOUT` (optional, from `1h` to `720h`, default `336h`) is how long a session may go unused before it stops being valid; the absolute 30 days do not change (see [Operations](docs/operations.md#environment-variables-and-secrets)). The app's profile has "Sair dos outros dispositivos", which ends the account's other sessions.

Without `OIDC_ISSUER` or without `DATABASE_URL`, the backend still starts, warns in the log that sign-in is off, and `/auth/login` answers 503.

To test sign-in on your machine:

1. At the provider, create a confidential client with Authorization Code, PKCE S256, the scopes `openid email` and the redirect URL `http://localhost:8080/auth/callback`.
2. Start only the database and the migrations, leaving port 8080 free: `docker compose -f deploy/local/compose.yaml up -d cockroach migrate`.
3. Run the backend with the variables, for example `OIDC_ISSUER=... OIDC_CLIENT_ID=... OIDC_CLIENT_SECRET=... OIDC_REDIRECT_URL=http://localhost:8080/auth/callback make run`. The startup log says `sign-in is enabled`.
4. Open `http://localhost:8080/auth/login?return_to=/` in Chrome or Firefox. Safari does not accept a `Secure` cookie on `http://localhost`.
5. To see who is signed in, open in the same browser `http://localhost:8080/meurpg.identity.v1.IdentityService/GetMe?connect=v1&encoding=json&message=%7B%7D`.

Connect calls by `curl` need the header `-H 'Connect-Protocol-Version: 1'` (CSRF protection, see [Architecture](docs/architecture.md#csrf)).

Sign-in tests:

- `make test` runs everything with a fake OIDC provider, inside the test itself. It needs no network.
- With `MEURPG_TEST_DATABASE_URL` pointing at a CockroachDB (for example `postgresql://root@localhost:26257/defaultdb?sslmode=disable`), the same tests also run against the database.
- With the `MEURPG_TEST_OIDC_*` variables (the list is in the comment of `TestRealProviderSignIn`, in `backend/internal/identity`), a test signs in for real with a local OIDC provider, without a browser.

## Local sign-in with the devidp

`make up` already starts sign-in ready to use: the `idp` service of `compose.yaml` runs the **devidp** (`backend/cmd/devidp`), a minimal OpenID Connect provider, only for your machine and for CI, in place of Google.

1. `make up`.
2. Open `http://localhost:8080/auth/login?return_to=/` in Chrome. Safari does not accept a `Secure` cookie on `http://localhost`, and Firefox has not been tested with the devidp yet.
3. The browser goes to `http://idp.localhost:9090`, which lists the test users. One click and you are back, signed in.

| Test user | `sub` | E-mail | What for |
| --- | --- | --- | --- |
| Mestre Teste | `devidp-mestre` | `mestre@example.com` (verified) | The tests' master. |
| Jogador Teste | `devidp-jogador` | `jogador@example.com` (verified) | A second person, for the tests with a player. |
| E-mail Não Verificado | `devidp-nao-verificado` | `nao-verificado@example.com` (**not** verified) | Checking that an unverified e-mail is not stored. |
| Sessões Teste | `devidp-sessoes` | `sessoes@example.com` (verified) | Only the "Sair dos outros dispositivos" test (`e2e/tests/sessions.spec.ts`): that action ends every other session of the account, so no other shared account will do. No other test signs in with it. |

The `sub` is fixed, so each test user always lands on the same account of the local database. There is no password: anyone who reaches the devidp signs in as any test user. That is why it **never** goes to production:

- the production image (`backend/Dockerfile`) only compiles `cmd/api` and `cmd/migrate`; the devidp has its own image (`deploy/local/devidp.Dockerfile`), used only by `compose.yaml`, and CI checks that the production image does not have the binary;
- it refuses to start if the issuer is not on a loopback host (`localhost`, `*.localhost`, `127.0.0.1`, `::1`), or if it is on Cloud Run (`K_SERVICE` set); a test covers this guard;
- when it starts, it prints a big warning in the log.

The configuration comes from flags or environment variables: `DEVIDP_ISSUER`, `DEVIDP_LISTEN`, `DEVIDP_CLIENT_ID`, `DEVIDP_CLIENT_SECRET` and `DEVIDP_REDIRECT_URIS` (the full list and the defaults are in the comment of `backend/cmd/devidp/main.go`). To run outside Docker, with ports 8080 and 9090 free: `cd backend && go run ./cmd/devidp` in one terminal and, in another, the backend with `OIDC_ISSUER=http://localhost:9090 OIDC_CLIENT_ID=meurpg-local OIDC_CLIENT_SECRET=meurpg-local-secret OIDC_REDIRECT_URL=http://localhost:8080/auth/callback make run` (with port 8080 free).

The devidp also honours `max_age` and `prompt=login|none`: it keeps its own login in a `devidp_session` cookie, so a second sign-in in the same browser, within `max_age`, returns at once, without the list. The provider itself lives in `backend/internal/identity/oidctest` and is the same one the Go tests use.

**Why `idp.localhost`.** The issuer must be the same string for the browser and for the API container, because the backend checks the ID token's `iss`. The browser resolves any `*.localhost` to `127.0.0.1` by itself (RFC 6761) and reaches the devidp through the published port; the API container resolves the same name through Docker's network alias. Details in [Architecture](docs/architecture.md#test-layers-and-the-development-provider).

## Gallery images

The images the master uploads (MR-019) live in a folder in the local stack, the one in `BLOB_DIR`, and in a Cloud Storage bucket on Cloud Run (`BLOB_BUCKET`, [Operations](docs/operations.md#images)). `make up` already turns the folder on: `compose.yaml` mounts the `images` volume at `/var/lib/meurpg/images`, and it survives `make down` (`docker volume rm meurpg-local_images` deletes the images).

| Variable | Required | What it is |
| --- | --- | --- |
| `BLOB_DIR` | No | The folder where the API stores the images and thumbnails. Without it, images are off: `POST /uploads/images`, `GET /images/...` and `GalleryService` answer `503`/`unavailable`, the startup log warns, and the rest of the app works. A folder without write permission keeps the API from starting. Never together with `BLOB_BUCKET` (the API refuses to start), and refused on Cloud Run. |
| `BLOB_BUCKET` | No | The Cloud Storage bucket where the API stores the images, with the Cloud Run service account's token. For production only: the local stacks and the tests keep `BLOB_DIR`. If the bucket cannot be reached at start, the log warns and images stay on; each call is tried on its own. The first-deploy steps are in [Operations](docs/operations.md#first-deploy-step-by-step). |

With `make run`, the API runs in the terminal without images; to turn them on, `BLOB_DIR=/tmp/meurpg-images make run` (the folder is created if it does not exist). The tests use a temporary folder each.

To test the upload with `curl`:

1. Sign in with Chrome at `http://localhost:8080` as "Mestre Teste" and create a campaign.
2. Copy the value of the `__Host-meurpg_session` cookie (DevTools → Application → Cookies → `http://localhost:8080`) and the campaign ID (what comes after `/campaigns/` in the page URL). The cookie is your session: do not paste it anywhere.
3. Upload. The fields go in this order, `campaign_id` and then `file`, and `curl`'s `-F` respects the order:

   ```bash
   SESSION='cookie value'
   curl -s -X POST http://localhost:8080/uploads/images \
     -b "__Host-meurpg_session=$SESSION" \
     -F campaign_id=<campaign id> -F file=@mapa.png
   ```

   The answer is `201` with the image in JSON (`id`, `url`, `thumbnailUrl`...). An error comes as `{"code": "...", "reason": "...", "message": "..."}`; the `reason`s are in the comment of `GalleryService` (`proto/meurpg/maps/v1/gallery.proto`) and in [Architecture](docs/architecture.md#upload-errors).
4. Download: `curl -s -b "__Host-meurpg_session=$SESSION" http://localhost:8080/images/<id> -o image` (or `/images/<id>/thumb`, the thumbnail).

The upload is not Connect, so it does not need `Connect-Protocol-Version`. CSRF protection (`http.CrossOriginProtection`) judges only the headers the browser sets by itself, `Sec-Fetch-Site` and `Origin`: `curl` sends neither, and passes. With `-H 'Sec-Fetch-Site: cross-site'` or `-H 'Origin: https://outro.site'`, the answer is `403`, as it would be for a page from another site. In the app, the browser sends `Sec-Fetch-Site: same-origin`, which passes.

## AI-generated images

Image generation (MR-039, RN-28) calls the Gemini API. Without a key, it is off and the rest of the app works.

| Variable | Required | What it is |
| --- | --- | --- |
| `GEMINI_API_KEY` | No | The Google AI Studio key. **A secret:** it never goes to the repository, a test, the log or an error message (the backend shows it as `[REDACTED]`). Without it, the startup log says generation is off and the calls answer `failed_precondition` with the reason `OFF`. |
| `GEMINI_IMAGE_MODEL` | No | The model; default `gemini-3.1-flash-image`. |
| `IMAGE_GENERATOR` | No | `fake` uses the fake generator (`backend/internal/maps/images/gen`): no key and no network, it returns a deterministic PNG of the requested aspect ratio. It is what `make up` (Docker and native) uses. A `[recusa]`, `[vazio]`, `[erro]` or `[lento]` in the request text makes the fake refuse, return no image, fail or take long. The server refuses to start with `fake` on Cloud Run. |
| `IMAGE_MONTHLY_LIMIT` | No | Images per campaign per month; default 20 (see [Operations](docs/operations.md#generated-images-the-gemini-api)). |
| `IMAGE_DAILY_LIMIT` | No | Images for the whole server per Brasília day; default 100. It is the ceiling of the Gemini account (see [Operations](docs/operations.md#abuse-limits)). |

The tests always use the fake generator. The test with the real model, `TestRealGemini` (in `backend/internal/maps/images/gen`), generates a small image and only runs with `MEURPG_TEST_GEMINI_API_KEY` set on your machine (and, if you want, `MEURPG_TEST_GEMINI_MODEL`): without it, it is skipped, and CI never sets it. The memory measurements (`MEURPG_MEASURE=1`) are in [Operations](docs/operations.md#generated-images-the-gemini-api). The module's integration tests: `go test -race -run 'TestMR039|TestRN28|TestRN10_AGeneratedImage' ./internal/maps/`, against the test database.

## Abuse limits

The backend limits the request rate (per IP and per user, in memory) and what one account creates. The defaults suit one table and don't get in the way of development; the variables raise or lower a ceiling.

| Variable | Required | What it is |
| --- | --- | --- |
| `MAX_CAMPAIGNS_PER_USER` | No | Campaigns one account may be the master of; default 10 (RN-30). The local stacks and CI use `off` (no ceiling), because the e2e suite creates hundreds of campaigns with the same test master; the API refuses `off` on Cloud Run. |
| `CAMPAIGN_CREATORS` | No | Verified e-mails, comma-separated, that may create campaigns; empty, anyone creates. In `make up`, "Mestre Teste" is `mestre@example.com`. |
| `RATE_LIMIT_MULTIPLIER` | No | Multiplies every rate limit; default 1. `make up` (Docker and native) uses 10, because the e2e suite sends the requests of several accounts from the same address and shares some accounts between workers. |

The numbers, the answers (`429` with `Retry-After`, `resource_exhausted`) and how each limit counts are in [Architecture](docs/architecture.md#abuse-limits) and [Operations](docs/operations.md#abuse-limits). The limiters' tests use a fake clock (`go test ./internal/platform/ratelimit`; the dungeon creation limit and the map hint gate in `maps` take a test clock too); the campaign cap and the daily image cap run against the database, like the rest (`TestRN30_*` in `campaigns`, `TestMR039_TheServersDailyCap` and `TestRateLimitsOfTheImageRoutes` in `maps`).

## End-to-end tests (Playwright)

The acceptance tests through the screen live in `e2e/`, a Playwright project in TypeScript, with its own `package.json` and pinned versions.

- `make e2e` starts the local environment, installs the `e2e/` dependencies if missing (`npm ci --ignore-scripts`), runs the tests and shows where the HTML report is (`cd e2e && npx playwright show-report`).
- On your machine, the tests use the **installed Google Chrome** (`channel: 'chrome'`), without downloading a browser. In CI, they use the Chromium that `npx playwright install chromium` downloads, at the version pinned by `package-lock.json`. `E2E_BROWSER_CHANNEL` changes this (empty = Playwright's Chromium, which needs `npx playwright install chromium` first).
- Sign-in goes through the devidp, clicking the test user as a person would. The RPCs go with `page.request`, which uses the same cookies as the page, and with the header `Connect-Protocol-Version: 1`.
- The suite signs in for real only where signing in is the acceptance criterion itself: `login.spec.ts`, `ui.spec.ts` and the "visitor without a session" half of `invite.spec.ts`. Everything else reuses a ready session: the `setup` project (`e2e/tests/auth.setup.ts`), which runs before everything (`playwright.config.ts`: the `chrome` project depends on it), signs in once as "Mestre Teste", once as "Jogador Teste" and once as "E-mail Não Verificado" (the second player of tests that need two people at the table, such as the fog-of-war ones) and saves each one's `storageState` in `e2e/.auth/` (in `.gitignore`: it is a session, it is a secret). The other tests use `newSignedInContext(browser, 'Jogador Teste')` (or `test.use({ storageState: authStatePath(...) })` for the file's default `page`) instead of signing in each time; see `support.ts`. This exists because `/auth/login` and `/auth/callback` share a limit of 40 hits per client at once (20 sign-ins, since a sign-in is one of each), then one every 3 seconds (`backend/internal/identity/login.go`, `loginRateLimit`): signing in every time, the whole suite exceeded that limit by itself, even on a quiet machine; with the reuse, a full run makes about 17 real sign-ins, against the more than 30 before.
  - A test that signs out (`SignOut`) never reuses a session: it signs the real session out on the server, which would break any other test reusing the same one.
  - `browser.newContext()` without an explicit `storageState` inherits the `storageState` the file configured through `test.use()`, if any; that is why a context that must start signed out (the "visitor" of `invite.spec.ts`) lives in a file with no `test.use()` at all, with every context explicit about its own state.
  - No test sets the display name of the shared accounts: `ui.spec.ts` expects "Minha conta" (the default text of `user-menu.ts` with no name set), which stays right only while no other test goes through `/profile` on those accounts.
- Each test that proves an acceptance criterion carries the story's or rule's tag, such as `@MR-001`. `npx playwright test --grep @MR-001` runs only that story's.
- `a11y.spec.ts` runs [axe](https://github.com/dequelabs/axe-core) (`@axe-core/playwright`) on the main screens, in the light and dark themes, on desktop and mobile, and fails on any serious or critical violation of the WCAG 2.1 A and AA rules. On the same screen, it runs the alignment checks of `layout.ts`: icon at the height of its own words, button content in the middle of it, nothing on top of an icon, words away from the edge of a radio card (see [Design](docs/design.md#how-a-screen-is-made)). `npx playwright test --grep @a11y` runs only it.
- `ui.spec.ts` signs in through the screen, clicking "Entrar" and "Sair". The other sign-in tests start straight at `/auth/login?return_to=/`, which is faster and keeps the focus on the server.

To add or update an `e2e/` dependency: `cd e2e && npm install <package>@<version>`. `e2e/.npmrc` already prevents install scripts and writes the exact version.

## Screens: design and review

Every new screen, or visible change to a screen, follows the app's [design](docs/design.md), the "paper sheet", and goes through five steps:

1. **Brief:** the screen's task, who uses it, where, the real data and all the states (empty, loading, error, locked, pending).
2. **Design** in Claude Design, with the app's visual system, at 390px and 1280px, approved before the code. A small fix inside the system needs no design.
3. **Code with the tokens** (`--mr-*` in `web/src/styles.scss`) and the common pieces (`web/src/styles/_ui.scss`), never a hand-written colour.
4. **Review through the screen:** screenshots at 390, 768 and 1280px, in the light and dark themes, in each state, checked against the design, and each piece looked at up close (2x): alignment does not show in a shrunk screenshot. The PR carries the before and after screenshots and the filled-in screen checklist.
5. **Automatic check:** the screen enters `a11y.spec.ts`, which runs axe and the alignment checks of `layout.ts`, with no failures.

### Fog measurements

Fog of war (MR-036) computes, per player, what the character sees. `go test -run '^$' -bench FilteredMap -benchmem ./internal/maps` (in `backend/`) measures only the computing part of a player's filtered read (the vision, the per-square states and the filtered layers), without the database: well below 1 ms with the scene and the visions in the cache (the usual read), and also without the cache on the maps measured. `TestFogGetMapTiming` measures the whole `GetMap`, with the database, and shows the times with `-v`. Measured on 04/10/2026 (Apple M1 Pro):

| Map | With cache (the usual read) | Without cache (after a move) |
| --- | --- | --- |
| The cave (24 × 16, one torch, 6 players) | 3.5 µs, 1 KB | 9.4 µs, 1.5 KB |
| 60 × 40, 6 players and 3 lights | 34 µs, 5.8 KB | 166 µs, 8.5 KB |

Memory (`TestSceneMemory -v`): a compiled 200 × 400 scene takes about 430 kB and each vision, 85 kB. The cache holds 8 scenes with up to 24 visions each: at most 19 MB, inside the 512 MiB budget.

### Fog tile measurements

The player of a fog map receives the image in pieces assembled on the server (MR-036, RN-10; [Architecture](docs/architecture.md#per-player-image-tiles)). The numbers below are from an Apple M1 Pro, with test images (a gradient with texture; a real photo compresses worse, and the pieces get bigger); the production server has 1 vCPU, so count on about 2 to 3 times that in time.

| What | Measurement |
| --- | --- |
| First tile of the cave (240 × 160 px, 24 × 16 squares) | About 2 ms; a 2 kB tile; 1 MB of memory |
| First tile of a 4,096 × 2,048 px upload (64 columns) | About 150 ms (decoding and shrinking square by square, most of it) and 11 ms of rendering; the working copy ends up at 2,048 × 1,024 = 8.4 MB; 46 MB peak memory during decoding |
| First tile of an 8,192 × 4,096 px upload | About 460 ms; the working copy is also 8.4 MB; 146 MB peak (the decoded image, 4 bytes per pixel: an upload can have up to 40 megapixels, about 175 MB) |
| A ready tile, from the working copy (`BenchmarkTileWarm`, a fully seen tile of 4,096 × 2,048, PNG) | 10 ms, 2.5 MB of allocation |
| The same on a JPEG map (`BenchmarkTileWarmJPEG`, with the block computation) | 22 ms, 4.2 MB of allocation |
| A cold tile (`BenchmarkTileCold`: decode the PNG, shrink and render) | 128 ms, 44 MB of allocation |
| A request for a tile already in the cache, through the whole route (`TestTileRequestTiming`, with the test's own HTTP and the in-memory database) | 6.7 ms |
| A `304` (`If-None-Match`), through the whole route | 4.8 ms: renders and decodes nothing; it is the cost of the three database reads that check the session, the participation and whether the player sees the map |

**The memory worst case**, against the 400 MiB `GOMEMLIMIT` (and the instance's 512 MiB), with a 40-megapixel upload in 8-bit PNG (the biggest the server stores):

| Part | Memory |
| --- | --- |
| One decoded image in the upload slot (`processing`: one at a time on the server, never coinciding with another): the first fog tile (the decoded image, the file of up to 10 MiB and the copy `io.ReadAll` makes), the upload (which takes the slot **before** it reads its file, so the file is part of this figure and queued uploads hold nothing) and the reference for the AI (MR-039). Measured with 40 megapixels in 8-bit PNG: the first tile about 195 MB; the upload (`images.Process`) **178 MiB** plus the file read and its copy (up to about 20 MB), about **200 MB**; shrinking the reference 172 MiB (the file is read once, into a buffer of the exact size). **The upload figure was underestimated and was measured and corrected:** it used to be 230 MiB, because the thumbnail and the reference used a rescaler workspace of the image's size (77 MB and 163 MB); now they scale in strips of 32 rows (about 8 MB). The 40-megapixel image in JPEG: 78 MiB. **The textured-map crop** (`images.CropFit`, MR-039), which uses the same slot: refuses a model answer more than 4,096 px on a side or that, decoded with the output's working copy, exceeds 178 MiB; the worst accepted case (a 4,096 × 4,096 answer for a 16-megapixel map image) measured **151 MiB** live (`TestCropFitMemory`), inside the slot's roughly 200 MB and not added to it | up to about **200 MB**, transient |
| The working copies: 2 maps of up to 2,048 × 2,048 (16.8 MB in the worst aspect-ratio case) | up to 34 MB |
| The ready-tile cache | 32 MiB (33.5 MB), counted in bytes |
| The fog scenes and per-player visions (`TestSceneMemory`) | up to 19 MB |
| The image generation answers (MR-039): **1 call to the model at a time** (`maxGenerating`), an answer of up to 8 MiB, read straight into a structure (measured: about 2.5 times the answer, about 20 MiB at the ceiling; a 1K image is about 2 MB, and in practice about 5 MiB), and the shrunken images of the request, up to about 6 MiB | up to about 26 MB |
| Sum | 200 + 34 + 33.5 + 19 + 26 = **about 313 MB**, plus about 50 MB for the rest of the server (**about 363 MB**): inside the 400 MiB `GOMEMLIMIT` and the instance's 512 MiB. With 2 calls at a time it would be about 389 MB, which is why it is 1 |

An image whose decoding would exceed 192 MiB (a 16-bit PNG of more than 24 megapixels, from before the upload stored only 8 bits per channel) is refused at the first tile, with `503`; a 40-megapixel JPEG decodes in about 60 MB.

To measure again: `MEURPG_MEASURE=1 go test -run 'TestTileMemory|TestTileRequestTiming' -v ./internal/maps` (the latter with the test database), and, for image generation, `MEURPG_MEASURE=1 go test -run 'TestShrinkMemory|TestMeasureMemory' -v ./internal/maps/images/...` and `go test -run '^$' -bench 'Tile' -benchmem ./internal/maps` (in `backend/`, without `-race`: the detector multiplies time and memory).

### Generated map images

The drawing that goes to the model (`backend/internal/maps/refimg`) and the crop of the result (`images.CropFit`) are pure: `go test ./internal/maps/refimg ./internal/maps/images` runs the aspect-ratio maths (which of the model's 10, the rock fill and the crop back), the black where nobody sees, the creature discs and the crop in a few seconds, without a database. The measurements of the biggest map (200 × 400) are in [Operations](docs/operations.md#generated-images-the-gemini-api): `MEURPG_MEASURE=1 go test -run 'TestMeasureTheBiggestMap' -v ./internal/maps/refimg ./internal/maps` (the drawing, without a database), `-bench BiggestMap -benchmem ./internal/maps/refimg` and `MEURPG_MEASURE=1 go test -run TestCropFitMemory -v ./internal/maps/images` (the worst-case crop, about 150 MiB and a few seconds).

### Map editor in the browser

Painting on a big map must not freeze the screen: `PaintedLayers` decodes the whole grid once per animation frame (`requestAnimationFrame`), not on every square the pointer crosses, and the batches go to the server outside the drawing path (see [Architecture](docs/architecture.md#map-editor)). Measured on 05/10/2026 in Chrome, on an Apple M1 Pro, with a 200 × 400 square map (an image of 1,200 × 2,400 px and the 200-column grid, the biggest the server accepts) and the 3 × 3 brush: a drag of 240 pointer events from one side of the map to the other, over 5.5 s, gave **335 frames with a mean of 16.7 ms (60 frames per second), p95 of 16.7 ms and a maximum of 16.8 ms**: no dropped frame. The drag was driven by Playwright, which sends one event per round trip to the browser; a real mouse sends more events per frame, and they merge into the same frame. To measure again: a Playwright test that creates the map, enters "Pintar", picks the Wall and the 3 × 3 brush, and records the duration of each `requestAnimationFrame` while `page.mouse` drags.

### Dungeon map measurements

Creating the map of a generated dungeon (MR-010, [Architecture](docs/architecture.md#generated-dungeon-maps)) generates, draws, encodes, stores the two files and writes the rows. Measured on 06/10/2026 on an Apple M1 Pro (the machine was busy; without `-race`), with the default options and seed 11:

| Dungeon | Image | The whole `CreateDungeonMap` call | Only the drawing, the PNG, the thumbnail and the files (`BenchmarkDungeonImage`) |
| --- | --- | --- | --- |
| 121 × 121 (the biggest on the page), 134 rooms | 2,904 × 2,904 px (24 per square), 59 KiB PNG, 65 KiB thumbnail | 140 ms | 114 ms, 12 MB allocated |
| 199 × 399 (the biggest the API accepts), 500 rooms | 3,980 × 7,980 px (20 per square, 31.8 megapixels), 223 KiB PNG, 130 KiB thumbnail | 379 ms | 502 ms, 36 MB allocated |

The preview (`PreviewDungeon`) takes 3 ms at 121 × 121 and 10 ms at 199 × 399. The pure drawing (`go test -run '^$' -bench Render ./internal/maps/dungeonimg`) takes 48 ms and 197 ms; the rest is PNG encoding and the thumbnail. The thumbnail of a palette image is a box average done in one pass (`images.thumbnailPaletted`): the generic reducer read the palette pixel by pixel and took more than 2 s. To measure again: `bash dbtest.sh go test -run TestDungeonMapCreationTiming -v ./internal/maps/` (in `backend/`, without `-race`, which multiplies the time) and `go test -run '^$' -bench DungeonImage -benchmem ./internal/maps/`.

## Queries with sqlc

Each module's SQL lives in `backend/internal/<module>/queries.sql`, and sqlc generates the typed Go methods in a package next to it (`identitydb`, `campaignsdb`, `charactersdb`, `playdb`, `mapsdb`). The schema sqlc uses is the goose migrations themselves, so there is no second copy of the schema to keep equal. The configuration is in `backend/sqlc.yaml`.

To change a query or create one:

1. Write the SQL in `queries.sql`, with a comment `-- name: QueryName :one` (or `:many`, `:exec`, `:execrows`) on top.
2. Run `make sqlc` and commit the generated code along with it. CI generates again and fails if there is a difference.
3. Writes go through `db.InTx`, which retries the transaction on CockroachDB's `40001` error: `s.queries.WithTx(tx).QueryName(...)`.
   - **Inside `InTx`, only `WithTx(tx)`.** No read through the pool (`s.queries.X`, `s.pool`, or a call to another module that reads through the pool): it takes a second connection while the transaction holds the first, and a few such requests lock up the whole pool until the context ends. A cross-module call that reads receives the `tx` (first argument after `ctx`; `nil` outside a transaction); what cannot stay inside the transaction is read before it. See [Architecture → Transactions and the connection pool](docs/architecture.md#transactions-and-the-connection-pool). `dbtest` gives every test a pool of **one** connection and fails the test that does this, with the stack.

sqlc is pinned to version **1.31.1**, because the version is written into every generated file. `make sqlc` runs that exact version with `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1`, installing nothing; the first time takes about a minute to compile. Homebrew's `sqlc` at the same version generates the same result.

sqlc reads the migrations with the PostgreSQL parser. So a new migration uses SQL that both PostgreSQL and CockroachDB accept: an index in its own migration, with `CREATE INDEX IF NOT EXISTS`, never an `INDEX` line inside `CREATE TABLE`; a covered column with `INCLUDE` (CockroachDB's `STORING`). CockroachDB options in `WITH (...)`, such as row-level TTL, work. Every migration must be able to run twice: the `TestMigrationsAreSafeToRerun` test checks. **Production is live, so a migration only adds:** a new table, column, index or row, never a rename, a drop or a new `NOT NULL` without a default on something the running code reads. The migration job runs before the new revision gets traffic, so the old code must keep working on the new schema, during the switch and after a roll back ([Operations](docs/operations.md#releasing-a-new-version)); to remove something, first release the code that stops using it, and drop it in a later release. **A new session event kind is a row in `session_event_kinds`**: an `INSERT INTO session_event_kinds (kind) VALUES (...) ON CONFLICT (kind) DO NOTHING` in a new migration (and the `event...` constant in the code; `TestEveryEventKindOfTheCodeIsInTheTable` fails if one is missing). There is no `CHECK` with the list any more, so `session_events` is never re-read for it. `migrate up` and `migrate down` take the `migration_lock` lock (see [Data](docs/data.md#migration-lock)): two at the same time wait for each other. The reference tables (only `session_event_kinds` today) are copied with their rows into the test databases (`referenceTables`, in `backend/internal/platform/dbtest`) and never emptied between tests: a new table of that kind goes into that list. Every `ADD COLUMN` on the `characters` table must set the TTL expression of `00020` again at the end, as `00122` does, because CockroachDB rewrites the expression when a column is added and, without that, the second run leaves the table different.

## Rules content (SRD)

The `rules` module is pure: its tests run without a database, Docker or network, in about a second. It is the fast loop for working on the engine, the effects or the names.

| Command (in `backend/`) | What it does |
| --- | --- |
| `go test ./internal/rules/...` | Runs the tests of the engine, the SRD snapshot and the formulas. |
| `go test ./internal/rules -run Golden -update` | Rewrites `testdata/golden/pensantus.json` after an intentional change to the numbers. Review the diff before committing. |
| `go test ./internal/rules/formula -run '^$' -fuzz FuzzCompileFormula -fuzztime 30s` | Fuzzes the formulas: no input may hang or bring down the engine. CI does not run the fuzz; run it when touching `formula/`. |
| `go test ./internal/rules -run '^$' -bench Derive -benchmem` | Measures one `Derive`, which runs on every sheet read. |
| `go test ./internal/rules/vision -run '^$' -bench . -benchmem` | Measures fog of war: the cave of the design, a 60 × 40 map and the biggest (200 × 400), dark and lit, with a border and pillars and with sparse walls. The budget: the cave well below 1 ms, the 60 × 40 below 10 ms and the 200 × 400 below 300 ms. |
| `go test ./internal/rules -run 'TestWith\|TestLevelUpSweepTable\|TestDeriveTable\|TestSpellDetailsOfTable\|TestTable\|TestThirdCaster\|TestAlwaysPrepared\|TestRaceBonuses\|TestArchived\|TestMissingTable\|TestRealistic\|TestEffectMenu\|TestClassRefusalsNameTheirField\|TestChangedClassSentences\|TestChangeSentence\|TestStrayEffect\|TestThirdCasterUnder\|TestLevelUpSubclassOffers'` | The table's content (`Content.With`, see [Architecture](docs/architecture.md#table-content-contentwith)): the refusals (one line per rule), the SRD that does not change, two contents that do not see each other, `With` in parallel (use `-race`), the one-third caster, the level-up sweep over table classes of each casting type, in multiclass too, the golden of a table character, the 20-level table defaults, the effect menu, the field of each refusal and the "A classe mudou" sentences. No database. |
| `go test ./internal/rules -run TableCharacterGolden -update` | Rewrites `testdata/golden/table-character.json` (the table character at levels 1, 5 and 11). Review the diff before committing. |
| `go test -run '^$' -bench BenchmarkWith -benchmem ./internal/rules` | Measures a `With` with 300 entries (the ADR-0018 budget: about 25 ms and 4 MB) and an empty `With`. Run in `backend/`, without `-race`. `MEURPG_MEASURE=1 go test ./internal/rules -run TestWithMemory -v` measures the memory a table content keeps, and `MEURPG_MEASURE=1 go test ./internal/rules -run TestWithAtTheBudgets -v` checks the time of a content at the limits (below 50 ms; only with the variable, because on a busy machine or in CI a clock test fails for nothing). |
| `go test ./internal/rules/puzzle` | The puzzle maths (MR-038): comparing the riddle and the cipher without capitals, accents or punctuation, the cipher (shift and keyword) round trip, the sequence check and the step-by-step tap, the minimum taps of "Apagar as luzes" in GF(2) against brute force (3 × 3 and 4 × 4), the core cases (4 × 4 and 5 × 5), the never-solved starts from 3 to 7, the lock that wraps around, the pillar search with the links, and the seed that always gives the same start. `-bench .` measures the 7 × 7 minimum and the 6-pillar search. No database. |
| `go test ./internal/rules -run 'Treasure\|Hoard\|Families\|MagicItemValues'` | The treasure generator (MR-044, see [Architecture](docs/architecture.md#treasure-generator-mr-044)): the level bands, determinism, the fixed treasure of a seed (`TestTreasureGolden`), the properties of thousands of treasures (never an artefact, every item exists and has a Portuguese name, the totals add up, the piece limit), the item values (half for the consumable, the whole scroll, the artefact without a price) and loading refusing a malformed table. No database. |
| `go test ./internal/rules -run '^$' -fuzz FuzzGenerateTreasure -fuzztime 30s` | Fuzzes the treasure generator: no seed, level or mode may panic, and every accepted treasure passes the properties. CI does not run the fuzz; run it when touching the tables or the generator. `-bench GenerateTreasure -benchmem` measures a level-17 hoard (about 21 µs, 40 allocations). |
| `go test ./internal/rules -run 'TestTextsPT\|TestSampleTextsPT'` | The Portuguese spell and magic item texts (`effects/spells_pt.json`, `effects/magic_items_pt.json`): the loader's refusals, what the content exposes, and `TestTextsPTCoverEverySpellAndItem`, which fails while any SRD spell or item has no Portuguese text. No database. |
| `go test ./internal/rules/grid ./internal/rules/vision` | The map geometry (grid, layers, straight line, movement cost, reach, cover) and vision: pure, checked against the numbers of the cave in the Etapa 9 designs. They run without a database. |
| `go test ./internal/rules/dungeon` | The dungeon generator (MR-010): properties over many options (all the specification's invariants), the golden hashes, the number generator's vector, the independence of the streams and the edge cases. Pure, no database, about 4 seconds (about 32 with `-race`); `DUNGEON_SWEEP=1` also runs a sweep of 20,000 combinations. |
| `go test ./internal/rules/dungeon -run Golden -update` | Rewrites `testdata/golden.json` (one grid hash per case) after an intentional change to the generator's output. Changing the output requires bumping `Version` in `types.go`; review the diff before committing. |
| `go test ./internal/rules/dungeon -run '^$' -fuzz FuzzOptions -fuzztime 60s` | Fuzzes the generator's options: no input may panic, and every accepted output passes all the invariants. CI does not run the fuzz; run it when touching the generator. |
| `go test ./internal/rules/dungeon -run '^$' -bench 'Generate|Worst|PromptOf' -benchmem` | Measures the generator from 31 × 31 up to 199 × 399, the constructed worst cases (`Worst`) and `PromptOf` in the worst case (`PromptOf`). The budget: the biggest below 50 ms (ceiling 250 ms) and `PromptOf` below 20 ms. The tests only check these times with `MEURPG_MEASURE=1` and without `-race`, on a quiet machine: on a busy machine, or in CI, a clock test fails for nothing. |
| `go test ./internal/rules/encounter` | The encounter builder maths (MR-043): the party budget from the SRD 5.2.1 table, the band (below "Baixa" is still "Baixa"; above "Alta" is allowed), the maximum CR (the lowest level plus 3), the generator (deterministic by seed, never above the budget or with a CR above the ceiling, spends what it can, a leader and a group of one or two creatures, over many seeds, budgets and types) and the swaps for the same XP. Pure, no database. `go test ./internal/rules -run 'Encounter'` runs the loaded table (`effects/encounter_budget.json`) and the numbers of design E10-09 over the real SRD. |
| `go test ./internal/rules/encounter -run '^$' -fuzz FuzzGenerate -fuzztime 30s` | Fuzzes the encounter generator: no input panics, the cost never exceeds the budget and the same seed gives the same encounter. CI does not run the fuzz; run it when touching the generator. |
| `go test ./internal/rules/encounter -run '^$' -bench . -benchmem` and `go test ./internal/rules -run '^$' -bench GenerateEncounter -benchmem` | `GenerateEncounter` measures the generator over the 334 real SRD creatures, with the measurement of the result: about 1.1 ms and 2.6 MB per encounter (Apple M1 Pro, 06/10/2026); the `encounter` package's `BenchmarkGenerate` uses a test set of 334 creatures and is only for comparing changes. |

The content lives in `backend/internal/rules/srd51`:

- `data/` is generated by `cmd/srdimport` from the 5e-database (`packages/5e-database/src/2014/en` of the `5e-bits/5e-srd-api` repository), at a pinned commit. **Never edit by hand:** `TestSnapshot` compares each file with the sha256 in `manifest.json`. The input files are 20, and `5e-SRD-Monsters.json` (the 334 creatures, MR-037) generates `monsters.json`, about 740 kB (97 kB compressed); the importer refuses a total other than 334. `5e-SRD-Spells.json` also gives each spell's area (`area_of_effect`: the shape and the size in feet, 88 of the 319 spells), which goes to `spells.json` as `area_type` and `area_size_ft`; the importer refuses a shape that is not cone, cube, cylinder, line or sphere and a size that is not a multiple of 5, and loading (`rules`) checks again. With the structured area, `characters` does not read the SRD text to know whether a spell takes an area; the text is only a fallback, for the spells without one (see [Architecture](docs/architecture.md#spell-targets-and-the-magias-page)). The twentieth, `5e-SRD-Magic-Items.json` (the 362 magic items of SRD 5.1, MR-044), generates `magic-items.json`: per item, the key (`item:<index>`), the SRD name, the category, the rarity (`common`, `uncommon`, `rare`, `very_rare`, `legendary`, `artifact` or `varies`), whether it needs attunement (and by whom, in English, as in the SRD), the variants and the SRD text in English. The importer refuses: a total other than 362 (239 items and families, 123 variants); a category or a rarity it does not know; an entry without text or repeated; a family that lists a variant that does not exist or is not a variant; a variant in two families, without a family, with variants or with rarity `varies`; and a rarity `varies` without variants. Loading (`rules`) checks everything again on what is in `data/`. It enters the same pinned commit, so only `manifest.json` and `magic-items.json` change in `data/`.
- `effects/` is handwritten: the effects of each feature and trait (`<class>.json`, `races.json`, `backgrounds.json`); the Portuguese names (`names_pt.json`), with the 362 magic items as `item:<index>` and the label of each attunement restriction as `attunement:<restriction>` (`TestMagicItemNamesPT` fails if one is missing); the single-use magic items (`consumables.json`: the categories and the items, which loading checks against the items); and the revision (`revision.json`).

### Updating the SRD to a new commit

1. Download the JSON files at the new commit, to a folder outside the repository:

   ```bash
   git clone --filter=blob:none --no-checkout https://github.com/5e-bits/5e-srd-api.git /tmp/5e-srd-api
   git -C /tmp/5e-srd-api sparse-checkout set --no-cone packages/5e-database/src/2014/en
   git -C /tmp/5e-srd-api checkout <commit>
   ```

2. In `backend/cmd/srdimport/main.go`, change `sourceCommit` and the `inputHashes` table (`shasum -a 256 /tmp/5e-srd-api/packages/5e-database/src/2014/en/5e-SRD-*.json`). The importer refuses any file with another sha256.
3. Generate the snapshot: `cd backend && go run ./cmd/srdimport -src /tmp/5e-srd-api/packages/5e-database/src/2014/en`.
4. Change the commit in `NOTICE` (`TestSnapshot` checks).
5. Run `go test ./internal/rules/...`. `TestReferences` shows broken references, `TestEffectsCoverLevels1To5` and `TestNamesPT` show what was left without an effect or a name, and the golden shows the numbers that changed.

### Changing an effect or a name

- A content version never changes in place (ADR-0008). After editing any file in `effects/`, `TestSnapshot` fails and tells you the new `revision` and `sha256` to put in `effects/revision.json`; the `content_version` goes from `srd51@<commit>+fx.<n>` to `fx.<n+1>`. The same goes for a change in `data/` (`TestSnapshot` only checks the hash of `effects/`, so the revision goes up by hand, with the same `sha256`). The revisions so far: the spells' area is `fx.14`, the creatures' multiattack corrections `fx.15`, the encounter budget table `fx.16`, the item values and the treasure tables `fx.17`, and the Portuguese names aligned with the official terms of the Brazilian editions of the Player's Handbook and the Dungeon Master's Guide `fx.18`, the Portuguese texts of the spells and magic items `fx.36`, and the 10 ft reach of the weapons with the reach property (the importer sets it; the source gives every melee weapon 5 ft) `fx.19`, and the experience of the four creatures whose source gives half the XP of their challenge rating (the importer reads it from the rating) `fx.20`, and the corrections of spells (attack, saving throw, damage by slot), of the spells a subclass always has and of creature stat blocks `fx.21`, and the class features of levels 6 and up as effects (Extra Attack at 11 and 20, Aura of Protection, the uses of Indomitable and the other limited features, Archdruid, Beast Spells, the scores of Primal Champion, the recharge of Bardic Inspiration by level) `fx.22`, and the spell engine's content (the damage types a spell lets the caster pick, the Fiend's expanded spell list, False Life and Aid as temporary and maximum hit points, Aid's three targets and Flame Strike's higher-slot damage, and the official names of Blight and Sleet Storm, which repeated other spells' names) `fx.23`, and the ki cost of Flurry of Blows, Patient Defense and Step of the Wind, with the Superior Critical entry `fx.24`, and the spell lists of the bard, the cleric and the druid as the SRD 5.1 gives them, the Circle of the Land's terrain choice and circle spells in the level rows, the Channel Divinity and Cutting Words actions that spend their uses, and the `replaces` tiers of the scaling features `fx.25`. and the feats (the SRD's Grappler, imported from `5e-SRD-Feats.json` with its effects and its Portuguese name, and the `ability_increase` effect and the `feat` choice the table's feats use) `fx.26`, and the tiefling's Infernal Legacy as a resource (a use per long rest from the 3rd level, with its Portuguese name) for the reaction window `fx.30`, and the Bard's multiclass instrument choice (the ten instruments of the SRD's Multiclassing proficiencies table, which the snapshot lists and the importer leaves out) `fx.31`, and the Brutal Critical and Reliable Talent features as effects the engine reads (`Derived.BrutalCriticalDice`, `Derived.ReliableTalent`) `fx.32`, and the damage resistances of the races and of the Rage (`damage_resistances.json`), with Reckless Attack as a free action `fx.33`, and the class and race choices (`effects/choices.json`, the Devil's Sight sense, the Great Weapon Fighting and Two-Weapon Fighting notes) `fx.34`, and the `revive` kind of spell effect (Revivify: 1 hit point, a window of 10 rounds) `fx.35`, and the creature type names "Construto" and "Gosma" (the official terms, in the Favored Enemy options too) `fx.37`, and the effects that last (`combat_effects.json`: the 15 conditions with who reads each, Bless, Bane, Haste with its lethargy, Hold Person, Faerie Fire, Hideous Laughter, Web, Pass without Trace, Longstrider, Guidance, Resistance, Enhance Ability and Heroism as effects with a duration, the clock in rounds, the end-of-turn saves, exhaustion) with the Portuguese names of their effects `fx.38`.
- The effect types are closed and loading refuses the rest. Formulas only use `level()`, `classLevel("wizard")`, `mod("int")`, `score("int")`, `prof()`, `armor()`, `shield()`, `floor`, `ceil`, `min` and `max` (see [Architecture → Rules module](docs/architecture.md#rules-module-rules-as-data)).
- `effects/feats.json` holds the effects of the SRD's feats (`feat:grappler`); the feat itself, its text and its ability score minimum come from the importer (`data/feats.json`, from `5e-SRD-Feats.json`), and its name from `names_pt.json`. Loading refuses a feat that is not in `data/feats.json`, a prerequisite with an unknown ability, proficiency or race, and an `ability_increase` that is not a whole 1 or 2 on a list of distinct abilities (the table's menu also keeps `ability_increase` to feats), and the file follows the revision rule above.
- `effects/advancement.json` holds the SRD's experience tables: the XP of each level (1 to 20) and the XP of each challenge rating (0 to 30). Loading refuses a table out of format, and changing any number follows the revision rule above.
- `effects/encounter_budget.json` holds the SRD 5.2.1 "XP Budget per Character" table (the 2024 rules, CC BY 4.0, p. 201): the XP per character of each level (1 to 20) in the low, moderate and high bands, with the source in the file. Loading refuses a missing or out-of-order level, a row that does not grow from low to moderate to high, a level that costs less than the previous one in some band and an unknown field, and the file follows the revision rule above (`fx.16`). It is one of the three SRD 5.2.1 tables the app uses (the others are the ways to make ability scores and the magic item values by rarity), each credited in `NOTICE` and labelled "SRD 5.2.1 (regras de 2024)" on the screen.
- `effects/standard_actions.json` holds the ten actions every character has (Attack, Dash...); the resources' Portuguese names live in `names_pt.json` as `resource:<name>`. Both follow the revision rule above.
- `effects/spells.json` holds what the spells that read HP do (Sleep, Color Spray, Power Word: Stun and Kill, Spare the Dying, Heal), in six closed types: `hp_pool`, `hp_threshold`, `zero_hp_target`, `flat_heal`, `temp_hp` and `max_hp` (see [Architecture → Spells that read HP](docs/architecture.md#spells-that-read-hp)). Loading refuses a spell that is not from the SRD, an unknown type, a field the type does not use and a condition that does not exist, and the file follows the revision rule above. A new spell of this kind also needs a computation in `rules/combat/hpspells.go` and its test. A spell outside SRD 5.1 (such as Danse Macabre) never enters. The `ignores_cover` type (with no other fields) marks the spell whose saving throw gets no cover bonus, such as Sacred Flame (SRD 5.1; see [Movement in combat](docs/architecture.md#movement-in-combat)); it does not read HP, and `Content.IgnoresCover` answers it. The `revive` type (Revivify, SRD 5.1) takes `hit_points`, `window_rounds` (a minute is 10 rounds: SRD 5.1, "The Order of Combat") and the `source` it comes from, and nothing else; loading refuses a revive without them and the revive fields on any other type; `Content.Revive` answers it, and `play` (`revivify.go`) casts it.
- The same file has the fifth type, `summon`, for the spells that summon creatures (Find Familiar, Animate Dead, Conjure Animals; Find Steed is left out). Either it lists the creatures (`creatures`, with `count`, `count_per_level` and what each one may do, `attack`: `none`, `reaction` or `full`, and `unlocks` for a feature, such as Pact of the Chain, which adds forms and gives all of them the unlock's `attack`), or it gives CR bands of a type (`type`, `options` with `count` and `max_cr`, `multiplier_at_level`). Loading refuses a creature that does not exist (and a creature sheet with an attack count below 1, a Multiattack action that does not exist or armour that is not equipment), a CR that is not from the SRD, an unknown field and a type that is not a creature type. The casting time, the ritual and the concentration come from the spell, not from the file. A new spell of this kind needs its test in `rules/summon_test.go`.
- A `resource` effect may carry `recharge_if` (a Bool formula) and `recharge_then` together, to recharge differently from a level on; a `modifier` on a score (`score.str`...) is an `add` with a `cap` above 20; `save.all` is a modifier target; `beast_spells` is the druid's level 18 effect and takes no field. Loading refuses the rest.
- A `replaces` effect, on a higher tier of a scaling feature (Channel Divinity twice between rests, Extra Attack 2, the Bardic Inspiration die...), names the lower tier's feature key: the sheet lists only the newest tier, while the lower one's effects still count. It takes that key and nothing else.
- A `grant_action` may carry `resource`, the key of a resource of another feature that each use spends (the monk's Flurry of Blows, Patient Defense and Step of the Wind spend `ki`); without that resource at the character's level the action is not offered.
- `wild_shape` is the effect type of the druid's Wild Shape, in `effects/druid.json`, at the three levels (2, 4 and 8): `max_cr` (an SRD CR, such as `"1/4"`), `no_fly` and `no_swim`. Loading refuses a `max_cr` that is not a CR, any other field in the effect, and `max_cr`, `no_fly` or `no_swim` in another type. The creature names (`monster:<index>`, 334) live in `effects/names_pt.json`, and `TestNamesPT` checks all 334. No creature from another book enters. The Portuguese names of the attacks of the creatures a character can have (beasts up to CR 2, the undead of Animate Dead and the familiar's forms) are `attack:<SRD name in lower case, with hyphens>` (`attack:bite` is "Mordida"): the server puts them in `Attack.name_pt` and `CreatureAction.name_pt`, `TestAttackNamesPT` checks that none was left without a name and loading refuses an `attack:` that no creature uses; the attack text (`notes`) stays the SRD's, in English. `TestAttackNamesPT` checks every attack of the 334 creatures, because the bestiary's "Criar NPC" copies them to the sheet. A change here follows the revision rule above.
- `effects/traps.json` holds the SRD's eight example traps (Simple pit, Hidden pit, Poison needle, Poison darts, Collapsing roof, Fire-breathing statue, Falling net, Rolling sphere), with the SRD's severity tables (DC and attack bonus; damage by level). Each trap has `kind`, `description_pt` (in our words), the DC to notice (`notice_dc`, optional) and to find (`find_dc`; the SRD only gives the DC to notice for the Simple pit, the Collapsing roof, the Statue and the Net, so the find DC is ours and carries `find_dc_ours: true`), the `trigger` (`enter` or `manual`), the `area_size` (1 to 4), the `targets` (`area` or `manual`) and the effect parts: an `attack`, `damage` that always hits, `conditions` that always apply (the pits leave Prone) and a `save` (ability, DC, `applies_to`, what happens on a failure and `on_pass`: `half` or `none`). Loading refuses an unknown field, a condition or damage type that does not exist, a save without an ability, "half" without damage on a failure, a pit whose fall is not 1d6 per 10 ft and a trap without a name in `names_pt.json` (`trap:<key>`), and the file follows the revision rule above. The numbers come from the pinned commit's `5e-SRD-Rules.json` (the `sample-traps` section); only SRD traps enter.
- `effects/spell_targets.json` holds the target of some SRD spells where the database's structured area or the text is wrong (`creature`, `creatures` with a number, `area`, `self`, `none` and `label_pt`): every key is an SRD spell, the types are closed, the text is ours, and the file follows the revision rule above. The other spells follow the structured area and, without it, the text (see [Architecture](docs/architecture.md#spell-targets-and-the-magias-page)).
- `effects/lights.json` holds the SRD lights in feet (`bright_ft` and, beyond that, `dim_ft`): Candle, Torch, Lamp, Hooded lantern, the Light spell, Continual Flame and Daylight, with the names in `names_pt.json` (`light:<key>`). The bullseye lantern (a cone) is left out: the app only lights circles. Loading refuses a negative or zero radius, a radius that is not a multiple of 5 ft, and a light without a name or without a duration. The file follows the revision rule above.
- `effects/magic_item_values.json` holds the magic item values in gp, the "Magic Item Rarities and Values" table of SRD 5.2.1 (p. 205, CC BY 4.0, 2024 rules), with the source: common 100, uncommon 400, rare 4,000, very rare 40,000 and legendary 200,000 (the artefact has no price), and the consumable divisor (2). It is the only part of SRD 5.2.1 that the treasure uses. Loading refuses a rarity without a value, values that do not rise, a value for the artefact and a divisor below 1, and the file follows the revision rule above (revision 16). The magic spell Scroll is not divided (an engineering decision); the code that applies this is `Content.MagicItemValue`.
- `effects/treasure.json` holds **our own tables** of the treasure generator (MR-044): the value steps of gems (6 gp and three times the previous, up to 4,400 gp) and art objects (the same ratio shifted half a step, from 9 to 6,600 gp), each with Portuguese names we wrote (real stones, grouped by how easy they are to find; art objects we invented), and, per party-level band (1 to 4, 5 to 10, 11 to 16 and 17 to 20), the coins of the individual treasure and the hoard (type, roll such as `2d6*100`, chance) and, in the hoard, how many gems, art objects and magic items, with the weights of the steps and rarities. SRD 5.1 has no random treasure tables, so the numbers and names are ours, with no book or other-site table (the repository is public). Loading refuses a band that leaves a level out, a roll that cannot be read, a coin, step or rarity that does not exist, the artefact and `varies`, a repeated name, a step above 10,000 gp, a name over 60 characters, an individual treasure without coins, a draw with steps and rarities together, a treasure over 12 gems, 6 art objects, 6 items or 1,000,000 gp of gold, and a rarity without an item; the file follows the revision rule above. The gold targets and the scales are in [Architecture](docs/architecture.md#treasure-generator-mr-044). Changing a table or an item name changes the treasure of every seed (`TestTreasureGolden` warns), so the revision goes up: a seed is only valid within one content version.
- `effects/damage_resistances.json` holds the damage resistances a character has by race or feature: the tiefling's fire, the dwarf's poison, the ten draconic lineages, and the Rage's bludgeoning, piercing and slashing (`while: "rage"`). Each entry has the `source` (`trait:<name>` or `feature:<name>`), the damage types, the content that grants it and, for the Rage, the state it needs. Loading refuses an unknown damage type, a source that does not exist and an unknown field; the file follows the revision rule above. The same resistance from two sources counts once (SRD 5.1, "Damage Resistance and Vulnerability"); `combat.Adjust` applies it.
- `effects/choices.json` holds the data of the class and race choices (RN-33): the Draconic Ancestry table (damage type, breath weapon shape and save, the Sorcerer's Dragon Ancestor), the breath weapon dice by level, the prerequisites of the 32 invocations, the Favored Enemy types, the Natural Explorer terrains, the racial resistances and one Portuguese line per option, each row with its SRD source. Loading refuses a row without source, a key the content lacks, a bad shape, an option without its line and a field it does not know (`TestChoiceDataIsClosed`), and the file follows the revision rule above (`fx.34`).
- `effects/spells_pt.json` and `effects/magic_items_pt.json` hold the **Portuguese texts** of the SRD: for each spell (`"spell:<index>"`) `desc`, `higher_level` (lists of paragraphs) and `material` (a string), and for each magic item (`"item:<index>"`) `desc`. They are **our own translation of the SRD 5.1 English text in `data/`**, an adaptation as CC BY 4.0 allows (credited in `NOTICE` and on the app's "Créditos" page): written from the English alone, in our own words, **never** from the text of the Brazilian books (the Livro do Jogador, the Guia do Mestre or any other publisher's); only names and short terms follow their official terms, as in `names_pt.json`. Units are metric, as the app shows them (5 ft = 1,5 m; 1 lb = 0,5 kg; decimal comma, thousands dot), and the markdown is kept as it is (bold, italics, `- ` lists, `|` tables with the same columns and rows). Loading is strict: a key that is not an SRD spell or item, a JSON field the format does not know, a paragraph count different from the English one, an empty paragraph, and a `material` on a spell with no material component (or a spell with one and no `material`) refuse to load. An entry the files lack falls back to the English on the screen, flagged (`text_pt_missing`, `description_pt_missing` in the API); `go test ./internal/rules -run TestTextsPTCoverEverySpellAndItem` is what asks for every one (it fails while any of the 319 spells or 362 items lacks its text). The files follow the revision rule above. The app shows the Portuguese text first, with a "Ver em inglês" button (see [Architecture](docs/architecture.md#portuguese-texts-of-the-srd)).
- `effects/corrections.json` fixes what the 5e-database snapshot gets wrong or lacks against the tables and text of SRD 5.1, each entry with its `source` line. `corrections` are class table columns (the Warlock's known invocations). `creature_corrections` are closed fields of a stat block, one value each: `attacks_per_action` (fx.15), `armor_class`, `hit_points` with `hit_points_roll` (same dice as the creature), the speeds (`speed_walk`, `speed_fly`, `speed_swim`, `speed_climb`, `speed_burrow`, 0 removes one), the senses (`darkvision`, `blindsight`, `tremorsense`, `truesight`, 0 removes one), `passive_perception`, `skills` (a map of skill key to total bonus, added over the listed ones), `damage_immunities` (the whole list of damage types) and `condition_immunities` (the whole list of conditions). `spell_corrections` give a spell an `attack_type` (`melee` or `ranged`), a saving throw (`save_ability` with `save_success`: `half`, `none` or `other`) and a `damage` table by slot level that replaces the snapshot's (every upcast row listed, dice that parse, a slot from the spell's level to the 9th). A spell whose effect comes on later turns or on a trigger gets its dice recorded without a saving throw, so a cast does not roll it at once. `spell_list_corrections` add spells to (`add`) or take them off (`remove`) a class's spell list (the bard's Faerie Fire; the cleric's Divination and Arcane Eye; the druid's Meld into Stone, Create Food and Water and Divination), and refuse a spell added that is on the list, one removed that is not, a spell named twice and a class corrected twice. `subclass_feature_corrections` add features to a subclass's row at a class level, creating the row when the snapshot has none (the Circle of the Land's terrain at 2 and its circle spells at 3, 5, 7 and 9), and refuse a feature that does not belong to the subclass or that the row already has. `subclass_corrections` add spells at a class level to the ones a subclass always has (`add_spells`), and `expanded_list_corrections` mark a subclass whose spells only join the class list to choose from (the Fiend). A spell correction may also say what the caster picks among a spell's damage types (`damage_choice`: `scale`, every type is dealt and the higher-slot dice go to the picked one; `alternative`, only the picked one is dealt). `TestMultiattackCountsFollowTheSRDText` reads the Multiattack text of every creature and checks the engine's number, with an explicit list of the five the text does not let it read (each with the reason). The loader applies the corrections over the snapshot and refuses an unknown class, creature, spell, subclass, field or level, a value that does not fit its field, a repeated correction and a correction without a source (`TestCorrectionsAreClosed`, `TestCreatureCorrectionsAreClosed`, `TestStatBlockCorrectionsAreClosed`, `TestSpellAndSubclassCorrectionsAreClosed`, `TestSpellListAndSubclassFeatureCorrectionsAreClosed`), and the file follows the revision rule above. `data/` is never edited by hand.
- Names and texts in `effects/` are ours, in Portuguese. No text from a book outside the SRD enters here: the repository is public.
- The Portuguese names (`names_pt.json`) follow the official terms of the Brazilian editions of the Player's Handbook and the Dungeon Master's Guide (names and short terms only, never the books' text), with two exceptions: the product owner's choice ("Derrubado" for Prone) and the SRD 5.1 name when the official one carries a WotC proper name the SRD removed ("Esfera Congelante", not "... de Otiluke"). A new or renamed entry checks its name against the glossary, and `TestNamesPTAreUnique` fails when two spells (or creatures, items, races, classes, subclasses or backgrounds) share a name. The app's text uses the same terms: "habilidade" for ability (never "atributo"; never for a skill or a feature), "nível da magia" or "1º nível" for a spell level (never "círculo"; where it could be confused with the character's level, write "nível da magia" and "nível do personagem") and "teste de resistência" for a saving throw (never "salvaguarda"). The browser still keeps a list of the condition names (`web/src/app/core/combat/conditions.ts`): when a name changes in `names_pt.json`, change it there too.

## Go linters and the ratchet

**New findings fail the build; old ones do not.** `backend/.golangci.yml` enables more linters than the existing code satisfies, so CI judges only the lines a change touches: on a pull request, the diff against the merge base with the base branch (`--new-from-merge-base=origin/<base>`); on a push to `main`, the pushed commits (`--new-from-rev=<before>`), so `main` is never judged as a whole. The lint job checks out the whole history (`fetch-depth: 0`) for that. Fixing old findings is always welcome and never required: every fix lowers the numbers `make lint-all` prints.

| Linter | What it asks for | Setting and why |
| --- | --- | --- |
| `sloglint` | Log calls: a fixed message per call site, `snake_case` keys, no mixing of `"k", v` pairs with `slog.Attr`, the `...Context` variant when a `ctx` is in scope | See [Architecture → Logs](docs/architecture.md#logs). `slog.LogAttrs` with `slog.Attr` is allowed (typed, no allocation). A message built from a variable needs `//nolint:sloglint // why`. |
| `nolintlint` | Every `//nolint` names its linter, says why, and still suppresses something | A stale directive is a finding: remove it when the code no longer needs it. |
| `thelper` | A test helper starts with `t.Helper()` (or `b.`/`tb.`) and takes `t` first | The `tb` parameter name is not enforced (`testing.TB` parameters may be called `t`). |
| `tparallel` | A parallel test's subtests call `t.Parallel()` too | Add it only when the subtests share nothing mutable; otherwise make the parent not parallel, or add `//nolint:tparallel // why`. |
| `usetesting`, `copyloopvar`, `intrange` | `t.TempDir()`, `t.Context()`, no `x := x` in loops, `for i := range n` | Go 1.22+ idioms. |
| `errname` | `ErrFoo` for sentinel errors, `FooError` for error types | |
| `gocritic` | The default checks plus the `diagnostic` and `performance` tags | `rangeValCopy` and `hugeParam` are off (not a bottleneck here). |
| `forcetypeassert` | `x.(T)` needs the `, ok` form | Tests are exempt. |
| `gocyclo` (30), `gocognit` (50) | Functions that are hard to follow | 128 functions are over gocognit's default of 30; 50 flags only the worst 49. Tighten the numbers as those functions are split. Tests are exempt. |
| `mnd` | A bare number in an assignment, case, condition or return | Arguments and operations are not checked (too noisy); `time.*`, `context.With*`, `strconv.*`, `make` and `math.*` calls, file modes, 1000 and 1024 are ignored; tests are exempt. The fix is a named `const` with a comment saying what the number is. |
| `goconst` | The same string 5 or more times (5+ characters) | `snake_case` identifiers (reasons, enum names) are ignored; tests are exempt. |
| `godot` | A declaration comment ends with a period | A command line in a doc comment goes in an indented block. |

Not enabled, on purpose: `wrapcheck` (the authz errors are `*connect.Error` on purpose and stay unwrapped for `rpclog`'s type switch), `gochecknoglobals` (every global is a read-only lookup table), `funlen`, `nilnil` and `exhaustive` (decided per enum).

The backlog on 07/10/2026 (`make lint-all`, 294 findings): `mnd` 154, `gocognit` 49, `gocritic` 36, `tparallel` 12 (DB test matrices), `forcetypeassert` 11, `intrange` 11, `thelper` 11 (one-line closures), `gocyclo` 10.

### Coverage

Each `go-db` group runs `go test -race -covermode=atomic -coverpkg=./...` and uploads its profile as the artifact `go-cover-<group>` (kept 3 days). `-coverpkg=./...` means a store tested through its service counts for the store's package. The `go (coverage)` job merges the profiles (a block is covered if any group ran it) with `.github/scripts/go-coverage.sh`, prints the total and a per-package table in the job summary, and fails if a package in `FLOOR_PACKAGES` is under `FLOOR_PERCENT` (90). `formula` (81 %) and `srd51` (generated data) are not in the list; `formula` joins it when its tests reach 90 %. Locally: `cd backend && go test -covermode=atomic -coverpkg=./... -coverprofile=c.out ./internal/rules/... && ../.github/scripts/go-coverage.sh c.out`, or `go tool cover -html=c.out`.

Cost: `-race -covermode=atomic` doubled the time of the pure `rules` package on an Apple M1 Pro (32 s to 65 s); `-coverpkg=./...` added nothing on top. Tests bound by the database lose less. If a group nears the 30-minute job limit, move a package between groups in `.github/scripts/go-db-shard.sh`.

## Web and e2e linters

**The same ratchet as the Go linters: new violations fail, old ones are a recorded backlog.** Every violation that existed when ESLint came in is listed in `web/eslint-suppressions.json` and `e2e/eslint-suppressions.json` (ESLint's bulk suppressions: a count per file and rule), so CI fails only when a file gets a violation it didn't have.

- Never grow a suppressions file. Fix the new violation; if it is really unavoidable, an `// eslint-disable-next-line <rule> -- why` on that line, explained in the PR.
- When you touch a suppressed spot, fix it, run `npx eslint --prune-suppressions` and commit the smaller file.
- `eslint --suppress-rule <rule>` is only for turning on a new rule: enable it, record what exists, fix it over time.
- A magic number in app code becomes a `const` whose name (and comment) says what it is. -1, 0, 1, 2, the radix 10, 100 and 1000, default values, enum members, readonly fields and array indexes are allowed; specs are exempt.

The backlog on 07/10/2026 (`web/`, 1,123 entries): `no-magic-numbers` 589, `prefer-on-push-component-change-detection` 378, `complexity` 66, `no-output-native` 30 (outputs named like DOM events), template `interactive-supports-focus` 24, `no-explicit-any` 13 (specs), `await-thenable` 10 (specs) and a few others. `e2e/`: 109 entries (`no-conditional-in-test` 40, `no-explicit-any` 29, `no-force-option` 20, and others). `expect-expect` is off in `e2e/`: the tests assert through helpers.

The web coverage on 10/10/2026: 90.0 % of lines, 88.2 % of statements, 84.4 % of branches and 83.6 % of functions, with no threshold yet.

## Branches

Create a branch from `main`: `feat/`, `fix/`, `docs/` or `chore/` and a short name. Whoever has no write access works in a fork, such as `vfraga/meuRPG`.

## Commits

Commits in English following [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(characters): lock sheet on first session
```

## From code to merge

1. Create the branch (see "Branches" above).
2. Write the code, the tests and the documentation in the same PR.
3. Make the commits in Conventional Commits.
4. Open the PR citing the story (`MR-006`) and the rule (`RN-01`) it fulfils. Use the [PR template](.github/pull_request_template.md).
5. CI must be green. Until the MVP, the author squash-merges the PR as soon as CI is green, without waiting for a review; after the MVP, Samuel reviews first.
6. A single PR is squash-merged. A **merge train** (stacked PR branches, each opened against the previous one) is merged with merge commits, in order, so each branch keeps its history. Never force-push a branch that others build on.

Summary: fork, if needed → PR to `PuraFome/meuRPG` → green CI → squash merge (merge commit for a train).

## What CI checks

| Job | Checks |
| --- | --- |
| backend | `buf lint`, `buf format` and `buf breaking` (against `main`); generated code equal to the `.proto` files' and the queries' (`make sqlc`); `golangci-lint` v2.14.0 as a ratchet (see [Go linters and the ratchet](#go-linters-and-the-ratchet)); `govulncheck` (dependencies with known flaws). The `go` job does not run `go test`: `go-db` runs every package, also those that use no database, so running them again only repeated the work. The Docker image build moved to the e2e `images` job. |
| web | `npm ci --ignore-scripts` in `web/`, `npm run lint` (the ratchet), `npm run format:check` (Prettier), the Angular tests and the build. The coverage runs in its own job, `coverage (report only)` (25 minutes, in parallel), so it never cancels or skips the build and never decides the verdict: the job keeps `continue-on-error`, and it is not a required check, but a failing test fails its step, so the job shows red (it does not fail the workflow run). Its summary goes to that job's summary and `web/coverage/` to the `web-coverage` artifact (7 days). It takes about 5 minutes against about 100 seconds for the plain run (the v8 instrumentation costs; both run the same tests) |
| backend (`go-db`) | Four machines at the same time (`go-db play`, `maps`, `characters` and `rest`), each with its own CockroachDB: the job takes as long as the slowest group, not the sum (it was 1105 s on one machine). The groups are in `.github/scripts/go-db-shard.sh`, with the heavy packages by name and `rest` holding everything else, so a new package lands in `rest` by itself; each machine checks (`--check`) that the groups together are exactly `go list ./...`. The `go (integration tests against CockroachDB)` job only waits for the four and is the stable check for a branch protection rule. Each group runs `go test -race` with `MEURPG_TEST_DATABASE_URL` pointing at a real CockroachDB (the same image, pinned by the same digest, as `compose.yaml`), so the integration tests run instead of being skipped; with `MEURPG_REQUIRE_DB=1` (the job sets it), a missing variable makes a test **fail** instead of skipping, so the job is never green without running the database (`backend/internal/platform/testenv`). |
| e2e (`lint and typecheck`) | Without Docker, before the images: `npm run typecheck` and `npm run lint` in `e2e/`. |
| e2e | On six machines at the same time (the accessibility tests, `@a11y`, which are the long ones, on four, and the rest on the other two, with `--grep`, `--grep-invert` and `--shard`), starts the local environment with `docker compose up --no-build` (with `deploy/local/compose.ci.yaml`, which keeps the database in memory, and `compose.ci-images.yaml`, which points at the ready images). Before them, the `images` job builds the production image and the devidp's **only once**, with the GitHub Actions layer cache (`type=gha`), and hands both over as an artifact (1 day); each machine runs `docker load`. A broken Dockerfile fails there, before any shard. The job checks that the production image does not have the devidp (on one of them) and runs the Playwright tests of `e2e/` on Chromium. If it fails, it shows the environment logs and keeps that part's Playwright report as an artifact for 7 days (`playwright-report-0` to `-5`). A documentation-only change (`docs/`, `.md` files) does not run this job. |
| backend (`go (coverage)`) | Merges the coverage profiles of the four `go-db` groups (`-race -covermode=atomic -coverpkg=./...`), writes the total and a per-package table to the job summary (no outside service), and fails if one of the pure rules packages (`rules`, `combat`, `grid`, `vision`, `dungeon`, `encounter`, `puzzle`) is under **90 %**. The other packages are reported without a floor. See [Coverage](#coverage). |
| workflows | `actionlint` and `zizmor` (workflow security), at versions pinned by number and sha256, when something in `.github/` changes. |
| codeql | CodeQL (the `security-extended` queries) for Go (built from `backend/`), JavaScript/TypeScript and the GitHub Actions workflows, on PRs that touch them, on every push to `main` and weekly; findings show in the Security tab (code scanning). |

Every job has `timeout-minutes` (proto 10, go 15, web 15 (test and build), web coverage 25 (report only), go-db 30 per group, go coverage 10, images 15, e2e 30, workflows 5, codeql 30), so a stuck test or build doesn't burn the 6-hour default. In `go-db`, each package's `-timeout 25m` fits in the job's 30. A new run of the same PR cancels the previous one; a push to `main` never cancels, so every merge leaves a complete result.

Every GitHub action is pinned by commit SHA, not by tag. Whoever controls an action can move a tag to malicious code, but cannot change a SHA. Each job's machine is also pinned to an Ubuntu version (`runs-on: ubuntu-26.04`), never `ubuntu-latest`, which changes version by itself.

Dependabot (`.github/dependabot.yml`) opens every week the PRs that keep these pins up to date: the actions (the SHA and the comment with the version), the Go modules, the npm packages of `web/` and `e2e/`, the base images of the Dockerfiles and the CockroachDB image of `compose.yaml`. Each new version waits 7 days before it becomes a PR (`cooldown`), so a compromised release can be pulled before it reaches us; a security update skips the wait. The CockroachDB PR (patches only) only changes `compose.yaml`: change the `go-db` job's `COCKROACH_IMAGE` and the `image` of `deploy/local/compose.ci.yaml` in the same PR. Those two pull the same digest from Cockroach Labs' own registry on Google Cloud (`us-docker.pkg.dev/cockroach-cloud-images/cockroachdb/cockroach`), and the e2e image build pulls every Docker Hub image through Google's Docker Hub mirror (`mirror.gcr.io`, set on BuildKit in `e2e.yml`): GitHub's runners share Docker Hub's anonymous pull limit and failed jobs on it. A digest names the bytes, so the registry does not change the image. Minor versions and fixes arrive together, in one PR per group. These do not come through Dependabot, and are done by hand:

- a major of Angular, TypeScript or vitest in `web/`, which comes with `ng update` and its migrations (the Angular builder accepts one major at a time);
- a major of `@types/node`, which follows the Node version the code runs on (22, in CI and in `backend/Dockerfile`);
- a new Node or Go version in the images, which changes together with `.node-version` and `go.mod`;
- the Ubuntu version of CI's machines (`runs-on`), which changes in every job at once, tested in a PR first;
- a new CockroachDB release line: only the "Regular" lines (26.2, 26.4...), never the "Innovation" ones (26.3), which have no LTS ([CockroachDB's support policy](https://docs.cockroachlabs.com/docs/releases/release-support-policy)); it changes `compose.yaml`, `compose.ci.yaml`, `COCKROACH_IMAGE` in CI, the `Makefile` and the Homebrew formula (`cockroach@26.2`) together.

## Test types

| Type | Tool | Covers |
| --- | --- | --- |
| Unit | `go test`, with case tables | The `rules` module maths and each business rule in isolation. |
| Integration | `go test` + CockroachDB (in Docker or, on a Mac, native) | The sqlc queries, migrations, the retry on error `40001` and who may do what. |
| Leak (RN-10) | `go test` + CockroachDB, `backend/internal/leaktest` | What the master hid never reaches a player: every read, every stream event, every image and fog tile, against a scenario with one hidden thing of each kind. A new RPC must be classified, or the test fails. |
| End to end | Playwright + a local OIDC provider | The acceptance criteria, through the screen, as the user would. |
| Accessibility | axe (`@axe-core/playwright`) in Playwright | Each main screen, in the light and dark themes: no serious or critical WCAG 2.1 A and AA violation. |

**Leak needles.** What a test looks for in an answer, to prove something hidden did not arrive, is a unique marker that is neither hexadecimal nor a word (`LEAKCANARY-<kind>-<n>`), an exact ID or a secret number in a numeric field; never a short word or a piece of text (the letters `dc` appear in one UUID in 65: that is what broke a test by chance). Every test that looks for something absent needs a positive control, which shows the same needle in the answer of someone who may read it (the master, or the owner). The per-feature `TestRN10_*` tests follow the same rule, and `internal/leaktest` applies it to every read at once.

**Acceptance tests.** Each acceptance criterion of a story (`docs/product/stories.md`) becomes an automatic test of this kind: `go test` for the rule that runs on the server, Playwright for what appears on the screen. A story is only done when its tests pass. There is no characterization test of the old app: it is deprecated, and the new system only has to prove its own criteria.

## How to keep the docs current

Documentation changes in the same PR as the code. A PR that changes behaviour without changing the matching document goes back in review. English is the canonical language of every doc.

| Change in the PR | Update |
| --- | --- |
| New or changed rule | `docs/product/rules.md` (a new RN) and the affected story, plus their PT-BR versions under `docs/pt-BR/produto/` |
| New story or changed priority | `docs/product/stories.md` and `docs/pt-BR/produto/historias.md` |
| New screen or flow | The acceptance criteria and their Playwright test; the approved design and the screenshots in the PR (see [Screens](#screens-design-and-review)) |
| Colour, font, spacing or visual component | `docs/design.md` and the visual system in Claude Design |
| Table or column | Migration and `docs/data.md`, with the diagram |
| API service or message | Comments in the `.proto`; the reference is generated by itself |
| New module, external service or infrastructure | `docs/architecture.md` and, if hard to undo, an ADR |
| Deploy, secret, alert or cost | `docs/operations.md` |
| New command or tool | `README.md` (and `README.pt-BR.md`) or `CONTRIBUTING.md` |
| New term | `docs/product/glossary.md` and `docs/pt-BR/produto/glossario.md` |
| New personal data, log, cookie, image or vendor | `docs/privacy.md` and `docs/pt-BR/privacidade.md` (inventory and processors) and the privacy checklist in the PR |
| New dependency, font, icon or third-party content | A licence compatible with Apache 2.0; the `NOTICE`, the licence in `third_party/licenses/` and the app's "Créditos" page, when the licence asks for attribution (see [Licence](#licence)) |
| Project milestone reached | `docs/roadmap.md` |

Which docs have a Portuguese version:

| Docs | Languages |
| --- | --- |
| The readers' docs: `README.md`, `docs/product/*.md`, `docs/privacy.md` (and the user guides, when they exist) | English (canonical) and PT-BR: `README.pt-BR.md` at the root and `docs/pt-BR/` (`docs/pt-BR/produto/*.md`, `docs/pt-BR/privacidade.md`, `docs/pt-BR/README.md`). The PT-BR version is updated in the same PR, with the same structure and the same ids (RN-xx, MR-xx). |
| The engineering docs: `docs/architecture.md`, `docs/data.md`, `docs/design.md`, `docs/operations.md`, `docs/roadmap.md` and this file | English only. |

The app's UI text stays in Portuguese until i18n lands (after the MVP), so strings quoted from the screen (such as the button "Sair dos outros dispositivos") stay in Portuguese inside the English docs.

How to write:

- The first sentence of each section is the answer. The context comes after.
- Short sentences, in English, with software terms as usual.
- Flow, states or relations become Mermaid diagrams (render-check any Mermaid you touch). Compared items become tables.
- One subject per file. The others point to it by link, without copying. When something moves, fix the links.
- Every number has a unit and a source. What nobody knows yet is asked of the product owner (Samuel), never guessed; until it is answered, it stays in the story's "Doubts".
- A doc says what the system does and why. *When* and *who decided* does not go into the docs: the decision history goes to `docs/archive/decisions.md`, and the per-Etapa delivery history to `docs/archive/etapas.md`.

Template for a new story (`docs/product/stories.md`):

```markdown
## MR-0xx: Short title

**As a** role, **I want** what the story delivers,
**so that** the reason.

- Priority: MVP | MVP (prerequisite) | Later | Existed in the old app
- Rules: RN-xx or —
- Modules: names of the modules involved

### Acceptance criteria
- **Given** ... **when** ... **then** ...

### Doubts
- ...
```

The template for an ADR (for the private ADR repository) and more context on why each document exists: see [docs/README.md](docs/README.md).

## Licence

MeuRPG is distributed under the [Apache License 2.0](LICENSE). Every PR enters under the same licence (section 5 of Apache 2.0), so only submit code you wrote or that has a compatible licence.

Code under an incompatible licence (for example CC BY-NC) is never copied: if an idea is worth it, one person (or agent) writes a specification of the behaviour, in our own words, and another implements from it alone (clean room).

Third-party content keeps its own licence, and the [NOTICE](NOTICE) lists each one: SRD 5.1 (CC BY 4.0), the 5e-database data (MIT), the fonts (OFL 1.1) and the icons (Apache 2.0). A new dependency or content that asks for attribution enters the `NOTICE`, with the whole licence in `third_party/licenses/` and a line on the app's "Créditos" page.

## User guides

The user guides (master's guide, player's guide and FAQ) start once the MVP screens stabilise, on a documentation site separate from the code.

## See also

- [docs/README.md](docs/README.md): index of all the documentation.
- [README.md](README.md): project overview.
