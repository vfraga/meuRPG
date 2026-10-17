# Lasting effects

`LastingEffectService` (`lasting_effects_service.proto`, messages in `lasting_effects.proto`) holds what lasts on a creature: a spell's effect, a feature's, a condition the master marks (RN-22).
The code is `lasting_effects*.go` and `lasting_exhaustion.go`.
The content (the catalogue of effects, with their conditions, dice and modifiers) is `rules/srd51/effects/combat_effects.json`.

| Call | What it does |
| --- | --- |
| `ListLastingEffects`, `ListCharacterEffects` | The effects in a combat; the effects of characters outside one |
| `AddLastingEffect`, `AddCharacterEffect` | The master adds an effect to combatants or to characters |
| `ChangeLastingEffectDuration`, `SetLastingEffectVisibility`, `EndLastingEffect`, `RemoveEffectTarget` | The master changes, hides, ends an effect or removes one of its targets |
| `RollEffectSave` | A target's saving throw against an effect (a held roll; see [Held rolls](held-rolls.md)) |
| `AdvanceGameTime` | The master moves game time outside a combat |
| `SetExhaustion`, `LowerExhaustion` | Exhaustion (below) |

## One record, two homes

In a running combat an effect is a `combatant_states` row of kind `effect` with its clock in rounds (`ends_round`, `ends_combatant_id`, `ends_phase`); outside a combat it is a `character_effects` row with its clock in game time (`seconds_left`).
Taking a character into a combat (`effectsToCombat`, called by `addParticipants`) or ending the combat or removing the character (`effectsToCharacters`) moves the row, with the same id, inside the transaction.
A monster's or a creature's effects have no outside home and end with the combat.
A round is 6 seconds everywhere (`rules.SecondsPerRound`): seconds become `seconds / 6` rounds (rounded down, `roundsOfSeconds`) and rounds become `rounds * 6` seconds.
`combatants` keeps what the effects leave on it, worked out again by `refreshCombatants` after every change: `conditions`, `effect_conditions`, `effect_ac_bonus`, `effect_speed_pct`, `effect_speed_add_ft`, `effect_no_action`, `effect_no_move`, `exhaustion_level`.
The rolls and the speed read those, so the advantage rules see an effect's condition like any other.

## The order of a turn

Code in `lasting_effects_clock.go`.

- **End of a turn.** `EndTurn` ends the combatant's part, opens one `EFFECT_SAVE` reaction window for each effect that asks an end-of-turn save (ordered by effect id), and returns while any is open; `settleReactions` calls `releaseHeldTurn`, which passes the turn once none is open.
  `RollEffectSave` closes its window first and then resolves the effect (a pass ends it), so the window never closes twice.
  A window whose effect ended closes with `EFFECT_ENDED` or `CASTER_LOST_CONCENTRATION`.
- **Start of a turn.** `startTurn` first ends what ends at the start (`effectsBeforeTurn`), then rolls the damage of a turn trigger once for the creature (`effectsAfterTurnStart`) and opens the start saves.

## A rider spent by a roll

The rider of an instantaneous spell has a clock of its own on its target's next turn: `EffectDef.Duration`, which the loader accepts on a spell's entry only when the spell is instantaneous and only as `until_start_of_turn_of` or `until_end_of_turn_of` (`lasting_effects_once.go`).
`applyLastingSpell` gives it no anchor, so `endsOf` anchors it on each target and `nextTurnRound` picks the target's turn this round when it has not acted yet, the next round's when it has.
Vicious Mockery's rider is the modifier `attack_disadvantage` with `once`: `creatureFacts` fills `combat.Creature.AttackDisadvantage` with the key of the first such effect not spent (`effectAttackDisadvantage`), and `combat.AttackMode` lists it as `SourceEffectAttack` ("Zombaria Viciosa: desvantagem neste ataque"; "Outra fonte" when the master hides an effect).
The next attack roll spends it, hit or miss: `spendAttackEffects` sets `combatant_states.spent_at`, called from `afterAttack` (every `RollAttack`) and from `spellAttack`.
The row stays until its clock ends it, and its card adds "Já usada em um ataque".
The attack's or cast's event keeps the ids (`EffectsSpent`), and the master's undo clears `spent_at` (`restoreContestState`, which also gives back the Help a spell attack used: `HelpUsed`).
A Guidance or Resistance die instead ends its effect with the roll (`spendOnceEffects`).

## Outside a combat

`castCharacterEffects` writes the rows when a cast that lasts takes effect (`takeEffect`), and `closeCast` deletes the cast's rows with it (`endCharacterEffectsOfCast`).
`AdvanceGameTime`, a finished long cast and `TakeRest` move game time on (`advanceGameTime`); an effect whose seconds run out is deleted.
`checkModeOf` (scene checks, puzzle hints, trap searches) reads the character's rows when it is not a combatant (`outsideModeOf`): conditions, exhaustion, the advantage on saves, the dice (Bless, Bane) and the fixed bonuses (Pass without Trace) go in the roll, and `checkMode.shownCheck` writes the sources.
A rider that asks a saving throw is not cast outside a combat (`castCharacterEffects` takes no such spell).

## What a player may read

`effectViewer` (`lasting_effects_view.go`) decorates the combat the way every viewer may read it (`readsEffect`, `conditionHidden`, `decorate`).
The master's table (`ListLastingEffects`, with the turn clock, the concentrations and the catalog) is built in `lasting_effects_table.go`; the master's commands on the effects are in `lasting_effects_rpc.go`.
The master reads everything; a player reads the effects the master leaves visible on the creatures they see, with the owner-class conditions limited to the owner.
The label of an effect that gives conditions (`labelFor`, `labelForOthers`) and the log's `CONDITIONS_CHANGED` line (`withoutOwnerOnly`) leave out the ones the viewer may not read, and a log line whose readable conditions did not change is not sent.
An effect the master hides turns, in the sources of a roll, into `OTHER_SOURCE` ("Outra fonte", `hiddenFrom`, `hideFor`).
The DC of an effect is the master's and the caster's player's.
Tests: `TestRN10_AnOwnerOnlyConditionIsInNoReadOfAnotherPlayer`, `TestRN10_TheLabelOfAnEffectNamesOnlyTheConditionsTheViewerMayRead`, and the leak tests `TestAnOwnerOnlyConditionReachesNoOtherPlayer` and `TestAnEffectTheMasterHidesReachesNoPlayer` (which plants a canary label on a hidden effect and reads the combat, the log, the turn options and both lists of effects as every player).

## Exhaustion

A player's character keeps its level in `character_vitals.exhaustion_level` (the vitals apply the halved maximum of level 4: `combat.ExhaustedMaxHP`); an NPC's is a column of the combatant.
`SetExhaustion` and `LowerExhaustion` (master only, `expected_level` required) change it in one transaction, and a long rest lowers it by one.
Level 6 needs `confirm_death`; in a combat it puts the player's character at three death failures (the `ConfirmDeath` flow), outside one the call marks the character dead (`CombatRoster.MarkDead`).
The concentration save reads `combat.SaveMode` like the other saves (exhaustion 3 gives disadvantage, with `second_d20_face` for physical dice); its prompt (`ConcentrationSavePrompt`) carries the same mode and its sources (`mode`, `mode_sources_pt`, from `concentrationSources`, the function the roll uses too), so the sheet asks for both d20 whenever the roll needs two.

## Other details

- A base-AC effect (Mage Armor) is worked out per target (`modifiersForTarget`: the sheet for a character; 13 + the initiative bonus, the Dexterity modifier, for a summoned creature).
- When a caster who concentrates dies or leaves, the effects it held on others end with reason `caster_left` (logged as `left`) and their follow-up runs (Haste's lethargy, Heroism's temporary hit points).
- An effect whose end gives another (`on_end` in the catalogue) makes it in the same change (`leaveFollowUp`), with a `lasting_effect_added` line before the change's own event (see [Undo](undo.md)).
- Shield's +5 (`combatants.ac_bonus`) is cleared when the caster's turn starts; `startTurn` writes a `combat_effect_ended` event first (the line "O Escudo Arcano de X acabou", read by the master and the caster).
  Aid is ended by `EndCombatEffect` (master only, idempotent by key, not undoable: the HP cut is not given back), which writes the same kind of event with the HP before and after.
