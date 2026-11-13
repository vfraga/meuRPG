package rules

import "slices"

// ShieldKey is the key under which the shield is carried or equipped (Build.Carried).
const ShieldKey = shieldKey

// ValidateCarried checks Build.Carried alone: each key is the sheet's armor, its shield
// (when it has one) or a weapon it lists, with no repeats, at most MaxCarried.
func ValidateCarried(b Build) error { return validateCarried(&b) }

// listsGear says key is the armor, the shield (when the sheet has one) or a weapon the
// sheet lists, equipped or carried.
func (b Build) listsGear(key string) bool {
	switch key {
	case "":
		return false
	case shieldKey:
		return b.Shield
	case b.Armor:
		return true
	}
	return slices.Contains(b.Weapons, key)
}

// IsCarried says the piece of gear key is carried, not equipped.
func (b Build) IsCarried(key string) bool { return slices.Contains(b.Carried, key) }

// ArmorOn is the armor key when it is equipped, or empty.
func (b Build) ArmorOn() string {
	if b.IsCarried(b.Armor) {
		return ""
	}
	return b.Armor
}

// ShieldOn says the sheet has a shield and it is equipped.
func (b Build) ShieldOn() bool { return b.Shield && !b.IsCarried(shieldKey) }

// ShieldCounts says the shield adds to d, the sheet derived from b: it is equipped and no
// two-handed weapon takes its hand.
func (b Build) ShieldCounts(d Derived) bool { return b.ShieldOn() && !d.ShieldIdle }

// equip leaves in the deriver's Build only the armor and the shield that are equipped,
// so everything downstream reads them as the sheet's gear. A carried armor key is never
// resolved, so an unknown one raises no issue until it is equipped. A carried weapon stays
// listed, to keep its index in the issues, and is skipped where weapons count.
func (x *deriver) equip() {
	x.sheetArmor = x.b.Armor
	x.b.Armor = x.b.ArmorOn()
	x.b.Shield = x.b.ShieldOn()
}
