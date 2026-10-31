package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// endTurnCall is what EndTurn reads from its request, checked: the ids are well-formed, the key
// is a UUID.
type endTurnCall struct {
	key         string
	encounterID string
	// expected is the member whose part ends; empty only when nobody is acting (see the
	// orphaned turn).
	expected string
	// round is the round the caller's screen shows; zero means it did not say.
	round   int32
	discard bool
}

// endTurnOutcome is what the write of EndTurn leaves for the publishers after the commit.
type endTurnOutcome struct {
	dropped []playdb.PendingDamage // the damage the master passed the turn over
	passed  bool                   // the turn passed to the next group (the last part ended)
	secret  bool                   // the part that ended is a hidden member's: no line for the players
}

// readEndTurnCall reads and checks the request, in the order the answers are given: who may call, the
// key, the encounter, then the expected member.
func readEndTurnCall(ctx context.Context, req *playv1.EndTurnRequest) (authz.Membership, endTurnCall, error) {
	m, key, encID, err := memberCall(ctx, req.GetCampaignId(), req.GetIdempotencyKey(), req.GetEncounterId())
	if err != nil {
		return m, endTurnCall{}, err
	}
	call := endTurnCall{key: key, encounterID: encID, round: req.GetExpectedRound(), discard: req.GetDiscardPendingDamage()}
	if raw := req.GetExpectedCombatantId(); raw != "" {
		if call.expected, err = parseCombatID(raw, "combatant"); err != nil {
			return m, call, err
		}
	}
	return m, call, nil
}

// errStaleTurnPart is the answer to a tap from a screen that saw another turn.
func errStaleTurnPart() error {
	return connect.NewError(connect.CodeAborted, errors.New("that combatant's part of the turn is not the one running"))
}

// endTurnWrite is the transaction of EndTurn: the checks, the part that ends and where the turn
// goes. It returns the payload write stores as the event, or nil to wait for the player.
func (s *Service) endTurnWrite(ctx context.Context, c *combatTx, m authz.Membership, call endTurnCall, out *endTurnOutcome) (any, error) {
	cs, v, err := s.readEndTurnState(ctx, c, m, call)
	if err != nil {
		return nil, err
	}
	// An orphaned turn: nobody acts, because the combatants of the turn left the fight (the
	// last one who could act was removed, or its character was deleted).
	if !slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.TurnState == turnActing }) {
		return s.restartOrphanedTurn(ctx, c, v, cs, call, out)
	}
	current, err := endingMember(cs, v, call.expected)
	if err != nil {
		return nil, err
	}
	if wait, err := s.endTurnGates(ctx, c, v, current); err != nil || wait {
		return nil, err
	}
	if out.dropped, err = s.dropPendingDamageOnEnd(ctx, c, m, v, cs, current, call.discard); err != nil {
		return nil, err
	}
	if err := s.closeTurnPart(ctx, c, current, out.dropped); err != nil {
		return nil, err
	}
	return s.passTheTurn(ctx, c, cs, current, out)
}

// readEndTurnState is the read and validate phase of the transaction: the combat must be running,
// the screen not from an earlier round, and a combatant the player does not see is not found.
func (s *Service) readEndTurnState(ctx context.Context, c *combatTx, m authz.Membership, call endTurnCall) ([]playdb.Combatant, combatViewer, error) {
	if c.enc.Status != statusActive {
		return nil, combatViewer{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	}
	// A late tap from a screen that saw an earlier round: when a group is the
	// only one, the last part ending starts the next round with the same members.
	if call.round != 0 && call.round != c.enc.Round {
		return nil, combatViewer{}, errStaleTurnPart()
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, combatViewer{}, err
	}
	// A combatant the player does not see is not found, before anything else can
	// answer differently (a stale turn, a part that ended): so it is on every call.
	v := c.viewer(m, cs)
	if !v.master && call.expected != "" && slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.ID == call.expected }) {
		if _, err := findCombatant(cs, call.expected, v); err != nil {
			return nil, v, err
		}
	}
	return cs, v, nil
}

// restartOrphanedTurn starts the turns again from the top of the order, in the same round, when
// nobody acts; only the master may, and without it the combat could never move.
func (s *Service) restartOrphanedTurn(ctx context.Context, c *combatTx, v combatViewer, cs []playdb.Combatant, call endTurnCall, out *endTurnOutcome) (any, error) {
	// The turn that ended waits for the saving throws of the effects on its members: it is
	// held, not orphaned, and another tap changes nothing.
	if held, err := s.endSaveOpen(ctx, c); err != nil {
		return nil, err
	} else if held {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING, "a reaction waits for its answer")
	}
	if !v.master {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the master can start the turns again"))
	}
	// The master's screen may still hold the id of the one who left.
	if call.expected != "" && (c.enc.CurrentCombatantID == nil || *c.enc.CurrentCombatantID != call.expected) {
		return nil, errStaleTurnPart()
	}
	first, _, ok := nextTurnGroup(cs, "", "")
	if !ok {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "no combatant can take a turn")
	}
	out.passed = true
	if err := startTurn(ctx, c, first, c.enc.Round); err != nil {
		return nil, err
	}
	return map[string]any{"to": first[0], "round": c.enc.Round}, nil
}

// endingMember is the member whose part ends: the one the screen named, still acting, and one the
// caller may act for.
func endingMember(cs []playdb.Combatant, v combatViewer, expected string) (playdb.Combatant, error) {
	// A double tap, or a stale screen: that member's part ended already, or
	// the turn passed, and nothing changes, so one tap never ends two parts
	// or two turns.
	at := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == expected })
	if at < 0 || cs[at].TurnState != turnActing {
		return playdb.Combatant{}, errStaleTurnPart()
	}
	current, err := findCombatant(cs, expected, v)
	if err != nil {
		return current, err
	}
	return current, v.mayAct(current)
}

// endTurnGates are the things that hold a part from ending: a question for the master, the rage
// the player is asked about, an opportunity attack's answer and a death save. wait reports that
// the turn must wait for the player's answer about the rage.
func (s *Service) endTurnGates(ctx context.Context, c *combatTx, v combatViewer, current playdb.Combatant) (wait bool, err error) {
	// The turn waits for the master's answer to a question of an area spell, and
	// his own turn passing waits too.
	if err := s.mustNotHold(ctx, c); err != nil {
		return false, err
	}
	// A rage ends with the turn if the barbarian attacked no hostile creature and took
	// no damage: the player is asked first (the master applies the rule himself).
	if wait, err := s.endOfTurn(ctx, c, v, current); err != nil || wait {
		return wait, err
	}
	// The turn waits for an opportunity attack's answer; the master may end it
	// anyway, which passes the offers over.
	if err := s.mustNotWaitForOffers(ctx, c, current); err != nil {
		return false, err
	}
	if _, err := c.q.SkipPendingOpportunityOffersOfMover(ctx, playdb.SkipPendingOpportunityOffersOfMoverParams{EncounterID: c.enc.ID, MoverID: current.ID, AnsweredAt: &c.now}); err != nil {
		return false, fmt.Errorf("pass the opportunity offers over: %w", err)
	}
	return false, s.mustHaveRolledDeathSave(ctx, c, current)
}

// mustHaveRolledDeathSave refuses a player whose character is down and has not rolled its death
// save (RN-03). The master is held too: he rolls the save for the character (in the app or with a
// typed d20) and the turn then ends.
func (s *Service) mustHaveRolledDeathSave(ctx context.Context, c *combatTx, current playdb.Combatant) error {
	if current.Kind != kindPlayer {
		return nil
	}
	vit, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, current.CharacterID)
	if err != nil && connect.CodeOf(err) != connect.CodeNotFound { // not found: the character died meanwhile
		return err
	}
	if err == nil && deathSaveDue(current, vit) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_DUE, "roll your death save before ending the turn")
	}
	return nil
}

// dropPendingDamageOnEnd settles a damage still to roll or to apply, which holds the member's part;
// only the master may pass it over, which drops the damage (RN-02: he has the last word). It
// returns the damage dropped.
func (s *Service) dropPendingDamageOnEnd(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, cs []playdb.Combatant, current playdb.Combatant, discard bool) ([]playdb.PendingDamage, error) {
	dropped, err := c.pendingOf(ctx, current.ID)
	if err != nil {
		return dropped, err
	}
	if !v.master {
		// A damage on a creature the player does not see is the master's to roll (an
		// area spell hit a hidden creature): it does not hold their turn, and no
		// refusal says it exists (RN-10).
		dropped = slices.DeleteFunc(dropped, func(p playdb.PendingDamage) bool { return !pendingVisible(p, cs, v) })
	}
	if len(dropped) == 0 {
		return dropped, nil
	}
	if !v.master || !discard {
		return dropped, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_PENDING_DAMAGE, "a damage is still to roll or to apply")
	}
	for i, p := range dropped {
		if dropped[i], err = c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: pendingDiscarded, ResolvedAt: &c.now}); err != nil {
			return dropped, fmt.Errorf("discard the pending damage: %w", err)
		}
		// One event for each, written before the turn's own, so the log shows
		// the attack as dropped and an undo stays closed once the turn passes.
		tgt, _ := findCombatant(cs, p.TargetID, combatViewer{master: true})
		if err := insertEvent(ctx, c, eventDamageDiscarded, &m.UserID, nil, actionEvent{
			Round: c.enc.Round, Secret: secretOf(current, tgt), Actor: current.ID, Target: p.TargetID, Pending: p.ID, Key: p.AttackKey,
			Amount: conv.OrZero(p.Amount), PrevStatus: p.Status,
		}); err != nil {
			return dropped, err
		}
	}
	return dropped, nil
}

// closeTurnPart ends the member's part: after the reaction windows the dropped damage was not
// waiting on, the states that end now go, and the saving throws of its effects open.
func (s *Service) closeTurnPart(ctx context.Context, c *combatTx, current playdb.Combatant, dropped []playdb.PendingDamage) error {
	// A reaction window holds the turn too (PM-04), except the ones the damage the
	// master just dropped was waiting on: they close with it.
	droppedIDs := make([]string, len(dropped))
	for i, p := range dropped {
		droppedIDs[i] = p.ID
	}
	if err := s.reactionGate(ctx, c, droppedIDs...); err != nil {
		return err
	}
	c.characterID = &current.CharacterID
	if err := c.q.EndCombatantTurnPart(ctx, current.ID); err != nil {
		return fmt.Errorf("end the part: %w", err)
	}
	if err := s.afterTurnPart(ctx, c, current); err != nil {
		return err
	}
	// A zone that catches who ends its turn inside (Grease) opens its saving throw here; the effects' own open right after, so both questions are open together.
	if err := s.zonesAtTurnEnd(ctx, c, current); err != nil {
		return err
	}
	// The saving throws the effects on the member ask at the end of its turn open now.
	return s.openEndSaves(ctx, c, current)
}

// partIsMasters says the line of current's part ending is the master's alone: a part of a group of
// NPCs alone is the master's, like the group (RN-20), and so is a hidden member's.
func partIsMasters(cs []playdb.Combatant, current playdb.Combatant) bool {
	holdsPlayer := slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.TurnState != turnIdle && inParty(o) })
	return current.Hidden || !holdsPlayer
}

// passTheTurn says where the turn goes once current's part ended: to the members of its group who
// still act, nowhere while the saves of the effects wait, or to the next group.
func (s *Service) passTheTurn(ctx context.Context, c *combatTx, cs []playdb.Combatant, current playdb.Combatant, out *endTurnOutcome) (any, error) {
	// Who still acts: the turn passes only when the last member ends.
	if acting := othersActing(cs, current.ID); len(acting) > 0 {
		out.secret = partIsMasters(cs, current)
		c.kind = eventTurnPartEnded
		if err := setCurrent(ctx, c, acting[0], c.enc.Round); err != nil {
			return nil, err
		}
		return actionEvent{Round: c.enc.Round, Secret: out.secret, Actor: current.ID}, nil
	}
	// The turn waits for the saving throws of the effects: it passes when they are answered.
	if held, err := s.endSaveOpen(ctx, c); err != nil {
		return nil, err
	} else if held {
		out.secret = partIsMasters(cs, current)
		c.kind = eventTurnPartEnded
		return actionEvent{Round: c.enc.Round, Secret: out.secret, Actor: current.ID}, nil
	}
	next, newRound, ok := nextTurnGroup(cs, current.ID, "")
	if !ok {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "no combatant can take a turn")
	}
	round := c.enc.Round
	if newRound {
		round++
	}
	// The next group's turn starts: every member's movement, action, bonus
	// action, dash and reaction come back.
	out.passed = true
	if err := startTurn(ctx, c, next, round); err != nil {
		return nil, err
	}
	return map[string]any{"from": current.ID, "to": next[0], "round": round, "discarded": len(out.dropped)}, nil
}

// publishEndTurn tells the streams what the turn ending changed.
func (s *Service) publishEndTurn(ctx context.Context, campaignID string, out *endTurnOutcome, d *encounterData) {
	s.publishTurnChanged(ctx, campaignID, d)
	s.publishEncounterChanged(ctx, campaignID, d.enc)
	if len(out.dropped) > 0 { // the attacks they belong to show them dropped
		s.publishLogChanged(ctx, campaignID, d.enc.ID, !touchesHidden(d.cs, out.dropped))
	}
	if !out.passed { // a part ended: its line, which a hidden member keeps from the players
		s.publishLogChanged(ctx, campaignID, d.enc.ID, !out.secret)
	}
}
