# Rules module: combat engine

`rules/combat` is the pure part of the fight (MR-012 to MR-014): what a character can do now, and the arithmetic of attack, damage, healing, slots, rolls, lasting effects and death saves.
Like `rules`, it uses no database, network, clock or randomness: dice are rolled outside (on the session server or by the player, RN-18) and enter as numbers, and every function returns a new value instead of changing its input.
The [play module](play/README.md) stores the state, reads the facts and calls these functions; the app writes the results in Portuguese.
For the engine around it see [Rules module](rules.md).

## What is in the package

| File | Subject |
| --- | --- |
| `turn.go`, `usage.go` | `Options` (the "Sua vez" menu), the turn economy, the reason codes, `Usage` (what is spent since the last rest). |
| `rolls.go`, `vitals.go` | Attack resolution, critical rules, damage dice, concentration DC, `ApplyDamage`, `ApplyHeal`, death saves, `SpendSlot`, `SpendPactSlot`, `SpendResource`. |
| `bonusattack.go` | Which rule lets an attack be made with the bonus action (off hand, Martial Arts, Flurry of Blows). |
| `advantage.go` | Advantage and disadvantage from circumstances (Pack Tactics and Magic Resistance among them); both cancel. The play module works out the facts and writes the sentence of each source. |
| `damagetype.go`, `steps.go` | Resistance, vulnerability and immunity; the order damage steps apply in (the flat weapon reduction first, once for the whole attack and largest type first (`Reduce`, `StepReduction`, `Origin.Weapon`), then resistance, then vulnerability, immunity last; several instances of the same count once). A resistance or immunity can be limited to nonmagical attacks (`Modifier.Qualifier`); `Origin` says whether a damage is an attack, whether it is magical and whether the weapon is silvered or adamantine (`Origin.Silvered`, `Origin.Adamantine`). |
| `extras.go` | What is added to a hit: the extras a player chooses (Sneak Attack, Divine Smite, Hunter's Mark, Colossus Slayer) with the condition that makes each valid, and the automatic lines (Rage, Improved Divine Smite, Great Weapon Fighting). |
| `healing.go` | What the Life Domain features add to a healing spell. |
| `hpspells.go` | The spells that read hit points (below). |
| `contest.go` | Contests and special actions: grapple and shove (size rules, escape DC, dragging), Hide, Help, group checks, surprise. |
| `effects.go` | Effects that last (RN-22): what a condition does, exhaustion levels, the turn clock (`EndsAtStartOf`), and modifiers read from lasting effects. |
| `jump.go` | `JumpLimits`, `HasRunningStart`: the reach of a jump from the sheet (the cost of the jump line is in `rules/grid`). |
| `outside.go` | `OutsideSpells`: what a character can cast outside a fight (rituals, long casts). |

## The functions

| Function | What it does |
| --- | --- |
| `Options(derived, TurnState, Usage)` | The `TurnOptions`: the economy (action, bonus action, reaction, each with `used` and `available`; movement in feet, doubled after Dash), the attacks, the spells, the standard actions and the features' actions, each option enabled or disabled with a reason code. Computed on every read, never stored. |
| `ResolveAttack(bonus, AC, d20)` | Hit or miss: a natural 20 always hits and is a critical, a natural 1 always misses. `ResolveAttackFrom` takes the lowest critical roll (19 with Improved Critical, 18 with Superior Critical; anything outside 2 to 20 counts as 20). |
| `BonusAttack(turn, next)`, `OffHandBonus(bonus, mod, style)` | Whether an attack may be made now with the bonus action and by which rule, and the off-hand damage bonus (without the ability modifier unless the style gives it). `Options` calls `BonusAttack` for every weapon attack once the Attack action has none left, so the options and `RollAttack` pick the same rule. |
| `DamageTotal`, `DiceToRoll`, `DiceRange` | Sum the rolled faces and the bonus; how many dice to roll; the valid range, to check a total typed with a physical die. A critical doubles the dice, never the bonus. |
| `CriticalDice(dice, critical, rule)` | How many dice to roll and what comes without rolling, under the table rule (RN-24): `CriticalDoubledDice` rolls twice the dice; `CriticalMaxPlusRoll` rolls the dice once and adds their maximum (`fixed`). The bonus enters once, in both. `play` uses it in every critical (combat and traps). |
| `ApplyDamage(hp, temporary, damage)` | Temporary hit points go first; floor at 0; says whether it "dropped to 0" and how much of the damage was left over. |
| `ApplyHeal(hp, maximum, heal)` | Heals up to the maximum and says how much it healed. |
| `SpendSlot`, `SpendPactSlot`, `SpendResource` | Spend a spell slot, a pact slot or a resource use; `ErrNoSlot` and `ErrNoUses` when none. They return a new `Usage`. |
| `AdjustForType`, `Adjust` | The damage that lands on a creature after the bonuses and the half of a save: immunity makes it 0, resistance halves it (rounded down), vulnerability doubles it. A damage with no type is not touched. `Adjust` also takes the `Origin` of the damage and applies the modifiers limited to nonmagical attacks only to a nonmagical attack (and, for the silvered and adamantine qualifiers, one whose weapon has not that material); `AdjustForType`, for a damage that is no attack, never applies them. |
| `DeathSave(d20, successes, failures)` | 10 or more is a success, less a failure, a natural 1 two failures, a natural 20 returns with 1 hit point. Three successes: stable (the three stay as the stable marker and the failures are cleared). Three failures: `dying`, which the master confirms (RN-03); the engine never kills by itself. `DamageWhileDown(critical)` gives 1 failure (2 on a critical) for damage to someone at 0. |
| `ConcentrationDC(damage)` | `max(10, damage/2)`, the number of the concentration reminder (RN-22). |
| `HalfSaveShare(creature, ability, saved)` | The share (`ShareFull`, `ShareHalf`, `ShareNone`) of a damage that a save halving it on a success leaves: Evasion makes a passed Dexterity save `ShareNone` and a failed one `ShareHalf`, unless the creature is incapacitated; otherwise a pass is half and a failure all. |
| `AttacksLeft`, `MissileDarts`, `SaveSucceeded`, `HalfDamage`, `ShieldACBonus` | How many attacks of the Attack action still fit (Extra Attack), Magic Missile's darts (3, plus one per level above 1st), whether a save reaches the DC, half the damage rounded down and Shield's +5. |

## Reason codes of a disabled option

The reasons are codes, never text; the app maps each to the Portuguese sentence.
The first reason that applies wins: what the spell is, then the economy, then the slots.

| Code | When | Parameters |
| --- | --- | --- |
| `ACTION_USED`, `BONUS_ACTION_USED`, `REACTION_USED` | The action, bonus action or reaction the option needs was already used. | none |
| `ATTACKS_USED` | The Attack action made all its attacks (Extra Attack); with a single attack the code is `ACTION_USED`. | none |
| `ATTACK_ACTION_FIRST` | Flurry of Blows before the Attack action was taken with a weapon or an unarmed strike (a cantrip does not count). | none |
| `ALREADY_USED_THIS_TURN` | A feature usable once per turn (Action Surge) was already used this turn. | none |
| `NO_SLOT` | A spell of level 1 or higher with no free slot at its level or above (nor pact). | `min_level`: the spell's level |
| `NO_USES` | The feature's resource ran out. | `recharge`: when it returns (`short_rest`, `long_rest`...) |
| `REACTION_ONLY_WHEN_HIT` | Shield: only cast when an attack hits the caster. | none |
| `REACTION_ONLY` | The other reaction spells (Counterspell, Feather Fall, Hellish Rebuke). | none |
| `BONUS_ACTION_SPELL_LIMIT` | A spell cast with a bonus action leaves no other spell for the turn, in either order, except a cantrip with a casting time of 1 action (SRD 5.1, Casting Time). The master may cast it anyway, like the economy. | none |
| `CASTING_TIME_TOO_LONG` | Casting time of 1 minute or more. | none |

**Spell order.**
`Options` returns the spells already sorted: first the ones that can be cast now, then the others; within each group cantrips first, then by level and by Portuguese name (without accents, case-insensitive; a tie keeps the sheet's order).
Shield can never be cast on the character's own turn, so it goes among the others and the screen shows "Só fora da sua vez".
Each spell carries the levels it can be cast at (`slots`: from its own level up, only with a free slot, plus the warlock's pact slot with how many are free).
Damage cantrips stay only in `attacks`, not in `spells`.

**Bonus action attacks and beams.**
`TurnOptions` is returned by `CombatService.GetTurnOptions`, with the economy including `attacks_per_action` and `attacks_left`.
Each `AttackOption` says whether the attack is a bonus action attack right now (`bonus_rule`: off hand, Martial Arts, Flurry of Blows, or unspecified for an attack of the Attack action).
The combatant's `action_attack_key` and `bonus_attacks_left` feed it.
Such an attack is enabled even with `attacks_left` 0 and has its `attack_targets`; Extra Attack's attacks come first, so `bonus_rule` stays unspecified while one is left.
The off-hand option carries the damage `RollAttack` will roll (`bonus_drops_modifier` says the ability modifier was left out); during Flurry of Blows `bonus_attacks_left` counts the strikes down; when the bonus action is spent the option is disabled with `BONUS_ACTION_USED` and keeps its `bonus_rule`.
A cantrip cast with the action spends the whole Attack action in the options too: `attacks_left` is 0 and every weapon attack is disabled with `ACTION_USED`.
The cantrip with several beams (Eldritch Blast) is the exception: while the cast that spent the action has beams left, its option stays enabled with `beams_left` counting them, and every other attack and spell is disabled as the action's.
`TakeAction` refuses Flurry of Blows before the Attack action the same way (`ATTACK_ACTION_FIRST`, the master excepted) before it spends the ki point.

**Standard actions and extra attack.**
The ten actions everyone has (Attack, Cast a Spell, Dash, Disengage, Dodge, Help, Hide, Ready, Search, Use an Object) live in `effects/standard_actions.json`.
Action Surge is a free action (`grant_action` with `free`) tied to the `action_surge` resource.

## Spells that read hit points

`effects/spells.json` marks the SRD spells whose effect depends on the hit points creatures have now, in closed kinds; loading refuses anything else.
The kinds are `hp_pool`, `hp_threshold`, `zero_hp_target`, `flat_heal`, `temp_hp` and `max_hp`; the same file has `summon` (see [Creatures, items and presets](rules-creatures-items.md#summons)), `revive` (Revivify: hit points and a window of rounds) and `ignores_cover` (a spell whose saving throw gets no cover bonus; `Content.IgnoresCover`).
`combat/hpspells.go` resolves the numbers (`ResolvePool`, `ResolveThreshold`, `ResolveZeroHP`, `ResolveFlatHeal`) from the creatures' current hit points and the rolled dice; the play module reads the real hit points and applies the result.
A spell of this kind also needs its computation in `combat` and a test, and a spell outside SRD 5.1 never enters.
No table spell reads hit points or summons: the table's keys are not in the file.

## Spell details

`ListContent` stays light (name, spell level, school, classes, ritual, concentration, casting time).
Everything the SRD says about one spell comes from `ContentService.GetSpellDetails(campaign_id, spell_key)`, for the "?" dialog: casting time (quantity, unit and the reaction trigger when there is one), range (type and distance in feet), components (V, S, M and the material in both languages), duration (instantaneous, timed, until dispelled, special, and whether it needs concentration), attack, saving throw (ability and what happens on a pass), damage per slot level (and per character level for cantrips), healing per level, and the text with "At Higher Levels" in both languages.
The server returns structured data and the raw SRD text for what does not fit ("4d6 OR 5d6").
Writing "60 ft" as "18 m" and "1 action" as "1 ação" is the app's job (`spell-details-format.ts`, which falls back to the raw text, marked, when the value does not map).
In Go, `SpellDetails.DamageAt(level, level)` and `HealAt(level)` give the rolls as numbers (`ParseDice`) for the combat code.
The Portuguese texts are described in [Rules module](rules.md#portuguese-texts-of-the-srd).

## Reactions

`rules/reaction` is the pure part of the reaction window (PM-04): the numbers of each reaction as SRD 5.1 gives them (Counterspell, Uncanny Dodge, Deflect Missiles, Hellish Rebuke, Cutting Words, Feather Fall, Shield), the rule that closes a window by itself (`Closure`), the order windows are answered in, and the sentence the combat shows while one waits (`Detail`).
It reads no database; the play module keeps the windows.
