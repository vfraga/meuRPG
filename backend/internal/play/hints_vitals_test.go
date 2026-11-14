package play

import (
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// The writers of vitals, resources, rests and Revivify (#735, part 2, 4b) go through Service.tx: the session's number moves once
// with the write, the events carry it, a repeat of the key moves nothing and tells again what the first call told (to the
// audiences it reached at most), and a refused call tells nothing and moves nothing.

// hintRig is a table with a stream open for each person, and the three phases every keyed writer goes through.
type hintRig struct {
	t         *testing.T
	h         *harness
	session   string
	listeners map[string]heardBy
	// quiet are the people whose streams must hear nothing of the write: it is none of their business.
	quiet []string
	// unnumbered are the kinds of event that another module sends itself, after the commit (the maps module's map_changed and
	// trap_noticed): they carry no number of the session, which is the maps module's to give (4d maps).
	unnumbered []string
	// dropping loses every event the service sends, as a send lost to a crash after the commit is; dropped keeps their kinds.
	dropping atomic.Bool
	mu       sync.Mutex
	dropped  map[string]bool
}

func newHintRig(t *testing.T, h *harness, campaignID string, master *user, players map[string]*user, quiet ...string) *hintRig {
	t.Helper()
	open, err := h.svc.queries.GetOpenGameSession(t.Context(), campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	g := &hintRig{t: t, h: h, session: open.ID, listeners: map[string]heardBy{"master": listen(t, h, campaignID, master.id, true)}, quiet: quiet}
	for name, u := range players {
		g.listeners[name] = listen(t, h, campaignID, u.id, false)
	}
	g.dropped = map[string]bool{}
	h.svc.setDropSend(func(_ string, ev live.Event) bool {
		if !g.dropping.Load() {
			return false
		}
		g.mu.Lock()
		g.dropped[fmt.Sprintf("%T", ev.Message.GetEvent())] = true
		g.mu.Unlock()
		return true
	})
	h.hints.Settle(t)
	g.drain() // what the fixture's own writes sent
	return g
}

// numbered says the event carries the number want, or none when it is of a kind another module sends by itself.
func (g *hintRig) numbered(ev *playv1.WatchGameSessionResponse, want int64) bool {
	return ev.GetSessionSeq() == want || (ev.GetSessionSeq() == 0 && slices.Contains(g.unnumbered, fmt.Sprintf("%T", ev.GetEvent())))
}

func (g *hintRig) seq() int64 { return g.h.hintSeq(g.t, g.session) }

func (g *hintRig) drain() map[string][]*playv1.WatchGameSessionResponse {
	g.t.Helper()
	out := map[string][]*playv1.WatchGameSessionResponse{}
	for name, l := range g.listeners {
		out[name] = l.heard(g.t, g.h)
	}
	return out
}

func kindsOf(heard map[string][]*playv1.WatchGameSessionResponse) map[string][]string {
	out := map[string][]string{}
	for name, evs := range heard {
		out[name] = eventKinds(evs)
	}
	return out
}

// quietHeard fails for a person of the quiet list who heard something.
func (g *hintRig) quietHeard(phase string, heard map[string][]*playv1.WatchGameSessionResponse) {
	g.t.Helper()
	for _, name := range g.quiet {
		if got := heard[name]; len(got) != 0 {
			g.t.Errorf("%s: %s heard %v, want nothing", phase, name, eventKinds(got))
		}
	}
}

// first is the write that happens: the number moves by one, and every event it tells carries it.
func (g *hintRig) first(call func() error, wantEvents bool) map[string][]string {
	g.t.Helper()
	return g.firstOf(call, wantEvents, 1)
}

// firstOf is first for a call that is several writes, bumps of them in all: the number moves by bumps, and every event carries one of the
// numbers the writes made.
func (g *hintRig) firstOf(call func() error, wantEvents bool, bumps int64) map[string][]string {
	g.t.Helper()
	before := g.seq()
	if err := call(); err != nil {
		g.t.Fatalf("the write: error = %v", err)
	}
	if got, want := g.seq(), before+bumps; got != want {
		g.t.Errorf("hint_seq = %d after the write, want %d (%d more)", got, want, bumps)
	}
	heard := g.drain()
	told := 0
	for name, evs := range heard {
		for _, ev := range evs {
			told++
			ok := false
			for n := before + 1; n <= before+bumps && !ok; n++ {
				ok = g.numbered(ev, n)
			}
			if !ok {
				g.t.Errorf("%s heard %T with number %d, want one of the %d the write stored after %d", name, ev.GetEvent(), ev.GetSessionSeq(), bumps, before)
			}
		}
	}
	if wantEvents && told == 0 {
		g.t.Errorf("nobody heard the write")
	}
	g.quietHeard("the write", heard)
	return kindsOf(heard)
}

// replay is the repeat of the key: nothing moves, and each person hears again what the first call told them (more is allowed,
// to those who heard something at all).
func (g *hintRig) replay(call func() error, first map[string][]string) map[string][]string {
	g.t.Helper()
	stored := g.seq()
	if err := call(); err != nil {
		g.t.Fatalf("the repeat: error = %v", err)
	}
	if got := g.seq(); got != stored {
		g.t.Errorf("hint_seq = %d after the repeat, want %d: a repeat writes nothing", got, stored)
	}
	heard := g.drain()
	for name, evs := range heard {
		for _, ev := range evs {
			if !g.numbered(ev, stored) {
				g.t.Errorf("%s heard %T on the repeat with number %d, want the stored %d", name, ev.GetEvent(), ev.GetSessionSeq(), stored)
			}
		}
		if len(first[name]) == 0 && len(evs) != 0 {
			g.t.Errorf("%s heard %v on the repeat but nothing of the first call: the repeat reaches more people", name, eventKinds(evs))
		}
		again := eventKinds(evs)
		for _, k := range first[name] {
			if !slices.Contains(again, k) {
				g.t.Errorf("%s heard %v on the repeat, missing %s that the first call told", name, again, k)
			}
		}
	}
	g.quietHeard("the repeat", heard)
	return kindsOf(heard)
}

// lost is a write whose events were all lost on the way to the streams (the first call committed and died before it sent): the
// number moved and nobody heard anything; the repeat of the key then tells everything the first call tried to tell, stamped with
// the stored number, and moves nothing.
func (g *hintRig) lost(call func() error) {
	g.t.Helper()
	before := g.seq()
	g.dropping.Store(true)
	err := call()
	g.dropping.Store(false)
	if err != nil {
		g.t.Fatalf("the write: error = %v", err)
	}
	stored := g.seq()
	if stored != before+1 {
		g.t.Errorf("hint_seq = %d after the write, want %d (one more)", stored, before+1)
	}
	g.mu.Lock()
	want := slices.Sorted(maps.Keys(g.dropped))
	g.mu.Unlock()
	if len(want) == 0 {
		g.t.Fatalf("the write tried to tell nothing: the test proves nothing")
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			g.t.Errorf("%s heard %v although every send was lost", name, eventKinds(evs))
		}
	}
	heard := g.replayAll(call)
	if got := g.seq(); got != stored {
		g.t.Errorf("hint_seq = %d after the repeat, want %d: a repeat writes nothing", got, stored)
	}
	var union []string
	for name, evs := range heard {
		for _, ev := range evs {
			union = append(union, fmt.Sprintf("%T", ev.GetEvent()))
			if !g.numbered(ev, stored) {
				g.t.Errorf("%s heard %T on the repeat with number %d, want the stored %d", name, ev.GetEvent(), ev.GetSessionSeq(), stored)
			}
		}
	}
	for _, k := range want {
		if !slices.Contains(union, k) {
			g.t.Errorf("the repeat did not tell %s, which the first call tried to tell (told: %v)", k, union)
		}
	}
}

// replayAll is the repeat of the key, and what each stream heard.
func (g *hintRig) replayAll(call func() error) map[string][]*playv1.WatchGameSessionResponse {
	g.t.Helper()
	if err := call(); err != nil {
		g.t.Fatalf("the repeat: error = %v", err)
	}
	return g.drain()
}

// refused is a call the writer turns down: it tells nothing and moves nothing.
func (g *hintRig) refused(call func() error) {
	g.t.Helper()
	before := g.seq()
	if err := call(); err == nil {
		g.t.Fatalf("the call was not refused")
	}
	if got := g.seq(); got != before {
		g.t.Errorf("hint_seq = %d after a refused call, want %d", got, before)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			g.t.Errorf("%s heard %v of a refused call", name, eventKinds(evs))
		}
	}
}

// keyed runs the three phases of a writer that takes an idempotency key: the write, its repeat and the key used for another request.
// The phases are plain steps, not subtests: the helpers fail the test they were built with (g.t), which a subtest's goroutine must
// not do, and with -failfast (the repeat of the changed tests) a subtest that starts after another test failed is skipped, so
// the writes never happen and the test fails later on a row that was never made.
func (g *hintRig) keyed(call, again, other func() error) {
	g.t.Helper()
	first := g.first(call, true) // the write moves the number once and the events carry it
	g.replay(again, first)       // the repeat of the key moves nothing and tells again
	g.refused(other)             // the key used for another request tells nothing
}

// setupAdjust is the table, and the writer's call (repeatable: one key) and its key used for another request.
func setupAdjust(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	r := newRestTable(t)
	req := &playv1.AdjustCharacterVitalsRequest{
		CampaignId: r.campaign, CharacterId: r.fighter.GetId(), IdempotencyKey: newKey(),
		ExpectedRevision: r.master.revisionIfOpen(r.campaign, r.fighter.GetId()), HitPointsCurrent: proto.Int32(3),
	}
	send := func(msg *playv1.AdjustCharacterVitalsRequest) func() error {
		return func() error {
			_, err := r.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(proto.Clone(msg).(*playv1.AdjustCharacterVitalsRequest)))
			return err
		}
	}
	changed := proto.Clone(req).(*playv1.AdjustCharacterVitalsRequest)
	changed.HitPointsCurrent = proto.Int32(4)
	g = newHintRig(t, r.h, r.campaign, r.master, map[string]*user{"Ana": r.ana, "Bia": r.bia, "Caio": r.caio}, "Bia", "Caio")
	return g, send(req), send(changed)
}

// A correction that does not say which vitals it read is refused (aborted): it tells nothing.
func TestAdjustCharacterVitalsStaleTellsNothing(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	g := newHintRig(t, r.h, r.campaign, r.master, map[string]*user{"Ana": r.ana})
	g.refused(func() error {
		_, err := r.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
			CampaignId: r.campaign, CharacterId: r.fighter.GetId(), IdempotencyKey: newKey(), HitPointsCurrent: proto.Int32(3),
		}))
		return err
	})
}

// setupTakeRest is the table, and the writer's call (repeatable: one key) and its key used for another request.
func setupTakeRest(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	r := newRestTable(t)
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "second_wind", Used: 1}}
	})
	req := &playv1.TakeRestRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()}
	send := func(msg *playv1.TakeRestRequest) func() error {
		return func() error {
			_, err := r.master.resource.TakeRest(t.Context(), connect.NewRequest(proto.Clone(msg).(*playv1.TakeRestRequest)))
			return err
		}
	}
	changed := proto.Clone(req).(*playv1.TakeRestRequest)
	changed.Kind = playv1.RestKind_REST_KIND_LONG
	g = newHintRig(t, r.h, r.campaign, r.master, map[string]*user{"Ana": r.ana, "Caio": r.caio}, "Caio")
	return g, send(req), send(changed)
}

// setupSpendHitDice is the table, and the writer's call (repeatable: one key) and its key used for another request.
func setupSpendHitDice(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	r := newRestTable(t)
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) { q.HitPointsCurrent = proto.Int32(2) })
	req := &playv1.SpendHitDiceRequest{
		CampaignId: r.campaign, CharacterId: r.fighter.GetId(), Faces: 10, IdempotencyKey: newKey(),
		Roll: &playv1.SpendHitDiceRequest_RollInApp{RollInApp: true},
	}
	send := func(msg *playv1.SpendHitDiceRequest) func() error {
		return func() error {
			r.h.roller.queue(5)
			_, err := r.ana.resource.SpendHitDice(t.Context(), connect.NewRequest(proto.Clone(msg).(*playv1.SpendHitDiceRequest)))
			return err
		}
	}
	changed := proto.Clone(req).(*playv1.SpendHitDiceRequest)
	changed.Faces = 6
	g = newHintRig(t, r.h, r.campaign, r.master, map[string]*user{"Ana": r.ana, "Bia": r.bia}, "Bia")
	return g, send(req), send(changed)
}

// setupArcane is the table, and the writer's call (repeatable: one key) and its key used for another request.
func setupArcane(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	r := newRecoveryTable(t)
	mage := r.wizard(t, r.ana, "Elora", 3)
	r.expend(t, mage, map[int32]int32{1: 4, 2: 2})
	r.shortRest(t)
	req := &playv1.UseArcaneRecoveryRequest{
		CampaignId: r.campaign, CharacterId: mage.GetId(), IdempotencyKey: newKey(), Slots: []*playv1.SpellSlotBack{{Level: 1, Count: 2}},
	}
	send := func(msg *playv1.UseArcaneRecoveryRequest) func() error {
		return func() error {
			_, err := r.ana.resource.UseArcaneRecovery(t.Context(), connect.NewRequest(proto.Clone(msg).(*playv1.UseArcaneRecoveryRequest)))
			return err
		}
	}
	changed := proto.Clone(req).(*playv1.UseArcaneRecoveryRequest)
	changed.Slots = []*playv1.SpellSlotBack{{Level: 1, Count: 1}}
	g = newHintRig(t, r.h, r.campaign, r.master, map[string]*user{"Ana": r.ana, "Bia": r.bia}, "Bia")
	return g, send(req), send(changed)
}

// The keeper spends the use and gives the slots back before the short rest is looked for: the refusal rolls it all back and
// tells nobody.
func TestUseArcaneRecoveryRefusedAfterTheWriteTellsNothing(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	mage := r.wizard(t, r.ana, "Elora", 3)
	r.expend(t, mage, map[int32]int32{1: 4})
	g := newHintRig(t, r.h, r.campaign, r.master, map[string]*user{"Ana": r.ana, "Bia": r.bia})
	g.refused(func() error {
		_, err := r.recoverSlots(t, r.ana, mage, newKey(), map[int32]int32{1: 2})
		wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_SHORT_REST)
		return err
	})
	if v := r.vitals(t, r.ana, mage); slotsUsed(v, 1) != 4 || usedOf(v, "arcane_recovery") != 0 {
		t.Errorf("after the refusal: 1st-level slots used %d, uses spent %d; want 4 and 0", slotsUsed(v, 1), usedOf(v, "arcane_recovery"))
	}
}

// setupGive is the table, and the writer's call (repeatable: one key) and its key used for another request.
func setupGive(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	a := newResourceTable(t)
	req := &playv1.GiveBardicInspirationOutsideRequest{CampaignId: a.campaignID, TargetCharacterId: a.toren.GetId(), IdempotencyKey: newKey()}
	send := func(msg *playv1.GiveBardicInspirationOutsideRequest) func() error {
		return func() error {
			_, err := a.bia.resource.GiveBardicInspirationOutside(t.Context(), connect.NewRequest(proto.Clone(msg).(*playv1.GiveBardicInspirationOutsideRequest)))
			return err
		}
	}
	changed := proto.Clone(req).(*playv1.GiveBardicInspirationOutsideRequest)
	changed.TargetCharacterId = a.pens.GetId()
	g = newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Ana": a.ana, "Bia": a.bia, "Caio": a.caio})
	return g, send(req), send(changed)
}

// setupRequest is the table, and the writer's call (repeatable: one key) and its key used for another request.
func setupRequest(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	a := newVigil(t)
	key := newKey()
	g = newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Ana": a.ana, "Bia": a.bia, "Caio": a.caio}, "Bia", "Caio")
	ask := func(target *charactersv1.Character) func() error {
		return func() error {
			_, err := a.ask(a.ana, a.pens, target, key)
			return err
		}
	}
	return g, ask(a.toren), ask(a.bri)
}

// setupConfirm is the table, and the writer's call (repeatable: one key) and its key used for another request.
func setupConfirm(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	a := newVigil(t)
	asked, err := a.ask(a.ana, a.pens, a.toren, newKey())
	if err != nil {
		t.Fatalf("RequestRevivify() error = %v", err)
	}
	key := newKey()
	answer := func(within bool) func() error {
		return func() error {
			_, err := a.answerRevivify(t, a.master, asked.GetId(), within, key)
			return err
		}
	}
	g = newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Ana": a.ana, "Bia": a.bia, "Caio": a.caio}, "Bia")
	return g, answer(true), answer(false)
}

func TestSetRevivifyBlockedHints(t *testing.T) {
	t.Parallel()
	a := newVigil(t)
	g := newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Ana": a.ana, "Bia": a.bia, "Caio": a.caio}, "Ana", "Bia", "Caio")
	// A dead character with no combat: the switch moves the number once and tells nobody.
	g.first(func() error { return a.switchRaw(t, switchCharacter(a.toren.GetId()), true) }, false)
	// A living character is refused: nothing moves.
	g.refused(func() error { return a.switchRaw(t, switchCharacter(a.bri.GetId()), true) })
}

func TestSetRevivifyBlockedInACombatHints(t *testing.T) {
	t.Parallel()
	a, e := newMortuary(t)
	a.dies(t, e, "Toren", a.toren)
	g := newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Ana": a.ana, "Bia": a.bia, "Caio": a.caio})
	// A dead combatant: the switch moves the number once and tells the combat.
	g.first(func() error { return a.switchRaw(t, switchCombatant(a.id(t, "Toren")), true) }, true)
	// A living combatant is refused: nothing moves.
	g.refused(func() error { return a.switchRaw(t, switchCombatant(a.id(t, "Brisa")), true) })
}

func TestAdjustCharacterVitalsHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupAdjust(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestAdjustCharacterVitalsLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupAdjust(t)
	g.lost(call)
}

func TestTakeRestHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupTakeRest(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestTakeRestLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupTakeRest(t)
	g.lost(call)
}

func TestSpendHitDiceHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupSpendHitDice(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestSpendHitDiceLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupSpendHitDice(t)
	g.lost(call)
}

func TestUseArcaneRecoveryHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupArcane(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestUseArcaneRecoveryLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupArcane(t)
	g.lost(call)
}

func TestGiveBardicInspirationOutsideHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupGive(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestGiveBardicInspirationOutsideLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupGive(t)
	g.lost(call)
}

func TestRequestRevivifyHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupRequest(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestRequestRevivifyLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupRequest(t)
	g.lost(call)
}

func TestConfirmRevivifyTimeHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupConfirm(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestConfirmRevivifyTimeLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupConfirm(t)
	g.lost(call)
}

// The switch of a dead character with no open session is a plain transaction: it is written, no session's number moves and nobody
// is told. A combatant has no combat to be in.
func TestSetRevivifyBlockedWithNoOpenSession(t *testing.T) {
	t.Parallel()
	a := newVigil(t)
	ended, err := a.h.svc.queries.GetOpenGameSession(t.Context(), a.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	a.endRevivifySession(t)
	before := a.h.hintSeq(t, ended.ID)
	master := listen(t, a.h, a.campaignID, a.master.id, true)
	if err := a.switchRaw(t, switchCharacter(a.toren.GetId()), true); err != nil {
		t.Fatalf("the switch with no open session: error = %v", err)
	}
	var blocked bool
	if err := a.h.pool.QueryRow(t.Context(), `SELECT revivify_blocked FROM characters WHERE id = $1`, a.toren.GetId()).Scan(&blocked); err != nil || !blocked {
		t.Errorf("the character's switch = %v, %v; want written", blocked, err)
	}
	if got := a.h.hintSeq(t, ended.ID); got != before {
		t.Errorf("hint_seq of the ended session = %d, want %d", got, before)
	}
	if got := master.heard(t, a.h); len(got) != 0 {
		t.Errorf("the master heard %v of a switch with no session", eventKinds(got))
	}
	wantCode(t, "a combatant with no open session", a.switchRaw(t, switchCombatant(newKey()), true), connect.CodeFailedPrecondition)
}

// endAndStart ends the open session of the campaign and starts another, from a test hook (it runs on the handler's goroutine, so it
// reports with Errorf).
func endAndStart(t *testing.T, a *armed, again bool) {
	t.Helper()
	open, err := a.h.svc.queries.GetOpenGameSession(t.Context(), a.campaignID)
	if err != nil {
		t.Errorf("GetOpenGameSession() error = %v", err)
		return
	}
	if _, err := a.master.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: a.campaignID, GameSessionId: open.ID})); err != nil {
		t.Errorf("EndGameSession() error = %v", err)
	}
	if !again {
		return
	}
	if _, err := a.master.play.StartGameSession(t.Context(), connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: a.campaignID})); err != nil {
		t.Errorf("StartGameSession() error = %v", err)
	}
}

// A write that read the open session and found it ended when it took the lock is refused as having no open session: nothing is
// written, told or moved.
func TestATxOpenWriteWhoseSessionEndedBeforeTheLockIsRefused(t *testing.T) {
	t.Parallel()
	r := newRestTable(t)
	r.use(t, r.fighter, func(q *playv1.AdjustCharacterVitalsRequest) {
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "second_wind", Used: 1}}
	})
	g := newHintRig(t, r.h, r.campaign, r.master, map[string]*user{"Ana": r.ana})
	var afterEnd int64
	r.h.svc.afterSessionRead = func(string) {
		r.h.svc.afterSessionRead = nil
		if _, err := r.master.play.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: r.campaign, GameSessionId: g.session})); err != nil {
			t.Errorf("EndGameSession() error = %v", err)
		}
		afterEnd = g.seq()
		g.drain() // what the end of the session told
	}
	_, err := r.master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: r.campaign, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()}))
	wantBlocked(t, "TakeRest() on a session that ended", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	if got := g.seq(); got != afterEnd {
		t.Errorf("hint_seq = %d after the refused rest, want %d", got, afterEnd)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of a refused rest", name, eventKinds(evs))
		}
	}
	if v, err := r.h.svc.vitals.GetVitals(t.Context(), r.campaign, r.fighter.GetId()); err != nil || usedOf(v, "second_wind") != 1 {
		t.Errorf("second wind used = %d, %v; want 1: the refused rest changed nothing", usedOf(v, "second_wind"), err)
	}
}

// The session the switch read ended and another opened before the lock: the write starts over under the new session's lock, so it is
// made under the open session and moves that session's number, not the ended one's.
func TestSetRevivifyBlockedFollowsTheSessionThatOpenedBeforeTheLock(t *testing.T) {
	t.Parallel()
	a := newVigil(t)
	first, err := a.h.svc.queries.GetOpenGameSession(t.Context(), a.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	endedSeq := func() int64 { return a.h.hintSeq(t, first.ID) }
	var movedBefore int64
	a.h.svc.afterSessionRead = func(string) {
		a.h.svc.afterSessionRead = nil
		endAndStart(t, a, true)
		movedBefore = endedSeq()
	}
	if err := a.switchRaw(t, switchCharacter(a.toren.GetId()), true); err != nil {
		t.Fatalf("the switch: error = %v", err)
	}
	second, err := a.h.svc.queries.GetOpenGameSession(t.Context(), a.campaignID)
	if err != nil || second.ID == first.ID {
		t.Fatalf("open session = %v, %v; want a new one", second.ID, err)
	}
	if got := endedSeq(); got != movedBefore {
		t.Errorf("hint_seq of the ended session = %d, want %d: the write moved it", got, movedBefore)
	}
	if got := a.h.hintSeq(t, second.ID); got != 1 {
		t.Errorf("hint_seq of the open session = %d, want 1: the switch is told under its lock", got)
	}
	var blocked bool
	if err := a.h.pool.QueryRow(t.Context(), `SELECT revivify_blocked FROM characters WHERE id = $1`, a.toren.GetId()).Scan(&blocked); err != nil || !blocked {
		t.Errorf("the character's switch = %v, %v; want written", blocked, err)
	}
}
