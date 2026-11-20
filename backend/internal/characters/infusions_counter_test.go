package characters

import (
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

const infusionCounterKey = "itens_infundidos"

func artificerBuild() rules.Build {
	return rules.Build{
		BaseScores: map[rules.Ability]int{rules.STR: 15, rules.DEX: 14, rules.CON: 13, rules.INT: 12, rules.WIS: 10, rules.CHA: 8},
		Race:       "race:human", Background: "background:acolyte",
		Classes:        []rules.ClassLevel{{Class: infusionClassKey, Level: 6}},
		FeatureChoices: []string{infusionKeyBlade, infusionKeyPlate, infusionKeyAsh},
	}
}

// The host feature's own "infused items" counter is a second count of what the infusion block counts: the fixture's host
// has the typed limit, so the derived resources leave the counter out and the limit stays on the host.
func TestTheHostsCounterIsNotShownAsASecondCountOfTheLimit(t *testing.T) {
	t.Parallel()
	if !slices.ContainsFunc(infusionHost(t).GetEffects(), func(e *rulesv1.TableEffect) bool {
		return e.GetType() == "resource" && e.GetResource() == infusionCounterKey
	}) {
		t.Fatal("the fixture's host has no resource effect: the test would prove nothing")
	}
	d := rules.Derive(artificerBuild(), infusionContent(t))
	for _, r := range d.Resources {
		if r.Key == infusionCounterKey || r.Source == infusionHostKey {
			t.Errorf("the resources hold %+v, a second count of the infusions the host already limits", r)
		}
	}
	if len(d.InfusionHosts) != 1 || d.InfusionHosts[0].Limit != 3 {
		t.Errorf("the hosts = %+v, want one with the limit 3", d.InfusionHosts)
	}
}

// A feature with the same counter and no infusion rule is no host: its resource stays.
func TestAFeatureWithNoRuleKeepsItsCounter(t *testing.T) {
	t.Parallel()
	pack := proto.CloneOf(loadInfusionPack(t))
	for _, l := range bodyMessage(pack.GetEntries()[0]).(*rulesv1.TableClass).GetLevels() {
		for _, f := range l.GetFeatures() {
			if f.GetNamePt() != infusionHostName {
				continue
			}
			f.InfusionRule = nil
			for _, o := range f.GetOptions() {
				o.Infusion = nil
			}
		}
	}
	d := rules.Derive(artificerBuild(), contentOfPack(t, pack))
	if !slices.ContainsFunc(d.Resources, func(r rules.Resource) bool { return r.Key == infusionCounterKey && r.Max == 3 }) {
		t.Errorf("the resources = %+v, want the counter of the feature that has no infusion rule", d.Resources)
	}
}
