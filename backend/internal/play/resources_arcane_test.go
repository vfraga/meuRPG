package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The wizard's Arcane Recovery (SRD 5.1, Wizard): after a short rest, once a day, expended
// slots of a combined level up to half the WIZARD level (rounded up) come back, none of the
// 6th level or higher. These tests need the database (MEURPG_TEST_DATABASE_URL).

// newRecoveryTable (Arcane and Natural Recovery) is a campaign with a master, Ana (whose character the test makes) and Bia, a
// fighter, with a session open.
func newRecoveryTable(t *testing.T) *restTable {
	t.Helper()
	h := newHarness(t)
	r := &restTable{h: h, master: h.newUser("Mestre"), ana: h.newUser("Ana"), bia: h.newUser("Bia")}
	r.campaign = h.newCampaign(r.master, "Mirathel", r.ana, r.bia)
	r.fighter = r.bia.hero(t, r.campaign, "Toren", "class:fighter", "race:human", 4, abilities(16, 13, 14, 10, 10), []string{battleaxe}, nil)
	r.session = r.master.start(t, r.campaign).GetGameSession()
	return r
}

// wizard creates a player's wizard of the level, Intelligence 16.
func (r *restTable) wizard(t *testing.T, u *user, name string, level int32) *charactersv1.Character {
	t.Helper()
	scores := abilities(10, 14, 14, 10, 10)
	scores.Intelligence = 16
	return u.hero(t, r.campaign, name, "class:wizard", "race:human", level, scores, nil, nil)
}

// expend marks the slots as spent, by spell level, as the master's hand does.
func (r *restTable) expend(t *testing.T, c *charactersv1.Character, used map[int32]int32) {
	t.Helper()
	r.use(t, c, func(q *playv1.AdjustCharacterVitalsRequest) {
		for level, n := range used {
			q.SpellSlotsUsed = append(q.SpellSlotsUsed, &playv1.SpellSlotsUsed{Level: level, Used: n})
		}
	})
}

func slotsUsed(v *playv1.CharacterVitals, level int32) int32 {
	for _, s := range v.GetSpellSlots() {
		if s.GetLevel() == level {
			return s.GetUsed()
		}
	}
	return -1
}

// recoverSlots calls UseArcaneRecovery as u for the slots (spell level to count).
func (r *restTable) recoverSlots(t *testing.T, u *user, c *charactersv1.Character, key string, slots map[int32]int32) (*playv1.UseArcaneRecoveryResponse, error) {
	t.Helper()
	req := &playv1.UseArcaneRecoveryRequest{CampaignId: r.campaign, CharacterId: c.GetId(), IdempotencyKey: key}
	for level := int32(1); level <= 9; level++ {
		if n, ok := slots[level]; ok {
			req.Slots = append(req.Slots, &playv1.SpellSlotBack{Level: level, Count: n})
		}
	}
	res, err := u.resource.UseArcaneRecovery(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (r *restTable) shortRest(t *testing.T) {
	t.Helper()
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_SHORT); err != nil {
		t.Fatalf("TakeRest(short) error = %v", err)
	}
}

// A level 3 wizard recovers slots of a combined level 2: two 1st-level slots, or one
// 2nd-level slot. The use is then spent until a long rest gives it back.
func TestArcaneRecoveryRecoversHalfTheWizardLevelOnceUntilALongRest(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	mage := r.wizard(t, r.ana, "Elora", 3)
	if v := r.vitals(t, r.ana, mage); v.GetArcaneRecoveryAllowance() != 2 || usedOf(v, "arcane_recovery") != 0 {
		t.Fatalf("allowance = %d, uses spent = %d; want 2 and 0", v.GetArcaneRecoveryAllowance(), usedOf(v, "arcane_recovery"))
	}
	r.expend(t, mage, map[int32]int32{1: 4, 2: 2})
	r.shortRest(t)

	key := newKey()
	res, err := r.recoverSlots(t, r.ana, mage, key, map[int32]int32{1: 2})
	if err != nil {
		t.Fatalf("UseArcaneRecovery() error = %v", err)
	}
	if v := res.GetVitals(); res.GetRecoveredLevels() != 2 || slotsUsed(v, 1) != 2 || slotsUsed(v, 2) != 2 || usedOf(v, "arcane_recovery") != 1 {
		t.Errorf("after: recovered %d levels, used 1st=%d 2nd=%d, uses spent %d; want 2, 2, 2 and 1",
			res.GetRecoveredLevels(), slotsUsed(v, 1), slotsUsed(v, 2), usedOf(v, "arcane_recovery"))
	}
	// The same call again (a retry) changes nothing.
	if _, err := r.recoverSlots(t, r.ana, mage, key, map[int32]int32{1: 2}); err != nil {
		t.Fatalf("the retry error = %v", err)
	}
	if v := r.vitals(t, r.ana, mage); slotsUsed(v, 1) != 2 {
		t.Errorf("the retry gave back more: 1st-level slots used = %d, want 2", slotsUsed(v, 1))
	}
	// A second use before a long rest is refused, a short rest in between does not help.
	r.shortRest(t)
	_, err = r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{1: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_USES_LEFT)
	// The long rest gives the use back (and the slots); after the next short rest it works again.
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_LONG); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	if v := r.vitals(t, r.ana, mage); usedOf(v, "arcane_recovery") != 0 {
		t.Fatalf("uses spent after a long rest = %d, want 0", usedOf(v, "arcane_recovery"))
	}
	r.expend(t, mage, map[int32]int32{2: 2})
	r.shortRest(t)
	res, err = r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{2: 1}) // one 2nd-level slot
	if err != nil || slotsUsed(res.GetVitals(), 2) != 1 {
		t.Errorf("a 2nd-level slot after the long rest: err = %v, 2nd-level slots used = %d; want nil and 1", err, slotsUsed(res.GetVitals(), 2))
	}
}

// 3rd-level + 1st-level is combined level 4: over the allowance of a level 5 wizard (3). The
// refusal spends nothing; the same wizard recovers 3 levels in a 2nd and a 1st.
func TestArcaneRecoveryRefusesAnOverAllowanceTotal(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	mage := r.wizard(t, r.ana, "Elora", 5)
	r.expend(t, mage, map[int32]int32{1: 2, 2: 1, 3: 1})
	r.shortRest(t)
	_, err := r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{3: 1, 1: 1})
	b := wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_OVER_ALLOWANCE)
	if b.GetNeeded() != 4 || b.GetAvailable() != 3 {
		t.Errorf("needed %d, available %d; want 4 and 3", b.GetNeeded(), b.GetAvailable())
	}
	if v := r.vitals(t, r.ana, mage); usedOf(v, "arcane_recovery") != 0 || slotsUsed(v, 3) != 1 {
		t.Errorf("the refusal changed the vitals: uses spent %d, 3rd-level used %d", usedOf(v, "arcane_recovery"), slotsUsed(v, 3))
	}
	res, err := r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{3: 1})
	if err != nil || res.GetRecoveredLevels() != 3 || slotsUsed(res.GetVitals(), 3) != 0 {
		t.Errorf("the 3rd-level slot alone: err = %v, levels = %d; want nil and 3", err, res.GetRecoveredLevels())
	}
}

// "None of the slots can be 6th level or higher": a level 11 wizard has a 6th-level slot and
// an allowance of 6, and still cannot recover it.
func TestArcaneRecoveryRefusesASixthLevelSlot(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	mage := r.wizard(t, r.ana, "Elora", 11)
	r.expend(t, mage, map[int32]int32{5: 1, 6: 1})
	r.shortRest(t)
	_, err := r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{6: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SLOT_LEVEL_TOO_HIGH)
	res, err := r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{5: 1})
	if err != nil || slotsUsed(res.GetVitals(), 5) != 0 || slotsUsed(res.GetVitals(), 6) != 1 {
		t.Errorf("a 5th-level slot: err = %v, 5th used = %d, 6th used = %d; want nil, 0 and 1", err, slotsUsed(res.GetVitals(), 5), slotsUsed(res.GetVitals(), 6))
	}
}

// Only expended slots come back; a character that is not a wizard has no feature.
func TestArcaneRecoveryRefusesASlotThatIsNotExpendedAndANonWizard(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	mage := r.wizard(t, r.ana, "Elora", 3)
	r.expend(t, mage, map[int32]int32{1: 1})
	r.shortRest(t)
	_, err := r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{1: 2})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SLOT_NOT_EXPENDED)
	_, err = r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{2: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SLOT_NOT_EXPENDED)
	if _, err := r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{1: 1}); err != nil {
		t.Errorf("the one expended slot error = %v", err)
	}
	_, err = r.recoverSlots(t, r.bia, r.fighter, newKey(), map[int32]int32{1: 1})
	wantCode(t, "UseArcaneRecovery(Bia's fighter)", err, connect.CodeFailedPrecondition)
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_AVAILABLE)
}

// "When you finish a short rest": with no rest, or after a long rest, it is refused and the
// use stays; a malformed list of slots is invalid_argument.
func TestArcaneRecoveryNeedsAShortRest(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	mage := r.wizard(t, r.ana, "Elora", 3)
	r.expend(t, mage, map[int32]int32{1: 2})
	_, err := r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{1: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_SHORT_REST)
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_LONG); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	r.expend(t, mage, map[int32]int32{1: 2})
	_, err = r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{1: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_SHORT_REST)
	if v := r.vitals(t, r.ana, mage); usedOf(v, "arcane_recovery") != 0 || slotsUsed(v, 1) != 2 {
		t.Errorf("a refused call changed the vitals: uses spent %d, 1st-level used %d", usedOf(v, "arcane_recovery"), slotsUsed(v, 1))
	}
	r.shortRest(t)
	for name, slots := range map[string]map[int32]int32{"none": {}, "level 0": {0: 1}, "count 0": {1: 0}, "level 10": {10: 1}} {
		_, err := r.recoverSlots(t, r.ana, mage, newKey(), slots)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("slots %s = %v, want invalid_argument", name, err)
		}
	}
	if _, err := r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{1: 1}); err != nil {
		t.Errorf("after the short rest error = %v", err)
	}
}

// The character's player or the master; another player is refused.
func TestArcaneRecoveryIsTheCharactersPlayersOrTheMasters(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	mage := r.wizard(t, r.ana, "Elora", 3)
	r.expend(t, mage, map[int32]int32{1: 2})
	r.shortRest(t)
	_, err := r.recoverSlots(t, r.bia, mage, newKey(), map[int32]int32{1: 1})
	wantCode(t, "UseArcaneRecovery(Bia)", err, connect.CodePermissionDenied)
	if v := r.vitals(t, r.ana, mage); usedOf(v, "arcane_recovery") != 0 {
		t.Fatalf("a refused call spent the use")
	}
	if _, err := r.recoverSlots(t, r.master, mage, newKey(), map[int32]int32{1: 1}); err != nil {
		t.Errorf("the master error = %v", err)
	}
}

// Multiclass: only the wizard level counts (Fighter 2 / Wizard 1 recovers 1 level), and the
// slots are the spellcasting slots.
func TestArcaneRecoveryCountsOnlyTheWizardLevel(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	scores := abilities(14, 12, 14, 10, 10)
	scores.Intelligence = 13
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: scores, RaceKey: "race:human",
		Classes: []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}, {ClassKey: "class:wizard", Level: 1}},
	}}}
	res, err := r.ana.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: r.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Doran", Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(multiclass) error = %v", err)
	}
	doran := res.Msg.GetCharacter()
	if v := r.vitals(t, r.ana, doran); v.GetArcaneRecoveryAllowance() != 1 {
		t.Fatalf("allowance = %d, want 1: half of the wizard level 1, rounded up", v.GetArcaneRecoveryAllowance())
	}
	r.expend(t, doran, map[int32]int32{1: 2})
	r.shortRest(t)
	_, err = r.recoverSlots(t, r.ana, doran, newKey(), map[int32]int32{1: 2})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_OVER_ALLOWANCE)
	out, err := r.recoverSlots(t, r.ana, doran, newKey(), map[int32]int32{1: 1})
	if err != nil || out.GetRecoveredLevels() != 1 || slotsUsed(out.GetVitals(), 1) != 1 {
		t.Errorf("one 1st-level slot: err = %v, levels = %d, used = %d; want nil, 1 and 1", err, out.GetRecoveredLevels(), slotsUsed(out.GetVitals(), 1))
	}
}

// A stored answer goes to the user who made the first call: another member, the master and the character's new player are
// refused as a reused key and no slot is given back again, while the first caller's retry gets the answer, also after the
// character changed hands.
func TestArcaneRecoveryAnswersAStoredCallToItsAuthorOnly(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	mage := r.wizard(t, r.ana, "Elora", 3)
	r.expend(t, mage, map[int32]int32{1: 2})
	r.shortRest(t)
	key := newKey()
	slots := map[int32]int32{1: 1}
	first, err := r.recoverSlots(t, r.ana, mage, key, slots)
	if err != nil {
		t.Fatalf("UseArcaneRecovery() error = %v", err)
	}
	check := func(step string) {
		t.Helper()
		v := r.vitals(t, r.master, mage)
		if slotsUsed(v, 1) != 1 || usedOf(v, "arcane_recovery") != 1 {
			t.Errorf("%s: slots used = %d, use = %d, want 1 and 1", step, slotsUsed(v, 1), usedOf(v, "arcane_recovery"))
		}
		if n := r.h.eventsOfKind(t, r.campaign, eventArcaneRecovery); n != 1 {
			t.Errorf("%s: the session has %d arcane_recovery_used events, want 1", step, n)
		}
	}
	retry := func(step string) {
		t.Helper()
		again, err := r.recoverSlots(t, r.ana, mage, key, slots)
		if err != nil {
			t.Fatalf("%s: UseArcaneRecovery() error = %v", step, err)
		}
		if again.GetRecoveredLevels() != first.GetRecoveredLevels() || slotsUsed(again.GetVitals(), 1) != 1 {
			t.Errorf("%s: answer = %d levels, %d used; first = %d levels", step, again.GetRecoveredLevels(), slotsUsed(again.GetVitals(), 1), first.GetRecoveredLevels())
		}
	}
	_, err = r.recoverSlots(t, r.bia, mage, key, slots)
	wantKeyReused(t, "another member's UseArcaneRecovery with the key", err)
	check("another member")
	_, err = r.recoverSlots(t, r.master, mage, key, slots)
	wantKeyReused(t, "the master's UseArcaneRecovery with the key", err)
	check("the master")
	retry("the author")
	dora := r.h.newUser("Dora")
	r.h.join(r.master, r.campaign, dora)
	r.h.giveCharacter(t, mage.GetId(), dora)
	_, err = r.recoverSlots(t, dora, mage, key, slots)
	wantKeyReused(t, "the new player's UseArcaneRecovery with the key", err)
	check("the new player")
	retry("the author after the gift")
	check("the author's retry")
}
