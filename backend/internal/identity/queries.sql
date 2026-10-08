-- name: InsertLoginState :exec
INSERT INTO oidc_login_states
    (state_hash, code_verifier, nonce, return_to, created_at, expires_at, intent_kind, intent_data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: TakeLoginState :one
-- Reads and deletes in one statement, so two callbacks racing with the same
-- state cannot both get it.
DELETE FROM oidc_login_states
WHERE state_hash = $1
RETURNING code_verifier, nonce, return_to, created_at, expires_at, intent_kind, intent_data;

-- name: UpdateIdentityEmail :one
UPDATE user_identities SET email = $3
WHERE issuer = $1 AND subject = $2
RETURNING user_id;

-- name: InsertUser :one
INSERT INTO users DEFAULT VALUES RETURNING id;

-- name: InsertIdentity :exec
INSERT INTO user_identities (issuer, subject, user_id, email)
VALUES ($1, $2, $3, $4);

-- name: InsertSession :one
-- A new session counts as used at the moment it starts.
INSERT INTO auth_sessions (token_hash, user_id, created_at, expires_at, auth_time, last_used_at)
VALUES ($1, $2, $3, $4, $5, $3)
RETURNING id;

-- name: LookupSession :one
-- A session ends at expires_at (30 days), or when it has not been used since
-- idle_since (now minus the idle timeout). Either way it looks like a missing
-- one. last_used_at is not in the covering index, so this reads the row.
SELECT id, user_id, created_at, expires_at, last_used_at
FROM auth_sessions
WHERE token_hash = $1 AND expires_at > sqlc.arg(now) AND last_used_at > sqlc.arg(idle_since);

-- name: SessionIsActive :one
-- A long-lived stream checks its session again by ID (RecheckSession): it
-- is still there (no sign-out, no revocation), has not expired and is not idle.
SELECT EXISTS (
    SELECT 1 FROM auth_sessions
    WHERE id = $1 AND expires_at > sqlc.arg(now) AND last_used_at > sqlc.arg(idle_since)
);

-- name: TouchSession :execrows
-- One conditional UPDATE: it writes only when the stored time is older than
-- stale_before, so the callers can try on every request and a busy session
-- still costs about one write per throttle window. A race between two
-- instances just writes twice.
UPDATE auth_sessions SET last_used_at = sqlc.arg(now)
WHERE id = $1 AND last_used_at < sqlc.arg(stale_before);

-- name: DeleteSession :exec
DELETE FROM auth_sessions WHERE id = $1;

-- name: DeleteUserSessions :exec
DELETE FROM auth_sessions WHERE user_id = $1;

-- name: DeleteOtherUserSessions :execrows
-- "Sign out of other devices": every session of the user that has not expired,
-- but the current one. Idle rows go too: an idle row is only unusable under
-- today's idle timeout, and a longer one would make it valid again.
DELETE FROM auth_sessions
WHERE user_id = $1 AND id <> sqlc.arg(keep_id)
  AND expires_at > sqlc.arg(now);

-- name: CountOtherSessions :one
-- The user's other sessions that still work.
SELECT count(*) FROM auth_sessions
WHERE user_id = $1 AND id <> sqlc.arg(keep_id)
  AND expires_at > sqlc.arg(now) AND last_used_at > sqlc.arg(idle_since);

-- name: GetDisplayName :one
SELECT display_name FROM users WHERE id = $1;

-- name: SetDisplayName :execrows
UPDATE users SET display_name = $2 WHERE id = $1;

-- name: ListVerifiedEmails :many
-- The e-mails the provider vouched for (an unverified one is never stored).
-- campaigns reads them for the allow-list of who may create campaigns (RN-30).
SELECT email::TEXT AS email
FROM user_identities
WHERE user_id = $1 AND email IS NOT NULL;

-- name: ListDisplayNames :many
-- Users without a display name are left out.
SELECT id, display_name::TEXT AS display_name
FROM users
WHERE id = ANY(sqlc.arg(ids)::UUID[]) AND display_name IS NOT NULL;
