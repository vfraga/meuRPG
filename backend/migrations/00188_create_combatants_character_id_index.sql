-- +goose Up
-- Deleting a character cascades to its combatants in every combat.
CREATE INDEX IF NOT EXISTS combatants_character_id_idx
    ON combatants (character_id);

-- +goose Down
DROP INDEX IF EXISTS combatants_character_id_idx;
