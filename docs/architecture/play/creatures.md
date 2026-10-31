# Creatures, Wild Shape and the familiar

A creature belongs to a character, and in combat it is a combatant of kind `creature` (MR-037).
The rules (what a spell lets you summon, a creature's numbers) belong to `rules` (see [Architecture](../rules-creatures-items.md#creatures)); this page is what the server stores and does with them, plus Wild Shape and the familiar's eyes (MR-036).

The `characters` module stores the creature (`character_creatures`); `play` puts it in combat (`combatants.kind = 'creature'`).
The two meet in two small interfaces wired in `cmd/api`: `play.CombatRoster` (what combat asks of `characters`) and `characters.CreatureHost` (what `characters` asks of `play`, set with `SetCreatureHost`).

| Who | What |
| --- | --- |
| `CharacterService` | `ListCharacterCreatures` (the master and the owning player; any other player gets `not_found`, and the HP never reaches them), `GiveCreature` (master only, any SRD creature, up to 40 per character; it takes the open session's lock before the character's, the order every combat write uses), `RenameCreature` (its combatant in an open combat gets the new name), `DismissCreature` (the owner or the master; dismissing one already dismissed changes nothing), `AdjustCreatureHitPoints` (master only, RN-02, only outside combat; at 0 HP the creature is dismissed), `GetSummonOptions`, `ListWildShapeForms`. The creature's sheet is `ContentService.GetCreature` |
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

**`GetSummonOptions`** (read, `IDEMPOTENT`).
For a player character it answers what the cast sheet draws: for each summoning spell it may cast (on the sheet, prepared or in the spellbook; the Pact of the Chain gives the Find Familiar ritual) the key, the Portuguese name, the level, the time, whether it is a ritual and needs concentration, `can_ritual` and `can_cast_with_slot`; for each level it can use, the options `rules.SummonOptions` gives (the count already with the level, and the forms or the type and maximum CR); the slots (`SummonSlot`, pact ones with `pact`); and per spell what a casting would send away (the living familiar; the creatures of the current concentration).
It is the same code `CheckSummon` uses (`summonStanding`), so the read and the casting agree; `CastSummon` still checks every choice.
Access is as `ListCharacterCreatures`; no HP goes in the response (RN-20).

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
`characters` is the only place that reads the form back into the character's numbers: `Service.derive` calls `rules.WildShapeDerived` (AC, attacks, speeds, senses and the beast's Strength, Dexterity and Constitution; no spells), and that is what the combat sheet, turn options, `combatCharacter` (speed, size, flying, jumps) and `PartyVision` use.

| Call | Who | What it does |
| --- | --- | --- |
| `CharacterService.ListWildShapeForms` | The master and the owning player | The beasts Wild Shape allows now (`rules.WildShapeForms`: level 2, CR up to 1/4 without flying or swimming; 4, up to 1/2 without flying; 8, up to 1), by Portuguese name, with the limit that decided the list. Empty for non-druids |
| `PlayService.AssumeWildShape` | The owning player and the master | In combat or out, with the session open. Spends one use of `wild_shape` (`NO_USES`; at druid level 20 it has 99, unlimited) and, in combat, the action, on the character's turn (the master may act again with the action spent). Typed refusals: `WILD_SHAPE_BEAST_NOT_ALLOWED`, `ALREADY_IN_WILD_SHAPE`, `ACTION_USED`, `NOT_YOUR_TURN`. The sheet's action (`feature:wild-shape...`) does not go through `TakeAction` (it refuses it with `invalid_argument`). Writes `wild_shape_started` |
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
