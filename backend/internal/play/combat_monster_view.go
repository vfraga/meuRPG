package play

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// GetCreatureTurn implements playv1connect.CreatureServiceHandler.
func (s *Service) GetCreatureTurn(
	ctx context.Context,
	req *connect.Request[playv1.GetCreatureTurnRequest],
) (*connect.Response[playv1.GetCreatureTurnResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	// The combat, its combatants and the stat block are one snapshot: the reads of the
	// creature go through the transaction too (no second connection).
	var out *playv1.CreatureTurn
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		out = nil
		q := s.queries.WithTx(tx)
		session, err := openSessionWith(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		enc, err := encounterInSessionWith(ctx, q, session.ID, encID)
		if err != nil {
			return err
		}
		cs, err := q.ListCombatants(ctx, enc.ID)
		if err != nil {
			return fmt.Errorf("list the combatants: %w", err)
		}
		who, err := findCombatant(cs, combID, combatViewer{master: true})
		if err != nil {
			return err
		}
		mon, err := s.monsterOf(ctx, tx, m.CampaignID, who)
		if err != nil {
			return err
		}
		if mon == nil {
			return errNotAMonster()
		}
		st, err := monsterStateOf(who)
		if err != nil {
			return err
		}
		creature, ok, err := s.roster.CreatureStatBlock(ctx, tx, m.CampaignID, mon.key)
		if err != nil {
			return err
		}
		if !ok {
			return errNotAMonster()
		}
		out = creatureTurnProto(mon, enc, who, st, creature)
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "read a monster's turn", err)
	}
	return connect.NewResponse(&playv1.GetCreatureTurnResponse{Turn: out}), nil
}

var abilityProto = map[rules.Ability]rulesv1.Ability{
	rules.STR: rulesv1.Ability_ABILITY_STRENGTH, rules.DEX: rulesv1.Ability_ABILITY_DEXTERITY, rules.CON: rulesv1.Ability_ABILITY_CONSTITUTION,
	rules.INT: rulesv1.Ability_ABILITY_INTELLIGENCE, rules.WIS: rulesv1.Ability_ABILITY_WISDOM, rules.CHA: rulesv1.Ability_ABILITY_CHARISMA,
}

// abilityFromProto reads an ability of the API.
var abilityFromProto = func() map[rulesv1.Ability]rules.Ability {
	out := make(map[rulesv1.Ability]rules.Ability, len(abilityProto))
	for k, v := range abilityProto {
		out[v] = k
	}
	return out
}()

// rulesv1Unspecified is the unset ability.
const rulesv1Unspecified = rulesv1.Ability_ABILITY_UNSPECIFIED

var (
	actionKindProto = map[string]playv1.CreatureActionKind{
		rules.ActionKindAttack:      playv1.CreatureActionKind_CREATURE_ACTION_KIND_ATTACK,
		rules.ActionKindSave:        playv1.CreatureActionKind_CREATURE_ACTION_KIND_SAVE,
		rules.ActionKindMultiattack: playv1.CreatureActionKind_CREATURE_ACTION_KIND_MULTIATTACK,
		rules.ActionKindOther:       playv1.CreatureActionKind_CREATURE_ACTION_KIND_TEXT_ONLY,
	}
	readingProto = map[string]playv1.CreatureReading{
		rules.ActionStructured: playv1.CreatureReading_CREATURE_READING_STRUCTURED,
		rules.ActionPartial:    playv1.CreatureReading_CREATURE_READING_PARTIAL,
		rules.ActionText:       playv1.CreatureReading_CREATURE_READING_TEXT,
	}
	usageKindProto = map[string]playv1.CreatureUsageKind{
		rules.UsageAtWill:   playv1.CreatureUsageKind_CREATURE_USAGE_KIND_AT_WILL,
		rules.UsageRecharge: playv1.CreatureUsageKind_CREATURE_USAGE_KIND_RECHARGE,
		rules.UsagePerDay:   playv1.CreatureUsageKind_CREATURE_USAGE_KIND_PER_DAY,
		rules.UsageRest:     playv1.CreatureUsageKind_CREATURE_USAGE_KIND_REST,
	}
)

// formulaString writes dice as the stat block does: "2d10+8", "18d6", a flat "1".
func formulaString(f rules.DiceFormula) string {
	if f.Count == 0 {
		return fmt.Sprint(f.Bonus)
	}
	text := fmt.Sprintf("%dd%d", f.Count, f.Sides)
	switch {
	case f.Bonus > 0:
		return fmt.Sprintf("%s+%d", text, f.Bonus)
	case f.Bonus < 0:
		return fmt.Sprintf("%s%d", text, f.Bonus)
	}
	return text
}

func damagePartsProto(parts []rules.ActionDamage) []*playv1.CreatureDamagePart {
	var out []*playv1.CreatureDamagePart
	for _, d := range parts {
		out = append(out, &playv1.CreatureDamagePart{Dice: formulaString(d.Dice), DamageTypeKey: d.TypeKey, DamageTypePt: d.TypeNamePT})
	}
	return out
}

func areaProto(a *rules.ActionArea) *playv1.CreatureArea {
	if a == nil {
		return nil
	}
	return &playv1.CreatureArea{Shape: a.Shape, LengthFt: clamp32(a.SizeFt, 0, 10000), WidthFt: clamp32(a.WidthFt, 0, 10000)}
}

func savePlanProto(sv *rules.ActionSave, area *rules.ActionArea, conditionPT func(string) string) *playv1.CreatureSavePlan {
	if sv == nil {
		return nil
	}
	return &playv1.CreatureSavePlan{
		Ability: abilityProto[sv.Ability], Dc: clamp32(sv.DC, 0, 100), HalfOnSuccess: sv.OnSuccess == "half", OnSuccess: sv.OnSuccess,
		Damage: damagePartsProto(sv.Damage), ConditionKey: sv.ConditionKey, ConditionPt: conditionPT(sv.ConditionKey),
		Duration: sv.Duration, RepeatSave: sv.RepeatSave, Area: areaProto(area),
	}
}

func stepProto(st rules.MultiattackStep) *playv1.CreatureMultiattackStep {
	return &playv1.CreatureMultiattackStep{Name: st.Name, ActionKey: st.ActionKey, Count: clamp32(st.Count, 0, 100), CountText: st.Text, Kind: st.Kind}
}

// actionProto is an action of the stat block with what the creature has spent of it.
func actionProto(a rules.ActionPlan, st combat.MonsterState, conditionPT func(string) string) *playv1.CreatureAction {
	act := &playv1.CreatureAction{
		Key: a.Key, Name: a.Name, NamePt: a.NamePT, Kind: actionKindProto[a.Kind], Reading: readingProto[a.Class],
		DamageParts: damagePartsProto(a.Damage), Text: a.Text, Available: st.CanUse(a.Usage, a.Key) == nil,
	}
	act.Usage = &playv1.CreatureUsage{
		Kind: usageKindProto[a.Usage.Kind], RechargeMin: clamp32(a.Usage.RechargeMin, 0, 6), UsesMax: clamp32(a.Usage.Uses, 0, 100),
		Recharging: st.IsRecharging(a.Key), Text: a.Usage.Text,
	}
	if left := st.UsesLeft(a.Usage, a.Key); left >= 0 {
		act.Usage.UsesLeft = clamp32(left, 0, 100)
		if a.Usage.Kind == rules.UsageRest {
			act.Usage.UsesMax = 1
		}
	}
	for _, r := range st.RechargeRolls {
		if r.Action == a.Key {
			act.Usage.RechargeRoll = clamp32(r.Face, 0, 6)
		}
	}
	if a.Attack != nil {
		act.AttackBonus, act.Melee, act.SpellAttack = clamp32(a.Attack.Bonus, -100, 100), a.Attack.Melee, a.Attack.Spell
		act.ReachFt, act.RangeFt, act.LongRangeFt = clamp32(a.Attack.ReachFt, 0, 10000), clamp32(a.Attack.RangeFt, 0, 10000), clamp32(a.Attack.LongRangeFt, 0, 10000)
	}
	if a.Kind == rules.ActionKindSave {
		act.Save = savePlanProto(a.Save, a.Area, conditionPT)
	}
	if a.Kind == rules.ActionKindAttack && (a.Save != nil || a.HitCondition != nil) {
		rider := &playv1.CreatureRider{}
		if a.Save != nil {
			rider.Save = savePlanProto(a.Save, nil, conditionPT)
		}
		if h := a.HitCondition; h != nil {
			rider.ConditionKey, rider.ConditionPt, rider.EscapeDc, rider.MaxSize = h.ConditionKey, conditionPT(h.ConditionKey), clamp32(h.EscapeDC, 0, 100), h.MaxSize
		}
		act.Rider = rider
	}
	for _, routine := range a.Routines {
		r := &playv1.CreatureRoutine{}
		for _, step := range routine {
			r.Steps = append(r.Steps, stepProto(step))
		}
		act.Routines = append(act.Routines, r)
	}
	if ms := st.Multiattack; ms != nil && ms.Action == a.Key {
		if seq := sequenceOf(ms, a); seq != nil {
			act.SequenceStep = seq.Next
		}
	}
	return act
}

// sequenceOf is the Multiattack sequence of the turn: the routine chosen, flattened step by step
// (an action made 2 times is 2 steps), with how many are done. a is the Multiattack action.
func sequenceOf(ms *combat.MultiattackState, a rules.ActionPlan) *playv1.CreatureSequence {
	if ms.Routine >= len(a.Routines) {
		return nil
	}
	out := &playv1.CreatureSequence{ActionKey: ms.Action, Routine: clamp32(ms.Routine, 0, 100)}
	for i, step := range a.Routines[ms.Routine] {
		left := 0
		if i < len(ms.Left) {
			left = ms.Left[i]
		}
		for n := range max(step.Count, 1) {
			out.Steps = append(out.Steps, &playv1.CreatureSequenceStep{Name: step.Name, ActionKey: step.ActionKey, Done: n < step.Count-left})
		}
	}
	for i, st := range out.Steps {
		if !st.Done {
			out.Next = int32(i + 1)
			break
		}
	}
	return out
}

// legendaryProto is the creature's legendary actions: the options as actions with their cost.
func legendaryProto(mon *monsterRef, enc playdb.Encounter, who playdb.Combatant, st combat.MonsterState, conditionPT func(string) string) *playv1.CreatureLegendary {
	lg := &playv1.CreatureLegendary{State: legendaryStateProto(mon, st)}
	lg.BlockedReason = legendaryReason(enc, who, st, mon.plan, 1)
	for _, o := range mon.plan.Legendary.Options {
		opt := &playv1.CreatureAction{
			Key: o.Key, Name: o.Name, NamePt: o.NamePT, Text: o.Text, LegendaryCost: clamp32(o.Cost, 0, 10), LinkedActionKey: o.ActionKey,
			Kind: playv1.CreatureActionKind_CREATURE_ACTION_KIND_TEXT_ONLY, Reading: playv1.CreatureReading_CREATURE_READING_TEXT,
			Available: legendaryReason(enc, who, st, mon.plan, o.Cost) == playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNSPECIFIED,
		}
		switch {
		case o.Action != nil:
			opt.Kind, opt.Reading = playv1.CreatureActionKind_CREATURE_ACTION_KIND_SAVE, readingProto[o.Action.Class]
			opt.Save = savePlanProto(o.Action.Save, o.Action.Area, conditionPT)
		case o.ActionKey != "":
			if linked, ok := mon.plan.Action(o.ActionKey); ok {
				opt.Kind, opt.Reading = actionKindProto[linked.Kind], readingProto[linked.Class]
			}
		}
		lg.Options = append(lg.Options, opt)
	}
	return lg
}

func legendaryStateProto(mon *monsterRef, st combat.MonsterState) *playv1.CreatureLegendaryState {
	out := &playv1.CreatureLegendaryState{
		ResistanceUses: clamp32(mon.plan.LegendaryResistance, 0, 10), ResistanceLeft: clamp32(st.ResistanceLeft(mon.plan), 0, 10),
	}
	if mon.plan.Legendary != nil {
		out.PerRound, out.Left = clamp32(mon.plan.Legendary.PerRound, 0, 10), clamp32(st.LegendaryLeft(mon.plan), 0, 10)
	}
	return out
}

func traitProto(t rules.TraitPlan) *playv1.CreatureTrait {
	return &playv1.CreatureTrait{Key: t.Key, Name: t.Name, NamePt: t.NamePT, Text: t.Text, Usage: t.Usage.Text, EngineReads: t.EngineReads}
}

// sheetProto is the master's copy of a monster's sheet: what the rolls read, the immunities, the
// spellcasting with what is left, and the traits.
func sheetProto(mon *monsterRef, st combat.MonsterState, creature *rulesv1.Creature) *playv1.CreatureSheet {
	out := &playv1.CreatureSheet{
		Saves: creature.GetSavingThrows(), Skills: creature.GetSkills(), Immunities: creature.GetImmunities(),
		Resistances: creature.GetResistances(), Vulnerabilities: creature.GetVulnerabilities(),
		ConditionImmunities: creature.GetConditionImmunities(), PassivePerception: creature.GetPassivePerception(),
	}
	for _, sc := range mon.plan.Spellcasting {
		out.Spellcasting = append(out.Spellcasting, spellcastingProto(sc, st))
	}
	for _, t := range mon.plan.Traits {
		out.Traits = append(out.Traits, traitProto(t))
	}
	for _, t := range mon.plan.Reactions {
		out.Reactions = append(out.Reactions, traitProto(t))
	}
	return out
}

// creatureTurnProto is the master's view of a monster's turn.
func creatureTurnProto(mon *monsterRef, enc playdb.Encounter, who playdb.Combatant, st combat.MonsterState, creature *rulesv1.Creature) *playv1.CreatureTurn {
	conditionPT := func(key string) string {
		if key == "" {
			return ""
		}
		return mon.content.NamePT(key)
	}
	out := &playv1.CreatureTurn{CombatantId: who.ID, Creature: creature, Sheet: sheetProto(mon, st, creature)}
	for _, a := range mon.plan.Actions {
		out.Actions = append(out.Actions, actionProto(a, st, conditionPT))
	}
	if mon.plan.Legendary != nil {
		out.Legendary = legendaryProto(mon, enc, who, st, conditionPT)
	}
	if ms := st.Multiattack; ms != nil {
		if a, ok := mon.plan.Action(ms.Action); ok {
			out.Sequence = sequenceOf(ms, a)
		}
	}
	return out
}

func spellRefProto(r rules.SpellRef, uses, left int) *playv1.CreatureSpell {
	return &playv1.CreatureSpell{
		Key: r.Key, Name: r.Name, NamePt: r.NamePT, Level: clamp32(r.Level, 0, 9), Note: r.Note,
		AtWill: uses < 0, Uses: clamp32(max(uses, 0), 0, 100), UsesLeft: clamp32(max(left, 0), 0, 100),
	}
}

func spellcastingProto(sc rules.SpellcastingPlan, st combat.MonsterState) *playv1.CreatureSpellcasting {
	out := &playv1.CreatureSpellcasting{
		Innate: sc.Innate, Name: sc.Name, Ability: abilityProto[sc.Ability], SaveDc: clamp32(sc.SaveDC, 0, 100),
		AttackBonus: clamp32(sc.AttackBonus, -100, 100), CasterLevel: clamp32(sc.CasterLevel, 0, 20), ClassKey: sc.ClassKey, Text: sc.Text,
	}
	for _, r := range sc.AtWill {
		out.AtWill = append(out.AtWill, spellRefProto(r, -1, 0))
	}
	for _, l := range sc.Levels {
		lvl := &playv1.CreatureSpellLevel{Level: clamp32(l.Level, 1, 9), Slots: clamp32(l.Slots, 0, 100), SlotsLeft: clamp32(st.SlotLeft(l.Level, l.Slots), 0, 100)}
		for _, r := range l.Spells {
			lvl.Spells = append(lvl.Spells, spellRefProto(r, -1, 0))
		}
		out.Levels = append(out.Levels, lvl)
	}
	for g, grp := range sc.PerDay {
		for _, r := range grp.Spells {
			left := grp.Uses - st.Used[innateUseKey(sc, g, r.Key)]
			out.PerDay = append(out.PerDay, spellRefProto(r, grp.Uses, left))
		}
	}
	return out
}

// innateUseKey is the key the uses of an innate spell are counted under: the spell's own for a
// group whose spells each have the uses ("3/day each"), the group's for a group that shares them.
func innateUseKey(sc rules.SpellcastingPlan, group int, spellKey string) string {
	if sc.PerDay[group].Each {
		return spellKey
	}
	return combat.InnateGroupKey(sc.Key, group)
}

// creatureView adds to the master's copy of a combat what only the master gets of the monsters in
// it (RN-20): each one's sheet and legendary actions, the chances to take a legendary action and
// the failed saving throws that wait for a Legendary Resistance answer. A player's copy has none.
func (s *Service) creatureView(ctx context.Context, m authz.Membership, d *encounterData, out *playv1.Encounter) error {
	if m.Role != authz.RoleMaster {
		return nil
	}
	mons, err := s.monstersOf(ctx, nil, m.CampaignID, d.cs)
	if err != nil || len(mons) == 0 {
		return err
	}
	open, err := s.queries.ListOpenPendingDamages(ctx, d.enc.ID)
	if err != nil {
		return s.dbError(ctx, "list the pending damage", err)
	}
	waiting := map[string]bool{} // the damage still to be rolled
	for _, p := range open {
		waiting[p.ID] = p.Status == pendingAwaitingRoll
	}
	names := s.namesFor(ctx, m.CampaignID)
	sheets := map[string]*rulesv1.Creature{}
	for _, who := range d.cs {
		mon := mons[who.ID]
		if mon == nil {
			continue
		}
		st, err := monsterStateOf(who)
		if err != nil {
			return s.dbError(ctx, "read the monster state", err)
		}
		creature, ok := sheets[mon.key]
		if !ok {
			if creature, ok, err = s.roster.CreatureStatBlock(ctx, nil, m.CampaignID, mon.key); err != nil || !ok {
				continue
			}
			sheets[mon.key] = creature
		}
		for _, cb := range out.Combatants {
			if cb.GetId() != who.ID {
				continue
			}
			cb.CreatureSheet = sheetProto(mon, st, creature)
			if mon.plan.Legendary != nil || mon.plan.LegendaryResistance > 0 {
				cb.Legendary = legendaryStateProto(mon, st)
			}
		}
		if st.Offer != nil && mon.plan.Legendary != nil && !who.Defeated && combat.CanAct(who.Conditions) && d.enc.Status == statusActive {
			offer := &playv1.LegendaryOffer{
				CombatantId: who.ID, AfterCombatantId: st.Offer.After, Left: clamp32(st.LegendaryLeft(mon.plan), 0, 10), PerRound: clamp32(mon.plan.Legendary.PerRound, 0, 10),
			}
			for _, o := range mon.plan.Legendary.Options {
				offer.Options = append(offer.Options, &playv1.LegendaryOfferOption{
					Key: o.Key, Name: o.Name, NamePt: o.NamePT, Cost: clamp32(o.Cost, 0, 10), ActionKey: o.ActionKey, Text: o.Text,
					Available: st.LegendaryLeft(mon.plan) >= o.Cost,
				})
			}
			out.LegendaryOffers = append(out.LegendaryOffers, offer)
		}
		for _, p := range st.Prompts {
			stillOpen := len(p.Pending) == 0 || slices.ContainsFunc(p.Pending, func(id string) bool { return waiting[id] })
			if !stillOpen || st.ResistanceLeft(mon.plan) < 1 {
				continue
			}
			out.LegendaryResistancePrompts = append(out.LegendaryResistancePrompts, &playv1.LegendaryResistancePrompt{
				CombatantId: who.ID, CastId: p.Cast, SpellKey: p.Spell, SpellNamePt: actionOrSpellName(names, p.Spell), CasterId: p.Caster,
				Ability: abilityProto[rules.Ability(p.Ability)], Dc: clamp32(p.DC, 0, 100), D20: clamp32(p.D20, 1, 20),
				Bonus: clamp32(p.Bonus, -100, 100), Total: clamp32(p.Total, -100, 200),
				UsesLeft: clamp32(st.ResistanceLeft(mon.plan), 0, 10), Uses: clamp32(mon.plan.LegendaryResistance, 0, 10),
			})
		}
	}
	return nil
}

// actionOrSpellName is the Portuguese name of a spell, or of an action of a stat block
// ("monster:ghoul#claws" is the name of "claws"); "" when the content has none.
func actionOrSpellName(names func(string) string, key string) string {
	if _, slug, ok := strings.Cut(key, "#"); ok {
		return names("attack:" + strings.TrimPrefix(slug, "legendary-"))
	}
	return names(key)
}
