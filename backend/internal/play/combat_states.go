package play

import (
	"context"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The states of a combatant (PM-07a): a rage, a Hunter's Mark on a target, the Dodge
// action, Reckless Attack. They are rows of combatant_states with when they end;
// the advantage rules, the damage extras and the resistances read them, the combat's
// view draws them as chips. The master decides nothing about them: the server
// begins and ends them as the SRD says.

// rageRounds is how long a rage lasts: 1 minute is 10 rounds (SRD 5.1, Barbarian).
const rageRounds = 10

// The chips: a state's name on the screen and what it does. "Em fúria", "Esquivando",
// "Marcado" and "Ataque descuidado" are screen labels, not entries of names_pt.json.
func stateLabelPT(kind string) string {
	switch kind {
	case stateRage:
		return "Em fúria"
	case stateDodging:
		return "Esquivando"
	case stateMark:
		return "Marcado"
	case stateReckless:
		return "Ataque descuidado"
	}
	return ""
}

func stateKindProto(kind string) playv1.CombatantStateKind {
	switch kind {
	case stateRage:
		return playv1.CombatantStateKind_COMBATANT_STATE_KIND_RAGE
	case stateMark:
		return playv1.CombatantStateKind_COMBATANT_STATE_KIND_HUNTERS_MARK_TARGET
	case stateDodging:
		return playv1.CombatantStateKind_COMBATANT_STATE_KIND_DODGING
	case stateReckless:
		return playv1.CombatantStateKind_COMBATANT_STATE_KIND_RECKLESS
	}
	return playv1.CombatantStateKind_COMBATANT_STATE_KIND_UNSPECIFIED
}

// stateEffectPT says what a state does, in Portuguese.
func stateEffectPT(st playdb.CombatantState) string {
	switch st.Kind {
	case stateRage:
		bonus := max(st.Amount, 2)
		text := fmt.Sprintf("Resistência a concussão, perfurante e cortante · +%d no dano corpo a corpo com Força · vantagem em testes e testes de resistência de Força · não conjura nem se concentra. Acaba no fim da vez se não atacar uma criatura hostil nem sofrer dano.", bonus)
		if frenzied(st) {
			text += " Em frenesi: um ataque corpo a corpo como ação bônus em cada turno seguinte; quando a fúria acabar, ganha 1 nível de exaustão."
		}
		return text
	case stateDodging:
		return "Ataques contra a criatura têm desvantagem se quem ataca é visto, e ela tem vantagem em testes de resistência de Destreza. Acaba no começo da próxima vez dela."
	case stateReckless:
		return "Vantagem nos ataques corpo a corpo com Força neste turno; ataques contra a criatura têm vantagem até o começo da próxima vez dela."
	case stateMark:
		return "Marcado pela Marca do Caçador: dano extra a cada acerto de arma de quem o marcou, enquanto a concentração durar."
	}
	return ""
}

// addState begins a state of who. A combatant has one state of a kind from a source
// at a time: the earlier one goes first.
func (s *Service) addState(ctx context.Context, c *combatTx, who playdb.Combatant, kind string, source, endsCombatant *string, phase *string, endsRound *int32, amount int32) (playdb.CombatantState, error) {
	if err := s.dropState(ctx, c, who.ID, kind); err != nil {
		return playdb.CombatantState{}, err
	}
	st, err := c.q.InsertCombatantState(ctx, playdb.InsertCombatantStateParams{
		EncounterID: c.enc.ID, CombatantID: who.ID, Kind: kind, SourceID: source, EndsCombatantID: endsCombatant, EndsPhase: phase,
		EndsRound: endsRound, StartedRound: c.enc.Round, Amount: amount, CreatedAt: c.now,
	})
	if err != nil {
		return playdb.CombatantState{}, fmt.Errorf("begin the state: %w", err)
	}
	return st, nil
}

// dropState ends the states of the kind a combatant has, without a line.
func (s *Service) dropState(ctx context.Context, c *combatTx, id, kind string) error {
	if _, err := c.q.DeleteStatesOfCombatant(ctx, playdb.DeleteStatesOfCombatantParams{CombatantID: id, Kind: kind}); err != nil {
		return fmt.Errorf("end the state: %w", err)
	}
	return nil
}

// stateEvent writes the line of a state that began or ended.
func (s *Service) stateEvent(ctx context.Context, c *combatTx, who playdb.Combatant, kind string, started bool, reason string) error {
	return insertEvent(ctx, c, eventStateChanged, &c.actorUserID, nil, actionEvent{
		Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, StateKind: kind, StateStarted: started, StateReason: reason,
	})
}

// beginRage starts a rage, in a frenzy when the Berserker asks for it (SRD 5.1, Barbarian,
// Path of the Berserker): it lasts 10 rounds and a raging character neither casts nor
// concentrates, so the concentration it had ends (SRD 5.1, Barbarian, Rage). It
// returns the state for the action's event, to undo it.
func (s *Service) beginRage(ctx context.Context, c *combatTx, who playdb.Combatant, sheet link.Sheet, frenzy bool, ev *actionEvent) error {
	if frenzy && !sheet.Traits.Frenzy {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_FRENZY_UNAVAILABLE, "only a Berserker has Frenzy")
	}
	ends := c.enc.Round + rageRounds
	phase := (*string)(nil)
	var source *string // a frenzied rage is its own source (see frenzied)
	if frenzy {
		source = &who.ID
	}
	st, err := s.addState(ctx, c, who, stateRage, source, nil, phase, &ends, clamp32(combat.RageBonus(sheet.Traits.BarbarianLevel), 0, 10))
	if err != nil {
		return err
	}
	ev.StateID = st.ID
	if who.ConcentrationSpell != nil {
		if err := s.stopConcentrating(ctx, c, who, ev); err != nil {
			return err
		}
	}
	return nil
}

// endRage ends a combatant's rage with a line that says why: "no_attack" (the turn ended
// with no attack on a hostile creature and no damage taken), "unconscious", "by_choice"
// (the bonus action), "duration" (the minute is over) or "left" (the combat ended).
func (s *Service) endRage(ctx context.Context, c *combatTx, who playdb.Combatant, reason string) error {
	rows, err := c.q.DeleteStatesOfCombatant(ctx, playdb.DeleteStatesOfCombatantParams{CombatantID: who.ID, Kind: stateRage})
	if err != nil {
		return fmt.Errorf("end the rage: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	if err := c.q.SetCombatantRageFlags(ctx, playdb.SetCombatantRageFlagsParams{ID: who.ID}); err != nil {
		return fmt.Errorf("clear the rage flags: %w", err)
	}
	if err := s.stateEvent(ctx, c, who, stateRage, false, reason); err != nil {
		return err
	}
	return s.afterFrenzyEnded(ctx, c, who, rows)
}

// markHuntersMark puts the caster's Hunter's Mark on the target: one mark at a time (SRD
// 5.1, Hunter's Mark), and it ends with the concentration. The cast's event keeps a mark it
// takes off (clearHuntersMark).
func (s *Service) markHuntersMark(ctx context.Context, c *combatTx, caster, target playdb.Combatant, ev *actionEvent) error {
	if err := s.clearHuntersMark(ctx, c, caster, ev); err != nil {
		return err
	}
	_, err := c.q.InsertCombatantState(ctx, playdb.InsertCombatantStateParams{
		EncounterID: c.enc.ID, CombatantID: target.ID, Kind: stateMark, SourceID: &caster.ID, StartedRound: c.enc.Round, CreatedAt: c.now,
	})
	if err != nil {
		return fmt.Errorf("mark the target: %w", err)
	}
	return nil
}

// clearHuntersMark takes the caster's marks off: its concentration ended. ev, when not nil,
// is the event of a cast, which keeps the marks taken off (MarksBefore) so that its undo
// puts them back (marksBack, combat_undo_kinds.go).
func (s *Service) clearHuntersMark(ctx context.Context, c *combatTx, caster playdb.Combatant, ev *actionEvent) error {
	rows, err := c.q.DeleteCombatantStatesBySource(ctx, playdb.DeleteCombatantStatesBySourceParams{SourceID: &caster.ID, Kind: stateMark})
	if err != nil {
		return fmt.Errorf("take the marks off: %w", err)
	}
	if ev == nil {
		return nil
	}
	for _, r := range rows {
		ev.MarksBefore = append(ev.MarksBefore, markState{ID: r.ID, Source: caster.ID, Target: r.CombatantID, Round: r.StartedRound, CreatedAt: r.CreatedAt})
	}
	return nil
}

// refuseWhileRaging refuses a spell to a raging character (SRD 5.1, Barbarian, Rage).
// The master has the last word.
func (s *Service) refuseWhileRaging(ctx context.Context, c *combatTx, who playdb.Combatant, master bool) error {
	if master {
		return nil
	}
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return err
	}
	if hasState(states, who.ID, stateRage) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_RAGING_CANNOT_CAST, "a raging character casts no spells")
	}
	return nil
}

// ---- turns ----

// endStatesAtStart ends what a turn starting ends: the dodge and the reckless attack of
// the combatants whose turn starts, the states that end at the start of their turn, and
// the rages whose minute is over (a line each for a rage).
func (s *Service) endStatesAtStart(ctx context.Context, c *combatTx, ids []string, round int32) error {
	rows, err := c.q.DeleteExpiredCombatantStates(ctx, playdb.DeleteExpiredCombatantStatesParams{EncounterID: c.enc.ID, Column2: ids, EndsRound: &round})
	if err != nil {
		return fmt.Errorf("end the states: %w", err)
	}
	return s.afterStatesEnded(ctx, c, rows, "duration")
}

// afterStatesEnded writes the line of each rage among the states that ended, takes the
// condition a state gave off its combatant and returns.
func (s *Service) afterStatesEnded(ctx context.Context, c *combatTx, rows []playdb.CombatantState, reason string) error {
	if len(rows) == 0 {
		return nil
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	for _, st := range rows {
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == st.CombatantID })
		if i < 0 || st.Kind != stateRage {
			continue
		}
		if err := c.q.SetCombatantRageFlags(ctx, playdb.SetCombatantRageFlagsParams{ID: st.CombatantID}); err != nil {
			return fmt.Errorf("clear the rage flags: %w", err)
		}
		if err := s.stateEvent(ctx, c, cs[i], stateRage, false, reason); err != nil {
			return err
		}
		if err := s.afterFrenzyEnded(ctx, c, cs[i], []playdb.CombatantState{st}); err != nil {
			return err
		}
	}
	return nil
}

// endOfTurn settles what the end of a combatant's part of the turn does. It reports
// whether the turn must wait for the player's answer about the rage (SRD 5.1,
// Barbarian, Rage: it ends if the turn ends and the barbarian has not attacked a
// hostile creature since its last turn or taken damage since). The master ending the
// turn applies the rule himself.
func (s *Service) endOfTurn(ctx context.Context, c *combatTx, v combatViewer, current playdb.Combatant) (wait bool, err error) {
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return false, err
	}
	if hasState(states, current.ID, stateRage) && !current.AttackedHostile && !current.TookDamage {
		if !v.master {
			if !current.RageEndPending {
				if err := c.q.SetCombatantRageFlags(ctx, playdb.SetCombatantRageFlagsParams{ID: current.ID, RageEndPending: true}); err != nil {
					return false, fmt.Errorf("ask about the rage: %w", err)
				}
				if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
					return false, fmt.Errorf("touch the encounter: %w", err)
				}
			}
			return true, nil
		}
		if err := s.endRage(ctx, c, current, "no_attack"); err != nil {
			return false, err
		}
	}
	return false, nil
}

// afterTurnPart does what a combatant's part of the turn ending leaves: the states that
// end now go, the player's open requests for a mode close, and the rage flags start
// counting again.
func (s *Service) afterTurnPart(ctx context.Context, c *combatTx, current playdb.Combatant) error {
	rows, err := c.q.DeleteEndedCombatantStates(ctx, playdb.DeleteEndedCombatantStatesParams{EncounterID: c.enc.ID, EndsCombatantID: &current.ID})
	if err != nil {
		return fmt.Errorf("end the states: %w", err)
	}
	if err := s.afterStatesEnded(ctx, c, rows, "duration"); err != nil {
		return err
	}
	if err := s.dropConditionSourcesEndingNow(ctx, c, current); err != nil {
		return err
	}
	// A request for a better mode lives for the turn it was made in.
	if err := closeRequestsOf(ctx, c, current.ID); err != nil {
		return err
	}
	if current.AttackedHostile || current.TookDamage || current.RageEndPending {
		if err := c.q.SetCombatantRageFlags(ctx, playdb.SetCombatantRageFlagsParams{ID: current.ID}); err != nil {
			return fmt.Errorf("reset the rage flags: %w", err)
		}
	}
	return nil
}

// AnswerRageEnd implements playv1connect.CombatServiceHandler: the barbarian's player
// answers "A sua fúria vai acabar?". Letting it end ends the rage; the screen then
// ends the turn. Going on drops the question: the barbarian attacks.
func (s *Service) AnswerRageEnd(
	ctx context.Context,
	req *connect.Request[playv1.AnswerRageEndRequest],
) (*connect.Response[playv1.AnswerRageEndResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	endRage := req.Msg.GetEndRage()
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventStateChanged, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v := c.viewer(m, cs)
		who, err := findCombatant(cs, combID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(who); err != nil {
			return nil, err
		}
		if !who.RageEndPending {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_RAGE_END_NOT_PENDING, "no question waits")
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &who.CharacterID
		if err := c.q.SetCombatantRageFlags(ctx, playdb.SetCombatantRageFlagsParams{ID: who.ID, AttackedHostile: who.AttackedHostile, TookDamage: who.TookDamage}); err != nil {
			return nil, fmt.Errorf("drop the question: %w", err)
		}
		if !endRage {
			return nil, nil // the turn goes on: nothing to write down
		}
		// The rage ends: its own line.
		who.RageEndPending = false
		if err := s.endRage(ctx, c, who, "no_attack"); err != nil {
			return nil, err
		}
		return actionEvent{Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, StateKind: stateRage, StateReason: "no_attack"}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer the rage question", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AnswerRageEndResponse{Encounter: out}), nil
}

// EndRage implements playv1connect.CombatServiceHandler: a bonus action on the
// barbarian's own turn ends the rage (SRD 5.1, Barbarian, Rage).
func (s *Service) EndRage(
	ctx context.Context,
	req *connect.Request[playv1.EndRageRequest],
) (*connect.Response[playv1.EndRageResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventStateChanged, encounterID: encID}, func(c *combatTx) (any, error) {
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v := c.viewer(m, cs)
		who, err := findCombatant(cs, combID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(who); err != nil {
			return nil, err
		}
		if err := s.mustActNow(ctx, c, who); err != nil {
			return nil, err
		}
		states, err := s.readStates(ctx, c.tx, c.enc.ID)
		if err != nil {
			return nil, err
		}
		if !hasState(states, who.ID, stateRage) {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_RAGING, "the combatant is not raging")
		}
		if who.BonusActionUsed && !v.master {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED, "the bonus action of this turn is used")
		}
		if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
			ID: who.ID, ActionUsed: who.ActionUsed, BonusActionUsed: true, ReactionUsed: who.ReactionUsed, Dashed: who.Dashed,
		}); err != nil {
			return nil, fmt.Errorf("spend the bonus action: %w", err)
		}
		if err := s.endRage(ctx, c, who, "by_choice"); err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &who.CharacterID
		return actionEvent{Round: c.enc.Round, Secret: who.Hidden, Actor: who.ID, StateKind: stateRage, StateReason: "by_choice", BonusBefore: who.BonusActionUsed}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "end the rage", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.EndRageResponse{Encounter: out}), nil
}

// ---- the view ----

// statesFor builds the states a viewer reads on a combatant. A Hunter's Mark is the
// master's and its caster's player's alone; the others are public chips (RN-20: an
// NPC's state is only the chip).
func statesFor(states []playdb.CombatantState, cs []playdb.Combatant, v combatViewer) []*playv1.CombatantEffect {
	var out []*playv1.CombatantEffect
	for _, st := range states {
		if isEffect(st) { // the effects that last have their own cards (lasting_effects_view.go)
			continue
		}
		source, _ := findByID(cs, deref(st.SourceID))
		if st.Kind == stateMark && !v.master && !v.owns(source) {
			continue
		}
		e := &playv1.CombatantEffect{
			Id: st.ID, Kind: stateKindProto(st.Kind), SourceId: deref(st.SourceID), LabelPt: stateLabelPT(st.Kind), EffectPt: stateEffectPT(st),
			EndsCombatantId: deref(st.EndsCombatantID), EndsRound: derefInt32(st.EndsRound),
		}
		if frenzied(st) {
			e.SourceId, e.LabelPt, e.Frenzy = "", "Em frenesi", true
		}
		switch deref(st.EndsPhase) {
		case "start_of_turn":
			e.EndsPhase = playv1.StatePhase_STATE_PHASE_START_OF_TURN
		case "end_of_turn":
			e.EndsPhase = playv1.StatePhase_STATE_PHASE_END_OF_TURN
		}
		if st.Kind == stateMark && source.ID != "" {
			e.SourceLabel = source.Label
			e.LabelPt = "Marcado por " + source.Label
		}
		out = append(out, e)
	}
	return out
}

func findByID(cs []playdb.Combatant, id string) (playdb.Combatant, bool) {
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == id })
	if i < 0 {
		return playdb.Combatant{}, false
	}
	return cs[i], true
}

func derefInt32(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

// ---- conditions with a source ----

// conditionSource says where a condition of a combatant comes from and when it ends.
type conditionSource struct {
	Condition string `json:"condition"`
	SourceID  string `json:"source_id,omitempty"`
	// EndsCombatantID and EndsPhase: it ends at the start or the end of this
	// combatant's turn.
	EndsCombatantID string `json:"ends_combatant_id,omitempty"`
	EndsPhase       string `json:"ends_phase,omitempty"`
}

func readConditionSources(raw []byte) []conditionSource {
	var out []conditionSource
	if len(raw) == 0 || jsonUnmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

// dropConditionSourcesEndingNow ends the conditions that end at the end of this
// combatant's turn (a Stunned from a Stunning Strike ends at the end of the monk's next
// turn): the condition leaves its combatant.
func (s *Service) dropConditionSourcesEndingNow(ctx context.Context, c *combatTx, current playdb.Combatant) error {
	return s.endConditionSources(ctx, c, func(cs conditionSource) bool {
		return cs.EndsCombatantID == current.ID && cs.EndsPhase == "end_of_turn"
	})
}

// endConditionSources ends the conditions whose source entry matches, in the whole combat.
func (s *Service) endConditionSources(ctx context.Context, c *combatTx, ends func(conditionSource) bool) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	for _, who := range cs {
		sources := readConditionSources(who.ConditionSources)
		if len(sources) == 0 {
			continue
		}
		var keep []conditionSource
		conditions := slices.Clone(who.Conditions)
		changed := false
		for _, src := range sources {
			if !ends(src) {
				keep = append(keep, src)
				continue
			}
			changed = true
			conditions = slices.DeleteFunc(conditions, func(k string) bool { return k == src.Condition })
		}
		if !changed {
			continue
		}
		if err := c.q.SetCombatantConditionSources(ctx, playdb.SetCombatantConditionSourcesParams{ID: who.ID, ConditionSources: mustJSON(append([]conditionSource{}, keep...))}); err != nil {
			return fmt.Errorf("end the condition's source: %w", err)
		}
		if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: conditions}); err != nil {
			return fmt.Errorf("end the condition: %w", err)
		}
	}
	return nil
}

// huntersMark is the spell of the ranger's mark.
const huntersMark = "spell:hunters-mark"

// reckless and rage are the feature actions that begin a state.
const (
	recklessAttackAction = "feature:reckless-attack"
	rageAction           = "feature:rage"
)

// beginFeatureState begins the state a feature action gives: a rage, or a reckless
// attack, which is declared with the first attack of the turn (SRD 5.1, Barbarian).
func (s *Service) beginFeatureState(ctx context.Context, c *combatTx, v combatViewer, who playdb.Combatant, actionKey string, sheet link.Sheet, frenzy bool, ev *actionEvent) error {
	switch actionKey {
	case rageAction:
		return s.beginRage(ctx, c, who, sheet, frenzy, ev)
	case recklessAttackAction:
		if who.AttacksMade > 0 && !v.master {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_RECKLESS_TOO_LATE, "Reckless Attack is declared with the first attack of the turn")
		}
		phase := "start_of_turn"
		st, err := s.addState(ctx, c, who, stateReckless, nil, &who.ID, &phase, nil, 0)
		if err != nil {
			return err
		}
		ev.StateID = st.ID
	}
	return nil
}

// afterDamageTaken settles a damage that landed on a combatant: a raging barbarian that
// took damage keeps its rage, and one that fell to 0 hit points is unconscious and rages
// no more (SRD 5.1, Barbarian, Rage).
func (s *Service) afterDamageTaken(ctx context.Context, c *combatTx, target playdb.Combatant, amount int32, before, after *hpState) error {
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return err
	}
	if !hasState(states, target.ID, stateRage) {
		return nil
	}
	if amount > 0 && (!target.TookDamage || target.RageEndPending) {
		if err := c.q.SetCombatantRageFlags(ctx, playdb.SetCombatantRageFlagsParams{ID: target.ID, AttackedHostile: target.AttackedHostile, TookDamage: true}); err != nil {
			return fmt.Errorf("note the damage taken: %w", err)
		}
	}
	if before != nil && after != nil && before.HP > 0 && after.HP == 0 {
		return s.endRage(ctx, c, target, "unconscious")
	}
	return nil
}

// afterConditionsSet runs when the master set a combatant's conditions: a barbarian made
// unconscious stops raging, and a condition that left has no source any more.
func (s *Service) afterConditionsSet(ctx context.Context, c *combatTx, target playdb.Combatant, conditions []string) error {
	if slices.Contains(conditions, "condition:unconscious") {
		if err := s.endRage(ctx, c, target, "unconscious"); err != nil {
			return err
		}
	}
	sources := readConditionSources(target.ConditionSources)
	if len(sources) == 0 {
		return nil
	}
	kept := slices.DeleteFunc(slices.Clone(sources), func(cs conditionSource) bool { return !slices.Contains(conditions, cs.Condition) })
	if len(kept) == len(sources) {
		return nil
	}
	if err := c.q.SetCombatantConditionSources(ctx, playdb.SetCombatantConditionSourcesParams{ID: target.ID, ConditionSources: mustJSON(append([]conditionSource{}, kept...))}); err != nil {
		return fmt.Errorf("drop the source of a condition: %w", err)
	}
	return nil
}

// conditionSourcesFor says where the conditions of a combatant that come from something
// come from and when they end, written for the viewer: a source it does not see is left out.
func conditionSourcesFor(who playdb.Combatant, cs []playdb.Combatant, v combatViewer, names func(string) string) []*playv1.ConditionSource {
	var out []*playv1.ConditionSource
	for _, src := range readConditionSources(who.ConditionSources) {
		source, _ := findByID(cs, src.SourceID)
		if source.ID != "" && !v.sees(source) {
			continue // what a player does not see never explains a condition (RN-10)
		}
		item := &playv1.ConditionSource{
			ConditionKey: src.Condition, SourceId: src.SourceID, SourceLabel: source.Label, EndsCombatantId: src.EndsCombatantID,
		}
		switch src.EndsPhase {
		case "start_of_turn":
			item.EndsPhase = playv1.StatePhase_STATE_PHASE_START_OF_TURN
		case "end_of_turn":
			item.EndsPhase = playv1.StatePhase_STATE_PHASE_END_OF_TURN
		}
		item.TextPt = names(src.Condition)
		if source.Label != "" {
			item.TextPt += " (" + source.Label + ")"
		}
		if ends, ok := findByID(cs, src.EndsCombatantID); ok && v.sees(ends) {
			when := "até o fim da vez de "
			if src.EndsPhase == "start_of_turn" {
				when = "até o começo da vez de "
			}
			item.TextPt += " · " + when + ends.Label
		}
		out = append(out, item)
	}
	return out
}

// rollModeRequestsFor lists the requests for a better mode as the viewer reads them: the
// master all, a player the ones of their own characters.
func (s *Service) rollModeRequestsFor(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer) []*playv1.RollModeRequest {
	var out []*playv1.RollModeRequest
	for _, r := range d.requests {
		who, ok := findByID(d.cs, r.CombatantID)
		if !ok || (!v.master && !v.owns(who)) {
			continue
		}
		name := ""
		if sheet, err := s.sheetOf(ctx, nil, m.CampaignID, who); err == nil {
			if i := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == r.AttackKey }); i >= 0 {
				name = sheet.Attacks[i].Name
			}
		}
		out = append(out, rollModeRequestProto(r, name))
	}
	return out
}
