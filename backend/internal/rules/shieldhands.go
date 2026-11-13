package rules

import "slices"

// twoHandedProperty is the weapon property of a weapon that needs both hands.
const twoHandedProperty = "weapon-property:two-handed"

// shieldKey is the source of the hint that says a shield adds nothing, and
// shieldACBonus the armor class a shield adds (SRD 5.1, Equipment).
const (
	shieldKey     = "equipment:shield"
	shieldACBonus = 2
)

// wieldsTwoHanded says a weapon that needs both hands is on the sheet. The sheet lists
// the weapons and has no notion of the hand each one is in, so a two-handed
// weapon on it is read as the one in hand, and the player puts the shield back to use
// by carrying that weapon instead of equipping it. A versatile weapon counts as one-handed: the
// sheet does not say whether the player grips it with both hands.
func (x *deriver) wieldsTwoHanded() bool {
	return slices.ContainsFunc(x.b.Weapons, func(key string) bool {
		eq, ok := x.c.equipment[key]
		return ok && !x.b.IsCarried(key) && eq.Weapon != nil && slices.Contains(eq.Weapon.Properties, twoHandedProperty)
	})
}

// shieldWielded says the shield on the sheet counts: it is equipped and no two-handed
// weapon takes the hand it needs.
func (x *deriver) shieldWielded() bool {
	return x.b.Shield && !x.wieldsTwoHanded()
}

// withShield adds the shield to the armor class ac under name, or leaves both as they
// are, with a hint that says why, when a two-handed weapon is on the sheet.
func (x *deriver) withShield(ac int, name string) (int, string) {
	if !x.shieldWielded() {
		x.d.ShieldIdle = true
		x.d.Hints = append(x.d.Hints, Hint{
			Source: shieldKey, Target: "ac", Targets: []string{"ac"}, Mode: "note",
			TextPT: "O escudo não conta na CA enquanto uma arma de duas mãos está equipada: guarde a arma de duas mãos.",
		})
		return ac, name
	}
	return ac + shieldACBonus, name + " + escudo"
}
