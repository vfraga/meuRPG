package characters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The phases of CreateCharacter: the request read and checked before the transaction
// (readNewCharacter), then the transaction that checks again what may have moved and
// inserts the row (insertNewCharacter).

// newCharacter is a CreateCharacter request read and checked before its transaction:
// what the transaction checks again and writes. content and sheet become the ones the
// transaction reads when the table's content moved in between, and the response is
// built with that content.
type newCharacter struct {
	m       authz.Membership
	msg     *charactersv1.CreateCharacterRequest
	kind    string
	content *rules.Content
	sheet   *charactersv1.CharacterSheet
	params  charactersdb.InsertCharacterParams
}

// readNewCharacter reads and checks a CreateCharacter request, in the order the client
// is answered: the kind and who makes it, the ability method, the name, the sheet, the
// story, then the idempotency key.
func (s *Service) readNewCharacter(ctx context.Context, m authz.Membership, msg *charactersv1.CreateCharacterRequest) (*newCharacter, error) {
	kind, name, err := checkNewCharacterRequest(m, msg)
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	nc := &newCharacter{m: m, msg: msg, kind: kind, content: content}
	if nc.sheet, err = s.checkNewCharacterSheet(ctx, nc); err != nil {
		return nil, err
	}
	story, err := checkStory(msg.GetStory())
	if err != nil {
		return nil, invalidArgument(err)
	}
	if err := s.newCharacterParams(ctx, nc, name, story); err != nil {
		return nil, err
	}
	// The key is unique per caller in the campaign (the unique index holds the key scoped to
	// the caller, so another member's key is a different one), and kept with a hash of the
	// whole request: a retry returns the first character only when it is the same request.
	if nc.params.CreateKey, err = newCharacterKey(m, msg); err != nil {
		return nil, err
	}
	nc.params.CreateHash = idem.Hash(msg)
	return nc, nil
}

// checkNewCharacterRequest checks what a CreateCharacter request asks for before its
// sheet: the kind, that the caller may make it, the ability method and the name.
func checkNewCharacterRequest(m authz.Membership, msg *charactersv1.CreateCharacterRequest) (kind, name string, err error) {
	kind, ok := kindToDB[msg.GetKind()]
	if !ok {
		return "", "", invalidArgument(fieldErr("kind", "is required"))
	}
	if err := checkCreatorKindFor(m, kind, msg.GetForPlayer()); err != nil {
		return "", "", err
	}
	if _, known := charactersv1.AbilityMethod_name[int32(msg.GetAbilityMethod())]; !known {
		return "", "", invalidArgument(fieldErr("ability_method", "is not a known method"))
	}
	name, err = names.Clean(msg.GetName(), MaxNameLength)
	if err != nil {
		return "", "", invalidArgument(&fieldError{field: "name", err: err})
	}
	return kind, name, nil
}

// checkNewCharacterSheet checks the sheet of a new character with the content read
// before the transaction, and returns the sheet to store.
func (s *Service) checkNewCharacterSheet(ctx context.Context, nc *newCharacter) (*charactersv1.CharacterSheet, error) {
	sheet, err := checkNewSheet(nc.content, nc.m, nc.kind, nc.msg.GetSheet())
	if err != nil {
		return nil, err
	}
	if err := refuseChoices(nc.content, nc.m, nc.kind, sheet.GetFull(), nil); err != nil {
		return nil, err
	}
	if err := refuseFeatChange(nc.m, nil, sheet.GetFull()); err != nil {
		return nil, err
	}
	if err := s.checkPortrait(ctx, nc.m.CampaignID, nc.kind, sheet); err != nil {
		return nil, invalidArgument(err)
	}
	return sheet, nil
}

// newCharacterParams fills the row the transaction inserts, but for the create key.
func (s *Service) newCharacterParams(ctx context.Context, nc *newCharacter, name string, story *charactersv1.CharacterStory) error {
	m := nc.m
	nc.params = charactersdb.InsertCharacterParams{
		CampaignID: m.CampaignID, Kind: nc.kind, Status: newCharacterStatus(m), Name: name, Now: s.now(),
	}
	var err error
	if nc.params.Sheet, err = storeJSON.Marshal(nc.sheet); err != nil {
		return s.dbError(ctx, "encode a sheet", err)
	}
	if nc.params.Story, err = storeJSON.Marshal(story); err != nil {
		return s.dbError(ctx, "encode a story", err)
	}
	switch {
	case nc.msg.GetForPlayer():
		nc.params.Reserved = true // no owner until a player claims it (MR-049)
	case nc.kind == kindPlayer:
		nc.params.PlayerUserID = &m.UserID
	default:
		nc.params.MasterUserID = &m.UserID // the NPC is the master's (RN-04)
	}
	return nil
}

// newCharacterKey is the create key of the request scoped to the caller, nil without a
// key.
func newCharacterKey(m authz.Membership, msg *charactersv1.CreateCharacterRequest) (*string, error) {
	key, err := idem.Clean(msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, nil
	}
	// The column is a UUID: the key of this create is the one the app made for the form.
	if _, ok := parseUUID(key); !ok {
		return nil, invalidArgument(fieldErr("idempotency_key", "must be a UUID"))
	}
	scoped := callerCreateKey(m.UserID, key)
	return &scoped, nil
}

// insertNewCharacter is CreateCharacter's transaction: the row of the key when the same
// request made it before, or a new one, checked again with the content the transaction
// reads.
func (s *Service) insertNewCharacter(ctx context.Context, nc *newCharacter) (charactersdb.Character, error) {
	var row charactersdb.Character
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var replayed bool
		var err error
		row, replayed, err = idem.Create(ctx, nc.params.CreateKey, nc.params.CreateHash,
			func(ctx context.Context, k *string) (charactersdb.Character, error) {
				return q.GetCharacterByCreateKey(ctx, charactersdb.GetCharacterByCreateKeyParams{CampaignID: nc.m.CampaignID, CreateKey: *k})
			},
			func(c charactersdb.Character) *string { return c.CreateHash },
			func() (charactersdb.Character, error) { return s.insertCharacterIn(ctx, tx, q, nc) })
		if err != nil || replayed {
			return err // a replay: the first call made the character and cleared the pending deadline
		}
		// A pending member without a character is removed after 30 days.
		// Now that they have one, the master decides on it, so the deadline
		// goes, in this same transaction: the character and the lack of a
		// deadline appear together or not at all.
		if nc.m.Pending {
			return s.members.ClearPendingExpiry(ctx, tx, nc.m.CampaignID, nc.m.UserID)
		}
		return nil
	})
	if isUniqueViolation(err, "characters_one_living_player_character") {
		// Another CreateCharacter of the same player won the race: answer
		// as the check in the transaction would have.
		return row, s.livingCharacterRaced(ctx, nc.m)
	}
	if err != nil {
		return row, s.dbError(ctx, "create a character", err)
	}
	return row, nil
}

// insertCharacterIn checks the new character again inside the transaction and inserts
// it. It returns pgx.ErrNoRows when the create key's row is already there (idem.Create).
func (s *Service) insertCharacterIn(ctx context.Context, tx pgx.Tx, q *charactersdb.Queries, nc *newCharacter) (charactersdb.Character, error) {
	// The table's content and rules, read in this transaction: if the content
	// moved since it was read above (an archive, an edit), the sheet is checked
	// again with it, so a write is always ordered against a content write
	// (ADR-0018, section 5).
	tc, tr, err := s.content.For(ctx, tx, nc.m.CampaignID)
	if err != nil {
		return charactersdb.Character{}, wrap("read the table content and rules", err)
	}
	if err := nc.recheck(tc); err != nil {
		return charactersdb.Character{}, err
	}
	nc.content = tc
	if nc.kind == kindPlayer && !nc.msg.GetForPlayer() {
		if err := s.checkPlayersNewSheet(ctx, q, nc, tr); err != nil {
			return charactersdb.Character{}, err
		}
	}
	if err := s.checkRoom(ctx, q, nc.m.CampaignID, false); err != nil {
		return charactersdb.Character{}, err
	}
	row, err := q.InsertCharacter(ctx, nc.params)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return row, wrap("insert character", err)
	}
	return row, err
}

// recheck checks the sheet again with tc, the content the transaction read, when it is
// not the one the sheet was checked with.
func (nc *newCharacter) recheck(tc *rules.Content) error {
	if tc.TableRevision() == nc.content.TableRevision() {
		return nil
	}
	again, err := checkNewSheet(tc, nc.m, nc.kind, nc.sheet)
	if err != nil {
		return err
	}
	if err := refuseChoices(tc, nc.m, nc.kind, again.GetFull(), nil); err != nil {
		return err
	}
	nc.sheet = again
	if nc.params.Sheet, err = storeJSON.Marshal(nc.sheet); err != nil {
		return wrap("encode a sheet", err)
	}
	return nil
}

// checkPlayersNewSheet is what a player's own new character must also follow, inside the
// transaction: the way the base scores were made and the table allows (RN-24), the
// table's hit point rule, the multiclass prerequisites, and one living character per
// player (RN-03). It records the scores' origin on the sheet and spends the stored 4d6
// the sheet used.
func (s *Service) checkPlayersNewSheet(ctx context.Context, q *charactersdb.Queries, nc *newCharacter, tr TableRules) error {
	full := nc.sheet.GetFull()
	origin, err := s.checkAbilityScores(ctx, q, nc.m, nc.content, tr, nc.msg.GetAbilityMethod(), full)
	if err != nil {
		return err
	}
	if err := hitPointsRuleRefusal(tr, full); err != nil {
		return err
	}
	if err := refuseMulticlassGap(nc.content, nc.m, nil, full); err != nil {
		return err
	}
	// The server records how the scores were made (never the client).
	full.AbilityOrigin = origin
	if nc.params.Sheet, err = storeJSON.Marshal(nc.sheet); err != nil {
		return wrap("encode a sheet", err)
	}
	if origin.GetMethod() == charactersv1.AbilityMethod_ABILITY_METHOD_ROLLED_4D6 {
		// The stored 4d6 belong to this sheet now: the next character rolls anew.
		if err := q.DeleteAbilityRolls(ctx, charactersdb.DeleteAbilityRollsParams{CampaignID: nc.m.CampaignID, UserID: nc.m.UserID}); err != nil {
			return wrap("use the ability rolls", err)
		}
	}
	return refuseSecondLivingCharacter(ctx, q, nc.m)
}

// refuseSecondLivingCharacter is RN-03: one living character per player per campaign.
// This check gives the clear error; the unique index is what makes it hold when two
// calls race (livingCharacterRaced).
func refuseSecondLivingCharacter(ctx context.Context, q *charactersdb.Queries, m authz.Membership) error {
	living, err := q.GetLivingPlayerCharacterID(ctx, charactersdb.GetLivingPlayerCharacterIDParams{
		CampaignID: m.CampaignID, PlayerUserID: m.UserID,
	})
	switch {
	case err == nil:
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS, living)
	case !errors.Is(err, pgx.ErrNoRows):
		return wrap("find the living character", err)
	}
	return nil
}

// livingCharacterRaced is the answer when the unique index of the living characters
// refused the insert: another CreateCharacter of the same player won the race. It reads
// the winner after the transaction.
func (s *Service) livingCharacterRaced(ctx context.Context, m authz.Membership) error {
	living, lookupErr := s.queries.GetLivingPlayerCharacterID(ctx, charactersdb.GetLivingPlayerCharacterIDParams{
		CampaignID: m.CampaignID, PlayerUserID: m.UserID,
	})
	if lookupErr != nil {
		living = "" // it died in between; the reason still holds
	}
	return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS, living)
}
