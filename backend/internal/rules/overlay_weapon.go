package rules

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The table's weapons: an entry "weapon:<slug>@mesa" is a weapon the SRD's equipment list does not have. It becomes an
// equipment row of the weapon kind, by its own key, so the sheet's weapon list, the attack lines, the proficiency by
// category and the rules that read a weapon's properties read it as they read an SRD weapon. It is mundane: a magic
// weapon is an item. Weight and price are the table's notes, kept with the entry and not counted by the engine.

// WeaponKeyPrefix starts the key of a table weapon.
const WeaponKeyPrefix = "weapon:"

// The limits of a table weapon.
const (
	// MaxWeaponProperties is the most properties one weapon has.
	MaxWeaponProperties = 10
	// MaxTableWeaponRangeFt bounds a normal and a long range, and MaxWeaponWeightOz and MaxWeaponPriceCP the notes.
	MaxTableWeaponRangeFt = 1000
	MaxWeaponWeightOz     = 160000
	MaxWeaponPriceCP      = 100_000_000
)

// weaponDice is the damage of a weapon: dice of the dice a weapon rolls, or a flat 1 to 99.
var weaponDice = regexp.MustCompile(`^(?:(?:[1-9]|1\d|20)d(?:4|6|8|10|12|20)|[1-9]\d?)$`)

// TableWeapon is a table weapon. Category is "simple" or "martial"; Ranged says whether it is a ranged weapon.
type TableWeapon struct {
	TableEntry
	DescPT          []string
	Category        string
	Ranged          bool
	Damage          string
	DamageType      string
	TwoHandedDamage string
	// Properties are "weapon-property:" keys of the content.
	Properties []string
	// NormalRangeFt and LongRangeFt are the range of a ranged weapon or of a thrown one; 0 otherwise.
	NormalRangeFt, LongRangeFt int
	// WeightOz and PriceCP are notes.
	WeightOz, PriceCP int
	// TwoHandedSince is the table revision at which the weapon last became two-handed, as the server keeps it (0 when it is
	// not, or was before it was kept): Content.TwoHandedSince.
	TwoHandedSince int
}

// addWeapon registers a table weapon as an equipment row of the clone. Every field that is wrong is reported at its own
// path.
func (b *overlayBuilder) addWeapon(w *TableWeapon, path string) error {
	key := w.Key
	var c entryErrors
	checkTextAt(&c, key, path, ".desc_pt", w.DescPT)
	b.checkWeaponShape(&c, w, path)
	b.checkWeaponRange(&c, w, path)
	if err := c.err(); err != nil {
		return err
	}
	b.register(w.TableEntry)
	b.ownEquipment()
	b.n.equipment[key] = &srd51.Equipment{Key: key, Name: w.NamePT, Kind: "weapon", Weapon: weaponRow(w)}
	b.n.weaponOf[key] = WeaponTableEntry{Key: key, NamePT: w.NamePT, TwoHandedSince: w.TwoHandedSince}
	return nil
}

// checkWeaponShape checks the category, the damage and the properties.
func (b *overlayBuilder) checkWeaponShape(c *entryErrors, w *TableWeapon, path string) {
	key := w.Key
	if w.Category != "simple" && w.Category != "martial" {
		c.at(key, path, ".category", ReasonValue, "the category is simple or martial")
	}
	if !weaponDice.MatchString(w.Damage) {
		c.at(key, path, ".damage", ReasonValue, "the damage is dice such as 1d8 (1 to 20 dice of 4, 6, 8, 10, 12 or 20 sides) or a flat 1 to 99")
	}
	if !b.base.isDamageTypeKey(w.DamageType) {
		c.at(key, path, ".damage_type", ReasonReference, "%q is not a damage type", w.DamageType)
	}
	has := func(p string) bool { return slices.Contains(w.Properties, weaponPropertyPrefix+p) }
	switch {
	case w.TwoHandedDamage != "" && !weaponDice.MatchString(w.TwoHandedDamage):
		c.at(key, path, ".two_handed_damage", ReasonValue, "the two-handed damage is dice such as 1d10")
	case w.TwoHandedDamage != "" && !has("versatile"):
		c.at(key, path, ".two_handed_damage", ReasonValue, "only a versatile weapon has a two-handed damage")
	case w.TwoHandedDamage == "" && has("versatile"):
		c.at(key, path, ".two_handed_damage", ReasonValue, "a versatile weapon needs its two-handed damage")
	}
	b.checkWeaponProperties(c, w, path)
	checkWeaponCombinations(c, w, path)
	if w.WeightOz < 0 || w.WeightOz > MaxWeaponWeightOz {
		c.at(key, path, ".weight_oz", ReasonValue, "the weight is 0 to %d ounces", MaxWeaponWeightOz)
	}
	if w.PriceCP < 0 || w.PriceCP > MaxWeaponPriceCP {
		c.at(key, path, ".price_cp", ReasonValue, "the price is 0 to %d copper pieces", MaxWeaponPriceCP)
	}
}

// checkWeaponProperties checks the property list: at most MaxWeaponProperties, each a weapon property, none twice.
func (b *overlayBuilder) checkWeaponProperties(c *entryErrors, w *TableWeapon, path string) {
	key := w.Key
	if len(w.Properties) > MaxWeaponProperties {
		c.at(key, path, ".properties", ReasonLimit, "%d properties; the limit is %d", len(w.Properties), MaxWeaponProperties)
	}
	for i, p := range w.Properties {
		switch {
		case !b.base.isWeaponProperty(p):
			c.at(key, path, fmt.Sprintf(".properties[%d]", i), ReasonReference, "%q is not a weapon property", p)
		case slices.Contains(w.Properties[:i], p):
			c.at(key, path, fmt.Sprintf(".properties[%d]", i), ReasonValue, "the property %q is listed twice", p)
		}
	}
}

// checkWeaponCombinations refuses the property combinations the SRD's weapons never have, each at .properties.
func checkWeaponCombinations(c *entryErrors, w *TableWeapon, path string) {
	has := func(p string) bool { return slices.Contains(w.Properties, weaponPropertyPrefix+p) }
	refuse := func(format string) { c.at(w.Key, path, ".properties", ReasonValue, "%s", format) }
	switch {
	case has("loading") && !has("ammunition"):
		refuse("Recarga só vale para uma arma com Munição.")
	case has("ammunition") && !w.Ranged:
		refuse("Munição é de arma à distância: uma arma corpo a corpo arremessada usa Arremesso.")
	case has("reach") && w.Ranged:
		refuse("Alcance é de arma corpo a corpo.")
	case has("two-handed") && has("versatile"):
		refuse("Versátil já diz que a arma vai em uma ou duas mãos: tire Duas mãos ou tire Versátil.")
	case has("two-handed") && has("light"):
		refuse("Leve é de arma usada em cada mão: ela não pode exigir Duas mãos.")
	}
}

// checkWeaponRange checks the range: a ranged weapon and a thrown melee weapon have a normal and a long range, and any
// other melee weapon has none.
func (b *overlayBuilder) checkWeaponRange(c *entryErrors, w *TableWeapon, path string) {
	thrown := slices.Contains(w.Properties, weaponPropertyPrefix+"thrown")
	switch {
	case w.Ranged || thrown:
		if w.NormalRangeFt < 1 || w.NormalRangeFt > MaxTableWeaponRangeFt {
			c.at(w.Key, path, ".normal_range_ft", ReasonValue, "the normal range is 1 to %d feet", MaxTableWeaponRangeFt)
		}
		if w.LongRangeFt < w.NormalRangeFt || w.LongRangeFt > MaxTableWeaponRangeFt {
			c.at(w.Key, path, ".long_range_ft", ReasonValue, "the long range is the normal range to %d feet", MaxTableWeaponRangeFt)
		}
	case w.NormalRangeFt != 0 || w.LongRangeFt != 0:
		c.at(w.Key, path, ".normal_range_ft", ReasonValue, "a melee weapon that is not thrown has no range: its reach is 5 feet, or 10 with the property reach")
	}
}

// weaponRow is the equipment part of a table weapon, as the SRD's rows are: the reach of a melee weapon is its normal range,
// and a thrown one has its throwing range apart.
func weaponRow(w *TableWeapon) *srd51.Weapon {
	row := &srd51.Weapon{
		Category: w.Category, Range: "melee", Damage: w.Damage, DamageType: w.DamageType, TwoHandedDamage: w.TwoHandedDamage,
		Properties: slices.Clone(w.Properties),
	}
	if row.Properties == nil {
		row.Properties = []string{}
	}
	if w.Ranged {
		row.Range, row.NormalRangeFt, row.LongRangeFt = "ranged", w.NormalRangeFt, w.LongRangeFt
		return row
	}
	row.NormalRangeFt = 5
	if slices.Contains(w.Properties, weaponPropertyPrefix+"reach") {
		row.NormalRangeFt = 10
	}
	if slices.Contains(w.Properties, weaponPropertyPrefix+"thrown") {
		row.ThrowNormalFt, row.ThrowLongFt = w.NormalRangeFt, w.LongRangeFt
	}
	return row
}

// ownEquipment copies the equipment map, once, so the base content's is never written.
func (b *overlayBuilder) ownEquipment() {
	if b.equipmentOwned {
		return
	}
	b.equipmentOwned = true
	b.n.equipment = maps.Clone(b.n.equipment)
}

// WeaponTableEntry is a table weapon as the content lists it.
type WeaponTableEntry struct {
	Key, NamePT   string
	Archived, Off bool
	// TwoHandedSince is TableWeapon.TwoHandedSince.
	TwoHandedSince int
}

// TwoHandedSince is the table revision at which the table's weapon key last became two-handed, or 0 when it is not a table
// weapon, is not two-handed, or was before the server kept it. A sheet that fits the table at that revision or a later one
// already had the weapon as it is.
func (c *Content) TwoHandedSince(key string) int { return c.c.weaponOf[key].TwoHandedSince }

// TableWeapons lists the table's weapons, sorted by Portuguese name.
func (c *Content) TableWeapons() []WeaponTableEntry {
	x := c.c
	out := make([]WeaponTableEntry, 0, len(x.weaponOf))
	for _, k := range sortedKeys(x.weaponOf) {
		e := x.weaponOf[k]
		e.Archived, e.Off = x.archived[k], x.off[k]
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b WeaponTableEntry) int { return strings.Compare(a.NamePT, b.NamePT) })
	return out
}
