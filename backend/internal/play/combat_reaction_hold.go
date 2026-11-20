package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// A held action (PM-04): a cast, an attack or a damage roll that a reaction
// window can change waits for the windows of its group. The request itself is
// kept (reaction_holds.request) before anything of it happens, and when every
// window of the group is answered or closed the server replays it, in the
// transaction of the last answer, as the action's own call would have run, with
// what the answers changed: a Counterspell that fails the spell, a Cutting Words
// die taken off a roll, half the damage of an Uncanny Dodge. Nothing is spent and
// nothing is rolled before the replay, so a held action that is dropped (the combat
// ended, or what it needed is gone) leaves no trace.

// replayState is what a replay of a held action carries: what the answers did.
type replayState struct {
	hold playdb.ReactionHold
	// countered: a Counterspell failed the spell. The cast spends its slot and its
	// action, and does nothing else.
	countered bool
	// reduction is what Cutting Words took off the roll (an attack's total, a
	// damage), and deflected what Deflect Missiles took off the damage.
	reduction int32
	deflected int32
	// after is the total of a held check once the die came off (cutRoll).
	after int32
	// halve: Uncanny Dodge halved the damage.
	halve bool
	// protectors are the shield-bearers whose Protection gave the attack roll disadvantage.
	protectors []string
	// rolled is the damage roll made when the hold was written, so that the
	// reactor decided with the number it would take (Uncanny Dodge, Deflect
	// Missiles) and the replay lands that very roll.
	rolled *rolledDamage
	// parts and partRolls are the parts of a damage with extras as they were rolled when
	// the hold was written: the replay lands those dice, not new ones.
	parts     []partRecord
	partRolls []partRoll
	// saves are the saving throws a held cast rolled when the hold was written, and resisted the creatures
	// whose failed save the master turned into a success (combat_legendary_hold.go): the replay lands
	// those dice, not new ones.
	saves    []keptSave
	resisted map[string]bool
}

// answeredRef is an answer's event, kept to be completed once the replay it released
// says what the roll became.
type answeredRef struct {
	window string
	ev     *reactionEvent
}

// rolledDamage is a damage roll made before the replay.
type rolledDamage struct {
	Faces    []int32 `json:"faces,omitempty"`
	Total    int32   `json:"total"`
	Physical bool    `json:"physical,omitempty"`
	// Counted are the dice a table minimum raised (damage_floor.go); the faces stay as they came up.
	Counted []countedDie `json:"counted_dice,omitempty"`
}

// holdData is what a hold keeps besides the request.
type holdData struct {
	Rolled    *rolledDamage `json:"rolled,omitempty"`
	Parts     []partRecord  `json:"parts,omitempty"`
	PartRolls []partRoll    `json:"part_rolls,omitempty"`
	Saves     []keptSave    `json:"saves,omitempty"`
}

// The ambient write: a replay runs the action's own handler inside the
// transaction of the answer that released it. The handler calls s.write as in any other change;
// write sees the ambient and runs the change in the open transaction instead of
// opening one, writes the event, and returns errAmbientDone so that the handler
// stops before it reads and publishes (the answer's handler does that, after the
// commit).
type ambient struct{ c *combatTx }

type ambientKey struct{}

func withAmbient(ctx context.Context, c *combatTx) context.Context {
	return context.WithValue(ctx, ambientKey{}, &ambient{c: c})
}

func ambientOf(ctx context.Context) *ambient {
	a, _ := ctx.Value(ambientKey{}).(*ambient)
	return a
}

// ambientDoneError is the error a replayed change ends with once its event is written.
type ambientDoneError struct {
	event actionEvent
	// asking says the replayed attack was not resolved: its d20 was kept for the player's
	// Lucky or Bardic Inspiration answer (combat_lucky.go, combat_inspiration.go), which resolves
	// it, and no event was written.
	asking bool
}

func (*ambientDoneError) Error() string { return "the held action was replayed" }

// writeAmbient is write inside an open transaction (see ambient).
func (s *Service) writeAmbient(ctx context.Context, a *ambient, w combatWrite, do func(c *combatTx) (any, error)) (combatResult, error) {
	c := a.c
	kind, stamped, characterID := c.kind, c.stamped, c.characterID
	defer func() { c.kind, c.stamped, c.characterID = kind, stamped, characterID }()
	c.kind, c.characterID = w.kind, nil
	payload, err := do(c)
	if err != nil {
		return combatResult{}, err
	}
	if payload == nil { // a roll kept for a Lucky or Bardic Inspiration question: the answer resolves it
		return combatResult{}, connect.NewError(connect.CodeUnknown, &ambientDoneError{asking: true})
	}
	ev, ok := payload.(actionEvent)
	if !ok {
		return combatResult{}, fmt.Errorf("a held action wrote an event that is not an action: %T", payload)
	}
	// The event of the replayed action carries no key: the request's own key
	// belongs to the event that held it.
	if err := insertEvent(ctx, c, w.kind, &w.m.UserID, nil, ev); err != nil {
		return combatResult{}, err
	}
	return combatResult{}, connect.NewError(connect.CodeUnknown, &ambientDoneError{event: ev})
}

// holdAction keeps a request that windows hold, opens the windows and returns the
// event that stands for the held change: it carries the request's idempotency key
// (a retry answers with it) and never reaches the log.
func (s *Service) holdAction(ctx context.Context, c *combatTx, m authz.Membership, kind string, actor playdb.Combatant, msg proto.Message, data holdData, specs []windowSpec, ev actionEvent) (actionEvent, error) {
	body, err := proto.Marshal(msg)
	if err != nil {
		return ev, fmt.Errorf("keep the held request: %w", err)
	}
	group := newGroup()
	dataBody, err := json.Marshal(data)
	if err != nil {
		return ev, err
	}
	hold, err := c.q.InsertReactionHold(ctx, playdb.InsertReactionHoldParams{
		EncounterID: c.enc.ID, GroupID: group, Kind: kind, ActorID: actor.ID, ActorUserID: m.UserID, ActorIsMaster: m.Role == authz.RoleMaster,
		Request: body, Data: dataBody, CreatedAt: c.now,
	})
	if err != nil {
		return ev, fmt.Errorf("hold the action: %w", err)
	}
	if _, err := s.openWindows(ctx, c, group, &hold.ID, specs); err != nil {
		return ev, err
	}
	ev.Reaction = &reactionEvent{Hold: true, Group: group}
	return ev, nil
}

// releaseHold replays a held action once its windows are all closed. A change
// the replay refuses (a client error: the target is gone, the slot was spent) drops
// the hold, as the table would: the action does not happen and nothing was spent.
func (s *Service) releaseHold(ctx context.Context, c *combatTx, h playdb.ReactionHold) error {
	windows, err := c.q.ListReactionWindowsOfGroup(ctx, playdb.ListReactionWindowsOfGroupParams{EncounterID: c.enc.ID, GroupID: h.GroupID})
	if err != nil {
		return fmt.Errorf("list the windows of a held action: %w", err)
	}
	st := replayStateOf(h, windows)
	if err := c.q.SetReactionHoldState(ctx, playdb.SetReactionHoldStateParams{ID: h.ID, State: "released"}); err != nil {
		return fmt.Errorf("release a held action: %w", err)
	}
	m := authz.Membership{CampaignID: c.session.CampaignID, UserID: h.ActorUserID, Role: authz.RolePlayer}
	if h.ActorIsMaster {
		m.Role = authz.RoleMaster
	}
	c.replay = st
	defer func() { c.replay = nil }()
	// The replay runs in a savepoint: a refusal drops the action, and what it wrote before it met the refusal (the slot, the
	// action, the damage of the rays it had rolled) goes with it. What the answers of the windows did stays.
	sp, err := c.tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("guard the replay of a held action: %w", err)
	}
	before, marked := *c, c.hints.mark()
	err = s.replayHeld(withAmbient(ctx, c), m, h)
	if err != nil && clientError(err) {
		_ = sp.Rollback(ctx) // the transaction decides what a failed rollback means: the next statement fails with it
		*c = before
		c.hints.rewind(marked) // hints is a pointer: the restore above leaves what the replay told, and the streams would hear of writes rolled back
	} else if cerr := sp.Commit(ctx); cerr != nil {
		return fmt.Errorf("release the guard of the replay of a held action: %w", cerr)
	}
	return s.settleReplay(ctx, c, h, windows, st, err)
}

// replayStateOf is what the answered windows of a held action change in its replay: a
// Counterspell that took, the reduction of Cutting Words, the halving of Uncanny Dodge
// and what Deflect Missiles or a maneuver took off.
func replayStateOf(h playdb.ReactionHold, windows []playdb.ReactionWindow) *replayState {
	st := &replayState{hold: h}
	var data holdData
	_ = json.Unmarshal(h.Data, &data)
	st.rolled = data.Rolled
	st.parts, st.partRolls, st.saves = data.Parts, data.PartRolls, data.Saves
	for _, w := range windows {
		if w.Status != windowAnswered {
			continue
		}
		out := windowOutcomeOf(w)
		if !out.Used {
			continue
		}
		switch reaction.Kind(w.Kind) {
		case reaction.CounterspellKind:
			st.countered = st.countered || out.Countered
		case reaction.CuttingWords:
			st.reduction += out.Reduction
		case reaction.UncannyDodgeKind:
			st.halve = true
		case reaction.ProtectionKind:
			st.protectors = append(st.protectors, conv.OrZero(w.ReactorID))
		case reaction.DeflectKind, reaction.ManeuverReduceKind:
			st.deflected += out.Reduction
		case reaction.LegendaryResistance:
			st.resisted = map[string]bool{}
			for _, id := range out.Saved {
				st.resisted[id] = true
			}
		}
	}
	return st
}

// replayHeld runs the held request again through the handler of its kind, as the actor.
func (s *Service) replayHeld(ctx context.Context, m authz.Membership, h playdb.ReactionHold) error {
	switch h.Kind {
	case "cast":
		var req playv1.CastSpellRequest
		if err := proto.Unmarshal(h.Request, &req); err != nil {
			return fmt.Errorf("read a held cast: %w", err)
		}
		_, err := s.castSpell(ctx, m, connect.NewRequest(&req), false)
		return err
	case "attack":
		var req playv1.RollAttackRequest
		if err := proto.Unmarshal(h.Request, &req); err != nil {
			return fmt.Errorf("read a held attack: %w", err)
		}
		_, err := s.rollAttack(ctx, m, connect.NewRequest(&req))
		return err
	case "damage":
		var req playv1.RollDamageRequest
		if err := proto.Unmarshal(h.Request, &req); err != nil {
			return fmt.Errorf("read a held damage roll: %w", err)
		}
		_, err := s.rollDamage(ctx, m, connect.NewRequest(&req))
		return err
	case holdHitRider:
		return s.replayHeldRider(ctx, m, h)
	}
	return s.replayHeldCheck(ctx, m, h)
}

// settleReplay reads how the replay of a held action ended: the event that stands for
// it, a refusal that drops the hold, or a failure.
func (s *Service) settleReplay(ctx context.Context, c *combatTx, h playdb.ReactionHold, windows []playdb.ReactionWindow, st *replayState, err error) error {
	var done *ambientDoneError
	switch {
	case errors.As(err, &done) && done.asking:
		return nil
	case errors.As(err, &done):
		c.tellActorVitals(done.event)
		c.pushed = append(pushesOf(done.event.Hits), c.pushed...)
		if done.event.Rider != nil {
			c.pushed = append(c.pushed, done.event.Rider.Pushed)
		}
		s.completeAnswers(c, windows, st, done.event)
		return nil
	case err != nil && clientError(err):
		// The action cannot happen any more: it is dropped, and the master reads that it was, since the
		// answers may have spent slots and reactions already.
		if err := c.q.SetReactionHoldState(ctx, playdb.SetReactionHoldStateParams{ID: h.ID, State: "dropped"}); err != nil {
			return err
		}
		ev := actionEvent{Round: c.enc.Round, Secret: true, Actor: h.ActorID, Reaction: &reactionEvent{Kind: reactionHeldDropped, Used: true, Group: h.GroupID, Roll: h.Kind, Why: err.Error()}}
		return insertEvent(ctx, c, eventReactionAnswered, &c.actorUserID, nil, ev)
	case err != nil:
		return err
	}
	return fmt.Errorf("the replay of a held %s ended without an event", h.Kind)
}

// clientError says an error is the caller's to fix (a refusal), not a failure of
// the server: only these drop a held action; anything else is retried with the
// transaction.
func clientError(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeInvalidArgument, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodePermissionDenied, connect.CodeOutOfRange, connect.CodeAlreadyExists:
		return true
	}
	return false
}

// tellActorVitals queues the vitals of the player characters a replayed action
// changed (its actor's slot, a target's hit points) to be told after the commit.
func (c *combatTx) tellActorVitals(ev actionEvent) {
	c.tellIDs = append(c.tellIDs, ev.Actor, ev.Target)
	for _, h := range ev.Hits {
		c.tellIDs = append(c.tellIDs, h.Target)
	}
	for _, h := range ev.Settled {
		c.tellIDs = append(c.tellIDs, h.Target)
	}
	slices.Sort(c.tellIDs)
	c.tellIDs = slices.Compact(c.tellIDs)
}

// completeAnswers tells the answers that released a held action what it became: an
// attack roll's outcome, total and armor class (the master's log reads "17 → 13 contra CA
// 15"), and a damage's final amount.
func (s *Service) completeAnswers(c *combatTx, windows []playdb.ReactionWindow, st *replayState, done actionEvent) {
	for _, ref := range answersOf(c, windows) {
		switch st.hold.Kind {
		case "attack":
			ref.ev.Outcome, ref.ev.AC = done.Outcome, done.TargetAC
			ref.ev.After = done.Total
			ref.ev.Before = done.Total + st.reduction
		case "damage":
			ref.ev.Before = st.rolledTotal()
			ref.ev.After = done.Amount
		case "cast":
			// A spell attack: the one roll the die came off (a Counterspell's answer keeps its own numbers).
			if len(done.Hits) == 1 && ref.ev.Kind == string(reaction.CuttingWords) {
				ref.ev.Outcome, ref.ev.AC, ref.ev.After = done.Hits[0].Outcome, done.Hits[0].TargetAC, done.Hits[0].Total
				ref.ev.Before = done.Hits[0].Total + st.reduction
			}
		default: // a check: the total it became (a hit rider's answer has nothing to learn from the rider)
			ref.ev.After = st.after
			ref.ev.Before = st.after + st.reduction
		}
	}
}

// rolledTotal is the damage the hold rolled.
func (st *replayState) rolledTotal() int32 {
	if st.rolled == nil {
		return 0
	}
	return st.rolled.Total
}
