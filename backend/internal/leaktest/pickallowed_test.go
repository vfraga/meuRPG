package leaktest

import (
	"slices"
	"testing"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The skills the editor offers come from the sources of the sheet, as the table's own reader sees them: a class the table
// switches off adds no key to what a player reads, and the master, who reads what is switched off, still sees its keys.
func TestASwitchedOffTableClassAddsNoSkillKeyToAPlayersList(t *testing.T) {
	p := newChoicesProbe(t)
	const extra = "skill:performance"
	class := p.createEntry(t, tableClass("Menestrel da Mesa", &rulesv1.TableFeature{
		NamePt: "Canto", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: []string{extra}}},
	}))
	has := func(t *testing.T, who reader) (listed, ok bool) {
		t.Helper()
		r := p.preview(who, sheet(class.GetKey(), 1))
		if !r.ok() {
			return false, false
		}
		return slices.Contains(choicesOf(t, r).GetSkillsAllowed().GetKeys(), extra), true
	}
	for _, who := range p.readers() {
		if !who.member {
			continue
		}
		if listed, ok := has(t, who); !ok || !listed {
			t.Fatalf("%s: before the switch the list holds %s = %v (answered %v), the positive control", who.name, extra, listed, ok)
		}
	}
	must(p.w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{
		CampaignId: p.w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: class.GetKey(), Off: true}},
	})))
	for _, who := range p.readers() {
		if !who.member {
			continue
		}
		listed, ok := has(t, who)
		switch {
		case who.name == "the master":
			if !ok || !listed {
				t.Errorf("%s reads what the table switched off, answered %v with %s listed %v", who.name, ok, extra, listed)
			}
		case listed:
			t.Errorf("%s: the list holds %s of a class the table switched off", who.name, extra)
		}
	}
}
