# Rules review: R3, spellcasting

- Scope: spell slots, multiclass table, pact magic, cantrips/spells known/prepared, DC/attack, always-prepared spells, cantrip and slot damage, concentration DC, HP-reading spells, targets/areas, summons; code in `rules`, `characters`, `play`, web cast flow.
- Commit reviewed: `933df75` (main). SRD 5.1 data: 5e-database `a8abc93b235c158bb8cbf042e54425b9c2fd79b8`; SRD prose for subclass tables from the public SRD 5.1 markdown.
- Model: claude-sonnet-5-5 (main session); verification subagents also `sonnet`.
- Checked by sweep and found correct: all 8 caster class tables (cantrips, known, slots, pact slots vs SRD), save DC / attack bonus / prepared max at scores 1-30 and levels 1-20, multiclass slot table over every class pair and level split, Sleep/Color Spray/Power Word/Heal numbers, Conjure Animals/Animate Dead/Find Familiar counts, concentration DC formula, cantrip scaling by character level (server).

## Findings

| id | severity | status | file:line | defect | scenario (app vs SRD) | evidence |
| --- | --- | --- | --- | --- | --- | --- |
| R3-1 | critical | confirmed | `rules/attacks.go:170`, `srd51/data/spells.json` (eldritch-blast) | Eldritch Blast never gets extra beams | Warlock 5/11/17: sheet shows one attack, 1d10 (same as level 1). SRD, Eldritch Blast: 2/3/4 beams, each its own attack roll and 1d10 (plus Agonizing Blast per beam) | `TestRulesReviewSpells_EldritchBlastBeams` (rules) |
| R3-2 | high | confirmed | `characters/combatspells.go:73-78`, `play/link` Spell.Damage | Spells with two damage types keep only the first | Ice Storm slot 4: app 2d8 bludgeoning, SRD 2d8 + 4d6 cold. Meteor Swarm slot 9: 20d6 fire, SRD 40d6 (fire + bludgeoning). Flame Strike slot 5: 4d6 fire, SRD 4d6 fire + 4d6 radiant | `TestRulesReviewSpells_IceStormTwoTypes`, `_MeteorSwarmTwoTypes`, `_FlameStrikeTwoTypes` (characters) |
| R3-3 | high | confirmed | `srd51/data/spells.json` (scorching-ray, flame-blade), `characters/combatspells.go:59`, `play/combat_spells.go:548` | Spells the SRD makes spell attacks have no attack type, so the cast rolls nothing | Scorching Ray: app spends the slot, no attack roll, no damage (2d6 per ray expected); Flame Blade same (melee spell attack, 3d6) | `TestRulesReviewSpells_ScorchingRayIsASpellAttack`, `_FlameBladeIsASpellAttack` (rules) |
| R3-4 | high | confirmed | `rules/spellcasting.go:173-190,357-361`, data `subclass:fiend` | Fiend warlock gets the patron spells free as always-prepared | Warlock 5 Fiend, nothing chosen: app has command, burning hands, blindness/deafness, scorching ray, fireball, stinking cloud prepared (plus 6 known). SRD, Fiend Expanded Spell List: they only join the warlock list to choose from, and cost known spells | `TestRulesReviewSpells_FiendSpellsAreNotFree` (rules) |
| R3-5 | high | confirmed | `srd51/data/spells.json` (spirit-guardians) | Spirit Guardians has no damage data | Slot 3: app no roll, SRD 3d8 (+1d8 per slot above 3rd; slot 5 = 5d8) | `TestRulesReviewSpells_SpiritGuardiansNoDamage` (rules). Same gap, not tested: Spike Growth, Web, Earthquake, Weird, Teleport, Arcane Hand |
| R3-6 | high | confirmed | `srd51/data/spells.json` (disintegrate, freezing-sphere, phantasmal-killer, wall-of-fire), `rules/spelldetails.go:260` | Higher-slot dice missing from the table | Disintegrate slot 7: app 10d6+40, SRD 13d6+40. Freezing Sphere slot 7: 10d6 vs 11d6. Phantasmal Killer slot 5: 4d10 vs 5d10. Wall of Fire slot 5: 5d8 vs 6d8 | `TestRulesReviewSpells_HigherSlotScaling` (rules) |
| R3-7 | high | confirmed | `srd51/data/spells.json` (flame-strike), `characters/combatspells.go:72` | Flame Strike slot 6+ stored as unparsable "4d6 OR 5d6", so no damage at all | Cleric 11 casting at slot 6: app no damage; SRD 5d6 fire + 4d6 radiant (the extra die goes to one type of choice) | `TestRulesReviewSpells_FlameStrikeSlot6NoDamage` (characters), `_HigherSlotScaling/flame-strike` (rules). Partly documented (architecture.md:813 keeps raw text for the dialog) but not for combat |
| R3-8 | medium | confirmed | `srd51/data/spells.json` (spell:false-life, spell:aid), `characters/combatspells.go:79-86` | False Life and Aid applied as ordinary healing | False Life slot 1: app heals 1d4+4 (capped at max HP, can raise someone from 0); SRD: 1d4+4 temporary HP. Aid slot 2: app heals 5; SRD: +5 current and maximum HP | `TestRulesReviewSpells_FalseLifeIsNotAHeal`, `_AidIsNotAHeal` (characters) |
| R3-9 | medium | confirmed | `srd51/effects/spell_targets.json` (no aid entry) | Aid takes one target | App 1 target; SRD: up to three creatures (+ none per slot; HP +5 per slot above 2nd) | `TestRulesReviewSpells_AidThreeTargets` (characters) |
| R3-10 | medium | confirmed | `srd51/data/subclasses.json` subclass:life | Life Domain lacks Guardian of Faith at cleric 7 | Cleric 7 Life: app only death ward; SRD Life Domain Spells, 7th: death ward and guardian of faith | `TestRulesReviewSpells_LifeDomainSpells` (rules) |
| R3-11 | medium | confirmed | `srd51/data/spells.json` (call-lightning) | No save recorded for the bolt | Call Lightning: app Save nil, no save/damage on cast; SRD: Dex save, 3d10, half on success | `TestRulesReviewSpells_CallLightningSave` (rules) |
| R3-12 | low | policy | `srd51/data/spells.json` (flaming-sphere, web, sleet-storm, earthquake) | Saves that happen on later turns are not opened at cast | No doc names a decision; RN-22 (effects not automated) is weak support | no test |
| R3-13 | low | unverified | `play/combat_actions.go:1144` | Concentration DC ignores damage soaked by temporary HP | App: damage fully absorbed by temp HP gives no concentration reminder; SRD says "damage you take", which is ambiguous about temp HP | no test |
| R3-14 | medium | confirmed | `web/src/app/core/combat/cast-flow.ts:337` | Browser shows level-1 dice for a save cantrip | Sacred Flame at character level 11: list line and cast sheet show "1d8 de radiante", server rolls 3d8 (SRD 3d8 at 11th) | web spec `rules review spells` (3 failing; `npx ng test --watch=false --include src/app/core/combat/rules-review-spells.spec.ts`) |

Notes for other areas (one line each): Sorcerer Metamagic at levels 10 and 17 has no `choice` effect (sheet/progression; documented as handwritten effects only to level 5); web `hit-points-preview.ts:36` computes the ability modifier itself (sheet area); Warlock Mystic Arcanum and the spell swap at level-up are documented as not built.

## Fix direction

- **R3-1**: root cause: the data row has the damage per beam and nothing carries the beam count. Fix in code: give `Attack` a beam/attack count derived from character level for this cantrip (tiers 1/5/11/17), show it on the sheet and make combat roll one attack per beam; Agonizing Blast adds per beam (`effects/warlock.json` modifier). Same mistake: none other in SRD. Docs: architecture.md (attacks), rules.md combat section. Guards: `derive_test.go:531` (asserts 1d10+3 at 5), combat tests for cantrip attacks.
- **R3-2 / R3-7**: root cause: `link.Spell` has a single `Damage`. Fix in code: carry a list of damage rolls through `CombatSpell`, pending damage and the log; for Flame Strike/Ice Storm/Meteor Swarm apply each type (resistance per type). Flame Strike slot 6+ also needs the data fixed (handwritten correction: per-type tables 5d6 + 4d6 with the extra die on the chosen type) which bumps the content revision. Same mistake: Prismatic Spray, Wall of Ice. Docs: architecture.md spell details (813) and combat pending damage. Guards: `combat_spells_test.go`.
- **R3-3**: add `attack_type` for scorching-ray ("ranged"), flame-blade ("melee"), arcane-hand in `effects/corrections.json` style data fix (needs a spells correction section, bumps revision). Same mistake: check other spells whose desc says "spell attack" with empty attack_type (arcane-hand). Guards: `combat_tablespells_test.go` scorching ray tests only count targets; add a roll/damage assertion.
- **R3-4**: root cause: `subclass.spells` is used for "always prepared" for every subclass. Fix in code and data: mark fiend's list as "expanded list" (offered on the warlock choice list, counted against known), keep life/devotion/land. Docs: architecture.md spell list section. Guards: `levelup_sweep_test.go`, golden tests.
- **R3-5 / R3-6**: handwritten data in `effects/` (damage tables with the missing rows) bumping the content revision, or extend `corrections.json` to cover spells. Same mistake: arcane-hand, glyph of warding (+1d8 per slot) in the scan. Guards: `spelldetails_test.go`.
- **R3-8 / R3-9**: add a closed spell effect kind for temporary HP / max HP (next to `flat_heal` in `spelleffects.go`) and set Aid's target to 3 in `spell_targets.json`. Docs: architecture.md spell effects. Guards: `combat_spells_hp_test.go`.
- **R3-10**: add `guardian-of-faith` at class level 7 to subclass:life (data correction, revision bump). Guard: golden/derive tests.
- **R3-14**: root cause: `damageDice` has no character level. Fix: have the server send the dice at the caster's level with the spell option (ADR-0008), not the table; same mistake in `spell-summary.ts:28` and `castSubtitle`. Guard: `cast-flow.spec.ts`.
- **R3-11**: add dex/half save for call-lightning in data; decide policy for R3-12.

## Checklist

- Slots, full/half casters, paladin/ranger from 2: covered by `TestRulesReviewSpells` sweep (scratch, all tables equal SRD) and `derive_test.go`.
- Multiclass table, third casters, pact apart: verified by pair sweep; covered by `levelup_sweep_test.go`.
- Pact magic slots/level: covered by `TestCatalogMaxSpellLevelByLevel`; short-rest recovery: other area (rests).
- Cantrips known, spells known, prepared (min 1): covered by `TestDeriveEachClassAtLevel1`, `derive_test.go`.
- Save DC, attack bonus: covered by `derive_test.go`.
- Ritual casting per class: covered by `summonoptions_test.go`.
- Always-prepared domain/oath/circle spells not counted: covered; Life domain finding R3-10; Fiend finding R3-4.
- Cantrip scaling by character level: covered by `derive_test.go:531`; Eldritch Blast finding R3-1.
- Damage at higher slot: finding R3-6, R3-7.
- Concentration (one at a time, DC): covered by `combat_spells_test.go`; temp HP case R3-13.
- HP-reading spells: covered by `hpspells_test.go`, `combat_spells_hp_test.go`.
- Area shapes and sizes, targets: covered by `spelltarget_test.go`; Aid R3-9.
- Summons (Animate Dead, Conjure Animals): covered by `summon_test.go`.
- Spell attacks and saves on cast: findings R3-3, R3-11, R3-12.
- Mystic Arcanum, Spell Mastery, Signature Spells: not built (doc).
