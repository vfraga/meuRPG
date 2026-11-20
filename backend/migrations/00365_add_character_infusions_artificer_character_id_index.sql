-- +goose Up
-- The ON DELETE CASCADE of the artificer.
CREATE INDEX IF NOT EXISTS character_infusions_artificer_character_id_idx ON character_infusions (artificer_character_id);

-- +goose Down
DROP INDEX IF EXISTS character_infusions_artificer_character_id_idx;
