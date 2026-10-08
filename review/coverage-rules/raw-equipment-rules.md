# Raw sweep: SRD Equipment chapter (+ appendix planes/pantheons)

Sources read: SRD 5.1 Rules JSON, sections standard-exchange-rates, statistics-for-objects, poisons, sample-poisons, attunement, wearing-and-wielding-items, multiple-items-of-the-same-kind, paired-items, activating-an-item (command-word, consumables, spells, charges), sentient-magic-items (creating, conflict); 5e-SRD-Weapon-Properties.json; the weapon/armor rules in backend code. Armor donning/doffing, shields, encumbrance and the weapon-property rules are NOT in the Rules JSON; they come from the Equipment JSON and Weapon-Properties JSON, and are checked against code below. Paths are from /home/user/meuRPG unless absolute. "grep hit" means the line was seen in a Grep result, not in an opened file view.

## Answers to the chapter questions

- Attunement tracked (counter or 3-item limit)? NO. Item metadata only (Attunement flag and restriction, Portuguese label). No character-side attuned list, no counter, no 3-item limit, no one-copy rule, no short-rest attunement flow. Web shows the tag "Exige sintonização" on the treasure page only.
- Charges tracked and spent? NO. No item charges, no recharge at dawn, no destruction at 0. Only class "resource" effects (uses per rest) exist, which are not item charges.
- Consumables (potions, scrolls, ammunition) used up in play? NO. `effects/consumables.json` is a list used for value halving in the treasure generator only. Character equipment is free text (name + quantity), and nothing decrements it.
- Weapon properties enforced in the attack flow? PARTLY. Finesse and Light are enforced in the derived attack data; Thrown changes the range used; heavy-for-Small and versatile are display text only; loading, two-handed (as a rule), and ammunition count are not enforced at all.
- Currency tracked and converted? TRACKED as five integer counts, NOT converted in the wallet. Conversion appears only as display text for generated treasure piles.
- Poisons applicable? NO. The SRD poisons table is not in code. Only the trap "Agulha envenenada" applies poison damage and the poisoned condition.

## Mechanics

MECHANIC: standard-exchange-rates / currency (cp/sp/ep/gp/pp conversion table, 1 gp = 10 sp = 100 cp, 1 pp = 10 gp)
SERVER: backend/internal/rules/treasure.go:34-40 (coin constants pc, pp, pe, po, pl; comment "10 PP = 1 PO"); no wallet conversion or exchange function found (Grep for câmbio/cambio/exchange/converte in backend returned none outside treasure value code)
SCREEN: web/src/app/core/treasure/treasure-format.ts:40-43 (line under a non-gold coin, e.g. "10 PP valem 1 PO", display only); web/src/app/pages/character-sheet/combat-column/combat-column.html:166-172 (sheet lists the character's coins, "N ABBR" per non-zero coin, no conversion)
DOCS: docs/product/rules.md:119 (By gold: "1 XP per gold piece (GP)"; GP converted to XP, not to other coins); no doc claims wallet conversion
TESTS: backend/internal/rules/treasure_test.go:99 (TestTreasureGolden, generated coin values); backend/internal/progression/gold_test.go:91 (TestMR041, GP to XP)

MECHANIC: standard-exchange-rates / character coins held (wallet)
SERVER: backend/internal/characters/sheet.go:242-257 (five coin counts copper, silver, electrum, gold, platinum, each 0 to maxCoins); backend/internal/characters/sheet.go:34 (maxCoins = 1,000,000)
SCREEN: web/src/app/pages/character-sheet/combat-column/combat-column.html:166-175 ("Moedas" list, or "Sem moedas"); web/src/app/pages/character-sheet/sheet-format.ts:158-168 (coinEntries, platinum first, skips zeros); no coin input found in web/src/app/pages/character-editor/character-editor.html (Grep for coins/moedas/ouro: no match)
DOCS: no match
TESTS: backend/internal/characters/unit_test.go:558 (negative gold refused); backend/internal/characters/harness_test.go:464 (sample coins)

MECHANIC: objects / statistics-for-objects (AC by substance, hit points by size, damage threshold, immunity to poison and psychic)
SERVER: no match for "object armor class", "damage threshold", "objeto" stats (Grep for object AC/HP/limiar returned none in backend/internal/rules; objects are not a rules type)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: poisons / types (contact, ingested, inhaled, injury; poison applied to weapons and ammunition)
SERVER: no match for poison item type or application. Only: backend/internal/rules/traps.go:63-66 (trap damage type "damage-type:poison", comment about the poison needle); backend/internal/rules/srd51/effects/names_pt.json:133 ("condition:poisoned": "Envenenado") and :870 ("item:potion-of-poison", a magic potion, not an SRD poison)
SCREEN: web/src/app/pages/maps/point-kinds/trap-point-panel.spec.ts:44-60 (trap preset "Agulha envenenada": poison damage and poisoned condition, spec only; the panel itself is web/src/app/pages/maps/point-kinds/trap-point-panel.ts:340)
DOCS: docs/product/stories.md:825 (eight SRD trap presets incl. "2d10 veneno"); no doc mentions poison items or the poison table
TESTS: backend/internal/maps/kinds_test.go:43 (trap with poisoned condition); backend/internal/rules/traps_test.go (poison needle, not opened)

MECHANIC: poisons / sample poisons (Assassin's Blood, Burnt Othur Fumes, Crawler Mucus, Drow Poison, Essence of Ether, Malice, Midnight Tears, Oil of Taggit, Pale Tincture, Purple Worm Poison, Serpent Venom, Torpor, Truth Serum, Wyvern Poison; DCs, durations, repeat saves)
SERVER: no match for these poisons (Grep "assassin|crawler-mucus|wyvern-poison|serpent-venom|oil-of-taggit|burnt-othur" in backend/internal and backend/cmd: only names in srd51/data/magic-items.json and monsters.json and names_pt.json:659 "Flecha assassina", an unrelated arrow)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: attunement / 3-item limit and one-copy rule
SERVER: no character-side state; item flags only: backend/internal/rules/magicitems.go:45-50 (Attunement, AttunementBy); backend/internal/rules/magicitems.go:94-96 (restriction needs attunement); backend/internal/rules/magicitems.go:238 (copied into the entry). Grep "attun|sintoniz" in backend/internal/characters and backend/internal/play: no match. backend/internal/maps/treasure.go:418 (grep hit: "if it.Attunement")
SCREEN: web/src/app/pages/treasure/item-sheet/item-sheet.html:35-36 (shows "Exige sintonização por um paladino", the requirement only); web/src/app/core/treasure/treasure-format.ts:136-155 (tags and text); no attuned-items list or counter on the character sheet (Grep web for sintoniz on character-sheet and character-editor: none)
DOCS: docs/product/glossary.md:192 (Attunement defined as the character's bond, "only works for whoever attuned"); no doc claims the 3-item limit or a tracked list
TESTS: backend/internal/rules/magicitems_test.go:246 (TestMagicItemRaritiesAndAttunement, item flags only); web/src/app/core/treasure/treasure-format.spec.ts:62-72 (tag and text only); backend/internal/maps/treasure_test.go:256 (attunement label)

MECHANIC: attunement / short rest to attune (focused short rest, interruption fails, no same-rest learning)
SERVER: no match (no attunement action or short rest focus in backend/internal/play or backend/internal/characters; rest logic in backend/internal/rules/effects.go:145 covers only resource recharges "short_rest", "long_rest", "dawn", "none")
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: attunement / class and spellcaster restrictions (must be a member of the class; spellcaster must cast a spell from own traits)
SERVER: backend/internal/rules/magicitems.go:125-129 (attunementPT: Portuguese label for the restriction); backend/internal/rules/magicitems.go:175-186 (restriction labels checked against names_pt.json); no check that a character meets the class or spellcaster prerequisite (no match in play/characters)
SCREEN: web/src/app/pages/treasure/item-sheet/item-sheet.html:35-36 (text "por um paladino" only)
DOCS: docs/product/glossary.md:192 ("Some items limit who can (por um paladino, por um conjurador)")
TESTS: backend/internal/rules/magicitems_test.go:263-272 (Portuguese label for Holy Avenger restriction)

MECHANIC: wearing-and-wielding / donning the intended item (magic items worn or held; size and fit)
SERVER: no match (items are text; no worn or held state for magic items on the sheet). Equipment is backend/internal/characters/sheet.go:225-240 (free-text name and quantity, 1 to 9,999; 0 means 1)
SCREEN: web/src/app/pages/character-sheet/combat-column/combat-column.html:142-163 (Equipamento list, names only, no worn/held flag)
DOCS: no match
TESTS: no match

MECHANIC: wearing-and-wielding / multiple items of the same kind (one pair of footwear, gloves, bracers, armor suit, headwear, cloak)
SERVER: no match (no check of duplicates in backend/internal/characters or backend/internal/rules)
SCREEN: web/src/app/pages/character-editor/character-editor.html:659-667 (free-text hint: "Para mais de um item igual, escreva (xN)"), no kind limit
DOCS: no match
TESTS: no match

MECHANIC: wearing-and-wielding / paired items (both needed for benefit)
SERVER: no match
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: activating-an-item / command word (must be spoken; not in silence)
SERVER: no match (only item text in backend/internal/rules/srd51/data/magic-items.json)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: activating-an-item / consumables (used up on activation; potion swallowed, scroll read)
SERVER: backend/internal/rules/srd51/effects/consumables.json:1-42 (list of potions, scrolls and single-use items, used only for value halving); backend/internal/rules/magicitems.go:57-62 (Consumable flag: the value table halves a consumable's value); backend/internal/rules/treasure_generate.go:243-245 (halving applied). No decrement of any inventory item or potion on use (no match in backend/internal/play)
SCREEN: web/src/app/core/treasure/treasure-format.ts:25 (CONSUMABLE_RULE text: "Itens que se gastam valem a metade"); no use button or count for potions
DOCS: docs/product/glossary.md (no consumable use claim found; not opened in full)
TESTS: backend/internal/rules/magicitems_test.go:276 (TestMagicItemConsumables, list only)

MECHANIC: activating-an-item / spells from items (cast at lowest level, no slot, no components; concentration)
SERVER: no match (no item-cast path in backend/internal/rules/spellcasting.go or backend/internal/play; spell scroll is a catalog flag only, backend/internal/rules/magicitems.go:61)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: activating-an-item / charges (expended to activate, identify reveals count, regain at dawn, destroyed at 0)
SERVER: no match for item charges. The word "carga" is used only for spell slot and class resource names (backend/internal/rules/effects.go:68-78, resource recharge; not item charges)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: sentient-magic-items / creating (Int, Wis, Cha, communication, senses, alignment, special purpose)
SERVER: no match (only item text in backend/internal/rules/srd51/data/magic-items.json)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: sentient-magic-items / conflict (Charisma contest, demands, charm save DC 12 + Cha mod)
SERVER: no match
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: armor / donning and doffing (time to put on and take off armor; shield strapped)
SERVER: no match (no donning time or state in backend/internal/rules/armor.go or backend/internal/characters)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: armor / heavy armor Strength requirement (speed cut 10 ft unless met; dwarves exempt)
SERVER: backend/internal/rules/hitpoints.go:77-79 (speed cut 10 ft, dwarves ignored; enforced in SpeedWalkFt); backend/internal/rules/armor.go:46-50 (hint text "pede FOR N")
SCREEN: web/src/app/pages/character-sheet/combat-column/combat-column.html:142-163 (no speed-cut text in this file; the speed hint is not rendered on the sheet UI I opened)
DOCS: docs/product/rules.md:281 (speed "cuts it by 10 ft when the character wears heavy armor whose Strength requirement they do not meet")
TESTS: backend/internal/rules/attacks_test.go:186 (TestHeavyArmorStrengthRequirementCutsSpeed)

MECHANIC: armor / shields (+2 AC; separate from armor; proficiency)
SERVER: backend/internal/rules/armor.go:75-82 (+2 AC, proficiency issue); backend/internal/rules/armor.go:98-100 (shield category refused as armor)
SCREEN: web/src/app/pages/character-sheet/combat-column/combat-column.html:145-146 (line "Escudo" in Equipamento)
DOCS: no match for shield-specific claim
TESTS: backend/internal/rules/attacks_test.go:14 (TestMartialArtsNeedsNoArmorOrShield, shield off monk)

MECHANIC: weapon properties / finesse (STR or DEX for attack and damage, same modifier for both)
SERVER: backend/internal/rules/attacks.go:49-54 (better of STR and DEX, melee or ranged); backend/internal/rules/attacks.go:67 (damage uses the same ability mod, x.mods[ab])
SCREEN: web/src/app/pages/character-sheet/combat-column/combat-column.html:24-63 (Ataques table shows the bonus and damage; the choice is not shown)
DOCS: no match
TESTS: backend/internal/rules/attacks_test.go:101 (TestFinesseWeaponTakesBetterAbilityWhenRanged); backend/internal/rules/attacks_test.go:27 (dagger finesse damage)

MECHANIC: weapon properties / light (two-weapon fighting, bonus action attack)
SERVER: backend/internal/rules/attacks.go:77 (Light = melee and light property)
SCREEN: no match for the light flag on the sheet
DOCS: no match
TESTS: backend/internal/rules/attacks_test.go:293-311 (TestAttacksCarryWhatTheBonusActionAttacksRead)

MECHANIC: weapon properties / heavy (Small creatures: disadvantage on attacks)
SERVER: backend/internal/rules/attacks.go:56-61 (adds a disadvantage Hint, text only). backend/internal/characters/derived.go:178-179 maps hints to text. No play code reads Hints for attack rolls (Grep: Hints only in backend/internal/play/puzzles*.go). NOT enforced on the roll.
SCREEN: no match for a heavy warning on the attack UI (the hint text is not rendered in combat-column.html)
DOCS: no match
TESTS: backend/internal/rules/attacks_test.go:119 (TestHeavyWeaponHintForSmallRace, hint only)

MECHANIC: weapon properties / reach (5 ft added)
SERVER: backend/internal/play/combat_actions.go:45-46 (meleeReachFt = 5); backend/internal/play/combat_actions.go:179-191 (reach = max of range, long range, 5 ft); no reach property code in backend/internal/rules/attacks.go (Grep: none)
SCREEN: web/src/app/pages/character-sheet/combat-column/combat-column.html:24-63 (no range shown; the lines 24-63 of the Ataques table, opened in part)
DOCS: no match
TESTS: backend/internal/rules/attacks_test.go:14 (not on reach; no reach test found)

MECHANIC: weapon properties / thrown (ranged attack with the weapon; normal and long range; melee weapon uses STR or finesse DEX)
SERVER: backend/internal/rules/attacks.go:90-93 (throw range used as RangeFt and LongRangeFt when ThrowNormalFt > 0); backend/internal/play/combat_actions.go:659-690 (opportunity attack is melee, reach 5 ft, thrown counts); backend/internal/play/combat_opportunity.go:63-72 (long range means thrown, opportunity uses melee)
SCREEN: web/src/app/pages/character-sheet/combat-column/combat-column.html:24-63 (no range shown; the lines 24-63 of the Ataques table, opened in part)
DOCS: no match
TESTS: backend/internal/rules/attacks_test.go:101 (dart finesse thrown)

MECHANIC: weapon properties / two-handed and versatile (damage in parentheses used with two hands)
SERVER: backend/internal/rules/attacks.go:82-87 (VersatileDamage computed, monk die applied); backend/internal/rules/attacks.go:96 (VersatileDice). No play code reads VersatileDice or VersatileDamage (Grep versatile|VersatileDice in backend/internal/play non-test: no match). Not chosen per attack.
SCREEN: web/src/app/pages/character-sheet/combat-column/combat-column.html:48 ("Com duas mãos: {{ atk.versatileDamage }}", shown as a label only)
DOCS: no match
TESTS: backend/internal/rules/attacks_test.go:45-57 (TestMonkDieAppliesToVersatileDamage)

MECHANIC: weapon properties / two-handed (requires two hands to attack)
SERVER: no match (no hands state, no check; only versatile label above)
SCREEN: no match beyond the versatile label
DOCS: no match
TESTS: no match

MECHANIC: weapon properties / loading (one piece of ammunition per action, bonus action or reaction)
SERVER: no match (Grep "loading|carregamento|recarreg" in backend/internal/play and rules attack code: none)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: weapon properties / ammunition (expend one piece per attack; need ammunition; recover half after battle)
SERVER: no match for expenditure or count. Data only: backend/internal/rules/srd51/data/equipment.json:38,127-172,468 (ammunition property on bows, crossbows); backend/internal/rules/srd51/effects/consumables.json:5-6 (ammunition items as treasure). No quiver count and no decrement in backend/internal/play or backend/internal/characters.
SCREEN: no match (Grep munição/Munição in web/src/app: no match)
DOCS: no match
TESTS: no match

MECHANIC: weapon properties / special (special rules in weapon description)
SERVER: no match (no special-weapon rule code)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: encumbrance (carry weight, speed penalty, weight of gear)
SERVER: no match (Grep encumb|sobrecarga|peso in backend/internal/rules and characters: only appearance weight at backend/internal/characters/sheet.go:384 and treasure weights)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: appendix / the-planes-of-existence (content only)
SERVER: no match (lore text only; no planar rules in backend/internal/rules)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: appendix / fantasy-historical-pantheons (content only)
SERVER: no match (lore text only)
SCREEN: no match
DOCS: no match
TESTS: no match

MECHANIC: equipment / treasure value halving for consumables (SRD 5.2.1 table) (extra, grep-confirmed)
SERVER: backend/internal/rules/treasure_generate.go:216-217 and :243-245 (Halved, except Spell Scroll)
SCREEN: web/src/app/core/treasure/treasure-format.ts:25 (CONSUMABLE_RULE)
DOCS: no match
TESTS: backend/internal/rules/magicitems_test.go:276 (TestMagicItemConsumables)
