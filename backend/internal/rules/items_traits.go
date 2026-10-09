package rules

import (
	"slices"
	"strings"
)

// What the inventory rules need to know of an item, worked out from the SRD
// content and effects/items.json.

// The kinds of line of an inventory.
const (
	ItemFree      = "free"
	ItemEquipment = "equipment"
	ItemMagic     = "magic"
)

// EquipmentInfo is an item of the SRD equipment as the inventory lists it.
type EquipmentInfo struct {
	Key, Name, NamePT string
	// Kind is "armor", "shield", "weapon", "tool" or "gear"; Category the armor's
	// ("light", "medium", "heavy") or the weapon's ("simple", "martial").
	Kind, Category string
	// Ammunition says it is ammunition; PackQuantity is the pieces of one purchase.
	Ammunition   bool
	PackQuantity int
}

func (c *content) equipmentInfo(key string) (EquipmentInfo, bool) {
	e, ok := c.equipment[key]
	if !ok {
		return EquipmentInfo{}, false
	}
	info := EquipmentInfo{Key: key, Name: e.Name, NamePT: c.namePT(key), Kind: e.Kind}
	switch {
	case e.Armor != nil && e.Armor.Category == "shield":
		info.Kind = "shield"
	case e.Armor != nil:
		info.Category = e.Armor.Category
	case e.Weapon != nil:
		info.Category = e.Weapon.Category
	case e.Gear != nil:
		info.Ammunition, info.PackQuantity = e.Gear.Ammunition, e.Gear.PackQuantity
	}
	return info, true
}

// Equipment returns an item of the SRD equipment ("equipment:longsword").
func (c *Content) Equipment(key string) (EquipmentInfo, bool) { return c.c.equipmentInfo(key) }

// EquipmentList returns every item of the SRD equipment the inventory can hold, sorted
// by Portuguese name.
func (c *Content) EquipmentList() []EquipmentInfo {
	out := make([]EquipmentInfo, 0, len(c.c.equipment))
	for _, k := range sortedKeys(c.c.equipment) {
		info, _ := c.c.equipmentInfo(k)
		out = append(out, info)
	}
	sortPT(out, func(e EquipmentInfo) string { return e.NamePT })
	return out
}

// ItemTraits is what the inventory rules ask of one line.
type ItemTraits struct {
	// Kind is ItemFree, ItemEquipment or ItemMagic; Known is false for a key the
	// content does not have.
	Kind  string
	Known bool
	// Slot is where it is worn ("" for none); Equippable says it can be worn,
	// wielded or quivered at all.
	Slot       string
	Equippable bool
	// Stackable says several pieces share a line; Ammunition that it is fired from a
	// weapon.
	Stackable, Ammunition bool
	// Usable says it is drunk or read (a potion, a scroll).
	Usable bool
	// RequiresAttunement and Restriction are the magic item's; Cursed says its
	// attunement cannot be ended by choice.
	RequiresAttunement bool
	Restriction        Restriction
	AttunementByPT     string
	Cursed             bool
	// Def is what the engine applies of a magic item, nil for a reminder or any
	// other line.
	Def *ItemDef
}

// Traits works out the traits of an inventory line.
func (c *Content) Traits(it Item) ItemTraits {
	switch {
	case strings.HasPrefix(it.Key, "equipment:"):
		return c.c.equipmentTraits(it)
	case strings.HasPrefix(it.Key, "item:"):
		return c.c.magicTraits(it)
	}
	return ItemTraits{Kind: ItemFree, Known: it.Key == "", Stackable: true}
}

func (c *content) equipmentTraits(it Item) ItemTraits {
	t := ItemTraits{Kind: ItemEquipment, Stackable: true}
	info, ok := c.equipmentInfo(it.Key)
	if !ok {
		return t
	}
	t.Known = true
	switch info.Kind {
	case "armor":
		t.Slot, t.Equippable = SlotBody, true
	case "shield":
		t.Slot, t.Equippable = SlotShield, true
	case "weapon":
		t.Slot, t.Equippable = SlotHand, true
	}
	if info.Ammunition {
		t.Equippable, t.Ammunition = true, true // quivered
	}
	return t
}

func (c *content) magicTraits(it Item) ItemTraits {
	t := ItemTraits{Kind: ItemMagic}
	m, ok := c.magicItems[it.Key]
	if !ok {
		return t
	}
	t.Known = true
	t.Def = c.items.defs[it.Key]
	t.RequiresAttunement = m.Attunement
	t.Restriction = ParseRestriction(m.AttunementBy)
	t.AttunementByPT = c.attunementPT(m.AttunementBy)
	t.Cursed = slices.ContainsFunc(m.Desc, func(line string) bool { return strings.Contains(line, "***Curse") })
	t.Ammunition = m.Category == "ammunition"
	if t.Def != nil {
		t.Slot = t.Def.Slot
		if t.Def.Use != nil {
			t.Usable = true
		}
	}
	consumable := c.consumables[it.Key]
	t.Usable = t.Usable || m.Category == "potion" || m.Category == "scroll"
	t.Equippable = !consumable || t.Ammunition
	t.Stackable = consumable && !it.Unidentified // identical unidentified items never stack
	// A generic armor or shield takes the slot of its base.
	if t.Def != nil && (t.Def.BaseFixed != "" || it.Base != "") {
		base := t.Def.BaseFixed
		if base == "" {
			base = it.Base
		}
		if info, ok := c.equipmentInfo(base); ok {
			switch info.Kind {
			case "shield":
				t.Slot = SlotShield
			case "armor":
				t.Slot = SlotBody
			}
		}
	}
	return t
}

// Cursed says an SRD magic item is cursed: its description has a "Curse" paragraph.
func (c *Content) Cursed(key string) bool { return c.c.magicTraits(Item{Key: key}).Cursed }

// SameStack says two lines are the same thing and share a line when they are
// stackable: the same key, base, option, scroll spell, look and name.
func SameStack(a, b Item, scrollA, scrollB string) bool {
	return a.Key == b.Key && a.Base == b.Base && a.Option == b.Option && a.Name == b.Name &&
		a.Unidentified == b.Unidentified && a.Look == b.Look && scrollA == scrollB
}

// SpellLevel is the level of an SRD spell (0 for a cantrip), and false for a key that
// is not a spell.
func (c *Content) SpellLevel(key string) (int, bool) {
	s, ok := c.c.spells[key]
	if !ok {
		return 0, false
	}
	return s.Level, true
}

// ScrollReader says what a character can do with a scroll of a spell (SRD 5.1 "Spell
// Scroll"): readable is false when the spell is on none of the character's class lists (the
// scroll is unintelligible); tooHigh is true when it is of a higher level than any class that
// has it on its list can cast, which takes an ability check with the spellcasting ability
// (the best of the classes that have the spell), whose modifier is mod.
func (c *Content) ScrollReader(d Derived, spellKey string) (readable, tooHigh bool, ability Ability, mod int) {
	s, ok := c.c.spells[spellKey]
	if !ok {
		return false, false, "", 0
	}
	best := -1 << 30
	canCast := false
	for _, sc := range d.Spellcasting {
		if !c.c.onList(s, sc.SpellList) {
			continue
		}
		readable = true
		if s.Level <= sc.MaxSpellLevel {
			canCast = true
		}
		for _, a := range d.Abilities {
			if a.Ability == sc.Ability && a.Modifier > best {
				best, ability, mod = a.Modifier, a.Ability, a.Modifier
			}
		}
	}
	return readable, readable && !canCast, ability, mod
}

// CategoryNamePT is the Portuguese label of a magic item category ("Item
// maravilhoso") or of an equipment kind ("Arma"); kind is "item" or "equipment".
func (c *Content) CategoryNamePT(kind, category string) string {
	return c.c.namesPT[kind+"-"+map[string]string{"item": "category", "equipment": "kind"}[kind]+":"+category]
}
