# Spell batches: addendum to brief.md

Your entries are every spell of backend/internal/rules/srd51/data/spells.json at your spell level whose "classes" contains class:wizard, class:warlock or class:druid. Generate the list with python from the file; do not skip any. "list" = the matching classes among wizard/warlock/druid, comma-separated; "level" = the spell level.

Order of work:
1. First every spell with an attack, a save, damage or healing, a condition, a lasting effect, a zone (an area that stays) or a summon: there the app acts by itself and can be wrong. Check, against the SRD desc and higher_level: the attack type or the save ability, the damage dice and type, the upcast scaling (at_slot_level / cantrip scaling by character level at 5, 11, 17), half on a save or not, the condition and how it ends (repeat save at end of turn etc.), the duration and concentration, the targets/area (backend/internal/rules/srd51/effects/spell_targets.json and the structured area), the range, and what happens on the map for a zone (docs/product/rules.md RN-23 "Area spells on the grid", RN-37 "Areas that spells keep on the map", RN-22 "Effects that last").
2. Then the rest, where the usual verdict is quick: the app rolls nothing; does the cast screen or the log tell the table what to apply by hand (a reminder text, e.g. in spells_pt.json or a note), or only the spell's own description?

Where to look: backend/internal/rules/srd51/effects/spells.json (special kinds: hp_pool, half_on_miss, temp hp...), spell_targets.json, spells_pt.json, combat_effects.json, corrections.json; backend/internal/rules/casting.go and neighbours (grep "spell:"), backend/internal/play/ (cast handling, saves, zones, summons, conditions), web/src for the cast screen. Find the generic path a spell takes first (how damage/save/attack are derived from the structured data), then judge each spell against it; a spell that only follows the generic path still needs its own row.

Throwaway pure tests of the rules package are welcome to settle a number (e.g. the damage the app derives for a spell at a slot level).
