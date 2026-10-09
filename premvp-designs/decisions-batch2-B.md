# Decisions on Designer B's open questions (PM-05, PM-08), 08/10

1. **Favored Enemy follows SRD 5.1,** which is also the official 2014 PHB: creature types (or two humanoid races), with
   an extra language. The PDF's version (5 types, +2 damage) is the 2016 playtest's Revised Ranger. If the table plays
   that ranger, it comes as table content in a pack, not as the built-in class.
2. **The creature type names** use the labels the app already shows in the bestiary. Where none exists, use the PHB's
   term and add the entry to `names_pt.json` in the implementation (`creature-type:<key>`).
3. **A locked sheet can complete the half-elf's "+1 em duas habilidades".** It is an open choice the creation skipped.
   Modifiers, and HP when Constitution changes, are recomputed, and the master gets the log line.
4. **The multiclass prerequisites** (SRD "Multiclassing"): the main ability of **every** current class and of the new
   one. A player is refused both at level-up and at creation; the master's editor may override, with a warning. Taking
   a new class at level-up is the guided level-up's step, as drawn.
5. **"Recusar"** follows `docs/design.md` (danger-outline confirmation, focus on "Cancelar") on today's screen too.
6. **Hit dice are tracked per die type** (SRD Multiclassing, "Hit Points and Hit Dice"): a new field and a migration, in
   this stage. Short rests spend them by type.
7. **Revived mid-combat:** the combatant keeps its place in the initiative order and acts on its next turn. There is no
   "no turn this round" rule; the SRD has none.
8. **"Reviver" is blocked** while the player already has another living character in the campaign (RN-03). The master
   archives or retires the other one first, and the refusal says so.
9. **Revivificar's diamonds (300 PO):** a reminder the caster ticks; the app does not track the inventory yet. The
   spell's limits (death of old age, missing body parts) are in its text, and the master decides.
10. **The dead character's player** keeps the party's view, the map included, as decided for the fix wave.
