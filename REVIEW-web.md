# REVIEW-web: Angular app after the finish-line refactor

Branch `review/finish-web`, never merged. Read-only review of `web/src/app/**`, its specs and `e2e/`. History visible in this clone starts at #403 (shallow), so "added today" means added after `3265aab`.

## Verdict in five lines

- **No high-severity finding.** Four reviewers diffed every split against `<sha>^` (live-session, live-stream, combat-view, character-editor, character-sheet, level-up, class-editor, map-editor, map-view, area-map, trap-draft, image-generate, puzzles). Effect order and dependencies, injection context of effects moved into plain classes, template members and `OnPush`/zoneless reads are equivalent. The few differences are listed as low findings.
- **Mechanical gates, measured here:** `npx tsc --noEmit -p tsconfig.app.json` clean; `npx eslint .` clean (complexity 15, max-lines 400 code lines per app file); `npx ng build` initial total **481.40 kB** raw / 133.73 kB transfer (budget 500 kB). Nothing in `web/src` (outside specs and `gen/`) touches `localStorage`, `sessionStorage`, IndexedDB or `document.cookie`; `no-web-storage.spec.ts` guards it.
- **The metric moved the size, not the weight.** App code (non-spec, non-gen) grew by 8.6k lines net since `3265aab` (+34.3k/-25.7k) and the specs by +32.9k lines (277 files). 47 app `.ts` files are still over 400 physical lines (the gate counts code lines only). Templates are not capped: `character-editor.html` is 846 lines, `combat-view.html` 645.
- **The recurring cost is indirection**: abstract `@Directive()` chains and `extends` chains used only to get under 400 lines (`CombatView` 5 levels, `CombatClient` 4 levels), `(services, state, parts)` bags, forwarder methods and alias fields that exist for specs, compat re-exports.
- **`eslint-suppressions.json` still holds 452 files / 372 `prefer-on-push-component-change-detection` suppressions and 450 `no-magic-numbers`**; the split did not retire them, so "OnPush not broken" is true only because most components were never OnPush (the app is zoneless and signal-driven, so it works).

## Counts

| Category | high | medium | low |
|---|---|---|---|
| behaviour | 0 | 1 | 4 |
| docs | 0 | 2 | 9 |
| duplication | 0 | 8 | 17 |
| readability | 0 | 17 | 38 |
| **total** | 0 | 28 | 68 |

## Findings (sorted by severity)

Each entry: `[category/severity]`, file:line, problem, fix, whether the fix changes behaviour, whether a test guards it. Area of origin in brackets: live = live-session + core/play/combat/puzzles; combat = pages/live-session/combat; editors = character-editor/sheet/level-up/content; maps = maps, map-view, area-map, image-generate and the rest.

### web-1 [behaviour/medium] (maps)

web/src/app/core/connect/transport.ts:48 (token at web/src/app/core/auth/session-refresher.ts:13, provider at web/src/app/app.config.ts:23) — `fe76ca5` replaced `injector.get(AuthService)` (providedIn root) with `injector.get(SESSION_REFRESHER)`, a token that has no default factory and is provided only in `appConfig`. `CONNECT_TRANSPORT` is `providedIn: 'root'`, so any injector built without `appConfig` (a spec using the real transport, a standalone bootstrap, a future second app config) now throws `NullInjectorError` inside the interceptor's `catch`, replacing the original `unauthenticated` ConnectError the caller was about to handle. Nothing guards the `useExisting: AuthService` binding itself: `transport.spec.ts` passes a hand-made injector, `app.spec.ts` stubs AuthService. Fix: (a) `injector.get(SESSION_REFRESHER, null)?.refresh()` so a missing binding degrades to "no refresh", and (b) one spec in `app.config` / `app.spec.ts` asserting `TestBed.inject(SESSION_REFRESHER)` is the `AuthService` instance. Behaviour change: only in an injector without the binding (production unchanged). Guarded by test: no (transport.spec.ts uses a fake injector).

### web-2 [readability/medium] (live)

`live-session-loader.ts:21` + `live-session.ts:56-63` — loader takes `scheduleVision: () => this.events.scheduleVision()` and events takes the loader: a cycle closed with a lazy closure, built in the component's field list. `scheduleVision` (a debounce on `state.visionTimer` + `parts.fog.refresh` + `state.visionTick`) uses nothing of the events class. Fix: move `scheduleVision` into `LiveSessionLoader` (it already reads and writes the same state) and delete the constructor callback; events calls `loader.scheduleVision()`. Behaviour change: no. Guarded by test: yes (live-session.spec covers vision debounce).

### web-3 [readability/medium] (live)

`live-session-*.ts` (11 files, 1,700 lines for a page that was one file) — everything takes `(services, state, parts[, view, loader])` and every method opens with `const { state, parts } = this`; `LiveSessionServices` is a 22-field bag of which each class uses 2-5; `LiveSessionEffectsContext` bundles 6 members to call five effects once. Concrete merges: (a) delete `live-session-pure.ts`: `mageArmorClassOf` + `COMBAT_REREADS` + `COMBAT_RETRY_MS` go to the loader (only user), `highlightsSubOf` into the view (only user), `VISION_DEBOUNCE_MS` next to `scheduleVision`, `notesAria` into the component; (b) `live-session-services.ts` + `live-session-parts.ts` into one `live-session-deps.ts` (parts are built from services and never used apart); (c) drop `LiveSessionEffectsContext` and pass `wireLiveSessionEffects(services, state, parts, view, loader, notesBar, load)` positionally like the other classes. Behaviour change: no. Guarded by test: yes (live-session.spec, 2,490 lines).

### web-4 [readability/medium] (live)

`live-session-state.ts:97-135` vs `live-session-view.ts` — the split by "state holds signals, view derives" is not kept: `LiveSessionState` carries `ownSheet`, `ownArmorClass`, `trapSkills`, `playerNames`, `viewAsPeople`, `cluePlayers`, `finderRows`, `effectsTick`, `connection`, `lastUpdate`, `isMaster`, `ownVitals`, `ownCharacterId`, `seeingFamiliar` (14 computeds) while its header says "the page's own state, with no service in it" and `LiveSessionView` header says it holds what the page derives. Plus plain mutable fields on it (`generation`, `leftSeq`, `mapEvents`, `imageEvents`, `callingTimer`, `visionTimer`, `listedCampaign`, `loadedSheetFor`, `partyInfoIds`, `campaignLoad`) that are bookkeeping of the loader/events, not page state. Fix: move the five list/sheet computeds that only the template reads (`playerNames`, `viewAsPeople`, `cluePlayers`, `finderRows`, `trapSkills`) to the view; move the counters/timers/promise into private fields of the class that owns them (`generation`, `leftSeq`, `campaignLoad`, `loadedSheetFor`, `partyInfoIds` -> loader; `mapEvents`, `imageEvents`, `visionTimer`, `callingTimer` -> events, with `onDestroy` clearing through a `dispose()`), which also removes the cross-class writes `state.generation` seen in 14 places. `isMaster`/`ownVitals`/`ownCharacterId` stay (used by every class). Behaviour change: no. Guarded by test: yes.

### web-5 [readability/medium] (live)

`live-session-events.ts:292-310` — `takenBack` and `familiarStopped` are not stream news (the template calls them from child outputs: `events.takenBack`, `events.familiarStopped`, `events.showMap`). `LiveSessionEvents` is documented as "opens [the stream], and does what each of its news asks". Fix: move `takenBack` (leftSeq + leftImages) next to `reloadLeftImages` in the loader, `familiarStopped` into the loader too (it only edits vitals, refreshes the map and schedules vision); keep `showMap` where it is (the stream calls it). Behaviour change: no. Guarded by test: yes.

### web-6 [readability/medium] (live)

`core/combat/combat-client*.ts` — `CombatClient` (367 lines) extends `CombatAttackCalls` (316) extends `CombatMoveCalls` (302) extends `CombatTurnCalls` (208) extends `CombatClientBase` (39): a four-deep inheritance chain used only to get under the 400-line cap, with `protected client`/`keyed`/`sending` as the glue. A reader looking for `rollAttack` opens up to five files; the order of the chain is arbitrary (Attack sits above Move). `export * from './combat-client-types'` and `export { attackResult }` re-exports hide where things live. Fix: either compose (`CombatClient` holds `attack = new AttackCalls(base)`, etc., and screens call `client.attack.roll(...)`; larger change) or at least flatten to two layers (`CombatClientBase` + one file per feature, mixed by composition of plain functions taking `(base, ...args)`), and import `newKey`/types from their real modules. Behaviour change: no. Guarded by test: yes (`combat-client.spec.ts`, `combat-client.rolls.spec.ts`).

### web-7 [readability/medium] (live)

`core/combat/combat-log-hp-cast.ts:79-138` — `poolCard` is again one function with `let math = ''` and an inline row builder and a 7-line template-string summary. Before cae6f1c the same code (already shortened by 05f90e5, #432) had `poolRow`, `poolMath`, `poolSummary`; cae6f1c re-wrote the file from an older shape and dropped them, so the work of #432 is undone while the file was moved. Same file renamed `hpTargetText`/`hpTargetResult` to `masterTargetText`/`masterResult`/`hitPointFacts` for no gain. Fix: restore `poolRow(spell, t, condition)`, `poolMath(fx, affected)`, `poolSummary(roll, physical, rows)` from `cae6f1c^:web/src/app/core/combat/combat-log.ts` lines 550-600 (same behaviour, tested by `combat-log.branches.spec.ts:451-572`). Behaviour change: no. Guarded by test: yes.

### web-8 [duplication/medium] (live)

`core/combat/combat-log-effects.ts:108-111` — `effectEndedText` rebuilds `ctx.players.has(label) ? \`de ${label}\` : \`${article(label)==='a'?'da':'do'} ${label}\`` which is exactly `ofSubject(label, ctx)` in `combat-log-words.ts:24-28` (same split created `ofSubject` but this caller kept the inline copy). Related copies of "da/do by article": `combat-log.ts:91` (`ofThe`, private), `move-plan.ts:143` (`ofThe(labels[])`, exported, different signature, same name), `area-text.ts:171`, `contest-view.ts:70`, `creature-names.ts:70`, `joint-turn.ts:353`, `summon-result.ts:34`. Fix: `ofSubject` in `effectEndedText`; replace `combat-log.ts` `ofThe` by `ofSubject(name, NO_CONTEXT)`; rename `move-plan.ts` `ofThe` to `ofEach` or reuse a `core/format` helper. Behaviour change: no. Guarded by test: yes (`combat-log.effects.spec.ts`, `hp-spells-log.spec.ts`).

### web-9 [duplication/medium] (live)

first-letter-up helper written 7 times: `combat-log-words.ts:19 upFirst`, `contest-view.ts:84 upFirst` (exported), `reaction-view.ts:65 capitalized`, `opportunity.ts:89 capitalize`, `opportunity/opportunity-card.ts:268`, `attack-sheet/attack-result.ts:140`, `effects/effects.ts:33 capitalized`, plus `creatures/bestiary-format.ts:5`, `character-sheet-map.ts:128`. Fix: one `upFirst` in `core/format/text.ts` (next to `tight`/`joinDots`), delete the rest. Behaviour change: no (all are `charAt(0).toUpperCase()+slice(1)`; verify `bestiary-format` takes a word the same way). Guarded by test: yes.

### web-10 [duplication/medium] (combat)

`attack-sheet/attack-questions.ts:70-139`, `cast-sheet/cast-questions.ts:81-161`, `reaction-sheet/reaction-flow.ts:210-265` — `answerLucky` and `answerInspiration`/`answerDie` are written three times with the same skeleton: guard `!offer || busy`, `busy.set(true)`, `error.set('')`, `try`, `renew()` the key, `settle`, animate, `catch` -> `classResourceErrorMessage`, `finally`. `AttackState.run` (`attack-state.ts:65`) and `CastState.run` (`cast-state.ts:41`) exist to do exactly this, but the question classes and `ModeRequests.ask/cancel` (`attack-sheet/mode-requests.ts:68-122`) do not use them. `AttackRolls` uses `s.run`, `AttackQuestions` right next to it does not. Fix: one shared helper, e.g. `runBusy(busy, error, work, fail)` in `shared/sheet/` or `core/combat/`, used by the 2 state classes, 3 question classes, `ModeRequests` and `ReactionFlow`. Delete the two identical `run` methods. Behaviour change: no. Guarded by test: yes (`attack-questions.spec.ts`, `cast-questions.spec.ts`, `reaction-sheet.flows.spec.ts`, the `*.branches.spec.ts` files).

### web-11 [duplication/medium] (combat)

`attack-sheet/attack-state.ts:65` and `cast-sheet/cast-state.ts:41` — `run()` is byte-identical in both. `npc-card/pending-damages.ts:135` has a third copy and `contest-master/contest-card.ts:383` and `surprise-card/surprise-card.ts:160` have two more `run(what, call)` variants. Fix: fold into the helper above. Behaviour change: no. Guarded by test: yes (sheet specs).

### web-12 [duplication/medium] (combat)

The same "flag + subscribe(complete/error) clears it" pattern appears 5 times, with inconsistent error handling: `combat-sheets.ts:78-96` (`attackOpen`, handles `error`), `contest-host.ts:145-153` (`sheetOpen`, handles `error`), `reaction-sheets.ts:69-86` and `:105-111` (`reactionOpen`, `complete` only). If the sheet observable errors, `reactionOpen` stays true forever. The effect in `combat-view.ts` (reaction window auto-open) and `ownEffectSave` both read it, so no reaction sheet would ever open again. This was already so before 659a114 (`659a114^:combat-sheets.ts:474,499`), so it is not new, but the split left four copies and only the reaction one is unsafe. Fix: one `trackOpen(flag, obs)` helper that clears on `complete` and `error` (use `finalize`). Behaviour change: yes, small (the flag is cleared on error). Guarded by test: no (no spec makes the sheet observable error for reactions).

### web-13 [readability/medium] (combat)

`combat-view-inputs.ts`, `combat-view-state.ts`, `combat-view-derived.ts`, `combat-view-parts.ts`, `combat-view-flows.ts` — five abstract `@Directive()` classes chained by `extends` so that `CombatView` stays under 400 lines. None is ever used as a directive and there is exactly one concrete subclass. A reader of one template binding hops across up to 5 files, and the dependency direction is hidden because every member is `protected` and visible to every later layer. The split that worked is the one that made plain classes with explicit contexts (`CombatCalls`, `CombatReads`, `MoveFlow`, `ContestHost`, `CombatSheets`). Fix: stop adding layers; move the remaining members into plain classes the same way. First candidates: the notice and concentration computeds from `-derived.ts` into a `CombatNotices` class, and the death/rage/initiative calls from `-flows.ts` into the `CombatCalls` owner. Then collapse `-inputs`+`-state`. Behaviour change: no (but templates and about 15 spec files read the members off the component, so it is a large edit). Guarded by test: yes (`combat-view.*.spec.ts`).

### web-14 [readability/medium] (combat)

`combat-view-flows.ts:380-428` — 15 one-line pass-throughs whose only job is to hide `sheets.actions.X` and `moves.X` from the template: `askFamiliarEyes`, `openTransform`, `openDoor`, `describeSpell`, `openCast`, `openMasterArea`, `openConditions`, `openEffectSave`, `adjustNpc`, `openSpend`, `drop`, `confirmMove`, `confirmJump`, `disengage`, `closeFullPage`, `openMove`, `openCreatureMove`. `combat-view-parts.ts:79-95` adds 14 `protected readonly xxx = this.moves.xxx` alias fields (`moverId`, `dropStart`, `moveError`, `moveNote`, `lockedDoor`, `forcedMove`, `forcedNote`, `moveJump`, `reachOn`, `moveOptions`, `canDisengage`, `moverJumps`, `masterReach`, `reachSwitch`). The template could say `moves.confirmMove($event)` and `moves.moverId()`. Fix: bind `sheets`/`moves` directly in `combat-view.html` and delete the wrappers and aliases. About 17 of these methods are called by name in `combat-view.*.spec.ts`, so those specs have to be updated. Behaviour change: no. Guarded by test: yes.

### web-15 [duplication/medium] (editors)

web/src/app/core/levelup/levelup-abilities.ts:12 — `WIRE_OF_ABILITY` is the same table as `ABILITY_FROM_GEN` at core/levelup/levelup-flow.ts:231 (same six entries, same type shape), added today beside it. Two more copies exist as `Record<GenAbility, AbilityKey>`: pages/character-editor/character-editor-catalog-map.ts:87 and pages/character-sheet/character-sheet-map.ts:91 (both new files today, copied out of the old live sources). Fix: export one `ABILITY_FROM_GEN` from core/characters (or keep levelup-flow's) and import it in all four places; delete the three copies. Behaviour change: no. Guarded by test: yes, levelup-draft.spec / character-sheet-source-vm.spec / character-editor-source.rpc.spec cover the mapped values.

### web-16 [duplication/medium] (editors)

`SavingState` `{idle}|{saving}|{error,message}` is declared four times: pages/character-editor/editor-state.ts:53, pages/character-sheet/character-sheet-writes.ts:11 (new today, exported and imported back by character-sheet.ts:59), pages/character-sheet/story-panel/story-panel.ts:11, pages/character-sheet/master-notes/master-notes.ts:15. Fix: one exported type in core/ (for example core/connect/saving-state.ts) and import it; `IDLE` in character-sheet-writes.ts moves with it. Behaviour change: no. Guarded by test: compile only.

### web-17 [readability/medium] (editors)

web/src/app/pages/character-editor/character-editor.ts:281-345 (and 346-373 for draft/server) — about 84 lines of `protected readonly x = this.views.x / this.picks.x / this.server.x / this.draft.x` whose only job is to give the template the old member names; the real logic moved out and this file is now a re-export table (495 raw lines). Same pattern in pages/character-sheet/character-sheet.ts:164-190 (29 alias lines for `SheetWrites`). Fix: make `views`, `picks`, `spellPicks`, `server`, `draft`, `saving` `protected readonly` and let the templates read `views.blocks()`, `picks.toggleSkill`, `server.pendingSteps()` and so on; delete the alias block. Same for SheetWrites: `writes.confirmingDeath()` in the template. Behaviour change: no. Guarded by test: character-editor.spec / character-sheet.spec render the template; strict templates catch a missed rename at compile time.

### web-18 [readability/medium] (editors)

web/src/app/core/levelup/levelup-draft.ts:61-100 and 102-137 — 31 `readonly effective: LevelUpLists['effective']` declarations plus 31 `this.effective = lists.effective` assignments (about 65 lines) that re-export the members of `LevelUpLists` and `LevelUpAbilities` so callers do not change. A pass-through chain: draft, lists, computed. Fix: expose `readonly lists` and `readonly abilities` and migrate callers (`draft.cantripItems()` becomes `draft.lists.cantripItems()`), or make the picks a base class that `LevelUpLists` extends. Behaviour change: no. Guarded by test: levelup-draft.spec, levelup-draft-edges.spec, level-up.spec.

### web-19 [readability/medium] (editors)

web/src/app/core/levelup/levelup-missing.ts:12-43 (`MissingSource`) and levelup-picks.ts:~95 (`PickedSource`) — mirror interfaces of 25 and 13 members that list exactly the members of `LevelUpDraft`, which is the only thing passed in (`missingChoices(this)`, `anyPicked(this, ...)`). pages/level-up/level-up-gives.ts takes `LevelUpDraft` directly with `import type`, so the circular import is not a problem. Fix: replace both interfaces with `import type { LevelUpDraft }`. Behaviour change: no. Guarded by test: levelup-draft.spec (missing and dirty paths).

### web-20 [docs/medium] (editors)

web/src/app/pages/character-editor/character-editor.ts:76-133 — the class doc was copied unchanged and is wrong now. It names `filteredCantrips`/`filteredSpellsKnown`/`filteredSpellsPrepared` (they exist nowhere in web/src), `availableCantrips`/`availableSpells` (now `EditorCatalog`), and `toggleExpertise`/`customBackgroundSkills` as members of this class. Lines 105-113 say `feature_choice_keys` "has no field at all" and "survives a save unchanged" and that the form "now covers" the rest. False: the Escolhas step edits `featureChoiceKeys` (editor-picks.ts:25) and `buildFullValue` sends it (full-draft.ts:98), so the save overwrites it (sheet-map `{...rest, ...init}`). History words: "now", "integrator fix, phase 2b". Fix: rewrite the doc to the six plain classes and the template contract; delete the three paragraphs about skills, no-data-loss and typed keys (they belong on `mergeFullSheetInit` and `EditorPicks`, where the first already has them). Behaviour change: no. Guarded by test: no.

### web-21 [docs/medium] (editors)

docs/architecture/characters-web.md:25-37 — the file table and the paragraph "State several parts share is in plain classes ... `SpellPicks`, `EditorLoader` and `SaveFlow`" were not updated by cae6f1c: `editor-catalog`, `editor-picks`, `editor-draft`, `editor-server-draft`, `editor-submit`, `editor-opening`, `editor-spell-details`, `editor-core`, `character-editor-sheet-map`, `character-editor-catalog-map` are missing; "the component is the composition and the members its template reads" is only true because of the alias block. The level-up section (line ~58) lists `LevelUpDraft` as holding "what is missing, and the LevelUpChoices to send" but not `levelup-lists/abilities/missing/picks/request` or `LevelUpReader`/`LevelUpFocus`. Fix: add the rows. Behaviour change: no. Guarded by test: no.

### web-22 [readability/medium] (editors)

Compat re-exports kept "so the specs and old imports still work":
  - core/content/spell-draft.ts:26-31 `export *` x3. The comment says "keeps the old import path for all of them", which is a history word.
  - core/content/content-read.ts:25-33
  - core/content/content-testing.ts:21
  - core/levelup/levelup-draft.ts:37 (`AsiMode, HpCard, Rolled`) and levelup-summary.ts:23
  - pages/level-up/level-up-session.ts:24 (`newSpellsTitle`, `GiveRow`: only specs import them from there)
  - pages/character-editor/character-editor-source.live.ts:58-64 and pages/character-sheet/character-sheet-source.live.ts:23 ("the specs import them from here")
  - shared/image-generate/image-generate-dialog.ts:55

  Fix: point the importers at the real module and delete the re-exports. For the specs-only ones (the two source.live files, level-up-session) the spec should import `character-editor-sheet-map`, `character-sheet-map`, `level-up-gives`. Behaviour change: no. Guarded by test: compile.

### web-23 [duplication/medium] (editors)

"toggle a key in a bounded set" is written five ways in the area: pages/character-editor/spell-picks.ts:6 `toggledIn`, skill-picks.ts:45 `toggledCapped`, core/levelup/levelup-picks.ts:23 `flip` (set signal plus max, with the `max===1` replace rule), pages/content/class-editor/class-editor-model.ts:154 `toggled` (array), and `toggledOption` in core/resources/metamagic.ts:70. One `toggledSet(set, key, max?)` would serve the first three. In addition `toggledIn` lives in spell-picks.ts but skill-picks.ts and editor-picks.ts import it from there: a generic helper in a spell-named file, and skill-picks to spell-picks to nothing is an odd dependency direction. Fix: move `toggledIn`/`toggledCapped` to one `core/format/sets.ts` (or similar) and let `flip` call it. Behaviour change: no, if `flip`'s `max===1` rule is kept as a parameter. Guarded by test: yes (skill-picks.spec, spell-picks.spec, levelup-draft.spec).

### web-24 [readability/medium] (editors)

character-editor loader, opening and callbacks. `EditorLoader` (editor-loader.ts) reaches the page through `reset/created/edited` callbacks (editor-loader.ts:36-42), which call `EditorOpening` (editor-opening.ts, 99 lines), which has its own `forget` callback back into `server`/`saving` (character-editor.ts:256-270). One flow (route to read to fill the form) is spread over three files plus 30 lines of wiring in the page. Together the loader and opening are about 308 lines. Fix: merge `EditorOpening` into `EditorLoader` (loader writes `state`, `abilityTable`, the forms and picks directly through the core), keep `forget` as the single callback. Drops `reset/created/edited` from the context and `EditorOpeningHost`. Behaviour change: no. Guarded by test: editor-loader.spec, character-editor.spec, branches.spec.

### web-25 [readability/medium] (maps)

web/src/app/pages/maps/map-editor/map-editor.ts:373,392-402,409,415,444,450,454,478,482,488,492 — 14 methods that only forward to a controller (`addToken`→`tokens.add`, `saveAndContinue`/`discardAndContinue`/`keepEditing`→`sel.*`, `onMoved`→`moves.move`, `save`/`remove`→`pointSaves.*`, `toggleTokenHidden`/`removeToken`→`tokens.*`, `onRoomOutline`/`reloadDungeon`→`dungeonCtl.*`, `onForgotten`/`setViewAs`/`viewAsGone`→`viewAsCtl.*`, `refreshFlags`→`staleFlags.emit`). Each costs a hop and a name (`viewAsGone` vs `gone`, `onForgotten` vs `forgotten`). Fix: make the controllers `protected readonly` and bind the template directly (`tokens.add($event)`, `sel.keepEditing()`, `moves.move($event)`, `viewAsCtl.set(null)`), keep only methods with real logic (`setMode`, `applyMode`, `onEmptyClick`, `goToGrid`). Cuts ~60 lines. Behaviour change: no. Guarded by test: map-editor.spec.ts / map-editor.flows.spec.ts drive the DOM, so they keep guarding it.

### web-26 [readability/medium] (maps)

web/src/app/pages/maps/map-editor/map-editor.ts:133-145,189-193,210-231 with editor-moves.ts:97, editor-tokens.ts:16, editor-point-saves.ts:193, editor-view-as.ts:103, editor-dungeon.ts:185, editor-selection.ts:8 — six `*Host` interfaces plus a shared `ctx` object, each re-declaring `api/state/campaignId/mapId/message`; `EditorPointSavesHost` has 10 members including the whole `EditorSelection`. The host objects are built by spreading `ctx` and adding closures back into the component, so reading one flow means opening 3 files. `EditorMoves` (94 lines) and `EditorTokens` (91 lines) take the same host fields and both manage "items on the map". Fix: merge `EditorMoves` + `EditorTokens` into one `EditorItems` (about 170 lines, one host); let `EditorPointSaves` take `EditorSelection` as a plain constructor argument instead of through a host object. Behaviour change: no. Guarded by test: map-editor.flows.spec.ts (tokens, moving).

### web-27 [readability/medium] (maps)

web/src/app/pages/maps/map-editor/editor-point-saves.ts:272-285,229,253 — `EditorPointSaves` writes `sel.panelError`, `sel.justCreated`, `sel.selection`, `sel.dirty` from outside, re-doing by hand part of `EditorSelection.move()` (editor-selection.ts:51, which clears the same fields) with `justCreated=true` in between. Two places now decide what "selection changed" resets, and `remove()` calls `sel.dirty.set(false)` + `sel.move(null)` while `create()` bypasses `move`. Fix: add `EditorSelection.select(id, { created })` (clears pending/panelError/lightReach, sets justCreated) and `EditorSelection.reset()` for remove; make the signals `private` behind it. Behaviour change: no if `select` keeps today's order (create does not clear `lightReach`/`pending`; check before folding). Guarded by test: map-editor.flows.spec.ts "new points", "saving a point".

### web-28 [readability/medium] (maps)

web/src/app/shared/image-generate/image-generate-dialog.ts:100-113,144-150,159-173 — about 16 one-line forwarders (`cancelRun`, `keepWaiting`, `requestClose`, `stopAndClose`, `askShow`, `cancelShow`, `askUse`, `cancelUse`, `confirmUse`, `show`, `shown`, `pickChain`, `patch`, `removeReference`, and 10 `protected readonly x = this.result.x` aliases) so the template can keep its old names after `RunFlow`/`GeneratedResult`/`ImageFormState` were extracted. The component's class is now a name-mapping layer. Fix: expose `protected readonly flow`, `result`, `formState` and change the template to `flow.cancel()`, `result.askShow()`, `formState.patch($event)`; the template is 174 lines and these names appear about 30 times. Also `RunFlowDeps` (run-flow.ts:12) has 8 callbacks (`stage, started, status, done, ended, finish` plus api/campaignId/injector) which is the dialog's own method set passed back in; `GeneratedResult`'s deps struct (image-result.ts:170) carries one callback. Fix for those: let `RunFlow` emit `output`-like signals/promises (`phase` signal already exists on `ImageRun`) and keep only `stage()` and `finish()`. Behaviour change: no. Guarded by test: image-generate-dialog.spec.ts (1108 lines, DOM-driven).

### web-29 [behaviour/low] (live)

`web/src/app/pages/live-session/live-session-events.ts:106` — `onVisionChanged` (in `mapHandlers()`) calls `loader.loadCombat(state.generation)`; the old code used the `generation` captured when the stream was opened. `mapHandlers()` was not given `generation` (only `sessionHandlers` and `combatHandlers` are), so the stale-load guard now reads the live counter at event time and can never be "older". Today unreachable (a new `begin()` stops the old stream and `LiveStream.connect` drops messages of a replaced controller), so no visible bug, but the guard the rest of the file keeps is silently gone for this one handler. Fix: `mapHandlers(generation)` and pass `generation`, same as `combatHandlers`. Behaviour change: no (in practice). Guarded by test: no (live-session.spec has the `vision_changed` case but not an older-generation one).
  Certainty: high on the diff, high on "unreachable now".

### web-30 [behaviour/low] (editors)

pages/character-editor/character-editor.ts:218 — the three effects of `EditorServerDraft` are now created in a field initializer, before the constructor body (character-editor.ts:411, 426) that creates the subclass-disable effect and the starting-XP effect. Before cae6f1c the order was subclass-disable, HP, choices, prune-picks, XP; now HP, choices, prune-picks, subclass-disable, XP. I found no dependency between them (`build()` reads `getRawValue()`, which includes disabled controls), so no visible change. Fix: none needed; if one wants the old order, create `EditorServerDraft` in the constructor after the first effect. Behaviour change: no, as far as I can see. Guarded by test: no.

### web-31 [behaviour/low] (editors)

pages/level-up/level-up.ts:164 — `this.route.paramMap.pipe().subscribe(...)` is never unsubscribed (the empty `pipe()` is noise); `CharacterEditor` does the same with `takeUntilDestroyed` (character-editor.ts:431). Not caused by the split, but the reader now makes the stale-read guard (`reads` counter) the only protection if a param emission arrives after destroy: `LevelUpReader.load` has no destroyed check, unlike `EditorLoader.destroy()`. Fix: `takeUntilDestroyed(destroyRef)` and drop `.pipe()`. Behaviour change: no for live pages. Guarded by test: no.

### web-32 [behaviour/low] (maps)

web/src/app/core/maps/map-state.ts:96-98 — in `readAs`'s `catch`, a read that fails after any in-place edit (`edits !== this.edits`) returns silently. If that failure is `NotFound` (map deleted / hidden from this player), status stays `ready` and the stale map stays on screen until the next stream event; the success path in the same function re-reads in this case (line 88-90), the failure path does not. Pre-existing, not introduced by `2785ee9`, but `2785ee9` now makes callers `await` this path (`open()`/`refresh()` resolve as if the map were fine). Fix: in that branch do `return await this.read(mapId)` like the success path (bounded by the next edit anyway), or at minimum only swallow non-NotFound errors. Behaviour change: yes (a deleted map is noticed). Guarded by test: no (map-state.spec.ts covers stale success and the overtaken wait, not this branch). Certainty: medium (logic read, not run).

### web-33 [readability/low] (live)

`live-session.ts:44-66` — nine pass-through aliases (`mapState`, `trapBoard`, `toasts`, `fog`, `combat`, `scene`, `puzzles`, `notes` = `this.parts.x`; `ownEffects` = `this.view.ownEffects`) plus three one-line handlers that only forward (`sessionEndedHere` -> `loader.ended()`, `combatStarted` -> `combat.apply`, `retry`). The template reads `mapState`, `combat`, `scene`... 80 times; either bind `parts.mapState` in the html (consistent with `state.`/`view.`) or drop `parts`/`view` from the template surface and keep aliases, not a mix. Fix: template uses `parts.x`; delete the aliases; template calls `loader.ended()` directly. Behaviour change: no. Guarded by test: yes (the spec reads some of these aliases through `fixture.componentInstance` casts, would need the same rename).

### web-34 [readability/low] (live)

`live-session.ts:19-20` — `export { mageArmorClassOf }` and `export { CALLING_NOTICE_MS }` exist only so `live-session.spec.ts:88` can import from `./live-session`. Fix: import from `live-session-pure` / `live-session-events` in the spec (or the files they move to) and delete both re-exports. Behaviour change: no. Guarded by test: n/a.

### web-35 [readability/low] (live)

`core/combat/combat-log.ts:20` — `export { article }` re-exported from `format/article`, imported as `from './combat-log'` by 13 files (`creature-names`, `contest-view`, `move-plan`, `combat-view`, `contest-master`, `reaction-view`, `cast-flow-rows`, `summon-result`, `reaction-master`, `area-text`, `combat-notices`, `joint-turn`, `opportunity`): a pass-through that makes unrelated modules depend on the log module (and `cast-flow-rows` <- `combat-log` <- `combat-log-lines` <- `combat-options` chain). Same for the `logLine`/`monstersAddedText`/`pairOf` re-export block. Fix: import `../format/article` directly everywhere; keep only the barrel the screens use (`logGroups`, `latestLine`, ...). Behaviour change: no. Guarded by test: no (build only).

### web-36 [readability/low] (live)

`core/combat/cast-flow.ts:1-12, 45-48` — header still says "The sheet's logic is kept here as small pure functions"; since cae6f1c the file is half slot/target/dart logic and half a re-export barrel of `cast-flow-text` + `cast-flow-rows` (`damageDice, damageText, castSubtitle, saveAbility, castRows, dartLines, effectSentence, rollGroups, spentLine`, types). Importers: `spell-summary.ts`, `reaction-view.ts`, `reaction-prompts.ts` import `freeText`/`damageDice` through it. Fix: importers use the real file; rewrite the header ("slot rows, target rules, darts"); drop the barrel. Behaviour change: no. Guarded by test: no.

### web-37 [readability/low] (live)

`core/combat/combat-log-lines.ts:50-330` — 24 builders written `({ e, base }) => { return { ... }; }` (block body + single return) where the original was an expression. Pure noise from the split; `LineArgs` bundling `{e, base, encounterName, ctx}` for builders that use 1-2 of them. Fix: expression bodies (`({ e, base }) => ({ ...base, ... })`). Behaviour change: no. Guarded by test: yes.

### web-38 [readability/low] (live)

`core/play/live-session-source.live.ts:72` — `export { classifyLiveError, toShownImageVm, toVitalsVm }` re-exports kept so `hit-dice-sheet.ts:39`, `rest-card.ts:32`, `flexible-casting-sheet.ts:27` import `toVitalsVm` from the 219-line source class file (which drags `LiveSessionSourceLive` and its clients into those sheets' import graph). Fix: import `toVitalsVm` from `core/play/live-vitals-map`, delete the re-export. Behaviour change: no (bundle: slightly smaller). Guarded by test: no.

### web-39 [readability/low] (live)

`core/play/live-event-map.ts:13` — local helper named `signal` shadows the Angular `signal` concept used everywhere else in `core/play`; it builds a plain event, not a signal. Also `hiddenHitPending: signal('heartbeat')` maps a real wire event to "heartbeat" with no comment (it used to fall in the `default:` branch with a comment "still proof the stream is alive"; the intent of the explicit entry is lost). Fix: rename to `bare(kind)`, add `// nothing for the page to do; it still proves the stream is alive` on that entry. Behaviour change: no. Guarded by test: yes (`live-session-source.live.spec.ts`).

### web-40 [readability/low] (live)

`core/play/live-stream.ts:151-165` — `receive()` returns the strings `'more' | 'over'` for a boolean; `LiveStream` needs only "stop looping". Fix: return `boolean` (`true` = keep reading), or let `ended` throw nothing and `connect` check `this.status() === 'closed'`. Also `live-stream-dispatch.ts:51`: `ROUTES as Record<string, ...>` then `route?.()` means a `kind` of `'constructor'`/`'toString'` would call an `Object.prototype` member (kinds come from our own mapper, so theoretical): use `Object.hasOwn` or a `Map`. Behaviour change: no. Guarded by test: yes (`live-stream.dispatch.spec.ts:281` unknown kind).

### web-41 [readability/low] (live)

`shared/start-combat/start-combat-plan.ts:14-30, 98-105` — `BlockInput` (12 fields) and `JoinInput` (6) are one-call parameter structs built only at `start-combat-dialog.ts:220` and `:421`; `blockedReason` is correct (checked order against the old computed: map, grid, name, group, NPC, 40 cap are mutually exclusive in the same way) but the struct restates 12 computeds the dialog already holds. Acceptable if kept pure and tested; if you want less: make `mapBlock`/`entrantsBlock` the only exported rules and have the dialog pass the signals' values. Low priority. Behaviour change: no. Guarded by test: yes (`start-combat-plan.spec.ts`, `start-combat-blocked.spec.ts`).

### web-42 [readability/low] (live)

`pages/live-session/casting/cast-out-view.ts:12-85` — `casterRowsOf` (maps npc rows to `ChoiceRow` field for field) and `targetChoicesOf` (adds " (você)") are one-caller moves of two `.map(...)` bodies; `CastPicks` (6 fields) and `CastRequestParts` (9 fields) are context structs rebuilt from signals in `cast-out-sheet.ts` just to be passed to `missingOf`/`castRequestOf`. Side effect: `missing` now reads all six signals eagerly where the old computed read `slot`/`chosen` only on the branches that needed them (wider dependency set, same value). Fix: keep `diceOf`, `summonedNotice`, `endsConfirmText` (real text/logic); inline `casterRowsOf`/`targetChoicesOf` back, or make `ChoiceRow` the type `npcCasters`/`targetRows` already produce. Behaviour change: no (only recompute count). Guarded by test: yes (`cast-out-view.spec.ts`, `cast-out-sheet.spec.ts`).

### web-43 [readability/low] (live)

`pages/live-session/traps/trap-search-sheet/trap-search-sheet.ts:122-128` — two `inject(Injector)` fields (`lazyInjector` and `injector`) are the same object; pre-existing but the split left both. Also file is still 462 lines (> 400) because the split moved helpers but not the Lucky/Inspiration question handling (`holdFor`, `answerDie`, `answerLucky` copies in `effect-save-sheet.ts`, `scene-roll-sheet`, `PuzzleHints.answerDie/answerLucky`: four near-identical busy/try/finally state machines). Fix: one `injector` field; consider a shared `HeldRollQuestions` helper. Behaviour change: no. Guarded by test: yes.

### web-44 [readability/low] (live)

`core/puzzles/puzzle-play.ts:70-77, 124-127` — the 7 `hintBusy/hintTry/...` fields exist only to re-publish `this.hints.busy` etc.; pages read `play.hintBusy()` while the class under it is `PuzzleHints`. Pass-through. Also `private pendingId` (declared after its use, line 124) is the open puzzle's id, named like the `pending` counter. Fix: expose `readonly hints: PuzzleHints` and let pages read `play.hints.busy()`; rename `pendingId` -> `openId`. Behaviour change: no. Guarded by test: yes (puzzle-play specs, would need the same rename).

### web-45 [duplication/low] (live)

`core/puzzles/puzzle-play.ts:111-124` — `open()` resets `message/run/gone/loadError` but not `hints.offer`/`hints.lucky` (`PuzzleHints.reset()` clears busy/last/pair/extraDice only), so a Lucky or Inspiration question left open on puzzle A is still on screen when puzzle B opens. Pre-existing (old `open` did not clear `hintOffer`/`hintLucky` either), not caused by the split, but the split moved the reset into `PuzzleHints.reset()` where the omission is easy to see. Fix: `this.offer.set(null); this.lucky.set(null);` in `reset()`. Behaviour change: yes (bug fix). Guarded by test: no. Certainty: high that it is missing, medium that it is reachable (needs a hint question pending while the player navigates `?puzzle=`).

### web-46 [docs/low] (live)

spec headers that narrate the refactor: `core/play/live-stream.dispatch.spec.ts:1-4` ("Written green on the stream as it was before `connect` was split into its reconnect, its dispatch and its state updates"), `core/combat/combat-log.branches.spec.ts:1-4` ("the text the code wrote before `undoLabel` and the spell lines were split"). They describe a past state; the next reader needs what the file tests, not the history. Fix: "What each kind of message hands the page, and the lifecycle corners `live-stream.spec.ts` does not reach" / "Lines of the log not covered by combat-log.spec.ts". Behaviour change: no.

### web-47 [docs/low] (live)

`core/combat/reaction-autopass.ts:6` — "Vinicius, 10/10: an unanswered optional reaction passes after 30 s." Name and date in a doc comment; says nothing the constant does not. Fix: "An unanswered optional reaction passes after 30 s." Behaviour change: no.

### web-48 [docs/low] (live)

design-board / requirement ids kept in the new files (brief: "audit ids"): `live-session.ts:24` (`artboards E5-02 to E5-08`), `live-session-effects.ts:36` (`E9-09 4`), `live-session-events.ts:262` (`E9-08 G`), `live-session-parts.ts:30` (`E10-06`), `live-session-sheets.ts:66` (`E9-08`), `live-session-state.ts:67` (`W7-X`), `combat-log-hp-cast.ts:13` (`E8-03`), `combat-log.ts:9` (`E6-11, E6-14, E6-15`). 61 `MR-/RN-/E..-..` tokens in the 17 split files; MR/RN are product rules (keep), `E..`/`W7-X` board ids are not resolvable from the repo. Fix: delete the `E`/`W` ids, keep `MR-`/`RN-`. Behaviour change: no.

### web-49 [docs/low] (live)

`live-session-state.ts:14-15` header "(the page's own state, with no service in it)" is false for the timer handles and the `campaignLoad` promise; `live-session-effects.ts:12` "What the effects of the page need from it" documents a struct that merely re-lists `LiveSession` fields. Fix with the state/loader field moves above. Behaviour change: no.

### web-50 [duplication/low] (combat)

The `[{key:'d20-1', label:'Primeiro d20',...},{key:'d20-2',...}]` constant is defined 7 times: `reaction-queue/reaction-queue.ts:309`, `npc-card/npc-card-view.ts:27`, `cast-sheet/cast-mode.ts:12` (exported as `D20_FIELDS`), `attack-sheet/attack-sheet.ts:187`, `reaction-sheet/reaction-sheet.ts:191`, `check-roll-form/check-roll-form.ts:13`. Fix: export one `D20_PAIR_FIELDS` from `multi-roll/multi-roll.ts` and import it. Behaviour change: no. Guarded by test: no.

### web-51 [duplication/low] (combat)

`attack-sheet/attack-sheet.ts:60` and `attack-sheet/attack-sheet-view.ts:56` both declare `const REACTION_REACH_FT = 5`. The view copy is used by `attackTargetRows`. The component copy is used for `detail` and `rangeText`. `reaction-sheet/reaction-sheet.ts:73` and `core/combat/auto-pass.ts:18` both declare `AUTO_PASS_TICK_MS = 1000`. Fix: export one and import it. Behaviour change: no. Guarded by test: no.

### web-52 [duplication/low] (combat)

`AttackRolls.play` (`attack-sheet/attack-rolls.ts:204`) and `AttackQuestions.play` (`attack-sheet/attack-questions.ts:134`) are identical private helpers. `ReactionFlow.play` and `reaction-queue.ts:296` are close copies. Fix: give `RollAnimator` a `playIf(show | null)` method, or a shared function. Behaviour change: no. Guarded by test: yes (indirectly).

### web-53 [duplication/low] (combat)

`cast-sheet/cast-sheet.ts:279-294` (`describe()`) and `action-sheets.ts:55-67` (`spellDetailsData`) build the same `{namePt, load: async () => { details = await catalog.details(...); if (!details) throw ...; return spellDetailsFromGen(details) }}`. Fix: export a `spellDetailsData(catalog, campaignId, key, name)` from `shared/spell-details/` and use it in both. Behaviour change: no. Guarded by test: partly (`combat-view.sheets.spec.ts` for the view side; the `CastSheet` describe path is covered in `cast-sheet.spec.ts`, I did not verify the `load` throw).

### web-54 [duplication/low] (combat)

`attack-sheet/attack-mode.ts` and `cast-sheet/cast-mode.ts`: both compute `suggested`, `chosen`, `faceCount`, `combine`, `ready` through `modeStatus`/`orNormal`/`d20Count` with only the source of the targets and the `approval` literal differing (`'request'` vs `'blocked'`). Fix: leave as is if the 2 `approval` values must differ, otherwise extract the shared computeds. Behaviour change: no. Guarded by test: yes. Certainty: medium that merging pays off.

### web-55 [readability/low] (combat)

`combat-view-parts.ts:41-62`, `:67-84`, `combat-view-state.ts:20-60` — the `CombatSheets` context is 24 lines and `MoveFlow`'s 12, most of them `campaignId: () => this.campaignId()`, `isMaster: () => this.isMaster()`, `state: () => this.state()`, `diceMode: () => this.diceMode()`. `this.campaignId` and the other inputs are already `() => T` signals; `ContestHost` (same file, `:100-112`) passes `encounter: this.encounter` directly while the same file wraps the other 40. Fix: pass the signal itself (`campaignId: this.campaignId`) everywhere. Behaviour change: no. Guarded by test: yes. Also: `CombatSheetsContext` (`combat-sheets-context.ts`) is shared by `CombatSheets`, `ReactionSheets` and `ActionSheets`, but each uses a subset (`ReactionSheets` never reads `layers`, `catalog`, `casterClasses`, `mapState`; `ActionSheets` never reads `armorClass`, `diceMode` only for `secondWind`). One bundle for three consumers hides what each needs. Fix: give each class its own narrow `Pick<>` of the context. Behaviour change: no.

### web-56 [readability/low] (combat)

`combat-view-state.ts:81` — `lookingThroughFamiliar` is read only by `combat-view.state.spec.ts:1377`; no template or class uses it. Fix: delete it and the one assertion, or move the check into a `CombatReads` spec. Behaviour change: no. Guarded by test: yes (the one assertion).

### web-57 [readability/low] (combat)

`combat-view-flows.ts:410` `openSpend` is not in `combat-view.html` and nothing in non-spec code calls it (`combat-view-parts.ts:78` calls `this.sheets.actions.spend` directly). Only `combat-view.calls.spec.ts:1712` calls it. Same for `combat-view-flows.ts:435` `leave` (used by the template, so that one is fine). Fix: delete `openSpend`, change the spec to `sheets.actions.spend()`. Behaviour change: no. Guarded by test: yes.

### web-58 [readability/low] (combat)

`cast-sheet/cast-sheet.ts:264-271` `answerLucky` and `answerDie` — the template calls `questions.answerLucky/answerDie` directly (`cast-sheet.html:23-34`). The wrappers are called only from specs (`cast-sheet.branches.spec.ts`, `cast-sheet.lucky.spec.ts`, `cast-sheet.inspiration.spec.ts`). Same for the six alias fields at `cast-sheet.ts:180-186` (`offer`, `luckyOffer`, `cast`, `typing`, `error`, `rows`), under the comment "What the specs next to this one reach by name": not used in `cast-sheet.html` (grep for bare `offer()`, `cast()` etc. returns 0). `pickSlot` (`:259`) is a real template call but only forwards to `ch.pickSlot`. Production code carries members that exist for tests. Fix: make the specs read `sheet.questions.answerLucky`, `sheet.s.cast` etc., delete the wrappers, aliases and the comment; bind `ch.pickSlot($event)` in the template. Behaviour change: no. Guarded by test: yes.

### web-59 [readability/low] (combat)

`attack-sheet/attack-sheet.ts:311` `busy()` — template uses `s.busy()` everywhere; the method is called only by `attack-sheet.close.spec.ts:85`. `rollAttackTyped`/`rollAttackPair`/`rollDamageTyped` (`:319-336`) are template-used but one-line forwards to `rolls.*` with a changed argument shape; they could be inline in the template (`rolls.rollAttack({ face: $event })`). Fix: delete `busy()` (spec reads `cmp.s.busy()`), optionally inline the others. Behaviour change: no. Guarded by test: yes.

### web-60 [readability/low] (combat)

`npc-card/npc-card.ts:291-297` — `coverNote(t)` and `distance(ft, tooFar)` are wrappers that exist only to call `coverNoteOf` and `distanceText` from the template; `rollApp/rollTyped/rollTypedPair` (`:299-307`) are 3 one-liners over `rollAttack`. Fix: expose the pure functions as fields (`protected readonly coverNote = coverNoteOf;`) or use `roll({ inApp: true })` in the template. Behaviour change: no. Guarded by test: yes (`npc-card.view.spec.ts`).

### web-61 [readability/low] (combat)

`reaction-sheet/reaction-sheet.ts:354-359` — 6 `protected readonly concentrate = this.flow.concentrate.bind(this.flow)` fields next to normal methods (`use`, `pass`, `done`). Mixed styles for the same job, and `.bind` at construction freezes the method (a spy installed later on `flow.x` is not seen). Fix: call `flow.concentrate(...)` from the template, or write them as ordinary methods. Behaviour change: no. Guarded by test: yes (`reaction-sheet.flows.spec.ts`). Also `reaction-sheet.ts:107` and `cast-sheet.ts:110/116`: the lazy `Injector` field. `CastSheet` has two (`lazyInjector` and `injector`), both `inject(Injector)`; use one.

### web-62 [readability/low] (combat)

Name noise in the new files: `cast-sheet/cast-sheet.ts:107-180` exposes sub-objects as `s`, `ar`, `ch`, `md`, `tx`, `damage`, `caller`, `questions`; the template reads `ch.rows()`, `s.slot()`, `ar.flow`. `attack-sheet.ts` uses `s`, `dmg`, `modes`, `mode`. The `*-view.ts` files (`attack-sheet-view.ts`, `cast-sheet-view.ts`, `npc-card-view.ts`, `move-page-view.ts`, `contest-sheet-view.ts`) are pure function bags, not views, with `xxxOf` names (`damageDiceOf`, `brutalOf`, `fixedTextOf`, `statsOf`, `resultOf`, `reachOf`, `canMoveOf`, `goLabelOf`). Fix: rename the sub-objects to what they are (`state`, `area`, `choices`, `mode`, `texts`) and the files to `*-text.ts`/`*-rules.ts`, or put the functions next to the one class that calls them. Behaviour change: no. Guarded by test: yes.

### web-63 [duplication/low] (combat)

`npc-card/npc-card-view.ts:176` `attackShow` and `attack-sheet/attack-sheet-view.ts:325` `attackShow` — same name, same job (an attack d20 as a `RollShow`), slightly different options (NPC card always `withTotal: true` and no `told` guard for `UNSPECIFIED`). Two functions with one name in neighbouring files will drift. Fix: one `attackRollShow(label, roll, {told})` in `core/combat/`. Behaviour change: no if the options are kept. Guarded by test: yes (`npc-card-view.spec.ts`, `attack-sheet-view.spec.ts`).

### web-64 [duplication/low] (combat)

`combat-view-derived.ts:218-240` `leaveForm` and `move-flow.ts:344-371` `runMove` re-implement `CombatCalls.run` (`core/combat/combat-calls.ts:36-52`): busy guard, `busy.set(true)`, `error.set('')`, try, `combatErrorMessage`, `refreshAfter`, finally. `runMove` needs the full `MoveResult`, and `leaveForm` calls another client. Fix: give `CombatCalls.run` a generic form `run<T>(call, applyOf)` that returns `T`, so both reuse it. Behaviour change: no. Guarded by test: yes (`combat-view.calls.spec.ts`, `move-page.jump.spec.ts`).

### web-65 [docs/low] (combat)

`npc-card/npc-card.ts:176` — a stray doc comment: `/** The one on turn is at 0 hit points: nobody spends movement for them. */` sits directly above another doc comment (`/** "do Goblin", "da Brisa". */` on `ofLabel`); the first describes `down` (line 179) and was left behind when members were reordered. Fix: move it onto `down`. Behaviour change: no. Guarded by test: no.

### web-66 [docs/low] (combat)

`action-sheets.ts:157` `secondWind(own, e, name, economy)` hard-codes `actionKey: 'feature:second-wind'` while its only caller (`combat-view-parts.ts`, `useFeature`) already branches on that very key. Pass the key through, or name the method after the key constant. Also `reaction-sheets.ts:194` wraps `this.opportunity(offer)` in `untracked` inside a subscribe callback (not an effect; `rearm` is only called from the sheet's `subscribe`). The `untracked` does nothing there. Fix: drop it. Behaviour change: no. Guarded by test: no.

### web-67 [docs/low] (combat)

`cast-sheet/cast-sheet.ts:50-72` (class doc) still says "The keys (one for the cast, one for each damage) are made once", and `attack-sheet/attack-sheet.ts` doc says "It calls `RollAttack` and `RollDamage` itself". After the split those calls and keys live in `CastCaller`/`CastDamage` and `AttackRolls`. The docs describe behaviour of classes the reader no longer sees. Fix: one line each, pointing to the class. Behaviour change: no. Guarded by test: no.

### web-68 [readability/low] (combat)

`combat-view-testing.ts` (194 lines, imports `vitest` and `@angular/core/testing`) sits in `src/app/` and is not a `.spec.ts`, so `tsconfig.app.json` (`include src/**/*.ts`, exclude only `*.spec.ts`) type-checks it as app code. Nothing in the bundle imports it, so there is no runtime impact. Same for `core/combat/combat-testing.ts`. Fix: name it `*.testing.spec-helper.ts`? Better: add `src/**/*-testing.ts` to the `exclude`. Behaviour change: no. Guarded by test: no.

### web-69 [readability/low] (editors)

`EditorCore` (editor-core.ts) is spread into `EditorCatalogHost`, `EditorDraftHost`, `ServerDraftHost`, `EditorOpeningHost`, `EditorSubmitHost` (character-editor.ts:202-300), so every class receives `state, fullForm, basicForm, picks, spellPicks` and most use two or three of them (EditorCatalog never touches `basicForm`; EditorSubmit uses `basicForm` only for `markAllAsTouched`). `EditorSubmitHost` has 14 members, `EditorOpeningHost` 12. Fix: pass the 2-3 real dependencies, or have the plain classes receive the sibling objects they read (`picks`, `views`) and not the bundle. Behaviour change: no. Guarded by test: compile.

### web-70 [readability/low] (editors)

One-caller pass-throughs in character-editor: `EditorDraft.intersectWithAvailable` (editor-draft.ts:97, public, called 3 times from `build`, wraps `keepAvailable` with one argument); `EditorCatalog.sectionHeading` (editor-catalog.ts:345, one line over `sectionHeadingOf`) and `nameOfKey` (340, one line over `catalogNameOf`). Fix: call `keepAvailable`/`sectionHeadingOf` directly; make `intersectWithAvailable` a private local const in `build`. `classProblemsOf(extra, totalLevelProblem: string)` (full-draft.ts:106-118) takes the problem sentence only for its truthiness and then re-types "o nível total vai até 20" by hand, duplicating `MAX_TOTAL_LEVEL` in editor-catalog.ts:95; pass a boolean and use the constant. Behaviour change: no. Guarded by test: full-draft.spec, character-editor.branches.spec ("lists what the blocks still lack").

### web-71 [readability/low] (editors)

pages/content/content-entry/content-entry-model.ts:117-235 — about ten one-caller wrappers over a one-line computed body: `kindTagOf`, `usageOf`, `nounPhraseOf`, `archivedWordOf`, `subraceParentOf`, `replaceEntry`, `backLabelOf`, `headingOf`, `updatedLine`, `archivedDay`. They exist for size, not for a second use. Same style in class-editor-model.ts: `addedRequirement`/`removedRequirement`/`movedRequirement`/`typedRequirement` (one expression each), and `D6..D12` constants (lines ~262-270) added only to quiet the magic-number lint (`[6, 8, 10, 12]` with a doc comment reads better). Fix: inline the ones whose body is one expression back into the component unless the file limit needs them; keep `mergeRefreshed`, `entriesAfterSave`, `extrasErrors`. `NEW_TITLES` (line 33) is exported but used only inside the file. Behaviour change: no. Guarded by test: content-entry-model.spec, class-editor-model.spec.

### web-72 [readability/low] (editors)

shared/image-generate: (a) image-generate-dialog.ts:460 `private focusAfterStage(field?)` has the same name as the imported function it calls (line 49) with the injector and scroll closure bound; call the function directly through a one-liner or rename the method. (b) line 268 `this.referenceState.set('loading')` writes into the state `ImageReferences` owns (`this.refs.state`) right before `this.refs.load(mapId)` (line ~270) whose first statement is `this.state.set('loading')`; the line is dead. Delete it. (c) `RunFlowDeps` (run-flow.ts:9-25) is 9 callbacks into the dialog, and `started/status/done/ended/finish` each do one `signal.set`; consider passing the dialog's stage/status signals. (d) `GeneratedResult.changedMap` (image-result.ts:28) is a public mutable plain field read by the dialog on close. Behaviour change: no. Guarded by test: image-generate-view.spec covers only the pure functions; the dialog spec covers the flow.

### web-73 [readability/low] (editors)

pages/level-up: `ReaderView` (level-up-reader.ts:64-77) is a 10-callback context struct; `ready` reads as a flag, it is `firstScreen`. `LeaveFrom` is exported from level-up-focus.ts:4 but level-up.ts:91 and :318 retype `'top' | 'foot'`. `choiceRow` in level-up-gives.ts:48 recomputes `d.steps().indexOf(step) + 1` that `LevelUpSession.stepNumber` (level-up-session.ts:281) already holds (still used by hp-step.ts:78). Fix: import `LeaveFrom`; make `choiceRow` take `stepNumber` from the session, or make `stepNumber` a `LevelUpDraft` method used by both. Behaviour change: no. Guarded by test: level-up-session.spec ("lists what the level gives").

### web-74 [readability/low] (editors)

core/levelup: `missingIn` is two different functions: levelup-choices.ts:54 `missingIn(choices, drafts)` and `LevelUpDraft.missingIn(step)` (levelup-draft.ts:173), both called in the same file levelup-missing.ts:65 / level-up-gives.ts:133. `levelup-picks.ts` holds set helpers, hit-point adoption and `anyPicked` (not "picks"); `levelup-lists.ts` holds computed lists. Fix: rename the first to `openChoices`/`missingCount`; split `adoptedHitPoints` into the draft file or `levelup-hit-points.ts`. Behaviour change: no. Guarded by test: compile.

### web-75 [duplication/low] (editors)

pages/table-rules/table-rules-options.ts:13 `DICE_TITLES` repeats the titles of `DICE_OPTIONS` (line 144) word for word ('Todos rolam no app', 'Todos rolam os próprios dados', 'Cada jogador escolhe'). Fix: `titleOf(DICE_OPTIONS, d.diceMode)` in `ruleRows`/`styleNoteFor`; delete `DICE_TITLES`. Behaviour change: no. Guarded by test: table-rules-words.spec.

### web-76 [duplication/low] (editors)

`KIND_FROM_GEN: Record<GenCharacterKind, CharacterKind>` is written three times: pages/character-editor/character-editor-source.live.ts:66 (with `KIND_TO_GEN`), pages/character-sheet/character-sheet-map.ts:43, pages/campaign-detail/characters/campaign-characters-source.live.ts:28. Same fix as the ability table: one mapper in core/characters. Behaviour change: no. Guarded by test: source specs.

### web-77 [docs/low] (editors)

History and audit words copied into the files created today: `integrator fix`/`phase 2b`/`phase 2`: editor-picks.ts:23,30,33; full-draft.ts:83; full-form.ts:54; character-editor-sheet-map.ts:199; character-sheet-map.ts:342; character-editor.types.ts:14,82,91,279; character-sheet-writes.ts:226 (`integrator amendment to A3, 29/09/2026`). Audit ids: save-flow.ts:158 (`E10-01 state 5`), run-flow.ts (`E10-07 4`), class-editor.ts doc (`E10-02 states 1 to 3`, `slice 10.11c`). These comments describe who asked for the change, not what the code does. Fix: delete the parenthesis. Behaviour change: no. Guarded by test: no.

### web-78 [duplication/low] (maps)

web/src/app/pages/maps/map-editor/map-editor.ts:268 — `pendingName` re-finds the point by `selection().id` with no kind check; `selectedPoint()` (line 262) already is that lookup (`selectedPointOf`). Fix: `computed(() => this.selectedPoint()?.name ?? '')`. Behaviour change: no (token and point ids never collide). Guarded by test: map-editor.spec.ts (the "Salvar as mudanças em <name>" question).

### web-79 [readability/low] (maps)

web/src/app/pages/maps/map-editor/editor-people.ts:314,322 — `selectedPointOf` and `selectedTokenOf` are 3-line helpers used once each, in two `computed`s that moved nowhere shorter. `occupantsOf` (298) also one caller. Fix: inline `selectedPointOf` into `selectedPoint`, keep `occupantsOf`/`notOnMap`/`cluePlayers`/`finderRows` (pure and spec-tested). Behaviour change: no. Guarded by test: editor-people.spec.ts pins the helpers themselves (delete those cases with the helper).

### web-80 [readability/low] (maps)

web/src/app/core/maps/point-changes.ts:1 and web/src/app/core/maps/maps-client.ts:25 — `PointChanges` was moved to its own file only to shorten maps-client.ts, then re-exported from maps-client (`export type { PointChanges }`). All 10 importers (map-editor.ts:74, editor-point-saves.ts:7, trap-draft.ts:19, light-point-panel.ts:22, treasure-point-panel.ts:23, trap-point-panel.ts:35, treasure-draft.ts:2, light-draft.ts:4, maps-testing.ts:22) still import from `maps-client`, so the new file is a pass-through; map-editor.ts imports `maps-client` twice (lines 31 and 74). Same pattern: web/src/app/core/maps/map-builders-testing.ts:11 is re-exported by `maps-testing.ts:23` and every spec still imports `maps-testing`. Fix: point the importers at `point-changes` / `map-builders-testing` and delete both re-exports, or undo the split (maps-client is 474 lines but under the 400-code-line gate). Behaviour change: no. Guarded by test: tsc.

### web-81 [duplication/low] (maps)

web/src/app/shared/map-view/map-gestures.ts:12 — `GestureMove` is a field-for-field copy of `MapMove` (map-view.ts:65, `MapSelection` + `xBp`,`yBp`); `MapView` passes `MapMove` where `GestureMove` is expected only because they are structurally equal. Fix: `map-gestures.ts` imports the type from one shared place (move `MapSelection`/`MapMove` to `map-geometry.ts` or a `map-view.types.ts`, re-export from map-view). Behaviour change: no. Guarded by test: tsc.

### web-82 [readability/low] (maps)

web/src/app/shared/map-view/map-view.ts:416-430 and shared/area-picker/area-map.ts:305-319 — `onPointerDown/Move/Up/Wheel` (MapView) and `onPointerDown/Move/Up/Leave/Cancel` (AreaMap) are one-line forwards. The template could bind `(pointerdown)="gestures.down($event)"` with `gestures`/`pointer` made `protected readonly`. Also `GestureHost` (9 members: `interactive, viewport, transform, size, override, movable, itemOf, report, emptyClick`) is a closure bag over the component's private methods; `AreaPointerHost` (area-pointer.ts:11-18) exposes both the `hover` signal and `setHover()`, and `leave()`/`cancel()` (area-pointer.ts:78,84) write `hover` directly while `down/move` go through `setHover` (which also resets `hoverByKey`): two ways to set the same state, only one of which resets `hoverByKey` (same as before the split, now harder to see). Fix: drop `hover` from the host, add `clearHover()` on the host that calls `setHover(null,false)`. Behaviour change: yes, tiny (`hoverByKey` reset on leave/cancel; today a keyboard hover followed by mouse leave keeps `hoverByKey=true` until the next mouse move, so the hint keeps following the last key pick). Guarded by test: area-map.spec.ts / area-map.list.spec.ts do not cover it.

### web-83 [readability/low] (maps)

web/src/app/shared/area-picker/area-moves.ts:1 — file name says "moves" but holds four unrelated things: arrow moves (`movedPoint`, `turnedDirection`, `clampSquare`), the list keys (`listMove`), pointer geometry (`squareUnder`, `fingerAt`, used only by area-pointer.ts), and overlay geometry (`areaLine`, `edgeMarkOf`, used only by area-map.ts computeds). Fix: move `squareUnder`/`fingerAt` into area-pointer.ts, `areaLine`/`edgeMarkOf` next to the overlay (area-overlay.ts exists, 34 lines), keep `area-moves.ts` for keyboard only. Behaviour change: no. Guarded by test: area-moves.spec.ts (58 lines, pins `areaLine`/`edgeMarkOf` etc. by path, update imports).

### web-84 [readability/low] (maps)

web/src/app/shared/relay-events/relay-events.ts:11 — `appRelayEvents` exists so templates can keep `(click)`/`(keydown)` on non-focusable containers while `interactive-supports-focus` stays quiet (its own doc says so). It is used 17 times (map-view.html:2, editor-bar.html:2,86,109,121, trap-point-panel.html:5,97,106,294, kind-strip.ts:25, choice-row.ts:29, level-grid.html:7, door-sheet.ts:131, gallery-lightbox.html:1, doc-dialog.html:1, ...). For the radiogroups the rule is arguably a false positive; for `map-view.html:2` (`role="group"`, click handler on the whole viewport) and the `<dialog>`/`<table>`/`<section>` uses it simply hides the rule. Every listener now fires through an extra `output()` hop. Fix: for the radiogroup/dialog cases keep the directive but name it for what it is (`appKeyDelegate`, only keydown), and add a one-line justified `eslint-disable-next-line` for the map viewport instead of routing clicks through the directive. Behaviour change: no. Guarded by test: relay-events.spec.ts (33 lines, keep).

### web-85 [readability/low] (maps)

web/src/app/shared/image-generate/image-generate-view.ts (`WhyInput`, `SubtitleInput`, `DoneInput`) — three parameter structs, each used by exactly one call in the dialog (lines 200, 215, 418), plus 5 helpers (`mainOf`, `summaryOf`, `editWhyOf`, `resultKindOf`, `isStale`) of 1-6 lines each used once. Fix: keep the genuinely branchy ones (`whyOf`, `subtitleOf`, `doneView`) with positional args or the existing signals, inline the rest. Behaviour change: no. Guarded by test: image-generate-view.spec.ts (pins these helpers; trim with them).

### web-86 [readability/low] (maps)

web/src/app/shared/image-generate/image-form-state.ts:302 — `readonly form: ReturnType<typeof signal<ImageForm>>` assigned in the constructor, only to read `origin`; `WritableSignal<ImageForm>` is what is meant. Also web/src/app/shared/image-generate/image-generate-dialog.ts:267-269 sets `referenceState` to `'loading'` itself, then calls `refs.load()` (image-references.ts:441) which sets `'loading'` again; `state` is written from two classes. Fix: drop the dialog's write; keep `state` private to `ImageReferences` behind a read-only view. Behaviour change: no. Guarded by test: image-generate-dialog.spec.ts.

### web-87 [duplication/low] (maps)

web/src/app/pages/maps/point-kinds/trap-draft.ts:87-92 — `blankDamage` (arrow const) and the exported `newDamage()` are identical; `attackDraft` (line 145) calls one, panel/specs the other. Fix: keep `newDamage` only. Behaviour change: no. Guarded by test: trap-draft.spec.ts.

### web-88 [duplication/low] (maps)

web/src/app/pages/maps/point-kinds/trap-draft.ts:314 and web/src/app/core/traps/trap-text.ts:45 — two `Ability → Portuguese name` tables inside the traps feature (`ABILITY_NAME` built from `ABILITIES` here, `ABILITY` there), both with the fallback `'habilidade'` in `presetSummary` (trap-draft.ts:332) and `saveText` (trap-text.ts), plus `creature-format.ts:100` outside it. Fix: one exported `ABILITY_NAME` in `core/traps/trap-text.ts` (or the existing `creature-format` map) used by both; `ABILITIES` keeps the dropdown order. Behaviour change: no. Guarded by test: trap-text.spec.ts, trap-draft.branches.spec.ts ("presetSummary, the other shapes").

### web-89 [readability/low] (maps)

web/src/app/pages/maps/point-kinds/trap-draft.ts:28-41 — the file's doc comment ("What the trap form edits … Pure functions") now sits on top of an `export type { … } from './trap-draft-types'` and `export { … } from './trap-errors'` block instead of on a declaration, and the draft is spread over `trap-draft.ts` + `trap-draft-types.ts` (types only, 57 lines) + `trap-errors.ts` + `trap-edits.ts` (a grab-bag: severity line formatting, list helper, listbox key map; one caller, trap-point-panel.ts:66). Consumers import only from `trap-draft` except the panel. Fix: fold `trap-draft-types` back into `trap-draft` (types go at the top; file is 336 lines, +55 stays under 400) and delete the type re-export block; keep `trap-errors.ts`; move `alsoKeyMove` next to `toggleAlsoFind` and `changeAt`/`severityLine` into the panel's file or `trap-draft`, delete `trap-edits.ts`. Behaviour change: no. Guarded by test: trap-draft.spec.ts + trap-point-panel.parts.spec.ts.

### web-90 [readability/low] (maps)

web/src/app/pages/maps/point-kinds/trap-errors.ts:57-88 — `fieldErrors` and `attackErrors` are typed `Omit<TrapErrors,'parts'>` but each builds `{ parts: {} }` internally to satisfy the mutable alias, and `trapErrors` (line 139) spreads them then overwrites `parts`. Hardcoded copy beside constants: line 64 `'Use até 2.000 caracteres.'` next to `POINT_DESCRIPTION_MAX`, line 67 `'… de 1 a 30 …'` next to `DC_MAX`. Fix: return `Partial<Omit<TrapErrors,'parts'>>` from a plain `const errors: … = {}`; build both messages from the constants. Behaviour change: no. Guarded by test: trap-draft.branches.spec.ts / trap-draft.spec.ts.

### web-91 [readability/low] (maps)

web/src/app/pages/maps/point-kinds/trap-point-panel.ts:400 and 408 — `onSave(edit)` reads like an event handler but is "edit the save sub-draft if there is one"; rename `editSave`. At 408 `this.draft().save?.failDamage ?? []` is dead (`patchSave` → `onSave` already does nothing without a save, and the template only shows the button inside the save). Lines 218 and 222 carry the same comment twice. Behaviour change: no. Guarded by test: trap-point-panel.parts.spec.ts ("edits the save … ignores it all when there is no save").

### web-92 [duplication/low] (maps)

web/src/app/pages/table-rules/table-rules-options.ts:14 and :147-157 — `DICE_TITLES` repeats the three titles of `DICE_OPTIONS`, and `XP_OPTION_TITLES` (line 20) is a third copy of XP mode names; the file also has `titleOf(options, value)` that already does this for every other rule. Carried over from the pre-split table-rules.ts, now visible side by side. Fix: `titleOf(DICE_OPTIONS, d.diceMode)` in `ruleRows` and delete `DICE_TITLES`. Behaviour change: no. Guarded by test: table-rules-words.spec.ts.

### web-93 [readability/low] (maps)

web/src/app/pages/maps/map-page/map-page.ts:38 — `PHASE_OF_STATUS: Partial<Record<string, Phase>>` keyed by `string`, indexed with `this.state.status()` (a `MapStatus`): a new status added to `MapStatus` compiles and falls into `'ready'` silently (line 169-172). Fix: export `MapStatus` is already public; type it `Partial<Record<MapStatus, Phase>>` and write `'idle'|'loading'` explicitly if they must also map to `ready`. Behaviour change: no. Guarded by test: map-page.load.spec.ts.

### web-94 [duplication/low] (maps)

web/src/app/core/maps/door-paint.ts:172-174 — `CLOSED = 2, LOCKED = 3, SECRET = 5` are new private magic-number names for `DoorKind`; the same numbers are still literal in core/maps/layers.ts:112 (`state = 2`) and the `DOOR_NAME` table in shared/map-layers/door-mark.ts:6. Fix: one exported `DoorKind` const object in layers.ts and use it in all three. Behaviour change: no. Guarded by test: door-paint.spec.ts.

### web-95 [duplication/low] (maps)

web/src/app/shared/map-pins/map-pins-build.ts:70-74 — `DRAWN_KINDS` lists the same three kinds as the `switch` in `pinOf` (48-66); adding a pin kind needs two edits. The pre-filter exists because `pinOf` returns `null` both for "not a drawn kind" and "a light the player does not see", and only the second must still advance the square's `n`. Fix: have `pinOf` return `undefined` for not-drawn and `null` for hidden-light, drop `DRAWN_KINDS`. Behaviour change: no. Guarded by test: map-pins-build.spec.ts.

### web-96 [docs/low] (maps)

History or audit words in comments written or kept by the splits: web/src/app/shared/area-picker/center-options.ts:48 "as before" (and the spread-then-override at 49-53: `optionOf` already sets `id: c.id`, equal to `t.combatantId`, so only `initial` needs the target's label; pass the label to `optionOf` instead); web/src/app/shared/map-pins/map-pins-build.ts:100 "as before"; web/src/app/pages/maps/map-editor/map-editor.ts:77 "the shape `PointPanel` always had", :84-87 audit ids "(E5-23, E9-01, E9-02, MR-008, MR-034, MR-035, MR-036, MR-041)", "the editor it always was", "and now Luz, Armadilha e Tesouro"; web/src/app/core/auth/session-refresher.ts:4-8 "was an import cycle", "as before"; web/src/app/shared/combat-map/combat-map.ts:73-76 "Etapa 9/10"; web/src/app/shared/map-layers/map-layers.ts:7,13 and pages/maps/layers-panel/layers-panel.ts:24 "Etapa 9/10"; web/src/app/core/maps/paint-tools.ts:7 "(E9-01, and 'Porta' of Etapa 10)"; web/src/app/pages/maps/map-page/map-page.ts:198 "a live master page is a later step". Fix: reword to present tense without ids. Behaviour change: no. Guarded by test: no.

## Notes the reviewers verified as equivalent (do not re-check)

See the head of each area section below.

#### live

# Review: live-session split, live-stream, combat-log, cast-flow, puzzles, start-combat/xp/creatures

Scope: `web/src/app/pages/live-session/**` (not `combat/`), `core/play`, `core/combat`, `core/puzzles`, `shared/relay-events|start-combat|xp|creatures`.
Method: old `live-session.ts` (0405b41^), `live-stream.ts` (05f90e5^), `combat-log.ts` / `cast-flow.ts` / `combat-client.ts` (cae6f1c^), `puzzle-play.ts` (cae6f1c^), `start-combat-dialog.ts` (edd071e^) read against the new files; every moved block compared by hand or by line-set diff.

Behaviour result up front: I found NO behaviour break in the live-session split. Verified equal: effect order and dependencies (the five effects in `wireLiveSessionEffects` are created in the constructor after the `paramMap` subscribe, as before; `OwnEffects` still created during field initialisation, so `inject`/`effect` have a context); every template member in `live-session.html` exists (`state.*` 41 names, `view.*` 19, `events.*` 3, `sheets.*` 5, `loader.*` 2, plus the 10 own handlers); `bumpTicks` keeps the old order; all 29 stream handlers are present across the four handler groups (`satisfies Partial<LiveStreamHandlers>` rejects typos and `LiveStreamOptions.handlers` still demands `onReady/onVitals/onEnded/onFatal`); `begin()` before `open()` as before; `LiveStream.receive`/`routeEvent` equal to the old switch (including `characterRevived` firing `onCharacterChanged` first, `ended` stopping then calling `onEnded`, unknown kind doing nothing); `live-event-map` equal to the old `watch` switch (`vitalsChanged` without body yields nothing, unknown/`hiddenHitPending` yield heartbeat); `castRows`/`slotRows`/`castSubtitle`, `PuzzleHints`, `blockedReason`/`joinSpecs`, `awardInputOf`, `rollLabel/listKeyMove/typedLineOf`, `cast-out-view` builders all equal. No storage API use in my area. Findings below are therefore mostly maintenance and one lost guard.

#### combat

# Review: web/src/app/pages/live-session/combat (split commits d2c0393, a422876, 659a114, ce764ea)

Paths below are relative to `web/src/app/pages/live-session/combat/` unless they start with `core/`.

## Behaviour equivalence: what I checked and found equal

- CombatView constructor effects: `git show 659a114^:.../combat-view.ts` diffed against current `combat-view.ts`. The 17 effects are identical except `this.sheets.reaction` -> `this.sheets.reactions.reaction` (and `.opportunity`). Same order, same reads, same `untracked`.
- Effects moved by d2c0393 into plain classes: `CombatReads` (6), `MoveFlow` (2), `MasterAutoPass` (1, in `core/combat/auto-pass.ts`), `ContestHost` (2). All are built in field initializers of the Directive chain, so they have an injection context. They take signals or accessor lambdas, not copied values.
- Field-initializer order across the 5-level Directive chain: every forward reference (`sheets` -> `moves.moveNote`, `calls` -> `encounter`, `hooks` -> `caller` in `CastSheet`) is a lazy arrow. The only eager captures (`ContestHost` gets `encounter: this.encounter`, `own: this.own`, `options: this.options`) are already defined in `CombatViewStateDirective`. No finding.
- One ordering change: the `CombatReads`, `MasterAutoPass` and `MoveFlow` effects are now created before the constructor effects. The old order was autoPass, refresh, deathRule, ... All are independent writers of different signals, so the observable result is the same. Not guarded by a test; low risk.
- Templates: the extracted `reaction-sheet.html` and `reaction-queue.html` are byte-identical (`diff -w`) to the old inline templates. The `AttackSheet`, `CastSheet` and `ReactionSheet` constructor effects match the pre-split ones.
- Keyboard changes in 659a114 inside this folder: only the extracted templates. No `(click)` on non-interactive elements remains in `combat/**/*.html`. No finding.
- No localStorage, sessionStorage, IndexedDB or cookie use in `combat/**` (non-spec).
- Nothing in the view chain reads a plain field from the template. `offerKeys`, `reactorReading`, `refreshChecked` and `concentration.endedByMe` are only read from effects or handlers.
- `CombatView` has no `ChangeDetectionStrategy.OnPush`. The pre-split file did not either, so this is not a regression. The app is zoneless and everything the template reads is a signal.

I found no high-severity regression. Findings, most important first:

#### editors

# Review: editors area (character-editor, character-sheet, level-up, content, spells, table-rules, core/levelup, core/content, summon-sheet, image-generate)

Method: pre-refactor files read with `git show <sha>^:<path>` and compared function by function against current main.

Behaviour equivalence result (priority 1). I found no behaviour regression in the areas below. Details are here so nobody re-checks them:
- character-editor: submit guard order (not-ready, choicesBlocked, hasProblems, showProblems, run), the create/update request, the idempotency key input, `stillHere`/destroy handling, `refused` ordering, `markSwitchedOff`, `reset` coverage, `buildFullValue`, effect dependencies, and all `toSignal`/field-init order are the same as before. `EditorPicks.reset()` plus `SpellPicks.reset()` cover everything `resetEditing` cleared.
- Source mappers (`character-editor-sheet-map`, `-catalog-map`, `character-sheet-map`) are the same.
- SheetWrites, LevelUpReader, LevelUpFocus, LevelUpDraft/Lists/Abilities/Missing/Request, `levelup-errors` tables, class-editor-model, content-entry-model, SummonLists, ImageGenerate (RunFlow, GeneratedResult, ImageFormState, ImageReferences, view fns), spells-params and table-rules-words are the same.
- Nothing reads or writes localStorage, sessionStorage, IndexedDB or cookies.
- The only change in execution order is finding 18 (low, uncertain).

#### maps

# Review: maps / shared map components / web leftovers (reviewer f-maps)

Scope read: pages/maps/** (map-editor split, point-kinds/trap-*, map-page, map-print), core/maps/** (map-state, point-changes, maps-testing), shared/map-view, combat-map, area-picker, map-pins, image-generate, start-combat, summon-sheet, markdown, xp, relay-events, core/connect/transport + auth/session-refresher (fe76ca5), pages/spells, pages/table-rules. Each old file was diffed against its pre-split version (`<sha>^`).

Result: no behaviour regression found in the splits themselves (details in "Verified equivalent" at the end). Findings are one real wiring risk, one async gap next to the stale-read guards, and mostly maintenance cost. No finding is severity high.

## Spec verdicts by reviewer (detail)

Verdicts per reviewer, with line counts. Everything not listed is KEEP (it guards behaviour or a pure rule table).

#### live

Delete / merge:
- `core/puzzles/puzzles-testing.spec.ts` (147): tests the fixture builders in `puzzles-testing.ts` (`lightsPuzzle`, `playerRun`, `NOW`, `SKILLS`...), i.e. test helpers testing themselves; every real spec already exercises them. DELETE.
- `core/combat/combat-log.undo.spec.ts` (60): same three cases as the `undoLabel` describe at `combat-log.branches.spec.ts:32-127` and `combat-log.spec.ts:498,933,1080`. DELETE.
- `core/combat/combat-log.branches.spec.ts` (583): coverage-driven inline snapshots of strings pinned "as the code wrote them before the split"; the pool-card and hp-spell parts overlap `hp-spells-log.spec.ts` (287) and `combat-log.spec.ts`. MERGE the non-duplicate parts (small kinds, riders, states) into `combat-log.spec.ts`/`combat-log.resources.spec.ts`, drop the rest.
- `shared/start-combat/start-combat-blocked.spec.ts` (158): drives the whole dialog to re-prove the same `blockedReason` rows already in `start-combat-plan.spec.ts:29-72` (roster, map, grid, name 1-80, group, NPC, 40 cap). KEEP only "says what an NPC becomes" (copiesText wiring) and the 40-count-with-existing case in `start-combat-dialog.spec.ts`; delete the rest.
- `pages/live-session/casting/cast-out-view.spec.ts` (189): pins `casterRowsOf`/`targetChoicesOf`/`wayRowsOf`/`spellRowsOf` internals that `cast-out-sheet.spec.ts` (389) already reaches through the sheet. MERGE `diceOf`, `missingOf`, `castRequestOf`, `summonedNotice`, `endsConfirmText` cases into `cast-out-sheet.spec.ts`, DELETE the row-mapper cases.
- `core/play/live-stream.dispatch.spec.ts` (510): useful (every kind -> handler, optional handler skipped, replaced connection) but half of it ("lifecycle corners", 303-510) repeats `live-stream.spec.ts` (548) with the same FakeCall/fake clock. MERGE into `live-stream.spec.ts`, keeping the per-kind table (lines 55-300).
- `shared/xp/award-input.spec.ts` (25): three cases of a 19-line one-caller function that `award-xp-sheet.spec.ts` (485) covers through the sheet (gold / combat / manual). MERGE into `award-xp-sheet.spec.ts` or delete if the sheet spec already asserts the request modes.

Keep (behaviour or real rules, not internals): `core/combat/auto-pass.spec.ts` (149), `combat-calls.spec.ts` (93), `combat-derived.spec.ts` (275), `cast-flow.text.spec.ts` (216), `combat-notices.spec.ts` (261), `combat-view.spec.ts` (294), `hp-spells-log.spec.ts` (287), `spell-summary.spec.ts` (79), `core/puzzles/puzzle-draft.spec.ts` (889), `puzzle-errors.spec.ts` (364), `puzzle-format.spec.ts` (387), `puzzle-play-edges.spec.ts` (559), `puzzle-play-support.spec.ts` (90), `core/play/live-session-source.live.spec.ts` (525), `shared/relay-events/relay-events.spec.ts` (33), `shared/creatures/stat-block-data.spec.ts` (146), `shared/start-combat/start-combat-plan.spec.ts` (133), `shared/xp/award-xp-sheet.spec.ts` (485), `pages/live-session/live-session.spec.ts` (2,490; the only guard of the split, keep, but split by concern after the field moves), `puzzles/master-run/master-run.spec.ts` (928), `traps/trap-search-sheet/trap-search-sheet.spec.ts` (455), `trap-search-view.spec.ts` (37), `group-check/group-check-dc.spec.ts` (22), `casting/cast-out-sheet.spec.ts` (389).

#### combat

Added by d2c0393, a422876, 659a114, ce764ea (lines now):

- `combat-view.state.spec.ts` 1935, `combat-view.sheets.spec.ts` 2274, `combat-view.calls.spec.ts` 1755: KEEP, but they are huge. They are the characterization tests for the view. They read protected members through the component, which is what pins the pass-through aliases and wrappers listed above. Update them together with any removal. Note the three together are 5964 lines, over the 400-line file limit by 15x; split by subject when touched.
- `combat-view.sheets-edges.spec.ts` 82: MERGE into `combat-view.sheets.spec.ts` (same subject, "the edges of the sheets the page opens", only 82 lines, own mount helper import).
- `attack-sheet/attack-sheet.branches.spec.ts` 887 and `cast-sheet/cast-sheet.branches.spec.ts` 1326: KEEP (branch characterization of the sheets), but they call `sheet.answerLucky`, `sheet.pickSlot` and `sheet.busy()`; see the dead-member findings.
- `attack-sheet/attack-sheet-view.spec.ts` 480 and `cast-sheet/cast-sheet-view.spec.ts` 702: KEEP (pure function tables). `cast-sheet/cast-sheet-data.spec.ts` 295, `attack-sheet/attack-sheet-data.spec.ts` 224: KEEP.
- `npc-card/npc-card-view.spec.ts` 149 and `npc-card/npc-card.view.spec.ts` 249: MERGE. Two files with near-identical names (`npc-card-view` vs `npc-card.view`) for the same card's words; the 149-line one tests the pure functions, the 249-line one the component. Put them in one file with two `describe`s, or rename the second to `npc-card.words.spec.ts`.
- `attack-sheet/attack-questions.spec.ts` 187, `cast-sheet/cast-questions.spec.ts` 184, `attack-sheet/mode-requests.spec.ts` 203, `cast-sheet/cast-damage.spec.ts` 136: KEEP, but if the busy-run helper is extracted they should assert through it once and drop the repeated busy/error assertions.
- `attack-sheet/attack-sheet.words.spec.ts` 231 and `cast-sheet/cast-sheet.result.spec.ts` 441: KEEP (rendered words and result steps).
- Remaining new specs, all KEEP: `reaction-sheet/reaction-sheet.flows.spec.ts` 558, `reaction-sheet/reaction-sheet-view.spec.ts` 108, `reaction-queue/reaction-queue.flows.spec.ts` 445, `reaction-queue/reaction-queue-text.spec.ts` 272, `contest-sheet/contest-sheet-view.spec.ts` 89, `contest-sheet/contest-sheet.maneuver.spec.ts` 252, `move-page/move-page-view.spec.ts` 205, `move-page/move-page.jump.spec.ts` 228, `npc-card/pending-damage-card.spec.ts` 64, `npc-card/pending-damages.cards.spec.ts` 204.
- Specs I list as pinning internals only in the sense of the findings above: none are pure internals-only; I would not delete any. The only DELETE-style change is removing assertions on `lookingThroughFamiliar`, `openSpend`, `cast-sheet` alias fields, `busy()`.

Note: I did not run any tests (per brief). Spec membership is from `git show --name-status` of the listed commits; `edd071e`, `cae6f1c` touched nothing in this folder.

#### editors

Delete or merge (the test only pins the split's internals or repeats another test):
- pages/character-editor/skill-picks.spec.ts (49): merge into editor-picks.spec.ts (121). Both test the same toggles; editor-picks.spec already covers "picks and drops a skill, takes the expertise", "does not take a skill the race gives", "gives expertise only to a proficient skill" and "caps the custom background" through `EditorPicks`.
- pages/character-editor/character-editor.branches.spec.ts (1064): keep the file, delete the cases it now duplicates (line numbers as of main): "does not toggle a skill the race gives" (644), "stops at two skills for a custom background" (694), "adds a choice cantrip once" (707) and "adds a feat once and takes it off" (714) repeat editor-picks.spec/spell-picks.spec; "drops the answer of a read the person left" (856) and "drops a stale error of the read of an edit" (875) repeat editor-loader.spec "drops the answer of a route the person left" (145, 212); "keeps the key of a reserved character" (802) repeats save-flow.spec createInputOf (79).
- pages/level-up/level-up.spec.ts:1672 `describe('newSpellsTitle')` (4 cases) duplicates level-up-session.spec.ts:132 (and the function moved to level-up-gives): keep one, in a level-up-gives.spec.
- pages/character-sheet/character-sheet-writes.spec.ts (101): keep, but its "brings a character back", "goes back after a rejection" repeat character-sheet.spec (36 matches for the write names); acceptable because it is the only unit test of `SheetWrites` itself. Verdict: keep; remove the overlapping cases from character-sheet.spec when that file is next touched.

Keep (pure functions, a class with its own contract, or a mapper):
catalog-views.spec (239), editor-state.spec (241), full-draft.spec (238), save-flow.spec (276), spell-picks.spec (98), spell-views.spec (287), editor-loader.spec (374), editor-picks.spec (121), editor-spell-details.spec (61), character-editor-source.rpc.spec (670), character-editor-choices.spec (434), character-editor.spec (3107; changed), core/content: catalog.spec (119), class-draft.spec (405), content-read-kinds.spec (461), content-support.spec (137), content-violations-texts.spec (108), effect-request.spec (132), spell-form.spec (1316); core/levelup: levelup-draft-edges.spec (451), levelup-errors.spec (408), levelup-picks.spec (142), levelup-request.spec (82), levelup-summary-rows.spec (28), levelup-summary.spec (857); pages: character-sheet-source-vm.spec (211), character-sheet.spec (3148), sheet-format.spec (214), class-editor-model.spec (190), class-editor.spec (657), content-entry-model.spec (151), content-entry.spec (573), level-up-session.spec (173), level-up.spec (1680), spells-params.spec (61), spells.spec (632), table-rules-words.spec (75), table-rules.spec (602); shared: summon-lists.spec (81), summon-views.spec (294), image-generate-view.spec (235).

#### maps

- web/src/app/pages/maps/point-kinds/trap-edits.spec.ts (43) — delete: the severity line, `changeAt` and the skill-list cursor are all asserted again through the panel in trap-point-panel.parts.spec.ts (146, 179, 242-270); it only pins the helper file that finding above removes.
- web/src/app/pages/maps/map-editor/new-point.spec.ts (65) — merge into map-editor.flows.spec.ts "new points": the cases "names each kind" and "gives a light, a trap and a treasure the start the server takes" repeat flows.spec.ts:223; keep only the "falls back to an empty preset and a DC of 10" case.
- web/src/app/shared/start-combat/start-combat-blocked.spec.ts (158) + start-combat-plan.spec.ts (133) — merge: every `blockedReason` branch is asserted twice, once through the dialog (blocked.spec 80-145) and once on the pure function (plan.spec 29-72). Keep the pure cases plus two dialog wiring cases (loading, the 40-combatant count with `existing`), drop the rest (about 80 lines).
- web/src/app/pages/maps/map-print/print-sheets/print-sheet.spec.ts (32) + print-sheets.spec.ts (82) — merge into one file: `buildSheet` neighbours/bands/crosses are the same facts the component spec asserts ("tint the 1 cm …", "one sheet with no band, no neighbour and no cross").
- web/src/app/pages/maps/point-kinds/trap-draft.branches.spec.ts (191) — merge into trap-draft.spec.ts (326) by `describe`; the file name records the refactor's coverage goal, not a subject.
- Keep (group): map-editor.flows.spec.ts (463), editor-people.spec.ts (93), map-page.load.spec.ts (417), trap-point-panel.parts.spec.ts (327, rename without "parts"), map-view.gestures.spec.ts (252), combat-map.input.spec.ts (252), area-map.list.spec.ts (242), area-moves.spec.ts (58), map-pins-build.spec.ts (56), door-paint.spec.ts (208), map-state.spec.ts (259, includes the two 2785ee9 cases), relay-events.spec.ts (33), image-generate-view.spec.ts (235, trim with the one-caller helpers), stat-block-data.spec.ts (146), award-input.spec.ts (25), summon-lists.spec.ts (81), summon-views.spec.ts (294), spells-params.spec.ts (61), table-rules-words.spec.ts (75), markdown.spec.ts (354), spell-details-map.spec.ts (186), map-layers.spec.ts (229), fog-map.spec.ts (402), map-marker.spec.ts (62), plus the older specs touched only for imports (game-session-card, table-rules, spells, award-xp-sheet, upload-errors, xp-errors, trap-text, core/campaigns/table-rules). Overlap between map-editor.flows.spec.ts and the 1247-line map-editor.spec.ts (both drive selection, dirty question, save) was not audited line by line; certainty low.

## The test-trim table

Specs added after `3265aab` in `web/src`: 93 files; the specs of `web/src` changed by +32.9k lines in that range. Everything not in the table is KEEP (pure rule tables, mappers, a class with its own contract, or the only guard of a split). The large `combat-view.*.spec.ts`, `live-session.spec.ts`, `character-editor.spec.ts`, `character-sheet.spec.ts` are the real guards of the splits and stay, but they read the pass-through aliases listed in the findings, so removing those wrappers edits them.

| Spec | Lines | Verdict | Reason | Lines saved (est.) |
|---|---|---|---|---|
| `core/puzzles/puzzles-testing.spec.ts` | 147 | delete | tests the fixture builders of `puzzles-testing.ts`; every real spec exercises them | 147 |
| `core/combat/combat-log.undo.spec.ts` | 60 | delete | same cases as `combat-log.branches.spec.ts` (undoLabel) and `combat-log.spec.ts` | 60 |
| `pages/maps/point-kinds/trap-edits.spec.ts` | 43 | delete | everything is asserted again through the panel in `trap-point-panel.parts.spec.ts` | 43 |
| `core/combat/combat-log.branches.spec.ts` | 583 | merge | inline snapshots pinned "as before the split"; pool and hp-spell parts overlap `hp-spells-log.spec.ts` and `combat-log.spec.ts`; move the non-duplicates into `combat-log.spec.ts` | ~250 |
| `core/play/live-stream.dispatch.spec.ts` | 510 | merge | keep the per-kind table (lines 55-300); the lifecycle half repeats `live-stream.spec.ts` | ~200 |
| `pages/live-session/casting/cast-out-view.spec.ts` | 189 | merge | row-mapper cases pin internals that `cast-out-sheet.spec.ts` reaches; keep `diceOf`, `missingOf`, `castRequestOf` | ~100 |
| `shared/start-combat/start-combat-blocked.spec.ts` | 158 | merge | every `blockedReason` row is asserted twice (dialog and `start-combat-plan.spec.ts`) | ~110 |
| `pages/character-editor/character-editor.branches.spec.ts` | 1064 | trim | 7 cases repeat `editor-picks`, `spell-picks`, `editor-loader` and `save-flow` specs | ~120 |
| `pages/character-editor/skill-picks.spec.ts` | 49 | merge | same toggles as `editor-picks.spec.ts` | ~40 |
| `pages/level-up/level-up.spec.ts` (`newSpellsTitle`) | 4 cases | delete cases | duplicate of `level-up-session.spec.ts:132` | ~30 |
| `pages/maps/map-editor/new-point.spec.ts` | 65 | merge | two of three cases repeat `map-editor.flows.spec.ts:223` | ~45 |
| `pages/maps/map-print/print-sheets/print-sheet.spec.ts` | 32 | merge | same facts as `print-sheets.spec.ts` | ~15 |
| `shared/xp/award-input.spec.ts` | 25 | merge | the sheet spec covers the three modes | ~25 |
| `combat-view.sheets-edges.spec.ts` | 82 | merge | same subject as `combat-view.sheets.spec.ts` | 0 (file count) |
| `npc-card-view.spec.ts` + `npc-card.view.spec.ts` | 149 + 249 | merge | two near-identical names for one card; rename or one file | 0 (clarity) |
| `trap-draft.branches.spec.ts` | 191 | merge | file name records the coverage goal; merge into `trap-draft.spec.ts` by `describe` | 0 (clarity) |

Estimated saving: about 1,200 lines of about 28.7k spec lines added in the range (roughly 4%), and 8 fewer spec files. CI time: these are small, pure unit specs, so the saving is a few seconds of the vitest run at most; it was not measured (no test run in this phase). The value is less duplicated assertions to update when the pass-through wrappers go. One audit not done: `map-editor.flows.spec.ts` (463) against `map-editor.spec.ts` (1247) overlap, low certainty.

## Other global findings (reviewer's own measurements)

- web-G1 [readability/medium] `web/eslint.config.js:87` — `max-lines` counts code lines only and templates are exempt, so the gate was met by moving code into abstract `@Directive()` chains (`combat-view-inputs/state/derived/parts/flows.ts`) and `(services, state, parts)` bags, not by removing it. Fix: stop adding layers; move members into plain classes with narrow contexts (`CombatCalls`, `MoveFlow`, `ContestHost` are the model that worked). Behaviour change: no.
- web-G2 [docs/low] `docs/architecture/characters-web.md:25-37` (see the editors finding on this page) and `docs/architecture/web.md` — `web.md` says nothing about the layout the splits created (`*-view.ts` pure builders, plain classes built in field initializers, `*-testing.ts`); one short section would let a junior find the next split's pattern. The file names the docs mention all exist (checked).
- web-G3 [readability/low] `web/tsconfig.app.json` — `*-testing.ts` helpers that import `vitest` live in `src/app` and are type-checked as app code (`combat-view-testing.ts`, `combat-testing.ts`, `maps-testing.ts`, `map-builders-testing.ts`). Fix: add `src/**/*-testing.ts` to `exclude`. Behaviour change: no.
- web-G4 [docs/low] Audit and board ids (`E9-10`, `E10-07`, `W7-X`, `quadro 3`) remain in about 20 app files (`shared/image-generate/*`, `shared/creatures/*`, `shared/dice-choice/dice-choice.ts`); `MR-`/`RN-` ids are product rules and stay. Fix: delete the `E`/`W` ids.
