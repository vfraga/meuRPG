package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// Effects that last (RN-22, SRD 5.1: "Conditions", "Duration", "Concentration"): a
// spell, an action or something the master put on a combatant that stays for a while.
// An effect is a row of combatant_states of the kind 'effect' (the states of PM-07a
// share the table and the clock's columns), one for each target; the rows of one
// casting share a group and a source key, and the master reads them as one effect with
// several targets. The clock is the turn's (lasting_effects_clock.go), the saving
// throws they ask are reaction windows (lasting_effects_save.go), what a player may
// read is lasting_effects_view.go. This file keeps the rows: making them, ending them,
// and working out again what they leave on the combatant.

// The kind of the effects among the states, and the phases of a turn as the states keep
// them (combatant_states_phase_valid).
const (
	stateEffect   = "effect"
	phaseStartKey = "start_of_turn"
	phaseEndKey   = "end_of_turn"
)

// The kinds of session event the effects write (session_event_kinds, migration 00237).
const (
	eventLastingAdded      = "lasting_effect_added"
	eventLastingChanged    = "lasting_effect_changed"
	eventLastingEnded      = "lasting_effect_ended"
	eventLastingVisibility = "lasting_effect_visibility_changed"
	eventLastingSaved      = "lasting_effect_saved"
	eventLastingTriggered  = "lasting_effect_triggered"
	eventExhaustion        = "exhaustion_changed"
)

// The reasons an effect ended, as an event keeps them.
const (
	endDuration      = "duration"        // its time ran out
	endByMaster      = "ended_by_master" // the master ended it
	endSaved         = "saved"           // the target passed the saving throw that frees it
	endReplaced      = "replaced"        // the same spell was cast again (SRD, Combining Magical Effects)
	endBurned        = "burned"          // the web burned away
	endLeft          = "left"            // its target or its caster left the combat
	endCasterLeft    = "caster_left"     // internal: the caster left or died; the log keeps it as endLeft
	endIncapacitated = "incapacitated"   // the caster was incapacitated: no concentration
	endUsed          = "used"            // a roll spent it (Guidance, Resistance)
)

// lastingEvent is the payload of an effect's event: keys, ids and numbers, never a name
// or a free text (docs/privacy.md). Players never get a line whose Hidden is set.
type lastingEvent struct {
	Key     string   `json:"key"`
	Change  string   `json:"change"`
	Targets []string `json:"targets,omitempty"`
	Caster  string   `json:"caster,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Ability string   `json:"ability,omitempty"`
	D20     int32    `json:"d20,omitempty"`
	Total   int32    `json:"total,omitempty"`
	DC      int32    `json:"dc,omitempty"`
	Amount  int32    `json:"amount,omitempty"`
	DType   string   `json:"damage_type,omitempty"`
	Level   int32    `json:"level,omitempty"`
	Before  int32    `json:"before,omitempty"`
	Rounds  int32    `json:"rounds,omitempty"`
	// Effects are the ids of the rows it concerns.
	Effects []string `json:"effects,omitempty"`
	// OwnerOnly says only the targets' players read the line (an effect for its owner alone,
	// the owner-class conditions, exhaustion): the master reads it always.
	OwnerOnly bool `json:"owner_only,omitempty"`
}

// isEffect says a state is an effect that lasts.
func isEffect(st playdb.CombatantState) bool { return st.Kind == stateEffect }

// effectModifiers are the modifiers an effect makes.
func effectModifiers(st playdb.CombatantState) []rules.EffectModifier {
	var out []rules.EffectModifier
	_ = json.Unmarshal(st.Modifiers, &out) // a value this package wrote
	return out
}

// effectsOn are the effects on a combatant, from the states of a combat.
func effectsOn(states map[string][]playdb.CombatantState, id string) []playdb.CombatantState {
	var out []playdb.CombatantState
	for _, st := range states[id] {
		if isEffect(st) {
			out = append(out, st)
		}
	}
	return out
}

// hasEffect says the combatant has an effect of the source key.
func hasEffect(states map[string][]playdb.CombatantState, id, key string) bool {
	return slices.ContainsFunc(states[id], func(st playdb.CombatantState) bool { return isEffect(st) && deref(st.SourceKey) == key })
}

// durationSpec is how long a new effect lasts.
type durationSpec struct {
	Kind   string
	Rounds int32
	Anchor string // the combatant whose turn it is anchored to ("" for the default)
}

// effectSpec is an effect about to be made, for each of its targets.
type effectSpec struct {
	key        string
	sourceKind string
	def        *rules.EffectDef
	caster     *playdb.Combatant
	group      string
	targets    []playdb.Combatant
	dur        durationSpec
	// concentration says the caster's concentration holds it.
	concentration bool
	dc            *int32
	visible       *bool
	audience      string
	label         string
	// conditions and modifiers replace the definition's when set.
	conditions []string
	modifiers  []rules.EffectModifier
	follows    string
}

// nextTurnRound is the round of a combatant's next turn: this round when its turn has
// not come yet, the next one when it has (or is running). Before the combat begins its
// first turn is in round 1.
func nextTurnRound(enc playdb.Encounter, cs []playdb.Combatant, anchor string) int32 {
	round := max(enc.Round, 1)
	if enc.Status != statusActive {
		return round
	}
	runs := groupRuns(cs)
	groupOf := func(pred func(playdb.Combatant) bool) int {
		return slices.IndexFunc(runs, func(g []playdb.Combatant) bool { return slices.ContainsFunc(g, pred) })
	}
	at := groupOf(func(c playdb.Combatant) bool { return c.ID == anchor })
	current := groupOf(func(c playdb.Combatant) bool { return c.TurnState != turnIdle })
	if at > current {
		return round
	}
	return round + 1
}

// endsOf works out when an effect ends on its target's row: the round, the combatant
// whose turn it is and the phase; all nil for one with no end on the clock.
func endsOf(enc playdb.Encounter, cs []playdb.Combatant, d durationSpec, caster *playdb.Combatant, target playdb.Combatant) (round *int32, who, phase *string) {
	switch d.Kind {
	case rules.EffectDurationRounds:
		anchor := d.Anchor
		if anchor == "" {
			anchor = target.ID
			if caster != nil {
				anchor = caster.ID
			}
		}
		return new(max(enc.Round, 1) + d.Rounds), &anchor, new(phaseStartKey)
	case rules.EffectDurationUntilStartOfTurnOf, rules.EffectDurationUntilEndOfTurnOf:
		anchor := d.Anchor
		if anchor == "" {
			anchor = target.ID
		}
		phase := phaseStartKey
		if d.Kind == rules.EffectDurationUntilEndOfTurnOf {
			phase = phaseEndKey
		}
		return new(nextTurnRound(enc, cs, anchor)), &anchor, &phase
	}
	return nil, nil, nil
}

// endsAtOf is the clock of a row, for the pure arithmetic.
func endsAtOf(st playdb.CombatantState) combat.EndsAt {
	if st.EndsRound == nil {
		return combat.EndsAt{}
	}
	phase := combat.PhaseStart
	if deref(st.EndsPhase) == phaseEndKey {
		phase = combat.PhaseEnd
	}
	return combat.EndsAt{Round: int(*st.EndsRound), CombatantID: deref(st.EndsCombatantID), Phase: phase}
}

func (s *Service) contentOf(ctx context.Context, c *combatTx) (*rules.Content, error) {
	return s.roster.RulesContent(ctx, c.tx, c.session.CampaignID)
}

// addEffects makes the effect on each of its targets, ends the same spell on them when it
// was already there (SRD, "Combining Magical Effects": the effects of the same spell do not
// add up) and works out what the targets are now. It writes no event: the caller says what
// it did.
func (s *Service) addEffects(ctx context.Context, c *combatTx, cs []playdb.Combatant, spec effectSpec) ([]playdb.CombatantState, error) {
	if len(spec.targets) == 0 {
		return nil, nil
	}
	if spec.group == "" {
		spec.group = uuid.New().String()
	}
	def := spec.def
	conditions, mods := spec.conditions, spec.modifiers
	if def != nil {
		if conditions == nil {
			conditions = def.Conditions
		}
		if mods == nil {
			mods = def.Modifiers
		}
	}
	body, err := json.Marshal(append([]rules.EffectModifier{}, mods...))
	if err != nil {
		return nil, fmt.Errorf("encode the modifiers: %w", err)
	}
	visible, audience := true, rules.EffectVisibilityPublic
	if def != nil && def.Visibility != "" {
		audience = def.Visibility
	}
	if spec.audience != "" {
		audience = spec.audience
	}
	if spec.visible != nil {
		visible = *spec.visible
	}
	dbAudience := "all"
	if audience == rules.EffectVisibilityOwner || audience == "owner" {
		dbAudience = "owner"
	}
	var label *string
	if spec.label != "" {
		label = &spec.label
	}
	var casterID *string
	if spec.caster != nil {
		casterID = &spec.caster.ID
	}
	var rows []playdb.CombatantState
	for _, target := range spec.targets {
		if spec.sourceKind == "spell" {
			if err := s.dropSameSpell(ctx, c, cs, target.ID, spec.key, spec.group); err != nil {
				return nil, err
			}
		}
		round, who, phase := endsOf(c.enc, cs, spec.dur, spec.caster, target)
		p := playdb.InsertLastingEffectParams{
			EncounterID: c.enc.ID, CombatantID: target.ID, StartedRound: max(c.enc.Round, 1), CreatedAt: c.now,
			GroupID: &spec.group, SourceKey: &spec.key, SourceKind: &spec.sourceKind, Concentration: spec.concentration,
			ConditionKeys: append([]string{}, conditions...), Modifiers: body, DurationKind: &spec.dur.Kind,
			PlayerVisible: visible, Audience: dbAudience, SourceID: casterID,
			EndsRound: round, EndsCombatantID: who, EndsPhase: phase, SaveDc: spec.dc, PlayerLabel: label,
		}
		if spec.follows != "" {
			p.FollowsKey = &spec.follows
		}
		if def != nil {
			if def.EndSave != nil {
				p.EndSaveAbility = &def.EndSave.Ability
			}
			if def.StartSave != nil {
				p.StartSaveAbility = &def.StartSave.Ability
				if def.StartSave.OnFail != "" {
					p.OnFailEffect = &def.StartSave.OnFail
				}
			}
			if t := def.Trigger; t != nil {
				p.TriggerDice, p.TriggerDamageType, p.TriggerMaxTriggers = &t.Dice, &t.DamageType, new(int32(t.MaxTriggers)) //nolint:gosec // a small number of the file
			}
		}
		row, err := c.q.InsertLastingEffect(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("make the effect: %w", err)
		}
		rows = append(rows, row)
	}
	ids := make([]string, len(spec.targets))
	for i, t := range spec.targets {
		ids[i] = t.ID
	}
	return rows, s.refreshCombatants(ctx, c, ids...)
}

// dropSameSpell ends, on a target, the effect of the same spell another casting left
// (SRD 5.1, "Combining Magical Effects": the effects of the same spell cast several times
// do not combine).
func (s *Service) dropSameSpell(ctx context.Context, c *combatTx, cs []playdb.Combatant, target, key, group string) error {
	rows, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	old := slices.DeleteFunc(rows, func(st playdb.CombatantState) bool {
		return st.CombatantID != target || deref(st.SourceKey) != key || deref(st.GroupID) == group
	})
	return s.endEffectRows(ctx, c, cs, old, endReplaced)
}

// refreshCombatants works out again what the effects leave on each combatant: the
// conditions (the ones set by hand stay, the effects' come and go), the armor class, the
// speed, and whether it can act and move; and the exhaustion it carries (a character's is
// its vitals').
func (s *Service) refreshCombatants(ctx context.Context, c *combatTx, ids ...string) error { //nolint:gocognit,gocyclo // the steps of one transaction in one closure, like the other writes of the combat
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	rows, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	var players []string // the characters whose base armor class is worked out again
	for _, id := range ids {
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == id })
		if i < 0 {
			continue
		}
		cb := cs[i]
		level := int(cb.ExhaustionLevel)
		if cb.Kind == kindPlayer {
			vit, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, cb.CharacterID)
			if err != nil && connect.CodeOf(err) != connect.CodeNotFound {
				return err
			}
			if err == nil {
				level = int(vit.GetExhaustionLevel())
				if int32(level) != cb.ExhaustionLevel { //nolint:gosec // 0 to 6
					if err := c.q.SetCombatantExhaustionLevel(ctx, playdb.SetCombatantExhaustionLevelParams{ID: cb.ID, ExhaustionLevel: int32(level)}); err != nil { //nolint:gosec // 0 to 6
						return fmt.Errorf("copy the exhaustion: %w", err)
					}
				}
			}
		}
		var derived []string
		var mods []rules.EffectModifier
		for _, st := range rows {
			if st.CombatantID != id {
				continue
			}
			for _, k := range st.ConditionKeys {
				if !slices.Contains(derived, k) {
					derived = append(derived, k)
				}
			}
			mods = append(mods, effectModifiers(st)...)
		}
		if level > 0 && !slices.Contains(derived, "condition:exhaustion") {
			derived = append(derived, "condition:exhaustion")
		}
		base := slices.DeleteFunc(slices.Clone(cb.Conditions), func(k string) bool { return slices.Contains(cb.EffectConditions, k) })
		conds := slices.Clone(base)
		for _, k := range derived {
			if !slices.Contains(conds, k) {
				conds = append(conds, k)
			}
		}
		pct := combat.SpeedPercent(mods)
		if combat.ExhaustionAt(level).SpeedHalved {
			pct /= 2
		}
		ac := combat.ArmorClassBonus(mods)
		// Mage Armor: the base armor class the effects give, which the combat's rolls read.
		if want := int32(combat.BaseAC(mods)); (cb.MageArmorAc == nil && want != 0) || (cb.MageArmorAc != nil && *cb.MageArmorAc != want) { //nolint:gosec // 0 to 60
			var value *int32
			if want > 0 {
				value = &want
			}
			if err := c.q.SetCombatantMageArmorAC(ctx, playdb.SetCombatantMageArmorACParams{ID: cb.ID, MageArmorAc: value}); err != nil {
				return fmt.Errorf("work out the base armor class: %w", err)
			}
			cs[i].MageArmorAc = value
		}
		if cb.Kind == kindPlayer {
			players = append(players, cb.CharacterID)
		}
		noAction := slices.ContainsFunc(mods, func(m rules.EffectModifier) bool { return m.Kind == rules.ModifierNoAction })
		noReaction := slices.ContainsFunc(mods, func(m rules.EffectModifier) bool { return m.Kind == rules.ModifierNoReaction })
		// Speed 0 (exhaustion 5, the lethargy) is no movement; a percentage of 0 is never stored.
		noMove := ex0(level) || slices.ContainsFunc(mods, func(m rules.EffectModifier) bool { return m.Kind == rules.ModifierNoMove })
		if derived == nil {
			derived = []string{}
		}
		if conds == nil {
			conds = []string{}
		}
		if slices.Equal(conds, cb.Conditions) && slices.Equal(derived, cb.EffectConditions) && int(cb.EffectAcBonus) == ac && int(cb.EffectSpeedPct) == pct && cb.EffectNoAction == noAction && cb.EffectNoReaction == noReaction && cb.EffectNoMove == noMove && int(cb.EffectSpeedAddFt) == combat.SpeedAddFt(mods) {
			continue
		}
		if err := c.q.SetCombatantEffectState(ctx, playdb.SetCombatantEffectStateParams{
			ID: cb.ID, Conditions: conds, EffectConditions: derived, EffectAcBonus: int32(ac), EffectSpeedPct: int32(pct), EffectNoAction: noAction, EffectNoReaction: noReaction, EffectNoMove: noMove, EffectSpeedAddFt: int32(combat.SpeedAddFt(mods)), //nolint:gosec // small numbers
		}); err != nil {
			return fmt.Errorf("work out the effects' state: %w", err)
		}
		cs[i].Conditions, cs[i].EffectConditions = conds, derived
		// A caster that cannot act loses the concentration (SRD, Concentration:
		// incapacitated).
		if cb.ConcentrationSpell != nil && (combat.Creature{Conditions: conds}).Incapacitated() {
			ev := actionEvent{Round: c.enc.Round, Secret: cb.Hidden, Actor: cb.ID}
			if err := s.stopConcentrating(ctx, c, cs[i], &ev); err != nil {
				return err
			}
			if err := insertEvent(ctx, c, eventConditionsSet, &c.actorUserID, nil, ev); err != nil {
				return err
			}
		}
	}
	return s.syncArmorBase(ctx, c, players...)
}

// endEffectRows ends effects: it closes the saving throws they ask, takes the rows away,
// gives what follows (the lethargy a Velocidade leaves, the web that burns away freeing
// its prisoners), writes a line for each casting that ended and works out what the
// targets are now. The rows are all of the encounter's.
func (s *Service) endEffectRows(ctx context.Context, c *combatTx, cs []playdb.Combatant, rows []playdb.CombatantState, reason string) error {
	if len(rows) == 0 {
		return nil
	}
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return err
	}
	type casting struct {
		key, group string
		rows       []playdb.CombatantState
	}
	var castings []*casting
	for _, st := range rows {
		i := slices.IndexFunc(castings, func(k *casting) bool { return k.group == deref(st.GroupID) && k.key == deref(st.SourceKey) })
		if i < 0 {
			castings = append(castings, &casting{key: deref(st.SourceKey), group: deref(st.GroupID)})
			i = len(castings) - 1
		}
		castings[i].rows = append(castings[i].rows, st)
	}
	followUp := reason
	if reason == endCasterLeft {
		reason = endLeft // the log and the windows know one reason for a leaver
	}
	touched := map[string]bool{}
	for _, k := range castings {
		var targets, ids []string
		for _, st := range k.rows {
			c.endedEffects = append(c.endedEffects, st)
			if err := s.closeEffectWindows(ctx, c, st, reason); err != nil {
				return err
			}
			if err := c.q.DeleteLastingEffect(ctx, st.ID); err != nil {
				return fmt.Errorf("end the effect: %w", err)
			}
			targets, ids = append(targets, st.CombatantID), append(ids, st.ID)
			touched[st.CombatantID] = true
		}
		if err := s.afterEffectEnded(ctx, c, cs, content, k.rows, followUp); err != nil {
			return err
		}
		first := k.rows[0]
		hidden := s.effectHiddenFrom(first, targetsOf(cs, targets))
		line, err := insertEventID(ctx, c, eventLastingEnded, &c.actorUserID, nil, actionEvent{
			Round: c.enc.Round, Secret: hidden, Actor: deref(first.SourceID), Target: targets[0],
			Lasting: &lastingEvent{Key: k.key, Change: "ended", Targets: targets, Reason: reason, Effects: ids, OwnerOnly: first.Audience == audienceOwner},
		})
		if err != nil {
			return err
		}
		// The change's own event names the line, so that its undo, which puts the rows
		// back, takes the line back too (write, UndoLastAction).
		c.endedLines = append(c.endedLines, line)
	}
	ids := make([]string, 0, len(touched))
	for id := range touched {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return s.refreshCombatants(ctx, c, ids...)
}

// targetsOf are the combatants of ids.
func targetsOf(cs []playdb.Combatant, ids []string) []playdb.Combatant {
	var out []playdb.Combatant
	for _, id := range ids {
		if i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == id }); i >= 0 {
			out = append(out, cs[i])
		}
	}
	return out
}

// afterEffectEnded does what follows an effect that ended on its targets.
func (s *Service) afterEffectEnded(ctx context.Context, c *combatTx, cs []playdb.Combatant, content *rules.Content, rows []playdb.CombatantState, reason string) error {
	first := rows[0]
	key := deref(first.SourceKey)
	def, ok := s.effectDef(content, key)
	if !ok {
		return nil
	}
	if reason == endLeft || reason == endReplaced {
		return nil
	}
	for _, st := range rows {
		if def.OnEnd != "" {
			next, ok := content.CombatEffect(def.OnEnd)
			i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == st.CombatantID })
			if ok && i >= 0 && !cs[i].Defeated {
				var caster *playdb.Combatant
				if j := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == deref(st.SourceID) }); j >= 0 {
					caster = &cs[j]
				}
				made, err := s.addEffects(ctx, c, cs, effectSpec{
					key: def.OnEnd, sourceKind: "feature", def: next, caster: caster, targets: []playdb.Combatant{cs[i]},
					dur: durationSpec{Kind: next.Duration.Kind}, follows: key,
				})
				if err != nil {
					return err
				}
				if err := s.addedEvent(ctx, c, cs, made, def.OnEnd); err != nil {
					return err
				}
			}
		}
		// The temporary hit points the spell gave go with it (SRD 5.1, Heroism).
		if slices.ContainsFunc(def.Modifiers, func(m rules.EffectModifier) bool { return m.Kind == rules.ModifierTurnTempHP }) {
			if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == st.CombatantID }); i >= 0 {
				if err := s.clearTempHP(ctx, c, cs[i]); err != nil {
					return err
				}
			}
		}
		if len(def.EndsEffectsOf) > 0 {
			all, err := c.q.ListLastingEffects(ctx, c.enc.ID)
			if err != nil {
				return fmt.Errorf("list the effects: %w", err)
			}
			burned := slices.DeleteFunc(all, func(o playdb.CombatantState) bool {
				return o.CombatantID != st.CombatantID || !slices.Contains(def.EndsEffectsOf, deref(o.SourceKey)) || o.ID == st.ID
			})
			if err := s.endEffectRows(ctx, c, cs, burned, endBurned); err != nil {
				return err
			}
		}
	}
	return nil
}

// effectDef is the catalog entry of an effect's key: a spell's or an app effect's.
func (s *Service) effectDef(content *rules.Content, key string) (*rules.EffectDef, bool) {
	if d, ok := content.CombatSpellEffect(key); ok {
		return d, true
	}
	return content.CombatEffect(key)
}

// addedEvent writes the line of an effect that began: one for the casting.
func (s *Service) addedEvent(ctx context.Context, c *combatTx, cs []playdb.Combatant, rows []playdb.CombatantState, key string) error {
	if len(rows) == 0 {
		return nil
	}
	var targets, ids []string
	for _, st := range rows {
		targets, ids = append(targets, st.CombatantID), append(ids, st.ID)
	}
	first := rows[0]
	var rounds int32
	if first.EndsRound != nil {
		rounds = max(*first.EndsRound-max(c.enc.Round, 1), 0)
	}
	return insertEvent(ctx, c, eventLastingAdded, &c.actorUserID, nil, actionEvent{
		Round: c.enc.Round, Secret: s.effectHiddenFrom(first, targetsOf(cs, targets)), Actor: deref(first.SourceID), Target: targets[0],
		Lasting: &lastingEvent{Key: key, Change: "added", Targets: targets, Caster: deref(first.SourceID), Rounds: rounds, Effects: ids, OwnerOnly: first.Audience == audienceOwner},
	})
}

// effectHiddenFrom says the players do not read a casting's lines: the master hides it,
// it is for its targets' players alone and one of them is not a player's character, or a
// target is hidden from them.
func (s *Service) effectHiddenFrom(st playdb.CombatantState, targets []playdb.Combatant) bool {
	if !st.PlayerVisible {
		return true
	}
	for _, t := range targets {
		if t.Hidden || (st.Audience == "owner" && !inParty(t)) {
			return true
		}
	}
	return false
}

// endConcentrationEffects ends the effects a caster's concentration holds (all of its
// castings): the caster stopped concentrating.
func (s *Service) endConcentrationEffects(ctx context.Context, c *combatTx, caster playdb.Combatant, reason string) error {
	rows, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	held := slices.DeleteFunc(rows, func(st playdb.CombatantState) bool { return !st.Concentration || deref(st.SourceID) != caster.ID })
	if len(held) == 0 {
		return nil
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	return s.endEffectRows(ctx, c, cs, held, reason)
}

// errEffectNotFound is the not_found of an effect that is not in the combat: the same for
// one that never was (RN-10).
func errEffectNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("effect not found"))
}

// windowReasonOf is how a window closes when its effect ended for the reason.
func windowReasonOf(reason string) reaction.Reason {
	if reason == endConcentration || reason == endIncapacitated {
		return reaction.ReasonCasterLostConcentration
	}
	return reaction.ReasonEffectEnded
}

// effectSaveAdvantage are the abilities the effects on a combatant give advantage on the saving
// throws of (Velocidade: Destreza).
func effectSaveAdvantage(states map[string][]playdb.CombatantState, id string) []string {
	var out []string
	for _, st := range effectsOn(states, id) {
		for _, m := range effectModifiers(st) {
			if m.Kind == rules.ModifierSaveAdvantage {
				out = append(out, m.Abilities...)
			}
		}
	}
	return out
}

// effectAttackDisadvantage is the key of the effect on a combatant that gives its attack rolls
// disadvantage (Zombaria Viciosa), "" for none. A spent one gives it no more: the attack roll it was
// for was made (spendAttackEffects).
func effectAttackDisadvantage(states map[string][]playdb.CombatantState, id string) string {
	for _, st := range effectsOn(states, id) {
		if st.SpentAt == nil && slices.ContainsFunc(effectModifiers(st), func(m rules.EffectModifier) bool { return m.Kind == rules.ModifierAttackDisadvantage }) {
			return deref(st.SourceKey)
		}
	}
	return ""
}

// effectCheckAdvantage are the abilities the effects on a combatant give advantage on the ability
// checks of (Aprimorar Habilidade).
func effectCheckAdvantage(states map[string][]playdb.CombatantState, id string) []string {
	var out []string
	for _, st := range effectsOn(states, id) {
		for _, m := range effectModifiers(st) {
			if m.Kind == rules.ModifierCheckAdvantage {
				out = append(out, m.Abilities...)
			}
		}
	}
	return out
}

// ex0 says the level of exhaustion takes the speed to 0 (SRD 5.1, Conditions: level 5).
func ex0(level int) bool { return combat.ExhaustionAt(level).SpeedZero }

// effectModsOf are the modifiers of every effect on a combatant.
func effectModsOf(states map[string][]playdb.CombatantState, id string) []rules.EffectModifier {
	var out []rules.EffectModifier
	for _, st := range effectsOn(states, id) {
		out = append(out, effectModifiers(st)...)
	}
	return out
}
