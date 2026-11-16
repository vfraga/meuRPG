package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Using a consumable outside a combat (ChargesService.UseConsumable): the unit is spent and the effect of the healing and
// the resistance potions happens, any other consumable only returns its text, the receiver may be another character, and
// a retry of a key answers the first call.

const (
	potionCommon   = "item:potion-of-healing-common"   // 2d4+2
	potionGreater  = "item:potion-of-healing-greater"  // 4d4+4
	potionSuperior = "item:potion-of-healing-superior" // 8d4+8
	potionSupreme  = "item:potion-of-healing-supreme"  // 10d4+20
	potionFire     = "item:potion-of-resistance-fire"
	potionClimb    = "item:potion-of-climbing" // no automatic effect
	oilSharpness   = "item:oil-of-sharpness"   // an oil of the potion category: applied, not drunk
	scrollFirst    = "item:spell-scroll-1st"   // no automatic effect: it is not cast
)

// stocked is newArmed with the potions on the sheets of Toren (a fighter, 20 hit points or so) and Pensantus.
type stocked struct{ *armed }

func newStocked(t *testing.T, keys ...string) *stocked {
	t.Helper()
	a := newArmed(t)
	a.toren = a.withItems(t, a.toren, keys...)
	a.pens = a.withItems(t, a.pens, keys...)
	return &stocked{a}
}

// entryOf is the id of the item on the character's sheet.
func (s *stocked) entryOf(t *testing.T, c *charactersv1.Character, key string) string {
	t.Helper()
	cur, err := s.master.characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: s.campaignID, CharacterId: c.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	for _, it := range cur.Msg.GetCharacter().GetSheet().GetFull().GetMagicItems() {
		if it.GetKey() == key {
			return it.GetId()
		}
	}
	t.Fatalf("%s is not on %s's sheet", key, c.GetName())
	return ""
}

func (s *stocked) holds(t *testing.T, c *charactersv1.Character, key string) bool {
	t.Helper()
	cur, err := s.master.characters.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: s.campaignID, CharacterId: c.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	return slices.ContainsFunc(cur.Msg.GetCharacter().GetSheet().GetFull().GetMagicItems(), func(it *charactersv1.SheetMagicItem) bool { return it.GetKey() == key })
}

// use is u using the potion of who's sheet, app-rolled, with a new key.
func (s *stocked) use(t *testing.T, u *user, who *charactersv1.Character, key string, edit ...func(*playv1.UseConsumableRequest)) (*playv1.UseConsumableResponse, error) {
	t.Helper()
	req := &playv1.UseConsumableRequest{
		CampaignId: s.campaignID, CharacterId: who.GetId(), IdempotencyKey: newKey(), RollInApp: true,
	}
	for _, e := range edit {
		e(req)
	}
	if req.EntryId == "" { // a test that names the entry (one already used, one that is not there) says so
		req.EntryId = s.entryOf(t, who, key)
	}
	res, err := u.charges.UseConsumable(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (s *stocked) mustUse(t *testing.T, u *user, who *charactersv1.Character, key string, edit ...func(*playv1.UseConsumableRequest)) *playv1.UseConsumableResponse {
	t.Helper()
	res, err := s.use(t, u, who, key, edit...)
	if err != nil {
		t.Fatalf("UseConsumable(%s) error = %v", key, err)
	}
	return res
}

func giveTo(c *charactersv1.Character) func(*playv1.UseConsumableRequest) {
	return func(r *playv1.UseConsumableRequest) { r.TargetCharacterId = c.GetId() }
}

func typedDice(faces ...int32) func(*playv1.UseConsumableRequest) {
	return func(r *playv1.UseConsumableRequest) { r.RollInApp, r.TypedFaces = false, faces }
}

func (s *stocked) maxHP(t *testing.T, c *charactersv1.Character) int32 {
	t.Helper()
	return s.vitals(t, c).GetHitPointsMax()
}

func TestAHealingPotionHealsWithItsDiceAtEveryRarity(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		key   string
		faces []int
		text  string
		total int32
	}{
		"common":   {potionCommon, []int{3, 4}, "2d4+2", 9},
		"greater":  {potionGreater, []int{1, 2, 3, 4}, "4d4+4", 14},
		"superior": {potionSuperior, []int{1, 1, 1, 1, 1, 1, 1, 1}, "8d4+8", 16},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newStocked(t, tc.key)
			maxHP := s.maxHP(t, s.toren)
			s.hurt(t, s.toren, 1)
			s.h.roller.queue(tc.faces...)
			res := s.mustUse(t, s.caio, s.toren, tc.key)
			if want := min(tc.total, maxHP-1); res.GetHealed() != want {
				t.Errorf("healed = %d, want %d", res.GetHealed(), want)
			}
			if res.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_HEALED || res.GetDiceText() != tc.text || res.GetRollTotal() != tc.total || len(res.GetFaces()) != len(tc.faces) {
				t.Errorf("response = %v, want HEALED %s rolled to %d", res, tc.text, tc.total)
			}
			if got := s.vitals(t, s.toren).GetHitPointsCurrent(); got != 1+res.GetHealed() {
				t.Errorf("hit points = %d, want %d", got, 1+res.GetHealed())
			}
			if res.GetItemKey() != tc.key || res.GetItemNamePt() == "" || res.GetEntryId() == "" || res.GetTargetCharacterId() != s.toren.GetId() {
				t.Errorf("response = %v, want the item, its name, the entry and Toren as the receiver", res)
			}
			if s.holds(t, s.toren, tc.key) {
				t.Error("the potion is still on the sheet")
			}
			if len(res.GetVitals()) != 1 || res.GetVitals()[0].GetCharacterId() != s.toren.GetId() || res.GetVitals()[0].GetHitPointsCurrent() != 1+res.GetHealed() {
				t.Errorf("vitals = %v, want Toren's as they stand", res.GetVitals())
			}
		})
	}
}

func TestASupremePotionIsCutAtTheMaximumAndHealedIsWhatWasRegained(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionSupreme)
	maxHP := s.maxHP(t, s.toren)
	s.hurt(t, s.toren, maxHP-3)
	s.h.roller.queue(4, 4, 4, 4, 4, 4, 4, 4, 4, 4) // 40 + 20
	res := s.mustUse(t, s.caio, s.toren, potionSupreme)
	if res.GetHealed() != 3 || res.GetRollTotal() != 60 {
		t.Errorf("healed %d of a roll of %d, want 3 of 60", res.GetHealed(), res.GetRollTotal())
	}
	if got := s.vitals(t, s.toren).GetHitPointsCurrent(); got != maxHP {
		t.Errorf("hit points = %d, want the maximum %d", got, maxHP)
	}
}

func TestAHealingPotionAtFullHealthIsSpentAllTheSame(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	s.h.roller.queue(2, 2)
	res := s.mustUse(t, s.caio, s.toren, potionCommon)
	if res.GetHealed() != 0 || res.GetRollTotal() != 6 || s.holds(t, s.toren, potionCommon) {
		t.Errorf("response = %v, want 0 healed of a 6 and the potion gone", res)
	}
}

func TestAResistancePotionLastsOneHourOfGameTimeAndIsTheOwnersAlone(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionFire)
	res := s.mustUse(t, s.ana, s.pens, potionFire)
	if res.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_RESISTANCE || res.GetDamageTypeKey() != "damage-type:fire" {
		t.Errorf("response = %v, want RESISTANCE to fire", res)
	}
	if seconds, found := s.potionEffectSeconds(t, s.pens.GetId(), potionFire); !found || seconds != 3600 {
		t.Errorf("the effect = %d seconds, %v; want 3600", seconds, found)
	}
	if got := s.seenBy(t, s.ana, s.bia, s.pens.GetId()); got != "1/1/0" {
		t.Errorf("effects read by master/owner/other = %s, want 1/1/0", got)
	}
	s.advanceGameTimeKeyed(t, newKey(), 3599)
	if seconds, found := s.potionEffectSeconds(t, s.pens.GetId(), potionFire); !found || seconds != 1 {
		t.Errorf("after 3599 seconds the effect = %d, %v; want 1 second left", seconds, found)
	}
	s.advanceGameTimeKeyed(t, newKey(), 1)
	if _, found := s.potionEffectSeconds(t, s.pens.GetId(), potionFire); found {
		t.Error("the effect is still there after the hour")
	}
}

func TestAResistancePotionOutsideACombatIsAnEffectOfTheItem(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionFire)
	s.mustUse(t, s.ana, s.pens, potionFire)
	var kind string
	if err := s.h.pool.QueryRow(t.Context(), `SELECT source_kind FROM character_effects WHERE character_id = $1 AND source_key = $2`, s.pens.GetId(), potionFire).Scan(&kind); err != nil || kind != "item" {
		t.Errorf("the effect on the character is of the kind %q (%v), want item", kind, err)
	}
}

func (s *stocked) potionEffectSeconds(t *testing.T, characterID, key string) (seconds int32, found bool) {
	t.Helper()
	rows, err := s.h.pool.Query(t.Context(), `SELECT seconds_left FROM character_effects WHERE character_id = $1 AND source_key = $2`, characterID, key)
	if err != nil {
		t.Fatalf("read the effect: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&seconds); err != nil {
			t.Fatalf("read the effect: %v", err)
		}
		return seconds, true
	}
	return 0, false
}

func TestAConsumableWithNoAutomaticEffectIsSpentAndReturnsItsText(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionClimb, scrollFirst)
	for _, key := range []string{potionClimb, scrollFirst} {
		before := s.vitals(t, s.pens).GetHitPointsCurrent()
		res := s.mustUse(t, s.ana, s.pens, key)
		if res.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_TEXT_ONLY || len(res.GetText()) == 0 || res.GetItemKey() != key {
			t.Errorf("%s: response = %v, want TEXT_ONLY with the item's text", key, res)
		}
		if res.GetHealed() != 0 || res.GetDiceText() != "" || s.holds(t, s.pens, key) || s.vitals(t, s.pens).GetHitPointsCurrent() != before {
			t.Errorf("%s: it healed, rolled or stayed on the sheet: %v", key, res)
		}
	}
	if n := s.countEvents(t, eventConsumableUsed); n != 2 {
		t.Errorf("the session has %d consumable_used events, want 2", n)
	}
}

func TestAPotionGivenHealsTheReceiverAndTheGiversEntryIsTheOneSpent(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	s.hurt(t, s.toren, 2)
	s.h.roller.queue(4, 4)
	res := s.mustUse(t, s.ana, s.pens, potionCommon, giveTo(s.toren))
	if res.GetHealed() != 10 || res.GetTargetCharacterId() != s.toren.GetId() {
		t.Errorf("response = %v, want Toren healed by 10", res)
	}
	if got := s.vitals(t, s.toren).GetHitPointsCurrent(); got != 12 {
		t.Errorf("Toren's hit points = %d, want 12", got)
	}
	if s.holds(t, s.pens, potionCommon) || !s.holds(t, s.toren, potionCommon) {
		t.Error("the giver's potion must be the one spent, the receiver's own stays")
	}
	var got string
	if err := s.h.pool.QueryRow(t.Context(), `SELECT character_id::text FROM session_events WHERE kind = $1`, eventConsumableUsed).Scan(&got); err != nil || got != s.pens.GetId() {
		t.Errorf("the event is the giver's: %q, %v", got, err)
	}
}

func TestAResistancePotionGivenIsTheReceiversEffect(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionFire)
	s.mustUse(t, s.ana, s.pens, potionFire, giveTo(s.toren))
	if _, found := s.potionEffectSeconds(t, s.toren.GetId(), potionFire); !found {
		t.Error("Toren has no resistance")
	}
	if _, found := s.potionEffectSeconds(t, s.pens.GetId(), potionFire); found {
		t.Error("the giver got the resistance")
	}
}

func TestTheMasterUsesAndGivesForAnyCharacter(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	s.hurt(t, s.toren, 1)
	s.h.roller.queue(1, 1)
	if res := s.mustUse(t, s.master, s.pens, potionCommon, giveTo(s.toren)); res.GetHealed() != 4 {
		t.Errorf("the master's gift healed %d, want 4", res.GetHealed())
	}
	s.h.roller.queue(2, 2)
	if res := s.mustUse(t, s.master, s.toren, potionCommon); res.GetHealed() != 6 {
		t.Errorf("the master's use for Toren healed %d, want 6", res.GetHealed())
	}
}

func TestUsingAConsumableIsRefused(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon, "item:bag-of-holding")
	_, err := s.use(t, s.bia, s.pens, potionCommon)
	wantCode(t, "a stranger's use", err, connect.CodePermissionDenied)
	_, err = s.use(t, s.caio, s.pens, potionCommon)
	wantCode(t, "another player's character", err, connect.CodePermissionDenied)
	_, err = s.use(t, s.ana, s.pens, "item:bag-of-holding")
	wantCode(t, "an item that is no consumable", err, connect.CodeInvalidArgument)
	_, err = s.use(t, s.ana, s.pens, potionCommon, func(r *playv1.UseConsumableRequest) { r.EntryId = "m99" })
	wantBlockedBy(t, "an entry that is not there", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_GONE)
	_, err = s.use(t, s.ana, s.pens, potionCommon, func(r *playv1.UseConsumableRequest) { r.CharacterId = newKey() })
	wantCode(t, "a character that is not there", err, connect.CodeNotFound)
	_, err = s.use(t, s.ana, s.pens, potionCommon, func(r *playv1.UseConsumableRequest) { r.TargetCharacterId = newKey() })
	wantCode(t, "a receiver that is not there", err, connect.CodeNotFound)
	_, err = s.use(t, s.ana, s.pens, potionCommon, func(r *playv1.UseConsumableRequest) { r.IdempotencyKey = "no" })
	wantCode(t, "a key that is no UUID", err, connect.CodeInvalidArgument)
	_, err = s.use(t, s.ana, s.pens, potionCommon, func(r *playv1.UseConsumableRequest) { r.IdempotencyKey = "" })
	wantCode(t, "no key", err, connect.CodeInvalidArgument)
	if !s.holds(t, s.pens, potionCommon) || s.countEvents(t, eventConsumableUsed) != 0 {
		t.Error("a refused use spent the potion or wrote an event")
	}
}

func TestADeadCharacterNeitherUsesNorReceivesAPotion(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	if _, err := s.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: s.campaignID, CharacterId: s.toren.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	for _, u := range []*user{s.caio, s.master} {
		_, err := s.use(t, u, s.toren, potionCommon)
		wantChargeBlock(t, "the dead user", err, charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD)
	}
	_, err := s.use(t, s.ana, s.pens, potionCommon, giveTo(s.toren))
	wantChargeBlock(t, "the dead receiver", err, charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD)
	if !s.holds(t, s.pens, potionCommon) {
		t.Error("a potion given to the dead was spent")
	}
}

func TestUsingAConsumableIsIdempotent(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	s.hurt(t, s.toren, 1)
	key := newKey()
	withKey := func(r *playv1.UseConsumableRequest) { r.IdempotencyKey = key }
	entry := s.entryOf(t, s.toren, potionCommon)
	withEntry := func(r *playv1.UseConsumableRequest) { r.EntryId = entry }
	s.h.roller.queue(3, 3)
	first := s.mustUse(t, s.caio, s.toren, potionCommon, withKey, withEntry)
	s.h.roller.queue(4, 4) // a second roll would show
	again := s.mustUse(t, s.caio, s.toren, potionCommon, withKey, withEntry)
	if first.GetHealed() != 8 || again.GetHealed() != 8 || again.GetRollTotal() != first.GetRollTotal() || again.GetEntryId() != first.GetEntryId() || again.GetItemKey() != potionCommon {
		t.Errorf("the retry answered %v after %v, want the first answer", again, first)
	}
	if got := s.vitals(t, s.toren).GetHitPointsCurrent(); got != 9 || len(again.GetVitals()) != 1 || again.GetVitals()[0].GetHitPointsCurrent() != 9 {
		t.Errorf("hit points = %d and the retry's vitals %v, want 9", got, again.GetVitals())
	}
	if n := s.countEvents(t, eventConsumableUsed); n != 1 {
		t.Errorf("the session has %d consumable_used events, want 1", n)
	}
	_, err := s.use(t, s.caio, s.toren, potionCommon, withKey, withEntry, func(r *playv1.UseConsumableRequest) { r.TargetCharacterId = s.pens.GetId() })
	wantCode(t, "the key reused for another request", err, connect.CodeInvalidArgument)
}

func TestTwoKeysOnOneEntryGiveOneUse(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	entry := s.entryOf(t, s.toren, potionCommon)
	withEntry := func(r *playv1.UseConsumableRequest) { r.EntryId = entry }
	s.mustUse(t, s.caio, s.toren, potionCommon, withEntry)
	_, err := s.use(t, s.master, s.toren, potionCommon, withEntry) // another key: no retry, the entry is gone
	wantBlockedBy(t, "a second key on the used entry", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_GONE)
	if n := s.countEvents(t, eventConsumableUsed); n != 1 {
		t.Errorf("the session has %d consumable_used events, want 1", n)
	}
}

func TestTheDiceOfAPotionFollowTheTablesDiceMode(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	s.hurt(t, s.toren, 1)
	s.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	_, err := s.use(t, s.caio, s.toren, potionCommon)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE)
	for name, faces := range map[string][]int32{"too few faces": {3}, "too many faces": {1, 1, 1}, "a face off the die": {5, 1}, "a zero": {0, 2}, "no faces": nil} {
		_, err = s.use(t, s.caio, s.toren, potionCommon, typedDice(faces...))
		wantCode(t, name, err, connect.CodeInvalidArgument)
	}
	res := s.mustUse(t, s.caio, s.toren, potionCommon, typedDice(3, 2))
	if res.GetRollTotal() != 7 || res.GetHealed() != 7 || len(res.GetFaces()) != 2 || res.GetFaces()[0] != 3 {
		t.Errorf("typed faces 3 and 2 = %v, want a 7 healed", res)
	}

	d := newStocked(t, potionCommon)
	d.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	_, err = d.use(t, d.caio, d.toren, potionCommon, typedDice(3, 2))
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE)
	d.h.roller.queue(2, 2)
	if _, err := d.use(t, d.caio, d.toren, potionCommon); err != nil {
		t.Errorf("an app roll at an app table = %v", err)
	}
}

func TestAPotionWithNoDiceNeedsNoRollWhateverTheTableRolls(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionFire, potionClimb)
	s.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	for _, key := range []string{potionFire, potionClimb} {
		if _, err := s.use(t, s.ana, s.pens, key, typedDice()); err != nil {
			t.Errorf("%s at a table of real dice = %v, want it spent with no roll", key, err)
		}
	}
}

func TestUsingAConsumableWorksOnALockedSheet(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionClimb) // the session started: the sheet is locked for the player
	if res := s.mustUse(t, s.ana, s.pens, potionClimb); len(res.GetText()) == 0 {
		t.Errorf("response = %v, want the text", res)
	}
}

// The receiver's player and the master hear of the change; another player hears nothing.
func TestUsingAConsumableTellsTheOwnersAndTheMasterAlone(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	s.hurt(t, s.toren, 1)
	master, ana, caio, bia := s.master.watch(t, s.campaignID), s.ana.watch(t, s.campaignID), s.caio.watch(t, s.campaignID), s.bia.watch(t, s.campaignID)
	for _, w := range []*watcher{master, ana, caio, bia} {
		w.ready(t)
	}
	s.mustUse(t, s.ana, s.pens, potionCommon, giveTo(s.toren))
	s.master.markCurrentMap(t, s.campaignID, s.mapID, master, ana, caio, bia)
	heard := func(w *watcher, id string) bool {
		for _, ev := range w.beforeMarker(t) {
			if ev.GetVitalsChanged().GetVitals().GetCharacterId() == id {
				return true
			}
		}
		return false
	}
	if !heard(master, s.toren.GetId()) || !heard(caio, s.toren.GetId()) {
		t.Error("the master and the receiver's player were not told of the receiver's vitals")
	}
	if evs := bia.beforeMarker(t); len(evs) != 0 {
		t.Errorf("another player received %v", evs)
	}
}

// A second potion of the same resistance ends the first and starts its hour again, as a spell cast twice does.
func TestASecondResistancePotionStartsTheHourAgain(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionFire, potionFire)
	s.mustUse(t, s.ana, s.pens, potionFire)
	s.advanceGameTimeKeyed(t, newKey(), 1000)
	if seconds, _ := s.potionEffectSeconds(t, s.pens.GetId(), potionFire); seconds != 2600 {
		t.Fatalf("after 1000 seconds the effect = %d, want 2600", seconds)
	}
	s.mustUse(t, s.ana, s.pens, potionFire)
	if seconds, found := s.potionEffectSeconds(t, s.pens.GetId(), potionFire); !found || seconds != 3600 {
		t.Errorf("the second potion left %d seconds, %v; want a new hour", seconds, found)
	}
	var rows int
	if err := s.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM character_effects WHERE character_id = $1 AND source_key = $2`, s.pens.GetId(), potionFire).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("the character has %d resistance rows, %v; want 1", rows, err)
	}
}

func TestTheRetryOfAResistanceUseAnswersTheFirstAndAddsNothing(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionFire)
	key, entry := newKey(), s.entryOf(t, s.pens, potionFire)
	same := func(r *playv1.UseConsumableRequest) { r.IdempotencyKey, r.EntryId = key, entry }
	first := s.mustUse(t, s.ana, s.pens, potionFire, same)
	again := s.mustUse(t, s.ana, s.pens, potionFire, same)
	if again.GetDamageTypeKey() != first.GetDamageTypeKey() || again.GetOutcome() != first.GetOutcome() || again.GetEntryId() != first.GetEntryId() {
		t.Errorf("the retry answered %v after %v", again, first)
	}
	if n := s.countEvents(t, eventConsumableUsed); n != 1 {
		t.Errorf("the session has %d consumable_used events, want 1", n)
	}
	if seconds, found := s.potionEffectSeconds(t, s.pens.GetId(), potionFire); !found || seconds != 3600 {
		t.Errorf("the effect = %d, %v after the retry", seconds, found)
	}
}

// The giver is told what the use did, not the receiver's sheet: its vitals go to the master and to the receiver's own
// player (the retry too).
func TestAGiverThatDoesNotPlayTheReceiverIsToldNoVitals(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon, potionCommon)
	s.hurt(t, s.toren, 1)
	key, entry := newKey(), s.entryOf(t, s.pens, potionCommon)
	same := func(r *playv1.UseConsumableRequest) {
		r.IdempotencyKey, r.EntryId, r.TargetCharacterId = key, entry, s.toren.GetId()
	}
	first := s.mustUse(t, s.ana, s.pens, potionCommon, same)
	again := s.mustUse(t, s.ana, s.pens, potionCommon, same)
	for name, res := range map[string]*playv1.UseConsumableResponse{"the first answer": first, "the retry": again} {
		if len(res.GetVitals()) != 0 || res.GetHealed() == 0 || res.GetTargetCharacterId() != s.toren.GetId() {
			t.Errorf("%s = %v, want the healed amount and the receiver, no vitals", name, res)
		}
	}
	s.hurt(t, s.toren, 1)
	if res := s.mustUse(t, s.master, s.pens, potionCommon, giveTo(s.toren), func(r *playv1.UseConsumableRequest) { r.EntryId = s.entryOf(t, s.pens, potionCommon) }); len(res.GetVitals()) != 1 {
		t.Errorf("the master's answer = %v, want the receiver's vitals", res)
	}
}

// A creature at 0 hit points does not act (SRD: it is unconscious), but another creature may give it a potion that
// wakes it.
func TestAnUnconsciousCreatureCannotUseAPotionButMayReceiveOne(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	s.hurt(t, s.pens, 0)
	_, err := s.use(t, s.ana, s.pens, potionCommon)
	wantBlockedBy(t, "the unconscious user", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_COMBATANT_DOWN)
	if !s.holds(t, s.pens, potionCommon) {
		t.Fatal("the refused use spent the potion")
	}
	res := s.mustUse(t, s.caio, s.toren, potionCommon, giveTo(s.pens))
	if res.GetHealed() == 0 || s.vitals(t, s.pens).GetHitPointsCurrent() == 0 {
		t.Errorf("the potion did not wake Pensantus: %v", res)
	}
}

// Only a player's character of the campaign receives a potion: not an NPC, not one of another campaign, not a stranger id.
func TestAPotionIsGivenToAPlayersCharacterOfTheCampaignOnly(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	other := s.h.newCampaign(s.master, "Outra", s.ana)
	scores := &rulesv1.AbilityScores{Strength: 10, Dexterity: 10, Constitution: 10, Intelligence: 10, Wisdom: 10, Charisma: 10}
	foreign := s.ana.hero(t, other, "Forasteira", "class:fighter", "race:human", 1, scores, nil, nil)
	for name, id := range map[string]string{"an NPC": s.goblin.GetId(), "another campaign's": foreign.GetId(), "unknown": newKey()} {
		_, err := s.use(t, s.ana, s.pens, potionCommon, func(r *playv1.UseConsumableRequest) { r.TargetCharacterId = id })
		wantCode(t, "a potion given to "+name, err, connect.CodeNotFound)
	}
	if !s.holds(t, s.pens, potionCommon) {
		t.Error("a refused gift spent the potion")
	}
}

// In a beast form the healing is the beast's (Wild Shape), in a combat as out of one.
func TestAHealingPotionOutsideACombatHealsTheBeastFormOfADruid(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	s.bri = s.withItems(t, s.bri, potionCommon)
	st := &stocked{s.armed}
	s.mustAssume(t, s.bia, s.bri, wolfKey)
	s.correct(t, s.bri, func(r *playv1.AdjustCharacterVitalsRequest) { r.WildShapeHitPointsCurrent = new(int32(3)) })
	own := s.vitals(t, s.bri).GetHitPointsCurrent()
	s.h.roller.queue(2, 3) // 2d4 + 2 = 7
	res := st.mustUse(t, s.bia, s.bri, potionCommon)
	v := s.vitals(t, s.bri)
	if res.GetHealed() != 7 || v.GetWildShape().GetHitPointsCurrent() != 10 || v.GetHitPointsCurrent() != own {
		t.Errorf("healed %d: beast %d (want 10), own %d (want %d)", res.GetHealed(), v.GetWildShape().GetHitPointsCurrent(), v.GetHitPointsCurrent(), own)
	}
}

// The master may use a potion of an NPC's sheet (an NPC has no hit points of its own outside a combat, so the use is spent and
// logged and the master tells what it did) and give it to a player's character, whom it heals.
func TestTheMasterUsesAPotionOfAnNPCsSheet(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	boss := s.withItems(t, s.master.npcWizard(t, s.campaignID, "Mago Rival"), potionCommon, potionCommon)
	_, err := s.use(t, s.caio, boss, potionCommon)
	wantCode(t, "a player using an NPC's potion", err, connect.CodePermissionDenied)
	res, err := s.use(t, s.master, boss, potionCommon)
	if err != nil {
		t.Fatalf("the NPC's own potion: %v", err)
	}
	if res.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_TEXT_ONLY || res.GetHealed() != 0 {
		t.Errorf("the NPC's own potion = %v, want it spent with no automatic effect", res)
	}
	s.hurt(t, s.toren, 1)
	res, err = s.use(t, s.master, boss, potionCommon, giveTo(s.toren))
	if err != nil || res.GetHealed() == 0 || s.vitals(t, s.toren).GetHitPointsCurrent() != 1+res.GetHealed() {
		t.Errorf("the NPC's potion given to Toren = %v, %v; want Toren healed", res, err)
	}
	if s.holds(t, boss, potionCommon) {
		t.Error("the NPC still holds a potion it used twice")
	}
}
