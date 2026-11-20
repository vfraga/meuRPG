# The master's Inspiration

The master grants Inspiration to a player character; the rule is [RN-43](../../product/rules.md#rn-43-the-masters-inspiration).
The code calls it favor because "inspiration" already means the Bardic Inspiration die ([Held rolls](held-rolls.md)); the app says "Inspiração".
The contract is `FavorService` (`proto/meurpg/play/v1/favor.proto`).
The code is `favor_grant.go`, `favor_read.go`, `favor_events.go`, `favor_rpc.go` and `favor_spend.go`.
`GrantFavor` and `GetFavor` are built; `PassFavor` answers `unimplemented` to a member of the campaign.
The spend is the helper `useFavor`, which the roll paths call.

## The table

`character_favor` has one row for each character that has it, keyed by the character, so it never stacks ([Data](../../data/play.md#character_favor)).
The row carries the campaign and the master that granted it.
Only a player character that is not in the reserve gets a row.
An NPC, a combatant's id, a reserved character, a character of another campaign and an id nobody has all get the same refusal.
A player character gets a grant whatever its status: a dead one is granted too.

## The grant

`GrantFavor` is the master's call, in an open session, with or without a combat.
It does not touch the combat: no combatant is read or written.

1.
   It checks the caller is the master, the key and the character id, and finds the open session.
2.
   `Service.tx` locks the session's row, so grants take turns ([Live session](live-session.md#the-hint-number)).
3.
   It looks for the idempotency key in `session_events`: the same request is answered as the first call was, another request under the key is `invalid_argument`.
4.
   It asks the characters module (`CombatRoster.SessionCharacters`) whether the id is a player character of the campaign that is not reserved.
5.
   It inserts the row with `ON CONFLICT DO NOTHING`: no row back means the character has it.
6.
   It writes the session event `favor_granted`, with the character's id and no text, under the key.
7.
   It collects the hint, which `Service.tx` sends after the commit.

A refusal writes nothing and moves no hint number.
A grant moves it once.
A retry tells the hint again, from the stored event, and moves nothing.
Of concurrent grants to the same character exactly one succeeds and the rest get `ALREADY_HAS`.

## The read

`GetFavor` is a read (`IDEMPOTENT`) for any member of the campaign, in or out of a session.
The master reads every holder of the campaign.
A player reads only the holders among their own characters, and a player with none reads an empty list.
Someone outside the campaign, and a pending member, get `not_found`.
Each holder is the character's id and name; the name comes from the characters module, since the query cannot join its table.

## The spend

The owner of a player character that has the Inspiration spends it for advantage on one of the character's own d20 rolls.
The player chooses it before the d20 is rolled, with `spend_favor` on the roll's request.
The roll is an attack, a saving throw or an ability check; the request of each roll path has the field.
Eleven paths carry it.
Four are combat rolls: `RollAttack`, `CastSpell` (a spell attack against one target), `ResolveConcentrationSave` and `RollEffectSave`.
Seven are checks: `RollSceneCheck`, `RollGroupCheck`, `StartContest`, `RespondContest`, `Hide`, `SearchForTraps` and `TryPuzzleHint`.
The death save, the counterspell check and initiative do not carry it yet.
`useFavor` (`favor_spend.go`) is the one place that decides it, inside the transaction of the roll.

| The roll before the spend | The roll with it |
| --- | --- |
| Normal, with no source | Advantage |
| Disadvantage only | Normal |
| Advantage | Refused with `ADDS_NOTHING` |
| Advantages and disadvantages that cancel to normal | Refused with `ADDS_NOTHING` |

The helper answers in this order:

1. The hold the request belongs to is read first, whatever the request says.
2. The answer to a held roll counts the Inspiration the hold remembers, and never spends or refuses.
3. A retry of a held roll made with the Inspiration counts it and spends nothing more, even if the retry no longer asks for it.
4. A retry that asks for it on a held roll made without it is refused with `AFTER_THE_ROLL`.
5. A request that does not ask for it, and has no hold, keeps its mode.
6. A roller that is not the owner (the master rolling for an absent player included) gets `NOT_THE_HOLDER`.
7. A call that rolls no d20 gets `NOTHING_TO_ROLL`.
8. A roll the Inspiration would not change gets `ADDS_NOTHING`: it already has advantage, or an advantage and a disadvantage cancel.
   The judgement is the roller's: only the sources the roller may read count (`visibleSources`, in `spendFavor` and nowhere else).
   The sources the roller may not read are marked where the roll's sources are made (a path's `Hide`, or `combat.Source.Hidden` for the notes of a contest start, a contest answer and a Hide, from the same rule the other checks use), and `spendFavor` is the one place that leaves them out of the judgement.
   A hidden advantage, or a hidden advantage and disadvantage that cancel, never cause the refusal: the spend goes, the roll is made with every source, and the hidden ones read "Outra fonte".
   So a refusal never tells a player that a source they cannot see exists, and they cannot probe for it.
   The refusal of a `roll_mode` worse than the one the spend gives (`favorAgainst`, `INVALID_ARGUMENT`) is judged the same way, on `favorUse.Seen` (the mode of the sources the roller reads alone) and not on the mode the roll is made with, so a hidden disadvantage cannot be told apart by it.
   `NOTHING_TO_ROLL` leaks nothing: it reads what the call is (no d20 at all), not any source.
9. The helper deletes the character's row, and no row back is `HAS_NONE`.

Of concurrent spends of one Inspiration exactly one wins and the rest get `HAS_NONE`.
A refusal spends nothing and moves no hint number.
The row is deleted in the transaction of the roll, so a roll that is not written leaves it.
The spend is part of the roll's event (`Favor`), never an event of its own, so it never closes the undo chain.
`favorGiveBack` puts the row back, for the undo of such a roll; it is idempotent.
The spend is valid when made; a reaction that comes later (a Protection's disadvantage) can cancel the advantage, as it cancels any advantage, and the Inspiração is still spent.
The Inspiration is a source of the roll, `SourceFavor`, shown as "Inspiração" to the master and the roller and never hidden.
A hold keeps `favor_spent`, so a retry and the answer count the Inspiration again without spending it.

A hold that is dropped before it is answered (the die ran out, the round turned, the combat began or ended, the combatant or the character left it) gives the Inspiration back: the roll never happens, and no event exists to undo.
There is one way to drop a hold, `hold_forget.go`: `forgetHoldsOf` (every hold of a combatant, and the character's holds outside any combat, which go too when its combatant leaves or the combat ends and are refunded the same way), `deleteCombatant` (the one function that forgets a combatant's holds and then deletes the combatant), `forgetHold` (the gate's, for a stale question) and `forgetOrphanHolds` (the sweep of game time, for a die that ran out).
Each deletes the rows `RETURNING` which of them spent the Inspiration with no answer, and `favorGiveBackIf` puts it back before the call returns; it does not stack on a newer grant, and a hold that was answered gives nothing.
The paths that use it: the gate when it meets a stale hold of the roller (`gateOpen`), `RemoveCombatant` and `dropCombatants`, which delete the combatant only through `deleteCombatant`, the end of a combat and the leaving of a character (`dieToCharacters`, for every combatant of a character, with a die or without), and the move of a character into a combat (`carryDieIn`).
Nothing relies on the foreign key's cascade from `combatants` and `encounters`, which would delete the row with the Inspiration.
`archtest` keeps it so, by path and not by file: only the three queries delete from `inspiration_holds`, only `hold_forget.go` calls them, only `DeleteCombatant` may delete from `combatants` (and no query deletes from `encounters`), only `hold_forget.go` calls `DeleteCombatant`, and the one function that calls it calls `forgetHoldsOf` before it.
A stale hold the gate has not met yet is forgotten before a spend that finds no row (`owedBeforeTheSpend`, `forgetStaleHoldsOf`), so the player is not told `HAS_NONE` for an Inspiration that is only waiting to be given back; with a row (a grant came in) the spend takes it and the later forget gives the refund back, so the one-row rule loses nothing. A hold that still waits stays.
The held roll is found by its roller as well as its key (`GetInspirationHoldOfRoller`: the character or the combatant), because the members of a group check send one key: a member never reads another's hold, nor counts the Inspiration it spent.

Known limit: the undo of a cast gives back the Inspiration a catch spent on its zone save through the live combatant. A character that left the combat before the undo is no longer there, so its Inspiration is not given back; the master can grant it again.
The spend is applied to the roll's truth before the roller's mode is chosen, so a typed roll carries two faces and the mode cannot be better than the truth.
A Protection disadvantage is applied after it and cancels the advantage, so the roll is normal.
A spend that comes with a worse `roll_mode` than the one it gives is refused with `invalid_argument`.
A cast that makes no single attack roll gets `NOTHING_TO_ROLL`, and so does an effect save that fails by itself.
A spell attack finds its truth in one function (`spellAttackTruth`), shared by the roll and by the hold of a Bardic die.
The attack's pinned request keeps `spend_favor`, so a retry that changes it is a different request.

## The checks

The seven checks give the helper the check's own sources before the Inspiration (`checkMode.withFavor` and `preparedCheck.withFavor` add it after).
Scene, group, trap and hint are the player-only roles' calls, for the player's own character; the master gets `NOT_THE_HOLDER`.
A contest start, a contest answer and Hide are the owner's of the combatant; the master rolling for a player's combatant gets `NOT_THE_HOLDER`.
A grapple that comes from an attack, and a contest answer left to the master (`defer_to_master`), roll no d20 and get `NOTHING_TO_ROLL` when they ask for it.
The master's `RollForPlayer` for a group check has no field to spend with, so it never spends.
A trap search in a combat spends the combatant's action and the Inspiration in the same transaction, and a refusal keeps both.
The roll's event or stored row keeps `Favor`, and the held roll keeps `favor_spent`.
"Inspiração" is a source of the check, or a note of a contest or a Hide, and only the roller and the master read it; there is no log line.
A settled retry answers as the first call did and never spends again.
The retry that flips `spend_favor` on a settled check is a different request (`invalid_argument`), except the hint try, which keeps no request hash: a retry of a written try answers the stored result, ignores `spend_favor`, spends nothing and refuses nothing.
A held retry never gets `HAS_NONE`, and the answer to a Bardic die or a Lucky question never spends.
The replay of a check a window held, when it spends and then holds a Bardic die, keeps `favor_spent` in that hold, so a retry of the key is not told `HAS_NONE` and does not spend again.
The same holds for a weapon attack, whose hold also carries the window's mark.
Each spend moves the session's hint number by the one step the roll already moves it, and a refusal moves none.

## The Perception search

The spend counts against the search's mode, so a Perception search with it rolls two d20 like any advantage.
The squares in dim light keep their own disadvantage rule, so the Inspiration does not help on them.
A disadvantage and the Inspiration leave a straight roll for the bright squares and the lower of two for the dim squares.
`ADDS_NOTHING` reads the mode of the search, not each square.
With a real die the two faces of the advantage are typed in `d20_faces`; one face is refused as for any advantage, and the Inspiration stays.
Under a spend `d20_face_2` is not read, and on any skill but Perception it is never read.

## A reaction window

A window that would hold an attack or a cast checks the spend without making it: the row stays.
A spend the replay would refuse (no Inspiration, nothing to add, a worse `roll_mode`) is refused at hold time: the call errors and nothing is held.
The replay is the real roll and spends the row.
A replay whose Inspiration is gone, or whose sources changed so that it adds nothing, does not drop the roll: it is made without the Inspiração, in the normal mode of its real sources, and nothing is spent (so nothing is given back).
The roll's event says `favor` false and `favor_lost` ("gone" or "adds_nothing").
The master and the roller's own player read a line in Portuguese: in the combat log of an attack or a cast ("<name> não tinha mais a Inspiração: a rolagem foi feita sem ela." or "A Inspiração de <name> não mudaria mais a rolagem: ela foi guardada e a rolagem foi feita sem ela."), and as a note of the roll for a check, a contest or a Hide ("Inspiração não usada: não estava mais com você" or "Inspiração guardada: não mudaria mais a rolagem").
No other player reads it, and there is no "usou a Inspiração" line.
It applies to every roll a window holds: the weapon attack, the spell attack, the contest start, the contest answer and Hide.
A concentration save and an effect save have no window, so they have no replay.
The typed faces are checked only at the replay.
A replay that the roll's checks refuse after the Inspiração was spent (the player asked an advantage with a reason over a disadvantage that came while the window waited, and `chooseMode` refuses it) is rolled back whole: the replay runs in a savepoint (`guardReplay`, `settleReplay`).
The Inspiração is back, no `favor` event or `inspiration_holds` row stands, the hold is `dropped` and the master reads the `held_dropped` line.
What stays spent is only what the windows' own answers wrote before the savepoint: the reactor's slot and reaction, as a reaction used against an action that then did not happen.
A contest start, a contest answer and Hide are held the same way by Cutting Words and the master's check window: the hold only checks the Inspiration is there, and the replay spends it.

## The log and the undo

The log entry of a roll made with the Inspiration carries `favor_text_pt`: "<name> usou a Inspiração."
The master and the roller's own player read it; another player and someone outside the campaign read the roll as any other.
A zone's save names the creature that rolled it, not the zone's owner.
The undo of an attack or a cast puts the row back in the undo's transaction, and the master's undo does not stack: a character that was granted another since keeps the one.
A concentration save and an effect save are not undoable kinds, so they have no undo, except the save of a zone at its cast: the undo of the cast gives its Inspiration back.
`keep` and `hand_to_master` on a concentration save, and a skipped effect save, ignore `spend_favor` without refusing it.
Each roll that spends moves the session's hint number once, as the roll already does, and a refusal moves none.

## Who hears and who reads

| What | The master | The holder's player | Another player |
| --- | --- | --- | --- |
| The row, in `GetFavor` | Every holder | Their own characters | None |
| The hint of a grant | Yes | Yes | No |
| The session's hint number | Steps by one | Steps by one | Steps by one, the same for whoever was picked |
| The combat log line of a roll that spent it | Yes | Yes | No |
| The combat log line of a grant, and the session summary | No line | No line | No line |

The hint is `vitals_changed` with the character's vitals, which the master and the player already read, so it carries nothing about the Inspiration; the screens read the character and `GetFavor` again.
A character whose vitals cannot be read (a dead one) tells nothing, and the number moves all the same.
`favor_granted` is no line of the combat log: it belongs to no combat, so the log never reads it.

## Reasons

`FavorBlocked` carries the reason of a `failed_precondition`.

| Reason | When |
| --- | --- |
| `ALREADY_HAS` | The character already has the Inspiration |
| `NOT_A_PLAYER_CHARACTER` | The id is not a player character of this campaign that is out of the reserve |

The spend sends `NOT_THE_HOLDER`, `HAS_NONE`, `ADDS_NOTHING`, `AFTER_THE_ROLL` and `NOTHING_TO_ROLL`.
`CANNOT_RECEIVE` and `SELF` are for the pass, which is not built.
A grant with no open session is `NO_OPEN_SESSION` (`GameSessionBlocked`).
A player calling it is `permission_denied`.

## Tests

`favor_grant_test.go` covers the grant, the read, the refusals (the combatants of a combat among them), a dead holder, the retry and the races.
`favor_spend_test.go` covers the decision table of the helper, the held retry and answer, the race of twenty spends and the adapters.
`favor_combat_test.go` covers the four combat paths: the spend, the refusals, the retry, the typed faces, the log, the undo, the held roll and the window replay.
`favor_checks_test.go`, `favor_checks_more_test.go` and `favor_checks_special_test.go` cover the seven checks: the spend, the refusals, the settled retry, the held roll, the typed faces, the number, the window and the trap search in a combat.
`favor_round2_test.go` covers the spell attack that spends once, `ADDS_NOTHING` judged on what the roller reads (an attack, a save and a check, with a hidden advantage, a hidden pair and a visible control) and the spend before a Protection that cancels the advantage.
`TestAuthorizationMatrix` has the rows of the three calls.
`internal/leaktest/favor_test.go` proves that a third player reads nothing of who has it, in the read, the log and the stream.
`internal/leaktest/favor_checks_test.go` proves that the source "Inspiração" of a spent check reaches the roller and the master and no one else.
