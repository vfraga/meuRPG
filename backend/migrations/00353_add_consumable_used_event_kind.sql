-- +goose Up
-- Using up a consumable (a potion, a scroll) is a session event of its own, so the key of a retry finds the use that was made.
-- What a potion of resistance leaves is an effect of the item that was used, so the card of the effect names the potion and
-- not the master: the kind of an effect's source gets a value, 'item', on a combatant (combatant_states_effect_valid) and on a
-- character out of a combat (character_effects). The CHECK on character_effects was written inline, so it has the name its
-- engine gave it (check_source_kind on CockroachDB, character_effects_source_kind_check on PostgreSQL); both are dropped when
-- present. The widened CHECKs only add a value, so every row stays valid.
INSERT INTO session_event_kinds (kind) VALUES ('consumable_used')
ON CONFLICT (kind) DO NOTHING;
ALTER TABLE character_effects DROP CONSTRAINT IF EXISTS check_source_kind;
ALTER TABLE character_effects DROP CONSTRAINT IF EXISTS character_effects_source_kind_check;
ALTER TABLE character_effects DROP CONSTRAINT IF EXISTS character_effects_source_kind_valid;
ALTER TABLE character_effects ADD CONSTRAINT character_effects_source_kind_valid
    CHECK (source_kind IN ('spell', 'feature', 'master', 'item'));
ALTER TABLE combatant_states
    DROP CONSTRAINT IF EXISTS combatant_states_effect_valid,
    ADD CONSTRAINT combatant_states_effect_valid CHECK (
        kind <> 'effect'
        OR (group_id IS NOT NULL AND source_key IS NOT NULL AND source_kind IN ('spell', 'feature', 'master', 'item')
            AND duration_kind IN ('rounds', 'until_start_of_turn_of', 'until_end_of_turn_of', 'concentration', 'until_dismissed', 'long_rest'))
    );

-- +goose Down
DELETE FROM session_events WHERE kind = 'consumable_used';
DELETE FROM session_event_kinds WHERE kind = 'consumable_used';
DELETE FROM combatant_states WHERE kind = 'effect' AND source_kind = 'item';
DELETE FROM character_effects WHERE source_kind = 'item';
ALTER TABLE combatant_states
    DROP CONSTRAINT IF EXISTS combatant_states_effect_valid,
    ADD CONSTRAINT combatant_states_effect_valid CHECK (
        kind <> 'effect'
        OR (group_id IS NOT NULL AND source_key IS NOT NULL AND source_kind IN ('spell', 'feature', 'master')
            AND duration_kind IN ('rounds', 'until_start_of_turn_of', 'until_end_of_turn_of', 'concentration', 'until_dismissed', 'long_rest'))
    );
ALTER TABLE character_effects DROP CONSTRAINT IF EXISTS character_effects_source_kind_valid;
ALTER TABLE character_effects ADD CONSTRAINT character_effects_source_kind_valid
    CHECK (source_kind IN ('spell', 'feature', 'master'));
