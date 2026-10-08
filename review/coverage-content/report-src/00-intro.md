# MeuRPG content coverage: what the app runs, what it only lists, what is missing

Inventory of the SRD 5.1 content in `backend/internal/rules/srd51/data/*.json` against what the code does with it.
Scope: spells, equipment, magic items, monsters, races, backgrounds and feats. Class features and the rules chapters are
mapped elsewhere. **No code was changed.** Base: `origin/main` of `vfraga/meuRPG`.

How to read it:

- **State**: `built` (the app computes what the text asks for), `partial` (it computes something real and misses a named
  part), `reminder` (the content is listed or shown as text, or a resource is spent, and nothing is computed) and
  `absent` (it cannot be used at all, or is not in the data).
- **Evidence** is `file:line` on `origin/main`. Every state was checked in code; docs are cited only in the
  "Deliberate?" column, and a doc is never the evidence for a state.
- **Deliberate?** quotes where a doc says the gap is on purpose. "no doc" means none was found.
- **Table impact** is a judgement of how often a table would hit the gap in a first session: `high`, `medium`, `low`.
- Counts come from the scripts in `review/coverage-content/scripts/` and the overlay tests in
  `review/coverage-content/overlay/` (re-runnable: see the README at the end). Raw lists are in the same folder.

## 0. The engine, in the five facts that decide most rows

These are the mechanisms the rest of the report keeps pointing at. Each was read in code.

1. **A cast computes five things and nothing else.** `CastSpell` (`backend/internal/play/combat_spells.go:192`) spends
   the slot and the action, sets concentration, and then, for each target, runs one of: a spell attack roll with the
   first damage type (`:598`), a saving throw with full/half/no damage (`:636`), Magic Missile's darts (`:670`), a
   pending heal (`:560`+), or one of 12 hit-point effects (`play/combat_spells_hp.go:91`, data in
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
