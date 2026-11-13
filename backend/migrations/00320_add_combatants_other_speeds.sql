-- +goose Up
-- The swim, climb and burrow speeds of a combatant, in feet, 0 for none (#567). They are copied from
-- the sheet when the combatant joins, like speed_fly_ft (00098), and shown to whoever reads the fly
-- speed. The grid does not spend them yet. Numbers only; no personal data.
--
-- Production is live: the columns only add, with every row as it was (0), so running the migration twice is safe.
ALTER TABLE combatants
    ADD COLUMN IF NOT EXISTS speed_swim_ft INT4 NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS speed_climb_ft INT4 NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS speed_burrow_ft INT4 NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE combatants
    DROP COLUMN IF EXISTS speed_burrow_ft,
    DROP COLUMN IF EXISTS speed_climb_ft,
    DROP COLUMN IF EXISTS speed_swim_ft;
