-- +goose Up
-- A character drew or stowed a piece of gear in a running combat (`gear_swapped`): the payload holds the combatant, the
-- round, the piece, whether it was equipped before and the turn's economy before, which the master's undo puts back. The row
-- is secret: only the master and the character's owner read its line in the combat log.
INSERT INTO session_event_kinds (kind) VALUES
    ('gear_swapped')
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DELETE FROM session_events WHERE kind = 'gear_swapped';
DELETE FROM session_event_kinds WHERE kind = 'gear_swapped';
