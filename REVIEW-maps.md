# Review: maps module after the finish-line refactor

Scope: `backend/internal/maps/**` (with `images`, `refimg`, `dungeonimg`) and the campaign package. Commits compared before and after: #439 (train 3), #445 (train 6), #446 (train 7), #448 (train 8).
Method: `git diff <sha>^ <sha>` per function, `go build`, `go vet`, gocyclo v0.6.0 and gocognit v1.2.0. No database, no Docker, no test run.

**Verdict on behaviour.** I found no behaviour change introduced by a split, and no RN-10 regression. I compared the order of refusals, the `db.InTx` boundaries, the retry resets, writes, stream notices and idempotency keys. Tile masks, the tile and image routes' 404 matrix and the package export were also checked. Two weak spots were carried over unchanged from before the split (maps-2, maps-3). Neither is guarded by a test.

**Verdict on the finish line.** It is not met in this area: see maps-1.

Conventions: severity high = likely bug or leak, medium = hurts maintenance, low = polish. "Changes behaviour" says whether the fix does. Line numbers are on `main` at 35346c9.

## Findings

### Medium

**maps-1 · readability · medium · `imagegen.go:1211`, `imagegen_map.go:262,546,657,453`, `imagegen.go:477`, `images/jpeg.go:32`**
- `.github/go-complexity-baseline.txt` still lists 7 maps functions over the limit, and measuring confirms it:

  | Function | Cyclomatic | Cognitive |
  | --- | --- | --- |
  | `(*Service).run` | 30 | 32 |
  | `checkCharacters` | 21 | 31 |
  | `playersSeenOf` | 20 | 30 |
  | `prepare` | 15 | 20 |
  | `prepareMap` | 14 | 20 |
  | `GenerateMapImage` | 16 | 17 |
  | `images.readJPEGHeader` | 15 | 16 |

- Train 8 touched both files and left these.
- File sizes also contradict "split big files": `imagegen.go` 1480 lines, `dungeons.go` 1361, `fog.go` 1145, `tiles.go` 1120, `imagegen_map.go` 1011, `mapservice.go` 990 (after its "split").
- Fix: finish these 7 functions first (image generation is the whole remaining debt of the module). Then cut `imagegen.go` into request (reserve, begin), run (call, store) and read (`generationToProto`).
- Changes behaviour: no.

**maps-2 · behaviour · medium · `mapupdate.go:66-70`, `mapgrid.go:40-52`, `layers.go:687-700`, `imagegen_map.go:850-860`**
- The four pre-reads use `if err == nil && …`, so every error other than not-found is dropped. The comment says the map is "refused inside the transaction", which is true only for not-found.
- On a pool timeout or a cancelled context, a fog map takes the raw, shared image with no copy.
- RN-10 is held only by the serve route refusing a fog map's image. This is defence in depth lost silently.
- Not introduced by the split, but the split left it in four files.
- Fix: return `s.dbError(...)` unless the error is `pgx.ErrNoRows` (best in the single helper of maps-5).
- Changes behaviour: yes, on a database failure only.
- Test guard: none.

**maps-3 · behaviour · medium · `mapupdate.go:171-178` (`pickMapImage`)**
- The pre-transaction copy is used whenever the image id matches the copy's source. `row.FogEnabled`, the locked value, is never compared with the kind of copy that was prepared.
- A `SetMapFog` between the pre-read and the transaction gives a fog map the raw image. `useFogCopy` (`layers.go:734`) does this check.
- Fix: pass the locked `row.FogEnabled` and drop the copy when it differs from the one the copy was made for, as `useFogCopy` does.
- Changes behaviour: yes, closes the race.
- Test guard: none.

**maps-4 · duplication · medium · `mapservice.go:200`, `layers.go:645`, `imagegen_map.go:780`, `mapupdate.go:198`, `mapgrid.go:221`**
- "Delete the copy that was made for nothing" is written 5 times. `discardUpdateCopy` and `discardGridCopy` are identical, and their `unused bool` parameter reads inverted.
- Fix: one `discardFogCopy(ctx, campaignID, c *fogCopy, used bool)` in `fogimage.go`.
- Changes behaviour: no.

**maps-5 · duplication · medium · `mapupdate.go:62-87` (`updateImageCopy` → `prepareNewImageCopy` → `prepareFogCopy`/`prepareReuseCopy`), `imagegen_map.go:850`, `mapgrid.go:36`, `layers.go:687`**
- "Pre-read the map, then prepare a fog or reuse copy" is written 3 or 4 times. `useImageCopy` is `updateImageCopy` with another error wrapper, and `prepareNewImageCopy` is a pass-through.
- Fix: one `imageCopyFor(ctx, campaignID, mapID, imageID) (*fogCopy, error)` in `fogimage.go`, which returns the raw error. Callers wrap it. This fixes maps-2 in one place.
- Changes behaviour: only maps-2's.

**maps-6 · duplication · medium · `mapupdate.go:145`, `mapgrid.go:103`, `mapcrud.go:325`, `layers.go:199,709`**
- `GetMapForUpdate`, then no-rows → `errMapNotFound`, otherwise "find map", is written 4 times, and once more inline in `paintInTx`. `campaignMap` (`mapservice.go:800`) is the read twin.
- Fix: one `lockMap` beside `campaignMap`.
- Changes behaviour: no.

**maps-7 · duplication · medium · `tiles.go` (`readImageHeader`, `decodeBytesPerPixel`, ~380-410) and `images/images.go` (~336-445)**
- The PNG decode-cost estimate (colour-model table, interlace byte, tRNS chunk) is written twice with different numbers.
- Stored images are already re-encoded to 8 bits, so the 16-bit branches in `tiles.go` are mostly unreachable.
- Fix: export `images.DecodeCostPerPixel` and call it from `decodeWorkingCopy`.
- Changes behaviour: possibly the budget for gray16, which is unreachable. Cover it with the `TestTileBudget_*` tests first.

**maps-8 · duplication · medium · `clues.go:336,357` and `traps.go:208,228`**
- `parseRevealIDs` is `parseCharacterIDs`, and `revealRecipients` is `playerCharacters(..., needUser=true)`.
- Fix: `RevealSceneClue` calls `s.playerCharacters(ctx, campaignID, ids, true)`. Delete the `clues.go` pair. Check first that both refusals have the same code.
- Changes behaviour: no, if the refusals match.

**maps-9 · duplication · medium · `clues.go:225` / `scenes.go:359` (also `clues.go:130` / `scenes.go:183`, and `addClueCall` / `addActionCall`)**
- `moveClueRows` and `moveActionRows` are the same swap-and-renumber loop. `insertClue` and `insertAction` repeat the same position-and-limit logic.
- Fix: one generic `renumber` and one `nextPosition`, with thin callers.
- Changes behaviour: no.

**maps-10 · duplication · medium · tests (`fogstreams_golden_test.go:33`, `layersgolden_test.go:100`, `imagestore_freeze_test.go:61`, `package_freeze_test.go:39`)**
- Four copies of "compare with the golden, or write it" with three different update switches: `-update-fog-golden`, `-update-layers-golden`, env `UPDATE_IMAGE_GOLDENS`, env `UPDATE_PACKAGE_GOLDENS`. `uuidPattern`/`maskUUIDs` is a fifth copy of the id scrubber.
- The same happened across the repo: characters and play have a dozen more flags.
- Fix: one `checkGolden(t, path, got)` in `harness_test.go` with a single `-update` flag for maps.
- Changes behaviour: no (tests only).

**maps-11 · readability · medium · `mapservice.go:76-725` vs `mapcrud.go`, `mapupdate.go`, `mapgrid.go`, `pointwrites.go`**
- Each RPC handler sits in `mapservice.go` and its helpers in another file. `pointChange` is declared at `mapservice.go:389` and its methods are in `pointwrites.go`. `mapcrud.go` holds the helpers of four calls, so it is not a CRUD file.
- To read `UpdateMap` you open two files.
- Fix: move each handler next to its helpers. `mapservice.go` keeps `ListMaps` and the shared lookups. Rename `mapcrud.go` to `mapread.go` or split it. Pure move.
- Changes behaviour: no.

**maps-12 · readability · medium · `mapcrud.go:185`, `mapupdate.go:90`, `layers.go:176-187`, `imagegen_map.go:798`, `pointwrites.go`**
- Parameter structs with pointer out-params that each have one caller and remove no argument: `newMap`, `mapUpdateTx` (embeds `mapUpdate`), `gridChange`, `useCall`+`useOutcome`, `paintBatch`/`paintDone`, `pointChanged`/`pointDeleted`/`tokenPlacement`, `reserveIn(n, res)`, `putToken(..., pl)`.
- Fix: return values instead of out-params. Drop `mapUpdateTx`. Make `pointText` a plain return. Keep the per-attempt reset visible in the closure.
- Changes behaviour: no. Re-run the retry test (`createmapretry_test.go`) and write the same for `UpdateMap`, `SetMapGrid`, `SetMapFog` first (maps-26).

**maps-13 · readability · medium · pass-through helpers, one caller each**
- `createMapTx` (`mapcrud.go:194`), `mapFogView` (`:64`), `isEmptyPointChange` (`pointwrites.go:165`), `checkChangeSceneOnlyData` (`:322`), `carriedKey` (`carriedlight.go:69`), `revealToAll` (`traps.go:400`), `checkTreasureVersion` (`treasure.go:333`), `treasureInputs` (`treasure.go:299`), `checkImagesInCampaign` (`sceneimages.go`), `claimImageFile` (`package.go`), `readRedrawMap` (`dungeons.go:646`, called only by `readRedrawSource`).
- Fix: inline each. The announce helpers stay only if they sit beside their handler (maps-11).
- Changes behaviour: no.

**maps-14 · readability · medium · name noise**
- `…In`: `revealTrapIn` (`traps.go:365`), `markTreasureIn` (`:480`), `unmarkTreasureIn` (`:569`), `disarmTrapIn` (`traps_play.go:830`), `placeTreasureIn` (`treasure.go:357`), `reserveIn` (`imagegen.go:641`), `useInTx` (`imagegen_map.go:863`), `paintInTx` (`layers.go:196`).
- `…Tx`: `redrawTx` (`dungeons.go:724`), `createPointTx`, `changePointTx`, `deletePointTx`, `placeTokenTx`, `deleteTokenTx`, `writeMapUpdate`, `writeGrid`.
- `…Rows`: `addClueRows`, `revealRows`, `addActionRows`, `updateActionRows`, `moveActionRows`, `moveClueRows`.
- Others: `fillPointText`/`fillPointSpec` actually validate (`package.go`); `stageMapShell`, `readExportStart` (it authorizes and rate-limits), `readImportStart`; `useCall.changed()` (`imagegen_map.go:817`) returns an error, not a bool.
- Fix: name by effect: `findTreasure`, `unfindTreasure`, `disarm`, `insertTreasure`, `swapRedrawnImage`, `appendClue`, `giveClue`, `checkPoint*`, `checkExportStart`, `errMapChanged`.
- Changes behaviour: no.

**maps-15 · readability · medium · result structs written through a pointer: `trapReveal` (`traps.go:356`), `treasureChange` (`:471`), `trapDisarm` (`traps_play.go:820`), `treasurePlacement` (`treasure.go:271`), `revealCall` (`clues.go:289`), `addClueCall`, `addActionCall`**
- `revealToCharacters` (`traps.go:407`) returns `gave` and also appends to `res.newUsers`.
- Fix: return the values and reset them in the closure. `trapRevealCall` and `dungeonCreate` forward three fields, so use plain parameters.
- Changes behaviour: no.

**maps-16 · readability · medium · `tiles.go` (~777-800: `showMask`, `pixelSquares`, `aroundKnown`, `tileRect`, `paintTile`, `encodeTile`)**
- `renderTile` became 7 one-caller helpers, and `showMask` takes 8 parameters (`x0,y0,w,h` carried across 5 signatures). This is RN-10 pixel code.
- Fix: `type tileBox struct{x0,y0,w,h int}` from `tileRect`. Merge `pixelSquares` and `aroundKnown` into `showMask`. Inline `beltSide = 3` with a one-line why (the constant's comment says "side of the block" but it is a 3×3 neighbourhood). Keep `encodeTile`.
- Changes behaviour: no. The byte-digest test must stay green.

**maps-17 · readability · medium · `tiles.go:965-972` (`tileAccess`)**
- The field `master` is set at `:1069` and never read. The struct is built once to be passed twice.
- Fix: drop `master`. Shrink the struct to `{userID, viewer, as, src}` or pass the three values.
- Changes behaviour: no.

**maps-18 · readability · medium · `package.go` (`stagePoint` → `checkPoint` → `fillPointText`/`fillPointSpec`/`fillPointStairs`/`stagePointTarget`; `stageMap`/`stageMapShell`/`stageMapPoints`; `stageListed`)**
- Two return conventions are mixed in one chain: `bool` and `reason` compared to `reasonNone`. `checkPoint` has 6 parameters, one of them a `knownKind bool`.
- Problem ownership is inconsistent: the caller records `stageMapShell`'s failure, but `stageMapPoints` records its own.
- `stageListed` returns the bytes added, and 0 also means "problem".
- Fix: one convention `(reason, ok)`. Inline the unknown-kind test in `stagePoint`. `stageListed` returns `(bytes, ok)`.
- Changes behaviour: no.

**maps-19 · readability · medium · `upload.go:~249` (`imageStore` / `storeImage`)**
- `imageStore` copies the 9 parameters of `storeImage`, and `storeImage` still takes the 9. Two callers (`upload.go:246`, `imagegen.go:1324`).
- Fix: callers build `imageStore`. Delete the 9-argument signature.
- Changes behaviour: no.

**maps-20 · readability · medium · `gallery.go:~160-235`**
- `deleteImageRows` is only the `db.InTx` wrapper, and `deleteImageTx` is its closure body. `publishImageDeleted(campaignID, wasShown, hadPortraits, wasLeft)` takes three unlabelled bools.
- Fix: merge the two delete functions. Keep the three publish `if`s in `DeleteGalleryImage`.
- Changes behaviour: no.

**maps-21 · readability · medium · `campaignpackage/rpc_export.go:~96-160`**
- `StartCampaignExport` → `startExport` → `insertExportUnlessRunning`, with a named result written from a closure inside `idem.Create`.
- Fix: merge `startExport` and `insertExportUnlessRunning`. Rename `readExportStart` to `checkExportStart` (and `readImportStart` to `checkImportStart`, where `resumeImport` says "an hour more" and means `UploadTTL`).
- Changes behaviour: no.

**maps-22 · readability · medium · `dungeons.go:229,622`**
- `readDungeonCreate` returns `(in, replay, err)`. `dungeonKeyReplay` returns `nil, nil` under `//nolint:nilnil`, and `dungeonReplay` is another name for nearly the same idea. `readRedrawSource` → `readRedrawMap` returns a four-value tuple, all zero on every error.
- Fix: `readDungeonCreate` returns only `in`, and the caller handles the replay. Merge the two redraw readers into one returning `*redrawSource`. Rename one of the replay helpers.
- Changes behaviour: no.

**maps-23 · readability · medium · `package.go:192-194`**
- `si.parentOld` and `si.copyOld` are set by `in.IDs.Ref(...)` and then overwritten on the next line. The `Ref` calls stay only for the recorded problem.
- Fix: call `Ref` without assigning, with a one-line why, or drop them if the problem is recorded elsewhere. Check `package_stage_images.golden.txt` first.
- Changes behaviour: no.

**maps-24 · docs · medium · audit ids in comments (RN/MR ids stay)**
- Locations: `layers.go:25,478,569`; `visibility.go:24,132` (`Etapa 9, pointVisible`: that function no longer exists); `fog.go:23-42`; `carriedlight.go:18,109`; `treasure_convert.go:17`; `traps_play.go:27,413` (`E9-08 G`); `traps.go:25`; `clues.go:28`; `spec.go:20`; `treasure.go:24`; `dungeons.go:33`; `dungeonimg/dungeonimg.go:2`; `mapcrud.go:25,156`; `mapgrid.go:141`; `mapupdate.go:124`; `pointwrites.go:360`; `imagegen_map.go:259`; `imagegen.go:591` (`#121 rule`); `tiles.go:33`.
- Fix: replace each with the rule in words and delete the id.
- Changes behaviour: no.

**maps-25 · docs · medium · broken and drifted references**
- `tiles.go:57` cites `CONTRIBUTING, "As medidas dos tiles da névoa"`. That section does not exist. The numbers are in `docs/performance.md` (`:18`).
- `docs/architecture/maps/editor.md:4` says "The server code is in `layers.go`, `calibration.go` and `mapservice.go`", but `SetMapGrid` and `UpdateMap` helpers live in `mapgrid.go` and `mapupdate.go`.
- `docs/architecture/maps/points.md:5` lists `spec.go`, `traps.go`, `traps_play.go`, `carriedlight.go` and misses `applyspec.go` and `pointwrites.go`; it says `traps.go` is "revealing a trap, treasure found", which also holds the transaction helpers.
- Fix: rewrite those sentences. Prefer a path to a package part and a subject over a file list (the list is what rots).
- Changes behaviour: no.

**maps-26 · tests · medium · no test guards the per-attempt reset on a retry for `UpdateMap`, `SetMapGrid`, `SetMapFog`, `UseGeneratedImage`, nor `trapReveal`/`call.newUsers[:0]`**
- Only `createmapretry_test.go` covers it for `CreateMap`. The new code keeps the values per attempt, so it is correct today, but the old code needed the reset and nothing pins it.
- Also unguarded: `context.WithoutCancel` in `CreateDungeonMap`; the `viewTilesOf` cache-hit path (it returns `acc.src`, not the fresh row); `Content-Disposition` in both image routes; the speed of `dungeonimg.Render` after it became per-pixel method calls (a pixel hash exists, no benchmark).
- Fix: one table test with a fault injector for the 40001 retry (like `createmapretry_test.go`). Add `-bench` for `Render` before touching it.
- Changes behaviour: no (tests only).

### Low

**maps-27 · duplication · low · `findPointForUpdate` (`pointwrites.go:153`) vs inline copies in `traps.go:282`, `scenes.go:462`, `puzzleseam.go:172`.** Call the helper. No behaviour change.

**maps-28 · duplication · low · `storeLayers` (`layers.go:273`) vs inline `UpsertMapLayers`+`nilIfBlank` in `puzzleseam.go:82`, `dungeons.go:372`, `calibration.go:49`, `package.go:892`.** Call it, after checking the error wrapping is the same. No behaviour change.

**maps-29 · duplication · low · the "insert the copy, `SetMapImageOnly`, wrap" block in `applyFirstGridFog` (`mapgrid.go:190-205`) and `useFogCopy` (`layers.go:733-745`).** One `giveMapItsCopy`. No behaviour change.

**maps-30 · duplication · low · `setImageHeaders` (`serve.go:~189`) vs `setTileHeaders` (`tiles.go:~1097`).** The lists of hardening headers already differ (`Content-Disposition`). Share a `setPrivateImageHeaders`. Changes behaviour only if you unify `Content-Disposition`: decide it on purpose.

**maps-31 · readability · low · `creatureTokens` (`fog.go:482`)** is a lazy loader with a back-pointer, and its name collides with `creaturetokens.go` and the local in `mapcrud.go:113`. A memoising closure is shorter. Rename to `creatureSquares`.

**maps-32 · readability · low · `SetMapTokenHidden` (`mapservice.go:642-690`)** is the only point/token handler with its transaction inline. Make it like the others, or stop extracting the others (see maps-11).

**maps-33 · readability · low · `layers.go:653`** `playersRead :=` shadows the function `playersRead` (`layers.go:308`). Rename the local to `playersHear`.

**maps-34 · readability · low · `carriedLights` (`fog.go:571`) takes `s` only for `s.rules`; `livingCarriers` (`:557`) is a thin wrapper of `MapCharacters`.** Merge `livingCarriers` into `lightSources`.

**maps-35 · readability · low · `treasure.go`:** `coinsLines`, `piecesLines`, `itemsLines` and `itemLine` are four one-use helpers for one text. `treasureSquareCenter` returns `(int, int, error)` and `insertTreasurePoint` takes `center [2]int`: pass a square. `noticerOf` (`traps_play.go:747`) and `placeTreasureIn` take 8 parameters.

**maps-36 · readability · low · `dungeonimg.go:100-144`:** `inked` is `edgeInked || cornerInked`. Call both from `paintRow` after the benchmark in maps-26.

**maps-37 · readability · low · comments that restate the code:** `applyTo`, `floorHolds`, `characterIDs` (`traps.go`), `tileRect`. Delete them. The comments on `redrawFreed`, `revealRecipients` and `oldImageGoes` say why and stay.

**maps-38 · readability · low · floating "phase" header comments** (`mapcrud.go:19,166,239`; `mapgrid.go:16`; `mapupdate.go:17`; `applyspec.go:11`; `pointwrites.go:19-24`, which says "which these call exactly as the handlers did"). They restate the file's function list or narrate the refactor. Delete them, and keep one-line comments only for hidden rules (RN-10, RN-24).

**maps-39 · readability · low · `upload.go:~264` (`storeImage` doc)** is one 140-character sentence with a parenthesis that never closes. Rewrite it as 3 short sentences. Same for the garbled line break at `pointwrites.go:337-339`.

**maps-40 · readability · low · `campaignpackage/export.go:162-205`:** `exportProgress` holds `s *Service` only to call a method; the `e.blob != ""` test is repeated in `writeZip` and `writeEntry`. `sweep.go:55-95` has two copies of the batch loop (two uses: leave it).

**maps-41 · tests · low · test scaffolding duplicated:** `trapCallsTable.heard` (`trap_calls_test.go:53`) and `trapPlayTable.heard` (`trap_disarm_test.go:43`) are the same function. Ten or more `want*Refusal` helpers (`wantTrapCallRefusal`, `wantTrapPlayRefusal`, `wantDungeonRefusal`, `wantTrapSpecRefusal`, `sceneRefusal`, and others in the area) repeat one body. One `wantRefusal(t, err, code, msg)` in `harness_test.go`.

**maps-42 · docs · low · test headers narrate the refactor** ("written on the code before it was split into phases", "frozen here before … is split", "digests recorded on the code before the refactor", "step 1"): `tiles_freeze_test.go:21,141`, `imageedit_freeze_test.go:13`, `package_freeze_test.go`, `mapupdate_test.go:14`, `mapgrid_test.go:17`, `mapcrud_test.go:17`, `pointwrites_test.go:14`, `applyspec_test.go:16`, `layersgolden_test.go`, `trap_calls_test.go:14`, `trap_disarm_test.go:12`, `sceneedit_freeze_test.go:20`, `imagegen_reserve_test.go:15`. Say what the test guards and how to regenerate it. Also rename files and tests called `*_freeze_*`/`…IsFrozen`: that is the refactor's word, not the behaviour's.

## Test trim

Test files and goldens added today in this area, with a class for each. "Lines" are the file's lines. The estimates were not measured by a run.

| File | Lines | Class | Reason |
| --- | ---: | --- | --- |
| `tiles_freeze_test.go` · `TestTilesAreByteForByteFrozen` | ~150 | keep | The byte-for-byte RN-10 net for each viewer |
| `tiles_freeze_test.go` · `TestBlackenBlocksIsFrozen` | ~45 | delete | Internal mask, proven by the digests and `TestRN10_JPEGKeepsTheInteriorOfAKnownArea` |
| `tiles_freeze_test.go` · `TestBuildTilesIsFrozen` | ~45 | merge | Into `TestRN10_TheIndexListsOnlyTilesWithAKnownSquare` |
| `tiles_freeze_test.go` · `TestDecodeWorkingCopyIsFrozen` | ~55 | merge | As rows of `TestTileBudget_*` |
| `tiles_freeze_test.go` · `TestTileServeIsFrozen` | ~95 | merge | Into `TestRN10_TileAuthorization` (same 404 matrix) |
| `package_freeze_test.go` + 5 `package_*.golden.txt` | 685 + 770 | keep, trim | Staging order and problems are behaviour. `package_stage_point.golden.txt` (289 lines) can drop to the distinct cases (about −150) |
| `imagestore_freeze_test.go` · `TestStoreImageIsFrozen` | ~130 | keep | Quota, parent and rollback order |
| `imagestore_freeze_test.go` · `TestServeImageIsFrozen` + `image_serve.golden.txt` | ~75 + 22 | merge | Into `TestUploadServeRenameDelete` and the RN-10 serve tests |
| `imageedit_freeze_test.go` | 210 | keep | Refusal order and who is told |
| `layersgolden_test.go` + 29 `layers_*.golden.json` | 716 + ~6,700 | merge | Keep visitor, one player and master per scene of the fog case plus Mirathel. The `nofog`/`group` × `master_as_pens`/`master_as_toren` combinations repeat `layers_test.go`'s leak tests. Keep `…Refusals`, `…OpenDoors` (order of refusals is behaviour). Target about 12 golden files (−3,500 lines) |
| `fogstreams_golden_test.go` + 3 goldens | 296 + 280 | keep | Who is told of each token write is RN-10 and was thin before |
| `fogsight_golden_test.go` + 5 small goldens | 400 | merge | `NewSight`/`UpdateVision` pin helper internals; keep `LightSources` and `FamiliarEyes` (−150) |
| `applyspec_test.go` | 508 | merge | Keep `…Matrix`. `_TheFirstTriggerIsKept` and `_WhatAKindChangeForgets` overlap it (−100) |
| `mapcrud_test.go` / `mapgrid_test.go` / `mapupdate_test.go` / `pointwrites_test.go` | 536 / 211 / 287 / 473 | keep, trim | Refusal order and who hears are behaviour. Merge the `…Refusals` tables that repeat `maps_test.go` and `stories_test.go` (−250). Keep `…ACopyOfNoUseIsDeleted` |
| `sceneedit_freeze_test.go` | 547 | merge | Keep refusals, limits and replays. `…ResultsAreFrozen` repeats `scenes_test.go`, `scene_options_test.go`, `stage_test.go` (−200) |
| `trap_calls_test.go` / `trap_disarm_test.go` | 284 / 248 | keep, trim helpers | Order and who hears are real; share `heard` and the refusal helper (−60) |
| `trap_spec_clean_test.go` | 236 | keep | Pure and fast, needs no database |
| `dungeons_refusals_test.go` | 249 | keep | Refusal order is user-visible |
| `treasure_test.go` (+66) | 66 | keep | Pins the check order of `PlaceTreasure` |
| `imagegen_reserve_test.go` / `imagegen_use_test.go` | 205 / 99 | keep | Reach `reserve`/`use` checks the RPCs cannot reach |
| `images/cropfit_freeze_test.go` | 64 | keep | Pins the memory-budget order; cheap |
| `images/gen/freeze_test.go` | 156 | keep one, merge | `TestParseAnswerSentencesAreFrozen` pins log text only: merge into the gemini tests (−40) |
| `dungeonimg/render_golden_test.go`, `refimg/render_golden_test.go` | 76, 91 | keep | The only guards of the renderers |
| `campaignpackage/characterization_faults_test.go` | 709 | keep | Request-order and fault paths |
| `campaignpackage/reader_characterization_test.go` · `TestCharacterizeFileNameEdges` | ~100 | merge | Into `TestFileNamesAreSafe` (`reader_test.go:358`) |
| the four golden helpers | ~80 | merge | See maps-10 (−60) |

**Estimated saving:** about 1,400 lines of Go test plus about 4,000 lines of golden files, roughly 20% of the 11,000 lines of tests added today in this area. CI time is probably 5 to 15 seconds, mostly the one-connection fixture, not tile rendering. I did not measure it.
