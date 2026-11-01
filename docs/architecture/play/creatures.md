# Creatures, Wild Shape and the familiar

A creature belongs to a character, and in combat it is a combatant of kind `creature` (MR-037).
The rules (what a spell lets you summon, a creature's numbers) belong to `rules` (see [Architecture](../rules-creatures-items.md#creatures)); this page is what the server stores and does with them, plus Wild Shape and the familiar's eyes (MR-036).

The `characters` module stores the creature (`character_creatures`); `play` puts it in combat (`combatants.kind = 'creature'`).
The two meet in two small interfaces wired in `cmd/api`: `play.CombatRoster` (what combat asks of `characters`) and `characters.CreatureHost` (what `characters` asks of `play`, set with `SetCreatureHost`).

| Who | What |
| --- | --- |
| `CharacterService` | `ListCharacterCreatures` (the master and the owning player; any other player gets `not_found`, and the HP never reaches them), `GiveCreature` (master only, any SRD or table creature, up to 40 per character; it takes the open session's lock before the character's, the order every combat write uses), `RenameCreature` (its combatant in an open combat gets the new name), `DismissCreature` (the owner or the master; dismissing one already dismissed changes nothing), `AdjustCreatureHitPoints` (master only, RN-02, only outside combat; at 0 HP the creature is dismissed), `GetSummonOptions`, `ListWildShapeForms`. The creature's sheet is `ContentService.GetCreature` |
| `PlayService.CastSummon` | Casting outside combat what does not fit inside it (below) |
| `CombatService.CastSpell` | Conjure Animals in combat (below) |
| `CombatService.EndConcentration` | Ends concentration (the player or the master) and dismisses that casting's creatures, with a `creature_dismissed` for each. `SetCombatantConditions` with `end_concentration` and casting another concentration spell do the same (RN-22) |
| Stream hints | `creatures_changed` for the master and the owner outside combat (in combat `encounter_changed` covers it). `revivify_changed`, `character_changes_requested`, `character_resubmitted` and `character_revived` are contentless; `revivify_changed` goes to the master and the caster's player, the others to the master and the owner (the reason of a review is read with `GetCharacter`, never streamed; RN-10) |

## Casting summons

**Outside combat.** `PlayService.CastSummon` casts Find Familiar (1 hour, ritual: no slot), Animate Dead (1 minute, spends the slot) and Conjure Animals (1 action).
It checks that the character has the spell (the spellbook's counts as ritual for the wizard; the Pact of the Chain gives the Find Familiar ritual without the spell), the slot (the vitals, like every spend) and the choice (`rules.CheckSummon`), creates the creatures and writes the `creature_summoned` event (ids and keys, never the name the player typed).
A new familiar dismisses the old one (reason `replaced`), and a summon that needs concentration (Conjure Animals) first dismisses the creatures of the previous concentration, inside or outside combat (reason `concentration`).
It is refused while the character is in a combat that has not ended (`SUMMON_IN_COMBAT`: there it is `CastSpell`).
In the "Conjurar" sheet a spell with `CastingSpell.summons` asks for no target: the sheet opens the creature sheet (`SummonSheet`), which picks the form, the slot or the ritual and calls `CastSummon`.

**In combat.** `CastSpell` carries `summon` (the option, the creature keys and names) and the group's initiative roll in the same `roll_in_app` / `d20_face` (the spell rolls nothing else), which the player makes like any die (RN-18).
The creatures are born on free squares next to the caster and enter the order with a single initiative; they play together if the total ties with another combatant (the usual joint turn).
Refusals: `CASTING_TIME_TOO_LONG` ("Leva 1 hora: conjure fora do combate"), `SUMMON_CHOICE_INVALID`, `SUMMON_NEEDS_INITIATIVE`, and `TOO_MANY_COMBATANTS` before spending the slot (a combat has 40 combatants, counting the creatures of the concentration that ends).
The group's bonus is the first creature's of the summon, whichever member the player rolls for.

**Table spells.** A table spell with a `summon` goes through `GetSummonOptions`, `CastSummon` and `CastSpell.summon` with the SRD's checks: the slot, concentration, the dismissal of the previous concentration and the undo.
The creatures have `source` `table` and `summon_key` the spell's key.
The count is the spell's formula with `slot()` the cast slot, plus the creatures added for each circle above the spell's own.
A formula creature resolves with `slot()` the cast slot, and its numbers are kept in `character_creatures.stat_block`.
A summon with its own initiative gives each creature its own `summon_group_id` and its own roll; one with the owner's group roll shares the casting's group.
The turn mode is read from the spell's rule when it is cast and stays in each creature's group: editing the spell changes no creature already made.
In a combat, a casting of more than one creature with its own initiative needs `roll_in_app`: one typed die is one face, and a typed face is refused with `SUMMON_NEEDS_INITIATIVE` before anything is spent.
The `creature_summoned` event keeps the roll of the first creature only; each combatant keeps its own initiative.
`max_live` refuses a casting that would leave more creatures of the spell than it allows (`CREATURE_LIMIT`).
A summon that replaces its own dismisses the creatures of the old casting of the spell (reason `replaced`), in or out of a combat.
In a combat their combatants leave the order and the map, the room for the new ones counts them as gone, and an undo of the new casting brings the old ones back and removes the new ones.
The line of a dismissal carries the reason the creature was dismissed for.
`GetSummonOptions` is read by the character's owner and the master, and names the table creatures a spell offers, with key and name, before the owner holds one.
In a campaign that forces physical dice, a player cannot cast a spell whose creatures each roll their own initiative (the app's roller is refused and a typed die is one face): the master casts it, who is exempt from the forced mode.
The owner plays the creatures in the same tabs as any summon.
The undo gives the slot back and removes the creatures.

**`GetSummonOptions`** (read, `IDEMPOTENT`).
For a player character it answers what the cast sheet draws: for each summoning spell it may cast (on the sheet, prepared or in the spellbook; the Pact of the Chain gives the Find Familiar ritual) the key, the Portuguese name, the level, the time, whether it is a ritual and needs concentration, `can_ritual` and `can_cast_with_slot`; for each level it can use, the options `rules.SummonOptions` gives (the count already with the level, and the forms or the type and maximum CR); the slots (`SummonSlot`, pact ones with `pact`); and per spell what a casting would send away (the living familiar; the creatures of the current concentration).
It is the same code `CheckSummon` uses (`summonStanding`), so the read and the casting agree; `CastSummon` still checks every choice.
Access is as `ListCharacterCreatures`; no HP goes in the response (RN-20).

## Table creatures

A table creature is an SRD creature plus the master's adjustments: hit points, armor class, a damage bonus, extra text traits, and attacks of its own.
Its numbers can also be formulas of its owner (see "Formula creatures" below).
It is a table entry of kind `creature`, with the key `creature:<slug>@mesa`; the fields and limits are in [Table creatures](../rules-table-content.md#table-creatures).
The creature resolves wherever an SRD one does, so `GiveCreature`, the list, the creature sheet (`GetCreature`) and the combat read it by its key.

- **What is copied and what is read.** `GiveCreature` copies the entry's maximum HP into `character_creatures.hp_max` when the creature is given.
  Editing the entry later changes no creature already held: it keeps the HP it was given.
  For a creature with no formula, the armor class (`CharacterCreature.armor_class`) and the actions come from the content every time, so a held creature shows the entry's current armor class and attacks with its current damage bonus.
- **Formula creatures.** An entry with a formula in any number or attack makes a creature whose numbers depend on its owner.
  When it is given, the owner's sheet is derived with the campaign's content, and `rules.ResolveCreature` runs the formulas with the slot 0.
  The result is kept as JSON in `character_creatures.stat_block`, and `hp_max` is the resolved hit points.
  It is a snapshot, like `hp_max`: a later level of the owner or an edit of the entry's formulas changes no creature already held.
  A creature with a snapshot builds its stat block with `rules.ResolvedDerived`, which takes the numbers from the snapshot and the text (name, traits, attack names) from the content.
  The snapshot's own attacks are matched to the entry's by the key of their action (`<creature key>#<slug>`), not by position: one in both keeps the snapshot's numbers, one new in the entry, or renamed (its key follows its name), has the static numbers (+0 to hit, the static damage bonus), and one the entry lost is dropped.
  A snapshot is used whenever the row has one, even when the entry has lost all its formulas.
  The list (`CharacterCreature.armor_class`), the combat sheet, the turn options, the saving throws and the checks of a held creature read the snapshot.
  A trap that hits a creature outside a combat reads it too.
  `GetCreature` and `ListCreatures` show the content's owner-less stat block, not a snapshot; the creature list's `armor_class` and the combat show the snapshot.
  A creature with no formula has no snapshot, and reads the content as described below.
  "Pôr no combate" and "Criar NPC" have no owner: they use the entry's static fields, and no formula is run.
- **Archived and switched off.** An archived or switched-off entry keeps resolving for the creatures already held.
  They stay in the owner's list and join combats.
  Entries are never deleted, so a held creature never loses its key.
  A player who holds none of an archived or switched-off entry's creatures does not find it.
- **Who sees what (RN-20, RN-23).** In combat the HP is a number for the owning player and the master and the armor class for the master alone.
  Everyone else gets the state word, as for an SRD creature.
  The owner's list (`ListCharacterCreatures`) carries both numbers to the owner and the master, and to nobody else.
  The combatant's name and key are visible to everyone in the combat, as for SRD summons and NPCs.
  The stat block, the HP and the armor class are not.
  This holds when the entry is archived.
  A table creature is the master's, live, archived or switched off.
  A player's `ListCreatures` and `ListTableEntries` lack it, and `GetCreature` answers `not_found`, unless a character of theirs holds a live creature of its key.
  The holder reads it whatever the entry's state.
  The master reads them all.
  `TestRN20_TableCreatureHitPointsOnlyToOwnerAndMaster` and the leak matrix pin this.
- **Where it is used.** A table spell with a `summon` brings table creatures or SRD creatures through the same paths as the SRD's summoning spells (below).
- **Where it is not used.** The SRD's own summoning spells offer SRD creatures only.
  A table creature is not a Wild Shape form.
  The encounter generator, evaluator and swaps leave it out, because it has the challenge rating and XP of its base, which its adjustments may not keep.
  The encounter tools refuse its key as unknown content: `encounterEntries` answers `failed_precondition` and `GetBattleEncounter` lists the key in `unknown_creature_keys`.
- **Giving.** The master can give an archived table creature.
- **Editing and combats.** A held creature with no formula in a combat reads its armor class, attacks and traits from the entry every time.
  An edit of the entry during the fight changes them at once.
  A formula creature keeps its numbers and reads only its text from the entry.
  Its hit points are its own, copied when it was given.
  A monster put in a combat with "Pôr no combate" is a combatant made from the combat's NPC for the creature.
  Its armor class, speed, initiative and sheet attacks are the NPC's.
  Its hit points are the creature's at the time it is added.
  Its traits, saving throws, skills, ability scores, passive Perception, attacks per action and the actions beyond the sheet attacks are read from the entry as it is now.
  The NPC is keyed by the numbers the entry makes (`monsterNpcKeyOf`).
  An edit that changes them makes another NPC, and the monsters already in a fight keep reading the old one.
  An edit that changes only the name finds the NPC it had.
- **Other limits.** The web app has no editor for creatures: they are created through the API or a content pack.
  The master switches them on and off through the API only, because the options screen has no creature group.
  A package battle encounter that names a table creature is refused on import as unknown content.
  `characters_using` counts 0 for a held creature, since it is not part of a sheet.

## What the combat does with a creature

- **Who enters.** The living creatures of a player character enter when the combat is set up (`StartEncounter`), one row each, without initiative and without a square while the owner has no token.
  The owning player (or the master) rolls initiative with `SubmitInitiative`: one roll per summon (creatures with the same `summon_group_id` get the same d20 and total, and the bonus of whoever rolled; Conjure Animals is SRD rule, and the same holds, by our decision, for Animate Dead); the familiar rolls its own.
  The master can take one out of the combat (`RemoveCombatant`), and it stays in the owner's list.
  Whoever leaves a combat in preparation takes the creatures along.
  Those of Conjure Animals cast before combat enter with the owner already concentrating.
  A creature given by the master during a combat does not enter it.
  The player's initiative screen lists a "Suas criaturas" section below the character's roll, one row per casting, with the same ways to roll and the same dice-mode rules.
- **Whose row it is.** `combatants.character_id` is the owner (the column stays `NOT NULL` and the creature disappears with the character, in cascade) and `user_id` the owner's player, so the usual `mayAct` works: the player acts for the creature, and only they and the master.
  `Combatant.mine` stays true only for the player's own character; `controlled_by_me` also covers the creatures, and `owner_character_id` says whose it is.
  `character_id` is empty on a creature, even for the master.
- **Condition immunities.** The conditions a stat block lists as immune (SRD 5.1: the Black Pudding is immune to prone and charmed) travel on the traits of a creature and of an NPC made from it.
  `monsterDerived` copies them into `Derived.StatBlockConditionImmunities`, `basicDerived` does the same for the NPC, and `Content.ConditionImmunities` lists them with the source `stat-block`.
  Every place that gives a condition asks `immuneToCondition`, which reads the sheet of a creature as it reads a character's.
  A spell that lasts also keeps the stat block check of `noEffectWhy`, which knows the humanoid rule too.
  When the stat block refuses a condition of a spell's effect (`giveCondition`), the hit carries `no_effect_why`.
  Only the master reads it, as `SpellTargetResult.no_effect_reason_master` (RN-20, RN-29); a player's view is that of a hit nothing refused.
  A rider, a hold or a trap refused by an immunity keeps its public outcome and carries no reason line.
- **Traits that change rolls.** Pack Tactics and Magic Resistance are read from the stat block's traits and become advantage sources (`combat.SourcePackTactics`, `combat.SourceMagicResistance`).
  Magic Resistance gives advantage on a saving throw against a spell or another magical effect; a creature's own non-spell action is not magic.
  Only the master and the owner of the creature read the mode of its save; any other player reads the outcome.
  Sunlight Sensitivity and Sunlight Weakness are not read: the engine has no sunlight, so they stay a trait the master reads on the creature's sheet.
- **Spells of a stat block** (an NPC, not a summoned creature: a creature has no "Cast a spell", see the next bullet). An NPC made from a creature with a Spellcasting or Innate Spellcasting trait casts through `CastSpell`, like an NPC with a full sheet.
  A creature whose Innate Spellcasting text lists no spells by frequency stays a reminder: the four mephits ("The mephit can innately cast sleep"), which lists no frequency in the data.
  They keep only the description note, and the master casts for them.
  `rules.Content.MonsterCaster` reads the traits: the slots, the prepared spells and the numbers of Spellcasting, and the innate spells as granted spells (at will, or with a resource of the uses a day, one per spell for "N/day each").
  `MonsterCaster` is the only reader of a stat block's casting: `MonsterSpellcasting`, the reaction view (Shield, Counterspell), is a thin view of it.
  `rules.SpellcastingNumbers` is the only reader of the numbers a trait's text writes (level, ability, DC, attack bonus); the NPC's description note prints what it finds.
  A trait with neither a casting ability nor a DC or attack bonus makes no caster.
  A DC with no readable spell is no caster.
  Loading the SRD snapshot refuses a stat block whose Spellcasting or Innate Spellcasting lists a spell the content does not have, or lists spells with no ability, DC or attack bonus (`casterProblem`).
  That check applies to the SRD snapshot only: a pack or table creature cannot carry a Spellcasting or Innate Spellcasting trait.
  `basicDerived` puts them on the NPC's derived sheet, so `CombatTurnOptions` lists them and `GrantedCombatSpell` casts them with the stat block's save DC and attack bonus.
  When the trait lists no DC, it is 8 plus the proficiency bonus and the spellcasting ability modifier.
  The NPC's slots and uses are counted on the combatant (`combatants.slots_used`, the count a reaction's Shield uses), whatever its sheet.
  A cast spends them, the undo gives them back, and an at-will spell spends nothing.
  A monster (the `combat_only` NPC of a creature) starts every combat with an empty count.
  A hand-made NPC keeps its count in `npc_spell_use`: each copy starts a combat with it, each change writes it back as the highest count any copy holds (never the sum), and the table's long rest clears it.
  The master reads the slots and uses left in the options; a player never does.
- **Turn and economy.** The creature has its own turn and economy, computed from `MonsterDerived`: attacks (each action with an attack bonus, the first damage part), Dash and the standard actions, without "Cast a spell".
  The familiar does not attack (no attacks nor the Attack action, even for the master: `CREATURE_CANNOT_ATTACK`); the Pact of the Chain's has its attacks disabled as an action (`REACTION_ONLY`) and only attacks with `as_reaction`; those of Animate Dead and Conjure Animals attack like any creature.
  On entering combat the creature copies, like the character, the side (`party`), the size, the flying speed (`size`, `speed_fly_ft`) and the jump (`combat.JumpLimits` from its Strength).
  Movement uses the better of walking and flying; whoever moves on the fly speed ignores difficult terrain; swimming and climbing are never the speed on the map.
  The owning player moves it on its turn and the master always; another player gets `permission_denied`.
  A dismissed combatant occupies no square and gives no cover.
- **HP and defeat.** HP lives on the combatant, like an NPC's: damage lands at once, without waiting for the master (RN-02 is about character HP), and at 0 HP the creature is defeated (no death saves) and dismissed from the owner's list.
  Every combat write goes through `syncCreatures`, which, after the change and before its event, dismisses the defeated creature and brings back the one an undo or a heal raised.
  At the end of combat, and when it leaves it, the HP goes back to `character_creatures`.
- **Who sees what (RN-20).** The owning player and the master receive the creature's HP as a number (`hit_points_*`); everyone else receives only the state word, in the order, in the log and in the stream (never a `hit_points_*`, nor `hit_points_after`).
  The creature's initiative is public, like the characters'; the owner, the type and the Portuguese name too.
  The group and the permitted attack go only to the owner and the master.
- **Outside combat.** HP belongs to the master (`AdjustCreatureHitPoints`); the creature may have a token on the exploration map, which the master places like any token (below).
- **What undo restores.** The master's undo returns the creature's HP (and brings it back if it had fallen), removes the creatures of a `CastSpell` and returns the slot and the concentration.
  When concentration ends the creatures' combatants are not deleted: they are dismissed (`combatants.dismissed`), out of the order, map and turns, but with the same id, HP, conditions and log lines; undoing the end of concentration removes the mark and they come back.
  `creature_summoned` and `creature_dismissed` are written before the change's event and do not close the undo chain.
  In combat the defeated creature writes no event of its own (the damage that defeated it is its record); outside combat `creature_dismissed` with `reason: defeated` is written.

### Where a creature is not an NPC

Some combat paths ask "does it hold HP on the combatant?" (`holdsHP`: NPC or creature) or "is it on the group's side?" (`inParty`: character or creature); the rest still ask for the kind.

| Where | Creature |
| --- | --- |
| `landDamage`, `healCombatant`, `AdjustCombatantHitPoints`, `readFxTarget`, `dropToZero`, `pendingProto` (`target_defeated`), `UndoLastAction` (damage, heal, spell, HP correction) | `holdsHP`: takes damage and healing at once; `syncCreatures` rebuilds the owner's list |
| `stateOf` | `holdsHP`: Ileso / Ferido / Muito ferido / Derrotado |
| `turnFor`, `EndTurn`, the joint turn's split economy | `inParty`: a group with a creature is the player's, not "NPCs only" (`npcOnlyGroups`) |
| `combatantToProto` | `owner_character_id` in place of `character_id`; public initiative |
| `armorClasses`, portraits, log names | The sheet comes from the creature (`sheetKey`, `sheetOf`): never the owner's AC; portraits are NPCs only |
| `ResetDeathSavesOfCharacter`, `MarkDeathSaveRolledOnTurn` | `AND kind = 'player'`: a creature's `character_id` is the owner's |
| `RollAttack`, `GetTurnOptions`, `TakeAction`, `CastSpell` | The creature's sheet; `mayAttack` applies what the spell left |

Creatures never "fall" (`isDown`), have no death saves or confirmation, cast no Shield, spend no slots, never get `ApplyPendingDamage`, are never hidden, give no XP (only a defeated NPC does), and are not in the highlights for the owner.
Tests: `TestMR037_TheKindAudit`, `TestMR037_ACreatureAtZeroLeavesAndTheHitPointsGoBack`, `TestRN20_CreatureHitPointsOnlyToOwnerAndMaster`, `TestMR037_ACreatureMovesLikeAnyCombatant`, `TestMR037_ADismissedCreatureBlocksNoSquare`, `TestMR037_SummonOptions`, `TestRN20_SummonOptionsCarryNoHitPoints`.

## Legendary Resistance

A creature whose stat block has the Legendary Resistance trait (SRD 5.1, "3/Day") can choose to succeed on a saving throw it fails.
The number of uses a day is the trait's own, read from the stat block (`LegendaryProfile.ResistancePerDay`).
The uses spent are kept on the combatant as `creature_spent.legendary_resistance` ([Data](../../data/combat.md)).
A monster's combat starts with none spent.
The "day" is the rest between combats at the table.
A persistent NPC with a stat block keeps the uses it spent until the table's long rest ([Kept until the long rest](#kept-until-the-long-rest)).
It is a trait, not an action: it works on the creature's own turn, on another's, and while the creature is incapacitated.
Only the master chooses it, and only for a creature that has the trait: a player's character and a creature without it never do.

The saves the master rolls for the creature take the choice with the roll (`combat_legendary_resist.go`):

| Save | Call | Field |
| --- | --- | --- |
| An effect's save, at the start or end of the turn or after damage | `RollEffectSave` | `use_legendary_resistance` |
| The Constitution save to keep a concentration | `ResolveConcentrationSave` | `use_legendary_resistance` |

The master does not need to guess before the dice.
When the roll fails, or the creature fails the save without a roll, the save succeeds instead and one use is spent, in the same change as the save.
When the roll succeeds, nothing is spent and the choice is ignored.
The choice is ignored the same way for a save the master skips or keeps.
The refusals change nothing: `permission_denied` for a player, `LEGENDARY_RESISTANCE_UNAVAILABLE` for a combatant without the trait (before the roll), and `LEGENDARY_RESISTANCE_NONE_LEFT` for a failed save with no use left.
A refusal for no uses left comes after the roll, so the master may roll that window again.
A request sent twice spends once, and its retry answers the same.

**Who sees what (RN-20, RN-29).**
The master reads the roll that failed, the choice and the uses left in the result of the call.
The uses left are also on the master's copy of the combatant.
His log line says it: "falhou no teste (…) e usou Resistência Lendária: sucesso (restam 2)".
A player reads the line of a save that succeeded, the same one a roll that passed writes.
The effect ends or the concentration is kept.
The line has no roll, no count and no name for the trait.
The event is the ordinary save event, with `resisted` and `resist_left`, neutral names that no player-visible payload carries.
The hints are those of a save that passed, so the stream tells a player nothing more.
The save events are not among the undoable kinds ([Undo](undo.md)), so the use stays spent.

**A player's action that makes the creature save** (`combat_legendary_hold.go`).
A spell with a saving throw, on one target or many (`CastSpell`), asks the master first when a creature with the trait and a use left is among its targets, whether its save passed or failed.
The cast is held with the request, in a window of kind `legendary_resistance` that only the master reads and answers (`AnswerReaction`).
The window offers one choice for each failed save of such a creature (none when every one passed): "Usar Resistência Lendária (restam K)", with the d20, the total, the DC and the uses left.
The master chooses the failures to turn (`creature_ids`) or declines (`PASS`).
A window with nothing to offer takes only `PASS`: a `USE` that names a creature is refused and spends nothing.
The hold keeps the d20 and the effect dice of every save of the cast, so the answer settles the cast with the same dice and rolls nothing again.
A turned failure is a save that succeeded, and the cast writes the damage, the conditions and the effects of a success.
Each turned failure spends one use, in the same change as the cast.
An answer that names more than the creature has left is refused with `LEGENDARY_RESISTANCE_NONE_LEFT` and changes nothing.
A request sent twice spends once.
No window opens when no target has the trait with a use left, or when the spell has no saving throw.
The wait shows only that the creature has the trait and a use left.
It lasts the same whatever the save rolled: it ends when none of those creatures is in the combat.
A player sees only the neutral wait ("Esperando o mestre"): not the window, the offer, the roll that failed, the count or the trait.
The wait itself is visible, as it is for a Shield on an NPC, so a player who sees it open for a creature can tell that the master was asked something.
The app does not hide that timing.
The players' line of the cast is a save that passed.
A hold that is dropped (the combat ends, the cast is no longer possible, the kept dice no longer fit the save) spends nothing and leaves no part of the cast.
The undo of the cast gives back the uses it spent, with the slot and the damage ([Undo](undo.md)).

Only the saves of a player's spell are held for the choice.
Not offered, so a failed save stands there: a trap's save, the save of a weapon's hit rider, the Wisdom save against Turn Undead and the Dexterity save against Hellish Rebuke.
The save of a creature a spell zone catches is rolled by the server at once, with no choice for the master; the cast that lays the zone holds nothing.
A player's character caught by a zone is asked in a window of its own, and has no such trait.
The choice the master has with the roll of an effect's save does not reach the saves of a zone.
A spell cast for an NPC is not offered either.

## Limited actions

An action of a stat block with a limit is enforced by the server: a Recharge action, one with uses a day, and one that returns after a rest (`rules.ActionUsage`, SRD 5.1: "Recharge 5-6", "3/Day").
What a creature spent of them is kept on the combatant in `creature_spent`, under `use:<action key>` (N/day), `rest:<action key>` (after a rest) and `recharge:<action key>` ([Data](../../data/combat.md)).
A monster's combat starts with none spent: the "day" is the rest between combats at the table.
Only a creature with a stat block has these limits, so an NPC with no stat block has none.

| Usage | Spent by | Comes back |
| --- | --- | --- |
| Recharge X-Y | The first use | On a d6 the server rolls at the start of the creature's turn, when the face is in X-Y |
| N/Day | Each use, up to N | Never in a combat |
| After a rest | The first use | Never in a combat; a persistent NPC gets it back at a short or long rest |

The spend is part of the use (`combat_action_limit.go`).
It happens in the same change as the attack (`RollAttack`) or the action (`UseCreatureAction`) that uses the action.
A Multiattack spends nothing of its own: each step is its own call and is limited by its own action key.
A spent action is refused with nothing changed.
The master and the player who controls the creature read `CREATURE_ACTION_RECHARGING` for a Recharge action and `CREATURE_ACTION_NO_USES_LEFT` for the others.
The refusal comes before any roll, so the master may use another action.

**The roll at the turn start** (`combat_recharge.go`).
When a creature's turn starts, the server rolls one d6 for each of its spent Recharge actions, in the order of its stat block.
A face in the action's range makes it available again.
A defeated creature does not roll, an action that is available does not, and an action with uses a day or after a rest never does.
Each roll is the event `recharge_rolled`, written secret and with no user and no character, so it is not counted in any player's map revision on a fog map.
Nothing is published for it: the turn start already tells everybody that the combat changed.

**Who sees what (RN-20, RN-29).**
The master and the player who controls the creature read each limited action with what it has left (`Combatant.limited_actions`), the roll in a log line ([Combat log](combat-log.md)) and the refusal reasons.
The controller reads the limits of its own creature and nothing else of the master's NPC data.
Every other player never receives the field, the line, the event or a hint of them: a creature that recharged and one that did not look the same.
Another player has no call that reaches the creature's action.

**What undo does** ([Undo](undo.md)).
The undo of the use gives it back: the action is available again or has the use back.
The roll is not undoable, and it closes the chain like the turn change it belongs to.

**Kept until the long rest.**
A persistent NPC with a stat block is a character of kind NPC that is not `combat_only`, made from a creature.
It keeps its `use:<action key>` and `legendary_resistance` counts from one combat to the next, until the table's long rest.
It keeps its `rest:<action key>` counts until the next short or long rest.
The counts are in `npc_spell_use.creature_spent` ([Data](../../data/characters.md#npc_spell_use)), beside the slots of its spells.
`bumpCreatureSpent` writes them back with `SyncNPCCreatureSpent` after each change of those keys (an undo too): the row keeps the highest count any copy holds in the combat, never the sum, and a key that falls to 0 leaves it.
`SeedCombatantSlotsUsed` gives each copy the row when it joins a combat, and the table's long rest deletes the row; a short rest removes its `rest:` keys ([Rests](rests-and-resources.md)).
`recharge:<action key>` and `legendary_actions` are not kept: a new combat starts with every Recharge action available.
A monster (the `combat_only` NPC of a creature) never has a row and starts every combat with all its actions and uses.
The master reads the uses left as in any combat; a player never reads them (RN-20, RN-29).

**What is not covered.**
A trait with a number a day (Legendary Resistance has its own path above) and a spell's uses are other paths with their own counts.

## Monster reactions

A stat block's reactions come from its `reactions` (`rules.LegendaryProfile.Reactions`, read by `characters.creatureReactionStats` into `ReactionStats.MonsterReactions`).
Parry is the one the engine resolves: the Bandit Captain, Erinyes, Gladiator, Knight, Marilith and Noble add 2 to 5 to their armor class against one melee weapon attack that would hit them.
Every other reaction (Split, Shriek, Rock Catching, Unnerving Mask, the Shield Guardian's Shield...) is text the engine cannot resolve: it opens no window, and the master reads it on the creature's sheet.
Parry goes through the reaction window ([Reactions](reactions.md#parry)), one per round, answered by the master for the creature.
It is the NPC's alone: a player's character made from a stat block gets no Parry window.
The reaction is the creature's `reaction_used` column, the same as a player's: spent by the answer, back at the start of the creature's own turn.
Nothing is kept in `creature_spent`.

**Who sees what (RN-20, RN-29).**
Only the master reads the window, the bonus, the armor class with it and that the creature has the reaction.
A player reads the neutral wait ("Esperando o mestre") while it is open, as for any NPC that may react.
The wait itself is visible, so it tells a player that the creature may have a reaction; the app does not hide that timing.
After the answer a player reads an ordinary attack line: a miss when Parry turned the hit, a hit when it did not, with no armor class and no word of a reaction.
The master's line says "<name> aparou (+N CA)" with the total and the armor class.

## On screen

The browser reads the list with `ListCharacterCreatures`, the sheets with `ContentService.GetCreature` (kept in memory; public rules), the master's catalogue with `ListCreatures` and what the sheet can summon with `GetSummonOptions`, and casts with `CastSummon`.
The `creatures_changed` hint makes the panel and the master's list read again (the same stream client as `xp_changed`, only while a session is open and when the stream comes back).
The sheet's panel also reads the druid's Wild Shape form again on `vitals_changed` and `encounter_changed`, because a form ends by damage, sleep or the master's hand without any creature changing.
The browser holds no summon rule.
In combat it gathers what the player plays (`Combatant.controlled_by_me`) into tabs (`core/combat/mine.ts`: the character first, then each summon by `summon_group_id`, or a lone creature) and reads each creature's options with `GetTurnOptions` (`CreatureOptionsState`); attacking and moving use the same flow and Move page as the character (`moverId`).
"Encerrar a parte dos Lobos" is one `EndTurn` per member still acting.
The server does not send a creature's or beast's AC (RN-20): the screen reads the book's (`GetCreature`).
HP pools come from `Combatant.wild_shape_hit_points_*` (owner and master only) and the uses from `CharacterVitals.resources`.
The damage that passes from the beast to the druid comes from the server in the combat log line (`COMBAT_LOG_KIND_WILD_SHAPE`, `CombatLogEntry.wild_shape`, `CombatLogWildShape`: the beast, whether it started or ended, the reason `WildShapeEndReason`, `carried_damage`); the number goes only to the druid's player and the master, and the others read "voltou à forma normal".
The player's notices (`core/combat/combat-notices.ts`: `FormNotices`, `ConcentrationWatch`, `wildActionLine`) read that line and say the reason and the number; what the log already had when the combat was read is not news.
The master's "Perdeu a concentração" question is offered only to whoever has creatures of a summon (`summon_group_id` with the owner).
The Portuguese action names (`attack:<slug>` in `names_pt.json`, `Attack.name_pt`, `CreatureAction.name_pt`, `action_name_pt` of the log) mean the browser translates no name; only the SRD's usage text ("Recharge 5-6") is translated there, by `usageText`.

## Wild Shape

The druid's form is part of the vitals for the API (`CharacterVitals.wild_shape`) and has its own table `character_wild_shapes`: one row per character in beast form, with the beast key (`monster:wolf`) and its current HP, always 1 or more (at 0 the form ends and the row disappears); changing the form raises the vitals `revision`.
It is a separate table because of locking: a call such as Shield's answer spends a slot (writes the vitals row) and then reads the sheet, through another connection, for the AC, and that read joins the form; with the form in the vitals row the read would wait for its own transaction's write.
The beast's maximum is never stored; it comes from its sheet (`MonsterDerived`).
The druid's own HP (`hit_points_current`) stays still and only takes the damage that remains when the beast falls.
The table's `wild_shape` effects (`Derived.WildShape`: the highest challenge rating, the lowest cost, magical attacks if any gives them) set the limit, the cost (`link.Character.WildShapeCost`, which `shapeApply` spends and `shapeUndoStart` gives back from the event's `PointsSpent`) and whether the beast's attacks count as magical; `ListWildShapeForms`, `WildShapeAllows` and the choice read the same limit.
`characters` is the only place that reads the form back into the character's numbers: `Service.derive` calls `rules.WildShapeDerived` (AC, attacks, speeds, senses and the beast's Strength, Dexterity and Constitution; no spells), and that is what the combat sheet, turn options, `combatCharacter` (speed, size, flying, jumps) and `PartyVision` use.

| Call | Who | What it does |
| --- | --- | --- |
| `CharacterService.ListWildShapeForms` | The master and the owning player | The beasts Wild Shape allows now (`rules.WildShapeForms`: level 2, CR up to 1/4 without flying or swimming; 4, up to 1/2 without flying; 8, up to 1, or the highest rating a table feature's `wild_shape` effect gives), by Portuguese name, with the limit that decided the list. Empty for non-druids |
| `PlayService.AssumeWildShape` | The owning player and the master | In combat or out, with the session open. Spends the cost in uses of `wild_shape` (1, or the lowest `form_cost` of the table's `wild_shape` effects, which may be 0; `NO_USES`; at druid level 20 it has 99, unlimited) and, in combat, the action, on the character's turn (the master may act again with the action spent). Typed refusals: `WILD_SHAPE_BEAST_NOT_ALLOWED`, `ALREADY_IN_WILD_SHAPE`, `ACTION_USED`, `NOT_YOUR_TURN`. The sheet's action (`feature:wild-shape...`) does not go through `TakeAction` (it refuses it with `invalid_argument`). Writes `wild_shape_started` |
| `PlayService.LeaveWildShape` | The owning player and the master | "Voltar à forma normal": bonus action on the character's turn (the master's spends nothing). `NOT_IN_WILD_SHAPE` in own form. Writes `wild_shape_ended` (`reason: left`) |
| `PlayService.AdjustCharacterVitals` | The master | `wild_shape_hit_points_current` (1 up to the beast's maximum; 0 ends the form, `wild_shape_ended` with `reason: master`). Without a form it is `invalid_argument` |

- **Damage on the beast, healing, what is left over.** The damage the master applies (`ApplyPendingDamage`) falls on the beast's pool, without temporary HP (`damageBeast`); if the beast falls to 0 the form ends, the combatant goes back to the character's numbers and what was left over passes to the druid by the usual path (temporary HP first, `changeVitals`, the death saves).
  Healing a druid in the form (`healCombatant`: a spell, Second Wind) heals the beast, up to its maximum.
  `wild_shape_ended` with `reason: damage` and `carried_damage` is written before the damage event and does not close the undo chain.
  The beast does not take temporary HP.
- **In combat.** On assuming or leaving the form the combatant swaps `speed_ft`, `speed_fly_ft`, `size` and the jumps (`SetCombatantBody`), so movement uses the beast's size and speed; a combat that starts with a druid already in the form puts them in with the beast's numbers.
  The turn options come from the beast's sheet: its attacks (`monster:wolf#bite`), the standard actions, no spell.
  `CastSpell` and `CastSummon` are refused with `WILD_SHAPE_NO_SPELLS` before spending anything, except for a druid of level 18 or more (Beast Spells: `link.Character.CastsInBeastForm`).
  `Combatant` carries `wild_shape_beast_key` and its Portuguese name for everyone who sees it, and the beast's HP (`wild_shape_hit_points_*`) only for the master and the owning player (RN-20).
  The druid goes back to their own form when falling to 0 HP or unconscious (SRD): `changeVitals` ends the form whenever the druid's own HP reaches 0, and giving the Unconscious condition also does (`endFormIfAsleep`); each end writes `wild_shape_ended` (`reason`: `zero_hp` or `unconscious`) before the change's event, without closing undo.
  `AssumeWildShape` on a druid at 0 HP or unconscious is `COMBATANT_DOWN`.
  Healing a druid at 0 HP heals the druid.
- **Undo.** The `hpState` of HP events carries the form (`shape`: the beast and its HP), and `putVitals` puts the form back with the HP and the combatant's numbers.
  The undo of the action that began the form ends it and returns the use, the action and the dash; the undo of "Voltar à forma normal" puts the beast back with the HP it had.
  Undoing the damage that took the druid to 0 puts the form back; undoing the condition does not.
- **Events** carry ids, keys and numbers: `wild_shape_started` (`character_id`, `beast`, `resource`, and what undo restores) and `wild_shape_ended` (`beast`, `reason`: `left`, `damage`, `master`, `zero_hp`, `unconscious`; `carried_damage`).

## The familiar's eyes (MR-036)

The player of a character with a familiar (the creature of origin `familiar`) sees what the familiar sees, with its senses (the owl's darkvision, the bat's blindsight), while it is on the same map and 30 m or less away (20 squares, the RN-21 circle).
The state lives in the vitals (`familiar_sight_creature_id`, `familiar_sight_in_combat`, `familiar_sight_conditions`) and only holds while the familiar is alive and with the character.

| Call | What it does |
| --- | --- |
| `PlayService.StartFamiliarSight` | Checks the familiar (`FAMILIAR_SIGHT_BLOCKED`: `NO_FAMILIAR`, `NOT_ON_MAP`, `TOO_FAR`, `ALREADY_SEEING`, `NO_MAP`, in `familiar_sight_reason`). Outside combat it lasts until the player stops, and ends when the character enters a combat or the combat starts (`reason: combat`; in a combat in preparation, `COMBAT_NOT_BEGUN`); the positions are those of the tokens on the current map (it needs a grid). In combat it spends the action, on the character's turn, the positions are the combatants', and it ends at the start of their next turn (`startTurn` calls `endFamiliarSights`, `reason: turn`) or with the end of the combat (`endCombatSights`, no event). While it lasts the character is blind and deaf and the player sees only what the familiar sees (the character's own view leaves the calculation; with "Visão do grupo" the other characters' views still count). In combat the combatant gains `blinded` and `deafened` (only those it did not already have, kept in `conditions_given`: ending removes only those); outside it the notice is the `FamiliarSightState` in the vitals. It is a reminder for the master, like every condition (RN-22) |
| `PlayService.StopFamiliarSight` | The player stops whenever they like: removes the conditions and does not return the action. `NOT_SEEING` if not seeing |

The master's undo undoes only the start in combat (returns the action and removes the conditions); the end is not undone.
On the map, `PartyVision` sends `link.PartyMember.Eyes` (the creature and its senses) and `maps` adds one more observer at the square where the creature is (its token, or the combatant while a combat runs: `CombatPositions.Creatures`), only while it is within 30 m of the character (`familiarEyes`): beyond that the familiar's sight leaves, the player sees nothing new (the character is still blind) and what was seen is remembered.
`play` notifies the fog (`MapKeeper.VisionChanged`, after the commit) on assuming or leaving the form and on starting or stopping the sight.
If the familiar is dismissed with the eyes on outside a combat, the sight ends by itself; in a combat, dismissing it ends the sight in the same transaction (`CreaturesLeaving`), the conditions it gave go back, the vitals forget it and a `familiar_sight` stop event has the reason `gone`.
Only the familiar, and only while its eyes are on, sees through the owner; the other creatures appear to the group (their token belongs to the group, never hidden) but see through nobody.

**Idempotency keys and sheet reads.** An idempotency key belongs to whoever used it: `write()` (and the scene roll) refuses with `invalid_argument` a key repeated by another member, and the Wild Shape and familiar calls also refuse a key repeated for another character.
`CombatSheet`, `CombatTurnOptions` and `CombatSpell` (`play.CombatRoster`) receive the caller's transaction (`nil`: the pool): a change that wrote the vitals or the form and then reads the sheet must read it inside the transaction, or the read waits for its own transaction's write.
When the form or the familiar's sight changes by any path, `write()` publishes the vitals and notifies the fog after the commit (`combatTx.told`).

## Creature tokens

`map_creature_tokens` (module `maps`) stores `(map_id, creature_id, x_bp, y_bp)`, one token per creature per map, in its own table because `map_tokens` is keyed by `(map_id, character_id)` and a character has many creatures.
The master places and moves with `PlaceMapToken` (`creature_id` in place of `character_id`) and removes with `RemoveMapToken`; a player gets `permission_denied`, and `character_id` and `creature_id` together or neither is `invalid_argument`.
A creature's `MapToken` carries `creature_id`, the owner's `character_id`, the creature's name, `hidden` always false and never carried light.
It is a group token: every player who sees the map receives it, with fog or without, even on a black square (`creatureTokensOf`); moving one is `map_changed`, not `token_moved`, and under fog it notifies the view.
A dismissed creature, or one whose owner is dead, is not listed (`CharacterDirectory.MapCreatures`); the token stays in the table and disappears with the creature or the map (`CASCADE`).
At the end of combat the creature combatant's position goes back to its token, if it has one (`SetTokenPositions` with `CreatureID`); combat does not create a token.
