-- +goose Up
-- A persistent NPC with a stat block (a character of kind NPC that is not combat_only) keeps what it spent of its
-- uses a day and its Legendary Resistance until the table's long rest (SRD 5.1, "Resting"), as it keeps its spell
-- slots (slots_used, 00298). creature_spent holds the combatant's keys for it as JSON {"<key>": N}: "use:<action key>",
-- "rest:<action key>" (an action that recharges after a short or long rest) and "legendary_resistance". Only what was spent is kept, a key that is absent is 0.
-- The Recharge state ("recharge:<action key>") and the legendary actions of a round are never kept: a new combat starts
-- with every Recharge action available. A monster of a combat (the combat_only NPC the app keeps for a creature) never
-- has a row. The highest count any copy of the NPC holds in a combat is kept, and the long rest deletes the row and a short rest removes its "rest:" keys.
-- Keys, numbers and nothing else; no personal data. Its text is at most 4 KiB, as the combatant's.
--
-- Production is live: the column only adds, with every row as it was ('{}'), so running the migration twice is safe.
ALTER TABLE npc_spell_use
    ADD COLUMN IF NOT EXISTS creature_spent JSONB NOT NULL DEFAULT '{}',
    DROP CONSTRAINT IF EXISTS npc_spell_use_creature_spent_valid,
    ADD CONSTRAINT npc_spell_use_creature_spent_valid CHECK (jsonb_typeof(creature_spent) = 'object' AND octet_length(creature_spent::TEXT) <= 4096);

-- +goose Down
ALTER TABLE npc_spell_use
    DROP CONSTRAINT IF EXISTS npc_spell_use_creature_spent_valid,
    DROP COLUMN IF EXISTS creature_spent;
