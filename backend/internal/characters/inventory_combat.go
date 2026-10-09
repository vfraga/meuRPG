package characters

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What package play asks of the inventory during a combat (play.CombatRoster): the
// ammunition a ranged attack spends, and the items a combatant uses as an action. Like the
// rest of the roster they take no caller: they run after play's own authorization check, in
// play's transaction.

// lockedInventory locks a character of the campaign and reads its inventory.
func (s *Service) lockedInventory(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (*charactersdb.Queries, *inventoryDoc, error) {
	id, ok := parseUUID(characterID)
	if !ok {
		return nil, nil, errCharacterNotFound()
	}
	q := s.queries.WithTx(tx)
	row, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: campaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, errCharacterNotFound()
	}
	if err != nil {
		return nil, nil, wrap("lock a character", err)
	}
	d, err := inventoryDocOf(row)
	return q, d, err
}

// SpendAmmunition spends one piece of a stack of ammunition for a ranged attack, and counts it
// for the battle's recovery (SRD 5.1 "Ammunition": each attack expends one piece). A stack
// with none left is refused.
func (s *Service) SpendAmmunition(ctx context.Context, tx pgx.Tx, campaignID, characterID, itemID string) error {
	q, d, err := s.lockedInventory(ctx, tx, campaignID, characterID)
	if err != nil {
		return err
	}
	it, _ := findItem(d.inv, itemID)
	if it == nil || it.GetQuantity() < 1 {
		return (&itemBlock{reason: charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH, itemID: itemID}).err()
	}
	it.Quantity--
	it.AmmunitionSpent++
	d.changed = true
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return wrap("read rules content", err)
	}
	return s.saveInventory(ctx, q, content, d)
}

// ItemForUse describes a line the combatant could use now.
func (s *Service) ItemForUse(ctx context.Context, tx pgx.Tx, campaignID, characterID, itemID string) (link.UsableItem, error) {
	_, d, err := s.lockedInventory(ctx, tx, campaignID, characterID)
	if err != nil {
		return link.UsableItem{}, err
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return link.UsableItem{}, wrap("read rules content", err)
	}
	it, _ := findItem(d.inv, itemID)
	if it == nil {
		return link.UsableItem{}, invalidArgument(fieldErr("item_id", "is not an item of this inventory"))
	}
	ri := ruleItem(it)
	t := content.Traits(ri)
	out := link.UsableItem{
		ID: it.GetId(), Key: it.GetCatalogKey(), NamePT: content.ItemName(ri), Quantity: int(it.GetQuantity()),
		Unidentified: it.GetUnidentified(), Equipped: it.GetEquipped(),
	}
	def := t.Def
	switch {
	case t.Slot == rules.SlotShield && t.Equippable:
		out.Kind = "shield"
	case def != nil && def.Use != nil && def.Use.Scroll:
		out.Kind = "scroll"
		out.Spell, out.SpellLevel = it.GetScrollSpellKey(), def.Use.ScrollLevel
		out.SpellNamePT = content.NamePT(out.Spell)
		out.SaveDC, out.AttackBonus = def.Use.ScrollDC, def.Use.ScrollAttack
		derived := rules.Derive(buildOf(d.full), content)
		var ability rules.Ability
		out.Readable, out.TooHigh, ability, out.CheckMod = content.ScrollReader(derived, out.Spell)
		out.CheckAbility = string(ability)
	case def != nil && def.Use != nil:
		out.Kind, out.HealDice, out.TempHP = "potion", def.Use.HealDice, def.Use.TempHP
	case def != nil && def.Charges != nil:
		out.Kind = "charges"
		out.ChargesMax, out.ChargesLeft = def.Charges.Max, def.Charges.Max-int(it.GetChargesUsed())
		out.DestroyOnEmpty = def.Charges.DestroyOnEmpty
		for _, cs := range def.Charges.Spells {
			out.ChargeSpells = append(out.ChargeSpells, link.ChargeSpell{Spell: cs.Spell, NamePT: content.NamePT(cs.Spell), Level: cs.Level, Cost: cs.Cost})
		}
	}
	return out, nil
}

// ApplyItemUse does what a use did to the inventory, after play settled it.
func (s *Service) ApplyItemUse(ctx context.Context, tx pgx.Tx, campaignID, characterID string, w link.ItemUseWrite) error {
	q, d, err := s.lockedInventory(ctx, tx, campaignID, characterID)
	if err != nil {
		return err
	}
	it, _ := findItem(d.inv, w.ItemID)
	if it == nil {
		return invalidArgument(fieldErr("item_id", "is not an item of this inventory"))
	}
	switch w.Op {
	case "consume":
		if it.GetQuantity() <= 1 {
			d.inv.Items = slices.DeleteFunc(d.inv.Items, func(o *charactersv1.InventoryItem) bool { return o == it })
		} else {
			it.Quantity--
		}
	case "spend":
		it.ChargesUsed += clamp32(w.Charges)
		if w.Destroyed {
			d.inv.Items = slices.DeleteFunc(d.inv.Items, func(o *charactersv1.InventoryItem) bool { return o == it })
		}
	case "equip", "unequip":
		it.Equipped = w.Op == "equip"
	default:
		return connect.NewError(connect.CodeInternal, errors.New("unknown item use"))
	}
	d.changed = true
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return wrap("read rules content", err)
	}
	if err := s.saveInventory(ctx, q, content, d); err != nil {
		return err
	}
	return nil
}
