package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Finishing, interrupting and ending the casts outside a combat (casting.go), and what
// ending one takes back.

// castTargetsOf reads the targets a cast stored.
func castTargetsOf(r playdb.SpellCast) []castTarget {
	var out []castTarget
	if len(r.Targets) > 0 {
		_ = json.Unmarshal(r.Targets, &out) // our own JSON: an unreadable one has no targets
	}
	return out
}

// closeCast ends a cast: ended (it took effect and is over) or failed (the casting did
// not finish), and takes back what it did where it can be taken back: Mage Armor's
// armor class, and the creatures that lasted only while the caster concentrated.
// Healing and temporary hit points stay (they were instantaneous, or run out by
// themselves).
func (s *Service) closeCast(ctx context.Context, c *combatTx, r playdb.SpellCast, status, reason string) (playdb.SpellCast, []*playv1.CharacterVitals, []string, error) {
	was := r
	row, err := c.q.EndSpellCast(ctx, playdb.EndSpellCastParams{ID: r.ID, Status: status, EndReason: &reason, EndedAt: &c.now})
	if err != nil {
		return r, nil, nil, fmt.Errorf("end the cast: %w", err)
	}
	// What the cast left on the characters out of a combat goes with it.
	if err := s.endCharacterEffectsOfCast(ctx, c, r.ID); err != nil {
		return r, nil, nil, err
	}
	var vitals []*playv1.CharacterVitals
	if was.Status == castActive {
		v, err := s.takeBackCast(ctx, c, row)
		if err != nil {
			return row, nil, nil, err
		}
		vitals = v
	}
	var dismissed []string
	if was.Status == castActive && was.Concentrating {
		if dismissed, err = s.dismissConcentrationCreatures(ctx, c, was.CasterID); err != nil {
			return row, nil, nil, err
		}
	}
	return row, vitals, dismissed, nil
}

// takeBackCast undoes what a cast that lasted did to its targets. Mage Armor stays on a
// target while another Mage Armor still does (SRD 5.1, "Combining Magical Effects":
// the effects of the same spell cast several times don't combine, the most potent one
// applies while the durations overlap).
func (s *Service) takeBackCast(ctx context.Context, c *combatTx, r playdb.SpellCast) ([]*playv1.CharacterVitals, error) {
	return s.takeBackMaxHP(ctx, c, r)
}

// dismissConcentrationCreatures sends away the creatures that lasted only while the
// caster concentrated, and writes the line of each (MR-037, RN-22).
func (s *Service) dismissConcentrationCreatures(ctx context.Context, c *combatTx, characterID string) ([]string, error) {
	creatures, err := s.roster.ConcentrationCreatures(ctx, c.tx, c.session.CampaignID, characterID)
	if err != nil || len(creatures) == 0 {
		return nil, err
	}
	ids := make([]string, 0, len(creatures))
	for _, cr := range creatures {
		ids = append(ids, cr.ID)
	}
	dismissed, err := s.roster.DismissCreatures(ctx, c.tx, c.session.CampaignID, ids, "concentration", c.now)
	if err != nil {
		return nil, err
	}
	was := c.characterID
	c.characterID = &characterID
	defer func() { c.characterID = was }()
	for _, id := range dismissed {
		if err := insertEvent(ctx, c, eventCreatureDismissed, &c.actorUserID, nil, actionEvent{OwnerCharacter: characterID, Created: []string{id}, Reason: "concentration"}); err != nil {
			return nil, err
		}
	}
	return dismissed, nil
}

// endOtherConcentration ends the concentration cast of the caster other than keep: the
// caster cannot concentrate on two spells at once (SRD 5.1, "Duration"). It returns the
// casts it ended, the creatures it sent away and the vitals it changed.
func (s *Service) endOtherConcentration(ctx context.Context, c *combatTx, caster link.Character, keep string) (ended, dismissed []string, vitals []*playv1.CharacterVitals, err error) {
	live, err := c.q.ListLiveSpellCastsOfCaster(ctx, caster.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list the caster's casts: %w", err)
	}
	for _, r := range live {
		if r.ID == keep || !r.Concentrating {
			continue
		}
		_, v, d, err := s.closeCast(ctx, c, r, castEnded, endConcentration)
		if err != nil {
			return nil, nil, nil, err
		}
		ended = append(ended, r.ID)
		dismissed = append(dismissed, d...)
		vitals = append(vitals, v...)
	}
	return ended, dismissed, vitals, nil
}

// castRowFor reads the cast a request names and checks the caller may act on it: its
// caster's player or the master. A cast the caller may not see is not found (RN-10).
func (s *Service) castRowFor(ctx context.Context, c *combatTx, m authz.Membership, castID string) (playdb.SpellCast, link.Character, error) {
	id, ok := parseID(castID)
	if !ok {
		return playdb.SpellCast{}, link.Character{}, errCastNotFound()
	}
	row, err := c.q.GetSpellCast(ctx, playdb.GetSpellCastParams{ID: id, CampaignID: m.CampaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, link.Character{}, errCastNotFound()
	}
	if err != nil {
		return row, link.Character{}, fmt.Errorf("find the cast: %w", err)
	}
	master := m.Role == authz.RoleMaster
	if row.Secret && !master {
		return row, link.Character{}, errCastNotFound()
	}
	chars, err := s.roster.SessionCharacters(ctx, c.tx, m.CampaignID, []string{row.CasterID})
	if err != nil {
		return row, link.Character{}, err
	}
	if len(chars) == 0 {
		if !master {
			return row, link.Character{}, errCastNotFound()
		}
		return row, link.Character{ID: row.CasterID}, nil
	}
	caster := chars[0]
	if !master && (!caster.Player || caster.PlayerUserID != m.UserID) {
		return row, caster, connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may do this"))
	}
	return row, caster, nil
}

// ConfirmCastTimePassed implements playv1connect.CastingServiceHandler.
func (s *Service) ConfirmCastTimePassed(
	ctx context.Context,
	req *connect.Request[playv1.ConfirmCastTimePassedRequest],
) (*connect.Response[playv1.ConfirmCastTimePassedResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	in, err := finishInputOf(req.Msg)
	if err != nil {
		return nil, err
	}

	var made castOutcome
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventSpellCastFinished}, func(c *combatTx) (any, error) {
		made = castOutcome{}
		out, err := s.finishCastTx(ctx, c, m, req.Msg.GetCastId(), in)
		if err != nil {
			return nil, err
		}
		made = out
		return out.ev, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "finish a cast", err)
	}
	return s.castAnswerFinish(ctx, m, res, made)
}

// castFinishInput is what the master's request to finish a cast says: the dice (when the spell rolls any) and the
// creatures picked for a summoning spell.
type castFinishInput struct {
	in     rollInput
	rolled bool
	pick   *playv1.SummonChoice
}

// finishInputOf reads the dice and the summon pick of a request to finish a cast.
func finishInputOf(msg *playv1.ConfirmCastTimePassedRequest) (castFinishInput, error) {
	var inApp *bool
	var poolSum *int32
	switch roll := msg.GetRoll().(type) {
	case *playv1.ConfirmCastTimePassedRequest_RollInApp:
		inApp = &roll.RollInApp
	case *playv1.ConfirmCastTimePassedRequest_TypedSum:
		poolSum = &roll.TypedSum
	}
	in, rolled, err := castRoll(inApp, poolSum)
	if err != nil {
		return castFinishInput{}, err
	}
	return castFinishInput{in: in, rolled: rolled, pick: msg.GetSummon()}, nil
}

// finishCastTx is ConfirmCastTimePassed inside the transaction: the cast and who may finish it, the characters it
// is about, the plan rebuilt from the sheet, the dice, the time that passed and the effect.
func (s *Service) finishCastTx(ctx context.Context, c *combatTx, m authz.Membership, castID string, input castFinishInput) (castOutcome, error) {
	row, caster, err := s.castRowFor(ctx, c, m, castID)
	if err != nil {
		return castOutcome{}, err
	}
	if m.Role != authz.RoleMaster {
		// Outside a combat there is no clock: the master says the time has passed.
		return castOutcome{}, connect.NewError(connect.CodePermissionDenied, errors.New("only the master completes a cast"))
	}
	if row.Status != castCasting {
		return castOutcome{}, errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_CAST_NOT_GOING, "the spell is not being cast")
	}
	// The targets chosen at the start: one that is gone (died, left) is dropped.
	g, err := s.guardFinish(ctx, c, m, caster, row)
	if err != nil {
		return castOutcome{}, err
	}
	if err := s.mayFinishNow(ctx, c, g.caster); err != nil {
		return castOutcome{}, err
	}
	plan, err := s.planFinish(ctx, c, m, g, row)
	if err != nil {
		return castOutcome{}, err
	}
	plan.in, plan.rolled, plan.pick, plan.row = input.in, input.rolled, input.pick, row
	if err := s.checkCastInput(ctx, c, m, plan); err != nil {
		return castOutcome{}, err
	}
	// The time the casting took has passed: game time moves on for what had a clock.
	if err := s.advanceGameTime(ctx, c, clamp32(int(row.CastingMinutes)*60, 0, 1<<30)); err != nil {
		return castOutcome{}, err
	}
	out, err := s.takeEffect(ctx, c, m, plan)
	if err != nil {
		return castOutcome{}, err
	}
	out.guard = g
	c.characterID = &g.caster.ID
	return out, nil
}

// guardFinish reads the characters a started cast is about for FinishCast. The targets
// were checked when the cast started, so one that left the stage meanwhile still
// counts, and one that is not a living character any more is dropped.
func (s *Service) guardFinish(ctx context.Context, c *combatTx, m authz.Membership, caster link.Character, row playdb.SpellCast) (castGuard, error) {
	g := castGuard{caster: caster, secret: row.Secret}
	var ids []string
	for _, t := range castTargetsOf(row) {
		ids = append(ids, t.ID)
	}
	chars, err := s.roster.CombatCharacters(ctx, c.tx, m.CampaignID, append([]string{row.CasterID}, ids...))
	if err != nil {
		return g, err
	}
	byID := make(map[string]link.Character, len(chars))
	for _, ch := range chars {
		byID[ch.ID] = ch
	}
	living, ok := byID[row.CasterID]
	if !ok {
		return g, errCastNotFound() // the caster died or left meanwhile; InterruptCast closes it
	}
	g.caster = living
	if g.stage, err = stageOf(ctx, c.q, c.session.ID); err != nil {
		return g, err
	}
	for _, id := range ids {
		if t, ok := byID[id]; ok && !t.CombatOnly {
			g.targets = append(g.targets, t)
		}
	}
	return g, nil
}

// mayFinishNow checks the caster is not in a combat and is not in a beast form.
func (s *Service) mayFinishNow(ctx context.Context, c *combatTx, caster link.Character) error {
	if caster.Player {
		now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, caster.ID)
		if err != nil {
			return err
		}
		if err := noSpellsIn(now, caster.CastsInBeastForm); err != nil {
			return err
		}
	}
	fighting, err := s.inCombat(ctx, c.q, c.session.ID, caster.ID)
	if err != nil {
		return err
	}
	if fighting {
		return errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_IN_COMBAT, "the character is in a combat: the casting goes on there, and completes when the combat is over")
	}
	return nil
}

// planFinish rebuilds the plan of a started cast and checks the sheet still allows it
// and the slot is still free: the slot is spent now.
func (s *Service) planFinish(ctx context.Context, c *combatTx, m authz.Membership, g castGuard, row playdb.SpellCast) (castPlan, error) {
	osp, err := s.roster.OutsideSpell(ctx, c.tx, m.CampaignID, g.caster.ID, row.SpellKey, int(row.SlotLevel))
	if err != nil {
		return castPlan{}, err
	}
	plan := castPlan{g: g, osp: osp, spell: row.SpellKey, ritual: row.Ritual, slot: slotOfRow(row), minutes: int(row.CastingMinutes)}
	switch {
	case row.Ritual:
		if !osp.CanRitual {
			return plan, badCast("the character cannot cast this spell as a ritual")
		}
	case !osp.Prepared:
		return plan, badCast("spell_key is not one of the caster's spells")
	case plan.slot != nil:
		if !slices.ContainsFunc(osp.Slots, func(sl link.CastSlot) bool { return sl.Level == int(plan.slot.Level) && sl.Pact == plan.slot.Pact }) {
			return plan, errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_NO_SLOT, "there is no free spell slot for this spell",
				func(b *playv1.CastingBlocked) { b.MinLevel = plan.slot.Level })
		}
	}
	return plan, nil
}

// AbandonCast implements playv1connect.CastingServiceHandler.
func (s *Service) AbandonCast(
	ctx context.Context,
	req *connect.Request[playv1.AbandonCastRequest],
) (*connect.Response[playv1.AbandonCastResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	var made castOutcome
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventSpellCastInterrupted}, func(c *combatTx) (any, error) {
		made = castOutcome{}
		row, _, err := s.castRowFor(ctx, c, m, req.Msg.GetCastId())
		if err != nil {
			return nil, err
		}
		made.row = row
		if row.Status != castCasting {
			return nil, nil // not being cast any more: nothing changes, and no event
		}
		ended, vitals, dismissed, err := s.closeCast(ctx, c, row, castFailed, endInterrupted)
		if err != nil {
			return nil, err
		}
		made.row, made.vitals, made.dismissed = ended, vitals, dismissed
		made.ev = castEvent{CastID: row.ID, Caster: row.CasterID, Key: row.SpellKey, Secret: row.Secret, Reason: endInterrupted}
		c.characterID = &row.CasterID
		return made.ev, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "interrupt a cast", err)
	}
	cast, err := s.afterCastChange(ctx, m, res, made, "")
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AbandonCastResponse{Cast: cast}), nil
}

// EndActiveSpell implements playv1connect.CastingServiceHandler.
func (s *Service) EndActiveSpell(
	ctx context.Context,
	req *connect.Request[playv1.EndActiveSpellRequest],
) (*connect.Response[playv1.EndActiveSpellResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	var made castOutcome
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventSpellCastEnded}, func(c *combatTx) (any, error) {
		made = castOutcome{}
		return s.endSpellTx(ctx, c, m, req.Msg.GetCastId(), &made)
	})
	if err != nil {
		return nil, s.dbError(ctx, "end a spell", err)
	}
	cast, err := s.afterCastChange(ctx, m, res, made, "")
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.EndActiveSpellResponse{Cast: cast, Vitals: filterVitals(m, made.vitals), DismissedCreatureIds: made.dismissed}), nil
}

// endSpellTx is EndActiveSpell inside the transaction. It fills made and returns the event to write; a cast that is
// over already changes nothing and writes no event (nil).
func (s *Service) endSpellTx(ctx context.Context, c *combatTx, m authz.Membership, castID string, made *castOutcome) (any, error) {
	row, _, err := s.castRowFor(ctx, c, m, castID)
	if err != nil {
		return nil, err
	}
	made.row = row
	if over, err := s.mayEndSpell(ctx, c, row); over || err != nil {
		return nil, err
	}
	ended, vitals, dismissed, err := s.closeCast(ctx, c, row, castEnded, endDismissed)
	if err != nil {
		return nil, err
	}
	made.row, made.vitals, made.dismissed = ended, vitals, dismissed
	made.ev = castEvent{CastID: row.ID, Caster: row.CasterID, Key: row.SpellKey, Secret: row.Secret, Dismissed: dismissed, Reason: endDismissed}
	c.characterID = &row.CasterID
	return made.ev, nil
}

// mayEndSpell checks the cast can be ended now. over says it is over already: nothing changes, and no event.
func (s *Service) mayEndSpell(ctx context.Context, c *combatTx, row playdb.SpellCast) (over bool, err error) {
	switch row.Status {
	case castCasting:
		return false, errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_CAST_NOT_ACTIVE, "the spell is still being cast: interrupt the casting")
	case castEnded, castFailed:
		return true, nil
	}
	if row.CarriedEncounterID == nil {
		return false, nil
	}
	// The combatant holds the concentration while the combat lasts: it is ended there.
	fighting, err := s.inCombat(ctx, c.q, c.session.ID, row.CasterID)
	if err != nil {
		return false, err
	}
	if fighting {
		return false, errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_IN_COMBAT, "the concentration is in a combat: end it there")
	}
	return false, nil
}
