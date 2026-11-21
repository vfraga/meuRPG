package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The Circle of the Land druid's Natural Recovery (SRD 5.1, Druid 2): after a short rest, once a
// day, expended slots of a combined level up to half the DRUID level (rounded up) come back, none
// of the 6th level or higher. It shares the keeper of Arcane Recovery (resources_arcane_test.go
// covers the shared refusals for the wizard); these tests need the database
// (MEURPG_TEST_DATABASE_URL). There is no undo: the master corrects a wrong recovery with
// AdjustCharacterVitals, as for Arcane Recovery.

// newPlayer joins one more player to the campaign: a player has one living character per campaign.
func (r *restTable) newPlayer(name string) *user {
	u := r.h.newUser(name)
	r.h.join(r.master, r.campaign, u)
	return u
}

// landDruid creates a player's druid of the level in the Circle of the Land, Wisdom 16.
func (r *restTable) landDruid(t *testing.T, u *user, name string, level int32) *charactersv1.Character {
	t.Helper()
	scores := abilities(10, 14, 14, 16, 10)
	return u.heroSheet(t, r.campaign, name, &charactersv1.FullSheet{
		BaseScores: scores, RaceKey: "race:human",
		Classes: []*charactersv1.ClassLevel{classLevel("class:druid", level, "subclass:land")},
	})
}

// recoverNatural calls UseNaturalRecovery as u for the slots (spell level to count).
func (r *restTable) recoverNatural(t *testing.T, u *user, c *charactersv1.Character, key string, slots map[int32]int32) (*playv1.UseNaturalRecoveryResponse, error) {
	t.Helper()
	req := &playv1.UseNaturalRecoveryRequest{CampaignId: r.campaign, CharacterId: c.GetId(), IdempotencyKey: key}
	for level := int32(1); level <= 9; level++ {
		if n, ok := slots[level]; ok {
			req.Slots = append(req.Slots, &playv1.SpellSlotBack{Level: level, Count: n})
		}
	}
	res, err := u.resource.UseNaturalRecovery(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// A level 3 druid recovers slots of a combined level 2. The use is then spent until a long rest
// gives it back, and a retry with the same key changes nothing.
func TestNaturalRecoveryRecoversHalfTheDruidLevelOnceUntilALongRest(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	druid := r.landDruid(t, r.ana, "Silvana", 3)
	if v := r.vitals(t, r.ana, druid); v.GetNaturalRecoveryAllowance() != 2 || usedOf(v, "natural_recovery") != 0 || v.GetArcaneRecoveryAllowance() != 0 {
		t.Fatalf("natural allowance = %d, arcane = %d, uses spent = %d; want 2, 0 and 0",
			v.GetNaturalRecoveryAllowance(), v.GetArcaneRecoveryAllowance(), usedOf(v, "natural_recovery"))
	}
	r.expend(t, druid, map[int32]int32{1: 4, 2: 2})
	r.shortRest(t)

	key := newKey()
	res, err := r.recoverNatural(t, r.ana, druid, key, map[int32]int32{1: 2})
	if err != nil {
		t.Fatalf("UseNaturalRecovery() error = %v", err)
	}
	if v := res.GetVitals(); res.GetRecoveredLevels() != 2 || slotsUsed(v, 1) != 2 || slotsUsed(v, 2) != 2 || usedOf(v, "natural_recovery") != 1 {
		t.Errorf("after: recovered %d levels, used 1st=%d 2nd=%d, uses spent %d; want 2, 2, 2 and 1",
			res.GetRecoveredLevels(), slotsUsed(v, 1), slotsUsed(v, 2), usedOf(v, "natural_recovery"))
	}
	if _, err := r.recoverNatural(t, r.ana, druid, key, map[int32]int32{1: 2}); err != nil {
		t.Fatalf("the retry error = %v", err)
	}
	if v := r.vitals(t, r.ana, druid); slotsUsed(v, 1) != 2 {
		t.Errorf("the retry gave back more: 1st-level slots used = %d, want 2", slotsUsed(v, 1))
	}
	r.shortRest(t)
	_, err = r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{1: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_USES_LEFT)
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_LONG); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	if v := r.vitals(t, r.ana, druid); usedOf(v, "natural_recovery") != 0 {
		t.Fatalf("uses spent after a long rest = %d, want 0", usedOf(v, "natural_recovery"))
	}
	r.expend(t, druid, map[int32]int32{2: 2})
	r.shortRest(t)
	res, err = r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{2: 1})
	if err != nil || slotsUsed(res.GetVitals(), 2) != 1 {
		t.Errorf("a 2nd-level slot after the long rest: err = %v, 2nd-level slots used = %d; want nil and 1", err, slotsUsed(res.GetVitals(), 2))
	}
}

// 3rd-level + 1st-level is combined level 4: over the allowance of a level 5 druid (3).
func TestNaturalRecoveryRefusesAnOverAllowanceTotalAndASixthLevelSlot(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	druid := r.landDruid(t, r.ana, "Silvana", 5)
	r.expend(t, druid, map[int32]int32{1: 2, 3: 1})
	r.shortRest(t)
	_, err := r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{3: 1, 1: 1})
	b := wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_OVER_ALLOWANCE)
	if b.GetNeeded() != 4 || b.GetAvailable() != 3 {
		t.Errorf("needed %d, available %d; want 4 and 3", b.GetNeeded(), b.GetAvailable())
	}
	if v := r.vitals(t, r.ana, druid); usedOf(v, "natural_recovery") != 0 || slotsUsed(v, 3) != 1 {
		t.Errorf("the refusal changed the vitals: uses spent %d, 3rd-level used %d", usedOf(v, "natural_recovery"), slotsUsed(v, 3))
	}
	res, err := r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{3: 1})
	if err != nil || res.GetRecoveredLevels() != 3 || slotsUsed(res.GetVitals(), 3) != 0 {
		t.Errorf("the 3rd-level slot alone: err = %v, levels = %d; want nil and 3", err, res.GetRecoveredLevels())
	}

	// A level 11 druid has an allowance of 6 and a 6th-level slot, and still cannot recover it.
	velha := r.newPlayer("Velha")
	elder := r.landDruid(t, velha, "Velha", 11)
	r.expend(t, elder, map[int32]int32{5: 1, 6: 1})
	r.shortRest(t)
	_, err = r.recoverNatural(t, velha, elder, newKey(), map[int32]int32{6: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SLOT_LEVEL_TOO_HIGH)
	if res, err := r.recoverNatural(t, velha, elder, newKey(), map[int32]int32{5: 1}); err != nil || slotsUsed(res.GetVitals(), 6) != 1 {
		t.Errorf("a 5th-level slot: err = %v; want nil with the 6th-level slot still used", err)
	}
}

// Only expended slots come back.
func TestNaturalRecoveryRefusesASlotThatIsNotExpended(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	druid := r.landDruid(t, r.ana, "Silvana", 3)
	r.expend(t, druid, map[int32]int32{1: 1})
	r.shortRest(t)
	_, err := r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{1: 2})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SLOT_NOT_EXPENDED)
	_, err = r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{2: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SLOT_NOT_EXPENDED)
	if _, err := r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{1: 1}); err != nil {
		t.Errorf("the one expended slot error = %v", err)
	}
}

// Natural Recovery is the Circle of the Land's: a druid of no circle, a wizard and a fighter do not
// have it, and a Land druid has no Arcane Recovery.
func TestNaturalRecoveryIsNotAvailableWithoutTheLandFeature(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	scores := abilities(10, 14, 14, 16, 10)
	plain := r.ana.heroSheet(t, r.campaign, "Folha", &charactersv1.FullSheet{
		BaseScores: scores, RaceKey: "race:human", Classes: []*charactersv1.ClassLevel{classLevel("class:druid", 3, "")},
	})
	elora, silvana := r.newPlayer("Elora"), r.newPlayer("Silvana")
	mage := r.wizard(t, elora, "Elora", 3)
	land := r.landDruid(t, silvana, "Silvana", 3)
	r.expend(t, plain, map[int32]int32{1: 2})
	r.expend(t, mage, map[int32]int32{1: 2})
	r.expend(t, land, map[int32]int32{1: 2})
	r.shortRest(t)
	for name, c := range map[string]struct {
		u *user
		c *charactersv1.Character
	}{"druid of no circle": {r.ana, plain}, "wizard": {elora, mage}, "fighter": {r.bia, r.fighter}} {
		_, err := r.recoverNatural(t, c.u, c.c, newKey(), map[int32]int32{1: 1})
		wantCode(t, "UseNaturalRecovery("+name+")", err, connect.CodeFailedPrecondition)
		wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_AVAILABLE)
	}
	if v := r.vitals(t, r.ana, plain); v.GetNaturalRecoveryAllowance() != 0 {
		t.Errorf("a druid of no circle has an allowance of %d, want 0", v.GetNaturalRecoveryAllowance())
	}
	_, err := r.recoverSlots(t, silvana, land, newKey(), map[int32]int32{1: 1}) // Arcane Recovery
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_AVAILABLE)
}

// "When you finish a short rest": with no rest, or after a long rest, it is refused and the use
// stays; a malformed list of slots is invalid_argument.
func TestNaturalRecoveryNeedsAShortRest(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	druid := r.landDruid(t, r.ana, "Silvana", 3)
	r.expend(t, druid, map[int32]int32{1: 2})
	_, err := r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{1: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_SHORT_REST)
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_LONG); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	r.expend(t, druid, map[int32]int32{1: 2})
	_, err = r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{1: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_SHORT_REST)
	if v := r.vitals(t, r.ana, druid); usedOf(v, "natural_recovery") != 0 || slotsUsed(v, 1) != 2 {
		t.Errorf("a refused call changed the vitals: uses spent %d, 1st-level used %d", usedOf(v, "natural_recovery"), slotsUsed(v, 1))
	}
	r.shortRest(t)
	for name, slots := range map[string]map[int32]int32{"none": {}, "level 0": {0: 1}, "count 0": {1: 0}, "level 10": {10: 1}} {
		_, err := r.recoverNatural(t, r.ana, druid, newKey(), slots)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("slots %s = %v, want invalid_argument", name, err)
		}
	}
	if _, err := r.recoverNatural(t, r.ana, druid, newKey(), map[int32]int32{1: 1}); err != nil {
		t.Errorf("after the short rest error = %v", err)
	}
}

// The character's player or the master; another player is refused.
func TestNaturalRecoveryIsTheCharactersPlayersOrTheMasters(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	druid := r.landDruid(t, r.ana, "Silvana", 3)
	r.expend(t, druid, map[int32]int32{1: 2})
	r.shortRest(t)
	_, err := r.recoverNatural(t, r.bia, druid, newKey(), map[int32]int32{1: 1})
	wantCode(t, "UseNaturalRecovery(Bia)", err, connect.CodePermissionDenied)
	if v := r.vitals(t, r.ana, druid); usedOf(v, "natural_recovery") != 0 {
		t.Fatalf("a refused call spent the use")
	}
	if _, err := r.recoverNatural(t, r.master, druid, newKey(), map[int32]int32{1: 1}); err != nil {
		t.Errorf("the master error = %v", err)
	}
}

// Multiclass: only the druid level counts (Fighter 4 / Land druid 2 recovers 1 level, not 3).
func TestNaturalRecoveryCountsOnlyTheDruidLevel(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	doran := r.ana.heroSheet(t, r.campaign, "Doran", &charactersv1.FullSheet{
		BaseScores: abilities(14, 12, 14, 14, 10), RaceKey: "race:human",
		Classes: []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 4}, classLevel("class:druid", 2, "subclass:land")},
	})
	if v := r.vitals(t, r.ana, doran); v.GetNaturalRecoveryAllowance() != 1 {
		t.Fatalf("allowance = %d, want 1: half of the druid level 2", v.GetNaturalRecoveryAllowance())
	}
	r.expend(t, doran, map[int32]int32{1: 2})
	r.shortRest(t)
	_, err := r.recoverNatural(t, r.ana, doran, newKey(), map[int32]int32{1: 2})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_OVER_ALLOWANCE)
	out, err := r.recoverNatural(t, r.ana, doran, newKey(), map[int32]int32{1: 1})
	if err != nil || out.GetRecoveredLevels() != 1 || slotsUsed(out.GetVitals(), 1) != 1 {
		t.Errorf("one 1st-level slot: err = %v, levels = %d, used = %d; want nil, 1 and 1", err, out.GetRecoveredLevels(), slotsUsed(out.GetVitals(), 1))
	}
}

// A stored answer goes to the user who made the first call: another member, the master and the character's new player are
// refused as a reused key and no slot is given back again, while the first caller's retry gets the answer, also after the
// character changed hands.
func TestNaturalRecoveryAnswersAStoredCallToItsAuthorOnly(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	druid := r.landDruid(t, r.ana, "Silvana", 3)
	r.expend(t, druid, map[int32]int32{1: 2})
	r.shortRest(t)
	key := newKey()
	slots := map[int32]int32{1: 1}
	first, err := r.recoverNatural(t, r.ana, druid, key, slots)
	if err != nil {
		t.Fatalf("UseNaturalRecovery() error = %v", err)
	}
	check := func(step string) {
		t.Helper()
		v := r.vitals(t, r.master, druid)
		if slotsUsed(v, 1) != 1 || usedOf(v, "natural_recovery") != 1 {
			t.Errorf("%s: slots used = %d, use = %d, want 1 and 1", step, slotsUsed(v, 1), usedOf(v, "natural_recovery"))
		}
		if n := r.h.eventsOfKind(t, r.campaign, eventNaturalRecovery); n != 1 {
			t.Errorf("%s: the session has %d natural_recovery_used events, want 1", step, n)
		}
	}
	retry := func(step string) {
		t.Helper()
		again, err := r.recoverNatural(t, r.ana, druid, key, slots)
		if err != nil {
			t.Fatalf("%s: UseNaturalRecovery() error = %v", step, err)
		}
		if again.GetRecoveredLevels() != first.GetRecoveredLevels() || slotsUsed(again.GetVitals(), 1) != 1 {
			t.Errorf("%s: answer = %d levels, %d used; first = %d levels", step, again.GetRecoveredLevels(), slotsUsed(again.GetVitals(), 1), first.GetRecoveredLevels())
		}
	}
	_, err = r.recoverNatural(t, r.bia, druid, key, slots)
	wantKeyReused(t, "another member's UseNaturalRecovery with the key", err)
	check("another member")
	_, err = r.recoverNatural(t, r.master, druid, key, slots)
	wantKeyReused(t, "the master's UseNaturalRecovery with the key", err)
	check("the master")
	retry("the author")
	dora := r.h.newUser("Dora")
	r.h.join(r.master, r.campaign, dora)
	r.h.giveCharacter(t, druid.GetId(), dora)
	_, err = r.recoverNatural(t, dora, druid, key, slots)
	wantKeyReused(t, "the new player's UseNaturalRecovery with the key", err)
	check("the new player")
	retry("the author after the gift")
	check("the author's retry")
}
