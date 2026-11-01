package play

import (
	"context"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Stroke of Luck on an attack (held_stroke.go): the weapon attack and the spell attack against one target tell the gate
// how they miss. The armor class stays on the server (RN-20); the player learns only that the attack missed, which the
// result would tell anyway. The dice an effect adds after the gate (Bênção, Perdição) move the total, so an attack
// with one asks on every roll and says nothing of the result.

// attackStrokeRule is the rule of an attack whose target's armor class `armor` reads (once, when the attacker has the
// feature with a use left) and whose critical hits start at criticalFrom (0: the SRD's 20): a miss is what
// ResolveAttackFrom says is not a hit. blind is set when dice come after the gate.
func attackStrokeRule(armor func() (int, error), criticalFrom int, blind bool) *strokeRule {
	var ac int
	return &strokeRule{
		Attack: true, Blind: blind,
		Prepare: func() (err error) { ac, err = armor(); return err },
		Failed:  func(face, total int) bool { return !combat.ResolveAttackFrom(total-face, ac, face, criticalFrom).Hit },
	}
}

// strokeMayApply says the attacker is a player's character, the only roller that asks Stroke of Luck.
func strokeMayApply(a playdb.Combatant) bool { return a.Kind == kindPlayer && backedByCharacter(a) }

// armorClass is the armor class the attack is compared with (the +5 of an active Escudo, the cover and the effects
// included), read once.
func (f *attackFlow) armorClass(ctx context.Context) (int, error) {
	if f.acRead {
		return f.targetAC, nil
	}
	targetSheet, err := f.s.sheetOf(ctx, f.c.tx, f.m.CampaignID, f.target)
	if err != nil {
		return 0, err
	}
	f.targetAC, f.acRead = targetSheet.ArmorClass+int(f.target.AcBonus)+int(f.target.EffectAcBonus)+f.cp.real.bonus(), true
	return f.targetAC, nil
}

// criticalFrom is the natural roll from which the attack always hits and is a critical hit (0: the SRD's 20).
func (f *attackFlow) criticalFrom() int {
	if f.attack.Spell { // Improved Critical and Superior Critical are for weapon attacks only
		return 0
	}
	return f.sheet.CriticalRange
}

// strokeSpec tells the gate how the attack misses, when its attacker is a player's character.
func (f *attackFlow) strokeSpec(ctx context.Context) {
	if !strokeMayApply(f.attacker) {
		return
	}
	blind := len(effectDiceFor(f.modeIn.states, f.attacker.ID, rules.RollAppliesAttack)) > 0
	f.spec.Stroke = attackStrokeRule(func() (int, error) { return f.armorClass(ctx) }, f.criticalFrom(), blind)
}

// strokeHit turns the miss into a hit when the player used Stroke of Luck: the d20 is not changed, so the hit is an
// ordinary one (not a critical hit, unless the target's state makes every hit one).
func (f *attackFlow) strokeHit() {
	if f.rolled.Stroke != nil && !f.result.Hit {
		f.result.Hit, f.result.Fumble = true, false
	}
}

// strokeBack gives back the use of Stroke of Luck the attack spent.
func (u *undoer) strokeBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if ev.Stroke == nil {
		return nil
	}
	return u.spendResourceBack(ctx, who, strokeResource)
}

// strokeArmor is the armor class of a single target of the spell attack the cast holds, with the cover of the cast's
// caster and the effects on the target: what resolveSpellAttack will compare the roll with.
func (f *castFlow) strokeArmor(ctx context.Context) func(playdb.Combatant) (int, error) {
	return func(target playdb.Combatant) (int, error) {
		cp, err := f.c.coverOf(ctx, f.v, f.terrain, f.known, f.caster, target, f.cs)
		if err != nil {
			return 0, err
		}
		cp = cp.ignoring(f.out.modes != nil && spellIgnoresCover(f.out.modes.traits, f.sp))
		sheet, err := f.s.sheetOf(ctx, f.c.tx, f.m.CampaignID, target)
		if err != nil {
			return 0, err
		}
		return sheet.ArmorClass + int(target.AcBonus) + int(target.EffectAcBonus) + cp.real.bonus(), nil
	}
}
