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

The five engine facts that most rows depend on, and the shared causes, are in section 7.

## Summary

### Counts by part and state

| Part | Unit counted | built | partial | reminder | absent | Total |
| --- | --- | --- | --- | --- | --- | --- |
| 1. Spells | spells of `data/spells.json` | 35 | 82 | 142 | 60 | 319 |
| 2. Equipment | rules rows (14 weapon properties and rules, 8 armour and shield, 11 gear / tools / mounts / coins); all 37 weapons, 12 armours and the shield do produce an attack line or an AC | 8 | 5 | 5 | 15 | 33 |
| 3. Magic items | rows by kind of effect (12); per item: 362 catalogued (text, rarity, value), **0 whose effect the app applies** | 2 | 0 | 5 | 5 | 12 |
| 4. Monsters | rows by kind of feature (28); per monster: 334 fight as an NPC with HP, AC and speed; 311 with at least one attack line, **23 with none** (19 whose only damage is a flat 1, 4 with no attack action) | 6 | 2 | 10 (9 + 1 mixed) | 10 | 28 |
| 5. Races | the 38 traits of `data/traits.json` | 11 | 3 | 24 | 0 | 38 |
| 6. Backgrounds and feats | rows (5 background, 5 feat) | 3 | 1 | 0 | 5 | 9 + 1 context row |

How to read the unit: a **spell** is one row; for monsters and items the unit is a kind of feature, because the same
mechanism covers many entries (for example 134 save actions in 86 monsters are one row). The per-entry facts that matter are
in the second column.

What the app runs well, with evidence: attack rolls and saving throws with damage (full, half, none), healing, cantrip
scaling and slot upcasting, damage-type choices, 8 hit-point spells, Shield, three summons, bonus-action-spell limits,
concentration prompts, AC from armour and shields, Strength requirement of armour, finesse / light / reach / monk weapons, darkvision
in the fog of war, a monster's AC / HP / Multiattack count / plain resistances / opportunity attacks, ability bonuses,
speeds and proficiencies from races, and the custom ("Outro") background.

### The 20 gaps with the highest table impact

Ordered by how soon a table hits them. "Cause" points to the shared causes of section 7.

| # | Gap | Scope | Cause | Impact |
| --- | --- | --- | --- | --- |
| 1 | A spell that should impose a condition (Hold Person, Web, Entangle, Fear, Hypnotic Pattern, Blindness...) rolls the save and nothing else; the master marks the label and the label does nothing but stop movement | at least 38 spells, 129 monsters with a rider in an action | C1 | high |
| 2 | No advantage or disadvantage in any roll: Bless / Bane / Guidance as roll bonuses, Pack Tactics, Magic Resistance, Lucky, Fey Ancestry, Brave, Gnome Cunning, Dwarven Resilience, flanking, long range, heavy weapons, stealth armour | 12 race traits, 48 monsters (Pack Tactics 17, Magic Resistance 31), 8 weapon / armour rows, spells | C2 | high |
| 3 | Buff and debuff spells compute nothing (Bless, Bane, Haste, Slow, Mage Armor, Shield of Faith, Hunter's Mark, Heroism, Blur, Barkskin, Enlarge / Reduce...) | 53 spells | C4 | high |
| 4 | Nothing expires or repeats: durations, saves at the end of a turn (Hold Person, Hold Monster, Fear), ongoing damage (Cloudkill, Spirit Guardians, Moonbeam, Acid Arrow, Heat Metal) | 136 spells with a duration, 33 with repeat saves, 22 with ongoing damage | C3 | high |
| 5 | A monster's save actions and breath weapons are never rolled and never recharge | 86 monsters, 134 actions; recharge 71 monsters, 112 actions (every dragon, ghoul, spider, gelatinous cube) | C8, C3 | high |
| 6 | An NPC made from a stat block keeps three attacks with one damage die; **19 monsters have no attack at all** (bat, cat, rat, spider, hawk, raven, badger, owl, sprite...), 28 lose an attack, 57 attacks lose their second damage part to the description | 334 monsters | C8 | high |
| 7 | Legendary actions, Legendary Resistance, Regeneration, Undead Fortitude: text only | 32 + 25 + 7 + 2 monsters (every boss, every troll) | C3, C8 | high |
| 8 | A monster or NPC cannot cast its spells | 36 monsters (Mage, Priest, Archmage, Lich, Drow, Couatl...) | C8 | high |
| 9 | Magic items do nothing: +N weapons and armour, Ring and Cloak of protection, Potion of healing, wands, ability-score items, resistance rings | 362 items, 0 applied | C7 | high |
| 10 | There is no way to hold, drink, attune to or charge an item: no item list, no attunement slots (limit 3), no charges, no coins from treasure | 175 attunement items, 53 with charges, 88 consumables | C7 | high |
| 11 | There is no cast outside a combat: healing between fights, Mage Armor before one, rituals; the slot is spent by hand | every spell cast outside an encounter; 29 rituals (only Find Familiar works) | C6 | high |
| 12 | Spells of a minute or more cannot be cast at all: Identify, Alarm, Tiny Hut, Magic Circle, Raise Dead, Resurrection, Commune, Augury, Mending... | 57 spells | C6 | medium-high |
| 13 | Reactions: Counterspell, Hellish Rebuke, Feather Fall, and every monster reaction (Parry x6) are refused; only Shield and opportunity attacks run | 3 spells, 12 monsters | C9 | high |
| 14 | Concentration is a flag: no automatic save, no end when the caster is incapacitated or dies, no end on duration | 118 concentration spells | C3 | high |
| 15 | Area and zone spells: nobody is picked from a shape; walls, clouds, fog, light and spheres are not on the map | 94 area spells, 57 zone / object spells | C5, C12 | medium-high |
| 16 | Racial and monster triggers: Relentless Endurance (half-orc), Breath Weapon damage (dragonborn), Savage Attacks, Lucky, Infernal Legacy spells | 5 traits | C2, C3 | high |
| 17 | Spells that read a damage table but roll nothing: Web, Flaming Sphere, Spike Growth, Earthquake, Fire Shield, Branding Smite, Divine Favor, Dimension Door, Teleport, Glyph of Warding; and 34 spells whose text asks for a save that the data does not carry | 10 + 34 spells | data | medium |
| 18 | Adventuring gear is not content: torches burn forever, no healer's kit, rope, caltrops, holy water, acid, alchemist's fire, oil, ammunition counts, mounts, vehicles | every adventuring-gear, ammunition, pack, mount and vehicle entry (the importer drops them) | C11 | medium |
| 19 | Weapon rules: versatile (always one-handed in combat), two-handed with a shield, thrown (no loss, no 20 ft limit), long range (no disadvantage), loading, ammunition, net and lance | 6 + 11 + 8 + 7 + 4 + 7 + 2 weapons | C2, C7 | medium |
| 20 | Feats: none, and the level-up offers only the ability score improvement | the whole feat system | C11 | high for a table that wants them |

Also worth the master's attention: condition immunities of 92 monsters and the conditional resistances of 70 ("nonmagical
weapons") are not applied; Sleep ignores undead immunity to charm.

## 1. Spells (319 of `data/spells.json`)

### 1.1 What a cast does, and where

| Piece | State | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- |
| Slot and action spent, bonus-action-spell rule, cantrip scaling by character level | built | `rules/combat/turn.go:458-485` (`spellOption`), `play/combat_spells.go:392` (spend), `:412` (bonus-action rule), `rules/spelldetails.go:237` `DamageAt` | - | - |
| Spell attack roll against AC, crit on 20, cover, first damage type | built | `play/combat_spells.go:598-634` | - | - |
| Saving throw rolled by the server for every target, full / half / none damage, cover on Dex saves, one damage roll for an area | built | `play/combat_spells.go:636-668` | - | - |
| Damage by slot level (upcasting), damage-type choice (`alternative` / `scale`) | built | `characters/combatspells.go:71`, `rules/spelldetails.go:269` `DamageAtChoosing` | - | - |
| Healing with the casting modifier, up to the maximum, revives from 0 | built | `play/combat_spells.go:574`, `play/combat_actions.go:1169` `healCombatant` | - | - |
| 8 spells that read hit points (Sleep, Color Spray, Power Word Stun / Kill, Heal, Aid, False Life, Spare the Dying; `effects/spells.json` also holds Sacred Flame's cover rule and the 3 summons) | built | `play/combat_spells_hp.go:91`, `rules/srd51/effects/spells.json` | no doc (a closed, hand-written list) | - |
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

What the codes mean (a spell can carry several; `partial` and `reminder` spells only):

| Code | Meaning |
| --- | --- |
| `condition:X` | the condition the text imposes is not applied (Hold Person: paralyzed) |
| `repeat_save` | a later saving throw (end of the target's turn, when it is hurt) is not run |
| `ongoing_damage` | damage at a later turn, or when a creature enters / stays in an area, is not rolled |
| `movement` | push, pull, teleport, speed change, flying or difficult terrain is not applied |
| `stat_change` | a bonus, penalty, advantage, resistance or changed score is not applied |
| `zone` | the wall, cloud, light, sphere or object does not exist on the map |
| `rider` | a second effect on a hit or a failed save (blinded on a hit, extra dice, a curse) is not applied |
| `duration_not_tracked` | the effect lasts N rounds / minutes / hours and nothing ends it |
| `creation` | it conjures a creature or an object that is not a modelled summon |
| `upcast_extra` | a higher slot adds something other than dice, healing or targets |
| `damage_not_rolled` | the data has a damage table but no attack or save, so the cast rolls nothing |
| `effect_not_applied`, `pool_not_shared_out`, `heal_half_not_applied`, `repeat_attack_not_run` | one-off gaps named by the hand review (`scripts/spell-overrides.json`) |


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

#### conditions a spell should impose and the app does not apply

- blinded: 9 - blindness-deafness, contagion, divine-word, holy-aura, mislead, prismatic-spray, prismatic-wall, sunbeam, sunburst
- charmed: 7 - animal-friendship, charm-person, dominate-beast, dominate-monster, dominate-person, hypnotic-pattern, modify-memory
- restrained: 7 - black-tentacles, entangle, flesh-to-stone, prismatic-spray, prismatic-wall, telekinesis, web
- deafened: 5 - blindness-deafness, divine-word, mislead, silence, storm-of-vengeance
- prone: 5 - command, earthquake, grease, hideous-laughter, sleet-storm
- incapacitated: 4 - banishment, hideous-laughter, hypnotic-pattern, modify-memory
- frightened: 4 - eyebite, fear, phantasmal-killer, weird
- petrified: 3 - flesh-to-stone, prismatic-spray, prismatic-wall
- invisible: 3 - greater-invisibility, invisibility, mislead
- stunned: 2 - contagion, divine-word
- paralyzed: 2 - hold-monster, hold-person
- grappled: 1 - arcane-hand
- unconscious: 1 - eyebite

#### concentration spells castable in combat: 118 (flag set and DC reminder only; no auto end on duration, incapacitation or failed save)


#### rituals: 29 (alarm, animal-messenger, augury, commune, commune-with-nature, comprehend-languages, contact-other-plane, detect-magic, detect-poison-and-disease, divination, find-familiar, floating-disk, forbiddance, gentle-repose, identify, illusory-script, instant-summons, locate-animals-or-plants, magic-mouth, meld-into-stone, phantom-steed, purify-food-and-drink, silence, speak-with-animals, telepathic-bond, tiny-hut, unseen-servant, water-breathing, water-walk); only find-familiar can be cast as a ritual (CastSummon); 12 of them are absent


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

## 2. Equipment (`data/equipment.json`: 81 items = 37 weapons, 12 armours + the shield, 31 tools)

**What is in the data.** The importer keeps only the SRD's weapons, armour and tools and drops the rest of the
equipment list with `default: continue` (`backend/cmd/srdimport/main.go:914`). So adventuring gear, ammunition, packs, mounts,
vehicles, trade goods and services are **not content at all**: they exist only as free-text lines of a sheet
(`characters.proto:1150` `Item`: a name of up to 100 characters and a quantity, at most 100 lines, `:1003`). Doc: no doc says
this is on purpose (`docs/architecture.md` and `docs/data.md` do not mention the dropped categories).

**How a sheet uses what exists.** `Build.Armor`, `Build.Shield` and `Build.Weapons` are content keys
(`rules/api.go:90-140`); `Derive` turns weapons into attack lines (`rules/attacks.go:36`) and armour into AC
(`rules/armor.go:23`). The combat then rolls those attack lines (`characters/combatturn.go:140-160`).

### 2.1 Weapon properties (37 weapons, counts per property)

Checked by deriving a level 3 Fighter with each weapon (`overlay/zz_equip_dump_test.go`, output `equip.json`).

| Property (weapons) | State | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- |
| Finesse (6) | built: the better of STR / DEX for attack and damage | `rules/attacks.go:50` | - | - |
| Monk weapon (10) | built: Martial Arts die and DEX while unarmoured | `rules/attacks.go:49` | - | - |
| Light (8) | built: the off-hand bonus attack needs two light melee weapons | `rules/attacks.go:77`, `rules/combat/bonusattack.go:67` | - | - |
| Reach (5: glaive, halberd, lance, pike, whip) | built: reach 10 ft in targeting and for opportunity attacks | `rules/attacks.go:92-93`, `play/combat_opportunity.go:66-77` | - | - |
| Versatile (6) | partial: the sheet shows the two-handed damage as text ("Com duas mãos"); the combat always rolls the one-handed dice | `rules/attacks.go:82`, `web/.../combat-column.html:47`; no use in `play/` or `link.Attack` (`play/link/link.go`) | no doc | medium |
| Thrown (8) | partial: the weapon gets its throwing range (`20/60`) as its only range, so a dagger "reaches" 60 ft with no disadvantage and is never gone after a throw | `rules/attacks.go:90-91`, `play/combat_actions.go:181-196` `reachFt` takes the largest of range and long range | no doc | medium |
| Range / long range (ranged: 7 + thrown) | partial: a target within the long range is accepted; the "disadvantage beyond the normal range" is not shown or applied | `play/combat_actions.go:181-196` | `docs/product/rules.md:353` (advantage is not applied) | medium |
| Heavy (8) | reminder: only a Small creature gets a sheet hint "Desvantagem nas jogadas de ataque" | `rules/attacks.go:56-60` | `rules.md:353` | low |
| Two-handed (11) | absent: nothing stops a two-handed weapon with a shield, or a second weapon | no use of the property in `rules/` or `play/` (`grep weapon-property:` finds only finesse, heavy, light, monk) | no doc | medium |
| Ammunition (7) | absent: no ammunition item, no count, no "recover half after the fight" | no `equipment:arrow` in `data/equipment.json` (importer drop, `srdimport/main.go:914`); magic ammunition is only a catalogue entry | no doc | medium |
| Loading (4) | absent: a loading weapon still fires every attack of the Attack action | not read anywhere | no doc | low |
| Special: net (1) | absent: the net has no damage, so the line shows an attack with no effect; no restrained, no size limit, no AC 10 net | `equip.json` (`net`: `Damage: ""`) | no doc | low |
| Special: lance (1) | absent: no disadvantage inside 5 ft, no two-hand when not mounted | `equip.json` (`lance`) | no doc | low |
| Damage with no dice (blowgun "1") | built | `equip.json` (`blowgun`: `1+2`) | - | - |
| Improvised weapons, silvered / adamantine, magic +N | absent | no field on `Weapon` (`srd51/schema.go:259`) | no doc | medium (magic +1 is common loot, see part 3) |

### 2.2 Armour and shields

| Piece | State | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- |
| Base AC, DEX cap of medium armour, no DEX for heavy | built | `rules/armor.go:23-72`; checked on all 12 armours with `equip.json` | - | - |
| Shield +2 | built | `rules/armor.go:75-80` | - | - |
| Strength requirement (chain mail 13, splint, plate 15) | built: speed drops 10 ft below the requirement, dwarves ignore it, and a hint says so | `rules/hitpoints.go:77-78`, `rules/armor.go:46-49`; `equip.json` (plate at STR 8: speed 20; dwarf: 25) | - | - |
| Stealth disadvantage (padded, ring, scale, half plate, chain mail, splint, plate) | reminder: a sheet hint, no roll mode | `rules/armor.go:40-44` | `rules.md:353` | low |
| Wearing armour without the proficiency | reminder: an Issue text only; disadvantage is not applied and spellcasting is not blocked | `rules/armor.go:38`, `:79` | no doc | medium |
| Donning and doffing times (1 to 10 minutes) | absent | no field on `Armor` | no doc | low |
| Mage Armor (13 + DEX) and other AC from spells | absent on the sheet; Shield is the exception (+5 AC in combat, `combat_reactions.go`) | `rules/armor.go:18-20` comment: "Spells such as Mage Armor or Shield are not counted" | same comment | medium |
| Shield spell | built | `play/combat_reactions.go:31`, `UseReaction` `:169` | - | - |

### 2.3 Adventuring gear and consumables with a rule

| Item | State | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- |
| Torch, candle, lamp, hooded lantern (light radius) | partial: a light preset the master or a character can carry on the map and in the fog of war; burn time and oil are text | `rules/lights.go:12-30`, `effects/lights.json`, `maps/carriedlight.go:25` | the bullseye lantern is left out on purpose (`rules/lights.go:15`) | medium |
| Healer's kit (10 uses, stabilises) | absent: not content; the stabilising rule is not a button (Spare the Dying is a spell) | importer drop `srdimport/main.go:914` | no doc | medium |
| Potion of healing (and every potion) | reminder: a catalogue entry (see part 3); nothing drinks it | `rules/magicitems.go:29` list; no `UseItem` RPC in `proto/` | `docs/product/rules.md:353`: "drinking a potion is a bonus action" is a house rule the app does not enforce | high |
| Holy water, acid, alchemist's fire, oil flask (ranged attack, damage, fire on a square) | absent | not content | no doc | medium |
| Caltrops, ball bearings (area, save, speed or prone) | absent | not content | no doc | low |
| Rope, grappling hook, pitons, crowbar, lock, manacles, ladder | absent (a free-text line on the sheet) | `characters.proto:1150` | no doc | low |
| Spell components and spellcasting focus | absent: the material text is shown; no component, no cost | `rules/spelldetails.go` `MaterialText` | no doc | low |
| Tools (31: artisan tools, instruments, dice, cards, thieves' tools) | reminder: proficiency choices and a list on the sheet; no check, no disarm, no forgery uses a tool | `rules/srd51/effects/races.json` (`trait:tool-proficiency`), no `tool` in `play/traps*.go`, `play/scene.go` | no doc | medium (thieves' tools on traps and locks) |
| Mounts and vehicles (horse, warhorse, cart, rowboat, ship) | absent: not content; only the "land vehicles / water vehicles" proficiencies exist | `data/proficiencies.json:339`, `:907`; importer drop | no doc | low |
| Coins | partial: five coin counts per sheet, edited by hand, 0 to 1,000,000; treasure on the map is shown to the master but no call moves it to a sheet | `characters.proto:1159`, `characters/sheet.go:242-255`, `maps/treasure.go`; no claim / loot RPC in `proto/` | no doc | medium |
| Weight and encumbrance, carrying capacity | absent: no weight in the data, no weight on a sheet line | `characters.proto:1150` | no doc | low |

Counts of the rows above. Weapon properties and rules (14 rows): built 4 (finesse, monk, light, reach), partial 3
(versatile, thrown, range), reminder 1 (heavy), absent 6 (two-handed, ammunition, loading, net, lance, magic / improvised). Armour
and shield (8 rows): built 4, reminder 2 (stealth, proficiency), absent 2 (donning, spell AC). Gear and
consumables (11 rows): partial 2 (lights, coins), reminder 2 (potions, tools), absent 7. Every one of the 37 weapons, 12
armours and the shield does produce its attack line or its AC; no tool does anything but appear on the sheet.

## 3. Magic items (`data/magic-items.json`: 362 entries)

**Short answer: the app applies the effect of none of them.** Magic items are content for the **treasure generator** and
its item card; a character cannot hold one as content (a sheet's equipment is free text, `characters.proto:1150`), and no
rule reads an item key during a cast, an attack, a derive or a combat. Evidence: the only code that names an
`item:` key outside the importer is the treasure generator (`rules/treasure_generate.go:40`) and the consumable list
(`effects/consumables.json`); the only web code that reads a magic item is `web/src/app/pages/treasure/` (item card:
rarity, value, attunement tag, English text, `item-sheet.ts`). Doc: "enter `rules` as data, without effects"
(`docs/architecture.md:754`).

### 3.1 What the 362 entries are (`magic-counts.json`, `scripts/magic-counts.py`)

| Cut | Count |
| --- | --- |
| Category: wondrous item 177, potion 40, ring 36, weapon 30, armour 29, wand 16, staff 12, scroll 11, rod 6, ammunition 5 | 362 |
| Rarity: rare 119, uncommon 94, very rare 90, legendary 43, varies (families) 11, common 4, artifact 1 | 362 |
| Families (Armor +N, Weapon +N, Potion of healing, Spell scroll, Belt / Potion of giant strength...) and their variants | 21 + 123 |
| Require attunement (the header says so) | 175 (29 name a class or kind of wielder) |
| Consumable (potions, scrolls, single-use items of `effects/consumables.json`) | 88 |
| Text mentions charges / regains them at dawn | 53 / 41 |
| Text gives a +N bonus to attack, damage, AC or saves | 59 |
| Text grants a damage resistance | 66 |
| Text casts a spell or lets the wearer cast | 88 |
| Text sets or raises an ability score | 32 |
| Text changes a speed | 23 |

(The last six rows count by keywords in the SRD text, so they are close, not exact; the raw per-item flags are in
`magic-items-by-effect.json`.)

### 3.2 State per kind of effect

| Kind | State | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- |
| Catalogue, English text, Portuguese name, rarity, families / variants | built | `rules/magicitems.go:248-273`, `names_pt.json` | - | - |
| Treasure drawn by rarity and band, value in gp (SRD 5.2.1 table), halved for consumables | built | `rules/treasure_generate.go:205-250`, `maps/treasure.go:203-205`, `effects/magic_item_values.json` | the values come from SRD 5.2.1 on purpose (`architecture.md:2861`) | - |
| Item card for the master / players (rarity, attunement tag, value, text) | reminder | `web/src/app/pages/treasure/item-sheet/item-sheet.ts`, `core/treasure/treasure-format.ts:140-150` | `architecture.md:754` "without effects" | medium |
| Attunement flag (175 items) and who may attune | reminder: shown as a tag ("Exige sintonização") | `rules/magicitems.go:45-49`, `treasure-format.ts:145` | `architecture.md:760` (the flag is data) | medium |
| Attunement limit of 3 | absent: there is no attuned list on a sheet, so nothing counts to 3 | no `attune` in `characters/`, `proto/meurpg/characters/` | no doc | medium |
| Charges and dawn recharge (53 items) | absent: class features have `resource` effects (uses, short / long rest, `rules/effects.go:68`); no item does | no item key in `effects/*.json` except `consumables.json` | no doc | medium |
| +1 / +2 / +3 weapons and armour (Armor +1 to +3, Weapon +1 to +3, Ammunition +1 to +3, and the named ones such as Dwarven plate or Holy avenger | absent: no field on a weapon line or on AC | `rules/attacks.go:36`, `rules/armor.go:23` read only `Build.Weapons` / `Build.Armor` content keys of `data/equipment.json` | no doc | high |
| Ring of protection, Cloak of protection (+1 AC and saves), Bracers of defense, Ring of resistance (10), Boots / Cloak / Ring that change speed or rolls | absent | same | no doc | high |
| Ability-score items (Belt of giant strength x7, Gauntlets of ogre power, Headband of intellect, Amulet of health, tomes and manuals, 32 by keyword) | reminder: the player or master types the bonus by hand into the sheet's manual ability bonuses (-10 to +10, shown as "manual") | `rules/api.go:129-132`, `characters.proto` `extra_ability_bonuses` comment ("an Ability Score Improvement or a magic item") | by design: the manual field | medium |
| Potions (40) and scrolls (11), including Potion of healing (4 variants) and Spell scroll (levels 0 to 9) | reminder: nothing drinks, reads or casts them; a Spell scroll has no spell attached | `rules/magicitems.go:56-58`, `treasure_generate.go:243`; no item RPC in `proto/` | `docs/product/rules.md:353` (a potion as a bonus action is a house-rule reminder) | high (Potion of healing is the commonest loot) |
| Item bonuses to spell attack / save DC, wands of spells (88 cast a spell) | absent: a wand cannot cast, because the cast reads only the caster's own prepared spells (`rules/combat/turn.go:458`) | `characters/combatspells.go:30` | no doc | medium |
| Items with their own mechanics (Bag of holding, Immovable rod, Portable hole, Cube of force, Deck of many things...) | reminder: text only | - | no doc | low |

### 3.3 Items whose effect the app applies

None. The honest list is empty: of 362 entries, 0 change a derived number, an attack line, a damage roll, a save, a
resource or a combat state. The closest are the 32 ability-score items, which a human can enter by hand
(partial by hand, not by the app), and the treasure generator, which hands the master a Portuguese name and a value.

What a table would lack the first time loot is found: a place to put it (an item bin on the sheet), the +1 sword in the
attack line, attunement slots, and a Potion of healing that heals.

## 4. Monsters (`data/monsters.json`: 334 stat blocks)

**How a monster fights.** A monster enters a combat as an NPC made from its stat block
(`play/combat_monsters.go:206` calls `characters/combatmonsters.go:88` `MonsterNpc`, which calls `npcSheetFromCreature`,
`characters/npcfromcreature.go:109`). That NPC has a **basic sheet**: AC, hit points (rolled or average), speed, ability
scores, a challenge rating and XP, **up to three attacks** (each one a to-hit and **one damage die**, with a damage type from
the 13-type enum), and a link to the stat block for its saves, its Multiattack count, its plain resistances and its senses.
Everything else of the stat block is English text the master reads (`rules/creatures.go:265`; the stat block panel).
I ran `npcSheetFromCreature` over all 334 creatures (`overlay/zz_npc_dump_test.go`, output `npc-sheets.json`) and
simulated the rest with `scripts/monster-counts.py` (`monsters-counts.json`, `monsters-features.json`).

### 4.1 What a stat block gives the combat, per kind of feature

| Kind of feature | Monsters | State | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- | --- |
| AC, hit points (average or rolled), speed, abilities, initiative bonus, passive Perception, CR, XP | 334 | built | `characters/npcfromcreature.go:109-160`, `play/combat_monsters.go` (rolls the HP), `rules/creatures.go:501` | - | - |
| Attack actions (to-hit, first damage part, reach / range) | 330 with an attack action (534 attack actions) | partial: 504 of the 534 reach the sheet (the sheet holds 3); **19 monsters have no attack at all on the NPC** because their damage is a flat "1" (badger, bat, cat, crab, flying snake, hawk, homunculus, lizard, octopus, owl, poisonous snake, quipper, rat, raven, rug of smothering, scorpion, spider, sprite, weasel); 28 lose at least one attack; 4 have more than 3 (lizardfolk, pit fiend, tarrasque, weretiger hybrid) | `characters/npcfromcreature.go:105` (`maxNpcAttacks = 3`), loop `:138-160`, `basicAttackOf` `:181-195` (die must be d4 to d12, 1 to 20 dice) | "the basic sheet holds three" (`npcfromcreature.go:104`) | high (the 19 cannot attack) |
| Second damage part of an attack (a dragon's bite "+2d6 fire", a wolf's none) | 66 attacks | reminder: appended as text to the NPC description, 57 NPCs | `characters/npcfromcreature.go:137-139`, `:165` `extraDamage` | comment at `:137-139`: "the saving throw or rider of an action stays in the SRD stat block" | high |
| Multiattack | 148 monsters | partial: the Attack action allows N attacks (the SRD count, corrected for 5 stat blocks); the mix is the master's; 30 Multiattacks include an ability or a spell (dragons' Frightful Presence, Mage...) that is not counted | `rules/creatures.go:571-577`, `characters/combatturn.go:75-78`, `effects/corrections.json` (`attacks_per_action`) | `docs/product/rules.md:417` ("the app stores N attacks ... and the master plays the mix") | medium |
| Riders on a hit: a condition (grappled 31 monsters, restrained 32, frightened 29, poisoned 23, paralyzed 17, prone 28, blinded 13, charmed 7, stunned 2...) or a curse / disease | 129 monsters mention one in an action | reminder: in the action text only (a ghoul's paralysis, an ogre's grab, a spider's poison) | `rules/api.go:808` (`Attack.Notes`: "The engine rolls the to-hit and the first damage part; the master reads the rest here"); conditions are labels (`play/combat_conditions.go:14`) | `rules.md:329` | high |
| Saving-throw actions with no attack (breaths, gazes, auras, Frightful Presence) | 86 monsters, 134 actions | reminder: listed in `SaveActions` on the stat block with ability, DC and "half / none"; no combat call rolls them (nothing in `play/` reads `SaveActions`; the web does not read it either) | `rules/api.go:817`, `rules/creatures.go:565`, `characters/derived.go:142`; `grep SaveActions play/ web/` finds no use | `rules.md:329` | high (every dragon, every spider's web, ghoul, gelatinous cube) |
| A save riding on an attack (poison Con save on a bite, Str save to stay up) | 68 monsters, 72 actions | reminder: same list | same | `rules.md:329` | high |
| Recharge abilities ("Recharge 5-6", breath weapons) | 71 monsters, 112 actions (72 are breath weapons) | absent: no recharge die, no used / ready flag for a monster action; the usage string is text | `rules/creatures.go:311-312` (`Usage` is text); no `Recharge` outside class resources (`characters/derived.go:138`) | no doc | high |
| N/Day abilities | 14 monsters | absent | same | no doc | medium |
| Legendary actions | 32 monsters, 99 entries | absent: shown as text; no legendary pool, no actions at the end of another turn | `characters/creatures.go:205`, nothing in `play/` | no doc | high (every boss) |
| Lair actions, regional effects | 0 in the data (the 8 text mentions are the SRD's intro to a stat block) | absent | `data/monsters.json` has no such field | no doc | medium |
| Reactions | 12 monsters (Parry x6, Split x2, Shield, Rock Catching, Shriek, Unnerving Mask) | absent: the app's reaction is the opportunity attack (built) and Shield for a player's character; a monster's Parry is not offered | `play/combat_reactions.go:38` (`target.Kind != kindPlayer` returns no Shield), `play/combat_opportunity.go` for the attack | `architecture.md:821` (reaction spells), no doc for monsters | medium |
| Opportunity attack by a monster | all with a melee attack | built | `play/combat_opportunity.go:62-95` | - | - |
| Special traits (279 monsters have at least one, 152 distinct names) | 279 | reminder: shown in the stat block as English text | `rules/creatures.go:423`, `:585` (`Features`) | `architecture.md:775` ("the sheets' text stays in English") | see below |
| - Pack Tactics (advantage when an ally is adjacent) | 17 | reminder (there is no advantage anywhere: shared cause C2 in section 7) | `rules.md:353` | `rules.md:353` | high |
| - Magic Resistance (advantage on saves vs spells) | 31 | reminder: the server rolls a monster's save with no advantage | `play/combat_spells.go:636-668` | `rules.md:353` | high |
| - Legendary Resistance (3 / day, turn a failed save into a success) | 25 | absent: the save outcome is final; no counter | same | no doc | high |
| - Regeneration (start of turn) | 7 (troll, oni, shield guardian, 3 vampire forms, vampire bat) | absent: no start-of-turn hook | `play/combat_turn.go:109`, `play/combat.go:744` | `rules.md:329` | high (trolls) |
| - Undead Fortitude (Con save at 0 HP, DC 5 + damage) | 2 (zombie, ogre zombie) | absent: an NPC at 0 HP is defeated at once (`play/combat_actions.go:1151`) | no doc | medium |
| - Rejuvenation, Death Burst, Relentless, Siege Monster, Sunlight Sensitivity, Charge / Pounce / Trampling Charge, Shapechanger, Amphibious, Keen senses, Magic Weapons, Swarm | 4 / 5 / 5 / 4 / 8 / 27 / 23 / 45 / 57 / 12 / 10 | reminder / absent: text only | same | no doc | medium |
| Innate Spellcasting and Spellcasting (an NPC Mage, Priest, Lich, Archmage, Drow, Couatl...) | 24 + 12 = 36 monsters | absent: "A creature casts no spells here" (`rules/creatures.go:499`); the basic NPC sheet has no spell list, so the cast options of an NPC made from a stat block are empty | `rules/creatures.go:499`, `characters/combatturn.go:60-90`, `play/combat_spells.go:62` `castableOf` | no doc | high |
| Damage resistances, immunities, vulnerabilities (plain: "fire", "poison") | 58 / 120 / 14 monsters | built: applied when the damage lands on the NPC (half, none, double) | `play/combat_actions.go:1126`, `rules/combat/damagetype.go:18`, `characters/charactercreatures_roster.go:536-570` | `rules.md:56` | - |
| Damage modifiers with a condition ("nonmagical attacks", "silvered") | 70 monsters (44 resist, 26 immune, 1 vulnerable) | reminder: left to the master, who changes the amount that lands | `characters/charactercreatures_roster.go:530-533` ("Only the plain entries count") | same comment | medium (ghosts, werewolves, golems, demons, devils) |
| Condition immunities | 92 monsters | reminder: shown; never checked. Sleep works on a zombie, a master can mark "charmed" on an elf-proof creature | `grep ConditionImmunities play/` finds no use; `rules/creatures.go:106` only validates the keys | no doc | medium |
| Senses: darkvision (181), blindsight (83), truesight (13) | 277 | built for the fog of war and for noticing traps (a creature's senses and passive Perception) | `characters/partyvision.go:63-83`, `play/combat_fog.go:276-282`, `play/combat_traps.go:458` | - | - |
| Senses: tremorsense | 6 | absent: not read | `characters/partyvision.go:66-73` (three senses only) | no doc | low |
| Skills, saves (94 monsters have listed saves) | 94 | built for the save a spell asks (`CombatSave`); skills shown | `characters/combatspells.go:150` | - | - |
| Wild Shape and summoned beasts (not "monsters in a fight", for contrast) | the 334 are the beast pool | built: a druid's beast form and the three summon spells use `MonsterDerived` directly, with all attacks | `rules/wildshape.go:97`, `rules/summon.go` | - | - |

Rows: 28. Built 6 (stat basics, opportunity attack, plain damage modifiers, darkvision / blindsight / truesight, saves and
skills, Wild Shape and summons), partial 2 (attack lines, Multiattack count), reminder 9, mixed reminder / absent 1 (the minor traits),
absent 10 (recharge, N/Day, legendary actions, lair actions, reactions, Legendary Resistance, Regeneration, Undead Fortitude,
spellcasting, tremorsense).

### 4.2 Answers to the questions in the brief

- **Can an NPC Mage cast its spells?** No. The stat block lists the spells in text (36 monsters). A master who wants
  one builds a full-sheet NPC (ENEMY or BOSS, `characters/npcfromcreature.go:41-43`), which casts like a character.
- **Does a ghoul's claw paralyse?** The attack and 2d4 are rolled; the Con save and the paralysis are text.
- **Does a grapple on a hit grapple?** No: 31 monsters (giant constrictor snake, owlbear, kraken...). The master marks
  "Agarrado" and the target's speed becomes 0 (`play/combat_move.go:144`), which is the only automated part of the condition.
- **A dragon's breath?** The breath is on the stat block with its DC and the recharge text. The master rolls the damage by hand;
  nothing offers a "use / ready" state.
- **Pack Tactics, Parry, Magic Resistance?** Text. There is no advantage or disadvantage mechanism at all.
- **Regeneration, Undead Fortitude?** Text; no start-of-turn hook and no 0-HP check for an NPC.

## 5. Races and subraces (9 races, 4 subraces, 38 traits)

Method: `overlay/zz_races_dump_test.go` derives a level 3 Fighter of every race and subrace and writes what the sheet
shows (`races.json`: speed, senses, skill proficiencies, hints, resources, actions, languages, issues). A trait's state is
what that sheet does with it, not what `effects/races.json` says it is.

**Race-level numbers (built).** Ability bonuses (`rules/abilities.go:38-70`), size and speed (dwarf and halfling 25, others 30), languages,
subrace bonuses, weapon / tool proficiencies written in `data/traits.json` (dwarven combat training, elf weapon training,
tinker), and the half-elf's two free +1 as a prompt to type them into the manual bonuses (`rules/abilities.go:381-407`). The dwarf keeps
its speed in heavy armour (`rules/hitpoints.go:77`). Armour and heavy weapons for Small races are hints only (part 2).

### 5.1 Traits, one row each

| Trait (races / subraces) | State | What the sheet does | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- | --- |
| Darkvision 60 ft (dwarf, elf, gnome, half-elf, half-orc, tiefling) | built | derived sense; feeds the fog of war and trap noticing (dim light is read as bright, darkness as grey) | `effects/races.json:3`, `rules/hitpoints.go:81-105`, `rules/vision/vision.go:258-270`, `characters/partyvision.go:63` | - | - |
| Dwarven Toughness (hill dwarf) | built | +1 max HP per level (+3 at level 3, checked in `races.json`) | `effects/races.json:19` | - | - |
| Keen Senses (elf) | built | Perception proficiency | `data/traits.json` (`proficiencies`), `races.json` dump: elf has `skill:perception` | - | - |
| Menacing (half-orc) | built | Intimidation proficiency | same | - | - |
| Dwarven Combat Training, Elf Weapon Training | built | four weapon proficiencies each | `data/traits.json` | - | - |
| Tool Proficiency (dwarf), Extra Language (high elf), High Elf Cantrip, Skill Versatility (half-elf), Draconic Ancestry | built | a choice the builder asks for; missing choices are listed as "Faltam N perícias" | `effects/races.json:13,34,37,52,102` | - | - |
| Tinker (rock gnome) | partial | Tinker's tools proficiency is real; building the devices (clockwork toy, fire starter, music box) is text | `effects/races.json:99`, `data/traits.json` | no doc | low |
| Breath Weapon (dragonborn) | partial | a "Sua vez" action that spends 1 use (short rest) and a hint "CD 12; 2d6, 3d6 at 6th..., half on a save"; **no save is rolled, no damage is rolled, no area** | `effects/races.json:85-89`, `play/combat_actions.go:1770-1776` (`spendResource`) | `rules.md:329` | high (dragonborn) |
| Relentless Endurance (half-orc) | partial | a once-per-long-rest counter and a hint; nothing fires when the character drops to 0 HP | `effects/races.json:108-111`; nothing in `play/combat_death.go` or `play/combat_actions.go` reads `relentless_endurance` | `rules.md:329` | high |
| Dwarven Resilience (dwarf): advantage on saves vs poison, poison resistance | reminder | two hints; no advantage mechanism, and damage to a player waits for the master | `effects/races.json:6-9`, `rules.md:56` | `rules.md:353`, `:56` | medium |
| Fey Ancestry (elf, half-elf): advantage vs charm, no magical sleep | reminder | hint; Sleep still works on an elf (no check of the target's traits) | `effects/races.json:25-27`, `effects/spells.json` `spell:sleep` | `rules.md:353` | medium |
| Brave (halfling) | reminder | hint "Vantagem em testes ... amedrontado" | `effects/races.json:43` | `rules.md:353` | low |
| Gnome Cunning (gnome): advantage on INT / WIS / CHA saves vs magic | reminder | hint; the server's save roll for a player's character in a spell has no advantage | `effects/races.json:93`, `play/combat_spells.go:636-668` | `rules.md:353` | medium |
| Lucky (halfling): reroll a natural 1 | reminder | hint; the d20 is rolled once (`d20`, `play/combat_spells.go:582`), no reroll button | `effects/races.json:40` | no doc | medium |
| Stonecunning (dwarf), Artificer's Lore (rock gnome): double proficiency on a subject | reminder | hint with the computed bonus ("História +4") | `effects/races.json:16,96` | tags are "never applied" (`rules/effects.go:35`) | low |
| Hellish Resistance (tiefling), Damage Resistance (dragonborn) and the 10 Draconic Ancestry colours (breath shape, DC and resistance) | reminder | text hints; a resistance is never applied to damage on a player's character | `effects/races.json:55-84,90,115` | `rules.md:56` ("Rage and other resistances are notes the master applies") | medium |
| Infernal Legacy (tiefling): Thaumaturgy, Hellish Rebuke 1/day at 3, Darkness 1/day at 5 | reminder | the spells are only allowed on the list; they are not added to the sheet, not gated by level and not counted per day; Hellish Rebuke is a reaction, which cannot be cast (part 1) | `effects/races.json:118`, `rules/spellcasting.go:164-166` (granted spells only widen the "on list" check); the tiefling Fighter dump has no spells | no doc | medium |
| Savage Attacks (half-orc): one extra weapon die on a critical hit | reminder | no text on the sheet (an empty note); the critical rule doubles the dice only | `effects/races.json:112`, `play/combat_actions.go` (`openHit`, `play/combat_reactions.go:66`) | no doc | medium |
| Trance, Halfling Nimbleness, Naturally Stealthy | reminder | the SRD feature text, nothing else | `effects/races.json:28,46,49` | no doc | low |

Totals over the 38 traits: **built 11, partial 3, reminder 24, absent 0**. Every trait at least appears as feature text.
Counted by what a table notices in play: of the 12 traits that touch a roll (advantage, reroll, extra die, resistance) none
is applied, because the combat has no roll modes (shared cause C2 in section 7).

## 6. Backgrounds and feats

### 6.1 Backgrounds

| Piece | State | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- |
| The SRD's one background, Acolyte: two skills, two languages to choose, the feature "Shelter of the Faithful" | built for skills and languages; the feature is text | `data/backgrounds.json`, `effects/backgrounds.json` (a note with no effect) | - | - |
| Acolyte equipment, personality traits, ideals, bonds, flaws tables | absent: the SRD data does not carry them (`BackgroundEquipmentPT` is empty for an SRD background) | `rules/api.go:558-560` | the comment itself | low (the sheet has free text boxes for the four personality fields: `characters.proto:1311-1330`) |
| "Outro" (custom) background: name, two skills, two tools or languages, a written feature, equipment text | built, following SRD "Customizing a Background"; a missing part is a notice, not an error | `rules/api.go:100-112`, `proto characters.proto:1109-1125`, `docs/architecture.md:1131` | by design | - |
| Table backgrounds (the master writes one) | built | `table_content.proto:288-298` (`TableBackground`), `web/.../background-editor` | - | - |
| A background feature with a mechanical effect (e.g. contacts, a skill bonus) | partial: a table background's feature may carry the closed effects (proficiency, sense, note...); the SRD's own is a note | `rules/tablemenu.go:120-170` effect menu | - | low |

### 6.2 Feats

| Piece | State | Evidence | Deliberate? | Impact |
| --- | --- | --- | --- | --- |
| The SRD's feat (Grappler) in the content | absent: the importer reads features, not feats | `backend/cmd/srdimport/main.go:150` (converters: classes, levels, subclasses, features, backgrounds, ... no feats), `data/` has no feats file, `grep -ri grappler` finds only the grappled condition and monster text | `docs/product/rules.md:47` ("SRD 5.1 has no feats") | medium |
| A feat field on the sheet (`FullSheet`, `Build`) | absent | `characters.proto:943-1010`, `rules/api.go:90-160` | same | high for a table that wants feats |
| Level-up choice "feat instead of an ability score improvement" | absent: only +2 / +1 +1 (none above 20) goes to `extra_ability_bonuses` | `rules/levelup.go:111-113` ("The SRD 5.1 has no feats"), `:335`, `:725` | `rules.md:47` | high |
| A table (house) feat: a content kind the master can write | absent: the kinds are class, subclass, race, subrace, background, spell | `proto/meurpg/rules/v1/table_content.proto:31-46` | no doc | high |
| What a feat would need that already exists | the closed effect vocabulary can express Tough (`hp.max`), Alert (`initiative`), Mobile (`speed.walk`), Resilient / Skilled / Linguist / Keen Mind style proficiencies, Observant (passive), and notes for the rest | `rules/effects.go:135`, `rules/tablemenu.go:120-170` | - | - |

**Workaround a table has today.** Take the ability score improvement and write the feat in the character's free-text notes
or in a custom background / table class feature; or make a table subclass whose feature carries the effects. Nothing lets a
player pick a feat at level-up. A table that wanted feats would lack: (1) a feats catalogue (the SRD gives Grappler only,
the PHB feats are not shippable, so the table would write its own), (2) a `feat` content kind in the table editor with
prerequisites, (3) a "feat or ASI" choice in `GetLevelUpOptions` / `LevelUpCharacter`, (4) a place on the sheet and in the
derive for the feat's effects, and (5) the combat hooks that make the popular ones matter (Great Weapon Master and Sharpshooter
need a to-hit penalty toggle, Sentinel and Polearm Master need reactions, Lucky and Alert need roll modes), which today do not exist.

## 7. Shared causes

One missing mechanism, many rows. First the five engine facts every part points at, then the causes.

### 7.1 The engine, in five facts that decide most rows

These are the mechanisms the rest of the report keeps pointing at. Each was read in code.

1. **A cast computes five things and nothing else.** `CastSpell` (`backend/internal/play/combat_spells.go:192`) spends
   the slot and the action, sets concentration, and then, for each target, runs one of: a spell attack roll with the
   first damage type (`:598`), a saving throw with full/half/no damage (`:636`), Magic Missile's darts (`:670`), a
   pending heal (`:560`+), or one of 8 hit-point effects (`play/combat_spells_hp.go:91`, data in
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

### 7.2 Causes

| ID | Missing mechanism | What it explains | Rows / counts | Evidence |
| --- | --- | --- | --- | --- |
| C1 | **Conditions are labels with no effect.** Only speed 0 and "cannot react" read them; no roll, save or attack reads a condition; immunities are never checked | spells that impose a condition (38 flagged, plus Hold Person's paralysis...), monsters' riders (129), condition immunities (92), Sleep ignoring immunities | spells 1.3 "condition"; monsters 4.1 | `play/combat_conditions.go:14-18`, `play/combat_move.go:144`, `play/combat_opportunity.go:42` |
| C2 | **No roll modes (advantage / disadvantage) in the combat, the cast or the check.** A roll is one d20; "Hints" carry the rest as text | 12 race traits, Pack Tactics, Magic Resistance, flanking, long range, heavy weapons, stealth armour, Bless / Bane / Guidance as bonus dice, guiding bolt, faerie fire, true strike, Vicious Mockery | races 5.1, monsters 4.1, equipment 2.1 and 2.2 | `play/combat_spells.go:582`, `combat.proto:2539`, `rules/effects.go:35` (tagged roll modes are "never applied"), `docs/product/rules.md:353` |
| C3 | **No turn clock.** Start / end of turn does nothing; no durations, no repeat saves, no ongoing damage, no recharge dice, no regeneration, no legendary-action pool, no concentration timer | 136 spells with a duration, 33 repeat saves, 22 ongoing damage, 118 concentration spells, recharge (71 monsters), Regeneration, legendary actions (32), Undead Fortitude, Relentless Endurance | spells 1.3, monsters 4.1, races 5.1 | `play/combat.go:744` `EndTurn`, `play/combat_turn.go:109` `startTurn`, `docs/product/rules.md:329` |
| C4 | **No temporary modifiers on a combatant.** Only Shield's `AcBonus`, temporary hit points and a raised maximum exist; no timed +AC, +attack, +save, speed, resistance, extra damage dice | 53 stat-change spells; items; hunters mark, hex style riders | spells 1.3 "stat_change" | `play/combat_reactions.go` (`AcBonus`), `play/combat_spells_hp.go:308-336` |
| C5 | **Nothing a spell leaves on the map.** Area shapes are data; there are no zones, walls, clouds, lights or summoned objects as map pieces | 57 zone / object spells, 68 with movement, 94 area spells | spells 1.3 "zone" | `rules/spelldetails.go:140`, `maps/` has traps and lights as separate master tools |
| C6 | **No cast flow outside a combat, for rituals or for a long casting time.** `CastSpell` needs an encounter; `CastSummon` handles three spells | 57 long spells, 29 rituals (Find Familiar only), healing between fights | spells 1.1 | `play/combat_spells.go:192`, `rules/combat/turn.go:478`, `play/creature_cast.go:170` |
| C7 | **No item model on a sheet.** Equipment is free text; weapons / armour are content keys without bonuses; magic items are a catalogue; no charges, attunement list, weight, ammunition, consumption | magic items (362), gear, ammunition, potions, coins, +N weapons | parts 2 and 3 | `characters.proto:1150`, `rules/attacks.go:36`, `docs/architecture.md:754` |
| C8 | **A monster fights as a basic NPC sheet.** Three attacks, one damage die each, no save actions, no recharge, no traits, no spells, no reactions, no legendary actions | 19 attack-less monsters, 134 save actions, 112 recharge actions, 32 legendary monsters, 36 casters, 12 reaction monsters | part 4 | `characters/npcfromcreature.go:105-165`, `rules/creatures.go:499` |
| C9 | **Reactions are two special cases.** Shield and the opportunity attack are coded; the rest of the reaction family has no trigger / prompt | Counterspell, Hellish Rebuke, Feather Fall, Parry (6 monsters), Rock Catching, Shield Guardian | spells 1.1, monsters 4.1 | `play/combat_reactions.go:31`, `rules/combat/turn.go:482` |
| C10 | **Damage to a player's character is applied by the master**, so resistances (Rage, Hellish Resistance, Dwarven Resilience, dragonborn ancestry) are notes | 4 race traits and class features | races 5.1 | `docs/product/rules.md:56`, `play/combat_actions.go:1126-1155` |
| C11 | **The importer's scope.** Equipment categories other than weapon, armour and tools, feats, lair actions are not imported | gear, mounts, vehicles, ammunition, Grappler | parts 2 and 6 | `backend/cmd/srdimport/main.go:914`, `:150` |
| C12 | **Targets are picked by hand.** The server limits the number (10) and the range, but does not work out who stands in a cone, cube, sphere or line | 94 area spells | spells 1.1 | `play/combat_spells.go:47`, `rules/spelldetails.go:140` |

### 7.3 What the best-return mechanisms would unlock

- **C1 + C3 together** (a condition on a combatant with a duration and a repeat-save hook) would turn about 70 spells and most of the
  129 monster riders from reminders into rolls: Hold Person, Web, Fear, Entangle, a ghoul's claw.
- **C2** (an advantage / disadvantage flag on a roll, set by a condition or by hand) unlocks Pack Tactics, Magic Resistance and the
  12 race traits that are hints today. Bless / Bane / Guidance as extra dice need **C4**.
- **C8** (let a stat block keep all of its attacks and save actions, plus a "used / ready" state) is the most visible one at the table:
  dragons, spiders, ghouls, and every bat and rat.
- **C7** is the largest. A cheap first step exists: a list of item keys on a sheet whose +N and protection items are `modifier`
  effects, which the effect vocabulary can already express (`rules/effects.go:135`).

## Appendix: how this was made and how to repeat it

Everything lives in `review/coverage-content/`. No file of the application was changed.

| What | Where |
| --- | --- |
| Overlay tests (Go, run with `go test -overlay`, kept outside the packages) | `overlay/zz_*_dump_test.go`; `overlay/run.sh <name> <TestName> [package]` writes `<name>.json` |
| Spells: what a cast reads for each of the 319 | `spells-machine.json` (`overlay/zz_spells_dump_test.go`, run with `overlay/run-spells.sh`) |
| Spells: tags from the description (haiku sweep, 8 batches of 40, two agents at a time) | prompt `scripts/spell-tag-prompt.md`, inputs `raw/in/`, outputs `raw/spell-tags-b*.jsonl` |
| Spells: state, impact, groups | `scripts/spell-state.py` (rules and hand overrides in `scripts/spell-overrides.json`), `scripts/spell-groups.py`; results `spells-states.json`, `spells-states.csv`, `spells-groups.md` |
| Spells: sample of the content side of a cast | `spells-cast-sample.txt` (`overlay/zz_sample_dump_test.go`) |
| Equipment | `equip.json` (`overlay/zz_equip_dump_test.go`: 37 weapons and 13 armour pieces through `Derive`) |
| Races | `races.json` (`overlay/zz_races_dump_test.go`) |
| Magic items | `scripts/magic-counts.py`, `magic-counts.json`, `magic-items-by-effect.json` |
| Monsters | `scripts/monster-counts.py`, `monsters-counts.json`, `monsters-features.json`, `npc-sheets.json` (`overlay/zz_npc_dump_test.go`: `npcSheetFromCreature` over all 334) |
| Report | `report-src/*.md` assembled by `scripts/build-report.py` into `../coverage-content.md` |

To repeat: `cd backend && ../review/coverage-content/overlay/run-spells.sh`, then `run.sh races TestCoverageDumpRaces`, `run.sh equip TestCoverageDumpEquip`,
`run.sh sample TestCoverageSampleSpells`, `run.sh npc TestCoverageDumpNpcSheets characters`, then the Python scripts, then `build-report.py`
(the `run.sh` files write to `review/coverage-content/<name>.json`; the sample output was renamed to `spells-cast-sample.txt`).

**Limits to know.**

- The play tests that cast through the RPC (`play/combat_spells_test.go` and others) need Postgres, which this session did not have, so the
  engine side of a cast was verified by reading the code at the lines cited, and the content side by the overlay tests above. The sample file shows what the
  cast would roll for 22 spell and slot combinations.
- The haiku tags give the *text* of each spell; the *state* of every spell was decided by me from the machine view plus the rules written in
  `scripts/spell-state.py` and fixed by 16 hand overrides. The impact column of the spell table is a rule of thumb (level, kind, a list of staple spells in the script),
  not a measurement.
- Magic item rows 3.1 that count "text mentions" use keywords over the SRD text and are approximate.
- "Deliberate?" quotes a doc line only when the doc says the gap is on purpose or out of scope; a doc was never used as evidence of a state.
