# Readying an action

The standard action Ready (SRD 5.1, "Ready") holds another action until a trigger the player names.
The code is `ready*.go` in `internal/play`; the contract is `ReadyService` in `ready.proto`.
The rules for the table are in [RN-31](../../product/rules.md#rn-31-reactions-and-the-reaction-window).

## Model

A readied action is a `readied_actions` row (migration 00334).
It holds the readier, the trigger text (1 to 200 characters), the kind, the held action and its name, the target named at Ready and the move.
A spell row also holds its slot level (`slot_level`, 0 for a cantrip) and the cast id of its Ready (`spell_cast_id`).
The kinds are `attack`, `move`, `other` and `spell`.
`attack` holds one of the readier's weapon or natural attacks by its key; a spell attack is readied as a `spell`.
`other` holds a standard action other than Attack, Cast a Spell and Ready.
Ready also refuses Hide and Help, which have their own calls a reaction does not run.
It refuses Dash and Disengage too: they last only for the readier's own turn, and no reaction reaches it.
Each of them would be taken for nothing.
`move` holds the readier's most speed on any surface at Ready, as at the start of a turn (`move_ft`).
A combat without a map (theatre of the mind) refuses a readied move (`READY_ACTION`): there is no square to walk to.
`spell` holds one of the readier's class spells by its key: see [Readied spells](#readied-spells).
A combatant has at most one waiting row.
`Combatant.readied` (119) is the waiting row, for the master and the readier's player.

## States

| Status | How it gets there |
| --- | --- |
| `waiting` | `ReadyAction` on the readier's turn spends the action and writes the row |
| `fired` | The readier used its READIED window ("Usar") |
| `ignored` | The readier let its READIED window pass ("Deixar passar"), or it passed by itself after 30 s |
| `lost` | The readier's next turn started, or it was defeated or dismissed (`loseReadiedOfDefeated`, in the settle step of every change) |
| `broken` | The concentration that held a readied spell ended (`settleReadiedSpells`) |
| `canceled` | The readier's player, or the master, gave it up on the readier's turn (`CancelReadied`) |

A row that does not wait is history: no call changes it again, and the undo is the only way back to `waiting`.
The end of the combat drops every row.

## Flow

1. On its turn the readier calls `ReadyAction`: the action is spent and the row waits.
2. When the trigger happens, the master calls `FireReadied` (master only).
3. `FireReadied` opens a reaction window of kind `READIED` for the readier, in its own group, through `openWindows`.
4. The window holds the table like any other: `reactionGate` answers `REACTION_PENDING` until it is answered or closes.
5. The readier's player answers it, or the master for an NPC, with `AnswerReaction` or, for a move, `MoveCombatant`.
   The master's walk of the token is the release of a move only with `release_readied`; without it, it is a free token move.

`FireReadied` refuses, in this order and writing nothing:

- a row that does not wait, or a readier out of the order (dismissed): `not_found`;
- a defeated readier: `COMBATANT_DOWN`;
- a readier at 0 hit points: `COMBATANT_DOWN` (`mustReactNowOrOnTurn`);
- a surprised readier before its first turn ends: the surprise refusal of every reaction (`mustReactNowOrOnTurn`);
- a readier that acts now: `READY_OWN_TURN` (a trigger cannot come during its own turn);
  A readier tied in initiative with the creature that triggers it counts as acting until its own part of the tie ends.
  Its turn is not over, so its Ready cannot fire yet, and the row keeps waiting.
- a readier that owes a concentration save: `CONCENTRATION_SAVE_PENDING` (`readiedGate`);
- a question about a hidden creature the master owes: `HIDDEN_REVEAL_PENDING` (`readiedGate`);
- an incapacitated or unconscious readier, or one an effect keeps from reacting: `CANNOT_ACT`;
- a readier whose reaction is spent: `REACTION_USED`;
- a row whose window is already open: `REACTION_PENDING`.

A refused fire leaves the row waiting until it is lost.
The fire event (`readied_fired` with `opened`) writes no log line.

## The answers

"Usar" runs the held action with the reaction, through the action's own path:

- **attack**: `RollAttack` as a reaction (`as_reaction`), with the cover and the fog as usual.
  `AnswerReaction` builds the request from the row.
  The target is the one named at Ready, or `creature_ids[0]`.
  The d20 is the typed one (`typed`), the table's (`d20_faces`, with `roll_mode` and `mode_reason`), or the app's.
  `RollAttack` with `catch_window_id` set to the READIED window does the same.
  The roll waits for the windows any attack roll opens first: Cutting Words, Protection, the master's check of "Sempre" (`holdAttackRoll`).
  It also waits for its roller's answer about a die (a Bardic Inspiration die, Lucky, Stroke of Luck), kept in `inspiration_holds` (`readiedHeldForDie`).
  Meanwhile the READIED window stays open and the row waits; `AnswerReaction` answers `used` false.
  While the roll is held, the window takes no answer: a second "Usar" and "Deixar passar" are `REACTION_PENDING` (`mustNotBeHeld`).
  The master reads it with `answer_now` false, so the automatic pass leaves it alone.
  A held roll that the attack refuses once replayed is dropped (`settleReplay`), as when the target fell meanwhile.
  The reaction stays free, the row waits and the window takes an answer again.
  The master reads the drop in the log (`held_dropped`); the readier's player reads only the window again.
  A readier that falls meanwhile loses the row, and its window closes.
  The target must be one the attacker can attack now (reach, sight).
  Otherwise the attack's own refusal comes back and the window stays open.
- **move**: `MoveCombatant` by the readier's player off its turn, while its READIED move window is open (`readiedMoveOf`).
  Its budget is its own (`readiedSpeedsOf`): each surface's speed as at the start of a turn, held to `move_ft`.
  The movement the readier spent on its own turn does not count, and neither does its Dash.
  A readied move is up to the combatant's speed, not the movement its turn had left (SRD 5.1, Ready: "move up to your speed").
  A speed that fell since Ready (a condition, an effect) shrinks the budget; one that rose does not widen it.
  So a swimmer that readied in water may swim its swim speed, and walk only its walking speed.
  It pays its way like a move on turn, difficult terrain included, and spends no movement of the turn.
  `GetMoveOptions` reads the same budget for the readier's player while the window is open (the master's options of the token are those of a free walk).
  Move's own refusals apply: a grappled readier has no speed (`TOO_FAR`).
  A refused move spends nothing and the window stays open.
  A move to the square the readier stands on is refused for a player (`invalid_argument`) and the window stays open.
  A move that stops short (a wall, a creature in the way, a trap) after at least one square is still the use, and the window ends.
  So does a walk shorter than the budget: the first move is the whole use, and a second one is an ordinary move off turn (`NOT_YOUR_TURN`).
  A walk that goes nowhere, stopped before its first square (a creature the player cannot see blocks it), has length 0 and is no use (`spendReadiedMove`): nothing is spent, the row waits and the window stays open.
  Such a release writes no session event (the undo's next target stays what it was) and tells nobody; only its own reply says `stopped_early`, as a walk on a turn does (`moveAttempt.idle`).
  The web reads that reply on the Mover page of the release.
  When `stopped_early` comes with the readier still on its square, the page stays open and shows the notice "Algo bloqueia o caminho: nada foi gasto. Escolha outro quadrado."
  The release is left unfinished, so the player picks another square or lets it pass.
  The master's release to the square the readier stands on is the same walk of length 0.
  A walk on the mover's own turn that stops at its first step still writes its event and sends the moved hint: that is not changed.
  The move is a line of the log, after the `readied_fired` line.
  It offers opportunity attacks as any walk does: only a forced move or a teleport would not.
  A Disengage does not cover it: Disengage lasts only for the turn it was taken on (SRD 5.1).
  Its offers hold the end of the turn in progress, the master's too: `EndTurn` is `OPPORTUNITY_PENDING` until each is answered.
  So none of them outlives that turn (`mustNotWaitForReadiedOffers`).
  It is held by a pending hidden reveal (`mustNotHold`) as a move on turn is.
  The master's plain walk of the readier is not the release.
  It is what it is on any off-turn walk of his: free, with no limit, and none of the release's checks (a pending concentration save, a held question, surprise) refuse it.
  It spends no reaction and leaves the row waiting with its window open, as do his forced move, placement, jump and walk of another token.
  The master releases the move for the player only when the request says so: `release_readied` on `MoveCombatantRequest`.
  That walk is a release in every way: paid and limited as the player's, with the release's checks, and its undo gives it all back.
  `release_readied` with no open READIED window of a move for the combatant, or on a move that is not a plain walk, is refused as a release with no window is: `NOT_YOUR_TURN`, with nothing written.
  A player sending `release_readied` is accepted and means nothing more: their own off-turn move while the window is open is the release anyway.
  Another player's move of the readier is `permission_denied`.
  A window the player lets pass, or the master passes, ends the move: the row is ignored, the reaction stays and no `readied_lost` line is written.
  A move after that is `NOT_YOUR_TURN`.
  The reactor's mark of being on the map (`MarkReactionRolling`) changes nothing of the window.
  `AnswerReaction` "Usar" on a move window is `invalid_argument`: it carries no square.
- **other**: the `TakeAction` path, charged to the reaction instead of the action.
- **spell**: the `CastSpell` path as a reaction: see [Readied spells](#readied-spells).

A use closes the window as answered, marks the row `fired` and writes the `readied_fired` line just before the action's own event.
It records the reaction's kind as `readied` (`recordReaction`).
The attack spends the reaction in its own economy, as an opportunity attack does.
Another action spends it in `TakeAction`'s economy, charged to the reaction.
A move spends it with `spendReactionAs`.
A refused use spends nothing and the window stays open: the readier may still pass.
Every use is refused while its readier owes a concentration save (`CONCENTRATION_SAVE_PENDING`) or the master owes a question about a hidden creature (`HIDDEN_REVEAL_PENDING`), as the fire is (`readiedGate` in `readiedUseOf`).
The answer to a held roll's die runs the use again and passes these checks: it is the use they waited for.
"Deixar passar" marks the row `ignored`, keeps the reaction and writes the `readied_fired` line (status `IGNORED`).

The window closes by itself, with the reason told to its reactor alone.
That happens when the readier is defeated or incapacitated, spends its reaction elsewhere, or the row stops waiting.
A change that ends the row (a held spell broken, a readier defeated) closes its window in the same call, a correction to 0 hit points too: the table does not wait for it.

The automatic pass of 30 s counts on the master's screen from when the window opened.
It can pass a window whose player is still choosing a square for a move, or a target: the row is then ignored.
The readier's player sets the mark (`MarkReactionRolling`) on the first touch of the list of creatures or the darts in the window, or on entering the map.
The player can clear it, and the master's wait is 120 s from `wait_began_at`, not the 30 s of the window.
The readier's prompt says it: once the player is on the map, its line reads that the table waits for the player for up to 2 minutes.
The master's own touch does not mark it: the mark is the reactor player's alone.

## The reaction

The reaction is spent only when the readied action is used.
While the row waits, the readier's other reactions stay offered.
A readier that spent its reaction elsewhere cannot be fired, and the row waits until it is lost.
A lost row gives nothing back: the action spent at Ready stays spent.

## Who reads what (RN-10)

A player who is not the reactor's reads a READIED window as one that does not exist.
`AnswerReaction` checks the caller before it reads what the row holds (`mayAnswerReadied`).
`RollAttack` refuses another's READIED window in `catch_window_id` as a window it does not find.
A `catch_window_id` that is not a UUID is `invalid_argument`.

The trigger and the held action are the master's and the readier's player's: on `Combatant.readied`, the window and the log lines.
The window carries no prompt; the master's `ReactionTrigger` names the readier, the target and the held action.
Another player reads only `reaction_wait`: "Esperando a reação de <name>" for a player character they see, else "Esperando o mestre".
Another player reads the lines (`ACTION_READIED`, `READIED_FIRED`, `READIED_LOST`) with the actor alone.
Their `ACTION_READIED` line has no `concentration_ended_key` and no `spell` (`fillReadied`).
They still see the readier's earlier concentration end: its `concentration_spell` goes blank and the effects of that spell end.
A hidden readier's window and lines are the master's alone.
`internal/leaktest` (`ready_test.go`, `ready_fire_test.go`, `ready_spell_test.go`) checks each of these with canaries and positive controls.

## Readied spells

A readied spell is cast at Ready and released by the trigger (SRD 5.1, "Ready").
The code is `ready_spell.go`, `ready_spell_attack.go` and `ready_spell_lost.go`.

### At Ready

`ReadyAction` with `kind` `SPELL` names the spell (`spell_key`) and the slot level (`slot_level`, 0 for a cantrip).
It checks the spell as `CastSpell` does (`checkSpell`): a class spell the caster knows or prepared, a slot of that level, silence.
Metamagic and spells granted by a feature are not offered: Ready reads no `granted_by`, so a row is always the caster's own spell.
A release is from the source the row was readied with, so one that names a grant (`granted_by`) is `invalid_argument`, writes nothing and spends no charge of the grant.
The casting time must be exactly 1 action, else `READY_SPELL_TIME` (a reaction or a bonus action spell, a ritual is never cast in combat).
A spell attack cantrip (Fire Bolt, Ray of Frost) is readied as a spell, with no slot (`checkSpellAttack`): its energy holds the concentration too.
Its row has no `spell_cast_id` (`heldSpellAttackRow`), and its release is `RollAttack` as a reaction, as a readied attack's (`usedAs`).
As an `attack` it is refused (`READY_ACTION`): it would hold no concentration.
A pact slot is spent only when no plain slot of that level is free.
Targets and range are checked at the release, not at Ready.
Ready refuses with `READY_SPELL_NOT_RELEASABLE` a spell no release can carry, decided by what the spell is (`checkReadyReleasable`, never a list of keys): one that summons, that holds an ability or a damage type the caster picks, or that revives.
Nothing is spent; neither the answer nor `CastSpell` on the map has a picker for those choices, so the spell could only be let pass.
A spell placed at a point or in a direction on a map, or laying a zone there, is readied: it is released on the map with `CastSpell`.
Ready spends the slot and the action, and notes the spell cast this turn.
It sets the readier's concentration to the sentinel `readied` (`concentrationReadied`), even for a spell without concentration.
A previous concentration ends there, with its line and its dismissed creatures, as a new concentration spell ends it.
No effect of the spell happens: no damage, no lasting effect, no zone, no `spell_casts` record.

### The release

The release is the spell's cast as a reaction, through `castSpell` with `answering` set (`findReadied`).
`AnswerReaction` "Usar" builds the `CastSpellRequest` from the answer and the row.
The targets are `targets` (each with its darts), or `creature_ids`, or the target named at Ready.
`targets` and `creature_ids` together are `invalid_argument`.
The target named at Ready is used only for a spell that takes named targets (`namedAtReady`): a spell that reaches its caster alone, or an area, released with no targets is cast with none.
In the window, a creature named at Ready that cannot be aimed at now is not chosen: the readier picks another or lets it pass.
`castSpell` checks them as a cast on the caster's turn: the darts of the slot and the number of targets.
`targets` on another kind of row, on a held spell attack, on a pass or on a window that is not READIED are `invalid_argument` (`badTargets`, with `targetsNotUsed`): an answer never drops what it carries.
The answer carries no point and no direction: a spell placed on a map (a sphere at a point, a cone, a wall) or laying a zone there (`laysZoneOnMap`) is refused by the answer, with targets or without, and the window stays open.
A spell that needs the map (a point, a direction, a wall, a zone) is released with `CastSpell` by the readier's player, or the master, while the READIED window is open ("Soltar no mapa").
A release with an origin lays the zone of a zone spell at the held slot: `placeArea` refuses a zone only for the answer (`answering`), never for `CastSpell`.
`PreviewSpellArea` reads the held spell at the slot level of the row while the caster's READIED window holds it (`heldByWindow`).
That preview takes no free slot of that level and reads no slot from the request; any other preview is checked against the free slots as before.
In a combat with no map the area is released with the creatures the answer picks, or with nobody.
The Ready sheet offers the spells the window releases: with darts, creatures or nothing, and the spells placed on a map or laying a zone there, which are released on the map with `CastSpell`.
It shows with a reason, and does not offer, a spell that asks the caster an ability (Enhance Ability), the creatures of a summon (Conjure Animals) or a damage type (Flame Strike): the answer carries no `ability_key`, `summon` or `damage_type_key`.
The sheet reads them as the cast sheet does: `needsAbility`, the summon options of the character and the `damage_choice` of the spell's details.
The server refuses the same spells at Ready, so a client that does not offer them is not the only guard.
The sheet fails open: a read that fails leaves the spell offered.
A revival (Revivify) is told by nothing the web reads, so only the server refuses it.
When the server refuses a spell with `READY_SPELL_NOT_RELEASABLE`, the sheet shows that sentence.
While the row waits, `GetTurnOptions.spell_targets` of its readier also lists the targets of the held spell (`heldEntry`), with the darts of its slot, so the window can show them.
It does so only off the readier's turn, where the window opens and no ordinary entry is listed: on the turn a second action (Action Surge) could list the same spell twice, with nothing to tell the entries apart.
A held spell attack lists none.
The list is the owner's and the master's, as the options are (RN-10).
The release picks only what the readier's player could pick, whoever answers, so the master reads the list under that player's sight too: an NPC the player does not see is not in it (`listHeldTargets`).
The release spends no slot, the reaction instead of the action, and takes the slot level of Ready (a pact slot spent at Ready counts as a slot of its level).
`ReadiedAction.slot_level` carries that level to the master and the readier's player, 0 for a cantrip.
A release that names a slot of another level than the held one is `invalid_argument` (`chosenSlot`): it names no slot, or that level.
The release is checked as a cast: a spell the caster no longer has prepared is refused, and can only be let pass.
`CastSpell` is a release only off the caster's turn, for the spell its own open READIED window holds (`findReadied`, with `heldByWindow` matching the spell's key); any other cast is an ordinary one.
Another combatant's window of the same spell, and an open window of another kind for the caster, are not its release.
Metamagic at the release is `invalid_argument`.
A cast of another spell while the window is open is refused as a cast off the caster's turn (`NOT_YOUR_TURN`).
A concentration spell keeps the concentration, now with its own key; another spell ends it.
The cast waits for the windows any cast opens (Counterspell), with the READIED window open and `REACTION_PENDING` (`readiedHeld`).
A spell attack's d20 held for a die waits the same way, and the master's window reads `answer_now` false.
A countered release is the use: the row is `fired`, the reaction spent, the concentration ended and the slot lost.
A refused release spends nothing and the window stays open.
The readier's player sets the mark on entering the map (`MarkReactionRolling`) and clears it on going back (the same call with `clear`).
The first set starts one wait of 120 s, and no later set or clear restarts it.
A set or a clear that changes nothing (already set, already clear) writes nothing and tells nothing.
One that changes the mark tells the master and the reactor's player alone, through `Audience{Master, UserID}`: no other player counts the hints of a wait they do not read.
It is two events, one to the master and one to the reactor's player, each an encounter hint.
On a fog map the player's copy is blind (revision 0), as every player's hint is there; the master's carries the real revision.
The server records when the wait began, once, in the window's trigger, and no later mark or clear changes it.
An undo that reopens the window (of a spell release, a move release or a pass) starts the wait clean: `rolling` false and no `wait_began_at`, so the next mark begins a new wait.
The master's row reads whether the player is on the map now (`rolling`), when the wait began (`wait_began_at`) and how long it has run (`waited_ms`).
`waited_ms` is the server's clock, worked out at each read as now minus `wait_began_at`, 0 before the first mark: a screen with a wrong clock still counts the wait right.
The master's screen passes the window when the 120 s end, with `AnswerReaction` PASS: the server runs no timer.
The reactor's player reads the mark back too (a second device of the same player shows it), `rolling`, `wait_began_at` and `waited_ms`.
No other player reads either: another player and the TV read the same wait line in every moment.
Both ways are idempotent and the reactor's player's alone (the master and another player are refused), nothing is written to the log, and the window stays open and answerable.
"Usar" on a window that was answered or closed is `NOT_YOUR_TURN_TO_ANSWER`, even on the readier's own turn: only an open window releases the spell, and the cast an answer makes names its window.
A held release whose spell breaks before the Counterspell windows are answered is lost: the row is `broken` and its window closes, the replay is refused as any cast off the readier's turn, and the master reads `held_dropped` in the log (`settleReplay`).
Nothing is cast, and no write fails.
"Deixar passar" loses the spell: the row is `ignored`, a `READIED_LOST` line names the spell, and the concentration ends.

### When it is lost

A held spell and its concentration go together, checked in the settle step of every change (`settleReadiedSpells`).
A waiting spell whose readier no longer holds the sentinel is `broken`, with a `READIED_LOST` line (status `BROKEN`).
That covers a failed concentration save, 0 hit points (`endConcentrationOfDownCharacter` calls it too), an incapacitating condition, a new concentration spell, and the player or the master ending the concentration.
A readier that holds the sentinel with no waiting spell stops concentrating, with no line of its own.
That covers the next turn, a defeat, a dismissal, `CancelReadied` and the pass, whose own lines name the loss.
The slot stays spent in every case.

### Who reads it

The held spell's key and name, its level (`slot_level`) and that it is a spell are the master's and the readier's player's (RN-10).
`Combatant.concentration_spell` carries the held key for them and nothing for the others (`heldSpellView`).
The `ACTION_READIED` line carries the slot (`spell.slot`) for them alone.
The end of the previous concentration (`concentration_ended_key`) is public on that line, as on a cast line.
The concentration save of a held spell is the master's and the owner's: its window, its result and its line name the held spell (`HeldSpell` on the trigger, `Held` on the event).
Another player reads none of it: no window, no line, and the wait reads "Esperando o mestre", never the readier's Constitution save.
A line whose held spell is unknown names it "a magia preparada".
The targets the held spell lists leave out an NPC the player does not see, and an answer that picks one is refused as one that picks an id that is nowhere.
Another player's answer to the window, with targets or without, is denied as an answer to a window that does not exist.
`internal/leaktest` (`ready_spell_test.go`, `ready_spell_release_test.go`) checks these, the save's wait included, with canaries and positive controls.

## Undo

The undo of a Ready gives the action back and deletes the row and its line.
The Ready of a row already lost cannot be undone (`chainReadied`): the lost row and its line stay.
The undo of a use takes back the action, the line and the reaction, makes the row wait and opens its window again.
The undo of a pass makes the row wait and opens its window again.
The undo of the Ready of a spell also gives back the slot, the spell cast this turn and the concentration it ended, with its creatures.
The undo of a spell's release is the cast's undo, the slot excepted, and holds the spell again with the sentinel.
The undo of a spell's pass holds the spell again.
The undo of a change that broke a held spell makes the row wait again when it gives the concentration back (`brokenReadiedBack`), and takes the line with it.
Otherwise the row stays broken: `chainReadiedDefeat` goes past its line.
The undo of the fire deletes its window while it was not answered (open, or closed by itself); the row keeps waiting.
It is refused while a use of the window is held (a roll waiting for a die or for other windows names the window): `REACTION_PENDING`.
The master ends such a hold without its player: it answers a roll held for a die with `AnswerHeldRoll` once the player's first 10 s are over (at once when no player plays the roller, `playerFirst`), and a hold waiting for other windows ends when they are answered or pass.
The undo of a change that broke a held spell makes the row wait, but does not open its closed window again: the master fires it again.
A fire whose window was answered comes back after the answer's undo.
The lost line of a defeat or a dismissal is written before the change's own event; the undo of that change goes past it and the row stays lost.

The answer carries the table's dice as the cast and the attack do.
`typed` (one d20 face, kept for old clients), `pool_sum` (the sum of a pool: Sleep, Color Spray), `d20_faces` (the physical d20s, the pair for advantage or disadvantage), `roll_mode` and `mode_reason` are copied one to one into the `RollAttackRequest` or the `CastSpellRequest` the release builds (`useReadiedAttack`, `useReadiedSpell`).
Every check of the cast or the attack applies: the number and range of the faces, the mode rules, the table's dice mode (`mustRollThisWay`).
The answer refuses what the released action does not use (`releaseFieldsOf`, `castFlow.checkReleaseDice`, `diceNotUsed`) with `invalid_argument`, and the window stays open.
`ray_rolls` (the physical d20s of each attack roll of a spell with several, the rays of Scorching Ray, one entry per attack roll in the order of the targets and of the rays each target takes) is copied too, into `CastSpellRequest.ray_rolls`, and the cast checks it (`readRayRolls`, `checkRayRolls`): a held Scorching Ray is released at a physical-dice table with one entry per ray, and `invalid_argument` on a pass, another kind of window, a non-spell row, a readied attack, a single-attack-roll spell or a roll beside it (`diceNotUsed`, `releaseFieldsOf`).
A pool sum outside the range of the dice the spell rolls at its slot is refused in `prepare` (`checkPool`), before a Counterspell can hold the release, so nothing is spent and no window opens.
The count of faces of a ray depends on that ray's mode, which is read when the ray is rolled (`d20With`): an earlier ray spends what it used, and the windows may change the mode, so a wrong count is met only at the replay of a held release.
The replay runs in a savepoint (`releaseHold`): a refusal drops the held cast and takes with it everything the replay had written, so the slot, the action and the damage of the rays rolled are not spent and the readied row stays waiting with its READIED window open, for the readier to answer again.

## Events and log

| Event | Line | Undo |
| --- | --- | --- |
| `action_readied` | `ACTION_READIED` (104) | Yes, unless the row was lost since |
| `readied_fired` with `opened` (the fire) | None | Yes, while its window was not answered |
| `readied_fired` with `ended: ignored` (the pass) | `READIED_FIRED` (105), status `IGNORED` | Yes |
| The used action's event with `readied.ended: fired` | `READIED_FIRED` (105), status `FIRED`, before the action's own line | Yes, with the action |
| `readied_lost` | `READIED_LOST` (106) | Passed over by the undo of the change that defeated or dismissed the readier |
| `readied_lost` with `ended: broken` | `READIED_LOST` (106), status `BROKEN` | With the change that ended the concentration, when it gives it back |
| `readied_lost` with `ended: ignored` (a spell's pass) | `READIED_LOST` (106), status `IGNORED` | With the pass |
