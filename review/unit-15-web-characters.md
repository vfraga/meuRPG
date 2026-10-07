# Review unit 15: characters on the web (sheet, editor, level-up)

- **Scope:** `web/src/app/pages/character-sheet/`, `pages/character-editor/`, `pages/level-up/`, and `shared/{sheet,level-grid,spell-details,form-fields,effect-picker,feature-editor}`. The `core/levelup` draft and summary code was read too, because the level-up screen is built on it.
- **Commit reviewed:** `ae40f74` (`main` of the fork).
- **Model:** Sonnet 5.5 (`claude-sonnet-5-5`) for the review. The finder and verifier subagents ran on `sonnet`.
- **Time spent:** about 2 hours of wall-clock time. Most of it was verifier subagents sharing one machine.
- **Method:** I read the code paths and the docs, then wrote candidates down. Each candidate went to a fresh verifier subagent that got only the candidate text. Each had to reproduce it with a failing Vitest spec or refute it. No production code was changed.
- **Run all proving specs:** `cd web && npx ng test --watch=false --include 'src/app/**/review15-*.spec.ts'`. Every spec listed below fails on current code, on the assertion that states the correct behaviour. A run with all ten files gave 31 failing assertions and no compile errors.
- **Not found:** no `innerHTML` or `bypassSecurityTrust*` in scope, and no Web Storage. I found no screen that offers a player a master action, and no hidden master data rendered. Double submit of "Confirmar o nível" and of the editor save is guarded.

## Findings

Confirmed findings, most severe first. Spec paths are under `web/src/app/`.

| id | sev | status | file:line | defect | failure scenario | evidence |
|---|---|---|---|---|---|---|
| U15-10a | high | confirmed | `pages/character-editor/character-editor.html:480` | `<app-hit-points-rolls>` is not given `[levelDice]`, although `character-editor.ts:456` computes it. Every level is shown, rolled and validated with the first class's die. | Fighter 1 (d10) + Wizard 3 (d6), method "Rolado". Rows for levels 2-4 say "1d10", "Rolar" rolls d10 and 9 is accepted. The server flags "não cabe num d6" and silently uses the average, so the HP differ from what was rolled and previewed. | `pages/character-editor/review15-u15-10.spec.ts`: `expected ['Nível 2 (1d10)', …] to deeply equal ['Nível 2 (1d6)', …]` |
| U15-4b | medium | confirmed | `pages/character-editor/character-editor.ts:1053` (`loadForEdit`), also `loadForCreate` | The load paths have no stale guard, and they reset no state when the route params change (the component is reused). | Open `/characters/A/edit`, A is slow, go to `/characters/B/edit`, B answers first. A's late answer overwrites the form, and the state keeps A's id and revision. The person edits and saves what they think is B. A full sheet followed by an NPC also leaves `selectedSkills` and the spell sets set. | `review15-u15-04.spec.ts`: `expected 'Ana A' to be 'Bruxa B'`; `expected 1 to be +0` (`selectedSkills().size`) |
| U15-4a | medium | confirmed | `pages/character-editor/character-editor.ts:1421` and `:1431` | After `await createCharacter` / `updateCharacter`, `router.navigate` runs with no check that the editor is still alive or on the same route. | Press "Criar", then "Cancelar" or follow a link while the request is pending. The character is created anyway, and the late answer yanks the person to its sheet. | `review15-u15-04.spec.ts`: `expected "navigate" to not be called at all, but actually been called 1 times` (create and edit) |
| U15-3 | medium | confirmed | `pages/character-sheet/character-sheet.ts:192-210` (`reloadQuietly`, `load`) | The async reads have no sequence or identity guard. `reloadQuietly` only checks that the state is `ready`. | (a) An `xp_changed` reload is slow and still carries an alive character. The master confirms "morte", `markDead` sets the dead sheet, then the old answer puts the living character back. `approve`, story save and `setStoryEditing` are exposed the same way. (b) Go from sheet A to sheet B: a late `load` or `reloadQuietly` answer for A replaces B (the route is reused). The creatures panel and notes then bind to A's id. | `pages/character-sheet/review15-u15-03.spec.ts`: `expected 'lock Travada ' to contain 'Morto'`; `expected 'arrow_back Voltar para a campanha Alf…' to contain 'Bravo'` (twice) |
| U15-5 | medium | confirmed | `pages/character-sheet/story-panel/story-panel.ts:116` | `save()` sends `this.vm().revision` read at click time. `startEditing` does not record the revision the form was copied from. | The player opens "Editar história" and types. A background reload (stream hint, another tab, the master) brings revision+1 with other text. "Salvar história" sends the new revision with the old form. The server's lock (`rpc.go`, `revision = $5`) passes and the other writer's story is silently overwritten. | `pages/character-sheet/review15-u15-05.spec.ts`: `expected [ { revision: 5 } ] to deeply equal [ { revision: 4 } ]` |
| U15-2 | medium | confirmed | `pages/character-editor/catalog-changes.ts:4-23`, used by `character-editor.ts` `refreshCatalog` (~1489) | `catalogChanged` compares only keys, names, archived and off flags. A re-read catalog that differs in anything else is thrown away, and no note is shown. | The master edits a table class's hit die (d8 to d10), skill count, subclass level, a spell's circle, a race's Constitution bonus or a background's equipment. The player's open editor keeps the old values. HP preview, skill count and spell circles come from stale content, and the save is built from it after `content_changed`. | `pages/character-editor/review15-u15-02.spec.ts`: `catalogChanged` returns false for hitDie, skillChoose and spell level; `expected 8 to be 10` (editor state after the hint) |
| U15-1 | medium | confirmed (same class); class-change variant unverified | `core/levelup/levelup-draft.ts:374` (`adopt`), via `pages/level-up/level-up.ts` `rereadSheet` | `adopt` copies an in-app roll (`rolled`) from the old draft whatever the new options are for. The server keeps app rolls per character and level. | The player rolls in the app at level 3→4. A second tab confirms. The first tab gets "stale", clicks "Ler a ficha de novo" and the page now says "nível 5" but still shows "Rolado no app: 7". The roll picker is hidden and "Digitar outro" exists only for physical rolls, so the player cannot roll again. The preview and confirm send ROLLED_IN_APP and the server answers `HIT_POINT_ROLL_MISSING`. | `pages/level-up/review15-u15-01.spec.ts`: `expected '…Rolado no app: 7 no d6…' not to contain 'Rolado no app: 7'` |
| U15-8a | medium | confirmed | `core/levelup/levelup-draft.ts:149` (`preparedAsked`), `:437` | Nothing trims `prepared` when the maximum falls. `missing()` reports only shortage. | Cleric with an ability increase: pick WIS, prepare N+1 spells, go back and pick STR. The preview lowers the maximum, but N+1 keys are still sent. The server refuses with PREPARED or SHEET_ISSUE, and the screen has no "go to step" for it. | `core/levelup/review15-u15-08.spec.ts`: `expected 2 to be less than or equal to 1` |
| U15-8b | medium | confirmed | `core/levelup/levelup-draft.ts:362` (`setSubclass`), `:389-395` (`adopt`) | Expertise is not cleaned when the skills it depends on are trimmed. `adopt` trims skills after keeping expertise. | Pick a subclass that adds skills, put expertise on one, then switch to a subclass that adds none (or re-read with fewer skills asked). `expertiseSkillKeys` still has an untrained skill, and the server refuses with EXPERTISE. | `review15-u15-08.spec.ts`: `expected ['skill:stealth'] to deeply equal []`; `expected ['skill:perception'] to deeply equal []` |
| U15-8c | medium | confirmed | `core/levelup/levelup-draft.ts:84`, `:347`, `:359-361`, `:393` | A new draft (re-read) or `setSubclass` resets `preparedMaxAfter` to the server's value without the pending ability choice, then trims prepared picks to it. | With an ASI that raises the maximum and N+1 prepared, a content change or "Ler a ficha de novo" silently drops one pick. The page's note ("uma das suas escolhas saiu da lista") is then wrong. | `review15-u15-08.spec.ts`: `expected ['spell:misty-step'] to have a length of 2 but got 1` (adopt and setSubclass) |
| U15-8d | low-medium | confirmed | `core/levelup/levelup-draft.ts:373-374` | `adopt` copies `hpCard='roll'` and `rolled` even when the new options say AVERAGE_ONLY. | The master switches the table to "average only" while a player has chosen the die. After a re-read the card chooser is hidden, but the roll is still sent and the server answers HIT_POINTS_RULE. The player cannot switch back. | `review15-u15-08.spec.ts`: `expected 'roll' to be 'average'` |
| U15-9 | medium | confirmed | `core/levelup/levelup-summary.ts:73-75` (`cast`), used `:175-249`; caller `pages/level-up/level-up-session.ts:117-128` | The summary uses `spellcasting[0]` instead of the entry of the class being levelled. `changeRows` has no class key. The `known` count (`:201`) sums all classes. | Wizard 3 / Cleric 1 levels Cleric. Index 0 is the Wizard, whose numbers do not change, so the Cleric's DC, attack, cantrips and prepared rows are dropped. | `core/levelup/review15-u15-09.spec.ts`: `expected undefined to match object { before: '12', after: '13' }` |
| U15-6a | medium | confirmed | `pages/character-sheet/creature-page/creature-page.ts:139-204` | The creature page has no sequence guard and no reset when the route changes. | Back or forward between `/creatures/A` and `/creatures/B` (the component is reused). If A's slow answer lands last, the page shows A under B's URL. Rename, Dispensar and Corrigir PV then act on A's id. | `pages/character-sheet/review15-u15-06.spec.ts`: `expected 'Alfa' to be 'Beta'` |
| U15-6c | low-medium | confirmed | `pages/character-sheet/creatures-panel/creatures-panel.ts` `loadWild` | `loadWild` has no sequence guard, unlike `load` in the same file. | An older vitals read that resolves after a newer one flips "Voltar à forma normal" back to "Transformar" and shows a stale form and uses count. | `review15-u15-06.spec.ts`: `expected 'Transformar: Forma Selvagem' to contain 'Voltar à forma normal'` |
| U15-6b | low | confirmed | `creatures-panel/creatures-panel.html:~66`, `creatures-panel.ts` `leave()` | The button stays live while its request is pending, and the key is made per click. A second tap sends another key, and the backend (`play/wildshape.go`) refuses it with NOT_IN_WILD_SHAPE. | Double tap on "Voltar à forma normal". The first call succeeds, but the panel also shows a refusal alert. | `review15-u15-06.spec.ts`: `expected "vi.fn()" to be called 1 times, but got 2 times` |
| U15-7 | medium | confirmed | `shared/effect-picker/effect-picker.ts:160,183` and `.html:41-50`; `core/content/effect-draft.ts:320-328`; `core/units.ts` | The "Alcance" field stores feet but shows metres, and re-renders from the feet value after every keystroke. | Type "4" in the range of a sense effect: `metersToFeet(4)` is 13 ft and the input is rewritten to "3,9". Only multiples of 0.3 m survive, and the saved value is the mangled one. The existing spec types "9", which round-trips. | `shared/effect-picker/review15-u15-07.spec.ts`: `expected '3,9' to be '4'` |
| U15-4c / U15-10b | medium | confirmed | `pages/character-editor/character-editor.ts:1305` (`buildFullValue`), `hit-points-rolls/hit-points-rolls.ts:90-96` | Rolls are sent raw, and `set()` pads with 0. `submit()` never checks that each roll fits its die. The preview already counts such rows as "missing". | Level 4, method "Rolado", only the last row rolled: `[0,0,15]` is sent and accepted. `hitpoints.go` raises an issue and uses the average, so a table that says "roll" gets averages silently. A 20 on a d6, or a roll made before a class switch, goes out the same way. The server's refusal names no field ("Confira os campos da ficha."). | `review15-u15-04.spec.ts`: `expected [ 4, 20 ] to be undefined`; `review15-u15-10.spec.ts`: `expected false to be true` (every sent roll in 1..12) |
| U15-10c | low-medium | confirmed (source level) | `pages/character-editor/character-editor-source.live.ts:~494-500` | `getCampaign(...).catch(() => false)` makes `viewerIsMaster` false on any transient failure. | A network blip while loading the catalog makes a master's editor hide what is switched off for players, with no error. | `review15-u15-10.spec.ts`: `expected false to be true` (`viewerIsMaster`) |

### Unverified candidates (not proved, not refuted)

| id | sev | status | file:line | defect | what would settle it |
|---|---|---|---|---|---|
| U15-11 | low-medium | unverified | `pages/level-up/level-up.ts:485-549`, `:231-283` | `rereadSheet`, `contentChanged` and `load` have no ordering guard. Two overlapping re-reads can leave the older reading on screen, and the loser session is not stopped. | A page spec with two deferred re-reads resolving out of order. |
| U15-12 | low-medium | unverified | `pages/character-sheet/creatures-panel/creatures-panel.ts:~164-170` | Wild Shape state goes stale when the form ends elsewhere. `onVitals` is a no-op and no hint reloads `loadWild`. | A spec or Playwright test with a form ended in another tab, checking the panel. Does the server send `creatures_changed` for it? That is a backend question. |
| U15-13 | low-medium | unverified | `creatures-panel/summon-sheet.ts:~403` (`loadForms`) | No sequence guard (`loadBeasts` has one). | A spec with two quick option changes and out-of-order answers. |
| U15-14 | low | unverified | `creatures-panel/creatures-panel.ts:~200`, `~215` | A failed reload keeps the old list silently. A creature gift is not announced while a cast is in flight. | Specs with a rejected reload and a `creatures_changed` during `cast()`. |
| U15-15 | low-medium | unverified | `pages/character-editor/hit-points-preview.ts:56-77` | The HP preview ignores `hp.max` effects (Hill Dwarf, Draconic Sorcerer) and CON from features. The server does apply them, so the shown number is lower than the sheet's (ADR-0008). | A spec with a Hill Dwarf catalog and the server's `hp.max` modifier. Needs the server number for comparison. |
| U15-16 | low | unverified | `character-editor.ts:~1093`, `~707` (`grantedSpells`, `otherGranted`) | The "Já na ficha" list is set once at load. After changing the subclass or level, old domain spells appear as "da raça, da classe ou de uma característica". | A spec that loads an edit with granted spells, then changes the subclass. |
| U15-17 | low | unverified | `table-ability-scores/table-ability-scores.ts:~295-330` | After `ROLLS_ALREADY_STORED` or `DICE_FORCED_*` the step offers no re-read, so only a reload recovers. | A spec with a rejected `saveDice` followed by a check for a re-read. |
| U15-18 | low | unverified | `pages/character-editor/character-editor.ts` `refreshCatalog` | Two overlapping catalog reads can land out of order and overwrite newer lists (no sequence guard). | A spec with two deferred `loadCatalog` calls. |
| U15-19 | low | unverified | `pages/character-sheet/xp-block/xp-block.ts:~70`, `core/progression/xp-labels.ts:83-91` | The XP block computes "Faltam" and the percentage in the browser. I found no case where it differs from the server. | A server-sent number to compare against. |
| U15-1b | low | unverified | `core/levelup/levelup-draft.ts:374` | The variant of U15-1 where the re-read options are for another class. The verifier's test passed, but it did not confirm the Vida step was on screen. | A page spec that stays on the Vida step after the re-read. |

## Fix direction

**U15-10a (unbound `levelDice`)**
- **Root cause:** the computed `levelDice` is never passed to the child component.
- **Fix:** add `[levelDice]="levelDice()"` at `character-editor.html:480`.
- **Same pattern:** `hit-points-preview.ts:61` and `hit-points-rolls.ts:67` already read it, so nothing else needs wiring. Check any other `<app-hit-points-rolls>` user with a grep.
- **Rules and docs:** ADR-0008 (the server stays the judge).
- **Risk:** low. `character-editor-classes.spec.ts` covers `levelDice()` but not the binding, so extend it.

**U15-4b / U15-3 / U15-6a / U15-6c / U15-11 / U15-13 / U15-18 (late answers landing on a moved-on screen)**
- **Root cause:** async reads write state without checking they are still the latest, or for the current route.
- **Where the fix belongs:** use a per-page sequence counter, as `LevelUpPreview` and `creatures-panel.load` already do. Bump it at the start of every load and every write that sets state (`markDead`, `approve`, `replaceVm`). Drop an answer whose number or ids no longer match. The alternative is `switchMap` on the route params. The counter is simpler and also covers the writes, so I'd pick it.
- **Same pattern elsewhere:** `character-sheet.ts:192-210`; `character-editor.ts:1053` and `loadForCreate`; `creature-page.ts:139-204`; `creatures-panel.ts` `loadWild`; `summon-sheet.ts` `loadForms`; `level-up.ts` `rereadSheet` and `load`; `refreshCatalog`.
- **Also reset on param change:** `loadForEdit` and `loadForCreate` must reset every signal (`selectedSkills`, spells, `extraClasses`, rolls, expertise, forms), and the creature page must set `loading`.
- **Risk:** a write that gets overtaken must still apply its own answer. The guard goes on reads, plus a bump on writes. Existing specs `character-sheet.spec.ts`, `character-editor.spec.ts` and `creatures-panel.spec.ts` guard the normal flows.

**U15-4a (navigate after leaving)**
- **Fix:** after the await, return early if the component was destroyed (`DestroyRef`) or the route params changed. On a created character after "Cancelar", let it be, and do not navigate.
- **Same pattern:** `character-sheet.ts` `reject()` navigates after an await. `level-up.ts` `confirm()` does the same, but it should still navigate, so check it separately.
- **Risk:** low. The idempotency key still protects against a duplicate.

**U15-5 (story revision)**
- **Fix:** capture `vm().revision` in `startEditing()` and send that value. A server `aborted` then shows the stale message.
- **Optional:** the panel could offer to re-read and keep the typed text.
- **Same pattern:** `master-notes` (last save wins by design, so it is fine) and the editor (`s.revision` is read once at load, which is the correct shape).
- **Rules:** RN-01 and the revision check in `UpdateCharacterStory`. Add a spec to `story-panel`'s folder.

**U15-2 (catalog signature)**
- **Root cause:** an equality shortcut over a hand-picked subset of the catalog.
- **Fix:** drop the signature and compare the whole catalog (`RulesCatalogVm` is plain data, so a deep compare or a JSON of everything works). Or always swap the catalog and only decide whether to show the note from the signature. I'd pick the second: swapping always is cheap and always correct, and the note can stay conservative.
- **Same pattern:** `level-up.ts` `offerSignature` is also a subset. It matters only for the note there, but it shares the shape.
- **Risk:** the form keeps what was typed, and picks that left the lists are already handled by `intersectWithAvailable` and `markSwitchedOff`. `catalog-changes.spec.ts` and `character-editor-classes.spec.ts:996` guard that.

**U15-1 / U15-8d (`adopt` carries choices from another level or rule)**
- **Root cause:** `adopt` trusts the old draft's roll and card without comparing the new options.
- **Fix:** in `adopt`, keep an app roll only when the new options' `keptHitPointRoll` equals it (and the class matches), or when level and class are unchanged. Force `hpCard='average'` and `rolled=null` when `hitPointsRule` is AVERAGE_ONLY. Keep physical rolls only while they fit the new die.
- **Alternative:** have `adopt` take the server's kept roll as the only source of an app roll. That is simpler, and the roll then always comes from the server, so I'd pick it.
- **Rules:** RN-18 and RN-24 (a stored roll applies to the level it was made for). Existing guard: `levelup-draft.spec.ts` `adopt` tests and `level-up.spec.ts`.

**U15-8a/b/c (draft sets out of step with the counts)**
- **Root cause:** each pick set is trimmed only in some transitions, and `preparedMaxAfter` has two writers.
- **Fix:** put one `reconcile()` in the draft (prepared to `preparedAsked`, expertise to trained skills, then counts). Call it from `setSubclass`, `adopt` and whenever `preparedMaxAfter` changes. In `adopt`, copy the old `preparedMaxAfter` and trim skills before expertise. Also let the page effect lower the maximum when the preview says 0.
- **Same pattern:** every toggle in `levelup-draft.ts` that depends on another set (`toggleSkill` already cleans expertise, as the model).
- **Risk:** over-trimming silently loses a pick, so say it when it happens, as the content note does.

**U15-9 (summary reads the wrong caster)**
- **Fix:** add `classKey` to `SummaryContext`, pass `options.classKey` from `level-up-session.ts`, and make `cast()` use `spellcasting.find(c => c.classKey === key)`. Count `known` only for that class.
- **Same pattern:** no other production use. `level-up.ts:216` already does it right.
- **Rules:** ADR-0008 (the numbers stay the server's). The spec supplies `classKey` through a cast, so adjust it when the field exists. `levelup-summary.spec.ts` has no multiclass fixture, so the new spec fills that.

**U15-7 (range field)**
- **Fix:** keep the typed metres text in the draft (as `speedM` and `darkvisionM` do in `feature-draft.ts`, and `rangeM` in `spell-draft.ts`) and convert once on save and load.
- **Same pattern:** other editable users already use that shape. Check `npc-short-form/basic-form.ts:148,167`, which maps on load and submit only.
- **Risk:** the saved feet value changes for the entries that were mangled. Existing `effect-picker.spec.ts` guards the happy path.

**U15-4c / U15-10b (unvalidated rolls)**
- **Fix:** in `submit()`, with method "rolled", require every roll to be valid for its die (`validRoll` with `levelDice`). Add the missing levels to the invalid summary. Drop or ignore stored rolls when the class changes the die.
- **Server side:** `abilityscores.go:176` accepts zeros, and `validate.go` accepts 1 to 12 whatever the die. A server fix belongs to the backend units, but the client check does not depend on it. The server's refusal field `full.hit_points.rolls[i]` could also be mapped in `describeCharacterError`.
- **Same pattern:** the level-up typed die is already checked on the server (`HIT_POINTS`).

**U15-10c (swallowed master check)**
- **Fix:** let the failure reach `loadCatalog`'s caller (or retry), so the page shows an error instead of treating a master as a player. Only a `PermissionDenied`/`NotFound` answer should become "not master".

**U15-6b (double tap on "Voltar à forma normal")**
- **Fix:** disable the button while the request is pending, and make the key per intent (an `ActionKey` renewed on success), as the editor does.
- **Same pattern:** other creature-panel actions with `newKey()` per click: check `creatures-panel.ts` and `summon-sheet.ts`.
