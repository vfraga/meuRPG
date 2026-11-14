-- name: InsertSessionMessage :one
-- A line of the session's conversation (MR-051). create_key and create_hash are the idempotency key
-- of SendSessionMessage and the hash of its request (NULL when none): two calls with the same key at
-- once make one line, and the loser gets no row and reads the winner's (GetSessionMessageByCreateKey).
INSERT INTO session_messages (
    campaign_id, game_session_id, author_user_id, speaker_kind, speaker_ref, speaker_name, speaker_image_id,
    text, image_id, for_everyone, create_key, create_hash, created_at
)
VALUES (
    sqlc.arg(campaign_id), sqlc.arg(game_session_id), sqlc.arg(author_user_id), sqlc.arg(speaker_kind), sqlc.narg(speaker_ref),
    sqlc.narg(speaker_name), sqlc.narg(speaker_image_id), sqlc.arg(text), sqlc.narg(image_id), sqlc.arg(for_everyone),
    sqlc.narg(create_key), sqlc.narg(create_hash), sqlc.arg(now)
)
ON CONFLICT (create_key) WHERE create_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: GetSessionMessageByCreateKey :one
SELECT * FROM session_messages WHERE create_key = $1;

-- name: InsertSessionMessageReader :exec
-- A player who reads a line that is not for everyone. The master and the author read it without a row.
INSERT INTO session_message_readers (message_id, user_id)
VALUES ($1, $2)
ON CONFLICT (message_id, user_id) DO NOTHING;

-- name: ListSessionMessageReaders :many
-- The readers of the given lines (only lines that are not for everyone have rows).
SELECT message_id, user_id FROM session_message_readers
WHERE message_id = ANY(sqlc.arg(message_ids)::UUID[])
ORDER BY message_id, user_id;

-- name: ListSessionMessages :many
-- A page of a session's lines the user may read (RN-36), newest first: the master reads all, a player
-- the lines for everyone, the ones that chose them and their own. before_id, when set, is the oldest line
-- of the page already read; the caller has checked that the user may read it. The same visibility rule is
-- in GetSessionMessageForReader, SessionImageInReadableMessage and ListSessionMessagesForSummary.
SELECT m.* FROM session_messages m
WHERE m.game_session_id = sqlc.arg(game_session_id)
  AND (
      sqlc.arg(is_master)::BOOL OR m.for_everyone OR m.author_user_id = sqlc.arg(user_id)::UUID
      OR EXISTS (SELECT 1 FROM session_message_readers r WHERE r.message_id = m.id AND r.user_id = sqlc.arg(user_id)::UUID)
  )
  AND (
      sqlc.narg(before_id)::UUID IS NULL
      OR (m.created_at, m.id) < (SELECT b.created_at, b.id FROM session_messages b WHERE b.id = sqlc.narg(before_id)::UUID)
  )
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_size);

-- name: GetSessionMessageForReader :one
-- One line of the campaign, only if the user may read it (RN-36): no row for a line that does not exist,
-- is another campaign's, or is not the user's to read. The voice route and the paging cursor use it, so
-- both answer a line the user may not read exactly like one that does not exist.
SELECT m.* FROM session_messages m
WHERE m.id = sqlc.arg(id) AND m.campaign_id = sqlc.arg(campaign_id)
  AND (
      sqlc.arg(is_master)::BOOL OR m.for_everyone OR m.author_user_id = sqlc.arg(user_id)::UUID
      OR EXISTS (SELECT 1 FROM session_message_readers r WHERE r.message_id = m.id AND r.user_id = sqlc.arg(user_id)::UUID)
  );

-- name: ImageInReadableMessage :one
-- Whether a line the user may read carries the image, or has it as the portrait of the NPC that speaks
-- it (RN-36): that is the one thing a line lets its readers open.
SELECT EXISTS (
    SELECT 1 FROM session_messages m
    WHERE m.campaign_id = sqlc.arg(campaign_id)
      AND (m.image_id = sqlc.arg(image_id) OR m.speaker_image_id = sqlc.arg(image_id))
      AND (
          m.for_everyone OR m.author_user_id = sqlc.arg(user_id)::UUID
          OR EXISTS (SELECT 1 FROM session_message_readers r WHERE r.message_id = m.id AND r.user_id = sqlc.arg(user_id)::UUID)
      )
)::BOOL AS readable;

-- name: CountSessionMessagesFrom :one
-- How many voices were made since the given instant, for the server-wide daily cap.
SELECT count(*) FROM session_messages WHERE audio_made_at >= $1;

-- name: MarkSessionMessageVoiced :execrows
UPDATE session_messages SET audio_made_at = $2 WHERE id = $1 AND audio_made_at IS NULL;

-- name: GetSessionMessage :one
-- One line by id, whatever its readers, only to find its campaign: the voice route then checks the
-- caller's membership and reads the line again with GetSessionMessageForReader.
SELECT * FROM session_messages WHERE id = $1;
