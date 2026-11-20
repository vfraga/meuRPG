-- +goose Up
-- One live record for each item of a holder.
CREATE UNIQUE INDEX IF NOT EXISTS character_infusions_one_live_item_idx
    ON character_infusions (holder_character_id, item_key) WHERE ended_at IS NULL;

-- +goose Down
-- CASCADE: CockroachDB keeps a unique index as a constraint and refuses to drop it without it.
DROP INDEX IF EXISTS character_infusions_one_live_item_idx CASCADE;
