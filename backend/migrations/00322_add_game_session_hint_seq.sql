-- +goose Up
-- hint_seq counts the writes of a game session that told the live streams something (or must move the screens without
-- telling them what). Every transaction that sends a hint, or only touches the session, adds 1 to it in the same
-- transaction as the write (play's Service.tx), so the number and the write are saved together or not at all. A stream that
-- sees a number it has not applied knows it missed a hint.
-- It counts transactions, never events and never content. It holds no personal data.
--
-- Production is live: the column only adds, every existing row starts at 0 and no constraint is named, so running the
-- migration twice is safe.
ALTER TABLE game_sessions
    ADD COLUMN IF NOT EXISTS hint_seq BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE game_sessions
    DROP COLUMN IF EXISTS hint_seq;
