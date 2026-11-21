package play

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Characterization of ConfirmCastTimePassed, AbandonCast and EndActiveSpell, written before the functions were
// split into phases: every refusal with its code and message, and what each caller reads when the call goes
// through. These tests need the database (MEURPG_TEST_DATABASE_URL).

const (
	castEndGolden        = "testdata/casting_end.golden"
	castEndConfirmGolden = "testdata/casting_end_confirm.golden"
)

// castEndStartPrayer starts Brisa's Prayer of Healing at Toren (a long cast) and returns its id.
func castEndStartPrayer(t *testing.T, a *armed) string {
	t.Helper()
	return a.mustCastOut(t, a.bia, a.bri, prayerKey, slotOfLevel(2), false, []*charactersv1.Character{a.toren}).GetCast().GetId()
}

// castEndInterrupt interrupts a cast as the master, so that the character may start another.
func castEndInterrupt(t *testing.T, a *armed, castID string) {
	t.Helper()
	if err := a.interrupt(t, a.master, castID); err != nil {
		t.Fatalf("AbandonCast() error = %v", err)
	}
}

// castEndConfirm calls ConfirmCastTimePassed as u with the request edit changes.
func castEndConfirm(t *testing.T, a *armed, u *user, castID string, edit func(*playv1.ConfirmCastTimePassedRequest)) error {
	t.Helper()
	_, err := a.finish(t, u, castID, edit)
	return err
}

// TestCastEndConfirmRefusals: every refusal of ConfirmCastTimePassed, in the order the checks run.
func TestCastEndConfirmRefusals(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	prayer := castEndStartPrayer(t, a)
	noEdit := func(*playv1.ConfirmCastTimePassedRequest) {}

	// The campaign and the request come first.
	err := castEndConfirm(t, a, a.master, prayer, func(r *playv1.ConfirmCastTimePassedRequest) { r.CampaignId = "nope" })
	wantRefusal(t, "a campaign that is not one", err, connect.CodeNotFound, "campaign not found")
	err = castEndConfirm(t, a, a.master, prayer, func(r *playv1.ConfirmCastTimePassedRequest) { r.IdempotencyKey = "nope" })
	wantRefusal(t, "a key that is not a UUID", err, connect.CodeInvalidArgument, "idempotency_key must be a UUID")
	err = castEndConfirm(t, a, a.master, prayer, func(r *playv1.ConfirmCastTimePassedRequest) {
		r.Roll = &playv1.ConfirmCastTimePassedRequest_RollInApp{RollInApp: false}
	})
	wantRefusal(t, "roll_in_app false", err, connect.CodeInvalidArgument, "roll_in_app must be true")

	// The cast: one that is not found, one the caller may not act on, and who completes it.
	err = castEndConfirm(t, a, a.master, "nope", noEdit)
	wantRefusal(t, "a cast id that is not a UUID", err, connect.CodeNotFound, "cast not found")
	err = castEndConfirm(t, a, a.master, newKey(), noEdit)
	wantRefusal(t, "a cast that does not exist", err, connect.CodeNotFound, "cast not found")
	err = castEndConfirm(t, a, a.ana, prayer, noEdit)
	wantRefusal(t, "another player's cast", err, connect.CodePermissionDenied, "only the campaign's master can do this")
	err = castEndConfirm(t, a, a.bia, prayer, noEdit)
	wantRefusal(t, "the caster's player", err, connect.CodePermissionDenied, "only the campaign's master can do this")

	// A cast that is not being cast.
	done := a.mustCastOut(t, a.ana, a.pens, mageArmorKey, slotOfLevel(1), false, []*charactersv1.Character{a.toren}).GetCast().GetId()
	err = castEndConfirm(t, a, a.master, done, noEdit)
	wantCastingBlocked(t, "a cast that is not going", err, playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_CAST_NOT_GOING)
	wantRefusal(t, "a cast that is not going", err, connect.CodeFailedPrecondition, "the spell is not being cast")

	// A cast that has a creature the players do not see in it: a player is refused as the master's alone, before the cast is read.
	a.execSQL(t, `UPDATE spell_casts SET secret = true WHERE id = $1`, prayer)
	err = castEndConfirm(t, a, a.bia, prayer, noEdit)
	wantRefusal(t, "a secret cast as its player", err, connect.CodePermissionDenied, "only the campaign's master can do this")
}

// TestCastEndConfirmPlanChangedSinceTheStart: the plan is rebuilt when the master confirms, and the sheet has to
// allow it still. The rows are changed in the database, as a sheet changed since the start would leave them.
func TestCastEndConfirmPlanChangedSinceTheStart(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	noEdit := func(*playv1.ConfirmCastTimePassedRequest) {}

	ritual := castEndStartPrayer(t, a)
	a.execSQL(t, `UPDATE spell_casts SET ritual = true WHERE id = $1`, ritual)
	err := castEndConfirm(t, a, a.master, ritual, noEdit)
	wantRefusal(t, "a ritual the cleric cannot cast", err, connect.CodeInvalidArgument, "the character cannot cast this spell as a ritual")

	castEndInterrupt(t, a, ritual)
	stranger := castEndStartPrayer(t, a)
	a.execSQL(t, `UPDATE spell_casts SET spell_key = $2 WHERE id = $1`, stranger, magicMissileSpell)
	err = castEndConfirm(t, a, a.master, stranger, noEdit)
	wantRefusal(t, "a spell the sheet does not prepare", err, connect.CodeInvalidArgument, "spell_key is not one of the caster's spells")

	castEndInterrupt(t, a, stranger)
	noSlot := castEndStartPrayer(t, a)
	a.execSQL(t, `UPDATE spell_casts SET slot_level = 4 WHERE id = $1`, noSlot)
	err = castEndConfirm(t, a, a.master, noSlot, noEdit)
	b := wantCastingBlocked(t, "a slot level the character has not got", err, playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_NO_SLOT)
	wantRefusal(t, "a slot level the character has not got", err, connect.CodeFailedPrecondition, "there is no free spell slot for this spell")
	if b.GetMinLevel() != 4 {
		t.Errorf("min level = %d, want the slot's level 4", b.GetMinLevel())
	}
}

// TestCastEndAbandonAndEndRefusals: the refusals AbandonCast and EndActiveSpell share (the request and the cast
// row), and the ones of EndActiveSpell alone.
func TestCastEndAbandonAndEndRefusals(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	prayer := castEndStartPrayer(t, a)
	calls := map[string]func(u *user, campaign, castID, key string) error{
		"AbandonCast": func(u *user, campaign, castID, key string) error {
			_, err := u.casting.AbandonCast(t.Context(), connect.NewRequest(&playv1.AbandonCastRequest{CampaignId: campaign, CastId: castID, IdempotencyKey: key}))
			return err
		},
		"EndActiveSpell": func(u *user, campaign, castID, key string) error {
			_, err := u.casting.EndActiveSpell(t.Context(), connect.NewRequest(&playv1.EndActiveSpellRequest{CampaignId: campaign, CastId: castID, IdempotencyKey: key}))
			return err
		},
	}
	for name, call := range calls {
		wantRefusal(t, name+" on a campaign that is not one", call(a.master, "nope", prayer, newKey()), connect.CodeNotFound, "campaign not found")
		wantRefusal(t, name+" with a key that is not a UUID", call(a.master, a.campaignID, prayer, "nope"), connect.CodeInvalidArgument, "idempotency_key must be a UUID")
		wantRefusal(t, name+" on a cast id that is not a UUID", call(a.master, a.campaignID, "nope", newKey()), connect.CodeNotFound, "cast not found")
		wantRefusal(t, name+" on a cast that does not exist", call(a.master, a.campaignID, newKey(), newKey()), connect.CodeNotFound, "cast not found")
		wantRefusal(t, name+" on another player's cast", call(a.ana, a.campaignID, prayer, newKey()), connect.CodePermissionDenied, "only the character's player or the master may do this")
	}
	a.execSQL(t, `UPDATE spell_casts SET secret = true WHERE id = $1`, prayer)
	for name, call := range calls {
		wantRefusal(t, name+" on a secret cast as a player", call(a.bia, a.campaignID, prayer, newKey()), connect.CodeNotFound, "cast not found")
	}
	a.execSQL(t, `UPDATE spell_casts SET secret = false WHERE id = $1`, prayer)
	// A cast being cast is interrupted, not ended.
	_, err := a.endSpell(t, a.bia, prayer)
	wantCastingBlocked(t, "EndActiveSpell on a cast being cast", err, playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_CAST_NOT_ACTIVE)
	wantRefusal(t, "EndActiveSpell on a cast being cast", err, connect.CodeFailedPrecondition, "the spell is still being cast: interrupt the casting")
}

// castEndNames names the ids a response holds: the characters of the party and the casts by what they are.
func castEndNames(a *armed, casts map[string]string) map[string]string {
	names := map[string]string{a.toren.GetId(): "Toren", a.pens.GetId(): "Pensantus", a.bri.GetId(): "Brisa"}
	for id, n := range casts {
		names[id] = n
	}
	return names
}

// castEndJSON is spellJSON with the instants replaced, which differ from one run to the next.
func castEndJSON(m proto.Message, names map[string]string) string {
	return castTimePattern.ReplaceAllString(spellJSON(m, names), "<time>")
}

// castEndAid casts Brisa's Aid (2nd level, three targets: it raises their maximum hit points) and returns its id.
func castEndAid(t *testing.T, a *armed) string {
	t.Helper()
	return a.mustCastOut(t, a.bia, a.bri, aidSpell, slotOfLevel(2), false, []*charactersv1.Character{a.toren, a.pens, a.bri}).GetCast().GetId()
}

// TestCastEndTheEndingIsWhatEachCallerReads: EndActiveSpell for an Aid that raised three characters' maximum, as
// the caster's player (who reads the vitals of his own character only) and as the master (who reads them all),
// what a second call on the same cast reads (nothing changes, no event), and the dismissal of a cast already over.
func TestCastEndTheEndingIsWhatEachCallerReads(t *testing.T) {
	t.Parallel()
	g := &goldenBlocks{}
	for _, who := range []struct {
		name string
		as   func(a *armed) *user
	}{{"the caster's player", asBia}, {"the master", castEndAsMaster}} {
		a := newCastingParty(t)
		id := castEndAid(t, a)
		names := castEndNames(a, map[string]string{id: "cast"})
		before := a.eventCount(t, eventSpellCastEnded)
		res, err := a.endSpell(t, who.as(a), id)
		if err != nil {
			t.Fatalf("EndActiveSpell(%s) error = %v", who.name, err)
		}
		again, err := a.endSpell(t, who.as(a), id)
		if err != nil {
			t.Fatalf("EndActiveSpell(%s) again error = %v", who.name, err)
		}
		if n := a.eventCount(t, eventSpellCastEnded) - before; n != 1 {
			t.Errorf("%s: spell_cast_ended events written = %d, want 1 (a second call changes nothing)", who.name, n)
		}
		g.put("aid ended by "+who.name, "first "+castEndJSON(res, names)+"\nagain "+castEndJSON(again, names)+"\nmax hp "+
			a.castEndMaxima(t, a.toren, a.pens, a.bri))
	}
	g.settle(t, castEndGolden)
}

// castEndMaxima is the maximum hit points of the characters, in the order given.
func (a *armed) castEndMaxima(t *testing.T, cs ...*charactersv1.Character) string {
	t.Helper()
	out := ""
	for _, c := range cs {
		out += c.GetName() + "=" + fmt.Sprint(a.vitals(t, c).GetHitPointsMax()) + " "
	}
	return out
}

// TestCastEndAConcentrationInACombatIsEndedThere: a concentration spell carried into a combat is ended in the
// combat while it lasts, and from outside once the combat is over.
func TestCastEndAConcentrationInACombatIsEndedThere(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	bless := a.mustCastOut(t, a.bia, a.bri, blessKey, slotOfLevel(1), false, []*charactersv1.Character{a.toren}).GetCast()
	if bless.GetStatus() != playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_ACTIVE || !bless.GetConcentrating() {
		t.Fatalf("Bless = %v, want an active concentration", bless)
	}
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{2},
		players: map[string]int32{"Toren": 12, "Brisa": 11, "Pensantus": 5}, reveal: []string{"Goblin"},
		at: map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Brisa": {6, 5}},
	})
	_, err := a.endSpell(t, a.bia, bless.GetId())
	wantCastingBlocked(t, "EndActiveSpell in a combat", err, playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_IN_COMBAT)
	wantRefusal(t, "EndActiveSpell in a combat", err, connect.CodeFailedPrecondition, "the concentration is in a combat: end it there")
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	res, err := a.endSpell(t, a.bia, bless.GetId())
	if err != nil {
		t.Fatalf("EndActiveSpell after the combat error = %v", err)
	}
	if c := res.GetCast(); c.GetStatus() != playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_ENDED || c.GetEndReason() != playv1.OutsideCastEnd_OUTSIDE_CAST_END_DISMISSED {
		t.Errorf("cast after the combat = %v, want ended, dismissed", c)
	}
}

// TestCastEndConfirmIsWhatEachCallerReads: ConfirmCastTimePassed with the sum of physical dice, as the master, then
// the same cast read by the caster's player, and AbandonCast on a cast that is not being cast any more.
func TestCastEndConfirmIsWhatEachCallerReads(t *testing.T) {
	t.Parallel()
	g := &goldenBlocks{}
	a := newCastingParty(t)
	id := castEndStartPrayer(t, a)
	names := castEndNames(a, map[string]string{id: "cast"})
	a.hurt(t, a.toren, 1)
	res, err := a.finish(t, a.master, id, func(r *playv1.ConfirmCastTimePassedRequest) {
		r.Roll = &playv1.ConfirmCastTimePassedRequest_TypedSum{TypedSum: 9}
	})
	if err != nil {
		t.Fatalf("ConfirmCastTimePassed() error = %v", err)
	}
	g.put("prayer confirmed with a typed sum", castEndJSON(res, names)+"\nplayer reads "+castEndJSON(a.castByID(t, a.bia, id), names)+
		"\ntoren hp "+fmt.Sprint(a.vitals(t, a.toren).GetHitPointsCurrent()))
	again := a.interrupt(t, a.bia, id)
	g.put("abandon of a cast that is over", fmt.Sprint(again)+" "+castEndJSON(a.castByID(t, a.master, id), names))
	g.settle(t, castEndConfirmGolden)
}
