package characters

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// maxGrants is how many things one GiveItems gives.
const maxGrants = 20

// readInventory runs fn on a character's inventory without changing it.
func (s *Service) readInventory(ctx context.Context, m authz.Membership, characterID string, content *rules.Content) (*charactersv1.InventoryView, error) {
	id, ok := parseUUID(characterID)
	if !ok {
		return nil, errCharacterNotFound()
	}
	var view *charactersv1.InventoryView
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		row, err := q.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !canSee(m, row.Kind, row.Status, row.PlayerUserID)) {
			return errCharacterNotFound()
		}
		if err != nil {
			return wrap("read a character", err)
		}
		d, err := inventoryDocOf(row)
		if err != nil {
			return err
		}
		t, err := s.newInvTx(ctx, tx, q, m, content, d)
		if err != nil {
			return err
		}
		view = t.view(d)
		return nil
	})
	return view, err
}

// GetInventory implements charactersv1connect.InventoryServiceHandler.
func (s *Service) GetInventory(
	ctx context.Context,
	req *connect.Request[charactersv1.GetInventoryRequest],
) (*connect.Response[charactersv1.GetInventoryResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	view, err := s.readInventory(ctx, m, req.Msg.GetCharacterId(), content)
	if err != nil {
		return nil, s.dbError(ctx, "read an inventory", err)
	}
	return connect.NewResponse(&charactersv1.GetInventoryResponse{Inventory: view}), nil
}

// ListItemCatalog implements charactersv1connect.InventoryServiceHandler.
func (s *Service) ListItemCatalog(
	ctx context.Context,
	req *connect.Request[charactersv1.ListItemCatalogRequest],
) (*connect.Response[charactersv1.ListItemCatalogResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	out := &charactersv1.ListItemCatalogResponse{}
	for _, e := range content.EquipmentList() {
		out.Items = append(out.Items, &charactersv1.CatalogItem{
			Key: e.Key, Kind: charactersv1.ItemKind_ITEM_KIND_EQUIPMENT, NamePt: e.NamePT, Name: e.Name,
			Category: content.CategoryNamePT("equipment", e.Kind), Stackable: true, PackQuantity: int32(e.PackQuantity), ScrollLevel: -1,
		})
	}
	if isMaster(m) {
		for _, e := range content.MagicItems() {
			out.Items = append(out.Items, catalogMagic(content, e))
		}
	}
	return connect.NewResponse(out), nil
}

func catalogMagic(content *rules.Content, e rules.MagicItemEntry) *charactersv1.CatalogItem {
	t := content.Traits(rules.Item{Key: e.Key})
	c := &charactersv1.CatalogItem{
		Key: e.Key, Kind: charactersv1.ItemKind_ITEM_KIND_MAGIC, NamePt: e.NamePT, Name: e.Name,
		Category: content.CategoryNamePT("item", e.Category), Rarity: e.Rarity,
		RequiresAttunement: e.Attunement, AttunementByPt: e.AttunementByPT,
		IsFamily: e.IsFamily() && !e.Standalone, Variants: slices.Clone(e.Variants),
		Coverage: coverageToProto[content.ItemCoverage(e.Key)], Stackable: t.Stackable, ScrollLevel: -1,
	}
	if d := t.Def; d != nil {
		c.BaseChoices = slices.Clone(d.BaseChoices)
		c.OptionDamageTypes = slices.Clone(d.OptionDamageTypes)
		if d.Use != nil && d.Use.Scroll {
			c.ScrollLevel = int32(d.Use.ScrollLevel)
		}
	}
	return c
}

// GiveItems implements charactersv1connect.InventoryServiceHandler.
func (s *Service) GiveItems(
	ctx context.Context,
	req *connect.Request[charactersv1.GiveItemsRequest],
) (*connect.Response[charactersv1.GiveItemsResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if n := len(req.Msg.GetGrants()); n < 1 || n > maxGrants {
		return nil, invalidArgument(fieldErr("grants", "must have 1 to %d entries", maxGrants))
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		if !t.mayAdd() {
			return errCharacterNotFound()
		}
		if b := t.notDead(); b != nil {
			return b.err()
		}
		gr := grantRules{master: t.master}
		for i, g := range req.Msg.GetGrants() {
			field := fmt.Sprintf("grants[%d]", i)
			n, err := t.rl.newItem(field, g, gr)
			if err != nil {
				return invalidArgument(err)
			}
			wantEquipped := n.Equipped
			n.Equipped = false
			line, err := t.rl.stackInto(t.doc.inv, n)
			if err != nil {
				var full *fullError
				if errors.As(err, &full) {
					return full.block().err()
				}
				return invalidArgument(err)
			}
			if wantEquipped {
				if b := t.rl.equip(t.doc.inv, line, t.master); b != nil {
					return b.err()
				}
				line.Equipped = true
			}
			t.event("item_given", map[string]any{
				"character_id": t.doc.row.ID, "item_id": line.GetId(), "item_key": n.GetCatalogKey(),
				"quantity": n.GetQuantity(), "unidentified": n.GetUnidentified(),
			})
		}
		t.touch()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "give items", err)
	}
	return connect.NewResponse(&charactersv1.GiveItemsResponse{Inventory: view}), nil
}

// SetEquipped implements charactersv1connect.InventoryServiceHandler.
func (s *Service) SetEquipped(
	ctx context.Context,
	req *connect.Request[charactersv1.SetEquippedRequest],
) (*connect.Response[charactersv1.SetEquippedResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg}, func(t *invTx) error {
		if b := t.notDead(); b != nil {
			return b.err()
		}
		it, err := t.itemOf(t.doc, req.Msg.GetItemId())
		if err != nil {
			return err
		}
		if it.GetEquipped() == req.Msg.GetEquipped() {
			return nil
		}
		if req.Msg.GetEquipped() {
			if b := t.rl.equip(t.doc.inv, it, t.master); b != nil {
				return b.err()
			}
		} else if b := t.rl.unequip(it, t.master); b != nil {
			return b.err()
		}
		it.Equipped = req.Msg.GetEquipped()
		t.touch()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "wear an item", err)
	}
	return connect.NewResponse(&charactersv1.SetEquippedResponse{Inventory: view}), nil
}

// RemoveItem implements charactersv1connect.InventoryServiceHandler.
func (s *Service) RemoveItem(
	ctx context.Context,
	req *connect.Request[charactersv1.RemoveItemRequest],
) (*connect.Response[charactersv1.RemoveItemResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetQuantity() < 0 || req.Msg.GetQuantity() > maxItemQuantity {
		return nil, invalidArgument(fieldErr("quantity", "must be 0 to %d", maxItemQuantity))
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		it, err := t.itemOf(t.doc, req.Msg.GetItemId())
		if err != nil {
			return err
		}
		if !t.mayRemove(it) {
			return errPermission("only the master takes a magic item away")
		}
		n := req.Msg.GetQuantity()
		if n == 0 || n >= it.GetQuantity() {
			if n > it.GetQuantity() {
				return (&itemBlock{reason: charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH, itemID: it.GetId(), have: it.GetQuantity()}).err()
			}
			n = it.GetQuantity()
			t.doc.inv.Items = slices.DeleteFunc(t.doc.inv.Items, func(o *charactersv1.InventoryItem) bool { return o == it })
		} else {
			it.Quantity -= n
		}
		t.touch()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "remove an item", err)
	}
	return connect.NewResponse(&charactersv1.RemoveItemResponse{Inventory: view}), nil
}

// AdjustCoins implements charactersv1connect.InventoryServiceHandler.
func (s *Service) AdjustCoins(
	ctx context.Context,
	req *connect.Request[charactersv1.AdjustCoinsRequest],
) (*connect.Response[charactersv1.AdjustCoinsResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	delta := req.Msg.GetDelta()
	if delta == nil {
		return nil, invalidArgument(fieldErr("delta", "is required"))
	}
	for name, v := range coinFields(delta) {
		if v < -maxCoins || v > maxCoins {
			return nil, invalidArgument(fieldErr("delta."+name, "must be -%d to %d", maxCoins, maxCoins))
		}
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		if b := t.notDead(); b != nil {
			return b.err()
		}
		if t.doc.full.Coins == nil {
			t.doc.full.Coins = &charactersv1.Coins{}
		}
		purse := t.doc.full.Coins
		have := coinFields(purse)
		next := map[string]int32{}
		for name, d := range coinFields(delta) {
			n := have[name] + d
			if n < 0 {
				return (&itemBlock{reason: charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH, have: have[name]}).err()
			}
			if n > maxCoins {
				return invalidArgument(fieldErr("delta."+name, "would pass %d", maxCoins))
			}
			next[name] = n
		}
		purse.Copper, purse.Silver, purse.Electrum, purse.Gold, purse.Platinum = next["copper"], next["silver"], next["electrum"], next["gold"], next["platinum"]
		t.touch()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "adjust the coins", err)
	}
	return connect.NewResponse(&charactersv1.AdjustCoinsResponse{Inventory: view}), nil
}

// coinFields lists the coins of a purse by name.
func coinFields(c *charactersv1.Coins) map[string]int32 {
	return map[string]int32{"copper": c.GetCopper(), "silver": c.GetSilver(), "electrum": c.GetElectrum(), "gold": c.GetGold(), "platinum": c.GetPlatinum()}
}

// UpdateItem implements charactersv1connect.InventoryServiceHandler.
func (s *Service) UpdateItem(
	ctx context.Context,
	req *connect.Request[charactersv1.UpdateItemRequest],
) (*connect.Response[charactersv1.UpdateItemResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		if err := t.requireMaster(); err != nil {
			return err
		}
		it, err := t.itemOf(t.doc, req.Msg.GetItemId())
		if err != nil {
			return err
		}
		return applyItemUpdate(t, it, req.Msg)
	})
	if err != nil {
		return nil, s.dbError(ctx, "update an item", err)
	}
	return connect.NewResponse(&charactersv1.UpdateItemResponse{Inventory: view}), nil
}

// ListItemLog implements charactersv1connect.InventoryServiceHandler.
func (s *Service) ListItemLog(
	ctx context.Context,
	req *connect.Request[charactersv1.ListItemLogRequest],
) (*connect.Response[charactersv1.ListItemLogResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	return s.itemLog(ctx, m, req.Msg)
}

// diceFor rolls or checks the potion's dice as the campaign's setting asks of the caller.
func (s *Service) diceFor(ctx context.Context, tx pgx.Tx, m authz.Membership, expr dice.Expr, req *charactersv1.UseItemRequest) (dice.Result, error) {
	switch roll := req.GetRoll().(type) {
	case *charactersv1.UseItemRequest_RollInApp:
		if !roll.RollInApp {
			return dice.Result{}, invalidArgument(fieldErr("roll_in_app", "must be true"))
		}
		if !isMaster(m) {
			if rule, err := s.diceRule(ctx, tx, m); err != nil {
				return dice.Result{}, err
			} else if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL {
				return dice.Result{}, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_PHYSICAL, "")
			}
		}
		res, err := dice.Roll(s.roller, expr)
		if err != nil {
			return dice.Result{}, wrap("roll the potion", err)
		}
		return res, nil
	case *charactersv1.UseItemRequest_TypedSum:
		if !isMaster(m) {
			if rule, err := s.diceRule(ctx, tx, m); err != nil {
				return dice.Result{}, err
			} else if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_IN_APP {
				return dice.Result{}, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_IN_APP, "")
			}
		}
		res, err := dice.Physical(expr, int(roll.TypedSum)-expr.Modifier)
		if err != nil {
			return dice.Result{}, invalidArgument(fieldErr("typed_sum", "must be %d to %d", expr.Count, expr.Count*expr.Sides))
		}
		return res, nil
	}
	return dice.Result{}, invalidArgument(fieldErr("roll", "set roll_in_app or typed_sum"))
}

// UseItem implements charactersv1connect.InventoryServiceHandler.
func (s *Service) UseItem(
	ctx context.Context,
	req *connect.Request[charactersv1.UseItemRequest],
) (*connect.Response[charactersv1.UseItemResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	view, t, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		t.outcome = nil
		return s.useItem(t, req.Msg)
	})
	if err != nil {
		return nil, s.dbError(ctx, "use an item", err)
	}
	resp := &charactersv1.UseItemResponse{Inventory: view}
	if t != nil {
		resp.Outcome = t.outcome
	}
	return connect.NewResponse(resp), nil
}

// useItem drinks a potion.
func (s *Service) useItem(t *invTx, req *charactersv1.UseItemRequest) error {
	if b := t.notDead(); b != nil {
		return b.err()
	}
	it, err := t.itemOf(t.doc, req.GetItemId())
	if err != nil {
		return err
	}
	tr := t.rl.traits(it)
	switch {
	case t.rl.inCombat:
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_USE_IN_COMBAT, it.GetId()).err()
	case !t.knownToCaller(it) || !tr.Usable || tr.Def == nil || tr.Def.Use == nil:
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_USABLE, it.GetId()).err()
	case tr.Def.Use.Scroll:
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_CAST_IT, it.GetId()).err()
	}
	outcome, err := s.drink(t, it, tr.Def.Use, req)
	if err != nil {
		return err
	}
	if it.GetQuantity() <= 1 {
		t.doc.inv.Items = slices.DeleteFunc(t.doc.inv.Items, func(o *charactersv1.InventoryItem) bool { return o == it })
		outcome.Consumed = true
	} else {
		it.Quantity--
	}
	t.outcome = outcome
	t.touch()
	return nil
}

// drink applies a potion to the drinker, or to the character it is given to.
func (s *Service) drink(t *invTx, it *charactersv1.InventoryItem, use *rules.ItemUse, req *charactersv1.UseItemRequest) (*charactersv1.UseOutcome, error) {
	targetID := t.doc.row.ID
	if req.GetTargetCharacterId() != "" {
		id, ok := parseUUID(req.GetTargetCharacterId())
		if !ok {
			return nil, errCharacterNotFound()
		}
		row, err := t.q.GetCharacter(t.ctx, charactersdb.GetCharacterParams{CampaignID: t.m.CampaignID, ID: id})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, invalidArgument(fieldErr("target_character_id", "is not a character of the campaign"))
			}
			return nil, wrap("read the character the potion goes to", err)
		}
		if row.Status == statusDead || row.Status == statusPending || (row.Kind != kindPlayer && !t.master) {
			return nil, block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_CANNOT_RECEIVE, "").err()
		}
		targetID = row.ID
	}
	before, err := s.getVitals(t.ctx, t.tx, t.m.CampaignID, targetID)
	if err != nil {
		return nil, err
	}
	out := &charactersv1.UseOutcome{}
	hp, temp := before.GetHitPointsCurrent(), before.GetHitPointsTemporary()
	if use.HealDice != "" {
		expr, err := dice.Parse(use.HealDice)
		if err != nil {
			return nil, wrap("read the potion's dice", err)
		}
		res, err := s.diceFor(t.ctx, t.tx, t.m, expr, req)
		if err != nil {
			return nil, err
		}
		out.Dice, out.Healed = use.HealDice, int32(res.Total)
		hp = min(before.GetHitPointsMax(), hp+out.Healed)
	}
	if use.TempHP > 0 {
		temp = max(temp, int32(use.TempHP)) // temporary hit points do not add up (SRD 5.1 "Temporary Hit Points")
		out.TempHitPointsGiven = int32(use.TempHP)
	}
	adjust := &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp, HitPointsTemporary: &temp}
	_, after, err := s.AdjustVitals(t.ctx, t.tx, t.m.CampaignID, targetID, adjust)
	if err != nil {
		return nil, err
	}
	out.HitPoints, out.TempHitPoints = after.GetHitPointsCurrent(), after.GetHitPointsTemporary()
	t.event("item_used", map[string]any{
		"character_id": t.doc.row.ID, "target_id": targetID, "item_id": it.GetId(), "item_key": it.GetCatalogKey(),
		"unidentified": it.GetUnidentified(), "amount": out.Healed,
	})
	if targetID != t.doc.row.ID {
		t.notify[targetID] = ""
	}
	return out, nil
}

// SpendCharges implements charactersv1connect.InventoryServiceHandler.
func (s *Service) SpendCharges(
	ctx context.Context,
	req *connect.Request[charactersv1.SpendChargesRequest],
) (*connect.Response[charactersv1.SpendChargesResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetCharges() < 1 || req.Msg.GetCharges() > 100 {
		return nil, invalidArgument(fieldErr("charges", "must be 1 to 100"))
	}
	view, t, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		t.d20, t.destroyed = 0, false
		if b := t.notDead(); b != nil {
			return b.err()
		}
		it, err := t.itemOf(t.doc, req.Msg.GetItemId())
		if err != nil {
			return err
		}
		return s.spendCharges(t, it, int(req.Msg.GetCharges()))
	})
	if err != nil {
		return nil, s.dbError(ctx, "spend charges", err)
	}
	resp := &charactersv1.SpendChargesResponse{Inventory: view}
	if t != nil {
		resp.D20, resp.Destroyed = t.d20, t.destroyed
	}
	return connect.NewResponse(resp), nil
}

// spendCharges spends n charges of an item, and rolls for the item when the last one
// goes (SRD 5.1 wands: on a 1 on a d20 it is destroyed).
func (s *Service) spendCharges(t *invTx, it *charactersv1.InventoryItem, n int) error {
	tr := t.rl.traits(it)
	if !t.knownToCaller(it) || tr.Def == nil || tr.Def.Charges == nil {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_USABLE, it.GetId()).err()
	}
	left := tr.Def.Charges.Max - int(it.GetChargesUsed())
	if n > left {
		return (&itemBlock{reason: charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH, itemID: it.GetId(), have: int32(left)}).err()
	}
	it.ChargesUsed += int32(n)
	if n == left && tr.Def.Charges.DestroyOnEmpty {
		res, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: 20})
		if err != nil {
			return wrap("roll the last charge", err)
		}
		t.d20 = int32(res.Total)
		if res.Total == 1 {
			t.destroyed = true
			t.doc.inv.Items = slices.DeleteFunc(t.doc.inv.Items, func(o *charactersv1.InventoryItem) bool { return o == it })
		}
	}
	t.event("item_charges", map[string]any{
		"character_id": t.doc.row.ID, "item_id": it.GetId(), "item_key": it.GetCatalogKey(), "unidentified": it.GetUnidentified(),
		"amount": -n, "destroyed": t.destroyed,
	})
	t.touch()
	return nil
}
