# Review unit 2: live session, stream, scenes, stage, puzzles, traps

- Scope: `backend/internal/play` (play.go, rpc.go, live.go, live/hub.go, link, content_hint.go, mapseam.go, vitals.go, xp.go, summary.go, highlights.go, scene.go, stage.go, onscreen.go, puzzles*.go, traps*.go).
- Commit reviewed: `ae40f74b648bf653560d54e7ab1b3128a5370f8c` (main).
- Time spent: about 1 hour 30 minutes of reading plus about 10 minutes of parallel verification.
- Model: reviewer ran on Sonnet 5.5 (`claude-sonnet-5-5`); the verification subagents also ran on `sonnet`.
- Test runs are slow (100-160 s each) because eight verifiers shared one in-memory CockroachDB.

## Findings

| id | severity | status | where | defect | failure scenario | evidence |
| --- | --- | --- | --- | --- | --- | --- |
| U2-3 | medium | confirmed | `scene.go:526` (`tallyAttempts`), `RollSceneCheck`; `summary.go:216`; query `ListSessionSceneEvents` | A scene action with `max_attempts` 0 (unlimited) has no cap, each roll adds a `session_events` row, and the summary refuses more than 20000 scene events. | One player rolls an unlimited action in a loop (rate limit is 40 calls/s, so about 8 minutes). The ended session's `GetSessionSummary` then returns `internal` to everyone, forever. Meanwhile every `RollSceneCheck` and `GetOpenScene` re-reads all rolls of the opening (O(n)): 3.8 s and 7.3 s at 20004 rolls (with `-race`). | `TestReview2_SceneRollVolumeBreaksSummary` (`internal/play/review02_scenevolume_test.go`). `go test -race -count=1 -run TestReview2_SceneRollVolumeBreaksSummary ./internal/play/` gives `master: GetSessionSummary() code = internal ... want the summary`. |
| U2-1 | medium | confirmed | `queries.sql:694` (`ListTrapEventsOfSession`), `traps_activity.go:84` | The query is `ORDER BY seq LIMIT 500`, so it keeps the oldest 500 trap events. Newer firings, searches and notices vanish from `ListTrapActivity` for the master and the player. | `SearchForTraps` has no limit outside combat, so one player (505 calls, fresh keys) fills the list; the master then fires a trap by hand and the activity list shows 500 lines, 0 firings. The combat log does the opposite on purpose (keeps the newest). | `TestReview2_TrapActivityTruncation` (`review02_trapactivity_test.go`). Output: `master: 500 lines, 0 firings; want the newest event (the firing) listed` (same for the player). |
| U2-6 | medium | confirmed | `onscreen.go:110` (`SetShownImage` calls `MapKeeper.ImageToShow` before locking the session); `maps/fogimage.go:146` (`ownImage`) | For an image that is a fog map's background, every call makes a new gallery copy (file plus quota). It happens outside the transaction and before the session check. | (1) With no open session, each refused call returns `NO_OPEN_SESSION` but leaves an orphan copy (gallery 1 to 3 after two calls). (2) Calling again with the same image to turn "keep" on (documented as sending no event) makes another copy, so players receive `shown_image_changed` with a different image id and the gallery grows (4 to 5). | `TestReview2_ShownFogImageCopies` (`internal/maps/review2_shownfogimage_test.go`). Output: `gallery grew from 1 to 3 images after refused calls`; `the player received shown_image_changed ... on the keep call, want nothing`. |
| U2-4 | low | confirmed (claim 1); unverified (claim 2) | `traps_damage.go:129` (`settleTrapDamage`) | Applying a trap damage changes a combatant's vitals but does not touch the running encounter or publish `encounter_changed` (`AdjustCharacterVitals` does both). | A trap fired outside combat leaves a `trap_damages` row; a combat then starts with that character; the master applies the damage and the character goes to 0 HP. The encounter revision stays at 21 and the player's stream gets no `encounter_changed`, so the "Caído" state shown by other screens is stale until another combat change. | `TestReview2_TrapDamageDoesNotTouchEncounter` (`review02_trapdamage_test.go`). Output: `encounter revision after = 21, before = 21; want it raised`; `no encounter_changed event reached the player's stream`. Claim 2 is in the fix direction. |
| U2-2 | low | confirmed | `traps.go:183-190` (`SearchForTraps`, replay branch) | The replay checks only `done.Kind`, not the actor. Every other player call (`RollSceneCheck`, `MakePuzzleMove`, `TryPuzzleHint`, `AdjustCharacterVitals`) refuses a key used by someone else. | Player B sends the idempotency key player A used for a successful search and gets A's roll and `found_point_ids` (a trap id B's character never found). Nothing is stored or revealed on the map. Needs B to know A's random UUID key, hence low. | `TestReview2_SearchKeyReplayAcrossUsers` (`review02_searchkey_test.go`). Output: `B reusing A's key got no error; response = roll:{... total:24} found_point_ids:"..."`; `B received trap id ... that B's character never found`. |
| U2-5 | low | refuted | `rpc.go:91` | Concurrent `StartGameSession` with one key could answer `SESSION_ALREADY_OPEN`. | 100 rounds of 3 to 8 racers: always one session, all calls succeeded with the same id. `InsertGameSession` is `ON CONFLICT (create_key) DO NOTHING` and `db.InTx` retries 40001. Existing `TestStartGameSessionWithTheSameKeyAtOnceStartsOne` covers it. | No test kept. |
| U2-7 | medium | refuted | `live/hub.go`, `puzzles.go:142` (`puzzleGate`) | Hub drops, duplicates or reorders events; `Coalesce` key stuck; puzzle gate loses a hint. | Publish, remove and Taken all run under one mutex; `Taken` is called on every read path in `WatchGameSession`; a gate race can only duplicate a hint in a later interval, never lose one. 16 goroutines x 500 publishes under `-race` showed nothing. The web app also applies `vitals_changed` only if its revision is newer, so commit-vs-publish reordering is harmless. | `go test -race -run TestReview2_ ./internal/play/live/` ok (temporary test deleted); gate by code reading only. |
| U2-8 | low | refuted (documented) | `live.go:270` | A removed member's open stream keeps receiving events. | It ends with `not_found` at the next Recheck (60 s in production; 1 s in the test). In the window they got `current_map_changed`, `shown_image_changed` and their own `vitals_changed`, nothing a member would not get. A removed player cannot reopen a stream. This is the documented design. No RPC removes an active member yet. | Temporary test deleted. |

Also checked with no finding: every write in the unit is inside `db.InTx` with a resettable closure and no pool read inside a closure; every handler resolves ids through the caller's campaign (no cross-campaign id accepted); `playerRunProto` shows no answer, hidden hint or other player's part; ending a session leaves no player write open (all take the session row lock).

Observations, not counted as findings (no spec to test against): `SearchForTraps` outside combat can be repeated without limit, so a failed search can be re-rolled until it passes; `ReleaseNextPuzzleHint`, `ResetPuzzle` and `PlayPuzzleSequence` have no idempotency key, so a client retry releases a second hint or resets progress.

## Fix direction

### U2-3 unbounded scene rolls
- Root cause: nothing bounds the rows a member can append, and the readers load all of them.
- Fix: cap total rolls per character and action per opening in `RollSceneCheck` (e.g. refuse past a few hundred with `ALREADY_ROLLED`), and make the summary count in SQL (`count(*) filter`, grouped by character) instead of loading rows. Alternative: only raise the limit. The SQL aggregate is better because it removes the failure mode.
- Same pattern: `ListSceneRollEvents` in `sceneInfo`/`sceneRolls`/`tallyAttempts` (`scene.go:363-372, 459, 526`), `ListEncounterCombatEvents` `LIMIT 20000` in `summary.go:123` (truncates silently), `SearchForTraps` and `TryPuzzleHint` event growth.
- Rules: keep `ALREADY_ROLLED` semantics in `docs/architecture.md` "RP scenes"; roll closures stay idempotent; update the `maxSummaryEvents` comment.
- Risk: attempt counts change meaning for unlimited actions. Guarded by `TestMR015_AttemptsPerAction`, `TestMR015_OneMoreAttempt`, `summary_test.go`.

### U2-1 trap activity truncation
- Root cause: the query orders ascending and limits, so the cut falls on the newest rows.
- Fix: `ORDER BY seq DESC LIMIT N`, reversed by the caller, as `ListEncounterEvents` does. Do the player filter in SQL (character ids) so one player's spam does not hide others' lines. Optionally also rate-limit searches per character.
- Same pattern: `queries.sql:433/440` are already newest-first; `ListSessionSceneEvents` is ascending with a fail-loud limit (U2-3). The extension merge in `traps_activity.go:95` needs the host line in the window.
- Rules: none beyond RN-10 (player sees only own lines).
- Risk: line order and the `ExtendsID` merge. Guarded by `TestMR035_TheActivityOutsideACombat`.

### U2-6 fog image copies
- Root cause: the copy is made by `ImageToShow` before the session lock and on every call, and `changed` compares copy ids.
- Fix: (a) check the open session first, and reuse the copy already shown when the same source image is shown again (remember the source id on the session row or look the copy up by source); or (b) make the copy inside `SetShownImage`'s transaction. (a) is simpler and fixes the keep toggle. The maps module must expose a "copy once per source" lookup.
- Same pattern: `ownImage` also serves NPC portraits (`stage`, `characters` portrait) and `LeaveImage` of the shown copy at `rpc.go:158`.
- Rules: gallery quota (300 images / 500 MB), RN-10 for fog images (the master's image is never shown as is), "Images left with the players" docs.
- Risk: leaving and showing a fog image twice. Guarded by `TestRN10_FogMapImageIsCopiedWhenShared`, `TestMR028_MasterLeavesAnImageWithThePlayers`.

### U2-4 trap damage and the running combat
- Root cause: `settleTrapDamage` is a second path to vitals that skips the combat bookkeeping `AdjustCharacterVitals` does.
- Fix: share one helper (vitals change plus `TouchEncounter` plus `publishEncounterChanged` when the character is a combatant of the open encounter) between the two. Pass the open session's id, not `row.GameSessionID`, to `changeVitals` so the death-save marks hit the running combat (claim 2, read from code, unverified: it would need a damage fired in an earlier session and applied in a later combat).
- Same pattern: any other writer of `character_vitals` via `changeVitals` outside `combatTx` (`vitals.go:82`, `traps_damage.go:197`).
- Rules: one transaction, no pool reads; RN-02 and RN-03.
- Risk: encounter revision bumps. Guarded by `TestAdjustingVitalsInACombatRaisesItsRevision`, `TestMR035_AnEndedCombatKeepsTheTrapDamageForTheMaster`.

### U2-2 search key replay
- Root cause: missing actor check in the replay branch.
- Fix: in `traps.go:186` also require `done.ActorUserID == m.UserID` (and the character), else `invalid_argument` "idempotency_key was already used for another change".
- Same pattern: `FireTrap` replay in `traps_fire.go:387` (master only) does not compare the trap or targets either; `AdjustCharacterVitals` compares only the character.
- Rules: error codes in `docs/architecture.md`; RN-10.
- Risk: none beyond the new refusal; `TestMR035_AKeyReusedOnAnotherTrapDamageIsRefused` is the model.

Sketch (U2-1): `ORDER BY seq DESC LIMIT 500` in `ListTrapEventsOfSession`, then `slices.Reverse(rows)` in `ListTrapActivity`.

## Tests added

- `backend/internal/play/review02_scenevolume_test.go`: `TestReview2_SceneRollVolumeBreaksSummary` (U2-3)
- `backend/internal/play/review02_trapactivity_test.go`: `TestReview2_TrapActivityTruncation` (U2-1)
- `backend/internal/maps/review2_shownfogimage_test.go`: `TestReview2_ShownFogImageCopies` (U2-6)
- `backend/internal/play/review02_trapdamage_test.go`: `TestReview2_TrapDamageDoesNotTouchEncounter` (U2-4)
- `backend/internal/play/review02_searchkey_test.go`: `TestReview2_SearchKeyReplayAcrossUsers` (U2-2)

All five fail on the reviewed commit, by design.
