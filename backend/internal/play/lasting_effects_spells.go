package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/play/steed"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// What a spell that lasts leaves on its targets (RN-22): Bênção, Perdição, Velocidade,
// Imobilizar Pessoa, Riso Histérico, Fogo das Fadas and Teia (SRD 5.1). The cast rolls the
// saving throw of the spell; this puts the effect on the targets it took
// hold of, with the duration of the spell on the clock (one minute is 10 rounds, counted to
// the start of the caster's turn) and the caster's concentration holding it. The rider of an
// instantaneous spell (Zombaria Viciosa's disadvantage) has a clock of its own, on its target's
// next turn.

// The outcomes of a spell's effect on a target, as a cast event keeps them.
const (
	lastingApplied  = "applied"
	lastingNoEffect = "no_effect"
)

// Why a spell had no effect on a target: only the master reads it (RN-20: a player reads
// "A magia não teve efeito." and never why).
const (
	whyNotHumanoid = "alvo não é humanoide"
	whyImmune      = "o alvo é imune à condição"
	whyTooSimple   = "a Inteligência do alvo é 4 ou menos"
	whyWearsArmor  = "o alvo veste armadura"
)

// applyLastingSpell puts the effect of a spell on the targets it took hold of. targs and
// made.Hits are in the same order. A spell without an effect that lasts does nothing here.
func (s *Service) applyLastingSpell(
	ctx context.Context, c *combatTx, cs []playdb.Combatant, sp link.Spell, slotLevel int, caster playdb.Combatant, targs []playdb.Combatant, ability string, made *actionEvent,
) error {
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return err
	}
	def, ok := content.CombatSpellEffect(sp.Key)
	if !ok || len(targs) == 0 || len(made.Hits) != len(targs) {
		return nil
	}
	if _, hits := hitModifierOf(def.Modifiers); hits { // the damage it adds to the caster's weapon hits (lasting_effects_hit.go)
		return s.applyHitSpell(ctx, c, cs, sp, slotLevel, caster, targs, ability, def, made)
	}
	_, err = s.putLastingOnTargets(ctx, c, cs, sp, slotLevel, caster, targs, ability, def, made)
	return err
}

// putLastingOnTargets puts the effect on the targets it takes hold of, and returns the rows it made.
func (s *Service) putLastingOnTargets(
	ctx context.Context, c *combatTx, cs []playdb.Combatant, sp link.Spell, slotLevel int, caster playdb.Combatant, targs []playdb.Combatant, ability string, def *rules.EffectDef, made *actionEvent,
) ([]playdb.CombatantState, error) {
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return nil, err
	}
	taken, err := s.spellHolds(ctx, c, content, def, ability, caster, targs, made)
	if err != nil || len(taken) == 0 {
		return nil, err
	}
	def = defAtSlot(def, slotLevel-sp.Level)
	dur := spellEffectDuration(content, def, sp.Key, caster)
	var dc *int32
	if sp.CasterDC > 0 {
		dc = new(clamp32(sp.CasterDC, 1, 40))
	}
	made.Lasting = &lastingEvent{Key: sp.Key, Change: "added"}
	// An effect that sets a base armor class (Mage Armor) is worked out for each target, and does
	// not take hold of a creature that wears armor.
	opts, err := s.castOptsOf(ctx, c, def, ability, caster.CharacterID)
	if err != nil {
		return nil, err
	}
	groups, mods, err := s.spellEffectGroups(ctx, c, def, taken, opts)
	if err != nil {
		return nil, err
	}
	prior, err := s.priorTempClaims(ctx, c, sp.Key, made.CastID)
	if err != nil {
		return nil, err
	}
	var rows []playdb.CombatantState
	for i, g := range groups {
		made1, err := s.addEffects(ctx, c, cs, effectSpec{
			key: sp.Key, sourceKind: "spell", def: def, caster: &caster, group: made.CastID, targets: g, dur: dur,
			concentration: def.Concentration, dc: dc, modifiers: mods[i],
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, made1...)
	}
	for _, r := range rows {
		made.Lasting.Targets = append(made.Lasting.Targets, r.CombatantID)
		made.Lasting.Effects = append(made.Lasting.Effects, r.ID)
	}
	return rows, s.giveEffectTempHP(ctx, c, cs, rows, targs, made, prior)
}

// giveEffectTempHP gives the temporary hit points a temp_hp modifier of the effect gives each target it
// took hold of. The pool is not added to: the target keeps the larger (SRD 5.1, "Temporary Hit Points").
// The row then keeps, as its temp_hp, the pool it set, or no temp_hp when it raised nothing, so the end
// of the effect clears only what this effect put there (never a pool from False Life or another effect),
// and it reads it from its own row, not from the catalog. The effect that raised the pool last owns it:
// the temp_hp claim of the other effects on the target is dropped.
func (s *Service) giveEffectTempHP(ctx context.Context, c *combatTx, cs []playdb.Combatant, rows []playdb.CombatantState, targs []playdb.Combatant, made *actionEvent, prior map[string]int) error {
	for _, r := range rows {
		mods := effectModifiers(r)
		if combat.TempHP(mods) <= 0 {
			continue
		}
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == r.CombatantID })
		j := slices.IndexFunc(targs, func(o playdb.Combatant) bool { return o.ID == r.CombatantID })
		pool := 0 // not given here (i or j not found): the end has nothing of this effect to take back
		if i >= 0 && j >= 0 {
			var err error
			if pool, err = s.giveRowTempHP(ctx, c, cs[i], r, mods, made, j, prior[r.CombatantID]); err != nil {
				return err
			}
		}
		if err := s.setClaim(ctx, c, r, mods, pool); err != nil {
			return err
		}
	}
	return nil
}

// giveRowTempHP gives one row's temporary hit points and returns the pool the row claims. When nothing was
// raised a recast of the same spell keeps the claim of the cast it replaced (inherited).
func (s *Service) giveRowTempHP(ctx context.Context, c *combatTx, who playdb.Combatant, r playdb.CombatantState, mods []rules.EffectModifier, made *actionEvent, hit, inherited int) (int, error) {
	t, err := s.readFxTarget(ctx, c, who)
	if err != nil {
		return 0, err
	}
	amount := combat.TempHP(mods)
	if amount <= t.temp {
		return inherited, nil
	}
	return amount, s.takeOverTempHP(ctx, c, t, r, amount, made, hit)
}

// priorTempClaims are the temp_hp claims, by combatant, of the effects of the same spell another casting left:
// the recast replaces them (dropSameSpell) and inherits what they claimed.
func (s *Service) priorTempClaims(ctx context.Context, c *combatTx, key, group string) (map[string]int, error) {
	all, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the effects: %w", err)
	}
	out := map[string]int{}
	for _, o := range all {
		if conv.OrZero(o.SourceKey) == key && conv.OrZero(o.GroupID) != group {
			out[o.CombatantID] = max(out[o.CombatantID], combat.TempHPClaim(effectModifiers(o)))
		}
	}
	return out, nil
}

func (s *Service) setClaim(ctx context.Context, c *combatTx, r playdb.CombatantState, mods []rules.EffectModifier, pool int) error {
	if err := c.q.SetLastingEffectModifiers(ctx, playdb.SetLastingEffectModifiersParams{ID: r.ID, Modifiers: claimedTempHP(mods, pool)}); err != nil {
		return fmt.Errorf("keep the temporary hit points the effect gave: %w", err)
	}
	return nil
}

// takeOverTempHP raises the pool of the target to amount and takes the temp_hp claim off its other effects.
func (s *Service) takeOverTempHP(ctx context.Context, c *combatTx, t fxTarget, r playdb.CombatantState, amount int, made *actionEvent, hit int) error {
	var given castHit
	v, err := s.giveTempHP(ctx, c, t, amount, &given)
	if err != nil {
		return err
	}
	if v != nil {
		c.told = append(c.told, v)
	}
	if made.Hits[hit].Restore == nil {
		made.Hits[hit].Restore, made.Hits[hit].DeathBefore = given.Restore, given.DeathBefore
	}
	all, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	for _, o := range all {
		if o.CombatantID != r.CombatantID || o.ID == r.ID || combat.TempHPClaim(effectModifiers(o)) == 0 {
			continue
		}
		made.Hits[hit].Claims = append(made.Hits[hit].Claims, claimBack{ID: o.ID, Mods: o.Modifiers})
		if err := c.q.SetLastingEffectModifiers(ctx, playdb.SetLastingEffectModifiersParams{ID: o.ID, Modifiers: claimedTempHP(effectModifiers(o), 0)}); err != nil {
			return fmt.Errorf("drop the claim of an older effect: %w", err)
		}
	}
	return nil
}

// claimedTempHP is the modifiers with the claim on their temp_hp set to the pool the effect set (none when pool is 0).
// The modifier's value stays the spell's own number, which the card shows.
func claimedTempHP(mods []rules.EffectModifier, pool int) []byte {
	out := slices.Clone(mods)
	for i := range out {
		if out[i].Kind == rules.ModifierTempHP {
			out[i].Claim, out[i].Unclaimed = pool, pool == 0
		}
	}
	body, _ := json.Marshal(out)
	return body
}

// defAtSlot is the effect as a cast levelsAbove slot levels above the spell's own makes it: the
// damage of its trigger grows (Acid Arrow's later damage). The catalogue's entry is not changed.
func defAtSlot(def *rules.EffectDef, levelsAbove int) *rules.EffectDef {
	if def.Trigger == nil || def.Trigger.PerSlotLevel == "" || levelsAbove < 1 {
		return def
	}
	cast, trigger := *def, *def.Trigger
	trigger.Dice = trigger.DiceAt(levelsAbove)
	cast.Trigger = &trigger
	return &cast
}

// creatureKindsOf is the kind of creature an NPC was made from, by character, for the spells
// that pick a type.
func (s *Service) creatureKindsOf(ctx context.Context, c *combatTx, targs []playdb.Combatant) (map[string]string, error) {
	kinds := map[string]string{}
	var ids []string
	for _, t := range targs {
		if t.CharacterID != "" && t.MonsterKey == nil {
			ids = append(ids, t.CharacterID)
		}
	}
	if len(ids) == 0 {
		return kinds, nil
	}
	chars, err := s.roster.CombatCharacters(ctx, c.tx, c.session.CampaignID, ids)
	if err != nil {
		return nil, err
	}
	for _, ch := range chars {
		kinds[ch.ID] = ch.MonsterKey
	}
	return kinds, nil
}

// spellHolds are the targets an effect of a spell takes hold of: the ones it may affect (their kind of creature, the
// conditions they are immune to) and, for a base armor class, the ones that wear no armor. The hits say which.
func (s *Service) spellHolds(ctx context.Context, c *combatTx, content *rules.Content, def *rules.EffectDef, ability string, caster playdb.Combatant, targs []playdb.Combatant, made *actionEvent) ([]playdb.Combatant, error) {
	kinds, err := s.creatureKindsOf(ctx, c, targs)
	if err != nil {
		return nil, err
	}
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return nil, err
	}
	sheets, err := c.sheetImmunities(ctx, targs)
	if err != nil {
		return nil, err
	}
	taken := s.markSpellHits(content, def, states, kinds, sheets, targs, made)
	return s.dropWithoutHold(ctx, c, def, ability, caster, targs, taken, made)
}

// markSpellHits says, on each hit, whether the effect took hold of its target, and returns the
// targets it took hold of.
func (s *Service) markSpellHits(content *rules.Content, def *rules.EffectDef, states map[string][]playdb.CombatantState, kinds map[string]string, sheets map[string]link.Traits, targs []playdb.Combatant, made *actionEvent) []playdb.Combatant {
	var taken []playdb.Combatant
	for i, t := range targs {
		hit := &made.Hits[i]
		key := conv.OrZero(t.MonsterKey)
		if key == "" {
			key = kinds[t.CharacterID]
		}
		if why := s.noEffectWhy(content, def, key, sheets[t.ID].Intelligence); why != "" {
			hit.Lasting, hit.NoEffectWhy = lastingNoEffect, why
			continue
		}
		if slices.ContainsFunc(def.Conditions, func(k string) bool {
			return combat.ImmuneTo(effectModsOf(states, t.ID), k) || sheetImmuneTo(sheets[t.ID], k)
		}) {
			hit.Lasting, hit.NoEffectWhy = lastingNoEffect, whyImmune
			continue
		}
		// The spell asks a saving throw and holds the ones that failed it.
		if def.Applies == "failed_save" && (hit.Save == nil || hit.Save.Saved) {
			continue
		}
		// The spell makes an attack roll and holds the ones it hit.
		if def.Applies == "hit" && hit.Outcome != outcomeHit && hit.Outcome != outcomeCrit {
			continue
		}
		hit.Lasting = lastingApplied
		taken = append(taken, t)
	}
	return taken
}

// dropWithoutHold takes out of the targets an effect that sets a base armor class cannot take hold
// of (Mage Armor on a creature that wears armor, SRD 5.1) and marks their hits "no effect".
func (s *Service) dropWithoutHold(ctx context.Context, c *combatTx, def *rules.EffectDef, ability string, caster playdb.Combatant, targs, taken []playdb.Combatant, made *actionEvent) ([]playdb.Combatant, error) {
	if !hasBaseAC(def) {
		return taken, nil
	}
	opts, err := s.castOptsOf(ctx, c, def, ability, caster.CharacterID)
	if err != nil {
		return nil, err
	}
	var kept []playdb.Combatant
	for _, t := range taken {
		_, ok, err := s.modifiersForTarget(ctx, c, def, t, opts)
		if err != nil {
			return nil, err
		}
		if ok {
			kept = append(kept, t)
			continue
		}
		i := slices.IndexFunc(targs, func(o playdb.Combatant) bool { return o.ID == t.ID })
		made.Hits[i].Lasting, made.Hits[i].NoEffectWhy = lastingNoEffect, whyWearsArmor
	}
	return kept, nil
}

// spellEffectDuration is how long the effect of a spell lasts.
func spellEffectDuration(content *rules.Content, def *rules.EffectDef, spellKey string, caster playdb.Combatant) durationSpec {
	rounds := content.SpellEffectRounds(spellKey)
	switch {
	case def.Duration != nil:
		// The rider of an instantaneous spell keeps its own clock (Zombaria Viciosa: until the end
		// of the target's next turn). With no anchor, endsOf anchors it on each target, at the
		// target's next turn: this round when it has not acted yet, the next when it has.
		return durationSpec{Kind: def.Duration.Kind}
	case rounds > 0:
		return durationSpec{Kind: rules.EffectDurationRounds, Rounds: int32(rounds), Anchor: caster.ID} //nolint:gosec // 10 rounds a minute
	case def.Concentration:
		return durationSpec{Kind: rules.EffectDurationConcentration}
	}
	return durationSpec{Kind: rules.EffectDurationUntilDismissed}
}

// spellEffectGroups are the targets that get the effect together, each group with the modifiers
// it gets: all of them, or one by one for an effect that sets a base armor class.
func (s *Service) spellEffectGroups(ctx context.Context, c *combatTx, def *rules.EffectDef, taken []playdb.Combatant, opts castOpts) ([][]playdb.Combatant, [][]rules.EffectModifier, error) {
	common, _, err := s.modifiersFor(ctx, c, def, "", opts)
	if err != nil {
		return nil, nil, err
	}
	if !hasBaseAC(def) {
		return [][]playdb.Combatant{taken}, [][]rules.EffectModifier{common}, nil
	}
	var groups [][]playdb.Combatant
	var mods [][]rules.EffectModifier
	for _, t := range taken {
		m, ok, err := s.modifiersForTarget(ctx, c, def, t, opts)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			groups, mods = append(groups, []playdb.Combatant{t}), append(mods, m)
		}
	}
	return groups, mods, nil
}

// noEffectWhy says why a spell takes no hold of a target, "" when it may: a spell that picks a
// humanoid (Imobilizar Pessoa), a condition the creature is immune to, a creature too simple for
// Riso Histérico (SRD 5.1: an Intelligence score of 4 or less). A creature's own score (a steed has at least 6) is read when the sheet
// knows it, and the stat block's otherwise.
func (s *Service) noEffectWhy(content *rules.Content, def *rules.EffectDef, monsterKey string, score int) string {
	if monsterKey == "" {
		return ""
	}
	creature, known := content.CreatureByKey(monsterKey)
	if !known {
		return ""
	}
	if def.TargetType == "humanoid" && !combat.IsHumanoid(creature.Type) {
		return whyNotHumanoid
	}
	if slices.ContainsFunc(def.Conditions, func(k string) bool { return immuneCreature(creature, k) }) {
		return whyImmune
	}
	if def.Key == "spell:hideous-laughter" && steed.TooSimple(score, creature.Abilities) {
		return whyTooSimple
	}
	return ""
}

// immuneCreature says the creature is immune to the condition.
func immuneCreature(creature rules.Creature, condition string) bool {
	return slices.ContainsFunc(creature.ConditionImmunities, func(n rules.NamedKey) bool { return n.Key == condition })
}

// abilityChoiceOf checks the ability a spell is cast for: Enhance Ability needs one of the six
// (SRD 5.1: Bull's Strength, Cat's Grace, Bear's Endurance, Fox's Cunning, Owl's Wisdom, Eagle's
// Splendor); any other spell takes none and ignores it.
func (s *Service) abilityChoiceOf(ctx context.Context, c *combatTx, spellKey, ability string) (string, error) {
	asks, err := s.spellAsksAbility(ctx, c, spellKey)
	if err != nil || !asks {
		return "", err
	}
	if !slices.Contains(effectAbilities, ability) {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("ability_key must be one of str, dex, con, int, wis or cha for this spell"))
	}
	return ability, nil
}

// spellAsksAbility says the spell's effect holds an ability the caster chooses (Enhance Ability).
func (s *Service) spellAsksAbility(ctx context.Context, c *combatTx, spellKey string) (bool, error) {
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return false, err
	}
	def, ok := content.CombatSpellEffect(spellKey)
	return ok && slices.ContainsFunc(def.Modifiers, func(m rules.EffectModifier) bool { return m.Choose }), nil
}

// effectAbilities are the abilities an effect may be chosen for.
var effectAbilities = []string{"str", "dex", "con", "int", "wis", "cha"}
