-- +goose Up
-- The master's choice for the TV at the table: follow what the table sees (tv_mirror) or one
-- pinned item (tv_pin, a TableScreenKind: 1 idle, 2 map, 3 image, 4 scene, 5 combat). tv_choice_revision
-- counts the master's writes of the choice, so a stale write is refused. The choice stays when its item ends.
-- tv_mirror is true while the TV follows the table; tv_pin is read only while it is false. The pin is never
-- cleared when its item ends. No personal data.
--
-- Every column has a constant default and the statement adds only what is missing (IF NOT EXISTS), so it is safe
-- on a table with rows and to run twice.
-- The check is its own named constraint, dropped first, so the migration can run twice (an inline
-- CHECK on ADD COLUMN IF NOT EXISTS adds a second constraint on the rerun in CockroachDB).
ALTER TABLE game_sessions
    ADD COLUMN IF NOT EXISTS tv_mirror BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS tv_pin SMALLINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tv_choice_revision INT NOT NULL DEFAULT 0;
ALTER TABLE game_sessions DROP CONSTRAINT IF EXISTS game_sessions_tv_pin_valid;
ALTER TABLE game_sessions ADD CONSTRAINT game_sessions_tv_pin_valid CHECK (tv_pin BETWEEN 1 AND 5);

-- +goose Down
ALTER TABLE game_sessions DROP CONSTRAINT IF EXISTS game_sessions_tv_pin_valid;
ALTER TABLE game_sessions
    DROP COLUMN IF EXISTS tv_choice_revision,
    DROP COLUMN IF EXISTS tv_pin,
    DROP COLUMN IF EXISTS tv_mirror;
