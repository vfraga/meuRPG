package play

import (
	"context"
	"fmt"
	"slices"
	"strings"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// What a caller reads of the effects that last (RN-10, RN-20): the master everything, the DC
// too. A player reads, on their own character, each effect on it, never the DC of an effect an
// NPC cast; on the other combatants only the labels of the effects the master leaves visible
// (the "Os jogadores veem este efeito" switch); and nothing of an effect the master hides, in
// the order, a wait, the source of an advantage, the log or a count.

// ownerOnlyConditions are the conditions with no sign on the outside, which only the creature's
// player and the master read (effects/combat_effects.json says it; a test keeps the two equal).
var ownerOnlyConditions = []string{"condition:charmed", "condition:deafened", conditionExhaustion, "condition:frightened", "condition:invisible", "condition:poisoned"}

// The line a player reads for what they may not read.
const (
	unseenOrigin  = "De alguém que você não vê"
	otherSourceTx = "Outra fonte"
)

// effectGroup is the rows of one casting of one effect: one for each target.
type effectGroup struct {
	rows []playdb.CombatantState
}

// first is the row that names the group: the lowest id.
func (g effectGroup) first() playdb.CombatantState {
	best := g.rows[0]
	for _, r := range g.rows[1:] {
		if r.ID < best.ID {
			best = r
		}
	}
	return best
}

// groupsOf gathers the rows of the effects into castings, in the order they began.
func groupsOf(rows []playdb.CombatantState) []effectGroup {
	var out []effectGroup
	for _, st := range rows {
		if !isEffect(st) {
			continue
		}
		i := slices.IndexFunc(out, func(g effectGroup) bool {
			f := g.rows[0]
			return deref(f.GroupID) == deref(st.GroupID) && deref(f.SourceKey) == deref(st.SourceKey)
		})
		if i < 0 {
			out = append(out, effectGroup{})
			i = len(out) - 1
		}
		out[i].rows = append(out[i].rows, st)
	}
	return out
}

// effectViewer is what builds what a caller reads of the effects of a combat.
type effectViewer struct {
	s       *Service
	m       authz.Membership
	v       combatViewer
	d       *encounterData
	content *rules.Content
	names   func(string) string
	byID    map[string]playdb.Combatant
}

func (s *Service) effectViewerFor(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, names func(string) string) (*effectViewer, error) {
	content, err := s.roster.RulesContent(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the rules content", err)
	}
	ev := &effectViewer{s: s, m: m, v: v, d: d, content: content, names: names, byID: combatantsByID(d.cs)}
	return ev, nil
}

// readsEffect says the viewer reads an effect row on its target: the master always; the target's
// own player when the master leaves it visible; the others when the master leaves it visible to everyone and they see the
// target.
func (ev *effectViewer) readsEffect(st playdb.CombatantState) bool {
	if ev.v.master {
		return true
	}
	target, ok := ev.byID[st.CombatantID]
	if !ok {
		return false
	}
	if !st.PlayerVisible {
		return false // what the master hides is his alone, the target's player included (RN-10)
	}
	if ev.v.owns(target) {
		return true
	}
	return ev.v.sees(target) && st.Audience == "all"
}

// conditionHidden says the viewer may not read a condition of a combatant: one with no outward
// sign, or one only an effect the master hides gives (RN-10).
func (ev *effectViewer) conditionHidden(who playdb.Combatant, cond string) bool {
	if ev.v.master || ev.v.owns(who) {
		return false
	}
	if slices.Contains(who.EffectConditions, cond) {
		for _, st := range effectsOn(statesOf(ev.d.states), who.ID) {
			if slices.Contains(st.ConditionKeys, cond) && st.PlayerVisible && st.Audience == "all" && !slices.Contains(ownerOnlyConditions, cond) {
				return false
			}
		}
		return true
	}
	return slices.Contains(ownerOnlyConditions, cond)
}

// decorate fills what the effects add to a combatant the viewer sees: the conditions they may
// read, the labels, the cards and the exhaustion.
func (ev *effectViewer) decorate(p *playv1.Combatant, c playdb.Combatant) {
	if !ev.v.master && !ev.v.owns(c) {
		ev.decorateConditions(p, c)
	}
	if ev.v.master || ev.v.owns(c) {
		p.ExhaustionLevel = c.ExhaustionLevel
	}
	states := statesOf(ev.d.states)
	for _, g := range groupsOf(effectsOn(states, c.ID)) {
		own := g.rows[0] // the row of this combatant (the rows of a group are one for each target)
		if !ev.readsEffect(own) {
			continue
		}
		if label := ev.labelFor(own); label != "" {
			p.EffectLabels = append(p.EffectLabels, &playv1.EffectLabel{EffectId: own.ID, TextPt: label})
		}
		ev.decorateCard(p, c, own)
	}
}

// decorateConditions leaves on the combatant only the conditions the viewer may read.
func (ev *effectViewer) decorateConditions(p *playv1.Combatant, c playdb.Combatant) {
	p.Conditions, p.ConditionNamesPt = nil, nil
	for _, k := range c.Conditions {
		if !ev.conditionHidden(c, k) {
			p.Conditions = append(p.Conditions, k)
			p.ConditionNamesPt = append(p.ConditionNamesPt, ev.names(k))
		}
	}
}

// decorateCard puts the card of an effect on its combatant, for the master and the combatant's
// own player.
func (ev *effectViewer) decorateCard(p *playv1.Combatant, c playdb.Combatant, own playdb.CombatantState) {
	if !ev.v.master && !ev.v.owns(c) {
		return
	}
	if card := ev.card(effectGroup{rows: []playdb.CombatantState{own}}, c.ID); card != nil && ev.hasCard(own) {
		p.Effects = append(p.Effects, card)
	}
}

// hasCard says an effect has a card of its own: a condition the master only marked is the
// label of the header and no more.
func (ev *effectViewer) hasCard(st playdb.CombatantState) bool {
	return st.SourceKind == nil || *st.SourceKind != "master" || len(effectModifiers(st)) > 0 || st.EndSaveAbility != nil || st.TriggerDice != nil
}

// labelOf is what the others read on a target for an effect: the master's free label, the
// catalog's ("Preso numa teia", "Delineado"), or the names of the conditions it gives.
func (ev *effectViewer) labelOf(st playdb.CombatantState) string {
	if st.PlayerLabel != nil {
		return *st.PlayerLabel
	}
	if def, ok := ev.s.effectDef(ev.content, deref(st.SourceKey)); ok && def.PlayerLabelPT != "" {
		return def.PlayerLabelPT
	}
	return ev.conditionsLabel(st, func(string) bool { return false })
}

// conditionsLabel is the names of the conditions an effect gives, leaving out the ones hidden reports the reader may not read.
func (ev *effectViewer) conditionsLabel(st playdb.CombatantState, hidden func(string) bool) string {
	var names []string
	for _, k := range st.ConditionKeys {
		if !hidden(k) {
			names = append(names, ev.names(k))
		}
	}
	return strings.Join(names, ", ")
}

// labelFor is the label of an effect as this viewer reads it. A label that is the names of the
// conditions an effect gives leaves out the ones the viewer may not read (RN-10): "Envenenado" is
// the owner's and the master's, so another player reads "Derrubado" alone, or no label at all.
func (ev *effectViewer) labelFor(st playdb.CombatantState) string {
	if st.PlayerLabel != nil || ev.v.master {
		return ev.labelOf(st)
	}
	if def, ok := ev.s.effectDef(ev.content, deref(st.SourceKey)); ok && def.PlayerLabelPT != "" {
		return def.PlayerLabelPT
	}
	target, ok := ev.byID[st.CombatantID]
	if !ok {
		return ""
	}
	return ev.conditionsLabel(st, func(k string) bool { return ev.conditionHidden(target, k) })
}

// labelForOthers is what the players who do not own the target read, for the master's "Os jogadores veem".
func (ev *effectViewer) labelForOthers(st playdb.CombatantState) string {
	if st.PlayerLabel != nil {
		return *st.PlayerLabel
	}
	if def, ok := ev.s.effectDef(ev.content, deref(st.SourceKey)); ok && def.PlayerLabelPT != "" {
		return def.PlayerLabelPT
	}
	return ev.conditionsLabel(st, func(k string) bool { return slices.Contains(ownerOnlyConditions, k) })
}

// card builds an effect as the viewer reads it. onTarget is the combatant whose card it is (a
// player's own); empty for the master's table.
func (ev *effectViewer) card(g effectGroup, onTarget string) *playv1.LastingEffect {
	f := g.first()
	key := deref(f.SourceKey)
	def, _ := ev.s.effectDef(ev.content, key)
	out := ev.cardHead(f, key, def)
	ev.cardTargets(out, g, onTarget)
	caster, casterSeen := ev.byID[deref(f.SourceID)]
	casterSeen = casterSeen && ev.v.sees(caster)
	if casterSeen {
		out.CasterId = caster.ID
	}
	out.OriginPt = ev.originOf(f, caster, casterSeen)
	for _, m := range effectModifiers(f) {
		out.Modifiers = append(out.Modifiers, modifierProto(m))
	}
	ev.cardClock(out, f, caster, casterSeen)
	ev.cardSaves(out, f, ev.v.master || (casterSeen && ev.v.owns(caster)))
	ev.cardNotes(out, f, def)
	if ev.v.master {
		out.PlayerVisible, out.Audience, out.PlayerLabel = f.PlayerVisible, audienceProto(f.Audience), deref(f.PlayerLabel)
		out.EndTextPt = ev.endText(f, caster)
		if f.PlayerVisible {
			out.PlayersSeePt = ev.labelForOthers(f)
		}
	}
	return out
}

// cardHead is the card with what names the effect: the ids, the source, the conditions it gives.
func (ev *effectViewer) cardHead(f playdb.CombatantState, key string, def *rules.EffectDef) *playv1.LastingEffect {
	// The casting's id is the master's: for an effect that came from a cast outside a combat it is
	// that cast's id (RN-10).
	group := f.ID
	if ev.v.master {
		group = deref(f.GroupID)
	}
	out := &playv1.LastingEffect{
		Id: f.ID, GroupId: group, EncounterId: f.EncounterID, SourceKey: key, SourceNamePt: ev.names(key),
		Concentration: f.Concentration, ConditionKeys: f.ConditionKeys,
	}
	out.SourceKind = sourceKindProto(deref(f.SourceKind), len(f.ConditionKeys) > 0 && def == nil)
	if out.SourceNamePt == "" && len(f.ConditionKeys) > 0 {
		out.SourceNamePt = ev.names(f.ConditionKeys[0])
	}
	for _, k := range f.ConditionKeys {
		out.ConditionNamesPt = append(out.ConditionNamesPt, ev.names(k))
	}
	return out
}

// sourceKindProto is where an effect comes from; one the master made that only gives conditions
// and is not in the catalog is a condition.
func sourceKindProto(kind string, onlyConditions bool) playv1.EffectSourceKind {
	switch kind {
	case "spell":
		return playv1.EffectSourceKind_EFFECT_SOURCE_KIND_SPELL
	case "feature":
		return playv1.EffectSourceKind_EFFECT_SOURCE_KIND_FEATURE
	}
	if onlyConditions {
		return playv1.EffectSourceKind_EFFECT_SOURCE_KIND_CONDITION
	}
	return playv1.EffectSourceKind_EFFECT_SOURCE_KIND_MASTER
}

// cardTargets names the targets the viewer may read (all of them, or the one of a player's card).
func (ev *effectViewer) cardTargets(out *playv1.LastingEffect, g effectGroup, onTarget string) {
	for _, st := range g.rows {
		if onTarget != "" && st.CombatantID != onTarget {
			continue
		}
		if !ev.readsEffect(st) {
			continue
		}
		out.TargetIds = append(out.TargetIds, st.CombatantID)
		out.TargetLabels = append(out.TargetLabels, ev.byID[st.CombatantID].Label)
	}
}

// cardClock puts the duration of the effect on its card: the kind, where it ends and the text.
func (ev *effectViewer) cardClock(out *playv1.LastingEffect, f playdb.CombatantState, caster playdb.Combatant, casterSeen bool) {
	out.DurationKind = durationKindProto(deref(f.DurationKind))
	if e := endsAtOf(f); e.Timed() {
		end := &playv1.EffectEnd{Round: clamp32(e.Round, 0, 1<<20), CombatantId: e.CombatantID, Phase: phaseProto(e.Phase)}
		round := max(ev.d.enc.Round, 1)
		out.EndsAt, out.RoundsLeft = end, new(clamp32(e.RoundsLeft(int(round)), 0, 1<<20))
	}
	out.ClockTextPt = ev.clockText(f, caster, casterSeen)
}

// cardSaves puts the saving throws and the trigger of the effect on its card; the DC is the
// caster's own to read.
func (ev *effectViewer) cardSaves(out *playv1.LastingEffect, f playdb.CombatantState, ownCaster bool) {
	if f.EndSaveAbility != nil {
		out.EndSave = ev.saveProto(*f.EndSaveAbility, combat.PhaseEnd, f.SaveDc, ownCaster)
	}
	if f.StartSaveAbility != nil {
		out.StartSave = ev.saveProto(*f.StartSaveAbility, combat.PhaseStart, f.SaveDc, ownCaster)
	}
	if f.TriggerDice != nil {
		out.TurnTrigger = &playv1.EffectTrigger{
			Phase: playv1.EffectPhase_EFFECT_PHASE_START, Dice: *f.TriggerDice, DamageTypeKey: deref(f.TriggerDamageType),
			DamageTypePt: ev.names(deref(f.TriggerDamageType)), MaxTriggers: derefInt32(f.TriggerMaxTriggers),
		}
	}
}

// cardNotes puts the tags, how to break free and what the effect changes on its card.
func (ev *effectViewer) cardNotes(out *playv1.LastingEffect, f playdb.CombatantState, def *rules.EffectDef) {
	if def != nil {
		out.TagsPt = append(append(append([]string{}, def.TagsPT...), modifierTagsPT(effectModifiers(f))...), spentTagsPT(f)...)
		out.BreakFreeAbility = def.BreakFree
	}
	for _, k := range f.ConditionKeys {
		if info, ok := ev.content.ConditionInfo(k); ok {
			out.ChangesPt = append(out.ChangesPt, info.ChangesPT...)
		}
	}
	out.ChangesPt = append(out.ChangesPt, narratedNotesPT(effectModifiers(f))...)
}

// originOf says whose it is, without naming a caster the viewer does not see.
func (ev *effectViewer) originOf(f playdb.CombatantState, caster playdb.Combatant, seen bool) string {
	if f.FollowsKey != nil {
		name := ev.names(*f.FollowsKey)
		if seen {
			return fmt.Sprintf("O fim de %s de %s", name, caster.Label)
		}
		return "O fim de " + name
	}
	switch {
	case deref(f.SourceKind) == "master" || f.SourceID == nil:
		return "Do mestre"
	case seen:
		return "De " + caster.Label
	}
	return unseenOrigin
}

// clockText is the clock as a card reads it: "Restam 8 rodadas: acaba no turno de Tavo, na
// rodada 11." A player who does not see the caster reads the end without the name and the count.
func (ev *effectViewer) clockText(f playdb.CombatantState, caster playdb.Combatant, casterSeen bool) string {
	e := endsAtOf(f)
	round := int(max(ev.d.enc.Round, 1))
	who := func() string {
		if a, ok := ev.byID[e.CombatantID]; ok && ev.v.sees(a) {
			return "o turno de " + a.Label
		}
		return "o turno de quem a conjurou"
	}
	switch deref(f.DurationKind) {
	case rules.EffectDurationRounds:
		if !casterSeen && !ev.v.master && f.SourceID != nil && deref(f.SourceKind) != "master" {
			return fmt.Sprintf("Resta o tempo da magia: acaba no turno de quem a conjurou, na rodada %d.", e.Round)
		}
		left := e.RoundsLeft(round)
		return fmt.Sprintf("%s: acaba no %s, na rodada %d.", roundsLeftText(left), strings.TrimPrefix(who(), "o "), e.Round)
	case rules.EffectDurationUntilStartOfTurnOf:
		return fmt.Sprintf("Até o começo d%s, na rodada %d.", who(), e.Round)
	case rules.EffectDurationUntilEndOfTurnOf:
		return fmt.Sprintf("Até o fim d%s, na rodada %d.", who(), e.Round)
	case rules.EffectDurationConcentration:
		if casterSeen || ev.v.master {
			return "Dura enquanto " + caster.Label + " se concentrar."
		}
		return "Dura enquanto quem a conjurou se concentrar."
	case rules.EffectDurationLongRest:
		return "Dura até um descanso longo."
	}
	return "Dura até o mestre encerrar."
}

// roundsLeftText is "Resta 1 rodada" or "Restam N rodadas".
func roundsLeftText(n int) string {
	if n == 1 {
		return "Resta 1 rodada"
	}
	return fmt.Sprintf("Restam %d rodadas", n)
}

// endText is the master's line of how an effect ends by itself.
func (ev *effectViewer) endText(f playdb.CombatantState, caster playdb.Combatant) string {
	e := endsAtOf(f)
	a := ev.byID[e.CombatantID]
	switch deref(f.DurationKind) {
	case rules.EffectDurationRounds:
		return fmt.Sprintf("%s: acaba no turno de %s, rodada %d.", roundsLeftText(e.RoundsLeft(int(max(ev.d.enc.Round, 1)))), a.Label, e.Round)
	case rules.EffectDurationUntilStartOfTurnOf:
		return fmt.Sprintf("Até o começo do turno de %s, rodada %d.", a.Label, e.Round)
	case rules.EffectDurationUntilEndOfTurnOf:
		return fmt.Sprintf("Até o fim do turno de %s, rodada %d.", a.Label, e.Round)
	case rules.EffectDurationConcentration:
		return "Enquanto " + caster.Label + " se concentrar."
	case rules.EffectDurationLongRest:
		return "Até um descanso longo."
	}
	return "Até o mestre encerrar."
}

func (ev *effectViewer) saveProto(ability, phase string, dc *int32, ownCaster bool) *playv1.EffectSave {
	out := &playv1.EffectSave{Ability: ability, AbilityNamePt: ev.names(ability), Phase: phaseProto(phase)}
	if ownCaster {
		out.Dc = dc
	}
	return out
}

func phaseProto(p string) playv1.EffectPhase {
	switch p {
	case combat.PhaseStart:
		return playv1.EffectPhase_EFFECT_PHASE_START
	case combat.PhaseEnd:
		return playv1.EffectPhase_EFFECT_PHASE_END
	}
	return playv1.EffectPhase_EFFECT_PHASE_UNSPECIFIED
}

func audienceProto(a string) playv1.EffectAudience {
	if a == "owner" {
		return playv1.EffectAudience_EFFECT_AUDIENCE_OWNER
	}
	return playv1.EffectAudience_EFFECT_AUDIENCE_ALL
}

func durationKindProto(k string) playv1.EffectDurationKind {
	switch k {
	case rules.EffectDurationRounds:
		return playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS
	case rules.EffectDurationUntilStartOfTurnOf:
		return playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_START_OF_TURN_OF
	case rules.EffectDurationUntilEndOfTurnOf:
		return playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_END_OF_TURN_OF
	case rules.EffectDurationConcentration:
		return playv1.EffectDurationKind_EFFECT_DURATION_KIND_CONCENTRATION
	case rules.EffectDurationLongRest:
		return playv1.EffectDurationKind_EFFECT_DURATION_KIND_LONG_REST
	}
	return playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_DISMISSED
}

func durationKindOf(k playv1.EffectDurationKind) string {
	switch k {
	case playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS:
		return rules.EffectDurationRounds
	case playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_START_OF_TURN_OF:
		return rules.EffectDurationUntilStartOfTurnOf
	case playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_END_OF_TURN_OF:
		return rules.EffectDurationUntilEndOfTurnOf
	case playv1.EffectDurationKind_EFFECT_DURATION_KIND_CONCENTRATION:
		return rules.EffectDurationConcentration
	case playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_DISMISSED:
		return rules.EffectDurationUntilDismissed
	case playv1.EffectDurationKind_EFFECT_DURATION_KIND_LONG_REST:
		return rules.EffectDurationLongRest
	}
	return ""
}

func modifierProto(m rules.EffectModifier) *playv1.EffectModifier {
	out := &playv1.EffectModifier{Die: clamp32(m.Die, 0, 100), Sign: clamp32(m.Sign, -1, 1), Value: clamp32(m.Value, 0, 1000), Abilities: m.Abilities, AllowedActions: m.Allowed, MaxWeaponAttacks: clamp32(m.MaxWeaponAttacks, 0, 10)}
	switch m.Kind {
	case rules.ModifierRollDie:
		out.Kind = playv1.EffectModifierKind_EFFECT_MODIFIER_KIND_ROLL_DIE
	case rules.ModifierACBonus:
		out.Kind = playv1.EffectModifierKind_EFFECT_MODIFIER_KIND_AC_BONUS
	case rules.ModifierSpeedMultiplier:
		out.Kind = playv1.EffectModifierKind_EFFECT_MODIFIER_KIND_SPEED_MULTIPLIER
	case rules.ModifierSaveAdvantage:
		out.Kind = playv1.EffectModifierKind_EFFECT_MODIFIER_KIND_SAVE_ADVANTAGE
	case rules.ModifierExtraAction:
		out.Kind = playv1.EffectModifierKind_EFFECT_MODIFIER_KIND_EXTRA_ACTION
	case rules.ModifierNoMove:
		out.Kind = playv1.EffectModifierKind_EFFECT_MODIFIER_KIND_NO_MOVE
	case rules.ModifierNoAction:
		out.Kind = playv1.EffectModifierKind_EFFECT_MODIFIER_KIND_NO_ACTION
	case rules.ModifierNoReaction:
		out.Kind = playv1.EffectModifierKind_EFFECT_MODIFIER_KIND_NO_REACTION
	}
	for _, a := range m.AppliesTo {
		switch a {
		case rules.RollAppliesAttack:
			out.AppliesTo = append(out.AppliesTo, playv1.EffectRollKind_EFFECT_ROLL_KIND_ATTACK)
		case rules.RollAppliesSave:
			out.AppliesTo = append(out.AppliesTo, playv1.EffectRollKind_EFFECT_ROLL_KIND_SAVE)
		}
	}
	return out
}

// decorateEncounter puts what the effects add on the combatants of a view the viewer reads.
func (s *Service) decorateEffects(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, names func(string) string, out *playv1.Encounter) error {
	ev, err := s.effectViewerFor(ctx, m, d, v, names)
	if err != nil {
		return err
	}
	for _, p := range out.Combatants {
		c, ok := ev.byID[p.Id]
		if !ok {
			continue
		}
		ev.decorate(p, c)
	}
	return nil
}

// abilityPT are the abilities in Portuguese, for the tags an effect chosen for one reads.
var abilityPT = map[string]string{"str": "Força", "dex": "Destreza", "con": "Constituição", "int": "Inteligência", "wis": "Sabedoria", "cha": "Carisma"}

// modifierTagsPT are the tags the modifiers of an effect add to the file's: the ability an
// Aprimorar Habilidade was cast for.
func modifierTagsPT(mods []rules.EffectModifier) []string {
	var out []string
	for _, m := range mods {
		if m.Kind != rules.ModifierCheckAdvantage {
			continue
		}
		for _, a := range m.Abilities {
			out = append(out, "Vantagem em testes de "+abilityPT[a])
		}
	}
	return out
}

// spentTagPT is the tag of an effect a roll used up while its clock still runs (Zombaria Viciosa:
// the attack roll it gave disadvantage to was made).
const spentTagPT = "Já usada em um ataque"

// spentTagsPT is the tag of a spent effect, none for one that is not.
func spentTagsPT(st playdb.CombatantState) []string {
	if st.SpentAt == nil {
		return nil
	}
	return []string{spentTagPT}
}

// enhanceNotesPT are what an Aprimorar Habilidade gives besides the advantage, which the app does
// not count and the master narrates (SRD 5.1, Enhance Ability).
var enhanceNotesPT = map[string]string{
	"con": "Resistência do Urso: 2d6 PV temporários, perdidos quando a magia acaba (o mestre concede).",
	"str": "Força do Touro: a capacidade de carga dobra.",
	"dex": "Graça do Gato: não sofre dano de queda de até 6 m se não estiver incapacitado.",
}

// narratedNotesPT are the lines of what the effect gives that the table plays by narration.
func narratedNotesPT(mods []rules.EffectModifier) []string {
	var out []string
	for _, m := range mods {
		if m.Kind != rules.ModifierCheckAdvantage {
			continue
		}
		for _, a := range m.Abilities {
			if note := enhanceNotesPT[a]; note != "" {
				out = append(out, note)
			}
		}
	}
	return out
}
