package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// IssueInventory is the code of a problem with an item on the sheet: a key the
// content does not have, two suits of armor worn, a base the item cannot have.
const IssueInventory = "inventory"

// Item is one line of the inventory, as Derive reads it. Only equipped items
// change the sheet's numbers.
type Item struct {
	// ID identifies the line; a weapon item's attack key is "inv:" + ID.
	ID string
	// Key is the content key: "equipment:..." for the SRD's equipment, "item:..."
	// for a magic item, "" for a free-text line.
	Key string
	// Base is the equipment a generic magic item is ("Weapon, +1" is a longsword)
	// and Option the damage type chosen for an item that offers one.
	Base, Option string
	// Name is a free-text line's name.
	Name     string
	Quantity int
	// Equipped says it is worn or wielded; Attuned says the character is attuned
	// to it.
	Equipped, Attuned bool
	// Unidentified says the master has not revealed what it is; Look is how it
	// shows meanwhile.
	Unidentified bool
	Look         string
}

// Alignments the sheet knows, for the attunement restrictions "by a creature of
// good alignment".
const (
	AlignmentGood = "good"
	AlignmentEvil = "evil"
)

// ItemResistance is a resistance an item gives and the item it comes from.
type ItemResistance struct {
	DamageType, Source string
}

// itemRuntime is an equipped item resolved against the content.
type itemRuntime struct {
	item Item
	def  *ItemDef
	// eq is the weapon, armor or shield the item is, and eqKey its key.
	eq    *srd51.Equipment
	eqKey string
	// name is the Portuguese name the sheet shows for it.
	name string
	// magic says the item's magic works: it is equipped and, when it requires
	// attunement, the character is attuned and meets its restriction.
	magic bool
	// superseded says another item took its slot: it counts for nothing.
	superseded bool
}

// itemMagicBonus is the +N the item adds as armor, weapon or ammunition, when its
// magic works.
func (r *itemRuntime) bonus(of func(*ItemDef) int) int {
	if r == nil || !r.magic || r.def == nil {
		return 0
	}
	return of(r.def)
}

// Restriction is an attunement prerequisite already read: an item with
// "(requires attunement by a cleric, druid, or paladin)" has the terms "cleric",
// "druid" and "paladin", any of which meets it.
type Restriction struct {
	// Classes are class names ("paladin"), Races race names ("dwarf"), and
	// Spellcaster, Good and Evil the other prerequisites. Unchecked is a term the
	// sheet cannot test ("outdoors at night"): the master decides it.
	Classes, Races []string
	Spellcaster    bool
	Good, Evil     bool
	Unchecked      bool
	empty          bool
}

// ParseRestriction reads the SRD's attunement restriction ("by a spellcaster",
// "by a cleric, druid, or paladin", "by a creature of good alignment"). An empty
// restriction is met by everyone.
func ParseRestriction(by string) Restriction {
	r := Restriction{empty: strings.TrimSpace(by) == ""}
	if r.empty {
		return r
	}
	by = strings.ToLower(strings.TrimSpace(by))
	for _, p := range []string{"by an ", "by a ", "by "} {
		if rest, ok := strings.CutPrefix(by, p); ok {
			by = rest
			break
		}
	}
	by = strings.NewReplacer(", or ", ",", " or ", ",", ", ", ",").Replace(by)
	for _, term := range strings.Split(by, ",") {
		term = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(term, "an "), "a "))
		switch {
		case term == "spellcaster":
			r.Spellcaster = true
		case term == "creature of good alignment":
			r.Good = true
		case term == "creature of evil alignment":
			r.Evil = true
		case slices.Contains(restrictionClasses, term):
			r.Classes = append(r.Classes, term)
		case slices.Contains(restrictionRaces, term):
			r.Races = append(r.Races, term)
		default:
			r.Unchecked = true
		}
	}
	return r
}

// The class and race names an attunement restriction can name in SRD 5.1.
var (
	restrictionClasses = []string{"barbarian", "bard", "cleric", "druid", "fighter", "monk", "paladin", "ranger", "rogue", "sorcerer", "warlock", "wizard"}
	restrictionRaces   = []string{"dwarf", "elf", "halfling", "human", "dragonborn", "gnome", "half-elf", "half-orc", "tiefling"}
)

// Character is what a restriction is tested against.
type Character struct {
	// Classes are the class names without the prefix ("paladin"), Race the base
	// race ("dwarf"), Spellcaster whether the character casts spells with its
	// own class features, and Alignment AlignmentGood, AlignmentEvil or "".
	Classes     []string
	Race        string
	Spellcaster bool
	Alignment   string
}

// Met says whether the character meets the restriction (SRD 5.1 "Attunement").
func (r Restriction) Met(c Character) bool {
	if r.empty {
		return true
	}
	if r.Unchecked {
		return true
	}
	switch {
	case r.Spellcaster && c.Spellcaster, r.Good && c.Alignment == AlignmentGood, r.Evil && c.Alignment == AlignmentEvil:
		return true
	}
	for _, cl := range r.Classes {
		if slices.Contains(c.Classes, cl) {
			return true
		}
	}
	return slices.Contains(r.Races, c.Race)
}

// characterOf reads the restriction's view of the build being derived.
func (x *deriver) characterOf() Character {
	c := Character{Race: strings.TrimPrefix(x.b.Race, "race:"), Alignment: x.b.Alignment}
	for _, oc := range x.classes {
		c.Classes = append(c.Classes, strings.TrimPrefix(oc.key, "class:"))
		if cast, ok := x.c.castingFor(oc.key, oc.subclass); ok && oc.level >= cast.level {
			c.Spellcaster = true
		}
	}
	return c
}

// resolveItems finds the equipped items, decides whether their magic works, and
// makes the worn armor and the shield of the sheet the ones the items say. It runs
// before the armor is resolved.
func (x *deriver) resolveItems() {
	c := x.c
	who := x.characterOf()
	for i, it := range x.b.Items {
		if !it.Equipped {
			continue
		}
		field := fmt.Sprintf("full.inventory[%d]", i)
		r := &itemRuntime{item: it}
		switch {
		case strings.HasPrefix(it.Key, "equipment:"):
			eq, ok := c.equipment[it.Key]
			if !ok {
				x.issue(IssueUnknownKey, field, "O item não existe no conteúdo %s.", c.version)
				continue
			}
			r.eq, r.eqKey = eq, it.Key
		case strings.HasPrefix(it.Key, "item:"):
			m, ok := c.magicItems[it.Key]
			if !ok {
				x.issue(IssueUnknownKey, field, "O item não existe no conteúdo %s.", c.version)
				continue
			}
			r.def = c.items.defs[it.Key]
			if !x.itemBase(r, field) {
				continue
			}
			r.magic = !m.Attunement || (it.Attuned && ParseRestriction(m.AttunementBy).Met(who))
		default:
			continue // a free-text line is only text
		}
		r.name = c.ItemName(it)
		x.items = append(x.items, r)
	}
	// An item takes a slot; past its capacity the later ones count for nothing.
	taken := map[string]int{}
	for _, r := range x.items {
		slot := r.slot()
		if slot == "" {
			continue
		}
		if capacity := SlotCapacity(slot); capacity > 0 && taken[slot] >= capacity {
			r.superseded = true
			x.issue(IssueInventory, "", "Itens demais no mesmo lugar do corpo (%s): só os primeiros valem.", r.name)
			continue
		}
		taken[slot]++
		switch slot {
		case SlotShield:
			x.shieldItem = r
		case SlotBody:
			x.armorItem = r
		}
	}
	x.items = slices.DeleteFunc(x.items, func(r *itemRuntime) bool { return r.superseded })
	if x.armorItem != nil {
		x.b.Armor = x.armorItem.eqKey
	}
	if x.shieldItem != nil {
		x.b.Shield = true
	}
}

// itemBase settles the equipment a magic item is, and reports a base it cannot
// have.
func (x *deriver) itemBase(r *itemRuntime, field string) bool {
	d := r.def
	if d == nil {
		return true
	}
	switch {
	case d.BaseFixed != "":
		r.eqKey = d.BaseFixed
	case len(d.BaseChoices) > 0:
		if !slices.Contains(d.BaseChoices, r.item.Base) {
			x.issue(IssueInventory, field, "Escolha de que arma ou armadura é o item %s.", x.c.namePT(r.item.Key))
			return false
		}
		r.eqKey = r.item.Base
	default:
		return true
	}
	r.eq = x.c.equipment[r.eqKey]
	return r.eq != nil
}

// adjustItemArmor applies what the worn armor item changes in the armor itself.
func (x *deriver) adjustItemArmor() {
	r := x.armorItem
	if r == nil || x.armor == nil || !r.magic || r.def == nil || !r.def.LightArmor {
		return
	}
	light := *x.armor
	light.StealthDisadvantage, light.StrMinimum = false, 0
	x.armor = &light
}

// armorItemProficient says an item makes the character proficient with its armor.
func (x *deriver) armorItemProficient() bool {
	r := x.armorItem
	return r != nil && r.magic && r.def != nil && r.def.ArmorProficiency
}

// armorItemBonus and shieldItemBonus are the +N magic armor and shield add to
// the AC.
func (x *deriver) armorItemBonus() int {
	return x.armorItem.bonus(func(d *ItemDef) int { return d.ArmorBonus })
}

func (x *deriver) shieldItemBonus() int {
	return x.shieldItem.bonus(func(d *ItemDef) int { return d.ArmorBonus })
}

// itemWeapons are the equipped weapons of the inventory, as lines of the attacks
// list after the sheet's own weapons.
func (x *deriver) itemWeapons() []weaponLine {
	var out []weaponLine
	for _, r := range x.items {
		if r.eq == nil || r.eq.Weapon == nil {
			continue
		}
		l := weaponLine{key: "inv:" + r.item.ID, eqKey: r.eqKey, name: r.name, field: "full.inventory", magic: r.magic}
		if r.magic && r.def != nil {
			l.bonus, l.damageType, l.finesse = r.def.WeaponBonus, r.def.WeaponDamageType, r.def.Finesse
		}
		out = append(out, l)
	}
	return out
}

// weaponLine is one line of the attacks list: a weapon of the sheet or an
// equipped weapon item.
type weaponLine struct {
	// key is the Attack.Key, eqKey the weapon's equipment key and name the
	// Portuguese name of an item ("" for the sheet's weapon, named by its key).
	key, eqKey, name, field string
	// bonus is the +N to attack and damage, damageType replaces the weapon's damage
	// type and finesse gives it the property.
	bonus      int
	damageType string
	finesse    bool
	magic      bool
}

// ammunitionFor picks the stack of ammunition a ranged attack with the weapon
// spends. A character with no counted ammunition of the kind fires without limit
// (the sheets from before the inventory); one whose stacks are all empty cannot
// fire. An equipped stack comes first, so putting the magic arrows in the quiver
// makes them the ones fired; then the first stack with pieces left.
func (x *deriver) ammunitionFor(ammo string) (stack *Item, bonus int, out bool) {
	var found []*Item
	for i := range x.b.Items {
		if it := &x.b.Items[i]; x.ammunitionKind(it) == ammo {
			found = append(found, it)
		}
	}
	if len(found) == 0 {
		return nil, 0, false
	}
	pick := func(equipped bool) *Item {
		for _, it := range found {
			if it.Quantity > 0 && (!equipped || it.Equipped) {
				return it
			}
		}
		return nil
	}
	stack = pick(true)
	if stack == nil {
		stack = pick(false)
	}
	if stack == nil {
		return nil, 0, true
	}
	if d := x.c.items.defs[stack.Key]; d != nil {
		bonus = d.AmmoBonus
	}
	return stack, bonus, false
}

// ammunitionKind is the kind of ammunition an item is: the SRD's arrow itself, or
// the base of a piece of magic ammunition.
func (x *deriver) ammunitionKind(it *Item) string {
	switch {
	case strings.HasPrefix(it.Key, "equipment:"):
		if eq := x.c.equipment[it.Key]; eq != nil && eq.Gear != nil && eq.Gear.Ammunition {
			return it.Key
		}
	case strings.HasPrefix(it.Key, "item:"):
		if d := x.c.items.defs[it.Key]; d != nil && d.AmmoBonus > 0 {
			return it.Base
		}
	}
	return ""
}

// addItemEffects joins the effects of the items whose magic works to the active
// ones, and reads the rest of what they give: resistances, spell bonuses and the
// reminders.
func (x *deriver) addItemEffects() {
	for _, r := range x.items {
		if r.def == nil || !r.magic {
			continue
		}
		d := r.def
		for _, e := range d.Effects {
			x.active = append(x.active, activeEffect{owner: r.item.Key, effect: e})
		}
		for _, t := range d.Resistances {
			x.addResistance(t, r.item.Key)
		}
		if d.OptionResistance && r.item.Option != "" && slices.Contains(d.OptionDamageTypes, r.item.Option) {
			x.addResistance(r.item.Option, r.item.Key)
		}
		x.itemSpellAttack += d.SpellAttack
		x.itemSpellDC += d.SpellDC
		if d.NotePT != "" {
			x.d.Hints = append(x.d.Hints, Hint{Source: r.item.Key, Target: "item", Targets: []string{"item"}, Mode: "note", TextPT: r.name + ": " + d.NotePT})
		}
	}
}

func (x *deriver) addResistance(damageType, source string) {
	if !slices.ContainsFunc(x.d.ItemResistances, func(r ItemResistance) bool { return r.DamageType == damageType }) {
		x.d.ItemResistances = append(x.d.ItemResistances, ItemResistance{DamageType: damageType, Source: source})
	}
}

// ItemName is the Portuguese name the sheet shows for an inventory line: the
// player's own words for a free-text line, the look of an item the master has not
// identified, and otherwise the SRD item's name, with the base and the bonus of a
// generic one ("Espada longa +1").
func (c *Content) ItemName(it Item) string { return c.c.ItemName(it) }

func (c *content) ItemName(it Item) string {
	switch {
	case it.Key == "":
		return it.Name
	case strings.HasPrefix(it.Key, "item:") && it.Unidentified:
		if strings.TrimSpace(it.Look) != "" {
			return it.Look
		}
		return unidentifiedLook
	case strings.HasPrefix(it.Key, "equipment:"):
		return c.namePT(it.Key)
	}
	name := c.namePT(it.Key)
	d := c.items.defs[it.Key]
	if d != nil && len(d.BaseChoices) > 0 && it.Base != "" {
		n := max(d.WeaponBonus, d.ArmorBonus, d.AmmoBonus)
		if n > 0 {
			name = fmt.Sprintf("%s +%d", c.namePT(it.Base), n)
		} else {
			name = c.namePT(it.Base)
		}
	}
	if d != nil && len(d.OptionDamageTypes) > 0 && it.Option != "" {
		name += " (" + strings.ToLower(c.namePT(it.Option)) + ")"
	}
	return name
}

// unidentifiedLook is what an item the master did not describe shows as.
const unidentifiedLook = "Um item mágico que você não reconhece"

// displayName is the Portuguese name of the attack line.
func (l weaponLine) displayName(c *content) string {
	if l.name != "" {
		return l.name
	}
	return c.namePT(l.eqKey)
}

// ammoAttack is what a ranged attack with a weapon of the ammunition kind spends.
type ammoAttack struct {
	kind, item string
	bonus      int
	out        bool
}

// ammunitionAttack says what the weapon fires: nothing for a melee weapon or one
// without the ammunition property.
func (x *deriver) ammunitionAttack(eqKey, kind string) ammoAttack {
	ammo := x.c.items.ammunition[eqKey]
	if ammo == "" || kind != "ranged" {
		return ammoAttack{}
	}
	stack, bonus, out := x.ammunitionFor(ammo)
	a := ammoAttack{kind: ammo, bonus: bonus, out: out}
	if stack != nil {
		a.item = stack.ID
	}
	return a
}

// slot is where the item sits: the magic item's slot, or the slot its armor
// goes in; "" for one that takes none.
func (r *itemRuntime) slot() string {
	switch {
	case r.eq != nil && r.eq.Armor != nil && r.eq.Armor.Category == "shield":
		return SlotShield
	case r.eq != nil && r.eq.Armor != nil:
		return SlotBody
	case r.def != nil:
		return r.def.Slot
	}
	return ""
}
