## 7. Shared causes

One missing mechanism, many rows. First the five engine facts every part points at, then the causes.

### 7.1 The engine, in five facts that decide most rows

These are the mechanisms the rest of the report keeps pointing at. Each was read in code.

1. **A cast computes five things and nothing else.** `CastSpell` (`backend/internal/play/combat_spells.go:192`) spends
   the slot and the action, sets concentration, and then, for each target, runs one of: a spell attack roll with the
   first damage type (`:598`), a saving throw with full/half/no damage (`:636`), Magic Missile's darts (`:670`), a
   pending heal (`:560`+), or one of 8 hit-point effects (`play/combat_spells_hp.go:91`, data in
   `effects/spells.json`). Summons go through a separate choice (`play/creature_cast.go:42`). The proto says it
   plainly: "Anything else (Teia, Passo Nebuloso...): it spends and goes to the log; the effect is the table's"
   (`proto/meurpg/play/v1/combat.proto:885`), and the web shows "A magia foi conjurada: o mestre resolve o efeito."
   (`web/src/app/pages/live-session/combat/cast-sheet/cast-result.ts:63`).
2. **Conditions are labels.** `play/combat_conditions.go:14-18`: "labels the master marks and the app reminds the table
   about; the engine applies no effect." The only code that reads a condition is speed 0 for grappled, restrained,
   paralyzed, petrified, stunned, unconscious (`play/combat_move.go:144-146`) and "cannot react" for
   incapacitated/paralyzed/petrified/stunned/unconscious/blinded (`play/combat_opportunity.go:42-44`). No spell applies a
   condition except Sleep, Color Spray and Power Word Stun (`effects/spells.json`).
3. **There is no advantage or disadvantage in any roll.** An attack, save or check is one d20
   (`RollAttackRequest` has `roll_in_app` or `d20_face` only, `combat.proto:2539`; `play/combat_spells.go:582` `d20`).
   Every `roll_mode` effect, "Desvantagem em Furtividade", Pack Tactics and flanking are text.
   Doc: "Out of scope: flanking, which asks for advantage on attacks (the app does not apply it yet)"
   (`docs/product/rules.md:353`).
4. **Nothing happens at the start or end of a turn, and nothing expires.** `EndTurn` (`play/combat.go:744`) and
   `startTurn` (`play/combat_turn.go:109`) reset the economy and the familiar's sight; there are no durations, no repeat
   saves, no ongoing damage, no regeneration, no recharge dice. Doc: "Applying effects automatically is left for after
   the MVP. That includes the saving throws a spell forces later ... the master calls for the later ones"
   (`docs/product/rules.md:329`).
5. **Content that is not a character's own sheet is a catalogue.** Magic items, adventuring gear, mounts, vehicles,
   monsters' traits and spellcasting are data plus English text. A character's equipment is free text
   (`proto/meurpg/characters/v1/characters.proto:1150` `Item`, 1003 `equipment`), and an NPC made from a monster keeps at most
   three attacks (`backend/internal/characters/npcfromcreature.go:105`).

Not in the list of five but used everywhere: **damage to a player's character waits for the master**, who applies it
(`docs/product/rules.md:56`: "A player character's resistances (Rage and others) are notes the master applies"), while
damage to an NPC lands at once with its plain resistances (`play/combat_actions.go:1126` `afterResistance`).

### 7.2 Causes

| ID | Missing mechanism | What it explains | Rows / counts | Evidence |
| --- | --- | --- | --- | --- |
| C1 | **Conditions are labels with no effect.** Only speed 0 and "cannot react" read them; no roll, save or attack reads a condition; immunities are never checked | spells that impose a condition (38 flagged, plus Hold Person's paralysis...), monsters' riders (129), condition immunities (92), Sleep ignoring immunities | spells 1.3 "condition"; monsters 4.1 | `play/combat_conditions.go:14-18`, `play/combat_move.go:144`, `play/combat_opportunity.go:42` |
| C2 | **No roll modes (advantage / disadvantage) in the combat, the cast or the check.** A roll is one d20; "Hints" carry the rest as text | 12 race traits, Pack Tactics, Magic Resistance, flanking, long range, heavy weapons, stealth armour, Bless / Bane / Guidance as bonus dice, guiding bolt, faerie fire, true strike, Vicious Mockery | races 5.1, monsters 4.1, equipment 2.1 and 2.2 | `play/combat_spells.go:582`, `combat.proto:2539`, `rules/effects.go:35` (tagged roll modes are "never applied"), `docs/product/rules.md:353` |
| C3 | **No turn clock.** Start / end of turn does nothing; no durations, no repeat saves, no ongoing damage, no recharge dice, no regeneration, no legendary-action pool, no concentration timer | 136 spells with a duration, 33 repeat saves, 22 ongoing damage, 118 concentration spells, recharge (71 monsters), Regeneration, legendary actions (32), Undead Fortitude, Relentless Endurance | spells 1.3, monsters 4.1, races 5.1 | `play/combat.go:744` `EndTurn`, `play/combat_turn.go:109` `startTurn`, `docs/product/rules.md:329` |
| C4 | **No temporary modifiers on a combatant.** Only Shield's `AcBonus`, temporary hit points and a raised maximum exist; no timed +AC, +attack, +save, speed, resistance, extra damage dice | 53 stat-change spells; items; hunters mark, hex style riders | spells 1.3 "stat_change" | `play/combat_reactions.go` (`AcBonus`), `play/combat_spells_hp.go:308-336` |
| C5 | **Nothing a spell leaves on the map.** Area shapes are data; there are no zones, walls, clouds, lights or summoned objects as map pieces | 57 zone / object spells, 68 with movement, 94 area spells | spells 1.3 "zone" | `rules/spelldetails.go:140`, `maps/` has traps and lights as separate master tools |
| C6 | **No cast flow outside a combat, for rituals or for a long casting time.** `CastSpell` needs an encounter; `CastSummon` handles three spells | 57 long spells, 29 rituals (Find Familiar only), healing between fights | spells 1.1 | `play/combat_spells.go:192`, `rules/combat/turn.go:478`, `play/creature_cast.go:170` |
| C7 | **No item model on a sheet.** Equipment is free text; weapons / armour are content keys without bonuses; magic items are a catalogue; no charges, attunement list, weight, ammunition, consumption | magic items (362), gear, ammunition, potions, coins, +N weapons | parts 2 and 3 | `characters.proto:1150`, `rules/attacks.go:36`, `docs/architecture.md:754` |
| C8 | **A monster fights as a basic NPC sheet.** Three attacks, one damage die each, no save actions, no recharge, no traits, no spells, no reactions, no legendary actions | 19 attack-less monsters, 134 save actions, 112 recharge actions, 32 legendary monsters, 36 casters, 12 reaction monsters | part 4 | `characters/npcfromcreature.go:105-165`, `rules/creatures.go:499` |
| C9 | **Reactions are two special cases.** Shield and the opportunity attack are coded; the rest of the reaction family has no trigger / prompt | Counterspell, Hellish Rebuke, Feather Fall, Parry (6 monsters), Rock Catching, Shield Guardian | spells 1.1, monsters 4.1 | `play/combat_reactions.go:31`, `rules/combat/turn.go:482` |
| C10 | **Damage to a player's character is applied by the master**, so resistances (Rage, Hellish Resistance, Dwarven Resilience, dragonborn ancestry) are notes | 4 race traits and class features | races 5.1 | `docs/product/rules.md:56`, `play/combat_actions.go:1126-1155` |
| C11 | **The importer's scope.** Equipment categories other than weapon, armour and tools, feats, lair actions are not imported | gear, mounts, vehicles, ammunition, Grappler | parts 2 and 6 | `backend/cmd/srdimport/main.go:914`, `:150` |
| C12 | **Targets are picked by hand.** The server limits the number (10) and the range, but does not work out who stands in a cone, cube, sphere or line | 94 area spells | spells 1.1 | `play/combat_spells.go:47`, `rules/spelldetails.go:140` |

### 7.3 What the best-return mechanisms would unlock

- **C1 + C3 together** (a condition on a combatant with a duration and a repeat-save hook) would turn about 70 spells and most of the
  129 monster riders from reminders into rolls: Hold Person, Web, Fear, Entangle, a ghoul's claw.
- **C2** (an advantage / disadvantage flag on a roll, set by a condition or by hand) unlocks Pack Tactics, Magic Resistance and the
  12 race traits that are hints today. Bless / Bane / Guidance as extra dice need **C4**.
- **C8** (let a stat block keep all of its attacks and save actions, plus a "used / ready" state) is the most visible one at the table:
  dragons, spiders, ghouls, and every bat and rat.
- **C7** is the largest. A cheap first step exists: a list of item keys on a sheet whose +N and protection items are `modifier`
  effects, which the effect vocabulary can already express (`rules/effects.go:135`).
