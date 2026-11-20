package characters

import (
	"os"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The fixture pack of the infusion tests: testdata/pack_engenhista.json, the invented class "Engenhista de Ligas".
// Its level 2 feature "Gravar Peças" has the infusion rule (2 live from level 2, 3 from 6, 4 from 10) and eight
// inline options with infusion data, one of every effect and two with no effect (text only). Its features at
// levels 6, 10 and 14 pick more of the same options through a `choice` of kind feature with `from`. Later tests
// load it with loadInfusionPack, import it into a campaign with packTable.apply or build the engine's content from
// it with infusionContent.

const (
	infusionClassKey = "class:engenhista-de-ligas@mesa"
	infusionHostName = "Gravar Peças"
)

// loadInfusionPack reads the fixture the way an import reads a file: strictly, so a field the message lacks fails.
func loadInfusionPack(t testing.TB) *rulesv1.TableContentPack {
	t.Helper()
	data, err := os.ReadFile("testdata/pack_engenhista.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, unknown, top := readPackJSON(data)
	if len(unknown) != 0 || len(top) != 0 {
		t.Fatalf("the fixture pack has unknown fields: %v %v", unknown, top)
	}
	return pack
}

// infusionContent is the engine's content with the fixture's class on top of the SRD.
func infusionContent(t testing.TB) *rules.Content {
	t.Helper()
	return contentOfPack(t, loadInfusionPack(t))
}

// contentOfPack is the engine's content with the pack's entries on top of the SRD.
func contentOfPack(t testing.TB, pack *rulesv1.TableContentPack) *rules.Content {
	t.Helper()
	var rows []entryRow
	for _, e := range pack.GetEntries() {
		body, ok := bodyMessage(e).(tableBody)
		if !ok {
			t.Fatalf("entry %s has no body", e.GetKey())
		}
		rows = append(rows, entryRow{row: charactersdb.CampaignContent{ContentKey: e.GetKey(), NamePt: e.GetNamePt()}, kind: e.GetKind(), body: body})
	}
	overlay, _ := overlayFromEntries(rows, 1)
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	c, err := srd.With(overlay)
	if err != nil {
		t.Fatalf("the fixture pack does not build: %v", err)
	}
	return c
}

// infusionHost is the fixture's host feature, as stored.
func infusionHost(t testing.TB) *rulesv1.TableFeature {
	t.Helper()
	for _, l := range bodyMessage(loadInfusionPack(t).GetEntries()[0]).(*rulesv1.TableClass).GetLevels() {
		for _, f := range l.GetFeatures() {
			if f.GetNamePt() == infusionHostName {
				return f
			}
		}
	}
	t.Fatalf("the fixture has no %q", infusionHostName)
	return nil
}

func TestTheFixturePackBuildsAndItsInfusionsAreKnown(t *testing.T) {
	t.Parallel()
	c := infusionContent(t)
	host := infusionHost(t)
	if len(host.GetOptions()) != 8 {
		t.Fatalf("the host has %d options, want 8", len(host.GetOptions()))
	}
	effects := map[rulesv1.InfusionEffect]int{}
	for _, o := range host.GetOptions() {
		in, ok := c.Infusion(o.GetKey())
		if !ok {
			t.Fatalf("%s is not a known infusion", o.GetNamePt())
		}
		if c.InfusionHost(o.GetKey()) != host.GetKey() {
			t.Errorf("the host of %s = %q, want %q", o.GetNamePt(), c.InfusionHost(o.GetKey()), host.GetKey())
		}
		effects[o.GetInfusion().GetEffect()]++
		if in.Effect == rules.InfusionEffectNone && in.Accepts == "" {
			t.Errorf("%s has no accepts", o.GetNamePt())
		}
	}
	for _, e := range []rulesv1.InfusionEffect{
		rulesv1.InfusionEffect_INFUSION_EFFECT_ITEM_BONUS, rulesv1.InfusionEffect_INFUSION_EFFECT_SPELL_FOCUS_BONUS,
		rulesv1.InfusionEffect_INFUSION_EFFECT_REPEATING_AMMUNITION, rulesv1.InfusionEffect_INFUSION_EFFECT_RETURNING,
		rulesv1.InfusionEffect_INFUSION_EFFECT_RESISTANCE_LABEL,
	} {
		if effects[e] == 0 {
			t.Errorf("no option of the fixture has the effect %s", e)
		}
	}
	if effects[rulesv1.InfusionEffect_INFUSION_EFFECT_NONE] != 2 {
		t.Errorf("%d options are text only, want 2", effects[rulesv1.InfusionEffect_INFUSION_EFFECT_NONE])
	}
	for level, want := range map[int]int{1: 0, 2: 2, 5: 2, 6: 3, 9: 3, 10: 4, 20: 4} {
		if got := c.InfusionLimit(host.GetKey(), level); got != want {
			t.Errorf("InfusionLimit at class level %d = %d, want %d", level, got, want)
		}
	}
}

// The later levels pick more of the same options: each is a choice of features whose `from` lists the host's options.
func TestTheFixturePacksLaterLevelsPickMoreOfTheSameOptions(t *testing.T) {
	t.Parallel()
	host := infusionHost(t)
	var optionKeys []string
	for _, o := range host.GetOptions() {
		optionKeys = append(optionKeys, o.GetKey())
	}
	later := 0
	for _, l := range bodyMessage(loadInfusionPack(t).GetEntries()[0]).(*rulesv1.TableClass).GetLevels() {
		for _, f := range l.GetFeatures() {
			for _, e := range f.GetEffects() {
				if f.GetNamePt() != infusionHostName && e.GetType() == "choice" && e.GetChoice() == "feature" {
					later++
					if !slices.Equal(e.GetFrom(), optionKeys) {
						t.Errorf("%s offers %v, want the host's options", f.GetNamePt(), e.GetFrom())
					}
				}
			}
		}
	}
	if later != 3 {
		t.Errorf("%d later features pick more infusions, want 3", later)
	}
}

// The pack goes through an import and an export unchanged, and the player's read of the class has no infusion data.
func TestTheFixturePackRoundTripsThroughImportAndExport(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	pack := loadInfusionPack(t)
	if pv := pt.preview(t, pt.to, pack); pv.GetTotals().GetNew() != 1 || pv.GetTotals().GetRefused() != 0 {
		t.Fatalf("preview totals = %v, want one new entry and none refused", pv.GetTotals())
	}
	pt.apply(t, pt.to, pack, "infusions-1")
	got := pt.export(t, pt.to)
	if len(got.GetEntries()) != 1 || !proto.Equal(bodyMessage(got.GetEntries()[0]), bodyMessage(pack.GetEntries()[0])) {
		t.Fatalf("the export does not give the fixture back:\n%v\nwant\n%v", got.GetEntries(), pack.GetEntries())
	}
	var seen int
	for _, l := range pt.master.entries(t, pt.to)[infusionClassKey].GetTableClass().GetLevels() {
		for _, f := range l.GetFeatures() {
			if f.GetInfusionRule() != nil {
				seen++
			}
		}
	}
	if seen != 1 {
		t.Errorf("the master reads %d features with an infusion rule, want 1", seen)
	}
	for _, l := range pt.player.entries(t, pt.to)[infusionClassKey].GetTableClass().GetLevels() {
		for _, f := range l.GetFeatures() {
			if f.GetInfusionRule() != nil {
				t.Errorf("the player reads the infusion rule of %q", f.GetNamePt())
			}
			for _, o := range f.GetOptions() {
				if o.GetInfusion() != nil {
					t.Errorf("the player reads the infusion data of %q", o.GetNamePt())
				}
			}
		}
	}
}

// A refusal of the infusion data names the field of the entry's body, in a class's level row and in an option.
func TestTheRefusalsOfInfusionDataNameTheirFieldInTheBody(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel: a Costa")
	cases := []struct {
		name  string
		edit  func(host *rulesv1.TableFeature)
		field string
	}{
		{"a rule with no steps", func(h *rulesv1.TableFeature) { h.InfusionRule.LimitSteps = nil }, "table_class.levels[1].features[0].infusion_rule.limit_steps"},
		{"an infusion that accepts nothing", func(h *rulesv1.TableFeature) {
			h.Options[0].Infusion.Accepts = rulesv1.InfusionAccepts_INFUSION_ACCEPTS_UNSPECIFIED
		}, "table_class.levels[1].features[0].options[0].infusion.accepts"},
		{"a bonus step past the cap", func(h *rulesv1.TableFeature) { h.Options[0].Infusion.BonusSteps[0].Bonus = 4 }, "table_class.levels[1].features[0].options[0].infusion.bonus_steps[0].bonus"},
		{"an infusion with no rule on its host", func(h *rulesv1.TableFeature) { h.InfusionRule = nil }, "table_class.levels[1].features[0].options[0].infusion"},
	}
	for _, tc := range cases {
		class := classWithInfusions()
		tc.edit(class.GetLevels()[1].GetFeatures()[0])
		_, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, class)))
		if got := violationFields(violationsOfErr(t, err)); !slices.Contains(got, tc.field) {
			t.Errorf("%s: violations at %v, want one at %s", tc.name, got, tc.field)
		}
	}
}
