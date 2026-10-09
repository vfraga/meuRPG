-- name: GetOpenGameSession :one
-- The campaign's open session, if any. The partial unique index
-- game_sessions_one_open_per_campaign allows at most one.
SELECT * FROM game_sessions
WHERE campaign_id = $1 AND ended_at IS NULL;

-- name: NextSessionNumber :one
-- Sessions count from 1. Two starts racing both read the same number;
-- CockroachDB's SERIALIZABLE isolation makes one of them retry, and the
-- UNIQUE (campaign_id, session_number) constraint is the backstop.
SELECT (COALESCE(max(session_number), 0) + 1)::INT4 AS next
FROM game_sessions
WHERE campaign_id = $1;

-- name: InsertGameSession :one
-- create_key and create_hash are the idempotency key of StartGameSession and the hash of its
-- request (NULL when the call sent no key); the caller reads the first row with
-- GetGameSessionByCreateKey.
INSERT INTO game_sessions (campaign_id, session_number, started_at, create_key, create_hash)
VALUES (sqlc.arg(campaign_id), sqlc.arg(session_number), sqlc.arg(started_at), sqlc.narg(create_key), sqlc.narg(create_hash))
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetGameSessionByCreateKey :one
-- The session a StartGameSession with this idempotency key started, if any (the key carries the
-- campaign's ID), open or ended.
SELECT * FROM game_sessions WHERE create_key = $1;

-- name: EndGameSession :one
-- Ending a session twice keeps the first ended_at, so the call is
-- idempotent. GREATEST keeps ended_at from being before started_at if two
-- servers' clocks disagree by a little (game_sessions_ends_after_start).
UPDATE game_sessions
SET ended_at = COALESCE(ended_at, GREATEST(sqlc.arg(now)::TIMESTAMPTZ, started_at))
WHERE campaign_id = $1 AND id = $2
RETURNING *;

-- name: ListGameSessions :many
-- Newest first.
SELECT * FROM game_sessions
WHERE campaign_id = $1
ORDER BY session_number DESC;

-- name: GetOpenGameSessionForUpdate :one
-- GetOpenGameSession, locking the session's row until the transaction ends.
-- Every change made during a session locks it first, so two changes at
-- once take turns: each one reads the next event number after the other
-- wrote its own, and EndGameSession waits for a change in progress.
SELECT * FROM game_sessions
WHERE campaign_id = $1 AND ended_at IS NULL
FOR UPDATE;

-- name: GetGameSessionForUpdate :one
-- One session of the campaign, locked until the transaction ends.
SELECT * FROM game_sessions
WHERE campaign_id = $1 AND id = $2
FOR UPDATE;

-- name: ListOpenGameSessions :many
-- The open sessions of the given campaigns, newest first (RN-06). The
-- partial unique index game_sessions_one_open_per_campaign finds each one.
SELECT * FROM game_sessions
WHERE campaign_id = ANY(sqlc.arg(campaign_ids)::UUID[]) AND ended_at IS NULL
ORDER BY started_at DESC, id;

-- name: GetSessionEventByIdempotencyKey :one
-- The event a change with this key already wrote, if any, with the hash of the request that
-- wrote it (NULL on an event made without one).
SELECT id, seq, kind, actor_user_id, character_id, payload, created_at, idempotency_hash FROM session_events
WHERE game_session_id = $1 AND idempotency_key = $2;

-- name: NextSessionEventSeq :one
-- Events count from 1 in each session. The caller holds the session's row
-- lock (GetOpenGameSessionForUpdate), so no other change reads the same
-- number; UNIQUE (game_session_id, seq) is the backstop.
SELECT (COALESCE(max(seq), 0) + 1)::INT4 AS next
FROM session_events
WHERE game_session_id = $1;

-- name: InsertSessionEvent :one
INSERT INTO session_events
    (game_session_id, seq, kind, actor_user_id, character_id, payload, idempotency_key, created_at, encounter_id, idempotency_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, sqlc.narg(idempotency_hash))
RETURNING id, seq;

-- name: GetOnScreen :one
-- What the open session shows: its current map and the image the master
-- shows (either NULL when none). No row: no open session.
SELECT current_map_id, shown_image_id FROM game_sessions
WHERE campaign_id = $1 AND ended_at IS NULL;

-- name: SetCurrentMap :one
-- The caller holds the session's row lock (GetOpenGameSessionForUpdate).
UPDATE game_sessions
SET current_map_id = sqlc.narg(current_map_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetShownImage :one
-- The caller holds the session's row lock (GetOpenGameSessionForUpdate).
UPDATE game_sessions
SET shown_image_id = sqlc.narg(shown_image_id), shown_image_keep = sqlc.arg(shown_image_keep)
WHERE id = sqlc.arg(id)
RETURNING *;

-- Combat (MR-013). Every write below runs after the caller locked the open
-- session's row (GetOpenGameSessionForUpdate), so two changes to a combat take
-- turns, as for the vitals.

-- name: GetLatestEncounter :one
-- The session's latest combat, ended or not: GetEncounter shows it, so the app
-- can also show the end of a combat that just ended. A session has at most one open
-- combat (encounters_one_open_per_session), and it is the latest whatever the clock that
-- stamped it said; the ended ones follow by the time they were made.
SELECT * FROM encounters
WHERE game_session_id = $1
ORDER BY (status <> 'ended') DESC, created_at DESC, id DESC
LIMIT 1;

-- name: GetOpenEncounter :one
-- The session's combat that is not ended, if any. The partial unique index
-- encounters_one_open_per_session allows at most one.
SELECT * FROM encounters
WHERE game_session_id = $1 AND status <> 'ended';

-- name: GetEncounterInSession :one
-- A combat by its ID, if it is in the session (so a combat of another
-- campaign matches no row).
SELECT * FROM encounters
WHERE game_session_id = $1 AND id = $2;

-- name: InsertEncounter :one
-- A new combat starts in setup, in round 0. The mode ('grid' or 'theatre') never
-- changes afterwards; a combat without a grid has no map and a grid of 0 by 0.
INSERT INTO encounters (game_session_id, map_id, map_point_id, name, status, grid_columns, grid_rows, created_at, mode)
VALUES ($1, $2, $3, $4, 'setup', $5, $6, $7, $8)
RETURNING *;

-- name: SetEncounterState :one
-- Where the combat is: its status, round and whose turn it is. Every change
-- to a combat raises its revision, so a write that changes only combatants
-- uses TouchEncounter.
UPDATE encounters
SET status = sqlc.arg(status), round = sqlc.arg(round), current_combatant_id = sqlc.narg(current_combatant_id),
    started_at = sqlc.narg(started_at), ended_at = sqlc.narg(ended_at), revision = revision + 1
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: TouchEncounter :one
UPDATE encounters
SET revision = revision + 1
WHERE id = $1
RETURNING *;

-- name: ListCombatants :many
-- The combat's combatants in turn order. A creature whose concentration ended
-- is dismissed: it is out of the order and the map until an undo brings it back.
SELECT * FROM combatants
WHERE encounter_id = $1 AND NOT dismissed
ORDER BY order_index, created_at, id;

-- name: ListCombatantsWithDismissed :many
-- The same with the dismissed ones too: the combat log names who they were.
SELECT * FROM combatants
WHERE encounter_id = $1
ORDER BY order_index, created_at, id;

-- name: InsertCombatant :one
INSERT INTO combatants (
    encounter_id, character_id, user_id, label, kind, hidden, initiative, initiative_bonus, initiative_face,
    order_index, grid_col, grid_row, speed_ft, hp_current, hp_max, hp_temp, created_at, xp_value,
    side, size, speed_fly_ft, jump_long_dft, jump_high_dft
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15, $16, $17, $18,
    $19, $20, $21, $22, $23
)
RETURNING *;

-- name: SetCombatantInitiative :exec
-- A new roll breaks any tie order decided before (tie_ordered).
UPDATE combatants
SET initiative = $2, initiative_face = $3, tie_ordered = false
WHERE id = $1;

-- name: SetCombatantOrder :exec
UPDATE combatants
SET order_index = $2, tie_ordered = $3
WHERE id = $1;

-- name: SetCombatantMove :exec
-- A move, or its undo: the square (NULL when the combatant was not on the map),
-- the movement walked (in feet, rounded down, for the shipped web, and in tenths
-- of a foot, which is the truth), the length of the last move on foot (the
-- running start of a jump) and the master's cover mark, which a move clears.
UPDATE combatants
SET grid_col = $2, grid_row = $3, movement_used_ft = $4, movement_used_dft = $5, last_move_dft = $6, cover_mark = $7
WHERE id = $1;

-- name: SetCombatantRun :exec
-- The length of the last run of moves on foot this turn (the running start of a
-- jump): 0 when an action, an attack, a spell or a jump breaks it, or its undo.
UPDATE combatants
SET last_move_dft = $2
WHERE id = $1;

-- name: SetCombatantSide :exec
-- Whose side the combatant fights on ('party' or 'enemy'), the master's "Aliado".
UPDATE combatants
SET side = $2
WHERE id = $1;

-- name: SetCombatantCoverMark :exec
-- The cover the master marked on the combatant ('none', 'half', 'three_quarters'
-- or 'total').
UPDATE combatants
SET cover_mark = $2
WHERE id = $1;

-- name: SetCombatantHidden :exec
UPDATE combatants
SET hidden = $2
WHERE id = $1;

-- name: ResetCombatantTurn :exec
-- The start of a combatant's own turn (every living member of a joint turn
-- gets it): movement, action, bonus action, dash, reaction and the attacks
-- made come back, the Escudo bonus ends, a death save is due again, and the
-- combatant acts ('acting') in the turn that starts.
UPDATE combatants
SET movement_used_ft = 0, movement_used_dft = 0, last_move_dft = 0, dashed = false, disengaged = false, action_surged = false, spell_cast = false, bonus_spell_cast = false, action_used = false, bonus_action_used = false, reaction_used = false,
    attacks_made = 0, action_attack_key = NULL, bonus_attacks_left = 0, ac_bonus = 0, death_save_rolled = false, turn_state = 'acting'
WHERE id = $1;

-- name: ClearCombatTurns :exec
-- Nobody is on turn: every combatant of the combat goes back to 'idle'. A new
-- turn starts with this and then ResetCombatantTurn for each of its members.
UPDATE combatants
SET turn_state = 'idle'
WHERE encounter_id = $1 AND turn_state <> 'idle';

-- name: EndCombatantTurnPart :exec
-- A member of the joint turn ended its part: it acts no more until its group's
-- next turn.
UPDATE combatants
SET turn_state = 'ended'
WHERE id = $1;

-- name: MarkCombatantDashed :exec
UPDATE combatants
SET dashed = true
WHERE id = $1;

-- name: SetCombatantDisengaged :exec
-- The Disengage action of this turn (true), or its undo (false).
UPDATE combatants
SET disengaged = $2
WHERE id = $1;

-- name: SetCombatantActionSurged :exec
-- Action Surge was used in this turn (true), or its undo (false).
UPDATE combatants
SET action_surged = $2
WHERE id = $1;

-- name: DeleteCombatant :exec
DELETE FROM combatants
WHERE id = $1;

-- name: SetCombatantBody :exec
-- A character's numbers as a combatant change with its Wild Shape form (MR-037):
-- the beast's speed, fly speed, size and jumps while it lasts, the character's own
-- again when it ends.
UPDATE combatants
SET speed_ft = $2, speed_fly_ft = $3, size = $4, jump_long_dft = $5, jump_high_dft = $6
WHERE id = $1;

-- name: SetCombatantBodyOfCharacter :exec
-- SetCombatantBody for a player's character, found by the character, in the session's
-- combat that is not ended: the form ended where only the vitals were at hand (the
-- druid fell to 0 hit points).
UPDATE combatants
SET speed_ft = sqlc.arg(speed_ft), speed_fly_ft = sqlc.arg(speed_fly_ft), size = sqlc.arg(size),
    jump_long_dft = sqlc.arg(jump_long_dft), jump_high_dft = sqlc.arg(jump_high_dft)
WHERE character_id = sqlc.arg(character_id) AND kind = 'player'
  AND encounter_id IN (SELECT e.id FROM encounters AS e WHERE e.game_session_id = sqlc.arg(game_session_id) AND e.status <> 'ended');

-- name: SetCombatantHitPoints :exec
-- An NPC's hit points, temporary hit points and defeated flag (damage, healing,
-- the master's hand, an undo).
UPDATE combatants
SET hp_current = $2, hp_temp = $3, defeated = $4
WHERE id = $1;

-- name: SetCombatantHitPointsMax :exec
-- An NPC's maximum hit points, when a spell raises them (Ajuda) or its undo
-- puts them back.
UPDATE combatants
SET hp_max = $2
WHERE id = $1;

-- name: SetCombatantEconomy :exec
-- The turn's economy as an action, or its undo, leaves it.
UPDATE combatants
SET action_used = $2, bonus_action_used = $3, reaction_used = $4, dashed = $5
WHERE id = $1;

-- name: SetCombatantSpellsCast :exec
-- Which kinds of spell the combatant cast this turn (the bonus action spell
-- limit), or the cast's undo.
UPDATE combatants
SET spell_cast = $2, bonus_spell_cast = $3
WHERE id = $1;

-- name: SetCombatantAttacksMade :exec
-- The attacks the Attack action made this turn (Extra Attack), or its undo.
UPDATE combatants
SET attacks_made = $2
WHERE id = $1;

-- name: SetCombatantAttackState :exec
-- The attack of the action this turn and the Flurry of Blows strikes left, or
-- their undo.
UPDATE combatants
SET action_attack_key = $2, bonus_attacks_left = $3
WHERE id = $1;

-- name: SetCombatantAcBonus :exec
-- Escudo's +5 until the start of the combatant's next turn, or its undo.
UPDATE combatants
SET ac_bonus = $2
WHERE id = $1;

-- name: SetCombatantConcentration :exec
-- The spell the combatant concentrates on, or NULL when it stops (RN-22).
UPDATE combatants
SET concentration_spell = $2
WHERE id = $1;

-- name: SetCombatantConditions :exec
-- The condition labels the master marked (RN-22).
UPDATE combatants
SET conditions = $2
WHERE id = $1;

-- name: SetCombatantDeathSaves :exec
-- The death save counts, whether the turn's save was rolled, and whether the
-- combatant is out of the fight (a death the master confirmed), or their undo.
UPDATE combatants
SET death_successes = $2, death_failures = $3, death_save_rolled = $4, defeated = $5
WHERE id = $1;

-- name: ResetDeathSavesOfCharacter :exec
-- Healing above 0 resets both counts (RN-03): the character's combatant in the
-- session's combat that is not ended. An ended combat keeps its counts for the
-- summary.
UPDATE combatants
SET death_successes = 0, death_failures = 0
WHERE character_id = sqlc.arg(character_id)
  AND kind = 'player'
  AND encounter_id IN (SELECT id FROM encounters WHERE game_session_id = sqlc.arg(game_session_id) AND status <> 'ended');

-- name: MarkDeathSaveRolledOnTurn :exec
-- A character that drops to 0 hit points during its own turn owes no death save
-- until its next turn starts (SRD 5.1: the save is rolled at the start of the
-- turn), so this turn's save counts as done. Only a combatant who acts in the
-- turn, in the session's active combat.
UPDATE combatants
SET death_save_rolled = true
WHERE character_id = sqlc.arg(character_id)
  AND kind = 'player'
  AND turn_state = 'acting'
  AND encounter_id IN (
      SELECT id FROM encounters
      WHERE game_session_id = sqlc.arg(game_session_id) AND status = 'active'
  );

-- Pending damage (MR-012, MR-014): the damage of an attack that hit. Every write
-- below runs after the caller locked the open session's row.

-- name: InsertPendingDamage :one
-- A hit's damage waits for its roll, or, for a hit on a player's character that
-- may cast Escudo, for the target's reaction (attack_total is then kept for the
-- new comparison). A spell's damages carry their cast_id, and may be a heal or
-- a half damage. critical_max is what a critical hit adds without rolling (the
-- table's rule "máximo mais uma rolagem", RN-24).
INSERT INTO pending_damages (
    encounter_id, attacker_id, target_id, attack_key, status, critical,
    dice_count, dice_sides, dice_bonus, damage_type, created_at,
    cast_id, healing, half, attack_total, attack_armor_class, critical_max, critical_max_rule
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING *;

-- name: GetPendingDamage :one
-- A pending damage by its ID, if it is in the combat.
SELECT * FROM pending_damages
WHERE encounter_id = $1 AND id = $2;

-- name: ListOpenPendingDamages :many
-- What still waits for a reaction, to be rolled or applied in the combat,
-- oldest first.
SELECT * FROM pending_damages
WHERE encounter_id = $1 AND status IN ('awaiting_reaction', 'awaiting_roll', 'rolled')
ORDER BY created_at, id;

-- name: ListCastPendingDamages :many
-- The pending damages of one spell cast, oldest first.
SELECT * FROM pending_damages
WHERE encounter_id = $1 AND cast_id = $2
ORDER BY created_at, id;

-- name: SetPendingDamageRolled :one
-- The roll of a pending damage: 'rolled' for a player's character (waits for
-- the master), 'applied' for an NPC. amount is what lands, roll_total the roll
-- before a half damage halves it.
UPDATE pending_damages
SET status = $2, faces = $3, physical = $4, amount = $5, resolved_at = $6, roll_total = $7
WHERE id = $1
RETURNING *;

-- name: SetPendingDamageTaken :exec
-- What a damage that landed cost its target after temporary hit points.
UPDATE pending_damages
SET taken = $2
WHERE id = $1;

-- name: SetPendingDamageStatus :one
-- Applied or discarded by the master, a reaction's answer, or back to where it
-- was (an undo).
UPDATE pending_damages
SET status = $2, resolved_at = $3
WHERE id = $1
RETURNING *;

-- name: SetPendingDamageApplied :one
-- Applied by the master, with the amount when it is not the rolled one.
UPDATE pending_damages
SET status = 'applied', resolved_at = $2, applied_amount = $3
WHERE id = $1
RETURNING *;

-- name: ClearPendingDamageApplied :one
-- An undo of an applied damage: back to waiting for the master.
UPDATE pending_damages
SET status = 'rolled', resolved_at = NULL, applied_amount = NULL, taken = NULL
WHERE id = $1
RETURNING *;

-- name: ClearPendingDamageRoll :one
-- An undo of the damage roll: it waits to be rolled again.
UPDATE pending_damages
SET status = 'awaiting_roll', faces = '{}', physical = false, amount = NULL, resolved_at = NULL, roll_total = NULL, taken = NULL
WHERE id = $1
RETURNING *;

-- name: DeletePendingDamage :exec
DELETE FROM pending_damages
WHERE id = $1;

-- The combat log and the undo read the session's events (ADR-0007).

-- name: ListEncounterEvents :many
-- A combat's latest events, newest first: when a combat is longer than the
-- limit, it is the oldest lines that fall off the log, never the newest (or the
-- one an undo would take back). The caller reverses them.
SELECT id, seq, kind, actor_user_id, payload, created_at FROM session_events
WHERE encounter_id = $1
ORDER BY seq DESC
LIMIT $2;

-- name: ListRecentSessionEvents :many
-- The session's latest events, newest first (the undo looks for the last action).
SELECT id, seq, kind, encounter_id, payload FROM session_events
WHERE game_session_id = $1
ORDER BY seq DESC
LIMIT $2;

-- The RP scene open in the session (MR-015, D7). The caller holds the
-- session's row lock (GetOpenGameSessionForUpdate) for the writes.

-- name: SetOpenScene :one
UPDATE game_sessions
SET open_scene_point_id = sqlc.narg(open_scene_point_id)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: GetOpenSceneEvent :one
-- The event that opened the scene now open: the session's latest scene_opened.
-- Rolls after it belong to this opening; closing and opening again starts a new
-- one, so the players may roll again (question 55).
SELECT id, seq, created_at FROM session_events
WHERE game_session_id = $1 AND kind = 'scene_opened'
ORDER BY seq DESC
LIMIT 1;

-- name: ListSceneRollEvents :many
-- The scene checks rolled since the opening (seq), newest first.
SELECT id, seq, character_id, payload, created_at FROM session_events
WHERE game_session_id = $1 AND kind = 'scene_check_rolled' AND seq > $2
ORDER BY seq DESC;

-- name: CountSceneRolls :many
-- How many scene checks each character rolled at each action since the
-- opening (seq): what the attempts left are counted from, without reading the
-- rows.
SELECT character_id, COALESCE(payload->>'action_id', '')::TEXT AS action_id, count(*)::INT8 AS rolls
FROM session_events
WHERE game_session_id = $1 AND kind = 'scene_check_rolled' AND seq > $2
GROUP BY character_id, 2;

-- name: ListSceneAttemptGrantEvents :many
-- The attempts the master granted since the opening (seq): which character, at
-- which action (the payload's action_id).
SELECT character_id, payload FROM session_events
WHERE game_session_id = $1 AND kind = 'scene_attempt_granted' AND seq > $2;

-- name: GetOpenScenePoint :one
-- The map point of the scene open in the campaign's open session (NULL when
-- none). No row: no open session.
SELECT open_scene_point_id FROM game_sessions
WHERE campaign_id = $1 AND ended_at IS NULL;

-- The stage (MR-031): the NPCs "em cena" in the open scene. Every write below
-- runs after the caller locked the open session's row.

-- name: ListStage :many
-- The session's stage, in the order the NPCs came in.
SELECT * FROM stage_npcs
WHERE game_session_id = $1
ORDER BY position, id;

-- name: InsertStageNPC :one
-- An NPC comes in after the ones already there: the highest position plus one.
INSERT INTO stage_npcs (game_session_id, character_id, position, created_at)
VALUES (
    sqlc.arg(game_session_id), sqlc.arg(character_id),
    (SELECT COALESCE(max(position) + 1, 0)::INT4 FROM stage_npcs WHERE game_session_id = sqlc.arg(game_session_id)),
    sqlc.arg(created_at)
)
RETURNING *;

-- name: DeleteStageNPC :execrows
DELETE FROM stage_npcs
WHERE game_session_id = $1 AND character_id = $2;

-- name: ClearStage :execrows
-- The scene closed or changed: nobody is on the stage.
DELETE FROM stage_npcs
WHERE game_session_id = $1;

-- name: ClearStageSpeakers :exec
UPDATE stage_npcs SET speaking = false
WHERE game_session_id = $1 AND speaking;

-- name: SetStageSpeaker :execrows
UPDATE stage_npcs SET speaking = true
WHERE game_session_id = $1 AND character_id = $2;

-- name: ListEncounterCombatEvents :many
-- The events that count in the combat highlights (MR-032), oldest first, with
-- the undos that may take them back.
SELECT id, kind, payload FROM session_events
WHERE encounter_id = $1
  AND kind IN ('attack_rolled', 'damage_rolled', 'damage_applied', 'spell_cast', 'action_taken', 'action_undone')
ORDER BY seq
LIMIT 20000;

-- The session summary (MR-032): what happened in an ended session.

-- name: GetGameSessionInCampaign :one
-- One session of the campaign, ended or not (a session of another campaign
-- matches no row).
SELECT * FROM game_sessions
WHERE campaign_id = $1 AND id = $2;

-- name: ListSessionCombats :many
-- The combats that began in the session, oldest first. A combat that never
-- left setup (the session ended first) has no started_at and is not a combat
-- that happened.
SELECT * FROM encounters
WHERE game_session_id = $1 AND started_at IS NOT NULL
ORDER BY created_at, id;

-- name: CountSessionScenesOpened :one
-- The scenes the master opened in the session.
SELECT count(*)::INT8 FROM session_events
WHERE game_session_id = $1 AND kind = 'scene_opened';

-- name: TallySessionSceneChecks :many
-- The checks rolled outside combat, per character: tried and passed, counting
-- only a roll the players could see the DC of, on an action that had one
-- (RN-20). Done in SQL, so a session with any number of rolls costs one row
-- per character.
SELECT character_id,
       count(*)::INT8 AS tried,
       (count(*) FILTER (WHERE payload->>'passed' = 'true'))::INT8 AS passed
FROM session_events
WHERE game_session_id = $1 AND kind = 'scene_check_rolled'
  AND character_id IS NOT NULL
  AND payload->>'dc_shown' = 'true' AND payload ? 'passed'
GROUP BY character_id;

-- Progression (MR-016): what the XP awards read from the combats and the log.

-- name: GetCampaignEncounterXP :one
-- A combat of the campaign (never another campaign's) with the XP its defeated
-- NPC combatants give. No row: no such combat in the campaign.
SELECT e.name, e.status,
       COALESCE(SUM(c.xp_value) FILTER (WHERE c.kind = 'npc' AND c.defeated), 0)::INT8 AS xp
FROM encounters AS e
JOIN game_sessions AS gs ON gs.id = e.game_session_id
LEFT JOIN combatants AS c ON c.encounter_id = e.id
WHERE gs.campaign_id = sqlc.arg(campaign_id)::UUID AND e.id = sqlc.arg(id)
GROUP BY e.id, e.name, e.status;

-- name: ListCampaignEncounterNames :many
-- The names of the given combats of the campaign.
SELECT e.id, e.name FROM encounters AS e
JOIN game_sessions AS gs ON gs.id = e.game_session_id
WHERE gs.campaign_id = sqlc.arg(campaign_id)::UUID AND e.id = ANY(sqlc.arg(ids)::UUID[]);

-- The character's creatures in a combat (MR-037, Etapa 9). A creature is a
-- combatant of kind 'creature': character_id is its owner's.

-- name: InsertCreatureCombatant :one
-- A creature joins a combat. It has no initiative until its group rolls one,
-- and no square until somebody places it; its hit points come from the
-- creature.
INSERT INTO combatants (
    encounter_id, character_id, user_id, label, kind, hidden, initiative, initiative_bonus, initiative_face,
    order_index, grid_col, grid_row, speed_ft, hp_current, hp_max, hp_temp, created_at,
    creature_id, monster_key, summon_attack, summon_group_id,
    side, size, speed_fly_ft, jump_long_dft, jump_high_dft
) VALUES (
    $1, $2, $3, $4, 'creature', false, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, 0, $14,
    $15, $16, $17, $18,
    'party', $19, $20, $21, $22
)
RETURNING *;

-- name: SetGroupInitiative :exec
-- The roll of a group of creatures that came from one casting: every member of
-- the group in the combat takes the same total, face and bonus, so they take a
-- joint turn. A new roll breaks any tie order decided before.
UPDATE combatants
SET initiative = sqlc.arg(initiative), initiative_face = sqlc.arg(initiative_face),
    initiative_bonus = sqlc.arg(initiative_bonus), tie_ordered = false
WHERE encounter_id = sqlc.arg(encounter_id) AND summon_group_id = sqlc.arg(summon_group_id)::UUID;

-- name: GetEncounterByID :one
-- A combat by its ID alone (the creatures' host publishes the change of the
-- combat a dismissed creature left).
SELECT * FROM encounters WHERE id = $1;

-- name: ListCreatureCombatants :many
-- The creatures in a combat, in turn order: what the combat writes back to
-- their owners' lists and what it keeps in step with them.
SELECT * FROM combatants
WHERE encounter_id = $1 AND kind = 'creature' AND NOT dismissed
ORDER BY order_index, created_at, id;

-- name: ListOpenCombatantsOfCreatures :many
-- The combatants of these creatures in the campaign's combat that is not ended
-- (a session has at most one).
SELECT cb.* FROM combatants AS cb
JOIN encounters AS e ON e.id = cb.encounter_id
JOIN game_sessions AS gs ON gs.id = e.game_session_id
WHERE gs.campaign_id = sqlc.arg(campaign_id)::UUID
  AND e.status <> 'ended'
  AND NOT cb.dismissed
  AND cb.creature_id = ANY(sqlc.arg(creature_ids)::UUID[])
ORDER BY cb.order_index, cb.created_at, cb.id;

-- name: SetCreatureCombatantsDismissed :many
-- Hides (or brings back) the combatants of the creatures in the combat. A
-- dismissed one is out of the turn.
UPDATE combatants
SET dismissed = sqlc.arg(dismissed), turn_state = 'idle'
WHERE encounter_id = sqlc.arg(encounter_id) AND creature_id = ANY(sqlc.arg(creature_ids)::UUID[])
RETURNING id;

-- name: SetCreatureCombatantLabel :many
-- A creature's new name on its combatant, in the combat that is not ended.
UPDATE combatants
SET label = sqlc.arg(label)
WHERE creature_id = sqlc.arg(creature_id)::UUID
  AND encounter_id IN (
      SELECT e.id FROM encounters AS e
      JOIN game_sessions AS gs ON gs.id = e.game_session_id
      WHERE gs.campaign_id = sqlc.arg(campaign_id)::UUID AND e.status <> 'ended'
  )
RETURNING encounter_id;

-- Trap damage (MR-035, Etapa 9, D5): the server rolls it when the trap fires.

-- name: InsertTrapPendingDamage :one
-- A trap's damage in a combat: no attacker, rolled already. 'rolled' for a
-- player's character (waits for the master), 'applied' for an NPC or a creature
-- (the caller put it on the combatant). amount is what lands, roll_total the roll
-- before a half damage halves it.
INSERT INTO pending_damages (
    encounter_id, attacker_id, target_id, attack_key, status, critical,
    dice_count, dice_sides, dice_bonus, damage_type, faces, amount, roll_total, half,
    created_at, resolved_at, trap_point_id, critical_max, critical_max_rule
) VALUES (
    $1, NULL, $2, 'trap', $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, sqlc.narg(resolved_at), $14, $15, $16
)
RETURNING *;

-- name: ListOpenTrapPendingDamages :many
-- The trap damages of the session's combats that wait for the master, oldest
-- first. A combat that ended takes its open damage with it: nothing can be applied
-- there anymore.
SELECT p.* FROM pending_damages AS p
JOIN encounters AS e ON e.id = p.encounter_id
WHERE e.game_session_id = $1 AND e.status <> 'ended' AND p.trap_point_id IS NOT NULL AND p.status = 'rolled'
ORDER BY p.created_at, p.id;

-- name: InsertTrapDamage :one
-- A trap's damage to a player's character outside a combat.
INSERT INTO trap_damages (
    game_session_id, trap_point_id, fire_id, character_id, status, critical,
    dice_count, dice_sides, dice_bonus, damage_type, faces, roll_total, half, amount, created_at, critical_max
) VALUES ($1, $2, $3, $4, 'rolled', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: ListCampaignTrapDamages :many
-- The campaign's trap damages that wait for the master, outside a combat, from any
-- session (they outlive it), with the session's number and start.
SELECT d.*, g.session_number, g.started_at AS session_started_at FROM trap_damages AS d
JOIN game_sessions AS g ON g.id = d.game_session_id
WHERE g.campaign_id = $1 AND d.status = 'rolled'
ORDER BY d.created_at, d.id;

-- name: GetCampaignTrapDamageForUpdate :one
-- One trap damage of the campaign, locked, with its session's number and start.
SELECT d.*, g.session_number, g.started_at AS session_started_at FROM trap_damages AS d
JOIN game_sessions AS g ON g.id = d.game_session_id
WHERE g.campaign_id = $1 AND d.id = $2
FOR UPDATE OF d;

-- name: ListOpenTrapPendingDamagesOfEncounter :many
-- The trap damages a combat still holds for the master: what its end turns into
-- trap_damages rows.
SELECT * FROM pending_damages
WHERE encounter_id = $1 AND trap_point_id IS NOT NULL AND status = 'rolled'
ORDER BY created_at, id;

-- name: ListTrapEventsOfSession :many
-- The trap firings, searches and passive notices of a session outside a combat, newest first,
-- at most 500: a long session keeps the latest ones, and the caller puts them back in order.
-- (A notice has no combat even in one: the maps module writes it after the move.)
SELECT id, kind, character_id, payload, created_at FROM session_events
WHERE game_session_id = $1 AND encounter_id IS NULL AND kind IN ('trap_triggered', 'trap_searched', 'trap_noticed')
ORDER BY seq DESC
LIMIT 500;

-- name: GetSessionEventByID :one
SELECT id, kind, encounter_id, payload FROM session_events
WHERE game_session_id = $1 AND id = $2;

-- name: SetTrapDamageStatus :one
-- Applied (with the amount when it is not the rolled one) or discarded, with the key and the
-- request hash of the call that did it.
UPDATE trap_damages
SET status = $2, resolved_at = $3, applied_amount = $4, settle_key = $5, settle_hash = $6
WHERE id = $1
RETURNING *;

-- name: GetTrapDamageBySettleKey :one
-- The damage settled by the call with this (scoped) key, for a retry.
SELECT * FROM trap_damages WHERE settle_key = $1;

-- Opportunity offers (MR-034, RN-21): the right to one attack on a mover that
-- left a reactor's reach.

-- name: InsertOpportunityOffer :one
INSERT INTO opportunity_offers (encounter_id, move_id, mover_id, reactor_id, left_col, left_row, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPendingOpportunityOffers :many
-- What waits in the combat, oldest first.
SELECT * FROM opportunity_offers
WHERE encounter_id = $1 AND state = 'pending'
ORDER BY created_at, id;

-- name: GetOpportunityOffer :one
SELECT * FROM opportunity_offers
WHERE encounter_id = $1 AND id = $2;

-- name: GetOpportunityOfferByAttack :one
-- The offer an opportunity attack answered, by the damage it opened.
SELECT * FROM opportunity_offers
WHERE encounter_id = $1 AND attack_pending_id = $2;

-- name: SetOpportunityOfferState :one
-- Answered (attacked, declined, skipped or withdrawn), or back to pending (an undo).
UPDATE opportunity_offers
SET state = $2, attack_pending_id = $3, answered_at = $4
WHERE id = $1
RETURNING *;

-- name: SkipPendingOpportunityOffersOfMover :execrows
-- The master ended the mover's turn: what it waited for is passed over.
UPDATE opportunity_offers
SET state = 'skipped', answered_at = $3
WHERE encounter_id = $1 AND mover_id = $2 AND state = 'pending';

-- name: SkipPendingOpportunityOffersBetweenAllies :execrows
-- A combatant changed side: the offers it is in (as mover or as reactor) whose
-- two sides are now the same are passed over, since only a hostile reactor may
-- attack.
UPDATE opportunity_offers AS o
SET state = 'skipped', answered_at = $3
FROM combatants AS mover, combatants AS reactor
WHERE o.encounter_id = $1 AND o.state = 'pending' AND (o.mover_id = $2 OR o.reactor_id = $2)
  AND mover.id = o.mover_id AND reactor.id = o.reactor_id AND mover.side = reactor.side;

-- name: DeleteOpportunityOffersOfMove :exec
-- The master's undo of the move that made them.
DELETE FROM opportunity_offers
WHERE move_id = $1;

-- name: ListWaitingOpportunityOffers :many
-- What holds the mover's turn: the offers nobody answered yet, and the offers
-- answered with an attack whose damage is still to roll, apply or discard.
SELECT o.* FROM opportunity_offers o
LEFT JOIN pending_damages p ON p.id = o.attack_pending_id
WHERE o.encounter_id = $1
  AND (o.state = 'pending' OR (o.state = 'attacked' AND p.status IN ('awaiting_reaction', 'awaiting_roll', 'rolled')))
ORDER BY o.created_at, o.id;

-- name: ListTrapDamageStatuses :many
-- Where trap damages are now (applied by the master since the firing), by ID.
SELECT id, status, applied_amount FROM trap_damages
WHERE id = ANY($1::uuid[]);

-- Puzzles (MR-038, RN-27, Etapa 10): what the master makes (puzzles), what a
-- session plays (puzzle_runs) and every move (puzzle_moves). The service is in
-- puzzles*.go.

-- name: InsertPuzzle :one
INSERT INTO puzzles (
    campaign_id, kind, name, config, solution, seed, start, minimum_moves, clue, hints,
    solve_action, solve_target, hint_skill, hint_dc, parts, on_wrong, created_at, updated_at, create_key, create_hash
) VALUES (sqlc.arg(campaign_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(config), sqlc.arg(solution), sqlc.arg(seed), sqlc.arg(start), sqlc.arg(minimum_moves), sqlc.arg(clue), sqlc.arg(hints), sqlc.arg(solve_action), sqlc.arg(solve_target), sqlc.arg(hint_skill), sqlc.arg(hint_dc), sqlc.arg(parts), sqlc.arg(on_wrong), sqlc.arg(created_at), sqlc.arg(created_at), sqlc.narg(create_key), sqlc.narg(create_hash))
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetPuzzleByCreateKey :one
-- The puzzle a CreatePuzzle with this idempotency key made, if any (the key carries the
-- campaign's ID).
SELECT * FROM puzzles WHERE create_key = $1;

-- name: GetPuzzle :one
SELECT * FROM puzzles
WHERE campaign_id = $1 AND id = $2;

-- name: GetPuzzleForUpdate :one
-- One puzzle of the campaign, locked until the transaction ends: an edit and a
-- show of the same puzzle take turns.
SELECT * FROM puzzles
WHERE campaign_id = $1 AND id = $2
FOR UPDATE;

-- name: ListPuzzles :many
-- The campaign's puzzles, newest first; the archived ones only when asked.
SELECT * FROM puzzles
WHERE campaign_id = $1 AND (archived_at IS NULL OR sqlc.arg(include_archived)::bool)
ORDER BY created_at DESC, id;

-- name: UpdatePuzzle :one
-- Everything the master writes about a puzzle.
UPDATE puzzles
SET name = $3, config = $4, solution = $5, seed = $6, start = $7, minimum_moves = $8,
    clue = $9, hints = $10, solve_action = $11, solve_target = $12,
    hint_skill = $13, hint_dc = $14, parts = $15, on_wrong = $16, updated_at = $17
WHERE campaign_id = $1 AND id = $2
RETURNING *;

-- name: SetPuzzleArchived :one
UPDATE puzzles
SET archived_at = $3, updated_at = $4
WHERE campaign_id = $1 AND id = $2
RETURNING *;

-- name: ListShownPuzzleIDs :many
-- The campaign's puzzles that were shown in any session: they can no longer be edited.
SELECT DISTINCT r.puzzle_id FROM puzzle_runs AS r
JOIN puzzles AS p ON p.id = r.puzzle_id
WHERE p.campaign_id = $1 AND r.shown_at IS NOT NULL;

-- name: InsertPuzzleRun :one
-- A run starts with its state at its start. $5 is when it was shown (NULL for a run
-- that is only prepared) and $6 when the round's clock starts (NULL too for a sequence,
-- whose clock starts at its first play).
INSERT INTO puzzle_runs (game_session_id, puzzle_id, seed, start, state, shown_at, round_started_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $7)
RETURNING *;

-- name: GetPuzzleRun :one
SELECT * FROM puzzle_runs
WHERE game_session_id = $1 AND puzzle_id = $2;

-- name: GetPuzzleRunForUpdate :one
-- The run, locked until the transaction ends. Every change locks the session first
-- (GetOpenGameSessionForUpdate) and then the run, so two changes take turns.
SELECT * FROM puzzle_runs
WHERE game_session_id = $1 AND puzzle_id = $2
FOR UPDATE;

-- name: ListPuzzleRuns :many
SELECT * FROM puzzle_runs
WHERE game_session_id = $1;

-- name: ListShownPuzzleRuns :many
-- What a session shows now: shown and not closed, in the order they were shown.
SELECT r.*, p.name AS puzzle_name, p.kind AS puzzle_kind, p.on_wrong AS puzzle_on_wrong FROM puzzle_runs AS r
JOIN puzzles AS p ON p.id = r.puzzle_id
WHERE r.game_session_id = $1 AND r.shown_at IS NOT NULL AND r.closed_at IS NULL AND p.archived_at IS NULL
ORDER BY r.shown_at, r.id;

-- name: SavePuzzleRun :one
-- Writes every field of the run that changes after it is made: the caller holds the
-- run's row lock and worked the new values out from the row it read.
UPDATE puzzle_runs
SET seed = $2, start = $3, state = $4, released_hints = $5, shown_at = $6, closed_at = $7,
    solved_at = $8, solved_by_character_id = $9, solve_outcome = $10,
    last_mover_character_id = $11, last_move = $12, last_moved_at = $13,
    moves_made = $14, revision = $15, updated_at = $16, solve_message = $17,
    plays = $18, play_started_at = $19, round_start_seq = $20, round_started_at = $21
WHERE id = $1
RETURNING *;

-- name: GetPuzzleMoveByKey :one
-- The move a call with this key already made, if any.
SELECT * FROM puzzle_moves
WHERE run_id = $1 AND idempotency_key = $2;

-- name: InsertPuzzleMove :one
INSERT INTO puzzle_moves (run_id, seq, user_id, character_id, idempotency_key, move, revision, solved, wrong, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: CountWrongPuzzleMoves :many
-- The wrong moves of each player in the run's current round: the moves after the
-- seq the round began at. A player whose account was deleted has no user and no row.
SELECT user_id, count(*)::int4 AS wrong FROM puzzle_moves
WHERE run_id = $1 AND seq > $2 AND wrong AND user_id IS NOT NULL
GROUP BY user_id;

-- name: InsertPuzzleHintTry :one
INSERT INTO puzzle_hint_tries (run_id, user_id, character_id, idempotency_key, hint_index, passed, granted_count, d20, modifier, total, physical, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetPuzzleHintTryByKey :one
-- The try a call with this key already made, if any.
SELECT * FROM puzzle_hint_tries
WHERE run_id = $1 AND idempotency_key = $2;

-- name: GetPuzzleHintCursor :one
-- How many hints the player has won in the run: the largest count a pass left.
SELECT COALESCE(max(granted_count), 0)::int4 AS granted FROM puzzle_hint_tries
WHERE run_id = $1 AND user_id = $2 AND passed;

-- name: HasTriedPuzzleHint :one
-- Whether the player tried for this hint already (a pass or a fail).
SELECT EXISTS (
    SELECT 1 FROM puzzle_hint_tries WHERE run_id = $1 AND user_id = $2 AND hint_index = $3
) AS tried;

-- name: ListRecentPuzzleHintTries :many
-- The run's latest tries, newest first (the master's view keeps the last 50).
SELECT * FROM puzzle_hint_tries
WHERE run_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 50;

-- name: DeletePreparedPuzzleRuns :exec
-- The runs of a puzzle that were prepared ("Gerar outro começo") and never shown: an
-- edit of the puzzle makes their start wrong, so they go (the master draws another).
DELETE FROM puzzle_runs
WHERE puzzle_id = $1 AND shown_at IS NULL;

-- The encounter kept on a battle point (MR-043, slice 10.9c; encounters.go): the master's
-- secret, read by no query that serves a player.

-- name: GetBattleEncounter :one
SELECT * FROM battle_encounters
WHERE campaign_id = $1 AND map_point_id = $2;

-- name: UpsertBattleEncounter :one
-- Keeps the encounter on the point, in place of the one it had.
INSERT INTO battle_encounters (map_point_id, campaign_id, map_id, encounter, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (map_point_id) DO UPDATE
SET encounter = excluded.encounter, map_id = excluded.map_id, updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteBattleEncounter :execrows
DELETE FROM battle_encounters
WHERE campaign_id = $1 AND map_point_id = $2;

-- name: ListBattleEncounters :many
-- The battle points of a map that keep an encounter, oldest first. A point that stopped being a
-- battle point is left out (its row stays until cleared).
SELECT e.* FROM battle_encounters AS e
JOIN map_points AS p ON p.id = e.map_point_id
WHERE e.campaign_id = $1 AND e.map_id = $2 AND p.kind = 'battle'
ORDER BY e.created_at, e.map_point_id;

-- name: NextHiddenRevealSeq :one
-- The place of the next question of the combat in the order they are answered.
SELECT (COALESCE(MAX(seq), 0) + 1)::INT4 FROM hidden_reveals WHERE encounter_id = $1;

-- name: InsertHiddenReveal :one
-- A player's area spell hit hidden creatures and the table asks the master: the
-- question, with where the area landed.
INSERT INTO hidden_reveals (encounter_id, caster_id, spell_key, combatant_ids, origin_col, origin_row, squares, seq, created_at)
VALUES (
    sqlc.arg(encounter_id), sqlc.arg(caster_id), sqlc.arg(spell_key), sqlc.arg(combatant_ids)::TEXT[],
    sqlc.arg(origin_col), sqlc.arg(origin_row), sqlc.arg(squares)::INT4[], sqlc.arg(seq), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListPendingHiddenReveals :many
-- The questions the master has still to answer, oldest first. They hold the turn.
SELECT * FROM hidden_reveals WHERE encounter_id = $1 AND state = 'pending' ORDER BY seq;

-- name: GetHiddenReveal :one
SELECT * FROM hidden_reveals WHERE encounter_id = $1 AND id = $2;

-- name: AnswerHiddenReveal :one
-- The master's answer; only a question that waits can be answered.
UPDATE hidden_reveals SET state = sqlc.arg(state), answered_at = sqlc.arg(answered_at)
WHERE encounter_id = sqlc.arg(encounter_id) AND id = sqlc.arg(id) AND state = 'pending'
RETURNING *;

-- name: DeleteHiddenReveal :exec
-- An undo of the cast that opened the question takes it away.
DELETE FROM hidden_reveals WHERE id = $1;

-- name: DeleteHiddenRevealsOfEncounter :exec
-- Ending the combat drops what was still to answer.
DELETE FROM hidden_reveals WHERE encounter_id = $1;

-- name: NextMapZoneSeq :one
-- The order the zones of a combat were put in, for the next one.
SELECT (COALESCE(MAX(seq), 0) + 1)::INT4 FROM map_zones WHERE encounter_id = $1;

-- name: InsertMapZone :one
-- A zone a spell or the master left on the map. The squares it covers were worked out
-- by the server from the shape and the walls.
INSERT INTO map_zones (
    encounter_id, seq, spell_key, name, caster_id, shape, origin_col, origin_row, dir_dx, dir_dy, size_ft, ring_radius, cells,
    obscurity, difficult, halves_speed, camouflaged, visible_to_players, concentration, slot_level, cast_round, duration_rounds,
    moves_with, step_squares, caster_moves, triggers, rules, save_dc, damage_count, damage_sides, damage_bonus, damage_type,
    damage_side, reach_squares, anchored, excluded_ids, known_by, members, created_at
) VALUES (
    sqlc.arg(encounter_id), sqlc.arg(seq), sqlc.arg(spell_key), sqlc.arg(name), sqlc.narg(caster_id), sqlc.arg(shape),
    sqlc.narg(origin_col), sqlc.narg(origin_row), sqlc.arg(dir_dx), sqlc.arg(dir_dy), sqlc.arg(size_ft), sqlc.arg(ring_radius),
    sqlc.arg(cells)::INT4[], sqlc.arg(obscurity), sqlc.arg(difficult), sqlc.arg(halves_speed), sqlc.arg(camouflaged),
    sqlc.arg(visible_to_players), sqlc.arg(concentration), sqlc.arg(slot_level), sqlc.arg(cast_round), sqlc.arg(duration_rounds),
    sqlc.arg(moves_with), sqlc.arg(step_squares), sqlc.arg(caster_moves), sqlc.arg(triggers)::JSONB, sqlc.arg(rules)::TEXT[],
    sqlc.arg(save_dc), sqlc.arg(damage_count), sqlc.arg(damage_sides), sqlc.arg(damage_bonus), sqlc.arg(damage_type),
    sqlc.arg(damage_side), sqlc.arg(reach_squares), sqlc.arg(anchored), sqlc.arg(excluded_ids)::TEXT[], sqlc.arg(known_by)::TEXT[],
    sqlc.arg(members)::TEXT[], sqlc.arg(created_at)
)
RETURNING *;

-- name: ListMapZones :many
-- The zones of a combat, in the order they were put.
SELECT * FROM map_zones WHERE encounter_id = $1 AND ended_at IS NULL ORDER BY seq;

-- name: ListAllMapZones :many
-- Every zone of a combat, the ended ones too: the combat log names them.
SELECT * FROM map_zones WHERE encounter_id = $1 ORDER BY seq;

-- name: GetMapZone :one
SELECT * FROM map_zones WHERE encounter_id = $1 AND id = $2 AND ended_at IS NULL;

-- name: EndMapZone :exec
-- Ending a zone keeps its row, for the log; its ledger and the conditions it put go with the caller.
UPDATE map_zones SET ended_at = $2 WHERE id = $1;

-- name: DeleteMapZonesOfEncounter :exec
-- Ending a combat discards its zones and the saves they waited for.
DELETE FROM map_zones WHERE encounter_id = $1;

-- name: DeleteZoneFiredOfZone :exec
DELETE FROM map_zone_fired WHERE zone_id = $1;

-- name: SetMapZonePlace :one
-- A zone moved: its new point and the squares it covers.
UPDATE map_zones SET origin_col = sqlc.narg(origin_col), origin_row = sqlc.narg(origin_row), cells = sqlc.arg(cells)::INT4[]
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetMapZoneVisible :exec
UPDATE map_zones SET visible_to_players = $2 WHERE id = $1;

-- name: SetMapZoneKnownBy :exec
UPDATE map_zones SET known_by = sqlc.arg(known_by)::TEXT[] WHERE id = sqlc.arg(id);

-- name: SetMapZoneMembers :exec
UPDATE map_zones SET members = sqlc.arg(members)::TEXT[] WHERE id = sqlc.arg(id);

-- name: SetMapZoneDisperseRound :exec
UPDATE map_zones SET disperse_round = $2 WHERE id = $1;

-- name: SetMapZoneDuration :exec
-- A zone that collapses (a Web not anchored) ends at the start of the caster's next turn.
UPDATE map_zones SET cast_round = $2, duration_rounds = $3 WHERE id = $1;

-- name: ClearMapZoneCaster :exec
-- The caster left the combat: the zone goes on without one.
UPDATE map_zones SET caster_id = NULL WHERE caster_id = $1;

-- name: MarkZoneFired :execrows
-- Notes that a zone hurt a creature in a turn. One row was inserted when the creature
-- had not been hurt yet in that turn of that zone: that is the answer.
INSERT INTO map_zone_fired (zone_id, combatant_id, round, turn_of) VALUES ($1, $2, $3, $4)
ON CONFLICT (zone_id, combatant_id, round, turn_of) DO NOTHING;

-- name: UnmarkZoneFired :exec
DELETE FROM map_zone_fired WHERE zone_id = $1 AND combatant_id = $2 AND round = $3 AND turn_of = $4;

-- name: PruneZoneFired :exec
-- Turns of earlier rounds are over for good.
DELETE FROM map_zone_fired WHERE round < $2 AND zone_id IN (SELECT id FROM map_zones WHERE encounter_id = $1);

-- name: InsertZoneEffect :execrows
INSERT INTO map_zone_effects (zone_id, combatant_id, condition) VALUES ($1, $2, $3)
ON CONFLICT (zone_id, combatant_id, condition) DO NOTHING;

-- name: ListZoneEffects :many
SELECT * FROM map_zone_effects WHERE zone_id = $1 ORDER BY combatant_id, condition;

-- name: ListZoneEffectsOfCombatant :many
SELECT * FROM map_zone_effects WHERE combatant_id = $1 ORDER BY zone_id, condition;

-- name: DeleteZoneEffect :exec
DELETE FROM map_zone_effects WHERE zone_id = $1 AND combatant_id = $2 AND condition = $3;

-- name: NextZoneSaveSeq :one
SELECT (COALESCE(MAX(seq), 0) + 1)::INT4 FROM zone_save_windows WHERE encounter_id = $1;

-- name: InsertZoneSaveWindow :one
-- A saving throw a zone asks of a creature: the turn of the one it is asked of waits.
INSERT INTO zone_save_windows (
    encounter_id, zone_id, reactor_id, caster_id, seq, trigger_kind, round, turn_of, spell_key, ability, dc,
    damage_count, damage_sides, damage_bonus, damage_type, on_success, on_fail, cover_bonus, created_at
) VALUES (
    sqlc.arg(encounter_id), sqlc.narg(zone_id), sqlc.arg(reactor_id), sqlc.narg(caster_id), sqlc.arg(seq), sqlc.arg(trigger_kind),
    sqlc.arg(round), sqlc.narg(turn_of), sqlc.arg(spell_key), sqlc.arg(ability), sqlc.arg(dc), sqlc.arg(damage_count),
    sqlc.arg(damage_sides), sqlc.arg(damage_bonus), sqlc.arg(damage_type), sqlc.arg(on_success), sqlc.arg(on_fail), sqlc.arg(cover_bonus), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListOpenZoneSaveWindows :many
SELECT * FROM zone_save_windows WHERE encounter_id = $1 AND state = 'open' ORDER BY seq;

-- name: GetZoneSaveWindow :one
SELECT * FROM zone_save_windows WHERE encounter_id = $1 AND id = $2;

-- name: AnswerZoneSaveWindow :one
UPDATE zone_save_windows
SET state = 'answered', d20 = sqlc.arg(d20), modifier = sqlc.arg(modifier), total = sqlc.arg(total), saved = sqlc.arg(saved),
    physical = sqlc.arg(physical), answered_at = sqlc.arg(answered_at)
WHERE id = sqlc.arg(id) AND state = 'open'
RETURNING *;

-- name: CloseZoneSaveWindow :exec
UPDATE zone_save_windows SET state = 'closed', close_reason = $2, answered_at = $3 WHERE id = $1 AND state = 'open';

-- name: CloseZoneSaveWindowsOfZone :exec
UPDATE zone_save_windows SET state = 'closed', close_reason = $2, answered_at = $3 WHERE zone_id = $1 AND state = 'open';

-- name: DeleteMapZone :exec
-- A zone whose cast an undo took back never was.
DELETE FROM map_zones WHERE id = $1;

-- name: UnendMapZone :exec
-- A zone an ended concentration took with it comes back with the undo.
UPDATE map_zones SET ended_at = NULL WHERE id = $1;

-- name: DeleteZoneSaveWindowsOfEncounter :exec
DELETE FROM zone_save_windows WHERE encounter_id = $1;
