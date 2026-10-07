# Review unit 1: sign-in, permissions, campaigns and the platform

- Scope: `backend/internal/{identity,authz,campaigns,platform/*}`, `backend/cmd/api`.
- Commit reviewed: `ae40f74` (main).
- Time spent: about 1 hour 15 minutes of wall clock (4 parallel readers, then 18 independent verifiers).
- Model: the main session ran on `claude-sonnet-5-5`; the four finder agents inherited it; the 18 verification subagents ran on `sonnet`.
- Method: finders read the code and listed candidates; each candidate then went to a fresh verifier that got only its description and had to write a failing test or refute it. Failing tests are in the tree, one file per finding: `review01_u1_NN_test.go`, test names `TestReview1_*`.
- Run all integration tests with `MEURPG_TEST_DATABASE_URL='postgresql://root@localhost:26257/defaultdb?sslmode=disable' go test -race -run TestReview1 ./...` from `backend/`.
- No auth bypass, open redirect, session fixation, cross-campaign IDOR, or secret in a log was found. The sign-in, redirect, state/PKCE, session lookup, invite lock and the RN-30 cap paths were traced and are sound.

| id | severity | status | file:line | defect | failure scenario | evidence |
|---|---|---|---|---|---|---|
| U1-01 | medium | confirmed | `campaigns/queries.sql` GetMembership (~40-45), ClearPendingExpiry (~62-66) | A pending member past the 30-day deadline is still treated as pending until the daily TTL job deletes the row, and can clear the deadline. | RN-15 member with no character, 31 days old, TTL not yet run: still passes pending checks, and creating a character runs ClearPendingExpiry which removes the deadline for good (the row never expires). `acceptInvite` also sees "already pending" for the stale row and spends no use. | `TestReview1_StalePendingMember` (`campaigns/review01_u1_01_test.go`): "GetMembership of an expired pending member = {player pending}, want no rows"; "ClearPendingExpiry removed the deadline of an already-expired pending member". |
| U1-03 | medium | confirmed | `platform/ratelimit/http.go:42`, `platform/httpserver/health.go` | `/readyz` pings the database on every call and is outside `limitedPrefixes`, so no per-IP limit applies; the code comment claims probes touch no database. | Unauthenticated loop on `GET /readyz` takes pool connections (10 in the pool, 2 s each) and starves real RPCs; logged only at DEBUG. | `TestReview1_ReadyzIsUnlimitedDBHit`: "flood=5000 limited(429)=0 db pings=5000". |
| U1-04 | medium | confirmed | `identity/login.go` handleCallback (~276-345), `ratelimit/http.go:42` | `GET /auth/callback` has no rate limit; only `/auth/login` calls `allowLogin`. | Attacker invents X, sends `?state=X` with cookie `__Host-meurpg_login=X`: state==cookie passes, `takeLoginState` runs a DB delete per request, bounded only by the pool. | `TestReview1_CallbackIsRateLimited`: "500 callback requests from one IP, none got 429: every one reached the database". |
| U1-05 | medium | confirmed | `characters/rpc.go:~112-122` (outside unit, uses `platform/idem`), migration 00123 index | CreateCharacter idempotency key is scoped to the campaign, not the caller; architecture.md rule 9 says owner:key. | Members A and B of one campaign send the same key and body: B gets A's character back as replayed (with A's `player_user_id`) and B's create is dropped; a different body leaks that the key exists. | `TestReview1_CreateKeyIsPerCaller`: "B got A's character ... B's create was dropped". |
| U1-06 | medium | confirmed | `platform/rpcerr/rpcerr.go` classify/isConnectionFailure (~75-101); `platform/db/tx.go:55` | `*crdb.AmbiguousCommitError` (connection lost during COMMIT) unwraps to io.EOF and is answered as retryable `unavailable`, though the write may have committed. | Connection drops at RELEASE SAVEPOINT on a create without idempotency key: client is told to retry, a second row is inserted. | `TestReview1_AmbiguousCommit`: "ambiguous commit reported as unavailable ... clients retry it and may duplicate the write". |
| U1-11 | low | confirmed | `cmd/api/main.go:~335` | `identityService.Mount` is called without the per-user limiter, unlike every other service; docs say Connect calls are limited by signed-in user. | 23 `CountOtherSessions` calls with a per-user burst of 3 never get 429; `UpdateProfile` and `SignOutOtherSessions` (DB writes) are only bounded by the per-IP 100/s. | `TestReview1_IdentityRPCsAreRateLimitedPerUser` (control `ListMyCampaigns` does return 429): "23 identity calls with burst 3 were never rate limited". |
| U1-07 | low | confirmed | `platform/db/tx.go:75-77` | After a 40001 retry, `ReadTx` runs read-write: cockroach-go's retry does ROLLBACK then a raw BEGIN, which drops the READ ONLY option. | A write inside `ReadTx` is rejected on attempt 1 but commits on attempt 2. No caller writes today, so it is a latent safety gap. | `TestReview1_ReadTxStaysReadOnlyAfterRetry`: "attempts=2 err=<nil> writeErr=<nil>", "rows committed by ReadTx = 1, want 0". |
| U1-08 | low | confirmed | `platform/names/names.go:38-41, 64-70` | `Clean`/`CleanText` accept only-invisible names (U+200B, 200C/D, 2060, FEFF, 3164) and U+2028/2029 inside one-line fields; no NFC. Docs say one-line fields refuse line breaks. | `UpdateProfile` with display name U+200B: stored, renders blank, passes the DB CHECK. | `TestReview1_InvisibleChars`: "Clean("​") accepted ... want an error"; "Clean("ab cd") accepted". |
| U1-09 | low | confirmed | `platform/config/config.go:~571-586` | `CAMPAIGN_CREATORS` of only separators (",") yields an empty list, which `campaigns/limits.go:27` reads as "anyone may create"; no startup error. | Operator sets `CAMPAIGN_CREATORS=","` meaning to restrict; everyone can create campaigns. | `TestReview1_CreatorsOnlySeparators`: "Load() error = nil, creators=[]". |
| U1-12 | low | confirmed | `identity/login.go:~393-399` | The previous session is revoked before the new one is created. | Signed-in user re-signs in and `createSession` fails (DB blip): the old session is gone, the user is signed out. | `TestReview1_OldSessionSurvivesFailedRelogin`: "old session no longer valid after failed re-login". |
| U1-13 | low | confirmed | `identity/queries.sql:65-67, 35-37` | "Sign out other devices" skips idle-but-unexpired rows; raising `SESSION_IDLE_TIMEOUT` later (or an instance with a larger value) makes them valid again. Docs state the filter, not the revival. | Sign out others, operator raises timeout 336h to 720h: a stolen idle session works again until `expires_at`. | `TestReview1_SignOutOthersIdleRevived`: "signed-out-others session revived after raising idle timeout: err = <nil>, want ErrNotFound". |
| U1-15 | low | confirmed | `platform/httpserver/static.go:~142` | Every static file except index.html gets `public, max-age=31536000, immutable`, but `favicon.ico` and `material-symbols/*` are not hashed (architecture.md:292 says all are). | A changed favicon or icon font is served stale for a year to returning browsers. | `TestReview1_UnhashedStaticNotImmutable`: "unhashed favicon.ico must not be immutable, got public, max-age=31536000, immutable". |
| U1-02 | n/a | refuted | `campaigns/invites.go:~159-208` | Same user accepting two different invites at once: CockroachDB reports a retryable conflict, `InTx` retries and the loser takes the already-member path. | 120 races (8 goroutines, 40 rounds, 2 runs) all succeeded. Test removed (it passes). | n/a |
| U1-10 | n/a | refuted | `campaigns/tablerules.go:305-340` | Last writer wins on SetTableRules, but docs (RN-24, proto comment) define "replace all, each field as sent". Design, not defect. | n/a | n/a |
| U1-14 | n/a | refuted | `platform/config/config.go:~265` | `BLOB_DIR` accepted on Cloud Run, no GOMEMLIMIT check: docs promise neither a guard nor a check; the Cloud Storage backend does not exist yet. | n/a | n/a |
| U1-16 | n/a | refuted | `httpserver/middleware.go:44-61` | 429s still log one INFO line; the "does not fill the log" promise (architecture.md:195) is about the limiter's own throttled WARN. | n/a (doc wording could be clearer). | n/a |
| U1-17 | low (docs) | refuted as code bug | `campaigns/invites.go:161-170`; `docs/architecture.md:525` | Code follows Q25 (stories.md:441, architecture.md:495). Only architecture.md:524-525 ("remain pending with any invite") is stale. | n/a | n/a |
| U1-18 | n/a | refuted | `campaigns/rpc.go:308-368` | No invite cap, no CreateInvite idempotency key and unpaginated lists are documented choices (architecture.md:105). | n/a | n/a |
| U1-19 | medium | unverified | `cmd/api/main.go:125,581`, `slowclient`, `httpserver/server.go:299-309` | Unary RPC bodies up to 4 MiB are read before any auth, with no body deadline: stalled POSTs hold memory against the 512 MiB budget (architecture.md lists it as "left for later"). | Settle with a test that opens N stalled 4 MiB POSTs and measures heap, plus the Cloud Run concurrency setting (no deploy config in repo). | none |
| U1-20 | medium | unverified | `platform/db/db.go:76-112`, `db/tx.go:51-56` | No deadline on pool acquisition or whole transaction; with 10 busy connections requests queue until the client leaves (or 35 min). | Settle with a test saturating a small pool with `pg_sleep` and timing a request. | none |
| U1-21 | low | unverified | `ratelimit/policy.go`, `ratelimit.go` | About 5 IPs at 100 req/s drain the global IP bucket (2000 burst, 500/s) so signed-in users get 429; documented "no DDoS defence". | Needs a limiter simulation. | none |
| U1-22 | low | unverified | `identity/login.go:67-80` | Cross-site GET `/auth/login` overwrites the login cookie mid-flow, so the first callback fails `state_mismatch` (annoyance only). | Settle with a two-request handler test. | none |
| U1-23 | low | unverified | `campaigns/tablerules.go:222-230` | `GetTableRules` reads campaign and rules as two pool statements, so a concurrent Set can yield new dice mode with old rules. | Needs an interleaving test with a hook. | none |
| U1-24 | low | unverified | `platform/config/config.go:419,435` | Startup error echoes an `OIDC_ISSUER`/`OIDC_REDIRECT_URL` value that contains userinfo. | One-line config test would settle it. | none |
| U1-25 | low | unverified | `backend/migrations/lock.go` heartbeat | Renew UPDATE has no per-call timeout, so a stalled DB can let a run migrate past its lease. | Needs a stalled-connection test. | none |
| U1-26 | low | unverified | `ratelimit/http.go:42`, `httpserver/static.go` | Static files are unlimited, so egress cost is unbounded (operations.md prices it). | Cost question, no failure to test. | none |
| U1-27 | low | unverified | `platform/nostore/nostore.go:173-178`, `rpclog.go:178-184` | `Cache-Control: no-store` can be lost when a handler returns a plain error (rpclog rebuilds it). A 500 is not heuristically cacheable. | A chain test with a plain error. | none |

## Fix directions (confirmed findings)

### U1-01 stale pending membership
- Root cause: the 30-day deadline is enforced only by the CockroachDB row TTL (about daily), never by the queries that read or mutate the row.
- Fix: `campaigns` queries. Add `AND (status = 'active' OR pending_expires_at IS NULL OR pending_expires_at > $now)` to `GetMembership`, `ActivatePendingMember`, `ListPendingMembersWithoutCharacter`, and a `pending_expires_at > now` guard to `ClearPendingExpiry`; pass `now` from the service clock. Alternative: delete expired rows lazily inside `acceptInvite`; more code and writes, so pick the query guard.
- Same pattern: `campaigns/queries.sql` ActivatePendingMember (~54-58), ListPendingMembersWithoutCharacter (~68-72); `invites.go:~152-181`; `authz/authz.go:175-180` consumes GetMembership; `characters/rpc.go:214` calls ClearPendingExpiry.
- Rules: RN-15; no pool reads in `InTx` (pass `now`, do not read the clock in SQL if tests use a fake clock); docs: architecture.md "Pending member".
- Risk: an invite accepted by a user whose row just expired must insert a fresh row (primary key clash with the stale row: delete it in the same tx). Guards: the RN-15 and Q25 tests in `campaigns`.

### U1-03 and U1-04 unlimited DB-touching routes
- Root cause: `limitedPrefixes` is an allow-list of prefixes; routes that hit the database (`/readyz`, `/auth/callback`) are outside it, and the comment wrongly says probes and `/auth/` are covered.
- Fix: `platform/ratelimit/http.go` and `identity` callback. For `/auth/callback`, call `allowLogin` (same limiter as login) at the top of `handleCallback`, before the state check. For `/readyz`, cache the ping result for a few seconds (singleflight) instead of limiting by IP, because Cloud Run probes share addresses; fix the comment.
- Same pattern: any other non-prefixed route that reaches the pool (check `httpserver/server.go` mux).
- Rules: login limit is per instance in memory; Cloud Run client IP rule (right-most XFF); update architecture.md "Abuse limits" and "Login attempt limit".
- Risk: the callback shares the login budget (20 burst per IP); a table behind one Wi-Fi signing in together is the case `login_ratelimit_test.go` guards.

### U1-05 idempotency key scope
- Root cause: `createKey` is the bare client UUID and the unique index is (campaign_id, create_key); the rule is owner:key.
- Fix: `characters` create paths: make the unique index (campaign_id, player_user_id, create_key) (or a TEXT column with `idem.Scope(userID, key)` as migrations 00178-00180 do) and look up by the same tuple. Index change is a new migration, idempotent.
- Same pattern: `characters/npcfromcreature.go:58`, `characters/combatmonsters.go:106` (unchecked). Already scoped: `charactercreatures.go:302`, `tablecontent_rpc.go:300`, `progression/milestones.go:103`.
- Rules: architecture.md rule 9 and "Left out, and why"; migrations must run twice.
- Risk: existing rows keep working (null-safe tuple); guard: the idempotency tests in `characters`.

### U1-06 ambiguous commit
- Root cause: `rpcerr.classify` unwraps `AmbiguousCommitError` to the underlying connection error and calls it retryable.
- Fix: `platform/rpcerr`: test `errors.As(err, *crdb.AmbiguousCommitError)` before `isConnectionFailure` and return a distinct non-retry answer (`CodeUnknown`/`internal` with a fixed message "the change may or may not have been saved, check before retrying"); log at ERROR. Alternative: nothing better; this is the smallest.
- Same pattern: every `db.InTx` write (single place: `db/tx.go:55`), worst for creates without an idempotency key (`characters/npcfromcreature.go`, `progression/award.go`).
- Rules: error codes and fixed messages in architecture.md "Error codes"/"Database errors"; update that section.
- Risk: the client UI maps `unavailable` to a retry button; check the web error handler for the new code.

### U1-11 identity RPCs without the per-user limiter
- Root cause: `mountModules` mounts identity before `limitedSessions` exists and `identity.Mount` only adds its own interceptor.
- Fix: `cmd/api/main.go`: build `limitedSessions` first and let `identityService.Mount` chain it (per-user limit needs the session the identity interceptor resolves, so chain the limiter after it).
- Same pattern: none; `SystemService` is public and keyed by IP only.
- Rules: architecture.md "Abuse limits" table. Risk: sign-in flow uses `GetMe` heavily; the limit should be generous. Guard: `cmd/api` tests plus `TestReview1_IdentityRPCsAreRateLimitedPerUser`.

### U1-07 ReadTx after retry
- Root cause: read-only is set only in the `BEGIN` options; crdb's retry restarts with a plain BEGIN.
- Fix: `platform/db/tx.go` `readTx`: run `SET TRANSACTION READ ONLY` as the first statement of every attempt (inside the wrapper closure), instead of relying on `TxOptions`.
- Same pattern: only `tx.go:76`; callers `play/live.go:184`, `combat_view.go:550`, `combat_log.go:106`, `highlights.go:215`, `maps/layers.go:479`, `mapservice.go:148`, `visibility.go:135`.
- Rules: one connection in tests, so no pool reads inside the closure. Risk: low; guard `db_integration_test.go:244`.

### U1-08 names
- Root cause: `Clean` blocks Cc and Bidi_Control only.
- Fix: `platform/names`: reject `unicode.Is(unicode.Cf, r)`, U+2028/2029, U+3164, U+115F/1160; require at least one letter, digit or symbol after trimming; normalise to NFC (`golang.org/x/text` is probably already a dependency; check before adding). Keep ZWJ legal only if emoji names are wanted (product decision).
- Same pattern: callers `identity/rpc.go:212`, `characters/rpc.go:59,355,695`, `npcfromcreature.go:40`, `charactercreatures.go:193`, `progression/award.go:197`, `milestones.go:408`, campaign name in `campaigns/rpc.go`. Docs: architecture.md:994, data.md:256. Risk: existing stored names are not re-validated.

### U1-09 CAMPAIGN_CREATORS
- Root cause: parser skips empty entries and never checks that a non-empty value produced entries.
- Fix: `platform/config/config.go`: error when the trimmed value is non-empty and zero entries were accepted. Same pattern: check other list variables (not audited). Docs: operations.md:147.

### U1-12 revoke order
- Root cause: `revokeQuietly` runs before `startSession`.
- Fix: `identity/login.go` `completeLogin`: call it after `startSession` succeeds (a new token is always fresh, so fixation stays impossible). Same pattern: none found. Risk: low; guard the login tests.

### U1-13 sign out others
- Root cause: `DeleteOtherUserSessions` treats idle as already dead using today's timeout.
- Fix: `identity/queries.sql`: delete all other sessions of the user that are not expired (drop the idle condition); same for `CountOtherSessions` if the count should match what a longer timeout would show (otherwise leave it). Same pattern: `store.go:317,321`, memstore. Doc: architecture.md:400.

### U1-15 immutable on unhashed files
- Root cause: `static.go` assumes outputHashing hashes every asset.
- Fix: `platform/httpserver/static.go`: send `immutable` only for names matching Angular's hash pattern (for example `-[A-Za-z0-9]{8}\.`); others `no-cache`. Same pattern: `favicon.ico`, `material-symbols/outlined.css` and `.woff2`. Doc: architecture.md:292. Risk: a few more conditional requests; guard `static_test.go`.
