package play

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The pass of the Inspiration (favor_pass.go): a player asks, the master answers, and nothing moves until he accepts (RN-43).
// Toren is Caio's, Pensantus is Ana's and Brisa is Bia's.

const (
	blockedNotTheHolder  = playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_THE_HOLDER
	blockedNotAPlayer    = playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_A_PLAYER_CHARACTER
	blockedSelf          = playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_SELF
	blockedPassPending   = playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_PASS_PENDING
	blockedPassClosed    = playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_PASS_CLOSED
	blockedNoSession     = playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NO_SESSION
	blockedCannotReceive = playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_CANNOT_RECEIVE
)

// ownerOf is the player of the party's character.
func (f *favorParty) ownerOf(id string) *user {
	switch id {
	case f.toren.GetId():
		return f.caio
	case f.pens.GetId():
		return f.ana
	case f.bri.GetId():
		return f.bia
	}
	f.h.t.Fatalf("ownerOf(%s): not a character of the party", id)
	return nil
}

// askAs sends PassFavor as u.
func (f *favorParty) askAs(u *user, from, to, key string) (*playv1.FavorPass, error) {
	res, err := u.favor.PassFavor(f.h.t.Context(), connect.NewRequest(&playv1.PassFavorRequest{CampaignId: f.campaignID, FromCharacterId: from, ToCharacterId: to, IdempotencyKey: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetPass(), nil
}

// askPass is the PassFavor of the giver's own player, with a new key.
func (f *favorParty) askPass(from, to string) (*playv1.FavorPass, error) {
	return f.askAs(f.ownerOf(from), from, to, newKey())
}

// mustAsk is askPass that must succeed.
func (f *favorParty) mustAsk(t *testing.T, from, to string) *playv1.FavorPass {
	t.Helper()
	p, err := f.askPass(from, to)
	if err != nil {
		t.Fatalf("PassFavor(%s -> %s) error = %v", from, to, err)
	}
	return p
}

// answerAs sends AnswerFavorPass as u.
func (f *favorParty) answerAs(u *user, id string, accept bool, key string) (*playv1.FavorPass, error) {
	res, err := u.favor.AnswerFavorPass(f.h.t.Context(), connect.NewRequest(&playv1.AnswerFavorPassRequest{CampaignId: f.campaignID, PassId: id, Accept: accept, IdempotencyKey: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetPass(), nil
}

// answer is the master's answer with a new key.
func (f *favorParty) answer(id string, accept bool) (*playv1.FavorPass, error) {
	return f.answerAs(f.master, id, accept, newKey())
}

// cancelAs sends CancelFavorPass as u.
func (f *favorParty) cancelAs(u *user, id, key string) (*playv1.FavorPass, error) {
	res, err := u.favor.CancelFavorPass(f.h.t.Context(), connect.NewRequest(&playv1.CancelFavorPassRequest{CampaignId: f.campaignID, PassId: id, IdempotencyKey: key}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetPass(), nil
}

// passesOf is what u reads of GetFavor's passes.
func (f *favorParty) passesOf(t *testing.T, u *user) []*playv1.FavorPass {
	t.Helper()
	res, err := u.favor.GetFavor(t.Context(), connect.NewRequest(&playv1.GetFavorRequest{CampaignId: f.campaignID}))
	if err != nil {
		t.Fatalf("GetFavor() error = %v", err)
	}
	return res.Msg.GetPasses()
}

// has says the character holds the Inspiration.
func (f *favorParty) has(t *testing.T, id string) bool {
	t.Helper()
	var n int
	if err := f.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM character_favor WHERE character_id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("read the row: %v", err)
	}
	return n == 1
}

// eventsOf counts the session's events of a kind.
func (f *favorParty) eventsOf(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := f.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1 AND game_session_id = $2`, kind, f.sessionID).Scan(&n); err != nil {
		t.Fatalf("count the events: %v", err)
	}
	return n
}

// passRows counts the rows of favor_passes, and how many are pending.
func (f *favorParty) passRows(t *testing.T) (all, pending int) {
	t.Helper()
	if err := f.h.pool.QueryRow(t.Context(), `SELECT count(*), count(*) FILTER (WHERE status = 'pending') FROM favor_passes WHERE campaign_id = $1`, f.campaignID).Scan(&all, &pending); err != nil {
		t.Fatalf("count the passes: %v", err)
	}
	return all, pending
}

// passStatus is the stored status of a pass.
func (f *favorParty) passStatus(t *testing.T, id string) string {
	t.Helper()
	var s string
	if err := f.h.pool.QueryRow(t.Context(), `SELECT status FROM favor_passes WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatalf("read the pass: %v", err)
	}
	return s
}

// payloadOf is the payload of the session's only event of the kind.
func (f *favorParty) payloadOf(t *testing.T, kind string) []byte {
	t.Helper()
	var payload []byte
	if err := f.h.pool.QueryRow(t.Context(), `SELECT payload FROM session_events WHERE kind = $1 AND game_session_id = $2`, kind, f.sessionID).Scan(&payload); err != nil {
		t.Fatalf("read the %s event: %v", kind, err)
	}
	return payload
}

// passWorld is the state a refusal must leave as it was.
type passWorld struct {
	seq                    int64
	rows                   [3]bool
	passes, pending        int
	asked, passed, refused int
	canceled               int
}

func (f *favorParty) world(t *testing.T) passWorld {
	t.Helper()
	w := passWorld{seq: f.seq(t), rows: [3]bool{f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()), f.has(t, f.bri.GetId())}}
	w.passes, w.pending = f.passRows(t)
	w.asked, w.passed = f.eventsOf(t, eventFavorPassAsked), f.eventsOf(t, eventFavorPassed)
	w.refused, w.canceled = f.eventsOf(t, eventFavorAskRefused), f.eventsOf(t, eventFavorPassCanceled)
	return w
}

// wantSame checks that nothing moved since w: a refusal and a replay write nothing.
func (f *favorParty) wantSame(t *testing.T, what string, w passWorld) {
	t.Helper()
	if got := f.world(t); got != w {
		t.Errorf("after %s the world is %+v, want it unchanged: %+v", what, got, w)
	}
}

// heardBy reads what the watchers heard since the last marker, per name.
func (f *favorParty) heardBy(t *testing.T, ws map[string]*watcher) map[string][]*playv1.WatchGameSessionResponse {
	t.Helper()
	list := make([]*watcher, 0, len(ws))
	for _, w := range ws {
		list = append(list, w)
	}
	f.master.markCurrentMap(t, f.campaignID, f.mapID, list...)
	out := map[string][]*playv1.WatchGameSessionResponse{}
	for name, w := range ws {
		out[name] = w.beforeMarker(t)
	}
	return out
}

// passWatchers are the streams of the master and the three players, ready.
func (f *favorParty) passWatchers(t *testing.T) map[string]*watcher {
	t.Helper()
	ws := map[string]*watcher{"master": f.master.watch(t, f.campaignID), "caio": f.caio.watch(t, f.campaignID), "ana": f.ana.watch(t, f.campaignID), "bia": f.bia.watch(t, f.campaignID)}
	for _, w := range ws {
		w.ready(t)
	}
	return ws
}

// wantHints checks that each named reader heard exactly one vitals hint, for the character and with the number, and the others nothing.
func wantHints(t *testing.T, heard map[string][]*playv1.WatchGameSessionResponse, seq int64, who map[string][]string) {
	t.Helper()
	for name, got := range heard {
		var ids []string
		for _, ev := range got {
			ids = append(ids, ev.GetVitalsChanged().GetVitals().GetCharacterId())
			if ev.GetSessionSeq() != seq {
				t.Errorf("%s heard a hint on number %d, want %d", name, ev.GetSessionSeq(), seq)
			}
		}
		slices.Sort(ids)
		want := slices.Clone(who[name])
		slices.Sort(want)
		if !slices.Equal(ids, want) {
			t.Errorf("%s heard hints for %v, want %v", name, ids, want)
		}
	}
}

func TestPassFavorAsksAndMovesNothing(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	ws := f.passWatchers(t)
	before := f.seq(t)

	p, err := f.askPass(f.toren.GetId(), f.pens.GetId())
	if err != nil {
		t.Fatalf("PassFavor() error = %v", err)
	}
	if p.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_PENDING || p.GetFromCharacterId() != f.toren.GetId() || p.GetToCharacterId() != f.pens.GetId() ||
		p.GetFromName() != "Toren" || p.GetToName() != "Pensantus" || p.GetId() == "" || p.GetReceiverHas() {
		t.Errorf("PassFavor() = %v, want Toren's pending ask for Pensantus, receiver_has false", p)
	}
	if !f.has(t, f.toren.GetId()) || f.has(t, f.pens.GetId()) {
		t.Errorf("rows after the ask: Toren %v, Pensantus %v, want it still with Toren", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
	}
	if got := f.seq(t); got != before+1 {
		t.Errorf("hint_seq = %d after the ask, want %d (one step)", got, before+1)
	}
	var got favorPassEvent
	payload := f.payloadOf(t, eventFavorPassAsked)
	if err := json.Unmarshal(payload, &got); err != nil || got != (favorPassEvent{PassID: p.GetId(), FromCharacterID: f.toren.GetId(), ToCharacterID: f.pens.GetId()}) {
		t.Errorf("favor_pass_asked payload = %s (%v), want the ids only", payload, err)
	}
	var keys map[string]any
	if err := json.Unmarshal(payload, &keys); err != nil || len(keys) != 3 {
		t.Errorf("favor_pass_asked payload = %s, want exactly pass_id, from_character_id and to_character_id", payload)
	}
	wantHints(t, f.heardBy(t, ws), before+1, map[string][]string{"master": {f.toren.GetId()}, "caio": {f.toren.GetId()}})
}

func TestPassFavorRefusalsAreInOrderAndChangeNothing(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	other := f.h.newUser("Outra mestra")
	elsewhere := f.h.newCampaign(other, "Outra", f.ana)
	foreign := f.ana.createCharacter(t, elsewhere, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "De outra mesa")
	reserved := f.reservedCharacter(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	w := f.world(t)
	toren, pens := f.toren.GetId(), f.pens.GetId()
	for _, c := range []struct {
		name     string
		by       *user
		from, to string
		want     playv1.FavorBlockedReason
	}{
		{"the giver as the receiver", f.caio, toren, toren, blockedSelf},
		{"a giver that is not the caller's", f.caio, pens, toren, blockedNotTheHolder},
		{"the master as the giver's owner", f.master, toren, pens, blockedNotTheHolder},
		{"a giver that has none", f.ana, pens, toren, blockedNotTheHolder},
		{"an NPC as the receiver", f.caio, toren, f.goblin.GetId(), blockedNotAPlayer},
		{"a reserved character as the receiver", f.caio, toren, reserved.GetId(), blockedNotAPlayer},
		{"a character of another game as the receiver", f.caio, toren, foreign.GetId(), blockedNotAPlayer},
		{"an id nobody has as the receiver", f.caio, toren, newKey(), blockedNotAPlayer},
		{"the giver as the receiver, with an NPC giver that is not the caller's", f.caio, f.goblin.GetId(), f.goblin.GetId(), blockedSelf},
	} {
		_, err := f.askAs(c.by, c.from, c.to, newKey())
		wantFavorBlocked(t, c.name, err, c.want)
	}
	f.wantSame(t, "the refusals", w)

	// The one answer for every id that is not a player character: it does not tell them apart.
	var texts []string
	for _, id := range []string{f.goblin.GetId(), newKey(), reserved.GetId()} {
		_, err := f.askAs(f.caio, toren, id, newKey())
		texts = append(texts, err.Error())
	}
	if texts[0] != texts[1] || texts[1] != texts[2] {
		t.Errorf("the refusals for an NPC, a stranger's id and a reserved character say %q: they must not tell them apart", texts)
	}

	// With the giver's pass pending the next ask is PASS_PENDING, whoever the receiver is, and after the cheaper checks.
	first := f.mustAsk(t, toren, pens)
	w = f.world(t)
	_, err := f.askAs(f.caio, toren, f.bri.GetId(), newKey())
	wantFavorBlocked(t, "a second ask", err, blockedPassPending)
	_, err = f.askAs(f.caio, toren, toren, newKey())
	wantFavorBlocked(t, "SELF before PASS_PENDING", err, blockedSelf)
	_, err = f.askAs(f.caio, toren, f.goblin.GetId(), newKey())
	wantFavorBlocked(t, "NOT_A_PLAYER_CHARACTER before PASS_PENDING", err, blockedNotAPlayer)
	f.wantSame(t, "the second asks", w)
	if first.GetId() == "" {
		t.Fatal("the first ask has no id")
	}
}

func TestPassFavorNeedsAnOpenSession(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	f.master.end(t, f.master.liveSession(t, f.campaignID).GetGameSession())
	w := f.world(t)
	_, err := f.askPass(f.toren.GetId(), f.pens.GetId())
	wantFavorBlocked(t, "PassFavor with no session", err, blockedNoSession)
	// NO_SESSION reads the campaign, not the character: the shape checks still come first.
	_, err = f.askAs(f.caio, f.toren.GetId(), f.toren.GetId(), newKey())
	wantFavorBlocked(t, "SELF with no session", err, blockedSelf)
	if all, _ := f.passRows(t); all != 0 || !f.has(t, f.toren.GetId()) {
		t.Errorf("%d passes and Toren has it = %v, want none and true", all, f.has(t, f.toren.GetId()))
	}
	if got := f.world(t); got.passes != w.passes || got.seq != w.seq {
		t.Errorf("the world after the refusal = %+v, want %+v", got, w)
	}
}

func TestPassFavorInputsAndRoles(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	outsider := f.h.newUser("Fora")
	pending := f.h.newUser("Pendente")
	f.h.joinPending(f.master, f.campaignID, pending)
	for _, c := range []struct {
		name     string
		by       *user
		from, to string
		key      string
		code     connect.Code
	}{
		{"someone outside the campaign", outsider, f.toren.GetId(), f.pens.GetId(), newKey(), connect.CodeNotFound},
		{"a pending member", pending, f.toren.GetId(), f.pens.GetId(), newKey(), connect.CodeNotFound},
		{"a key that is not a UUID", f.caio, f.toren.GetId(), f.pens.GetId(), "nope", connect.CodeInvalidArgument},
		{"a giver id that is not a UUID", f.caio, "toren", f.pens.GetId(), newKey(), connect.CodeInvalidArgument},
		{"a receiver id that is not a UUID", f.caio, f.toren.GetId(), "pens", newKey(), connect.CodeInvalidArgument},
	} {
		_, err := f.askAs(c.by, c.from, c.to, c.key)
		wantCode(t, c.name, err, c.code)
	}
	if all, _ := f.passRows(t); all != 0 {
		t.Errorf("%d passes after refused calls, want none", all)
	}
}

// The receiver's state is told to the master alone, and only when he accepts: the ask is made the same, and a player reads
// receiver_has false.
func TestPassFavorDoesNotTellTheReceiversState(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	for _, id := range []string{f.toren.GetId(), f.pens.GetId()} {
		if err := f.grant(id); err != nil {
			t.Fatalf("GrantFavor() error = %v", err)
		}
	}
	p, err := f.askPass(f.toren.GetId(), f.pens.GetId())
	if err != nil || p.GetReceiverHas() {
		t.Fatalf("PassFavor() to a receiver that has one = %v, %v, want the ask made with receiver_has false", p, err)
	}
	for who, u := range map[string]*user{"Caio (the giver)": f.caio, "Ana (the receiver)": f.ana, "Bia": f.bia} {
		for _, read := range f.passesOf(t, u) {
			if read.GetReceiverHas() {
				t.Errorf("%s reads receiver_has true: %v", who, read)
			}
		}
	}
	reads := f.passesOf(t, f.master)
	if len(reads) != 1 || !reads[0].GetReceiverHas() {
		t.Errorf("the master reads %v, want the pass with receiver_has true", reads)
	}
}

func TestPassFavorRetries(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	ws := f.passWatchers(t)
	key := newKey()
	first, err := f.askAs(f.caio, f.toren.GetId(), f.pens.GetId(), key)
	if err != nil {
		t.Fatalf("PassFavor() error = %v", err)
	}
	w := f.world(t)
	again, err := f.askAs(f.caio, f.toren.GetId(), f.pens.GetId(), key)
	if err != nil || again.GetId() != first.GetId() || again.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_PENDING {
		t.Errorf("the same request again = %v, %v, want the same pending pass %s", again, err, first.GetId())
	}
	f.wantSame(t, "the retry", w)
	// The retry tells again what the first call may have died before telling, on the number as it stands.
	asked := w.seq
	heard := f.heardBy(t, ws)
	w = f.world(t) // the marker is a write too: it moved the number
	if len(heard["master"]) != 2 || heard["master"][1].GetSessionSeq() != asked || len(heard["caio"]) != 2 || len(heard["ana"]) != 0 || len(heard["bia"]) != 0 {
		t.Errorf("heard %v, want the ask's hint and its replay for the master and Caio, on number %d, and nothing for the others", heard, asked)
	}
	_, err = f.askAs(f.caio, f.toren.GetId(), f.bri.GetId(), key)
	wantCode(t, "the key for another receiver", err, connect.CodeInvalidArgument)
	f.wantSame(t, "the key reused", w)
}

// Two asks of one giver at once: exactly one pending pass, the other is PASS_PENDING.
func TestPassFavorRacesOnTheSameGiver(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8)
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	before := f.seq(t)
	const racers = 12
	errs := make([]error, racers)
	var start, wg sync.WaitGroup
	start.Add(1)
	for i := range racers {
		to := f.pens.GetId()
		if i%2 == 1 {
			to = f.bri.GetId()
		}
		wg.Go(func() {
			start.Wait()
			_, errs[i] = f.askPass(f.toren.GetId(), to)
		})
	}
	start.Done()
	wg.Wait()
	made := 0
	for _, err := range errs {
		if err == nil {
			made++
			continue
		}
		wantFavorBlocked(t, "a racing ask", err, blockedPassPending)
	}
	if all, pending := f.passRows(t); made != 1 || all != 1 || pending != 1 {
		t.Errorf("%d asks succeeded and %d rows (%d pending), want exactly 1 of each", made, all, pending)
	}
	if got := f.seq(t); got != before+1 {
		t.Errorf("hint_seq = %d, want %d: one step for the one ask", got, before+1)
	}
	if got := f.eventsOf(t, eventFavorPassAsked); got != 1 {
		t.Errorf("%d favor_pass_asked events, want 1", got)
	}
}

func TestAnswerFavorPassAcceptMovesTheInspiration(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	ws := f.passWatchers(t)
	before := f.seq(t)

	got, err := f.answer(p.GetId(), true)
	if err != nil {
		t.Fatalf("AnswerFavorPass(accept) error = %v", err)
	}
	if got.GetId() != p.GetId() || got.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_ACCEPTED {
		t.Errorf("AnswerFavorPass() = %v, want the pass accepted", got)
	}
	if f.has(t, f.toren.GetId()) || !f.has(t, f.pens.GetId()) {
		t.Errorf("rows: Toren %v, Pensantus %v, want it moved to Pensantus", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
	}
	if s := f.passStatus(t, p.GetId()); s != "accepted" {
		t.Errorf("stored status = %q, want accepted", s)
	}
	if now := f.seq(t); now != before+1 {
		t.Errorf("hint_seq = %d, want %d (one step)", now, before+1)
	}
	var ev favorPassedEvent
	payload := f.payloadOf(t, eventFavorPassed)
	if err := json.Unmarshal(payload, &ev); err != nil || ev != (favorPassedEvent{PassID: p.GetId(), FromCharacterID: f.toren.GetId(), ToCharacterID: f.pens.GetId()}) {
		t.Errorf("favor_passed payload = %s (%v), want the ids only", payload, err)
	}
	// The master, the giver's owner and the receiver's owner hear; a third player hears nothing.
	wantHints(t, f.heardBy(t, ws), before+1, map[string][]string{
		"master": {f.toren.GetId(), f.pens.GetId()}, "caio": {f.toren.GetId()}, "ana": {f.pens.GetId()},
	})
	if holders := f.holders(t, f.master); len(holders) != 1 || holders[0].GetCharacterId() != f.pens.GetId() {
		t.Errorf("GetFavor() for the master = %v, want Pensantus", holders)
	}
	if reads := f.passesOf(t, f.master); len(reads) != 0 {
		t.Errorf("the master still reads the answered pass: %v", reads)
	}
}

// The master's Aceitar is refused when the receiver already has one: to the master alone, and nothing changes, the pass stays pending.
func TestAnswerFavorPassCannotReceive(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	for _, id := range []string{f.toren.GetId(), f.pens.GetId()} {
		if err := f.grant(id); err != nil {
			t.Fatalf("GrantFavor() error = %v", err)
		}
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	ws := f.passWatchers(t)
	w := f.world(t)
	_, err := f.answer(p.GetId(), true)
	wantFavorBlocked(t, "Aceitar with a receiver that has one", err, blockedCannotReceive)
	f.wantSame(t, "the refused accept", w)
	if s := f.passStatus(t, p.GetId()); s != "pending" {
		t.Errorf("stored status = %q, want the pass still pending", s)
	}
	for name, heard := range f.heardBy(t, ws) {
		if len(heard) != 0 {
			t.Errorf("%s heard %v of a refused accept, want nothing", name, heard)
		}
	}
	// Recusar still closes it, and the giver reads the one sentence's state: refused, nothing about the receiver.
	if got, err := f.answer(p.GetId(), false); err != nil || got.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_REFUSED {
		t.Errorf("Recusar after the refused accept = %v, %v, want REFUSED", got, err)
	}
}

func TestAnswerFavorPassRefuseKeepsItWithTheGiver(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	ws := f.passWatchers(t)
	before := f.seq(t)
	got, err := f.answer(p.GetId(), false)
	if err != nil || got.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_REFUSED {
		t.Fatalf("AnswerFavorPass(refuse) = %v, %v, want REFUSED", got, err)
	}
	if !f.has(t, f.toren.GetId()) || f.has(t, f.pens.GetId()) {
		t.Errorf("rows: Toren %v, Pensantus %v, want it still with Toren", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
	}
	if now := f.seq(t); now != before+1 {
		t.Errorf("hint_seq = %d, want %d", now, before+1)
	}
	if f.eventsOf(t, eventFavorAskRefused) != 1 || f.eventsOf(t, eventFavorPassed) != 0 {
		t.Errorf("events: %d refused, %d passed, want 1 and 0", f.eventsOf(t, eventFavorAskRefused), f.eventsOf(t, eventFavorPassed))
	}
	// The receiver's owner and the third player read nothing of a refusal.
	wantHints(t, f.heardBy(t, ws), before+1, map[string][]string{"master": {f.toren.GetId()}, "caio": {f.toren.GetId()}})
	// The giver may ask again once it is answered.
	if _, err := f.askPass(f.toren.GetId(), f.bri.GetId()); err != nil {
		t.Errorf("a new ask after the refusal error = %v", err)
	}
}

func TestAnswerFavorPassRefusals(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	outsider := f.h.newUser("Fora")
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	w := f.world(t)
	for _, c := range []struct {
		name string
		by   *user
		id   string
		key  string
		code connect.Code
	}{
		{"the giver", f.caio, p.GetId(), newKey(), connect.CodePermissionDenied},
		{"the receiver", f.ana, p.GetId(), newKey(), connect.CodePermissionDenied},
		{"someone outside", outsider, p.GetId(), newKey(), connect.CodeNotFound},
		{"a pass nobody asked", f.master, newKey(), newKey(), connect.CodeNotFound},
		{"an id that is not a UUID", f.master, "pass", newKey(), connect.CodeInvalidArgument},
		{"a key that is not a UUID", f.master, p.GetId(), "nope", connect.CodeInvalidArgument},
	} {
		_, err := f.answerAs(c.by, c.id, true, c.key)
		wantCode(t, c.name, err, c.code)
	}
	f.wantSame(t, "the refused answers", w)
	// A pass of another game is not found either.
	other := f.h.newUser("Outra mestra")
	elsewhere := f.h.newCampaign(other, "Outra", f.ana)
	_, err := other.favor.AnswerFavorPass(t.Context(), connect.NewRequest(&playv1.AnswerFavorPassRequest{CampaignId: elsewhere, PassId: p.GetId(), Accept: true, IdempotencyKey: newKey()}))
	wantCode(t, "a pass of another campaign", err, connect.CodeNotFound)
	f.wantSame(t, "the foreign answer", w)
}

func TestAnswerFavorPassOfAClosedPass(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	if _, err := f.answer(p.GetId(), true); err != nil {
		t.Fatalf("AnswerFavorPass() error = %v", err)
	}
	w := f.world(t)
	for _, accept := range []bool{true, false} {
		_, err := f.answer(p.GetId(), accept)
		wantFavorBlocked(t, "a second answer", err, blockedPassClosed)
	}
	_, err := f.cancelAs(f.caio, p.GetId(), newKey())
	wantFavorBlocked(t, "a cancel of an answered pass", err, blockedPassClosed)
	f.wantSame(t, "the answers of a closed pass", w)
}

func TestAnswerFavorPassRetries(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	for _, id := range []string{f.toren.GetId(), f.bri.GetId()} {
		if err := f.grant(id); err != nil {
			t.Fatalf("GrantFavor() error = %v", err)
		}
	}
	accepted := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	refused := f.mustAsk(t, f.bri.GetId(), f.pens.GetId())
	ws := f.passWatchers(t)
	acceptKey, refuseKey := newKey(), newKey()
	if _, err := f.answerAs(f.master, accepted.GetId(), true, acceptKey); err != nil {
		t.Fatalf("accept error = %v", err)
	}
	if _, err := f.answerAs(f.master, refused.GetId(), false, refuseKey); err != nil {
		t.Fatalf("refuse error = %v", err)
	}
	_ = f.heardBy(t, ws)
	w := f.world(t)
	for _, c := range []struct {
		name   string
		id     string
		accept bool
		key    string
		status playv1.FavorPassStatus
	}{
		{"the accept", accepted.GetId(), true, acceptKey, playv1.FavorPassStatus_FAVOR_PASS_STATUS_ACCEPTED},
		{"the refusal", refused.GetId(), false, refuseKey, playv1.FavorPassStatus_FAVOR_PASS_STATUS_REFUSED},
	} {
		got, err := f.answerAs(f.master, c.id, c.accept, c.key)
		if err != nil || got.GetId() != c.id || got.GetStatus() != c.status {
			t.Errorf("a retry of %s = %v, %v, want the stored outcome %v", c.name, got, err, c.status)
		}
	}
	f.wantSame(t, "the retries", w)
	// A key is for one answer.
	_, err := f.answerAs(f.master, accepted.GetId(), false, acceptKey)
	wantCode(t, "the accept's key for a refusal", err, connect.CodeInvalidArgument)
	_, err = f.answerAs(f.master, refused.GetId(), true, acceptKey)
	wantCode(t, "the accept's key for another pass", err, connect.CodeInvalidArgument)
	_, err = f.cancelAs(f.caio, accepted.GetId(), acceptKey)
	wantCode(t, "the accept's key for a cancel", err, connect.CodeInvalidArgument)
	f.wantSame(t, "the keys reused", w)
	// The retry tells again what the first call may have died before telling: the receiver's owner included, on the number as it stands.
	// Brisa's owner is told of the refusal of her own ask and of nothing that Toren and Pensantus did.
	heard := f.heardBy(t, ws)
	hintsFor := func(name string) []string {
		var ids []string
		for _, ev := range heard[name] {
			ids = append(ids, ev.GetVitalsChanged().GetVitals().GetCharacterId())
		}
		return ids
	}
	if got := hintsFor("ana"); len(got) == 0 || slices.ContainsFunc(got, func(id string) bool { return id != f.pens.GetId() }) {
		t.Errorf("Ana (the receiver) heard hints for %v, want Pensantus only", got)
	}
	if got := hintsFor("bia"); slices.ContainsFunc(got, func(id string) bool { return id != f.bri.GetId() }) {
		t.Errorf("Bia heard hints for %v, want Brisa's alone (her own ask)", got)
	}
	if got := hintsFor("caio"); len(got) == 0 || slices.ContainsFunc(got, func(id string) bool { return id != f.toren.GetId() }) {
		t.Errorf("Caio (the giver) heard hints for %v, want Toren only", got)
	}
}

func TestCancelFavorPass(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	ws := f.passWatchers(t)
	w := f.world(t)
	for _, c := range []struct {
		name string
		by   *user
	}{{"the receiver", f.ana}, {"a third player", f.bia}, {"the master", f.master}} {
		_, err := f.cancelAs(c.by, p.GetId(), newKey())
		wantCode(t, "a cancel by "+c.name, err, connect.CodeNotFound)
	}
	_, err := f.cancelAs(f.caio, newKey(), newKey())
	wantCode(t, "a cancel of a pass nobody asked", err, connect.CodeNotFound)
	_, err = f.cancelAs(f.caio, "pass", newKey())
	wantCode(t, "a cancel of an id that is not a UUID", err, connect.CodeInvalidArgument)
	f.wantSame(t, "the refused cancels", w)

	key := newKey()
	got, err := f.cancelAs(f.caio, p.GetId(), key)
	if err != nil || got.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_CANCELED {
		t.Fatalf("CancelFavorPass() = %v, %v, want CANCELED", got, err)
	}
	if !f.has(t, f.toren.GetId()) || f.has(t, f.pens.GetId()) {
		t.Errorf("rows: Toren %v, Pensantus %v, want it with Toren", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
	}
	if now := f.seq(t); now != w.seq+1 {
		t.Errorf("hint_seq = %d, want %d", now, w.seq+1)
	}
	if f.eventsOf(t, eventFavorPassCanceled) != 1 {
		t.Errorf("%d favor_pass_canceled events, want 1", f.eventsOf(t, eventFavorPassCanceled))
	}
	wantHints(t, f.heardBy(t, ws), w.seq+1, map[string][]string{"master": {f.toren.GetId()}, "caio": {f.toren.GetId()}})
	after := f.world(t)
	again, err := f.cancelAs(f.caio, p.GetId(), key)
	if err != nil || again.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_CANCELED {
		t.Errorf("a retry of the cancel = %v, %v, want the same outcome", again, err)
	}
	f.wantSame(t, "the retry of the cancel", after)
	_, err = f.cancelAs(f.caio, p.GetId(), newKey())
	wantFavorBlocked(t, "a second cancel", err, blockedPassClosed)
	_, err = f.answer(p.GetId(), true)
	wantFavorBlocked(t, "an accept of a canceled pass", err, blockedPassClosed)
	f.wantSame(t, "the later calls", after)
}

// Of an accept and a cancel at once exactly one wins, and the world agrees with the winner.
func TestAcceptAndCancelRace(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8)
	f := newFavorParty(t)
	for range 4 {
		if err := f.grant(f.toren.GetId()); err != nil && !isAlreadyHas(err) {
			t.Fatalf("GrantFavor() error = %v", err)
		}
		p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
		before := f.seq(t)
		var accErr, canErr error
		var start, wg sync.WaitGroup
		start.Add(1)
		wg.Go(func() { start.Wait(); _, accErr = f.answer(p.GetId(), true) })
		wg.Go(func() { start.Wait(); _, canErr = f.cancelAs(f.caio, p.GetId(), newKey()) })
		start.Done()
		wg.Wait()
		if (accErr == nil) == (canErr == nil) {
			t.Fatalf("accept error = %v and cancel error = %v, want exactly one to win", accErr, canErr)
		}
		if accErr != nil {
			wantFavorBlocked(t, "the accept that lost", accErr, blockedPassClosed)
		}
		if canErr != nil {
			wantFavorBlocked(t, "the cancel that lost", canErr, blockedPassClosed)
		}
		if got := f.seq(t); got != before+1 {
			t.Errorf("hint_seq = %d, want %d: one step for the one write", got, before+1)
		}
		moved := accErr == nil
		if f.has(t, f.toren.GetId()) == moved || f.has(t, f.pens.GetId()) != moved {
			t.Errorf("accept won = %v but Toren has it = %v and Pensantus = %v", moved, f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
		}
		// Back to the start for the next round: Pensantus gives it back through the master.
		if moved {
			f.execSQL(t, `UPDATE character_favor SET character_id = $1 WHERE character_id = $2`, f.toren.GetId(), f.pens.GetId())
		}
	}
}

func isAlreadyHas(err error) bool {
	return isBlockedBy(err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ALREADY_HAS)
}

// blockedReasonOf is the FavorBlocked reason of a failed_precondition.
func blockedReasonOf(err error) (playv1.FavorBlockedReason, bool) {
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Code() != connect.CodeFailedPrecondition {
		return 0, false
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, isBlocked := v.(*playv1.FavorBlocked); isBlocked {
				return b.GetReason(), true
			}
		}
	}
	return 0, false
}

// Two accepts at once of one pass: one wins, the other is told it is closed, the Inspiration moves once.
func TestTwoAcceptsRace(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8)
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	before := f.seq(t)
	const racers = 10
	errs := make([]error, racers)
	var start, wg sync.WaitGroup
	start.Add(1)
	for i := range racers {
		wg.Go(func() { start.Wait(); _, errs[i] = f.answer(p.GetId(), true) })
	}
	start.Done()
	wg.Wait()
	won := 0
	for _, err := range errs {
		if err == nil {
			won++
			continue
		}
		wantFavorBlocked(t, "an accept that lost", err, blockedPassClosed)
	}
	if won != 1 {
		t.Errorf("%d accepts won, want exactly 1", won)
	}
	if f.has(t, f.toren.GetId()) || !f.has(t, f.pens.GetId()) {
		t.Errorf("Toren has it = %v, Pensantus = %v, want it moved once", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
	}
	if got, ev := f.seq(t), f.eventsOf(t, eventFavorPassed); got != before+1 || ev != 1 {
		t.Errorf("hint_seq = %d (want %d) and %d favor_passed events (want 1)", got, before+1, ev)
	}
}

// The one-row rule: an accept racing a grant to the receiver. Either the grant came first (the accept is CANNOT_RECEIVE and nothing
// moved, the pass stays pending) or the accept did (the grant is ALREADY_HAS): the receiver never ends with two, the giver never
// loses it for nothing.
func TestAcceptAndAGrantToTheReceiverRace(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8)
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	var accErr, grantErr error
	var start, wg sync.WaitGroup
	start.Add(1)
	wg.Go(func() { start.Wait(); _, accErr = f.answer(p.GetId(), true) })
	wg.Go(func() { start.Wait(); grantErr = f.grant(f.pens.GetId()) })
	start.Done()
	wg.Wait()
	if !f.has(t, f.pens.GetId()) {
		t.Fatalf("Pensantus has none after the accept (%v) and the grant (%v)", accErr, grantErr)
	}
	if accErr == nil {
		wantFavorBlocked(t, "the grant after the accept", grantErr, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ALREADY_HAS)
		if f.has(t, f.toren.GetId()) {
			t.Error("the accept won and Toren still has it")
		}
		return
	}
	if grantErr != nil {
		t.Fatalf("the grant error = %v with the accept refused (%v)", grantErr, accErr)
	}
	wantFavorBlocked(t, "the accept after the grant", accErr, blockedCannotReceive)
	if !f.has(t, f.toren.GetId()) || f.passStatus(t, p.GetId()) != "pending" {
		t.Errorf("the accept was refused but Toren has it = %v and the pass is %s", f.has(t, f.toren.GetId()), f.passStatus(t, p.GetId()))
	}
}

func TestGetFavorShowsEachReaderTheirPasses(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	for _, id := range []string{f.toren.GetId(), f.pens.GetId(), f.bri.GetId()} {
		if err := f.grant(id); err != nil {
			t.Fatalf("GrantFavor() error = %v", err)
		}
	}
	// Toren asks to pass to Pensantus (who has one) and Brisa asks to pass to a receiver that has none: take Pensantus's away first.
	f.execSQL(t, `DELETE FROM character_favor WHERE character_id = $1`, f.pens.GetId())
	toPens := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	toToren := f.mustAsk(t, f.bri.GetId(), f.toren.GetId())
	if err := f.grant(f.pens.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	for who, c := range map[string]struct {
		u    *user
		want map[string]bool // pass id -> receiver_has
	}{
		"the master": {f.master, map[string]bool{toPens.GetId(): true, toToren.GetId(): true}},
		"Caio":       {f.caio, map[string]bool{toPens.GetId(): false}},
		"Bia":        {f.bia, map[string]bool{toToren.GetId(): false}},
		"Ana":        {f.ana, map[string]bool{}},
	} {
		got := map[string]bool{}
		for _, p := range f.passesOf(t, c.u) {
			got[p.GetId()] = p.GetReceiverHas()
			if p.GetStatus() != playv1.FavorPassStatus_FAVOR_PASS_STATUS_PENDING || p.GetFromName() == "" || p.GetToName() == "" {
				t.Errorf("%s reads %v, want a pending pass with both names", who, p)
			}
		}
		if len(got) != len(c.want) {
			t.Errorf("%s reads %v, want %v", who, got, c.want)
			continue
		}
		for id, has := range c.want {
			if v, ok := got[id]; !ok || v != has {
				t.Errorf("%s reads %v, want %v", who, got, c.want)
			}
		}
	}
	// Only pending ones are read.
	if _, err := f.answer(toPens.GetId(), false); err != nil {
		t.Fatalf("Recusar error = %v", err)
	}
	if got := f.passesOf(t, f.master); len(got) != 1 || got[0].GetId() != toToren.GetId() {
		t.Errorf("the master reads %v after the refusal, want only Brisa's ask", got)
	}
}

func TestSpendIsRefusedWhileThePassIsPending(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	w := f.world(t)
	if got := f.tryFavor(t.Context(), f.ask(combat.ModeNormal)); !isBlockedBy(got.err, blockedPassPending) {
		t.Errorf("a spend with the pass pending = %+v, want PASS_PENDING", got)
	}
	// PASS_PENDING comes after the caller's refusals and before the judgement of the roll.
	notOwn := f.ask(combat.ModeNormal)
	notOwn.Own = false
	if got := f.tryFavor(t.Context(), notOwn); !isBlockedBy(got.err, blockedNotTheHolder) {
		t.Errorf("a spend by a roller that is not the owner = %+v, want NOT_THE_HOLDER first", got)
	}
	noRoll := f.ask(combat.ModeNormal)
	noRoll.Rolls = false
	if got := f.tryFavor(t.Context(), noRoll); !isBlockedBy(got.err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOTHING_TO_ROLL) {
		t.Errorf("a spend on a call that rolls no d20 = %+v, want NOTHING_TO_ROLL first", got)
	}
	if got := f.tryFavor(t.Context(), f.ask(combat.ModeAdvantage)); !isBlockedBy(got.err, blockedPassPending) {
		t.Errorf("a spend on a roll that has advantage = %+v, want PASS_PENDING before ADDS_NOTHING", got)
	}
	check := f.ask(combat.ModeNormal)
	check.Check = true
	if got := f.tryFavor(t.Context(), check); !isBlockedBy(got.err, blockedPassPending) {
		t.Errorf("the check of a spend a window will make = %+v, want PASS_PENDING", got)
	}
	f.wantSame(t, "the refused spends", w)

	// The replay of a held roll is not dropped: it rolls without the Inspiração, and says so.
	replay := f.ask(combat.ModeNormal)
	replay.Replay = true
	got := f.trySpend(t.Context(), replay)
	if got.err != nil || got.use.Lost != favorLostPending || got.use.Used || got.use.Spent {
		t.Errorf("a replay with the pass pending = %+v, want the roll without it, Lost %q", got, favorLostPending)
	}
	f.wantSame(t, "the replay", w)

	// Once the ask is canceled the spend goes again.
	if _, err := f.cancelAs(f.caio, p.GetId(), newKey()); err != nil {
		t.Fatalf("CancelFavorPass() error = %v", err)
	}
	if got := f.tryFavor(t.Context(), f.ask(combat.ModeNormal)); got.err != nil || !got.use.Spent {
		t.Errorf("a spend after the cancel = %+v, want it spent", got)
	}
}

// trySpend is tryFavor on spendFavor itself: the helper decides Replay from the gate it runs in, which a test outside a window has none of.
func (f *favorParty) trySpend(ctx context.Context, a favorAsk) spendTry {
	var out spendTry
	err := db.InTx(ctx, f.h.pool, func(tx pgx.Tx) error {
		out.use, out.err = spendFavor(ctx, f.h.svc.queries.WithTx(tx), a)
		return nil
	})
	if err != nil {
		f.h.t.Fatalf("the transaction of the spend failed: %v", err)
	}
	return out
}

func isBlockedBy(err error, reason playv1.FavorBlockedReason) bool {
	got, ok := blockedReasonOf(err)
	return ok && got == reason
}

// The check the player really rolls: the scene check with spend_favor is refused PASS_PENDING and rolls nothing.
func TestSceneCheckSpendIsRefusedWhileThePassIsPending(t *testing.T) {
	t.Parallel()
	env := newSceneCheckEnv(t)
	if err := env.grant(env.who.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := env.mustAsk(t, env.who.GetId(), env.pens.GetId())
	w := env.world(t)
	_, err := env.send(t, newKey(), true)
	wantFavorBlocked(t, "a scene check that spends, with the pass pending", err, blockedPassPending)
	env.wantSame(t, "the refused roll", w)
	if _, err := env.cancelAs(env.roller, p.GetId(), newKey()); err != nil {
		t.Fatalf("CancelFavorPass() error = %v", err)
	}
	if _, err := env.send(t, newKey(), true); err != nil {
		t.Errorf("a scene check that spends, after the cancel, error = %v", err)
	}
}

// Of a spend and an accept at once the spend never wins: it is PASS_PENDING before the accept commits and HAS_NONE after, and the
// Inspiration moves once.
func TestSpendAndAcceptRace(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8)
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	const spenders = 8
	spent := make([]spendTry, spenders)
	var accErr error
	var start, wg sync.WaitGroup
	start.Add(1)
	wg.Go(func() { start.Wait(); _, accErr = f.answer(p.GetId(), true) })
	for i := range spenders {
		wg.Go(func() { start.Wait(); spent[i] = f.tryFavor(context.Background(), f.ask(combat.ModeNormal)) })
	}
	start.Done()
	wg.Wait()
	if accErr != nil {
		t.Fatalf("the accept error = %v", accErr)
	}
	for i, s := range spent {
		if s.use.Spent || (!isBlockedBy(s.err, blockedPassPending) && !isBlockedBy(s.err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_HAS_NONE)) {
			t.Errorf("spend %d = %+v, want PASS_PENDING or HAS_NONE", i, s)
		}
	}
	if f.has(t, f.toren.GetId()) || !f.has(t, f.pens.GetId()) {
		t.Errorf("Toren has it = %v, Pensantus = %v, want it moved once", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
	}
}

// The end of a session cancels the passes still pending: the Inspiration stays with the giver, the giver's owner and the master are
// told, a third player is not.
func TestTheEndOfASessionCancelsPendingPasses(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	for _, id := range []string{f.toren.GetId(), f.bri.GetId()} {
		if err := f.grant(id); err != nil {
			t.Fatalf("GrantFavor() error = %v", err)
		}
	}
	pending := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	answered := f.mustAsk(t, f.bri.GetId(), f.pens.GetId())
	if _, err := f.answer(answered.GetId(), false); err != nil {
		t.Fatalf("Recusar error = %v", err)
	}
	ws := f.passWatchers(t)
	w := f.world(t)
	session := f.master.liveSession(t, f.campaignID).GetGameSession()
	f.master.end(t, session)

	if s := f.passStatus(t, pending.GetId()); s != "canceled" {
		t.Errorf("the pending pass is %q after the session ended, want canceled", s)
	}
	if s := f.passStatus(t, answered.GetId()); s != "refused" {
		t.Errorf("the refused pass is %q after the session ended, want it left refused", s)
	}
	if !f.has(t, f.toren.GetId()) || f.has(t, f.pens.GetId()) {
		t.Errorf("the Inspiration moved with the session's end: Toren %v, Pensantus %v", f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
	}
	if got := f.eventsOf(t, eventFavorPassCanceled); got != w.canceled+1 {
		t.Errorf("%d favor_pass_canceled events, want %d: one for the pending pass", got, w.canceled+1)
	}
	var ev favorPassEvent
	if err := json.Unmarshal(f.payloadOf(t, eventFavorPassCanceled), &ev); err != nil || ev.PassID != pending.GetId() {
		t.Errorf("favor_pass_canceled payload = %+v (%v), want the pending pass's ids", ev, err)
	}
	if reads := f.passesOf(t, f.master); len(reads) != 0 {
		t.Errorf("the master reads %v after the session, want no pending pass", reads)
	}
	// The answers of the closed pass and the new asks are refused for what they are.
	_, err := f.answer(pending.GetId(), true)
	wantFavorBlocked(t, "an accept after the session ended", err, blockedPassClosed)
	_, err = f.cancelAs(f.caio, pending.GetId(), newKey())
	wantFavorBlocked(t, "a cancel after the session ended", err, blockedPassClosed)
	_, err = f.askPass(f.toren.GetId(), f.pens.GetId())
	wantFavorBlocked(t, "an ask after the session ended", err, blockedNoSession)
	f.wantEndHints(t, hintsUntilTheEnd(t, ws))
}

// hintsUntilTheEnd reads each stream up to the end of the session and gives the characters of the vitals hints it heard on the way (every
// reader is in the map, the ones that heard nothing with an empty list).
func hintsUntilTheEnd(t *testing.T, ws map[string]*watcher) map[string][]string {
	t.Helper()
	heard := map[string][]string{}
	for name, w := range ws {
		heard[name] = nil
		for {
			ev := w.nextChange(t)
			if ev.GetSessionEnded() != nil {
				break
			}
			if v := ev.GetVitalsChanged().GetVitals().GetCharacterId(); v != "" {
				heard[name] = append(heard[name], v)
			}
		}
	}
	return heard
}

func TestTheEndOfASessionTellsTheGiverAndTheMasterOnly(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	ws := f.passWatchers(t)
	f.master.end(t, f.master.liveSession(t, f.campaignID).GetGameSession())
	f.wantEndHints(t, hintsUntilTheEnd(t, ws))
}

// wantEndHints checks the end of a session that canceled Toren's ask told Toren's vitals to the master and Caio, and told Ana and Bia nothing.
func (f *favorParty) wantEndHints(t *testing.T, heard map[string][]string) {
	t.Helper()
	for name, ids := range heard {
		want := []string(nil)
		if name == "master" || name == "caio" {
			want = []string{f.toren.GetId()}
		}
		if !slices.Equal(ids, want) {
			t.Errorf("%s heard vitals hints for %v before the end, want %v", name, ids, want)
		}
	}
}

func TestPassFavorReadsNoSession(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
	f.master.end(t, f.master.liveSession(t, f.campaignID).GetGameSession())
	if got := f.passesOf(t, f.master); len(got) != 0 {
		t.Errorf("GetFavor() after the session = %v, want no pending pass", got)
	}
}

// A giver sent back to the reserve while its ask waits is no holder of the table: the accept is NOT_THE_HOLDER (the reason of a giver
// that is no player character of the caller's), nothing moves and the pass stays pending. A receiver in the reserve is CANNOT_RECEIVE.
func TestAnswerFavorPassAcceptWithAGiverOrAReceiverInTheReserve(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		reserved func(f *favorParty) string
		want     playv1.FavorBlockedReason
	}{
		{"the giver", func(f *favorParty) string { return f.toren.GetId() }, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_THE_HOLDER},
		{"the receiver", func(f *favorParty) string { return f.pens.GetId() }, blockedCannotReceive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFavorParty(t)
			if err := f.grant(f.toren.GetId()); err != nil {
				t.Fatalf("GrantFavor() error = %v", err)
			}
			p := f.mustAsk(t, f.toren.GetId(), f.pens.GetId())
			f.execSQL(t, `UPDATE characters SET reserved = true, player_user_id = NULL WHERE id = $1`, tc.reserved(f))
			w := f.world(t)
			_, err := f.answer(p.GetId(), true)
			wantFavorBlocked(t, "Aceitar with "+tc.name+" in the reserve", err, tc.want)
			f.wantSame(t, "the refused accept", w)
			if s := f.passStatus(t, p.GetId()); s != "pending" || !f.has(t, f.toren.GetId()) || f.has(t, f.pens.GetId()) {
				t.Errorf("pass %q, Toren has it %v, Pensantus %v, want it pending and unmoved", s, f.has(t, f.toren.GetId()), f.has(t, f.pens.GetId()))
			}
		})
	}
}
