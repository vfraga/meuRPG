package characters

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// #580 S2: the weapon content kind, as the master writes it and the sheet and a player read it.

func TestATableWeaponIsReadByAPlayerAndWornByASheet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	w := testWeapon("Espada do Vale")
	w.Ranged, w.NormalRangeFt, w.LongRangeFt = false, 0, 0
	key := master.addEntry(t, campaign, w).GetKey()

	if got := player.entries(t, campaign)[key].GetTableWeapon(); got.GetDamage() != "1d8" || got.GetTwoHandedDamage() != "1d10" || got.GetPriceCp() != 1500 {
		t.Errorf("the player reads %v, want the weapon whole", got)
	}
	sheet := pensantusSheet()
	sheet.GetFull().WeaponKeys = []string{key}
	pc := player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", sheet)
	if !slices.ContainsFunc(pc.GetDerived().GetAttacks(), func(a *rulesv1.Attack) bool { return a.GetName() == "Espada do Vale" }) {
		t.Errorf("attacks = %v, want a line for the table's weapon", pc.GetDerived().GetAttacks())
	}
}

func TestATableWeaponRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	cases := []struct {
		name  string
		edit  func(w *rulesv1.TableWeapon)
		field string
	}{
		{"no category", func(w *rulesv1.TableWeapon) { w.Category = rulesv1.WeaponCategory_WEAPON_CATEGORY_UNSPECIFIED }, "table_weapon.category"},
		{"bad damage", func(w *rulesv1.TableWeapon) { w.Damage = "dois d8" }, "table_weapon.damage"},
		{"two-handed damage without the property", func(w *rulesv1.TableWeapon) { w.Properties = nil }, "table_weapon.two_handed_damage"},
		{"a range on a plain melee weapon", func(w *rulesv1.TableWeapon) { w.NormalRangeFt = 20 }, "table_weapon.normal_range_ft"},
		{"a thrown weapon without ranges", func(w *rulesv1.TableWeapon) {
			w.Properties = []string{"weapon-property:versatile", "weapon-property:thrown"}
		}, "table_weapon.normal_range_ft"},
		{"a negative weight", func(w *rulesv1.TableWeapon) { w.WeightOz = -1 }, "table_weapon.weight_oz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := testWeapon("Arma de teste")
			tc.edit(w)
			_, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, w)))
			if got := violationFields(violationsOfErr(t, err)); !slices.Contains(got, tc.field) {
				t.Errorf("violations at %v, want one at %s", got, tc.field)
			}
		})
	}
}

// An archived or switched off weapon is not in a player's catalog unless their own sheet carries it, and the master reads it.
func TestAnArchivedWeaponIsNotInAPlayersCatalog(t *testing.T) {
	t.Parallel()
	c := &rulesv1.Content{ContentVersion: "v", Weapons: []*rulesv1.Weapon{{Key: "weapon:a@mesa"}, {Key: "weapon:b@mesa", Archived: true}, {Key: "weapon:c@mesa", Off: true}, {Key: "weapon:d@mesa", Archived: true}}}
	p := withoutArchived(c, map[string]bool{"weapon:d@mesa": true})
	var keys []string
	for _, w := range p.GetWeapons() {
		keys = append(keys, w.GetKey())
	}
	if !slices.Equal(keys, []string{"weapon:a@mesa", "weapon:d@mesa"}) {
		t.Errorf("a player's weapons = %v, want the live one and the one their sheet carries", keys)
	}
	if len(c.GetWeapons()) != 4 {
		t.Errorf("the master's catalog lost entries: %v", c.GetWeapons())
	}
}

// A weapon goes through a pack and comes back as the same entry, and a pack that breaks its rules is refused as an editor is.
func TestATableWeaponSurvivesAPackRoundTrip(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	w := testWeapon("Arco do Vale")
	w.Ranged, w.NormalRangeFt, w.LongRangeFt, w.Properties, w.TwoHandedDamage = true, 80, 320, []string{"weapon-property:ammunition"}, ""
	made := pt.master.addEntry(t, pt.from, w)

	pack := pt.export(t, pt.from)
	file, err := storeJSON.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	loaded := &rulesv1.TableContentPack{}
	if err := loadJSON.Unmarshal(file, loaded); err != nil {
		t.Fatal(err)
	}
	if pv := pt.preview(t, pt.to, loaded); pv.GetTotals().GetNew() != 1 || pv.GetTotals().GetRefused() != 0 {
		t.Fatalf("preview totals = %v, want one new weapon", pv.GetTotals())
	}
	pt.apply(t, pt.to, loaded, "weapon-import")
	got := pt.entries(t, pt.to)[made.GetKey()]
	if got == nil || !proto.Equal(bodyMessage(got), bodyMessage(made)) {
		t.Errorf("the weapon after the round trip = %v, want %v", got, made)
	}

	bad := proto.Clone(loaded).(*rulesv1.TableContentPack)
	bad.Entries[0].GetTableWeapon().NormalRangeFt = 0
	if pv := pt.preview(t, pt.to, bad); pv.GetTotals().GetRefused() != 1 {
		t.Errorf("a pack with a ranged weapon with no range: totals %v, want it refused", pv.GetTotals())
	}
}

// RN-23 through the service: the weapon a player's own sheet carries stays in their catalog when the master archives it,
// and no other player gets it, nor the owner for a weapon their sheet does not carry.
func TestACarriedTableWeaponStaysInTheOwnersCatalog(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	w := master.addEntry(t, campaign, testWeapon("Espada do Vale"))
	spare := master.addEntry(t, campaign, testWeapon("Lâmina Sobressalente"))
	sheet := pensantusSheet()
	sheet.GetFull().WeaponKeys = []string{w.GetKey()}
	mine := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", sheet)
	for _, key := range []string{w.GetKey(), spare.GetKey()} {
		if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: key})); err != nil {
			t.Fatalf("ArchiveTableEntry(%s) error = %v", key, err)
		}
	}
	weapons := func(u *user, character string) []string {
		res, err := u.catalogFor(t, campaign, character)
		if err != nil {
			t.Fatalf("ListContent() error = %v", err)
		}
		var keys []string
		for _, x := range res.GetContent().GetWeapons() {
			if strings.HasSuffix(x.GetKey(), "@mesa") {
				keys = append(keys, x.GetKey())
			}
		}
		return keys
	}
	if got := weapons(owner, mine.GetId()); !slices.Equal(got, []string{w.GetKey()}) {
		t.Errorf("the owner's catalog weapons = %v, want only the one their sheet carries", got)
	}
	if got := weapons(other, ""); len(got) != 0 {
		t.Errorf("another player's catalog weapons = %v, want none", got)
	}
}

// The catalog's weapon carries its damage dice, for the row of the sheet editor's list ("Simples · 1d6", board 580-16 state 4).
func TestTheCatalogWeaponCarriesItsDamageDice(t *testing.T) {
	t.Parallel()
	got := weaponToProto(rules.WeaponEntry{Key: "weapon:machado@mesa", NamePT: "Machado de Lenhador", Category: "simple", Range: "melee", Damage: "1d6"})
	if got.GetDamage() != "1d6" || got.GetCategory() != rulesv1.WeaponCategory_WEAPON_CATEGORY_SIMPLE {
		t.Errorf("weapon = %v, want simple with 1d6", got)
	}
}
