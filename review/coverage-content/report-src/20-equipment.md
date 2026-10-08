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
