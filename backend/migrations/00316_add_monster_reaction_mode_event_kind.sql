-- +goose Up
-- The master chose how a monster's Parry is decided (`monster_reaction_mode_set`): the server parries on its own, or
-- asks the master in a window. The payload holds the combatant id, the round and the mode; the row is secret and
-- has no line in the combat log.
INSERT INTO session_event_kinds (kind) VALUES
    ('monster_reaction_mode_set')
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DELETE FROM session_events WHERE kind = 'monster_reaction_mode_set';
DELETE FROM session_event_kinds WHERE kind = 'monster_reaction_mode_set';
