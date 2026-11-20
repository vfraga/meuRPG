package rules

import "testing"

func multiclass(first string, firstLevel int, second string, secondLevel int) Build {
	b := standard(first, firstLevel)
	b.Classes = append(b.Classes, ClassLevel{Class: second, Level: secondLevel})
	return b
}

// What reading the spell of a scroll comes to for each kind of reader: on the list, off it, above what they cast (a warlock's
// pact slots count), and no spellcasting at all.
func TestScrollSpellReading(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	tests := []struct {
		name  string
		build Build
		spell string
		want  ScrollRead
	}{
		{
			"a wizard reads a spell of a level they cast", standard("class:wizard", 5), "spell:fireball",
			ScrollRead{Level: 3, Reads: true, Economy: EconomyAction},
		},
		{
			"a wizard reads a cantrip without a check", standard("class:wizard", 1), "spell:fire-bolt",
			ScrollRead{Level: 0, Reads: true, Economy: EconomyAction},
		},
		{
			"a wizard asks a check for a spell above what they cast", standard("class:wizard", 1), "spell:fireball",
			ScrollRead{Level: 3, Reads: true, CheckNeeded: true, CheckDC: 13, Ability: INT, Economy: EconomyAction},
		},
		{
			"a wizard of level 3 casts the 2nd level and asks a check for the 3rd", standard("class:wizard", 3), "spell:fly",
			ScrollRead{Level: 3, Reads: true, CheckNeeded: true, CheckDC: 13, Ability: INT, Economy: EconomyAction},
		},
		{
			"a wizard of level 3 casts the 2nd level", standard("class:wizard", 3), "spell:web",
			ScrollRead{Level: 2, Reads: true, Economy: EconomyAction},
		},
		{
			"a wizard off the list", standard("class:wizard", 5), "spell:cure-wounds",
			ScrollRead{Level: 1, UnreadablePT: "Não é da lista de Mago."},
		},
		{
			"a cleric asks a check with wisdom", standard("class:cleric", 1), "spell:revivify",
			ScrollRead{Level: 3, Reads: true, CheckNeeded: true, CheckDC: 13, Ability: WIS, Economy: EconomyAction},
		},
		{
			"a warlock casts the level of the pact slots", standard("class:warlock", 3), "spell:hold-person",
			ScrollRead{Level: 2, Reads: true, Economy: EconomyAction},
		},
		{
			"a warlock asks a check above the pact slots", standard("class:warlock", 3), "spell:fear",
			ScrollRead{Level: 3, Reads: true, CheckNeeded: true, CheckDC: 13, Ability: CHA, Economy: EconomyAction},
		},
		{
			"a fighter has no spellcasting", standard("class:fighter", 5), "spell:magic-missile",
			ScrollRead{Level: 1, UnreadablePT: "Não conjura magias."},
		},
		{
			"a paladin of level 1 has no spellcasting yet", standard("class:paladin", 1), "spell:bless",
			ScrollRead{Level: 1, UnreadablePT: "Não conjura magias."},
		},
		{
			"two casting classes name both lists", multiclass("class:wizard", 3, "class:cleric", 2), "spell:eldritch-blast",
			ScrollRead{Level: 0, UnreadablePT: "Não é da lista de Mago nem de Clérigo."},
		},
		{
			"the slots of every class count, so a multiclass casts what the table gives", multiclass("class:wizard", 3, "class:cleric", 2), "spell:fireball",
			ScrollRead{Level: 3, Reads: true, Economy: EconomyAction},
		},
		{"a reaction", standard("class:wizard", 5), "spell:shield", ScrollRead{Level: 1, Reads: true, Economy: EconomyReaction}},
		{"a bonus action", standard("class:wizard", 5), "spell:misty-step", ScrollRead{Level: 2, Reads: true, Economy: EconomyBonusAction}},
		{"a casting of a minute costs none of the turn", standard("class:wizard", 5), "spell:alarm", ScrollRead{Level: 1, Reads: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := c.ReadScrollSpell(Derive(tt.build, c), tt.spell)
			if !ok || got != tt.want {
				t.Errorf("ReadScrollSpell(%s) = %+v, %v; want %+v", tt.spell, got, ok, tt.want)
			}
		})
	}
}

func TestScrollSpellReadingRefusesAKeyThatIsNoSpell(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, key := range []string{"", "spell:nope", "item:bag-of-holding"} {
		if got, ok := c.ReadScrollSpell(Derive(standard("class:wizard", 5), c), key); ok || got != (ScrollRead{}) {
			t.Errorf("%q: %+v %v", key, got, ok)
		}
	}
}

// The part of the turn a casting time spends: an action, a bonus action and a reaction spend theirs, and every longer or unparsed
// casting spends none.
func TestCastingTimeEconomy(t *testing.T) {
	t.Parallel()
	for unit, want := range map[string]string{
		CastAction: EconomyAction, CastBonusAction: EconomyBonusAction, CastReaction: EconomyReaction,
		CastMinute: "", CastHour: "", "": "", "fortnight": "",
	} {
		if got := (CastingTime{Amount: 1, Unit: unit}).Economy(); got != want {
			t.Errorf("unit %q: economy %q, want %q", unit, got, want)
		}
	}
}
