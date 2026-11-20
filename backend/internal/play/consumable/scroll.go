package consumable

import (
	"encoding/json"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The reading of a Spell Scroll (ChargesService.ReadScroll): what refuses it, the check, what a reading leaves behind and how long
// a pass stands.

// The outcomes of a reading as a payload says them (Used.Outcome). A read that needs no check or whose check passed is the session
// event ScrollRead, the pass of the cast; a failed one is a consumable_used event.
const (
	// KindScrollRead is the kind of the session event of a pass.
	KindScrollRead      = eventScrollRead
	OutcomeScrollRead   = "scroll_read"
	OutcomeScrollFailed = "scroll_failed"
)

// eventScrollRead is the session event kind of a pass (the code and the table of kinds are kept in step by the scan of event constants).
const eventScrollRead = "scroll_read"

// The kinds of the other session events a pass reads: a failed reading, and what ends a reader's turn.
const (
	kindConsumableUsed = "consumable_used"
	kindTurnEnded      = "turn_ended"
	kindEncounterEnded = "encounter_ended"
)

// PassWindow is how many of the session's latest events a pass is looked for in (the undo's window): a read older than that has
// lapsed whatever else happened.
const PassWindow = 100

// Refusal is a reading the server refuses, with nothing spent: the reason the app reads and the words it says.
type Refusal struct {
	Reason playv1.EncounterBlockedReason
	Text   string
}

// ScrollRefusal says why the entry cannot be read, whatever the turn: it gives no spell (none stored, one of another level, or one
// the table switched off or archived, which no refusal names to a player, RN-23), or the spell is on the list of no class the
// reader casts from. nil when it can be read.
func ScrollRefusal(e link.ScrollEntry) *Refusal {
	switch {
	case e.NoSpell && e.Off:
		return &Refusal{playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_SPELL_OFF, rules.ScrollSpellGonePT}
	case e.NoSpell:
		return &Refusal{playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_NO_SPELL, "O pergaminho não guarda uma magia."}
	case !e.Reads:
		return &Refusal{playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_SPELL_NOT_ON_LIST, e.UnreadablePT}
	}
	return nil
}

// EconomyRefusal says whether the turn leaves the reader the economy the spell's casting time costs (what a cast of it would need):
// nil when it does. The master has the last word on the economy, as for a cast, and is never refused here.
func EconomyRefusal(turn combat.TurnState, e link.ScrollEntry, master bool) *Refusal {
	if master {
		return nil
	}
	r := combat.ScrollReadReason(turn, e.Economy, e.Level)
	if r == nil {
		return nil
	}
	switch r.Code {
	case combat.ReasonActionUsed:
		return &Refusal{playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used"}
	case combat.ReasonBonusActionUsed:
		return &Refusal{playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED, "the bonus action of this turn is used"}
	case combat.ReasonReactionUsed:
		return &Refusal{playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction of this turn is used"}
	case combat.ReasonBonusActionSpellLimit:
		return &Refusal{playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_SPELL_LIMIT, "a spell cast with a bonus action leaves no other spell this turn but a cantrip of 1 action"}
	}
	return nil
}

// CheckOf is the check of reading the entry's spell for the d20 rolled or typed: the d20 and the reader's spellcasting modifier
// against the DC (rules.ScrollRead.Check), a pass with nothing rolled when the read asks none.
func CheckOf(e link.ScrollEntry, d20 int) rules.ScrollCheck {
	return rules.ScrollRead{CheckNeeded: e.CheckNeeded, CheckDC: e.CheckDC}.Check(d20, e.Modifier)
}

// Reading fills the payload of a reading that came to be: the scroll's spell, its level and, when a check was made, the check's
// numbers. reader is the character that read it and physical says the d20 was typed.
func (u *Used) Reading(reader string, e link.ScrollEntry, check rules.ScrollCheck, physical bool) {
	u.Reader, u.Spell, u.Level = reader, e.SpellKey, conv.Int32(e.Level)
	u.Checked = e.CheckNeeded
	u.CheckDC, u.D20, u.Modifier, u.Total, u.Physical = conv.Int32(check.DC), conv.Int32(check.D20), conv.Int32(check.Modifier), conv.Int32(check.Total), physical
	u.Outcome = OutcomeScrollRead
	if !check.Passed {
		u.Outcome = OutcomeScrollFailed
	}
}

// ScrollReading is what the reading answers, from the payload the call wrote or found; spellName is the spell's name in Portuguese as
// the reader may read it ("" when it may not, and then the key is left out too: RN-23).
func ScrollReading(u Used, spellName string) *playv1.ScrollReading {
	key := u.Spell
	if spellName == "" {
		key = ""
	}
	out := &playv1.ScrollReading{
		EntryId: u.Entry, Outcome: playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_NO_CHECK_NEEDED, SpellKey: key, SpellNamePt: spellName,
		ScrollLevel: u.Level, CastLevel: u.Level, CheckDc: u.CheckDC, D20: u.D20, Modifier: u.Modifier,
	}
	if u.Checked {
		out.Total = u.Total
		out.Outcome = playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_PASSED
	}
	if u.Outcome == OutcomeScrollFailed {
		out.Outcome, out.EntryGone = playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_FAILED, true
	}
	return out
}

// ScrollLine is the scroll's part of the log line of a failed reading, for the readers of the numbers (the master and the owner): the
// spell and the check. nil for any other line. spellName is the spell's name as the reader may read it ("" when it may not).
func ScrollLine(u Used, spellName string) *playv1.ScrollReadLogLine {
	if u.Outcome != OutcomeScrollFailed {
		return nil
	}
	key := u.Spell
	if spellName == "" { // a spell the reader may not read is not named by its key either (RN-23)
		key = ""
	}
	return &playv1.ScrollReadLogLine{
		SpellKey: key, SpellNamePt: spellName, ScrollLevel: u.Level, CheckDc: u.CheckDC, D20: u.D20, Modifier: u.Modifier, Total: u.Total,
	}
}

// Pass is a reading that stands: the scroll's entry, its spell and level, and when and where it was read.
type Pass struct {
	Entry, Spell string
	Level        int32
	// Encounter is the combat it was read in, "" for a read out of one, and Round that combat's round.
	Encounter string
	Round     int32
	// Checked says a check was made, and CheckDC, D20, Modifier and Total are its numbers.
	Checked                       bool
	CheckDC, D20, Modifier, Total int32
}

// LatestPass finds the reading of the reader's that stands among the session's latest events (newest first, PassWindow of them): the
// reader's latest reading of a scroll, if it came to a pass. It lapsed when a reading of the reader's came after it, whatever its
// result (a failed one is one), and when the turn of the combat it was read in ended or the combat ended. A pass read out of a combat
// has no turn to end: it stands until the reader reads again.
func LatestPass(events []playdb.ListRecentSessionEventsRow, reader string) (Pass, bool) {
	ended := map[string]bool{} // the combats whose turn ended, or that ended, after the event looked at
	for _, e := range events {
		switch e.Kind {
		case kindTurnEnded, kindEncounterEnded:
			if e.EncounterID != nil {
				ended[*e.EncounterID] = true
			}
		case KindScrollRead, kindConsumableUsed:
			var u Used
			if json.Unmarshal(e.Payload, &u) != nil || u.Reader != reader {
				continue
			}
			enc := conv.OrZero(e.EncounterID)
			if u.Outcome != OutcomeScrollRead || (enc != "" && ended[enc]) {
				return Pass{}, false
			}
			return Pass{
				Entry: u.Entry, Spell: u.Spell, Level: u.Level, Encounter: enc, Round: u.Round,
				Checked: u.Checked, CheckDC: u.CheckDC, D20: u.D20, Modifier: u.Modifier, Total: u.Total,
			}, true
		}
	}
	return Pass{}, false
}

// RollOK says the roll of a request has the shape the call allows: none (a read that asks no check needs none), the app's roll or a
// face of the d20.
func RollOK(r *playv1.ReadScrollRequest) bool {
	switch r.GetRoll().(type) {
	case *playv1.ReadScrollRequest_RollInApp:
		return r.GetRollInApp()
	case *playv1.ReadScrollRequest_D20Face:
		return r.GetD20Face() >= 1 && r.GetD20Face() <= 20
	}
	return true
}

// TurnOf is the turn the combatant has used, as the rules of the casting time read it.
func TurnOf(c playdb.Combatant) combat.TurnState {
	return combat.TurnState{ActionUsed: c.ActionUsed, BonusActionUsed: c.BonusActionUsed, ReactionUsed: c.ReactionUsed, SpellCast: c.SpellCast, BonusSpellCast: c.BonusSpellCast}
}

// Spend marks on the combatant the economy the spell's casting time costs, and says whether it costs one ("" is a longer time).
func Spend(c *playdb.Combatant, economy string) bool {
	switch economy {
	case rules.EconomyAction:
		c.ActionUsed = true
	case rules.EconomyBonusAction:
		c.BonusActionUsed = true
	case rules.EconomyReaction:
		c.ReactionUsed = true
	default:
		return false
	}
	return true
}
