package rules

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The inventory's rules content (effects/items.json): what the engine applies
// of each magic item, in a closed list of typed effects, and what it leaves as
// a reminder. The SRD text of every item stays in data/magic-items.json; this
// file only says which numbers of that text the sheet computes.

// MaxInventoryItems bounds the lines of an inventory (a stack is one line).
const MaxInventoryItems = 200

// MaxItemQuantity bounds one line's quantity.
const MaxItemQuantity = 9999

// MaxItemNameLength bounds a free-text item's name and an unidentified item's
// look, in characters.
const MaxItemNameLength = 100

// ItemBonusMax is the largest +N of a weapon, armor or ammunition (SRD 5.1
// "Weapon, +1, +2, or +3").
const itemBonusMax = 3

// The slots an equipped item takes. An item without a slot (a bag, a potion)
// is carried, and Equipped on it is only a note.
const (
	SlotBody    = "body"
	SlotShield  = "shield"
	SlotHead    = "head"
	SlotCloak   = "cloak"
	SlotBoots   = "boots"
	SlotGloves  = "gloves"
	SlotBracers = "bracers"
	SlotNeck    = "neck"
	SlotRing    = "ring"
	SlotBelt    = "belt"
	SlotHand    = "hand"
)

var itemSlots = []string{SlotBody, SlotShield, SlotHead, SlotCloak, SlotBoots, SlotGloves, SlotBracers, SlotNeck, SlotRing, SlotBelt, SlotHand}

// SlotCapacity is how many items fit in a slot at once. SRD 5.1 "Multiple Items
// of the Same Kind" allows one pair of footwear, one pair of gloves, one pair of
// bracers, one suit of armor, one item of headwear and one cloak, and says to use
// common sense for the rest; the sheet allows one amulet and belt, two rings
// (SRD 5.1 is silent on rings) and, held in the hands, any number of weapons and
// wands (a table limit the app does not impose).
func SlotCapacity(slot string) int {
	switch slot {
	case SlotRing:
		return 2
	case SlotHand, "":
		return 0 // no limit
	}
	return 1
}

// itemAmmoGroup, itemWeaponGroup and itemArmorGroup are the groups the loader
// builds from the SRD equipment; a file's own groups sit beside them.
const (
	groupWeapons    = "@weapons"
	groupArmor      = "@armor"
	groupAmmunition = "@ammunition"
)

// ItemCharges is how a magic item with charges works (SRD 5.1 "Charges").
type ItemCharges struct {
	// Max is the charges a full item has.
	Max int
	// RegainDice is what the item regains each dawn, as dice ("1d6+1"), or "" when
	// it regains none (the item's text says what else happens).
	RegainDice string
	// Spells are what the item casts by spending charges.
	Spells []ChargeSpell
}

// ChargeSpell is a spell cast from an item for charges (SRD 5.1 "Spells" of
// "Magic Items": at the lowest possible spell level, no spell slot, no components).
type ChargeSpell struct {
	// Spell is the spell key; Level its level when cast for Cost charges.
	Spell string
	Level int
	// Cost is the charges of the base level. CostPerLevel more charges raise the
	// level by one, up to MaxLevel (0: the level cannot be raised).
	Cost, CostPerLevel, MaxLevel int
	// DC is the item's own saving throw DC (the wand of fireballs: 15), or 0 when
	// the item says "your spell save DC".
	DC int
}

// CostAt is the charges the spell costs at a level, and false when the item
// cannot cast it at that level.
func (s ChargeSpell) CostAt(level int) (int, bool) {
	switch {
	case level < s.Level:
		return 0, false
	case level == s.Level:
		return s.Cost, true
	case s.CostPerLevel == 0 || level > s.MaxLevel:
		return 0, false
	}
	return s.Cost + (level-s.Level)*s.CostPerLevel, true
}

// ItemUse is what using a consumable item does.
type ItemUse struct {
	// HealDice is the hit points a potion restores ("2d4+2"), or "".
	HealDice string
	// TempHP is the temporary hit points a potion gives.
	TempHP int
	// Scroll marks a spell scroll; ScrollLevel is the level of its spell and
	// ScrollDC and ScrollAttack the numbers that level gives (SRD 5.1 "Spell
	// Scroll" table).
	Scroll                 bool
	ScrollLevel            int
	ScrollDC, ScrollAttack int
}

// ItemDef is what the engine knows of a magic item.
type ItemDef struct {
	Key string
	// Slot is where it is worn or held, or "".
	Slot string
	// BaseFixed is the one equipment the item is (the Dwarven Thrower is a
	// warhammer); BaseChoices are the equipment a generic item can be
	// ("Weapon, +1" is any weapon). Both empty: the item has no base.
	BaseFixed   string
	BaseChoices []string
	// OptionDamageTypes are the damage types the master picks one of when giving
	// the item (Armor of Resistance).
	OptionDamageTypes []string
	// WeaponBonus is the +N to attack and damage rolls of the item as a weapon,
	// ArmorBonus the +N to the AC of the item as armor or shield, and AmmoBonus the
	// +N of ammunition. WeaponDamageType and Finesse change the base weapon.
	WeaponBonus, ArmorBonus, AmmoBonus int
	WeaponDamageType                   string
	Finesse                            bool
	// ArmorProficiency says the wearer is proficient with the armor (Elven
	// Chain); LightArmor drops the armor's stealth disadvantage and Strength
	// requirement (Mithral Armor).
	ArmorProficiency, LightArmor bool
	// Resistances are the damage types the item gives resistance to;
	// OptionResistance says the damage type chosen when the item was given is one
	// more (Armor of Resistance).
	Resistances      []string
	OptionResistance bool
	// SpellAttack and SpellDC are added to the wearer's spell attack bonus and
	// spell save DC.
	SpellAttack, SpellDC int
	// Effects are the engine effects (modifier, sense) the item gives while it is
	// in use; compiled at load.
	Effects []*Effect
	// Charges and Use are the item's charges and the use of a consumable.
	Charges *ItemCharges
	Use     *ItemUse
	// NotePT is what the sheet leaves to the table, in Portuguese: the item's
	// properties the engine does not apply.
	NotePT string
}

// ItemCoverage says how much of an item the engine applies.
type ItemCoverage string

// The three coverages.
const (
	// CoverageApplied: everything the item does that a number can say is applied.
	CoverageApplied ItemCoverage = "applied"
	// CoveragePartial: some properties are applied, and NotePT lists the rest.
	CoveragePartial ItemCoverage = "partial"
	// CoverageReminder: nothing is applied; the SRD text is the reminder.
	CoverageReminder ItemCoverage = "reminder"
)

// itemsFile is effects/items.json.
type itemsFile struct {
	Source     string                `json:"source"`
	Groups     map[string][]string   `json:"groups"`
	Ammunition map[string]string     `json:"ammunition"`
	Items      map[string]*itemEntry `json:"items"`
}

type itemEntry struct {
	Slot    string           `json:"slot,omitempty"`
	Base    *itemBaseEntry   `json:"base,omitempty"`
	Option  *itemOptionEntry `json:"option,omitempty"`
	Effects []itemEffect     `json:"effects,omitempty"`
	Charges *itemChargesJSON `json:"charges,omitempty"`
	Use     *itemUseJSON     `json:"use,omitempty"`
	NotePT  string           `json:"note_pt,omitempty"`
}

type itemBaseEntry struct {
	Fixed string `json:"fixed,omitempty"`
	Group string `json:"group,omitempty"`
}

type itemOptionEntry struct {
	DamageTypes []string `json:"damage_types"`
}

// itemEffect is one typed effect of the closed list: weapon_bonus, armor_bonus,
// ammunition_bonus, weapon_damage_type, weapon_finesse, armor_proficiency,
// armor_light, ac, ac_base, save, ability, hp_per_level, speed, sense, resistance,
// spell_attack and spell_dc.
type itemEffect struct {
	Type       string `json:"type"`
	Value      int    `json:"value,omitempty"`
	Ability    string `json:"ability,omitempty"`
	DamageType string `json:"damage_type,omitempty"`
	Sense      string `json:"sense,omitempty"`
	Mode       string `json:"mode,omitempty"`
	When       string `json:"when,omitempty"`
	Cap        int    `json:"cap,omitempty"`
	Dex        bool   `json:"dex,omitempty"`
}

type itemChargesJSON struct {
	Max        int               `json:"max"`
	RegainDice string            `json:"regain_dice,omitempty"`
	Spells     []chargeSpellJSON `json:"spells,omitempty"`
}

type chargeSpellJSON struct {
	Spell        string `json:"spell"`
	Level        int    `json:"level"`
	Cost         int    `json:"cost"`
	CostPerLevel int    `json:"cost_per_level,omitempty"`
	MaxLevel     int    `json:"max_level,omitempty"`
	DC           int    `json:"dc,omitempty"`
}

type itemUseJSON struct {
	Heal   string `json:"heal,omitempty"`
	TempHP int    `json:"temp_hp,omitempty"`
	Scroll *int   `json:"scroll,omitempty"`
}

// The item effect types, and the "when" values an ac effect takes.
var (
	itemEffectTypes = []string{
		"weapon_bonus", "armor_bonus", "ammunition_bonus", "weapon_damage_type", "weapon_finesse",
		"armor_proficiency", "armor_light", "ac", "ac_base", "save", "ability", "hp_per_level",
		"speed", "sense", "resistance", "option_resistance", "spell_attack", "spell_dc",
	}
	itemWhens = map[string]string{
		"":                   "",
		"no_armor_no_shield": `armor() == "none" && !shield()`,
		"no_armor":           `armor() == "none"`,
	}
	itemSenseModes = []string{"at_least", "extend"}
	itemSpeedModes = []string{"at_least", "add"}
)

// The Spell Scroll table of SRD 5.1: the saving throw DC and the attack bonus of
// a scroll, by the level of its spell (index 0 is a cantrip).
var scrollDC = [...]int{13, 13, 13, 15, 15, 17, 17, 18, 18, 19}
var scrollAttack = [...]int{5, 5, 5, 7, 7, 9, 9, 10, 10, 11}

// itemsContent is the loaded effects/items.json.
type itemsContent struct {
	defs       map[string]*ItemDef
	ammunition map[string]string
	groups     map[string][]string
}

// loadItems reads effects/items.json and checks it against the SRD content:
// every key is a magic item, every base an equipment of the right kind, every
// spell a spell, every damage type a damage type. It refuses unknown fields,
// unknown effect types and an effect with a field its type does not use.
func (c *content) loadItems(fsys fs.FS) error {
	const name = "effects/items.json"
	var f itemsFile
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	if strings.TrimSpace(f.Source) == "" {
		return fmt.Errorf("%s: no source", name)
	}
	out := &itemsContent{defs: map[string]*ItemDef{}, ammunition: map[string]string{}, groups: map[string][]string{}}
	if err := c.loadItemGroups(name, &f, out); err != nil {
		return err
	}
	for _, k := range sortedKeys(f.Items) {
		def, err := c.compileItem(name, k, f.Items[k], out)
		if err != nil {
			return err
		}
		out.defs[k] = def
	}
	c.items = out
	return nil
}

func (c *content) loadItemGroups(name string, f *itemsFile, out *itemsContent) error {
	var weapons, armor, ammo []string
	for _, k := range sortedKeys(c.equipment) {
		e := c.equipment[k]
		switch {
		case e.Weapon != nil:
			weapons = append(weapons, k)
		case e.Armor != nil && e.Armor.Category != "shield":
			armor = append(armor, k)
		case e.Gear != nil && e.Gear.Ammunition:
			ammo = append(ammo, k)
		}
	}
	out.groups[groupWeapons], out.groups[groupArmor], out.groups[groupAmmunition] = weapons, armor, ammo
	for _, g := range sortedKeys(f.Groups) {
		if strings.HasPrefix(g, "@") || g == "" {
			return fmt.Errorf("%s: group %q is reserved or empty", name, g)
		}
		list := f.Groups[g]
		if len(list) == 0 {
			return fmt.Errorf("%s: group %q is empty", name, g)
		}
		for _, k := range list {
			if !slices.Contains(weapons, k) && !slices.Contains(armor, k) {
				return fmt.Errorf("%s: group %q: %q is not a weapon or a body armor", name, g, k)
			}
		}
		out.groups[g] = slices.Clone(list)
	}
	for _, w := range sortedKeys(f.Ammunition) {
		eq, ok := c.equipment[w]
		if !ok || eq.Weapon == nil || !slices.Contains(eq.Weapon.Properties, "weapon-property:ammunition") {
			return fmt.Errorf("%s: ammunition: %q is not a weapon with the ammunition property", name, w)
		}
		if !slices.Contains(ammo, f.Ammunition[w]) {
			return fmt.Errorf("%s: ammunition: %q is not ammunition", name, f.Ammunition[w])
		}
		out.ammunition[w] = f.Ammunition[w]
	}
	for _, w := range weapons {
		eq := c.equipment[w]
		if slices.Contains(eq.Weapon.Properties, "weapon-property:ammunition") && out.ammunition[w] == "" {
			return fmt.Errorf("%s: ammunition: the %s has no ammunition", name, w)
		}
	}
	return nil
}

func (c *content) compileItem(file, key string, e *itemEntry, out *itemsContent) (*ItemDef, error) {
	fail := func(format string, a ...any) error {
		return fmt.Errorf("%s: %s: %s", file, key, fmt.Sprintf(format, a...))
	}
	m, ok := c.magicItems[key]
	if !ok {
		return nil, fail("is not a magic item")
	}
	if len(m.Variants) > 0 && !m.Standalone {
		return nil, fail("is a family: write its variants")
	}
	d := &ItemDef{Key: key, Slot: e.Slot, NotePT: strings.TrimSpace(e.NotePT)}
	if e.Slot != "" && !slices.Contains(itemSlots, e.Slot) {
		return nil, fail("%q is not a slot", e.Slot)
	}
	if len([]rune(d.NotePT)) > maxItemNoteLength {
		return nil, fail("note_pt is longer than %d characters", maxItemNoteLength)
	}
	if err := c.compileItemBase(key, e, d, out); err != nil {
		return nil, fail("%v", err)
	}
	if e.Option != nil {
		if len(e.Option.DamageTypes) == 0 {
			return nil, fail("option has no damage types")
		}
		for _, t := range e.Option.DamageTypes {
			if !c.isDamageType(t) {
				return nil, fail("option: %q is not a damage type", t)
			}
		}
		d.OptionDamageTypes = slices.Clone(e.Option.DamageTypes)
	}
	for i, fx := range e.Effects {
		if err := c.compileItemEffect(key, fx, d); err != nil {
			return nil, fail("effects[%d] (%s): %v", i, fx.Type, err)
		}
	}
	if err := c.compileItemCharges(e, d); err != nil {
		return nil, fail("%v", err)
	}
	if err := c.compileItemUse(e, d); err != nil {
		return nil, fail("%v", err)
	}
	// A bonus needs the thing it is a bonus of.
	baseOf := func(pred func(*srd51.Equipment) bool) bool {
		for _, b := range append([]string{d.BaseFixed}, d.BaseChoices...) {
			if eq, ok := c.equipment[b]; ok && pred(eq) {
				return true
			}
		}
		return false
	}
	isWeapon := func(eq *srd51.Equipment) bool { return eq.Weapon != nil }
	isArmor := func(eq *srd51.Equipment) bool { return eq.Armor != nil }
	isAmmo := func(eq *srd51.Equipment) bool { return eq.Gear != nil && eq.Gear.Ammunition }
	switch {
	case (d.WeaponBonus != 0 || d.WeaponDamageType != "" || d.Finesse) && !baseOf(isWeapon):
		return nil, fail("a weapon effect needs a weapon base")
	case (d.ArmorBonus != 0 || d.ArmorProficiency || d.LightArmor) && !baseOf(isArmor):
		return nil, fail("an armor effect needs an armor base")
	case d.AmmoBonus != 0 && !baseOf(isAmmo):
		return nil, fail("an ammunition bonus needs an ammunition base")
	}
	switch {
	case baseOf(isArmor) && d.Slot != SlotBody && d.Slot != SlotShield:
		return nil, fail("an armor goes in the body or shield slot")
	case baseOf(isWeapon) && d.Slot != SlotHand:
		return nil, fail("a weapon goes in the hand slot")
	}
	return d, nil
}

// maxItemNoteLength bounds an item's reminder, in characters.
const maxItemNoteLength = 300

func (c *content) isDamageType(k string) bool {
	_, ok := c.named[k]
	return ok && strings.HasPrefix(k, "damage-type:")
}

func (c *content) compileItemBase(key string, e *itemEntry, d *ItemDef, out *itemsContent) error {
	if e.Base == nil {
		return nil
	}
	b := e.Base
	if (b.Fixed == "") == (b.Group == "") {
		return fmt.Errorf("base is a fixed equipment or a group, one of them")
	}
	if b.Fixed != "" {
		eq, ok := c.equipment[b.Fixed]
		if !ok || (eq.Weapon == nil && eq.Armor == nil) {
			return fmt.Errorf("base %q is not a weapon or an armor", b.Fixed)
		}
		d.BaseFixed = b.Fixed
		return nil
	}
	g, ok := out.groups[b.Group]
	if !ok {
		return fmt.Errorf("base group %q does not exist", b.Group)
	}
	d.BaseChoices = slices.Clone(g)
	return nil
}

func (c *content) compileItemEffect(key string, fx itemEffect, d *ItemDef) error {
	if !slices.Contains(itemEffectTypes, fx.Type) {
		return fmt.Errorf("unknown effect type")
	}
	// Which fields a type uses; the rest must be empty.
	used := map[string]bool{}
	use := func(names ...string) {
		for _, n := range names {
			used[n] = true
		}
	}
	var eff *Effect
	switch fx.Type {
	case "weapon_bonus", "armor_bonus", "ammunition_bonus":
		use("value")
		if fx.Value < 1 || fx.Value > itemBonusMax {
			return fmt.Errorf("value is 1 to %d", itemBonusMax)
		}
		switch fx.Type {
		case "weapon_bonus":
			d.WeaponBonus = fx.Value
		case "armor_bonus":
			d.ArmorBonus = fx.Value
		default:
			d.AmmoBonus = fx.Value
		}
	case "weapon_damage_type":
		use("damage_type")
		if !c.isDamageType(fx.DamageType) {
			return fmt.Errorf("%q is not a damage type", fx.DamageType)
		}
		d.WeaponDamageType = fx.DamageType
	case "weapon_finesse":
		d.Finesse = true
	case "armor_proficiency":
		d.ArmorProficiency = true
	case "armor_light":
		d.LightArmor = true
	case "ac":
		use("value", "when")
		when, ok := itemWhens[fx.When]
		if !ok || fx.Value < 1 || fx.Value > itemBonusMax {
			return fmt.Errorf("an ac bonus is 1 to %d, with a known when", itemBonusMax)
		}
		eff = &Effect{Type: "modifier", Target: "ac", Mode: "add", Value: fmt.Sprint(fx.Value), When: when}
	case "ac_base":
		use("value", "dex", "when")
		when, ok := itemWhens[fx.When]
		if !ok || fx.When == "" || fx.Value < 11 || fx.Value > 20 {
			return fmt.Errorf("an ac_base is 11 to 20, with a when")
		}
		formula := fmt.Sprint(fx.Value)
		if fx.Dex {
			formula += ` + mod("dex")`
		}
		eff = &Effect{Type: "modifier", Target: "ac.base", Mode: "max", Value: formula, When: when}
	case "save":
		use("value")
		if fx.Value < 1 || fx.Value > itemBonusMax {
			return fmt.Errorf("a save bonus is 1 to %d", itemBonusMax)
		}
		eff = &Effect{Type: "modifier", Target: "save.all", Mode: "add", Value: fmt.Sprint(fx.Value)}
	case "ability":
		use("ability", "value", "mode", "cap")
		if _, ok := abilityIndex[Ability(fx.Ability)]; !ok {
			return fmt.Errorf("%q is not an ability", fx.Ability)
		}
		switch fx.Mode {
		case "set": // the score becomes Value, unless it is higher already
			if fx.Value < 1 || fx.Value > MaxScore || fx.Cap != 0 {
				return fmt.Errorf("set takes a value of 1 to %d and no cap", MaxScore)
			}
			eff = &Effect{Type: "modifier", Target: "score." + fx.Ability, Mode: "add", Value: fmt.Sprint(MaxScore), Cap: fx.Value}
		case "add": // the score goes up by Value, to at most Cap
			if fx.Value < 1 || fx.Value > itemBonusMax+1 || fx.Cap < 1 || fx.Cap > MaxScore {
				return fmt.Errorf("add takes a value of 1 to %d and a cap", itemBonusMax+1)
			}
			eff = &Effect{Type: "modifier", Target: "score." + fx.Ability, Mode: "add", Value: fmt.Sprint(fx.Value), Cap: fx.Cap}
		default:
			return fmt.Errorf("an ability effect is set or add")
		}
	case "hp_per_level":
		eff = &Effect{Type: "modifier", Target: "hp.max", Mode: "add", Value: "level()"}
	case "speed":
		use("value", "mode")
		if !slices.Contains(itemSpeedModes, fx.Mode) || fx.Value < 5 || fx.Value%5 != 0 {
			return fmt.Errorf("a speed is a multiple of 5 ft, at_least or add")
		}
		mode := "max"
		if fx.Mode == "add" {
			mode = "add"
		}
		eff = &Effect{Type: "modifier", Target: "speed.walk", Mode: mode, Value: fmt.Sprint(fx.Value)}
	case "sense":
		use("sense", "value", "mode")
		if !slices.Contains(senses, fx.Sense) || !slices.Contains(itemSenseModes, fx.Mode) || fx.Value < 5 || fx.Value%5 != 0 {
			return fmt.Errorf("a sense is a known sense, a range in multiples of 5 ft, at_least or extend")
		}
		eff = &Effect{Type: "sense", Sense: fx.Sense, RangeFt: fx.Value, Extend: fx.Mode == "extend"}
	case "resistance":
		use("damage_type")
		if !c.isDamageType(fx.DamageType) {
			return fmt.Errorf("%q is not a damage type", fx.DamageType)
		}
		if !slices.Contains(d.Resistances, fx.DamageType) {
			d.Resistances = append(d.Resistances, fx.DamageType)
		}
	case "option_resistance":
		if len(d.OptionDamageTypes) == 0 {
			return fmt.Errorf("it needs an option of damage types")
		}
		d.OptionResistance = true
	case "spell_attack", "spell_dc":
		use("value")
		if fx.Value < 1 || fx.Value > itemBonusMax {
			return fmt.Errorf("a spell bonus is 1 to %d", itemBonusMax)
		}
		if fx.Type == "spell_attack" {
			d.SpellAttack = fx.Value
		} else {
			d.SpellDC = fx.Value
		}
	}
	for field, set := range map[string]bool{
		"value": fx.Value != 0, "ability": fx.Ability != "", "damage_type": fx.DamageType != "", "sense": fx.Sense != "",
		"mode": fx.Mode != "", "when": fx.When != "", "cap": fx.Cap != 0, "dex": fx.Dex,
	} {
		if set && !used[field] {
			return fmt.Errorf("the type does not use %s", field)
		}
	}
	if eff == nil {
		return nil
	}
	if err := c.compileEffect(key, eff); err != nil {
		return err
	}
	d.Effects = append(d.Effects, eff)
	return nil
}

func (c *content) compileItemCharges(e *itemEntry, d *ItemDef) error {
	ch := e.Charges
	if ch == nil {
		return nil
	}
	if ch.Max < 1 || ch.Max > maxItemCharges {
		return fmt.Errorf("charges: max is 1 to %d", maxItemCharges)
	}
	if ch.RegainDice != "" {
		if _, ok := ParseDice(ch.RegainDice); !ok {
			return fmt.Errorf("charges: regain_dice %q is not dice", ch.RegainDice)
		}
	}
	out := &ItemCharges{Max: ch.Max, RegainDice: ch.RegainDice}
	for i, s := range ch.Spells {
		sp, ok := c.spells[s.Spell]
		if !ok {
			return fmt.Errorf("charges.spells[%d]: %q is not a spell", i, s.Spell)
		}
		switch {
		case s.Level < sp.Level || s.Level > maxSpellLevel:
			return fmt.Errorf("charges.spells[%d]: level %d is below the spell's level or above 9", i, s.Level)
		case s.Cost < 1 || s.Cost > ch.Max:
			return fmt.Errorf("charges.spells[%d]: cost is 1 to the item's charges", i)
		case (s.CostPerLevel == 0) != (s.MaxLevel == 0) || s.MaxLevel > maxSpellLevel || (s.MaxLevel != 0 && s.MaxLevel <= s.Level),
			s.CostPerLevel < 0 || s.DC < 0 || s.DC > maxItemDC:
			return fmt.Errorf("charges.spells[%d]: cost_per_level and max_level go together, above the level, and the DC is 0 to %d", i, maxItemDC)
		}
		out.Spells = append(out.Spells, ChargeSpell{Spell: s.Spell, Level: s.Level, Cost: s.Cost, CostPerLevel: s.CostPerLevel, MaxLevel: s.MaxLevel, DC: s.DC})
	}
	d.Charges = out
	return nil
}

// Bounds of the item numbers.
const (
	maxItemCharges = 50
	maxSpellLevel  = 9
	maxItemDC      = 30
)

func (c *content) compileItemUse(e *itemEntry, d *ItemDef) error {
	u := e.Use
	if u == nil {
		return nil
	}
	out := &ItemUse{TempHP: u.TempHP}
	if u.Heal != "" {
		if _, ok := ParseDice(u.Heal); !ok {
			return fmt.Errorf("use: heal %q is not dice", u.Heal)
		}
		out.HealDice = u.Heal
	}
	if u.Scroll != nil {
		lvl := *u.Scroll
		if lvl < 0 || lvl > maxSpellLevel || u.Heal != "" || u.TempHP != 0 {
			return fmt.Errorf("use: a scroll is a spell level of 0 to 9, and nothing else")
		}
		out.Scroll, out.ScrollLevel, out.ScrollDC, out.ScrollAttack = true, lvl, scrollDC[lvl], scrollAttack[lvl]
	}
	if u.TempHP < 0 || u.TempHP > maxItemTempHP || (out.HealDice == "" && out.TempHP == 0 && !out.Scroll) {
		return fmt.Errorf("use: heal, temp_hp (up to %d) or scroll", maxItemTempHP)
	}
	d.Use = out
	return nil
}

// maxItemTempHP bounds the temporary hit points of a potion.
const maxItemTempHP = 100

// Coverage says how much of the item the engine applies (the item has no
// definition: a reminder).
func (d *ItemDef) Coverage() ItemCoverage {
	if d == nil || !d.appliesSomething() {
		return CoverageReminder
	}
	if d.NotePT != "" {
		return CoveragePartial
	}
	return CoverageApplied
}

// appliesSomething says the engine computes at least one property of the item
// beyond acting as its base weapon or armor.
func (d *ItemDef) appliesSomething() bool {
	return d.WeaponBonus != 0 || d.ArmorBonus != 0 || d.AmmoBonus != 0 || d.WeaponDamageType != "" || d.Finesse ||
		d.ArmorProficiency || d.LightArmor || len(d.Resistances) > 0 || d.OptionResistance || d.SpellAttack != 0 ||
		d.SpellDC != 0 || len(d.Effects) > 0 || d.Charges != nil || d.Use != nil
}

// ItemDef returns what the engine applies of a magic item, and false for an
// item that is only a reminder.
func (c *Content) ItemDef(key string) (*ItemDef, bool) {
	d, ok := c.c.items.defs[key]
	return d, ok
}

// ItemCoverage says how much of an SRD magic item the engine applies.
func (c *Content) ItemCoverage(key string) ItemCoverage {
	d, _ := c.ItemDef(key)
	return d.Coverage()
}

// AmmunitionFor is the ammunition a weapon with the ammunition property fires
// ("equipment:longbow" fires "equipment:arrow"), or "".
func (c *Content) AmmunitionFor(weaponKey string) string { return c.c.items.ammunition[weaponKey] }

// ItemGroup returns the equipment keys of a base group of items.json, or of the
// built-in groups "@weapons", "@armor" and "@ammunition".
func (c *Content) ItemGroup(name string) []string { return slices.Clone(c.c.items.groups[name]) }
