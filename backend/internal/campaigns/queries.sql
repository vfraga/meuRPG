-- name: InsertCampaign :one
-- create_key and create_hash are the idempotency key of CreateCampaign and the hash of its
-- request (NULL when the call sent no key). Two calls with the same key at once make one
-- campaign: the loser gets no row, and reads the winner's (GetCampaignByCreateKey).
INSERT INTO campaigns (name, xp_mode, created_by, create_key, create_hash)
VALUES (sqlc.arg(name), sqlc.arg(xp_mode), sqlc.arg(created_by), sqlc.narg(create_key), sqlc.narg(create_hash))
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetCampaignByCreateKey :one
-- The campaign a CreateCampaign with this idempotency key made, if any. The key carries the
-- user's ID, so it is unique for the user.
SELECT * FROM campaigns WHERE create_key = $1;

-- name: GetCampaign :one
SELECT * FROM campaigns WHERE id = $1;

-- name: CountMasteredCampaigns :one
-- How many campaigns the user is master of, for the cap on creating (RN-30).
-- The index on campaign_members (user_id) finds the user's few rows.
SELECT count(*)::INT4 FROM campaign_members
WHERE user_id = $1 AND role = 'master';

-- name: ListCampaignsOfUser :many
-- Newest first. The index on campaign_members (user_id) finds the rows.
-- Pending memberships (RN-15) come too: the handler shows only the name.
SELECT sqlc.embed(c), m.role, m.status
FROM campaign_members AS m
JOIN campaigns AS c ON c.id = m.campaign_id
WHERE m.user_id = $1
ORDER BY c.created_at DESC, c.id;

-- name: InsertMember :one
-- pending_expires_at is NULL, except for a pending member, who has no
-- character yet: joined time + 30 days (RN-15, migration 00034).
INSERT INTO campaign_members (campaign_id, user_id, role, status, pending_expires_at)
VALUES ($1, $2, $3, $4, sqlc.narg(pending_expires_at))
RETURNING *;

-- name: GetMembership :one
-- The query behind every authorization check (package authz): one read of
-- the primary key. A pending member past the 30-day deadline (RN-15) is
-- no member, even before the daily TTL job deletes the row.
SELECT role, status FROM campaign_members
WHERE campaign_id = $1 AND user_id = $2
  AND (pending_expires_at IS NULL OR pending_expires_at > sqlc.arg(now)::timestamptz);

-- name: ListMembers :many
-- The master first, then the players in the order they joined. Pending
-- members (RN-15) are not members yet, so they are left out.
SELECT * FROM campaign_members
WHERE campaign_id = $1 AND status = 'active'
ORDER BY role = 'master' DESC, joined_at, user_id;

-- name: ActivatePendingMember :execrows
-- The master approved the pending member's character (RN-15): the
-- membership becomes an ordinary one. An active membership matches no row
-- and stays as it is.
UPDATE campaign_members
SET status = 'active', pending_expires_at = NULL
WHERE campaign_id = $1 AND user_id = $2 AND status = 'pending';

-- name: ClearPendingExpiry :execrows
-- A pending member created their character (RN-15): the master decides on
-- it now, so the 30-day deadline for a pending member without a character
-- no longer applies. A deadline that has already passed stays: that member
-- is no member any more (GetMembership).
UPDATE campaign_members
SET pending_expires_at = NULL
WHERE campaign_id = $1 AND user_id = $2 AND status = 'pending'
  AND (pending_expires_at IS NULL OR pending_expires_at > sqlc.arg(now)::timestamptz);

-- name: ListPendingMembersWithoutCharacter :many
-- The pending members who have not created a character, in the order they
-- joined. pending_expires_at is set exactly for them (migration 00034).
SELECT user_id, joined_at, pending_expires_at FROM campaign_members
WHERE campaign_id = $1 AND status = 'pending' AND pending_expires_at > sqlc.arg(now)::timestamptz
ORDER BY joined_at, user_id;

-- name: DeleteExpiredPendingMember :execrows
-- A pending member who never created a character and whose deadline has
-- passed, but whom the TTL job has not deleted yet, joins again with a new
-- invite: the stale row goes first, so the new one does not clash with it.
DELETE FROM campaign_members
WHERE campaign_id = $1 AND user_id = $2 AND status = 'pending'
  AND pending_expires_at IS NOT NULL AND pending_expires_at <= sqlc.arg(now)::timestamptz;

-- name: DeletePendingMemberWithoutCharacter :execrows
-- The master removed a pending member who has no character. The WHERE
-- clause matches only that: an active member, or a pending member whose
-- character waits for approval (pending_expires_at is NULL), is never
-- deleted here; that one goes through RejectCharacter.
DELETE FROM campaign_members
WHERE campaign_id = $1 AND user_id = $2 AND status = 'pending' AND pending_expires_at IS NOT NULL;

-- name: DeletePendingMember :execrows
-- The master rejected the pending member's character (RN-15): the pending
-- membership goes with it. status = 'pending' in the WHERE clause means an
-- active membership is never deleted here.
DELETE FROM campaign_members
WHERE campaign_id = $1 AND user_id = $2 AND status = 'pending';

-- name: InsertInvite :one
INSERT INTO campaign_invites
    (campaign_id, token_hash, created_by, max_uses, created_at, expires_at, requires_approval)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListInvites :many
-- Newest first. Invites that expired more than 30 days ago are gone (TTL).
SELECT * FROM campaign_invites
WHERE campaign_id = $1
ORDER BY created_at DESC, id;

-- name: RevokeInvite :one
-- Revoking twice keeps the first revoked_at, so RevokeInvite is idempotent.
-- An invite of another campaign matches no row, which reads as "not found".
UPDATE campaign_invites
SET revoked_at = COALESCE(revoked_at, sqlc.arg(now))
WHERE campaign_id = $1 AND id = $2
RETURNING *;

-- name: GetInviteByTokenHashForUpdate :one
-- FOR UPDATE locks the invite until the transaction ends. A second
-- AcceptInvite for the same invite waits here, instead of both reading the
-- same use_count, and then sees the first one's result.
SELECT * FROM campaign_invites WHERE token_hash = $1 FOR UPDATE;

-- name: IncrementInviteUses :execrows
-- The WHERE clause repeats the rules, so an invite is never used more than
-- max_uses times, after it expires or after it is revoked, even if the Go
-- checks before it had a bug.
UPDATE campaign_invites
SET use_count = use_count + 1
WHERE id = $1 AND use_count < max_uses AND revoked_at IS NULL AND expires_at > sqlc.arg(now);

-- name: GetCampaignDocument :one
-- The campaign's document (MR-018). No row means it was never saved: an
-- empty document at revision 0.
SELECT * FROM campaign_documents WHERE campaign_id = $1;

-- name: InsertCampaignDocument :one
-- The first save of a campaign's document, at revision 1. When someone else
-- saved first, ON CONFLICT DO NOTHING returns no row: the caller's revision
-- (0) is stale.
INSERT INTO campaign_documents (campaign_id, body, revision, updated_at, updated_by)
VALUES (sqlc.arg(campaign_id), sqlc.arg(body), 1, sqlc.arg(updated_at), sqlc.arg(updated_by))
ON CONFLICT (campaign_id) DO NOTHING
RETURNING *;

-- name: UpdateCampaignDocument :one
-- Every later save: it replaces the body only while the stored revision is
-- the one the caller read (compare-and-swap), and raises it by one. No row
-- means the revision is stale.
UPDATE campaign_documents
SET body = sqlc.arg(body),
    revision = revision + 1,
    updated_at = sqlc.arg(updated_at),
    updated_by = sqlc.arg(updated_by)
WHERE campaign_id = sqlc.arg(campaign_id) AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- name: SetCampaignDiceMode :one
-- The master changed how the campaign rolls dice (RN-18).
UPDATE campaigns SET dice_mode = $2 WHERE id = $1 RETURNING dice_mode;

-- name: SetMemberDicePreference :one
-- An active member chose how they roll (RN-18). Pending members are not
-- members yet, so no row matches them.
UPDATE campaign_members SET dice_preference = $3
WHERE campaign_id = $1 AND user_id = $2 AND status = 'active'
RETURNING dice_preference;

-- name: GetMemberDicePreference :one
SELECT dice_preference FROM campaign_members
WHERE campaign_id = $1 AND user_id = $2 AND status = 'active';

-- name: GetTableRules :one
-- The table's rules (RN-24). No row means the defaults.
SELECT * FROM campaign_table_rules WHERE campaign_id = $1;

-- name: UpsertTableRules :one
-- The master saved "Regras da mesa": every setting is written.
INSERT INTO campaign_table_rules (
    campaign_id, hit_points_rule, ability_standard_array, ability_point_buy, ability_roll_4d6, ability_typed,
    critical_rule, death_saves, combat_starts_with_map, fog_on_new_maps, house_rules, updated_at
)
VALUES (
    sqlc.arg(campaign_id), sqlc.arg(hit_points_rule), sqlc.arg(ability_standard_array), sqlc.arg(ability_point_buy),
    sqlc.arg(ability_roll_4d6), sqlc.arg(ability_typed), sqlc.arg(critical_rule), sqlc.arg(death_saves),
    sqlc.arg(combat_starts_with_map), sqlc.arg(fog_on_new_maps), sqlc.arg(house_rules)::TEXT[], sqlc.arg(now)
)
ON CONFLICT (campaign_id) DO UPDATE SET
    hit_points_rule = excluded.hit_points_rule,
    ability_standard_array = excluded.ability_standard_array,
    ability_point_buy = excluded.ability_point_buy,
    ability_roll_4d6 = excluded.ability_roll_4d6,
    ability_typed = excluded.ability_typed,
    critical_rule = excluded.critical_rule,
    death_saves = excluded.death_saves,
    combat_starts_with_map = excluded.combat_starts_with_map,
    fog_on_new_maps = excluded.fog_on_new_maps,
    house_rules = excluded.house_rules,
    updated_at = excluded.updated_at
RETURNING *;

-- name: GetCampaignForUpdate :one
-- The campaign, locked until the transaction ends: a change of the XP mode reads
-- the awards and writes the mode as one step.
SELECT * FROM campaigns WHERE id = $1 FOR UPDATE;

-- name: SetCampaignXPMode :one
-- The master changed how the campaign levels (RN-09).
UPDATE campaigns SET xp_mode = $2, xp_mode_changed_at = sqlc.arg(now)::TIMESTAMPTZ WHERE id = $1 RETURNING xp_mode, xp_mode_changed_at;
