package play

import (
	"context"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// opportunityOffers lists the pending offers as the caller may see them (RN-10,
// RN-20): the master all, with names; a player the offers of the reactors they
// control (to answer) and the ones on a combatant of theirs, the reactor named
// only when they see it. Nobody else gets any.
func (s *Service) opportunityOffers(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer) ([]*playv1.OpportunityOffer, error) {
	if d.enc.Status != statusActive {
		return nil, nil
	}
	pending, err := s.queries.ListPendingOpportunityOffers(ctx, d.enc.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the opportunity offers", err)
	}
	var out []*playv1.OpportunityOffer
	for _, o := range pending {
		if offer, ok := s.offerForViewer(ctx, m, d, v, o); ok {
			out = append(out, offer)
		}
	}
	return out, nil
}

// offerForViewer is one pending offer as v reads it, or false when it is not v's
// to know.
func (s *Service) offerForViewer(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, o playdb.OpportunityOffer) (*playv1.OpportunityOffer, bool) {
	mover, ok1 := combatantByID(d.cs, o.MoverID)
	reactor, ok2 := combatantByID(d.cs, o.ReactorID)
	// The mover is judged on the square it left: a retreating NPC the player saw
	// leave still has the offer for their reactor, though they do not see it now.
	if !ok1 || !ok2 || !v.seesAtOffer(mover, o) {
		return nil, false
	}
	answers := v.master || v.owns(reactor)
	if !answers && !v.owns(mover) {
		return nil, false
	}
	offer := &playv1.OpportunityOffer{Id: o.ID, MoverId: mover.ID, MoverLabel: mover.Label, ForYou: answers}
	if o.Jumped {
		offer.Jump = playv1.JumpKind_JUMP_KIND_LONG
	}
	if v.sees(reactor) {
		offer.ReactorId, offer.ReactorLabel = reactor.ID, reactor.Label
	}
	if answers {
		offer.LeftCol, offer.LeftRow = valueOf(o.LeftCol), valueOf(o.LeftRow)
		offer.ByHand = o.LeftCol == nil
		offer.Attacks = s.reactorMeleeAttacks(ctx, m.CampaignID, reactor)
	}
	return offer, true
}

// reactorMeleeAttacks are the melee attacks the reactor could answer with; a sheet that
// cannot be read lists none.
func (s *Service) reactorMeleeAttacks(ctx context.Context, campaignID string, reactor playdb.Combatant) []*playv1.OpportunityAttack {
	sheet, err := s.sheetOf(ctx, nil, campaignID, reactor)
	if err != nil {
		return nil
	}
	var out []*playv1.OpportunityAttack
	for _, a := range sheet.Attacks {
		if meleeAttack(a) {
			out = append(out, &playv1.OpportunityAttack{Key: a.Key, NamePt: a.Name})
		}
	}
	return out
}

// provokeSight asks the fog's rule of the real offers (a reactor must see the
// mover), once for each reactor a square would make provoke.
type provokeSight struct {
	s          *Service
	campaignID string
	f          *fogSight
	from       grid.Square
	saw        map[string]bool
}

// sees says whether the provoker sees the mover on the square it starts from.
func (ps *provokeSight) sees(ctx context.Context, p provoker) (bool, error) {
	if ps.f == nil {
		return true, nil
	}
	if ok, done := ps.saw[p.reactor.ID]; done {
		return ok, nil
	}
	sheet := p.sheet
	if len(sheet.Attacks) == 0 { // a player's warning never read the sheet
		var err error
		if sheet, err = ps.s.sheetOf(ctx, nil, ps.campaignID, p.reactor); err != nil && connect.CodeOf(err) != connect.CodeNotFound {
			return false, err
		}
	}
	ps.saw[p.reactor.ID] = ps.f.reactorSees(p.reactor, sheet, ps.from)
	return ps.saw[p.reactor.ID], nil
}

// markProvokes fills, on each square a move can reach, the reactors it would let
// attack, for the squares the caller's screen warns about. Only reactors the
// viewer sees (RN-10): a hidden one is never named. Nothing off the mover's turn
// (a placement offers nothing) and nothing after Disengage.
func (s *Service) markProvokes(ctx context.Context, m authz.Membership, enc playdb.Encounter, cs []playdb.Combatant, who playdb.Combatant, v combatViewer, f *fogSight, out *playv1.GetMoveOptionsResponse) error {
	if !actsNow(enc, who) || len(out.Reachable) == 0 {
		return nil
	}
	reactors, err := s.opportunityReactors(ctx, nil, m.CampaignID, cs, who, &v, out.DraggingCombatantId)
	if err != nil || len(reactors) == 0 {
		return err
	}
	ps := &provokeSight{s: s, campaignID: m.CampaignID, f: f, from: squareOfCombatant(who), saw: map[string]bool{}}
	for _, r := range out.Reachable {
		ids, err := ps.provokedAt(ctx, reactors, grid.Square{Col: int(r.Col), Row: int(r.Row)})
		if err != nil {
			return err
		}
		r.ProvokesReactorIds = append(r.ProvokesReactorIds, ids...)
	}
	return nil
}

// provokedAt are the reactors a move to the square leaves behind and that see it.
func (ps *provokeSight) provokedAt(ctx context.Context, reactors []candidate, to grid.Square) ([]string, error) {
	var ids []string
	for _, p := range provokedBy(reactors, ps.from, to) {
		ok, err := ps.sees(ctx, p)
		if err != nil {
			return nil, err
		}
		if ok {
			ids = append(ids, p.reactor.ID)
		}
	}
	return ids, nil
}
