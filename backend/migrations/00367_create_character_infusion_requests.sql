-- +goose Up
-- The answer of an InfuseItem or a MoveInfusion that carried an idempotency key: the key is scoped by the campaign, and
-- request_hash is the hash of the request, so the same key with another request is refused. infusion_id is the record the
-- call made or moved; ended_infusion_id is the record that ended in the same call, or NULL. A retry returns that answer and
-- writes nothing. The rows hold ids and a hash, no text, and go with the record or the campaign.
CREATE TABLE IF NOT EXISTS character_infusion_requests (
    campaign_id UUID NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    idempotency_key UUID NOT NULL,
    request_hash TEXT NOT NULL,
    infusion_id UUID NOT NULL REFERENCES character_infusions (id) ON DELETE CASCADE,
    ended_infusion_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (campaign_id, idempotency_key)
);

-- +goose Down
DROP TABLE IF EXISTS character_infusion_requests;
