# Combat

A combat (in the code, an *encounter*) lives inside the open session: the master picks who fights, everyone rolls initiative, and turns rotate until the master ends it (MR-013).
The contract is `CombatService` (`proto/meurpg/play/v1/combat.proto`); its events travel on the session stream.
It is a separate service from `PlayService` because combat keeps growing; both are implemented by `play.Service`.

This page covers the lifecycle, the calls, the turn order, the joint turn and what each side sees.
Attacks and damage are in [Attacks and damage](attacks-and-damage.md), movement in [Movement](movement.md), spells in [Casting](casting.md).

## Where the code is

| File | Holds |
| --- | --- |
| `combat.go` | The lifecycle calls |
| `combat_write.go` | The transaction every change shares (`combatTx`, `write`, `finish`) |
| `combat_view.go` | Who sees what |
| `combat_rules.go` | The arithmetic, pure and tested without a database |
| `combat_actions.go`, `combat_parts.go` | Options, attacks, damage, standard actions, NPC HP |
| `combat_turn.go` | The joint turn |
| `combat_events.go`, `combat_log.go` | Event payloads and the log |
| `combat_vitals.go` | Slots, uses and HP through `character_vitals` |
| `combat_undo.go`, `combat_undo_kinds.go` | Undo |

Pure rules live in `rules/combat`.

## Lifecycle

```mermaid
stateDiagram-v2
    [*] --> setup: StartEncounter
    setup --> active: BeginCombat (everyone has initiative)
    active --> active: EndTurn
    setup --> ended: EndEncounter
    active --> ended: EndEncounter
    setup --> ended: the session ends
    active --> ended: the session ends
    ended --> [*]
```

A partial unique index (`encounters_one_open_per_session`) guarantees one open combat per session even when two calls race.
Ending the session ends the combat in the same transaction.
The mode (`grid` or `theatre`) is chosen at the start and never changes (see [Theatre of the mind](theatre.md)).

## Calls

### Setup and turns

| Call | Who may | What it does |
| --- | --- | --- |
| `StartEncounter` | Master, session open | Creates the combat in `setup`, on the current map (which needs a grid), on a battle point's map, or with none in theatre. Puts the group and NPC copies; each NPC rolls initiative at once. With `monsters`, also puts bestiary monsters in the same transaction (see [Encounters](encounters.md)) |
| `GetEncounter` | Members, filtered | The session's latest combat, including one that just ended, or none |
| `SubmitInitiative` | The player on their own character; the master on any | d20 rolled in the app (`roll_in_app`) or the typed physical d20 (`d20_face`), under RN-18. Only in `setup`. A character creature's roll sets its whole summon group |
| `SetInitiativeOrder` | Master | Orders a tie (same total and bonus) |
| `BeginCombat` | Master | `active`, round 1, the first in the order on turn. Without someone's initiative: `INITIATIVE_MISSING` with who is missing |
| `EndTurn` | The member's player; the master (any) | Ends one member's part of the turn; when the last acting member ends, the turn passes to the next group not defeated and, after the last, the round rises. `aborted` if `expected_combatant_id` is no longer a member who acts. Refused with `PENDING_DAMAGE` (damage to roll, to apply, or a reaction of the combatant on turn pending) unless the master passes anyway (`discard_pending_damage`); `DEATH_SAVE_DUE` for a player with a death save to roll (the master passes anyway); `REACTION_PENDING`, `OPPORTUNITY_PENDING`, `HIDDEN_REVEAL_PENDING` while those wait. If nobody is on turn (the combatant left or its character was deleted) only the master calls, and the turn restarts from the first in the order in the same round |
| `EndEncounter` | Master | `ended`; the players' tokens go where they stopped. Ending again is not an error |
| `AddCombatants` | Master | Reinforcements (NPCs), initiative rolled at once |
| `AddMonsters` | Master | "Pôr no combate" (see [Encounters](encounters.md)) |
| `RemoveCombatant` | Master | Takes someone out; if it was their turn, the turn passes. Refused (`PENDING_DAMAGE`) while a damage the combatant attacked with or took is still to roll or apply. A creature leaving because its owner or the master dismissed it cannot be refused: the damage waiting on it is written as dropped (`damage_discarded`, before the `combatant_removed`) |
| `SetCombatantSide` | Master | "Aliado" or enemy for an NPC (`side_set`); passes over opportunity offers now between allies |
| `SetCombatantHidden` | Master | Hides or shows an NPC |
| `SetCombatantCover` | Master | Cover by hand (none, half, three-quarters, total); cleared when the combatant moves |
| `SetCombatantConditions` | Master; the player only to end their own concentration | See [Attacks and damage](attacks-and-damage.md#conditions-and-concentration) |

### Acting

| Call | Who may | What it does |
| --- | --- | --- |
| `GetTurnOptions` | The player on their own character; the master on any | What the combatant can do now, with targets, distances, modes and the damage still to roll; answers off turn with everything disabled and the reason |
| `RollAttack`, `RollDamage`, `ApplyPendingDamage`, `DiscardPendingDamage`, `RemoveDamagePart` | See [Attacks and damage](attacks-and-damage.md) | The two-step attack |
| `RequestRollMode`, `AnswerRollModeRequest`, `CancelRollModeRequest` | Player asks; master answers | A mode the server did not suggest |
| `CastSpell`, `PreviewSpellArea`, `ResolveHiddenReveal`, `EndConcentration` | See [Casting](casting.md) | Spells |
| `TakeAction` | The player on their own character, on their turn; the master on any combatant on turn | Standard actions and feature actions |
| `AnswerRageEnd`, `EndRage`, `UseHitRider` | The barbarian's player / monk's player; master | See [Attacks and damage](attacks-and-damage.md) |
| `MoveCombatant`, `GetMoveOptions`, `SpendMovement`, `StandUp` | See [Movement](movement.md) and [Theatre](theatre.md) | Movement |
| `OfferOpportunity`, `WithdrawOpportunity`, `DeclineOpportunity`, `SkipOpportunity` | See [Movement](movement.md) | Opportunity attacks |
| `AnswerReaction`, `MarkReactionRolling`, `ResolveConcentrationSave`, `UseReaction`, `DeclineReaction` | See [Reactions](reactions.md) | Reaction windows |
| `RollDeathSave`, `ConfirmDeath` | See [Death and revival](death-and-revival.md) | The fallen |
| `AdjustCombatantHitPoints` | Master | "Dano/Cura" of an NPC: damage, healing or an exact value, and temporary HP |
| `EndCombatEffect` | Master | Ends an effect such as Aid (idempotent by key, not undoable) |
| `UndoLastAction` | Master | See [Undo](undo.md) |
| `ListCombatLog`, `GetCombatHighlights` | Members, filtered | See [Combat log and events](combat-log.md), [Highlights](highlights-and-summary.md) |

State errors are `failed_precondition` with the `EncounterBlocked` detail and a reason: `MAP_HAS_NO_GRID` (with the `map_id`), `NO_CURRENT_MAP`, `ENCOUNTER_ALREADY_OPEN`, `NOT_IN_SETUP`, `NOT_ACTIVE`, `NOT_YOUR_TURN`, `NOT_PLACED`, `TOO_FAR` (with `missing_ft`), `SQUARE_OCCUPIED`, `WALL_ON_SQUARE`, `NO_FREE_SQUARE`, `TOO_MANY_COMBATANTS`, and in actions `ACTION_USED`, `BONUS_ACTION_USED`, `REACTION_USED`, `ATTACKS_USED`, `ATTACK_ACTION_FIRST`, `NO_SLOT`, `NO_USES`, `TARGET_OUT_OF_REACH`, `TARGET_DEFEATED`, `TARGET_DEAD`, `COMBATANT_DOWN`, `PENDING_DAMAGE`, `REACTION_PENDING`, `DEATH_SAVE_DUE`, `DAMAGE_ALREADY_ROLLED`, `DAMAGE_NOT_ROLLED`, `NOTHING_TO_UNDO`, `WRONG_DICE_MODE` and more (the enum in `combat.proto` is the list).
Callers never depend on message text; the app turns the code and detail into Portuguese (`combat-errors.ts`).

## How `play` reads the other modules

| Interface | Implemented by | What for |
| --- | --- | --- |
| `CombatRoster` | `characters.Service` | Who can fight, with initiative bonus, speed and maximum HP (`rules.Derive` for a full sheet; as it stands for a basic one). A character's AC, attacks, standard actions and feature actions (`link.Sheet`); `TurnOptions` with the slots and uses already spent; the spell as the caster casts it (`link.Spell`); a target's save bonus (a basic sheet has none: `Known` false); `MarkDead`, `ReviveDead`, `DamageModifiers`, `SceneOptions`, `PartyLevels`, `MonsterNpc`, `CreatureEyes` |
| `DiceModes` (`ForcedDice`) | Adapter over `campaigns.Service.CampaignDiceMode` | What the campaign forces: nothing, everything in the app, or everything physical (RN-18). The player's preference only sets the default the screen highlights |
| `MapKeeper` (and `MapGrid`, `BattlePoint`, `MapTokens`, `SetTokenPositions`) | `maps.SessionMaps` | The grid, a battle point's map, token positions, and moving the players' tokens at the end |
| `CombatDefaults` | `campaigns.Service` adapter | The table rules (`StoredTableRules`, `CombatWithoutMap`) |

A basic sheet becomes a minimal `rules.Derived` (structured attacks as `basic:0`, `basic:1`, and the ten standard actions), so the engine serves the Goblin as it serves a full sheet.

## The grid and the order

The map has `grid_columns` (4 to 200 squares of 1.5 m across the image's width); rows follow the image's proportion (`round(columns × height / width)`, 1 to 400) and are not stored.
The combat copies the grid when it starts (`encounters.grid_columns` and `grid_rows`), so changing the map's grid afterwards touches nobody.
Each combatant starts on the square of the character's token if it has one (of several copies of an NPC, the first), otherwise without a position: the master places it.
Movement is in [Movement](movement.md).

**The order (RN-19).** Highest total first; on a tie, highest bonus; on a tie of both, the order the master decided (`SetInitiativeOrder`), otherwise the order of entry.
Whoever has not rolled goes last.
The server stores each position (`order_index`) and rebuilds the order on every roll or reinforcement.
A player rolls their own initiative once; the master can correct any value.
Each NPC rolls its own on the server with `crypto/rand`; the physical die is validated by `platform/dice`.

## Joint turn

A turn belongs to a group (MR-013, RN-19): combatants side by side in the order with the same initiative total, players and NPCs, form a group (`groupRuns`, `combat_turn.go`); someone alone on their total is a group of one.
The group is not stored: it is computed from the order and totals when the turn reaches it (`nextTurnGroup`), and `combatants.turn_state` stores the answer (`idle`, `acting`, `ended`) until the turn passes.
Reordering a tie, bringing in a reinforcement or hiding a member mid-turn does not change a turn that began.

```mermaid
stateDiagram-v2
    [*] --> acting: the group's turn begins (each living member)
    acting --> ended: member's EndTurn (player or master)
    ended --> [*]: the last one ends, the turn passes to the next group
    acting --> [*]: the member leaves, dies or is removed
```

- **Who still acts.** `othersActing` counts members whose part has not ended and who are not defeated; a member that dies or is taken to 0 HP (an NPC) never holds the group, and the current member is the first living one (`turnFor`).
- **Start of the turn.** `startTurn` ends what ends at the start of effects, resets for each living member the movement, action, bonus action, Dash, reaction, Shield bonus and the death save due (`ResetCombatantTurn`), and points `current_combatant_id` at the first.
- **"Is it their turn"** is always `actsNow` (in the turn and the part not ended): `MoveCombatant`, `RollAttack`, `RollDamage`, `TakeAction`, `CastSpell`, the death saves, the reactions and `GetTurnOptions`.
  Undo stays closed after an ended part (`turn_part_ended` is not undoable).
- **What each receives (RN-20).** `Encounter.turn_group_ids` and `Combatant.turn_part_ended` only for a group that has a player character, never with a hidden member.
  An NPC-only group reaches the player as the ids they see, with no chosen `current_combatant_id` or total, plus `npc_only_groups` for the plural "os Goblins".
  A player's economy reaches them, the master and the other players of the same group.
- **Events.** `turn_part_ended` (a part ended, the turn still standing) and `turn_ended` (the turn passed), plus the log line "Brisa encerrou a parte dela".
  No stream event of its own: `encounter_changed` and `turn_changed` suffice.
- **On the web.** `core/combat/joint-turn.ts` (pure) groups the order; the master's card is `joint-card`, and `end-part` and `joint-others` are the player's.
  `CombatState.applyTurn` keeps the group and ended parts when `turn_changed` moves the current combatant inside the same round.

## Who sees what (RN-10, RN-20)

The filter runs on the server (`combat_view.go`).
`TestRN20_PlayersNeverReceiveHiddenCombatantsOrNPCNumbers` reads the player's response as the app's JSON and searches it for the hidden combatant and the NPC's numbers.

| What | The master sees | The player sees |
| --- | --- | --- |
| A hidden combatant (a new NPC is born hidden) | All | None: not the id, name or position |
| A hidden one's turn | The combatant | "Vez do mestre": empty `current_combatant_id`, `master_turn` true |
| An NPC's HP and temporary HP | The numbers | One word: Ileso, Ferido, Muito ferido (half or less) or Derrotado |
| Any combatant's AC | `Combatant.armor_class` and `AttackRoll.target_armor_class` | Never: only Acertou or Errou |
| An NPC's initiative | Total, d20 and bonus | Only the order |
| A player's initiative | Everything | The total of every player; the d20 and bonus only of their own |
| A player's HP | The numbers | Their own from the live session; the others' only as "Caído" at 0 HP |
| Speed, movement left, action, bonus action, reaction | All | Only of their own character |
| Which character an NPC is a copy of | Yes | No |

## Idempotency and concurrency

Every write carries `idempotency_key`.
One transaction locks the open session's row, checks the key in `session_events`, changes the rows, writes the event and ends.
A repeated key changes nothing and stores nothing; the response is the combat as it is, and only the generic hints are published (`encounter_changed`, and `combat_log_changed` when the first event was not hidden), because the first call may have died before telling.
An attack's or damage's response is rebuilt from the stored event: a resend never rolls again; another change's key, or the same key with another request (the hash kept on the event), is `invalid_argument`.
Two simultaneous changes wait for each other, so a double tap on "Encerrar turno" never skips two turns: the second brings an `expected_combatant_id` that is no longer on turn and gets `aborted` (`TestEndTurnIsIdempotent`).

## Combat on screen

The session page (`pages/live-session`) reads the combat (`CombatClient.turn.get`, `core/combat`) at every `ready` and after each `encounter_changed` newer than its copy; `CombatState` compares revisions and sets an older copy aside.
`turn_changed` and `combatant_moved` apply in place; a combatant the screen does not know makes it read.
A read that began before one of those events, or before an answer to a call was applied, is dropped when it answers, and the page reads again up to 3 times (`CombatState.changedSince`).
A read that fails is retried once 1.5 s later; after that the next stream event or reconnection reads it.
The server has already filtered what each sees: the screen only draws what arrives.
`CombatView` (`pages/live-session/combat`) picks the screen: for the master the initiative, the combat and the summary; for a player "Role a iniciativa", the turn with the map and the Move page, and the summary.
Errors become Portuguese through the code and `EncounterBlocked` detail (`combat-errors.ts`); `blockedMessage` is a table of sentences by reason, and the log's `logLine` a table of one builder per kind in `combat-log.ts`.
Dragging a token saves one move at a time per combatant (`MoveSaves`); if the server refuses, the token goes back and the screen says why.
The combat code loads only with the session page.
The grid has its own page (`/campaigns/:id/maps/:mapId/grid`, `MapService.SetMapGrid`) and printing the map another (`/campaigns/:id/maps/:mapId/print`, MR-033, web only; see [Design](../../design/maps.md#printing-pagesmapsmap-print)).

`CombatClient` (`core/combat/combat-client.ts`) holds five parts built on one `CombatClientBase` (the generated client and the keyed sender): `turn` (setting up, turns, actions, undo, log, highlights), `moves` (movement, sides, cover, opportunities, roster), `attacks` (attacks, damage, roll modes, reactions, concentration), `casts` (spells, hidden reveals) and `combatants` (death saves, conditions, effects, hit points). Screens call `api.attacks.rollAttack(...)`, never a forwarder on the client.

**How `CombatView` is put together.** The component (`combat-view.ts`) is the composition and the inputs the template reads; the logic is in parts it builds in field initializers, each a plain class that reads the screen through a few getters, so each is tested with the same fake `CombatClient`:

- `CombatCalls` (`core/combat`): the one call at a time with its `busy` and `error`, the answer applied and the combat read again after a stale refusal; every other part shares it.
- `MoveFlow`: the "Mover" page, the jump, the master's drag and the reach.
- `CombatSheets`: opens every sheet and dialog and knows which are open.
- `CombatReads`: what the screen reads from the services around the combat (death-save rule, spell details, roster, beast, familiar).
- `MasterAutoPass` (`core/combat`): the master's reaction clock (see [Reactions](reactions.md)).
- What a sheet is handed is built by pure functions next to it (`attack-sheet/attack-sheet-data.ts`, `cast-sheet/cast-sheet-data.ts`) and what the screen derives by `core/combat/combat-derived.ts`.

The cast sheet and the attack sheet follow the same pattern one level down: `cast-sheet-view.ts`, `cast-result-view.ts` and `attack-sheet-view.ts` are pure; the state several parts share lives in plain classes (`CastDamage`, `CastQuestions`, `ModeRequests`, `AttackQuestions`).
Sheets live in one frame (`sheet-frame`, scrolling inside with a fixed footer) and open through `openSheet` (a sheet on the phone, a dialog from the tablet up).
While a sheet's own request is in the air it cannot be dismissed, so its answer is always shown and a second sheet never starts a second spend.
Each action carries its own `idempotency_key`, made once and resent on the retry; the key follows the request, not the sheet (`ActionKey`): the same values again are a retry and keep it, while another die, slot or target is a new request with a new key.
`SpellCatalog` reads `GetSpellDetails` only when the screen needs it and forgets it on `content_changed` and on every `ready`.
