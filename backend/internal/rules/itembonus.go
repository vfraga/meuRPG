package rules

import "fmt"

// Magic bonus of gear (SRD 5.1, paraphrased).
// A magic weapon adds its bonus to the attack and damage rolls made with it.
// Magic armor and a magic shield add theirs to the armor class.
// The bonus stands apart from proficiency and ability scores.
// A carried piece adds nothing, and neither does one that asks for attunement while the character has none.
// An infusion with a bonus gives it the same way: the record becomes a bonus of its piece, and the piece takes no manual one.
// The bonus of a weapon key applies to every copy of that key.

// MaxItemBonus is the largest magic bonus of a piece.
const MaxItemBonus = 3

// counts says the bonus adds to the sheet once the piece is equipped.
func (ib ItemBonus) counts() bool { return ib.Bonus > 0 && (!ib.RequiresAttunement || ib.Attuned) }

func (b Build) itemBonus(key string) (ItemBonus, bool) {
	for _, ib := range b.ItemBonuses {
		if ib.Key == key {
			return ib, true
		}
	}
	return ItemBonus{}, false
}

// armorItemBonus is what the equipped armor and the shield that counts add to the armor class.
func (x *deriver) armorItemBonus() int {
	n := 0
	if ib, ok := x.b.itemBonus(x.b.Armor); ok && ib.counts() {
		n += ib.Bonus
	}
	if ib, ok := x.b.itemBonus(shieldKey); ok && x.shieldWielded() && ib.counts() {
		n += ib.Bonus
	}
	return n
}

// weaponItemBonus is what the weapon key adds to its attack and to its damage while it is equipped.
func (x *deriver) weaponItemBonus(key string) int {
	ib, ok := x.b.itemBonus(key)
	if !ok || x.b.IsCarried(key) || !ib.counts() {
		return 0
	}
	return ib.Bonus
}

// listsBonus says the piece of the bonus has a number to change: the armor and the shield do, and a weapon only when the
// content knows it, because a weapon of the content has no attack line.
func (x *deriver) listsBonus(key string) bool {
	if key == shieldKey || key == x.sheetArmor {
		return true
	}
	eq, ok := x.c.equipment[key]
	return ok && eq.Weapon != nil
}

// itemModifiers fills Derived.ItemModifiers: the bonuses in order (an infusion's after the manual ones), then the
// spell focus lines.
func (x *deriver) itemModifiers() {
	for _, ib := range x.b.ItemBonuses {
		if !x.listsBonus(ib.Key) {
			continue
		}
		name := x.c.namePT(ib.Key)
		if name == "" {
			name = ib.Key
		}
		reason, on := x.bonusReason(ib)
		m := ItemModifier{Value: ib.Bonus, SourceKey: ib.Key, SourceNamePT: name, Counts: on, ReasonPT: reason}
		if l, ok := x.bonusFrom[ib.Key]; ok {
			m.InfusionNamePT, m.InfusionID = x.infusionNamePT(l.rec.InfusionKey), l.rec.ID
		}
		if ib.Key == shieldKey || ib.Key == x.sheetArmor {
			m.Target = "ac"
			x.d.ItemModifiers = append(x.d.ItemModifiers, m)
			continue
		}
		for _, target := range []string{"attack:", "damage:"} {
			m.Target = target + ib.Key
			x.d.ItemModifiers = append(x.d.ItemModifiers, m)
		}
	}
	x.d.ItemModifiers = append(x.d.ItemModifiers, x.focusLines()...)
}

// bonusReason says whether the bonus counts now and, when it does not, why.
func (x *deriver) bonusReason(ib ItemBonus) (reason string, on bool) {
	on = ib.counts()
	switch {
	case x.b.IsCarried(ib.Key):
		return "Carregado, não equipado.", false
	case ib.Key == shieldKey && x.b.Shield && !x.shieldWielded():
		return "Uma arma de duas mãos equipada deixa o escudo parado.", false
	case !on && ib.Bonus > 0:
		return "Pede sintonização.", false
	}
	return "", on
}

// checkItemBonuses checks the bonuses on the gear: at most MaxCarried, each on a piece of the sheet, none twice, each
// from 1 to MaxItemBonus.
func checkItemBonuses(b *Build, c *content) error {
	if len(b.ItemBonuses) > MaxCarried {
		return invalid("full.item_bonuses", "must have at most %d entries", MaxCarried)
	}
	seen := make(map[string]bool, len(b.ItemBonuses))
	for i, ib := range b.ItemBonuses {
		f := fmt.Sprintf("full.item_bonuses[%d]", i)
		switch {
		case !b.listsGear(ib.Key):
			return invalid(f+".key", "is not the sheet's armor, its shield or one of its weapons")
		case seen[ib.Key]:
			return invalid(f+".key", "repeats an entry")
		case ib.Bonus < 1 || ib.Bonus > MaxItemBonus:
			return invalid(f+".bonus", "must be from 1 to %d", MaxItemBonus)
		case ib.ItemKey != "" && !fitsCatalogItem(b, c, ib):
			return invalid(f+".item_key", "is not a catalog item (no family) that fits the piece: armor for the armor and the shield, a weapon for a weapon")
		}
		seen[ib.Key] = true
	}
	return nil
}

// CheckManualBonuses refuses a manual item bonus that the save adds or changes on a gear key that a valid item bonus
// infusion of incoming (its Infusions) gives its bonus. A pair that is already on the stored sheet is left alone:
// Derive resolves it, so the sheet keeps saving. The error is a ValidationError at full.item_bonuses[i].key.
func CheckManualBonuses(stored, incoming Build, c *Content) error {
	for i, ib := range incoming.ItemBonuses {
		if old, ok := stored.itemBonus(ib.Key); ok && old.Bonus == ib.Bonus && old.RequiresAttunement == ib.RequiresAttunement && old.ItemKey == ib.ItemKey {
			continue
		}
		if c.c.infusesBonusOf(incoming, ib.Key) {
			return invalid(fmt.Sprintf("full.item_bonuses[%d].key", i), "has an infusion that gives its magic bonus: end the infusion, or leave the manual bonus off")
		}
	}
	return nil
}

// fitsCatalogItem says the piece names a catalog item that is no family and whose category fits it.
func fitsCatalogItem(b *Build, c *content, ib ItemBonus) bool {
	m, ok := c.magicItems[ib.ItemKey]
	if !ok || len(m.Variants) > 0 && !m.Standalone {
		return false
	}
	if ib.Key == b.Armor || ib.Key == ShieldKey {
		return m.Category == "armor"
	}
	return m.Category == "weapon"
}
