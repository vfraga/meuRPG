-- +goose Up
-- A saving throw that failed also waits for the fighter's Indomitable (SRD 5.1, Fighter: "You can
-- reroll a saving throw that you fail. If you do so, you must use the new roll, and you can't use
-- this feature again until you finish a long rest."). It comes after the halfling's Lucky and the
-- Bardic Inspiration die (inspiration_holds, 00244, 00254): the player sees the result of the save
-- and decides before it is told. The held rolls of the effect's saving throw, the concentration save
-- and the death save are the ones that ask it.
--   - indomitable_ask is true once the question was asked: the save failed on the roll as it stood
--     after Lucky and the die, or the table hides the DC and so asks on every save. The question
--     waits while it is true and indomitable_answer_key and answer_key are NULL.
--   - indomitable_failed says the question may tell the save failed (the roller may know it); it is
--     false when the question comes on every save, with the result unknown to the roller.
--   - indomitable_answer_key is the idempotency key of the answer, set when it is given.
--   - indomitable_from and indomitable_to are the d20 that counted before and after the new roll;
--     both NULL when the player kept the roll. faces, counted and total then hold the new roll.
-- A Bardic Inspiration die answered while Indomitable follows is kept on the row, since the roll
-- is not closed yet: die_answer_key is the key of the die's answer (the die was kept or used) and
-- die_used the die the player added, as a JSON object with sides, face, from and expires_round.
--
-- Production is live: the columns only add and have defaults or allow NULL, and the checks are
-- dropped before they are added, so running the migration twice is safe.
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS indomitable_ask BOOL NOT NULL DEFAULT FALSE;
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS indomitable_failed BOOL NOT NULL DEFAULT FALSE;
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS indomitable_answer_key UUID NULL;
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS indomitable_from INT4 NULL;
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS indomitable_to INT4 NULL;
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS die_answer_key UUID NULL;
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS die_used JSONB NULL;

ALTER TABLE inspiration_holds DROP CONSTRAINT IF EXISTS inspiration_holds_indomitable_faces_valid;
ALTER TABLE inspiration_holds ADD CONSTRAINT inspiration_holds_indomitable_faces_valid CHECK (
    (indomitable_from IS NULL AND indomitable_to IS NULL)
    OR (indomitable_from BETWEEN 1 AND 20 AND indomitable_to BETWEEN 1 AND 20));

-- +goose Down
ALTER TABLE inspiration_holds DROP CONSTRAINT IF EXISTS inspiration_holds_indomitable_faces_valid;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS die_used;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS die_answer_key;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS indomitable_to;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS indomitable_from;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS indomitable_answer_key;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS indomitable_failed;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS indomitable_ask;
