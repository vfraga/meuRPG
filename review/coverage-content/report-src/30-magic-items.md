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
