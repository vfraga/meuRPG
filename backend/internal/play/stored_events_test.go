package play

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/goldentest"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Stored events must keep decoding as the code changes. A session event is written once and read for
// years: the history, the highlights, the undo and the session summary read rows that an older version
// of the code wrote. These tests keep one payload for each shape the code has written, under
// testdata/events/<kind>/, and decode each with today's readers. They need no database.
//
//	today.json, today-<what>.json  the payload as the real writer's type writes it now, every field set.
//	                               Written by `go test ./internal/play -run TestTodaysPayloads -update`;
//	                               a change to the writer's type shows up as a diff to review.
//	external-<what>.json           the payload of a writer in another module (progression, maps,
//	                               characters), copied by hand from its code: play cannot build it.
//	v<n>-<what>.json               an older shape (a field added or renamed since). Never rewritten.
//	                               docs: testdata/events/README.md says where each came from.
//
// A fixture that no longer decodes fails with its kind and file. A key of a fixture that today's type no
// longer reads fails too (the field was renamed or dropped: json.Unmarshal would lose it without a
// word), unless retiredKeys says why that is fine.

const storedEventsDir = "testdata/events"

// storedShape is one shape of a kind's payload that the code writes today: the file that holds it and
// the type the writer marshals.
type storedShape struct {
	file string
	typ  reflect.Type
}

func storedShapeOf[T any](file string) storedShape { return storedShape{file, reflect.TypeFor[T]()} }

// actionEventKinds are the kinds whose writer marshals an actionEvent: every event of a combat, and the
// lines of traps, creatures, lasting effects and Wild Shape.
var actionEventKinds = []string{
	"encounter_started", "initiative_submitted", "initiative_order_set", "combat_begun", "turn_ended", "combatant_moved",
	"combatant_hidden_set", "combatants_added", "combatant_removed", "encounter_ended", "attack_rolled", "damage_rolled",
	"damage_applied", "damage_discarded", "action_taken", "hit_points_adjusted", "action_undone", "spell_cast", "reaction_used",
	"reaction_declined", "death_save_rolled", "death_confirmed", "conditions_set", "turn_part_ended", "trap_searched",
	"trap_triggered", "cover_set", "side_set", "opportunity_offered", "creature_summoned", "creature_dismissed",
	"wild_shape_started", "wild_shape_ended", "familiar_sight", "door_opened", "hidden_reveal_answered", "combat_effect_ended",
	"roll_mode_requested", "roll_mode_answered", "damage_part_removed", "state_changed", "reaction_answered",
	"concentration_save_rolled", "character_revived", "lasting_effect_added", "lasting_effect_changed", "lasting_effect_ended",
	"lasting_effect_visibility_changed", "lasting_effect_saved", "lasting_effect_triggered", "spell_zone_triggered", "spell_zone_moved", "exhaustion_changed",
	"contest_started", "contest_resolved", "contest_deferred", "contest_closed", "shove_resolved", "grapple_released",
	"hide_attempted", "hide_resolved", "hide_ended", "help_given", "help_cleared", "surprise_set", "hit_rider", "legendary_action_spent", "recharge_rolled", "monster_reaction_mode_set",
}

// otherWriters are the kinds a play writer marshals as something else than an actionEvent, and the second
// shape of the kinds that two writers write differently.
var otherWriters = map[string][]storedShape{
	"spell_cast_outside":        {storedShapeOf[castEvent]("today.json")},
	"spell_cast_started":        {storedShapeOf[castEvent]("today.json")},
	"spell_cast_finished":       {storedShapeOf[castEvent]("today.json")},
	"spell_cast_interrupted":    {storedShapeOf[castEvent]("today.json")},
	"spell_cast_ended":          {storedShapeOf[castEvent]("today.json")},
	"group_check_requested":     {storedShapeOf[groupCheckEvent]("today.json")},
	"group_check_rolled":        {storedShapeOf[groupCheckEvent]("today.json")},
	"group_check_closed":        {storedShapeOf[groupCheckEvent]("today.json")},
	"scene_opened":              {storedShapeOf[sceneEvent]("today.json")},
	"scene_closed":              {storedShapeOf[sceneEvent]("today.json")},
	"scene_check_rolled":        {storedShapeOf[sceneRollEvent]("today.json")},
	"scene_attempt_granted":     {storedShapeOf[sceneGrantEvent]("today.json")},
	"stage_changed":             {storedShapeOf[stageEvent]("today.json")},
	"puzzle_shown":              {storedShapeOf[puzzleEvent]("today.json")},
	"puzzle_solved":             {storedShapeOf[puzzleEvent]("today.json")},
	"puzzle_reset":              {storedShapeOf[puzzleEvent]("today.json")},
	"puzzle_closed":             {storedShapeOf[puzzleEvent]("today.json")},
	"rest_taken":                {storedShapeOf[restEvent]("today.json")},
	"hit_dice_spent":            {storedShapeOf[hitDieEvent]("today.json")},
	"arcane_recovery_used":      {storedShapeOf[slotRecoveryEvent]("today.json")},
	"natural_recovery_used":     {storedShapeOf[slotRecoveryEvent]("today.json")},
	"character_vitals_adjusted": {storedShapeOf[vitalsEvent]("today.json")},
	"inspiration_given":         {storedShapeOf[inspirationGivenEvent]("today.json")},
	"game_time_advanced":        {storedShapeOf[gameTimeEvent]("today.json")},
	"free_roll_rolled":          {storedShapeOf[freeRollEvent]("today.json")},
}

// vitalsEvent is the payload vitalsPayload writes, named so that a test can reflect on it.
type vitalsEvent struct {
	Before vitalsNumbers `json:"before"`
	After  vitalsNumbers `json:"after"`
}

// extraShapes are the files of a kind that the same writer type writes in a second way.
var extraShapes = map[string][]storedShape{
	"lasting_effect_added": {storedShapeOf[charEffectEvent]("today-character.json")}, // out of a combat: lasting_effects_charadd.go
	"lasting_effect_ended": {storedShapeOf[gameTimeEvent]("today-character.json")},   // out of a combat: lasting_effects_chars.go
}

// externalKinds are the kinds that only another module writes. Their payloads are copied by hand.
var externalKinds = []string{
	"xp_awarded", "xp_award_undone", "milestone_marked", // progression
	"clue_revealed", "trap_revealed", "trap_noticed", "trap_disarmed", "treasure_found", "treasure_unfound", // maps
	"character_choices_completed", // characters
}

// writersOf lists the shapes the code writes today for a kind, and false for a kind nobody classified.
func writersOf(kind string) ([]storedShape, bool) {
	var shapes []storedShape
	switch {
	case slices.Contains(actionEventKinds, kind):
		shapes = append(shapes, storedShapeOf[actionEvent]("today.json"))
	case otherWriters[kind] != nil:
		shapes = append(shapes, otherWriters[kind]...)
	case slices.Contains(externalKinds, kind):
		return nil, true
	default:
		return nil, false
	}
	return append(shapes, extraShapes[kind]...), true
}

// retiredKeys are the keys of an old fixture that today's type no longer reads on purpose, by
// "<kind>/<file>: <path>", with why that is fine (the reader never needed it, or it had a default).
var retiredKeys = map[string]string{}

// eventKindsOfTheMigrations are the kinds of session_event_kinds, read from the migrations' INSERTs
// (the Up part only): the table the database enforces with a foreign key.
func eventKindsOfTheMigrations(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found (%v)", err)
	}
	insert := regexp.MustCompile(`(?s)INSERT INTO session_event_kinds\s*\(kind\)\s*VALUES(.*?)(?:ON CONFLICT|;)`)
	literal := regexp.MustCompile(`'([a-z_]+)'`)
	var kinds []string
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(body), "-- +goose Down")
		for _, m := range insert.FindAllStringSubmatch(up, -1) {
			for _, k := range literal.FindAllStringSubmatch(m[1], -1) {
				if !slices.Contains(kinds, k[1]) {
					kinds = append(kinds, k[1])
				}
			}
		}
	}
	sort.Strings(kinds)
	return kinds
}

// storedFiles lists the fixtures of a kind, by file name.
func storedFiles(t *testing.T, kind string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(storedEventsDir, kind))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestEveryEventKindHasAStoredPayload(t *testing.T) {
	t.Parallel()
	kinds := eventKindsOfTheMigrations(t)
	if len(kinds) < 90 {
		t.Fatalf("read %d kinds from the migrations, want about 97: the INSERT pattern changed", len(kinds))
	}
	for _, kind := range kinds {
		if _, ok := writersOf(kind); !ok {
			t.Errorf("kind %s: no writer is classified for it. Add it to actionEventKinds, otherWriters or externalKinds in stored_events_test.go, then write its fixtures (-update)", kind)
			continue
		}
		files := storedFiles(t, kind)
		if shapes, _ := writersOf(kind); len(shapes) == 0 {
			if !slices.ContainsFunc(files, func(f string) bool { return strings.HasPrefix(f, "external-") }) {
				t.Errorf("kind %s: no external-*.json in %s/%s: copy the payload its writer in another module marshals", kind, storedEventsDir, kind)
			}
		} else if !slices.Contains(files, "today.json") {
			t.Errorf("kind %s: %s/%s/today.json is missing. Run `go test ./internal/play -run TestTodaysPayloads -update`", kind, storedEventsDir, kind)
		}
	}
	dirs, _ := os.ReadDir(storedEventsDir)
	for _, d := range dirs {
		if d.IsDir() && !slices.Contains(kinds, d.Name()) {
			t.Errorf("%s/%s holds payloads of a kind that session_event_kinds does not have", storedEventsDir, d.Name())
		}
	}
}

// TestTodaysPayloadsAreWhatTheWritersWrite keeps the today files equal to the writers' types, every field
// set. When a field is added, renamed or retyped, the diff is the review of what is stored from now on.
func TestTodaysPayloadsAreWhatTheWritersWrite(t *testing.T) {
	t.Parallel()
	for _, kind := range eventKindsOfTheMigrations(t) {
		shapes, _ := writersOf(kind)
		for _, s := range shapes {
			t.Run(kind+"/"+s.file, func(t *testing.T) {
				t.Parallel()
				body, err := todaysPayload(kind, s)
				if err != nil {
					t.Fatalf("kind %s: build the payload: %v", kind, err)
				}
				goldentest.Compare(t, filepath.Join(storedEventsDir, kind, s.file), body)
			})
		}
	}
}

// todaysPayload is what the writer's type marshals with every field set.
func todaysPayload(kind string, s storedShape) ([]byte, error) {
	var raw []byte
	var err error
	if kind == "character_vitals_adjusted" {
		raw, err = vitalsPayload(storedVitals(1), storedVitals(2)) // the real writer's function
	} else {
		raw, err = json.Marshal(populatedPayload(s.typ))
	}
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func storedVitals(n int32) *playv1.CharacterVitals {
	return &playv1.CharacterVitals{
		HitPointsCurrent: n + 10, HitPointsTemporary: n,
		SpellSlots:  []*playv1.SpellSlotUsage{{Level: 1, Used: n}, {Level: 2, Used: n}},
		PactSlots:   &playv1.PactSlotUsage{Used: n},
		Resources:   []*playv1.ResourceUsage{{Key: "resource:rage", Used: n}},
		WildShape:   &playv1.WildShapeState{BeastKey: "monster:wolf", HitPointsCurrent: n},
		HitDiceUsed: n,
	}
}

// TestStoredPayloadsDecode decodes every fixture with the readers of today: the undo's walk, the combat
// log (the history), the highlights and the session summary. A payload that fails names its kind and file.
func TestStoredPayloadsDecode(t *testing.T) {
	t.Parallel()
	for _, kind := range eventKindsOfTheMigrations(t) {
		shapes, _ := writersOf(kind)
		for _, file := range storedFiles(t, kind) {
			raw, err := os.ReadFile(filepath.Join(storedEventsDir, kind, file))
			if err != nil {
				t.Fatal(err)
			}
			t.Run(kind+"/"+file, func(t *testing.T) {
				t.Parallel()
				for _, problem := range readStored(kind, file, raw, shapes) {
					t.Errorf("kind %s, file %s: %s", kind, file, problem)
				}
			})
		}
	}
}

// readStored runs the readers on one payload and returns what failed.
func readStored(kind, file string, raw []byte, shapes []storedShape) (problems []string) {
	try := func(reader string, f func() error) {
		defer func() {
			if r := recover(); r != nil {
				problems = append(problems, fmt.Sprintf("%s panicked: %v", reader, r))
			}
		}()
		if err := f(); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", reader, err))
		}
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	try("the payload is not JSON", func() error {
		if !json.Valid(raw) {
			return fmt.Errorf("invalid JSON")
		}
		return nil
	})
	// The readers of a combat (the undo's walk, the log, the highlights) read the events of an
	// encounter, which are actionEvents: for those a payload that does not decode is a bug. The events
	// of other writers pass through the same walk and are skipped when they are not one.
	strict := slices.Contains(actionEventKinds, kind) && !strings.HasPrefix(file, "external-") && file != "today-character.json"
	try("the undo", func() error { // the walk reads every recent event of the session, of any kind
		if _, err := readEvent(raw); err != nil && strict {
			return err
		}
		lastAction([]playdb.ListRecentSessionEventsRow{{ID: "e1", Seq: 1, Kind: kind, Payload: raw}}, "enc")
		markUndone(map[string]bool{}, raw)
		return nil
	})
	try("the history", func() error {
		buildLog([]playdb.ListEncounterEventsRow{{ID: "e1", Seq: 1, Kind: kind, Payload: raw, CreatedAt: at}})
		return nil
	})
	try("the highlights", func() error {
		var ev actionEvent
		if err := json.Unmarshal(raw, &ev); err != nil && strict {
			return err
		}
		tallyHighlights([]playdb.ListEncounterCombatEventsRow{{ID: "e1", Kind: kind, Payload: raw}}, nil)
		return nil
	})
	try("the session summary", func() error { return readForSummary(kind, file, raw, at) })
	if len(shapes) > 0 && !strings.HasPrefix(file, "external-") { // another writer's payload is read by its own module
		try("its own writer's type", func() error { return readWithWriters(kind, file, raw, shapes) })
	}
	return problems
}

// readForSummary runs the readers the summary has outside SQL (the trap activity) and checks the JSON
// paths its SQL queries read (sqlPayloadPaths) in the payloads of today.
func readForSummary(kind, file string, raw []byte, at time.Time) error {
	if kind == eventTrapTriggered { // a firing that extends another is merged into it: read it as a first one
		var ev actionEvent
		if json.Unmarshal(raw, &ev) == nil && ev.Trap != nil {
			ev.Trap.ExtendsID = ""
			raw, _ = json.Marshal(ev)
		}
	}
	line := activityLineOf(playdb.ListTrapEventsOfSessionRow{ID: "e1", Kind: kind, Payload: raw, CreatedAt: at}, map[string]*activityLine{})
	if file == "today.json" && line == nil && (kind == eventTrapNoticed || kind == eventTrapSearched || kind == eventTrapTriggered) {
		return fmt.Errorf("the trap activity reads no line from today's payload")
	}
	if !strings.HasPrefix(file, "today") {
		return nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	for _, p := range sqlPayloadPaths[kind] {
		v, ok := body[p.key]
		if !ok {
			return fmt.Errorf("the SQL of %s reads payload->>'%s', which the payload does not have", p.query, p.key)
		}
		if got := payloadJSONType(v); got != p.typ {
			return fmt.Errorf("the SQL of %s reads payload->>'%s' as a %s, the payload has a %s", p.query, p.key, p.typ, got)
		}
	}
	return nil
}

func payloadJSONType(raw json.RawMessage) string {
	switch strings.TrimSpace(string(raw))[0] {
	case '"':
		return "string"
	case 't', 'f':
		return "bool"
	case '{':
		return "object"
	case '[':
		return "array"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// sqlPayloadPaths are the keys the SQL queries of the summary and the rests read inside a payload
// (queries.sql: payload->>'key'), by kind. TestEverySQLPayloadPathIsListed keeps this list equal to the queries.
var sqlPayloadPaths = map[string][]sqlPath{
	"scene_check_rolled":    {{"CountSceneRolls", "action_id", "string"}, {"TallySessionSceneChecks", "passed", "bool"}, {"TallySessionSceneChecks", "dc_shown", "bool"}, {"TallySessionSceneChecks", "audience", "array"}},
	"scene_attempt_granted": {{"ListSceneAttemptGrantEvents", "action_id", "string"}},
	"rest_taken":            {{"HasLongRestInSession", "kind", "string"}, {"GetLatestRestKindInSession", "kind", "string"}},
	"hit_dice_spent":        {{"HasSongOfRestSinceLatestRest", "song_sides", "number"}},
	"free_roll_rolled":      {{"ListFreeRollEvents", "secret", "bool"}},
}

type sqlPath struct{ query, key, typ string }

// TestEverySQLPayloadPathIsListed fails when a query starts to read another key of a payload: the new key
// must be in sqlPayloadPaths, so that the fixtures keep it.
func TestEverySQLPayloadPathIsListed(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	read := regexp.MustCompile(`payload\s*(?:->>|\?)\s*'([a-z_0-9]+)'`)
	known := map[string]bool{}
	for _, paths := range sqlPayloadPaths {
		for _, p := range paths {
			known[p.key] = true
		}
	}
	for _, m := range read.FindAllStringSubmatch(string(body), -1) {
		if !known[m[1]] {
			t.Errorf("queries.sql reads payload key %q, which sqlPayloadPaths does not list: add it with its kind, so that today's fixture is checked for it", m[1])
		}
	}
}

// readWithWriters decodes the payload with the type of each shape the kind has and, for a fixture that
// is not a hand-copied external one, checks that every key it holds is still read by one of them.
func readWithWriters(kind, file string, raw []byte, shapes []storedShape) error {
	var failures []string
	for _, s := range shapes {
		if err := json.Unmarshal(raw, reflect.New(s.typ).Interface()); err != nil {
			failures = append(failures, fmt.Sprintf("%s does not decode: %v", s.typ.Name(), err))
		}
	}
	if len(failures) == len(shapes) {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	if strings.HasPrefix(file, "external-") {
		return nil
	}
	var lost []string
	for _, s := range shapes {
		if s.file != file && strings.HasPrefix(file, "today") {
			continue // another shape of the kind's today files
		}
		lost = unreadPayloadKeys(raw, s.typ, "")
		if len(lost) == 0 {
			break
		}
	}
	lost = slices.DeleteFunc(lost, func(path string) bool { _, ok := retiredKeys[kind+"/"+file+": "+path]; return ok })
	if len(lost) > 0 {
		return fmt.Errorf("the key(s) %s are no longer read by %s: a field was renamed or dropped, and json.Unmarshal would lose its value in silence (add it to retiredKeys with the reason if that is intended)", strings.Join(lost, ", "), shapesNames(shapes))
	}
	return nil
}

func shapesNames(shapes []storedShape) string {
	var names []string
	for _, s := range shapes {
		names = append(names, s.typ.Name())
	}
	return strings.Join(names, "/")
}

// unreadPayloadKeys lists the keys (as paths) of the JSON that the Go type has no field for.
func unreadPayloadKeys(raw []byte, t reflect.Type, path string) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		if t.PkgPath() == "time" {
			return nil
		}
		var body map[string]json.RawMessage
		if json.Unmarshal(raw, &body) != nil {
			return nil
		}
		fields := storedJSONFields(t)
		var out []string
		for key, value := range body {
			f, ok := fields[key]
			if !ok {
				out = append(out, path+key)
				continue
			}
			out = append(out, unreadPayloadKeys(value, f, path+key+".")...)
		}
		sort.Strings(out)
		return out
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return nil
		}
		var out []string
		for _, item := range items {
			out = append(out, unreadPayloadKeys(item, t.Elem(), path)...)
		}
		return dedupePaths(out)
	case reflect.Map:
		var items map[string]json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return nil
		}
		var out []string
		for _, item := range items {
			out = append(out, unreadPayloadKeys(item, t.Elem(), path)...)
		}
		return dedupePaths(out)
	}
	return nil
}

func dedupePaths(in []string) []string {
	sort.Strings(in)
	return slices.Compact(in)
}

// storedJSONFields maps the JSON names of a struct to the types of its fields, embedded structs included.
func storedJSONFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				for k, v := range storedJSONFields(ft) {
					out[k] = v
				}
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = f.Type
	}
	return out
}

// populated is a value of the type with every field set, so that json.Marshal writes every key.
func populatedPayload(t reflect.Type) any {
	v := reflect.New(t).Elem()
	fillPayload(v, map[reflect.Type]int{})
	return v.Interface()
}

var (
	rawMessageType = reflect.TypeFor[json.RawMessage]()
	timeType       = reflect.TypeFor[time.Time]()
	marshalerType  = reflect.TypeFor[json.Marshaler]()
)

func fillPayload(v reflect.Value, seen map[reflect.Type]int) {
	t := v.Type()
	switch {
	case t == rawMessageType:
		v.Set(reflect.ValueOf(json.RawMessage(`{}`)))
		return
	case t == timeType:
		v.Set(reflect.ValueOf(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
		return
	case t.Implements(marshalerType) || reflect.PointerTo(t).Implements(marshalerType):
		return
	}
	switch t.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	case reflect.Pointer:
		if seen[t.Elem()] >= 2 {
			return // a type that holds itself: one level is enough
		}
		v.Set(reflect.New(t.Elem()))
		fillPayload(v.Elem(), seen)
	case reflect.Struct:
		seen[t]++
		defer func() { seen[t]-- }()
		for i := range t.NumField() {
			if f := t.Field(i); f.IsExported() && f.Tag.Get("json") != "-" {
				fillPayload(v.Field(i), seen)
			}
		}
	case reflect.Slice:
		if seen[t.Elem()] >= 2 {
			return
		}
		s := reflect.MakeSlice(t, 1, 1)
		fillPayload(s.Index(0), seen)
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(t)
		k := reflect.New(t.Key()).Elem()
		fillPayload(k, seen)
		e := reflect.New(t.Elem()).Elem()
		fillPayload(e, seen)
		m.SetMapIndex(k, e)
		v.Set(m)
	}
}

// TestStoredMovesKeepTheirUndoVerdict: a move written before the undo knew moves says where it came from
// nowhere, so the undo closes the chain there; a move of today, built by the writer's type, is the action
// to take back.
func TestStoredMovesKeepTheirUndoVerdict(t *testing.T) {
	t.Parallel()
	enc := "enc"
	old, err := os.ReadFile(filepath.Join(storedEventsDir, "combatant_moved", "v1-whole-feet.json"))
	if err != nil {
		t.Fatal(err)
	}
	now, err := json.Marshal(actionEvent{Round: 1, Actor: "a", Col: 4, Row: 6, CostFt: 10, CostDFt: 100, From: &moveState{Placed: true, Col: 3, Row: 6}})
	if err != nil {
		t.Fatal(err)
	}
	last := func(payload []byte) bool {
		_, ok := lastAction([]playdb.ListRecentSessionEventsRow{{ID: "e1", Seq: 1, Kind: eventCombatantMoved, EncounterID: &enc, Payload: payload}}, enc)
		return ok
	}
	if last(old) {
		t.Error("an old move (v1-whole-feet.json) can be undone: it knows nothing of where it came from")
	}
	if !last(now) {
		t.Error("a move of today cannot be undone")
	}
}
