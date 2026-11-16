package play

import (
	"context"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// GetFavor implements playv1connect.FavorServiceHandler: the master reads every holder of the campaign, a player reads only the
// holders among their own characters, and a member with none reads an empty list. It needs no open session.
func (s *Service) GetFavor(ctx context.Context, req *connect.Request[playv1.GetFavorRequest]) (*connect.Response[playv1.GetFavorResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListCampaignFavor(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list who has the inspiration", err)
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.CharacterID
	}
	// The names are the characters module's: the query cannot join its table.
	chars, err := s.roster.SessionCharacters(ctx, nil, m.CampaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "name who has the inspiration", err)
	}
	byID := make(map[string]link.Character, len(chars))
	for _, c := range chars {
		byID[c.ID] = c
	}
	holders := make([]*playv1.FavorHolder, 0, len(rows))
	for _, r := range rows {
		if c, ok := byID[r.CharacterID]; ok && (m.Role == authz.RoleMaster || (c.PlayerUserID == m.UserID && !c.Reserved)) {
			holders = append(holders, &playv1.FavorHolder{CharacterId: c.ID, CharacterName: c.Name})
		}
	}
	passes, err := s.readPasses(ctx, m, ids)
	if err != nil {
		return nil, s.dbError(ctx, "list the asks to pass the inspiration", err)
	}
	var prompts []*playv1.FavorRefundPrompt
	if m.Role == authz.RoleMaster {
		if prompts, err = s.readRefundPrompts(ctx, m); err != nil {
			return nil, s.dbError(ctx, "list the refund questions", err)
		}
	}
	return connect.NewResponse(&playv1.GetFavorResponse{Holders: holders, Passes: passes, RefundPrompts: prompts}), nil
}

// readPasses are the pending passes the caller reads: the master all of them, with receiver_has (who has the Inspiration now, which is
// the ids), and a player only the asks of their own characters, with receiver_has never sent (the receiver's state is not a player's
// to read). The names come from the characters module.
func (s *Service) readPasses(ctx context.Context, m authz.Membership, holding []string) ([]*playv1.FavorPass, error) {
	rows, err := s.queries.ListPendingFavorPasses(ctx, m.CampaignID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	ids := make([]string, 0, 2*len(rows))
	for _, p := range rows {
		ids = append(ids, p.FromCharacterID, p.ToCharacterID)
	}
	chars, err := s.roster.SessionCharacters(ctx, nil, m.CampaignID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]link.Character, len(chars))
	names := make(map[string]string, len(chars))
	for _, c := range chars {
		byID[c.ID], names[c.ID] = c, c.Name
	}
	var out []*playv1.FavorPass
	for _, p := range rows {
		giver, ok := byID[p.FromCharacterID]
		if m.Role == authz.RoleMaster {
			out = append(out, favorPassProto(p, names, slices.Contains(holding, p.ToCharacterID)))
		} else if ok && giver.PlayerUserID == m.UserID && !giver.Reserved {
			out = append(out, favorPassProto(p, names, false))
		}
	}
	return out, nil
}
