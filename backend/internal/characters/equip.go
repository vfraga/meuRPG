package characters

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Equipping and stowing gear (#562): each armor, shield and weapon the sheet lists is
// equipped or carried, and only the equipped ones count. The set of carried keys is the
// sheet's carried_keys; SetEquipped is the one call that moves a piece in or out of it
// outside the editor, and it changes nothing else on the sheet.

// equipRequest is a SetEquipped request whose shape is checked.
type equipRequest struct {
	id, key string
	equip   bool
}

// equipped is what SetEquipped's transaction leaves: the row as it stands and whether the
// call changed the sheet.
type equipped struct {
	row        charactersdb.Character
	carried    []string
	changed    bool
	changeInfo EquipChange
}

// SetEquipped implements charactersv1connect.CharacterServiceHandler.
func (s *Service) SetEquipped(
	ctx context.Context,
	req *connect.Request[charactersv1.SetEquippedRequest],
) (*connect.Response[charactersv1.SetEquippedResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	in, err := readEquipRequest(req.Msg)
	if err != nil {
		return nil, err
	}
	var done equipped
	// Under the funnel: the session's row first, then the character's, and a change of gear moves the session's number once.
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		done, err = s.equipIn(ctx, tx, m, in, t)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "equip a character's gear", err)
	}
	if done.changed {
		logging.Event(ctx, s.logger, "character.equipped", slog.String("character_id", in.id), slog.String("kind", string(done.changeInfo.Kind)), slog.Bool("equip", in.equip))
	}
	return connect.NewResponse(&charactersv1.SetEquippedResponse{CarriedKeys: done.carried, Revision: done.row.Revision}), nil
}

// readEquipRequest checks what needs no database, in order: the character's id (an id
// that is no UUID names nothing) and the key.
func readEquipRequest(msg *charactersv1.SetEquippedRequest) (equipRequest, error) {
	id, ok := uid.Canonical(msg.GetCharacterId())
	if !ok {
		return equipRequest{}, errCharacterNotFound()
	}
	if msg.GetKey() == "" {
		return equipRequest{}, invalidArgument(fieldErr("key", "is required"))
	}
	return equipRequest{id: id, key: msg.GetKey(), equip: msg.GetEquipped()}, nil
}

// equipIn is the transaction of SetEquipped: it locks the character, checks the caller
// and the key, asks play (inside the transaction, before the write) when the state really
// changes, and writes the sheet with only carried_keys changed.
//
// The lock order is play's session row, then the character row: the one order of a combat write,
// which takes the session lock first and then key-shares the character row. inSession holds the
// session's row before the character is read, since play's combat check, further on, would take it
// after the character's lock.
func (s *Service) equipIn(ctx context.Context, tx pgx.Tx, m authz.Membership, in equipRequest, t sessiontell.Tell) (equipped, error) {
	q := s.queries.WithTx(tx)
	current, err := visibleForUpdate(ctx, q, m, in.id)
	if err != nil {
		return equipped{}, err
	}
	if current.Status == statusDead {
		return equipped{}, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD, current.ID)
	}
	stored, err := loadSheet(current.ID, current.Sheet)
	if err != nil {
		return equipped{}, err
	}
	full := stored.GetFull()
	if full == nil {
		return equipped{}, invalidArgument(fieldErr("character_id", "is a character with a basic sheet, which has no gear to equip"))
	}
	change, err := equipChangeOf(full, in)
	if err != nil {
		return equipped{}, err
	}
	if slices.Contains(full.GetCarriedKeys(), in.key) == !in.equip {
		return equipped{row: current, carried: full.GetCarriedKeys()}, nil // already as asked: nothing changes
	}
	if s.reviewHost != nil {
		if err := s.reviewHost.EquipInCombat(ctx, tx, m.CampaignID, current.ID, m.UserID, change, t); err != nil {
			return equipped{}, err
		}
	}
	return s.writeCarried(ctx, tx, m, current, full, change, t)
}

// equipChangeOf names the piece of gear the request moves, or refuses a key the sheet
// does not list (the armor, a weapon, or the shield when it has one).
func equipChangeOf(full *charactersv1.FullSheet, in equipRequest) (EquipChange, error) {
	change := EquipChange{Key: in.key, Equip: in.equip}
	switch {
	case in.key == rules.ShieldKey && full.GetShield():
		change.Kind = EquipShield
	case in.key == full.GetArmorKey():
		change.Kind = EquipArmor
	case slices.Contains(full.GetWeaponKeys(), in.key):
		change.Kind = EquipWeapon
	default:
		return change, invalidArgument(fieldErr("key", "is not the armor, the shield or a weapon of the sheet"))
	}
	return change, nil
}

// writeCarried stores the sheet with the piece moved in or out of carried_keys, and
// tells play what the equipped armor and shield became.
func (s *Service) writeCarried(ctx context.Context, tx pgx.Tx, m authz.Membership, current charactersdb.Character, full *charactersv1.FullSheet, change EquipChange, t sessiontell.Tell) (equipped, error) {
	return s.setCarried(ctx, tx, m.CampaignID, current, full, change, t)
}

// setCarried is the write of writeCarried and of the undo of a swap (RestoreEquipped): the sheet with the piece moved
// in or out of carried_keys, checked, with the known issues refreshed, and what play is told of the gear. The write touches the
// session's number through t.
func (s *Service) setCarried(ctx context.Context, tx pgx.Tx, campaignID string, current charactersdb.Character, full *charactersv1.FullSheet, change EquipChange, t sessiontell.Tell) (equipped, error) {
	next := proto.CloneOf(full)
	if change.Equip {
		next.CarriedKeys = slices.DeleteFunc(next.CarriedKeys, func(k string) bool { return k == change.Key })
	} else {
		next.CarriedKeys = append(next.CarriedKeys, change.Key)
	}
	if err := rules.ValidateCarried(buildOf(next)); err != nil {
		return equipped{}, wrap("check the carried keys", err)
	}
	if err := s.refreshKnownIssues(ctx, tx, campaignID, full, next); err != nil {
		return equipped{}, err
	}
	doc, err := storeJSON.Marshal(&charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: next}})
	if err != nil {
		return equipped{}, wrap("encode a sheet", err)
	}
	row, err := s.queries.WithTx(tx).UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
		CampaignID: campaignID, ID: current.ID, Revision: current.Revision, Name: current.Name, Sheet: doc, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return equipped{}, errStaleRevision()
	}
	if err != nil {
		return equipped{}, wrap("update sheet", err)
	}
	t.Touch()
	if err := s.tellGearChange(ctx, tx, campaignID, current.ID, full, next, t); err != nil {
		return equipped{}, err
	}
	return equipped{row: row, carried: next.CarriedKeys, changed: true, changeInfo: change}, nil
}

// refreshKnownIssues keeps known_issues true to the gear in use: Derive reads only the
// equipped armor and weapons, so moving one in or out can add or end an issue (an unknown
// key). The issues Derive finds are swapped for the ones it finds now; what other flags
// the sheet holds (a changed table entry, RN-23) stays as it was.
func (s *Service) refreshKnownIssues(ctx context.Context, tx pgx.Tx, campaignID string, was, next *charactersv1.FullSheet) error {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return wrap("read rules content", err)
	}
	before := issueIDs(rules.Derive(buildOf(was), content).Issues)
	after := issueIDs(rules.Derive(buildOf(next), content).Issues)
	known := slices.DeleteFunc(slices.Clone(next.KnownIssues), func(id string) bool {
		return slices.Contains(before, id) && !slices.Contains(after, id)
	})
	for _, id := range after {
		if !slices.Contains(known, id) {
			known = append(known, id)
		}
	}
	slices.Sort(known)
	next.KnownIssues = known
	return nil
}

// RestoreEquipped implements play.CombatRoster: the master's undo of a swap puts the piece back as it was, inside the undo's
// transaction. It writes carried_keys as SetEquipped does and asks nobody about the turn and spends nothing: play
// puts the economy back itself. `failed_precondition` when the sheet no longer lists the piece or no longer allows the gear
// (the sheet changed since the swap): the undo then fails and writes nothing. What the gear tells goes through t, the undo's.
func (s *Service) RestoreEquipped(ctx context.Context, tx pgx.Tx, campaignID, characterID, key string, equip bool, t sessiontell.Tell) error {
	q := s.queries.WithTx(tx)
	current, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: campaignID, ID: characterID})
	if errors.Is(err, pgx.ErrNoRows) {
		return errCharacterNotFound()
	}
	if err != nil {
		return wrap("read character", err)
	}
	stored, err := loadSheet(current.ID, current.Sheet)
	if err != nil {
		return err
	}
	full := stored.GetFull()
	if full == nil {
		return errGearChanged()
	}
	change, err := equipChangeOf(full, equipRequest{id: characterID, key: key, equip: equip})
	if err != nil {
		return errGearChanged()
	}
	if slices.Contains(full.GetCarriedKeys(), key) == !equip {
		return nil // the piece is already as it was
	}
	if _, err := s.setCarried(ctx, tx, campaignID, current, full, change, t); err != nil {
		if connect.CodeOf(err) == connect.CodeInvalidArgument {
			return errGearChanged()
		}
		return err
	}
	return nil
}

// errGearChanged is the refusal of an undo whose piece the sheet no longer has as the swap left it.
func errGearChanged() error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("the sheet's gear changed since the swap: it cannot be undone"))
}
