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
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
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

// equipped is what SetEquipped's transaction leaves: the row as it stands, whether the
// call changed the sheet, and what play asks to run after the commit.
type equipped struct {
	row         charactersdb.Character
	carried     []string
	changed     bool
	afterGear   func(ctx context.Context)
	afterCombat func(ctx context.Context)
	changeInfo  EquipChange
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
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		done, err = s.equipIn(ctx, tx, m, in)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "equip a character's gear", err)
	}
	if done.changed {
		logging.Event(ctx, s.logger, "character.equipped", slog.String("character_id", in.id), slog.String("kind", string(done.changeInfo.Kind)), slog.Bool("equip", in.equip))
		for _, after := range []func(context.Context){done.afterCombat, done.afterGear} {
			if after != nil {
				after(ctx)
			}
		}
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
// The lock order is the character row, then play's session lock. A combat write takes the
// session lock first and then key-shares the character row, so a concurrent attack and equip
// can deadlock; db.InTx retries the one the database aborts (40P01), as for ReviveCharacter.
func (s *Service) equipIn(ctx context.Context, tx pgx.Tx, m authz.Membership, in equipRequest) (equipped, error) {
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
	var afterCombat func(context.Context)
	if s.reviewHost != nil {
		if afterCombat, err = s.reviewHost.EquipInCombat(ctx, tx, m.CampaignID, current.ID, m.UserID, change); err != nil {
			return equipped{}, err
		}
	}
	done, err := s.writeCarried(ctx, tx, m, current, full, change)
	done.afterCombat = afterCombat
	return done, err
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
func (s *Service) writeCarried(ctx context.Context, tx pgx.Tx, m authz.Membership, current charactersdb.Character, full *charactersv1.FullSheet, change EquipChange) (equipped, error) {
	return s.setCarried(ctx, tx, m.CampaignID, current, full, change)
}

// setCarried is the write of writeCarried and of the undo of a swap (RestoreEquipped): the sheet with the piece moved
// in or out of carried_keys, checked, with the known issues refreshed, and what play is told of the gear.
func (s *Service) setCarried(ctx context.Context, tx pgx.Tx, campaignID string, current charactersdb.Character, full *charactersv1.FullSheet, change EquipChange) (equipped, error) {
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
	after, err := s.tellGearChange(ctx, tx, campaignID, current.ID, full, next)
	if err != nil {
		return equipped{}, err
	}
	return equipped{row: row, carried: next.CarriedKeys, changed: true, afterGear: after, changeInfo: change}, nil
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
// (the sheet changed since the swap): the undo then fails and writes nothing. It returns what to call after the commit.
func (s *Service) RestoreEquipped(ctx context.Context, tx pgx.Tx, campaignID, characterID, key string, equip bool) (func(context.Context), error) {
	q := s.queries.WithTx(tx)
	current, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: campaignID, ID: characterID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errCharacterNotFound()
	}
	if err != nil {
		return nil, wrap("read character", err)
	}
	stored, err := loadSheet(current.ID, current.Sheet)
	if err != nil {
		return nil, err
	}
	full := stored.GetFull()
	if full == nil {
		return nil, errGearChanged()
	}
	change, err := equipChangeOf(full, equipRequest{id: characterID, key: key, equip: equip})
	if err != nil {
		return nil, errGearChanged()
	}
	if slices.Contains(full.GetCarriedKeys(), key) == !equip {
		return nil, nil // the piece is already as it was
	}
	done, err := s.setCarried(ctx, tx, campaignID, current, full, change)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeInvalidArgument {
			return nil, errGearChanged()
		}
		return nil, err
	}
	return done.afterGear, nil
}

// errGearChanged is the refusal of an undo whose piece the sheet no longer has as the swap left it.
func errGearChanged() error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("the sheet's gear changed since the swap: it cannot be undone"))
}
