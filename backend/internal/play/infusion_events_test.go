package play

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/play/infusion"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// What play does with the changes of an artificer's infusions (characters.ReviewHost.InfusionChanged): one session event for
// each change when the campaign has an open session and none otherwise, the open combat that holds the character told that it
// changed, and the master and the owner told, with no content, that the infusions changed.

const (
	kEncounterChanged       = "*playv1.WatchGameSessionResponse_EncounterChanged_"
	kCharacterInfusionsHint = "*playv1.WatchGameSessionResponse_CharacterInfusionsChanged_"
	infusionKeyOfTheTests   = "feature:class-artifex--blade@table"
)

func infusionChange(kind link.InfusionChangeKind, record, character string) link.InfusionChange {
	return link.InfusionChange{
		Kind: kind, RecordID: record, ArtificerID: character, HolderID: character, InfusionKey: infusionKeyOfTheTests,
		ItemKind: "gear", ItemKey: "equipment:dagger",
	}
}

// reportInfusion is what the characters module does in a write: it runs inside the session's funnel, moves the number and
// tells play.
func (a *armed) reportInfusion(t *testing.T, by *user, changes ...link.InfusionChange) error {
	t.Helper()
	return a.h.svc.InSession(t.Context(), a.campaignID, func(tx pgx.Tx, tl sessiontell.Tell) error {
		tl.Touch()
		return a.h.svc.InfusionChanged(t.Context(), tx, a.campaignID, by.id, changes, tl)
	})
}

// infusionRows are the session events of the infusion kinds, oldest first.
type infusionRow struct {
	Kind, Character, Actor string
	Encounter              *string
	Payload                []byte
}

func (a *armed) infusionRows(t *testing.T) []infusionRow {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `
		SELECT e.kind, COALESCE(e.character_id::TEXT, ''), COALESCE(e.actor_user_id::TEXT, ''), e.encounter_id::TEXT, e.payload
		FROM session_events e JOIN game_sessions s ON s.id = e.game_session_id
		WHERE s.campaign_id = $1 AND e.kind IN ('infusion_infused', 'infusion_moved', 'infusion_ended') ORDER BY e.seq`, a.campaignID)
	if err != nil {
		t.Fatalf("read the infusion events: %v", err)
	}
	defer rows.Close()
	var out []infusionRow
	for rows.Next() {
		var r infusionRow
		if err := rows.Scan(&r.Kind, &r.Character, &r.Actor, &r.Encounter, &r.Payload); err != nil {
			t.Fatalf("read an infusion event: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the infusion events: %v", err)
	}
	return out
}

// logLines counts the lines the combat's log shows to u.
func (a *armed) logLines(t *testing.T, u *user, e *playv1.Encounter) int {
	t.Helper()
	n := 0
	for _, r := range a.log(t, u, e).GetRounds() {
		n += len(r.GetEntries())
	}
	return n
}

func (a *armed) encounterRevision(t *testing.T) int32 {
	t.Helper()
	var rev int32
	if err := a.h.pool.QueryRow(t.Context(), `SELECT e.revision FROM encounters e JOIN game_sessions s ON s.id = e.game_session_id WHERE s.campaign_id = $1 AND e.status <> 'ended'`, a.campaignID).Scan(&rev); err != nil {
		t.Fatalf("read the revision of the combat: %v", err)
	}
	return rev
}

// With an open session each change is one session event, in the order of the call, by the author of the write, about the
// artificer, with no combat of its own and a payload of ids and keys only.
func TestInfusionChangesAreSessionEventsOfTheOpenSession(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	pens := a.pens.GetId()
	ended := infusionChange(link.InfusionEnded, "00000000-0000-4000-8000-000000000001", pens)
	ended.EndedReason = "replaced"
	infused := infusionChange(link.InfusionInfused, "00000000-0000-4000-8000-000000000002", pens)
	moved := infusionChange(link.InfusionMoved, "00000000-0000-4000-8000-000000000003", pens)
	if err := a.reportInfusion(t, a.ana, ended, infused, moved); err != nil {
		t.Fatalf("InfusionChanged() error = %v", err)
	}
	rows := a.infusionRows(t)
	kinds := []string{infusion.KindEnded, infusion.KindInfused, infusion.KindMoved}
	if len(rows) != len(kinds) {
		t.Fatalf("the session holds %d infusion events, want %d", len(rows), len(kinds))
	}
	for i, r := range rows {
		if r.Kind != kinds[i] || r.Character != pens || r.Actor != a.ana.id || r.Encounter != nil {
			t.Errorf("event %d = %+v, want kind %s about %s by Ana, with no combat", i, r, kinds[i], pens)
		}
	}
	var got infusion.Event
	if err := json.Unmarshal(rows[0].Payload, &got); err != nil {
		t.Fatal(err)
	}
	want := infusion.Event{
		RecordID: "00000000-0000-4000-8000-000000000001", ArtificerID: pens, HolderID: pens, Infusion: infusionKeyOfTheTests,
		ItemKind: "gear", ItemKey: "equipment:dagger", Ended: "replaced",
	}
	if got != want {
		t.Errorf("payload = %+v, want %+v", got, want)
	}
	var keys map[string]any
	if err := json.Unmarshal(rows[1].Payload, &keys); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	slices.Sort(names)
	if wantKeys := []string{"artificer_character_id", "holder_character_id", "infusion_key", "item_key", "item_kind", "record_id"}; !slices.Equal(names, wantKeys) {
		t.Errorf("the payload of an infusion holds %v, want ids and keys only: %v", names, wantKeys)
	}
}

// With no open session the record and the hint still happen (the characters module's), but play writes no session event and
// moves no number.
func TestInfusionChangesWithNoOpenSessionWriteNoEvent(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.partyHintRig(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	g.drain()
	before := g.seq()
	if err := a.reportInfusion(t, a.ana, infusionChange(link.InfusionInfused, "00000000-0000-4000-8000-000000000002", a.pens.GetId())); err != nil {
		t.Fatalf("InfusionChanged(no open session) error = %v", err)
	}
	if rows := a.infusionRows(t); len(rows) != 0 {
		t.Errorf("a campaign with no open session holds %d infusion events, want none", len(rows))
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d", got, before)
	}
}

// The master and the owner hear that the infusions changed; no other player does, and the hint names only the character.
func TestInfusionsChangedHintGoesToTheMasterAndTheOwnerOnly(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.partyHintRig(t, "Caio", "Bia")
	tell := func() error {
		return a.h.svc.InSession(t.Context(), a.campaignID, func(_ pgx.Tx, tl sessiontell.Tell) error {
			tl.CharacterEvent(a.ana.id, a.pens.GetId(), "infusions_changed")
			return nil
		})
	}
	heard := g.first(tell, true)
	for _, name := range []string{"master", "Ana"} {
		if !slices.Equal(heard[name], []string{kCharacterInfusionsHint}) {
			t.Errorf("%s heard %v, want the infusions hint", name, heard[name])
		}
	}
}

func infusionFight(t *testing.T, hidden bool) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmed(t)
	p := plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	}
	if !hidden {
		p.reveal = []string{"Goblin"}
	}
	enc := a.start(t, p)
	return a, enc
}

// A combatant of the open combat (running or in setup) that is the artificer: the combat changes, so its screens read the turn
// and the attack options again, and its log gets no line of it. A character that is no combatant touches nothing.
func TestInfusionChangeTellsTheCombatOfTheArtificer(t *testing.T) {
	t.Parallel()
	a, e := infusionFight(t, false)
	g := a.partyHintRig(t)
	logBefore := a.logLines(t, a.master, e)
	revision := a.encounterRevision(t)
	heard := g.first(func() error {
		return a.reportInfusion(t, a.ana, infusionChange(link.InfusionEnded, "00000000-0000-4000-8000-000000000001", a.pens.GetId()))
	}, true)
	for _, name := range []string{"master", "Caio", "Ana", "Bia"} {
		if !slices.Contains(heard[name], kEncounterChanged) {
			t.Errorf("%s heard %v, want the combat changed", name, heard[name])
		}
	}
	if got := a.encounterRevision(t); got != revision+1 {
		t.Errorf("the combat's revision = %d, want %d", got, revision+1)
	}
	if got := a.logLines(t, a.master, e); got != logBefore {
		t.Errorf("the combat's log has %d lines, want the %d it had: an infusion writes no line", got, logBefore)
	}
	if got := a.logLines(t, a.ana, e); got != logBefore {
		t.Errorf("the player's combat log has %d lines, want %d", got, logBefore)
	}

	// A character that fights in no combat: the Captain was never added.
	revision = a.encounterRevision(t)
	g.first(func() error {
		return a.reportInfusion(t, a.master, infusionChange(link.InfusionEnded, "00000000-0000-4000-8000-000000000004", a.capitao.GetId()))
	}, false)
	if got := a.encounterRevision(t); got != revision {
		t.Errorf("the combat's revision = %d after a character that is no combatant, want %d", got, revision)
	}
}

func TestInfusionChangeTellsACombatStillInSetup(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3}, setup: true,
		players: map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
	})
	revision := a.encounterRevision(t)
	g := a.partyHintRig(t)
	heard := g.first(func() error {
		return a.reportInfusion(t, a.ana, infusionChange(link.InfusionInfused, "00000000-0000-4000-8000-000000000002", a.pens.GetId()))
	}, true)
	if !slices.Contains(heard["master"], kEncounterChanged) {
		t.Errorf("the master heard %v, want the combat changed", heard["master"])
	}
	if got := a.encounterRevision(t); got != revision+1 {
		t.Errorf("the combat's revision = %d, want %d", got, revision+1)
	}
}

// An artificer the players cannot see (a hidden combatant): only the master hears that the combat changed.
func TestInfusionChangeOfAHiddenCombatantTellsOnlyTheMaster(t *testing.T) {
	t.Parallel()
	a, _ := infusionFight(t, true)
	g := a.partyHintRig(t, "Caio", "Ana", "Bia")
	heard := g.first(func() error {
		return a.reportInfusion(t, a.master, infusionChange(link.InfusionEnded, "00000000-0000-4000-8000-000000000001", a.goblin.GetId()))
	}, true)
	if !slices.Contains(heard["master"], kEncounterChanged) {
		t.Errorf("the master heard %v, want the combat changed", heard["master"])
	}
}

// A combatant that was dismissed no longer fights: a change of its infusions touches no combat.
func TestInfusionChangeOfADismissedCombatantTouchesNoCombat(t *testing.T) {
	t.Parallel()
	a, _ := infusionFight(t, false)
	a.execSQL(t, `UPDATE combatants SET dismissed = true WHERE label = 'Pensantus'`)
	revision := a.encounterRevision(t)
	g := a.partyHintRig(t)
	heard := g.first(func() error {
		return a.reportInfusion(t, a.ana, infusionChange(link.InfusionEnded, "00000000-0000-4000-8000-000000000001", a.pens.GetId()))
	}, false)
	if slices.Contains(heard["master"], kEncounterChanged) {
		t.Errorf("the master heard %v, want no combat change for a dismissed combatant", heard["master"])
	}
	if got := a.encounterRevision(t); got != revision {
		t.Errorf("the combat's revision = %d, want %d", got, revision)
	}
}
