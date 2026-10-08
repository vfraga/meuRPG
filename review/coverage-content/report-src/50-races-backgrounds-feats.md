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
