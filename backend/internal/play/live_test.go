package play

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The live session (Etapa 5): the notice (RN-06), the vitals and the
// master's correction (RN-02), session_events (ADR-0007), and the stream
// (ADR-0005). These tests need the database (MEURPG_TEST_DATABASE_URL).

// waitLimit bounds every wait on a stream, so a broken stream fails the
// test instead of hanging it.
const waitLimit = 5 * time.Second

func newKey() string { return uuid.New().String() }

// adjust calls AdjustCharacterVitals as u, with a new idempotency key.
func (u *user) adjust(t *testing.T, campaignID, characterID string, edit func(*playv1.AdjustCharacterVitalsRequest)) (*playv1.CharacterVitals, error) {
	t.Helper()
	req := &playv1.AdjustCharacterVitalsRequest{CampaignId: campaignID, CharacterId: characterID, IdempotencyKey: newKey()}
	edit(req)
	res, err := u.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetVitals(), nil
}

// liveSession calls GetLiveSession as u, or fails the test.
func (u *user) liveSession(t *testing.T, campaignID string) *playv1.GetLiveSessionResponse {
	t.Helper()
	res, err := u.play.GetLiveSession(t.Context(), connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("GetLiveSession() error = %v", err)
	}
	return res.Msg
}

// watcher reads one WatchGameSession stream in the background.
type watcher struct {
	events chan *playv1.WatchGameSessionResponse
	done   chan error // the stream's end: nil, or its error
	cancel context.CancelFunc
	// markers is how many markers (markCurrentMap) the stream has been sent and
	// beforeMarker has not read yet.
	markers int
}

// watch opens WatchGameSession as u.
func (u *user) watch(t *testing.T, campaignID string) *watcher {
	t.Helper()
	return watchWith(t, u.play, campaignID)
}

func watchWith(t *testing.T, client playv1connect.PlayServiceClient, campaignID string) *watcher {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	w := &watcher{events: make(chan *playv1.WatchGameSessionResponse, 64), done: make(chan error, 1), cancel: cancel}
	stream, err := client.WatchGameSession(ctx, connect.NewRequest(&playv1.WatchGameSessionRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("WatchGameSession() error = %v", err)
	}
	go func() {
		defer close(w.events)
		for stream.Receive() {
			w.events <- stream.Msg()
		}
		w.done <- stream.Err()
		_ = stream.Close()
	}()
	return w
}

// next returns the stream's next event, heartbeats included.
func (w *watcher) next(t *testing.T) *playv1.WatchGameSessionResponse {
	t.Helper()
	select {
	case ev, ok := <-w.events:
		if !ok {
			t.Fatalf("the stream ended (%v), want another event", <-w.done)
		}
		return ev
	case <-time.After(waitLimit):
		t.Fatal("no event within the time limit")
	}
	return nil
}

// nextChange returns the stream's next event that is not a heartbeat.
func (w *watcher) nextChange(t *testing.T) *playv1.WatchGameSessionResponse {
	t.Helper()
	for {
		if ev := w.next(t); ev.GetHeartbeat() == nil {
			return ev
		}
	}
}

// markCurrentMap has the master show the map the table already shows: every stream
// of the campaign gets a current_map_changed, the marker that ends a wait for nothing
// (see beforeMarker). ws are all the watchers of the campaign that will be read with
// beforeMarker, each of which gets to count the marker.
func (u *user) markCurrentMap(t *testing.T, campaignID, mapID string, ws ...*watcher) {
	t.Helper()
	if _, err := u.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: campaignID, MapId: mapID})); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	for _, w := range ws {
		w.markers++
	}
}

// beforeMarker reads the stream up to the markers markCurrentMap has sent it, and
// returns the events (heartbeats left out) that came before the last one. The hub
// delivers in the order it published, so what an action before the marker sent is
// among them however late it was delivered, and an empty result means it sent nothing.
// It fails if a marker does not arrive.
func (w *watcher) beforeMarker(t *testing.T) []*playv1.WatchGameSessionResponse {
	t.Helper()
	if w.markers == 0 {
		t.Fatal("beforeMarker: no marker was sent to this stream (markCurrentMap)")
	}
	var before []*playv1.WatchGameSessionResponse
	for w.markers > 0 {
		ev := w.next(t)
		switch {
		case ev.GetCurrentMapChanged() != nil:
			w.markers--
		case ev.GetHeartbeat() == nil:
			before = append(before, ev)
		}
	}
	return before
}

// ready reads the first event, which must be ready, and returns its session.
func (w *watcher) ready(t *testing.T) *playv1.GameSession {
	t.Helper()
	ev := w.next(t)
	if ev.GetReady() == nil {
		t.Fatalf("first event = %v, want ready", ev)
	}
	return ev.GetReady().GetGameSession()
}

// end waits for the stream to end, skipping heartbeats, and returns its
// error. Any other event fails the test.
func (w *watcher) end(t *testing.T) error {
	t.Helper()
	timeout := time.After(waitLimit)
	for {
		select {
		case ev, ok := <-w.events:
			if !ok {
				return <-w.done
			}
			if ev.GetHeartbeat() == nil {
				t.Fatalf("event %v, want the stream to end", ev)
			}
		case <-timeout:
			t.Fatal("the stream did not end within the time limit")
		}
	}
}

func wantBlocked(t *testing.T, call string, err error, reason playv1.GameSessionBlockedReason) {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		if msg, derr := d.Value(); derr == nil {
			if b, ok := msg.(*playv1.GameSessionBlocked); ok && b.GetReason() == reason {
				return
			}
		}
	}
	t.Errorf("%s error = %v, want a GameSessionBlocked detail with %v", call, err, reason)
}

// sessionEvent is a session_events row, read directly.
type sessionEvent struct {
	seq         int32
	kind        string
	actorUserID *string
	characterID *string
	payload     map[string]map[string]any
	key         *string
}

func (h *harness) sessionEvents(sessionID string) []sessionEvent {
	h.t.Helper()
	rows, err := h.pool.Query(h.t.Context(),
		`SELECT seq, kind, actor_user_id::STRING, character_id::STRING, payload, idempotency_key::STRING
		 FROM session_events WHERE game_session_id = $1 ORDER BY seq`, sessionID)
	if err != nil {
		h.t.Fatalf("read session_events: %v", err)
	}
	defer rows.Close()
	var out []sessionEvent
	for rows.Next() {
		var e sessionEvent
		var payload []byte
		if err := rows.Scan(&e.seq, &e.kind, &e.actorUserID, &e.characterID, &payload, &e.key); err != nil {
			h.t.Fatalf("scan session_events: %v", err)
		}
		if err := json.Unmarshal(payload, &e.payload); err != nil {
			h.t.Fatalf("payload %s: %v", payload, err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		h.t.Fatalf("read session_events: %v", err)
	}
	return out
}

// createWarlock creates a level 1 warlock as u, who has pact magic slots
// and no ordinary spell slots.
func (u *user) createWarlock(t *testing.T, campaignID, name string) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 8, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16},
		RaceKey:    "race:human",
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:warlock", Level: 1}},
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(warlock) error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// RN-02: the master corrects hit points, spell slots and hit dice during
// the session, and only then; a player may not; values stay within the
// sheet's maximums; each correction is a session event.
func TestRN02_MasterAdjustsVitalsDuringSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, other, outsider := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Bruxo"), h.newUser("De fora")
	campaign := h.newCampaign(master, "Mirathel", player, other)
	wizard := player.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	warlock := other.createWarlock(t, campaign, "Morgana")
	npc := master.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	hpMax := wizard.GetDerived().GetHitPointsMax()
	if hpMax < 2 {
		t.Fatalf("the wizard's hit_points_max = %d, want a real number", hpMax)
	}

	// Outside a session: failed_precondition, NO_OPEN_SESSION.
	_, err := master.adjust(t, campaign, wizard.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(1) })
	wantBlocked(t, "AdjustCharacterVitals() before the session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)

	session := master.start(t, campaign).GetGameSession()

	// Fresh: full hit points, nothing used, revision 0.
	fresh := master.liveSession(t, campaign).GetVitals()
	if len(fresh) != 2 {
		t.Fatalf("master's vitals = %d characters, want the 2 player characters (no NPC)", len(fresh))
	}
	v := fresh[0]
	if v.GetCharacterId() != wizard.GetId() || v.GetHitPointsCurrent() != hpMax || v.GetHitPointsMax() != hpMax ||
		v.GetRevision() != 0 || v.GetUpdatedAt() != nil || v.GetHitDiceUsed() != 0 || v.GetHitDiceTotal() != 1 {
		t.Errorf("fresh vitals = %v, want full hit points and nothing used", v)
	}
	if len(v.GetSpellSlots()) != 1 || v.GetSpellSlots()[0].GetLevel() != 1 || v.GetSpellSlots()[0].GetTotal() != 2 || v.GetPactSlots() != nil {
		t.Errorf("wizard's slots = %v / %v, want 2 of level 1 and no pact slots", v.GetSpellSlots(), v.GetPactSlots())
	}
	if w := fresh[1]; w.GetPactSlots().GetTotal() != 1 || w.GetPactSlots().GetSlotLevel() != 1 || len(w.GetSpellSlots()) != 0 {
		t.Errorf("warlock's slots = %v / %v, want 1 pact slot of level 1 only", w.GetSpellSlots(), w.GetPactSlots())
	}

	got, err := master.adjust(t, campaign, wizard.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) {
		r.HitPointsCurrent = proto.Int32(1)
		r.HitPointsTemporary = proto.Int32(5)
		r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 1, Used: 2}}
		r.HitDiceUsed = proto.Int32(1)
	})
	if err != nil {
		t.Fatalf("master's AdjustCharacterVitals() error = %v", err)
	}
	if got.GetHitPointsCurrent() != 1 || got.GetHitPointsTemporary() != 5 || got.GetSpellSlots()[0].GetUsed() != 2 ||
		got.GetHitDiceUsed() != 1 || got.GetRevision() != 1 || got.GetUpdatedAt() == nil {
		t.Errorf("AdjustCharacterVitals() = %v, want the new values at revision 1", got)
	}
	// Unset values stay as they are.
	got, err = master.adjust(t, campaign, wizard.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(0) })
	if err != nil || got.GetHitPointsCurrent() != 0 || got.GetHitPointsTemporary() != 5 || got.GetSpellSlots()[0].GetUsed() != 2 || got.GetRevision() != 2 {
		t.Errorf("second AdjustCharacterVitals() = %v, %v; want only the hit points changed, revision 2", got, err)
	}
	if pact, err := master.adjust(t, campaign, warlock.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.PactSlotsUsed = proto.Int32(1) }); err != nil || pact.GetPactSlots().GetUsed() != 1 {
		t.Errorf("warlock's AdjustCharacterVitals(pact) = %v, %v; want 1 pact slot used", pact, err)
	}
	// The player sees the master's correction.
	if mine := player.liveSession(t, campaign).GetVitals(); len(mine) != 1 || mine[0].GetHitPointsCurrent() != 0 || mine[0].GetRevision() != 2 {
		t.Errorf("player's vitals = %v, want their character at 0 hit points, revision 2", mine)
	}

	// Each correction is one event, in order, with who and what.
	events := h.sessionEvents(session.GetId())
	if len(events) != 3 {
		t.Fatalf("session_events = %d rows, want 3", len(events))
	}
	first := events[0]
	if first.seq != 1 || first.kind != "character_vitals_adjusted" || first.actorUserID == nil || *first.actorUserID != master.id ||
		first.characterID == nil || *first.characterID != wizard.GetId() || first.key == nil {
		t.Errorf("first event = %+v", first)
	}
	if first.payload["before"]["hit_points_current"] != float64(hpMax) || first.payload["after"]["hit_points_current"] != float64(1) {
		t.Errorf("first event payload = %v, want the hit points before and after", first.payload)
	}

	// Who may not.
	_, err = player.adjust(t, campaign, wizard.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(1) })
	wantCode(t, "player's AdjustCharacterVitals()", err, connect.CodePermissionDenied)
	_, err = outsider.adjust(t, campaign, wizard.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(1) })
	wantCode(t, "non-member's AdjustCharacterVitals()", err, connect.CodeNotFound)

	// What may not be adjusted: an NPC, or a character that does not exist.
	for name, id := range map[string]string{"an NPC": npc.GetId(), "a made-up character": newKey(), "not a UUID": "x"} {
		_, err = master.adjust(t, campaign, id, func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(1) })
		wantCode(t, "AdjustCharacterVitals("+name+")", err, connect.CodeNotFound)
	}

	// Values outside 0 to their maximum, named by field.
	invalid := []struct {
		field string
		edit  func(*playv1.AdjustCharacterVitalsRequest)
	}{
		{"hit_points_current", func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = new(hpMax + 1) }},
		{"hit_points_current", func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(-1) }},
		{"hit_points_temporary", func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsTemporary = proto.Int32(1000) }},
		{"spell_slots_used[0].used", func(r *playv1.AdjustCharacterVitalsRequest) {
			r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 1, Used: 3}}
		}},
		{"spell_slots_used[0].level", func(r *playv1.AdjustCharacterVitalsRequest) {
			r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 2, Used: 1}}
		}},
		{"spell_slots_used[1].level", func(r *playv1.AdjustCharacterVitalsRequest) {
			r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 1, Used: 1}, {Level: 1, Used: 0}}
		}},
		{"pact_slots_used", func(r *playv1.AdjustCharacterVitalsRequest) { r.PactSlotsUsed = proto.Int32(0) }},
		{"hit_dice_used", func(r *playv1.AdjustCharacterVitalsRequest) { r.HitDiceUsed = proto.Int32(2) }},
		{"request", func(*playv1.AdjustCharacterVitalsRequest) {}},
		{"idempotency_key", func(r *playv1.AdjustCharacterVitalsRequest) {
			r.IdempotencyKey = "not-a-uuid"
			r.HitPointsCurrent = proto.Int32(1)
		}},
	}
	for _, tt := range invalid {
		_, err := master.adjust(t, campaign, wizard.GetId(), tt.edit)
		wantCode(t, "AdjustCharacterVitals("+tt.field+")", err, connect.CodeInvalidArgument)
		if err != nil && !strings.Contains(err.Error(), tt.field) {
			t.Errorf("AdjustCharacterVitals() error = %v, want it to name %s", err, tt.field)
		}
	}
	if n := len(h.sessionEvents(session.GetId())); n != 3 {
		t.Errorf("session_events = %d rows after refused calls, want still 3", n)
	}

	// A dead character is no longer at the table.
	if _, err := master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: campaign, CharacterId: warlock.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	_, err = master.adjust(t, campaign, warlock.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.PactSlotsUsed = proto.Int32(0) })
	wantCode(t, "AdjustCharacterVitals(dead)", err, connect.CodeNotFound)
	if n := len(master.liveSession(t, campaign).GetVitals()); n != 1 {
		t.Errorf("master's vitals after a death = %d characters, want 1", n)
	}

	// After the session ends: failed_precondition again.
	master.end(t, session)
	_, err = master.adjust(t, campaign, wizard.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(1) })
	wantBlocked(t, "AdjustCharacterVitals() after the session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	// The vitals last until the next session (RN-02).
	master.start(t, campaign)
	if v := master.liveSession(t, campaign).GetVitals()[0]; v.GetHitPointsCurrent() != 0 || v.GetRevision() != 2 {
		t.Errorf("vitals in the next session = %v, want them kept", v)
	}
}

// TestAdjustCharacterVitalsIsIdempotent: a retry with the same key writes
// nothing, publishes nothing, and answers with the current vitals.
func TestAdjustCharacterVitalsIsIdempotent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, other := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Outro")
	campaign := h.newCampaign(master, "Mirathel", player, other)
	pc := player.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	otherPC := other.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Lia")
	session := master.start(t, campaign).GetGameSession()
	stream := player.watch(t, campaign)
	stream.ready(t)

	call := func(key, characterID string, hp int32) (*playv1.CharacterVitals, error) {
		res, err := master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
			CampaignId: campaign, CharacterId: characterID, IdempotencyKey: key, HitPointsCurrent: new(hp),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetVitals(), nil
	}
	keyA, keyB := newKey(), newKey()
	first, err := call(keyA, pc.GetId(), 3)
	if err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	again, err := call(keyA, pc.GetId(), 3)
	if err != nil || !proto.Equal(first, again) {
		t.Errorf("the same call again = %v, %v; want the same answer %v", again, err, first)
	}
	if n := len(h.sessionEvents(session.GetId())); n != 1 {
		t.Errorf("session_events = %d rows, want 1", n)
	}
	// Later changes win: a late retry answers with the vitals as they are
	// now, and does not undo them.
	if _, err := call(keyB, pc.GetId(), 5); err != nil {
		t.Fatalf("AdjustCharacterVitals(another key) error = %v", err)
	}
	late, err := call(keyA, pc.GetId(), 3)
	if err != nil || late.GetHitPointsCurrent() != 5 || late.GetRevision() != 2 {
		t.Errorf("a late retry = %v, %v; want the current vitals (5 hit points, revision 2)", late, err)
	}
	if n := len(h.sessionEvents(session.GetId())); n != 2 {
		t.Errorf("session_events = %d rows, want 2", n)
	}
	// The key belongs to that change: using it for another character is a
	// mistake, not a retry.
	_, err = call(keyA, otherPC.GetId(), 3)
	wantCode(t, "AdjustCharacterVitals(the key of another change)", err, connect.CodeInvalidArgument)
	// Nor is it a retry when it comes with other numbers for the same character.
	_, err = call(keyA, pc.GetId(), 4)
	wantCode(t, "AdjustCharacterVitals(the key with other hit points)", err, connect.CodeInvalidArgument)
	if v := master.liveSession(t, campaign).GetVitals(); len(v) == 0 || v[0].GetHitPointsCurrent() != 5 {
		t.Errorf("vitals after the refused call = %v, want them as they were", v)
	}

	// The player's stream got exactly the two changes, not the retries.
	for _, want := range []int32{3, 5} {
		if got := stream.nextChange(t).GetVitalsChanged().GetVitals().GetHitPointsCurrent(); got != want {
			t.Errorf("vitals_changed hit points = %d, want %d", got, want)
		}
	}
	master.end(t, session)
	if ev := stream.nextChange(t); ev.GetSessionEnded() == nil {
		t.Errorf("event after the retries = %v, want session_ended (no vitals_changed for a retry)", ev)
	}
}

// TestSessionEventsAreOrderedPerSession: concurrent corrections get the
// event numbers 1, 2, 3... with no gap and no repeat, and each session
// counts from 1.
func TestSessionEventsAreOrderedPerSession(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 12) // the racers must overlap: one connection would run them one by one
	h := newHarness(t)
	master := h.newUser("Mestre")
	players := []*user{h.newUser("Ana"), h.newUser("Bruno"), h.newUser("Carla")}
	campaign := h.newCampaign(master, "Mirathel", players...)
	var pcs []*charactersv1.Character
	for _, p := range players {
		pcs = append(pcs, p.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "PC de "+p.id[:4]))
	}
	session := master.start(t, campaign).GetGameSession()

	const perCharacter = 3
	var wg sync.WaitGroup
	errs := make(chan error, len(pcs)*perCharacter)
	start := dbtest.NewBarrier(len(pcs) * perCharacter)
	for _, pc := range pcs {
		for _, temporary := range []int32{1, 2, 3} {
			wg.Go(func() {
				start.Wait()
				_, err := master.play.AdjustCharacterVitals(context.Background(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
					CampaignId: campaign, CharacterId: pc.GetId(), IdempotencyKey: newKey(), HitPointsTemporary: new(temporary),
				}))
				errs <- err
			})
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent AdjustCharacterVitals() error = %v", err)
		}
	}

	events := h.sessionEvents(session.GetId())
	var seqs []int32
	for _, e := range events {
		seqs = append(seqs, e.seq)
	}
	want := []int32{1, 2, 3, 4, 5, 6, 7, 8, 9}
	if !slices.Equal(seqs, want) {
		t.Errorf("seq = %v, want %v", seqs, want)
	}
	// Each character's revision counts its own corrections.
	for _, v := range master.liveSession(t, campaign).GetVitals() {
		if v.GetRevision() != perCharacter {
			t.Errorf("%s revision = %d, want %d", v.GetName(), v.GetRevision(), perCharacter)
		}
	}

	master.end(t, session)
	next := master.start(t, campaign).GetGameSession()
	if _, err := master.adjust(t, campaign, pcs[0].GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitDiceUsed = proto.Int32(1) }); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	if events := h.sessionEvents(next.GetId()); len(events) != 1 || events[0].seq != 1 {
		t.Errorf("the next session's events = %+v, want one with seq 1", events)
	}
}

// TestPlayersSeeOnlyTheirOwnVitals (question 28's default): the master sees
// every character's vitals; a player, only their own character's, in the
// snapshot and on the stream.
func TestPlayersSeeOnlyTheirOwnVitals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana, bruno, carla := h.newUser("Mestre"), h.newUser("Ana"), h.newUser("Bruno"), h.newUser("Carla")
	campaign := h.newCampaign(master, "Mirathel", ana, bruno, carla) // Carla has no character
	anaPC := ana.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	brunoPC := bruno.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Lia")
	master.start(t, campaign)

	ids := func(vs []*playv1.CharacterVitals) []string {
		var out []string
		for _, v := range vs {
			out = append(out, v.GetCharacterId())
		}
		return out
	}
	if got := ids(master.liveSession(t, campaign).GetVitals()); !slices.Equal(got, []string{anaPC.GetId(), brunoPC.GetId()}) {
		t.Errorf("master's vitals = %v, want both characters", got)
	}
	if got := ids(ana.liveSession(t, campaign).GetVitals()); !slices.Equal(got, []string{anaPC.GetId()}) {
		t.Errorf("Ana's vitals = %v, want only her character", got)
	}
	if got := carla.liveSession(t, campaign).GetVitals(); len(got) != 0 {
		t.Errorf("Carla's vitals = %v, want none", got)
	}

	masterStream, anaStream, brunoStream, carlaStream := master.watch(t, campaign), ana.watch(t, campaign), bruno.watch(t, campaign), carla.watch(t, campaign)
	for _, s := range []*watcher{masterStream, anaStream, brunoStream, carlaStream} {
		s.ready(t)
	}
	if _, err := master.adjust(t, campaign, brunoPC.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(1) }); err != nil {
		t.Fatalf("AdjustCharacterVitals(Bruno) error = %v", err)
	}
	if _, err := master.adjust(t, campaign, anaPC.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(2) }); err != nil {
		t.Fatalf("AdjustCharacterVitals(Ana) error = %v", err)
	}
	changed := func(w *watcher) string { return w.nextChange(t).GetVitalsChanged().GetVitals().GetCharacterId() }
	if first, second := changed(masterStream), changed(masterStream); first != brunoPC.GetId() || second != anaPC.GetId() {
		t.Errorf("master's stream = %s, %s; want Bruno's then Ana's", first, second)
	}
	// Ana's next change is her own: Bruno's never reached her.
	if got := changed(anaStream); got != anaPC.GetId() {
		t.Errorf("Ana's stream got %s, want only her character's change", got)
	}
	if got := changed(brunoStream); got != brunoPC.GetId() {
		t.Errorf("Bruno's stream got %s, want his character's change", got)
	}
	// Carla has no character: nothing reaches her until the session ends.
	master.end(t, master.liveSession(t, campaign).GetGameSession())
	if ev := carlaStream.nextChange(t); ev.GetSessionEnded() == nil {
		t.Errorf("Carla's stream got %v, want only session_ended", ev)
	}
}

// TestPublishVitalsChangedGoesToTheMasterAndTheOwnerOnly: the vitals a level-up
// sends (PublishVitalsChanged) reach the master and the player of the character,
// and no other player.
func TestPublishVitalsChangedGoesToTheMasterAndTheOwnerOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana, bruno := h.newUser("Mestre"), h.newUser("Ana"), h.newUser("Bruno")
	campaign := h.newCampaign(master, "Mirathel", ana, bruno)
	anaPC := ana.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	brunoPC := bruno.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Lia")
	master.start(t, campaign)
	masterStream, anaStream, brunoStream := master.watch(t, campaign), ana.watch(t, campaign), bruno.watch(t, campaign)
	for _, s := range []*watcher{masterStream, anaStream, brunoStream} {
		s.ready(t)
	}

	h.svc.PublishVitalsChanged(t.Context(), campaign, anaPC.GetId())
	h.svc.PublishVitalsChanged(t.Context(), campaign, brunoPC.GetId()) // after Ana's: proves her stream did not skip a message

	changed := func(w *watcher) string { return w.nextChange(t).GetVitalsChanged().GetVitals().GetCharacterId() }
	if first, second := changed(masterStream), changed(masterStream); first != anaPC.GetId() || second != brunoPC.GetId() {
		t.Errorf("master's stream = %s, %s; want Ana's character then Bruno's", first, second)
	}
	if got := changed(anaStream); got != anaPC.GetId() {
		t.Errorf("Ana's stream got %s, want only her character's vitals", got)
	}
	if got := changed(brunoStream); got != brunoPC.GetId() {
		t.Errorf("Bruno's stream got %s, want only his character's vitals", got)
	}
	h.svc.PublishVitalsChanged(t.Context(), campaign, "not-a-character") // sends nothing, and does not panic
}

// RN-11 on the live session: nothing the live session sends, to the
// player or to the master, carries the master's notes, and neither does
// the session's history.
func TestRN11_LiveSessionNeverCarriesMasterNotes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	pc := player.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	const notes = "filho-perdido-do-lich"
	if _, err := master.characters.UpdateMasterNotes(t.Context(), connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{
		CampaignId: campaign, CharacterId: pc.GetId(), Notes: "Pensantus é o " + notes,
	})); err != nil {
		t.Fatalf("UpdateMasterNotes() error = %v", err)
	}
	session := master.start(t, campaign).GetGameSession()
	playerStream, masterStream := player.watch(t, campaign), master.watch(t, campaign)

	var seen []proto.Message
	seen = append(seen, playerStream.ready(t), masterStream.ready(t))
	seen = append(seen, player.liveSession(t, campaign), master.liveSession(t, campaign))
	adjusted, err := master.adjust(t, campaign, pc.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = proto.Int32(1) })
	if err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	seen = append(seen, adjusted, playerStream.nextChange(t), masterStream.nextChange(t))
	open, err := player.play.ListOpenGameSessions(t.Context(), connect.NewRequest(&playv1.ListOpenGameSessionsRequest{}))
	if err != nil {
		t.Fatalf("ListOpenGameSessions() error = %v", err)
	}
	seen = append(seen, open.Msg)
	master.end(t, session)
	seen = append(seen, playerStream.nextChange(t))

	for _, m := range seen {
		b, err := protojson.Marshal(m)
		if err != nil {
			t.Fatalf("protojson.Marshal() error = %v", err)
		}
		if strings.Contains(string(b), notes) {
			t.Errorf("a live session message carries the master's notes: %s", b)
		}
	}
	for _, e := range h.sessionEvents(session.GetId()) {
		b, _ := json.Marshal(e.payload)
		if strings.Contains(string(b), notes) || strings.Contains(string(b), "Pensantus") {
			t.Errorf("a session event carries text: %s", b)
		}
	}
}

// TestWatchGameSession: ready first, heartbeats, and session_ended, after
// which the stream ends without an error.
func TestWatchGameSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t, LiveConfig{Heartbeat: 50 * time.Millisecond})
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	session := master.start(t, campaign).GetGameSession()

	stream := player.watch(t, campaign)
	if got := stream.ready(t); got.GetId() != session.GetId() || got.GetEndedAt() != nil {
		t.Errorf("ready = %v, want the open session %s", got, session.GetId())
	}
	if ev := stream.next(t); ev.GetHeartbeat() == nil {
		t.Errorf("event with nothing going on = %v, want a heartbeat", ev)
	}
	ended := master.end(t, session)
	ev := stream.nextChange(t)
	if got := ev.GetSessionEnded().GetGameSession(); got.GetId() != session.GetId() || !got.GetEndedAt().AsTime().Equal(ended.GetEndedAt().AsTime()) {
		t.Errorf("event after EndGameSession = %v, want session_ended with its ended_at", ev)
	}
	if err := stream.end(t); err != nil {
		t.Errorf("stream after session_ended: error = %v, want a clean end", err)
	}
	// Ending it again sends nothing to a new session's streams.
	next := master.start(t, campaign).GetGameSession()
	stream = player.watch(t, campaign)
	stream.ready(t)
	master.end(t, session) // already ended: publishes nothing
	master.end(t, next)
	if ev := stream.nextChange(t); ev.GetSessionEnded().GetGameSession().GetId() != next.GetId() {
		t.Errorf("first change on the new session's stream = %v, want its own session_ended", ev)
	}
}

// TestWatchGameSessionRefuses: without an open session, failed_precondition
// NO_OPEN_SESSION; a pending member (RN-15) and a stranger, not_found; the
// same answers as GetLiveSession, so the session link can show the right
// page.
func TestWatchGameSessionRefuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, pending, outsider := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Pendente"), h.newUser("De fora")
	campaign := h.newCampaign(master, "Mirathel", player)
	h.joinPending(master, campaign, pending)

	err := player.watch(t, campaign).end(t)
	wantBlocked(t, "WatchGameSession() without a session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	_, err = player.play.GetLiveSession(t.Context(), connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: campaign}))
	wantBlocked(t, "GetLiveSession() without a session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)

	master.start(t, campaign)
	for name, u := range map[string]*user{"pending member": pending, "non-member": outsider} {
		wantCode(t, name+"'s WatchGameSession()", u.watch(t, campaign).end(t), connect.CodeNotFound)
		_, err := u.play.GetLiveSession(t.Context(), connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: campaign}))
		wantCode(t, name+"'s GetLiveSession()", err, connect.CodeNotFound)
	}
	wantCode(t, "WatchGameSession(made-up campaign)", player.watch(t, newKey()).end(t), connect.CodeNotFound)
	if n := h.svc.hub.Count(campaign); n != 0 {
		t.Errorf("hub subscriptions after refused streams = %d, want 0", n)
	}
}

// TestWatchGameSessionChecksAgain: every Recheck the stream reads the login
// session and the membership again; a removed member's stream ends with
// not_found, a signed-out one's with unauthenticated (ADR-0005).
func TestWatchGameSessionChecksAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t, LiveConfig{Recheck: 50 * time.Millisecond})
	master, removed, signedOutUser, staying := h.newUser("Mestre"), h.newUser("Removida"), h.newUser("Saiu"), h.newUser("Fica")
	campaign := h.newCampaign(master, "Mirathel", removed, signedOutUser, staying)
	master.start(t, campaign)
	removedStream, signedOutStream, stayingStream := removed.watch(t, campaign), signedOutUser.watch(t, campaign), staying.watch(t, campaign)
	for _, s := range []*watcher{removedStream, signedOutStream, stayingStream} {
		s.ready(t)
	}

	// There is no RPC to remove a member yet; remove the row directly.
	if _, err := h.pool.Exec(t.Context(), "DELETE FROM campaign_members WHERE campaign_id = $1 AND user_id = $2", campaign, removed.id); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	signedOut.Store(signedOutUser.id, true)

	wantCode(t, "removed member's stream", removedStream.end(t), connect.CodeNotFound)
	wantCode(t, "signed-out member's stream", signedOutStream.end(t), connect.CodeUnauthenticated)
	// The others keep watching through several rechecks: wait until the member's
	// stream has started eight more (each one starts with the session check the
	// harness counts, and the recheck before it was finished by then), watching for
	// it to end meanwhile.
	const survived = 8
	until := recheckCount(staying.id) + survived + 1
	deadline := time.After(waitLimit)
	for recheckCount(staying.id) < until {
		select {
		case err := <-stayingStream.done:
			t.Fatalf("a member's stream ended: %v", err)
		case <-deadline:
			t.Fatalf("the member's stream was rechecked %d times in %v, want %d", recheckCount(staying.id)-until+survived+1, waitLimit, survived+1)
		case <-time.After(5 * time.Millisecond):
		}
	}
	select {
	case err := <-stayingStream.done:
		t.Fatalf("a member's stream ended: %v", err)
	default:
	}
}

// TestWatchGameSessionNoticesAnEndItMissed: if a session ends without the
// stream hearing it (with more than one server, the end could happen on
// another one), the next recheck reads the database, sends session_ended
// and ends the stream.
func TestWatchGameSessionNoticesAnEndItMissed(t *testing.T) {
	t.Parallel()
	h := newHarness(t, LiveConfig{Recheck: 50 * time.Millisecond})
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	session := master.start(t, campaign).GetGameSession()
	stream := player.watch(t, campaign)
	stream.ready(t)

	// End it behind the service's back: nothing is published.
	if _, err := h.pool.Exec(t.Context(), "UPDATE game_sessions SET ended_at = now() WHERE id = $1", session.GetId()); err != nil {
		t.Fatalf("end the session directly: %v", err)
	}
	ev := stream.nextChange(t)
	if got := ev.GetSessionEnded().GetGameSession(); got.GetId() != session.GetId() || got.GetEndedAt() == nil {
		t.Errorf("event after a missed end = %v, want session_ended with its ended_at", ev)
	}
	if err := stream.end(t); err != nil {
		t.Errorf("stream after session_ended: error = %v, want a clean end", err)
	}
}

// TestWatchGameSessionEnds: after its maximum lifetime, and when the server
// shuts down (Service.Close), a stream ends without an error, promptly, and
// the app reconnects.
func TestWatchGameSessionEnds(t *testing.T) {
	t.Parallel()
	t.Run("lifetime", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, LiveConfig{MaxLifetime: 200 * time.Millisecond})
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master, "Mirathel")
		master.start(t, campaign)
		stream := master.watch(t, campaign)
		stream.ready(t)
		if err := stream.end(t); err != nil {
			t.Errorf("stream after its lifetime: error = %v, want a clean end", err)
		}
	})
	t.Run("shutdown", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		master := h.newUser("Mestre")
		campaign := h.newCampaign(master, "Mirathel")
		master.start(t, campaign)
		stream := master.watch(t, campaign)
		stream.ready(t)
		h.svc.Close()
		if err := stream.end(t); err != nil {
			t.Errorf("stream after Close: error = %v, want a clean end", err)
		}
	})
}

// TestListOpenGameSessions (RN-06): the notice lists the open sessions of
// the caller's campaigns, as master or active player, and nothing for a
// pending member or a stranger.
func TestListOpenGameSessions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, pending, outsider := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Pendente"), h.newUser("De fora")
	campaign := h.newCampaign(master, "Mirathel", player)
	quiet := h.newCampaign(master, "Sem sessão", player)
	h.joinPending(master, campaign, pending)

	list := func(u *user) []*playv1.OpenGameSession {
		t.Helper()
		res, err := u.play.ListOpenGameSessions(t.Context(), connect.NewRequest(&playv1.ListOpenGameSessionsRequest{}))
		if err != nil {
			t.Fatalf("ListOpenGameSessions() error = %v", err)
		}
		return res.Msg.GetOpenGameSessions()
	}
	if got := list(player); len(got) != 0 {
		t.Errorf("before any session: %v, want none", got)
	}
	session := master.start(t, campaign).GetGameSession()

	for name, tt := range map[string]struct {
		u    *user
		role campaignsv1.Role
	}{"master": {master, campaignsv1.Role_ROLE_MASTER}, "player": {player, campaignsv1.Role_ROLE_PLAYER}} {
		got := list(tt.u)
		if len(got) != 1 || got[0].GetGameSession().GetId() != session.GetId() || got[0].GetCampaignName() != "Mirathel" || got[0].GetMyRole() != tt.role {
			t.Errorf("%s's open sessions = %v, want Mirathel's session as %v", name, got, tt.role)
		}
	}
	for name, u := range map[string]*user{"pending member": pending, "non-member": outsider} {
		if got := list(u); len(got) != 0 {
			t.Errorf("%s's open sessions = %v, want none", name, got)
		}
	}
	other := master.start(t, quiet).GetGameSession()
	if got := list(player); len(got) != 2 || got[0].GetGameSession().GetId() != other.GetId() {
		t.Errorf("with two open sessions = %v, want both, newest first", got)
	}
	master.end(t, session)
	master.end(t, other)
	if got := list(player); len(got) != 0 {
		t.Errorf("after the sessions ended: %v, want none", got)
	}
}

// TestLiveStreamIsNotBuffered runs the stream through the API's real HTTP
// server (httpserver: request logging, cross-origin protection, h2c and
// HTTP/1.1) and checks that each event arrives right away: with a
// heartbeat far away, only a flushed write can deliver ready and
// vitals_changed in time.
func TestLiveStreamIsNotBuffered(t *testing.T) {
	t.Parallel()
	h := newHarness(t, LiveConfig{Heartbeat: time.Hour})
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	pc := player.createCharacter(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	master.start(t, campaign)

	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- h.http.Serve(ctx, ln) }()
	t.Cleanup(func() {
		stop()
		<-served
	})
	baseURL := "http://" + ln.Addr().String()

	var h2c http.Protocols
	h2c.SetUnencryptedHTTP2(true)
	transports := map[string]http.RoundTripper{
		"HTTP/1.1": &http.Transport{},
		"h2c":      &http.Transport{Protocols: &h2c},
	}
	for name, transport := range transports {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: userTransport{userID: player.id, next: transport}}
			stream := watchWith(t, playv1connect.NewPlayServiceClient(client, baseURL), campaign)
			start := time.Now()
			stream.ready(t)
			if _, err := master.adjust(t, campaign, pc.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsTemporary = proto.Int32(1) }); err != nil {
				t.Fatalf("AdjustCharacterVitals() error = %v", err)
			}
			if ev := stream.next(t); ev.GetVitalsChanged() == nil {
				t.Errorf("event = %v, want vitals_changed", ev)
			}
			if elapsed := time.Since(start); elapsed > 3*time.Second {
				t.Errorf("ready and vitals_changed took %v, want them right away", elapsed)
			}
			stream.cancel()
		})
	}
}

// TestWatchGameSessionCapsStreamsPerUser: one user may hold only so many live
// streams on a campaign; the next is refused with resource_exhausted, other
// members are not affected, and a stream that closes frees its place.
func TestWatchGameSessionCapsStreamsPerUser(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.svc.hub.SetMaxPerUser(2)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	master.start(t, campaign)

	first, second := player.watch(t, campaign), player.watch(t, campaign)
	first.ready(t)
	second.ready(t)

	wantCode(t, "a third stream of the same user", player.watch(t, campaign).end(t), connect.CodeResourceExhausted)
	master.watch(t, campaign).ready(t) // another member has their own allowance
	if n := h.svc.hub.Count(campaign); n != 3 {
		t.Errorf("hub subscriptions = %d, want 3 (the refused stream holds none)", n)
	}

	first.cancel() // a closed tab gives its place back
	deadline := time.Now().Add(waitLimit)
	for h.svc.hub.Count(campaign) != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	player.watch(t, campaign).ready(t)
}
