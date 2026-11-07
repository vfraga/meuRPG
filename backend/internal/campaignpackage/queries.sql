-- The campaign package's own tables: the exports and the uploads in progress
-- (MR-050). What a package carries belongs to the other modules' tables, which
-- their own queries read and write.

-- name: InsertCampaignExport :one
-- A new export, pending: its task claims it (ClaimCampaignExport). create_key and create_hash are the idempotency key of
-- StartCampaignExport; a retry with the same key reads the first export
-- (GetCampaignExportByCreateKey).
INSERT INTO campaign_exports (id, campaign_id, requested_by, state, create_key, create_hash, created_at, updated_at, expires_at)
VALUES (sqlc.arg(id), sqlc.arg(campaign_id), sqlc.arg(requested_by), 'pending', sqlc.narg(create_key), sqlc.narg(create_hash), sqlc.arg(now), sqlc.arg(now), sqlc.arg(expires_at))
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetCampaignExportByCreateKey :one
SELECT * FROM campaign_exports WHERE create_key = $1;

-- name: GetCampaignExport :one
SELECT * FROM campaign_exports WHERE id = $1;

-- name: GetCampaignExportForUpdate :one
SELECT * FROM campaign_exports WHERE id = $1 FOR UPDATE;

-- name: GetLatestCampaignExport :one
-- The campaign's latest export that has not expired.
SELECT * FROM campaign_exports
WHERE campaign_id = $1 AND expires_at > sqlc.arg(now)
ORDER BY created_at DESC, id
LIMIT 1;

-- name: GetRunningCampaignExport :one
SELECT * FROM campaign_exports
WHERE campaign_id = $1 AND state IN ('pending', 'running')
ORDER BY created_at DESC, id
LIMIT 1;

-- name: ClaimCampaignExport :one
-- The task takes the export: only a pending one, so a duplicate delivery gets no row.
UPDATE campaign_exports SET state = 'running', updated_at = $2
WHERE id = $1 AND state = 'pending'
RETURNING *;

-- name: SetCampaignExportProgress :exec
UPDATE campaign_exports SET percent = $2, updated_at = $3
WHERE id = $1 AND state = 'running';

-- name: FinishCampaignExport :one
-- The zip is in the store: the file lives until expires_at.
UPDATE campaign_exports
SET state = 'done', percent = 100, file_name = $2, blob_key = $3, byte_size = $4, entry_count = $5,
    updated_at = $6, finished_at = $6, expires_at = $7
WHERE id = $1 AND state = 'running'
RETURNING *;

-- name: FailCampaignExport :one
UPDATE campaign_exports
SET state = 'failed', failure = $2, updated_at = $3, finished_at = $3, expires_at = $4
WHERE id = $1 AND state IN ('pending', 'running')
RETURNING *;

-- name: CancelCampaignExport :one
UPDATE campaign_exports
SET state = 'canceled', updated_at = $2, finished_at = $2, expires_at = $3
WHERE id = $1 AND state IN ('pending', 'running')
RETURNING *;

-- name: FailStaleCampaignExports :execrows
-- A run that has not moved for a while was cut off by a restart: it will not finish.
UPDATE campaign_exports
SET state = 'failed', failure = 'interrupted', updated_at = sqlc.arg(now), finished_at = sqlc.arg(now), expires_at = sqlc.arg(expires_at)
WHERE state IN ('pending', 'running') AND updated_at < sqlc.arg(stale_before);

-- name: ListExpiredCampaignExports :many
-- What the sweeper takes: the exports past their time and the orphans, whose campaign
-- or account no longer exists (neither column has a foreign key, see migration 00205).
SELECT * FROM campaign_exports AS e
WHERE e.expires_at <= $1
   OR NOT EXISTS (SELECT 1 FROM campaigns AS c WHERE c.id = e.campaign_id)
   OR NOT EXISTS (SELECT 1 FROM users AS u WHERE u.id = e.requested_by)
ORDER BY e.expires_at LIMIT $2;

-- name: ListCampaignExportsOfCampaign :many
SELECT * FROM campaign_exports WHERE campaign_id = $1 ORDER BY id;

-- name: DeleteCampaignExport :exec
DELETE FROM campaign_exports WHERE id = $1;

-- name: DeleteCampaignExportsOfCampaign :exec
DELETE FROM campaign_exports WHERE campaign_id = $1;

-- name: DeleteCampaignExportsOfUser :exec
DELETE FROM campaign_exports WHERE requested_by = $1;

-- name: ListCampaignExportsOfUser :many
SELECT * FROM campaign_exports WHERE requested_by = $1 ORDER BY id;

-- name: GetCampaignImportOfUser :one
SELECT * FROM campaign_imports WHERE user_id = $1;

-- name: GetCampaignImport :one
SELECT * FROM campaign_imports WHERE id = $1;

-- name: InsertCampaignImport :one
INSERT INTO campaign_imports (id, user_id, file_name, fingerprint, total_bytes, part_size, part_count, created_at, updated_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9)
RETURNING *;

-- name: TouchCampaignImport :exec
-- A part arrived: the parts are kept an hour from now.
UPDATE campaign_imports SET updated_at = $2, expires_at = $3 WHERE id = $1;

-- name: UpsertCampaignImportPart :exec
INSERT INTO campaign_import_parts (import_id, part_number, byte_size, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (import_id, part_number) DO UPDATE SET byte_size = excluded.byte_size, created_at = excluded.created_at;

-- name: ListCampaignImportParts :many
SELECT * FROM campaign_import_parts WHERE import_id = $1 ORDER BY part_number;

-- name: DeleteCampaignImport :exec
-- The parts' rows go with it (ON DELETE CASCADE).
DELETE FROM campaign_imports WHERE id = $1;

-- name: ListExpiredCampaignImports :many
-- What the sweeper takes: the uploads past their time and the orphans, whose account
-- no longer exists (user_id has no foreign key, see migration 00207).
SELECT * FROM campaign_imports AS i
WHERE i.expires_at <= $1
   OR NOT EXISTS (SELECT 1 FROM users AS u WHERE u.id = i.user_id)
ORDER BY i.expires_at LIMIT $2;

-- name: ListCampaignImportsOfUser :many
SELECT * FROM campaign_imports WHERE user_id = $1 ORDER BY id;
