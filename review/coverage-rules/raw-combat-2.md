# raw-combat-2 (Combat part 2: attacks, cover, damage and healing, mounted, underwater)

MECHANIC: making-an-attack / attack-rolls: d20 + modifiers reaches AC = hit
SERVER: backend/internal/play/combat_actions.go:728-734 (targetAC = sheet AC + AcBonus + cover bonus; ResolveAttackFrom total vs AC)
SCREEN: web/src/app/pages/live-session/combat/attack-sheet/attack-result.ts:86 (shows "Acertou"/"Errou" and cover degree, never the AC)
DOCS: docs/product/rules.md:226 "RollAttack compares the d20 with the AC on the server"
TESTS: backend/internal/rules/combat/rolls_test.go:8 TestResolveAttackFromCriticalRange

MECHANIC: making-an-attack / attack-rolls: natural 20 always hits (critical), natural 1 always misses
SERVER: backend/internal/rules/combat/rolls.go:39-52 (face >= criticalFrom hits+crit; face==1 Fumble, never hits)
SCREEN: web/src/app/pages/live-session/combat/attack-sheet/attack-sheet.ts:314 (critical line for physical dice)
DOCS: docs/product/rules.md:353 "a natural 1 always misses", which is already SRD (stated as SRD, not added by app)
TESTS: backend/internal/rules/combat/rolls_test.go:8

MECHANIC: making-an-attack / attack-rolls: ability modifier (STR melee, DEX ranged, finesse/thrown)
SERVER: backend/internal/rules/attacks.go:45-53 (STR default, DEX if ranged; finesse or monk weapon picks the better); attacks.go:139-141 (unarmed uses STR, DEX with Martial Arts)
SCREEN: no match for "modificador de atributo" on attack sheet (not traced further)
DOCS: no match
TESTS: no match by name

MECHANIC: making-an-attack / attack-rolls: proficiency bonus added to attack roll
SERVER: backend/internal/rules/attacks.go:62-65 (bonus += prof only when weaponProficient); attacks.go:146 (unarmed always proficient)
SCREEN: no match in attack-sheet for proficiency text
DOCS: no match
TESTS: no match by name

MECHANIC: making-an-attack / attack-rolls: monster attack uses its stat-block modifier
SERVER: backend/internal/play/combat_actions.go:643-651 (attacker sheet's Attacks[attack_key].ToHit used as is); origin of NPC sheet not traced (unsure)
SCREEN: no match
DOCS: docs/product/rules.md:417 (monster enters combat as NPC with SRD creature key) - not a modifier claim
TESTS: backend/internal/rules/creatures_multiattack_test.go:60 (multiattack text only)

MECHANIC: making-an-attack / unseen-attackers-and-targets: attacking an unseen target has disadvantage
SERVER: no match for disadvantage; RollAttack rolls one d20 (play/combat_spells.go:582-595 d20(); combat_actions.go:720). Player's attack on an unseen combatant is refused as not found: combat_actions.go:591-604 (findCombatant with fog viewer)
SCREEN: no match for "desvantagem" in web/src/app/pages/live-session/combat and web/src/app/core/combat
DOCS: no match (no doc claims disadvantage for unseen targets)
TESTS: combat_fog_test.go:757 TestRN10_FogCombatCoverIsToldOnWhatThePlayerKnows (fog, not disadvantage)

MECHANIC: making-an-attack / unseen-attackers-and-targets: attacker has advantage vs unseen/unable-to-see target
SERVER: no match for advantage in play/ or rules/combat/ (no roll_mode applied to combat attacks; characters/ no "advantage" mode in combat turn)
SCREEN: no match
DOCS: docs/product/rules.md:353 "flanking, which asks for advantage on attacks (the app does not apply it yet)" - deliberately not built
TESTS: no match

MECHANIC: making-an-attack / unseen-attackers-and-targets: wrong square guess = automatic miss; hidden attacker gives away position
SERVER: no match for location-guess miss; hidden NPC: combat_actions.go:962-963 (target hidden after the hit: master resolves); combat.go:926-971 SetCombatantHidden (master hides NPC, not a hidden-attacker reveal rule)
SCREEN: web/src/app/pages/live-session/combat/order-list/order-list.html:164 "Esconder dos jogadores" (master hides a combatant)
DOCS: docs/product/rules.md:128 (map hidden points; not attack rule)
TESTS: no match

MECHANIC: making-an-attack / ranged-attacks: beyond normal range = disadvantage
SERVER: no match for disadvantage. Range check uses only max(range, long range) as reach: play/combat_actions.go:181-183 reachFt; combat_actions.go:686-690 (refuses target beyond reach, no disadvantage)
SCREEN: web/src/app/core/combat/attack-flow.ts:111 ("Longe demais: alcance de X" - target shown as blocked)
DOCS: no match
TESTS: no match by name

MECHANIC: making-an-attack / ranged-attacks: cannot attack beyond long range
SERVER: play/combat_actions.go:187-191 attackReach (= reachFt of range and long range); combat_actions.go:686-690 refuses dist > reach with TARGET_OUT_OF_REACH ("beyond the attack's range"), skipped for an opportunity offer (687)
SCREEN: web/src/app/core/combat/combat-errors.ts:107 ("Longe demais: faltam X para chegar ao alvo.")
DOCS: docs/product/rules.md:323 is about movement; no doc for long range rule (no match)
TESTS: combat_opportunity_test.go:889 TestRN21_AReactorsReachIsItsLongestMeleeReach (reach, not long range)

MECHANIC: making-an-attack / ranged-attacks: ranged attack within 5 ft of hostile who sees you = disadvantage
SERVER: no match for "5 ft" disadvantage in play/ or rules/combat (no hostile-adjacency check on ranged attacks)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: making-an-attack / melee-attacks: reach 5 ft (and greater reach of creatures)
SERVER: play/combat_actions.go:45-46 (meleeReachFt = 5); combat_actions.go:181-196 (reachFt, attackReach, distanceFt 5 ft per square); combat_actions.go:681-691 (player must be placed; dist > reach refused, not theatre)
SCREEN: web/src/app/core/combat/attack-flow.ts:111 (target out of reach shown as blocked)
DOCS: docs/product/rules.md:29 RN-21 (grid squares 1.5 m; circle of movement)
TESTS: combat_opportunity_test.go:889 TestRN21_AReactorsReachIsItsLongestMeleeReach

MECHANIC: making-an-attack / melee-attacks: unarmed strike, 1 + STR mod bludgeoning
SERVER: backend/internal/rules/attacks.go:133 (unarmedReachFt = 5); attacks.go:143-153 (Key attack:unarmed-strike, Melee, proficient, DamageType damage-type:bludgeoning, Damage = max(1+mod, 0) flat; Martial Arts die if monk)
SCREEN: no match for "Ataque desarmado" text in attack screens (not traced)
DOCS: docs/architecture.md:1847 mentions unarmed strike only for Martial Arts/Flurry
TESTS: no match by name

MECHANIC: making-an-attack / melee-attacks: opportunity attack (reaction, when hostile leaves reach; Disengage and teleport/forced moves exempt)
SERVER: play/combat_opportunity.go:20-24 (rule: move on foot leaving reach of hostile offers one attack; master offers); combat_opportunity.go:62-80 (meleeReachOf); combat_actions.go:573-574 (asReaction), 659-663 (must be melee), 686-687 (no reach check for offer); combat_actions.go:1806-1808 (standard:disengage sets Disengaged); combat_move.go:273-276 (forced move is master only)
SCREEN: web/src/app/core/combat/move-plan.ts:140 ("Sair do alcance ... pode provocar um ataque de oportunidade."); web/src/app/pages/live-session/combat/move-page/move-status.ts:54 ("Com Desengajar, nenhum movimento deste turno provoca isso.")
DOCS: docs/product/rules.md:313 (forced move and master placement do not provoke); docs/product/rules.md:323 ("Sair do alcance do Goblin 2 pode provocar um ataque de oportunidade")
TESTS: backend/internal/play/combat_opportunity_test.go:194 TestRN21_WhoProvokesAnOpportunityAttack; combat_opportunity_test.go:679 TestMR034_OpportunityAuthorizationMatrix

MECHANIC: making-an-attack / melee-attacks: two-weapon fighting (light weapon in each hand, bonus action, no ability mod to off-hand damage unless negative or style)
SERVER: play/combat_actions.go:788-793 (off-hand: combat.OffHandBonus unless TwoWeaponFighting style); rules/combat/bonusattack.go:67 (both last and next must be Melee and Light); bonusattack.go:76-82 (OffHandBonus: keeps mod only if style or mod<=0); play/combat_bonus.go:43 attackEconomy
SCREEN: no match for "duas armas" in web (only "Ação bônus" economy group in design.md:225)
DOCS: docs/product/rules.md: no match; docs/architecture.md:1847 (Two-Weapon rules not listed as unbuilt)
TESTS: backend/internal/play/combat_bonus_attacks_test.go:175 TestTwoWeaponFightingAttacksWithTheBonusAction; backend/internal/rules/combat/bonusattack_test.go:44 TestOffHandBonus
NOTE: thrown-weapon alternative for two-weapon (throw instead of melee) - no match for "thrown" in rules/combat/bonusattack.go

MECHANIC: making-an-attack / melee-attacks: grappling (Athletics/Acrobatics contest, grappled condition)
SERVER: no match for grapple contest or Athletics check in play/ or rules/combat. Condition only: play/combat_move.go:141-146 (noSpeed list includes condition:grappled; comment says grappled sets speed 0)
SCREEN: web/src/app/core/combat/conditions.ts:17 ({key:'condition:grappled', name:'Agarrado'} - label only)
DOCS: docs/product/rules.md:281 ("A condition that sets the speed to 0 (grappled"); docs/product/stories.md:550 TestRN25_ConditionsThatLeaveNoSpeed
TESTS: backend/internal/play/combat_move_test.go:1380 (grappled has no speed); backend/internal/play/combat_theatre_test.go:1092 TestRN25_ConditionsThatLeaveNoSpeed

MECHANIC: making-an-attack / melee-attacks: escaping a grapple (grappled creature's action)
SERVER: no match (no escape action, no contest)
SCREEN: no match
DOCS: review/coverage-rules.md:23 (already noted by master's sweep: contests absent) - not docs/product
TESTS: no match

MECHANIC: making-an-attack / melee-attacks: moving a grappled creature (speed halved unless 2+ sizes smaller)
SERVER: no match (dragging not modeled; grappled speed is 0 per combat_move.go:141-146; no halving rule)
SCREEN: web/src/app/pages/live-session/combat/move-page/move-page.ts:435 is "agarrar uma borda" (grab a ledge while jumping), not grapple - not a match
DOCS: no match
TESTS: no match

MECHANIC: making-an-attack / melee-attacks: shoving (prone or push 5 ft, contest)
SERVER: no match for shove/empurrar as an attack. Push only as forced move: play/combat_move.go:273-276 (forced = master teleport/push)
SCREEN: web/src/app/pages/live-session/combat/combat-map-card/combat-map-card.html:109 ("Para teletransporte (Passo Nebuloso), empurrão ou puxão: o token chega ao quadrado sem provocar ataque") - a forced move, not a shove
DOCS: docs/design.md:300 ("Movimento forçado": teleport, push or pull, no opportunity attack)
TESTS: no match

MECHANIC: cover / half cover: +2 to AC and Dex saves, obstacle blocks at least half
SERVER: play/combat_cover.go:78-86 (bonus() = 2 for half); combat_cover.go:55-60 (largest degree wins); combat_actions.go:728 (added to AC); rules/grid/cover.go:11 CoverBetween (line of squares); combat_cover.go:48 (creatures between = half cover; hidden ones ignored for players)
SCREEN: web/src/app/pages/live-session/combat/order-list/cover-mark.ts:75 ("Meia cobertura", sub "+2 na CA e em Destreza"); web/src/app/pages/maps/cover-degrees/cover-degrees.ts:20 ("+2 na CA e nos testes de resistência de Destreza")
DOCS: docs/product/rules.md:250 ("Cover never leaks the AC (MR-034)"); docs/product/stories.md:554 (no cover, half +2, three-quarters +5, total)
TESTS: backend/internal/play/combat_move_test.go:753 TestMR034_CoverRaisesTheArmorClassOfTheTarget; backend/internal/rules/grid/move_test.go:254 TestCoverBetweenAgainstTheCave

MECHANIC: cover / three-quarters cover: +5 to AC and Dex saves
SERVER: play/combat_cover.go:78-86 (bonus() = 5 for three-quarters); rules/grid/cover.go:11 CoverBetween
SCREEN: web/src/app/pages/live-session/combat/order-list/cover-mark.ts:76 ("Três quartos", "+5 na CA e em Destreza")
DOCS: docs/product/stories.md:798 (half: low wall, crates; three-quarters: column, arrow slit)
TESTS: backend/internal/rules/grid/door_test.go:228 TestCoverFromDoors

MECHANIC: cover / total cover: cannot be targeted directly
SERVER: play/combat_cover.go:89 (total()); combat_actions.go:714-716 (player refused with errCoverTotal; master may override); combat_cover.go:125-127 (error)
SCREEN: web/src/app/pages/live-session/combat/theatre/cover-panel.ts:29 ("Total (it cannot be aimed at)")
DOCS: docs/product/rules.md:250 (cover degree shown to player)
TESTS: backend/internal/play/combat_move_test.go:1141 TestMR034_SacredFlameGetsNoCover (no-cover case, not total)

MECHANIC: cover / degrees do not add together (most protective applies)
SERVER: play/combat_cover.go:16 (comment: degrees never add); combat_cover.go:54-60 (switch: mark > mapCover, else map; max, not sum)
SCREEN: no match for explicit "not added" text
DOCS: no match
TESTS: no match by name

MECHANIC: damage-and-healing / hit-points: current HP, damage subtracted, no effect until 0
SERVER: rules/combat/vitals.go:18-28 ApplyDamage (temp first, then HP, floor 0); combat_actions.go:1151-1158 (NPC/creature HP on combatant row); combat_actions.go:1405-1413 (player HP via vitals)
SCREEN: web/src/app/core/combat/hp-effects.ts:50 ("ganha N PV temporários")
DOCS: docs/product/rules.md:56 (current HP, temporary HP stored; master corrects)
TESTS: backend/internal/rules/combat/combat_test.go:127 TestApplyHeal (heal side)

MECHANIC: damage-and-healing / damage-rolls: weapon adds ability mod once; spell damage rolled once for all targets
SERVER: play/combat_actions.go:994 (expr = dice + DiceBonus + CriticalMax, rolled once); combat_actions.go:974-987 (cast's pending damages of same type settle with one roll); combat_actions.go:1013 (total = max(total,0): never negative)
SCREEN: web/src/app/pages/live-session/combat/attack-sheet/attack-sheet.ts:314 (damage step)
DOCS: docs/architecture.md:1628 (pending damage keeps dice from the hit)
TESTS: no match by name

MECHANIC: damage-and-healing / critical hits: roll all attack dice twice, add modifiers once
SERVER: rules/combat/rolls.go:80-88 CriticalDice (doubled dice by default; bonus not doubled); rules/combat/rolls.go:55-57 DiceToRoll comment ("the bonus is never doubled"); play/combat_reactions.go:66 openHit calls CriticalDice; combat_actions.go:785-787 (natural 20 -> outcomeCrit)
SCREEN: web/src/app/pages/live-session/combat/attack-sheet/attack-sheet.ts:314 (line "role os dados duas vezes" / "o máximo mais uma rolagem")
DOCS: docs/product/rules.md:358 (critical hit holds on combat attacks and spells); docs/product/stories.md:544 (server applies critical by table rule)
TESTS: backend/internal/rules/combat/combat_test.go:76 TestCriticalDice; backend/internal/play/combat_tablerules_test.go:226 TestRN24_TheCriticalFollowsTheTablesRule; backend/internal/play/combat_bonus_attacks_test.go:152 TestImprovedCriticalMakesANatural19ACriticalHit

MECHANIC: damage-and-healing / critical hits: "maximum plus one roll" table rule (not in SRD, table option)
SERVER: rules/combat/rolls.go:84-85 (CriticalMaxPlusRoll: count dice once + count*sides fixed); combat_actions.go:991-993 comment; combat_actions.go:1019 (CriticalMax on made event)
SCREEN: web/src/app/pages/live-session/combat/attack-sheet/attack-sheet.ts:314
DOCS: docs/product/rules.md:353 (critical hit: doubled dice or maximum plus a roll), docs/product/rules.md:358 (Brutal Critical extra dice not built)
TESTS: backend/internal/play/combat_tablerules_test.go:226 TestRN24_TheCriticalFollowsTheTablesRule

MECHANIC: damage-and-healing / critical hits: Improved Critical / Superior Critical widen crit range (weapon only)
SERVER: play/combat_actions.go:730-733 (criticalFrom = CriticalRange; 0 for spell attacks, so spells need natural 20); rules/combat/rolls.go:39-52
SCREEN: no match
DOCS: docs/product/rules.md:358 ("A weapon attack is a critical hit on a natural 20, or on a 19 or 20 for a Champion")
TESTS: backend/internal/play/combat_bonus_attacks_test.go:152 TestImprovedCriticalMakesANatural19ACriticalHit

MECHANIC: damage-and-healing / damage-types: list of 13 types
SERVER: proto/meurpg/characters/v1/characters.proto:1277-1291 (DAMAGE_TYPE_ACID, BLUDGEONING, COLD, FIRE, FORCE, LIGHTNING, NECROTIC, PIERCING, POISON, PSYCHIC, RADIANT, SLASHING, THUNDER); backend/internal/characters/combatturn.go:84 (key damage-type:<name>)
SCREEN: no match checked for type list
DOCS: no match
TESTS: no match by name

MECHANIC: damage-and-healing / damage-resistance-and-vulnerability: resistance halves, vulnerability doubles (NPC/creature only)
SERVER: rules/combat/damagetype.go:18-32 AdjustForType (immune=0, resistant /2, vulnerable *2); play/combat_actions.go:1126-1135 afterResistance (only if holdsHP: NPC or creature; not player); combat_actions.go:1030-1032 (RollDamage applies it when not healing); characters/charactercreatures_roster.go:536-565 DamageModifiers (player character returns empty: line ~546-548 kindPlayer check); plainTypes skips entries with a condition note
SCREEN: web/src/app/shared/creatures/stat-block.ts:176-178 ("Vulnerável a", "Resistente a", "Imune a"); web/src/app/pages/live-session/traps/trap-damages/trap-damages.html:40 ("Mude o número se houver resistência: o app não calcula.")
DOCS: docs/product/rules.md:56 ("after the resistances, vulnerabilities and immunities of its SRD creature"); docs/architecture.md:805 (AdjustForType); docs/architecture.md:1628 (afterResistance on pending damage)
TESTS: backend/internal/play/combat_monsters_test.go:769 TestMonsterTakesDoubleDamageOfATypeItIsVulnerableTo; combat_monsters_test.go:789 TestMonsterTakesNoDamageOfATypeItIsImmuneTo; backend/internal/rules/combat/combat_test.go:632 TestAdjustForType

MECHANIC: damage-and-healing / damage-resistance-and-vulnerability: PLAYER CHARACTER resistance/vulnerability/immunity
SERVER: NO ADJUSTMENT. afterResistance returns amount unchanged when !holdsHP (combat_actions.go:1127-1128); holdsHP is NPC or creature only (play/combat_rules.go:37); ApplyPendingDamage for a player (combat_actions.go:1353-1355, 1401-1423) never calls afterResistance; the master may type a different amount (combat_actions.go:1329-1332 override). Rage and other resistances of a character are notes only: combat_actions.go:1123-1125 comment. Source of player modifiers: none (DamageModifiers returns empty for kindPlayer, charactercreatures_roster.go:536-565). Out-of-combat trap damage on a player: play/traps_damage.go:28-29 ("resistances are not modeled"). In-combat trap: play/combat_traps.go:248 calls afterResistance, which is a no-op for a player.
SCREEN: web/src/app/pages/live-session/combat/npc-card/pending-damages.html:38 ("O dado deu N. Por exemplo, metade por resistência.") - master hint only
DOCS: docs/product/rules.md:56 (applies to NPC, monster, creature of a character - not the character itself); docs/architecture.md:1628
TESTS: no test for player resistance found (no match by name)

MECHANIC: damage-and-healing / damage-resistance-and-vulnerability: a character's creature (familiar, summon) gets its monster modifiers
SERVER: play/combat_actions.go:1127 holdsHP includes kindCreature (combat_rules.go:37); DamageModifiers with monsterKey (charactercreatures_roster.go:536-565)
SCREEN: no match
DOCS: docs/product/rules.md:56 ("and a character's creature")
TESTS: no match by name

MECHANIC: damage-and-healing / damage-resistance-and-vulnerability: multiple sources of same type count once; resistant+vulnerable both apply
SERVER: rules/combat/damagetype.go:22-30 (lists, not counted: Contains, one halving; resistance then vulnerability; immune returns 0 first)
SCREEN: no match
DOCS: docs/architecture.md:805 (immunity 0, resistance halves, vulnerability doubles, both in that order)
TESTS: backend/internal/rules/combat/combat_test.go:632 TestAdjustForType

MECHANIC: damage-and-healing / healing: capped at maximum, excess lost
SERVER: rules/combat/vitals.go:39-42 ApplyHeal (min(hp+amount, max)); combat_actions.go:1171-1179 (NPC heal); combat_actions.go:1198-1208 (player heal through vitals)
SCREEN: web/src/app/core/combat/hp-effects.ts:50 (healing line)
DOCS: docs/product/rules.md:56 (maximums come from sheet; stored value clamped)
TESTS: backend/internal/rules/combat/combat_test.go:127 TestApplyHeal; backend/internal/play/highlights_test.go:465 TestMR032_HealingIsWhatWasGivenBack

MECHANIC: damage-and-healing / healing: healing from 0 HP (player)
SERVER: play/combat_vitals.go:43-46 (healed from 0 to >0 resets death save counts); combat_actions.go:1181-1208 (healCombatant via vitals); combat_actions.go:1060-1061 comment (0 hit points rule)
SCREEN: web/src/app/core/combat/combat-notices.ts:189 ("volta no descanso longo" only for wild shape)
DOCS: docs/product/rules.md:56 (healed at 0 gets up)
TESTS: backend/internal/play/combat_spells_test.go:553 TestHealingSpellRevivesAndResetsDeathSaves

MECHANIC: damage-and-healing / healing: healing from 0 HP (NPC/creature)
SERVER: combat_actions.go:1171-1179 (ApplyHeal; Defeated = HP==0 so healed above 0 returns to order)
SCREEN: no match checked
DOCS: docs/product/rules.md:211-218 RN-20 (state word only)
TESTS: no match by name

MECHANIC: damage-and-healing / healing: a dead creature cannot regain HP until revivify
SERVER: no match for revivify / dead-heal refusal in play/ (not traced further; unsure)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: damage-and-healing / dropping-to-0-hit-points: instant death from massive damage (remaining >= max HP)
SERVER: NO MATCH in play/. rules/combat/vitals.go:10-13 computes Excess ("A hit with Excess of at least the maximum hit points kills a character outright, which the master decides (RN-03)") and vitals.go:25 sets it; the field is read nowhere outside rules/combat (grep Excess in play = none). ApplyPendingDamage down branch (combat_actions.go:1371-1381) and default branch (1401-1423) never check excess vs max.
SCREEN: no match for instant death text in web
DOCS: no match (docs/product/rules.md contains no "morte instantânea")
TESTS: no match by name

MECHANIC: damage-and-healing / dropping-to-0-hit-points: falling unconscious at 0 HP
SERVER: no match for automatic addition of condition:unconscious at 0 (only the condition key list: web/src/app/core/combat/conditions.ts:27 'Inconsciente'; play/combat_move.go:145 noSpeed list). Dropping to 0 keeps state in vitals: combat_actions.go:1371 (isDownIn)
SCREEN: web/src/app/core/combat/conditions.ts:27 ("Inconsciente" label)
DOCS: no match for "cai inconsciente" in rules.md
TESTS: no match by name

MECHANIC: damage-and-healing / death-saving-throws: d20 10+ success, below 10 failure, 1 = two failures, 20 = regain 1 HP
SERVER: rules/combat/vitals.go:71-84 (DeathSave); play/combat_death.go:56-182 (RollDeathSave; 137 combat.DeathSave; 143-153 natural 20 revives via vitals with HP 1); play/combat_view.go:417-420 (deathSaveDue: player, down, counts < 3, not rolled)
SCREEN: web/src/app/pages/live-session/combat/death-saves/death-saves.ts:71 ("3 sucessos: você se estabiliza.") and :72 ("3 falhas: o mestre confirma a morte.")
DOCS: docs/product/rules.md:11 (RN-03 summary: a dead character is not deleted; death saves in rules.md RN-03 section)
TESTS: backend/internal/play/combat_spells_test.go:870 TestRN03_DeathSavesAndTheMasterConfirms; combat_spells_test.go:1011 TestRN03_NaturalOneTwentyAndStable; backend/internal/rules/combat/combat_test.go:148 TestDeathSave

MECHANIC: damage-and-healing / death-saving-throws: NPC drops to 0 and is defeated (monsters die at 0)
SERVER: rules/combat/vitals.go:18-28 (FellToZero); combat_actions.go:1153-1154 (NPC Defeated = HP==0; leaves order)
SCREEN: web/src/app/pages/live-session/combat/npc-card/npc-card.ts:102 (what the turn still owes; NPC defeat state)
DOCS: docs/product/rules.md:56 ("at 0 HP it is defeated and leaves the order")
TESTS: no match by name

MECHANIC: damage-and-healing / dropping-to-0-hit-points: damage at 0 HP = death save failure (critical = two)
SERVER: play/combat_actions.go:1371-1381 (down: failuresWhileDown with p.Critical); play/combat_death.go:285-302 (failuresWhileDown; amount<=0 nothing; stable resets counts first 291-293); rules/combat/vitals.go:86-91 DamageWhileDown (2 if critical else 1)
SCREEN: web/src/app/pages/live-session/combat/death-saves/death-saves.ts:65 ("Estável: não rola mais testes contra a morte.")
DOCS: docs/product/rules.md:56 (damage at 0 is handled by the death save rule)
TESTS: backend/internal/play/combat_spells_test.go:1011 TestRN03_NaturalOneTwentyAndStable

MECHANIC: damage-and-healing / death-saving-throws: three failures = death (master confirms)
SERVER: play/combat_death.go:200-278 (ConfirmDeath: master only, needs DeathFailures>=3 at combat_death.go:238; MarkDead 243)
SCREEN: web/src/app/pages/live-session/combat/death-saves/death-saves.ts:72 ("3 falhas: o mestre confirma a morte.")
DOCS: docs/product/rules.md:11 (RN-03: a dead character is not deleted)
TESTS: backend/internal/play/combat_spells_test.go:870 TestRN03_DeathSavesAndTheMasterConfirms; combat_tablerules_test.go:636 TestRN24_ADeathTheMasterConfirmsIsTheTables

MECHANIC: damage-and-healing / stabilizing: Medicine check DC 10 (first aid, action)
SERVER: no match for Medicina / Medicine / first aid / DC 10 check on unconscious creature (checked backend/internal and web/src/app and docs)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: damage-and-healing / stabilizing: spare the dying / Estabilizar spell makes a 0 HP character stable
SERVER: play/combat_spells_hp.go:153-170 (SpellKindZeroHP: rules/combat/hpspells.go:84-88 ResolveZeroHP hp==0; player: SetCombatantDeathSaves successes=3 failures=0 at 161-167); spell list docs in backend/internal/rules/spelleffects.go:11-28
SCREEN: web/src/app/core/combat/hp-effects.ts:14 (Estabilizar listed as HP spell); web/src/app/pages/live-session/combat/death-saves/death-saves.ts:65 ("Estável")
DOCS: docs/product/rules.md:230 ("Spare the Dying and Heal ... are resolved by the server")
TESTS: web/src/app/core/combat/hp-spells-log.spec.ts:141 (Estabilizar: Brisa estável); backend/internal/play/combat_spells_hp_test.go:396 TestMR014_CompleteHealHealsAndEndsBlindnessAndDeafness (Cura Completa, not Estabilizar)

MECHANIC: damage-and-healing / stabilizing: stable creature regains 1 HP after 1d4 hours
SERVER: no match for 1d4 hours regain (play/, characters/vitals.go)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: damage-and-healing / stabilizing: stable creature that takes damage starts death saves again
SERVER: play/combat_death.go:291-293 (successes>=3 resets both counts before adding failures)
SCREEN: no match
DOCS: no match beyond rules.md:RN-03
TESTS: no match by name

MECHANIC: damage-and-healing / monsters-and-death: monsters die at 0 HP (GM choice for special NPCs)
SERVER: combat_actions.go:1153 (NPC defeated at 0 directly, no death saves); combat_death.go:109-111 (only player characters roll death saves); combat_death.go:235-237 (only player dies via ConfirmDeath)
SCREEN: web/src/app/pages/live-session/combat/npc-card/npc-card.ts:102
DOCS: no match for "special NPC falls unconscious" option
TESTS: no match by name

MECHANIC: damage-and-healing / knocking-a-creature-out: melee hit to 0 HP may knock out (unconscious, stable) instead of killing
SERVER: no match (no melee/non-lethal flag on pending damage; openHit combat_reactions.go:66-88 stores no choice; ApplyPendingDamage has no knock-out option)
SCREEN: no match for "nocaute" / "desacordar" / "knock out"
DOCS: no match
TESTS: no match

MECHANIC: damage-and-healing / temporary-hit-points: absorb damage first, leftover carries to HP
SERVER: rules/combat/vitals.go:18-28 ApplyDamage (Absorbed = min(amount, temp)); combat_actions.go:1152 (NPC), combat_actions.go:1405 (player)
SCREEN: web/src/app/pages/live-session/adjust-vitals/adjust-vitals.html:47 ("PV temporários" field)
DOCS: docs/product/rules.md:56 (temporary HP first)
TESTS: no match by name

MECHANIC: damage-and-healing / temporary-hit-points: do not stack; keep the larger
SERVER: play/combat_spells_hp.go:305-311 (giveTempHP: temp = max(amount, current); comment "they do not stack")
SCREEN: no match
DOCS: docs/architecture.md:1656 ("ganha 5 PV temporários" wording only)
TESTS: backend/internal/play/combat_spells_hp_test.go:574 TestFalseLifeGivesTemporaryHitPoints
NOTE: master manual set overwrites temp HP without max: characters/vitals.go:385-389 (HitPointsTemporary = req value); combat_actions.go:1927-1969 (AdjustCombatantHitPoints temp set directly, no max rule)

MECHANIC: damage-and-healing / temporary-hit-points: healing cannot restore temporary HP
SERVER: rules/combat/vitals.go:39-42 ApplyHeal only touches HP; combat_actions.go:1173-1174 (healCombatant keeps Temp: before.Temp)
SCREEN: no match
DOCS: no match
TESTS: no match by name

MECHANIC: damage-and-healing / temporary-hit-points: temp HP at 0 HP does not wake or stabilize
SERVER: play/combat_spells_hp.go:320-326 (giveTempHP only sets temp via vitals; no death-state change); combat_vitals.go:43-46 reset only when HP goes from 0 to >0
SCREEN: no match
DOCS: no match
TESTS: no match by name

MECHANIC: damage-and-healing / temporary-hit-points: duration until long rest
SERVER: no match for long-rest reset of temp HP in play/ or characters/vitals.go
SCREEN: no match (web/src/app/core/combat/combat-errors.ts:49 "volta num descanso longo" is wild shape only)
DOCS: no match
TESTS: no match

MECHANIC: mounted-combat / mounting-and-dismounting: half speed to mount, DC 10 Dex save when mount moves against will
SERVER: no match (rules/summon.go:15-16 "Find Steed is out (question 74: mounted combat comes after the MVP)")
SCREEN: no match
DOCS: backend/internal/rules/summon.go:15-16 (code comment: mounted combat after MVP, deliberately not built)
TESTS: no match

MECHANIC: mounted-combat / controlling-a-mount: controlled mount shares initiative, only Dash/Disengage/Dodge
SERVER: no match
SCREEN: no match
DOCS: backend/internal/rules/summon.go:15-16 (same deliberate exclusion)
TESTS: no match

MECHANIC: mounted-combat / controlling-a-mount: opportunity attack on mount may target rider
SERVER: no match
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: underwater-combat / melee disadvantage without swim speed (except dagger, javelin, shortsword, spear, trident)
SERVER: no match (no swim speed in movement: play/combat_move.go:134-136 "Swimming and climbing are out of scope"; speedDFt uses walk or fly only, combat_move.go:150-160 area)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: underwater-combat / ranged weapon beyond normal range auto-misses; disadvantage underwater
SERVER: no match (no underwater state; range only via reachFt, play/combat_actions.go:181-183)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: underwater-combat / fully immersed creatures resistance to fire
SERVER: no match (no immersion state; AdjustForType only from stat block, rules/combat/damagetype.go:18-32)
SCREEN: no match
DOCS: no match
TESTS: no match
