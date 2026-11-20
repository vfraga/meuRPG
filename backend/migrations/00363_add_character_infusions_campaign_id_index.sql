-- +goose Up
-- The ON DELETE CASCADE of the campaign.
CREATE INDEX IF NOT EXISTS character_infusions_campaign_id_idx ON character_infusions (campaign_id);

-- +goose Down
DROP INDEX IF EXISTS character_infusions_campaign_id_idx;
