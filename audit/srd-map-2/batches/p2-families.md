# Part 2 baseline: SRD magic item families (batch p2g)

Checkout: /home/user/meuRPG-audit2 (audit/srd-map-2). Paths below are under backend/internal/ or web/src/app/.
Counts come from a text scan of data/magic-items.json (362 entries: 342 holdable, 20 families whose variants are held instead);
the family of an item is a heuristic (charges first, then curse, container, consumable, bonus, ability, resistance, sense, spell, other).

## How an SRD magic item gets on a sheet

- The one path is the sheet's "Adicionar item mágico" picker (web pages/character-sheet/magic-items/magic-item-picker-sheet.ts, rpc
  ContentService.ListMagicItems, characters/magicitemlist.go -> rules/attunement.go ListMagicItems). It searches by Portuguese or English name,
  shows families with their variants, hides the "Armor +N" and "Weapon +N" families, and adds one catalog key.
- Who: the character's owner while the sheet is unlocked (RN-01), the master at any time; not a dead character (canManageMagic,
  character-sheet.html:330). After the lock the server refuses a changed item list (rules/levelup.go:1490 sameMagicItems).
- Fields stored (rules/api.go:1037 SheetMagicItem): key, id (server gives "m<n>"), attuned (only SetAttunement changes it). Max 20 entries, copies allowed
  (checkMagicItems, rules/attunement.go:173). Server fills on read: requires_attunement, restriction, name_pt, charges_max/left, lost, recharge, effect_pt.
  No notes, quantity, worn flag, identified flag, free text or custom item. The master cannot register a custom magic item (RN-23 lists weapons, not items).
- +N armor, shield and weapons come by the piece's "⋮" (piece-bonus-sheet.ts, character-sheet-source.live.ts:125): ItemBonus{key = gear key, bonus 1..3}. The web never sets
  item_key or requires_attunement, so a +N piece is never gated by attunement from the UI.
- Treasure (maps/treasure.go, master only) rolls items and "Pôr no mapa" places them; no path gives a treasure item to a character. The item text (Desc/DescPT)
  reaches only the master (GetMagicItem, treasure page "Ver descrição") and the UseConsumable reply (no screen calls it). A player never reads an item's text.

## What exists for items at all

- Catalog and text: rules/magicitems.go (362 entries, rarity, category, attunement, variants), effects/magic_items_pt.json (Portuguese text), effects/magic_item_values.json (gp value).
- effects/item_charges.json: 51 keys, numbers only (max, start, recharge, regain, empty d20/always/none, lost, empty_high_regain). Loader rules/itemcharges.go:87.
- effects/potions.json: 14 keys (4 healing dice, 10 resistance 600 rounds). Loader rules/potions.go:50. effects/consumables.json: categories potion and scroll plus 37 single-use keys (80 holdable consumables).
- Attunement: data attunement and attunement_by (175 and 29 entries); parser rules/attunement.go:51; Portuguese labels names_pt.json "attunement:*".
- Nothing else keys on an item: combat_effects.json and damage_resistances.json hold spells, conditions and traits only (no "item:" key); the effect types (rules/effects.go:363)
  and modifier targets (:370: ac, hp.max, speed.*, attack/damage.weapon.*, carry capacity) are reached from class, race, feat, background and table content, never from an item.
  The only item-sourced effect is the potion of resistance (play/consumables.go putPotionResistance, backend only).
- Engine items: attunement (characters/attunement.go), charges and dawn (characters/itemchargeskeeper.go, dawn.go; play/charges.go), consumables (play/consumables*.go),
  +N gear (rules/itembonus.go), sheet marks magical/silvered/adamantine (rules/attacks.go:178,195).
- Web: equipment-panel (magic item rows, pills, "Usar", "Rolar a última carga"), attunement-section, piece-bonus-sheet, party-charges (master "Amanhecer"), treasure item-sheet.
- Quick test of "does the app handle this item specifically": key in item_charges.json (charges), key in potions.json (effect), SheetMagicItem.charges_max > 0, attunement_by non-empty (restriction),
  key weapon-1..3 or armor-1..3 (gear bonus, by the piece's ⋮). Any other key is a name, a rarity, an attunement pill and nothing else.

## Verdict by family (what a typical item gets today)

Scale: runs / runs-part / by-hand-said / by-hand-silent / wrong. No item family is by-hand-said: no screen shows the player an item text.

### bonus (45 by scan; +N to attacks, damage, AC, saves)
- Needs: a number on a line of the sheet, counted only while worn or attuned. Exists: ItemBonus for armor, shield and weapons only (rules/itembonus.go:16-75).
- Typical item: weapon-1..3 and armor-1..3 are runs via the piece's "⋮" (a manual number, not linked to the catalog entry). Everything else (Ring and Cloak of Protection, Bracers of Defense,
  Wand of the War Mage, Stone of Good Luck, Defender, Holy Avenger, Sun Blade, Vorpal Sword, Dragon Scale Mail AC) is by-hand-silent: no source for save or spell-attack bonuses.
- Quick tell: weapon-1/2/3, armor-1/2/3 (via gear). +N ammunition is by-hand-silent (ammunition.json counts kinds only).

### charges (51)
- Needs: max, spend, dawn regain, last-charge d20, loss. Exists: all of it (item_charges.json, SpendItemCharges characters/itemchargeskeeper.go:124, stepDawn dawn.go:34, ResolveLastCharge :161, SetItemCharges :191).
- Typical item: runs-part. The count, recharge line, log and d20 run; the spell, damage or rider the charge buys is by hand, and the text is not shown to the player.
  Usar does not test attunement and spends no action or slot. Edge: the ivory goats' 24 charges cover only the goat of traveling; Cube of Force charge losses are by hand.
- Quick tell: key in item_charges.json (every item whose text says "charges"); about 40 more items have once-per-day properties with no counter (by-hand-silent).

### spell (18 non-charge items plus the casts of the 51 charged ones)
- Needs: a spell cast with the item's DC or attack bonus, no components, no slot, and its own save. Exists: nothing links an item to a spell; the granted-spell effect (RN-23) is reachable only from table content;
  SheetMagicItem.spell_key is a proto field with no Go writer.
- Typical item: by-hand-silent (Ring of Spell Storing, Rod of Absorption, Wind Fan, Amulet of the Planes, helms). Spell scrolls: by-hand-silent (not cast, no spell stored, no DC table, no check).
- Quick tell: none; the key list is the text "cast" in the entry.

### consumable (80: 37 potions, 10 scrolls, 4 ammunition, 29 wondrous such as dusts, feather tokens, manuals, tomes)
- Needs: use up a unit, the potion's action, the effect. Exists: play/consumables.go UseConsumable (single-use entry removed, heal dice, 1 hour resistance as a character effect, action and 5 ft in a running combat via
  consumables_combat.go:116, text-only for the rest, RN-02) but NO web caller (core/play/charges-client.ts has no method; the row has "Remover item" only; effect_pt is filled and never rendered).
- Typical item: by-hand-silent, by hand at the table. The backend would give runs for the 14 potions of potions.json and by-hand-said (text reply) for the rest once a screen calls it.
  Oils are excluded from the potion action (rules/potions.go:124 oils). Magic ammunition does nothing.
- Quick tell: category potion/scroll or key in consumables.json (the "Consumível" tag shows on the treasure page only); effect keys: potions.json.

### resistance (29 by scan; resistance, immunity, vulnerability)
- Needs: a damage step on the character (damage_resistances.json shape, a table feature with kind resistance). Exists: engine support for resistance/condition_immunity effects from traits and table features;
  nothing from an item. Potion of resistance is the one item case (backend only, no screen).
- Typical item (Armor of Resistance, Ring of Resistance, Brooch of Shielding, Shield of Missile Attraction, Ring of Mind Shielding): by-hand-silent; the master applies "um valor diferente" on the damage.
- Quick tell: none in data; the master's damage dialog is the only lever.

### ability-score (16; Belts of Giant Strength, Gauntlets of Ogre Power, Headband of Intellect, Amulet of Health, Ioun stones)
- Needs: set or raise a score, derived numbers follow. Exists: modifier mode "set"/"max" exists for derived numbers and ability_increase for classes/feats, never wired to an item.
- Typical item: by-hand-silent (the sheet's scores stay as the player typed them).

### sense-movement (26; darkvision, speeds, flight, water breathing, invisibility)
- Needs: sense and speed lines and a mode on the sheet. Exists: sense effects, speed.walk/fly/swim/climb/burrow targets, breathes_water effect (rules/effects.go:363-375) from traits and table content; none from an item.
- Typical item (Boots of Speed, Cloak of the Bat, Goggles of Night, Ring of Invisibility, Winged Boots): by-hand-silent. Potions of flying, invisibility, speed, water breathing are text only.

### curse (5 by scan: Armor of Vulnerability, Berserker Axe, Demon Armor, Shield of Missile Attraction, Deck of Many Things)
- Needs: a curse that blocks ending attunement or doffing. Exists: nothing; "Encerrar" is open to the owner in any window and to the master at any time (characters/attunement.go:339).
- Typical item: by-hand-silent; the curse text is not on a player screen. Quick tell: none.

### sentient
- The SRD 5.1 snapshot holds no sentient item rules (no entry text names them; Orb of Dragonkind mentions only its random properties). The app has nothing: no alignment, Intelligence or conflict on an entry. Not applicable, nothing to judge.

### container (13 by scan: Bag of Holding, Handy Haversack, Portable Hole, Bag of Devouring, Bag of Tricks, Efreeti Bottle, Iron Flask, Decanter, Folding Boat...)
- Needs: capacity, contents, weight. Exists: none; the sheet's gear is a text list, no weights (a carry capacity modifier target exists for table content only). The bag is a name on the magic item list.
- Typical item: by-hand-silent. Bag of Tricks, Efreeti Bottle and Iron Flask also carry per-day uses with no counter.

### other (59; wondrous items with a situational property: Boots of Elvenkind, Eyes of Minute Seeing, Rope of Climbing, Deck of Illusions...)
- Needs: item text read by the table. Exists: only the entry name, rarity, attunement pill and restriction line. Typical item: by-hand-silent; the text is visible to the master on the treasure page only.

## Common gaps (one-line list)
1. No screen shows an item's text to a player (GetMagicItem is master only).
2. UseConsumable (potions, scrolls, text-only) has no web caller; docs RN-02 describes it as done.
3. Usar on a charged item ignores attunement, action cost and the spell.
4. Attunement gates only +N gear that the web never flags as requiring it, so "works only when attuned" has no effect on a catalog item.
5. No worn/kind limits, no identify state, no curse state, no per-day counter for items without "charges".
