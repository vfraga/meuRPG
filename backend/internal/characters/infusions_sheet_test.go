package characters

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// The fixture's infusions, by the key of their option.
const (
	infusionKeyBlade   = "feature:class-engenhista-de-li-6d59b9--gume-vivo@mesa"
	infusionKeyPlate   = "feature:class-engenhista-de-li-6d59b9--couraca-de-petalas@mesa"
	infusionKeyFocus   = "feature:class-engenhista-de-li-6d59b9--diapasao-interno@mesa"
	infusionKeyAsh     = "feature:class-engenhista-de-li-6d59b9--manto-de-cinza@mesa"
	infusionKeyHouse   = "feature:class-engenhista-de-li-6d59b9--selo-de-casa@mesa"
	infusionKeyFalse   = "feature:class-engenhista-de-li-6d59b9--fundo-falso@mesa"
	infusionHostKey    = "feature:class-engenhista-de-li-6d59b9--gravar-pecas@mesa"
	infusionMace       = "equipment:mace"
	infusionLeather    = "equipment:leather-armor"
	infusionShieldGear = "equipment:shield"
)

// infusedTable is a campaign whose content has the fixture's class and Dona's artificer of level 6, holding a mace, a
// leather armor and a shield, with the five infusions the class lets her know at that level (the blade, the plate, the
// ash cloak, the false bottom and the house seal).
type infusedTable struct {
	*editorTable
	pc *charactersv1.Character
}

func newInfusedTable(t *testing.T) *infusedTable {
	t.Helper()
	return newInfusedTableWith(t, nil)
}

// newInfusedTableWith is newInfusedTable with a say in the artificer's sheet.
func newInfusedTableWith(t *testing.T, tweak func(*charactersv1.FullSheet)) *infusedTable {
	t.Helper()
	et := newEditorTable(t, nil)
	pt := &packTable{h: et.h, live: &liveSpy{}, master: et.master}
	pt.apply(t, et.campaign, loadInfusionPack(t), "infusions-sheet")
	sheet := choiceSheet("race:human", "", infusionClassKey, "", 6,
		infusionKeyBlade, infusionKeyPlate, infusionKeyAsh, infusionKeyFalse, infusionKeyHouse)
	sheet.GetFull().WeaponKeys = []string{infusionMace}
	sheet.GetFull().ArmorKey = infusionLeather
	sheet.GetFull().Shield = true
	sheet.GetFull().SkillProficiencyKeys = []string{"skill:arcana", "skill:history"}
	sheet.GetFull().ExperiencePoints = 23000 // the level 7 is hers to take
	et.h.svc.SetLevelUps(xpLevelUps{})
	if tweak != nil {
		tweak(sheet.GetFull())
	}
	pc := et.dona.create(t, et.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Faísca", sheet)
	return &infusedTable{editorTable: et, pc: pc}
}

// infuse writes a live record of the artificer's own gear, as the infusion calls will.
func (it *infusedTable) infuse(t *testing.T, infusion, gearKey string) charactersdb.CharacterInfusion {
	t.Helper()
	rec, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: it.pc.GetId(),
		InfusionKey: infusion, ItemKind: infusionItemGear, GearKey: gearKey,
	})
	if err != nil {
		t.Fatalf("insertInfusion(%s on %s) error = %v", infusion, gearKey, err)
	}
	return rec
}

// read is the character as the master reads it.
func (it *infusedTable) read(t *testing.T) *charactersv1.Character {
	t.Helper()
	res, err := it.master.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	return res.Msg.GetCharacter()
}

func attackOf(c *charactersv1.Character, key string) *rulesv1.Attack {
	for _, a := range c.GetDerived().GetAttacks() {
		if a.GetKey() == key {
			return a
		}
	}
	return nil
}

// TestTheFixtureArtificerExists is the table the other tests stand on.
func TestTheFixtureArtificerExists(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	c := it.read(t)
	a := attackOf(c, infusionMace)
	if a == nil {
		t.Fatalf("the sheet has no attack of the mace: %v", c.GetDerived().GetAttacks())
	}
	if a.GetMagicBonus() != 0 || a.GetInfusionNamePt() != "" || len(c.GetSheet().GetFull().GetInfusions()) != 0 {
		t.Errorf("a sheet with no record has the attack %v and infusions %v, want none", a, c.GetSheet().GetFull().GetInfusions())
	}
}

// A live record of the character as holder gives its numbers and its name on every read of the sheet, and the sheet lists
// it: one entry per record, valid or not, the oldest first.
func TestTheSheetReadShowsTheInfusionsOfTheCharacter(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	blade := it.infuse(t, infusionKeyBlade, infusionMace)
	it.h.clock.Advance(time.Minute)
	it.infuse(t, infusionKeyFalse, infusionShieldGear)
	it.h.clock.Advance(time.Minute)
	gone, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: it.pc.GetId(), InfusionKey: "feature:no-longer-there", ItemKind: infusionItemGear, GearKey: infusionLeather,
	})
	if err != nil {
		t.Fatal(err)
	}
	it.h.clock.Advance(time.Minute)
	lantern, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: it.pc.GetId(), InfusionKey: infusionKeyHouse, ItemKind: infusionItemObject, ObjectName: "Lanterna",
	})
	if err != nil {
		t.Fatal(err)
	}

	c := it.read(t)
	if a := attackOf(c, infusionMace); a == nil || a.GetMagicBonus() != 1 || a.GetInfusionNamePt() != "Gume Vivo" {
		t.Errorf("the mace attack = %v, want +1 and the name Gume Vivo", a)
	}
	list := c.GetSheet().GetFull().GetInfusions()
	if len(list) != 4 {
		t.Fatalf("the sheet lists %d infusions, want 4: %v", len(list), list)
	}
	for i, id := range []string{blade.ID, "", gone.ID, lantern.ID} {
		if id != "" && list[i].GetId() != id {
			t.Errorf("entry %d is %s, want %s (the oldest first)", i, list[i].GetId(), id)
		}
	}
	b := list[0]
	if b.GetInfusionKey() != infusionKeyBlade || b.GetNamePt() != "Gume Vivo" || b.GetHolderCharacterId() != it.pc.GetId() ||
		b.GetItemKind() != charactersv1.InfusionItemKind_INFUSION_ITEM_KIND_GEAR || b.GetItemKey() != infusionMace || b.GetItemNamePt() != "Maça" ||
		!b.GetValidNow() || !b.GetComputed() || b.GetBonus() != 1 || b.GetRequiresAttunement() || b.GetAttuned() || b.GetSince() == nil {
		t.Errorf("the blade entry = %v", b)
	}
	if text := list[1]; text.GetValidNow() != true || text.GetComputed() || text.GetBonus() != 0 || text.GetItemKey() != infusionShieldGear {
		t.Errorf("the text-only entry = %v, want valid, not computed, no bonus, on the shield", text)
	}
	if lost := list[2]; lost.GetValidNow() || lost.GetBonus() != 0 || lost.GetNamePt() != "Infusão que a ficha não conhece" {
		t.Errorf("the entry of an infusion the content lost = %v, want not valid, no bonus", lost)
	}
	if obj := list[3]; obj.GetItemKind() != charactersv1.InfusionItemKind_INFUSION_ITEM_KIND_OBJECT || obj.GetObjectNamePt() != "Lanterna" ||
		obj.GetItemKey() != "" || obj.GetItemNamePt() != "Lanterna" || !obj.GetValidNow() {
		t.Errorf("the object entry = %v", obj)
	}
}

func attackOptionOf(opts *rulesv1.TurnOptions, key string) *rulesv1.AttackOption {
	for _, o := range opts.GetAttacks() {
		if o.GetAttack().GetKey() == key {
			return o
		}
	}
	return nil
}

// The combat's reads of the sheet carry the infusion: the attack option names it and has its bonus, the attack the combat
// rolls hits one better, and the armor class of a piece an infusion gives its number is the sheet's.
func TestTheCombatReadsTheInfusionOfTheCharacter(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	before, err := it.h.svc.CombatSheet(t.Context(), nil, it.campaign, it.pc.GetId())
	if err != nil {
		t.Fatalf("CombatSheet() error = %v", err)
	}
	it.infuse(t, infusionKeyPlate, infusionLeather)
	it.infuse(t, infusionKeyBlade, infusionMace)
	after, err := it.h.svc.CombatSheet(t.Context(), nil, it.campaign, it.pc.GetId())
	if err != nil {
		t.Fatalf("CombatSheet() error = %v", err)
	}
	tohit := func(s link.Sheet) int {
		for _, a := range s.Attacks {
			if a.Key == infusionMace {
				return a.ToHit
			}
		}
		t.Fatalf("no attack of the mace in %v", s.Attacks)
		return 0
	}
	if tohit(after) != tohit(before)+1 || after.ArmorClass != before.ArmorClass+1 {
		t.Errorf("the combat sheet hits %d for armor class %d, want one better than %d and %d", tohit(after), after.ArmorClass, tohit(before), before.ArmorClass)
	}
	opts, err := it.h.svc.CombatTurnOptions(t.Context(), nil, it.campaign, it.pc.GetId(), link.Turn{SpeedFt: 30})
	if err != nil {
		t.Fatalf("CombatTurnOptions() error = %v", err)
	}
	o := attackOptionOf(opts, infusionMace)
	if o == nil || o.GetAttack().GetInfusionNamePt() != "Gume Vivo" || o.GetMagicBonus() != 1 {
		t.Fatalf("the mace option = %v, want the name Gume Vivo and +1", o)
	}
}

// Mage Armor reads a shield an infusion gives its number: the character wears no armor, and the armor class the spell
// gives has the shield's +2 and the infusion's +1.
func TestMageArmorReadsTheInfusionOfTheShield(t *testing.T) {
	t.Parallel()
	it := newInfusedTableWith(t, func(f *charactersv1.FullSheet) { f.ArmorKey = "" })
	plain, err := it.h.svc.MageArmorAC(t.Context(), nil, it.campaign, it.pc.GetId())
	if err != nil || !plain.Applies || plain.Wears {
		t.Fatalf("MageArmorAC() = %+v, %v; want it to apply and the character to wear none", plain, err)
	}
	it.infuse(t, infusionKeyPlate, infusionShieldGear)
	got, err := it.h.svc.MageArmorAC(t.Context(), nil, it.campaign, it.pc.GetId())
	if err != nil || got.AC != plain.AC+1 {
		t.Errorf("MageArmorAC() = %+v, %v; want %d, one more than without the infusion", got, err, plain.AC+1)
	}
}

// A manual bonus the save adds on a piece an infusion gives its bonus is refused: the check reads the records. A pair the
// stored sheet already had is left alone.
func TestTheSaveRefusesAManualBonusOnAnInfusedPiece(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	it.infuse(t, infusionKeyBlade, infusionMace)
	req := editorSaveOf(it.read(t))
	req.Sheet.GetFull().ItemBonuses = []*charactersv1.ItemBonus{{Key: infusionMace, Bonus: 1}}
	_, err := editorUpdate(it.master, req)
	wantEditorRefusal(t, "a manual +1 on the infused mace", err,
		`invalid_argument: sheet.full.item_bonuses[0].key has an infusion that gives its magic bonus: end the infusion, or leave the manual bonus off | InvalidField{field="sheet.full.item_bonuses[0].key"}`)

	// A pair the stored sheet has stays: the sheet keeps saving.
	plain := newInfusedTable(t)
	withBonus := editorSaveOf(plain.read(t))
	withBonus.Sheet.GetFull().ItemBonuses = []*charactersv1.ItemBonus{{Key: infusionMace, Bonus: 1}}
	saved, err := editorUpdate(plain.master, withBonus)
	if err != nil {
		t.Fatalf("the master saves a manual bonus: %v", err)
	}
	plain.infuse(t, infusionKeyBlade, infusionMace)
	if _, err := editorUpdate(plain.master, editorSaveOf(saved)); err != nil {
		t.Errorf("a save that keeps the pair the sheet already had: error = %v", err)
	}
}

// The level-up preview derives the sheet at the new level with the infusions: the blade gives its +1 there too.
func TestTheLevelUpPreviewCarriesTheInfusions(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	it.infuse(t, infusionKeyBlade, infusionMace)
	res, err := it.dona.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{
		CampaignId: it.campaign, CharacterId: it.pc.GetId(), Choices: &charactersv1.LevelUpChoices{ClassKey: infusionClassKey},
	}))
	if err != nil {
		t.Fatalf("PreviewLevelUp() error = %v", err)
	}
	var found *rulesv1.Attack
	for _, a := range res.Msg.GetAfter().GetAttacks() {
		if a.GetKey() == infusionMace {
			found = a
		}
	}
	if found == nil || found.GetMagicBonus() != 1 || found.GetInfusionNamePt() != "Gume Vivo" {
		t.Errorf("the mace after the level = %v, want +1 and the name Gume Vivo", found)
	}
}

// PreviewCharacter of a character that exists derives what the save would show, infusions included.
func TestThePreviewOfACharacterCarriesTheInfusions(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	it.infuse(t, infusionKeyBlade, infusionMace)
	c := it.read(t)
	res, err := it.master.api.PreviewCharacter(t.Context(), connect.NewRequest(&charactersv1.PreviewCharacterRequest{
		CampaignId: it.campaign, CharacterId: c.GetId(), Sheet: c.GetSheet(),
	}))
	if err != nil {
		t.Fatalf("PreviewCharacter() error = %v", err)
	}
	var found *rulesv1.Attack
	for _, a := range res.Msg.GetDerived().GetAttacks() {
		if a.GetKey() == infusionMace {
			found = a
		}
	}
	if found == nil || found.GetMagicBonus() != 1 {
		t.Errorf("the mace in the preview = %v, want +1", found)
	}
}
