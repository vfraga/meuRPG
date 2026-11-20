-- +goose Up
-- The reading of a Spell Scroll that passed (or needed no check) is a session event, so the key of a retry finds it and the cast of the
-- scroll's spell finds the pass. A reading that failed is a consumable_used event.
INSERT INTO session_event_kinds (kind) VALUES ('scroll_read')
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DELETE FROM session_events WHERE kind = 'scroll_read';
DELETE FROM session_event_kinds WHERE kind = 'scroll_read';
