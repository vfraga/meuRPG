# How a bestiary monster becomes a combatant (p3d, for the trait batches)

Paths are under backend/internal. A bestiary monster joins as kind NPC (kindNPC) with a BASIC sheet, not a full sheet.

## Build (stat block to sheet)
- characters/combatmonsters.go MonsterNpc, monsterNpcParams: the master adds a monster; play/combat_monsters.go AddMonsters.
- characters/npcfromcreature.go npcSheetFromCreature: copies HP, AC (printed number), XP, size, MonsterKey. Speed is the WALK speed only (best other speed when walk is 0, :146). At most 3 valid attacks (maxNpcAttacks :132, :169; die must be d4-d12, basicAttackOf :225); extra damage clauses go through extraDamage.
- The basic sheet has no swim/climb/burrow/fly fields (characters/combatants.go combatCharacter basic case :178-185); play/combat_join.go joinRow:255 copies them from the sheet, so they stay 0.
- characters/combatturn.go basicDerived (:108-160) rebuilds rules.Derived each time: attacks as basic:N WITHOUT Magical/Silvered/Adamantine (no field in proto BasicAttack), AttacksPerAction from MonsterDerived, SavingThrows, Skills, Abilities, StatBlockConditionImmunities, Features, MonsterCaster (Spellcasting/Innate Spellcasting become real spells and resources).

## What the engine reads from the stat block (rules.Content)
- Creature/MonsterDerived (rules/creatures.go creature(), monsterAttack:862 first damage clause only, monsterSenses:679, armorClassNote:647). Features from SpecialAbilities (:735+) are read by KEY SUFFIX only: #pack-tactics, #magic-resistance, #sunlight-sensitivity, #sunlight-weakness, #evasion (characters/combattraits.go traitsOf), plus Magic Weapons (rules/creatures.go hasMagicWeapons:855, used only on the rules.Attack at :872, which basicDerived drops) and Legendary Resistance (rules/legendary.go ReadLegendaryProfile). Every other trait is text on the stat block.
- Actions: rules/monsterplan.go planAction builds an ActionPlan per action by regex over the SRD text. Class structured/partial/text; kind attack/save/multiattack/other; usage at_will/recharge(min face)/per_day/rest (parseUsage:276). failedSaveCondition:377 takes the first condition word after "saving throw" in a sentence with a save (misreads swallow, cockatrice). Multiattack: planRoutines, correctAttacksPerAction (rules/content.go:1385).
- Reactions: only Parry is resolved (rules/legendary.go monsterReaction; play/combat_parry_auto.go autoParry, characters/combatreactions.go creatureReactionStats:46-66). Others are text (RN-29).
- Legendary: rules/legendary.go (3 a round, options with cost); play/combat_legendary*.go; Legendary Resistance play/combat_legendary_resist.go legendaryResistOpen (callers: lasting_effects_save.go:333, combat_concentration.go:321, combat_legendary_hold.go:359; NOT traps).

## Using an action in combat
- Attacks: play/combat_attack.go RollAttack (routineLeft:459, checkLimit); damage origin play/combat_damage.go originOf:505-540 (a basic-sheet attack is Magical only if the sheet says; a stat-block save action counts as an attack for resistances).
- Save and other actions: play/combat_creature_action.go UseCreatureAction (reuses the spell machinery); plan in combat_creature_plan.go planOfAction/planOfAttack/spellOfPlan; rider after a hit (Aplicar/Dispensar, RN-40) in combat_creature_rider.go (riderOffered:29, grapplingRider:93; the plan's MaxSize is never checked).
- Limits live in combatants.creature_spent JSON: legendary_actions, legendary_resistance, use:<key>, rest:<key>, recharge:<key>, parry_ask (play/combat_spent.go, combat_action_limit.go limitKeyOf:22, one key per plan, so sub-breaths do not share). use:/rest: persist to the long rest (keptUntilLongRest; characters/resources.go:413-421); recharge and legendary actions reset each combat.
- Recharge: play/combat_recharge.go rechargeAtTurnStart/rollRecharge, a d6 per spent action at the creature's turn start, stat block order.

## Defenses and senses
- Damage: rules/combat/steps.go Adjust = immunity, then resistance (half, round down), then vulnerability (double). Modifiers from characters/charactercreatures_roster.go DamageModifiers/typeModifiersOf:768/damageTypesOf:780 (a plain entry with a Note, such as "damage from spells", is SKIPPED :783). Qualifier clauses (nonmagical, unless silvered/adamantine) are decided by Origin from originOf.
- Condition immunities: play/condition_immunity.go immuneToCondition (callers: spells giveCondition combat_spells_hp.go:352, riders, traps, zones, contests).
- Senses: characters/partyvision.go sensesOf:80 keeps darkvision, blindsight, truesight, devils-sight (no tremorsense) for vision.Senses; used by reaction sight (play/combat_reaction_counterspell.go reactorSees:62, eyesCannotSee:82 makes a blinded reactor or invisible actor unseen even with blindsight/truesight), fog (combat_fog.go:294), traps noticing (combat_traps.go:677).
- Speeds: play/combat_speed.go speedsOf, combat_move.go flies/moverOf/speedFtOf; master drags an NPC past its movement with "Mover mesmo assim" (RN-21).

## Screens
- web live-session/combat/npc-card (npc-card.html, multiattack.ts, creature-actions.html): Multiattack routines, "Outras ações" with SRD text and limit tags, recharge notice, legendary card. Full stat block: bestiary page via "Ficha da criatura" (shared/creatures/stat-block-view.ts).
