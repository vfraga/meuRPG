package characters

import (
	"context"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
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
	r := reader{master: isMaster(m)} // a sheet being made has no name yet: its sentences say "o personagem"
	if req.Msg.GetCharacterId() == "" {
		kind, ok := kindToDB[req.Msg.GetKind()]
		if !ok {
			return nil, invalidArgument(fieldErr("kind", "is required"))
		}
		if err := checkCreatorKindFor(m, kind, req.Msg.GetForPlayer()); err != nil {
			return nil, err
		}
		if sheet, err = checkNewSheet(content, m, kind, req.Msg.GetSheet()); err != nil {
			return nil, err
		}
	} else if sheet, r.name, err = s.checkEditedSheet(ctx, m, content, req.Msg.GetCharacterId(), req.Msg.GetSheet()); err != nil {
		return nil, err
	}
	var derived rules.Derived
	if full := sheet.GetFull(); full != nil {
		if derived, err = s.previewDerived(ctx, content, req.Msg.GetCharacterId(), full); err != nil {
			return nil, s.dbError(ctx, "read the infusions of a character to preview its sheet", err)
		}
	} else {
		derived = basicDerived(content, sheet.GetBasic())
	}
	return connect.NewResponse(&charactersv1.PreviewCharacterResponse{Derived: r.derived(derived)}), nil
}

// previewDerived derives the full sheet a preview shows. The sheet of a character that exists is derived with the
// character's live infusion records, as the save would show it; a sheet being made has none.
func (s *Service) previewDerived(ctx context.Context, content *rules.Content, characterID string, full *charactersv1.FullSheet) (rules.Derived, error) {
	build := buildOf(full)
	if id, ok := uid.Canonical(characterID); ok {
		var err error
		if build, err = s.withInfusions(ctx, s.queries, id, build); err != nil {
			return rules.Derived{}, err
		}
	}
	return rules.Derive(build, content), nil
}

// checkEditedSheet is what UpdateCharacter checks before it writes, for the
// character id: the caller sees it (otherwise not_found), may edit its sheet
// now (RN-01), the sheet fits the row's kind, and no choice is new that is
// archived or switched off, judged against the stored sheet. It returns the
// checked sheet.
func (s *Service) checkEditedSheet(ctx context.Context, m authz.Membership, content *rules.Content, characterID string, raw *charactersv1.CharacterSheet) (*charactersv1.CharacterSheet, string, error) {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return nil, "", errCharacterNotFound()
	}
	sheet, err := checkSheet(content, raw)
	if err != nil {
		return nil, "", invalidArgument(err)
	}
	// The stored row is read once, whenever a check first needs it; the fit of the picks to their lists is
	// refuseNewChoices's, after the permission answers and the answers that name a closed entry.
	stored := s.storedReadOf(ctx, m, id)
	if err := refuseKeptOutPick(content, m, stored.Visible(), sheet.GetFull()); err != nil {
		return nil, "", err
	}
	row, err := stored.Row()
	if err != nil {
		return nil, "", s.dbError(ctx, "read a character to preview its sheet", err)
	}
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return nil, "", errCharacterNotFound()
	}
	if !mayEditSheet(m, row) {
		return nil, "", errBlocked(blockedReason(characterState(row.Status, row.SheetLockedAt)), row.ID)
	}
	if err := checkSheetKind(row.Kind, sheet); err != nil {
		return nil, "", invalidArgument(err)
	}
	storedSheet, err := stored.Sheet()
	if err != nil {
		return nil, "", s.dbError(ctx, "read a character to preview its sheet", err)
	}
	if err := refuseNewChoices(content, m, row.ID, storedSheet.GetFull(), sheet.GetFull()); err != nil {
		return nil, "", err
	}
	return sheet, row.Name, nil
}
