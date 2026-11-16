package characters

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What a player reads about a table class feature the master switched off (RN-23): the sheet no longer has it, so
// the player reads about it as anyone else does. These tests need the database (MEURPG_TEST_DATABASE_URL).

// featureReadsTable is a table with a table class feature of the wizard at level 2 that has a summon, a class feature
// and a feat that name it, a wizard player (Ana, level 3) and a fighter player (Caio).
type featureReadsTable struct {
	*choiceTable
	feature, replacer, lacker string
}

func newFeatureReadsTable(t *testing.T) *featureReadsTable {
	t.Helper()
	tb := &featureReadsTable{choiceTable: newChoiceTable(t)}
	body := classFeatureSummonBody()
	body.ClassKey = "class:wizard"
	tb.feature = tb.master.addEntry(t, tb.campaign, body).GetKey()
	tb.replacer = tb.master.addEntry(t, tb.campaign, &rulesv1.TableClassFeature{
		NamePt: "Substituta da Matilha", DescPt: []string{"Ocupa o lugar do chamado."}, ClassKey: "class:wizard", Level: 2, ReplacesFeatureKey: tb.feature,
	}).GetKey()
	feat := testFeat("Sem o chamado")
	feat.Prerequisite = &rulesv1.FeatPrerequisite{LacksFeatureKey: tb.feature}
	tb.lacker = tb.master.addEntry(t, tb.campaign, feat).GetKey()
	tb.ana.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", pensantusSheet())
	fighter := choiceSheet("race:human", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense")
	tb.caio.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", fighter)
	return tb
}

func (tb *featureReadsTable) spell(t *testing.T, u *user, key string) (*rulesv1.SpellDetails, error) {
	t.Helper()
	res, err := u.content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: tb.campaign, SpellKey: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetSpell(), nil
}

// The spell of the summon of a class feature that is off reads as an unknown spell to a player whose class and level
// would have had the feature.
func TestAClassFeatureThatIsOffIsNotOneTheSheetHasForItsSummonSpell(t *testing.T) {
	t.Parallel()
	tb := newFeatureReadsTable(t)
	spell := rules.FeatureSummonSpellKey(tb.feature)
	if d, err := tb.spell(t, tb.ana, spell); err != nil || d == nil {
		t.Fatalf("with the feature on, the wizard's read of %s = %v, %v; want the spell", spell, d, err)
	}
	tb.master.setOff(t, tb.campaign, true, tb.feature)

	_, unknown := tb.spell(t, tb.ana, "spell:nao-existe")
	if connect.CodeOf(unknown) != connect.CodeNotFound {
		t.Fatalf("an unknown spell: error = %v, want not_found", unknown)
	}
	for _, u := range []*user{tb.ana, tb.caio} {
		d, err := tb.spell(t, u, spell)
		if d != nil || err == nil || connect.CodeOf(err) != connect.CodeNotFound || err.Error() != unknown.Error() {
			t.Errorf("a player's read of the switched-off feature's spell = %v, %v; want the answer of an unknown spell (%v)", d, err, unknown)
		}
	}
	// The master reads it as before.
	if d, err := tb.spell(t, tb.master, spell); err != nil || d == nil {
		t.Errorf("the master's read of %s = %v, %v; want the spell", spell, d, err)
	}
}

// Another entry that names a class feature that is off reads the same to a player of its class and level as to
// a player of another class.
func TestAClassFeatureThatIsOffIsNotNamedToThePlayersOfItsClass(t *testing.T) {
	t.Parallel()
	tb := newFeatureReadsTable(t)
	tb.master.setOff(t, tb.campaign, true, tb.feature)

	ana, caio := tb.ana.entries(t, tb.campaign), tb.caio.entries(t, tb.campaign)
	if ana[tb.feature] != nil || caio[tb.feature] != nil {
		t.Errorf("a player reads the entry that is off: %v / %v", ana[tb.feature], caio[tb.feature])
	}
	for name, u := range map[string]map[string]*rulesv1.TableEntry{"the wizard": ana, "the fighter": caio} {
		if r := u[tb.replacer]; r == nil || r.GetTableClassFeature().GetReplacesFeatureKey() != "" {
			t.Errorf("%s reads the class feature that replaces it as %v, want it without the key", name, r)
		}
		p := u[tb.lacker].GetTableFeat().GetPrerequisite()
		if u[tb.lacker] == nil || p.GetLacksFeatureKey() != "" || !p.GetLacksHidden() {
			t.Errorf("%s reads the feat that lacks it as %v, want the key blank and marked", name, u[tb.lacker])
		}
	}
	// The master reads both whole.
	master := tb.master.entries(t, tb.campaign)
	if master[tb.replacer].GetTableClassFeature().GetReplacesFeatureKey() != tb.feature || master[tb.lacker].GetTableFeat().GetPrerequisite().GetLacksFeatureKey() != tb.feature {
		t.Errorf("the master's reads = %v / %v, want both keys", master[tb.replacer], master[tb.lacker])
	}
}

// What a sheet keeps stays readable to its player after the master switches it off or retires it, and the master
// reads all of it as before; a player whose sheet does not keep it reads nothing of it.
func TestAPlayerStillReadsTheSpellAndFeatTheirSheetKeepsWhenSwitchedOffOrRetired(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, dona, davi := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Davi")
	campaign := h.newCampaign(master, "Mirathel", dona, davi)
	race := master.addEntry(t, campaign, testRace("Anão das Brumas")).GetKey()
	bg := master.addEntry(t, campaign, testBackground("Guarda de farol")).GetKey()
	off := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard")).GetKey()
	retired := master.addEntry(t, campaign, testSpell("Raio de reserva", "class:wizard")).GetKey()
	feat := master.addEntry(t, campaign, testFeat("Punho de ferro")).GetKey()
	lackerFeat := testFeat("Sem punho")
	lackerFeat.Prerequisite = &rulesv1.FeatPrerequisite{LacksFeatureKey: feat}
	lacker := master.addEntry(t, campaign, lackerFeat).GetKey()

	sheet := sheetWithTable(race, bg, off)
	sheet.GetFull().KnownSpellKeys = append(sheet.GetFull().KnownSpellKeys, retired)
	ch := dona.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Icaro", sheet)
	sheet.GetFull().FeatKeys = []string{feat}
	if _, err := master.update(t, ch, "Icaro", sheet); err != nil {
		t.Fatalf("UpdateCharacter(feat) error = %v", err)
	}
	master.setOff(t, campaign, true, off, feat)
	if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: retired})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}

	read := func(u *user, key string) error {
		_, err := u.content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: campaign, SpellKey: key}))
		return err
	}
	for _, key := range []string{off, retired} {
		if err := read(dona, key); err != nil {
			t.Errorf("the player whose sheet keeps %s reads it: error = %v, want none", key, err)
		}
		if err := read(master, key); err != nil {
			t.Errorf("the master's read of %s: error = %v, want none", key, err)
		}
		if err := read(davi, key); connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("the player whose sheet does not keep %s: error = %v, want not_found", key, err)
		}
	}
	// Another entry that names the feat their sheet keeps still names it to them, and not to the other player.
	lacks := func(u *user) *rulesv1.FeatPrerequisite {
		return u.entries(t, campaign)[lacker].GetTableFeat().GetPrerequisite()
	}
	if p := lacks(dona); p.GetLacksFeatureKey() != feat || p.GetLacksHidden() {
		t.Errorf("the player whose sheet keeps the feat reads the entry that lacks it as %v, want the key", p)
	}
	if p := lacks(davi); p.GetLacksFeatureKey() != "" || !p.GetLacksHidden() {
		t.Errorf("the other player reads the entry that lacks the feat as %v, want the key blank and marked", p)
	}
	for _, u := range []*user{dona, davi} {
		if e := u.entries(t, campaign); e[feat] != nil || e[off] != nil || e[retired] != nil {
			t.Errorf("a player's entries = %v, want none of the entries that are off or retired", e)
		}
	}
	if e := master.entries(t, campaign); e[feat] == nil || e[off] == nil || e[retired] == nil {
		t.Errorf("the master's entries = %v, want all three", e)
	}
}

// A fighting style option the master switches off or retires after a sheet picked it: another entry that names it
// still names it to the player whose sheet has it, and not to another player; the master reads both entries whole.
func TestAPlayerStillReadsTheOptionTheirSheetKeepsWhenSwitchedOffOrRetired(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(*testing.T, *choiceTable, string){
		"switched off": func(t *testing.T, tb *choiceTable, key string) {
			t.Helper()
			tb.master.setOff(t, tb.campaign, true, key)
		},
		"retired": func(t *testing.T, tb *choiceTable, key string) {
			t.Helper()
			if _, err := tb.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: key})); err != nil {
				t.Fatalf("ArchiveTableEntry() error = %v", err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tb := newChoiceTable(t)
			key := tb.master.addEntry(t, tb.campaign, &rulesv1.TableOption{NamePt: "Guarda de teste", Set: "fighting_style", DescPt: []string{"Texto."}}).GetKey()
			feat := testFeat("Sem guarda")
			feat.Prerequisite = &rulesv1.FeatPrerequisite{LacksFeatureKey: key}
			lacker := tb.master.addEntry(t, tb.campaign, feat).GetKey()
			if _, err := tb.create(tb.caio, choiceSheet("race:human", "", "class:fighter", "", 1, key)); err != nil {
				t.Fatalf("create the sheet that picks %s: %v", key, err)
			}
			change(t, tb, key)

			lacks := func(u *user) *rulesv1.FeatPrerequisite {
				return u.entries(t, tb.campaign)[lacker].GetTableFeat().GetPrerequisite()
			}
			if p := lacks(tb.caio); p.GetLacksFeatureKey() != key || p.GetLacksHidden() {
				t.Errorf("the player whose sheet keeps the option reads the entry that lacks it as %v, want the key", p)
			}
			if p := lacks(tb.ana); p.GetLacksFeatureKey() != "" || !p.GetLacksHidden() {
				t.Errorf("the other player reads the entry that lacks the option as %v, want the key blank and marked", p)
			}
			if p := lacks(tb.master); p.GetLacksFeatureKey() != key || p.GetLacksHidden() {
				t.Errorf("the master reads the entry that lacks the option as %v, want the key", p)
			}
		})
	}
}
