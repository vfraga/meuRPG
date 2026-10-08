-- +goose Up
-- Deleting an account sets combatants.user_id to NULL (ON DELETE SET NULL). Partial: NPCs have none.
CREATE INDEX IF NOT EXISTS combatants_user_id_idx
    ON combatants (user_id)
    WHERE user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS combatants_user_id_idx;
