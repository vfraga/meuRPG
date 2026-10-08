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
