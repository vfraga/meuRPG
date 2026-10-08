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
