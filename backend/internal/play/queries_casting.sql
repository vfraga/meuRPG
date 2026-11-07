-- The spells cast outside a combat (casting.go). Every write below runs after the
-- caller locked the open session's row.

-- name: InsertSpellCast :one
INSERT INTO spell_casts (
    campaign_id, game_session_id, caster_id, spell_key, ritual, slot_level, slot_pact, status, concentrating,
    casting_minutes, lasts, duration_seconds, rest_ends, secret, started_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15
)
RETURNING *;

-- name: GetSpellCast :one
SELECT * FROM spell_casts
WHERE id = $1 AND campaign_id = $2;

-- name: ListLiveSpellCasts :many
-- The casts going or lasting in the campaign, oldest first. They last from one session
-- to the next, until something ends them.
SELECT * FROM spell_casts
WHERE campaign_id = $1 AND status IN ('casting', 'active')
ORDER BY started_at, id;

-- name: ListLiveSpellCastsOfCaster :many
SELECT * FROM spell_casts
WHERE caster_id = $1 AND status IN ('casting', 'active')
ORDER BY started_at, id;

-- name: ListLiveSpellCastsOfCasters :many
-- The live casts of several casters: what a combat's start carries.
SELECT * FROM spell_casts
WHERE caster_id = ANY(sqlc.arg(caster_ids)::UUID[]) AND status IN ('casting', 'active')
ORDER BY started_at, id;

-- name: ListSessionSpellCasts :many
-- The session's casts, newest first: the log.
SELECT * FROM spell_casts
WHERE game_session_id = $1
ORDER BY started_at DESC, id DESC
LIMIT $2;

-- name: ListCarriedSpellCasts :many
-- The casts whose concentration a combat's combatants hold now.
SELECT * FROM spell_casts
WHERE carried_encounter_id = $1 AND status IN ('casting', 'active')
ORDER BY started_at, id;

-- name: FinishSpellCast :one
-- The casting is done and the spell took effect: it lasts (active) or it is over
-- (ended), with what it did.
UPDATE spell_casts
SET status = $2, end_reason = $3, ended_at = $4, concentrating = $5, targets = $6,
    dice_count = $7, dice_sides = $8, roll_faces = $9, roll_total = $10, physical = $11,
    creature_ids = $12, cast_at = $13, slot_level = $14, slot_pact = $15
WHERE id = $1
RETURNING *;

-- name: EndSpellCast :one
-- A cast is over: ended (it took effect) or failed (it did not finish).
UPDATE spell_casts
SET status = $2, end_reason = $3, ended_at = $4, concentrating = false, carried_encounter_id = NULL
WHERE id = $1
RETURNING *;

-- name: SetSpellCastTargets :exec
UPDATE spell_casts SET targets = $2 WHERE id = $1;

-- name: SetSpellCastConcentration :exec
-- Who holds the concentration: the cast (carried NULL) or the combatant of a combat.
UPDATE spell_casts
SET concentrating = $2, carried_encounter_id = $3
WHERE id = $1;

-- name: SetCombatantMageArmorAC :exec
UPDATE combatants SET mage_armor_ac = $2 WHERE id = $1;

-- name: ClearMageArmorACOfCharacter :exec
-- Mage Armor ended: take it off the character's combatants in the combats that are not ended.
UPDATE combatants
SET mage_armor_ac = NULL
WHERE character_id = $1 AND mage_armor_ac IS NOT NULL
  AND encounter_id IN (SELECT id FROM encounters WHERE status <> 'ended');

-- name: SetMageArmorACOfCharacter :exec
-- Mage Armor is on the character: put it on its combatants in the combats that are not ended.
UPDATE combatants
SET mage_armor_ac = $2
WHERE character_id = $1
  AND encounter_id IN (SELECT id FROM encounters WHERE status <> 'ended');

-- name: ListLiveSpellCastsOnTarget :many
-- The live casts that name the character among their targets (Mage Armor, Aid): what
-- is still on a target when one of them ends.
SELECT * FROM spell_casts
WHERE campaign_id = $1 AND status = 'active' AND targets @> sqlc.arg(target)::JSONB
ORDER BY started_at, id;

-- name: SetSpellCastDuration :exec
-- The game time a cast has left, when a combat gives back the rounds an effect had left
-- (6 seconds each).
UPDATE spell_casts SET duration_seconds = $2 WHERE id = $1;
