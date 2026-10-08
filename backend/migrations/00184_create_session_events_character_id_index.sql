-- +goose Up
-- Deleting a character sets session_events.character_id to NULL (ON DELETE SET NULL);
-- without this index the cascade scans the whole history. Partial: most rows have none.
-- Creating it reads session_events once, so it is built in a quiet window.
CREATE INDEX IF NOT EXISTS session_events_character_id_idx
    ON session_events (character_id)
    WHERE character_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS session_events_character_id_idx;
