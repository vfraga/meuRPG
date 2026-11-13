package play

import (
	"context"
	"errors"
	"sync"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// nudgeCooldown is the shortest time between two nudges to the same character. Without it a
// master's double tap, or a stuck finger, would buzz a phone in a pocket over and over.
const nudgeCooldown = 10 * time.Second

// nudgeBook remembers, in memory, when each character was last nudged. The server runs one
// instance (docs/operations/README.md) and a nudge is ephemeral, so a restart that forgets the times
// costs nothing. The zero value is ready to use.
type nudgeBook struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// take says whether the character may be nudged at now and, if so, records it. Entries past the
// cooldown are dropped on the way, so the map never grows beyond the characters nudged lately.
func (b *nudgeBook) take(characterID string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.last == nil {
		b.last = map[string]time.Time{}
	}
	for id, at := range b.last {
		if now.Sub(at) >= nudgeCooldown {
			delete(b.last, id)
		}
	}
	if _, waiting := b.last[characterID]; waiting {
		return false
	}
	b.last[characterID] = now
	return true
}

// NudgePlayer implements playv1connect.PlayServiceHandler.
func (s *Service) NudgePlayer(
	ctx context.Context,
	req *connect.Request[playv1.NudgePlayerRequest],
) (*connect.Response[playv1.NudgePlayerResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	characterID, err := uuid.Parse(req.Msg.GetCharacterId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	if _, err := s.queries.GetOpenGameSession(ctx, m.CampaignID); errors.Is(err, pgx.ErrNoRows) {
		return nil, errNoOpenSession() // RN-02: during the session
	} else if err != nil {
		return nil, s.dbError(ctx, "find the open session to nudge in", err)
	}
	party, err := s.roster.CombatParty(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, err
	}
	var userID string
	for _, c := range party {
		if c.ID == characterID.String() {
			userID = c.PlayerUserID
		}
	}
	if userID == "" {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	if !s.nudges.take(characterID.String(), s.now()) {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("this character was called a moment ago"))
	}
	// Only that player hears it (RN-10), and it carries nothing else: the line is the app's.
	s.send(m.CampaignID, live.Event{
		Audience: live.Audience{UserID: userID},
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CallingYou_{
			CallingYou: &playv1.WatchGameSessionResponse_CallingYou{},
		}},
	})
	return connect.NewResponse(&playv1.NudgePlayerResponse{}), nil
}
