package play

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The table's content at play (MR-025): the actions that spend several points, heal or give temporary hit points, the
// pools that recharge on initiative, and the owner's on_event hooks. The rules say what each one gives (rules.Action,
// rules.EventHook, combat.ApplyHeal and combat.ApplyTempHP: the same maths as the SRD spells); this file reads the
// points left in the transaction of the use, applies the result through the vitals and records it, with what an undo
// needs, in the event the action, the death save or the damage already writes.

// asRules is the rules' view of a feature action: its resource, its cost range and what it gives.
func asRules(fa link.FeatureAction) rules.Action {
	r := rules.ActionResult{TempHP: fa.TempHP, TempHPPerPoint: fa.TempHPPerPoint, HealPerPoint: rules.DiceFormula{Count: fa.HealPerPoint.Count, Sides: fa.HealPerPoint.Sides, Bonus: fa.HealPerPoint.Bonus}}
	if fa.Heal != nil {
		r.Heal = &rules.DiceFormula{Count: fa.Heal.Count, Sides: fa.Heal.Sides, Bonus: fa.Heal.Bonus}
	}
	return rules.Action{Key: fa.Key, Resource: fa.Resource, Cost: fa.Cost, CostMax: fa.CostMax, GivesSlot: fa.GivesSlot, SpendsSlot: fa.SpendsSlot, Result: r}
}

// pickSpend works out how many uses the use spends, from the number the player picked and the uses left, read in the
// transaction of the use: a number outside the range is refused with nothing spent.
func (f *takeActionFlow) pickSpend(ctx context.Context, fa link.FeatureAction) (int, error) {
	now, err := f.s.vitals.GetVitalsTx(ctx, f.c.tx, f.c.session.CampaignID, f.who.CharacterID)
	if err != nil {
		return 0, err
	}
	left, recharge := 0, rulesv1.Recharge_RECHARGE_UNSPECIFIED
	if i := slices.IndexFunc(now.GetResources(), func(r *playv1.ResourceUsage) bool { return r.GetKey() == fa.Resource }); i >= 0 {
		r := now.GetResources()[i]
		left, recharge = int(r.GetTotal()-r.GetUsed()), r.GetRecharge()
	}
	act := asRules(fa)
	chosen := int(f.call.spend)
	if !act.ChoosesSpend() {
		chosen = 0 // a fixed cost takes no pick
	}
	n, err := act.Spend(chosen, left)
	switch {
	case err == nil:
		return n, nil
	case left < act.UnitCost():
		return 0, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "no uses left",
			func(b *playv1.EncounterBlocked) { b.Recharge = recharge })
	}
	return 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("spend must be %d to %d: %w", act.UnitCost(), min(act.CostMax, left), err))
}

// givesResult says the feature action heals or gives temporary hit points.
func givesResult(fa link.FeatureAction) bool {
	return fa.Heal != nil || fa.TempHP > 0 || fa.TempHPPerPoint > 0
}

// healTarget finds who gets the heal and the temporary hit points: the actor, or one ally the player names, another
// player's character the caller sees. A dead target is refused.
func (f *takeActionFlow) healTarget(ctx context.Context) (playdb.Combatant, error) {
	target := f.who
	if id := f.call.healTarget; id != "" && id != f.who.ID {
		cs, err := listCombatants(ctx, f.c)
		if err != nil {
			return target, err
		}
		if target, err = findCombatant(cs, id, f.v); err != nil {
			return target, err
		}
		if target.Kind != kindPlayer {
			return target, connect.NewError(connect.CodeInvalidArgument, errors.New("heal_target_combatant_id must be the actor or an ally, another player's character"))
		}
	}
	return target, f.s.refuseTheDead(ctx, f.c, []playdb.Combatant{target})
}

// grantResult gives the heal and the temporary hit points of a feature action that spent spent points, to the actor or
// to the ally chosen. The heal is rolled like a damage (RN-18); it fills the event with the roll and what changed.
func (f *takeActionFlow) grantResult(ctx context.Context, fa link.FeatureAction, spent int) error {
	out := asRules(fa).Outcome(spent)
	if out.Heal != nil && !f.call.rolled {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or typed_sum for this action"))
	}
	if out.Heal != nil && !f.v.master {
		if err := f.s.mustRollThisWay(ctx, f.c.tx, f.m, f.call.in); err != nil {
			return err
		}
	}
	target, err := f.healTarget(ctx)
	if err != nil {
		return err
	}
	ev := &f.made
	if target.ID != f.who.ID {
		ev.Target = target.ID
	}
	var vitals []*playv1.CharacterVitals
	if out.Heal != nil {
		v, err := f.heal(ctx, target, *out.Heal)
		if err != nil {
			return err
		}
		vitals = append(vitals, v)
	}
	if out.TempHP > 0 {
		v, err := f.temp(ctx, target, out.TempHP)
		if err != nil {
			return err
		}
		vitals = append(vitals, v)
	}
	f.keepVitals(target, vitals)
	return nil
}

// keepVitals keeps the vitals of the target for the streams: the actor's go in the answer's own slot, an ally's beside.
func (f *takeActionFlow) keepVitals(target playdb.Combatant, vitals []*playv1.CharacterVitals) {
	for _, v := range vitals {
		switch {
		case v == nil:
		case target.ID == f.who.ID:
			f.out.vitals = v
		default:
			f.out.extra = append(f.out.extra, v)
		}
	}
}

// heal rolls the dice and heals the target: the dead are refused before, the maximum caps it, and standing up from 0
// follows the vitals' own rules.
func (f *takeActionFlow) heal(ctx context.Context, target playdb.Combatant, d rules.DiceFormula) (*playv1.CharacterVitals, error) {
	expr := dice.Expr{Count: d.Count, Sides: d.Sides, Modifier: d.Bonus}
	var roll dice.Result
	var err error
	if f.call.in.inApp {
		roll, err = dice.Roll(f.s.roller, expr)
	} else {
		roll, err = dice.Physical(expr, f.call.in.typed)
	}
	if err != nil {
		if f.call.in.inApp {
			return nil, fmt.Errorf("roll the heal: %w", err)
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("typed_sum must be %d to %d", d.Count, d.Count*d.Sides))
	}
	hit, vit, err := f.s.healCombatant(ctx, f.c, target, clamp32(max(roll.Total, 0), 0, math.MaxInt32))
	if err != nil {
		return nil, err
	}
	ev := &f.made
	ev.Heal, ev.DiceCount, ev.DiceSides, ev.Faces = true, clamp32(d.Count, 0, 100), clamp32(d.Sides, 0, 100), faces32(roll.Faces)
	ev.Modifier, ev.Total, ev.Physical = clamp32(d.Bonus, 0, 1000), clamp32(roll.Total, 0, math.MaxInt32), roll.Physical
	ev.Amount, ev.Before, ev.After, ev.DeathBefore = hit.Amount, hit.Before, hit.After, hit.DeathBefore
	return vit, nil
}

// temp gives the target temporary hit points: they do not stack, so it keeps the larger number.
func (f *takeActionFlow) temp(ctx context.Context, target playdb.Combatant, amount int) (*playv1.CharacterVitals, error) {
	t, err := f.s.readFxTarget(ctx, f.c, target)
	if err != nil {
		return nil, err
	}
	var h castHit
	v, err := f.s.giveTempHP(ctx, f.c, t, amount, &h)
	if err != nil {
		return nil, err
	}
	ev := &f.made
	ev.TempTried, ev.TempGained = true, conv32(h.Healed)
	if ev.Before == nil { // a heal before it has the hit points as they were
		ev.Before, ev.DeathBefore = h.Restore, h.DeathBefore
	}
	return v, nil
}

// conv32 is the number a castHit's pointer holds, 0 for none.
func conv32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// hookEvent is an on_event hook that ran, as an event keeps it: what it gave, and the uses it spent and restored.
type hookEvent struct {
	Source    string `json:"source,omitempty"`
	Healed    int32  `json:"healed,omitempty"`
	Temp      int32  `json:"temp,omitempty"`
	Spent     string `json:"spent,omitempty"`
	SpentN    int32  `json:"spent_n,omitempty"`
	Restored  string `json:"restored,omitempty"`
	RestoredN int32  `json:"restored_n,omitempty"`
}

// hookProto is the hook as the log and the answers tell it, with the name of the feature that has it ("" where the line
// cannot name it: the key goes along); nil for none. An event keeps the key, never the name.
func hookProto(h *hookEvent, name string) *playv1.HookResult {
	if h == nil {
		return nil
	}
	return &playv1.HookResult{
		Source: h.Source, SourceNamePt: name, Healed: h.Healed, TempHpGained: h.Temp,
		SpentResource: h.Spent, Spent: h.SpentN, RestoredResource: h.Restored, Restored: h.RestoredN,
	}
}

// pickHook finds the first hook of the event that the character can pay, in the order of its features: one hook
// answers an event, whichever it is.
func pickHook(hooks []link.Hook, event string, now *playv1.CharacterVitals) (link.Hook, bool) {
	for _, h := range hooks {
		if h.Event != event {
			continue
		}
		if h.Spends == "" || leftOf(now, h.Spends) >= clamp32(h.SpendCount, 0, math.MaxInt32) {
			return h, true
		}
	}
	return link.Hook{}, false
}

// leftOf is how many uses of a resource the character has left, 0 for a resource it does not have.
func leftOf(now *playv1.CharacterVitals, key string) int32 {
	for _, r := range now.GetResources() {
		if r.GetKey() == key {
			return r.GetTotal() - r.GetUsed()
		}
	}
	return 0
}

// hookPlan is what a hook does to a character: the hit points and temporary hit points after, the uses spent after, and
// the event that tells it.
type hookPlan struct {
	hp, temp  int32
	resources []*playv1.ResourceUsed
	ev        hookEvent
}

// request is the change of the vitals the plan makes.
func (p hookPlan) request() *playv1.AdjustCharacterVitalsRequest {
	hp, temp := p.hp, p.temp
	return &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp, HitPointsTemporary: &temp, ResourcesUsed: p.resources}
}

// planHook works out what the hook gives a character with hp and temp hit points now (the vitals of now carry the
// maximum and the uses): the heal (its dice rolled by the app, plus its share of the maximum), the temporary hit points
// (the larger number is kept) and the uses it spends and restores (never more than are spent).
func (s *Service) planHook(h link.Hook, now *playv1.CharacterVitals, hp, temp int32) (hookPlan, error) {
	p := hookPlan{hp: hp, temp: temp, ev: hookEvent{Source: h.Source}}
	used := map[string]int32{}
	usedNow := func(key string) int32 {
		if n, ok := used[key]; ok {
			return n
		}
		return usesSpentOf(now, key)
	}
	if h.Spends != "" {
		n := clamp32(h.SpendCount, 0, math.MaxInt32)
		used[h.Spends] = usedNow(h.Spends) + n
		p.ev.Spent, p.ev.SpentN = h.Spends, n
	}
	amount := 0
	if h.Heal != nil {
		roll, err := dice.Roll(s.roller, dice.Expr{Count: h.Heal.Count, Sides: h.Heal.Sides, Modifier: h.Heal.Bonus})
		if err != nil {
			return p, fmt.Errorf("roll the hook's heal: %w", err)
		}
		amount += roll.Total
	}
	if h.HealPercentMax > 0 {
		amount += rules.PercentOfMax(h.HealPercentMax, int(now.GetHitPointsMax()))
	}
	if r := combat.ApplyHeal(int(hp), int(now.GetHitPointsMax()), max(amount, 0)); r.Healed > 0 {
		p.hp, p.ev.Healed = clamp32(r.HP, 0, math.MaxInt32), clamp32(r.Healed, 0, math.MaxInt32)
	}
	if h.TempHP > 0 {
		g := combat.ApplyTempHP(int(temp), h.TempHP)
		p.temp, p.ev.Temp = clamp32(g.TempHP, 0, maxTempHitPoints), clamp32(g.Gained, 0, math.MaxInt32)
	}
	if h.Restore != "" && h.RestoreCount > 0 {
		cur := usedNow(h.Restore)
		if back := clamp32(rules.RestoreUses(int(cur), h.RestoreCount), 0, math.MaxInt32); back > 0 {
			used[h.Restore] = cur - back
			p.ev.Restored, p.ev.RestoredN = h.Restore, back
		}
	}
	for key, n := range used {
		p.resources = append(p.resources, &playv1.ResourceUsed{Key: key, Used: n})
	}
	slices.SortFunc(p.resources, func(a, b *playv1.ResourceUsed) int { return cmp.Compare(a.GetKey(), b.GetKey()) })
	return p, nil
}

// hookBack gives back what a hook that ran spent and restored: the uses go back to where they were (the hit points
// are put back by the event's Before).
func (u *undoer) hookBack(ctx context.Context, who playdb.Combatant, h *hookEvent) error {
	if h == nil {
		return nil
	}
	if h.Spent != "" && h.SpentN != 0 {
		v, err := u.s.spendResource(ctx, u.c, who.CharacterID, h.Spent, -h.SpentN)
		if err != nil {
			return err
		}
		u.keep(v)
	}
	if h.Restored != "" && h.RestoredN != 0 {
		v, err := u.s.spendResource(ctx, u.c, who.CharacterID, h.Restored, h.RestoredN)
		if err != nil {
			return err
		}
		u.keep(v)
	}
	return nil
}

// failedDeathSave says the outcome of a death save is a failure the hook answers (a natural 1 included).
func failedDeathSave(outcome string) bool {
	return outcome == deathFailure || outcome == deathCritical
}

// deathSaveHook runs the character's "death_save_failed" hook on a failed save, before the failure is added to the
// count: it says whether the hook stood the character up (healed above 0), in which case the counts start over and
// the failure is not added. A hook that cannot be paid does nothing.
func (s *Service) deathSaveHook(ctx context.Context, c *combatTx, who playdb.Combatant, sheet link.Sheet, out *deathSaveOutcome) (bool, error) {
	if len(sheet.Hooks) == 0 {
		return false, nil
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, who.CharacterID)
	if err != nil {
		return false, err
	}
	h, ok := pickHook(sheet.Hooks, rules.EventDeathSaveFailed, now)
	if !ok {
		return false, nil
	}
	plan, err := s.planHook(h, now, now.GetHitPointsCurrent(), now.GetHitPointsTemporary())
	if err != nil {
		return false, err
	}
	before, after, err := s.vitalsOf(ctx, c, who.CharacterID, plan.request())
	if err != nil {
		return false, err
	}
	out.vitals, out.hookName = after, h.SourceNamePT
	out.made.Hook = &plan.ev
	out.made.Before, out.made.After = new(hpStateOf(before)), new(hpStateOf(after))
	return plan.ev.Healed > 0 && after.GetHitPointsCurrent() > 0, nil
}

// dropToZeroHook runs the character's "drop_to_zero" hook on a damage that took it from above 0 to 0 hit points (dmg is
// combat.ApplyDamage on the vitals now): the combat's damage and a trap's both land here, the one copy of the rule. It
// returns the plan (the hit points and temporary hit points the character keeps and the uses spent), or false when the
// character has no hook it can pay, the hook does not heal, or the damage is one no hook answers: it did not drop the
// character, it was massive (the left-over damage reaches the maximum: instant death is not prevented), or it fell on a
// beast form. A dead or dying character is never raised: damage at 0 hit points is not a drop. The master's corrections
// never come here.
func (s *Service) dropToZeroHook(ctx context.Context, tx pgx.Tx, campaignID, characterID string, now *playv1.CharacterVitals, dmg combat.DamageResult) (hookPlan, bool, error) {
	if !dmg.FellToZero || now.GetWildShape() != nil || dmg.Excess >= int(now.GetHitPointsMax()) {
		return hookPlan{}, false, nil
	}
	sheet, err := s.roster.CombatSheet(ctx, tx, campaignID, characterID)
	if err != nil {
		return hookPlan{}, false, err
	}
	h, ok := pickHook(sheet.Hooks, rules.EventDropToZero, now)
	if !ok {
		return hookPlan{}, false, nil
	}
	plan, err := s.planHook(h, now, 0, 0)
	if err != nil || plan.ev.Healed == 0 {
		return plan, false, err // a hook that does not heal does not keep the character up: nothing is spent
	}
	return plan, true, nil
}

// regainOnInitiative gives back the use of each resource of the character that recharges on initiative and has none
// left, when it rolls its initiative. It returns the vitals when something changed.
func (s *Service) regainOnInitiative(ctx context.Context, c *combatTx, target playdb.Combatant) (*playv1.CharacterVitals, error) {
	if target.Kind != kindPlayer || !backedByCharacter(target) {
		return nil, nil
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, target.CharacterID)
	if err != nil {
		return nil, err
	}
	var last *playv1.CharacterVitals
	for _, r := range now.GetResources() {
		recharge := ""
		if r.GetRecharge() == rulesv1.Recharge_RECHARGE_INITIATIVE {
			recharge = rules.RechargeInitiative
		}
		n := rules.InitiativeRecovery(recharge, int(r.GetUsed()), int(r.GetTotal()))
		if n == 0 {
			continue
		}
		if last, err = s.spendResource(ctx, c, target.CharacterID, r.GetKey(), -clamp32(n, 0, math.MaxInt32)); err != nil {
			return nil, err
		}
	}
	return last, nil
}
