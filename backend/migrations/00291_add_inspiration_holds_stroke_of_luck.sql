-- +goose Up
-- A roll that missed or failed also waits for the rogue's Stroke of Luck (SRD 5.1, Rogue 20: "If your attack misses a
-- target within range, you can turn the miss into a hit. Alternatively, if you fail an ability check, you can treat
-- the d20 roll as a 20. Once you use this feature, you can't use it again until you finish a short or long rest.").
-- It comes after the halfling's Lucky and the Bardic Inspiration die (inspiration_holds, 00244, 00254), in the same
-- place as Indomitable's question (00290), which it never meets: a save is not an attack or an ability check.
--   - stroke_ask is true once the question was asked: the attack missed or the check failed on the roll as it stood
--     after Lucky and the die, or the roller may not know the result (the table hides the DC, an opposed check, the
--     dice an effect adds after the gate) and so the question comes on every eligible roll. It waits while it is true
--     and stroke_answer_key and answer_key are NULL.
--   - stroke_failed says the question may tell the roll missed or failed (the roller may know it); it is false when
--     the question comes on every roll, with the result unknown to the roller.
--   - stroke_answer_key is the idempotency key of the answer, set when it is given.
--   - stroke_from is the d20 that counted when the player used the feature; NULL when the player kept the roll. For a
--     check the faces hold the 20 then; an attack keeps its d20 and the path turns the miss into a hit.
-- A Bardic Inspiration die answered while the question follows is kept on the row with Indomitable's columns
-- (die_answer_key, die_used, 00290).
--
-- Production is live: the columns only add and have defaults or allow NULL, and the check is dropped before it is
-- added, so running the migration twice is safe.
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS stroke_ask BOOL NOT NULL DEFAULT FALSE;
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS stroke_failed BOOL NOT NULL DEFAULT FALSE;
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS stroke_answer_key UUID NULL;
ALTER TABLE inspiration_holds ADD COLUMN IF NOT EXISTS stroke_from INT4 NULL;

ALTER TABLE inspiration_holds DROP CONSTRAINT IF EXISTS inspiration_holds_stroke_face_valid;
ALTER TABLE inspiration_holds ADD CONSTRAINT inspiration_holds_stroke_face_valid CHECK (
    stroke_from IS NULL OR stroke_from BETWEEN 1 AND 20);

-- +goose Down
ALTER TABLE inspiration_holds DROP CONSTRAINT IF EXISTS inspiration_holds_stroke_face_valid;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS stroke_from;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS stroke_answer_key;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS stroke_failed;
ALTER TABLE inspiration_holds DROP COLUMN IF EXISTS stroke_ask;
