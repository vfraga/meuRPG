# Rules module: table content (`Content.With`)

The content a master registers enters the engine as a layer over the SRD (MR-025, RN-23, ADR-0018): `(*Content).With(Overlay)` returns a new `*Content`, the SRD plus the table's entries, and never changes what it received.
`rules` stays pure; who stores the entries is [Characters](characters-table-content.md), and the editors are in the web app.
The code is in `rules/overlay*.go`.
For the engine around it see [Rules module](rules.md).

## The overlay

| Piece | What it does |
| --- | --- |
| `Overlay` | The revision, the keys switched off, the entries of ten kinds (`TableClass`, `TableSubclass`, `TableRace`, `TableSubrace`, `TableBackground`, `TableSpell`, `TableFeat`, `TableOption`, `TableClassFeature`, `TableCreature`) and `Strict`, the keys being written now. Each entry starts with a `TableEntry` (key, Portuguese name, `Archived`, the revision and date of its last change) and carries the features (`TableFeature`: key, name, text, effects). The fields follow what the editors fill in, so the protos map one to one. |
| Keys | Every table key is `<kind>:<name>@mesa`, the name 1 to 60 characters of `[a-z0-9-]`; a feature's key starts `feature:`, `trait:` or `background-feature:`. `With` refuses a key without `@mesa`, of another kind, malformed or already taken (even by another entry's feature). Because the prefix is the SRD's, every place that decides by prefix works for table keys. |
| Reserved words | `With` refuses (`reserved_key`) a key, of an entry or a feature, with `magical-secrets`, `wild-shape` or `ability-score-improvement` in the name, or ending in `-spellcasting`, because the engine, `play` and the web react to those words in SRD keys. The keys the engine writes carry the kind (`feature:class-<name>-spellcasting@mesa`, `feature:subclass-<name>-spellcasting@mesa`, `feature:class-<name>-ability-score-improvement-<n>@mesa`), so a class and a subclass of the same name never collide. |
| Copy-on-write | The SRD is a singleton many requests read at once. `With` copies (shallowly) only the maps it changes, adds the entries, rebuilds the indexes and the catalogue, and leaves the values shared. A table subclass of an SRD class enters a copy of the class with the new subclass list; the same for a race's subraces. Two contents made from the same SRD do not see each other, and a `Content` that already has a table layer does not accept another. |
| Same loaders | Formulas go through the usual compiler (`classLevel("cavaleiro-runico@mesa")` works) and effects through the SRD's `compileEffect`; a repeated formula is compiled once per `With`. |
| Archived | An `Archived` entry keeps holding everywhere (`Derive`, `Validate`, names, level-up); the catalogue marks it. Refusing an archived key as a new choice needs the earlier sheet, so it belongs to the server: `Content.ArchivedKeys(build)` lists the archived keys a sheet uses and `TableKeys(build)` the `@mesa` keys. |
| Switched off | `Overlay.Off` carries the keys the master switched off for players. See [Switches](#switches-and-hidden-keys). |
| Missing key | A sheet that cites an `@mesa` key the content lacks opens with an `unknown_key` `Issue`; nothing panics. `Validate` refuses it like any unknown key. |
| Version | `srd51@<commit>+fx.<n>+mesa.<revision>`. The SRD part does not change; the revision is per campaign, so a content cache is indexed by campaign and version, never the version alone. |
| Errors | `*OverlayError{Key, Field, Reason, Message}`. `Field` is the path in the entry as the caller sent it (`classes[2].levels[4].features[0].effects[1].formula`); `With` sorts entries by key so the result does not depend on order, but the indexes are the caller's. `Reason` is a stable code: `limit`, `bad_key`, `duplicate_key`, `reserved_key`, `bad_name`, `bad_text`, `dangling_reference`, `forbidden_effect`, `bad_formula`, `bad_table`, `bad_casting`, `bad_value`, `overlay`. The server turns each into a field violation and the web into Portuguese. `OverlayError.More` carries the rest of an entry's violations, so a class reports every one at once. |

## Limits

All checked in the first pass, before anything compiles; the error names the limit.

| What | Limit |
| --- | --- |
| Entries (of the ten kinds; features do not count) | 1,000 (`MaxOverlayEntries`) |
| Features | 60 per class and 60 per subclass, counted apart (`MaxTableFeatures`); the features the engine writes do not count |
| Traits | 60 per race and per subrace (`MaxTraits`) |
| Options in a feature's option list | 60 (`MaxFeatureOptions`) |
| Effects | 20 per feature (`MaxFeatureEffects`); 6,000 in the set, the effects the engine writes included (`MaxOverlayEffects`) |
| Distinct formulas | 500 in the set, and 100 KiB of their text added up; a repeated one counts once (`MaxOverlayFormulas`, `MaxOverlayFormulaBytes`) |
| One formula | 1,024 bytes and 256 nodes (`formula.MaxSourceBytes`, `formula.MaxNodes`) |
| Items in a list of an effect | 20 |
| `choice` count | 1 to 18 (`MaxChoiceCount`) |
| Name | 80 characters; a name over it is `bad_name` |
| Text | 4,000 characters a paragraph (`bad_text`), 50 paragraphs (`limit`) |

The 64 KiB per entry is a storage limit, checked by the server.
Without these budgets, hundreds of classes of 60 features of 20 effects each cost seconds and gigabytes.
The ADR-0018 budget for a realistic content of 300 entries is about 25 ms and 4 MB per `With` (19 ms and 3.8 MB measured); a table of 1,000 entries takes 28 ms and 8.3 MB, and one with 6,000 effects 40 ms and 16.6 MB (76 ms and 33 MB with 480 distinct formulas), a content keeping 3.7 to 7 MB.
The numbers, the machine and the commands (`BenchmarkWithEntries`, `BenchmarkWithEffects`, `TestWithMemoryByEntries`, `TestWithMemoryByEffects`) are in [Performance](../performance.md#table-content-at-its-limits).
Cache and memory limits of the live source are in [Characters](characters-table-content.md#the-live-source).

## The closed menu of effects

A table feature has effects of these types only; anything else is `forbidden_effect`.

| Type | Notes |
| --- | --- |
| `modifier`, `proficiency`, `sense`, `roll_mode`, `extra_attack`, `note` | As in [Effects](rules.md#effects). A `modifier` on `score.<ability>` is an `add` with a `cap` from 20 to 30 (`MaxNormalScore` to `MaxScore`), and no other modifier has a cap. A score above its cap stays as it is. |
| `proficiency` with `only_proficient` | A switch of the effect with level `expertise` on a skill (`skill:*` is every skill): the skill is doubled only where the character already has its proficiency from another source when the sheet is worked out (the race, the background, the sheet's picks or another feature), never from the effect itself or from another `only_proficient` effect. Without the switch the effect doubles the skill, proficient or not. Any other level, a target that is not a skill and any other effect type refuse it at `.only_proficient` (`bad_value`). It is a `yes_no` field of the proficiency type in the effect menu. |
| `resource` | With uses and recharge. The resource may not have the name of an SRD one. |
| `grant_action` | May carry `resource`: the name of a `resource` effect of the same entry (a feat included), its class, or an SRD resource. Each use spends `cost` of it (1 when empty) and the action is off while fewer are left; see [Costs, healing actions and hooks](#costs-healing-actions-and-hooks). Refused as `bad_reference` when nothing defines the resource. |
| `grant_action` with `gives_slot` or `spends_slot` | `gives_slot` is an Int formula (1 to 9; a literal outside is refused at `.gives_slot`, and a result outside it removes the action): the action spends `cost` uses of its `resource` (required) and adds a free slot of that level until the next long rest; it takes no `cost_max`, `heal`, `temp_hp` or `value_per_point`. `spends_slot` makes the action spend a spell slot of the level the player picks at use (`TakeActionRequest.spend`, 1 to 9) and heal: `heal` at level 1 plus `value_per_point` (dice of the same sides) for each level above; it needs `heal` and takes no `resource`, `cost`, `cost_max` or `temp_hp`. Not both. `Derived.Actions` carries `GivesSlot` and `SpendsSlot` (`actionslots.go`), `Action.UnitCost` is 1 for a spent slot. |
| `wild_shape` | `max_cr` (a challenge rating, `0`, `1/8`, `1/4`, `1/2` or a whole number up to 30) or `max_cr_from` (an Int formula, such as `floor(level() / 3)`), not both; `no_fly` and `no_swim` with a rating; `form_cost` (an Int formula, the uses of `wild_shape` one change of form costs, 0 for a free change; empty is 1); `magical_attacks`. At least one of rating, cost or magical attacks. `Derived.WildShape` takes the highest rating (with the limits of the effect that gives it; a tie goes to the effect with fewer limits), the lowest cost and magical attacks if any effect gives them; the limits are those of the one effect with the highest rating. `WildShapeLimitFor`, `WildShapeForms` and `WildShapeAllows` read it, so the list and the check agree. A class without the SRD Wild Shape resource needs `form_cost: 0`: the change would spend a use it does not have. |
| `slot_recovery` | `resource` (a resource of the entry or its class, `bad_reference` otherwise), `recover_max_level` (an Int formula, the highest slot level, a literal 1 to 9) and `recover_levels` (an Int formula, the combined level, a literal of at least 1), all required; `when` and `text_pt` are allowed. `Derived.TableRecoveries` carries them for the character's level. `UseArcaneRecovery` with a `feature_key` spends a use of the resource and recovers slots within them (`TableSlotRecovery.Check`). A feature carries one `slot_recovery`: the call reads the first. |
| `on_event` | The owner's hook on `short_rest`, `long_rest`, `death_save_failed` or `drop_to_zero`; see [Costs, healing actions and hooks](#costs-healing-actions-and-hooks). |
| `ranged_attack` | Checked at its fields: `applies` is `spell` or `weapon`; `range_multiplier` is 2 or 3 and only on `spell`; `no_long_range_disadvantage` only on `weapon`; an effect that sets none of the three switches is refused at `.type`; `tags` at `.tags`; the switches on any other type are stray fields. |
| `resistance` | Resistance, immunity or vulnerability (`kind`: `resist`, the default, `immune` or `vulnerable`) to the `damage_types` it lists: `damage-type:` keys of the content, `all` (alone) or `nonmagical_bps` (bludgeoning, piercing and slashing from attacks that are not magical); or to every type but `except`. `except` and `damage_types` exclude each other and one is required. `while` is empty (always), `rage` or the `state_name` of a `state` effect of the same entry (`.while`: `bad_reference` for a state nothing defines). Refusals name the field: `.damage_types` (unknown type, empty, a type twice, `all` mixed with others, both `except` and `damage_types`), `.except` (`bad_reference`), `.kind`, `.while`, and `.when` or `.tags` (`bad_value`): `while` is the condition. See [Effects](rules.md#effects). |
| `state` | A temporary state the character turns on and off: a stance, a form. `state_name` (1 to 40 of `a-z`, `0-9`, `_`; the name the entry's other effects read it by; unique in the entry), `duration_rounds` (an Int formula in rounds, 0 to 14,400; empty or 0 lasts until it ends), `ends_on` (any of `dismiss`, `incapacitated`, `short_rest`, `long_rest`), `economy` (`action`, `bonus_action` or `free`, what turning it on costs) and optionally `resource` (as in `grant_action`). At most 8 states for an entry (`limit`). See [States](#states). |
| `weapon_rule` | Changes the weapon lines its filter picks. `weapon` is `any`, `melee`, `ranged`, `finesse`, `magic` or `named` (then `weapon_name` is the Portuguese name of the weapon); `attack_ability` and `damage_ability` name another ability (the better of it and the weapon's counts); `attack_bonus` and `damage_bonus` are -5 to 10; `reach_ft` (5 to 60, in steps of 5) is a longer melee reach. `magical`, `silvered` and `adamantine` mark the lines the filter picks for the resistances to nonmagical attacks (weapon_rule only; the `magic` filter reads only the sheet's own marks, `full.weapon_marks`, never a rule's nor the equipment's; the carried weapons and the unarmed strike are marked, a `pseudo_weapon` line is never marked by a rule). At least one ability, bonus, reach or mark must be set (refused at `.type`); `weapon_name` is required with `named` and refused otherwise; a reach is refused on `ranged`; no `tags`. Gated by `while` and `when`. |
| `pseudo_weapon` | An attack line of its own on the sheet: `attack_name`, `weapon` (`melee` or `ranged`), `damage_dice` (`NdS`, N up to 10, S a weapon die), `damage_type`, `ability`, `reach_ft` (melee, 5 to 60, optional) or `range_ft` (ranged, 5 to 300, required; each kind refuses the other's field). It is never an item, counts as a weapon the character is proficient with, and the weapon rules change it too. Gated by `while` and `when`. |
| `condition_immunity` | `conditions`: the keys of the conditions (`condition:poisoned`, one of the 15 of `combat_effects.json`), listed once each, at least one; refused at `.conditions`. The character takes none of them (see [Effects](rules.md#effects)). |
| `breathes_water` | The character breathes underwater (`Derived.BreathesWater`). No field of its own: a stray one is refused, `when` and `text_pt` are allowed. |
| `granted_spell` | See [Granted spells](#granted-spells). |
| `extra_damage` | See [Extra damage](#extra-damage). May carry `while`. |
| `choice` | Picks among SRD options (a key the SRD has) or, for kind `feature`, among the option list of a table feature. Kinds: `skill`, `expertise`, `cantrip`, `spell`, `language`, `tool`, `feature`, `feat`, and, in a feat only, `ability` and `damage_type` (see [Feat options and picks](#feat-options-and-picks)). A `feature` choice never asks for more than the options it lists. |
| `ability_increase` | Feats only. `count` different abilities from `from`, `value` 1 or 2; at most one per feat. The sheet keeps the result in `ExtraAbilityBonuses`. |
| `ambush` | See [Ambush](#ambush). |
| `reroll` | See [Reroll](#reroll). |
| `roll_minimum` | See [Roll minimum](#roll-minimum). |
| `modifier` with target `critical_range` | See [Critical range](#critical-range). |
| `spell_modifier` | See [Spell modifier](#spell-modifier). |
| `superiority_die` | See [Maneuvers](#maneuvers). |
| Spells in a `note` | A `note` with `spells` (like the SRD's Infernal Legacy): the character knows the cantrip, or a spell once per day when the same feature has a `resource`. The spell is on the sheet's list and does not count in the class numbers. Only a `note` carries `spells`. The structured way is [`granted_spell`](#granted-spells). |

Never allowed: `handler` (not even one that exists, since a handler reads columns of an SRD class a table class lacks), `spellcasting` (the engine writes casting from the class), `replaces`, `beast_spells`, `song_of_rest`.
A text-only feature always holds.

**Option lists.**
A class or subclass feature may carry `TableFeature.options` (a table's maneuvers).
Each option is a feature of its own (name, text, effects) with a server-made key (`feature:<stem><option name>@mesa`) and the offering feature as its `Parent`.
An option is in no level row: it is gained only when picked, and applies (its effects, its action under the parent) only while the character has the parent.
A `feature` choice with an empty `from` offers all the feature's options; on another feature of the same entry, `from` may list option keys (a later level's "two more").
The choice engine deals one pool of picks to choices that share options, in level order, so options an earlier choice took are blocked in the later one, and a pick whose parent is gone is reported as no longer valid.
A race or subrace trait may carry options too, but there they are a one-time choice of one, not a choice of features.
Each option is a trait with a server-made key (`trait:<stem>--<option name>@mesa`) that no race lists: it is gained only when picked.
The pick is stored with the sheet's choices and checked against the options; one outside them is reported as not offered.
A trait with options takes no `choice` of kind `feature` (refused at `.effects[i].choice`, `bad_value`).
The character picks one option, never more.
Refused, each at its field: options on a background feature, nested options, options on a class or subclass feature with no `choice` of features, a repeated option name or `from` key, a `from` key of another entry's list, a `choice` inside an option, more options than the limit.

**Feat options and picks.**
A feat may carry `TableFeat.options`, at most 60 (`MaxFeatureOptions`), checked like a class feature's option list.
Each option is a feature with a server-made key (`feature:<stem>--<option name>@mesa`).
It applies only while the character has the feat and has picked it.
A `feature` choice of the feat with an empty `from` offers them; a feat has one choice of options.
That choice may instead name `set`, one of the SRD's option sets but `table_feature`, and offers every option of the set, the table's included.
A `set` choice cannot ask for more than the options of the set (`.count`, `bad_value`), and only a feat has `set`.
An option of a feat never reads the feat's picks.

A choice of kind `ability` or `damage_type` is a pick, and only a feat has one.
It needs a `name` (1 to 40 of `a-z`, `0-9` and `_`, the first a letter, and not `mesa`), picks one (`count` 1), and lists `from` keys of its kind once each (empty is all six abilities or all damage types).
Only these two kinds have a name, and a feat's names are unique.
A later effect of the same feat writes `@name` for the local part of the pick (`ability:dex` is `dex`, `damage-type:fire` is `fire`), in `proficiency` (`save.@name`), `target`, `targets`, `ability`, `attack_ability`, `damage_ability`, `damage_type`, `damage_types` and `except` (`damage-type:@name`), and in the formulas of the fields `pickFields` lists as `mod("@name")` and `score("@name")`.
A token for a name the feat has not declared before the effect is refused at the field (`bad_reference`).
A token of the wrong kind, in an option of a feat or in an entry that is not a feat is refused at the field (`bad_value`).
A formula that writes `@name` anywhere but in `mod` or `score` does not compile (`bad_formula`).
A formula that reads a pick is checked when the content is imported and uses the pick the player made when the sheet is worked out.
An effect whose pick is missing or not offered does not apply.
The pick is a choice of the choice engine with the key `<feat>#<name>`, stored as `<feat>#<name>=ability:dex`, and `Derived.FeatPicks` holds the valid picks of the feats that read one.
`Content.Resistances` must be given the `Derived` of the same sheet, since the picks come from it.
A feat's own options hide only with the feat.
Only the options of an option set hide one by one.
A hidden option stays named for a sheet that holds it.

### Maneuvers

`superiority_die` (menu name "Dado de manobra"; fields `applies`, `resource`, `value`, `ability`, `economy`, `text_pt`; `rules/maneuvers.go`) is a die from a resource.
`applies` is one of `damage`, `attack`, `grapple`, `reduce_melee_damage`, `goad`.
Checked at its fields:

- the resource is one the entry or its class defines (`.resource`, `bad_reference`);
- `value` compiles;
- a reduction names `ability` and costs the `reaction`; a grapple costs a `bonus_action` or nothing; a goad names `ability` (the modifier of its DC) and costs nothing; a die added to an attack or damage costs nothing (`.economy`, `.ability`, `bad_value`).

`Derive` fills `Derived.Maneuvers` from the effects of what the character has (picked options included), with the die's sides worked out for the level, and only when the resource exists at that level; a `value` that gives fewer than 2 or more than 100 sides is ignored with a notice.
A goad is a damage die with a rider: the target makes a Wisdom save against `Maneuver.SaveDC` (8 + proficiency bonus + the `ability` modifier) and, on a failure, takes the combat effect `effect:goaded` until the end of the attacker's next turn. That effect is one `attack_disadvantage` modifier with `except_source`, so the target's attacks against anyone but the goader have disadvantage.
## Costs, healing actions and hooks

A `grant_action` spends a resource, heals and gives temporary hit points; an `on_event` runs the same results on the owner's events.
The formulas are Int formulas ([Effects](rules.md#effects)); dice are `NdS`, `NdS+B` or `NdS-B` with 1 to 20 dice of 2 to 100 sides (`parseHealDice`).

| Field | On | Meaning |
| --- | --- | --- |
| `cost` | `grant_action`, `on_event` | Uses of `resource` each use spends: a formula, 1 when empty, at least 1 at any level. Needs `resource`. |
| `cost_max` | `grant_action` | When set, the player picks how many to spend at use, from `cost` to `cost_max` or what is left. Never below `cost`; needs `resource`. A `cost_max` that is not above the cost (or that the resource's maximum cuts down to it) leaves the cost fixed. |
| `value_per_point` | `grant_action` | What each point spent above `cost` adds to the result. Dice (`1d6`, with the sides of `heal`) add to `heal`; a flat formula adds to `temp_hp`, or to `heal` when there is no `temp_hp`. Needs `cost_max` and a `heal` or `temp_hp`. |
| `heal` | `grant_action`, `on_event` | Dice of hit points regained, at least one die; `ability` adds that ability's modifier (worked out at `Derive`, so the dice carry it in `Bonus`). |
| `temp_hp` | `grant_action`, `on_event` | Temporary hit points: a formula. |
| `event` | `on_event` | `short_rest`, `long_rest`, `death_save_failed` or `drop_to_zero`; required. `drop_to_zero` answers damage that drops the owner to 0 hit points (a blow, a spell, a trap), never the master's "Ajustar PV" and never damage that kills outright. |
| `restore` | `on_event` | A resource of the entry (or its class) whose uses the hook gives back, `value` of them (an Int formula, 1 when empty), never more than are spent. |
| `heal_percent_max` | `on_event` | 1 to 100: that share of the maximum hit points (rounded down, at least 1) regained, instead of `heal`. |
| `resource` | `on_event` | The resource the hook spends (`cost` uses, 1 when empty): the hook runs only when that many are left. |

Refusals, each at its field and with the reason `bad_value` unless said: a `cost` that no level from 1 to 20 can pay with the resource's maximum (`.cost`; judged when both formulas read only `level()` and `prof()`, always when both are numbers); a `cost` or `cost_max` below 1 or with no `resource`; a `cost_max` below `cost` (`.cost_max`); a `heal` with no die, with `+ MOD` in the text or with an unknown or lonely `ability` (`.heal`, `.ability`); a `value_per_point` with no `cost_max`, with no result to scale, or with dice of other sides than `heal` (`.value_per_point`); an `on_event` with an unknown or empty `event` (`.event`), with no result (`.type`), with both `heal` and `heal_percent_max` or a share outside 1 to 100 (`.heal_percent_max`), with a `value` and no `restore` (`.value`), or with `cost_max`, `value_per_point` or `economy`; a `resource` or a `restore` that nothing defines (`bad_reference`).
Each of these fields on any other type is a stray field (`ownFields` in `overlay_features.go` lists what each type reads), and a formula that fails to compile is refused at its field with `bad_formula`.

What `Derive` gives the runtime:

- **`Action`** carries `Cost` (0 for the default of 1), `CostMax` (0 for a fixed cost) and `Result` (`Heal`, `TempHP`, and `HealPerPoint`, `TempHPPerPoint` for each point above the cost). `UnitCost()`, `ChoosesSpend()`, `SpendRange(left)` (the range offered, `ok` false when fewer than the cost are left), `Spend(chosen, left)` (validates the number picked, 0 takes the least; `ErrSpend` outside the range) and `Outcome(spent)` (the dice and temporary hit points of that spend) are the only arithmetic.
  The server reads `left` in the transaction of the use, so the client only supplies the number it picked.
  The action reaches the sheet as the proto `Action` (`cost`, `cost_max`, `heal`, `temp_hp`, `heal_per_point`, `temp_hp_per_point`).
- **`Derived.Hooks`** lists the character's `EventHook`s; `HooksFor(event)` picks those of an event.
  A hook that spends a resource the character does not have at its level is left out, and a `restore` of such a resource is dropped from the hook.
  A hook runs for its **owner only**: an effect on allies who rest nearby stays text.
  `PercentOfMax(pct, maxHP)` and `RestoreUses(used, count)` (never more than are spent, so a hook that runs again changes nothing) are its arithmetic.
- **Hit points and temporary hit points** follow the rules the spells share: `combat.ApplyHeal` (never above the maximum) and `combat.ApplyTempHP` (temporary hit points do not stack: the larger is kept).
  The target of an action's heal or temporary hit points is its owner or one creature chosen at use (the owner and one ally); a dead creature is not healed.

Recharges: besides `short_rest`, `long_rest`, `dawn` and `none`, a resource may recharge as `initiative` (one use when the owner rolls initiative, only when none is left: `InitiativeRecovery`), `restore_one_on_short_rest` (one use on a short rest, all on a long rest) or `restore_one_on_long_rest` (one use on a long rest, none on a short one).
`RestRecovery(recharge, kind)` answers `RecoverNone`, `RecoverOne` or `RecoverAll`; `RechargesOnRest` is `RecoverAll`.
A resource counts uses; a pool of hit points is not offered.

An option with a `superiority_die` does not list its `grant_action`, so the resource is not spent twice.
An option with a `grant_action` and no die is a plain maneuver: an action that spends one use of the shared pool and the economy it declares, with no number to roll.
How combat spends the die (damage, grapple, reduction) is in the [play module](play/README.md).

**Dice on the character's own rolls.** `applies` may also be `check` (an ability check; `targets` lists the skills `skill.stealth` and abilities `check.dex` it is limited to, empty is every check), `save` (a saving throw; `ability` limits it to one), `initiative` or `attack` (a weapon attack roll). These cost no action (`.economy`), `targets` belongs to `check` alone (`.targets`) and `ability` to `save` (and the other maneuvers that already name one). `Derive` copies `Targets` into `Derived.Maneuvers`. Play spends them on the scene check, the concentration save, the effect save, the initiative and the weapon attack roll ([Rests and resources](play/rests-and-resources.md#table-dice-on-the-characters-own-rolls)); the derived sheet lists them as `DerivedSheet.roll_dice`. A number that is always on is a `modifier` (`skill.*`, `save.*`, `initiative`, `ac`); an always-on die has no effect type.

### Extra damage

`extra_damage` (menu name "Dano extra"; fields `applies`, `value`, `dice`, `dice_sides`, `damage_type`, `weapon`, `frequency`, `no_crit_doubling`, `resource`, `while`, `when`, `text_pt`; `rules/extradamage.go`) is damage a feature adds to a weapon hit when the target is what `applies` asks: a fixed number, dice, or both.
Checked at its fields:

- `applies` is `any_target` or `larger_target` (`.applies`, `bad_value`);
- `value` (the fixed part, never doubled) and `dice` (an Int formula for how many dice, 1 to 20 once worked out) compile (`.value`, `.dice`, `bad_value`, `bad_formula`); at least one is set;
- `dice` and `dice_sides` (4, 6, 8, 10 or 12) go together (`.dice`, `.dice_sides`, `bad_value`);
- `damage_type` is a `damage-type:` key of the content; empty is the weapon's own type (`.damage_type`, `bad_reference`);
- `weapon` is `any` (empty), `melee` or `ranged`; `frequency` is `once_per_turn` (empty) or `each_hit` (`.weapon`, `.frequency`, `bad_value`);
- `no_crit_doubling` keeps a critical hit from doubling the dice, and needs dice (`.no_crit_doubling`, `bad_value`);
- `resource`, when set, is one the entry or its class defines (`.resource`, `bad_reference`);
- any field the type does not read is refused for the entry being written (`economy`, `ability`, or these fields on another type), and a feature has at most one (`.type`, `forbidden_effect`).

`Derive` fills `Derived.ExtraDamages` with the fixed amount and the dice worked out (`Amount`, `Dice`), the damage type, the weapon and the frequency with their defaults; a resource the character does not have at that level leaves the effect out, and so does a formula that gives no damage.
The attack card offers it after a hit as a choosable part ([Attacks and damage](play/attacks-and-damage.md)).

### Granted spells

`granted_spell` (menu name "Magia concedida"; `rules/grantedspells.go`) gives the character a spell apart from the lists of the classes. It is the table's way to write what a race trait, a feat or a feature grants: a cantrip at will, a spell with a few uses per rest, a ritual.

| Field | Meaning |
| --- | --- |
| `spell` | An SRD key or a spell of the same table (`bad_reference` at `.spell`). |
| `ability` | `int`, `wis`, `cha`, or `choice`: the player picks one of the three once for the entry, as a pick of the choice engine (`<entry>#grant-ability=ability:int`, in `feature_choice_keys`; until it is made the grant has no numbers and the sheet asks for it). It gives the spell attack bonus (proficiency + modifier) and the save DC (8 + the same) of the grant, whatever the classes cast with. |
| `from_level` | An Int formula, 1 when empty: the class level for a feature of a class or subclass, the character level for anything else. Below it the effect is not there. |
| `uses` | Empty or `at_will` for no limit; otherwise the name of a `resource` effect of the same entry (or its class, or an SRD resource): `bad_reference` at `.uses` when nothing defines it. Every cast spends one use. A feat's own `resource` effects count. |
| `also_with_slot` | The grant may also be cast with a spell slot, which spends no use. |
| `ritual_only` | The grant casts the spell as a ritual only (no slot, 10 minutes more); the spell needs the ritual tag, and the switch excludes `also_with_slot`. |
| `counts_as_known` | The spell is also listed among the known ones, without using any pick of the class. |

`when` gates the grant like any effect. Without `counts_as_known` the grant leaves the sheet's known spells, cantrips and prepared numbers as they are.
A spell above a cantrip cast through a grant is cast at its own level; it cannot be cast at a higher level for free, and the material components are not waived.
A cantrip scales with the character level.
The effect fills `Derived.GrantedSpells`; `combat.Options` lists each one as a turn option with a `Grant` (no slots unless `also_with_slot`, disabled with `NO_USES` when the resource is spent), and `combat.OutsideSpells` as one more way to cast the spell. The players' catalog drops a hidden `spell` from the effect like it does for `spells`.

`CastSpell` and `CastSpellOutsideCombat` take `granted_by`, the grant's source key.
With it, `GrantedCombatSpell` reads the spell with the grant's numbers (`rules.GrantedSpell`), no slot is spent, and a limited grant spends one use of its resource (`NO_USES` when none is left); the combat's undo gives the use back (`actionEvent.GrantUse`).
Outside a combat a use is spent when the casting starts, so a long casting through a grant has spent it before it ends; a ritual-only grant casts with `as_ritual`.
A summoning spell is cast with the classes, not through a grant.

### Spell modifier

`spell_modifier` (menu name "Opção de magia"; fields `applies_to`, `spell_change`, `from`, `value`, `damage_type`, `count`, `resource`, `cost`, `when`, `text_pt`; `rules/spellmodifier.go`) is a cast option a feature offers for the spells it fits.
`applies_to` is `damage_spell` (a spell that rolls damage), `cantrip`, an SRD school key (`school:evocation`) or an SRD damage type key (`damage-type:fire`: a spell that deals damage of that type); `spell_change` is the one change:

| `spell_change` | Fields | What it does |
| --- | --- | --- |
| `damage_type_swap` | `from`: at least two different damage types | The damage of a type in the list becomes another type of the list, which the caster picks at the cast |
| `bonus_damage` | `value`: a formula, 1 to 99 once worked out; `damage_type`: empty is the spell's own | Adds a fixed damage |
| `extra_dice_per_slot_over` | `count`: 1 to 6 | Adds that many dice of the spell's damage die for each slot level above the spell's own (none for a cantrip) |

`resource`, when set, is one the entry or its class defines (`.resource`, `bad_reference`): the option is ticked on the cast sheet and spends `cost` uses of it (the `cost` formula of `grant_action`, empty is 1, checked as there: it needs the resource, is at least 1 and is refused at `.cost` when the resource can never pay it).
Without a resource, `bonus_damage` and `extra_dice_per_slot_over` are always on and `damage_type_swap` is a free choice.
A feature has at most one (`.type`, `forbidden_effect`).
Checked at its fields: `.applies_to` and `.spell_change` (`bad_value`; `bad_reference` for a key the content does not have), `.from`, `.value` (`bad_formula`), `.damage_type` and `.count`; the fields of the other changes, and these fields on any other type, are refused for the entry being written.
`Derive` fills `Derived.SpellModifiers` with the amounts worked out; a resource the character does not have at that level leaves the effect out.
How a cast applies it is in [Spells](play/casting.md#casting-in-combat).

### Ambush

`ambush` (menu name "Emboscada"; fields `applies`, `when`, `text_pt`; `rules/ambush.go`) is an edge against a target caught unprepared.
`applies` is `unacted_advantage` (advantage on attack rolls against a creature that has not taken a turn in the combat yet) or `surprised_critical` (a hit against a surprised creature is a critical hit).
Checked at its fields: `applies` is one of the two (`.applies`, `bad_value`); any field the type does not read (`value`, `resource`, `economy`...) is refused for the entry being written.
`Derive` sets `Derived.UnactedAdvantage` and `Derived.SurprisedCritical` to the key of the feature that gives each.
Surprise is the master's mark at the start of the combat, which the app already keeps ([Play](play/README.md)); "has not taken a turn" is the combat before it runs, or the first round before the creature's place in the order (`combat.HasNotActed`).
The advantage is a source of the attack roll ("Name: vantagem neste ataque") and the critical hit is the roll's `critical_on_hit`.

### Reroll

`reroll` (menu name "Rolar de novo"; fields `rolls`, `at_most`, `resource`, `cost`, `when`, `text_pt`; `rules/rolleffects.go`) lets a die be rolled again, and the new roll must be used (SRD 5.1).
`rolls` lists what it covers: `attack`, `check`, `save` (the d20) and `damage` (a die of weapon or spell damage).
Checked at its fields (`bad_value` unless said):

- `.rolls` is required, holds only those four words and names none twice;
- `.at_most` is the highest face that may be rolled again, 1 to 20;
- `.resource` is one the entry or its class defines (`.resource`, `bad_reference`);
- `.cost` is an Int formula, 1 when empty, and needs the resource; a literal below 1 is refused;
- `.value` and `.tags` are refused (a tagged effect is never applied, and a reroll has no hint to show).

`Derive` fills `Derived.Rerolls` in the order of the effects, from the ones whose `when` holds.
A reroll is left out when its resource does not exist at the character's level, when `cost` fails or gives less than 1, or when `cost` is more than the resource's maximum at that level.
A rule with no resource is free and has cost 1 in `Derived.Rerolls`.
The combat link carries them as `Traits.RerollRules` (`Applies`, `AtMost`, `Resource`, `Cost`, `Source`: the key of the feature that gives it).
How the d20 is asked is in [Held rolls](play/held-rolls.md#the-d20-rolled-again-by-a-reroll-rule); how the damage dice are rolled again is in [Attacks and damage](play/attacks-and-damage.md).

### Roll minimum

`roll_minimum` (menu name "Valor mínimo no dado"; fields `rolls`, `value`, `damage_type`, `when`, `text_pt`; `rules/rolleffects.go`) makes a die that shows less than `value` count as `value`.
`rolls` is the list of [Reroll](#reroll): `attack`, `check`, `save` (the d20) and `damage` (a die of weapon or spell damage).
Checked at its fields (`bad_value` unless said):

- `.rolls` is required, holds only those four words and names none twice;
- `.value` is a literal from 2 to 20 (`MinRollMinimum`, `MaxRollMinimum`), never a formula;
- `.damage_type` is a damage type of the content (`bad_reference`) and is accepted only when `rolls` is exactly `["damage"]`;
- `.at_most`, `.resource`, `.cost` and `.tags` are refused (a tagged effect is never applied, and a minimum has no hint to show).

`Derive` fills `Derived.RollMinimums` in the order of the effects, from the ones whose `when` holds.
The combat link carries them as `Traits.RollMinimums` (`Applies`, `Value`, `DamageType`, `Source`: the key of the feature that gives it).
The highest applicable minimum wins.
For a d20, Reliable Talent's 10 competes with the table's minimums, and a tie keeps Reliable Talent.
For a die of damage, a minimum with a `damage_type` counts only on the dice of that type, and one without counts on all of them.
A die of damage smaller than `value` counts as its own highest face.
A reroll comes first and the minimum counts the die that stays.
The minimum counts the die for every rule that reads it: the total, a critical hit or a miss on a natural 20 or 1, and the 1 or 20 of a death save.
Where each roll counts it is in [Held rolls](play/held-rolls.md#a-d20-that-counts-as-another-number) and [Attacks and damage](play/attacks-and-damage.md#the-damage-minimum).

### Critical range

`modifier` with the target `critical_range` (menu name "Margem de acerto crítico") sets the lowest natural d20 that is a critical hit.
The mode is `set` (an empty mode is read as `set`; any other is refused at `.mode`), and a literal `value` is 15 to 20 (`.value`); a formula is clamped to 15..20 when it is worked out.
The menu entry carries `only_modes`, `min` and `max`; the editor reads `only_modes` and locks the mode to "Definir", and the server refuses a mode or a value outside them.
`applies` is the scope: `weapon` (the default, as the SRD's Improved Critical), `spell` or `both`.
The list `critical_scopes` carries the three words; any other word is refused at `.applies`, and `applies` on any other target is refused at `.applies`.
`Derive` lowers `Derived.CriticalRange` (the weapon attacks) to the lowest of the range the SRD features give and the value of each `weapon` or `both` modifier whose `when` holds.
It lowers `Derived.SpellCriticalRange` (the spell attack rolls), which starts at 20, to the lowest value of each `spell` or `both` modifier whose `when` holds; Improved Critical and Superior Critical never lower it.
A tagged modifier is a hint only.
Combat reads `Derived.CriticalRange` for a weapon attack and `Traits.SpellCriticalRange` (0 is 20) for a spell attack roll.

## Table options

`TableOption` (`rules/overlay_option.go`) adds an option to one of the SRD's option sets.
Its key is `option:<set>:<slug>@mesa` and `Set` is the set the key names: `invocation` (the Eldritch Invocations), `pact_boon`, `metamagic` (the three Metamagic features), `fighting_style` (the fighter's, the additional one, the paladin's and the ranger's), `hunters_prey`, `defensive_tactics`, `multiattack`, `superior_hunters_defense` and `table_feature` (`optionSetDefs`; the menu's closed list `option_set_names` carries them with their Portuguese names).
`With` writes the option as a feature of the clone and appends its key to the `Options` of every SRD feature that offers the set (the offering features are copied first: the SRD's are shared), and to `offeredBy`.
So the choice engine needs nothing of its own: the picks are dealt in the pool of the set, a set with table options is not bound by the 20 keys of a listed `from` (the pool is computed), the option is a feature once picked (its effects apply while the character owns an offering feature), and the same option cannot be taken twice by name.

The set `table_feature` is for the option list of a class or subclass feature of the content (a table's maneuvers): `offered_by` is that feature's key (`feature:...@mesa`), and the option is appended to its `Options` and to `offeredBy`.
It is checked at `.offered_by` (`dangling_reference`: empty, not a table feature key, missing from the content, or a feature with no choice of features to join (a class feature of kind `classfeature` is never one, since options are added before them); `bad_value` when another set carries it), and the builder adds classes and subclasses before options.
A feature's own inline `options` and the options that name it share one pool, which the 60 of `MaxFeatureOptions` do not bound: the bounds are the table's (`MaxOverlayEntries`, 1,000 entries; 64 KiB an entry; `MaxOverlayEffects`, 6,000 effects) and 20 effects an option.
The 20 keys of `maxEffectList` bind only an explicit `from` list.
An option of this set is read as an inline option of its feature: it spends the resources the feature's entry or class declares, and reads the states they declare.

Checked at its fields: the key's set is known and equals `set` (`.set`, `bad_value`); `prerequisite.class_level` is a class of the content and a level 1 to 20, both or neither (`.prerequisite.class_level.class_key`, `.level`); `requires_option_keys` are SRD options of a set or table options, at most 10, never the option itself (`.prerequisite.requires_option_keys[i]`); `requires_pact` is a Pact Boon, the SRD's or a table option of `pact_boon` (`.prerequisite.requires_pact`); `requires_spell_key` is a spell of the content (`.prerequisite.requires_spell_key`); the effects are the menu's, but a `choice` is refused (`.effects[i].type`: an option does not ask for another choice).
Every prerequisite set must hold.
The pool lists an option that does not qualify with its reason and `Blocked()` (as the invocations' own prerequisites), and a sheet that picks it has the problem `prerequisite`; a spell the character knows is a cantrip, a known or a prepared spell, or one a pick grants.
An archived or switched-off option is left out of the pool unless the sheet already has it.

## Table class features

`TableClassFeature` (`rules/overlay_classfeature.go`) adds a feature to a class, the SRD's or the table's, at a class level.
Its key is `classfeature:<slug>@mesa`; it carries `Class`, `Level` (1 to 20), an optional `Replaces` and the feature body (name, text, effects).
`With` writes it as a feature of the class and `applyClassFeatures` puts its key in the class's row of that level, the same row a feature of a table class sits in, so Derive, the choice engine, the level-up and `play` read it where they read the class's own features, and a subclass level row is built the same way (rows keyed by class level).
The SRD's rows are shared, so a row is copied before it changes.
`Replaces` is a feature of the class (an SRD one or another table class feature) that leaves the rows from the entry's level on, as the `replaces` effect hides lower tiers; the entry is refused if the class has no such feature at that level or later, or if it came earlier ("a character never loses what it already had"), and a feature replaced twice is refused at the second entry (in key order).
A switched-off entry is skipped entirely: the class reads as it did without it, and the replaced feature stays.
An archived entry still applies to the sheets that have the class.
Checked at its fields: `.class_key` (a class of the content), `.level` (1 to 20), `.replaces_feature_key`, `.desc_pt`, `.effects` and `.summon`.
A class feature may carry a `summon` (`TableClassFeature.summon`).
It brings creatures to every character that has the feature at its level.
It is checked and cast as a [table feature's summon](#table-feature-summons).
Its `resource_key` is a resource the entry's own `resource` effects define, or one of its class or the SRD.
A player reads only its `creature_keys`.

## Table creatures

`TableCreature` (`rules/overlay_creature.go`) is an SRD creature with a few adjustments.
Its key is `creature:<slug>@mesa`.
It names a `BaseKey` (an SRD creature such as `monster:wolf`) and changes `HitPoints`, `ArmorClass`, `DamageBonus` and `Traits`, and adds `Attacks`; the rest of the stat block is the base's.
Its numbers can be formulas of its owner (below).
`With` copies the base's stat block, applies the adjustments and registers the copy under the entry's key, so the creature resolves where an SRD creature does: `MonsterDerived`, `CreatureByKey`, `MonsterPlan`, `ListCreatures` and the combat.
The base is never written: the SRD's maps are copied before the first table creature goes in.

| Field | What it does | Limit |
| --- | --- | --- |
| `BaseKey` | The SRD creature it is made from. A table creature is never the base of another. It never changes after the entry is created (`immutable` at `table_creature.base_key`) | Must exist in the SRD (`dangling_reference` at `.base_key`) |
| `HitPoints` | The creature's maximum hit points. 0 keeps the base's. The base's hit dice stay and its hit points roll is dropped | 1 to 9,999 (`MaxCreatureHitPoints`) |
| `ArmorClass` | The creature's armor class. 0 keeps the base's. The kind and the notes of the base's armor class are dropped | 1 to 40 (`MaxCreatureArmorClass`) |
| `DamageBonus` | A whole number added once to the first damage part of every attack action, so each attack of a Multiattack gets it. 0 adds nothing. A flat damage stops at 0, and dice that end in `+MOD` are left as they are. The text of the action stays the base's | -10 to 30 |
| `Traits` | Traits added after the base's special abilities, as text only: a name and a text, no effects | At most 10 (`MaxCreatureTraits`); a name of 1 to 80 characters on one line; a text of 1 to 4,000 characters |
| `HitPointsFormula`, `ArmorClassFormula`, `DamageBonusFormula` | Formulas of the owner that replace `HitPoints`, `ArmorClass` and `DamageBonus` | Results clamped to 1 to 9,999, 1 to 40 and -10 to 30; a result beyond the engine's limit (`formula.MaxResult`, 10,000 in absolute value) is a failed formula and the static value stands |
| `AttackBonusFormula` | The to-hit of the base's attacks, and of an own attack with no `ToHit` | Clamped to -10 to 30; a result beyond the engine's limit is a failed formula and the base's value stands |
| `Attacks` | Attack actions added after the base's, keyed `<creature key>#<slug of the name>`, with an attack roll. Not part of the base's Multiattack. A melee attack has a reach (0 is 5 ft., to 30) and no range; a ranged one has a range (1 to 600) and a long range (0 or up to 1,200). The dice are 1 to 20 of a d4, d6, d8, d10, d12 or d20. `DamageType` is a `damage-type:` key of the SRD | At most 8 (`MaxCreatureAttacks`, `limit` at `.attacks`); a name as a trait's, not repeating a trait, a base action or another attack |
| `DamageMods` | Resistances, immunities and vulnerabilities, added to the base's lists (rules under the table). | At most 12 (`MaxCreatureDamageMods`, `limit` at `.damage_mods`) |

The rules of `DamageMods`:

- Each entry has a `kind`: `resist`, `immune` or `vulnerable`.
- Its `damage_types` are `damage-type:` keys of the SRD, each once, at least 1.
- Its `qualifier` is empty, `nonmagical`, `nonmagical_unless_silvered` or `nonmagical_unless_adamantine`, mapped to the `srd51.Qualifier*` words in one table.
- A qualifier is only on `resist` or `immune`, and only when every type is bludgeoning, piercing or slashing.
- Each entry becomes a `srd51.MonsterDamageMod` with an English note like the SRD's.
- The stat block and combat read it as an SRD one.
- An entry is added to the base's lists and never replaces or narrows them.
- If the base already resists a type, a qualified entry for that type does not remove the base's resistance.
- The entries are read from the content's entry by key, not kept per owner, so editing them changes the creatures already given.
- A snapshot of a creature's numbers does not hold them: `ResolvedDerived` ignores any `DamageMods` in it (`trusted` takes them from the entry).
- Violations: `bad_value` at `.damage_mods[i].kind`, `.damage_types` or `.qualifier`; `bad_reference` and `bad_value` at `.damage_types[j]` (not an SRD damage type, repeated).
- A bad kind or a bad damage type is the only violation of its entry: the qualifier is not judged then.

### Formulas of the owner

A formula is compiled once, when the overlay is built, with `formula.Compiler.WithSlot`, and kept on the creature's entry.
A formula that does not compile, or calls a function that does not exist, is `bad_formula` at its field, such as `.attacks[2].to_hit`.
The environment is `OwnerEnv`: `level()`, `classLevel("<class>")`, `prof()`, `mod("<ability>")`, `score("<ability>")`, `armor()`, `shield()` and `speedwalk()`, with `floor`, `ceil`, `min` and `max`.
It may also call `slot()`, the circle of the spell slot a spell was cast with.
A creature given to a character was not made by a slot, so `slot()` is 0 in its formulas; one a spell summoned has the circle of the slot it was cast with.
`slot()` is legal only in these fields; an effect, a condition and every other formula refuse it.
The formulas count in the overlay's formula budget.
The specific beats the general: an own attack's `ToHit` wins over `AttackBonusFormula`, and its `DamageBonus` over the creature's damage bonus.
An own attack with neither has +0 to hit and the creature's damage bonus.

`Content.ResolveCreature(key, env, slot)` runs the formulas for an owner (`OwnerEnv(derived, shield)` makes the environment from a derived sheet) and clamps each result to its field's range.
A result beyond the formula engine's limit (`formula.MaxResult`, 10,000 in absolute value) is a failed formula, not a clamped one.
A formula that fails when it runs keeps the static value and is named in `ResolvedCreature.Failed`.
`Content.ResolvedDerived` builds the stat block and the plan of the actions from the result; it takes the base, the name, the traits and the shape of the attacks from the entry, and only the numbers from the value it is given.
`ApplyResolved` writes a result on the base's stat block the same way the static adjustments are written, so a creature with no formula has the same block with or without an owner.
`CreatureHasFormulas` says whether an entry has any formula.
The content's own block for a table creature (`MonsterDerived`) runs no formula: its own attacks have +0 to hit and the static damage bonus.

A trait name is refused (`bad_name`) when it repeats a trait or an action of the base, compared by its letters `a` to `z` and digits, or when it is `Magic Weapons`, `Spellcasting` or `Innate Spellcasting`, ignoring case, because the engine reads those names.
An adjustment out of its range is `bad_value` at its field, a trait text out of its length is `bad_text` and too many traits is `limit`.
The checks run before anything is built, so a creature reports every refusal at once.

A table creature is left out of every use that depends on the base's challenge rating and XP, which the adjustments may not keep:

- The encounter evaluator, the generator and the swaps (`EvaluateEncounter`, `GenerateEncounter`, `EncounterSwaps`) read the SRD's creatures only.
- The summon choices of the SRD spells (`CheckSummon`) accept SRD creatures only; a table spell's `summon` also accepts table creatures.
- The Wild Shape forms (`WildShapeForms`) are SRD beasts only.
- The encounter tools refuse a table creature's key as unknown content, because `HasCreature` is true for SRD creatures only.

An archived or switched-off entry keeps resolving.
A creature already held, or an NPC made from it, keeps working.
`Content.Creatures()` lists the table's creatures with the marks.
`Content.Hidden` is true for it, as for any other kind (see [Switches](#switches-and-hidden-keys)).
The server does not use that predicate for a table creature: a player reads one only when a character of theirs holds a live creature of its key, whatever the entry's state ([characters](characters-table-content.md#table-creatures)).

## States

A `state` effect defines a state; the character's player turns it on and off.
`Derive` lists the states of the sheet in `Derived.States` (key = the feature's key, `#`, the state's name) and gives each an action in `Derived.Actions`, with `state_key` and `state_active`, so the card is the one of a granted action: "Ativar" spends the economy and the use of `resource`, "Encerrar" costs nothing.
Which states are on is not on the sheet but a session input, `Build.ActiveStates` (the keys, kept per character; see [Characters](characters.md#temporary-states)); a key no state effect of the sheet defines is ignored.
Mage Armor's armor class (`MageArmorAC`), worked out outside the combat, counts the states that are on, as the sheet does.
A `duration_rounds` formula that fails when it runs leaves the state listed with no duration, so it lasts until it ends, and records an issue that says so.
The state stays listed because one that is on must stay on the sheet to be ended.

**The gate.** `while` is on `resistance`, `roll_mode`, `extra_damage`, `weapon_rule` and `pseudo_weapon` (`gatedEffectTypes` in `rules/gate.go`), and `when` of any type may read `active("name")`, a Bool that is true while the state of that name in the same entry is on.
A name belongs to the entry of the effect, as a resource name does: `.while` and `active("x")` name a `state` effect of the same class, subclass, race, subrace, background or feat (refused `bad_reference`).
`active()` is refused in a formula that is not a condition (`bad_formula`).
A gated effect whose state is off does nothing and is not listed (a resistance is listed only while its state is on).
To gate another type: add it to `gatedEffectTypes` and `while` to its fields in the menu (`state_ref`); a Derive step that asks `applies` needs nothing else.

**Weapons.** `weapon_rule` and `pseudo_weapon` are read where the sheet builds a weapon line (`attacks.go`), so the attack list, the attack roll and the damage roll agree.
A pseudo-weapon's line has the key `pseudo:<feature key>` (`#2`, `#3` for a feature's further ones) and is proficient; a weapon rule that names no ability keeps the weapon's.

## The class

The ADR-0018 contract: hit die (6, 8, 10 or 12), two saves, skills, proficiencies, multiclass prerequisites, the subclass level (3 if unsaid) and exactly 20 table rows (proficiency bonus, which defaults to the SRD's, the features, cantrips, spells known and slots).
The ability score improvement levels default to 4, 8, 12, 16 and 19; the server writes the features `<class>-ability-score-improvement-<n>@mesa`, which the engine finds by key like the SRD's.

**Casting.** `none`, `full`, `half` or `pact`; the ability; `known` or `prepared`; and the list: an SRD class's (or a table class's own), plus the spells that cite the class.
A class with a list of its own (`list_from` empty) can also name SRD spells in `casting.srd_spell_keys`, which join the table spells that cite the class (an apprentice-like class built on Cure Wounds, Aid and Revivify).
Each key is an SRD spell (`spell:<index>`, never a table key), named once, and a class names at most `MaxClassSRDSpells` (400, checked with the other budgets).
A class that reuses another list (`list_from` set), a class that does not cast and a third caster's subclass have none: the refusal is `bad_casting` at `casting.srd_spell_keys`; an unknown or table key is `dangling_reference` and a repeated one `bad_value`, at the key's index.
The builder puts the class on the `Classes` of a copy of each named spell, so every reader of the list follows: the "Magias" page and its learnable filter, the catalog entry's `class_keys`, what a class prepares or learns, the level-up check and the sheet's `spell_not_on_list`.
The number of prepared spells is a formula (default `max(1, mod + level)`, with half the level for the half caster).
The "Conjuração" feature is written by the engine.
Before the level casting starts the columns are zero; after, they have slots (a single slot level for pact).

**Third caster.**
It is the casting of a subclass (`CastingThird`), as in the SRD: the parent class does not cast, and the subclass brings the table of cantrips, spells and slots for each level from the level it starts to 20.
The multiclass rule (`multiclassSlots`) counts the full caster's whole level, half of the half caster's and a third (rounded down) of the third caster's, and reads the slots from the SRD full caster table, never a table class's.
Alone, the caster uses its own table.

**Subclass.**
It carries the parent class (SRD's or the table's, which never changes), the features by class level, the optional third-caster casting and the always-prepared spells by class level, which do not count against the prepared limit.
The choice level is the parent class's.
The SRD's Fiend patron is the exception: its spells are an expanded list (`expanded_list_corrections` in `effects/corrections.json`), so they only join the warlock's list, to choose from, and count against the spells known.
A table subclass does the same with `spells_expanded`: its always-prepared spells (and their class levels) are then not given, they join the class's list for a character of that subclass, and the picker's sources (`SpellSources`) offer them from their class level on.
The builder puts the subclass on the `Subclasses` of a copy of each spell, as the SRD data does, so the sheet, the level-up check and the picker read one rule.
The flag needs the subclass to name spells; without any it is refused at `table_subclass.spells_expanded` (`bad_value`).
The catalog lists no always-prepared spell for an expanded subclass.

## Race, subrace, background

- **Race and subrace** (2014 format): ability bonuses, size, speed, darkvision (a sense of the race itself), languages and how many to choose, and the traits as features with effects from the menu. The bonuses the player places come as `choice_bonuses` (one set, each bonus 1 to 4, at most six) or as `choice_bonus_sets` (up to four sets of 1 to 6 bonuses of +1 or +2, no two the same; a race has one or the other, refused at `.choice_bonus_sets`): the player picks them in "Escolhas", as the half-elf's +1s: a choice of the set when the race has several (`race:x#bonus-set=bonus-set:1`) and, for each distinct bonus of the set, a choice of as many abilities as the set has of it (`race:x#bonus-2=ability:str`). Each ability takes one bonus (a repeated pick is refused as unmet and gives nothing, and the other choices do not offer the ability), `Derive` adds the picks to the race's own bonuses, and the issue `race_bonus` says so until the picks fill the set. A sheet that placed the bonuses in the manual ones before they were picks, covering a set, stands for the picks. `sizes` (`Small`, `Medium`, once each, with the race's own `size` among them) are the sizes the player picks one from, kept in `FullSheet.size`; empty is the race's own size, a size the race does not offer is the issue `race_size`, and `Derived.Size` is the size that counts (the Small heavy-weapon reminder and the combatant's size read it).
  "+2 and +1 of your choice" is the master's option (`ChoiceBonuses`, a single set): the player picks the abilities in "Escolhas" and `Derive` warns (`race_bonus`) until they are picked.
  A subrace adds its bonuses and traits to the race's, unless it replaces some:
  - `replaces_trait_keys` lists traits of the parent race (SRD or table) that the subrace takes the place of.
    Each key must be a trait of the parent race and appear once.
    A wrong key refuses the entry at `table_subrace.replaces_trait_keys[i]`, naming the key.
    The list has at most 60 keys (`MaxTraits`).
    A replaced trait is not owned by the character; see [Characters: table content](characters-table-content.md#the-sheet-of-a-race-and-a-subrace).
  - `replaces_ability_bonuses` swaps the race's ability bonuses whole for the subrace's.
    The fixed ones and the ones the player places (`choice_bonuses`, `choice_bonus_sets`, the half-elf's picks) are never added under the subrace's.
    The sheet asks no pick for them.

  How the sheet merges the two is in [Characters: table content](characters-table-content.md#the-sheet-of-a-race-and-a-subrace).
- **Background:** two skills, tools, languages to choose, the equipment as text and the feature, usually a note.
  A table background's equipment reaches `Derive` (`background_equipment_pt`).

## Feat

`TableFeat` (`feat:<name>@mesa`) has a name, text, a `FeatPrerequisite` and effects (the feat owns them; it has no features).
The prerequisite is all of: minimum scores that must all be met, scores of which one is enough, a proficiency key, "can cast at least one spell", a race or subrace key or a list of them of which one is enough (a subrace satisfies its parent race's key; a feat sets the key or the list, never both, and the list repeats no key), a size (`Tiny`, `Small`, `Medium` or `Large`, the one the sheet counts), a feature, trait or feat key the character must not have (never the feat itself), a total level.
Each key must exist, SRD or table; the table refuses a wrong one at its own field of `prerequisite`, when the master writes the feat and when a pack brings it.
`Content.Feats`, `FeatOptions(build)` and `CheckFeat` answer who qualifies and what they lack (`FeatUnmet`; the kinds `race`, `size` and `lacks_feature` are among them).
The guided level-up judges a lacked feature before the feat's own grants.
The sheet judges it on what the character owns (`lostFeats`).
The guided level-up refuses an unmet feat with `LEVEL_UP_REFUSAL_REASON_FEAT_PREREQUISITE`.
A feat chosen inside a level-up is refused with `LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE`.
The choice paths, feat choices at creation included, refuse it with `CHOICE_REFUSAL_REASON_PREREQUISITE_UNMET`.
A player never reads the name of a hidden race, feat or feature, in a key or in a text.
That holds for the player's copy of a feat, the level-up options and the texts of the feat-choice cards.
A feat that asks only for hidden races is not offered to a player and is not in their copy, because without the races its prerequisite would read as none.
A feat that asks for some visible races and some hidden ones names the visible ones only.
A condition on something the player may not read comes with `hidden` (`FeatUnmet.hidden`, `FeatOption.lacks_feature_hidden`) set and the key and name empty, so the app shows a neutral condition, never a name.
The `race` kind is hidden only when every asked race is.
The master's view is never hidden.
What a sheet has is named to it: the race, subrace, feat or feature it holds, and the traits, features and options of the entries it keeps.
That holds in the level-up options, the cards and the player's copies of the entries.
A condition on a hidden feature the sheet does not have reads as a text that names nothing, never as no condition.
The SRD's feat (Grappler, from `5e-SRD-Feats.json`, effects in `effects/feats.json`) is in the same `feats` map.
`Build.Feats` are the keys a character took: `Derive` adds their effects and lists each among the features (source "Talento").
A character that no longer meets the prerequisite of a feat it holds keeps the feat on the sheet with the issue `feat_prerequisite`, and its effects do not apply until it meets it again.

## Spell

A table spell has spell level, school, casting time, range, components (with the material), duration, concentration, ritual, classes, text and "at higher levels", plus the target (`SpellTarget`): one creature, several (with one more per level above), an area (cone, cube, cylinder, line or sphere, in feet, in steps of 5; an area takes no spell attack) or only the caster.
Time, range, duration and components have input-only types without the fields `With` would ignore.
Optionally it has an attack, a saving throw (half or nothing), damage (extra dice per level above or per cantrip tier) and healing, whose dice are plain (`8d6`, no bonus) and of a die the table rolls (d4, d6, d8, d10, d12, d20 or d100, the dice `platform/dice` rolls).
`With` writes the tables in the SRD format and uses the same constructor, so `SpellDetails` equals the SRD spells' (`DamageAt`, `HealAt`) and combat resolves a table spell the same way.
The target is `SpellDetails.Target` (`IsArea()`, `AnyNumber()`, `MaxTargets(level, spellLevel)`); the engine stores the size in feet and writes the label (`LabelPT`, "Cone de 4,5 m").
How an SRD spell's target is decided is in [Characters](characters-table-content.md#spell-targets-and-the-magias-page).
A spell never crosses between cantrip and leveled spell on an edit (the sheet stores them in different lists); the server refuses that change.

The spell-list question "is this spell on this class's list?" has one answer, `content.onList` (`spell.classes` plus the list the class reuses); `Derive` and level-up use it.
The catalogue answers from the other side, `spellClasses`.
`Validate` does not look at lists: an off-list spell is a `Derive` `Issue`.
Combat uses `Spellcasting.SpellList`, the list the caster reads: the class's or, for a third caster, the class the subclass casts from.

## Switches and hidden keys

The master switches classes, subclasses, races, subraces, backgrounds, spells, feats, options, class features and creatures on or off per campaign, SRD and table alike; everything is on by default.
The master switches creatures on the options screen, in its "Criaturas" group.
The creature editor has the same switch.
`Overlay.Off` carries the switched-off keys to `With`, which stores them on a copy and ignores a key that is not switchable (`Content.Switchable`).

- `Content.Off(key)` is the key's own switch.
- `Content.Hidden(key)` is what the player does not receive: archived, switched off, the subclass or subrace of a class or race that is switched off or archived, or a trait, feature, background feature or option of a hidden race, subrace, class, subclass or background (`Content.Owners` names those).
  A trait that several races share, and an option that several classes offer, is hidden once all its owners are.
  The children keep their own switch, so turning the class back on returns each subclass as it was.
- A parent only hides the children of whoever does not have it: `Content.OffKeys(build)` lists the switched-off keys a sheet uses, and a subclass (or subrace) under a switched-off class (or race) counts only when the sheet does not have that class (or race).
  A sheet that already has the switched-off class keeps reaching its subclass level; a new sheet cannot take the class, so not the subclass either.
- `Content.ForPlayer()` is the content as a player reads it: the feat-choice cards and the feats of the level-up options built from it name no hidden entry.
- `Content.AnyHidden()` lets a caller that only filters skip the work when nothing is hidden.

## "A classe mudou"

A change to table content takes effect at once, also on locked sheets.
`Content.ChangedSince(build, savedRevision, baselines)` lists the table entries a sheet uses that changed after the revision it was saved at; each entry stores the revision and date of its own last change (an archive is not a change).
`Derive` ties every notice to the table entries it depends on (`Issue.Keys`: the entry of the field it points to, and the entries the number is computed from, such as the classes in a skill count or all the keys in a formula that failed), so the server can show, for each changed entry, only the new notices whose keys include it.

`Issue` also carries `ChangeMessage` and `ChangeSubject`: the sentence of the same notice told as a change, without the entry's name, and the class or subclass it is about.
Only notices tied to a table entry have one.
The server builds the final sentence where the changed entry is known (`changeSentence`): the name enters only when the sentence is about the very entry that changed.
The sentences:

| Subject | Sentence |
| --- | --- |
| Skills | "Guardião do Vale agora dá 2 perícias no nível 1; esta ficha tem 3." (with another skill source: "O total de perícias para escolher agora é 4; esta ficha tem 3.") |
| Cantrips | "Mago agora conhece 3 truques; esta ficha tem 4." |
| Spells known | "… agora conhece 5 magias; esta ficha tem 6." |
| Spells prepared | "… agora prepara 4 magias; esta ficha tem 5." (the third caster is named by its subclass; with several sources: "O total de magias preparadas agora é 4; esta ficha tem 5.") |
| Subclass level | "Guardião do Vale agora escolhe a subclasse no nível 5; esta ficha tem nível 3 nela." |
| Multiclass | "Guardião do Vale agora pede outras habilidades para multiclasse; os desta ficha não cumprem." |
| SRD option no longer offered | "Guardião do Vale agora não oferece a escolha Estilo de Luta: Defesa; ela não vale mais nesta ficha." The notice is tied to the entry whose feature offers options of the same SRD set; if none offers it any more, there is no entry to blame and it does not appear in "A classe mudou". |

A sentence says what the entry gives today and what the sheet has, in the singular when it is 1.
Other notices (a spell that left the class list) use their own sentence.
The neutral sentence in `DerivedSheet.issues` keeps the notice's identity (`code`, field and sentence) and holds in a draft.

## The class editor's data

Two reads give the class editor everything it needs, so the browser computes nothing:

- **`TableDefaults`** returns the SRD proficiency bonus per level (+2 at levels 1 to 4 up to +6 at 17 to 20), the ability score improvement levels, the subclass level (3) and a 20-row table per casting type: none, full, half, pact and a third (the subclass's), each "knows" or "prepares".
  Each row is a `TableClassLevel` ready to be the class's `levels` (features empty): bonus, cantrips known, spells known and the nine slot counts.
  The numbers are read from the SRD itself, never from the campaign's content: full and third use the Wizard/Cleric's, half the Paladin's (starts at level 2), pact the Warlock's, "knows" the cantrips and spells known of the Sorcerer (full), the Ranger (half) and the Warlock; "prepares" has no spells known.
  The third caster, which SRD 5.1 lacks, is our default: the full table at a third of the level, rounded up, 2 cantrips (3 from level 10) and, for one that knows spells, 3 at levels 3 and 4 and one more every two levels; the master changes whatever they want.
- **`EffectMenu`** returns the effect types of the menu with their Portuguese names and sentences, each one's fields (required or not, the field's kind and list, a number's limits, `always_shown` for an optional field the editor never folds away), and every closed list with Portuguese names: modifier targets, modes, proficiency targets, levels, advantage modes and targets, senses, recharges, `events`, action economies, choice kinds, `ranged_attack_applies`, `damage_types`, `resistance_damage_types`, `resistance_kinds`, `resistance_conditions`, `conditions`, `granted_spell_abilities`, `state_ends`, `state_economies`, `weapon_filters`, `weapon_kinds`, `lackable_features`, `race_traits`, skills, languages, tools and tag prefixes.
  `lackable_features` is the SRD features, traits, background features and feats a feat's `lacks_feature_key` may name (each passes the same key check the server uses; the keys of options are not listed), with where it comes from in `hint_pt` ("Monge, nível 5", "traço do SRD", "talento do SRD"); the table's own entries are not in it, because the editor already holds them.
  `race_traits` is every trait of every SRD race, the keys a subrace of an SRD parent may name in `replaces_trait_keys`, with the Portuguese name, the race in `group` and the first sentence of the trait's Portuguese text in `hint_pt` (empty when it has none).
  The table's own races and traits are not in it, because the editor already holds them.
  Only the master reads the menu.
  It also carries the SRD option lists a "one option from a list" choice can offer, the formula functions with a sentence each, the class indexes `classLevel` accepts (SRD and table) and the limits.
  A test puts each value of each list in an effect and checks that `With` accepts it, so the menu never offers what the server refuses; it offers a subset of what the engine understands.

A field an effect type does not read is refused only when an entry is written (`Overlay.Strict`); in an entry already stored it is ignored on read, so a lost field never makes a campaign unreadable.
Every refusal of a class or subclass comes out with the exact field the editor draws, in the proto's names and with its indexes (`table_class.levels[4].slots[2]`, `table_class.casting.list_from`, `table_class.skill_from[2]`, `table_subclass.always_prepared[1].spell_key`), and a stable reason.

## Quality gate

ADR-0018 requires that a table class works end to end.
`TestLevelUpSweepTable` levels from 1 to 20, doing exactly what the options ask, one table class of each casting type (none, full that prepares, full that knows, half, pact) with subclasses (one with features, another with always-prepared spells) and the third casters of two SRD classes (the Fighter's Eldritch Knight, which knows, and the Rogue's Arcane Trickster, which prepares).
`TestLevelUpSweepTableMulticlass` repeats it with each of them together with an SRD class, in both orders.
No step may fail `CheckLevelUp` or `Validate`, leave an `Issue` or panic.
`TestDeriveTableCharacterGolden` pins a table character's sheet at levels 1, 5 and 11 (`testdata/golden/table-character.json`; rewrite with `go test ./internal/rules -update`).

## The area a spell keeps on the map

`TableSpell.Zone` (`TableSpellZone`: `Trigger` of `ZoneStartOfTurn`, `ZoneFirstEntry` or `ZoneBoth`, and `FollowsCaster`) is checked by `checkSpellZone` and kept in `Content.spellZones` (`Content.SpellZone`). Refused at `.zone.trigger` (not one of the three), `.zone.follows_caster` (needs the range Self if it follows, a distance if it does not) and `.zone` (a cantrip; a target that is not a sphere, a cylinder or a cube; not exactly one damage type; a spell attack or healing; neither a timed duration nor concentration). The combat reads it through `link.Spell.Zone` ([Spell zones](play/spell-zones.md)).

## The lasting effect of a spell

`TableSpell.Effect` (`TableSpellEffect`, `rules/overlay_spell_effect.go`) is what the spell leaves on its targets while it lasts.
`checkSpellEffect` turns it into the `EffectDef` of the spell's key.
`With` keeps it in `Content.tableSpellEffects`, so `Content.CombatSpellEffect` answers for a table spell as for an SRD one.
The whole of [Lasting effects](play/lasting-effects.md) then works unchanged: the clock, the concentration, the saves, the undo and the card.
An archived or switched-off spell has no effect.

- **Where it takes hold.**
  `applies` is `all` (the default), `failed_save` or `hit`.
  `failed_save` takes the targets that fail the spell's save, so the spell needs a `save`.
  `hit` takes the targets the spell attack hits, so the spell needs an `attack`.
  A spell whose target is Self applies `all` to its caster.
- **How long.**
  The spell's duration and concentration.
  An instantaneous spell has no effect (`.effect`).
- **What it is.**
  `conditions` are SRD condition keys, each once, at most 8.
  `modifiers` are at most 8.
  An effect with neither is refused.
- **The modifiers.**
  The kinds are those of `combat_effects.json` that fit a table spell, and nothing else.
  `ac_bonus`, `speed_add` and `speed_multiplier` change the numbers of the creature.
  `roll_die` adds a die as Bless does, `roll_bonus` a flat bonus on `attack`, `save` or `check`, and `check_bonus` a flat bonus on the checks of one skill.
  `save_advantage`, `check_advantage`, `attack_advantage`, `attack_disadvantage`, `save_disadvantage` and `check_disadvantage` give a roll a mode.
  `resistance` takes a `resistance_kind` (`resist`, `immune` or `vulnerable`) and `damage_types` as the feature effect does.
  `temp_hp` gives temporary hit points when the effect lands.
  `roll_bonus`, `temp_hp`, `resistance`, `attack_advantage`, `save_disadvantage` and `check_disadvantage` are kinds of the shared vocabulary: the SRD file may use them too.
  The table keeps no list of its own.
- **Refused, each at its field.**
  `base_ac`, `extra_action`, `no_move`, `no_action`, `no_reaction`, `condition_immunity`, `turn_temp_hp`, `choose` and `from_caster`, and `once` or `except_source` on `attack_disadvantage`.
  A number out of its range, at `.value` or `.die` (`checkModifier` has the ranges).
  An unknown condition, skill or damage type, at its own field.
  A repeated condition, ability or roll.
  A `player_label_pt` over 30 characters.
  A `visibility` that is not `public` or `owner`.
  An `applies` the spell cannot honor, at `.effect.applies`.
  The path is `spells[i].effect...`, and `table_spell.effect...` in the API.
- **Tags.**
  `EffectTagsPT` writes the short Portuguese tags of the owner's card ("CA +2", "PV temporários: 5").
  `player_label_pt` is the label the other players read.

A feature's lasting effect is its owner's and needs no field of its own.
A `state` effect turns on the effects gated by `when active("name")` or `while` for its duration (see [States](#states)).

## Table spell summons

A `TableSpell` may carry a `summon` (`TableSummon`, `rules/overlay_summon.go`): the creatures it brings to its caster.
The spell is registered with the SRD's summoning spells, so `SummonSpells`, `SummonOptions` and `CheckSummon` read it like any other.

| Field | Meaning | Limits |
| --- | --- | --- |
| `creature_keys` | Table creatures or SRD creatures the caster picks from | 1 to 8 (`MaxTableSummonCreatures`), none twice, each one that exists |
| `count` | A formula of the caster and of `slot()`; one that fails when it runs counts as 1 | The sum with the per-circle part is clamped to 1 to 20 (`MaxTableSummonCount`); a formula that fails to compile is `formula` at `.summon.count`; a plain number above `max_live` (the number clamped to 20) is `value` at `.summon.max_live` |
| `count_per_slot_level` | Creatures added for each circle above the spell's own | 0 to 10 |
| `turn_mode` | `own_initiative` (the default) or `owner_roll_group` | `after_owner` is refused |
| `role` | `companion` | `mount` is refused |
| `max_live` | The most creatures of the spell the caster keeps; a casting that would pass it is refused; 0 is no limit of its own | 0 to 40 |
| `replaces` | `none` (the default) or `same_summon` | |
| `resource_key`, `resource_cost` | What a feature's summon spends | Refused on a spell (`value` at `.summon.resource_key`) |
| `casting_time`, `duration`, `concentration` | What a feature's summon takes to cast and how long it lasts | Refused on a spell, which has its own |

The spell is a leveled spell with only the caster as target, and with no attack, saving throw, damage, healing or area.
It is not a ritual: the cost of a summon is a slot (`value` at `.ritual`).
A refusal of a summon's own field is a violation at `.summon.<field>`.
The target is refused at `.target.kind`, and a cantrip or a spell with damage, healing, a saving throw, an attack or an area at `.summon`.
`slot()` is legal in a summon's count as in a creature's formulas, and nowhere else.
An archived or switched-off creature is not offered by a summoning spell and is refused in a cast (`ErrSummonCreature`); the creatures already made keep working.
A player reads the summon of a spell or a feature in `ListTableEntries` as a list of creature keys and nothing else.
Of those keys, a table creature stays only while a live creature of that key belongs to a character of the player; an SRD creature stays.
The count, its growth, the limit, the turn mode, the role, what is replaced, the cost and the timing are the master's.
A field that `TableSummon` gets later is the master's until it is named in `playerSummonFields` (`characters/tablecontent.go`).
The caster reads the creatures she may pick, with their key and name, in `GetSummonOptions`, before she holds one.
Tests: `TestATableSpellSummonsThroughTheSRDChecks`, `TestATableSummonIsRefusedAtTheFieldThatIsWrong`, `TestSlotInASummonCountOnlyInASummonCount`.

## Table feature summons

A `TableFeature` of a class, a subclass, a race trait or a background, and a `TableClassFeature` entry, may carry a `summon` (`TableFeature.Summon` or `TableClassFeature.summon`, a `TableSummon`).
It has the same creatures, count and limits as a spell's summon.
It is spent with a use of a resource instead of a slot.
An option carries none (`value` at `.summon`).
The holder casts it through the same options, checks, concentration, replacement, dismissal and undo.
It is not a spell of the holder's list: a sheet cannot list it.
No list, search or entry shows it.
Only the master and a player whose character has the feature's entry read its details.
The summon takes a spell key made from the feature's key.
If another entry already has that key, the feature is refused at `.key`, and the message names the key.

| Field | Meaning | Limits |
| --- | --- | --- |
| `resource_key` | The resource a casting spends a use of | A resource the feature's entry defines with a `resource` effect, or an SRD resource; otherwise `reference` at `.summon.resource_key` |
| `resource_cost` | Uses a casting spends | 0 or 1 |
| `casting_time` | Time to cast | An action by default |
| `duration`, `concentration` | How long the creatures last, and whether the holder concentrates | Instantaneous by default |
| `count_per_slot_level` | No slot is spent, so nothing is added for a higher circle | Must be 0 |

`slot()` is 0 in the count of a feature's summon and in the formulas of its creatures.
How the uses come back is the resource's `recharge` (short rest, long rest, dawn and the others the `resource` effect accepts).
A `role` of `mount` and a `turn_mode` of `after_owner` are refused as in a spell.
Tests: `TestATableFeatureSummonsThroughTheSpellChecks`, `TestTheSpellOfAFeatureSummonIsHidden`, `TestTheSpellOfAFeatureSummonIsNoChoiceOfASheet`, `TestAFeatureWithALongKeyKeepsASpellKeyWithinTheLimitAndATakenOneIsRefused`, `TestATableFeatureSummonIsRefusedAtTheFieldThatIsWrong`, `TestASpellSummonStillTakesNothingOfAFeatureSummon`.

## Entries the rules refuse on reading

A write is checked, so a stored entry is valid when it is saved.
A later revision of the effects, or a package, can make it invalid.
`TableSource` then builds the content again without the entries the rules refuse (`buildTolerant`).
It blames each violation that names an entry by its key, leaves that entry out and builds again.
Each build leaves out at least one live entry or ends the read, so a campaign takes at most one build more than it has entries.
The engine stops at the first entry it refuses, so 20 refused entries take 21 builds.
A subclass of a class left out fails the next pass with `dangling_reference` and is left out too.
A violation with no entry key, or one that names an entry that is not live, fails the read.
The read never returns a part.
A campaign with no refused entry costs one build.
`Content.Refused()` lists what was left out, and is cached with the content for its revision.
`ListTableEntriesResponse.left_out` gives it to the master of the campaign and to nobody else.
A sheet that uses a left-out entry gets `unknown_key`, as for an entry that does not exist.
A write of the master that repairs a left-out entry is accepted, as are archiving, unarchiving and the option switches; a write is refused for its own violations and for the healthy entries it breaks.
An import is checked over the stored entries as they are stored, so a pack that breaks one is refused.
Rows that cannot be decoded fail the listing, the export, the import and every write.
The deploy audit of the same rules is in the [runbook](../operations/runbook.md#entries-a-new-revision-would-leave-out).
Tests: `TestBuildTolerantLeavesOutManyRefusedEntries`, `TestTableSourceReadsWithoutTheEntriesTheRulesRefuse`, `TestTableContentWritesWorkWithARefusedEntryStored`, `TestListTableEntriesTellsOnlyTheMasterWhatIsLeftOut`, `TestAuditTableContentLogsWhatTheRulesWouldLeaveOut`.
