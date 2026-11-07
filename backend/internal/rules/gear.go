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
	// Count is how many of the piece the sheet lists: weapons with the same key are one row, because the state
	// (equipped or carried) is kept by key, so the copies always move together. 1 for armor and shield.
	Count int
}

// DeriveGear lists the gear of b, derived as d (what Derive returned for b). It is not part of Derive: it derives the
// sheet again for each piece whose flipping can change the armor class, so only the sheet's projection asks for it.
func DeriveGear(b Build, c *Content, d Derived) []GearPiece {
	return gearOf(b, c.c, d)
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
		p := GearPiece{Key: key, NamePT: name, Kind: kind, Equipped: !b.IsCarried(key), TwoHanded: twoHanded, Count: 1}
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
	seen := map[string]bool{}
	for _, key := range b.Weapons {
		if seen[key] {
			continue // a second copy is the same row: its state is the key's
		}
		seen[key] = true
		eq, ok := c.equipment[key]
		add(key, GearWeapon, ok && eq.Weapon != nil && slices.Contains(eq.Weapon.Properties, twoHandedProperty))
		out[len(out)-1].Count = countOf(b.Weapons, key)
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

func countOf(keys []string, key string) int {
	n := 0
	for _, k := range keys {
		if k == key {
			n++
		}
	}
	return n
}
