# SRD map, second pass: equipment, checks, items and monsters

This is a read-only audit of what the app does with the SRD's equipment, ability checks, small lists, magic items and monsters, judged on `origin/main` 1d80454a2.
Verdict rulings the coordinator applied across batches are in `batches/rejudge.md`.
The verdicts and row fields are the first pass's (`audit/srd-map`), without `step`.
`batches/` holds each investigator batch's raw rows, the brief they worked from, and the coordinator's re-judged rows (`rejudge.md`).

## Totals

| Part | Rows | runs | runs-part | by-hand-said | by-hand-silent | wrong | by-design | unsettled |
|---|---|---|---|---|---|---|---|---|
| Equipment: weapon properties, weapons, ammunition, armor, tools, gear, rules (`equipment.json`) | 125 | 31 | 28 | 3 | 54 | 4 | 5 | 0 |
| Checks and skills (`checks.json`) | 36 | 27 | 3 | 0 | 4 | 1 | 1 | 0 |
| Background, feat, languages, proficiencies by kind (`lists.json`) | 30 | 21 | 0 | 3 | 6 | 0 | 0 | 0 |
| Magic items: 24 general rules and 362 items (`items.json`) | 386 | 13 | 2 | 0 | 326 | 43 | 2 | 0 |
| Monster kinds: 150 traits and 71 action, usage, reaction, legendary, defense, sense, speed and AC kinds (`monster-kinds.json`) | 221 | 21 | 8 | 0 | 109 | 54 | 29 | 0 |

`monsters.json` lists the 334 monsters with their challenge rating and the kinds whose verdict is not `runs`; `p3/build_monsters.py` builds it and `monster-kinds.json`'s `party` field from the kind rows (`p3/inventory.py` finds each monster's kinds).
A kind is `party` when a beast of challenge 1 or lower or a familiar form (Find Familiar, Pact of the Chain) has it: 74 forms, 58 kinds.

## wrong

Except where a line says otherwise, the master can correct each one by hand (undo, another damage amount, removing a condition).
- Weapons:
  - `equipment:blowgun`: the damage "1" plus the Dexterity modifier fails to parse, so the combat rolls 0 damage whenever the modifier is not 0.
  - `equipment:spear`, `equipment:trident`, `weapon-property:versatile`: a lone versatile weapon is held in two hands, and a throw rolls the two-handed die. The player cannot change the grip.
- Checks:
  - `rule:conditions-on-checks`: Frightened gives disadvantage on every ability check, with no line-of-sight test to the source (the first pass's `conditions-frightened`, on checks).

- Magic items (one cause): an item that requires attunement spends its charges while it is not attuned (`rule:item-attunement`; `SpendItemCharges` never reads attunement). The master gives a charge back by hand. The 42 items: eyes-of-charming, medallion-of-thoughts, pipes-of-the-sewers, trident-of-fish-command, wand-of-web, cube-of-force, gem-of-seeing, helm-of-teleportation, mace-of-terror, ring-of-evasion, ring-of-the-ram, staff-of-charming, staff-of-healing, staff-of-the-woodlands, staff-of-withering, wand-of-binding, wand-of-enemy-detection, wand-of-fear, wand-of-fireballs, wand-of-lightning-bolts, wand-of-paralysis, wand-of-wonder, nine-lives-stealer, ring-of-shooting-stars, robe-of-scintillating-colors, staff-of-fire, staff-of-frost, staff-of-power, staff-of-striking, staff-of-swarming-insects, wand-of-polymorph, hammer-of-thunderbolts (its charges are spent "while attuned", though the data entry carries no attunement), luck-blade, orb-of-dragonkind, the four rings of elemental command, scarab-of-protection, staff-of-the-magi, talisman-of-pure-good, talisman-of-ultimate-evil. Every charge number (maximum, dawn regain, the last-charge d20, the loss) matches the text.
- Monsters, the combat's own mechanics (`*` marks a `party` kind):
  - Attacks: only the first damage part is rolled, so a swarm always rolls its full dice and a thrown weapon uses the melee dice; a bestiary NPC's sheet holds three attacks (`*action:melee-attack`).
  - Save riders read the wrong condition: the cockatrice petrifies at once, and Swallow (kraken, purple worm, remorhaz, tarrasque, behir) gives prone and never swallowed (`*action:melee-attack+save`, `*action:save-effect`, `action:save-effect+damage` and the swallow rows); the bulette's leap is a Strength save only.
  - A dragon's breath options each keep their own recharge (`action:other:breath weapons`).
  - Resistances and vulnerabilities with a source ("damage from spells") are skipped (`*resistances:plain`, `vulnerabilities:plain`).
  - Blindsight and truesight do not see past blinded or invisible, so reaction and opportunity windows are refused (`*sense:blindsight`, `sense:truesight`).
  - A bestiary NPC keeps only one speed: flying, swimming and climbing are lost (`*speed:fly`, `*speed:swim`, `*speed:climb`); "Mover mesmo assim" moves it by hand.
  - Legendary Resistance is not offered on a trap's save (`usage:per-day`).
- Monster traits the app works against (it rolls or applies the mechanic and leaves the trait out):
  - Magic Weapons (12 monsters), Angelic Weapons (3) and Hellish Weapons (1): a bestiary NPC's attacks are not magical, so a resistance to nonmagical attacks halves them.
  - Saves without the trait's advantage: brave, dark devotion, duergar resilience, fey ancestry (Sleep still works), gnome cunning, limited magic immunity, multiple heads, two heads, two-headed, `*sure-footed`, turn resistance, turn defiance, steadfast.
  - Attack or check modes left out: ambusher, assassinate, `*blood frenzy`, grappler, vampire weaknesses, the keen sight rows in Hide (`*keen sight`, `*keen senses`, `*keen sight and smell`, `*keen hearing and sight`), invisibility (the invisible stalker enters with no Invisible condition).
  - Effects applied that the trait stops: `*web walker` (the Web zone restrains), freedom of movement, reflective carapace (Magic Missile and ranged spell attacks), swarm (healing and temporary hit points), `*flyby`, `*echolocation` and blind senses (opportunity attacks still offered), `*devil's sight` (magical darkness blocks it), air, fire and water form and swarm (refused a hostile creature's space), reactive and reactive heads (one reaction a round).

## by-hand-silent

The app applies nothing, or only a part, and no screen text names the rest.
- Weapons: the monk property (wielding only monk weapons is not checked), the lance's special rule, the net (no Restrained, no escape, a damage pending with no dice), improvised weapons other than an ammunition weapon in melee.
- Gear: acid, alchemist's fire, holy water, oil, caltrops, ball bearings, healer's kit, antitoxin, manacles, hunting trap, the net as gear, the torch (as a weapon; the light's one hour is never counted). All are free-text lines; the rest of the gear is not in the data (`gear:not-in-data`).
- Tools: all 30 tool rows but thieves' tools. No check of the app takes a tool, so a tool proficiency never adds its bonus. The same holds for the proficiency kinds artisan's tools, gaming sets, musical instruments, vehicles and other tools.
- Equipment rules: variant encumbrance, lifting and pushing, selling equipment, coins and their weight (no screen edits coins), starting equipment and gold.
- Checks: working together and the Help action outside a combat, contests other than grapple, shove, escape and hide, lifting and carrying, Remarkable Athlete (no effect at all).
- Background: the Acolyte's equipment (not in the data) and its tool line.
- Monster traits (109 rows): every trait the engine does not read is text in the bestiary stat block only, which the master opens from "Ficha da criatura"; the combat card shows no trait. Among them the `party` ones: amphibious, beast of burden, charge, hold breath, illumination, keen hearing, keen hearing and smell, keen smell, limited telepathy, magic resistance (traps), mimicry, pounce, rampage, relentless, running leap, shapechanger, spider climb, standing leap, trampling charge, underwater camouflage, water breathing, web sense, burrow speed, a recharge after a rest. The others include regeneration, undead fortitude, death burst, incorporeal movement, false appearance, the auras, legendary resistance and innate spellcasting beyond what is cast, tremorsense, hover and a monster's alternate armor class.
- Magic items, by family (326 rows). A player never reads an item's text (`GetMagicItem` is master-only), so an item that only sits on the sheet is silent.
  - Consumables (54): potions, oils, scrolls, dusts and ammunition. `UseConsumable` exists in the backend, with correct dice for the healing potions and the resistance potions, but no screen calls it; a potion row offers only "Remover item".
  - Charges (10 without attunement, among them the wands of magic missiles, magic detection and secrets): the counter, the dawn regain and the last-charge d20 run; what a charge does is by hand, and the use sheet only points to the item's text.
  - Bonus (49): only +N armor, shield and weapons apply, typed on the piece and not linked to the catalog; rings, cloaks, bracers, Ioun stones, wands of the war mage and the riders of named weapons are silent.
  - Spell (31), resistance (28), ability score (27), sense and movement (23), container (9), curse (4) and other (85): no item reaches an effect, a resistance, a score, a speed or a sense; nothing casts a spell from an item; curses do not hold ("Encerrar" ends any attunement).
  - General rules: wearing limits, activation, spells from items, daily uses with no charges, identifying, a potion's action, the spell scroll (its check, DC, attack and copying), cursed items, magic ammunition, bonuses from other items, item effects.

## Not claimed anywhere

Neither the code nor `docs/product/rules.md` mentions these.
- Weapons: the lance and net special rules; improvised weapons in general; a sling loaded to deal damage; a free hand to load a one-handed ammunition weapon; Loading (Extra Attack fires a crossbow twice); Heavy for a Small creature reaches the sheet's reminders but never the roll; the off-hand attack does not check that the weapon differs from the main hand, and Dueling's +2 stays on the off-hand line.
- Armor: donning and doffing times (1, 5 and 10 minutes); initiative ignores the disadvantage of armor worn without proficiency.
- Gear: adventuring gear, its weight and price are not in the data; the torch's and other lights' durations are never counted.
- Coins: no screen edits them; found gold becomes experience only, never coins.
- Carrying: encumbrance, lifting and pushing (30 x Strength), overload.
- Checks: the low-obstacle jump check and the landing check after a jump (only a log line); stabilizing with Medicine (no check tied to death saves); Animal Handling for a mount; opposed checks other than the built ones; a table of typical DCs (the master types 1 to 30); Remarkable Athlete's running long jump.
- Proficiencies: the Bard's three starting instruments are neither granted nor offered; the tool picks (the dwarf's choice, a background's tool group) offer only kind "tool", not the kits, thieves' tools or vehicles; four kit proficiencies point at equipment rows that do not exist (disguise, forgery, herbalism, poisoner's kits); expertise cannot take thieves' tools; only 2 gaming sets.
- Languages: the script is never read; nothing compares a character's languages with a creature's.
- Feat: the Grappler pin reminder says "contidos" where the app names the condition "Impedido"; its attack advantage tag is never read by the combat.

- Items: wearing limits (two rings, one cloak); identifying; curses and attunement that cannot end; a counter for once-a-day uses without charges (about 40 items); the spell scroll's check, DC table and copying; +N ammunition; whether charges need attunement; item-sourced effects of every kind; the restorative ointment counted as one unit where the text gives 1d4+1 doses; the Staff of Striking's 3-charge limit per hit (the use sheet takes any amount); RN-02 describes "Usar" on potions and consumables, which no screen offers.

- Monsters: SRD monster traits as text only (the ruling covers a table creature's own traits); one recharge shared by a multi-breath action; staged petrification and swallowed as a state; size limits on grapple riders; a swarm's half damage under half hit points; tremorsense and burrowing; a monster's alternate armor class and hover.
- By-design kinds (29): the 24 actions that roll nothing (change shape, teleport, leadership and the like) spend the action and write a log line (RN-41), and five reactions are text the master marks on the reaction chip (RN-29).

## unsettled

None.
