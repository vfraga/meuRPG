package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The combat without a grid, the "teatro da mente" (MR-025, RN-25, ADR-0017).
//
// A combat has a mode, chosen when it starts and never changed (encounters.mode).
// In `grid` mode (what every combat was) the server knows where everybody stands.
// In `theatre` mode it knows nothing about places: no combatant ever has a square
// (grid_col and grid_row stay NULL), the encounter has no map and a grid of 0 by 0,
// and the server never checks a player's reach, range or distance, because the
// master judges them. What is left of movement is a number (SpendMovement), and
// the opportunity attack is something the master offers (OfferOpportunity).
//
// The paths that read positions each know the mode, and say so where they do:
// RollAttack and CastSpell skip the reach checks, the target lists carry no
// distance and no "too far", GetMoveOptions answers empty, MoveCombatant, the traps
// and the placement of summoned creatures are refused or left out. This file has
// the new calls and the helpers the others share.

// The two modes as the table stores them (encounters_mode_valid).
const (
	modeGrid    = "grid"
	modeTheatre = "theatre"
)

// isTheatre says the combat is played without a grid.
func isTheatre(e playdb.Encounter) bool { return e.Mode == modeTheatre }

// modeProto is a combat's mode as the API says it. A combat that says none (a
// zero value, which no stored combat is) is a grid one.
func modeProto(mode string) playv1.EncounterMode {
	if mode == modeTheatre {
		return playv1.EncounterMode_ENCOUNTER_MODE_THEATRE
	}
	return playv1.EncounterMode_ENCOUNTER_MODE_GRID
}

// errTheatreOnly is the refusal of what only a combat without a grid has.
func errTheatreOnly() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_THEATRE_ONLY,
		"this is for a combat without a map: on a map, move on it")
}

// errNeedsAMap is the refusal of what a combat without a grid cannot have: it has
// no squares.
func errNeedsAMap() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NEEDS_A_MAP,
		"this needs a map: a combat without one has no squares")
}

// valueOf is what the pointer points to, the zero value for nil.
func valueOf[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// modeOfStart works out the mode a new combat is played in: the one the request
// chose, or, when it chose none, the table's rule "combate com mapa" (RN-24), read
// in the caller's transaction (tx). Without the rule's source, every combat is on a
// map, as it was before the modes.
func (s *Service) modeOfStart(ctx context.Context, tx pgx.Tx, campaignID string, asked playv1.EncounterMode) (string, error) {
	switch asked {
	case playv1.EncounterMode_ENCOUNTER_MODE_GRID:
		return modeGrid, nil
	case playv1.EncounterMode_ENCOUNTER_MODE_THEATRE:
		return modeTheatre, nil
	case playv1.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED:
	}
	if s.defaults == nil {
		return modeGrid, nil
	}
	without, err := s.defaults.CombatWithoutMap(ctx, tx, campaignID)
	if err != nil {
		return "", fmt.Errorf("read the table's rule for combat: %w", err)
	}
	if without {
		return modeTheatre, nil
	}
	return modeGrid, nil
}

// theatreRunning says the session has a combat without a grid that is not ended.
// A trap, a door or a fog belongs to a map, and while such a combat is on the table
// nothing of the map's happens (RN-25), whichever map is current.
func theatreRunning(ctx context.Context, q *playdb.Queries, sessionID string) (bool, error) {
	enc, err := q.GetLatestEncounter(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find the session's combat: %w", err)
	}
	return enc.Status != statusEnded && isTheatre(enc), nil
}

// keySeen says the session's history already holds an event written under the key:
// a retry then gets the stored answer, whatever the table is like now.
func keySeen(ctx context.Context, q *playdb.Queries, sessionID, key string) (bool, error) {
	_, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: sessionID, IdempotencyKey: &key})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// SpendMovement implements playv1connect.CombatServiceHandler.
func (s *Service) SpendMovement(
	ctx context.Context,
	req *connect.Request[playv1.SpendMovementRequest],
) (*connect.Response[playv1.SpendMovementResponse], error) {
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	ft := req.Msg.GetDistanceFt()
	if ft < 1 || ft > maxSpendFt {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("distance_ft must be 1 to %d", maxSpendFt))
	}
	v := viewerOf(m)

	var made actionEvent
	var hidden, onTurn bool
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantMoved, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs)
		target, err := findCombatant(cs, combID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(target); err != nil {
			return nil, err
		}
		if !isTheatre(c.enc) {
			return nil, errTheatreOnly() // on a map, the movement is a move on it
		}
		if c.enc.Status != statusActive {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
		}
		onTurn = actsNow(c.enc, target)
		if !v.master {
			// RN-21: a player moves only on their own turn, and not while an
			// opportunity attack on them waits for its answer.
			if !onTurn {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "it is not your turn")
			}
			if err := s.mustNotWait(ctx, c, target); err != nil {
				return nil, err
			}
		}
		// Never more than the movement the turn has (speed, Dash): the same limit a
		// move on a map has, for the master too. The path is not judged.
		cost := int(ft) * 10
		if left := movementLeftDFt(target); cost > left {
			return nil, tooFar(cost - left)
		}
		used := int(target.MovementUsedDft) + cost
		if err := c.q.SetCombatantMove(ctx, playdb.SetCombatantMoveParams{
			ID: target.ID, GridCol: nil, GridRow: nil,
			MovementUsedFt: clamp32(used/10, 0, math.MaxInt32), MovementUsedDft: clamp32(used, 0, math.MaxInt32),
			LastMoveDft: target.LastMoveDft, CoverMark: target.CoverMark, // nobody moved on a map: the master's cover mark stays
		}); err != nil {
			return nil, fmt.Errorf("spend the movement: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &target.CharacterID
		hidden = target.Hidden
		// A combatant_moved event, so the log, the undo and the combat's history
		// treat the spent movement as the move it is: how far it went and what it
		// had walked before, which the undo puts back. There is no square.
		made = actionEvent{
			Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID, OnTurn: onTurn, From: moveStateOf(target),
			CostFt: ft, CostDFt: clamp32(cost, 0, math.MaxInt32), DistanceFt: ft, DistanceDFt: clamp32(cost, 0, math.MaxInt32),
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "spend movement", err)
	}
	var left int32
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		if onTurn {
			s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !hidden)
		}
	})
	if err != nil {
		return nil, err
	}
	// What is left, from the combatant as the caller reads it: a replay answers with
	// what is left now, as every read does.
	if i := slices.IndexFunc(out.GetCombatants(), func(c *playv1.Combatant) bool { return c.GetId() == combID }); i >= 0 {
		left = out.GetCombatants()[i].GetMovementLeftDft()
	}
	return connect.NewResponse(&playv1.SpendMovementResponse{Encounter: out, MovementLeftDft: left}), nil
}

// maxSpendFt is the most a call spends: 600 ft, twice the largest speed a
// combatant has, so a typo cannot overflow the arithmetic.
const maxSpendFt = 600

// OfferOpportunity implements playv1connect.CombatServiceHandler.
func (s *Service) OfferOpportunity(
	ctx context.Context,
	req *connect.Request[playv1.OfferOpportunityRequest],
) (*connect.Response[playv1.OfferOpportunityResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
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
	moverID, err := parseCombatID(req.Msg.GetMoverId(), "combatant")
	if err != nil {
		return nil, err
	}
	reactorID, err := parseCombatID(req.Msg.GetReactorId(), "combatant")
	if err != nil {
		return nil, err
	}
	if moverID == reactorID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a combatant cannot leave its own reach"))
	}

	var made actionEvent
	var mover playdb.Combatant
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventOpportunityOffered, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		if !isTheatre(c.enc) {
			return nil, errTheatreOnly() // on a map, the server finds the opportunity attacks
		}
		if c.enc.Status != statusActive {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		master := combatViewer{master: true}
		if mover, err = findCombatant(cs, moverID, master); err != nil {
			return nil, err
		}
		reactor, err := findCombatant(cs, reactorID, master)
		if err != nil {
			return nil, err
		}
		// The offer is made on the mover's turn, like the offers a move makes.
		if !actsNow(c.enc, mover) {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "the mover is not on turn")
		}
		if err := s.checkReactor(ctx, c, reactor, mover); err != nil {
			return nil, err
		}
		pending, err := c.q.ListPendingOpportunityOffers(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the opportunity offers: %w", err)
		}
		if slices.ContainsFunc(pending, func(o playdb.OpportunityOffer) bool { return o.MoverID == mover.ID && o.ReactorID == reactor.ID }) {
			return nil, errNoOpportunity("the same offer already waits")
		}
		moveID := uuid.New().String()
		offer, err := c.q.InsertOpportunityOffer(ctx, playdb.InsertOpportunityOfferParams{
			EncounterID: c.enc.ID, MoveID: moveID, MoverID: mover.ID, ReactorID: reactor.ID, CreatedAt: c.now, // no square: nobody left one
		})
		if err != nil {
			return nil, fmt.Errorf("offer the opportunity attack: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &reactor.CharacterID
		// ByHand: an undo never takes the offer back (it is withdrawn), and it
		// closes the chain of what can be undone, which a move's offers do not.
		made = actionEvent{
			Round: c.enc.Round, Secret: mover.Hidden || reactor.Hidden, Actor: mover.ID, Target: reactor.ID,
			OfferID: offer.ID, MoveID: moveID, ByHand: true,
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "offer an opportunity attack", err)
	}
	out, err := s.finish(ctx, m, res, func(_ context.Context, d *encounterData) {
		s.publishOffersMade(m.CampaignID, d, mover, []string{reactorID})
	})
	if err != nil {
		return nil, err
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the offer", err)
	}
	return connect.NewResponse(&playv1.OfferOpportunityResponse{Encounter: out, OpportunityOfferId: ev.OfferID}), nil
}

// errNoOpportunity is the refusal of an offer that cannot be made now.
func errNoOpportunity(msg string) error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_OPPORTUNITY, msg)
}

// checkReactor says whether the reactor may be offered an opportunity attack on the
// mover, by the rules a move's offers follow (opportunityReactors): hostile to the
// mover, awake, with its reaction, able to attack with it and holding a melee
// attack; and a mover that is still there to be attacked (not defeated, not hidden,
// not after the Disengage action). It does not look at places: in a combat without a
// grid the master says who left whose reach.
func (s *Service) checkReactor(ctx context.Context, c *combatTx, reactor, mover playdb.Combatant) error {
	switch {
	case mover.Defeated:
		return errNoOpportunity("the mover is defeated")
	case mover.Hidden:
		return errNoOpportunity("the mover is hidden") // a player is never offered an attack on what they do not see (RN-10)
	case mover.Disengaged:
		return errNoOpportunity("the mover took the Disengage action")
	case reactor.Defeated:
		return errNoOpportunity("the reactor is defeated")
	case reactor.Side == mover.Side:
		return errNoOpportunity("the reactor is not hostile to the mover")
	case slices.ContainsFunc(reactor.Conditions, func(k string) bool { return slices.Contains(cantReact, k) }):
		return errNoOpportunity("the reactor cannot react now")
	case reactor.ReactionUsed:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is used")
	}
	if err := mayAttack(reactor, true); err != nil {
		return err
	}
	down, err := s.isDown(ctx, c.tx, c.session.CampaignID, reactor)
	if err != nil {
		return err
	}
	if down {
		return errNoOpportunity("the reactor is down")
	}
	sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, reactor)
	if err != nil {
		return err
	}
	if meleeReachOf(sheet) == 0 {
		return errNoOpportunity("the reactor has no melee attack")
	}
	return nil
}

// WithdrawOpportunity implements playv1connect.CombatServiceHandler.
func (s *Service) WithdrawOpportunity(
	ctx context.Context,
	req *connect.Request[playv1.WithdrawOpportunityRequest],
) (*connect.Response[playv1.WithdrawOpportunityResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	out, err := s.answerOffer(ctx, m, req.Msg.GetEncounterId(), req.Msg.GetOpportunityOfferId(), req.Msg.GetIdempotencyKey(), offerWithdrawn)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.WithdrawOpportunityResponse{Encounter: out}), nil
}
