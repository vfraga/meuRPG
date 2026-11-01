# API

Every API call is declared in a `.proto` file, and buf generates the Go and TypeScript code from it.
This page is the contract for how calls are named and versioned, how they fail, how they are repeated safely and how the server protects itself from a client that does too much.

## Contracts

Rules for the files in `proto/meurpg/<module>/v1/`:

1. **One package per module and version:** `meurpg.<module>.v1`.
   The modules and their services follow the code, not a table here: read `proto/`.
2. **Services by subject.** A module has one service as a rule; a separate subject may have its own service in the same package (`campaigns` has `CampaignService` and `CampaignDocumentService`; `play` has several).
   Every call has its own `XxxRequest` and `XxxResponse`, even when empty, as `buf lint` demands.
3. **A field number never changes and is never reused.** A removed field becomes `reserved`.
4. **A breaking change becomes a `v2` package.** CI runs `buf breaking` against `main`.
5. **Generated code is committed** (`backend/gen`, `web/src/gen`).
   CI generates again and fails on any difference.
   `proto/buf.gen.yaml` runs `protoc-gen-go` and `protoc-gen-connect-go` (pinned as `tool` directives in `backend/go.mod`) and `protoc-gen-es` (from `web/node_modules`).
6. **Fields are `snake_case`** in the `.proto`; the generated TypeScript is `camelCase`.
7. **A read-only call declares an idempotency level**, which depends on the request:
   - A request with no ID and no personal data (empty, like `GetMe`, `ListMyCampaigns` and `GetServerInfo`) is `NO_SIDE_EFFECTS`.
     Connect then accepts GET and the browser may cache it.
   - A request with an ID or personal data (like `GetCampaign` or `GetCharacter`) is `IDEMPOTENT`.
     It is a read and safe to repeat, but accepts only POST: in a GET the whole message would be in the URL, and URLs reach the platform logs ([Privacy](../privacy.md)).
   - `TestConnectGETOnlyForRequestsWithoutData` (`backend/cmd/api`) fails if a `NO_SIDE_EFFECTS` method has a request with a field.
8. **Names follow Google AIP.**
   Standard methods start with `Get`, `List`, `Create`, `Update` or `Delete` plus the resource (AIP-131 to 135).
   Custom methods start with the action verb (`AcceptInvite`, `RevokeInvite`; AIP-136).
   A search with a filter is `Search<Resource>`, `IDEMPOTENT`.
   An existing name never changes: renaming an RPC breaks the contract.
9. **Every call that creates something carries an `idempotency_key`** ([below](#idempotency-keys)).
10. **Every unary call requires `Connect-Protocol-Version: 1`** (`connect.WithRequireConnectProtocolHeader()`), which is also a CSRF layer ([Identity](identity.md#csrf)).
    A `curl` needs the header:

```bash
curl -s -X POST http://localhost:8080/meurpg.system.v1.SystemService/GetServerInfo \
  -H 'Content-Type: application/json' -H 'Connect-Protocol-Version: 1' -d '{}'
```

The server reads no RPC message over 4 MiB (`maxRequestBytes`), and a unary Connect call has a lower body limit per procedure ([Request size limits](#request-size-limits)).

## Error codes

Every refused business rule returns a Connect code, always the same one for the same case.

| Code | When |
| --- | --- |
| `unauthenticated` | No valid sign-in session. |
| `permission_denied` | The caller is a campaign member but lacks the role (a player tries a master action), or the account may not create campaigns (`CampaignCreationRefused`, `NOT_ALLOWED`, RN-30). |
| `failed_precondition` | The rule does not allow it now: a locked sheet (RN-01), a session that has not started, an unusable invite, an image used on a map. When there is more than one reason, a typed detail says which (`InviteUnusable`, `CharacterBlocked`, `GameSessionBlocked`, `XPBlocked`, `XpModeChangeBlocked`, `ImageInUse`...). |
| `not_found` | It does not exist, or the caller may not know it exists: a campaign the caller is not a member of, a map hidden from a player (ADR-0011). A pending member gets the same `not_found` outside the few calls they may make. |
| `invalid_argument` | Invalid input, or a request with a field over its size limit (a `RequestRefused` detail, [Request size limits](#request-size-limits)). |
| `resource_exhausted` | A quota ran out: a campaign, gallery or map limit, the account's campaign cap, pending image requests, live streams per person. A unary body over its read limit, or a message over the 4 MiB backstop, gets it too (from Connect, with no detail). A rate limit uses the same code with a `Retry-After` header ([Abuse limits](#abuse-limits)). |
| `aborted` | A transaction conflict (`40001`) that persisted after the retries, or a sheet or document that changed since the app read it (stale revision, AIP-154). The app reloads and the person tries again. |
| `deadline_exceeded`, `canceled`, `unavailable`, `unknown`, `internal` | Infrastructure outcomes, in the next table. |

Typed details are messages in the `.proto` of the service that returns them; each `.proto` comment says which reasons a call can give.

### Database errors

A read or write error becomes a Connect code in one place, `platform/rpcerr` (`rpcerr.FromDB`), which every module calls through its `dbError`.

| What happened | Code | Log level |
| --- | --- | --- |
| A Connect error the handler chose | the same, not logged again | none |
| The request was canceled (the client left) | `canceled` | `DEBUG` |
| The request deadline passed, or the database canceled a statement over `statement_timeout` (SQLSTATE `57014`, 30 s by default, `db.defaultStatementTimeout`) | `deadline_exceeded` | `WARN` |
| `db.InTx` retried `40001` until it gave up | `aborted` | `WARN` |
| A commit lost its connection midway (`crdb.AmbiguousCommitError`): the change may or may not be saved | `unknown`, with a sentence telling the client to check before trying again (never `unavailable`: a blind retry of a create without a key could duplicate it) | `ERROR` |
| The database was unreachable (class `08`, `57P01`, `57P03`) | `unavailable` | `ERROR` |
| Anything else | `internal` | `ERROR` |

The client receives a fixed sentence, never the error text (it may hold SQL or a server name); the cause goes to the log line, once.
The error keeps the original as its cause (`Unwrap`), so a method that runs inside another module's `db.InTx` may return it from the closure and `InTx` still sees a `40001` and retries.
A module handles its own special cases first (the row that vanished between the check and the query).

### Panics

`net/http` recovers only from a panic in the handler's own goroutine; a goroutine the server starts that panics takes down the process and every live session with it.

- **Connect handlers** have `connect.WithRecover` right after `rpclog`: the panic becomes `internal` with a fixed sentence, the stack goes to the log (`rpc handler panicked`, with the request id, user and campaign), and `rpclog` still writes the `rpc` line with `code=internal`.
- **Background goroutines** (the image generation request, the live-hint timers) start with `defer safego.Recover(logger, "...")` (`platform/safego`): the work that broke is lost, the stack goes to the log and the server keeps going.

## Error reasons

Every reason the server can send a client for a refusal is in one closed list, `proto/error-reasons.json`, and the web has a Portuguese sentence for each.
The API does not use `google.rpc.ErrorInfo`: the reason is an enum in a typed detail where the call is a Connect call, and a word in a JSON body or a query where it is not.
A test fails when the server can send a reason that is not listed, and another when a listed reason has no sentence in the web.

### Where a reason travels

| Carrier | Used by | Closed by | The sentence is in |
| --- | --- | --- | --- |
| An enum field of a typed error detail (`CharacterBlocked.reason`) | The Connect calls that refuse a rule | The `.proto` enum, mirrored by the list's `details` | The module's `web/src/app/core/<module>/*-errors.ts`, one `Enum.VALUE` per reason |
| `reason` in the JSON body `{"code", "reason", "message"}` of a plain HTTP route | Image upload and downloads, package upload, the voice of a line, the rate limit of every API route | The `rpcerr.Reason` constants (`platform/rpcerr/reasons.go`), mirrored by the list's `body` | `core/errors/body-reasons.ts` |
| `TableContentViolation.reason`, a string | The writes and the import of `TableContentService` | The constants `rules.Reason*` and the characters package's `reason*`, mirrored by the list's `violations` | `core/content/content-violations.ts` |
| `?reason=` of the redirect to `/invite/error` | Accepting an invite at sign-in | The constants `campaigns.InviteFailure*`, mirrored by the list's `redirects` | `describeInviteErrorCode` in `core/campaigns/invite-errors.ts` |

Identity and notes answer with a Connect code alone.
The `reason` in identity's sign-in log line is for the log and never reaches the client.

### By module

| Module | Typed details with a reason | Plain routes with a reason | Registered in `rpcerr` |
| --- | --- | --- | --- |
| `characters` | `CharacterBlocked`, `ChoiceRefusal`, `AbilityScoresRefusal`, `LevelUpRefusal`; `InvalidField` names a field | none | n/a |
| `campaigns` | `InviteUnusable` (its `state`), `CampaignCreationRefused`; `XpModeChangeBlocked` carries counts | the redirect to `/invite/error` | n/a |
| `campaignpackage` | `CampaignPackageBlocked`; `PackageProblem` in a response | package upload | yes |
| `maps` | `TreasureBlocked`, `MapBlocked`, `ImageGenerationBlocked`; `ImageInUse`, `DungeonOptionRefused` and the invalid-field details name what they refuse | image upload, image and tile downloads | no: spelled as constants and literals, named in `unmigratedBodyReasons` |
| `play` | `EncounterBlocked` (and its familiar-sight reason), `EncounterBuildBlocked`, `SceneBlocked`, `PuzzleBlocked`, `PuzzleInvalid`, `GameSessionBlocked`, `ResourceBlocked`, `ContestBlocked`, `CastingBlocked` | the voice of a line | no: literals, named in `unmigratedBodyReasons` |
| `progression` | `XPBlocked` | none | n/a |
| `rules` | `TableContentBlocked`; the violations of `TableContentRefusal` | none | n/a |
| `platform` | `RequestRefused` (any call; its sentences are in `core/connect/connect-errors.ts`) | the rate limit of every API route | yes |

### The two checks

- **Go** (`backend/internal/archtest`, `errorreasons_*_test.go`, no database):
  - every value of the enum a listed detail carries is in the list, as sent or as unsent, and the list has no value the enum lacks;
  - every value listed as sent is used by some non-test Go file;
  - every `connect.NewErrorDetail` builds a message the list has;
  - every `reason` of an HTTP error body is an `rpcerr.Reason` constant, or a pass-through, or a listed value spelled in a place `unmigratedBodyReasons` names with its package;
  - the `rpcerr.Reason` constants, the violation constants and the redirect constants equal their parts of the list.
- **Web** (`web/src/app/core/errors/error-reasons.spec.ts`): every listed detail reason is named in the web code, every body reason has a sentence in `BODY_REASON_TEXT` and no other key does, every violation and redirect reason has a sentence of its own.
  A typed detail is checked by the reference to its enum value (a `case` or a map key), not by reading the sentence; `character-blocked-reasons.spec.ts` also walks the whole `CharacterBlockedReason` enum.

### Adding a reason

1. A detail reason: add the enum value to the `.proto`, run the generator, add the value to its `reasons` in `proto/error-reasons.json`, and add the sentence where the module's `*-errors.ts` words its other reasons (`RequestRefused`, which any call can carry, is worded in `core/connect/connect-errors.ts`).
   A new detail message goes in `details` with its enum field.
2. A body reason: add the `rpcerr.Reason` constant, its entry in `body`, and its key in `BODY_REASON_TEXT`.
   Set it with `reason: rpcerr.ReasonX`, never a literal.
3. A violation or redirect reason: add the constant, its entry in `violations` or `redirects`, and its sentence in the web.

The sentence says what happened and how to fix it, in the words the screen already uses, with no apology.
Both checks run in CI: `go test ./internal/archtest` and the web unit tests.

### What is not yet registered

The body reasons of `maps` and `play` are spelled where those modules define them, so `unmigratedBodyReasons` lists each by package and reason.
The value must still be in the list; only the spelling is excused, and an entry that excuses nothing fails the test, so moving a call site to `rpcerr` removes its entry.

## Idempotency keys

A call that creates something carries an `idempotency_key`, so a second tap, or a retry after a lost response, does not create a second resource (AIP-155).
One mechanism serves them all; the shared code is `platform/idem` (`Clean`, `Scope`, `Hash`, `Create`) and each module adds its columns and queries.

- **The key** is 1 to 64 characters, chosen by the app once per action and repeated on the retry.
  `ActionKey` (`web/src/app/core/connect/idempotency.ts`) keeps the same key while the values stay the same and makes a new one when they change or when the action succeeded.
- **The row stores** `create_key` (the key's owner, a colon and the key, so it is unique within the owner) and `create_hash` (the hash of the whole request minus the key), with a partial unique index on `create_key`.
- **Inside the create's transaction** (`db.InTx`, only `WithTx`), the server looks the key up.
  The same hash returns what the first call did and creates, publishes and counts nothing.
  Another hash is `invalid_argument` ("idempotency_key was used for another change").
  No row means create.
  Two simultaneous calls with the same key create one: the loser's `INSERT ... ON CONFLICT DO NOTHING` returns no row and it reads the winner's.
- **Where the key lives** differs by call (a column on the created row, the hash on a session event, on an XP award, on an image request...).
  `grep idempotency_key proto/` lists the calls; the `.proto` comment of each says what a repeat returns.
- **State changes that are not creates** keep the last key and request hash on the row they change, or, when no single row changes (`MoveMilestone` reorders several), in a small table of keys (`planned_milestone_moves`), or take the revision the caller read (`expected_revision`): a call on another revision is refused with `STALE_REVISION` and changes nothing.
- **Exempt on purpose:**
  - Calls that return a secret once (`CreateInvite`, `CreateClaimLink`): only the hash is stored, so a replay could not return the first secret.
    A retry makes a new link, and a new claim link revokes the live one.
  - Calls that only compute from a seed and store nothing (`GenerateTreasure`, `GenerateEncounter`).
  - Calls that are already idempotent by nature: `PlaceMapToken` (put or move), `TakeBackLeftImage`, `TakeOffStage`, `SetStageNpcHidden`, `SetSceneAudience`, `SetShownImageAudience`, `RollAbilityScores` (a second call returns the same dice).
- **The key is optional** on calls that existed without it; the app always sends one.
  Newer writes require it.

## Slow clients

The server has no global `ReadTimeout` or `WriteTimeout`, on purpose: they would cut the streams.
The limit sits on the operation that can hang (`platform/slowclient`):

| Operation | Deadline |
| --- | --- |
| Each `Send` of a live stream | 30 s to reach the client (`streamSendTimeout`). A reader that stopped reading ends the stream as `client_gone`. |
| The body of a unary RPC | 1 minute to arrive (`UnaryBodyTimeout`), cleared once read. A stream's request has none. |
| The body of an image upload | 2 minutes to arrive until the upload takes the processing slot, then 45 s for the file itself (`uploadSlotReadTimeout`): the slot is shared with fog tiles and other image jobs. |
| The `POST /auth/login` form | 10 s (`slowclient.ReadBody`). |
| The body of an image, thumbnail, fog tile or app file | 2 minutes to be written (`slowclient.WriteBody`, cleared when the handler ends). |
| A part of a campaign import | 2 minutes (`partReadTimeout`). |

Three routes read a body that a client can trickle and call `slowclient.ReadBody` before they read it: the image upload, the `POST /auth/login` form and a part of a campaign import. The rule for such a handler:

- Call `ReadBody` before any refusal (a missing session, a rate limit, a wrong media type), so a path that returns with the body unread keeps the deadline armed. The server has no `ReadTimeout`, and net/http discards what is left of an unread body (up to 256 KB, chunked or not) under the connection's deadline; with none, a client that keeps trickling holds the connection for the 35 minutes Cloud Run allows. The maps upload puts its `http.MaxBytesReader` first so that the deadline body stays the outermost one.
- Never turn the deadline off by hand: the body does it when it ends, and remembers that it ended.
- A second `ReadBody` on a body that has not ended gives the rest of it a new deadline (the upload does this once it holds the processing slot, so the slot's own, shorter deadline replaces the form's); on a body that has ended it does nothing. It protects the request: a deadline set after the body has ended would fire on net/http's background read, the one that notices a client that left, and cancel the request while the handler still decodes or stores what it read.

The other routes have no such deadline on their bodies; a unary RPC has the one in the table above.

`ReadHeaderTimeout` (10 s) and `IdleTimeout` (2 minutes) apply to every connection.
A live stream ends by itself after `play.DefaultMaxLifetime` (10 minutes) and the app opens a new one.

## Request size limits

A unary request has a size limit on its body as it is read and on each field after it is decoded.
The per-procedure body limit applies to the Connect protocol (the content types `application/json` and `application/proto`), which is what the app uses.
A stream's request, and a gRPC or gRPC-Web call, have the field limits and the 4 MiB backstop, not the per-procedure body limit.
Every limit has a default, so a field or a procedure that nobody annotated is still bounded.
The code is `platform/reqlimit`.

| Limit | Default | Where it is raised | The client sees |
| --- | --- | --- | --- |
| Items of a repeated field, entries of a map | 100 (`DefaultMaxItems`) | `(meurpg.options.v1.max_items)` on the field | `invalid_argument` with a `RequestRefused` detail, reason `LIST_TOO_LONG` |
| Bytes of a string (each element of a repeated string, each key and value of a map) | 8 KiB (`DefaultMaxString`) | `(meurpg.options.v1.max_len)` on the field | the same, reason `TEXT_TOO_LONG` |
| Bytes of a bytes field, counted the same way | 4 KiB (`DefaultMaxBytes`) | `(meurpg.options.v1.max_len)` on the field | the same, reason `TEXT_TOO_LONG` |
| Bytes of the body of a unary Connect RPC | 64 KiB (`DefaultReadLimit`) | an entry of `raisedReadLimits` in `platform/reqlimit/readlimit.go`, by procedure ([Procedures with a raised read limit](#procedures-with-a-raised-read-limit)) | `resource_exhausted`, from Connect |
| Bytes of any one RPC message | 4 MiB (`Backstop`, `connect.WithReadMaxBytes`) | never | `resource_exhausted`, from Connect |

The rule a reader follows:

- **The field limit** is the option of the field if it has one, else the default for its kind.
  `reqlimit.ItemsLimit` gives it for a repeated or map field and `reqlimit.LenLimit` for the bytes of a string or bytes value; `reqlimit.EffectiveLimit` gives the item count for a list or map, else the byte length.
  An option states the whole limit of that field, up or down; it does not add to the default.
  `max_items` counts the field's own items, `max_len` counts bytes of each string or bytes value in it.
- **The interceptor** (`reqlimit.Interceptor`, in `connectOptions` inside `rpclog`) walks the decoded request before the handler, for unary calls and for each message a stream receives.
  It refuses the first field over its limit, in the order the fields are declared in the `.proto`, and checks the size of a list or map before it looks at what is inside.
  The detail names the field by its path in proto names (`party.members.notes`) and never the limit, which is in the `.proto`.
- **The read limit** (`reqlimit.Middleware`, in front of `slowclient.Middleware`) wraps the body of a unary Connect RPC in `http.MaxBytesReader` at `ReadLimit(procedure)`.
  `raisedReadLimits` is the one place that lists the procedures allowed more than the default, each entry with a comment that says why.
  A stream's request, and a gRPC or gRPC-Web call, keep only the 4 MiB backstop.
- **A field option that goes over the default** needs an entry in `raisedReadLimits` for every procedure that reaches the field, and that entry never goes over the backstop.
- **The registry test** (`TestRegistry`, no database) walks every procedure of every `meurpg.*` service and fails when:
  - a procedure's read limit is not the default or its `raisedReadLimits` entry, or an entry names something that is not a procedure, does not raise the default or is over the backstop;
  - a list, map, string or bytes field of a request (nested messages and map values included) has a limit different from the one the test works out itself, the field's option or else the default of its kind, or a limit of 0;
  - a procedure reaches a field raised over its default and has no entry in `raisedReadLimits`.
  Another test fails if a default is changed to 0 or to another number without the test.
  It does not add up a worst case: whoever writes the option chooses the procedure's read limit on purpose, and the test makes sure they did.
- **The sentences** for `RequestRefused` and for a bare `resource_exhausted` are in `web/src/app/core/connect/connect-errors.ts`, so every screen shows them without wording them.

The options are custom (`proto/meurpg/options/v1/limits.proto`) instead of protovalidate.
The need is two numeric options (`max_items`, `max_len`) and a read limit per procedure, and protovalidate would bring a rules engine and a CEL runtime into the server, the generated code and the build for them.
The option is a few small functions that read a descriptor and walk a message, and the registry test reads the same options as the interceptor.

### Fields with a raised limit

A field has an option only where the app's own rule allows more than the default.
The limit of the field is the app's own cap, in bytes for a string (a character takes up to 4), and the `.proto` comment of the field cites where the cap is.

| Field | Limit | The app's own cap |
| --- | --- | --- |
| `known_spell_keys` of `FullSheet` and of `LevelUpChoices` | 200 items | `rules.MaxKnownSpells` (`rules/api.go`) |
| `FullSheet.known_issues` | 1000 items | `rules.MaxOverlayEntries` (`rules/overlay.go`); the server computes it again on every save |
| `FullSheet.content_baselines` | 1000 entries | `rules.MaxOverlayEntries`; the server clears it on every save |
| `FullSheet.custom_features_text` | 20 KiB | 5,000 characters (`maxCustomFeaturesLength`, `characters/sheet.go`) |
| `CharacterStory.backstory` | 40 KiB | 10,000 characters (`maxBackstoryLength`, `characters/sheet.go`) |
| `UpdateMasterNotesRequest.notes` | 80 KiB | 20,000 characters (`characters.MaxMasterNotesLength`) |
| `PaintMapCellsRequest.squares` | 400 items | `maxPaintSquares` (`maps/layers.go`) |
| `CreateMapPointRequest.hooks`, `UpdateMapPointRequest.hooks` | 16 KiB | 4,000 characters (`maxHooksLength`, `maps/mapservice.go`) |
| `UpdateCampaignDocumentRequest.body` | 400 KiB | `campaigns.MaxDocumentBytes`, 200 KiB after a line break `\r\n` becomes `\n`, so a document with `\r\n` line breaks may be up to 400 KiB (`MaxDocumentBytes` and `cleanDocument` in `campaigns/document.go`) |
| `SetOptionSwitchesRequest.switches` | 1400 items | `characters.MaxOptionSwitches` (`characters/tableswitches.go`) |
| `TableContentPack.entries` | 1000 items | `rules.MaxOverlayEntries` |
| `ImportTableContentRequest.pack_json` | 3 MiB | `characters.MaxPackBytes` (`characters/tablecontent_pack.go`); in proto JSON the bytes are base64, so a pack within about 50 bytes of 3 MiB is over the 4 MiB backstop and is refused as `resource_exhausted` first, and real packs are far below |
| `TableCasting.srd_spell_keys` | 400 items | `rules.MaxClassSRDSpells` (`rules/overlay_budget.go`) |
| `desc_pt` of a table feature, feat, option, class feature, subclass and spell, `TableSpell.higher_level_pt`, `TableEffect.text_pt`, `TableBackground.equipment_pt`, `TableSpellComponents.material_pt`, `TableCreatureTrait.text_pt` | 16 KiB for each text | 4,000 characters (`maxTextRunes` in `rules/overlay.go`, checked in `rules/overlay_budget.go`, `rules/overlay_spell.go` and `rules/overlay_creature.go`) |

The entries of a table (`TableEntry`) are in `ImportTableContent`, `CreateTableEntry` and `UpdateTableEntry`, so the rows for `TableCasting.srd_spell_keys` and for the table texts apply to all three.
Every other list in a request keeps the default of 100, and a string up to 2,000 characters fits the default of 8 KiB.

### Procedures with a raised read limit

| Procedure | Read limit | Why |
| --- | --- | --- |
| `ImportTableContent` | 4 MiB (the backstop) | `pack_json` is up to 3 MiB, and proto JSON writes bytes in base64 (4 MiB for a pack at the limit) |
| `UpdateCampaignDocument` | 512 KiB | `body` is up to 400 KiB; a document of `\n` line breaks (what the editor sends) is up to 200 KiB, or 400 KiB of proto JSON, and a document of only `\r\n` line breaks at 200 KiB is over the read limit and is refused with `resource_exhausted` |
| `CreateTableEntry`, `UpdateTableEntry` | 128 KiB | An entry's data is up to 64 KiB (`characters.MaxTableEntryBytes`), with texts of 16 KiB |
| `SetOptionSwitches` | 256 KiB | `switches` is up to 1400 entries |
| `CreateCharacter`, `UpdateCharacter`, `PreviewCharacter`, `PreviewChoices` | 128 KiB | A realistic full sheet with its lists and texts, with room for the echoed `known_issues` and `content_baselines` |
| `UpdateCharacterStory` | 128 KiB | `backstory` is up to 40 KiB |
| `UpdateMasterNotes` | 128 KiB | `notes` is up to 80 KiB |
| `LevelUpCharacter`, `PreviewLevelUp` | 128 KiB | The read limit must exceed the raised field's limit (registry rule); a realistic choice is far below it |
| `CreateMapPoint`, `UpdateMapPoint` | 128 KiB | The read limit must exceed the raised field's limit (registry rule); a realistic point is far below it |
| `PaintMapCells` | 128 KiB | The read limit must exceed the raised field's limit (registry rule); 400 squares are about 10 KiB |

Every other unary procedure has the default of 64 KiB.

### In tests

A module's test harness mounts its handlers with `reqlimit.HandlerOption()` (the protocol header, the field limits and the 4 MiB message backstop) and puts `reqlimit.Middleware` in front of its mux (`httpserver.Server.Handler` already does), so a test through a real handler is refused as in production.
`platform/reqlimit` tests the limits of the fields above without a database (`appfields_test.go`).

## Abuse limits

There are in-memory rate limits and per-account caps.
The rate limits stop a script; none hinders a real table.
All rate limits use `platform/ratelimit`, a token bucket with a per-client and a global limit.
`RATE_LIMIT_MULTIPLIER` scales all of them (default 1; the e2e stack uses 10, see [Operations](../operations/reference.md#abuse-limits)).
The numbers are in `ratelimit.NewPolicy` and in the constants next to each limiter.

| Limit | Key | Where | Answer |
| --- | --- | --- | --- |
| Every API request (`/meurpg.*`, `/images/`, `/uploads/`, `/session-messages/`) | Client IP | `ratelimit.Middleware`, before any session lookup | `429` with `Retry-After` and JSON `{"code":"resource_exhausted","reason":"RATE_LIMITED"}` |
| Connect calls and stream opens | Signed-in user | Interceptor right after the session one (`limitedSessions` in `cmd/api`) | `resource_exhausted` with `Retry-After` |
| Images, thumbnails and fog tiles (downloads) | User | `maps.Mount`, after the session | `429`, reason `RATE_LIMITED` |
| Fog tiles that need rendering | User | `maps/tiles.go`, stricter than the download limit | the same |
| Image uploads (`POST /uploads/images`) | User | `maps.Mount` | the same |
| Campaign package: start an export, begin, preview and create an import, download an export | User | `campaignpackage`, in each handler | `resource_exhausted` with `Retry-After` (`429` on the download) |
| Parts of an import upload | User | `campaignpackage` | `429`, reason `RATE_LIMITED` |
| Lines of the scene's conversation (`SendSessionMessage`) | User | `play`, first thing in the handler | `resource_exhausted` with `Retry-After` |
| Free dice rolls (`RollFreeDice`) | User | `play`, right after the membership check | `resource_exhausted` with `Retry-After` |
| The audio of a line (`GET /session-messages/{id}/audio`) | User | `play`, after the session | `429`, reason `RATE_LIMITED` |
| `/auth/login` and `/auth/callback` | Client IP | `identity`, first thing in each handler ([Identity](identity.md#login-attempt-limit)) | `429` with `Retry-After`, plain text |
| `PreviewClaim` and `ClaimCharacter` | User, and client IP | `characters`, first thing in each handler ([Campaigns](campaigns.md#claim-links)) | `resource_exhausted` with `Retry-After` |

- **The IP limit comes before the session** because a request with no session, or with an invented cookie, costs a database read to be refused.
  It counts everything on the API with or without a session, so its value is generous.
  The per-user limit holds back a member who repeats an expensive call.
- **Outside the limit:** the static app and `/healthz` touch no database.
  `/readyz` does, but is not limited by IP: Cloud Run's probes can share an address, and refusing a probe takes the instance out of service.
  It is bounded instead by sharing one database ping among all callers per 2 seconds (`pingCache` in `platform/httpserver`), so a change in database state shows up to 2 seconds late.
- **A call with no session** passes the per-user limit (the IP limit covered it and the handler refuses it).
  A stream spends one token on opening, not per message.
- **Each refusal logs one `WARN`** (`rate limit hit`) with the limit's name and the `user_id`, at most one per 10 s per limit, with the count of those left out.
  Never with the IP ([Privacy](../privacy.md)).
- **The client IP** comes from the connection (`RemoteAddr`) locally.
  On Cloud Run it is the item of `X-Forwarded-For` that `TRUSTED_PROXY_HOPS` counts from the right (1 on Cloud Run alone, 2 behind the load balancer): Google appends the address of whoever connected after what the client sent, so only what comes from the right is trustworthy.
  Setting the variable off Cloud Run is a configuration error.
- **The web app does not retry on its own** a refused call.
  `rateLimitInterceptor` (`web/src/app/core/connect/transport.ts`) turns a `resource_exhausted` with `Retry-After` into `unavailable` with "Muitas ações em pouco tempo. Espere N segundos e tente de novo.", which every screen treats as "try again", never as the final refusal of a full gallery (also `resource_exhausted`).
  The exceptions are images: a `<img>` cannot read `Retry-After`, so a fog tile answered `429` or `503` is asked again after 2, 4, 8, 16 and 30 s, and the map picture and gallery thumbnails do the same through the `appRetryImage` directive (`shared/retry-image`).
- **One instance only:** the counters live in process memory.
  With two instances each would have its own.

### What an account or a campaign may hold (RN-30)

| Cap | Value and where it is set | Refusal |
| --- | --- | --- |
| Campaigns an account is master of | `MAX_CAMPAIGNS_PER_USER`, default 10 (`campaigns.DefaultMaxCampaignsPerUser`). The local stacks and CI use `off`; the API refuses `off` on Cloud Run. | `resource_exhausted` with `CampaignCreationRefused` `LIMIT_REACHED` |
| Who may create campaigns | `CAMPAIGN_CREATORS`: verified e-mails, comma-separated; empty means anyone. Someone invited to a campaign plays without being on the list. | `permission_denied` with `NOT_ALLOWED` |
| Invites that still work in a campaign | 50 (`campaigns.MaxActiveInvites`): not revoked, not expired, with uses left | `resource_exhausted` |
| Characters and NPCs in a campaign, living or dead (combat-only NPCs not counted toward it) | 1,000 (`characters.DefaultMaxCharactersPerCampaign`) | `resource_exhausted` |
| Combat-only NPCs in a campaign (their own cap) | 1,000 (`characters.DefaultMaxCombatOnlyNpcsPerCampaign`) | `resource_exhausted` |
| Images generated per campaign per month, and per server per Brasília day | `IMAGE_MONTHLY_LIMIT` and `IMAGE_DAILY_LIMIT` ([Operations](../operations/reference.md#generated-images-the-gemini-api)) | `failed_precondition` with reason `DAILY_LIMIT_REACHED` for the daily cap |
| Voices made for the conversation, per server per Brasília day | `TTS_DAILY_LIMIT` ([Operations](../operations/reference.md)) | `429` on the audio route, reason `DAILY_LIMIT_REACHED` |

Each count runs in the transaction of the create it guards, with `WithTx(tx)`, so under `SERIALIZABLE` two creates racing for the last place conflict and the one that retries counts again.
The campaign cap reads verified e-mails through `identity` (`PostgresStore.VerifiedEmails`); `campaigns` never reads the users table.
The gallery, map and point limits belong to the `maps` module.
