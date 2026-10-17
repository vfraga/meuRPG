# Post-refactor review, area play-b

Branch `review/finish-play-b` (a review file only; never merged). Read at origin/main 33927224 (train 12) plus the pushed branches; train 13 (#455) was not merged when this was written, so findings about functions that train 13 splits are marked **train 13**. Old = 28dd0d5, the base of the day. Every finding names where it was read; nothing was run (no database, no tests).

## Counts

| category | high | medium | low |
|---|---|---|---|
| behaviour | 2 | 2 | 2 |
| docs | 0 | 1 | 4 |
| duplication | 0 | 11 | 5 |
| readability | 0 | 8 | 12 |
| tests | 0 | 3 | 3 |

Total: 53.

## Findings

### play-b-1 · behaviour · high
- Scope: the code is in `internal/maps`, outside play-b; found while reading the trap search. Reported here so it is not lost; the fix belongs to the maps reviewer.
- Where: `backend/internal/maps/traps_play.go:447` and `:458-459` (SearchTraps), surfaced by `backend/internal/play/traps_search.go:418-420`. Docs: `docs/architecture/play/traps.md` row "Search rules (SRD)".
- What: RN-10 oracle for the existence of a trap. `SearchTraps` returns early at :447 when the map has no trap point (`len(ts.points) == 0`), otherwise it refuses a one-die Perception search with `SEARCH_NEEDS_TWO_DICE` whenever a square within 3 m is lightly obscured (:458). `ts.points` is every point of kind trap on the map: hidden, unknown to the character, far away or disarmed. A player standing in dim light who sends a one-die Perception search is refused on a map with any trap point and accepted on a map with none. A player learns that a hidden trap exists on the map before finding it. Nothing in the answer depends on the searcher having found anything. The docs row describes the rule as only "any square within 3 m ... is poorly visible", so docs and code differ.
- Fix: do the light test before the empty-map return so it no longer depends on the traps: compute `eyes`/`viewer` first and test `skill == "perception" && len(totals) < 2 && nearbyObscured(viewer)`, then return `ErrSearchNeedsTwoDice` even when the map has zero trap points. That needs `loadTrapScene` to run for maps without traps (grid and fog only). Add the case to leaktest: same request on a fog map with a trap and a fog map without one must give the same status. State the rule in traps.md.
- Changes behaviour: yes (a one-die Perception search on a trap-less map in dim light now asks for the second die, like every other map).
- Guarded by a test: no. `TestMR035_APerceptionSearchInDimLightHasDisadvantage` (traps_round2_test.go:400) only covers a map with a trap. leaktest `searchRequest` (trapskills_test.go:36) always sends `D20Face_2`, so the refusal is never reached there.

### play-b-2 · behaviour · high
- Where: `lasting_effects_clock.go:191-250` (`effectDamage`), `backend/migrations/00236_add_combat_effects.sql:150-152` vs `backend/migrations/00110_add_pending_damages_trap_point.sql:21`; pinned by `lasting_effects_core_frozen_test.go:164-189` and `testdata/lasting_core/core_web_fire_blocks_turn.golden`.
- What: pre-existing bug, now frozen into a golden. The master can add "Teia em chamas" (`effect:web-fire`, `kind: master`, so it is in the catalog) and `effectDamage` writes its `pending_damages` row through `InsertEffectPendingDamage` with neither attacker nor trap point. Migration 00236 adds the column `effect_source_key` ("has no attacker and no trap") but never relaxes `CHECK pending_damages_source_valid (attacker_id IS NOT NULL OR trap_point_id IS NOT NULL)`. Result: the start of the burning creature's turn fails with `internal`, EndTurn rolls back, and the combat cannot pass that creature until the master ends the effect. The golden says `the turn that passes to the goblin -> internal: internal error`.
- Fix: new migration: drop and re-add `pending_damages_source_valid` with `OR effect_source_key IS NOT NULL`; then regenerate the golden (it must change, which is the point). Until then the catalog entry is a trap for the master.
- Changes behaviour: yes (fix)
- Guarded by a test: yes, but it guards the bug (`TestLastingCoreAnEffectWithItsOwnDamageStopsTheTurnFromPassing`).

### play-b-3 · readability · medium
- Status: **train 13** (#455) holds the `refactor/play-puzzles-*` and `refactor/play-contests-*` branches that split these; on main 33927224 they are still in the baseline. Re-check after it merges and drop what it fixed.
- Where: `puzzles_rules.go:50` (`checkOnWrong`, cognitive 31, cyclomatic 24), `combat_contests_window.go:49` (`syncContests`, 26/19), `puzzles.go:453` (`checkOnSolve`, 18/16), `puzzles.go:330` (`decodePuzzle`, 18), `puzzles_rules.go:131` (`checkParts`, 16), `combat_hide_reveal.go:24` (`RevealHider`, 18). All six are in `.github/go-complexity-baseline.txt` (lines 12-20) and none of these files was touched since 28dd0d5 (`git diff --stat` empty).
- What: the finish line is not met in this area. These are the only over-limit functions in my files, and they are exactly the ones the trains skipped. `RevealHider` is the only RPC left in `combat_hide*.go`/`combat_help.go`/`combat_contests*.go` that still keeps the whole change inside the `s.write` closure, while all its neighbours (`Hide`, `ResolveHide`, `Help`, `ClearHelp`, `CloseContest`) got a named change function.
- Fix: `checkOnWrong` -> one tiny validator per field (`checkAttempts(kind, n)`, `checkMaxMoves`, `checkTimeLimit`, `checkTrapTarget(ctx, tx, campaignID, kind, t)`) returning `(*PuzzleOnWrong, error)` pieces, plus the "all zero means nil" test as a helper. `syncContests` -> `waitingContests(rows)` (see play-b-29), `contestWindowsByID(open)` and `reconcileContestWindow(ctx, c, row, byID, have)`. `RevealHider` -> `revealChange(ctx, c, combID) (actionEvent, error)` like `decideHide`. `decodePuzzle` -> `decodeParts(row.Parts)` and `decodeOnWrong(row.OnWrong)`. `checkOnSolve` -> per-action `targetCheck(action, on)` returning the `func() error`, so the switch leaves the function.
- Changes behaviour: no
- Guarded by a test: yes (`puzzles_test.go`/`puzzles_more_test.go` create/update refusals, `combat_hide_reveal_test.go` for RevealHider, contests tests for `syncContests`)

### play-b-4 · duplication · medium
- Where: `combat_reaction_view_more.go:559` (`rollKindOf`) and `combat_reaction_answers.go:232` (`cuttingRollKind`) have identical bodies (damage / test / default attack). The inline switch in `combat_reaction_view.go:342-349` (`trigger()`) is a third copy of the same mapping without the default.
- What: two sessions (train 9 for the view, train 9/10 for the answer) split neighbouring Cutting Words code and each wrote the mapping.
- Fix: keep `rollKindOf` (in `combat_reaction_events.go` or next to `windowTrigger`), delete `cuttingRollKind`, call it from `answerCuttingWords` (`combat_reaction_answers.go:211`). In `trigger()` leave the switch (it deliberately yields UNSPECIFIED for an empty roll) but add one line saying so, or give `rollKindOf` a second return `known bool`.
- Changes behaviour: no
- Guarded by a test: yes (`TestCuttingWordsTakesTheDieOffAnAttackAgainstAnAlly`, `TestCuttingWordsOnADamageRollCutsTheHeldDamage`)

### play-b-5 · duplication · medium
- Where: `combat_opportunity_view.go:37` (`combatantIn`, new in train 9), `combat_contests_core.go:563` (`combatantByID`), `combat_states.go:511` (`findByID`): three identical `slices.IndexFunc` lookups by id. `combat_contests_core.go:572` (`combatantsByID`, map builder) exists but is used once (`combat_contests.go:1298`); the same loop is written inline at `combat_contests_window.go:80`, `combat_opportunity.go:222`, `combat_reaction_events.go:128`, `combat_reaction_view.go:69-72`, `combat_reaction_window.go:454`, `combat_rules.go:110`, `combat_spells_view.go:172`, `combat_log.go:187`, `lasting_effects_view.go:84`. Also `combat_damage.go:283` `combatantIndex`.
- What: `combatantIn` was added after the others existed. Eleven places rebuild `map[string]playdb.Combatant` by hand.
- Fix: keep `combatantByID` and `combatantsByID`, move both to `combat_write.go` next to `findCombatant`, delete `combatantIn` and `findByID` (rename call sites, 33 uses in total), and replace the nine inline builders with `combatantsByID(cs)`.
- Changes behaviour: no
- Guarded by a test: yes (compile-time; all call sites are covered by the existing suites)

### play-b-6 · duplication · medium
- Where: `combat_contests_core.go:603` `touch(ctx, c)` is the 6-line "TouchEncounter then c.enc = enc" helper, but 27 other files still inline `c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID)` with the same wrap text, among them the new `combat_opportunity_answer.go:94` (`storeAnswer`), `combat_riders.go` (`closeRider`), `combat_reactions.go:~395` (`shieldChange`), `combat_reaction_rpc.go:~149` (`answerWindow`) and `:~223` (`markRolling`), `puzzles_play.go`, and the four `f.touch` methods of the flow structs (`combat_apply_damage.go:335,466`, `combat_hit_points.go:191`, `combat_take_action.go`). The doc comment on `touch` says "eventTouch bumps..." (a name that does not exist).
- What: a generic helper with a generic name lives in the contests file, so each new split re-wrote it.
- Fix: move to `combat_write.go` as `touchEncounter`, fix the comment, and use it in the files this train just wrote (the ones above first).
- Changes behaviour: no
- Guarded by a test: yes (every write path touches; compile-time)

### play-b-7 · duplication · medium
- Where: the call prologue (member, idempotency key, encounter id) is written again in `combat_contests.go:43` (`contestCaller`), `combat_setup.go:29` (`masterCaller`), `combat_reaction_rpc.go:41` (`parseAnswerInput`), `combat_reactions.go:370` (`parseReactionCall`, returns five values), plus inline in `combat_hide.go` (`Hide`, `ResolveHide`), `combat_help.go` (`Help`, `ClearHelp`), `combat_hide_reveal.go:30-42`, `combat_contests.go:1075` (`parseContestClose`, master-only variant).
- What: every train that split an RPC wrote its own "parse the common header" function with a different shape. Note the order (member, key, encounter) is a behaviour contract, so one function is the safer place for it.
- Fix: one `combatCall{m, key, encID}` + `parseCombatCall(ctx, campaignID, keyRaw, encRaw, role...)` in `combat_write.go`; `contestCaller`, `masterCaller` and `parseReactionCall` become that call plus their own extra id. `parseReactionCall`'s 5-value return disappears.
- Changes behaviour: no (keep the order member, key, encounter, then the rest)
- Guarded by a test: yes (`combat_reaction_auth_test.go`, `combat_write_refusals_test.go`, contests refusal tests)

### play-b-8 · readability · medium
- Where: `combat_reaction_view.go:73-83` (`wait`, `holds`, `holdsSet`) threaded by pointer through `openWindows` (:89), `offerWindows` (:144) and `revealWindows` (:169), each with the same `if !*holdsSet ... *holds, *holdsSet = X, true` fragment (:106, :160, :182).
- What: three loose variables passed as three pointers to three helpers that all do "add to the wait line, remember the first thing held". It is an accumulator that was never named.
- Fix: `type waitLine struct { wait reaction.Wait; holds reaction.Holds; holdsSet bool }` with `func (w *waitLine) holdFirst(h reaction.Holds)`; the three builders take `*waitLine`; `waitLine.build(master bool)` replaces `wv.waitLine`. Also drop the `error` return of `masterCheckPrompt` (`combat_reaction_view.go:401`), it is always nil.
- Changes behaviour: no
- Guarded by a test: yes (`combat_view_golden_test.go`, `combat_reaction_window_test.go`, leaktest `reactions_test.go`)

### play-b-9 · readability · medium
- Where: `combat_reactions.go:135` `func (s *Service) shieldPrompt(...) (*ReactionPrompt, bool, error)` and `combat_reaction_view.go:378` `func (wv *windowView) shieldPrompt(...) error` have the same name for two different jobs (the legacy `reaction_prompts` list and the `reaction_windows` prompt). `nameShieldAttacker` (`combat_reactions.go:161`) is a second copy of `windowView.attackerOf` + `attackName` (`combat_reaction_view.go:354-376`). Three places read the kit with the same `kitOf(...)` + `dbError("work out the reaction")`: `combat_reaction_view.go:383` (shieldPrompt), `combat_reaction_view_more.go:472` (`kitFor`), `combat_reaction_counterspell.go:~162` (`counterspellSlots`).
- What: the train 9 split made the legacy Shield prompt grow helpers that mirror the window view one for one. A reader who greps `shieldPrompt` lands in the wrong one.
- Fix: rename the legacy pair `legacyShieldPrompt`/`legacyShieldAttacker`, or better build `reaction_prompts` from the `windowView` (`wv.attackerOf`, `wv.kitFor`) so the RN-20 "name the attacker only if the viewer sees it" rule lives once. Make `shieldPrompt` (window view) and `counterspellSlots` use `wv.kitFor`.
- Changes behaviour: no
- Guarded by a test: yes (`TestTheShieldPromptTellsEachViewerOnlyWhatTheyMayRead`, `combat_reactions_names_test.go`)

### play-b-10 · docs · medium
- Where (comments that name a thing that no longer exists, or narrate history):
  - `combat_reaction_window.go:534` the comment says `errWindowGone is the not_found of a window...` but the function below it is `errWindowDenied` and returns PermissionDenied (the comment moved and the name changed). Move/reword: "errWindowDenied is the refusal for a window the caller may not answer, the same for one that never existed (RN-10)".
  - `combat_contests_core.go:602` "eventTouch bumps the encounter's revision" (function is `touch`).
  - History words introduced by the trains: `combat_reaction_rpc.go:40` "the order its refusals have always come", `combat_reaction_window.go:526` "as the loop always did", `combat_reactions.go:369` "the order the handlers always checked them", `combat_contests.go:249,614,903,1074` (four copies of "in the order the refusals have always come"), `combat_help.go:115` "as it always has", `combat_reaction_hold.go:73` "as always". Say what the order is for ("member, key, combat: a stranger must get permission_denied before any id is parsed") or delete.
  - Process ids: `combat_reactions.go:25` "(MR-014, timeline decision 5, Etapa 6, slice 6.4b)", `puzzles.go:28` "(MR-038, RN-27, Etapa 10, slice 10.7a)", `combat_reaction_damage.go:63` "(decisions-batch2-A, item 4)". The "Etapa"/"slice" ids point at a plan that is not in the repo; keep the MR/RN id, drop the rest.
  - `combat_opportunity_reactors.go:120` the long doc comment of `opportunityReactors` has one line that runs on to ~130 columns in the middle ("...reactions on any turn). Not exact is a player's warning in GetMoveOptions: only what the viewer") left over from the move; re-wrap.
  - Doc pages `docs/architecture/play/reactions.md`, `contests.md`, `puzzles.md`: checked every file name they cite (`combat_reactions.go`, `combat_reaction_*.go`, `combat_contests*.go`, `combat_contests_window.go`, `puzzles*.go`, `puzzles_rules.go`, `puzzles_hints.go`, `playerRunProto`) exist; no sentence describes the old structure. The only soft spot: `reactions.md` still describes the `reaction_prompts` Shield notice as a separate mechanism (section "Shield") although `reaction_windows` carries the same prompt (see play-b-9).
- Fix: as listed.
- Changes behaviour: no
- Guarded by a test: no (comments)

### play-b-11 · behaviour · medium
- Where: `traps_search.go:438` (`Modifier: clampInt32(r.opt.Bonus + r.dieFace)`) and `:472` (`checkRoll(ev.D20, ev.Modifier, ...)`); frozen in `traps_search_test.go:476-486`.
- What: an effect die the roll used (Guidance's d4) counts in the search total (`d.roll.Total`, event `Total: 16`) but not in `Modifier`, so the answer the player reads is "9 + 4 = 13" while the same search found a DC 14 trap. The player sees a roll that could not have found it. The old code had the same bug (`options[0].Bonus + dieFace`); the characterization test pinned it with a "Finding" comment and an assertion `GetTotal() != 13`. The effect die appears only in `Sources`.
- Fix: in `trapSearchEvent` use the bonus the roll was made with: `Modifier: clampInt32(r.bonus + r.dieFace)` (r.bonus already includes the effect dice from `withEffectDice`). Keep the second Perception die on `r.opt.Bonus` as it is now (that is deliberate). Update the test to expect 16 and the modifier 7.
- Changes behaviour: yes (the answer's total/modifier and the stored event modifier).
- Guarded by a test: it is frozen the other way: `TestSearchForTrapsEffectDiceAndSources` expects the bug.

### play-b-12 · duplication · medium
- Where: `scene.go:750` `errBadIdempotencyKey()` and `puzzles.go:238` `errKeyReused()` (identical bodies, written by two sessions); the same error is also built inline at `traps_search.go:242`, `traps_fire.go:624`, `traps_damage.go:338`, `combat_groupcheck.go:106`, `combat_monsters.go:200`, `combat_start.go:77`, `familiarsight.go:235`, `inspiration_outside.go:124`, `resources.go:212` and `:585`, `vitals.go:114`. A third text exists in `idem.ErrReused()` ("idempotency_key was used for another change"), used at traps_damage.go:`replayByDamage` (`idem.ErrReused()`), which is why `traps_firing_test.go` has to pin two different messages for the same refusal.
- What: one error, 2 named helpers and 11 copies.
- Fix: keep `errKeyReused` (the clearer name), move it to rpc.go next to `errNoOpenSession`, delete `errBadIdempotencyKey`, replace the inline copies (the three in my files are traps_search.go:242, traps_fire.go:624, traps_damage.go:338, scene.go callers use the helper already). Decide once whether `idem.ErrReused()` should say "already used" and fix the one message.
- Changes behaviour: no (the message in `idem.ErrReused()` would change by one word if unified; say so in the commit).
- Guarded by a test: yes for the texts (`searchKeyReused`, `wantTrapRefusal(... "idempotency_key was already used ..." / "was used ...")`).

### play-b-13 · duplication · medium
- Where: `scene.go:176` `lockSceneSession`; `resources.go:557` `lockOpenSessionOrNone` (same body, same error); `puzzles.go:563` `lockOpenSession` (same, different error); inline copies of the same 8 lines: `onscreen.go:59-66` (SetCurrentMap), `onscreen.go:185-192` (swapShownImageIn), `stage.go:166-173` (changeStage), `traps_search.go:202-208`, `traps_fire.go:587-593`, `traps_damage.go:292` (`lockTrapSettleSession`, same plus a "has session" flag), and in other files `combat_groupcheck.go:74`, `inspiration_outside.go:98`, `revivify.go:619`, `revivify_requests.go:136,342`, `lasting_effects_*`, `xp.go:79`, `creature_host.go:71`, `mapseam.go:44`.
- What: the "lock the open session, no rows means NO_OPEN_SESSION, wrap other errors as 'lock the open session'" block exists as 3 helpers under 3 names and about 20 inline copies.
- Fix: one `lockOpenSession(ctx, q, campaignID)` in live.go next to `openSession` (body of `lockSceneSession`); delete `lockSceneSession` and `lockOpenSessionOrNone`; puzzles keep a thin caller that maps `errNoOpenSession` to its own reason if its detail must differ (or accept the generic one if the app reads only the code); replace the inline copies in my files (the six above). Net about 70 lines.
- Changes behaviour: no (puzzles: only if the puzzle blocked-reason is dropped, keep it).
- Guarded by a test: yes, the refusals ("no open session") are tested per call.

### play-b-14 · readability · medium
- Where: `traps_search.go:40-77` (three structs `trapSearch`, `searchTurn`, `searchDice`), `:224`, `:281`, `:318`, `:415`, `:436` (`(ctx, tx, q, ts, r)` everywhere), `:342-347` `rollSearch`, `:185-195` `searchSecondFace`.
- What: the 330-line function became 590 lines (traps.go 509 lines before; traps.go + traps_search.go 705 now) with state spread over `ts` (inputs and outputs), `r` (the transaction's working values) and `d` (dice). Every phase takes both `ts` and `r` and mutates both (`ts.shown`, `ts.place`, `ts.offer` are set from `r` helpers; `r.step` is set by the gate). `prepareSearchRoll` and `askSearchGate` take `tx` and `q` although `r.c` already holds `tx`, `q`, `session`; `rollSearch` is a 5-line dispatch with one caller; `searchSecondFace` has one caller.
- Fix: fold `searchTurn` into `trapSearch` (the old function had them as locals of one closure); drop the `tx, q` parameters of `prepareSearchRoll`/`askSearchGate` and use `r.c.tx`/`r.c.q`; inline `rollSearch` into `searchInTx` (`if cm.Mode != combat.ModeNormal { rollSearchWithMode } else { rollSearchNormal }`); move `searchSecondFace` into `searchRollOf`. Keep `rollSearchWithMode`/`rollSearchNormal` (they carry real branches).
- Changes behaviour: no.
- Guarded by a test: yes (traps_search_test.go, 9 tests).

### play-b-15 · behaviour · medium
- Status: not a regression: the old value was also always `true`, so the refactor kept the behaviour and added a comment that justifies it. It is still a leak of the hint (not of content).
- Where: `lasting_exhaustion.go:140-146` (old `changeExhaustion`: `var secret bool`, never assigned).
- What: RN-10. Old code called `publishLogChanged(..., !secret)` with a `secret` that no line ever set, so it was always `true`. The refactor removed the dead variable and wrote the literal `true` plus a comment ("a hidden NPC's line is Secret in the event itself") that rationalises it. The line is `OwnerOnly` (`writeExhaustion`, `ev.Lasting`) and `Secret` for a hidden NPC, but the `combat_log_changed` hint still goes to every player, so players learn that something happened to a hidden NPC or to another player's exhaustion. Every other handler in the area (`AddLastingEffect`, `ChangeLastingEffectDuration`, `castRevivify`) passes `!ev.Secret`.
- Fix: take `ev, err := resultEvent(res, ...)` as the other handlers do and pass `!ev.Secret && !ev.Lasting.OwnerOnly` (the owner is told through `publishVitals`). Drop the comment.
- Changes behaviour: yes (fix, stream only)
- Guarded by a test: no. `TestLastingCoreExhaustionOfAnNPCOffTurnAndOfAHiddenOne` golden covers rows and events; no test reads the stream for exhaustion, and `leaktest` has no exhaustion probe.

### play-b-16 · readability · medium
- Status: **train 13** holds `refactor/play-casting-end` (head ba2d3eba), which splits `castProto`, `ConfirmCastTimePassed` and `EndActiveSpell`. Stale once it merges.
- Where: `casting_end.go:181-262` (`ConfirmCastTimePassed`, cognitive 23 / cyclomatic 16), `casting_end.go:378-431` (`EndActiveSpell`, cognitive 18); both listed in `.github/go-complexity-baseline.txt:10-11,19`.
- What: the two largest functions of my casting files were not split in train 8 or 12; they are the same "request parse + one closure of 40 lines" shape that `CastSpellOutsideCombat` (`casting.go:284`) was split into `readCastRequest` / `castOutside` / `planOutside`. Also `ConfirmCastTimePassed` and `CastSpellOutsideCombat` parse the roll oneof twice with the same 8-line switch (`casting_end.go:195-205`, `casting.go:711-718`).
- Fix: `readConfirmCall` (roll via a shared `castRollOf(oneof)`), `confirmCastTx(c, call)` containing guardFinish -> mayFinishNow -> planFinish -> checkCastInput -> advanceGameTime -> takeEffect; `endActiveTx` with a `refuseIfCarriedInCombat` helper. Remove both lines from the baseline.
- Changes behaviour: no
- Guarded by a test: yes (`casting_flow_test.go`, `casting_characterization_test.go`)

### play-b-17 · duplication · medium
- Where: `resources.go:601` (`appendKeyedEvent`), `lasting_effects_charadd.go:415` (`writeCharEffectEvent`), `lasting_effects_chars.go:266` (`writeCharacterEndEvent`), `lasting_effects_time.go:119` (`insertGameTimeEvent`); also `puzzles.go:545` (`puzzleSessionEvent`) and the raw copies in `wildshape.go:99-117`, `vitals.go:170-185`.
- What: four helpers in my files (written by different trains) do exactly "json.Marshal(payload) -> NextSessionEventSeq -> InsertSessionEvent with actor, key, hash, CreatedAt: s.now()", with the same three error strings. `appendKeyedEvent` is the general one (it even takes `characterID`).
- Fix: keep `appendKeyedEvent`; delete the other three and call it (`keyedEvent{sessionID, kind, userID, key, hash}`); `writeCharacterEndEvent` also takes a `now time.Time` that is always `s.now()`. Net about -50 lines.
- Changes behaviour: no
- Guarded by a test: yes (`lasting_rpc_home_pin_test.go`, `lasting_effects_charadd_test.go`, `lasting_effects_time_frozen_test.go` read the events)

### play-b-18 · duplication · medium
- Where: "stop the concentration and write the `conditions_set` line" is open-coded at `lasting_effects_rpc.go:571-577` (`holdConcentration`), `lasting_effects.go:490-499` (`dropConcentrationIfIncapacitated`), `lasting_effects_once.go:107-121` (`stopHeldConcentration`), and is already a function: `combat_concentration.go:320` (`loseConcentration`). A fourth place, `lasting_effects_rpc.go:1032-1042` (`stopCasterOf`), re-implements it without `stopConcentrating`.
- What: three copies of `loseConcentration`. `stopCasterOf` diverged: it clears `concentration_spell` and writes the line, but skips `stopConcentrating`, so it does not call `endSummons`, `clearHuntersMark` or `endConcentrationEffects` (compare `creature_cast.go:509-528`). Same in old code, so not a regression; I found no spell that has both a lasting-effect row and creatures or a mark, so I could not make it reachable.
- Fix: call `loseConcentration` from the three copies; make `stopCasterOf` call it after the same lookup. Rename `loseConcentration` to `endConcentrationWithLine` (it is not only for a failed save).
- Changes behaviour: no for the three, yes for `stopCasterOf` (fix)
- Guarded by a test: yes for the three (`lasting_core` goldens); no for `stopCasterOf` creatures/hunter's mark.

### play-b-19 · duplication · medium
- Where: `lasting_effects_rpc.go:722-728` (`touchEncounter`) is byte-identical to `combat_contests_core.go:603-610` (`touch`, 25 callers; its comment still says `eventTouch`). Open-coded `c.enc, err = c.q.TouchEncounter(...)` + `"touch the encounter: %w"` in my files: `lasting_effects_rpc.go:435`, `lasting_exhaustion.go` (`writeExhaustion`), `lasting_effects_save.go` (`settleEffectSave`), `familiarsight.go:387`, `wildshape.go:531`, `creature_cast.go:489`, `revive.go:49`, `revivify.go:379`.
- Fix: delete `touchEncounter`, use `touch` everywhere (fix its comment). Net about -35 lines.
- Changes behaviour: no
- Guarded by a test: yes (any combat write test)

### play-b-20 · duplication · medium
- Where: `resources.go:557` (`lockOpenSessionOrNone`: GetOpenGameSessionForUpdate, ErrNoRows -> `errNoOpenSession`) exists, yet the same 8 lines are open-coded at `lasting_effects_charadd.go:130`, `lasting_effects_chars.go:184`, `lasting_effects_time.go:74`, `revivify_requests.go:136` and `:342`, and in other files (`inspiration_outside.go:98`, `onscreen.go:185`). The name is misleading too ("OrNone" returns an error when there is none).
- Fix: rename to `lockOpenSession`, move it next to `openSessionWith` in `live.go:152`, use it in the five sites of my files (about -35 lines).
- Changes behaviour: no
- Guarded by a test: yes (`TestRevivifyFrozen_RequestsNeedTheOpenSession`, `TestLastingRPCEndingWithNoOpenSession`)

### play-b-21 · duplication · medium
- Where: "the open encounter of the session and its combatants, or none" is written seven times in my files with three different predicates: `casting.go:184` (`inCombat`: CharacterID match and not Dismissed), `creature_cast.go:332` (`summonNotInCombat`: Kind player and CharacterID, a Dismissed combatant counts), `revivify_requests.go:192` (`refuseCasterInCombat`: player and not Defeated), `lasting_effects_charadd.go:182` (`backedByCharacter`), `wildshape.go:67` (`shapeLine`) and `:304` (`playerCharacterOf`), `revivify.go:667,697`, `creature_host.go:70` (`openCombatOf`).
- What: the "is this character in the combat" checks answer slightly different questions (a dismissed or defeated combatant counts in some and not in others), and nobody can tell if that is intended. A defeated caster is "in a combat" for `inCombat` (cast refused) and not for `refuseCasterInCombat` (Revivify request allowed).
- Fix: one `openCombatants(ctx, q, sessionID) (enc, cs, found, err)` next to `openCombatOf`, and one `characterFights(cs, characterID, includeDefeated bool)`; make the predicate explicit at each call and comment why it differs.
- Changes behaviour: no if the predicates are kept; each difference must be decided
- Guarded by a test: partly (`TestRevivifyFrozen_RequestRefusedForACasterInACombat`, `TestSummonsCastSummonInACombatAndInABeastForm`); the Dismissed/Defeated difference is unguarded.

### play-b-22 · readability · medium
- Where: `familiarsight.go:173-193` (`sightRun`) and `wildshape.go:371-392` (`shapeRun`); `familiarsight.go:259-285` (`sightTx`) and `wildshape.go:441-476` (`shapeTx`); `familiarsight.go:377-393` (`sightSeal`) and `wildshape.go:520-537` (`shapeSeal`); generic helpers `playerCharacterOf` (`wildshape.go:304`), `spendAction` (`:339`) and `putBackSpentAction` (`familiarsight.go:477`) live in the shape/sight files but serve both and the summon path.
- What: the two splits are twins written separately: the same preamble (`playerCharacterOf`, `inCombat := who != nil`, `turns := inCombat && Status == active`, `mapID` from the encounter), the same seal (owner, `EncounterID`, `Actor == ""` fallback, TouchEncounter, `c.characterID`), the same undo shape (`find`, `putBackSpentAction`, `keep`, `c.told`). Only `answerFormChange` was shared. `summonCaster` (`creature_cast.go:310-328`) repeats the first 10 lines of `playerCharacterOf` with other messages.
- Fix: new `form_change.go` holding `playerCharacterOf`, `spendAction`, `putBackSpentAction`, `answerFormChange` and one `formScene{who, cs, inCombat, turns, mapID}` built by one `s.formSceneOf(ctx, c, m, characterID)` and one `sealFormEvent(ctx, c, scene, made)`; `sightRun`/`shapeRun` shrink to their own fields. Leave `summonCaster` with its own messages.
- Changes behaviour: no
- Guarded by a test: yes (`familiarsight_test.go`, `wildshape_test.go`, `summons_frozen_test.go`)

### play-b-23 · readability · medium
- Where: `lasting_effects_rpc.go` (1130 lines).
- What: one file holds a read model (`ListLastingEffects`, `looseConditions`, the 6-method `turnClockBuilder`, `concentrations`, `catalog`: lines 38-273, no write) and the nine commands. `turnClockBuilder` exists only for `turnClock` (one caller) and `clockKeyed` only for the builder. A junior looking for "what the master's table shows" opens `lasting_effects_view.go`, which has the player-side view only.
- Fix: move lines 38-273 to `lasting_effects_table.go` (or into `lasting_effects_view.go`); keep `lasting_effects_rpc.go` for the commands (about 850 lines). `entries()` and `add()` of the builder can become a plain function; the sort key struct can be local.
- Changes behaviour: no
- Guarded by a test: yes (`lasting_rpc_view_pin_test.go`, `list_table*.golden`)

### play-b-24 · readability · medium
- Where: `lasting_effects_save.go:276-295` (`effectSaveStep`, 14 fields), `:371` (`effectSaveFacts`), `:297-360` (`writeEffectSave`).
- What: the split of `RollEffectSave` threads one mutable bag `x` through `loadEffectSave -> answerEffectSave -> rollEffectSaveD20 -> readEffectSaveFacts -> recordEffectSaveRoll -> settleEffectSave`; each step reads and writes any field (`x.ev`, `x.le`, `x.run.result`), so the order is the only contract. `recordEffectSaveRoll` returns a `held` that is always false. The sibling concentration save (`combat_concentration.go:266-440`) solved the same problem with a call, a run and explicit values (`concentrationStep`), in fewer types.
- Fix: follow `writeConcentrationSave`: keep `effectSaveCall`/`effectSaveRun`, return `(saved, held)` from one `rollEffectSave` that takes `(st, reactor, t, ability, dc)` as arguments; drop `x.master`, `x.cs`, `x.content` (pass them), drop the unused `held` from `recordEffectSaveRoll`.
- Changes behaviour: no
- Guarded by a test: yes (`effect_save_characterization_test.go`, `lasting_effects_once_frozen_test.go`)

### play-b-25 · tests · medium
- Where: 17 clones of `wantXRefusal(t, call, err, code, msg)`: `combat_start_contest_test.go:26` (`wantRefusal`, the one to keep), `casting_characterization_test.go:21`, `casting_combat_characterization_test.go:19`, `lasting_effects_add_helpers_test.go:41`, `resources_movement_frozen_test.go:17`, `revivify_frozen_test.go:23`, `summons_frozen_test.go:25` (+`asConnectError`, `:48`), `effect_save_characterization_test.go:17`, and 9 more (`combat_core_request_test.go:21`, `combat_help_hide_edges_test.go:18`, `combat_partedit_refusals_test.go:19`, `combat_reaction_branches_test.go:20`, `combat_riders_branches_test.go:17`, `inspiration_outside_refusals_test.go:35`, `puzzles_session_master_test.go:98`, `scene_test.go:23`, `traps_firing_test.go:62`).
- What: same body (errors.As or `errors.AsType`, compare code and message), different name and `Fatalf` vs `Errorf`.
- Fix: keep `wantRefusal`, move it to a `helpers_test.go`, delete the others (about 12 lines each, about -200 lines).
- Changes behaviour: no
- Guarded by a test: n/a

### play-b-26 · tests · medium
- Where: `lasting_core_frozen_test.go:33-52` (`updateLastingCore`, `coreUUID`, `wantLastingGolden`) and `lasting_rpc_pin_helpers_test.go:30-60` (`updateLastingRPC`, `rpcUUID`, `rpcTime`, `wantRPCGolden`); also `puzzles_play_view_test.go:26`, `traps_activity_test.go:27`, `traps_firing_test.go:483`, `traps_firing_view_test.go:24`; 5 UUID regexps (`combat_endturn_test.go:34`, `combat_undo_snapshot_test.go:43`, ...).
- What: two golden harnesses for the same feature (lasting effects), six `-update-*` flags in the package, five identical UUID regexps, two label-memory schemes (`coreNames` vs `rpcRemember`).
- Fix: one `golden_test.go`: `wantGolden(t, dir, name, got)` and a single `-update` flag; one `normalizeIDs`. `lasting_core` and `lasting_rpc` become two directories of the same helper.
- Changes behaviour: no
- Guarded by a test: n/a

### play-b-27 · tests · medium
- Where: `testdata/lasting_rpc/list_table.golden` (645 lines), `change_duration.golden` (593), `list_table_round_two.golden` (509), `visibility.golden` (434), `view_master.golden` (432), `move_combat_round_trip.golden` (351), `characters_master.golden` (172); about 4,000 golden lines for tests of 70-100 lines each.
- What: they dump whole `protojson` of the Encounter/LastingEffect for the master, so any new proto field rewrites seven goldens, and a reviewer cannot read 600-line diffs. They pin the shape of the response (already covered by proto + the web client) and not rules. The `view_*` goldens for other viewers (`view_other`, `view_bystander`, `view_goblin_hidden_*`, 58-199 lines) are the RN-10 guards and are worth keeping, but a golden is a poor tool for "this string is absent".
- Fix: for the master-side goldens assert the ten fields that matter (clock text, ends_at, audience, player_label, targets) and keep only the refusal text and the "rows" tables (`## after the refusals: rows`, 4 lines) as goldens. For `view_*` add assertions that name the secret (canary, as `leaktest/effects_test.go` does) and keep the golden as documentation.
- Changes behaviour: no
- Guarded by a test: n/a

### play-b-28 · readability · low
- Where: `combat_reaction_window.go:287` `kindTriggerGone(...) (gone, decided bool, err error)`.
- What: a three-valued return so that a `switch` can say "nothing of my own, fall through" for the cases `EffectSave` (non-defeated) and `Shield`/`MasterCheck` without a pending id. The caller (`:273-281`) then reads `if gone, decided, err := ...; decided {`. It is the one place where the split made control flow harder than the original (a plain switch with fall-through to the shared checks).
- Fix: keep `triggerGone` as the switch, but call the small kind helpers directly (`concentrationGone`, `fallGone`, `pendingGone`, `contestStillWaits` already exist) and put the shared `holdGone`/`actorGone` tail after it; delete `kindTriggerGone` and `decided`.
- Changes behaviour: no
- Guarded by a test: yes (`TestAWindowClosesByItselfWhenWhatOpenedItIsGone`)

### play-b-29 · duplication · low
- Where: "a contest is waiting" is written three times: `combat_contests_window.go:59` and `:117`, `combat_contests.go:1105` (`closeContestTx`), and the same-meaning partial test at `combat_contests.go:706`.
- What: `row.Status == contestAwaitingDefender || row.Status == contestAwaitingOutcome`.
- Fix: `func contestWaits(row playdb.CombatContest) bool` in `combat_contests_core.go`; use it in the three places.
- Changes behaviour: no
- Guarded by a test: yes (`TestCloseContestClosesWhatWaits`)

### play-b-30 · readability · low
- Where: parameter bundles that have one user or are rebuilt from the same arguments: `groupCall` (`combat_groupcheck.go:57`, built at :69 from `groupWrite`'s own four params; the four callers at :228, :345, :574, :610 each pass `m, key, hash, kind` separately), `outsideGift` (`inspiration_outside.go:148`, only `storeDie`), `hintTryTx` (`puzzles_hints.go:192`, 11 fields; `lock`, `plan`, `skill`, `roll`, `write`, `spend`, `record` are generic one-word method names), `promptFacts` (`combat_reaction_view_more.go:13`, holds both `w` and `t = windowTriggerOf(w)`; `counterspellPrompt` and `fallPrompt` do not take it, so the signatures of the prompt builders are not uniform).
- What: structs added to make a long signature short. `groupWrite` receives the four values only to put them in a struct.
- Fix: make `groupWrite(ctx, call groupCall, do)` and build `groupCall` at the callers; inline `outsideGift` into `storeDie` params (it has two callers' worth of arguments only); name the `hintTryTx` methods `lockRun`, `planTry`, `readSkill`, `rollTry`, `writeTry`; drop `w` from `promptFacts` or pass `t` only.
- Changes behaviour: no
- Guarded by a test: yes (group check, puzzle hint and outside inspiration suites)

### play-b-31 · readability · low
- Where: `combat_contests_options.go:253` `featureHideActions(...) (gone bool, err error)` with `characterGone`/`ignoreGone` (:53, :270), used by `hideAndHelpActions` (:296) and `hideOptionOf` (:278).
- What: the "character is gone" case is carried as a bool plus a swallowed error to stop the next step. `return characterGone(err), ignoreGone(err)` is two helpers to say one thing.
- Fix: `featureHideActions` returns `err` unchanged; `hideAndHelpActions` does `if connect.CodeOf(err) == connect.CodeNotFound { return nil }` after it, and `hideOptionOf` does the same for its read. Delete `ignoreGone`.
- Changes behaviour: no
- Guarded by a test: yes (`TestTurnOptionsReadWhoHoldsAndWhoHelps`, `combat_help_hide_edges_test.go`)

### play-b-32 · readability · low
- Where: names. `combat_opportunity_answer.go:42` `answerInTx` (a function, not a method; suffix noise, `answerOffer` already names the RPC side), `combat_contests.go:510,778,1011,1097,1156` `startContestTx`/`answerContestTx`/`resolveShoveTx`/`closeContestTx`/`releaseTx`, `combat_help.go` `checkHelpWithACheck`/`checkHelpWithAnAttack`, `combat_groupcheck.go:482,499` `memberRollOf` -> `gatedMemberRoll` (returns `roll, settled, held, err`, a 4-tuple behind a 3-level chain `rollMember` -> `memberRollOf` -> `gatedMemberRoll` -> `heldGate`), and the same-name pair `combat_contests.go:131` `releaseHold` (end a grapple) vs `combat_reaction_hold.go:157` `(*Service).releaseHold` (replay a held action).
- What: the `Tx` suffix says nothing the `*combatTx` parameter does not; the two `releaseHold` are unrelated and easy to mix up when reading a stack trace.
- Fix: rename the grapple one to `endGrapple`; drop `Tx` (`startContest`, `answerContest`, `resolveShove`, `closeContest`, `releaseGrapple`) and let the RPC be the exported name; fold `memberRollOf` and `gatedMemberRoll` into one function returning `(contestRoll, held bool, err)` (use an early `if gh == nil || spec.byMaster { return plain }`). In the five RPCs the closure `made = actionEvent{}; var err error; if made, err = s.xTx(...); err != nil {return nil, err}; return made, nil` can be `made, err = s.x(...); return made, err`.
- Changes behaviour: no
- Guarded by a test: yes

### play-b-33 · tests · low
- Where: `combat_reaction_each_test.go:340` `TestAFallThatNobodySlowsLandsWhenTheWindowIsPassed` is a strict subset of `combat_reaction_branches_test.go:381` `TestWhereAFallLandsWhenTheWindowIsPassed` (the latter passes the same window with three fallers and checks Toren's `rolled` too). Also `combat_reaction_branches_test.go:116`, `:271`, `:658` (`TestPassing{HellishRebuke,UncannyDodge,DeflectMissiles}AnswersItAndSpendsNothing`) share one assertion block ("result not used, reaction not spent, no windows left / pending rolled for 8") on three different classes.
- Fix: delete the each_test one (about 12 lines, one `fallingRig` setup if nothing else uses it: check `fallingRig` callers, `TestFeatherFallTakesTheFallDamageOfAPitAway` at :306 still does). Turn the three pass tests into one table `TestPassingAReactionAnswersItAndSpendsNothing` with `reactorFight(t, <class>)` per row; the setups stay (different class) but 40 lines of repeated assertions go.
- Changes behaviour: no
- Guarded by a test: n/a

### play-b-34 · readability · low
- Where: `traps_search.go:284-286`, `:294`, `:310`.
- What: (a) the comment "Reliable Talent: a d20 of 9 or lower counts as 10..." is stranded between `checkModeOf` and its `if err != nil`; it described `reliable := options[0].ReliableTalent`, which moved to :223. (b) `ts.shown = shownSources(r.cm.Sources, r.names)` at :294 is overwritten at :310 by `r.cm.shownCheck(r.names)` before anything reads it (the held return in between does not use it): a dead assignment.
- Fix: delete :294; move the Reliable Talent comment to :223 where `reliable: opt.ReliableTalent` is set.
- Changes behaviour: no.
- Guarded by a test: n/a.

### play-b-35 · behaviour · low
- Where: `traps_search.go:95-98` and `:476` (`Sources: ts.shown`); frozen in `traps_search_test.go:519-523`.
- What: the retry of a search answers without `Sources` (only the first call works them out), so the replay is a smaller message than the first answer. Pre-existing, pinned by a "Finding" test.
- Fix: either keep the circumstances in the event (`actionEvent` has `Sources []sourceRec` elsewhere: use `sourceRecs`/`shownRecs`) and rebuild them on the replay, or document in the proto comment that a replay carries no sources and drop the "Finding" test as noise. First option is two lines in `trapSearchEvent` and one in `trapSearchAnswer`.
- Changes behaviour: yes (replay answer gains `sources`).
- Guarded by a test: frozen the other way (`TestSearchForTrapsRetries`).

### play-b-36 · duplication · low
- Where: `traps_damage.go:168-171` (map literal built on every call inside `trapDamageProto`), `traps_activity.go:282-285` `damageStatusProto`, `combat_actions.go:344` `pendingStatusToProto` (superset).
- What: the same status map three times.
- Fix: delete the first two, use `pendingStatusToProto` (also swap the string literals `"rolled"`, `"applied"`, `"discarded"` in traps_damage.go:267-276 for `pendingRolled`/`pendingApplied`/`pendingDiscarded`).
- Changes behaviour: no.
- Guarded by a test: yes (`TestListTrapDamagesGolden`, trap_damages golden).

### play-b-37 · duplication · low
- Where: `traps_activity.go:25-47` `creatureLabels` and `traps_activity.go:257-278` `readCreatureNames`; the "list ids, ask the roster, fill `map[id]name`" loop also at `scene.go:550` (masterSceneNotes) and `:614` (rollerNames), `traps_activity.go:247`, `traps_damage.go:130`, `traps_fire.go:417`, `stage.go:98`.
- What: `creatureLabels` and `readCreatureNames` run the same query (`CharacterCreatures` in `db.InTx`, sorted+compact owners, `id -> Name`); one wraps the error, the other returns the partial map and the error. Six more loops build character-name maps from `CombatCharacters`/`SessionCharacters`.
- Fix: `creatureLabels` becomes `readCreatureNames` into a fresh map (callers wrap with `dbError` already at traps_fire.go:236); add one `s.characterNames(ctx, campaignID, ids) (map[string]string, error)` using `SessionCharacters` and use it in the six places.
- Changes behaviour: no (CombatCharacters and SessionCharacters differ on dead characters: keep the right call per site, the helper takes a flag or two variants).
- Guarded by a test: yes (golden/activity tests).

### play-b-38 · readability · low
- Where: `traps_fire.go:521` `fireOutsideCombat(ctx, campaignID, actorUserID, key, hash, trap, targetIDs, creatures, manual, extend)` (10 parameters) and `:499` `outsideFiring`, built at `:529` from the same 9 values.
- What: the parameter list and the struct carry the same fields; callers `fireByHandOutsideCombat` (:225) and `fireDropped` (:918) pass 9 positional values, two of them `nil`/`""`/`false` that mean "dropped token".
- Fix: let the two callers build the `*outsideFiring` and pass it; `fireOutsideCombat(ctx, f)` does `outsideCaught`, the test hook, the tx, and returns `f`. Drop the 4-value return.
- Changes behaviour: no.
- Guarded by a test: yes (traps_firing_test.go, traps_test.go).

### play-b-39 · readability · low
- Where: `onscreen.go:173-183` `swapShownImage`+`swapShownImageIn`; `traps_fire.go:584` `fireOutsideTx`; `traps_damage.go:244` `settleTrapDamageTx`; `traps_search.go:199` `searchInTx`; `scene.go:189` `openSceneIn`; `rpc.go:176` `endSessionIn`; `rpc.go:92` `openGameSession`; `combat_traps.go:252` `triggerInCombat`; `traps_effect.go` `saveDice`/`rollParts`.
- What: four suffixes for "the part inside the transaction": `In`, `InTx`, `Tx`, `On`. `swapShownImage` is a 7-line wrapper whose only job is to reset two fields on retry; `triggerInCombat` is 6 lines with one caller (`fireInCombat`) stacked on `triggerFiredTrap`; `trapRolls.saveDice` and `rollParts` have one caller and 5-10 lines. `openGameSession` reads like "open a session" (the read is `openSession`) but it inserts one and locks the sheets.
- Fix: merge `swapShownImage` and `swapShownImageIn` (reset inside the closure, as the old code did); fold `triggerInCombat` into `fireInCombat`; rename `openGameSession` to `insertGameSession`; pick `...Tx` as the only suffix (`searchTx`, `endSessionTx`, `swapShownImageTx`) and rename `openSceneIn` to `openScenePoint`.
- Changes behaviour: no.
- Guarded by a test: yes.

### play-b-40 · readability · low
- Where: `live.go:270-289` `forwardHubEvent(stream, sub, sessionID, ev, ok) (bool, error)` and `:290` `recheckWatch`.
- What: `forwardHubEvent` receives the `ok` of the channel receive and handles the closed channel inside; the `(done bool, err error)` return needs a comment on every use and a `//nolint:nilerr` (:285). The select in `watchLoop` now reads `if done, err := ...; done { return err }` twice.
- Fix: in `watchLoop` handle `!ok` there (`return watchEnd(sub)` with the 8-line slow/replaced switch), and let `forwardHubEvent` return `(end bool, err error)` only for the send. Or keep as is and drop the `ok` parameter by passing the closed case through a small `streamEnd(sub) error`.
- Changes behaviour: no.
- Guarded by a test: yes (live_test.go).

### play-b-41 · readability · low
- Where: `traps.go:69-80` (`searchPerception`, `searchInvestigation`, `searchCheckKey`, `errNoTraps`), `:91-107` (`runningEncounterOn`, `errCombatBegan`), `:109-114` (`mapChangedHint`); header comment `traps.go:17-27`.
- What: traps.go is now the `TrapBook` interface plus leftovers; the search constants live 600 lines away from the only file that uses them, `runningEncounterOn`/`errCombatBegan` belong to the firing, `mapChangedHint` is a stream helper used by 4 files. The header says traps.go holds "the player's Procurar armadilhas, the master's firing by hand and the damage ..." which are in traps_search.go, traps_fire.go and traps_damage.go.
- Fix: move the search constants to traps_search.go, `runningEncounterOn`+`errCombatBegan` to traps_fire.go, `mapChangedHint` to onscreen.go next to `Publish`; rewrite the traps.go header to "the TrapBook interface".
- Changes behaviour: no.
- Guarded by a test: n/a.

### play-b-42 · docs · low
- Where: comments with project history: `traps.go:17` ("Etapa 9, D5, slice 9.8"), `traps_damage.go:25`, `traps_effect.go:17`, `combat_traps.go:23`, `scene.go:29` ("Etapa 7"), `live.go:20` ("Etapa 5"), `play.go:3` `:8` `:293`, `traps_fire.go:859-860` ("9.10 left this to the traps slice"), `mapseam.go:90` ("slice 9.7 sees combatants by...").
- What: phase ids and "left to the slice" history in comments; none says why.
- Fix: delete the "Etapa/slice" tokens (keep MR/RN ids); rewrite `traps_fire.go:859` and `mapseam.go:90` to say what the code does today.
- Changes behaviour: no.
- Guarded by a test: n/a.

### play-b-43 · docs · low
- Where: `play.go:237-238` and `:671-672` (also `progression/progression.go:240-241`, not mine); `traps_effect.go:24`.
- What: the comments cite `docs/architecture/README.md, "Dentro de uma transação, nenhuma leitura pelo pool"`. That heading does not exist; the rule is now `docs/architecture/transactions.md` ("Inside a `db.InTx` or `db.ReadTx` closure, nothing reads through the pool"). `traps_effect.go:24` says "The rules the table can read in docs/architecture/README.md" followed by a rule list; the rules are in `docs/architecture/play/traps.md`, row "The effect".
- Fix: point the first two to `docs/architecture/transactions.md`, the third to `docs/architecture/play/traps.md`.
- Changes behaviour: no.
- Guarded by a test: n/a.

### play-b-44 · docs · low
- Where: `docs/architecture/play/traps.md` row "Searching", first sentence: "In a running combat the character is part of it is the Search action"; row "Search rules (SRD)": the `SEARCH_NEEDS_TWO_DICE` rule (see play-b-1).
- What: a garbled sentence, and the rule is stated without the "only on a map that has a trap point" condition the code has today (which is the bug of play-b-1). The page never names `traps_search.go`/`trapSearch`, the file the search lives in.
- Fix: "In a running combat where the character is a combatant, the search is the Search action"; state the rule after the play-b-1 fix; add the three files to the piece table (`TrapBook` in traps.go, search in traps_search.go, firing in traps_fire.go/combat_traps.go).
- Changes behaviour: no.
- Guarded by a test: n/a.

### play-b-45 · tests · low
- Where: test headers and comments in `traps_firing_test.go:32-36`, `traps_effect_rolls_test.go:14-17`, `traps_search_test.go:17`, `traps_search_test.go:479` and `:519` ("traps.go:410", "traps.go: shown is only worked out ..."), `traps_firing_test.go:436,464`, `scene_test.go:233` ("Finding: ...", "written before FireTrap ... were split", "compared to what it was before the function was split"), plus `onscreen_shown_test.go:15`, `package_stage_test.go:12`, `rpc_sessions_test.go:17`, `stage_chars_test.go:11` ("Characterization of ...").
- What: the headers narrate the refactor ("before the function was split"), and the "Finding" notes point at code that has moved (`traps.go:410` is now `traps_search.go:472`). A new reader cannot tell which "Finding" is a bug (play-b-11, play-b-35) and which is a design decision.
- Fix: rewrite each header as one sentence of what the file guards; turn each "Finding" into either a bug entry (play-b-11/play-b-35) or a plain comment of why.
- Changes behaviour: no.
- Guarded by a test: n/a.

### play-b-46 · behaviour · low
- Where: `familiarsight.go:222-225` vs `wildshape.go:430-435`.
- What: `shapeChange` publishes vitals and calls `maps.VisionChanged` under `afterCommit(ctx)` ("a caller that hangs up must not leave the fog unheard"); `sightChange`, the other half of the same pair and the one that changes the fog most (it adds an observer at the familiar), uses the request `ctx`. If the player's request is cancelled after commit, the fog is not updated for a sight start, but is for a Wild Shape. Same in old code; the two splits made the difference visible.
- Fix: move the post-commit publish (vitals + VisionChanged under `afterCommit`) into the shared `form_change.go` (play-b-22).
- Changes behaviour: yes (fix)
- Guarded by a test: no (`TestMR036_EveryChangeOfAFormOrASightTellsTheFog` does not cancel the context)

### play-b-47 · duplication · low
- Where: `wildshape.go:87-118` (`formEnds`), `wildshape.go:122-149` (`endFormIfAsleep`), `wildshape.go:238-265` (`damageBeast`), `vitals.go:162-190` (`leaveBeastForm`, another train's file).
- What: four writers of `eventWildShapeEnded`; `formEnds` and `leaveBeastForm` both do NextSessionEventSeq + shapeLine + InsertSessionEvent by hand with the same payload fields (`Beast`, `Reason`, `EncounterID`, `Round`, `Actor`, `Secret`).
- Fix: one `s.writeFormEnded(ctx, q, sessionID, ev actionEvent, userID *string)` used by `formEnds` and `leaveBeastForm`; `endFormIfAsleep` and `damageBeast` already share `insertEvent` and can share an `endBeastForm(ctx, c, who, w, reason, carried)`.
- Changes behaviour: no
- Guarded by a test: yes (`wildshape_review_test.go`, `wildshape_log_test.go`)

### play-b-48 · duplication · low
- Where: `casting_effects.go:395-435` (`summonFor`) vs `creature_cast.go:276-421` (`castSummonTx`, `summonWrite`, `dismissReplaced`); `creature_cast.go:409-421` and `casting_effects.go:424-433` carry the same "reason := concentration, replaced when Source == familiar; insertEvent(creature_dismissed)" loop.
- What: `summonFor` (a cast through `CastSpellOutsideCombat`) and `CastSummon` are two entry points for the same write: checkSummonChoice, ritual refusal, summonNames, SummonCreatures, dismissal events.
- Fix: `summonOutside(ctx, c, m, characterID, spellKey, circle, ritual, pick) (sum, chk, err)` and `dismissReplacedCreatures(ctx, c, userID, characterID, replaced)`; both callers use them.
- Changes behaviour: no
- Guarded by a test: yes (`creatures_test.go:222`, `summons_frozen_test.go:123`)

### play-b-49 · readability · low
- Where: `casting_end.go:72-80` (`armorOf`), `casting_end.go:82-95` (`otherCastsOn`), `queries_casting.sql:91` (`ListLiveSpellCastsOnTarget`), `casting_end.go:68-70` (`takeBackCast`) and `casting_effects.go:437-440` (`takeBackMaxHP`), `casting.go:56` (`castMaxHP`).
- What: the `unused` linter (`/tmp/gl/golangci-lint run --enable-only=unused --new=false ./internal/play/...`) reports exactly two: `armorOf` and `(*Service).otherCastsOn`. Both were dead at 28dd0d5 too and no test calls them (`casting_flow_test.go:239` has a local closure called `armorOfToren`, unrelated). `castSpellsOn` does not exist at 28dd0d5 or on main. After deleting `otherCastsOn`, the sqlc query `ListLiveSpellCastsOnTarget` has no caller. Separately, `takeBackCast` only forwards to `takeBackMaxHP`, which returns `nil, nil` (a stub: "takes Aid's hit point bonus off"), and `closeCast` (`casting_end.go:49`) calls it for every active cast.
- Fix: delete `armorOf`, `otherCastsOn`, the query (then `sqlc generate`); the `encoding/json` import stays (used by `castTargetsOf`). Delete `takeBackCast` and `takeBackMaxHP` and the `takeBackCast` block in `closeCast`, or write Aid's take-back.
- Changes behaviour: no
- Guarded by a test: n/a (unused)

### play-b-50 · readability · low
- Where: `casting.go:670-676` (`effectTargets`) vs `casting_effects.go:360-365` (`effectTargetsAll`); `casting.go:170-181` (`stageOf`) vs `casting.go:220-223` (`guardCast` builds the same map inline from `sm.byChar`); `casting.go:485-510` (`mayCastNow`) vs `casting_end.go:290-306` (`mayFinishNow`); `casting_view.go:193` (`afterCastChange(..., _ string)`, six callers pass `""`); `casting_view.go:230-258` (`castAnswer`, `castAnswerFinish` each re-read the event after `afterCastChange` read it already); `casting.go:592` (`checkCastTargets(_ context.Context, _ *combatTx, ...)`).
- What: `effectTargets` is `effectTargetsAll` minus NPCs, written twice with different names; unused parameters; `afterCastChange` reads the event and the caller reads it again.
- Fix: `effectTargets = filter(effectTargetsAll, Player)`; `g.stage = sm.onStage()` (method on `stageMap`) and delete the duplicate body in `stageOf`; `mayCastNow` calls `mayFinishNow` (the message differs: pass it); drop the `string` and the unused parameters; have `afterCastChange` return the event it read.
- Changes behaviour: no
- Guarded by a test: yes

### play-b-51 · readability · low
- Where: `wildshape.go:544-592` (`shapeUndo` -> `shapeUndoStart`/`shapeUndoEnd` -> `shapeUndoSet`, each carrying `(who, ok)`); `familiarsight.go:88-146` (`familiarSquares` -> `combatantSquares` / `familiarMapSquares` -> `squaresOfTokens`) and `:336-375` (`sightFamiliar` -> `sightInReach`); `lasting_effects_home.go:68-72` (`castClock`) and `:272-277` (`effectEndAt`); `revivify.go:617` (`setBlockedIn`), `lasting_effects_time.go:72` (`advanceGameTimeIn`).
- What: one-caller helpers that rename and pass the same four arguments down; `castClock`/`effectEndAt` are 2- and 3-value tuples given a type to avoid a long parameter list; two `xxxIn` names.
- Fix: fold `shapeUndoStart` and `shapeUndoEnd` into `shapeUndo` (about 25 lines; `ok bool` + `who` becomes `who *playdb.Combatant`); fold `sightInReach` into `sightFamiliar` and `combatantSquares` into `familiarSquares`; pass `seconds, kind` to `putCharacterEffect` as plain parameters; rename `setBlockedIn` to `setBlocked` and `advanceGameTimeIn` to `advanceGameTimeTx`.
- Changes behaviour: no
- Guarded by a test: yes

### play-b-52 · docs · low
- Where: `docs/architecture/play/casting.md:49` ("The disabled spell row says which side blocks it (`spellReasonText`)": no such symbol anywhere in Go or the web client); `creature_cast.go:22` ("MR-037, Etapa 9"), `familiarsight.go:18` ("MR-036, Etapa 9, D6"), `wildshape.go:23` ("MR-037, Etapa 9, D7"); `casting.go:371` (user-visible error "summon goes in FinishCast ..."), `casting.go:399,403`, `casting_view.go:245`: the RPC is `ConfirmCastTimePassed` (`proto/meurpg/play/v1/casting.proto:175`), `FinishCast` does not exist; test headers "written before the function split" in `casting_characterization_test.go:15`, `casting_combat_characterization_test.go:14`, `revivify_frozen_test.go:17`, `revivify_requests_frozen_test.go:16`, `summons_frozen_test.go:13`, `resources_movement_frozen_test.go:13`, `lasting_core_frozen_test.go` (header, "before the big functions are split").
- Fix: replace `spellReasonText` with the real function (look in `combat_spells*.go` for the disabled-reason text) or drop the sentence; strip "Etapa N, Dn" from the three headers; rename `FinishCast` to `ConfirmCastTimePassed` in the error text and comments; reword test headers to say what they guard, not when they were written. The other four pages (creatures, lasting-effects, death-and-revival, rests-and-resources) name only symbols that exist; I checked every camelCase identifier and `.go` file in them (`familiarEyes` is in `maps`, `moverId`/`wildActionLine` in the web client, as intended).
- Changes behaviour: yes for the error text only
- Guarded by a test: no

### play-b-53 · tests · low
- Where: known-wrong behaviours pinned as expected values: `lasting_effects_lastadd_test.go:200,218,341` and `lasting_effects_charadd_test.go:172,193,196` ("frozen: looks wrong because ..."), `lasting_effects_core_frozen_test.go:164` (play-b-2), `lasting_effects_armor_frozen_test.go:127-154` (`TestLastingCoreTheBaseArmorClassOfACreatureWithNoSheet`: calls an unreachable branch with a hand-made combatant).
- What: six "looks wrong" cases (an unknown audience reads as everyone for an owner-only Poisoned, seconds with no kind are dropped, Mage Armor on an armored target succeeds and writes an empty event, a retry answers without the cards) live only in test names. Nobody owns them; the first change that touches them will update a long expected string without noticing.
- Fix: one issue per case, with the test name in it; delete the `NoSheet` test (the branch is unreachable by its own comment: `combatants.character_id` is NOT NULL) and the branch itself if truly dead.
- Changes behaviour: no
- Guarded by a test: yes (that is the problem)

## Test trim table

### reactions, contests, puzzles

Tests/goldens added since 28dd0d5 in my area. Every file below builds the full API stack (campaign, characters, encounter) through helpers such as `reactorFight`, `newArmedWith`, `newSneaks`/`arena`; only counts of tests are given since I could not run anything.

| file | lines | added by | verdict | reason |
| --- | --- | --- | --- | --- |
| `combat_contests_answer_test.go` | 806 (17 tests) | train 12 (#454) | keep | refusal order of RespondContest/ResolveShove/CloseContest/Release and the not_found vs permission_denied split (RN-10); most pin cases no other test reaches. Optional: fold `TestResolveShoveByAPlayerThatDefendsIsRefused` (:451) into `TestResolveShoveRefusals` (:377), about 35 lines and one setup |
| `combat_contests_read_test.go` | 353 (7) | train 12 | keep | RN-10 reads of contest, help and grapple state for a player who does not see the NPC |
| `combat_groupcheck_refusals_test.go` | 343 (6) | train 7 (#446) | keep | refusals and the player-not-asked-reads-nothing rule |
| `combat_help_hide_edges_test.go` | 184 (4) | train 8 (#448) | keep | Help/Hide edge refusals; small |
| `combat_hide_reveal_test.go` | 97 (2) | train 11/12 | keep | only coverage of RevealHider and its undo |
| `combat_reaction_branches_test.go` | 761 (18) | train 9 (#450) | merge | 14 `reactorFight` setups; merge the three "passing X" tests into one table (play-b-33, about 40 lines); keep the rest (refusal words, window auto-close, answer order, shield prompt per viewer) |
| `combat_reaction_each_test.go` (existing) | 400 | before | trim | delete `TestAFallThatNobodySlows...` (:340, about 12 lines, 1 setup) |
| `combat_reaction_counterspell_check_test.go` | 157 (3) | train 9 | keep | the ability-check branch of Counterspell |
| `combat_reaction_counterspell_sight_test.go` | 157 (1, 5 subtests) | train 9 | keep | who sees the caster; RN-10 relevant |
| `combat_riders_branches_test.go` | 229 (6) | train 8/9 | keep | Stunning Strike / Open Hand branches |
| `inspiration_outside_refusals_test.go` | 242 (4) | train 12 | keep | refusal order of the outside die |
| `puzzles_play_view_test.go` | 226 (3) + 11 goldens `puzzle_view_*.golden.json` (552 lines) | train 9/10 | keep | the goldens are the only whole-view RN-10 check of the puzzle per viewer (master, player, other player); they overlap with the field asserts of `TestRN10_PlayersNeverReceiveTheAnswer` (`puzzles_test.go:1458`) and `TestRN10_PlayersNeverReceiveWhatTheNewKindsHide` (`puzzles_more_test.go:1367`) but are stronger. If a trim is wanted, drop the two `trap_*` goldens that duplicate `trap_firing_*`-style asserts in `puzzles_test.go` (about 150 lines) |
| `puzzles_session_master_test.go` | 413 (11) | train 10 | keep | master-only gate, create/replay/key, archive idempotence, hint-try refusals |

Estimated saving from the trims above: about 90 lines of test code and 2 campaign setups out of roughly 15 in these files, i.e. a few percent of this area. There is no large pinning-only test in my area; the cost is in the number of full-stack setups, not in the added files.

### traps, scenes, plumbing

Added since 28dd0d5 in my area (non-lasting, non-puzzle). CI time is estimated, not measured (no test runs allowed): each `newTrapRig` builds a cave (campaign, 4 users, map, characters) plus an httptest server and the maps service; assume 0.3-0.5 s each, the tests run `t.Parallel()`, so the saving is a few CPU seconds, not wall minutes.

| file | lines | added by | verdict | reason |
| --- | --- | --- | --- | --- |
| traps_search_test.go | 595 (9 tests, 16 `newTrapRig`) | #446 (train 7) | merge | Keep `RefusesABadRequest`, `WhoMayCall`, `NeedsTrapsASessionAndAPlace`, `EffectDiceAndSources` (needed for play-b-11). `TestSearchForTrapsRetries`+`KeyOfAnotherChange` repeat `TestSearchForTrapsKeyIsPerCaller` and `TestAKeyReusedForAnotherVitalsOrSearchRequestIsRefused` (traps_round2_test.go:675,747): one table test with one rig. `RollModes` repeats `TestMR035_APerceptionSearchInDimLightHasDisadvantage` and `TestMR035_APhysicalDieFollowsTheCampaign`: fold the 6 sub-rigs into subtests that reuse one rig. `InACombat` overlaps `TestMR035_SearchingInACombatCostsTheAction`: keep only the cases the old one lacks (not begun, not placed). About -150 lines, -6 rigs |
| traps_firing_test.go | 551 (6 tests, 6 rigs) | #441 (train 4) | merge | `TestTrapFiringRefusals`, `TestTrapFiringThatLosesItsSession`, `TestListTrapDamagesGolden`: keep. `TestTrapDamageSettlingRefusals` repeats `TestMR035_AKeyReusedOnAnotherTrapDamageIsRefused` and `ATrapDamageKeyReusedWithAnotherAmountIsRefusedInTheSession` (traps_round2_test.go:505,897): fold them in one test. `TrapFiringOutsideACombatRollsEveryCreature` and `InACombatMergesConditions` overlap `TestMR035_TheMasterFiresATrapByHand`/`TestMR035_OutsideACombat`: keep the unique assertions only. About -100 lines, -2 rigs |
| traps_firing_view_test.go + 4 `trap_firing_*.golden.json` + `trap_damages_master.golden.json` | 126 | #441 | keep | per-viewer proto of a firing (master, owner, other player, bare): this is the RN-10/RN-20 read; a golden is the right tool and it needs no database |
| traps_effect_rolls_test.go | 296 (2 tests, 0 rigs) | #441 | merge | pure arithmetic like `traps_effect_test.go` (172 lines): move into that file; the header "compared to what it was before the function was split" goes (play-b-45). Cheap to run, no CI gain, one file less |
| scene_test.go | 892 (11 tests, 11 harness setups) | #446 | merge | Real behaviour (refusals with exact messages, per-viewer read, retry answer): keep. `TestSceneGrantAttemptOnAnUnlimitedAction`, `TestSceneThePointStoppedBeingAScene`, `TestSceneRemovedActionAndOpenScenePoint`, `TestSceneOpenedBeforeEventsWereKept` each build a full app for 20-40 lines of assertions: make them subtests of the tests that already build one (`...GrantAttemptRefusalsAndEffects`, `...OpenCloseRefusals`, `...ReadByEachViewer`). About -80 lines, -4 setups. `TestSceneRollWithDisadvantage` overlaps `inspiration_lucky_test.go`/`held_roll_kinds_test.go` only on the gate, not on the mode: keep |
| onscreen_shown_test.go | 297 (6 tests) | #452 (train 10) | keep | the only tests of `SetShownImage`'s keep/replace/copy/FK paths, which were split; each needs the real maps module. `...RefusalsBeforeAndWithoutASession` could join `ShowKeepReplaceAndStop` as subtests (-1 setup) |
| package_stage_test.go | 213 (4 tests) | #452 | keep | tests `stageOnSolve`/`stageOnWrong` split in package.go; no database, fast, covers the new `stageSolveTarget`/`stageWrongLimits`/`stageWrongTrap` |
| rpc_sessions_test.go | 113 (2 tests) | #452 | keep | StartGameSession key/replay and EndGameSession with the kept image: the two paths the split moved |
| stage_chars_test.go | 48 (1 test) | #452 | delete or move | builds `newSneaks` + a combat arena to get a monster's NPC for one refusal; the master-only check at its end repeats `TestStageAuthorizationMatrix` (maps/stage_test.go:463). Move the monster-NPC refusal next to `TestMR031_TheMasterPutsNPCsOnStage`, where a stage rig exists |

Estimated total from the trims above: about 450 lines and 12 rigs/harness setups, a few CPU seconds per run, no loss of a branch (every removed assertion has a twin listed in the row).

### casting, lasting, revival, resources

Added since 28dd0d5 in my area (`git diff --diff-filter=A`). Each `Test` function builds `newCasters`/`newArmed` (a campaign, four users, heroes created through RPCs); about 119 setups in these files. I did not run the suite: the time figures are estimates, assuming 0.3-0.5 s of setup each, and most tests are `t.Parallel`, so the saving is CPU time more than wall time.

| file | lines | added by | verdict | reason |
| --- | --- | --- | --- | --- |
| casting_characterization_test.go | 435 | train 8 (casting) | keep | refusal messages and the per-viewer `ListSpellCasts`/`GetCastOptions` (RN-10); only swap `wantCastRefusal` for the shared helper (play-b-25) |
| casting_combat_characterization_test.go | 210 | train 8/11 | keep | belongs to the combat-casting reviewer; same helper clone (`wantCombatCastMessage`) |
| lasting_core_frozen_test.go | 322 | train 8 | merge | pure golden infra, duplicated by `lasting_rpc_pin_helpers_test.go` (play-b-26) |
| lasting_effects_core_frozen_test.go | 267 | train 8 | keep | rows/events/clock of effects; includes the play-b-2 test (flip it when fixed) |
| lasting_effects_gaps_frozen_test.go | 119 | train 8 | merge | 4 small tests of core branches; fold into core_frozen (saves 3 setups) |
| lasting_effects_armor_frozen_test.go | 154 | train 8 | keep 2 of 3 | `GearChanged`, `ArmorWorn` guard writes; delete `...WithNoSheet` (unreachable, play-b-53) |
| lasting_effects_log_frozen_test.go | 126 | train 8 | keep | RN-10: DC and roll only for master/owner |
| lasting_effects_once_frozen_test.go | 127 | train 8 | keep | the die spent, in and out of combat |
| lasting_effects_spells_frozen_test.go | 169 | train 8 | keep | take-hold rules by target kind, `outsideModeOf` |
| lasting_effects_time_frozen_test.go | 93 | train 8 | merge | overlaps `lasting_rpc_time_pin_test.go` (68) and `outside_time.golden`; keep one |
| lasting_exhaustion_frozen_test.go | 304 | train 8 | keep | ladder, death at 6, hidden NPC; add the stream assertion of play-b-15 |
| lasting_rpc_calls_pin_test.go | 489 | train 8 | keep refusals, cut goldens | refusal tables are the value; the 593/434-line goldens of `change_duration`/`visibility` pin the proto (play-b-27) |
| lasting_rpc_cast_pin_test.go | 115 | train 8 | keep | cast ends in combat, effects leave |
| lasting_rpc_home_pin_test.go | 186 | train 8 | keep | effects on characters |
| lasting_rpc_move_pin_test.go | 102 | train 8 | keep | combat/home round trip; the 351-line golden can shrink |
| lasting_rpc_time_pin_test.go | 68 | train 8 | merge | see time_frozen |
| lasting_rpc_view_pin_test.go | 143 | train 8 | keep | the RN-10 views per viewer (other, bystander, hidden goblin); add canary-style asserts |
| lasting_rpc_pin_helpers_test.go | 476 | train 8 | merge | about a quarter is golden infra duplicated with core (play-b-26) |
| resources_movement_frozen_test.go | 147 | train 12 | keep | refusal order of SpendHitDice/ArcaneRecovery/rage; `wantResFrozen` is a clone |
| revivify_frozen_test.go | 509 | train 8 | keep | NOT_SEEN, HIDDEN, fog, window edges: the RN-10 guards of revival |
| revivify_requests_frozen_test.go | 394 | train 8 | keep | request refusals, replays |
| summons_frozen_test.go | 357 | train 12 | merge | 8 tests that mostly pin refusal text in call order; fold the three `...RefusesEachBadRequest` into table tests, keep `RetryAnswersTheSame` and `HuntersMark`; overlaps `familiarsight_test.go:312`, `wildshape_review_test.go:163` |
| testdata/lasting_core/*.golden (24 files) | about 550 | train 8 | keep | small, readable |
| testdata/lasting_rpc/*.golden (31 files) | about 5,200 | train 8 | cut 7 files | play-b-27: about -2,900 golden lines |

Estimated saving: about 200 lines (play-b-25), about 120 lines of golden infra (play-b-26), about 2,900 golden lines plus 100 test lines (play-b-27), 25 lines (play-b-53); 8-10 fewer harness setups (about 3-4 s CPU) by folding `gaps` into `core`, `time_pin` into `time_frozen`, and the three summons refusal tests together.

## Notes

### reactions, contests, puzzles

- Functions compared old vs new line by line (all equivalent in error order, tx boundaries, writes and events): `reactionView`/`waitOf`/`openWindows`, `AnswerReaction`/`answerWindow`/`MarkReactionRolling`, `answerCuttingWords`, `answerHellishRebuke`/`resolveRebuke`, `answerCounterspell`, `useShield`/`UseReaction`, `triggerGone`, `settleOnce`/`releaseWaiting`, `releaseHold`/`replayHeld`/`settleReplay` (the settle step now runs with the plain `ctx`, not the ambient one; only `s.write` reads the ambient, and the settle code never calls it), `cuttingWindows`/`damageWindows`/`hellishWindow`, `publishReactions` (the old code computed a `names` func that nothing used; dropped, no effect), `answerFeatherFall`, opportunity `answerOffer`/`pruneOffers`/`opportunityOffers`/`markProvokes`, riders `answerRider`/`applyRider`, `StartContest`, `RespondContest` (the "not in the contest" check still precedes the status check), `ResolveShove` (an unknown outcome still settles with an empty stored outcome, as before), `CloseContest`, `ReleaseGrapple`, `pruneHolds` (now stops at the first player at 0 hp: one read fewer, no write difference), `checkSources`, `contestView`, `Hide`/`ResolveHide`, `Help`/`ClearHelp`, `groupWrite` and the rest of the group check, `GiveBardicInspirationOutside`/`GetOutsideInspiration`/`dieToCombat`/`dieToCharacters`, `MakePuzzleMove`/`fireWrongTrap`/`runSolveAction`, `TryPuzzleHint`, `CreatePuzzle`/`UpdatePuzzle`/`setArchived`, `changeRun` and the run RPCs, `viewerOf`/`masterExtras`/`masterRunProto`.
- RN-10: reaction window other side (players get only `for_you` windows and the `reaction_wait` line; trigger numbers, ac with shield, aggressor modifier, counterspell slot effects are master-only; `eyesCannotSee` keeps hidden NPCs out of every "sees" test), contests (non-participants get no view, unseen creature ids are blanked, rolls are the roller's and the master's), Hide (`hiddenIDs`/attempts are the hider's player and master only), puzzles (`playerLastMove` strips the move for judging kinds, `masterNumbersInto` is master-only) are all unchanged by the splits. `backend/internal/leaktest` has `reactions_test.go`, `masteronly_test.go`, `stream_test.go` and puzzle cases in `actions_test.go`; they run against the public RPCs and cover this code, but only for the fixtures they build (no hidden NPC with a Hellish Rebuke second step, no contest with an unseen defender): the guard for those is `combat_contests_read_test.go` and `combat_reaction_branches_test.go` only.
- Not checked: CI run time (no test runs allowed), the web side, the files `combat_reaction_kit.go`, `combat_reaction_log.go` beyond the one diff shown, `combat_riders.go` beyond `answerRider`/`applyRider`/`riderConsequence` (the diff output ended there), and `puzzles_kinds.go` (untouched since the base).
- `castSpellsOn`, `otherCastsOn`, `armorOf`: not in my files; the unused linter reports only `armorOf` (`casting_end.go:73`) and `otherCastsOn` (`casting_end.go:84`) for the whole `internal/play`; nothing unused in my files.

### traps, scenes, plumbing

- Not measured: CI time (no test runs), cognitive/cyclomatic of the new helpers (I read them; the largest are `fireInCombat` and `sceneInfo`, both short).
- leaktest coverage of my area: `ListTrapActivity`, `GetOpenScene`, `ListTrapDamages` are in the table of reads (reads_test.go:341-356); `SearchForTraps` with any skill is covered by `trapskills_test.go` ("reads the same whatever the traps nearby") but always with a second die, so play-b-1 is invisible to it; `FireTrap` public after firing is `firedtrap_test.go`; the stage hints go through `stream_test.go:248`. Not covered: the stream events `trap_noticed`/`map_changed` hints per viewer on an unfound trap beyond `stream_test.go:202-218`, and `ImageOnStage` for an NPC taken off the stage (the maps module's `TestMR031_PortraitsAreVisibleOnlyOnStage` covers it).
- Not read in full: `traps_round2_test.go`, `traps_test.go` (used only to confirm overlaps), `mapseam.go` (unchanged since base), `nudge.go` (doc comment change only), `content_hint.go` and `events_choices.go` (unchanged, small, no finding). `combat_log.go` handling of `trap_searched` is outside my files; I did not check whether a player's combat log can show another player's search (docs say the search is "a master-only line").
- `castSpellsOn`, `armorOf`, `otherCastsOn`: the unused linter (`unused`) reports only `armorOf` (casting_end.go:73) and `otherCastsOn` (casting_end.go:84) in `internal/play`; `castSpellsOn` is not reported (so it is used). Not mine to delete.
- `afterTrapRead` (traps_fire.go:526-528) is a test hook inside the production path (`s.afterTrapRead != nil`); pre-existing, left out of the findings.

### casting, lasting, revival, resources

- Not diffed line by line: `lasting_effects_charadd.go`, `lasting_effects_turn.go`, `lasting_effects_prompt.go`, `lasting_effects_add.go`, `casting_reach.go`, `casting_combat.go`, and the unchanged handlers of `casting_end.go` (not touched since 28dd0d5). `lasting_rpc_pin_helpers_test.go` only skimmed.
- RN-10 results: `familiarsight.go` is behaviour-identical and writes ids only; the sight's reach is the maps module's (`link.PartyMember.Eyes`, `characters/partyvision.go:71`), not `play`, so "what the familiar sees must not reveal what the owner could not" is decided there (not reviewed). `leaktest` has no probe for familiar sight, summons or exhaustion (`classify_test.go` only lists the procedures). Revival: `revivifyVerdict` and `listRevivifyTargets` keep HIDDEN/NOT_SEEN for the master only and give a player `errRevivifyTarget` for every failure; the split preserved that, and `TestRevivifyFrozen_FogHidesAnNPCInTheDarkFromThePlayer` guards NOT_SEEN. Lasting effects per viewer: `readsEffect`, `conditionHidden`, `cardSaves` (DC only for the caster's own player) unchanged by the split.
- Not a regression, noted: `summonHasRoom` (`creature_cast.go:80-95`) and `joinCreatures` refuse with `TOO_MANY_COMBATANTS` counting hidden combatants, so a player who sees 36 of 40 and is refused learns that 4 more exist. The cap is a rule; left for a decision.
- `gocognit -top 12` over my files: no function above 15 except the two in play-b-16; ten sit exactly at 15 (`giveEffectBack`, `effectDamage`, `endCombatSights`, `afterCastChange`, `reachOfTargets`, `resolveEffectSave`, `endLongRestEffects`, `effectDamageSaves`, `effectsToCombat`, `effectSavePrompt`), so any one more branch breaks the lint.
- Not run: no tests, no DB, as instructed. play-b-2's cause is read from the migrations (`00110` constraint, `00236` column only) and from the golden; I did not execute a migration.
- Duplication across areas I met but did not own: `combat_concentration.go:266-440` (concentration save) and `lasting_effects_save.go:276-577` (effect save) are two designs for one flow (play-b-24); `inspiration_outside.go:189` (`requireNoActiveCombat`) and `resources.go:144` (`activeCombatBlocks`) are the same query with different messages.
