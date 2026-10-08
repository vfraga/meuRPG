# Operations

The goal is a cost close to zero: with no active session, nothing is running or connected.

**Status.** There is no production deployment yet. This page describes the plan for the first deploy (Cloud Run) and the settings the code already enforces. Items marked **planned** do not exist yet. The local environment (`make up`, the optional ELK) does exist; see [CONTRIBUTING](../CONTRIBUTING.md).

## Hosting

The backend runs on Cloud Run in `southamerica-east1` (São Paulo).

| Item | Value |
| --- | --- |
| Region | `southamerica-east1` |
| CPU | 1 vCPU |
| Memory | 512 MiB |
| `min-instances` | 0 (scales to zero when idle) |
| `max-instances` | **1**, while the live-session stream fan-out is in memory (see [Live session stream](#live-session-stream)) |
| Request timeout | At least 35 minutes (`--timeout=2100`, the maximum is 60 minutes): the stream lives up to 30. The Cloud Run default, 5 minutes, would cut the stream short |

CockroachDB runs on Google Cloud, in the same region, on the project owner's current plan: the legacy Unlimited plan, bought before the 2024 licensing change. Changing plan loses Unlimited. Backups stay in São Paulo and are kept for 30 days at most, to meet the deletion deadline (see [Privacy](privacy.md)). This configuration still has to be checked in the console before the first deploy.

## Estimated costs (São Paulo)

These are estimates, not a bill. They are used to decide architecture, such as keeping the stream open only during a session.

| Scenario | Estimated cost | Note |
| --- | --- | --- |
| Base (normal table use) | ~US$ 0.15/month | Cloud Run scales to zero outside sessions. |
| Heavy (more use, more tables) | ~US$ 1.60–6.40/month | Still a few dollars. |
| Accident: stream/WebSocket open all month | ~US$ 55–88/month | This is why the Connect stream only opens during a session (see [Architecture](architecture.md)). |
| Egress (data out of São Paulo) | US$ 0.19/GiB | No free tier. This is why map images go to Cloud Storage and leave through their own URL, not as base64 inside responses. The master's browser keeps each image for a year (`private, immutable`) and downloads it once. The player's browser asks again on every use (`no-cache`, so it stops showing what the master hid), but the answer is a bodiless `304` while the image does not change, so each device still downloads each image once (see [Architecture](architecture.md#serving-images)). |
| Gallery images in Cloud Storage (Standard, São Paulo) | US$ 0.035 per GiB per month ([Cloud Storage pricing](https://cloud.google.com/storage/pricing), checked 30/09/2026) | A campaign with a full gallery (500 MiB, the proposed limit) costs ~US$ 0.02/month. The 7-day soft delete charges the same price for deleted data during those days. The API reads the bucket in the same region, with no egress; each read is a class B operation, billed per thousand (same page). |

## Live session stream

The stream (`PlayService.WatchGameSession`) is open only while someone has the session page open and visible. While a stream is open, the Cloud Run instance is serving a request and is billed, so the server and the app limit how long each one lives (see [Architecture](architecture.md#live-session)).

| Rule | Value | Where |
| --- | --- | --- |
| Server heartbeat | Every 25 seconds | `play.DefaultHeartbeat` |
| Dead stream, on the app side | Nothing arrived in about 60 seconds: the app reconnects | App |
| Re-check of the login session and of the participation | Every 60 seconds, in the database | `play.DefaultRecheck` |
| Maximum life of a stream | 30 minutes; then the server ends it without an error and the app opens another | `play.DefaultMaxLifetime` |
| Deadline of each message sent | 30 seconds to reach the client; after that `Send` fails and the stream ends (`client_gone`). The deadline does not count the silence between two messages | `streamSendTimeout`, in `cmd/api/main.go` (`platform/slowclient`) |
| Streams per person | At most 8 per campaign (tabs and devices); the next one is refused with `resource_exhausted` and the app retries with a backoff | `live.DefaultMaxPerUser` |
| Reconnection | Growing wait: 1 s, 2 s, 4 s, up to 30 s, with jitter; only with the tab visible | App |
| Hidden tab | After 2 minutes hidden, the app closes the stream; when the tab returns, it reconnects and reads the snapshot again | App |
| Session notice | Light query (`ListOpenGameSessions`) every 30 seconds with the tab visible, without a stream | App |
| Server shutdown | Streams end immediately when the graceful shutdown starts (`httpserver.Server.OnShutdown`), and the apps reconnect | `cmd/api` |

**`max-instances = 1` while the fan-out is in memory.** A change made by the master is delivered to the streams by the `play/live` hub, in server memory. With two instances, the master on one and a player on the other, the change would not reach the player. One instance (1 vCPU, 512 MiB) serves a table comfortably. When it stops being enough, the hub gives way to a shared channel (a CockroachDB changefeed or Pub/Sub), and `max-instances` can go up.

## Fog image tiles

For each player, the server builds the tiles of the image of a map with fog (MR-036, RN-10; [Architecture](architecture.md#per-player-image-tiles)). This is CPU and memory work on a 1 vCPU, 512 MiB server, so it has a budget:

| Rule | Value | Where |
| --- | --- | --- |
| Working copy of a map | Up to 2,048 px on the longer side (8.4 MB of RGBA for 4,096 × 2,048); decoded once, shrunk square by square | `maps/tiles.go`, `workingMaxSide` |
| Maps with a copy in memory | 2, for the whole server; the least recently used leaves first | `workingCopies` |
| Simultaneous renders | 1; other requests wait up to 8 s | `renderWait` |
| Cache misses per user | 60 at once and 10 per second; beyond that, `429` with `Retry-After` | `newTileRenderer` |
| Cache of finished tiles | 32 MiB of PNG, counted in bytes, least recently used leaves first | `tileCacheBytes` |
| Image refused | The decoding would exceed 192 MiB (a 16-bit PNG of more than 24 megapixels, from before uploads kept 8 bits; a 16-bit gray PNG with a `tRNS` chunk is priced as the 8-byte pixel it decodes to) | `decodeLimit` |
| Worst case of memory | About 280 MB plus about 50 MB for the rest, against the `GOMEMLIMIT` of 400 MiB (the sum is in [CONTRIBUTING](../CONTRIBUTING.md)) | CONTRIBUTING |

- **The `503` with `Retry-After: 2` and `reason: BUSY`** means that more than one render waited over 8 s. It happens when many players open, at the same time, a map nobody has opened since the server started (each one needs the first tile), or right after a grid or image change. The app retries by itself; tiles already in the cache wait for nothing. A `429` with `reason: RATE_LIMITED` is one user asking for too many uncached tiles; the app retries too.
- **The capacity limit: two fog maps in play at the same time.** The working copies belong to the whole server. With three or more fog maps being played together (several campaigns on the same instance), the copies take turns, and each turn decodes the image again (150 ms to 600 ms, holding the render turn and the upload slot): first tiles take longer and the `503` becomes more likely. The remedy is to raise `workingCopies` (each copy costs up to 17 MB) or to have more instances.
- **None of this is stored on disk or in the database.** Restarting the server empties the working copy, the tiles and each player's tiles, and they rebuild themselves. The memory of what each player has seen (which decides the tiles) is `map_vision_memory`.
- **The image of a fog map stays in the blob store, once.** Tiles do not use gallery quota.

## Table content cache

The content the master registers (MR-025, RN-23; [Architecture](architecture.md#live-table-content)) is assembled in server memory, per campaign and revision, and kept in a small cache. This is memory and CPU on a 1 vCPU, 512 MiB server, so it has a budget:

| Rule | Value | Where |
| --- | --- | --- |
| Contents in the cache | 8, for the whole server, per (campaign, revision); least recently used leaves first | `characters/tablesource.go`, `maxLiveContents` |
| `ListContent` catalogs | Up to 8 contents, each with the master's catalog and the players' catalog | `characters/contentsource.go`, `maxCatalogs` |
| Entries per campaign | 300; 64 KiB of data per entry | `rules.MaxOverlayEntries`, `characters.MaxTableEntryBytes` |
| One assembled content | About **1.4 MB** retained beyond the SRD (the SRD is one, shared), for a table of 300 entries (10 classes, 30 subclasses, 20 races, 40 subraces, 40 backgrounds and 160 spells, with 135 KB of data) | `TestLiveContentMemory` |
| The catalogs of a content | About **0.14 MB** (the master's and the players'; 75 KB on the wire) | `TestLiveContentMemory` |
| The 8 together | **About 12 MB** (11 MB of contents and 1 MB of catalogs), measured, in the case where both caches keep the same 8 contents | `TestLiveContentMemory` |
| **The worst case** | **About 23 MB** (computed, not measured: 16 contents × 1.37 MB = 22 MB, plus 1.1 MB of catalogs). The catalog cache (`maxCatalogs`) uses the content as key and keeps it alive, and has its own eviction order. When the campaigns in play rotate faster than 8 revisions, the two caches keep different contents, up to 8 + 8. It fits in the 24 MB reserved for it and in the 400 MiB `GOMEMLIMIT` (add the 280 MB of the fog-tiles sum, in CONTRIBUTING) | derived |
| Assembling after a write | **6 to 9 ms** per campaign, once per revision (read the data, build the overlay, `With`); the worst case the engine budgets let through takes about 31 ms | `TestLiveContentMemory`, `BenchmarkWith` |

- **None of this is a real database cache.** Restarting the server empties it and it rebuilds on the first read of each campaign. The campaign revision (`campaign_content_state`) is read on **every** content read, inside the reader's transaction: it is a primary-key lookup, and it is what makes a master's edit count immediately.
- **With more than 8 campaigns with table content in play at the same time** the cache thrashes: each read of a campaign outside it assembles again (6 to 9 ms). With one instance and one table at a time this does not happen; when it does, the number to change is `maxLiveContents`, and each extra content costs about 1.5 MB.
- **No personal data:** table content is names and texts the master writes for the game (see [Privacy](privacy.md)).
- **To measure again:** `MEURPG_MEASURE=1 go test ./internal/characters -run TestLiveContentMemory -v` (in `backend/`, no database).

## Environment variables and secrets

Credentials (the Google OAuth client secret, the database connection string and others) live in Google Cloud Secret Manager, never in a loose environment variable in the repository or in the deploy. The exact list of secrets per environment is **planned** (not defined yet).

### Sign-in settings

The master's sign-in (`identity` module) needs one secret, the OIDC provider's client secret, and some values that are not secrets:

| Variable | Secret? | In production |
| --- | --- | --- |
| `OIDC_CLIENT_SECRET` | Yes | Secret Manager, delivered to Cloud Run as an environment variable |
| `DATABASE_URL` | Yes (has the database password) | Secret Manager. Accepts `pool_max_conns=N`, `connect_timeout`, `statement_timeout` and `idle_in_transaction_session_timeout`; without `pool_max_conns`, the pool opens at most 10 connections (see [The connection pool](#the-connection-pool)). **On Cloud Run it needs `sslmode=verify-full`** (or `verify-ca`): without that, or with `disable`, `allow`, `prefer` (pgx's default, which falls back to plain text) or `require` (encrypts without checking who is on the other end), the server does not start, and the error message never repeats the URL |
| `OIDC_ISSUER` | No | `https://accounts.google.com` |
| `OIDC_CLIENT_ID` | No | Service environment variable |
| `OIDC_REDIRECT_URL` | No | `https://<domain>/auth/callback`, registered identically on Google's OAuth client. Changing domain requires registering the new URL before the deploy |
| `SESSION_IDLE_TIMEOUT` | No | A duration, from `1h` to `720h`; default `336h` (14 days). A sign-in session with no use for that long stops being valid (ASVS 5.0 V7.3.1), as if it did not exist. The 14 days exist because the table plays roughly every week: they forgive two missed sessions in a row and still cut a forgotten browser well before the absolute 30 days, which nothing changes. Lowering the value signs out people who go a while without playing; raising it helps little, because the absolute 30 days remain |
| `OIDC_MAX_AGE` | No | Do not set it with Google, which does not document `max_age`. Re-authentication every 30 days (NIST SP 800-63B-4) comes from the 30-day server-side session. In the local environment, with the devidp, it is `1h` |
| `GOOGLE_CLOUD_PROJECT` | No | The project id, set at deploy (`--set-env-vars`): **Cloud Run does not set it by itself**. Without it, log lines do not carry the Cloud Logging trace, and the lines of one request do not show up together (see [Architecture](architecture.md#logs)) |
| `LISTEN_HOST` | No | Do not set it: empty, the server listens on all interfaces, which is what Cloud Run requires. Local environments use `127.0.0.1` |

The backend never writes the client secret to the log: the `config.Secret` type prints as `[REDACTED]`. `DATABASE_URL` and `GEMINI_API_KEY` are `config.Secret` too. `migrate` reads the URL with pgx and, if it is invalid, returns a fixed sentence instead of the driver error, which could repeat the password.

### The connection pool

The backend talks to CockroachDB through a pgx pool. Without `pool_max_conns` in `DATABASE_URL`, the pool opens at most **10** connections (pgx's default, `max(4, vCPUs)`, would give 4 on the 1 vCPU Cloud Run). To change the size, put `?pool_max_conns=N` in the Secret Manager connection string. A new secret version only takes effect in a new Cloud Run revision or when an instance starts again, so deploy a revision (or let the instance restart) after creating the version.

- **Why 10.** There is one instance (`max-instances` 1), and a table is one master and up to six players; each request is a short transaction. 10 covers the streams' periodic reads and a burst of actions, and stays far below what CockroachDB recommends (about four connections per cluster vCPU, summed across instances). If `max-instances` ever goes up, the total (instances × `pool_max_conns`) is what counts.
- **Waiting for a connection is normal; waiting forever is not.** With the pool full, a request waits its turn. A request **never** asks for a second connection while its transaction holds the first (see [Architecture](architecture.md#transactions-and-the-connection-pool)): that mistake once stalled 5 requests for 176 s in CI. If `context canceled` or `context deadline exceeded` show up in simple reads ("look up session", "get membership") together with a slow request, that is the symptom: look for a read through the pool inside a `db.InTx`.
- **Memory and cost.** Each connection uses memory on the CockroachDB node; that is why the pool is small and does not grow by itself.
- **Two maximum times per connection.** Every pool connection opens with `statement_timeout` of **30 s** (the database cancels a command that exceeds it, SQLSTATE `57014`, and the API answers `deadline_exceeded`) and `idle_in_transaction_session_timeout` of **60 s** (the database closes an open, idle transaction). Both come disabled in CockroachDB, and the maximum request time on Cloud Run is 35 minutes: without them, a stuck command or an abandoned transaction would hold one of the 10 connections, and its locks, for all that time. Each value only applies when `DATABASE_URL` does not define its own (`&statement_timeout=60000`, in milliseconds). No legitimate API command comes close to 30 s: they are primary-key reads and writes or small scans, and slow work (drawing a dungeon, decoding an image, calling the model) is Go code between commands, never inside a transaction. What CockroachDB itself does (row TTL, schema jobs) does not go through the API connection. `migrate` uses its own connection and has **no** `statement_timeout`: a data backfill can take long. Two `migrate up` at the same time block each other: goose has no lock for CockroachDB, so `migrate` takes a lease lock before `up` and `down` (the `migration_lock` table, see [Data](data.md#migration-lock)). The second waits up to 10 minutes for the first and exits with a clear error if it does not finish; a run that crashed blocks nobody, because the lock's expiry (60 s, renewed every 20 s) passes by itself. If the lock is lost midway (another run took it), the migration in progress stops between two commands, and running `migrate up` again finishes the work. Still, the deploy must not trigger two.

### The devidp is never deployed

The devidp (`backend/cmd/devidp`) is the development OIDC provider of `make up` and of CI. It signs anyone in as any test user, with no password, so it cannot exist in any real environment:

- The image that goes to Cloud Run is the one from `backend/Dockerfile`, which has only `api` and `migrate`. The devidp has its own image (`deploy/local/devidp.Dockerfile`), which nobody publishes. The `e2e` workflow checks on every PR that the production image does not have the binary.
- If someone tries, it does not start: it refuses an issuer outside loopback (`localhost`, `*.localhost`, `127.0.0.1`, `::1`) and refuses to run on Cloud Run (`K_SERVICE` set).

In production, sign-in is Google only (`OIDC_ISSUER=https://accounts.google.com`).

### Sign-in rate limit and the client IP

`/auth/login` and `/auth/callback` share a per-IP limit and a global one, in memory (see [Architecture](architecture.md#login-attempt-limit)). On Cloud Run, the client IP is the last item of `X-Forwarded-For`, which Google's front end appends. This holds for Cloud Run **without a load balancer in front**, which is the current plan.

- **On the first deploy, check** that the last item of `X-Forwarded-For` really is the caller's IP: Google documents the format for load balancers, but does not say how many items the front end appends without a load balancer. If the last item belongs to a Google machine, all clients share a single limit (only the global one remains); nobody can bypass the limit, but `cloudRunTrustedHops` in `ratelimit.ClientKey` has to change.
- **If a load balancer is ever put in front**, it appends `<client-ip>,<load-balancer-ip>`, and the client IP becomes the second-to-last item: `cloudRunTrustedHops` becomes 2.
- With several instances, each has its own counters.

### Abuse limits

The app has rate limits and caps on what one account creates (see [Architecture](architecture.md#abuse-limits) and rule RN-30). The defaults serve one table; the variables below exist to raise a cap without changing code.

| Variable | Secret? | What it is |
| --- | --- | --- |
| `MAX_CAMPAIGNS_PER_USER` | No | Campaigns an account can be master of. Default: **10** (1 to 1,000). `off` removes the cap, only in local environments and CI, where e2e creates hundreds of campaigns with the same test master; on Cloud Run the API refuses to start with `off`. There is no "delete campaign" in the app yet: an account that reached the cap only gets out of it with a higher value here |
| `CAMPAIGN_CREATORS` | No, but personal data | **Verified** e-mails allowed to create campaigns, comma-separated (case-insensitive). Empty (the default): any account creates; a value with only separators (`,`) stops the server from starting instead of reading as empty. Filled: only those; people who join by invite keep playing. Before opening sign-in to strangers, fill it with the master's e-mail. A value that is not an e-mail stops the server from starting (the error gives the position, never the text) |
| `IMAGE_DAILY_LIMIT` | No | Images the **whole server** generates per Brasília day, all campaigns together. Default: **100** (1 to 10,000), the same value as the proposed daily quota at Google (see [Generated images](#generated-images-the-gemini-api)). Beyond it, the app refuses with reason `DAILY_LIMIT_REACHED`, without calling Gemini |
| `RATE_LIMIT_MULTIPLIER` | No | Multiplies every rate limit (default **1**; above 0 and up to 1,000). Raise it if a bigger table or a shared Wi-Fi bumps into them: the symptom is `429` or "Muitas ações em pouco tempo" in the app, and a `rate limit hit` line in the log (level `WARN`, with the limit name, no IP). The local environment and the e2e suite's CI use **10** (`compose.yaml` and `native.sh`): the suite sends requests of several accounts from the same address |

- **How to raise a limit on Cloud Run:** `gcloud run services update <service> --update-env-vars MAX_CAMPAIGNS_PER_USER=20` (creates a new revision; the limit variables are not secrets).
- **One instance only.** Rate counters live in process memory. The plan is `max-instances` 1, and with it the limits are exact. With a second instance each would have its own (the effect is up to double), and exact limits would need shared storage (Redis or the database). Changing `max-instances` requires rereading this. The same goes for the sign-in limit.
- **The client IP** is the last item of `X-Forwarded-For`, as in the [sign-in limit](#sign-in-rate-limit-and-the-client-ip); the first-deploy check covers both. If the last item belongs to a Google machine, all clients share the bucket of a single IP (100 per second in total): nobody bypasses it, but the limit stops telling one person from another, and `RATE_LIMIT_MULTIPLIER` has to go up until the check is done.
- **Cloud Run concurrency.** The app has no global `WriteTimeout`, so the streams are not cut: an image, a thumbnail, a tile or an app file is bounded by its own 2-minute write deadline instead (see [Slow clients](architecture.md#slow-clients)). Set `--concurrency` for the service (the number of requests one instance takes at once) to a value the instance's 512 MiB and 10 database connections carry, and keep the live streams (at most 8 per user) in mind when choosing it: a slow client that holds a request for its 2 minutes holds one of those slots.
- **What the limits do not do.** They are not a defence against a volume attack (DDoS): for that, use Cloud Armor, or Cloud Run concurrency and `max-instances`, which the production plan must define. The app's global limit only stops the database (10 connections) from stalling because of a few sources.

### Images

Gallery images (MR-019) live in a blob store (see [Architecture](architecture.md#where-images-are-stored)).

| Variable | Secret? | Where |
| --- | --- | --- |
| `BLOB_DIR` | No | The image folder on disk. In the local environment, `/var/lib/meurpg/images`, on a Docker Compose volume. Without it, images are off (`503`). In production disk is not used: Cloud Storage comes instead (below) |

**On the first deploy (the Cloud Storage implementation does not exist yet, because there is no deploy):** one bucket only for the images, in `southamerica-east1`, Standard class, with uniform bucket-level access and public-access prevention; a 7-day soft delete (the deadline in [Privacy](privacy.md)); and only the API's service account with access (`roles/storage.objectUser` on the bucket), with no public URL and no signed URL: the API is always the one that delivers the image, after checking who is asking. The implementation goes in the `blob` package, behind the same interface.

**Memory.** An upload reads and processes one image at a time on each instance (it waits for its turn before reading the file, so queued uploads hold no body), and refuses an image whose decoding would exceed 256 MiB (an estimate in the `maps/images` package). With 512 MiB per instance there is room for the rest, as long as Go's garbage collector knows the limit: set `GOMEMLIMIT` (for example `400MiB`) on the first deploy. The worst case of the fog tiles, summed, is about 330 MB (see [Fog image tiles](#fog-image-tiles)); PNG uploads are stored with 8 bits per channel, so that decoding them later costs 4 bytes per pixel. The image of a generated dungeon (MR-010) goes through the same one-image-at-a-time slot and is drawn with a palette (1 byte per pixel): the largest, 199 × 399 squares, is 3,980 × 7,980 px and uses about 32 MB while drawing and about 36 MB allocated in all (see [Architecture](architecture.md#generated-dungeon-maps)); the fog tiles decode the stored image, which is a palette PNG and costs less than a photo.

### Generated images (the Gemini API)

Image generation (MR-039, RN-28) calls the Gemini API with a Google AI Studio key. No image model has a free tier: billing is on, and the paid-services terms apply.

| Variable | Secret? | What it is |
| --- | --- | --- |
| `GEMINI_API_KEY` | **Yes** | The API key. In Secret Manager, delivered to Cloud Run as an environment variable, like `OIDC_CLIENT_SECRET`. Never in the repository, a test, a log or an error message (`config.Secret` prints as `[REDACTED]`). Without it, generation is off: the log says `GEMINI_API_KEY is not set; image generation is off` and calls answer `failed_precondition` with `OFF`; the rest of the app works |
| `GEMINI_IMAGE_MODEL` | No | The model. Default: `gemini-3.1-flash-image`. `gemini-3.1-flash-lite-image` is the cheapest (1K only) |
| `IMAGE_MONTHLY_LIMIT` | No | Images per campaign per month. Default: **20** (1 to 500) |
| `IMAGE_DAILY_LIMIT` | No | Images of the whole server per Brasília day. Default: **100** (1 to 10,000). See [Abuse limits](#abuse-limits) |
| `IMAGE_GENERATOR` | No | `fake` uses the fake generator (local environment and CI). Never on Cloud Run: the server refuses to start |

- **The key is restricted to the Gemini API** (the key's API restriction, in Google Cloud), and the project has a **daily quota at Google as a hard cap**: if a bug of ours generates images nonstop, Google refuses. Proposed: 100 images per day (about US$ 6.70 per day). The app has the same cap on its own (`IMAGE_DAILY_LIMIT`, default 100, the Brasília day): it refuses before calling Gemini and gives the table a clear message, and Google's quota is the second lock. Rotate the key by creating a new one, updating the secret version and deploying a new revision (like `DATABASE_URL`), and deleting the old one.
- **The per-campaign limit** is each table's cost cap: 20 images per month, counted in the database (`image_requests`) by the Brasília month (UTC-3). A refusal, a response without an image, a key the service refused and a "Cancelar" before the request leaves give the slot back; a "Cancelar" after it does not, and neither does a timeout, a connection cut after the request left, an answer that cannot be read (they may have been billed) or a picture the service returned that cannot be stored. **Proposed:** 20, because a 4-hour session uses about 3 to 6 images (a scene's art and one or two adjustments), and a weekly campaign plays 4 to 5 sessions a month; 20 covers most without exceeding **US$ 1.34 per campaign per month** (20 × US$ 0.067). The per-session usage numbers are an estimate, not measured; the limit is changed with `IMAGE_MONTHLY_LIMIT` after measuring with the real key.
- **The cost per image** ([pricing page](https://ai.google.dev/gemini-api/docs/pricing), checked 06/10/2026): US$ 0.067 per 1K image on `gemini-3.1-flash-image` (about 1,120 output tokens at US$ 60 per million), US$ 0.0336 on `gemini-3.1-flash-lite-image` and US$ 0.134 on `gemini-3-pro-image`. The server always asks for 1K. The text and the reference images count as input tokens, much smaller; the measurement with the real key comes with `TestRealGemini`. An adjustment costs the same as a new image. Whether Google bills an answer without an image or a refused one is not clear in the documentation: check on the first bill.
- **Budget alert:** the Google Cloud one (below) also covers the key's project; the threshold is set together with the others.
- **The long wait (Cloud Run).** The service runs with CPU only during a request, scales to zero and has `max-instances` 1: a goroutine that continues after the response leaves would have no CPU, and the instance could stop. So the app keeps a long wait open (`GetImageGeneration` with `wait_seconds` 25) while waiting for the image, and the instance always has a request in progress during a generation; the wait ends the moment the state changes, at the deadline, or at shutdown. With a larger `max-instances`, the wait still ends at the deadline, but only the instance that generates notifies right away (the notifier is in memory). At shutdown, requests still queued give their slot back, a call already sent is cut (the slot stays spent, and the image, if it arrives later, still enters once) and the server waits about 1 s for the goroutines, within Cloud Run's 10 s.
- **Memory** (measured 06/10/2026, Apple M1 Pro, `MEURPG_MEASURE=1 go test -run 'TestShrinkMemory|TestMeasureMemory' -v ./internal/maps/images/...`). A call used to carry up to 14 images of 10 MiB in base64 (about 512 MiB live and 1.36 GiB allocated). Now:

  | Part | Measure |
  | --- | --- |
  | A 40-megapixel image (8,000 × 5,000), **in the upload slot** (`processing`: one at a time on the server), measured | Shrinking the reference: PNG **172 MiB**, JPEG 77 MiB. An upload (`images.Process`), with the reference made: PNG **178 MiB**, JPEG 78 MiB (plus the file, up to about 20 MB). The first fog tile: about 195 MB. The slot's peak is the largest of them, about **200 MB**, and they do not add up. The thumbnail and the reference scale in strips of 32 rows (about 8 MB) instead of using a working area the size of the image. The file is read once, into a buffer of the exact size |
  | The shrunk reference (JPEG, 1024 px, quality 85) | about 25 KB in test images; tens to a few hundred KB in real ones. Stored next to the image (`<id>.ref`, only for images larger than 1024 px): made at upload, with the image already decoded, or the first time an older image is used; deleted with the image. The original is decoded only once for references |
  | The call body, with 14 references of 420 KB (7.7 MiB estimated in base64) | **0.05 MiB** more: written as a stream. The shrunk images stay in the request's memory (up to about 6 MiB) until the call goes out, also while the request waits for the call slot: queued requests held about 4 MiB each (104 MiB for 25), so the server keeps at most 5 requests alive at once (`maxPendingRequests`) |
  | The 6.7 MiB response (the image and a draft) | peak of about 17 MiB, roughly 2.5 times the response; at the 8 MiB cap, about **20 MiB**. A 1K image is about 2 MB in base64 (in practice, about 5 MiB) |

- **Memory of drawing a map** (measured 06/10/2026, Apple M1 Pro, the largest map, 200 × 400 squares; `MEURPG_MEASURE=1 go test -run 'TestMeasureTheBiggestMapReference|TestMeasureTheBiggestMap' -v ./internal/maps ./internal/maps/refimg` and `-run TestCropFitMemory ./internal/maps/images`). The reference drawing is a flat drawing of about 1024 px on the longer side, 1 byte per pixel, not the map image (which is never decoded for this):

  | Part | Measure |
  | --- | --- |
  | The players' view and its drawing: the layers, the floor and walls, the sight of two characters (the union), the seen squares, cropping and drawing, and the PNG | about **39 ms** and **4.5 MB allocated** in all; about 0.7 MB live after the sight and the floor and walls (the compiled scene the fog already keeps in cache, 8 scenes, is not new). The drawing is 400 × 800 px (2 px per square: on big maps each square is small) and the PNG is about 12 kB |
  | The drawing of the textured map (the whole map, completed to the model's aspect ratio: 576 × 1024 px, 9:16) | about 7 ms and 2 MB allocated; PNG of about 4 kB. Smaller maps have squares of up to 24 px (the 24 × 16 cave is 576 × 384) |
  | Cropping and fitting the model's result to the size of the map image (`CropFit`), **in the upload slot** (`processing`: one image at a time on the server) | the **worst case the server accepts**: `CropFit` refuses (`ErrDimensions`, the slot is returned) an answer wider than 4,096 px on a side, or whose decoding plus the output working copy (4 bytes per pixel) exceeds **178 MiB**, so a small PNG on disk that is huge decoded (8,000 × 5,000, 0.5 MiB) does not blow the slot (`TestCropFitRefusesAnAnswerThatBreaksTheMemoryBudget`). The largest that passes, a 4,096 × 4,096 px answer (the 1:1 4K; the server asks for 1K) for a 16-megapixel map image (4,000 × 4,000, the cap): a peak of about **151 MiB** live, 880 MiB allocated in total and about 3.5 s on a loaded machine. It is the decoded answer (4 bytes per pixel) plus the working copy the size of the map (64 MB); a map image over 16 megapixels (4,000 × 4,000 px) is refused before reserving the slot (`MAP_IMAGE_TOO_LARGE`, and `GetMapImageReference` warns beforehand). A 4,000 × 2,000 map and a 1K answer cost about 40 MiB. It does not add to an upload's slot (it is the same one) |

  Within the `GOMEMLIMIT` sum above, the crop enters the **decoded-image slot** (about 200 MB, transient in the worst upload): the 151 MiB crop fits in it and does not add to the others.

  **The sum against the 400 MiB `GOMEMLIMIT`** (the full table is in [CONTRIBUTING](../CONTRIBUTING.md)): the decoded-image slot (about 200 MB, transient) + the working copies (34 MB) + the tile cache (33.5 MB) + the fog scenes (19 MB) + generation (a response of up to 8 MiB, about 20 MiB, and up to 6 MiB of shrunk images: about 26 MB) = about 313 MB, plus about 50 MB for the rest: **about 363 MB**, within the 400 MiB and the 512 MiB of the instance. The number of simultaneous calls is **1** (`maxGenerating`): with 2 it would be about 389 MB. A second generation waits its turn (the request stays `PENDING` and the long wait stays open).

## Logs in ELK

The API logs go to an ELK stack (Elasticsearch, Kibana and Filebeat) so that whoever investigates bugs can read and filter them. The local ELK exists; production is still a plan. The API writes one JSON line per event to stdout, in the backend log contract (`backend/internal/platform/logging`: fields, levels and what never goes in); ELK only reads those lines. The files are in `deploy/elk/`.

| Part | Local (`make elk-up`) | Production |
| --- | --- | --- |
| Who reads the log | Filebeat, from the `meurpg-local` project containers and the `~/.meurpg/run*/api.log` files of the native environment | **Planned.** Cloud Run → Cloud Logging → sink to Pub/Sub → Filebeat with the `gcp-pubsub` input (or Elastic Cloud) |
| Mapping to ECS | Ingest pipeline `meurpg-logs` (`deploy/elk/setup/pipeline.json`) | The same pipeline and template, unchanged |
| Retention | ILM `meurpg-logs`: deletes at 14 days | The same policy |
| State | Ready, **not verified live** (written without Docker; the check is in [Check it in five commands](#check-it-in-five-commands)) | Does not exist |

### Start and use

`make elk-up` starts everything, `make elk-down` stops it (logs stay in the `esdata` volume) and `make elk-logs` shows ELK's own logs. The first time, `deploy/elk/init-env.sh` creates `deploy/elk/.env` with random passwords (`.env.example` shows the names; the `.env` and the CA generated in `deploy/elk/certs/` are in `.gitignore`). Kibana is at `http://localhost:5601` (user `elastic`) and the Elasticsearch API at `https://localhost:9200`; both ports listen only on `127.0.0.1`. It needs about 3.5 GB of free memory for Docker (Elasticsearch 2 GiB with a 1 GiB heap, Kibana 1 GiB, Filebeat 512 MiB).

The services, in order: `certs` (creates the CA and the HTTP and transport TLS certificates, once), `es01`, `setup` (passwords, roles, ILM, pipeline, template and the `logs-meurpg-default` data stream; can run again without changing anything), `kibana`, `kibana-setup` (the `logs-meurpg-*` data view and the saved searches) and `filebeat`. The images are pinned by tag and digest, like the repository's others:

| Image | Version | License |
| --- | --- | --- |
| `docker.elastic.co/elasticsearch/elasticsearch` | 9.5.5, `sha256:13b4a40b…5984` | Elastic License 2.0 (the default distribution; the source code is also released under AGPLv3 and SSPL, but the images are not) |
| `docker.elastic.co/kibana/kibana` | 9.5.5, `sha256:13b72ce7…bb78` | Elastic License 2.0 (like Elasticsearch) |
| `docker.elastic.co/beats/filebeat` | 9.5.5, `sha256:4b2042fc…78aa` | Elastic License 2.0 (the default distribution) |

They are external services: MeuRPG distributes none of them (no change to `NOTICE`); each license text ships inside the image. To update, change the tag and digest on the three `compose.yaml` lines (the digest is the manifest list's, which covers amd64 and arm64: `docker buildx imagetools inspect <image>:<tag>`).

### What the pipeline does with the contract

| Contract | ECS |
| --- | --- |
| `time` | `@timestamp` (`date_nanos`, with the nanoseconds) |
| `severity` | `log.level`, lowercase |
| `message` | `message` |
| `service`, `version` | `service.name`, `service.version` |
| `request_id`, `user_id` | `http.request.id`, `user.id` (`keyword`) |
| `method`, `path`, `status` | `http.request.method`, `url.path`, `http.response.status_code` |
| `duration_ms` | `event.duration`, in nanoseconds |
| `error` | `error.message` |
| `logging.googleapis.com/trace` | `trace.id` (what comes after `/traces/`); the whole value stays in `meurpg.gcp_trace` |
| the rest (`procedure`, `code`, `reason`, `stream`, `event`, `campaign_id`, `session_id`...) | `meurpg.<name>`; ids are `keyword`, `stream` is boolean and `messages` is a number |

A key the contract does not foresee is kept in `meurpg.*` (text becomes `keyword`): nothing rejects the document, and a wrong type is stored without indexing (`ignore_malformed`). If a pipeline step fails, the document enters with the tag `meurpg_pipeline_failure`. Filebeat only adds `labels.stack` (`docker` or `native`) and `container.*`. `logs.sh simulate` runs four sample lines through the pipeline and shows the documents.

### Read the logs

`deploy/elk/logs.sh` (bash, curl and jq) is the interface for whoever does not open Kibana, and prints one line per record: time, level, `req=`, message, procedure or event, `code=`, `status=`, duration and `error=`. It uses the `meurpg_reader` user, which only reads `logs-meurpg-*` (the `READER_PASSWORD` in `.env`).

| Command | What it shows |
| --- | --- |
| `logs.sh errors [since]` | Level `error` or a 5xx response (default: 1 hour) |
| `logs.sh request <id>` | Everything one request logged, oldest to newest; the id is the response's `X-Request-Id` |
| `logs.sh campaign <id> [since]`, `logs.sh user <id> [since]` | One campaign or one user |
| `logs.sh search '<query>' [since]` | A `query_string` (Lucene), such as `meurpg.procedure:*CreateCampaign AND NOT meurpg.code:ok` |
| `logs.sh tail [query]` | The last 10 lines and then only new ones, every 2 s |
| `logs.sh health`, `sample`, `simulate` | Cluster health; sample lines; the pipeline over them |

`since` is a number with a unit (`30s`, `15m`, `2h`, `1d`, `1w`) or `all`. In Kibana, the saved searches are "MeuRPG: erros", "RPCs lentas (> 500 ms)", "uma requisição" and "uma campanha" (the last two ask for the id in place of `COLE-O-ID-AQUI`).

### Check it in five commands

```bash
make elk-up                                   # 1. starts (the first time it downloads the images and takes about 2 minutes)
deploy/elk/logs.sh health                     # 2. cluster=green (or yellow) log_lines=0
deploy/elk/logs.sh sample                     # 3. writes 4 lines to ~/.meurpg/run-sample/api.log; Filebeat ships them in ~5 s
deploy/elk/logs.sh request 0123456789abcdef0123456789abcdef   # 4. shows the 4 lines, in ECS, in order
make elk-down                                 # 5. stops; delete ~/.meurpg/run-sample if you want
```

If step 4 comes back empty, wait about 10 seconds; then `make elk-logs` shows what Filebeat said. To see the Docker environment (`make up`), just play: the `api` of the `meurpg-local` project is read automatically.

### Production (plan)

In production, Cloud Run's stdout already goes to Cloud Logging, and the path to ELK is a Cloud Logging sink to a Pub/Sub topic, read by a Filebeat (`gcp-pubsub` input) or by Elastic Cloud (the Google Cloud integration). **It does not exist yet.** What stays the same: the template, the ILM and the pipeline (`setup/*.json`, applied with `PUT`). What changes:

- **Format.** Pub/Sub delivers a Cloud Logging `LogEntry`, with the line in `jsonPayload`; the pipeline accepts the JSON line in `message` (or already in `json`), so the Pub/Sub Filebeat has to put `jsonPayload` in `json` (a `processors` step in the input) or the pipeline gains a first step for it. Still to be written and tested.
- **Cluster.** Three nodes (or Elastic Cloud), `number_of_replicas: 1` in the template and SSD disk. The local ELK is a single node with zero replicas and is not suitable for production.
- **Network and TLS.** Elasticsearch and Kibana behind a proxy with TLS and login (IAP, or Elastic Cloud SSO); no port open to the internet. The local Kibana speaks HTTP on `127.0.0.1`.
- **Credentials.** The four passwords and Kibana's encryption key in Secret Manager (never in a `.env` in the repository), with the users `meurpg_filebeat` (write only) and `meurpg_reader` (read only) and one API key per service instead of a password. See [Environment variables and secrets](#environment-variables-and-secrets).
- **Who reads.** Only the operations team. The log has `user_id`, which is a pseudonym but is personal data (see [Privacy](privacy.md#logs-in-elk)); the contract forbids e-mail, names and free text.
- **Retention.** 14 days, the same as the local ELK, and shorter than Cloud Logging's 30 days. Changing it means editing `ilm.json`.
- **Size.** At `info`, production writes one line per request (about 300 to 500 bytes): 1 million requests per month are about 0.5 GB, or about 7 GB with the full 14-day retention of a heavy month. At `debug` (rehearsal), count 5 to 10 times more. An estimate by line size, **not measured**.
- **Cost.** An estimate, not a bill and not checked against today's prices: the sink and Pub/Sub cost cents at this volume (the first GiB of the month are free). The real cost is where Elasticsearch runs: a small Google Cloud VM (2 vCPU and 8 GB) is tens of dollars a month, and so is Elastic Cloud's cheapest plan. That is far above the "close to zero" target of this page, so the production ELK is a decision for the project owner, with Cloud Logging (free at the current volume, 30 days) as the alternative.

## Budget alerts

A Google Cloud budget alert warns if the cost exceeds what is expected. The exact thresholds are **planned** (not defined yet).

## Open items before the first deploy

- Domain name (comes from the GitHub Student Developer Pack; only needed on the first deploy).
- Check the region and the backup retention in the CockroachDB Cloud console (São Paulo, 30 days at most).
- Evaluate whether migrating from CockroachDB to Cloud SQL or another product is worth it, since the legacy plan cannot change without losing Unlimited. It weighs on the account: the code uses CockroachDB's row TTL (sessions, sign-in states and invites) and retries transactions on error `40001`; on PostgreSQL, the cleanup would become a scheduled job.
- The full list of secrets per environment and who has access.
- Budget alert thresholds.
- The production ELK (see [Logs in ELK](#logs-in-elk)): whether it is worth the cost, where Elasticsearch runs, the Cloud Logging sink to Pub/Sub and the pipeline step that reads `jsonPayload`.
- The Gemini API key in Secret Manager, the daily quota and the key's API restriction (see [Generated images](#generated-images-the-gemini-api)); measure the real cost per image with it, and adjust `IMAGE_MONTHLY_LIMIT`.
- The images bucket and the Cloud Storage implementation in the `blob` package (private bucket in São Paulo, 7-day soft delete, only the API's service account), and `GOMEMLIMIT` on Cloud Run (see [Images](#images)).

## See also

- [Architecture](architecture.md)
- [Data model](data.md)
- `docs/adr/` (private repository): infrastructure decisions that are hard to undo.
