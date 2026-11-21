package play

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// The charges of magic items and the ammunition at the table (ChargesService): who may spend, roll and correct, what
// each refusal says, the retry of a key, the dice of RN-18, the combat log line only the master and the owner read, and
// the recovery of the pieces a combat fired.

const (
	chargeWand   = "item:wand-of-magic-missiles" // 7 charges, a d20 on the last: a 1 destroys it
	chargeStaff  = "item:staff-of-power"         // 20 charges, a d20 on the last: a 1 keeps its bonus, a 20 gives 1d8+2 back
	chargeScarab = "item:scarab-of-protection"   // 12 charges, lost at once with the last
	chargeLuck   = "item:luck-blade"             // the SRD rolls its charges: none until the master sets them
	chargeBag    = "item:bag-of-holding"         // no charges
	arrowAmmo    = "equipment:arrow"
)

// charged is newArmed with the items on Pensantus's sheet, put there by the master after the session locked it.
type charged struct {
	*armed
}

func newCharged(t *testing.T, keys ...string) *charged {
	t.Helper()
	a := newArmed(t)
	a.pens = a.withItems(t, a.pens, keys...)
	return &charged{a}
}

// withItems puts items on the character's sheet as the master.
func (a *armed) withItems(t *testing.T, c *charactersv1.Character, keys ...string) *charactersv1.Character {
	t.Helper()
	cur, err := a.master.characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: a.campaignID, CharacterId: c.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	sheet := proto.CloneOf(cur.Msg.GetCharacter().GetSheet())
	for _, k := range keys {
		sheet.GetFull().MagicItems = append(sheet.GetFull().MagicItems, &charactersv1.SheetMagicItem{Key: k})
	}
	res, err := a.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: a.campaignID, CharacterId: c.GetId(), Revision: cur.Msg.GetCharacter().GetRevision(), Name: c.GetName(), Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("UpdateCharacter(items) error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// entry is the id of the item on Pensantus's sheet.
func (c *charged) entry(t *testing.T, key string) string {
	t.Helper()
	cur, err := c.master.characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: c.campaignID, CharacterId: c.pens.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	for _, it := range cur.Msg.GetCharacter().GetSheet().GetFull().GetMagicItems() {
		if it.GetKey() == key {
			return it.GetId()
		}
	}
	t.Fatalf("%s is not on the sheet", key)
	return ""
}

// itemOf reads the item as u reads the sheet.
func (c *charged) itemOf(t *testing.T, u *user, key string) *charactersv1.SheetMagicItem {
	t.Helper()
	res, err := u.characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: c.campaignID, CharacterId: c.pens.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	for _, it := range res.Msg.GetCharacter().GetSheet().GetFull().GetMagicItems() {
		if it.GetKey() == key {
			return it
		}
	}
	t.Fatalf("%s is not on the sheet", key)
	return nil
}

func (c *charged) spend(u *user, key string, amount int32, edit ...func(*playv1.SpendChargesRequest)) func(t *testing.T) (*playv1.SpendChargesResponse, error) {
	return func(t *testing.T) (*playv1.SpendChargesResponse, error) {
		t.Helper()
		req := &playv1.SpendChargesRequest{CampaignId: c.campaignID, CharacterId: c.pens.GetId(), EntryId: c.entry(t, key), Amount: amount, IdempotencyKey: newKey()}
		for _, e := range edit {
			e(req)
		}
		res, err := u.charges.SpendCharges(t.Context(), connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
}

func (c *charged) mustSpend(t *testing.T, u *user, key string, amount int32) *charactersv1.SheetMagicItem {
	t.Helper()
	res, err := c.spend(u, key, amount)(t)
	if err != nil {
		t.Fatalf("SpendCharges(%s, %d) error = %v", key, amount, err)
	}
	return res.GetItem()
}

func (c *charged) roll(t *testing.T, u *user, key string, edit ...func(*playv1.RollLastChargeRequest)) (*playv1.RollLastChargeResponse, error) {
	t.Helper()
	req := &playv1.RollLastChargeRequest{
		CampaignId: c.campaignID, CharacterId: c.pens.GetId(), EntryId: c.entry(t, key), IdempotencyKey: newKey(),
		Roll: &playv1.RollLastChargeRequest_RollInApp{RollInApp: true},
	}
	for _, e := range edit {
		e(req)
	}
	res, err := u.charges.RollLastCharge(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func lastChargeD20(face int32) func(*playv1.RollLastChargeRequest) {
	return func(r *playv1.RollLastChargeRequest) { r.Roll = &playv1.RollLastChargeRequest_D20Face{D20Face: face} }
}

func (c *charged) correct(t *testing.T, u *user, key string, left int32, lost charactersv1.ItemLoss, edit ...func(*playv1.SetChargesRequest)) (*playv1.SetChargesResponse, error) {
	t.Helper()
	req := &playv1.SetChargesRequest{CampaignId: c.campaignID, CharacterId: c.pens.GetId(), EntryId: c.entry(t, key), ChargesLeft: left, Lost: lost, IdempotencyKey: newKey()}
	for _, e := range edit {
		e(req)
	}
	res, err := u.charges.SetCharges(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func wantChargeBlock(t *testing.T, call string, err error, want charactersv1.CharacterBlockedReason) {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("%s: %v is not a connect error", call, err)
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*charactersv1.CharacterBlocked); ok && b.GetReason() == want {
				return
			}
		}
	}
	t.Fatalf("%s error = %v, want CharacterBlocked %v", call, err, want)
}

func wantChargesBlocked(t *testing.T, call string, err error, want playv1.ChargesBlockedReason) *playv1.ChargesBlocked {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("%s: %v is not a connect error", call, err)
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*playv1.ChargesBlocked); ok && b.GetReason() == want {
				return b
			}
		}
	}
	t.Fatalf("%s error = %v, want ChargesBlocked %v", call, err, want)
	return nil
}

// countEvents counts the session's events of a kind.
func (a *armed) countEvents(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1 AND game_session_id IN (SELECT id FROM game_sessions WHERE campaign_id = $2)`, kind, a.campaignID).Scan(&n); err != nil {
		t.Fatalf("count the %s events: %v", kind, err)
	}
	return n
}

func TestSpendingChargesTakesThemForTheOwnerAndTheMaster(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	if got := c.mustSpend(t, c.ana, chargeWand, 2); got.GetChargesLeft() != 5 || got.GetChargesMax() != 7 || got.GetLastRollPending() {
		t.Errorf("the owner's spend left %v, want 5 of 7", got)
	}
	if got := c.mustSpend(t, c.master, chargeWand, 1); got.GetChargesLeft() != 4 {
		t.Errorf("the master's spend left %v, want 4", got)
	}
	for _, u := range []*user{c.ana, c.master} {
		if got := c.itemOf(t, u, chargeWand).GetChargesLeft(); got != 4 {
			t.Errorf("a read of the sheet shows %d charges, want 4", got)
		}
	}
	if n := c.countEvents(t, eventItemChargeSpent); n != 2 {
		t.Errorf("the session has %d item_charge_spent events, want 2", n)
	}
}

func TestSpendingTheLastChargeLosesAScarabAndWaitsTheD20OfAWand(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand, chargeScarab)
	got := c.mustSpend(t, c.ana, chargeWand, 7)
	if got.GetChargesLeft() != 0 || !got.GetLastRollPending() || got.GetLost() != charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED {
		t.Errorf("the wand's last charge left %v, want 0 left and the d20 pending", got)
	}
	got = c.mustSpend(t, c.ana, chargeScarab, 12)
	if got.GetLost() != charactersv1.ItemLoss_ITEM_LOSS_GONE || got.GetLastRollPending() {
		t.Errorf("the scarab's last charge left %v, want it lost at once", got)
	}
	if n := c.countEvents(t, eventItemDestroyed); n != 1 {
		t.Errorf("the session has %d item_destroyed events, want 1 (the scarab)", n)
	}
}

func TestSpendChargesRefusals(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand, chargeScarab, chargeLuck, chargeBag)
	_, err := c.spend(c.bia, chargeWand, 1)(t)
	wantCode(t, "another player's SpendCharges", err, connect.CodePermissionDenied)
	_, err = c.spend(c.ana, chargeWand, 0)(t)
	wantCode(t, "an amount of 0", err, connect.CodeInvalidArgument)
	_, err = c.spend(c.ana, chargeBag, 1)(t)
	wantCode(t, "an item with no charges", err, connect.CodeInvalidArgument)
	_, err = c.spend(c.ana, chargeWand, 8)(t)
	wantChargeBlock(t, "more than left", err, charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARGES_NOT_ENOUGH)
	_, err = c.spend(c.ana, chargeLuck, 1)(t)
	wantChargeBlock(t, "a blade with no count", err, charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ITEM_NO_CHARGES)
	_, err = c.spend(c.ana, chargeWand, 1, func(r *playv1.SpendChargesRequest) { r.EntryId = "m99" })(t)
	wantCode(t, "an entry that is not on the sheet", err, connect.CodeNotFound)
	_, err = c.spend(c.ana, chargeWand, 1, func(r *playv1.SpendChargesRequest) { r.CharacterId = newKey() })(t)
	wantCode(t, "a character that is not there", err, connect.CodeNotFound)
	_, err = c.spend(c.ana, chargeWand, 1, func(r *playv1.SpendChargesRequest) { r.IdempotencyKey = "no" })(t)
	wantCode(t, "a key that is no UUID", err, connect.CodeInvalidArgument)

	c.mustSpend(t, c.ana, chargeWand, 7)
	_, err = c.spend(c.ana, chargeWand, 1)(t)
	wantChargeBlock(t, "an item that waits for its d20", err, charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ITEM_NO_CHARGES)
	c.mustSpend(t, c.ana, chargeScarab, 12)
	_, err = c.spend(c.ana, chargeScarab, 1)(t)
	wantChargeBlock(t, "a destroyed item", err, charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ITEM_DESTROYED)

	if _, err := c.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: c.campaignID, CharacterId: c.pens.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	for _, u := range []*user{c.ana, c.master} {
		_, err = c.spend(u, chargeLuck, 1)(t)
		wantChargeBlock(t, "a dead character's SpendCharges", err, charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD)
	}
}

func TestSpendingChargesWorksOnALockedSheetAndOutsideAnyCombat(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	if got := c.itemOf(t, c.ana, chargeWand); got.GetChargesLeft() != 7 {
		t.Fatalf("the wand starts with %v, want 7", got)
	}
	c.mustSpend(t, c.ana, chargeWand, 1) // the session started: the sheet is locked for Ana, and there is no combat
}

func TestSpendChargesIsIdempotent(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	key := newKey()
	withKey := func(r *playv1.SpendChargesRequest) { r.IdempotencyKey = key }
	first, err := c.spend(c.ana, chargeWand, 2, withKey)(t)
	if err != nil {
		t.Fatalf("SpendCharges() error = %v", err)
	}
	again, err := c.spend(c.ana, chargeWand, 2, withKey)(t)
	if err != nil {
		t.Fatalf("SpendCharges() retry error = %v", err)
	}
	if again.GetItem().GetChargesLeft() != first.GetItem().GetChargesLeft() || again.GetItem().GetChargesLeft() != 5 {
		t.Errorf("the retry left %v after %v, want 5 left both times", again.GetItem(), first.GetItem())
	}
	if n := c.countEvents(t, eventItemChargeSpent); n != 1 {
		t.Errorf("the session has %d item_charge_spent events, want 1", n)
	}
	_, err = c.spend(c.ana, chargeWand, 1, withKey)(t)
	wantCode(t, "the key reused for another amount", err, connect.CodeInvalidArgument)
}

// Two spends of the last charge at once: one wins, the other is refused as if it came after.
func TestTwoSpendsOfTheLastChargeGiveOneSuccess(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	if _, err := c.correct(t, c.master, chargeWand, 1, charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED); err != nil {
		t.Fatalf("SetCharges() error = %v", err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, u := range []*user{c.ana, c.master} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = c.spend(u, chargeWand, 1)(t)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("the two spends of the last charge = %v, %v; want exactly one success", errs[0], errs[1])
	}
	if n := c.countEvents(t, eventItemChargeSpent); n != 1 {
		t.Errorf("the session has %d item_charge_spent events, want 1", n)
	}
}

func TestRollingTheLastChargeOfAWand(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	_, err := c.roll(t, c.ana, chargeWand)
	wantCode(t, "a roll before the last charge", err, connect.CodeInvalidArgument)
	c.mustSpend(t, c.ana, chargeWand, 7)
	_, err = c.roll(t, c.bia, chargeWand)
	wantCode(t, "another player's roll", err, connect.CodePermissionDenied)
	_, err = c.roll(t, c.ana, chargeWand, lastChargeD20(21))
	wantCode(t, "a d20 of 21", err, connect.CodeInvalidArgument)
	_, err = c.roll(t, c.ana, chargeWand, func(r *playv1.RollLastChargeRequest) { r.Roll = nil })
	wantCode(t, "no roll", err, connect.CodeInvalidArgument)

	c.h.roller.queue(1)
	res, err := c.roll(t, c.ana, chargeWand)
	if err != nil {
		t.Fatalf("RollLastCharge() error = %v", err)
	}
	if res.GetD20() != 1 || res.GetLost() != charactersv1.ItemLoss_ITEM_LOSS_GONE || res.GetItem().GetLost() != charactersv1.ItemLoss_ITEM_LOSS_GONE || res.GetItem().GetLastRollPending() {
		t.Errorf("a 1 = %v, want the wand lost and the wait over", res)
	}
	if n := c.countEvents(t, eventItemDestroyed); n != 1 {
		t.Errorf("the session has %d item_destroyed events, want 1", n)
	}
	_, err = c.roll(t, c.ana, chargeWand)
	wantCode(t, "a second roll", err, connect.CodeInvalidArgument)
}

func TestAMiddleFaceOnTheLastChargeOnlyEndsTheWait(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	c.mustSpend(t, c.ana, chargeWand, 7)
	c.h.roller.queue(12)
	res, err := c.roll(t, c.master, chargeWand)
	if err != nil {
		t.Fatalf("RollLastCharge() error = %v", err)
	}
	if res.GetD20() != 12 || res.GetLost() != charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED || res.GetItem().GetLastRollPending() || res.GetItem().GetChargesLeft() != 0 || res.GetRegained() != 0 {
		t.Errorf("a 12 = %v, want the wait over and nothing else", res)
	}
	if n := c.countEvents(t, eventItemChargeSpent); n != 2 {
		t.Errorf("the session has %d item_charge_spent events, want 2 (the spend and the roll)", n)
	}
}

func TestATwentyOnAStaffOfPowerGivesChargesBack(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeStaff)
	c.mustSpend(t, c.ana, chargeStaff, 20)
	c.h.roller.queue(20, 4) // the d20, then the d8 of 1d8 + 2
	res, err := c.roll(t, c.ana, chargeStaff)
	if err != nil {
		t.Fatalf("RollLastCharge() error = %v", err)
	}
	if res.GetD20() != 20 || res.GetRegained() != 6 || res.GetRegainFace() != 4 || res.GetItem().GetChargesLeft() != 6 || res.GetItem().GetLastRollPending() {
		t.Errorf("a 20 = %v, want 6 charges back (4 + 2)", res)
	}
	if n := c.countEvents(t, eventItemChargesRestored); n != 1 {
		t.Errorf("the session has %d item_charges_restored events, want 1", n)
	}
}

func TestAOneOnAStaffOfPowerKeepsItsBonus(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeStaff)
	c.mustSpend(t, c.ana, chargeStaff, 20)
	c.h.roller.queue(1)
	res, err := c.roll(t, c.ana, chargeStaff)
	if err != nil {
		t.Fatalf("RollLastCharge() error = %v", err)
	}
	if res.GetLost() != charactersv1.ItemLoss_ITEM_LOSS_PARTLY_MAGIC || res.GetItem().GetLost() != charactersv1.ItemLoss_ITEM_LOSS_PARTLY_MAGIC {
		t.Errorf("a 1 = %v, want the staff partly magic", res)
	}
}

// RN-18: a table of real dice types the d20 (and the regain die of a 20); a table of app dice refuses a typed face.
func TestTheDiceOfTheLastChargeFollowTheTablesDiceMode(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeStaff)
	c.mustSpend(t, c.ana, chargeStaff, 20)
	c.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	_, err := c.roll(t, c.ana, chargeStaff)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE)
	_, err = c.roll(t, c.ana, chargeStaff, lastChargeD20(20))
	blocked := wantChargesBlocked(t, "a typed 20 without the regain die", err, playv1.ChargesBlockedReason_CHARGES_BLOCKED_REASON_REGAIN_FACE_NEEDED)
	if blocked.GetNeeded() != 8 {
		t.Errorf("the die to type = d%d, want d8", blocked.GetNeeded())
	}
	_, err = c.roll(t, c.ana, chargeStaff, lastChargeD20(20), func(r *playv1.RollLastChargeRequest) { r.TypedRegainFace = 9 })
	wantCode(t, "a regain face off the die", err, connect.CodeInvalidArgument)
	res, err := c.roll(t, c.ana, chargeStaff, lastChargeD20(20), func(r *playv1.RollLastChargeRequest) { r.TypedRegainFace = 3 })
	if err != nil || res.GetRegained() != 5 || res.GetItem().GetChargesLeft() != 5 {
		t.Fatalf("a typed 20 with a 3 = %v, %v, want 5 charges back", res, err)
	}

	d := newCharged(t, chargeWand)
	d.mustSpend(t, d.ana, chargeWand, 7)
	d.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	_, err = d.roll(t, d.ana, chargeWand, lastChargeD20(5))
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE)
	d.h.roller.queue(5)
	if _, err = d.roll(t, d.ana, chargeWand); err != nil {
		t.Errorf("an app roll at an app table = %v", err)
	}
}

// A second call for the last charge with another key is no retry: whoever gets the lock second finds the wait over and is
// refused with invalid_argument "no last charge to roll" (no ChargesBlocked, no CharacterBlocked), also when the first one
// destroyed the item. Only the same key reads the first answer back (TestRollLastChargeIsIdempotent).
func TestTwoRollsOfTheLastChargeWithTwoKeysGiveOneSuccess(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	c.mustSpend(t, c.ana, chargeWand, 7)
	c.h.roller.queue(5) // one d20: the roll that loses the race never rolls
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, u := range []*user{c.ana, c.master} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = c.roll(t, u, chargeWand)
		}()
	}
	wg.Wait()
	var refused error
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else {
			refused = err
		}
	}
	if ok != 1 {
		t.Fatalf("the two rolls = %v, %v; want exactly one success", errs[0], errs[1])
	}
	wantCode(t, "the roll that came second", refused, connect.CodeInvalidArgument)
	var ce *connect.Error
	if !errors.As(refused, &ce) || !strings.Contains(ce.Message(), "no last charge to roll") || len(ce.Details()) != 0 {
		t.Errorf("the second roll's error = %v, want invalid_argument \"no last charge to roll\" with no blocked detail", refused)
	}
}

func TestRollLastChargeIsIdempotent(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeStaff)
	c.mustSpend(t, c.ana, chargeStaff, 20)
	key := newKey()
	withKey := func(r *playv1.RollLastChargeRequest) { r.IdempotencyKey = key }
	c.h.roller.queue(20, 5)
	first, err := c.roll(t, c.ana, chargeStaff, withKey)
	if err != nil {
		t.Fatalf("RollLastCharge() error = %v", err)
	}
	c.h.roller.queue(1, 1) // a second roll would show
	again, err := c.roll(t, c.ana, chargeStaff, withKey)
	if err != nil {
		t.Fatalf("RollLastCharge() retry error = %v", err)
	}
	if again.GetD20() != 20 || again.GetRegained() != first.GetRegained() || again.GetItem().GetChargesLeft() != 7 {
		t.Errorf("the retry = %v, want the first roll's numbers", again)
	}
	_, err = c.roll(t, c.ana, chargeStaff, withKey, lastChargeD20(3))
	wantCode(t, "the key reused for another roll", err, connect.CodeInvalidArgument)
}

func TestTheMastersCorrectionOfTheCharges(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand, chargeLuck)
	_, err := c.correct(t, c.ana, chargeWand, 3, charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED)
	wantCode(t, "a player's SetCharges", err, connect.CodePermissionDenied)
	res, err := c.correct(t, c.master, chargeWand, 3, charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED)
	if err != nil || res.GetItem().GetChargesLeft() != 3 {
		t.Fatalf("SetCharges(3) = %v, %v", res, err)
	}
	if res, err = c.correct(t, c.master, chargeLuck, 2, charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED); err != nil || res.GetItem().GetChargesLeft() != 2 {
		t.Errorf("setting the count of a rolled start = %v, %v", res, err)
	}
	if res, err = c.correct(t, c.master, chargeWand, 0, charactersv1.ItemLoss_ITEM_LOSS_GONE); err != nil || res.GetItem().GetLost() != charactersv1.ItemLoss_ITEM_LOSS_GONE {
		t.Fatalf("SetCharges(lost) = %v, %v", res, err)
	}
	if n := c.countEvents(t, eventItemDestroyed); n != 1 {
		t.Errorf("the session has %d item_destroyed events, want 1", n)
	}
	if res, err = c.correct(t, c.master, chargeWand, 5, charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED); err != nil || res.GetItem().GetLost() != charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED || res.GetItem().GetChargesLeft() != 5 {
		t.Errorf("bringing it back = %v, %v", res, err)
	}
	if n := c.countEvents(t, eventItemChargesRestored); n != 3 {
		t.Errorf("the session has %d item_charges_restored events, want 3", n)
	}
	_, err = c.correct(t, c.master, chargeWand, 8, charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED)
	wantCode(t, "more than the maximum", err, connect.CodeInvalidArgument)
	_, err = c.correct(t, c.master, chargeWand, -1, charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED)
	wantCode(t, "a negative count", err, connect.CodeInvalidArgument)
	_, err = c.correct(t, c.master, chargeWand, 1, charactersv1.ItemLoss(9))
	wantCode(t, "an unknown loss", err, connect.CodeInvalidArgument)

	key := newKey()
	withKey := func(r *playv1.SetChargesRequest) { r.IdempotencyKey = key }
	before := c.countEvents(t, eventItemChargesRestored)
	for range 2 {
		if _, err := c.correct(t, c.master, chargeWand, 4, charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED, withKey); err != nil {
			t.Fatalf("SetCharges() with a key error = %v", err)
		}
	}
	if got := c.countEvents(t, eventItemChargesRestored); got != before+1 {
		t.Errorf("a retried correction wrote %d events, want 1", got-before)
	}
	_, err = c.correct(t, c.master, chargeWand, 2, charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED, withKey)
	wantCode(t, "the key reused for another count", err, connect.CodeInvalidArgument)
}

// The combat's line is the master's and the owner's: the other players read nothing of it (RN-10).
// A charge's use is no action an undo can take back: item_charge_spent, the d20 of the last charge and the master's correction
// all close the undo chain, so the attack before them cannot be undone any more (the charges are not given back either).
func TestSpendingAChargeClosesTheUndoChain(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	e := c.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: c.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 10, "Pensantus": 18, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	if _, err := c.attack(t, c.ana, e, "Pensantus", fireBolt, "Goblin", d20(1)); err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	attackID := c.log(t, c.master, c.get(t, c.master)).GetUndoableEventId()
	if attackID == "" {
		t.Fatal("the attack is not offered to undo before the charge is spent")
	}
	c.mustSpend(t, c.ana, chargeWand, 1)
	if id := c.log(t, c.master, c.get(t, c.master)).GetUndoableEventId(); id != "" {
		t.Errorf("after a charge was spent the log offers %s to undo, want nothing", id)
	}
	wantEncounterBlocked(t, c.undo(t, c.master, e, attackID), blockedNothingToUndo)
	if left := c.itemOf(t, c.ana, chargeWand).GetChargesLeft(); left != 6 {
		t.Errorf("charges left = %d, want 6", left)
	}
}

func TestACombatLogLineOfTheChargesIsTheOwnersAndTheMastersAlone(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	e := c.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: c.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 10, "Pensantus": 18, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	c.mustSpend(t, c.ana, chargeWand, 1)
	c.mustSpend(t, c.ana, chargeWand, 6)
	c.h.roller.queue(1)
	if _, err := c.roll(t, c.ana, chargeWand); err != nil {
		t.Fatalf("RollLastCharge() error = %v", err)
	}
	lines := func(u *user) []*playv1.CombatLogEntry {
		var out []*playv1.CombatLogEntry
		for _, round := range c.log(t, u, e).GetRounds() {
			for _, en := range round.GetEntries() {
				if en.GetItemUse() != nil || en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ITEM_CHARGE_SPENT || en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ITEM_LAST_CHARGE_ROLLED {
					out = append(out, en)
				}
			}
		}
		slices.Reverse(out) // the log is the latest first
		return out
	}
	for _, u := range []*user{c.master, c.ana} {
		got := lines(u)
		if len(got) != 3 {
			t.Fatalf("a reader with the line sees %d item lines, want 3", len(got))
		}
		first, last, roll := got[0].GetItemUse(), got[1].GetItemUse(), got[2].GetItemUse()
		if got[0].GetKind() != playv1.CombatLogKind_COMBAT_LOG_KIND_ITEM_CHARGE_SPENT || first.GetAmount() != 1 || first.GetLeft() != 6 || first.GetMax() != 7 || first.GetOutcome() != playv1.ItemUseOutcome_ITEM_USE_OUTCOME_SPENT || first.GetItemNamePt() == "" {
			t.Errorf("the first line = %v, want 1 spent, 6 of 7 left", got[0])
		}
		if last.GetOutcome() != playv1.ItemUseOutcome_ITEM_USE_OUTCOME_LAST_CHARGE || last.GetLeft() != 0 {
			t.Errorf("the last charge's line = %v", got[1])
		}
		if got[2].GetKind() != playv1.CombatLogKind_COMBAT_LOG_KIND_ITEM_LAST_CHARGE_ROLLED || roll.GetD20() != 1 || roll.GetOutcome() != playv1.ItemUseOutcome_ITEM_USE_OUTCOME_LOST || roll.GetLost() != charactersv1.ItemLoss_ITEM_LOSS_GONE {
			t.Errorf("the roll's line = %v, want a 1 that lost the wand", got[2])
		}
		if got[0].GetActorLabel() != "Pensantus" {
			t.Errorf("the line's actor = %q, want Pensantus", got[0].GetActorLabel())
		}
	}
	for name, u := range map[string]*user{"Caio": c.caio, "Bia": c.bia} {
		if got := lines(u); len(got) != 0 {
			t.Errorf("%s reads %d item lines, want none", name, len(got))
		}
	}
}

func TestOutsideACombatOnlyTheSessionEventIsWritten(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	c.mustSpend(t, c.ana, chargeWand, 1)
	var n int
	if err := c.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1 AND encounter_id IS NOT NULL`, eventItemChargeSpent).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 || c.countEvents(t, eventItemChargeSpent) != 1 {
		t.Errorf("outside a combat the event belongs to an encounter (%d), want a plain session event", n)
	}
}

// The master and the owner hear that the sheet changed (the vitals hint of every sheet change); another player hears nothing of it.
func TestSpendingChargesTellsTheOwnerAndTheMasterAlone(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	master, ana, caio := c.master.watch(t, c.campaignID), c.ana.watch(t, c.campaignID), c.caio.watch(t, c.campaignID)
	for _, w := range []*watcher{master, ana, caio} {
		w.ready(t)
	}
	c.mustSpend(t, c.ana, chargeWand, 1)
	c.master.markCurrentMap(t, c.campaignID, c.h.newMap(c.campaignID, 0), master, ana, caio)
	heard := func(w *watcher) bool {
		for _, ev := range w.beforeMarker(t) {
			if ev.GetVitalsChanged().GetVitals().GetCharacterId() == c.pens.GetId() {
				return true
			}
		}
		return false
	}
	if !heard(master) || !heard(ana) {
		t.Error("the master and the owner were not told that the sheet changed")
	}
	if evs := caio.beforeMarker(t); len(evs) != 0 {
		t.Errorf("another player received %v", evs)
	}
}

// Ammunition is what the sheet holds while it is open to editing: the owner sets it as a draft, the master any time.
func TestSettingAmmunition(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana, bia := h.newUser("Mestre"), h.newUser("Ana"), h.newUser("Bia")
	campaign := h.newCampaign(master, "Mirathel", ana, bia)
	hero := ana.hero(t, campaign, "Tor", "class:fighter", "race:human", 2, abilities(16, 13, 14, 10, 10), []string{battleaxe}, nil)
	set := func(u *user, key string, quantity int32) (*playv1.SetAmmunitionResponse, error) {
		res, err := u.charges.SetAmmunition(t.Context(), connect.NewRequest(&playv1.SetAmmunitionRequest{CampaignId: campaign, CharacterId: hero.GetId(), AmmoKey: key, Quantity: quantity}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	res, err := set(ana, arrowAmmo, 20)
	if err != nil || res.GetAmmunition().GetQuantity() != 20 || res.GetAmmunition().GetNamePt() == "" {
		t.Fatalf("the owner's draft SetAmmunition = %v, %v", res, err)
	}
	_, err = set(bia, arrowAmmo, 5)
	wantCode(t, "another player's SetAmmunition", err, connect.CodePermissionDenied)
	_, err = set(ana, "equipment:longsword", 5)
	wantCode(t, "a key that is no ammunition", err, connect.CodeInvalidArgument)
	_, err = set(master, arrowAmmo, 10000)
	wantCode(t, "10000 pieces", err, connect.CodeInvalidArgument)
	_, err = set(master, arrowAmmo, -1)
	wantCode(t, "a negative count", err, connect.CodeInvalidArgument)
	_, err = ana.charges.SetAmmunition(t.Context(), connect.NewRequest(&playv1.SetAmmunitionRequest{CampaignId: campaign, CharacterId: newKey(), AmmoKey: arrowAmmo, Quantity: 1}))
	wantCode(t, "a character that is not there", err, connect.CodeNotFound)

	master.start(t, campaign) // the sheet locks
	_, err = set(ana, arrowAmmo, 30)
	wantCode(t, "the owner's SetAmmunition after the lock", err, connect.CodePermissionDenied)
	if res, err = set(master, arrowAmmo, 30); err != nil || res.GetAmmunition().GetQuantity() != 30 {
		t.Fatalf("the master's SetAmmunition after the lock = %v, %v", res, err)
	}
	read, err := ana.characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: campaign, CharacterId: hero.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	if am := read.Msg.GetCharacter().GetSheet().GetFull().GetAmmunition(); len(am) != 1 || am[0].GetQuantity() != 30 {
		t.Errorf("the sheet shows %v, want 30 arrows", am)
	}
}

// fire spends pieces for Pensantus in the encounter, as the attack of a later change does.
func (c *charged) fire(t *testing.T, encounterID string, pieces int) {
	t.Helper()
	err := db.InTx(t.Context(), c.h.pool, func(tx pgx.Tx) error {
		for range pieces {
			if _, _, err := c.h.chars.SpendAmmunition(t.Context(), tx, c.campaignID, c.pens.GetId(), arrowAmmo, encounterID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("spend the ammunition: %v", err)
	}
}

func (c *charged) recoverAmmo(t *testing.T, u *user, key string) (*playv1.RecoverAmmunitionResponse, error) {
	t.Helper()
	res, err := u.charges.RecoverAmmunition(t.Context(), connect.NewRequest(&playv1.RecoverAmmunitionRequest{CampaignId: c.campaignID, CharacterId: c.pens.GetId(), IdempotencyKey: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func TestRecoveringAmmunitionAfterTheCombat(t *testing.T) {
	t.Parallel()
	c := newCharged(t)
	_, err := c.master.charges.SetAmmunition(t.Context(), connect.NewRequest(&playv1.SetAmmunitionRequest{CampaignId: c.campaignID, CharacterId: c.pens.GetId(), AmmoKey: arrowAmmo, Quantity: 20}))
	if err != nil {
		t.Fatalf("SetAmmunition() error = %v", err)
	}
	_, err = c.recoverAmmo(t, c.ana, newKey())
	wantCode(t, "a recovery before any combat", err, connect.CodeInvalidArgument)

	e := c.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: c.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 10, "Pensantus": 18, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	c.fire(t, e.GetId(), 5)
	_, err = c.recoverAmmo(t, c.ana, newKey())
	wantChargesBlocked(t, "a recovery during the combat", err, playv1.ChargesBlockedReason_CHARGES_BLOCKED_REASON_COMBAT_RUNNING)

	c.endEncounter(t, e)
	_, err = c.recoverAmmo(t, c.bia, newKey())
	wantCode(t, "another player's recovery", err, connect.CodePermissionDenied)
	key := newKey()
	res, err := c.recoverAmmo(t, c.ana, key)
	if err != nil {
		t.Fatalf("RecoverAmmunition() error = %v", err)
	}
	if got := res.GetAmmunition(); len(got) != 1 || got[0].GetKey() != arrowAmmo || got[0].GetQuantity() != 17 || !got[0].GetRecovered() {
		t.Errorf("the recovery = %v, want 17 arrows (15 held and half of the 5 fired, rounded down)", got)
	}
	again, err := c.recoverAmmo(t, c.ana, key)
	if err != nil || len(again.GetAmmunition()) != 1 {
		t.Errorf("the retry of the key = %v, %v, want the first answer", again, err)
	}
	if n := c.countEvents(t, eventAmmunitionRecovered); n != 1 {
		t.Errorf("the session has %d ammunition_recovered events, want 1", n)
	}
	_, err = c.recoverAmmo(t, c.master, newKey())
	wantChargesBlocked(t, "the second recovery of the same combat", err, playv1.ChargesBlockedReason_CHARGES_BLOCKED_REASON_ALREADY_RECOVERED)
}

func TestRecoveringAmmunitionAfterACombatThatFiredNothing(t *testing.T) {
	t.Parallel()
	c := newCharged(t)
	e := c.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: c.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 10, "Pensantus": 18, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	c.endEncounter(t, e)
	_, err := c.recoverAmmo(t, c.ana, newKey())
	wantCode(t, "a recovery with nothing fired", err, connect.CodeInvalidArgument)
	if !strings.Contains(err.Error(), "ammunition") {
		t.Errorf("the refusal = %q, want it to say why", err)
	}
}

// A combat that fired nothing is passed over: the recovery goes to the newest earlier combat that fired some.
func TestRecoveringAmmunitionSkipsALaterCombatThatFiredNothing(t *testing.T) {
	t.Parallel()
	c := newCharged(t)
	_, err := c.master.charges.SetAmmunition(t.Context(), connect.NewRequest(&playv1.SetAmmunitionRequest{CampaignId: c.campaignID, CharacterId: c.pens.GetId(), AmmoKey: arrowAmmo, Quantity: 20}))
	if err != nil {
		t.Fatalf("SetAmmunition() error = %v", err)
	}
	fight := plan{
		npcs:     []*playv1.Participant{{CharacterId: c.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 10, "Pensantus": 18, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	}
	first := c.start(t, fight)
	c.fire(t, first.GetId(), 4)
	c.endEncounter(t, first)
	c.endEncounter(t, c.start(t, fight))

	res, err := c.recoverAmmo(t, c.ana, newKey())
	if err != nil {
		t.Fatalf("RecoverAmmunition() error = %v", err)
	}
	if got := res.GetAmmunition(); len(got) != 1 || got[0].GetQuantity() != 18 || got[0].GetSpentLastCombat() != 4 {
		t.Errorf("the recovery = %v, want 18 arrows (16 held and half of the 4 fired in the first combat)", got)
	}
	_, err = c.recoverAmmo(t, c.ana, newKey())
	wantChargesBlocked(t, "a second recovery", err, playv1.ChargesBlockedReason_CHARGES_BLOCKED_REASON_ALREADY_RECOVERED)
}

// A stored answer goes to the user who made the first call: another member, the master and the character's new player are
// refused as a reused key and no charge is spent again, while the first caller's retry gets the answer, also after the
// character changed hands.
func TestSpendChargesAnswersAStoredCallToItsAuthorOnly(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeWand)
	key := newKey()
	withKey := func(r *playv1.SpendChargesRequest) { r.IdempotencyKey = key }
	first, err := c.spend(c.ana, chargeWand, 2, withKey)(t)
	if err != nil {
		t.Fatalf("SpendCharges() error = %v", err)
	}
	check := func(step string) {
		t.Helper()
		if got := c.itemOf(t, c.master, chargeWand).GetChargesLeft(); got != 5 {
			t.Errorf("%s: the wand has %d charges, want 5", step, got)
		}
		if n := c.countEvents(t, eventItemChargeSpent); n != 1 {
			t.Errorf("%s: the session has %d item_charge_spent events, want 1", step, n)
		}
	}
	retry := func(step string) {
		t.Helper()
		again, err := c.spend(c.ana, chargeWand, 2, withKey)(t)
		if err != nil {
			t.Fatalf("%s: SpendCharges() error = %v", step, err)
		}
		if again.GetItem().GetChargesLeft() != first.GetItem().GetChargesLeft() {
			t.Errorf("%s: the answer left %d, the first left %d", step, again.GetItem().GetChargesLeft(), first.GetItem().GetChargesLeft())
		}
	}
	_, err = c.spend(c.bia, chargeWand, 2, withKey)(t)
	wantKeyReused(t, "another member's SpendCharges with the key", err)
	check("another member")
	_, err = c.spend(c.master, chargeWand, 2, withKey)(t)
	wantKeyReused(t, "the master's SpendCharges with the key", err)
	check("the master")
	retry("the author")
	dora := c.h.newUser("Dora")
	c.h.join(c.master, c.campaignID, dora)
	c.h.giveCharacter(t, c.pens.GetId(), dora)
	_, err = c.spend(dora, chargeWand, 2, withKey)(t)
	wantKeyReused(t, "the new player's SpendCharges with the key", err)
	check("the new player")
	retry("the author after the gift")
	check("the author's retry")
}

// A stored answer goes to the user who made the first call: another member, the master and the character's new player are
// refused as a reused key and nothing is rolled again, while the first caller's retry gets the roll, also after the
// character changed hands.
func TestRollLastChargeAnswersAStoredCallToItsAuthorOnly(t *testing.T) {
	t.Parallel()
	c := newCharged(t, chargeStaff)
	c.mustSpend(t, c.ana, chargeStaff, 20)
	key := newKey()
	withKey := func(r *playv1.RollLastChargeRequest) { r.IdempotencyKey = key }
	c.h.roller.queue(20, 5)
	first, err := c.roll(t, c.ana, chargeStaff, withKey)
	if err != nil {
		t.Fatalf("RollLastCharge() error = %v", err)
	}
	c.h.roller.queue(1, 1) // a second roll would show
	check := func(step string) {
		t.Helper()
		if got := c.itemOf(t, c.master, chargeStaff).GetChargesLeft(); got != first.GetItem().GetChargesLeft() {
			t.Errorf("%s: the staff has %d charges, want %d", step, got, first.GetItem().GetChargesLeft())
		}
		if n := c.countEvents(t, eventItemChargesRestored); n != 1 {
			t.Errorf("%s: the session has %d item_charges_restored events, want 1", step, n)
		}
	}
	retry := func(step string) {
		t.Helper()
		again, err := c.roll(t, c.ana, chargeStaff, withKey)
		if err != nil {
			t.Fatalf("%s: RollLastCharge() error = %v", step, err)
		}
		if again.GetD20() != 20 || again.GetRegained() != first.GetRegained() || again.GetItem().GetChargesLeft() != first.GetItem().GetChargesLeft() {
			t.Errorf("%s: the answer = %v, want the first roll's numbers", step, again)
		}
	}
	_, err = c.roll(t, c.bia, chargeStaff, withKey)
	wantKeyReused(t, "another member's RollLastCharge with the key", err)
	check("another member")
	_, err = c.roll(t, c.master, chargeStaff, withKey)
	wantKeyReused(t, "the master's RollLastCharge with the key", err)
	check("the master")
	retry("the author")
	dora := c.h.newUser("Dora")
	c.h.join(c.master, c.campaignID, dora)
	c.h.giveCharacter(t, c.pens.GetId(), dora)
	_, err = c.roll(t, dora, chargeStaff, withKey)
	wantKeyReused(t, "the new player's RollLastCharge with the key", err)
	check("the new player")
	retry("the author after the gift")
	check("the author's retry")
}

// A stored answer goes to the user who made the first call: another member, the master and the character's new player are
// refused as a reused key and nothing is recovered again, while the first caller's retry gets the answer, also after the
// character changed hands.
func TestRecoverAmmunitionAnswersAStoredCallToItsAuthorOnly(t *testing.T) {
	t.Parallel()
	c := newCharged(t)
	_, err := c.master.charges.SetAmmunition(t.Context(), connect.NewRequest(&playv1.SetAmmunitionRequest{CampaignId: c.campaignID, CharacterId: c.pens.GetId(), AmmoKey: arrowAmmo, Quantity: 20}))
	if err != nil {
		t.Fatalf("SetAmmunition() error = %v", err)
	}
	e := c.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: c.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 10, "Pensantus": 18, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	c.fire(t, e.GetId(), 5)
	c.endEncounter(t, e)
	key := newKey()
	first, err := c.recoverAmmo(t, c.ana, key)
	if err != nil {
		t.Fatalf("RecoverAmmunition() error = %v", err)
	}
	if got := first.GetAmmunition(); len(got) != 1 || got[0].GetQuantity() != 17 {
		t.Fatalf("the recovery = %v, want 17 arrows", got)
	}
	check := func(step string) {
		t.Helper()
		if n := c.countEvents(t, eventAmmunitionRecovered); n != 1 {
			t.Errorf("%s: the session has %d ammunition_recovered events, want 1", step, n)
		}
	}
	retry := func(step string) {
		t.Helper()
		again, err := c.recoverAmmo(t, c.ana, key)
		if err != nil {
			t.Fatalf("%s: RecoverAmmunition() error = %v", step, err)
		}
		if got := again.GetAmmunition(); len(got) != 1 || got[0].GetKey() != arrowAmmo || got[0].GetQuantity() != 17 || !got[0].GetRecovered() {
			t.Errorf("%s: the answer = %v, want the first one (17 arrows)", step, got)
		}
	}
	_, err = c.recoverAmmo(t, c.bia, key)
	wantKeyReused(t, "another member's RecoverAmmunition with the key", err)
	check("another member")
	_, err = c.recoverAmmo(t, c.master, key)
	wantKeyReused(t, "the master's RecoverAmmunition with the key", err)
	check("the master")
	retry("the author")
	dora := c.h.newUser("Dora")
	c.h.join(c.master, c.campaignID, dora)
	c.h.giveCharacter(t, c.pens.GetId(), dora)
	_, err = c.recoverAmmo(t, dora, key)
	wantKeyReused(t, "the new player's RecoverAmmunition with the key", err)
	check("the new player")
	retry("the author after the gift")
	check("the author's retry")
}
