package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Hide (SRD 5.1, "Hide", "Hiding", "Passive Perception", "Unseen Attackers and Targets").
//
// The Hide action is a Dexterity (Stealth) check. The master decides whether there is a
// place to hide, and what each creature notices: the hider's total against the creature's
// passive Perception (10 plus the Perception bonus, plus 5 for advantage, minus 5 for
// disadvantage); the SRD does not say what a tie does, and the app keeps the status quo:
// a tie keeps the hider noticed. The master may also say a creature sees the hider
// clearly (cover the app does not see), and the hider cannot hide from it.
//
// The result is one state per creature (combat_hiding): the hider is hidden while one
// creature has not noticed it. A hidden attacker has advantage on its attack roll against
// a creature that has not noticed it, and the attack gives its position away, hit or miss:
// the hiding ends for every creature (hide_ended). The player reads only "Você está
// escondida": never from whom, never who noticed, never anybody's Perception (RN-10,
// RN-20).

// observerView is what the app compares for one creature that could see a hider.
type observerView struct {
	passive int
	known   bool
	noticed bool
}

// observerOf is how a creature stands to a hider whose Stealth total is given: its passive
// Perception (the sheet's or the stat block's, with the circumstances that give its
// Perception advantage or disadvantage counted), and whether the total does not beat it.
// q and tx are the change's own (nil for a read).
func (s *Service) observerOf(ctx context.Context, tx pgx.Tx, campaignID string, enc playdb.Encounter, o playdb.Combatant, stealth int, cs []playdb.Combatant, q *playdb.Queries) (observerView, error) {
	n, err := s.numbersOf(ctx, tx, campaignID, o, skillPercept)
	if err != nil {
		return observerView{}, err
	}
	if q == nil {
		q = s.queries
	}
	notes, err := s.checkSources(ctx, tx, campaignID, q, enc, o, skillPercept, cs)
	if err != nil {
		return observerView{}, err
	}
	passive := passivePerceptionOf(n, notesMode(notes))
	return observerView{passive: passive, known: n.Known, noticed: combat.NoticesHider(stealth, passive)}, nil
}

// hidePlan is a Hide request once the checks that need no database are done.
type hidePlan struct {
	combatantID string
	actionKey   string
	in          checkInput
}

// Hide implements playv1connect.ContestServiceHandler.
func (s *Service) Hide(
	ctx context.Context,
	req *connect.Request[playv1.HideRequest],
) (*connect.Response[playv1.HideResponse], error) {
	m, key, encID, err := contestCaller(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	plan, err := parseHidePlan(req.Msg)
	if err != nil {
		return nil, err
	}

	var made actionEvent
	var vitals *playv1.CharacterVitals // the resource a feature spent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventHideAttempted, encounterID: encID}, func(c *combatTx) (any, error) {
		made, vitals = actionEvent{}, nil
		ev, spent, err := s.attemptHide(ctx, c, m, plan)
		if err != nil {
			return nil, err
		}
		made, vitals = ev, spent
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "hide", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the hide", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		// The master decides the result: only he is told it waits for him. The hider's player
		// reads their own pending attempt.
		s.publishEncounterChangedFor(ctx, m.CampaignID, d, ev.Actor)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, false)
		s.publishVitals(m.CampaignID, vitals)
	})
	if err != nil {
		return nil, err
	}
	attempt, err := s.hideAttemptFor(ctx, m, res.encounterID, ev.Contest.ContestID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.HideResponse{Encounter: out, Attempt: attempt}), nil
}

// parseHidePlan reads the combatant, the action (the standard Hide unless a feature gives it) and
// the roll of a Hide.
func parseHidePlan(msg *playv1.HideRequest) (hidePlan, error) {
	combatantID, err := parseCombatID(msg.GetCombatantId(), "combatant")
	if err != nil {
		return hidePlan{}, err
	}
	actionKey := msg.GetActionKey()
	if actionKey == "" {
		actionKey = actionHide
	}
	in, err := parseCheckInput(msg.GetRoll())
	if err != nil {
		return hidePlan{}, err
	}
	return hidePlan{combatantID: combatantID, actionKey: actionKey, in: in}, nil
}

// attemptHide is the change of a Hide: the cost of the action, the Stealth roll and the pending
// attempt that waits for the master. It gives what a feature spent, for the vitals to stream.
func (s *Service) attemptHide(ctx context.Context, c *combatTx, m authz.Membership, p hidePlan) (actionEvent, *playv1.CharacterVitals, error) {
	if err := mustBeRunning(c); err != nil {
		return actionEvent{}, nil, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return actionEvent{}, nil, fmt.Errorf("list the combatants: %w", err)
	}
	v := c.viewer(m, cs)
	who, err := s.hider(ctx, c, m, v, cs, p)
	if err != nil {
		return actionEvent{}, nil, err
	}
	vitals, err := s.spendHide(ctx, c, m, v, who, p.actionKey)
	if err != nil {
		return actionEvent{}, nil, err
	}
	roll, err := s.rollFor(ctx, c, who, skillStealth, p.in, cs)
	if err != nil {
		return actionEvent{}, vitals, err
	}
	roll.ByMaster = v.master && !initiatorIsNPC(who)
	stored, err := encodeRoll(roll)
	if err != nil {
		return actionEvent{}, vitals, err
	}
	attempt, err := c.q.InsertHideAttempt(ctx, playdb.InsertHideAttemptParams{
		EncounterID: c.enc.ID, HiderID: who.ID, Status: hidePending, Roll: stored, Round: c.enc.Round, CreatedAt: c.now,
	})
	if err != nil {
		return actionEvent{}, vitals, fmt.Errorf("keep the hide attempt: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return actionEvent{}, vitals, err
	}
	c.characterID = &who.CharacterID
	return actionEvent{
		Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, Key: p.actionKey,
		Contest: &contestEvent{ContestID: attempt.ID, Line: contestLineHideTried},
	}, vitals, nil
}

// hider finds the combatant that hides, who may act now and, for a player, roll this way.
func (s *Service) hider(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, cs []playdb.Combatant, p hidePlan) (playdb.Combatant, error) {
	who, err := findCombatant(cs, p.combatantID, v)
	if err != nil {
		return who, err
	}
	if err := v.mayAct(who); err != nil {
		return who, err
	}
	if err := s.mustActNow(ctx, c, who); err != nil {
		return who, err
	}
	if !v.master {
		return who, s.mustRollThisWay(ctx, c.tx, m, p.in.rollInput())
	}
	return who, nil
}

// spendHide spends what the Hide costs the combatant: the action of the standard Hide, or
// the economy and the resource of a feature that gives Hide (the rogue's Cunning Action).
// It refuses a combatant that has no such action, or whose economy is spent.
func (s *Service) spendHide(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, who playdb.Combatant, actionKey string) (*playv1.CharacterVitals, error) {
	opts, err := s.optionsOf(ctx, c.tx, m.CampaignID, who)
	if err != nil {
		return nil, err
	}
	feature := actionKey != actionHide
	list := opts.GetStandardActions()
	if feature {
		list = opts.GetFeatureActions()
	}
	i := slices.IndexFunc(list, func(a *rulesv1.ActionOption) bool { return a.GetAction().GetKey() == actionKey })
	if i < 0 {
		return nil, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_AVAILABLE, "the combatant has no such action")
	}
	option := list[i]
	var fa link.FeatureAction
	if feature {
		if fa, err = s.hideFeature(ctx, c, m, who, actionKey); err != nil {
			return nil, err
		}
	}
	if !option.GetEnabled() && !v.master {
		return nil, spentHideRefusal(option, feature, v.master)
	}
	if _, err := breakRun(ctx, c, who); err != nil {
		return nil, err
	}
	if err := spendHideEconomy(ctx, c, who, option.GetAction().GetEconomy()); err != nil {
		return nil, err
	}
	if feature && fa.Resource != "" && !fa.Pool && who.Kind == kindPlayer {
		return s.spendResource(ctx, c, who.CharacterID, fa.Resource, 1)
	}
	return nil, nil
}

// hideFeature finds the feature action of the sheet that gives the Hide, refusing a key that is
// not a Hide.
func (s *Service) hideFeature(ctx context.Context, c *combatTx, m authz.Membership, who playdb.Combatant, actionKey string) (link.FeatureAction, error) {
	sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, who)
	if err != nil {
		return link.FeatureAction{}, err
	}
	fi := slices.IndexFunc(sheet.FeatureActions, func(a link.FeatureAction) bool { return a.Key == actionKey })
	if fi < 0 || sheet.FeatureActions[fi].Standard != actionHide {
		return link.FeatureAction{}, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_AVAILABLE, "the action is not a Hide")
	}
	return sheet.FeatureActions[fi], nil
}

// spentHideRefusal is why a Hide whose economy is spent is refused: the reason of the feature,
// or the bonus action or the action that is used.
func spentHideRefusal(option *rulesv1.ActionOption, feature, master bool) error {
	if feature {
		if err := featureError(option.GetReason(), master); err != nil {
			return err
		}
	}
	if option.GetAction().GetEconomy() == rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED, "the bonus action of this turn is used")
	}
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
}

// spendHideEconomy marks the bonus action or the action of the Hide as used.
func spendHideEconomy(ctx context.Context, c *combatTx, who playdb.Combatant, economy rulesv1.ActionEconomy) error {
	after := who
	switch economy {
	case rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION:
		after.BonusActionUsed = true
	case rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION, rulesv1.ActionEconomy_ACTION_ECONOMY_FREE, rulesv1.ActionEconomy_ACTION_ECONOMY_MOVEMENT:
		return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_AVAILABLE, "Hide is an action or a bonus action")
	default:
		after.ActionUsed = true
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	return nil
}

// resolveHidePlan is a ResolveHide request once the checks that need no database are done.
type resolveHidePlan struct {
	attemptID string
	refusal   string   // the master's reason, trimmed
	clearly   []string // the creatures that see the hider clearly, once each
	refuse    bool
}

// parseResolveHidePlan reads the attempt, the reason and the creatures of the master's decision.
func parseResolveHidePlan(msg *playv1.ResolveHideRequest) (resolveHidePlan, error) {
	var p resolveHidePlan
	var err error
	if p.attemptID, err = parseCombatID(msg.GetAttemptId(), "hide attempt"); err != nil {
		return p, err
	}
	if p.refusal, err = trimReason(msg.GetRefusal()); err != nil {
		return p, err
	}
	for _, id := range msg.GetSeesClearlyIds() {
		cid, err := parseCombatID(id, "combatant")
		if err != nil {
			return p, err
		}
		if !slices.Contains(p.clearly, cid) {
			p.clearly = append(p.clearly, cid)
		}
	}
	p.refuse = msg.GetRefuse()
	return p, nil
}

// ResolveHide implements playv1connect.ContestServiceHandler.
func (s *Service) ResolveHide(
	ctx context.Context,
	req *connect.Request[playv1.ResolveHideRequest],
) (*connect.Response[playv1.ResolveHideResponse], error) {
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
	plan, err := parseResolveHidePlan(req.Msg)
	if err != nil {
		return nil, err
	}

	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventHideResolved, encounterID: encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		ev, err := s.decideHide(ctx, c, m, plan)
		if err != nil {
			return nil, err
		}
		made = ev
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "decide a hide", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the hide", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChangedFor(ctx, m.CampaignID, d, ev.Actor)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, false)
	})
	if err != nil {
		return nil, err
	}
	attempt, err := s.hideAttemptFor(ctx, m, res.encounterID, plan.attemptID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ResolveHideResponse{Encounter: out, Attempt: attempt}), nil
}

// decideHide is the change of ResolveHide: the master refuses the attempt, or applies it and each
// creature that could see the hider keeps what it noticed.
func (s *Service) decideHide(ctx context.Context, c *combatTx, m authz.Membership, p resolveHidePlan) (actionEvent, error) {
	if err := notEnded(c.enc); err != nil {
		return actionEvent{}, err
	}
	attempt, err := pendingHideAttempt(ctx, c, p.attemptID)
	if err != nil {
		return actionEvent{}, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return actionEvent{}, fmt.Errorf("list the combatants: %w", err)
	}
	hider, ok := combatantByID(cs, attempt.HiderID)
	if !ok {
		return actionEvent{}, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_HIDE_NOT_PENDING, "the hider left the combat")
	}
	roll, err := decodeRoll(attempt.Roll)
	if err != nil || roll == nil {
		return actionEvent{}, fmt.Errorf("read the roll of hide attempt %s: %w", attempt.ID, err)
	}
	status, line := hideRefused, contestLineHideRefused
	var reason *string
	if p.refuse {
		reason = refusalReason(p.refusal)
	} else {
		status, line = hideApplied, contestLineHideApplied
		if err := s.noteObservers(ctx, c, m, hider, roll, p.clearly, cs); err != nil {
			return actionEvent{}, err
		}
	}
	if _, err := c.q.SetHideAttemptDecision(ctx, playdb.SetHideAttemptDecisionParams{
		EncounterID: c.enc.ID, ID: attempt.ID, Status: status, Refusal: reason, ResolvedAt: &c.now,
	}); err != nil {
		return actionEvent{}, fmt.Errorf("decide the hide attempt: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &hider.CharacterID
	return actionEvent{
		Round: c.enc.Round, Secret: true, Actor: hider.ID,
		Contest: &contestEvent{ContestID: attempt.ID, Line: line},
	}, nil
}

// refusalReason is the master's reason as the row keeps it: none when he gave none.
func refusalReason(refusal string) *string {
	if refusal == "" {
		return nil
	}
	return &refusal
}

// pendingHideAttempt reads the attempt the master decides, which has to be waiting for him.
func pendingHideAttempt(ctx context.Context, c *combatTx, attemptID string) (playdb.CombatHideAttempt, error) {
	attempt, err := c.q.GetHideAttempt(ctx, playdb.GetHideAttemptParams{EncounterID: c.enc.ID, ID: attemptID})
	if errors.Is(err, pgx.ErrNoRows) {
		return attempt, connect.NewError(connect.CodeNotFound, errors.New("hide attempt not found"))
	}
	if err != nil {
		return attempt, fmt.Errorf("find the hide attempt: %w", err)
	}
	if attempt.Status != hidePending {
		return attempt, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_HIDE_NOT_PENDING, "the hide attempt is answered already")
	}
	return attempt, nil
}

// noteObservers replaces the hiding of the hider with what each creature of the other side
// notices of the roll: its passive Perception against the total, or seeing the hider clearly.
func (s *Service) noteObservers(ctx context.Context, c *combatTx, m authz.Membership, hider playdb.Combatant, roll *contestRoll, clearly []string, cs []playdb.Combatant) error {
	if err := c.q.DeleteHidingOfHider(ctx, hider.ID); err != nil {
		return fmt.Errorf("forget the earlier hiding: %w", err)
	}
	for _, o := range cs {
		if o.ID == hider.ID || o.Defeated || !oppositeSide(hider, o) {
			continue
		}
		ob, err := s.observerOf(ctx, c.tx, m.CampaignID, c.enc, o, int(roll.Total), cs, c.q)
		if err != nil {
			return err
		}
		noticed := ob.noticed || slices.Contains(clearly, o.ID)
		if err := c.q.UpsertHiding(ctx, playdb.UpsertHidingParams{
			HiderID: hider.ID, ObserverID: o.ID, EncounterID: c.enc.ID, Noticed: noticed,
			Total: roll.Total, Passive: clamp32(ob.passive, math.MinInt32, math.MaxInt32), CreatedAt: c.now,
		}); err != nil {
			return fmt.Errorf("keep what a creature noticed: %w", err)
		}
	}
	return nil
}

// hideAttemptFor reads a hide attempt back as the caller may read it.
func (s *Service) hideAttemptFor(ctx context.Context, m authz.Membership, encounterID, attemptID string) (*playv1.HideAttemptView, error) {
	d, err := s.readContestData(ctx, m.CampaignID, encounterID)
	if err != nil {
		return nil, err
	}
	r, err := s.contestReaderFor(ctx, m, d)
	if err != nil {
		return nil, err
	}
	for _, a := range d.attempts {
		if a.ID == attemptID {
			view, err := r.hideAttemptView(ctx, a)
			if err != nil {
				return nil, s.dbError(ctx, "read the hide attempt", err)
			}
			return view, nil
		}
	}
	return nil, nil
}

// hideSnap is one creature's state before a hiding ended, for an undo.
type hideSnap struct {
	Observer string `json:"o"`
	Noticed  bool   `json:"n,omitempty"`
	Total    int32  `json:"t"`
	Passive  int32  `json:"p"`
}

// endHiding ends the hiding of a combatant for every creature: an attack gives its position
// away, hit or miss, and so does casting (SRD 5.1, "Unseen Attackers and Targets"). It
// writes hide_ended before the change's own event, and returns what the hiding was, for the
// undo of that change to give back. Nothing when the combatant was not hidden.
func (s *Service) endHiding(ctx context.Context, c *combatTx, who playdb.Combatant) ([]hideSnap, error) {
	rows, err := c.q.ListHiding(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the hiding: %w", err)
	}
	var snaps []hideSnap
	for _, h := range rows {
		if h.HiderID == who.ID {
			snaps = append(snaps, hideSnap{Observer: h.ObserverID, Noticed: h.Noticed, Total: h.Total, Passive: h.Passive})
		}
	}
	if len(snaps) == 0 {
		return nil, nil
	}
	if err := c.q.DeleteHidingOfHider(ctx, who.ID); err != nil {
		return nil, fmt.Errorf("end the hiding: %w", err)
	}
	if err := insertEvent(ctx, c, eventHideEnded, &c.actorUserID, nil, actionEvent{Round: c.enc.Round, Secret: true, Actor: who.ID}); err != nil {
		return nil, err
	}
	return snaps, nil
}

// restoreHiding gives a hiding back: the undo of the attack or the spell that ended it.
func restoreHiding(ctx context.Context, c *combatTx, hider string, snaps []hideSnap) error {
	for _, h := range snaps {
		if err := c.q.UpsertHiding(ctx, playdb.UpsertHidingParams{
			HiderID: hider, ObserverID: h.Observer, EncounterID: c.enc.ID, Noticed: h.Noticed, Total: h.Total, Passive: h.Passive, CreatedAt: c.now,
		}); err != nil {
			return fmt.Errorf("give the hiding back: %w", err)
		}
	}
	return nil
}

// contestFacts reads what the attack modes need of the contests' state: who is hidden from
// whom, and the Helps that hold.
func (s *Service) contestFacts(ctx context.Context, q *playdb.Queries, enc playdb.Encounter, cs []playdb.Combatant) ([]playdb.CombatHiding, []playdb.CombatHelp, error) {
	hiding, err := q.ListHiding(ctx, enc.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("list the hiding: %w", err)
	}
	helps, err := s.liveHelps(ctx, q, enc, cs)
	return hiding, helps, err
}

// afterAttack is what an attack roll does to the contests' state: the Help aimed at the
// target is used by the first attack roll against it, the effect that gave the attacker's
// next attack roll disadvantage is spent (Zombaria Viciosa), and the hiding ends for every
// creature, hit or miss. It keeps in made what it took, for the attack's undo.
func (s *Service) afterAttack(ctx context.Context, c *combatTx, attacker, target playdb.Combatant, cs []playdb.Combatant, made *actionEvent) error {
	used, err := s.consumeHelps(ctx, c, attacker, target, cs)
	if err != nil {
		return err
	}
	made.HelpUsed = append(made.HelpUsed, used...)
	if err := s.spendAttackEffects(ctx, c, attacker, made); err != nil {
		return err
	}
	made.HidBefore, err = s.endHiding(ctx, c, attacker)
	return err
}

// consumeHelps uses the Help aimed at the target for the attacker's attack roll.
func (s *Service) consumeHelps(ctx context.Context, c *combatTx, attacker, target playdb.Combatant, cs []playdb.Combatant) ([]string, error) {
	helps, err := s.liveHelps(ctx, c.q, c.enc, cs)
	if err != nil {
		return nil, err
	}
	var used []string
	for _, h := range helps {
		if h.Kind == helpAttack && helpsAlly(h, attacker) && deref(h.TargetID) == target.ID {
			if err := c.q.SetHelpConsumed(ctx, playdb.SetHelpConsumedParams{ID: h.ID, ConsumedAt: &c.now}); err != nil {
				return nil, fmt.Errorf("use the help: %w", err)
			}
			used = append(used, h.ID)
		}
	}
	return used, nil
}

// restoreContestState gives back what an attack or a cast took from the contests' state:
// the hiding it ended, the Help it used and the effect whose disadvantage it spent (the
// master's undo). An effect whose clock has ended it since is gone, and nothing comes back.
func restoreContestState(ctx context.Context, c *combatTx, ev actionEvent) error {
	if err := restoreHiding(ctx, c, ev.Actor, ev.HidBefore); err != nil {
		return err
	}
	for _, id := range ev.HelpUsed {
		if err := c.q.SetHelpConsumed(ctx, playdb.SetHelpConsumedParams{ID: id, ConsumedAt: nil}); err != nil {
			return fmt.Errorf("give the help back: %w", err)
		}
	}
	for _, id := range ev.EffectsSpent {
		if err := c.q.SetLastingEffectSpent(ctx, playdb.SetLastingEffectSpentParams{ID: id, SpentAt: nil}); err != nil {
			return fmt.Errorf("give the effect back: %w", err)
		}
	}
	return nil
}
