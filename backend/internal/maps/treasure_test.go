package maps

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// MR-044: the treasure generator. The rules of the
// tables are tested in package rules; these tests prove what the server does with
// them: the party's level, who may ask, "Pôr no mapa" and what a player never reads.

func treasureRules(t testing.TB) *rules.Content {
	t.Helper()
	c, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	return c
}

func (u *user) generateTreasure(req *mapsv1.GenerateTreasureRequest) (*mapsv1.Treasure, error) {
	res, err := u.treasure.GenerateTreasure(u.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetTreasure(), nil
}

func (u *user) placeTreasure(req *mapsv1.PlaceTreasureRequest) (*mapsv1.PlaceTreasureResponse, error) {
	res, err := u.treasure.PlaceTreasure(u.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// treasureTable is newScenes' table with a grid on the map: Ana's Pensantus is a
// level 3 wizard and Caio's Toren a level 1 one.
func treasureTable(t *testing.T) *scenes {
	t.Helper()
	s := newScenes(t, false)
	s.master.mustSetGrid(s.campaign, s.mapID, 20)
	return s
}

func (s *scenes) place(edit func(*mapsv1.PlaceTreasureRequest)) (*mapsv1.PlaceTreasureResponse, error) {
	req := &mapsv1.PlaceTreasureRequest{
		CampaignId: s.campaign, MapId: s.mapID, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: 4,
		Seed: proto.Uint64(2209), Column: 7, Row: 5, IdempotencyKey: "dialogo-1", ContentVersion: treasureRules(s.h.t).Version(),
	}
	if edit != nil {
		edit(req)
	}
	return s.master.placeTreasure(req)
}

func (s *scenes) pointCount() int {
	s.h.t.Helper()
	return len(s.master.mustGetMap(s.campaign, s.mapID).GetPoints())
}

// MR-044, "individual" and "de covil": the master generates a treasure for the
// party's lowest living level, or for the level they give, and the same request
// and seed give the same treasure.
func TestMR044_GenerateAsTheMaster(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	m := s.master

	party, err := m.treasure.GetTreasureParty(t.Context(), connect.NewRequest(&mapsv1.GetTreasurePartyRequest{CampaignId: s.campaign}))
	if err != nil {
		t.Fatal(err)
	}
	if p := party.Msg; p.GetLivingCount() != 2 || p.GetLowestLevel() != 1 || p.GetHighestLevel() != 3 {
		t.Errorf("GetTreasureParty() = %v, want 2 characters from level 1 to 3", p)
	}

	// No party_level: the lowest living level, 1.
	hoard := mapsv1.TreasureMode_TREASURE_MODE_HOARD
	a, err := m.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: s.campaign, Mode: hoard, Seed: proto.Uint64(77)})
	if err != nil {
		t.Fatalf("GenerateTreasure(hoard) error = %v", err)
	}
	if a.GetPartyLevel() != 1 || a.GetSeed() != 77 || a.GetMode() != hoard {
		t.Errorf("treasure = level %d, seed %d, mode %v; want the lowest level, the seed and the mode back", a.GetPartyLevel(), a.GetSeed(), a.GetMode())
	}
	// The same request and seed: the same treasure. Another seed: another.
	b, _ := m.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: s.campaign, Mode: hoard, Seed: proto.Uint64(77)})
	if !proto.Equal(a, b) {
		t.Error("the same request and seed gave two treasures")
	}
	// What the server says is what the rules generate, with the totals adding up.
	want, err := treasureRules(t).GenerateTreasure(rules.TreasureHoard, 1, 77)
	if err != nil {
		t.Fatal(err)
	}
	if int(a.GetGoldPo()) != want.GoldPO || int(a.GetItemsPo()) != want.ItemsPO || len(a.GetItems()) != len(want.Items) {
		t.Errorf("the answer (%d PO, %d in items) is not the generator's (%d, %d)", a.GetGoldPo(), a.GetItemsPo(), want.GoldPO, want.ItemsPO)
	}
	if a.GetGoldPo() != a.GetCoinsPo()+a.GetGemsPo()+a.GetArtPo() {
		t.Errorf("gold_po %d is not the coins %d + the gems %d + the art %d", a.GetGoldPo(), a.GetCoinsPo(), a.GetGemsPo(), a.GetArtPo())
	}
	// The items carry what the screen shows.
	for _, it := range a.GetItems() {
		if it.GetNamePt() == "" || it.GetValuePo() <= 0 || it.GetRarity() == mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_ARTIFACT || it.GetRarity() == mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_VARIES {
			t.Errorf("a bad item of the treasure: %v", it)
		}
	}

	// Individual: coins only. The level the master gives wins over the party's.
	ind, err := m.generateTreasure(&mapsv1.GenerateTreasureRequest{
		CampaignId: s.campaign, Mode: mapsv1.TreasureMode_TREASURE_MODE_INDIVIDUAL, PartyLevel: proto.Int32(17), Seed: proto.Uint64(5148),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ind.GetPartyLevel() != 17 || len(ind.GetCoins()) == 0 || len(ind.GetGems())+len(ind.GetArt())+len(ind.GetItems()) != 0 || ind.GetGoldPo() != ind.GetCoinsPo() {
		t.Errorf("an individual treasure of level 17 = %v, want coins only", ind)
	}

	// No seed: the server draws one, and returns it.
	one, _ := m.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: s.campaign, Mode: hoard})
	two, _ := m.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: s.campaign, Mode: hoard})
	if one.GetSeed() == two.GetSeed() {
		t.Error("two requests with no seed got the same seed")
	}

	// Refusals.
	_, err = m.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: s.campaign})
	wantCode(t, "no mode", err, connect.CodeInvalidArgument)
	for _, level := range []int32{0, 21, -3} {
		_, err = m.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: s.campaign, Mode: hoard, PartyLevel: new(level)})
		wantCode(t, "party level out of range", err, connect.CodeInvalidArgument)
	}
}

// The party's level comes from the living player characters only: a dead one does not count, and
// a campaign with none asks the master for a level.
func TestMR044_PartyLevelComesFromTheLivingCharacters(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Mestre"), h.newUser("Ana")
	campaign := h.newCampaign(master, ana)
	hoard := mapsv1.TreasureMode_TREASURE_MODE_HOARD

	_, err := master.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: campaign, Mode: hoard})
	wantCode(t, "no party and no level", err, connect.CodeFailedPrecondition)
	if ce, ok := errors.AsType[*connect.Error](err); !ok || !hasTreasureBlocked(ce, mapsv1.TreasureBlockedReason_TREASURE_BLOCKED_REASON_NO_PARTY) {
		t.Errorf("the refusal has no TreasureBlocked NO_PARTY detail: %v", err)
	}
	party, err := master.treasure.GetTreasureParty(t.Context(), connect.NewRequest(&mapsv1.GetTreasurePartyRequest{CampaignId: campaign}))
	if err != nil || party.Msg.GetLivingCount() != 0 || party.Msg.GetLowestLevel() != 0 {
		t.Errorf("GetTreasureParty() with no party = %v, %v", party, err)
	}
	// An NPC is not the party.
	master.createCharacter(campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	_, err = master.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: campaign, Mode: hoard})
	wantCode(t, "only an NPC", err, connect.CodeFailedPrecondition)
	// With a level, it works without a party.
	if tr, err := master.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: campaign, Mode: hoard, PartyLevel: proto.Int32(5)}); err != nil || tr.GetPartyLevel() != 5 {
		t.Errorf("with a level, no party = %v, %v", tr, err)
	}

	// A character who died leaves the party.
	pens := ana.pensantus(campaign)
	toren := h.newUser("Caio")
	h.join(master, campaign, false, toren)
	toren.createCharacter(campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren")
	if lvl := lowestLevel(t, master, campaign); lvl != 1 {
		t.Errorf("lowest level with Pensantus (3) and Toren (1) = %d, want 1", lvl)
	}
	if _, err := h.pool.Exec(t.Context(), `UPDATE characters SET status = 'dead', died_at = now() WHERE campaign_id = $1 AND name = 'Toren'`, campaign); err != nil {
		t.Fatal(err)
	}
	if lvl := lowestLevel(t, master, campaign); lvl != 3 {
		t.Errorf("lowest level after Toren died = %d, want Pensantus's 3 (%s)", lvl, pens.GetName())
	}
}

func lowestLevel(t *testing.T, u *user, campaign string) int32 {
	t.Helper()
	tr, err := u.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: campaign, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD})
	if err != nil {
		t.Fatalf("GenerateTreasure() error = %v", err)
	}
	return tr.GetPartyLevel()
}

func hasTreasureBlocked(ce *connect.Error, reason mapsv1.TreasureBlockedReason) bool {
	for _, d := range ce.Details() {
		if v, err := d.Value(); err == nil {
			if b, ok := v.(*mapsv1.TreasureBlocked); ok && b.GetReason() == reason {
				return true
			}
		}
	}
	return false
}

// RN-10 and the authorization matrix: every method of TreasureService is the master's. A
// player, a pending member and a stranger get not_found; nobody signed in gets
// unauthenticated.
func TestTreasureAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	pending := s.h.newUser("Pendente")
	s.h.join(s.master, s.campaign, true, pending)
	stranger := s.h.newUser("Estranho")

	calls := map[string]func(u *user) error{
		"GetTreasureParty": func(u *user) error {
			_, err := u.treasure.GetTreasureParty(t.Context(), connect.NewRequest(&mapsv1.GetTreasurePartyRequest{CampaignId: s.campaign}))
			return err
		},
		"GenerateTreasure": func(u *user) error {
			_, err := u.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: s.campaign, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: proto.Int32(4)})
			return err
		},
		"GetMagicItem": func(u *user) error {
			_, err := u.treasure.GetMagicItem(t.Context(), connect.NewRequest(&mapsv1.GetMagicItemRequest{CampaignId: s.campaign, Key: "item:ring-of-protection"}))
			return err
		},
		"PlaceTreasure": func(u *user) error {
			_, err := u.placeTreasure(&mapsv1.PlaceTreasureRequest{
				CampaignId: s.campaign, MapId: s.mapID, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: 4, Seed: proto.Uint64(1), Column: 1, Row: 1, IdempotencyKey: "k-" + u.id, ContentVersion: treasureRules(t).Version(),
			})
			return err
		},
	}
	for name, call := range calls {
		wantCode(t, name+" as a player", call(s.ana), connect.CodeNotFound)
		wantCode(t, name+" as a pending member", call(pending), connect.CodeNotFound)
		wantCode(t, name+" as a stranger", call(stranger), connect.CodeNotFound)
		wantCode(t, name+" signed out", call(s.h.anonymous()), connect.CodeUnauthenticated)
		wantCode(t, name+" as the master", call(s.master), allowed)
	}
	// A player's call changed nothing: the master's own PlaceTreasure made the only point.
	if n := s.pointCount(); n != 2 { // the scene of newScenes and the master's treasure
		t.Errorf("the map has %d points, want the scene and one treasure", n)
	}
}

// MR-044, "Ver descrição": rarity, value with the 2024 label, attunement and the
// SRD's text in English.
func TestMR044_GetMagicItem(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	get := func(key string) (*mapsv1.GetMagicItemResponse, error) {
		res, err := s.master.treasure.GetMagicItem(t.Context(), connect.NewRequest(&mapsv1.GetMagicItemRequest{CampaignId: s.campaign, Key: key}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	ring, err := get("item:ring-of-protection")
	if err != nil {
		t.Fatal(err)
	}
	if ring.GetNamePt() != "Anel de proteção" || ring.GetName() != "Ring of Protection" || ring.GetRarity() != mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_RARE ||
		!ring.GetAttunement() || ring.GetValuePo() != 4000 || ring.GetHalved() || ring.GetPriceless() || ring.GetValueLabel() != "Valores do SRD 5.2.1 (regras de 2024)" {
		t.Errorf("the Ring of Protection = %v", ring)
	}
	if len(ring.GetDescription()) < 2 || !strings.Contains(strings.Join(ring.GetDescription(), " "), "+1 bonus to AC") {
		t.Errorf("the SRD text = %v", ring.GetDescription())
	}

	// The Portuguese text rides next to the English, paragraph for paragraph; an item
	// without one says so.
	bag, err := get("item:bag-of-holding")
	if err != nil {
		t.Fatal(err)
	}
	if bag.GetDescriptionPtMissing() || len(bag.GetDescriptionPt()) != len(bag.GetDescription()) {
		t.Errorf("the Bag of Holding's Portuguese text = %v (missing %v), want %d paragraphs", bag.GetDescriptionPt(), bag.GetDescriptionPtMissing(), len(bag.GetDescription()))
	}
	if ring.GetDescriptionPtMissing() != (len(ring.GetDescriptionPt()) == 0) {
		t.Errorf("the Ring of Protection: missing %v with %d Portuguese paragraphs", ring.GetDescriptionPtMissing(), len(ring.GetDescriptionPt()))
	}

	potion, _ := get("item:potion-of-healing-common")
	if potion.GetValuePo() != 50 || !potion.GetHalved() || !potion.GetConsumable() {
		t.Errorf("a consumable of rarity common = %v, want 50 PO halved", potion)
	}
	scroll, _ := get("item:spell-scroll-3rd")
	if scroll.GetValuePo() != 400 || scroll.GetHalved() || !scroll.GetSpellScroll() || !scroll.GetConsumable() {
		t.Errorf("a 3rd level scroll = %v, want 400 PO, whole", scroll)
	}
	family, _ := get("item:potion-of-healing")
	if family.ValuePo != nil || family.GetPriceless() || family.GetRarity() != mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_VARIES || len(family.GetVariants()) != 4 {
		t.Errorf("the Potion of Healing family = %v, want no value and its 4 variants", family)
	}
	variant, _ := get("item:potion-of-healing-greater")
	if variant.GetVariantOf() != "item:potion-of-healing" || variant.GetValuePo() != 200 {
		t.Errorf("a variant = %v", variant)
	}
	var artifact *mapsv1.GetMagicItemResponse
	for _, e := range treasureRules(t).MagicItems() {
		if e.Rarity == rules.RarityArtifact {
			if artifact, err = get(e.Key); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if artifact == nil || !artifact.GetPriceless() || artifact.ValuePo != nil {
		t.Errorf("an artifact = %v, want priceless with no value", artifact)
	}
	_, err = get("item:nope")
	wantCode(t, "an unknown item", err, connect.CodeNotFound)
	_, err = get("spell:fireball")
	wantCode(t, "a key that is not an item", err, connect.CodeNotFound)
}

// MR-044, "Pôr no mapa": a hidden TREASURE point, its value the gold of the treasure
// (never the items), the description in Portuguese, rolled again by the server.
func TestMR044_PlaceTreasureMakesAHiddenTreasurePoint(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	before := s.pointCount()

	res, err := s.place(nil)
	if err != nil {
		t.Fatalf("PlaceTreasure() error = %v", err)
	}
	want, _ := treasureRules(t).GenerateTreasure(rules.TreasureHoard, 4, 2209)
	p := res.GetPoint()
	if p.GetKind() != mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE || p.GetRevealed() || p.GetName() != "Tesouro de covil" {
		t.Errorf("point = kind %v, revealed %v, name %q; want a hidden treasure named Tesouro de covil", p.GetKind(), p.GetRevealed(), p.GetName())
	}
	if int(p.GetTreasureValuePo()) != want.GoldPO || int(res.GetTreasure().GetGoldPo()) != want.GoldPO || want.GoldPO == 0 {
		t.Errorf("the point is worth %d PO, the treasure's gold is %d (rules: %d); never the items (%d)", p.GetTreasureValuePo(), res.GetTreasure().GetGoldPo(), want.GoldPO, want.ItemsPO)
	}
	if int(p.GetTreasureValuePo()) == want.GoldPO+want.ItemsPO {
		t.Error("the items were counted in the point's gold")
	}
	// The middle of the square (7, 5) of the 20 x 15 grid.
	if p.GetXBp() != (2*7+1)*10000/40 || p.GetYBp() != (2*5+1)*10000/30 {
		t.Errorf("the point is at %d, %d, want the middle of square 7, 5", p.GetXBp(), p.GetYBp())
	}
	if p.GetDescription() != treasureDescription(want) {
		t.Errorf("description = %q, want %q", p.GetDescription(), treasureDescription(want))
	}
	for _, it := range want.Items {
		if !strings.Contains(p.GetDescription(), it.NamePT) {
			t.Errorf("the description lacks %q: %s", it.NamePT, p.GetDescription())
		}
	}
	if s.pointCount() != before+1 {
		t.Errorf("the map has %d points, want %d", s.pointCount(), before+1)
	}

	// A name of the master's, and the individual mode.
	ind, err := s.place(func(r *mapsv1.PlaceTreasureRequest) {
		r.Mode, r.Name, r.IdempotencyKey, r.Column = mapsv1.TreasureMode_TREASURE_MODE_INDIVIDUAL, new("A bolsa do bandido"), "dialogo-2", 8
	})
	if err != nil || ind.GetPoint().GetName() != "A bolsa do bandido" || strings.Contains(ind.GetPoint().GetDescription(), "Itens mágicos") {
		t.Errorf("an individual treasure = %v, %v", ind.GetPoint(), err)
	}
	if def, err := s.place(func(r *mapsv1.PlaceTreasureRequest) {
		r.Mode, r.IdempotencyKey = mapsv1.TreasureMode_TREASURE_MODE_INDIVIDUAL, "dialogo-3"
	}); err != nil || def.GetPoint().GetName() != "Tesouro individual" {
		t.Errorf("the default name of an individual treasure = %v, %v", def.GetPoint().GetName(), err)
	}

	// The point is the master's own, and the master edits it as any treasure.
	if _, err := s.master.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: p.GetId(), TreasureValuePo: proto.Int32(10)}); err != nil {
		t.Errorf("editing a placed treasure: %v", err)
	}
}

// What the app must send, and what the server refuses.
func TestMR044_PlaceTreasureRefusals(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	cases := map[string]struct {
		edit  func(*mapsv1.PlaceTreasureRequest)
		code  connect.Code
		field string
	}{
		"no mode":           {func(r *mapsv1.PlaceTreasureRequest) { r.Mode = 0 }, connect.CodeInvalidArgument, "mode"},
		"no level":          {func(r *mapsv1.PlaceTreasureRequest) { r.PartyLevel = 0 }, connect.CodeInvalidArgument, "party_level"},
		"level 21":          {func(r *mapsv1.PlaceTreasureRequest) { r.PartyLevel = 21 }, connect.CodeInvalidArgument, "party_level"},
		"no seed":           {func(r *mapsv1.PlaceTreasureRequest) { r.Seed = nil }, connect.CodeInvalidArgument, "seed"},
		"no key":            {func(r *mapsv1.PlaceTreasureRequest) { r.IdempotencyKey = "" }, connect.CodeInvalidArgument, "idempotency_key"},
		"a key of 65":       {func(r *mapsv1.PlaceTreasureRequest) { r.IdempotencyKey = strings.Repeat("k", 65) }, connect.CodeInvalidArgument, "idempotency_key"},
		"a name too long":   {func(r *mapsv1.PlaceTreasureRequest) { r.Name = new(strings.Repeat("n", 81)) }, connect.CodeInvalidArgument, "name"},
		"an empty name":     {func(r *mapsv1.PlaceTreasureRequest) { r.Name = new("  ") }, connect.CodeInvalidArgument, "name"},
		"a column off":      {func(r *mapsv1.PlaceTreasureRequest) { r.Column = 20 }, connect.CodeInvalidArgument, "column"},
		"a row off":         {func(r *mapsv1.PlaceTreasureRequest) { r.Row = 15 }, connect.CodeInvalidArgument, "row"},
		"a negative square": {func(r *mapsv1.PlaceTreasureRequest) { r.Row = -1 }, connect.CodeInvalidArgument, "row"},
		"no map":            {func(r *mapsv1.PlaceTreasureRequest) { r.MapId = "" }, connect.CodeNotFound, ""},
		"another map":       {func(r *mapsv1.PlaceTreasureRequest) { r.MapId = "00000000-0000-4000-8000-000000000000" }, connect.CodeNotFound, ""},
	}
	before := s.pointCount()
	for name, c := range cases {
		_, err := s.place(c.edit)
		wantCode(t, name, err, c.code)
		if got := treasureInvalidField(err); got != c.field {
			t.Errorf("%s: the refusal names the field %q, want %q", name, got, c.field)
		}
	}
	if s.pointCount() != before {
		t.Errorf("a refused call made a point: %d points, want %d", s.pointCount(), before)
	}

	// A map with no grid: the point has no square.
	bare := s.master.createMap(s.campaign, "Sem grade", s.master.newImage(s.campaign))
	_, err := s.place(func(r *mapsv1.PlaceTreasureRequest) { r.MapId = bare.GetId() })
	wantCode(t, "a map with no grid", err, connect.CodeFailedPrecondition)
	if ce, ok := errors.AsType[*connect.Error](err); !ok || !hasMapBlocked(ce, mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_NO_GRID) {
		t.Errorf("the refusal has no MapBlocked NO_GRID detail: %v", err)
	}

	// A map of another campaign is not found.
	h := s.h
	otherCampaign := h.newCampaign(s.master)
	foreign := s.master.createMap(otherCampaign, "De outra mesa", s.master.newImage(otherCampaign))
	_, err = s.place(func(r *mapsv1.PlaceTreasureRequest) { r.MapId = foreign.GetId() })
	wantCode(t, "a map of another campaign", err, connect.CodeNotFound)
}

// PlaceTreasure checks in a fixed order: the first fault is the one the caller reads, so
// a call with two faults pins which check runs first.
func TestMR044_PlaceTreasureRefusalOrder(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	type edit = func(*mapsv1.PlaceTreasureRequest)
	badMode := func(r *mapsv1.PlaceTreasureRequest) { r.Mode = 0 }
	badLevel := func(r *mapsv1.PlaceTreasureRequest) { r.PartyLevel = 0 }
	noSeed := func(r *mapsv1.PlaceTreasureRequest) { r.Seed = nil }
	badKey := func(r *mapsv1.PlaceTreasureRequest) { r.IdempotencyKey = "" }
	noVersion := func(r *mapsv1.PlaceTreasureRequest) { r.ContentVersion = "" }
	oldVersion := func(r *mapsv1.PlaceTreasureRequest) { r.ContentVersion = "srd51@old+fx.1" }
	badName := func(r *mapsv1.PlaceTreasureRequest) { r.Name = new("  ") }
	badColumn := func(r *mapsv1.PlaceTreasureRequest) { r.Column = 20 }
	badRow := func(r *mapsv1.PlaceTreasureRequest) { r.Row = 15 }
	noMap := func(r *mapsv1.PlaceTreasureRequest) { r.MapId = "" }
	all := func(edits ...edit) edit {
		return func(r *mapsv1.PlaceTreasureRequest) {
			for _, e := range edits {
				e(r)
			}
		}
	}
	cases := []struct {
		name  string
		edit  edit
		code  connect.Code
		field string
	}{
		{"a missing map before the mode", all(noMap, badMode), connect.CodeNotFound, ""},
		{"the mode before the level", all(badMode, badLevel), connect.CodeInvalidArgument, "mode"},
		{"the level before the seed", all(badLevel, noSeed), connect.CodeInvalidArgument, "party_level"},
		{"the seed before the key", all(noSeed, badKey), connect.CodeInvalidArgument, "seed"},
		{"the key before the version", all(badKey, noVersion), connect.CodeInvalidArgument, "idempotency_key"},
		{"the empty version before the name", all(noVersion, badName), connect.CodeInvalidArgument, "content_version"},
		{"an old version before the name", all(oldVersion, badName), connect.CodeFailedPrecondition, ""},
		{"the name before the square", all(badName, badColumn), connect.CodeInvalidArgument, "name"},
		{"the column before the row", all(badColumn, badRow), connect.CodeInvalidArgument, "column"},
	}
	for _, c := range cases {
		_, err := s.place(c.edit)
		wantCode(t, c.name, err, c.code)
		if got := treasureInvalidField(err); got != c.field {
			t.Errorf("%s: the refusal names the field %q, want %q", c.name, got, c.field)
		}
	}

	// A player is refused as a stranger before anything else is read.
	_, err := s.ana.placeTreasure(&mapsv1.PlaceTreasureRequest{CampaignId: s.campaign, MapId: "", Mode: 0})
	wantCode(t, "a player with a bad request", err, connect.CodeNotFound)

	// The grid is read before the square, and a replay comes before both the grid and the
	// cap: a map with no grid refuses a square, and a bad square is refused before the cap.
	bare := s.master.createMap(s.campaign, "Sem grade", s.master.newImage(s.campaign))
	_, err = s.place(func(r *mapsv1.PlaceTreasureRequest) { r.MapId = bare.GetId(); badColumn(r) })
	wantCode(t, "no grid before a square off the grid", err, connect.CodeFailedPrecondition)
	s.h.svc.maxPoints = 1
	_, err = s.place(badColumn)
	wantCode(t, "a square off the grid at the cap", err, connect.CodeInvalidArgument)
	if got := treasureInvalidField(err); got != "column" {
		t.Errorf("a square off the grid at the cap names %q, want column", got)
	}
	_, err = s.place(nil)
	wantCode(t, "a good square at the cap", err, connect.CodeResourceExhausted)
}

// treasureInvalidField is the field a TreasureInvalidField detail names, "" without one.
func treasureInvalidField(err error) string {
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		return ""
	}
	for _, d := range ce.Details() {
		if v, err := d.Value(); err == nil {
			if f, ok := v.(*mapsv1.TreasureInvalidField); ok {
				return f.GetField()
			}
		}
	}
	return ""
}

func hasMapBlocked(ce *connect.Error, reason mapsv1.MapBlockedReason) bool {
	for _, d := range ce.Details() {
		if v, err := d.Value(); err == nil {
			if b, ok := v.(*mapsv1.MapBlocked); ok && b.GetReason() == reason {
				return true
			}
		}
	}
	return false
}

// MR-044: "Pôr no mapa" is idempotent. The same key for the same request returns the
// point the first call made; the key is unique in the campaign and is kept with a hash of
// the whole request, so any other request with it is refused, and a retry never returns a
// treasure the point does not hold.
func TestMR044_PlaceTreasureIsIdempotent(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	first, err := s.place(nil)
	if err != nil {
		t.Fatal(err)
	}
	before := s.pointCount()
	again, err := s.place(nil)
	if err != nil {
		t.Fatalf("PlaceTreasure() again error = %v", err)
	}
	if again.GetPoint().GetId() != first.GetPoint().GetId() || s.pointCount() != before {
		t.Errorf("a retry made another point: %s and %s, %d points", first.GetPoint().GetId(), again.GetPoint().GetId(), s.pointCount())
	}
	if !proto.Equal(again.GetTreasure(), first.GetTreasure()) {
		t.Error("a retry gave another treasure")
	}

	// The same key with any other request is refused, whatever differs.
	other := map[string]func(*mapsv1.PlaceTreasureRequest){
		"another square": func(r *mapsv1.PlaceTreasureRequest) { r.Column = 3 },
		"another seed":   func(r *mapsv1.PlaceTreasureRequest) { r.Seed = proto.Uint64(2210) },
		"another mode":   func(r *mapsv1.PlaceTreasureRequest) { r.Mode = mapsv1.TreasureMode_TREASURE_MODE_INDIVIDUAL },
		"another level":  func(r *mapsv1.PlaceTreasureRequest) { r.PartyLevel = 5 },
		"another name":   func(r *mapsv1.PlaceTreasureRequest) { r.Name = new("Outro nome") },
	}
	for name, edit := range other {
		_, err := s.place(edit)
		wantCode(t, "the key with "+name, err, connect.CodeInvalidArgument)
		if got := treasureInvalidField(err); got != "idempotency_key" {
			t.Errorf("the key with %s: the refusal names the field %q, want idempotency_key", name, got)
		}
	}
	// Another map of the campaign: the key is the campaign's, so it is refused there too.
	second := s.master.createMap(s.campaign, "Outro mapa", s.master.newImage(s.campaign))
	s.master.mustSetGrid(s.campaign, second.GetId(), 20)
	_, err = s.place(func(r *mapsv1.PlaceTreasureRequest) { r.MapId = second.GetId() })
	wantCode(t, "the key on another map of the campaign", err, connect.CodeInvalidArgument)
	if s.pointCount() != before {
		t.Errorf("a refused retry made a point: %d, want %d", s.pointCount(), before)
	}
	// A new key is a new treasure, and the key of another campaign is another key.
	if res, err := s.place(func(r *mapsv1.PlaceTreasureRequest) { r.IdempotencyKey = "dialogo-2" }); err != nil || res.GetPoint().GetId() == first.GetPoint().GetId() {
		t.Errorf("another key = %v, %v, want a new point", res.GetPoint().GetId(), err)
	}
	otherCampaign := s.h.newCampaign(s.master)
	foreign := s.master.createMap(otherCampaign, "De outra mesa", s.master.newImage(otherCampaign))
	s.master.mustSetGrid(otherCampaign, foreign.GetId(), 20)
	if res, err := s.place(func(r *mapsv1.PlaceTreasureRequest) { r.CampaignId, r.MapId = otherCampaign, foreign.GetId() }); err != nil || res.GetPoint().GetMapId() != foreign.GetId() {
		t.Errorf("the same key in another campaign = %v, %v, want a point there", res.GetPoint(), err)
	}

	// A replay returns the point as it is now: edited and revealed by the master.
	if _, err := s.master.updatePoint(&mapsv1.UpdateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: first.GetPoint().GetId(), Name: new("Renomeado")}); err != nil {
		t.Fatal(err)
	}
	if replay, err := s.place(nil); err != nil || replay.GetPoint().GetName() != "Renomeado" {
		t.Errorf("a replay after an edit = %v, %v, want the point as it is now", replay.GetPoint().GetName(), err)
	}
}

// Four calls at once with a new key make one point: the others take the
// ON CONFLICT DO NOTHING branch and read the winner's row.
func TestMR044_PlaceTreasureRacingCallsMakeOnePoint(t *testing.T) {
	t.Parallel()
	const racers = 4
	dbtest.PoolSize(t, racers)
	s := treasureTable(t)
	before := s.pointCount()
	type result struct {
		id  string
		err error
	}
	start := dbtest.NewBarrier(racers)
	out := make(chan result, racers)
	for range racers {
		go func() {
			start.Wait()
			res, err := s.place(func(r *mapsv1.PlaceTreasureRequest) { r.IdempotencyKey = "corrida" })
			if err != nil {
				out <- result{err: err}
				return
			}
			out <- result{id: res.GetPoint().GetId()}
		}()
	}
	ids := map[string]bool{}
	for range racers {
		r := <-out
		if r.err != nil {
			t.Fatalf("a racing PlaceTreasure failed: %v", r.err)
		}
		ids[r.id] = true
	}
	var rows int
	if err := s.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM map_points WHERE create_key = $1`, s.campaign+":corrida").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || rows != 1 || s.pointCount() != before+1 {
		t.Errorf("%d racers gave %d points and %d rows, the map has %d points (was %d); want one", racers, len(ids), rows, s.pointCount(), before)
	}
}

// The 200-point cap (here 3): a new treasure is refused at it, and a replay of one that
// already exists still succeeds.
func TestMR044_PlaceTreasureAtThePointCap(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	s.h.svc.maxPoints = 3 // the scene of newScenes and two treasures
	first, err := s.place(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.place(func(r *mapsv1.PlaceTreasureRequest) { r.IdempotencyKey = "dialogo-2" }); err != nil {
		t.Fatal(err)
	}
	_, err = s.place(func(r *mapsv1.PlaceTreasureRequest) { r.IdempotencyKey = "dialogo-3" })
	wantCode(t, "a treasure at the cap", err, connect.CodeResourceExhausted)
	again, err := s.place(nil)
	if err != nil || again.GetPoint().GetId() != first.GetPoint().GetId() {
		t.Errorf("a replay at the cap = %v, %v, want the first point", again.GetPoint().GetId(), err)
	}
}

// A map calibrated with grid_factor 2: the square is one of the rules' squares, so the
// grid has 16 x 12 of them, and the point goes in the middle of the one asked.
func TestMR044_PlaceTreasureOnACalibratedMap(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	s.master.mustSetCalibration(s.campaign, s.mapID, 8, 2)
	res, err := s.place(func(r *mapsv1.PlaceTreasureRequest) { r.Column, r.Row = 15, 11 })
	if err != nil {
		t.Fatalf("PlaceTreasure(15, 11) on a 16 x 12 grid error = %v", err)
	}
	if p := res.GetPoint(); p.GetXBp() != (2*15+1)*10000/32 || p.GetYBp() != (2*11+1)*10000/24 {
		t.Errorf("the point is at %d, %d, want the middle of the rules' square 15, 11", p.GetXBp(), p.GetYBp())
	}
	_, err = s.place(func(r *mapsv1.PlaceTreasureRequest) { r.Column, r.IdempotencyKey = 16, "outra" })
	wantCode(t, "a column past the 16 rules' squares", err, connect.CodeInvalidArgument)
}

// A treasure generated under another content version is not placed: the same seed would
// now give another treasure.
func TestMR044_PlaceTreasureNeedsTheContentVersion(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	tr, err := s.master.generateTreasure(&mapsv1.GenerateTreasureRequest{CampaignId: s.campaign, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: proto.Int32(4), Seed: proto.Uint64(9)})
	if err != nil {
		t.Fatal(err)
	}
	if tr.GetContentVersion() == "" || tr.GetContentVersion() != treasureRules(t).Version() {
		t.Errorf("content_version = %q, want the server's %q", tr.GetContentVersion(), treasureRules(t).Version())
	}
	_, err = s.place(func(r *mapsv1.PlaceTreasureRequest) { r.ContentVersion = "" })
	wantCode(t, "no content version", err, connect.CodeInvalidArgument)
	if got := treasureInvalidField(err); got != "content_version" {
		t.Errorf("no content version: the refusal names the field %q, want content_version", got)
	}
	_, err = s.place(func(r *mapsv1.PlaceTreasureRequest) { r.ContentVersion = "srd51@old+fx.1" })
	wantCode(t, "an old content version", err, connect.CodeFailedPrecondition)
	if ce, ok := errors.AsType[*connect.Error](err); !ok || !hasTreasureBlocked(ce, mapsv1.TreasureBlockedReason_TREASURE_BLOCKED_REASON_CONTENT_CHANGED) {
		t.Errorf("the refusal has no TreasureBlocked CONTENT_CHANGED detail: %v", err)
	}
	if _, err := s.place(func(r *mapsv1.PlaceTreasureRequest) {
		r.PartyLevel, r.Seed, r.ContentVersion = 4, proto.Uint64(9), tr.GetContentVersion()
	}); err != nil {
		t.Errorf("the version the treasure came with was refused: %v", err)
	}
}

// RN-10: a player never receives the placed treasure, in a read or in the stream,
// until the master reveals the point.
func TestRN10_APlayerNeverSeesAPlacedTreasureUntilRevealed(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	m.mustSetGrid(s.campaign, s.mapID, 20)
	probeMap := m.createMap(s.campaign, "Sonda", m.newImage(s.campaign)).GetId()
	anaWatch, caioWatch := s.ana.watch(s.campaign), s.caio.watch(s.campaign)

	res, err := s.place(func(r *mapsv1.PlaceTreasureRequest) { r.Name = new("TESOURO-SECRETO-X") })
	if err != nil {
		t.Fatal(err)
	}
	point := res.GetPoint()
	s.probe(probeMap)
	if got := anaWatch.drain(probeMap); len(got) != 0 {
		t.Errorf("Ana's stream got %v for a hidden treasure, want nothing", got)
	}
	if got := caioWatch.drain(probeMap); len(got) != 0 {
		t.Errorf("Caio's stream got %v for a hidden treasure, want nothing", got)
	}

	secrets := []string{"TESOURO-SECRETO-X", point.GetId()}
	for _, it := range res.GetTreasure().GetItems() {
		secrets = append(secrets, it.GetNamePt(), it.GetKey())
	}
	for _, p := range res.GetTreasure().GetGems() {
		secrets = append(secrets, p.GetNamePt())
	}
	read := func(u *user) string {
		t.Helper()
		list, err := u.maps.ListMaps(t.Context(), connect.NewRequest(&mapsv1.ListMapsRequest{CampaignId: s.campaign}))
		if err != nil {
			t.Fatal(err)
		}
		return compactJSON(t, list.Msg) + compactJSON(t, u.mustGetMap(s.campaign, s.mapID)) + compactJSON(t, u.mustLayers(s.campaign, s.mapID))
	}
	for name, u := range map[string]*user{"Ana": s.ana, "Caio": s.caio} {
		saw := read(u)
		for _, secret := range secrets {
			if strings.Contains(saw, secret) {
				t.Errorf("%s received %q of a hidden treasure", name, secret)
			}
		}
	}
	// The player has no way to read it either.
	if _, err := s.ana.treasure.GetTreasureParty(t.Context(), connect.NewRequest(&mapsv1.GetTreasurePartyRequest{CampaignId: s.campaign})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a player's GetTreasureParty = %v, want not_found", err)
	}

	// Revealed, the players see the point on the map and nothing of what is inside: the
	// description and the value come with the find (MR-041), as for any treasure.
	m.setPointRevealed(s.campaign, point, true)
	anaWatch.mapChanged(s.mapID)
	seenBy := func(u *user) *mapsv1.MapPoint {
		for _, p := range u.mustGetMap(s.campaign, s.mapID).GetPoints() {
			if p.GetId() == point.GetId() {
				return p
			}
		}
		return nil
	}
	seen := seenBy(s.ana)
	if seen == nil || seen.GetDescription() != "" || seen.GetTreasureValuePo() != 0 {
		t.Fatalf("after the reveal Ana reads %v, want the point with nothing of what is inside", seen)
	}
	s.markFound(point, s.pens.GetId())
	if found := seenBy(s.ana); found == nil || found.GetDescription() != point.GetDescription() || found.GetTreasureValuePo() != point.GetTreasureValuePo() {
		t.Errorf("after the find Ana reads %v, want the description and the value", found)
	}
}

// MR-044 and MR-041: in a gold campaign, the placed point converts to XP in "Voltar
// à cidade" for its gold only; the magic items never become XP.
func TestMR044_AGoldCampaignConvertsTheGoldOnly(t *testing.T) {
	t.Parallel()
	s := treasureTable(t)
	if _, err := s.master.campaigns.SetCampaignXpMode(t.Context(), connect.NewRequest(&campaignsv1.SetCampaignXpModeRequest{CampaignId: s.campaign, XpMode: campaignsv1.XpMode_XP_MODE_GOLD})); err != nil {
		t.Fatalf("SetCampaignXpMode(gold) error = %v", err)
	}
	// A hoard of level 17 with items worth much more than its gold.
	res, err := s.place(func(r *mapsv1.PlaceTreasureRequest) {
		r.PartyLevel, r.Seed = 17, proto.Uint64(31)
	})
	if err != nil {
		t.Fatal(err)
	}
	tr := res.GetTreasure()
	if tr.GetItemsPo() == 0 || tr.GetGoldPo() == 0 {
		t.Fatalf("seed 31 at level 17 gave %d PO of gold and %d of items; the test needs both", tr.GetGoldPo(), tr.GetItemsPo())
	}

	// Found, it waits to be converted: the value is the gold.
	s.markFound(res.GetPoint(), s.pens.GetId())
	list, err := s.master.xp.ListTreasuresToConvert(t.Context(), connect.NewRequest(&progressionv1.ListTreasuresToConvertRequest{CampaignId: s.campaign}))
	if err != nil {
		t.Fatal(err)
	}
	if got := list.Msg.GetTreasures(); len(got) != 1 || got[0].GetPointId() != res.GetPoint().GetId() || got[0].GetValuePo() != tr.GetGoldPo() {
		t.Fatalf("ListTreasuresToConvert() = %v, want the placed treasure at %d PO", got, tr.GetGoldPo())
	}
	award, err := s.master.xp.AwardXP(t.Context(), connect.NewRequest(&progressionv1.AwardXPRequest{
		CampaignId: s.campaign, Mode: progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, Reason: "Voltar à cidade",
		CharacterIds: []string{s.pens.GetId()}, TreasurePointIds: []string{res.GetPoint().GetId()}, IdempotencyKey: uuid.New().String(),
	}))
	if err != nil {
		t.Fatalf("AwardXP(Voltar à cidade) error = %v", err)
	}
	if award.Msg.GetXpEach() != tr.GetGoldPo() {
		t.Errorf("Pensantus got %d XP, want the treasure's gold, %d (its items, %d PO, never count)", award.Msg.GetXpEach(), tr.GetGoldPo(), tr.GetItemsPo())
	}
}

// xpChanged reads the next event, which must be xp_changed.
func (w *watcher) xpChanged() {
	w.t.Helper()
	if ev := w.next(); ev.GetXpChanged() == nil {
		w.t.Fatalf("event = %v, want xp_changed", ev)
	}
}

// MR-041: converting a found treasure into XP, and undoing that award, change what
// the master reads of the map (the converted state), so the master's open maps are
// told; the players read nothing different, so they are told nothing (RN-10), and
// what the event carries is the map's ID alone.
func TestMR041_AConversionAndItsUndoTellTheMastersMapsOnly(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true) // the streams need an open session
	s.master.mustSetGrid(s.campaign, s.mapID, 20)
	m := s.master
	if _, err := m.campaigns.SetCampaignXpMode(t.Context(), connect.NewRequest(&campaignsv1.SetCampaignXpModeRequest{CampaignId: s.campaign, XpMode: campaignsv1.XpMode_XP_MODE_GOLD})); err != nil {
		t.Fatalf("SetCampaignXpMode(gold) error = %v", err)
	}
	probeMap := m.createMap(s.campaign, "Sonda", m.newImage(s.campaign)).GetId()
	res, err := s.place(func(r *mapsv1.PlaceTreasureRequest) {
		r.Name, r.PartyLevel, r.Seed = new("LEAKCANARY-treasure-1"), 17, proto.Uint64(31)
	})
	if err != nil {
		t.Fatal(err)
	}
	point := res.GetPoint()
	m.setPointRevealed(s.campaign, point, true)
	s.markFound(point, s.pens.GetId())
	masterWatch, anaWatch := m.watch(s.campaign), s.ana.watch(s.campaign)

	award, err := m.xp.AwardXP(t.Context(), connect.NewRequest(&progressionv1.AwardXPRequest{
		CampaignId: s.campaign, Mode: progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, Reason: "Voltar à cidade",
		CharacterIds: []string{s.pens.GetId()}, TreasurePointIds: []string{point.GetId()}, IdempotencyKey: uuid.New().String(),
	}))
	if err != nil {
		t.Fatalf("AwardXP(Voltar à cidade) error = %v", err)
	}
	masterWatch.xpChanged() // the funnel sends what the award told, then the steps it collected after the commit (the map's hint)
	masterWatch.mapChanged(s.mapID)
	if _, err := m.xp.UndoLastXPAward(t.Context(), connect.NewRequest(&progressionv1.UndoLastXPAwardRequest{
		CampaignId: s.campaign, IdempotencyKey: uuid.New().String(), ExpectedAwardId: award.Msg.GetAward().GetId(),
	})); err != nil {
		t.Fatalf("UndoLastXPAward() error = %v", err)
	}
	masterWatch.xpChanged() // the funnel sends what the award told, then the steps it collected after the commit (the map's hint)
	masterWatch.mapChanged(s.mapID)

	// The probe is the last change, and reaches the players: what came before it
	// on their stream is all they were sent.
	m.setMapRevealed(s.campaign, probeMap, true)
	sent := anaWatch.drain(probeMap)
	for _, ev := range sent {
		if ev.GetXpChanged() == nil {
			t.Errorf("Ana's stream got %v for a conversion, want nothing about the map", ev)
		}
		if saw := asJSON(t, ev); strings.Contains(saw, "LEAKCANARY-treasure-1") || strings.Contains(saw, point.GetId()) {
			t.Errorf("Ana's stream carries the treasure: %s", saw)
		}
	}
}

// The description always fits a point's 2,000 characters: for every treasure the
// tables can roll (many seeds, every level) and for the worst one the limits allow.
func TestTreasureDescriptionAlwaysFits(t *testing.T) {
	t.Parallel()
	c := treasureRules(t)
	longest := 0
	for level := 1; level <= 20; level++ {
		for seed := range uint64(400) {
			tr, err := c.GenerateTreasure(rules.TreasureHoard, level, seed*0x9E3779B97F4A7C15)
			if err != nil {
				t.Fatal(err)
			}
			d := treasureDescription(tr)
			if _, err := cleanDescription(d); err != nil {
				t.Fatalf("level %d seed %d: %v", level, seed, err)
			}
			longest = max(longest, len([]rune(d)))
		}
	}
	// The worst case: 5 coins with 6 digits and separators, 12 different gems, 6 art
	// objects and 6 items with the longest names and notes.
	worst := rules.Treasure{}
	for _, coin := range rules.Coins() {
		worst.Coins = append(worst.Coins, rules.TreasureCoin{Coin: coin, Count: 999_999})
	}
	for i := range 12 {
		worst.Gems = append(worst.Gems, rules.TreasurePiece{NamePT: strings.Repeat("g", 20) + string(rune('a'+i)), ValuePO: 5000, Count: 1})
	}
	for i := range 6 {
		worst.Art = append(worst.Art, rules.TreasurePiece{NamePT: strings.Repeat("a", 40) + string(rune('a'+i)), ValuePO: 7500, Count: 9})
	}
	var longestItem string
	for _, e := range c.MagicItems() {
		if len([]rune(e.NamePT)) > len([]rune(longestItem)) {
			longestItem = e.NamePT
		}
	}
	for i := range 6 {
		worst.Items = append(worst.Items, rules.TreasureItem{Key: strconv.Itoa(i), NamePT: longestItem, Rarity: rules.RarityVeryRare, Attunement: true})
	}
	if n := len([]rune(treasureDescription(worst))); n > maxDescriptionLength {
		t.Errorf("the worst description has %d characters, over the %d of a point", n, maxDescriptionLength)
	}
	t.Logf("the longest description of the tables: %d characters; the worst the limits allow fits", longest)
}

func TestPtInt(t *testing.T) {
	t.Parallel()
	for in, want := range map[int]string{0: "0", 7: "7", 999: "999", 1000: "1.000", 1200: "1.200", 12345: "12.345", 200000: "200.000", 1234567: "1.234.567", -1500: "-1.500"} {
		if got := ptInt(in); got != want {
			t.Errorf("ptInt(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestTreasureDescriptionReadsInPortuguese(t *testing.T) {
	t.Parallel()
	tr := rules.Treasure{
		Coins: []rules.TreasureCoin{{Coin: rules.CoinSilver, Count: 1200}, {Coin: rules.CoinGold, Count: 340}, {Coin: rules.CoinPlatinum, Count: 2}},
		Gems:  []rules.TreasurePiece{{NamePT: "Ágata-de-fogo", ValuePO: 18, Count: 2}, {NamePT: "Citrino", ValuePO: 54, Count: 1}},
		Art:   []rules.TreasurePiece{{NamePT: "Dedal de prata com um pássaro gravado", ValuePO: 27, Count: 1}},
		Items: []rules.TreasureItem{
			{Key: "item:potion-of-healing-common", NamePT: "Poção de cura", Rarity: rules.RarityCommon},
			{Key: "item:spell-scroll-1st", NamePT: "Pergaminho de magia (1º nível)", Rarity: rules.RarityCommon},
			{Key: "item:ring-of-protection", NamePT: "Anel de proteção", Rarity: rules.RarityRare, Attunement: true},
			{Key: "item:spell-scroll-1st", NamePT: "Pergaminho de magia (1º nível)", Rarity: rules.RarityCommon},
		},
	}
	const want = "Moedas: 1.200 PP, 340 PO e 2 PL.\n" +
		"Gemas:\n- 2 × Ágata-de-fogo (18 PO cada)\n- Citrino (54 PO)\n" +
		"Obras de arte:\n- Dedal de prata com um pássaro gravado (27 PO)\n" +
		"Itens mágicos:\n- Poção de cura (item comum)\n- 2 × Pergaminho de magia (1º nível) (item comum)\n- Anel de proteção (item raro, exige sintonização)"
	if got := treasureDescription(tr); got != want {
		t.Errorf("description:\n%s\nwant:\n%s", got, want)
	}
	if got := treasureDescription(rules.Treasure{}); got != "" {
		t.Errorf("an empty treasure reads %q", got)
	}
	if got := joinPT([]string{"a"}); got != "a" {
		t.Errorf("joinPT(one) = %q", got)
	}
}
