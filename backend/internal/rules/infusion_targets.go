package rules

import "slices"

// What the infusion calls read of the content and the gear: which piece an infusion goes on, and the text of an option.

// InfusionTakesObject says an infusion of this kind goes on an object the player names, and on no piece of the sheet:
// the sheet lists only armor, a shield and weapons, so a spellcasting focus and any other object are not on it.
func InfusionTakesObject(accepts string) bool {
	return accepts == InfusionAcceptsAnyObject || accepts == InfusionAcceptsSpellcastingFocus
}

// InfusionAcceptsGear says the piece of gear key, of kind GearArmor, GearShield or GearWeapon, is of the kind accepts asks.
// A thrown weapon is a weapon with the property thrown, and a weapon with ammunition one with the property ammunition.
func (c *Content) InfusionAcceptsGear(accepts, kind, key string) bool {
	switch accepts {
	case InfusionAcceptsWeapon:
		return kind == GearWeapon
	case InfusionAcceptsArmor:
		return kind == GearArmor
	case InfusionAcceptsShield:
		return kind == GearShield
	case InfusionAcceptsArmorOrShield:
		return kind == GearArmor || kind == GearShield
	case InfusionAcceptsThrownWeapon:
		return kind == GearWeapon && c.weaponHas(key, weaponPropertyPrefix+"thrown")
	case InfusionAcceptsAmmunitionWeapon:
		return kind == GearWeapon && c.weaponHas(key, weaponPropertyAmmunition)
	}
	return false
}

// weaponHas says the weapon key has the property.
func (c *Content) weaponHas(key, property string) bool {
	eq, ok := c.c.equipment[key]
	return ok && eq.Weapon != nil && slices.Contains(eq.Weapon.Properties, property)
}

// FeatureTextPT is the Portuguese text of a feature or an option, one paragraph per entry; nil for a key with none.
func (c *Content) FeatureTextPT(key string) []string {
	f, ok := c.c.features[key]
	if !ok {
		return nil
	}
	text, _, _ := c.c.textPT(key, f.Desc)
	return text
}

// MagicalGear says the gear key already counts as a magic item before an infusion: the sheet gives it a magic bonus
// (bonus is that number, 0 when the entry has none) or marks the weapon magical (marked).
func MagicalGear(b Build, key string) (bonus int, magical bool) {
	if ib, ok := b.itemBonus(key); ok {
		return ib.Bonus, true
	}
	for _, m := range b.WeaponMarks {
		if m.Weapon == key && m.Magical {
			return 0, true
		}
	}
	return 0, false
}
