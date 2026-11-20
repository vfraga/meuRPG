-- +goose Up
-- The ON DELETE CASCADE of the holder, and the live records on a holder's items.
CREATE INDEX IF NOT EXISTS character_infusions_holder_character_id_idx ON character_infusions (holder_character_id);

-- +goose Down
DROP INDEX IF EXISTS character_infusions_holder_character_id_idx;
