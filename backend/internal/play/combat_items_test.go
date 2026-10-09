package play

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// giveItems gives a character items as the master and returns the inventory the master reads.
func (a *armed) giveItems(t *testing.T, characterID string, grants ...*charactersv1.ItemGrant) *charactersv1.InventoryView {
	t.Helper()
	res, err := a.master.inventory.GiveItems(t.Context(), connect.NewRequest(&charactersv1.GiveItemsRequest{
		CampaignId: a.campaignID, CharacterId: characterID, Grants: grants, IdempotencyKey: newKey(),
	}))
	if err != nil {
		t.Fatalf("GiveItems() error = %v", err)
	}
	return res.Msg.GetInventory()
}

func (a *armed) itemOf(t *testing.T, characterID, key string) *charactersv1.InventoryEntry {
	t.Helper()
	res, err := a.master.inventory.GetInventory(t.Context(), connect.NewRequest(&charactersv1.GetInventoryRequest{CampaignId: a.campaignID, CharacterId: characterID}))
	if err != nil {
		t.Fatalf("GetInventory() error = %v", err)
	}
	for _, e := range res.Msg.GetInventory().GetItems() {
		if e.GetCatalogKey() == key {
			return e
		}
	}
	return nil
}

func (a *armed) equipItem(t *testing.T, u *user, characterID, itemID string) {
	t.Helper()
	if _, err := u.inventory.SetEquipped(t.Context(), connect.NewRequest(&charactersv1.SetEquippedRequest{
		CampaignId: a.campaignID, CharacterId: characterID, ItemId: itemID, Equipped: true,
	})); err != nil {
		t.Fatalf("SetEquipped() error = %v", err)
	}
}

// TestARangedAttackSpendsAPieceOfAmmunition: a bow from the inventory spends one arrow per attack
// and, with none left, refuses to shoot (SRD 5.1 "Ammunition").
func TestARangedAttackSpendsAPieceOfAmmunition(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	v := a.giveItems(t, a.toren.GetId(), &charactersv1.ItemGrant{CatalogKey: "equipment:shortbow"}, &charactersv1.ItemGrant{CatalogKey: "equipment:arrow", Quantity: 1})
	var bow *charactersv1.InventoryEntry
	for _, e := range v.GetItems() {
		if e.GetCatalogKey() == "equipment:shortbow" {
			bow = e
		}
	}
	a.equipItem(t, a.caio, a.toren.GetId(), bow.GetId())
	e := a.threeAndAGoblin(t)
	bowKey := "inv:" + bow.GetId()
	a.h.roller.queue(0)
	res, err := a.attack(t, a.caio, e, "Toren", bowKey, "Goblin", d20(15))
	if err != nil {
		t.Fatalf("shoot the bow: %v", err)
	}
	if res.GetPendingDamage() == nil {
		t.Errorf("the shot did not hit: %v", res)
	}
	if got := a.itemOf(t, a.toren.GetId(), "equipment:arrow"); got.GetQuantity() != 0 || got.GetAmmunitionSpent() != 1 {
		t.Errorf("arrows after the shot = %v, want none left and one counted as spent", got)
	}
	if _, err := a.master.combat.DiscardPendingDamage(t.Context(), connect.NewRequest(&playv1.DiscardPendingDamageRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: res.GetPendingDamage().GetId(), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("DiscardPendingDamage() error = %v", err)
	}
	// His next turn: nothing to shoot.
	e = a.mustEndTurn(t, a.caio, e)
	e = a.mustEndTurn(t, a.ana, e)
	e = a.mustEndTurn(t, a.master, e)
	e = a.mustEndTurn(t, a.bia, e)
	_, err = a.attack(t, a.caio, e, "Toren", bowKey, "Goblin", d20(15))
	wantBlockedBy(t, "a shot with no arrows", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_AMMUNITION)
}

// TestADrunkPotionHealsAndSpendsTheAction: drinking is the action of the turn (SRD 5.1 "Potions").
func TestADrunkPotionHealsAndSpendsTheAction(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	v := a.giveItems(t, a.toren.GetId(), &charactersv1.ItemGrant{CatalogKey: "item:potion-of-healing-common", Quantity: 2})
	potion := v.GetItems()[0]
	for _, e := range v.GetItems() {
		if e.GetCatalogKey() == "item:potion-of-healing-common" {
			potion = e
		}
	}
	e := a.threeAndAGoblin(t)
	if _, err := a.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: a.campaignID, CharacterId: a.toren.GetId(), IdempotencyKey: newKey(), HitPointsCurrent: proto.Int32(4),
	})); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	a.h.roller.queue(3, 4)
	drink := func(u *user) (*playv1.UseItemResponse, error) {
		res, err := u.combat.UseItem(t.Context(), connect.NewRequest(&playv1.UseItemRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), ItemId: potion.GetId(),
			Use: playv1.CombatItemUse_COMBAT_ITEM_USE_DRINK, IdempotencyKey: newKey(), Roll: &playv1.UseItemRequest_RollInApp{RollInApp: true},
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	// Another player cannot drink Toren's potion.
	if _, err := drink(a.ana); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Pensantus's player drinking Toren's potion: %v, want permission_denied", err)
	}
	res, err := drink(a.caio)
	if err != nil {
		t.Fatalf("drink: %v", err)
	}
	if res.GetHealed() != 9 || !byLabel(t, res.GetEncounter(), "Toren").GetActionUsed() {
		t.Errorf("drink = healed %d, action used %v; want 9 healed (2d4+2) and the action spent", res.GetHealed(), byLabel(t, res.GetEncounter(), "Toren").GetActionUsed())
	}
	if got := a.itemOf(t, a.toren.GetId(), "item:potion-of-healing-common"); got.GetQuantity() != 1 {
		t.Errorf("potions left = %v, want 1", got)
	}
	// The action is gone: a second potion waits for the next turn.
	_, err = drink(a.caio)
	wantBlockedBy(t, "a second potion in the turn", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED)
}
