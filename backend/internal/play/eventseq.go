package play

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// sessionEventSeqIndex is the unique index on (game_session_id, seq).
const sessionEventSeqIndex = "session_events_game_session_id_seq_key"

// insertSessionEvent is InsertSessionEvent for an event number read with NextSessionEventSeq.
//
// A change that waited for the session's row lock still reads the numbers from the snapshot it
// began with on PostgreSQL (a lock is not a write, so the snapshot is not renewed), and takes
// the number the change before it took: the unique index refuses with 23505 where CockroachDB
// answers 40001. Both mean "a concurrent change got there first", so the 23505 on that index
// comes back as a 40001 and the transaction runs again and reads the next number.
func insertSessionEvent(ctx context.Context, q *playdb.Queries, arg playdb.InsertSessionEventParams) (playdb.InsertSessionEventRow, error) {
	row, err := q.InsertSessionEvent(ctx, arg)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" && pgErr.ConstraintName == sessionEventSeqIndex {
		return row, fmt.Errorf("%w: %w", &pgconn.PgError{Code: "40001", Message: "the event number was taken by a concurrent change"}, err)
	}
	return row, err
}
