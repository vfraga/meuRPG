package play

import (
	"context"
	"fmt"
	"math"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// What a caller reads of the open reaction windows (PM-04, RN-10, RN-20): the master
// every window with its numbers; a player only the windows they answer, with what their
// own character sees; everyone else only the line "Esperando ...", written for them.

var reactionKindProto = map[reaction.Kind]playv1.ReactionKind{
	reaction.Shield:             playv1.ReactionKind_REACTION_KIND_SHIELD,
	reaction.UncannyDodgeKind:   playv1.ReactionKind_REACTION_KIND_UNCANNY_DODGE,
	reaction.HellishRebukeKind:  playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE,
	reaction.CounterspellKind:   playv1.ReactionKind_REACTION_KIND_COUNTERSPELL,
	reaction.CuttingWords:       playv1.ReactionKind_REACTION_KIND_CUTTING_WORDS,
	reaction.DeflectKind:        playv1.ReactionKind_REACTION_KIND_DEFLECT_MISSILES,
	reaction.ManeuverReduceKind: playv1.ReactionKind_REACTION_KIND_MANEUVER_REDUCE,
	reaction.ProtectionKind:     playv1.ReactionKind_REACTION_KIND_PROTECTION,
	reaction.FeatherFall:        playv1.ReactionKind_REACTION_KIND_FEATHER_FALL,
	reaction.Concentration:      playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE,
	reaction.MasterCheck:        playv1.ReactionKind_REACTION_KIND_MASTER_CHECK,
	reaction.Contest:            playv1.ReactionKind_REACTION_KIND_CONTEST,
	reaction.EffectSave:         playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE,
}

// windowView is what the builders of a caller's windows share.
type windowView struct {
	s     *Service
	ctx   context.Context
	m     authz.Membership
	v     combatViewer
	d     *encounterData
	names func(key string) string
	byID  map[string]playdb.Combatant
}

// reactionView builds Encounter.reaction_windows and Encounter.reaction_wait for the
// caller. It reads through the pool: it is a read (GetEncounter, a response after
// the commit).
func (s *Service) reactionView(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, names func(key string) string) ([]*playv1.ReactionWindow, *playv1.ReactionWait, error) {
	if d.enc.Status != statusActive {
		return nil, nil, nil
	}
	open, err := s.queries.ListOpenReactionWindows(ctx, d.enc.ID)
	if err != nil {
		return nil, nil, s.dbError(ctx, "list the reaction windows", err)
	}
	offers, err := s.queries.ListPendingOpportunityOffers(ctx, d.enc.ID)
	if err != nil {
		return nil, nil, s.dbError(ctx, "list the opportunity offers", err)
	}
	reveals, err := s.queries.ListPendingHiddenReveals(ctx, d.enc.ID)
	if err != nil {
		return nil, nil, s.dbError(ctx, "list the hidden reveals", err)
	}
	if len(open) == 0 && len(offers) == 0 && len(reveals) == 0 {
		return nil, nil, nil
	}
	wv := &windowView{s: s, ctx: ctx, m: m, v: v, d: d, names: names, byID: combatantsByID(d.cs)}
	line := waitLine{holds: reaction.HoldsTurn}
	windows, err := wv.openWindows(open, &line)
	if err != nil {
		return nil, nil, err
	}
	windows = wv.offerWindows(offers, windows, &line)
	// The master's question about a hidden creature an area hit (PM-02c) is a window too: the
	// master answers it with ResolveHiddenReveal, and every player reads the same wait.
	windows = wv.revealWindows(reveals, windows, &line)
	return windows, line.build(wv.v.master), nil
}

// waitLine gathers what the windows the caller does not answer say of the wait: who is waited
// for, and what the first of them holds.
type waitLine struct {
	wait     reaction.Wait
	holds    reaction.Holds
	holdsSet bool
}

// holdFirst remembers what a window holds; only the first one counts.
func (l *waitLine) holdFirst(h reaction.Holds) {
	if !l.holdsSet {
		l.holds, l.holdsSet = h, true
	}
}

// openWindows builds the open windows the caller answers and adds each to the line the
// caller reads.
func (wv *windowView) openWindows(open []playdb.ReactionWindow, line *waitLine) ([]*playv1.ReactionWindow, error) {
	order := make([]reaction.Window, len(open))
	for i, w := range open {
		order[i] = reaction.Window{ID: w.ID, Group: w.GroupID, Seq: w.Seq, Open: true}
	}
	var windows []*playv1.ReactionWindow
	for _, w := range open {
		pw, err := wv.openOne(w, order, line)
		if err != nil {
			return nil, err
		}
		if pw != nil {
			windows = append(windows, pw)
		}
	}
	return windows, nil
}

// openOne is one open window: the window the caller answers (nil when it is not theirs, or not theirs yet) and its
// part in the line the caller reads.
func (wv *windowView) openOne(w playdb.ReactionWindow, order []reaction.Window, line *waitLine) (*playv1.ReactionWindow, error) {
	reactor := wv.reactorOf(w)
	answers := wv.v.master || (reactor != nil && wv.v.owns(*reactor))
	now := reaction.AnswerNow(order, w.ID)
	var pw *playv1.ReactionWindow
	if answers && wv.shownNow(w, now) {
		var err error
		if pw, err = wv.window(w, reactor, now); err != nil {
			return nil, err
		}
	}
	wv.waitOf(w, reactor, answers, &line.wait)
	if !line.holdsSet && !answers {
		h, err := wv.holdsOf(w)
		if err != nil {
			return nil, err
		}
		line.holdFirst(h)
	}
	return pw, nil
}

// shownNow says the window the caller answers is in their list now: a player is asked the questions of the zones
// one at a time, the next opening when the one before is answered.
func (wv *windowView) shownNow(w playdb.ReactionWindow, now bool) bool {
	return now || wv.v.master || windowTriggerOf(w).Zone == ""
}

// reactorOf is the combatant that reacts in the window; nil when it has none or it left.
func (wv *windowView) reactorOf(w playdb.ReactionWindow) *playdb.Combatant {
	if w.ReactorID == nil {
		return nil
	}
	if r, ok := wv.byID[*w.ReactorID]; ok {
		return &r
	}
	return nil
}

// build is the line the caller reads of the windows it does not answer, with what they hold for
// the player; nil when nothing is waited for.
func (l *waitLine) build(master bool) *playv1.ReactionWait {
	title := l.wait.Title()
	if title == "" {
		return nil
	}
	out := &playv1.ReactionWait{TitlePt: title}
	if !master {
		out.DetailPt = reaction.Detail(l.holds, l.wait, "")
	}
	return out
}

// offerWindows adds the opportunity-attack offers the reader may see, as windows (RN-21): the offer's
// own calls answer them.
func (wv *windowView) offerWindows(offers []playdb.OpportunityOffer, windows []*playv1.ReactionWindow, line *waitLine) []*playv1.ReactionWindow {
	// The opportunity attacks are windows too (RN-21): the offer's own calls answer them.
	for _, o := range offers {
		mover, ok1 := wv.byID[o.MoverID]
		reactor, ok2 := wv.byID[o.ReactorID]
		if !ok1 || !ok2 || !wv.v.seesAtOffer(mover, o) {
			continue
		}
		answers := wv.v.master || wv.v.owns(reactor)
		if !answers && !wv.v.owns(mover) {
			continue
		}
		if answers {
			windows = append(windows, wv.opportunityWindow(o, mover, reactor))
		}
		wv.waitOf(playdb.ReactionWindow{Kind: "opportunity"}, &reactor, answers, &line.wait)
		if !answers {
			line.holdFirst(reaction.HoldsTurn)
		}
	}
	return windows
}

// revealWindows adds the master's questions about hidden creatures an area hit: the master reads each
// as a window, and every reader's wait counts them (the master as his own reaction, a player as "o mestre").
func (wv *windowView) revealWindows(reveals []playdb.HiddenReveal, windows []*playv1.ReactionWindow, line *waitLine) []*playv1.ReactionWindow {
	for _, q := range reveals {
		switch {
		case wv.v.master:
			windows = append(windows, &playv1.ReactionWindow{
				Id: q.ID, Kind: playv1.ReactionKind_REACTION_KIND_HIDDEN_REVEAL, Status: playv1.ReactionWindowStatus_REACTION_WINDOW_STATUS_OPEN,
				GroupId: q.ID, ForYou: true, AnswerNow: true,
				Trigger: &playv1.ReactionTrigger{ActorId: q.CasterID},
			})
			line.wait.Self = true
		default:
			line.wait.Master = true
		}
		line.holdFirst(reaction.HoldsTurn)
	}
	return windows
}

// opportunityWindow is the window of an opportunity-attack offer for someone who answers it:
// the master also reads who moves and who may react.
func (wv *windowView) opportunityWindow(o playdb.OpportunityOffer, mover, reactor playdb.Combatant) *playv1.ReactionWindow {
	pw := &playv1.ReactionWindow{
		Id: o.ID, Kind: playv1.ReactionKind_REACTION_KIND_OPPORTUNITY, Status: playv1.ReactionWindowStatus_REACTION_WINDOW_STATUS_OPEN,
		GroupId: o.MoveID, ReactorId: reactor.ID, ReactorLabel: reactor.Label, ReactorIsPlayer: reactor.Kind == kindPlayer, ForYou: true, AnswerNow: wv.v.master,
	}
	if wv.v.master {
		pw.Trigger = &playv1.ReactionTrigger{ActorId: mover.ID, ActorLabel: mover.Label, TargetId: reactor.ID, TargetLabel: reactor.Label}
	}
	return pw
}

// waitOf adds a window to the line the caller reads. A window the caller answers is
// not waited for: they have its prompt. A reactor that is an NPC, the master's check
// or a creature the caller does not see is "o mestre"; only a player's character the
// caller sees is named (never which NPC, never why).
func (wv *windowView) waitOf(w playdb.ReactionWindow, reactor *playdb.Combatant, answers bool, wait *reaction.Wait) {
	switch reaction.Kind(w.Kind) {
	case reaction.Contest:
		wv.contestWait(reactor, answers, wait)
	case reaction.EffectSave:
		wv.effectSaveWait(w, reactor, answers, wait)
	default:
		wv.reactionWait(reaction.Kind(w.Kind) == reaction.Concentration, reactor, answers, wait)
	}
}

// contestWait names the player that owes the next step of a contest, and is the master's
// otherwise; whoever answers it reads its sheet instead.
func (wv *windowView) contestWait(reactor *playdb.Combatant, answers bool, wait *reaction.Wait) {
	switch {
	case answers:
	case reactor != nil && reactor.Kind == kindPlayer && wv.v.sees(*reactor):
		wait.Contesters = append(wait.Contesters, reactor.Label)
	default:
		wait.Master = true
	}
}

// reactionWait adds a reaction or a concentration save (save) to the line: the master
// reads every player's, a player only the ones of characters they see.
func (wv *windowView) reactionWait(save bool, reactor *playdb.Combatant, answers bool, wait *reaction.Wait) {
	player := reactor != nil && reactor.Kind == kindPlayer
	if wv.v.master {
		switch {
		case player && save:
			wait.Savers = append(wait.Savers, reactor.Label)
		case player:
			wait.Reactors = append(wait.Reactors, reactor.Label)
		default:
			wait.Self = true
		}
		return
	}
	if answers {
		return
	}
	switch {
	case player && wv.v.sees(*reactor) && save:
		wait.Savers = append(wait.Savers, reactor.Label)
	case player && wv.v.sees(*reactor):
		wait.Reactors = append(wait.Reactors, reactor.Label)
	default:
		wait.Master = true
	}
}

// holdsOf says what the window holds, for the caller: their own attack or spell, or
// only the turn.
func (wv *windowView) holdsOf(w playdb.ReactionWindow) (reaction.Holds, error) {
	t := windowTriggerOf(w)
	if w.HoldID != nil {
		h, err := wv.s.queries.GetReactionHold(wv.ctx, playdb.GetReactionHoldParams{EncounterID: wv.d.enc.ID, ID: *w.HoldID})
		if err == nil && h.ActorUserID == wv.v.userID {
			if h.Kind == "cast" {
				return reaction.HoldsYourSpell, nil
			}
			return reaction.HoldsYourAttack, nil
		}
		return reaction.HoldsTurn, nil
	}
	if w.PendingDamageID != nil {
		if a, ok := wv.byID[t.Actor]; ok && wv.v.owns(a) {
			return reaction.HoldsYourAttack, nil
		}
	}
	return reaction.HoldsTurn, nil
}

// window builds one window for whoever answers it.
func (wv *windowView) window(w playdb.ReactionWindow, reactor *playdb.Combatant, answerNow bool) (*playv1.ReactionWindow, error) {
	kind := reaction.Kind(w.Kind)
	out := &playv1.ReactionWindow{
		Id: w.ID, Kind: reactionKindProto[kind], Status: playv1.ReactionWindowStatus_REACTION_WINDOW_STATUS_OPEN, GroupId: w.GroupID,
		ForYou: true, SecondStep: w.Step == stepSecond, Rolling: windowTriggerOf(w).Rolling,
	}
	if reactor != nil {
		out.ReactorId, out.ReactorLabel, out.ReactorIsPlayer = reactor.ID, reactor.Label, reactor.Kind == kindPlayer
	}
	if wv.v.master {
		out.AnswerNow = answerNow
		out.Trigger = wv.trigger(w)
		out.Unanswered = wv.unanswered(w, reactor)
	}
	var err error
	switch kind {
	case reaction.Shield:
		err = wv.shieldPrompt(w, reactor, out)
	case reaction.MasterCheck:
		wv.masterCheckPrompt(w, out)
	case reaction.Contest:
		out.Prompt = &playv1.ReactionWindow_Contest{Contest: &playv1.ContestPrompt{ContestId: windowTriggerOf(w).Contest}}
	case reaction.Concentration:
		err = wv.concentrationPrompt(w, reactor, out)
	case reaction.EffectSave:
		err = wv.effectSavePrompt(w, reactor, out)
	default:
		err = wv.morePrompt(kind, w, reactor, out)
	}
	return out, err
}

// unanswered says the question of a player's character has waited for the player longer than
// the player's first chance (the same 10 s as a held roll's): it comes to the master now.
func (wv *windowView) unanswered(w playdb.ReactionWindow, reactor *playdb.Combatant) bool {
	return reactor != nil && reactor.Kind == kindPlayer && reactor.UserID != nil && w.Status == windowOpen &&
		wv.s.now().Sub(w.CreatedAt) >= heldPlayerFirst
}

// trigger is what happened, with the master's numbers.
func (wv *windowView) trigger(w playdb.ReactionWindow) *playv1.ReactionTrigger {
	t := windowTriggerOf(w)
	out := &playv1.ReactionTrigger{
		ActorId: t.Actor, TargetId: t.Target, ActionKey: t.Key,
		SpellKey: t.Spell, SpellLevel: t.Level, ConcentrationSpellKey: "", DamageTaken: t.Taken,
	}
	if a, ok := wv.byID[t.Actor]; ok {
		out.ActorLabel = a.Label
	}
	if x, ok := wv.byID[t.Target]; ok {
		out.TargetLabel = x.Label
	}
	if t.Key != "" {
		out.ActionNamePt = wv.names(t.Key)
		if out.ActionNamePt == "" {
			out.ActionNamePt = wv.attackName(t)
		}
	}
	if t.Distance > 0 {
		out.DistanceFt = &t.Distance
	}
	if t.Total != 0 || t.AC != 0 {
		out.AttackTotal, out.TargetArmorClass = &t.Total, &t.AC
	}
	if t.Damage != 0 {
		out.Damage = &t.Damage
	}
	if reaction.Kind(w.Kind) == reaction.Concentration {
		out.ConcentrationSpellKey = t.Spell
	}
	// Unlike rollKindOf, a trigger that names no roll stays unspecified.
	switch t.Roll {
	case "attack":
		out.RollKind = playv1.ReactionRollKind_REACTION_ROLL_KIND_ATTACK
	case "test":
		out.RollKind = playv1.ReactionRollKind_REACTION_ROLL_KIND_TEST
	case "damage":
		out.RollKind = playv1.ReactionRollKind_REACTION_ROLL_KIND_DAMAGE
	}
	return out
}

// attackName is the Portuguese name of the attack of a trigger, from its actor's sheet.
func (wv *windowView) attackName(t windowTrigger) string {
	a, ok := wv.byID[t.Actor]
	if !ok {
		return ""
	}
	sheet, err := wv.s.sheetOf(wv.ctx, nil, wv.m.CampaignID, a)
	if err != nil {
		return ""
	}
	if i := slices.IndexFunc(sheet.Attacks, func(x link.Attack) bool { return x.Key == t.Key }); i >= 0 {
		return sheet.Attacks[i].Name
	}
	return ""
}

// attackerOf names who attacked and with what, only when the caller sees them (RN-20).
func (wv *windowView) attackerOf(t windowTrigger) (label, attack string) {
	a, ok := wv.byID[t.Actor]
	if !ok || !wv.v.sees(a) {
		return "", ""
	}
	return a.Label, wv.attackName(t)
}

// shieldPrompt is the Shield prompt of a window: the pending damage, the attacker when the viewer
// sees it and the free slots of the reactor; the master also reads the armor class with the spell.
func (wv *windowView) shieldPrompt(w playdb.ReactionWindow, reactor *playdb.Combatant, out *playv1.ReactionWindow) error {
	t := windowTriggerOf(w)
	p := &playv1.ShieldPrompt{PendingDamageId: conv.OrZero(w.PendingDamageID), MagicMissile: t.Magic, SpellNamePt: wv.names(shield)}
	p.AttackerLabel, p.AttackNamePt = wv.attackerOf(t)
	if reactor != nil {
		k, ok, err := wv.s.kitOf(wv.ctx, nil, wv.m.CampaignID, *reactor)
		if err != nil {
			return wv.s.dbError(wv.ctx, "work out the reaction", err)
		}
		if ok {
			p.Slots = k.slotsFor(spellShield)
		}
		if wv.v.master {
			with := clamp32(int(t.AC)-int(reactor.AcBonus)+5, 0, math.MaxInt32)
			if out.Trigger != nil {
				out.Trigger.ArmorClassWithShield = &with
			}
		}
	}
	out.Prompt = &playv1.ReactionWindow_Shield{Shield: p}
	return nil
}

func (wv *windowView) masterCheckPrompt(w playdb.ReactionWindow, out *playv1.ReactionWindow) {
	t := windowTriggerOf(w)
	a, tg := wv.byID[t.Actor], wv.byID[t.Target]
	summary := fmt.Sprintf("Ataque de %s contra %s", a.Label, tg.Label)
	switch {
	case t.Magic:
		summary = fmt.Sprintf("Mísseis Mágicos de %s contra %s", a.Label, tg.Label)
	case t.Roll == "cast" || t.Spell != "":
		summary = fmt.Sprintf("Conjuração de %s", a.Label)
	case t.Roll == "damage":
		summary = fmt.Sprintf("Dano de %s em %s", a.Label, tg.Label)
	case t.Roll == "test":
		summary = fmt.Sprintf("Teste de %s", a.Label)
	case t.AC != 0 || t.Total != 0:
		summary += " · acertou"
	}
	out.Prompt = &playv1.ReactionWindow_MasterCheck{MasterCheck: &playv1.MasterCheckPrompt{SummaryPt: summary}}
}

func (wv *windowView) concentrationPrompt(w playdb.ReactionWindow, reactor *playdb.Combatant, out *playv1.ReactionWindow) error {
	t := windowTriggerOf(w)
	p := &playv1.ConcentrationSavePrompt{
		SpellKey: t.Spell, SpellNamePt: wv.names(t.Spell), DamageTaken: t.Taken,
		HandedToMaster: t.Handed,
	}
	if wv.v.master || (reactor != nil && wv.v.saveDCFor(*reactor)) { // the DC is the master's, and the reading member's when the table shows it
		p.Dc = clamp32(reaction.ConcentrationDC(int(t.Taken)), 0, math.MaxInt32)
	}
	if a, ok := wv.byID[t.Actor]; ok && wv.v.sees(a) && t.Key != "" {
		if name := wv.names(t.Key); name != "" {
			p.SourceLabel = name + " de " + a.Label
		} else if n := wv.attackName(t); n != "" {
			p.SourceLabel = n + " de " + a.Label
		}
	}
	if reactor != nil {
		save, err := wv.s.saveOf(wv.ctx, nil, wv.m.CampaignID, *reactor, "con")
		if err != nil {
			return wv.s.dbError(wv.ctx, "read a saving throw", err)
		}
		p.SaveBonus, p.BonusKnown = conv.Int32(save.Bonus), save.Known
		sheet, err := wv.s.sheetOf(wv.ctx, nil, wv.m.CampaignID, *reactor)
		if err != nil {
			return wv.s.dbError(wv.ctx, "read a sheet", err)
		}
		sources := concentrationSources(creatureFacts(*reactor, statesOf(wv.d.states), sheet.Traits))
		p.Mode = modeKey(combat.Resolve(sources))
		for _, src := range shownSources(sources, wv.names) {
			p.ModeSourcesPt = append(p.ModeSourcesPt, src.GetTextPt())
		}
	}
	out.Prompt = &playv1.ReactionWindow_ConcentrationSave{ConcentrationSave: p}
	return nil
}
