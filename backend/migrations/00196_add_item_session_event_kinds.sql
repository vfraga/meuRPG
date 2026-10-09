-- +goose Up
-- The kinds of the inventory's history (package characters writes them through
-- AppendEvent, like the creatures'): an item the master gave (item_given), one handed
-- from a character to another (item_transferred), a potion drunk or a scroll read
-- (item_used), an attunement made or ended (item_attuned), an item identified
-- (item_identified), and the charges spent or regained (item_charges). The payload
-- carries IDs and numbers only; the master's log names the items from the sheets.
INSERT INTO session_event_kinds (kind) VALUES
    ('item_given'),
    ('item_transferred'),
    ('item_used'),
    ('item_attuned'),
    ('item_identified'),
    ('item_charges')
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DELETE FROM session_events WHERE kind IN ('item_given', 'item_transferred', 'item_used', 'item_attuned', 'item_identified', 'item_charges');
DELETE FROM session_event_kinds WHERE kind IN ('item_given', 'item_transferred', 'item_used', 'item_attuned', 'item_identified', 'item_charges');
