package play

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// SignalMasterTyping implements playv1connect.PlayServiceHandler.
func (s *Service) SignalMasterTyping(
	ctx context.Context,
	req *connect.Request[playv1.SignalMasterTypingRequest],
) (*connect.Response[playv1.SignalMasterTypingResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	if _, err := s.queries.GetOpenGameSession(ctx, m.CampaignID); errors.Is(err, pgx.ErrNoRows) {
		return nil, errNoOpenSession()
	} else if err != nil {
		return nil, s.dbError(ctx, "find the open session to signal typing in", err)
	}
	// A hint that comes too soon after the last is answered and sent to nobody: the app asks
	// while the master types, and the players need it once every few seconds.
	if s.msgs.typing.take(m.CampaignID, s.now()) {
		s.send(m.CampaignID, live.Event{
			Audience: live.Audience{Players: true},
			Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_MasterTyping_{
				MasterTyping: &playv1.WatchGameSessionResponse_MasterTyping{ExpiresInSeconds: int32(typingTTL.Seconds())},
			}},
		})
	}
	return connect.NewResponse(&playv1.SignalMasterTypingResponse{}), nil
}
