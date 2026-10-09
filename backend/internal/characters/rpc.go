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
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
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
	// them a player, so the kind check below applies to them too.
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	kind, ok := kindToDB[req.Msg.GetKind()]
	if !ok {
		return nil, invalidArgument(fieldErr("kind", "is required"))
	}
	if err := checkCreatorKind(m, kind); err != nil {
		return nil, err
	}

	if _, known := charactersv1.AbilityMethod_name[int32(req.Msg.GetAbilityMethod())]; !known {
		return nil, invalidArgument(fieldErr("ability_method", "is not a known method"))
	}
	name, err := names.Clean(req.Msg.GetName(), MaxNameLength)
	if err != nil {
		return nil, invalidArgument(&fieldError{field: "name", err: err})
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	sheet, err := checkNewSheet(content, m, kind, req.Msg.GetSheet())
	if err != nil {
		return nil, err
	}
	if err := s.checkPortrait(ctx, m.CampaignID, kind, sheet); err != nil {
		return nil, invalidArgument(err)
	}
	story, err := checkStory(req.Msg.GetStory())
	if err != nil {
		return nil, invalidArgument(err)
	}
	params := charactersdb.InsertCharacterParams{
		CampaignID: m.CampaignID, Kind: kind, Status: newCharacterStatus(m), Name: name, Now: s.now(),
	}
	if params.Sheet, err = storeJSON.Marshal(sheet); err != nil {
		return nil, s.dbError(ctx, "encode a sheet", err)
	}
	if params.Story, err = storeJSON.Marshal(story); err != nil {
		return nil, s.dbError(ctx, "encode a story", err)
	}
	if kind == kindPlayer {
		params.PlayerUserID = &m.UserID
	} else {
		params.MasterUserID = &m.UserID // the NPC is the master's (RN-04)
	}

	key, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err == nil && key != "" {
		// The column is a UUID: the key of this create is the one the app made for the form.
		if _, ok := parseUUID(key); !ok {
			err = invalidArgument(fieldErr("idempotency_key", "must be a UUID"))
		}
	}
	if err != nil {
		return nil, err
	}
	// The key is unique per caller in the campaign (the unique index holds the key scoped to
	// the caller, so another member's key is a different one), and kept with a hash of the
	// whole request: a retry returns the first character only when it is the same request.
	var createKey *string
	if key != "" {
		scoped := callerCreateKey(m.UserID, key)
		createKey = &scoped
	}
	requestHash := idem.Hash(req.Msg)
	params.CreateKey, params.CreateHash = createKey, requestHash

	var row charactersdb.Character
	replayed := false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		row, replayed, err = idem.Create(ctx, createKey, requestHash,
			func(ctx context.Context, k *string) (charactersdb.Character, error) {
				return q.GetCharacterByCreateKey(ctx, charactersdb.GetCharacterByCreateKeyParams{CampaignID: m.CampaignID, CreateKey: *k})
			},
			func(c charactersdb.Character) *string { return c.CreateHash },
			func() (charactersdb.Character, error) {
				// The table's content and rules, read in this transaction: if the content
				// moved since it was read above (an archive, an edit), the sheet is checked
				// again with it, so a write is always ordered against a content write
				// (ADR-0018, section 5).
				tc, tr, err := s.content.For(ctx, tx, m.CampaignID)
				if err != nil {
					return row, wrap("read the table content and rules", err)
				}
				if tc.TableRevision() != content.TableRevision() {
					again, err := checkNewSheet(tc, m, kind, sheet)
					if err != nil {
						return row, err
					}
					sheet = again
					if params.Sheet, err = storeJSON.Marshal(sheet); err != nil {
						return row, wrap("encode a sheet", err)
					}
				}
				content = tc
				if kind == kindPlayer {
					// The base scores follow the way the player made them, and the table
					// allows it (RN-24).
					origin, err := s.checkAbilityScores(ctx, q, m, content, tr, req.Msg.GetAbilityMethod(), sheet.GetFull())
					if err != nil {
						return row, err
					}
					if err := hitPointsRuleRefusal(tr, sheet.GetFull()); err != nil {
						return row, err
					}
					// The server records how the scores were made (never the client).
					sheet.GetFull().AbilityOrigin = origin
					if params.Sheet, err = storeJSON.Marshal(sheet); err != nil {
						return row, wrap("encode a sheet", err)
					}
					if origin.GetMethod() == charactersv1.AbilityMethod_ABILITY_METHOD_ROLLED_4D6 {
						// The stored 4d6 belong to this sheet now: the next character rolls anew.
						if err := q.DeleteAbilityRolls(ctx, charactersdb.DeleteAbilityRollsParams{CampaignID: m.CampaignID, UserID: m.UserID}); err != nil {
							return row, wrap("use the ability rolls", err)
						}
					}
					// RN-03: one living character per player per campaign. This
					// check gives the clear error; the unique index is what makes it
					// hold when two calls race (below).
					living, err := q.GetLivingPlayerCharacterID(ctx, charactersdb.GetLivingPlayerCharacterIDParams{
						CampaignID: m.CampaignID, PlayerUserID: m.UserID,
					})
					switch {
					case err == nil:
						return row, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS, living)
					case !errors.Is(err, pgx.ErrNoRows):
						return row, wrap("find the living character", err)
					}
				}
				if err := s.checkRoom(ctx, q, m.CampaignID, false); err != nil {
					return row, err
				}
				row, err := q.InsertCharacter(ctx, params)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return row, wrap("insert character", err)
				}
				return row, err
			})
		if err != nil || replayed {
			return err // a replay: the first call made the character and cleared the pending deadline
		}
		// A pending member without a character is removed after 30 days.
		// Now that they have one, the master decides on it, so the deadline
		// goes, in this same transaction: the character and the lack of a
		// deadline appear together or not at all.
		if m.Pending {
			return s.members.ClearPendingExpiry(ctx, tx, m.CampaignID, m.UserID)
		}
		return nil
	})
	if isUniqueViolation(err, "characters_one_living_player_character") {
		// Another CreateCharacter of the same player won the race: answer
		// as the check above would have.
		living, lookupErr := s.queries.GetLivingPlayerCharacterID(ctx, charactersdb.GetLivingPlayerCharacterIDParams{
			CampaignID: m.CampaignID, PlayerUserID: m.UserID,
		})
		if lookupErr != nil {
			living = "" // it died in between; the reason still holds
		}
		return nil, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS, living)
	}
	if err != nil {
		return nil, s.dbError(ctx, "create a character", err)
	}
	logging.Event(ctx, s.logger, "character.created", slog.String("character_id", row.ID), slog.String("kind", row.Kind), slog.String("status", row.Status))
	c, err := s.character(ctx, content, row, m)
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
	params := charactersdb.ListCharactersParams{CampaignID: m.CampaignID}
	if !isMaster(m) {
		params.PlayerUserID = &m.UserID // a player sees only their own
	}
	if m.Pending {
		pending := statusPending
		params.Status = &pending // and a pending member only their pending one (canSee)
	}
	rows, err := s.queries.ListCharacters(ctx, params)
	if err != nil {
		return nil, s.dbError(ctx, "list characters", err)
	}

	var playerIDs []string
	for _, row := range rows {
		if row.PlayerUserID != nil {
			playerIDs = append(playerIDs, *row.PlayerUserID)
		}
	}
	displayNames, err := s.displayNames(ctx, playerIDs)
	if err != nil {
		return nil, s.dbError(ctx, "read display names", err)
	}

	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	res := &charactersv1.ListCharactersResponse{}
	for _, row := range rows {
		summary := &charactersv1.CharacterSummary{
			Id:                row.ID,
			Kind:              kindFromDB[row.Kind],
			State:             characterState(row.Status, row.SheetLockedAt),
			Name:              row.Name,
			PlayerUserId:      deref(row.PlayerUserID),
			PlayerDisplayName: displayNames[deref(row.PlayerUserID)],
			CreatedAt:         timestamppb.New(row.CreatedAt),
		}
		sheet, err := loadSheet(row.ID, row.Sheet)
		if err != nil {
			return nil, s.dbError(ctx, "list characters", err)
		}
		if row.Kind != kindPlayer {
			if id := portraitOf(sheet); id != "" {
				summary.PortraitUrl = "/images/" + id
			}
		}
		if full := sheet.GetFull(); full != nil {
			labels := content.Summary(buildOf(full))
			summary.ClassSummary = labels.ClassSummaryPT
			summary.RaceNamePt = labels.RaceNamePT
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
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	revision := req.Msg.GetRevision()
	if revision < 1 {
		return nil, invalidArgument(fieldErr("revision", "must be at least 1"))
	}
	name, err := names.Clean(req.Msg.GetName(), MaxNameLength)
	if err != nil {
		return nil, invalidArgument(&fieldError{field: "name", err: err})
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	sheet, err := checkSheet(content, req.Msg.GetSheet())
	if err != nil {
		return nil, invalidArgument(err)
	}
	// The portrait is prepared before the transaction: a fog map's background is
	// copied (its files now, its gallery row inside the transaction). It is
	// prepared only for a caller the transaction would let save the sheet: the
	// same checks, in the same order (visible, RN-01, kind), so nobody else makes
	// the server copy files. A caller who fails one skips the preparation, and the
	// transaction answers with that check's error. A character's kind never
	// changes, so the row read here is the one the transaction reads. A read that
	// fails is the request's failure: an unchecked portrait never goes through.
	var portraitErr error
	var portraitCopy PortraitCopy
	var copyCreated bool // the copy's row is the one this call inserted
	if portraitOf(sheet) != "" {
		pre, err := s.queries.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// No such character: the transaction says so.
		case err != nil:
			return nil, s.dbError(ctx, "read a character to check its portrait", err)
		case canSee(m, pre.Kind, pre.Status, pre.PlayerUserID) && mayEditSheet(m, pre) && checkSheetKind(pre.Kind, sheet) == nil:
			var failure error
			if portraitCopy, portraitErr, failure = s.preparePortrait(ctx, m.CampaignID, pre.Kind, sheet); failure != nil {
				return nil, failure
			}
		}
	}

	var row charactersdb.Character
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		current, err := visibleForUpdate(ctx, q, m, id)
		if err != nil {
			return err
		}
		// RN-01: after a game session starts, only the master edits the
		// sheet. This comes before the revision: retrying would not help.
		if !mayEditSheet(m, current) {
			return errBlocked(blockedReason(characterState(current.Status, current.SheetLockedAt)), current.ID)
		}
		if err := checkSheetKind(current.Kind, sheet); err != nil {
			return invalidArgument(err)
		}
		if portraitErr != nil {
			return invalidArgument(portraitErr)
		}
		if current.Revision != revision {
			return errStaleRevision()
		}
		// The table's content, read in this transaction: if it moved since it was
		// read above (an archive, an edit), the sheet is checked again with it, so a
		// write is always ordered against a content write (ADR-0018, section 5).
		tc, err := s.contentFor(ctx, tx, m.CampaignID)
		if err != nil {
			return wrap("read rules content", err)
		}
		if tc.TableRevision() != content.TableRevision() {
			if sheet, err = checkSheet(tc, sheet); err != nil {
				return invalidArgument(err)
			}
		}
		content = tc
		storedSheet, err := loadSheet(current.ID, current.Sheet)
		if err != nil {
			return err
		}
		if err := refuseNewChoices(content, m, current.ID, storedSheet.GetFull(), sheet.GetFull()); err != nil {
			return err
		}
		// The ability origin is the server's: kept from the stored sheet, and a
		// player's draft edit of the base scores is checked against it (RN-24).
		if err := keepAbilityOrigin(m, content, storedSheet, sheet); err != nil {
			return err
		}
		// The inventory is the server's too: the stored one stays, and the equipment
		// lines the client sent join it as free text.
		keepInventory(current.ID, storedSheet, sheet)
		// A save never clears an issue that a change of the table's content flagged
		// and that still stands.
		carryFlags(content, storedSheet.GetFull(), sheet.GetFull())
		copyCreated = false
		if portraitCopy != nil { // the copy's gallery row, in this transaction
			id, created, err := portraitCopy.Insert(ctx, tx)
			if err != nil {
				return err
			}
			copyCreated = created
			setPortrait(sheet, id)
		}
		// An NPC made from a creature keeps the link to it (MR-042): the
		// client never sends it, so it is carried over from the saved sheet.
		sheetDoc, err := storeJSON.Marshal(keepCreatureLink(current, sheet))
		if err != nil {
			return wrap("encode a sheet", err)
		}
		row, err = q.UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
			CampaignID: m.CampaignID, ID: id, Revision: revision, Name: name, Sheet: sheetDoc, Now: s.now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStaleRevision()
		}
		if err != nil {
			return wrap("update sheet", err)
		}
		return s.carryHitPoints(ctx, q, content, id, current.Sheet, sheetDoc)
	})
	if portraitCopy != nil && (err != nil || !copyCreated) {
		portraitCopy.Discard(ctx) // no gallery row: the copy's files go
	}
	if err != nil {
		return nil, s.dbError(ctx, "update a character", err)
	}
	c, err := s.character(ctx, content, row, m)
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
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	revision := req.Msg.GetRevision()
	if revision < 1 {
		return nil, invalidArgument(fieldErr("revision", "must be at least 1"))
	}
	story, err := checkStory(req.Msg.GetStory())
	if err != nil {
		return nil, invalidArgument(err)
	}
	storyDoc, err := storeJSON.Marshal(story)
	if err != nil {
		return nil, s.dbError(ctx, "encode a story", err)
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
		// The story's own lock: the player edits it while the character is
		// a draft, and afterwards only while the master allows it (RN-01).
		state := characterState(current.Status, current.SheetLockedAt)
		if !isMaster(m) && !playerEditsStory(state, current.StoryEditingAllowed) {
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_STORY_LOCKED, current.ID)
		}
		if current.Revision != revision {
			return errStaleRevision()
		}
		row, err = q.UpdateCharacterStory(ctx, charactersdb.UpdateCharacterStoryParams{
			CampaignID: m.CampaignID, ID: id, Revision: revision, Story: storyDoc, Now: s.now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStaleRevision()
		}
		if err != nil {
			return wrap("update story", err)
		}
		return nil
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
		keepInventory("", nil, sheet)       // equipment lines become the inventory's free text
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
