package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"connectrpc.com/connect"

	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The armor class an effect gives (Mage Armor, SRD 5.1: "the target's base AC becomes 13 + its
// Dexterity modifier", for a willing creature that wears no armor; the spell ends if the target
// dons armor, and lasts 8 hours). It is an effect on the character like the others: a row of
// character_effects out of a combat, of combatant_states in it. The base armor class the effects
// give is worked out again, in the transaction of every change to them, to the character's vitals
// (what the session's panels read) and to its combatants (what the combat's rolls read).

// modifiersFor are the modifiers an effect of the catalog gives a character: the ones the file
// lists, with the armor class worked out for the target when the effect sets one. false means the
// effect does not take hold (Mage Armor on a creature that wears armor, or on a basic sheet).
func (s *Service) modifiersFor(ctx context.Context, c *combatTx, def *rules.EffectDef, characterID string, opts castOpts) ([]rules.EffectModifier, bool, error) {
	mods := slices.Clone(def.Modifiers)
	for i, m := range mods {
		switch {
		case m.Kind == rules.ModifierCheckAdvantage && m.Choose:
			mods[i].Abilities, mods[i].Choose = []string{opts.ability}, false
			continue
		case m.Kind == rules.ModifierTurnTempHP && m.FromCaster:
			mods[i].Value, mods[i].FromCaster = opts.casterMod, false
			continue
		case m.Kind != rules.ModifierBaseAC:
			continue
		}
		if characterID == "" {
			continue // worked out per target (modifiersForTarget): there is no one to read yet
		}
		mage, err := s.roster.MageArmorAC(ctx, c.tx, c.session.CampaignID, characterID)
		if err != nil {
			return nil, false, err
		}
		if !mage.Applies || mage.Wears || mage.AC < 1 {
			return nil, false, nil
		}
		mods[i].Value = int(clamp32(mage.AC, 1, 60))
	}
	return mods, true, nil
}

// mageArmorBaseAC is the 13 in Mage Armor's "base AC becomes 13 + Dexterity modifier" (SRD 5.1).
const mageArmorBaseAC = 13

// hasBaseAC says whether the effect sets a base armor class (Mage Armor).
func hasBaseAC(def *rules.EffectDef) bool {
	return def != nil && slices.ContainsFunc(def.Modifiers, func(m rules.EffectModifier) bool { return m.Kind == rules.ModifierBaseAC })
}

// modifiersForTarget is modifiersFor for a combatant: a character (a player's or an NPC with a
// sheet) is worked out from its sheet; a summoned creature has no sheet to wear armor on, so
// its base is 13 + its own Dexterity modifier, which its initiative bonus is.
func (s *Service) modifiersForTarget(ctx context.Context, c *combatTx, def *rules.EffectDef, t playdb.Combatant, opts castOpts) ([]rules.EffectModifier, bool, error) {
	if t.CharacterID != "" {
		return s.modifiersFor(ctx, c, def, t.CharacterID, opts)
	}
	mods, ok, err := s.modifiersFor(ctx, c, def, "", opts)
	if err != nil || !ok {
		return mods, ok, err
	}
	for i, m := range mods {
		if m.Kind == rules.ModifierBaseAC {
			mods[i].Value = int(clamp32(mageArmorBaseAC+int(t.InitiativeBonus), 1, 60))
		}
	}
	return mods, true, nil
}

// castOpts are what the cast decides about the modifiers of an effect: the ability the caster
// picked (Enhance Ability) and the caster's spellcasting modifier (Heroism's temporary hit points).
type castOpts struct {
	ability   string
	casterMod int
}

// castOptsOf reads the cast's options for an effect: the spellcasting modifier of the caster's
// character, when the effect needs it.
func (s *Service) castOptsOf(ctx context.Context, c *combatTx, def *rules.EffectDef, ability, casterCharacterID string) (castOpts, error) {
	opts := castOpts{ability: ability}
	if casterCharacterID == "" || !slices.ContainsFunc(def.Modifiers, func(m rules.EffectModifier) bool { return m.FromCaster }) {
		return opts, nil
	}
	st, err := s.roster.ReactionStats(ctx, c.tx, c.session.CampaignID, casterCharacterID)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return opts, nil
		}
		return opts, err
	}
	opts.casterMod = int(clamp32(max(st.CastingMod, 0), 0, 20))
	return opts, nil
}

// armorBaseOf is the base armor class the effects on a character give, in a combat or out of
// it, 0 for none.
func (s *Service) armorBaseOf(ctx context.Context, q *playdb.Queries, characterID string) (int32, error) {
	var best int
	out, err := q.ListCharacterEffects(ctx, []string{characterID})
	if err != nil {
		return 0, fmt.Errorf("list the character's effects: %w", err)
	}
	for _, r := range out {
		best = max(best, combat.BaseAC(modifiersOf(r.Modifiers)))
	}
	in, err := q.ListCombatEffectsOfCharacter(ctx, characterID)
	if err != nil {
		return 0, fmt.Errorf("list the effects in the combats: %w", err)
	}
	for _, st := range in {
		best = max(best, combat.BaseAC(effectModifiers(st)))
	}
	return clamp32(best, 0, 60), nil
}

// syncArmorBase works the base armor class of the characters out again and writes it to their
// vitals; the streams are told after the commit. A character that is no active player's (an NPC)
// has none.
func (s *Service) syncArmorBase(ctx context.Context, c *combatTx, characterIDs ...string) error {
	slices.Sort(characterIDs)
	for _, id := range slices.Compact(characterIDs) {
		if id == "" {
			continue
		}
		base, err := s.armorBaseOf(ctx, c.q, id)
		if err != nil {
			return err
		}
		before, after, err := s.vitals.SetArmorBase(ctx, c.tx, c.session.CampaignID, id, base)
		if connect.CodeOf(err) == connect.CodeNotFound {
			continue // an NPC, a dead or a reserved character: there are no vitals to show
		}
		if err != nil {
			return err
		}
		if after != nil && after != before {
			c.told = append(c.told, after)
		}
	}
	return nil
}

// gearBase works out, once, the base armor class Mage Armor gives a character from the gear it
// carries now, and refits the modifiers of the records with it.
type gearBase struct {
	s                       *Service
	tx                      pgx.Tx
	campaignID, characterID string
	mage                    *link.MageArmor
}

// worked is the base the roster says, read the first time it is asked.
func (g *gearBase) worked(ctx context.Context) (link.MageArmor, error) {
	if g.mage == nil {
		m, err := g.s.roster.MageArmorAC(ctx, g.tx, g.campaignID, g.characterID)
		if err != nil {
			return m, err
		}
		g.mage = &m
	}
	return *g.mage, nil
}

// refit works the base out again in the modifiers of one record; it says whether it changed.
func (g *gearBase) refit(ctx context.Context, body []byte) ([]byte, bool, error) {
	mods := modifiersOf(body)
	changed := false
	for i, m := range mods {
		if m.Kind != rules.ModifierBaseAC {
			continue
		}
		now, err := g.worked(ctx)
		if err != nil {
			return nil, false, err
		}
		if now.AC < 1 || now.Wears || int(clamp32(now.AC, 1, 60)) == m.Value {
			continue
		}
		mods[i].Value, changed = int(clamp32(now.AC, 1, 60)), true
	}
	if !changed {
		return body, false, nil
	}
	enc, err := json.Marshal(mods)
	if err != nil {
		return nil, false, fmt.Errorf("encode the modifiers: %w", err)
	}
	return enc, true, nil
}

// refitEffects works the base out again in the effects of the character, out of a combat and in
// the combats, and says whether any record changed.
func (g *gearBase) refitEffects(ctx context.Context, q *playdb.Queries, out []playdb.CharacterEffect, in []playdb.CombatantState) (bool, error) {
	touched := false
	for _, r := range out {
		body, changed, err := g.refit(ctx, r.Modifiers)
		if err != nil {
			return false, err
		}
		if !changed {
			continue
		}
		if err := q.SetCharacterEffectModifiers(ctx, playdb.SetCharacterEffectModifiersParams{ID: r.ID, Modifiers: body}); err != nil {
			return false, fmt.Errorf("work out the base armor class again: %w", err)
		}
		touched = true
	}
	for _, st := range in {
		body, changed, err := g.refit(ctx, st.Modifiers)
		if err != nil {
			return false, err
		}
		if !changed {
			continue
		}
		if err := q.SetLastingEffectModifiers(ctx, playdb.SetLastingEffectModifiersParams{ID: st.ID, Modifiers: body}); err != nil {
			return false, fmt.Errorf("work out the base armor class again: %w", err)
		}
		touched = true
	}
	return touched, nil
}

// openArmorTx opens the change of the master's that the sheet's gear makes: in the open session,
// or with none (only the campaign) when the session ended.
func (s *Service) openArmorTx(ctx context.Context, tx pgx.Tx, q *playdb.Queries, campaignID string) (*combatTx, error) {
	session, err := q.GetOpenGameSessionForUpdate(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		session = playdb.GameSession{CampaignID: campaignID}
	} else if err != nil {
		return nil, fmt.Errorf("lock the open session: %w", err)
	}
	return s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), svc: s, master: true})
}

// GearChanged implements characters.ReviewHost: the armor class Mage Armor gives is 13 + Dexterity
// with the shield the character carries now (SRD 5.1, Armor Class), so it is worked out again
// from the gear whenever the sheet's armor or shield changes, in a combat or out of it.
//
// It runs inside characters' InSession, with the session's row already held: what it tells goes out through t after the commit, with
// the write's number, and a change that touches nothing tells and moves nothing.
func (s *Service) GearChanged(ctx context.Context, tx pgx.Tx, campaignID, characterID string, t sessiontell.Tell) error {
	q := s.queries.WithTx(tx)
	out, err := q.ListCharacterEffects(ctx, []string{characterID})
	if err != nil {
		return fmt.Errorf("list the character's effects: %w", err)
	}
	in, err := q.ListCombatEffectsOfCharacter(ctx, characterID)
	if err != nil {
		return fmt.Errorf("list the effects in the combats: %w", err)
	}
	g := &gearBase{s: s, tx: tx, campaignID: campaignID, characterID: characterID}
	touched, err := g.refitEffects(ctx, q, out, in)
	if err != nil || !touched {
		return err
	}
	now, err := g.worked(ctx)
	if err != nil {
		return err
	}
	if err := q.SetMageArmorACOfCharacter(ctx, playdb.SetMageArmorACOfCharacterParams{CharacterID: characterID, MageArmorAc: new(clamp32(now.AC, 1, 60))}); err != nil {
		return fmt.Errorf("work out the base armor class of the combatants again: %w", err)
	}
	c, err := s.openArmorTx(ctx, tx, q, campaignID)
	if err != nil {
		return err
	}
	if err := s.syncArmorBase(ctx, c, characterID); err != nil {
		return err
	}
	told := c.told
	st := tellOf(t)
	st.touchSteps()
	st.later(func(ctx context.Context, say sayFn) {
		s.tellVitalsOf(say, campaignID, told)
		if c.enc.ID != "" {
			s.tellEncounterChanged(ctx, say, campaignID, c.enc)
		}
	})
	return nil
}

// ArmorWorn implements characters.ReviewHost: a character that dons armor loses what needs none
// (Mage Armor, SRD 5.1: "the spell ends if the target dons armor"). The effects that set a base
// armor class go, in a combat or out of it, the cast that held them ends, and the streams are
// told after the commit.
//
// It runs inside characters' InSession, with the session's row already held: what it tells goes out through t after the commit, with
// the write's number, and a change that ends nothing tells and moves nothing.
func (s *Service) ArmorWorn(ctx context.Context, tx pgx.Tx, campaignID, characterID string, t sessiontell.Tell) error {
	q := s.queries.WithTx(tx)
	c, err := s.openArmorTx(ctx, tx, q, campaignID)
	if err != nil {
		return err
	}
	casts, err := endArmorEffects(ctx, q, characterID)
	if err != nil || len(casts) == 0 {
		return err
	}
	if err := q.ClearMageArmorACOfCharacter(ctx, characterID); err != nil {
		return fmt.Errorf("take the base armor class off the combatants: %w", err)
	}
	slices.Sort(casts)
	for _, id := range slices.Compact(casts) {
		cast, err := q.GetSpellCast(ctx, playdb.GetSpellCastParams{ID: id, CampaignID: campaignID})
		if err == nil && cast.Status == castActive {
			if _, _, _, err := s.closeCast(ctx, c, cast, castEnded, endDismissed); err != nil {
				return err
			}
		}
	}
	if err := s.syncArmorBase(ctx, c, characterID); err != nil {
		return err
	}
	told := c.told
	st := tellOf(t)
	st.touchSteps()
	st.later(func(ctx context.Context, say sayFn) {
		s.tellVitalsOf(say, campaignID, told)
		say(campaignID, castsChangedEvent(false))
		if c.enc.ID != "" {
			s.tellEncounterChanged(ctx, say, campaignID, c.enc)
		}
	})
	return nil
}

// endArmorEffects takes away the effects of a character, out of a combat and in the combats, that
// set a base armor class; it returns the castings that held them.
func endArmorEffects(ctx context.Context, q *playdb.Queries, characterID string) ([]string, error) {
	var casts []string
	out, err := q.ListCharacterEffects(ctx, []string{characterID})
	if err != nil {
		return nil, fmt.Errorf("list the character's effects: %w", err)
	}
	for _, r := range out {
		if combat.BaseAC(modifiersOf(r.Modifiers)) == 0 {
			continue
		}
		if err := q.DeleteCharacterEffect(ctx, r.ID); err != nil {
			return nil, fmt.Errorf("end an effect that needs no armor: %w", err)
		}
		casts = append(casts, r.GroupID)
	}
	in, err := q.ListCombatEffectsOfCharacter(ctx, characterID)
	if err != nil {
		return nil, fmt.Errorf("list the effects in the combats: %w", err)
	}
	for _, st := range in {
		if combat.BaseAC(effectModifiers(st)) == 0 {
			continue
		}
		if err := q.DeleteLastingEffect(ctx, st.ID); err != nil {
			return nil, fmt.Errorf("end an effect that needs no armor: %w", err)
		}
		casts = append(casts, conv.OrZero(st.GroupID))
	}
	return casts, nil
}
