package rules

import "testing"

// #580, board 580-05: the row of an option the character picked reads "Opção de <what offers it>", so the engine says
// which feature, trait or feat offers it (Feature.OptionOfPT); the other rows say nothing.

func featureOf(t *testing.T, d Derived, key string) Feature {
	t.Helper()
	for _, f := range d.Features {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("features = %+v, want %q", d.Features, key)
	return Feature{}
}

func TestAnOptionOfATableFeatureNamesTheFeatureThatOffersIt(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, postureOverlay())
	d := Derive(postureBuild(5, postureHeron), c)
	if got := featureOf(t, d, postureHeron).OptionOfPT; got != "Posturas" {
		t.Errorf("option of = %q, want %q", got, "Posturas")
	}
	if got := featureOf(t, d, postureFeature).OptionOfPT; got != "" {
		t.Errorf("the offering feature's own row says %q, want nothing", got)
	}
}

func TestAnSRDFightingStyleNamesTheFeatureThatOffersIt(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 1)
	b.FeatureChoices = []string{"feature:fighter-fighting-style-defense"}
	d := Derive(b, c)
	if got := featureOf(t, d, "feature:fighter-fighting-style-defense").OptionOfPT; got != "Estilo de Luta" {
		t.Errorf("option of = %q, want %q", got, "Estilo de Luta")
	}
	for _, f := range d.Features {
		if f.Key != "feature:fighter-fighting-style-defense" && f.OptionOfPT != "" {
			t.Errorf("%q says it is an option of %q", f.Key, f.OptionOfPT)
		}
	}
}

func TestAnOptionNamesTheKeyOfTheFeatureThatOffersIt(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, postureOverlay())
	d := Derive(postureBuild(5, postureHeron), c)
	if got := featureOf(t, d, postureHeron).OptionOfKey; got != postureFeature {
		t.Errorf("option of key = %q, want %q", got, postureFeature)
	}
	if got := featureOf(t, d, postureFeature).OptionOfKey; got != "" {
		t.Errorf("the offering feature's own row says %q, want nothing", got)
	}
}

// A Fighter and a Paladin each have a feature called "Estilo de Luta": the option says which one offers it by key.
func TestTwoFeaturesWithOneNameAreToldApartByKey(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 2)
	b.Classes = []ClassLevel{{Class: "class:fighter", Level: 1}, {Class: "class:paladin", Level: 2}}
	b.FeatureChoices = []string{"feature:fighter-fighting-style-defense", "feature:fighting-style-dueling"}
	d := Derive(b, c)
	for option, parent := range map[string]string{
		"feature:fighter-fighting-style-defense": "feature:fighter-fighting-style",
		"feature:fighting-style-dueling":         "feature:paladin-fighting-style",
	} {
		f := featureOf(t, d, option)
		if f.OptionOfPT != "Estilo de Luta" || f.OptionOfKey != parent {
			t.Errorf("%s is an option of %q (%q), want %q", option, f.OptionOfKey, f.OptionOfPT, parent)
		}
	}
}
