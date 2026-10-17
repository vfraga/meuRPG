package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
	"uuid"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The master's calls on the effects that last (RN-22): the table of "Efeitos em jogo", "Adicionar
// efeito", "Mudar a duração", "Os jogadores veem este efeito", "Encerrar" and "Tirar de um alvo".
// Every one is the master's (a player gets not_found for anything of it, RN-10) and every write
// takes an idempotency key.

// The limits of what the master writes.
const (
	maxEffectTargets = 10
	maxEffectRounds  = 600
	maxEffectLabel   = 30
	// looseConditionPrefix is the id of a condition the master marked by hand, which the table lists
	// as an effect with no origin and no end: "condition:<combatant id>:<condition key>".
	looseConditionPrefix = "loose:"
)

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

// lastingDurationOf reads the duration the master picked.
func lastingDurationOf(ch *playv1.EffectDurationChoice, fallback durationSpec) (durationSpec, error) {
	if ch == nil || ch.GetKind() == playv1.EffectDurationKind_EFFECT_DURATION_KIND_UNSPECIFIED {
		return fallback, nil
	}
	d := durationSpec{Kind: durationKindOf(ch.GetKind()), Rounds: ch.GetRounds()}
	if raw := ch.GetAnchorCombatantId(); raw != "" {
		id, err := parseCombatID(raw, "combatant")
		if err != nil {
			return d, err
		}
		d.Anchor = id
	}
	bad := func(text string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(text)) }
	switch d.Kind {
	case rules.EffectDurationRounds:
		if d.Rounds < 1 || d.Rounds > maxEffectRounds {
			return d, bad("duration.rounds must be 1 to 600")
		}
	case rules.EffectDurationUntilStartOfTurnOf, rules.EffectDurationUntilEndOfTurnOf:
		if d.Anchor == "" {
			return d, bad("duration.anchor_combatant_id is the combatant whose turn ends it")
		}
		d.Rounds = 0
	default:
		d.Rounds, d.Anchor = 0, ""
	}
	return d, nil
}

// lastingInput is what AddLastingEffect reads from its request before it opens the transaction:
// the key, the combat, and the combatants the effect is about.
type lastingInput struct {
	key         string
	encounterID string
	targetIDs   []string
	casterID    string
}

// The limits of the saving throw's difficulty class the master may set.
const (
	minEffectDC = 1
	maxEffectDC = 40
)

// parseLastingInput checks the request: the key, the combat, the targets and the caster, the
// difficulty class and the label.
func parseLastingInput(msg *playv1.AddLastingEffectRequest) (lastingInput, error) {
	var in lastingInput
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return in, err
	}
	in.key = key
	if in.encounterID, err = parseCombatID(msg.GetEncounterId(), "encounter"); err != nil {
		return in, err
	}
	if in.targetIDs, err = parseTargetIDs(msg.GetTargetIds()); err != nil {
		return in, err
	}
	if raw := msg.GetCasterId(); raw != "" {
		if in.casterID, err = parseCombatID(raw, "combatant"); err != nil {
			return in, err
		}
	}
	if dc := msg.SaveDc; dc != nil && (*dc < minEffectDC || *dc > maxEffectDC) {
		return in, errInvalidEffect("save_dc must be 1 to 40")
	}
	return in, checkEffectLabel(msg.GetPlayerLabel())
}

// parseTargetIDs reads the targets: one to ten combatants, none twice.
func parseTargetIDs(raw []string) ([]string, error) {
	if n := len(raw); n < 1 || n > maxEffectTargets {
		return nil, errInvalidEffect("target_ids must have 1 to 10 combatants")
	}
	var ids []string
	for _, r := range raw {
		id, err := parseCombatID(r, "combatant")
		if err != nil {
			return nil, err
		}
		if slices.Contains(ids, id) {
			return nil, errInvalidEffect("target_ids repeats a combatant")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// AddLastingEffect implements playv1connect.LastingEffectServiceHandler.
func (s *Service) AddLastingEffect(
	ctx context.Context,
	req *connect.Request[playv1.AddLastingEffectRequest],
) (*connect.Response[playv1.AddLastingEffectResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	in, err := parseLastingInput(req.Msg)
	if err != nil {
		return nil, err
	}
	var made []playdb.CombatantState
	var secret bool
	res, err := s.write(ctx, combatWrite{m: m, key: in.key, hash: idem.Hash(req.Msg), kind: eventLastingAdded, encounterID: in.encounterID}, func(c *combatTx) (any, error) {
		made = nil
		rows, ev, err := s.addLastingEffectTx(ctx, c, m, in, req.Msg)
		if err != nil {
			return nil, err
		}
		made, secret = rows, ev.Secret
		return ev, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "add an effect", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !secret)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AddLastingEffectResponse{Encounter: out, Effects: s.addedCards(ctx, m, res, made)}), nil
}

// addLastingEffectTx is the change AddLastingEffect makes inside the combat's transaction: read
// and validate (the combat, the combatants), resolve the effect, write it and the concentration,
// and answer the effect it made and the event that tells it.
func (s *Service) addLastingEffectTx(ctx context.Context, c *combatTx, m authz.Membership, in lastingInput, msg *playv1.AddLastingEffectRequest) ([]playdb.CombatantState, actionEvent, error) {
	var none actionEvent
	if err := notEnded(c.enc); err != nil {
		return nil, none, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, none, fmt.Errorf("list the combatants: %w", err)
	}
	targets, caster, err := lastingPartiesOf(cs, in, viewerOf(m))
	if err != nil {
		return nil, none, err
	}
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return nil, none, err
	}
	spec, opts, err := s.planLastingEffect(ctx, c, content, cs, msg, targets, caster)
	if err != nil {
		return nil, none, err
	}
	if err := s.holdConcentration(ctx, c, spec); err != nil {
		return nil, none, err
	}
	rows, err := s.putLastingEffects(ctx, c, cs, spec, opts)
	if err != nil {
		return nil, none, err
	}
	ev := addedPayload(c, rows, spec.key)
	c.characterID = &targets[0].CharacterID
	if err := touchEncounter(ctx, c); err != nil {
		return nil, none, err
	}
	return rows, ev, nil
}

// lastingPartiesOf finds the targets and the caster among the combatants, as the viewer sees them.
func lastingPartiesOf(cs []playdb.Combatant, in lastingInput, v combatViewer) ([]playdb.Combatant, *playdb.Combatant, error) {
	var targets []playdb.Combatant
	for _, id := range in.targetIDs {
		t, err := findCombatant(cs, id, v)
		if err != nil {
			return nil, nil, err
		}
		targets = append(targets, t)
	}
	if in.casterID == "" {
		return targets, nil, nil
	}
	cb, err := findCombatant(cs, in.casterID, v)
	if err != nil {
		return nil, nil, err
	}
	return targets, &cb, nil
}

// lastingSource is what the catalog says about an effect: how long it lasts when the master says
// nothing, and what the cast decided about its modifiers.
type lastingSource struct {
	fallback durationSpec
	opts     castOpts
}

// planLastingEffect resolves the request into the spec of the effect: the catalog entry, the
// duration, the concentration, who reads it and the label.
func (s *Service) planLastingEffect(ctx context.Context, c *combatTx, content *rules.Content, cs []playdb.Combatant, msg *playv1.AddLastingEffectRequest, targets []playdb.Combatant, caster *playdb.Combatant) (effectSpec, castOpts, error) {
	catalogKey := msg.GetCatalogKey()
	spec := effectSpec{key: catalogKey, caster: caster, targets: targets, dc: msg.SaveDc, audience: ""}
	var src lastingSource
	var err error
	switch catalogKindOf(catalogKey) {
	case catalogCondition:
		src, err = lastingConditionSource(content, &spec)
	case catalogSpell:
		src, err = s.lastingSpellSource(ctx, c, content, &spec, msg.GetAbilityKey())
	default:
		src, err = lastingMasterSource(content, &spec)
	}
	if err != nil {
		return spec, src.opts, err
	}
	err = applyLastingChoices(&spec, cs, msg, src.fallback)
	return spec, src.opts, err
}

// lastingConditionSource is a condition: until dismissed, read as the condition says.
func lastingConditionSource(content *rules.Content, spec *effectSpec) (lastingSource, error) {
	info, err := conditionOfCatalog(content, spec.key)
	if err != nil {
		return lastingSource{}, err
	}
	spec.sourceKind, spec.conditions = "master", []string{spec.key}
	spec.audience = info.Visibility
	return lastingSource{fallback: durationSpec{Kind: rules.EffectDurationUntilDismissed}}, nil
}

// lastingSpellSource is a spell's effect: its modifiers worked out from the caster, and the spell's
// own duration.
func (s *Service) lastingSpellSource(ctx context.Context, c *combatTx, content *rules.Content, spec *effectSpec, abilityKey string) (lastingSource, error) {
	src := lastingSource{fallback: durationSpec{Kind: rules.EffectDurationUntilDismissed}}
	def, err := spellEffectOfCatalog(content, spec.key)
	if err != nil {
		return src, err
	}
	spec.sourceKind, spec.def, spec.concentration = "spell", def, def.Concentration && spec.caster != nil
	ability, err := s.abilityChoiceOf(ctx, c, spec.key, abilityKey)
	if err != nil {
		return src, err
	}
	casterCharacter := ""
	if spec.caster != nil {
		casterCharacter = spec.caster.CharacterID
	}
	if src.opts, err = s.castOptsOf(ctx, c, def, ability, casterCharacter); err != nil {
		return src, err
	}
	if spec.modifiers, _, err = s.modifiersFor(ctx, c, def, "", src.opts); err != nil {
		return src, err
	}
	if r := content.SpellEffectRounds(spec.key); r > 0 {
		src.fallback = durationSpec{Kind: rules.EffectDurationRounds, Rounds: int32(r)} //nolint:gosec // 10 rounds a minute
	} else if def.Concentration {
		src.fallback = durationSpec{Kind: rules.EffectDurationConcentration}
	}
	return src, nil
}

// lastingMasterSource is an effect of the master's own, with the duration the catalog gives it.
func lastingMasterSource(content *rules.Content, spec *effectSpec) (lastingSource, error) {
	def, err := masterEffectOfCatalog(content, spec.key)
	if err != nil {
		return lastingSource{}, err
	}
	spec.sourceKind, spec.def = "master", def
	return lastingSource{fallback: durationSpec{Kind: def.Duration.Kind, Rounds: int32(def.Duration.Rounds)}}, nil //nolint:gosec // a small number of the file
}

// applyLastingChoices puts what the master chose on the spec: the duration (checked against the
// combatants), the concentration, who sees the effect and the label.
func applyLastingChoices(spec *effectSpec, cs []playdb.Combatant, msg *playv1.AddLastingEffectRequest, fallback durationSpec) error {
	var err error
	if spec.dur, err = lastingDurationOf(msg.GetDuration(), fallback); err != nil {
		return err
	}
	if spec.dur.Anchor != "" && !slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.ID == spec.dur.Anchor }) {
		return errCombatantNotFound()
	}
	if msg.Concentration != nil && spec.caster != nil && spec.def != nil && spec.sourceKind == "spell" {
		spec.concentration = msg.GetConcentration()
	}
	if v := msg.PlayerVisible; v != nil {
		spec.visible = v
	}
	spec.audience = chosenAudience(msg.GetAudience(), spec.audience)
	spec.label = msg.GetPlayerLabel()
	return nil
}

// holdConcentration makes the effect the caster's concentration. One concentration for each
// caster: the new spell ends the old one and what it held.
func (s *Service) holdConcentration(ctx context.Context, c *combatTx, spec effectSpec) error {
	caster := spec.caster
	if !spec.concentration || caster == nil {
		return nil
	}
	if caster.ConcentrationSpell != nil && *caster.ConcentrationSpell != spec.key {
		stop := actionEvent{Round: c.enc.Round, Secret: caster.Hidden, Actor: caster.ID}
		if err := s.stopConcentrating(ctx, c, *caster, &stop); err != nil {
			return err
		}
		if err := insertEvent(ctx, c, eventConditionsSet, &c.actorUserID, nil, stop); err != nil {
			return err
		}
	}
	if err := c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: caster.ID, ConcentrationSpell: &spec.key}); err != nil {
		return fmt.Errorf("set the concentration: %w", err)
	}
	return nil
}

// putLastingEffects writes the effect on the targets. An effect that sets a base armor class is a
// record for each target, with the class worked out for it, and none for a target that wears armor.
func (s *Service) putLastingEffects(ctx context.Context, c *combatTx, cs []playdb.Combatant, spec effectSpec, opts castOpts) ([]playdb.CombatantState, error) {
	if !hasBaseAC(spec.def) {
		return s.addEffects(ctx, c, cs, spec)
	}
	// The base armor class is the target's own (13 + its Dexterity).
	var rows []playdb.CombatantState
	spec.group = uuid.New().String()
	for _, t := range spec.targets {
		mods, ok, err := s.modifiersForTarget(ctx, c, spec.def, t, opts)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		one := spec
		one.targets, one.modifiers = []playdb.Combatant{t}, mods
		made, err := s.addEffects(ctx, c, cs, one)
		if err != nil {
			return nil, err
		}
		rows = append(rows, made...)
	}
	return rows, nil
}

// addedCards are the cards of the effects the call made, as the master reads them; read after the
// commit, and none when the read fails or the call made nothing.
func (s *Service) addedCards(ctx context.Context, m authz.Membership, res combatResult, made []playdb.CombatantState) []*playv1.LastingEffect {
	if len(made) == 0 {
		return nil
	}
	d, err := s.encounterDataOf(ctx, res)
	if err != nil {
		return nil
	}
	ev, err := s.effectViewerFor(ctx, m, d, combatViewer{master: true}, s.namesFor(ctx, m.CampaignID))
	if err != nil {
		return nil
	}
	var cards []*playv1.LastingEffect
	for _, g := range groupsOf(d.states) {
		if slices.ContainsFunc(g.rows, func(st playdb.CombatantState) bool {
			return slices.ContainsFunc(made, func(o playdb.CombatantState) bool { return o.ID == st.ID })
		}) {
			cards = append(cards, ev.card(g, ""))
		}
	}
	return cards
}

// addedPayload is the event of a casting of an effect that began: one line for it.
func addedPayload(c *combatTx, rows []playdb.CombatantState, key string) actionEvent {
	var targets, ids []string
	for _, st := range rows {
		targets, ids = append(targets, st.CombatantID), append(ids, st.ID)
	}
	ev := actionEvent{Round: c.enc.Round}
	if len(rows) == 0 {
		return ev
	}
	first := rows[0]
	var rounds int32
	if first.EndsRound != nil {
		rounds = max(*first.EndsRound-max(c.enc.Round, 1), 0)
	}
	ev.Actor, ev.Target = deref(first.SourceID), targets[0]
	ev.Secret = !first.PlayerVisible || first.Audience == "owner"
	ev.Lasting = &lastingEvent{Key: key, Change: "added", Targets: targets, Caster: deref(first.SourceID), Rounds: rounds, Effects: ids}
	return ev
}

// encounterDataOf reads the combat a change was about, after the commit.
func (s *Service) encounterDataOf(ctx context.Context, res combatResult) (*encounterData, error) {
	pctx, stop := afterCommit(ctx)
	defer stop()
	enc, err := s.queries.GetEncounterInSession(pctx, playdb.GetEncounterInSessionParams{GameSessionID: res.session.ID, ID: res.encounterID})
	if err != nil {
		return nil, err
	}
	return loadEncounter(pctx, s.queries, enc)
}

// groupRowsOf are the rows of the casting an effect id belongs to: every target of it. A row that
// is not in the combat is not found, the same for one that never was.
func groupRowsOf(rows []playdb.CombatantState, id string) ([]playdb.CombatantState, error) {
	i := slices.IndexFunc(rows, func(st playdb.CombatantState) bool { return st.ID == id })
	if i < 0 {
		return nil, errEffectNotFound()
	}
	f := rows[i]
	return slices.DeleteFunc(slices.Clone(rows), func(st playdb.CombatantState) bool {
		return deref(st.GroupID) != deref(f.GroupID) || deref(st.SourceKey) != deref(f.SourceKey)
	}), nil
}

// effectCall is what the master's calls on one running effect read before the transaction: the
// campaign's master, the key, the combat and the effect.
type effectCall struct {
	m     authz.Membership
	key   string
	encID string
	effID string
}

// readEffectCall checks who calls and reads the key, the combat and the effect, in this order.
func readEffectCall(ctx context.Context, campaignID, rawKey, rawEncounter, rawEffect string) (effectCall, error) {
	var call effectCall
	m, err := authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster)
	if err != nil {
		return call, err
	}
	call.m = m
	if call.key, err = parseKey(rawKey); err != nil {
		return call, err
	}
	if call.encID, err = parseCombatID(rawEncounter, "encounter"); err != nil {
		return call, err
	}
	if call.effID, err = parseCombatID(rawEffect, "effect"); err != nil {
		return call, errEffectNotFound()
	}
	return call, nil
}

// loadEffectGroup reads the rows of the casting an effect id belongs to.
func loadEffectGroup(ctx context.Context, c *combatTx, effID string) ([]playdb.CombatantState, error) {
	all, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the effects: %w", err)
	}
	return groupRowsOf(all, effID)
}

// roundsLeftOf are the rounds from the one that runs to the end of a row, none for one with no end.
func roundsLeftOf(enc playdb.Encounter, endsRound *int32) int32 {
	if endsRound == nil {
		return 0
	}
	return max(*endsRound-max(enc.Round, 1), 0)
}

// ChangeLastingEffectDuration implements playv1connect.LastingEffectServiceHandler.
func (s *Service) ChangeLastingEffectDuration(
	ctx context.Context,
	req *connect.Request[playv1.ChangeLastingEffectDurationRequest],
) (*connect.Response[playv1.ChangeLastingEffectDurationResponse], error) {
	call, err := readEffectCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetEffectId())
	if err != nil {
		return nil, err
	}
	m := call.m
	var secret bool
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventLastingChanged, encounterID: call.encID}, func(c *combatTx) (any, error) {
		ev, err := s.changeDurationTx(ctx, c, call.effID, req.Msg.GetDuration())
		if err != nil {
			return nil, err
		}
		secret = ev.Secret
		return ev, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "change an effect's duration", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !secret)
	})
	if err != nil {
		return nil, err
	}
	resp := &playv1.ChangeLastingEffectDurationResponse{Encounter: out}
	resp.Effect = s.cardAfter(ctx, m, res, call.effID)
	return connect.NewResponse(resp), nil
}

// changeDurationTx is the change ChangeLastingEffectDuration makes inside the combat's
// transaction: read the casting and the combatants, work out the new end of each target, write it
// and answer the event.
func (s *Service) changeDurationTx(ctx context.Context, c *combatTx, effID string, choice *playv1.EffectDurationChoice) (actionEvent, error) {
	var none actionEvent
	if err := notEnded(c.enc); err != nil {
		return none, err
	}
	rows, err := loadEffectGroup(ctx, c, effID)
	if err != nil {
		return none, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return none, fmt.Errorf("list the combatants: %w", err)
	}
	var caster *playdb.Combatant
	if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == deref(rows[0].SourceID) }); i >= 0 {
		caster = &cs[i]
	}
	dur, err := lastingDurationOf(choice, durationSpec{Kind: deref(rows[0].DurationKind)})
	if err != nil {
		return none, err
	}
	if dur.Anchor != "" && !slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.ID == dur.Anchor }) {
		return none, errCombatantNotFound()
	}
	changed, targets, err := s.writeNewEnds(ctx, c, cs, rows, dur, caster)
	if err != nil {
		return none, err
	}
	first := changed[0]
	secret := s.effectHiddenFrom(first, targetsOf(cs, targets))
	rounds := roundsLeftOf(c.enc, first.EndsRound)
	c.characterID = &cs[0].CharacterID
	if err := touchEncounter(ctx, c); err != nil {
		return none, err
	}
	return actionEvent{
		Round: c.enc.Round, Secret: secret, Actor: deref(first.SourceID), Target: targets[0],
		Lasting: &lastingEvent{Key: deref(first.SourceKey), Change: "duration", Targets: targets, Rounds: rounds, Effects: []string{first.ID}},
	}, nil
}

// writeNewEnds writes the end the duration gives each row of a casting, and answers the rows
// written and the combatants they are on.
func (s *Service) writeNewEnds(ctx context.Context, c *combatTx, cs []playdb.Combatant, rows []playdb.CombatantState, dur durationSpec, caster *playdb.Combatant) ([]playdb.CombatantState, []string, error) {
	var changed []playdb.CombatantState
	var targets []string
	for _, st := range rows {
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == st.CombatantID })
		if i < 0 {
			continue
		}
		round, who, phase := endsOf(c.enc, cs, dur, caster, cs[i])
		row, err := c.q.SetLastingEffectEnds(ctx, playdb.SetLastingEffectEndsParams{ID: st.ID, DurationKind: &dur.Kind, EndsRound: round, EndsCombatantID: who, EndsPhase: phase})
		if err != nil {
			return nil, nil, fmt.Errorf("change the duration: %w", err)
		}
		changed = append(changed, row)
		targets = append(targets, st.CombatantID)
	}
	return changed, targets, nil
}

// cardAfter reads the effect a change was about, as the master reads it now; nil when it is gone.
func (s *Service) cardAfter(ctx context.Context, m authz.Membership, res combatResult, id string) *playv1.LastingEffect {
	d, err := s.encounterDataOf(ctx, res)
	if err != nil {
		return nil
	}
	ev, err := s.effectViewerFor(ctx, m, d, combatViewer{master: true}, s.namesFor(ctx, m.CampaignID))
	if err != nil {
		return nil
	}
	for _, g := range groupsOf(d.states) {
		if slices.ContainsFunc(g.rows, func(st playdb.CombatantState) bool { return st.ID == id }) {
			return ev.card(g, "")
		}
	}
	return nil
}

// visibilityInput is what SetLastingEffectVisibility reads of the master's choice: the label
// (trimmed, none when empty) and who reads the effect.
type visibilityInput struct {
	label    string
	audience string
}

// readVisibilityInput checks the label and reads the audience.
func readVisibilityInput(msg *playv1.SetLastingEffectVisibilityRequest) (visibilityInput, error) {
	in := visibilityInput{label: strings.TrimSpace(msg.GetPlayerLabel()), audience: "all"}
	if utf8.RuneCountInString(in.label) > maxEffectLabel {
		return in, connect.NewError(connect.CodeInvalidArgument, errors.New("player_label must have at most 30 characters"))
	}
	if msg.GetAudience() == playv1.EffectAudience_EFFECT_AUDIENCE_OWNER {
		in.audience = "owner"
	}
	return in, nil
}

// SetLastingEffectVisibility implements playv1connect.LastingEffectServiceHandler.
func (s *Service) SetLastingEffectVisibility(
	ctx context.Context,
	req *connect.Request[playv1.SetLastingEffectVisibilityRequest],
) (*connect.Response[playv1.SetLastingEffectVisibilityResponse], error) {
	call, err := readEffectCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetEffectId())
	if err != nil {
		return nil, err
	}
	m := call.m
	in, err := readVisibilityInput(req.Msg)
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventLastingVisibility, encounterID: call.encID}, func(c *combatTx) (any, error) {
		return s.setVisibilityTx(ctx, c, call.effID, in, req.Msg.GetPlayerVisible())
	})
	if err != nil {
		return nil, s.dbError(ctx, "set an effect's visibility", err)
	}
	out, err := s.finish(ctx, m, res, s.changed(m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetLastingEffectVisibilityResponse{Encounter: out, Effect: s.cardAfter(ctx, m, res, call.effID)}), nil
}

// setVisibilityTx is the change SetLastingEffectVisibility makes inside the combat's transaction:
// every row of the casting takes the switch, and one line for the master alone tells it.
func (s *Service) setVisibilityTx(ctx context.Context, c *combatTx, effID string, in visibilityInput, visible bool) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	rows, err := loadEffectGroup(ctx, c, effID)
	if err != nil {
		return nil, err
	}
	var lbl *string
	if in.label != "" {
		lbl = &in.label
	}
	for _, st := range rows {
		if _, err := c.q.SetLastingEffectVisibility(ctx, playdb.SetLastingEffectVisibilityParams{ID: st.ID, PlayerVisible: visible, Audience: in.audience, PlayerLabel: lbl}); err != nil {
			return nil, fmt.Errorf("set the visibility: %w", err)
		}
	}
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	// A line for the master alone: a switch is nothing the table reads.
	return actionEvent{
		Round: c.enc.Round, Secret: true, Actor: deref(rows[0].SourceID), Target: rows[0].CombatantID,
		Lasting: &lastingEvent{Key: deref(rows[0].SourceKey), Change: "visibility", Effects: []string{rows[0].ID}},
	}, nil
}

// endedPublisher tells the table that effects ended: the combat, the log and the turn.
func (s *Service) endedPublisher(campaignID string) func(ctx context.Context, d *encounterData) {
	return func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, campaignID, d.enc)
		s.publishLogChanged(ctx, campaignID, d.enc.ID, false)
		s.publishTurnChanged(ctx, campaignID, d)
	}
}

// EndLastingEffect implements playv1connect.LastingEffectServiceHandler.
func (s *Service) EndLastingEffect(
	ctx context.Context,
	req *connect.Request[playv1.EndLastingEffectRequest],
) (*connect.Response[playv1.EndLastingEffectResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	scope := req.Msg.GetScope()
	if scope != playv1.EffectEndScope_EFFECT_END_SCOPE_THIS && scope != playv1.EffectEndScope_EFFECT_END_SCOPE_CONCENTRATION_GROUP {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("scope must be THIS or CONCENTRATION_GROUP"))
	}
	if req.Msg.GetEncounterId() == "" {
		return s.endCharacterEffect(ctx, m, req.Msg, key)
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	var ended int32
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventLastingEnded, encounterID: encID}, func(c *combatTx) (any, error) {
		ended = 0
		return s.endEffectTx(ctx, c, req.Msg.GetEffectId(), scope, &ended)
	})
	if err != nil {
		return nil, s.dbError(ctx, "end an effect", err)
	}
	out, err := s.finish(ctx, m, res, s.endedPublisher(m.CampaignID))
	if err != nil {
		return nil, err
	}
	// The call makes no effect of its own: created_effect_ids stays empty.
	return connect.NewResponse(&playv1.EndLastingEffectResponse{Encounter: out, Ended: ended}), nil
}

// endEffectTx is the change EndLastingEffect makes inside the combat's transaction: a loose
// condition goes by its id; any other effect ends with its casting, or with the whole
// concentration it belongs to.
func (s *Service) endEffectTx(ctx context.Context, c *combatTx, rawID string, scope playv1.EffectEndScope, ended *int32) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	if strings.HasPrefix(rawID, looseConditionPrefix) {
		return s.endLooseCondition(ctx, c, cs, rawID, ended)
	}
	effID, err := parseCombatID(rawID, "effect")
	if err != nil {
		return nil, errEffectNotFound()
	}
	all, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the effects: %w", err)
	}
	rows, err := groupRowsOf(all, effID)
	if err != nil {
		return nil, err
	}
	first := rows[0]
	reason := endByMaster
	if scope == playv1.EffectEndScope_EFFECT_END_SCOPE_CONCENTRATION_GROUP && first.Concentration && first.SourceID != nil {
		// The whole concentration: the caster stops concentrating and everything it held ends.
		rows = slices.DeleteFunc(all, func(st playdb.CombatantState) bool { return !st.Concentration || deref(st.SourceID) != *first.SourceID })
		if err := s.stopCasterOf(ctx, c, cs, *first.SourceID); err != nil {
			return nil, err
		}
		reason = endConcentration
	}
	before := len(rows)
	if err := s.endEffectRows(ctx, c, cs, rows, reason); err != nil {
		return nil, err
	}
	*ended = clamp32(before, 0, 100)
	c.characterID = &cs[0].CharacterID
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	return actionEvent{
		Round: c.enc.Round, Secret: s.effectHiddenFrom(first, targetsOf(cs, []string{first.CombatantID})), Actor: deref(first.SourceID), Target: first.CombatantID,
		Lasting: &lastingEvent{Key: deref(first.SourceKey), Change: "ended", Reason: reason, Effects: []string{first.ID}},
	}, nil
}

// stopCasterOf makes the caster stop concentrating, with the line of the combat log, when it holds
// a spell.
func (s *Service) stopCasterOf(ctx context.Context, c *combatTx, cs []playdb.Combatant, casterID string) error {
	i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == casterID && o.ConcentrationSpell != nil })
	if i < 0 {
		return nil
	}
	if err := c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: cs[i].ID}); err != nil {
		return fmt.Errorf("end the concentration: %w", err)
	}
	stop := actionEvent{Round: c.enc.Round, Secret: cs[i].Hidden, Actor: cs[i].ID, ConcBefore: *cs[i].ConcentrationSpell, ConcEnded: *cs[i].ConcentrationSpell}
	return insertEvent(ctx, c, eventConditionsSet, &c.actorUserID, nil, stop)
}

// looseConditionParts are the two parts of a loose condition's id: the combatant and the condition.
const looseConditionParts = 2

// endLooseCondition takes a condition the master marked by hand off a combatant: the id names it.
func (s *Service) endLooseCondition(ctx context.Context, c *combatTx, cs []playdb.Combatant, id string, ended *int32) (any, error) {
	parts := strings.SplitN(strings.TrimPrefix(id, looseConditionPrefix), ":", 2)
	if len(parts) != looseConditionParts {
		return nil, errEffectNotFound()
	}
	who, err := findCombatant(cs, parts[0], combatViewer{master: true})
	if err != nil {
		return nil, errEffectNotFound()
	}
	cond := parts[1]
	if !slices.Contains(who.Conditions, cond) || slices.Contains(who.EffectConditions, cond) {
		return nil, errEffectNotFound()
	}
	next := slices.DeleteFunc(slices.Clone(who.Conditions), func(k string) bool { return k == cond })
	if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: next}); err != nil {
		return nil, fmt.Errorf("take the condition off: %w", err)
	}
	*ended = 1
	c.characterID = &who.CharacterID
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	return actionEvent{Round: c.enc.Round, Secret: true, Actor: who.ID, CondSet: true, Conditions: next, CondBefore: who.Conditions}, nil
}

// RemoveEffectTarget implements playv1connect.LastingEffectServiceHandler.
func (s *Service) RemoveEffectTarget(
	ctx context.Context,
	req *connect.Request[playv1.RemoveEffectTargetRequest],
) (*connect.Response[playv1.RemoveEffectTargetResponse], error) {
	call, err := readEffectCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetEffectId())
	if err != nil {
		return nil, err
	}
	m := call.m
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, errEffectNotFound()
	}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventLastingEnded, encounterID: call.encID}, func(c *combatTx) (any, error) {
		return s.removeTargetTx(ctx, c, call.effID, combID)
	})
	if err != nil {
		return nil, s.dbError(ctx, "take a target off an effect", err)
	}
	out, err := s.finish(ctx, m, res, s.endedPublisher(m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RemoveEffectTargetResponse{Encounter: out}), nil
}

// removeTargetTx is the change RemoveEffectTarget makes inside the combat's transaction: the row of
// the combatant ends and one line tells it.
func (s *Service) removeTargetTx(ctx context.Context, c *combatTx, effID, combID string) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	rows, err := loadEffectGroup(ctx, c, effID)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(rows, func(st playdb.CombatantState) bool { return st.CombatantID == combID })
	if i < 0 {
		return nil, errEffectNotFound()
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	row := rows[i]
	if err := s.endEffectRows(ctx, c, cs, []playdb.CombatantState{row}, endByMaster); err != nil {
		return nil, err
	}
	c.characterID = &cs[0].CharacterID
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	return actionEvent{
		Round: c.enc.Round, Secret: s.effectHiddenFrom(row, targetsOf(cs, []string{combID})), Actor: deref(row.SourceID), Target: combID,
		Lasting: &lastingEvent{Key: deref(row.SourceKey), Change: "ended", Reason: endByMaster, Targets: []string{combID}, Effects: []string{row.ID}},
	}, nil
}
