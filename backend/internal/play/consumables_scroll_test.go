package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/consumable"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The reading of a Spell Scroll (ChargesService.ReadScroll): Pensantus is a wizard of level 1 with a modifier of +4 in Intelligence, so he
// casts the 1st level and reads anything above it with a check against DC 10 plus the spell's level. These tests need the database.

const (
	scrollCantrip = "item:spell-scroll-cantrip" // Fire Bolt: a cantrip, no check
	scrollSecond  = "item:spell-scroll-2nd"     // Misty Step: a bonus action, DC 12
	scrollThird   = "item:spell-scroll-3rd"     // Fireball: an action, DC 13
	scrollFourth  = "item:spell-scroll-4th"     // Death Ward: a cleric's, on no list of the wizard
	scrollFifth   = "item:spell-scroll-5th"     // Cone of Cold: the master switches it off
	scrollSixth   = "item:spell-scroll-6th"     // no spell

	spellMagicMissile = "spell:magic-missile"
	spellMistyStep    = "spell:misty-step"
	spellFireball     = "spell:fireball"
	spellDeathWard    = "spell:death-ward"
	spellConeOfCold   = "spell:cone-of-cold"

	scrollGoneWords = "A magia deste pergaminho não está mais na mesa."
)

var (
	blockedScrollNotOnList = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_SPELL_NOT_ON_LIST
	blockedScrollOff       = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_SPELL_OFF
	blockedBonusLimit      = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_SPELL_LIMIT
	blockedScrollNoSpell   = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SCROLL_NO_SPELL
	blockedItemGone        = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_GONE
)

// scrolls puts the scrolls on Pensantus's sheet and the spells on them, as the master.
func (s *stocked) scrolls(t *testing.T) {
	t.Helper()
	s.pens = s.withItems(t, s.pens, scrollFirst, scrollCantrip, scrollSecond, scrollThird, scrollFourth, scrollFifth, scrollSixth)
	for scroll, spell := range map[string]string{
		scrollFirst: spellMagicMissile, scrollCantrip: fireBolt, scrollSecond: spellMistyStep, scrollThird: spellFireball,
		scrollFourth: spellDeathWard, scrollFifth: spellConeOfCold,
	} {
		s.putSpell(t, s.pens, scroll, spell)
	}
}

// putSpell is the master putting a spell on the scroll of the character's sheet.
func (s *stocked) putSpell(t *testing.T, who *charactersv1.Character, scroll, spell string) {
	t.Helper()
	if _, err := s.master.characters.SetScrollSpell(t.Context(), connect.NewRequest(&charactersv1.SetScrollSpellRequest{
		CampaignId: s.campaignID, CharacterId: who.GetId(), EntryId: s.entryOf(t, who, scroll), SpellKey: spell,
	})); err != nil {
		t.Fatalf("SetScrollSpell(%s, %s) error = %v", scroll, spell, err)
	}
}

// read is u reading the scroll of who's sheet with a new key, with no roll unless an edit gives one.
func (s *stocked) read(t *testing.T, u *user, who *charactersv1.Character, scroll string, edit ...func(*playv1.ReadScrollRequest)) (*playv1.ScrollReading, error) {
	t.Helper()
	req := &playv1.ReadScrollRequest{CampaignId: s.campaignID, CharacterId: who.GetId(), IdempotencyKey: newKey()}
	for _, e := range edit {
		e(req)
	}
	if req.EntryId == "" {
		req.EntryId = s.entryOf(t, who, scroll)
	}
	res, err := u.charges.ReadScroll(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetReading(), nil
}

func (s *stocked) mustRead(t *testing.T, u *user, who *charactersv1.Character, scroll string, edit ...func(*playv1.ReadScrollRequest)) *playv1.ScrollReading {
	t.Helper()
	res, err := s.read(t, u, who, scroll, edit...)
	if err != nil {
		t.Fatalf("ReadScroll(%s) error = %v", scroll, err)
	}
	return res
}

func scrollD20(face int32) func(*playv1.ReadScrollRequest) {
	return func(r *playv1.ReadScrollRequest) { r.Roll = &playv1.ReadScrollRequest_D20Face{D20Face: face} }
}

func scrollAppD20(r *playv1.ReadScrollRequest) {
	r.Roll = &playv1.ReadScrollRequest_RollInApp{RollInApp: true}
}

// passOf is the pass the character holds now, as the cast of the scroll's spell will read it: the session's latest events.
func (s *stocked) passOf(t *testing.T, who *charactersv1.Character) (consumable.Pass, bool) {
	t.Helper()
	var session string
	if err := s.h.pool.QueryRow(t.Context(), `SELECT id FROM game_sessions WHERE campaign_id = $1 AND ended_at IS NULL`, s.campaignID).Scan(&session); err != nil {
		t.Fatalf("find the open session: %v", err)
	}
	rows, err := playdb.New(s.h.pool).ListRecentSessionEvents(t.Context(), playdb.ListRecentSessionEventsParams{GameSessionID: session, Limit: consumable.PassWindow})
	if err != nil {
		t.Fatalf("read the latest events: %v", err)
	}
	events := make([]consumable.Event, len(rows))
	for i, r := range rows {
		events[i] = consumable.Event{Kind: r.Kind, EncounterID: r.EncounterID, Payload: r.Payload}
	}
	return consumable.LatestPass(events, who.GetId())
}

// nothingSpent says no scroll left the sheet and no reading was written.
func (s *stocked) nothingSpent(t *testing.T, what string, scrolls ...string) {
	t.Helper()
	for _, k := range scrolls {
		if !s.holds(t, s.pens, k) {
			t.Errorf("%s: %s left the sheet", what, k)
		}
	}
	if n := s.countEvents(t, consumable.KindScrollRead) + s.countEvents(t, eventConsumableUsed); n != 0 {
		t.Errorf("%s: %d reading events were written", what, n)
	}
}

func TestReadingAScrollThatAsksNoCheckSpendsNothingAndLeavesAPass(t *testing.T) {
	t.Parallel()
	s := newStocked(t)
	s.scrolls(t)
	taken := s.h.roller.taken()
	// A spell of a level the reader casts, and a cantrip: no check, and a roll the request carries is not used.
	first := s.mustRead(t, s.ana, s.pens, scrollFirst, scrollD20(20))
	want := &playv1.ScrollReading{
		EntryId: s.entryOf(t, s.pens, scrollFirst), Outcome: playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_NO_CHECK_NEEDED,
		SpellKey: spellMagicMissile, SpellNamePt: first.GetSpellNamePt(), ScrollLevel: 1, CastLevel: 1,
	}
	if first.GetSpellNamePt() == "" || first.String() != want.String() {
		t.Errorf("the 1st level scroll reads %v, want %v", first, want)
	}
	if pass, ok := s.passOf(t, s.pens); !ok || pass.Entry != want.GetEntryId() || pass.Spell != spellMagicMissile || pass.Checked {
		t.Errorf("the pass = %+v, %v; want the 1st level scroll with no check", pass, ok)
	}
	cantrip := s.mustRead(t, s.ana, s.pens, scrollCantrip, scrollAppD20)
	if cantrip.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_NO_CHECK_NEEDED || cantrip.GetScrollLevel() != 0 || cantrip.GetCastLevel() != 0 || cantrip.GetSpellKey() != fireBolt {
		t.Errorf("the cantrip scroll reads %v, want no check at level 0", cantrip)
	}
	if got := s.h.roller.taken(); got != taken {
		t.Errorf("%d dice were rolled for reads that need no check", got-taken)
	}
	if !s.holds(t, s.pens, scrollFirst) || !s.holds(t, s.pens, scrollCantrip) || s.countEvents(t, eventConsumableUsed) != 0 || s.countEvents(t, consumable.KindScrollRead) != 2 {
		t.Error("a read that needs no check spent a unit, or wrote something other than its pass")
	}
}

func TestAPassedCheckSpendsNothingAndAFailedOneSpendsTheScroll(t *testing.T) {
	t.Parallel()
	s := newStocked(t)
	s.scrolls(t)
	// 9 + 4 = 13 reaches the DC of 13: a tie passes.
	passed := s.mustRead(t, s.ana, s.pens, scrollThird, scrollD20(9))
	wantPassed := &playv1.ScrollReading{
		EntryId: s.entryOf(t, s.pens, scrollThird), Outcome: playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_PASSED, SpellKey: spellFireball, SpellNamePt: passed.GetSpellNamePt(),
		ScrollLevel: 3, CastLevel: 3, CheckDc: 13, D20: 9, Modifier: 4, Total: 13,
	}
	if passed.GetSpellNamePt() == "" || passed.String() != wantPassed.String() {
		t.Fatalf("the passed check reads %v, want %v", passed, wantPassed)
	}
	if pass, ok := s.passOf(t, s.pens); !ok || pass.Entry != wantPassed.GetEntryId() || !pass.Checked || pass.D20 != 9 || pass.Total != 13 {
		t.Errorf("the pass = %+v, %v; want the checked read", pass, ok)
	}
	if !s.holds(t, s.pens, scrollThird) || s.countEvents(t, eventConsumableUsed) != 0 {
		t.Error("a passed check spent the scroll")
	}
	// 8 + 4 = 12: one short. The scroll comes apart, and the read is no pass any more.
	failed := s.mustRead(t, s.ana, s.pens, scrollThird, scrollD20(8))
	wantFailed := &playv1.ScrollReading{
		EntryId: wantPassed.GetEntryId(), Outcome: playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_FAILED, SpellKey: spellFireball, SpellNamePt: passed.GetSpellNamePt(),
		ScrollLevel: 3, CastLevel: 3, CheckDc: 13, D20: 8, Modifier: 4, Total: 12, EntryGone: true,
	}
	if failed.String() != wantFailed.String() {
		t.Errorf("the failed check reads %v, want %v", failed, wantFailed)
	}
	if s.holds(t, s.pens, scrollThird) || !s.holds(t, s.pens, scrollFirst) || s.countEvents(t, eventConsumableUsed) != 1 {
		t.Error("a failed check must spend this scroll and no other")
	}
	if pass, ok := s.passOf(t, s.pens); ok {
		t.Errorf("the pass = %+v after the scroll came apart, want none", pass)
	}
	_, err := s.read(t, s.ana, s.pens, scrollThird, scrollD20(20), func(r *playv1.ReadScrollRequest) { r.EntryId = wantPassed.GetEntryId() })
	wantBlockedBy(t, "reading the scroll that came apart", err, blockedItemGone)
}

func TestTheCheckIsRolledByTheAppOrTypedByTheTableRule(t *testing.T) {
	t.Parallel()
	s := newStocked(t)
	s.scrolls(t)
	s.h.roller.queue(8)
	taken := s.h.roller.taken()
	rolled := s.mustRead(t, s.ana, s.pens, scrollSecond, scrollAppD20) // 8 + 4 = 12 against 12
	if rolled.GetD20() != 8 || rolled.GetTotal() != 12 || rolled.GetCheckDc() != 12 || rolled.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_PASSED || s.h.roller.taken() != taken+1 {
		t.Errorf("the app's roll reads %v with %d dice, want one d20 of 8 reaching DC 12", rolled, s.h.roller.taken()-taken)
	}

	s.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	_, err := s.read(t, s.ana, s.pens, scrollThird, scrollD20(15))
	wantBlockedBy(t, "a typed d20 where the table rolls in the app", err, blockedWrongDice)
	s.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	_, err = s.read(t, s.ana, s.pens, scrollThird, scrollAppD20)
	wantBlockedBy(t, "an app d20 where the table rolls real dice", err, blockedWrongDice)
	if !s.holds(t, s.pens, scrollThird) || s.countEvents(t, eventConsumableUsed) != 0 {
		t.Error("a refused roll spent the scroll")
	}
	typed := s.mustRead(t, s.ana, s.pens, scrollThird, scrollD20(15))
	if typed.GetD20() != 15 || typed.GetTotal() != 19 || typed.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_PASSED {
		t.Errorf("the typed d20 reads %v, want 15 + 4 = 19", typed)
	}
	// The master rolls as he likes, whatever the table's rule.
	s.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	if got := s.mustRead(t, s.master, s.pens, scrollThird, scrollD20(2)); got.GetD20() != 2 || got.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_FAILED {
		t.Errorf("the master's typed d20 reads %v, want a failure on 2", got)
	}
}

func TestAReadThatAsksACheckNeedsAFaceOfTheD20(t *testing.T) {
	t.Parallel()
	s := newStocked(t)
	s.scrolls(t)
	_, err := s.read(t, s.ana, s.pens, scrollThird)
	wantCode(t, "a check with no roll", err, connect.CodeInvalidArgument)
	for _, face := range []int32{0, 21, -3} {
		_, err = s.read(t, s.ana, s.pens, scrollThird, scrollD20(face))
		wantCode(t, "a face off the d20", err, connect.CodeInvalidArgument)
	}
	_, err = s.read(t, s.ana, s.pens, scrollThird, func(r *playv1.ReadScrollRequest) { r.Roll = &playv1.ReadScrollRequest_RollInApp{} })
	wantCode(t, "roll_in_app set to false", err, connect.CodeInvalidArgument)
	_, err = s.read(t, s.ana, s.pens, scrollFirst, scrollD20(0)) // a read that needs no check still has a shape
	wantCode(t, "a face off the d20 on a read with no check", err, connect.CodeInvalidArgument)
	s.nothingSpent(t, "the refused rolls", scrollThird, scrollFirst)
}

func TestAReadIsRefusedWithNothingSpent(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	s.scrolls(t)
	s.putSpell(t, s.pens, scrollFifth, spellConeOfCold)
	if _, err := s.master.table.SetOptionSwitches(t.Context(), connect.NewRequest(&rulesv1.SetOptionSwitchesRequest{
		CampaignId: s.campaignID, Switches: []*rulesv1.OptionSwitch{{Key: spellConeOfCold, Off: true}},
	})); err != nil {
		t.Fatalf("SetOptionSwitches() error = %v", err)
	}
	tests := []struct {
		name   string
		by     *user
		scroll string
		edit   func(*playv1.ReadScrollRequest)
		reason playv1.EncounterBlockedReason
		words  string
	}{
		{name: "a spell on no list of the reader", by: s.ana, scroll: scrollFourth, reason: blockedScrollNotOnList, words: "Não é da lista de Mago."},
		{name: "the same, for the master", by: s.master, scroll: scrollFourth, reason: blockedScrollNotOnList, words: "Não é da lista de Mago."},
		{name: "a scroll with no spell", by: s.ana, scroll: scrollSixth, reason: blockedScrollNoSpell},
		{name: "a spell the master switched off, for the owner", by: s.ana, scroll: scrollFifth, reason: blockedScrollOff, words: scrollGoneWords},
		{name: "a spell the master switched off, for the master", by: s.master, scroll: scrollFifth, reason: blockedScrollOff},
		{name: "an entry that is not on the sheet", by: s.ana, scroll: scrollFirst, edit: func(r *playv1.ReadScrollRequest) { r.EntryId = newKey() }, reason: blockedItemGone},
	}
	for _, tt := range tests {
		edit := []func(*playv1.ReadScrollRequest){scrollD20(20)}
		if tt.edit != nil {
			edit = append(edit, tt.edit)
		}
		_, err := s.read(t, tt.by, s.pens, tt.scroll, edit...)
		blocked := wantBlockedBy(t, tt.name, err, tt.reason)
		if tt.reason != blockedItemGone && blocked.GetReason() != tt.reason {
			t.Errorf("%s: reason %v", tt.name, blocked.GetReason())
		}
		if tt.words != "" && !strings.Contains(err.Error(), tt.words) {
			t.Errorf("%s: refused with %q, want the words %q", tt.name, err.Error(), tt.words)
		}
		if msg := strings.ToLower(err.Error()); tt.reason == blockedScrollOff && (strings.Contains(msg, "cone") || strings.Contains(msg, "cold") || strings.Contains(msg, spellConeOfCold)) {
			t.Errorf("%s: the refusal %q names the spell", tt.name, err)
		}
	}
	// Not a scroll, and not the character's: no scroll is read, no matter how the call is made.
	_, err := s.read(t, s.ana, s.pens, potionCommon, scrollD20(10))
	wantCode(t, "a potion", err, connect.CodeInvalidArgument)
	_, err = s.read(t, s.caio, s.pens, scrollFirst)
	wantCode(t, "another player's scroll", err, connect.CodePermissionDenied)
	_, err = s.read(t, s.ana, s.pens, scrollFirst, func(r *playv1.ReadScrollRequest) { r.IdempotencyKey = "" })
	wantCode(t, "no key", err, connect.CodeInvalidArgument)
	s.nothingSpent(t, "the refused reads", scrollFirst, scrollFourth, scrollFifth, scrollSixth, potionCommon)
}

// In a running combat the read needs the reader's turn and the economy of the spell's casting time, as a cast does; the master has the last
// word on the economy and not on whose turn it is.
func TestAReadInACombatNeedsTheReadersTurnAndTheEconomyOfTheCastingTime(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	f.scrolls(t)
	_, err := f.read(t, f.ana, f.pens, scrollThird, scrollD20(5))
	wantBlockedBy(t, "a read off turn", err, blockedNotYourTurn)
	_, err = f.read(t, f.master, f.pens, scrollThird, scrollD20(5))
	wantBlockedBy(t, "the master's read off turn", err, blockedNotYourTurn)
	f.nothingSpent(t, "the reads off turn", scrollThird)

	f.mustEndTurn(t, f.caio, f.e) // Pensantus's turn
	f.mustUse(t, f.ana, f.pens, potionCommon)
	_, err = f.read(t, f.ana, f.pens, scrollThird, scrollD20(5))
	wantBlockedBy(t, "a read with the action used", err, blockedActionUsed)
	if !f.holds(t, f.pens, scrollThird) || f.countEvents(t, eventConsumableUsed) != 1 { // the potion's
		t.Error("the refused read spent the scroll")
	}
	// The master has the last word on the economy: his read stands with the action spent, and a failure leaves it spent.
	if got := f.mustRead(t, f.master, f.pens, scrollThird, scrollD20(5)); got.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_FAILED {
		t.Errorf("the master's read reads %v, want the failure", got)
	}
	// The bonus action of Misty Step is free: the read of a scroll of it is not stopped by the action.
	if got := f.mustRead(t, f.ana, f.pens, scrollSecond, scrollD20(20)); got.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_PASSED {
		t.Errorf("the bonus action scroll reads %v with the action used, want a pass", got)
	}
}

func TestAFailedCheckInACombatSpendsTheEconomyOfTheCastingTimeAndAPassSpendsNone(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t)
	f.scrolls(t)
	f.mustEndTurn(t, f.caio, f.e) // Pensantus's turn
	spent := func() (action, bonus bool) {
		c := byLabel(t, f.get(t, f.master), "Pensantus")
		return c.GetActionUsed(), c.GetBonusActionUsed()
	}
	if got := f.mustRead(t, f.ana, f.pens, scrollThird, scrollD20(13)); got.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_PASSED {
		t.Fatalf("the read reads %v, want a pass", got)
	}
	if action, bonus := spent(); action || bonus {
		t.Errorf("the pass spent the action (%v) or the bonus action (%v)", action, bonus)
	}
	// A failed read of a spell with a casting time of a bonus action pays the bonus action, and of an action pays the action.
	second := f.entryOf(t, f.pens, scrollSecond)
	f.mustRead(t, f.ana, f.pens, scrollSecond, scrollD20(1))
	if action, bonus := spent(); action || !bonus || f.holds(t, f.pens, scrollSecond) {
		t.Errorf("after the failed read of Misty Step: action %v, bonus action %v, scroll held %v; want only the bonus action spent and the scroll gone", action, bonus, f.holds(t, f.pens, scrollSecond))
	}
	_, err := f.read(t, f.ana, f.pens, scrollSecond, scrollD20(1), func(r *playv1.ReadScrollRequest) { r.EntryId = second })
	wantBlockedBy(t, "a second bonus action read", err, blockedItemGone)
	f.mustRead(t, f.ana, f.pens, scrollThird, scrollD20(1))
	if action, bonus := spent(); !action || !bonus || f.holds(t, f.pens, scrollThird) {
		t.Errorf("after the failed read of Fireball: action %v, bonus action %v; want both spent and the scroll gone", action, bonus)
	}
	_, err = f.read(t, f.ana, f.pens, scrollFirst)
	wantBlockedBy(t, "a read with the action spent by the failure", err, blockedActionUsed)
	if !f.holds(t, f.pens, scrollFirst) {
		t.Error("the refused read spent the scroll")
	}
}

// Out of a running combat nothing of the turn is spent: a combat that has not begun has no turn, a character out of the combat has none.
func TestAFailedCheckOutOfARunningCombatSpendsNoEconomy(t *testing.T) {
	t.Parallel()
	s := newStocked(t)
	s.scrolls(t)
	s.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: s.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1}, reveal: []string{"Goblin"},
		at:    map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Goblin": {12, 8}},
		setup: true,
	})
	if got := s.mustRead(t, s.ana, s.pens, scrollThird, scrollD20(1)); got.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_FAILED || !got.GetEntryGone() {
		t.Fatalf("the read in a combat that has not begun reads %v, want the failure", got)
	}
	if c := byLabel(t, s.get(t, s.master), "Pensantus"); c.GetActionUsed() || c.GetBonusActionUsed() || c.GetReactionUsed() {
		t.Errorf("a combat that has not begun lost the economy of Pensantus: %v", c)
	}
	if s.holds(t, s.pens, scrollThird) {
		t.Error("the scroll is still on the sheet")
	}
}

// A pass lapses when the reader's turn ends, and when the reader reads again, whatever the result; a pass out of a combat lasts until the
// next read.
func TestAPassLapsesAtTheEndOfTheReadersTurnAndOnAnotherRead(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t)
	f.scrolls(t)
	f.mustEndTurn(t, f.caio, f.e) // Pensantus's turn
	first := f.mustRead(t, f.ana, f.pens, scrollFirst)
	if pass, ok := f.passOf(t, f.pens); !ok || pass.Entry != first.GetEntryId() || pass.Encounter != f.e.GetId() {
		t.Fatalf("the pass = %+v, %v; want the 1st level scroll in this combat", pass, ok)
	}
	cantrip := f.mustRead(t, f.ana, f.pens, scrollCantrip)
	if pass, ok := f.passOf(t, f.pens); !ok || pass.Entry != cantrip.GetEntryId() {
		t.Errorf("the pass = %+v, %v after a second read; want the cantrip's, the first has lapsed", pass, ok)
	}
	f.mustRead(t, f.master, f.pens, scrollThird, scrollD20(1)) // a failed read is a read: it lapses the pass, and the master reads for the character
	if pass, ok := f.passOf(t, f.pens); ok {
		t.Errorf("the pass = %+v after a failed read, want none", pass)
	}
}

// The combat ending lapses the pass of the turn it was read in.
func TestAPassLapsesWhenTheCombatEnds(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t)
	f.scrolls(t)
	f.mustEndTurn(t, f.caio, f.e) // Pensantus's turn
	f.mustRead(t, f.ana, f.pens, scrollFirst)
	if _, ok := f.passOf(t, f.pens); !ok {
		t.Fatal("no pass in the turn of the read")
	}
	if _, err := f.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: f.campaignID, EncounterId: f.e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	if pass, ok := f.passOf(t, f.pens); ok {
		t.Errorf("the pass = %+v after the combat ended, want none", pass)
	}
}

// A spell cast with a bonus action leaves no other spell of the turn but a cantrip of 1 action: the read of a scroll of one is refused
// with nothing spent, and the cantrip's is not.
func TestAReadAfterABonusActionSpellKeepsTheSpellLimit(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t)
	f.scrolls(t)
	f.mustEndTurn(t, f.caio, f.e) // Pensantus's turn
	f.execSQL(t, `UPDATE combatants SET spell_cast = false, bonus_spell_cast = true WHERE id = $1`, f.id(t, "Pensantus"))
	_, err := f.read(t, f.ana, f.pens, scrollFirst)
	wantBlockedBy(t, "a leveled spell after a bonus action spell", err, blockedBonusLimit)
	f.nothingSpent(t, "the refused read", scrollFirst)
	if got := f.mustRead(t, f.ana, f.pens, scrollCantrip); got.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_NO_CHECK_NEEDED {
		t.Errorf("the cantrip's read reads %v after a bonus action spell, want it read", got)
	}
}

// The master has the last word on the economy and none on whose turn it is: his read for an NPC combatant off its turn is refused.
func TestTheMastersReadForAnNPCOffItsTurnIsRefused(t *testing.T) {
	t.Parallel()
	s := newStocked(t)
	mage := s.castCharEnemy(t, "Mago Sombrio")
	mage = s.withItems(t, mage, scrollFifth)
	s.putSpell(t, mage, scrollFifth, spellConeOfCold)
	s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}, {CharacterId: mage.GetId()}},
		npcRolls: []int{3, 2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin", "Mago Sombrio"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Goblin": {12, 8}, "Mago Sombrio": {13, 9}},
	})
	_, err := s.read(t, s.master, mage, scrollFifth, scrollD20(2))
	wantBlockedBy(t, "the master's read for an NPC off its turn", err, blockedNotYourTurn)
	if !s.holds(t, mage, scrollFifth) {
		t.Error("the refused read spent the mage's scroll")
	}
}

func TestAPassOfTheTurnLapsesWhenTheTurnEnds(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t)
	f.scrolls(t)
	f.mustEndTurn(t, f.caio, f.e) // Pensantus's turn
	f.mustRead(t, f.ana, f.pens, scrollFirst)
	if _, ok := f.passOf(t, f.pens); !ok {
		t.Fatal("no pass in the turn of the read")
	}
	f.mustEndTurn(t, f.ana, f.e)
	if pass, ok := f.passOf(t, f.pens); ok {
		t.Errorf("the pass = %+v after the turn ended, want none", pass)
	}
	// Out of a combat there is no turn: the pass stays until the next read.
	s := newStocked(t)
	s.scrolls(t)
	s.mustRead(t, s.ana, s.pens, scrollFirst)
	s.mustRead(t, s.ana, s.pens, scrollFirst)
	if pass, ok := s.passOf(t, s.pens); !ok || pass.Encounter != "" {
		t.Errorf("the pass out of a combat = %+v, %v; want one with no combat", pass, ok)
	}
}

func TestTheRetryOfAReadAnswersTheFirstOneAndSpendsNothingAgain(t *testing.T) {
	t.Parallel()
	s := newStocked(t)
	s.scrolls(t)
	same := func(key, entry string, face int32) func(*playv1.ReadScrollRequest) {
		return func(r *playv1.ReadScrollRequest) {
			r.IdempotencyKey, r.EntryId, r.Roll = key, entry, &playv1.ReadScrollRequest_D20Face{D20Face: face}
		}
	}
	key, entry := newKey(), s.entryOf(t, s.pens, scrollThird)
	failed := s.mustRead(t, s.ana, s.pens, scrollThird, same(key, entry, 3))
	again, err := s.read(t, s.ana, s.pens, scrollThird, same(key, entry, 3))
	if err != nil || again.String() != failed.String() || s.countEvents(t, eventConsumableUsed) != 1 {
		t.Errorf("the retry of the failed read answered %v, %v after %v, with %d events", again, err, failed, s.countEvents(t, eventConsumableUsed))
	}
	passKey, passEntry := newKey(), s.entryOf(t, s.pens, scrollFirst)
	passed := s.mustRead(t, s.ana, s.pens, scrollFirst, func(r *playv1.ReadScrollRequest) { r.IdempotencyKey, r.EntryId = passKey, passEntry })
	again, err = s.read(t, s.ana, s.pens, scrollFirst, func(r *playv1.ReadScrollRequest) { r.IdempotencyKey, r.EntryId = passKey, passEntry })
	if err != nil || again.String() != passed.String() || s.countEvents(t, consumable.KindScrollRead) != 1 {
		t.Errorf("the retry of the pass answered %v, %v after %v, with %d events", again, err, passed, s.countEvents(t, consumable.KindScrollRead))
	}
	// The key of one read is no key for another entry, nor for a use of a unit.
	_, err = s.read(t, s.ana, s.pens, scrollCantrip, func(r *playv1.ReadScrollRequest) { r.IdempotencyKey = passKey })
	wantCode(t, "a key reused for another entry", err, connect.CodeInvalidArgument)
	_, err = s.use(t, s.ana, s.pens, scrollCantrip, func(r *playv1.UseConsumableRequest) { r.IdempotencyKey = key })
	wantCode(t, "a key reused for a use of a unit", err, connect.CodeInvalidArgument)
}

// RN-23: the retry of a failed read answers the spell to no player once the master has switched it off, and the master still reads it.
func TestTheRetryOfAFailedReadNamesNoSwitchedOffSpellToTheOwner(t *testing.T) {
	t.Parallel()
	s := newStocked(t)
	s.scrolls(t)
	key, entry := newKey(), s.entryOf(t, s.pens, scrollThird)
	same := func(r *playv1.ReadScrollRequest) {
		r.IdempotencyKey, r.EntryId, r.Roll = key, entry, &playv1.ReadScrollRequest_D20Face{D20Face: 3}
	}
	failed := s.mustRead(t, s.ana, s.pens, scrollThird, same)
	if failed.GetSpellKey() != spellFireball || failed.GetSpellNamePt() == "" {
		t.Fatalf("the failed read answers %v, want the spell", failed)
	}
	if _, err := s.master.table.SetOptionSwitches(t.Context(), connect.NewRequest(&rulesv1.SetOptionSwitchesRequest{
		CampaignId: s.campaignID, Switches: []*rulesv1.OptionSwitch{{Key: spellFireball, Off: true}},
	})); err != nil {
		t.Fatalf("SetOptionSwitches() error = %v", err)
	}
	events := s.countEvents(t, eventConsumableUsed)
	owner, err := s.read(t, s.ana, s.pens, scrollThird, same)
	if err != nil || owner.GetSpellKey() != "" || owner.GetSpellNamePt() != "" || owner.GetCheckDc() != failed.GetCheckDc() || owner.GetTotal() != failed.GetTotal() {
		t.Errorf("the owner's retry answers %v, %v; want the check and no spell", owner, err)
	}
	master, err := s.read(t, s.master, s.pens, scrollThird, same)
	if err != nil || master.GetSpellKey() != spellFireball || master.GetSpellNamePt() == "" {
		t.Errorf("the master's retry answers %v, %v; want the spell still", master, err)
	}
	if got := s.countEvents(t, eventConsumableUsed); got != events {
		t.Errorf("the retries wrote %d events", got-events)
	}
}

// The failure closes the undo chain as every use of a consumable does (consumable_used is no undoable kind), and so does the pass
// (scroll_read is none either): the master's undo has nothing to undo after either.
func TestAReadClosesTheUndoChain(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		scroll string
		face   int32
	}{"a failed check": {scrollThird, 1}, "a pass": {scrollThird, 20}, "a read that needs no check": {scrollFirst, 0}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newPotionFight(t)
			f.scrolls(t)
			f.mustEndTurn(t, f.caio, f.e) // Pensantus's turn
			if _, err := f.adjustHP(t, f.e, "Goblin", healHP(1)); err != nil {
				t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
			}
			if f.log(t, f.master, f.e).GetUndoableEventId() == "" {
				t.Fatal("the heal of the goblin is not undoable")
			}
			if tc.face == 0 {
				f.mustRead(t, f.ana, f.pens, tc.scroll)
			} else {
				f.mustRead(t, f.ana, f.pens, tc.scroll, scrollD20(tc.face))
			}
			if got := f.log(t, f.master, f.e).GetUndoableEventId(); got != "" {
				t.Errorf("the undoable event = %q after the read, want none: the read closes the chain", got)
			}
		})
	}
}

// consumeLine is the consumable line a user reads in the log of the combat, nil when it reads none.
func (f *potionFight) consumeLines(t *testing.T, u *user) []*playv1.CombatLogEntry {
	t.Helper()
	var out []*playv1.CombatLogEntry
	for _, r := range f.log(t, u, f.e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_CONSUMABLE_USED {
				out = append(out, en)
			}
		}
	}
	return out
}

// The log of a failed read: the master and the owner read the spell and the check; any other player reads that the scroll came apart and
// nothing of what was on it; a read that passed is no line (the cast's own will be).
func TestTheLogLineOfAFailedReadTellsTheOwnersTheSpellAndTheCheck(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t)
	f.scrolls(t)
	f.mustEndTurn(t, f.caio, f.e) // Pensantus's turn
	f.mustRead(t, f.ana, f.pens, scrollFirst)
	for who, u := range map[string]*user{"the master": f.master, "the owner": f.ana, "another player": f.caio} {
		if lines := f.consumeLines(t, u); len(lines) != 0 {
			t.Errorf("%s reads %d lines for a read that passed, want none", who, len(lines))
		}
	}
	failed := f.mustRead(t, f.ana, f.pens, scrollThird, scrollD20(6))
	want := &playv1.ScrollReadLogLine{SpellKey: spellFireball, SpellNamePt: failed.GetSpellNamePt(), ScrollLevel: 3, CheckDc: 13, D20: 6, Modifier: 4, Total: 10}
	for who, u := range map[string]*user{"the master": f.master, "the owner": f.ana} {
		lines := f.consumeLines(t, u)
		if len(lines) != 1 || lines[0].GetConsumableUse().GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_SCROLL_FAILED ||
			lines[0].GetConsumableUse().GetScroll().String() != want.String() || lines[0].GetActorLabel() != "Pensantus" || lines[0].GetConsumableUse().GetItemNamePt() == "" {
			t.Errorf("%s reads %v, want Pensantus's failed scroll with %v", who, lines, want)
		}
	}
	for who, u := range map[string]*user{"Toren's player": f.caio, "Brisa's player": f.bia} {
		lines := f.consumeLines(t, u)
		if len(lines) != 1 || lines[0].GetConsumableUse().GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_SCROLL_FAILED || lines[0].GetActorLabel() != "Pensantus" ||
			lines[0].GetConsumableUse().GetItemNamePt() == "" {
			t.Fatalf("%s reads %v, want the line that Pensantus's scroll came apart", who, lines)
		}
		text, err := protojson.Marshal(lines[0])
		if err != nil {
			t.Fatalf("marshal the line: %v", err)
		}
		for _, leak := range []string{"fireball", "Bola", "checkDc", "d20", "modifier", "total", "rollTotal"} {
			if strings.Contains(strings.ToLower(string(text)), strings.ToLower(leak)) {
				t.Errorf("%s reads %s in %s", who, leak, text)
			}
		}
		if lines[0].GetConsumableUse().GetScroll() != nil || lines[0].GetConsumableUse().GetRollTotal() != 0 {
			t.Errorf("%s reads the scroll or a total: %v", who, lines[0].GetConsumableUse())
		}
	}
}

// RN-23: after the master switches the spell off, the line of the earlier read names no spell to the owner, and the master still reads it.
func TestTheLogLineOfAnEarlierReadNamesNoSwitchedOffSpellToTheOwner(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t)
	f.scrolls(t)
	f.mustEndTurn(t, f.caio, f.e)
	f.mustRead(t, f.ana, f.pens, scrollThird, scrollD20(6))
	if _, err := f.master.table.SetOptionSwitches(t.Context(), connect.NewRequest(&rulesv1.SetOptionSwitchesRequest{
		CampaignId: f.campaignID, Switches: []*rulesv1.OptionSwitch{{Key: spellFireball, Off: true}},
	})); err != nil {
		t.Fatalf("SetOptionSwitches() error = %v", err)
	}
	owner := f.consumeLines(t, f.ana)
	if len(owner) != 1 || owner[0].GetConsumableUse().GetScroll() == nil {
		t.Fatalf("the owner reads %v, want the line with its check", owner)
	}
	if got := owner[0].GetConsumableUse().GetScroll(); got.GetSpellKey() != "" || got.GetSpellNamePt() != "" || got.GetCheckDc() != 13 || got.GetTotal() != 10 {
		t.Errorf("the owner reads %v after the switch-off, want the check and no spell", got)
	}
	master := f.consumeLines(t, f.master)
	if len(master) != 1 || master[0].GetConsumableUse().GetScroll().GetSpellKey() != spellFireball || master[0].GetConsumableUse().GetScroll().GetSpellNamePt() == "" {
		t.Errorf("the master reads %v, want the spell still", master)
	}
}

// A hidden caster's read tells no player anything: the master reads the line, the players read none.
func TestAHiddenCastersFailedReadIsNoPlayersLine(t *testing.T) {
	t.Parallel()
	s := newStocked(t)
	mage := s.castCharEnemy(t, "Mago Sombrio")
	mage = s.withItems(t, mage, scrollFifth)
	s.putSpell(t, mage, scrollFifth, spellConeOfCold)
	e := s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}, {CharacterId: mage.GetId()}},
		npcRolls: []int{3, 30},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin", "Mago Sombrio"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Goblin": {12, 8}, "Mago Sombrio": {13, 9}},
	})
	if _, err := s.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: s.campaignID, EncounterId: e.GetId(), CombatantId: s.id(t, "Mago Sombrio"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	f := &potionFight{stocked: s, e: e}
	if got := s.mustRead(t, s.master, mage, scrollFifth, scrollD20(2)); got.GetOutcome() != playv1.ScrollReadOutcome_SCROLL_READ_OUTCOME_FAILED {
		t.Fatalf("the master's read for the hidden mage reads %v, want the failure", got)
	}
	if lines := f.consumeLines(t, s.master); len(lines) != 1 || lines[0].GetConsumableUse().GetScroll().GetSpellKey() != spellConeOfCold {
		t.Errorf("the master reads %v, want the line of the hidden mage's scroll", lines)
	}
	for who, u := range map[string]*user{"Toren's player": s.caio, "Pensantus's player": s.ana, "Brisa's player": s.bia} {
		if lines := f.consumeLines(t, u); len(lines) != 0 {
			t.Errorf("%s reads %v of a hidden caster's read, want nothing", who, lines)
		}
	}
}
