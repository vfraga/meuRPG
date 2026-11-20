package rules

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/golden from the current engine")

// TestDerivePensantusGolden compares Pensantus's whole Derived with
// testdata/golden/pensantus.json, which was checked once against his real
// sheet. Descriptions (SRD text) are left out so the file stays readable.
//
// After a deliberate change, rewrite it with
// `go test ./internal/rules -run Golden -update` and review the diff.
func TestDerivePensantusGolden(t *testing.T) {
	t.Parallel()
	d := Derive(pensantus(), loadForTest(t))
	for i := range d.Features {
		d.Features[i].Description = nil
	}
	got, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "golden", "pensantus.json")
	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Derive(Pensantus) differs from %s; if the change is right, run with -update and review the diff.\ngot:\n%s", path, got)
	}
}

// sageOverlay is the Sage, which is not in the SRD 5.1, as a table background
// written in our own words for the tests (ADR-0008, section 6; ADR-0018): two
// skills, two languages to choose, and a feature shown as a note. It has only the
// mechanical fields, so no book text appears here.
func sageOverlay() Overlay {
	return Overlay{Revision: 1, Backgrounds: []TableBackground{{
		Key: "background:sabio@mesa", NamePT: "Sábio",
		Skills:          []string{"skill:arcana", "skill:history"},
		LanguageChoices: 2,
		Feature: TableFeature{
			Key: "background-feature:contatos-na-biblioteca@mesa", NamePT: "Contatos na biblioteca",
			DescPT:  []string{"Texto de teste escrito para o MeuRPG: anos entre livros ensinaram este personagem onde o saber fica guardado."},
			Effects: []Effect{{Type: "note", TextPT: "Contatos na biblioteca: quando não sabe algo, costuma saber onde ou com quem descobrir."}},
		},
	}}}
}

// TestDeriveWithTheSageOverlay derives Pensantus with the Sage from the table's
// overlay instead of a custom background: the numbers are the same, and the
// background's feature shows.
func TestDeriveWithTheSageOverlay(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, sageOverlay())
	b := pensantus()
	b.Background = "background:sabio@mesa"
	b.Languages = testLanguages
	b.dropCustomBackground()
	if err := Validate(b, c); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	d := Derive(b, c)
	for key, want := range map[string]int{"skill:arcana": 6, "skill:history": 6, "skill:investigation": 6, "skill:insight": 3} {
		if got := skillOf(d, key).Bonus; got != want {
			t.Errorf("%s = %+d, want %+d", key, got, want)
		}
	}
	if d.BackgroundNamePT != "Sábio" || !hasFeature(d, "background-feature:contatos-na-biblioteca@mesa") {
		t.Errorf("background %q, features %+v", d.BackgroundNamePT, d.Features)
	}
	if h, ok := hintFrom(d, "background-feature:contatos-na-biblioteca@mesa"); !ok || h.Mode != "note" {
		t.Errorf("Library Contacts hint = %+v", h)
	}
	if len(d.Issues) != 0 {
		t.Errorf("issues = %v", issueCodes(d))
	}
	if want := loadForTest(t).Version() + "+mesa.1"; d.ContentVersion != want {
		t.Errorf("content version = %q, want %q", d.ContentVersion, want)
	}
}

// TestValidate checks the structural validation of writes.
func TestValidate(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	if err := Validate(pensantus(), c); err != nil {
		t.Fatalf("Validate(Pensantus) = %v", err)
	}
	tests := []struct {
		name  string
		edit  func(b *Build)
		field string
	}{
		{"score above 30", func(b *Build) { b.BaseScores[INT] = 31 }, "full.base_scores.intelligence"},
		{"score below 1", func(b *Build) { b.BaseScores[STR] = 0 }, "full.base_scores.strength"},
		{"missing score", func(b *Build) { delete(b.BaseScores, CHA) }, "full.base_scores.charisma"},
		{"unknown ability", func(b *Build) { b.BaseScores["luck"] = 10 }, "full.base_scores"},
		{"manual bonus too big", func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{DEX: 11} }, "full.extra_ability_bonuses.dexterity"},
		{"no race", func(b *Build) { b.Race = "" }, "full.race_key"},
		{"unknown race", func(b *Build) { b.Race = "race:owlin" }, "full.race_key"},
		{"subrace of another race", func(b *Build) { b.Subrace = "subrace:high-elf" }, "full.subrace_key"},
		{"no class", func(b *Build) { b.Classes = nil }, "full.classes"},
		{"unknown class", func(b *Build) { b.Classes[0].Class = "class:artificer" }, "full.classes[0].class_key"},
		{"level 0", func(b *Build) { b.Classes[0].Level = 0 }, "full.classes[0].level"},
		{"level 21", func(b *Build) { b.Classes[0].Level = 21 }, "full.classes[0].level"},
		{"total above 20", func(b *Build) {
			b.Classes = append(b.Classes, ClassLevel{Class: "class:fighter", Level: 18})
		}, "full.classes"},
		{"class twice", func(b *Build) { b.Classes = append(b.Classes, b.Classes[0]) }, "full.classes[1].class_key"},
		{"subclass of another class", func(b *Build) { b.Classes[0].Subclass = "subclass:life" }, "full.classes[0].subclass_key"},
		{"subclass and custom subclass", func(b *Build) { b.Classes[0].CustomSubclassName = "Cronurgia" }, "full.classes[0]"},
		{"custom subclass too long", func(b *Build) {
			b.Classes[0].Subclass, b.Classes[0].CustomSubclassName = "", "Uma subclasse com um nome longo demais para a ficha"
		}, "full.classes[0].custom_subclass_name"},
		{"unknown background", func(b *Build) {
			b.Background = "background:sage"
			b.dropCustomBackground()
		}, "full.background_key"},
		{"background and custom background", func(b *Build) { b.Background = "background:acolyte" }, "full.custom_background"},
		{"three custom background skills", func(b *Build) {
			b.CustomBackgroundSkills = append(b.CustomBackgroundSkills, "skill:religion")
		}, "full.custom_background.skill_keys"},
		{"unknown skill", func(b *Build) { b.SkillProficiencies[0] = "skill:cooking" }, "full.skill_proficiency_keys[0]"},
		{"repeated skill", func(b *Build) { b.SkillProficiencies[1] = b.SkillProficiencies[0] }, "full.skill_proficiency_keys[1]"},
		{"unknown expertise", func(b *Build) { b.Expertise = []string{"arcana"} }, "full.expertise_skill_keys[0]"},
		{"too many rolls", func(b *Build) { b.HitPoints.Rolls = make([]int, 20) }, "full.hit_points.rolls"},
		{"roll of 13", func(b *Build) { b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{13}} }, "full.hit_points.rolls[0]"},
		{"unknown method", func(b *Build) { b.HitPoints.Method = 7 }, "full.hit_points.method"},
		{"shield as armor", func(b *Build) { b.Armor = "equipment:shield" }, "full.armor_key"},
		{"weapon as armor", func(b *Build) { b.Armor = "equipment:dagger" }, "full.armor_key"},
		{"armor as weapon", func(b *Build) { b.Weapons = []string{"equipment:plate-armor"} }, "full.weapon_keys[0]"},
		{"spell as cantrip", func(b *Build) { b.Cantrips[0] = "spell:fireball" }, "full.cantrip_keys[0]"},
		{"cantrip as spell", func(b *Build) { b.SpellsKnown[0] = "spell:fire-bolt" }, "full.known_spell_keys[0]"},
		{"unknown prepared spell", func(b *Build) { b.SpellsPrepared[0] = "spell:toll-the-dead" }, "full.prepared_spell_keys[0]"},
		{"too many weapons", func(b *Build) {
			for len(b.Weapons) <= MaxWeapons {
				b.Weapons = append(b.Weapons, "equipment:dagger")
			}
		}, "full.weapon_keys"},
		{"mark on a weapon not carried", func(b *Build) { b.WeaponMarks = []WeaponMark{{Weapon: "equipment:dagger", Silvered: true}} }, "full.weapon_marks[0].weapon_key"},
		{"mark twice", func(b *Build) {
			b.Weapons = []string{"equipment:dagger"}
			b.WeaponMarks = []WeaponMark{{Weapon: "equipment:dagger", Silvered: true}, {Weapon: "equipment:dagger", Magical: true}}
		}, "full.weapon_marks[1].weapon_key"},
		{"mark that sets nothing", func(b *Build) {
			b.Weapons = []string{"equipment:dagger"}
			b.WeaponMarks = []WeaponMark{{Weapon: "equipment:dagger"}}
		}, "full.weapon_marks[0]"},
		{"two pact weapons", func(b *Build) {
			b.Weapons = []string{"equipment:dagger", "equipment:rapier"}
			b.WeaponMarks = []WeaponMark{{Weapon: "equipment:dagger", PactWeapon: true}, {Weapon: "equipment:rapier", PactWeapon: true}}
		}, "full.weapon_marks[1].pact_weapon"},
		{"pact mark on a weapon not carried", func(b *Build) { b.WeaponMarks = []WeaponMark{{Weapon: "equipment:dagger", PactWeapon: true}} }, "full.weapon_marks[0].weapon_key"},
		{"too many marks", func(b *Build) { b.WeaponMarks = make([]WeaponMark, MaxWeapons+1) }, "full.weapon_marks"},
		{"option that is not an option", func(b *Build) { b.FeatureChoices = []string{"feature:arcane-recovery"} }, "full.feature_choice_keys[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := pensantus()
			tt.edit(&b)
			err := Validate(b, c)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Validate = %v, want a *ValidationError", err)
			}
			if ve.Field != tt.field {
				t.Errorf("field = %q, want %q (%v)", ve.Field, tt.field, err)
			}
		})
	}

	t.Run("allowed", func(t *testing.T) {
		t.Parallel()
		for name, edit := range map[string]func(b *Build){
			"custom subclass": func(b *Build) { b.Classes[0].Subclass, b.Classes[0].CustomSubclassName = "", "Cronurgia" },
			"SRD background": func(b *Build) {
				b.Background = "background:acolyte"
				b.dropCustomBackground()
			},
			"no background yet":   func(b *Build) { b.dropCustomBackground() },
			"manual bonus of -10": func(b *Build) { b.ExtraAbilityBonuses = map[Ability]int{STR: -10} },
			"a roll above the d6": func(b *Build) { b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{12, 1}} },
			"a pact mark alone": func(b *Build) {
				b.Weapons = []string{"equipment:dagger"}
				b.WeaponMarks = []WeaponMark{{Weapon: "equipment:dagger", PactWeapon: true}}
			},
			"a fighting style option": func(b *Build) { b.FeatureChoices = []string{"feature:fighter-fighting-style-defense"} },
			"a twinned spell option":  func(b *Build) { b.FeatureChoices = []string{"feature:metamagic-twinned-spell"} },
			"a trait option":          func(b *Build) { b.FeatureChoices = []string{"trait:draconic-ancestry-red"} },
		} {
			b := pensantus()
			edit(&b)
			if err := Validate(b, c); err != nil {
				t.Errorf("%s: Validate = %v", name, err)
			}
		}
	})

	t.Run("error message never echoes the value", func(t *testing.T) {
		t.Parallel()
		b := pensantus()
		b.Race = "race:<script>"
		err := Validate(b, c)
		if err == nil || bytes.Contains([]byte(err.Error()), []byte("<script>")) {
			t.Errorf("Validate = %v", err)
		}
	})
}

// TestDeriveDoesNotShareState: Derive may run from many requests at once,
// and must not change the content or leak between calls.
func TestDeriveDoesNotShareState(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	first := Derive(pensantus(), c)
	done := make(chan Derived, 8)
	for range 8 {
		go func() { done <- Derive(pensantus(), c) }()
	}
	for range 8 {
		d := <-done
		if d.HitPointsMax != first.HitPointsMax || !slices.EqualFunc(d.Skills, first.Skills, func(a, b Skill) bool { return a == b }) {
			t.Error("concurrent Derive differs")
		}
	}
}

// dropCustomBackground clears every part of the custom background, as a sheet
// that picks an SRD or table background has.
func (b *Build) dropCustomBackground() {
	b.CustomBackgroundName, b.CustomBackgroundSkills = "", nil
	b.CustomBackgroundProficiencies, b.CustomBackgroundFeatureName, b.CustomBackgroundFeature, b.CustomBackgroundEquipment = nil, "", "", ""
}
