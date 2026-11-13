package rules

import "fmt"

// Magic bonus of gear (SRD 5.1, paraphrased).
// A magic weapon adds its bonus to the attack and damage rolls made with it.
// Magic armor and a magic shield add theirs to the armor class.
// The bonus stands apart from proficiency and ability scores.
// A carried piece adds nothing, and neither does one that asks for attunement while the character has none.
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

// itemModifiers fills Derived.ItemModifiers.
func (x *deriver) itemModifiers() {
	for _, ib := range x.b.ItemBonuses {
		if !x.listsBonus(ib.Key) {
			continue
		}
		name := x.c.namePT(ib.Key)
		if name == "" {
			name = ib.Key
		}
		reason, on := "", ib.counts()
		switch {
		case x.b.IsCarried(ib.Key):
			reason, on = "Carregado, não equipado.", false
		case ib.Key == shieldKey && x.b.Shield && !x.shieldWielded():
			reason, on = "Uma arma de duas mãos equipada deixa o escudo parado.", false
		case !ib.counts() && ib.Bonus > 0:
			reason = "Pede sintonização."
		}
		add := func(target string) {
			x.d.ItemModifiers = append(x.d.ItemModifiers, ItemModifier{
				Target: target, Value: ib.Bonus, SourceKey: ib.Key, SourceNamePT: name, Counts: on, ReasonPT: reason,
			})
		}
		if ib.Key == shieldKey || ib.Key == x.sheetArmor {
			add("ac")
			continue
		}
		add("attack:" + ib.Key)
		add("damage:" + ib.Key)
	}
}

// checkItemBonuses checks the bonuses on the gear: at most MaxCarried, each on a piece of the sheet, none twice, each
// from 1 to MaxItemBonus.
func checkItemBonuses(b *Build) error {
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
		}
		seen[ib.Key] = true
	}
	return nil
}
