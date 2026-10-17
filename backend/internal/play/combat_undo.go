package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The master's "Desfazer" (MR-012, MR-014): one step back, a compensating
// event. Only the very last change of the session can be undone, and only
// when it is one of the actions below: any later event (a turn passing, a move,
// a correction of the vitals) puts what the action changed beyond a simple
// put-back, so there is nothing to undo then. The master's confirming a death
// is not one of them: the character is dead in the characters module. The events keep what was before
// (actionEvent: Before, ActionBefore...), and the undo writes it back.

// undoableKinds are the events UndoLastAction can take back.
var undoableKinds = []string{
	eventAttackRolled, eventDamageRolled, eventDamageApplied, eventDamageDiscarded, eventActionTaken, eventHitPointsAdjusted,
	eventSpellCast, eventReactionUsed, eventReactionDeclined, eventDeathSaveRolled, eventConditionsSet,
	eventCombatantMoved, eventTrapTriggered, eventWildShapeStarted, eventWildShapeEnded, eventFamiliarSight,
	eventHideResolved, // only the master's reveal of a hider (undoableHideResolved)
}

// recentEvents is how many of the session's latest events the search for the
// last action reads: each undo leaves two rows behind, so this allows dozens.
const recentEvents = 100

// lastAction finds the event an undo would take back among the session's
// latest events, newest first: the first one an undo did not take back
// already, if it belongs to this combat and is an action. Anything else
// being the last change leaves nothing to undo.
func lastAction(recent []playdb.ListRecentSessionEventsRow, encounterID string) (playdb.ListRecentSessionEventsRow, bool) {
	undone := map[string]bool{}
	for _, e := range recent {
		if e.Kind == eventActionUndone {
			markUndone(undone, e)
			continue
		}
		if undone[e.ID] {
			continue
		}
		switch chainVerdictOf(chainEvent{recent: recent, row: e, encounterID: encounterID}) {
		case chainSkip:
			continue
		case chainTake:
			return e, true
		}
		return playdb.ListRecentSessionEventsRow{}, false
	}
	return playdb.ListRecentSessionEventsRow{}, false
}

// markUndone notes the events an undo event took back.
func markUndone(undone map[string]bool, e playdb.ListRecentSessionEventsRow) {
	if ev, err := readEvent(e.Payload); err == nil {
		undone[ev.Undone] = true
		for _, id := range ev.UndoneAlso {
			undone[id] = true
		}
	}
}

// chainVerdict is what one rule of the walk says of the event it looks at.
type chainVerdict int

const (
	// chainGoOn: the rule has nothing to say, the next one looks.
	chainGoOn chainVerdict = iota
	// chainSkip: the event never closes the undo chain, the walk goes on past it.
	chainSkip
	// chainClose: the event is the last change and nothing an undo can take back.
	chainClose
	// chainTake: the event is the action to undo.
	chainTake
)

// chainEvent is the event the walk looks at, with what the rules read of it.
type chainEvent struct {
	recent      []playdb.ListRecentSessionEventsRow
	row         playdb.ListRecentSessionEventsRow
	encounterID string
}

// payload reads the event; ok is false for a payload that is not one.
func (c chainEvent) payload() (ev actionEvent, ok bool) {
	ev, err := readEvent(c.row.Payload)
	return ev, err == nil
}

// chainRules are the rules of the walk, in the order they look at an event: the first
// that says something decides. The last one always does.
var chainRules = []func(c chainEvent) chainVerdict{
	chainCreatures,
	chainConcentrationEnd,
	chainOffer,
	chainMoveCompanions,
	chainPuzzle,
	chainTrapOutside,
	chainShapeAndSight,
	chainHeld,
	chainHeldFall,
	chainRevive,
	chainHideDecision,
	chainAction,
}

// chainVerdictOf runs the rules on an event.
func chainVerdictOf(c chainEvent) chainVerdict {
	for _, rule := range chainRules {
		if v := rule(c); v != chainGoOn {
			return v
		}
	}
	return chainClose
}

// chainCreatures: the creatures a cast or an ended concentration made or dismissed are
// written before the change's own event, which is what an undo acts on: they never
// close the undo chain.
func chainCreatures(c chainEvent) chainVerdict {
	if c.row.Kind == eventCreatureSummoned || c.row.Kind == eventCreatureDismissed {
		return chainSkip
	}
	return chainGoOn
}

// chainConcentrationEnd: a concentration that ended at 0 hit points is written before
// the damage's own event, and goes with it when the damage is undone.
func chainConcentrationEnd(c chainEvent) chainVerdict {
	if ev, ok := c.payload(); ok && ev.Reaction != nil && ev.Reaction.Kind == concentrationEnd {
		return chainSkip
	}
	return chainGoOn
}

// chainOffer: the offers a move made are written before the move's own event, the same
// way: the undo acts on the move. One the master made by hand (a combat without a grid)
// is no part of a move: it closes the chain, and the master withdraws it instead.
func chainOffer(c chainEvent) chainVerdict {
	if c.row.Kind != eventOpportunityOffered {
		return chainGoOn
	}
	if ev, ok := c.payload(); ok && ev.ByHand {
		return chainClose
	}
	return chainSkip
}

// chainMoveCompanions: a door a move opened is written before the move's own event too.
// An undo of the move leaves the door open (a door opened stays opened), and the line
// stays: it never closes the chain either.
func chainMoveCompanions(c chainEvent) chainVerdict {
	if c.row.Kind == eventDoorOpened || c.row.Kind == eventHideEnded {
		return chainSkip
	}
	return chainGoOn
}

// chainPuzzle: a puzzle shown, solved, reset or closed is no action of the combat
// (MR-038), and the clue a solve gave is written just before the solve's own event:
// neither closes the chain.
func chainPuzzle(c chainEvent) chainVerdict {
	if strings.HasPrefix(c.row.Kind, "puzzle_") || clueOfASolve(c.recent, c.row) {
		return chainSkip
	}
	return chainGoOn
}

// chainTrapOutside: what the traps did that is not this combat's never closes the chain:
// a trap noticed after a move (the knowledge stays after an undo), a firing, a search or
// a disarm outside a combat or in another one, and the master settling a trap damage that
// has no combat (MR-035).
func chainTrapOutside(c chainEvent) chainVerdict {
	if trapOutsideTheChain(c.row, c.encounterID) {
		return chainSkip
	}
	return chainGoOn
}

// chainShapeAndSight: a form that ended because the beast fell, and a sight that ended at
// a turn start or by the player's hand, are no action to undo: the first is part of the
// damage that did it (written before it), the others change nothing an undo can put back.
func chainShapeAndSight(c chainEvent) chainVerdict {
	if c.row.Kind != eventWildShapeEnded && c.row.Kind != eventFamiliarSight {
		return chainGoOn
	}
	ev, ok := c.payload()
	if ok && c.row.Kind == eventWildShapeEnded && endsBySelf(ev.Reason) {
		return chainSkip
	}
	if !ok || (c.row.Kind == eventFamiliarSight && ev.Sight != "start") {
		return chainClose
	}
	return chainGoOn
}

// chainHeld: the event that stands for a held action is no action to undo: nothing of it
// happened yet (combat_reaction_hold.go).
func chainHeld(c chainEvent) chainVerdict {
	if ev, ok := c.payload(); ok && ev.Reaction != nil && ev.Reaction.Hold {
		return chainClose
	}
	return chainGoOn
}

// chainHeldFall: a fall whose damage waited for a Feather Fall landed by itself: the hit
// points it took were not written with the firing, so the firing is no longer undoable.
func chainHeldFall(c chainEvent) chainVerdict {
	if c.row.Kind != eventTrapTriggered {
		return chainGoOn
	}
	if ev, ok := c.payload(); ok && ev.Trap != nil && ev.Trap.Held {
		return chainClose
	}
	return chainGoOn
}

// chainRevive: a Revivify cast is not undone: the creature lives again in the characters
// module, as after the master's Reviver, and the diamonds are spent. It closes the chain.
func chainRevive(c chainEvent) chainVerdict {
	if c.row.Kind != eventSpellCast {
		return chainGoOn
	}
	if ev, ok := c.payload(); ok && ev.FxKind == rules.SpellKindRevive {
		return chainClose
	}
	return chainGoOn
}

// chainHideDecision: the master's decision on a hide attempt is not undone, his reveal of
// a hider is.
func chainHideDecision(c chainEvent) chainVerdict {
	if c.row.Kind == eventHideResolved && !undoableHideResolved(c.row.Payload) {
		return chainClose
	}
	return chainGoOn
}

// chainAction: the action of this combat an undo can take back, or the end of the chain.
func chainAction(c chainEvent) chainVerdict {
	e := c.row
	if !slices.Contains(undoableKinds, e.Kind) || e.EncounterID == nil || *e.EncounterID != c.encounterID {
		return chainClose
	}
	// A move written before the undo knew moves says where it came from
	// nowhere: there is nothing to put back.
	if e.Kind == eventCombatantMoved {
		if ev, ok := c.payload(); !ok || ev.From == nil {
			return chainClose
		}
	}
	return chainTake
}

// firingBefore lists the older events of the firing that last (a trap_triggered
// event) is a part of, newest first, ending with the event that holds the firing
// itself: a firing whose creatures did not fit one event is written as several,
// back to back (trapFireEvent.Part). Nothing when last is not a part.
func firingBefore(recent []playdb.ListRecentSessionEventsRow, last playdb.ListRecentSessionEventsRow) []playdb.ListRecentSessionEventsRow {
	at := slices.IndexFunc(recent, func(e playdb.ListRecentSessionEventsRow) bool { return e.ID == last.ID })
	if ev, err := readEvent(last.Payload); at < 0 || err != nil || ev.Trap == nil || !ev.Trap.Part {
		return nil
	}
	var out []playdb.ListRecentSessionEventsRow
	for _, e := range recent[at+1:] {
		ev, err := readEvent(e.Payload)
		if e.Kind != eventTrapTriggered || err != nil || ev.Trap == nil {
			break
		}
		out = append(out, e)
		if !ev.Trap.Part {
			break
		}
	}
	return out
}

// undoRequest is what UndoLastAction reads of its request, checked.
type undoRequest struct {
	m        authz.Membership
	key      string
	encID    string
	expected string // the event the master saw as the last action
}

// undoRequestOf checks the request in the order the handler always refused it: the
// master's role, the key, the combat, the event the master expects.
func undoRequestOf(ctx context.Context, msg *playv1.UndoLastActionRequest) (undoRequest, error) {
	m, err := authz.RequireCampaignRole(ctx, msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return undoRequest{}, err
	}
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return undoRequest{}, err
	}
	encID, err := parseCombatID(msg.GetEncounterId(), "encounter")
	if err != nil {
		return undoRequest{}, err
	}
	expected, err := parseKey(msg.GetExpectedEventId())
	if err != nil {
		return undoRequest{}, connect.NewError(connect.CodeInvalidArgument, errors.New("expected_event_id must be a UUID"))
	}
	return undoRequest{m: m, key: key, encID: encID, expected: expected}, nil
}

// undoRun is what one try of the undo leaves for what is told after the commit.
type undoRun struct {
	made       actionEvent
	vitals     []*playv1.CharacterVitals // the characters' vitals put back, if any
	undoneMove *actionEvent              // the move that was taken back, for the fog
	snapBack   *grid.Square              // where a mover stood before the undo of the 0 hit points rule put it back
	snapped    string                    // and who it is
	rearmed    *trapFireEvent            // the trap an undone firing armed again
	alsoUndone []string                  // the older parts of a firing that went with it
}

// UndoLastAction implements playv1connect.CombatServiceHandler.
func (s *Service) UndoLastAction(
	ctx context.Context,
	req *connect.Request[playv1.UndoLastActionRequest],
) (*connect.Response[playv1.UndoLastActionResponse], error) {
	r, err := undoRequestOf(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var run undoRun
	res, err := s.write(ctx, combatWrite{m: r.m, key: r.key, hash: idem.Hash(req.Msg), kind: eventActionUndone, encounterID: r.encID}, func(c *combatTx) (any, error) {
		return s.undoInTx(ctx, c, r.expected, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "undo the last action", err)
	}
	ev, err := resultEvent(res, run.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the undone action", err)
	}
	out, err := s.finish(ctx, r.m, res, func(ctx context.Context, d *encounterData) {
		s.publishUndoRun(ctx, r.m.CampaignID, d, ev, &run)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.UndoLastActionResponse{Encounter: out}), nil
}

// undoInTx is the undo inside its transaction: it finds the action the master expects
// to be the last, takes it back with what went with it, and returns the undo's event.
func (s *Service) undoInTx(ctx context.Context, c *combatTx, expected string, run *undoRun) (any, error) {
	*run = undoRun{}
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	recent, err := c.q.ListRecentSessionEvents(ctx, playdb.ListRecentSessionEventsParams{GameSessionID: c.session.ID, Limit: recentEvents})
	if err != nil {
		return nil, fmt.Errorf("read the latest events: %w", err)
	}
	last, ok := lastAction(recent, c.enc.ID)
	if !ok {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOTHING_TO_UNDO, "there is no action to undo")
	}
	if last.ID != expected {
		return nil, connect.NewError(connect.CodeAborted, errors.New("another action is the last now"))
	}
	ev, err := readEvent(last.Payload)
	if err != nil {
		return nil, err
	}
	if ev.ReturnedFrom != nil { // the undo of a damage that took the mover back to where it left the reach
		if err := run.noteSnapBack(ctx, c, ev); err != nil {
			return nil, err
		}
	}
	if run.vitals, err = s.takeBack(ctx, c, last.Kind, ev); err != nil {
		return nil, err
	}
	// What the action ended or revealed is as it was (takeBack): the lines it wrote
	// about it go too.
	run.alsoUndone = append(run.alsoUndone, ev.linesBefore()...)
	if err := s.takeBackCompanions(ctx, c, run, recent, last, ev); err != nil {
		return nil, err
	}
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	// The undo is told to the players who were told of the action it takes back,
	// and counts for them alone.
	run.made = actionEvent{Round: c.enc.Round, Secret: ev.Secret, Fogged: ev.Fogged, SeenBy: ev.SeenBy, Actor: ev.Actor, Target: ev.Target, Undone: last.ID, UndoneKind: last.Kind, UndoneAlso: run.alsoUndone}
	return run.made, nil
}

// noteSnapBack remembers where the target of a damage that took the mover back stands
// now, if it is placed: the fog follows it when the undo puts it back.
func (u *undoRun) noteSnapBack(ctx context.Context, c *combatTx, ev actionEvent) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == ev.Target }); i >= 0 && placed(cs[i]) {
		sq := squareOfCombatant(cs[i])
		u.snapBack, u.snapped = &sq, ev.Target
	}
	return nil
}

// takeBackCompanions takes back what goes with the action of its kind: the move for the
// fog, the concentration a damage ended, the other parts of a trap's firing.
func (s *Service) takeBackCompanions(ctx context.Context, c *combatTx, run *undoRun, recent []playdb.ListRecentSessionEventsRow, last playdb.ListRecentSessionEventsRow, ev actionEvent) error {
	if last.Kind == eventCombatantMoved {
		run.undoneMove = &ev
	}
	if last.Kind == eventDamageRolled || last.Kind == eventDamageApplied {
		if err := s.takeBackEndedConcentration(ctx, c, run, recent, last, ev); err != nil {
			return err
		}
	}
	if last.Kind == eventTrapTriggered {
		return s.takeBackFiring(ctx, c, run, recent, last, ev)
	}
	return nil
}

// takeBackEndedConcentration takes back the concentration the damage ended (at 0 hit
// points), written just before the damage's own event.
func (s *Service) takeBackEndedConcentration(ctx context.Context, c *combatTx, run *undoRun, recent []playdb.ListRecentSessionEventsRow, last playdb.ListRecentSessionEventsRow, ev actionEvent) error {
	at := slices.IndexFunc(recent, func(e playdb.ListRecentSessionEventsRow) bool { return e.ID == last.ID })
	for _, older := range recent[at+1:] {
		pe, ok := concentrationEndOf(older, ev.Pending)
		if !ok {
			break
		}
		if err := s.takeBackOlder(ctx, c, run, older, pe); err != nil {
			return err
		}
	}
	return nil
}

// concentrationEndOf reads the event of a concentration that ended at 0 hit points because
// of the damage with the pending id; ok is false for any other event.
func concentrationEndOf(row playdb.ListRecentSessionEventsRow, pending string) (ev actionEvent, ok bool) {
	ev, err := readEvent(row.Payload)
	if err != nil {
		return ev, false
	}
	return ev, ev.Reaction != nil && ev.Reaction.Kind == concentrationEnd && ev.Reaction.Group == pending
}

// takeBackFiring takes a firing written as several events back whole.
func (s *Service) takeBackFiring(ctx context.Context, c *combatTx, run *undoRun, recent []playdb.ListRecentSessionEventsRow, last playdb.ListRecentSessionEventsRow, ev actionEvent) error {
	run.rearmed = ev.Trap
	for _, older := range firingBefore(recent, last) {
		pe, err := readEvent(older.Payload)
		if err != nil {
			return err
		}
		if err := s.takeBackOlder(ctx, c, run, older, pe); err != nil {
			return err
		}
	}
	return nil
}

// takeBackOlder takes back one older event that goes with the undone action, and keeps
// the vitals it restored and the lines it wrote.
func (s *Service) takeBackOlder(ctx context.Context, c *combatTx, run *undoRun, older playdb.ListRecentSessionEventsRow, pe actionEvent) error {
	more, err := s.takeBack(ctx, c, older.Kind, pe)
	if err != nil {
		return err
	}
	run.vitals = append(run.vitals, more...)
	run.alsoUndone = append(append(run.alsoUndone, older.ID), pe.linesBefore()...)
	return nil
}

// publishUndoRun tells what the undo changed after the commit: the combat, the log, the
// fog around the combatants that went back, the vitals put back and the trap armed again.
func (s *Service) publishUndoRun(ctx context.Context, campaignID string, d *encounterData, ev actionEvent, run *undoRun) {
	s.publishUndone(ctx, campaignID, d, ev)
	s.publishLogChanged(ctx, campaignID, d.enc.ID, !ev.Secret)
	if run.undoneMove != nil { // the combatant is back where it was: the fog follows it
		s.followToSquare(ctx, campaignID, d, run.undoneMove.Actor, &grid.Square{Col: int(run.undoneMove.Col), Row: int(run.undoneMove.Row)})
	}
	if run.snapped != "" { // the mover is back where it was: the fog follows it
		s.followToSquare(ctx, campaignID, d, run.snapped, run.snapBack)
	}
	for _, vit := range run.vitals {
		s.publishVitals(campaignID, vit)
	}
	if run.rearmed != nil && s.traps != nil {
		s.traps.TrapChanged(ctx, campaignID, run.rearmed.MapID, run.rearmed.PointID) // players who saw it fire lose it again
	}
}

// followToSquare tells the fog that a combatant of the combat is back on a square.
func (s *Service) followToSquare(ctx context.Context, campaignID string, d *encounterData, combatantID string, sq *grid.Square) {
	if i := slices.IndexFunc(d.cs, func(c playdb.Combatant) bool { return c.ID == combatantID }); i >= 0 {
		s.positionChanged(ctx, campaignID, d.enc, d.cs[i], sq)
	}
}

// publishUndone tells the combat changed after an undo. On a map with the fog the
// players' hint goes only to those who were told of the action it takes back (the
// event keeps who saw it) and to the owners of the player characters and creatures
// in it, whose own sheet it changes; the others would learn that something happened
// out of their sight (RN-10).
func (s *Service) publishUndone(ctx context.Context, campaignID string, d *encounterData, ev actionEvent) {
	f, err := s.fogSightOf(ctx, campaignID, d.enc)
	if err != nil || f == nil || !ev.Fogged {
		s.publishEncounterChanged(ctx, campaignID, d.enc)
		return
	}
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: encounterChangedMessage(d.enc)})
	told := slices.Clone(ev.SeenBy)
	if !ev.Secret {
		for _, id := range ev.combatantIDs() {
			if i := slices.IndexFunc(d.cs, func(o playdb.Combatant) bool { return o.ID == id }); i >= 0 && d.cs[i].Kind != kindNPC && d.cs[i].UserID != nil {
				told = append(told, *d.cs[i].UserID)
			}
		}
	}
	slices.Sort(told)
	blind := encounterChangedMessage(playdb.Encounter{ID: d.enc.ID, Mode: d.enc.Mode})
	for _, u := range slices.Compact(told) {
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{UserID: u}, Message: blind})
	}
}

// takeBack puts back what the event changed, inside the undo's transaction.
// It returns the vitals of the characters it restored. A combatant or a pending
// damage that is gone (its character was deleted meanwhile) is skipped: there
// is nothing left to put back on it. Each kind of event has its own function
// (combat_undo_kinds.go); this one reads what they all need and dispatches.
func (s *Service) takeBack(ctx context.Context, c *combatTx, kind string, ev actionEvent) ([]*playv1.CharacterVitals, error) {
	if err := closeDamageWindows(ctx, c, kind, ev); err != nil {
		return nil, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	// The hiding an attack or a cast gave away, and the Help the attack used, come back.
	if err := restoreContestState(ctx, c, ev); err != nil {
		return nil, err
	}
	// What the action ended (an effect whose caster stopped concentrating, a spell it replaced)
	// comes back with its id and its clock, and what that left behind goes.
	if err := s.effectsBack(ctx, c, ev); err != nil {
		return nil, err
	}
	undo, ok := undoers[kind]
	if !ok {
		return nil, fmt.Errorf("event kind %q cannot be undone", kind)
	}
	u := &undoer{s: s, c: c, cs: cs}
	if err := undo(u, ctx, ev); err != nil {
		return nil, err
	}
	return u.vitals, nil
}

// effectsBack puts back the effects the action ended (ev.Restore), then takes away what
// their end left behind (ev.FollowUps: Haste's lethargy), which never happened once the
// effects are back. A follow-up the action made and ended again is in both lists, and goes.
func (s *Service) effectsBack(ctx context.Context, c *combatTx, ev actionEvent) error {
	if err := s.restoreEffects(ctx, c, ev.Restore); err != nil {
		return err
	}
	if len(ev.FollowUps) == 0 {
		return nil
	}
	targets := make([]string, 0, len(ev.FollowUps))
	for _, f := range ev.FollowUps {
		if err := c.q.DeleteLastingEffect(ctx, f.ID); err != nil {
			return fmt.Errorf("take away what an ended effect left: %w", err)
		}
		targets = append(targets, f.Target)
	}
	slices.Sort(targets)
	return s.refreshCombatants(ctx, c, slices.Compact(targets)...)
}

// closeDamageWindows closes the windows a damage opened (a concentration save, a
// Hellish Rebuke) when the damage is taken back: they were about it.
func closeDamageWindows(ctx context.Context, c *combatTx, kind string, ev actionEvent) error {
	if kind != eventDamageRolled && kind != eventDamageApplied {
		return nil
	}
	pendings := []string{ev.Pending}
	for _, h := range ev.Settled {
		pendings = append(pendings, h.Pending)
	}
	for _, id := range pendings {
		if id == "" {
			continue
		}
		if err := c.q.CloseOpenReactionWindowsOfPending(ctx, playdb.CloseOpenReactionWindowsOfPendingParams{EncounterID: c.enc.ID, Pending: id, AnsweredAt: c.now}); err != nil {
			return fmt.Errorf("close the windows of a damage taken back: %w", err)
		}
	}
	return nil
}

// nonNil is the list, or an empty one: the conditions column is never NULL.
func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// trapOutsideTheChain says an event is about traps and not this combat's: the undo
// goes past it. `trap_noticed` is always so (it is written after the move that caused
// it, with no combat); the others are when they belong to no combat or to another.
func trapOutsideTheChain(e playdb.ListRecentSessionEventsRow, encounterID string) bool {
	ours := e.EncounterID != nil && *e.EncounterID == encounterID
	switch e.Kind {
	case eventTrapNoticed:
		return true
	case eventTrapSearched, eventTrapTriggered, eventTrapDisarmed, eventTrapRevealed:
		return !ours
	case eventDamageApplied, eventDamageDiscarded:
		return e.EncounterID == nil // ApplyTrapDamage: a damage with no combat
	}
	return false
}

// clueOfASolve says whether the event is the `clue_revealed` that a solved puzzle's
// "Ao resolver" wrote: it comes right before the `puzzle_solved` of the same
// transaction (recent is newest first, so that one has the next number).
func clueOfASolve(recent []playdb.ListRecentSessionEventsRow, e playdb.ListRecentSessionEventsRow) bool {
	if e.Kind != eventClueRevealed {
		return false
	}
	return slices.ContainsFunc(recent, func(o playdb.ListRecentSessionEventsRow) bool {
		return o.Seq == e.Seq+1 && o.Kind == eventPuzzleSolved
	})
}

// nilIfEmpty is a text column's value: NULL for no text.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
