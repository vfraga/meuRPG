package characters

import (
	"context"

	"connectrpc.com/connect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// ListContent implements rulesv1connect.ContentServiceHandler.
//
// It lives here, not in package campaigns as ADR-0008 sketches: the content
// comes from the ContentSource, which today gives every campaign the SRD
// snapshot that package rules embeds. The catalog is built once per content
// (catalogFor), so a campaign with its own content gets its own.
func (s *Service) ListContent(
	ctx context.Context,
	req *connect.Request[rulesv1.ListContentRequest],
) (*connect.Response[rulesv1.ListContentResponse], error) {
	// A pending member may read it too (RN-15): the editor needs it to
	// create their character. The catalog is the same for everyone, but the
	// entries the master archived: only the master receives those (RN-23).
	// With character_id, "my sheet's entries": only the caller's own sheet (RN-10), which a pending
	// member edits too (their character waits for approval, RN-15).
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	characterID := req.Msg.GetCharacterId()
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	if characterID != "" && !isMaster(m) {
		keys, err := s.ownSheetKeys(ctx, m, characterID)
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&rulesv1.ListContentResponse{
			Content: s.catalogKeeping(content, keys), TableRevision: i32(content.TableRevision()),
		}), nil
	}
	// A content's catalog is shared and never modified; marshaling it from
	// several requests at once is safe.
	return connect.NewResponse(&rulesv1.ListContentResponse{
		Content: s.catalogFor(content, isMaster(m)), TableRevision: i32(content.TableRevision()),
	}), nil
}

// ownSheetKeys are the content keys the caller's own full sheet uses; any other
// character (another player's, an NPC, a basic sheet, one that does not exist) is
// `not_found`, so the answer never says which exist. The keys of the build are enough here: the catalog lists races, sub-races,
// classes, subclasses, backgrounds and spells, and a table class feature (which callerKeys adds through the class and level) is
// none of them; the hidden spell of a feature's summon is no entry of the catalog either (a leak test holds both).
func (s *Service) ownSheetKeys(ctx context.Context, m authz.Membership, characterID string) (map[string]bool, error) {
	sheets, err := s.queries.ListCampaignSheets(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the caller's sheet", err)
	}
	for _, row := range sheets {
		if row.ID != characterID || conv.OrZero(row.PlayerUserID) != m.UserID {
			continue
		}
		sheet, err := loadSheet(row.ID, row.Sheet)
		if err != nil {
			return nil, s.dbError(ctx, "read the caller's sheet", err)
		}
		full := sheet.GetFull()
		if full == nil {
			break
		}
		out := map[string]bool{}
		for _, kf := range rules.BuildKeys(buildOf(full)) {
			out[kf.Key] = true
		}
		return out, nil
	}
	return nil, errCharacterNotFound()
}
