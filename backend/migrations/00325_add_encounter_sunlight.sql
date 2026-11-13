-- +goose Up
-- The sunlight of a combat (`SetSunlight`): sunlight_on says whether the combat is in sunlight, sunlight_scope where
-- it reaches ('map': the whole map, or the whole combat when it has none; 'squares': only sunlight_squares) and
-- sunlight_squares the sunlit squares, a JSON array of [col, row] pairs sorted by row then column. Master only; a
-- player never reads them. Turning the sunlight off keeps the scope and the squares. No personal data.
--
-- Every column has a constant default and the statement adds only what is missing (IF NOT EXISTS), so it is safe
-- on a table with rows and to run twice.
-- The checks are their own named constraints, dropped first, so the migration can run twice (an inline
-- CHECK on ADD COLUMN IF NOT EXISTS adds a second constraint on the rerun in CockroachDB).
ALTER TABLE encounters
    ADD COLUMN IF NOT EXISTS sunlight_on BOOL NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS sunlight_scope TEXT NOT NULL DEFAULT 'map',
    ADD COLUMN IF NOT EXISTS sunlight_squares JSONB NOT NULL DEFAULT '[]';
ALTER TABLE encounters DROP CONSTRAINT IF EXISTS encounters_sunlight_scope_valid;
ALTER TABLE encounters ADD CONSTRAINT encounters_sunlight_scope_valid CHECK (sunlight_scope IN ('map', 'squares'));
ALTER TABLE encounters DROP CONSTRAINT IF EXISTS encounters_sunlight_squares_valid;
ALTER TABLE encounters ADD CONSTRAINT encounters_sunlight_squares_valid
    CHECK (jsonb_typeof(sunlight_squares) = 'array' AND jsonb_array_length(sunlight_squares) <= 4096);

-- The master switched the sunlight (`sunlight_set`): the payload holds the state and the number of sunlit squares;
-- the row is secret and has no line in the combat log.
INSERT INTO session_event_kinds (kind) VALUES
    ('sunlight_set')
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DELETE FROM session_events WHERE kind = 'sunlight_set';
DELETE FROM session_event_kinds WHERE kind = 'sunlight_set';
ALTER TABLE encounters DROP CONSTRAINT IF EXISTS encounters_sunlight_squares_valid;
ALTER TABLE encounters DROP CONSTRAINT IF EXISTS encounters_sunlight_scope_valid;
ALTER TABLE encounters
    DROP COLUMN IF EXISTS sunlight_squares,
    DROP COLUMN IF EXISTS sunlight_scope,
    DROP COLUMN IF EXISTS sunlight_on;
