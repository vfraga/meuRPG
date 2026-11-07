package rules

import "slices"

// Kinds of a GearPiece.
const (
	GearArmor  = "armor"
	GearShield = "shield"
	GearWeapon = "weapon"
)

// GearPiece is one piece of the gear a sheet lists, equipped or carried, as the sheet's gear list reads it.
type GearPiece struct {
	Key    string
	NamePT string
	// Kind is GearArmor, GearShield or GearWeapon.
	Kind     string
	Equipped bool
	// TwoHanded says the piece is a weapon that takes both hands.
	TwoHanded bool
	// ArmorClassIfToggled is the sheet's armor class with this piece's state flipped (equipped to carried, or the
	// other way round), when flipping it changes the armor class; zero otherwise. Spells and effects are not
	// counted, as in Derived.ArmorClass.
	ArmorClassIfToggled int
}

// gearOf lists the gear of b, derived as d: the armor, the shield and the weapons in that order. A piece whose key
// the catalog does not know is listed under its key.
func gearOf(b Build, c *content, d Derived) []GearPiece {
	var out []GearPiece
	add := func(key, kind string, twoHanded bool) {
		name := c.namePT(key)
		if name == "" {
			name = key
		}
		p := GearPiece{Key: key, NamePT: name, Kind: kind, Equipped: !b.IsCarried(key), TwoHanded: twoHanded}
		if kind != GearWeapon || twoHanded {
			if ac := derive(toggled(b, key), c).ArmorClass; ac != d.ArmorClass {
				p.ArmorClassIfToggled = ac
			}
		}
		out = append(out, p)
	}
	if b.Armor != "" {
		add(b.Armor, GearArmor, false)
	}
	if b.Shield {
		add(shieldKey, GearShield, false)
	}
	for _, key := range b.Weapons {
		eq, ok := c.equipment[key]
		add(key, GearWeapon, ok && eq.Weapon != nil && slices.Contains(eq.Weapon.Properties, twoHandedProperty))
	}
	return out
}

// toggled is b with the piece key equipped if it was carried, or carried if it was equipped.
func toggled(b Build, key string) Build {
	if b.IsCarried(key) {
		b.Carried = slices.DeleteFunc(slices.Clone(b.Carried), func(k string) bool { return k == key })
		return b
	}
	b.Carried = append(slices.Clone(b.Carried), key)
	return b
}
