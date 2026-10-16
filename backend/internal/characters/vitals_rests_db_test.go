package characters

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// AdjustVitals and TakeRest against the database: who answers not_found, what each
// refusal says, and what lands in character_vitals (and character_wild_shapes) after
// each accepted change. The pure part (every field's check, every rest's plan) is
// frozen in vitals_golden_test.go.

// vitalsDBTable is a campaign with Pensantus (a level 3 wizard) and a level 3 warlock,
// each a player's, plus a third player's fresh wizard, an NPC and a dead character.
type vitalsDBTable struct {
	h                     *harness
	master                *user
	campaign              string
	wizard, warlock, idle *charactersv1.Character
	npc, dead             *charactersv1.Character
}

func newVitalsDBTable(t *testing.T) *vitalsDBTable {
	t.Helper()
	h := newHarness(t)
	tb := &vitalsDBTable{h: h, master: h.newUser("Mestre")}
	ana, bia, caio, dora := h.newUser("Ana"), h.newUser("Bia"), h.newUser("Caio"), h.newUser("Dora")
	tb.campaign = h.newCampaign(tb.master, "Mirathel", ana, bia, caio, dora)
	tb.wizard = ana.createPensantus(t, tb.campaign)
	tb.warlock = bia.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Bruxo",
		casterSheet("class:warlock", "", 3, 8, 14, 14, 10, 10, 16, nil, nil, "feature:pact-of-the-chain",
			"feature:eldritch-invocation-armor-of-shadows", "feature:eldritch-invocation-beast-speech"))
	tb.idle = caio.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ocioso", pensantusSheet())
	tb.npc = tb.master.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, "Bandido", enemySheet())
	tb.dead = tb.master.markDead(t, dora.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Morta", pensantusSheet()))
	return tb
}

// vitalsDBAdjust runs AdjustVitals in its own transaction.
func (tb *vitalsDBTable) vitalsDBAdjust(campaign, characterID string, req *playv1.AdjustCharacterVitalsRequest) (before, after *playv1.CharacterVitals, err error) {
	err = db.InTx(tb.h.t.Context(), tb.h.pool, func(tx pgx.Tx) error {
		var err error
		before, after, err = tb.h.svc.AdjustVitals(tb.h.t.Context(), tx, campaign, characterID, req)
		return err
	})
	return before, after, err
}

// vitalsDBExec runs a statement on the test database, or fails the test.
func (tb *vitalsDBTable) vitalsDBExec(sql string, args ...any) {
	tb.h.t.Helper()
	if _, err := tb.h.pool.Exec(tb.h.t.Context(), sql, args...); err != nil {
		tb.h.t.Fatalf("%s: %v", sql, err)
	}
}

// vitalsDBRow is a character's character_vitals and character_wild_shapes rows as one
// line of text, "no row" for none: what the tests compare.
func (tb *vitalsDBTable) vitalsDBRow(characterID string) string {
	tb.h.t.Helper()
	var line string
	err := tb.h.pool.QueryRow(tb.h.t.Context(), `
		SELECT concat_ws(' ',
			'hp=' || COALESCE(v.hit_points_current::TEXT, 'null'),
			'tmp=' || v.hit_points_temporary::TEXT,
			'slots=' || v.spell_slots_used::TEXT,
			'created=' || v.spell_slots_created::TEXT,
			'pact=' || v.pact_slots_used::TEXT,
			'dice=' || v.hit_dice_used::TEXT,
			'by_die=' || COALESCE(v.hit_dice_used_by_die::TEXT, 'null'),
			'resources=' || v.resources_used::TEXT,
			'bonus=' || v.hit_points_max_bonus::TEXT,
			'exhaustion=' || v.exhaustion_level::TEXT,
			'rev=' || v.revision::TEXT,
			'beast=' || COALESCE(ws.beast || '/' || ws.hp::TEXT, 'none'))
		FROM character_vitals AS v
		LEFT JOIN character_wild_shapes AS ws ON ws.character_id = v.character_id
		WHERE v.character_id = $1`, characterID).Scan(&line)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return "no row"
		}
		tb.h.t.Fatalf("read the vitals row: %v", err)
	}
	return line
}

// wantVitalsRefusal checks a Connect error's code, its whole message and, for
// invalid_argument, the InvalidField detail.
func wantVitalsRefusal(t *testing.T, call string, err error, code connect.Code, message, field string) {
	t.Helper()
	if connect.CodeOf(err) != code || err.Error() != message {
		t.Errorf("%s error = %v (%v), want %v %q", call, err, connect.CodeOf(err), code, message)
		return
	}
	if code == connect.CodeInvalidArgument {
		if got := invalidField(t, call, err); got != field {
			t.Errorf("%s InvalidField = %q, want %q", call, got, field)
		}
	}
}

// TestAdjustVitalsRefusals: not_found for anything but a living, active player character
// of the campaign (checked before the request), then the request's own refusal as an
// invalid_argument naming the field; nothing is written either way.
func TestAdjustVitalsRefusals(t *testing.T) {
	t.Parallel()
	tb := newVitalsDBTable(t)
	other := tb.h.newCampaign(tb.master, "Outra")
	hp := int32(1)
	set := &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp}
	for _, c := range []struct{ name, campaign, id string }{
		{"not a UUID", tb.campaign, "pensantus"},
		{"nobody", tb.campaign, "00000000-0000-4000-8000-000000000001"},
		{"an NPC", tb.campaign, tb.npc.GetId()},
		{"a dead character", tb.campaign, tb.dead.GetId()},
		{"another campaign's", other, tb.wizard.GetId()},
	} {
		_, _, err := tb.vitalsDBAdjust(c.campaign, c.id, set)
		wantVitalsRefusal(t, "AdjustVitals("+c.name+")", err, connect.CodeNotFound, "not_found: character not found", "")
		// The character is looked up before the request is read.
		_, _, err = tb.vitalsDBAdjust(c.campaign, c.id, &playv1.AdjustCharacterVitalsRequest{})
		wantVitalsRefusal(t, "AdjustVitals("+c.name+", empty)", err, connect.CodeNotFound, "not_found: character not found", "")
	}
	v, err := tb.h.svc.GetVitals(t.Context(), tb.campaign, tb.wizard.GetId())
	if err != nil {
		t.Fatalf("GetVitals() error = %v", err)
	}
	maxHP := v.GetHitPointsMax()
	above := maxHP + 1
	for _, c := range []struct {
		name         string
		req          *playv1.AdjustCharacterVitalsRequest
		field, error string
	}{
		{"nothing", &playv1.AdjustCharacterVitalsRequest{}, "request", "request must set at least one value to change"},
		{"too many hit points", &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &above}, "hit_points_current", fmt.Sprintf("hit_points_current must be 0 to %d", maxHP)},
		{"a beast's hit points", &playv1.AdjustCharacterVitalsRequest{WildShapeHitPointsCurrent: &hp}, "wild_shape_hit_points_current", "wild_shape_hit_points_current must be unset: the character is not in a beast form"},
		{"pact slots of a wizard", &playv1.AdjustCharacterVitalsRequest{PactSlotsUsed: &hp}, "pact_slots_used", "pact_slots_used must be unset: the character has no pact magic slots"},
		{"a 3rd-level slot", &playv1.AdjustCharacterVitalsRequest{SpellSlotsUsed: []*playv1.SpellSlotsUsed{{Level: 3, Used: 1}}}, "spell_slots_used[0].level", "spell_slots_used[0].level must be a spell level the character has slots of"},
		{"a d8", &playv1.AdjustCharacterVitalsRequest{HitDiceUsedByDie: map[int32]int32{8: 1}}, "hit_dice_used_by_die[8]", "hit_dice_used_by_die[8] must be a die size the character has"},
		{"an unknown resource", &playv1.AdjustCharacterVitalsRequest{ResourcesUsed: []*playv1.ResourceUsed{{Key: "rage", Used: 1}}}, "resources_used[0].key", "resources_used[0].key must be a resource the character has"},
	} {
		_, _, err := tb.vitalsDBAdjust(tb.campaign, tb.wizard.GetId(), c.req)
		wantVitalsRefusal(t, "AdjustVitals("+c.name+")", err, connect.CodeInvalidArgument, "invalid_argument: "+c.error, c.field)
	}
	if got := tb.vitalsDBRow(tb.wizard.GetId()); got != "no row" {
		t.Errorf("the vitals after the refusals = %s, want no row", got)
	}
	// A sheet that does not read is the module's own error, for the caller to log.
	tb.vitalsDBExec(`UPDATE characters SET sheet = '{"full": 5}' WHERE id = $1`, tb.idle.GetId())
	_, _, err = tb.vitalsDBAdjust(tb.campaign, tb.idle.GetId(), set)
	if !errors.Is(err, errCorruptDocument) || connect.CodeOf(err) != connect.CodeUnknown {
		t.Errorf("AdjustVitals(a corrupt sheet) error = %v, want errCorruptDocument as it is", err)
	}
}

// TestAdjustVitalsWrites: each correction writes what it sets and keeps the rest as
// stored, including what today's sheet cannot show; the answer carries the stored
// revision.
func TestAdjustVitalsWrites(t *testing.T) {
	t.Parallel()
	tb := newVitalsDBTable(t)
	n := func(v int32) *int32 { return &v }
	id := tb.wizard.GetId()
	step := func(name string, req *playv1.AdjustCharacterVitalsRequest, want string) *playv1.CharacterVitals {
		t.Helper()
		was, err := tb.h.svc.GetVitals(t.Context(), tb.campaign, id)
		if err != nil {
			t.Fatalf("%s: GetVitals() error = %v", name, err)
		}
		before, after, err := tb.vitalsDBAdjust(tb.campaign, id, req)
		if err != nil {
			t.Fatalf("%s: AdjustVitals() error = %v", name, err)
		}
		if before.String() != was.String() {
			t.Errorf("%s: before = %v, want what GetVitals read: %v", name, before, was)
		}
		now, err := tb.h.svc.GetVitals(t.Context(), tb.campaign, id)
		if err != nil {
			t.Fatalf("%s: GetVitals() after error = %v", name, err)
		}
		if after.String() != now.String() {
			t.Errorf("%s: after = %v, want what GetVitals reads now: %v", name, after, now)
		}
		if got := tb.vitalsDBRow(id); got != want {
			t.Errorf("%s: stored\n  %s\nwant\n  %s", name, got, want)
		}
		return after
	}
	// A first write of the temporary hit points leaves the current ones unset (full).
	step("temporary hit points", &playv1.AdjustCharacterVitalsRequest{HitPointsTemporary: n(5)},
		`hp=null tmp=5 slots={0,0,0,0,0,0,0,0,0} created={} pact=0 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=1 beast=none`)
	step("hit points and a slot", &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: n(7), SpellSlotsUsed: []*playv1.SpellSlotsUsed{{Level: 1, Used: 2}}},
		`hp=7 tmp=5 slots={2,0,0,0,0,0,0,0,0} created={} pact=0 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=2 beast=none`)
	// What the sheet cannot show stays: a 3rd-level slot used, a resource it lacks, a
	// created slot; a row from before the sizes were kept gets them.
	tb.vitalsDBExec(`UPDATE character_vitals SET spell_slots_used = '{2,0,1,0,0,0,0,0,0}', spell_slots_created = '{1,0,0,0,0,0,0,0,0}',
		resources_used = '{"gone": 2, "arcane_recovery": 1}', hit_dice_used = 2, hit_dice_used_by_die = NULL WHERE character_id = $1`, id)
	after := step("a resource given back", &playv1.AdjustCharacterVitalsRequest{ResourcesUsed: []*playv1.ResourceUsed{{Key: "arcane_recovery", Used: 0}}},
		`hp=7 tmp=5 slots={2,0,1,0,0,0,0,0,0} created={1,0,0,0,0,0,0,0,0} pact=0 dice=2 by_die={"6": 2} resources={"gone": 2} bonus=0 exhaustion=0 rev=3 beast=none`)
	if s := slotOf(after, 1); s.GetTotal() != 5 || s.GetCreated() != 1 || s.GetUsed() != 2 {
		t.Errorf("1st-level slots after = %v, want 2 of 5 with 1 created", s)
	}
	step("a resource spent and a size of hit dice", &playv1.AdjustCharacterVitalsRequest{
		ResourcesUsed: []*playv1.ResourceUsed{{Key: "arcane_recovery", Used: 1}}, HitDiceUsedByDie: map[int32]int32{6: 1},
	}, `hp=7 tmp=5 slots={2,0,1,0,0,0,0,0,0} created={1,0,0,0,0,0,0,0,0} pact=0 dice=1 by_die={"6": 1} resources={"arcane_recovery": 1, "gone": 2} bonus=0 exhaustion=0 rev=4 beast=none`)
	step("a count of hit dice", &playv1.AdjustCharacterVitalsRequest{HitDiceUsed: n(3), HitPointsCurrent: n(0)},
		`hp=0 tmp=5 slots={2,0,1,0,0,0,0,0,0} created={1,0,0,0,0,0,0,0,0} pact=0 dice=3 by_die={"6": 3} resources={"arcane_recovery": 1, "gone": 2} bonus=0 exhaustion=0 rev=5 beast=none`)

	// The beast's pool: a row of its own, 0 ends the form.
	tb.vitalsDBExec(`INSERT INTO character_wild_shapes (character_id, beast, hp, updated_at) VALUES ($1, 'monster:wolf', 5, now())`, id)
	after = step("the beast's hit points", &playv1.AdjustCharacterVitalsRequest{WildShapeHitPointsCurrent: n(3)},
		`hp=0 tmp=5 slots={2,0,1,0,0,0,0,0,0} created={1,0,0,0,0,0,0,0,0} pact=0 dice=3 by_die={"6": 3} resources={"arcane_recovery": 1, "gone": 2} bonus=0 exhaustion=0 rev=6 beast=monster:wolf/3`)
	if w := after.GetWildShape(); w.GetBeastKey() != "monster:wolf" || w.GetHitPointsCurrent() != 3 {
		t.Errorf("wild shape after = %v, want the wolf at 3", w)
	}
	_, _, err := tb.vitalsDBAdjust(tb.campaign, id, &playv1.AdjustCharacterVitalsRequest{WildShapeHitPointsCurrent: n(99)})
	wantVitalsRefusal(t, "AdjustVitals(a beast above its maximum)", err, connect.CodeInvalidArgument,
		fmt.Sprintf("invalid_argument: wild_shape_hit_points_current must be 0 to %d", after.GetWildShape().GetHitPointsMax()), "wild_shape_hit_points_current")
	after = step("the beast at 0", &playv1.AdjustCharacterVitalsRequest{WildShapeHitPointsCurrent: n(0), HitPointsTemporary: n(0)},
		`hp=0 tmp=0 slots={2,0,1,0,0,0,0,0,0} created={1,0,0,0,0,0,0,0,0} pact=0 dice=3 by_die={"6": 3} resources={"arcane_recovery": 1, "gone": 2} bonus=0 exhaustion=0 rev=7 beast=none`)
	if after.GetWildShape() != nil {
		t.Errorf("wild shape after 0 = %v, want none", after.GetWildShape())
	}

	// A warlock's pact slots: set, then kept by a change of something else.
	wid := tb.warlock.GetId()
	if _, _, err := tb.vitalsDBAdjust(tb.campaign, wid, &playv1.AdjustCharacterVitalsRequest{PactSlotsUsed: n(1)}); err != nil {
		t.Fatalf("AdjustVitals(pact) error = %v", err)
	}
	if _, _, err := tb.vitalsDBAdjust(tb.campaign, wid, &playv1.AdjustCharacterVitalsRequest{HitPointsTemporary: n(2)}); err != nil {
		t.Fatalf("AdjustVitals(temporary) error = %v", err)
	}
	if got, want := tb.vitalsDBRow(wid), `hp=null tmp=2 slots={0,0,0,0,0,0,0,0,0} created={} pact=1 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=2 beast=none`; got != want {
		t.Errorf("the warlock's vitals = %s, want %s", got, want)
	}
	_, _, err = tb.vitalsDBAdjust(tb.campaign, wid, &playv1.AdjustCharacterVitalsRequest{PactSlotsUsed: n(3)})
	wantVitalsRefusal(t, "AdjustVitals(too many pact slots)", err, connect.CodeInvalidArgument, "invalid_argument: pact_slots_used must be 0 to 2", "pact_slots_used")
}

// restDBTake runs TakeRest in its own transaction and says what it returned: one line per
// character, "<name> rev <before>-><after>".
func (tb *vitalsDBTable) restDBTake(req *playv1.TakeRestRequest) (string, error) {
	var before, after []*playv1.CharacterVitals
	err := db.InTx(tb.h.t.Context(), tb.h.pool, func(tx pgx.Tx) error {
		var err error
		before, after, err = tb.h.svc.TakeRest(tb.h.t.Context(), tx, tb.campaign, req)
		return err
	})
	if err != nil {
		return "", err
	}
	if len(before) != len(after) {
		tb.h.t.Fatalf("TakeRest() returned %d before and %d after", len(before), len(after))
	}
	var b strings.Builder
	for i := range before {
		if before[i].GetCharacterId() != after[i].GetCharacterId() {
			tb.h.t.Fatalf("TakeRest() pairs %s with %s", before[i].GetName(), after[i].GetName())
		}
		fmt.Fprintf(&b, "%s rev %d->%d hp %d/%d->%d/%d exhaustion %d->%d; ", before[i].GetName(), before[i].GetRevision(), after[i].GetRevision(),
			before[i].GetHitPointsCurrent(), before[i].GetHitPointsMax(), after[i].GetHitPointsCurrent(), after[i].GetHitPointsMax(),
			before[i].GetExhaustionLevel(), after[i].GetExhaustionLevel())
		now, err := tb.h.svc.GetVitals(tb.h.t.Context(), tb.campaign, after[i].GetCharacterId())
		if err != nil {
			tb.h.t.Fatalf("GetVitals() error = %v", err)
		}
		if now.String() != after[i].String() {
			tb.h.t.Errorf("TakeRest() after for %s = %v, want what GetVitals reads: %v", after[i].GetName(), after[i], now)
		}
	}
	return b.String(), nil
}

// restDBSet runs one of the service's setters in its own transaction.
func (tb *vitalsDBTable) restDBSet(name string, set func(tx pgx.Tx) error) {
	tb.h.t.Helper()
	if err := db.InTx(tb.h.t.Context(), tb.h.pool, set); err != nil {
		tb.h.t.Fatalf("%s: %v", name, err)
	}
}

// TestTakeRestRefusals: the kind first, then each hit dice choice (a character not in the
// party, one named twice, a size named twice, a choice the rest cannot give); nothing is
// written.
func TestTakeRestRefusals(t *testing.T) {
	t.Parallel()
	tb := newVitalsDBTable(t)
	tb.h.adjustVitals(tb.campaign, tb.wizard.GetId(), &playv1.AdjustCharacterVitalsRequest{HitDiceUsed: new(int32(2)), HitPointsCurrent: new(int32(4))})
	wizardRow := tb.vitalsDBRow(tb.wizard.GetId())
	long := playv1.RestKind_REST_KIND_LONG
	dice := func(pairs ...int32) []*rulesv1.HitDice {
		var out []*rulesv1.HitDice
		for i := 0; i+1 < len(pairs); i += 2 {
			out = append(out, &rulesv1.HitDice{Faces: pairs[i], Count: pairs[i+1]})
		}
		return out
	}
	choice := func(id string, d ...int32) *playv1.HitDiceChoice {
		return &playv1.HitDiceChoice{CharacterId: id, Dice: dice(d...)}
	}
	_, err := tb.restDBTake(&playv1.TakeRestRequest{})
	wantVitalsRefusal(t, "TakeRest(no kind)", err, connect.CodeInvalidArgument, "invalid_argument: kind must be a short or a long rest", "")
	_, err = tb.restDBTake(&playv1.TakeRestRequest{Kind: playv1.RestKind(99), HitDiceChoices: []*playv1.HitDiceChoice{choice("nobody")}})
	wantVitalsRefusal(t, "TakeRest(an unknown kind)", err, connect.CodeInvalidArgument, "invalid_argument: kind must be a short or a long rest", "")
	for _, c := range []struct {
		name    string
		kind    playv1.RestKind
		choices []*playv1.HitDiceChoice
	}{
		{"a character not in the party", long, []*playv1.HitDiceChoice{choice(tb.npc.GetId(), 6, 1)}},
		{"a dead character", long, []*playv1.HitDiceChoice{choice(tb.dead.GetId(), 6, 1)}},
		{"a character twice", long, []*playv1.HitDiceChoice{choice(tb.wizard.GetId(), 6, 1), choice(tb.wizard.GetId(), 6, 1)}},
		{"a size twice", long, []*playv1.HitDiceChoice{choice(tb.wizard.GetId(), 6, 1, 6, 1)}},
		{"more dice than the limit", long, []*playv1.HitDiceChoice{choice(tb.wizard.GetId(), 6, 2)}},
		{"a size the character has not", long, []*playv1.HitDiceChoice{choice(tb.wizard.GetId(), 8, 1)}},
		{"a size of nobody's on a short rest is still checked for twice", playv1.RestKind_REST_KIND_SHORT, []*playv1.HitDiceChoice{choice(tb.warlock.GetId(), 8, 1, 8, 1)}},
		{"a bad choice of a later character", long, []*playv1.HitDiceChoice{choice(tb.wizard.GetId(), 6, 1), choice("not-a-character")}},
	} {
		_, err := tb.restDBTake(&playv1.TakeRestRequest{Kind: c.kind, HitDiceChoices: c.choices})
		if err != link.ErrBadHitDiceChoice { //nolint:errorlint // the very error play words
			t.Errorf("TakeRest(%s) error = %v, want link.ErrBadHitDiceChoice", c.name, err)
		}
	}
	// On a short rest the choice is read (each character and size once) and then ignored.
	if _, err := tb.restDBTake(&playv1.TakeRestRequest{Kind: playv1.RestKind_REST_KIND_SHORT, HitDiceChoices: []*playv1.HitDiceChoice{choice(tb.wizard.GetId(), 8, 9)}}); err != nil {
		t.Errorf("TakeRest(short, a choice the long rest would refuse) error = %v, want none", err)
	}
	if got := tb.vitalsDBRow(tb.wizard.GetId()); got != wizardRow {
		t.Errorf("the wizard's vitals after the refusals = %s, want %s", got, wizardRow)
	}
}

// TestTakeRestWrites: who a rest changes, what it stores for each and the revisions it
// returns: Aid's end bumps the revision a second time, the exhaustion goes down after it,
// and a character the rest changes nothing for is not returned unless its exhaustion
// goes.
func TestTakeRestWrites(t *testing.T) {
	t.Parallel()
	tb := newVitalsDBTable(t)
	n := func(v int32) *int32 { return &v }
	wiz, war, idle := tb.wizard.GetId(), tb.warlock.GetId(), tb.idle.GetId()
	tb.h.adjustVitals(tb.campaign, wiz, &playv1.AdjustCharacterVitalsRequest{
		HitPointsCurrent: n(5), HitPointsTemporary: n(3), HitDiceUsed: n(3),
		SpellSlotsUsed: []*playv1.SpellSlotsUsed{{Level: 1, Used: 3}, {Level: 2, Used: 1}},
		ResourcesUsed:  []*playv1.ResourceUsed{{Key: "arcane_recovery", Used: 1}},
	})
	tb.vitalsDBExec(`UPDATE character_vitals SET spell_slots_used = '{3,1,1,0,0,0,0,0,0}', spell_slots_created = '{1,0,0,0,0,0,0,0,0}',
		resources_used = resources_used || '{"gone": 2}' WHERE character_id = $1`, wiz)
	tb.restDBSet("Aid", func(tx pgx.Tx) error {
		_, _, err := tb.h.svc.SetHitPointsMaxBonus(t.Context(), tx, tb.campaign, wiz, 5)
		return err
	})
	tb.restDBSet("exhaustion", func(tx pgx.Tx) error {
		_, _, err := tb.h.svc.SetExhaustion(t.Context(), tx, tb.campaign, wiz, 2)
		return err
	})
	tb.h.adjustVitals(tb.campaign, war, &playv1.AdjustCharacterVitalsRequest{PactSlotsUsed: n(2), HitPointsCurrent: n(0)})
	tb.restDBSet("the idle one's exhaustion", func(tx pgx.Tx) error {
		_, _, err := tb.h.svc.SetExhaustion(t.Context(), tx, tb.campaign, idle, 1)
		return err
	})
	rows := func() string {
		return "wizard: " + tb.vitalsDBRow(wiz) + "\nwarlock: " + tb.vitalsDBRow(war) + "\nidle: " + tb.vitalsDBRow(idle)
	}
	check := func(name, gotOut, wantOut, wantRows string) {
		t.Helper()
		if gotOut != wantOut {
			t.Errorf("%s returned\n  %s\nwant\n  %s", name, gotOut, wantOut)
		}
		if got := rows(); got != wantRows {
			t.Errorf("%s stored\n%s\nwant\n%s", name, got, wantRows)
		}
	}
	start := rows()

	// A short rest: the warlock's pact slots come back (at 0 hit points too); the wizard
	// gets nothing back on one, so is left alone; nobody's exhaustion moves.
	out, err := tb.restDBTake(&playv1.TakeRestRequest{Kind: playv1.RestKind_REST_KIND_SHORT})
	if err != nil {
		t.Fatalf("TakeRest(short) error = %v", err)
	}
	check("TakeRest(short)", out, "Bruxo rev 1->2 hp 0/24->0/24 exhaustion 0->0; ",
		strings.Replace(start, "created={} pact=2 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=1", "created={0,0,0,0,0,0,0,0,0} pact=0 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=2", 1))

	// A long rest without food or drink: no exhaustion goes; the warlock at 0 hit points
	// gains nothing and is left alone.
	out, err = tb.restDBTake(&playv1.TakeRestRequest{
		Kind: playv1.RestKind_REST_KIND_LONG, WithoutFoodOrDrink: true,
		HitDiceChoices: []*playv1.HitDiceChoice{{CharacterId: wiz, Dice: []*rulesv1.HitDice{{Faces: 6, Count: 1}}}},
	})
	if err != nil {
		t.Fatalf("TakeRest(long, no food) error = %v", err)
	}
	check("TakeRest(long, no food)", out, "Pensantus rev 3->5 hp 10/28->23/23 exhaustion 2->2; ", strings.Join([]string{
		`wizard: hp=23 tmp=0 slots={0,0,0,0,0,0,0,0,0} created={0,0,0,0,0,0,0,0,0} pact=0 dice=2 by_die={"6": 2} resources={"gone": 2} bonus=0 exhaustion=2 rev=5 beast=none`,
		`warlock: hp=0 tmp=0 slots={0,0,0,0,0,0,0,0,0} created={0,0,0,0,0,0,0,0,0} pact=0 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=2 beast=none`,
		`idle: hp=null tmp=0 slots={} created={} pact=0 dice=0 by_die=null resources={} bonus=0 exhaustion=1 rev=1 beast=none`,
	}, "\n"))

	// A long rest with food and drink: one level of exhaustion goes from everyone who has
	// it. The characters the rest changes come first, in the party's order, each with the
	// exhaustion's revision; then the ones it changes in nothing else. Finding: the
	// warlock at 0 hit points loses a level too, though "a character must have at least 1
	// hit point" to gain a long rest's benefits (planRest gives it nothing else).
	tb.h.adjustVitals(tb.campaign, wiz, &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: n(9)})
	// The idle one spent a die and was never hurt: the rest gives the die back and keeps
	// its hit points unset (full, whatever the maximum becomes).
	tb.h.adjustVitals(tb.campaign, idle, &playv1.AdjustCharacterVitalsRequest{HitDiceUsed: n(1)})
	tb.restDBSet("the warlock's exhaustion", func(tx pgx.Tx) error {
		_, _, err := tb.h.svc.SetExhaustion(t.Context(), tx, tb.campaign, war, 1)
		return err
	})
	out, err = tb.restDBTake(&playv1.TakeRestRequest{Kind: playv1.RestKind_REST_KIND_LONG})
	if err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	check("TakeRest(long)", out, "Pensantus rev 6->8 hp 9/23->23/23 exhaustion 2->1; Ocioso rev 2->4 hp 23/23->23/23 exhaustion 1->0; Bruxo rev 3->4 hp 0/24->0/24 exhaustion 1->0; ", strings.Join([]string{
		`wizard: hp=23 tmp=0 slots={0,0,0,0,0,0,0,0,0} created={0,0,0,0,0,0,0,0,0} pact=0 dice=1 by_die={"6": 1} resources={"gone": 2} bonus=0 exhaustion=1 rev=8 beast=none`,
		`warlock: hp=0 tmp=0 slots={0,0,0,0,0,0,0,0,0} created={0,0,0,0,0,0,0,0,0} pact=0 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=4 beast=none`,
		`idle: hp=null tmp=0 slots={0,0,0,0,0,0,0,0,0} created={0,0,0,0,0,0,0,0,0} pact=0 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=4 beast=none`,
	}, "\n"))

	// Another long rest: the wizard's last die comes back and its last level of
	// exhaustion goes; the others have none to lose.
	out, err = tb.restDBTake(&playv1.TakeRestRequest{Kind: playv1.RestKind_REST_KIND_LONG})
	if err != nil {
		t.Fatalf("TakeRest(long, again) error = %v", err)
	}
	check("TakeRest(long, again)", out, "Pensantus rev 8->10 hp 23/23->23/23 exhaustion 1->0; ", strings.Join([]string{
		`wizard: hp=23 tmp=0 slots={0,0,0,0,0,0,0,0,0} created={0,0,0,0,0,0,0,0,0} pact=0 dice=0 by_die={} resources={"gone": 2} bonus=0 exhaustion=0 rev=10 beast=none`,
		`warlock: hp=0 tmp=0 slots={0,0,0,0,0,0,0,0,0} created={0,0,0,0,0,0,0,0,0} pact=0 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=4 beast=none`,
		`idle: hp=null tmp=0 slots={0,0,0,0,0,0,0,0,0} created={0,0,0,0,0,0,0,0,0} pact=0 dice=0 by_die={} resources={} bonus=0 exhaustion=0 rev=4 beast=none`,
	}, "\n"))

	// A hit die spent on a short rest keeps a resource the rest did not give back.
	tb.h.adjustVitals(tb.campaign, wiz, &playv1.AdjustCharacterVitalsRequest{ResourcesUsed: []*playv1.ResourceUsed{{Key: "arcane_recovery", Used: 1}}, HitPointsCurrent: n(2)})
	tb.restDBSet("SpendHitDie", func(tx pgx.Tx) error {
		_, _, _, _, err := tb.h.svc.SpendHitDie(t.Context(), tx, tb.campaign, wiz, 6, 4, 0)
		return err
	})
	if got, want := tb.vitalsDBRow(wiz), `hp=9 tmp=0 slots={0,0,0,0,0,0,0,0,0} created={0,0,0,0,0,0,0,0,0} pact=0 dice=1 by_die={"6": 1} resources={"arcane_recovery": 1, "gone": 2} bonus=0 exhaustion=0 rev=12 beast=none`; got != want {
		t.Errorf("the wizard after a hit die = %s, want %s", got, want)
	}

	// A rest that changes nothing returns nothing and writes nothing.
	before := rows()
	out, err = tb.restDBTake(&playv1.TakeRestRequest{Kind: playv1.RestKind_REST_KIND_SHORT})
	if err != nil {
		t.Fatalf("TakeRest(short, nothing to give) error = %v", err)
	}
	check("TakeRest(short, nothing to give)", out, "", before)
}
