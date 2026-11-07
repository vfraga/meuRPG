# Play module

The `play` module runs the game table: game sessions, the live session stream, combat, casting, reactions, scenes, traps and puzzles.
The code is `backend/internal/play`; the contracts are `proto/meurpg/play/v1/*.proto`.
This page is the map: what the module owns, its services, the seams to other modules, how a write works and how it keeps secrets.
Every subject has its own page, listed at the end.

## What it owns

| Area | Stories and rules | Page |
| --- | --- | --- |
| Game sessions, the live stream, the shown image | MR-028, RN-01, RN-06, RN-07 | [Live session](live-session.md) |
| HP, slots and hit dice; rests; class resources | RN-02 | [Vitals, rests and class resources](rests-and-resources.md) |
| Held d20 rolls (Lucky, Bardic Inspiration) | RN-18, RN-35 | [Held rolls](held-rolls.md) |
| RP scenes, clues, NPCs on stage | MR-015, MR-029, MR-030, MR-031 | [Scenes and stage](scenes-and-stage.md) |
| Combat: lifecycle, order, calls, visibility | MR-012, MR-013, MR-014 | [Combat](combat.md) |
| Attacks, damage, conditions, states, feature actions | MR-012, MR-014, RN-22, RN-35 | [Attacks and damage](attacks-and-damage.md) |
| Movement, cover, opportunity attacks, area spells | MR-034, RN-21 | [Movement](movement.md) |
| The areas spells keep on the map | RN-37 | [Spell zones](spell-zones.md) |
| Casting in combat and outside it | MR-014, MR-048, RN-32 | [Casting](casting.md) |
| Reaction windows | PM-04 | [Reactions](reactions.md) |
| Death saves and bringing the dead back | RN-03 | [Death and revival](death-and-revival.md) |
| Grapple, shove, hide, help, group check, surprise | RN-34 | [Contests](contests.md) |
| Lasting effects and exhaustion | RN-22 | [Lasting effects](lasting-effects.md) |
| Table rules inside combat | MR-025, RN-24 | [Table rules in combat](table-rules.md) |
| Combat without a grid | MR-025, RN-25 | [Theatre of the mind](theatre.md) |
| Undo, the combat log, highlights, session summary | MR-032, ADR-0007 | [Undo](undo.md), [Combat log and events](combat-log.md), [Highlights and summary](highlights-and-summary.md) |
| Per-player combat under fog | MR-036, RN-10 | [Combat under fog](fog.md) |
| Traps in play | MR-035 | [Traps](traps.md) |
| Character creatures, Wild Shape, the familiar's eyes | MR-037 | [Creatures](creatures.md) |
| Monsters in combat, the encounter builder | MR-042, MR-043 | [Encounters](encounters.md) |
| Puzzles | MR-038, RN-27 | [Puzzles](puzzles.md) |
| The free dice roller | MR-052, RN-10, RN-18 | [The free dice roller](free-dice-roller.md) |
| The TV at the table | RN-10, RN-36 | [The table screen](table-screen.md) |

## Services

`play.Service` implements several Connect services, mounted together with the same interceptors.
A service is separate when its contract grows large enough to deserve its own `.proto`.

| Service | Proto | Covers |
| --- | --- | --- |
| `PlayService` | `play.proto` | Sessions, the stream, vitals, current map and shown image, scenes, stage, wild shape, familiar sight, traps, summary |
| `CombatService` | `combat.proto` | Encounters, turns, attacks, damage, casting in combat, reactions, undo, the log |
| `ResourceService` | `resources.proto` | Rests, hit dice, class resource flows, the answers to held rolls |
| `ContestService` | `contests.proto`, `contest_types.proto` | Grapple, shove, hide, help, group check, surprise |
| `RevivifyService` | `revivify.proto` | Revivify requests outside combat (see [Death and revival](death-and-revival.md)) |
| `LastingEffectService` | `lasting_effects_service.proto` | Effects that last, exhaustion |
| `CastingService` | `casting.proto` | Spells cast outside combat |
| `EncounterService` | `encounters.proto` | The encounter builder and stored battle encounters |
| `PuzzleService` | `puzzles.proto` | Puzzles |
| `TableScreenService` | `table_screen.proto` | The master's choice for the TV at the table, what the TV shows and its stream (see [The table screen](table-screen.md)) |

### `PlayService` calls

| Call | Who may | Own errors |
| --- | --- | --- |
| `StartGameSession` | The campaign master | `failed_precondition` `SESSION_ALREADY_OPEN` |
| `EndGameSession` | The campaign master | `not_found` for another campaign's session; ending again returns the session as it is |
| `ListGameSessions` | Campaign members | none |
| `ListOpenGameSessions` | Anyone signed in; each sees only the campaigns they are an active member of | none |
| `GetLiveSession`, `WatchGameSession` | Campaign members | `failed_precondition` `NO_OPEN_SESSION` |
| `AdjustCharacterVitals` | The master, session open | `NO_OPEN_SESSION`; `not_found` unless the character is a living player character of the campaign; `invalid_argument` outside 0 to the maximum. `aborted` when `expected_revision` is absent or no longer the vitals' revision. The response carries `concentration_dc` and `concentration_cast_id` when a character concentrating on a spell cast outside combat lost hit points. At 0 HP the concentration of a character in the open combat ends too |
| `NudgePlayer` | The master, session open | `NO_OPEN_SESSION`; `not_found`; `resource_exhausted` for a second nudge to the same character within 10 s |
| `SetCurrentMap`, `SetShownImage`, `SetShownImageAudience` | The master, session open | `NO_OPEN_SESSION`; `not_found` for a map or image of another campaign (or no image shown); `invalid_argument` for an audience that is malformed; `SceneBlocked` `SCENE_NOT_PARTIAL` for the scene's players with no scene open to only some |
| `OpenScene`, `SetSceneAudience`, `CloseScene`, `GetOpenScene`, `RollSceneCheck`, `GrantSceneAttempt` | See [Scenes and stage](scenes-and-stage.md) | `SceneBlocked` reasons |
| `PutOnStage`, `TakeOffStage`, `SetStageNpcHidden`, `SetSpeaker` | The master, session open | `SceneBlocked` `NO_OPEN_SCENE`, `STAGE_FULL` |
| `ListLeftImages`, `TakeBackLeftImage` | Members read; the master takes back | `not_found` |
| `GetSessionSummary` | Campaign members | `GameSessionBlocked` `SESSION_NOT_ENDED` for an open session |
| `RollFreeDice`, `ListFreeRolls` | Campaign members; `secret` only the master | See [The free dice roller](free-dice-roller.md) |

A pending member (RN-15) gets `not_found` on everything that names a campaign, like a non-member.
`TestAuthorizationMatrix` has a column for them.
The combat, casting, wild shape, familiar sight, trap and puzzle calls are described on their own pages.

## Seams to other modules

`play` never imports another module.
It declares small interfaces; `cmd/api` wires an implementation into each.
The plain types that cross are in `play/link`, a package that imports nothing from the project.

| `play` interface | Implemented by | What for |
| --- | --- | --- |
| `SheetLocker` | `characters.Service` | `LockSheets`: lock the living draft sheets in the transaction that opens a session (RN-01). Called only by `play`, after it checked the caller is the master |
| `VitalsKeeper` (embeds `RestKeeper`) | `characters.Service` | Read and change `character_vitals`: HP, slots, uses, hit dice, exhaustion, the maximum-HP bonus |
| `CombatRoster` | `characters.Service` | Who can fight; sheets, attacks, turn options, spells as the caster casts them, save bonuses, death confirmation, damage modifiers, scene options, party levels |
| `CombatDefaults`, `DiceModes` | `campaigns.Service` adapters | The table rules and the campaign's dice mode (RN-18, RN-24) |
| `MapKeeper` | `maps.SessionMaps` | The current map and its reveal, the shown image, images left with players, grid, tokens, battle points, scene points, treasure found in a session |
| `TerrainSource` | `maps.Service`, set with `SetTerrain` | The terrain (walls, difficult terrain, cover, doors) of the combat's grid |
| `FogSource` | `maps.Service`, set with `SetFog` | What the players see now of the combat's map |
| `TrapBook` | `maps.Service`, set with `SetTraps` | The map's traps, noticing, searching, triggering |

The other direction (`maps` calls `play`) goes through interfaces `maps` declares and `play.Service` implements: `LiveSession` (`OnScreen`, `OpenScenePoint`, `ImageOnStage`, `ImageShownTo`, `Publish`, `PublishToScene`, `PublishToUsers`, `AppendEvent`) and `TrapFirer`.
Because each side only needs the other's interface, `cmd/api` creates `maps.SessionMaps` first, then `play` with it, then `maps` with `play`.
`characters` reaches `play` through interfaces it declares and `cmd/api` sets: `SetLive`, `SetCreatureHost` and `SetReviewHost`.

## How a write works

A write locks, checks, changes, records and only then tells.

1.
   The call checks the caller (authz) and, for a master action, that the session is open.
2.
   One `db.InTx` locks the open session's row (`FOR UPDATE`), so two changes wait for each other.
3.
   It checks the idempotency key in `session_events`: a repeated key changes nothing, stores nothing and answers with the state as it is now; the same key with another request is `invalid_argument`.
4.
   It reads the other modules through the interfaces above, passing the transaction (`WithTx`; never the pool inside `InTx`, see [Architecture](../transactions.md#transactions-and-the-connection-pool)).
5.
   It writes the rows and one `session_events` row, with ids and numbers only (ADR-0007).
6.
   After `COMMIT` it publishes to the stream, on a context that outlives the call (`context.WithoutCancel`), so a client that hangs up never leaves the table without the hint.

Each event's `seq` is the session's next number, read with the row locked: no gap, no repeat.
`EndGameSession` locks the same row and ends an open combat in the same transaction.
Combat writes share one helper, `combatTx` (`combat_write.go`).
An event payload is at most 16 KiB of database text (a `CHECK` on `session_events`); an event that lists one record per creature is split past `eventPayloadBudget` (3,500 bytes), and one over `undoHelpersBudget` drops the lists only its undo needs (see [Undo](undo.md)).

## Privacy: who sees what

The server filters; the browser only draws what arrives (RN-10, RN-20).
What a player may not see stays out of the response, never blank or zero.

- Each stream event carries its own audience (everyone, the master, or one user), and the hub delivers only to the subscriptions in it.
- Most events are contentless hints (`encounter_changed`, `scene_changed`, `puzzle_changed`): the app reads the state again, already filtered.
- Armor class, an NPC's numbers, the DC of a check, a hidden combatant, a puzzle's answer and a clue's text never reach a player who may not have them.
- The leak tests (`internal/leaktest`) read every response and stream event as every person against a campaign with one hidden thing of each kind; a new RPC must be classified or the test fails (see [Architecture](../../testing.md#leak-tests-rn-10)).
- A test that proves something absent looks for a unique needle and has a positive control (see [Contributing](../../testing.md)).

## Pages

[Live session](live-session.md) · [Vitals, rests and class resources](rests-and-resources.md) · [Held rolls](held-rolls.md) · [Scenes and stage](scenes-and-stage.md) · [Combat](combat.md) · [Attacks and damage](attacks-and-damage.md) · [Movement](movement.md) · [Spell zones](spell-zones.md) · [Casting](casting.md) · [Reactions](reactions.md) · [Death and revival](death-and-revival.md) · [Contests](contests.md) · [Lasting effects](lasting-effects.md) · [Table rules in combat](table-rules.md) · [Theatre of the mind](theatre.md) · [Undo](undo.md) · [Combat log and events](combat-log.md) · [Highlights and summary](highlights-and-summary.md) · [Combat under fog](fog.md) · [Traps](traps.md) · [Creatures](creatures.md) · [Encounters](encounters.md) · [Puzzles](puzzles.md) · [The free dice roller](free-dice-roller.md) · [The table screen](table-screen.md)
