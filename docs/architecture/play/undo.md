# Undo

`UndoLastAction(expected_event_id)` undoes one step, master only.
It is a compensation, not an erasure: the history keeps both the action and the undo.
The code is `combat_undo.go` and `combat_undo_kinds.go`.

## What can be undone

The last action is the session's latest `session_events` row that an undo has not yet undone (`lastAction`, looking at the latest 100 events).
It can be undone if it is one of `undoableKinds`: an attack, a damage, an "Aplicar" or "Não aplicar", a standard or feature action, an NPC's HP adjustment, a cast spell (a teleport puts the caster back on the square it left), a Shield used or declined, a Parry used (the creature gets its reaction back and the hit waits for the window again), a death save, the conditions or concentration, a move (including a jump), a trap triggered in the combat, the start of Wild Shape or of the familiar's sight, and the master's reveal of a hider.
A legendary action is given back by the undo of the event that spent it (`spend_cost` on the event).
That is the master-only text spend (`legendary_action_spent`), a legendary attack and a legendary move.
An undo cannot cross the reset of the creature's own turn: the turn passing is the last event and closes the chain.
So only the spends after the last turn change can be undone.
A legendary attack's undo gives back only the cost, since it spent no action, attack or reaction.
The undo of a cast gives back the uses of Legendary Resistance the master spent on its saves with the slot and the damage.
It also gives back the uses spent on the answers to the catches of the cast's zone (`zoneResistsBack`).
The use of a limited action is given back by the undo of the event that spent it (`limit_key` on the event): the Recharge action is available again, or the action with uses a day has the use back ([Creatures](creatures.md#limited-actions)).
The roll of a Recharge action at a turn start (`recharge_rolled`) cannot be undone, and it closes the chain like the turn change.
The death confirmation cannot be undone, and neither can a contest, a hit rider, a turn passing, an effect's or a concentration's saving throw (so a use of Legendary Resistance on those saves, or on a rider's, is not given back; a zone's catch at a cast is the exception above).
The line of a Parry the server used on an attack is written just before the attack's event, and the undo of the attack takes it back with the attack and gives the creature its reaction back.
The master's choice of a monster's Parry mode (`monster_reaction_mode_set`) never closes the chain: the undo goes on to the action before it.
The master's switch of the sunlight (`sunlight_set`, secret, no log line) is the same: it never closes the chain.
Any later event that is not undoable leaves `NOTHING_TO_UNDO`, because undoing over it would stop being a restoration of what was before.

Some events are written just before the change's own event and never close the chain, because the undo acts on the change and takes them along:

- the creatures a cast made or an ended concentration dismissed (`creature_summoned`, `creature_dismissed`);
- a concentration that ended at 0 HP, whether a damage or the master's adjustment of an NPC's HP took the combatant there (the event is grouped by the pending damage or by the combatant, and the undo of the change takes it back);
- the opportunity offers a move made (an offer the master makes by hand closes the chain instead);
- a door a move opened (it stays open after the undo) and a hiding an attack ended;
- the puzzle events and the clue a solve gave;
- what the traps did that is not this combat's (a trap noticed, a firing or search outside a combat or in another);
- a Wild Shape that ended by itself (the beast fell, the druid fell, the master).
- a swap of gear in the combat (`gear_swapped`): drawing or stowing a weapon, the shield or, by the master alone, the armor (a player's armor swap is refused in a combat). The event keeps the turn's economy from before the swap (`action_before`, `bonus_before`, `reaction_before`, `dashed_before`, `object_interaction_before`, `extra_action_before`) and whether the piece was equipped (`equipped_before`).
  The undo of the last action takes the swap first: it gives the economy back and asks the characters module (`CombatRoster.RestoreEquipped`, the same write as `SetEquipped`'s, inside the undo's transaction) to put the piece back in or out of `carried_keys`, so the armor class and the attacks come back with it.
  An undo of an earlier event is refused while the swap stands, so it never gives back what the swap spent.
  The undo is refused (`failed_precondition`) and writes nothing when the sheet no longer lists the piece or no longer allows the gear since the swap. A player's combatant cannot leave a running combat, so the chain is never closed that way; the undo still answers `nothing to undo` if the combatant is gone.
  The undo locks the session first and the character second, `SetEquipped` the character and then the session: when the two cross, the database aborts one of them (40P01 on PostgreSQL, 40001 on CockroachDB; either can be the one) and `db.InTx` runs it again, so neither error reaches the user. `TestUndoOfASwapCrossingASetEquippedTakesTurns` runs the real calls at the same moment, many times, on a pool of two connections, and checks that `SetEquipped` always succeeds and the undo either applied once or was refused as stale.

A held action's event closes the chain, because nothing of it happened yet.

The walk over those events is `chainRules`, a list of rules read in order, the first that has a say deciding.
An event that a new change writes beside its own is a new rule there.

Each event stores the before (HP and `defeated`, the economy, the attacks made, the damage state, the death counts, the concentration, the conditions, the AC bonus, the form) and what it spent (the slot, the resource use).
Undo writes it back through the same paths (`character_vitals` for slots, uses and HP; `combatants.slots_used` and `combatants.creature_spent` and, for a persistent NPC, `npc_spell_use` for an NPC's slots, uses, uses a day and Legendary Resistance) and stores an `action_undone` event with the undone event's id.
The undo of an NPC's cast gives the slot back only when the event says `slot_counted`, and the use of a limited grant when it stores `grant_use`; an at-will cast gives nothing back.
`takeBack` reads the combatants once and hands the event to the function of its kind in the `undoers` map (`combat_undo_kinds.go`); a new undoable kind needs an entry there and in `undoableKinds`.
`expected_event_id` is the log's `undoable_event_id`; if it is already another, the call is `aborted`, and two masters never undo two things.
The undone line disappears from the log.

## What the befores hold

- **The hit points the action left.** An undo is refused when a hit point value it would overwrite is not the one the action left.
  The values are the hit points, the temporary hit points, the Wild Shape pool and the defeat of each combatant whose numbers the action changed.
  The refusal names the combatants that changed and never says a number; a combatant that can no longer be read counts as changed.
  A refused undo writes nothing.
  The undo writes no hit points for a combatant the action did not change, so a change made since stays.
  A write that leaves no event, such as a rest or a level change, is what the check catches.
  A character's held hit points are cut to the maximum it has now, so a lowered maximum does not refuse the undo.
  An event without held values is not checked, and neither are death saves or conditions, which change only through combat events.
- **The melee hit mark.** A melee weapon hit marks the turn on the attacker (`combatants.melee_hit_turn`, which the grapple maneuver needs), and the attack's event keeps the mark it replaced (`melee_hit`, `melee_hit_before`).
  The undo puts that one back: undoing the second hit of a turn leaves the first one's mark, undoing the only hit takes the maneuver away.
- **The parts of a damage.** A roll that has no parts of its own leaves `pending_damages.parts` as the hit left it (`ClearPendingDamageRoll` gives back the empty list).
  A roll with parts rewrites the list with the choice made (Divine Smite and its slot, the maneuver) and keeps the list the hit offered in the event (`parts_before`); the undo writes it back, so the damage waits to be rolled again offered as it was, nothing chosen.
- **The "ended" lines of the effects a change ended.** A change that ends lasting effects (the end of a concentration, the master's conditions taking off a condition an effect holds, a damage at 0 HP, a cast that replaces a concentration) writes each effect's `lasting_effect_ended` line before its own event.
  The event keeps the effects (`restore`) and the ids of those lines (`ended_lines`).
  The undo puts the effects back with their ids and clocks and lists the lines in `action_undone.undone_also`, so no log still says the effects ended, and the lines do not stop the chain.
  A line is only hidden from whoever could already read it, so a hidden effect tells a player nothing (RN-10).
- **What a table action or hook gave.** The event of a table action keeps the points spent (`points_spent`), the target and its hit points before (the ally's, when it went to one) and the temporary hit points gained; a death save or a damage that ran a hook keeps `hook` (the uses it spent and restored). The undo gives the uses back (`hookBack`) and puts the hit points as the event had them.
- **The temporary hit points an ended effect took off.** An effect that gave temporary hit points (Heroism) takes them off its target when it ends (`clearTempHP`); the change's event keeps the amount for each combatant (`temp_cleared`) and the undo gives it back, after the change's own undo for an action that ended the effect before its changes (a cast that replaces a concentration) and before it for a damage (which ended it after the damage landed, so the damage's own before wins on the same combatant).
- **What an ended effect left behind.** An effect whose end gives another (Haste's lethargy, `on_end` in the catalogue) makes it in the same change (`leaveFollowUp`), with a `lasting_effect_added` line before the change's own event.
  The event keeps the new rows (`follow_ups`) and their lines (`follow_up_lines`); the undo puts the ended effects back (`effectsBack`), deletes those rows, works the combatants out again and takes the lines back in `undone_also`.
- **The concentration a rage ended.** A barbarian who rages while concentrating loses the concentration (`beginRage` calls `stopConcentrating`); the action's event keeps the spell (`conc_ended`) and the creatures that went with it, and the undo of the rage puts both back, as the undo of a conditions change does.
- **The windows of a discarded hit.** A hit that waits for a reaction holds it in a reaction window, and the master's "Não aplicar" closes that window (`settleReactions` finds the trigger gone).
  The discard's `damage_discarded` keeps the ids of the windows that were open (`closed_windows`, `discardHit`), and its undo puts the damage back in `awaiting_reaction` and reopens them as they were (`reopenWindows`: same row, reactor, trigger and place in the order).
  The wait's clock is the master's screen's, which starts again when it sees the window open.
- **A Hunter's Mark the cast moved.** The caster has one mark at a time: a new Hunter's Mark on another target, or another concentration spell, takes the old one off.
  The cast's event keeps the marks it took off (`marks_before`, `clearHuntersMark`), and its undo writes them back with their ids (`marksBack`, `RestoreHuntersMark`).
- **A Hunter's Mark a concentration ended.** A rage, a damage to 0 HP, a failed concentration save and the master's end of the concentration end the Hunter's Mark with `stopConcentrating`, which takes the mark off its target; the event keeps it (`marks_before`) and `endedConcentrationBack` writes it back with the concentration, for every undo that gives a concentration back.
  A mark whose target or caster left the combat stays off.
- **The zone a cast kept.** A cast of a spell that keeps an area on the map (see [Spell zones](spell-zones.md)) stores the zone with the cast's `cast_id`; the undo of the cast deletes the zone with that id, together with the concentration it gives back. What a zone later did (a creature caught, an area moved) is not undoable as an action, and closes the chain like any other event.
- **A creature a cast pushed.** Each hit keeps the forced move of a failed save (`pushed`, with the square it left); the undo puts the creature back there and the fog follows it (see [Movement](movement.md#forced-movement)).
- **The "revealed" lines of an area spell.** Under the table rule that reveals the hidden creatures an area hits, the cast writes a `combatant_hidden_set` line (`by_area`) for each before its own event (`revealCombatants`).
  The event says it revealed the creatures its hits name (`revealed_hits`, read by `revealedIDs`, so that the ids are not written twice) and keeps the lines' ids (`reveal_lines`), and its undo takes the lines back in `undone_also`: the master's log keeps no "revealed" line for a creature hidden again.
  A player's log never listed them once the creature was hidden again, so nothing appears or disappears for a player (RN-10).
- **Wild Shape, a trap, a creature.** See [Creatures](creatures.md) and [Traps](traps.md).

## When the event is too large

An action never fails because of what only its undo needs.
An event whose Go encoding is over `undoHelpersBudget` (14,000 bytes: the database's text is about 8% longer, and the table allows 16 KiB) drops those lists one at a time, in a fixed order, until it fits (`trimUndoHelpers`, `combat_event_budget.go`): the line lists first (`reveal_lines`, `follow_up_lines`, `ended_lines`), then `follow_ups`, `marks_before` and `closed_windows`.
`play` logs a warning with the event's kind and the names of the lists it dropped (never an id).
The undo of that event then does what it did without them: the lines stay and stop the chain, the lethargy stays, the first target stays unmarked, the hit goes on to its roll.
`restore` is never dropped.
A placed area cast with 40 targets fits: each target adds a few hundred bytes (more with a saving throw, two damage types, cover, fog or a hidden target).
