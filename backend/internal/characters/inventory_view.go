package characters

import (
	"cmp"
	"slices"
	"strings"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What a viewer may know of the inventory (RN-10). The master sees every line as it
// is. A player sees their own lines, except that a magic item the master has not
// identified shows only its look, its quantity, whether it is worn and a request to
// identify it: nothing of what it is, so no name, key, category, rarity, charges,
// attunement or use (SRD 5.1 "Identifying a Magic Item").

var itemKindToProto = map[string]charactersv1.ItemKind{
	rules.ItemFree:      charactersv1.ItemKind_ITEM_KIND_FREE_TEXT,
	rules.ItemEquipment: charactersv1.ItemKind_ITEM_KIND_EQUIPMENT,
	rules.ItemMagic:     charactersv1.ItemKind_ITEM_KIND_MAGIC,
}

var coverageToProto = map[rules.ItemCoverage]charactersv1.ItemCoverage{
	rules.CoverageApplied:  charactersv1.ItemCoverage_ITEM_COVERAGE_APPLIED,
	rules.CoveragePartial:  charactersv1.ItemCoverage_ITEM_COVERAGE_PARTIAL,
	rules.CoverageReminder: charactersv1.ItemCoverage_ITEM_COVERAGE_REMINDER,
}

// entryOf is the line as the viewer may know it.
func entryOf(content *rules.Content, it *charactersv1.InventoryItem, master bool) *charactersv1.InventoryEntry {
	ri := ruleItem(it)
	t := content.Traits(ri)
	e := &charactersv1.InventoryEntry{
		Id: it.GetId(), Kind: itemKindToProto[t.Kind], NamePt: content.ItemName(ri), Quantity: it.GetQuantity(),
		Equipped: it.GetEquipped(), Unidentified: it.GetUnidentified(),
	}
	if it.GetUnidentified() && !master {
		// The player knows what it looks like and whether it is worn. The only thing
		// they may ask of it is to learn what it is.
		if it.GetRequest() == charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_IDENTIFY {
			e.Request = it.GetRequest()
		}
		return e
	}
	e.Attuned, e.Request = it.GetAttuned(), it.GetRequest()
	e.CatalogKey, e.BaseKey, e.OptionKey = it.GetCatalogKey(), it.GetBaseKey(), it.GetOptionKey()
	e.Equippable, e.Cursed, e.AmmunitionSpent = t.Equippable, t.Cursed, it.GetAmmunitionSpent()
	if t.Ammunition {
		e.AmmunitionRecoverable = ammunitionRecoverable(it.GetAmmunitionSpent())
	}
	e.EffectsPending = master && it.GetUnidentified() && t.Def != nil && t.Def.Coverage() != rules.CoverageReminder
	e.Unknown = !t.Known
	e.Slot = t.Slot
	if master {
		e.Look = it.GetLook()
	}
	switch t.Kind {
	case rules.ItemEquipment:
		if info, ok := content.Equipment(it.GetCatalogKey()); ok {
			e.Category = content.CategoryNamePT("equipment", info.Kind)
		}
	case rules.ItemMagic:
		fillMagicEntry(content, it, t, e)
	}
	if it.GetEquipped() && isWeaponLine(content, ri, t) {
		e.AttackKey = "inv:" + it.GetId()
	}
	return e
}

func fillMagicEntry(content *rules.Content, it *charactersv1.InventoryItem, t rules.ItemTraits, e *charactersv1.InventoryEntry) {
	m, ok := content.MagicItem(it.GetCatalogKey())
	if !ok {
		return
	}
	e.Category = content.CategoryNamePT("item", m.Category)
	e.Rarity = m.Rarity
	e.RequiresAttunement, e.AttunementByPt = m.Attunement, m.AttunementByPT
	e.Coverage = coverageToProto[content.ItemCoverage(it.GetCatalogKey())]
	e.Usable = t.Usable && t.Def != nil && t.Def.Use != nil && !t.Def.Use.Scroll
	e.ScrollSpellKey = it.GetScrollSpellKey()
	if e.ScrollSpellKey != "" {
		e.ScrollSpellNamePt = content.NamePT(e.ScrollSpellKey)
	}
	if d := t.Def; d != nil {
		e.NotePt = d.NotePT
		if d.Charges != nil {
			e.ChargesMax, e.ChargesUsed = int32(d.Charges.Max), it.GetChargesUsed()
		}
		if d.Use != nil {
			e.HealDice = d.Use.HealDice
		}
	}
}

// isWeaponLine says the line is a weapon or has a weapon as its base: it makes an
// attack while it is wielded.
func isWeaponLine(content *rules.Content, ri rules.Item, t rules.ItemTraits) bool {
	base := ri.Key
	if t.Kind == rules.ItemMagic {
		base = ri.Base
		if t.Def != nil && t.Def.BaseFixed != "" {
			base = t.Def.BaseFixed
		}
	}
	info, ok := content.Equipment(base)
	return ok && info.Kind == "weapon"
}

// viewOf builds the inventory the caller may see.
func viewOf(content *rules.Content, rl invRules, full *charactersv1.FullSheet, characterID string, master, canEdit bool) *charactersv1.InventoryView {
	inv := full.GetInventory()
	v := &charactersv1.InventoryView{
		CharacterId: characterID, Coins: full.GetCoins(), AttunedCount: int32(rl.attunements(inv)),
		AttunementMax: attunementMax, CanEdit: canEdit, InCombat: rl.inCombat,
	}
	if v.Coins == nil {
		v.Coins = &charactersv1.Coins{}
	}
	v.ConvertedTexts = slices.Clone(inv.GetConvertedTexts())
	for _, it := range inv.GetItems() {
		v.Items = append(v.Items, entryOf(content, it, master))
	}
	// The sheet's order is the order things were given; the list reads by kind then name.
	slices.SortStableFunc(v.Items, func(a, b *charactersv1.InventoryEntry) int {
		return cmp.Or(cmp.Compare(rankOfKind(a), rankOfKind(b)), cmpFold(a.GetNamePt(), b.GetNamePt()))
	})
	return v
}

func rankOfKind(e *charactersv1.InventoryEntry) int {
	switch {
	case e.GetEquipped():
		return 0
	case e.GetKind() == charactersv1.ItemKind_ITEM_KIND_FREE_TEXT:
		return 2
	}
	return 1
}

func cmpFold(a, b string) int { return cmp.Compare(strings.ToLower(a), strings.ToLower(b)) }
