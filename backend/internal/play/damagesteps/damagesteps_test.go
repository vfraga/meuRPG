package damagesteps

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The damage steps: what lands, what is stored, and what each viewer reads.

// A reduction whose source is marked hidden reads "Outra fonte" and names no key to a player, and the source to the master.
func TestAHiddenReductionSourceReadsOutraFonteToAPlayer(t *testing.T) {
	t.Parallel()
	mods := []combat.Modifier{{Kind: combat.StepReduction, Source: "feat:couro-grosso@mesa", Amount: 3, Hidden: true}}
	groups := Settle(map[string]int{"damage-type:slashing": 8}, mods, nil, combat.Origin{Attack: true, Weapon: true}).Groups
	names := func(k string) string { return map[string]string{"feat:couro-grosso@mesa": "Couro grosso"}[k] }
	player := Proto(groups, nil, names, dtPT, false)
	if len(player) != 1 || player[0].GetLabelPt() != "Redução de dano de arma (Outra fonte)" || len(player[0].GetSourceKeys()) != 0 {
		t.Errorf("the player reads %v, want the reduction from Outra fonte and no key", player)
	}
	master := Proto(groups, nil, names, dtPT, true)
	if len(master) != 1 || master[0].GetLabelPt() != "Redução de dano de arma (Couro grosso)" || len(master[0].GetSourceKeys()) != 1 {
		t.Errorf("the master reads %v, want the feat and its key", master)
	}
	if master[0].GetBefore() != 8 || master[0].GetAfter() != 5 {
		t.Errorf("the step goes %d to %d, want 8 to 5", master[0].GetBefore(), master[0].GetAfter())
	}
}

// A weapon hit of two damage types loses the reduction once, not once per type (#572).
func TestAReductionIsTakenOnceFromAHitOfTwoDamageTypes(t *testing.T) {
	t.Parallel()
	cut, fire := "damage-type:slashing", "damage-type:fire"
	mods := []combat.Modifier{{Kind: combat.StepReduction, Source: "feat:couro-grosso@mesa", Amount: 3}}
	from := combat.Origin{Attack: true, Weapon: true}
	land := Settle(map[string]int{cut: 8, fire: 4}, mods, nil, from)
	if land.Amount != 9 {
		t.Errorf("the Landing is %d, want 9: 12 minus 3 once", land.Amount)
	}
	steps := Proto(land.Groups, nil, func(string) string { return "Couro grosso" }, dtPT, true)
	if len(steps) != 1 || steps[0].GetBefore() != 12 || steps[0].GetAfter() != 9 || steps[0].GetDamageTypeKey() != "" {
		t.Errorf("steps = %v, want one reduction of the whole attack, 12 to 9, with no damage type", steps)
	}
	if got := Reapply(12, land.Groups, nil); got != 9 {
		t.Errorf("Reapply = %d, want 9", got)
	}
	if got := Reapply(12, land.Groups, []string{"feat:couro-grosso@mesa"}); got != 12 {
		t.Errorf("Reapply without the source = %d, want 12", got)
	}
	struck := Proto(land.Groups, []string{"feat:couro-grosso@mesa"}, func(string) string { return "Couro grosso" }, dtPT, true)
	if len(struck) != 1 || !struck[0].GetIgnored() {
		t.Errorf("the struck steps = %v, want one, ignored", struck)
	}
	// The master edits a part: the damage is worked out again from the stored groups.
	again := Settle(map[string]int{cut: 8, fire: 2}, Mods(land.Groups), nil, Origin(land.Groups))
	if again.Amount != 7 {
		t.Errorf("the Landing after an edit is %d, want 7: 10 minus 3 once", again.Amount)
	}
}

// The reduction spills to the next type, and resistance then works on what is left of each type.
func TestAReductionLargerThanTheBiggestTypeSpillsAndThenResistanceApplies(t *testing.T) {
	t.Parallel()
	cut, fire := "damage-type:slashing", "damage-type:fire"
	mods := []combat.Modifier{
		{Kind: combat.StepReduction, Source: "feat:couro-grosso@mesa", Amount: 5},
		{Kind: combat.StepResistance, Source: "feat:sangue-frio@mesa", DamageTypes: []string{fire}},
	}
	land := Settle(map[string]int{cut: 3, fire: 6}, mods, nil, combat.Origin{Attack: true, Weapon: true})
	// fire is the largest: 6 - 5 = 1, halved to 0; slashing stays 3.
	if land.Amount != 3 {
		t.Errorf("the Landing is %d, want 3", land.Amount)
	}
	spill := Settle(map[string]int{cut: 3, fire: 1}, mods[:1], nil, combat.Origin{Attack: true, Weapon: true})
	if spill.Amount != 0 {
		t.Errorf("the Landing is %d, want 0: 4 minus 5 stops at 0", spill.Amount)
	}
}

// The same blow from a silvered weapon is not reduced; a damage with no attack is not
// either; the adamantine entry follows the adamantine weapon.
func TestSettleStepsMaterialOrigin(t *testing.T) {
	t.Parallel()
	slash := "damage-type:slashing"
	silver := []combat.Modifier{{Kind: combat.StepResistance, Source: "monster:wight", DamageTypes: []string{slash}, Qualifier: combat.QualifierNonmagicalUnlessSilvered}}
	adamant := []combat.Modifier{{Kind: combat.StepResistance, Source: "monster:gargoyle", DamageTypes: []string{slash}, Qualifier: combat.QualifierNonmagicalUnlessAdamantine}}
	tests := []struct {
		name string
		mods []combat.Modifier
		from combat.Origin
		want int32
	}{
		{"unsilvered", silver, combat.Origin{Attack: true}, 4},
		{"silvered", silver, combat.Origin{Attack: true, Silvered: true}, 9},
		{"silver entry, adamantine weapon", silver, combat.Origin{Attack: true, Adamantine: true}, 4},
		{"no attack", silver, combat.Origin{}, 9},
		{"non-adamantine", adamant, combat.Origin{Attack: true, Silvered: true}, 4},
		{"adamantine", adamant, combat.Origin{Attack: true, Adamantine: true}, 9},
	}
	for _, tt := range tests {
		if got := Settle(map[string]int{slash: 9}, tt.mods, nil, tt.from).Amount; got != tt.want {
			t.Errorf("%s: amount = %d, want %d", tt.name, got, tt.want)
		}
	}
	names := func(s string) string { return s }
	pt := func(s string) string { return s }
	for q, want := range map[string]string{
		combat.QualifierNonmagicalUnlessSilvered:   "Resistência a x de ataque não mágico nem prateado",
		combat.QualifierNonmagicalUnlessAdamantine: "Resistência a x de ataque não mágico nem adamantino",
	} {
		if got := stepLabel(combat.Step{Kind: combat.StepResistance, DamageType: "x", Qualifier: q}, names, pt); got != want {
			t.Errorf("label = %q, want %q", got, want)
		}
	}
}

// The master's "ignore_modifiers" leaves a source out of the stored steps (Reapply): the wight's silvered-qualified
// resistance on a plain hit is the whole 9 again; another source's key, or an unmarked silvered hit, change nothing
// to what the groups say. (ApplyPendingDamage settles only a player's character, which a stat block never covers, so
// this is the function the call reads.)
func TestIgnoringTheSilveredQualifiedSourceLandsAPlainHitInFull(t *testing.T) {
	t.Parallel()
	const piercing = "damage-type:piercing"
	mods := []combat.Modifier{{Kind: combat.StepResistance, Source: "monster:wight", DamageTypes: []string{piercing}, Qualifier: combat.QualifierNonmagicalUnlessSilvered}}
	tests := []struct {
		name    string
		from    combat.Origin
		ignored []string
		want    int32
	}{
		{"a plain hit, nothing ignored", combat.Origin{Attack: true}, nil, 4},
		{"a plain hit, the wight ignored", combat.Origin{Attack: true}, []string{"monster:wight"}, 9},
		{"a plain hit, another source ignored", combat.Origin{Attack: true}, []string{"feature:rage"}, 4},
		{"a silvered hit", combat.Origin{Attack: true, Silvered: true}, nil, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			land := Settle(map[string]int{piercing: 9}, mods, nil, tt.from)
			if got := Reapply(9, land.Groups, tt.ignored); got != tt.want {
				t.Errorf("Reapply = %d, want %d", got, tt.want)
			}
		})
	}
}

// Steps stored before the material qualifiers existed (an origin with no silvered or adamantine key, a modifier with
// the nonmagical qualifier) still read and work out the same: the removal of a part works the damage out again from
// them with Origin, Mods and Settle, and an ignored source works out through Reapply.
func TestStepsStoredBeforeTheMaterialQualifiersWorkOutTheSame(t *testing.T) {
	t.Parallel()
	const stored = `[{"damage_type":"damage-type:piercing","before":9,` +
		`"mods":[{"Kind":"resistance","Source":"monster:grick","DamageTypes":["damage-type:piercing"],"Qualifier":"nonmagical"}],` +
		`"origin":{"attack":true,"magical":false}}]`
	groups := Read([]byte(stored))
	want := []Group{{
		DamageType: "damage-type:piercing", Before: 9, From: combat.Origin{Attack: true},
		Mods: []combat.Modifier{{Kind: combat.StepResistance, Source: "monster:grick", DamageTypes: []string{"damage-type:piercing"}, Qualifier: combat.QualifierNonmagical}},
	}}
	gotJSON, _ := json.Marshal(groups)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("the stored steps read as %s, want %s", gotJSON, wantJSON)
	}
	from := Origin(groups)
	if from != (combat.Origin{Attack: true}) {
		t.Fatalf("origin = %+v, want an unmarked attack", from)
	}
	// The removal of a part: the counted damage (9 piercing, as when it was stored) is halved through the stored modifiers.
	land := Settle(map[string]int{"damage-type:piercing": 9}, Mods(groups), nil, from)
	if land.Amount != 4 || !land.Changed || len(land.Groups) != 1 || land.Groups[0].From != from {
		t.Errorf("Landing = %+v, want 4, changed, one group with the same origin", land)
	}
	if got := Reapply(9, groups, nil); got != 4 {
		t.Errorf("Reapply = %d, want 4", got)
	}
	if got := Reapply(9, groups, []string{"monster:grick"}); got != 9 {
		t.Errorf("Reapply ignoring the source = %d, want 9", got)
	}
}

// dtPT writes the damage types the tests use in Portuguese, as play's table does.
func dtPT(key string) string {
	return map[string]string{
		"damage-type:slashing": "cortante",
		"damage-type:piercing": "perfurante",
		"damage-type:fire":     "fogo",
	}[key]
}

// The stored form of a group is what pending damages already hold: its keys do not change.
func TestAGroupIsStoredUnderTheSameKeys(t *testing.T) {
	t.Parallel()
	g := Group{
		DamageType: "damage-type:fire", Before: 7, Mods: []combat.Modifier{{Kind: combat.StepResistance, Source: "trait:hellish-resistance"}},
		From: combat.Origin{Attack: true}, Allot: map[string]int{"damage-type:fire": 7},
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"damage_type", "before", "mods", "origin", "allot"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("the stored group %s has no %q key", raw, k)
		}
	}
	if len(keys) != 5 {
		t.Errorf("the stored group has %d keys, want 5: %s", len(keys), raw)
	}
}
