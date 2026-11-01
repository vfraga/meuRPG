package play

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The characterization of the master's calls on the effects that last (list, change the
// duration, the visibility, end, take a target off), of the card a viewer reads of them and of the
// way the effects move between a combat and game time. A scenario's text is a file in
// testdata/lasting_rpc with the ids replaced by labels; a change here is a change of behavior, not
// of structure. Run with -update-lasting to write the files again after a change that was
// meant, and read the diff.

var updateLasting = flag.Bool("update-lasting", false, "write the golden files of the lasting effects (testdata/lasting_core and testdata/lasting_rpc) again")

var (
	rpcTime = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z`)
)

// wantLastingGolden compares got with the golden file of the scenario in testdata/<dir>.
func wantLastingGolden(t *testing.T, dir, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", dir, name+".golden")
	if *updateLasting {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the golden file: %v (write it with -update-lasting)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from its golden file (-update-lasting writes it again):\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

// rpcMemory keeps the label of every id the tests saw, by harness: a row that is gone keeps the
// name it had.
var rpcMemory sync.Map

// rpcRemember adds the labels to the memory of the harness and returns it; a label is kept as
// first made.
func (a *armed) rpcRemember(labels map[string]string) map[string]string {
	mem, _ := rpcMemory.LoadOrStore(a.h, &rpcSeen{labels: map[string]string{}})
	seen := mem.(*rpcSeen)
	seen.mu.Lock()
	defer seen.mu.Unlock()
	for id, l := range labels {
		if _, ok := seen.labels[id]; !ok {
			seen.labels[id] = l
		}
	}
	out := make(map[string]string, len(seen.labels))
	for id, l := range seen.labels {
		out[id] = l
	}
	return out
}

type rpcSeen struct {
	mu     sync.Mutex
	labels map[string]string
}

// rpcLabels are the names that stand for the ids in a text: the combatants by label, the effect
// rows by the key and the targets of their casting (the rows of one casting are made at one
// instant, so which of them has the lowest id is random), the users and the characters.
func (a *armed) rpcLabels(t *testing.T) map[string]string {
	t.Helper()
	labels := map[string]string{}
	rows, err := a.h.pool.Query(t.Context(), `SELECT id::TEXT, label FROM combatants`)
	if err != nil {
		t.Fatalf("read the combatants: %v", err)
	}
	for rows.Next() {
		var id, label string
		if err := rows.Scan(&id, &label); err != nil {
			t.Fatalf("scan a combatant: %v", err)
		}
		labels[id] = "<" + label + ">"
	}
	rows.Close() // the pool has one connection
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	states, err := a.h.pool.Query(t.Context(), `SELECT s.id::TEXT, s.source_key, COALESCE(s.group_id::TEXT, ''), c.label FROM combatant_states s JOIN combatants c ON c.id = s.combatant_id WHERE s.kind = 'effect'`)
	if err != nil {
		t.Fatalf("read the effects: %v", err)
	}
	type fxRow struct{ id, key, group, label string }
	var fx []fxRow
	for states.Next() {
		var r fxRow
		if err := states.Scan(&r.id, &r.key, &r.group, &r.label); err != nil {
			t.Fatalf("scan an effect: %v", err)
		}
		fx = append(fx, r)
	}
	states.Close()
	if err := states.Err(); err != nil {
		t.Fatal(err)
	}
	for _, r := range fx {
		var who []string
		for _, o := range fx {
			if o.group == r.group && o.key == r.key {
				who = append(who, o.label)
			}
		}
		slices.Sort(who)
		labels[r.id] = "<fx:" + r.key + "@" + strings.Join(who, "+") + ">"
		if r.group != "" {
			labels[r.group] = "<group:" + r.key + ">"
		}
	}
	char, err := a.h.pool.Query(t.Context(), `SELECT e.id::TEXT, e.source_key, e.group_id::TEXT, e.character_id::TEXT FROM character_effects e`)
	if err != nil {
		t.Fatalf("read the characters' effects: %v", err)
	}
	type cfxRow struct{ id, key, group, char string }
	var cfx []cfxRow
	for char.Next() {
		var r cfxRow
		if err := char.Scan(&r.id, &r.key, &r.group, &r.char); err != nil {
			t.Fatalf("scan a character's effect: %v", err)
		}
		cfx = append(cfx, r)
	}
	char.Close()
	if err := char.Err(); err != nil {
		t.Fatal(err)
	}
	names := map[string]string{a.toren.GetId(): "toren", a.pens.GetId(): "pens", a.bri.GetId(): "bri", a.goblin.GetId(): "goblin", a.capitao.GetId(): "capitao"}
	for _, r := range cfx {
		var who []string
		for _, o := range cfx {
			if o.group == r.group && o.key == r.key {
				who = append(who, names[o.char])
			}
		}
		slices.Sort(who)
		labels[r.id] = "<cfx:" + r.key + "@" + strings.Join(who, "+") + ">"
		if _, ok := labels[r.group]; !ok {
			labels[r.group] = "<group:" + r.key + ">"
		}
	}
	for name, u := range map[string]*user{"master": a.master, "caio": a.caio, "ana": a.ana, "bia": a.bia} {
		labels[u.id] = "<user:" + name + ">"
	}
	for id, name := range names {
		if id != "" {
			labels[id] = "<char:" + name + ">"
		}
	}
	return a.rpcRemember(labels)
}

// rpcClean replaces the ids of a text by their labels, and any other id by <uuid>.
func (a *armed) rpcClean(t *testing.T, text string) string {
	t.Helper()
	labels := a.rpcLabels(t)
	text = rpcTime.ReplaceAllString(text, "<time>")
	return uuidText.ReplaceAllStringFunc(text, func(id string) string {
		if l, ok := labels[id]; ok {
			return l
		}
		return "<uuid>"
	})
}

// rpcSortedKeys are the lists in the answers that come in the order of random ids (the rows of one
// casting are made at one instant); they are written sorted.
var rpcSortedKeys = []string{"targetIds", "targetLabels", "effectIds", "targets"}

// rpcJSON writes a message the same way every time: the keys sorted, the lists of ids sorted, and
// the ids replaced by their labels.
func (a *armed) rpcJSON(t *testing.T, m proto.Message) string {
	t.Helper()
	raw, err := protojson.Marshal(m)
	if err != nil {
		t.Fatalf("encode a message: %v", err)
	}
	return a.rpcCanonicalText(t, raw)
}

// rpcRuleKeys are the fields of an answer that carry the rules of an effect and not the shape of
// the proto: who it is on, who may read it, whose it is, when it ends and the clock text that says
// so. rpcRuleJSON keeps only these, so a new field of the proto does not rewrite a golden.
var rpcRuleKeys = map[string]bool{
	"effect": true, "effects": true, "concentrations": true, "id": true, "sourceKey": true,
	"audience": true, "playerVisible": true, "label": true, "playerLabel": true,
	"casterId": true, "casterLabel": true, "targetLabels": true, "combatantLabel": true,
	"effectId": true, "effectIds": true, "spellKey": true, "concentration": true,
	"durationKind": true, "roundsLeft": true, "clockTextPt": true,
	"endsAt": true, "combatantId": true, "phase": true, "round": true, "currentCombatantId": true,
	"characterName": true, "durationTextPt": true, "secondsLeft": true,
}

// rpcRuleJSON writes a message like rpcJSON, with only the fields of rpcRuleKeys.
func (a *armed) rpcRuleJSON(t *testing.T, m proto.Message) string {
	t.Helper()
	raw, err := protojson.Marshal(m)
	if err != nil {
		t.Fatalf("encode a message: %v", err)
	}
	var v any
	if err := json.Unmarshal([]byte(a.rpcClean(t, string(raw))), &v); err != nil {
		t.Fatalf("read a message: %v", err)
	}
	return rpcCanonical(rpcKeepKeys(v))
}

// rpcKeepKeys drops, at every depth, the object fields that are not in rpcRuleKeys.
func rpcKeepKeys(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			if rpcRuleKeys[k] {
				out[k] = rpcKeepKeys(e)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = rpcKeepKeys(e)
		}
		return out
	}
	return v
}

// rpcCanonicalText replaces the ids of a JSON text by their labels and then writes it indented,
// with the lists of ids sorted (by label: the order of the ids themselves is random).
func (a *armed) rpcCanonicalText(t *testing.T, raw []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(a.rpcClean(t, string(raw))), &v); err != nil {
		t.Fatalf("read a message: %v", err)
	}
	return rpcCanonical(v)
}

// rpcCanonical writes a decoded JSON value indented with its lists of ids sorted by their text.
func rpcCanonical(v any) string {
	var walk func(key string, x any) any
	walk = func(key string, x any) any {
		switch x := x.(type) {
		case map[string]any:
			for k, e := range x {
				x[k] = walk(k, e)
			}
		case []any:
			for i, e := range x {
				x[i] = walk("", e)
			}
			if slices.Contains(rpcSortedKeys, key) {
				sort.SliceStable(x, func(i, j int) bool { return fmt.Sprint(x[i]) < fmt.Sprint(x[j]) })
			}
		}
		return x
	}
	out, err := json.MarshalIndent(walk("", v), "", " ")
	if err != nil {
		panic(err)
	}
	return string(out)
}

// rpcEvents prints the events of the open session of the kinds, oldest first, with their payload
// and the ids replaced.
func (a *armed) rpcEvents(t *testing.T, kinds ...string) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT kind, payload::TEXT, COALESCE(actor_user_id::TEXT, '-') FROM session_events WHERE kind = ANY($1) ORDER BY seq`, kinds)
	if err != nil {
		t.Fatalf("read the events: %v", err)
	}
	type raw struct{ kind, payload, actor string }
	var found []raw
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.kind, &r.payload, &r.actor); err != nil {
			t.Fatalf("scan an event: %v", err)
		}
		r.payload = dbtest.SortJSONKeys(r.payload)
		found = append(found, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	labels := a.rpcLabels(t)
	var lines []string
	for _, r := range found {
		var v any
		if err := json.Unmarshal([]byte(r.payload), &v); err != nil {
			t.Fatalf("read a payload: %v", err)
		}
		rpcAnyTarget(v, labels)
		plain, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(a.rpcClean(t, string(plain))), &v); err != nil {
			t.Fatal(err)
		}
		var flat strings.Builder
		enc := json.NewEncoder(&flat)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(rpcSortIDs(v)); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, r.kind+" by "+r.actor+" "+strings.TrimSpace(flat.String()))
	}
	return a.rpcClean(t, strings.Join(lines, "\n")+"\n")
}

// rpcAnyTarget makes the target a line names the same in every run when the line has several
// targets: it is the first row of the casting, in the order of random ids.
func rpcAnyTarget(v any, labels map[string]string) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	l, ok := m["lasting"].(map[string]any)
	if !ok {
		return
	}
	if ts, ok := l["targets"].([]any); ok && len(ts) > 1 && slices.Contains(ts, m["target_id"]) {
		m["target_id"] = "<one of the targets>"
	}
	// The line of an end names the first row of the casting too.
	if effs, ok := l["effects"].([]any); ok && len(effs) == 1 && (l["change"] == "ended" || l["change"] == "visibility") && l["targets"] == nil {
		if id, ok := effs[0].(string); ok && strings.Contains(labels[id], "+") {
			m["target_id"] = "<a row of the casting>"
		}
	}
}

// rpcSortIDs sorts the lists of strings in a payload, which are lists of ids made at one instant.
func rpcSortIDs(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = rpcSortIDs(e)
		}
	case []any:
		strs := make([]string, 0, len(x))
		for i, e := range x {
			x[i] = rpcSortIDs(e)
			if s, ok := e.(string); ok {
				strs = append(strs, s)
			}
		}
		if len(strs) == len(x) {
			slices.Sort(strs)
			for i, s := range strs {
				x[i] = s
			}
		}
	}
	return v
}

// rpcRows prints the effect rows of a combat, one line each.
func (a *armed) rpcRows(t *testing.T) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT c.label, s.source_key, COALESCE(s.source_kind, '-'), s.concentration, COALESCE(s.duration_kind, '-'),
			COALESCE(s.ends_round::TEXT, '-'), COALESCE(e.label, '-'), COALESCE(s.ends_phase, '-'), s.player_visible, s.audience, COALESCE(s.player_label, '-'),
			COALESCE(src.label, '-')
		FROM combatant_states s JOIN combatants c ON c.id = s.combatant_id
		LEFT JOIN combatants e ON e.id = s.ends_combatant_id LEFT JOIN combatants src ON src.id = s.source_id
		WHERE s.kind = 'effect' ORDER BY c.label, s.source_key, s.duration_kind, COALESCE(s.ends_round, 0)`)
	if err != nil {
		t.Fatalf("read the effects: %v", err)
	}
	var b strings.Builder
	for rows.Next() {
		var who, key, kind, dur, endsRound, endsWho, phase, audience, label, caster string
		var conc, visible bool
		if err := rows.Scan(&who, &key, &kind, &conc, &dur, &endsRound, &endsWho, &phase, &visible, &audience, &label, &caster); err != nil {
			t.Fatalf("scan an effect: %v", err)
		}
		fmt.Fprintf(&b, "%s %s kind=%s conc=%v dur=%s ends=r%s/%s/%s visible=%v audience=%s label=%s caster=%s\n", who, key, kind, conc, dur, endsRound, endsWho, phase, visible, audience, label, caster)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// rpcCharRows prints the effects the database keeps on the characters.
func (a *armed) rpcCharRows(t *testing.T) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT character_id::TEXT, source_key, source_kind, concentration, duration_kind, COALESCE(seconds_left::TEXT, '-'),
			player_visible, audience, COALESCE(player_label, '-'), COALESCE(source_character_id::TEXT, '-'), array_to_string(condition_keys, ',')
		FROM character_effects ORDER BY source_key, (SELECT name FROM characters WHERE characters.id = character_effects.character_id), seconds_left`)
	if err != nil {
		t.Fatalf("read the characters' effects: %v", err)
	}
	var b strings.Builder
	for rows.Next() {
		var who, key, kind, dur, left, audience, label, caster, conds string
		var conc, visible bool
		if err := rows.Scan(&who, &key, &kind, &conc, &dur, &left, &visible, &audience, &label, &caster, &conds); err != nil {
			t.Fatalf("scan a character's effect: %v", err)
		}
		fmt.Fprintf(&b, "%s %s kind=%s conc=%v dur=%s left=%s visible=%v audience=%s label=%s caster=%s conds=[%s]\n", who, key, kind, conc, dur, left, visible, audience, label, caster, conds)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return a.rpcClean(t, b.String())
}

// rpcView writes what a viewer reads of the effects on each combatant of the combat: the
// conditions, their names, the labels and the cards.
func (a *armed) rpcView(t *testing.T, u *user) string {
	t.Helper()
	e := a.get(t, u)
	type view struct {
		Label      string            `json:"label"`
		Conditions []string          `json:"conditions,omitempty"`
		Names      []string          `json:"names,omitempty"`
		Exhaustion int32             `json:"exhaustion,omitempty"`
		Labels     []string          `json:"labels,omitempty"`
		Cards      []json.RawMessage `json:"cards,omitempty"`
	}
	var out []view
	for _, c := range e.GetCombatants() {
		v := view{Label: c.GetLabel(), Conditions: c.GetConditions(), Names: c.GetConditionNamesPt(), Exhaustion: c.GetExhaustionLevel()}
		for _, l := range c.GetEffectLabels() {
			v.Labels = append(v.Labels, l.GetEffectId()+" = "+l.GetTextPt())
		}
		for _, f := range c.GetEffects() {
			raw, err := protojson.Marshal(f)
			if err != nil {
				t.Fatal(err)
			}
			v.Cards = append(v.Cards, raw)
		}
		out = append(out, v)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return a.rpcCanonicalText(t, raw)
}

// rpcAddOpts changes an AddLastingEffectRequest: who casts it, its duration, the label...
type rpcAddOpts func(*playv1.AddLastingEffectRequest)

// rpcCaster names the caster of an effect.
func (a *armed) rpcCaster(t *testing.T, label string) rpcAddOpts {
	t.Helper()
	return func(r *playv1.AddLastingEffectRequest) { r.CasterId = a.id(t, label) }
}

// rpcDuration sets the duration of an effect by kind, rounds and anchor label.
func (a *armed) rpcDuration(t *testing.T, kind playv1.EffectDurationKind, rounds int32, anchor string) rpcAddOpts {
	t.Helper()
	return func(r *playv1.AddLastingEffectRequest) {
		r.Duration = &playv1.EffectDurationChoice{Kind: kind, Rounds: rounds}
		if anchor != "" {
			r.Duration.AnchorCombatantId = a.id(t, anchor)
		}
	}
}

// rpcAdd adds an effect as the master with all the options and returns the id of the first card.
func (a *armed) rpcAdd(t *testing.T, e *playv1.Encounter, key string, targets []string, opts ...rpcAddOpts) *playv1.AddLastingEffectResponse {
	t.Helper()
	return a.mustAddEffect(t, e, key, targets, func(r *playv1.AddLastingEffectRequest) {
		for _, o := range opts {
			o(r)
		}
	})
}

// rpcEffectID is the id of the first effect of the key on the label, as the master reads it.
func (a *armed) rpcEffectID(t *testing.T, key, label string) string {
	t.Helper()
	a.rpcLabels(t)
	res, err := a.master.lasting.ListLastingEffects(t.Context(), connect.NewRequest(&playv1.ListLastingEffectsRequest{CampaignId: a.campaignID, EncounterId: a.get(t, a.master).GetId()}))
	if err != nil {
		t.Fatalf("ListLastingEffects() error = %v", err)
	}
	for _, f := range res.Msg.GetEffects() {
		if f.GetSourceKey() == key && slices.Contains(f.GetTargetLabels(), label) {
			return f.GetId()
		}
	}
	t.Fatalf("%s has no %s", label, key)
	return ""
}
