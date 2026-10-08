# Rules review: Tier 5 R2, the character sheet

- **Scope:** `rules` (derive, abilities, ability methods, armor, hit points, attacks, actions, choices, validate, table-class overlay), `characters` (sheet checks, RN-24 ability scores, level-up), and the web editor, level-up and sheet pages.
- **Commit reviewed:** `ae40f74` (`main`). **SRD data commit:** `a8abc93b235c158bb8cbf042e54425b9c2fd79b8`.
- **Time spent:** about 2 hours. **Model:** claude-sonnet-5-5 (subagents: sonnet).
- **No production code changed.** Run: `cd backend && go test ./internal/rules -run TestRulesReviewSheet_` (all 10 tests fail today, on purpose). No database needed: every finding lives in the pure `rules` package.

## Findings

| id | severity | status | file:line | defect | failure scenario (SRD section) | evidence |
| --- | --- | --- | --- | --- | --- | --- |
| R2-1 | high | confirmed | `rules/attacks.go:40-52` | Monk Martial Arts (monk die, DEX on monk weapons) ignores armor and shield. | Monk 5, STR 10, DEX 19, leather armor or shield: dagger `1d6+4`, quarterstaff uses DEX. SRD (Monk, Martial Arts): only while unarmored and without a shield, so dagger `1d4+4`, quarterstaff uses STR (+0). | `TestRulesReviewSheet_MartialArtsNeedsNoArmor`: `dagger damage = "1d6+4", want 1d4+4`; `quarterstaff ability = dex, want STR` |
| R2-2 | medium | confirmed | `rules/choices.go:94`, `effects/fighter.json` | Number of fighting styles is never checked, and the same style from two classes stacks. | Fighter 1 with Archery and Defense: no issue, +1 AC. Fighter 2 / Paladin 2, chain mail, both Defense: AC 18; SRD (Fighter, Fighting Style: one style, never the same twice) gives 17. | `TestRulesReviewSheet_FightingStyleCount`: `AC got 18, want 17`; `got 1 Issues (same as with one style), want an extra one` |
| R2-3 | medium | policy | `rules/armor.go:46-51` | Heavy armor Strength requirement only prints a hint; speed is not reduced. | Fighter, STR 9, chain mail (STR 13): sheet speed 30. SRD (Armor, Strength requirement): speed -10, so 20 (dwarves exempt). The hint describes it; no doc records a decision. | `TestRulesReviewSheet_HeavyArmorStrengthSpeed`: `SpeedWalkFt = 30, want 20` |
| R2-4 | medium | confirmed | `rules/actions.go` (`seen[e.Resource]`) | Shared resource: the first class listed wins instead of the largest. | Paladin 3 / Cleric 6: Channel Divinity max 1. SRD (Multiclassing, Channel Divinity): the cleric level grants a second use, so 2 (the SRD's own example). Cleric 6 alone gives 2. | `TestRulesReviewSheet_ChannelDivinityPool`: `Max got 1, want 2` |
| R2-5 | medium | confirmed (also not built, doc) | `effects/fighter.json` | Fighter Extra Attack stays at 2 attacks at levels 11 and 20; the sheet shows a wrong number, not nothing. | Fighter 11: `AttacksPerAction` 2, SRD (Fighter, Extra Attack) says 3; Fighter 20: 2, SRD says 4. `architecture.md:720` says effects cover levels 1-5, but the number is shown and wrong. | `TestRulesReviewSheet_FighterExtraAttack`: `got 2, want 3`; `got 2, want 4` |
| R2-6 | medium | confirmed (also not built, doc) | `effects/barbarian.json`; `rules/abilitymethods.go:152` | Primal Champion (level 20: STR and CON +4, maximum 24) is not applied, and a player's draft cannot add it by hand: RN-24 allows only the race's choice points plus 2 per Ability Score Improvement. | Barbarian 20, base STR 18, CON 18, human: STR 19, CON 19; SRD (Barbarian, Primal Champion) says 23. | `TestRulesReviewSheet_PrimalChampion`: `str got 19, want 23`; `con got 19, want 23` (the RN-24 refusal was read in the code, not run) |
| R2-7 | low | confirmed | `rules/attacks.go:63-65` | The Martial Arts die is not applied to versatile (two-handed) damage. | Monk 17, DEX 19, quarterstaff: `Damage 1d10+4`, `Versatile 1d8+4`. SRD (Monk, Martial Arts: die "in place of" the normal damage) says both 1d10+4. | `TestRulesReviewSheet_MartialArtsDieOnVersatile` (an interpretation of the wording) |
| R2-8 | low | confirmed | `rules/attacks.go:28` | No unarmed strike line on the sheet, so a monk's main attack never appears without a listed weapon. No doc records this as a decision. | Monk or Fighter with no weapons: 0 attack lines. SRD (Combat, Unarmed Strike): 1 + STR modifier bludgeoning; monk uses the die. | `TestRulesReviewSheet_UnarmedStrike`: `Attacks = 0 lines` |
| R2-9 | low | confirmed | `rules/attacks.go` | The Heavy weapon property (disadvantage for Small creatures) is not shown anywhere. | Lightfoot Halfling Fighter with a greatsword: no hint. SRD (Weapon Properties, Heavy). | `TestRulesReviewSheet_HeavyWeaponSmall`: no hint from `equipment:greatsword` |
| R2-10 | low | policy | `rules/abilities.go:62` | `score_above_20` is skipped whenever a manual bonus is positive, so editor ASIs can pass 20 with no warning (the level-up path does refuse). Only a code comment describes it. | Fighter 19, base STR 15, manual +10: STR 26, no issue. SRD (class tables, Ability Score Improvement): cap 20 outside features that raise it. | `TestRulesReviewSheet_ScoreAbove20`: `got no score_above_20 issue` |

Checked and found right (no finding): modifier rounding for scores 1-30; proficiency bonus 1-20; first-class saving throws; the 18 skills' abilities; Jack of All Trades (half, rounded down, initiative too); expertise allowances (rogue 1/6, bard 3/10); passive scores; all 13 SRD armors (AC, Dex caps, Str, stealth) and AC rules (Unarmored Defense best-of, monk no shield, Draconic Resilience, Defense); hit points (level 1 maximum, average `die/2+1`, minimum 1 per level, Con retroactive, Dwarven Toughness, Draconic, multiclass dice and roll order); hit dice; speed (race, Fast Movement, Unarmored Movement and conditions); senses; languages; racial and class proficiencies, multiclass proficiency sets and prerequisites against SRD 5.1; skill counts per class; weapon data against the SRD table; ASI levels (fighter 6/14, rogue 10); point buy costs and 27 budget, standard array, 4d6 drop lowest, typed range (SRD 5.2.1 and RN-24); level-up HP and ASI cap. The web has no rules math: `abilityModifier`, the HP preview and `feetToMeters` match the server and the table's 5 ft = 1.5 m.

Other areas (one line each): Eldritch Blast shows one beam with no beam count (spells); Rage's "no heavy armor" limit is not in the Rage hint (combat).

## Fix direction

- **R2-1.** Root cause: the `monk.martial_arts` handler has no armor or shield condition. Fix in code (`attacks.go`): treat `monkWeapon` and the die as true only when `x.armorCategory == "none" && !x.b.Shield`. Same mistake elsewhere: none (Unarmored Defense and Movement already carry the condition in effect data). Docs: none. Guard: `TestDeriveRules` ("monk 5: a dagger uses DEX and the martial arts die") must keep passing.
- **R2-2.** Root cause: `checkChoices` only counts skills and expertise. Fix in code: count the chosen options per offering feature (`choice` effect `count`, `options_choose`) and raise an Issue above the count, and treat the same style from two classes as one (the keys differ per class, so compare by option name). Same mistake: metamagic, invocations, pact boon, hunter's prey. Docs: architecture "What Derive computes". Guards: the level-up sweep tests, since level-up adds choices.
- **R2-3.** Fix in code: subtract 10 ft in `speedAndSenses` when `StrMinimum > STR` unless the race has the dwarven trait; keep the hint. Docs: the architecture armor line. Guard: Fast Movement/barbarian tests.
- **R2-4.** Fix in code (`actions.go`): for a repeated resource keep the largest `Max`, not the first. Same mistake: none today (Channel Divinity is the only shared name). Guard: resource tests in `derive_test.go`.
- **R2-5, R2-6.** Content: handwritten `effects/` data (`fighter.json`: `feature:extra-attack-2` and `-3` with counts 3 and 4; barbarian Primal Champion as modifiers with max 24; RN-24's free points need the same +4/+4). Bumps the content revision per CONTRIBUTING. Docs: architecture line 720 should list what is covered above level 5. Guard: golden tests (`-update` after review). The same gap hides other level 6+ numbers: Monk Diamond Soul (all saves), Paladin Aura of Protection, Rogue Slippery Mind, Champion Remarkable Athlete (all "not built (doc)").
- **R2-7.** Code: also upgrade `VersatileDamage` in `attacks.go`. Guard: the monk dagger test.
- **R2-8.** Code: add an unarmed strike line (1 + STR, or the martial arts die with DEX for a monk). Docs: architecture "attacks".
- **R2-9.** Code: a `Hint` per heavy weapon when the race size is Small (needs the race size in the deriver).
- **R2-10.** Decide the rule: warn above 20 only for scores without a recorded item bonus, or add a separate field for items. Guard: the level-up "ability above 20" tests.

## Checklist

| rule | status |
| --- | --- |
| Ability modifier = floor((score-10)/2), scores 1-30 | covered by `TestDerive`, golden |
| Racial and subrace increases (9 races, 4 subraces) | covered by `TestDeriveEachRace` |
| Score cap 20 | finding R2-10 (level-up path covered by `TestLevelUpRefusals`) |
| ASI levels per class | covered by `levelup_test.go`; data checked |
| Three ways to make scores, 4d6 drop lowest | covered by `abilitymethods_test.go`, `TestRN24_*` |
| Proficiency bonus by total level | covered by `TestDeriveRules` |
| Saving throws (first class) | covered by `TestDeriveEachClassAtLevel1` |
| Skills, expertise, Jack of All Trades | covered by `TestDeriveRules` ("bard 2", "rogue 1") |
| Remarkable Athlete | not built (doc) |
| Passive scores, initiative | covered by `TestDerive` |
| AC for every SRD armor, shield, Unarmored Defense, Draconic Resilience | covered by `TestDeriveRules` |
| Strength requirement and speed penalty | finding R2-3 |
| Stealth disadvantage | covered (hint) |
| Hit points, hit dice, Con change, Dwarven Toughness, Draconic | covered by `TestDeriveRules` ("rolled hit points", "draconic sorcerer 1") |
| Speed: Fast Movement, Unarmored Movement | covered by `TestDeriveRules` |
| Senses, languages, tool and weapon proficiencies | covered by `TestDeriveEachRace` |
| Multiclass proficiencies, prerequisites, saves | covered by `TestDeriveRules` ("multiclass prerequisites") |
| Backgrounds (the SRD has the Acolyte) | covered by `TestDeriveEachClassAtLevel1` |
| Weapon attack and damage (finesse, ranged, thrown, versatile) | covered by `TestDerive` attacks |
| Fighting styles | finding R2-2 |
| Martial Arts | findings R2-1, R2-7, R2-8 |
| Extra Attack 11/20, Primal Champion | findings R2-5, R2-6 |
| Diamond Soul, Aura of Protection, Slippery Mind | not built (doc) |
