# Maps module

A map is a gallery image with points of interest and tokens over it (MR-008, MR-009, MR-012).
The master prepares everything hidden and reveals it little by little; the server decides what each person sees (RN-10), and the player never receives what is hidden.
The code is in `backend/internal/maps`, the contract in `proto/meurpg/maps/v1` (`MapService` in `maps.proto`), and the screens in `web/src/app/pages/maps` and `web/src/app/shared/map-view`.

## The module's subjects

| Subject | Page |
| --- | --- |
| Uploads, the blob store, serving images, the gallery | [Images](images.md) |
| Generating images with Gemini (scene art, from a map) | [Image generation](image-generation.md) |
| Fog of war, the fogged map's image, per-player tiles | [Fog](fog.md) |
| Layers, painting, grid calibration, the map editor | [Editor](editor.md) |
| Point kinds, traps, treasure found, carried light | [Points](points.md) |
| Doors | [Doors](doors.md) |
| What reaches the stream | [Maps in the live session](live-session.md) |
| Generated dungeon maps | [Dungeons](dungeons.md) |
| The treasure generator | [Treasure](treasure.md) |

RP scenes (a scene point's actions, clues and opening in the session) are `maps` code too (`scenes.go`, `clues.go`, `sceneimages.go`): see RP scenes in [Architecture](../play/scenes-and-stage.md).

## `MapService`

| Call | Who may | What it does |
| --- | --- | --- |
| `ListMaps` | Members, filtered | The maps, oldest first, each with the image, whether revealed, whether it is the current map, how many points it has and which maps it is a sub-map of |
| `GetMap` | Members, filtered | A map with its points and tokens. A hidden map is `not_found` for the player. On a fogged map the player gets only what their character sees or remembers ([Fog](fog.md)); the master may send `as_character_id` ("Ver como") |
| `CreateMap` | Master | Creates the map from a campaign gallery image. Born hidden and without fog; if the table rule `fog_on_new_maps` (RN-24) says so, fog turns on with the first grid. `resource_exhausted` beyond 200 maps |
| `UpdateMap` | Master | Changes the name or the image, with the revision (`aborted` if stale) |
| `DeleteMap` | Master | Deletes the map with its points and tokens. Sub-map points that led to it lose their destination; if it was the current map, the session is left without one |
| `SetMapRevealed` | Master | Reveals or hides the map |
| `SetMapGrid` | Master | Sets the grid and its calibration ([Editor](editor.md#grid-calibration)) |
| `PaintMapCells`, `GetMapLayers` | Master; members, filtered | Paints and reads the layers ([Editor](editor.md#layers)) |
| `SetMapFog`, `GetMapVision`, `ForgetMapVision` | Master; members; master | Fog of war ([Fog](fog.md)) |
| `ListCharactersInTheDark` | Master | The player characters that see nothing where they stand ([Fog](fog.md#the-masters-light-warning)) |
| `RevealTrap`, `MarkTreasureFound`, `UnmarkTreasureFound`, `SetCarriedLight` | Master (the light also the owning player) | Trap and treasure state, the light a character carries ([Points](points.md)) |
| `CreateMapPoint`, `UpdateMapPoint`, `DeleteMapPoint`, `SetMapPointRevealed` | Master | Points of interest. `CreateMapPoint` puts a point at a position, born hidden (`resource_exhausted` beyond 200 per map). `UpdateMapPoint` changes only what the request carries: kind, name, description, the hooks (scene point only, up to 4,000 characters), `show_dc` (scene point only), position (moving is this), sub-map destination, revealed, and the data of the trap, treasure or light. Revealing a scene point discovers the scene (MR-030) |
| `AddSceneAction`, `UpdateSceneAction`, `MoveSceneAction`, `RemoveSceneAction` | Master | An RP scene's actions (MR-015), one change per call, each returning the point's list. At most 20 actions; a key must be a skill, ability or saving throw, a name up to 60 characters, a DC from 1 to 30 |
| `AddSceneClue`, `UpdateSceneClue`, `MoveSceneClue`, `RemoveSceneClue`, `RevealSceneClue` | Master | An RP scene's clues (MR-029). At most 30 clues of up to 500 characters; `RevealSceneClue` gives a clue to chosen player characters (1 to 50), once each, with no undo |
| `SetMapPlayersMove` | Master | Allows or forbids the players to move their own characters' tokens on the map outside combat (`maps.players_move_tokens`, off by default; every viewer reads it in `Map.players_move_tokens`) |
| `MoveMapTokens` | Master; a player for their own token, when allowed | Moves one or several living characters' tokens outside combat, with no movement limit; see [Tokens outside combat](#tokens-outside-combat) |
| `PutPartyOnMap` | Master | Puts several characters' tokens on the map in a formation (line, block or arc), each on its place or the nearest free square; see [Tokens outside combat](#tokens-outside-combat) |
| `PlaceMapToken`, `SetMapTokenHidden`, `RemoveMapToken` | Master | Puts a living campaign character's token or moves it, hides or shows it, removes it. A player's token is born visible, an NPC's hidden. With `creature_id` in place of `character_id` it is a character creature's group token (MR-037), never hidden |

Other `maps` services: `GalleryService` ([Images](images.md)), `ImageGenerationService` ([Image generation](image-generation.md)), `DungeonService` ([Dungeons](dungeons.md)) and `TreasureService` ([Treasure](treasure.md)).
A non-active member (and a pending member) gets `not_found` on everything, as in the rest of the app (`TestMapServiceAuthorizationMatrix`).

## Maps, points and tokens

- **The battle grid (MR-013, RN-21).** `maps.grid_columns` stores how many 1.5 m squares fit across the image's width; rows come from the image's proportion. Without a grid (`NULL`), combat does not start on the map (see Combat without a grid in [Architecture](../play/theatre.md)). The grid arithmetic and `play`'s grid read are in `maps.SessionMaps`.
- **Positions are in base points.** `x_bp` and `y_bp` go from 0 to 10000 across the image's width and height (5000 is the middle), independent of the image's size, so swapping the image or zooming moves nothing. The response carries the image's width and height so the screen draws the map before the image arrives.
- **Point kinds.** A point has a name (up to 80 characters) and a description for the players (up to 2,000, with line breaks). The kinds are battle, sub-map, RP scene, trap, treasure and light ([Points](points.md)).
  - A sub-map point leads to another map of the same campaign, never itself; the app first opens the point's sheet, with "Abrir <mapa>".
  - A battle point can also have a destination map, the combat's: starting the combat from the point (`CombatService.StartEncounter`) makes that map the session's current map and reveals it. It does not count as "sub-map of".
  - Changing a scene point to another kind deletes its actions.
- **Tokens.** One per character per map. The character must be living and in the campaign (a player's, neither dead nor pending, or an NPC); `characters` answers through the `maps.CharacterDirectory` interface (`characters.Service.MapCharacters`), which returns only ID, kind, name and player, nothing from the sheet or the master's notes. A character who dies keeps the row but no longer appears on the map.
- **One move at a time.** Moving (`PlaceMapToken`, `UpdateMapPoint`) carries no revision: the last write wins. Two moves of the same token sent together may reach the database out of order, so the screen sends one move of each point or token at a time (`web/src/app/core/maps/move-saves.ts`): while one is in flight only the last waits. If the server refuses, the item goes back to the last saved position. `shared/map-view` starts from the last position it reported until the new state arrives.
- **Limits.** 200 maps per campaign and 200 points per map (`DefaultMaxMaps`, `DefaultMaxPointsPerMap`), because the lists are not paged.
- **A map's image cannot be deleted** from the gallery: `DeleteGalleryImage` answers `failed_precondition` with the `ImageInUse` detail, which names the maps; the gallery shows on each image the maps that use it (`GalleryImage.used_in_maps`).

## Tokens outside combat

Outside combat the master moves tokens freely (plan B, B2); the code is `tokenmoves.go`, `tokenplan.go` and `putparty.go`.
While a combat runs on the map every one of these calls is refused with `MapBlocked COMBAT_RUNNING`: the tokens move through `CombatService`, whose squares are the combatants'.

- **`MoveMapTokens`** takes 1 to 40 characters (each with a token on the map, no repeats) and a destination point in base points for the first one, the anchor. A lone token walks the straight line toward the destination square, with no limit of movement, and stops before the first wall, locked door or square marked "can't pass" (`grid.Terrain.MoveUntil`, no occupants: tokens do not block each other). The answer has a `MapTokenMove` for each character with the token as it is now, the squares asked and moved, and `stopped_by` (`WALL`, `NO_PASS`, `LOCKED`, or `BLOCKED` for a player who does not know the square in the way). A token that reaches a new square lands on its centre; a token that stays in its square keeps its point. On a map with no grid there are no squares and nothing stops a token: it goes to the point.
- **A group** keeps its offsets from the anchor and moves only when every arrival square is on the map and standable (`planMove`); otherwise nobody moves, `moved` is false and `blocked_character_id` names the first one that did not fit. The line between is not walked for a group: what the master sees on the screen is where each one arrives.
- **Closed doors** a lone token walks through open, as in combat (`openClosedDoors`), and the layers' revision rises.
- **A player** may call it for their own character alone (several tokens, another character, an NPC or a character that does not exist all answer `permission_denied`, so the answer says nothing about what exists), when the map's `players_move_tokens` is on (`SetMapPlayersMove`), on a map they see and with a token they see. The answer names only that token.
- **The web screens.** The master's "Tokens" tab and the player's line under the map send `MoveMapTokens` (one move at a time, nothing drawn on a guess) and read `Map.players_move_tokens` from the map the stream keeps fresh (`map_changed`). A player's answer for `stopped_by` `BLOCKED` is written as "O movimento parou antes." with no reason; a `permission_denied` for a player who was moving is written as "O mestre voltou a mover os tokens.".
- **Traps and the stream.** After the commit each token that moved goes through the same `tokenLanded` as `PlaceMapToken` (a trap may fire, MR-035; what the character notices is part of the landing's own transaction) and the stream sends `token_moved` for each ([Maps in the live session](live-session.md)); opened doors send the usual layers notice.
- **`PutPartyOnMap`** takes 1 to 40 living characters, a formation and an anchor point. `grid.Arrange` chooses the squares: the formation's own (`LINE` to the right of the anchor, `BLOCK` in rows of two, `ARC` around the anchor), and for a place that is a wall, a "can't pass" square, or held by a token that is not part of the group, the nearest free standable square (rings of growing distance, ties in row-major order). `moved_for_room` says how many did not get their own place; fewer free squares than characters is `MapBlocked NO_ROOM`, and a map with no grid `NO_GRID`. Each token is written as `PlaceMapToken` writes it (a player's born visible, an NPC's hidden).
- **Creature tokens** (a familiar, a summoned animal) are still the master's through `PlaceMapToken`; the new calls take characters only.

## Who sees what on the map

The filter runs on the server, in `visibility.go`.
What the player does not see stays out of the response, never blank: neither the ID, the name nor the description.
`TestMR009_PlayersNeverReceiveHiddenPoints` reads the player's response as the JSON the app receives and searches it for the hidden point.

| What | The master sees | The player sees |
| --- | --- | --- |
| Map | All, with "revealed" and "current map" | The revealed one, and the session's current map even if hidden. Any other is `not_found`, like a map that does not exist |
| Point | All, with the state | Only the revealed ones, on a map they see, with three exceptions: light, trap and treasure |
| Trap | All, with DCs, trigger, effect and who knows it | Only if revealed to one of their characters, triggered, or revealed to all; then only name, description, position, area and state (armed, triggered or disarmed), never the DCs, trigger, effect or preset |
| Treasure | All, with the value and who found it | The revealed (name and position only) or the found (with description, value and finders, even if nobody revealed it) |
| Light (point) | All | Never: they see the light, not the point |
| Painted layers | All seven | Wall, terrain, cover, doors, the "can't pass" squares and the surface (water, climbable) of a map they see, never light; on a fogged map, only the squares the character sees or remembers |
| Why a move stopped | The real reason | Only for their own token (a player moves one alone), and the square in the way (a wall, a locked door, a "can't pass" square) only when they see or remember it; otherwise `BLOCKED` |
| The light a character carries | All | Their own character's; the others' arrive only through the fog (what it lights) |
| Token | All, with the state | Only the visible ones, on a map they see; their own is marked `mine`. On a fogged map: every player character's, always; an NPC's only on a square seen now. A character creature's group token is always received |
| Sub-map point's destination | Always | Only if they also see the destination map; otherwise the point has no destination |
| Which maps this is a sub-map of | All | Only those they see, through a revealed point |
| How many points the map has | All | Only those they see (plus the traps they know); on a fogged map, only those they receive |
| The fog of war | `fog_enabled`, `base_light`, `group_vision` | `fog_enabled` and `group_vision`; not the base light |
| The image of a fogged map | Always | Never: no ID, URL or thumbnail (`image_withheld`), only the size |
| The image's gallery name | Yes | No |
| The image file (`GET /images/{id}`) | Every campaign image | Only while they see a map with that image, while it is the session's shown image, or while the master leaves it with the players, and never a fogged map's ([Images](images.md#serving-images)) |
