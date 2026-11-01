# Vitals, rests and class resources

The numbers that change during play live in `character_vitals`, in the `characters` module, and `play` changes them through a seam.
Rests, hit dice and the class resource flows are `ResourceService` (`resources.proto`), which `play.Service` implements.

## Vitals

`character_vitals` holds current HP, temporary HP, spell slots used per level, the slots Flexible Casting created, pact slots used, hit dice used by die size, resource uses spent, exhaustion level and the maximum-HP bonus (see [Data model](../../data/README.md#modules-and-tables)).
They belong to the character and last from one session to the next.
`play` reads and changes them through `play.VitalsKeeper` (`ListVitals`, `GetVitals`, `AdjustVitals` and more), which `characters.Service` implements.
The messages (`CharacterVitals`) belong to `play.proto`, because only `PlayService` serves them.

- **The maximums are never stored.** They come from `rules.Derive`: maximum HP, slots per level, pact slots and hit dice (the total is the level).
  On every read the stored value is clipped to today's maximum.
- **Aid's bonus is stored.** `character_vitals.hit_points_max_bonus` (0 without Aid) is added to the derived maximum on every read: `CharacterVitals.hit_points_max` is the effective maximum, current HP are clamped to it and `AdjustVitals` validates against it.
  `VitalsKeeper.SetHitPointsMaxBonus` changes it in the caller's transaction: a bonus that grew raises the current HP by the same amount, one that fell cuts only the excess and never the last HP of someone who still had one.
  `CarryHitPoints` keeps the bonus when the sheet's maximum changes.
- **With no row, the character is whole:** full HP, nothing used, `revision` 0.
- **Only living, active player characters** have vitals.
  An NPC gets combat HP on its combatant.
- The master's correction and the nudge are in [Live session](live-session.md).

## The master's rests

`GetRestPreview(kind)` lists, for each living active player character, what a short or long rest would give back, and writes nothing.
`TakeRest(kind, hit_dice_choices)` does it in one transaction: it locks the open session, checks the idempotency key (the request's hash is kept with the `rest_taken` event, so the same key for another request is `invalid_argument`), changes every character the rest touches, writes one `rest_taken` event (kind and character ids) and, after the commit, sends each changed character's vitals (`vitals_changed`) and, for a combat in setup, `encounter_changed`.
A character the rest changes nothing for keeps its revision.
What each kind restores follows SRD 5.1:

| | Short rest (at least 1 hour) | Long rest (at least 8 hours) |
| --- | --- | --- |
| Resources | Those that recharge on a short or long rest (Ki, Channel Divinity, Second Wind, Action Surge, Wild Shape, Bardic Inspiration from bard level 5, Breath Weapon, Stroke of Luck, Dark One's Own Luck) | All of them (also Rage, Sorcery Points, Lay on Hands, Divine Sense, Cleansing Touch, Indomitable, Arcane Recovery, Natural Recovery, Wholeness of Body, Hurl Through Hell, Relentless Endurance) |
| Spell slots | Only the warlock's pact slots | All; slots Flexible Casting created vanish |
| Hit points | Not restored (the player spends hit dice) | Restored to the maximum; temporary HP end (which ends Aid given as temporary HP) |
| Hit dice | Not restored | Half of the total, at least one, by the player's choice of die sizes |
| Exhaustion | Unchanged | One level lower |
| A character at 0 HP | Changes what it changes | Nothing: "a character must have at least 1 hit point at the start of the rest" |

The recharge of each resource is the effect's (`recharge`, and `recharge_if` for Bardic Inspiration), so `rules.RestRecovery` is the only rule: `short_rest` means "short or long rest", `long_rest` waits for the long one, and `dawn` and `none` are restored by no rest.
The table's content adds `restore_one_on_short_rest` (one use on a short rest, all on a long one) and `restore_one_on_long_rest` (one use on a long rest, none on a short one): `restResources` takes one use off `used` for `RecoverOne` and sets the preview's `back` to 1.
A rest is refused with `COMBAT_OPEN` while a combat is running.
A long rest, in the same transaction, deletes the campaign's `npc_spell_use` rows and empties the count of the persistent NPCs in a combat in setup (an ended combat is not touched), so every persistent NPC has its slots, uses, uses a day and Legendary Resistance back (see [Creatures](creatures.md#kept-until-the-long-rest)).
A short rest, in the same transaction and before the spells, removes the `rest:` keys from the campaign's `npc_spell_use` rows and from the persistent NPCs in a combat in setup, and drops a row left with nothing spent.
It leaves the slots, the uses a day and Legendary Resistance.

**The clock.** The SRD allows one long rest in 24 hours and the app keeps no game clock, so it records the rest and warns: `GetRestPreview.long_rest_already_taken` says the open session already had one, and `TakeRest` never refuses for it.
**Not tracked:** the death saves (they live on the combatant of one combat).
The slots Arcane Recovery and Natural Recovery give back are chosen by the player (see below).
A rest also moves game time on (`advanceGameTime`), which ends effects whose seconds run out (see [Lasting effects](lasting-effects.md)) and spells that last (see [Casting](casting.md)).

### The table's hooks on a rest

A table feature can carry an `on_event` effect for `short_rest` or `long_rest` (see [Costs, healing actions and hooks](../rules-table-content.md#costs-healing-actions-and-hooks)): the owner's alone, never the allies who rest nearby (that stays text).
`planRestWith` runs the character's hooks last (`restHooks`, from `vitalsMax.hooks`, which `maxima` takes from `Derive`): a hook that spends a resource runs only with the uses left and spends them, `heal` (rolled by the app with `Service.roller`; `PreviewRest` shows the average) and `heal_percent_max` heal up to the maximum, `temp_hp` is given after a long rest ended the old temporary hit points (so the rest does not wipe the hook's gift) and keeps the larger number, and `restore` gives back uses, never more than are spent: a rest taken again changes nothing, and a character a rest changes in nothing keeps its revision.
The result goes where the rest already goes: the vitals after (sent to the streams by `publishRest`) and the master's preview (`RestPreview.hooks`).

**Initiative.** A resource that recharges as `initiative` gets one use back when its owner's initiative is rolled (`SubmitInitiative`, `regainOnInitiative`, in the transaction of the roll) and only if none is left (`rules.InitiativeRecovery`).

### Hit dice by die size

`character_vitals.hit_dice_used_by_die` is a JSON object `{die size: count}` (SRD 5.1, "Multiclassing": hit dice of different sizes are kept apart); `hit_dice_used` keeps the sum, so the two never disagree.
A row with `NULL` there reads its count as the largest dice spent first (`rules.SplitHitDiceUsed`), so nobody gets more healing than the old number allowed; the next write stores the object.
`SpendHitDice(character, faces)` spends one die of the size on a short rest: it is rolled in the app or typed from a real die (RN-18), the character regains the roll plus the Constitution modifier (never below 0), cut to the maximum, and the answer says the face, the modifier and what was healed.
The character's player or the master may call it, with no combat running.
The app does not require a short rest to have been taken first; the table is trusted, as for the clock.

### Song of Rest

Song of Rest (SRD 5.1, bard 2) gives an extra die when a creature regains HP by spending hit dice during a short rest.
The whole party hears it while a bard with the feature and at least 1 HP is in the campaign's party.
`SpendHitDice` adds the die when:

1. the latest rest of the open session is a short rest (`GetLatestRestKindInSession`);
2.
   `RestKeeper.SongOfRest` finds a singer (the largest `Derived.SongOfRestSides`, `rules.SongOfRestFor`);
3. the character has no `hit_dice_spent` event with a `song_sides` since that rest (`HasSongOfRestSinceLatestRest`): once per rest, with the first die spent.

An app roll rolls the extra die on the server.
A typed roll must send `typed_song_face`, or the call is refused with `ResourceBlocked` `SONG_OF_REST_FACE_NEEDED` (the die size in `needed`) and nothing is written.
The answer and the event carry `song_of_rest_sides` and `song_of_rest_face`.
Tests: `TestSongOfRest*`, `TestTheSongsDieRolledWithARealDieIsTyped` (`play`), `TestSongOfRestDieFollowsTheBardLevel` (`rules`).

Wholeness of Body (`wholeness_of_body`, one use, recharge `long_rest`) is the resource of an action in a combat: `TakeAction` spends the use and heals (see [Attacks and damage](attacks-and-damage.md#standard-and-feature-actions)).
A long rest gives the use back.

## Class resource flows

Each flow below, except the two slot recoveries, is a `ResourceService` call and an `action_taken` event with a `resourceEvent`; the master's undo takes them back (see [Undo](undo.md)).
Arcane Recovery and Natural Recovery are their own keyed events and have no undo.
The pure rules are in `rules` (`rest.go`, `resources.go`, `metamagic.go`); the numbers that change are `characters`' (`rests.go`), reached through `RestKeeper`.
The code is `resources.go` (rests), `combat_resources.go`, `combat_metamagic.go` and `combat_inspiration.go` (flows) and `resources_slot_recovery.go`.

### Flexible Casting (sorcerer 2)

`CreateSpellSlot(level)` turns sorcery points into a slot of level 1 to 5 as a bonus action, at the cost the class table says (2, 3, 5, 6, 7: `Content.SlotCreationCosts` reads the sorcerer's `creating_spell_slots` column).
The slot is stored in `character_vitals.spell_slots_created` and counts in the level's total with `SpellSlotUsage.created` set, until a long rest.
`ConvertSpellSlot(level)` spends one free slot for as many points as its level.
Sorcery points are the resource `sorcery_points`, whose maximum is the sorcerer level; a conversion past the maximum is refused (`SORCERY_POINTS_FULL`, `SORCERY_POINTS_OVER`), carrying the points and the maximum.

### Arcane Recovery (wizard 1) and Natural Recovery (Circle of the Land druid 2)

After a short rest, `UseArcaneRecovery(character, slots)` spends the resource `arcane_recovery` (one use, recharge `long_rest`) and lowers `used` of the chosen slots, in one transaction that locks the session, checks the key (scoped to the character) and writes the vitals and the `arcane_recovery_used` event (levels and counts only).
`UseNaturalRecovery` is the same call for the resource `natural_recovery` and the DRUID level, and writes `natural_recovery_used`.
Both run one path (`useRecovery`, `characters.UseSlotRecovery`) parameterised by a `rules.SlotRecovery` (resource key and class key).
Only the levels of that class count in a multiclass character, and the pact slots never do.
A character without the resource, such as a druid of another circle, is refused with `NOT_AVAILABLE`.
The master corrects a wrong recovery with `AdjustCharacterVitals`; there is no undo.
The rules are `rules.SlotRecoveryAllowance` (half the class level, rounded up) and `rules.SlotRecoveryCheck` (levels 1 to 5, combined level within the allowance).
The character checks are `characters`'; their refusals are `ResourceBlocked` reasons: `NOT_AVAILABLE`, `NO_USES_LEFT`, `SLOT_LEVEL_TOO_HIGH`, `SLOT_NOT_EXPENDED`, `OVER_ALLOWANCE` (with the level asked and the allowance).
The call needs the latest `rest_taken` of the open session to be a short rest (`NO_SHORT_REST` otherwise) and no combat open.
`CharacterVitals.arcane_recovery_allowance` and `natural_recovery_allowance` let the sheet word the allowance; each is 0 for a character without the feature.
The short rest sheet has one section for each recovery the character has.
With `feature_key` on `UseArcaneRecovery` the call is a table feature's `slot_recovery` effect: it spends a use of the effect's resource and keeps to the effect's highest slot level and combined level (`rules.TableSlotRecovery.Check`, run by `characters.UseTableSlotRecovery`), with the same refusals; a key the character does not have is `NOT_AVAILABLE`.
The `arcane_recovery_used` event carries the feature key.

### Lay on Hands (paladin 1)

`UseLayOnHands(actor, target, amount | cure)` is the action of the paladin's turn.
The touch (5 ft on a map; the master judges it in theatre) draws the amount from the pool (the resource `lay_on_hands`, 5 points per paladin level) and heals up to the target's maximum, through the same path as any heal; it spends the whole amount drawn even when less was needed.
A cure of a poison removes `condition:poisoned` for 5 points; a cure of a disease spends 5 and has no tracked effect (the master gives it).
"This feature has no effect on undead and constructs": the action and points are spent and nothing happens, and the answer says only that nothing happened, because a refusal on some targets would tell a player which creatures are alive (RN-10, RN-20).
The creature's type is read on the server; the reason is in the master's line of the log alone.
The healed number goes only to the master and the target's player.
`TakeAction` refuses the feature actions that have a flow of their own.

### Table dice on the character's own rolls

The `superiority_die` effects of applies `check`, `save` and `initiative` ([Rules: table content](../rules-table-content.md#maneuvers)) are dice the player adds to a roll of its character. The request names the maneuver (`maneuver_key`: `RollSceneCheck`, `ResolveConcentrationSave`, `RollEffectSave`, `SubmitInitiative`) and the derived sheet lists the choices (`DerivedSheet.roll_dice`: key, sides, resource, `targets` or `ability`). `combat_roll_maneuver.go`:

- `maneuverForRoll` checks the maneuver is the character's and fits the roll: a `check` die fits the skill (`skill.stealth`) or the ability (`check.dex`) of the check when it lists targets, a `save` die the ability of the save when it names one (the scene's `save:` actions are saves). Anything else is `invalid_argument`; no use left is `NO_USES_LEFT`.
- On the checks and saves the die joins the dice an effect adds (`effectDie.ManeuverResource`): it is rolled with them (in the app, or typed as the last of `extra_die_faces`), added to the total and shown as a source ("Nome +1d6: 4"). The resource is spent when the roll is written (`spendManeuverDice`, so a roll that waits for a Lucky or Bardic Inspiration question spends nothing), and the event keeps it in `ManeuverSpent`.
- On the initiative the die is rolled after the d20 is settled (in the app, or `maneuver_face` with physical dice) and added to the total (`initiativeManeuver`).

Not covered: the trap search, the puzzle hint, the group check, the checks of a contest, Hide and the Counterspell check, the death save, and the rolls made on someone else's call. Another creature's roll (an ally's) takes no die of the character.

### Indomitable (fighter 9)

The resource `indomitable` has 1, 2 or 3 uses (levels 9, 13 and 17) and recharges on a long rest.
It is spent when the player rerolls a failed saving throw (the held-roll question in [held rolls](held-rolls.md#indomitable-fighter-9-13-and-17)), through `spendResource` on the vitals.
Keeping the roll spends nothing.
The undo of a death save gives the use back (`spendResourceBack`).
The effect save and the concentration save cannot be undone, so the use they spent stays spent.
A saving throw rolled inside someone else's call never offers it.

### Stroke of Luck (rogue 20)

The resource `stroke_of_luck` has 1 use and recharges on a short or a long rest.
It is spent when the player uses it on a missed attack or a failed ability check (the held-roll question in [held rolls](held-rolls.md#stroke-of-luck-rogue-20)).
Outside a combat the use goes through the vitals (`AdjustVitals`), in a combat through `spendResource`.
Keeping the roll spends nothing.
The undo of an attack, a spell attack or a Counterspell gives the use back (`spendResourceBack`).
The scene, group, contest and Hide checks cannot be undone, so the use they spent stays spent until a rest.

### Metamagic (sorcerer 3)

`CastSpellRequest.metamagic` carries the options of a casting with their parameters.
`GetTurnOptions` lists, for each spell, the options the character knows (`SpellOption.metamagic_options`: cost, whether the spell takes it, and the reason in Portuguese when not), computed from the spell's SRD data by `Content.MetamagicOptions`:

| Option | Needs |
| --- | --- |
| Careful, Heightened | A saving throw |
| Distant | A range of 5 ft or more, or touch |
| Extended | A duration of a minute or more |
| Quickened | A casting time of 1 action |
| Twinned | A spell that takes one creature at the slot level and has no range of self |
| Empowered | Damage dice |

One option is allowed, or Empowered Spell with one other.
The server checks the choice (known, allowed for this spell and slot, parameters, points), spends the points with the slot (`sorcery_points`; the undo of the cast gives them back) and applies what it can:

- **Twinned Spell** adds a second target, checked on its own.
  A physical d20 fits a single target, so a twinned spell attack is rolled in the app.
  A twinned cantrip with an attack roll goes through `RollAttack` and is not offered.
- **Careful Spell** makes the chosen creatures pass their saving throw.
- **Heightened Spell** gives one target the lower of two d20s on its first save.
- **Distant Spell** doubles the range (30 ft for touch).
- **Quickened Spell** makes the casting a bonus action (the bonus action must be free and the bonus action spell limit applies).
- **Subtle, Extended and Empowered Spell** cost their points and are written on the line; the engine has no components, no duration clock and no choice of damage dice, so the table plays them.

### Bardic Inspiration in combat (bard 1)

`GiveBardicInspiration(actor, target)` is a bonus action that spends a use and puts the die on the target's combatant (`combatants.inspiration_sides`, `inspiration_from`, `inspiration_expires_round`).
The size is the bard's table die (d6 to d12); it lasts 100 rounds (the 10 minutes of the SRD in 6-second rounds).
The target is a creature other than the bard, within 60 ft on a map (the list only offers creatures the caller sees), that can hear (not `deafened`) and holds no die; the refusal is `TARGET_REFUSED`, and `GetTurnOptionsResponse.resource_targets` says why each one cannot, never anything about its type.
The die is visible on the combatant to its player and the master only; the others see the bard spend a use.
Using the die is a held-roll question; see [Held rolls](held-rolls.md).
The undo of an attack gives the die back.
