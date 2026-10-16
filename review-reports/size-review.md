# MeuRPG size review

Read-only, static analysis of `main` at d7c7741 (same tree as PuraFome/meuRPG `main`). Nothing was run except builds and analysis tools. "Estimate" marks a number I did not get straight from a tool.

## 1. Verdict

About 300K hand-written app lines (Go 145K + web TS 155K) for 326 RPCs, a full SRD 5.1 rules engine, combat with reactions and contests, maps/fog/dungeon generation, puzzles, traps, level-up and content packages is **justified by the scope**: roughly 900 app lines per RPC, with tests at about 1:1 to app code. The tools do not show a copy-paste problem: Go `dupl` (threshold 100 tokens) finds 1.4K duplicated lines in hand-written Go, `jscpd` finds 0.7% duplicated TS in app code (0.84% across all TS) and 5.6% in SCSS, dead code is about 40 lines of Go and 4 names in web. The size comes from **structure repeated N times, not from textual copies**: eight roll paths wired by hand to the held-roll logic, seven web sheets each with an `answerDie` copy, 18 request-parsing switches, 124 `busy`/`error` try/finally blocks. Realistic consolidation: **about 7K lines of the ~730K hand-written** (app 3.7K, tests 3.3K, estimate) plus about 0.9K generated lines once the held-roll RPCs are retired, so about 1%. The held-roll unification is worth doing for correctness and for the next feature (Lucky on checks and saves would otherwise be built a third time), not for size.

## 2. The held-roll mechanism (analysed first)

### 2.1 What exists today

| | Weapon attack | The other seven kinds |
|---|---|---|
| Table | `roll_holds` (00220, +Lucky columns 00253) | `inspiration_holds` (00244, kinds widened in 00251) |
| Owner | combatant (`combatant_id NOT NULL`, FK to `encounters`) | character (`character_id NOT NULL`, FK to `campaigns`) |
| Key | `idempotency_key TEXT`, unique per encounter; `"#window"` suffix for holds made before a reaction window (`combat_inspiration.go:129,238`) | `idempotency_key UUID`, unique per campaign |
| D20 | one `face` plus both faces inside the pinned `request` proto | `faces INT4[]`, `modifier`, `total`, `counted`, `physical` columns |
| Gate | `rollOrHold` (`combat_inspiration.go:89`) | `outsideRoll` (`inspiration_outside.go:174`), `holdSpellAttack`/`saveWithDie` (`inspiration_held.go:41,91`) |
| "Answered" context | `forcedRoll` + `withForced` (`combat_inspiration.go:35,66`) | `forcedOutside` + `withOutside` (`inspiration_outside.go:138-153`) |
| Answer RPC | `AnswerBardicInspiration`, `AnswerLucky` (`resources.proto:135,155`) | `AnswerOutsideInspiration` (`resources.proto:184`), a 7-way `oneof result` |
| Offer message | `InspirationOffer`, `LuckyOffer` (`combat.proto:3382,3398`) | `OutsideInspirationOffer` (`combat.proto:3432`) |
| Lucky question | yes (`combat_lucky.go`, 340 lines) | no |
| Touches encounter revision | yes (`combat_inspiration.go:153,175`) | no, even for holds made in a combat |
| Who may answer | any member, then `mayAct` on the combatant (`combat_inspiration.go:432`, `combat_lucky.go:309`), so the master can | `RolePlayer` and the hold's own `user_id` (`inspiration_outside.go:569,596`), so the master cannot |

Both sides do the same four things: keep the d20 and the pinned request before writing anything; answer by calling the original RPC again with the d20 carried in the context; refuse a new roll while a question is open (written twice, once per table: `combat_inspiration.go:200-226`, `inspiration_outside.go:228-251`); forget stale holds when the round ends (`combat_inspiration.go:108-112` vs `staleHold`, `inspiration_outside.go:133`).

The seven call sites of `outsideRoll`/`saveWithDie`, plus the attack gate, are: `scene.go:756` (scene_check), `combat_groupcheck.go:351` (group_check), `traps.go:284` (trap_search), `puzzles_hints.go:241` (hint_try), `combat_spells.go:504` via `inspiration_held.go:65` (spell_attack), `lasting_effects_save.go:303` (effect_save), `combat_concentration.go:231` (concentration_save), `combat_actions.go:891` (weapon attack). Each one repeats roughly: `checkModeOf` → `withEffectDice` → hold gate → (not settled) `d20With` → `reliableD20` → `spendOnceEffects`/`spendCheckHelps` (`scene.go:741-790`, `traps.go:252-313`, `puzzles_hints.go:231-264`, `combat_groupcheck.go:342-380`).

Two cross-checks already differ in behaviour (observed, not verified as bugs): (a) a pending spell-attack or save question in a combat is not in `ListOpenRollHolds` (`combat_view.go:134`), so the combat view the master reads never shows it; (b) the master can answer an attack hold but not an outside hold.

### 2.2 One mechanism: design sketch

Keep the trick that already works (re-run the original RPC with the d20 forced) and make it one thing.

1. **One storage shape.** Extend `inspiration_holds` (the more general table: kind, request, faces[], round) with nullable `combatant_id`, `encounter_id`, `lucky_index`, `lucky_answer_key`, `lucky_to`, a `window BOOL` (replaces the `"#window"` string suffix, which cannot live in a UUID column) and kind `weapon_attack`. `character_id` becomes nullable only if an attacker can be a combatant with no character (needs checking: `lucky` comes from the attacker's sheet traits, `combat_actions.go:843`). Alternative: a new `held_rolls` table; the extension route needs no new table and no data move.
2. **One gate.** `s.heldGate(ctx, env, rollSpec) (gate, error)` where `rollSpec` = kind, key, raw request, `rollInput`, modifier, mode, reliable, lucky. It returns Held (offer), Settled (d20, bonus die, Lucky reroll) or None. It replaces `rollOrHold`, `outsideRoll`, `saveWithDie`, `holdSpellAttack`'s hold half, `forcedOf`/`outsideOf`, `mayHold`, `refuseWhileAsking` and both stale-hold helpers. `rollAttack` keeps its own mode/window/action-economy code and only swaps the gate.
3. **Questions as a chain.** A hold carries an ordered list of pending questions computed from state (Lucky first, then the die), as `luckyPending` already does for attacks. Lucky then works on every kind by construction.
4. **One answer RPC and registry.** `AnswerHeldRoll{campaign_id, hold_id, idempotency_key, oneof answer{DieAnswer, LuckyAnswer}}`, response a `oneof result` of the eight roll responses. The 5-arm `answerCombatHold` switch (`inspiration_outside.go:657-716`; dupl -t60 flags the five 10-line arms at 661-700) and the two arms in `AnswerOutsideInspiration` become a registry of eight entries, `register[Req, Resp](kind, handler, wrap)`. Authorization is the union: owner user, or the master for combatants.
5. **One offer message.** `HeldRollOffer{hold_id, kind, d20, oneof question{DieQuestion, LuckyQuestion}, attack_key, target_id}` replaces three offers and the `inspiration_offer`/`lucky_offer` fields in 8 response messages and the combatant view (`combat.proto:2400,2405,3455,3460,4077,5550`, `contests.proto:508`, `lasting_effects_service.proto:280`, `puzzles.proto:1405`, `scene.proto:302`, `traps.proto:98`).
6. **Web.** One `HeldRollController` (offer signal, `answerDie`, `answerLucky`, key renewal, error text) used by the seven sheets that today each carry about 30 lines of the same method (`trap-search-sheet.ts:412-440`, `effect-save-sheet.ts:397`, `cast-sheet.ts:1082`, `reaction-sheet.ts:893`, `scene-roll-sheet.ts:268`, `puzzles-client.ts:236`, `bardic-block.ts:192`; attack-sheet has two more at `:926,:975`).

### 2.3 What it removes (estimates)

Mechanism today: 497 + 340 + 99 lines in the three files plus about 450 hold-specific lines in `inspiration_outside.go` (46-71,133-297,533-545,565-716) = about 1,390 lines. A single gate, store and registry is about 650-750 lines, so **app Go -650 to -750**. Web: seven `answerDie` copies plus `offer` signals and `holdForDie` at about 40 lines each, replaced by about 8 per sheet: **-200**. The eight `outsideRoll`/gate call sites also drop 10-15 lines each (counted inside the figure above). SQL: 16 hold queries generate 456 lines in `playdb/queries.sql.go`; about 9 would remain: **gen -175**. Protos/gen: three RPCs and six messages (about 570 generated Go lines of struct code, measured) plus three offers (203) become one request/response (about 280) and one offer (about 90); connect code is about 55 lines per RPC (`resources.connect.go`, 719 lines / 13 RPCs): **gen about -900 Go+TS in total, only after the old RPCs are removed**. Tests: the four held-specific Go files are 1,148 lines (`inspiration_held_test.go` 185, `inspiration_outside_test.go` 343, `combat_lucky_test.go` 564, `combat_inspiration_reaction_test.go` 56); a table over (kind × use/keep/retry/stale/wrong user) saves about 35%: **-400 Go**. Web specs for the same (`attack-sheet.inspiration.spec.ts` 321, `attack-sheet.lucky.spec.ts` 329, `scene-roll-sheet-inspiration.spec.ts` 108, `cast-sheet.inspiration.spec.ts` 99, `bardic-block.spec.ts` 139): **-300**.

### 2.4 Risks

- It is the path that decides whether a roll counts, with idempotent retries, in a live game. Behaviour must stay bit-identical for old rows and retries; the existing tests are the safety net, so refactor first (step 0).
- Attack holds live inside `rollAttack`'s transaction, with reaction windows, mode requests (`dry`/`trusted`, `combat_actions.go:882-886`) and the `"#window"` key; outside holds take `(tx, q)`. The gate must accept both environments.
- Authorization union (above) and the encounter-revision touch are behaviour changes: decide them explicitly, do not inherit them by accident.
- `buf breaking` uses the FILE rule (`proto/buf.yaml`), so the old RPCs cannot be deleted in the same release that adds the new one.

### 2.5 Migration (expand → migrate → contract; migrations only add)

0. **No schema, no wire change.** Introduce the gate and the registry in Go over the *existing* two tables behind one interface. Ship. Zero data risk; the 1,148 existing test lines prove equivalence.
1. **Additive migration.** `ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS` for the nullable columns above; widen the kind check the way 00251 does (drop by both names, re-add). Old code ignores the new columns and rows.
2. **Release N: read both, write old.** Code can read a `weapon_attack` row in `inspiration_holds` but still writes `roll_holds`. This makes rolling deploys and rollback to N safe whatever pod answers.
3. **Release N+1: write new, read both.** New attack holds go to `inspiration_holds`; open `roll_holds` rows made by N keep being answered through the legacy path (the old RPCs stay registered). New web reads either offer type and calls `AnswerHeldRoll`.
4. **Contract.** After `SELECT count(*) FROM roll_holds WHERE answer_key IS NULL` is 0 for a release, remove the legacy read path, deprecate then remove the three old RPCs (with an explicit `buf breaking` exception), and leave `roll_holds` empty and unused (the "migrations only add" rule means no `DROP`).
5. **Rollback:** N+1 → N is safe (N reads the new rows). N+1 → pre-0 leaves at most `weapon_attack` rows that old code cannot answer; they are stale after the round (`staleHold` has a round) and the cost is one refused roll ("answer the question first") for one round. No money, no spent resources: a hold writes nothing until it is answered.

## 3. Ranked consolidation candidates

Lines are estimates unless a tool number is given. Risk is the risk of the change, not of leaving it.

| # | Candidate | Where | App | Tests | Gen | Risk |
|---|---|---|---|---|---|---|
| 1 | One held-roll mechanism + `AnswerHeldRoll` (section 2) | `inspiration_outside.go`, `combat_inspiration.go`, `combat_lucky.go`, `inspiration_held.go`, 7 web sheets | -650/-750 Go, -200 web | -400 Go, -300 web | -900 (after contract) | med-high: live rolls; staged migration above |
| 2 | Shared SCSS partials/mixins | `pages/content/options.scss:1-55` header copied into content-import, content-entry, content-list, table-rules, campaign-export/import/document; `editor.scss` ↔ `spell-editor.scss` (114+74); `abilities-step.scss:79-140` ↔ `pick-list.scss:123-184` | -1,200 (jscpd: 2,898 dup lines in pairs) | 0 | 0 | low: visual only, needs a look at each page |
| 3 | Go test fixtures and harness helpers | `maps/fog_test.go:937-1100` = `rules/vision/cave_expected_test.go:7-170` (164 identical lines); `*/harness_test.go` copies (`play:225`, `maps:277`, `progression:274`, `campaignpackage:273`); 24-way repeated 5-line setup (`combat_brutal_test.go:27`, `combat_frenzy_test.go:27`, ...) | 0 | -1,500 (dupl -t60: 4,227 dup test lines in 182 groups, 2,549 removable) | 0 | low |
| 4 | Web spec builders | `options-class-filter.spec.ts:32-109` ↔ `options.spec.ts:53-130`; `combat-view.forced-move.spec.ts:72-116` ↔ `place.spec.ts:60-104`; 10 more combat-view specs; `import-part-uploader.spec.ts:13-48` ↔ `image-uploader.spec.ts:14-49` | 0 | -900 (jscpd: 1,725 dup spec lines) | 0 | low |
| 5 | `busy`/`error` try/catch/finally helper for sheets | 124 `busy.set(true)` in 91 files, 219 `error.set('')` | -600 (5-6 lines per site) | 0 | 0 | low; wide diff, merge-conflict heavy |
| 6 | Roll-input parser (`roll_in_app must be true` + range check) | 18 Go blocks: `combat_actions.go:626,1182,2033`, `scene.go:617`, `traps.go:162`, `puzzles_hints.go:61`, `lasting_effects_save.go:158`, `combat_concentration.go:114`, `combat_spells.go:261`, `combat.go:524`, `combat_death.go:80`, `resources.go:373`, `casting.go:132`, `combat_contests_core.go:243`, `combat_reaction_counterspell.go:264`, `combat_lucky.go:185`, `combat_inspiration.go:361`, `inspiration_outside.go:586` | -250 | 0 | 0 | low: pure, unit-testable |
| 7 | One check pipeline for the four player d20 checks | `scene.go:741-790`, `traps.go:252-313`, `puzzles_hints.go:231-264`, `combat_groupcheck.go:342-380` | -150 | 0 | 0 | med: traps have a second die for Perception/light |
| 8 | Merge the concentration and effect-save roll paths | `combat_concentration.go:109-262` ↔ `lasting_effects_save.go:153-330` (31 identical lines at :109-139/:153-183, then the same die/effect-dice/spend sequence) | -80 | 0 | 0 | med |
| 9 | Dead code | section 4 | -50 certain, -450 optional | -0..-200 | 0 | low |
| 10 | Small Go helpers from dupl | typed-detail error builders (`casting.go:108`, `combat_contests_core.go:215`, `combat_write.go:694`, `resources.go:70`), keyset token codec (`characters/levelup.go:87` ↔ `progression/read.go:106`), contest/hide preamble (`combat_contests_view.go:506` ↔ `combat_hide.go:364`), spell-slot twins (`combat_resources.go:445` ↔ `:466`), `nostore.go:29` ↔ `campaigns.go:220` | -150 | 0 | 0 | low |
| 11 | Thin web client forwarders | `combat-client.ts` (1,485 lines, 50 methods, median 19 lines), `resources-client.ts` (293 lines); sampled 6 methods, most only copy arguments | -300 (unverified) | 0 | 0 | low-med: loses named arguments |
| 12 | Shared `EncounterResponse` / identical 4-field command requests | 34 responses are exactly `{encounter}`; 6 requests are exactly `{campaign_id, encounter_id, combatant_id, idempotency_key}` (`StandUp`, `RemoveCombatant`, `EndRage`, `EndConcentration`, `ConfirmDeath`, `RevealHider`) | 0 | 0 | -3,000/-4,500 (upper bound: 33 msgs × 85 Go + 64 TS) | high: **not recommended**, see section 7 |
| 13 | Split `combat-view.ts` (3,170 lines, 98 methods in one class), `character-editor.ts` (2,070) | section 8 | 0 | 0 | 0 | low; moves lines only |
| 14 | Split `rollAttack` (508 lines), `castSpell` (507), `rollDamage` (304), `TakeAction` (273) | `combat_actions.go:598`, `combat_spells.go` | 0 | 0 | 0 | med; moves lines only, after #1 and #6 the prologue shrinks |
| 15 | Other ask/answer pairs (`AnswerReaction` `combat.proto:1236`, `AnswerRollModeRequest` `:1353`, `AnswerRageEnd` `:1383`) | not analysed in depth | ? | ? | ? | unknown: could share the held-roll registry |

## 4. Dead code

**Go** (`deadcode` v0.52.0, see section 10): from the `cmd` mains, 51 unreachable functions, 525 lines (function bodies plus doc comments, estimate from line ranges). With tests as roots (`-test ./...`): only **4 functions, 41 lines, are dead everywhere**: `armorOf` (`play/casting_end.go:73`), `Service.otherCastsOn` (`:84`), `rules.CheckFeat` (`rules/feats.go:203`), and the test helper `memStore.revokeUserSessions` (`identity/memstore_test.go:161`). The other **48 functions, 494 lines, are test-only**; by package: rules/combat 13 funcs (108 lines: `SpendSlot`, `SpendPactSlot`, `SpendResource`, `DiceToRoll`, `DamageTotal`, `EndsAtStartOf`, ...), rules/dungeon 4 (154: `ASCII`, `PromptJSON`, `PromptOf`, `connectionsAndNetworks`), rules 6 (about 30), characters 5 (32: `NewSRDSource`, `CheckSheetCanMove`), maps 4 (27), platform/logging 4 (17), grid 4 (35), puzzle `Decipher` (22), migrations `Fingerprint` (23), the rest 1-2 each. Confidence: **high** for the 4 (static, tests included); **medium** for the 48, since they are probably kept as tested pure rule functions; delete them only with their tests. False-positive risk: low for functions (RTA call graph, interface calls counted as reachable); false negatives are high because `deadcode` does not report unused types, constants, variables or fields.

**Web** (`knip` 5.88.1, custom config, see section 10): no unused files; no orphan `.html`/`.scss` (own script: every template and style is referenced). 170 exported value names in 98 files and 212 exported types in 147 files are not imported by any other file, but **only 4 names occur once in all of `web/src`**: `restWords` (`core/casting/cast-out-flow.ts`), `GRAPPLE_REMINDER` (`core/combat/contest-view.ts`), `roundsText` (`core/effects/effects-text.ts`), `ALSO_FIND_MAX` (`pages/maps/point-kinds/trap-draft.ts`). The other 166 are used inside their own file; removing `export` saves no line. False-positive risk: medium (no `node_modules`, Angular and ESLint plugins disabled; template-only references are not seen by knip, but standalone components must be imported in TS, so a component used only in a template would have been flagged as unused and none was).

## 5. Duplication totals

- **Go** (`dupl` v1.0.0, `-t 100`, after dropping `gen/` and sqlc packages): 36 groups, 1,457 duplicated lines across all instances, 801 removable (all but the largest instance): app 10 groups / 362 / 180, tests 24 / 1,033 / 583, mixed 2 / 62 / 38. At `-t 60`: 237 groups, 5,594 / 3,291 removable (app 52 groups, 1,245 / 654; tests 182, 4,227 / 2,549).
- **TS/HTML/SCSS** (`jscpd` 4.0.5, `--min-lines 15`, `web/src` + `e2e/tests`): 2,286 files, 359,695 lines, 235 clones, 5,482 duplicated lines (1.52%): TS 2,331 (0.84%; 1,725 of those are between specs), SCSS 2,898 (5.56%), HTML 253 (0.85%). Re-run on app TS only, no specs, `--min-lines 6 --min-tokens 60` (`pages/live-session` + `core`): 524 lines (0.7%). Web sheets are therefore **not copy-pasted**; they repeat a pattern in many small pieces (candidates 1, 5).

Top clusters (Go): 1) `maps/fog_test.go:937-1100` = `rules/vision/cave_expected_test.go:7-170`: the same cave grid fixture. 2) `inspiration_outside.go:661-700`: five 10-line arms (unmarshal, call, wrap in `oneof`) of `answerCombatHold`. 3) `combat_concentration.go:109-139` = `lasting_effects_save.go:153-183`: the roll-input switch (`concentrationRollOf`/`effectSaveRollOf`). 4) `combat_contests_view.go:506-525` = `combat_hide.go:364-383`: read contest data, build reader. 5) `characters/levelup.go:87-105` = `progression/read.go:106-124`: decode a keyset page token, copied between modules. 6) `combat_resources.go:445-462` = `:466-483`: `CreateSpellSlot`/`ConvertSpellSlot` preamble. 7) `puzzles_hints.go:58-74` = `traps.go:159-175`: roll-input switch. 8) `campaigns.go:220-235` = `platform/nostore/nostore.go:29-44`: Connect interceptor wrapper. 9) `casting.go:108-118`, `combat_contests_core.go:215-225`, `combat_write.go:694-704`, `resources.go:70-80`: error-with-typed-detail builders. 10) `combat_opportunity.go:364-377` = `combat_theatre.go:398-411`: master-RPC preamble (role check, id parsing).

Top clusters (web): 11) `content/editor.scss:1-115` = `spell-editor/spell-editor.scss:1-115` (+74 more, :115-189): editor chrome. 12) `options-class-filter.spec.ts:32-109` = `options.spec.ts:53-130`: 77 lines of spec setup. 13) `abilities-step.scss:79-140` = `pick-list.scss:123-184`. 14) `options.scss:1-55` = `content-import.scss`, `content-entry.scss`, `content-list.scss`, `table-rules.scss`, `campaign-*.scss`: one page header copied into 7+ files. 15) `action-groups.html:370-420` = `:40-91`: 50-line block twice in one template. 16) `combat-view.forced-move.spec.ts:72-116` = `combat-view.place.spec.ts:60-104`. 17) `damage-type-picker.scss:1-44` = `ability-picker.scss:1-44`. 18) `bestiary-list.scss:8-48` = `puzzle-form.scss:5-45`. 19) `bestiary-creature.html:14-49` = `encounter-builder.html:14-49`. 20) `choice-cards.scss:24-63` = `slot-picker.scss:24-63`.

## 6. API surface

326 RPCs, 1,161 messages, 26,832 proto lines in 24 services. RPCs per service: CombatService 51, CharacterService 39, MapService 35, PlayService 32, PuzzleService 20, ContestService 18, CampaignService 15, ResourceService 13, ProgressionService 13, LastingEffectService 12, TableContentService 11, ImageGenerationService 9, CampaignPackageService 8, EncounterService 7, ContentService 7, CastingService 6, five services with 5, TreasureService 4, GalleryService 3, CampaignDocumentService 2, SystemService 1. Messages by file: `combat.proto` 198 (5,657 lines), `characters.proto` 134, `rules.proto` 102, `puzzles.proto` 89, `maps.proto` 87.

Generated cost (measured): `play/v1` is 64,240 `pb.go` lines for 752 message structs (about 85 per message) plus 10,674 connect lines (about 55 per RPC); web `gen` is about 64 lines per message. One RPC with two small messages costs about 250 generated lines.

Findings:
- **Answer RPCs**: `AnswerBardicInspirationRequest` and `AnswerOutsideInspirationRequest` share 6 of 7 fields (Jaccard 0.86); `AnswerLuckyRequest` is the same shape with `reroll` for `use`. Three request/response pairs, three offers, `inspiration_offer`/`lucky_offer` fields in 8 responses plus the combatant view: candidate 1.
- **Roll oneof**: 19 `oneof roll { roll_in_app / typed face }` blocks (casting 2, combat 8, contests 1, lasting_effects 1, puzzles 1, resources 4, scene 1, traps 1) and 18 matching Go switches (candidate 6). A shared `RollSource` message would help new requests only; changing existing ones is a breaking change.
- **Command requests**: 94 message pairs have field sets with Jaccard ≥ 0.75; the biggest family is the 4-field combatant command (6 messages exact, 10+ near) and 34 `{encounter}` responses (candidate 12).
- `CreatePuzzleRequest`/`UpdatePuzzleRequest` (13 shared fields, 0.86) and `CreateMapPointRequest`/`UpdateMapPointRequest` (0.81) are create/update twins; merging saves little and loses field-level semantics.

## 7. Looks duplicated, should stay separate

- **Per-RPC request/response messages.** `buf.yaml` lints with STANDARD (unique, standard-named request and response per RPC; per the buf docs, not run here) and `breaking` uses FILE. A shared `EncounterResponse` would need lint exceptions and removes the ability to add a field to one RPC later. The 3-4K generated lines are real, but they are the price of a stable contract.
- **sqlc output per module** (`playdb`, `charactersdb`, `mapsdb`): `dupl` finds many clones there (excluded from the totals); they are generated and module boundaries stay clean.
- **`rollAttack` vs the other d20 paths.** Attacks have reaction windows (Cutting Words), action economy, roll-mode requests and an advantage pair; share only the hold gate (candidate 1), not the whole function.
- **Saves the server rolls for targets** (`combat_spells.go:951`, `combat_riders.go:342`) do not hold, on purpose: the target's player is not rolling (`inspiration_held.go` header). Do not fold them into the player save path.
- **`// X implements playv1connect...Handler` comments** (341 lines): required by `revive`'s `exported` rule (`backend/.golangci.yml:116`).
- **The seven web sheets' templates**: different questions, results and animations. Share the controller, not the markup.
- **`combat_spells_test.go` (2,980 lines), `combat_actions_test.go` (2,189), `a11y.spec.ts` (7,913)**: long scenario/matrix files with little textual repetition (`jscpd` finds one 19-line clone in `e2e/tests`). Splitting only moves lines.
- **Rules text in Go and in `combat-log.ts` (1,324 lines)**: not analysed; the web formats server lines, and RN-10/RN-20 (no hidden numbers on the wire) make a shared source unlikely.

## 8. Oversized files (15 largest hand-written, no tests/generated)

| File | Lines | Split helps? |
|---|---|---|
| `web/.../combat/combat-view.ts` | 3,170 | One class, 98 methods, 129 imports: mostly "open sheet X" handlers; per-feature openers would help tests, not size |
| `backend/internal/play/combat_actions.go` | 2,432 | `rollAttack` alone is 508 lines; extracting phases helps reading; shrinks only with #1/#6 |
| `web/.../character-editor/character-editor.ts` | 2,070 | One class, 42 methods; form sections could split; moves lines |
| `backend/internal/maps/mapservice.go` | 1,780 | 43 RPC handlers of ≤141 lines; fine as a file, split by resource |
| `backend/cmd/srdimport/main.go` | 1,687 | 48 converters ≤94 lines; one file per SRD table would be cleaner; moves lines |
| `web/.../core/combat/combat-client.ts` | 1,485 | 50 forwarders; thinning is #11 |
| `backend/internal/rules/levelup.go` | 1,472 | 41 funcs ≤137 lines; moves lines |
| `backend/internal/maps/imagegen.go` | 1,431 | `reserve` 160 + `run` 141 lines; moves lines |
| `backend/internal/rules/content.go` | 1,394 | `applyCorrections` 244 lines is the outlier; split helps |
| `web/.../live-session/live-session.ts` | 1,377 | One class, 36 methods; moves lines |
| `backend/internal/play/combat.go` | 1,377 | `EndTurn` 230, `StartEncounter` 163; moves lines |
| `web/.../core/combat/combat-log.ts` | 1,324 | Many formatters, no class; split per log kind would help navigation only |
| `web/.../cast-sheet/cast-sheet.ts` | 1,240 | Shrinks with #1 (answerDie, offer) and #5 |
| `backend/internal/play/combat_parts.go` | 1,223 | 54 small funcs; fine |
| `backend/internal/characters/levelup.go` | 1,189 | `planLevelUpIn` 119; moves lines |

(Next: `dungeons.go` 1,153, `overlay.go` 1,136, `attack-sheet.ts` 1,101, `rules/api.go` 1,072, `reaction-sheet.ts` 1,059, `combat_spells.go` 1,044, whose `castSpell` is 507 lines.)

## 9. Comments

Hand-written non-test code has 24,234 comment-prefixed lines in Go and 18,303 in TS. I read about 55 comments sampled from 10 files (`combat_actions.go`, `inspiration_outside.go`, `mapservice.go`, `characters/levelup.go`, `rules/levelup.go`, `scene.go`, `combat-view.ts`, `character-editor.ts`, `combat-log.ts`, `attack-sheet.ts`; per-file share 7-22%). Nearly all explain why: security rules (RN-10, RN-20 appear on 679 Go and 438 TS comment lines and are the traceable source of the "hidden numbers" rule), SRD behaviour, retry semantics, ordering. Keep them. Noise I found is narrow: (a) process-history tags in comments, `phase 2b`, `integrator fix`, `question 55` (`character-editor.ts:271,450`, `mapservice.go:1368`) and backlog IDs such as `MR-013, E6-01 to E6-16` (`combat-view.ts:243`); 441 Go and 938 TS comment lines match these tag patterns (MR-nnn is a product requirement id that exists in `docs/pt-BR/produto/`, so some are real traceability). Removing the tags would shorten lines, not delete them. (b) A comment copied between paths and now wrong: `scene.go:749` says "saving throw" above a scene check (the original is at `combat_concentration.go:245`); one or two cases in the sample. Recommendation: no comment campaign; drop tags like "phase 2b / integrator fix" when a file is touched anyway.

## 10. Commands, versions, failures

Commands (all from the repo root unless noted):
- `go build ./...` in `backend/` (passes). `go.mod` says `go 1.27.2`; the machine's `go` is 1.24.7 and auto-switched.
- `go run github.com/mibk/dupl@v1.0.0 -t 100 <package dirs>` and the same with `-t 60`, in `backend/`, on every package dir except `gen/` and `*db`. `dupl` recurses into sub-directories, so the sqlc and `gen/` files were scanned too and removed from the totals with a small script (groups mixing generated and hand-written code were trimmed to the hand-written side).
- `npx --yes jscpd@4.0.5 --min-lines 15 --ignore "**/gen/**,**/node_modules/**" --reporters json,console web/src e2e/tests`; second pass `--min-lines 6 --min-tokens 60 --format typescript --ignore "...,**/*.spec.ts,**/*-testing.ts" web/src/app/pages/live-session web/src/app/core`. Both with `npm_config_ignore_scripts=true`.
- `deadcode ./cmd/...`, `deadcode -test ./cmd/...`, `deadcode -test ./...` (golang.org/x/tools v0.52.0).
- `npx --yes knip@5 --config <scratch>/knip.json --no-progress --reporter compact` (knip 5.88.1) inside `web/`.
- Own scripts (Python, standard library) for dupl/jscpd/knip/deadcode summaries, proto message/field-set comparison, orphan `.html`/`.scss` check, generated-line counts per message.

Failures and workarounds:
- `go run golang.org/x/tools/cmd/deadcode@latest ./cmd/...` first failed: x/tools v0.52.0 needs Go ≥ 1.26, the go command switched to go1.26.9, which then refused the module (`package requires newer Go version go1.27`). Fix: `GOTOOLCHAIN=go1.27.2 go install golang.org/x/tools/cmd/deadcode@latest` and run the binary.
- `deadcode -test ./cmd/...` only roots the tests of the `cmd` packages, so it produced the same list; `-test ./...` is the meaningful one.
- `knip` with the repo's own setup failed: `web/eslint.config.js` needs `@eslint/js` and there is no `node_modules` (installing was out of scope). Used a minimal config (entries `src/main.ts`, `src/**/*.spec.ts`; `eslint`, `angular`, `vitest`, `jest`, `playwright` plugins off). Its "unused dependencies" and "unlisted binaries" sections are artefacts of that and are ignored.
- `jscpd --output` did not write a JSON report in this setup; clones were parsed from the console output (pairs only, so cluster sizes are pairwise).
- `buf lint` and `buf breaking` were not run; claims about them come from `proto/buf.yaml` and the buf documentation.

Not covered: runtime behaviour, test results, `maps`, `characters`, `progression`, `campaignpackage` internals beyond what the tools flagged, and `cmd/srdimport`.
