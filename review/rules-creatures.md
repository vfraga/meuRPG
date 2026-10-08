# Rules review R7: creatures, Wild Shape, familiars and summons

- Scope: bestiary data and monsters in combat (RN-29), the NPC made from a creature, Wild Shape, Find Familiar and the Pact of the Chain, Animate Dead, Conjure Animals, the web's bestiary/creature/Wild Shape screens.
- Commit reviewed: `933df75` (main). SRD data: 5e-database `a8abc93b235c158bb8cbf042e54425b9c2fd79b8`. Cross-check copy of the SRD 5.1 stat blocks: a public markdown transcription of the SRD (used only to compare, it has its own typos; refuted rows say so).
- Time spent: about 3 hours. Model: claude-sonnet-5-5 (subagents: sonnet).
- Web: the bestiary, creature and Wild Shape screens compute nothing (every number comes from the server; feet to metres is the table's 5 ft = 1.5 m). No web finding; the wrong data below shows on those screens.
- Tests: `go test ./internal/rules ./internal/characters ./internal/play -run TestRulesReviewCreatures` (play needs `MEURPG_TEST_DATABASE_URL`). All tests below fail today.

| id | severity | status | file:line | defect | failure scenario (app vs SRD) | evidence |
|---|---|---|---|---|---|---|
| R7-1 | high | confirmed | `characters/combatturn.go:86`, `characters/npcfromcreature.go:190` | Monsters in combat: opportunity attacks never happen for NPCs whose attacks have reach over 5 ft or are thrown weapons | `basicDerived` marks an attack melee only if `RangeFt <= 5` (`characters/combatturn.go:86`) while `basicAttackOf` stores the reach or the thrown range as `RangeFt` (`characters/npcfromcreature.go:190`). Hill Giant greatclub (reach 10 ft) and Guard spear (reach 5 or range 20/60) come out non-melee, so play never offers them an opportunity attack (SRD, Opportunity Attacks: a melee attack when a hostile leaves your reach). 45 of 330 creatures that have a melee attack end with none: all giants, all ancient dragons, aboleth, balor, chain/bone/horned devil, guard, mage, tarrasque, T. rex and more | `TestRulesReviewCreaturesReach`, `TestRulesReviewCreaturesNoMelee` (characters) |
| R7-2 | high | confirmed | `play` damage path | Damage to a monster ignores its vulnerabilities, resistances and immunities | Skeleton (vulnerable to bludgeoning, immune to poison): 6 bludgeoning leaves 7 of 13 HP (SRD, Damage Resistance and Vulnerability: double, so 1); Poison Spray 6 poison leaves 7 (SRD: immune, 13). No doc declares it not built; `play/traps_damage.go:26` only has a comment | `TestRulesReviewCreatures_MonsterVulnerabilityIgnored`, `TestRulesReviewCreatures_MonsterPoisonImmunityIgnored` (play) |
| R7-3 | high | confirmed | `rules/srd51/effects/druid.json:14` | Level 20 Archdruid does not give unlimited Wild Shape | Level 20 druid: Wild Shape max 2 uses (SRD, Druid, Archdruid: unlimited); the barbarian's unlimited Rage uses 99 | `TestRulesReviewCreatures_ArchdruidUnlimitedWildShape` |
| R7-4 | high | confirmed | `rules/wildshape.go:149`, `play/wildshape.go` `noSpellsIn` | Level 18 Beast Spells missing | Druid 18 in wolf form: no spellcasting, and CastSpell refused with `WILD_SHAPE_NO_SPELLS` (SRD, Druid, Beast Spells: can cast druid spells in any Wild Shape form, without material components). Docs describe the refusal without mentioning level 18 | `TestRulesReviewCreatures_BeastSpellsAt18`, `TestRulesReviewCreatures_BeastSpellsRefusedAt18` |
| R7-5 | medium | confirmed | `monsters.json` (basilisk) | Basilisk AC | 12 vs 15 (SRD, Basilisk: natural armor) | `TestRulesReviewCreatures_BasiliskAC` |
| R7-6 | medium | confirmed | `monsters.json` (cult-fanatic) | Cult Fanatic hit points | 22 (6d8-5) vs 33 (6d8+6); average and rolled HP both wrong | `TestRulesReviewCreatures_CultFanaticHP` |
| R7-7 | medium | confirmed | `monsters.json` (crocodile) | Crocodile swim speed, a Wild Shape form from druid level 4 | 20 ft vs 30 ft | `TestRulesReviewCreatures_CrocodileSwim` |
| R7-8 | medium | confirmed | `monsters.json` (giant-wasp, adult-brass-dragon, hunter-shark) | Wrong speeds and senses | Giant Wasp has a swim 50 (SRD: none); Adult Brass Dragon burrow 40 (SRD 30); Hunter Shark darkvision 30 and no blindsight (SRD: blindsight 30, no darkvision) | `GiantWaspSwim`, `AdultBrassDragonBurrow`, `HunterSharkSenses` |
| R7-9 | medium | confirmed | `monsters.json` (7 creatures) | Condition immunity "deafened" lost: the importer source repeats "blinded" | Black Pudding lacks deafened; Flying Sword, Ochre Jelly, Rug of Smothering, Shambling Mound, Shrieker, Violet Fungus list blinded twice and lack deafened (SRD stat blocks list deafened) | `TestRulesReviewCreatures_ConditionImmunities` |
| R7-10 | medium | confirmed | `monsters.json` (ice-devil) | Ice Devil lacks the cold immunity | immunities fire, poison vs cold, fire, poison | `TestRulesReviewCreatures_IceDevilImmunities` |
| R7-11 | medium | confirmed | `monsters.json`; copied by `characters/npcfromcreature.go:125` | XP does not match the challenge rating | Brass Dragon Wyrmling (CR 1) 100 vs 200; Deep Gnome (CR 1/2) 50 vs 100; Dretch (CR 1/4) 25 vs 50; Riding Horse (CR 1/4) 25 vs 50. Flows into the bestiary, NPCs made from creatures, XP by enemies, the encounter builder. The scan over all 334 finds only these four | `TestRulesReviewCreatures_XPFourCreatures`, `TestRulesReviewCreatures_XPMatchesChallenge` |
| R7-12 | low | confirmed | `monsters.json` | Missing skills / stale passive Perception | Black Bear lacks Perception +3; Half-Red Dragon Veteran lacks Athletics +5 and Perception +2; Swarm of Ravens lacks Perception +5; Blink Dog passive Perception 10 vs 13 | `PerceptionSkillsListed`, `BlinkDogPassive`, `PassivePerception` |
| R7-13 | medium | policy | `play/combat_opportunity.go:115`, `play/combat_creatures.go:89` | Pact of the Chain familiar attacks with its reaction at any time, also as an opportunity attack, without the warlock forgoing one of their attacks | SRD: only when the warlock takes the Attack action and forgoes one attack. Docs describe it (`architecture.md` lines 771, 1940), no decision | `TestRulesReviewCreatures_ChainFamiliarReactionNeedsForgoneAttack` |
| R7-14 | medium | unverified | none | Find Familiar: the familiar cannot deliver touch spells with its reaction (within 100 ft) | Not built, not listed as not built in the docs; no API surface to test against | grep of play, characters, rules and docs |
| R7-15 | low | policy | `characters/wildshape.go` (`ErrAlreadyInWildShape`) | A druid already in a beast form cannot use Wild Shape again to change form | SRD has no such limit (a use and an action per change). Only described in `architecture.md`, no decision | existing test asserts the refusal |
| R7-16 | low | not built (doc) | `architecture.md:1993` "Known limits" (2) | Wild Shape duration (half the druid level in hours) is not counted; the master ends the form | named as a known limit, so not a finding | doc |
| R7-17 | low | refuted | `monsters.json` | Bandit Captain/Mage dagger and Merrow harpoon damage bonus differ from the markdown transcription | The transcription is inconsistent with the creatures' own scores (e.g. "5 (1d4 + 2)"); the app's 1d4+3, 1d4+2, 2d6+4 are right | reading of the stat blocks |
| R7-18 | low | unverified | `monsters.json` (spider) | Spider passive Perception 12 (WIS 10, no Perception skill) | The transcription says 10; not sure which is the SRD's | none |

Notes for other areas: opportunity attack rules and damage resistance for characters belong to combat; "Known limits" (1) in `architecture.md:1993` (HP spells read the druid's HP, not the beast's) is documented. Documented simplifications not reported: only the first damage part of an attack is kept, at most three attacks on an NPC, walking speed for an NPC's speed, a single initiative for Animate Dead, summoned creatures lasting until dismissed.

## Fix direction

**R7-1.** Root cause: `BasicAttack` keeps one number (`range_ft`) for reach, thrown range and bow range, and `basicDerived` guesses melee from it. Fix in code: `basicAttackOf` should carry whether the attack is melee (from `rules.Attack.Melee`) and the long range, or `basicDerived` should read the linked creature's attacks (`MonsterDerived(monster_key)`) for kind, reach and range; opportunity reach comes from `meleeReachOf`. Same mistake: `combatturn.go:86`, and `attackReach` (`play/combat_actions.go:168`) using a fixed 5 ft for reactions. Docs: `architecture.md` "Bestiary and creating an NPC" (mentions "the reach"). Guards: `TestNpcSheetAttacks`, `TestBasicDerivedFollowsTheCreaturesMultiattack`, and the opportunity tests in `play`.

**R7-2.** Root cause: damage is applied as rolled; the stat block's `Resistances/Immunities/Vulnerabilities` are shown but never used. Fix in code: apply them in `rules/combat` (a pure function: immunity 0, then resistance halves rounded down, vulnerability doubles, after bonuses) when the damage lands on an NPC with a `monster_key` (and a creature of a character), keeping the master's amount override. The "nonmagical weapons" notes need a decision (master confirms). Docs: `rules.md` damage paragraph, RN-29. Guards: `TestMR042_*`, hp-spell tests.

**R7-3, R7-4.** Root cause: `feature:archdruid` and `feature:beast-spells` have no entries in `effects/druid.json`. Fix is handwritten effects data: Wild Shape max `classLevel("druid") >= 20 ? 99 : 2` (as the barbarian), and a flag on the `wild_shape` effect (or a new effect) that keeps spellcasting in `WildShapeDerived` and lets `noSpellsIn` pass from level 18 (material components stay the table's business). Bumps the content revision (`CONTRIBUTING.md`). Same mistake: `play/wildshape.go` `noSpellsIn`, `play/creature_cast.go` CastSummon. Docs: `architecture.md` Wild Shape sections, glossary "Wild Shape". Guards: `TestWildShapeDerived`, `TestMR037_WildShape*`.

**R7-5 to R7-12.** Root cause: 5e-database values wrong against the SRD text (and "deafened" lost in the source). Fix: handwritten data in `effects/corrections.json` with new `creature_corrections` fields (armor class, hit_points_roll and hit points, speeds, senses, immunities, condition immunities, xp, skills, passive Perception), each with a source line like the existing ones; the loader must refuse unknown creatures and fields; bump the content revision. Alternative for XP: derive it from `effects/advancement.json` by CR in `buildCreatures`, which fixes all four at once and any future one. Guards: the new review tests (rename them to permanent ones), `TestMonsterDerived`, `TestCreaturesAreTheSRD`, `TestEveryCreatureMakesAnNpc`.

**R7-13.** Needs a decision: either keep (document it as a decision) or require the warlock's Attack action: the familiar's reaction attack only while the owner has an attack left on the Attack action, and spend one of the owner's attacks; no opportunity attacks for the familiar. Touches `mayAttack`, `opportunityReactors`, `TestMR037_TheChainFamiliarAttacksOnlyWithItsReaction`.

## Checklist

- Monster AC, HP average, rolled HP (dice, bonus, minimum 1): covered by `TestMonsterDerived`, `TestMR042_MonstersRollTheirHitPoints`; findings R7-5, R7-6
- Attack bonus and damage of each attack: covered by `TestMonsterDerived`, `TestCreatureAttacksAndCounts`; only the first damage part (documented)
- Multiattack counts: covered by `TestMultiattackCountsFollowTheSRDText`
- Saving throws and skills, speeds, senses: covered by `TestMonsterDerived`; findings R7-7, R7-8, R7-12
- Damage and condition immunities, resistances, vulnerabilities: finding R7-2, R7-9, R7-10
- CR and XP; XP by enemies: covered by `TestMR042_MonstersGiveXPByTheirChallengeRating`; finding R7-11
- Monster to NPC conversion: covered by `TestNpcSheetFromCreature`; finding R7-1
- Wild Shape forms by level (31/48/70 beasts, no fly/swim limits): covered by `TestWildShapeForms`
- Wild Shape uses (2 per rest), action to enter, bonus action to leave: covered by `TestMR037_WildShapeInCombat`; Archdruid finding R7-3
- Wild Shape duration: not built (doc), R7-16
- Beast HP as separate pool, carry-over damage, exact damage: covered by `TestMR037_WildShapeDamageGoesToTheBeast`, `TestMR037_WildShapeExactDamageEndsTheFormWithNothingLeftOver`
- Mental scores kept, physical scores, saves/skills merged: covered by `TestWildShapeDerived`
- No spellcasting (and Beast Spells): finding R7-4
- Switching form while transformed: finding R7-15
- Find Familiar forms, no attack, ritual without slot, replace: covered by `TestSummonOptions`, `TestMR037_TheRitualFamiliarSpendsNoSlotAndANewOneReplacesTheOld`
- Familiar sight (action, ends next turn, 100 ft): covered by `TestMR036_FamiliarSightInCombat`, `TestMR036_FamiliarSightNeedsTheFamiliarWithin30m`
- Familiar touch spells: finding R7-14
- Pact of the Chain forms and attack: covered by `TestSummonOptions`; finding R7-13
- Animate Dead count and higher slots: covered by `TestMR037_AnimateDeadAtTheThirdAndFifthCircles`
- Conjure Animals options by CR, counts at 5th/7th/9th, concentration: covered by `TestSummonOptions`, `TestMR037_ConjureAnimalsInCombat`
- Control of creatures in combat: covered by `TestMR037_ACreatureMovesLikeAnyCombatant`, `TestRN20_CreatureHitPointsOnlyToOwnerAndMaster`
- Web screens: no rules computation (numbers from the server)
