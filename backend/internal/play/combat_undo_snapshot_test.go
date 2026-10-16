package play

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// The undo property (the master's Desfazer, takeBack in combat_undo.go): take a
// picture of everything the master and the players read and of the rows the combat
// keeps, do the action, undo it, take the picture again. A put-back that is exact
// leaves no difference. Where today's undo does not restore something exactly, the
// scenario names the difference it leaves (undoDelta), so that the code can be
// reorganized without moving it: the test fails if the difference grows, shrinks or
// changes.

// undoTables are the tables an action of the combat writes and an undo puts back:
// the combatants and their states, the pending damages, the windows and offers, the
// casts, the hiding, the characters' vitals and the map's layers and points.
var undoTables = []string{
	"encounters", "combatants", "combatant_states", "pending_damages", "reaction_windows", "reaction_holds",
	"opportunity_offers", "hidden_reveals", "spell_casts", "combat_hiding", "combat_hide_attempts", "combat_helps",
	"combat_contests", "combat_holds", "roll_holds", "hit_riders", "combat_surprised", "inspiration_holds",
	"trap_damages", "character_vitals", "character_creatures", "character_effects", "character_wild_shapes",
	"character_inspiration", "characters", "map_layers", "map_points", "map_tokens", "map_creature_tokens",
}

// volatileKey is a field no undo can put back and no player reads as state: a
// timestamp, or the counter of a correction (which goes up on every change).
var volatileKey = regexp.MustCompile(`(_at|[a-z]At|^at|^revision|^touched)$`)

var uuidText = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// undoWatch takes the pictures of one test's combat. Its IDs are the same in every
// picture, so that a row put back with another ID shows as a different row.
type undoWatch struct {
	a   *armed
	ids map[string]string
}

// newUndoWatch starts watching a fight. Every character's vitals get a row of their
// own first (a correction to the hit points they have), because the first spell or
// damage makes one and a character with no row reads the same as one with a full one.
func newUndoWatch(t *testing.T, a *armed) *undoWatch {
	t.Helper()
	for _, c := range []*charactersv1.Character{a.toren, a.pens, a.bri} {
		if c != nil {
			a.correct(t, c, hpIs(a.vitals(t, c).GetHitPointsCurrent()))
		}
	}
	return &undoWatch{a: a, ids: map[string]string{}}
}

// name is a short, stable name for a UUID: the order it was first seen in.
func (w *undoWatch) name(id string) string {
	if n, ok := w.ids[id]; ok {
		return n
	}
	n := fmt.Sprintf("#%d", len(w.ids)+1)
	w.ids[id] = n
	return n
}

func (w *undoWatch) mask(s string) string {
	return uuidText.ReplaceAllStringFunc(s, w.name)
}

// anonymous writes every UUID the same, to put the rows in an order that does not
// depend on the IDs the database drew.
func anonymous(s string) string {
	return uuidText.ReplaceAllString(s, "<id>")
}

// flatten writes a decoded JSON value as one "path=value" line per leaf.
func (w *undoWatch) flatten(prefix string, v any, out *[]string) {
	w.flattenWith(w.mask, prefix, v, out)
}

// flattenWith is flatten with the way a UUID is written.
func (w *undoWatch) flattenWith(mask func(string) string, prefix string, v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			if !volatileKey.MatchString(k) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			w.flattenWith(mask, prefix+"."+k, x[k], out)
		}
		if len(keys) == 0 {
			*out = append(*out, prefix+"={}")
		}
	case []any:
		for i, e := range x {
			w.flattenWith(mask, fmt.Sprintf("%s[%d]", prefix, i), e, out)
		}
		if len(x) == 0 {
			*out = append(*out, prefix+"=[]")
		}
	case string:
		*out = append(*out, prefix+"="+mask(x))
	default:
		*out = append(*out, fmt.Sprintf("%s=%v", prefix, x))
	}
}

// protoLines is a message as the lines of its JSON.
func (w *undoWatch) protoLines(t *testing.T, section string, m proto.Message) []string {
	t.Helper()
	raw, err := protojson.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal(%s) error = %v", section, err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", section, err)
	}
	var out []string
	w.flatten(section, v, &out)
	return out
}

// rowLines reads a table, every column, as the lines of its rows. A row with an id
// is named by it; one without (a join table) by its whole content. The rows are read
// in the order of their content, so that the names the IDs get do not depend on the
// order the database answered in.
func (w *undoWatch) rowLines(t *testing.T, table string) []string {
	t.Helper()
	rows, err := w.a.h.pool.Query(t.Context(), "SELECT row_to_json(t)::text FROM "+table+" t")
	if err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	defer rows.Close()
	type read struct {
		row  map[string]any
		sort string
	}
	var all []read
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(raw), &row); err != nil {
			t.Fatalf("decode %s: %v", table, err)
		}
		var lines []string
		w.flattenWith(anonymous, "", row, &lines)
		all = append(all, read{row: row, sort: strings.Join(lines, ",")})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows %s: %v", table, err)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].sort < all[j].sort })
	var out []string
	for _, r := range all {
		key := table
		switch id, _ := r.row["id"].(string); {
		case id != "":
			key += "[" + w.name(id) + "]"
			delete(r.row, "id")
		case table == "character_vitals":
			// One row for each character, made when a spell or a resource first spends
			// something. A row of zeros says what no row says: it is no state.
			owner, _ := r.row["character_id"].(string)
			delete(r.row, "character_id")
			if emptyRow(r.row) {
				continue
			}
			key += "[" + w.name(owner) + "]"
		default:
			key += "[" + anonymous(r.sort) + "]"
			r.row = map[string]any{}
		}
		var lines []string
		w.flatten("db/"+key, r.row, &lines)
		out = append(out, lines...)
	}
	return out
}

// emptyRow says a row holds nothing but zeros, empty lists and NULLs.
func emptyRow(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case map[string]any:
		for k, e := range x {
			if !volatileKey.MatchString(k) && !emptyRow(e) {
				return false
			}
		}
		return true
	case []any:
		return !slices.ContainsFunc(x, func(e any) bool { return !emptyRow(e) })
	case string:
		return x == ""
	case bool:
		return !x
	case float64:
		return x == 0
	}
	return false
}

// logLines is a combat log as one line for each entry: what it is, who did it to
// whom, and whether the master can undo it. The entries are made of events that
// never change, so their detail is not what an undo can get wrong.
func (w *undoWatch) logLines(who string, l *playv1.ListCombatLogResponse) []string {
	out := []string{who + "/log.undoable=" + w.mask(l.GetUndoableEventId())}
	for _, r := range l.GetRounds() {
		for _, e := range r.GetEntries() {
			kind := strings.TrimPrefix(e.GetKind().String(), "COMBAT_LOG_KIND_")
			text := fmt.Sprintf("%s/log R%d %s id=%s", who, r.GetRound(), kind, w.name(e.GetId()))
			if e.GetActorLabel() != "" || e.GetTargetLabel() != "" {
				text += fmt.Sprintf(" %s>%s", e.GetActorLabel(), e.GetTargetLabel())
			}
			if e.GetKey() != "" {
				text += " key=" + e.GetKey()
			}
			if e.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_UNSPECIFIED {
				text += " outcome=" + e.GetOutcome().String()
			}
			if e.GetUndoable() {
				text += " undoable"
			}
			if e.GetHidden() {
				text += " hidden"
			}
			if d := e.GetDamage(); d != nil {
				text += fmt.Sprintf(" damage=%d/%v/%v", d.GetAmount(), d.GetStatus(), d.GetTargetDefeated())
			}
			if f := e.GetEffect(); f != nil {
				text += fmt.Sprintf(" effect=%v/%s", f.GetChange(), f.GetReason())
			}
			out = append(out, text)
		}
	}
	return out
}

// picture is everything the master and the three players read, and the rows.
func (w *undoWatch) picture(t *testing.T) []string {
	t.Helper()
	a := w.a
	var out []string
	enc := a.get(t, a.master)
	out = append(out, w.protoLines(t, "master/combat", enc)...)
	out = append(out, w.logLines("master", a.log(t, a.master, enc))...)
	for _, p := range []struct {
		name string
		u    *user
	}{{"caio", a.caio}, {"ana", a.ana}, {"bia", a.bia}} {
		out = append(out, w.protoLines(t, p.name+"/combat", a.get(t, p.u))...)
		out = append(out, w.logLines(p.name, a.log(t, p.u, enc))...)
	}
	for _, c := range []*charactersv1.Character{a.toren, a.pens, a.bri} {
		if c != nil {
			out = append(out, w.protoLines(t, "vitals/"+c.GetName(), a.vitals(t, c))...)
		}
	}
	for _, table := range undoTables {
		out = append(out, w.rowLines(t, table)...)
	}
	return out
}

// difference lists what is only in one of two pictures: "- line" for the first,
// "+ line" for the second.
func difference(before, after []string) []string {
	count := map[string]int{}
	for _, l := range before {
		count[l]++
	}
	for _, l := range after {
		count[l]--
	}
	var out []string
	for l, n := range count {
		for ; n > 0; n-- {
			out = append(out, "- "+l)
		}
		for ; n < 0; n++ {
			out = append(out, "+ "+l)
		}
	}
	sort.Strings(out)
	return out
}

var idName = regexp.MustCompile(`#[0-9]+`)

// unnumbered writes the names of the IDs without their numbers, which depend on the
// order the picture met them in: a delta says "a row", not which one.
func unnumbered(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = idName.ReplaceAllString(l, "#")
	}
	sort.Strings(out)
	return out
}

// undoDelta is what an undo leaves different from before the action, in
// difference's form; nil is an exact put-back.
type undoDelta []string

// run does the action, checks it changed something the master can read, takes it
// back with the master's Desfazer and requires the difference from before to be
// exactly want.
func (w *undoWatch) run(t *testing.T, name string, want undoDelta, do func()) {
	t.Helper()
	a := w.a
	before := w.picture(t)
	do()
	e := a.get(t, a.master)
	id := a.log(t, a.master, e).GetUndoableEventId()
	if id == "" {
		t.Fatalf("%s: nothing to undo after it", name)
	}
	if len(difference(before, w.picture(t))) == 0 {
		t.Fatalf("%s changed nothing", name)
	}
	if err := a.undo(t, a.master, e, id); err != nil {
		t.Fatalf("%s: UndoLastAction() error = %v", name, err)
	}
	got := difference(before, w.picture(t))
	wantSorted := slices.Clone([]string(want))
	sort.Strings(wantSorted)
	if !slices.Equal(unnumbered(got), unnumbered(wantSorted)) {
		t.Errorf("%s: after the undo the combat differs from before the action by\n%s\nwant\n%s", name, strings.Join(got, "\n"), strings.Join(wantSorted, "\n"))
	}
}
