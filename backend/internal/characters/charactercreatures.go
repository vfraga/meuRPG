package characters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The character's creatures (MR-037, Etapa 9): the familiar, the animals and
// undead a spell summoned, the creatures the master gave. A creature belongs
// to a player's character and lasts, from one session to the next, until the
// player or the master dismisses it. This file has the CharacterService
// methods and the storage the play module reaches through play.CombatRoster
// (casting a summoning spell, joining a combat, concentration ending, a
// combat writing hit points back). The rules (what a spell may summon, a
// creature's numbers) are package rules's; this file only stores and checks
// who may.
//
// Who sees what (RN-20): the master and the creature's owner's player read a
// creature and its hit points; for everyone else the creature is not found, as
// a character they may not see is.

// maxCreaturesPerCharacter is how many creatures a character keeps at a time:
// a sanity limit above what Conjurar Animais with a 9th circle slot (8
// creatures, four times: 32) and a familiar need together.
const maxCreaturesPerCharacter = 40

// maxCreatureName is the longest creature name, in characters.
const maxCreatureName = 40

// The database's values for a creature's source and dismissal reason
// (character_creatures_source_valid, character_creatures_dismissed_valid).
const (
	creatureSourceFamiliar = "familiar"
	creatureSourceMaster   = "master"
)

var creatureSourceToProto = map[string]charactersv1.CreatureSource{
	"familiar":           charactersv1.CreatureSource_CREATURE_SOURCE_FAMILIAR,
	"animate_dead":       charactersv1.CreatureSource_CREATURE_SOURCE_ANIMATE_DEAD,
	"conjure_animals":    charactersv1.CreatureSource_CREATURE_SOURCE_CONJURE_ANIMALS,
	creatureSourceMaster: charactersv1.CreatureSource_CREATURE_SOURCE_MASTER,
}

// creatureAttackNumber is the attack permission as the API's number
// (meurpg.play.v1.CreatureAttack): 1 none, 2 reaction, 3 full.
var creatureAttackNumber = map[string]int32{"none": 1, "reaction": 2, "full": 3}

// CreatureHost is what the characters module needs from the play module to keep
// creatures and combats in step (MR-037). cmd/api connects play.Service
// (SetCreatureHost); without it, a creature still lives on the list and
// nothing reaches a combat, the history or the stream.
type CreatureHost interface {
	// CreaturesLeaving takes the combatants of these creatures out of the
	// campaign's combat that is not ended, inside tx, and writes the combat's
	// event. It returns the combat's ID, empty when none of them was in one.
	CreaturesLeaving(ctx context.Context, tx pgx.Tx, campaignID, actorUserID string, creatureIDs []string, at time.Time) (encounterID string, err error)
	// LockSession takes the campaign's open session's row inside tx, if there is
	// one. A change to a creature calls it first, as a combat's writes lock the
	// session before the creatures: one lock order, no deadlock.
	LockSession(ctx context.Context, tx pgx.Tx, campaignID string) error
	// CreatureRenamed gives the creature's combatant its new name in the
	// campaign's combat that is not ended, inside tx; it returns that combat's ID,
	// empty when the creature is in none.
	CreatureRenamed(ctx context.Context, tx pgx.Tx, campaignID, creatureID, name string) (encounterID string, err error)
	// CreatureInCombat says whether the creature is a combatant of the
	// campaign's combat that is not ended, inside tx.
	CreatureInCombat(ctx context.Context, tx pgx.Tx, campaignID, creatureID string) (bool, error)
	// AppendEvent appends an event to the history of the campaign's open
	// session inside tx; false when no session is open.
	AppendEvent(ctx context.Context, tx pgx.Tx, campaignID, kind, actorUserID string, payload []byte, at time.Time) (bool, error)
	// PublishCreaturesChanged tells the master and the owner's player that
	// their creature lists changed. Call it after the commit.
	PublishCreaturesChanged(campaignID, ownerUserID string)
	// PublishEncounterChanged tells every stream that the combat changed.
	// Call it after the commit.
	PublishEncounterChanged(ctx context.Context, campaignID, encounterID string)
	// CampaignInCombat says whether the campaign has a combat that is not ended,
	// inside tx: attuning to an item and changing body armor wait for a moment out
	// of one.
	CampaignInCombat(ctx context.Context, tx pgx.Tx, campaignID string) (bool, error)
	// PublishInventoryChanged tells the master and the character's player that the
	// inventory changed, with no content (RN-10). Call it after the commit.
	PublishInventoryChanged(campaignID, characterID, ownerUserID string)
	// ItemEvents returns the inventory's history in the campaign's open session,
	// newest first, for the master's item log.
	ItemEvents(ctx context.Context, campaignID string, limit int) ([]link.ItemEvent, error)
}

// SetCreatureHost connects the play module.
func (s *Service) SetCreatureHost(h CreatureHost) { s.creatureHost = h }

// creatureView is a creature row with its owner's player.
type creatureView struct {
	charactersdb.CharacterCreature
	ownerUserID *string
}

func (v creatureView) live() bool { return v.DismissedAt == nil }

// visibleTo says whether the caller may see the creature: the master, or the
// owner's player.
func (v creatureView) visibleTo(m authz.Membership) bool {
	return isMaster(m) || (v.ownerUserID != nil && *v.ownerUserID == m.UserID)
}

// characterCreatureToProto builds the CharacterCreature the caller sees. The caller may
// see it (visibleTo).
func characterCreatureToProto(content *rules.Content, c charactersdb.CharacterCreature) *charactersv1.CharacterCreature {
	return &charactersv1.CharacterCreature{
		Id: c.ID, CharacterId: c.CharacterID, MonsterKey: c.MonsterKey, MonsterNamePt: content.NamePT(c.MonsterKey), Name: c.Name,
		Source: creatureSourceToProto[c.Source], Attack: creatureAttackNumber[c.Attack], SummonGroupId: c.SummonGroupID,
		DependsOnConcentration: c.ConcentrationCastID != nil,
		HitPointsCurrent:       c.HpCurrent, HitPointsMax: c.HpMax, CreatedAt: timestamppb.New(c.CreatedAt),
	}
}

// creatureOf is the link.Creature of a row: the numbers a combat copies come
// from the stat block (its Dexterity modifier, its best speed).
func creatureOf(content *rules.Content, v creatureView) link.Creature {
	out := link.Creature{
		ID: v.ID, CharacterID: v.CharacterID, OwnerUserID: deref(v.ownerUserID), MonsterKey: v.MonsterKey, Name: v.Name,
		Source: v.Source, Attack: v.Attack, GroupID: v.SummonGroupID, DependsOnConcentration: v.ConcentrationCastID != nil,
		HitPointsCurrent: int(v.HpCurrent), HitPointsMax: int(v.HpMax),
	}
	if d, ok := content.MonsterDerived(v.MonsterKey); ok {
		out.InitiativeBonus = d.Initiative
		// It walks, or flies when it can (the movement uses the better of the two);
		// swimming and climbing speeds are never the speed on the map.
		out.SpeedFt, out.SpeedFlyFt = d.SpeedWalkFt, d.SpeedFlyFt
		jumps := combat.JumpLimits(d)
		out.JumpLongDFt, out.JumpHighDFt = jumps.LongRunning, jumps.HighRunning
	}
	if c, ok := content.CreatureByKey(v.MonsterKey); ok {
		out.Size = strings.ToLower(c.Size)
	}
	return out
}

func viewOfRow(r charactersdb.GetCharacterCreatureRow) creatureView {
	return creatureView{CharacterCreature: r.CharacterCreature, ownerUserID: r.PlayerUserID}
}

// findCreature reads a creature of the campaign that the caller may see and
// that is still with its owner; not found otherwise. With lock, it locks the
// row.
func (s *Service) findCreature(ctx context.Context, q *charactersdb.Queries, m authz.Membership, rawID string, lock bool) (creatureView, error) {
	id, ok := parseUUID(rawID)
	if !ok {
		return creatureView{}, errCreatureNotFound()
	}
	var v creatureView
	if lock {
		row, err := q.GetCharacterCreatureForUpdate(ctx, charactersdb.GetCharacterCreatureForUpdateParams{CampaignID: m.CampaignID, ID: id})
		if err != nil {
			return creatureView{}, notFoundOr(err, "read a creature")
		}
		v = creatureView{CharacterCreature: row.CharacterCreature, ownerUserID: row.PlayerUserID}
	} else {
		row, err := q.GetCharacterCreature(ctx, charactersdb.GetCharacterCreatureParams{CampaignID: m.CampaignID, ID: id})
		if err != nil {
			return creatureView{}, notFoundOr(err, "read a creature")
		}
		v = viewOfRow(row)
	}
	if !v.live() || !v.visibleTo(m) {
		return creatureView{}, errCreatureNotFound()
	}
	return v, nil
}

func notFoundOr(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errCreatureNotFound()
	}
	return wrap(what, err)
}

func errCreatureNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("creature not found"))
}

// cleanCreatureName checks a creature's name: 1 to 40 characters on one line.
func cleanCreatureName(raw string) (string, error) {
	name, err := names.Clean(raw, maxCreatureName)
	if err != nil {
		return "", invalidArgument(fieldErr("name", "%s", err.Error()))
	}
	return name, nil
}

// lockSession takes the session's lock before any creature row (see
// CreatureHost.LockSession).
func (s *Service) lockSession(ctx context.Context, tx pgx.Tx, campaignID string) error {
	if s.creatureHost == nil {
		return nil
	}
	return s.creatureHost.LockSession(ctx, tx, campaignID)
}

// ListCharacterCreatures implements charactersv1connect.CharacterServiceHandler.
func (s *Service) ListCharacterCreatures(
	ctx context.Context,
	req *connect.Request[charactersv1.ListCharacterCreaturesRequest],
) (*connect.Response[charactersv1.ListCharacterCreaturesResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	row, err := s.queries.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errCharacterNotFound()
	}
	if err != nil {
		return nil, s.dbError(ctx, "read a character", err)
	}
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return nil, errCharacterNotFound()
	}
	if row.Kind != kindPlayer {
		return nil, invalidArgument(fieldErr("character_id", "is an NPC: only a player's character has creatures"))
	}
	rows, err := s.queries.ListLiveCreaturesOfCharacters(ctx, charactersdb.ListLiveCreaturesOfCharactersParams{CampaignID: m.CampaignID, CharacterIds: []string{id}})
	if err != nil {
		return nil, s.dbError(ctx, "list the creatures", err)
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	out := &charactersv1.ListCharacterCreaturesResponse{}
	for _, r := range rows {
		out.Creatures = append(out.Creatures, characterCreatureToProto(content, r.CharacterCreature))
	}
	return connect.NewResponse(out), nil
}

// GiveCreature implements charactersv1connect.CharacterServiceHandler.
func (s *Service) GiveCreature(
	ctx context.Context,
	req *connect.Request[charactersv1.GiveCreatureRequest],
) (*connect.Response[charactersv1.GiveCreatureResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	key := req.Msg.GetMonsterKey()
	if _, ok := content.CreatureByKey(key); !ok {
		return nil, invalidArgument(fieldErr("monster_key", "is not an SRD creature"))
	}
	name := ""
	if req.Msg.GetName() != "" {
		if name, err = cleanCreatureName(req.Msg.GetName()); err != nil {
			return nil, err
		}
	}

	idemKey, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err == nil && idemKey != "" {
		if _, ok := parseUUID(idemKey); !ok {
			err = invalidArgument(fieldErr("idempotency_key", "must be a UUID"))
		}
	}
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and kept with a hash of the whole request: a retry
	// returns the first creature and gives no other.
	scopedKey, requestHash := idem.Scope(m.CampaignID, idemKey), idem.Hash(req.Msg)

	var created charactersdb.CharacterCreature
	var ownerUser string
	var replayed bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		// The session first, as every combat write does (the gift writes a session
		// event); then the owner, so two calls for the same character take turns.
		if err := s.lockSession(ctx, tx, m.CampaignID); err != nil {
			return err
		}
		owner, err := visibleForUpdate(ctx, q, m, id)
		if err != nil {
			return err
		}
		ownerUser = deref(owner.PlayerUserID)
		created, replayed, err = idem.Create(ctx, scopedKey, requestHash, q.GetCharacterCreatureByCreateKey,
			func(c charactersdb.CharacterCreature) *string { return c.CreateHash },
			func() (charactersdb.CharacterCreature, error) {
				if owner.Kind != kindPlayer || owner.Status != statusActive {
					return created, invalidArgument(fieldErr("character_id", "is not a living player's character"))
				}
				res, err := s.SummonCreatures(ctx, tx, link.Summon{
					CampaignID: m.CampaignID, CharacterID: id, Source: creatureSourceMaster, GroupID: uuid.New().String(),
					Creatures: []link.CreatureSpec{{MonsterKey: key, Name: name, Attack: "full"}},
				})
				if err != nil {
					return created, err
				}
				if scopedKey != nil {
					if err := q.SetCharacterCreatureCreateKey(ctx, charactersdb.SetCharacterCreatureCreateKeyParams{
						CampaignID: m.CampaignID, ID: res.Created[0].ID, CreateKey: scopedKey, CreateHash: requestHash,
					}); err != nil {
						return created, wrap("keep the creature's idempotency key", err)
					}
				}
				row, err := q.GetCharacterCreature(ctx, charactersdb.GetCharacterCreatureParams{CampaignID: m.CampaignID, ID: res.Created[0].ID})
				if err != nil {
					return created, wrap("read the new creature", err)
				}
				return row.CharacterCreature, s.logCreatureEvent(ctx, tx, m, "creature_summoned", map[string]any{
					"character_id": id, "creature_ids": []string{row.CharacterCreature.ID}, "monster_keys": []string{key}, "source": creatureSourceMaster,
				})
			})
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "give a creature", err)
	}
	if s.creatureHost != nil && !replayed {
		s.creatureHost.PublishCreaturesChanged(m.CampaignID, ownerUser)
	}
	return connect.NewResponse(&charactersv1.GiveCreatureResponse{Creature: characterCreatureToProto(content, created)}), nil
}

// logCreatureEvent writes a creature event to the open session's history, when
// there is a session and a host to write it (ADR-0007: ids and keys only).
func (s *Service) logCreatureEvent(ctx context.Context, tx pgx.Tx, m authz.Membership, kind string, payload map[string]any) error {
	if s.creatureHost == nil {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode the event payload: %w", err)
	}
	if _, err := s.creatureHost.AppendEvent(ctx, tx, m.CampaignID, kind, m.UserID, body, s.now()); err != nil {
		return wrap("append the creature event", err)
	}
	return nil
}

// RenameCreature implements charactersv1connect.CharacterServiceHandler.
func (s *Service) RenameCreature(
	ctx context.Context,
	req *connect.Request[charactersv1.RenameCreatureRequest],
) (*connect.Response[charactersv1.RenameCreatureResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	name, err := cleanCreatureName(req.Msg.GetName())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var v creatureView
	var encounterID string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		encounterID = ""
		q := s.queries.WithTx(tx)
		if err := s.lockSession(ctx, tx, m.CampaignID); err != nil {
			return err
		}
		if v, err = s.findCreature(ctx, q, m, req.Msg.GetCreatureId(), true); err != nil {
			return err
		}
		if s.creatureHost != nil {
			if encounterID, err = s.creatureHost.CreatureRenamed(ctx, tx, m.CampaignID, v.ID, name); err != nil {
				return err
			}
		}
		if err := q.SetCharacterCreatureName(ctx, charactersdb.SetCharacterCreatureNameParams{CampaignID: m.CampaignID, ID: v.ID, Name: name}); err != nil {
			return wrap("rename a creature", err)
		}
		v.Name = name
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "rename a creature", err)
	}
	if s.creatureHost != nil {
		s.creatureHost.PublishCreaturesChanged(m.CampaignID, deref(v.ownerUserID))
		if encounterID != "" {
			s.creatureHost.PublishEncounterChanged(ctx, m.CampaignID, encounterID)
		}
	}
	return connect.NewResponse(&charactersv1.RenameCreatureResponse{Creature: characterCreatureToProto(content, v.CharacterCreature)}), nil
}

// DismissCreature implements charactersv1connect.CharacterServiceHandler.
func (s *Service) DismissCreature(
	ctx context.Context,
	req *connect.Request[charactersv1.DismissCreatureRequest],
) (*connect.Response[charactersv1.DismissCreatureResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	reason := "owner"
	if isMaster(m) {
		reason = "master"
	}
	var v creatureView
	var encounterID string
	gone := false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		gone, encounterID = false, ""
		q := s.queries.WithTx(tx)
		if err := s.lockSession(ctx, tx, m.CampaignID); err != nil {
			return err
		}
		var err error
		if v, err = s.findCreature(ctx, q, m, req.Msg.GetCreatureId(), true); err != nil {
			// A creature that is gone already changes nothing, for the master and
			// the owner alike; one the caller may not see is not found.
			if connect.CodeOf(err) == connect.CodeNotFound && s.dismissedAlready(ctx, q, m, req.Msg.GetCreatureId()) {
				gone = true
				return nil
			}
			return err
		}
		if s.creatureHost != nil {
			if encounterID, err = s.creatureHost.CreaturesLeaving(ctx, tx, m.CampaignID, m.UserID, []string{v.ID}, s.now()); err != nil {
				return err
			}
		}
		if _, err := q.DismissCreatures(ctx, charactersdb.DismissCreaturesParams{CampaignID: m.CampaignID, Ids: []string{v.ID}, Reason: reason, At: s.now()}); err != nil {
			return wrap("dismiss a creature", err)
		}
		return s.logCreatureEvent(ctx, tx, m, "creature_dismissed", map[string]any{
			"character_id": v.CharacterID, "creature_ids": []string{v.ID}, "reason": reason,
		})
	})
	if err != nil {
		return nil, s.dbError(ctx, "dismiss a creature", err)
	}
	if !gone && s.creatureHost != nil {
		s.creatureHost.PublishCreaturesChanged(m.CampaignID, deref(v.ownerUserID))
		if encounterID != "" {
			s.creatureHost.PublishEncounterChanged(ctx, m.CampaignID, encounterID)
		}
	}
	return connect.NewResponse(&charactersv1.DismissCreatureResponse{}), nil
}

// dismissedAlready says whether the creature exists, belongs to someone the
// caller may see and was dismissed: dismissing it again is not an error.
func (s *Service) dismissedAlready(ctx context.Context, q *charactersdb.Queries, m authz.Membership, rawID string) bool {
	id, ok := parseUUID(rawID)
	if !ok {
		return false
	}
	row, err := q.GetCharacterCreature(ctx, charactersdb.GetCharacterCreatureParams{CampaignID: m.CampaignID, ID: id})
	if err != nil {
		return false
	}
	v := viewOfRow(row)
	return !v.live() && v.visibleTo(m)
}

// AdjustCreatureHitPoints implements charactersv1connect.CharacterServiceHandler.
func (s *Service) AdjustCreatureHitPoints(
	ctx context.Context,
	req *connect.Request[charactersv1.AdjustCreatureHitPointsRequest],
) (*connect.Response[charactersv1.AdjustCreatureHitPointsResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	var mode string
	var amount int32
	switch change := req.Msg.GetChange().(type) {
	case *charactersv1.AdjustCreatureHitPointsRequest_Damage:
		mode, amount = "damage", change.Damage
	case *charactersv1.AdjustCreatureHitPointsRequest_Heal:
		mode, amount = "heal", change.Heal
	case *charactersv1.AdjustCreatureHitPointsRequest_HitPoints:
		mode, amount = "set", change.HitPoints
	default:
		return nil, invalidArgument(fieldErr("change", "set damage, heal or hit_points"))
	}
	if amount < 0 || (mode != "set" && amount > 9999) {
		return nil, invalidArgument(fieldErr(mode, "must be 0 to 9999"))
	}

	content, err := s.contentFor(ctx, nil, m.CampaignID) // before the write: a failure after the commit would make the client retry it
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var v creatureView
	defeated := false
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		defeated = false
		q := s.queries.WithTx(tx)
		if err := s.lockSession(ctx, tx, m.CampaignID); err != nil {
			return err
		}
		var err error
		if v, err = s.findCreature(ctx, q, m, req.Msg.GetCreatureId(), true); err != nil {
			return err
		}
		if s.creatureHost != nil {
			inCombat, err := s.creatureHost.CreatureInCombat(ctx, tx, m.CampaignID, v.ID)
			if err != nil {
				return err
			}
			if inCombat {
				return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CREATURE_IN_COMBAT, v.CharacterID)
			}
		}
		hp := v.HpCurrent
		switch mode {
		case "damage":
			hp = max(hp-amount, 0)
		case "heal":
			hp = min(hp+amount, v.HpMax)
		case "set":
			if amount > v.HpMax {
				return invalidArgument(fieldErr("hit_points", "must be 0 to %d", v.HpMax))
			}
			hp = amount
		}
		if hp == 0 {
			// Out of the fight: it is dismissed (the master's correction may be undone
			// by a heal only while it is in a combat).
			defeated = true
			if _, err := q.DismissCreatures(ctx, charactersdb.DismissCreaturesParams{CampaignID: m.CampaignID, Ids: []string{v.ID}, Reason: "defeated", At: s.now()}); err != nil {
				return wrap("dismiss a defeated creature", err)
			}
			return s.logCreatureEvent(ctx, tx, m, "creature_dismissed", map[string]any{
				"character_id": v.CharacterID, "creature_ids": []string{v.ID}, "reason": "defeated",
			})
		}
		if err := q.SetCharacterCreatureHitPoints(ctx, charactersdb.SetCharacterCreatureHitPointsParams{CampaignID: m.CampaignID, ID: v.ID, HpCurrent: hp}); err != nil {
			return wrap("set a creature's hit points", err)
		}
		v.HpCurrent = hp
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "adjust a creature's hit points", err)
	}
	if s.creatureHost != nil {
		s.creatureHost.PublishCreaturesChanged(m.CampaignID, deref(v.ownerUserID))
	}
	out := &charactersv1.AdjustCreatureHitPointsResponse{}
	if !defeated {
		out.Creature = characterCreatureToProto(content, v.CharacterCreature)
	}
	return connect.NewResponse(out), nil
}
