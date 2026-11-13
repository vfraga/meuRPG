package play

import (
	"encoding/json"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// A roll stored before the table minimum has counted_as and no counted_by: only Reliable Talent could have raised it, so
// every reader of a stored countedRoll names that feature as the source.
func TestStoredCountedRollsWithoutASourceNameReliableTalent(t *testing.T) {
	t.Parallel()
	const stored = `{"counted_as":10}`
	decode := func(v any) {
		t.Helper()
		if err := json.Unmarshal([]byte(stored), v); err != nil {
			t.Fatalf("decode %T: %v", v, err)
		}
	}
	var contest contestRoll
	decode(&contest)
	var ev actionEvent
	decode(&ev)
	var rider riderEvent
	decode(&rider)
	var channel channelEvent
	if err := json.Unmarshal([]byte(`{"turns":[{"counted_as":10}]}`), &channel); err != nil {
		t.Fatalf("decode a channel: %v", err)
	}
	var save playv1.CombatLogHitRider
	riderNumbersProto(&save, &rider, false)
	checks := map[string]string{
		"contest side":     contest.proto(true).GetTreatedAsSource(),
		"attack or save":   ev.mark(&playv1.DiceRoll{}).GetTreatedAsSource(),
		"hit rider":        save.GetTreatedAsSource(),
		"channel divinity": channelProto(&channel).GetTurns()[0].GetTreatedAsSource(),
	}
	for what, got := range checks {
		if got != reliableTalentSource {
			t.Errorf("%s names %q as the source of a stored counted_as with no counted_by, want %q", what, got, reliableTalentSource)
		}
	}
}

// The save of an undead against Turn Undead counts the d20 by the floor: a 3 under a minimum of 10 counts as 10 (the faces
// stay), which saves against DC 12 with a +2 bonus; the same 3 with no floor does not.
func TestTurnUndeadSaveCountsTheMinimum(t *testing.T) {
	t.Parallel()
	const bonus, dc = 2, 12
	roll := d20Roll{Faces: []int{3}, Total: 3 + bonus}
	floored := turnedSave(playdbCombatant("u1", "Esqueleto"), bonus, roll, dc, rollFloor{Value: 10, Source: "feat:piso"})
	if floored.D20 != 3 || floored.Total != 12 || floored.Turned || floored.CountedAs != 10 || floored.CountedBy != "feat:piso" {
		t.Errorf("with the floor: %+v, want the 3 counted as 10 by the feat, total 12, not turned", floored)
	}
	plain := turnedSave(playdbCombatant("u1", "Esqueleto"), bonus, roll, dc, rollFloor{})
	if plain.Total != 5 || !plain.Turned || plain.CountedAs != 0 {
		t.Errorf("without a floor: %+v, want the 3 as it came up, total 5, turned", plain)
	}
}

func playdbCombatant(id, label string) playdb.Combatant {
	return playdb.Combatant{ID: id, Label: label}
}
