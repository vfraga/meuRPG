package consumable

import (
	"encoding/json"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

const (
	blocked = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_UNSPECIFIED
	reader  = "11111111-1111-4111-8111-111111111111"
	other   = "22222222-2222-4222-8222-222222222222"
	fight   = "33333333-3333-4333-8333-333333333333"
	another = "44444444-4444-4444-8444-444444444444"
)

// A read is refused, in this order, for a scroll that gives no spell (off, or none), and for a spell off the reader's lists.
func TestScrollRefusal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		entry  link.ScrollEntry
		reason playv1.EncounterBlockedReason
		text   string
	}{
		{"a spell that can be read", link.ScrollEntry{Reads: true}, blocked, ""},
		{
			"a spell that is off says it without a name",
			link.ScrollEntry{NoSpell: true, Off: true, SpellKey: "spell:fireball"},
			playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_SPELL_OFF, "A magia deste pergaminho não está mais na mesa.",
		},
		{
			"a scroll with no spell",
			link.ScrollEntry{NoSpell: true},
			playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_NO_SPELL, "O pergaminho não guarda uma magia.",
		},
		{
			"a spell of no list of the reader",
			link.ScrollEntry{SpellKey: "spell:cure-wounds", UnreadablePT: "Não é da lista de Mago."},
			playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_SPELL_NOT_ON_LIST, "Não é da lista de Mago.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScrollRefusal(tt.entry)
			if tt.reason == blocked {
				if got != nil {
					t.Fatalf("refused: %+v", got)
				}
				return
			}
			if got == nil || got.Reason != tt.reason || got.Text != tt.text {
				t.Errorf("refusal %+v, want %v %q", got, tt.reason, tt.text)
			}
		})
	}
}

// The economy refusal follows the casting time, and the master has the last word on it.
func TestEconomyRefusal(t *testing.T) {
	t.Parallel()
	action := link.ScrollEntry{Economy: rules.EconomyAction, Level: 3}
	bonus := link.ScrollEntry{Economy: rules.EconomyBonusAction, Level: 2}
	tests := []struct {
		name   string
		turn   combat.TurnState
		entry  link.ScrollEntry
		master bool
		want   playv1.EncounterBlockedReason
	}{
		{"a free turn", combat.TurnState{}, action, false, blocked},
		{"the action is used", combat.TurnState{ActionUsed: true}, action, false, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED},
		{"the action is used, for the master", combat.TurnState{ActionUsed: true}, action, true, blocked},
		{"the bonus action is used", combat.TurnState{BonusActionUsed: true}, bonus, false, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED},
		{
			"the reaction is used",
			combat.TurnState{ReactionUsed: true},
			link.ScrollEntry{Economy: rules.EconomyReaction, Level: 1},
			false,
			playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED,
		},
		{"the bonus action limit", combat.TurnState{SpellCast: true}, bonus, false, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_SPELL_LIMIT},
		{"a longer casting asks nothing", combat.TurnState{ActionUsed: true}, link.ScrollEntry{Level: 1}, false, blocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EconomyRefusal(tt.turn, tt.entry, tt.master)
			switch {
			case tt.want == blocked && got != nil:
				t.Errorf("refused: %+v", got)
			case tt.want != blocked && (got == nil || got.Reason != tt.want || got.Text == ""):
				t.Errorf("refusal %+v, want %v with words", got, tt.want)
			}
		})
	}
}

// The payload keeps the check's numbers, the answer and the log line read them back, and a read with no check carries none.
func TestReadingPayloadAnswerAndLine(t *testing.T) {
	t.Parallel()
	entry := link.ScrollEntry{SpellKey: "spell:fireball", Level: 3, CheckNeeded: true, CheckDC: 13, Modifier: 3}
	var failed, passed, none Used
	failed.Entry, passed.Entry, none.Entry = "m1", "m1", "m2"
	failed.Reading(reader, entry, CheckOf(entry, 9), true)
	passed.Reading(reader, entry, CheckOf(entry, 10), false)
	none.Reading(reader, link.ScrollEntry{SpellKey: "spell:fire-bolt"}, rules.ScrollRead{}.Check(0, 0), false)

	if failed.Outcome != OutcomeScrollFailed || !failed.Physical || passed.Outcome != OutcomeScrollRead || passed.Physical || none.Outcome != OutcomeScrollRead {
		t.Fatalf("outcomes %q %q %q, typed %v %v", failed.Outcome, passed.Outcome, none.Outcome, failed.Physical, passed.Physical)
	}
	wantFailed := &playv1.ScrollReading{
		EntryId: "m1", Outcome: playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_FAILED, SpellKey: "spell:fireball", SpellNamePt: "Bola de Fogo",
		ScrollLevel: 3, CastLevel: 3, CheckDc: 13, D20: 9, Modifier: 3, Total: 12, EntryGone: true,
	}
	assertReading(t, "failed", ScrollReading(failed, "Bola de Fogo"), wantFailed)
	wantPassed := &playv1.ScrollReading{
		EntryId: "m1", Outcome: playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_PASSED, SpellKey: "spell:fireball", SpellNamePt: "Bola de Fogo",
		ScrollLevel: 3, CastLevel: 3, CheckDc: 13, D20: 10, Modifier: 3, Total: 13,
	}
	assertReading(t, "passed", ScrollReading(passed, "Bola de Fogo"), wantPassed)
	wantNone := &playv1.ScrollReading{EntryId: "m2", Outcome: playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_NO_CHECK_NEEDED, SpellKey: "spell:fire-bolt", SpellNamePt: "Raio de Fogo"}
	assertReading(t, "no check", ScrollReading(none, "Raio de Fogo"), wantNone)

	line := ScrollLine(failed, "Bola de Fogo")
	if line == nil || line.GetSpellKey() != "spell:fireball" || line.GetSpellNamePt() != "Bola de Fogo" || line.GetScrollLevel() != 3 || line.GetCheckDc() != 13 ||
		line.GetD20() != 9 || line.GetModifier() != 3 || line.GetTotal() != 12 {
		t.Errorf("line %+v", line)
	}
	if hidden := ScrollLine(failed, ""); hidden == nil || hidden.GetSpellKey() != "" || hidden.GetSpellNamePt() != "" || hidden.GetCheckDc() != 13 || hidden.GetTotal() != 12 {
		t.Errorf("a spell the reader may not read: line %+v, want the check and neither the key nor the name", hidden)
	}
	if ScrollLine(passed, "x") != nil || ScrollLine(Used{Outcome: OutcomeHealed}, "x") != nil {
		t.Error("a line that is no failed reading carries a scroll")
	}
	if got := OutcomeOf(OutcomeScrollFailed); got != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_SCROLL_FAILED {
		t.Errorf("failed: %v", got)
	}
	if got := OutcomeOf(OutcomeScrollRead); got != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_SCROLL_READ {
		t.Errorf("read: %v", got)
	}
}

func assertReading(t *testing.T, name string, got, want *playv1.ScrollReading) {
	t.Helper()
	if got.String() != want.String() {
		t.Errorf("%s: %v, want %v", name, got, want)
	}
}

func event(kind string, encounter string, u Used) Event {
	body, err := json.Marshal(u)
	if err != nil {
		panic(err)
	}
	row := Event{Kind: kind, Payload: body}
	if encounter != "" {
		row.EncounterID = &encounter
	}
	return row
}

func read(entry string) Used {
	return Used{Entry: entry, Reader: reader, Spell: "spell:fireball", Level: 3, Outcome: OutcomeScrollRead, Checked: true, CheckDC: 13, D20: 12, Modifier: 3, Total: 15}
}

// A pass stands until the reader reads again, the reader's turn ends or the combat ends; nothing else lapses it.
func TestLatestPass(t *testing.T) {
	t.Parallel()
	failed := read("m1")
	failed.Outcome = OutcomeScrollFailed
	byOther := read("m9")
	byOther.Reader = other
	tests := []struct {
		name   string
		events []Event // newest first
		want   string  // the entry of the pass, "" for none
	}{
		{"no event", nil, ""},
		{"a read in a combat", []Event{event(KindScrollRead, fight, read("m1"))}, "m1"},
		{"a read out of a combat", []Event{event(KindScrollRead, "", read("m1"))}, "m1"},
		{"the turn ended after it", []Event{{Kind: kindTurnEnded, EncounterID: ptr(fight)}, event(KindScrollRead, fight, read("m1"))}, ""},
		{"the combat ended after it", []Event{{Kind: kindEncounterEnded, EncounterID: ptr(fight)}, event(KindScrollRead, fight, read("m1"))}, ""},
		{"the turn of another combat ended", []Event{{Kind: kindTurnEnded, EncounterID: ptr(another)}, event(KindScrollRead, fight, read("m1"))}, "m1"},
		{"a turn that ended before it", []Event{event(KindScrollRead, fight, read("m1")), {Kind: kindTurnEnded, EncounterID: ptr(fight)}}, "m1"},
		{"a turn that ends a read out of a combat", []Event{{Kind: kindTurnEnded, EncounterID: ptr(fight)}, event(KindScrollRead, "", read("m1"))}, "m1"},
		{"another read of the reader's", []Event{event(KindScrollRead, fight, read("m2")), event(KindScrollRead, fight, read("m1"))}, "m2"},
		{"a failed read after it", []Event{event(kindConsumableUsed, fight, failed), event(KindScrollRead, fight, read("m1"))}, ""},
		{"a pass after a failed read", []Event{event(KindScrollRead, fight, read("m2")), event(kindConsumableUsed, fight, failed)}, "m2"},
		{"another reader's read does not lapse it", []Event{event(KindScrollRead, fight, byOther), event(KindScrollRead, fight, read("m1"))}, "m1"},
		{"a consumable of another kind does not lapse it", []Event{event(kindConsumableUsed, fight, Used{Item: "item:potion-of-healing", Outcome: OutcomeHealed}), event(KindScrollRead, fight, read("m1"))}, "m1"},
		{"only another reader's pass", []Event{event(KindScrollRead, fight, byOther)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := LatestPass(tt.events, reader)
			if ok != (tt.want != "") || got.Entry != tt.want {
				t.Errorf("pass %+v, %v; want the entry %q", got, ok, tt.want)
			}
		})
	}
	got, _ := LatestPass([]Event{event(KindScrollRead, fight, read("m1"))}, reader)
	want := Pass{Entry: "m1", Spell: "spell:fireball", Level: 3, Encounter: fight, Checked: true, CheckDC: 13, D20: 12, Modifier: 3, Total: 15}
	if got != want {
		t.Errorf("pass %+v, want %+v", got, want)
	}
}

func ptr(s string) *string { return &s }
