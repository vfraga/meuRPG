package rules

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// Potions (SRD 5.1, "Potions"): the potions whose drinking the app carries out. A potion that is not in
// effects/potions.json is spent and logged by the table, with its text, and has no automatic effect; it is still a potion.

// The kinds of effect of a potion.
const (
	// PotionHeal restores hit points: the potion's dice, rolled once.
	PotionHeal = "heal"
	// PotionResistance gives the drinker resistance to one damage type for a time.
	PotionResistance = "resistance"
)

// magicCategoryPotion is the category of the SRD magic items that are potions (the oils are filed with them).
const magicCategoryPotion = "potion"

// PotionResistanceRounds is the hour the resistance potions last, in the rounds the lasting effects count (6 seconds).
const PotionResistanceRounds = 600

// PotionEffect is what drinking a catalog potion does.
type PotionEffect struct {
	// Kind is PotionHeal or PotionResistance.
	Kind string
	// Heal is the dice of the hit points restored (PotionHeal); the potion adds no modifier.
	Heal DiceFormula
	// DamageType is the key ("damage-type:fire") of the damage the drinker resists, and Rounds how long it lasts, in
	// rounds (PotionResistance).
	DamageType string
	Rounds     int
}

// potionFile is one entry of effects/potions.json, which maps a potion's key to it.
type potionFile struct {
	Kind       string `json:"kind"`
	Dice       string `json:"dice,omitempty"`
	DamageType string `json:"damage_type,omitempty"`
	Rounds     int    `json:"rounds,omitempty"`
}

// loadPotions reads effects/potions.json and refuses a key that is not a consumable of the catalog, an unknown kind, a
// heal without dice or with the fields of a resistance, and a resistance without a damage type of the catalog or without
// a time of one hour at most.
func (c *content) loadPotions(fsys fs.FS) error {
	const name = "effects/potions.json"
	var f map[string]potionFile
	if err := readJSON(fsys, name, &f); err != nil {
		return err
	}
	c.potions = map[string]PotionEffect{}
	for _, k := range sortedKeys(f) {
		e, err := c.potionOf(k, f[k])
		if err != nil {
			return fmt.Errorf("%s: %s: %w", name, k, err)
		}
		c.potions[k] = e
	}
	return nil
}

func (c *content) potionOf(key string, e potionFile) (PotionEffect, error) {
	if m, ok := c.magicItems[key]; !ok || !c.consumables[key] || (len(m.Variants) > 0 && !m.Standalone) {
		return PotionEffect{}, fmt.Errorf("is not a consumable the sheet holds")
	}
	switch e.Kind {
	case PotionHeal:
		return potionHeal(e)
	case PotionResistance:
		return c.potionResistance(e)
	}
	return PotionEffect{}, fmt.Errorf("kind is %q or %q", PotionHeal, PotionResistance)
}

func potionHeal(e potionFile) (PotionEffect, error) {
	if e.DamageType != "" || e.Rounds != 0 {
		return PotionEffect{}, fmt.Errorf("a heal has dice only")
	}
	var d DiceFormula
	if err := readDice(&d, e.Dice, "dice"); err != nil {
		return PotionEffect{}, err
	}
	if d.Count < 1 || d.Sides < 1 {
		return PotionEffect{}, fmt.Errorf("dice is dice such as 2d4+2")
	}
	return PotionEffect{Kind: PotionHeal, Heal: d}, nil
}

func (c *content) potionResistance(e potionFile) (PotionEffect, error) {
	switch {
	case e.Dice != "":
		return PotionEffect{}, fmt.Errorf("a resistance has no dice")
	case !c.isLiveDamageType(e.DamageType):
		return PotionEffect{}, fmt.Errorf("damage_type is a damage type of the catalog")
	case e.Rounds < 1 || e.Rounds > PotionResistanceRounds:
		return PotionEffect{}, fmt.Errorf("rounds is 1 to %d (one hour)", PotionResistanceRounds)
	}
	return PotionEffect{Kind: PotionResistance, DamageType: e.DamageType, Rounds: e.Rounds}, nil
}

// PotionEffect is what drinking the potion does, and false for a potion the app does not carry out.
func (c *Content) PotionEffect(key string) (PotionEffect, bool) {
	e, ok := c.c.potions[key]
	return e, ok
}

// oils are the oils the SRD files with the potions. Applying one takes a minute or ten, not an action, so the combat does not
// gate its use: the table spends it and the master judges the time. This is the one place that names them.
var oils = map[string]bool{
	"item:oil-of-etherealness": true,
	"item:oil-of-sharpness":    true,
	"item:oil-of-slipperiness": true,
}

// IsPotion says the item is a potion that is drunk or given, by its SRD category (magic-items.json), whether or not the app
// carries out what it does: the potions of effects/potions.json and the ones that are only text are all potions, and the use of one
// is an action in a combat. The oils of the same category are not (see oils), and neither are a scroll, any other item or a key
// that is no item of the catalog.
func (c *Content) IsPotion(key string) bool {
	m, ok := c.c.magicItems[key]
	return ok && m.Category == magicCategoryPotion && !oils[key]
}

// PotionEffectLinePT is the line the sheet shows for what the potion does ("Cura 2d4 + 2", "Resistência a fogo por 1
// hora"), and "" for an item the app carries out nothing for.
func (c *Content) PotionEffectLinePT(key string) string {
	e, ok := c.PotionEffect(key)
	switch {
	case !ok:
		return ""
	case e.Kind == PotionHeal:
		dice := strconv.Itoa(e.Heal.Count) + "d" + strconv.Itoa(e.Heal.Sides)
		if e.Heal.Bonus > 0 {
			dice += " + " + strconv.Itoa(e.Heal.Bonus)
		}
		return "Cura " + dice
	}
	return "Resistência a " + strings.ToLower(c.NamePT(e.DamageType)) + " por " + durationPT(e.Rounds*SecondsPerRound)
}

// durationPT is a time in seconds as the line says it: whole hours, else whole minutes, else seconds.
func durationPT(seconds int) string {
	unit := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return strconv.Itoa(n) + " " + many
	}
	switch {
	case seconds%3600 == 0:
		return unit(seconds/3600, "hora", "horas")
	case seconds%60 == 0:
		return unit(seconds/60, "minuto", "minutos")
	}
	return unit(seconds, "segundo", "segundos")
}
