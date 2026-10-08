# Gap hunt: maps, fog, images, dungeons, gallery, bestiary and treasure

Scope: MR-008, 009, 010, 018, 019, 028, 036, 039, 042, 044; RN-25, 26, 28, 30; the `maps` package and its protos; the web pages for them.
Code is the truth. Kinds: **doc** = doc out of date (fixed on `fix/t4-gap-docs-maps` unless noted), **web** = missing in the web, **server** = missing in the server, **diff** = behaviour differs.
No mismatch found that breaks a table session; the portcullis case (Gmaps-1) and the cap button (Gmaps-2) are the ones a table can notice.

| id | kind | where | what | evidence |
|---|---|---|---|---|
| Gmaps-1 | doc (fixed) | doc rules.md RN-26 + architecture "Doors" / server | Doc said every blocked move answers `locked_door`. Only a locked door does; a portcullis or secret door stops like a wall (`stopped_early`), so the player gets no "A porta está trancada." | `rules/grid/move.go:237,357` set `StopLocked` only for `DoorLocked` |
| Gmaps-2 | diff | doc stories (campaign cap, RN-30) / web | Doc said the cap sentence replaces the form. The form stays; the sentence is a danger notice after a refused submit; an account at the cap still sees an enabled "Criar campanha". Doc reworded (fixed). Whether to pre-check is a product call. | `web/.../campaigns.html:27-35`, `campaigns.ts:111-118`; server `campaigns/limits.go` |
| Gmaps-3 | doc (fixed) | rules.md RN-25, `maps.proto` SetMapGrid comment | "Calibration is refused while a combat is on the map" is only true when the rules grid changes. Rules.md fixed (EN+PT). The `.proto` comment (~line 226) still says it; not touched (needs `make proto`). | `mapservice.go:703` calls `refuseWhileCombat` only if `!sameGrid` |
| Gmaps-4 | web | MR-010 / `DungeonOptions` | Server options never sent by the web: `mask_hole` (the "Anel" always has a 40% hole), `aspect_limit`, `room_density`, `door_density` (so "the number of doors" is only the mix), `placement`, `extra_loops`. Belongs in "Gerar masmorra". | `web/.../core/maps/dungeon-options.ts` `optionsOf` |
| Gmaps-5 | web | MR-010 | Web is narrower than the server: size 21..121 (server 15..199 x 15..399), stairs 0..4 (server 0..8); other side is fixed at two thirds, so every web dungeon is landscape; web defaults (31, room max 9) differ from the server's (51, 11). The story states the page's limits; architecture "a typed size holds" is true only inside 21..121. | `dungeon-options.ts` `SIZE_MIN/MAX`, `STAIRS_MAX`; `rules/dungeon/options.go` |
| Gmaps-6 | diff | stories MR-010 / server | "uma porta precisa de chão dos dois lados" is a screen rule only; `PaintMapCells` accepts doors 0..5 anywhere. Doc now says so (fixed). | `door-paint.ts:20`; `layers.go` |
| Gmaps-7 | diff | stories MR-039 / web | Doc quoted "O serviço não gerou uma imagem"; the web shows "...esta imagem..." plus the slot sentence, ignoring `reason_pt`. Doc fixed. | `imagegen-errors.ts:131` |
| Gmaps-8 | doc (fixed) | architecture (image store step) | Said generated images are called "Imagem n"; names come from the request, "Imagem n" is only the fallback. | `imagegen.go:296,332,1246` |
| Gmaps-9 | doc (fixed) | stories MR-028 | Omitted that showing a fog map's background shows a "(névoa)" copy (new ID; `resource_exhausted` on a full gallery). | `onscreen.go:93-120`, `maps/fogimage.go:47` |
| Gmaps-10 | doc (fixed) | stories MR-044 | "individual or lair: coins, gems, art and items" - individual is coins only. | `srd51/effects/treasure.json`; `treasure.proto` |
| Gmaps-11 | doc (fixed) | stories MR-036 | Referenced `cave-data.md`, which does not exist; now points to `rules/vision/cave_expected_test.go`. `docs/design.md` and web comments cite a missing `MAP-LANGUAGE-E10.md` (design.md fixed; code comments left). | `git ls-files` |
| Gmaps-12 | doc | stories MR-036, architecture "The fog" | "Fog off by default" omits: dungeon maps are born with fog on; the table rule "névoa nos mapas novos" turns it on at the first grid. Not edited (wording choice). | `dungeons.go` `BaseLight: bright`; `ApplyFogRule`; migration 00091 |
| Gmaps-13 | web | `grid-panel.html:29` | "Mudar a grade?" warning omits that doors are erased too (calibrate-ask lists them). | `clearLayers` deletes the doors layer |
| Gmaps-14 | doc | MR-044 / stories 1107,1133 | Content revisions `fx.12`/`fx.16` are stale (current 20); harmless if read as "introduced in". Not edited. | `effects/revision.json` |
| Gmaps-15 | doc | code comment `maps/fog.go:116` | Says scene is ~1.6 MB / 80 kB per view; CONTRIBUTING/architecture say ~430 kB / 85 kB. Code comment, left alone. | `TestSceneMemory` |
| Gmaps-16 | doc | rules.md, architecture | Door state 4 is "portcullis" in docs, `BARRED` in proto/Go, "Grade" in the UI; values agree. Not edited. | `DOOR_STATE_BARRED` |
| Gmaps-17 | web | PlaceTreasure errors | `treasure-errors.ts` maps every `invalid_argument` to "fora da grade"; the server also uses it for long names, missing content version, reused key. Unlikely from the web form. | `treasure.go:93-97` |
| Gmaps-18 | web | EditGeneratedImage `name` | Never sent; adjusted images always get "<name> (ajuste)". | `client.edit()` |
| Gmaps-19 | web | unused server fields (no doc promises them) | `Map.image_withheld`, `GetMapLayersResponse.fog_withheld`, `generator_version`, `DungeonRoom.center_col/row`, `DungeonStair.facing`, `ImageGeneration` metadata (prompt, style, ids, created/finished), `ImageGenerationStatus.used_this_month`, `padded_ratio`, `max_texture_pixels` (16 MP text is hard-coded), `ShownImage.thumbnail_url`, `SetShownImageResponse.keep`, multiattack routines, `Treasure.category`, `variant_of`. | field scan of `web/src/app` |
| Gmaps-20 | server | none | No screen expects something the server lacks; every RPC of the area has a web caller (`RemoveMapToken` included). | RPC scan |

## Checked and consistent
- All Go test names and e2e spec files cited by these stories and rules exist.
- Every Portuguese UI string quoted in the stories exists in `web/src/app`.
- Limits: gallery 300 images / 500 MiB / 10 MB / 40 MP; tiles 16x16, 2048 px working copy, 429/503 behaviour; light radii 0..120 ft; monthly 20, daily 100, campaigns 10 (ranges, env vars, error codes); rate-limit table; treasure bands and values; bestiary 334 creatures, paging, 1-10 copies, 40-combatant ceiling; familiar 30 m.
- Grid calibration limits (200x400, factor 20), door layer encoding (4 bits, 0..5, 400 squares per paint), dungeon create/redraw/preview limits and refusals, RN-10 visibility of doors, light, base light and image.
