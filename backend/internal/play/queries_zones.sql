-- The areas spells keep on the map (migration 00267).

-- name: InsertSpellZone :one
INSERT INTO spell_zones (
    encounter_id, owner_id, spell_key, cast_id, slot_level, origin_col, origin_row, squares,
    save_ability, save_dc, damage_dice, damage_type, concentration, started_round, ends_round, created_at, def,
    wall_side, wall_dx, wall_dy, wall_length_ft
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
RETURNING *;

-- name: ListSpellZones :many
-- The zones of a combat, oldest first.
SELECT * FROM spell_zones
WHERE encounter_id = $1
ORDER BY created_at, id;

-- name: GetSpellZone :one
SELECT * FROM spell_zones
WHERE id = $1 AND encounter_id = $2;

-- name: SetSpellZonePlace :exec
-- A zone that moved: its point of origin and the squares it covers there.
UPDATE spell_zones
SET origin_col = $2, origin_row = $3, squares = $4
WHERE id = $1;

-- name: SetSpellZoneHitMarks :exec
UPDATE spell_zones
SET hit_marks = $2
WHERE id = $1;

-- name: DeleteSpellZone :exec
DELETE FROM spell_zones WHERE id = $1;

-- name: DeleteSpellZonesOfCast :exec
-- The undo of a cast takes away the zone it made.
DELETE FROM spell_zones WHERE encounter_id = $1 AND cast_id = $2;

-- name: DeleteConcentrationZonesOf :many
-- The zones a caster's concentration holds end with it; the rows that went, oldest first.
WITH changed AS (
    DELETE FROM spell_zones
    WHERE owner_id = $1 AND concentration
    RETURNING *
)
SELECT * FROM changed
ORDER BY created_at, id;
