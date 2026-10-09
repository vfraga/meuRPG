package characters

import (
	"context"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// PreviewCharacter implements charactersv1connect.CharacterServiceHandler.
//
// It derives a sheet that is being created or edited and writes nothing, so it
// runs no transaction: it reads through the pool. It accepts the callers and
// sheets that the save it previews accepts (CreateCharacter without a
// character ID, UpdateCharacter with one), through the same checks, so it never
// shows a sheet the save refuses and never tells anyone more than the save
// would (RN-10). The ability method (RN-24) and the portrait are not checked:
// they do not change the numbers, and the save checks them.
func (s *Service) PreviewCharacter(
	ctx context.Context,
	req *connect.Request[charactersv1.PreviewCharacterRequest],
) (*connect.Response[charactersv1.PreviewCharacterResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var sheet *charactersv1.CharacterSheet
	if req.Msg.GetCharacterId() == "" {
		kind, ok := kindToDB[req.Msg.GetKind()]
		if !ok {
			return nil, invalidArgument(fieldErr("kind", "is required"))
		}
		if err := checkCreatorKind(m, kind); err != nil {
			return nil, err
		}
		if sheet, err = checkNewSheet(content, m, kind, req.Msg.GetSheet()); err != nil {
			return nil, err
		}
	} else if sheet, err = s.checkEditedSheet(ctx, m, content, req.Msg.GetCharacterId(), req.Msg.GetSheet()); err != nil {
		return nil, err
	}
	var derived rules.Derived
	if full := sheet.GetFull(); full != nil {
		derived = rules.Derive(buildOf(full), content)
	} else {
		derived = basicDerived(content, sheet.GetBasic())
	}
	return connect.NewResponse(&charactersv1.PreviewCharacterResponse{Derived: derivedToProto(derived)}), nil
}

// checkEditedSheet is what UpdateCharacter checks before it writes, for the
// character id: the caller sees it (otherwise not_found), may edit its sheet
// now (RN-01), the sheet fits the row's kind, and no choice is new that is
// archived or switched off, judged against the stored sheet. It returns the
// checked sheet.
func (s *Service) checkEditedSheet(ctx context.Context, m authz.Membership, content *rules.Content, characterID string, raw *charactersv1.CharacterSheet) (*charactersv1.CharacterSheet, error) {
	id, ok := parseUUID(characterID)
	if !ok {
		return nil, errCharacterNotFound()
	}
	sheet, err := checkSheet(content, raw)
	if err != nil {
		return nil, invalidArgument(err)
	}
	row, err := s.queries.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
	if err != nil {
		return nil, s.dbError(ctx, "read a character to preview its sheet", err)
	}
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return nil, errCharacterNotFound()
	}
	if !mayEditSheet(m, row) {
		return nil, errBlocked(blockedReason(characterState(row.Status, row.SheetLockedAt)), row.ID)
	}
	if err := checkSheetKind(row.Kind, sheet); err != nil {
		return nil, invalidArgument(err)
	}
	stored, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return nil, s.dbError(ctx, "read a character to preview its sheet", err)
	}
	if err := refuseNewChoices(content, m, row.ID, stored.GetFull(), sheet.GetFull()); err != nil {
		return nil, err
	}
	keepInventory(row.ID, stored, sheet) // the preview wears what the character wears
	return sheet, nil
}
