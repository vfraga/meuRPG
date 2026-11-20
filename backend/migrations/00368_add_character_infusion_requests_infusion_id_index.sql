-- +goose Up
-- The ON DELETE CASCADE of the record.
CREATE INDEX IF NOT EXISTS character_infusion_requests_infusion_id_idx ON character_infusion_requests (infusion_id);

-- +goose Down
DROP INDEX IF EXISTS character_infusion_requests_infusion_id_idx;
