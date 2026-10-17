package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The standard and feature actions that only spend the economy and a use (TakeAction), in phases.
// TakeAction reads the request, runs the transaction as a sequence of phases of a takeActionFlow, each
// one of the checks or the writes the action has always made in that order, and answers from what the
// transaction left in a takeActionOut. Each kind of action (a standard action, a feature's, one of the
// feature kinds with its own effect) is a function behind a dispatch. The phases keep the order of the
// reads, the dice and the writes: the golden of combat_take_action_frozen_test.go reads it.

// The standard actions with an effect on the turn of their own.
const (
	actionDodge     = "standard:dodge"
	actionDash      = "standard:dash"
	actionDisengage = "standard:disengage"
)

// takeActionCall is what TakeAction reads from its request, checked.
type takeActionCall struct {
	key, encounterID, combatantID string
	actionKey                     string
	// in and rolled are the roll the request carries, for the actions that roll (Second Wind).
	in     rollInput
	rolled bool
	// useExtra says the action uses the extra action Haste gives (SRD 5.1, Haste).
	useExtra bool
	// frenzy says the rage is a frenzy (SRD 5.1, Path of the Berserker).
	frenzy bool
}

// readTakeActionCall reads the request, in the order the answers are given: the ids, the action, the roll.
func readTakeActionCall(req *playv1.TakeActionRequest) (takeActionCall, error) {
	var call takeActionCall
	var err error
	if call.key, err = parseKey(req.GetIdempotencyKey()); err != nil {
		return call, err
	}
	if call.encounterID, err = parseCombatID(req.GetEncounterId(), "encounter"); err != nil {
		return call, err
	}
	if call.combatantID, err = parseCombatID(req.GetCombatantId(), "combatant"); err != nil {
		return call, err
	}
	call.actionKey = req.GetActionKey()
	if call.actionKey == standardAttack || call.actionKey == standardCast {
		return call, connect.NewError(connect.CodeInvalidArgument, errors.New("an attack is rolled with RollAttack, and a spell is cast with CastSpell"))
	}
	if call.in, call.rolled, err = readTakeActionRoll(req); err != nil {
		return call, err
	}
	call.useExtra, call.frenzy = req.GetUseExtraAction(), req.GetFrenzy()
	return call, nil
}

// readTakeActionRoll reads the roll of the request: the app's, a typed sum, or none.
func readTakeActionRoll(req *playv1.TakeActionRequest) (in rollInput, rolled bool, err error) {
	switch roll := req.GetRoll().(type) {
	case *playv1.TakeActionRequest_RollInApp:
		if !roll.RollInApp {
			return in, false, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp, rolled = true, true
	case *playv1.TakeActionRequest_TypedSum:
		in.typed, rolled = int(roll.TypedSum), true
	}
	return in, rolled, nil
}

// takeActionOut is what the transaction of an action leaves for its answer.
type takeActionOut struct {
	made actionEvent
	// vitals are the vitals of the resource spent, or of the hit points healed.
	vitals *playv1.CharacterVitals
}

// TakeAction implements playv1connect.CombatServiceHandler.
func (s *Service) TakeAction(
	ctx context.Context,
	req *connect.Request[playv1.TakeActionRequest],
) (*connect.Response[playv1.TakeActionResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	call, err := readTakeActionCall(req.Msg)
	if err != nil {
		return nil, err
	}
	out := &takeActionOut{}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventActionTaken, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return newTakeActionFlow(s, c, m, call, out).run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "take an action", err)
	}
	return s.takeActionResponse(ctx, m, res, out)
}

// takeActionResponse answers from what the transaction left: the encounter as the caller sees it, and the
// roll and the hit points of an action that heals.
func (s *Service) takeActionResponse(ctx context.Context, m authz.Membership, res combatResult, out *takeActionOut) (*connect.Response[playv1.TakeActionResponse], error) {
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the action", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishVitals(m.CampaignID, out.vitals)
	})
	if err != nil {
		return nil, err
	}
	resp := &playv1.TakeActionResponse{Encounter: enc}
	if ev.Heal {
		resp.Roll = diceRoll(ev.DiceCount, ev.DiceSides, ev.Faces, ev.Modifier, ev.Total, ev.Physical)
		resp.Healed = &ev.Amount
	}
	return connect.NewResponse(resp), nil
}

// takeActionFlow is one run of the action's transaction: what each phase reads, and the next one uses.
type takeActionFlow struct {
	s    *Service
	c    *combatTx
	m    authz.Membership
	call takeActionCall
	out  *takeActionOut

	v    combatViewer
	who  playdb.Combatant
	opts *rulesv1.TurnOptions
	// feature says the action is a class or subclass feature's or a race trait's, and action is the option
	// the combatant has for it.
	feature bool
	action  *rulesv1.ActionOption
	// economy is what the action spends; the extra action of Haste pays for it instead.
	economy rulesv1.ActionEconomy
	made    actionEvent
}

func newTakeActionFlow(s *Service, c *combatTx, m authz.Membership, call takeActionCall, out *takeActionOut) *takeActionFlow {
	return &takeActionFlow{s: s, c: c, m: m, call: call, out: out}
}

// run is the transaction: the checks, what the action spends and what it does. It answers the event the
// write stores.
func (f *takeActionFlow) run(ctx context.Context) (any, error) {
	f.out.vitals = nil
	phases := []func(context.Context) error{f.locate, f.choose, f.mayTake, f.begin, f.spend, f.perform, f.touch}
	for _, phase := range phases {
		if err := phase(ctx); err != nil {
			return nil, err
		}
	}
	f.out.made = f.made
	return f.made, nil
}

// locate finds the combatant that acts, as the caller sees the fog, and the options it has.
func (f *takeActionFlow) locate(ctx context.Context) error {
	cs, err := f.c.q.ListCombatants(ctx, f.c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	f.v = f.c.viewer(f.m, cs) // the fog: an NPC the player does not see is not found
	if f.who, err = findCombatant(cs, f.call.combatantID, f.v); err != nil {
		return err
	}
	if err := f.v.mayAct(f.who); err != nil {
		return err
	}
	f.opts, err = f.s.optionsOf(ctx, f.c.tx, f.m.CampaignID, f.who)
	return err
}

// choose finds the action among the standard or feature actions the combatant has, and refuses the ones
// that have a call of their own.
func (f *takeActionFlow) choose(context.Context) error {
	key := f.call.actionKey
	if wildShapeFeature(key) {
		// It needs the beast: AssumeWildShape takes it (MR-037).
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the Wild Shape action is taken with AssumeWildShape"))
	}
	if slices.Contains(resourceFlowActions, key) {
		// They need a target or a number of points: UseLayOnHands, CreateSpellSlot,
		// ConvertSpellSlot and GiveBardicInspiration take them.
		return connect.NewError(connect.CodeInvalidArgument, errors.New("this action is taken with the resource service"))
	}
	// A feature action is a class or subclass feature's or a race trait's (the goblin's
	// Fuga Ágil, from the table's content).
	f.feature = strings.HasPrefix(key, "feature:") || strings.HasPrefix(key, "trait:")
	list := f.opts.GetStandardActions()
	if f.feature {
		list = f.opts.GetFeatureActions()
	}
	i := slices.IndexFunc(list, func(a *rulesv1.ActionOption) bool { return a.GetAction().GetKey() == key })
	if i < 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("action_key is not one of the standard or feature actions"))
	}
	f.action = list[i]
	if key == actionHide || key == actionHelp {
		return errHideOrHelp()
	}
	f.economy = f.action.GetAction().GetEconomy()
	return nil
}

// errHideOrHelp refuses Hide and Help, which are taken with the contest calls.
func errHideOrHelp() error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New("Hide and Help have their own calls: ContestService.Hide and ContestService.Help"))
}

// mayTake checks that the combatant may take the action now: its turn (or its reaction), the extra action it
// asks for, and the economy it has left.
func (f *takeActionFlow) mayTake(ctx context.Context) error {
	// A reaction is taken off turn, when its trigger happens; everything else
	// on the combatant's turn.
	if f.feature && f.economy == rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION {
		if err := f.s.mustReactNowOrOnTurn(ctx, f.c, f.who); err != nil {
			return err
		}
	} else if err := f.s.mustActNow(ctx, f.c, f.who); err != nil {
		return err
	}
	// The master has the last word: he may act again with the action used.
	if f.call.useExtra {
		if err := f.mayUseExtra(ctx); err != nil {
			return err
		}
	}
	if !f.action.GetEnabled() && !f.call.useExtra {
		return f.disabledError()
	}
	return nil
}

// mayUseExtra checks the extra action: it takes a standard action, the combatant has it and may spend it on
// this one.
func (f *takeActionFlow) mayUseExtra(ctx context.Context) error {
	if f.feature {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_EXTRA_ACTION_UNAVAILABLE, "the extra action takes a standard action")
	}
	return f.s.mustHaveExtraAction(ctx, f.c, f.who, f.call.actionKey)
}

// disabledError is the refusal of an action the options show disabled: a feature's own reason, or the
// action used. The master has the last word on the economy.
func (f *takeActionFlow) disabledError() error {
	if f.feature {
		return featureError(f.action.GetReason(), f.v.master)
	}
	if !f.v.master {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
	}
	return nil
}

// begin starts the event, with the state of the turn the undo puts back.
func (f *takeActionFlow) begin(ctx context.Context) error {
	run, err := breakRun(ctx, f.c, f.who)
	if err != nil {
		return err
	}
	who := f.who
	f.made = actionEvent{
		Round: f.c.enc.Round, Secret: who.Hidden, Actor: who.ID, Key: f.call.actionKey, RunBefore: run,
		ActionBefore: who.ActionUsed, BonusBefore: who.BonusActionUsed, ReactionBefore: who.ReactionUsed, DashedBefore: who.Dashed,
		AttacksBefore: who.AttacksMade, DisengagedBefore: who.Disengaged, SurgedBefore: who.ActionSurged,
		AttackKeyBefore: deref(who.ActionAttackKey), FlurryBefore: who.BonusAttacksLeft, ExtraUsed: f.call.useExtra,
	}
	return nil
}

// spend writes what the action costs: the extra action or the economy, and Action Surge's gift.
func (f *takeActionFlow) spend(ctx context.Context) error {
	if f.call.useExtra {
		// The extra action pays for it: the action of the turn stays.
		f.economy = rulesv1.ActionEconomy_ACTION_ECONOMY_FREE
		if err := f.c.q.SetCombatantExtraActionUsed(ctx, playdb.SetCombatantExtraActionUsedParams{ID: f.who.ID, ExtraActionUsed: true}); err != nil {
			return fmt.Errorf("spend the extra action: %w", err)
		}
	}
	after := f.afterEconomy()
	if f.call.actionKey == actionSurge {
		after.ActionUsed = false
		if err := f.surge(ctx); err != nil {
			return err
		}
	}
	if err := f.c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: f.who.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	return nil
}

// afterEconomy is the combatant with the economy the action spends marked used.
func (f *takeActionFlow) afterEconomy() playdb.Combatant {
	after := f.who
	switch f.economy {
	case rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION:
		after.BonusActionUsed = true
	case rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION:
		after.ReactionUsed = true
	case rulesv1.ActionEconomy_ACTION_ECONOMY_FREE, rulesv1.ActionEconomy_ACTION_ECONOMY_MOVEMENT:
		// costs no economy (Surto de ação)
	default:
		after.ActionUsed = true
	}
	return after
}

// surge is Action Surge: an additional action this turn, with the attacks of the Attack action.
func (f *takeActionFlow) surge(ctx context.Context) error {
	id := f.who.ID
	if err := f.c.q.SetCombatantActionSurged(ctx, playdb.SetCombatantActionSurgedParams{ID: id, ActionSurged: true}); err != nil {
		return fmt.Errorf("mark the action surge: %w", err)
	}
	if err := f.c.q.SetCombatantAttacksMade(ctx, playdb.SetCombatantAttacksMadeParams{ID: id, AttacksMade: 0}); err != nil {
		return fmt.Errorf("give the action back: %w", err)
	}
	// The new Attack action starts from nothing.
	if err := f.c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: id, BonusAttacksLeft: f.who.BonusAttacksLeft}); err != nil {
		return fmt.Errorf("forget the last attack: %w", err)
	}
	return nil
}

// perform does what the action does to the turn: a standard action's own, or the one a feature performs
// (Cunning Action's Dash, Step of the Wind's Disengage), and what the feature itself does.
func (f *takeActionFlow) perform(ctx context.Context) error {
	standard := f.call.actionKey
	if f.feature {
		var err error
		if standard, err = f.performFeature(ctx); err != nil {
			return err
		}
	}
	if err := f.performStandard(ctx, standard); err != nil {
		return err
	}
	// Step of the Wind (SRD 5.1, Monk): its Disengage or Dash also doubles the
	// jump distance for the rest of the turn.
	if f.feature && strings.HasPrefix(f.call.actionKey, combat.StepOfTheWindFeature+":") {
		return f.doubleJump(ctx)
	}
	return nil
}

// performStandard is the dispatch of the standard actions that change the turn: Dodge, Dash and
// Disengage. The others (Search, Ready, Use an Object) only spend the economy.
func (f *takeActionFlow) performStandard(ctx context.Context, standard string) error {
	switch standard {
	case actionDodge:
		return f.dodge(ctx)
	case actionDash:
		return markDashed(ctx, f.c.q, f.who.ID)
	case actionDisengage:
		return f.disengage(ctx)
	}
	return nil
}

// dodge is the Dodge action (or Patient Defense, which takes it): it lasts until the start of the
// dodger's next turn.
func (f *takeActionFlow) dodge(ctx context.Context) error {
	phase := "start_of_turn"
	st, err := f.s.addState(ctx, f.c, f.who, stateDodging, nil, &f.who.ID, &phase, nil, 0)
	if err != nil {
		return err
	}
	f.made.StateID = st.ID
	return nil
}

// disengage is the Disengage action: no opportunity attacks for the rest of the turn (slice 9.6b reads
// the flag).
func (f *takeActionFlow) disengage(ctx context.Context) error {
	if err := f.c.q.SetCombatantDisengaged(ctx, playdb.SetCombatantDisengagedParams{ID: f.who.ID, Disengaged: true}); err != nil {
		return fmt.Errorf("mark the disengage: %w", err)
	}
	return nil
}

// doubleJump doubles the jump distance for the rest of the turn.
func (f *takeActionFlow) doubleJump(ctx context.Context) error {
	if err := f.c.q.SetCombatantJumpDoubled(ctx, playdb.SetCombatantJumpDoubledParams{ID: f.who.ID, JumpDoubled: true}); err != nil {
		return fmt.Errorf("double the jump distance: %w", err)
	}
	f.made.JumpDoubled, f.made.JumpDoubledBefore = true, f.who.JumpDoubled
	return nil
}

// performFeature does what a feature action does by itself and answers the standard action it performs,
// when it is one.
func (f *takeActionFlow) performFeature(ctx context.Context) (string, error) {
	sheet, err := f.s.sheetOf(ctx, f.c.tx, f.m.CampaignID, f.who)
	if err != nil {
		return "", err
	}
	key := f.call.actionKey
	fi := slices.IndexFunc(sheet.FeatureActions, func(a link.FeatureAction) bool { return a.Key == key })
	if fi < 0 {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("action_key is not one of the character's feature actions"))
	}
	fa := sheet.FeatureActions[fi]
	standard := key
	if fa.Standard != "" {
		standard = fa.Standard
	}
	if standard == actionHide || standard == actionHelp {
		return "", errHideOrHelp()
	}
	if err := f.spendResource(ctx, fa); err != nil {
		return "", err
	}
	if err := f.performKind(ctx, sheet); err != nil {
		return "", err
	}
	return standard, f.s.beginFeatureState(ctx, f.c, f.v, f.who, key, sheet, f.call.frenzy, &f.made)
}

// spendResource spends one use of the feature's resource: only a player's character counts them, and a
// pool of points (Cura pelas mãos) is the master's.
func (f *takeActionFlow) spendResource(ctx context.Context, fa link.FeatureAction) error {
	if fa.Resource == "" || fa.Pool || f.who.Kind != kindPlayer {
		return nil
	}
	vitals, err := f.s.spendResource(ctx, f.c, f.who.CharacterID, fa.Resource, 1)
	if err != nil {
		return err
	}
	f.out.vitals = vitals
	f.made.Resource = fa.Resource
	return nil
}

// performKind is the dispatch of the feature actions with an effect of their own on the numbers; the
// others only spend the economy and a use.
func (f *takeActionFlow) performKind(ctx context.Context, sheet link.Sheet) error {
	switch f.call.actionKey {
	case slowFallAction:
		return f.slowFall(ctx, sheet)
	case flurryOfBlows:
		return f.flurry(ctx, sheet)
	case secondWind:
		return f.secondWind(ctx, sheet)
	}
	return nil
}

// slowFall is the monk's Slow Fall: it cuts the damage of the last fall.
func (f *takeActionFlow) slowFall(ctx context.Context, sheet link.Sheet) error {
	cuts, taken, err := f.s.slowFall(ctx, f.c, f.who, sheet.MonkLevel)
	if err != nil {
		return err
	}
	f.made.FallCuts, f.made.Amount = cuts, taken
	return nil
}

// flurry is Flurry of Blows: it comes right after the Attack action; its two unarmed strikes are rolled
// as attacks (the master has the last word).
func (f *takeActionFlow) flurry(ctx context.Context, sheet link.Sheet) error {
	who := f.who
	last, hasLast := lastAttack(sheet, who)
	if !f.v.master && (!who.ActionUsed || who.AttacksMade == 0 || !hasLast || last.Spell) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ATTACK_ACTION_FIRST, "Flurry of Blows comes after the Attack action")
	}
	if err := f.c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: who.ID, ActionAttackKey: who.ActionAttackKey, BonusAttacksLeft: flurryStrikes}); err != nil {
		return fmt.Errorf("give the flurry strikes: %w", err)
	}
	return nil
}

// secondWind is Retomar o fôlego: it heals the fighter and keeps the vitals for the answer.
func (f *takeActionFlow) secondWind(ctx context.Context, sheet link.Sheet) error {
	vit, err := f.s.secondWind(ctx, f.c, f.m, f.v, f.who, sheet.FighterLevel, f.call.in, f.call.rolled, &f.made)
	if err != nil {
		return err
	}
	if vit != nil {
		f.out.vitals = vit
	}
	return nil
}

// touch marks the encounter as changed and names the character the event belongs to.
func (f *takeActionFlow) touch(ctx context.Context) error {
	if err := touchEncounter(ctx, f.c); err != nil {
		return err
	}
	f.c.characterID = &f.who.CharacterID
	return nil
}
