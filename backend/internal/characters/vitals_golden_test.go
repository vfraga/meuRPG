package characters

import (
	"bytes"
	"encoding/json"
	"errors"
	goflag "flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// These tests freeze, case by case, what applyVitalsChange and planRest do to a
// character's vitals: every refusal (its field and its message, in the order the checks
// run) and every accepted change, written to golden files. No database: the vitals come
// from vitalsToProto on hand-made rows and maximums.

var updateVitalsGolden = goflag.Bool("update-vitals-golden", false, "write the golden files of applyVitalsChange and planRest again")

// vitalsGoldenJSON is a message as indented JSON with sorted keys, stable from one run to
// the next (protojson alone varies its spacing on purpose).
func vitalsGoldenJSON(t *testing.T, m proto.Message) any {
	t.Helper()
	raw, err := protojson.Marshal(m)
	if err != nil {
		t.Fatalf("protojson.Marshal() error = %v", err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return out
}

// checkVitalsGolden compares got with testdata/<name>, or writes it with -update-vitals-golden.
func checkVitalsGolden(t *testing.T, name string, got any) {
	t.Helper()
	body, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	body = append(body, '\n')
	path := filepath.Join("testdata", name)
	if *updateVitalsGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the golden file (run with -update-vitals-golden to write it): %v", err)
	}
	if !bytes.Equal(body, want) {
		t.Errorf("%s differs from the golden file; diff:\n%s", path, vitalsGoldenDiff(string(want), string(body)))
	}
}

// vitalsGoldenDiff is the first lines that differ, enough to see what moved.
func vitalsGoldenDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var b strings.Builder
	shown := 0
	for i := 0; i < max(len(w), len(g)) && shown < 20; i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			fmt.Fprintf(&b, "line %d:\n  want %s\n  got  %s\n", i+1, wl, gl)
			shown++
		}
	}
	return b.String()
}

// vitalsGoldenRich are the maximums of a character with a bit of everything: 30 hit
// points, slots of levels 1 to 3, two pact slots of level 2, 2d10 and 3d6, a resource of
// each recharge and a wolf's shape (11 hit points).
func vitalsGoldenRich() vitalsMax {
	m := vitalsMax{
		hitPoints: 30, pactLevel: 2, pactSlots: 2,
		hitDice:      []rules.HitDice{{Die: 10, Count: 2}, {Die: 6, Count: 3}},
		hitDiceTotal: 5, wizardLevel: 3, conMod: 2,
		resources: []rules.Resource{
			{Key: "arcane_recovery", NamePT: "Recuperação Arcana", Max: 1, Recharge: rules.RechargeLongRest},
			{Key: "second_wind", NamePT: "Retomar o Fôlego", Max: 1, Recharge: rules.RechargeShortRest},
			{Key: "channel_divinity", NamePT: "Canalizar Divindade", Max: 2, Recharge: rules.RechargeShortRest},
		},
		beast: "monster:wolf", beastNamePT: "Lobo", beastMax: 11,
	}
	m.slots[0], m.slots[1], m.slots[2] = 4, 3, 2
	return m
}

// vitalsGoldenRichRow is a wounded character of vitalsGoldenRich: 12 hit points, 3
// temporary, slots and resources spent, a created slot of level 1, a d10 spent and the
// wolf at 8.
func vitalsGoldenRichRow() vitalsRow {
	return vitalsRow{
		ID: "c-rich", Name: "Rica", PlayerUserID: new("u-rich"),
		HitPointsCurrent: new(int32(12)), HitPointsTemporary: new(int32(3)),
		SpellSlotsUsed: []int32{2, 1, 0}, SpellSlotsCreated: []int32{1, 0, 0},
		PactSlotsUsed: new(int32(1)), HitDiceUsedByDie: []byte(`{"10":1}`),
		ResourcesUsed:  []byte(`{"second_wind":1,"arcane_recovery":1}`),
		WildShapeBeast: new("monster:wolf"), WildShapeHp: new(int32(8)),
		Revision: new(int32(4)),
	}
}

// vitalsGoldenPlain is wizard3 with nothing spent: no pact slots, no resources, no beast.
func vitalsGoldenPlain() *playv1.CharacterVitals {
	return vitalsToProto(vitalsRow{ID: "c-plain", Name: "Simples"}, wizard3())
}

func vitalsGoldenRichView() *playv1.CharacterVitals {
	return vitalsToProto(vitalsGoldenRichRow(), vitalsGoldenRich())
}

// TestApplyVitalsChangeGolden freezes applyVitalsChange: each case is the vitals it
// starts from, the request and either the refusal (field and message) or the vitals
// after.
func TestApplyVitalsChangeGolden(t *testing.T) {
	t.Parallel()
	n := func(v int32) *int32 { return &v }
	slots := func(pairs ...int32) []*playv1.SpellSlotsUsed {
		var out []*playv1.SpellSlotsUsed
		for i := 0; i+1 < len(pairs); i += 2 {
			out = append(out, &playv1.SpellSlotsUsed{Level: pairs[i], Used: pairs[i+1]})
		}
		return out
	}
	res := func(key string, used int32) *playv1.ResourceUsed { return &playv1.ResourceUsed{Key: key, Used: used} }
	type req = playv1.AdjustCharacterVitalsRequest
	cases := []struct {
		name string
		rich bool
		req  *req
	}{
		{"nothing to change", true, &req{}},
		{"nothing to change on a plain character", false, &req{}},
		{"empty lists are nothing", true, &req{SpellSlotsUsed: []*playv1.SpellSlotsUsed{}, HitDiceUsedByDie: map[int32]int32{}}},
		// The beast's hit points.
		{"wild shape without a form", false, &req{WildShapeHitPointsCurrent: n(3)}},
		{"wild shape negative", true, &req{WildShapeHitPointsCurrent: n(-1)}},
		{"wild shape above the beast", true, &req{WildShapeHitPointsCurrent: n(12)}},
		{"wild shape at the beast's maximum", true, &req{WildShapeHitPointsCurrent: n(11)}},
		{"wild shape at 0 ends the form", true, &req{WildShapeHitPointsCurrent: n(0)}},
		{"wild shape is checked before the hit points", true, &req{WildShapeHitPointsCurrent: n(99), HitPointsCurrent: n(99)}},
		// Hit points.
		{"hit points at 0", true, &req{HitPointsCurrent: n(0)}},
		{"hit points at the maximum", true, &req{HitPointsCurrent: n(30)}},
		{"hit points above the maximum", true, &req{HitPointsCurrent: n(31)}},
		{"negative hit points", true, &req{HitPointsCurrent: n(-5)}},
		{"hit points are checked before the temporary ones", true, &req{HitPointsCurrent: n(31), HitPointsTemporary: n(-1)}},
		// Temporary hit points.
		{"temporary at 0", true, &req{HitPointsTemporary: n(0)}},
		{"temporary at the limit", true, &req{HitPointsTemporary: n(MaxTemporaryHitPoints)}},
		{"temporary above the limit", true, &req{HitPointsTemporary: n(MaxTemporaryHitPoints + 1)}},
		{"negative temporary", true, &req{HitPointsTemporary: n(-1)}},
		{"temporary is checked before the slots", true, &req{HitPointsTemporary: n(-1), SpellSlotsUsed: slots(9, 1)}},
		// Spell slots.
		{"slots of three levels", true, &req{SpellSlotsUsed: slots(3, 2, 1, 5, 2, 0)}},
		{"a created slot counts in the total", true, &req{SpellSlotsUsed: slots(1, 5)}},
		{"a slot above the total", true, &req{SpellSlotsUsed: slots(1, 6)}},
		{"a negative slot", true, &req{SpellSlotsUsed: slots(2, -1)}},
		{"a level without slots", true, &req{SpellSlotsUsed: slots(4, 0)}},
		{"level 0", true, &req{SpellSlotsUsed: slots(0, 0)}},
		{"a level twice", true, &req{SpellSlotsUsed: slots(1, 1, 1, 2)}},
		{"a level twice is found before a bad used", true, &req{SpellSlotsUsed: slots(2, 1, 2, 99)}},
		{"the first bad entry wins", true, &req{SpellSlotsUsed: slots(1, 1, 7, 0, 2, 99)}},
		{"slots are checked before the pact slots", true, &req{SpellSlotsUsed: slots(7, 0), PactSlotsUsed: n(9)}},
		// Pact slots.
		{"pact slots without pact magic", false, &req{PactSlotsUsed: n(0)}},
		{"pact slots at the total", true, &req{PactSlotsUsed: n(2)}},
		{"pact slots at 0", true, &req{PactSlotsUsed: n(0)}},
		{"pact slots above the total", true, &req{PactSlotsUsed: n(3)}},
		{"pact slots are checked before the hit dice", true, &req{PactSlotsUsed: n(3), HitDiceUsed: n(99)}},
		// Hit dice.
		{"a count and sizes together", true, &req{HitDiceUsed: n(1), HitDiceUsedByDie: map[int32]int32{6: 1}}},
		{"a count and sizes together, both bad", true, &req{HitDiceUsed: n(99), HitDiceUsedByDie: map[int32]int32{8: 99}}},
		{"a count spends the largest dice first", true, &req{HitDiceUsed: n(3)}},
		{"every hit die", true, &req{HitDiceUsed: n(5)}},
		{"no hit die", true, &req{HitDiceUsed: n(0)}},
		{"more hit dice than the level", true, &req{HitDiceUsed: n(6)}},
		{"negative hit dice", true, &req{HitDiceUsed: n(-1)}},
		{"a size set", true, &req{HitDiceUsedByDie: map[int32]int32{6: 2}}},
		{"a size cleared", true, &req{HitDiceUsedByDie: map[int32]int32{10: 0}}},
		{"two sizes", true, &req{HitDiceUsedByDie: map[int32]int32{10: 2, 6: 3}}},
		{"a size the character has not", true, &req{HitDiceUsedByDie: map[int32]int32{8: 1}}},
		{"a size above its count", true, &req{HitDiceUsedByDie: map[int32]int32{10: 3}}},
		{"a negative size", true, &req{HitDiceUsedByDie: map[int32]int32{6: -1}}},
		{"hit dice are checked before the resources", true, &req{HitDiceUsed: n(9), ResourcesUsed: []*playv1.ResourceUsed{res("nope", 0)}}},
		// Resources.
		{"a resource spent", true, &req{ResourcesUsed: []*playv1.ResourceUsed{res("channel_divinity", 2)}}},
		{"a resource given back", true, &req{ResourcesUsed: []*playv1.ResourceUsed{res("second_wind", 0)}}},
		{"two resources", true, &req{ResourcesUsed: []*playv1.ResourceUsed{res("arcane_recovery", 0), res("channel_divinity", 1)}}},
		{"a resource the character has not", true, &req{ResourcesUsed: []*playv1.ResourceUsed{res("rage", 1)}}},
		{"a resource above its total", true, &req{ResourcesUsed: []*playv1.ResourceUsed{res("second_wind", 2)}}},
		{"a negative resource", true, &req{ResourcesUsed: []*playv1.ResourceUsed{res("second_wind", -1)}}},
		{"a resource twice", true, &req{ResourcesUsed: []*playv1.ResourceUsed{res("second_wind", 0), res("second_wind", 1)}}},
		{"an unknown key is found before it repeats", true, &req{ResourcesUsed: []*playv1.ResourceUsed{res("rage", 0), res("rage", 0)}}},
		{"no resource on a plain character", false, &req{ResourcesUsed: []*playv1.ResourceUsed{res("second_wind", 0)}}},
		// Everything at once.
		{"everything at once", true, &req{
			WildShapeHitPointsCurrent: n(4), HitPointsCurrent: n(20), HitPointsTemporary: n(7),
			SpellSlotsUsed: slots(1, 0, 3, 2), PactSlotsUsed: n(0), HitDiceUsedByDie: map[int32]int32{6: 1, 10: 0},
			ResourcesUsed: []*playv1.ResourceUsed{res("arcane_recovery", 0), res("second_wind", 0), res("channel_divinity", 2)},
		}},
	}
	type outcome struct {
		Name    string `json:"name"`
		Request any    `json:"request"`
		Field   string `json:"field,omitempty"`
		Error   string `json:"error,omitempty"`
		After   any    `json:"after,omitempty"`
	}
	out := make([]outcome, 0, len(cases)+2)
	out = append(out,
		outcome{Name: "the rich character before", After: vitalsGoldenJSON(t, vitalsGoldenRichView())},
		outcome{Name: "the plain character before", After: vitalsGoldenJSON(t, vitalsGoldenPlain())})
	for _, c := range cases {
		v := vitalsGoldenPlain()
		if c.rich {
			v = vitalsGoldenRichView()
		}
		o := outcome{Name: c.name, Request: vitalsGoldenJSON(t, c.req)}
		err := applyVitalsChange(v, c.req)
		switch fe, ok := errors.AsType[*fieldError](err); {
		case ok:
			o.Field, o.Error = fe.field, fe.Error()
		case err != nil:
			t.Fatalf("%s: applyVitalsChange() error = %v, want a fieldError or nil", c.name, err)
		default:
			o.After = vitalsGoldenJSON(t, v)
		}
		out = append(out, o)
	}
	checkVitalsGolden(t, "apply_vitals_change.golden.json", out)

	// A refused change leaves on v what the checks before it set: AdjustVitals throws v
	// away, so nothing of it is stored.
	v := vitalsGoldenRichView()
	if err := applyVitalsChange(v, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: n(1), HitPointsTemporary: n(1000)}); err == nil || v.GetHitPointsCurrent() != 1 {
		t.Errorf("after a refusal = %d hit points, %v; want 1 and the refusal", v.GetHitPointsCurrent(), err)
	}
}

// TestApplyVitalsChangeHitDiceSizesTwoBadOrder: with two bad sizes in the map, which one
// is named depends on Go's map order. Finding: the refusal of hit_dice_used_by_die is not
// deterministic when more than one size is wrong; frozen as one of the two.
func TestApplyVitalsChangeHitDiceSizesTwoBadOrder(t *testing.T) {
	t.Parallel()
	for range 20 {
		v := vitalsGoldenRichView()
		err := applyVitalsChange(v, &playv1.AdjustCharacterVitalsRequest{HitDiceUsedByDie: map[int32]int32{8: 1, 12: 1}})
		fe, ok := errors.AsType[*fieldError](err)
		if !ok || (fe.field != "hit_dice_used_by_die[8]" && fe.field != "hit_dice_used_by_die[12]") || fe.err.Error() != "must be a die size the character has" {
			t.Fatalf("error = %v, want a die size the character has, on [8] or [12]", err)
		}
	}
}

// TestPlanRestGolden freezes planRest: for each character and rest, what the master's
// preview says and the vitals after, or the refusal.
func TestPlanRestGolden(t *testing.T) {
	t.Parallel()
	rich := vitalsGoldenRich()
	aided := vitalsGoldenRichRow()
	aided.HitPointsMaxBonus = new(int32(5))
	aided.HitPointsCurrent = new(int32(33))
	down := vitalsGoldenRichRow()
	down.HitPointsCurrent = new(int32(0))
	spentAll := vitalsGoldenRichRow()
	spentAll.HitDiceUsedByDie = []byte(`{"10":2,"6":3}`)
	createdOnly := vitalsGoldenRichRow()
	createdOnly.SpellSlotsCreated = []int32{0, 0, 0, 2} // two 4th-level slots the sheet has none of
	createdOnly.SpellSlotsUsed = []int32{0, 0, 0, 1}
	level1 := vitalsMax{hitPoints: 8, hitDice: []rules.HitDice{{Die: 8, Count: 1}}, hitDiceTotal: 1}
	level1Row := vitalsRow{ID: "c-l1", Name: "Novata", HitPointsCurrent: new(int32(3)), HitDiceUsedByDie: []byte(`{"8":1}`)}
	cases := []struct {
		name   string
		kind   string
		row    vitalsRow
		m      vitalsMax
		choice rules.HitDiceUsed
	}{
		{"short rest", rules.RestShort, vitalsGoldenRichRow(), rich, nil},
		{"short rest ignores a hit dice choice", rules.RestShort, vitalsGoldenRichRow(), rich, rules.HitDiceUsed{8: 9}},
		{"long rest, largest dice first", rules.RestLong, vitalsGoldenRichRow(), rich, nil},
		{"long rest, every die spent", rules.RestLong, spentAll, rich, nil},
		{"long rest, a chosen d6", rules.RestLong, spentAll, rich, rules.HitDiceUsed{6: 2}},
		{"long rest, an empty choice gives no die back", rules.RestLong, spentAll, rich, rules.HitDiceUsed{}},
		{"long rest, a choice above the limit", rules.RestLong, spentAll, rich, rules.HitDiceUsed{6: 3}},
		{"long rest, a size the character has not", rules.RestLong, spentAll, rich, rules.HitDiceUsed{8: 1}},
		{"long rest, a size not spent", rules.RestLong, vitalsGoldenRichRow(), rich, rules.HitDiceUsed{6: 1}},
		{"long rest, a zero count", rules.RestLong, spentAll, rich, rules.HitDiceUsed{6: 0}},
		{"long rest under Aid", rules.RestLong, aided, rich, nil},
		{"short rest under Aid", rules.RestShort, aided, rich, nil},
		{"long rest at 0 hit points gives nothing", rules.RestLong, down, rich, nil},
		{"short rest at 0 hit points still recharges", rules.RestShort, down, rich, nil},
		{"long rest drops a level of created slots only", rules.RestLong, createdOnly, rich, nil},
		{"long rest of a fresh character", rules.RestLong, vitalsRow{ID: "c-fresh", Name: "Nova"}, wizard3(), nil},
		{"short rest of a fresh character", rules.RestShort, vitalsRow{ID: "c-fresh", Name: "Nova"}, wizard3(), nil},
		{"long rest of a level 1 character gives back one die", rules.RestLong, level1Row, level1, nil},
		{"an unknown kind is a short rest without the short-rest gains", "nap", vitalsGoldenRichRow(), rich, nil},
	}
	type outcome struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		Before  any    `json:"before"`
		Error   string `json:"error,omitempty"`
		Preview any    `json:"preview,omitempty"`
		After   any    `json:"after,omitempty"`
		Same    bool   `json:"unchanged,omitempty"`
	}
	out := make([]outcome, 0, len(cases))
	for _, c := range cases {
		v := vitalsToProto(c.row, c.m)
		was := proto.CloneOf(v)
		o := outcome{Name: c.name, Kind: c.kind, Before: vitalsGoldenJSON(t, v)}
		plan, err := planRest(c.kind, v, c.m, c.choice)
		if !proto.Equal(v, was) {
			t.Errorf("%s: planRest changed the vitals it was given", c.name)
		}
		if err != nil {
			if !errors.Is(err, link.ErrBadHitDiceChoice) {
				t.Errorf("%s: error = %v, want link.ErrBadHitDiceChoice", c.name, err)
			}
			o.Error = err.Error()
		} else {
			o.Preview, o.After, o.Same = vitalsGoldenJSON(t, plan.preview), vitalsGoldenJSON(t, plan.after), proto.Equal(plan.after, v)
		}
		out = append(out, o)
	}
	checkVitalsGolden(t, "plan_rest.golden.json", out)
}
