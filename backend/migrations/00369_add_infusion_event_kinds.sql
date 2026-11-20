-- +goose Up
-- The infusions are session events, so the key of a retry finds what it did.
INSERT INTO session_event_kinds (kind) VALUES
    ('infusion_infused'),
    ('infusion_moved'),
    ('infusion_ended')
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DELETE FROM session_events WHERE kind IN ('infusion_infused', 'infusion_moved', 'infusion_ended');
DELETE FROM session_event_kinds WHERE kind IN ('infusion_infused', 'infusion_moved', 'infusion_ended');
