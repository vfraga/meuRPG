package characters

import (
	"context"
	"errors"
	"log/slog"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Every handler starts with one explicit check: authz.RequireCampaignMember
// when any member may call it, authz.RequireCampaignRole when only the
// master may, and authz.RequireCampaignMemberOrPending for the calls a
// pending member may make too (RN-15: CreateCharacter, GetCharacter,
// ListCharacters, UpdateCharacter, UpdateCharacterStory and ListContent).
// The check's error is already the right Connect error, so handlers return
// it as is. What a player, or a pending member, may see and change is
// decided next, from the character's row: canSee, playerEditsSheet and
// playerEditsStory (access.go).

// callerKeyNamespace is the namespace of the create keys scoped to a caller.
var callerKeyNamespace = uuid.MustParse("b3a1c0f2-5d7e-4a49-8c6b-2e91f4d07a35")

// callerCreateKey is the create_key of a character made by userID with the client's key: a
// version 5 UUID of both, so the same caller and key always give the same value (a retry
// finds the first character) and another caller's identical key gives another one. The
// column stays a UUID, as the unique index (campaign_id, create_key) needs no change.
func callerCreateKey(userID, key string) string {
	return nameUUID(callerKeyNamespace, userID+":"+key)
}

// CreateCharacter implements charactersv1connect.CharacterServiceHandler.
func (s *Service) CreateCharacter(
	ctx context.Context,
	req *connect.Request[charactersv1.CreateCharacterRequest],
) (*connect.Response[charactersv1.CreateCharacterResponse], error) {
	// A pending member creates their character here (RN-15); authz makes
	// them a player, so the kind check applies to them too.
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	nc, err := s.readNewCharacter(ctx, m, req.Msg)
	if err != nil {
		return nil, err
	}
	row, err := s.insertNewCharacter(ctx, nc)
	if err != nil {
		return nil, err
	}
	logging.Event(ctx, s.logger, "character.created", slog.String("character_id", row.ID), slog.String("kind", row.Kind), slog.String("status", row.Status))
	c, err := s.character(ctx, nc.content, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a new character", err)
	}
	return connect.NewResponse(&charactersv1.CreateCharacterResponse{Character: c}), nil
}

// GetCharacter implements charactersv1connect.CharacterServiceHandler.
func (s *Service) GetCharacter(
	ctx context.Context,
	req *connect.Request[charactersv1.GetCharacterRequest],
) (*connect.Response[charactersv1.GetCharacterResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	row, err := s.queries.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
	if err != nil {
		return nil, s.dbError(ctx, "get a character", err)
	}
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return nil, errCharacterNotFound()
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	c, err := s.character(ctx, content, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a character", err)
	}
	return connect.NewResponse(&charactersv1.GetCharacterResponse{Character: c}), nil
}

// ListCharacters implements charactersv1connect.CharacterServiceHandler.
func (s *Service) ListCharacters(
	ctx context.Context,
	req *connect.Request[charactersv1.ListCharactersRequest],
) (*connect.Response[charactersv1.ListCharactersResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	params := listParams(m)
	now := s.now()
	rows, err := s.queries.ListCharacters(ctx, params)
	if err != nil {
		return nil, s.dbError(ctx, "list characters", err)
	}
	l, err := s.readCharacterList(ctx, m, rows)
	if err != nil {
		return nil, err
	}
	res := &charactersv1.ListCharactersResponse{}
	for _, row := range rows {
		summary, err := l.summary(row, now)
		if err != nil {
			return nil, s.dbError(ctx, "list characters", err)
		}
		res.Characters = append(res.Characters, summary)
	}
	return connect.NewResponse(res), nil
}

// UpdateCharacter implements charactersv1connect.CharacterServiceHandler.
func (s *Service) UpdateCharacter(
	ctx context.Context,
	req *connect.Request[charactersv1.UpdateCharacterRequest],
) (*connect.Response[charactersv1.UpdateCharacterResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	u, err := s.readSheetSave(ctx, m, req.Msg)
	if err != nil {
		return nil, err
	}
	if err := s.preparePortraitSave(ctx, u); err != nil {
		return nil, err
	}
	row, err := s.writeSheetSave(ctx, u)
	if err != nil {
		return nil, err
	}
	c, err := s.character(ctx, u.content, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read an updated character", err)
	}
	return connect.NewResponse(&charactersv1.UpdateCharacterResponse{Character: c}), nil
}

// UpdateCharacterStory implements charactersv1connect.CharacterServiceHandler.
func (s *Service) UpdateCharacterStory(
	ctx context.Context,
	req *connect.Request[charactersv1.UpdateCharacterStoryRequest],
) (*connect.Response[charactersv1.UpdateCharacterStoryResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, revision, storyDoc, err := s.readStorySave(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID) // before the write: a failure after the commit would make the client retry it
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var row charactersdb.Character
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		row, err = s.saveStoryIn(ctx, s.queries.WithTx(tx), m, id, revision, storyDoc)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "update a story", err)
	}
	c, err := s.character(ctx, content, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read an updated character", err)
	}
	return connect.NewResponse(&charactersv1.UpdateCharacterStoryResponse{Character: c}), nil
}

// readStorySave reads and checks an UpdateCharacterStory request, in the order the
// client is answered: the character's ID, the revision, then the story, which it
// returns encoded.
func (s *Service) readStorySave(ctx context.Context, msg *charactersv1.UpdateCharacterStoryRequest) (id string, revision int32, storyDoc []byte, err error) {
	id, ok := parseUUID(msg.GetCharacterId())
	if !ok {
		return "", 0, nil, errCharacterNotFound()
	}
	revision = msg.GetRevision()
	if revision < 1 {
		return "", 0, nil, invalidArgument(fieldErr("revision", "must be at least 1"))
	}
	story, err := checkStory(msg.GetStory())
	if err != nil {
		return "", 0, nil, invalidArgument(err)
	}
	storyDoc, err = storeJSON.Marshal(story)
	if err != nil {
		return "", 0, nil, s.dbError(ctx, "encode a story", err)
	}
	return id, revision, storyDoc, nil
}

// saveStoryIn is UpdateCharacterStory's transaction: the stored character locked and
// checked, then the story written.
func (s *Service) saveStoryIn(ctx context.Context, q *charactersdb.Queries, m authz.Membership, id string, revision int32, storyDoc []byte) (charactersdb.Character, error) {
	current, err := visibleForUpdate(ctx, q, m, id)
	if err != nil {
		return current, err
	}
	// The story's own lock: the player edits it while the character is
	// a draft, and afterwards only while the master allows it (RN-01).
	state := characterState(current.Status, current.SheetLockedAt)
	if !isMaster(m) && !playerEditsStory(state, current.StoryEditingAllowed) {
		return current, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_STORY_LOCKED, current.ID)
	}
	if current.Revision != revision {
		return current, errStaleRevision()
	}
	row, err := q.UpdateCharacterStory(ctx, charactersdb.UpdateCharacterStoryParams{
		CampaignID: m.CampaignID, ID: id, Revision: revision, Story: storyDoc, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, errStaleRevision()
	}
	if err != nil {
		return row, wrap("update story", err)
	}
	return row, nil
}

// SetStoryEditing implements charactersv1connect.CharacterServiceHandler.
func (s *Service) SetStoryEditing(
	ctx context.Context,
	req *connect.Request[charactersv1.SetStoryEditingRequest],
) (*connect.Response[charactersv1.SetStoryEditingResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	allowed := req.Msg.GetAllowed()

	content, err := s.contentFor(ctx, nil, m.CampaignID) // before the write: a failure after the commit would make the client retry it
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var row charactersdb.Character
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		row, err = visibleForUpdate(ctx, q, m, id)
		if err != nil {
			return err
		}
		if row.Kind != kindPlayer {
			return invalidArgument(fieldErr("character_id", "is an NPC: the master always edits an NPC's story"))
		}
		if row.Reserved {
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_RESERVED, row.ID)
		}
		if row.StoryEditingAllowed == allowed {
			return nil // already so: nothing to change
		}
		row, err = q.SetStoryEditing(ctx, charactersdb.SetStoryEditingParams{CampaignID: m.CampaignID, ID: id, Allowed: allowed})
		if err != nil {
			return wrap("set story editing", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set story editing", err)
	}
	c, err := s.character(ctx, content, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a character", err)
	}
	return connect.NewResponse(&charactersv1.SetStoryEditingResponse{Character: c}), nil
}

// MarkCharacterDead implements charactersv1connect.CharacterServiceHandler.
func (s *Service) MarkCharacterDead(
	ctx context.Context,
	req *connect.Request[charactersv1.MarkCharacterDeadRequest],
) (*connect.Response[charactersv1.MarkCharacterDeadResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}

	content, err := s.contentFor(ctx, nil, m.CampaignID) // before the write: a failure after the commit would make the client retry it
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var row charactersdb.Character
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		current, err := visibleForUpdate(ctx, q, m, id)
		if err != nil {
			return err
		}
		if current.Kind != kindPlayer {
			return invalidArgument(fieldErr("character_id", "is an NPC: only player characters die"))
		}
		// A character waiting for approval is not in the campaign yet
		// (RN-15): the master approves or rejects it instead.
		if current.Status == statusPending {
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_AWAITING_APPROVAL, current.ID)
		}
		if current.Reserved {
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_RESERVED, current.ID)
		}
		// RN-03: the character changes status, never row.
		row, err = q.MarkCharacterDead(ctx, charactersdb.MarkCharacterDeadParams{CampaignID: m.CampaignID, ID: id, Now: s.now()})
		if err != nil {
			return wrap("mark dead", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "mark a character dead", err)
	}
	c, err := s.character(ctx, content, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a character", err)
	}
	return connect.NewResponse(&charactersv1.MarkCharacterDeadResponse{Character: c}), nil
}

// GetMasterNotes implements charactersv1connect.CharacterServiceHandler.
func (s *Service) GetMasterNotes(
	ctx context.Context,
	req *connect.Request[charactersv1.GetMasterNotesRequest],
) (*connect.Response[charactersv1.GetMasterNotesResponse], error) {
	// RN-11: only the master, and the notes never ride in any other
	// response.
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	exists, err := s.queries.CharacterIsInCampaign(ctx, charactersdb.CharacterIsInCampaignParams{CampaignID: m.CampaignID, ID: id})
	if err != nil {
		return nil, s.dbError(ctx, "find a character", err)
	}
	if !exists {
		return nil, errCharacterNotFound()
	}
	notes, err := s.queries.GetMasterNotes(ctx, charactersdb.GetMasterNotesParams{CampaignID: m.CampaignID, CharacterID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewResponse(&charactersv1.GetMasterNotesResponse{}), nil // none written
	}
	if err != nil {
		return nil, s.dbError(ctx, "get master notes", err)
	}
	return connect.NewResponse(&charactersv1.GetMasterNotesResponse{
		Notes:     notes.Notes,
		UpdatedAt: timestamppb.New(notes.UpdatedAt),
	}), nil
}

// UpdateMasterNotes implements charactersv1connect.CharacterServiceHandler.
func (s *Service) UpdateMasterNotes(
	ctx context.Context,
	req *connect.Request[charactersv1.UpdateMasterNotesRequest],
) (*connect.Response[charactersv1.UpdateMasterNotesResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	text, err := names.CleanText(req.Msg.GetNotes(), MaxMasterNotesLength)
	if err != nil {
		return nil, invalidArgument(&fieldError{field: "notes", err: err})
	}

	res := &charactersv1.UpdateMasterNotesResponse{}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		exists, err := q.CharacterIsInCampaign(ctx, charactersdb.CharacterIsInCampaignParams{CampaignID: m.CampaignID, ID: id})
		if err != nil {
			return wrap("find the character", err)
		}
		if !exists {
			return errCharacterNotFound()
		}
		if text == "" {
			// Empty notes are deleted, not stored.
			res.Notes, res.UpdatedAt = "", nil
			if err := q.DeleteMasterNotes(ctx, charactersdb.DeleteMasterNotesParams{CampaignID: m.CampaignID, CharacterID: id}); err != nil {
				return wrap("delete notes", err)
			}
			return nil
		}
		saved, err := q.UpsertMasterNotes(ctx, charactersdb.UpsertMasterNotesParams{
			CampaignID: m.CampaignID, CharacterID: id, Notes: text, UpdatedAt: s.now(),
		})
		if err != nil {
			return wrap("save notes", err)
		}
		res.Notes, res.UpdatedAt = saved.Notes, timestamppb.New(saved.UpdatedAt)
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "update master notes", err)
	}
	return connect.NewResponse(res), nil
}

// checkCreatorKindFor is checkCreatorKind with the master's for_player: the master makes
// a PLAYER character for a player to claim (MR-049), which no one else may and which
// no other kind may be.
func checkCreatorKindFor(m authz.Membership, kind string, forPlayer bool) error {
	if !forPlayer {
		return checkCreatorKind(m, kind)
	}
	if !isMaster(m) {
		return errPermission("only the campaign's master makes a character for a player")
	}
	if kind != kindPlayer {
		return invalidArgument(fieldErr("for_player", "needs the kind PLAYER"))
	}
	return nil
}

// checkCreatorKind is who creates what (RN-04): a player creates their own
// character and the master creates NPCs; a master is not a player of their own
// campaign. CreateCharacter and PreviewCharacter share it.
func checkCreatorKind(m authz.Membership, kind string) error {
	switch {
	case kind == kindPlayer && isMaster(m):
		return errPermission("only a player creates a player character; the master creates NPCs")
	case kind != kindPlayer && !isMaster(m):
		return errPermission("only the campaign's master creates NPCs")
	}
	return nil
}

// checkNewSheet checks the sheet of a character about to be created, with the
// table's content, and returns the sheet to store: the sheet's own rules, the
// sheet that fits the kind, the ability origin cleared (the server's record,
// set later for a player), and no new choice of an archived or switched-off
// entry. The errors are the ones CreateCharacter answers. CreateCharacter and
// PreviewCharacter share it, so a preview never shows a sheet the save
// refuses.
func checkNewSheet(content *rules.Content, m authz.Membership, kind string, raw *charactersv1.CharacterSheet) (*charactersv1.CharacterSheet, error) {
	sheet, err := checkSheet(content, raw)
	if err == nil {
		err = checkSheetKind(kind, sheet)
	}
	if err != nil {
		return nil, invalidArgument(err)
	}
	if sheet.GetFull() != nil {
		sheet.GetFull().AbilityOrigin = nil // the server's record, set later for a player
	}
	if err := refuseNewChoices(content, m, "", nil, sheet.GetFull()); err != nil {
		return nil, err
	}
	return sheet, nil
}

// refuseNewChoices refuses a choice the sheet adds to the stored one (nil when
// creating; characterID is empty then): a table entry the master archived is
// not a new choice (RN-23), and neither is one the master switched off, except
// for the master. The ones the stored sheet already has stay.
func refuseNewChoices(content *rules.Content, m authz.Membership, characterID string, stored, sheet *charactersv1.FullSheet) error {
	if key, _, found := newArchivedChoice(content, stored, sheet); found {
		return errArchivedChoice(characterID, key)
	}
	if key, _, found := newOffChoice(content, stored, sheet); found && !isMaster(m) {
		return errOffChoice(characterID, key)
	}
	return nil
}

// mayEditSheet is RN-01: after a game session starts, only the master edits the
// sheet. It comes before the revision: retrying would not help.
func mayEditSheet(m authz.Membership, row charactersdb.Character) bool {
	return isMaster(m) || playerEditsSheet(characterState(row.Status, row.SheetLockedAt))
}

// visibleForUpdate reads a character inside a transaction, locked until it
// ends, or answers not_found when it is not in the campaign or the caller
// may not see it.
func visibleForUpdate(ctx context.Context, q *charactersdb.Queries, m authz.Membership, id string) (charactersdb.Character, error) {
	row, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: m.CampaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, errCharacterNotFound()
	}
	if err != nil {
		return row, wrap("read character", err)
	}
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return row, errCharacterNotFound()
	}
	return row, nil
}

// character builds the Character response for a row the caller may see,
// with its player's display name.
func (s *Service) character(ctx context.Context, content *rules.Content, row charactersdb.Character, m authz.Membership) (*charactersv1.Character, error) {
	var displayName string
	if row.PlayerUserID != nil {
		displayNames, err := s.displayNames(ctx, []string{*row.PlayerUserID})
		if err != nil {
			return nil, err
		}
		displayName = displayNames[*row.PlayerUserID]
	}
	c, err := s.characterToProto(content, row, m, displayName)
	if err != nil {
		return nil, err
	}
	if err := s.fillLevelUp(ctx, c, row, m); err != nil {
		return nil, err
	}
	if err := s.fillReview(ctx, c, row, m); err != nil {
		return nil, err
	}
	return c, nil
}

// displayNames asks the identity module for players' display names.
func (s *Service) displayNames(ctx context.Context, userIDs []string) (map[string]string, error) {
	if len(userIDs) == 0 {
		return map[string]string{}, nil
	}
	displayNames, err := s.profiles.DisplayNames(ctx, userIDs)
	if err != nil {
		return nil, wrap("read display names", err)
	}
	return displayNames, nil
}
