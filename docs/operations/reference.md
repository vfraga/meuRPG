# Operations reference

Every value below is read from the code or from the Cloud Run configuration; the place is in the last column or the text. For the procedures, see the [runbook](runbook.md).

## Environment variables

The process reads them in `backend/internal/platform/config/config.go` (`config.Load`). An empty value counts as unset, and every problem is reported together in one startup error. `K_SERVICE` (set by Cloud Run itself) turns on the Cloud Run rules below.

| Variable | Secret | Default | Rule |
| --- | --- | --- | --- |
| `PORT` | No | `8080` | 1 to 65535. Cloud Run sets it |
| `LISTEN_HOST` | No | empty (all interfaces) | Leave unset on Cloud Run. The native local stack sets `127.0.0.1` |
| `WEB_DIR` | No | `/app/web` | The Angular build; if the folder is missing the server runs API-only |
| `LOG_LEVEL` | No | `info` | `debug`, `info`, `warn` or `error`, case-insensitive. `debug` makes the log 5 to 10 times bigger |
| `DATABASE_URL` | **Yes** | none | CockroachDB connection string. Empty or unset: the API exits at startup with an error that names the variable. **On Cloud Run it must use `sslmode=verify-full` or `verify-ca`**; any other mode (`disable`, `allow`, `prefer`, `require`) stops the server, and the error never repeats the URL |
| `OIDC_ISSUER` | No | empty | Sign-in is on only with the four `OIDC_*` variables below set together. `https`, no query or fragment; `http` only on loopback or `*.localhost`. `https://accounts.google.com` in production |
| `OIDC_CLIENT_ID` | No | empty | The app's client id at the provider |
| `OIDC_CLIENT_SECRET` | **Yes** | empty | The client secret |
| `OIDC_REDIRECT_URL` | No | empty | The public origin plus `/auth/callback`, exactly as registered at the provider; `https` (or loopback `http`) |
| `OIDC_CA_FILE` | No | empty | PEM file with extra CAs for a local provider; the system roots stay trusted |
| `OIDC_MAX_AGE` | No | unset | 1 s to 720 h, sent as `max_age`. Do not set it with Google, which does not document it |
| `SESSION_IDLE_TIMEOUT` | No | `336h` (14 days) | 1 h to 720 h. How long a session may go unused; the absolute 30 days do not change |
| `TRUSTED_PROXY_HOPS` | No | `1` on Cloud Run | Only on Cloud Run, and only `1` (Cloud Run alone) or `2` (behind the load balancer). Elsewhere any value stops the server. See [the client IP](#the-client-ip) |
| `GOOGLE_CLOUD_PROJECT` | No | empty | Read only on Cloud Run; Cloud Run does not set it, so the deploy does. Without it, log lines carry no trace id ([logs](logs.md)) |
| `BLOB_BUCKET` | No | empty | The Cloud Storage bucket for images. The production store |
| `BLOB_DIR` | No | empty | A local image folder. Refused on Cloud Run, and refused together with `BLOB_BUCKET`. With neither, images are off (`503`) |
| `GEMINI_API_KEY` | **Yes** | empty | Without it (and without the fake), image generation is off and calls answer `failed_precondition` with reason `OFF` |
| `GEMINI_IMAGE_MODEL` | No | `gemini-nano-banana-2.1` | `gen.DefaultModel` |
| `IMAGE_GENERATOR` | No | `gemini` | `gemini` or `fake`. `fake` is for local stacks and CI and is refused on Cloud Run |
| `IMAGE_MONTHLY_LIMIT` | No | `20` | Images per campaign per Brasília month, 1 to 500 (`maps.DefaultMonthlyImages`) |
| `IMAGE_DAILY_LIMIT` | No | `100` | Images for the whole server per Brasília day, 1 to 10,000 (`maps.DefaultDailyImages`) |
| `GEMINI_TTS_MODEL` | No | empty | The text-to-speech model of the conversation's "Ouvir" (MR-051), from Google's current documentation. It has no default: without it (and without the fake) the voice is off and the audio route answers `failed_precondition` with reason `OFF` |
| `TTS_GENERATOR` | No | `gemini` | `gemini` or `fake`. `fake` makes a short silent audio with no network, for local stacks and CI, and is refused on Cloud Run |
| `TTS_DAILY_LIMIT` | No | `300` | Voices for the whole server per Brasília day, 1 to 5,000 (`play.DefaultVoiceDailyLimit`) |
| `MAX_CAMPAIGNS_PER_USER` | No | `10` | Campaigns one account may be master of, 1 to 1,000. `off` removes the cap and is refused on Cloud Run |
| `CAMPAIGN_CREATORS` | No (personal data) | empty | Verified e-mails, comma-separated and case-insensitive, that may create campaigns. Empty: anyone. Only separators, or an entry without `@`, stops the server (the error gives the position, never the text) |
| `RATE_LIMIT_MULTIPLIER` | No | `1` | Multiplies every request-rate limit; above 0 and at most 1,000. The local stacks and the e2e CI run with `10` |
| `TASKS_QUEUE` | No | empty | The full name of the Cloud Tasks queue, `projects/<project>/locations/<region>/queues/<queue>`. The three `TASKS_*` variables are set together or not at all; a partial set stops the server and the error names the missing ones. None: detached work runs in the process |
| `TASKS_TARGET_URL` | No | empty | The service's own `https` URL, scheme and host only (the `run.app` URL): where Cloud Tasks calls back and the audience of the OIDC token. `http` only for localhost |
| `TASKS_SERVICE_ACCOUNT` | No | empty | The service account that signs that token: the service's own, `$SA` |
| `GOMEMLIMIT` | No | unset | Read by the Go runtime, not by `config`. Production sets `400MiB` |

`DATABASE_URL`, `OIDC_CLIENT_SECRET` and `GEMINI_API_KEY` are `config.Secret` values: they print as `[REDACTED]` in a log line or an error. `migrate` reads `DATABASE_URL` too and, when it is invalid, fails with a fixed sentence instead of the driver's error.

The production values that are not defaults, set at deploy: `OIDC_ISSUER=https://accounts.google.com`, `OIDC_CLIENT_ID`, `OIDC_REDIRECT_URL=https://meurpg.app/auth/callback`, `BLOB_BUCKET`, `CAMPAIGN_CREATORS`, `GOOGLE_CLOUD_PROJECT`, `GOMEMLIMIT=400MiB`, `TRUSTED_PROXY_HOPS=2`, and the limits pinned at their defaults. `LOG_LEVEL` is raised to `debug` or lowered to `info` with `--update-env-vars`.

## Secrets

| Secret (Secret Manager) | Variable | Used by |
| --- | --- | --- |
| `database-url` | `DATABASE_URL` | The service and the migration job |
| `oidc-client-secret` | `OIDC_CLIENT_SECRET` | The service |
| `gemini-api-key` | `GEMINI_API_KEY` | The service; optional |
| `cockroach-ca` | file `/etc/cockroach/ca.crt` | Only if the cluster's certificate is not signed by a public CA; mounted on the service and the job, and referenced by `sslrootcert` in `database-url` |

Only the service account `meurpg-api` reads them (`roles/secretmanager.secretAccessor` per secret), and it has no project-level role. A new secret version takes effect in a new revision, or when an instance starts again. How to rotate: [runbook](runbook.md#rotate-a-secret).

## Database connections

| Setting | Value | Where |
| --- | --- | --- |
| Pool size | 10 connections; `pool_max_conns=N` in `DATABASE_URL` overrides | `platform/db` (`defaultMaxConns`) |
| `statement_timeout` | 30 s per command (SQLSTATE `57014`, the API answers `deadline_exceeded`) | `platform/db`; the URL's own value wins |
| `idle_in_transaction_session_timeout` | 60 s | `platform/db`; the URL's own value wins |

With `max-instances=1`, 10 is the whole load on the cluster; if the instances grow, instances times `pool_max_conns` is what counts.
A request waits for a free connection, and the wait ends when a holder lets go (no holder keeps one longer than 30 s).
A request that asks for a second connection while its transaction holds the first stalls the pool; the symptom is `context canceled` or `context deadline exceeded` in simple reads together with a slow request ([architecture](../architecture/transactions.md#transactions-and-the-connection-pool)).
`migrate` has its own connection and no `statement_timeout`; two `migrate up` runs wait for each other on a lease lock ([data](../data/README.md#migration-lock)).

## The client IP

The rate limits find the client in `X-Forwarded-For`, counting `TRUSTED_PROXY_HOPS` items from the right.

| Setup | `TRUSTED_PROXY_HOPS` | Ingress |
| --- | --- | --- |
| Cloud Run alone, on the `run.app` URL | 1 (unset) | `all` |
| Behind the load balancer (production) | 2 | `internal-and-cloud-load-balancing` |

A value too low is safe: every client shares one bucket. A value too high is not: a client could choose its own bucket. That is why only 1 and 2 are accepted, and why 2 goes only with the ingress closed to everything but the load balancer.
After any change to the setup, check that the counted item is the caller's address: make a few requests and read the `client ip source` startup line and the `rate limit hit` lines; if every client shares one bucket, raise `TRUSTED_PROXY_HOPS` or `RATE_LIMIT_MULTIPLIER`.

## Abuse limits

The rate limits are in process memory ([architecture](../architecture/api.md#abuse-limits), rule RN-30). The symptom of a limit is `429` with `Retry-After`, or `resource_exhausted`, and a `rate limit hit` line at `WARN` with the limit name and no IP.

| Limit | Value | Set by |
| --- | --- | --- |
| Every request rate | Multiplied by `RATE_LIMIT_MULTIPLIER` | Variable |
| Campaigns per master | 10 | `MAX_CAMPAIGNS_PER_USER` |
| Who may create campaigns | Anyone, or the `CAMPAIGN_CREATORS` list | Variable |
| Generated images | 200 per campaign per month and 100 per server per day in production (the code's defaults are 20 and 100) | `IMAGE_MONTHLY_LIMIT`, `IMAGE_DAILY_LIMIT` |
| Active invites per campaign | 50 | `campaigns.MaxActiveInvites` (code) |
| Characters and NPCs per campaign (combat-only NPCs not counted toward it) | 1,000 | `characters.DefaultMaxCharactersPerCampaign` (code) |
| Combat-only NPCs per campaign (their own cap) | 1,000 | `characters.DefaultMaxCombatOnlyNpcsPerCampaign` (code) |
| Sign-in attempts | A per-IP and a global limit shared by `/auth/login` and `/auth/callback` | `identity` (`loginRateLimit`) |
| Upload read time | 45 s to send the file while holding the one image-processing slot | `maps/upload.go` (`uploadSlotReadTimeout`) |

To raise a variable limit: `gcloud run services update meurpg --region southamerica-east1 --update-env-vars MAX_CAMPAIGNS_PER_USER=20` (a new revision; the secrets and other variables stay).
The limits are not a defence against a volume attack: that is the job of the [edge policy](runbook.md#the-edge-policy-cloud-armor) and of `max-instances` and `concurrency`.

## Live session stream

`PlayService.WatchGameSession` is open only while someone has the session page open and visible. An open stream is a request in progress, so the instance is billed while it lasts ([architecture](../architecture/play/live-session.md#live-session)).

| Rule | Value | Where |
| --- | --- | --- |
| Server heartbeat | 25 s | `play.DefaultHeartbeat` |
| Re-check of the login session and participation | 60 s | `play.DefaultRecheck` |
| Maximum life of a stream | 10 minutes, then the server ends it without an error and the app opens another. Kept short because behind the load balancer the server is not told when a page went away | `play.DefaultMaxLifetime` |
| Idle limit of the instance | 3 minutes without a unary call; the next re-check ends its streams with `unavailable` (`instance_idle`) and the app reconnects to the active revision | `instanceIdleLimit` in `cmd/api/main.go` |
| Deadline of each message sent | 30 s; then the stream ends (`client_gone`) | `streamSendTimeout` in `cmd/api/main.go` |
| Streams per person per campaign | 4; a fifth replaces the oldest, which ends with `unavailable` | `live.DefaultMaxPerUser` |
| Dead stream, app side | The app reconnects after about 60 s of silence, with a growing wait (1 s up to 30 s, jittered), only with the tab visible | App |
| Hidden tab | The app closes the stream after 2 minutes hidden and reads the snapshot again on return | App |
| Shutdown | Streams end as soon as the graceful shutdown starts (8 s by default) | `httpserver` |

## Memory budgets

The instance has 1 GiB, and the service sets `GOMEMLIMIT=400MiB` (an environment variable of the Cloud Run service, kept by every release).
The caps that keep the worst case inside it are set in code and listed in [Performance](../performance.md#budgets-the-code-enforces), with the commands that measure them.
None of these caches is stored on disk or in the database: a restart empties them and they rebuild on the next read.
More fogged maps in play at once than `workingCopies` makes the working copies take turns and first tiles slower (`503` `BUSY` becomes likelier); the remedy is a higher `workingCopies` or more instances.
More than 8 campaigns with table content in play at once makes the content cache thrash, and each read assembles again; the number to change is `maxLiveContents`.

## Images and the bucket

Gallery images live in a blob store ([architecture](../architecture/maps/images.md#where-images-are-stored)); production uses one Cloud Storage bucket.

- The bucket is in `southamerica-east1`, Standard class, with uniform bucket-level access, public-access prevention and a 7-day soft delete (the deadline in [Privacy](../privacy.md)).
- Only `meurpg-api` has access, as `roles/storage.objectUser` on the bucket. There is no public URL and no signed URL: the API delivers every image after checking who is asking.
- The store (`platform/blob`, `GCS`) calls three methods of the JSON API with `net/http`: a media upload, a streamed read (with `Range` when the reader seeks) and a delete. The token comes from the metadata server and is cached until a minute before it expires. Calls have time limits (5 s for the token, 15 s for headers and deletes, 2 minutes for an upload and for an open read); a read is repeated once after a network error or a `5xx`.
- At start the server lists one object to check access and logs `the images bucket is reachable` or a warning. A failed check does not turn images off: each call succeeds or fails on its own. If the warning is not a passing hiccup (a wrong bucket name, a missing role), every image call fails until the cause is fixed and a new revision is deployed.
- A key is the object name, unchanged (`campaigns/<campaign>/images/<id>`); a new kind of file takes its own prefix under the campaign's.
- The lifecycle rule is a second net for the campaign package: delete objects under `imports/` after 1 day and the export zips (under `campaigns/`, ending in `.zip`) after 2.

### Campaign package

Export and import of a campaign (MR-050, [architecture](../architecture/campaign-package.md#campaign-package-mr-050)) use the same store and are off when it is off.

| What | Value | Where |
| --- | --- | --- |
| Package | At most 200 MiB, 10 MiB per entry, 500 MiB once unpacked, 2,000 entries, a compression ratio of at most 100 | `campaignpackage/format.go` |
| Upload parts | 5 MiB each, 2 minutes to send one; the parts are kept an hour after the last | `PartSize`, `partReadTimeout`, `UploadTTL` |
| Export file | `campaigns/<id>/exports/<id>.zip`, kept 24 hours, 20 minutes at most per export | `ExportTTL`, `exportTimeout` |
| Download | Between 2 and 30 minutes for the response, by size | `downloadFloor`, `downloadCeiling` |
| Cleanup | A sweep every 10 minutes (in the process, one at start; on Cloud Tasks, a chain of tasks, [Cloud Tasks](#cloud-tasks)); a running export idle for 10 minutes is marked interrupted | `campaignpackage/sweep.go`, `staleExportAfter` |

## Cloud Tasks

With the three `TASKS_*` variables set, every piece of detached work (an image being made, an export being written, the sweep of what expired) is a task on one queue in the service's region, and Cloud Tasks calls the service back at `POST /internal/tasks/<kind>` with an OIDC token (`platform/tasks`, [architecture](../architecture/tasks.md)). That is how the work gets CPU: the service bills CPU per request, and a goroutine that outlives its call would starve.

| What | Value | Where |
| --- | --- | --- |
| Queue | `meurpg-tasks` in `$REGION`: 5 attempts, backoff 10 s to 300 s (4 doublings), retry for 1 hour at most, 5 dispatches a second, 4 at once | [runbook](runbook.md#11-cloud-tasks) |
| Call | `POST $TASKS_TARGET_URL/internal/tasks/<kind>`, JSON body of IDs, `Authorization: Bearer <OIDC token>` | `platform/tasks/cloudtasks.go` |
| The check | Issuer `https://accounts.google.com`, audience `TASKS_TARGET_URL`, `email` equal to `TASKS_SERVICE_ACCOUNT` and `email_verified`. Anything else answers `401` with an empty body | `platform/tasks/endpoint.go` |
| Deadline | Each kind sets the time one delivery may run, 30 minutes at most: the limit of Cloud Tasks for an HTTP target, below Cloud Run's 35-minute request timeout | `tasks.MaxDeadline` |
| Answers | `200`: done, or a failure a retry cannot help (recorded in the row); `500`: Cloud Tasks retries | `platform/tasks/endpoint.go` |
| Sweeps | A chain of named tasks, one per 10-minute slot (`sweep-<kind>-<slot>`): each delivery first enqueues the next slot, an instance starting enqueues it too, and `ALREADY_EXISTS` counts as success, so several instances never double a sweep | `tasks.CloudTasks` |
| Client | `net/http` against `cloudtasks.googleapis.com/v2`, the token from the metadata server cached until a minute before it expires; a call is repeated twice after a network error, a `429` or a `5xx` | `platform/tasks/cloudtasks.go` |

The endpoint is not under the per-IP limit: `ratelimit.Middleware` limits only the API prefixes (`/meurpg.`, `/images/`, `/uploads/`, `/session-messages/`). The load balancer forwards every path, so `/internal/tasks/*` is reachable from the internet through the domain too: the token check is its only gate.

## Generated images (the Gemini API)

- No image model has a free tier: billing is on, and the paid-services terms apply.
- The key is restricted to the Gemini API in Google Cloud. The project has a daily quota at Google as a hard cap, and the app has its own (`IMAGE_DAILY_LIMIT`): it refuses with `DAILY_LIMIT_REACHED` before calling Gemini.
- The per-campaign limit is counted in the database (`image_requests`) by the Brasília month (UTC-3). A refusal, a response without an image, a rejected key and a cancel before the request leaves give the slot back; a cancel after it, a timeout, a cut connection, an unreadable answer and a picture that cannot be stored do not (they may have been billed).
- The service runs with CPU only during a request, so the app keeps a long wait open (`GetImageGeneration` with `wait_seconds` 25) while an image is generated; the instance always has a request in progress. At shutdown, queued requests give their slot back and a call already sent is cut.
- The server always asks for 1K images. The price per image depends on the model; see Google's [pricing page](https://ai.google.dev/gemini-api/docs/pricing). `TestRealGemini` (needs a key) measures the real answer, and the first bill shows the real cost.

## Alerts

| Alert | Fires when | Goes to |
| --- | --- | --- |
| Uptime check `meurpg-readyz` and policy `MeuRPG is down (readyz)` | `https://meurpg.app/readyz` fails from 2 or more checker regions for 10 minutes (period 5 minutes, timeout 10 s). `/readyz` answers `200` only with the database up | The operator's e-mail channel |
| Budget `R$100 Monthly budget alert` | The whole billing account passes 50, 90, 100 and 150 % of R$ 100 a month | The billing account's administrators |
| Budget `MeuRPG projeto R$250/mes` | This project passes 80, 100 and 120 % of R$ 250 a month, or 100 % of the forecast | The same |

A budget only warns; it never stops a cost. What caps the largest variable cost, the AI images, is the app's own daily and monthly limits. How to create each alert: [runbook](runbook.md#alerts).

## Costs

The costs as configured. These are estimates for deciding architecture, not a bill; prices change, so each row points to the price page.

| Item | Driver | Price page |
| --- | --- | --- |
| Cloud Run | 2 vCPU and 1 GiB, billed per request time, scales to zero. A stream open all month would cost tens of dollars, which is why streams exist only during a session and end after 10 minutes | [Cloud Run pricing](https://cloud.google.com/run/pricing) |
| Egress | Billed per GiB out of São Paulo with no free tier. Map images go through Cloud Storage and the browser caches them (the master's for a year, immutable; the player's revalidates with a `304`) ([architecture](../architecture/maps/images.md#serving-images)) | [Network pricing](https://cloud.google.com/vpc/network-pricing) |
| Global external Application Load Balancer | A forwarding rule billed all month plus a per-GB charge; the largest fixed cost | [VPC pricing](https://cloud.google.com/vpc/network-pricing) |
| Cloud Armor Standard | A policy, a charge per rule and per million requests, billed all month | [Cloud Armor pricing](https://cloud.google.com/armor/pricing) |
| Cloud Storage | Standard class, São Paulo, per GiB per month; the 7-day soft delete is billed at the same price; reads are class B operations | [Storage pricing](https://cloud.google.com/storage/pricing) |
| Cloud Build | One build per release on `E2_HIGHCPU_8`, per build-minute | [Build pricing](https://cloud.google.com/build/pricing) |
| Domain and DNS | The `.app` yearly price (renews automatically) and one Cloud DNS zone | [DNS pricing](https://cloud.google.com/dns/pricing) |
| Cloud Tasks | Billable operations (an API call or a delivery attempt). The first 1,000,000 a month are free and each million after costs US$ 0.40; the app makes a few thousand a month | [Cloud Tasks pricing](https://cloud.google.com/tasks/pricing) |
| Gemini images | Per image of output on the configured model, plus input tokens; capped by the two image limits | [Gemini pricing](https://ai.google.dev/gemini-api/docs/pricing) |
| CockroachDB | The project owner's plan; changing plan loses the legacy Unlimited plan | The cluster console |

Backups stay in São Paulo and are kept for 30 days at most, to meet the deletion deadline in [Privacy](../privacy.md).
