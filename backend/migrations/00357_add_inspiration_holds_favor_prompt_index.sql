-- +goose Up
-- The refund prompt a held roll remembers (inspiration_holds.favor_prompt_id, added with the favor passes and refund prompts) is looked up when the prompt is deleted (the
-- ON DELETE SET NULL) and when a prompt closes. Most holds carry none, so the partial index holds only the ones that do. Nothing but the
-- index changes, so running it twice is safe.
CREATE INDEX IF NOT EXISTS inspiration_holds_favor_prompt_idx ON inspiration_holds (favor_prompt_id) WHERE favor_prompt_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS inspiration_holds_favor_prompt_idx;
