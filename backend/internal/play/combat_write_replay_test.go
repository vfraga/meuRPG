package play

import (
	"fmt"
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// The replay rule of a combat write (#735): a change whose key is stored already changes nothing, but it tells the streams what
// the first call would have told, because that call may have committed and died before it sent. Each test makes the first call with
// nobody listening (every send of the call is dropped, as a send lost to a crash is), subscribes, and repeats the key.

// heardBy is a stream open on the table's hub: the master's, or a player's.
type heardBy struct{ sub *live.Subscription }

func listen(t *testing.T, h *harness, campaignID, userID string, master bool) heardBy {
	t.Helper()
	sub, err := h.svc.hub.Subscribe(campaignID, live.Subscriber{UserID: userID, Master: master})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	t.Cleanup(sub.Close)
	return heardBy{sub}
}

// heard drains what the stream received, once the hint gate has sent its trailing hints.
func (s heardBy) heard(t *testing.T, h *harness) []*playv1.WatchGameSessionResponse {
	t.Helper()
	h.hints.Settle(t)
	var out []*playv1.WatchGameSessionResponse
	for {
		select {
		case ev := <-s.sub.Events():
			s.sub.Taken(ev)
			if ev.Message != nil {
				out = append(out, ev.Message)
			}
		default:
			return out
		}
	}
}

// kinds names the events as the oneof of each one: "*playv1.WatchGameSessionResponse_VitalsChanged_".
func eventKinds(evs []*playv1.WatchGameSessionResponse) []string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, fmt.Sprintf("%T", ev.GetEvent()))
	}
	slices.Sort(out)
	return out
}

func (h *harness) hintSeq(t *testing.T, sessionID string) int64 {
	t.Helper()
	n, err := h.svc.queries.GetHintSeq(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("GetHintSeq() error = %v", err)
	}
	return n
}

const (
	kVitals = "*playv1.WatchGameSessionResponse_VitalsChanged_"
	kCasts  = "*playv1.WatchGameSessionResponse_SpellCastsChanged_"
	kVision = "*playv1.WatchGameSessionResponse_VisionChanged_"
)

// writeThatTells is a combat change that, as a real one does, leaves a public change of the casts and the vitals of Toren
// to tell.
func (x *coreFixture) writeThatTells(t *testing.T, key string) (combatResult, error) {
	t.Helper()
	ctx := t.Context()
	torenID := x.torenID(t)
	v, err := x.h.svc.vitals.GetVitals(ctx, x.campaignID, torenID)
	if err != nil {
		t.Fatalf("GetVitals() error = %v", err)
	}
	m := authz.Membership{UserID: x.master.id, CampaignID: x.campaignID, Role: authz.RoleMaster}
	w := combatWrite{m: m, key: key, kind: eventCombatantHiddenSet, encounterID: x.e.GetId()}
	return x.h.svc.write(ctx, w, func(c *combatTx) (any, error) {
		c.castsChanged = castsPublic
		c.told = append(c.told, v)
		return map[string]any{"encounter_id": x.e.GetId()}, nil
	})
}

func (x *coreFixture) torenID(t *testing.T) string {
	t.Helper()
	return x.toren.GetId()
}

func (x *coreFixture) sessionID(t *testing.T) string {
	t.Helper()
	sess, err := x.h.svc.queries.GetOpenGameSession(t.Context(), x.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	return sess.ID
}

// A change that happens tells what it left, to the audiences of each hint, and the number it stamps on the events is the one
// it stored: one more than before.
func TestWriteTellsWhatTheChangeLeftAndStampsTheNumber(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t)
	sid := x.sessionID(t)
	master := listen(t, x.h, x.campaignID, x.master.id, true)
	caio := listen(t, x.h, x.campaignID, x.caio.id, false)
	ana := listen(t, x.h, x.campaignID, x.ana.id, false)
	before := x.h.hintSeq(t, sid)
	fog := x.h.vision.n.Load()

	if _, err := x.writeThatTells(t, newKey()); err != nil {
		t.Fatalf("write() error = %v", err)
	}
	if got := x.h.vision.n.Load() - fog; got != 1 {
		t.Errorf("the fog was told %d times, want 1", got)
	}
	if got, want := x.h.hintSeq(t, sid), before+1; got != want {
		t.Errorf("hint_seq = %d after a change, want %d (one more)", got, want)
	}
	m, c, a := master.heard(t, x.h), caio.heard(t, x.h), ana.heard(t, x.h)
	// The master hears the casts and Toren's vitals; Toren's player the same; Ana the casts only.
	if got, want := eventKinds(m), []string{kCasts, kVitals}; !slices.Equal(got, want) {
		t.Errorf("the master heard %v, want %v", got, want)
	}
	if got, want := eventKinds(c), []string{kCasts, kVitals}; !slices.Equal(got, want) {
		t.Errorf("Toren's player heard %v, want %v", got, want)
	}
	if got, want := eventKinds(a), []string{kCasts}; !slices.Equal(got, want) {
		t.Errorf("Pensantus's player heard %v, want %v", got, want)
	}
	for _, ev := range append(m, c...) {
		if ev.GetSessionSeq() != before+1 {
			t.Errorf("%T carries number %d, want the stored %d", ev.GetEvent(), ev.GetSessionSeq(), before+1)
		}
	}
}

// RED before the replay rule: the repeat of a key whose first call told nobody (the send was lost) tells now what the first
// call would have told, and the number stays where the first call left it.
func TestWriteReplayTellsAgainWhatTheFirstCallLeft(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t)
	sid := x.sessionID(t)
	key := newKey()
	before := x.h.hintSeq(t, sid)

	first, err := x.writeThatTells(t, key) // nobody listens: the call's sends are lost
	if err != nil || first.repeated {
		t.Fatalf("first write() = repeated %v, %v; want a new change", first.repeated, err)
	}
	stored := x.h.hintSeq(t, sid)
	if stored != before+1 {
		t.Errorf("hint_seq = %d after the first call, want %d", stored, before+1)
	}
	master := listen(t, x.h, x.campaignID, x.master.id, true)
	caio := listen(t, x.h, x.campaignID, x.caio.id, false)
	ana := listen(t, x.h, x.campaignID, x.ana.id, false)

	fog := x.h.vision.n.Load()
	again, err := x.writeThatTells(t, key)
	if err != nil || !again.repeated {
		t.Fatalf("repeated write() = repeated %v, %v; want a repeat", again.repeated, err)
	}
	if got := x.h.hintSeq(t, sid); got != stored {
		t.Errorf("hint_seq = %d after the repeat, want %d: a repeat writes nothing", got, stored)
	}
	if got := x.h.vision.n.Load() - fog; got != 1 {
		t.Errorf("the fog was told %d times on the repeat, want 1", got)
	}
	m, c, a := master.heard(t, x.h), caio.heard(t, x.h), ana.heard(t, x.h)
	// The vitals of Toren, Pensantus and Brisa: each to the master and its own player; the casts to the master only (the level of the
	// first call's change is not kept).
	if got, want := eventKinds(m), []string{kCasts, kVitals, kVitals, kVitals}; !slices.Equal(got, want) {
		t.Errorf("the master heard %v on the repeat, want %v", got, want)
	}
	if got, want := eventKinds(c), []string{kVitals}; !slices.Equal(got, want) {
		t.Errorf("Toren's player heard %v on the repeat, want %v", got, want)
	}
	if got, want := eventKinds(a), []string{kVitals}; !slices.Equal(got, want) {
		t.Errorf("Pensantus's player heard %v on the repeat, want %v", got, want)
	}
	for _, ev := range slices.Concat(m, c, a) {
		if ev.GetSessionSeq() != stored {
			t.Errorf("%T carries number %d on the repeat, want the stored %d", ev.GetEvent(), ev.GetSessionSeq(), stored)
		}
	}
}

// A repeat of a key used for another kind of change is still refused, and tells nothing.
func TestWriteReplayOfAnotherChangeTellsNothing(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t)
	key := newKey()
	if _, err := x.writeThatTells(t, key); err != nil {
		t.Fatalf("write() error = %v", err)
	}
	master := listen(t, x.h, x.campaignID, x.master.id, true)
	m := authz.Membership{UserID: x.master.id, CampaignID: x.campaignID, Role: authz.RoleMaster}
	_, err := x.h.svc.write(t.Context(), combatWrite{m: m, key: key, kind: eventTurnEnded, encounterID: x.e.GetId()}, func(*combatTx) (any, error) {
		t.Error("the change ran for a key used by another kind")
		return nil, nil
	})
	wantCode(t, "write(reused key)", err, connect.CodeInvalidArgument)
	if got := master.heard(t, x.h); len(got) != 0 {
		t.Errorf("a refused repeat told %v", eventKinds(got))
	}
}

// A start that made the point's map the current one tells the current map, and its repeat tells it again.
func TestStartEncounterReplayTellsTheCurrentMapAgain(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	sid := f.session.GetId()
	target := f.h.newMap(f.campaignID, 10)
	var point string
	if err := f.h.pool.QueryRow(t.Context(),
		`INSERT INTO map_points (map_id, kind, name, x_bp, y_bp, target_map_id, created_at, updated_at)
		 VALUES ($1, 'battle', 'Emboscada', 100, 100, $2, now(), now()) RETURNING id`, f.mapID, target).Scan(&point); err != nil {
		t.Fatalf("insert point: %v", err)
	}
	req := &playv1.StartEncounterRequest{
		CampaignId: f.campaignID, IdempotencyKey: newKey(), Name: "Emboscada", MapPointId: point,
		Participants: []*playv1.Participant{{CharacterId: f.goblin.GetId()}},
	}
	start := func() error {
		_, err := f.master.combat.StartEncounter(t.Context(), connect.NewRequest(req))
		return err
	}
	before := f.h.hintSeq(t, sid)
	if err := start(); err != nil { // nobody listens: the sends are lost
		t.Fatalf("StartEncounter() error = %v", err)
	}
	stored := f.h.hintSeq(t, sid)
	if stored != before+1 {
		t.Errorf("hint_seq = %d after the start, want %d", stored, before+1)
	}
	master := listen(t, f.h, f.campaignID, f.master.id, true)
	ana := listen(t, f.h, f.campaignID, f.ana.id, false)
	if err := start(); err != nil {
		t.Fatalf("StartEncounter(repeat) error = %v", err)
	}
	if got := f.h.hintSeq(t, sid); got != stored {
		t.Errorf("hint_seq = %d after the repeat, want %d", got, stored)
	}
	for name, s := range map[string]heardBy{"master": master, "Pensantus's player": ana} {
		var maps []string
		for _, ev := range s.heard(t, f.h) {
			if m := ev.GetCurrentMapChanged(); m != nil {
				maps = append(maps, m.GetMapId())
				if ev.GetSessionSeq() != stored {
					t.Errorf("%s: current_map_changed carries number %d, want %d", name, ev.GetSessionSeq(), stored)
				}
			}
		}
		if !slices.Equal(maps, []string{target}) {
			t.Errorf("%s heard the current map %v on the repeat, want [%s]", name, maps, target)
		}
	}
}

// The start that happens tells the current map once, stamped with the number the start stored.
func TestStartEncounterTellsTheCurrentMapOnceAndStampsTheNumber(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	sid := f.session.GetId()
	target := f.h.newMap(f.campaignID, 10)
	var point string
	if err := f.h.pool.QueryRow(t.Context(),
		`INSERT INTO map_points (map_id, kind, name, x_bp, y_bp, target_map_id, created_at, updated_at)
		 VALUES ($1, 'battle', 'Emboscada', 100, 100, $2, now(), now()) RETURNING id`, f.mapID, target).Scan(&point); err != nil {
		t.Fatalf("insert point: %v", err)
	}
	master := listen(t, f.h, f.campaignID, f.master.id, true)
	before := f.h.hintSeq(t, sid)
	if _, err := f.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: f.campaignID, IdempotencyKey: newKey(), Name: "Emboscada", MapPointId: point,
		Participants: []*playv1.Participant{{CharacterId: f.goblin.GetId()}},
	})); err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	if got := f.h.hintSeq(t, sid); got != before+1 {
		t.Errorf("hint_seq = %d after the start, want %d", got, before+1)
	}
	maps := 0
	for _, ev := range master.heard(t, f.h) {
		if ev.GetCurrentMapChanged() != nil {
			maps++
			if ev.GetSessionSeq() != before+1 {
				t.Errorf("current_map_changed carries number %d, want %d", ev.GetSessionSeq(), before+1)
			}
		}
	}
	if maps != 1 {
		t.Errorf("the master heard the current map %d times, want 1", maps)
	}
}

// RED before the replay of the forced moves: a Thunderwave pushed Goblin 1 and the first call's sends were lost; the repeat of
// its key tells the push again, to the master, from the hits the stored event keeps.
func TestCastReplayTellsTheForcedMoveAgain(t *testing.T) {
	t.Parallel()
	c := newAreaCave(t)
	c.waveFight(t, [2]int32{17, 7}, [2]int32{18, 7})
	key := newKey()
	cast := func() {
		t.Helper()
		c.mustCastArea(t, c.ana, "Pensantus", thunderwave, slotOfLevel(1), nil, toward(1, 0), func(r *playv1.CastSpellRequest) { r.IdempotencyKey = key })
	}
	cast() // nobody listens: the first call's sends are lost
	master := listen(t, c.h, c.campaignID, c.master.id, true)
	ana := listen(t, c.h, c.campaignID, c.ana.id, false)
	cast()

	goblin := c.id(t, "Goblin 1")
	for name, s := range map[string]heardBy{"master": master, "Pensantus's player": ana} {
		var moved []string
		for _, ev := range s.heard(t, c.h) {
			if m := ev.GetCombatantMoved(); m != nil {
				moved = append(moved, fmt.Sprintf("%s@%d,%d", m.GetCombatantId(), m.GetCol(), m.GetRow()))
			}
		}
		if want := []string{fmt.Sprintf("%s@20,7", goblin)}; !slices.Equal(moved, want) {
			t.Errorf("%s heard the moves %v on the repeat, want %v", name, moved, want)
		}
	}
}

// On a repeat, a player who owns nothing in it hears no master-only cast change and nobody else's vitals.
func TestWriteReplayKeepsTheMasterOnlyHintsFromOtherPlayers(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t)
	key := newKey()
	if _, err := x.writeThatTells(t, key); err != nil {
		t.Fatalf("write() error = %v", err)
	}
	// Bia's player owns Brisa; the change the first call made was about Toren.
	bia := listen(t, x.h, x.campaignID, x.bia.id, false)
	if _, err := x.writeThatTells(t, key); err != nil {
		t.Fatalf("repeated write() error = %v", err)
	}
	for _, ev := range bia.heard(t, x.h) {
		switch {
		case ev.GetSpellCastsChanged() != nil:
			t.Error("a player heard the master-only cast change on a repeat")
		case ev.GetVitalsChanged() != nil && ev.GetVitalsChanged().GetVitals().GetCharacterId() != x.bri.GetId():
			t.Errorf("a player heard the vitals of %s on a repeat", ev.GetVitalsChanged().GetVitals().GetCharacterId())
		}
	}
}
