# Review unit 5: maps, fog of war, per-player images and tiles, uploads, gallery

- **Scope:** `backend/internal/maps` (`fog.go`, `fogreads.go`, `fogwrites.go`, `fogimage.go`, `fogcombat.go`, `tiles.go`, `visibility.go`, `carriedlight.go`, `creaturetokens.go`, `sessionmaps.go`, `serve.go`, `upload.go`, `gallery.go`, `httperror.go`, `ratelimit.go`) and `maps/images`.
- **Commit reviewed:** `ae40f74` (`main`). Baseline: `go test -race ./internal/maps/...` passes before any change.
- **Time spent:** about 3 hours of reading and probing, plus about 6 minutes of subagent verification per candidate.
- **Model:** main review ran on Sonnet 5.5 (`claude-sonnet-5-5`); the verification subagents also ran on `sonnet` (Sonnet 5.5).
- **Method:** each confirmed row was reproduced by a fresh subagent that saw only the candidate text. Failing tests are in `backend/internal/maps/review05_*_test.go`, named `TestReview5_*`. No production code was changed.
- **RN-10 verdict:** no way found for a player to get pixels of an unseen area (tiles, full image, thumbnail, `ETag`/304, `GetMap` metadata). The five findings below are availability, memory and data-hygiene defects, not leaks.

## Findings

| Id | Severity | Status | Where | Defect | Failure scenario | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| U5-1 | medium | confirmed | `upload.go:106-110`, `upload.go:177` | `upload()` reads the whole file (up to 10 MiB, `io.ReadAll` growth) into memory before `process()` waits for the one-at-a-time slot. | A master sends up to 20 parallel uploads (the per-user burst) while one image is decoding: all 20 bodies (about 12-20 MiB each) sit in memory on top of the slot's own ~200 MB. That passes the 400 MiB `GOMEMLIMIT` and the 512 MiB instance, contradicting the docs' "one image at a time" budget. Any account can create a campaign and so is a master of its own. | `TestReview5_QueuedUploadsHoldBodies`: `with the processing slot busy, the server read 56625480 of 56625480 bytes of 6 queued uploads`; 3/3 runs. `go test -run TestReview5_QueuedUploadsHoldBodies -count=3 -v ./internal/maps/` |
| U5-5 | medium | confirmed (impact on Cloud Run not shown) | `serve.go:177`, `tiles.go:897` (`http.ServeContent`), `platform/httpserver/server.go` | Image, thumbnail and tile responses set no write deadline; the server has no `WriteTimeout` and Cloud Run allows 35 minutes. | A member (a player is enough) requests a large visible image and stops reading: the handler goroutine, the blob file and a request slot stay held until the platform timeout. The per-user download burst is 500; `docs/operations.md` defines no Cloud Run concurrency (default 80), and the live streams share those slots. | `TestReview5_ImageAndTileWriteDeadline`: `GET /images/<id> set no (non-zero) write deadline (0 SetWriteDeadline calls)`, same for `/thumb` and the tile; `TestReview5_StalledReaderKeepsHandlerBlocked`: handler still blocked after 4 s. 3/3 runs. Cloud Run's front end may buffer; not tested. |
| U5-2 | low | confirmed | `fog.go:379-384` | The fog scene single flight (`once.Do(compile)`) reads the layers with the first caller's context; if that request is cancelled mid-compile, every waiter on the same key gets that error. | After a `vision_changed` all players re-read the map at once; one closes the tab during the compile and the others get `canceled` (HTTP 499 on HTTP routes) although their own contexts are live. The entry is then dropped, so the next request recovers. | `TestReview5_LitSingleFlightCancel`: `a waiter with a live context got read the layers: context canceled; want a valid scene`; 3/3 runs. |
| U5-3 | low | confirmed | `tiles.go:737-752` (`blackenBlocks`), `tiles.go:669` (`renderTile`), `tiles.go:203` (`buildTiles`) | A tile that covers zero pixels is still listed in the index. JPEG: `xlo[0]` on an empty slice panics. PNG: `png.Encode` of 0 pixels fails and becomes a logged ERROR plus `503`. | Master uploads a 10x10 px image (accepted: no minimum size), `SetMapGrid` 200 columns (accepted), fog on. The player's `GetMapVision` lists tile (0,0); fetching it drops the connection (JPEG) or returns 503 (PNG), forever. Fails whenever `16*width < columns` (or the same for height); brute force found no failure for images of 100 px or more per side. | `TestReview5_TinyImageTiles` (subtests `jpeg`, `png`): `request failed (server panic?) ... EOF` and `status 503`; 3/3 runs. |
| U5-4 | low | confirmed | `sessionmaps.go:113` (`ImageToShow`), `fogimage.go:141` (`ownImage`), `play/onscreen.go:110` | The copy of a fog map's image is committed in its own transaction before the show transaction runs, and a new copy is made on every call. | With no open session `SetShownImage` fails with `failed_precondition` but leaves one gallery row and two files; showing the same fog image twice adds one more each time. Each copy counts against the 300 image / 500 MB quota and the files stay on disk. Leave and NPC portrait use the same `ownImage` (not exercised). RN-10 is not broken. | `TestReview5_ShowFogImageCopyLeaks`: `(a) images 1 -> 2, files 2 -> 4` and `(b) images 3 -> 4, files 6 -> 8`; 3/3 runs. |
| U5-6 | n/a | refuted | `images/images.go`, x/image `webp` | A WebP whose container size differs from the inner bitstream size would bypass the decode-cost check. | Small `VP8X` header with a huge `VP8L` chunk. | `golang.org/x/image@v0.46.0/webp/decode.go:86,117-142` rejects the mismatch. |
| U5-7 | n/a | refuted | `images/images.go` (`thumbnail`, palette PNG) | A 40 MP palette PNG makes the upload or the first tile burn CPU for a long time. | Measured: `Process` 3.1 s (palette) and 1.0 s (gray) for 7900x5000; `decodeWorkingCopy` 0.16 s. Bounded by the slot and the 20/6 s upload limit. | local throwaway benchmark (not committed). |
| U5-8 | n/a | refuted | `tiles.go` (`decodeWorkingCopy`, `renderTile`) | Per-square scaling might read neighbour pixels for gray, palette or alpha PNG sources, which the existing oracle does not cover. | Same oracle as `TestRN10_TileOracle` with `image.Gray`, `image.Paletted`, NRGBA with full and partial alpha, at 4096x2048/64, 3000x2000/37 and 240x160/24 columns: 0 differing tiles in all. | throwaway test (not committed). |
| U5-9 | n/a | refuted | `serve.go`, `tiles.go`, `gallery.go`, `upload.go` | IDOR, other-campaign access, tile URL variants, `as=` misuse, fog image via `Range`/`If-None-Match`/thumb, odd campaign ids, error leaks. | All answered 401/403/404/400 as documented; `ETag` 304 only after the visibility checks; errors carry fixed text (`rpcerr.FromDB`). | throwaway probe (not committed). |
| U5-10 | unknown | unverified | `images/images.go` | Native fuzzing of `Process`, `readJPEGHeader`, `exifOrientation` for panics, hangs, and measured peak heap against `decodeCost` for 16-bit, interlaced, tRNS, progressive 4:4:4 JPEG. | A background agent was started for this; its report had not arrived when this file was written. | Would be settled by `go test -fuzz FuzzProcess -fuzztime 150s ./internal/maps/images/` and a peak-heap table per format. |

## Fix direction

### U5-1: queued upload bodies
- **Root cause:** the one-at-a-time slot guards decoding only; reading the body into a `[]byte` is unbounded by it.
- **Where:** `maps.Service.upload` / `process`. Option A (pick this): take `s.processing` (with `ctx`) right after authorization and the quota look, before `readFile`, and hold it through `process`; the body is then read, decoded and stored under one slot, so at most one body exists at a time. Option B: keep the slot as is and add a separate small semaphore (1 or 2) for "bodies being read"; more concurrency, but the budget must be re-measured.
- **Same pattern elsewhere:** `fogimage.go:117` `copyBlob` (`io.ReadAll` of up to 10 MiB, outside the slot, per fog on/swap/show); `tiles.go:653` `loadWorkingCopy` is already inside the slot; `imagegen*.go` and `dungeons.go:679` (`drawDungeon`) should be checked for reads before the slot.
- **Rules to respect:** "files before the row" order and cleanup on failure (`storeImage`); the slot wait must honour `ctx`; a slot held while reading a slow client is bounded by `uploadReadTimeout` (2 min), so a trickling client now blocks other uploads for up to 2 min, which Option A must weigh (use Option B if that matters).
- **Docs:** `CONTRIBUTING.md` memory table, `docs/architecture.md` "Uploading".
- **Risk:** upload latency under contention; guarded by `upload_slow_test.go`, `gallery_test.go`, `TestReview5_QueuedUploadsHoldBodies`.

### U5-5: no write deadline on downloads
- **Root cause:** no per-operation write bound on the plain HTTP routes; only stream sends and upload reads got one.
- **Where:** `serve` and `serveTile`: before `http.ServeContent`, call `http.NewResponseController(w).SetWriteDeadline(now + d)` with `d` sized for 10 MiB on a slow phone (about 2 min, like `uploadReadTimeout`), or wrap the writer with a progress-based deadline (reset on each write) for fairness; pick the first, it is simplest. Put it in `platform/slowclient` as `WriteBody(w, d)` next to `ReadBody`.
- **Same pattern elsewhere:** every non-stream handler that writes a large body (check `cmd/api` and other modules' HTTP routes; `handleUpload`'s answer is tiny).
- **Rules:** do not add a global `WriteTimeout` (it would cut the Connect streams); set a Cloud Run `--concurrency` in `docs/operations.md` (and cap by the 8 streams per user).
- **Docs:** `docs/operations.md` (Cloud Run table, "Slow clients" in `docs/architecture.md`).
- **Risk:** a legitimately slow client loses a big image; `TestReview5_ImageAndTileWriteDeadline`, `TestRN10_TileCaching`.

### U5-2: scene single flight
- **Root cause:** a shared once-per-key computation uses one caller's `ctx` and error.
- **Where:** `newSight`: compile with `context.WithoutCancel(ctx)` (the scene does not depend on who asks; bound it with its own timeout), or, when `entry.err` is a context error and the waiter's own `ctx` is alive, drop the entry and retry once. Pick `WithoutCancel` plus a timeout.
- **Same pattern elsewhere:** `memo.once.Do` in `partyOf` (`fog.go:239`) shares the first caller's context across `ListMaps` maps the same way (same request, so lower risk); the tile `working` copy is under the gate, not a single flight.
- **Rules:** inside a transaction the scene is compiled on the transaction and kept out of the cache (`tx != nil` branch); do not change that.
- **Risk:** a compile no one waits for still runs to the end; bounded by the timeout. Guarded by `TestMR036_*` and `TestSceneMemory`.

### U5-3: zero-pixel tiles
- **Root cause:** the grid may be finer than the image's pixels, and the tile code assumes at least one pixel per tile.
- **Where:** two fixes together. (1) `buildTiles`: skip a tile whose pixel rectangle is empty for the working copy, so the index never lists it. (2) `renderTile`: return a typed error for `w == 0 || h == 0` (never panic), mapped to `404`. A third option is refusing the grid in `SetMapGrid` when `image px / columns < 1` (or a small minimum such as 4 px); pick (1)+(2) because it also protects stored data, and add the grid check as a UX guard.
- **Same pattern elsewhere:** `blackenBlocks` `xlo[0]`, `xhi[w-1]`, `ylo[0]`, `yhi[h-1]`; `renderTile` `colSq`/`rowSq` sizing; `tileSource.ring` (`max(1, ...)` already guards division).
- **Rules:** unknown pixels stay opaque black (RN-10); error text must not carry sizes or paths; document in "Per-player image tiles".
- **Risk:** the app would show an unlisted tile as black; `TestRN10_TileOracle`, `TestRN10_TheIndexListsOnlyTilesWithAKnownSquare`.

### U5-4: orphan and duplicate fog copies
- **Root cause:** the copy is made and committed by `ownImage` on its own, before the caller's transaction, and is never looked up again.
- **Where:** make `ImageToShow` return a `PortraitCopy`-like handle (as `PreparePortrait` already does): files copied before, row inserted inside the show transaction (`Insert(ctx, tx)`), files removed on failure (`Discard`). To avoid duplicates, reuse an existing copy of the same source (for example by a `copy_of_image_id` column or the "(névoa)" name plus identical hash) when the master shows the same image again.
- **Same pattern elsewhere:** `PortraitImage` (non-tx variant of the same `ownImage`), `leave` paths that call `ImageToShow`; `imagegen_map.go:755-834` and `mapservice.go:268-315,376-453,652-744` already do the insert-inside-the-transaction pattern correctly and can be the model.
- **Rules:** idempotent `InTx` closures (the insert may run again on 40001), quota check inside the transaction, no effect outside the database before commit except the files, which must be cleaned.
- **Docs:** "The image of a fogged map" in `docs/architecture.md`.
- **Risk:** the `play` module's interface (`MapKeeper.ImageToShow`) changes; guarded by `TestMR028_*`, `TestRN10_FogMapImageIsCopiedWhenShared`, `TestRN10_AFogMapsImageIsNeverReusedRaw`.

## Unverified

- **U5-10:** fuzzing and measured-peak-vs-estimate of the image decoders; see the row above for the command that settles it.
- **Not tested at all:** the Cloud Run front end's behaviour with stalled readers (U5-5); `BLOB_DIR` on Cloud Run (no Cloud Storage implementation exists yet, a known open item in `docs/operations.md`, and the disk there is memory-backed).
