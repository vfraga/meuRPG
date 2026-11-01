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
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The handlers that set a combat up and keep its roster: the initiative, the order of a tie,
// the begin, the hiding of an NPC, and who joins and leaves. Each reads its request, writes in
// one transaction and tells the streams after the commit.

// minTiedCombatants is how few combatants an order of a tie can name.
const minTiedCombatants = 2

// initiativeCall is what SubmitInitiative reads from its request, checked.
type initiativeCall struct {
	m           authz.Membership
	key         string
	encounterID string
	combatantID string
	in          rollInput // the app rolls the d20, or the physical die the player typed
	msg         *playv1.SubmitInitiativeRequest
}

// SubmitInitiative implements playv1connect.CombatServiceHandler.
func (s *Service) SubmitInitiative(
	ctx context.Context,
	req *connect.Request[playv1.SubmitInitiativeRequest],
) (*connect.Response[playv1.SubmitInitiativeResponse], error) {
	call, err := readInitiativeCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	// RN-18: only a mode the master forced binds a player; with "each player
	// chooses" they pick on every roll. The master rolls either way, for anyone.
	var force DiceForce
	if !viewerOf(call.m).master {
		if force, err = s.dice.ForcedDice(ctx, nil, call.m.CampaignID, call.m.UserID); err != nil {
			return nil, s.dbError(ctx, "read the dice setting", err)
		}
	}

	var held *playv1.HeldRollOffer
	var regained *playv1.CharacterVitals // the pool that returned a use on initiative
	res, err := s.write(ctx, combatWrite{m: call.m, key: call.key, hash: idem.Hash(req.Msg), kind: eventInitiativeSubmitted, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		held, regained = nil, nil
		return s.initiativeWrite(ctx, c, call, force, &held, &regained)
	})
	if err != nil {
		return nil, s.dbError(ctx, "submit an initiative", err)
	}
	out, err := s.finish(ctx, call.m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, call.m.CampaignID, d.enc)
		s.publishVitals(call.m.CampaignID, regained)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SubmitInitiativeResponse{Encounter: out, HeldRollOffer: held}), nil
}

// readInitiativeCall reads the request in the order the answers are given: who may call,
// the key, the encounter, the combatant, then the roll.
func readInitiativeCall(ctx context.Context, req *playv1.SubmitInitiativeRequest) (initiativeCall, error) {
	m, err := heldMember(ctx, req.GetCampaignId()) // a held roll's answer runs this again as the roller
	if err != nil {
		return initiativeCall{}, err
	}
	key, encID, err := parseCall(req.GetIdempotencyKey(), req.GetEncounterId())
	if err != nil {
		return initiativeCall{}, err
	}
	call := initiativeCall{m: m, key: key, encounterID: encID, msg: req}
	if call.combatantID, err = parseCombatID(req.GetCombatantId(), "combatant"); err != nil {
		return initiativeCall{}, err
	}
	switch roll := req.GetRoll().(type) {
	case *playv1.SubmitInitiativeRequest_RollInApp:
		if !roll.RollInApp {
			return initiativeCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		call.in.inApp = true
	case *playv1.SubmitInitiativeRequest_D20Face:
		call.in.typed = int(roll.D20Face)
	default:
		return initiativeCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face"))
	}
	return call, nil
}

// initiativeWrite is the transaction of SubmitInitiative: who rolls, the roll, where it is
// stored, and the new order.
func (s *Service) initiativeWrite(ctx context.Context, c *combatTx, call initiativeCall, force DiceForce, held **playv1.HeldRollOffer, regained **playv1.CharacterVitals) (any, error) {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	v := c.viewer(call.m, cs) // the fog: an NPC the player does not see is not found
	target, err := findCombatant(cs, call.combatantID, v)
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(target); err != nil {
		return nil, err
	}
	if err := mayRollInitiative(c.enc, v, target, force, call.in.inApp); err != nil {
		return nil, err
	}
	gate, err := s.initiativeGate(ctx, c, call, target)
	if err != nil {
		return nil, err
	}
	if gate.Held != nil {
		_, _, *held = s.outsideOffers(ctx, c.tx, gate.Held)
		return nil, nil
	}
	extra, err := s.initiativeManeuver(ctx, c, call, target)
	if err != nil {
		return nil, err
	}
	total, f := conv.Int32(gate.Roll.Total)+extra.face, clamp32(gate.Roll.Face(), 1, 20)
	if err := saveInitiative(ctx, c, cs, target, total, f); err != nil {
		return nil, err
	}
	if _, err := saveOrder(ctx, c.q, cs, orderCombatants(cs)); err != nil {
		return nil, err
	}
	if *regained, err = s.regainOnInitiative(ctx, c, target); err != nil { // a pool that recharges on initiative
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	return initiativeEvent(target, f, total, gate.Roll.Physical, extra), nil
}

// initiativeEvent is the payload of the initiative event; a table die added to it is named with its face.
func initiativeEvent(target playdb.Combatant, face, total int32, physical bool, die initiativeDie) map[string]any {
	ev := map[string]any{"combatant_id": target.ID, "face": face, "total": total, "physical": physical}
	if die.key != "" {
		ev["maneuver_key"], ev["maneuver_face"] = die.key, die.face
	}
	return ev
}

// mayRollInitiative checks that the combat is in SETUP and, for a player, that the initiative
// is not set and the way of rolling is the table's.
func mayRollInitiative(enc playdb.Encounter, v combatViewer, target playdb.Combatant, force DiceForce, inApp bool) error {
	if err := notEnded(enc); err != nil {
		return err
	}
	if enc.Status != statusSetup {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_IN_SETUP, "initiative is rolled before the combat begins")
	}
	if v.master {
		return nil
	}
	if target.Initiative != nil {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_INITIATIVE_ALREADY_SET, "your initiative is already set")
	}
	if force.refuses(inApp) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
	}
	return nil
}

// initiativeGate rolls the d20 of an initiative, or keeps it for a question: initiative is a Dexterity check (SRD 5.1), so
// a character that holds a Bardic Inspiration die, or a halfling whose d20 shows a 1, is asked before the total is set.
// The combat is in SETUP, so the die is the character's own (it carries into the combat when it begins). A creature, an
// NPC and the master's roll for them wait for nobody.
func (s *Service) initiativeGate(ctx context.Context, c *combatTx, call initiativeCall, target playdb.Combatant) (gateResult, error) {
	spec := rollSpec{
		Kind: heldInitiative, Key: call.key, Request: call.msg, In: call.in, Modifier: int(target.InitiativeBonus), Mode: combat.ModeNormal,
		Roller: heldRoller{CampaignID: call.m.CampaignID, CharacterID: target.CharacterID, UserID: call.m.UserID},
	}
	if target.Kind == kindPlayer && backedByCharacter(target) {
		sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, target)
		if err != nil {
			return gateResult{}, err
		}
		spec.Lucky, spec.MayAsk = sheet.Traits.Lucky, playedBy(target, call.m)
	}
	return s.heldGate(ctx, s.inspirationGate(c), spec)
}

// saveInitiative stores the roll on the combatant, or on the group of creatures it rolled
// for, and in cs.
func saveInitiative(ctx context.Context, c *combatTx, cs []playdb.Combatant, target playdb.Combatant, total, f int32) error {
	if isCreature(target) {
		return saveGroupInitiative(ctx, c, cs, target, total, f)
	}
	if err := c.q.SetCombatantInitiative(ctx, playdb.SetCombatantInitiativeParams{ID: target.ID, Initiative: &total, InitiativeFace: &f}); err != nil {
		return fmt.Errorf("save the initiative: %w", err)
	}
	for i := range cs {
		if cs[i].ID == target.ID {
			cs[i].Initiative, cs[i].InitiativeFace, cs[i].TieOrdered = &total, &f, false
		}
	}
	return nil
}

// saveGroupInitiative stores the roll of the creatures of one casting (RN-18): every member of
// the group takes the roll, with the bonus of the one that rolled, so they take a joint turn.
func saveGroupInitiative(ctx context.Context, c *combatTx, cs []playdb.Combatant, target playdb.Combatant, total, f int32) error {
	if err := c.q.SetGroupInitiative(ctx, playdb.SetGroupInitiativeParams{
		EncounterID: c.enc.ID, SummonGroupID: conv.OrZero(target.SummonGroupID), Initiative: &total, InitiativeFace: &f, InitiativeBonus: target.InitiativeBonus,
	}); err != nil {
		return fmt.Errorf("save the group's initiative: %w", err)
	}
	for i := range cs {
		if isCreature(cs[i]) && conv.OrZero(cs[i].SummonGroupID) == conv.OrZero(target.SummonGroupID) {
			cs[i].Initiative, cs[i].InitiativeFace, cs[i].InitiativeBonus, cs[i].TieOrdered = &total, &f, target.InitiativeBonus, false
		}
	}
	return nil
}

// orderCall is what SetInitiativeOrder reads from its request, checked.
type orderCall struct {
	m           authz.Membership
	key         string
	encounterID string
	ids         []string
}

// SetInitiativeOrder implements playv1connect.CombatServiceHandler.
func (s *Service) SetInitiativeOrder(
	ctx context.Context,
	req *connect.Request[playv1.SetInitiativeOrderRequest],
) (*connect.Response[playv1.SetInitiativeOrderResponse], error) {
	call, err := readOrderCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: call.m, key: call.key, hash: idem.Hash(req.Msg), kind: eventInitiativeOrderSet, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return s.orderWrite(ctx, c, call)
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the initiative order", err)
	}
	out, err := s.finish(ctx, call.m, res, s.changed(call.m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetInitiativeOrderResponse{Encounter: out}), nil
}

// readOrderCall reads the request in the order the answers are given: who may call, the
// key, the encounter, then the combatants, each once and at least two.
func readOrderCall(ctx context.Context, req *playv1.SetInitiativeOrderRequest) (orderCall, error) {
	m, key, encID, err := masterCall(ctx, req.GetCampaignId(), req.GetIdempotencyKey(), req.GetEncounterId())
	if err != nil {
		return orderCall{}, err
	}
	ids, err := orderIDs(req.GetCombatantIds())
	if err != nil {
		return orderCall{}, err
	}
	if len(ids) < minTiedCombatants {
		return orderCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("combatant_ids must name at least two combatants"))
	}
	return orderCall{m: m, key: key, encounterID: encID, ids: ids}, nil
}

// orderIDs reads the combatants of SetInitiativeOrder: at most maxCombatants, each once.
func orderIDs(raw []string) ([]string, error) {
	if len(raw) > maxCombatants {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("combatant_ids must have at most %d entries", maxCombatants))
	}
	ids := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, r := range raw {
		id, err := parseCombatID(r, "combatant")
		if err != nil {
			return nil, err
		}
		if _, dup := seen[id]; dup {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("combatant_ids repeats a combatant"))
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

// orderWrite is the transaction of SetInitiativeOrder: the combatants must be tied, and take
// the order the master gave.
func (s *Service) orderWrite(ctx context.Context, c *combatTx, call orderCall) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	if err := mustBeTied(cs, call.ids, call.m); err != nil {
		return nil, err
	}
	if _, err := saveOrder(ctx, c.q, cs, withOrder(orderCombatants(cs), call.ids)); err != nil {
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	return map[string]any{"combatant_ids": call.ids}, nil
}

// mustBeTied checks that every combatant named is in the combat and tied with the first.
func mustBeTied(cs []playdb.Combatant, ids []string, m authz.Membership) error {
	var first playdb.Combatant
	for i, id := range ids {
		found, err := findCombatant(cs, id, viewerOf(m))
		if err != nil {
			return err
		}
		if i == 0 {
			first = found
		}
		if !sameTie(first, found) {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("combatant_ids must be tied: the same initiative total and bonus"))
		}
	}
	return nil
}

// BeginCombat implements playv1connect.CombatServiceHandler.
func (s *Service) BeginCombat(
	ctx context.Context,
	req *connect.Request[playv1.BeginCombatRequest],
) (*connect.Response[playv1.BeginCombatResponse], error) {
	m, key, encID, err := masterCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatBegun, encounterID: encID}, func(c *combatTx) (any, error) {
		return s.beginWrite(ctx, c)
	})
	if err != nil {
		return nil, s.dbError(ctx, "begin a combat", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishTurnChanged(ctx, m.CampaignID, d)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.BeginCombatResponse{Encounter: out}), nil
}

// beginWrite is the transaction of BeginCombat: everybody must have an initiative, the NPCs
// take their squares, and the first group starts its turn.
func (s *Service) beginWrite(ctx context.Context, c *combatTx) (any, error) {
	if c.enc.Status != statusSetup {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_IN_SETUP, "the combat already began or ended")
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the combat has no combatants"))
	}
	if err := initiativesMissing(cs); err != nil {
		return nil, err
	}
	// The first group starts its turn: every living member acts at once.
	first, _, ok := nextTurnGroup(cs, "", "")
	if !ok {
		first = []string{cs[0].ID} // everybody defeated: the first of the order starts
	}
	// A combat on a map starts with every NPC on it, away from the players.
	if cs, err = s.placeUnplaced(ctx, c, cs); err != nil {
		return nil, err
	}
	started := c.now
	c.enc.StartedAt = &started
	// No sight carries into the fight: it is an action of a turn there (MR-036).
	for _, member := range cs {
		if err := s.endSight(ctx, c, member, sightCombatJoined, false); err != nil {
			return nil, err
		}
	}
	if err := startTurn(ctx, c, first, 1); err != nil {
		return nil, err
	}
	return map[string]any{"round": 1, "current_combatant_id": first[0], "turn_group_ids": first}, nil
}

// initiativesMissing refuses a combat in which somebody has no initiative, naming who.
func initiativesMissing(cs []playdb.Combatant) error {
	var missing, labels []string
	for _, cb := range cs {
		if cb.Initiative == nil {
			missing, labels = append(missing, cb.ID), append(labels, cb.Label)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	// Only the master calls this, so the message may name who is missing.
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_INITIATIVE_MISSING,
		fmt.Sprintf("missing the initiative of: %v", labels), func(b *playv1.EncounterBlocked) { b.CombatantIds = missing })
}

// SetCombatantHidden implements playv1connect.CombatServiceHandler.
func (s *Service) SetCombatantHidden(
	ctx context.Context,
	req *connect.Request[playv1.SetCombatantHiddenRequest],
) (*connect.Response[playv1.SetCombatantHiddenResponse], error) {
	m, key, encID, err := masterCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	hidden := req.Msg.GetHidden()

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantHiddenSet, encounterID: encID}, func(c *combatTx) (any, error) {
		return s.hiddenWrite(ctx, c, m, combID, hidden)
	})
	if err != nil {
		return nil, s.dbError(ctx, "hide or show a combatant", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishHiddenSet(ctx, m.CampaignID, d, combID)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetCombatantHiddenResponse{Encounter: out}), nil
}

// hiddenWrite is the transaction of SetCombatantHidden: only an NPC hides.
func (s *Service) hiddenWrite(ctx context.Context, c *combatTx, m authz.Membership, combID string, hidden bool) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	target, err := findCombatant(cs, combID, viewerOf(m))
	if err != nil {
		return nil, err
	}
	if target.Kind != kindNPC {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a player's combatant is never hidden"))
	}
	if err := c.q.SetCombatantHidden(ctx, playdb.SetCombatantHiddenParams{ID: target.ID, Hidden: hidden}); err != nil {
		return nil, fmt.Errorf("hide the combatant: %w", err)
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	return map[string]any{"combatant_id": target.ID, "hidden": hidden, "round": c.enc.Round}, nil
}

// publishHiddenSet tells the streams a combatant was hidden or shown.
func (s *Service) publishHiddenSet(ctx context.Context, campaignID string, d *encounterData, combID string) {
	s.publishEncounterChanged(ctx, campaignID, d.enc)
	s.publishLogChanged(ctx, campaignID, d.enc.ID, false) // the master's line only
	// If it is in the turn, the players' copy of the turn changes.
	if i := slices.IndexFunc(d.cs, func(o playdb.Combatant) bool { return o.ID == combID }); i >= 0 && inTurn(d.enc, d.cs[i]) {
		s.publishTurnChanged(ctx, campaignID, d)
	}
}

// AddCombatants implements playv1connect.CombatServiceHandler.
func (s *Service) AddCombatants(
	ctx context.Context,
	req *connect.Request[playv1.AddCombatantsRequest],
) (*connect.Response[playv1.AddCombatantsResponse], error) {
	m, key, encID, err := masterCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	parts, err := s.participants(ctx, m.CampaignID, req.Msg.GetParticipants(), false, false)
	if err != nil {
		return nil, err
	}

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantsAdded, encounterID: encID}, func(c *combatTx) (any, error) {
		return s.addWrite(ctx, c, parts)
	})
	if err != nil {
		return nil, s.dbError(ctx, "add combatants", err)
	}
	out, err := s.finish(ctx, m, res, s.changed(m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AddCombatantsResponse{Encounter: out}), nil
}

// addWrite is the transaction of AddCombatants.
func (s *Service) addWrite(ctx context.Context, c *combatTx, parts []planned) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	if err := mayJoin(c.enc, cs, parts); err != nil {
		return nil, err
	}
	_, added, err := s.addParticipants(ctx, c, link.Grid{Columns: c.enc.GridColumns, Rows: c.enc.GridRows}, cs, parts)
	if err != nil {
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	return map[string]any{"added": len(added)}, nil
}

// mayJoin checks that the participants fit in the combat: a player's character joins only
// before the combat begins, and once, and the room is at most maxCombatants.
func mayJoin(enc playdb.Encounter, cs []playdb.Combatant, parts []planned) error {
	count := len(cs)
	for _, p := range parts {
		count += p.count
		if !p.char.Player {
			continue
		}
		// A player's character cannot be rolled for later in the order.
		if enc.Status != statusSetup || slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.CharacterID == p.char.ID }) {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("a player's character joins only before the combat begins, and once"))
		}
	}
	if count > maxCombatants {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a combat has at most %d combatants", maxCombatants))
	}
	return nil
}

// RemoveCombatant implements playv1connect.CombatServiceHandler.
func (s *Service) RemoveCombatant(
	ctx context.Context,
	req *connect.Request[playv1.RemoveCombatantRequest],
) (*connect.Response[playv1.RemoveCombatantResponse], error) {
	m, key, encID, err := masterCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}

	var turnPassed bool
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantRemoved, encounterID: encID}, func(c *combatTx) (any, error) {
		turnPassed = false
		return s.removeWrite(ctx, c, m, combID, &turnPassed)
	})
	if err != nil {
		return nil, s.dbError(ctx, "remove a combatant", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		if turnPassed {
			s.publishTurnChanged(ctx, m.CampaignID, d)
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RemoveCombatantResponse{Encounter: out}), nil
}

// removeWrite is the transaction of RemoveCombatant: the checks, the creatures that go with
// the combatant, the turn, and the rows.
func (s *Service) removeWrite(ctx context.Context, c *combatTx, m authz.Membership, combID string, turnPassed *bool) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	target, err := findCombatant(cs, combID, viewerOf(m))
	if err != nil {
		return nil, err
	}
	if err := mayLeave(ctx, c, target); err != nil {
		return nil, err
	}
	if cs, err = s.releaseCreatures(ctx, c, cs, target); err != nil {
		return nil, err
	}
	// It leaves the turn first; the turn passes when nobody who acts is left.
	if *turnPassed, err = leaveTurn(ctx, c, cs, target); err != nil {
		return nil, err
	}
	*turnPassed = *turnPassed || inTurn(c.enc, target)
	// A character keeps what is on it: the effects go back to game time before the row
	// (and the effects with it) is deleted.
	if err := s.effectRowsToCharacters(ctx, c, cs, []playdb.Combatant{target}); err != nil {
		return nil, err
	}
	if err := c.q.DeleteCombatant(ctx, target.ID); err != nil {
		return nil, fmt.Errorf("delete the combatant: %w", err)
	}
	rest := slices.DeleteFunc(slices.Clone(cs), func(o playdb.Combatant) bool { return o.ID == target.ID })
	if _, err := saveOrder(ctx, c.q, cs, rest); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	return map[string]any{"combatant_id": target.ID}, nil
}

// mayLeave checks that the combatant may leave: not a player in a combat that is running,
// and not while a damage still names it.
func mayLeave(ctx context.Context, c *combatTx, target playdb.Combatant) error {
	if target.Kind == kindPlayer && c.enc.Status == statusActive {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_PLAYER_IN_COMBAT, "a player's combatant cannot leave a combat that is running")
	}
	// A damage still to roll or to apply that names the combatant (as the attacker or
	// as the target) goes with it, and the log would keep an attack whose damage never
	// lands: the master rolls, applies or discards it first.
	open, err := c.q.ListOpenPendingDamages(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the pending damage: %w", err)
	}
	if slices.ContainsFunc(open, func(p playdb.PendingDamage) bool {
		return p.TargetID == target.ID || conv.OrZero(p.AttackerID) == target.ID
	}) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_PENDING_DAMAGE, "a damage is still to roll or to apply")
	}
	return nil
}

// releaseCreatures writes back the hit points of the creatures that leave with the combatant,
// and returns the combatants that are left. A player's character that leaves a combat in
// SETUP takes its creatures out with it; a creature that leaves keeps the hit points it has.
func (s *Service) releaseCreatures(ctx context.Context, c *combatTx, cs []playdb.Combatant, target playdb.Combatant) ([]playdb.Combatant, error) {
	var err error
	if target.Kind == kindPlayer {
		owned := slices.DeleteFunc(slices.Clone(cs), func(o playdb.Combatant) bool { return !isCreature(o) || o.CharacterID != target.CharacterID })
		if len(owned) > 0 {
			if err := s.writeBackCreatures(ctx, c, owned); err != nil {
				return nil, err
			}
			if cs, _, err = s.dropCombatants(ctx, c, cs, owned, false); err != nil {
				return nil, err
			}
		}
	}
	if isCreature(target) {
		if err := s.writeBackCreatures(ctx, c, []playdb.Combatant{target}); err != nil {
			return nil, err
		}
	}
	return cs, nil
}
