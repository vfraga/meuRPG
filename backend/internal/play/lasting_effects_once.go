package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The effects a single roll spends (Guidance: "the spell then ends"; Resistance likewise, SRD
// 5.1): the die is added to one roll, and the effect goes with it, with the casting that held it
// and the caster's concentration. The roll that used the die is the caller's; this runs in its
// transaction. The disadvantage Zombaria Viciosa gives the next attack roll is spent too, but its
// row stays until its clock ends it (spendAttackEffects).

// spendOnceEffects ends the effects whose die a roll used.
func (s *Service) spendOnceEffects(ctx context.Context, c *combatTx, dd []effectDie) error {
	for _, d := range dd {
		if !d.Once || d.EffectID == "" {
			continue
		}
		if err := s.spendOne(ctx, c, d); err != nil {
			return err
		}
	}
	return nil
}

// spendOne ends the effect a roll used.
func (s *Service) spendOne(ctx context.Context, c *combatTx, d effectDie) error {
	// Out of a combat the effect is a row of the character's.
	row, err := c.q.GetCharacterEffect(ctx, playdb.GetCharacterEffectParams{ID: d.EffectID, CampaignID: c.session.CampaignID})
	if err == nil {
		return s.spendCharacterEffect(ctx, c, row)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find the effect the roll spent: %w", err)
	}
	st, found, err := s.spentCombatState(ctx, c, d)
	if err != nil || !found {
		return err
	}
	return s.endSpentState(ctx, c, st)
}

// spentCombatState finds the effect a roll spent among the states of the character's combatant:
// in the combat the change runs in, or in the character's combat that is not ended. false when
// there is no such effect.
func (s *Service) spentCombatState(ctx context.Context, c *combatTx, d effectDie) (playdb.CombatantState, bool, error) {
	if c.enc.ID != "" {
		st, err := c.q.GetLastingEffect(ctx, playdb.GetLastingEffectParams{ID: d.EffectID, EncounterID: c.enc.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return st, false, nil
		}
		if err != nil {
			return st, false, fmt.Errorf("find the effect the roll spent: %w", err)
		}
		return st, true, nil
	}
	if d.CharacterID == "" {
		return playdb.CombatantState{}, false, nil
	}
	in, err := c.q.ListCombatEffectsOfCharacter(ctx, d.CharacterID)
	if err != nil {
		return playdb.CombatantState{}, false, fmt.Errorf("list the effects in the combats: %w", err)
	}
	i := slices.IndexFunc(in, func(o playdb.CombatantState) bool { return o.ID == d.EffectID })
	if i < 0 {
		return playdb.CombatantState{}, false, nil
	}
	return in[i], true, nil
}

// endSpentState ends a state of a combat the roll spent, in that combat.
func (s *Service) endSpentState(ctx context.Context, c *combatTx, st playdb.CombatantState) error {
	enc, err := c.q.GetEncounterByID(ctx, st.EncounterID)
	if err != nil {
		return fmt.Errorf("find the combat of the effect: %w", err)
	}
	cc := *c
	cc.enc = enc
	cs, err := cc.q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	stopped, err := s.stopHeldConcentration(ctx, &cc, cs, st)
	if err != nil {
		return err
	}
	if !stopped {
		if err := s.endEffectRows(ctx, &cc, cs, []playdb.CombatantState{st}, endUsed); err != nil {
			return err
		}
	}
	c.told = cc.told
	return nil
}

// stopHeldConcentration has the caster stop concentrating on the spell the state is: the spell
// ended. It says whether the caster was concentrating on it.
func (s *Service) stopHeldConcentration(ctx context.Context, cc *combatTx, cs []playdb.Combatant, st playdb.CombatantState) (bool, error) {
	if !st.Concentration || st.SourceID == nil {
		return false, nil
	}
	j := slices.IndexFunc(cs, func(o playdb.Combatant) bool {
		return o.ID == *st.SourceID && o.ConcentrationSpell != nil && *o.ConcentrationSpell == deref(st.SourceKey)
	})
	if j < 0 {
		return false, nil
	}
	if err := s.endConcentrationWithLine(ctx, cc, cs[j]); err != nil {
		return false, err
	}
	return true, nil
}

// spendAttackEffects spends the effects on the attacker whose disadvantage its attack roll used
// (SRD 5.1, Vicious Mockery: the next attack roll the creature makes), hit or miss and whatever
// mode the roll had: it was that roll. The row stays, spent, until its clock ends it (the end of
// the target's next turn), so the master's undo of the roll gives the disadvantage back
// (made.EffectsSpent, restoreContestState); a spent row gives none (effectAttackDisadvantage).
func (s *Service) spendAttackEffects(ctx context.Context, c *combatTx, attacker playdb.Combatant, made *actionEvent) error {
	rows, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	for _, st := range rows {
		if st.CombatantID != attacker.ID || st.SpentAt != nil || !spentByAttack(st) {
			continue
		}
		if err := c.q.SetLastingEffectSpent(ctx, playdb.SetLastingEffectSpentParams{ID: st.ID, SpentAt: &c.now}); err != nil {
			return fmt.Errorf("spend the effect: %w", err)
		}
		made.EffectsSpent = append(made.EffectsSpent, st.ID)
	}
	return nil
}

// spentByAttack says the effect gives a disadvantage the next attack roll uses up.
func spentByAttack(st playdb.CombatantState) bool {
	return slices.ContainsFunc(effectModifiers(st), func(m rules.EffectModifier) bool {
		return m.Kind == rules.ModifierAttackDisadvantage && m.Once
	})
}

// markSpent marks the rows of ids spent in a read of the states, so a later attack roll of the
// same change (a spell's second attack) reads them as the database now has them.
func markSpent(states map[string][]playdb.CombatantState, id string, ids []string, at time.Time) {
	for i, st := range states[id] {
		if slices.Contains(ids, st.ID) {
			states[id][i].SpentAt = &at
		}
	}
}

// spendCharacterEffect ends an effect on a character out of a combat: the whole casting goes
// with the cast that held it.
func (s *Service) spendCharacterEffect(ctx context.Context, c *combatTx, row playdb.CharacterEffect) error {
	if cast, err := c.q.GetSpellCast(ctx, playdb.GetSpellCastParams{ID: row.GroupID, CampaignID: c.session.CampaignID}); err == nil && cast.Status == castActive {
		_, _, _, err := s.closeCast(ctx, c, cast, castEnded, endDismissed)
		return err
	}
	if err := c.q.DeleteCharacterEffect(ctx, row.ID); err != nil {
		return fmt.Errorf("end the effect the roll spent: %w", err)
	}
	return s.syncArmorBase(ctx, c, row.CharacterID)
}
