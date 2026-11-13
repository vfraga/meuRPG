package play

import (
	"context"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// What GetTurnOptions says of the contests (SRD 5.1, "Grappling", "Shoving a Creature",
// "Hide", "Help", "Surprise"): the special attacks under "Atacar", the facts of the
// combatant's turn (grappled, holding, hidden, surprised) and the Hide and Help actions.

// contestTargets lists who a grapple or a shove may aim at, as the viewer sees them: every
// combatant they see, not defeated, within the melee reach (the whole table without a map,
// where the master judges the reach); a creature more than one size larger is listed with
// its reason. The master has the last word: nothing is refused to him.
func contestTargets(enc playdb.Encounter, cs []playdb.Combatant, who playdb.Combatant, v combatViewer) []*playv1.ContestTarget {
	var out []*playv1.ContestTarget
	for _, t := range cs {
		if t.ID == who.ID || t.Defeated || !v.sees(t) {
			continue
		}
		target := &playv1.ContestTarget{CombatantId: t.ID, Size: sizeProto(t.Size), Eligible: true}
		if !isTheatre(enc) {
			dist, ok := distanceFt(who, t)
			switch {
			case !ok && !v.master:
				continue // a player cannot reach what has no square
			case ok && dist > combat.MeleeReachFt && !v.master:
				continue
			case ok:
				target.DistanceFt = dist
			}
		}
		if !v.master && !combat.CanGrappleOrShove(who.Size, t.Size) {
			target.Eligible, target.Reason = false, playv1.ContestTargetReason_CONTEST_TARGET_REASON_TOO_BIG
		}
		out = append(out, target)
	}
	return out
}

// characterGone says an error is the answer for a character that is gone from the campaign.
func characterGone(err error) bool { return connect.CodeOf(err) == connect.CodeNotFound }

// checkOptionOf is what a check would roll, as an option reads it: the master reads whether
// the number is known, a player always reads it.
func checkOptionOf(n checkNumbers, notes []rollNote, master bool) *playv1.CheckOption {
	return &playv1.CheckOption{Modifier: conv.Int32(n.Bonus), Known: n.Known || !master, Mode: checkModeProto(notesMode(notes)), Notes: notesProto(notes, master)}
}

// escapeAvailability says whether the grappled combatant can escape now, and if not why: the
// gate (it is not its turn...) or the action it spent (the master is never refused).
func escapeAvailability(gate rulesv1.DisabledReasonCode, who playdb.Combatant, master bool) *rulesv1.DisabledReason {
	reason := gateReason(gate)
	if reason == nil && who.ActionUsed && !master {
		reason = &rulesv1.DisabledReason{Code: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED}
	}
	return reason
}

// fillGrappled fills the state of a grappled combatant: who holds it (never a creature the
// viewer does not see), whether it can escape and what each skill of the escape would roll.
func (s *Service) fillGrappled(ctx context.Context, d *encounterData, v combatViewer, who playdb.Combatant, hold playdb.CombatHold, gate rulesv1.DisabledReasonCode, r *contestReader, state *playv1.ContestTurnState) error {
	state.Grappled = true
	if grappler, ok := combatantByID(d.cs, hold.GrapplerID); ok && (v.master || v.sees(grappler)) {
		state.GrapplerId = grappler.ID
	}
	state.EscapeReason = escapeAvailability(gate, who, v.master)
	state.CanEscape = state.EscapeReason == nil
	for _, sk := range []string{skillAthletics, skillAcrobat} {
		n, notes, err := r.optionOf(ctx, who, sk)
		if err != nil {
			return err
		}
		state.EscapeOptions = append(state.EscapeOptions, &playv1.ContestSkillOption{
			Skill: skillEnum(sk), Modifier: conv.Int32(n.Bonus), Known: n.Known || !v.master,
			Mode: checkModeProto(notesMode(notes)), Notes: notesProto(notes, v.master),
		})
	}
	return nil
}

// holdingOf is the creature the combatant holds, when the viewer sees it.
func holdingOf(cd *contestData, cs []playdb.Combatant, v combatViewer, who playdb.Combatant) string {
	for _, h := range cd.holds {
		if h.GrapplerID != who.ID {
			continue
		}
		if g, ok := combatantByID(cs, h.GrappledID); ok && isGrappled(g) && (v.master || v.sees(g)) {
			return g.ID
		}
	}
	return ""
}

// helpTargetOf is the creature the combatant was helped to attack, when the viewer sees it.
func helpTargetOf(cd *contestData, cs []playdb.Combatant, v combatViewer, who playdb.Combatant) string {
	var out string
	for _, h := range cd.helps {
		if h.Kind == helpAttack && helpsAlly(h, who) {
			if t, ok := combatantByID(cs, conv.OrZero(h.TargetID)); ok && (v.master || v.sees(t)) {
				out = t.ID
			}
		}
	}
	return out
}

// contestTurnState is the state of a combatant's turn the contests add: hidden, grappled,
// holding, surprised, helped, and the Hide and Help actions.
func (s *Service) contestTurnState(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, who playdb.Combatant, opts *rulesv1.TurnOptions, gate rulesv1.DisabledReasonCode, r *contestReader) (*playv1.ContestTurnState, error) {
	cd := r.d
	state := &playv1.ContestTurnState{Hidden: isHidden(cd.hiding, who.ID)}
	// The grapple: who holds this combatant, and who it holds.
	if hold, grappled := holdOn(holdsOf(cd.holds), who); grappled {
		if err := s.fillGrappled(ctx, d, v, who, hold, gate, r, state); err != nil {
			return nil, err
		}
	}
	state.HoldingId = holdingOf(cd, d.cs, v, who)
	surprised, err := s.surprisedNow(ctx, nil, m.CampaignID, d.enc, who)
	if err != nil {
		return nil, err
	}
	state.Surprised = surprised
	state.HelpTargetId = helpTargetOf(cd, d.cs, v, who)
	if err := s.hideAndHelpActions(ctx, m, who, opts, state, r); err != nil {
		return nil, err
	}
	return state, nil
}

// attackBudget is how many attacks of the Attack action the combatant has left and why the
// special attacks are off now (nil when they are on): the gate, or the action or attacks
// spent. The master is never refused for the economy.
func attackBudget(sheet link.Sheet, who playdb.Combatant, master bool, gate rulesv1.DisabledReasonCode) (int, *rulesv1.DisabledReason) {
	perAction := max(sheet.AttacksPerAction, 1)
	last, hasLast := lastAttack(sheet, who)
	castCantrip := hasLast && last.Spell
	left := combat.AttacksLeft(perAction, combat.TurnState{ActionUsed: who.ActionUsed, AttacksMade: int(who.AttacksMade)})
	if castCantrip {
		left = 0
	}
	reason := gateReason(gate)
	if reason == nil && left == 0 && !master {
		code := rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED
		if !castCantrip && who.AttacksMade > 0 && perAction > 1 {
			code = rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ATTACKS_USED
		}
		reason = &rulesv1.DisabledReason{Code: code}
	}
	return left, reason
}

// specialAttacks are the grapple and the shove that replace one attack of the Attack action.
// A combatant that cannot attack, or whose character is gone, has none to list.
func (s *Service) specialAttacks(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, who playdb.Combatant, gate rulesv1.DisabledReasonCode, r *contestReader) ([]*playv1.ContestAttackOption, error) {
	if mayAttack(who, false) != nil {
		return nil, nil //nolint:nilerr // a combatant that cannot attack has no special attack to list; that is no failure
	}
	sheet, err := s.sheetOf(ctx, nil, m.CampaignID, who)
	if err != nil {
		if characterGone(err) { // a character that is gone attacks no more
			return nil, nil
		}
		return nil, err
	}
	left, reason := attackBudget(sheet, who, v.master, gate)
	roll, _, err := s.contestRollOption(ctx, r, who)
	if err != nil {
		if characterGone(err) { // a character that is gone attacks no more
			return nil, nil
		}
		return nil, err
	}
	targets := contestTargets(d.enc, d.cs, who, v)
	maneuvers, err := s.grappleManeuverOptions(ctx, m.CampaignID, d.enc, who, sheet, gateReason(gate), v.master)
	if err != nil {
		return nil, err
	}
	var out []*playv1.ContestAttackOption
	for _, kind := range []playv1.ContestAttackOptionKind{playv1.ContestAttackOptionKind_CONTEST_ATTACK_OPTION_KIND_GRAPPLE, playv1.ContestAttackOptionKind_CONTEST_ATTACK_OPTION_KIND_SHOVE} {
		opt := &playv1.ContestAttackOption{
			Kind: kind, ReplacesAttack: true, Enabled: reason == nil, Reason: reason, AttacksLeft: clamp32(left, 0, math.MaxInt32),
			Targets: targets, RollOption: roll,
		}
		if kind == playv1.ContestAttackOptionKind_CONTEST_ATTACK_OPTION_KIND_GRAPPLE {
			opt.Maneuvers = maneuvers
		}
		out = append(out, opt)
	}
	return out, nil
}

// contestOptionsFor works out the contest side of GetTurnOptions for a combatant: the
// special attacks that replace one attack of the Attack action, and the state of its turn. gate
// is why the combatant cannot act now (UNSPECIFIED when it can).
func (s *Service) contestOptionsFor(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, who playdb.Combatant, opts *rulesv1.TurnOptions, gate rulesv1.DisabledReasonCode) ([]*playv1.ContestAttackOption, *playv1.ContestTurnState, error) {
	cd, err := s.loadContestData(ctx, s.queries, d.enc, d.cs)
	if err != nil {
		return nil, nil, err
	}
	r := &contestReader{s: s, m: m, v: v, d: cd}
	state, err := s.contestTurnState(ctx, m, d, v, who, opts, gate, r)
	if err != nil {
		return nil, nil, err
	}
	out, err := s.specialAttacks(ctx, m, d, v, who, gate, r)
	if err != nil {
		return nil, nil, err
	}
	return out, state, nil
}

// contestRollOption is what the combatant's Strength (Athletics) check would roll.
func (s *Service) contestRollOption(ctx context.Context, r *contestReader, who playdb.Combatant) (*playv1.CheckOption, []rollNote, error) {
	n, notes, err := r.optionOf(ctx, who, skillAthletics)
	if err != nil {
		return nil, nil, err
	}
	return checkOptionOf(n, notes, r.v.master), notes, nil
}

// contestActionOf is a standard or feature action as the contests list it.
func contestActionOf(a *rulesv1.ActionOption) *playv1.ContestAction {
	return &playv1.ContestAction{Key: a.GetAction().GetKey(), Economy: a.GetAction().GetEconomy(), Enabled: a.GetEnabled(), Reason: a.GetReason()}
}

// standardHideAndHelp lists the standard Hide and Help of the turn options.
func standardHideAndHelp(opts *rulesv1.TurnOptions, state *playv1.ContestTurnState) {
	for _, a := range opts.GetStandardActions() {
		switch a.GetAction().GetKey() {
		case actionHide:
			state.HideActions = append(state.HideActions, contestActionOf(a))
		case actionHelp:
			state.HelpAction = contestActionOf(a)
		}
	}
}

// featureHideActions lists the features that give the Hide action (Cunning Action). It says
// gone when the character is gone, and the rest of the actions are not listed.
func (s *Service) featureHideActions(ctx context.Context, m authz.Membership, who playdb.Combatant, opts *rulesv1.TurnOptions, state *playv1.ContestTurnState) (gone bool, err error) {
	if len(opts.GetFeatureActions()) == 0 {
		return false, nil
	}
	sheet, err := s.sheetOf(ctx, nil, m.CampaignID, who)
	if err != nil {
		return characterGone(err), ignoreGone(err) // a character that is gone has no feature to list
	}
	for _, a := range opts.GetFeatureActions() {
		if i := slices.IndexFunc(sheet.FeatureActions, func(fa link.FeatureAction) bool { return fa.Key == a.GetAction().GetKey() }); i >= 0 && sheet.FeatureActions[i].Standard == actionHide {
			state.HideActions = append(state.HideActions, contestActionOf(a))
		}
	}
	return false, nil
}

// ignoreGone is the error of a call, or nil when it only says the character is gone.
func ignoreGone(err error) error {
	if characterGone(err) {
		return nil
	}
	return err
}

// hideOptionOf is what a Hide would roll, when the combatant can hide at all.
func (s *Service) hideOptionOf(ctx context.Context, who playdb.Combatant, state *playv1.ContestTurnState, r *contestReader) error {
	if len(state.HideActions) == 0 {
		return nil
	}
	n, notes, err := r.optionOf(ctx, who, skillStealth)
	if err != nil {
		if characterGone(err) { // a character that is gone hides no more
			state.HideActions = nil
		}
		return ignoreGone(err)
	}
	state.HideOption = checkOptionOf(n, notes, r.v.master)
	return nil
}

// hideAndHelpActions fills what the sheet's actions say of Hide and Help: the standard Hide
// and the features that give it (Cunning Action), the Help, each with its economy and
// whether it can be taken now, and what a Hide would roll.
func (s *Service) hideAndHelpActions(ctx context.Context, m authz.Membership, who playdb.Combatant, opts *rulesv1.TurnOptions, state *playv1.ContestTurnState, r *contestReader) error {
	standardHideAndHelp(opts, state)
	gone, err := s.featureHideActions(ctx, m, who, opts, state)
	if err != nil || gone {
		return err
	}
	return s.hideOptionOf(ctx, who, state, r)
}

// gateReason is the reason a gate code gives for a disabled option, nil when the combatant can act.
func gateReason(code rulesv1.DisabledReasonCode) *rulesv1.DisabledReason {
	if code == rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_UNSPECIFIED {
		return nil
	}
	return &rulesv1.DisabledReason{Code: code}
}
