package consumable

import (
	"errors"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

func TestTotalAddsTheFacesAndTheBonus(t *testing.T) {
	t.Parallel()
	f := rules.DiceFormula{Count: 2, Sides: 4, Bonus: 2}
	for name, tc := range map[string]struct {
		faces []int32
		want  int32
		err   bool
	}{
		"low":         {[]int32{1, 1}, 4, false},
		"high":        {[]int32{4, 4}, 10, false},
		"too few":     {[]int32{3}, 0, true},
		"too many":    {[]int32{1, 1, 1}, 0, true},
		"a zero":      {[]int32{0, 2}, 0, true},
		"off the die": {[]int32{5, 1}, 0, true},
	} {
		got, err := Total(f, tc.faces)
		if (err != nil) != tc.err || got != tc.want {
			t.Errorf("%s: Total(%v) = %d, %v; want %d, error %v", name, tc.faces, got, err, tc.want, tc.err)
		}
		if tc.err && !errors.Is(err, ErrFaces) {
			t.Errorf("%s: error %v is not ErrFaces", name, err)
		}
	}
}

func TestResistanceIsOneHourOfResistanceToTheType(t *testing.T) {
	t.Parallel()
	p := rules.PotionEffect{Kind: rules.PotionResistance, DamageType: "damage-type:fire", Rounds: rules.PotionResistanceRounds}
	m := Resistance(p)
	if m.Kind != rules.ModifierResistance || m.ResistanceKind != rules.ResistanceKindResist || len(m.DamageTypes) != 1 || m.DamageTypes[0] != "damage-type:fire" {
		t.Errorf("Resistance() = %+v", m)
	}
	if got := Seconds(p); got != 3600 {
		t.Errorf("Seconds() = %d, want 3600", got)
	}
}

func TestTextPrefersThePortugueseOne(t *testing.T) {
	t.Parallel()
	if got := Text(rules.MagicItem{Desc: []string{"en"}, DescPT: []string{"pt"}}); len(got) != 1 || got[0] != "pt" {
		t.Errorf("Text() = %v, want pt", got)
	}
	if got := Text(rules.MagicItem{Desc: []string{"en"}}); len(got) != 1 || got[0] != "en" {
		t.Errorf("Text() = %v, want en", got)
	}
}

// The line of a use in the log names the item, the outcome and the receiver to the table; the dice, the roll and the hit
// points regained are told only when the reader may read the numbers.
func TestLineTellsTheNumbersOnlyToWhoMayRead(t *testing.T) {
	t.Parallel()
	use := Used{
		Item: "item:potion-of-healing-common", Name: "Poção de Cura", Outcome: OutcomeHealed,
		Dice: "2d4+2", Faces: []int32{3, 4}, Total: 9, Healed: 7, DamageType: "damage-type:fire",
	}
	table := Line(use, charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION, "Fogo", "por 1 hora", "Pensantus", false)
	if table.GetItemNamePt() != "Poção de Cura" || table.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_HEALED ||
		table.GetTargetLabel() != "Pensantus" || table.GetDamageTypeKey() != "damage-type:fire" || table.GetDamageTypeNamePt() != "Fogo" {
		t.Errorf("the table's line = %v, want the item, the outcome, the receiver and the damage type", table)
	}
	if table.GetKind() != charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION || table.GetEffectDurationTextPt() != "" {
		t.Errorf("the table's line of a heal reads kind %v and length %q, want the potion and no length", table.GetKind(), table.GetEffectDurationTextPt())
	}
	if table.GetDiceText() != "" || len(table.GetFaces()) != 0 || table.GetRollTotal() != 0 || table.GetHealed() != 0 {
		t.Errorf("the table's line holds numbers: %v", table)
	}
	owner := Line(use, charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION, "Fogo", "por 1 hora", "Pensantus", true)
	if owner.GetDiceText() != "2d4+2" || len(owner.GetFaces()) != 2 || owner.GetRollTotal() != 9 || owner.GetHealed() != 7 {
		t.Errorf("the owner's line = %v, want the dice, the faces, the total and the hit points regained", owner)
	}
}

// Standing is the word the order of the combat gives a player: a death the master confirmed is on no list, three failed death
// saves keep a player's character listed and leave it out of the master's list, and 0 hit points are down.
func TestStandingFollowsTheStateWordOfThePlayer(t *testing.T) {
	t.Parallel()
	vitals := func(hp int32) *playv1.CharacterVitals {
		return &playv1.CharacterVitals{HitPointsMax: 10, HitPointsCurrent: hp}
	}
	for name, tc := range map[string]struct {
		gone             bool
		failures         int32
		master           bool
		hp               int32
		listed, wantDown bool
	}{
		"on its feet":                                           {hp: 5, listed: true},
		"on its feet, for the master":                           {hp: 5, master: true, listed: true},
		"at 0 hit points":                                       {hp: 0, listed: true, wantDown: true},
		"two failed saves":                                      {hp: 0, failures: 2, listed: true, wantDown: true},
		"three failed saves, for a player":                      {hp: 0, failures: 3, listed: true, wantDown: true},
		"three failed saves, for the master":                    {hp: 0, failures: 3, master: true},
		"a death the master confirmed":                          {hp: 0, failures: 3, gone: true},
		"a death confirmed, for the master":                     {hp: 0, gone: true, master: true},
		"three failed saves above 0 hit points, for a player":   {hp: 5, failures: 3, listed: true},
		"three failed saves above 0 hit points, for the master": {hp: 5, failures: 3, master: true},
		"no confirmed death, at 0 hit points":                   {hp: 0, listed: true, wantDown: true},
		"a character with no maximum is up":                     {listed: true},
	} {
		v := vitals(tc.hp)
		if name == "a character with no maximum is up" {
			v = &playv1.CharacterVitals{}
		}
		if listed, down := Standing(tc.gone, tc.failures, tc.master, v); listed != tc.listed || down != tc.wantDown {
			t.Errorf("%s: Standing = %v, %v; want %v, %v", name, listed, down, tc.listed, tc.wantDown)
		}
	}
}

// A resistance line carries the length of the effect for the whole table, and no other outcome does.
func TestLineCarriesTheLengthOfAResistanceOnly(t *testing.T) {
	t.Parallel()
	resist := Line(Used{Name: "Poção", Outcome: OutcomeResistance, DamageType: "damage-type:fire"}, charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION, "Fogo", "por 1 hora", "", false)
	if resist.GetEffectDurationTextPt() != "por 1 hora" {
		t.Errorf("a resistance line reads the length %q, want %q", resist.GetEffectDurationTextPt(), "por 1 hora")
	}
	if text := Line(Used{Name: "Óleo", Outcome: OutcomeTextOnly}, charactersv1.ConsumableKind_CONSUMABLE_KIND_OIL, "", "por 1 hora", "", false); text.GetEffectDurationTextPt() != "" || text.GetKind() != charactersv1.ConsumableKind_CONSUMABLE_KIND_OIL {
		t.Errorf("a text-only line reads %v, want the oil and no length", text)
	}
}
