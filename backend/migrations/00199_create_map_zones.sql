-- +goose Up
-- A zone is what a spell (or the master) leaves on the map for a while: a fog that
-- blocks sight, a web that holds, a wall of fire, spikes under the grass, a silence.
-- It lives in a combat, goes with the combat when it ends, and is the master's
-- unless visible_to_players says the players see it.
--
--   - spell_key is the spell that left it ("spell:web"), empty for a zone the master
--     put on the map himself; name is that zone's name (a scenario text the master
--     wrote, never a player's).
--   - caster_id is who cast it. A caster that leaves the combat leaves the zone
--     without one (the zone goes on until it ends, or the master ends it).
--   - shape is sphere, cube, cylinder, wall or ring. origin is the square it was set
--     on (NULL without a grid, where the master says who is inside: members);
--     dir_dx and dir_dy are the direction of a wall; size_ft is the radius, the side,
--     the length or the diameter, from the slot level; cells are the squares it covers
--     (col, row, col, row...) after the walls of the map cut it, row-major.
--   - obscurity is '' (none), 'light', 'heavy', 'dark' or 'opaque'; difficult and
--     halves_speed change what a move through it costs. camouflaged hides the zone, and
--     its cost, from a player who has not recognised it (known_by lists the combatants
--     that have).
--   - cast_round and duration_rounds (0: until the master ends it) are the clock; the
--     zone ends at the start of the caster's turn once the rounds have run, or at
--     disperse_round, when a wind was told to disperse it.
--   - moves_with is '', 'caster' or 'self_at_turn_start' (step_squares); caster_moves
--     says an action of the caster moves it (Moonbeam).
--   - triggers are the rules that act on a creature, as JSON; rules are the ones that
--     ask no roll. save_dc and the damage dice are the cast's own numbers.
--   - damage_side is the side of a wall that burns ('a' or 'b'), reach_squares how far it
--     reaches; ring_radius is a ring's radius in squares.
--   - ended_at is when it ended: an ended zone is kept so that the combat log can still name it,
--     and counts for nothing else. Ending the combat deletes them all.
--   - excluded_ids are the creatures the caster said the spell does not affect; members
--     are the combatants the master says are inside, in a combat without a map.
--
-- No personal data: ids, squares and numbers, and a name the master wrote.
CREATE TABLE IF NOT EXISTS map_zones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    encounter_id UUID NOT NULL REFERENCES encounters (id) ON DELETE CASCADE,
    seq INT4 NOT NULL,
    spell_key TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    caster_id UUID NULL REFERENCES combatants (id) ON DELETE SET NULL,
    shape TEXT NOT NULL,
    origin_col INT4 NULL,
    origin_row INT4 NULL,
    dir_dx INT2 NOT NULL DEFAULT 0,
    dir_dy INT2 NOT NULL DEFAULT 0,
    size_ft INT4 NOT NULL DEFAULT 0,
    ring_radius INT2 NOT NULL DEFAULT 0,
    cells INT4[] NOT NULL DEFAULT '{}',
    obscurity TEXT NOT NULL DEFAULT '',
    difficult BOOL NOT NULL DEFAULT false,
    halves_speed BOOL NOT NULL DEFAULT false,
    camouflaged BOOL NOT NULL DEFAULT false,
    visible_to_players BOOL NOT NULL DEFAULT true,
    concentration BOOL NOT NULL DEFAULT false,
    slot_level INT2 NOT NULL DEFAULT 0,
    cast_round INT4 NOT NULL DEFAULT 0,
    duration_rounds INT4 NOT NULL DEFAULT 0,
    disperse_round INT4 NULL,
    moves_with TEXT NOT NULL DEFAULT '',
    step_squares INT2 NOT NULL DEFAULT 0,
    caster_moves BOOL NOT NULL DEFAULT false,
    triggers JSONB NOT NULL DEFAULT '[]',
    rules TEXT[] NOT NULL DEFAULT '{}',
    save_dc INT4 NOT NULL DEFAULT 0,
    damage_count INT4 NOT NULL DEFAULT 0,
    damage_sides INT4 NOT NULL DEFAULT 0,
    damage_bonus INT4 NOT NULL DEFAULT 0,
    damage_type TEXT NOT NULL DEFAULT '',
    damage_side TEXT NOT NULL DEFAULT '',
    reach_squares INT2 NOT NULL DEFAULT 0,
    anchored BOOL NOT NULL DEFAULT true,
    excluded_ids TEXT[] NOT NULL DEFAULT '{}',
    known_by TEXT[] NOT NULL DEFAULT '{}',
    members TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NULL,
    CONSTRAINT map_zones_encounter_id_seq_key UNIQUE (encounter_id, seq),
    CONSTRAINT map_zones_shape_valid CHECK (shape IN ('sphere', 'cube', 'cylinder', 'wall', 'ring')),
    CONSTRAINT map_zones_obscurity_valid CHECK (obscurity IN ('', 'light', 'heavy', 'dark', 'opaque')),
    CONSTRAINT map_zones_moves_with_valid CHECK (moves_with IN ('', 'caster', 'self_at_turn_start')),
    CONSTRAINT map_zones_damage_side_valid CHECK (damage_side IN ('', 'a', 'b')),
    CONSTRAINT map_zones_origin_valid CHECK ((origin_col IS NULL) = (origin_row IS NULL) AND (origin_col IS NULL OR (origin_col >= 0 AND origin_row >= 0))),
    CONSTRAINT map_zones_cells_valid CHECK (cardinality(cells) % 2 = 0 AND cardinality(cells) <= 8400),
    CONSTRAINT map_zones_clock_valid CHECK (cast_round >= 0 AND duration_rounds >= 0 AND (disperse_round IS NULL OR disperse_round >= 0)),
    CONSTRAINT map_zones_sizes_valid CHECK (size_ft >= 0 AND size_ft <= 400 AND step_squares >= 0 AND reach_squares >= 0 AND save_dc >= 0 AND damage_count >= 0 AND damage_sides >= 0),
    CONSTRAINT map_zones_lists_valid CHECK (cardinality(excluded_ids) <= 40 AND cardinality(known_by) <= 40 AND cardinality(members) <= 40 AND cardinality(rules) <= 8),
    CONSTRAINT map_zones_name_valid CHECK (char_length(name) <= 80 AND char_length(spell_key) <= 100),
    CONSTRAINT map_zones_triggers_valid CHECK (jsonb_typeof(triggers) = 'array' AND jsonb_array_length(triggers) <= 8)
);

-- The ledger of who a zone already hurt in a turn: "on a turn" is the turn of the
-- combatant whose turn it is (turn_of), so a creature is hurt at most once for each
-- zone and each turn, entering it and starting its turn there together.
CREATE TABLE IF NOT EXISTS map_zone_fired (
    zone_id UUID NOT NULL REFERENCES map_zones (id) ON DELETE CASCADE,
    combatant_id UUID NOT NULL REFERENCES combatants (id) ON DELETE CASCADE,
    round INT4 NOT NULL,
    turn_of UUID NOT NULL,
    PRIMARY KEY (zone_id, combatant_id, round, turn_of)
);

-- What a zone put on a creature, so that it takes only that off when the creature
-- leaves or the zone ends: a condition the master set by hand stays.
CREATE TABLE IF NOT EXISTS map_zone_effects (
    zone_id UUID NOT NULL REFERENCES map_zones (id) ON DELETE CASCADE,
    combatant_id UUID NOT NULL REFERENCES combatants (id) ON DELETE CASCADE,
    condition TEXT NOT NULL,
    PRIMARY KEY (zone_id, combatant_id, condition)
);

-- A saving throw a zone asks of a creature, waiting for its answer: the creature's
-- player rolls it (or the master, for an NPC), and the turn waits meanwhile. It is
-- the zone's reaction window; zone_id is lost when the zone ends, and the window is
-- closed with the reason 'zone_ended'.
--
--   - trigger_kind is the zone's trigger that opened it; round and turn_of say whose
--     turn it was.
--   - ability, dc and the damage are the cast's numbers when the window opened; cover_bonus
--     is the cover the creature had against the point of origin (it counts for a Dexterity save).
--   - state is 'open', 'answered' or 'closed'.
CREATE TABLE IF NOT EXISTS zone_save_windows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    encounter_id UUID NOT NULL REFERENCES encounters (id) ON DELETE CASCADE,
    zone_id UUID NULL REFERENCES map_zones (id) ON DELETE SET NULL,
    reactor_id UUID NOT NULL REFERENCES combatants (id) ON DELETE CASCADE,
    caster_id UUID NULL REFERENCES combatants (id) ON DELETE SET NULL,
    seq INT4 NOT NULL,
    trigger_kind TEXT NOT NULL,
    round INT4 NOT NULL,
    turn_of UUID NULL,
    spell_key TEXT NOT NULL DEFAULT '',
    ability TEXT NOT NULL,
    dc INT4 NOT NULL,
    damage_count INT4 NOT NULL DEFAULT 0,
    damage_sides INT4 NOT NULL DEFAULT 0,
    damage_bonus INT4 NOT NULL DEFAULT 0,
    damage_type TEXT NOT NULL DEFAULT '',
    on_success TEXT NOT NULL DEFAULT 'none',
    on_fail TEXT NOT NULL DEFAULT '',
    cover_bonus INT4 NOT NULL DEFAULT 0,
    state TEXT NOT NULL DEFAULT 'open',
    close_reason TEXT NOT NULL DEFAULT '',
    d20 INT4 NULL,
    modifier INT4 NULL,
    total INT4 NULL,
    saved BOOL NULL,
    physical BOOL NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL,
    answered_at TIMESTAMPTZ NULL,
    CONSTRAINT zone_save_windows_encounter_id_seq_key UNIQUE (encounter_id, seq),
    CONSTRAINT zone_save_windows_state_valid CHECK (state IN ('open', 'answered', 'closed')),
    CONSTRAINT zone_save_windows_ability_valid CHECK (ability IN ('str', 'dex', 'con', 'int', 'wis', 'cha')),
    CONSTRAINT zone_save_windows_on_success_valid CHECK (on_success IN ('none', 'half')),
    CONSTRAINT zone_save_windows_numbers_valid CHECK (dc >= 0 AND damage_count >= 0 AND damage_sides >= 0)
);

-- The events of a zone: put, moved, ended, a creature caught by it, the answer of a saving
-- throw it asked, and the master's edits that are no line of the log (map_zone_changed).
INSERT INTO session_event_kinds (kind) VALUES
    ('map_zone_added'), ('map_zone_moved'), ('map_zone_ended'), ('map_zone_triggered'), ('map_zone_changed'), ('zone_save_answered')
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS zone_save_windows;
DROP TABLE IF EXISTS map_zone_effects;
DROP TABLE IF EXISTS map_zone_fired;
DROP TABLE IF EXISTS map_zones;
DELETE FROM session_event_kinds WHERE kind IN ('map_zone_added', 'map_zone_moved', 'map_zone_ended', 'map_zone_triggered', 'map_zone_changed', 'zone_save_answered');
