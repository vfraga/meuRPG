# The master's Inspiration

The master grants Inspiration to a player character; the rule is [RN-43](../../product/rules.md#rn-43-the-masters-inspiration).
The code calls it favor because "inspiration" already means the Bardic Inspiration die ([Held rolls](held-rolls.md)); the app says "Inspiração".
The contract is `FavorService` (`proto/meurpg/play/v1/favor.proto`).
The code is `favor_grant.go`, `favor_pass.go`, `favor_read.go`, `favor_events.go`, `favor_refund.go` and `favor_spend.go`.
`GrantFavor`, `GetFavor`, `PassFavor`, `AnswerFavorPass`, `CancelFavorPass` and `AnswerFavorRefund` are built (`favor_refund.go` is the refund).
The spend is the helper `useFavor`, which the roll paths call.

## The table

`character_favor` has one row for each character that has it, keyed by the character, so it never stacks ([Data](../../data/play.md#character_favor)).
The row carries the campaign and the master that granted it.
Only a player character that is not in the reserve gets a row.
An NPC, a combatant's id, a reserved character, a character of another campaign and an id nobody has all get the same refusal.
A player character gets a grant whatever its status: a dead one is granted too.

## The grant

`GrantFavor` is the master's call, in an open session, with or without a combat.
In a combat it writes the grant as the combat's last action: `favorStage` reads the encounter and the combatant, and the event is stamped with the encounter (below, "The log and the undo"). Outside a combat it touches nothing of one.

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

## The pass

A player passes the Inspiration only by asking, and the master answers ([RN-43](../../product/rules.md#rn-43-the-masters-inspiration)).
An ask is a row of `favor_passes` ([Data](../../data/play.md#favor_passes-and-favor_refund_prompts)), `pending` until it ends as `accepted`, `refused` or `canceled`.
Nothing moves while it is pending.
Each call is a `Service.tx` transaction on the session the pass was asked in, so asks and answers take turns with every other write of the session.

`PassFavor(campaign, from, to, key)` is the giver's owner's call, in an open session.
It refuses in this order, so that no refusal tells the caller what they may not read:

1. `not_found` for a caller that is not a member; `invalid_argument` for a key or an id that is not a UUID.
2. `SELF`: the receiver is the giver. It reads nothing.
3. `NO_SESSION`: the campaign has no open session (the master answers at the table). It is the campaign's own state, which every member reads, and the transaction needs the session to lock.
4. The retry of a key answers the stored pass whatever its state is now; the key of another request is `invalid_argument`.
5. `NOT_THE_HOLDER`: the giver is not a player character of the caller's (the master included), or has none. It reads only the caller's own state.
6. `NOT_A_PLAYER_CHARACTER`: the receiver is not a player character of the campaign that is out of the reserve; it is the one answer for an NPC, a reserved character, another game's character and an id nobody has.
7. `PASS_PENDING`: the giver already has a pending ask. Two asks of one giver at once leave one row (the partial unique index decides), and the other is told `PASS_PENDING`.

The receiver's state is never read at the ask: a receiver that already has the Inspiration is asked for all the same.
The master finds it in `GetFavor` (`receiver_has`) and at the accept.
The ask writes the row and the session event `favor_pass_asked` (ids only: the pass, the giver and the receiver), under the key, and tells the master and the giver's owner.

`AnswerFavorPass(campaign, pass, accept, key)` is the master's call.
`not_found` is the pass of another campaign or one that does not exist; a player gets `permission_denied`.
A retry of its key answers the stored outcome (the event of the key finds it) and moves no number; a key used for the other answer, another pass or a cancel is `invalid_argument`.
The pass is read again under the session's lock: one that is no longer pending is `PASS_CLOSED`, so of an accept and a cancel, or of two accepts, one wins and the others read `PASS_CLOSED`.

- **Accept** is one transaction.
  It closes the pass `accepted`, deletes the giver's row (`SpendCharacterFavor`), inserts the receiver's with `ON CONFLICT DO NOTHING` and writes `favor_passed`.
  If the receiver already has one (or is no player character of the table any more), the insert gives no row and the call answers `CANNOT_RECEIVE`: the transaction is rolled back whole, so nothing changed, no event was written, no number moved and no hint was sent, and the pass stays pending.
  Only the master is told: `CANNOT_RECEIVE` goes to nobody else.
  A giver that has none by now gets `NOT_THE_HOLDER`, again with nothing changed; that is a defence only, since nothing takes the row while the pass is pending (the master's undo of a grant cancels the ask in the same transaction, `takeFavorFrom`). A giver or a receiver sent back to the reserve while the ask waits is the same: `NOT_THE_HOLDER` for the giver, `CANNOT_RECEIVE` for the receiver.
  An accept racing a grant to the receiver ends one way or the other: the grant first (the accept is `CANNOT_RECEIVE`) or the accept first (the grant is `ALREADY_HAS`); the receiver never has two rows.
- **Refuse** closes it `refused` and writes `favor_pass_refused`; nothing moves.

`CancelFavorPass(campaign, pass, key)` is the giver's owner's call: it closes the pass `canceled` and writes `favor_pass_canceled`.
The master, the receiver and every other player get `not_found` for a pass that is not theirs to cancel, whatever its state, so a closed pass of someone else is never told apart from one that does not exist.
The owner of a closed pass gets `PASS_CLOSED`.
The owner is the current one: a character that changed hands while the ask waited is canceled by its new owner (`ownsGiver`), and the old owner gets `not_found`, so neither reads the other's ask.
A closing key that another session of the campaign already used is refused as reused (`closeKeyError`), never as an internal error.

**The spend.** While the giver's pass is pending a spend is refused `PASS_PENDING` (`favorInPass`).
It comes in `spendFavor` after the refusals that need no look at the character (`NOT_THE_HOLDER`, `NOTHING_TO_ROLL`) and before the judgement of the roll (`ADDS_NOTHING`), and in the check of a spend a window will make: the box that spends it is closed whatever the roll is, which the player knows because the ask is theirs.
The replay of a roll a window held, which finds the pass pending, behaves as the replay does for the other Inspirations it can no longer spend: the roll is made without it and says so (`favorLostPending`, "A Inspiração de <name> está num pedido de passagem que o mestre não respondeu: a rolagem foi feita sem ela." for the master and the roller's player; "Inspiração não usada: está num pedido de passagem sem resposta" as the note of a check).
A spend and an accept at once: the spend is `PASS_PENDING` before the accept commits and `HAS_NONE` after, never a spend, and the Inspiration moves once.

**The end of a session** closes the passes still pending in it `canceled` (`CloseFavorPassesOfSession`, in the transaction of `endSessionIn`).
Each writes `favor_pass_canceled` (the master's hand, no key) and its giver's vitals hint reaches the master and the giver's owner with the end.
The Inspiration stays with the giver.

The hints are the grant's: `vitals_changed` for the giver (and the receiver, at an accept), which reaches the master and the character's own player; the screens read `GetFavor` again.
Each write moves the session's number once, and a refusal or a replay moves none.
A third player hears nothing.

## The read

`GetFavor` is a read (`IDEMPOTENT`) for any member of the campaign, in or out of a session.
The master reads every holder of the campaign.
A player reads only the holders among their own characters, and a player with none reads an empty list.
Someone outside the campaign, and a pending member, get `not_found`.
Each holder is the character's id and name; the name comes from the characters module, since the query cannot join its table.
`passes` holds the pending asks (`ListPendingFavorPasses`): the master reads every one, with `receiver_has` true when the receiver has the Inspiration now (his Aceitar is dashed with the reason); a player reads only the asks of their own characters, and `receiver_has` is never sent to a player.
An answered or canceled pass is not listed.

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
A hold keeps `favor_spent`, so a retry and the answer count the Inspiration again without spending it. The key the ask looks the hold up by (`favorAsk.Key`) is always the parsed one (`parseKey`, which writes a UUID in lower case), the key the hold is kept under: a request that writes its key in upper case is the same request, so a path that took the raw string would find no hold and spend again.

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
When the `roll_mode` the replay asks is worse than the mode the Inspiração gives (judged on the sources the roller reads, as at hold time: `favorAgainstBeforeSpend`), the replay refuses it with `invalid_argument` before it spends: the refusal drops the held roll, and the Inspiração stays with the character, never taken.
A replay whose Inspiration is gone, or whose sources changed so that it adds nothing, does not drop the roll: it is made without the Inspiração, in the normal mode of its real sources, and nothing is spent (so nothing is given back).
The roll's event says `favor` false and `favor_lost` ("gone" or "adds_nothing").
The master and the roller's own player read a line in Portuguese: in the combat log of an attack or a cast ("<name> não tinha mais a Inspiração: a rolagem foi feita sem ela." or "A Inspiração de <name> não mudaria mais a rolagem: ela foi guardada e a rolagem foi feita sem ela."), and as a note of the roll for a check, a contest or a Hide ("Inspiração não usada: não estava mais com você" or "Inspiração guardada: não mudaria mais a rolagem").
No other player reads it, and there is no "usou a Inspiração" line.
It applies to every roll a window holds: the weapon attack, the spell attack, the contest start, the contest answer and Hide.
A concentration save and an effect save have no window, so they have no replay.
The typed faces are checked only at the replay.
A contest start, a contest answer and Hide are held the same way by Cutting Words and the master's check window: the hold only checks the Inspiration is there, and the replay spends it.

## The log and the undo

The log entry of a roll made with the Inspiration carries `favor_text_pt`: "<name> usou a Inspiração."
The master and the roller's own player read it; another player and someone outside the campaign read the roll as any other.
A zone's save names the creature that rolled it, not the zone's owner.
The undo of an attack or a cast puts the row back in the undo's transaction, and the master's undo does not stack: a character that was granted another since keeps the one.
A concentration save and an effect save are not undoable kinds, so they have no undo, except the save of a zone at its cast: the undo of the cast gives its Inspiration back and, as the undo of a roll does, closes the refund prompt that spend opened as kept (`favorBackOfRoll`, in the roll's undo, in `takeBackOlder` and in `zoneAnswerBack`, each from the roll's `favor_prompt` stamp), so a later "Devolver" cannot mint a second one.
`keep` and `hand_to_master` on a concentration save, and a skipped effect save, ignore `spend_favor` without refusing it.
Each roll that spends moves the session's hint number once, as the roll already does, and a refusal moves none.
**In a combat.** A favor event written while the session's combat is running (not in setup), and whose characters are all player combatants of it, carries the open encounter (the `session_events.encounter_id` column) and, in its payload, the round, the combatants (`combatant_id`, `target_id`), `secret` (a defensive stamp: no player's combatant can be hidden, so it is never set by the API) and the audience (`fogged` and `seen_by`: the owners of the characters named), with the names `actionEvent` uses, so the log reads them (`favorInCombat`, `favorStage`). Outside a combat, or with a character that is not a combatant, the event carries none of it: no line and no undo.
Three log kinds carry the line, in `CombatLogFavor.text_pt`, built for each reader (`combat_log_favor.go`):

| Kind | Event | Master | Giver or roller | Receiver |
| --- | --- | --- | --- | --- |
| `FAVOR_PASSED` (116) | `favor_passed`, and `favor_granted` (master only) | "Brisa passou a Inspiração para Tor." / "O mestre deu a Inspiração a Tor." | "O mestre aceitou: a sua Inspiração foi para Tor." | "Brisa passou a Inspiração para você." |
| `FAVOR_PASS_ANSWERED` (118) | `favor_pass_refused`, `favor_pass_canceled`, and the undo of an accepted pass | names | "O mestre recusou: a Inspiração continua com você." / "Você desistiu de passar a Inspiração." | after an undo only: "O mestre desfez a passagem: a Inspiração não está mais com você." (no actor in the entry) |
| `FAVOR_REFUNDED` (117) | `favor_refunded` | "O mestre devolveu a Inspiração de Brisa." | "O mestre devolveu a sua Inspiração." | none |

No other player reads a line, nor hears of it or of its undo (`seen_by`), and an entry with a hidden combatant is the master's alone, even after the master shows it (`secret`). No hidden player combatant can exist (`SetCombatantHidden` refuses a player's combatant, `TestAPlayersCombatantCannotBeHidden`), so the `secret` stamp is a defence for a row the API cannot make, not a feature; its tests set the flag in the table.
`chainFavor` keeps the ask, the prompt, the kept prompt and every favor event of another combat or of none out of the undo chain; a grant, an accepted pass, a refusal and a cancel of this combat are its actions (`favorUndoKinds`).
The master's undo (`takeBackFavor`) of a grant takes the row back; of an accepted pass it takes it from the receiver and gives it to the giver in the undo's own transaction, after the same check as an accept (`mayHold`): a giver that is no longer a player character out of the reserve is refused `CANNOT_RECEIVE` and the whole undo rolls back; of a refusal or a cancel it only closes the line.
It is refused `UNDO_TARGET_CHANGED` (naming the combatant) when the character spent it since, and `ALREADY_HAS` when the giver has another by then; nothing changes, since the transaction rolls back whole.
The undo tells the master and the owners of the event, and no other player.
The grant's line is the master's alone and reuses `FAVOR_PASSED` ("O mestre deu a Inspiração a Toren."; `fillFavor` says nothing to a player, and the event's `seen_by` is empty, so the owner of the character granted hears nothing of it either), so the screen offers its undo from the line the master reads (`undoable` on the entry), not from `undoable_event_id` alone. The stamp is the same defence as for the rest.
A giver's own cancel and a refusal written in a combat are the master's last action too (`favorUndoKinds`): his undo of one only takes its line away. A refusal or a cancel moved nothing, so its `seen_by` is the giver's owner alone (`stageOfPass(moved=false)`); only an accepted pass (and its undo) is told to the receiver's owner too.
The undo of a grant or an accepted pass also cancels the pending ask of the character that lost the row (`takeFavorFrom`): it closes the `favor_passes` row as `canceled` with no key and writes a `favor_pass_canceled` with no combat (so no line and no action to undo), in the undo's transaction. It sends no hint of its own: the vitals hint the undo already sends to the master and the owner makes their screens read `GetFavor` again.

## The refund prompt

`spendFavor` judges the sources the roller reads. A spend that adds something over them but nothing over every source (a hidden advantage already gave it) resolves the roll at once and returns `favorUse.Refund`; `useFavor` then opens a row of `favor_refund_prompts` (the roll's name by its held kind, the hidden sources), writes `favor_refund_prompted` (no key, the master only) and tells the roller's vitals to the master alone. The roll's own write moves the hint number: the prompt moves none by itself.
`GetFavor.refund_prompts` is the master's alone, with the hidden sources as the master reads them.
`AnswerFavorRefund` is the master's call, idempotent by its key (a retry answers the same prompt and moves nothing). A prompt already answered or ended is `PASS_CLOSED` (the closest existing reason). An answer's key that another session of the campaign already used is refused as reused (`invalid_argument`): the retry check reads the open session's events only, and the unique key on `answer_key` catches it. The spend that opens the prompt knows its id: `useFavor` takes it from `openRefundPrompt` into `favorUse.Prompt`, and it is empty for a spend that opened none. The roll's event is stamped with it (`favor_prompt`, `actionEvent.FavorPrompt`), and so is its held roll (`inspiration_holds.favor_prompt_id`, `heldRoll.FavorPrompt`). No key is used for this, neither the event's (a reaction window's replay writes none, and the answer to a die's or a Lucky question writes its own) nor the request's (two spends of one character can share it: a held roll that went stale is forgotten and the client sends its key again, which spends afresh, and a lookup by key would read the first prompt's answer for the second roll and leave the second prompt open). So the undo of the roll (`takeBackOf`, `takeBackOlder` and `zoneAnswerBack`, from the stamp) and the forgetting of its held roll (`favorGiveBackIf`, from the hold's prompt) close that prompt `kept` through `favorBackOfRoll`, since the Inspiration went back with the roll (`KeepFavorRefundPrompt` closes it by its id, and only while it is open), and the prompts of the character's other rolls, and of the other members of a group check, are never touched. A roll with no prompt has nothing to read or to close, and gives the Inspiration back unconditionally. If that roll's prompt was already answered `returned`, the give-back is skipped, so a "Devolver" followed by a dropped hold or an undo gives one Inspiração back, not two; those closings write no event of their own (the roll's undo is the record). "Give back" goes through the one refund path, `refundSpentFavor` in `hold_forget.go`, which first checks that the roller may hold one (`mayHold`: a player character out of the reserve; otherwise `CANNOT_RECEIVE`, the answer rolls back and the prompt stays open for a "keep"), and refuses a roller that has the Inspiration again with `ALREADY_HAS` (the spend was already refunded, or the master granted another; the answer rolls back and the prompt stays open for a "keep"), closes it `returned`, writes `favor_refunded` (the master and the owner; a line of the log in a combat) and tells the roller's vitals. "Keep" closes it `kept`, moves the hint number by one, tells the master's screens (the roller's vitals hint, the master's alone) so that his other tabs close the prompt, and writes `favor_refund_kept`, the master's alone. A retry of either answer tells again what the first told and moves no number. The session's end closes the open prompts `kept`. The player reads only "O mestre devolveu a sua Inspiração.", never why, and nothing when it is kept. `TestFavorGiveBackIsCalledOnlyFromTheRefundPaths` keeps every give-back in `favor_spend.go`, `hold_forget.go` and the zone's undo, and `TestInsertCharacterFavorIsCalledOnlyFromTheFavorFiles` keeps the raw insert of the row in `favor_grant.go`, `favor_pass.go`, `combat_undo_favor.go`, `favor_spend.go` and `hold_forget.go`.

## Who hears and who reads

| What | The master | The holder's player | Another player |
| --- | --- | --- | --- |
| The row, in `GetFavor` | Every holder | Their own characters | None |
| The hint of a grant | Yes | Yes | No |
| The ask, in `GetFavor` | Every pending ask, with `receiver_has` | Their own asks, never `receiver_has` | None |
| The hint of an ask, a refusal and a cancel (the giver's vitals) | Yes | The giver's owner | No |
| The hint of an accept (the giver's and the receiver's vitals) | Yes | Both owners | No |
| `CANNOT_RECEIVE` | Yes | No | No |
| The session's hint number | Steps by one | Steps by one | Steps by one, the same for whoever was picked |
| The combat log line of a roll that spent it | Yes | Yes | No |
| The combat log line of a grant in a combat | Yes ("O mestre deu a Inspiração a …") | No | No |
| The session summary | No line | No line | No line |
| The log line of a pass answered or a refund, in a combat | Yes, with names | The giver or roller: their part; the receiver: that they have it, only for an accepted pass | None |
| The refund prompt, in `GetFavor` | Yes, with the hidden sources | No | No |

The hint is `vitals_changed` with the character's vitals, which the master and the player already read, so it carries nothing about the Inspiration; the screens read the character and `GetFavor` again.
A character whose vitals cannot be read (a dead one) tells nothing, and the number moves all the same.
`favor_granted` is a line of the combat log only for the master, and only in a combat; it is there for his undo.

## Reasons

`FavorBlocked` carries the reason of a `failed_precondition`.

| Reason | When |
| --- | --- |
| `ALREADY_HAS` | The character already has the Inspiration |
| `NOT_A_PLAYER_CHARACTER` | The id is not a player character of this campaign that is out of the reserve |

The spend sends `NOT_THE_HOLDER`, `HAS_NONE`, `ADDS_NOTHING`, `AFTER_THE_ROLL` and `NOTHING_TO_ROLL`.
The pass sends `SELF`, `NO_SESSION`, `PASS_PENDING`, `PASS_CLOSED` and `CANNOT_RECEIVE` (the master's alone), besides `NOT_THE_HOLDER` and `NOT_A_PLAYER_CHARACTER`.
`CANNOT_RECEIVE` is always the master's, and means the character that would take the Inspiration cannot hold it now: the receiver of an accept that has one or is no player character out of the reserve, the giver of an accepted pass the master undoes, and the roller of a "Devolver" that is in the reserve or no player character. The two answers send `PASS_CLOSED` for a question or an ask already answered; the accept also `NOT_THE_HOLDER` (a giver in the reserve or with none), and "Devolver" also `ALREADY_HAS`. The web says one sentence for the three ("Essa personagem não pode ficar com a Inspiração agora.").
A give-back the app makes by itself (`favorBackOfRoll`: the undo of a roll, a held roll dropped) checks nothing of the character: it restores the row to the character that spent it, in the reserve too (`TestUndoOfARollGivesBackToACharacterInTheReserve`, `TestADroppedHoldGivesBackToACharacterInTheReserve`). A giver in the reserve has no owner, so its ask is closed by the master's refusal or the session's end, never by a cancel.
`PASS_PENDING` is also the spend's.
A grant with no open session is `NO_OPEN_SESSION` (`GameSessionBlocked`); an ask with none is `NO_SESSION` (`FavorBlocked`).
A player calling it is `permission_denied`.

## Tests

`favor_grant_test.go` covers the grant, the read, the refusals (the combatants of a combat among them), a dead holder, the retry and the races.
`favor_spend_test.go` covers the decision table of the helper, the held retry and answer, the race of twenty spends and the adapters.
`favor_combat_test.go` covers the four combat paths: the spend, the refusals, the retry, the typed faces, the log, the undo, the held roll and the window replay.
`favor_checks_test.go`, `favor_checks_more_test.go` and `favor_checks_special_test.go` cover the seven checks: the spend, the refusals, the settled retry, the held roll, the typed faces, the number, the window and the trap search in a combat.
`favor_round2_test.go` covers the spell attack that spends once, `ADDS_NOTHING` judged on what the roller reads (an attack, a save and a check, with a hidden advantage, a hidden pair and a visible control) and the spend before a Protection that cancels the advantage.
`favor_pass_test.go` covers the pass: the ask and its refusals in order, the retries, the answers, `CANNOT_RECEIVE`, the cancel, the read, the spend refused while pending, the end of a session, the hints, and the races (two asks, accept and cancel, two accepts, accept and a grant to the receiver, accept and a spend).
`favor_pass_log_test.go` and `favor_refund_test.go` cover the combat's lines, the hints of each reader (a third player hears nothing), the undo of a pass and of a grant with the ask they cancel, and the refund prompt: the answers, `ALREADY_HAS`, the prompt closed by the undo or the forgetting of its roll, and the hint number of a keep.
`favor_race_test.go` (the spend runs in `Service.tx`, which takes the session's lock, as every call does) covers a spend racing the master's answer of a refund prompt and an accept racing the undo of the grant; `favor_refund_test.go` has the undo of a cast whose zone's save opened a prompt, `favor_window_undo_test.go` the undo and the forgetting of an attack a reaction window held whose replay waited for a die's or a Lucky question (the roll is written under the answer's key) and of a zone's save whose event has no key, and `favor_pass_test.go` the accept with a giver or a receiver in the reserve (at most one row, one winner); the one-row rule is the primary key of `character_favor`, which holds the same on CockroachDB. The lock orders the transactions but a snapshot can predate it: a spend that waited for the answer can fail with `40001` when it deletes the row the answer returned, and `Service.tx` runs it again, which reads the row gone (`HAS_NONE`); the test spends through the same retry and asserts the end state.
`TestAuthorizationMatrix` has the rows of the calls.
`internal/leaktest/favor_test.go` proves that a third player reads nothing of who has it, in the read, the log and the stream.
`internal/leaktest/favor_checks_test.go` proves that the source "Inspiração" of a spent check reaches the roller and the master and no one else.
`internal/leaktest/favor_pass_test.go` proves, for the grant, the ask, the refusal, the cancel, the accept, the refund prompt and its answers and the undo of a grant, of a pass and of a refusal or a cancel, that each line, read and hint reaches only its readers, and that a third player reads nothing of the Inspiration in any read and hears no hint of it. The same file proves the end of a session (the pass it cancels and the prompt it keeps reach the master and the giver's owner alone), the text of the prompt's hidden sources (the master's `refund_prompts` and no other field or reader) and a third player's writes (`PassFavor` of a character that is not hers `FAILED_PRECONDITION` `NOT_THE_HOLDER`, `CancelFavorPass` of another's ask `NOT_FOUND`, `AnswerFavorPass` `PERMISSION_DENIED`).
