# Rules module: table content (`Content.With`)

The content a master registers enters the engine as a layer over the SRD (MR-025, RN-23, ADR-0018): `(*Content).With(Overlay)` returns a new `*Content`, the SRD plus the table's entries, and never changes what it received.
`rules` stays pure; who stores the entries is [Characters](characters-table-content.md), and the editors are in the web app.
The code is in `rules/overlay*.go`.
For the engine around it see [Rules module](rules.md).

## The overlay

| Piece | What it does |
| --- | --- |
| `Overlay` | The revision, the keys switched off, the entries of eight kinds (`TableClass`, `TableSubclass`, `TableRace`, `TableSubrace`, `TableBackground`, `TableSpell`, `TableFeat`, `TableOption`) and `Strict`, the keys being written now. Each entry starts with a `TableEntry` (key, Portuguese name, `Archived`, the revision and date of its last change) and carries the features (`TableFeature`: key, name, text, effects). The fields follow what the editors fill in, so the protos map one to one. |
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
| Entries (of the eight kinds; features do not count) | 1,000 (`MaxOverlayEntries`) |
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
| `modifier`, `proficiency`, `sense`, `roll_mode`, `extra_attack`, `note` | As in [Effects](rules.md#effects). |
| `proficiency` with `only_proficient` | A switch of the effect with level `expertise` on a skill (`skill:*` is every skill): the skill is doubled only where the character already has its proficiency from another source when the sheet is worked out (the race, the background, the sheet's picks or another feature), never from the effect itself or from another `only_proficient` effect. Without the switch the effect doubles the skill, proficient or not. Any other level, a target that is not a skill and any other effect type refuse it at `.only_proficient` (`bad_value`). It is a `yes_no` field of the proficiency type in the effect menu. |
| `resource` | With uses and recharge. The resource may not have the name of an SRD one. |
| `grant_action` | May carry `resource`: the name of a `resource` effect of the same entry (a feat included), its class, or an SRD resource. Each use spends `cost` of it (1 when empty) and the action is off while fewer are left; see [Costs, healing actions and hooks](#costs-healing-actions-and-hooks). Refused as `bad_reference` when nothing defines the resource. |
| `on_event` | The owner's hook on `short_rest`, `long_rest`, `death_save_failed` or `drop_to_zero`; see [Costs, healing actions and hooks](#costs-healing-actions-and-hooks). |
| `ranged_attack` | Checked at its fields: `applies` is `spell` or `weapon`; `range_multiplier` is 2 or 3 and only on `spell`; `no_long_range_disadvantage` only on `weapon`; an effect that sets none of the three switches is refused at `.type`; `tags` at `.tags`; the switches on any other type are stray fields. |
| `resistance` | Resistance, immunity or vulnerability (`kind`: `resist`, the default, `immune` or `vulnerable`) to the `damage_types` it lists: `damage-type:` keys of the content, `all` (alone) or `nonmagical_bps` (bludgeoning, piercing and slashing from attacks that are not magical); or to every type but `except`. `except` and `damage_types` exclude each other and one is required. `while` is empty (always), `rage` or the `state_name` of a `state` effect of the same entry (`.while`: `bad_reference` for a state nothing defines). Refusals name the field: `.damage_types` (unknown type, empty, a type twice, `all` mixed with others, both `except` and `damage_types`), `.except` (`bad_reference`), `.kind`, `.while`, and `.when` or `.tags` (`bad_value`): `while` is the condition. See [Effects](rules.md#effects). |
| `state` | A temporary state the character turns on and off: a stance, a form. `state_name` (1 to 40 of `a-z`, `0-9`, `_`; the name the entry's other effects read it by; unique in the entry), `duration_rounds` (an Int formula in rounds, 0 to 14,400; empty or 0 lasts until it ends), `ends_on` (any of `dismiss`, `incapacitated`, `short_rest`, `long_rest`), `economy` (`action`, `bonus_action` or `free`, what turning it on costs) and optionally `resource` (as in `grant_action`). At most 8 states for an entry (`limit`). See [States](#states). |
| `weapon_rule` | Changes the weapon lines its filter picks. `weapon` is `any`, `melee`, `ranged`, `finesse`, `magic` or `named` (then `weapon_name` is the Portuguese name of the weapon); `attack_ability` and `damage_ability` name another ability (the better of it and the weapon's counts); `attack_bonus` and `damage_bonus` are -5 to 10; `reach_ft` (5 to 60, in steps of 5) is a longer melee reach. At least one ability, bonus or reach must be set (refused at `.type`); `weapon_name` is required with `named` and refused otherwise; a reach is refused on `ranged`; no `tags`. Gated by `while` and `when`. |
| `pseudo_weapon` | An attack line of its own on the sheet: `attack_name`, `weapon` (`melee` or `ranged`), `damage_dice` (`NdS`, N up to 10, S a weapon die), `damage_type`, `ability`, `reach_ft` (melee, 5 to 60, optional) or `range_ft` (ranged, 5 to 300, required; each kind refuses the other's field). It is never an item, counts as a weapon the character is proficient with, and the weapon rules change it too. Gated by `while` and `when`. |
| `condition_immunity` | `conditions`: the keys of the conditions (`condition:poisoned`, one of the 15 of `combat_effects.json`), listed once each, at least one; refused at `.conditions`. The character takes none of them (see [Effects](rules.md#effects)). |
| `breathes_water` | The character breathes underwater (`Derived.BreathesWater`). No field of its own: a stray one is refused, `when` and `text_pt` are allowed. |
| `granted_spell` | See [Granted spells](#granted-spells). |
| `extra_damage` | See [Extra damage](#extra-damage). May carry `while`. |
| `choice` | Picks among SRD options (a key the SRD has) or, for kind `feature`, among the option list of a table feature. Kinds: `skill`, `expertise`, `cantrip`, `spell`, `language`, `tool`, `feature`, `feat`. A `feature` choice never asks for more than the options it lists. |
| `ability_increase` | Feats only. `count` different abilities from `from`, `value` 1 or 2; at most one per feat. The sheet keeps the result in `ExtraAbilityBonuses`. |
| `ambush` | See [Ambush](#ambush). |
| `spell_modifier` | See [Spell modifier](#spell-modifier). |
| `superiority_die` | See [Maneuvers](#maneuvers). |
| Spells in a `note` | A `note` with `spells` (like the SRD's Infernal Legacy): the character knows the cantrip, or a spell once per day when the same feature has a `resource`. The spell is on the sheet's list and does not count in the class numbers. Only a `note` carries `spells`. The structured way is [`granted_spell`](#granted-spells). |

Never allowed: `handler` (not even one that exists, since a handler reads columns of an SRD class a table class lacks), `spellcasting` and `wild_shape` (the engine writes casting from the class), `replaces`, `beast_spells`, `song_of_rest`.
A text-only feature always holds.

**Option lists.**
A class or subclass feature may carry `TableFeature.options` (a table's maneuvers).
Each option is a feature of its own (name, text, effects) with a server-made key (`feature:<stem><option name>@mesa`) and the offering feature as its `Parent`.
An option is in no level row: it is gained only when picked, and applies (its effects, its action under the parent) only while the character has the parent.
A `feature` choice with an empty `from` offers all the feature's options; on another feature of the same entry, `from` may list option keys (a later level's "two more").
The choice engine deals one pool of picks to choices that share options, in level order, so options an earlier choice took are blocked in the later one, and a pick whose parent is gone is reported as no longer valid.
Refused, each at its field: options on a trait or a background feature, nested options, options with no `choice` of features, a repeated option name or `from` key, a `from` key of another entry's list, a `choice` inside an option, more options than the limit.

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
How combat spends the die (damage, grapple, reduction) is in the [play module](play/README.md).

**Dice on the character's own rolls.** `applies` may also be `check` (an ability check; `targets` lists the skills `skill.stealth` and abilities `check.dex` it is limited to, empty is every check), `save` (a saving throw; `ability` limits it to one) or `initiative`. These cost no action (`.economy`), `targets` belongs to `check` alone (`.targets`) and `ability` to `save` (and the other maneuvers that already name one). `Derive` copies `Targets` into `Derived.Maneuvers`. Play spends them on the scene check, the concentration save, the effect save and the initiative ([Rests and resources](play/rests-and-resources.md#table-dice-on-the-characters-own-rolls)); the derived sheet lists them as `DerivedSheet.roll_dice`. A number that is always on is a `modifier` (`skill.*`, `save.*`, `initiative`, `ac`); an always-on die has no effect type.

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

## Table options

`TableOption` (`rules/overlay_option.go`) adds an option to one of the SRD's option sets.
Its key is `option:<set>:<slug>@mesa` and `Set` is the set the key names: `invocation` (the Eldritch Invocations), `pact_boon`, `metamagic` (the three Metamagic features), `fighting_style` (the fighter's, the additional one, the paladin's and the ranger's), `hunters_prey`, `defensive_tactics`, `multiattack` and `superior_hunters_defense` (`optionSetDefs`; the menu's closed list `option_set_names` carries them with their Portuguese names).
`With` writes the option as a feature of the clone and appends its key to the `Options` of every SRD feature that offers the set (the offering features are copied first: the SRD's are shared), and to `offeredBy`.
So the choice engine needs nothing of its own: the picks are dealt in the pool of the set, a set with table options is not bound by the 20 keys of a listed `from` (the pool is computed), the option is a feature once picked (its effects apply while the character owns an offering feature), and the same option cannot be taken twice by name.

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
Checked at its fields: `.class_key` (a class of the content), `.level` (1 to 20), `.replaces_feature_key`, `.desc_pt` and `.effects`.

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
  A subrace adds bonuses and traits.
- **Background:** two skills, tools, languages to choose, the equipment as text and the feature, usually a note.
  A table background's equipment reaches `Derive` (`background_equipment_pt`).

## Feat

`TableFeat` (`feat:<name>@mesa`) has a name, text, a `FeatPrerequisite` and effects (the feat owns them; it has no features).
The prerequisite is all of: minimum scores that must all be met, scores of which one is enough, a proficiency key, "can cast at least one spell", a race or subrace key, a total level.
`Content.Feats`, `FeatOptions(build)` and `CheckFeat` answer who qualifies and what they lack (`FeatUnmet`).
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

The master switches classes, subclasses, races, subraces, backgrounds, spells and feats on or off per campaign, SRD and table alike; everything is on by default.
`Overlay.Off` carries the switched-off keys to `With`, which stores them on a copy and ignores a key that is not switchable (`Content.Switchable`).

- `Content.Off(key)` is the key's own switch.
- `Content.Hidden(key)` is what the player does not receive: archived, switched off, or the subclass or subrace of a class or race that is switched off or archived.
  The children keep their own switch, so turning the class back on returns each subclass as it was.
- A parent only hides the children of whoever does not have it: `Content.OffKeys(build)` lists the switched-off keys a sheet uses, and a subclass (or subrace) under a switched-off class (or race) counts only when the sheet does not have that class (or race).
  A sheet that already has the switched-off class keeps reaching its subclass level; a new sheet cannot take the class, so not the subclass either.
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
- **`EffectMenu`** returns the effect types of the menu with their Portuguese names and sentences, each one's fields (required or not, the field's kind and list, a number's limits, `always_shown` for an optional field the editor never folds away), and every closed list with Portuguese names: modifier targets, modes, proficiency targets, levels, advantage modes and targets, senses, recharges, `events`, action economies, choice kinds, `ranged_attack_applies`, `damage_types`, `resistance_damage_types`, `resistance_kinds`, `resistance_conditions`, `conditions`, `granted_spell_abilities`, `state_ends`, `state_economies`, `weapon_filters`, `weapon_kinds`, skills, languages, tools and tag prefixes.
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
