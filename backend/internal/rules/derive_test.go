package rules

import (
	"slices"
	"strings"
	"testing"
)

// pensantus is the table's reference character (ADR-0008): Rock Gnome,
// Wizard 3 (School of Evocation), with the Sage as a custom background
// (the Sage is not in the SRD 5.1), fixed hit points.
func pensantus() Build {
	return Build{
		BaseScores: map[Ability]int{STR: 12, DEX: 16, CON: 15, INT: 16, WIS: 13, CHA: 12},
		Race:       "race:gnome", Subrace: "subrace:rock-gnome",
		Classes:                []ClassLevel{{Class: "class:wizard", Subclass: "subclass:evocation", Level: 3}},
		CustomBackgroundName:   "Sábio",
		CustomBackgroundSkills: []string{"skill:arcana", "skill:history"},
		// SRD 5.1 "Customizing a Background": two languages (tools or languages in
		// any mix), a feature in the player's words and the equipment.
		CustomBackgroundProficiencies: []string{"language:draconic", "language:elvish"},
		CustomBackgroundFeatureName:   "Pesquisador",
		CustomBackgroundFeature:       "Quando você não sabe uma informação, sabe a quem perguntar: bibliotecas, escribas e outros estudiosos.",
		CustomBackgroundEquipment:     "Um tinteiro, uma pena, uma faca pequena e roupas comuns.",
		SkillProficiencies:            []string{"skill:investigation", "skill:insight"},
		Weapons:                       []string{"equipment:quarterstaff"},
		Cantrips:                      []string{"spell:fire-bolt", "spell:ray-of-frost", "spell:minor-illusion"},
		SpellsKnown: []string{
			"spell:magic-missile", "spell:burning-hands", "spell:shield", "spell:mage-armor", "spell:sleep",
			"spell:find-familiar", "spell:detect-magic", "spell:comprehend-languages", "spell:scorching-ray", "spell:web",
		},
		SpellsPrepared: []string{
			"spell:magic-missile", "spell:burning-hands", "spell:shield", "spell:mage-armor", "spell:sleep",
			"spell:scorching-ray", "spell:web",
		},
	}
}

// Small helpers to read a Derived.
func abilityOf(d Derived, a Ability) AbilityScore {
	for _, s := range d.Abilities {
		if s.Ability == a {
			return s
		}
	}
	return AbilityScore{}
}

func saveOf(d Derived, a Ability) SavingThrow {
	for _, s := range d.SavingThrows {
		if s.Ability == a {
			return s
		}
	}
	return SavingThrow{}
}

func skillOf(d Derived, key string) Skill {
	for _, s := range d.Skills {
		if s.Key == key {
			return s
		}
	}
	return Skill{}
}

func hintFrom(d Derived, source string) (Hint, bool) {
	for _, h := range d.Hints {
		if h.Source == source {
			return h, true
		}
	}
	return Hint{}, false
}

func attackOf(d Derived, key string) (Attack, bool) {
	for _, a := range d.Attacks {
		if a.Key == key {
			return a, true
		}
	}
	return Attack{}, false
}

func hasFeature(d Derived, key string) bool {
	return slices.ContainsFunc(d.Features, func(f Feature) bool { return f.Key == key })
}

func issueCodes(d Derived) []string {
	var out []string
	for _, i := range d.Issues {
		out = append(out, i.Code+" "+i.Field)
	}
	return out
}

// TestDerive checks Derive against numbers worked out by hand from the SRD,
// starting with the ADR-0008 table for Pensantus.
func TestDerive(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	d := Derive(pensantus(), c)

	t.Run("Pensantus: abilities", func(t *testing.T) {
		t.Parallel()
		// CON 15+1 = 16 and INT 16+2 = 18; modifiers +1, +3, +3, +4, +1, +1.
		want := map[Ability][2]int{STR: {12, 1}, DEX: {16, 3}, CON: {16, 3}, INT: {18, 4}, WIS: {13, 1}, CHA: {12, 1}}
		for a, w := range want {
			if got := abilityOf(d, a); got.Score != w[0] || got.Modifier != w[1] {
				t.Errorf("%s = %d (%+d), want %d (%+d)", a, got.Score, got.Modifier, w[0], w[1])
			}
		}
	})
	t.Run("Pensantus: wizard 3", func(t *testing.T) {
		t.Parallel()
		if d.TotalLevel != 3 || d.ProficiencyBonus != 2 {
			t.Errorf("level %d, proficiency %+d; want 3, +2", d.TotalLevel, d.ProficiencyBonus)
		}
		if len(d.Spellcasting) != 1 {
			t.Fatalf("spellcasting = %+v", d.Spellcasting)
		}
		sc := d.Spellcasting[0]
		if sc.SaveDC != 14 || sc.AttackBonus != 6 || sc.CantripsKnown != 3 || sc.PreparedMax != 7 || !sc.PreparesSpells {
			t.Errorf("spellcasting = %+v; want DC 14, attack +6, 3 cantrips, 7 prepared (INT 4 + level 3)", sc)
		}
		if !slices.Equal(d.SpellSlots, []int{4, 2, 0, 0, 0, 0, 0, 0, 0}) {
			t.Errorf("slots = %v, want 4 x 1st and 2 x 2nd", d.SpellSlots)
		}
		if d.PactMagic != nil {
			t.Errorf("pact magic = %+v, want none", d.PactMagic)
		}
	})
	t.Run("Pensantus: hit points, AC, speed, senses", func(t *testing.T) {
		t.Parallel()
		// 6 + 3 at level 1, then 4 + 3 twice.
		if d.HitPointsMax != 23 {
			t.Errorf("HP = %d, want 23", d.HitPointsMax)
		}
		if !slices.Equal(d.HitDice, []HitDice{{Die: 6, Count: 3}}) {
			t.Errorf("hit dice = %v, want 3d6", d.HitDice)
		}
		// Unarmored: 10 + DEX. Mage Armor (16) and Shield (21) are spells of
		// the game session, not the sheet.
		if d.ArmorClass != 13 || d.ArmorClassDescription != "Sem armadura" {
			t.Errorf("AC = %d (%s), want 13 (Sem armadura)", d.ArmorClass, d.ArmorClassDescription)
		}
		if d.SpeedWalkFt != 25 {
			t.Errorf("speed = %d ft, want 25", d.SpeedWalkFt)
		}
		if len(d.Senses) != 1 || d.Senses[0].Key != "darkvision" || d.Senses[0].RangeFt != 60 || d.Senses[0].Source != "trait:darkvision" {
			t.Errorf("senses = %+v, want darkvision 60 ft", d.Senses)
		}
		if d.Initiative != 3 {
			t.Errorf("initiative = %+d, want +3", d.Initiative)
		}
	})
	t.Run("Pensantus: saves, skills and passives", func(t *testing.T) {
		t.Parallel()
		if s := saveOf(d, INT); !s.Proficient || s.Bonus != 6 {
			t.Errorf("INT save = %+v, want proficient +6", s)
		}
		if s := saveOf(d, WIS); !s.Proficient || s.Bonus != 3 {
			t.Errorf("WIS save = %+v, want proficient +3", s)
		}
		for key, want := range map[string]int{
			"skill:arcana": 6, "skill:history": 6, "skill:investigation": 6, "skill:insight": 3, "skill:perception": 1,
		} {
			if got := skillOf(d, key).Bonus; got != want {
				t.Errorf("%s = %+d, want %+d", key, got, want)
			}
		}
		if d.PassivePerception != 11 {
			t.Errorf("passive Perception = %d, want 11", d.PassivePerception)
		}
	})
	t.Run("Pensantus: hints", func(t *testing.T) {
		t.Parallel()
		// Artificer's Lore: History about magic items with twice the
		// proficiency bonus, +8, as a hint.
		h, ok := hintFrom(d, "trait:artificers-lore")
		if !ok || h.Mode != "bonus" || h.Value != 8 || h.Target != "skill:history" {
			t.Errorf("Artificer's Lore hint = %+v, want History +8", h)
		}
		h, ok = hintFrom(d, "trait:gnome-cunning")
		if !ok || h.Mode != "advantage" || !slices.Equal(h.Targets, []string{"save.int", "save.wis", "save.cha"}) || !slices.Contains(h.Tags, "against:magic") {
			t.Errorf("Gnome Cunning hint = %+v, want advantage on INT, WIS and CHA saves against magic", h)
		}
		// Arcane Recovery: up to half the wizard level, rounded up: 2.
		h, ok = hintFrom(d, "feature:arcane-recovery")
		if !ok || !strings.Contains(h.TextPT, "até 2 níveis") {
			t.Errorf("Arcane Recovery hint = %+v, want 2 levels", h)
		}
	})
	t.Run("Pensantus: attacks", func(t *testing.T) {
		t.Parallel()
		for key, want := range map[string][2]any{
			"equipment:quarterstaff": {3, "1d6+1"},
			"spell:fire-bolt":        {6, "1d10"},
			"spell:ray-of-frost":     {6, "1d8"},
		} {
			a, ok := attackOf(d, key)
			if !ok || a.AttackBonus != want[0] || a.Damage != want[1] {
				t.Errorf("%s = %+v, want %+d %s", key, a, want[0], want[1])
			}
		}
		if a, _ := attackOf(d, "equipment:quarterstaff"); a.VersatileDamage != "1d8+1" || a.DamageTypeNamePT != "concussão" {
			t.Errorf("quarterstaff = %+v, want versatile 1d8+1 concussão", a)
		}
	})
	t.Run("Pensantus: multiclass prerequisites", func(t *testing.T) {
		t.Parallel()
		// ADR-0008: Fighter allowed (DEX 16 and INT 18, both at least 13);
		// Paladin refused (STR 12 below 13).
		b := pensantus()
		b.Classes = append(b.Classes, ClassLevel{Class: "class:fighter", Level: 1})
		if codes := issueCodes(Derive(b, c)); slices.ContainsFunc(codes, func(s string) bool { return strings.HasPrefix(s, IssueMulticlass) }) {
			t.Errorf("wizard 3 / fighter 1: issues = %v", codes)
		}
		b.Classes[1].Class = "class:paladin"
		if codes := issueCodes(Derive(b, c)); !slices.Contains(codes, IssueMulticlass+" full.classes[1].class_key") {
			t.Errorf("wizard 3 / paladin 1: issues = %v", codes)
		}
	})
	t.Run("Pensantus: names, features, spells, no issues", func(t *testing.T) {
		t.Parallel()
		if d.RaceNamePT != "Gnomo" || d.SubraceNamePT != "Gnomo das Rochas" || d.BackgroundNamePT != "Sábio" {
			t.Errorf("names = %q / %q / %q", d.RaceNamePT, d.SubraceNamePT, d.BackgroundNamePT)
		}
		if len(d.Classes) != 1 || d.Classes[0].NamePT != "Mago" || d.Classes[0].SubclassNamePT != "Escola de Evocação" {
			t.Errorf("classes = %+v", d.Classes)
		}
		for _, key := range []string{"trait:darkvision", "trait:gnome-cunning", "trait:artificers-lore", "trait:tinker", "feature:arcane-recovery", "feature:sculpt-spells", "feature:evocation-savant"} {
			if !hasFeature(d, key) {
				t.Errorf("missing feature %s", key)
			}
		}
		if len(d.Spells) != 13 {
			t.Errorf("%d spells on the sheet, want 13", len(d.Spells))
		}
		for _, s := range d.Spells {
			wantPrepared := s.Spell.Level == 0 || slices.Contains(pensantus().SpellsPrepared, s.Spell.Key)
			if s.Prepared != wantPrepared {
				t.Errorf("%s prepared = %v, want %v", s.Spell.Key, s.Prepared, wantPrepared)
			}
		}
		if len(d.Issues) != 0 {
			t.Errorf("issues = %v, want none", issueCodes(d))
		}
		if !slices.ContainsFunc(d.Proficiencies, func(p Proficiency) bool { return p.Key == "proficiency:tinkers-tools" }) {
			t.Error("Tinker should give tinker's tools")
		}
	})
}

// standard is a human with the standard array (15, 14, 13, 12, 10, 8), so
// every score gets +1: STR 16 (+3), DEX 15 (+2), CON 14 (+2), INT 13 (+1),
// WIS 11 (+0), CHA 9 (-1).
func standard(class string, level int) Build {
	return Build{
		BaseScores: map[Ability]int{STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8},
		Race:       "race:human",
		Background: "background:acolyte",
		Classes:    []ClassLevel{{Class: class, Level: level}},
	}
}

// TestDeriveEachClassAtLevel1 has one row per class, worked out from the SRD
// class tables.
func TestDeriveEachClassAtLevel1(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	tests := []struct {
		class        string
		hp, ac       int
		saves        []Ability
		dc, cantrips int
		known        int
		prepared     int
		slots1       int
	}{
		{"class:barbarian", 14, 14, []Ability{STR, CON}, 0, 0, 0, 0, 0}, // AC 10 + DEX 2 + CON 2
		{"class:bard", 10, 12, []Ability{DEX, CHA}, 9, 2, 4, 0, 2},
		{"class:cleric", 10, 12, []Ability{WIS, CHA}, 10, 3, 0, 1, 2},
		{"class:druid", 10, 12, []Ability{INT, WIS}, 10, 2, 0, 1, 2},
		{"class:fighter", 12, 12, []Ability{STR, CON}, 0, 0, 0, 0, 0},
		{"class:monk", 10, 12, []Ability{STR, DEX}, 0, 0, 0, 0, 0}, // AC 10 + DEX 2 + WIS 0
		{"class:paladin", 12, 12, []Ability{WIS, CHA}, 0, 0, 0, 0, 0},
		{"class:ranger", 12, 12, []Ability{STR, DEX}, 0, 0, 0, 0, 0},
		{"class:rogue", 10, 12, []Ability{DEX, INT}, 0, 0, 0, 0, 0},
		{"class:sorcerer", 8, 12, []Ability{CON, CHA}, 9, 4, 2, 0, 2},
		{"class:warlock", 10, 12, []Ability{WIS, CHA}, 9, 2, 2, 0, 0},
		{"class:wizard", 8, 12, []Ability{INT, WIS}, 11, 3, 0, 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.class, func(t *testing.T) {
			t.Parallel()
			d := Derive(standard(tt.class, 1), c)
			if d.HitPointsMax != tt.hp || d.ArmorClass != tt.ac || d.SpeedWalkFt != 30 || d.ProficiencyBonus != 2 {
				t.Errorf("HP %d, AC %d, speed %d, prof %d; want %d, %d, 30, 2", d.HitPointsMax, d.ArmorClass, d.SpeedWalkFt, d.ProficiencyBonus, tt.hp, tt.ac)
			}
			for _, s := range d.SavingThrows {
				if want := slices.Contains(tt.saves, s.Ability); s.Proficient != want {
					t.Errorf("save %s proficient = %v, want %v", s.Ability, s.Proficient, want)
				}
			}
			if tt.dc == 0 {
				if len(d.Spellcasting) != 0 {
					t.Errorf("spellcasting = %+v, want none at level 1", d.Spellcasting)
				}
				return
			}
			if len(d.Spellcasting) != 1 {
				t.Fatalf("spellcasting = %+v", d.Spellcasting)
			}
			sc := d.Spellcasting[0]
			if sc.SaveDC != tt.dc || sc.CantripsKnown != tt.cantrips || sc.SpellsKnownMax != tt.known || sc.PreparedMax != tt.prepared {
				t.Errorf("spellcasting = %+v; want DC %d, %d cantrips, %d known, %d prepared", sc, tt.dc, tt.cantrips, tt.known, tt.prepared)
			}
			if d.SpellSlots[0] != tt.slots1 {
				t.Errorf("1st-level slots = %d, want %d", d.SpellSlots[0], tt.slots1)
			}
			if tt.class == "class:warlock" && (d.PactMagic == nil || *d.PactMagic != PactMagic{SlotLevel: 1, Slots: 1}) {
				t.Errorf("pact magic = %+v, want 1 slot of 1st level", d.PactMagic)
			}
		})
	}
}

// TestDeriveEachRace has one row per race (with its SRD subrace), on a
// level 1 wizard with every base score 10.
func TestDeriveEachRace(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	tests := []struct {
		race, subrace string
		speed         int
		darkvision    bool
		bonuses       map[Ability]int
		hp            int
		check         func(t *testing.T, d Derived)
	}{
		{"race:dwarf", "subrace:hill-dwarf", 25, true, map[Ability]int{CON: 2, WIS: 1}, 8, nil}, // 6 + CON 1 + Dwarven Toughness 1
		{"race:elf", "subrace:high-elf", 30, true, map[Ability]int{DEX: 2, INT: 1}, 6, func(t *testing.T, d Derived) {
			t.Helper()
			if s := skillOf(d, "skill:perception"); s.Proficiency != ProficiencyFull || s.Bonus != 2 {
				t.Errorf("Keen Senses: perception = %+v, want proficient +2", s)
			}
		}},
		{"race:halfling", "subrace:lightfoot-halfling", 25, false, map[Ability]int{DEX: 2, CHA: 1}, 6, nil},
		{"race:human", "", 30, false, map[Ability]int{STR: 1, DEX: 1, CON: 1, INT: 1, WIS: 1, CHA: 1}, 6, nil},
		{"race:dragonborn", "", 30, false, map[Ability]int{STR: 2, CHA: 1}, 6, func(t *testing.T, d Derived) {
			t.Helper()
			// Breath weapon DC: 8 + CON 0 + proficiency 2.
			if h, ok := hintFrom(d, "trait:breath-weapon"); !ok || !strings.Contains(h.TextPT, "CD 10") {
				t.Errorf("breath weapon hint = %+v, want CD 10", h)
			}
		}},
		{"race:gnome", "subrace:rock-gnome", 25, true, map[Ability]int{INT: 2, CON: 1}, 6, nil},
		{"race:half-elf", "", 30, true, map[Ability]int{CHA: 2}, 6, func(t *testing.T, d Derived) {
			t.Helper()
			// The two +1 are the player's choice: a reminder, not a number.
			if h, ok := hintFrom(d, "race:half-elf"); !ok || !strings.Contains(h.TextPT, "+1 em 2 habilidades") {
				t.Errorf("half-elf hint = %+v", h)
			}
		}},
		{"race:half-orc", "", 30, true, map[Ability]int{STR: 2, CON: 1}, 6, func(t *testing.T, d Derived) {
			t.Helper()
			if s := skillOf(d, "skill:intimidation"); s.Proficiency != ProficiencyFull {
				t.Errorf("Menacing: intimidation = %+v, want proficient", s)
			}
		}},
		{"race:tiefling", "", 30, true, map[Ability]int{INT: 1, CHA: 2}, 6, nil},
	}
	for _, tt := range tests {
		t.Run(tt.race, func(t *testing.T) {
			t.Parallel()
			b := Build{
				BaseScores: map[Ability]int{STR: 10, DEX: 10, CON: 10, INT: 10, WIS: 10, CHA: 10},
				Race:       tt.race, Subrace: tt.subrace, Background: "background:acolyte",
				Classes: []ClassLevel{{Class: "class:wizard", Level: 1}},
			}
			d := Derive(b, c)
			if d.SpeedWalkFt != tt.speed {
				t.Errorf("speed = %d, want %d", d.SpeedWalkFt, tt.speed)
			}
			if got := len(d.Senses) == 1 && d.Senses[0].RangeFt == 60; got != tt.darkvision {
				t.Errorf("darkvision = %v, want %v (%+v)", got, tt.darkvision, d.Senses)
			}
			for _, a := range AllAbilities() {
				if got := abilityOf(d, a).RaceBonus; got != tt.bonuses[a] {
					t.Errorf("%s race bonus = %d, want %d", a, got, tt.bonuses[a])
				}
			}
			if d.HitPointsMax != tt.hp {
				t.Errorf("HP = %d, want %d", d.HitPointsMax, tt.hp)
			}
			if len(d.Languages) == 0 || d.Languages[0].Key != "language:common" {
				t.Errorf("languages = %+v, want Common first", d.Languages)
			}
			if tt.check != nil {
				tt.check(t, d)
			}
		})
	}
}

// TestDeriveRules covers the rules that do not show on Pensantus.
func TestDeriveRules(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)

	t.Run("fighter with chain mail, shield and the Defense style", func(t *testing.T) {
		t.Parallel()
		b := standard("class:fighter", 1)
		b.Armor, b.Shield = "equipment:chain-mail", true
		b.FeatureChoices = []string{"feature:fighter-fighting-style-defense"}
		d := Derive(b, c)
		// Chain mail 16, shield +2, Defense +1.
		if d.ArmorClass != 19 || d.ArmorClassDescription != "Cota de malha + escudo" {
			t.Errorf("AC = %d (%s), want 19 (Cota de malha + escudo)", d.ArmorClass, d.ArmorClassDescription)
		}
		if _, ok := hintFrom(d, "equipment:chain-mail"); !ok {
			t.Error("chain mail should give a stealth disadvantage hint")
		}
		if !hasFeature(d, "feature:fighter-fighting-style-defense") {
			t.Error("the chosen fighting style should be listed")
		}
	})
	t.Run("medium armor caps DEX at +2", func(t *testing.T) {
		t.Parallel()
		b := standard("class:fighter", 1)
		b.BaseScores[DEX] = 17 // 18 with the human bonus, +4
		b.Armor = "equipment:scale-mail"
		if d := Derive(b, c); d.ArmorClass != 16 {
			t.Errorf("AC = %d, want 14 + 2 = 16", d.ArmorClass)
		}
	})
	t.Run("an option without its feature does nothing", func(t *testing.T) {
		t.Parallel()
		b := standard("class:wizard", 1)
		b.FeatureChoices = []string{"feature:fighter-fighting-style-defense"}
		d := Derive(b, c)
		if d.ArmorClass != 12 || hasFeature(d, "feature:fighter-fighting-style-defense") {
			t.Errorf("AC %d, features %v: the wizard has no fighting style", d.ArmorClass, d.Features)
		}
		if !slices.Contains(issueCodes(d), IssueUnknownKey+" full.feature_choice_keys[0]") {
			t.Errorf("issues = %v", issueCodes(d))
		}
	})
	t.Run("monk 3: unarmored defense and movement", func(t *testing.T) {
		t.Parallel()
		b := standard("class:monk", 3)
		b.BaseScores[DEX], b.BaseScores[WIS] = 15, 13 // DEX 16 (+3), WIS 14 (+2)
		d := Derive(b, c)
		if d.ArmorClass != 15 || d.SpeedWalkFt != 40 {
			t.Errorf("AC %d, speed %d; want 15 and 40", d.ArmorClass, d.SpeedWalkFt)
		}
		b.Shield = true
		// With a shield, Unarmored Defense and Movement stop: 10 + DEX 3 +
		// shield 2.
		if d := Derive(b, c); d.ArmorClass != 15 || d.SpeedWalkFt != 30 {
			t.Errorf("with a shield: AC %d, speed %d; want 15 and 30", d.ArmorClass, d.SpeedWalkFt)
		}
	})
	t.Run("monk 5: a dagger uses DEX and the martial arts die", func(t *testing.T) {
		t.Parallel()
		b := standard("class:monk", 5)
		b.BaseScores[STR], b.BaseScores[DEX] = 8, 15 // STR 9 (-1), DEX 16 (+3)
		b.Weapons = []string{"equipment:dagger"}
		a, _ := attackOf(Derive(b, c), "equipment:dagger")
		if a.Ability != DEX || a.AttackBonus != 6 || a.Damage != "1d6+3" {
			t.Errorf("dagger = %+v, want DEX, +6, 1d6+3", a)
		}
	})
	t.Run("barbarian 5: fast movement, unless in heavy armor", func(t *testing.T) {
		t.Parallel()
		b := standard("class:barbarian", 5)
		if d := Derive(b, c); d.SpeedWalkFt != 40 || d.ProficiencyBonus != 3 {
			t.Errorf("speed %d, prof %d; want 40 and 3", d.SpeedWalkFt, d.ProficiencyBonus)
		}
		b.Armor = "equipment:chain-mail"
		d := Derive(b, c)
		if d.SpeedWalkFt != 30 {
			t.Errorf("speed in heavy armor = %d, want 30", d.SpeedWalkFt)
		}
		if !slices.Contains(issueCodes(d), IssueArmorProficiency+" full.armor_key") {
			t.Errorf("issues = %v: a barbarian is not proficient in heavy armor", issueCodes(d))
		}
	})
	t.Run("bard 2: Jack of All Trades", func(t *testing.T) {
		t.Parallel()
		d := Derive(standard("class:bard", 2), c)
		if s := skillOf(d, "skill:arcana"); s.Proficiency != ProficiencyHalf || s.Bonus != 2 {
			t.Errorf("arcana = %+v, want half: INT +1 + 1", s)
		}
		if d.Initiative != 3 {
			t.Errorf("initiative = %+d, want DEX +2 + 1", d.Initiative)
		}
	})
	t.Run("rogue 1: expertise", func(t *testing.T) {
		t.Parallel()
		b := standard("class:rogue", 1)
		b.SkillProficiencies = []string{"skill:stealth", "skill:acrobatics", "skill:perception", "skill:investigation"}
		b.Expertise = []string{"skill:stealth", "skill:perception"}
		d := Derive(b, c)
		if s := skillOf(d, "skill:stealth"); s.Proficiency != ProficiencyExpertise || s.Bonus != 6 {
			t.Errorf("stealth = %+v, want expertise +6", s)
		}
		if len(d.Issues) != 0 {
			t.Errorf("issues = %v, want none", issueCodes(d))
		}
		b.Expertise = append(b.Expertise, "skill:acrobatics")
		if d := Derive(b, c); !slices.Contains(issueCodes(d), IssueExpertise+" full.expertise_skill_keys") {
			t.Errorf("3 expertise at rogue 1: issues = %v", issueCodes(d))
		}
	})
	t.Run("draconic sorcerer 1", func(t *testing.T) {
		t.Parallel()
		b := standard("class:sorcerer", 1)
		b.Classes[0].Subclass = "subclass:draconic"
		d := Derive(b, c)
		// HP 6 + CON 2 + 1; AC 13 + DEX 2.
		if d.HitPointsMax != 9 || d.ArmorClass != 15 {
			t.Errorf("HP %d, AC %d; want 9 and 15", d.HitPointsMax, d.ArmorClass)
		}
	})
	t.Run("warlock 2 with Agonizing Blast", func(t *testing.T) {
		t.Parallel()
		b := standard("class:warlock", 2)
		b.BaseScores[CHA] = 15 // 16, +3
		b.Cantrips = []string{"spell:eldritch-blast"}
		b.FeatureChoices = []string{"feature:eldritch-invocation-agonizing-blast"}
		d := Derive(b, c)
		a, _ := attackOf(d, "spell:eldritch-blast")
		if a.AttackBonus != 5 || a.Damage != "1d10+3" {
			t.Errorf("eldritch blast = %+v, want +5 and 1d10+3", a)
		}
		if d.PactMagic == nil || *d.PactMagic != (PactMagic{SlotLevel: 1, Slots: 2}) {
			t.Errorf("pact magic = %+v, want 2 slots of 1st level", d.PactMagic)
		}
	})
	t.Run("cleric: save cantrip and domain spells", func(t *testing.T) {
		t.Parallel()
		b := standard("class:cleric", 3)
		b.Classes[0].Subclass = "subclass:life"
		b.BaseScores[WIS] = 15 // 16, +3
		b.Cantrips = []string{"spell:sacred-flame"}
		b.SpellsPrepared = []string{"spell:cure-wounds", "spell:healing-word"}
		d := Derive(b, c)
		a, _ := attackOf(d, "spell:sacred-flame")
		if a.SaveDC != 13 || a.SaveAbility != DEX || a.Damage != "1d8" {
			t.Errorf("sacred flame = %+v, want DC 13 DEX, 1d8", a)
		}
		// Life domain: bless, cure wounds (1st), lesser restoration and
		// spiritual weapon (3rd) are always prepared.
		prepared := 0
		for _, s := range d.Spells {
			if s.Prepared && s.Spell.Level > 0 {
				prepared++
			}
		}
		if prepared != 5 {
			t.Errorf("%d prepared spells, want healing word + 4 domain spells", prepared)
		}
		if !slices.ContainsFunc(d.Proficiencies, func(p Proficiency) bool { return p.Key == "proficiency:heavy-armor" }) {
			t.Error("the life domain gives heavy armor")
		}
	})
	t.Run("multiclass slots: wizard 3 / cleric 2", func(t *testing.T) {
		t.Parallel()
		b := standard("class:wizard", 3)
		b.BaseScores[WIS] = 13
		b.Classes = append(b.Classes, ClassLevel{Class: "class:cleric", Level: 2})
		d := Derive(b, c)
		// Caster level 5: 4, 3, 2.
		if !slices.Equal(d.SpellSlots[:3], []int{4, 3, 2}) || len(d.Spellcasting) != 2 {
			t.Errorf("slots %v, spellcasting %d; want 4/3/2 and two classes", d.SpellSlots, len(d.Spellcasting))
		}
		// Saves only from the first class; 6 + 2 + (4 + 2) x 2 + (5 + 2) x 2 = 34.
		if s := saveOf(d, WIS); !s.Proficient || saveOf(d, CHA).Proficient {
			t.Errorf("saves: WIS %+v, CHA %+v", s, saveOf(d, CHA))
		}
		if d.HitPointsMax != 34 {
			t.Errorf("HP = %d, want 34", d.HitPointsMax)
		}
	})
	t.Run("multiclass slots: paladin 2 / ranger 2 and warlock pact apart", func(t *testing.T) {
		t.Parallel()
		b := standard("class:paladin", 2)
		b.BaseScores[WIS], b.BaseScores[CHA] = 13, 13
		b.Classes = append(b.Classes, ClassLevel{Class: "class:ranger", Level: 2}, ClassLevel{Class: "class:warlock", Level: 1})
		d := Derive(b, c)
		// Half of 2 + half of 2 = caster level 2: three 1st-level slots.
		if d.SpellSlots[0] != 3 || d.PactMagic == nil || d.PactMagic.Slots != 1 {
			t.Errorf("slots %v, pact %+v; want 3 and a pact slot", d.SpellSlots, d.PactMagic)
		}
		if slices.Contains(issueCodes(d), IssueMulticlass+" full.classes[0].class_key") {
			t.Errorf("issues = %v: STR 16 and CHA 14 meet the paladin's prerequisites", issueCodes(d))
		}
	})
	t.Run("multiclass prerequisites", func(t *testing.T) {
		t.Parallel()
		b := standard("class:wizard", 1)
		b.Classes = append(b.Classes, ClassLevel{Class: "class:paladin", Level: 1})
		// CHA 9 is below the paladin's 13; INT 13 meets the wizard's.
		d := Derive(b, c)
		if !slices.Contains(issueCodes(d), IssueMulticlass+" full.classes[1].class_key") || slices.Contains(issueCodes(d), IssueMulticlass+" full.classes[0].class_key") {
			t.Errorf("issues = %v", issueCodes(d))
		}
	})
	t.Run("rolled hit points", func(t *testing.T) {
		t.Parallel()
		b := standard("class:fighter", 3)
		b.HitPoints = HitPoints{Method: HitPointsRolled, Rolls: []int{10, 1}}
		// 10 + 2, 10 + 2, 1 + 2.
		if d := Derive(b, c); d.HitPointsMax != 27 || len(d.Issues) != 1 { // one issue: skills not chosen
			t.Errorf("HP = %d, issues %v; want 27", d.HitPointsMax, issueCodes(d))
		}
		b.HitPoints.Rolls = []int{12}
		d := Derive(b, c)
		// 12 does not fit a d10 (average 6 instead); the missing roll uses 6.
		if d.HitPointsMax != 12+8+8 {
			t.Errorf("HP = %d, want 28", d.HitPointsMax)
		}
		codes := issueCodes(d)
		if !slices.Contains(codes, IssueHitPointRolls+" full.hit_points.rolls[0]") || !slices.Contains(codes, IssueHitPointRolls+" full.hit_points.rolls") {
			t.Errorf("issues = %v", codes)
		}
	})
	t.Run("skill counts", func(t *testing.T) {
		t.Parallel()
		b := pensantus()
		b.SkillProficiencies = append(b.SkillProficiencies, "skill:arcana", "skill:history") // the background's: harmless
		if d := Derive(b, c); len(d.Issues) != 0 {
			t.Errorf("issues = %v, want none", issueCodes(d))
		}
		b.SkillProficiencies = append(b.SkillProficiencies, "skill:medicine")
		if d := Derive(b, c); !slices.Contains(issueCodes(d), IssueSkillCount+" full.skill_proficiency_keys") {
			t.Errorf("3 wizard skills: issues = %v", issueCodes(d))
		}
		b.CustomBackgroundSkills = []string{"skill:arcana"}
		if d := Derive(b, c); !slices.Contains(issueCodes(d), IssueSkillCount+" full.custom_background.skill_keys") {
			t.Errorf("1 background skill: issues = %v", issueCodes(d))
		}
	})
	t.Run("spell issues", func(t *testing.T) {
		t.Parallel()
		b := pensantus()
		b.Cantrips = append(b.Cantrips, "spell:sacred-flame")                  // not a wizard cantrip, and a 4th one
		b.SpellsKnown = append(b.SpellsKnown, "spell:fireball", "spell:bless") // 3rd level, and a cleric spell
		b.SpellsPrepared = append(b.SpellsPrepared, "spell:detect-magic")      // 8 prepared
		codes := issueCodes(Derive(b, c))
		for _, want := range []string{
			IssueSpellNotOnList + " full.cantrip_keys[3]",
			IssueSpellCount + " full.cantrip_keys",
			IssueSpellLevel + " full.known_spell_keys[10]",
			IssueSpellNotOnList + " full.known_spell_keys[11]",
			IssueSpellCount + " full.prepared_spell_keys",
		} {
			if !slices.Contains(codes, want) {
				t.Errorf("missing issue %q in %v", want, codes)
			}
		}
	})
	t.Run("unknown keys become issues, never panics", func(t *testing.T) {
		t.Parallel()
		b := Build{
			BaseScores: map[Ability]int{STR: 10},
			Race:       "race:elf", Subrace: "subrace:rock-gnome",
			Classes:    []ClassLevel{{Class: "class:artificer", Level: 3}, {Class: "class:wizard", Subclass: "subclass:life", Level: 30}},
			Background: "background:sage", Armor: "equipment:shield",
			Weapons:  []string{"equipment:lightsaber"},
			Cantrips: []string{"spell:nope", "spell:fireball"}, SpellsKnown: []string{"spell:fire-bolt"},
			Expertise:      []string{"skill:cooking"},
			FeatureChoices: []string{"trait:draconic-ancestry-red", "class:wizard"},
			HitPoints:      HitPoints{Method: HitPointsRolled, Rolls: []int{-3, 99}},
		}
		d := Derive(b, c)
		if len(d.Issues) < 10 {
			t.Errorf("issues = %v, want many", issueCodes(d))
		}
		if d.TotalLevel != MaxLevel || d.ContentVersion != c.Version() {
			t.Errorf("level %d, version %q", d.TotalLevel, d.ContentVersion)
		}
		if d := Derive(Build{}, c); len(d.Abilities) != 6 || len(d.Skills) != 18 || len(d.SavingThrows) != 6 {
			t.Errorf("an empty build still shows the sheet: %+v", d)
		}
	})
}

// TestSummary checks the light lookup for list rows.
func TestSummary(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	s := c.Summary(pensantus())
	if s != (Summary{RaceNamePT: "Gnomo das Rochas", ClassSummaryPT: "Mago 3", TotalLevel: 3}) {
		t.Errorf("Summary = %+v", s)
	}
	b := standard("class:fighter", 2)
	b.Classes = append(b.Classes, ClassLevel{Class: "class:wizard", Level: 1}, ClassLevel{Class: "class:nope", Level: 1})
	if s := c.Summary(b); s.ClassSummaryPT != "Guerreiro 2 / Mago 1" || s.RaceNamePT != "Humano" || s.TotalLevel != 3 {
		t.Errorf("Summary = %+v", s)
	}
	if got := c.NamePT("class:wizard"); got != "Mago" {
		t.Errorf("NamePT(class:wizard) = %q", got)
	}
	if got := c.NamePT("nope"); got != "" {
		t.Errorf("NamePT(nope) = %q, want empty", got)
	}
}

// TestEveryClassAndRaceDerivesAtLevels1To20 is a smoke test: every class,
// with and without its subclass, on every race, at every level, derives
// without panicking and with sane numbers.
func TestEveryClassAndRaceDerivesAtLevels1To20(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	cat := c.Catalog()
	for _, cl := range cat.Classes {
		for _, race := range cat.Races {
			subraces := append([]string{""}, race.Subraces...)
			for _, sub := range subraces {
				for level := 1; level <= MaxLevel; level++ {
					b := standard(cl.Key, level)
					b.Race, b.Subrace = race.Key, sub
					if level >= cl.SubclassLevel && len(cl.Subclasses) > 0 {
						b.Classes[0].Subclass = cl.Subclasses[0]
					}
					if err := Validate(b, c); err != nil {
						t.Fatalf("%s %s %d: Validate: %v", cl.Key, race.Key, level, err)
					}
					d := Derive(b, c)
					name := cl.Key + " " + race.Key + " " + sub
					switch {
					case d.TotalLevel != level:
						t.Errorf("%s %d: level %d", name, level, d.TotalLevel)
					case d.ProficiencyBonus != 2+(level-1)/4:
						t.Errorf("%s %d: proficiency %d", name, level, d.ProficiencyBonus)
					case d.HitPointsMax < level || d.HitPointsMax > 20*level:
						t.Errorf("%s %d: HP %d", name, level, d.HitPointsMax)
					case d.ArmorClass < 10 || d.ArmorClass > 30:
						t.Errorf("%s %d: AC %d", name, level, d.ArmorClass)
					case d.SpeedWalkFt < 25 || d.SpeedWalkFt > 60:
						t.Errorf("%s %d: speed %d", name, level, d.SpeedWalkFt)
					case len(d.Skills) != 18 || len(d.SavingThrows) != 6 || len(d.Abilities) != 6 || len(d.SpellSlots) != 9:
						t.Errorf("%s %d: incomplete sheet", name, level)
					}
					for _, i := range d.Issues {
						if i.Code == IssueFormula || i.Code == IssueUnknownKey {
							t.Errorf("%s %d: %+v", name, level, i)
						}
					}
				}
			}
		}
	}
}

// BenchmarkDerive measures one sheet read. Derive runs on every GetCharacter.
func BenchmarkDerive(b *testing.B) {
	c := loadForTest(b)
	build := pensantus()
	b.ReportAllocs()
	for b.Loop() {
		_ = Derive(build, c)
	}
}

// TestHitPointsFromEffects: HitPointsFromEffects is what the "hp.max" effects
// add to the dice and the Constitution modifier, with the floor of one hit
// point per level kept inside the dice part.
func tableRaceOf(b Build, race string) Build {
	b.Race = race
	return b
}

func TestHitPointsFromEffects(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	race, _, _ := tableMisc()
	race.Traits = []TableFeature{{
		Key: "trait:vigor-fragil" + tableSuffix, NamePT: "Vigor frágil", DescPT: []string{"Texto de teste."},
		Effects: []Effect{{Type: "modifier", Target: "hp.max", Mode: "add", Value: "-2"}},
	}}
	table := withOverlay(t, Overlay{Revision: 1, Races: []TableRace{race}})

	hillDwarf := func(class string, level, con int) Build {
		b := standard(class, level)
		b.Race, b.Subrace = "race:dwarf", "subrace:hill-dwarf"
		b.BaseScores[CON] = con
		return b
	}
	human := func(class string, level, con int) Build {
		b := standard(class, level)
		b.BaseScores[CON] = con
		return b
	}
	tableRace := human("class:fighter", 3, 14)
	tableRace.Race = race.Key
	// A table race whose trait takes hit points away without limit: the maximum stops at one per level.
	draining := race
	draining.Traits = []TableFeature{{
		Key: "trait:dreno" + tableSuffix, NamePT: "Dreno", DescPT: []string{"Texto de teste."},
		Effects: []Effect{{Type: "modifier", Target: "hp.max", Mode: "add", Value: "-1000"}},
	}}
	// Every build starts from standard()'s human, who adds 1 to each score.
	tests := []struct {
		name         string
		content      *Content
		build        Build
		fromEffects  int
		hitPointsMax int
	}{
		{"no effect", srd, human("class:fighter", 3, 14), 0, 12 + 2*(6+2)},                                    // CON 15, modifier 2
		{"Dwarven Toughness adds one per level", srd, hillDwarf("class:fighter", 3, 12), 3, 12 + 2*(6+2) + 3}, // CON 14, modifier 2
		{"a table effect may subtract", table, tableRace, -2, 12 + 2*(6+2) - 2},                               // CON 15, modifier 2
		{"the floor of one per level is not an effect", srd, human("class:wizard", 3, 2), 0, 2 + 1 + 1},       // CON 3, modifier -4: 6-4, then 1 and 1
		{"an effect that takes everything stops at one per level", withOverlay(t, Overlay{Revision: 1, Races: []TableRace{draining}}), tableRaceOf(human("class:fighter", 3, 14), draining.Key), 3 - (12 + 2*(6+2)), 3},
		{"an effect on top of the floor", srd, hillDwarf("class:wizard", 3, 1), 3, 2 + 1 + 1 + 3}, // CON 3, modifier -4
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := Derive(tt.build, tt.content)
			if d.HitPointsFromEffects != tt.fromEffects {
				t.Errorf("HitPointsFromEffects = %d, want %d", d.HitPointsFromEffects, tt.fromEffects)
			}
			if d.HitPointsMax != tt.hitPointsMax {
				t.Errorf("HitPointsMax = %d, want %d", d.HitPointsMax, tt.hitPointsMax)
			}
		})
	}
}

// A table's hp.max modifier that takes hit points away (or sets the maximum to
// 0) stops at one hit point per level, so a character is never born "down".
func TestHitPointsMaxNeverFallsBelowOnePerLevel(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	derive := func(t *testing.T, e Effect) Derived {
		t.Helper()
		tc := genClass(srd, genKinds[0])
		tc.Levels[0].Features = append(tc.Levels[0].Features, tf("drain", "Dreno", e))
		c, err := srd.With(Overlay{Classes: []TableClass{tc}})
		if err != nil {
			t.Fatalf("With refused the effect: %v", err)
		}
		return Derive(sweepBase(t, c, tc.Key, ""), c)
	}
	for name, e := range map[string]Effect{
		"add -1000": {Type: "modifier", Target: "hp.max", Mode: "add", Value: "-1000"},
		"set 0":     {Type: "modifier", Target: "hp.max", Mode: "set", Value: "0"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := derive(t, e)
			if d.HitPointsMax != d.TotalLevel {
				t.Errorf("HitPointsMax = %d at level %d, want %d", d.HitPointsMax, d.TotalLevel, d.TotalLevel)
			}
		})
	}
	t.Run("control: a bonus still adds", func(t *testing.T) {
		t.Parallel()
		d := derive(t, Effect{Type: "modifier", Target: "hp.max", Mode: "add", Value: "5"})
		if d.HitPointsMax <= d.TotalLevel {
			t.Errorf("HitPointsMax = %d with +5, want more than %d", d.HitPointsMax, d.TotalLevel)
		}
	})
}

// TestPaladinAuraOfProtectionAddsCharismaToEverySave: from level 6 the paladin
// adds the Charisma modifier, at least +1, to every saving throw.
func TestPaladinAuraOfProtectionAddsCharismaToEverySave(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:paladin", 6)
	b.BaseScores[CHA] = 15 // human +1 = 16 -> +3
	d := Derive(b, c)
	for _, a := range AllAbilities() {
		want := abilityOf(d, a).Modifier + 3
		if saveOf(d, a).Proficient {
			want += d.ProficiencyBonus
		}
		if got := saveOf(d, a).Bonus; got != want {
			t.Errorf("paladin 6, Cha 16, %s save = %+d, want %+d", a, got, want)
		}
	}
	// Below level 6 there is no aura, and a low Charisma still adds +1.
	if got, want := saveOf(Derive(standard("class:paladin", 5), c), STR).Bonus, 3; got != want {
		t.Errorf("paladin 5 Str save = %+d, want %+d (no aura yet)", got, want)
	}
	if low, base := saveOf(Derive(standard("class:paladin", 6), c), STR).Bonus, 3; low != base+1 {
		t.Errorf("paladin 6, Cha 9: Str save = %+d, want %+d (the aura adds at least +1)", low, base+1)
	}
}

// TestLevel20FeaturesRaiseScoresUpTo24: Primal Champion adds 4 to Strength and
// Constitution, with 24 as the limit of that increase; a score above 20 from it
// is not reported, and any other score above 20 still is.
func TestLevel20FeaturesRaiseScoresUpTo24(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	hasIssue := func(d Derived) bool {
		return slices.ContainsFunc(d.Issues, func(i Issue) bool { return i.Code == IssueScoreAbove20 })
	}
	b := standard("class:barbarian", 20)
	b.BaseScores[STR], b.BaseScores[CON] = 18, 18 // human +1 = 19
	d := Derive(b, c)
	for _, a := range []Ability{STR, CON} {
		if got := abilityOf(d, a).Score; got != 23 {
			t.Errorf("barbarian 20 %s = %d, want 23 (19 + 4)", a, got)
		}
	}
	if got := abilityOf(d, STR).Modifier; got != 6 {
		t.Errorf("barbarian 20 STR modifier = %d, want +6", got)
	}
	if hasIssue(d) {
		t.Errorf("a score of 23 from Primal Champion is reported as above 20: %v", d.Issues)
	}
	b.BaseScores[STR] = 20 // 21 + 4 stops at 24
	if got := abilityOf(Derive(b, c), STR).Score; got != 24 {
		t.Errorf("barbarian 20 STR 21 + 4 = %d, want the cap 24", got)
	}
	if got := abilityOf(Derive(standard("class:barbarian", 19), c), STR).Score; got != 16 {
		t.Errorf("barbarian 19 STR = %d, want 16 (no Primal Champion yet)", got)
	}

	// The monk's level 20 feature in SRD 5.1 is Perfect Self (ki back on
	// initiative): it raises no score.
	m := Derive(standard("class:monk", 20), c) // DEX 15, WIS 11
	if got, want := abilityOf(m, DEX).Score, 15; got != want {
		t.Errorf("monk 20 DEX = %d, want %d", got, want)
	}
	if got, want := abilityOf(m, WIS).Score, 11; got != want {
		t.Errorf("monk 20 WIS = %d, want %d", got, want)
	}

	f := standard("class:fighter", 19)
	f.BaseScores[STR] = 20 // human +1 = 21, and nothing raises the ceiling
	if !hasIssue(Derive(f, c)) {
		t.Error("a score of 21 without a feature that allows it is no longer reported")
	}
	b.ExtraAbilityBonuses = map[Ability]int{STR: 8} // 21 + 8 = 29, over the cap, left alone
	if got := abilityOf(Derive(b, c), STR).Score; got != 29 {
		t.Errorf("a score already above 24 = %d, want 29 (the feature never lowers it)", got)
	}
}

// landDruid is a human Circle of the Land druid of the level, in the terrain.
func landDruid(level int, terrain string) Build {
	b := standard("class:druid", level)
	b.Classes[0].Subclass = "subclass:land"
	b.FeatureChoices = []string{"feature:circle-of-the-land-" + terrain}
	b.BaseScores[WIS] = 15
	return b
}

// TestLandDruidCircleSpells: the circle spells of the chosen land come at druid
// levels 3, 5, 7 and 9, always prepared, off the druid's own list when the land has
// them there or not, and none of them counts against the spells prepared (SRD 5.1,
// Druid, Circle of the Land, Circle Spells).
func TestLandDruidCircleSpells(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	spellsOf := func(d Derived) map[string]bool {
		out := map[string]bool{}
		for _, s := range d.Spells {
			if s.Prepared {
				out[s.Spell.Key] = true
			}
		}
		return out
	}
	for _, tc := range []struct {
		level int
		have  []string
		not   []string
	}{
		{2, nil, []string{"spell:hold-person", "spell:spike-growth"}},
		{3, []string{"spell:hold-person", "spell:spike-growth"}, []string{"spell:sleet-storm", "spell:slow"}},
		{5, []string{"spell:hold-person", "spell:sleet-storm", "spell:slow"}, []string{"spell:ice-storm"}},
		{9, []string{"spell:ice-storm", "spell:freedom-of-movement", "spell:cone-of-cold", "spell:commune-with-nature"}, nil},
	} {
		d := Derive(landDruid(tc.level, "arctic"), c)
		got := spellsOf(d)
		for _, k := range tc.have {
			if !got[k] {
				t.Errorf("Land druid %d, arctic: %s is not prepared; prepared = %v", tc.level, k, got)
			}
		}
		for _, k := range tc.not {
			if got[k] {
				t.Errorf("Land druid %d, arctic: %s is prepared before its level", tc.level, k)
			}
		}
		if hasIssue(d, IssueSpellCount) {
			t.Errorf("Land druid %d: circle spells counted against the prepared spells: %v", tc.level, d.Issues)
		}
	}
	// Another land gives its own spells, and none of the arctic's.
	desert := spellsOf(Derive(landDruid(3, "desert"), c))
	if !desert["spell:blur"] || !desert["spell:silence"] || desert["spell:hold-person"] {
		t.Errorf("Land druid 3, desert: prepared = %v, want blur and silence only", desert)
	}
	// Without a terrain there are no circle spells.
	b := landDruid(3, "arctic")
	b.FeatureChoices = nil
	if got := spellsOf(Derive(b, c)); got["spell:hold-person"] {
		t.Errorf("Land druid with no terrain has circle spells: %v", got)
	}
	// The circle spells feature is on the sheet from level 3, and the terrain from level 2.
	d := Derive(landDruid(3, "arctic"), c)
	if !hasFeature(d, "feature:circle-of-the-land") || !hasFeature(d, "feature:circle-spells-1") {
		t.Errorf("Land druid 3 features lack the terrain or the circle spells")
	}
}

// TestSkillsTheRaceAndBackgroundGiveAreNoPicks: a skill the race or the background
// already gives is not one of the class's picks, so choosing it again leaves a pick
// unmade (SRD 5.1, Backgrounds: a repeated proficiency is traded for another).
func TestSkillsTheRaceAndBackgroundGiveAreNoPicks(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 1)
	b.Race = "race:half-orc"
	b.Background = "background:acolyte" // Insight and Religion
	// Fighters choose two: Intimidation comes from the half-orc, Insight from the acolyte.
	b.SkillProficiencies = []string{"skill:intimidation", "skill:insight"}
	d := Derive(b, c)
	if !hasIssue(d, IssueSkillCount) {
		t.Fatalf("both picks repeat a given skill: issues = %v, want the two picks missing", d.Issues)
	}
	if len(d.OpenChoices) != 1 || d.OpenChoices[0] != (OpenChoice{Kind: OpenChoiceSkills, Missing: 2}) {
		t.Errorf("open choices = %+v, want 2 skills missing", d.OpenChoices)
	}
	b.SkillProficiencies = []string{"skill:athletics", "skill:perception"}
	d = Derive(b, c)
	if hasIssue(d, IssueSkillCount) || len(d.OpenChoices) != 0 {
		t.Errorf("two new skills: issues = %v, open choices = %+v, want a complete sheet", d.Issues, d.OpenChoices)
	}
	// The catalog tells the editor which skills are given.
	cat := c.Catalog()
	var orc, acolyte, elf []string
	for _, r := range cat.Races {
		switch r.Key {
		case "race:half-orc":
			orc = r.SkillProficiencies
		case "race:elf":
			elf = r.SkillProficiencies
		}
	}
	for _, bg := range cat.Backgrounds {
		if bg.Key == "background:acolyte" {
			acolyte = bg.SkillProficiencies
		}
	}
	if !slices.Equal(orc, []string{"skill:intimidation"}) || !slices.Equal(elf, []string{"skill:perception"}) || !slices.Equal(acolyte, []string{"skill:insight", "skill:religion"}) {
		t.Errorf("given skills: half-orc %v, elf %v, acolyte %v", orc, elf, acolyte)
	}
}

// TestOpenChoicesNameWhatASheetLacks: a caster that picked no cantrip or prepared
// no spell, and a class that picked no skill, has the choices open; a complete
// sheet has none.
func TestOpenChoicesNameWhatASheetLacks(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	w := standard("class:wizard", 6)
	d := Derive(w, c)
	want := map[string]int{OpenChoiceSkills: 2, OpenChoiceCantrips: 4, OpenChoiceSpellsPrepared: 7}
	got := map[string]int{}
	for _, o := range d.OpenChoices {
		got[o.Kind] = o.Missing
	}
	if len(got) != len(want) {
		t.Fatalf("open choices = %+v, want %v", d.OpenChoices, want)
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("open %s = %d, want %d (%+v)", k, got[k], n, d.OpenChoices)
		}
	}
	// A Sorcerer learns a fixed number of spells.
	s := Derive(standard("class:sorcerer", 3), c)
	var known int
	for _, o := range s.OpenChoices {
		if o.Kind == OpenChoiceSpellsKnown {
			known = o.Missing
		}
	}
	if known != 4 {
		t.Errorf("a level 3 sorcerer with no spell has %d spells known open, want 4: %+v", known, s.OpenChoices)
	}
	// Picking them closes them.
	w.SkillProficiencies = []string{"skill:arcana", "skill:history"}
	w.Cantrips = []string{"spell:fire-bolt", "spell:light", "spell:mage-hand", "spell:prestidigitation"}
	w.SpellsKnown = []string{"spell:magic-missile", "spell:shield", "spell:sleep", "spell:burning-hands", "spell:detect-magic", "spell:mage-armor", "spell:fireball", "spell:fly"}
	w.SpellsPrepared = []string{"spell:magic-missile", "spell:shield", "spell:sleep", "spell:burning-hands", "spell:detect-magic", "spell:mage-armor", "spell:fireball", "spell:fly"}
	d = Derive(w, c)
	if len(d.OpenChoices) != 0 {
		t.Errorf("a complete wizard has open choices %+v (issues %v)", d.OpenChoices, d.Issues)
	}
}
