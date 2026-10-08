-- +goose Up
-- Deleting an account sets session_events.actor_user_id to NULL (ON DELETE SET NULL);
-- without this index the cascade scans the whole history. Partial: system events have none.
CREATE INDEX IF NOT EXISTS session_events_actor_user_id_idx
    ON session_events (actor_user_id)
    WHERE actor_user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS session_events_actor_user_id_idx;
