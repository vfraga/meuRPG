-- +goose Up
-- The hash of the whole request (without its key) kept next to an idempotency key, so a key
-- reused for another request is told from a retry (architecture, rule 9): the combat's and the
-- session's changes (session_events), the XP awards (xp_awards) and the generated images
-- (image_requests). NULL on rows made before the column existed or without a key: such a row is
-- replayed as it always was. A hash is an opaque value, not personal data.
ALTER TABLE session_events ADD COLUMN IF NOT EXISTS idempotency_hash TEXT NULL;
ALTER TABLE xp_awards ADD COLUMN IF NOT EXISTS idempotency_hash TEXT NULL;
ALTER TABLE image_requests ADD COLUMN IF NOT EXISTS idempotency_hash TEXT NULL;

-- +goose Down
ALTER TABLE image_requests DROP COLUMN IF EXISTS idempotency_hash;
ALTER TABLE xp_awards DROP COLUMN IF EXISTS idempotency_hash;
ALTER TABLE session_events DROP COLUMN IF EXISTS idempotency_hash;
