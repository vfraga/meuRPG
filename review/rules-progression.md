# Rules review: Tier 5 R6, XP, level-up, encounters and treasure

- Scope: `rules` (progression, levelup, tabledefaults, encounters, encounter/, treasure*, magicitems, the four effects files), `progression`, `characters` level-up/progress/party levels, `play/xp.go`, and the web's XP, level-up, encounter and treasure screens.
- Commit reviewed: `ae40f74` (main). SRD data commit: `a8abc93b235c158bb8cbf042e54425b9c2fd79b8`. SRD 5.1 text for the Life Domain and the multiclass table was read from a public SRD 5.1 markdown copy (BTMorton/dnd-5e-srd).
- Time spent: about 2 hours of session time. Model: claude-sonnet-5-5; subagents: sonnet.
- Method note: every table (XP per level, XP per CR, class casting columns, ASI levels, subclass levels and features, multiclass prerequisites, budget table, magic item values, treasure averages) was compared with the SRD by script or by hand. Browser screens of this area read their numbers from the server (checked: `xp-math.ts`, hp/abilities steps, encounter and treasure pages).

| id | severity | status | file:line | defect | scenario | evidence |
| --- | --- | --- | --- | --- | --- | --- |
| R6-1 | high | confirmed | `rules/creatures.go:172` (data `srd51/data/monsters.json`), used by `rules/encounters.go:127,154`, `characters/npcfromcreature.go:125` | A creature's XP is taken from the imported `xp` field, not from its CR, and 4 creatures have half the right value. | Riding horse and dretch (CR 1/4): app 25 XP, SRD 50. Deep gnome (CR 1/2): 50, SRD 100. Brass dragon wyrmling (CR 1): 100, SRD 200 (SRD 5.1 "Experience Points by Challenge Rating" and each stat block's "Challenge" line). An encounter of one wyrmling is budgeted at 100 XP instead of 200, the generator and swaps use the wrong cost, and an NPC made from the creature gives half the XP when defeated. | `TestRulesReviewProgression_CreatureXPFollowsChallengeRating` and `TestRulesReviewProgression_EncounterUsesWrongCreatureXP` fail: `go test ./internal/rules -run TestRulesReviewProgression_Creature` |
| R6-2 | medium | confirmed | `rules/srd51/data/subclasses.json` (subclass:life spells) | Life Domain is missing Guardian of Faith at cleric level 7. | Level 7 Life cleric: app always-prepares death ward only; SRD 5.1 "Life Domain Spells" gives death ward and guardian of faith at 7th. The snapshot of the 5e-database has the same gap, and `effects/corrections.json` does not fix it. | `TestRulesReviewProgression_LifeDomainSpells` fails: `spell:guardian-of-faith` not always prepared; level-7 spells = [death-ward] |
| R6-3 | low | unverified | `srd51/data/classes.json` (bard multiclass) | Multiclassing into Bard gives light armor and one skill but not "one musical instrument of your choice". | SRD 5.1 "Multiclassing Proficiencies": Bard gets light armor, one skill, one musical instrument. The app has no instrument choice anywhere (no instrument concept for the first Bard level either), so it is a missing choice, not a wrong number; no test written. | read from the SRD copy; no failing test |

Everything else checked in the area matched the SRD or a documented decision (list below). Frog and sea horse give 0 XP: the SRD allows 0 or 10 for CR 0, and RN-09 documents it.

Other areas (one line each): the Fiend's expanded list is stored at warlock levels 1/3/5/7/9 as a remap of spell circles, which is consistent (spells area); class_specific values (rage, ki, sneak attack) match the SRD but are used by combat/rests; Sorcerer "creating spell slots" data exists but is not used (not built, rests area).

## Fix direction

**R6-1.** Root cause: `CreatureEntry.XP` copies the snapshot's `xp`, which is wrong for 4 monsters; the CR table in `effects/advancement.json` is right. Fix in code: in `buildCreatures` (`creatures.go`) set `XP` from the CR table (`XPForChallenge`), keeping 0 for CR 0 creatures that the data gives 0 (frog, sea horse); or add a `creature_corrections` entry per monster in `effects/corrections.json` (handwritten effects data, so the content revision bumps as CONTRIBUTING.md says). Prefer deriving from the CR so no creature can drift. Same mistake elsewhere: `rules/encounters.go:127` and `:154` and `play/encounters.go:214` read the same field; `characters/creatures.go:165` shows it in the bestiary. Docs: `docs/product/rules.md` RN-09/RN-29 ("counts by its challenge rating") already say CR, nothing to change. Risks: generated encounters and swaps change for those creatures (seeded results change, content revision bump); guard tests: `TestGenerateEncounter*`, `TestEncounterSwapsAreTheSameXP`, golden treasure unaffected.

**R6-2.** Root cause: the 5e-database Life Domain list lacks the second 7th-level spell. Fix: data, not code. The snapshot is generated (`data/` is never edited by hand and is hash-checked), so add a handwritten correction or an `effects/` entry that adds `spell:guardian-of-faith` at class level 7 to `subclass:life`, extending the corrections mechanism (`corrections.json` today only covers class table fields and creature multiattack) and bump the content revision. Same mistake elsewhere: none found; Devotion, Fiend and all seven Land terrains match the SRD. Docs: `CONTRIBUTING.md` corrections paragraph if the mechanism grows. Guards: `TestLevelUpSweep` (a level-7 Life cleric must still pass), golden tests and the new failing test.

## Checklist

- XP per level 1 to 20: covered by `TestNextLevelXP` (values also compared with the SRD by script, all match).
- XP per CR 0 to 30: covered by `TestXPForChallenge` (table matches); creature XP: finding R6-1.
- XP split rounds down (RN-09): covered by `TestSplitXP`; browser `splitXp` identical (`xp-math.spec.ts`).
- Gold XP 1 per GP, items never XP, milestone mode, undo: covered by `TestMR041_*`, `TestRN09_*`, `TestMR016_*`; not an SRD rule (table decisions).
- Level-up reason by XP or milestone: covered by `TestLevelUpReason`, `TestXPCanLevelUpInAnXPCampaign`.
- Hit points per level (first level max die, average die/2+1, minimum 1, Con retroactive, rolled): covered by `TestLevelUpHitPointsRolled`, golden tests; probed by hand for 13 builds (single, multiclass, Con 3, Hill Dwarf, level 20), all correct.
- ASI levels (fighter 6 and 14, rogue 10, others 4/8/12/16/19): covered by `TestLevelUpSweep`; data checked, correct.
- Subclass level per class and subclass features per level: checked all 12 against SRD; correct apart from R6-2.
- Multiclass prerequisites (13, and the fighter's STR or DEX): checked in data and by probe; correct. Multiclass proficiencies: R6-3 for Bard, rest correct.
- Multiclass spell slots (full + half rounded down + third rounded down, pact separate): probed 6 combinations; correct.
- Cantrips and spells known per level, slots, pact slots, prepared formulas: all 20 levels of every casting class compared with the SRD tables; correct.
- Swap of a known spell on level-up: not built (doc, RN-01).
- Mystic Arcanum, Spell Mastery, favored enemy choices: not built (doc, `master_adds`).
- 20-level defaults of a table class (proficiency bonus, ASI levels, slots from the SRD tables, third caster at a third rounded up): covered by `TestTableDefaultsFollowTheSRDTables`; third-caster spells known is our own documented default (3 at 3 and 4, +1 every 2 levels), consistent with its doc.
- Encounter budget table SRD 5.2.1 (20 rows x 3): covered by `TestEncounterBudgetsAreTheSRD521Table` and `...MatchTheSourceTable`; compared again by hand, all correct.
- Band classification, party sum, maximum CR = lowest level + 3 (documented): covered by `TestEvaluateEncounterTheArtboardNumbers`, `encounter` package tests.
- Magic item values 100/400/4,000/40,000/200,000, consumables half, scroll whole (documented): covered by `TestMagicItemValues`, `TestMagicItemValuesOfAllItems`; item rarities checked against each item's own description line (0 mismatches).
- Own treasure tables: coin averages recomputed (hoard 207 / 1,981 / 19,798 / 99,750 gp, individual 9 / 95 / 944 / 5,055 gp) and match the docs and the targets; gem and art scales match the documented ratio: covered by `TestTreasureInvariants`.
- Web screens (level-up steps, XP block, encounter bar, treasure): numbers come from the server; the only browser arithmetic is the XP split and a progress-bar percentage (display only).
