package play

import (
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The rests and the hit dice (SRD 5.1, "Resting"): the master's short and long
// rest restore what the SRD says, a player spends hit dice on a short rest, and
// everything is recorded, idempotent and told to the session. These tests need
// the database (MEURPG_TEST_DATABASE_URL).

// restTable is a campaign with a master and two players, a session open, and the
// party the rests are tried on: a fighter (Ana), a barbarian (Bia) and a multiclass fighter and
// wizard (Caio).
type restTable struct {
	h                                *harness
	master, ana, bia, caio           *user
	campaign                         string
	session                          *playv1.GameSession
	fighter, barbarian, multiclassed *charactersv1.Character
}

func newRestTable(t *testing.T) *restTable {
	t.Helper()
	h := newHarness(t)
	r := &restTable{h: h, master: h.newUser("Mestre"), ana: h.newUser("Ana"), bia: h.newUser("Bia"), caio: h.newUser("Caio")}
	r.campaign = h.newCampaign(r.master, "Mirathel", r.ana, r.bia, r.caio)
	r.fighter = r.ana.hero(t, r.campaign, "Toren", "class:fighter", "race:human", 4, abilities(16, 13, 14, 10, 10), []string{battleaxe}, nil)
	r.barbarian = r.bia.hero(t, r.campaign, "Ragna", "class:barbarian", "race:human", 3, abilities(16, 13, 14, 10, 10), []string{battleaxe}, nil)
	// Multiclassing needs 13 in the main ability of each class (SRD 5.1): Strength for the fighter, Intelligence for the wizard.
	scores := abilities(14, 12, 14, 14, 10)
	scores.Intelligence = 13
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: scores, RaceKey: "race:human",
		Classes: []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 3}, {ClassKey: "class:wizard", Level: 2}},
	}}}
	res, err := r.caio.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: r.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Doran", Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(multiclass) error = %v", err)
	}
	r.multiclassed = res.Msg.GetCharacter()
	r.session = r.master.start(t, r.campaign).GetGameSession()
	return r
}

// use sets what a character has used, as the master's hand does (AdjustCharacterVitals).
func (r *restTable) use(t *testing.T, c *charactersv1.Character, edit func(*playv1.AdjustCharacterVitalsRequest)) *playv1.CharacterVitals {
	t.Helper()
	v, err := r.master.adjust(t, r.campaign, c.GetId(), edit)
	if err != nil {
		t.Fatalf("AdjustCharacterVitals(%s) error = %v", c.GetName(), err)
	}
	return v
}

func (r *restTable) vitals(t *testing.T, u *user, c *charactersv1.Character) *playv1.CharacterVitals {
	t.Helper()
	for _, v := range u.liveSession(t, r.campaign).GetVitals() {
		if v.GetCharacterId() == c.GetId() {
			return v
		}
	}
	t.Fatalf("no vitals of %s for the caller", c.GetName())
	return nil
}

func (r *restTable) rest(t *testing.T, kind playv1.RestKind, edit ...func(*playv1.TakeRestRequest)) (*playv1.TakeRestResponse, error) {
	t.Helper()
	req := &playv1.TakeRestRequest{CampaignId: r.campaign, Kind: kind, IdempotencyKey: newKey()}
	for _, e := range edit {
		e(req)
	}
	res, err := r.master.resource.TakeRest(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func usedOf(v *playv1.CharacterVitals, key string) int32 {
	for _, r := range v.GetResources() {
		if r.GetKey() == key {
			return r.GetUsed()
		}
	}
	return -1
}

func wantResourceBlocked(t *testing.T, err error, want playv1.ResourceBlockedReason) *playv1.ResourceBlocked {
	t.Helper()
	wantCode(t, "call", err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		msg, derr := d.Value()
		if derr != nil {
			continue
		}
		if b, ok := msg.(*playv1.ResourceBlocked); ok {
			if b.GetReason() != want {
				t.Fatalf("blocked reason = %v, want %v", b.GetReason(), want)
			}
			return b
		}
	}
	t.Fatalf("error %v has no ResourceBlocked detail", err)
	return nil
}

func hitDiceUsed(v *playv1.CharacterVitals) map[int32]int32 { return v.GetHitDiceUsedByDie() }

// SRD 5.1, "Short Rest": a short rest gives back the resources that recharge on a
// short or long rest, and the pact slots; not the ones that need a long rest, the
// hit points or the ordinary spell slots.
func TestAShortRestGivesBackWhatRechargesOnIt(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.HitPointsCurrent = proto.Int32(5)
		q.HitPointsTemporary = proto.Int32(4)
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "second_wind", Used: 1}, {Key: "action_surge", Used: 1}}
	})
	r.use(t, r.barbarian, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "rage", Used: 2}}
	})
	r.use(t, r.multiclassed, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 1, Used: 2}}
	})
	res, err := r.rest(t, playv1.RestKind_REST_KIND_SHORT)
	if err != nil {
		t.Fatalf("TakeRest(short) error = %v", err)
	}
	// Only the fighter had something a short rest gives back: rage and the slots wait for a long one.
	if len(res.GetVitals()) != 1 || res.GetVitals()[0].GetCharacterId() != r.fighter.GetId() {
		t.Errorf("the short rest touched %v, want the fighter alone", res.GetVitals())
	}
	f := r.vitals(t, r.master, r.fighter)
	if usedOf(f, "second_wind") != 0 || usedOf(f, "action_surge") != 0 {
		t.Errorf("fighter's second wind / action surge used = %d / %d, want both back after a short rest", usedOf(f, "second_wind"), usedOf(f, "action_surge"))
	}
	if f.GetHitPointsCurrent() != 5 || f.GetHitPointsTemporary() != 4 {
		t.Errorf("a short rest changed the hit points: %d current, %d temporary", f.GetHitPointsCurrent(), f.GetHitPointsTemporary())
	}
	if b := r.vitals(t, r.master, r.barbarian); usedOf(b, "rage") != 2 {
		t.Errorf("rage used = %d, want 2: rage needs a long rest", usedOf(b, "rage"))
	}
	if m := r.vitals(t, r.master, r.multiclassed); m.GetSpellSlots()[0].GetUsed() != 2 {
		t.Errorf("spell slots used = %d, want 2: the slots need a long rest", m.GetSpellSlots()[0].GetUsed())
	}
}

// SRD 5.1, "Long Rest" and "Temporary Hit Points": a long rest restores every hit
// point, ends the temporary ones, gives back the spell slots and every resource,
// and half of the hit dice (at least one), by the player's choice of sizes.
func TestALongRestGivesBackEverythingAndHalfTheHitDice(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.HitPointsCurrent = proto.Int32(5)
		q.HitPointsTemporary = proto.Int32(4)
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "second_wind", Used: 1}}
		q.HitDiceUsedByDie = map[int32]int32{10: 3}
	})
	r.use(t, r.barbarian, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "rage", Used: 2}}
	})
	r.use(t, r.multiclassed, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 1, Used: 2}}
		q.HitDiceUsedByDie = map[int32]int32{10: 3, 6: 2}
	})
	// The multiclass player chooses: the d6s come back first.
	choice := func(q *playv1.TakeRestRequest) {
		q.HitDiceChoices = []*playv1.HitDiceChoice{{CharacterId: r.multiclassed.GetId(), Dice: []*rulesv1.HitDice{{Faces: 6, Count: 2}}}}
	}
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_LONG, choice); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	f := r.vitals(t, r.master, r.fighter)
	if f.GetHitPointsCurrent() != f.GetHitPointsMax() || f.GetHitPointsTemporary() != 0 || usedOf(f, "second_wind") != 0 {
		t.Errorf("fighter after a long rest: %d/%d hit points, %d temporary, second wind used %d", f.GetHitPointsCurrent(), f.GetHitPointsMax(), f.GetHitPointsTemporary(), usedOf(f, "second_wind"))
	}
	// 4 hit dice: half is 2; the 3 spent d10 give back 2, the default taking the largest.
	if got := hitDiceUsed(f)[10]; got != 1 {
		t.Errorf("fighter's spent d10 = %d, want 1 (3 spent, half of 4 back)", got)
	}
	if b := r.vitals(t, r.master, r.barbarian); usedOf(b, "rage") != 0 {
		t.Errorf("rage used = %d, want 0 after a long rest", usedOf(b, "rage"))
	}
	m := r.vitals(t, r.master, r.multiclassed)
	if m.GetSpellSlots()[0].GetUsed() != 0 {
		t.Errorf("spell slots used = %d, want 0", m.GetSpellSlots()[0].GetUsed())
	}
	// 5 hit dice: half is 2: the player's choice, both d6.
	if d := hitDiceUsed(m); d[6] != 0 || d[10] != 3 || m.GetHitDiceUsed() != 3 {
		t.Errorf("multiclass hit dice spent = %v (total %d), want 3d10 and no d6", d, m.GetHitDiceUsed())
	}
}

// SRD 5.1, "Long Rest": a character must have at least 1 hit point at the start of
// the rest to gain its benefits.
func TestALongRestGivesNothingToACharacterAtZeroHitPoints(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.HitPointsCurrent = proto.Int32(0)
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "second_wind", Used: 1}}
	})
	r.use(t, r.barbarian, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitPointsCurrent = proto.Int32(3) })
	preview, err := r.master.resource.GetRestPreview(t.Context(), connect.NewRequest(&playv1.GetRestPreviewRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_LONG}))
	if err != nil {
		t.Fatalf("GetRestPreview() error = %v", err)
	}
	for _, p := range preview.Msg.GetCharacters() {
		if p.GetCharacterId() == r.fighter.GetId() && !p.GetNoBenefit() {
			t.Error("the preview of a character at 0 hit points does not say the rest gives it nothing")
		}
		if p.GetCharacterId() == r.barbarian.GetId() && p.GetNoBenefit() {
			t.Error("the preview of a wounded character says the rest gives it nothing")
		}
	}
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_LONG); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	f := r.vitals(t, r.master, r.fighter)
	if f.GetHitPointsCurrent() != 0 || usedOf(f, "second_wind") != 1 {
		t.Errorf("the fighter at 0 hit points: %d hit points, second wind used %d; the long rest must give it nothing", f.GetHitPointsCurrent(), usedOf(f, "second_wind"))
	}
	if b := r.vitals(t, r.master, r.barbarian); b.GetHitPointsCurrent() != b.GetHitPointsMax() {
		t.Errorf("the wounded barbarian has %d/%d hit points after the long rest", b.GetHitPointsCurrent(), b.GetHitPointsMax())
	}
}

// SRD 5.1, Warlock, Pact Magic: the pact slots come back after a short rest.
func TestAShortRestGivesBackPactSlots(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Mestre"), h.newUser("Ana")
	campaign := h.newCampaign(master, "Mirathel", ana)
	warlock := ana.createWarlock(t, campaign, "Morgana")
	master.start(t, campaign)
	if _, err := master.adjust(t, campaign, warlock.GetId(), func(q *playv1.AdjustCharacterVitalsRequest) { q.PactSlotsUsed = proto.Int32(1) }); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	if _, err := master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: campaign, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("TakeRest(short) error = %v", err)
	}
	if got := ana.liveSession(t, campaign).GetVitals()[0].GetPactSlots().GetUsed(); got != 0 {
		t.Errorf("pact slots used = %d, want 0 after a short rest", got)
	}
}

// The preview says what comes back, per character and per resource, and writes nothing.
func TestTheRestPreviewListsWhatComesBackAndWritesNothing(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	r.use(t, r.barbarian, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "rage", Used: 1}}
		q.HitPointsCurrent = proto.Int32(2)
		q.HitPointsTemporary = proto.Int32(3)
		q.HitDiceUsedByDie = map[int32]int32{12: 2}
	})
	short, err := r.master.resource.GetRestPreview(t.Context(), connect.NewRequest(&playv1.GetRestPreviewRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_SHORT}))
	if err != nil {
		t.Fatalf("GetRestPreview(short) error = %v", err)
	}
	long, err := r.master.resource.GetRestPreview(t.Context(), connect.NewRequest(&playv1.GetRestPreviewRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_LONG}))
	if err != nil {
		t.Fatalf("GetRestPreview(long) error = %v", err)
	}
	find := func(res *playv1.GetRestPreviewResponse) *playv1.RestPreview {
		for _, p := range res.GetCharacters() {
			if p.GetCharacterId() == r.barbarian.GetId() {
				return p
			}
		}
		t.Fatal("no preview of the barbarian")
		return nil
	}
	if p := find(short.Msg); len(p.GetResources()) != 0 || p.GetTemporaryHitPointsLost() != 0 || len(p.GetHitDiceBack()) != 0 {
		t.Errorf("the short rest preview of rage = %v, want nothing: rage waits for a long rest", p)
	}
	p := find(long.Msg)
	if len(p.GetResources()) != 1 || p.GetResources()[0].GetKey() != "rage" || p.GetResources()[0].GetSpent() != 1 || p.GetResources()[0].GetTotal() != 3 {
		t.Errorf("the long rest preview of resources = %v, want rage 1 of 3", p.GetResources())
	}
	if p.GetTemporaryHitPointsLost() != 3 || p.GetHitPointsCurrent() != 2 || p.GetHitDiceBackLimit() != 1 {
		t.Errorf("preview: %d temporary lost, %d hit points, limit %d", p.GetTemporaryHitPointsLost(), p.GetHitPointsCurrent(), p.GetHitDiceBackLimit())
	}
	if len(p.GetHitDiceBack()) != 1 || p.GetHitDiceBack()[0].GetFaces() != 12 || p.GetHitDiceBack()[0].GetCount() != 1 {
		t.Errorf("hit dice back = %v, want 1d12 (half of 3 dice, at least one)", p.GetHitDiceBack())
	}
	if long.Msg.GetLongRestAlreadyTaken() {
		t.Error("the preview says a long rest was already taken, and none was")
	}
	if b := r.vitals(t, r.master, r.barbarian); b.GetHitPointsCurrent() != 2 || usedOf(b, "rage") != 1 {
		t.Errorf("the preview changed the barbarian: %d hit points, rage used %d", b.GetHitPointsCurrent(), usedOf(b, "rage"))
	}
	// After a long rest the next preview warns.
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_LONG); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	again, err := r.master.resource.GetRestPreview(t.Context(), connect.NewRequest(&playv1.GetRestPreviewRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_LONG}))
	if err != nil || !again.Msg.GetLongRestAlreadyTaken() {
		t.Errorf("the preview after a long rest: warned = %v, err = %v", again.Msg.GetLongRestAlreadyTaken(), err)
	}
}

func TestOnlyTheMasterRestsAndPreviews(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	_, err := r.ana.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player's TakeRest = %v, want permission_denied", err)
	}
	_, err = r.ana.resource.GetRestPreview(t.Context(), connect.NewRequest(&playv1.GetRestPreviewRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_SHORT}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player's GetRestPreview = %v, want permission_denied", err)
	}
	_, err = r.master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: r.campaign, IdempotencyKey: newKey()}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("TakeRest without a kind = %v, want invalid_argument", err)
	}
	_, err = r.master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: "not a uuid"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("TakeRest with a bad key = %v, want invalid_argument", err)
	}
}

func TestARestNeedsAnOpenSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Mestre"), h.newUser("Ana")
	campaign := h.newCampaign(master, "Mirathel", ana)
	ana.hero(t, campaign, "Toren", "class:fighter", "race:human", 2, abilities(16, 13, 14, 10, 10), []string{battleaxe}, nil)
	_, err := master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: campaign, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()}))
	wantBlocked(t, "TakeRest() before the session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	_, err = master.resource.GetRestPreview(t.Context(), connect.NewRequest(&playv1.GetRestPreviewRequest{CampaignId: campaign, Kind: playv1.RestKind_REST_KIND_SHORT}))
	wantBlocked(t, "GetRestPreview() before the session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
}

// A rest has no place in the middle of a fight.
func TestARestIsRefusedWhileACombatRuns(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	e := f.startFight(t, 1)
	f.submitAll(t, e)
	if _, err := f.begin(t, f.get(t, f.master)); err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	_, err := f.master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: f.campaignID, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()}))
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_COMBAT_OPEN)
	_, err = f.ana.resource.SpendHitDice(t.Context(), connect.NewRequest(&playv1.SpendHitDiceRequest{
		CampaignId: f.campaignID, CharacterId: f.pens.GetId(), Faces: 6, IdempotencyKey: newKey(), Roll: &playv1.SpendHitDiceRequest_RollInApp{RollInApp: true},
	}))
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_COMBAT_OPEN)
}

// A key repeated gives the first answer and writes nothing more; the same key for
// another rest is refused.
func TestTakeRestIsIdempotent(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	r.use(t, r.barbarian, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "rage", Used: 2}}
	})
	key := newKey()
	req := &playv1.TakeRestRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_LONG, IdempotencyKey: key}
	first, err := r.master.resource.TakeRest(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("TakeRest() error = %v", err)
	}
	// Used again after the rest, then the retry arrives: it must not rest the party twice.
	r.use(t, r.barbarian, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "rage", Used: 1}}
	})
	again, err := r.master.resource.TakeRest(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("TakeRest() retry error = %v", err)
	}
	if len(again.Msg.GetVitals()) != len(first.Msg.GetVitals()) {
		t.Errorf("the retry answered %d characters, the first call %d", len(again.Msg.GetVitals()), len(first.Msg.GetVitals()))
	}
	if b := r.vitals(t, r.master, r.barbarian); usedOf(b, "rage") != 1 {
		t.Errorf("rage used = %d, want 1: the retry must not rest the party again", usedOf(b, "rage"))
	}
	var rests int
	if err := r.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE game_session_id = $1 AND kind = $2`, r.session.GetId(), eventRestTaken).Scan(&rests); err != nil {
		t.Fatalf("count the rest events: %v", err)
	}
	if rests != 1 {
		t.Errorf("the session has %d rest events, want 1", rests)
	}
	req.Kind = playv1.RestKind_REST_KIND_SHORT
	_, err = r.master.resource.TakeRest(t.Context(), connect.NewRequest(req))
	if connect.CodeOf(err) != connect.CodeInvalidArgument || !strings.Contains(err.Error(), "idempotency_key") {
		t.Errorf("the key reused for a short rest = %v, want invalid_argument", err)
	}
}

// The rest is told to the session: the master and the character's player get the new
// vitals, and nobody else.
func TestARestPushesTheNewVitalsToTheOwnerAndTheMaster(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "second_wind", Used: 1}}
	})
	master, ana, caio := r.master.watch(t, r.campaign), r.ana.watch(t, r.campaign), r.caio.watch(t, r.campaign)
	for _, w := range []*watcher{master, ana, caio} {
		w.ready(t)
	}
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_SHORT); err != nil {
		t.Fatalf("TakeRest() error = %v", err)
	}
	r.master.markCurrentMap(t, r.campaign, r.h.newMap(r.campaign, 0), master, ana, caio)
	for who, w := range map[string]*watcher{"master": master, "Ana": ana} {
		var got *playv1.CharacterVitals
		for _, ev := range w.beforeMarker(t) {
			if v := ev.GetVitalsChanged().GetVitals(); v.GetCharacterId() == r.fighter.GetId() {
				got = v
			}
		}
		if got == nil || usedOf(got, "second_wind") != 0 {
			t.Errorf("%s got %v, want the fighter's vitals with second wind back", who, got)
		}
	}
	// Caio's own character was not touched by the short rest (nothing of his was spent),
	// and the others' are not his to hear.
	if evs := caio.beforeMarker(t); len(evs) != 0 {
		t.Errorf("Caio received %v for a rest that touched Ana's character", evs)
	}
}

// spend spends one hit die of the character as u, rolled in the app.
func (r *restTable) spend(t *testing.T, u *user, c *charactersv1.Character, faces int32, edit ...func(*playv1.SpendHitDiceRequest)) (*playv1.SpendHitDiceResponse, error) {
	t.Helper()
	req := &playv1.SpendHitDiceRequest{
		CampaignId: r.campaign, CharacterId: c.GetId(), Faces: faces, IdempotencyKey: newKey(),
		Roll: &playv1.SpendHitDiceRequest_RollInApp{RollInApp: true},
	}
	for _, e := range edit {
		e(req)
	}
	res, err := u.resource.SpendHitDice(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// SRD 5.1, "Short Rest": for each hit die spent, the player rolls the die and adds the
// Constitution modifier, and the character regains hit points equal to the total.
func TestSpendingAHitDieRollsItAddsConstitutionAndHeals(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	hpMax := r.vitals(t, r.master, r.fighter).GetHitPointsMax()
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitPointsCurrent = proto.Int32(3) })
	r.h.roller.queue(7)
	res, err := r.spend(t, r.ana, r.fighter, 10)
	if err != nil {
		t.Fatalf("SpendHitDice() error = %v", err)
	}
	// Constitution 14 is +2: 7 + 2 = 9.
	if res.GetFace() != 7 || res.GetConstitutionModifier() != 2 || res.GetHealed() != 9 {
		t.Errorf("rolled %d, modifier %d, healed %d; want 7, +2 and 9", res.GetFace(), res.GetConstitutionModifier(), res.GetHealed())
	}
	if v := res.GetVitals(); v.GetHitPointsCurrent() != 12 || hitDiceUsed(v)[10] != 1 || v.GetHitDiceUsed() != 1 {
		t.Errorf("vitals after: %d hit points, hit dice spent %v (total %d); want 12 and one d10", v.GetHitPointsCurrent(), hitDiceUsed(v), v.GetHitDiceUsed())
	}
	// The player decides after each roll whether to spend another, and the healing stops at the maximum.
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitPointsCurrent = proto.Int32(hpMax - 2) })
	r.h.roller.queue(10)
	res, err = r.spend(t, r.ana, r.fighter, 10)
	if err != nil {
		t.Fatalf("SpendHitDice() error = %v", err)
	}
	if res.GetHealed() != 2 || res.GetVitals().GetHitPointsCurrent() != hpMax || hitDiceUsed(res.GetVitals())[10] != 2 {
		t.Errorf("healed %d, %d hit points, spent %v; want 2, the maximum and two d10", res.GetHealed(), res.GetVitals().GetHitPointsCurrent(), hitDiceUsed(res.GetVitals()))
	}
	var events int
	if err := r.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE game_session_id = $1 AND kind = $2 AND character_id = $3`, r.session.GetId(), eventHitDiceSpent, r.fighter.GetId()).Scan(&events); err != nil || events != 2 {
		t.Errorf("hit_dice_spent events = %d (%v), want 2", events, err)
	}
}

// SRD 5.1, "Multiclassing": hit dice of different sizes are kept apart.
func TestHitDiceAreSpentBySize(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	// Doran is a fighter 3 and a wizard 2: 3d10 and 2d6.
	if v := r.vitals(t, r.caio, r.multiclassed); v.GetHitDiceTotal() != 5 || len(v.GetHitDice()) != 2 {
		t.Fatalf("Doran's hit dice = %v (total %d), want 3d10 and 2d6", v.GetHitDice(), v.GetHitDiceTotal())
	}
	for range 2 {
		if _, err := r.spend(t, r.caio, r.multiclassed, 6); err != nil {
			t.Fatalf("SpendHitDice(d6) error = %v", err)
		}
	}
	// No d6 left: refused with the reason, while the d10s are untouched.
	_, err := r.spend(t, r.caio, r.multiclassed, 6)
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_HIT_DICE_LEFT)
	if v := r.vitals(t, r.caio, r.multiclassed); hitDiceUsed(v)[6] != 2 || hitDiceUsed(v)[10] != 0 || v.GetHitDiceUsed() != 2 {
		t.Errorf("spent = %v (total %d), want both d6 and no d10", hitDiceUsed(v), v.GetHitDiceUsed())
	}
	// A size the character has none of.
	_, err = r.spend(t, r.caio, r.multiclassed, 8)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("SpendHitDice(d8) = %v, want invalid_argument: Doran has no d8", err)
	}
	if _, err = r.spend(t, r.caio, r.multiclassed, 10); err != nil {
		t.Errorf("SpendHitDice(d10) error = %v", err)
	}
}

// A real die is typed (RN-18): the face must be on the die, and the campaign's dice setting binds the player.
func TestAHitDieRolledWithARealDieIsTyped(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	typedFace := func(face int32) func(*playv1.SpendHitDiceRequest) {
		return func(q *playv1.SpendHitDiceRequest) { q.Roll = &playv1.SpendHitDiceRequest_TypedFace{TypedFace: face} }
	}
	res, err := r.spend(t, r.ana, r.fighter, 10, typedFace(4))
	if err != nil || res.GetFace() != 4 {
		t.Fatalf("SpendHitDice(typed 4) = %v, %v", res, err)
	}
	for _, face := range []int32{0, 11, -3} {
		if _, err := r.spend(t, r.ana, r.fighter, 10, typedFace(face)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("typed face %d = %v, want invalid_argument", face, err)
		}
	}
	if _, err := r.spend(t, r.ana, r.fighter, 10, func(q *playv1.SpendHitDiceRequest) { q.Roll = nil }); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no roll = %v, want invalid_argument", err)
	}
	if _, err := r.master.campaigns.SetCampaignDiceMode(t.Context(), connect.NewRequest(&campaignsv1.SetCampaignDiceModeRequest{CampaignId: r.campaign, Mode: campaignsv1.DiceMode_DICE_MODE_PHYSICAL})); err != nil {
		t.Fatalf("SetCampaignDiceMode() error = %v", err)
	}
	_, err = r.spend(t, r.ana, r.fighter, 10)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE)
	if _, err := r.spend(t, r.ana, r.fighter, 10, typedFace(9)); err != nil {
		t.Errorf("a typed face when the table rolls real dice = %v", err)
	}
}

// A hit die is the character's player's to spend, or the master's.
func TestOnlyTheCharactersPlayerOrTheMasterSpendsItsHitDice(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	_, err := r.spend(t, r.bia, r.fighter, 10)
	wantCode(t, "another player's SpendHitDice", err, connect.CodePermissionDenied)
	if _, err := r.spend(t, r.master, r.fighter, 10); err != nil {
		t.Errorf("the master's SpendHitDice error = %v", err)
	}
	if _, err := r.spend(t, r.ana, r.fighter, 10); err != nil {
		t.Errorf("the player's SpendHitDice error = %v", err)
	}
}

// A key repeated spends nothing more and answers the first roll again.
func TestSpendHitDiceIsIdempotent(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitPointsCurrent = proto.Int32(2) })
	r.h.roller.queue(5)
	key := newKey()
	withKey := func(q *playv1.SpendHitDiceRequest) { q.IdempotencyKey = key }
	first, err := r.spend(t, r.ana, r.fighter, 10, withKey)
	if err != nil {
		t.Fatalf("SpendHitDice() error = %v", err)
	}
	r.h.roller.queue(9) // a second roll would show
	again, err := r.spend(t, r.ana, r.fighter, 10, withKey)
	if err != nil {
		t.Fatalf("SpendHitDice() retry error = %v", err)
	}
	if again.GetFace() != first.GetFace() || again.GetHealed() != first.GetHealed() || again.GetVitals().GetHitDiceUsed() != 1 {
		t.Errorf("retry = face %d healed %d spent %d; first = face %d healed %d", again.GetFace(), again.GetHealed(), again.GetVitals().GetHitDiceUsed(), first.GetFace(), first.GetHealed())
	}
	if _, err := r.spend(t, r.ana, r.fighter, 6, withKey); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("the key reused for another die = %v, want invalid_argument", err)
	}
	if _, err := r.spend(t, r.master, r.barbarian, 12, withKey); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("the key reused for another character = %v, want invalid_argument", err)
	}
}

// A hit die never takes hit points away: a Constitution penalty larger than the roll heals nothing.
func TestAHitDieNeverHurts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Mestre"), h.newUser("Ana")
	campaign := h.newCampaign(master, "Mirathel", ana)
	weak := ana.hero(t, campaign, "Frágil", "class:wizard", "race:human", 2, abilities(8, 10, 6, 16, 10), nil, nil)
	master.start(t, campaign)
	h.roller.queue(1)
	res, err := ana.resource.SpendHitDice(t.Context(), connect.NewRequest(&playv1.SpendHitDiceRequest{
		CampaignId: campaign, CharacterId: weak.GetId(), Faces: 6, IdempotencyKey: newKey(), Roll: &playv1.SpendHitDiceRequest_RollInApp{RollInApp: true},
	}))
	if err != nil {
		t.Fatalf("SpendHitDice() error = %v", err)
	}
	// Constitution 7 (6 plus the human +1) is -2: 1 - 2 is below 0.
	if res.Msg.GetHealed() != 0 || res.Msg.GetConstitutionModifier() >= 0 {
		t.Errorf("healed %d with a modifier of %d, want 0 and a penalty", res.Msg.GetHealed(), res.Msg.GetConstitutionModifier())
	}
	if res.Msg.GetVitals().GetHitPointsCurrent() != res.Msg.GetVitals().GetHitPointsMax() {
		t.Errorf("hit points = %d of %d, want them untouched", res.Msg.GetVitals().GetHitPointsCurrent(), res.Msg.GetVitals().GetHitPointsMax())
	}
}

// A character whose hit dice were stored as a count with no sizes (before they were kept
// by size) is read with the largest dice spent first, and the next write keeps the sizes.
func TestHitDiceStoredBeforeTheSizesWereKeptAreDerivedFromTheClasses(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	if _, err := r.h.pool.Exec(t.Context(),
		`INSERT INTO character_vitals (character_id, hit_points_current, hit_dice_used, revision, updated_at) VALUES ($1, NULL, 4, 1, now())`,
		r.multiclassed.GetId()); err != nil {
		t.Fatalf("insert the old vitals row: %v", err)
	}
	v := r.vitals(t, r.caio, r.multiclassed)
	// 3d10 and 2d6, four spent: the three d10 and one d6.
	if d := hitDiceUsed(v); d[10] != 3 || d[6] != 1 || v.GetHitDiceUsed() != 4 {
		t.Fatalf("old row read as %v (total %d), want 3d10 and 1d6 spent", d, v.GetHitDiceUsed())
	}
	got := r.use(t, r.multiclassed, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitPointsTemporary = proto.Int32(2) })
	if d := hitDiceUsed(got); d[10] != 3 || d[6] != 1 {
		t.Errorf("after an unrelated correction the spent dice are %v, want them kept", d)
	}
	var stored string
	if err := r.h.pool.QueryRow(t.Context(), `SELECT hit_dice_used_by_die::TEXT FROM character_vitals WHERE character_id = $1`, r.multiclassed.GetId()).Scan(&stored); err != nil {
		t.Fatalf("read the sizes: %v", err)
	}
	if !strings.Contains(stored, `"10": 3`) || !strings.Contains(stored, `"6": 1`) {
		t.Errorf("stored sizes = %s, want the object with the sizes", stored)
	}
}

// The master's correction takes the dice by size, and refuses a size the character has not.
func TestTheMastersCorrectionOfHitDiceIsBySize(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	v := r.use(t, r.multiclassed, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitDiceUsedByDie = map[int32]int32{10: 2, 6: 1} })
	if d := hitDiceUsed(v); d[10] != 2 || d[6] != 1 || v.GetHitDiceUsed() != 3 {
		t.Errorf("spent = %v (total %d), want 2d10 and 1d6", d, v.GetHitDiceUsed())
	}
	// The sizes not listed stay.
	v = r.use(t, r.multiclassed, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitDiceUsedByDie = map[int32]int32{6: 0} })
	if d := hitDiceUsed(v); d[10] != 2 || d[6] != 0 {
		t.Errorf("spent = %v, want 2d10 only", d)
	}
	for name, edit := range map[string]func(*playv1.AdjustCharacterVitalsRequest){
		"a size the character has not": func(q *playv1.AdjustCharacterVitalsRequest) { q.HitDiceUsedByDie = map[int32]int32{8: 1} },
		"more dice than it has":        func(q *playv1.AdjustCharacterVitalsRequest) { q.HitDiceUsedByDie = map[int32]int32{6: 3} },
		"a negative count":             func(q *playv1.AdjustCharacterVitalsRequest) { q.HitDiceUsedByDie = map[int32]int32{10: -1} },
		"the count and the sizes together": func(q *playv1.AdjustCharacterVitalsRequest) {
			q.HitDiceUsed = proto.Int32(1)
			q.HitDiceUsedByDie = map[int32]int32{10: 1}
		},
	} {
		if _, err := r.master.adjust(t, r.campaign, r.multiclassed.GetId(), edit); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: err = %v, want invalid_argument", name, err)
		}
	}
	// The older count with no sizes still works, and spends the largest dice first.
	v = r.use(t, r.multiclassed, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitDiceUsed = proto.Int32(4) })
	if d := hitDiceUsed(v); d[10] != 3 || d[6] != 1 {
		t.Errorf("a count of 4 = %v, want 3d10 and 1d6", d)
	}
}

// SRD 5.1, "Aid": the spell lasts 8 hours, which a long rest covers, so the long
// rest ends it and the hit points that come back fill the sheet's own maximum; a
// short rest does not.
func TestALongRestEndsAidAndAShortRestDoesNot(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitPointsCurrent = proto.Int32(10) })
	sheetMax := r.vitals(t, r.master, r.fighter).GetHitPointsMax()
	if _, err := r.h.pool.Exec(t.Context(), `UPDATE character_vitals SET hit_points_max_bonus = 5, hit_points_current = 15 WHERE character_id = $1`, r.fighter.GetId()); err != nil {
		t.Fatalf("give the fighter Aid: %v", err)
	}
	if v := r.vitals(t, r.master, r.fighter); v.GetHitPointsMaxBonus() != 5 || v.GetHitPointsMax() != sheetMax+5 {
		t.Fatalf("with Aid: bonus %d, maximum %d (sheet %d)", v.GetHitPointsMaxBonus(), v.GetHitPointsMax(), sheetMax)
	}
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_SHORT); err != nil {
		t.Fatalf("TakeRest(short) error = %v", err)
	}
	if v := r.vitals(t, r.master, r.fighter); v.GetHitPointsMaxBonus() != 5 || v.GetHitPointsMax() != sheetMax+5 {
		t.Errorf("after a short rest: bonus %d, maximum %d, want Aid kept", v.GetHitPointsMaxBonus(), v.GetHitPointsMax())
	}
	if _, err := r.rest(t, playv1.RestKind_REST_KIND_LONG); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	v := r.vitals(t, r.master, r.fighter)
	if v.GetHitPointsMaxBonus() != 0 || v.GetHitPointsMax() != sheetMax || v.GetHitPointsCurrent() != sheetMax {
		t.Errorf("after a long rest: bonus %d, %d/%d hit points, want Aid ended and %d/%d", v.GetHitPointsMaxBonus(), v.GetHitPointsCurrent(), v.GetHitPointsMax(), sheetMax, sheetMax)
	}
}
