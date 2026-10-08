-- +goose Up
-- The idempotency key (scoped to the campaign) and the request hash of the call that applied or
-- discarded a trap damage. A damage outlives its session, so the master may settle it with no
-- session open, where there is no session event to carry the key: a retry of a call whose answer
-- was lost then finds the key here and gets the first answer, and the same key for another damage,
-- another amount or the other action is refused (architecture, rule 9). NULL while the damage
-- waits, and on rows settled before the column existed. A key and a hash are opaque values, not
-- personal data.
ALTER TABLE trap_damages ADD COLUMN IF NOT EXISTS settle_key TEXT NULL;
ALTER TABLE trap_damages ADD COLUMN IF NOT EXISTS settle_hash TEXT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS trap_damages_settle_key_idx ON trap_damages (settle_key) WHERE settle_key IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS trap_damages_settle_key_idx;
ALTER TABLE trap_damages DROP COLUMN IF EXISTS settle_hash;
ALTER TABLE trap_damages DROP COLUMN IF EXISTS settle_key;
