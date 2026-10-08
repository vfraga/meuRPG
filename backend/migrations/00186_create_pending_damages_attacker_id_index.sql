-- +goose Up
-- Removing a combatant cascades to the damage it dealt. A trap's damage has no attacker,
-- so the index is partial.
CREATE INDEX IF NOT EXISTS pending_damages_attacker_id_idx
    ON pending_damages (attacker_id)
    WHERE attacker_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS pending_damages_attacker_id_idx;
