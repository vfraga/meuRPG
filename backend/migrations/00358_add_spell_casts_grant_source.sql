-- +goose Up
-- A long casting through a granted_spell effect remembers the effect it started through: grant_source is the source of that
-- grant (it names the table entry), so the finish casts at that grant's level with that grant's numbers. A row from before
-- has none (empty) and finishes as it did. Only a column is added, so running it twice is safe.
ALTER TABLE spell_casts ADD COLUMN IF NOT EXISTS grant_source TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE spell_casts DROP COLUMN IF EXISTS grant_source;
