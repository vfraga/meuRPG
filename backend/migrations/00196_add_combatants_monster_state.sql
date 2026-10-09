-- +goose Up
-- monster_state is what a monster of a combat has spent of its stat block, as a JSON
-- object (docs/data.md): the actions waiting for their recharge, the uses of the
-- limited actions and innate spells, the legendary actions and the Legendary
-- Resistance used, the spell slots used and the Multiattack routine of the turn.
-- It holds only what was spent: an empty object is a creature with everything
-- available. Only the master ever reads it (RN-10). At most 4 KiB.
--
-- One statement with several parts, so re-running it is safe (see 00036).
ALTER TABLE combatants
    ADD COLUMN IF NOT EXISTS monster_state JSONB NOT NULL DEFAULT '{}'::JSONB,
    DROP CONSTRAINT IF EXISTS combatants_monster_state_valid,
    ADD CONSTRAINT combatants_monster_state_valid CHECK (
        jsonb_typeof(monster_state) = 'object' AND length(monster_state::STRING) <= 4096
    );

-- The events of a monster's stat block (migrations 00168 and 00169 explain the table): the
-- recharge rolls at the start of its turn, a Legendary Resistance used on a failed save,
-- and a check the master rolled for it.
INSERT INTO session_event_kinds (kind) VALUES
    ('monster_recharge_rolled'),
    ('legendary_resistance_used'),
    ('combatant_check_rolled')
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DELETE FROM session_event_kinds
WHERE kind IN ('monster_recharge_rolled', 'legendary_resistance_used', 'combatant_check_rolled');

ALTER TABLE combatants
    DROP CONSTRAINT IF EXISTS combatants_monster_state_valid,
    DROP COLUMN IF EXISTS monster_state;
