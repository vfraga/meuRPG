# MeuRPG rules coverage: SRD 5.1 rules chapters (work in progress)

Status: chapters 1 (Using Ability Scores) and 2 (Adventuring) are judged. Combat, Spellcasting, Conditions and "anything else" follow in later pushes; the summary, the top-20 and the dependencies come last.

Source: 5e-database at `a8abc93b` (the importer's `sourceCommit`). Note: `5e-SRD-Rule-Sections.json` does not exist at that commit; the 137 sections (with their text) are all in `5e-SRD-Rules.json` (index in `review/coverage-rules/srd-rules-index.txt`).

States: built / partial / reminder / absent. Impact: high / medium / low. "Deliberate" quotes the doc that says it is left out.

## 1. Using Ability Scores

| Mechanic | State | Evidence | Deliberate? | Impact | Note |
| --- | --- | --- | --- | --- | --- |
| Ability modifier from score | built | backend/internal/rules/abilities.go:25 | – | high | Floor division, scores below 10 included. |
| Proficiency bonus by level, applied once | built | rules/abilities.go:310, :333 | – | high | |
| Half proficiency (Jack of All Trades) | built | rules/abilities.go:318, :348 (initiative) | – | low | |
| Expertise | built | rules/abilities.go:293-297; choices.go:118 | – | medium | Refuses expertise without proficiency. |
| 18 skills, bonus per skill | built | rules/abilities.go:329-341 | – | high | |
| Variant: skill with another ability | absent | no match for variant in rules/ | – | low | |
| Ability check in play (RP scene action: d20 + bonus vs DC, app or physical die) | built | play/scene.go:710-725; web .../scene-roll-sheet/scene-roll-sheet.ts:53 | – | high | One d20, no mode; DC 1 to 30 (maps/scenes.go:43). |
| Typical DC table (5/10/15/20/25/30 tiers) | absent | no preset picker; free number only (web .../scene-action-form.ts:59) | – | low | |
| Passive checks | partial | rules/abilities.go:344-346 | – | medium | Only Perception, Investigation, Insight; no passive for other skills. |
| Passive -5 for disadvantage | partial | rules/vision/vision.go:94, maps/traps_play.go:169 | – | low | Only the dim-light penalty against traps; no +5 and no generic use. |
| Contests (opposed checks) | absent | no match for contest/disputa/opposed in play/ rules/ | – | medium | Blocks grapple, shove, hide. |
| Working together / Help action | reminder | rules/srd51/effects/standard_actions.json:8; play/combat_actions.go:1799-1811 | combat.proto:742 "the rest only spend the action and go to the log" | medium | Spends the action; grants nothing. |
| Group check | absent | no match for group check/teste em grupo | – | low | |
| Advantage and disadvantage on attack rolls | absent | play/combat_actions.go:~700 (`s.d20(in, attack.ToHit)`), play/combat_spells.go:582 (one d20, no mode) | proto/meurpg/play/v1/combat.proto:639 "Disadvantage at long range or next to an enemy is not applied yet" | high | Confirmed: attacks and spell attacks roll one d20. |
| Advantage/disadvantage on ability checks and saves | absent | play/scene.go:710; play/combat_spells_view.go:38 | rules.proto:927 "The server never applies it on its own: the master decides" | high | One d20 everywhere. |
| Advantage/disadvantage sources (race, feature, armor, heavy weapon) shown as hints | reminder | rules/effects.go:47 (roll_mode), rules/choices.go:66-80, characters/derived.go:178 | glossary.md:89, tablemenu.go:140 | high | Sheet text only (Gnome Cunning, Rage, Danger Sense, stealth armor, heavy weapons). |
| Advantage + disadvantage cancel; Lucky reroll | absent | no match | – | medium | Needs advantage first. |
| Disadvantage on Perception search in dim light (two d20, lower counts) | built | play/traps.go:219-251; maps/traps_play.go:378; web trap-search-sheet.ts:147 | – | low | The only live use of disadvantage; trap search only. Docs conflict: stories.md:822 says "no disadvantage". |
| Saving throw bonus (class proficiencies) | built | rules/abilities.go:208-232; play/combat_spells_view.go:33 | – | high | |
| Spell save DC / spell attack bonus | built | rules/spellcasting.go:55 | – | high | |
| Death saving throw | built | play/combat_death.go:133 | – | high | |
| Concentration save (Con, DC 10 or half damage) | partial | rules/combat/rolls.go:117; play/combat_actions.go:1215 | rules.md RN-22 "reminds" | high | Server computes the DC and shows it; no roll, the table rolls and ends concentration by hand. |
| Saving throw halves or negates damage | built | rules/combat/rolls.go:138; rules/traps.go:50 | – | high | |
| Carrying capacity, push/drag/lift, encumbrance | absent | no match for carrying/carga/encumb; items have no weight | – | low | |
| Initiative = d20 + Dex (+ bonuses), per NPC | built | play/combat.go:389, :460; rules/abilities.go:348 | RN-19 | high | |
| Armor Class from Dex/armor/unarmored | built | rules/armor.go:11-27, abilities `x.d.ArmorClass` armor.go:82 | – | high | |
| Hit points: Con per hit die | built | rules/hitpoints.go:22-48 | – | high | |
| Long/high jump from Strength | built | rules/combat/jump.go:33-51; play/combat_move.go:399-425 | – | medium | |
| Acrobatics DC 10 on difficult landing | reminder | play/combat_move.go:493-496; web combat-log.ts:488 | "The app rolls nothing" (code comment) | low | |
| Stealth/Perception contest for Hide | absent | no match | – | medium | Hide only spends the action. |
| Athletics climb/swim checks, Medicine, Survival, Animal Handling flows | partial | generic via rules/progression.go:215 scene options | – | medium | Any skill can be asked as a scene check; no dedicated flows. |

## 2. Adventuring

| Mechanic | State | Evidence | Deliberate? | Impact | Note |
| --- | --- | --- | --- | --- | --- |
| Time scales (minutes, hours, days) | absent | no match in play/ rules/ | – | low | No in-game clock. |
| Travel pace (fast/normal/slow), distance per day | absent | no match for travel/ritmo/pace | – | low | |
| Forced march (Con save, exhaustion) | absent | no match | – | low | |
| Mounts and vehicles pace | absent | rules/summon.go:15 | summon.go:15 "mounted combat after the MVP" | low | |
| Speed in combat (walk/fly, Dash x2, 0 for grappled etc.) | built | play/combat_move.go:140-160 | – | high | |
| Difficult terrain (+5 ft per square) | built | rules/grid/move.go:82; web map-editor.ts:105 | – | high | Combat only, not travel. |
| Climbing, swimming, crawling cost; Athletics checks | absent | play/combat_move.go:133 "out of scope"; rules/api.go:594 speeds unused | architecture.md:1849 | medium | Climb/swim speeds are on the sheet but ignored. |
| Long jump / high jump | built | rules/combat/jump.go; play/combat_move.go:399-425; web jump-panel.ts | rules.md:293 | medium | Low-obstacle Athletics check and arm-reach rule absent. |
| Falling damage | partial | rules/traps.go:169 (FallDice) | – | medium | Only inside pit traps; no general fall (cliff, flight loss). |
| Suffocating, holding breath | absent | no match | – | low | |
| Lightly obscured / heavily obscured | partial | rules/vision/vision.go:25, :78; maps/traps_play.go:378 | – | medium | Fog of war uses it; no blinded effect from darkness, no disadvantage for attacks. |
| Light levels, light sources, painted light | built | rules/vision/vision.go; rules/srd51/effects/lights.json | – | high | |
| Darkvision, blindsight, truesight | partial | rules/vision/vision.go:19-23, :258-269 | – | medium | Ranges drive the fog; truesight sees darkness only (no invisibility or illusions). |
| Food and water, starvation | absent | no match | – | low | |
| Interacting with objects, object AC/HP, breaking objects | absent | only the "Use an Object" action (spends the action) | combat.proto:742 | low | |
| Traps: eight SRD samples, severity tables, notice/find DC, save/attack effects | partial | rules/traps.go; srd51/effects/traps.json; play/traps*.go; maps/traps_play.go | stories.md MR-035 | medium | Pits: 2 of 4 variants; Sphere of Annihilation absent; complex traps (initiative) absent. |
| Trap detection (passive notice, active search) | built | maps/traps_play.go:266, :363; play/traps.go:214 | – | medium | |
| Trap disarm | reminder | maps/traps_play.go:661 (no dice); stories.md:820 | – | low | Master clicks "Desarmar" after the table's check. |
| Diseases (general and the 3 samples) | absent | no match for doença/disease | – | low | |
| Madness (all four sections and tables) | absent | no match | – | low | |
| Short rest (hit dice spend and roll) | absent | only master-set `hit_dice_used`: characters/vitals.go:298; web adjust-vitals.ts:116 | rules.md:51 claims "a rest" is an action; no such RPC | high | Docs wrong: no rest action exists (`AdjustCharacterVitals` is the only vitals RPC). |
| Long rest (HP, half hit dice, slots, resources, duration, once per 24 h) | absent | same | same | high | Table restores HP, slots and uses by hand through the master correction. |
| Resource recharge labels (short/long/dawn) | reminder | rules/effects.go:66, rules/actions.go:21; web content-read.ts:48 | glossary.md:43 | high | Label says when it returns; nothing resets it. |
| Arcane Recovery, Natural Recovery | reminder | rules/srd51/effects/wizard.json:8, druid.json:51 | – | medium | |
| Lifestyle expenses | absent | no match | – | low | |
| Downtime (crafting, profession, recuperating, research, training) | absent | no match | – | low | |
| Exhaustion (see Conditions chapter) | reminder | web core/combat/conditions.ts:24 | – | high | |
