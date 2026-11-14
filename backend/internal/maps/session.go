package maps

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
)

// The writes that change what a live session shows (a map, a token, the fog, a point, a door, a clue, a trap, a treasure, a scene's
// actions, an image on screen) run under play's funnel (#735): inSession takes the open session's row first, runs the write on
// that transaction, and, once it committed, moves the session's number once if the write touched and runs the steps the write
// collected (Tell.After), which tell the streams with the number. Without a session host (no play module connected) the write runs
// in a transaction of its own and its steps send through the LiveSession with no number.
//
// The lock order is the session's row first, then the map, token and character rows. A write that another module calls from its
// own transaction (the puzzle seam, the session's maps) is already under play's funnel and takes that transaction and tell.

// SetSessionHost connects play's funnel (play.Service.InSession). Without it the writes run in a transaction of their own and
// tell the streams with no number.
func (s *Service) SetSessionHost(h sessiontell.Host) { s.sessionHost = h }

// inSession runs fn on a transaction of the campaign's session, through the funnel when it is connected.
func (s *Service) inSession(ctx context.Context, campaignID string, fn func(tx pgx.Tx, t sessiontell.Tell) error) error {
	host := s.sessionHost
	if host == nil {
		host = sessiontell.Direct{Pool: s.pool, Out: func(_ context.Context, campaignID string) sessiontell.Out {
			return liveOut{s: s, campaignID: campaignID}
		}}
	}
	return host.InSession(ctx, campaignID, fn)
}

// announce is the step of a write that works out who hears of it from what it committed: it touches (the write changed what the
// screens show) and runs fn once the transaction committed, with a context whose sends carry the session's number.
func (s *Service) announce(t sessiontell.Tell, fn func(ctx context.Context)) {
	t.Touch()
	t.After(func(ctx context.Context, out sessiontell.Out) { fn(withOut(ctx, out)) })
}

// withOut is ctx for the steps of a funnel write: the publish helpers of this package send through out, so what they tell carries
// the session's number.
func withOut(ctx context.Context, out sessiontell.Out) context.Context {
	return sessiontell.WithOut(ctx, out)
}

// withoutOut is ctx for a call that leaves this write for another one (a trap's firing by play, a notice that is a write of its own): it
// keeps the values and drops the cancellation and the Out, so nothing the call tells carries the number of the write it came from.
func withoutOut(ctx context.Context) context.Context {
	return sessiontell.WithOut(context.WithoutCancel(ctx), nil)
}
