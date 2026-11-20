package characters

import (
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The infusion data of a table feature: the engine reads it from the stored body, and a player's copy of an entry
// goes without it. Every name here is invented.

// infusionHostBody is a class feature that has an infusion rule and two options: a bonus and a resistance.
func infusionHostBody() *rulesv1.TableFeature {
	return &rulesv1.TableFeature{
		NamePt: "Gravar", DescPt: []string{"Escolha infusões."},
		Effects:      []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 2}},
		InfusionRule: &rulesv1.TableInfusionRule{LimitSteps: []*rulesv1.TableInfusionLimitStep{{ClassLevel: 2, Active: 2}, {ClassLevel: 6, Active: 3}}},
		Options: []*rulesv1.TableFeature{
			{NamePt: "Gume manso", DescPt: []string{"Corta um pouco melhor."}, Infusion: &rulesv1.TableInfusion{
				Accepts: rulesv1.InfusionAccepts_INFUSION_ACCEPTS_WEAPON, Effect: rulesv1.InfusionEffect_INFUSION_EFFECT_ITEM_BONUS,
				BonusSteps: []*rulesv1.TableInfusionStep{{ClassLevel: 2, Bonus: 1}, {ClassLevel: 10, Bonus: 2}},
			}},
			{NamePt: "Cobertura morna", DescPt: []string{"Aguenta o calor."}, Infusion: &rulesv1.TableInfusion{
				Accepts: rulesv1.InfusionAccepts_INFUSION_ACCEPTS_ARMOR, Effect: rulesv1.InfusionEffect_INFUSION_EFFECT_RESISTANCE_LABEL,
				RequiresAttunement: true, DamageTypes: []string{"damage-type:fire", "damage-type:cold"},
			}},
			{NamePt: "Traço mudo", Infusion: &rulesv1.TableInfusion{Accepts: rulesv1.InfusionAccepts_INFUSION_ACCEPTS_ANY_OBJECT, Effect: rulesv1.InfusionEffect_INFUSION_EFFECT_NONE}},
		},
	}
}

func TestTheEngineReadsTheInfusionDataOfAFeature(t *testing.T) {
	t.Parallel()
	got := tableFeatureOf(infusionHostBody())
	if got.InfusionRule == nil || !slices.Equal(got.InfusionRule.LimitSteps, []rules.TableInfusionLimitStep{{ClassLevel: 2, Active: 2}, {ClassLevel: 6, Active: 3}}) {
		t.Fatalf("InfusionRule = %+v", got.InfusionRule)
	}
	if got.Infusion != nil {
		t.Errorf("the host has infusion data: %+v", got.Infusion)
	}
	bonus, res, text := got.Options[0].Infusion, got.Options[1].Infusion, got.Options[2].Infusion
	if bonus == nil || bonus.Accepts != rules.InfusionAcceptsWeapon || bonus.Effect != rules.InfusionEffectItemBonus ||
		!slices.Equal(bonus.BonusSteps, []rules.TableInfusionStep{{ClassLevel: 2, Bonus: 1}, {ClassLevel: 10, Bonus: 2}}) {
		t.Errorf("bonus option = %+v", bonus)
	}
	if res == nil || res.Accepts != rules.InfusionAcceptsArmor || res.Effect != rules.InfusionEffectResistanceLabel || !res.RequiresAttunement ||
		!slices.Equal(res.DamageTypes, []string{"damage-type:fire", "damage-type:cold"}) {
		t.Errorf("resistance option = %+v", res)
	}
	if text == nil || text.Accepts != rules.InfusionAcceptsAnyObject || text.Effect != rules.InfusionEffectNone {
		t.Errorf("text-only option = %+v", text)
	}
	if plain := tableFeatureOf(&rulesv1.TableFeature{NamePt: "Simples"}); plain.Infusion != nil || plain.InfusionRule != nil {
		t.Errorf("a feature with no infusion data has some: %+v", plain)
	}
}

func TestAnUnspecifiedInfusionKindIsNotAKindTheEngineKnows(t *testing.T) {
	t.Parallel()
	got := tableInfusionOf(&rulesv1.TableInfusion{Accepts: rulesv1.InfusionAccepts(99), Effect: rulesv1.InfusionEffect_INFUSION_EFFECT_UNSPECIFIED})
	if got.Accepts != "" || got.Effect != "" {
		t.Errorf("tableInfusionOf = %+v, want empty kinds, which the engine refuses", got)
	}
}

// classWithInfusions is a class whose level 2 feature is the host.
func classWithInfusions() *rulesv1.TableClass {
	c := testClass("Engenhista de teste")
	c.Levels[1].Features = []*rulesv1.TableFeature{infusionHostBody()}
	return c
}

// A player's copy of an entry goes without the infusion data, whether or not something in it is hidden.
func TestAPlayersCopyOfAnEntryHasNoInfusionData(t *testing.T) {
	t.Parallel()
	for _, hidden := range []func(string) bool{nil, func(string) bool { return false }} {
		class := classWithInfusions()
		got := forPlayer(entryRow{body: class}, hidden, nil).body.(*rulesv1.TableClass)
		host := got.GetLevels()[1].GetFeatures()[0]
		if host.GetInfusionRule() != nil {
			t.Errorf("the player's copy keeps the rule: %v", host.GetInfusionRule())
		}
		for _, o := range host.GetOptions() {
			if o.GetInfusion() != nil {
				t.Errorf("the player's copy keeps the infusion of %q: %v", o.GetNamePt(), o.GetInfusion())
			}
		}
		if len(host.GetOptions()) != 3 || host.GetOptions()[0].GetNamePt() != "Gume manso" {
			t.Errorf("the options themselves left the copy: %v", host.GetOptions())
		}
		// The master's body is not touched.
		if class.GetLevels()[1].GetFeatures()[0].GetInfusionRule() == nil {
			t.Errorf("the original lost its rule")
		}
	}
	sub := &rulesv1.TableSubclass{NamePt: "Caminho", ClassKey: "class:fighter", Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{infusionHostBody()}}}}
	got := forPlayer(entryRow{body: sub}, nil, nil).body.(*rulesv1.TableSubclass)
	if got.GetLevels()[0].GetFeatures()[0].GetInfusionRule() != nil || got.GetLevels()[0].GetFeatures()[0].GetOptions()[0].GetInfusion() != nil {
		t.Errorf("a subclass's player copy keeps the infusion data")
	}
}

func TestACopyWithNoInfusionDataIsReturnedAsItIs(t *testing.T) {
	t.Parallel()
	class := testClass("Sem infusões")
	if got := forPlayer(entryRow{body: class}, nil, nil).body; !proto.Equal(got, class) {
		t.Errorf("the copy changed: %v", got)
	}
}

// A table option of the set "table_feature" may be an infusion: the engine reads it and a player's copy drops it.
func TestATableOptionsInfusionIsReadAndLeftOutOfAPlayersCopy(t *testing.T) {
	t.Parallel()
	body := &rulesv1.TableOption{
		NamePt: "Raiz Quieta", Set: "table_feature", OfferedBy: "feature:infundir@mesa",
		Infusion: &rulesv1.TableInfusion{Accepts: rulesv1.InfusionAccepts_INFUSION_ACCEPTS_SHIELD, Effect: rulesv1.InfusionEffect_INFUSION_EFFECT_NONE},
	}
	got := tableOptionOf(rules.TableEntry{Key: "option:table_feature:raiz-quieta@mesa"}, body)
	if got.Infusion == nil || got.Infusion.Accepts != rules.InfusionAcceptsShield || got.Infusion.Effect != rules.InfusionEffectNone {
		t.Errorf("tableOptionOf().Infusion = %+v", got.Infusion)
	}
	for _, hidden := range []func(string) bool{nil, func(string) bool { return false }} {
		copied := forPlayer(entryRow{body: body}, hidden, nil).body.(*rulesv1.TableOption)
		if copied.GetInfusion() != nil || copied.GetNamePt() != "Raiz Quieta" {
			t.Errorf("the player's copy = %v, want the option with no infusion", copied)
		}
	}
	if body.GetInfusion() == nil {
		t.Errorf("the original lost its infusion")
	}
}

// Only a class or a subclass can hold infusion data, but a body of another kind that carries some anyway never reaches a
// player: the copy drops it, whether or not something is hidden.
func TestAPlayersCopyOfARaceFeatAndBackgroundHasNoInfusionData(t *testing.T) {
	t.Parallel()
	withData := infusionHostBody
	bodies := map[string]tableBody{
		"race":       &rulesv1.TableRace{NamePt: "Povo", Traits: []*rulesv1.TableFeature{withData()}},
		"subrace":    &rulesv1.TableSubrace{NamePt: "Ramo", Traits: []*rulesv1.TableFeature{withData()}},
		"background": &rulesv1.TableBackground{NamePt: "Ofício", Feature: withData()},
		"feat":       &rulesv1.TableFeat{NamePt: "Talento", Options: []*rulesv1.TableFeature{withData()}},
	}
	for kind, body := range bodies {
		if !carriesInfusions(body) {
			t.Errorf("%s: carriesInfusions = false for a body with infusion data", kind)
		}
		got := forPlayer(entryRow{body: body}, nil, nil).body
		if carriesInfusions(got) {
			t.Errorf("%s: the player's copy still carries infusion data: %v", kind, got)
		}
		if !carriesInfusions(body) {
			t.Errorf("%s: the original lost its data", kind)
		}
	}
}
