-- The contests and the special actions of a combat (migration 00196): grapple,
-- shove, escape, Hide, Help, group checks and surprise. Every write runs in the
-- change's transaction, with the open session's row locked.

-- name: InsertContest :one
INSERT INTO combat_contests (
    encounter_id, kind, purpose, initiator_id, defender_id, status, escape_dc,
    initiator_roll, defender_roll, winner, shove_outcome, round, created_at, resolved_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14
)
RETURNING *;

-- name: GetContest :one
SELECT * FROM combat_contests
WHERE encounter_id = $1 AND id = $2;

-- name: ListContests :many
-- The combat's latest contests, newest first.
SELECT * FROM combat_contests
WHERE encounter_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: ListUnansweredContestsOf :many
-- The contests of the encounter that still wait for something and that the
-- combatant is in.
SELECT * FROM combat_contests
WHERE encounter_id = $1 AND status IN ('awaiting_defender', 'awaiting_outcome')
  AND (initiator_id = $2 OR defender_id = $2)
ORDER BY created_at, id;

-- name: SetContestAnswered :one
-- The defender's roll and what it decided.
UPDATE combat_contests
SET defender_roll = $3, winner = $4, status = $5, resolved_at = $6
WHERE encounter_id = $1 AND id = $2
RETURNING *;

-- name: SetContestState :one
-- A change of the contest's status (the shove's outcome, the master closing it),
-- or its undo.
UPDATE combat_contests
SET status = $3, shove_outcome = $4, resolved_at = $5
WHERE encounter_id = $1 AND id = $2
RETURNING *;

-- name: ListHolds :many
SELECT * FROM combat_holds
WHERE encounter_id = $1
ORDER BY created_at, grappled_id;

-- name: GetHold :one
SELECT * FROM combat_holds
WHERE grappled_id = $1;

-- name: UpsertHold :exec
INSERT INTO combat_holds (grappled_id, encounter_id, grappler_id, escape_dc, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (grappled_id) DO UPDATE
SET grappler_id = excluded.grappler_id, escape_dc = excluded.escape_dc, created_at = excluded.created_at;

-- name: DeleteHold :exec
DELETE FROM combat_holds
WHERE grappled_id = $1;

-- name: InsertHideAttempt :one
INSERT INTO combat_hide_attempts (encounter_id, hider_id, status, roll, refusal, round, created_at, resolved_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetHideAttempt :one
SELECT * FROM combat_hide_attempts
WHERE encounter_id = $1 AND id = $2;

-- name: ListHideAttempts :many
-- The combat's hide attempts, newest first.
SELECT * FROM combat_hide_attempts
WHERE encounter_id = $1
ORDER BY created_at DESC, id DESC;

-- name: SetHideAttemptDecision :one
UPDATE combat_hide_attempts
SET status = $3, refusal = $4, resolved_at = $5
WHERE encounter_id = $1 AND id = $2
RETURNING *;

-- name: ListHiding :many
SELECT * FROM combat_hiding
WHERE encounter_id = $1
ORDER BY created_at, hider_id, observer_id;

-- name: UpsertHiding :exec
INSERT INTO combat_hiding (hider_id, observer_id, encounter_id, noticed, total, passive, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (hider_id, observer_id) DO UPDATE
SET noticed = excluded.noticed, total = excluded.total, passive = excluded.passive, created_at = excluded.created_at;

-- name: DeleteHidingOfHider :exec
DELETE FROM combat_hiding
WHERE hider_id = $1;

-- name: InsertHelp :one
INSERT INTO combat_helps (
    game_session_id, encounter_id, kind, helper_character_id, ally_character_id, task, target_id,
    expires_round, created_round, created_at, helper_combatant_id, ally_combatant_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12
)
RETURNING *;

-- name: ListLiveHelps :many
-- The helps of the session not used or cleared yet: the caller decides which
-- ones ran out.
SELECT * FROM combat_helps
WHERE game_session_id = $1 AND consumed_at IS NULL AND cleared_at IS NULL
ORDER BY created_at, id;

-- name: GetHelp :one
SELECT * FROM combat_helps
WHERE game_session_id = $1 AND id = $2;

-- name: SetHelpConsumed :exec
UPDATE combat_helps
SET consumed_at = $2
WHERE id = $1;

-- name: SetHelpCleared :exec
UPDATE combat_helps
SET cleared_at = $2
WHERE id = $1;

-- name: ListSurprised :many
SELECT * FROM combat_surprised
WHERE encounter_id = $1
ORDER BY created_at, combatant_id;

-- name: UpsertSurprised :exec
INSERT INTO combat_surprised (combatant_id, encounter_id, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (combatant_id) DO NOTHING;

-- name: DeleteSurprised :exec
DELETE FROM combat_surprised
WHERE combatant_id = $1;

-- name: InsertGroupCheck :one
INSERT INTO group_checks (game_session_id, skill_key, dc, show_dc, is_group, status, created_at)
VALUES ($1, $2, $3, $4, $5, 'open', $6)
RETURNING *;

-- name: GetGroupCheck :one
SELECT * FROM group_checks
WHERE game_session_id = $1 AND id = $2;

-- name: GetOpenGroupCheck :one
SELECT * FROM group_checks
WHERE game_session_id = $1 AND status = 'open'
ORDER BY created_at DESC
LIMIT 1;

-- name: GetLatestGroupCheck :one
SELECT * FROM group_checks
WHERE game_session_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListGroupChecksOfSkill :many
-- The session's group checks of one skill, newest first (the surprise suggestion reads
-- the Stealth totals of the latest).
SELECT * FROM group_checks
WHERE game_session_id = $1 AND skill_key = $2
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- name: CloseGroupCheck :one
UPDATE group_checks
SET status = 'closed', passed = $3, closed_at = $4
WHERE game_session_id = $1 AND id = $2
RETURNING *;

-- name: InsertGroupCheckMember :exec
INSERT INTO group_check_members (group_check_id, character_id)
VALUES ($1, $2);

-- name: ListGroupCheckMembers :many
SELECT * FROM group_check_members
WHERE group_check_id = $1
ORDER BY character_id;

-- name: SetGroupCheckMemberRoll :one
UPDATE group_check_members
SET roll = $3, rolled_by_master = $4, rolled_at = $5
WHERE group_check_id = $1 AND character_id = $2
RETURNING *;

-- name: GetCombatantOrder :one
-- A combatant's place in the order of the turns, for what lasts through a turn
-- (surprise, a Help).
SELECT order_index FROM combatants
WHERE id = $1;

-- name: SetCombatantPlace :exec
-- A combatant put on a square without moving: a shove's push (no movement spent,
-- nothing provoked, the master's cover mark cleared).
UPDATE combatants
SET grid_col = $2, grid_row = $3, cover_mark = $4
WHERE id = $1;

-- name: IsCombatantSurprised :one
SELECT EXISTS (SELECT 1 FROM combat_surprised WHERE combatant_id = $1);
