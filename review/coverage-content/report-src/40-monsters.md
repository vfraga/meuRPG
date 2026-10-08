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
| - Pack Tactics (advantage when an ally is adjacent) | 17 | reminder (there is no advantage anywhere: part 0 fact 3) | `rules.md:353` | `rules.md:353` | high |
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
