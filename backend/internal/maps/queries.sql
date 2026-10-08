-- name: GetGalleryUsage :one
-- How much of the quota the campaign uses: its images and their bytes.
-- Upload reads it inside the transaction that inserts the new row, so two
-- uploads racing cannot both squeeze under the limit: CockroachDB's
-- SERIALIZABLE isolation makes one of them retry and count again.
SELECT
    count(*)::INT4 AS image_count,
    COALESCE(sum(byte_size), 0)::INT8 AS byte_count
FROM gallery_images
WHERE campaign_id = $1;

-- name: InsertGalleryImage :one
INSERT INTO gallery_images (id, campaign_id, uploaded_by, name, content_type, width, height, byte_size, created_at, generated, parent_image_id, generated_kind, copy_of_image_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: FindImageCopy :one
-- The copy already made of a fog map's image to show it (or to be a portrait), unless
-- it has become the background of a map with the fog on, which no player may receive
-- (RN-10). The newest one when there are several.
SELECT * FROM gallery_images g
WHERE g.campaign_id = $1 AND g.copy_of_image_id = $2
  AND NOT EXISTS (
      SELECT 1 FROM maps m
      WHERE m.campaign_id = g.campaign_id AND m.image_id = g.id AND m.fog_enabled
  )
ORDER BY g.created_at DESC, g.id DESC
LIMIT 1;

-- name: ListGalleryImages :many
-- Newest first; id breaks ties, so the order never changes between calls.
SELECT * FROM gallery_images
WHERE campaign_id = $1
ORDER BY created_at DESC, id DESC;

-- name: GetGalleryImage :one
-- An image by its ID alone, to serve it: the caller's membership in its
-- campaign is checked right after.
SELECT * FROM gallery_images
WHERE id = $1;

-- name: RenameGalleryImage :one
UPDATE gallery_images
SET name = $3
WHERE campaign_id = $1 AND id = $2
RETURNING *;

-- name: DeleteGalleryImage :one
-- The row goes first; the caller deletes the files after the commit.
DELETE FROM gallery_images
WHERE campaign_id = $1 AND id = $2
RETURNING *;

-- name: ListMapsUsingImage :many
-- The maps whose image this is, oldest first: DeleteGalleryImage names them
-- (MR-019), and RenameGalleryImage returns them with the image.
SELECT id, name FROM maps
WHERE campaign_id = $1 AND image_id = $2
ORDER BY created_at, id;

-- name: ListMapImageIDs :many
-- Every map of the campaign with its image, oldest first: the gallery
-- shows, on each image, the maps that use it.
SELECT id, name, image_id FROM maps
WHERE campaign_id = $1
ORDER BY created_at, id;

-- Maps (MR-008, MR-009, MR-012). Every query names the campaign next to the
-- map, or runs after a query that did: a map of another campaign matches no
-- row, which the handlers answer as "not found".

-- name: GetGalleryImageInCampaign :one
-- The image a map is made of must be one of the campaign's.
SELECT * FROM gallery_images
WHERE campaign_id = $1 AND id = $2;

-- name: CountMaps :one
-- The campaign's maps, for the limit. Read inside the transaction that
-- inserts a map, as GetGalleryUsage.
SELECT count(*)::INT4 AS map_count FROM maps
WHERE campaign_id = $1;

-- name: InsertMap :one
-- A new map starts hidden (revealed_at NULL), with no fog: when the table's
-- rules want it (RN-24) it comes on with the first grid (fog_on_first_grid).
-- create_key and create_hash are the idempotency key of CreateMap and CreateDungeonMap and the
-- hash of the request (NULL when the call sent no key). Two calls with the same key at once make
-- one map: the loser gets no row, and reads the winner's (GetMapByCreateKey).
INSERT INTO maps (campaign_id, name, image_id, fog_on_first_grid, create_key, create_hash, created_at, updated_at)
VALUES (sqlc.arg(campaign_id), sqlc.arg(name), sqlc.arg(image_id), sqlc.arg(fog_on_first_grid), sqlc.narg(create_key), sqlc.narg(create_hash), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetMapByCreateKey :one
-- The map a CreateMap or CreateDungeonMap with this idempotency key made, if any (the key
-- carries the campaign's ID, so it is unique in the campaign).
SELECT * FROM maps WHERE create_key = $1;

-- name: ListMapDetails :many
-- The campaign's maps, oldest first, with what the lists show about each:
-- its image's name and size, and how many points it has, in all and
-- revealed. Campaigns have a few maps, so one query answers ListMaps and
-- gives GetMap the names and states it needs (parents, submap targets).
--
-- revealed_point_count is how many points every player sees: not a light, which
-- no player ever receives, and either revealed, a triggered trap or a found
-- treasure (a trap revealed to some characters only is the handler's to add).
SELECT m.id, m.campaign_id, m.name, m.image_id, m.revealed_at, m.revision, m.created_at, m.updated_at, m.grid_columns, m.grid_factor,
       m.fog_enabled, m.base_light, m.group_vision, m.layers_revision, m.light_revision, m.vision_epoch,
       g.name AS image_name, g.width AS image_width, g.height AS image_height, g.content_type AS image_content_type,
       EXISTS (SELECT 1 FROM generated_dungeons AS gd WHERE gd.map_id = m.id) AS generated_dungeon,
       (SELECT count(*) FROM map_points AS p WHERE p.map_id = m.id)::INT4 AS point_count,
       (SELECT count(*) FROM map_points AS p
        WHERE p.map_id = m.id AND p.kind <> 'light'
          AND (p.revealed_at IS NOT NULL OR p.trap_triggered_at IS NOT NULL OR p.treasure_found_at IS NOT NULL))::INT4 AS revealed_point_count
FROM maps AS m
JOIN gallery_images AS g ON g.id = m.image_id
WHERE m.campaign_id = $1
ORDER BY m.created_at, m.id;

-- name: ListSubmapLinks :many
-- Every submap point of the campaign that leads to a map: the map it is on,
-- the map it leads to, and whether the point is revealed. It gives each map
-- its parents ("Submapa de ...").
SELECT p.map_id, p.target_map_id::UUID AS target_map_id, (p.revealed_at IS NOT NULL)::BOOL AS revealed, p.x_bp, p.y_bp
FROM map_points AS p
JOIN maps AS m ON m.id = p.map_id
WHERE m.campaign_id = $1 AND p.kind = 'submap' AND p.target_map_id IS NOT NULL
ORDER BY p.created_at, p.id;

-- name: GetMapForUpdate :one
-- FOR UPDATE locks the map's row until the transaction ends, so two edits of
-- the same map wait for each other instead of both reading the same
-- revision.
SELECT * FROM maps
WHERE campaign_id = $1 AND id = $2
FOR UPDATE;

-- name: GetMap :one
SELECT * FROM maps
WHERE campaign_id = $1 AND id = $2;

-- name: UpdateMap :one
-- revision in the WHERE clause is a second guard: the handler already
-- compared it under FOR UPDATE, so no row here means a stale revision.
UPDATE maps
SET name = sqlc.arg(name), image_id = sqlc.arg(image_id), revision = revision + 1, updated_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id) AND revision = sqlc.arg(revision)
RETURNING *;

-- name: SetMapRevealed :one
-- Revealing keeps the first revealed_at, so revealing twice changes
-- nothing; hiding clears it. PlayService.SetCurrentMap reveals through here
-- too (SessionMaps).
UPDATE maps
SET revealed_at = CASE WHEN sqlc.arg(revealed)::BOOL THEN COALESCE(revealed_at, sqlc.arg(now)::TIMESTAMPTZ) ELSE NULL END,
    updated_at = CASE WHEN (revealed_at IS NOT NULL) = sqlc.arg(revealed)::BOOL THEN updated_at ELSE sqlc.arg(now)::TIMESTAMPTZ END
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: SetMapGrid :one
-- The master's grid (MR-013): NULL clears it, and a map without a grid has no
-- fog of war. grid_columns is the engine's columns (the drawn ones times the
-- factor, MR-025). It is a change to the map itself, so updated_at moves, but the
-- revision (the name and the image's guard) does not.
UPDATE maps
SET grid_columns = sqlc.narg(grid_columns), grid_factor = sqlc.arg(grid_factor), fog_enabled = fog_enabled AND sqlc.narg(grid_columns)::INT4 IS NOT NULL,
    updated_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: ApplyFogRule :one
-- The table's rule "névoa nos mapas novos" met the map's first grid (RN-24): the
-- fog comes on, and the rule is spent.
UPDATE maps SET fog_enabled = true, fog_on_first_grid = false, updated_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: SetMapFog :one
-- The fog of war's settings (MR-036): each one the handler sends replaces the
-- current value, the others stay. updated_at moves only when something a
-- player reads changes (the switch or "Visão do grupo"): the base light is the
-- master's, and a player must not learn that it changed.
UPDATE maps
SET fog_enabled = COALESCE(sqlc.narg(fog_enabled)::BOOL, fog_enabled),
    fog_on_first_grid = fog_on_first_grid AND sqlc.narg(fog_enabled)::BOOL IS NULL,
    base_light = COALESCE(sqlc.narg(base_light)::TEXT, base_light),
    group_vision = COALESCE(sqlc.narg(group_vision)::BOOL, group_vision),
    updated_at = CASE WHEN COALESCE(sqlc.narg(fog_enabled)::BOOL, fog_enabled) <> fog_enabled
                        OR COALESCE(sqlc.narg(group_vision)::BOOL, group_vision) <> group_vision
                      THEN sqlc.arg(now)::TIMESTAMPTZ ELSE updated_at END
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: GetMapLayers :one
-- The map's painted layers; no row means nothing is painted.
SELECT * FROM map_layers
WHERE map_id = $1;

-- name: UpsertMapLayers :exec
-- Writes the five layers of a map (NULL for a layer with nothing painted). The
-- handler holds the map's row lock (GetMapForUpdate), so two batches of paint
-- take turns.
INSERT INTO map_layers (map_id, difficult_terrain, walls, cover, light, doors, updated_at)
VALUES (sqlc.arg(map_id), sqlc.narg(difficult_terrain), sqlc.narg(walls), sqlc.narg(cover), sqlc.narg(light), sqlc.narg(doors), sqlc.arg(now))
ON CONFLICT (map_id) DO UPDATE
SET difficult_terrain = excluded.difficult_terrain, walls = excluded.walls, cover = excluded.cover,
    light = excluded.light, doors = excluded.doors, updated_at = excluded.updated_at;

-- name: DeleteMapLayers :execrows
-- Clears every layer of the map: the grid's columns or the image changed.
DELETE FROM map_layers
WHERE map_id = $1;

-- name: BumpMapLayersRevision :one
-- A painted layer changed, or all of them were cleared: readers must read them
-- again. It leaves updated_at and revision (the name and image's guard) alone.
UPDATE maps
SET layers_revision = layers_revision + 1
WHERE id = $1
RETURNING layers_revision;

-- name: BumpMapLightRevision :one
-- The painted light changed. No player reads the light, so this is not the
-- number they see: the master reads layers_revision + light_revision.
UPDATE maps
SET light_revision = light_revision + 1
WHERE id = $1
RETURNING light_revision;

-- name: GetMapTileInfo :one
-- What the tile route needs of a map, in one read, from the map's ID alone: its
-- campaign, whether players see it, the fog and grid, and its image's size and type.
SELECT m.id, m.campaign_id, m.image_id, m.revealed_at, m.grid_columns, m.grid_factor, m.fog_enabled,
       g.width AS image_width, g.height AS image_height, g.content_type AS image_content_type
FROM maps AS m
JOIN gallery_images AS g ON g.id = m.image_id
WHERE m.id = $1;

-- name: GetMapGrid :one
-- A map's grid and its image's size, for the rows (package play, through
-- SessionMaps).
SELECT m.grid_columns, m.grid_factor, g.width AS image_width, g.height AS image_height
FROM maps AS m
JOIN gallery_images AS g ON g.id = m.image_id
WHERE m.campaign_id = $1 AND m.id = $2;

-- name: GetMapPointInCampaign :one
-- A point by its ID alone, if it is on one of the campaign's maps: the
-- battle point a combat starts from.
SELECT p.* FROM map_points AS p
JOIN maps AS m ON m.id = p.map_id
WHERE m.campaign_id = $1 AND p.id = $2;

-- name: UpsertMapTokenPosition :exec
-- Where a combatant ended its combat (package play): the token moves, or is
-- created visible (a player's character starts visible, like PlaceMapToken).
-- An existing token keeps its hidden flag.
INSERT INTO map_tokens (map_id, character_id, x_bp, y_bp, hidden, updated_at)
VALUES ($1, $2, $3, $4, false, $5)
ON CONFLICT (map_id, character_id) DO UPDATE
SET x_bp = excluded.x_bp, y_bp = excluded.y_bp, updated_at = excluded.updated_at;

-- name: DeleteMap :one
-- Points and tokens go with the map (CASCADE); submap points of other maps
-- lose their target, and a session's current map is unset (SET NULL).
DELETE FROM maps
WHERE campaign_id = $1 AND id = $2
RETURNING *;

-- name: CountMapPoints :one
-- The map's points, for the limit, inside the transaction that inserts one.
SELECT count(*)::INT4 AS point_count FROM map_points
WHERE map_id = $1;

-- name: InsertMapPoint :one
-- create_key and create_hash are the idempotency key of CreateMapPoint and the hash of its
-- request (NULL when the call sent no key): a retry reads the first point with
-- GetMapPointByCreateKey, like a retried "Pôr no mapa".
-- A new point starts hidden (revealed_at NULL), unless the caller gives revealed_at: a generated
-- dungeon's stairs are born revealed, like a door (MAP-LANGUAGE-E10).
INSERT INTO map_points (
    map_id, kind, name, description, hooks, show_dc, x_bp, y_bp, target_map_id,
    trap, trap_state, trap_triggered_at, treasure_value_po, light_preset, light_bright_ft, light_dim_ft, created_at, updated_at, revealed_at, stairs,
    create_key, create_hash
)
VALUES (
    sqlc.arg(map_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(description), sqlc.arg(hooks), sqlc.arg(show_dc), sqlc.arg(x_bp), sqlc.arg(y_bp),
    sqlc.narg(target_map_id), sqlc.narg(trap), sqlc.narg(trap_state), sqlc.narg(trap_triggered_at), sqlc.narg(treasure_value_po),
    sqlc.narg(light_preset), sqlc.narg(light_bright_ft), sqlc.narg(light_dim_ft), sqlc.arg(now), sqlc.arg(now), sqlc.narg(revealed_at), sqlc.narg(stairs),
    sqlc.narg(create_key), sqlc.narg(create_hash)
)
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: ListMapPoints :many
-- The map's points, oldest first. The handler filters them for a player.
SELECT * FROM map_points
WHERE map_id = $1
ORDER BY created_at, id;

-- name: GetMapPointForUpdate :one
-- The caller checked first that the map is the campaign's.
SELECT * FROM map_points
WHERE map_id = $1 AND id = $2
FOR UPDATE;

-- name: UpdateMapPoint :one
-- Every column the API may change, with the values the handler worked out
-- from the request and the current row.
UPDATE map_points
SET kind = sqlc.arg(kind), name = sqlc.arg(name), description = sqlc.arg(description), hooks = sqlc.arg(hooks),
    show_dc = sqlc.arg(show_dc), x_bp = sqlc.arg(x_bp), y_bp = sqlc.arg(y_bp), target_map_id = sqlc.narg(target_map_id),
    revealed_at = sqlc.narg(revealed_at), trap = sqlc.narg(trap), trap_state = sqlc.narg(trap_state), trap_triggered_at = sqlc.narg(trap_triggered_at),
    treasure_value_po = sqlc.narg(treasure_value_po), treasure_found_at = sqlc.narg(treasure_found_at),
    treasure_session_id = sqlc.narg(treasure_session_id), light_preset = sqlc.narg(light_preset),
    light_bright_ft = sqlc.narg(light_bright_ft), light_dim_ft = sqlc.narg(light_dim_ft), updated_at = sqlc.arg(now)
WHERE map_id = sqlc.arg(map_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: DeleteMapPoint :one
DELETE FROM map_points
WHERE map_id = $1 AND id = $2
RETURNING *;

-- name: ListMapTokens :many
-- The map's tokens. The handler orders them by character and filters them
-- for a player.
SELECT * FROM map_tokens
WHERE map_id = $1;

-- name: GetMapTokenForUpdate :one
SELECT * FROM map_tokens
WHERE map_id = $1 AND character_id = $2
FOR UPDATE;

-- name: InsertMapToken :one
INSERT INTO map_tokens (map_id, character_id, x_bp, y_bp, hidden, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: MoveMapToken :one
UPDATE map_tokens
SET x_bp = $3, y_bp = $4, updated_at = $5
WHERE map_id = $1 AND character_id = $2
RETURNING *;

-- name: SetMapTokenHidden :one
UPDATE map_tokens
SET hidden = sqlc.arg(hidden), updated_at = CASE WHEN hidden = sqlc.arg(hidden) THEN updated_at ELSE sqlc.arg(now)::TIMESTAMPTZ END
WHERE map_id = sqlc.arg(map_id) AND character_id = sqlc.arg(character_id)
RETURNING *;

-- name: DeleteMapToken :one
DELETE FROM map_tokens
WHERE map_id = $1 AND character_id = $2
RETURNING *;

-- name: SetMapTokenCarriedLight :one
-- The light a character carries (MR-036): a preset's key, NULL for none. Moving
-- the token is a change to it (the master and the owner read it again).
UPDATE map_tokens
SET carried_light = sqlc.narg(carried_light), updated_at = sqlc.arg(now)
WHERE map_id = sqlc.arg(map_id) AND character_id = sqlc.arg(character_id)
RETURNING *;

-- name: ListMapCreatureTokens :many
-- The creatures' tokens on the map (MR-037). The handler lists only the live
-- creatures of living characters.
SELECT * FROM map_creature_tokens
WHERE map_id = $1;

-- name: UpsertMapCreatureToken :one
-- Places a creature's token, or moves it if it is on the map already.
INSERT INTO map_creature_tokens (map_id, creature_id, x_bp, y_bp, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (map_id, creature_id) DO UPDATE
SET x_bp = excluded.x_bp, y_bp = excluded.y_bp, updated_at = excluded.updated_at
RETURNING *;

-- name: MoveMapCreatureToken :exec
-- Where a creature ended its combat (package play): an existing token moves; a
-- creature that has none gets none (the master places creature tokens).
UPDATE map_creature_tokens
SET x_bp = $3, y_bp = $4, updated_at = $5
WHERE map_id = $1 AND creature_id = $2;

-- name: DeleteMapCreatureToken :one
DELETE FROM map_creature_tokens
WHERE map_id = $1 AND creature_id = $2
RETURNING *;

-- name: ListPointRevealsOfMap :many
-- Who knows each trap of a map, for the master's read.
SELECT r.* FROM map_point_reveals AS r
JOIN map_points AS p ON p.id = r.point_id
WHERE p.map_id = $1
ORDER BY r.at, r.character_id;

-- name: ListPointRevealsOfCampaign :many
-- Every trap reveal of the campaign's maps, with whether the trap is already
-- visible to everyone: what a player's reads need to know which traps their
-- characters know (RN-10).
SELECT r.point_id, p.map_id, r.character_id,
       (p.revealed_at IS NOT NULL OR p.trap_triggered_at IS NOT NULL)::BOOL AS public
FROM map_point_reveals AS r
JOIN map_points AS p ON p.id = r.point_id
JOIN maps AS m ON m.id = p.map_id
WHERE m.campaign_id = $1;

-- name: InsertPointReveal :execrows
-- Tells a character about a trap, once: a second reveal inserts nothing.
INSERT INTO map_point_reveals (point_id, character_id, how, at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (point_id, character_id) DO NOTHING;

-- name: SetTreasureFound :one
-- Marks a treasure found (MR-041). The session is the one open at that time, or
-- NULL. A treasure already found keeps its first time and session.
UPDATE map_points
SET treasure_found_at = COALESCE(treasure_found_at, sqlc.arg(found_at)::TIMESTAMPTZ),
    treasure_session_id = CASE WHEN treasure_found_at IS NULL THEN sqlc.narg(session_id)::UUID ELSE treasure_session_id END,
    updated_at = sqlc.arg(now)
WHERE map_id = sqlc.arg(map_id) AND id = sqlc.arg(id) AND kind = 'treasure'
RETURNING *;

-- name: ClearTreasureFound :one
-- Takes the found mark off a treasure.
UPDATE map_points
SET treasure_found_at = NULL, treasure_session_id = NULL, updated_at = sqlc.arg(now)
WHERE map_id = sqlc.arg(map_id) AND id = sqlc.arg(id) AND kind = 'treasure'
RETURNING *;

-- name: InsertTreasureFinder :exec
INSERT INTO map_treasure_finders (point_id, character_id)
VALUES ($1, $2)
ON CONFLICT (point_id, character_id) DO NOTHING;

-- name: DeleteTreasureFinders :exec
DELETE FROM map_treasure_finders
WHERE point_id = $1;

-- name: ListTreasureFindersOfMap :many
-- Who found each treasure of a map.
SELECT f.* FROM map_treasure_finders AS f
JOIN map_points AS p ON p.id = f.point_id
WHERE p.map_id = $1;

-- name: ImageIsOnAVisibleMap :one
-- Whether the image is the background of a map the players see now: a
-- revealed map, or the open session's current map (NULL when none). The
-- image route asks it for a player (RN-10); maps_image_id_idx finds the
-- maps.
SELECT EXISTS (
    SELECT 1 FROM maps
    WHERE campaign_id = sqlc.arg(campaign_id) AND image_id = sqlc.arg(image_id)
      AND (revealed_at IS NOT NULL OR id = sqlc.narg(current_map_id)::UUID)
);

-- name: LeaveImage :execrows
-- Leaves the campaign's gallery image with the players (MR-028). Selecting
-- from gallery_images makes an image deleted meanwhile, or another
-- campaign's, insert nothing; an image already left keeps its left_at.
INSERT INTO campaign_left_images (campaign_id, image_id, left_at)
SELECT g.campaign_id, g.id, sqlc.arg(now)::TIMESTAMPTZ FROM gallery_images g
WHERE g.campaign_id = sqlc.arg(campaign_id) AND g.id = sqlc.arg(image_id)
ON CONFLICT (campaign_id, image_id) DO NOTHING;

-- name: ListLeftImages :many
-- The images left with the players, in the order they were left (id breaks
-- ties).
SELECT g.* FROM campaign_left_images l
JOIN gallery_images g ON g.id = l.image_id
WHERE l.campaign_id = $1
ORDER BY l.left_at, g.id;

-- name: TakeBackLeftImage :execrows
DELETE FROM campaign_left_images
WHERE campaign_id = $1 AND image_id = $2;

-- name: ImageIsLeft :one
-- Whether the image is left with the players. The image route asks it for a
-- player (RN-10).
SELECT EXISTS (
    SELECT 1 FROM campaign_left_images
    WHERE campaign_id = $1 AND image_id = $2
);

-- The actions of an RP scene (MR-015). A scene point has at most 20; the
-- handler counts them inside the transaction that inserts one.

-- name: ListSceneActions :many
-- One point's actions, in order.
SELECT * FROM scene_actions
WHERE point_id = $1
ORDER BY position, created_at, id;

-- name: ListSceneActionsOfMap :many
-- Every action of a map's points, for a map read: grouped by the handler,
-- each point's in order.
SELECT a.* FROM scene_actions AS a
JOIN map_points AS p ON p.id = a.point_id
WHERE p.map_id = $1
ORDER BY a.point_id, a.position, a.created_at, a.id;

-- name: InsertSceneAction :one
-- create_key and create_hash are the idempotency key of AddSceneAction and the hash of its request
-- (NULL when the call sent no key). The point is locked first, so a retry reads the first action
-- with GetSceneActionByCreateKey and never races it.
INSERT INTO scene_actions (point_id, position, key, name, dc, max_attempts, create_key, create_hash, created_at, updated_at)
VALUES (sqlc.arg(point_id), sqlc.arg(position), sqlc.arg(key), sqlc.arg(name), sqlc.narg(dc), sqlc.arg(max_attempts), sqlc.narg(create_key), sqlc.narg(create_hash), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetSceneActionByCreateKey :one
-- The action an AddSceneAction with this idempotency key made, if any (the key carries the
-- campaign's ID).
SELECT * FROM scene_actions WHERE create_key = $1;

-- name: GetSceneActionForUpdate :one
SELECT * FROM scene_actions
WHERE point_id = $1 AND id = $2
FOR UPDATE;

-- name: UpdateSceneAction :one
UPDATE scene_actions
SET key = sqlc.arg(key), name = sqlc.arg(name), dc = sqlc.narg(dc), max_attempts = sqlc.arg(max_attempts), updated_at = sqlc.arg(now)
WHERE point_id = sqlc.arg(point_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: SetSceneActionPosition :exec
UPDATE scene_actions
SET position = $2
WHERE id = $1;

-- name: DeleteSceneAction :execrows
DELETE FROM scene_actions
WHERE point_id = $1 AND id = $2;

-- name: DeleteSceneActionsOfPoint :exec
-- A point that stops being a scene has no actions.
DELETE FROM scene_actions
WHERE point_id = $1;

-- name: GetScenePoint :one
-- A SCENE point of the campaign's maps by its ID alone, hidden or not: the one
-- the master opens in a session (package play, through SessionMaps).
SELECT p.* FROM map_points AS p
JOIN maps AS m ON m.id = p.map_id
WHERE m.campaign_id = $1 AND p.id = $2 AND p.kind = 'scene';

-- name: ListSceneClues :many
-- A SCENE point's clues, in the master's order.
SELECT * FROM scene_clues
WHERE point_id = $1
ORDER BY position, created_at, id;

-- name: ListSceneCluesOfMap :many
-- Every clue of a map's points, for the master's map read: grouped by the
-- handler, each point's in order.
SELECT c.* FROM scene_clues AS c
JOIN map_points AS p ON p.id = c.point_id
WHERE p.map_id = $1
ORDER BY c.point_id, c.position, c.created_at, c.id;

-- name: InsertSceneClue :one
-- create_key and create_hash are the idempotency key of AddSceneClue and the hash of its request
-- (NULL when the call sent no key), read back with GetSceneClueByCreateKey.
INSERT INTO scene_clues (point_id, position, text, create_key, create_hash, created_at, updated_at)
VALUES (sqlc.arg(point_id), sqlc.arg(position), sqlc.arg(text), sqlc.narg(create_key), sqlc.narg(create_hash), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetSceneClueByCreateKey :one
-- The clue an AddSceneClue with this idempotency key made, if any (the key carries the
-- campaign's ID).
SELECT * FROM scene_clues WHERE create_key = $1;

-- name: GetSceneClueForUpdate :one
SELECT * FROM scene_clues
WHERE point_id = $1 AND id = $2
FOR UPDATE;

-- name: UpdateSceneClue :one
UPDATE scene_clues
SET text = sqlc.arg(text), updated_at = sqlc.arg(now)
WHERE point_id = sqlc.arg(point_id) AND id = sqlc.arg(id)
RETURNING *;

-- name: SetSceneCluePosition :exec
UPDATE scene_clues SET position = $2 WHERE id = $1;

-- name: DeleteSceneClue :execrows
DELETE FROM scene_clues
WHERE point_id = $1 AND id = $2;

-- name: DeleteSceneCluesOfPoint :exec
-- A point that stops being a scene has no clues. What players already
-- received stays (scene_clue_reveals keeps its own copy of the text).
DELETE FROM scene_clues
WHERE point_id = $1;

-- name: GetSceneClueInCampaign :one
-- A clue by its ID alone, if it is on a point of the campaign's maps: the one
-- the master reveals.
SELECT c.* FROM scene_clues AS c
JOIN map_points AS p ON p.id = c.point_id
JOIN maps AS m ON m.id = p.map_id
WHERE m.campaign_id = $1 AND c.id = $2;

-- name: InsertClueReveal :execrows
-- Gives a clue to a player, once: the unique index on (clue_id, user_id) turns
-- a second reveal into no row at all. It copies the clue's text, so what the
-- player received stays as it was said.
INSERT INTO scene_clue_reveals (campaign_id, clue_id, point_id, user_id, character_id, text, revealed_at)
VALUES (sqlc.arg(campaign_id), sqlc.arg(clue_id), sqlc.arg(point_id), sqlc.arg(user_id), sqlc.narg(character_id), sqlc.arg(text), sqlc.arg(now))
ON CONFLICT (clue_id, user_id) DO NOTHING;

-- name: HasClueReveal :one
-- Whether the player has this clue (the cipher puzzle shows them where the key is only
-- once they found it).
SELECT EXISTS (
    SELECT 1 FROM scene_clue_reveals WHERE campaign_id = $1 AND clue_id = $2 AND user_id = $3
) AS found;

-- name: ListClueRevealsOfPoint :many
-- Who has each clue of a point, oldest reveal first.
SELECT clue_id, character_id, revealed_at FROM scene_clue_reveals
WHERE point_id = $1 AND clue_id IS NOT NULL
ORDER BY revealed_at, id;

-- name: ListClueRevealsOfMap :many
-- Who has each clue of a map's points.
SELECT r.point_id, r.clue_id, r.character_id, r.revealed_at FROM scene_clue_reveals AS r
JOIN map_points AS p ON p.id = r.point_id
WHERE p.map_id = $1 AND r.clue_id IS NOT NULL
ORDER BY r.revealed_at, r.id;

-- name: UpsertSceneDiscovery :exec
-- The group discovered a scene (MR-030): the first time wins.
INSERT INTO scene_discoveries (campaign_id, point_id, discovered_at)
VALUES ($1, $2, $3)
ON CONFLICT (campaign_id, point_id) DO NOTHING;

-- name: ListDiscoveredScenes :many
-- The scenes the group discovered, with their current names, oldest discovery
-- first. A point that stopped being a scene is not listed, nor is one of a map the
-- players cannot open (not revealed, and not the session's current map).
SELECT p.id, p.name FROM scene_discoveries AS d
JOIN map_points AS p ON p.id = d.point_id
JOIN maps AS m ON m.id = p.map_id
WHERE d.campaign_id = sqlc.arg(campaign_id) AND p.kind = 'scene'
  AND (m.revealed_at IS NOT NULL OR m.id = sqlc.narg(current_map_id)::UUID)
ORDER BY d.discovered_at, p.id;

-- name: ListReceivedClues :many
-- The clues revealed to a player in a campaign, newest first.
SELECT id, point_id, text, revealed_at FROM scene_clue_reveals
WHERE campaign_id = $1 AND user_id = $2
ORDER BY revealed_at DESC, id;

-- name: DeletePointReveals :exec
-- A point that stops being a trap tells nobody anything.
DELETE FROM map_point_reveals
WHERE point_id = $1;

-- name: ListPointRevealsOfPoint :many
-- Who knows one trap: the players to tell when it changes or goes away.
SELECT * FROM map_point_reveals
WHERE point_id = $1;

-- name: GetMapTreasureLocks :one
-- Whether the map holds a treasure that was found or converted: such a map
-- cannot be deleted, so a found treasure never vanishes from a session's summary.
SELECT (count(*) FILTER (WHERE treasure_found_at IS NOT NULL))::INT4 AS found,
       (count(*) FILTER (WHERE treasure_converted_award_id IS NOT NULL))::INT4 AS converted
FROM map_points
WHERE map_id = $1 AND kind = 'treasure';

-- What each player saw of a map with the fog of war on (MR-036, D6). The bytes are
-- a packed bitmap in package rules/grid's layout; the handlers size them by the
-- map's grid.

-- name: GetMapVisionMemory :one
-- One player's memory of a map; no row means they have seen nothing yet. The
-- caller compares epoch with the map's vision_epoch: an older one reads as empty.
SELECT seen, epoch FROM map_vision_memory
WHERE map_id = $1 AND user_id = $2;

-- name: ListMapVisionMemory :many
-- Every player's memory of a map (a stream's hint needs each player's view).
SELECT user_id, seen, epoch FROM map_vision_memory
WHERE map_id = $1;

-- name: UpsertMapVisionMemory :execrows
-- The caller already merged the new squares into the old bytes, so the memory
-- only grows. It writes only while the map is still in the epoch the bytes were
-- built for: a clear that happened meanwhile bumped it, and nothing is written.
INSERT INTO map_vision_memory (map_id, user_id, seen, epoch, updated_at)
SELECT m.id, sqlc.arg(user_id)::UUID, sqlc.arg(seen)::BYTEA, m.vision_epoch, sqlc.arg(updated_at)::TIMESTAMPTZ
FROM maps AS m
WHERE m.id = sqlc.arg(map_id) AND m.vision_epoch = sqlc.arg(epoch)
ON CONFLICT (map_id, user_id) DO UPDATE
SET seen = excluded.seen, epoch = excluded.epoch, updated_at = excluded.updated_at;

-- name: SetMapVisionMemorySeen :exec
-- A new calibration (MR-025) scales every player's memory to the new grid: the same
-- player, the bytes of the bigger grid, in the epoch the calibration started (the
-- caller bumped the map's, so a refresh of the old grid cannot write over it).
-- updated_at moves, as for any write.
UPDATE map_vision_memory
SET seen = sqlc.arg(seen), epoch = sqlc.arg(epoch), updated_at = sqlc.arg(updated_at)
WHERE map_id = sqlc.arg(map_id) AND user_id = sqlc.arg(user_id);

-- name: ClearMapVisionMemory :execrows
-- "Esquecer o que foi visto", and a new grid or image: every player forgets. The
-- epoch goes up, so a refresh that was already running cannot write the old bitmap.
UPDATE maps SET vision_epoch = vision_epoch + 1
WHERE id = $1;

-- name: DeleteMapVisionMemory :execrows
-- The rows of the cleared epochs (they already read as empty); tidying only.
DELETE FROM map_vision_memory
WHERE map_id = $1;

-- name: ImageIsOnAFogMap :one
-- Whether the image is the background of a map with the fog of war on: a player
-- never receives it, whatever else shows it (RN-10, MR-036). The image route asks
-- it for a player. A map's image is found by maps_image_id_idx.
SELECT EXISTS (
    SELECT 1 FROM maps
    WHERE campaign_id = $1 AND image_id = $2 AND fog_enabled
);

-- name: ImageIsUsedElsewhere :one
-- Whether the image is also used another way than as the background of the
-- given map: the background of any other map, or an image the campaign left with
-- the players. Turning the fog on copies such an image first, so the fog map's
-- image is its own.
SELECT (
    EXISTS (
        SELECT 1 FROM maps AS m
        WHERE m.campaign_id = sqlc.arg(campaign_id) AND m.image_id = sqlc.arg(image_id) AND m.id <> sqlc.arg(map_id)
    )
    OR EXISTS (
        SELECT 1 FROM campaign_left_images AS l
        WHERE l.campaign_id = sqlc.arg(campaign_id) AND l.image_id = sqlc.arg(image_id)
    )
)::BOOL AS used;

-- name: SetMapImageOnly :one
-- A new copy of the image becomes the map's, without touching the name or the
-- revision's guard: the map's revision still moves, since the image changed.
UPDATE maps
SET image_id = sqlc.arg(image_id), revision = revision + 1, updated_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id)
RETURNING *;

-- Gold (slice 9.11, MR-032): the session summary's "Mais tesouro encontrado".

-- name: ListTreasureFindsOfSession :many
-- One row per treasure and finder of the treasures found while the game session
-- was open. A treasure unmarked later has no session and no finders.
SELECT p.id AS point_id, COALESCE(p.treasure_value_po, 0)::INT4 AS value_po, f.character_id
FROM map_points AS p
JOIN map_treasure_finders AS f ON f.point_id = p.id
WHERE p.kind = 'treasure' AND p.treasure_found_at IS NOT NULL AND p.treasure_session_id = sqlc.arg(session_id)::UUID
ORDER BY p.id, f.character_id;

-- name: SetTrapState :one
-- The live game changes a trap's state (MR-035, slice 9.8): it fires (state
-- 'triggered', with when), the master disarms it, or an undo puts back what
-- there was. The caller locked the point first.
UPDATE map_points
SET trap_state = sqlc.arg(trap_state), trap_triggered_at = sqlc.narg(trap_triggered_at), updated_at = sqlc.arg(now)
WHERE map_id = sqlc.arg(map_id) AND id = sqlc.arg(id) AND kind = 'trap'
RETURNING *;

-- name: ListPointRevealCharacters :many
-- The characters that know each trap of the map, for the noticing: one row per
-- trap and character.
SELECT r.point_id, r.character_id FROM map_point_reveals AS r
JOIN map_points AS p ON p.id = r.point_id
WHERE p.map_id = $1;

-- name: GetMapPoint :one
-- One point of a map, without locking it.
SELECT * FROM map_points
WHERE map_id = $1 AND id = $2;

-- name: GetMapPointByCreateKey :one
-- The point a "Pôr no mapa" with this idempotency key made (MR-044), if any. The key
-- carries the campaign's ID, so it is unique in the campaign.
SELECT * FROM map_points
WHERE create_key = $1;

-- name: InsertTreasurePoint :one
-- A hidden TREASURE point made by "Pôr no mapa" (MR-044), with the idempotency key
-- and the hash of the request. Two calls with the same key at once make one point:
-- the loser gets no row, and reads the winner's (GetMapPointByCreateKey).
INSERT INTO map_points (
    map_id, kind, name, description, hooks, show_dc, x_bp, y_bp, treasure_value_po, create_key, create_hash, created_at, updated_at
)
VALUES (
    sqlc.arg(map_id), 'treasure', sqlc.arg(name), sqlc.arg(description), '', FALSE, sqlc.arg(x_bp), sqlc.arg(y_bp),
    sqlc.arg(treasure_value_po), sqlc.arg(create_key), sqlc.arg(create_hash), sqlc.arg(now), sqlc.arg(now)
)
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: ListTrapNamesInCampaign :many
-- The names of traps by point ID, for the combat log: a trap that fired is
-- public, so its name may be told (MR-035). A deleted point is simply absent.
SELECT p.id, p.name FROM map_points AS p
JOIN maps AS m ON m.id = p.map_id
WHERE m.campaign_id = $1 AND p.kind = 'trap' AND p.id = ANY($2::uuid[]);

-- Generated images (MR-039, RN-28, ADR-0019). image_requests holds each
-- request and, by its rows of a month that were not refunded, the campaign's
-- monthly count.

-- name: GetImageRequestByKey :one
-- A retry with the same idempotency key finds the first request.
SELECT * FROM image_requests
WHERE campaign_id = $1 AND idempotency_key = $2;

-- name: GetImageRequest :one
SELECT * FROM image_requests
WHERE campaign_id = $1 AND id = $2;

-- name: GetImageRequestForImage :one
-- The request that made an image.
SELECT * FROM image_requests
WHERE campaign_id = $1 AND image_id = $2;

-- name: ExpireUnsentImageRequests :exec
-- A request that never left the server and is still pending long after it was
-- made lost its server (a restart): it fails and refunds its slot, so it never
-- blocks the month's count for good.
UPDATE image_requests
SET status = 'failed', reason = 'timeout', refunded = true, finished_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id) AND status = 'pending' AND sent_at IS NULL AND created_at < sqlc.arg(before);

-- name: ExpireSentImageRequests :exec
-- A request that left and is still pending long after it was sent: it timed
-- out. The slot stays spent (the call may have been billed), and a picture that
-- still arrives is stored (FinishImageRequestDone allows it, once).
UPDATE image_requests
SET status = 'failed', reason = 'timeout', finished_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id) AND status = 'pending' AND sent_at IS NOT NULL AND sent_at < sqlc.arg(before);

-- name: CountOpenImageRequests :one
-- The requests that may still put an image in the gallery: pending, or canceled
-- after the send without an image yet (only recent ones). The reserve step
-- counts them against the gallery's room, so a full gallery cannot fail after
-- the model was paid.
SELECT count(*)::INT4 FROM image_requests
WHERE campaign_id = $1 AND created_at > $2
  AND (status = 'pending' OR (status = 'canceled' AND sent_at IS NOT NULL AND image_id IS NULL));

-- name: CountImageSlots :one
-- The slots the campaign spent in a month: its requests that were not
-- refunded. Read inside the reserving transaction (SERIALIZABLE makes two
-- racing requests for the last slot retry, and the second one sees it taken).
SELECT count(*)::INT4 FROM image_requests
WHERE campaign_id = $1 AND quota_month = $2 AND NOT refunded;

-- name: CountImageRequestsSince :one
-- The slots the whole server spent since a moment (the start of Brazil's day):
-- the requests that were not refunded, in any campaign. The server's daily cap
-- on the Gemini bill reads it; image_requests_created_at_idx (migration 00176)
-- finds the day's rows.
SELECT count(*)::INT4 FROM image_requests
WHERE created_at >= $1 AND NOT refunded;

-- name: NextImageRequestNumber :one
SELECT (COALESCE(max(number), 0) + 1)::INT4 FROM image_requests
WHERE campaign_id = $1;

-- name: InsertImageRequest :one
INSERT INTO image_requests (
    id, campaign_id, requested_by, idempotency_key, kind, prompt, style, aspect_ratio, model,
    reference_ids, character_ids, source_image_id, number, quota_month, status, reason, refunded, created_at,
    map_id, map_image_id, map_grid_columns, map_grid_factor, map_width, map_height,
    map_plan_hash, pad_x0, pad_y0, pad_x1, pad_y1, image_name, idempotency_hash
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'pending', '', false, $15,
    $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)
RETURNING *;

-- name: MarkImageRequestSent :execrows
-- The moment the request leaves the server. No row means it was canceled
-- first, and nothing is sent.
UPDATE image_requests
SET sent_at = $3
WHERE campaign_id = $1 AND id = $2 AND status = 'pending' AND sent_at IS NULL;

-- name: CancelUnsentImageRequest :execrows
-- Cancel before the request left: the slot is refunded.
UPDATE image_requests
SET status = 'canceled', refunded = true, finished_at = $3
WHERE campaign_id = $1 AND id = $2 AND status = 'pending' AND sent_at IS NULL;

-- name: CancelSentImageRequest :execrows
-- Cancel after the request left: only the wait stops. The slot stays spent,
-- and an image that comes is stored all the same.
UPDATE image_requests
SET status = 'canceled', finished_at = $3
WHERE campaign_id = $1 AND id = $2 AND status = 'pending' AND sent_at IS NOT NULL;

-- name: FinishImageRequestFailed :exec
-- Nothing was generated: refused or failed, and the slot is back. A request
-- the master canceled in the meantime stays canceled (with the slot back).
UPDATE image_requests
SET status = CASE WHEN status = 'canceled' THEN status ELSE sqlc.arg(status) END,
    reason = sqlc.arg(reason), refunded = true, finished_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id) AND status IN ('pending', 'canceled') AND NOT refunded;

-- name: FinishImageRequestSpent :exec
-- The model answered but the picture could not be stored (the gallery filled up
-- meanwhile, or the answer cannot be used): the call was made and billed, so the slot
-- stays spent. A request the master canceled in the meantime stays canceled.
UPDATE image_requests
SET status = CASE WHEN status = 'canceled' THEN status ELSE sqlc.arg(status) END,
    reason = sqlc.arg(reason), finished_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id) AND status IN ('pending', 'canceled') AND NOT refunded;

-- name: FinishImageRequestDone :execrows
-- The image is in the gallery. Allowed once, from pending, from canceled (it
-- keeps its status, with the image) and from a timeout after the send; zero
-- rows means it ended another way or already has an image, and the caller
-- rolls its gallery insert back.
UPDATE image_requests
SET status = CASE WHEN status = 'canceled' THEN status ELSE 'done' END,
    reason = '', image_id = sqlc.arg(image_id), finished_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id) AND image_id IS NULL
  AND (status IN ('pending', 'canceled') OR (status = 'failed' AND reason = 'timeout' AND sent_at IS NOT NULL AND NOT refunded));

-- name: ListImageChain :many
-- Every image of the edit tree an image belongs to (its root, and every edit
-- below it), oldest first, with the master's text that made each one.
WITH RECURSIVE up AS (
    SELECT g.id, g.parent_image_id FROM gallery_images g
    WHERE g.campaign_id = $1 AND g.id = $2
    UNION ALL
    SELECT p.id, p.parent_image_id FROM gallery_images p JOIN up ON p.id = up.parent_image_id
), tree AS (
    SELECT g.id, g.parent_image_id FROM gallery_images g
    WHERE g.id = (SELECT u.id FROM up u WHERE u.parent_image_id IS NULL)
    UNION ALL
    SELECT c.id, c.parent_image_id FROM gallery_images c JOIN tree t ON c.parent_image_id = t.id
)
SELECT g.id, g.campaign_id, g.uploaded_by, g.name, g.content_type, g.width, g.height, g.byte_size,
       g.created_at, g.generated, g.parent_image_id, g.generated_kind,
       COALESCE(r.prompt, '')::TEXT AS prompt, COALESCE(r.number, 0)::INT4 AS number
FROM tree t
JOIN gallery_images g ON g.id = t.id
LEFT JOIN image_requests r ON r.image_id = g.id
ORDER BY g.created_at, g.id;

-- Generated dungeons (MR-010, slice 10.6d): the record DungeonService keeps of a map
-- it made. The master's alone: nothing here is ever sent to a player (RN-10).

-- name: InsertGeneratedDungeon :exec
INSERT INTO generated_dungeons (map_id, generator_version, seed, width, height, options, cells, rooms, image_id, created_at)
VALUES (sqlc.arg(map_id), sqlc.arg(generator_version), sqlc.arg(seed), sqlc.arg(width), sqlc.arg(height),
        sqlc.arg(options), sqlc.arg(cells), sqlc.arg(rooms), sqlc.arg(image_id), sqlc.arg(created_at));

-- name: SetGeneratedDungeonImage :exec
-- "Redesenhar" drew a new image for the map: it is the dungeon's own now.
UPDATE generated_dungeons SET image_id = sqlc.arg(image_id) WHERE map_id = sqlc.arg(map_id);

-- name: GetGeneratedDungeon :one
-- The map's dungeon record, found through the map so that a map of another campaign
-- is "not found".
SELECT d.* FROM generated_dungeons AS d
JOIN maps AS m ON m.id = d.map_id
WHERE m.campaign_id = $1 AND d.map_id = $2;

-- name: MarkImageRequestUsed :exec
-- "Usar como imagem do mapa" set this image on the map: a retry answers the same
-- while it is still the map's image.
UPDATE image_requests
SET used_map_image_id = sqlc.arg(used_map_image_id)
WHERE campaign_id = sqlc.arg(campaign_id) AND id = sqlc.arg(id);
