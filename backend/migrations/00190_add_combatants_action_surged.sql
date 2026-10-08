-- +goose Up
-- Action Surge can be used once on the same turn, even with two uses left
-- (fighter level 17). action_surged says it was used in the combatant's current
-- turn; the start of the next turn clears it.
ALTER TABLE combatants
    ADD COLUMN IF NOT EXISTS action_surged BOOL NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE combatants
    DROP COLUMN IF EXISTS action_surged;
