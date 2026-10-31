-- +goose Up
-- The kinds a session event may have, as rows instead of a CHECK list (audit D-02).
-- The CHECK `session_events_kind_valid` was dropped and added again in ten
-- migrations, and each time CockroachDB checked every row of the table that grows
-- fastest. With this table and a foreign key (00169), a new kind is one INSERT in
-- a migration, and no existing row is read. The Go code and this table are kept
-- equal by TestEveryEventKindOfTheCodeIsInTheTable. The kinds are reference data,
-- not game data: nothing deletes them, and the test databases keep them.
CREATE TABLE IF NOT EXISTS session_event_kinds (
    kind TEXT NOT NULL PRIMARY KEY
);

INSERT INTO session_event_kinds (kind) VALUES
    ('character_vitals_adjusted'),
    ('encounter_started'),
    ('initiative_submitted'),
    ('initiative_order_set'),
    ('combat_begun'),
    ('turn_ended'),
    ('combatant_moved'),
    ('combatant_hidden_set'),
    ('combatants_added'),
    ('combatant_removed'),
    ('encounter_ended'),
    ('attack_rolled'),
    ('damage_rolled'),
    ('damage_applied'),
    ('damage_discarded'),
    ('action_taken'),
    ('hit_points_adjusted'),
    ('action_undone'),
    ('spell_cast'),
    ('reaction_used'),
    ('reaction_declined'),
    ('death_save_rolled'),
    ('death_confirmed'),
    ('conditions_set'),
    ('xp_awarded'),
    ('xp_award_undone'),
    ('milestone_marked'),
    ('scene_opened'),
    ('scene_closed'),
    ('scene_check_rolled'),
    ('clue_revealed'),
    ('stage_changed'),
    ('scene_attempt_granted'),
    ('turn_part_ended'),
    ('trap_noticed'),
    ('trap_searched'),
    ('trap_triggered'),
    ('trap_disarmed'),
    ('trap_revealed'),
    ('treasure_found'),
    ('treasure_unfound'),
    ('cover_set'),
    ('side_set'),
    ('opportunity_offered'),
    ('creature_summoned'),
    ('creature_dismissed'),
    ('wild_shape_started'),
    ('wild_shape_ended'),
    ('familiar_sight'),
    ('door_opened'),
    ('puzzle_shown'),
    ('puzzle_solved'),
    ('puzzle_reset'),
    ('puzzle_closed')
ON CONFLICT (kind) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS session_event_kinds;
