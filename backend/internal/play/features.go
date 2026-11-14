package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A feature that declares a use (UseFeature, docs/architecture/play/feature-state.md "Using a feature"): a grant_action of a
// table feature with a limit per turn, round or combat, targets, a choice or an effect on use. UseFeature reads the request
// and runs the transaction on a featureFlow, which checks in the order the proto documents (the combat and the turn, the
// feature, the economy, the uses, the targets, the choice) and writes in one transaction with the idempotency key: the
// state, the economy, the resource, what the use applies, the event and, through it, the log and the undo.

// maxChoiceKey is the longest key of a choice a request may carry (the rules cap a content key at 40).
const maxChoiceKey = 64

// useFeatureCall is what UseFeature reads from its request, checked.
type useFeatureCall struct {
	key, encounterID, combatantID string
	actionKey, choice             string
	targets                       []string
	useExtra                      bool
}

// readUseFeatureCall reads the request, in the order the answers are given: the ids, the key, the targets, the choice.
func readUseFeatureCall(req *playv1.UseFeatureRequest) (useFeatureCall, error) {
	var call useFeatureCall
	var err error
	if call.key, call.encounterID, err = parseCall(req.GetIdempotencyKey(), req.GetEncounterId()); err != nil {
		return call, err
	}
	if call.combatantID, err = parseCombatID(req.GetCombatantId(), "combatant"); err != nil {
		return call, err
	}
	if call.actionKey = req.GetActionKey(); call.actionKey == "" || len(call.actionKey) > maxActionKey {
		return call, connect.NewError(connect.CodeInvalidArgument, errors.New("action_key is a feature's key"))
	}
	if call.choice = req.GetChoice(); len(call.choice) > maxChoiceKey {
		return call, connect.NewError(connect.CodeInvalidArgument, errors.New("choice is one of the keys the feature offers"))
	}
	if len(req.GetTargetIds()) > maxCombatants {
		return call, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target_ids has at most %d entries", maxCombatants))
	}
	for _, raw := range req.GetTargetIds() {
		id, err := parseCombatID(raw, "target")
		if err != nil {
			return call, err
		}
		if slices.Contains(call.targets, id) {
			return call, connect.NewError(connect.CodeInvalidArgument, errors.New("a target appears once in target_ids"))
		}
		call.targets = append(call.targets, id)
	}
	call.useExtra = req.GetUseExtraAction()
	return call, nil
}

// maxActionKey is the longest action key a request may carry.
const maxActionKey = 200

// useFeatureOut is what the transaction of a use leaves for its answer.
type useFeatureOut struct {
	made actionEvent
	// vitals are the vitals of the characters whose resource or hit points changed, to tell the streams.
	vitals []*playv1.CharacterVitals
}

// UseFeature implements playv1connect.FeatureServiceHandler.
func (s *Service) UseFeature(
	ctx context.Context,
	req *connect.Request[playv1.UseFeatureRequest],
) (*connect.Response[playv1.UseFeatureResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	call, err := readUseFeatureCall(req.Msg)
	if err != nil {
		return nil, err
	}
	out := &useFeatureOut{}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventFeatureUsed, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		return newFeatureFlow(s, c, m, call, out).run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "use a feature", err)
	}
	return s.useFeatureResponse(ctx, m, res, out)
}

// useFeatureResponse answers from what the transaction left: the encounter as the caller sees it, and what the use healed and
// gave (the caller is the master or the user's player: both read the numbers).
func (s *Service) useFeatureResponse(ctx context.Context, m authz.Membership, res combatResult, out *useFeatureOut) (*connect.Response[playv1.UseFeatureResponse], error) {
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the use of the feature", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishVitalsOf(ctx, m.CampaignID, out.vitals)
	})
	if err != nil {
		return nil, err
	}
	resp := &playv1.UseFeatureResponse{Encounter: enc}
	if ev.Feature != nil {
		resp.Healed, resp.TempHp = ev.Feature.Healed, ev.Feature.TempHP
	}
	return connect.NewResponse(resp), nil
}

// featureFlow is one run of the use's transaction: what its steps read, and the next ones use.
type featureFlow struct {
	s    *Service
	c    *combatTx
	m    authz.Membership
	call useFeatureCall
	out  *useFeatureOut

	v     combatViewer
	cs    []playdb.Combatant
	who   playdb.Combatant
	sheet link.Sheet
	fa    link.FeatureAction
	row   *rulesv1.ActionOption
	scene featureScene
	// targets are who the use is aimed at, resolved: the user alone when the call names none.
	targets []playdb.Combatant
	choice  *link.FeatureChoice
	hits    []castHit
	made    actionEvent
	fe      *featureUseEvent
}

func newFeatureFlow(s *Service, c *combatTx, m authz.Membership, call useFeatureCall, out *useFeatureOut) *featureFlow {
	return &featureFlow{s: s, c: c, m: m, call: call, out: out, fe: &featureUseEvent{}}
}

// run is the transaction: the checks, then what the use costs and does. It answers the event the write stores.
func (f *featureFlow) run(ctx context.Context) (any, error) {
	f.out.vitals = nil
	steps := []func(context.Context) error{
		f.locate, f.findFeature, f.gate, f.mayUse, f.aim, f.pick, f.begin, f.spend, f.apply, f.remember,
	}
	for _, step := range steps {
		if err := step(ctx); err != nil {
			return nil, err
		}
	}
	if err := touch(ctx, f.c); err != nil {
		return nil, err
	}
	f.c.characterID = &f.who.CharacterID
	f.out.made = f.made
	return f.made, nil
}

// locate finds the combatant that acts, as the caller sees the fog, and checks the caller may act for it: a player's
// character is used by its player or the master; an NPC and a creature have no features of this kind.
func (f *featureFlow) locate(ctx context.Context) error {
	cs, err := listCombatants(ctx, f.c)
	if err != nil {
		return err
	}
	f.cs = cs
	f.v = f.c.viewer(f.m, cs) // the fog: a combatant the player does not see is not found
	if f.who, err = findCombatant(cs, f.call.combatantID, f.v); err != nil {
		return err
	}
	if err := f.v.mayAct(f.who); err != nil {
		return err
	}
	if f.who.Kind != kindPlayer {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("this is for a player's character; the master adjusts an NPC by hand"))
	}
	return nil
}

// findFeature finds the feature among the combatant's, and checks it declares a use: one that does not is taken with
// TakeAction.
func (f *featureFlow) findFeature(ctx context.Context) error {
	opts, err := f.s.optionsOf(ctx, f.c.tx, f.m.CampaignID, f.who)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(opts.GetFeatureActions(), func(a *rulesv1.ActionOption) bool { return a.GetAction().GetKey() == f.call.actionKey })
	if i < 0 {
		return connect.NewError(connect.CodeNotFound, errors.New("the combatant does not have this feature"))
	}
	f.row = opts.GetFeatureActions()[i]
	if !f.row.GetViaUseFeature() {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("this feature is taken with TakeAction"))
	}
	if f.sheet, err = f.s.sheetOf(ctx, f.c.tx, f.m.CampaignID, f.who); err != nil {
		return err
	}
	var ok bool
	if f.fa, ok = featureOfKey(f.sheet, f.call.actionKey); !ok {
		return connect.NewError(connect.CodeNotFound, errors.New("the combatant does not have this feature"))
	}
	return nil
}

// gate checks the combat is running, the combatant may use the feature now and nothing holds it (featureStopNow, which the
// options ask too).
func (f *featureFlow) gate(ctx context.Context) error {
	stop, err := f.s.featureStopNow(ctx, f.c, f.who, isReaction(f.row))
	if err != nil {
		return err
	}
	if stop != nil {
		return stop.err
	}
	return nil
}

// featureGateError is the refusal of a gate code of a feature that spends the reaction: gateError's, and the reaction an
// effect took away.
func featureGateError(code rulesv1.DisabledReasonCode) error {
	switch code {
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_UNSPECIFIED:
		return nil
	case rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_USED:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the combatant cannot react now")
	}
	return gateError(code)
}

// mayUse checks the economy the feature spends, its resource and the uses of its scope, in the order the options say it. The
// master has the last word on the economy only: a spent resource or scope stops him too.
func (f *featureFlow) mayUse(ctx context.Context) error {
	if f.call.useExtra {
		// The extra action pays a standard action: a feature is no standard action (as TakeAction says).
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_EXTRA_ACTION_UNAVAILABLE, "the extra action takes a standard action")
	}
	var err error
	if f.scene, err = f.s.featureSceneOf(ctx, f.c.tx, f.m.CampaignID, f.c.enc, f.cs, f.who, f.v); err != nil {
		return err
	}
	// The resource is read in this transaction: the options of the rules read it too, but the row of the use is the
	// one this transaction would spend from.
	return featureRefusal(f.scene.reason(f.fa, f.row, f.v.master), f.v.master)
}

// featureRefusal is the refusal of a reason the checks found, nil when there is none. It fails closed: a disabled row whose
// code no case maps is refused all the same, never let through (the master's economy override is the one exception, and it is
// already out of the reason: featureScene.reason skips it). NO_VALID_TARGET is left to aim, which names the targets refused.
func featureRefusal(reason *rulesv1.DisabledReason, master bool) error {
	code := reason.GetCode()
	switch {
	case reason == nil, code == rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_VALID_TARGET:
		return nil
	case master && masterMayOverride(code):
		return nil
	case code == rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_SLOT:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "no spell slot")
	}
	if err := featureError(reason, master); err != nil {
		return err
	}
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("the feature cannot be used now"))
}

// aim resolves the targets: the ones the call names, each of which the caller sees and the feature may be aimed at, or the
// user alone. A refusal is FEATURE_TARGET; the ids it lists are the ones that cannot be picked, a creature the caller does
// not see among them with the answer of one that does not exist.
func (f *featureFlow) aim(_ context.Context) error {
	t := targetRules(*f.fa.Use)
	ids := f.call.targets
	if !t.Allows(len(ids)) || (t.Who == rules.FeatureWhoSelf && len(ids) == 1 && ids[0] != f.who.ID) {
		return errFeatureTarget(nil)
	}
	if len(ids) == 0 {
		f.targets = []playdb.Combatant{f.who}
		if t.Aimed() && t.Who != rules.FeatureWhoSelf {
			return errFeatureTarget(nil) // a use that picks creatures picks at least one
		}
		return nil
	}
	list := f.scene.candidates(*f.fa.Use)
	var refused []string
	for _, id := range ids {
		i := slices.IndexFunc(list, func(c *playv1.ResourceTarget) bool { return c.GetTarget().GetCombatantId() == id })
		if i < 0 || list[i].GetDisabledReasonPt() != "" {
			refused = append(refused, id)
			continue
		}
		c, _ := combatantByID(f.cs, id)
		f.targets = append(f.targets, c)
	}
	if len(refused) > 0 {
		return errFeatureTarget(refused)
	}
	return nil
}

// errFeatureTarget is the refusal of targets that cannot be picked: the same answer for one the caller does not see, one
// that is not in the combat and one the feature cannot reach (nothing says which).
func errFeatureTarget(ids []string) error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_FEATURE_TARGET, "the targets cannot be picked",
		func(b *playv1.EncounterBlocked) { b.CombatantIds = ids })
}

// pick checks the choice: one of the feature's keys, none when it offers none.
func (f *featureFlow) pick(_ context.Context) error {
	choices := f.fa.Use.Choices
	if len(choices) == 0 && f.call.choice == "" {
		return nil
	}
	i := slices.IndexFunc(choices, func(c link.FeatureChoice) bool { return c.Key == f.call.choice })
	if i < 0 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_FEATURE_CHOICE, "the choice is not one the feature offers")
	}
	f.choice = &choices[i]
	return nil
}

// begin starts the event, with the state of the turn the undo puts back.
func (f *featureFlow) begin(ctx context.Context) error {
	run, err := breakRun(ctx, f.c, f.who)
	if err != nil {
		return err
	}
	who := f.who
	f.made = actionEvent{
		Round: f.c.enc.Round, Secret: who.Hidden, Actor: who.ID, Key: f.call.actionKey, RunBefore: run,
		ActionBefore: who.ActionUsed, BonusBefore: who.BonusActionUsed, ReactionBefore: who.ReactionUsed, DashedBefore: who.Dashed,
		Feature: f.fe,
	}
	f.fe.Scope, f.fe.Choice = f.fa.Use.Scope, f.call.choice
	for _, t := range f.targets {
		f.made.Secret = f.made.Secret || t.Hidden
	}
	if len(f.call.targets) == 1 && f.call.targets[0] != who.ID {
		f.made.Target = f.call.targets[0]
	}
	return nil
}

// spend writes what the use costs: the economy and the use of the feature's resource.
func (f *featureFlow) spend(ctx context.Context) error {
	economy := featureEconomy(f.row)
	if err := spendFeatureEconomy(ctx, f.c, spentEconomy(f.who, economy), f.call.actionKey, economy); err != nil {
		return err
	}
	fa := f.fa
	if fa.Resource == "" || fa.Pool {
		return nil
	}
	n := clamp32(asRules(fa).UnitCost(), 0, math.MaxInt32)
	vitals, err := f.s.spendResource(ctx, f.c, f.who.CharacterID, fa.Resource, n)
	if err != nil {
		return err
	}
	f.out.vitals = append(f.out.vitals, vitals)
	f.made.Resource, f.made.PointsSpent = fa.Resource, n
	return nil
}

// apply gives each target what the use applies: the hit points healed (the dice rolled by the server, once, for every
// target), the temporary hit points, and the lasting effect. A target the use applies to nothing is still in the event.
func (f *featureFlow) apply(ctx context.Context) error {
	u := f.fa.Use
	f.hits = make([]castHit, len(f.targets))
	for i, t := range f.targets {
		f.hits[i].Target = t.ID
	}
	if u.Heal != nil || u.TempHP > 0 {
		if err := f.s.refuseTheDead(ctx, f.c, f.targets); err != nil {
			return err
		}
	}
	amount, err := f.rollHeal()
	if err != nil {
		return err
	}
	f.fe.Targets = nil
	for i, t := range f.targets {
		got, err := f.applyTo(ctx, i, t, amount)
		if err != nil {
			return err
		}
		f.fe.Healed += got.Healed
		f.fe.TempHP += got.TempHP
		f.fe.Targets = append(f.fe.Targets, got)
	}
	if len(f.call.targets) == 0 {
		f.fe.Targets = nil // the user alone: the event names no target
	}
	if err := f.effect(ctx); err != nil {
		return err
	}
	f.made.Hits = f.hits
	return nil
}

// rollHeal rolls the heal once for every target and writes the dice in the event; 0 for a use that heals nothing.
func (f *featureFlow) rollHeal() (int32, error) {
	u := f.fa.Use
	if u.Heal == nil {
		return 0, nil
	}
	roll, err := dice.Roll(f.s.roller, dice.Expr{Count: u.Heal.Count, Sides: u.Heal.Sides, Modifier: u.Heal.Bonus})
	if err != nil {
		return 0, fmt.Errorf("roll the heal: %w", err)
	}
	amount := clamp32(max(roll.Total, 0), 0, math.MaxInt32)
	f.fe.Faces, f.fe.Total = faces32(roll.Faces), amount
	return amount, nil
}

// applyTo gives one target the heal and the temporary hit points, and says what it got.
func (f *featureFlow) applyTo(ctx context.Context, i int, t playdb.Combatant, amount int32) (featureTargetUse, error) {
	u := f.fa.Use
	got := featureTargetUse{ID: t.ID}
	if u.Heal != nil {
		if err := f.heal(ctx, i, t, amount, &got); err != nil {
			return got, err
		}
	}
	if u.TempHP > 0 {
		if err := f.temp(ctx, i, t, u.TempHP, &got); err != nil {
			return got, err
		}
	}
	return got, nil
}

// heal heals one target, keeping the hit points before for the undo.
func (f *featureFlow) heal(ctx context.Context, i int, t playdb.Combatant, amount int32, got *featureTargetUse) error {
	hit, vit, err := f.s.healCombatant(ctx, f.c, t, amount)
	if err != nil {
		return err
	}
	got.Healed = hit.Amount
	f.keep(i, hit.Before, hit.DeathBefore, vit)
	return nil
}

// temp gives one target temporary hit points: they do not stack, the larger number is kept.
func (f *featureFlow) temp(ctx context.Context, i int, t playdb.Combatant, amount int, got *featureTargetUse) error {
	fx, err := f.s.readFxTarget(ctx, f.c, t)
	if err != nil {
		return err
	}
	var h castHit
	vit, err := f.s.giveTempHP(ctx, f.c, fx, amount, &h)
	if err != nil {
		return err
	}
	got.TempHP = conv32(h.Healed)
	f.keep(i, h.Restore, h.DeathBefore, vit)
	return nil
}

// keep remembers what a target's hit points were before the use touched them (the first change wins) and the vitals to tell.
func (f *featureFlow) keep(i int, before *hpState, death *deathState, vit *playv1.CharacterVitals) {
	if f.hits[i].Restore == nil {
		f.hits[i].Restore, f.hits[i].DeathBefore = before, death
	}
	if vit != nil {
		f.out.vitals = append(f.out.vitals, vit)
	}
}

// effect puts the lasting effect of the feature on its targets for the rounds the feature says: it has no concentration,
// its source is the feature, and the clock counts rounds from the user's turn. The same sequence as the effect of a spell
// (the modifiers worked out for the targets, the effect rows, the temporary hit points of a temp_hp modifier), then the
// targets are worked out again (refreshCombatants, inside addEffects).
func (f *featureFlow) effect(ctx context.Context) error {
	u := f.fa.Use
	if !u.HasEffect {
		return nil
	}
	content, err := f.s.contentOf(ctx, f.c)
	if err != nil {
		return err
	}
	def, ok := content.FeatureUseEffect(f.call.actionKey)
	if !ok {
		return fmt.Errorf("the content has no effect for the feature %q", f.call.actionKey)
	}
	opts, err := f.s.castOptsOf(ctx, f.c, def, "", f.who.CharacterID)
	if err != nil {
		return err
	}
	groups, mods, err := f.s.spellEffectGroups(ctx, f.c, def, f.targets, opts)
	if err != nil {
		return err
	}
	dur := durationSpec{Kind: rules.EffectDurationRounds, Rounds: clamp32(u.EffectRounds, 1, math.MaxInt32), Anchor: f.who.ID}
	cs, err := listCombatants(ctx, f.c)
	if err != nil {
		return err
	}
	caster := f.who
	var rows []playdb.CombatantState
	for i, g := range groups {
		made, err := f.s.addEffects(ctx, f.c, cs, effectSpec{
			key: f.call.actionKey, sourceKind: "feature", def: def, caster: &caster, targets: g, dur: dur, modifiers: mods[i],
		})
		if err != nil {
			return err
		}
		rows = append(rows, made...)
	}
	f.made.Lasting = &lastingEvent{Key: f.call.actionKey, Change: "added"}
	for _, r := range rows {
		f.made.Lasting.Targets = append(f.made.Lasting.Targets, r.CombatantID)
		f.made.Lasting.Effects = append(f.made.Lasting.Effects, r.ID)
	}
	f.fe.Effect = len(rows) > 0
	// A second use does not replace the first (dropSameSpell is for spells), so it inherits no claim: the row that raised the
	// pool owns it, and the later row claims only a pool it raises itself.
	scratch := actionEvent{Hits: f.hits}
	if err := f.s.giveEffectTempHP(ctx, f.c, cs, rows, f.targets, &scratch, nil); err != nil {
		return err
	}
	f.hits = scratch.Hits
	return nil
}

// remember writes the use into the feature state: one more use in the scope, under the mark that holds now, with the choice.
// A feature with no limit in a scope has nothing to count.
func (f *featureFlow) remember(ctx context.Context) error {
	scope := f.fa.Use.Scope
	if scope == "" {
		return nil
	}
	key, now := useStateKey(f.call.actionKey), f.scene.now
	state := readFeatureState(f.who.FeatureState)
	var before *featureEntry
	n := int32(1)
	if e, ok := state.live(key, now); ok {
		kept := e
		before = &kept
		if e.S == scope {
			n = e.N + 1
		}
	}
	f.made.FeatureBefore = map[string]*featureEntry{key: before}
	state.set(key, featureEntry{S: scope, At: f.scene.mark(scope), N: n, C: f.call.choice})
	if err := saveFeatureState(ctx, f.c, f.who.ID, state, now); err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return nil
}

// spentEconomy is the combatant with the economy the action spends marked used: the free and the movement cost none.
func spentEconomy(who playdb.Combatant, economy rulesv1.ActionEconomy) playdb.Combatant {
	switch economy {
	case rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION:
		who.BonusActionUsed = true
	case rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION:
		who.ReactionUsed = true
	case rulesv1.ActionEconomy_ACTION_ECONOMY_FREE, rulesv1.ActionEconomy_ACTION_ECONOMY_MOVEMENT:
		// costs no economy
	default:
		who.ActionUsed = true
	}
	return who
}
