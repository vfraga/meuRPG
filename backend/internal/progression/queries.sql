-- Every query names the campaign next to the award: an award ID of another
-- campaign matches no row.

-- name: InsertXPAward :one
INSERT INTO xp_awards
    (campaign_id, given_by, created_at, mode, reason, encounter_id, gold, total_xp, idempotency_key, milestone_id, milestone_again, idempotency_hash)
VALUES (
    sqlc.arg(campaign_id)::UUID, sqlc.arg(given_by)::UUID, sqlc.arg(created_at), sqlc.arg(mode), sqlc.arg(reason),
    sqlc.narg(encounter_id)::UUID, sqlc.narg(gold), sqlc.arg(total_xp), sqlc.arg(idempotency_key)::UUID,
    sqlc.narg(milestone_id)::UUID, sqlc.arg(milestone_again)::BOOL, sqlc.narg(idempotency_hash)
)
RETURNING *;

-- name: InsertXPShare :exec
INSERT INTO xp_award_shares (award_id, character_id, xp, level_at_mark)
VALUES (sqlc.arg(award_id)::UUID, sqlc.arg(character_id)::UUID, sqlc.arg(xp), sqlc.narg(level_at_mark));

-- name: GetXPAwardByKey :one
-- The award a retry of the same request finds.
SELECT * FROM xp_awards
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND idempotency_key = sqlc.arg(idempotency_key)::UUID;

-- name: GetXPAwardByUndoKey :one
-- The award a retry of the same undo finds.
SELECT * FROM xp_awards
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND undo_key = sqlc.arg(undo_key)::UUID;

-- name: GetLastXPAwardForUpdate :one
-- The award an undo would take back: the newest one not undone. FOR UPDATE, so
-- two undos wait for each other instead of both taking back the same award.
SELECT * FROM xp_awards
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND undone_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 1
FOR UPDATE;

-- name: GetLastXPAwardID :one
-- The newest award not undone, for the history's "Desfazer".
SELECT id FROM xp_awards
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND undone_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: MarkXPAwardUndone :one
-- undone_at IS NULL in the WHERE clause is a second guard: no row means it was
-- undone meanwhile.
UPDATE xp_awards
SET undone_at = sqlc.arg(undone_at), undone_by = sqlc.arg(undone_by)::UUID, undo_key = sqlc.arg(undo_key)::UUID
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)::UUID AND undone_at IS NULL
RETURNING *;

-- name: HasLiveEnemiesAward :one
-- Whether the combat's XP was already given (and not undone).
SELECT EXISTS (
    SELECT 1 FROM xp_awards
    WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND encounter_id = sqlc.arg(encounter_id)::UUID
      AND mode = 'enemies' AND undone_at IS NULL
);

-- name: ListXPAwardsPage :many
-- A page of the history, newest first: the awards before the cursor (created_at
-- and id of the last one of the previous page), or the first page without it.
SELECT * FROM xp_awards
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
  AND (
      sqlc.narg(before_created_at)::TIMESTAMPTZ IS NULL
      OR (created_at, id) < (sqlc.narg(before_created_at)::TIMESTAMPTZ, sqlc.narg(before_id)::UUID)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size);

-- name: ListXPSharesOfAwards :many
SELECT s.award_id, s.character_id, s.xp, s.level_at_mark
FROM xp_award_shares AS s
JOIN characters AS c ON c.id = s.character_id
WHERE s.award_id = ANY(sqlc.arg(award_ids)::UUID[])
ORDER BY c.created_at, c.id;

-- name: ListMilestoneMarks :many
-- The highest level each character of the campaign was marked at, by the
-- milestones not undone: the "pode subir de nível" tag lasts until the sheet's
-- level goes past it (RN-12).
SELECT s.character_id, max(s.level_at_mark)::INT4 AS level
FROM xp_award_shares AS s
JOIN xp_awards AS a ON a.id = s.award_id
WHERE a.campaign_id = sqlc.arg(campaign_id)::UUID AND a.mode = 'milestone' AND a.undone_at IS NULL
  AND s.level_at_mark IS NOT NULL
GROUP BY s.character_id;

-- name: GetMilestoneMark :one
-- ListMilestoneMarks for one character; no row means it is not marked.
SELECT COALESCE(max(s.level_at_mark), 0)::INT4 AS level
FROM xp_award_shares AS s
JOIN xp_awards AS a ON a.id = s.award_id
WHERE a.campaign_id = sqlc.arg(campaign_id)::UUID AND a.mode = 'milestone' AND a.undone_at IS NULL
  AND s.character_id = sqlc.arg(character_id)::UUID AND s.level_at_mark IS NOT NULL;

-- The planned milestones (00084). Every query names the campaign next to the
-- milestone: an ID of another campaign matches no row.

-- name: ListPlannedMilestones :many
SELECT * FROM planned_milestones
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
ORDER BY position, id;

-- name: ListPlannedMilestonesForUpdate :many
-- The campaign's whole list, locked: an add or a move takes turns with the
-- others, so positions never repeat and the count limit holds.
SELECT * FROM planned_milestones
WHERE campaign_id = sqlc.arg(campaign_id)::UUID
ORDER BY position, id
FOR UPDATE;

-- name: GetPlannedMilestoneForUpdate :one
SELECT * FROM planned_milestones
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)::UUID
FOR UPDATE;

-- name: InsertPlannedMilestone :one
-- create_key and create_hash are the idempotency key and the hash of the request (NULL when the
-- call sent no key); the caller reads the first row with GetPlannedMilestoneByCreateKey.
INSERT INTO planned_milestones (campaign_id, position, text, create_key, create_hash, created_at, updated_at)
VALUES (sqlc.arg(campaign_id)::UUID, sqlc.arg(position), sqlc.arg(text), sqlc.narg(create_key), sqlc.narg(create_hash), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetPlannedMilestoneByCreateKey :one
-- The milestone an AddMilestone with this idempotency key made, if any (the key carries the
-- campaign's ID).
SELECT * FROM planned_milestones WHERE create_key = $1;

-- name: UpdatePlannedMilestoneText :exec
UPDATE planned_milestones SET text = sqlc.arg(text), updated_at = sqlc.arg(now)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)::UUID;

-- name: SetPlannedMilestonePosition :exec
UPDATE planned_milestones SET position = sqlc.arg(position)
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)::UUID;

-- name: DeletePlannedMilestone :exec
DELETE FROM planned_milestones
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND id = sqlc.arg(id)::UUID;

-- name: ListLiveMilestoneAwards :many
-- The milestone marks that are not undone, oldest first: those of the planned
-- milestones (what makes one reached) and the ones marked off the list
-- (milestone_id NULL).
SELECT * FROM xp_awards
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND mode = 'milestone' AND undone_at IS NULL
ORDER BY created_at, id;

-- name: ListLiveMilestoneAwardsOf :many
-- The same for one milestone.
SELECT * FROM xp_awards
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND milestone_id = sqlc.arg(milestone_id)::UUID AND undone_at IS NULL
ORDER BY created_at, id;

-- name: ListMilestoneMarkedCharacters :many
-- The characters that have the milestone now (a mark not undone).
SELECT s.character_id
FROM xp_award_shares AS s
JOIN xp_awards AS a ON a.id = s.award_id
WHERE a.campaign_id = sqlc.arg(campaign_id)::UUID AND a.milestone_id = sqlc.arg(milestone_id)::UUID AND a.undone_at IS NULL;

-- name: ListMilestoneIDsWithAwards :many
-- The planned milestones any award names, undone ones included: HasMilestoneAwards
-- for the whole list, so the list can say which ones RemoveMilestone refuses.
SELECT DISTINCT milestone_id::UUID AS milestone_id FROM xp_awards
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND milestone_id IS NOT NULL;

-- name: HasMilestoneAwards :one
-- Whether any award, undone ones included, names the milestone: a milestone
-- with history is never removed (ADR-0007: awards are never rewritten).
SELECT EXISTS (
    SELECT 1 FROM xp_awards
    WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND milestone_id = sqlc.arg(milestone_id)::UUID
);

-- The treasures a "Voltar à cidade" award converted (00101).

-- name: InsertXPAwardTreasure :exec
INSERT INTO xp_award_treasures (award_id, point_id, value_po)
VALUES (sqlc.arg(award_id)::UUID, sqlc.arg(point_id)::UUID, sqlc.arg(value_po));

-- name: ListXPAwardTreasures :many
SELECT award_id, point_id, value_po
FROM xp_award_treasures
WHERE award_id = ANY(sqlc.arg(award_ids)::UUID[])
ORDER BY award_id, point_id;

-- name: SumLiveAwards :one
-- How much XP the campaign gave and has not undone (RN-09): the awards that
-- stand (a milestone mark is one, with no XP) and what the characters got from
-- them. The master's change of the XP mode asks before it goes on.
SELECT count(*)::INT4 AS awards,
       COALESCE((SELECT sum(s.xp) FROM xp_award_shares AS s JOIN xp_awards AS a2 ON a2.id = s.award_id
                 WHERE a2.campaign_id = sqlc.arg(campaign_id)::UUID AND a2.undone_at IS NULL), 0)::INT8 AS total_xp
FROM xp_awards
WHERE campaign_id = sqlc.arg(campaign_id)::UUID AND undone_at IS NULL;
