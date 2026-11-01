# Performance budgets

The server is small (see [Operations](operations/README.md#what-runs-where) for its CPU, memory and `GOMEMLIMIT`), so the work that can grow with user input has a budget in code.
This page lists each budget and where it is set, then the commands that measure the expensive paths.
Timings depend on the machine, so measure on yours with the commands below; the one section with numbers ([Table content at its limits](#table-content-at-its-limits)) says which machine they come from.
How the deployed numbers add up against the memory limit is in [Operations](architecture/maps/fog.md#per-player-image-tiles).

## Budgets the code enforces

| Budget | Value | Set in |
| --- | --- | --- |
| Uploaded image file | 10 MiB, before and after re-encoding | `images.MaxBytes`, `backend/internal/maps/images/images.go` |
| Uploaded image size | 8,192 px per side and 40 megapixels | `images.MaxSide`, `images.MaxPixels` |
| Decode memory of an upload | 256 MiB estimated; more is refused as `DIMENSIONS` | `maxDecodeBytes` in `images.go` |
| Decode, resize and crop of a model answer | 178 MiB | `maxFitBytes` in `images.go` |
| Image processing slot | One image decoded at a time on the server | `processing` in `backend/internal/maps` |
| Generated images | One call to the model at a time; an answer of at most 8 MiB | `maxGenerating` in `imagegen.go`; `maxResponseBytes` in `images/gen/gemini.go` |
| Fog tile size | 16 x 16 squares | `tileSquares` in `backend/internal/maps/tiles.go` |
| Working copy of a fogged map image | Long side capped at 2,048 px; two maps at once, least recently used first | `workingMaxSide`, `workingCopies` |
| Finished-tile cache | 32 MiB of PNG bytes, counted in bytes | `tileCacheBytes` |
| Image refused for a tile | Decoding would take more than 192 MiB (answered `503`) | `decodeLimit` |
| One render at a time | A request waits 8 s for its turn, then gets `503` with `Retry-After`; a user's cache misses are also rate limited | `renderWait`, `retryAfterSeconds`, `tileRenderer.gate` |
| Compiled fog scenes | 8 scenes, 24 viewers' views each | `litCacheSize`, `viewsPerScene` in `maps/fog.go` |
| Live table contents | 8 contents and 24 MiB of stored data; the newest always stays | `maxLiveContents`, `maxLiveBytes` in `backend/internal/characters/tablesource.go` |
| Table-content entries | 1,000 per campaign, 64 KiB each | `rules.MaxOverlayEntries`, `characters.MaxTableEntryBytes` |
| Option switches in one call | 1,400 (the SRD's 358 options and 1,000 table entries, with room) | `MaxOptionSwitches` in `backend/internal/characters/tableswitches.go` |
| Table-content pack | 3 MiB of proto JSON | `MaxPackBytes` in `backend/internal/characters/tablecontent_pack.go` |
| Content catalogs | 8, holding their content weakly | `maxCatalogs` in `backend/internal/characters/contentsource.go` |
| Generated-image requests kept alive | 5 | `maxPendingRequests` in `backend/internal/maps/imagegen.go` |
| One table overlay | 6,000 effects, 500 distinct formulas and 100 KiB of their text, 60 traits per race, 20 effects per feature | `MaxOverlayEffects`, `MaxOverlayFormulas`, `MaxOverlayFormulaBytes`, `MaxTraits`, `MaxFeatureEffects` in `backend/internal/rules/overlay_budget.go` |
| One formula | 1,024 bytes and 256 syntax nodes | `MaxSourceBytes`, `MaxNodes` in `backend/internal/rules/formula/formula.go` |
| Database pool | 10 connections in production | `defaultMaxConns` in `backend/internal/platform/db/db.go` |
| Dungeon size | 15 to 199 wide, 15 to 399 high | `Options` in `backend/internal/rules/dungeon/options.go` |
| Web initial bundle | 500 kB (warning from 490 kB) | `web/angular.json`; see [CI](ci.md#the-web-bundle-budget) |

Request rate limits and per-account caps are in [Architecture](architecture/api.md#abuse-limits).
Nothing the tile and scene caches hold is stored on disk or in the database: a restart empties them and they rebuild themselves.

## Table content at its limits

A campaign holds at most 1,000 table entries and 6,000 effects (the distinct formulas stay at 500).
The numbers below were measured on 4 vCPUs of an Intel Xeon at 2.1 GHz (`GOMAXPROCS` 4, 16 GiB), Go 1.27, without `-race`, with the commands in the last table of this page.
The tables are built like a large table's books: mostly spells, then subclasses with features, subraces, races, feats, backgrounds and a few classes, every name and number different.

`Content.With` (the cost of a content cache miss), by entries, with the effects such a table has (about 1.5 effects an entry):

| Entries | Time | Allocated | Allocations | Kept by the content |
| --- | --- | --- | --- | --- |
| 300 | 17.3 ms | 3.5 MB | 55,000 | 1.5 MB |
| 450 (a table whose books come to that) | 20.1 ms | 4.4 MB | 63,600 | 2.0 MB |
| 600 | 22.4 ms | 5.0 MB | 72,000 | 2.2 MB |
| 800 | 24.1 ms | 6.3 MB | 83,000 | 2.8 MB |
| 1,000 | 28.4 ms | 8.3 MB | 95,700 | 3.7 MB |

The cost is a fixed 3.5 ms and 1.4 MB for the SRD clone plus about 25 microseconds and 7 KB an entry, so it grows with the entries, not faster.
The realistic 300-entry table of `BenchmarkWith` takes 19 ms and 3.8 MB on this machine, inside the 25 ms and 4 MB the design budgeted.

At 1,000 entries, by effects:

| Effects | Distinct formulas | Time | Allocated | Allocations | Kept by the content |
| --- | --- | --- | --- | --- | --- |
| 2,000 | 100 | 36.5 ms | 13.5 MB | 125,800 | 4.4 MB |
| 4,000 | 100 | 39.5 ms | 15.0 MB | 136,000 | 5.1 MB |
| 6,000 | 100 | 40.4 ms | 16.6 MB | 146,100 | 6.0 MB |
| 6,000 | 480 | 75.9 ms | 32.9 MB | 237,600 | 6.8 MB |

A plain effect costs about 1 microsecond; what costs is a distinct formula, about 90 microseconds and 43 KB each to compile.
`TestWithAtTheBudgets` (6,000 effects of 300 entries with 500 distinct formulas) takes 45 ms.

The live source (through the database) for a campaign:

| Entries | Miss (read, decode, `With`) | Hit | One cached content keeps |
| --- | --- | --- | --- |
| 300 | 36 ms | 0.6 ms | 1.6 MB |
| 450 | 29 ms | 0.6 ms | 2.2 MB |
| 1,000 | 80 ms | 0.6 ms | 4.0 MB |
| 1,000, 6,000 effects | 91 ms | 0.7 ms | 7.3 MB |

A revision bump (any write or switch) makes the next read a miss; the old content stays until it is the least recently used.
The cache keeps at most 8 contents, so the worst case is 8 times 7.3 MB, about 59 MB, 15% of the 400 MiB `GOMEMLIMIT`; the 24 MiB of stored data (`maxLiveBytes`) is not reached by 1,000 entries of 1 to 1.3 MiB and holds back a table of long texts.

What a person waits for, with that content cached:

| Entries | Sheet read | Level-up options | "Magias" (every page of 400) | "Magias" (first page of 100) |
| --- | --- | --- | --- | --- |
| 300 | 4.8 ms | 6.3 ms | 7.5 ms (2 pages) | 2.9 ms |
| 450 | 4.6 ms | 6.7 ms | 6.7 ms (2 pages) | 2.7 ms |
| 1,000 | 4.4 ms | 6.3 ms | 11.2 ms (3 pages) | 2.8 ms |
| 1,000, 6,000 effects | 5.3 ms | 7.6 ms | 10.6 ms (3 pages) | 2.7 ms |

A sheet read walks the character's effects, not the table's, so it does not grow with the entries (about 2,300 allocations and 260 to 430 KiB).
The lists the master and the players read, at 1,000 entries: `ListTableEntries` 48 ms and 776 KiB (79 ms and 942 KiB with 6,000 effects), `ListContent` 6 ms and 143 KiB, `ListOptionSwitches` 7 ms and 58 KiB with its 1,358 options.
The largest request the server takes is 4 MiB (the read limit of `ImportTableContent`, see [Request size limits](architecture/api.md#request-size-limits)) and a pack of 1,000 entries is 1.1 to 1.4 MiB of JSON.

`SetOptionSwitches` with the database, a campaign of 1,000 entries (switching the keys off, then on):

| Keys in the call | Off | On | Answer |
| --- | --- | --- | --- |
| 450 (a book's worth) | 119 ms | 155 ms | 23 KiB |
| 700 | 164 ms | 178 ms | 34 KiB |
| 1,000 | 203 ms | 195 ms | 47 KiB |
| 1,358 (the whole campaign: 1,000 entries and 358 SRD options) | 189 ms | 189 ms | 64 KiB |

With 6,000 effects the same calls take 104, 233, 258 and 312 ms.
There is no grouping of entries by book, so switching a book off is the master choosing its keys, one call of at most 1,400.

A formula may have 1,024 bytes and 256 nodes, enough for a table class's 20-level table written as a sum of steps.
The biggest formula of each shape the limits take, compiled once when a content loads and evaluated on a sheet read:

| Shape | Limits of 256 bytes and 64 nodes | Limits of 1,024 bytes and 256 nodes |
| --- | --- | --- |
| Steps of a class table, longest class key | 191 bytes: 0.14 ms, 52 KB to compile; 1.0 microseconds to evaluate | 967 bytes: 0.41 ms, 105 KB; 3.0 microseconds |
| Steps with a one-letter key | 187 bytes: 0.20 ms, 68 KB; 1.9 microseconds | 804 bytes: 0.66 ms, 197 KB; 5.3 microseconds |
| Sum of ones (the most nodes) | 63 bytes: 0.14 ms, 58 KB; 0.2 microseconds | 255 bytes: 0.40 ms, 130 KB; 0.2 microseconds |
| Calls inside calls (the deepest tree) | 175 bytes: 0.21 ms, 80 KB; 3.2 microseconds | 687 bytes: 0.64 ms, 207 KB; 12.9 microseconds |
| Chain of conditions | 253 bytes: 0.21 ms, 70 KB; 0.6 microseconds | 995 bytes: 0.65 ms, 167 KB; 0.9 microseconds |

The nodes, not the bytes, bound the work (two bytes can be a node), so the limit of nodes rises with the bytes.
A formula compiles once per distinct text, so the text of a table has a budget too: 6,000 effects with formulas of about 950 bytes cost 78 ms and 27 MB of a `With` for 100 distinct formulas, and 310 ms and 106 MB for 500, so the distinct texts add up to at most 100 KiB.

## Time budgets the tests check

Some tests assert a time budget, but only with `MEURPG_MEASURE=1`, without `-race` and on a quiet machine: on a busy machine or in CI a clock test fails for nothing.

- Building a table overlay with `Content.With` at every budget stays under 100 ms (`TestWithAtTheBudgets`: 6,000 effects, 500 distinct formulas, 300 entries).
- The dungeon generator at its largest size and `PromptOf` in the worst case stay inside the budgets stated in `backend/internal/rules/dungeon` (`BenchmarkGenerate`, `BenchmarkWorst`, `BenchmarkPromptOf`).
- Fog vision for the 200 x 400 map has a budget in `backend/internal/rules/vision/bench_test.go`.

## Commands that measure

Run them from `backend/`, without `-race` (the detector multiplies time and memory). Tests that need a database take `MEURPG_TEST_DATABASE_URL`, see [Testing](testing.md#integration-tests).

| What | Command |
| --- | --- |
| The rules engine, one `Derive` (runs on every sheet read) | `go test ./internal/rules -run '^$' -bench Derive -benchmem` |
| Loading the SRD | `go test ./internal/rules -run '^$' -bench LoadSRD -benchmem` |
| A table overlay | `go test ./internal/rules -run '^$' -bench BenchmarkWith -benchmem` (`BenchmarkWithEntries` at 300 to 1,000 entries, `BenchmarkWithEffects` at 1,000 entries and 2,000 to 6,000 effects); memory: `MEURPG_MEASURE=1 go test ./internal/rules -run 'TestWithMemory' -v` (`TestWithMemoryByEntries`, `TestWithMemoryByEffects`); the live source, a sheet read, the spell list and the switches with the database: `MEURPG_MEASURE=1 go test ./internal/characters -run TestTableContentScale -v`; at the budgets: `MEURPG_MEASURE=1 go test ./internal/rules -run TestWithAtTheBudgets -v` |
| The biggest formulas the limits take, compile and evaluation | `go test ./internal/rules/formula -run '^$' -bench Worst -benchmem`; inside a `With`: `go test ./internal/rules -run '^$' -bench BigFormulas -benchmem` |
| Fog vision (cave, medium and largest maps, dark and lit) | `go test ./internal/rules/vision -run '^$' -bench . -benchmem` |
| A player's filtered map read, without the database | `go test -run '^$' -bench FilteredMap -benchmem ./internal/maps` |
| The whole `GetMap` with fog, with the database | `go test -run TestFogGetMapTiming -v ./internal/maps` |
| Memory of compiled scenes | `go test -run TestSceneMemory -v ./internal/maps` |
| Fog tiles: cold, warm, JPEG | `go test -run '^$' -bench Tile -benchmem ./internal/maps` |
| Tile memory and a request through the whole route | `MEURPG_MEASURE=1 go test -run 'TestTileMemory\|TestTileRequestTiming' -v ./internal/maps` |
| Dungeon generator, from 31 x 31 to 199 x 399, and the constructed worst cases | `go test ./internal/rules/dungeon -run '^$' -bench 'Generate\|Worst\|PromptOf' -benchmem` |
| Drawing a dungeon | `go test -run '^$' -bench Render ./internal/maps/dungeonimg` and `go test -run '^$' -bench DungeonImage -benchmem ./internal/maps` |
| The whole `CreateDungeonMap` call | `go test -run TestDungeonMapCreationTiming -v ./internal/maps` |
| The drawing sent to the image model, biggest map | `MEURPG_MEASURE=1 go test -run TestMeasureTheBiggestMap -v ./internal/maps/refimg ./internal/maps` and `go test -bench BiggestMap -benchmem ./internal/maps/refimg` |
| Image resizing and the worst-case crop | `MEURPG_MEASURE=1 go test -run 'TestShrinkMemory\|TestCropFitMemory' -v ./internal/maps/images` |
| Image generation answers | `MEURPG_MEASURE=1 go test -run TestMeasureMemory -v ./internal/maps/images/gen` |
| Treasure and encounter generators | `go test ./internal/rules -run '^$' -bench 'GenerateTreasure\|GenerateEncounter' -benchmem` |
| Puzzle maths | `go test ./internal/rules/puzzle -run '^$' -bench .` |

### The map editor in the browser

Painting on a big map must not freeze the screen: `PaintedLayers` (`web/src/app/core/maps/paint-layers.ts`) decodes the whole grid once per animation frame (`requestAnimationFrame`), not on every square the pointer crosses, and batches go to the server outside the drawing path (see [Architecture](architecture/maps/editor.md#the-map-editor)).
To measure it, write a Playwright test that creates the largest map the server accepts (200 x 400 squares), enters "Pintar", picks the wall and the 3 x 3 brush, and records the duration of each `requestAnimationFrame` while `page.mouse` drags across the map; a frame budget is 16.7 ms.
