package play

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The golden of the combat log (RN-10, RN-20): every kind of event buildLog, view and
// reactionLine know, built by hand as the payload the service writes, and read by the
// master, the acting player, another player and a player of a table that hides the
// death saves. testdata/combat_log.golden holds what each of them reads, line by line,
// so that what is hidden from a viewer (the dice, the hit points after, the "hidden"
// mark, the undo) is read in the file: a change of any line is a change of behavior.
// Run with UPDATE_GOLDEN=1 to write the file again, and read the diff.
//
// These tests need no database: the log is a function of the events and the combatants.

const goldenPath = "testdata/combat_log.golden"

// logRoster stands in for the content of the campaign: the names of spells and traps.
type logRoster struct{ CombatRoster }

func (logRoster) ContentNames(context.Context, pgx.Tx, string) (func(string) string, error) {
	return logContentName, nil
}

func (logRoster) CombatSpell(_ context.Context, _ pgx.Tx, _, _, key string, _ int, _ string) (link.Spell, error) {
	return link.Spell{Key: key, Name: logContentName(key)}, nil
}

type logTraps struct{ TrapBook }

func (logTraps) TrapNames(_ context.Context, _ string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if id == "pt-pit" {
			out[id] = "Fosso"
		}
	}
	return out, nil
}

func logContentName(key string) string {
	return map[string]string{
		"spell:fireball": "Bola de Fogo", "spell:shield": "Escudo Arcano", "spell:revivify": "Reviver", "spell:bless": "Bênção",
		"wolf": "Lobo", "effect:bless": "Bênção", "ability:dex": "Destreza", "damage-type:fire": "fogo", "rage": "Fúria",
	}[key]
}

// logViewer is who reads the log.
type logViewer struct {
	name string
	v    combatViewer
}

func logViewers() []logViewer {
	return []logViewer{
		{"master", combatViewer{master: true, userID: "u-master"}},
		{"acting player (Aria)", combatViewer{userID: "u1"}},
		{"other player (Bruno)", combatViewer{userID: "u2"}},
		{"other player (Bruno), death saves hidden", combatViewer{userID: "u2", hideDeath: true}},
	}
}

func logCombatants() map[string]playdb.Combatant {
	u1, u2 := "u1", "u2"
	return map[string]playdb.Combatant{
		"pc-aria":  {ID: "pc-aria", Label: "Aria", Kind: kindPlayer, UserID: &u1, CharacterID: "char-aria"},
		"pc-bruno": {ID: "pc-bruno", Label: "Bruno", Kind: kindPlayer, UserID: &u2, CharacterID: "char-bruno"},
		"npc-gob":  {ID: "npc-gob", Label: "Goblin 1", Kind: kindNPC},
		"npc-gob2": {ID: "npc-gob2", Label: "Goblin 2", Kind: kindNPC},
		"npc-lobo": {ID: "npc-lobo", Label: "Lobo Sombrio", Kind: kindNPC, Hidden: true},
	}
}

func logNames(svc *Service) *keyNames {
	sheet := link.Sheet{
		Attacks:        []link.Attack{{Key: "weapon:longsword", Name: "Espada Longa"}},
		Actions:        []link.Action{{Key: "standard:dash", Name: "Correr"}, {Key: "standard:search", Name: "Procurar"}},
		FeatureActions: []link.FeatureAction{{Key: "feature:second-wind", Name: "Retomar o Fôlego"}},
	}
	return &keyNames{
		s: svc, campaignID: "camp", byCharacter: map[string]link.Sheet{"char-aria": sheet, "char-bruno": sheet},
	}
}

// logCase is a combat's events, oldest first, and the event the master could undo.
type logCase struct {
	name string
	rows []playdb.ListEncounterEventsRow
	last string // "" is the last row
	// bare reads the entries with no reasons or requests tables (logRolls nil), as a reader that has none.
	bare bool
}

// ev is one event of a case: the id, the kind and the payload as JSON.
type ev struct{ id, kind, js string }

func rowsOf(evs []ev) []playdb.ListEncounterEventsRow {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	out := make([]playdb.ListEncounterEventsRow, len(evs))
	for i, e := range evs {
		out[i] = playdb.ListEncounterEventsRow{ID: e.id, Seq: int32(i + 1), Kind: e.kind, Payload: []byte(e.js), CreatedAt: base.Add(time.Duration(i) * time.Second)}
	}
	return out
}

// p is the payload of round 1 with the given members.
func p(members string) string {
	if members == "" {
		return `{"round":1}`
	}
	return `{"round":1,` + members + `}`
}

func (c logCase) render(svc *Service) string {
	var b strings.Builder
	fmt.Fprintf(&b, "######## %s\n", c.name)
	entries := buildLog(c.rows)
	b.WriteString("build:\n")
	for _, e := range entries {
		b.WriteString("  " + describeEntry(e) + "\n")
	}
	last := c.last
	if last == "" && len(c.rows) > 0 {
		last = c.rows[len(c.rows)-1].ID
	}
	byID := logCombatants()
	for _, vw := range logViewers() {
		fmt.Fprintf(&b, "-- %s\n", vw.name)
		names := logNames(svc)
		rolls := newLogRolls(
			[]playdb.CombatReason{{ID: "r1", Reason: "Estava na penumbra"}},
			[]playdb.RollModeRequest{{ID: "rq1", Reason: "Subiu na mesa"}},
			logContentName,
		)
		for _, e := range entries {
			if !c.bare {
				e.rolls = rolls
			}
		}
		lastID := ""
		if vw.v.master {
			lastID = last
		}
		for _, e := range entries {
			out, ok := e.view(context.Background(), vw.v, byID, names, lastID)
			if !ok {
				fmt.Fprintf(&b, "  %s: -\n", e.id)
				continue
			}
			fmt.Fprintf(&b, "  %s: %s\n", e.id, canonicalProto(out))
		}
	}
	return b.String()
}

func describeEntry(e *logEntry) string {
	var pend []string
	for id, d := range e.pend {
		pend = append(pend, fmt.Sprintf("%s=%s", id, d.status))
	}
	sort.Strings(pend)
	return fmt.Sprintf("%s kind=%s masterOnly=%t shapeStarted=%t status=%s hosts=%v stopped=%v returned=%t/%t pend=%v dmg=%t applied=%t removed=%t partLabel=%q",
		e.id, e.kind, e.masterOnly, e.shapeStarted, e.status, e.hosts, e.stopped, e.returned, e.returnBlocked, pend,
		e.dmg != nil, e.applied != nil, e.removed != nil, e.partLabel)
}

// canonicalProto is the entry as JSON with sorted keys: protojson's whitespace is not stable.
func canonicalProto(m *playv1.CombatLogEntry) string {
	raw, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(m)
	if err != nil {
		return "ERR " + err.Error()
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return "ERR " + err.Error()
	}
	out, err := json.Marshal(tree)
	if err != nil {
		return "ERR " + err.Error()
	}
	return string(out)
}

func renderGolden() string {
	svc := &Service{
		roster: logRoster{}, traps: logTraps{}, logger: slog.New(slog.DiscardHandler),
		conditionNames: map[string]string{"condition:prone": "Caído"},
	}
	var b strings.Builder
	for _, c := range logGoldenCases() {
		b.WriteString(c.render(svc))
	}
	return b.String()
}

func TestCombatLogGolden(t *testing.T) {
	t.Parallel()
	got := renderGolden()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read the golden (UPDATE_GOLDEN=1 writes it): %v", err)
	}
	if string(want) == got {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(gl) && i < len(wl); i++ {
		if gl[i] != wl[i] {
			t.Fatalf("the combat log changed at line %d of the golden:\n got: %s\nwant: %s", i+1, gl[i], wl[i])
		}
	}
	t.Fatalf("the combat log has %d lines, the golden %d", len(gl), len(wl))
}

// reactionLine is only reached for an entry that has a reaction; a nil one says nothing to
// anybody (unreachable through buildLog, which keeps the entries that have one).
func TestCombatLogReactionLineWithoutAReaction(t *testing.T) {
	t.Parallel()
	e := &logEntry{kind: playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW}
	text, named := e.reactionLine(context.Background(), combatViewer{master: true}, logCombatants(), &keyNames{})
	if text != "" || named {
		t.Errorf("reactionLine(no reaction) = %q, %t; want nothing", text, named)
	}
}

// What the golden shows, said in four assertions: the leaks the rules forbid (RN-10, RN-20).
func TestCombatLogLeakRules(t *testing.T) {
	t.Parallel()
	svc := &Service{roster: logRoster{}, traps: logTraps{}, logger: slog.New(slog.DiscardHandler)}
	c := caseAttacks()
	entries := buildLog(c.rows)
	var a1 *logEntry
	for _, e := range entries {
		if e.id == "a1" {
			a1 = e
		}
	}
	if a1 == nil {
		t.Fatal("the attack a1 is not in the log")
	}
	read := func(v combatViewer, last string) *playv1.CombatLogEntry {
		out, ok := a1.view(context.Background(), v, logCombatants(), logNames(svc), last)
		if !ok {
			t.Fatalf("%+v does not get the attack", v)
		}
		return out
	}
	master := read(combatViewer{master: true}, "a1d")
	other := read(combatViewer{userID: "u2"}, "")
	if master.GetAttackRoll() == nil || master.GetTargetArmorClass() == 0 || !master.GetUndoable() {
		t.Errorf("the master lacks the dice, the armor class or the undo: %v", master)
	}
	if other.GetAttackRoll() != nil || other.GetTargetArmorClass() != 0 || other.GetUndoable() || other.GetHidden() || other.HitPointsAfter != nil {
		t.Errorf("another player reads what is the master's: %v", other)
	}
	if read(combatViewer{userID: "u1"}, "").GetAttackRoll() == nil {
		t.Error("the attacker's player does not read their own dice")
	}
}
