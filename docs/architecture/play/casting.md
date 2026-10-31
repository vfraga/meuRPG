# Casting

In combat a spell is `CombatService.CastSpell` (MR-014); between fights it is `CastingService` (MR-048, RN-32).
Both spend the slot and the action in the same transaction as the effect, so a resend with the same key spends nothing and a refused cast does not either.
Summoning spells are described in [Creatures](creatures.md#casting-summons).

## Casting in combat

`CastSpell` spends the slot and the action (or bonus action) on casting.
Who casts: the player on their own character on their turn (a prepared spell or a save cantrip from the options list); the master on any combatant on turn (an NPC with a full sheet casts from the sheet; NPC slots are not counted, only player characters').
The spell, the level (`slot`: level or pact, none for a cantrip) and the targets are checked against the character's `TurnOptions`; the targets against what they see, the range and how many targets the spell accepts:

- **Range.** Self only the caster, Touch 5 ft, otherwise the spell's range; multiplied for a spell attack when the caster's table feat has a `ranged_attack` with `range_multiplier`.
  The master is never limited.
- **How many targets.** `SpellDetails.Target` says whether a spell hits an area and how many it takes: what the master wrote on a table spell, the 5e-database's structured area on an SRD one and, for one without a structured area, the SRD text read with two patterns (`rules/spelltarget.go`).
  A spell attack takes one target, so a wrong guess never blocks the master.
  A table spell goes through the same `CombatSpell`, with the campaign's content (see [Architecture](../characters-table-content.md#spell-targets-and-the-magias-page)).
- **Armor without proficiency.** A player's character that wears armor or carries a shield it is not proficient with (`rules.ArmorUntrained`, read from the derived sheet; `armorTrainingOf` in `characters` drops it for an NPC, a creature and a beast form, which are proficient with what they wear) casts nothing: `CombatTurnOptions` disables every spell, and every cantrip with an attack roll, with `ARMOR_NOT_PROFICIENT` and no slot, so `CastSpell`, `CastSummon` and the reaction spells (which read the slots of the options) refuse it, and `RollAttack` refuses a spell attack (`EncounterBlocked`, `ARMOR_NOT_PROFICIENT`, nothing spent).
  The master is refused too.
- **Scorching Ray's rays.** The rays (3 at the spell's level, one more per level above) go to the targets as the caster shares them out, all at one creature if they want: each target's `darts` is the rays it gets (none counts as one), and the rays together are at most what the slot makes (the master's at most 10).
  `GetTurnOptions` lists the rays of each slot level in `darts` (`DartsAtSlot`), beside Magic Missile's darts; unlike Magic Missile's, the rays need not add up exactly, and a listed target with no `darts` gets one ray.
  Each ray is its own spell attack with its own d20 and its own hit, miss or critical, an entry of its own in the cast's `targets` (the same combatant can appear more than once, in the order of the request and the rays of one target together; `darts` is 0 on each), and a hit opens its own pending damage, 2d6 fire (doubled dice on a critical), with no `cast_id`: each ray's damage is rolled and lands by itself, so concentration saves and damage steps run once per ray.
  A typed `d20_face` fits one ray only (`roll_in_app` for several), and a cast with more than one ray is not held for a Bardic Inspiration die or Lucky (see [Held rolls](held-rolls.md)).
- **At most 10 listed targets**, the master's too: what each target suffered stays in the cast's event, and a `session_events` payload is at most 16 KiB.
  A placed area spell is not listed: it reaches who the server finds inside, up to the 40 combatants a combat holds.
  Past 10 listed is `invalid_argument`.
- **Roll mode of a spell attack.** `GetTurnOptions` writes the mode, sources and `critical_on_hit` on each target of a spell attack, as on a weapon attack (see [Attacks and damage](attacks-and-damage.md#roll-modes)).
- **Held rolls.** A leveled spell attack against one target, an effect's save and a concentration save may wait for Lucky or a Bardic Inspiration die (see [Held rolls](held-rolls.md)).
  A cast held by a reaction window (Counterspell) is replayed with the stored request.

What the casting does depends on what the spell is:

| The spell is | What the casting does | What is left for `RollDamage` |
| --- | --- | --- |
| Spell attack (Ray of Frost, with `attack_type`) | One d20 per target, or per ray for Scorching Ray (`roll_in_app`, or `d20_face` for a single attack, RN-18 on each roll), against the AC as in `RollAttack`; a hit opens the pending damage, and Shield can hold it; a spell of the `half_on_miss` kind (Acid Arrow) opens half of it on a miss, and its effect (`applies: hit`) takes hold only of the targets the attack hit | Each hit's damage |
| Save (Burning Hands, Sacred Flame) | The server rolls each target's save: a full-sheet NPC uses `Derived.SavingThrows`, a basic sheet made from a creature uses the stat block's saves; a basic sheet typed by hand has no bonus, rolls d20 + 0, and the master's log says so (`bonus_known` false; for a player it is always false and the app never reads it). A player character is rolled in the app with the sheet's bonus (the master's `amount` has the last word). Whoever passes takes half (rounded down) or nothing, as the spell says; a target with Evasion (`Traits.Evasion`, not incapacitated) takes nothing on a passed Dexterity save that halves and half on a failed one (`combat.HalfSaveShare`) | One roll for the whole casting for each damage type: the pending damages of a casting share a `cast_id`, and rolling one settles all of the same type. A spell with two damage types (Ice Storm, Flame Strike, Meteor Swarm) opens one pending damage per type per target (`pending_damage_id`, `more_pending_damage_ids`), each read against the target's resistance to its own type |
| Magic Missile | The darts (3, plus one per level above) are split among the targets, at least one in each, summing exactly; no attack roll | Each target's damage: 1d4 + 1 per dart |
| Healing | A pending damage that is a heal | The roll with the casting modifier heals at once, up to the maximum, per NPC and per character (through `character_vitals`); whoever was at 0 HP rises and the death saves reset |
| Reads HP (Sleep, Colour Spray, Power Word, Spare the Dying, Heal, False Life, Aid) | The server reads each target's HP and applies the effect at once, in the same transaction | None |
| Revivify | See [Death and revival](death-and-revival.md) | None |
| Teleport (Misty Step, with `destination`) | Spends the slot and the bonus action, then puts the caster on the chosen square; see below | None |
| Zone spell (Web, Spirit Guardians, Moonbeam, Cloudkill, Grease, Entangle, or a table spell with a zone), cast at a point on the map | Spends and logs, lays the zone, and Web puts its effect on the creatures inside; Grease and Entangle make the creatures inside save at once (prone, restrained); the zone then makes the later saves and damage ([Spell zones](spell-zones.md)) | None at the cast; the zone's damage is pending damage of its own |
| The rest (Misty Step without a square, which only the master may send) | Spends and logs; the effect is the table's, and the master marks conditions afterwards | None |

**A teleport on a map.** `CastSpellRequest.destination` is the square the caster lands on (`teleportReachFt` has the spells and their reach: Misty Step, 30 ft).
`checkTeleport` runs with the other checks, before anything is spent: a square outside the grid or sent for another spell or in a combat without a map is `invalid_argument`; a square farther than the reach by `grid.RangeFt` is `TELEPORT_TOO_FAR`.
A wall, a creature (hidden ones too) and, for a player on a map with the fog, a square the caster does not see are all `DESTINATION_UNAVAILABLE` with the same message, so the answer never says that an unseen creature stands there (RN-10); the master is held to the reach and the free square, not to sight, and is told `WALL_ON_SQUARE` or `SQUARE_OCCUPIED`.
The cast then writes the square with no movement spent, no opportunity offer and the master's cover mark cleared, keeps where the caster stood in the event (`teleport`), and publishes the move and the fog as a move does.
A cast held by a reaction window checks the square again when it replays, and a countered one moves nobody.
The turn options tell the screen which spell teleports (`SpellTargets.teleport_range_ft`, 0 without a map).
On the web the cast sheet shows `app-teleport-map` (`shared/area-picker/teleport-map.ts`) when that field is above 0: the battle map in the "Mover" page's pick mode, with the reach as the dashed circle, the squares a player can use tinted, and a live region that says each square (column, row, distance, picked or why not). `core/combat/teleport-pick.ts` judges a square the way the server does (reach, wall, a creature the player sees, the fog), so the words match `TELEPORT_TOO_FAR` ("Longe demais") and `DESTINATION_UNAVAILABLE` ("Esse espaço não está disponível"); the master is not held locally and may skip. "Conjurar" stays off until a player has picked a square, and the square goes in `destination`.
A player's character must send `destination` on a map with a grid: without it `requireDestination` refuses the cast with `DESTINATION_REQUIRED` before anything is spent. The master may leave it out (and does for an NPC): the cast then changes nothing on the map and the master moves the token with a forced move.
A trap armed on the landing square fires through the move's own hook (`trapHook.fire`, after the teleport is written): the cast's event is written first and the firing is the change's own event, carrying the cast's fields so a retry under the same key still answers the cast. An undo takes the firing back, then the cast.

**What a casting rolls at once and what stays with the master (RN-22).** A casting opens the saving throw and damage the spell asks for when it is cast.
Saves that come later, when a creature enters the area or begins its turn in it, are automated for the spells that keep a zone on the map (Web, Spirit Guardians, Moonbeam, Cloudkill, Grease, Entangle, and a table spell with a zone), when the cast chose a point: see [Spell zones](spell-zones.md).
For the others (Flaming Sphere, Sleet Storm, Earthquake) they are not: the data records their dice without a saving throw, so a casting spends the slot, logs and rolls nothing, and the master calls for each save as creatures move.
A lasting effect with an end-of-turn save uses the reaction window (see [Lasting effects](lasting-effects.md)).

**Damage types a spell lets the caster pick.** A spell that lists more than one damage type either deals them all (Ice Storm, Meteor Swarm, Flame Strike) or lets the caster pick: `damage_type_key` of `CastSpellRequest` names the type, and `SpellDetails.damage_choice` says how.
With `scale` (Flame Strike) every type is dealt and the dice a higher slot adds go to the picked one; with `alternative` (Spirit Guardians) only the picked type is dealt.
An empty pick takes the spell's first type; a type the spell does not offer is `invalid_argument`.
The cast sheet draws the pick as radio cards (`damage-type-picker`) whenever `damage_choice` is set, and a new pick is a new request with a new idempotency key.

**Cast options of table features.** The `spell_modifier` effects of the caster's features ([Rules: table content](../rules-table-content.md#spell-modifier)) change the damage of the spell cast (`combat_spell_options.go`). `GetTurnOptions` lists, for each spell, the options that fit it at its own level (`SpellOption.table_options`: name, change, whether it is always on, the resource and its uses left, the damage types a swap offers). The cast works them out on `link.Spell` right after the spell is read at its slot (`applyTableOptions`), so the damage rolls as the spell's own, one pending damage for each damage type:

- always-on options (no resource, not a swap) apply by themselves; the others are named in `CastSpellRequest.spell_options` (a swap names the new type in `damage_type_key`, one of the types the option offers) and an option the spell does not fit, one ticked twice or a bad pick is `invalid_argument`;
- swaps apply first, so a bonus damage with no type of its own follows the new type; a bonus damage adds to the spell's part of that type (or is a part of its own), and the extra dice go to the first part with dice (the part of the named damage type for an `applies_to` that is one);
- each ticked option with a resource spends one use with the slot (`spendTableOptions`; no use left is `NO_USES` and the cast spends nothing); the uses are in the event (`actionEvent.TableUses`) and the master's undo gives them back;
- a spell attack opens a pending damage for each part, the first as the hit's and the rest with it.

Options apply to casts (`CastSpell`); the attack cantrips rolled by `RollAttack`, and the darts of Magic Missile, do not read them.

**The bonus action spell limit.** `combatants.spell_cast` and `combatants.bonus_spell_cast` say which kinds of spell the combatant cast this turn (a cantrip with a casting time of 1 action counts as neither); `ResetCombatantTurn` clears both and the undo of a cast puts them back.
A spell is refused with `BONUS_ACTION_SPELL_LIMIT` when a bonus action spell was cast and this is not that cantrip, or when this is a bonus action spell and another one was cast.
Reactions (Shield) do not count.
The disabled spell row carries a `DisabledReason` that says which side blocks it (`castError` turns it into the refusal).

**Concentration.** A concentration spell puts its name in `concentration_spell`; casting another swaps it, and the log says which ended (see [Attacks and damage](attacks-and-damage.md#conditions-and-concentration)).
The cast sheets show a damage cantrip's dice from `Attack.spell_dice`, never from the first row of the spell's table.

## Spells that read HP

Six SRD spells roll no damage because what they do depends on the HP of whoever is in the area, and two more (False Life and Aid) change hit points without being damage or healing (MR-014).
What each does is handwritten content in `rules/srd51/effects/spells.json`, in a closed set of types the loader checks; changing the file needs a new revision (see [Contributing](../../rules-content.md#changing-an-effect-a-name-or-a-text)).
`rules.SpellEffect(key, level)` returns the effect already at the slot's level.
An `hp_pool` takes plain dice (no bonus, at most 30) of a die that exists at a table (d4, d6, d8, d10, d12, d20, d100).

| Type | What it does | Spells |
| --- | --- | --- |
| `hp_pool` | Rolls a total of dice (plus extra dice per level above the spell's) and goes through the creatures in ascending current HP, skipping those already unconscious or at 0; each one whose HP fits in what is left gets the condition, and the total goes down by its HP | Sleep (5d8, +2d8, Unconscious), Colour Spray (6d10, +2d10, Blinded) |
| `hp_threshold` | A creature with HP equal to or less than N suffers the effect: a condition or death | Power Word Stun (150, Stunned), Power Word Kill (100, dies) |
| `zero_hp_target` | Only holds on a creature at 0 HP, and leaves it stable | Spare the Dying |
| `flat_heal` | Heals a fixed value (plus an amount per level) and ends conditions | Heal (70, +10 per level; ends Blinded and Deafened) |
| `temp_hp` | The targets gain temporary hit points: a die rolled (`dice`, in the app or typed with `pool_sum`) plus `amount`, and `amount_per_level` more for each level above. Not healing; temporary HP do not stack, so a target keeps the larger | False Life (1d4 + 4, +5 per level) |
| `max_hp` | The targets' maximum and current HP both rise by `amount` (plus `amount_per_level`). The bonus is kept (`character_vitals.hit_points_max_bonus` for a player character, `combatants.hp_max_bonus` for an NPC or creature) until the master ends it (`EndCombatEffect`; for a player's character a long rest also clears it). Two casts keep the stronger bonus; a character at 0 HP gets up with the new current HP and its death saves reset; an NPC or creature at 0 stays down; the undo puts bonus, HP and maximum back. No temporary HP. `SpellEffectResult.gain` says `MAXIMUM` or `CURRENT` to everyone who sees the line; `hit_points_after` and `hit_points_max_after` go to the master and the target's player | Aid (5, +5 per level; up to three creatures) |
| `ignores_cover` | No effect on HP; the spell's saving throw gets no cover bonus | Sacred Flame |
| `half_on_miss` | No effect on HP; a spell attack that misses still opens a pending damage of half the first damage (`pending.half`, rounded down when rolled), `openSplash` | Acid Arrow |
| `revive` | Brings a dead character back (see [Death and revival](death-and-revival.md)) | Revivify |
| `summon` | Creatures that join the group (see [Creatures](creatures.md#casting-summons)) | Find Familiar, Animate Dead, Conjure Animals |

The arithmetic is pure, in `rules/combat/hpspells.go` (`ResolvePool`, `ResolveThreshold`, `ResolveZeroHP`, `ResolveFlatHeal`): ties keep the targets' order and the limit holds with HP exactly equal to N.
In `play`, `combat_spells_hp.go` reads the real HP and applies the result: the condition through the same field `SetCombatantConditions` writes; "death" defeats an NPC (0 HP) and takes a player character to 0 HP with three death-save failures, for the master to confirm (`ConfirmDeath`, RN-03); a character at 0 HP under Spare the Dying becomes stable.
The pool's total is rolled on the server (`roll_in_app`) or typed (`pool_sum`) and follows the campaign's dice mode (RN-18).
The cast event stores, per target, the HP before, what was affected and what the undo needs.
A druid in a beast form has the beast's hit points for these spells: Sleep and the Power Words judge the beast's pool, and Power Word Kill takes the beast to 0 (the form ends, nothing carries over).

**Who receives what (RN-20).** `SpellCast` and `CombatLogSpell` carry typed fields; the app never reads text.
`effect_kind`, `effect_condition_key` and, per target, `SpellEffectResult.outcome` go to everyone who sees the line; `pool_roll` only to the master and the caster's player; `effect_threshold` and per target `reason`, `hit_points_before`, `pool_order` and `pool_left` only to the master; `healed` to the master and the target's player (on an NPC the amount would tell how many HP were missing).
A heal appears in the log with the recovered amount only for the master and the target; others receive the roll.

## Casting outside combat

`CastingService` (`casting.proto`) casts spells between fights.
The rules are in `rules/casting.go`, `rules/combat/outside.go` and `healing.go`; `play` (`casting*.go`) holds the state, takes the open session's row lock like every write, and publishes after `COMMIT`.
`characters` reads the sheet for it (`castingoutside.go`: the spells the character can cast and Mage Armor's armor class) and hands the answer over through `link` types.
One row of `spell_casts` is one casting, from start to end, and the rows of a session are also its log.
Armor or a shield without proficiency refuses the cast before anything else (`CastingBlocked`, `ARMOR_NOT_PROFICIENT`), at the start and at the end of a long casting, and `GetCastOptions.armor_not_proficient` says it so the screen lists no spell.

```mermaid
stateDiagram-v2
    [*] --> casting : takes minutes or hours, or a ritual
    [*] --> active : cast at once, and it lasts
    [*] --> ended : cast at once, instantaneous
    casting --> active : ConfirmCastTimePassed, it lasts
    casting --> ended : ConfirmCastTimePassed, instantaneous
    casting --> failed : AbandonCast or concentration broken
    active --> ended : EndActiveSpell, concentration, rest, caster gone
    ended --> [*]
    failed --> [*]
```

| Call | Who may | Typed refusals (`CastingBlocked.reason`, `failed_precondition`) |
| --- | --- | --- |
| `GetCastOptions` | The character's player; the master for any character with a full sheet | None of its own: lists each spell with its reason (`NO_SLOT`) and `in_combat` |
| `CastSpellOutsideCombat` | The character's player; the master for any, an NPC with a full sheet included | `IN_COMBAT`, `CAST_IN_PROGRESS`, `NO_SLOT`, `TARGET_WEARS_ARMOR`, `TARGET_OUT_OF_REACH`, `CLASS_CANNOT_RITUAL`, `NOT_A_RITUAL`, `NO_USES` |
| `ConfirmCastTimePassed` | The master only | `CAST_NOT_GOING`, `NO_SLOT`, `IN_COMBAT` |
| `AbandonCast` | The caster's player; the master | None. A cast that is not going again changes nothing |
| `EndActiveSpell` | The caster's player; the master | `CAST_NOT_ACTIVE` (still being cast: use `AbandonCast`) |
| `ListSpellCasts` | Campaign members, each reading what RN-10 and RN-20 allow | None |

Every call also answers `NO_OPEN_SESSION`, `unauthenticated` and `not_found` for a campaign the caller is not in.
The slot, targets and dice are checked by the server alone: `invalid_argument` covers a spell that is not the caster's, a slot that does not fit, a ritual with a slot and a missing roll.

- **Granted spells.** A spell a `granted_spell` effect gives ([Table content](../rules-table-content.md#granted-spells)) is one more way to cast it: `GetCastOptions` lists each grant on the spell (`CastingSpell.grants`: source, uses left, ability, attack bonus and DC), and a spell the character has from a grant alone is listed too.
  `granted_by` names the grant: the cast takes no slot (a slot only when the grant has `also_with_slot`, and then no use), spends one use of the grant's resource unless it is `at_will` (`NO_USES` when none is left), and reads the spell with the grant's casting ability.
  A ritual-only grant takes `as_ritual`.
  The use is spent when the cast starts, also for a long casting; the end of a long casting through a grant reads the sheet again for the way the row says.
  In a combat the same grant is a turn option (`SpellOption.grant`) and `CastSpellRequest.granted_by` picks it; the undo gives the use back.
- **One cast, two steps when it takes time.** An action, bonus action or reaction takes effect at once.
  A spell of a minute or more, and every ritual (casting time plus 10 minutes, no slot, no higher level: SRD 5.1 "Rituals"), starts in `casting`: nothing is spent and the caster concentrates on the casting from the start.
  The master says the time has passed and calls `ConfirmCastTimePassed`, which spends the slot and applies the effect.
  The app keeps no clock.
  If the concentration breaks, the cast fails (`interrupted`) and no slot is spent.
  The unique partial indexes make it a fact of the table: one `casting` row and one concentrating row per caster.
  Casting a second concentration spell ends the first and the creatures it summoned (`ended_cast_ids`, `dismissed_creature_ids`).
- **Rituals follow the class.** `rules/casting.go` allows a ritual to a bard (spells known), a cleric or druid (prepared) and a wizard (the spellbook).
  The warlock's Pact of the Chain casts only Find Familiar that way.
  The Book of Ancient Secrets invocation is not implemented.
- **A combat that starts does not cancel a long cast.** The combatant carries the concentration (`spell_casts.carried_encounter_id`, given back when the combat ends), the caster spends the action each turn, and the cast can be confirmed only outside combat (`IN_COMBAT`).
  `combatants.mage_armor_ac` copies Mage Armor's armor class when a combat starts or the character joins.

What the server applies when the spell takes effect:

| Spell kind | Effect |
| --- | --- |
| Healing | One roll for the whole cast (server roll or typed sum, RN-18), the slot's level and casting modifier, up to each target's maximum; the Life Domain adds Disciple of Life, Blessed Healer and Supreme Healing |
| False Life | Temporary hit points; the larger value stays |
| Mage Armor | Base AC 13 + Dexterity for a target with no armor; a target in armor is refused (`TARGET_WEARS_ARMOR`). Ending the spell takes it back |
| Summons | The same creatures as `CastSummon`, with the same concentration rule |
| Aid | The maximum and the current HP of each player character it reaches rise by the spell's amount for the slot (`maxHPTargets`, through the same `raiseCharacterMaxHP` as in a combat); a character at 0 HP gets up. Casts of Aid do not add: the stronger bonus applies. Ending the cast, a long rest or an interrupted cast gives the bonus back (`takeBackMaxHP`): the maximum falls to the strongest Aid still on the target, and the current HP lose only what is above it, never the last one |
| Anything else | Recorded in the log; the master narrates |

An NPC has no hit points or slots outside combat, so a cast on an NPC changes nothing about it and an NPC caster spends no slot.
Reach is measured on the session's current map (touch 5 ft, ranged the spell's distance, 5 ft a square); without a map, grid or token the master judges and nothing is refused (`TARGET_OUT_OF_REACH` carries `missing_ft`); the master is never held to reach.

**How a lasting spell ends.** The master or the caster ends it (`EndActiveSpell`, `dismissed`), the caster's concentration moves on or stops (`concentration`), the caster is gone (`caster_gone`), or a rest covers the whole duration: a short rest is 1 hour, a long rest 8 hours.
Durations are game time, never the wall clock.
`endSpellsAtRest` is the seam the master's rest calls.
After damage outside combat, the master's HP correction returns the DC (10 or half the damage) in `AdjustCharacterVitalsResponse.concentration_dc` and `concentration_cast_id`; 0 HP ends the concentration.
The same correction of a character who is a combatant of the open combat, taken to 0 HP, ends the combatant's concentration with the end a damage to 0 HP has (`stopConcentrating`: the spell's effects and creatures, and the log line); it writes no pending damage and no save, and it is nothing for an undo to take back. The same end follows an NPC the master takes to 0 HP with `AdjustCombatantHitPoints`, and that adjustment's undo gives the concentration back.

**Who sees what (RN-10, RN-20).** The master reads every cast.
A player names an NPC on the stage by its place on it (`StageNpc.id`), never by the character id, in the targets they send and in everything they read.
A cast by an NPC that is not on the stage is stored with `secret = true` and its hint goes to the master only.
What a cast did to a target (`effect`, `amount`, `extra_hit_points`, HP before and after, armor class) goes only to the master and the target's player; everyone else reads the dice and the spell (`TestAHealingCastShowsItsEffectOnlyToTheTargetAndTheMaster`).
The stream hint is `spell_casts_changed` (contentless); the caster and player targets also get `vitals_changed`.
The history gets `spell_cast_outside`, `spell_cast_started`, `spell_cast_finished`, `spell_cast_interrupted` and `spell_cast_ended`, with ids and numbers only.

**In the app.** The session page holds `casting-panel` (the caster's "Conjurar", the cast going on, the spells that last with "Encerrar", the log; for the master the queue with "Concluir conjuração", a note that outside a combat there is no clock, and "Conjurar como NPC") and `cast-out-sheet` (the spell, the way, the slot, the targets, the dice).
The panel reads `ListSpellCasts` again on each `spell_casts_changed` and on every `ready`.
Targets and reach come from `GetCastOptions`; the app holds no range, ritual or target-count rule.
Ending a concentration, or casting one over another, asks first in an alert dialog whose focus starts on "Cancelar".
A summoning spell asks for no target in `cast-out-sheet`: it opens the creature sheet (`SummonSheet`).

Tests: `go test -run 'TestCastingOutside|TestRN10_CastingOutside' ./internal/play/`.
