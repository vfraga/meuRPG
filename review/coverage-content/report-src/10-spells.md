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

{{SPELL_COUNTS}}

Sample checked by running the content side of a cast in a Go test overlay (the same calls as
`characters.CombatSpell`, `characters/combatspells.go:30`): `spells-cast-sample.txt` (Fireball at 3rd and 5th level gives
8d6 and 10d6 Dex-half, Cure Wounds gives `1d8 + MOD` and `3d8 + MOD`, Hold Person gives a Wisdom save with no damage and no
effect, Web gives no save and a 2d4 table, Sleep gives the hit point pool, Hellish Rebuke is a reaction so it is refused).
The full machine view of all 319 spells is `spells-machine.json`
(`overlay/zz_spells_dump_test.go`). The play tests for the cast (`play/combat_spells_test.go`, `combat_spells_hp_test.go`)
need Postgres, so the engine side was verified by reading, not by running a cast.

### 1.3 Grouped by what is missing

{{SPELL_GROUPS}}

### 1.4 Every spell

{{SPELL_TABLE}}
