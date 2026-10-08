You are tagging D&D 5e (SRD 5.1) spells for a code audit. Read ONE input file (a JSON array of spells; each has key, name, level, casting_time, duration, concentration, desc, higher_level). Do not read any other file and do not touch the repository code.

For EVERY spell in the file write one JSON object per line (JSONL) to the OUTPUT file named in your task. One line per spell, no omissions, no extra text in the file. Fields:

- "key": copied exactly.
- "kind": ONE of: "attack_damage" (spell attack roll, damage), "save_damage" (target saves, damage), "auto_damage" (damage with no roll), "heal", "condition" (main point is imposing a condition or control), "buff" (helps allies: AC, bonuses, advantage, resistance, speed, temp HP...), "debuff" (hinders enemies without a condition name), "zone" (persistent area / wall / object on the map), "summon" (creates or calls a creature), "utility" (information, travel, creation, communication, out-of-combat), "other".
- "conditions": array of SRD condition names the text can IMPOSE on a creature, lowercase, from: blinded, charmed, deafened, frightened, grappled, incapacitated, invisible, paralyzed, petrified, poisoned, prone, restrained, stunned, unconscious, exhaustion. Only conditions the spell inflicts or grants, not ones it merely mentions as immunity or something it ends. [] if none.
- "needs": array of zero or more tags from this CLOSED list, for mechanics the text asks for besides the first hit/save/damage/heal:
  "repeat_save" (a later saving throw: end of the target's turn, when it takes damage, each turn it repeats the action...),
  "ongoing_damage" (damage again at the start/end of a turn, when entering or staying in an area, or on a following turn),
  "movement" (pushes, pulls, teleports, changes speed, grants flying/climbing/swimming, difficult terrain, forced moves),
  "stat_change" (a bonus or penalty to AC, attacks, saves, checks, speed, HP maximum, an advantage/disadvantage, a resistance, extra damage dice on the target's attacks, a changed ability score),
  "zone" (an area or object that stays on the map after the cast: wall, cloud, fog, light, sphere, glyph),
  "rider" (a second effect on a hit or failed save that is not plain damage: extra effect, blindness on a hit, etc.),
  "duration_timer" (the effect lasts a number of rounds/minutes/hours and must end by itself),
  "choice" (the caster chooses among options at cast time beyond target and damage type),
  "creates_creature_or_object" (conjures something other than a plain damage effect),
  "reaction" (it is cast as a reaction to a trigger),
  "upcast_extra" (casting with a higher slot adds something other than more damage dice, more healing or more targets),
  "ritual_or_long_cast" (casting time of 1 minute or more),
  "no_mechanics" (nothing a combat engine could compute; pure narration).
- "evidence": quote, in at most 12 words, the part of the description that supports the tags that are not obvious.
- "note": at most 15 words, in English, saying what the spell does mechanically.

Rules: judge only from the text given. Do not invent conditions. When unsure between two tags, include both. Be fast and literal; do not explain. When the file is written, reply with ONE line: the count of lines written and the output path.
