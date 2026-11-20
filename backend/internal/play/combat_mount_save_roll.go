package play

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// mountSaveCall is a RollMountSave request once it is read.
type mountSaveCall struct {
	m                   authz.Membership
	key, encID, riderID string
	in                  rollInput
	msg                 *playv1.RollMountSaveRequest
}

// readMountSaveCall reads a RollMountSave request, refusing in this order: the caller, the keys, the ids, the way of rolling.
func readMountSaveCall(ctx context.Context, msg *playv1.RollMountSaveRequest) (mountSaveCall, error) {
	call := mountSaveCall{msg: msg}
	var err error
	if call.m, err = heldMember(ctx, msg.GetCampaignId()); err != nil {
		return call, err
	}
	if call.key, call.encID, err = parseCall(msg.GetIdempotencyKey(), msg.GetEncounterId()); err != nil {
		return call, err
	}
	if call.riderID, err = parseCombatID(msg.GetRiderId(), "combatant"); err != nil {
		return call, err
	}
	bad := func(text string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(text)) }
	switch roll := msg.GetRoll().(type) {
	case *playv1.RollMountSaveRequest_RollInApp:
		if !roll.RollInApp {
			return call, bad("roll_in_app must be true")
		}
		call.in.inApp = true
	case *playv1.RollMountSaveRequest_D20Face:
		call.in.typed = int(roll.D20Face)
		if call.in.typed < 1 || call.in.typed > 20 {
			return call, bad("d20_face must be 1 to 20")
		}
		if msg.SecondD20Face != nil {
			call.in.typedFaces = []int{call.in.typed, int(msg.GetSecondD20Face())}
		}
	default:
		return call, bad("set roll_in_app or d20_face")
	}
	return call, nil
}

// mountSaveRun is what a try of the change leaves for the answer. A retry starts it over.
type mountSaveRun struct {
	made   actionEvent
	result *playv1.MountSaveResult
	offer  *playv1.OutsideInspirationOffer
	lucky  *playv1.LuckyOffer
	held   *playv1.HeldRollOffer
}

func (r *mountSaveRun) asked() bool { return r.offer != nil || r.lucky != nil || r.held != nil }

// RollMountSave implements playv1connect.CombatServiceHandler.
func (s *Service) RollMountSave(ctx context.Context, req *connect.Request[playv1.RollMountSaveRequest]) (*connect.Response[playv1.RollMountSaveResponse], error) {
	call, err := readMountSaveCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var run mountSaveRun
	res, err := s.write(ctx, combatWrite{m: call.m, key: call.key, hash: idem.Hash(req.Msg), kind: eventMountSaveRolled, encounterID: call.encID}, func(c *combatTx) (any, error) {
		run = mountSaveRun{}
		return s.writeMountSave(ctx, c, call, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll a mount save", err)
	}
	if run.asked() { // only the held roll was written: the question is the whole answer, and the combat's screens read it
		out, err := s.finish(ctx, call.m, res, s.changed(call.m.CampaignID))
		if err != nil {
			return nil, err
		}
		return connect.NewResponse(&playv1.RollMountSaveResponse{Encounter: out, HeldRollOffer: run.held}), nil
	}
	out, err := s.finish(ctx, call.m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, call.m.CampaignID, d.enc)
		s.publishLogChanged(ctx, call.m.CampaignID, d.enc.ID, !run.made.Secret)
	})
	if err != nil {
		return nil, err
	}
	result := run.result
	if result == nil && res.repeated {
		result = mountSaveResultOfRetry(res.payload)
		if err := s.hideRetryMountResult(ctx, call.m, result); err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(&playv1.RollMountSaveResponse{Encounter: out, Result: result}), nil
}

// mountSaveResultOfRetry is the result a retry of the same request asks for again, read back from the event the first time wrote.
func mountSaveResultOfRetry(payload []byte) *playv1.MountSaveResult {
	var ev actionEvent
	if err := json.Unmarshal(payload, &ev); err != nil || ev.Reaction == nil {
		return nil
	}
	return mountSaveResultOf(ev)
}

// hideMountResult takes out of a player's result what the player does not read: the master's note that no square next to the mount was
// free, and the DC when the table does not show it (RN-24).
func hideMountResult(res *playv1.MountSaveResult, showDC bool) {
	if res == nil {
		return
	}
	res.NoRoomNextToMount = false
	if !showDC {
		res.Dc = 0
	}
}

// hideRetryMountResult is hideMountResult for the result of a retry, which reads the table's rule now.
func (s *Service) hideRetryMountResult(ctx context.Context, m authz.Membership, result *playv1.MountSaveResult) error {
	if result == nil || heldViewerIsMaster(ctx, m) {
		return nil
	}
	rules, err := s.tableRules(ctx, nil, m.CampaignID)
	if err != nil {
		return s.dbError(ctx, "read the table's rules", err)
	}
	hideMountResult(result, rules.ShowSaveDC)
	return nil
}

// mountSaveResultOf is the result a written save's event tells.
func mountSaveResultOf(ev actionEvent) *playv1.MountSaveResult {
	re := ev.Reaction
	out := &playv1.MountSaveResult{Dc: re.SaveDC, Held: re.Kept, AutoFail: re.AutoFail, LandedCol: re.LandCol, LandedRow: re.LandRow, NoRoomNextToMount: re.NoRoom}
	if ev.D20 == 0 {
		return out
	}
	faces := []int32{ev.D20}
	if ev.D20B != 0 {
		faces = []int32{ev.D20, ev.D20B}
		if ev.Counted == 1 {
			faces = []int32{ev.D20B, ev.D20}
		}
	}
	out.Save = ev.mark(diceRoll(clamp32(len(faces), 1, 2), 20, faces, re.SaveBonus, ev.Total, ev.Physical))
	out.Save.Rerolled = ev.rerolls()
	out.BonusDice = bonusDiceOf(ev)
	return out
}

// mountSaveWindowOf finds the open window of the rider's save, refusing a caller that may not answer it: a player sees only the
// riders it sees and answers only its own, and a rider that owes nothing is "not your turn to answer".
func (s *Service) mountSaveWindowOf(ctx context.Context, c *combatTx, call mountSaveCall) (playdb.ReactionWindow, playdb.Combatant, error) {
	var none playdb.ReactionWindow
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return none, playdb.Combatant{}, err
	}
	rider, err := findCombatant(cs, call.riderID, c.viewer(call.m, cs))
	if err != nil {
		return none, rider, err
	}
	if call.m.Role != authz.RoleMaster && (rider.UserID == nil || *rider.UserID != call.m.UserID || isCreature(rider)) {
		return none, rider, errWindowDenied()
	}
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return none, rider, err
	}
	i := slices.IndexFunc(open, func(w playdb.ReactionWindow) bool {
		return w.Kind == string(reaction.MountSave) && conv.OrZero(w.ReactorID) == rider.ID
	})
	if i < 0 {
		return none, rider, errNotYourTurnToAnswer()
	}
	w, reactor, err := s.windowToAnswer(ctx, c, call.m, open[i].ID)
	return w, reactor, err
}

// writeMountSave is the change of a rider's save, in the transaction: the window, the d20 (or the automatic failure), the fall on a
// failure, the window closed and the event.
func (s *Service) writeMountSave(ctx context.Context, c *combatTx, call mountSaveCall, run *mountSaveRun) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	w, rider, err := s.mountSaveWindowOf(ctx, c, call)
	if err != nil {
		return nil, err
	}
	t := windowTriggerOf(w)
	master := call.m.Role == authz.RoleMaster
	mount, err := s.mountOfWindow(ctx, c, t)
	if err != nil {
		return nil, err
	}
	ev := actionEvent{Round: c.enc.Round, Secret: rider.Hidden || mount.Hidden, Actor: rider.ID, Target: mount.ID}
	ev.Reaction = &reactionEvent{Kind: string(reaction.MountSave), Window: w.ID, ByMaster: master, SaveDC: mountSaveDC, Cause: t.Cause}
	held, err := s.mountSaveOutcome(ctx, c, call, rider, &ev, run)
	if err != nil {
		return nil, err
	}
	if run.asked() { // the d20 is kept for a question: nothing is written
		return nil, nil
	}
	ev.Reaction.Kept = held
	if !held {
		if err := s.throwRider(ctx, c, rider, mount, &ev); err != nil {
			return nil, err
		}
	}
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: master, Used: true}, ""); err != nil {
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &rider.CharacterID
	run.made = ev
	run.result = mountSaveResultOf(ev)
	if !master {
		hideMountResult(run.result, c.rules.ShowSaveDC)
	}
	return ev, nil
}

// mountOfWindow is the mount a window asks the save about.
func (s *Service) mountOfWindow(ctx context.Context, c *combatTx, t windowTrigger) (playdb.Combatant, error) {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return playdb.Combatant{}, err
	}
	mount, ok := combatantByID(cs, t.Target)
	if !ok { // the window is closed with the mount before an answer gets here
		return mount, errNotYourTurnToAnswer()
	}
	return mount, nil
}

// mountSaveOutcome rolls the save, or fails it with no d20 when the rider auto-fails Dexterity saves. It answers whether the rider holds,
// unless the gate kept the d20 for a question (the offers are in run).
func (s *Service) mountSaveOutcome(ctx context.Context, c *combatTx, call mountSaveCall, rider playdb.Combatant, ev *actionEvent, run *mountSaveRun) (bool, error) {
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return false, err
	}
	facts, err := s.mountSaveFacts(ctx, c.tx, c.q, c.session.CampaignID, c.enc, rider, states)
	if err != nil {
		return false, err
	}
	if facts.autoFail {
		ev.Reaction.AutoFail = true
		return false, nil
	}
	if call.m.Role != authz.RoleMaster {
		if err := s.mustRollThisWay(ctx, c.tx, call.m, call.in); err != nil {
			return false, err
		}
	}
	save, err := s.saveOf(ctx, c.tx, c.session.CampaignID, rider, "dex")
	if err != nil {
		return false, err
	}
	save = withRollBonus(save, states, rider.ID, rules.RollAppliesSave)
	spec := saveSpec(call.m, heldMountSave, rider, call.key, call.msg, call.in, save.Bonus, facts.mode, facts.traits)
	extras := len(effectDiceFor(states, rider.ID, rules.RollAppliesSave)) > 0
	spec.Save = &saveRule{DC: mountSaveDC, Blind: !c.rules.ShowSaveDC || extras}
	gate, err := s.heldGate(ctx, s.inspirationGate(c), spec)
	if err != nil {
		return false, err
	}
	if gate.Held != nil {
		run.offer, run.lucky, run.held = s.outsideOffers(ctx, c.tx, gate.Held)
		return false, nil
	}
	extra, delta, err := s.rollEffectDice(call.in, effectDiceFor(states, rider.ID, rules.RollAppliesSave), call.msg.GetExtraDieFaces())
	if err != nil {
		return false, err
	}
	if err := s.spendOnceEffects(ctx, c, extra); err != nil {
		return false, err
	}
	if ev.ManeuverSpent, err = s.spendManeuverDice(ctx, c, rider.CharacterID, extra); err != nil {
		return false, err
	}
	return recordMountRoll(ev, gate, save.Bonus, delta), nil
}

// recordMountRoll writes what the save showed in the event and answers whether the rider holds: the d20 and the dice the effects added
// reach the DC.
func recordMountRoll(ev *actionEvent, gate gateResult, bonus, delta int) bool {
	roll := gate.Roll
	bonusFace := 0
	if gate.Bonus != nil && gate.Indomitable == nil {
		bonusFace = int(gate.Bonus.Face)
	}
	total := roll.Total + delta
	held := total >= mountSaveDC
	re := ev.Reaction
	re.SaveD20, re.SaveBonus, re.Saved = clamp32(roll.Face(), 1, 20), conv.Int32(bonus+delta+bonusFace), held
	ev.D20, ev.Modifier, ev.Total, ev.Physical = re.SaveD20, re.SaveBonus, conv.Int32(total), roll.Physical
	ev.D20B, ev.Counted = clamp32(roll.otherFace(), 0, 20), clamp32(roll.Index, 0, 1)
	ev.countedRoll = countedOf(roll)
	ev.Lucky, ev.Indomitable = gate.Lucky, gate.Indomitable
	ev.D20Reroll = rerollUse(gate.Lucky)
	if gate.Bonus != nil {
		ev.Res = &resourceEvent{Kind: resBardicUse, Sides: gate.Bonus.Sides, Face: gate.Bonus.Face}
	}
	return held
}

// throwRider is the fall of a rider that failed its save: Prone (unless it is immune), on the square landingOf picks, off the mount,
// which gets its own initiative back. The line of the fall is written before the save's own event.
func (s *Service) throwRider(ctx context.Context, c *combatTx, rider, mount playdb.Combatant, ev *actionEvent) error {
	return s.dropRiderOf(ctx, c, rider, mount, ev, true)
}

// dropRiderOf takes the rider off the mount, on the square landingOf picks. A rider that falls is prone (unless it is immune) and has
// the line of a thrown rider; one that lands on its feet (prone false) has neither.
func (s *Service) dropRiderOf(ctx context.Context, c *combatTx, rider, mount playdb.Combatant, ev *actionEvent, prone bool) error {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	at, err := s.landAt(ctx, c, cs, rider, mount)
	if err != nil {
		return err
	}
	col, row, from, noRoom, cover := at.col, at.row, at.from, at.noRoom, at.cover
	if err := c.q.SetCombatantRide(ctx, playdb.SetCombatantRideParams{
		ID: rider.ID, GridCol: col, GridRow: row,
		MovementUsedFt: rider.MovementUsedFt, MovementUsedDft: rider.MovementUsedDft, MountMoveUsed: rider.MountMoveUsed, CoverMark: cover,
	}); err != nil {
		return err
	}
	after := rider
	after.MountedOnID, after.GridCol, after.GridRow, after.CoverMark = nil, col, row, cover
	if immune, err := c.immuneToCondition(ctx, rider, condProne); err != nil {
		return err
	} else if prone && !immune && !slices.Contains(rider.Conditions, condProne) && len(rider.Conditions) < maxConditions {
		after.Conditions = withCondition(rider.Conditions, condProne)
		if err := setConditionsOf(ctx, c, rider, after.Conditions); err != nil {
			return err
		}
	}
	if col != nil {
		ev.Reaction.LandCol, ev.Reaction.LandRow = *col, *row
	}
	ev.Reaction.NoRoom = noRoom
	ev.From = moveStateOf(rider)
	if err := settleMounts(ctx, c); err != nil {
		return err
	}
	c.landed = append(c.landed, riderLanded{rider: after, from: from})
	if prone {
		thrown := *ev
		if err := insertEvent(ctx, c, eventRiderThrown, &c.actorUserID, nil, thrown); err != nil {
			return err
		}
	}
	return nil
}

// riderLanding is where a thrown or landing rider ends up: no square at all when it or its mount has none.
type riderLanding struct {
	col, row *int32
	from     *grid.Square
	noRoom   bool
	cover    string
}

// landAt works out the landing of the rider next to its mount.
func (s *Service) landAt(ctx context.Context, c *combatTx, cs []playdb.Combatant, rider, mount playdb.Combatant) (riderLanding, error) {
	out := riderLanding{cover: rider.CoverMark}
	if !placed(rider) || !placed(mount) {
		return out, nil
	}
	to, noRoom, err := s.landingOf(ctx, c, cs, rider, mount)
	if err != nil {
		return out, err
	}
	lc, lr := clamp32(to.Col, 0, math.MaxInt32), clamp32(to.Row, 0, math.MaxInt32)
	was := squareOfCombatant(rider)
	out.col, out.row, out.from, out.noRoom = &lc, &lr, &was, noRoom
	if to != was {
		out.cover = "none"
	}
	return out, nil
}

// landingOf is the square a thrown rider lands on: the nearest free neighbor of the mount (SRD: a space within 5 feet), in the order
// of distance, then row, then column. A creature holds a square whether the players see it or not (a fall never stops at one, and
// nobody is told why). With no free neighbor it is the nearest free square, and noRoom says so.
func (s *Service) landingOf(ctx context.Context, c *combatTx, cs []playdb.Combatant, rider, mount playdb.Combatant) (to grid.Square, noRoom bool, err error) {
	if !placed(mount) {
		return grid.Square{}, true, nil // no square to land next to
	}
	at := squareOfCombatant(mount)
	terrain, err := s.terrainOf(ctx, c.tx, c.session.CampaignID, c.enc)
	if err != nil {
		return at, true, err
	}
	mover, err := s.moverWith(ctx, c.tx, c.session.CampaignID, rider)
	if err != nil {
		return at, true, err
	}
	occ := occupantsFor(cs, rider, combatViewer{master: true})
	if near := landingSquares(terrain, at, occ, mover); len(near) > 0 {
		return slices.MinFunc(near, nearerTo(at)), false, nil
	}
	if far := freeSquaresAround(terrain, occ, at); len(far) > 0 {
		return slices.MinFunc(far, nearerTo(at)), true, nil
	}
	return at, true, nil
}

// freeSquaresAround are the squares within mountSearchReach of at that a creature can stand on and none holds.
func freeSquaresAround(terrain grid.Terrain, occ grid.OccupantMap, at grid.Square) []grid.Square {
	var out []grid.Square
	for dc := -mountSearchReach; dc <= mountSearchReach; dc++ {
		for dr := -mountSearchReach; dr <= mountSearchReach; dr++ {
			sq := grid.Square{Col: at.Col + dc, Row: at.Row + dr}
			if _, taken := occ.At(sq); sq != at && !taken && terrain.Standable(sq) {
				out = append(out, sq)
			}
		}
	}
	return out
}

// nearerTo orders squares by their distance to at, then by row, then by column.
func nearerTo(at grid.Square) func(a, b grid.Square) int {
	dist := func(sq grid.Square) int { return (sq.Col-at.Col)*(sq.Col-at.Col) + (sq.Row-at.Row)*(sq.Row-at.Row) }
	return func(a, b grid.Square) int {
		switch {
		case dist(a) != dist(b):
			return dist(a) - dist(b)
		case a.Row != b.Row:
			return a.Row - b.Row
		}
		return a.Col - b.Col
	}
}

// mountSearchReach is how far, in squares, a rider with no free neighbor looks for the nearest free square.
const mountSearchReach = 10
