-- +goose Up
-- Removing a combatant cascades to the damage it took.
CREATE INDEX IF NOT EXISTS pending_damages_target_id_idx
    ON pending_damages (target_id);

-- +goose Down
DROP INDEX IF EXISTS pending_damages_target_id_idx;
