# Combat log and events

Every combat change becomes a `session_events` row, and the combat log is built from those rows on every read (ADR-0007).
`ListCombatLog` has no table of its own, so the history and the log cannot disagree.
The code is `combat_events.go`, `combat_log.go`, `combat_log_rolls.go` and `combat_write.go`.

## The log

`ListCombatLog` builds the log from the combat's `session_events` (the `encounter_id` column), from the latest round to the first.
Each line is structured (kind, round, who did it and to whom, the attack or action and its Portuguese name, the rolls, the result, the damage and type); the Portuguese sentence is the app's (`logLine` in `combat-log.ts`, one builder per kind).
An attack's events (the roll, the damage, the "Aplicar") become one line, the attack's.

The log contains: the start and end of the combat; attacks (with the damage, the amount the master applied in place of the rolled one, and the concentration reminder); spells cast (one line with the result on each target); reactions; death saves and the confirmed death; conditions and the end of concentration; standard and feature actions; the movement on turn; a rider's mount and dismount; and, for the master only, an NPC's HP adjustment, the show/hide, the monsters added (`COMBAT_LOG_KIND_MONSTERS_ADDED`, also before round 1), the legendary actions spent on a text option (`COMBAT_LOG_KIND_LEGENDARY_SPENT`) and the offers made by hand.
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
An attack that made a power attack carries `CombatLogEntry.power_attack` (`CombatLogPowerAttack`: `key`, `name_pt`, `penalty`, `damage_bonus`, `hidden`).
The master and the attacker's player read all of it, and the app writes "ataque poderoso (−N no ataque, +M no dano)" with its signed formatter (`signed` in `core/effects/effects.ts`, used by `core/combat/power-attack-text.ts`).
For another player the app prints no key, number or name, and writes only "usou um ataque poderoso", whether or not the master hid the feature.
`name_pt` for them is the feature's name, or "Outra fonte" when the master retired or switched off the feature (`hidden`); the app does not print it.
A swap of gear (`COMBAT_LOG_KIND_GEAR_SWAPPED`, `CombatLogEntry.gear_swapped`: the piece's name, `weapon`, `shield` or `armor` (the master's alone), drawn or stowed, and the cost `free`, `action`, `extra_action` or empty when the master made it) is a line for the master and the character's owner alone, on any map; no other player reads it, the event (`gear_swapped`, secret) or the hint that the log changed, and the master's undo of the swap takes the line away from both. When the master made the swap the line ends with "(pelo mestre)".
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

## Reading the log again

`ListCombatLog` answers with the whole log or with what changed after the number the app holds.
The app sends `since_seq`, the `last_seq` of its last answer; 0 asks for the whole log.
The answer carries `last_seq` (the number to send next time) and `full` (the rounds replace everything the app holds).
A delta holds the entries whose newest event is above `since_seq`, and the app puts each in place by id, in the round and the place it already has.
An entry that a later event joins (the damage of an attack) takes that event's number, so it comes again and replaces its old copy.
For a player, only an event the player hears of joins the number: one that is secret, that the fog keeps from them, or that adds only creatures they are not given (the master adding a hidden NPC to a firing the table saw) leaves the entry's number where it was, because it changes nothing they read.
The entry's own first event always counts: it is the line, and a player who is given a line the table is not (a reactor's own player) is given its number.

`last_seq` is a number of `session_events.seq`; so is `CombatLogEntry.seq`, which only the master reads.
A player's entries carry `seq` 0: the numbers of the entries a player reads would have gaps where an event they may not see sits.
The number of a player's answer is built from the same rule as the entries: the highest number among the entries they read (each one counting only the events the player hears of, see above) and the events that would have asked for the whole log; no event they do not hear of reaches it, whether it sits on a line of its own or joins one they read.
They are unrelated to the live stream's `session_seq`, the hint number ([Live session](live-session.md#the-hint-number)): two counters, never compared.

### When the answer is the whole log

An event after `since_seq` that may change how an entry the app holds is seen, without a new event on that entry, makes the answer `full`.
Only the kinds in `deltaSafeKinds` are safe: they add a line or join the entry they name (attacks and damage, actions, saves, spells, reactions, conditions, movement, turns, the start and end of the combat, the legendary spend and the Recharge roll).
Every other kind counts: a combatant shown or hidden, summoned, dismissed or removed, an answer to a roll request, an undo, and a kind the list does not know.
When in doubt a kind stays off the list.

| Reader | What counts as a change to the whole log |
| --- | --- |
| The master | Every event off `deltaSafeKinds`, and any event the server cannot read |
| A player | Only what the player may see: an event that is secret or out of their sight is ignored; an undo counts only when the action it took back was one they could see; a toggle counts only when the combatant is in a line the player holds (one at or below `since_seq`, not secret, not the master's alone, seen); an add whose combatants all came in hidden is ignored |

Because a player's answer ignores what they cannot see, it tells them nothing a hidden write did.
A player's `last_seq` is the highest entry number among the entries they read, raised to the highest event that would have asked for the whole log, so the next read does not meet the same event again and ask for everything once more.
It does not move on a write they may not see, whether that write makes a line of its own (the master's alone, secret, out of their sight) or only joins a line they hold (the damage the master applies to a hidden target of a cast they see, a hidden NPC added to a firing).
The master's `last_seq` is the highest event number of the combat.
In a delta, the master's `undoable_event_id` entry is always sent, so the app can derive the `undoable` flag: it is right only on the entry of that id.

`TestAPlayersSinceSeqAnswerIgnoresWhatTheyCannotSee` (`backend/internal/leaktest/seq_test.go`) checks the player's answer against the worlds where the hidden write was not made.
The row "a hidden NPC added to a firing the table saw" of `TestAHiddenWriteMovesThePlayersNumberAndNothingElse` does the same for a hidden event that joins a line the player reads, and `TestStampSeqsOfAPlayerCountsOnlyTheEventsTheyHearOf` (package `play`) pins the rule of the entry numbers.

### In the app

`CombatLogState` (`web/src/app/core/combat/combat-log-state.ts`) keeps the `last_seq` it holds and the combat it belongs to.
It asks with `since_seq = last_seq` on every `combat_log_changed` and every reconnection, and with 0 for a combat it does not hold, a first read, or a read that asked for the whole log.
A `full` answer, or a read that asked for the whole log, replaces the rounds; any other answer is merged by id.
The master's `combat_log_changed` carries `full` when the log was rewritten (an undo), so the app drops what it holds and asks with 0.
That is an optimisation only: the answer is `full` whenever the log was rewritten for that reader.
A player's `combat_log_changed` never carries `full`: an undo they cannot see must not tell them anything an ordinary change does not.
An `encounter_changed` hint with no revision may mean a combatant was shown or hidden, so the app reads the whole log then.
Renaming a creature (`RenameCreature`) writes no log event: it tells the combat changed (`encounter_changed`, with the session's number), and the app reads the log again.
Changing the table rules (`SetTableRules`) writes no event either: it moves the session's number, and the master hears a `combat_log_changed` with `full` for the combat in progress, so the app drops the log it holds and reads from 0 (the rules decide what a line shows: `hideDeath`, `showSaveDC`). A player hears no event of it; their stream shows the number at the next beat.

## Events

Each change is written in the same transaction as the change, with ids and numbers only; the event carries the `encounter_id`, the round and whether a hidden combatant was in it.
A weapon's or action's name never goes in the event: the log reads it from the sheet, on the spot.
The kinds include `encounter_started`, `initiative_submitted`, `initiative_order_set`, `combat_begun`, `turn_ended`, `turn_part_ended`, `combatant_moved`, `combatant_hidden_set`, `combatants_added`, `combatant_removed`, `encounter_ended`, `attack_rolled`, `damage_rolled`, `damage_applied`, `damage_discarded`, `action_taken`, `hit_points_adjusted`, `action_undone`, `spell_cast`, `reaction_used`, `reaction_declined`, `death_save_rolled`, `death_confirmed`, `conditions_set`, `legendary_action_spent` (the master's text spend of a legendary action), `gear_swapped` (a weapon, shield or armor drawn or stowed in the combat; see [Undo](undo.md)), `mount_kind_set` (the master's mark of a willing mount; it writes no line of the log, see [Mounted combat](movement.md#mounted-combat)), `combatant_mounted` and `combatant_dismounted` (`COMBAT_LOG_KIND_MOUNTED` and `COMBAT_LOG_KIND_DISMOUNTED`, the master's alone when either row is hidden), `feature_used` (a use of a feature, `COMBAT_LOG_KIND_FEATURE_USED`: the audience of an action, the numbers for the master and the user's player; see [Feature state](feature-state.md#using-a-feature)), and the kinds of the features around combat (reactions, contests, traps, wild shape, effects, exhaustion, and `spell_zone_triggered` and `spell_zone_moved` for the areas spells keep on the map, which read as effect lines; see [Spell zones](spell-zones.md)).
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
| `combat_log_changed{encounter_id, full}` | A log line appeared, changed or vanished | The master always, with `full` set when the log was rewritten (an undo); the player only if the line has no hidden one (the end of a hidden combatant's rage, `EndRage` and `AnswerRageEnd`, included), and never with `full`. A notice with no log content: the app reads `ListCombatLog` again ([Reading the log again](#reading-the-log-again)) |
| `reaction_window_opened`, `reaction_window_closed` | A reaction window opens or closes | The reactor and the master |

`finish` publishes after the commit on a context that outlives the call, with a timeout.
A repeat of an idempotent call tells the hints again, with the session's hint number as it stands, because the first call may have died before telling ([the replay rule](live-session.md#the-replay-rule)).
