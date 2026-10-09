package characters

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// scrollCheckBase is the DC of a scroll's ability check before the spell's level (SRD 5.1
// "Spell Scroll": 10 + the spell's level).
const scrollCheckBase = 10

// maxConvertedTexts is how many replaced free-text lines an inventory keeps.
const maxConvertedTexts = 20

// SetCoins implements charactersv1connect.InventoryServiceHandler.
func (s *Service) SetCoins(
	ctx context.Context,
	req *connect.Request[charactersv1.SetCoinsRequest],
) (*connect.Response[charactersv1.SetCoinsResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	coins := req.Msg.GetCoins()
	if coins == nil {
		return nil, invalidArgument(fieldErr("coins", "is required"))
	}
	for name, v := range coinFields(coins) {
		if v < 0 || v > maxCoins {
			return nil, invalidArgument(fieldErr("coins."+name, "must be 0 to %d", maxCoins))
		}
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		if b := t.notDead(); b != nil {
			return b.err()
		}
		cur := t.doc.full.GetCoins()
		if cur != nil && coinFields(cur)["copper"] == coins.Copper && coinFields(cur)["silver"] == coins.Silver && coinFields(cur)["electrum"] == coins.Electrum &&
			coinFields(cur)["gold"] == coins.Gold && coinFields(cur)["platinum"] == coins.Platinum {
			return nil
		}
		t.doc.full.Coins = &charactersv1.Coins{Copper: coins.Copper, Silver: coins.Silver, Electrum: coins.Electrum, Gold: coins.Gold, Platinum: coins.Platinum}
		t.touch()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the coins", err)
	}
	return connect.NewResponse(&charactersv1.SetCoinsResponse{Inventory: view}), nil
}

// d20For rolls the d20 of a scroll's check as the campaign's dice setting asks of the caller.
func (s *Service) d20For(ctx context.Context, tx pgx.Tx, m authz.Membership, req *charactersv1.UseScrollRequest) (int, error) {
	switch roll := req.GetRoll().(type) {
	case *charactersv1.UseScrollRequest_RollInApp:
		if !roll.RollInApp {
			return 0, invalidArgument(fieldErr("roll_in_app", "must be true"))
		}
		if !isMaster(m) {
			if rule, err := s.diceRule(ctx, tx, m); err != nil {
				return 0, err
			} else if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL {
				return 0, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_PHYSICAL, "")
			}
		}
		res, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: 20})
		if err != nil {
			return 0, wrap("roll the d20", err)
		}
		return res.Total, nil
	case *charactersv1.UseScrollRequest_D20Face:
		if !isMaster(m) {
			if rule, err := s.diceRule(ctx, tx, m); err != nil {
				return 0, err
			} else if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_IN_APP {
				return 0, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_IN_APP, "")
			}
		}
		if roll.D20Face < 1 || roll.D20Face > 20 {
			return 0, invalidArgument(fieldErr("d20_face", "must be 1 to 20"))
		}
		return int(roll.D20Face), nil
	}
	return 0, invalidArgument(fieldErr("roll", "set roll_in_app or d20_face"))
}

// UseScroll implements charactersv1connect.InventoryServiceHandler.
func (s *Service) UseScroll(
	ctx context.Context,
	req *connect.Request[charactersv1.UseScrollRequest],
) (*connect.Response[charactersv1.UseScrollResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	resp := &charactersv1.UseScrollResponse{}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		*resp = charactersv1.UseScrollResponse{}
		if b := t.notDead(); b != nil {
			return b.err()
		}
		it, err := t.itemOf(t.doc, req.Msg.GetItemId())
		if err != nil {
			return err
		}
		tr := t.rl.traits(it)
		switch {
		case t.rl.inCombat:
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_USE_IN_COMBAT, it.GetId()).err()
		case !t.knownToCaller(it) || tr.Def == nil || tr.Def.Use == nil || !tr.Def.Use.Scroll || it.GetScrollSpellKey() == "":
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_USABLE, it.GetId()).err()
		}
		return s.readScroll(t, it, tr.Def.Use, req.Msg, resp)
	})
	if err != nil {
		return nil, s.dbError(ctx, "read a scroll", err)
	}
	resp.Inventory = view
	return connect.NewResponse(resp), nil
}

// readScroll follows SRD 5.1 "Spell Scroll": the spell must be on the reader's class list;
// above the highest slot it takes an ability check, DC 10 + the spell's level; the scroll
// is spent when the spell is cast or lost.
func (s *Service) readScroll(t *invTx, it *charactersv1.InventoryItem, use *rules.ItemUse, req *charactersv1.UseScrollRequest, resp *charactersv1.UseScrollResponse) error {
	spell := it.GetScrollSpellKey()
	d := rules.Derive(buildOf(t.doc.full), t.content)
	readable, tooHigh, ability, mod := t.content.ScrollReader(d, spell)
	if !readable {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_SCROLL_UNREADABLE, it.GetId()).err()
	}
	resp.SpellKey, resp.SpellNamePt, resp.SpellLevel = spell, t.content.NamePT(spell), int32(use.ScrollLevel)
	resp.SaveDc, resp.AttackBonus = int32(use.ScrollDC), int32(use.ScrollAttack)
	resp.Cast = true
	if tooHigh {
		face, err := s.d20For(t.ctx, t.tx, t.m, req)
		if err != nil {
			return err
		}
		dc := scrollCheckBase + use.ScrollLevel
		check := &charactersv1.AbilityCheck{D20: int32(face), Modifier: int32(mod), Total: int32(face + mod), Dc: int32(dc), Ability: string(ability)}
		check.Passed = int(check.Total) >= dc
		resp.AbilityCheck, resp.Cast = check, check.Passed
	}
	t.doc.inv.Items = slices.DeleteFunc(t.doc.inv.Items, func(o *charactersv1.InventoryItem) bool { return o == it && o.GetQuantity() <= 1 })
	if it.GetQuantity() > 1 {
		it.Quantity--
	}
	t.event("item_used", map[string]any{
		"character_id": t.doc.row.ID, "item_id": it.GetId(), "item_key": it.GetCatalogKey(), "unidentified": it.GetUnidentified(),
		"amount": use.ScrollLevel,
	})
	t.touch()
	return nil
}

// ammunitionRecoverable is half of what was spent, rounded down: SRD 5.1 "Ammunition" says
// half and not how to round, so the app rounds down.
func ammunitionRecoverable(spent int32) int32 { return max(spent, 0) / 2 }

// RecoverAmmunition implements charactersv1connect.InventoryServiceHandler.
func (s *Service) RecoverAmmunition(
	ctx context.Context,
	req *connect.Request[charactersv1.RecoverAmmunitionRequest],
) (*connect.Response[charactersv1.RecoverAmmunitionResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetCount() < 1 || req.Msg.GetCount() > maxItemQuantity {
		return nil, invalidArgument(fieldErr("count", "must be 1 to %d", maxItemQuantity))
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		if b := t.notDead(); b != nil {
			return b.err()
		}
		it, err := t.itemOf(t.doc, req.Msg.GetItemId())
		if err != nil {
			return err
		}
		if t.rl.inCombat {
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_IN_COMBAT, it.GetId()).err()
		}
		most := ammunitionRecoverable(it.GetAmmunitionSpent())
		if most == 0 {
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOTHING_SPENT, it.GetId()).err()
		}
		if req.Msg.GetCount() > most {
			return (&itemBlock{reason: charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH, itemID: it.GetId(), have: most}).err()
		}
		if it.GetQuantity()+req.Msg.GetCount() > maxItemQuantity {
			return invalidArgument(fieldErr("count", "a line holds at most %d", maxItemQuantity))
		}
		it.Quantity += req.Msg.GetCount()
		it.AmmunitionSpent = 0 // one search per battle
		t.touch()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "recover ammunition", err)
	}
	return connect.NewResponse(&charactersv1.RecoverAmmunitionResponse{Inventory: view}), nil
}

// PreviewTextToItems implements charactersv1connect.InventoryServiceHandler.
func (s *Service) PreviewTextToItems(
	ctx context.Context,
	req *connect.Request[charactersv1.PreviewTextToItemsRequest],
) (*connect.Response[charactersv1.PreviewTextToItemsResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	var resp *charactersv1.PreviewTextToItemsResponse
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		row, err := s.queries.WithTx(tx).GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
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
		if !isMaster(m) && (row.PlayerUserID == nil || *row.PlayerUserID != m.UserID) {
			return errCharacterNotFound()
		}
		it, _ := findItem(d.inv, req.Msg.GetItemId())
		if it == nil || it.GetCatalogKey() != "" {
			return invalidArgument(fieldErr("item_id", "is not a free-text line of this inventory"))
		}
		resp = &charactersv1.PreviewTextToItemsResponse{Text: it.GetName()}
		for _, p := range content.ParseEquipmentText(it.GetName()) {
			resp.Proposals = append(resp.Proposals, &charactersv1.TextProposal{
				Source: p.Source, CatalogKey: p.Key, Name: p.Name, Quantity: int32(p.Quantity), QuantityGuessed: p.Guessed,
			})
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "read a free-text line", err)
	}
	return connect.NewResponse(resp), nil
}

// ConfirmTextToItems implements charactersv1connect.InventoryServiceHandler.
func (s *Service) ConfirmTextToItems(
	ctx context.Context,
	req *connect.Request[charactersv1.ConfirmTextToItemsRequest],
) (*connect.Response[charactersv1.ConfirmTextToItemsResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if n := len(req.Msg.GetLines()); n < 1 || n > rules.MaxTextPieces {
		return nil, invalidArgument(fieldErr("lines", "must have 1 to %d entries", rules.MaxTextPieces))
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg, keyRequired: true}, func(t *invTx) error {
		if !t.mayAdd() {
			return errCharacterNotFound()
		}
		if b := t.notDead(); b != nil {
			return b.err()
		}
		src, err := t.itemOf(t.doc, req.Msg.GetItemId())
		if err != nil {
			return err
		}
		if src.GetCatalogKey() != "" {
			return invalidArgument(fieldErr("item_id", "is not a free-text line"))
		}
		t.doc.inv.Items = slices.DeleteFunc(t.doc.inv.Items, func(o *charactersv1.InventoryItem) bool { return o == src })
		for i, l := range req.Msg.GetLines() {
			g := &charactersv1.ItemGrant{CatalogKey: l.GetCatalogKey(), Quantity: l.GetQuantity()}
			if l.GetCatalogKey() == "" {
				name, err := names.Clean(l.GetName(), maxItemNameLength)
				if err != nil {
					return invalidArgument(&fieldError{field: lineField(i, "name"), err: err})
				}
				g.Name = name
			} else if !isEquipmentKey(l.GetCatalogKey()) {
				return invalidArgument(fieldErr(lineField(i, "catalog_key"), "must be SRD equipment or empty"))
			}
			n, err := t.rl.newItem(lineField(i, ""), g, grantRules{master: t.master})
			if err != nil {
				return invalidArgument(err)
			}
			if _, err := t.rl.stackInto(t.doc.inv, n); err != nil {
				var full *fullError
				if errors.As(err, &full) {
					return full.block().err()
				}
				return invalidArgument(err)
			}
		}
		t.doc.inv.ConvertedTexts = append(t.doc.inv.ConvertedTexts, src.GetName())
		if over := len(t.doc.inv.ConvertedTexts) - maxConvertedTexts; over > 0 {
			t.doc.inv.ConvertedTexts = slices.Delete(t.doc.inv.ConvertedTexts, 0, over)
		}
		t.touch()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "turn a text into items", err)
	}
	return connect.NewResponse(&charactersv1.ConfirmTextToItemsResponse{Inventory: view}), nil
}

func lineField(i int, name string) string {
	f := fmt.Sprintf("lines[%d]", i)
	if name != "" {
		f += "." + name
	}
	return f
}

func isEquipmentKey(k string) bool {
	return len(k) > len("equipment:") && k[:len("equipment:")] == "equipment:"
}

// ListItemRests implements charactersv1connect.InventoryServiceHandler.
func (s *Service) ListItemRests(
	ctx context.Context,
	req *connect.Request[charactersv1.ListItemRestsRequest],
) (*connect.Response[charactersv1.ListItemRestsResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	resp := &charactersv1.ListItemRestsResponse{}
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := s.queries.WithTx(tx).ListCampaignSheets(ctx, m.CampaignID)
		if err != nil {
			return wrap("list the campaign's sheets", err)
		}
		inCombat := false
		if s.creatureHost != nil {
			if inCombat, err = s.creatureHost.CampaignInCombat(ctx, tx, m.CampaignID); err != nil {
				return wrap("find out whether a combat is going on", err)
			}
		}
		for _, r := range rows {
			d, err := inventoryDocOf(charactersdb.Character{ID: r.ID, Name: r.Name, Sheet: r.Sheet})
			if err != nil {
				continue
			}
			rl := invRules{content: content, who: rules.CharacterOf(buildOf(d.full), content), inCombat: inCombat}
			now := rl.attunements(d.inv)
			for _, it := range d.inv.GetItems() {
				kind := it.GetRequest()
				if kind == charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_UNSPECIFIED {
					continue
				}
				p := &charactersv1.PendingItemRest{
					CharacterId: r.ID, CharacterName: r.Name, ItemId: it.GetId(), Kind: kind,
					ItemNamePt: content.ItemName(ruleItem(it)), AttunedNow: int32(now), AttunedAfter: int32(now),
				}
				if m, ok := content.MagicItem(it.GetCatalogKey()); ok {
					p.Rarity = m.Rarity
				}
				var b *itemBlock
				switch kind {
				case charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_ATTUNE:
					p.AttunedAfter++
					b = rl.attune(d.inv, it, false)
				case charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_END_ATTUNEMENT:
					p.AttunedAfter--
					b = rl.endAttunement(it, false)
				default:
					b = rl.identify(it)
				}
				if b != nil {
					p.Blocked = b.reason
				}
				resp.Pending = append(resp.Pending, p)
			}
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "list the items marked for the short rest", err)
	}
	return connect.NewResponse(resp), nil
}
