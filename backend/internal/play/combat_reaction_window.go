package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The reaction window (PM-04): a question to a reactor that holds the action that
// triggered it until it is answered. Windows live in reaction_windows (see
// migration 00196), one per reactor and trigger; the windows of one trigger share
// a group and are answered in the order they were opened. This file keeps them:
// opening, closing by itself when they stop being valid, and releasing what they
// held. What each reaction does is in combat_reaction_kinds.go.

// The status of a window (reaction_windows_status_valid).
const (
	windowOpen     = "open"
	windowAnswered = "answered"
	windowClosed   = "closed"
)

// stepSecond is the step of the question an answer asks (the aggressor's saving throw, the
// monk's throw back); hellishDieSides is the die of Hellish Rebuke's fire.
const (
	stepSecond      = 2
	hellishDieSides = 10
)

// windowTrigger is what happened, as a window keeps it: IDs and numbers only (never
// a name), so a payload of it is safe to store and to log.
type windowTrigger struct {
	// Actor is who did it (the attacker, the caster, the roller, the aggressor) and
	// Target who it was done to. Key is the attack or the spell.
	Actor  string `json:"actor,omitempty"`
	Target string `json:"target,omitempty"`
	Key    string `json:"key,omitempty"`
	// An attack that hit: its total and the armor class it was compared with. Crit
	// says it was a critical hit.
	Total int32 `json:"total,omitempty"`
	AC    int32 `json:"ac,omitempty"`
	Crit  bool  `json:"crit,omitempty"`
	// Damage is what a damage waits to do (Uncanny Dodge, Deflect Missiles,
	// Hellish Rebuke's aggressor damage). Taken is what a concentration was
	// threatened with.
	Damage int32 `json:"damage,omitempty"`
	Taken  int32 `json:"taken,omitempty"`
	// Magic says a Magic Missile is what hits the reactor (Shield).
	Magic bool `json:"magic,omitempty"`
	// Ranged says the attack was a ranged weapon attack (Deflect Missiles).
	Ranged bool `json:"ranged,omitempty"`
	// Spell and Level are the spell a Counterspell is about (never sent to a
	// player), or the spell a concentration window is for.
	Spell string `json:"spell,omitempty"`
	Level int32  `json:"level,omitempty"`
	// Roll is what Cutting Words answers: "attack", "test" or "damage".
	Roll string `json:"roll,omitempty"`
	// Distance is how far the reactor is from the trigger, in feet (0: no map).
	Distance int32 `json:"distance,omitempty"`
	// Falling are the creatures of a fall, FallFt its height and Trap the trap point.
	Falling []string `json:"falling,omitempty"`
	FallFt  int32    `json:"fall_ft,omitempty"`
	Trap    string   `json:"trap,omitempty"`
	// Handed says a concentration's owner left the roll to the master.
	Handed bool `json:"handed,omitempty"`
	// Rolling says the reactor's player is using the reaction and rolling its die
	// (MarkReactionRolling): the master's clock waits longer before it passes the window.
	Rolling bool `json:"rolling,omitempty"`
	// Effect is the state (the effect that lasts) an EFFECT_SAVE window asks the saving throw
	// of, Phase says at the end or the start of the turn, and Damage that the window opened
	// because the target took damage (advantage on the roll).
	Effect   string `json:"effect,omitempty"`
	Phase    string `json:"phase,omitempty"`
	OnDamage bool   `json:"on_damage,omitempty"`
	// Hellish Rebuke's second step: the level it was cast at, whether through the
	// Infernal Legacy, and the damage dice.
	CastLevel int32 `json:"cast_level,omitempty"`
	Racial    bool  `json:"racial,omitempty"`
	Dice      int32 `json:"dice,omitempty"`
	// Deflect Missiles' second step: the monk caught the missile, and Reduced is what
	// the first step took off the damage.
	Caught  bool  `json:"caught,omitempty"`
	Reduced int32 `json:"reduced,omitempty"`
	// Contest is the contest a CONTEST window waits for.
	Contest string `json:"contest,omitempty"`
	// PendingKey is the pending damage the trigger is about (a hit's, or the damage
	// that was rolled and waits).
	Pending string `json:"pending,omitempty"`
}

// windowOutcome is what an answered window did: kept with the window, and, for the
// log, with the event.
type windowOutcome struct {
	ByMaster bool `json:"by_master,omitempty"`
	Used     bool `json:"used,omitempty"`
	// Slot is the slot spent, Racial the Infernal Legacy.
	Slot   *slotRef `json:"slot,omitempty"`
	Racial bool     `json:"racial,omitempty"`
	// Countered, Stopped: what the reaction did. Effective is false for a Cutting
	// Words that did nothing (the creature is immune).
	Countered bool `json:"countered,omitempty"`
	Stopped   bool `json:"stopped,omitempty"`
	Ineffect  bool `json:"ineffect,omitempty"`
	// Reduction is the die a Cutting Words or a Deflect Missiles took off, After
	// what was left, Before what there was.
	Reduction int32 `json:"reduction,omitempty"`
	Before    int32 `json:"before,omitempty"`
	After     int32 `json:"after,omitempty"`
	// Saved lists the creatures a Feather Fall saved.
	Saved []string `json:"saved,omitempty"`
}

func windowTriggerOf(w playdb.ReactionWindow) windowTrigger {
	var t windowTrigger
	_ = json.Unmarshal(w.Trigger, &t) // a value this package wrote
	return t
}

func windowOutcomeOf(w playdb.ReactionWindow) windowOutcome {
	var o windowOutcome
	_ = json.Unmarshal(w.Outcome, &o)
	return o
}

// windowSpec is a window about to open.
type windowSpec struct {
	kind    reaction.Kind
	reactor *playdb.Combatant // nil for the master's check
	trigger windowTrigger
	pending string // the pending damage it holds
}

// reactionNote is a change a window made, told after the commit: the reactor's
// player and the master get it, nobody else (RN-10).
type reactionNote struct {
	opened bool
	window playdb.ReactionWindow
	// byItself and reason say a window closed by itself.
	byItself bool
	reason   reaction.Reason
	// spentOn is the reaction the same reactor used that closed it (REACTION_SPENT).
	spentOn reaction.Kind
}

// openWindows opens the windows of one trigger, in the order given (the reactors'
// initiative order), as a group. holdID is the held action, if the group holds one.
func (s *Service) openWindows(ctx context.Context, c *combatTx, group string, holdID *string, specs []windowSpec) ([]playdb.ReactionWindow, error) {
	var out []playdb.ReactionWindow
	for _, sp := range specs {
		body, err := json.Marshal(sp.trigger)
		if err != nil {
			return nil, fmt.Errorf("encode the window's trigger: %w", err)
		}
		var reactor, pending *string
		if sp.reactor != nil {
			reactor = &sp.reactor.ID
		}
		if sp.pending != "" {
			pending = &sp.pending
		}
		w, err := c.q.InsertReactionWindow(ctx, playdb.InsertReactionWindowParams{
			EncounterID: c.enc.ID, GroupID: group, Kind: string(sp.kind), ReactorID: reactor, PendingDamageID: pending, HoldID: holdID,
			Step: 1, Trigger: body, CreatedAt: c.now,
		})
		if err != nil {
			return nil, fmt.Errorf("open a reaction window: %w", err)
		}
		c.reactionNotes = append(c.reactionNotes, reactionNote{opened: true, window: w})
		out = append(out, w)
	}
	return out, nil
}

// closeWindow answers a window, or closes it by itself with a reason.
func (s *Service) closeWindow(ctx context.Context, c *combatTx, w playdb.ReactionWindow, status string, reason reaction.Reason, out windowOutcome, spentOn reaction.Kind) (playdb.ReactionWindow, error) {
	body, err := json.Marshal(out)
	if err != nil {
		return w, fmt.Errorf("encode the window's outcome: %w", err)
	}
	var why *string
	if reason != reaction.ReasonNone {
		why = new(string(reason))
	}
	closed, err := c.q.CloseReactionWindow(ctx, playdb.CloseReactionWindowParams{ID: w.ID, Status: status, ClosedReason: why, Outcome: body, AnsweredAt: &c.now})
	if errors.Is(err, pgx.ErrNoRows) {
		return w, errNotYourTurnToAnswer()
	}
	if err != nil {
		return w, fmt.Errorf("close a reaction window: %w", err)
	}
	c.reactionNotes = append(c.reactionNotes, reactionNote{window: closed, byItself: status == windowClosed, reason: reason, spentOn: spentOn})
	return closed, nil
}

// errNotYourTurnToAnswer is the refusal of a window that was answered or closed
// already, or whose turn to be answered has not come.
func errNotYourTurnToAnswer() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN_TO_ANSWER, "this reaction window cannot be answered now")
}

// openWindowsOf lists the windows that are open in the combat, in the order they
// are answered.
func openWindowsOf(ctx context.Context, c *combatTx) ([]playdb.ReactionWindow, error) {
	open, err := c.q.ListOpenReactionWindows(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the reaction windows: %w", err)
	}
	return open, nil
}

// reactionGate refuses what the turn cannot do while a window is open: moving,
// attacking, casting, acting and passing the turn wait for every window (PM-04).
// It holds the master too: he answers the window, for an NPC or for a player who
// does not. The replay of a held action goes through it (it is the action the
// windows were holding).
func (s *Service) reactionGate(ctx context.Context, c *combatTx, ignoring ...string) error {
	if c.replay != nil {
		return nil
	}
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return err
	}
	open = slices.DeleteFunc(open, func(w playdb.ReactionWindow) bool {
		return w.PendingDamageID != nil && slices.Contains(ignoring, *w.PendingDamageID)
	})
	if len(open) == 0 {
		return nil
	}
	for _, w := range open {
		if w.Kind == string(reaction.Concentration) {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CONCENTRATION_SAVE_PENDING, "a concentration save waits for its answer")
		}
	}
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING, "a reaction waits for its answer")
}

// factsOf is what decides whether an open window still stands.
func (s *Service) factsOf(ctx context.Context, c *combatTx, w playdb.ReactionWindow, byID map[string]playdb.Combatant) (reaction.Facts, error) {
	var f reaction.Facts
	if w.ReactorID != nil {
		r, ok := byID[*w.ReactorID]
		if !ok {
			f.Incapacitated = true // gone from the combat
		} else {
			f.ReactionUsed = r.ReactionUsed && w.Step == 1 // a second step comes after the reaction was spent
			f.Incapacitated = r.Defeated || r.EffectNoReaction || slices.ContainsFunc(r.Conditions, func(k string) bool { return slices.Contains(incapacitating, k) })
			if !f.Incapacitated && r.Kind == kindPlayer {
				down, err := s.isDown(ctx, c.tx, c.session.CampaignID, r)
				if err != nil {
					return f, err
				}
				f.Incapacitated = down
			}
		}
	}
	gone, err := s.triggerGone(ctx, c, w, byID)
	f.TriggerGone = gone
	return f, err
}

// triggerGone says whether what opened a window is not there any more.
func (s *Service) triggerGone(ctx context.Context, c *combatTx, w playdb.ReactionWindow, byID map[string]playdb.Combatant) (bool, error) {
	t := windowTriggerOf(w)
	if gone, decided, err := s.kindTriggerGone(ctx, c, w, t, byID); decided {
		return gone, err
	}
	if gone, err := s.holdGone(ctx, c, w); gone || err != nil {
		return gone, err
	}
	return actorGone(w, t, byID), nil
}

// kindTriggerGone is the check each kind of window has of its own. decided is false for the kinds
// (and the cases of a kind) that have nothing of their own to say, which go on to the checks every
// window shares.
func (s *Service) kindTriggerGone(ctx context.Context, c *combatTx, w playdb.ReactionWindow, t windowTrigger, byID map[string]playdb.Combatant) (gone, decided bool, err error) {
	switch reaction.Kind(w.Kind) {
	case reaction.Concentration:
		return concentrationGone(w, t, byID), true, nil
	case reaction.FeatherFall:
		gone, err = fallGone(ctx, c, t)
		return gone, true, err
	case reaction.EffectSave:
		// A defeated creature or NPC owes no saving throw (RN-22): the window closes with it.
		r, ok := byID[deref(w.ReactorID)]
		if ok && r.Defeated && r.Kind != kindPlayer {
			return true, true, nil
		}
	case reaction.Contest:
		waits, err := contestStillWaits(ctx, c, w)
		return !waits, true, err
	case reaction.Shield, reaction.MasterCheck:
		if w.PendingDamageID != nil {
			gone, err = pendingGone(ctx, c, *w.PendingDamageID)
			return gone, true, err
		}
	}
	return false, false, nil
}

// concentrationGone says the reactor no longer concentrates on the spell the window asked a save for.
func concentrationGone(w playdb.ReactionWindow, t windowTrigger, byID map[string]playdb.Combatant) bool {
	r, ok := byID[deref(w.ReactorID)]
	return !ok || r.ConcentrationSpell == nil || *r.ConcentrationSpell != t.Spell
}

// fallGone says none of the creatures of the fall has its damage waiting any more.
func fallGone(ctx context.Context, c *combatTx, t windowTrigger) (bool, error) {
	pend, err := fallPendingsOf(ctx, c, t.Trap)
	if err != nil {
		return false, err
	}
	return !slices.ContainsFunc(pend, func(p playdb.PendingDamage) bool { return slices.Contains(t.Falling, p.TargetID) }), nil
}

// pendingGone says the pending damage a window is about was settled or is not there.
func pendingGone(ctx context.Context, c *combatTx, id string) (bool, error) {
	p, err := c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: c.enc.ID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("find the pending damage of a window: %w", err)
	}
	return p.Status == pendingDiscarded || p.Status == pendingApplied || p.Status == pendingRolled, nil
}

// holdGone says the held action a window holds is not held any more.
func (s *Service) holdGone(ctx context.Context, c *combatTx, w playdb.ReactionWindow) (bool, error) {
	if w.HoldID == nil {
		return false, nil
	}
	h, err := c.q.GetReactionHold(ctx, playdb.GetReactionHoldParams{EncounterID: c.enc.ID, ID: *w.HoldID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && h.State != "held") {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("find the hold of a window: %w", err)
	}
	return false, nil
}

// actorGone says the one who did it left the fight: there is nothing to react to.
func actorGone(w playdb.ReactionWindow, t windowTrigger, byID map[string]playdb.Combatant) bool {
	if t.Actor == "" || reaction.Kind(w.Kind) == reaction.FeatherFall {
		return false
	}
	a, ok := byID[t.Actor]
	return !ok || (a.Defeated && reaction.Kind(w.Kind) == reaction.CounterspellKind)
}

// settleReactions runs after every change of a combat: it closes the open windows
// that stopped being valid (the reactor spent its reaction or fell, the trigger is
// gone), then releases what a group held when none of its windows is open. It
// repeats while a release changes something (a hit that goes on may open the
// next group), a few times at most.
func (s *Service) settleReactions(ctx context.Context, c *combatTx) error {
	const maxRounds = 8
	if err := s.syncContests(ctx, c); err != nil {
		return err
	}
	for range maxRounds {
		changed, err := s.settleOnce(ctx, c)
		if err != nil {
			return err
		}
		if !changed {
			// A turn that waited for the saving throw of an effect passes when none is open (RN-22).
			return s.releaseHeldTurn(ctx, c)
		}
	}
	return nil
}

// settleState is what one round of settling looks at: the open windows, the held actions, the hits
// that wait for windows and the combatants by id.
type settleState struct {
	open    []playdb.ReactionWindow
	holds   []playdb.ReactionHold
	waiting []playdb.PendingDamage
	byID    map[string]playdb.Combatant
}

// settleOnce closes the open windows that stopped being valid and releases what no window holds
// any more; it tells whether it changed anything.
func (s *Service) settleOnce(ctx context.Context, c *combatTx) (bool, error) {
	if c.enc.ID == "" || c.enc.Status != statusActive {
		return false, nil
	}
	st, ok, err := s.loadSettleState(ctx, c)
	if err != nil || !ok {
		return false, err
	}
	changed, err := s.closeInvalidWindows(ctx, c, st)
	if err != nil {
		return false, err
	}
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return false, err
	}
	openGroup, openPending, openFall := openIndexOf(open)
	released, err := s.releaseWaiting(ctx, c, st.waiting, openPending, openFall, st.byID)
	if err != nil {
		return false, err
	}
	changed = changed || released
	// A held action whose windows are all closed is replayed.
	for _, h := range st.holds {
		if openGroup[h.GroupID] {
			continue
		}
		if err := s.releaseHold(ctx, c, h); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

// loadSettleState reads what settleOnce looks at; ok is false when there is nothing to settle (no
// window, no held action and no hit that waits), before the combatants are read.
func (s *Service) loadSettleState(ctx context.Context, c *combatTx) (settleState, bool, error) {
	var st settleState
	var err error
	if st.open, err = openWindowsOf(ctx, c); err != nil {
		return st, false, err
	}
	if st.holds, err = c.q.ListHeldReactionHolds(ctx, c.enc.ID); err != nil {
		return st, false, fmt.Errorf("list the held actions: %w", err)
	}
	if st.waiting, err = c.q.ListOpenPendingDamages(ctx, c.enc.ID); err != nil {
		return st, false, fmt.Errorf("list the pending damage: %w", err)
	}
	st.waiting = slices.DeleteFunc(st.waiting, func(p playdb.PendingDamage) bool { return p.Status != pendingAwaitingReaction })
	if len(st.open) == 0 && len(st.holds) == 0 && len(st.waiting) == 0 {
		return st, false, nil
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return st, false, fmt.Errorf("list the combatants: %w", err)
	}
	st.byID = combatantsByID(cs)
	return st, true, nil
}

// closeInvalidWindows closes the open windows that stopped being valid.
func (s *Service) closeInvalidWindows(ctx context.Context, c *combatTx, st settleState) (bool, error) {
	changed := false
	for _, w := range st.open {
		f, err := s.factsOf(ctx, c, w, st.byID)
		if err != nil {
			return false, err
		}
		if reason, closes := reaction.Closure(reaction.Kind(w.Kind), f); closes {
			if _, err := s.closeWindow(ctx, c, w, windowClosed, reason, windowOutcome{}, c.lastAnswered); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}

// openIndexOf indexes the open windows: the groups that still hold an action, the pending damages
// they are about and the creatures a Feather Fall window may still save (by creature and trap).
func openIndexOf(open []playdb.ReactionWindow) (openGroup, openPending, openFall map[string]bool) {
	openGroup = map[string]bool{}
	openPending = map[string]bool{}
	openFall = map[string]bool{}
	for _, w := range open {
		openGroup[w.GroupID] = true
		if reaction.Kind(w.Kind) == reaction.FeatherFall {
			for _, id := range windowTriggerOf(w).Falling {
				openFall[id+"/"+windowTriggerOf(w).Trap] = true
			}
		}
		if w.PendingDamageID != nil {
			openPending[*w.PendingDamageID] = true
		}
	}
	return openGroup, openPending, openFall
}

// releaseWaiting lets what waited for windows that are all closed go on: a hit to its damage roll,
// a fall damage to its target.
func (s *Service) releaseWaiting(ctx context.Context, c *combatTx, waiting []playdb.PendingDamage, openPending, openFall map[string]bool, byID map[string]playdb.Combatant) (bool, error) {
	changed := false
	for _, p := range waiting {
		if openPending[p.ID] {
			continue
		}
		released, err := s.releaseOne(ctx, c, p, openFall, byID)
		if err != nil {
			return false, err
		}
		changed = changed || released
	}
	return changed, nil
}

// releaseOne lets one waiting pending damage go on when nothing holds it any more.
func (s *Service) releaseOne(ctx context.Context, c *combatTx, p playdb.PendingDamage, openFall map[string]bool, byID map[string]playdb.Combatant) (bool, error) {
	if p.TrapPointID != nil { // a fall damage: it lands when no Feather Fall can take it away
		if openFall[p.TargetID+"/"+*p.TrapPointID] {
			return false, nil
		}
		return true, s.releaseFall(ctx, c, p, byID)
	}
	cur, err := c.q.GetPendingDamage(ctx, playdb.GetPendingDamageParams{EncounterID: c.enc.ID, ID: p.ID})
	if err != nil || cur.Status != pendingAwaitingReaction {
		return false, nil //nolint:nilerr // a hit that cannot be read, or that moved on already, is left alone, as the loop always did
	}
	if _, err := c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: pendingAwaitingRoll}); err != nil {
		return false, fmt.Errorf("let the hit go on: %w", err)
	}
	return true, nil
}

// errWindowGone is the not_found of a window that is not in the combat, the same
// for a window that never existed: a player never learns which are real (RN-10).
func errWindowDenied() error {
	return connect.NewError(connect.CodePermissionDenied, errors.New("you cannot answer this reaction"))
}

// mustBeFirstOfGroup refuses the answer of a window that is not the first one open
// of its trigger: the reactors answer in the order the windows were opened (the
// initiative order), and a window answered or closed already has no answer left.
func (s *Service) mustBeFirstOfGroup(ctx context.Context, c *combatTx, w playdb.ReactionWindow) error {
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return err
	}
	all := make([]reaction.Window, len(open))
	for i, o := range open {
		all[i] = reaction.Window{ID: o.ID, Group: o.GroupID, Seq: o.Seq, Open: true}
	}
	if !reaction.AnswerNow(all, w.ID) {
		return errNotYourTurnToAnswer()
	}
	return nil
}

// discardReactions drops every window and held action of a combat that ended.
func (s *Service) discardReactions(ctx context.Context, c *combatTx) error {
	if err := c.q.DeleteReactionWindowsOfEncounter(ctx, c.enc.ID); err != nil {
		return fmt.Errorf("discard the reaction windows: %w", err)
	}
	if err := c.q.DeleteReactionHoldsOfEncounter(ctx, c.enc.ID); err != nil {
		return fmt.Errorf("discard the held actions: %w", err)
	}
	return nil
}

// newGroup is the id of a new trigger, shared by the windows it opens.
func newGroup() string { return uuid.New().String() }
