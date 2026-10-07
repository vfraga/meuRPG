# Review unit 14: maps on the web (editor, points, doors, dungeons, gallery, AI images)

- Scope: `web/src/app/pages/maps/`, `pages/gallery/`, and the shared map, point-sheet, dungeon-preview, image-generate and gallery-picker pieces.
- Commit reviewed: `ae40f74` (main of the fork).
- Time spent: about 1.5 hours (reading plus six parallel verification runs).
- Model: claude-sonnet-5-5 (Sonnet 5.5). Verification subagents: `sonnet`.
- Run all proofs: `cd web && npx ng test --watch=false --include 'src/app/**/review14-*.spec.ts'` (13 tests fail today, all by the defects below).

| id | severity | status | where | defect | failure scenario | evidence |
| --- | --- | --- | --- | --- | --- | --- |
| U14-1 | medium | confirmed | `core/maps/paint-queue.ts` `clear()` / `drain()` (called from `paint-session.ts` `open()`) | `clear()` empties `batches` while `drain()` is awaiting the head batch; `drain()` then `shift()`s or overwrites index 0, which is now a new batch. | Master has a paint call in flight; the map's grid changes under the editor (second tab, or his own grid change) so the session re-opens and clears; he paints one more stroke before the old call returns. The old call resolves: the new stroke is dropped unsent and the tag says "Tudo salvo"; on a transient error the old batch is re-sent over the new one (to the old grid). | `review14-u14-01.spec.ts`: `expected [ 'a' ] to deeply equal [ 'a', 'b' ]`; `expected false to be true` (saved too early); `expected [ 'a', 'a' ] to include 'b'`; session: `expected 1 to be 2` on `paints.length`. |
| U14-5 | medium | confirmed | `pages/maps/map-editor/editor-painting.ts` `doorStroke`/`applyDoors`, `core/maps/door-paint.ts` `planDoor` | On a calibrated map (`squareFactor` > 1) the server paints and erases a door over the whole factor x factor block (`backend/internal/maps/layers.go` ~193), but the editor plans, paints and sends one rules' square. | Map at 3 m per square, Porta tool on a wall: the screen shows one small door and a one-square hole while the server holds a 2 x 2 (or larger) block of doors; no refusal, so no re-read; "Tirar a porta" has the same gap. `planDoor`/cursor judge one square, so on a thick wall they say "Aqui não dá". | `review14-u14-05.spec.ts`: `expected [ '4,2' ] to deeply equal [ '4,2', '4,3', '5,2', '5,3' ]`; erase: `expected [ '5,3' ] to deeply equal [ '4,2', '4,3', '5,2', '5,3' ]`. |
| U14-6 | medium | confirmed | `shared/image-generate/generate-image-button.ts:97` `open()` | No "opening" guard around `await import(...)` of the lazy dialog chunk. | First use in a session, slow network: a second tap before the chunk arrives opens a second "Gerar imagem" dialog; each has its own key and run, so two slots of the monthly cap (RN-28) can be spent from one button, and the first dialog's result hides under the second. Hosts: map-head, gallery, map-manage, scene-image. | `review14-u14-06.spec.ts`: `expected 2 to be 1`. |
| U14-3 | medium | confirmed | `pages/gallery/gallery.ts` `load()`/`refresh()` and `UploadQueue` use (`core/images/upload-queue.ts:135`) | No stale-answer guard when `:id` changes on the reused component; the upload queue reads `campaignId()` at send time and is not cleared on navigation. | Master goes from campaign A's gallery to B's (back/forward, link): A's slow list overwrites B's (rename/delete then calls B with A's image id); files queued on A are uploaded into B. | `review14-u14-03.spec.ts`: `expected [ 'Imagem de A' ] to deeply equal [ 'Imagem de B' ]`; `expected [ 'camp-A', 'camp-B' ] to not include 'camp-B'`. |
| U14-4 | low | confirmed | `pages/maps/map-editor/map-editor.ts:617` `save()` (also `setPoint` after a treasure/trap call) | `save()` does `upsertPoint(saved)` with an answer computed before an in-flight move committed, overwriting the optimistic position; a successful move never corrects the screen. | Drag a point, press "Salvar ponto" for a name change before the move returns: the point snaps back on screen while the server holds the new place; later saves build on the stale position. | `review14-u14-04.spec.ts`: `expected [ 4800, 5000 ] to deeply equal [ 1000, 2000 ]`. |
| U14-2 | low | confirmed | `pages/maps/player-map/player-map.html`; `shared/map-pins/map-pins.ts` | RN-10 second lock covers only `<app-map-view>`: pins, "Pontos deste mapa" list, point sheet and legends use raw `state().points()`; `MapPins` drops only LIGHT for non-masters. | If a hidden armed trap, unrevealed treasure or light point ever reaches the client (server bug, stale `MapState` after an un-reveal), the player gets its pin, name and description. The server (`visibility.go` `seesPoint`) never sends one today, so no normal sequence triggers it. | `review14-u14-02.spec.ts`: pins `to deeply equal [ 'p-trap-fired', 'p-trap-mine' ]`; list `to not include 'Armadilha secreta'`; sheet `to be null`. |
| U14-7 | low | unverified | `pages/maps/dungeon-new/dungeon-new.ts` `load` (~215), `pages/maps/map-print/map-print.ts` `load` (~151) | Same missing stale-answer guard as U14-3 (route param change, late answer sets name/map/phase). | Read from the code only (confirmed by the U14-3 agent, no test). Would be settled by a spec like U14-3 with two deferred `getCampaign` promises. | none |
| U14-8 | low | unverified | `pages/maps/map-grid/map-grid.ts:152` `save()` | Sends the factor read when the page opened; another tab that recalibrated meanwhile is reverted (and its layers cleared). | Needs two tabs; not tested. A spec with a fake `setGrid` recording the factor after `map` changes would settle it. | none |
| U14-9 | low | unverified | `pages/maps/map-manage/map-manage.ts` layers effect | Refetches layers on every `map` change with no generation guard (out-of-order answers). | Phone master view only (no painting); not tested. | none |
| U14-10 | n/a | refuted | `dungeon-new.ts` `create()`/`cancelCreate()`; `ActionKey`; `ImageGenerateDialog` keys; `ImageRun` | Checked for double submit and key-per-click: keys are per intent (`ActionKey`, `pendingKey`), BigInt seeds are handled, stage guards are synchronous, cancel/destroy paths are correct. | No failing sequence found. | code reading |

Not examined in depth: print math (it uses the server's rules' grid, which matches the doc), the web `gridRows` (same rounding as `RowsFor`, but no 400-row cap: display only). The map page does not subscribe to the live stream, so two tabs only meet through the "Aborted" message, which is by design.

## Fix directions

### U14-1
- Root cause: `PaintQueue.clear()` drops batches without telling the running `drain()`, which holds index-based assumptions about `batches[0]`.
- Where: `core/maps/paint-queue.ts`. Best: let `drain()` hold its batch by reference and remove it with `indexOf` (`splice` only if still present), and on transient error restore only if that batch is still `batches[0]`. Alternative: a `generation` counter bumped in `clear()` and checked after each `await send`. The reference approach is smaller and also fixes the transient overwrite.
- Same shape elsewhere: `PaintSession.open` skip path (`paint-session.ts` ~60-75) relies on `busy`; nothing else shifts the queue.
- Rules: `refused`/`syncAfterRefusal` semantics and `whenIdle` callbacks must stay; status must reach `saved` only when nothing waits.
- Risk: `paint-queue.spec.ts` and `paint-session` specs guard ordering and the 400-square chunks.

### U14-5
- Root cause: the client has no notion of the calibration block that the server applies to the DOORS layer.
- Where: `EditorPainting.doorStroke`/`applyDoors` and `planDoor` (`core/maps/door-paint.ts`): work in drawing squares (`squareFactor`): expand each tapped square to its block for the local paint and for the queue, and judge floor/wall at block level. Alternative: after a door batch, re-read the layers (simple, flickers, costs 60 KB); pick the expansion.
- Same shape: the brush cursor (`doorCursor`), `door-picks`, and the session door sheet (`pages/live-session/door-sheet`) if it paints one square.
- Rules: server stays the authority (docs "Doors on the web": no rule in the browser); update the "Doors on the web" and "Map editor" sections.
- Risk: door counts in the legend; guarded by `door-paint.spec.ts`, `map-editor.spec.ts`.

### U14-6
- Root cause: `open()` has no re-entrancy guard across the async import.
- Where: `generate-image-button.ts`: an `opening` flag set before the `await` and cleared when the dialog closes (or the sheet's `afterClosed`).
- Same shape: `gallery.ts` `onLightboxAdjust` (closes the lightbox first, so safe), `map-page.changeImage` (sync open, safe).
- Rules: keep the lazy chunk. Risk: `generate-image-button.spec.ts`.

### U14-3 (and U14-7)
- Root cause: route-param pages write async answers without checking they are still for the current id.
- Where: `GalleryPage.load/refresh` (compare `campaignId()` or a generation counter after the await); on a param change call `queue.clear/cancel` (`UploadQueue`) and reset `images`. Same in `dungeon-new.load`, `map-print.load`, and `GalleryPicker.load` (campaign input change).
- Rules: keep the not-found/forbidden states. Risk: `gallery.spec.ts`.

### U14-4
- Root cause: server answers are applied wholesale over a point with a pending optimistic move.
- Where: `MapEditor.save()`/`setPoint`: merge the answer but keep the screen's position while `MoveSaves` has an in-flight entry for that point (expose `pending(key)`), or take the position from the last move. Same in `treasure`/`trap` `pointChange` and `setPoint`.
- Risk: `move-saves.spec.ts`, `map-editor.spec.ts`.

### U14-2
- Root cause: the second lock lives inside `MapView` only.
- Where: `player-map.ts`: a `computed` `visible = visiblePoints(state().points(), false)` fed to pins, legends, the list and `selected`; also give `MapPins` the `pointHidden` check for non-masters. Rules: RN-10. Risk: `player-map.spec.ts`, `map-pins` specs (a fired or revealed trap must stay drawn).
