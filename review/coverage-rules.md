# MeuRPG rules coverage: SRD 5.1 rules chapters (work in progress)

Status: chapters 1 (Using Ability Scores), 2 (Adventuring) and 3 (Combat) are judged. Spellcasting, Conditions and "anything else" follow in later pushes; the summary, the top-20 and the dependencies come last.

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
