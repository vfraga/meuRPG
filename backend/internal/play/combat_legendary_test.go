package play

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Legendary creatures (MR-042, RN-20, RN-29): what a stat block with legendary
// actions has spent is kept on the combatant (creature_spent), the master reads what
// is left and the options, and no player reads any of it. These tests need the
// database (MEURPG_TEST_DATABASE_URL).

const (
	adultRedDragon = "monster:adult-red-dragon"
	// streamHints is how many changes of the stream the test reads after the turn ends.
	streamHints = 2
)

// dragonFight is a combat with the party, the goblin and an adult red dragon ("Draco",
// in plain sight), which plays first. It returns the combat as the master reads it
// and the dragon's combatant ID.
func dragonFight(t *testing.T) (*armed, *playv1.Encounter, string) {
	t.Helper()
	a := newArmed(t)
	e := a.monsterSetup(t, true)
	a.h.roller.queue(20)
	res := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey = adultRedDragon
		r.Count = 1
		r.Name = "Draco"
		r.Hidden = new(false)
	})
	e = a.begin(t, a.get(t, a.master))
	if e.GetCurrentCombatantId() != res.GetCombatantIds()[0] {
		t.Fatalf("on turn = %s, want the dragon", e.GetCurrentCombatantId())
	}
	return a, e, res.GetCombatantIds()[0]
}

// spentOf reads the column as the database holds it.
func (a *armed) spentOf(t *testing.T, id string) map[string]int {
	t.Helper()
	raw, err := playdb.New(a.h.pool).GetCombatantCreatureSpent(t.Context(), id)
	if err != nil {
		t.Fatalf("read creature_spent: %v", err)
	}
	return creatureSpentOf(raw)
}

// TestCreatureSpentHelper: a spend adds up, never goes below 0, a key at 0 is dropped,
// clear forgets one key, and a key the package does not know (one reserved for the N/day and Recharge actions) is kept.
func TestCreatureSpentHelper(t *testing.T) {
	t.Parallel()
	a, _, id := dragonFight(t)
	q := playdb.New(a.h.pool)
	ctx := t.Context()
	if got := a.spentOf(t, id); len(got) != 0 {
		t.Fatalf("a new combatant has spent %v, want nothing", got)
	}
	if err := q.SetCombatantCreatureSpent(ctx, playdb.SetCombatantCreatureSpentParams{ID: id, CreatureSpent: []byte(`{"use:fire-breath":2}`)}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		key   string
		delta int
		want  int
	}{{spentLegendaryActions, 2, 2}, {spentLegendaryActions, 1, 3}, {spentLegendaryActions, -1, 2}, {spentLegendaryResistance, 1, 1}, {spentLegendaryActions, -9, 0}} {
		got, err := bumpCreatureSpent(ctx, q, id, step.key, step.delta)
		if err != nil || got[step.key] != step.want {
			t.Fatalf("bump(%s, %d) = %v, %v; want %d", step.key, step.delta, got, err, step.want)
		}
	}
	got := a.spentOf(t, id)
	if _, kept := got[spentLegendaryActions]; kept || got[spentLegendaryResistance] != 1 || got["use:fire-breath"] != 2 || len(got) != 2 {
		t.Errorf("after the spends the column = %v, want legendary_resistance 1 and use:fire-breath 2 only", got)
	}
}

// TestCreatureSpentKeepsWhatItDoesNotKnow: a key of another owner (one reserved for the N/day and Recharge actions), whatever its
// value is, comes out of a spend as it went in.
func TestCreatureSpentKeepsWhatItDoesNotKnow(t *testing.T) {
	t.Parallel()
	a, _, id := dragonFight(t)
	q := playdb.New(a.h.pool)
	const others = `{"recharge:fire-breath": {"ready": false, "since": [1, 2]}, "use:note": "text", "use:x": 2.5}`
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET creature_spent = $2::JSONB WHERE id = $1`, id, others); err != nil {
		t.Fatal(err)
	}
	if _, err := bumpCreatureSpent(t.Context(), q, id, spentLegendaryActions, 1); err != nil {
		t.Fatalf("bump error = %v", err)
	}
	var kept bool
	if err := a.h.pool.QueryRow(t.Context(), `SELECT creature_spent @> $2::JSONB AND creature_spent->'legendary_actions' = '1'::JSONB FROM combatants WHERE id = $1`, id, others).Scan(&kept); err != nil || !kept {
		t.Errorf("after a spend the column lost what it does not know: kept = %v, err = %v", kept, err)
	}
}

// TestCreatureSpentRefusesWhatDoesNotFitIn4KiB: a spend that would take the column over
// 4096 bytes is refused with a clear error and changes nothing, and the database's own
// CHECK refuses a value written around the helper.
func TestCreatureSpentRefusesWhatDoesNotFitIn4KiB(t *testing.T) {
	t.Parallel()
	a, _, id := dragonFight(t)
	q := playdb.New(a.h.pool)
	ctx := t.Context()
	// Nearly full: the database writes ": " and ", " in the text it measures, so the
	// pairs below take 14 bytes each there, 12 in compact JSON.
	var full strings.Builder
	full.WriteString(`{"recharge:fill":1`)
	for i := range 290 {
		full.WriteString(`,"use:k` + string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + `":1`)
	}
	full.WriteString(`}`)
	if err := q.SetCombatantCreatureSpent(ctx, playdb.SetCombatantCreatureSpentParams{ID: id, CreatureSpent: []byte(full.String())}); err != nil {
		t.Fatalf("a column of %d bytes is refused: %v", full.Len(), err)
	}
	_, err := bumpCreatureSpent(ctx, q, id, "use:one-more-key-that-takes-it-over-the-limit", 1)
	if !errors.Is(err, errCreatureSpentFull) || !strings.Contains(err.Error(), "4096") { // canary-ok: the limit the refusal itself names, a positive check
		t.Errorf("bump over the limit error = %v, want errCreatureSpentFull naming 4096", err)
	}
	if got := a.spentOf(t, id); got["recharge:fill"] != 1 || got["use:one-more-key-that-takes-it-over-the-limit"] != 0 {
		t.Errorf("a refused spend changed the column")
	}
	tooBig := `{"x":"` + strings.Repeat("y", 4100) + `"}`
	if _, err := a.h.pool.Exec(ctx, `UPDATE combatants SET creature_spent = $2::JSONB WHERE id = $1`, id, tooBig); err == nil {
		t.Error("the database accepted a creature_spent over 4096 bytes")
	}
	if _, err := a.h.pool.Exec(ctx, `UPDATE combatants SET creature_spent = '[]'::JSONB WHERE id = $1`, id); err == nil {
		t.Error("the database accepted a creature_spent that is not an object")
	}
}

// TestCreatureSpentBoundaryIs4096BytesOfJSONBText: 4096 bytes in the database's text form
// are accepted and 4097 refused, by the helper and by the database's CHECK.
func TestCreatureSpentBoundaryIs4096BytesOfJSONBText(t *testing.T) {
	t.Parallel()
	a, _, id := dragonFight(t)
	ctx := t.Context()
	q := playdb.New(a.h.pool)
	// {"x": "..."} is 9 bytes with an empty string.
	for _, tc := range []struct {
		text int
		ok   bool
	}{{creatureSpentLimit, true}, {creatureSpentLimit + 1, false}} {
		body := json.RawMessage(`"` + strings.Repeat("y", tc.text-9) + `"`)
		err := writeCreatureSpent(ctx, q, id, spentValues{"x": body})
		if tc.ok != (err == nil) || (!tc.ok && !errors.Is(err, errCreatureSpentFull)) {
			t.Errorf("%d bytes: error = %v, want accepted = %v", tc.text, err, tc.ok)
		}
	}
	var n int
	if err := a.h.pool.QueryRow(ctx, `SELECT octet_length(creature_spent::TEXT) FROM combatants WHERE id = $1`, id).Scan(&n); err != nil || n != creatureSpentLimit {
		t.Errorf("the stored column has %d bytes (err %v), want %d", n, err, creatureSpentLimit)
	}
	if _, err := a.h.pool.Exec(ctx, `UPDATE combatants SET creature_spent = $2::JSONB WHERE id = $1`, id, `{"x": "`+strings.Repeat("y", creatureSpentLimit-8)+`"}`); err == nil {
		t.Error("the database accepted 4097 bytes")
	}
}

// TestLegendaryActionsComeBackWhenTheCreaturesOwnTurnStarts: the start of the dragon's
// turn clears the legendary actions it spent and nothing else it keeps; its own turn
// ending does not.
func TestLegendaryActionsComeBackWhenTheCreaturesOwnTurnStarts(t *testing.T) {
	t.Parallel()
	a, e, id := dragonFight(t)
	q := playdb.New(a.h.pool)
	ctx := t.Context()
	e = a.mustEndTurn(t, a.master, e) // the dragon's first turn is over
	for key, n := range map[string]int{spentLegendaryActions: 3, spentLegendaryResistance: 1, "use:fire-breath": 1} {
		if _, err := bumpCreatureSpent(ctx, q, id, key, n); err != nil {
			t.Fatalf("bump(%s) error = %v", key, err)
		}
	}
	for range 10 {
		if e.GetCurrentCombatantId() == id {
			break
		}
		if got := a.spentOf(t, id)[spentLegendaryActions]; got != 3 {
			t.Fatalf("on the turn of %s the legendary actions spent = %d, want 3 until the dragon's turn", e.GetCurrentCombatantId(), got)
		}
		e = a.mustEndTurn(t, a.master, e)
	}
	if e.GetCurrentCombatantId() != id {
		t.Fatalf("the dragon's turn never came back; on turn = %s", e.GetCurrentCombatantId())
	}
	got := a.spentOf(t, id)
	if _, kept := got[spentLegendaryActions]; kept || got[spentLegendaryResistance] != 1 || got["use:fire-breath"] != 1 {
		t.Errorf("at the start of its turn the column = %v, want legendary_actions gone and the other keys kept", got)
	}
}

// legendaryOf reads the legendary state of the dragon as u sees it.
func (a *armed) legendaryOf(t *testing.T, u *user) *playv1.LegendaryState {
	t.Helper()
	return byLabel(t, a.get(t, u), "Draco").GetLegendary()
}

// TestRN20_LegendaryStateIsTheMastersAlone: the master reads the dragon's 3 legendary
// actions, its 3 uses of Legendary Resistance and its options (the positive control);
// no player reads the field, not even empty, in the combat or in the stream. A creature
// without legendary actions has no field for the master either.
func TestRN20_LegendaryStateIsTheMastersAlone(t *testing.T) {
	t.Parallel()
	a, e, id := dragonFight(t)
	w := a.caio.watch(t, a.campaignID)
	w.ready(t)

	l := a.legendaryOf(t, a.master)
	if l == nil || l.GetActionsPerRound() != 3 || l.GetActionsLeft() != 3 || l.GetResistancePerDay() != 3 || l.GetResistanceLeft() != 3 {
		t.Fatalf("the master reads %v, want 3/3 legendary actions and 3/3 Legendary Resistance", l)
	}
	if o := l.GetOptions(); len(o) != 3 || o[0].GetNamePt() != "Detectar" || o[1].GetNamePt() != "Ataque de cauda" || o[1].GetAttackKey() == "" || o[0].GetAttackKey() != "" || o[2].GetNamePt() != "Ataque de asas" {
		t.Errorf("the options' Portuguese names and attack keys = %v", o)
	}
	if o := l.GetOptions(); o[1].GetAttackDetailPt() != "Cauda: +14 · 2d8 + 8 de concussão · alcance 4,5 m" || o[0].GetAttackDetailPt() != "" || o[2].GetAttackDetailPt() != "" {
		t.Errorf("the options' attack detail = %q, %q, %q, want the tail's line alone", o[0].GetAttackDetailPt(), o[1].GetAttackDetailPt(), o[2].GetAttackDetailPt())
	}
	if len(l.GetOptions()) != 3 || l.GetOptions()[1].GetKey() != "tail-attack" || l.GetOptions()[1].GetKind() != "attack" || l.GetOptions()[2].GetCost() != 2 || !l.GetOptions()[2].GetAffordable() {
		t.Errorf("the master's options = %v, want Detect, Tail Attack (attack) and Wing Attack (cost 2)", l.GetOptions())
	}
	if _, err := bumpCreatureSpent(t.Context(), playdb.New(a.h.pool), id, spentLegendaryActions, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := bumpCreatureSpent(t.Context(), playdb.New(a.h.pool), id, spentLegendaryResistance, 1); err != nil {
		t.Fatal(err)
	}
	l = a.legendaryOf(t, a.master)
	if l.GetActionsLeft() != 1 || l.GetResistanceLeft() != 2 || !l.GetOptions()[1].GetAffordable() || l.GetOptions()[2].GetAffordable() {
		t.Errorf("after the spends the master reads %v, want 1 action and 2 uses left, Wing Attack no longer affordable", l)
	}

	for who, u := range map[string]*user{"Caio": a.caio, "Ana": a.ana} {
		seen := a.get(t, u)
		if c := byLabel(t, seen, "Draco"); c.GetLegendary() != nil {
			t.Errorf("%s reads the dragon's legendary state %v, want the field absent", who, c.GetLegendary())
		}
		if js := asJSON(t, seen); strings.Contains(js, `"legendary"`) {
			t.Errorf("%s: the combat as JSON has the legendary state:\n%s", who, js)
		}
	}
	// The stream: what the dragon's turn ending sends has no content.
	a.mustEndTurn(t, a.master, e)
	var all []*playv1.WatchGameSessionResponse
	for len(all) < streamHints {
		all = append(all, w.nextChange(t))
	}
	if js := asJSONAll(t, all); strings.Contains(js, "legendary") || strings.Contains(js, "Draco") {
		t.Errorf("a player's stream carries the legendary state: %s", js)
	}
	// A bandit is no legendary creature: no field for the master either.
	a.h.roller.queue(3)
	a.mustAddMonsters(t, a.get(t, a.master), func(r *playv1.AddMonstersRequest) { r.Count = 1; r.Name = "Capanga"; r.Hidden = new(false) })
	if c := byLabel(t, a.get(t, a.master), "Capanga"); c.GetLegendary() != nil {
		t.Errorf("the master reads a legendary state %v on a bandit", c.GetLegendary())
	}
}
