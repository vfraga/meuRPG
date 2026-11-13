package play

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The effects on a character count in every roll it makes (RN-22), in a combat and out of it:
// the checks of an RP scene, a puzzle's hint, the search for traps, the saving throws a scene
// asks. Out of a combat the character's record is a row of character_effects.

// outsideModeOf works out the mode, the dice and the hidden origins of a roll of a character
// that is not a combatant of a running combat.
func (s *Service) outsideModeOf(ctx context.Context, tx pgx.Tx, campaignID, characterID, skillKey, ability string, check bool) (checkMode, error) {
	rows, err := s.queriesIn(tx).ListCharacterEffects(ctx, []string{characterID})
	if err != nil {
		return checkMode{}, fmt.Errorf("list the character's effects: %w", err)
	}
	level := 0
	if v, err := s.vitals.GetVitalsTx(ctx, tx, campaignID, characterID); err == nil {
		level = int(v.GetExhaustionLevel())
	} else if connect.CodeOf(err) != connect.CodeNotFound {
		return checkMode{}, err
	}
	untrained := s.wearsUntrainedArmor(ctx, tx, campaignID, characterID)
	if len(rows) == 0 && level == 0 && !untrained {
		return checkMode{}, nil
	}
	of := outsideEffectsOf(rows, skillKey, check)
	of.creature.Exhaustion, of.creature.ArmorUntrained = level, untrained
	sources := combat.SaveMode(combat.SaveScene{Creature: of.creature, Ability: ability, Check: check, EffectVisible: true})
	return checkMode{Mode: combat.Resolve(sources), Sources: sources, Dice: of.dice, Bonuses: of.bonuses, hide: hideFor(of.hiddenConds, of.hiddenAny)}, nil
}

// wearsUntrainedArmor says the character wears armor or a shield it is not proficient with, which
// counts in its rolls out of a combat as in one. A sheet that cannot be read adds no circumstance.
func (s *Service) wearsUntrainedArmor(ctx context.Context, tx pgx.Tx, campaignID, characterID string) bool {
	sheet, err := s.roster.CombatSheet(ctx, tx, campaignID, characterID)
	return err == nil && sheet.Traits.ArmorUntrained
}

// outsideEffects is what the effects on a character out of a combat give to a roll.
type outsideEffects struct {
	creature    combat.Creature
	hiddenConds []string
	hiddenAny   bool
	dice        []effectDie
	bonuses     []effectBonus
}

// outsideEffectsOf reads the effects on a character out of a combat for a check or a saving throw.
func outsideEffectsOf(rows []playdb.CharacterEffect, skillKey string, check bool) outsideEffects {
	var of outsideEffects
	applies := rules.RollAppliesSave
	if check {
		applies = rules.RollAppliesCheck
	}
	for _, r := range rows {
		mods := modifiersOf(r.Modifiers)
		n := combat.RollBonus(mods, applies)
		if check {
			n += combat.CheckBonus(mods, skillKey)
		}
		if n != 0 {
			of.bonuses = append(of.bonuses, effectBonus{Key: r.SourceKey, Value: n, Hidden: !r.PlayerVisible, OwnerOnly: r.Audience == audienceOwner})
		}
		of.creature.Conditions = append(of.creature.Conditions, r.ConditionKeys...)
		addAbilityEffects(&of.creature, mods)
		for _, d := range combat.EffectDice(r.SourceKey, mods, applies) {
			of.dice = append(of.dice, effectDie{Key: d.Source, Faces: d.Faces, Sign: d.Sign, Hidden: !r.PlayerVisible, Once: d.Once, EffectID: r.ID, CharacterID: r.CharacterID})
		}
		if !r.PlayerVisible {
			of.hiddenAny = true
			of.hiddenConds = append(of.hiddenConds, r.ConditionKeys...)
		}
	}
	return of
}

// addAbilityEffects puts on the creature the abilities the modifiers give advantage or disadvantage on.
func addAbilityEffects(c *combat.Creature, mods []rules.EffectModifier) {
	for _, m := range mods {
		switch m.Kind {
		case rules.ModifierSaveAdvantage:
			c.SaveAdvantage = append(c.SaveAdvantage, m.Abilities...)
		case rules.ModifierCheckAdvantage:
			c.CheckAdvantage = append(c.CheckAdvantage, m.Abilities...)
		case rules.ModifierSaveDisadvantage:
			c.SaveDisadvantage = append(c.SaveDisadvantage, m.Abilities...)
		case rules.ModifierCheckDisadvantage:
			c.CheckDisadvantage = append(c.CheckDisadvantage, m.Abilities...)
		}
	}
}

// hideFor says which sources the roller may not read the origin of: those that come from an effect the master hides,
// and the sun's.
func hideFor(conds []string, anyHidden bool) func(combat.Source) bool {
	return func(src combat.Source) bool {
		if isSunlight(src) {
			return true // the sun and its trait are the master's (RN-20, RN-29): the roller reads "Outra fonte"
		}
		switch src.Kind {
		case combat.SourceOutlinedTarget, combat.SourceEffectSave, combat.SourceEffectCheck:
			return anyHidden
		}
		return src.Condition != "" && slices.Contains(conds, src.Condition)
	}
}

// modifiersOf reads the modifiers of an effect's record.
func modifiersOf(body []byte) []rules.EffectModifier {
	st := playdb.CombatantState{Modifiers: body}
	return effectModifiers(st)
}

// withEffectDice rolls the dice the effects add to the roll (the app rolls them) and returns the
// bonus with them: Bênção's d4 on a saving throw. The dice and where they came from are kept for
// the sources of the roll.
func (s *Service) withEffectDice(in rollInput, typed []int32, cm *checkMode, bonus int) (int, error) {
	for _, b := range cm.Bonuses {
		bonus += b.Value
	}
	if len(cm.Dice) == 0 {
		return bonus, nil
	}
	rolled, delta, err := s.rollEffectDice(in, cm.Dice, typed)
	if err != nil {
		return bonus, err
	}
	cm.Rolled = rolled
	return bonus + delta, nil
}

// shownCheck writes the sources of a check or a saving throw: the mode's, and each die an
// effect added ("Bênção +1d4: 3"). What an effect the master hides gives reads "Outra fonte".
func (cm checkMode) shownCheck(names func(string) string) []*playv1.AdvantageSource {
	out := append(shownSourcesHiding(cm.Sources, names, cm.hide), bonusSources(cm.Bonuses, names, false, true)...)
	return append(out, effectDieSources(cm.Rolled, names, false)...)
}

// effectDieSources writes the dice an effect added to a roll as sources ("Bênção +1d4: 3"). What an
// effect the master hides gives reads "Outra fonte" to a player; the master reads the name.
func effectDieSources(dd []effectDie, names func(string) string, master bool) []*playv1.AdvantageSource {
	var out []*playv1.AdvantageSource
	for _, d := range dd {
		name, sign := names(d.Key), "+"
		if d.Sign < 0 {
			sign = "-"
		}
		if d.Hidden && !master {
			name = "Outra fonte"
		}
		out = append(out, &playv1.AdvantageSource{
			Kind: playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_EFFECT_DIE, TextPt: fmt.Sprintf("%s %s1d%d: %d", name, sign, d.Faces, d.Face),
		})
	}
	return out
}

// effectBonus is a fixed bonus an effect adds to a roll.
type effectBonus struct {
	Key    string
	Value  int
	Hidden bool
	// OwnerOnly says only the master and the character's player read the effect (RN-10).
	OwnerOnly bool
}

// outsideSaveDice are the dice the effects on a character out of a combat add to its saving
// throws (Bênção, Perdição), for a trap that fires on it.
func (s *Service) outsideSaveDice(ctx context.Context, tx pgx.Tx, characterID string) ([]effectDie, error) {
	rows, err := s.queriesIn(tx).ListCharacterEffects(ctx, []string{characterID})
	if err != nil {
		return nil, fmt.Errorf("list the character's effects: %w", err)
	}
	var out []effectDie
	for _, r := range rows {
		for _, d := range combat.EffectDice(r.SourceKey, modifiersOf(r.Modifiers), rules.RollAppliesSave) {
			out = append(out, effectDie{Key: d.Source, Faces: d.Faces, Sign: d.Sign, Hidden: !r.PlayerVisible, Once: d.Once, EffectID: r.ID, CharacterID: r.CharacterID})
		}
	}
	return out, nil
}

// outsideSaveBonus is the flat bonus the effects on a character out of a combat add to its saving
// throws (a table spell's roll_bonus), for a trap that fires on it.
func (s *Service) outsideSaveBonus(ctx context.Context, tx pgx.Tx, characterID string) (int, error) {
	rows, err := s.queriesIn(tx).ListCharacterEffects(ctx, []string{characterID})
	if err != nil {
		return 0, fmt.Errorf("list the character's effects: %w", err)
	}
	n := 0
	for _, r := range rows {
		n += combat.RollBonus(modifiersOf(r.Modifiers), rules.RollAppliesSave)
	}
	return n, nil
}
