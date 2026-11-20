package combat

import (
	"slices"
	"testing"
)

func TestResolvePool(t *testing.T) {
	t.Parallel()
	type want struct {
		index    int
		affected bool
		reason   string
		left     int
	}
	tests := []struct {
		name      string
		total     int
		creatures []HPCreature
		want      []want
	}{
		{
			// 5d8 = 15; Goblin 1 (7) sleeps, 8 are left; the Capitão (27) does not fit.
			name: "Sono in the canonical fight", total: 15,
			creatures: []HPCreature{{HP: 27}, {HP: 7}},
			want:      []want{{1, true, PoolAffected, 8}, {0, false, PoolTooHigh, 8}},
		},
		{
			name: "ascending order, each one subtracts", total: 20,
			creatures: []HPCreature{{HP: 9}, {HP: 4}, {HP: 7}},
			want:      []want{{1, true, "", 16}, {2, true, "", 9}, {0, true, "", 0}},
		},
		{
			name: "exactly what is left still fits", total: 7,
			creatures: []HPCreature{{HP: 7}},
			want:      []want{{0, true, "", 0}},
		},
		{
			name: "one more than what is left does not", total: 6,
			creatures: []HPCreature{{HP: 7}},
			want:      []want{{0, false, PoolTooHigh, 6}},
		},
		{
			name: "ties keep the target order", total: 8,
			creatures: []HPCreature{{HP: 5}, {HP: 5}, {HP: 5}},
			want:      []want{{0, true, "", 3}, {1, false, PoolTooHigh, 3}, {2, false, PoolTooHigh, 3}},
		},
		{
			name: "the unconscious and the ones at 0 are skipped and spend nothing", total: 10,
			creatures: []HPCreature{{HP: 3, Unconscious: true}, {HP: 0}, {HP: 6}},
			want:      []want{{1, false, PoolSkipped, 10}, {0, false, PoolSkipped, 10}, {2, true, "", 4}},
		},
		{
			name: "nobody in the area", total: 15,
			creatures: nil, want: []want{},
		},
		{
			name: "a pool of 0 affects nobody", total: 0,
			creatures: []HPCreature{{HP: 1}},
			want:      []want{{0, false, PoolTooHigh, 0}},
		},
		{
			// Upcast: Sono at the 3rd circle rolls 9d8; the same arithmetic.
			name: "a bigger pool reaches more", total: 40,
			creatures: []HPCreature{{HP: 27}, {HP: 7}, {HP: 7}},
			want:      []want{{1, true, "", 33}, {2, true, "", 26}, {0, false, PoolTooHigh, 26}},
		},
	}
	for _, tt := range tests {
		got := ResolvePool(tt.total, tt.creatures)
		if len(got) != len(tt.want) {
			t.Errorf("%s: %d steps, want %d", tt.name, len(got), len(tt.want))
			continue
		}
		for i, w := range tt.want {
			g := got[i]
			if g.Index != w.index || g.Affected != w.affected || g.Reason != w.reason || g.Left != w.left || g.HP != tt.creatures[w.index].HP {
				t.Errorf("%s: step %d = %+v, want %+v", tt.name, i, g, w)
			}
		}
	}
}

func TestResolveThreshold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		threshold, hp int
		want          bool
	}{
		{100, 27, true},
		{100, 100, true}, // exactly 100 is still affected (Matar)
		{100, 101, false},
		{150, 150, true}, // Atordoar
		{150, 151, false},
		{100, 0, true},
	}
	for _, tt := range tests {
		if got := ResolveThreshold(tt.threshold, tt.hp); got != tt.want {
			t.Errorf("ResolveThreshold(%d, %d) = %v, want %v", tt.threshold, tt.hp, got, tt.want)
		}
	}
}

func TestResolveZeroHP(t *testing.T) {
	t.Parallel()
	if !ResolveZeroHP(0) || ResolveZeroHP(1) || ResolveZeroHP(27) {
		t.Error("Estabilizar works only at exactly 0 hit points")
	}
}

func TestResolveFlatHeal(t *testing.T) {
	t.Parallel()
	ends := []string{"condition:blinded", "condition:deafened"}
	got := ResolveFlatHeal(70, 12, 40, []string{"condition:blinded", "condition:poisoned", "condition:deafened"}, ends)
	if got.HP != 40 || got.Healed != 28 || !slices.Equal(got.Conditions, []string{"condition:poisoned"}) || !slices.Equal(got.Ended, ends) {
		t.Errorf("heal capped at the maximum = %+v", got)
	}
	got = ResolveFlatHeal(70, 0, 200, nil, ends)
	if got.HP != 70 || got.Healed != 70 || got.Conditions == nil || len(got.Conditions) != 0 || len(got.Ended) != 0 {
		t.Errorf("heal from 0 with no conditions = %+v", got)
	}
}

// A creature with a Skip reason is passed with nothing spent (Sleep: undead, charm-immune, Fey Ancestry; Color Spray: cannot see), and
// the reason comes back on its step.
func TestResolvePoolSkipsWhatTheSpellCannotTouchAndSpendsNothingOnIt(t *testing.T) {
	t.Parallel()
	creatures := []HPCreature{{HP: 5}, {HP: 9, Skip: PoolSkipUndead}, {HP: 12, Skip: PoolSkipFeyAncestry}, {HP: 14}}
	steps := ResolvePool(20, creatures)
	want := []struct {
		index    int
		affected bool
		reason   string
		left     int
	}{{0, true, PoolAffected, 15}, {1, false, PoolSkipUndead, 15}, {2, false, PoolSkipFeyAncestry, 15}, {3, true, PoolAffected, 1}}
	if len(steps) != len(want) {
		t.Fatalf("steps = %v, want %d", steps, len(want))
	}
	for i, w := range want {
		if s := steps[i]; s.Index != w.index || s.Affected != w.affected || s.Reason != w.reason || s.Left != w.left {
			t.Errorf("step %d = %+v, want index %d affected %v reason %q left %d", i, s, w.index, w.affected, w.reason, w.left)
		}
	}
}

func TestPoolSkip(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		spell string
		facts PoolFacts
		want  string
	}{
		{"sleep on an undead", SpellSleep, PoolFacts{CreatureType: "undead"}, PoolSkipUndead},
		{"sleep on a creature immune to being charmed", SpellSleep, PoolFacts{CharmImmune: true}, PoolSkipCharmImmune},
		{"sleep on an elf", SpellSleep, PoolFacts{FeyAncestry: true}, PoolSkipFeyAncestry},
		{"sleep on an undead that is also immune to being charmed says undead", SpellSleep, PoolFacts{CreatureType: "undead", CharmImmune: true}, PoolSkipUndead},
		{"sleep on a construct that cannot see", SpellSleep, PoolFacts{CreatureType: "construct", CannotSee: true}, ""},
		{"sleep on a creature with no type", SpellSleep, PoolFacts{}, ""},
		{"color spray on a creature that cannot see", SpellColorSpray, PoolFacts{CannotSee: true}, PoolSkipCannotSee},
		{"color spray on an undead", SpellColorSpray, PoolFacts{CreatureType: "undead", CharmImmune: true, FeyAncestry: true}, ""},
		{"another spell", "spell:fireball", PoolFacts{CreatureType: "undead", CannotSee: true}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := PoolSkip(tc.spell, tc.facts); got != tc.want {
				t.Errorf("PoolSkip(%s, %+v) = %q, want %q", tc.spell, tc.facts, got, tc.want)
			}
		})
	}
}
