package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/zone"
)

// The saving throws a zone asks (W7-Z): when a zone's trigger needs a roll (Web, Stinking
// Cloud, Spirit Guardians...), the server does not roll behind the table's back. It opens a
// window for the creature, which its player (or the master, for an NPC) answers with the app's
// d20 or a physical one, and the turn waits meanwhile. It is the zone's reaction window: the
// kind ZONE_SAVE of the reaction mechanism, with the same ways to close; until that mechanism
// lands this package keeps the windows (zone_save_windows) and AnswerZoneSave answers them.
//
// What each viewer is told of a window (RN-10, RN-20):
//
//	                         master                     the creature's player      anyone else
//	the window               all                        the ones on their creature nothing but turn_held
//	the ability, modifier    yes                        yes                        no
//	the DC                   yes                        only when a player's
//	                                                    character cast the zone    no
//	the damage dice          yes                        no                         no
//	the result               passou or falhou, the roll for the master and the roller

// openZoneSave opens the window of a trigger with a saving throw. A creature that cannot be
// affected passes without a roll (Stinking Cloud: poison immunity), which a player reads as a
// success.
func (s *Service) openZoneSave(ctx context.Context, c *combatTx, z zoneState, who playdb.Combatant, tr zone.Trigger, o fireOpts) error {
	if z.row.SpellKey == "spell:stinking-cloud" {
		immune, err := s.immuneTo(ctx, c, who, "damage-type:poison")
		if err != nil {
			return err
		}
		if immune {
			return s.writeZoneEvent(ctx, c, eventMapZoneTriggered, z, zoneNote{
				What: "saved", Who: who.ID, Trigger: string(o.kind), Ability: tr.Save, Saved: true, Immune: true,
			}, who.ID, func(ev *actionEvent) { ev.Secret = ev.Secret || who.Hidden })
		}
	}
	seq, err := c.q.NextZoneSaveSeq(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("number the zone's save: %w", err)
	}
	p := playdb.InsertZoneSaveWindowParams{
		EncounterID: c.enc.ID, ZoneID: &z.row.ID, ReactorID: who.ID, CasterID: z.row.CasterID, Seq: seq, TriggerKind: string(o.kind),
		Round: c.enc.Round, SpellKey: z.row.SpellKey, Ability: tr.Save, Dc: z.row.SaveDc, OnSuccess: string(tr.OnSuccess),
		OnFail: string(tr.OnFail), CoverBonus: clamp32(o.coverBonus, 0, 5), CreatedAt: c.now,
	}
	if cur := deref(c.enc.CurrentCombatantID); cur != "" {
		p.TurnOf = &cur
	}
	if tr.Damage {
		p.DamageCount, p.DamageSides, p.DamageBonus, p.DamageType = z.row.DamageCount, z.row.DamageSides, z.row.DamageBonus, z.row.DamageType
	}
	w, err := c.q.InsertZoneSaveWindow(ctx, p)
	if err != nil {
		return fmt.Errorf("open the zone's save: %w", err)
	}
	return s.writeZoneEvent(ctx, c, eventMapZoneTriggered, z, zoneNote{
		What: "caught", Who: who.ID, Trigger: string(o.kind), Ability: tr.Save, Window: w.ID,
	}, who.ID, func(ev *actionEvent) { ev.Secret = ev.Secret || who.Hidden })
}

// immuneTo says the creature takes no damage of the type at all (its stat block says so).
func (s *Service) immuneTo(ctx context.Context, c *combatTx, who playdb.Combatant, damageType string) (bool, error) {
	mods, err := s.roster.DamageModifiers(ctx, c.tx, c.session.CampaignID, who.CharacterID, deref(who.MonsterKey))
	if err != nil {
		return false, err
	}
	return slices.Contains(mods.Immune, damageType), nil
}

// zoneBlocked is the error of an action refused while a zone's saving throw waits.
func zoneBlocked() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ZONE_SAVE_PENDING, "the turn waits for a saving throw")
}

// mustNotWaitForZone refuses what the turn cannot do while a zone's saving throw waits for
// its answer: everyone's turn waits, the master's too, as for a question of an area spell.
func (s *Service) mustNotWaitForZone(ctx context.Context, c *combatTx) error {
	if c.enc.ID == "" || c.enc.Status != statusActive {
		return nil
	}
	open, err := c.q.ListOpenZoneSaveWindows(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the zones' saves: %w", err)
	}
	if len(open) > 0 {
		return zoneBlocked()
	}
	return nil
}

// AnswerZoneSave implements playv1connect.CombatServiceHandler.
func (s *Service) AnswerZoneSave(
	ctx context.Context,
	req *connect.Request[playv1.AnswerZoneSaveRequest],
) (*connect.Response[playv1.AnswerZoneSaveResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	windowID, err := parseCombatID(req.Msg.GetZoneSaveId(), "zone save")
	if err != nil {
		return nil, err
	}
	var in rollInput
	skip := false
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.AnswerZoneSaveRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.AnswerZoneSaveRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	case *playv1.AnswerZoneSaveRequest_Skip:
		if !roll.Skip {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("skip must be true"))
		}
		skip = true
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app, d20_face or skip"))
	}
	if skip && m.Role != authz.RoleMaster {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the master does not fire a trigger"))
	}

	v := viewerOf(m)
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventZoneSaveAnswered, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		w, err := c.q.GetZoneSaveWindow(ctx, playdb.GetZoneSaveWindowParams{EncounterID: c.enc.ID, ID: windowID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errZoneSaveNotFound()
		}
		if err != nil {
			return nil, fmt.Errorf("find the zone's save: %w", err)
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs)
		reactor, err := findCombatant(cs, w.ReactorID, v)
		if err != nil {
			return nil, errZoneSaveNotFound() // a creature the player does not see has no window for them
		}
		if !v.master && !v.owns(reactor) {
			return nil, errZoneSaveNotFound()
		}
		if w.State != "open" {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ZONE_SAVE_CLOSED, "the saving throw is closed")
		}
		var z zoneState
		if w.ZoneID != nil {
			if row, err := c.q.GetMapZone(ctx, playdb.GetMapZoneParams{EncounterID: c.enc.ID, ID: *w.ZoneID}); err == nil {
				z = zoneStateOf(row)
			}
		}
		note := zoneNote{ID: deref(w.ZoneID), Key: w.SpellKey, What: "saved", Who: reactor.ID, Trigger: w.TriggerKind, Ability: w.Ability, Window: w.ID}
		made = actionEvent{Round: c.enc.Round, Actor: reactor.ID, Secret: reactor.Hidden, Zone: &note}
		if skip {
			if err := c.q.CloseZoneSaveWindow(ctx, playdb.CloseZoneSaveWindowParams{ID: w.ID, CloseReason: closeSkipped, AnsweredAt: &c.now}); err != nil {
				return nil, fmt.Errorf("close the zone's save: %w", err)
			}
			note.Skipped = true
		} else {
			if !v.master {
				if err := s.mustRollThisWay(ctx, c.tx, m, in); err != nil {
					return nil, err
				}
			}
			if err := s.settleZoneSave(ctx, c, w, z, reactor, in, &note); err != nil {
				return nil, err
			}
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &reactor.CharacterID
		made.Zone = &note
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer a zone's saving throw", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the zone's save", err)
	}
	if _, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	}); err != nil {
		return nil, err
	}
	out := &playv1.AnswerZoneSaveResponse{}
	if n := ev.Zone; n != nil {
		out.Saved, out.D20, out.Modifier, out.Total = n.Saved, n.D20, n.Modifier, n.Total
		out.PendingDamageId = n.Pending
		out.FailedWith = failToProto[zone.Fail(n.Failed)]
	}
	return connect.NewResponse(out), nil
}

// closeSkipped is the reason a window closes with when the master does not fire the trigger.
const closeSkipped = "skipped"

func errZoneSaveNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("zone save not found"))
}

// settleZoneSave rolls the saving throw of a window and does what it decides: the damage the
// zone deals (half, on a success that halves) opens as a pending damage, and a failure puts its
// condition on the creature.
func (s *Service) settleZoneSave(ctx context.Context, c *combatTx, w playdb.ZoneSaveWindow, z zoneState, who playdb.Combatant, in rollInput, note *zoneNote) error {
	save, err := s.saveOf(ctx, c.tx, c.session.CampaignID, who, w.Ability)
	if err != nil {
		return err
	}
	bonus := save.Bonus
	if w.Ability == "dex" {
		bonus += int(w.CoverBonus) // cover adds to a Dexterity save (SRD, "Cover")
	}
	face, roll, err := s.d20(in, bonus)
	if err != nil {
		return err
	}
	saved := combat.SaveSucceeded(roll.Total, int(w.Dc))
	note.D20, note.Modifier, note.Total, note.DC, note.Saved, note.Physical = clamp32(face, 1, 20), clamp32(bonus, math.MinInt32, math.MaxInt32), clamp32(roll.Total, math.MinInt32, math.MaxInt32), w.Dc, saved, roll.Physical
	if _, err := c.q.AnswerZoneSaveWindow(ctx, playdb.AnswerZoneSaveWindowParams{
		ID: w.ID, D20: &note.D20, Modifier: &note.Modifier, Total: &note.Total, Saved: &saved, Physical: roll.Physical, AnsweredAt: &c.now,
	}); err != nil {
		return fmt.Errorf("answer the zone's save: %w", err)
	}
	if w.DamageSides > 0 && (!saved || w.OnSuccess == string(zone.SuccessHalf)) {
		zz := z
		if zz.row.ID == "" { // the zone ended meanwhile: the window's own numbers carry the damage
			zz.row.DamageCount, zz.row.DamageSides, zz.row.DamageBonus, zz.row.DamageType = w.DamageCount, w.DamageSides, w.DamageBonus, w.DamageType
			zz.row.CasterID, zz.row.SpellKey = w.CasterID, w.SpellKey
		}
		if note.Pending, err = s.openZoneDamage(ctx, c, zz, who, 1, saved); err != nil {
			return err
		}
	}
	if saved {
		return nil
	}
	note.Failed = w.OnFail
	switch zone.Fail(w.OnFail) {
	case zone.FailRestrained:
		return s.putZoneCondition(ctx, c, z, who, condRestrained, z.row.ID == "")
	case zone.FailProne:
		return s.putZoneCondition(ctx, c, z, who, condProne, true)
	case zone.FailLoseAction:
		return c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
			ID: who.ID, ActionUsed: true, BonusActionUsed: who.BonusActionUsed, ReactionUsed: who.ReactionUsed, Dashed: who.Dashed,
		})
	}
	return nil
}

// ---- what a viewer is told ----

// zoneSavesView is the windows the viewer is told of, and whether any waits (the turn is held).
func (s *Service) zoneSavesView(ctx context.Context, campaignID string, d *encounterData, v combatViewer) ([]*playv1.ZoneSave, bool, error) {
	if d.enc.Status != statusActive {
		return nil, false, nil
	}
	open, err := s.queries.ListOpenZoneSaveWindows(ctx, d.enc.ID)
	if err != nil {
		return nil, false, s.dbError(ctx, "list the zones' saves", err)
	}
	var out []*playv1.ZoneSave
	for _, w := range open {
		i := slices.IndexFunc(d.cs, func(o playdb.Combatant) bool { return o.ID == w.ReactorID })
		if i < 0 {
			continue
		}
		reactor := d.cs[i]
		if !v.master && !v.owns(reactor) {
			continue
		}
		casterPlayer := w.CasterID != nil && isPlayerCombatant(d.cs, *w.CasterID)
		p := &playv1.ZoneSave{
			Id: w.ID, ZoneId: deref(w.ZoneID), ReactorId: w.ReactorID, TriggerKind: triggerKindToProto[zone.TriggerKind(w.TriggerKind)],
			SpellKey: w.SpellKey, Ability: w.Ability, OnFail: failToProto[zone.Fail(w.OnFail)], Mine: v.master || v.owns(reactor),
			OpenedAt: timestamppb.New(w.CreatedAt),
		}
		if bonus, err := s.saveOf(ctx, nil, campaignID, reactor, w.Ability); err == nil {
			p.Modifier = clamp32(bonus.Bonus, math.MinInt32, math.MaxInt32)
		}
		if v.master || casterPlayer {
			dc := w.Dc
			p.Dc = &dc
		}
		if v.master {
			p.DamageCount, p.DamageSides, p.DamageBonus, p.DamageType = w.DamageCount, w.DamageSides, w.DamageBonus, w.DamageType
			p.HalfOnSuccess = w.OnSuccess == string(zone.SuccessHalf)
		}
		out = append(out, p)
	}
	return out, len(open) > 0, nil
}
