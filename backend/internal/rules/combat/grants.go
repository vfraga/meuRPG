package combat

import (
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// SpellGrant is how a spell a granted_spell effect gives is cast: without a slot,
// spending a use of Resource unless the grant is at will. The spell attack bonus and the
// save DC are the grant's own, from its casting ability.
type SpellGrant struct {
	// Source is the key of the trait, feat or feature that grants the spell, and SourcePT its name.
	Source, SourcePT string
	AtWill           bool
	// Resource is the resource a cast spends one of, and UsesLeft how many uses remain (0 for an
	// at-will grant).
	Resource string
	UsesLeft int
	// Cost is the uses of Resource every cast spends (1 or more; 0 for an at-will grant), and
	// NoMaterialComponents says the grant waives the material components (display only).
	Cost                 int
	NoMaterialComponents bool
	// Recharge is when the resource comes back (a rules.Recharge* constant), for the refusal.
	Recharge string
	// AlsoWithSlot says a spell slot casts it too.
	AlsoWithSlot bool
	// Ability is the casting ability; AttackBonus and SaveDC follow from it.
	Ability     rules.Ability
	AttackBonus int
	SaveDC      int
}

// GrantOf is the SpellGrant of a granted spell, with the uses the character has left.
func GrantOf(d rules.Derived, u Usage, g rules.GrantedSpell) SpellGrant {
	out := SpellGrant{
		Source: g.Source, SourcePT: g.SourcePT, AtWill: g.AtWill, Resource: g.Resource, AlsoWithSlot: g.AlsoWithSlot,
		Cost: g.Cost, NoMaterialComponents: g.NoMaterialComponents, Ability: g.Ability, AttackBonus: g.AttackBonus, SaveDC: g.SaveDC,
	}
	if !g.AtWill {
		out.UsesLeft, _ = ResourceLeft(d, u, g.Resource)
		out.Recharge = rechargeOf(d, g.Resource)
	}
	return out
}

// castable says the grant can be cast now without a slot: it is at will, or a use is left.
func (g SpellGrant) castable() bool { return g.AtWill || g.UsesLeft >= max(g.Cost, 1) }

// grantedOptions are the spells the character's grants give, as options of the turn. A
// grant whose casting ability is not picked yet, and a grant that is only for rituals (a
// ritual is too long for a fight), are left out. A grant offers slots only when it allows them.
func grantedOptions(d rules.Derived, turn TurnState, u Usage) []SpellOption {
	var out []SpellOption
	for _, g := range d.GrantedSpells {
		if g.AbilityPending || g.RitualOnly {
			continue
		}
		grant := GrantOf(d, u, g)
		o := SpellOption{Spell: g.Spell, Economy: spellEconomy(g.Spell.CastingTime), Grant: &grant, FeatureSummon: g.FeatureSummon}
		if grant.AlsoWithSlot && g.Spell.Level > 0 {
			o.Slots = slotChoices(d, u, g.Spell.Level)
		}
		o.Reason = spellReason(turn, o, g.Spell)
		o.Enabled = o.Reason == nil
		out = append(out, o)
	}
	return out
}

// grantReason is why a granted spell cannot be cast now, once the turn allows it: no use is
// left, and no slot is there for a grant that allows one.
func grantReason(o SpellOption) *Reason {
	if o.Grant.castable() || len(o.Slots) > 0 {
		return nil
	}
	return &Reason{Code: ReasonNoUses, Recharge: o.Grant.Recharge}
}
