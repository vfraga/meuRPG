package characters

import (
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The rules of the inventory (SRD 5.1 "Magic Items": attunement, wearing and
// wielding items), as pure functions of the inventory, the character and the rules
// content: no database, no clock. The calls in inventory_rpc.go use them inside the
// transaction.

// itemBlock is a refusal by the rules: the reason, the item in the way and how many
// the character has.
type itemBlock struct {
	reason charactersv1.ItemBlockedReason
	itemID string
	have   int32
}

var itemBlockMessages = map[charactersv1.ItemBlockedReason]string{
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_SLOT_TAKEN:             "something else is worn there: take it off first",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_EQUIPPABLE:         "this item is not worn or wielded",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ATTUNABLE:          "this item does not need attunement",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ATTUNEMENT_FULL:        "a character attunes to at most three magic items: end one attunement first",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ATTUNEMENT_RESTRICTION: "the character does not meet the item's attunement restriction",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_COPY_ATTUNED:           "the character is attuned to another copy of this item",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ALREADY_ATTUNED:        "the character is already attuned to this item",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ATTUNED:            "the character is not attuned to this item",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ITEM_CURSED:            "the item is cursed: the attunement does not end by choice",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_IN_COMBAT:              "a combat is going on: this waits for a moment outside it",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH:             "there is not enough of it",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_USABLE:             "this item is not for drinking or reading",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ALREADY_IDENTIFIED:     "the item is already identified",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOTHING_TO_ANSWER:      "there is no request on this item to answer",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_REQUEST_WAITING:        "a request on this item is waiting for the master",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_SCROLL_UNREADABLE:      "the scroll's spell is not on the reader's class lists: the scroll is unintelligible",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_SCROLL_TOO_HIGH:        "the scroll's spell is above what the reader can cast: it takes an ability check the master rolls",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_FULL:                   "the inventory is full",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_USE_IN_COMBAT:          "in a combat, using an item is an action: use it from the combat",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_CAST_IT:                "this item is cast as a spell, not used here",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_CANNOT_RECEIVE:         "the receiver cannot take items",
	charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_DEAD:                   "the character is dead",
}

// err is the failed_precondition the client gets, with the ItemBlocked detail.
func (b *itemBlock) err() error {
	e := connect.NewError(connect.CodeFailedPrecondition, errors.New(itemBlockMessages[b.reason]))
	if d, err := connect.NewErrorDetail(&charactersv1.ItemBlocked{Reason: b.reason, ItemId: b.itemID, Have: b.have}); err == nil {
		e.AddDetail(d)
	}
	return e
}

func block(reason charactersv1.ItemBlockedReason, itemID string) *itemBlock {
	return &itemBlock{reason: reason, itemID: itemID}
}

// invRules is what the checks know: the content, the character they are about, and
// whether a combat is going on.
type invRules struct {
	content  *rules.Content
	who      rules.Character
	inCombat bool
}

func (r invRules) traits(it *charactersv1.InventoryItem) rules.ItemTraits {
	return r.content.Traits(ruleItem(it))
}

// ruleItem is one stored line as package rules reads it.
func ruleItem(it *charactersv1.InventoryItem) rules.Item {
	return rules.Item{
		ID: it.GetId(), Key: it.GetCatalogKey(), Base: it.GetBaseKey(), Option: it.GetOptionKey(), Name: it.GetName(),
		Quantity: int(it.GetQuantity()), Equipped: it.GetEquipped(), Attuned: it.GetAttuned(),
		Unidentified: it.GetUnidentified(), Look: it.GetLook(),
	}
}

// slotOfItem is where the line sits: "" for none.
func (r invRules) slotOfItem(it *charactersv1.InventoryItem) string { return r.traits(it).Slot }

// equip checks wearing or wielding an item (SRD 5.1 "Wearing and Wielding Items",
// "Multiple Items of the Same Kind"). master may change body armor in a combat.
func (r invRules) equip(inv *charactersv1.Inventory, it *charactersv1.InventoryItem, master bool) *itemBlock {
	t := r.traits(it)
	if !t.Equippable {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_EQUIPPABLE, it.GetId())
	}
	if r.inCombat && !master {
		switch t.Slot {
		case rules.SlotBody:
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ARMOR_IN_COMBAT, it.GetId())
		case rules.SlotShield: // an action: it goes through the combat
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_USE_IN_COMBAT, it.GetId())
		}
	}
	capacity := rules.SlotCapacity(t.Slot)
	if capacity == 0 {
		return nil
	}
	worn := 0
	var other string
	for _, o := range inv.GetItems() {
		if o.GetId() == it.GetId() || !o.GetEquipped() || r.slotOfItem(o) != t.Slot {
			continue
		}
		worn++
		if other == "" {
			other = o.GetId()
		}
	}
	if worn >= capacity {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_SLOT_TAKEN, other)
	}
	return nil
}

// unequip checks taking an item off: the body armor stays on in a combat for a
// player.
func (r invRules) unequip(it *charactersv1.InventoryItem, master bool) *itemBlock {
	if r.inCombat && !master {
		switch r.traits(it).Slot {
		case rules.SlotBody:
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ARMOR_IN_COMBAT, it.GetId())
		case rules.SlotShield:
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_USE_IN_COMBAT, it.GetId())
		}
	}
	return nil
}

// attunements counts the magic items the character is attuned to and still
// qualifies for: an attunement ends when the character no longer meets the item's
// restriction (SRD 5.1 "Attunement").
func (r invRules) attunements(inv *charactersv1.Inventory) int {
	n := 0
	for _, it := range inv.GetItems() {
		if it.GetAttuned() && r.traits(it).Restriction.Met(r.who) {
			n++
		}
	}
	return n
}

// attune checks attuning to a magic item. unknown says the player does not know the
// item (it is unidentified and the caller is not the master): then only what the
// player can know is checked, so a refusal does not give the item away.
func (r invRules) attune(inv *charactersv1.Inventory, it *charactersv1.InventoryItem, unknown bool) *itemBlock {
	if r.inCombat {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_IN_COMBAT, it.GetId())
	}
	if it.GetCatalogKey() == "" || it.GetAttuned() {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ALREADY_ATTUNED, it.GetId())
	}
	if r.attunements(inv) >= attunementMax {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ATTUNEMENT_FULL, it.GetId())
	}
	if unknown {
		return nil
	}
	t := r.traits(it)
	switch {
	case t.Kind != rules.ItemMagic || !t.RequiresAttunement:
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ATTUNABLE, it.GetId())
	case !t.Restriction.Met(r.who):
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ATTUNEMENT_RESTRICTION, it.GetId())
	}
	// A creature cannot attune to more than one copy of an item.
	for _, o := range inv.GetItems() {
		if o.GetId() != it.GetId() && o.GetAttuned() && o.GetCatalogKey() == it.GetCatalogKey() {
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_COPY_ATTUNED, o.GetId())
		}
	}
	return nil
}

// endAttunement checks ending an attunement by choice (SRD 5.1: another short rest
// focused on the item, unless the item is cursed).
func (r invRules) endAttunement(it *charactersv1.InventoryItem, unknown bool) *itemBlock {
	switch {
	case r.inCombat:
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_IN_COMBAT, it.GetId())
	case !it.GetAttuned():
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ATTUNED, it.GetId())
	case !unknown && r.traits(it).Cursed:
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ITEM_CURSED, it.GetId())
	}
	return nil
}

// identify checks identifying an item with a short rest.
func (r invRules) identify(it *charactersv1.InventoryItem) *itemBlock {
	switch {
	case r.inCombat:
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_IN_COMBAT, it.GetId())
	case !it.GetUnidentified():
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ALREADY_IDENTIFIED, it.GetId())
	}
	return nil
}

// grantRules says what a caller may give.
type grantRules struct {
	// master gives anything; the owner of a draft sheet gives equipment and free text.
	master bool
}

// newItem builds the line a grant describes, or the field it breaks.
func (r invRules) newItem(field string, g *charactersv1.ItemGrant, gr grantRules) (*charactersv1.InventoryItem, error) {
	it := &charactersv1.InventoryItem{Id: newItemID(), Quantity: g.GetQuantity()}
	if it.Quantity == 0 {
		it.Quantity = 1
	}
	if it.Quantity < 1 || it.Quantity > maxItemQuantity {
		return nil, fieldErr(field+".quantity", "must be 1 to %d", maxItemQuantity)
	}
	key := g.GetCatalogKey()
	if key == "" {
		name, err := names.Clean(g.GetName(), maxItemNameLength)
		if err != nil {
			return nil, &fieldError{field: field + ".name", err: err}
		}
		if g.GetBaseKey() != "" || g.GetOptionKey() != "" || g.GetUnidentified() || g.GetLook() != "" || g.GetScrollSpellKey() != "" || g.GetEquipped() {
			return nil, fieldErr(field, "a free-text item takes a name and a quantity only")
		}
		it.Name = name
		return it, nil
	}
	if g.GetName() != "" {
		return nil, fieldErr(field+".name", "must be empty when catalog_key is set")
	}
	it.CatalogKey = key
	t := r.content.Traits(rules.Item{Key: key})
	if !t.Known {
		return nil, fieldErr(field+".catalog_key", "is not an item of the rules content")
	}
	if t.Kind == rules.ItemEquipment {
		if g.GetBaseKey() != "" || g.GetOptionKey() != "" || g.GetUnidentified() || g.GetLook() != "" || g.GetScrollSpellKey() != "" {
			return nil, fieldErr(field, "equipment takes a quantity only")
		}
		return it, r.checkEquipGrant(field, g, it, gr)
	}
	if !gr.master {
		return nil, errPermission("only the master gives magic items")
	}
	if err := r.fillMagic(field, g, it, t); err != nil {
		return nil, err
	}
	return it, r.checkEquipGrant(field, g, it, gr)
}

func (r invRules) checkEquipGrant(field string, g *charactersv1.ItemGrant, it *charactersv1.InventoryItem, gr grantRules) error {
	if !g.GetEquipped() {
		return nil
	}
	if !gr.master {
		return errPermission("only the master gives an item already worn")
	}
	if !r.traits(it).Equippable {
		return fieldErr(field+".equipped", "this item is not worn or wielded")
	}
	it.Equipped = true
	return nil
}

// fillMagic checks the base, the option, the scroll and the look of a magic item.
func (r invRules) fillMagic(field string, g *charactersv1.ItemGrant, it *charactersv1.InventoryItem, t rules.ItemTraits) error {
	entry, _ := r.content.MagicItem(it.GetCatalogKey())
	if entry.IsFamily() && !entry.Standalone {
		return fieldErr(field+".catalog_key", "is a family: give one of its variants")
	}
	if !t.Stackable && it.GetQuantity() != 1 {
		return fieldErr(field+".quantity", "a magic item that is not ammunition, a potion or a scroll comes one at a time")
	}
	d := t.Def
	// The base of a generic item.
	switch {
	case d != nil && len(d.BaseChoices) > 0:
		if !slices.Contains(d.BaseChoices, g.GetBaseKey()) {
			return fieldErr(field+".base_key", "must be one of the equipment this item can be")
		}
		it.BaseKey = g.GetBaseKey()
	case g.GetBaseKey() != "":
		return fieldErr(field+".base_key", "this item has no base")
	}
	switch {
	case d != nil && len(d.OptionDamageTypes) > 0:
		if !slices.Contains(d.OptionDamageTypes, g.GetOptionKey()) {
			return fieldErr(field+".option_key", "must be one of the item's damage types")
		}
		it.OptionKey = g.GetOptionKey()
	case g.GetOptionKey() != "":
		return fieldErr(field+".option_key", "this item has no option")
	}
	if err := r.fillScroll(field, g, it, d); err != nil {
		return err
	}
	if g.GetUnidentified() {
		it.Unidentified = true
		if g.GetLook() != "" {
			look, err := names.Clean(g.GetLook(), maxItemNameLength)
			if err != nil {
				return &fieldError{field: field + ".look", err: err}
			}
			it.Look = look
		}
	} else if g.GetLook() != "" {
		return fieldErr(field+".look", "only an unidentified item has a look")
	}
	return nil
}

func (r invRules) fillScroll(field string, g *charactersv1.ItemGrant, it *charactersv1.InventoryItem, d *rules.ItemDef) error {
	if d == nil || d.Use == nil || !d.Use.Scroll {
		if g.GetScrollSpellKey() != "" {
			return fieldErr(field+".scroll_spell_key", "only a spell scroll has a spell")
		}
		return nil
	}
	level, ok := r.content.SpellLevel(g.GetScrollSpellKey())
	if !ok {
		return fieldErr(field+".scroll_spell_key", "must be a spell of the rules content")
	}
	if level != d.Use.ScrollLevel {
		return fieldErr(field+".scroll_spell_key", "must be a spell of the level %d the scroll holds", d.Use.ScrollLevel)
	}
	it.ScrollSpellKey = g.GetScrollSpellKey()
	return nil
}

// stackInto adds a new line to the inventory, joining the line it is the same as when
// the item stacks. It returns the line that now holds the pieces.
func (r invRules) stackInto(inv *charactersv1.Inventory, n *charactersv1.InventoryItem) (*charactersv1.InventoryItem, error) {
	if r.traits(n).Stackable {
		for _, o := range inv.GetItems() {
			if rules.SameStack(ruleItem(o), ruleItem(n), o.GetScrollSpellKey(), n.GetScrollSpellKey()) {
				if o.GetQuantity()+n.GetQuantity() > maxItemQuantity {
					return nil, fieldErr("quantity", "a line holds at most %d", maxItemQuantity)
				}
				o.Quantity += n.GetQuantity()
				return o, nil
			}
		}
	}
	if len(inv.GetItems()) >= rules.MaxInventoryItems {
		return nil, &fullError{}
	}
	inv.Items = append(inv.Items, n)
	return n, nil
}

// fullError says the inventory has no room for another line.
type fullError struct{}

func (*fullError) Error() string { return "the inventory is full" }

func (e *fullError) block() *itemBlock {
	return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_FULL, "")
}

// describe is a name for logs of the rules in tests.
func (b *itemBlock) String() string { return fmt.Sprint(b.reason) }
