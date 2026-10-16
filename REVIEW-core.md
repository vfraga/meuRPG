# REVIEW-core — post-refactor review of rules, characters and the smaller backend packages

Area `core`: `backend/internal/rules/**`, `characters`, `progression`, `campaigns`, `campaignpackage`, `notes`, `identity`, `platform/**`, `backend/cmd/**`.
Range reviewed: `1daf3b5..35346c9` (every refactor commit that touches these paths: #417 to #452). Read-only; nothing was run except `go build`, `go vet` and `gocyclo`.
Not reviewed: `internal/play`, `internal/maps`, `web/`.
The review was done by reading both versions of every split function (five parallel readers, then I spot-checked the claims below against the code).

## Verdict

- **No behaviour regression and no RN-10 leak found.** Compared old and new text of the split functions in all scoped packages: check/validation order, `db.InTx` boundaries, writes, events, notices, log lines, idempotency keys, RNG draw order in the dungeon generator. A scripted pass over every `characters` function taking a `tx` found no `s.queries.X` without `WithTx`. Three items are not strictly neutral, all low: `core-24` (a dead pre-read; removing it changes one error label), `core-25` (which error is reported when a content file has several faults: unsorted map order, pre-existing) and `core-26` (a config message suppressed by an unrelated error, frozen by the refactor).
- **The gate is met** (`gocyclo` top score in the area is 15; `go build` and `go vet` pass).
- **The problems are maintenance ones:** parallel sessions wrote the same helper under different names, many splits exist only to meet the limit (closure tables, one-caller wrappers, 7-8 parameter helpers), the transaction-body helpers have three naming conventions, and the test suite adds 66,655 lines (42k of goldens) of which about a third can go.
- Counts: 0 high, 16 medium, 37 low. By category: behaviour 3, readability 28, duplication 10, tests 8, docs 4.

Severity key: high = likely bug or leak; medium = hurts maintenance; low = polish. "Fix changes behaviour" says so only when it does.

## Findings, by severity

### core-1 · tests · medium · `backend/cmd/srdimport/converters_characterization_test.go:33`
`TestSrdImportReproducesTheSnapshot`, the only byte-for-byte check that the committed SRD snapshot (`rules/srd51/data`) equals what the importer produces, skips unless `SRDIMPORT_SRC` is set. Nothing in `.github`, the Makefile or the docs sets it, so it never runs and the snapshot is unchecked in CI.
Fix: run it in a manual/scheduled workflow, or vendor the few source rows under `testdata`; otherwise delete the test and the claim. Drop "characterization" from the name. Fix changes behaviour: no (CI only).

### core-2 · tests · medium · `backend/internal/characters/testdata/{plan_rest,apply_vitals_change,table_content_viewers,levelup_options_*,spells_*}.golden.json`
About 17,000 golden lines that nobody can review. `plan_rest` (2,674 lines) and `apply_vitals_change` (2,191) hold 369 unique lines: every case dumps the whole "before" vitals proto. `table_content_viewers` (5,332) has 422 unique lines. `levelup_options_pensantus_owner` equals `_master` and so does `pensantus_fighter_owner`; `spells_pending_member` equals `spells_stranger` (byte for byte).
Fix: vitals goldens become an inline table of the few fields each case changes (about 150 lines of Go); table-content viewers record only the keys that differ per viewer plus the switch answers (the master-versus-player differences are the RN-10/RN-23 behaviour: keep those); one golden per case and the viewer difference asserted inline. Saves about 12,000 lines. Fix changes behaviour: no.

### core-3 · tests · medium · `backend/internal/rules/testdata/golden/levelup-options-digests.json:1`, `creature-reads.json:1`, `rules/dungeon/generator_characterization_test.go:1` + `testdata/generator_golden.json`
`levelup-options-digests` (439) and `creature-reads` (683) are lists of 16-hex digests: a failure cannot be explained and any SRD edit regenerates them unread; `levelup_sweep_test.go` already proves options, check and validate agree at every class/subclass/level. The dungeon generator golden hashes private per-stage state (`g.kind`, `g.rid`, `g.blocked`) over 2,000 combinations, 7.8 s (the slowest characterization test), pins the phase structure, and sits beside `dungeon_test.go`'s documented golden that already carries `Version` and the bump-Version message.
Fix: delete the digest goldens with their generators (about 130 lines), delete the stage hashing and `generator_golden.json` (keep the readable branch cases). Fix changes behaviour: no.

### core-4 · tests · medium · `characters/claimsreview_frozen_test.go`, `campaigns/characterization_faults_test.go`, `campaignpackage/characterization_faults_test.go`, `progression/award_characterization_test.go`
These assert the server's log lines byte for byte, wrapped error causes and the call order of collaborators. A log wording fix or an added error context breaks them, and many permission/refusal cases repeat `claims_test.go`, `review_test.go`, `approval_test.go`, `campaigns_test.go`, `gold_test.go`, `xpmode_test.go`, `serializationretry_test.go`.
Fix: keep Connect code and message, who may call, "nothing written on failure", undo, same-key retry, failure of another module; drop log text and publish-count pins (keep one test that a failure is logged). About 900 lines. Fix changes behaviour: no.

### core-5 · tests · medium · `progression/award_rig_test.go:1`, `history_rig_test.go:1`; query-fault tracers; golden helpers
Two ~485-line rigs are 90% the same (`awardFault*`/`historyFault*`, two log buffers, label/redact/takeLogs/wantLines). The pgx "named query gets a cancelled context" tracer is copied 4 times (`campaignsQueryFaults`, `pkgQueryFaults`, `notesQueryFaults`, `historyFaults.TraceQueryStart`); `dbtest` already owns `acquireWatch`. There are 10 `-update-*` flags and 6+ "marshal, mask times and ids, compare, rewrite" helpers (`levelUpGolden`, `choicesGolden`, `checkCombatReadsGolden`, `vitalsGoldenJSON`, `tableContentGolden`, `wantHistoryGolden`, `goldenText`); `docs/testing.md` documents only the rules `-update`.
Fix: one `faultRig` in a shared test file, `dbtest.QueryFaults`, one `internal/platform/goldentest` helper with a single `-update`, and three lines in `docs/testing.md`. About 650 lines. Fix changes behaviour: no.

### core-6 · duplication · medium · `characters/derived.go:58,129,311`, `levelup.go:1016,1044,1052,1184,1193`, `vitals.go:187`, `vitals_hitdice.go:100`, `rests.go`
The same rules-to-proto conversions exist in 2-4 copies: `PactMagic` (inline in `derived.go:58` and `pactMagicToProto` at `levelup.go:1044`); `HitDice{Faces,Count}` (`derived.go:129`, `levelup.go:1193`, `vitals.go:187`, `vitals_hitdice.go:100`, plus `hitDiceProto` in `rests.go`); `[]int` to `[]int32` (`int32s` `derived.go:311` equals `mapToProto(in, i32)` at `derived.go:39`, and `slotCountsToProto` `levelup.go:1052`). `mapToProto` is used only in `derived.go`; `levelup.go` and `vitals.go` keep hand-written append loops.
Fix: keep one `hitDiceToProto`, one `pactMagicToProto`; `int32s` calls `mapToProto`; use `mapToProto` in `levelup.go`. `mapToProto` keeps the nil-for-empty rule. Fix changes behaviour: no. Guarded by the derived and level-up goldens.

### core-7 · readability · medium · `characters/{createcharacter,updatecharacter,levelup,levelup_plan,rpc,tablecontent_rpc,abilityscores,revive,charactercreatures,choices,approval,claims,review}.go`, `progression/{award,undo}.go`, `campaigns/{rpc,invites,tablerules,document}.go`, `notes/rpc.go`, `campaignpackage/rpc_{export,import}.go`
Three suffixes (and no suffix) for "the body of the transaction of RPC X": `insertCharacterIn` (`createcharacter.go:189`), `saveSheetIn` (`updatecharacter.go:119`), `levelUpIn` (`levelup.go:699`), `rollLevelUpIn` (`:567`), `planLevelUpIn` (`levelup_plan.go:140`), `saveStoryIn` (`rpc.go:216`), `setArchivedIn` (`tablecontent_rpc.go:803`), `rollAbilityScoresIn` (`abilityscores.go:310`); `reviveInTx` (`revive.go:94`), `giveCreatureInTx` (`charactercreatures.go:352`), `completeChoicesInTx` (`choices.go:363`), `giveInTx` (`progression/award.go:376`), `undoInTx` (`undo.go:87`); `approveTx` (`approval.go:70`), `claimTx` (`claims.go:590`), `requestChangesTx` (`review.go:196`), `createCampaignTx` (`campaigns/rpc.go:105`); verb phrases (`changeNote`, `startExport`, `beginImport`, `writeDocument`, `changeXPMode`). Worst case: `revive.go:94 reviveInTx` (the whole transaction) beside `revive.go:129 reviveTx` (only the row); `checkSheetSaveIn` and `planLevelUpIn` take a `tx` but are not the transaction.
Fix: one convention, `<rpc>Tx` = "runs in the caller's transaction". Rename `reviveInTx` to `reviveCharacterTx`, `reviveTx` to `revivedRow`; drop `In` from `checkSheetSaveIn`, `planLevelUpIn`, `rollAbilityScoresIn`; `giveInTx` to `giveTx`, `undoInTx` to `undoTx`. Mechanical. Fix changes behaviour: no.

### core-8 · duplication · medium · `rules/spellcasting.go:477-483`, `choicespells.go:31-33,92-98`, `levelup.go:502-516,602-604`
Magical Secrets detection is written three times (`countSecrets`, `magicalSecretsOf`, `addFeatureNotes`) with two constants for the value 2: `secretsPerFeature` (existing) and `magicalSecretsSpells` (added by the split).
Fix: delete `magicalSecretsSpells`; `countSecrets` and `addFeatureNotes` call `magicalSecretsOf` (the extra `!isTableKey` test for the "additional" key cannot matter). Fix changes behaviour: no. Guarded by `spellcasting_characterization_test.go`, `levelup_characterization_test.go`.

### core-9 · readability · medium · `rules/overlay.go:791-815,851,895,941`
The seven overlay entry kinds (list name, key prefix, order, accessor) are described in three parallel closure tables (`overlayOrder`/`orderOf`, the 7-row `entryStep` table in `addEntries`, the 7-row `claimStep` table in `claimKeys`) plus the entry-count sum in `checkOverlay`. Adding a kind touches four places, and the orders differ on purpose (`addEntries` starts with spells, `claimKeys` with classes), which is easy to break.
Fix: one `[]overlayKind{list, prefix, entry(i), add, learn}` iterated by both passes, with the claim order as a field. Fix changes behaviour: no, if both orders are kept. Guarded by `overlay_build_characterization_test.go`, `overlay_entries_characterization_test.go`.

### core-10 · readability · medium · `rules/content.go:423-470`
`indexDataFiles` wraps 15 identical one-line `index(...)` calls in 15 `func(fsys fs.FS) error { return index(...) }` closures only to fit `[]contentStep` (about 45 lines for what was 15).
Fix: a generic `indexStep[T any](c, file, key, name func(*T) string, into map[string]*T) contentStep`, or `Key()`/`Name()` on the srd51 types. Fix changes behaviour: no.

### core-11 · duplication · medium · `rules/combat/turn.go:503-518` vs `combat/outside.go:84-101`
`castingSlots` (added by the split) is the same loop as `slotChoices`, which already existed; `slotChoices` only adds a `level < 1` guard that `spellOption` already applies (`sp.Level > 0`).
Fix: call `slotChoices`; delete `castingSlots` and `highestSlotLevel`. Fix changes behaviour: no. Guarded by the combat `testdata` sweeps.

### core-12 · duplication · medium · `combat/turn.go:501`, `rules/summon.go:153`, `rules/creatures.go:644`, `characters/vitals.go:42`
The highest spell slot level (9) now has four constants (`highestSlotLevel`, `maxSummonCircle`, `maxSpellSlotLevel`, `maxSpellLevel`); the literal 9 remains in `combat/outside.go:91`, `combat/usage.go:39` and the `[9]int` arrays.
Fix: one exported `rules.MaxSlotLevel`. Fix changes behaviour: no.

### core-13 · duplication · medium · `rules/effects.go:199-201` vs `rules/tablemenu.go:255-258`
The extra_attack bounds (2 and 4) are defined twice (`minExtraAttacks`/`maxExtraAttacks` and `menuExtraAttackMin`/`menuExtraAttackMax`). `EffectMenu` reports bounds the compiler enforces, so they can drift.
Fix: delete the `menu*` pair. Fix changes behaviour: no.

### core-14 · readability · medium · `rules/armor.go:94,115-127`
`isBaseEffect(a, &unarmored)` reads as a pure predicate but writes `*unarmored` the first time an Unarmored Defense feature appears. A caller that reorders the condition `!isBaseEffect(...) || !x.applies(a)` changes which feature wins.
Fix: inline the rule in `effectBases` or rename it (`takesBase`) and document the write. Fix changes behaviour: no.

### core-15 · readability · medium · `progression/award.go:351-480`
`awardPlan` holds a copy of the grant (`g`) and a separate `reason`; `treasureGold` takes `*awardPlan` and rewrites `p.g.amount`; `insertAward` reads `p.reason` while `awardEvent` reads `g.*`. Which `reason` and `amount` is current depends on the stage; `g.amount` after `planAward` is 0 for treasures.
Fix: `planAward` returns the final grant (`p.g.reason = checked reason`), drop `awardPlan.reason`; or `treasureGold` returns `(treasures, gold)` and `planAward` assigns them. Fix changes behaviour: no.

### core-16 · duplication · medium · cross-module helpers (`characters/errors.go:87,98`, `campaigns/invites.go:311`, `notes/rpc.go:54`, `progression/errors.go:58`, `identity/rpc.go:273`, `characters/access.go:183`, `progression/read.go:423`, `characters/derived.go:471`)
Identical bodies under several names: `parseUUID` (characters, campaigns) = `parseID` (notes, maps `mapservice.go:942`); `isUniqueViolation` (progression, characters); `deref(*string)` (characters, progression, play); `clampInt32` in `identity/rpc.go:273` (upper bound only) and `maps/gallery.go:273` (both bounds), plus `clamp32`/`clampInt32` in play and `i32` in `characters/derived.go:471`; pointer helpers (`derefInt`, `derefBool`, `deref32`, `derefInt32`, `valueOf`, `ptr`) in characters and play.
Fix: keep one of each in `internal/platform` (`db.IsUniqueViolation(err, constraint)` next to `InTx`; a small ids/ptr package; the two-sided clamp) and delete the copies module by module (play and maps are other reviewers' packages: coordinate). Fix changes behaviour: none observable (counts are never negative).

### core-17 · readability · low · `rules/content.go:250-266`, `combateffects.go:268-275,406-421`, `corrections.go:100-115`, `validate.go:15-22`
Four versions of "run closures until the first error" with four signatures. `noFiles` exists only to adapt `checkMonsters`, `checkMagicItems`, `indexCasting`; `checkEffectNames` and `checkEffectDefs` take an unused `_ *combatEffectsFile`.
Fix: one `firstError(fns ...func() error) error`, or call the five fixed steps directly; drop the ignored parameters. Fix changes behaviour: no.

### core-18 · readability · low · `rules/effects.go:161,238`
`effectTypes` and `effectCheckers` must name the same 18 effect types; `effectTypes` has no other user, so a type added only there silently gets no checks. No test compares them.
Fix: drop `effectTypes`, use the map for the unknown-type test. Fix changes behaviour: no.

### core-19 · readability · low · `rules/derive.go:406,431,437,444`
`collector{x, owned}` exists only to hold helper methods (each starts with `x := col.x`); `raceTraits` and `subraceTraits` are 3-line one-caller wrappers that add a nil check.
Fix: methods on `(x *deriver)`, inline the nil checks in `collectEffects`. Fix changes behaviour: no.

### core-20 · readability · low · `rules/traps.go:275,421,424,437,465-507`
One-field `trapLoader{c}` whose methods mostly ignore `c`; `trapHead` to `trapNumbers` is one switch split in two (one caller); `trapAt` is a string concatenation; four methods without doc comments.
Fix: merge `trapNumbers` into `trapHead`; make the methods that do not need `c` free functions. Fix changes behaviour: no.

### core-21 · readability · low · `rules/levelup.go:467,1170,1486,1515,1535,1539,1664,1686,1696`
`bareLevelUp` recomputes `newClass` (`idx >= len(b.Classes)`) that its caller (about line 404) already has; `checkNewClass` returns four values and `tools` is used once (line 1123); `"full.extra_ability_bonuses"` is written six times (local `const field` four times, literal twice); `checkRaisedTo20` compares with `MaxNormalScore`, not 20.
Fix: pass `newClass` in, return `(ext, newClass, lerr)`, one package const, rename `checkRaisedPastMax`. Fix changes behaviour: no.

### core-22 · duplication · low · `rules/choicegroups_pools.go:48-70` vs `rules/choices.go:382-410`
Two implementations of "merge pools that share an option" (`groupPools`/`pool.absorb`/`pool.add` and `joinPool` on `optionPool`) over `options map[string]bool`; the next fix has to be made twice.
Fix: one small `optionSet` type embedded in both. Fix changes behaviour: no. Guarded by `choicegroups_pools_characterization_test.go`, `choices_characterization_test.go`.

### core-23 · readability · low · `rules/combateffects.go:469` vs `rules/spelleffects.go:331`
`checkEffectConditions` is a function in one file and a method on `*content` in the other, checking different things. `spelleffects.go:232 loadApartKind` returns `(done bool, err error)` where a `switch` on the kind in `loadSpellEffect` would do.
Fix: rename the spell one `checkSpellEffectConditions` (or inline, one caller); replace `loadApartKind` by the switch. Fix changes behaviour: no.

### core-24 · behaviour · low · `characters/choices.go:301-305`
`CompleteCharacterChoices` calls `s.contentFor(ctx, nil, m.CampaignID)` and discards the value (`_, err :=`). It was already a dead fallback; the new comment ("answers before any row is locked") claims a guarantee no test checks, and it costs one pool read per call.
Fix: delete the read and the comment; the transaction already reads the content (`completeChoicesInTx`, about line 375) and wraps `read rules content`. Fix changes behaviour: only the error path (an unreadable content fails inside the transaction, so the log label changes). No test guards the order.

### core-25 · behaviour · low · `rules/combateffects.go:285,297,311,331`, `corrections.go:225,369`, `validate.go` (`range b.ExtraAbilityBonuses`)
Unsorted `range` over maps decides which error a content file reports when it has several faults; the refactor added the sorted `namesWithPrefix` for four loaders and left these (`spelleffects.go` already uses `sortedKeys`).
Fix: iterate `sortedKeys(...)`. Fix changes behaviour: only which of several errors is reported, never the loaded content. No test depends on it (fixtures have one fault).

### core-26 · behaviour · low · `platform/config/config.go:683-685`
`len(l.CampaignCreators) == 0 && len(errs) == 0` uses `errs`, which also holds `RATE_LIMIT_MULTIPLIER` and `MAX_CAMPAIGNS_PER_USER` errors, so `CAMPAIGN_CREATORS=","` plus a bad multiplier reports only the multiplier. `Load`'s doc promises all problems together. The refactor froze the quirk (`TestLoadCharacterization`).
Fix: test `len(entryErrs) == 0`. Fix changes behaviour: yes, one more line in a failing `Load`; `testdata/golden/load.json` changes.

### core-27 · readability · low · `rules/*` (several): one-caller wrappers added only for the split
`spelleffects.go:265 loadSummonEntry`, `spelltarget.go:142 checkCreaturesOverride`, `:149 checkAreaOverride`, `overlay_spell.go:310 checkSelfTarget`, `metamagic.go:138-210` (five one-`if` helpers; keep only `metamagicTwinnedReason`), `tablemenu.go:280 menuAbilityName` (3 uses: keep), `puzzle/pillars.go:162 states()` and `:150 checkSearch`, `validate.go:27 invalid`, `lights.go:67 checkLight` (7 loose fields because the entry type is anonymous: name it `lightJSON`), `summon.go:159 summonFail` declared but `loadSummon` (162) spells the func type out, `treasure.go:221,260,296` alias `const name = treasureFileName`.
Fix: inline the single-caller ones; name the light entry type. Fix changes behaviour: no.

### core-28 · readability · low · `rules/overlay_features.go:466-536`
`effectFields` (32 `{name,set,clear}` rows) is re-indexed into `effectFieldClear` and `clearEffectField`; a third list of the same fields is in `overlay_entries_characterization_test.go:435`.
Fix: `stripUnused` loops `effectFields` (`if f.set(e) && !slices.Contains(own, f.name) { f.clear(e) }`); delete the map and `clearEffectField`. Fix changes behaviour: no (clearing is idempotent, fields independent).

### core-29 · duplication · low · `rules/overlay_features.go:130` vs `rules/derive.go:195`
`isFeatureChoice` exists; `offeringEntry` repeats `e.Type != "choice" || e.Choice != "feature"`.
Fix: use `isFeatureChoice` in `derive.go`; move it next to `Effect`. Fix changes behaviour: no.

### core-30 · readability · low · constants named for the linter, numbers repeated in the message
`combateffects.go:558-565,616,637` (`maxBaseAC` "1 to 60", `maxSpeedPercent` "0 to 400"), `effects.go:498` ("2 to 4"), `overlay_spell.go:50-58`, `spelltarget.go:114,144`, `treasure.go:265`, `characters/spelllist.go:82-85,110` (`lowestSpellCircle`/`highestSpellCircle` unused by the text "must be 0 to 9"), `platform/config/config.go:321-331` (`minPort`/`maxPort` vs "between 1 and 65535"). `treasure_generate.go:138` uses `maxChancePct` as the die size of the roll. Pure-literal names add noise: `overlay_class.go` `classSavingThrows = 2`, `secondSlotLevel = 2`, `levelup_multiclass.go:31`, `metamagic.go:239`, `tabledefaults.go:74-88` (`thirdCantrips+1` stands for 3), `encounter/generate.go:163-167` (while `:118-120` keeps raw `*8 *10 *6`), `puzzle/pillars.go:259-266`, `dungeon/doors.go:16-22,182-191` (`sideCount`, `maxSideThresholds`; `intn(4)` and `d < 4` at `deadends.go:62` stay literal).
Fix: `traps.go` already formats the constants into the text: do the same wherever a constant is a real limit; call the roll `percentileDie`; keep the plain literal (with a comment) where a number never varies. Fix changes behaviour: no.

### core-31 · readability · low · `rules/overlay_class.go`, `rules/levelup.go` (`checkSavingThrows`, `checkSubclassLevel`, `checkCasting`, `checkSpells`, `checkSubclass`)
`check*` functions that also return the value they computed; the name says "validate" and the result is easy to drop.
Fix: rename (`savingThrowsOf`, `subclassLevelOf`) or return `(value, error)`. Fix changes behaviour: no.

### core-32 · duplication · low · `rules/creatures.go:607` vs `rules/wildshape.go:172-174`; `grid/area.go:77,84` vs `dungeon/stairs.go:47`, `combat/contest.go:77`; `content.go:480`, `spelltarget.go:78`, `overlay_features.go:742`
`passiveBase = 10` was added in `creatures.go` but `wildshape.go` still writes `10 +`. `abs` is defined in `grid` and `dungeon`, `sign` in `grid` and `combat`. The 5 ft step has three constants (`spellAreaStepFt`, `targetSizeStepFt`, `feetStep`) and the area-shape list three copies (`targetShapesOfData` in `content.go:508` added by the split, `targetShapes` in `spelltarget.go:83`, an inline literal in `overlay_spell.go:336`).
Fix: keep `targetShapes` and `feetStep`, delete `targetShapesOfData` and `spellAreaStepFt`; use `passiveBase` in `wildshape.go`; leave `abs`/`sign` alone or share through `grid` if `dungeon` may import it. Fix changes behaviour: no.

### core-33 · readability · low · `rules/dungeon/connect.go:60,157,174,239`, `dungeon/finish.go:244-285`
In `connect.go` the method `linker.link` (174) shares its name with the type `link` (60), `pass(queue, pass)` (157) shadows its own method, and `tryLink(l link)` uses `l` where the receiver is also `l`. In `finish.go` the corridor scan is a three-deep chain and `visitCorridorNeighbor` takes 6 parameters.
Fix: rename the method and parameters; make the chain methods of `corridorScan` or merge the last two. Fix changes behaviour: no. Guarded by the dungeon goldens (RNG order is unchanged).

### core-34 · readability · low · `rules/grid/move.go:252,366-390`
`MoveUntil` copies every input into a 14-field `untilWalk` struct; `walkStep` takes 6 parameters and mutates `*Move`.
Fix: keep the loop in `MoveUntil`, split out only `backUp` and `result`. Fix changes behaviour: no. Guarded by `move_sweep`, `openarea_sweep`.

### core-35 · readability · low · loaders report problems in two styles
Some loaders return a `string` message (`magicitems`, `creatures`, `resistances`, `spelltarget`), others an `error` through a `fail` method (`traps`, `effects`, `spelleffects`, `summon`).
Fix: pick the `error`-returning style in new splits; migrate when a file is touched. Fix changes behaviour: no.

### core-36 · readability · low · `rules/encounter/generate.go`, `cmd/api/main.go:300,311,360`
The `encounter` split added six top-level names (`limits`, `candidates`, `leadersOf`, `groupOf`, `tally`, `byFallingXP`) for one function; fold `limits` and `candidates` back into `Generate`. In `cmd/api`, `signInParts` (4 fields, one use) and `wireSignIn` returns two values that are nil together, and `mountServices` takes both.
Fix: put the identity service in `modules`. Fix changes behaviour: no.

### core-37 · readability · low · `characters/levelup_plan.go:140,189,232,261`
The plan helpers take `tx` and `q` (always `s.queries.WithTx(tx)`), up to 8 parameters; `typedHitPoints` takes `tx` only to call `diceRule`.
Fix: pass `tx` only, build `q` in `inAppHitPoints`, read `diceRule` once in the two `case`s of `planHitPoints` (the average method reads nothing). Fix changes behaviour: no.

### core-38 · readability · low · `characters/charactercreatures_roster.go:122,136`; `npcfromcreature.go:90`; `tablecontent_rpc.go:120,512-535`; `claims.go:643`
`if out = append(out, replaced...); err != nil` mixes an append with the error test of the previous line. `encodeNpcFromCreature` returns `(string, error)` where the string is a log label. `checkUpdateRequest` returns `nil, KIND_UNSPECIFIED, err` four times and `req.Msg.GetKey()` is read twice; `res := &ListTableEntriesResponse{}` is dead (overwritten). `claimReadError` is used by `spendClaim` for a write.
Fix: split the append from the `if`; return `wrap("encode a sheet", err)` from the helper (log label text changes); return a `{key, body, kind}` struct; `var res`; rename `claimStepError`. Fix changes behaviour: no, except the log label.

### core-39 · readability · low · `characters/claims.go:205,390`, `access.go`
`characterAfterWrite` and `masterAndCharacter` live in `claims.go` but are used by `approval.go:61`, `review.go:157,277`; seven sites repeat the inline form (`rpc.go:96,289,342`, `revive.go:77`, `spelllist.go:195`, `wildshape.go:97`, `charactercreatures.go:231`).
Fix: move both to `access.go`, and either use them everywhere or drop the wrapper. Fix changes behaviour: no.

### core-40 · readability · low · `characters/levelup.go:913,936,980`, `levelup_plan.go`
`levelup.go` is still 1,202 lines: `levelUpChoicesFromProto`, `applyLevelUp`, `abilityMapsEqual` are called only from `levelup_plan.go`.
Fix: move them there, and the `*ToProto` mappers with `hitPointsMethodAllowed`/`hitPointsRuleToProto` to `levelup_proto.go`. Fix changes behaviour: no.

### core-41 · readability · low · five loops over a campaign's sheets (`characters/tablecontent_rpc.go:94,187`, `tableswitches.go:49`, `tablechoice.go:252,699`)
`characterUses` (all full sheets, `rules.TableKeys`), `sheetUses`, `playerUses` (players only, `rules.BuildKeys`), `callerKeys`, `affectedSheets` each re-run the "load sheet, skip if `GetFull() == nil`" loop; nothing in the names says one counts NPCs and another does not.
Fix: one `forEachFullSheet(sheets, fn)` and clearer names; do not merge `TableKeys` and `BuildKeys` (a real difference). Fix changes behaviour: no.

### core-42 · duplication · low · `characters/createcharacter.go:131-146`, `charactercreatures.go:226-238,335-346`, `levelup.go:556-561,871`, `npcfromcreature.go:83`, `listcharacters.go:44-49`, `summonoptions.go:91-107`
The "idempotency key must be a UUID" check is written four times (`newCharacterKey`, `cleanCreatureCreateKey`, `checkRollRequest`, `npcfromcreature`); the "player user ids of the rows" loop twice (`listcharacters.go`, `levelUpPlayerIDs`); `checkSummonOptionsCharacter` repeats the access block of `ListCharacterCreatures` (whose comment says it IS that access).
Fix: `uuidKey(raw, required)` in `errors.go` beside `parseUUID`; `ListCharacterCreatures` calls the summon check (`requirePlayerCharacterVisible`). Fix changes behaviour: only a log label.

### core-43 · readability · low · `characters/tablechoice.go:70-87`
`CheckSheetCanMove` is exported with no production caller (only `tablecontent_test.go:1068-1076`); its comment says "Nothing calls it yet".
Fix: delete it and its test, or schedule it and drop "yet". Fix changes behaviour: no.

### core-44 · readability · low · `progression/award.go:146-195,513-550`, `progression/milestones.go:488-531`
`awardGrant` dispatches through a package-level `map[mode]func` of three entries (each with one caller) to shorten a switch, and the rule "treasure_point_ids empty unless GOLD" is in `awardGrant` while the GOLD rules are in `goldGrant`. `share` is called only by `insertShares` (7 parameters); `convertTreasures` mixes three jobs. `milestoneViews` is split into `readMilestones`, `milestoneSnapshot`, `groupMarks`, `markGroups` (a 3-field struct returned to its only caller) for about 40 lines.
Fix: `switch msg.GetMode()`; merge `share` into `insertShares`; fold `groupMarks` into `milestoneViews`, keep `readMilestones` (the ReadTx boundary). Fix changes behaviour: no.

### core-45 · readability · low · `campaigns/rpc.go`, `notes/rpc.go`, `campaignpackage/rpc_export.go`, `rpc_import.go`, `progression/milestones.go`, `campaigns/document.go`
`readNewCampaign`, `readNewNote`, `readNoteChange`, `readExportStart`, `readImportStart` validate and read nothing from storage (the name suggests I/O); `documentSave` (5 fields) and `plannedInsert` are built at the single call site from locals already in scope.
Fix: rename to `checkNew…`; drop the two structs and pass 4-5 parameters. Fix changes behaviour: no.

### core-46 · readability · low · `identity/login.go:368,529-546`; `platform/config/config.go:248-252,306-319`
`completeLogin` keeps a lone `// 2. The state must exist` after steps 1, 3, 4 and 5 moved to helpers. `safeReturnTo`: `isSitePath` rejects `/\host` and `isPlainPath` rejects every backslash (the first test is redundant); `asciiSpace`, `asciiDelete` exist only for a linter. `config.Load` sets `Port`, `LogLevel`, `WebDir` defaults in the struct literal and again in `loadPort`, `loadLogLevel`, `orDefault`; `checkCloudRunDatabase` and `checkCloudRunCampaignCap` are one-caller wrappers.
Fix: drop the step numbers; leave the backslash to `isPlainPath`; remove the three defaults from the literal; one `checkCloudRun(cfg) []error`. Fix changes behaviour: no. Guarded by `login_test.go` and `load.json`.

### core-47 · tests · low · `platform/dbtest/template_characterization_test.go` (`TestGooseIDsOfACopiedDatabase`)
The test asserts a known goose sequence mismatch ("freezes a finding, not fixed") and fails when someone fixes it; the bug stays in `dbtest.go` (`setval` moves the wrong sequence).
Fix: `setval` the sequence `pg_get_serial_sequence('goose_db_version','id')` returns and flip the test, or open an issue and cite it. Fix changes behaviour: test database setup only.

### core-48 · tests · low · names and small overlaps
208 test functions are named `*Frozen*`, `*Characteri[sz]e*`, `*Freeze*` and about 45 files say "frozen before the split": they describe the history, not the guarantee (CONTRIBUTING: no history in comments). `identity/login_freeze_test.go:31` repeats `login_test.go:440-490`; `puzzle/characterization_test.go:57` hashes the minimum for every board beside `TestLightsMinimumMatchesBruteForce` (12.7 s oracle); `cmd/api/run_characterization_test.go:200` pins every startup line of 10 sequential boots (`run.json`, 132 lines); `rules/testdata/golden/scene-options.json` (1,203 lines) belongs to no level-up test; `platform/config/testdata/golden/load.json` (1,890) dumps the full `Config` per success case and says only "Load drifted".
Fix: rename in the same PR that merges (git keeps the rename), reword headers to the guarantee (RN/MR id, who sees what); delete the login freeze and the lights hash; assert the error and the "X is disabled" warnings in `run`; split `scene-options` into its own test with 3-4 cases; record only the differing fields in `load.json` and compare per case. Fix changes behaviour: no.

### core-49 · tests · low · gaps no test guards (nothing here is a known bug)
No concurrency test for `AdjustVitals`/`TakeRest` (single caller only; `dbtest.NewBarrier` is used by progression only). Unverified by me, to check before trusting: the "request raced with its own retry" branch of `progression/award.go:378-384` (`repeated: true`: if the flag were lost a raced retry would republish `map_changed` and log again); a `levelup_options_*` golden with a feat whose race prerequisite is hidden (`hideFeatRace`, `characters/levelup.go` about 345); a focused "a player's `ListTableEntries` has no switched-off key" assertion; a `cmd/migrate` test that a failed migration releases the lock; a `notes` test that `UpdateNote` refuses a non-author (only the `maps` package tests RN-10 for notes); `effectTypes` against `effectCheckers` (`core-18`); `dungeon.ASCII`/`PromptOf` (`floodNetwork`, `asciiOverlay`) and `cmd/migrate down`.
Fix: add focused tests for the first two before any related refactor. Fix changes behaviour: no.

### core-50 · docs · low · wrong pointers in comments
`progression/progression.go:240-241` (same text in `play/play.go:237,671`, `maps/maps.go:348`) cites `docs/architecture/README.md, "Dentro de uma transação, nenhuma leitura pelo pool"`, a sentence that exists in no doc; the rule is in `docs/architecture/transactions.md#transactions-and-the-connection-pool` (as `campaigns.go:329` already says). `platform/httpserver/server.go:21-23` says streams last up to 30 minutes so the service needs 35; `docs/operations/README.md:58` and `reference.md:103,138` say streams last 10 minutes and the 30 minutes is the campaign package download. `platform/ratelimit/clientkey.go:45` and `campaignpackage/sweep.go:18` cite `docs/operations/README.md`; the content is in `reference.md#the-client-ip` and `reference.md#campaign-package`.
Fix: repoint the four comments and reword `server.go`. Fix changes behaviour: no.

### core-51 · docs · low · audit, stage and slice ids in comments (56 matches of `Etapa|slice N.N|E4-1|question N` in the area; 13 added by the refactor)
`rules`: `api.go:102`, `creatures.go:15`, `effects.go:190-191`, `encounters.go:13` ("Etapa 10, slice 10.9c"), `lights.go:10`, `spelleffects.go:10`, `summon.go:11`, `traps.go:10`, `treasure.go:13` ("slice 10.10b"), `wildshape.go:10`, `tabledefaults.go:3`, `tablemenu.go:9`, `combat/hpspells.go:7`, `combat/jump.go:5`, `encounter/doc.go:1`, `vision/vision.go:1`, `grid/layer.go:172`, `grid/doc.go:1`, `dungeon/doc.go:1,16` ("the audit that preceded it"); `characters`: `charactercreatures.go:27`, `charactercreatures_roster.go:25`, `combatspells.go:17`, `wildshape.go:21`, `legacy_attack.go:11-12` ("Before Etapa 6 … Now it has"), `tablechoice.go:122,165,197` ("E10-02 state 8", "question 80"), `tablecontent_rpc.go:31,667`, `levelup.go:30-31` ("Until the full level-up exists"); `notes/notes.go:1,46` ("Etapa 8, D6"), `platform/dbtest/dbtest.go:12` ("Etapa 6 brought 46 migrations", a count that is stale), `platform/db/tx.go:60` ("audit D-03"); time words: "(none today)" in `tablemenu.go`, "today, accepting a campaign invite" in `cmd/api/main.go`.
Fix: keep the `MR-`/`RN-` ids; drop "Etapa N", slice and audit ids; rewrite the history sentences in the present tense. Fix changes behaviour: no.

### core-52 · docs · low · comments that name identifiers that no longer exist (found by script over all comment words; checked against the code)
`rules/choicegroups.go:470` (`effectCount`; the function is `effectChoices`), `choicegroups_sources.go:182` (`ordinalEnemy`, `ordinalTerrain`; the vars are `enemyTitles`, `terrainTitles`), `creaturelanguages.go:10` (`languagesPT`), `creatures.go:43` (`saveSuccessNamePT`), `formula/formula.go:84` (`ArmorCategories`; the var is `armorCategories`), `dungeon/gen.go:70` (field `onDiscard` does not exist), `levelup.go:102,374-376,979-980,1100` (`LevelUpSheetIssue`, `LevelUpClass`, `LevelUpMaxLevel`, `LevelUpMulticlassPrerequisite`; the constants are `LevelUpReason*`), `choicedata.go:46` (`OptionSummaries`), `puzzle/pillars.go:121` ("Minimum decodes it inline"; `Pillars.decode` exists at `:212`), `characters/abilityscores.go:66` (`storedSets` on `decodeSets`), `charactercreatures_roster.go:32` (`creatureRows` on `creatureViews`), `tablecontent.go:767` (`featuresOfBody` on `featureGroups`), `vitals.go:416` and `vitals_change.go:14` (`fieldErrors`), `rests.go:515` (`PointsErrors`). `rests.go:133-135`: the comment of `restRows` sits on `type restRow struct` and `func restRows` (`:141`) has none. `feat_choices.go:186`: "The label keeps the singular title: addChoice wrote it already" sits above the code that rewrites to the plural; `feat_choices.go:225 unmeetBlockedPicks` should read `markBlockedPicksUnmet`.
Fix: correct each name or delete the comment. Fix changes behaviour: no.

### core-53 · docs · low · `docs/testing.md:26,31`, `docs/architecture/rules-maps.md:150`, `docs/architecture/rules-table-content.md:195`
`docs/testing.md:31`: "The `*_characterization_test.go` files in `internal/rules` pin the behaviour of the engine's large functions" describes the refactor, not the guarantee, and the doc covers only the rules `-update` flag while the repo has 10 `-update-*` flags (`core-5`). `rules-maps.md:150` names only `testdata/golden.json`; the dungeon package also has `generator_golden.json` and `check_golden.json` (gone if `core-3` is done). The other `docs/architecture/**` pages for this area (rules, characters, progression, campaigns, identity, notes, campaign-package, logs, api) name only files and functions that exist: no stale structure sentence found.
Fix: reword the `testing.md` paragraph after the test trim; document the single `-update` flag; fix the `rules-maps.md` sentence. Fix changes behaviour: no.

## Test-trim table

Every test file added in the range for this area (76 `_test.go` files, 24,773 lines; 147 golden and testdata files, 41,988 lines; goldens are grouped with the test that reads them). Verdicts: KEEP = guards behaviour; KEEP (trim) = keep but cut a large golden or the log pins; MERGE = overlaps the named target or shares a rig; DELETE = remove or replace by a short inline table. Judged by reading heads, assertions and goldens (sampled, not every case). No test calls a helper introduced by a split (checked mechanically: 2,475 new unexported functions, the few called by the added tests all exist at the base commit); what the tests pin instead is log lines, wrapped error text, collaborator call order and the dungeon stage state.


| file | lines | package | verdict | reason | merge target |
|---|---|---|---|---|---|
| run_characterization_test.go | 241 | cmd/api | KEEP (trim) | Boots run() for 10 configs and pins every startup log line (severity+message). Guards the degraded-mode wiring (no DB / no sign-in / bad CA) but the log text is incidental; assert error + the "disabled" warnings only, drop the INFO lines. Not parallel (SIGTERM to process). Goldens: 1 files, 132 lines. | - |
| run_characterization_test.go | 254 | cmd/migrate | KEEP | Runs the real migrate command on a DB (refusals, idempotent re-run, DB errors); only test of cmd/migrate. | - |
| converters_characterization_test.go | 449 | cmd/srdimport | KEEP (fix CI) | Crafted-row goldens reach every converter branch and are small/reviewable. But TestSrdImportReproducesTheSnapshot skips unless SRDIMPORT_SRC is set and CI never sets it: the strongest test never runs. Rename file (no "characterization"). Goldens: 7 files, 685 lines. | - |
| characterization_faults_test.go | 709 | internal/campaignpackage | MERGE (trim) | Fault injection (cancelled-context tracer) for import/export/writeZip/Sweep. Real value: failure paths leave nothing half-written; but it pins log message text and wrapped error strings. Move the tracer to dbtest (copy of campaignsQueryFaults/notesQueryFaults/historyFaults), keep code + "nothing written" asserts, drop log-text pins. | internal/platform/dbtest (shared QueryFaults) |
| reader_characterization_test.go | 198 | internal/campaignpackage | KEEP | Pure table tests of FileName edge cases (mirrors web suggestedFileName) and nameImages labels; cheap, user-visible. Rename to reader_names_test.go. | - |
| characterization_faults_test.go | 763 | internal/campaigns | MERGE (trim) | Order of checks of CreateCampaign/acceptInvite/SetXpMode/UpdateDocument + failure logs. Check order is user visible; the log-line and wrapped-cause pins are internals. Reuse dbtest QueryFaults; fold the order cases into campaigns_test.go/invites_test.go tables. | internal/campaigns/{campaigns,invites,tablerules}_test.go + dbtest QueryFaults |
| charactercreatures_roster_test.go | 367 | internal/characters | KEEP | Only tests of play.CombatRoster SummonCreatures/SyncCreatures (summon limit, sync with combat). Real behaviour; MR-037. | - |
| charactercreatures_rpc_test.go | 655 | internal/characters | KEEP (trim) | Only RPC-level tests of Give/Rename/Dismiss/AdjustCreatureHitPoints (permissions, refusal order). 655 lines; the spy that asserts "what is asked of play" call by call is internal. Share harness with creatures_test.go. | internal/characters/creatures_test.go (spy/harness) |
| choices_frozen_test.go | 628 | internal/characters | KEEP (trim) | CompleteCharacterChoices/GetCampaignOpenChoices: writes, history line, who reads (hidden content). Goldens: choices_complete.golden.json is 2094 lines with 433 unique (25 steps each dumping a full sheet): assert the changed fields instead. Rename (Frozen). Goldens: 5 files, 3247 lines. | choices_test.go |
| claimsreview_frozen_test.go | 961 | internal/characters | MERGE (trim) | Claim/revoke/reserve/review calls: who may call, refusal order. Overlaps claims_test.go (MR-049), review_test.go (RN-15), approval_test.go (MR-024). Keep WhoMayCall + ClaimLosesRN03 race; drop byte-for-byte log lines and faults harness (collaborator fault injection). | claims_test.go, review_test.go, approval_test.go |
| combatreads_golden_test.go | 397 | internal/characters | KEEP (trim) | CombatSpell/CombatSheet/reaction stats/PartyVision are the contract with play and the fog (PartyVision is RN-10 adjacent). Goldens combat_sheet 1296, combat_spell 1142, reaction_stats 401, full_reaction_stats 111, party_vision 81: fine as maps of case->value but dump whole structs; trim to fields that differ by case. Goldens: 5 files, 3031 lines. | - |
| derived_golden_test.go | 221 | internal/characters | KEEP | derivedToProto/catalogToProto conversion of the full derived sheet (Pensantus 705) and the SRD class catalog (665). derived_catalog_srd_classes overlaps rules/testdata/golden/catalog.json: drop one. Defines choicesGolden (shared helper used by 3 files). Goldens: 7 files, 2199 lines. | - |
| editor_create_test.go | 439 | internal/characters | KEEP | CreateCharacter refusals and order (web places message under the input), idempotency, origin; no other test of these. Rename (Editor) is fine. | - |
| editor_harness_test.go | 363 | internal/characters | MERGE | Rig+refusal formatter for the editor tests: editorRefusal/editorMessage/editorField reimplement protoreflect printing that choicesRefusalShape, tableRefusalShape, claims, award/history rigs also do. Move to one shared helper next to harness_test.go. | harness_test.go (shared refusal-shape helper) |
| editor_list_test.go | 213 | internal/characters | KEEP | ListCharacters/GetCharacter per viewer (master, players, pending): who sees which character; RN-10 adjacent (reserved/pending). | - |
| editor_rolls_test.go | 107 | internal/characters | KEEP | RollAbilityScores: stored once, answered again (anti-reroll). Only test. | - |
| editor_sheet_test.go | 221 | internal/characters | KEEP | checkSheet refusal order by field; pure, cheap; the web marks the first failing field. | - |
| editor_update_test.go | 495 | internal/characters | KEEP (trim) | UpdateCharacter/Story refusals, portraits copy, gear told to play. Drop pins of log text. | - |
| levelup_frozen_more_test.go | 227 | internal/characters | MERGE | More level-up cases: switched-off content, preview shape, error sources. Same table (levelUpFighterSheet) as levelup_frozen_test.go; merge into one levelup_rpc_test.go. | levelup_frozen_test.go -> levelup_test.go |
| levelup_frozen_test.go | 824 | internal/characters | MERGE (trim) | Guided level-up RPCs: refusal order, who may call, what each viewer reads (switched-off content hidden from players = real). Goldens: 12 levelup_options_* viewer files (3084 lines) where pensantus_fighter_owner == _master and pensantus_owner == _master byte for byte; others differ only by hidden entries. Keep one master golden + a diff list for the player. Overlaps levelup_test.go/levelup_multiclass_test.go. Goldens: 11 files, 2866 lines. | levelup_test.go |
| levelup_frozen_unit_test.go | 367 | internal/characters | KEEP (trim) | checkLevelUpChoices shape check (pure) and levelUpOptionsToProto copy of every field. to_proto_every_field golden (229) is a field-copy pin; ok but a reflection test "every proto field set" is cheaper. Defines levelUpGolden (4th golden helper in this package). Goldens: 2 files, 253 lines. | - |
| npcspellcasting_test.go | 85 | internal/characters | KEEP | Spellcasting note of every SRD creature (what the master reads on an NPC); small golden (38). Goldens: 1 files, 38 lines. | - |
| package_part_golden_test.go | 562 | internal/characters | KEEP (trim) | Export/Stage/Apply of the characters part of a campaign package (MR-050): only test of Stage problem order. package_part.golden.json 2646 lines (815 unique) in one blob: split per kind/trim to problems list + one apply sample. Overlaps campaignpackage/roundtrip_test.go for export/apply. Goldens: 1 files, 2646 lines. | campaignpackage/roundtrip_test.go |
| revive_phases_test.go | 270 | internal/characters | MERGE | ReviveCharacter refusal order, log, stream, combat log routing. Merge into revive_test.go (RN-03); drop the log-line asserts. | revive_test.go |
| spelllist_frozen_test.go | 141 | internal/characters | MERGE (trim) | ListSpells per viewer. Goldens spells_* 7 viewer files (2239 lines): spells_stranger == spells_pending_member (identical), nobody differs only by code/message, owner vs another_player differ by 135 lines (own list). Collapse to master+owner goldens and inline want for refusals. Overlaps spelllist_test.go. Goldens: 7 files, 2239 lines. | spelllist_test.go |
| tablecontent_frozen_test.go | 374 | internal/characters | KEEP (trim) | Pure forPlayer/withoutArchived/checkPack/readPackJSON/assignIn (RN-23: hidden SRD keys removed from what a player reads); no DB. Goldens 968 lines of protojson: fine but shares fixtures with viewers test. Goldens: 5 files, 968 lines. | tablecontent_viewers_test.go (frozenEffects/frozenFeatures fixtures) |
| tablecontent_golden_test.go | 162 | internal/characters | MERGE | Golden writer for table content (own flag -update-table-content, own json masking). A 5th copy of the golden helper in this package. | shared goldenfile helper |
| tablecontent_viewers_test.go | 401 | internal/characters | KEEP (trim) | What master/players/pending/outsider/anonymous read of table entries after switches and archives (RN-10/RN-23, real). Golden table_content_viewers.golden.json = 5332 lines but 422 unique: master and Dona each dumped 4x ~1100 lines (full catalog). Record entry keys per viewer and only the changed answers; ~400 lines. Overlaps leaktest matrix for the hidden/not-hidden part. Goldens: 3 files, 5677 lines. | leaktest matrix + tablecontent_test.go |
| vitals_golden_test.go | 352 | internal/characters | DELETE (replace) | applyVitalsChange / planRest pure. plan_rest.golden.json 2674 lines with 132 unique (19 cases each dumping the full "before" vitals); apply_vitals_change.golden.json 2191 lines with 237 unique (60 cases). Rest/HP rules are real (RN rests), keep the cases as inline table with want fields (about 150 lines). Overlaps vitals_test.go TestApplyVitalsChange and vitals_rests_db_test.go. Goldens: 2 files, 4865 lines. | vitals_test.go |
| vitals_rests_db_test.go | 465 | internal/characters | KEEP | AdjustVitals/TakeRest against the DB: not_found, refusals, rows written. Only DB-level proof of rests. Comment points to vitals_golden_test.go: update after merge. | - |
| login_freeze_test.go | 60 | internal/identity | DELETE | TestSafeReturnToRefusalsAreFrozen repeats login_test.go (same refused list, same safe map, plus the HTTP 400 check). Move TestCallbackRefusesAMalformedStateBeforeTheStore into login_test.go. | internal/identity/login_test.go |
| characterization_test.go | 540 | internal/notes | KEEP (trim) | Only test of the notes service alone (CreateNote/UpdateNote check order, failures); privacy lives in maps tests. Move query-fault tracer to dbtest; rename. | dbtest QueryFaults |
| main_test.go | 11 | internal/notes | KEEP | Required TestMain (dbtest.Main) for the package. | - |
| gcs_do_characterization_test.go | 151 | internal/platform/blob | KEEP | Retry/status/network-error table of GCS.do against a fake; no other test of retry policy. Rename. | - |
| load_characterization_test.go | 200 | internal/platform/config | KEEP (trim) | 88 environments through Load: every misconfiguration message and the order of errors (operators read these). load.json 1890 lines because each success dumps the whole Config (~30 lines): store errors + only non-default fields. Failure message says only "drifted", not which case. Goldens: 1 files, 1890 lines. | - |
| template_characterization_test.go | 181 | internal/platform/dbtest | KEEP | Tests the test harness itself (DDL order, cycles, template reuse, goose ids); infrastructure every DB test depends on. Rename. | - |
| award_characterization_test.go | 1104 | internal/progression | MERGE (trim) | AwardXP/Undo: refusal order, rows/events/logs per kind, retries (idempotency), failure of other module, race. Real money/XP behaviour (keep refusals, retry, undo, race). Overlaps gold_test.go, xpmode_test.go, milestones_idempotency_test.go, serializationretry_test.go; log-line and publish-count pins are internals. 1104 lines. | gold_test.go, xpmode_test.go, serializationretry_test.go |
| award_rig_test.go | 480 | internal/progression | MERGE | Rig with faults/party/combats/log/treasure fakes: ~90% the same as history_rig_test.go (awardFault* vs historyFault*, awardLogBuffer vs historyLogBuffer, label/redact/takeLogs/refusal/wantLines). | history_rig_test.go -> one faultRig |
| history_rig_test.go | 485 | internal/progression | MERGE | See award_rig_test.go; also has a pgx tracer to fail queries (4th copy in the repo). | award_rig_test.go + dbtest QueryFaults |
| history_views_test.go | 234 | internal/progression | KEEP (trim) | What each viewer reads of the history/milestones (RN-10: gone accounts, off-list marks). Real. 6 viewer goldens 1000 lines (master 305, pensantus 283, brisa 283 almost identical structure): keep master + one player diff. Goldens: 7 files, 898 lines. | - |
| milestones_write_test.go | 296 | internal/progression | KEEP (trim) | AddMilestone/MoveMilestone steps, rows written, no session event. Goldens 551 lines step-by-step; trim to refusals + final order. Goldens: 2 files, 537 lines. | - |
| catalog_characterization_test.go | 98 | internal/rules | KEEP | Pure: BuildCatalog. Small golden (58). Overlaps derived_catalog goldens in characters; keep this one. Goldens: 1 files, 58 lines. | - |
| characterization_test.go | 58 | internal/rules | KEEP | Shared goldenText/errText helper for ~25 files (the good one). Comment mentions "split in E5": reword. | - |
| choicedata_characterization_test.go | 191 | internal/rules | KEEP | Loader refusals as text (small golden 72); what a table master reads when content is malformed. Goldens: 1 files, 72 lines. | - |
| choicegroups_characterization_test.go | 189 | internal/rules | KEEP | Choice-group sets/problems; goldens tiny. Goldens: 2 files, 39 lines. | - |
| choicegroups_pools_characterization_test.go | 153 | internal/rules | KEEP | assignPools; tiny golden (21). Goldens: 1 files, 21 lines. | - |
| choices_characterization_test.go | 540 | internal/rules | MERGE (candidate) | Derive choices/injected/option counts/spell sources; 4 small goldens; overlaps choicegroups + feat_choices tests partly. Same shape (case map -> text) as siblings: could live as inline tables in the existing *_test.go of the same loader. Goldens: 4 files, 94 lines. | internal/rules/choices_test.go |
| characterization_test.go | 268 | internal/rules/combat | KEEP (trim) | 4 sweeps (attack/save/bonus attack/options), digest only (sha256 files). Pin rules maths for every combination, but one digest says nothing about which case broke. Overlaps advantage_test.go, bonusattack_test.go, combat_test.go. Goldens: 4 files, 4 lines. | - |
| combateffects_characterization_test.go | 295 | internal/rules | KEEP | Loader refusals (modifier, effectdef, effects): text goldens, reviewable. Calls checkEffectDef/checkModifier (pre-split names). Goldens: 3 files, 124 lines. | - |
| content_characterization_test.go | 332 | internal/rules | KEEP | LoadContent refusals + correctCreature. Slowest in file (4.6s, loads content for every case). Goldens: 3 files, 150 lines. | - |
| corrections_characterization_test.go | 299 | internal/rules | KEEP | applyCorrections refusals; small golden. Goldens: 1 files, 115 lines. | - |
| creatures_characterization_test.go | 164 | internal/rules | KEEP | Monster loader refusals; small golden. Goldens: 1 files, 44 lines. | - |
| derive_collect_characterization_test.go | 221 | internal/rules | MERGE (candidate) | collectEffects golden (43). Overlaps derive_test.go partly. Same shape (case map -> text) as siblings: could live as inline tables in the existing *_test.go of the same loader. Goldens: 1 files, 43 lines. | internal/rules/derive_collect_test.go |
| derive_sheet_characterization_test.go | 588 | internal/rules | KEEP | DeriveSheet cases (117) + wild shape (30). Core maths; overlaps golden_test.go Pensantus for one sheet. Goldens: 3 files, 190 lines. | - |
| generator_characterization_test.go | 449 | internal/rules/dungeon | DELETE (merge) | Hashes the working state after each generator stage (g.kind, g.rid, g.blocked: private structure of the phases) over 2000 combos x 7.8s (slowest characterization test) plus generator_golden.json. dungeon_test.go already pins final hashes with Version (golden.json, documented in testing.md) and a bump-Version check; this adds a second golden that does not force a Version bump. Keep the readable white-box branches, drop the stage hashing. Goldens: 1 files, 90 lines. | dungeon_test.go goldenCases |
| effectloaders_characterization_test.go | 289 | internal/rules | KEEP | texts_pt refusals, effect menu, table defaults (3-line goldens), listspells. Overlaps tabledefaults_test.go/tablemenu_test.go for the 3-line goldens: drop those two. Goldens: 7 files, 138 lines. | - |
| effects_characterization_test.go | 243 | internal/rules | KEEP | Effect compile refusals; text golden 185. Goldens: 1 files, 185 lines. | - |
| characterization_test.go | 63 | internal/rules/encounter | KEEP | Sweep digest of encounter generation (seeded RNG order); 1.6s; overlaps encounter_test.go promises tests. Goldens: 1 files, 1 lines. | - |
| featchoices_characterization_test.go | 251 | internal/rules | MERGE (candidate) | 5 small goldens (19..56) on feat choice shape/unmet/picked/values/metamagic; overlaps feat_choices_test.go, metamagic_test.go. Same shape (case map -> text) as siblings: could live as inline tables in the existing *_test.go of the same loader. Goldens: 5 files, 129 lines. | internal/rules/featchoices_test.go |
| characterization_test.go | 118 | internal/rules/formula | KEEP | Compile golden (178) + trees; formula_test.go and fuzz exist; small. Goldens: 1 files, 178 lines. | - |
| characterization_test.go | 297 | internal/rules/grid | KEEP (trim) | Move/area sweeps as sha256 (460 KB text each). Real maths but opaque; Move off the grid and wall sight cases are readable and good. 2.6s. Goldens: 2 files, 2 lines. | - |
| items_characterization_test.go | 468 | internal/rules | KEEP (trim) | 8 goldens: magic item/treasure refusals, GenerateTreasure (201, seeded RNG order: real), creature-reads (683 lines of hashes: unreviewable, delete that one), spell texts. Goldens: 8 files, 1035 lines. | - |
| levelup_characterization_test.go | 688 | internal/rules | KEEP (trim) | Options for every class/subclass/level as 16-hex digests (439 lines), 9 full offers (1347), ApplyLevelUp (973), multiclass (123), scene options (1203, unrelated to level-up!). Digest golden is unreviewable and changes with any SRD edit; levelup_sweep_test.go already proves options==check==validate for every step (6.5s). Delete digests + sweep copy; keep full offers/apply/refusals; move scene-options to its own test. Goldens: 6 files, 4202 lines. | - |
| load_bench_test.go | 15 | internal/rules | KEEP | BenchmarkLoadSRD is the command named in docs/performance.md; comment mentions the split: reword. | - |
| overlay_budget_characterization_test.go | 261 | internal/rules | KEEP | Overlay budget refusal texts; reviewable. Goldens: 1 files, 86 lines. | - |
| overlay_build_characterization_test.go | 259 | internal/rules | KEEP | Overlay build refusals + overlay-content (24). Goldens: 2 files, 75 lines. | - |
| overlay_checkeffect_characterization_test.go | 195 | internal/rules | KEEP | checkeffect refusals text. Goldens: 1 files, 87 lines. | - |
| overlay_class_characterization_test.go | 261 | internal/rules | KEEP | ~100 refusals of table classes; overlaps overlay_test.go/tableclass tests partly; keep (message order is user visible). Goldens: 1 files, 104 lines. | - |
| overlay_entries_characterization_test.go | 553 | internal/rules | KEEP | Feature/spell/race refusals + effect fields; 4 goldens. Goldens: 4 files, 301 lines. | - |
| characterization_test.go | 218 | internal/rules/puzzle | KEEP (trim) | Sweep digests for lights/pillars/cipher; TestLightsMinimumSweepCharacterization duplicates TestLightsMinimumMatchesBruteForce (12.7s) in puzzle_test/lights. Goldens: 3 files, 3 lines. | - |
| spellcasting_characterization_test.go | 294 | internal/rules | KEEP | CharacterSpells golden (114). Goldens: 1 files, 114 lines. | - |
| spelleffects_characterization_test.go | 157 | internal/rules | MERGE (candidate) | Spell effect refusals; overlaps spelleffects_test.go. Same shape (case map -> text) as siblings: could live as inline tables in the existing *_test.go of the same loader. Goldens: 1 files, 116 lines. | internal/rules/spelleffects_test.go |
| summon_characterization_test.go | 128 | internal/rules | MERGE (candidate) | Summon load refusals + options (90); overlaps summon_test.go. Same shape (case map -> text) as siblings: could live as inline tables in the existing *_test.go of the same loader. Goldens: 2 files, 136 lines. | internal/rules/summon_test.go |
| traps_characterization_test.go | 272 | internal/rules | MERGE (candidate) | Trap loader refusals (131); calls loadTraps; overlaps traps_test.go. Same shape (case map -> text) as siblings: could live as inline tables in the existing *_test.go of the same loader. Goldens: 1 files, 131 lines. | internal/rules/traps_test.go |
| validate_characterization_test.go | 168 | internal/rules | KEEP | Validate refusals golden (98). Goldens: 1 files, 98 lines. | - |
| characterization_test.go | 77 | internal/rules/vision | KEEP | See sweep digest over seeded maps; fog of war maths, 77 lines; only exhaustive vision test beside vision_test.go. Goldens: 1 files, 1 lines. | - |

Goldens are counted with the test that reads them. The 12 `levelup_options_*` viewer files, the 6 `spells_*` files and the 6 `history_*` files are attached to levelup_frozen_test, spelllist_frozen_test and history_views_test.

### Totals

- Verdicts over 76 files: KEEP 37, KEEP (trim) 18, MERGE 18, DELETE 3 (`characters/vitals_golden_test.go`, `identity/login_freeze_test.go`, `rules/dungeon/generator_characterization_test.go`).

### Estimates

Lines saved if all trims above are done (estimate from the file sizes and uniqueness measured above):
- Goldens: vitals 4,800 + table-content viewers 4,900 + level-up/spells viewer files 3,000 + choices_complete 1,400 + package_part 1,500 + rules digests 1,122 + scene-options 1,000 + config load.json 1,200 + history/milestones 700 + combat/derived overlaps 500 = about 20,000 of 42,000 golden lines (48%). A conservative reading (only exact duplicates and identified digests) is about 11,000.
- Test code: rig merge 300 + tracer 120 + refusal-shape/golden helpers 350 + log pins 900 + dungeon stage hashing 250 + login_freeze 40 + digest generators 130 = about 2,100 of 24,800 (8%).
- Total about 17-22k of 66k lines (26-33%).

CI time (estimates unless marked measured):
- Measured: `go test ./internal/rules/... -count=1`, no -race, parallel: 38.5 s wall, 120 s CPU. 93 characterization/golden/sweep tests sum 46.6 s of test time; the largest are TestGoldenGeneratorCharacterization 7.8 s, TestLevelUpOptionsCharacterization 6.5 s, TestLoadContentCharacterization 4.6 s, MoveSweep 2.6 s, OptionsSweep 2.2 s. Package wall: rules 29.4 s, dungeon 15.6 s, puzzle 13.1 s.
- Saved by deleting digests, dungeon stage hashing and the lights hash: about 7.8 (partly kept) + 6.5 + 0.4 + 12.7x0 (oracle test stays) = roughly 10-14 s CPU; wall about 4-6 s for `rules` and 3-5 s for `dungeon` (critical path moves to Lights/oracle tests). With -race (CI) expect 3-5x, so ~30-50 s CPU, still only a few seconds of wall because packages run in parallel.
- DB-backed tests (not run): the added files build about 170 harness/rig instances (each copies the template tables into a one-connection database, ~0.3-0.5 s, more under -race, plus the test body). Estimated 2-4 minutes of CPU across the nine `go-db` groups. Removing duplicated cases (claims/review overlap, award overlap, faults) saves perhaps 20-30 rig instances: ~10-25 s CPU, under 10 s of wall because groups run in parallel. The golden trims do not change DB time (the answers are still computed), only review time and disk.
- Net: CI wall time barely moves (< 10 s); the gain is review and maintenance cost (a 5,000-line golden diff is never read).
