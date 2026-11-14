package progression

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/wiring"
)

// The writes that change what a live session shows (an award, its undo) run under play's funnel (#735): the open session's row is
// taken first, then the sheets and the campaign, in the order of every other change of the session; the session's number moves once
// and the xp_changed hint carries it. A retry of a key changes nothing and moves nothing.

// SetSessionHost connects play's funnel (play.Service.InSession). Without it the writes run in a transaction of their own and tell
// the streams nothing.
func (s *Service) SetSessionHost(h sessiontell.Host) { s.sessionHost = h }

// inSession runs fn on a transaction of the campaign's session, through the funnel when it is connected.
func (s *Service) inSession(ctx context.Context, campaignID string, fn func(tx pgx.Tx, t sessiontell.Tell) error) error {
	host := s.sessionHost
	if host == nil {
		host = sessiontell.Direct{Pool: s.pool}
	}
	return host.InSession(ctx, campaignID, fn)
}

// tellXP is what a write of XP tells when the open session logged it: the party's experience changed. A retry tells nothing new.
func tellXP(t sessiontell.Tell, repeated, logged bool) {
	switch {
	case repeated:
		t.Replay()
	case logged:
		t.XPChanged()
	}
}

// CheckWired fails when a collaborator that has a setter is still nil (see platform/wiring).
func (s *Service) CheckWired() error {
	return wiring.Check("progression", wiring.Dep{Setter: "SetSessionHost", Missing: s.sessionHost == nil})
}
