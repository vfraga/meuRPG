package play

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The read side of the master's table of effects ("Efeitos em jogo", RN-22): the effects in play
// with their clocks, the concentrations, the loose conditions and the catalog the master adds
// from. The commands that change them are in lasting_effects_rpc.go.

// ListLastingEffects implements playv1connect.LastingEffectServiceHandler.
func (s *Service) ListLastingEffects(
	ctx context.Context,
	req *connect.Request[playv1.ListLastingEffectsRequest],
) (*connect.Response[playv1.ListLastingEffectsResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	// Without a combat only the catalog comes: what the master gives a character between fights.
	if req.Msg.GetEncounterId() == "" {
		content, err := s.roster.RulesContent(ctx, nil, m.CampaignID)
		if err != nil {
			return nil, s.dbError(ctx, "read the rules", err)
		}
		ev := &effectViewer{s: s, m: m, v: combatViewer{master: true}, content: content, names: s.namesFor(ctx, m.CampaignID)}
		return connect.NewResponse(&playv1.ListLastingEffectsResponse{Catalog: ev.catalog()}), nil
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	_, d, err := s.readEncounter(ctx, m.CampaignID, encID)
	if err != nil {
		return nil, err
	}
	names := s.namesFor(ctx, m.CampaignID)
	ev, err := s.effectViewerFor(ctx, m, d, combatViewer{master: true}, names)
	if err != nil {
		return nil, err
	}
	res := &playv1.ListLastingEffectsResponse{Round: d.enc.Round, CurrentCombatantId: deref(d.enc.CurrentCombatantID)}
	groups := groupsOf(d.states)
	for _, g := range groups {
		res.Effects = append(res.Effects, ev.card(g, ""))
	}
	res.Effects = append(res.Effects, ev.looseConditions()...)
	res.TurnClock = ev.turnClock(groups)
	res.Concentrations = ev.concentrations(groups)
	res.Catalog = ev.catalog()
	return connect.NewResponse(res), nil
}

// looseConditions lists the conditions the master marked by hand as effects with no origin and
// no end.
func (ev *effectViewer) looseConditions() []*playv1.LastingEffect {
	var out []*playv1.LastingEffect
	for _, c := range ev.d.cs {
		for _, k := range c.Conditions {
			if slices.Contains(c.EffectConditions, k) || k == conditionExhaustion {
				continue
			}
			info, _ := ev.content.ConditionInfo(k)
			out = append(out, &playv1.LastingEffect{
				Id: looseConditionPrefix + c.ID + ":" + k, EncounterId: ev.d.enc.ID, TargetIds: []string{c.ID}, TargetLabels: []string{c.Label},
				SourceKind: playv1.EffectSourceKind_EFFECT_SOURCE_KIND_CONDITION, SourceKey: k, SourceNamePt: ev.names(k), OriginPt: "Do mestre",
				ConditionKeys: []string{k}, ConditionNamesPt: []string{ev.names(k)}, DurationKind: playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_DISMISSED,
				ClockTextPt: "Dura até o mestre encerrar.", EndTextPt: "Até o mestre encerrar.", PlayerVisible: info.Visibility == rules.EffectVisibilityPublic,
				Audience: audienceProto(map[bool]string{true: "all", false: "owner"}[info.Visibility == rules.EffectVisibilityPublic]), ChangesPt: info.ChangesPT,
				PlayersSeePt: map[bool]string{true: ev.names(k), false: ""}[info.Visibility == rules.EffectVisibilityPublic],
			})
		}
	}
	return out
}

// clockKeyed is an entry of the turn clock with the key it is sorted by: the round, the place of
// the combatant in the order, and the phase (the start of a turn before its end).
type clockKeyed struct {
	round int32
	idx   int
	phase int
	e     *playv1.TurnClockEntry
}

// turnClockBuilder gathers the entries of the clock of a combat.
type turnClockBuilder struct {
	ev  *effectViewer
	all []clockKeyed
}

// add puts an entry on the clock of the combatant, if it is in the combat.
func (b *turnClockBuilder) add(r int32, who, text string, phase string, save bool, effect string) {
	i := slices.IndexFunc(b.ev.d.cs, func(c playdb.Combatant) bool { return c.ID == who })
	if i < 0 {
		return
	}
	p := 0
	if phase == combat.PhaseEnd {
		p = 1
	}
	b.all = append(b.all, clockKeyed{r, i, p, &playv1.TurnClockEntry{
		Round: r, CombatantId: who, CombatantLabel: b.ev.byID[who].Label, Phase: phaseProto(phase), IsSave: save, EffectId: effect, TextPt: text,
	}})
}

// addGroup puts the end of a casting and the saving throws of its targets on the clock.
func (b *turnClockBuilder) addGroup(g effectGroup) {
	f := g.first()
	name := b.ev.names(deref(f.SourceKey))
	b.addEnd(g, f, name)
	casterLabel := b.ev.byID[deref(f.SourceID)].Label
	for _, st := range g.rows {
		b.addSaves(st, name, casterLabel)
	}
}

// addEnd puts the end of a casting that ends at a turn on the clock.
func (b *turnClockBuilder) addEnd(g effectGroup, f playdb.CombatantState, name string) {
	e := endsAtOf(f)
	if !e.Timed() || e.CombatantID == "" {
		return
	}
	var targets []string
	for _, st := range g.rows {
		targets = append(targets, b.ev.byID[st.CombatantID].Label)
	}
	text := fmt.Sprintf("%s em %s acaba.", name, strings.Join(targets, ", "))
	if len(targets) == 1 && f.SourceID == nil {
		text = name + " acaba."
	}
	b.add(int32(e.Round), e.CombatantID, text, e.Phase, false, f.ID) //nolint:gosec // a round
}

// addSaves puts the saving throws of one target on the clock: at the end of its turn, then at the
// start of the one after.
func (b *turnClockBuilder) addSaves(st playdb.CombatantState, name, casterLabel string) {
	dc := derefInt32(st.SaveDc)
	if dc == 0 {
		dc = defaultEffectDC
	}
	from := " contra " + name
	if casterLabel != "" {
		from += ", de " + casterLabel
	}
	for _, s := range []struct {
		ability *string
		phase   string
	}{{st.EndSaveAbility, combat.PhaseEnd}, {st.StartSaveAbility, combat.PhaseStart}} {
		if s.ability == nil {
			continue
		}
		text := fmt.Sprintf("Teste de %s (CD %d)%s.", b.ev.names(*s.ability), dc, from)
		b.add(nextTurnRound(b.ev.d.enc, b.ev.d.cs, st.CombatantID), st.CombatantID, text, s.phase, true, st.ID)
	}
}

// entries are the entries from the round that runs on, in the order of the clock.
func (b *turnClockBuilder) entries(round int32) []*playv1.TurnClockEntry {
	slices.SortStableFunc(b.all, func(x, y clockKeyed) int {
		switch {
		case x.round != y.round:
			return int(x.round - y.round)
		case x.idx != y.idx:
			return x.idx - y.idx
		}
		return x.phase - y.phase
	})
	var out []*playv1.TurnClockEntry
	for _, k := range b.all {
		if k.round >= round {
			out = append(out, k.e)
		}
	}
	return out
}

// turnClock is what the clock does from the turn that is running on, in the order of the
// initiative: the saving throws and the effects that end, over the next two rounds.
func (ev *effectViewer) turnClock(groups []effectGroup) []*playv1.TurnClockEntry {
	b := &turnClockBuilder{ev: ev}
	for _, g := range groups {
		b.addGroup(g)
	}
	return b.entries(max(ev.d.enc.Round, 1))
}

// concentrations lists the casters that concentrate on an effect and the effects they hold.
func (ev *effectViewer) concentrations(groups []effectGroup) []*playv1.ConcentrationEntry {
	var out []*playv1.ConcentrationEntry
	for _, g := range groups {
		f := g.first()
		if !f.Concentration || f.SourceID == nil {
			continue
		}
		i := slices.IndexFunc(out, func(e *playv1.ConcentrationEntry) bool { return e.CasterId == *f.SourceID })
		if i < 0 {
			caster := ev.byID[*f.SourceID]
			out = append(out, &playv1.ConcentrationEntry{CasterId: caster.ID, CasterLabel: caster.Label, SpellKey: deref(caster.ConcentrationSpell), SpellNamePt: ev.names(deref(caster.ConcentrationSpell))})
			i = len(out) - 1
		}
		for _, st := range g.rows {
			out[i].EffectIds = append(out[i].EffectIds, st.ID)
		}
	}
	return out
}

// catalog lists what the master may add: the spells that last, the 14 conditions (exhaustion has
// its own call) and what the app adds itself (the web that burns).
func (ev *effectViewer) catalog() []*playv1.CatalogEffect {
	var out []*playv1.CatalogEffect
	for _, key := range []string{"spell:hold-person", "spell:bless", "spell:bane", "spell:haste", "spell:web", "spell:faerie-fire", "spell:hideous-laughter", "spell:guidance", "spell:resistance", "spell:enhance-ability", "spell:heroism", "spell:longstrider", "spell:mage-armor", "spell:pass-without-trace", "spell:shield-of-faith"} {
		def, ok := ev.content.CombatSpellEffect(key)
		if !ok {
			continue
		}
		e := &playv1.CatalogEffect{Key: key, NamePt: ev.names(key), SourceKind: playv1.EffectSourceKind_EFFECT_SOURCE_KIND_SPELL, Concentration: def.Concentration, HasCaster: true}
		e.DefaultDurationKind, e.DefaultRounds = playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_DISMISSED, 0
		if r := ev.content.SpellEffectRounds(key); r > 0 {
			e.DefaultDurationKind, e.DefaultRounds = playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS, clamp32(r, 0, maxEffectRounds)
		} else if def.Concentration {
			e.DefaultDurationKind = playv1.EffectDurationKind_EFFECT_DURATION_KIND_CONCENTRATION
		}
		out = append(out, e)
	}
	for _, key := range ev.content.CombatEffectKeys() {
		def, _ := ev.content.CombatEffect(key)
		if def.Kind != "master" {
			continue
		}
		out = append(out, &playv1.CatalogEffect{
			Key: key, NamePt: ev.names(key), SourceKind: playv1.EffectSourceKind_EFFECT_SOURCE_KIND_MASTER,
			DefaultDurationKind: durationKindProto(def.Duration.Kind), DefaultRounds: clamp32(def.Duration.Rounds, 0, maxEffectRounds),
		})
	}
	var conds []string
	conds = append(conds, ev.content.ConditionKeys()...)
	slices.SortFunc(conds, func(a, b string) int { return strings.Compare(ev.names(a), ev.names(b)) })
	for _, k := range conds {
		if k == "condition:exhaustion" {
			continue
		}
		out = append(out, &playv1.CatalogEffect{Key: k, NamePt: ev.names(k), SourceKind: playv1.EffectSourceKind_EFFECT_SOURCE_KIND_CONDITION, DefaultDurationKind: playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNTIL_DISMISSED})
	}
	return out
}
