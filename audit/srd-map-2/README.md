# SRD map, second pass: equipment, checks, items and monsters

This is a read-only audit of what the app does with the SRD's equipment, ability checks, small lists, magic items and monsters, judged on `origin/main` 1d80454a2.
The verdicts and row fields are the first pass's (`audit/srd-map`), without `step`.
`batches/` holds each investigator batch's raw rows, the brief they worked from, and the coordinator's re-judged rows (`rejudge.md`).

## Totals

| Part | Rows | runs | runs-part | by-hand-said | by-hand-silent | wrong | by-design | unsettled |
|---|---|---|---|---|---|---|---|---|
| Equipment: weapon properties, weapons, ammunition, armor, tools, gear, rules (`equipment.json`) | 125 | 31 | 28 | 3 | 54 | 4 | 5 | 0 |
| Checks and skills (`checks.json`) | 36 | 27 | 3 | 0 | 4 | 1 | 1 | 0 |
| Background, feat, languages, proficiencies by kind (`lists.json`) | 30 | 21 | 0 | 3 | 6 | 0 | 0 | 0 |

## wrong

Except where a line says otherwise, the master can correct each one by hand (undo, another damage amount, removing a condition).
- Weapons:
  - `equipment:blowgun`: the damage "1" plus the Dexterity modifier fails to parse, so the combat rolls 0 damage whenever the modifier is not 0.
  - `equipment:spear`, `equipment:trident`, `weapon-property:versatile`: a lone versatile weapon is held in two hands, and a throw rolls the two-handed die. The player cannot change the grip.
- Checks:
  - `rule:conditions-on-checks`: Frightened gives disadvantage on every ability check, with no line-of-sight test to the source (the first pass's `conditions-frightened`, on checks).

## by-hand-silent

The app applies nothing, or only a part, and no screen text names the rest.
- Weapons: the monk property (wielding only monk weapons is not checked), the lance's special rule, the net (no Restrained, no escape, a damage pending with no dice), improvised weapons other than an ammunition weapon in melee.
- Gear: acid, alchemist's fire, holy water, oil, caltrops, ball bearings, healer's kit, antitoxin, manacles, hunting trap, the net as gear, the torch (as a weapon; the light's one hour is never counted). All are free-text lines; the rest of the gear is not in the data (`gear:not-in-data`).
- Tools: all 30 tool rows but thieves' tools. No check of the app takes a tool, so a tool proficiency never adds its bonus. The same holds for the proficiency kinds artisan's tools, gaming sets, musical instruments, vehicles and other tools.
- Equipment rules: variant encumbrance, lifting and pushing, selling equipment, coins and their weight (no screen edits coins), starting equipment and gold.
- Checks: working together and the Help action outside a combat, contests other than grapple, shove, escape and hide, lifting and carrying, Remarkable Athlete (no effect at all).
- Background: the Acolyte's equipment (not in the data) and its tool line.

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

## unsettled

None in part 1.
