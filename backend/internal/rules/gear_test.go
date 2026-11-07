package rules

import "testing"

// The gear list names every piece the sheet lists, carried ones too, and says what flipping each does to the armor class.
func TestGearListsCarriedPiecesWithTheirArmorClassIfToggled(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 5)
	b.Armor, b.Shield = "equipment:chain-mail", true
	b.Weapons = []string{"equipment:longsword", "equipment:longbow"}
	b.Carried = []string{"equipment:longbow"}
	d := Derive(b, c)
	d.Gear = DeriveGear(b, c, d)
	byKey := map[string]GearPiece{}
	for _, g := range d.Gear {
		byKey[g.Key] = g
	}
	if len(d.Gear) != 4 || d.Gear[0].Key != "equipment:chain-mail" || d.Gear[1].Key != ShieldKey {
		t.Fatalf("gear = %+v, want armor, shield, then the two weapons", d.Gear)
	}
	bow := byKey["equipment:longbow"]
	if bow.Equipped || !bow.TwoHanded || bow.NamePT != "Arco longo" {
		t.Errorf("longbow = %+v, want carried, two-handed, named Arco longo", bow)
	}
	if byKey["equipment:chain-mail"].NamePT != "Cota de malha" || !byKey["equipment:chain-mail"].Equipped {
		t.Errorf("chain mail = %+v", byKey["equipment:chain-mail"])
	}
	// Equipped: chain mail 16 + shield 2. Stowing the shield or the armor lowers it; drawing the bow leaves the shield idle.
	if d.ArmorClass != 18 {
		t.Fatalf("AC = %d, want 18", d.ArmorClass)
	}
	if got := byKey[ShieldKey].ArmorClassIfToggled; got != 16 {
		t.Errorf("AC with the shield stowed = %d, want 16", got)
	}
	if got := bow.ArmorClassIfToggled; got != 16 {
		t.Errorf("AC with the bow drawn = %d, want 16 (the shield goes idle)", got)
	}
	if got := byKey["equipment:longsword"].ArmorClassIfToggled; got != 0 {
		t.Errorf("a one-handed weapon changes no AC, got %d", got)
	}
}

// Derive is on every hot path (the lists, the combatants, the vitals): it never builds the gear, which derives the sheet again
// for each armor, shield and two-handed weapon. Only DeriveGear does.
func TestDeriveLeavesTheGearToDeriveGear(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 5)
	b.Armor, b.Shield = "equipment:chain-mail", true
	b.Weapons = []string{"equipment:longbow"}
	d := Derive(b, c)
	if d.Gear != nil {
		t.Fatalf("Derive built the gear: %+v", d.Gear)
	}
	if got := DeriveGear(b, c, d); len(got) != 3 {
		t.Fatalf("DeriveGear = %+v, want armor, shield and the bow", got)
	}
}

// Two weapons with one key are one row with a count: the state is kept by key, so the copies move together.
func TestGearMergesWeaponsWithTheSameKeyIntoOneRowWithACount(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	b := standard("class:fighter", 5)
	b.Weapons = []string{"equipment:dagger", "equipment:longsword", "equipment:dagger"}
	got := DeriveGear(b, c, Derive(b, c))
	if len(got) != 2 || got[0].Key != "equipment:dagger" || got[0].Count != 2 || got[1].Count != 1 {
		t.Fatalf("gear = %+v, want a dagger row with count 2 and a longsword row", got)
	}
}
