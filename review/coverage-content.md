# MeuRPG content coverage: what the app runs, what it only lists, what is missing

Inventory of the SRD 5.1 content in `backend/internal/rules/srd51/data/*.json` against what the code does with it.
Scope: spells, equipment, magic items, monsters, races, backgrounds and feats. Class features and the rules chapters are
mapped elsewhere. **No code was changed.** Base: `origin/main` of `vfraga/meuRPG`.

How to read it:

- **State**: `built` (the app computes what the text asks for), `partial` (it computes something real and misses a named
  part), `reminder` (the content is listed or shown as text, or a resource is spent, and nothing is computed) and
  `absent` (it cannot be used at all, or is not in the data).
- **Evidence** is `file:line` on `origin/main`. Every state was checked in code; docs are cited only in the
  "Deliberate?" column, and a doc is never the evidence for a state.
- **Deliberate?** quotes where a doc says the gap is on purpose. "no doc" means none was found.
- **Table impact** is a judgement of how often a table would hit the gap in a first session: `high`, `medium`, `low`.
- Counts come from the scripts in `review/coverage-content/scripts/` and the overlay tests in
  `review/coverage-content/overlay/` (re-runnable: see the README at the end). Raw lists are in the same folder.

## 0. The engine, in the five facts that decide most rows

These are the mechanisms the rest of the report keeps pointing at. Each was read in code.

1. **A cast computes five things and nothing else.** `CastSpell` (`backend/internal/play/combat_spells.go:192`) spends
   the slot and the action, sets concentration, and then, for each target, runs one of: a spell attack roll with the
   first damage type (`:598`), a saving throw with full/half/no damage (`:636`), Magic Missile's darts (`:670`), a
   pending heal (`:560`+), or one of 12 hit-point effects (`play/combat_spells_hp.go:91`, data in
   `effects/spells.json`). Summons go through a separate choice (`play/creature_cast.go:42`). The proto says it
   plainly: "Anything else (Teia, Passo Nebuloso...): it spends and goes to the log; the effect is the table's"
   (`proto/meurpg/play/v1/combat.proto:885`), and the web shows "A magia foi conjurada: o mestre resolve o efeito."
   (`web/src/app/pages/live-session/combat/cast-sheet/cast-result.ts:63`).
2. **Conditions are labels.** `play/combat_conditions.go:14-18`: "labels the master marks and the app reminds the table
   about; the engine applies no effect." The only code that reads a condition is speed 0 for grappled, restrained,
   paralyzed, petrified, stunned, unconscious (`play/combat_move.go:144-146`) and "cannot react" for
   incapacitated/paralyzed/petrified/stunned/unconscious/blinded (`play/combat_opportunity.go:42-44`). No spell applies a
   condition except Sleep, Color Spray and Power Word Stun (`effects/spells.json`).
3. **There is no advantage or disadvantage in any roll.** An attack, save or check is one d20
   (`RollAttackRequest` has `roll_in_app` or `d20_face` only, `combat.proto:2539`; `play/combat_spells.go:582` `d20`).
   Every `roll_mode` effect, "Desvantagem em Furtividade", Pack Tactics and flanking are text.
   Doc: "Out of scope: flanking, which asks for advantage on attacks (the app does not apply it yet)"
   (`docs/product/rules.md:353`).
4. **Nothing happens at the start or end of a turn, and nothing expires.** `EndTurn` (`play/combat.go:744`) and
   `startTurn` (`play/combat_turn.go:109`) reset the economy and the familiar's sight; there are no durations, no repeat
   saves, no ongoing damage, no regeneration, no recharge dice. Doc: "Applying effects automatically is left for after
   the MVP. That includes the saving throws a spell forces later ... the master calls for the later ones"
   (`docs/product/rules.md:329`).
5. **Content that is not a character's own sheet is a catalogue.** Magic items, adventuring gear, mounts, vehicles,
   monsters' traits and spellcasting are data plus English text. A character's equipment is free text
   (`proto/meurpg/characters/v1/characters.proto:1150` `Item`, 1003 `equipment`), and an NPC made from a monster keeps at most
   three attacks (`backend/internal/characters/npcfromcreature.go:105`).

Not in the list of five but used everywhere: **damage to a player's character waits for the master**, who applies it
(`docs/product/rules.md:56`: "A player character's resistances (Rage and others) are notes the master applies"), while
damage to an NPC lands at once with its plain resistances (`play/combat_actions.go:1126` `afterResistance`).

## 1. Spells (319 of `data/spells.json`)

### 1.1 What a cast does, and where

| Piece | State | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- |
| Slot and action spent, bonus-action-spell rule, cantrip scaling by character level | built | `rules/combat/turn.go:458-485` (`spellOption`), `play/combat_spells.go:392` (spend), `:412` (bonus-action rule), `rules/spelldetails.go:237` `DamageAt` | - | - |
| Spell attack roll against AC, crit on 20, cover, first damage type | built | `play/combat_spells.go:598-634` | - | - |
| Saving throw rolled by the server for every target, full / half / none damage, cover on Dex saves, one damage roll for an area | built | `play/combat_spells.go:636-668` | - | - |
| Damage by slot level (upcasting), damage-type choice (`alternative` / `scale`) | built | `characters/combatspells.go:71`, `rules/spelldetails.go:269` `DamageAtChoosing` | - | - |
| Healing with the casting modifier, up to the maximum, revives from 0 | built | `play/combat_spells.go:574`, `play/combat_actions.go:1169` `healCombatant` | - | - |
| 12 spells that read hit points (Sleep, Color Spray, Power Word Stun / Kill, Heal, Aid, False Life, Spare the Dying, 3 summons) | built | `play/combat_spells_hp.go:91`, `rules/srd51/effects/spells.json` | no doc (a closed, hand-written list) | - |
| Magic Missile darts, Scorching Ray (one ray per target) | built | `play/combat_spells.go:670`, `:42` | ray limit: comment at `:39-42` | low |
| Shield (the only reaction) | built | `play/combat_reactions.go:31`, `UseReaction` `:169` | `proto combat.proto:939`, `architecture.md:821` | - |
| Three summons (Find Familiar as ritual, Animate Dead, Conjure Animals) | built | `play/creature_cast.go:170` `CastSummon`, `rules/summon.go` | - | - |
| Concentration: flag set, replaced by a second cast, a DC reminder (10 or half the damage) after damage, ended by hand | partial | `play/combat_spells.go:418-431`, `play/combat_actions.go:1215`, `rules/combat/rolls.go:117`, `play/combat_conditions.go:95` | `docs/product/rules.md:329`, `:334` ("reminds the concentration check") | high |
| Duration: nothing expires, no round counter, no "ends at the end of its next turn" | absent | no timer in `play/combat_turn.go:109`, `play/combat.go:744` | `docs/product/rules.md:329` | high |
| Conditions a spell imposes | reminder | `play/combat_conditions.go:14-18`; only Sleep / Color Spray / Power Word Stun apply one (`effects/spells.json`) | `docs/product/rules.md:329` | high |
| Area spells: the shape is data, never drawn; the caster (or master) picks up to 10 targets by hand | partial | `rules/spelldetails.go:140` comment, `play/combat_spells.go:47` `maxSpellTargets = 10` | "The area's shape is data, never drawn by combat" (same comment) | medium |
| Casting time 1 minute or more | absent | `rules/combat/turn.go:478` `ReasonTooLong`, `play/combat_spells.go:300` | `docs/architecture.md:823` `CASTING_TIME_TOO_LONG` | medium |
| Reactions other than Shield (Counterspell, Feather Fall, Hellish Rebuke) | absent | `rules/combat/turn.go:482` `ReasonReactionOnly` | `docs/architecture.md:821` | high (Counterspell, Hellish Rebuke) |
| Rituals (29 spells) cast without a slot | absent, except Find Familiar | `play/creature_cast.go:194` takes `ritual`; `CastSpellRequest` has no ritual flag (`combat.proto:911`) | no doc | medium |
| Casting outside a combat | absent | the only spell RPCs are `CastSpell` (needs an encounter) and `CastSummon`; a slot is spent by hand through `AdjustCharacterVitals` (`play.proto:298`) | no doc | high (healing after a fight, Mage Armor before it) |
| An NPC made from a monster casting its spells | absent | `rules/creatures.go:499` ("A creature casts no spells here"); the NPC sheet is basic (`characters/npcfromcreature.go`) | no doc | high (see part 4) |

Two data facts also keep spells from rolling:

- **A damage table without a save or an attack rolls nothing at cast.** 10 spells have one and so show dice to the
  master only: Branding Smite, Dimension Door (fall damage), Divine Favor, Earthquake, Fire Shield, Flaming Sphere,
  Glyph of Warding, Spike Growth, Teleport (mishap), Web. Source: `effects/corrections.json` header, and the machine
  dump `spells-machine.json` (`damage_parsed` with no `attack_type` / `save_ability`).
- **34 spells say "saving throw" in their text and have no `save_ability` in the data**, so no save is rolled at cast.
  They are listed in `spells-cast-sample.txt` and in the table below (Web, Flaming Sphere, Earthquake, Sleet Storm,
  Haste, Bless, Zone of Truth, Forcecage...). Only five are corrected by `effects/corrections.json` (Spirit Guardians,
  Call Lightning, Flame Strike, Disintegrate, Phantasmal Killer).

### 1.2 Counts

| State | Spells | What it means here |
| --- | --- | --- |
| built | 35 | the cast computes what the text asks for in a fight |
| partial | 82 | the cast rolls an attack, a save, damage, a heal or a hit point effect, and a named part is left to the table |
| reminder | 142 | the cast spends the slot and the action, sets concentration if any, and logs; nothing is computed |
| absent | 60 | the app refuses to cast it: casting time of a minute or more (57) or a reaction other than Shield (3) |
| **total** | **319** | |

| spell level | built | partial | reminder | absent |
| --- | --- | --- | --- | --- |
| 0 | 6 | 5 | 12 | 1 |
| 1 | 10 | 10 | 24 | 5 |
| 2 | 3 | 13 | 34 | 4 |
| 3 | 5 | 9 | 22 | 6 |
| 4 | 1 | 13 | 13 | 4 |
| 5 | 3 | 8 | 11 | 15 |
| 6 | 4 | 9 | 7 | 11 |
| 7 | 1 | 7 | 5 | 7 |
| 8 | 0 | 5 | 8 | 3 |
| 9 | 2 | 3 | 6 | 4 |

Table impact of the 284 spells that are not built: high 82, medium 78, low 124.

Sample checked by running the content side of a cast in a Go test overlay (the same calls as
`characters.CombatSpell`, `characters/combatspells.go:30`): `spells-cast-sample.txt` (Fireball at 3rd and 5th level gives
8d6 and 10d6 Dex-half, Cure Wounds gives `1d8 + MOD` and `3d8 + MOD`, Hold Person gives a Wisdom save with no damage and no
effect, Web gives no save and a 2d4 table, Sleep gives the hit point pool, Hellish Rebuke is a reaction so it is refused).
The full machine view of all 319 spells is `spells-machine.json`
(`overlay/zz_spells_dump_test.go`). The play tests for the cast (`play/combat_spells_test.go`, `combat_spells_hp_test.go`)
need Postgres, so the engine side was verified by reading, not by running a cast.

### 1.3 Grouped by what is missing


- **duration_not_tracked**: 136 - alter-self, animal-friendship, animal-shapes, antilife-shell, antimagic-field, arcane-hand, bane, banishment, barkskin, beacon-of-hope, bestow-curse, black-tentacles, blade-barrier, bless, blindness-deafness, blur, call-lightning, calm-emotions, charm-person, chill-touch, cloudkill, command, compulsion, confusion, contagion, control-water, dancing-lights, darkness, darkvision, daylight, death-ward, delayed-blast-fireball, demiplane, dispel-evil-and-good, divine-favor, dominate-beast, dominate-monster, dominate-person, earthquake, enhance-ability, enlarge-reduce, entangle, enthrall, expeditious-retreat, eyebite, faerie-fire, fear, fire-shield, flaming-sphere, flesh-to-stone, fly, fog-cloud, forcecage, freedom-of-movement, gaseous-form, gate, glibness, globe-of-invulnerability, grease, greater-invisibility, guardian-of-faith, guidance, guiding-bolt, gust-of-wind, haste, heat-metal, heroism, hideous-laughter, hold-monster, hold-person, holy-aura, hunters-mark, hypnotic-pattern, incendiary-cloud, insect-plague, invisibility, irresistible-dance, jump, levitate, light, longstrider, mage-armor, magic-weapon, mass-suggestion, maze, mind-blank, mirror-image, mislead, modify-memory, moonbeam, move-earth, pass-without-trace, passwall, phantasmal-killer, polymorph, prismatic-wall, produce-flame, protection-from-energy, protection-from-evil-and-good, protection-from-poison, ray-of-enfeeblement, resilient-sphere, resistance, reverse-gravity, sanctuary, shapechange, shield-of-faith, shillelagh, silence, sleet-storm, slow, speak-with-plants, spider-climb, spike-growth, spirit-guardians, spiritual-weapon, stinking-cloud, stoneskin, storm-of-vengeance, suggestion, sunbeam, telekinesis, transport-via-plants, true-polymorph, true-seeing, true-strike, wall-of-fire, wall-of-force, wall-of-ice, wall-of-stone, wall-of-thorns, warding-bond, web, weird, wind-wall, zone-of-truth
- **movement**: 68 - alter-self, antilife-shell, arcane-hand, banishment, black-tentacles, blade-barrier, blink, command, compulsion, control-water, dancing-lights, delayed-blast-fireball, dimension-door, earthquake, entangle, etherealness, expeditious-retreat, eyebite, fear, flaming-sphere, floating-disk, fly, forcecage, freedom-of-movement, gaseous-form, gate, grease, gust-of-wind, haste, ice-storm, incendiary-cloud, insect-plague, jump, levitate, longstrider, mage-hand, maze, meld-into-stone, mislead, misty-step, passwall, plane-shift, plant-growth, prismatic-spray, project-image, ray-of-frost, resilient-sphere, reverse-gravity, sleet-storm, slow, speak-with-plants, spider-climb, spike-growth, spirit-guardians, telekinesis, teleport, thunderwave, transport-via-plants, tree-stride, unseen-servant, wall-of-force, wall-of-ice, wall-of-stone, wall-of-thorns, water-walk, web, wind-wall, word-of-recall
- **zone**: 57 - antilife-shell, antimagic-field, arcane-lock, black-tentacles, blade-barrier, call-lightning, cloudkill, continual-flame, control-water, dancing-lights, darkness, daylight, delayed-blast-fireball, demiplane, earthquake, entangle, faerie-fire, fire-shield, flaming-sphere, fog-cloud, forcecage, freezing-sphere, gate, globe-of-invulnerability, grease, guardian-of-faith, gust-of-wind, ice-storm, incendiary-cloud, insect-plague, light, moonbeam, move-earth, passwall, plant-growth, prismatic-wall, produce-flame, resilient-sphere, reverse-gravity, silence, sleet-storm, speak-with-plants, spike-growth, spirit-guardians, spiritual-weapon, stinking-cloud, storm-of-vengeance, sunbeam, transport-via-plants, wall-of-fire, wall-of-force, wall-of-ice, wall-of-stone, wall-of-thorns, web, wind-wall, zone-of-truth
- **stat_change**: 53 - alter-self, animal-shapes, bane, barkskin, beacon-of-hope, bestow-curse, bless, blur, charm-person, chill-touch, contagion, dispel-evil-and-good, divine-favor, enhance-ability, enlarge-reduce, enthrall, eyebite, faerie-fire, feeblemind, fire-shield, gaseous-form, glibness, guidance, guiding-bolt, harm, haste, heat-metal, holy-aura, irresistible-dance, mage-armor, magic-weapon, mind-blank, mirror-image, pass-without-trace, polymorph, protection-from-energy, protection-from-evil-and-good, protection-from-poison, ray-of-enfeeblement, resistance, shapechange, shield-of-faith, shillelagh, shocking-grasp, slow, stoneskin, sunbeam, sunburst, true-polymorph, true-strike, vicious-mockery, warding-bond, wish
- **condition**: 38 - animal-friendship, arcane-hand, banishment, black-tentacles, blindness-deafness, charm-person, command, contagion, divine-word, dominate-beast, dominate-monster, dominate-person, earthquake, entangle, eyebite, fear, flesh-to-stone, grease, greater-invisibility, hideous-laughter, hold-monster, hold-person, holy-aura, hypnotic-pattern, invisibility, mislead, modify-memory, phantasmal-killer, prismatic-spray, prismatic-wall, silence, sleet-storm, storm-of-vengeance, sunbeam, sunburst, telekinesis, web, weird
- **repeat_save**: 33 - bestow-curse, black-tentacles, blindness-deafness, call-lightning, compulsion, confusion, contagion, dominate-beast, dominate-monster, dominate-person, earthquake, eyebite, fear, feeblemind, flesh-to-stone, grease, hideous-laughter, hold-monster, hold-person, irresistible-dance, maze, phantasmal-killer, power-word-stun, prismatic-spray, prismatic-wall, ray-of-enfeeblement, slow, spirit-guardians, stinking-cloud, sunburst, web, weird, zone-of-truth
- **creation**: 26 - animate-objects, arcane-eye, arcane-hand, arcane-sword, conjure-woodland-beings, create-food-and-water, faithful-hound, finger-of-death, flame-blade, floating-disk, giant-insect, goodberry, mage-hand, major-image, minor-illusion, mirror-image, mislead, prestidigitation, programmed-illusion, project-image, rope-trick, secret-chest, silent-image, true-polymorph, unseen-servant, wish
- **ongoing_damage**: 22 - acid-arrow, arcane-hand, black-tentacles, blade-barrier, cloudkill, control-water, faithful-hound, flaming-sphere, guardian-of-faith, heat-metal, incendiary-cloud, insect-plague, moonbeam, phantasmal-killer, spike-growth, spirit-guardians, storm-of-vengeance, wall-of-fire, wall-of-ice, wall-of-thorns, web, weird
- **rider**: 22 - bestow-curse, branding-smite, chill-touch, divine-word, fear, feeblemind, finger-of-death, fire-shield, guiding-bolt, heat-metal, holy-aura, plane-shift, prismatic-spray, prismatic-wall, shocking-grasp, sleet-storm, storm-of-vengeance, sunbeam, sunburst, teleport, warding-bond, wish
- **upcast_extra**: 18 - bestow-curse, conjure-woodland-beings, create-or-destroy-water, dispel-magic, dominate-beast, dominate-monster, dominate-person, fog-cloud, globe-of-invulnerability, hold-person, hunters-mark, invisibility, longstrider, magic-weapon, major-image, modify-memory, moonbeam, phantasmal-killer
- **damage_not_rolled**: 9 - branding-smite, dimension-door, divine-favor, earthquake, fire-shield, flaming-sphere, spike-growth, teleport, web
- **effect_not_applied**: 3 - calm-emotions, mass-suggestion, suggestion
- **pool_not_shared_out**: 1 - mass-heal
- **heal_half_not_applied**: 1 - vampiric-touch
- **repeat_attack_not_run**: 1 - vampiric-touch

## conditions a spell should impose and the app does not apply

- blinded: 9 - blindness-deafness, contagion, divine-word, holy-aura, mislead, prismatic-spray, prismatic-wall, sunbeam, sunburst
- charmed: 7 - animal-friendship, charm-person, dominate-beast, dominate-monster, dominate-person, hypnotic-pattern, modify-memory
- restrained: 7 - black-tentacles, entangle, flesh-to-stone, prismatic-spray, prismatic-wall, telekinesis, web
- deafened: 5 - blindness-deafness, divine-word, mislead, silence, storm-of-vengeance
- prone: 5 - command, earthquake, grease, hideous-laughter, sleet-storm
- incapacitated: 4 - banishment, hideous-laughter, hypnotic-pattern, modify-memory
- frightened: 4 - eyebite, fear, phantasmal-killer, weird
- stunned: 3 - contagion, divine-word, power-word-stun
- petrified: 3 - flesh-to-stone, prismatic-spray, prismatic-wall
- invisible: 3 - greater-invisibility, invisibility, mislead
- paralyzed: 2 - hold-monster, hold-person
- grappled: 1 - arcane-hand
- unconscious: 1 - eyebite

## concentration spells castable in combat: 118 (flag set and DC reminder only; no auto end on duration, incapacitation or failed save)


## rituals: 29 (alarm, animal-messenger, augury, commune, commune-with-nature, comprehend-languages, contact-other-plane, detect-magic, detect-poison-and-disease, divination, find-familiar, floating-disk, forbiddance, gentle-repose, identify, illusory-script, instant-summons, locate-animals-or-plants, magic-mouth, meld-into-stone, phantom-steed, purify-food-and-drink, silence, speak-with-animals, telepathic-bond, tiny-hut, unseen-servant, water-breathing, water-walk); only find-familiar can be cast as a ritual (CastSummon); 12 of them are absent


### 1.4 Every spell

| Spell | L | State | Impact | Engine does | Missing / why | Evidence | Deliberate? |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Flecha Ácida (`acid-arrow`) | 2 | partial | high | attack_roll, damage, upcast_dice | ongoing_damage; Ranged spell attack; 4d4 acid now, 2d4 more at end of next turn. | `combat_spells.go:598` | rules.md:329 |
| Espirro Ácido (`acid-splash`) | 0 | built | low | save_roll, damage | Dex save for 1d6 acid to one or two nearby creatures. | `combat_spells.go:636` | - |
| Ajuda (`aid`) | 2 | built | low | hp_effect:max_hp, upcast_dice | duration_not_tracked; max_hp effect raises maximum and current HP for up to three targets (effects/spells.json) | `combat_spells_hp.go:91` | - |
| Alarme (`alarm`) | 1 | absent | high | - | zone, duration_not_tracked; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Alterar-se (`alter-self`) | 2 | reminder | medium | conc | movement, stat_change, duration_not_tracked; Choose aquatic, appearance, or natural weapon form for up to 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Amizade Animal (`animal-friendship`) | 1 | partial | high | save_roll | condition:charmed, duration_not_tracked; Beast with Int 3 or less must fail Wis save or be charmed for 24 hours. | `combat_spells.go:636` | rules.md:329 |
| Mensageiro Animal (`animal-messenger`) | 2 | reminder | low | - | Tiny beast carries a 25-word message up to 50 miles in 24 hours. | `cast-result.ts:63 (plain)` | no doc |
| Formas Animais (`animal-shapes`) | 8 | reminder | low | conc | stat_change, duration_not_tracked; Transform willing creatures into beasts of CR 4 or lower for up to 24 hours. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Animar Mortos (`animate-dead`) | 3 | built | low | summon, area: targets picked by hand | Raises a skeleton or zombie you command for 24 hours, renewable. | `creature_cast.go:170` | - |
| Animar Objetos (`animate-objects`) | 5 | reminder | high | conc | creation; Up to ten objects become creatures you command for one minute. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Cúpula Antivida (`antilife-shell`) | 5 | reminder | medium | conc, area: targets picked by hand | movement, zone, duration_not_tracked; 10-foot barrier around you that blocks non-undead, non-construct creatures for up to 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Campo Antimagia (`antimagic-field`) | 8 | reminder | low | conc, area: targets picked by hand | zone, duration_not_tracked; 10-foot invisible sphere around you suppressing magic for up to 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Antipatia/Simpatia (`antipathy-sympathy`) | 8 | absent | low | - | repeat_save, movement, zone, condition:frightened, duration_not_tracked; casting time of 1 hour: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Olho Arcano (`arcane-eye`) | 4 | reminder | low | conc | creation; Invisible sensor eye you move 30 feet per action for up to 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Mão Arcana (`arcane-hand`) | 5 | partial | medium | attack_roll, damage, upcast_dice, conc | ongoing_damage, movement, condition:grappled, creation, duration_not_tracked; Force hand strikes, pushes, grapples, or shields for up to 1 minute. | `combat_spells.go:418`; `combat_spells.go:598` | rules.md:329 |
| Tranca Arcana (`arcane-lock`) | 2 | reminder | medium | - | zone; Locks an object until dispelled; you and chosen creatures can open it. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Espada Arcana (`arcane-sword`) | 7 | partial | low | attack_roll, damage, conc | creation; Sword of force: melee spell attack 3d10, repeatable by bonus action for 1 minute. | `combat_spells.go:418`; `combat_spells.go:598` | rules.md:329 |
| Aura Mágica do Arcanista (`arcanists-magic-aura`) | 2 | reminder | low | - | Alters divination results on a creature or object for 24 hours. | `cast-result.ts:63 (plain)` | no doc |
| Projeção Astral (`astral-projection`) | 9 | absent | low | - | casting time of 1 hour: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Augúrio (`augury`) | 2 | absent | medium | - | casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Despertar (`awaken`) | 5 | absent | low | - | stat_change, condition:charmed; casting time of 8 hours: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Perdição (`bane`) | 1 | partial | high | save_roll, conc | stat_change, duration_not_tracked; Three targets subtract 1d4 from attacks and saves for 1 minute. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Banimento (`banishment`) | 4 | partial | high | save_roll, conc | movement, condition:incapacitated, duration_not_tracked; Cha save or banish a creature to another plane until concentration ends. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Pele de Árvore (`barkskin`) | 2 | reminder | high | conc | stat_change, duration_not_tracked; Touched creature's AC cannot be below 16 for up to 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Sinal de Esperança (`beacon-of-hope`) | 3 | reminder | medium | conc | stat_change, duration_not_tracked; Chosen creatures gain advantage on Wis and death saves, max healing for 1 minute. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Rogar Maldição (`bestow-curse`) | 3 | partial | high | save_roll, conc | repeat_save, stat_change, rider, upcast_extra, duration_not_tracked; Touch curse with one chosen effect, concentration up to 1 minute. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Tentáculos Negros (`black-tentacles`) | 4 | partial | medium | save_roll, damage, conc, area: targets picked by hand | repeat_save, ongoing_damage, movement, zone, condition:restrained, duration_not_tracked; Difficult terrain 20-foot square; Dex save or restrained and 3d6 bludgeoning each turn. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Barreira de Lâminas (`blade-barrier`) | 6 | partial | low | save_roll, damage, conc, area: targets picked by hand | ongoing_damage, movement, zone, duration_not_tracked; Blade wall: 6d10 slashing on entry or turn start, difficult terrain. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Bênção (`bless`) | 1 | reminder | high | conc | stat_change, duration_not_tracked; Up to three allies add 1d4 to attacks and saves for 1 minute. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Malogro (`blight`) | 4 | built | low | save_roll, damage, upcast_dice | Con save: 8d8 necrotic damage, half on success; plants take max. | `combat_spells.go:636` | - |
| Cegueira/Surdez (`blindness-deafness`) | 2 | partial | high | save_roll | repeat_save, condition:blinded+deafened, duration_not_tracked; Con save blinds or deafens foe for 1 minute, repeatable each turn. | `combat_spells.go:636` | rules.md:329 |
| Piscar (`blink`) | 3 | reminder | medium | - | movement; Each turn you roll d20; on 11+ you vanish to Ethereal Plane, returning later. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Nublar (`blur`) | 2 | reminder | high | conc | stat_change, duration_not_tracked; Attackers have disadvantage on attacks against you for 1 minute. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Marca da Punição (`branding-smite`) | 2 | reminder | medium | damage_table_only, upcast_dice, conc | rider, damage_not_rolled; Next weapon hit adds 2d6 radiant and marks the target for 1 minute. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Mãos Flamejantes (`burning-hands`) | 1 | built | low | save_roll, damage, upcast_dice, area: targets picked by hand | 15-foot cone; Dex save for 3d6 fire damage, half on success. | `combat_spells.go:636` | - |
| Convocar Relâmpagos (`call-lightning`) | 3 | partial | high | save_roll, damage, upcast_dice, conc, area: targets picked by hand | repeat_save, zone, duration_not_tracked; Lightning strikes a point each turn for 10 minutes; Dex save for 3d10. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Acalmar Emoções (`calm-emotions`) | 2 | partial | high | save_roll, conc, area: targets picked by hand | effect_not_applied, duration_not_tracked; the save is rolled; suppressing charm / fear (or making creatures indifferent) is not applied | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Corrente de Relâmpagos (`chain-lightning`) | 6 | built | low | save_roll, damage | Bolt jumps to three targets within 30 feet; Dex save for 10d8 lightning. | `combat_spells.go:636` | - |
| Enfeitiçar Pessoa (`charm-person`) | 1 | partial | high | save_roll | stat_change, condition:charmed, duration_not_tracked; Humanoid makes Wis save or is charmed for 1 hour; ends if harmed. | `combat_spells.go:636` | rules.md:329 |
| Toque Arrepiante (`chill-touch`) | 0 | partial | high | attack_roll, damage | stat_change, rider, duration_not_tracked; Ranged spell attack for 1d8 necrotic; blocks healing; undead have disadvantage. | `combat_spells.go:598` | rules.md:329 |
| Círculo da Morte (`circle-of-death`) | 6 | built | low | save_roll, damage, upcast_dice, area: targets picked by hand | Constitution save in 60-foot sphere: 8d6 necrotic damage, half on success. | `combat_spells.go:636` | - |
| Clarividência (`clairvoyance`) | 3 | absent | medium | - | zone, duration_not_tracked; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Clone (`clone`) | 8 | absent | low | - | creation; casting time of 1 hour: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Névoa Mortal (`cloudkill`) | 5 | partial | high | save_roll, damage, upcast_dice, conc, area: targets picked by hand | ongoing_damage, zone, duration_not_tracked; Poison fog: Constitution save, 5d8 poison on entry or turn start; concentration. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Leque Cromático (`color-spray`) | 1 | built | low | hp_effect:hp_pool, area: targets picked by hand | duration_not_tracked; hp_pool effect: blinded applied; end of the condition is the table's | `combat_spells_hp.go:91` | - |
| Comando (`command`) | 1 | partial | high | save_roll | movement, condition:prone, duration_not_tracked; Wisdom save; target obeys one-word command next turn, such as approach or flee. | `combat_spells.go:636` | rules.md:329 |
| Comunhão (`commune`) | 5 | absent | medium | - | casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Comunhão com a Natureza (`commune-with-nature`) | 5 | absent | medium | - | casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Compreender Idiomas (`comprehend-languages`) | 1 | reminder | medium | - | Understand spoken and touched written languages for 1 hour; no combat use. | `cast-result.ts:63 (plain)` | no doc |
| Compulsão (`compulsion`) | 4 | partial | medium | save_roll, conc, area: targets picked by hand | repeat_save, movement, duration_not_tracked; Wisdom save; each turn you direct affected targets to move; repeat saves end it. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Cone de Frio (`cone-of-cold`) | 5 | built | low | save_roll, damage, upcast_dice, area: targets picked by hand | the frozen-statue clause is flavour | `combat_spells.go:636` | - |
| Confusão (`confusion`) | 4 | partial | high | save_roll, conc, area: targets picked by hand | repeat_save, duration_not_tracked; Wisdom save in 10-foot sphere; d10 behavior table each turn; repeat saves. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Conjurar Animais (`conjure-animals`) | 3 | built | low | summon, conc | CastSummon / summon choice with the SRD count by slot (rules/summon.go) | `combat_spells.go:418`; `creature_cast.go:170` | - |
| Conjurar Celestial (`conjure-celestial`) | 7 | absent | low | - | creation, upcast_extra; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Conjurar Elemental (`conjure-elemental`) | 5 | absent | high | - | creation, upcast_extra; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Conjurar Fada (`conjure-fey`) | 6 | absent | high | - | creation, upcast_extra; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Conjurar Elementais Menores (`conjure-minor-elementals`) | 4 | absent | high | - | creation, upcast_extra; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Conjurar Seres da Floresta (`conjure-woodland-beings`) | 4 | reminder | high | conc | creation, upcast_extra; Summons fey creatures by challenge rating option; concentration up to 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Contato Extraplanar (`contact-other-plane`) | 5 | absent | low | - | rider; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Praga (`contagion`) | 5 | partial | medium | attack_roll, save_roll | repeat_save, stat_change, condition:blinded+stunned, duration_not_tracked; Melee spell attack inflicts disease with repeat Con saves; some diseases blind or stun. | `combat_spells.go:598`; `combat_spells.go:636` | rules.md:329 |
| Contingência (`contingency`) | 6 | absent | low | - | casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Chama Contínua (`continual-flame`) | 2 | reminder | low | - | zone; Permanent torch-bright magical flame on an object; creates no heat. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Controlar a Água (`control-water`) | 4 | partial | medium | save_roll, damage, conc, area: targets picked by hand | ongoing_damage, movement, zone, duration_not_tracked; Choose flood, part, redirect or whirlpool water effects; up to 10 minutes. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Controlar o Clima (`control-weather`) | 8 | absent | low | - | zone, duration_not_tracked; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Contramágica (`counterspell`) | 3 | absent | high | - | upcast_extra; reaction spell: the cast RPC refuses it (CASTING reason REACTION_ONLY) | `turn.go:482` | architecture.md:821 |
| Criar Alimentos (`create-food-and-water`) | 3 | reminder | low | - | creation; Creates food and water for fifteen humanoids for 24 hours. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Criar ou Destruir Água (`create-or-destroy-water`) | 1 | reminder | low | area: targets picked by hand | upcast_extra; Create or destroy up to 10 gallons of water or fog in a 30-foot cube. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Criar Mortos-Vivos (`create-undead`) | 6 | absent | low | - | creation, upcast_extra; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Criação (`creation`) | 5 | absent | low | - | creation, upcast_extra; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Curar Ferimentos (`cure-wounds`) | 1 | built | low | heal, upcast_dice | Touch heals 1d8 plus spellcasting modifier; no effect on undead or constructs. | `combat_spells.go:574` | - |
| Globos de Luz (`dancing-lights`) | 0 | reminder | low | conc | movement, zone, duration_not_tracked; Four floating lights shed dim light; bonus action moves them 60 feet. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Escuridão (`darkness`) | 2 | reminder | high | conc, area: targets picked by hand | zone, duration_not_tracked; 15-foot radius magical darkness up to 10 minutes; dispels overlapping light spells. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Visão no Escuro (`darkvision`) | 2 | reminder | medium | - | duration_not_tracked; Touch grants willing creature 60-foot darkvision for 8 hours. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Luz do Dia (`daylight`) | 3 | reminder | medium | area: targets picked by hand | zone, duration_not_tracked; 60-foot bright light sphere for 1 hour; dispels darkness of 3rd level or lower. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Proteção contra a Morte (`death-ward`) | 4 | reminder | high | - | duration_not_tracked; Touch prevents first drop to 0 HP for 8 hours; negates instant-death effects. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Bola de Fogo Controlável (`delayed-blast-fireball`) | 7 | partial | low | save_roll, damage, upcast_dice, conc, area: targets picked by hand | movement, zone, duration_not_tracked; Lingering bead explodes for 12d6 fire save damage, growing each turn. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Semiplano (`demiplane`) | 8 | reminder | low | - | zone, duration_not_tracked; Creates shadowy door to a 30-foot empty room demiplane for 1 hour. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Detectar o Bem e Mal (`detect-evil-and-good`) | 1 | reminder | low | conc | Detects certain creature types and consecrated places within 30 feet. | `combat_spells.go:418`; `cast-result.ts:63` | no doc |
| Detectar Magia (`detect-magic`) | 1 | reminder | medium | conc | Senses magic within 30 feet for 10 minutes; reveals magic school with action. | `combat_spells.go:418`; `cast-result.ts:63` | no doc |
| Detectar Veneno e Doença (`detect-poison-and-disease`) | 1 | reminder | low | conc | Senses and identifies poisons, poisonous creatures and diseases within 30 feet for 10 minutes. | `combat_spells.go:418`; `cast-result.ts:63` | no doc |
| Detectar Pensamentos (`detect-thoughts`) | 2 | reminder | low | conc | Reads thoughts; deeper probe needs Wisdom save; target can end it. | `combat_spells.go:418`; `cast-result.ts:63` | no doc |
| Porta Dimensional (`dimension-door`) | 4 | reminder | low | damage_table_only | movement, damage_not_rolled; Self-teleport plus one willing ally; collision deals 4d6 force. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Disfarçar-se (`disguise-self`) | 1 | reminder | medium | - | Changes appearance for 1 hour; Investigation check against save DC reveals. | `cast-result.ts:63 (plain)` | no doc |
| Desintegrar (`disintegrate`) | 6 | built | low | save_roll, damage, upcast_dice, area: targets picked by hand | 10d6+40 force with Dex save; the dust clause is flavour | `combat_spells.go:636` | - |
| Dissipar o Bem e Mal (`dispel-evil-and-good`) | 5 | partial | medium | save_roll, conc, area: targets picked by hand | stat_change, duration_not_tracked; Enemies of listed types get disadvantage vs you; Dismissal can send them home. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Dissipar Magia (`dispel-magic`) | 3 | reminder | high | - | upcast_extra; Ends spell of 3rd level or lower automatically; higher ones need ability check. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Adivinhação (`divination`) | 4 | reminder | low | - | Ask GM one question about events within seven days; reliability varies on recasts. | `cast-result.ts:63 (plain)` | no doc |
| Auxílio Divino (`divine-favor`) | 1 | reminder | medium | damage_table_only, conc | stat_change, damage_not_rolled, duration_not_tracked; Your weapon attacks deal extra 1d4 radiant damage for up to a minute. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Palavra Divina (`divine-word`) | 7 | partial | low | save_roll | rider, condition:blinded+deafened+stunned; Charisma save; HP-based deafen, blind, stun, or instant kill; fey and fiends banished. | `combat_spells.go:636` | rules.md:329 |
| Dominar Besta (`dominate-beast`) | 4 | partial | medium | save_roll, conc | repeat_save, condition:charmed, upcast_extra, duration_not_tracked; Wisdom save charms beast; commands it; repeats after each damage instance. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Dominar Monstro (`dominate-monster`) | 8 | partial | low | save_roll, conc | repeat_save, condition:charmed, upcast_extra, duration_not_tracked; Wisdom save charms any creature; commands it; repeats after each damage instance. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Dominar Pessoa (`dominate-person`) | 5 | partial | high | save_roll, conc | repeat_save, condition:charmed, upcast_extra, duration_not_tracked; Wisdom save charms humanoid; commands it; repeats after each damage instance. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Sonho (`dream`) | 5 | absent | low | - | rider; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Druidismo (`druidcraft`) | 0 | reminder | low | - | Minor harmless effects: weather prediction, bloom flower, light or snuff candle. | `cast-result.ts:63 (plain)` | no doc |
| Terremoto (`earthquake`) | 8 | reminder | low | damage_table_only, conc, area: targets picked by hand | repeat_save, movement, zone, condition:prone, damage_not_rolled, duration_not_tracked; Area becomes difficult terrain; Con save breaks concentration; Dex save knocks prone each turn. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Rajada Mística (`eldritch-blast`) | 0 | built | low | attack_roll, damage | beams are the cantrip's attack lines (rules/attacks.go beamsAt) | `combat_spells.go:598` | - |
| Aprimorar Habilidade (`enhance-ability`) | 2 | reminder | medium | conc | stat_change, duration_not_tracked; Touched ally gains advantage on one ability's checks; Bear's Endurance adds temp HP. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Aumentar/Reduzir (`enlarge-reduce`) | 2 | partial | high | save_roll, conc | stat_change, duration_not_tracked; Enlarge or reduce a creature or object; attack damage and Strength checks change. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Constrição (`entangle`) | 1 | partial | high | save_roll, conc, area: targets picked by hand | movement, zone, condition:restrained, duration_not_tracked; Str save restrains creatures in 20-foot square; area becomes difficult terrain. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Cativar (`enthrall`) | 2 | partial | high | save_roll, area: targets picked by hand | stat_change, duration_not_tracked; Wisdom save; failure gives disadvantage on Perception of others. | `combat_spells.go:636` | rules.md:329 |
| Forma Etérea (`etherealness`) | 7 | reminder | low | - | movement; Caster enters Ethereal Plane for 8 hours; returning causes force damage if blocked. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Recuo Acelerado (`expeditious-retreat`) | 1 | reminder | medium | conc | movement, duration_not_tracked; Dash as bonus action each turn for up to ten minutes. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Ataque Visual (`eyebite`) | 6 | partial | low | save_roll, conc, area: targets picked by hand | repeat_save, movement, stat_change, condition:unconscious+frightened, duration_not_tracked; Wisdom save; sleep, panic, or sicken one creature, retargetable each turn. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Fabricar (`fabricate`) | 4 | absent | low | - | creation; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Fogo das Fadas (`faerie-fire`) | 1 | partial | high | save_roll, conc, area: targets picked by hand | stat_change, zone, duration_not_tracked; Dex save outlines creatures; attacks against them have advantage, no invisibility. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Cão Fiel (`faithful-hound`) | 4 | partial | medium | attack_roll, damage | ongoing_damage, creation; Phantom hound bites a hostile creature each turn for 4d8 piercing. | `combat_spells.go:598` | rules.md:329 |
| Vitalidade Falsa (`false-life`) | 1 | built | low | hp_effect:temp_hp, upcast_dice | duration_not_tracked; Gain 1d4+4 temporary HP for one hour; more with higher slots. | `combat_spells_hp.go:91` | - |
| Medo (`fear`) | 3 | partial | high | save_roll, conc, area: targets picked by hand | repeat_save, movement, rider, condition:frightened, duration_not_tracked; Wisdom save or frightened in 30-foot cone; flee; repeat save out of sight. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Queda Suave (`feather-fall`) | 1 | absent | medium | - | movement, duration_not_tracked; reaction spell: the cast RPC refuses it (CASTING reason REACTION_ONLY) | `turn.go:482` | architecture.md:821 |
| Enfraquecer Intelecto (`feeblemind`) | 8 | partial | low | save_roll, damage | repeat_save, stat_change, rider; Int save: 4d6 psychic; on fail Int and Cha become 1 until restored. | `combat_spells.go:636` | rules.md:329 |
| Convocar Familiar (`find-familiar`) | 1 | built | low | summon | Summons a familiar with chosen animal form; it can deliver touch spells. | `creature_cast.go:170` | - |
| Convocar Montaria (`find-steed`) | 2 | absent | medium | - | creation; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Encontrar o Caminho (`find-the-path`) | 6 | absent | low | - | casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Encontrar Armadilhas (`find-traps`) | 2 | reminder | low | - | Reveals presence and general nature of traps in line of sight. | `cast-result.ts:63 (plain)` | no doc |
| Dedo da Morte (`finger-of-death`) | 7 | partial | low | save_roll, damage | rider, creation; Con save for 7d8+30 necrotic; slain humanoid rises as your zombie. | `combat_spells.go:636` | rules.md:329 |
| Raio de Fogo (`fire-bolt`) | 0 | built | low | attack_roll, damage | Ranged spell attack for 1d10 fire, scaling damage dice with character level. | `combat_spells.go:598` | - |
| Escudo de Fogo (`fire-shield`) | 4 | reminder | medium | damage_table_only, area: targets picked by hand | stat_change, zone, rider, damage_not_rolled, duration_not_tracked; Choose warm or cold shield: resistance, and melee attackers take 2d8 damage. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Tempestade de Fogo (`fire-storm`) | 7 | built | low | save_roll, damage, area: targets picked by hand | Dex save; 7d10 fire in up to ten 10-foot cubes, half on success. | `combat_spells.go:636` | - |
| Bola de Fogo (`fireball`) | 3 | built | low | save_roll, damage, upcast_dice, area: targets picked by hand | Dex save, 8d6 fire in 20-foot radius; half on success. | `combat_spells.go:636` | - |
| Lâmina Flamejante (`flame-blade`) | 2 | partial | high | damage, upcast_dice, conc | creation; Bonus-action blade; melee spell attack for 3d6 fire; sheds light. | `combat_spells.go:418` | rules.md:329 |
| Coluna de Chamas (`flame-strike`) | 5 | built | low | save_roll, damage, damage_type_choice, upcast_dice, area: targets picked by hand | Dex save for 4d6 fire plus 4d6 radiant damage in a column; half on success. | `combat_spells.go:636` | - |
| Esfera Flamejante (`flaming-sphere`) | 2 | reminder | high | damage_table_only, upcast_dice, conc, area: targets picked by hand | ongoing_damage, movement, zone, damage_not_rolled, duration_not_tracked; Persistent fire sphere: Dex save each turn adjacent; bonus action moves it 30 feet. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Carne para Pedra (`flesh-to-stone`) | 6 | partial | low | save_roll, conc | repeat_save, condition:restrained+petrified, duration_not_tracked; Con save restrains target; three failures petrify it; repeat saves each turn. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Disco Flutuante (`floating-disk`) | 1 | reminder | low | - | movement, creation; Creates a 3-foot force disk holding 500 pounds that follows caster for an hour. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Voo (`fly`) | 3 | reminder | high | conc | movement, duration_not_tracked; Touch grants willing creature 60-foot flying speed for up to 10 minutes. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Névoa Obscurecente (`fog-cloud`) | 1 | reminder | high | conc, area: targets picked by hand | zone, upcast_extra, duration_not_tracked; Heavily obscuring fog sphere up to an hour; higher slots widen radius. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Proibição (`forbiddance`) | 6 | absent | low | - | ongoing_damage, movement, zone, duration_not_tracked; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Prisão de Energia (`forcecage`) | 7 | reminder | low | area: targets picked by hand | movement, zone, duration_not_tracked; Immobile force cage or box traps creatures inside for an hour; Cha save escapes teleport. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Sexto Sentido (`foresight`) | 9 | absent | low | - | stat_change, duration_not_tracked; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Movimentação Livre (`freedom-of-movement`) | 4 | reminder | medium | - | movement, duration_not_tracked; Touch ignores difficult terrain, magical slow and paralysis or restraint for an hour. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Esfera Congelante (`freezing-sphere`) | 6 | partial | low | save_roll, damage, upcast_dice, area: targets picked by hand | zone; Con save for 10d6 cold in 60-foot sphere; half on success; may freeze water. | `combat_spells.go:636` | rules.md:329 |
| Forma Gasosa (`gaseous-form`) | 3 | reminder | medium | conc | movement, stat_change, duration_not_tracked; Touch turns creature to mist: flying 10 ft, resistance to nonmagical damage, save advantage. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Portal (`gate`) | 9 | reminder | low | conc | movement, zone, duration_not_tracked; Portal to another plane up to 1 minute; can pull a named creature through. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Missão (`geas`) | 5 | absent | low | - | rider, condition:charmed, upcast_extra, duration_not_tracked; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Repouso Tranquilo (`gentle-repose`) | 2 | reminder | low | - | Preserves a corpse for 10 days, preventing undead and extending raise dead limit. | `cast-result.ts:63 (plain)` | no doc |
| Inseto Gigante (`giant-insect`) | 4 | reminder | medium | conc | creation; Transforms small beasts into giant versions obeying commands; GM runs stats. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Loquacidade (`glibness`) | 8 | reminder | low | - | stat_change, duration_not_tracked; For an hour, Charisma checks can be replaced with 15; lies read as truthful. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Globo de Invulnerabilidade (`globe-of-invulnerability`) | 6 | reminder | low | conc, area: targets picked by hand | zone, upcast_extra, duration_not_tracked; 10-foot barrier blocks spells of 5th level or lower cast from outside. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Glifo de Vigilância (`glyph-of-warding`) | 3 | absent | medium | - | zone, rider, damage_not_rolled, upcast_extra; casting time of 1 hour: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Bom Fruto (`goodberry`) | 1 | reminder | medium | - | creation; Creates ten berries, each healing 1 HP when eaten, potent for 24 hours. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Área Escorregadia (`grease`) | 1 | partial | high | save_roll, area: targets picked by hand | repeat_save, movement, zone, condition:prone, duration_not_tracked; 10-foot square of difficult terrain; Dex save or fall prone, repeated on entry. | `combat_spells.go:636` | rules.md:329 |
| Invisibilidade Maior (`greater-invisibility`) | 4 | reminder | high | conc | condition:invisible, duration_not_tracked; Caster or touched creature becomes invisible for up to one minute; concentration. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Restauração Maior (`greater-restoration`) | 5 | reminder | medium | - | Touch removes one exhaustion level or one chosen curse, charm, petrification, or reduction. | `cast-result.ts:63 (plain)` | no doc |
| Guardião da Fé (`guardian-of-faith`) | 4 | partial | medium | save_roll, damage, area: targets picked by hand | ongoing_damage, zone, duration_not_tracked; Spectral guardian damages hostile creatures entering its reach; Dex save for 20 radiant. | `combat_spells.go:636` | rules.md:329 |
| Proteger Fortaleza (`guards-and-wards`) | 6 | absent | low | - | zone, duration_not_tracked; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Orientação (`guidance`) | 0 | reminder | high | conc | stat_change, duration_not_tracked; Touch grants a d4 bonus to one ability check, once, before it ends. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Raio Guiador (`guiding-bolt`) | 1 | partial | high | attack_roll, damage, upcast_dice | stat_change, rider, duration_not_tracked; Ranged spell attack for 4d6 radiant; hit gives attackers advantage on next attack. | `combat_spells.go:598` | rules.md:329 |
| Lufada de Vento (`gust-of-wind`) | 2 | partial | high | save_roll, conc, area: targets picked by hand | movement, zone, duration_not_tracked; Strength save or pushed 15 feet in a 60-foot line; costs double movement toward caster. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Consagrar (`hallow`) | 5 | absent | low | - | repeat_save, zone, condition:frightened; casting time of 24 hours: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Terreno Alucinógeno (`hallucinatory-terrain`) | 4 | absent | low | - | zone, duration_not_tracked; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Doença Plena (`harm`) | 6 | partial | low | save_roll, damage | stat_change; Con save for 14d6 necrotic, half on success; failed save cuts max HP for an hour. | `combat_spells.go:636` | rules.md:329 |
| Velocidade (`haste`) | 3 | reminder | high | conc | movement, stat_change, duration_not_tracked; Doubles speed, +2 AC, extra action each turn; lethargy after it ends. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Cura Completa (`heal`) | 6 | built | low | hp_effect:flat_heal, upcast_dice | Restores 70 hit points and ends blindness, deafness, and disease. | `combat_spells_hp.go:91` | - |
| Palavra Curativa (`healing-word`) | 1 | built | low | heal, upcast_dice | Bonus action: heal chosen creature in range 1d4 plus spellcasting modifier. | `combat_spells.go:574` | - |
| Esquentar Metal (`heat-metal`) | 2 | partial | high | save_roll, damage, upcast_dice, conc | ongoing_damage, stat_change, rider, duration_not_tracked; 2d8 fire contact damage, repeatable by bonus action; Con save or drop object or disadvantage. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Repreensão Infernal (`hellish-rebuke`) | 1 | absent | high | - | reaction spell: the cast RPC refuses it (CASTING reason REACTION_ONLY) | `turn.go:482` | architecture.md:821 |
| Banquete de Heróis (`heroes-feast`) | 6 | absent | low | - | stat_change; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Heroísmo (`heroism`) | 1 | reminder | high | conc | duration_not_tracked; Touch grants frightened immunity and temp HP each turn; concentration up to a minute. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Riso Histérico (`hideous-laughter`) | 1 | partial | high | save_roll, conc | repeat_save, condition:prone+incapacitated, duration_not_tracked; Wisdom save or prone and incapacitated; repeats save at turn end or on damage. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Imobilizar Monstro (`hold-monster`) | 5 | partial | high | save_roll, conc | repeat_save, condition:paralyzed, duration_not_tracked; Wisdom save or paralyzed; repeats save at end of each round, up to 1 minute. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Imobilizar Pessoa (`hold-person`) | 2 | partial | high | save_roll, conc | repeat_save, condition:paralyzed, upcast_extra, duration_not_tracked; Wis save or paralyzed; repeat save each turn ends it; concentration, 1 minute. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Aura Sagrada (`holy-aura`) | 8 | reminder | low | conc, area: targets picked by hand | stat_change, rider, condition:blinded, duration_not_tracked; Allies get advantage on saves; fiend/undead melee hitters risk blindness. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Marca do Caçador (`hunters-mark`) | 1 | reminder | high | conc | upcast_extra, duration_not_tracked; Extra 1d6 weapon damage on marked target; re-mark after kill, concentration. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Padrão Hipnótico (`hypnotic-pattern`) | 3 | partial | high | save_roll, conc, area: targets picked by hand | condition:charmed+incapacitated, duration_not_tracked; Area Wisdom save or charmed, incapacitated, speed 0; ends on damage. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Tempestade de Gelo (`ice-storm`) | 4 | partial | medium | save_roll, damage, upcast_dice, area: targets picked by hand | movement, zone; Dex save for 2d8 bludgeoning plus 4d6 cold; leaves difficult terrain. | `combat_spells.go:636` | rules.md:329 |
| Identificação (`identify`) | 1 | absent | high | - | casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Escrita Ilusória (`illusory-script`) | 1 | absent | low | - | casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Aprisionamento (`imprisonment`) | 9 | absent | low | - | movement, condition:restrained, creation; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Nuvem Incendiária (`incendiary-cloud`) | 8 | partial | low | save_roll, damage, conc, area: targets picked by hand | ongoing_damage, movement, zone, duration_not_tracked; 10d8 fire save for half; cloud persists, moves 10 feet per turn. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Infligir Ferimentos (`inflict-wounds`) | 1 | built | low | attack_roll, damage, upcast_dice | Melee spell attack; 3d10 necrotic damage on hit. | `combat_spells.go:598` | - |
| Praga de Insetos (`insect-plague`) | 5 | partial | medium | save_roll, damage, upcast_dice, conc, area: targets picked by hand | ongoing_damage, movement, zone, duration_not_tracked; Con save for 4d10 piercing; persistent sphere, difficult terrain, damage on entry. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Invocação Instantânea (`instant-summons`) | 6 | absent | low | - | movement; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Invisibilidade (`invisibility`) | 2 | reminder | high | conc | condition:invisible, upcast_extra, duration_not_tracked; Target invisible until it attacks or casts; concentration, 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Dança Irresistível (`irresistible-dance`) | 6 | reminder | low | conc | repeat_save, stat_change, duration_not_tracked; Target dances with disadvantage on Dex saves and attacks; Wis save ends. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Salto (`jump`) | 1 | reminder | medium | - | movement, duration_not_tracked; Touched creature's jump distance tripled for 1 minute, no concentration. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Arrombar (`knock`) | 2 | reminder | medium | - | Unlocks or unbars one object; audible knock up to 300 feet away. | `cast-result.ts:63 (plain)` | no doc |
| Conhecimento Lendário (`legend-lore`) | 5 | absent | low | - | casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Restauração Menor (`lesser-restoration`) | 2 | reminder | high | - | Touch: end one disease or one blinded, deafened, paralyzed, or poisoned condition. | `cast-result.ts:63 (plain)` | no doc |
| Levitação (`levitate`) | 2 | reminder | medium | conc | movement, duration_not_tracked; Raises target up to 20 feet; unwilling Con save; concentration, 10 minutes. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Luz (`light`) | 0 | reminder | medium | - | zone, duration_not_tracked; Touched object sheds 20-foot bright light for 1 hour; save if hostile-held. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Relâmpago (`lightning-bolt`) | 3 | built | low | save_roll, damage, upcast_dice, area: targets picked by hand | Dex save; 8d6 lightning in 100-foot line, half on success. | `combat_spells.go:636` | - |
| Localizar Animais ou Plantas (`locate-animals-or-plants`) | 2 | reminder | low | - | Direction and distance to nearest chosen beast or plant, 5 miles. | `cast-result.ts:63 (plain)` | no doc |
| Localizar Criatura (`locate-creature`) | 4 | reminder | low | conc | Senses direction to a known creature within 1,000 feet; concentration, 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | no doc |
| Localizar Objeto (`locate-object`) | 2 | reminder | low | conc | Direction to known or nearest object kind within 1,000 feet; concentration. | `combat_spells.go:418`; `cast-result.ts:63` | no doc |
| Passos Longos (`longstrider`) | 1 | reminder | medium | - | movement, upcast_extra, duration_not_tracked; Touched creature's speed +10 feet for 1 hour; upcast adds targets. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Armadura Arcana (`mage-armor`) | 1 | reminder | high | - | stat_change, duration_not_tracked; Willing unarmored creature gets AC 13 plus Dex modifier for 8 hours. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Mãos Mágicas (`mage-hand`) | 0 | reminder | medium | - | movement, creation; Spectral hand manipulates objects within 30 feet; can't attack, 1 minute. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Círculo Mágico (`magic-circle`) | 3 | absent | medium | - | stat_change, zone, upcast_extra, duration_not_tracked; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Recipiente Arcano (`magic-jar`) | 6 | absent | low | - | casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Mísseis Mágicos (`magic-missile`) | 1 | built | low | darts, damage, upcast_dice | darts shared among targets (combat_spells.go openDarts) | `combat_spells.go:670` | - |
| Boca Encantada (`magic-mouth`) | 2 | absent | medium | - | zone; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Arma Mágica (`magic-weapon`) | 2 | reminder | medium | conc | stat_change, upcast_extra, duration_not_tracked; Touched nonmagical weapon gets +1 attack and damage; upcast +2/+3. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Mansão Magnífica (`magnificent-mansion`) | 7 | absent | low | - | creation; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Imagem Maior (`major-image`) | 3 | reminder | low | conc | creation, upcast_extra; Illusory object or creature up to 20-foot cube; Investigation check reveals it. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Curar Ferimentos em Massa (`mass-cure-wounds`) | 5 | built | low | heal, upcast_dice, area: targets picked by hand | Six creatures in 30-foot sphere heal 3d8 plus modifier; not undead. | `combat_spells.go:574` | - |
| Cura Completa em Massa (`mass-heal`) | 9 | partial | low | heal, area: targets picked by hand | pool_not_shared_out; heal_at_slot_level is the flat 700: every target gets 700 (full heal), not a pool the caster divides; the ended conditions are not applied | `combat_spells.go:574` | rules.md:329 |
| Palavra Curativa em Massa (`mass-healing-word`) | 3 | built | low | heal, upcast_dice | Six creatures heal 1d4 plus modifier; bonus action; no undead. | `combat_spells.go:574` | - |
| Sugestão em Massa (`mass-suggestion`) | 6 | partial | low | save_roll | effect_not_applied, duration_not_tracked; the save is rolled, the suggestion itself is the table's | `combat_spells.go:636` | rules.md:329 |
| Labirinto (`maze`) | 8 | reminder | low | conc | repeat_save, movement, duration_not_tracked; Banishes creature into maze; it escapes on DC 20 Int check; concentration. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Mesclar-se às Rochas (`meld-into-stone`) | 3 | reminder | low | - | movement; Hide inside stone 8 hours; expelled by destruction, dealing 6d6 bludgeoning. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Consertar (`mending`) | 0 | absent | medium | - | casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Mensagem (`message`) | 0 | reminder | medium | - | Whispered communication to one creature within range; no mechanical effect. | `cast-result.ts:63 (plain)` | no doc |
| Chuva de Meteoros (`meteor-swarm`) | 9 | built | low | save_roll, damage, area: targets picked by hand | Four 40-foot spheres, Dex save, 20d6 fire and bludgeoning, half on save. | `combat_spells.go:636` | - |
| Limpar a Mente (`mind-blank`) | 8 | reminder | low | - | stat_change, duration_not_tracked; Target gains immunities to psychic damage, mind-reading, divination, charm for 24h. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Ilusão Menor (`minor-illusion`) | 0 | reminder | medium | - | creation; Creates a sound or small object image for 1 minute; out-of-combat utility. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Miragem (`mirage-arcane`) | 7 | absent | low | - | movement, zone, duration_not_tracked; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Reflexos (`mirror-image`) | 2 | reminder | high | - | stat_change, creation, duration_not_tracked; Creates three duplicates that absorb attacks targeting caster for 1 minute. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Despistar (`mislead`) | 5 | reminder | medium | conc | movement, condition:invisible+blinded+deafened, creation, duration_not_tracked; Caster turns invisible and creates an illusory double that it moves by action. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Passo Nebuloso (`misty-step`) | 2 | reminder | high | - | movement; Bonus action teleport up to 30 feet to a visible unoccupied space. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Modificar Memória (`modify-memory`) | 5 | partial | medium | save_roll, conc | condition:charmed+incapacitated, upcast_extra, duration_not_tracked; Wisdom save or charmed and incapacitated for 1 minute; caster edits memories. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Raio Lunar (`moonbeam`) | 2 | partial | high | save_roll, damage, upcast_dice, conc, area: targets picked by hand | ongoing_damage, zone, upcast_extra, duration_not_tracked; Radiant damage save when entering or starting turn in beam; caster moves beam. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Mover Terra (`move-earth`) | 6 | reminder | low | conc, area: targets picked by hand | zone, duration_not_tracked; Reshapes 40-foot terrain area over 10 minutes with concentration; no direct damage. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Dificultar Detecção (`nondetection`) | 3 | reminder | low | - | Hides touched creature or object from divination and scrying for 8 hours. | `cast-result.ts:63 (plain)` | no doc |
| Passos sem Pegadas (`pass-without-trace`) | 2 | reminder | medium | conc | stat_change, duration_not_tracked; Chosen allies within 30 feet get +10 Stealth and leave no tracks, 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Criar Passagem (`passwall`) | 5 | reminder | medium | - | movement, zone, duration_not_tracked; Creates a 5x8x20-foot passage through wooden, plaster, or stone surfaces for 1 hour. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Assassino Fantasmagórico (`phantasmal-killer`) | 4 | partial | medium | save_roll, damage, upcast_dice, conc | repeat_save, ongoing_damage, condition:frightened, upcast_extra, duration_not_tracked; Wis save or frightened; each turn, Wis save or 4d10 psychic damage. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Montaria Fantasmagórica (`phantom-steed`) | 3 | absent | medium | - | movement, creation; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Aliado Planar (`planar-ally`) | 6 | absent | low | - | creation; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Âncora Planar (`planar-binding`) | 5 | absent | low | - | creation, upcast_extra; casting time of 1 hour: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Viagem Planar (`plane-shift`) | 7 | partial | low | attack_roll, save_roll | movement, rider; Teleports up to 8 willing creatures to another plane; melee attack banishes foes. | `combat_spells.go:598`; `combat_spells.go:636` | rules.md:329 |
| Ampliar Plantas (`plant-growth`) | 3 | reminder | medium | area: targets picked by hand | movement, zone; 100-foot area becomes difficult terrain; 8-hour casting enriches land for a year. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Rajada de Veneno (`poison-spray`) | 0 | built | low | save_roll, damage | Con save or 1d12 poison damage, scaling with caster level. | `combat_spells.go:636` | - |
| Metamorfose (`polymorph`) | 4 | partial | high | save_roll, conc | stat_change, duration_not_tracked; Wis save or transform into a beast of equal or lower CR for 1 hour. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Palavra de Poder Matar (`power-word-kill`) | 9 | built | low | hp_effect:hp_threshold | Instantly kills one visible creature with 100 or fewer hit points. | `combat_spells_hp.go:91` | - |
| Palavra de Poder Atordoar (`power-word-stun`) | 8 | partial | low | hp_effect:hp_threshold | repeat_save; Stuns one creature with 150 or fewer HP; Con save ends it each turn. | `combat_spells_hp.go:91` | rules.md:329 |
| Oração Curativa (`prayer-of-healing`) | 2 | absent | medium | - | casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Prestidigitação (`prestidigitation`) | 0 | reminder | medium | - | creation; Minor non-combat trick chosen from a menu; lasts up to 1 hour. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Rajada Prismática (`prismatic-spray`) | 7 | partial | low | save_roll, area: targets picked by hand | repeat_save, movement, rider, condition:restrained+blinded+petrified; Random ray per target: five damage colors, plus restrain, blind, or planar banish. | `combat_spells.go:636` | rules.md:329 |
| Muralha Prismática (`prismatic-wall`) | 9 | reminder | low | area: targets picked by hand | repeat_save, zone, rider, condition:blinded+restrained+petrified, duration_not_tracked; 10-minute opaque wall or sphere; seven damaging layers, blinding and petrifying effects. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Santuário Particular (`private-sanctum`) | 4 | absent | low | - | zone, upcast_extra, duration_not_tracked; casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Criar Chamas (`produce-flame`) | 0 | partial | high | attack_roll, damage | zone, duration_not_tracked; Ranged spell attack for 1d8 fire, scaling with level; flame lasts 10 minutes. | `combat_spells.go:598` | rules.md:329 |
| Ilusão Programada (`programmed-illusion`) | 6 | reminder | low | area: targets picked by hand | creation; Illusion of up to 30-foot cube triggers on a set condition, then performs. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Projetar Imagem (`project-image`) | 7 | reminder | low | conc | movement, creation; Intangible copy at a seen location; ends when damaged; concentration up to 24 hours. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Proteção contra Energia (`protection-from-energy`) | 3 | reminder | medium | conc | stat_change, duration_not_tracked; Touched ally gains resistance to acid, cold, fire, lightning, or thunder for 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Proteção contra o Bem e Mal (`protection-from-evil-and-good`) | 1 | reminder | high | conc | stat_change, duration_not_tracked; Touched ally: listed creature types get disadvantage on attacks and can't charm, frighten, or possess. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Proteção contra Veneno (`protection-from-poison`) | 2 | reminder | medium | - | stat_change, duration_not_tracked; Neutralizes poison on touched creature; advantage vs poison and poison resistance for 1 hour. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Purificar Alimentos (`purify-food-and-drink`) | 1 | reminder | low | area: targets picked by hand | Removes poison and disease from food and drink in a 5-foot sphere. | `cast-result.ts:63 (plain)` | no doc |
| Reviver os Mortos (`raise-dead`) | 5 | absent | high | - | stat_change; casting time of 1 hour: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Raio do Enfraquecimento (`ray-of-enfeeblement`) | 2 | partial | high | attack_roll, save_roll, conc | repeat_save, stat_change, duration_not_tracked; Ranged spell attack halves target's Strength weapon damage until Con save ends it. | `combat_spells.go:418`; `combat_spells.go:598`; `combat_spells.go:636` | rules.md:329 |
| Raio de Gelo (`ray-of-frost`) | 0 | partial | high | attack_roll, damage | movement; Ranged spell attack for 1d8 cold and -10 speed until your next turn. | `combat_spells.go:598` | rules.md:329 |
| Regeneração (`regenerate`) | 7 | absent | low | - | casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Reencarnação (`reincarnate`) | 5 | absent | medium | - | creation; casting time of 1 hour: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Remover Maldição (`remove-curse`) | 3 | reminder | low | - | Ends all curses on one creature or object; cursed item's curse remains. | `cast-result.ts:63 (plain)` | no doc |
| Esfera Resiliente (`resilient-sphere`) | 4 | partial | medium | save_roll, conc | movement, zone, duration_not_tracked; Dex save or enclosed in an immune force sphere for a minute. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Resistência (`resistance`) | 0 | reminder | medium | conc | stat_change, duration_not_tracked; Touched ally adds a d4 to one saving throw, once. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Ressurreição (`resurrection`) | 7 | absent | high | - | stat_change; casting time of 1 hour: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Inverter a Gravidade (`reverse-gravity`) | 7 | partial | low | save_roll, conc, area: targets picked by hand | movement, zone, duration_not_tracked; Creatures in a 100-foot cylinder fall upward for a minute, Dex save to grab. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Revivificar (`revivify`) | 3 | reminder | high | - | Returns a creature dead under one minute to 1 hit point. | `cast-result.ts:63 (plain)` | no doc |
| Truque de Corda (`rope-trick`) | 2 | reminder | low | - | creation; Rope leads to an extradimensional hideout for eight creatures for an hour. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Chama Sagrada (`sacred-flame`) | 0 | built | low | save_roll, damage | Dex save or 1d8 radiant damage, scaling with character level. | `combat_spells.go:636` | - |
| Santuário (`sanctuary`) | 1 | reminder | medium | - | duration_not_tracked; Attackers must Wis save or retarget; ends if ward attacks. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Raio Ardente (`scorching-ray`) | 2 | built | low | attack_roll, damage | one ray per target (several rays on one target are the master's, play/combat_spells.go:44) | `combat_spells.go:598` | - |
| Vidência (`scrying`) | 5 | absent | medium | - | casting time of 10 minutes: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Arca Secreta (`secret-chest`) | 4 | reminder | low | - | creation; Stores a 12 cubic foot chest on the Ethereal Plane; recallable via replica. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Ver o Invisível (`see-invisibility`) | 2 | reminder | low | - | Lets caster see invisible creatures and Ethereal things for one hour. | `cast-result.ts:63 (plain)` | no doc |
| Similaridade (`seeming`) | 5 | reminder | low | - | Cha save for unwilling targets to change appearance; lasts eight hours. | `cast-result.ts:63 (plain)` | no doc |
| Enviar Mensagem (`sending`) | 3 | reminder | low | - | Sends a 25-word telepathic message to a familiar creature, which can reply. | `cast-result.ts:63 (plain)` | no doc |
| Isolamento (`sequester`) | 7 | reminder | low | - | Hides a willing creature or object in stasis until dispelled or triggered. | `cast-result.ts:63 (plain)` | no doc |
| Alterar Forma (`shapechange`) | 9 | reminder | low | conc | stat_change, duration_not_tracked; Transform into a creature of CR up to your level, using its stats. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Despedaçar (`shatter`) | 2 | built | low | save_roll, damage, upcast_dice, area: targets picked by hand | 3d8 thunder, Con save; the inorganic-object clause is flavour | `combat_spells.go:636` | - |
| Escudo Arcano (`shield`) | 1 | built | low | - | UseReaction: +5 AC until the next turn and the hit is judged again (play/combat_reactions.go) | `combat_reactions.go:169` | - |
| Escudo da Fé (`shield-of-faith`) | 1 | reminder | high | conc | stat_change, duration_not_tracked; Bonus action grants a creature +2 AC while concentrated, up to 10 minutes. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Bordão Místico (`shillelagh`) | 0 | reminder | medium | - | stat_change, duration_not_tracked; Bonus action makes a club or staff use spellcasting ability and d8 damage. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Toque Chocante (`shocking-grasp`) | 0 | partial | high | attack_roll, damage | stat_change, rider; Melee spell attack, 1d8 lightning; target loses reactions until its next turn. | `combat_spells.go:598` | rules.md:329 |
| Silêncio (`silence`) | 2 | reminder | medium | conc, area: targets picked by hand | zone, condition:deafened, duration_not_tracked; Silences a 20-foot sphere; creatures inside are deafened, no verbal spells. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Imagem Silenciosa (`silent-image`) | 1 | reminder | low | conc, area: targets picked by hand | creation; Creates a 15-foot-cube visual illusion you can move; Int check reveals it. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Simulacro (`simulacrum`) | 7 | absent | low | - | creation; casting time of 12 hours: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Sono (`sleep`) | 1 | built | low | hp_effect:hp_pool, area: targets picked by hand | duration_not_tracked; hp_pool effect: unconscious applied in ascending HP order; waking up is the table's | `combat_spells_hp.go:91` | - |
| Nevasca (`sleet-storm`) | 3 | reminder | high | conc, area: targets picked by hand | movement, zone, rider, condition:prone, duration_not_tracked; Icy 40-foot cylinder: Dex save or fall prone, difficult terrain, obscures. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Lentidão (`slow`) | 3 | partial | high | save_roll, conc, area: targets picked by hand | repeat_save, movement, stat_change, duration_not_tracked; Up to six creatures: Wis save, halved speed, -2 AC, one attack, repeat save. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Estabilizar (`spare-the-dying`) | 0 | built | low | hp_effect:zero_hp_target | Touch a 0 HP living creature to stabilize it; no effect on undead. | `combat_spells_hp.go:91` | - |
| Falar com Animais (`speak-with-animals`) | 1 | reminder | medium | - | Lets caster talk with beasts for ten minutes; favors at GM's discretion. | `cast-result.ts:63 (plain)` | no doc |
| Falar com os Mortos (`speak-with-dead`) | 3 | reminder | low | - | Corpse answers up to five questions for ten minutes; cannot be undead. | `cast-result.ts:63 (plain)` | no doc |
| Falar com Plantas (`speak-with-plants`) | 3 | reminder | low | - | movement, zone, duration_not_tracked; Talk to plants for info; can toggle terrain to difficult for ten minutes. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Patas de Aranha (`spider-climb`) | 2 | reminder | medium | conc | movement, duration_not_tracked; Touched ally climbs walls and ceilings at walking speed for an hour, concentration. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Crescer Espinhos (`spike-growth`) | 2 | reminder | medium | damage_table_only, conc, area: targets picked by hand | ongoing_damage, movement, zone, damage_not_rolled, duration_not_tracked; Difficult spiky terrain; 2d4 piercing per 5 feet moved, for ten minutes. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Espíritos Guardiões (`spirit-guardians`) | 3 | partial | high | save_roll, damage, damage_type_choice, upcast_dice, conc, area: targets picked by hand | repeat_save, ongoing_damage, movement, zone, duration_not_tracked; Aura 15 ft around you: halved speed; Wis save or 3d8 radiant/necrotic. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Arma Espiritual (`spiritual-weapon`) | 2 | partial | high | attack_roll, damage, upcast_dice | zone, duration_not_tracked; Floating force weapon attacks for 1d8+mod on cast and each bonus action, one minute. | `combat_spells.go:598` | rules.md:329 |
| Névoa Fétida (`stinking-cloud`) | 3 | partial | high | save_roll, conc, area: targets picked by hand | repeat_save, zone, duration_not_tracked; Con save each turn in 20-foot sphere; failure wastes the creature's action. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Moldar Rochas (`stone-shape`) | 4 | reminder | low | - | Reshapes a stone object or 5-foot section into any desired shape. | `cast-result.ts:63 (plain)` | no doc |
| Pele de Pedra (`stoneskin`) | 4 | reminder | medium | conc | stat_change, duration_not_tracked; Target gains resistance to nonmagical physical damage for up to 1 hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Tempestade da Vingança (`storm-of-vengeance`) | 9 | partial | low | save_roll, damage, conc, area: targets picked by hand | ongoing_damage, zone, rider, condition:deafened, duration_not_tracked; Storm cloud zone dealing damage each round for up to one minute. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Sugestão (`suggestion`) | 2 | partial | high | save_roll, conc | effect_not_applied, duration_not_tracked; the save is rolled, the suggestion itself is the table's | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Raio Solar (`sunbeam`) | 6 | partial | high | save_roll, damage, conc, area: targets picked by hand | stat_change, zone, rider, condition:blinded, duration_not_tracked; Radiant line damage save; blinds on fail; persistent light mote. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Explosão Solar (`sunburst`) | 8 | partial | low | save_roll, damage, area: targets picked by hand | repeat_save, stat_change, rider, condition:blinded; Radiant damage save; blinds for 1 minute, repeatable each turn end. | `combat_spells.go:636` | rules.md:329 |
| Símbolo (`symbol`) | 7 | absent | low | - | repeat_save, movement, stat_change, zone, rider, condition:frightened+incapacitated+unconscious+stunned; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Telecinésia (`telekinesis`) | 5 | reminder | medium | conc | movement, condition:restrained, duration_not_tracked; Contested check moves creature or object 30 feet; restrains moved creature. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Ligação Telepática (`telepathic-bond`) | 5 | reminder | low | - | Links up to eight willing creatures for telepathic communication for one hour. | `cast-result.ts:63 (plain)` | no doc |
| Teletransporte (`teleport`) | 7 | reminder | low | damage_table_only, area: targets picked by hand | movement, rider, damage_not_rolled; Teleports up to nine willing creatures; off-target mishaps deal 3d10 force. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Círculo de Teletransporte (`teleportation-circle`) | 5 | absent | low | - | zone, duration_not_tracked; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Taumaturgia (`thaumaturgy`) | 0 | reminder | low | - | Cosmetic or minor sensory effects lasting up to one minute; no combat use. | `cast-result.ts:63 (plain)` | no doc |
| Onda Trovejante (`thunderwave`) | 1 | partial | high | save_roll, damage, upcast_dice, area: targets picked by hand | movement; 15-foot cube save: 2d8 thunder damage and 10-foot push on fail. | `combat_spells.go:636` | rules.md:329 |
| Parar o Tempo (`time-stop`) | 9 | reminder | low | - | Caster takes 1d4+1 extra turns; ends if caster affects others. | `cast-result.ts:63 (plain)` | no doc |
| Pequena Cabana (`tiny-hut`) | 3 | absent | high | - | zone, duration_not_tracked; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Idiomas (`tongues`) | 3 | reminder | low | - | Touched creature understands and is understood in any spoken language, one hour. | `cast-result.ts:63 (plain)` | no doc |
| Teletransporte por Árvores (`transport-via-plants`) | 6 | reminder | low | - | movement, zone, duration_not_tracked; Creates a one-round plant link for creatures to travel between plants. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Caminhar em Árvores (`tree-stride`) | 5 | reminder | low | conc | movement; Move between same-kind trees within 500 feet, once per round, for one minute. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Metamorfose Verdadeira (`true-polymorph`) | 9 | reminder | low | conc | stat_change, creation, duration_not_tracked; Transforms creature or object; lasts one hour or becomes permanent if concentrated. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Ressurreição Verdadeira (`true-resurrection`) | 9 | absent | low | - | creation; casting time of 1 hour: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Visão da Verdade (`true-seeing`) | 6 | reminder | low | - | duration_not_tracked; Touched ally gains truesight 120 feet for one hour. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Ataque Certeiro (`true-strike`) | 0 | reminder | medium | conc | stat_change, duration_not_tracked; Grants advantage on your first attack against the target next turn. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Servo Invisível (`unseen-servant`) | 1 | reminder | medium | - | movement, creation; Creates invisible 1-HP servant for one hour; performs simple tasks. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Toque Vampírico (`vampiric-touch`) | 3 | partial | high | attack_roll, damage, upcast_dice, conc | heal_half_not_applied, repeat_attack_not_run; the melee spell attack and 3d6 necrotic are rolled; the caster's healing (half the damage) and the later attacks are not | `combat_spells.go:418`; `combat_spells.go:598` | rules.md:329 |
| Zombaria Viciosa (`vicious-mockery`) | 0 | partial | high | save_roll, damage | stat_change; Wisdom save: 1d4 psychic damage and next attack at disadvantage. | `combat_spells.go:636` | rules.md:329 |
| Muralha de Fogo (`wall-of-fire`) | 4 | partial | high | save_roll, damage, upcast_dice, conc, area: targets picked by hand | ongoing_damage, zone, duration_not_tracked; Fire wall: Dex save damage, then ongoing fire damage near one chosen side. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Muralha de Energia (`wall-of-force`) | 5 | reminder | high | conc | movement, zone, duration_not_tracked; Invisible impassable barrier shaped by caster for up to ten minutes. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Muralha de Gelo (`wall-of-ice`) | 6 | partial | low | save_roll, damage, upcast_dice, conc, area: targets picked by hand | ongoing_damage, movement, zone, duration_not_tracked; Ice wall: Dex-save cold damage on creation; frigid air hurts passers. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Muralha de Pedra (`wall-of-stone`) | 5 | reminder | medium | conc | movement, zone, duration_not_tracked; Solid stone wall of panels, up to ten minutes; permanent if concentration held. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Muralha de Espinhos (`wall-of-thorns`) | 6 | partial | low | save_roll, damage, upcast_dice, conc, area: targets picked by hand | ongoing_damage, movement, zone, duration_not_tracked; Thorn wall: Dex-save piercing damage, slashing damage on entry, costs movement. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Vínculo Protetor (`warding-bond`) | 2 | reminder | medium | - | stat_change, rider, duration_not_tracked; Touched ally gets +1 AC and saves, resistance; caster shares damage. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Respirar na Água (`water-breathing`) | 3 | reminder | low | - | Up to ten creatures can breathe underwater for 24 hours. | `cast-result.ts:63 (plain)` | no doc |
| Andar na Água (`water-walk`) | 3 | reminder | low | - | movement; Up to ten creatures walk on liquids for one hour; can lift submerged creatures. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Teia (`web`) | 2 | reminder | high | damage_table_only, conc, area: targets picked by hand | repeat_save, ongoing_damage, movement, zone, condition:restrained, damage_not_rolled, duration_not_tracked; Webs restrain creatures in a 20-foot cube; difficult terrain, flammable, for one hour. | `combat_spells.go:418`; `cast-result.ts:63` | rules.md:329 |
| Encarnação Fantasmagórica (`weird`) | 9 | partial | low | save_roll, conc, area: targets picked by hand | repeat_save, ongoing_damage, condition:frightened, duration_not_tracked; Frightens creatures in 30-foot sphere; repeated Wisdom save or 4d10 psychic damage. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Caminhar no Vento (`wind-walk`) | 6 | absent | low | - | movement, stat_change, condition:incapacitated, duration_not_tracked; casting time of 1 minute: refused in combat (CASTING_TIME_TOO_LONG) and there is no cast flow outside combat | `turn.go:478` | architecture.md:823 |
| Muralha de Vento (`wind-wall`) | 3 | partial | high | save_roll, damage, conc, area: targets picked by hand | movement, zone, duration_not_tracked; Strength save for 3d8 bludgeoning; wall blocks flyers, gas and arrows. | `combat_spells.go:418`; `combat_spells.go:636` | rules.md:329 |
| Desejo (`wish`) | 9 | reminder | low | - | stat_change, rider, creation; Duplicates spell up to 8th level or gives many effects; costs stress and Strength. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Palavra de Recordação (`word-of-recall`) | 6 | reminder | low | area: targets picked by hand | movement; Teleports you and five willing creatures to a prepared sanctuary. | `cast-result.ts:63 (plain)` | rules.md:329 |
| Zona da Verdade (`zone-of-truth`) | 2 | reminder | medium | area: targets picked by hand | repeat_save, zone, duration_not_tracked; Charisma save each turn in a 15-foot sphere, or the creature cannot lie. | `cast-result.ts:63 (plain)` | rules.md:329 |
