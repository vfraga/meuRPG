package characters

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// itemHost is the play module for the inventory tests: whether a combat is going on, the
// history it was asked to keep and the hints it was asked to publish.
type itemHost struct {
	mu        sync.Mutex
	combat    bool
	events    []link.ItemEvent
	published []string
}

func (*itemHost) CreaturesLeaving(context.Context, pgx.Tx, string, string, []string, time.Time) (string, error) {
	return "", nil
}
func (*itemHost) LockSession(context.Context, pgx.Tx, string) error { return nil }
func (*itemHost) CreatureRenamed(context.Context, pgx.Tx, string, string, string) (string, error) {
	return "", nil
}

func (*itemHost) CreatureInCombat(context.Context, pgx.Tx, string, string) (bool, error) {
	return false, nil
}

func (h *itemHost) AppendEvent(_ context.Context, _ pgx.Tx, _, kind, actor string, payload []byte, at time.Time) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append([]link.ItemEvent{{Kind: kind, ActorUserID: actor, Payload: payload, At: at}}, h.events...)
	return true, nil
}
func (*itemHost) PublishCreaturesChanged(string, string)                  {}
func (*itemHost) PublishEncounterChanged(context.Context, string, string) {}
func (h *itemHost) CampaignInCombat(context.Context, pgx.Tx, string) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.combat, nil
}

func (h *itemHost) PublishInventoryChanged(_, characterID, _ string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.published = append(h.published, characterID)
}

func (h *itemHost) ItemEvents(context.Context, string, int) ([]link.ItemEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.events), nil
}

func (h *itemHost) setCombat(on bool) { h.mu.Lock(); h.combat = on; h.mu.Unlock() }

// invTable is a campaign with a master, two players with Pensantus each and the host.
type invTable struct {
	h            *harness
	host         *itemHost
	master, ana  *user
	bruno        *user
	campaign     string
	pensA, pensB *charactersv1.Character
	faces        *dice.Fixed
}

func newInvTable(t *testing.T, faces ...int) *invTable {
	t.Helper()
	f := &dice.Fixed{Faces: faces}
	h := newHarnessWith(t, func(c *Config) { c.Roller = f })
	host := &itemHost{}
	h.svc.SetCreatureHost(host)
	tb := &invTable{h: h, host: host, faces: f}
	tb.master, tb.ana, tb.bruno = h.newUser("Mestre"), h.newUser("Ana"), h.newUser("Bruno")
	tb.campaign = h.newCampaign(tb.master, "Mirathel", tb.ana, tb.bruno)
	tb.pensA = tb.ana.createPensantus(t, tb.campaign)
	tb.pensB = tb.bruno.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Brisa", pensantusSheet())
	return tb
}

var keySeq int

func nextKey() string {
	keySeq++
	return "k" + strings.Repeat("0", 3) + string(rune('a'+keySeq%26)) + time.Now().Format("150405.000000") + string(rune('a'+(keySeq/26)%26))
}

func (tb *invTable) give(t *testing.T, u *user, characterID string, grants ...*charactersv1.ItemGrant) *charactersv1.InventoryView {
	t.Helper()
	res, err := u.inventory.GiveItems(t.Context(), connect.NewRequest(&charactersv1.GiveItemsRequest{
		CampaignId: tb.campaign, CharacterId: characterID, Grants: grants, IdempotencyKey: nextKey(),
	}))
	if err != nil {
		t.Fatalf("GiveItems(%v) error = %v", grants, err)
	}
	return res.Msg.GetInventory()
}

func (tb *invTable) view(t *testing.T, u *user, characterID string) *charactersv1.InventoryView {
	t.Helper()
	res, err := u.inventory.GetInventory(t.Context(), connect.NewRequest(&charactersv1.GetInventoryRequest{CampaignId: tb.campaign, CharacterId: characterID}))
	if err != nil {
		t.Fatalf("GetInventory() error = %v", err)
	}
	return res.Msg.GetInventory()
}

func entry(v *charactersv1.InventoryView, key string) *charactersv1.InventoryEntry {
	for _, e := range v.GetItems() {
		if e.GetCatalogKey() == key {
			return e
		}
	}
	return nil
}

func named(v *charactersv1.InventoryView, name string) *charactersv1.InventoryEntry {
	for _, e := range v.GetItems() {
		if e.GetNamePt() == name {
			return e
		}
	}
	return nil
}

// refused returns the ItemBlocked reason of a failed_precondition.
func refused(t *testing.T, call string, err error) charactersv1.ItemBlockedReason {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		v, derr := d.Value()
		if derr != nil {
			t.Fatalf("decode error detail: %v", derr)
		}
		if b, ok := v.(*charactersv1.ItemBlocked); ok {
			return b.GetReason()
		}
	}
	t.Fatalf("%s error %v has no ItemBlocked detail", call, err)
	return 0
}

func (tb *invTable) equip(u *user, characterID, itemID string, on bool) error {
	_, err := u.inventory.SetEquipped(context.Background(), connect.NewRequest(&charactersv1.SetEquippedRequest{
		CampaignId: tb.campaign, CharacterId: characterID, ItemId: itemID, Equipped: on,
	}))
	return err
}

func (tb *invTable) request(u *user, characterID, itemID string, kind charactersv1.ItemRequestKind) error {
	_, err := u.inventory.RequestItemRest(context.Background(), connect.NewRequest(&charactersv1.RequestItemRestRequest{
		CampaignId: tb.campaign, CharacterId: characterID, ItemId: itemID, Kind: kind,
	}))
	return err
}

func (tb *invTable) rest(t *testing.T) []*charactersv1.ItemRestDone {
	t.Helper()
	res, err := tb.master.inventory.ConfirmItemRests(t.Context(), connect.NewRequest(&charactersv1.ConfirmItemRestsRequest{CampaignId: tb.campaign, IdempotencyKey: nextKey()}))
	if err != nil {
		t.Fatalf("ConfirmItemRests() error = %v", err)
	}
	return res.Msg.GetDone()
}

func grant(key string) *charactersv1.ItemGrant { return &charactersv1.ItemGrant{CatalogKey: key} }

func (tb *invTable) ac(t *testing.T, id string) int32 {
	t.Helper()
	return tb.master.get(t, tb.campaign, id).GetDerived().GetArmorClass()
}

const (
	attuneKind   = charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_ATTUNE
	endKind      = charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_END_ATTUNEMENT
	identifyKind = charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_IDENTIFY
)

func TestInventoryReadsTheSheetsEquipmentTextWordForWord(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	v := tb.view(t, tb.ana, tb.pensA.GetId())
	if len(v.GetItems()) != 2 || named(v, "Grimório") == nil || named(v, "Corda de cânhamo") == nil {
		t.Fatalf("inventory = %v, want the sheet's two lines", v.GetItems())
	}
	for _, e := range v.GetItems() {
		if e.GetKind() != charactersv1.ItemKind_ITEM_KIND_FREE_TEXT || e.GetQuantity() != 1 {
			t.Errorf("line %v is not a free-text item of one", e)
		}
	}
	// The sheet answers no equipment and no inventory.
	c := tb.ana.get(t, tb.campaign, tb.pensA.GetId())
	if len(c.GetSheet().GetFull().GetEquipment()) != 0 || c.GetSheet().GetFull().GetInventory() != nil {
		t.Errorf("the sheet carries the equipment %v / inventory %v", c.GetSheet().GetFull().GetEquipment(), c.GetSheet().GetFull().GetInventory())
	}
	// A save of the sheet keeps the inventory, and an old editor's lines join it once.
	sheet := c.GetSheet()
	sheet.GetFull().Equipment = []*charactersv1.Item{{Name: "Grimório", Quantity: 1}, {Name: "Lanterna", Quantity: 2}}
	if _, err := tb.master.update(t, tb.master.get(t, tb.campaign, tb.pensA.GetId()), c.GetName(), sheet); err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	v = tb.view(t, tb.ana, tb.pensA.GetId())
	if len(v.GetItems()) != 3 || named(v, "Lanterna") == nil || named(v, "Lanterna").GetQuantity() != 2 {
		t.Errorf("inventory after a save = %v, want the two old lines and the new one, none twice", v.GetItems())
	}
	// What the client sends as inventory is ignored.
	sheet.GetFull().Inventory = &charactersv1.Inventory{Items: []*charactersv1.InventoryItem{{Id: "x", CatalogKey: "item:vorpal-sword", Quantity: 1}}}
	if _, err := tb.master.update(t, tb.master.get(t, tb.campaign, tb.pensA.GetId()), c.GetName(), sheet); err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	if v = tb.view(t, tb.master, tb.pensA.GetId()); entry(v, "item:vorpal-sword") != nil || len(v.GetItems()) != 3 {
		t.Errorf("a client wrote into the inventory: %v", v.GetItems())
	}
}

func TestPlayersGiveThemselvesEquipmentAndTextOnly(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	v := tb.give(t, tb.ana, tb.pensA.GetId(), &charactersv1.ItemGrant{CatalogKey: "equipment:dagger", Quantity: 2}, &charactersv1.ItemGrant{Name: "Mapa velho"})
	if e := entry(v, "equipment:dagger"); e == nil || e.GetQuantity() != 2 {
		t.Fatalf("inventory = %v, want two daggers", v.GetItems())
	}
	// Another dagger joins the line.
	v = tb.give(t, tb.ana, tb.pensA.GetId(), &charactersv1.ItemGrant{CatalogKey: "equipment:dagger", Quantity: 3})
	if e := entry(v, "equipment:dagger"); e.GetQuantity() != 5 {
		t.Errorf("daggers = %d, want 5 in one line", e.GetQuantity())
	}
	for name, g := range map[string]*charactersv1.ItemGrant{
		"a magic item":           grant("item:ring-of-protection"),
		"an equipped item":       {CatalogKey: "equipment:dagger", Equipped: true},
		"an unidentified item":   {CatalogKey: "item:ring-of-protection", Unidentified: true},
		"an unknown key":         grant("equipment:lightsaber"),
		"a free item with a key": {Name: "x", BaseKey: "equipment:dagger"},
		"nothing":                {},
	} {
		_, err := tb.ana.inventory.GiveItems(t.Context(), connect.NewRequest(&charactersv1.GiveItemsRequest{CampaignId: tb.campaign, CharacterId: tb.pensA.GetId(), Grants: []*charactersv1.ItemGrant{g}, IdempotencyKey: nextKey()}))
		if code := connect.CodeOf(err); code != connect.CodePermissionDenied && code != connect.CodeInvalidArgument {
			t.Errorf("a player giving themselves %s: code %v, want permission_denied or invalid_argument", name, code)
		}
	}
	// Not another player's character.
	_, err := tb.ana.inventory.GiveItems(t.Context(), connect.NewRequest(&charactersv1.GiveItemsRequest{CampaignId: tb.campaign, CharacterId: tb.pensB.GetId(), Grants: []*charactersv1.ItemGrant{grant("equipment:dagger")}, IdempotencyKey: nextKey()}))
	wantCode(t, "GiveItems to another player", err, connect.CodeNotFound)
	// The master gives a magic item.
	v = tb.give(t, tb.master, tb.pensA.GetId(), grant("item:ring-of-protection"))
	if entry(v, "item:ring-of-protection") == nil {
		t.Errorf("the master's gift is missing: %v", v.GetItems())
	}
}

func TestGivingRefusesWhatTheRulesDoNot(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	for name, g := range map[string]*charactersv1.ItemGrant{
		"a family":                             grant("item:ring-of-resistance"),
		"a generic item without base":          grant("item:weapon-1"),
		"a base the item cannot be":            {CatalogKey: "item:weapon-1", BaseKey: "equipment:chain-mail"},
		"a base for a plain item":              {CatalogKey: "item:ring-of-protection", BaseKey: "equipment:dagger"},
		"armor of resistance without its type": {CatalogKey: "item:armor-of-resistance", BaseKey: "equipment:chain-mail"},
		"a scroll without a spell":             grant("item:spell-scroll-3rd"),
		"a scroll of another level":            {CatalogKey: "item:spell-scroll-3rd", ScrollSpellKey: "spell:magic-missile"},
		"two magic rings":                      {CatalogKey: "item:ring-of-protection", Quantity: 2},
		"a look on an identified one":          {CatalogKey: "item:ring-of-protection", Look: "Um anel"},
		"too many":                             {CatalogKey: "equipment:dagger", Quantity: 10000},
	} {
		_, err := tb.master.inventory.GiveItems(t.Context(), connect.NewRequest(&charactersv1.GiveItemsRequest{CampaignId: tb.campaign, CharacterId: id, Grants: []*charactersv1.ItemGrant{g}, IdempotencyKey: nextKey()}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("giving %s: %v, want invalid_argument", name, err)
		}
	}
	v := tb.give(t, tb.master, id,
		&charactersv1.ItemGrant{CatalogKey: "item:weapon-2", BaseKey: "equipment:longsword"},
		&charactersv1.ItemGrant{CatalogKey: "item:spell-scroll-3rd", ScrollSpellKey: "spell:fireball"},
		&charactersv1.ItemGrant{CatalogKey: "item:armor-of-resistance", BaseKey: "equipment:chain-mail", OptionKey: "damage-type:fire"},
		&charactersv1.ItemGrant{CatalogKey: "item:potion-of-healing-common", Quantity: 3},
	)
	if named(v, "Espada longa +2") == nil || entry(v, "item:potion-of-healing-common").GetQuantity() != 3 {
		t.Errorf("inventory = %v", v.GetItems())
	}
	if e := entry(v, "item:spell-scroll-3rd"); e.GetScrollSpellKey() != "spell:fireball" || e.GetScrollSpellNamePt() == "" {
		t.Errorf("scroll = %v", e)
	}
}

func TestEquippedArmorAndRingsChangeTheSheet(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	base := tb.ac(t, id)
	v := tb.give(t, tb.master, id, grant("equipment:chain-mail"), grant("equipment:shield"), grant("equipment:plate-armor"),
		grant("item:ring-of-protection"), grant("item:cloak-of-protection"), grant("item:ring-of-resistance-fire"), grant("item:ring-of-warmth"))
	if tb.ac(t, id) != base {
		t.Fatal("carried items changed the armor class")
	}
	chain, shield, plate := entry(v, "equipment:chain-mail"), entry(v, "equipment:shield"), entry(v, "equipment:plate-armor")
	if err := tb.equip(tb.ana, id, chain.GetId(), true); err != nil {
		t.Fatalf("equip chain mail: %v", err)
	}
	if got := tb.ac(t, id); got != 16 {
		t.Errorf("AC with chain mail = %d, want 16", got)
	}
	// One suit of armor at a time (SRD 5.1 "Multiple Items of the Same Kind").
	err := tb.equip(tb.ana, id, plate.GetId(), true)
	if reason := refused(t, "equip a second armor", err); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_SLOT_TAKEN {
		t.Errorf("reason = %v, want SLOT_TAKEN", reason)
	}
	if err := tb.equip(tb.ana, id, shield.GetId(), true); err != nil {
		t.Fatalf("equip shield: %v", err)
	}
	if got := tb.ac(t, id); got != 18 {
		t.Errorf("AC with chain mail and shield = %d, want 18", got)
	}
	// A ring of protection needs attunement: worn but not attuned it does nothing.
	ring := entry(v, "item:ring-of-protection")
	if err := tb.equip(tb.ana, id, ring.GetId(), true); err != nil {
		t.Fatalf("equip ring: %v", err)
	}
	if got := tb.ac(t, id); got != 18 {
		t.Errorf("AC with an unattuned ring = %d, want 18", got)
	}
	for _, key := range []string{"item:ring-of-resistance-fire", "item:ring-of-warmth"} {
		err := tb.equip(tb.ana, id, entry(v, key).GetId(), true)
		if key == "item:ring-of-warmth" {
			if reason := refused(t, "a third ring", err); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_SLOT_TAKEN {
				t.Errorf("third ring: reason %v, want SLOT_TAKEN", reason)
			}
		} else if err != nil {
			t.Errorf("second ring: %v", err)
		}
	}
	// The body armor is not changed in a combat; a shield goes through the combat.
	tb.host.setCombat(true)
	if reason := refused(t, "armor in combat", tb.equip(tb.ana, id, chain.GetId(), false)); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ARMOR_IN_COMBAT {
		t.Errorf("armor in combat: reason %v, want ARMOR_IN_COMBAT", reason)
	}
	if reason := refused(t, "shield in combat", tb.equip(tb.ana, id, shield.GetId(), false)); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_USE_IN_COMBAT {
		t.Errorf("shield in combat: reason %v, want USE_IN_COMBAT", reason)
	}
	if err := tb.equip(tb.master, id, chain.GetId(), false); err != nil {
		t.Errorf("the master takes the armor off in a combat: %v", err)
	}
	tb.host.setCombat(false)
	// Not a thing to wear.
	v = tb.give(t, tb.master, id, grant("item:potion-of-healing-common"))
	if reason := refused(t, "wear a potion", tb.equip(tb.ana, id, entry(v, "item:potion-of-healing-common").GetId(), true)); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_EQUIPPABLE {
		t.Errorf("reason = %v, want NOT_EQUIPPABLE", reason)
	}
}

func TestAttunementIsMarkedByThePlayerAndDoneInTheMastersShortRest(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	base := tb.ac(t, id)
	v := tb.give(t, tb.master, id, grant("item:cloak-of-protection"), grant("item:ring-of-protection"), grant("item:bracers-of-defense"),
		grant("item:amulet-of-health"), &charactersv1.ItemGrant{CatalogKey: "item:holy-avenger", BaseKey: "equipment:longsword"}, grant("item:staff-of-fire"), grant("item:potion-of-healing-common"))
	cloak := entry(v, "item:cloak-of-protection")
	if err := tb.equip(tb.ana, id, cloak.GetId(), true); err != nil {
		t.Fatal(err)
	}
	// Marked, nothing happens yet.
	if err := tb.request(tb.ana, id, cloak.GetId(), attuneKind); err != nil {
		t.Fatalf("ask to attune: %v", err)
	}
	if tb.ac(t, id) != base {
		t.Fatal("marking an attunement changed the armor class before the rest")
	}
	pending, err := tb.master.inventory.ListItemRests(t.Context(), connect.NewRequest(&charactersv1.ListItemRestsRequest{CampaignId: tb.campaign}))
	if err != nil || len(pending.Msg.GetPending()) != 1 || pending.Msg.GetPending()[0].GetAttunedAfter() != 1 || pending.Msg.GetPending()[0].GetCharacterName() != "Pensantus" {
		t.Fatalf("pending = %v, %v; want the cloak, 0 -> 1 attunements", pending.Msg.GetPending(), err)
	}
	// Players do not list them.
	_, err = tb.ana.inventory.ListItemRests(t.Context(), connect.NewRequest(&charactersv1.ListItemRestsRequest{CampaignId: tb.campaign}))
	wantCode(t, "ListItemRests as a player", err, connect.CodePermissionDenied)
	// Not in a combat.
	tb.host.setCombat(true)
	_, err = tb.master.inventory.ConfirmItemRests(t.Context(), connect.NewRequest(&charactersv1.ConfirmItemRestsRequest{CampaignId: tb.campaign, IdempotencyKey: nextKey()}))
	if reason := refused(t, "short rest in combat", err); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_IN_COMBAT {
		t.Errorf("reason = %v, want IN_COMBAT", reason)
	}
	if reason := refused(t, "ask in combat", tb.request(tb.ana, id, entry(v, "item:ring-of-protection").GetId(), attuneKind)); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_IN_COMBAT {
		t.Errorf("reason = %v, want IN_COMBAT", reason)
	}
	tb.host.setCombat(false)
	done := tb.rest(t)
	if len(done) != 1 || done[0].GetBlocked() != 0 || done[0].GetKind() != attuneKind {
		t.Fatalf("the rest did %v, want the attunement", done)
	}
	if got := tb.ac(t, id); got != base+1 {
		t.Errorf("AC after attuning to the cloak = %d, want %d", got, base+1)
	}
	got := tb.view(t, tb.ana, id)
	if got.GetAttunedCount() != 1 || !entry(got, "item:cloak-of-protection").GetAttuned() || entry(got, "item:cloak-of-protection").GetRequest() != 0 {
		t.Errorf("view = %v", got)
	}
	// Restrictions: a paladin's sword is not a wizard's; the staff of fire is.
	refusal := func(key string) charactersv1.ItemBlockedReason {
		return refused(t, "ask for "+key, tb.request(tb.ana, id, entry(v, key).GetId(), attuneKind))
	}
	if r := refusal("item:holy-avenger"); r != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ATTUNEMENT_RESTRICTION {
		t.Errorf("holy avenger: %v, want ATTUNEMENT_RESTRICTION", r)
	}
	if r := refusal("item:potion-of-healing-common"); r != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ATTUNABLE {
		t.Errorf("potion: %v, want NOT_ATTUNABLE", r)
	}
	// Three items at most: the cloak, the ring and the amulet; the staff is a fourth.
	for _, key := range []string{"item:ring-of-protection", "item:amulet-of-health"} {
		if err := tb.request(tb.ana, id, entry(v, key).GetId(), attuneKind); err != nil {
			t.Fatalf("ask for %s: %v", key, err)
		}
	}
	if done = tb.rest(t); len(done) != 2 {
		t.Fatalf("the rest did %v, want two attunements", done)
	}
	if r := refusal("item:staff-of-fire"); r != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ATTUNEMENT_FULL {
		t.Errorf("fourth item: %v, want ATTUNEMENT_FULL", r)
	}
	// Ending the attunement is marked and done in a rest too, and frees the place.
	if err := tb.request(tb.ana, id, cloak.GetId(), endKind); err != nil {
		t.Fatal(err)
	}
	if done = tb.rest(t); len(done) != 1 || done[0].GetKind() != endKind {
		t.Fatalf("the rest did %v, want the end of the cloak's attunement", done)
	}
	if err := tb.request(tb.ana, id, entry(v, "item:staff-of-fire").GetId(), attuneKind); err != nil {
		t.Errorf("a place was freed: %v", err)
	}
	// The master may refuse a request, and takes it back with UNSPECIFIED.
	if _, err := tb.master.inventory.ResolveItemRest(t.Context(), connect.NewRequest(&charactersv1.ResolveItemRestRequest{
		CampaignId: tb.campaign, CharacterId: id, ItemId: entry(v, "item:staff-of-fire").GetId(), Approve: false, IdempotencyKey: nextKey(),
	})); err != nil {
		t.Errorf("refuse a request: %v", err)
	}
	if r := tb.view(t, tb.master, id); entry(r, "item:staff-of-fire").GetRequest() != 0 {
		t.Error("the refused request stays marked")
	}
}

func TestACursedItemsAttunementDoesNotEnd(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	v := tb.give(t, tb.master, id, grant("item:shield-of-missile-attraction"))
	shield := entry(v, "item:shield-of-missile-attraction")
	if _, err := tb.master.inventory.ResolveItemRest(t.Context(), connect.NewRequest(&charactersv1.ResolveItemRestRequest{
		CampaignId: tb.campaign, CharacterId: id, ItemId: shield.GetId(), Kind: attuneKind, Approve: true, IdempotencyKey: nextKey(),
	})); err != nil {
		t.Fatalf("attune directly: %v", err)
	}
	if reason := refused(t, "end a cursed attunement", tb.request(tb.ana, id, shield.GetId(), endKind)); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_ITEM_CURSED {
		t.Errorf("reason = %v, want ITEM_CURSED", reason)
	}
}

func TestOnlyOneCopyOfAnItemIsAttunedTo(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	v := tb.give(t, tb.master, id, grant("item:ring-of-protection"), grant("item:ring-of-protection"))
	var rings []*charactersv1.InventoryEntry
	for _, e := range v.GetItems() {
		if e.GetCatalogKey() == "item:ring-of-protection" {
			rings = append(rings, e)
		}
	}
	if len(rings) != 2 {
		t.Fatalf("two rings are two lines, got %v", v.GetItems())
	}
	a, b := rings[0], rings[1]
	call := func(it *charactersv1.InventoryEntry) error {
		_, err := tb.master.inventory.ResolveItemRest(context.Background(), connect.NewRequest(&charactersv1.ResolveItemRestRequest{
			CampaignId: tb.campaign, CharacterId: id, ItemId: it.GetId(), Kind: attuneKind, Approve: true, IdempotencyKey: nextKey(),
		}))
		return err
	}
	if err := call(a); err != nil {
		t.Fatal(err)
	}
	if reason := refused(t, "a second copy", call(b)); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_COPY_ATTUNED {
		t.Errorf("reason = %v, want COPY_ATTUNED", reason)
	}
}

func TestUnidentifiedItemsShowOnlyTheirLookAndTheirMagicWaits(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	_ = tb.ac(t, id)
	v := tb.give(t, tb.master, id,
		&charactersv1.ItemGrant{CatalogKey: "item:armor-1", BaseKey: "equipment:chain-mail", Unidentified: true, Look: "Uma armadura com runas"},
		&charactersv1.ItemGrant{CatalogKey: "item:wand-of-fireballs", Unidentified: true, Look: "Uma varinha de osso"},
		&charactersv1.ItemGrant{CatalogKey: "item:potion-of-healing-common", Unidentified: true, Look: "Uma poção vermelha"},
		&charactersv1.ItemGrant{CatalogKey: "item:potion-of-healing-common", Unidentified: true, Look: "Uma poção vermelha"},
	)
	// Identical unidentified items never share a line.
	if n := len(v.GetItems()); n != 2+4 {
		t.Fatalf("%d lines, want the 2 free-text ones and 4 unidentified ones: %v", n, v.GetItems())
	}
	armor := entry(v, "item:armor-1")
	if armor == nil || !armor.GetUnidentified() || armor.GetLook() != "Uma armadura com runas" || !armor.GetEffectsPending() {
		t.Fatalf("the master's view of the armor = %v", armor)
	}
	pv := tb.view(t, tb.ana, id)
	var hidden []*charactersv1.InventoryEntry
	for _, e := range pv.GetItems() {
		if e.GetKind() == charactersv1.ItemKind_ITEM_KIND_MAGIC {
			hidden = append(hidden, e)
		}
	}
	if len(hidden) != 4 {
		t.Fatalf("the player sees %d magic lines, want 4: %v", len(hidden), pv.GetItems())
	}
	for _, e := range hidden {
		// Only the look, the quantity and the flags: nothing else (RN-10).
		want := &charactersv1.InventoryEntry{Id: e.GetId(), Kind: charactersv1.ItemKind_ITEM_KIND_MAGIC, NamePt: e.GetNamePt(), Quantity: 1, Unidentified: true}
		if e.String() != want.String() {
			t.Errorf("the player's view of an unidentified item = %v, want only %v", e, want)
		}
	}
	// Equip: the armor works as plain chain mail until identified.
	pArmor := named(pv, "Uma armadura com runas")
	if err := tb.equip(tb.ana, id, pArmor.GetId(), true); err != nil {
		t.Fatalf("equip: %v", err)
	}
	if got := tb.ac(t, id); got != 16 {
		t.Errorf("AC = %d, want plain chain mail's 16 while it is unidentified", got)
	}
	c := tb.ana.get(t, tb.campaign, id)
	if d := c.GetDerived().GetArmorClassDescription(); !strings.HasPrefix(d, "Uma armadura com runas") {
		t.Errorf("the AC description %q names the armor", d)
	}
	// A player can only ask to learn it; they cannot use it, or spend its charges.
	if err := tb.request(tb.ana, id, pArmor.GetId(), attuneKind); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("asking to attune to an unidentified item: %v, want invalid_argument (the same whatever it is)", err)
	}
	if err := tb.request(tb.ana, id, pArmor.GetId(), identifyKind); err != nil {
		t.Errorf("asking to identify: %v", err)
	}
	pot := named(tb.view(t, tb.ana, id), "Uma poção vermelha")
	_, err := tb.ana.inventory.UseItem(t.Context(), connect.NewRequest(&charactersv1.UseItemRequest{
		CampaignId: tb.campaign, CharacterId: id, ItemId: pot.GetId(), IdempotencyKey: nextKey(), Roll: &charactersv1.UseItemRequest_RollInApp{RollInApp: true},
	}))
	if reason := refused(t, "drink an unidentified potion", err); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_USABLE {
		t.Errorf("reason = %v, want NOT_USABLE", reason)
	}
	// Another player sees nothing of it, and the stream names nobody's item.
	_, err = tb.bruno.inventory.GetInventory(t.Context(), connect.NewRequest(&charactersv1.GetInventoryRequest{CampaignId: tb.campaign, CharacterId: id}))
	wantCode(t, "another player's inventory", err, connect.CodeNotFound)
	// The master identifies it in the short rest: the bonus counts and the log says so.
	done := tb.rest(t)
	if len(done) != 1 || done[0].GetKind() != identifyKind || done[0].GetNamePt() != "Cota de malha +1" {
		t.Fatalf("the rest did %v, want the identification, named for the master", done)
	}
	if got := tb.ac(t, id); got != 17 {
		t.Errorf("AC = %d, want 17 once identified", got)
	}
	pv = tb.view(t, tb.ana, id)
	if e := named(pv, "Cota de malha +1"); e == nil || e.GetUnidentified() || e.GetCatalogKey() != "item:armor-1" {
		t.Errorf("the identified armor as the player sees it: %v", e)
	}
	// Log: the master's lines name the item; a player's name it as the character saw it.
	log, err := tb.master.inventory.ListItemLog(t.Context(), connect.NewRequest(&charactersv1.ListItemLogRequest{CampaignId: tb.campaign}))
	if err != nil || len(log.Msg.GetEntries()) == 0 {
		t.Fatalf("log = %v, %v", log, err)
	}
	for _, e := range log.Msg.GetEntries() {
		if e.GetKind() == charactersv1.ItemLogKind_ITEM_LOG_KIND_GIVEN && e.GetItemNamePt() == "" {
			t.Errorf("the master's log line has no item name: %v", e)
		}
	}
	// Positive control for the leak canary below: the master's view does show the name.
	if entry(tb.view(t, tb.master, id), "item:wand-of-fireballs") == nil {
		t.Error("the master cannot see the wand")
	}
}

func TestPotionsHealWhenDrunkAndAreUsedUp(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t, 3, 4) // 2d4: 3 + 4
	id := tb.pensA.GetId()
	v := tb.give(t, tb.master, id, &charactersv1.ItemGrant{CatalogKey: "item:potion-of-healing-common", Quantity: 2}, grant("item:potion-of-heroism"))
	// Wound the character first: 20 of 23.
	five := int32(5)
	tb.h.adjustVitals(tb.campaign, id, &playv1.AdjustCharacterVitalsRequest{IdempotencyKey: uuid.New().String(), HitPointsCurrent: &five})
	tb.h.adjustVitals(tb.campaign, tb.pensB.GetId(), &playv1.AdjustCharacterVitalsRequest{IdempotencyKey: uuid.New().String(), HitPointsCurrent: &five})
	potion := entry(v, "item:potion-of-healing-common")
	use := func(it *charactersv1.InventoryEntry, target string, roll *charactersv1.UseItemRequest) (*charactersv1.UseItemResponse, error) {
		roll.CampaignId, roll.CharacterId, roll.ItemId, roll.IdempotencyKey, roll.TargetCharacterId = tb.campaign, id, it.GetId(), nextKey(), target
		res, err := tb.ana.inventory.UseItem(t.Context(), connect.NewRequest(roll))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	if _, err := use(potion, "", &charactersv1.UseItemRequest{}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a potion with dice and no roll: %v, want invalid_argument", err)
	}
	if _, err := use(potion, "", &charactersv1.UseItemRequest{Roll: &charactersv1.UseItemRequest_TypedSum{TypedSum: 9}}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a typed sum above the dice: %v, want invalid_argument", err)
	}
	res, err := use(potion, "", &charactersv1.UseItemRequest{Roll: &charactersv1.UseItemRequest_RollInApp{RollInApp: true}})
	if err != nil {
		t.Fatalf("drink: %v", err)
	}
	if res.GetOutcome().GetHealed() != 3+4+2 || res.GetOutcome().GetDice() != "2d4+2" {
		t.Errorf("outcome = %v, want 2d4+2 = 9", res.GetOutcome())
	}
	if e := entry(res.GetInventory(), "item:potion-of-healing-common"); e.GetQuantity() != 1 {
		t.Errorf("potions left = %v, want 1", e)
	}
	// Administering one to another character is the same item going to the other's vitals.
	res, err = use(potion, tb.pensB.GetId(), &charactersv1.UseItemRequest{Roll: &charactersv1.UseItemRequest_TypedSum{TypedSum: 5}})
	if err != nil {
		t.Fatalf("give to drink: %v", err)
	}
	if res.GetOutcome().GetHealed() != 7 || entry(res.GetInventory(), "item:potion-of-healing-common") != nil {
		t.Errorf("outcome = %v, inventory %v; want 7 healed and the potion used up", res.GetOutcome(), res.GetInventory().GetItems())
	}
	// Heroism gives temporary hit points, once.
	heroism := entry(res.GetInventory(), "item:potion-of-heroism")
	res, err = use(heroism, "", &charactersv1.UseItemRequest{})
	if err != nil || res.GetOutcome().GetTempHitPointsGiven() != 10 || res.GetOutcome().GetTempHitPoints() != 10 {
		t.Errorf("heroism = %v, %v; want 10 temporary hit points", res.GetOutcome(), err)
	}
	// In a combat the potion is an action of the combat.
	tb.host.setCombat(true)
	v = tb.give(t, tb.master, id, grant("item:potion-of-healing-greater"))
	_, err = use(entry(v, "item:potion-of-healing-greater"), "", &charactersv1.UseItemRequest{Roll: &charactersv1.UseItemRequest_RollInApp{RollInApp: true}})
	if reason := refused(t, "drink in combat", err); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_USE_IN_COMBAT {
		t.Errorf("reason = %v, want USE_IN_COMBAT", reason)
	}
}

func TestScrollsFollowTheClassListAndTheAbilityCheck(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t, 12, 4)
	id := tb.pensA.GetId()
	v := tb.give(t, tb.master, id,
		&charactersv1.ItemGrant{CatalogKey: "item:spell-scroll-1st", ScrollSpellKey: "spell:magic-missile"},
		&charactersv1.ItemGrant{CatalogKey: "item:spell-scroll-4th", ScrollSpellKey: "spell:greater-invisibility"},
		&charactersv1.ItemGrant{CatalogKey: "item:spell-scroll-1st", ScrollSpellKey: "spell:cure-wounds"},
		&charactersv1.ItemGrant{CatalogKey: "item:spell-scroll-3rd", ScrollSpellKey: "spell:fireball"},
	)
	read := func(it *charactersv1.InventoryEntry, roll *charactersv1.UseScrollRequest) (*charactersv1.UseScrollResponse, error) {
		roll.CampaignId, roll.CharacterId, roll.ItemId, roll.IdempotencyKey = tb.campaign, id, it.GetId(), nextKey()
		res, err := tb.ana.inventory.UseScroll(t.Context(), connect.NewRequest(roll))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	items := map[string]*charactersv1.InventoryEntry{}
	for _, e := range v.GetItems() {
		if e.GetScrollSpellKey() != "" {
			items[e.GetScrollSpellKey()] = e
		}
	}
	// A level 1 spell a wizard 3 casts: read at once.
	res, err := read(items["spell:magic-missile"], &charactersv1.UseScrollRequest{})
	if err != nil || !res.GetCast() || res.GetSaveDc() != 13 || res.GetAttackBonus() != 5 || res.GetAbilityCheck() != nil {
		t.Fatalf("magic missile = %v, %v", res, err)
	}
	for _, e := range res.GetInventory().GetItems() {
		if e.GetScrollSpellKey() == "spell:magic-missile" {
			t.Error("the scroll was not used up")
		}
	}
	// A 4th-level spell is above a wizard 3's highest slot (2nd): an ability check, DC 14.
	if _, err := read(items["spell:greater-invisibility"], &charactersv1.UseScrollRequest{}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no roll for a check: %v, want invalid_argument", err)
	}
	res, err = read(items["spell:greater-invisibility"], &charactersv1.UseScrollRequest{Roll: &charactersv1.UseScrollRequest_D20Face{D20Face: 12}})
	if err != nil {
		t.Fatalf("read the 4th-level scroll: %v", err)
	}
	check := res.GetAbilityCheck()
	if check == nil || check.GetDc() != 14 || check.GetAbility() != "int" || check.GetTotal() != 12+check.GetModifier() || check.GetPassed() != (check.GetTotal() >= 14) || res.GetCast() != check.GetPassed() {
		t.Errorf("check = %v, cast %v; want DC 14 with the Intelligence modifier", check, res.GetCast())
	}
	if entry(res.GetInventory(), "item:spell-scroll-4th") != nil {
		t.Errorf("a scroll read with a check stays: %v", res.GetInventory().GetItems())
	}
	// A spell on none of the reader's class lists is unintelligible and stays.
	_, err = read(items["spell:cure-wounds"], &charactersv1.UseScrollRequest{Roll: &charactersv1.UseScrollRequest_D20Face{D20Face: 20}})
	if reason := refused(t, "unreadable scroll", err); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_SCROLL_UNREADABLE {
		t.Errorf("reason = %v, want SCROLL_UNREADABLE", reason)
	}
	if entry(tb.view(t, tb.ana, id), "item:spell-scroll-1st") == nil {
		t.Error("an unreadable scroll was used up")
	}
}

func TestChargesAreSpentAndComeBackAtDawn(t *testing.T) {
	t.Parallel()
	// Dice in order: the d20 of the last charge (1: destroyed), then 1d6+1 for dawn = 4.
	tb := newInvTable(t, 6, 3)
	id := tb.pensA.GetId()
	v := tb.give(t, tb.master, id, grant("item:wand-of-magic-missiles"), grant("item:wand-of-secrets"))
	wand := entry(v, "item:wand-of-magic-missiles")
	spend := func(it *charactersv1.InventoryEntry, n int32) (*charactersv1.SpendChargesResponse, error) {
		res, err := tb.ana.inventory.SpendCharges(t.Context(), connect.NewRequest(&charactersv1.SpendChargesRequest{
			CampaignId: tb.campaign, CharacterId: id, ItemId: it.GetId(), Charges: n, IdempotencyKey: nextKey(),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	res, err := spend(wand, 3)
	if err != nil || entry(res.GetInventory(), "item:wand-of-magic-missiles").GetChargesUsed() != 3 || entry(res.GetInventory(), "item:wand-of-magic-missiles").GetChargesMax() != 7 {
		t.Fatalf("spend 3 = %v, %v", res, err)
	}
	if _, err := spend(wand, 5); err == nil {
		t.Fatal("spending more charges than are left was accepted")
	} else if r := refused(t, "too many charges", err); r != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH {
		t.Errorf("reason = %v, want NOT_ENOUGH", r)
	}
	// Dawn: the master rolls 1d6+1 (6 + 1 = 7 > 3 spent, so all 3 come back; the dice are 6 then 3).
	regained, err := tb.master.inventory.RegainCharges(t.Context(), connect.NewRequest(&charactersv1.RegainChargesRequest{CampaignId: tb.campaign, IdempotencyKey: nextKey()}))
	if err != nil || len(regained.Msg.GetRegained()) != 1 || regained.Msg.GetRegained()[0].GetRegained() != 3 || regained.Msg.GetRegained()[0].GetChargesUsed() != 0 {
		t.Fatalf("dawn = %v, %v; want the 3 spent charges back", regained, err)
	}
	_, err = tb.ana.inventory.RegainCharges(t.Context(), connect.NewRequest(&charactersv1.RegainChargesRequest{CampaignId: tb.campaign, IdempotencyKey: nextKey()}))
	wantCode(t, "RegainCharges as a player", err, connect.CodePermissionDenied)
	// The last charge risks the wand: the d20 is rolled (3 here), the wand lives.
	res, err = spend(wand, 7)
	if err != nil || res.GetD20() != 3 || res.GetDestroyed() {
		t.Fatalf("last charge = %v, %v; want the d20 3 and the wand kept", res, err)
	}
	// An item whose charges the engine does not roll for (no regain dice) is fine too.
	if secrets := entry(res.GetInventory(), "item:wand-of-secrets"); secrets == nil {
		t.Error("the other wand is gone")
	}
}

func TestTheLastChargeCanDestroyTheWand(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t, 1)
	id := tb.pensA.GetId()
	v := tb.give(t, tb.master, id, grant("item:wand-of-web"))
	res, err := tb.ana.inventory.SpendCharges(t.Context(), connect.NewRequest(&charactersv1.SpendChargesRequest{
		CampaignId: tb.campaign, CharacterId: id, ItemId: entry(v, "item:wand-of-web").GetId(), Charges: 7, IdempotencyKey: nextKey(),
	}))
	if err != nil || !res.Msg.GetDestroyed() || res.Msg.GetD20() != 1 || entry(res.Msg.GetInventory(), "item:wand-of-web") != nil {
		t.Fatalf("last charge on a 1 = %v, %v; want the wand destroyed and gone", res, err)
	}
}

func TestAmmunitionIsRecoveredHalfRoundedDown(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	v := tb.give(t, tb.master, id, &charactersv1.ItemGrant{CatalogKey: "equipment:arrow", Quantity: 20}, grant("equipment:shortbow"))
	arrows, bow := entry(v, "equipment:arrow"), entry(v, "equipment:shortbow")
	if err := tb.equip(tb.ana, id, bow.GetId(), true); err != nil {
		t.Fatal(err)
	}
	c := tb.ana.get(t, tb.campaign, id)
	var attack *rulesv1.Attack
	for _, a := range c.GetDerived().GetAttacks() {
		if a.GetKey() == "inv:"+bow.GetId() {
			attack = a
		}
	}
	if attack == nil || attack.GetAmmunitionKey() != "equipment:arrow" || attack.GetAmmunitionItemId() != arrows.GetId() {
		t.Fatalf("the bow's attack = %v, want it to spend the arrows %q", attack, arrows.GetId())
	}
	// 11 arrows were fired in the fight.
	for range 11 {
		if err := db.InTx(t.Context(), tb.h.svc.pool, func(tx pgx.Tx) error {
			return tb.h.svc.SpendAmmunition(t.Context(), tx, tb.campaign, id, arrows.GetId())
		}); err != nil {
			t.Fatal(err)
		}
	}
	got := entry(tb.view(t, tb.ana, id), "equipment:arrow")
	if got.GetQuantity() != 9 || got.GetAmmunitionRecoverable() != 5 {
		t.Fatalf("after the fight: %d arrows, %d recoverable; want 9 and 5 (half of 11, rounded down)", got.GetQuantity(), got.GetAmmunitionRecoverable())
	}
	recoverArrows := func(n int32) error {
		_, err := tb.ana.inventory.RecoverAmmunition(t.Context(), connect.NewRequest(&charactersv1.RecoverAmmunitionRequest{
			CampaignId: tb.campaign, CharacterId: id, ItemId: arrows.GetId(), Count: n, IdempotencyKey: nextKey(),
		}))
		return err
	}
	if reason := refused(t, "recover too many", recoverArrows(6)); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH {
		t.Errorf("reason = %v, want NOT_ENOUGH", reason)
	}
	if err := recoverArrows(5); err != nil {
		t.Fatalf("recover 5: %v", err)
	}
	if got = entry(tb.view(t, tb.ana, id), "equipment:arrow"); got.GetQuantity() != 14 || got.GetAmmunitionRecoverable() != 0 {
		t.Errorf("after the search: %v, want 14 arrows and nothing more to recover", got)
	}
	if reason := refused(t, "recover twice", recoverArrows(1)); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOTHING_SPENT {
		t.Errorf("reason = %v, want NOTHING_SPENT", reason)
	}
}

func TestCoinsAreSetAndAdjustedByTheOwnerAndTheMaster(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	set := func(u *user, coins *charactersv1.Coins) (*charactersv1.InventoryView, error) {
		res, err := u.inventory.SetCoins(t.Context(), connect.NewRequest(&charactersv1.SetCoinsRequest{CampaignId: tb.campaign, CharacterId: id, Coins: coins, IdempotencyKey: nextKey()}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetInventory(), nil
	}
	v, err := set(tb.ana, &charactersv1.Coins{Gold: 85, Silver: 12})
	if err != nil || v.GetCoins().GetGold() != 85 || v.GetCoins().GetSilver() != 12 || v.GetCoins().GetCopper() != 0 {
		t.Fatalf("SetCoins = %v, %v", v, err)
	}
	if _, err := set(tb.ana, &charactersv1.Coins{Gold: -1}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("negative coins: %v, want invalid_argument", err)
	}
	if _, err := set(tb.bruno, &charactersv1.Coins{}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another player's purse: %v, want not_found", err)
	}
	adjust := func(delta *charactersv1.Coins) (*charactersv1.AdjustCoinsResponse, error) {
		res, err := tb.master.inventory.AdjustCoins(t.Context(), connect.NewRequest(&charactersv1.AdjustCoinsRequest{CampaignId: tb.campaign, CharacterId: id, Delta: delta, IdempotencyKey: nextKey()}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	if res, err := adjust(&charactersv1.Coins{Gold: -80, Platinum: 2}); err != nil || res.GetInventory().GetCoins().GetGold() != 5 || res.GetInventory().GetCoins().GetPlatinum() != 2 {
		t.Fatalf("AdjustCoins = %v, %v", res, err)
	}
	if _, err := adjust(&charactersv1.Coins{Gold: -6}); refused(t, "overdraw", err) != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH {
		t.Error("overdrawing was not refused as NOT_ENOUGH")
	}
}

func TestIdempotencyKeysAreTheCallersAndHoldTheRequest(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	give := func(u *user, key string, g *charactersv1.ItemGrant) (*charactersv1.GiveItemsResponse, error) {
		res, err := u.inventory.GiveItems(t.Context(), connect.NewRequest(&charactersv1.GiveItemsRequest{CampaignId: tb.campaign, CharacterId: id, Grants: []*charactersv1.ItemGrant{g}, IdempotencyKey: key}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	first, err := give(tb.master, "same", &charactersv1.ItemGrant{CatalogKey: "equipment:torch", Quantity: 3})
	if err != nil || entry(first.GetInventory(), "equipment:torch").GetQuantity() != 3 {
		t.Fatalf("first = %v, %v", first, err)
	}
	again, err := give(tb.master, "same", &charactersv1.ItemGrant{CatalogKey: "equipment:torch", Quantity: 3})
	if err != nil || entry(again.GetInventory(), "equipment:torch").GetQuantity() != 3 {
		t.Errorf("a retry = %v, %v; want the same three torches, not six", again, err)
	}
	if _, err := give(tb.master, "same", &charactersv1.ItemGrant{CatalogKey: "equipment:torch", Quantity: 4}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("the key with another request: %v, want invalid_argument", err)
	}
	// The key is the caller's: the owner's identical key is a request of their own.
	other, err := give(tb.ana, "same", &charactersv1.ItemGrant{CatalogKey: "equipment:torch", Quantity: 4})
	if err != nil || entry(other.GetInventory(), "equipment:torch").GetQuantity() != 7 {
		t.Errorf("another caller's key = %v, %v; want it to run (3 + 4 torches)", other, err)
	}
	if _, err := give(tb.master, "", grant("equipment:torch")); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no key: %v, want invalid_argument", err)
	}
}

func TestItemsChangeHandsAndTheMasterSeesItInTheLog(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	a, b := tb.pensA.GetId(), tb.pensB.GetId()
	v := tb.give(t, tb.master, a, &charactersv1.ItemGrant{CatalogKey: "equipment:torch", Quantity: 5}, grant("item:ring-of-protection"),
		&charactersv1.ItemGrant{CatalogKey: "item:weapon-1", BaseKey: "equipment:dagger", Unidentified: true, Look: "Uma adaga com runas"})
	torch, ring, dagger := entry(v, "equipment:torch"), entry(v, "item:ring-of-protection"), named(v, "Uma adaga com runas")
	if err := tb.equip(tb.ana, a, ring.GetId(), true); err != nil {
		t.Fatal(err)
	}
	transfer := func(u *user, from, to, item string, qty int32) (*charactersv1.TransferItemResponse, error) {
		res, err := u.inventory.TransferItem(t.Context(), connect.NewRequest(&charactersv1.TransferItemRequest{
			CampaignId: tb.campaign, FromCharacterId: from, ToCharacterId: to, ItemId: item, Quantity: qty, IdempotencyKey: nextKey(),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	res, err := transfer(tb.ana, a, b, torch.GetId(), 2)
	if err != nil || entry(res.GetFrom(), "equipment:torch").GetQuantity() != 3 || res.GetTo() != nil {
		t.Fatalf("part of a stack = %v, %v; want 3 left and no view of Bruno's inventory", res, err)
	}
	if got := entry(tb.view(t, tb.bruno, b), "equipment:torch"); got.GetQuantity() != 2 {
		t.Errorf("Bruno's torches = %v, want 2", got)
	}
	// A worn magic ring arrives in the pack, not worn and not attuned; the dagger stays unidentified.
	if _, err := transfer(tb.ana, a, b, ring.GetId(), 0); err != nil {
		t.Fatalf("give the ring: %v", err)
	}
	if _, err := transfer(tb.ana, a, b, dagger.GetId(), 0); err != nil {
		t.Fatalf("give the dagger: %v", err)
	}
	pv := tb.view(t, tb.bruno, b)
	if r := entry(pv, "item:ring-of-protection"); r == nil || r.GetEquipped() || r.GetAttuned() {
		t.Errorf("the ring in Bruno's hands: %v", r)
	}
	if d := named(pv, "Uma adaga com runas"); d == nil || !d.GetUnidentified() || d.GetCatalogKey() != "" {
		t.Errorf("the dagger in Bruno's hands: %v", d)
	}
	// Not to a character who is not a player's living one, not from another player.
	_, err = transfer(tb.bruno, a, b, torch.GetId(), 1)
	wantCode(t, "taking from another player", err, connect.CodeNotFound)
	if _, err = transfer(tb.ana, a, a, torch.GetId(), 1); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("to oneself: %v, want invalid_argument", err)
	}
	npc := tb.master.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Strahd", enemySheet())
	if reason := refused(t, "to an NPC", errOf(transfer(tb.ana, a, npc.GetId(), torch.GetId(), 1))); reason != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_CANNOT_RECEIVE {
		t.Errorf("reason = %v, want CANNOT_RECEIVE", reason)
	}
	// The master's log has the hand-overs, naming the items; a player reads the dagger as its look.
	log, err := tb.master.inventory.ListItemLog(t.Context(), connect.NewRequest(&charactersv1.ListItemLogRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatal(err)
	}
	var transfers, daggerLines int
	for _, e := range log.Msg.GetEntries() {
		if e.GetKind() == charactersv1.ItemLogKind_ITEM_LOG_KIND_TRANSFERRED {
			transfers++
			if e.GetCharacterName() != "Pensantus" || e.GetToCharacterName() != "Brisa" {
				t.Errorf("transfer line %v does not name the characters", e)
			}
		}
		if e.GetItemNamePt() == "Adaga +1" {
			daggerLines++
		}
	}
	if transfers != 3 || daggerLines == 0 {
		t.Errorf("the master's log has %d transfers and %d lines naming the dagger as it is; want 3 and some", transfers, daggerLines)
	}
	plog, err := tb.bruno.inventory.ListItemLog(t.Context(), connect.NewRequest(&charactersv1.ListItemLogRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range plog.Msg.GetEntries() {
		if strings.Contains(e.GetItemNamePt(), "Adaga +1") || strings.Contains(e.GetItemNamePt(), "Arma +1") {
			t.Errorf("a player's log line names the unidentified dagger: %v", e)
		}
	}
	if len(tb.host.published) == 0 {
		t.Error("no stream hint was published")
	}
}

func errOf[T any](_ T, err error) error { return err }

func TestAFreeTextLineBecomesItemsAfterAPreview(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	v := tb.give(t, tb.ana, id, &charactersv1.ItemGrant{Name: "Corda de cânhamo (15 m), 10 rações, tochas"})
	line := named(v, "Corda de cânhamo (15 m), 10 rações, tochas")
	prev, err := tb.ana.inventory.PreviewTextToItems(t.Context(), connect.NewRequest(&charactersv1.PreviewTextToItemsRequest{CampaignId: tb.campaign, CharacterId: id, ItemId: line.GetId()}))
	if err != nil || len(prev.Msg.GetProposals()) != 3 {
		t.Fatalf("preview = %v, %v", prev, err)
	}
	p := prev.Msg.GetProposals()
	if p[0].GetCatalogKey() != "equipment:rope-hempen-50-feet" || p[1].GetQuantity() != 10 || p[1].GetQuantityGuessed() || p[2].GetCatalogKey() != "equipment:torch" || !p[2].GetQuantityGuessed() || p[2].GetQuantity() != 10 {
		t.Errorf("proposals = %v", p)
	}
	// Nothing changed yet.
	if named(tb.view(t, tb.ana, id), line.GetNamePt()) == nil {
		t.Fatal("the preview changed the inventory")
	}
	lines := p
	lines[2].Quantity = 6
	res, err := tb.ana.inventory.ConfirmTextToItems(t.Context(), connect.NewRequest(&charactersv1.ConfirmTextToItemsRequest{
		CampaignId: tb.campaign, CharacterId: id, ItemId: line.GetId(), Lines: lines, IdempotencyKey: nextKey(),
	}))
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	inv := res.Msg.GetInventory()
	if named(inv, line.GetNamePt()) != nil || entry(inv, "equipment:torch").GetQuantity() != 6 || entry(inv, "equipment:rations-1-day").GetQuantity() != 10 {
		t.Errorf("inventory = %v", inv.GetItems())
	}
	if len(inv.GetConvertedTexts()) != 1 || inv.GetConvertedTexts()[0] != "Corda de cânhamo (15 m), 10 rações, tochas" {
		t.Errorf("the original text = %v, want it kept", inv.GetConvertedTexts())
	}
	// Only a free-text line, only the owner's.
	if _, err := tb.ana.inventory.PreviewTextToItems(t.Context(), connect.NewRequest(&charactersv1.PreviewTextToItemsRequest{CampaignId: tb.campaign, CharacterId: id, ItemId: entry(inv, "equipment:torch").GetId()})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("preview of an item: %v, want invalid_argument", err)
	}
}

func TestOnlyTheMasterRemovesAMagicItem(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	v := tb.give(t, tb.master, id, grant("item:ring-of-protection"), &charactersv1.ItemGrant{CatalogKey: "equipment:torch", Quantity: 4})
	remove := func(u *user, it *charactersv1.InventoryEntry, qty int32) error {
		_, err := u.inventory.RemoveItem(t.Context(), connect.NewRequest(&charactersv1.RemoveItemRequest{CampaignId: tb.campaign, CharacterId: id, ItemId: it.GetId(), Quantity: qty, IdempotencyKey: nextKey()}))
		return err
	}
	if err := remove(tb.ana, entry(v, "item:ring-of-protection"), 0); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player removing a magic ring: %v, want permission_denied", err)
	}
	if err := remove(tb.ana, entry(v, "equipment:torch"), 3); err != nil {
		t.Errorf("a player removing torches: %v", err)
	}
	if err := remove(tb.ana, entry(v, "equipment:torch"), 9); refused(t, "remove too many", err) != charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH {
		t.Error("removing more than there is was not refused")
	}
	if err := remove(tb.master, entry(v, "item:ring-of-protection"), 0); err != nil {
		t.Errorf("the master removing it: %v", err)
	}
	if got := tb.view(t, tb.ana, id); entry(got, "item:ring-of-protection") != nil || entry(got, "equipment:torch").GetQuantity() != 1 {
		t.Errorf("inventory = %v", got.GetItems())
	}
}

func TestUpdateItemIsTheMastersCorrection(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	v := tb.give(t, tb.master, id, grant("item:wand-of-magic-missiles"), &charactersv1.ItemGrant{CatalogKey: "item:ring-of-protection", Unidentified: true})
	wand, ring := entry(v, "item:wand-of-magic-missiles"), entry(v, "item:ring-of-protection")
	upd := func(u *user, req *charactersv1.UpdateItemRequest) (*charactersv1.InventoryView, error) {
		req.CampaignId, req.CharacterId, req.IdempotencyKey = tb.campaign, id, nextKey()
		res, err := u.inventory.UpdateItem(t.Context(), connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetInventory(), nil
	}
	n := int32(4)
	if _, err := upd(tb.ana, &charactersv1.UpdateItemRequest{ItemId: wand.GetId(), ChargesUsed: &n}); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player correcting charges: %v, want permission_denied", err)
	}
	if got, err := upd(tb.master, &charactersv1.UpdateItemRequest{ItemId: wand.GetId(), ChargesUsed: &n}); err != nil || entry(got, "item:wand-of-magic-missiles").GetChargesUsed() != 4 {
		t.Errorf("charges = %v, %v", got, err)
	}
	n = 99
	if _, err := upd(tb.master, &charactersv1.UpdateItemRequest{ItemId: wand.GetId(), ChargesUsed: &n}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("charges above the maximum: %v, want invalid_argument", err)
	}
	hide := false
	if got, err := upd(tb.master, &charactersv1.UpdateItemRequest{ItemId: ring.GetId(), Unidentified: &hide}); err != nil || entry(got, "item:ring-of-protection").GetUnidentified() {
		t.Errorf("identify = %v, %v", got, err)
	}
	two := int32(2)
	if _, err := upd(tb.master, &charactersv1.UpdateItemRequest{ItemId: ring.GetId(), Quantity: &two}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("two rings in a line: %v, want invalid_argument", err)
	}
}

// The authorization matrix of InventoryService: every method, as each of the six callers.
func TestInventoryAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	pending := tb.h.newUser("Pendente")
	tb.h.joinPending(tb.master, tb.campaign, pending)
	pendingPC := pending.createPensantus(t, tb.campaign)
	v := tb.give(t, tb.master, id, grant("item:ring-of-protection"), &charactersv1.ItemGrant{CatalogKey: "equipment:torch", Quantity: 9}, grant("item:potion-of-healing-common"))
	torch, ring := entry(v, "equipment:torch"), entry(v, "item:ring-of-protection")
	ctx := t.Context()
	callers := []struct {
		name string
		u    *user
	}{{"master", tb.master}, {"owner", tb.ana}, {"other player", tb.bruno}, {"non-member", tb.h.newUser("De fora")}, {"anonymous", tb.h.anonymous()}, {"pending", pending}}
	nf, pd, ok, un := connect.CodeNotFound, connect.CodePermissionDenied, connect.Code(0), connect.CodeUnauthenticated
	type call func(u *user, character string) error
	camp := tb.campaign
	rows := []struct {
		method string
		call   call
		want   [6]connect.Code // master, owner, other player, non-member, anonymous, pending (asking on Ana's character)
	}{
		{"GetInventory", func(u *user, c string) error {
			_, err := u.inventory.GetInventory(ctx, connect.NewRequest(&charactersv1.GetInventoryRequest{CampaignId: camp, CharacterId: c}))
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"ListItemCatalog", func(u *user, _ string) error {
			_, err := u.inventory.ListItemCatalog(ctx, connect.NewRequest(&charactersv1.ListItemCatalogRequest{CampaignId: camp}))
			return err
		}, [6]connect.Code{ok, ok, ok, nf, un, nf}},
		{"GiveItems", func(u *user, c string) error {
			_, err := u.inventory.GiveItems(ctx, connect.NewRequest(&charactersv1.GiveItemsRequest{CampaignId: camp, CharacterId: c, Grants: []*charactersv1.ItemGrant{grant("equipment:dagger")}, IdempotencyKey: nextKey()}))
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"SetEquipped", func(u *user, c string) error { return tb.equip(u, c, torch.GetId(), false) }, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"RemoveItem", func(u *user, c string) error {
			_, err := u.inventory.RemoveItem(ctx, connect.NewRequest(&charactersv1.RemoveItemRequest{CampaignId: camp, CharacterId: c, ItemId: torch.GetId(), Quantity: 1, IdempotencyKey: nextKey()}))
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"TransferItem", func(u *user, c string) error {
			_, err := u.inventory.TransferItem(ctx, connect.NewRequest(&charactersv1.TransferItemRequest{CampaignId: camp, FromCharacterId: c, ToCharacterId: tb.pensB.GetId(), ItemId: torch.GetId(), Quantity: 1, IdempotencyKey: nextKey()}))
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"AdjustCoins", func(u *user, c string) error {
			_, err := u.inventory.AdjustCoins(ctx, connect.NewRequest(&charactersv1.AdjustCoinsRequest{CampaignId: camp, CharacterId: c, Delta: &charactersv1.Coins{Gold: 1}, IdempotencyKey: nextKey()}))
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"SetCoins", func(u *user, c string) error {
			_, err := u.inventory.SetCoins(ctx, connect.NewRequest(&charactersv1.SetCoinsRequest{CampaignId: camp, CharacterId: c, Coins: &charactersv1.Coins{Gold: 7}, IdempotencyKey: nextKey()}))
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"RequestItemRest", func(u *user, c string) error { return tb.request(u, c, ring.GetId(), attuneKind) }, [6]connect.Code{pd, ok, nf, nf, un, nf}},
		{"ResolveItemRest", func(u *user, c string) error {
			_, err := u.inventory.ResolveItemRest(ctx, connect.NewRequest(&charactersv1.ResolveItemRestRequest{CampaignId: camp, CharacterId: c, ItemId: ring.GetId(), Approve: false, IdempotencyKey: nextKey()}))
			return err
		}, [6]connect.Code{ok, pd, pd, nf, un, nf}},
		{"ListItemRests", func(u *user, _ string) error {
			_, err := u.inventory.ListItemRests(ctx, connect.NewRequest(&charactersv1.ListItemRestsRequest{CampaignId: camp}))
			return err
		}, [6]connect.Code{ok, pd, pd, nf, un, nf}},
		{"ConfirmItemRests", func(u *user, _ string) error {
			_, err := u.inventory.ConfirmItemRests(ctx, connect.NewRequest(&charactersv1.ConfirmItemRestsRequest{CampaignId: camp, IdempotencyKey: nextKey()}))
			return err
		}, [6]connect.Code{ok, pd, pd, nf, un, nf}},
		{"UseItem", func(u *user, c string) error {
			_, err := u.inventory.UseItem(ctx, connect.NewRequest(&charactersv1.UseItemRequest{CampaignId: camp, CharacterId: c, ItemId: torch.GetId(), IdempotencyKey: nextKey()}))
			if connect.CodeOf(err) == connect.CodeFailedPrecondition { // a torch is not for drinking: the caller got in
				return nil
			}
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"UseScroll", func(u *user, c string) error {
			_, err := u.inventory.UseScroll(ctx, connect.NewRequest(&charactersv1.UseScrollRequest{CampaignId: camp, CharacterId: c, ItemId: torch.GetId(), IdempotencyKey: nextKey()}))
			if connect.CodeOf(err) == connect.CodeFailedPrecondition {
				return nil
			}
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"SpendCharges", func(u *user, c string) error {
			_, err := u.inventory.SpendCharges(ctx, connect.NewRequest(&charactersv1.SpendChargesRequest{CampaignId: camp, CharacterId: c, ItemId: torch.GetId(), Charges: 1, IdempotencyKey: nextKey()}))
			if connect.CodeOf(err) == connect.CodeFailedPrecondition {
				return nil
			}
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"RecoverAmmunition", func(u *user, c string) error {
			_, err := u.inventory.RecoverAmmunition(ctx, connect.NewRequest(&charactersv1.RecoverAmmunitionRequest{CampaignId: camp, CharacterId: c, ItemId: torch.GetId(), Count: 1, IdempotencyKey: nextKey()}))
			if connect.CodeOf(err) == connect.CodeFailedPrecondition {
				return nil
			}
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"PreviewTextToItems", func(u *user, c string) error {
			_, err := u.inventory.PreviewTextToItems(ctx, connect.NewRequest(&charactersv1.PreviewTextToItemsRequest{CampaignId: camp, CharacterId: c, ItemId: torch.GetId()}))
			if connect.CodeOf(err) == connect.CodeInvalidArgument { // a torch is not a text line: the caller got in
				return nil
			}
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"ConfirmTextToItems", func(u *user, c string) error {
			_, err := u.inventory.ConfirmTextToItems(ctx, connect.NewRequest(&charactersv1.ConfirmTextToItemsRequest{CampaignId: camp, CharacterId: c, ItemId: torch.GetId(), Lines: []*charactersv1.TextProposal{{Name: "x", Quantity: 1}}, IdempotencyKey: nextKey()}))
			if connect.CodeOf(err) == connect.CodeInvalidArgument {
				return nil
			}
			return err
		}, [6]connect.Code{ok, ok, nf, nf, un, nf}},
		{"RegainCharges", func(u *user, _ string) error {
			_, err := u.inventory.RegainCharges(ctx, connect.NewRequest(&charactersv1.RegainChargesRequest{CampaignId: camp, IdempotencyKey: nextKey()}))
			return err
		}, [6]connect.Code{ok, pd, pd, nf, un, nf}},
		{"UpdateItem", func(u *user, c string) error {
			q := int32(1)
			_, err := u.inventory.UpdateItem(ctx, connect.NewRequest(&charactersv1.UpdateItemRequest{CampaignId: camp, CharacterId: c, ItemId: ring.GetId(), Quantity: &q, IdempotencyKey: nextKey()}))
			return err
		}, [6]connect.Code{ok, pd, nf, nf, un, nf}},
		{"ListItemLog", func(u *user, _ string) error {
			_, err := u.inventory.ListItemLog(ctx, connect.NewRequest(&charactersv1.ListItemLogRequest{CampaignId: camp}))
			return err
		}, [6]connect.Code{ok, ok, ok, nf, un, nf}},
	}
	covered := map[string]bool{}
	for _, r := range rows {
		covered[r.method] = true
	}
	service := charactersv1.File_meurpg_characters_v1_inventory_service_proto.Services().ByName("InventoryService")
	for i := range service.Methods().Len() {
		if name := string(service.Methods().Get(i).Name()); !covered[name] {
			t.Errorf("InventoryService.%s is missing from the authorization matrix", name)
		}
	}
	_ = pendingPC
	for _, r := range rows {
		for i, c := range callers {
			// The pending member asks about their own character where the row has a character.
			character := id
			if c.name == "pending" {
				character = pendingPC.GetId()
			}
			err := r.call(c.u, character)
			want := r.want[i]
			if got := connect.CodeOf(err); err == nil && want != ok || err != nil && got != want {
				if err != nil && want == ok && (connect.CodeOf(err) == connect.CodeFailedPrecondition || connect.CodeOf(err) == connect.CodeInvalidArgument) {
					continue // it got past authorization to a rule of the call
				}
				t.Errorf("%s as %s: %v, want %v", r.method, c.name, err, want)
			}
		}
	}
}

// RN-10: what an unidentified item is stays with the master. The canary is its look, which the
// owner and the master may read and nobody else; the true name, key and numbers are the master's.
func TestUnidentifiedItemLeaksNothingToThePlayer(t *testing.T) {
	t.Parallel()
	tb := newInvTable(t)
	id := tb.pensA.GetId()
	const canary = "LEAKCANARY-item-1"
	tb.give(t, tb.master, id,
		&charactersv1.ItemGrant{CatalogKey: "item:wand-of-fireballs", Unidentified: true, Look: "Varinha " + canary},
		&charactersv1.ItemGrant{CatalogKey: "item:ring-of-protection", Unidentified: true, Look: "Anel " + canary})
	secrets := []string{"wand-of-fireballs", "ring-of-protection", "Bolas de Fogo", "Anel de proteção", "bolas de fogo", "proteção", "requires_attunement", "charges_max"}
	dump := func(u *user) string {
		var b strings.Builder
		b.WriteString(tb.view(t, u, id).String())
		log, err := u.inventory.ListItemLog(t.Context(), connect.NewRequest(&charactersv1.ListItemLogRequest{CampaignId: tb.campaign}))
		if err != nil {
			t.Fatalf("ListItemLog() error = %v", err)
		}
		b.WriteString(log.Msg.String())
		b.WriteString(u.get(t, tb.campaign, id).String())
		return b.String()
	}
	owner := dump(tb.ana)
	if !strings.Contains(owner, canary) {
		t.Fatal("positive control failed: the owner cannot read the look")
	}
	for _, s := range secrets {
		if strings.Contains(owner, s) {
			t.Errorf("the owner's views carry %q of an unidentified item", s)
		}
	}
	// A positive control for each secret: the master reads them.
	master := dump(tb.master)
	for _, s := range []string{"wand-of-fireballs", "ring-of-protection"} {
		if !strings.Contains(master, s) {
			t.Errorf("positive control failed: the master's views miss %q", s)
		}
	}
	// Another player: no way into the owner's inventory; the table log shows an unidentified item
	// as it looks, which the whole table saw, and never as it is.
	_, err := tb.bruno.inventory.GetInventory(t.Context(), connect.NewRequest(&charactersv1.GetInventoryRequest{CampaignId: tb.campaign, CharacterId: id}))
	wantCode(t, "GetInventory as another player", err, connect.CodeNotFound)
	log, err := tb.bruno.inventory.ListItemLog(t.Context(), connect.NewRequest(&charactersv1.ListItemLogRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatal(err)
	}
	if got := log.Msg.String(); strings.Contains(got, "wand-of-fireballs") || strings.Contains(got, "Bolas de Fogo") || strings.Contains(got, "proteção") {
		t.Errorf("another player's views carry the item: %s", got)
	}
	// The stream hint names the character and no item.
	for _, p := range tb.host.published {
		if strings.Contains(p, canary) {
			t.Errorf("hint %q names an item", p)
		}
	}
}
