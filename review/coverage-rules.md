# MeuRPG rules coverage: SRD 5.1 rules chapters (work in progress)

Status: complete. Every row below was judged by reading the code; the haiku sweeps' raw lists are in `review/coverage-rules/raw-*.md` (`raw-adventuring.md` was not written by its sweep, which was read-only; its content is folded into chapter 2). Nothing in the repository was changed.

Source: 5e-database at `a8abc93b` (the importer's `sourceCommit`). Note: `5e-SRD-Rule-Sections.json` does not exist at that commit; the 137 sections (with their text) are all in `5e-SRD-Rules.json` (index in `review/coverage-rules/srd-rules-index.txt`).

States: built / partial / reminder / absent. Impact: high / medium / low. "Deliberate" quotes the doc that says it is left out.

## Summary

Scope: the SRD 5.1 rules chapters (Using Ability Scores, Adventuring, Combat, Spellcasting, the 15 conditions and exhaustion, and the Equipment/Appendix rules). 187 mechanics, one row each (rows differ in size, so read the counts as a rough shape; `review/coverage-rules/count.py` recomputes them).

| State | high | medium | low | total |
| --- | --- | --- | --- | --- |
| built | 50 | 17 | 6 | 73 |
| partial | 6 | 16 | 6 | 28 |
| reminder (text only) | 9 | 8 | 3 | 20 |
| absent | 21 | 16 | 29 | 66 |
| **total** | 86 | 57 | 44 | 187 |

The shape: the **rolls and the bookkeeping around a single attack or spell are solid** (initiative, turn economy, movement on the grid, opportunity attacks, cover, damage and critical hits, temp HP, death saves, saving throws, spell slots, upcasting, cantrips, light and fog). What is missing is almost everything that **modifies a d20**: advantage and disadvantage exist nowhere in the engine, so every condition, Dodge, Help, Hide, prone, long range, unseen targets and the like is a label. The second hole is **time and recovery**: there is no rest action, no duration, no condition expiry, no concentration loss. The two known facts are confirmed: attack and spell-attack rolls use one d20 (`play/combat_spells.go:582-595`, `play/combat_actions.go:720`), and a player character's resistances are never applied (`play/combat_actions.go:1127`, `characters/charactercreatures_roster.go:536-545`; `docs/architecture.md:1847` item 7 admits it).

Docs that disagree with the code (found while verifying):
- `docs/product/rules.md:51` (RN-02) says the system tracks HP and slots from "a rest"; no rest RPC exists, only the master's `AdjustCharacterVitals`.
- `docs/product/stories.md:822` says an active check in dim light has no disadvantage; the code gives a Perception trap search disadvantage (`play/traps.go:251`), and `stories.md:818` and `architecture.md:1928` say so.
- `docs/product/stories.md:825` says "eight SRD presets" for traps; the SRD pits have four variants and the app has two, and the Sphere of Annihilation is absent.
- `5e-SRD-Rule-Sections.json` does not exist at the pinned commit; all sections are in `5e-SRD-Rules.json`.

### The 20 gaps with the highest table impact

Ranked by how often a normal table would hit them, then by how many other rows they unblock.

1. **Advantage and disadvantage on d20 rolls** (attacks, checks, saves): absent; the sheet only shows hints. (Ch. 1, 3)
2. **Rests**: no short rest (spend and roll hit dice), no long rest (HP, half the hit dice, slots, resource recharge); the master types values by hand. (Ch. 2)
3. **Resistance, vulnerability and immunity of player characters**: never applied (Rage, racial, spells, items). (Ch. 3)
4. **Condition effects**: all 15 are labels; only speed 0 and "no reaction" are read. (Ch. 4)
5. **Dodge**: spends the action, no effect. (Ch. 3)
6. **Duration and expiry**: spell durations and condition durations never run out. (Ch. 4, 5)
7. **Concentration**: the Con save is never rolled in the app, and it does not end on incapacitation or death. (Ch. 1, 5)
8. **Surprise**: no surprised state; first-turn and reaction rules absent. (Ch. 3)
9. **Long-range and close-range disadvantage on ranged attacks**; unseen target/attacker. (Ch. 3)
10. **Hide** (Stealth vs passive Perception) and **contests** in general. (Ch. 1, 3)
11. **Grappling and shoving** (and their escape/contest rules). (Ch. 3)
12. **Prone**: no drop, stand-up cost, crawling or attack modifiers. (Ch. 3, 4)
13. **Exhaustion levels**: a checkbox with no counter and no effect. (Ch. 4)
14. **Area of effect geometry**: the caster picks targets by name; no shape on the map. (Ch. 5)
15. **Consumables used in play** (potions, scrolls, ammunition): never counted or spent. (Ch. 6)
16. **Spells that only ask a save** (49) and the 187 text-only spells: nothing is applied. (Ch. 5)
17. **Condition-applying spells** (Hold Person, Charm Person, etc.): only 3 spells set a condition. (Ch. 5)
18. **Unconscious and prone at 0 HP, instant death from massive damage**: not applied. (Ch. 3)
19. **Slot and resource recovery on rest** (Arcane Recovery, Natural Recovery): text only. (Ch. 2, 5)
20. **Help action and group checks**: no effect; no group check. (Ch. 1, 3)

Close behind (medium): rituals, spell components, knocking a creature out, stabilizing with Medicine, free object interaction, charges and attunement counts, weapon loading/two-handed/ammunition, creature footprints larger than one square, mounted and underwater combat.

## Dependencies: which gaps block which

```
Advantage / disadvantage on a d20  (root)
├─ Dodge, Help, Hide (the actions only matter through advantage)
├─ Ranged long range / adjacent-enemy disadvantage, unseen attackers and targets
├─ Condition effects: blinded, frightened, poisoned, prone, restrained, invisible,
│  paralyzed/unconscious/stunned (advantage against), exhaustion level 3+
├─ Racial / feature advantage now shown as hints (Gnome Cunning, Rage, Danger Sense,
│  Brave, Fey Ancestry, Pack Tactics-like monster traits)
├─ Sneak Attack's "has advantage or an ally within 5 ft" condition (class features, mapped elsewhere)
├─ Stealth disadvantage armor, heavy weapon for Small, Perception in dim light
└─ Cancellation rule and Lucky-style rerolls

Contests (opposed checks)  (root)
├─ Grapple, shove, escape
├─ Hide vs passive Perception (also needs Hide state -> unseen attackers)
└─ Group checks / Help (share the check machinery)

Rest action  (root)
├─ HP / hit dice recovery, hit dice spending
├─ Spell slot and pact slot recovery
├─ Resource recharge (Rage, Ki, Second Wind, Channel Divinity, Arcane/Natural Recovery)
├─ Exhaustion removal, temp HP expiry
└─ Death-save and concentration cleanup at rest

Duration / timers  (root)
├─ Spell durations (Bless, Haste, Mage Armor, ...), condition expiry
├─ Concentration ending after a time and on incapacitation (needs condition reads)
└─ Hit dice / long rest 8 h rules, travel and downtime (need an in-game clock)

Condition reads in the engine  (needs Advantage for most)
├─ Incapacitated must block actions (cheap; independent)
├─ Unconscious/prone automatically at 0 HP (cheap; independent)
├─ Concentration ends on incapacitated/unconscious/dead (cheap; independent)
└─ Auto-crit within 5 ft vs paralyzed/unconscious (needs a melee-distance test, no advantage)

Character resistance  (independent)
└─ Needs a source: Rage on/off state, racial traits, item bonuses, spell effects (Absorb Elements, Protection from Energy) -> shares the "active effect with duration" gap

Area geometry  (independent)
└─ Needs a point of origin on CastSpell (architecture.md:1847 item 4) and the same geometry feeds cover from the origin

Consumable/charge tracking  (independent)
└─ Needs per-item state on the sheet (equipment is free text + quantity today)
```

Practical reading: **three roots unblock the most for a first session**: advantage/disadvantage (one d20 vs two, a mode on `RollAttack`, `CastSpell` saves and `RollSceneCheck`), a rest action, and "active effects with a duration" (conditions, concentration, spell durations, Rage). Incapacitated blocking actions, unconscious at 0 HP, and concentration ending on incapacitation need none of them.

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

## 3. Combat

Paths are under `backend/internal/` unless they start with `web/` or `docs/`. "KL" = `docs/architecture.md:1847` ("Known limits of the combat engine").

### 3a. Order of combat, movement and position, actions

| Mechanic | State | Evidence | Deliberate? | Impact | Note |
| --- | --- | --- | --- | --- | --- |
| Rounds and turns in initiative order | built | play/combat_turn.go:63-143 | – | high | Skips the defeated, counts rounds. |
| Initiative roll (d20 + Dex, each NPC its own) | built | play/combat.go:389, :460, :544 | RN-19 | high | |
| Initiative ties | partial | play/combat_rules.go:61-103; combat.go:599 SetInitiativeOrder | – | low | Sorts by total then bonus; master orders unresolved ties; no optional d20 re-roll. |
| Joint turn for equal totals | built | play/combat_turn.go:31-43 | project rule, not SRD | medium | |
| Joining a running combat | partial | play/combat.go:991-1034 | – | medium | NPCs may join; a player's character may not join once the combat began. |
| Surprise (surprised creatures skip first turn and reaction) | absent | no match for surpris/surpres in play/ rules/ characters/ proto docs | – | high | Stealth vs passive Perception to decide it also absent. |
| Move + action in either order, split movement | built | play/combat_move.go (no action gate); queries.sql:231 reset | – | high | Movement between the attacks of one Attack action is allowed. |
| Bonus action (one per turn, only from features/spells) | built | rules/combat/turn.go:383; rules/actions.go:69 | – | high | |
| Bonus-action spell limit | built | rules/combat/turn.go:392-494 | – | medium | |
| Reaction (one, resets on own turn) | built | play/combat_actions.go:1728; queries.sql:231 | – | high | |
| Opportunity attack (offer, Disengage, incapacitated reactors) | built | play/combat_opportunity.go:20-155; combat_actions.go:573-663 | – | high | Melee only, offer to the reactor; forced moves exempt. |
| Free object interaction | absent | no match; only doors open on a move (play/combat_move.go:552) | – | low | |
| Dash | built | play/combat_actions.go:1799; combat_move.go:156 | KL(1) second Dash gives x2 not x3 | high | |
| Disengage | built | play/combat_actions.go:1806; combat_opportunity.go:96 | – | high | |
| Dodge | reminder | play/combat_actions.go:1732-1753 (spends action only); no dodge flag | KL(2) "Patient Defense's Dodge only logs" | high | Needs advantage/disadvantage. |
| Help | reminder | same path | combat.proto:742 | medium | Needs advantage. |
| Hide | reminder | same path; `Hidden` is the master's toggle (play/combat.go:927) | – | high | No Stealth roll, no contest. |
| Ready (trigger, readied spell) | reminder | same path; no trigger stored | – | medium | The later reaction is an ordinary reaction. |
| Search (combat action) | reminder | same path; trap search is separate (play/traps.go:336) | – | medium | |
| Use an Object | reminder | same path; web combat-log.ts:455 | – | medium | |
| Attack action, Extra Attack | built | rules/combat/turn.go:367; play/combat_actions.go:1644 | – | high | |
| Cast a Spell action | built | rules/combat/turn.go:458-494 | – | high | Casting times of a minute or more refused in combat. |
| Improvised actions | absent | play/combat_actions.go:1685 refuses unknown keys | – | low | |
| Difficult terrain (+5 ft/square, creatures too) | built | rules/grid/move.go:249-251 | RN-21 | high | |
| Being prone (drop, stand up costs half speed, crawl) | absent | play/combat_move.go:142 comment "not modeled" | comment only | high | Prone is a label; nothing reads it. |
| Moving through creatures (allies, two sizes apart, no ending on one) | built | rules/grid/move.go:149-160, :255 | – | medium | |
| Flying movement, falling when it cannot move | partial | play/combat_move.go:131 | – | low | Flier ignores terrain; no falling. |
| Creature size and space | partial | rules/grid/move.go:29-41 | – | medium | Size only rules passing; every creature takes exactly one square. |
| Squeezing | absent | no match (grid "squeeze" is wall corners) | – | low | |
| Speed 0 from conditions | built | play/combat_move.go:144-160 | – | medium | Grappled, restrained, paralyzed, petrified, stunned, unconscious. |

### 3b. Attacks, cover, damage, death, mounts

| Mechanic | State | Evidence | Deliberate? | Impact | Note |
| --- | --- | --- | --- | --- | --- |
| Attack roll vs AC, nat 20 hits and crits, nat 1 misses | built | rules/combat/rolls.go:39-52; play/combat_actions.go:728-734 | – | high | |
| Attack modifier (Str/Dex/finesse, proficiency) | built | rules/attacks.go:45-65 | – | high | |
| Advantage/disadvantage on attacks (any source) | absent | play/combat_spells.go:582-595, combat_actions.go:720 | combat.proto:639; KL(2) | high | Confirmed: one d20. |
| Unseen attacker/target (advantage, disadvantage, guess square) | absent | no match; a player cannot target what the fog hides (combat_actions.go:591-604) | – | high | |
| Ranged: long range disadvantage | absent | combat_actions.go:181-191, :686 (range is a limit only) | combat.proto:639 | high | |
| Ranged: hostile within 5 ft disadvantage | absent | no match | combat.proto:639 | high | |
| Melee reach | built | combat_actions.go:45, :181-196 | – | high | Players are limited by reach on a map; master is not. |
| Unarmed strike | built | rules/attacks.go:143-153 | – | medium | |
| Two-weapon fighting | built | combat_actions.go:788; rules/combat/bonusattack.go:67-82 | – | medium | |
| Grappling, escaping, moving a grappled creature | absent | no match; label "Agarrado" only (web conditions.ts:17) | – | high | Needs contests. |
| Shoving | absent | no match; forced move is the master's (combat_move.go:273) | – | medium | |
| Cover (half +2, 3/4 +5, total) on AC and Dex saves | built | play/combat_cover.go:16-127; rules/grid/cover.go:11; combat_spells.go:641 | KL(4) area-spell cover measured from the caster | high | Master can override; doors count as walls. |
| Damage roll, damage rolled once for all targets of a spell | built | combat_actions.go:974-1013 | – | high | |
| Critical hit (double dice; table option max + roll) | built | rules/combat/rolls.go:80-88; combat_actions.go:730 | KL(3) Brutal Critical not built | high | Spells crit only on a natural 20. |
| 13 damage types | built | proto/meurpg/characters/v1/characters.proto:1277-1291 | – | high | |
| Resistance/vulnerability/immunity of NPCs and creatures | partial | rules/combat/damagetype.go:18-32; play/combat_actions.go:1126 | KL(7) | high | Applied only for plain stat-block entries; conditional ones ("nonmagical weapons") are left to the master. |
| Resistance/vulnerability/immunity of player characters | absent | play/combat_actions.go:1127 (`holdsHP`), characters/charactercreatures_roster.go:536-545 (empty for players), traps_damage.go:28 | KL(7) "not applied" | high | Confirmed: Rage, Tiefling, Dwarf, Absorb Elements, items are notes; the master edits the amount. |
| Hit points and temp HP absorb first | built | rules/combat/vitals.go:18-28 | – | high | |
| Temp HP do not stack; healing does not restore them | built | play/combat_spells_hp.go:305; combat_actions.go:1173 | – | low | Manual set has no max rule (characters/vitals.go:385). |
| Healing capped at max; from 0 revives and resets death saves | built | rules/combat/vitals.go:39; play/combat_vitals.go:43; combat_actions.go:1171-1208 | – | high | |
| Dead cannot be healed | absent | no match for a refusal in play/combat_actions.go healCombatant or characters/vitals.go | – | low | A dead character is a state of the character, not of HP; the table just does not heal it. |
| Dropping to 0: unconscious + prone automatically | absent | no match; condition not added (combat_actions.go:1371) | – | medium | The player sees the death-save state, but no unconscious label. |
| Instant death (excess damage >= max HP) | absent | rules/combat/vitals.go:10-25 computes `Excess`; nothing reads it | vitals.go:12 "the master decides (RN-03)" | medium | |
| Death saves (d20, 1 = two, 20 = revive, 3 fails -> master confirms death) | built | play/combat_death.go:56-278; rules/combat/vitals.go:71 | – | high | |
| Damage at 0 HP = failure (critical = two) | built | combat_actions.go:1371; combat_death.go:285; rules/combat/vitals.go:86 | – | high | |
| Stabilize: Medicine check DC 10 | absent | no match | – | medium | |
| Stabilize: Spare the Dying | built | play/combat_spells_hp.go:153-170 | – | low | Sets 3 successes. |
| Stable regains 1 HP after 1d4 hours | absent | no match | – | low | |
| Knocking a creature out (non-lethal melee) | absent | no match; `openHit` stores no choice (play/combat_reactions.go:66) | – | medium | |
| Monsters die at 0 HP | built | combat_actions.go:1153 | – | high | |
| Mounted combat (mounting, controlling, mount opportunity attacks) | absent | no match | rules/summon.go:15 "mounted combat comes after the MVP" | low | |
| Underwater combat | absent | no match; combat_move.go:134 swimming out of scope | combat_move.go:134 | low | |

## 4. Conditions (15) and exhaustion

All paths under `backend/internal/` unless `web/`. General: the master marks conditions as labels (`play/combat_conditions.go:26-146`, players cannot); the screen says "Só rótulos: o app não aplica os efeitos" (web `.../conditions-dialog/conditions-dialog.ts:47`), and shows only the name, never the SRD effect text (web `core/combat/conditions.ts:17-33`). Deliberate: `docs/product/rules.md` RN-22 ("The app marks conditions ... and reminds"). Conditions never expire by themselves; removal is manual or by Heal's `ends` (`rules/srd51/effects/spells.json:46`). Conditions are not on the character sheet. Impact is high for the whole group because they depend on advantage/disadvantage (see Dependencies).

| Condition: effect | State | Evidence | Deliberate? | Impact | Note |
| --- | --- | --- | --- | --- | --- |
| Marking a condition on a combatant, listing the 15 | built (label) | play/combat_conditions.go:134-146; web core/combat/conditions.ts:17 | RN-22 | high | Visible to every viewer who sees the combatant (play/combat_view.go:293). |
| Conditions applied automatically by an effect | partial | rules/srd51/effects/spells.json:7,13,18 (Sleep -> unconscious, Color Spray -> blinded, Power Word Stun -> stunned); play/traps_effect.go:183-214 (traps) | – | medium | Few spells; most condition-giving spells (Hold Person, Charm Person, Bless-like) set nothing. Sleep/Color Spray/Stun apply with no save of their own. |
| Condition duration / expiry | absent | no timer anywhere; `duration_pt` is text | – | high | |
| Blinded: auto-fail sight checks; attacks against have advantage, its attacks disadvantage | absent | no read of the key for rolls; only `cantReact` (play/combat_opportunity.go:43) | – | high | Server also blocks the blinded creature's opportunity attacks, which the SRD does not say. |
| Charmed: cannot attack the charmer; charmer has advantage on social checks | absent | no read | – | medium | |
| Deafened: auto-fail hearing checks | absent | only set/cleared (play/familiarsight.go:49) | – | low | |
| Frightened: disadvantage while source in sight; cannot move closer | absent | no read | – | high | |
| Grappled: speed 0 | built | play/combat_move.go:145 | – | medium | No grapple action, so it is hand-marked. Ends-on-separation rule absent. |
| Incapacitated: no actions or reactions | partial | reactions: play/combat_opportunity.go:43, combat_theatre.go:365; actions: not blocked; speed not zeroed | – | high | An incapacitated creature can still take an action and walk. |
| Invisible: heavily obscured, advantage on attacks, attacks against disadvantage | absent | no read | – | medium | |
| Paralyzed: incapacitated, speed 0, auto-fail Str/Dex saves, attacks advantage, auto-crit within 5 ft | partial | speed 0 and no reactions only (combat_move.go:145; combat_opportunity.go:43) | – | high | No auto-fail saves, no advantage, no auto-crit. |
| Petrified: as paralyzed plus resistance to all damage, immune to poison/disease | partial | speed 0, no reactions only | – | low | |
| Poisoned: disadvantage on attacks and checks | absent | no read; also traps apply it (traps.json:141) | – | high | |
| Prone: crawl, stand up, attacks advantage within 5 ft / disadvantage beyond, own attacks disadvantage | absent | play/combat_move.go:142 comment "not modeled" | comment | high | Prone is set by pit traps and jumps (combat_move.go:494) but nothing reads it. |
| Restrained: speed 0, attacks disadvantage, against advantage, Dex save disadvantage | partial | speed 0 only (combat_move.go:145) | – | medium | |
| Stunned: incapacitated, speed 0, auto-fail Str/Dex, advantage against | partial | speed 0, no reactions only | – | medium | |
| Unconscious: incapacitated, drops items, prone, auto-fail Str/Dex, advantage against, auto-crit within 5 ft | partial | speed 0 and no reactions; Sleep's pool skips it (play/combat_spells_hp.go:115); ends a druid's Wild Shape (play/combat_conditions.go:90, wildshape.go:119-154); not added at 0 HP | – | high | |
| Concentration ends when incapacitated or killed | absent | no code path | rules.md:329 "the master decides" | medium | Master or player ends it by hand (SetCombatantConditions end_concentration). |
| Exhaustion, six levels (level counter, disadvantage on checks, speed halved, HP max halved, speed 0, death) | reminder | web core/combat/conditions.ts:24 is a plain on/off checkbox; no counter; no effect | – | high | Nothing removes a level on a long rest; forced march, starvation and disease feed it by hand. |

## 5. Spellcasting

Paths under `backend/internal/` unless `web/`. Machine coverage of the 319 SRD spells (counted from `rules/srd51/data/spells.json` and `effects/spells.json`, see `raw-spellcasting.md`): 12 have an engine effect entry (3 summon, 2 hp_pool, 2 hp_threshold, 1 zero-HP, 1 flat heal, 1 temp HP, 1 max HP, 1 ignores cover); 66 have damage the server rolls; 10 heal; 16 are spell attacks; 92 ask a saving throw the server rolls; 132 have some server mechanic; 187 are text only. 49 save spells have no damage, heal or effect, so a failed save changes nothing in the app.

| Mechanic | State | Evidence | Deliberate? | Impact | Note |
| --- | --- | --- | --- | --- | --- |
| Spell level, spell slots by class/level, pact slots | built | rules/spellcasting.go:55; characters/vitals.go:298 | – | high | Slots are tracked per level, and spent on a cast. |
| Slot spent on cast (player characters) | built | play/combat_spells.go:391-397 | – | high | NPCs and creatures spend nothing. |
| Slot recovery (rest) | absent | see Adventuring: no rest action | rules.md:51 claims it | high | The master fixes slots by hand. |
| Known and prepared spells, class lists, validation | built | rules/spelllist.go; rules/validate.go; characters/spelllist.go | – | high | The player's spell list screen too. |
| Upcasting: more damage dice and more targets | built | rules/spelldetails.go:269-290; play/combat_spells.go:159-180; rules/combat/rolls.go:127 (Magic Missile) | – | high | Only for spells whose data has the scaling. |
| Cantrips scale by character level | built | characters/combatspells.go:71; rules/spelldetails.go:237-251 | – | high | |
| Rituals (cast without a slot, +10 min) | absent | no ritual flag on CastSpell; only summons outside combat (play/creature_cast.go:194-270) | – | medium | |
| Casting time: action / bonus action / reaction | built | rules/combat/turn.go:392-494 | – | high | Reaction spells (Shield, etc.) only on their trigger. |
| Casting time of a minute or longer | partial | rules/combat/turn.go:477 refuses in combat | – | low | Out of combat the cast is not tracked at all. |
| One bonus-action spell per turn rule | built | rules/combat/turn.go (BonusActionSpellLimit) | – | medium | |
| Range (checked on a map for players) | partial | play/combat_spells.go:534-552 | RN-25 | medium | Master is exempt; without a grid not checked; measured from the caster. |
| Components V/S/M, costly or consumed material, focus, free hand, silence | absent | no match for component/material in the cast path (play/combat_spells.go) | – | medium | Nothing stops a gagged caster or one without the diamond. |
| Targets: clear path, total cover | partial | play/combat_spells.go:538 | – | medium | Total cover refuses a single-target spell; area spells exempt. |
| Areas of effect (cone, cube, cylinder, line, sphere) | reminder | rules/ stores the shape; no geometry in play/ | rules.md:344 "the caster picks the targets, no area drawn" | high | The caster ticks targets by name. |
| Saving throw spells: server rolls target save vs caster DC | built | play/combat_spells_view.go:33-38; rules/spellcasting.go:55 | – | high | Half on success where the data says so; cover bonus on Dex saves (combat_spells.go:641). |
| Save with no damage or effect (49 spells) | reminder | effects/spells.json holds only 12 entries | rules.md:329, :336-340 "the master decides" | high | The log shows the result; nothing is applied. |
| Spell attack rolls | built | play/combat_spells.go:598-612 | – | high | One d20; no disadvantage rules. |
| Damage and healing spells | built | play/combat_actions.go:974-1013 | – | high | 66 damage + 10 heal spells. |
| Condition-applying spells | partial | effects/spells.json:7,13,18 | – | high | Only Sleep, Color Spray, Power Word Stun apply conditions. |
| Summoning spells | partial | rules/summon.go; play/creature_cast.go | – | medium | 3 spells (Animate Dead, Conjure Animals, Find Familiar). Find Steed out. |
| Concentration: one at a time (new cast replaces the old) | built | play/combat_spells.go:418-429 | – | high | |
| Concentration: save after damage | reminder | rules/combat/rolls.go:117; play/combat_actions.go:1211-1219 | RN-22 | high | DC shown, nobody rolls it in the app. |
| Concentration ends on incapacitated / unconscious / death | absent | play/combat_spells_hp.go:269-303, combat_death.go:247 do not touch it | rules.md:329 | high | |
| Duration (rounds, minutes, hours) | absent | text only; no round counter or expiry | – | high | Spell effects never time out; the master ends them. |
| Combining magical effects (same spell does not stack) | absent | no match | – | low | |
| Magic school | built (data) | rules/srd51/data/spells.json; filters in ListContent | – | low | Used for lists only. |
| Reading spell details in play | built | characters/spelldetails.go; web pages/spells | – | medium | |

## 6. Equipment rules, objects, poisons, appendix

Paths under `backend/internal/` unless `web/`.

| Mechanic | State | Evidence | Deliberate? | Impact | Note |
| --- | --- | --- | --- | --- | --- |
| Armor class from armor, shield, unarmored defense | built | rules/armor.go:11-82 | – | high | |
| Heavy armor Strength requirement (speed -10 ft, dwarves exempt) | built | rules/hitpoints.go:77-79; rules.md:281 | – | low | |
| Armor without proficiency / stealth disadvantage | reminder | rules/armor.go:38-43 | rules.md hints | medium | Issue text and sheet hint only. |
| Donning and doffing armor | absent | no match | – | low | |
| Weapon property: finesse | built | rules/attacks.go:49-54 | – | high | |
| Weapon property: thrown (range) | built | rules/attacks.go:90-93 | – | medium | |
| Weapon property: light (two-weapon fighting) | built | rules/attacks.go:77; rules/combat/bonusattack.go:67 | – | medium | |
| Weapon property: reach | built | combat_actions.go:181-196 | – | medium | |
| Weapon property: heavy (Small wielder) | reminder | rules/attacks.go:56-61 | – | low | Disadvantage hint. |
| Weapon property: versatile | partial | rules/attacks.go:82-87; web combat-column.html:48 | – | medium | Shown as a label; the player does not choose one or two hands. |
| Weapon properties: loading, two-handed, ammunition count | absent | no match in play/ or rules/attacks.go | – | medium | Arrows are never counted or spent. |
| Equipment list: name + quantity, never decremented | partial | characters/sheet.go:225-240 | – | medium | Nothing is consumed in play. |
| Consumables (potions, scrolls, ammunition) used up | absent | rules/srd51/effects/consumables.json used only to halve treasure value (rules/treasure_generate.go:243) | – | high | Drinking a potion has no effect and no count. |
| Magic item bonuses to AC/attack/saves | built | rules/magicitems.go; rules/derive.go | architecture.md:760 | medium | |
| Attunement (3-item limit, short rest to attune, class restriction) | reminder | rules/magicitems.go:45-50 flags only | architecture.md:760 | medium | No character-side state or counter. |
| Charges (use, recharge, destroy at 0) | absent | no match | – | medium | |
| Command word, spells from items, sentient items, paired/duplicate items | absent | no match | – | low | |
| Currency (five coin counts) | partial | characters/sheet.go:242-257; web treasure-format.ts:40 | – | low | No conversion or wallet; gold turns into XP when the campaign counts gold (rules.md:119). |
| Poisons (types, application, sample poisons) | absent | only the poison-needle trap | – | low | |
| Objects: AC, HP, damage immunities, size tiers | absent | no match | – | low | |
| Appendix: planes of existence, pantheons | absent (content only) | no match | – | low | Lore, no mechanic. |
