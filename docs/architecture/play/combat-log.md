# Combat log and events

Every combat change becomes a `session_events` row, and the combat log is built from those rows on every read (ADR-0007).
`ListCombatLog` has no table of its own, so the history and the log cannot disagree.
The code is `combat_events.go`, `combat_log.go`, `combat_log_rolls.go` and `combat_write.go`.

## The log

`ListCombatLog` builds the log from the combat's `session_events` (the `encounter_id` column), from the latest round to the first.
Each line is structured (kind, round, who did it and to whom, the attack or action and its Portuguese name, the rolls, the result, the damage and type); the Portuguese sentence is the app's (`logLine` in `combat-log.ts`, one builder per kind).
An attack's events (the roll, the damage, the "Aplicar") become one line, the attack's.

The log contains: the start and end of the combat; attacks (with the damage, the amount the master applied in place of the rolled one, and the concentration reminder); spells cast (one line with the result on each target); reactions; death saves and the confirmed death; conditions and the end of concentration; standard and feature actions; the movement on turn; and, for the master only, an NPC's HP adjustment, the show/hide, the monsters added (`COMBAT_LOG_KIND_MONSTERS_ADDED`, also before round 1), the legendary actions spent on a text option (`COMBAT_LOG_KIND_LEGENDARY_SPENT`) and the offers made by hand.
The line of a legendary spend reads "Draco gastou 2 ações lendárias: Wing Attack", the option as the SRD names it.
A save the master made a success with Legendary Resistance is, for the master, the effect line (`COMBAT_LOG_EFFECT_CHANGE_SAVED`, with `resisted`, `resisted_left` and the roll that failed) or the concentration line that names the failed roll and the choice.
A save of a spell the master turned into a success is, for the master, the spell line with the failed roll, the choice (`resisted`) and the uses left (`resisted_left`).
A rider's save the master turned into a success is, for the master, the rider line with the failed roll, `resisted` and `resisted_left`; a player reads a save that passed.
The window and its answer write no line of their own.
A player reads the line of a save that passed: no roll, no count, no name for the trait.
A legendary attack or move is the ordinary attack or movement line, which says nothing of the spend.
No player reads the master's line, the event (`legendary_action_spent`, secret) or the stream change it sends.
A Recharge roll at a creature's turn start is, for the master and for the player who controls the creature, a line (`COMBAT_LOG_KIND_RECHARGE_ROLLED`, `CombatLogEntry.recharge`) with the action, the d6 and whether it recharged.
No other player reads that line, the event (`recharge_rolled`, secret) or any change for it, and the log of those players is the same with or without the roll.
The roll is in no player's revision, the controller's included: the turn start already changes it.
What a table feature's `on_event` hook gave (the hit points, the temporary hit points, the uses it spent and restored) is in the line the death save (`CombatLogDeathSave.hook`) or the damage (`CombatLogDamage.hook`) already writes, and what a table action gave in the line of the action (`CombatLogDamage` with `healing`, `amount` and `temp_hp_gained`); the master and the character's own player read the numbers and everyone else reads the line without them (RN-10). The event keeps the feature's key, never its name; the line names it.
Movement lines carry `distance_ft`, `distance_dft` and, for a walk, `spent_dft` with `move_dragging` and `move_crawling`: what the movement really spent (the path's cost doubled again by a drag and by a crawl).
The line says the second number only when it differs from the distance, with the reason in words (another creature's space and difficult terrain, "arrastando", "rastejando").
The master's free move, a jump, and the no-map combat's "gastou 6,0 m de movimento" carry no `spent_dft` and say the distance; the numbers are those the panel already shows every player.
A failed save that pushes a creature (Thunderwave, the Open Hand push) shows in the line of the cast or of the rider.
`pushed_ft` is the distance and `push_stopped` says something ended the push early.
`push_no_map` says the combat has no map.
A creature that saves has none of them.
Initiative, turn order and reinforcements are not log lines.

| What | The master | The player |
| --- | --- | --- |
| A line with a hidden combatant (now, or when it happened) | Yes, with `hidden` on ("Só o mestre vê") | None: not the id, the name or "waits hidden" |
| A combatant the master shows later | | The new lines; the old ones (made hidden) stay out (each event stores `secret`) |
| The dice (d20 and damage) | All | Only their own character's; for the others, the result and the damage they suffer or cause |
| A spell target's save | The dice, the bonus and whether the bonus is known | The result as a word; the dice only if the target is their character (never an NPC's); the DC, only the caster |
| A casting that touches a hidden one | Yes, with `hidden` on | No line |
| The amount the master applied in place of the rolled one | Both | Only what the character took |
| The reaction notice (Shield) | All, with the attacker | Only their own character's, without the attacker and the blow's total |
| An NPC's Parry used by the server | The line "<name> aparou (+N CA): T contra CA A, o ataque errou." (`COMBAT_LOG_KIND_REACTION_WINDOW`), and the attack's own line, a plain miss | The line "<name> aparou o ataque." when the master has hidden neither the creature nor the attacker from them, with no number, and the attack's own line, a plain miss |
| An NPC's Parry answered in a window ("Perguntar") | The line "<name> aparou (+N CA)" with the total and the armor class, and the attack's line stopped by a reaction (`stopped_by_reaction`) | No line of the reaction; the attack's line is a plain miss (`stopped_by_reaction` unset, no armor class), or a hit when Parry did not turn it |
| A Parry that changed nothing | No line | No line |
| Conditions | All | Those of the combatants they see |
| "Morrendo" | Yes | Never: "Caído", with the counts (public unless the table hides them); "Estável" and "Morto" are for all |
| HP after the damage (`hit_points_after`) | Yes | Never (RN-20) |
| Anyone's AC | Not part of the log | Never |
| An NPC's HP adjustment, show/hide | Yes | None |
| `undoable_event_id`, `undoable` | The last action | Never |

On a fogged map, each event is also stamped with the users who saw its NPCs (see [Combat under fog](fog.md)).

## Events

Each change is written in the same transaction as the change, with ids and numbers only; the event carries the `encounter_id`, the round and whether a hidden combatant was in it.
A weapon's or action's name never goes in the event: the log reads it from the sheet, on the spot.
The kinds include `encounter_started`, `initiative_submitted`, `initiative_order_set`, `combat_begun`, `turn_ended`, `turn_part_ended`, `combatant_moved`, `combatant_hidden_set`, `combatants_added`, `combatant_removed`, `encounter_ended`, `attack_rolled`, `damage_rolled`, `damage_applied`, `damage_discarded`, `action_taken`, `hit_points_adjusted`, `action_undone`, `spell_cast`, `reaction_used`, `reaction_declined`, `death_save_rolled`, `death_confirmed`, `conditions_set`, `legendary_action_spent` (the master's text spend of a legendary action), and the kinds of the features around combat (reactions, contests, traps, wild shape, effects, exhaustion, and `spell_zone_triggered` and `spell_zone_moved` for the areas spells keep on the map, which read as effect lines; see [Spell zones](spell-zones.md)).
A new kind is a row in `session_event_kinds` (a migration) and an `event...` constant in the code; `TestEveryEventKindOfTheCodeIsInTheTable` fails if one is missing.

A payload is at most 16 KiB of the database's text of the JSON (a `CHECK` of `session_events`).
An event that lists one record per creature is written as several events past 3,500 bytes (`eventPayloadBudget`, `fitPrefix`, `combat_write.go`): the master's lines of a start's monsters (`combatants_added`, each with its dice and faces) and a trap's firing.
Any other event over `undoHelpersBudget` drops the lists only its undo needs (see [Undo](undo.md)).
An area cast that reveals the hidden creatures it hit does not list them a second time: the event says `revealed_hits`, and the creatures are the targets of its hits marked `HiddenAtCast` (`revealedIDs`), so a cast on forty hidden creatures stays under the budget. The answer to the master's question (`hidden_reveal_answered`) lists its creatures in `revealed`.

## Stream events

After `COMMIT`, `play` publishes to each audience what it may see (the hub has a "players" audience for an event the master receives another way).

| Stream event | When | Who receives it |
| --- | --- | --- |
| `encounter_changed{encounter_id, revision, mode}` | Any change in the combat | Everyone. A contentless notice: the app reads `GetEncounter` again, already filtered |
| `turn_changed{round, current_combatant_id, master_turn}` | The combat begins, the turn passes, whoever is on turn disappears or changes state | Each their own copy: the master sees who it is; the player "Vez do mestre" if it is a hidden one |
| `combatant_moved{combatant_id, col, row}` | A combatant moves | The master always; the player only if they see the combatant |
| `combat_log_changed{encounter_id}` | A log line appeared, changed or vanished | The master always; the player only if the line has no hidden one (the end of a hidden combatant's rage, `EndRage` and `AnswerRageEnd`, included). A contentless notice: the app reads `ListCombatLog` again |
| `reaction_window_opened`, `reaction_window_closed` | A reaction window opens or closes | The reactor and the master |

`finish` publishes after the commit on a context that outlives the call, with a timeout.
A repeat of an idempotent call publishes only the generic hints, because the first call may have died before telling.
