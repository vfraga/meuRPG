# Contests and special actions

`ContestService` (`contests.proto`, messages in `contest_types.proto`) is the contract of grapple, shove, escape, dragging, Hide, Help, the group check and surprise (W7-X, RN-34).
`play.Service` implements it next to `CombatService`.
The pure rules are `rules/combat/contest.go` (comparison of totals, the size limit, the escape DC, passive Perception, the half-of-the-group count, the dragging cost).
The service is `play/combat_contests*.go` (start, answer, shove choice, close, release, views), `combat_drag.go`, `combat_hide.go`, `combat_help.go`, `combat_surprise.go` and `combat_groupcheck.go`; the queries are in `queries_contests.sql` (tables in [Data](../../data/README.md)).

| Call | Who may | What it does |
| --- | --- | --- |
| `GetContestState` | Any member; the player reads what concerns their characters | The contests the caller is in, the hide attempts, who is hidden (for the master), the Helps and grapples, with the surprise |
| `StartContest` | The player on their own character, on their turn; the master on any | Grapple, shove or escape: spends the attack (or the action of an escape) and rolls the initiator's check in one transaction. An escape against a fixed DC, and a grapple that came from an attack, settle at once |
| `RespondContest` | The defender's player; the master, for an NPC or to roll for a player | The defender's skill and roll (or "deixar para o mestre"); compares the totals; a tie changes nothing |
| `ResolveShove` | The winner's controller; the master | Knock prone or push 1.5 m (`stays` when there is no free square) |
| `CloseContest`, `ReleaseGrapple` | The master | Closes a waiting contest; lets a creature go |
| `Hide`, `ResolveHide` | The player on their own character, on their turn; the master decides | The Stealth check, then the comparison with each creature's passive Perception (a tie keeps the hider noticed); the master may refuse or say who "vê claramente" |
| `RevealHider` | The master | Ends a combatant's hiding for every creature (`endHiding`, the function an attack uses), writing a `hide_resolved` event with the line `hide_revealed` and the hiding it ended (`HidBefore`); the line is the master's and the hider's player's. `UndoLastAction` gives the hiding back (`hide_resolved` is undoable only for this line) |
| `Help`, `ClearHelp` | The player on their own character; the master clears | The two forms of Help (check, attack) |
| `SetSurprised`, `GetSurpriseSuggestion` | The master | Marks who is surprised (before the combat begins); the suggestion compares each hider's Stealth with each creature's passive Perception |
| `RequestGroupCheck`, `RollGroupCheck`, `RollForPlayer`, `CloseGroupCheck`, `GetGroupCheck` | The master asks, rolls for a player and closes; each player rolls their own | A check of the party; the group passes when at least half pass |

## How contests fit into combat

- **A start, an answer or a Hide held by a reaction window** (Cutting Words, or the master's check of "Sempre") is run again when the window is answered. If the replay then holds the d20 for a Bardic Inspiration die, the player's answer runs the request under the answer's own key, because the request's key belongs to the event that held it and a run under it would count as a retry and write nothing (`held-rolls.md`, "With a reaction window").
- **A contest waits in the reaction window** of kind `CONTEST` (`combat_contests_window.go`): `settleReactions` calls `syncContests`, which opens a window for each waiting contest (the reactor is the defender or the shover when a player, the master otherwise) and replaces it when the reactor changes (a roll left to the master); `triggerGone` closes it when the contest no longer waits.
  The player of an NPC's contest reads "Esperando o mestre", of a player's "Esperando <nome>", with no timeout.
  `Hide` and `Help` go through `mustActNow`, and so through the reaction gate (see [Reactions](reactions.md)).
- **Advantage** comes from `combat_advantage.go`: `modeFacts` carries the hiding and the Helps; a hidden attacker is `AttackerUnseen`; the Help is the source `help`.
  The ability checks of the contests and the group check read the advantage rules' `SaveMode` (Poisoned, Frightened, exhaustion, an effect's advantage, the Rage on Strength, armor that gives disadvantage on Stealth, armor or a shield without proficiency on Strength and Dexterity) and the Helps with the check: `checkModeOf` serves the scene checks, the trap search, the puzzle hints and the group check, and `spendCheckHelps` uses the Help up on the roll.
  Grappled, Prone and Restrained are the conditions the advantage rules already know.
- **After an attack roll**, `afterAttack` uses the Help and ends the hiding for every creature (`hide_ended`; the attack's undo gives them back).
  A cast with a verbal component ends the hiding too.
  A grapple or a shove starts with `endHiding` and, against the other side, notes the attack for the Rage.
- **The stand-in attack key.** A grapple or a shove writes `special:contest` (`combat.ContestAttackKey`): Flurry of Blows sees the Attack action taken, the Martial Arts strike does not.
- **Dragging.** `GetMoveOptions` halves the movement (`dragging_halved`, the path costs twice) and `MoveCombatant` carries the grappled creature to the penultimate square of the path (`NO_ROOM_TO_DRAG` when a creature the caller sees is on that square; for a player, a creature they do not see there gets the walk's generic `MOVE_BLOCKED` refusal, so the drag does not reveal it, RN-10).
  The dragged creature is not a reactor against its grappler (`opportunityReactors` skips it), while the other enemies the pair leaves still get their opportunity attacks.
  `pruneHolds` ends a grapple whose grappler is incapacitated or out of the fight, or whose target left the reach.
- **Surprise.** A surprised creature's turn options are all disabled with `SURPRISED`, and an opportunity attack skips it. A character whose sheet has the `never_surprised` flag is never surprised: `surprisedIDs` and `surprisedNow` ignore a mark made before the flag came, `SetSurprised` refuses to mark it (`FAILED_PRECONDITION`) and `GetSurpriseSuggestion` does not suggest it. A character with no readable sheet is not immune.
- **Privacy.** The escape DC, who noticed a hider, an NPC's roll and another player's total never reach a player: the views build the player's copy apart, and the leak tests cover each read and write.
  Contest events are not undoable and close the undo chain.

## Asking for a roll: the group check

The group check is also how the master asks one, some or all living player characters for a skill check, an ability check or a saving throw (`skill_key` `skill:x`, `ability:x` or `save:x`) outside a combat.

- `character_ids` (empty: the whole party) are checked against `CombatParty` (living player characters of the campaign, no NPC, no repeat) and become `group_check_members` rows.
- `group` (optional) becomes `group_checks.is_group`: unset it is true when no ids are named and false when some are; an explicit true needs two characters or more.
- The roll path is the check's own (`rollMember`): `SceneOptions` gives the check or save bonus from the derived sheet, `checkModeOf` the advantage (with `check` false for a save), then the Bardic Inspiration and Lucky holds (see [Held rolls](held-rolls.md)), the physical dice and the Helps.
- `CloseGroupCheck` works out the half verdict only when `is_group`.
  The view sets `group`, `save` and (master only) `asked_count`, and leaves `passed_count`, `needed` and the verdict out for a request judged alone.
- A player who was not asked gets no view (`GetGroupCheck` answers none) and cannot roll (`NOT_IN_GROUP_CHECK`).
  The `group_check_changed` hint stays content-free for everyone (RN-10, RN-20).
- The master's `RollForPlayer` waits for no one.

## The bonuses of a check

Both sides of a contest, and a Hide, add the flat bonuses of their lasting effects that apply to ability checks (`roll_bonus` of checks, `check_bonus` of the skill): `prepareCheck` and the answer options go through `withCheckBonus`, which reads them with `checkBonusesOf`, the reader the other checks use.
A bonus to saves or to attacks never enters a contest.
Each bonus is a note of the roll (`effect_bonus`, "Marca de Nanquim +3"; it says nothing of advantage, `notesMode` skips it); an effect the master hides reads "Outra fonte +3" to the player and by name to the master alone (RN-10), decided when the roll is read (`rollNote.proto(master)`), not when it is kept.

## The floor of a check

The d20 of a group check (`memberRollOf`), of either side of a contest and of a Hide counts the floor of its kind as the scene check does.
The floor is the higher of Reliable Talent's 10 and the table's [`roll_minimum`](../rules-table-content.md#roll-minimum) for checks; a tie keeps Reliable Talent.
The roster says per check whether the character has Reliable Talent (`SceneOption.ReliableTalent`, read by `numbersOf` into `checkNumbers.Reliable`).
`rollCheck` and the held-roll gate (`rollSpec.Floor`) count a face below the floor as the floor, and the stored roll keeps the number it counted as in `counted_as`.
`CheckRoll.treated_as` and `treated_as_source` (the content key of the feature that gave the floor) are set only when the number changed; `total` already uses it.
A creature has none.
