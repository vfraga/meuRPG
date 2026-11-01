# Local development

The local environment is `make up`: CockroachDB, the devidp (a development OpenID Connect provider) and the backend, which also serves the Angular app, on `http://localhost:8080`.
This page covers every way to run it and the variables that matter locally.
The everyday commands are in [CONTRIBUTING.md](../CONTRIBUTING.md); how to test is in [Testing](testing.md).

## The default stack: Docker

`make up` builds and starts `deploy/local/compose.yaml` in the background; `make down` stops it and `make logs` follows its logs.
Every published port binds `127.0.0.1`: the local CockroachDB has no password.

| Service | Port | Notes |
| --- | --- | --- |
| `api` | 8080 | Server, API and the Angular build on one origin. `LOG_LEVEL` defaults to `debug` (every request, RPC, stream and game event, with a `request_id`); `LOG_LEVEL=info make up` lowers it. See [Architecture](architecture/logs.md#logs). |
| `idp` (devidp) | 9090 | Sign-in, see [below](#sign-in-with-the-devidp). |
| `cockroach` | 26257 (SQL), 8081 (console) | One insecure node. |

`make run` runs the backend straight in the terminal against the `make up` database.
`make migrate` applies the goose migrations to it (`make migrate MIGRATE_ARGS=status` passes another sub-command).
`WEB_DIR=../web/dist/web/browser PORT=8090 go run -C backend ./cmd/api` runs only the API as production does, serving a built Angular app, with the real cache headers and CSP and no database: sign-in is off, but the public screen and `SystemService.GetServerInfo` work.
`cd web && npm start` runs Angular alone in dev mode; `web/proxy.conf.json` forwards `/meurpg.*`, `/auth`, `/images`, `/uploads`, `/healthz` and `/readyz` to `localhost:8080`.

## Native CockroachDB (Mac, optional)

`LOCAL_DB=native` keeps the database out of Docker: on a Mac, every container runs inside a Linux VM and CockroachDB is by far the heaviest part.

1. Install the version `compose.yaml` and CI use (`COCKROACH_VERSION` in the `Makefile`) from Cockroach Labs' tap, and do not run it with `brew services`:

   ```bash
   brew tap cockroachdb/tap
   brew trust --formula cockroachdb/tap/cockroach@26.2
   brew trust --formula cockroachdb/tap/cockroach
   brew install cockroachdb/tap/cockroach@26.2
   brew link cockroachdb/tap/cockroach@26.2
   ```

2. `make db-native-start` starts it on `localhost:26257` (console `http://localhost:8081`), with the data in `~/.meurpg/cockroach` (shared by every worktree), and creates the `meurpg` database. `make db-native-stop` stops it.
3. `make up LOCAL_DB=native` (also `down`, `logs`, `e2e`) uses it: `deploy/local/compose.native-db.yaml` drops the CockroachDB container and points the API and the migrations at `host.docker.internal:26257`. `export LOCAL_DB=native` pins it in the shell.

The container database and the native one use the same port: stop one before starting the other.

## Everything native (Mac, optional)

`LOCAL_STACK=native` runs the devidp and the API as processes, with no Docker, against the native CockroachDB.

```bash
make db-native-start
make up LOCAL_STACK=native     # builds, migrates, starts the devidp and the API
make logs LOCAL_STACK=native
make e2e LOCAL_STACK=native
make down LOCAL_STACK=native
```

- `deploy/local/native.sh` builds the API, `migrate` and the devidp, rebuilds the Angular app only when something in `web/` changed, applies the migrations and starts both processes with the same variables as `compose.yaml`. PIDs and logs are in `~/.meurpg/run/`; the gallery images are in `~/.meurpg/images`.
- Both processes listen on `127.0.0.1` only (`LISTEN_HOST`; in production the variable is empty and the server listens on every interface, as Cloud Run requires).
- The issuer is still `http://idp.localhost:9090`: macOS resolves any `*.localhost` name to the machine (RFC 6761), and Go uses that resolver on a Mac. Go's own resolver (`GODEBUG=netdns=go`) and Linux do not; use Docker there.
- Ports 8080 and 9090 must be free.
- A second environment next to the first: `API_PORT=8180 IDP_PORT=9190 DB_NAME=meurpg_2 deploy/local/native.sh up` uses other ports, another database, `~/.meurpg/run-8180` and `~/.meurpg/images-8180`. Point Playwright at it with `E2E_BASE_URL=http://localhost:8180 E2E_IDP_ORIGIN=http://idp.localhost:9190`. Stop it with `API_PORT=8180 deploy/local/native.sh down`.

## Test databases

The integration tests need a database of their own: see [Testing](testing.md#integration-tests).
`make db-test-start` starts a PostgreSQL on port 5433, with its data under `~/.meurpg/postgres-test-<port>`, SERIALIZABLE by default and `fsync` off, as a preflight runs it.
`TEST_PG_PORT=5434` starts a second one beside it, and `make db-test-stop` (same `TEST_PG_PORT`) stops it.
It needs `pg_ctl` and `initdb` on the `PATH` (`brew install postgresql@17`).
`make db-test-cockroach-start` starts a CockroachDB on port 26258 with the data in memory (2 GiB at most, gone on shutdown), the database of the tests that need it and of a train; `TEST_DB_PORT=26259` starts a second one beside it, with the console on the port after the first one's (8082, 8083...).
`make db-test-cockroach-stop` (same `TEST_DB_PORT`) stops it.
It uses the same CockroachDB version as `db-native-start`.
`make up` and the e2e tests always use CockroachDB.

## Sign-in with the devidp

`make up` starts sign-in ready to use: the `idp` service runs the devidp (`backend/cmd/devidp`), a minimal OIDC provider, in place of Google.

1. `make up`.
2. Open `http://localhost:8080/auth/login?return_to=/` in Chrome. Safari does not accept a `Secure` cookie on `http://localhost`.
3. The browser goes to `http://idp.localhost:9090`, which lists the test users. One click signs you in.

| Test user | `sub` | E-mail | Used for |
| --- | --- | --- | --- |
| Mestre Teste | `devidp-mestre` | `mestre@example.com` | The tests' master |
| Jogador Teste | `devidp-jogador` | `jogador@example.com` | A player |
| E-mail Não Verificado | `devidp-nao-verificado` | `nao-verificado@example.com`, **not** verified | The check that an unverified e-mail is not stored; the second player of tests that need two |
| Sessões Teste | `devidp-sessoes` | `sessoes@example.com` | Only `e2e/tests/sessions.spec.ts` ("Sair dos outros dispositivos" ends every other session of the account, so no shared account will do) |

The users are defined in `backend/internal/identity/oidctest/provider.go`; each `sub` is fixed, so a user always lands on the same account.
There is no password: anyone who reaches the devidp signs in as any test user, which is why it never goes to production.
Its image (`deploy/local/devidp.Dockerfile`) is separate from the production image, it refuses an issuer that is not on a loopback host, it refuses to start on Cloud Run (`K_SERVICE`), and CI checks that the production image does not contain it.
It honours `max_age` and `prompt=login|none` with its own `devidp_session` cookie.
Configuration is by flags or environment variables (`DEVIDP_ISSUER`, `DEVIDP_LISTEN`, `DEVIDP_CLIENT_ID`, `DEVIDP_CLIENT_SECRET`, `DEVIDP_REDIRECT_URIS`; the list and defaults are in the comment of `backend/cmd/devidp/main.go`).

The issuer must be the same string for the browser and for the API container, because the backend checks the ID token's `iss`: the browser resolves `*.localhost` by itself, and the API container resolves the same name through a Docker network alias (see [Testing](testing.md#the-devidp-and-the-test-identity-provider)).

## Sign-in with another OIDC provider

Master sign-in works with any OpenID Connect provider.
Without `DATABASE_URL` the backend exits at startup with an error that names the variable.
Without `OIDC_ISSUER` it still starts, warns that sign-in is off, and `/auth/login` answers 503.

| Variable | Required | What it is |
| --- | --- | --- |
| `OIDC_ISSUER` | To turn sign-in on | The provider's issuer, equal to `issuer` in its discovery document. `https`; `http` only on `localhost`, `*.localhost` or a loopback IP. |
| `OIDC_CLIENT_ID` | Yes | MeuRPG's client ID at the provider. |
| `OIDC_CLIENT_SECRET` | Yes | The client secret. Never goes to the repository, an issue or a log (the backend prints `[REDACTED]`). |
| `OIDC_REDIRECT_URL` | Yes | The backend's public URL plus `/auth/callback`; locally `http://localhost:8080/auth/callback`, registered at the provider exactly the same. |
| `OIDC_CA_FILE` | No | A PEM file with the certificate of a local provider with a self-signed certificate. |
| `OIDC_MAX_AGE` | No | A duration such as `1h`, sent as `max_age`. Only for a provider that documents it (the devidp does, `compose.yaml` sets `1h`); not with Google. |
| `SESSION_IDLE_TIMEOUT` | No | How long a session may go unused before it ends: `1h` to `720h`. The 30-day absolute limit does not change. See [Operations](operations/reference.md#environment-variables). |

To try a real provider: create a confidential client (Authorization Code, PKCE S256, scopes `openid email`, the redirect URL above); start only the database with `docker compose -f deploy/local/compose.yaml up -d cockroach migrate`; run `OIDC_ISSUER=... OIDC_CLIENT_ID=... OIDC_CLIENT_SECRET=... OIDC_REDIRECT_URL=http://localhost:8080/auth/callback make run`; open `http://localhost:8080/auth/login?return_to=/` in Chrome or Firefox.
To see who is signed in: `http://localhost:8080/meurpg.identity.v1.IdentityService/GetMe?connect=v1&encoding=json&message=%7B%7D`.
A Connect call from `curl` needs `-H 'Connect-Protocol-Version: 1'` (CSRF protection, see [Architecture](architecture/identity.md#csrf)).

## Gallery images

The images the master uploads (MR-019) live in the folder `BLOB_DIR` locally and in a Cloud Storage bucket (`BLOB_BUCKET`) on Cloud Run.
`make up` turns the folder on: `compose.yaml` mounts the `images` volume at `/var/lib/meurpg/images`, and it survives `make down` (`docker volume rm meurpg-local_images` deletes the images).

| Variable | What it does |
| --- | --- |
| `BLOB_DIR` | The folder for images and thumbnails. Without it, images are off: `POST /uploads/images`, `GET /images/...` and `GalleryService` answer `503`/`unavailable`, and the rest of the app works. A folder without write permission keeps the API from starting. Never together with `BLOB_BUCKET`; refused on Cloud Run. |
| `BLOB_BUCKET` | The Cloud Storage bucket, for production. See [Operations](operations/reference.md#images-and-the-bucket). |

With `make run`, `BLOB_DIR=/tmp/meurpg-images make run` turns images on.
To upload with `curl`, sign in with Chrome as "Mestre Teste", create a campaign, copy the `__Host-meurpg_session` cookie value (a secret: paste it nowhere) and the campaign ID from the URL, then send `campaign_id` before `file`:

```bash
SESSION='cookie value'
curl -s -X POST http://localhost:8080/uploads/images \
  -b "__Host-meurpg_session=$SESSION" \
  -F campaign_id=<campaign id> -F file=@mapa.png
```

The answer is `201` with the image in JSON; an error is `{"code", "reason", "message"}` (the reasons are in the comment of `GalleryService` in `proto/meurpg/maps/v1/gallery.proto`).
Download with `curl -s -b "__Host-meurpg_session=$SESSION" http://localhost:8080/images/<id> -o image` (`/images/<id>/thumb` for the thumbnail).
The upload is not Connect, so it needs no `Connect-Protocol-Version`.
CSRF protection (`http.CrossOriginProtection`) judges only `Sec-Fetch-Site` and `Origin`, which `curl` does not send; with `-H 'Sec-Fetch-Site: cross-site'` or a foreign `Origin` the answer is `403`.

## AI-generated images

Image generation (MR-039, RN-28) calls the Gemini API; without a key it is off and the rest of the app works.

| Variable | What it does |
| --- | --- |
| `GEMINI_API_KEY` | The Google AI Studio key. A secret: never in the repository, a test, a log or an error. Without it the calls answer `failed_precondition` with the reason `OFF`. |
| `GEMINI_IMAGE_MODEL` | The model; the default is in `backend/internal/maps/images/gen`. |
| `IMAGE_GENERATOR` | `fake` uses the deterministic fake generator (`backend/internal/maps/images/gen`): no key, no network, a PNG of the requested aspect ratio. `make up` (both stacks) uses it; the server refuses it on Cloud Run. `[recusa]`, `[vazio]`, `[erro]` or `[lento]` in the request text makes the fake refuse, return no image, fail or take long. |
| `TTS_GENERATOR` | `fake` makes the voice of the conversation a short silent audio (`backend/internal/play/voice`): no key, no network. `make up` uses it; refused on Cloud Run. Real voices need `GEMINI_API_KEY` and `GEMINI_TTS_MODEL`. |
| `IMAGE_MONTHLY_LIMIT` | Images per campaign per month (default in `config.go`). |
| `IMAGE_DAILY_LIMIT` | Images for the whole server per Brasília day: the ceiling of the Gemini account. `make up` raises it so repeated e2e runs are not capped. |

The tests always use the fake generator. `TestRealGemini` (in `backend/internal/maps/images/gen`) uses the real model and only runs with `MEURPG_TEST_GEMINI_API_KEY` (and optionally `MEURPG_TEST_GEMINI_MODEL`) set on your machine; CI never sets them.
The limits are explained in [Operations](operations/reference.md#generated-images-the-gemini-api).

## Detached work

Images, exports and sweeps run after the call that asked for them has answered. Locally, in tests and in CI they run in the server process (`tasks.InProcess`): leave `TASKS_QUEUE`, `TASKS_TARGET_URL` and `TASKS_SERVICE_ACCOUNT` unset, and the start-up log says `TASKS_QUEUE is not set; detached work (images, exports, sweeps) runs in this process`. The three are for production only ([Operations](operations/reference.md#cloud-tasks)); setting some but not all stops the server.

## Abuse-limit variables

The backend limits the request rate (per IP and per user, in memory) and what one account creates; the defaults suit one table.

| Variable | What it does |
| --- | --- |
| `MAX_CAMPAIGNS_PER_USER` | Campaigns one account may master (RN-30). The local stacks and CI use `off`, because the e2e suite creates hundreds of campaigns with one master; the API refuses `off` on Cloud Run. |
| `CAMPAIGN_CREATORS` | Verified e-mails, comma-separated, that may create campaigns; empty means anyone. |
| `RATE_LIMIT_MULTIPLIER` | Multiplies every rate limit. `make up` uses 10, because the e2e suite sends several accounts' requests from one address. |

The numbers and how each limit counts are in [Architecture](architecture/api.md#abuse-limits) and [Operations](operations/reference.md#abuse-limits).

## Logs in ELK

`make elk-up` starts Elasticsearch, Kibana and Filebeat (`deploy/elk/compose.yaml`) that keep the API's logs, reading the containers of `make up` and the `~/.meurpg/run*/api.log` files of `LOCAL_STACK=native`.
The first run creates `deploy/elk/.env` with random passwords; Kibana is at `http://localhost:5601`.
From the terminal, `deploy/elk/logs.sh help` lists `errors`, `request <id>`, `tail` and more; `make elk-down` and `make elk-logs` stop it and follow its own logs.
See [Operations](operations/logs.md#locally-with-elk).

## Tools

| Tool | Version | Where it is pinned |
| --- | --- | --- |
| Go | `backend/go.mod` | The Dockerfiles must match it |
| Node | `.node-version` | CI's only source |
| buf | any recent | `make proto`, `make proto-lint` |
| sqlc | 1.31.1 | `Makefile` (`make sqlc` runs it with `go run`) |
| golangci-lint | 2.14.0 | `Makefile`; see [CI](ci.md#go-linters-and-the-ratchet) |
| govulncheck | 1.8.0 | `govulncheck` job of `backend.yml`; see [CI](ci.md#vulnerability-scans) |
| osv-scanner | 2.7.0 | `osv-scan.yml`, with its sha256 |
| CockroachDB | `COCKROACH_VERSION` in the `Makefile` | `compose.yaml`, `compose.ci.yaml`, `COCKROACH_IMAGE` in `backend.yml` |

`make web-install` runs `npm ci --ignore-scripts` in `web/` and never runs third-party install scripts.
To add or update a `web/` dependency use `npm install` with Corepack enabled (`corepack enable`, once): `web/package.json` pins the npm version because the stock npm hangs resolving the peer-dependency graph; `npm ci` works with either.
To add or update an `e2e/` dependency use `cd e2e && npm install <package>@<version>`; `e2e/.npmrc` prevents install scripts and writes the exact version.
