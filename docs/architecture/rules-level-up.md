# Rules module: level-up

Three pure functions in `rules/levelup.go` say what a level gives and what a locked sheet may change when it levels up (RN-01, RN-12, MR-040).
They use no database, clock or randomness: the hit die is rolled outside and enters as a number.
The part that needs the database (who may call, the kept die roll, the record) is in [Characters](characters.md#leveling-up).
For the engine around them see [Rules module](rules.md).

| Function | What it does |
| --- | --- |
| `LevelUpOptions(before, classKey, c)` | What the next level of that class gives, as data the screen draws: whether it has an Ability Score Improvement, the hit die and its average, how many new cantrips and spells (from which list, up to which spell level), the new maximum of prepared spells, the subclass, the options of the level's features, the skills and expertise, and what changes by itself (slots, proficiency bonus, the new features by name). Computed from the `Derive` of the sheet before and after, without choices. |
| `ApplyLevelUp(before, choices, c)` | The `Build` the choices make: the class with one more level, plus the choices. Hit points follow the sheet's method: the average on a fixed-HP sheet writes nothing; a die on a fixed-HP sheet turns the sheet into "rolled", with the average in the earlier levels; on a rolled-dice sheet the average enters as a roll. It judges nothing. |
| `CheckLevelUp(before, after, c)` | Refuses everything the level does not give. |

**What `CheckLevelUp` accepts.**
The only differences are:

- one of the character's classes with one more level (or a new class at level 1, see [Multiclassing](#multiclassing-at-level-up));
- the ability score improvement, only at a level that has one: +2 on one ability or +1 on two, none above 20;
- a feat in place of the improvement (below);
- the new hit points entry (1 up to the die) with the earlier levels equal;
- exactly the cantrips and spells known or in the spellbook that the table gives (two per level for the Wizard), and new prepared spells, none removed;
- the subclass at its level;
- exactly the options, skills and expertise the level's features give, and any choice an earlier level left open (`late_choice`).

Everything else (name, race, background, base scores, armor, shield, weapons, the other classes) is `locked_field`.
At the end the new sheet may not have an `Issue` the old one lacked (a spell outside the class list or above the level, too many prepared): that is `sheet_issue`.

**Refusals.**
Each refusal is a `*LevelUpError` with the field (in the proto's names, e.g. `full.cantrip_keys`) and a stable reason code, which the screen translates to Portuguese; the message is never read.
The codes are the `LevelUpReason*` constants in `levelup.go` and the `LevelUpRefusalReason` enum in `characters.proto` (which adds the ones the server raises itself: the hit point roll, the table's hit point rule, archived and switched-off choices, feats not allowed, `SHEET_NEEDS_MASTER`).
A player's request whose `feature_choice_keys` or swap `to_key` is not among the options `GetLevelUpOptions` offers that character at that level (after `hideFromPlayer`) is refused before anything is derived (`unofferedRefusal`, `levelup_offered.go`): `PreviewLevelUp` answers the refusal alone, with no `after` and no multiclass summary, and `LevelUpCharacter` refuses it. The master's reads are unchanged.

**One source for the offer and the check.**
What the options say and what the check demands come from the same numbers (`levelUpOptionsWith`).
What a chosen subclass adds (the Lore College's three skills, the Circle of the Land's cantrip and terrain, a third caster's spells, the options of its features) is in each candidate subclass's own part, `LevelUpSubclass`, because the level has no subclass yet when the offer is read.
`LevelUpSubclass` also says whether the table archived it (`Archived`) or switched it off (`Off`, its own switch only).

- The Warlock's new invocations come from the `invocations_known` column of the class table (corrected in `effects/corrections.json`).
- Swapping an invocation away takes its picks with it: the Book of Ancient Secrets' two rituals go with the invocation, and the swap stays one removal.
- Removing one of those picks alone is refused.
- A plain option the level picks (the Pact of the Tome, the Book of Ancient Secrets) opens a choice of its own, and the level may leave it open: `checkChoiceProblems` lets a missing choice through when the sheet before did not ask it and the option that opens it is new.
  The next level lists it in `LateChoices` and refuses the level without its picks (`late_choice`).
- The Bard's Magical Secrets (levels 10, 14 and 18) let some of the new spells come from any class's list (`AnyClassSpells`); `Derive` stops flagging those spells as off-list.
- `MasterAdds` lists features of the level whose choice the guided level-up does not cover and the master adds in the editor.
- **Not covered** by the guided level-up: a custom multiclass or subclass, and **swapping a known spell for another**, which the SRD allows Bard, Ranger, Sorcerer and Warlock.
  Level-up only adds, never removes.
- A repeated key in any list is refused.

**Feats at level-up.**
A feat replaces the Ability Score Improvement (`LevelUpOffer.Feats` at those levels, `LevelUpChoices.Feat`).
`CheckLevelUp` judges the prerequisite on the sheet with the level gained and without the feat, applies exactly the feat's own increase and keeps every ability at 20 or less.
A feat that raises every ability it lists is still taken at 20: the increase just stops there and the other effects apply.
A feat that lets the player choose never offers an ability at 20 (`FeatOption.Capped`).
A feat taken in place of an improvement is recorded in `Build.FeatSlots`: the sheet lists it as "Talento · Classe nível" and no longer lists the improvement it replaced.
Whether the table allows feats (`tablerules.Rules.FeatsAllowed`) is for the server to check.

**Swapping a pick at an improvement.**
`LevelUpOffer.PickSwaps` (`levelup_swap.go`) lists, at a level with an Ability Score Improvement, the groups with something to swap: `cantrip` (the class's cantrips and the other cantrips of its list), and the option sets `fighting_style`, `metamagic`, `invocation` and `pact_boon`.
A group's pool is the options of the features that offered the picks (a fighter's styles, not the paladin's), the table's options included, without the picks the sheet has; a warlock's offer leaves out `invocation` because its own swap covers it.
`LevelUpChoices.Swap` gives up `From` for `To`: `ApplyLevelUp` removes one and adds the other, and `CheckLevelUp` finds the swap by the pick the new sheet lost (`swapOf`), lets that one removal through (the cantrip count rises by one) and judges the new pick like any other: the choice engine's `prerequisite` and `not_offered` problems, the spell list.
A pick lost with nothing in its place is still "an option was removed".
A swap of the `pact_boon` or of the `invocation` takes the picks of the choice the pick opened with it (`ownPick`).
Swapping the Pact of the Tome away drops its `#cantrips` picks.
Swapping the Book of Ancient Secrets away drops its `#rituals` picks.
The swap stays one change.
Whether the table plays with the swap (`tablerules.Rules.PickSwapAllowed`) is for the server to check, as for feats: `GetLevelUpOptions` leaves `swap_groups` empty and `LevelUpCharacter` refuses with `SWAP_NOT_ALLOWED` (`choices.swap_pick`).

**Swapping an option of a table feature.**
A table class or subclass feature that has options may carry `TableFeature.swap_levels` (`FeatureSwapLevels`: `levels`, the class levels 1 to 20 each at most once, or `every_level`, true for every class level from the feature's own level on; never both).
The overlay refuses it (`addSwapLevels`, `featureswap.go`, `bad_value`) at `.swap_levels`, `.swap_levels.levels[i]` or `.swap_levels.every_level` on a feature that is not a class or subclass feature, on an option, and on a feature with no options of its own and none offered by another feature; the data is kept in `content.featureSwaps`.
`LevelUpOffer.FeatureSwaps` lists, for the class that gains the level (a subclass feature needs that subclass chosen), each owned feature whose `swap_levels` allows the new class level and that has something to swap, as a `SwapGroup` whose `Group` is the feature's key, the picks are the character's picks in the pool and the pool is what is offered.
The pool is the feature's options plus the options of the entry's other owned features that share an option with it, transitively (`poolOptionsOf`, over `groupPools`), without the picks the sheet has and without archived or switched-off options.
The permission is the feature's own data: it does not depend on `PickSwapAllowed` or on an Ability Score Improvement at the level, and `LevelUpChoices.Swap` and the cantrip swap work at the same level.
`LevelUpChoices.FeatureSwaps` (`[]PickSwap`) gives up `From` for `To`: at most one per group, `From` values distinct, `To` values distinct, each one of the offer's, else the swap is refused (`SWAP_NOT_ALLOWED`).
`ApplyLevelUp` replaces the pick in place (the choice engine deals the picks in order); `CheckLevelUp` lets exactly the offered removals through (`unmatchedRemovals`), and `CheckLevelUpOf`, which the level-up calls with the choices the sheet was made from, only the removals a sent swap names (`sentSwaps`); `applyPicks` takes a pick out through `SwapInvocation` only for the Warlock (the check refuses the level unless that pick is an invocation); the check judges the new pick on the sheet after the level (prerequisite, no repeat).
A history record is never rewritten.
The sheet text of a feature with `swap_levels` carries one more paragraph, "**Troca:** nos níveis 5 e 6 de Guerreiro você pode trocar uma opção desta característica por outra." ("em todo nível de <Classe> a partir do nível N" for every_level; markdown, the lead in bold), saying at which class levels of the class a pick may be swapped (`withSwapLine`, in both `Description` and `DescriptionPT`).

## Multiclassing at level-up

SRD 5.1 "Multiclassing", in `levelup_multiclass.go`.
A class the character does not have is a valid `class_key`: `levelUpClass` gives it the index past the last class, and the three functions treat it as a class at level 0 whose first level is the level.
The new class goes last in `Build.Classes`, so the first class stays the starting class (saving throws, level-1 hit points).

- **Prerequisites.** The main ability of every class the character has and of the new one, on the final scores, must be 13; the Fighter asks Strength or Dexterity, the Monk, Paladin and Ranger both of theirs.
  The numbers are the class's `Multiclass.Minimums` and `AnyOf`; table classes carry their own.
  `LevelUpClasses` lists the ways the level can go for the class step (the classes the character has, then the others by Portuguese name, each with its prerequisite and why it is closed).
  The refusals are `multiclass_prerequisite` (the new class) and `multiclass_prerequisite_current` (a class the character has, which closes every new class), with `class_key`, `ability`, `minimum` and `have`.
- **Experience, proficiency bonus, hit points** all follow the total level.
  The new class's die gives its average, `die/2 + 1` plus the Constitution modifier (at least 1), never the maximum, which only the first level takes.
  A rolled sheet takes the roll at its place in the class order.
  Hit dice stay one pool per die size.
- **Proficiencies.** Only the class's multiclass table (`Class.Multiclass.Proficiencies`, which `Derive` already grants to every class after the first).
  The skill (Bard: any; Ranger and Rogue: their list) is a pick checked against the list and what the character has (`proficiency_choice`).
  The Bard's musical instrument (`multiclass_corrections` in `effects/corrections.json`, since the snapshot lists the ten instruments but the importer keeps only the skill) is a pick written to the sheet's free-text `tool_proficiencies` (`instrument_choice`).
  A proficiency an armor, weapon or category already covers is shown as had.
  No starting equipment, ever.
- **Class features.** The level-1 features of the new class come as for any class.
  Extra Attack takes the larger count, Channel Divinity shares one pool with the larger maximum, and Unarmored Defense counts only the first of the Barbarian's and the Monk's.
  Fighting Style options already taken, by name, are not offered again.
- **Spellcasting.** Each class counts as if alone: cantrips and spells known or in the spellbook (a new Wizard starts with six) and prepared spells are the class's level-1 numbers.
  Slots follow the multiclass spellcaster table when more than one class has the Spellcasting feature: all levels of Bard, Cleric, Druid, Sorcerer and Wizard, half (rounded down) of Paladin and Ranger, a third (rounded down) of a casting Fighter or Rogue subclass.
  With one casting class, its own table.
  Pact Magic is apart and counts for neither; the SRD does not say how a second Pact Magic class behaves, and `Derive` keeps the last class's slots.
- **Subclass and Ability Score Improvement** follow the class level, not the total level.
- **Limits.** Total level 20 (`max_level`); an archived class is not a new choice and a class the master switched off is not offered to a player (the same refusals as any table entry).

`MulticlassSummaryOf` gives the summary the preview shows when the sheet has more than one class after the level: the caster levels before and after, whether the table gives the slots, the slots and hit dice, the proficiencies gained and the exceptions the level meets.
`LevelUpOptions` carries `class_choices`, `is_new_class`, `proficiency_gains` and `proficiency_choices`.
The guided level-up's first step is always "Subir em qual classe?".

## Tests

`TestLevelUpSweep` levels the 12 SRD classes from 1 to 20, with each SRD subclass, doing exactly what the options ask, and checks that `CheckLevelUp`, `Validate` and `Derive` accept each step with no `Issue`.
`TestLevelUpSweepTable` does the same with table classes of each casting type (see [Table content](rules-table-content.md#quality-gate)).
`levelup_multiclass_test.go` holds each multiclass rule above, and the refusal tests have one line per reason.
