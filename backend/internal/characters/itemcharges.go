package characters

import (
	"context"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Charges (SRD 5.1): the maximum of an item comes from the catalog (rules.ItemCharges) and what a character
// spent from character_item_charges, one row for each entry of the sheet's magic items. A read fills
// charges_max, charges_left and destroyed on the entries; a save never changes them.

// fillItemCharges fills the charges of the character's magic items from the stored state. The caller is the player
// the character belongs to or the master (canSee), so the counts reach nobody else (RN-10).
func (s *Service) fillItemCharges(ctx context.Context, content *rules.Content, c *charactersv1.Character) error {
	full := c.GetSheet().GetFull()
	if len(full.GetMagicItems()) == 0 {
		return nil
	}
	rows, err := s.queries.ListItemCharges(ctx, c.GetId())
	if err != nil {
		return wrap("read item charges", err)
	}
	state := map[string]charactersdb.ListItemChargesRow{}
	for _, r := range rows {
		state[r.EntryID] = r
	}
	for _, it := range full.GetMagicItems() {
		it.EffectPt = content.PotionEffectLinePT(it.GetKey())
		ic, ok := content.ItemCharges(it.GetKey())
		if !ok {
			continue
		}
		r, set := state[it.GetId()]
		applyItemCharges(it, ic, r, set)
		it.Recharge = itemRechargeOf(ic)
	}
	return nil
}

// applyItemCharges fills the charge fields of an entry from the catalog's numbers and the stored row (set says the
// row exists).
func applyItemCharges(it *charactersv1.SheetMagicItem, ic rules.ItemCharges, r charactersdb.ListItemChargesRow, set bool) {
	it.ChargesMax, it.ChargesLeft = ic.Max, ic.Left(r.ChargesUsed)
	it.Lost, it.LastRollPending = charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED, false
	switch {
	case r.Lost != "":
		it.Lost, it.ChargesLeft = itemLossOf(r.Lost), 0
	case !set && ic.HasStart():
		it.ChargesLeft = -1 // the SRD rolls its charges: the master sets them
	default:
		it.LastRollPending = r.LastRollPending
	}
}

// dropRemovedItemCharges deletes the spent charges of the entries a saved sheet no longer has, so an entry that
// takes a freed id starts full.
func dropRemovedItemCharges(ctx context.Context, q *charactersdb.Queries, characterID string, full *charactersv1.FullSheet) error {
	if full == nil {
		return nil
	}
	keep := make([]string, 0, len(full.GetMagicItems()))
	for _, it := range full.GetMagicItems() {
		keep = append(keep, it.GetId())
	}
	if err := q.DeleteItemChargesExcept(ctx, charactersdb.DeleteItemChargesExceptParams{CharacterID: characterID, KeepIds: keep}); err != nil {
		return wrap("drop the charges of removed items", err)
	}
	return nil
}

// itemLossOf is the enum of the loss a row stores.
func itemLossOf(lost string) charactersv1.ItemLoss {
	switch lost {
	case rules.LostGone:
		return charactersv1.ItemLoss_ITEM_LOSS_GONE
	case rules.LostPlain:
		return charactersv1.ItemLoss_ITEM_LOSS_PLAIN_ITEM
	case rules.LostPartly:
		return charactersv1.ItemLoss_ITEM_LOSS_PARTLY_MAGIC
	}
	return charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED
}

// itemRechargeOf is the catalog's recharge rules of an item, which the sheet draws its lines from.
func itemRechargeOf(ic rules.ItemCharges) *charactersv1.ItemRecharge {
	out := &charactersv1.ItemRecharge{
		WaitDawns:      ic.WaitDawns,
		RegainDice:     ic.Regain.Text(),
		StartDice:      ic.Start.Text(),
		OnLoss:         itemLossOf(ic.Lost),
		RegainOnTwenty: ic.EmptyHighRegain.Text(),
	}
	switch ic.Recharge {
	case rules.RechargeDawn:
		out.Kind = charactersv1.ItemRechargeKind_ITEM_RECHARGE_KIND_DAWN
	case rules.RechargeDawnsAfterEmpty:
		out.Kind = charactersv1.ItemRechargeKind_ITEM_RECHARGE_KIND_DAWNS_AFTER_EMPTY
	}
	switch ic.Empty {
	case rules.ChargesEmptyD20:
		out.Empty = charactersv1.ItemEmptyRule_ITEM_EMPTY_RULE_D20
	case rules.ChargesEmptyAlways:
		out.Empty = charactersv1.ItemEmptyRule_ITEM_EMPTY_RULE_ALWAYS
	}
	return out
}
