package characters

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// The character's creatures as the API serves them (MR-037): GiveCreature,
// RenameCreature, DismissCreature and AdjustCreatureHitPoints. These tests freeze
// each refusal (code, message, field) in the order the checks run, and what each
// call asks of the play module (the session's lock, the combat, the history, the
// streams), through a spy that stands in for it.

// creatureHostSpy is the play module's side of the creatures: it records every call
// in order and answers with what the test set.
type creatureHostSpy struct {
	mu    sync.Mutex
	calls []string
	// inCombat is CreatureInCombat's answer; encounter the combat CreaturesLeaving and
	// CreatureRenamed name; fail, when set, the error of those three.
	inCombat  bool
	encounter string
	fail      error
	// spendFail is the refusal of SpendDismissal (the owner's action in the combat).
	spendFail error
}

func (s *creatureHostSpy) record(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, fmt.Sprintf(format, args...))
}

// take returns the calls recorded since the last take.
func (s *creatureHostSpy) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.calls
	s.calls = nil
	return out
}

func (s *creatureHostSpy) CreaturesLeaving(_ context.Context, _ pgx.Tx, _, actorUserID string, creatureIDs []string, _ time.Time) (string, error) {
	s.record("leaving by %s: %s", actorUserID, strings.Join(creatureIDs, ","))
	return s.encounter, s.fail
}

func (s *creatureHostSpy) SpendDismissal(_ context.Context, _ pgx.Tx, _, _, _, creatureID string, _ time.Time) error {
	s.record("spend the action for %s", creatureID)
	return s.spendFail
}

func (s *creatureHostSpy) SteedSentAway(_ context.Context, _ pgx.Tx, _, _, _, creatureID string, released bool, _ time.Time) error {
	s.record("steed %s sent away (released %v)", creatureID, released)
	return nil
}

func (s *creatureHostSpy) LockSession(context.Context, pgx.Tx, string) error {
	s.record("lock")
	return nil
}

func (s *creatureHostSpy) CreatureRenamed(_ context.Context, _ pgx.Tx, _, creatureID, name string) (string, error) {
	s.record("renamed %s to %q", creatureID, name)
	return s.encounter, s.fail
}

func (s *creatureHostSpy) CreatureInCombat(_ context.Context, _ pgx.Tx, _, creatureID string) (bool, error) {
	s.record("in combat? %s", creatureID)
	return s.inCombat, s.fail
}

func (s *creatureHostSpy) CharacterInCombat(context.Context, pgx.Tx, string, string) (bool, error) {
	s.record("character in combat?")
	return false, nil
}

func (s *creatureHostSpy) AppendEvent(_ context.Context, _ pgx.Tx, _, kind, actorUserID string, payload []byte, _ time.Time) (bool, error) {
	s.record("event %s by %s %s", kind, actorUserID, payload)
	return true, nil
}

func (s *creatureHostSpy) PublishCreaturesChanged(_, ownerUserID string) {
	s.record("publish creatures of %s", ownerUserID)
}

func (s *creatureHostSpy) PublishEncounterChanged(_ context.Context, _, encounterID string) {
	s.record("publish encounter %s", encounterID)
}

// creatureTable is a campaign with a master, the owner of Pensantus, another
// player and the spy.
type creatureTable struct {
	h                    *harness
	spy                  *creatureHostSpy
	master, owner, other *user
	campaign             string
	pc                   *charactersv1.Character
}

func newCreatureTable(t *testing.T) *creatureTable {
	t.Helper()
	tb := &creatureTable{h: newHarness(t), spy: &creatureHostSpy{}}
	tb.h.svc.SetCreatureHost(tb.spy)
	tb.master, tb.owner, tb.other = tb.h.newUser("Mestre"), tb.h.newUser("Dona"), tb.h.newUser("Outra")
	tb.campaign = tb.h.newCampaign(tb.master, "Mirathel", tb.owner, tb.other)
	tb.pc = tb.owner.createPensantus(t, tb.campaign)
	return tb
}

func (tb *creatureTable) give(u *user, r *charactersv1.GiveCreatureRequest) (*charactersv1.CharacterCreature, error) {
	if r.CampaignId == "" {
		r.CampaignId = tb.campaign
	}
	res, err := u.api.GiveCreature(context.Background(), connect.NewRequest(r))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCreature(), nil
}

// wolf gives Pensantus a wolf named name, or fails the test; the spy forgets the gift.
func (tb *creatureTable) wolf(t *testing.T, name string) *charactersv1.CharacterCreature {
	t.Helper()
	c, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf", Name: name})
	if err != nil {
		t.Fatalf("GiveCreature(%q) error = %v", name, err)
	}
	tb.spy.take()
	return c
}

func (tb *creatureTable) rename(u *user, id, name string) (*charactersv1.CharacterCreature, error) {
	res, err := u.api.RenameCreature(context.Background(), connect.NewRequest(&charactersv1.RenameCreatureRequest{CampaignId: tb.campaign, CreatureId: id, Name: name}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCreature(), nil
}

func (tb *creatureTable) dismiss(u *user, id string) error {
	_, err := u.api.DismissCreature(context.Background(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: tb.campaign, CreatureId: id}))
	return err
}

func (tb *creatureTable) adjust(u *user, r *charactersv1.AdjustCreatureHitPointsRequest) (*charactersv1.AdjustCreatureHitPointsResponse, error) {
	r.CampaignId = tb.campaign
	res, err := u.api.AdjustCreatureHitPoints(context.Background(), connect.NewRequest(r))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// creatureRow reads a creature's stored hit points and dismissal straight from the table.
func (tb *creatureTable) creatureRow(t *testing.T, id string) string {
	t.Helper()
	var hp, hpMax int32
	var reason *string
	if err := tb.h.pool.QueryRow(t.Context(), "SELECT hp_current, hp_max, dismissed_reason FROM character_creatures WHERE id = $1", id).Scan(&hp, &hpMax, &reason); err != nil {
		t.Fatalf("read the creature: %v", err)
	}
	return fmt.Sprintf("hp %d/%d dismissed %q", hp, hpMax, conv.OrZero(reason))
}

// describeCharacterCreature is what a CharacterCreature says, without its IDs and time.
func describeCharacterCreature(c *charactersv1.CharacterCreature) string {
	if c == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s (%s, %s) source %v attack %d concentration %v hp %d/%d",
		c.GetName(), c.GetMonsterKey(), c.GetMonsterNamePt(), c.GetSource(), c.GetAttack(), c.GetDependsOnConcentration(), c.GetHitPointsCurrent(), c.GetHitPointsMax())
}

// wantCreatureRefusal fails the test unless err is exactly the refusal: its
// text (code and message) and the InvalidField detail's field ("" for none).
func wantCreatureRefusal(t *testing.T, call string, err error, want, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s error = nil, want %q", call, want)
	}
	if err.Error() != want {
		t.Errorf("%s error = %q, want %q", call, err.Error(), want)
	}
	got := ""
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		for _, d := range ce.Details() {
			if v, err := d.Value(); err == nil {
				if f, ok := v.(*charactersv1.InvalidField); ok {
					got = f.GetField()
				}
			}
		}
	}
	if got != field {
		t.Errorf("%s field = %q, want %q", call, got, field)
	}
}

// wantCreatureCalls fails the test unless the spy recorded exactly want since the last take.
func wantCreatureCalls(t *testing.T, call string, spy *creatureHostSpy, want ...string) {
	t.Helper()
	if got := spy.take(); !slices.Equal(got, want) {
		t.Errorf("%s calls to play =\n  %s\nwant\n  %s", call, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestMR037_GiveCreatureRefusalsInTheirOrder(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	npc := tb.master.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin", basicSheet())
	deadOwner := tb.h.newUser("Morta")
	tb.h.join(tb.master, tb.campaign, deadOwner)
	dead := tb.master.markDead(t, deadOwner.createPensantus(t, tb.campaign))
	long := strings.Repeat("a", maxCreatureName+1)

	for _, tt := range []struct {
		name  string
		as    *user
		req   *charactersv1.GiveCreatureRequest
		want  string
		field string
		calls []string
	}{
		{
			"a player", tb.owner, &charactersv1.GiveCreatureRequest{CharacterId: "x", MonsterKey: "nope"},
			"permission_denied: only the campaign's master can do this", "", nil,
		},
		{
			"a character ID that is no UUID comes before the creature", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: "x", MonsterKey: "nope"},
			"not_found: character not found", "", nil,
		},
		{
			"the creature comes before the name", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "spell:fireball", Name: long},
			"invalid_argument: monster_key is not an SRD creature", "monster_key", nil,
		},
		{
			"no creature", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId()},
			"invalid_argument: monster_key is not an SRD creature", "monster_key", nil,
		},
		{
			"the name comes before the key", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf", Name: long, IdempotencyKey: "abc"},
			"invalid_argument: name must be at most 40 characters, got 41", "name", nil,
		},
		{
			"a name of two lines", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf", Name: "Pre\nsa"},
			"invalid_argument: name must not contain the character U+000A", "name", nil,
		},
		{
			"a key too long", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf", IdempotencyKey: strings.Repeat("k", 65)},
			"invalid_argument: idempotency_key must be 1 to 64 characters", "", nil,
		},
		{
			"a key that is no UUID", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf", IdempotencyKey: "abc"},
			"invalid_argument: idempotency_key must be a UUID", "idempotency_key", nil,
		},
		{
			"a character of no campaign", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: uuid.New().String(), MonsterKey: "monster:wolf"},
			"not_found: character not found", "",
			[]string{"lock"},
		},
		{
			"an NPC", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: npc.GetId(), MonsterKey: "monster:wolf"},
			"invalid_argument: character_id is not a living player's character", "character_id",
			[]string{"lock"},
		},
		{
			"a dead character", tb.master, &charactersv1.GiveCreatureRequest{CharacterId: dead.GetId(), MonsterKey: "monster:wolf"},
			"invalid_argument: character_id is not a living player's character", "character_id",
			[]string{"lock"},
		},
	} {
		_, err := tb.give(tt.as, tt.req)
		wantCreatureRefusal(t, tt.name, err, tt.want, tt.field)
		wantCreatureCalls(t, tt.name, tb.spy, tt.calls...)
	}
}

func TestMR037_GiveCreatureWritesTheGiftAndTellsPlay(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)

	presa, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf", Name: "  Presa "})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := describeCharacterCreature(presa), "Presa (monster:wolf, Lobo) source CREATURE_SOURCE_MASTER attack 3 concentration false hp 11/11"; got != want {
		t.Errorf("gift = %s, want %s", got, want)
	}
	if presa.GetCharacterId() != tb.pc.GetId() || presa.GetSummonGroupId() == "" || presa.GetCreatedAt() == nil {
		t.Errorf("gift = %v", presa)
	}
	wantCreatureCalls(t, "a gift", tb.spy, "lock",
		fmt.Sprintf(`event creature_summoned by %s {"character_id":%q,"creature_ids":[%q],"monster_keys":["monster:wolf"],"source":"master"}`, tb.master.id, tb.pc.GetId(), presa.GetId()),
		"publish creatures of "+tb.owner.id)

	// No name: the creature's Portuguese name.
	unnamed, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:raven"})
	if err != nil || unnamed.GetName() != "Corvo" {
		t.Errorf("an unnamed gift = %v, %v; want Corvo", unnamed, err)
	}
	tb.spy.take()

	// A key: the retry answers the first creature, writes nothing and tells nobody.
	key := uuid.New().String()
	first, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf", IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	wantCreatureCalls(t, "a gift with a key", tb.spy, "lock",
		fmt.Sprintf(`event creature_summoned by %s {"character_id":%q,"creature_ids":[%q],"monster_keys":["monster:wolf"],"source":"master"}`, tb.master.id, tb.pc.GetId(), first.GetId()),
		"publish creatures of "+tb.owner.id)
	again, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf", IdempotencyKey: key})
	if err != nil || again.GetId() != first.GetId() || describeCharacterCreature(again) != describeCharacterCreature(first) {
		t.Errorf("the retry = %v, %v; want %v", again, err, first)
	}
	wantCreatureCalls(t, "the retry", tb.spy, "lock")

	// Without the play module the gift is still kept, and nothing is told.
	tb.h.svc.SetCreatureHost(nil)
	if _, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf"}); err != nil {
		t.Errorf("a gift without play: %v", err)
	}
	tb.h.svc.SetCreatureHost(tb.spy)
	wantCreatureCalls(t, "a gift without play", tb.spy)
}

// Finding: the CREATURE_LIMIT refusal (and CREATURE_IN_COMBAT below) has an
// empty message: errBlocked's table of messages (errors.go) lacks both reasons,
// so the text is the bare code. Frozen as it is.
func TestMR037_GiveCreatureAtTheLimitIsBlocked(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	specs := make([]link.CreatureSpec, maxCreaturesPerCharacter)
	for i := range specs {
		specs[i] = link.CreatureSpec{MonsterKey: "monster:rat", Attack: "full"}
	}
	if err := db.InTx(t.Context(), tb.h.pool, func(tx pgx.Tx) error {
		_, err := tb.h.svc.SummonCreatures(t.Context(), tx, link.Summon{
			CampaignID: tb.campaign, CharacterID: tb.pc.GetId(), Source: "conjure_animals", GroupID: uuid.New().String(), Creatures: specs,
		})
		return err
	}); err != nil {
		t.Fatalf("40 rats: %v", err)
	}
	_, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "monster:wolf"})
	if b := blocked(t, "the 41st creature", err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CREATURE_LIMIT || b.GetCharacterId() != tb.pc.GetId() {
		t.Errorf("blocked = %v", b)
	}
	wantCreatureRefusal(t, "the 41st creature", err, "failed_precondition", "")
	wantCreatureCalls(t, "the 41st creature", tb.spy, "lock")
}

func TestMR037_RenameCreature(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	presa := tb.wolf(t, "Presa")
	gone := tb.wolf(t, "Sumida")
	if err := tb.dismiss(tb.owner, gone.GetId()); err != nil {
		t.Fatal(err)
	}
	tb.spy.take()

	for _, tt := range []struct {
		name   string
		as     *user
		id, to string
		want   string
		field  string
		calls  []string
	}{
		{"someone of no campaign", tb.h.newUser("De fora"), presa.GetId(), "", "not_found: campaign not found", "", nil},
		{"the name comes first", tb.owner, "x", "  ", "invalid_argument: name must not be empty", "name", nil},
		{"an ID that is no UUID", tb.owner, "x", "Nome", "not_found: creature not found", "", []string{"lock"}},
		{"a creature of no campaign", tb.owner, uuid.New().String(), "Nome", "not_found: creature not found", "", []string{"lock"}},
		{"another player's", tb.other, presa.GetId(), "Nome", "not_found: creature not found", "", []string{"lock"}},
		{"a dismissed one", tb.owner, gone.GetId(), "Nome", "not_found: creature not found", "", []string{"lock"}},
	} {
		_, err := tb.rename(tt.as, tt.id, tt.to)
		wantCreatureRefusal(t, tt.name, err, tt.want, tt.field)
		wantCreatureCalls(t, tt.name, tb.spy, tt.calls...)
	}

	got, err := tb.rename(tb.owner, presa.GetId(), " Dentes ")
	if err != nil || describeCharacterCreature(got) != "Dentes (monster:wolf, Lobo) source CREATURE_SOURCE_MASTER attack 3 concentration false hp 11/11" || got.GetId() != presa.GetId() {
		t.Errorf("the owner renames = %v, %v", got, err)
	}
	wantCreatureCalls(t, "the owner renames", tb.spy, "lock", fmt.Sprintf("renamed %s to %q", presa.GetId(), "Dentes"), "publish creatures of "+tb.owner.id)

	// In a combat: the combat is told after the creatures.
	tb.spy.encounter = "combate-1"
	if got, err := tb.rename(tb.master, presa.GetId(), "Garra"); err != nil || got.GetName() != "Garra" {
		t.Errorf("the master renames = %v, %v", got, err)
	}
	wantCreatureCalls(t, "the master renames in a combat", tb.spy, "lock", fmt.Sprintf("renamed %s to %q", presa.GetId(), "Garra"),
		"publish creatures of "+tb.owner.id, "publish encounter combate-1")
	tb.spy.encounter = ""

	// play fails: nothing is written.
	tb.spy.fail = errors.New("play is down")
	_, err = tb.rename(tb.owner, presa.GetId(), "Outro")
	wantCreatureRefusal(t, "play fails", err, "internal: internal error", "")
	wantCreatureCalls(t, "play fails", tb.spy, "lock", fmt.Sprintf("renamed %s to %q", presa.GetId(), "Outro"))
	tb.spy.fail = nil

	// Without play: renamed, nobody told.
	tb.h.svc.SetCreatureHost(nil)
	if got, err := tb.rename(tb.owner, presa.GetId(), "Sem jogo"); err != nil || got.GetName() != "Sem jogo" {
		t.Errorf("a rename without play = %v, %v", got, err)
	}
	tb.h.svc.SetCreatureHost(tb.spy)
	wantCreatureCalls(t, "a rename without play", tb.spy)
}

func TestMR037_DismissCreature(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	presa, garra, dente := tb.wolf(t, "Presa"), tb.wolf(t, "Garra"), tb.wolf(t, "Dente")

	for _, tt := range []struct {
		name  string
		as    *user
		id    string
		want  string
		calls []string
	}{
		{"someone of no campaign", tb.h.newUser("De fora"), presa.GetId(), "not_found: campaign not found", nil},
		{"an ID that is no UUID", tb.owner, "x", "not_found: creature not found", []string{"lock"}},
		{"a creature of no campaign", tb.owner, uuid.New().String(), "not_found: creature not found", []string{"lock"}},
		{"another player's", tb.other, presa.GetId(), "not_found: creature not found", []string{"lock"}},
	} {
		wantCreatureRefusal(t, tt.name, tb.dismiss(tt.as, tt.id), tt.want, "")
		wantCreatureCalls(t, tt.name, tb.spy, tt.calls...)
	}

	if err := tb.dismiss(tb.owner, presa.GetId()); err != nil {
		t.Fatal(err)
	}
	wantCreatureCalls(t, "the owner dismisses", tb.spy, "lock", fmt.Sprintf("leaving by %s: %s", tb.owner.id, presa.GetId()),
		fmt.Sprintf(`event creature_dismissed by %s {"character_id":%q,"creature_ids":[%q],"reason":"owner"}`, tb.owner.id, tb.pc.GetId(), presa.GetId()),
		"publish creatures of "+tb.owner.id)
	if got := tb.creatureRow(t, presa.GetId()); got != `hp 11/11 dismissed "owner"` {
		t.Errorf("after the owner dismisses: %s", got)
	}

	// Gone already: no error and nothing told, for the owner and the master; another
	// player still gets not_found.
	for _, u := range []*user{tb.owner, tb.master} {
		if err := tb.dismiss(u, presa.GetId()); err != nil {
			t.Errorf("dismiss again: %v", err)
		}
		wantCreatureCalls(t, "dismiss again", tb.spy, "lock")
	}
	wantCreatureRefusal(t, "another player, a dismissed one", tb.dismiss(tb.other, presa.GetId()), "not_found: creature not found", "")
	wantCreatureCalls(t, "another player, a dismissed one", tb.spy, "lock")

	// The master, in a combat.
	tb.spy.encounter = "combate-1"
	if err := tb.dismiss(tb.master, garra.GetId()); err != nil {
		t.Fatal(err)
	}
	wantCreatureCalls(t, "the master dismisses in a combat", tb.spy, "lock", fmt.Sprintf("leaving by %s: %s", tb.master.id, garra.GetId()),
		fmt.Sprintf(`event creature_dismissed by %s {"character_id":%q,"creature_ids":[%q],"reason":"master"}`, tb.master.id, tb.pc.GetId(), garra.GetId()),
		"publish creatures of "+tb.owner.id, "publish encounter combate-1")
	tb.spy.encounter = ""
	if got := tb.creatureRow(t, garra.GetId()); got != `hp 11/11 dismissed "master"` {
		t.Errorf("after the master dismisses: %s", got)
	}

	// play fails: the creature stays.
	tb.spy.fail = errors.New("play is down")
	wantCreatureRefusal(t, "play fails", tb.dismiss(tb.owner, dente.GetId()), "internal: internal error", "")
	wantCreatureCalls(t, "play fails", tb.spy, "lock", fmt.Sprintf("leaving by %s: %s", tb.owner.id, dente.GetId()))
	tb.spy.fail = nil
	if got := tb.creatureRow(t, dente.GetId()); got != `hp 11/11 dismissed ""` {
		t.Errorf("after play fails: %s", got)
	}

	// Without play: dismissed, nobody told.
	tb.h.svc.SetCreatureHost(nil)
	if err := tb.dismiss(tb.owner, dente.GetId()); err != nil {
		t.Errorf("a dismissal without play: %v", err)
	}
	tb.h.svc.SetCreatureHost(tb.spy)
	wantCreatureCalls(t, "a dismissal without play", tb.spy)
	if got := tb.creatureRow(t, dente.GetId()); got != `hp 11/11 dismissed "owner"` {
		t.Errorf("after a dismissal without play: %s", got)
	}
}

func damage(n int32) *charactersv1.AdjustCreatureHitPointsRequest_Damage {
	return &charactersv1.AdjustCreatureHitPointsRequest_Damage{Damage: n}
}

func heal(n int32) *charactersv1.AdjustCreatureHitPointsRequest_Heal {
	return &charactersv1.AdjustCreatureHitPointsRequest_Heal{Heal: n}
}

func setHitPoints(n int32) *charactersv1.AdjustCreatureHitPointsRequest_HitPoints {
	return &charactersv1.AdjustCreatureHitPointsRequest_HitPoints{HitPoints: n}
}

// Finding: a negative hit_points is refused naming the field "set" (the mode's
// name), not "hit_points", the request's field; the message says "set must be 0
// to 9999". Frozen as it is.
func TestMR037_AdjustCreatureHitPointsRefusalsInTheirOrder(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	presa := tb.wolf(t, "Presa")

	for _, tt := range []struct {
		name   string
		as     *user
		id     string
		change func(*charactersv1.AdjustCreatureHitPointsRequest)
		want   string
		field  string
		calls  []string
	}{
		{
			"a player", tb.owner, presa.GetId(), func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = damage(1) },
			"permission_denied: only the campaign's master can do this", "", nil,
		},
		{
			"no change comes before the creature", tb.master, "x", func(*charactersv1.AdjustCreatureHitPointsRequest) {},
			"invalid_argument: change set damage, heal or hit_points", "change", nil,
		},
		{
			"a negative damage", tb.master, "x", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = damage(-1) },
			"invalid_argument: damage must be 0 to 9999", "damage", nil,
		},
		{
			"a damage too big", tb.master, "x", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = damage(10000) },
			"invalid_argument: damage must be 0 to 9999", "damage", nil,
		},
		{
			"a heal too big", tb.master, "x", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = heal(10000) },
			"invalid_argument: heal must be 0 to 9999", "heal", nil,
		},
		{
			"negative hit points", tb.master, "x", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = setHitPoints(-1) },
			"invalid_argument: set must be 0 to 9999", "set", nil,
		},
		{
			"hit points above 9999 pass to the creature", tb.master, "x", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = setHitPoints(10000) },
			"not_found: creature not found", "",
			[]string{"lock"},
		},
		{
			"a creature of no campaign", tb.master, uuid.New().String(), func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = damage(1) },
			"not_found: creature not found", "",
			[]string{"lock"},
		},
		{
			"hit points above the creature's maximum", tb.master, presa.GetId(), func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = setHitPoints(12) },
			"invalid_argument: hit_points must be 0 to 11", "hit_points",
			[]string{"lock", "in combat? " + presa.GetId()},
		},
	} {
		r := &charactersv1.AdjustCreatureHitPointsRequest{CreatureId: tt.id}
		tt.change(r)
		_, err := tb.adjust(tt.as, r)
		wantCreatureRefusal(t, tt.name, err, tt.want, tt.field)
		wantCreatureCalls(t, tt.name, tb.spy, tt.calls...)
	}

	// In a combat the combat keeps the hit points.
	tb.spy.inCombat = true
	_, err := tb.adjust(tb.master, &charactersv1.AdjustCreatureHitPointsRequest{CreatureId: presa.GetId(), Change: damage(1)})
	if b := blocked(t, "in a combat", err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CREATURE_IN_COMBAT || b.GetCharacterId() != tb.pc.GetId() {
		t.Errorf("blocked = %v", b)
	}
	wantCreatureRefusal(t, "in a combat", err, "failed_precondition", "")
	wantCreatureCalls(t, "in a combat", tb.spy, "lock", "in combat? "+presa.GetId())
	tb.spy.inCombat = false

	tb.spy.fail = errors.New("play is down")
	_, err = tb.adjust(tb.master, &charactersv1.AdjustCreatureHitPointsRequest{CreatureId: presa.GetId(), Change: damage(1)})
	wantCreatureRefusal(t, "play fails", err, "internal: internal error", "")
	wantCreatureCalls(t, "play fails", tb.spy, "lock", "in combat? "+presa.GetId())
	tb.spy.fail = nil
	if got := tb.creatureRow(t, presa.GetId()); got != `hp 11/11 dismissed ""` {
		t.Errorf("after the refusals: %s", got)
	}
}

func TestMR037_AdjustCreatureHitPoints(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	presa, garra := tb.wolf(t, "Presa"), tb.wolf(t, "Garra")

	for _, tt := range []struct {
		name   string
		change func(*charactersv1.AdjustCreatureHitPointsRequest)
		want   string
	}{
		{"a damage", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = damage(3) }, "hp 8/11"},
		{"no damage", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = damage(0) }, "hp 8/11"},
		{"a heal stops at the maximum", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = heal(9999) }, "hp 11/11"},
		{"set", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = setHitPoints(5) }, "hp 5/11"},
		{"set to the maximum", func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = setHitPoints(11) }, "hp 11/11"},
	} {
		r := &charactersv1.AdjustCreatureHitPointsRequest{CreatureId: presa.GetId()}
		tt.change(r)
		res, err := tb.adjust(tb.master, r)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got, want := describeCharacterCreature(res.GetCreature()), "Presa (monster:wolf, Lobo) source CREATURE_SOURCE_MASTER attack 3 concentration false "+tt.want; got != want {
			t.Errorf("%s = %s, want %s", tt.name, got, want)
		}
		if got := tb.creatureRow(t, presa.GetId()); got != tt.want+` dismissed ""` {
			t.Errorf("%s stored %s", tt.name, got)
		}
		wantCreatureCalls(t, tt.name, tb.spy, "lock", "in combat? "+presa.GetId(), "publish creatures of "+tb.owner.id)
	}

	// At 0 the creature is defeated: dismissed, and the answer has no creature.
	for _, tt := range []struct {
		name   string
		id     string
		change func(*charactersv1.AdjustCreatureHitPointsRequest)
	}{
		{"a damage past 0", presa.GetId(), func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = damage(9999) }},
		{"set to 0", garra.GetId(), func(r *charactersv1.AdjustCreatureHitPointsRequest) { r.Change = setHitPoints(0) }},
	} {
		r := &charactersv1.AdjustCreatureHitPointsRequest{CreatureId: tt.id}
		tt.change(r)
		res, err := tb.adjust(tb.master, r)
		if err != nil || res.GetCreature() != nil {
			t.Errorf("%s = %v, %v; want no creature", tt.name, res, err)
		}
		if got := tb.creatureRow(t, tt.id); got != `hp 0/11 dismissed "defeated"` {
			t.Errorf("%s stored %s", tt.name, got)
		}
		wantCreatureCalls(t, tt.name, tb.spy, "lock", "in combat? "+tt.id,
			fmt.Sprintf(`event creature_dismissed by %s {"character_id":%q,"creature_ids":[%q],"reason":"defeated"}`, tb.master.id, tb.pc.GetId(), tt.id),
			"publish creatures of "+tb.owner.id)
	}
	_, err := tb.adjust(tb.master, &charactersv1.AdjustCreatureHitPointsRequest{CreatureId: presa.GetId(), Change: heal(1)})
	wantCreatureRefusal(t, "a defeated creature", err, "not_found: creature not found", "")
	wantCreatureCalls(t, "a defeated creature", tb.spy, "lock")

	// Without play: no combat to ask, nobody told.
	dente := tb.wolf(t, "Dente")
	tb.h.svc.SetCreatureHost(nil)
	res, err := tb.adjust(tb.master, &charactersv1.AdjustCreatureHitPointsRequest{CreatureId: dente.GetId(), Change: damage(1)})
	tb.h.svc.SetCreatureHost(tb.spy)
	if err != nil || res.GetCreature().GetHitPointsCurrent() != 10 {
		t.Errorf("a damage without play = %v, %v", res, err)
	}
	wantCreatureCalls(t, "a damage without play", tb.spy)
}
