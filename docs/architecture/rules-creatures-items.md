# Rules module: creatures, items and presets

The SRD 5.1 creatures, the magic items and the trap and light presets enter `rules` as data, like the rest of the content.
How the snapshot files are imported is in [Rules content](../rules-content.md); this page is what the engine does with them.
The services that serve them are in [Characters](characters.md#content-service).
For the engine around them see [Rules module](rules.md).

## Creatures

`srd51.Monster` holds a creature's size, type, armor class and what gives it, hit points and dice, speeds in feet, the six abilities, save and skill bonuses, vulnerabilities, resistances, immunities, condition immunities, senses, challenge rating and XP, traits, actions (with the attack bonus, the damage parts, the save DC and the Multiattack routines), reactions and legendary actions.
The Portuguese names are `monster:<index>` in `effects/names_pt.json`; the sheets' text stays in English, like the spells'.

Three importer readings deserve a warning.
When the SRD gives a damage choice (the longsword one- or two-handed), the first option is kept and the text has the rest.
The XP of a creature is the one of its challenge rating in the SRD table, not the source's, which has four creatures at half; a creature with challenge rating 0 and no XP keeps 0.
A save that 5e-database leaves only in the text ("Força CD 11 ou cai Derrubado") is read from the action's text.

| Function (`rules.Content`) | What it does |
| --- | --- |
| `ListCreatures(filter)`, `CreatureByKey(key)` | The list and one creature's sheet with Portuguese labels. The filter has a name query (Portuguese or English, without accents), type, size, minimum and maximum CR, no flying and no swimming; the list is sorted by Portuguese name. It refuses a CR that is not in the SRD, a minimum above the maximum and a size that does not exist. |
| `MonsterDerived(key)` | The creature as a `Derived` that combat already reads: abilities and modifiers, saves and skills, armor class, hit points, the walking speed in `SpeedWalkFt` and the others (`SpeedFlyFt`, `SpeedSwimFt`, `SpeedClimbFt`, `SpeedBurrowFt`, `Hover`), senses, passive Perception, the attacks (bonus and the first damage part; the full text goes in `Attack.Notes` for the master), Multiattack as `AttacksPerAction` (the largest routine) and the save actions in `SaveActions` with their DC. No spellcasting. |
| `SummonOptions(spell, level, build)`, `CheckSummon(...)` | What a summoning spell lets the caster pick with that slot, and the check of a choice (`ErrSummonCreature`, `ErrSummonCount`, `ErrSummonOption`, `ErrSummonCircle`, `ErrNotSummonSpell`). Casting time, ritual and concentration come from the spell's data. |
| `WildShapeLimitFor(build)`, `WildShapeForms(build)`, `WildShapeAllows(build, beast)`, `WildShapeDerived(sheet, beast)`, `CastsInBeastForm(derived)` | See [Wild Shape](#wild-shape). |

A table creature (`creature:<slug>@mesa`) is an SRD creature with the master's adjustments and resolves through the same functions; the encounter tools, the summon choices and Wild Shape leave it out ([Table creatures](rules-table-content.md#table-creatures)).

**A creature's armor class** comes from the armor it uses, and the note says what it wears and the other values ("15 com Armadura Arcana").

**Multiattack corrections.**
The Multiattack count (`MonsterDerived(...).AttacksPerAction`, the same number the character's creatures use on their turn) applies to any basic sheet linked to a creature, so a monster's turn options count the attacks of the Attack action, and NPCs made by "Criar NPC" follow it.
The master plays the monsters and has the last word, so no extra attack is refused to them.
The number comes from SRD 5.1 and not from the snapshot, which gets some wrong: `effects/corrections.json` corrects it (`creature_corrections`, a closed type: a known creature and field).
The same file corrects other stat-block fields, a spell's attack, saving throw and damage by slot level, and the spells a subclass always has, all applied over the snapshot at load (see [Rules content](../rules-content.md)).
`TestMultiattackCountsFollowTheSRDText` reads the Multiattack text of every creature and checks the number the engine uses, with an explicit list of the creatures whose text it cannot read, each with the reason.

### The actions of a stat block

`rules/monsterplan.go` turns a creature's actions into a plan a combat can play: `Content.MonsterPlan(key)` returns a `MonsterPlan` (the creature's `[]ActionPlan`, built at load for the 334 creatures) and `MonsterActionPlan("<creature key>#<slug of the name>")` one `ActionPlan`, the key of its Attack ("monster:ghoul#claws") and of the creature action the master uses.
The snapshot already holds an action's attack bonus, damage parts and saving throw; what it leaves in the text (the damage a failed save deals to the target of a hit, the condition a failed save gives and its duration, the escape DC of a grapple, an area, a usage limit) the plan reads from the SRD's own words with patterns checked against every creature. What a pattern cannot read reliably stays in `Text`, which the master always reads.

An `ActionPlan` has:

- `Kind`: `attack` (an attack roll), `save` (asks the creatures it affects for a saving throw), `multiattack` (a routine of other actions, `Routines`) or `other` (no roll).
- `Attack` (bonus, melee or spell, reach and range) and `Damage` (every part of a hit, in order); `Save` (`ActionSave`: ability, DC, `OnSuccess` of `none`, `half` or `other`, the `Damage` it governs, the condition a failure gives with its `Duration` and `RepeatSave`, and `OnHit` when the save is the rider of an attack rather than asked of the creatures the action affects); `HitCondition` (a condition a hit gives with no save, with the escape DC and the largest size it holds); `Area` (cone, line, sphere, cube, cylinder or a radius around the creature, with its size); `Usage` (at will, recharge, per day, per rest; read and shown, nothing enforces it).
- `Class`, how far the plan reads the action. *Structured*: every damage part, save and condition the text states is in the plan. *Partial*: the plan has the attack, damage or save the data states, but the text carries a number or a condition the plan does not hold, so the master reads it. *Text*: nothing to roll, the text is the reminder. The counts over the 334 creatures (884 actions: 721 structured, 94 partial, 69 text, 148 with a Multiattack count) are pinned by `TestMonsterPlanEveryCreature`, so a pattern that moves an action between classes fails the test and the move is chosen; `TestMonsterPlanNamesWhatTheMasterReads` pins by name the partial and text-only actions of challenge rating 0 to 5.

Traits, reactions, legendary actions and spellcasting are not planned.

**Corrections of action-level data.**
`creature_corrections` take an optional `action` (the action's name in the stat block) beside `creature` and `field`; `action_save_success` is the one field that belongs to an action, not to the creature: it sets what the action's saving throw does on a success (`half`, `none` or `other`) when the snapshot or the text reading gets it wrong. The loader refuses an unknown action, an action with no saving throw, a value that is not one of the three, an action named on a field of the creature, and two corrections of the same field of the same action. The play flow that uses the plan is in [Attacks and damage](play/attacks-and-damage.md#a-creatures-actions-and-riders); the API sends the plan to the master as `CreatureAction` (`rules.v1`), with `kind`, `plan_class`, `save`, `area` and `text`.

## Summons

The `summon` effect is a closed type of `effects/spells.json`, next to `hp_pool`, `hp_threshold`, `zero_hp_target`, `flat_heal`, `temp_hp` and `max_hp`.
Loading refuses a creature, field, CR or attack mode that does not exist.
Either it lists the creatures (`creatures`, with `count`, `count_per_level` and what each may do, `attack`: `none`, `reaction` or `full`; `unlocks` for a feature that adds forms) or it gives CR bands of a creature type (`options` with `count` and `max_cr`, `multiplier_at_level`).

| Spell | What it summons |
| --- | --- |
| `spell:find-familiar` | The SRD's forms, one creature, a 1-hour ritual; it does not attack. The Pact of the Chain (`feature:pact-of-the-chain`) adds Imp, Pseudodragon, Quasit and Sprite and makes every warlock familiar attack, with the reaction only. |
| `spell:animate-dead` | Skeleton or Zombie: one creature, plus two per level above the 3rd. |
| `spell:conjure-animals` | Beasts only: 1 of CR 2, 2 of CR 1, 4 of CR 1/2 or 8 of CR 1/4, multiplied at the 5th, 7th and 9th level; concentration. |

Find Steed is not covered.
Whoever spends the slot and creates the creatures is the [play module](play/README.md); `rules` only says what is allowed.

## Wild Shape

The druid's `wild_shape` effect (in `effects/druid.json`, at levels 2, 4 and 8) carries `max_cr` (or `max_cr_from`), `no_fly` and `no_swim`, and a table feature can add `form_cost` and `magical_attacks`: level 2 allows CR up to 1/4 without flying or swimming, level 4 up to 1/2 without flying, level 8 up to 1.
Loading refuses a `max_cr` that is not a CR and any other field.
`WildShapeForms(build)` lists the beasts the druid may assume now; `WildShapeAllows` checks one.

`WildShapeDerived(sheet, beast)` is the druid's sheet in beast form, by the SRD:

- armor class, hit points (a separate pool: `HitPointsMax` is the beast's), speeds, Strength, Dexterity and Constitution, attacks and senses come from the beast;
- Intelligence, Wisdom and Charisma, proficiency bonus, features and resources stay the character's;
- skill and saving throw proficiencies stay and gain the beast's, and where both have the same, the larger bonus wins;
- the character's darkvision only holds if the beast also has it (the larger range wins);
- no spells: `Spellcasting`, `Spells` and `PactMagic` come out empty, unless Beast Spells (`CastsInBeastForm`) applies.

## Magic items

The SRD 5.1 magic items (MR-044) enter `rules` as data, without effects: `srd51.MagicItem` has the key `item:<index>`, the SRD name, category, rarity (`common`, `uncommon`, `rare`, `very_rare`, `legendary`, `artifact` or `varies`), attunement and by whom, variants, and the English text.
The Portuguese names are `item:<index>` in `names_pt.json`, one per item and variant.
The gold values (SRD 5.2.1 table) belong to the treasure generator, not to the items.

**Families and variants.**
The SRD lists "Armor +1, +2 or +3" once, as a family, and again as three variants.
Each is a real item with its own key: the family holds `Variants` (the keys, in SRD order) and each variant holds `VariantOf` and its own rarity.
A family whose variants differ has the rarity `varies`.
The Crystal ball is the special case: a family that is also an item (`Standalone`, a fixed list in the importer), drawn as the common Crystal ball among the very rare and its variants among the legendary.

**Attunement.**
`Attunement` is true when the SRD header says "(requires attunement)"; `AttunementBy` keeps the restriction in English ("by a paladin") and `AttunementByPT` the Portuguese label (`attunement:<restriction>` in `names_pt.json`; loading refuses a restriction without a label and the reverse).
An attunement that applies to one property only stays in the text.

**What gets drawn.**
A flat list would overweight the families, so `MagicItemUnits(rarity)` returns units: each standalone item and one unit per (family, rarity), with that rarity's variants in `Options`.
Whoever draws picks a unit and, within it, an option: the Ring of Resistance is a rare unit with ten options.
A family is never a unit in a rarity its variants do not have, except the Crystal ball.

| Function (`rules.Content`) | What it does |
| --- | --- |
| `MagicItems()` | All entries (families and variants), sorted by Portuguese name, as `MagicItemEntry`. Copies. |
| `MagicItem(key)` | One entry with the SRD text in English (`Desc`) and our translation (`DescPT`, empty while missing: `DescPTMissing()`). |
| `MagicItemUnits(rarity)` | The units a treasure may contain at that rarity, sorted by the key's Portuguese name. Copies. |
| `MagicRarities()` | The six rarities, from most common to artifact. |

**Consumables.**
`effects/consumables.json` is handwritten: the potion and scroll categories and the single-use items (ammunition, arrow of slaying, the dusts, elemental gem, the manuals and tomes that lose their magic once read, and so on).
Loading refuses a category or item that does not exist, a repeated one and one the category already covers.
The SRD 5.2.1 value table halves a consumable, except the Spell scroll, which `SpellScroll` marks; the treasure generator applies it through `Content.MagicItemValue`.

## Trap and light presets

`effects/traps.json` (the SRD's eight sample traps, in our words and with the SRD's numbers, plus the severity table and the damage per level) and `effects/lights.json` (the SRD's lights, in feet) are handwritten and checked on load.
The Portuguese names are `trap:<key>` and `light:<key>`.
`Content.TrapPresets()`, `TrapPreset(key)`, `TrapSeverities()`, `TrapDamageByLevel()` and `LightPresets()` return deep copies.

- A trap has the effect parts of the design (an attack, damage that always hits, conditions that always apply, and a save with what happens on failure and on pass), the DC to notice and to find, the trigger (`enter` or `manual`), the area (1 to 4 squares) and the targets (`area` or `manual`).
- A pit has `fall_ft` (a fall of 1d6 per 10 ft) and whoever falls ends Prone.
- The SRD gives the DC to notice only for some traps; the DC to find is ours and `find_dc_ours` marks it (`TrapPreset.FindDCIsOurs`).
- Lights are circles with `bright_ft` and `dim_ft`.
  The bullseye lantern (a cone) is left out: the app only lights circles.

`ContentService.ListTrapPresets` and `ListLightPresets` deliver them to the master's map form.
A trap or light on a map keeps its own copy of the numbers (see the [maps module](maps/README.md)).
