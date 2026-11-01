package progression

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The acceptance criteria of MR-041's server part (docs/product/stories.md):
// "Voltar à cidade" turns the treasures the party found into one GOLD award,
// each treasure converted once, and the undo of that award frees them. The
// treasure points are written straight into the tables, as the maps module's
// own tests cover marking them found.

// treasure writes a TREASURE point of the map, found by the characters (it is
// not found when there are none), in the session when one is given.
func (tb *table) treasure(t *testing.T, mapID, name string, po int32, session *string, finders ...string) string {
	t.Helper()
	id := newKey()
	found := "NULL"
	if len(finders) > 0 {
		found = "now()"
	}
	if _, err := tb.h.pool.Exec(t.Context(),
		`INSERT INTO map_points (id, map_id, kind, name, x_bp, y_bp, treasure_value_po, treasure_found_at, treasure_session_id, created_at, updated_at)
		 VALUES ($1, $2, 'treasure', $3, 100, 100, $4, `+found+`, $5, now(), now())`, id, mapID, name, po, session); err != nil {
		t.Fatalf("insert the treasure point: %v", err)
	}
	for _, f := range finders {
		if _, err := tb.h.pool.Exec(t.Context(), `INSERT INTO map_treasure_finders (point_id, character_id) VALUES ($1, $2)`, id, f); err != nil {
			t.Fatalf("insert a finder: %v", err)
		}
	}
	return id
}

// startSession opens a game session and returns its ID.
func (tb *table) startSession(t *testing.T) string {
	t.Helper()
	res, err := tb.master.play.StartGameSession(t.Context(), connect.NewRequest(&playv1.StartGameSessionRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatalf("StartGameSession() error = %v", err)
	}
	return res.Msg.GetGameSession().GetId()
}

// toConvert lists the treasures to convert as the master, or fails the test.
func (tb *table) toConvert(t *testing.T) []*progressionv1.TreasureToConvert {
	t.Helper()
	res, err := tb.master.xp.ListTreasuresToConvert(t.Context(), connect.NewRequest(&progressionv1.ListTreasuresToConvertRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatalf("ListTreasuresToConvert() error = %v", err)
	}
	return res.Msg.GetTreasures()
}

// backToTown calls "Voltar à cidade" with the treasures.
func (tb *table) backToTown(key string, characters []string, treasures ...string) (*progressionv1.AwardXPResponse, error) {
	return tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.CharacterIds, r.TreasurePointIds, r.Reason = progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, characters, treasures, "Voltar à cidade"
		if key != "" {
			r.IdempotencyKey = key
		}
	})
}

func treasureIDs(ts []*progressionv1.XPAwardTreasure) []string {
	var ids []string
	for _, t := range ts {
		ids = append(ids, t.GetPointId())
	}
	return ids
}

// TestMR041_VoltarACidadeConvertsTreasuresIntoOneGoldAward: three treasures of
// 120, 50 and 250 PO make one GOLD award of 420 XP, 140 for each of three
// characters; the history, the response and the session's event carry the
// treasures (IDs and PO), and the list of treasures to convert is empty after.
func TestMR041_VoltarACidadeConvertsTreasuresIntoOneGoldAward(t *testing.T) {
	t.Parallel()
	tb := newTable(t, gold, 3)
	mapID := tb.newMap()
	session := tb.startSession(t)
	pens, toren, brisa := tb.pcs[0].GetId(), tb.pcs[1].GetId(), tb.pcs[2].GetId()
	bolsa := tb.treasure(t, mapID, "Bolsa do capitão", 120, &session, toren)
	idolo := tb.treasure(t, mapID, "Ídolo de prata", 50, nil, pens, brisa) // found outside a session
	bau := tb.treasure(t, mapID, "Baú de moedas", 250, &session, brisa)
	tb.treasure(t, mapID, "Cálice não encontrado", 999, nil) // not found: not listed

	listed := tb.toConvert(t)
	if len(listed) != 3 {
		t.Fatalf("ListTreasuresToConvert() = %d treasures, want the 3 found", len(listed))
	}
	byID := map[string]*progressionv1.TreasureToConvert{}
	for _, tr := range listed {
		byID[tr.GetPointId()] = tr
	}
	if tr := byID[idolo]; tr == nil || tr.GetValuePo() != 50 || tr.GetFoundInSession() || tr.GetMapName() != "Emboscada" || tr.GetName() != "Ídolo de prata" ||
		len(tr.GetFoundBy()) != 2 || tr.GetFoundBy()[0].GetCharacterName() != "Brisa" || tr.GetFoundBy()[1].GetCharacterName() != "Pensantus" || tr.GetFoundAt() == nil {
		t.Errorf("the idol = %v, want 50 PO on Emboscada, found by Brisa and Pensantus (by name), outside a session", tr)
	}
	if tr := byID[bolsa]; tr == nil || !tr.GetFoundInSession() || tr.GetValuePo() != 120 {
		t.Errorf("the purse = %v, want 120 PO found in the session", tr)
	}

	key := newKey()
	res, err := tb.backToTown(key, tb.ids(3), bolsa, idolo, bau)
	if err != nil {
		t.Fatalf("AwardXP(voltar à cidade) error = %v", err)
	}
	award := res.GetAward()
	if res.GetXpEach() != 140 || res.GetLostXp() != 0 || award.GetGold() != 420 || award.GetTotalXp() != 420 ||
		award.GetMode() != progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD || len(res.GetTreasures()) != 3 || len(award.GetTreasures()) != 3 {
		t.Errorf("award = each %d, lost %d, gold %d, total %d, %d treasures; want 140, 0, 420, 420, 3",
			res.GetXpEach(), res.GetLostXp(), award.GetGold(), award.GetTotalXp(), len(res.GetTreasures()))
	}
	var po int32
	for _, tr := range award.GetTreasures() {
		po += tr.GetValuePo()
	}
	if po != 420 {
		t.Errorf("the treasures' PO = %d, want 420", po)
	}
	for _, pc := range tb.pcs {
		if got := tb.master.xpOf(t, pc); got != 140 {
			t.Errorf("sheet XP = %d, want 140", got)
		}
	}
	if left := tb.toConvert(t); len(left) != 0 {
		t.Errorf("ListTreasuresToConvert() after = %d treasures, want none", len(left))
	}

	// The history shows it to a player too, but RN-10: a player
	// gets the count and the PO together, never which treasures nor each one's PO.
	if h := tb.master.history(t, tb.campaign); len(h) != 1 || len(h[0].GetTreasures()) != 3 || h[0].GetTreasureCount() != 3 || h[0].GetGold() != 420 {
		t.Errorf("the master's history = %v, want the award with its 3 treasures", h)
	}
	h := tb.players[0].history(t, tb.campaign)
	if len(h) != 1 || h[0].GetTreasureCount() != 3 || h[0].GetGold() != 420 || len(h[0].GetTreasures()) != 0 {
		t.Errorf("a player's history = %v, want 3 treasures counted and 420 PO, with none listed", h)
	}
	for _, tr := range award.GetTreasures() {
		if strings.Contains(asJSON(t, h[0]), tr.GetPointId()) {
			t.Errorf("a player's history holds the treasure %s", tr.GetPointId())
		}
	}

	// The session's event holds IDs and PO only.
	var payload []byte
	if err := tb.h.pool.QueryRow(t.Context(), `SELECT payload FROM session_events WHERE kind = 'xp_awarded'`).Scan(&payload); err != nil {
		t.Fatalf("read the event: %v", err)
	}
	var decoded struct {
		Gold      int32 `json:"gold"`
		Treasures []struct {
			PointID string `json:"point_id"`
			ValuePO int32  `json:"value_po"`
		} `json:"treasures"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Gold != 420 || len(decoded.Treasures) != 3 {
		t.Errorf("event payload %s (%v), want gold 420 and 3 treasures", payload, err)
	}
	for _, banned := range []string{"Bolsa", "Baú", "Ídolo", "Voltar", "Pensantus"} {
		if strings.Contains(string(payload), banned) {
			t.Errorf("event payload %s carries %q: IDs and PO only", payload, banned)
		}
	}

	// A retry of the same request answers with the same award and changes
	// nothing; the same key for other treasures is a bug in the app.
	again, err := tb.backToTown(key, tb.ids(3), bolsa, idolo, bau)
	if err != nil || again.GetAward().GetId() != award.GetId() {
		t.Errorf("a retry = %v, %v; want the same award", again.GetAward().GetId(), err)
	}
	_, err = tb.backToTown(key, tb.ids(3), bolsa)
	wantCode(t, "the key for other treasures", err, connect.CodeInvalidArgument)
	_, err = tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Gold, r.CharacterIds, r.IdempotencyKey = progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, 420, tb.ids(3), key
	})
	wantCode(t, "the key for a typed amount", err, connect.CodeInvalidArgument)
	if got := tb.master.xpOf(t, tb.pcs[0]); got != 140 {
		t.Errorf("sheet XP after the retries = %d, want still 140", got)
	}
	if n := len(tb.master.history(t, tb.campaign)); n != 1 {
		t.Errorf("history has %d awards, want 1", n)
	}
}

// TestMR041_ATreasureIsConvertedOnce: a transaction holds the treasure's row
// locked while two awards for it start together. Neither finishes before the
// lock is released (the proof that the award takes the lock); then one wins and
// the other is refused with TREASURE_ALREADY_CONVERTED, and the XP is given once.
func TestMR041_ATreasureIsConvertedOnce(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 4) // the racers must overlap: one connection would run them one by one
	tb := newTable(t, gold, 2)
	mapID := tb.newMap()
	chest := tb.treasure(t, mapID, "Baú de moedas", 250, nil, tb.pcs[0].GetId())

	// A connection of its own: the service's pool has one, and the awards need it.
	holder, err := dbtest.SideConnection(t, tb.h.pool).Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = holder.Rollback(context.Background()) }()
	if _, err := holder.Exec(t.Context(), `SELECT id FROM map_points WHERE id = $1 FOR UPDATE`, chest); err != nil {
		t.Fatalf("lock the treasure: %v", err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	done := make(chan int, 2)
	start := dbtest.NewBarrier(len(errs))
	for i := range errs {
		wg.Go(func() {
			start.Wait()
			_, errs[i] = tb.backToTown("", tb.ids(2), chest)
			done <- i
		})
	}
	// Wait, on a condition and not on a clock, until both awards are stopped at the
	// treasure's lock; neither may have finished by then.
	observer := dbtest.SideConnection(t, tb.h.pool)
	for waiting, deadline := 0, time.Now().Add(30*time.Second); waiting < 2; time.Sleep(5 * time.Millisecond) {
		var err error
		if waiting, err = dbtest.WaitingStatements(t.Context(), observer, `%FOR UPDATE OF p%`); err != nil {
			t.Fatalf("look for the waiting awards: %v", err)
		}
		select {
		case i := <-done:
			t.Fatalf("award %d finished while the treasure's row was locked: %v", i, errs[i])
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d awards were stopped at the treasure's lock in 30 s, want 2", waiting)
		}
	}
	select {
	case i := <-done:
		t.Fatalf("award %d finished while the treasure's row was locked: %v", i, errs[i])
	default:
	}
	if err := holder.Commit(t.Context()); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	wg.Wait()
	var won, refused int
	for _, err := range errs {
		if err == nil {
			won++
			continue
		}
		wantBlocked(t, "the losing award", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_TREASURE_ALREADY_CONVERTED)
		refused++
	}
	if won != 1 || refused != 1 {
		t.Fatalf("%d awards won and %d were refused, want 1 and 1 (%v)", won, refused, errs)
	}
	if got := tb.master.xpOf(t, tb.pcs[0]); got != 125 {
		t.Errorf("sheet XP = %d, want 125: given once", got)
	}
	// And later too, with another key.
	_, err = tb.backToTown("", tb.ids(2), chest)
	wantBlocked(t, "converting it again", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_TREASURE_ALREADY_CONVERTED)
}

// TestMR041_ADoubleSubmitConvertsOnce: the same request, same idempotency key,
// sent twice at the same moment: one award, the XP given once, and each call
// answers with that award (or asks to try again).
func TestMR041_ADoubleSubmitConvertsOnce(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 4) // the racers must overlap: one connection would run them one by one
	tb := newTable(t, gold, 2)
	mapID := tb.newMap()
	chest := tb.treasure(t, mapID, "Baú de moedas", 250, nil, tb.pcs[0].GetId())
	key := newKey()
	var wg sync.WaitGroup
	res := make([]*progressionv1.AwardXPResponse, 2)
	errs := make([]error, 2)
	start := dbtest.NewBarrier(len(res))
	for i := range res {
		wg.Go(func() {
			start.Wait()
			res[i], errs[i] = tb.backToTown(key, tb.ids(2), chest)
		})
	}
	wg.Wait()
	var ids []string
	for i := range res {
		switch {
		case errs[i] == nil:
			ids = append(ids, res[i].GetAward().GetId())
		case connect.CodeOf(errs[i]) != connect.CodeAborted:
			t.Errorf("submit %d error = %v, want it answered or aborted", i, errs[i])
		}
	}
	if len(ids) == 0 || (len(ids) == 2 && ids[0] != ids[1]) {
		t.Errorf("the answers = %v (%v), want the same award", ids, errs)
	}
	if n := len(tb.master.history(t, tb.campaign)); n != 1 {
		t.Errorf("history has %d awards, want 1", n)
	}
	if got := tb.master.xpOf(t, tb.pcs[0]); got != 125 {
		t.Errorf("sheet XP = %d, want 125: given once", got)
	}
}

// TestMR041_TheListStopsAtWhatOneConversionTakes: more than 100 found treasures
// list the oldest 100 and the total.
func TestMR041_TheListStopsAtWhatOneConversionTakes(t *testing.T) {
	t.Parallel()
	tb := newTable(t, gold, 1)
	mapID := tb.newMap()
	if _, err := tb.h.pool.Exec(t.Context(), `
		INSERT INTO map_points (id, map_id, kind, name, x_bp, y_bp, treasure_value_po, treasure_found_at, created_at, updated_at)
		SELECT gen_random_uuid(), $1, 'treasure', 'Tesouro ' || n::TEXT, 100, 100, 10, now() + (n || ' seconds')::INTERVAL, now(), now()
		FROM generate_series(1, 101) AS n`, mapID); err != nil {
		t.Fatalf("insert the treasures: %v", err)
	}
	res, err := tb.master.xp.ListTreasuresToConvert(t.Context(), connect.NewRequest(&progressionv1.ListTreasuresToConvertRequest{CampaignId: tb.campaign}))
	if err != nil {
		t.Fatalf("ListTreasuresToConvert() error = %v", err)
	}
	got := res.Msg.GetTreasures()
	if len(got) != 100 || res.Msg.GetTotal() != 101 || got[0].GetName() != "Tesouro 1" || got[99].GetName() != "Tesouro 100" {
		t.Errorf("list = %d treasures (%s to %s), total %d; want the oldest 100 of 101", len(got), got[0].GetName(), got[len(got)-1].GetName(), res.Msg.GetTotal())
	}
	// And all of them together convert (100 at most per award).
	var ids []string
	for _, tr := range got {
		ids = append(ids, tr.GetPointId())
	}
	if out, err := tb.backToTown("", tb.ids(1), ids...); err != nil || out.GetXpEach() != 1000 {
		t.Errorf("converting the 100 = %v, %v; want 1000 XP", out, err)
	}
	if res2 := tb.toConvert(t); len(res2) != 1 {
		t.Errorf("%d treasures left, want 1", len(res2))
	}
}

// TestMR041_UndoFreesTheTreasures: undoing the conversion takes the XP back and
// makes the treasures "found, not converted" again, to convert again; the
// history keeps the undone award with its treasures. After a later award the
// conversion is not the last one: the treasures stay converted until that
// award is undone.
func TestMR041_UndoFreesTheTreasures(t *testing.T) {
	t.Parallel()
	tb := newTable(t, gold, 2)
	mapID := tb.newMap()
	session := tb.startSession(t)
	a := tb.treasure(t, mapID, "Bolsa", 120, &session, tb.pcs[0].GetId())
	b := tb.treasure(t, mapID, "Ídolo", 80, &session, tb.pcs[1].GetId())

	res, err := tb.backToTown("", tb.ids(2), a, b)
	if err != nil {
		t.Fatalf("AwardXP(voltar à cidade) error = %v", err)
	}
	// A hand-typed award after it: the conversion is no longer the last.
	tb.master.manual(t, tb.campaign, 10, tb.ids(2)...)
	_, err = tb.master.xp.UndoLastXPAward(t.Context(), connect.NewRequest(&progressionv1.UndoLastXPAwardRequest{
		CampaignId: tb.campaign, IdempotencyKey: newKey(), ExpectedAwardId: res.GetAward().GetId(),
	}))
	wantCode(t, "undoing the conversion under a later award", err, connect.CodeAborted)
	if left := tb.toConvert(t); len(left) != 0 {
		t.Fatalf("%d treasures are free under a later award, want none", len(left))
	}
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil { // the manual one
		t.Fatalf("UndoLastXPAward(manual) error = %v", err)
	}
	if left := tb.toConvert(t); len(left) != 0 {
		t.Fatalf("%d treasures are free after undoing another award, want none", len(left))
	}

	undone, err := tb.master.undo(tb.campaign, newKey())
	if err != nil || undone.GetId() != res.GetAward().GetId() || !undone.GetUndone() {
		t.Fatalf("UndoLastXPAward(conversion) = %v, %v; want the conversion undone", undone, err)
	}
	for _, pc := range tb.pcs {
		if got := tb.master.xpOf(t, pc); got != 0 {
			t.Errorf("sheet XP = %d, want 0 after the undo", got)
		}
	}
	freed := tb.toConvert(t)
	if len(freed) != 2 {
		t.Fatalf("ListTreasuresToConvert() after the undo = %d, want the 2 treasures back", len(freed))
	}
	// The history keeps what the undone award converted.
	var line *progressionv1.XPAward
	for _, a := range tb.master.history(t, tb.campaign) {
		if a.GetId() == res.GetAward().GetId() {
			line = a
		}
	}
	if line == nil || !line.GetUndone() || len(line.GetTreasures()) != 2 || line.GetGold() != 200 {
		t.Errorf("the undone award in the history = %v, want 200 PO and its 2 treasures", line)
	}
	var payload []byte
	if err := tb.h.pool.QueryRow(t.Context(), `SELECT payload FROM session_events WHERE kind = 'xp_award_undone' AND payload->>'award_id' = $1`, res.GetAward().GetId()).Scan(&payload); err != nil ||
		!strings.Contains(string(payload), "treasures") {
		t.Errorf("the undo event = %s, %v; want the treasures in it", payload, err)
	}

	// Free again: the same treasures convert once more.
	again, err := tb.backToTown("", tb.ids(2), a, b)
	if err != nil || again.GetXpEach() != 100 || !sameSet(treasureIDs(again.GetTreasures()), []string{a, b}) {
		t.Errorf("converting again = %v, %v; want 100 XP each for the same treasures", again, err)
	}
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			found = found || g == w
		}
		if !found {
			return false
		}
	}
	return true
}

// TestMR041_VoltarACidadeRefusesWhatDoesNotFit: a campaign by enemies refuses
// the mode; treasures with a typed amount, in another mode, repeated or too
// many are invalid; a treasure of another campaign is not found; one not found
// yet or already converted has its reason; and a refusal converts nothing, not
// even the valid treasures sent with the bad one.
func TestMR041_VoltarACidadeRefusesWhatDoesNotFit(t *testing.T) {
	t.Parallel()
	tb := newTable(t, gold, 2)
	mapID := tb.newMap()
	found := tb.treasure(t, mapID, "Bolsa", 120, nil, tb.pcs[0].GetId())
	hidden := tb.treasure(t, mapID, "Cálice escondido", 40, nil)
	converted := tb.treasure(t, mapID, "Ídolo", 80, nil, tb.pcs[1].GetId())
	if _, err := tb.backToTown("", tb.ids(2), converted); err != nil {
		t.Fatalf("AwardXP(voltar à cidade) error = %v", err)
	}
	// Another campaign's treasure (the same master, a second campaign).
	other := tb.h.newCampaign(tb.master, "Outra", gold)
	imageID, otherMap := newKey(), newKey()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at) VALUES ($1, $2, 'Mapa', 'image/png', 1000, 500, 100, now())`, []any{imageID, other}},
		{`INSERT INTO maps (id, campaign_id, name, image_id, grid_columns, created_at, updated_at) VALUES ($1, $2, 'Outro', $3, 20, now(), now())`, []any{otherMap, other, imageID}},
	} {
		if _, err := tb.h.pool.Exec(t.Context(), q.sql, q.args...); err != nil {
			t.Fatalf("insert the other campaign's map: %v", err)
		}
	}
	foreign := tb.treasure(t, otherMap, "Tesouro de outra mesa", 500, nil, tb.pcs[0].GetId())
	xpBefore := tb.master.xpOf(t, tb.pcs[0])

	check := func(call string, treasures []string, edit func(*progressionv1.AwardXPRequest), want connect.Code, reason progressionv1.XPBlockedReason) {
		t.Helper()
		_, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
			r.Mode, r.CharacterIds, r.TreasurePointIds = progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, tb.ids(2), treasures
			if edit != nil {
				edit(r)
			}
		})
		if want == connect.CodeFailedPrecondition {
			wantBlocked(t, call, err, reason)
			return
		}
		wantCode(t, call, err, want)
	}
	const none = progressionv1.XPBlockedReason_XP_BLOCKED_REASON_UNSPECIFIED
	check("treasures with gold", []string{found}, func(r *progressionv1.AwardXPRequest) { r.Gold = 120 }, connect.CodeInvalidArgument, none)
	check("treasures in MANUAL mode", []string{found}, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Amount = progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL, 10
	}, connect.CodeInvalidArgument, none)
	check("a repeated treasure", []string{found, found}, nil, connect.CodeInvalidArgument, none)
	check("a treasure that is not a UUID", []string{"x"}, nil, connect.CodeInvalidArgument, none)
	check("101 treasures", func() []string {
		var ids []string
		for range 101 {
			ids = append(ids, newKey())
		}
		return ids
	}(), nil, connect.CodeInvalidArgument, none)
	scene := newKey()
	if _, err := tb.h.pool.Exec(t.Context(), `INSERT INTO map_points (id, map_id, kind, name, x_bp, y_bp, created_at, updated_at) VALUES ($1, $2, 'scene', 'A carroça', 100, 100, now(), now())`, scene, mapID); err != nil {
		t.Fatalf("insert the scene point: %v", err)
	}
	check("a scene point of the campaign", []string{found, scene}, nil, connect.CodeNotFound, none)
	check("another campaign's treasure", []string{found, foreign}, nil, connect.CodeNotFound, none)
	check("an ID that is no point at all", []string{found, newKey()}, nil, connect.CodeNotFound, none)
	check("a treasure not found yet", []string{found, hidden}, nil, connect.CodeFailedPrecondition, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_TREASURE_NOT_FOUND_YET)
	check("a treasure converted already", []string{found, converted}, nil, connect.CodeFailedPrecondition, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_TREASURE_ALREADY_CONVERTED)
	if got := tb.master.xpOf(t, tb.pcs[0]); got != xpBefore {
		t.Errorf("sheet XP = %d after refusals, want %d", got, xpBefore)
	}
	if left := tb.toConvert(t); len(left) != 1 || left[0].GetPointId() != found {
		t.Errorf("ListTreasuresToConvert() = %d treasures, want only the purse: a refusal converts nothing", len(left))
	}
	// Together they are worth more than a sheet holds: a typed refusal.
	big1 := tb.treasure(t, mapID, "Tesouro enorme", 600000, nil, tb.pcs[0].GetId())
	big2 := tb.treasure(t, mapID, "Outro tesouro enorme", 600000, nil, tb.pcs[0].GetId())
	check("treasures over the limit", []string{big1, big2}, nil, connect.CodeFailedPrecondition, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_TREASURES_OVER_LIMIT)
	// Treasures worth nothing give nothing.
	zero := tb.treasure(t, mapID, "Baú vazio", 0, nil, tb.pcs[0].GetId())
	check("treasures worth 0 PO", []string{zero}, nil, connect.CodeFailedPrecondition, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_NOTHING_TO_GIVE)
	// Typed gold keeps working (the alternative of "Dar XP por ouro").
	if _, err := tb.master.award(tb.campaign, func(r *progressionv1.AwardXPRequest) {
		r.Mode, r.Gold, r.CharacterIds = progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD, 60, tb.ids(2)
	}); err != nil {
		t.Errorf("AwardXP(typed gold) error = %v", err)
	}

	// A campaign by enemies refuses the mode, and still lists its treasures.
	en := newTable(t, enemies, 1)
	enMap := en.newMap()
	chest := en.treasure(t, enMap, "Baú", 250, nil, en.pcs[0].GetId())
	_, err := en.backToTown("", en.ids(1), chest)
	wantBlocked(t, "voltar à cidade in an enemies campaign", err, progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED)
	if got := en.toConvert(t); len(got) != 1 {
		t.Errorf("an enemies campaign lists %d treasures, want its 1 (it keeps them with no conversion)", len(got))
	}
}

// TestMR041_OnlyTheMasterReadsTheTreasuresToConvert: a player is refused (the
// authorization matrix has the other callers).
func TestMR041_OnlyTheMasterReadsTheTreasuresToConvert(t *testing.T) {
	t.Parallel()
	tb := newTable(t, gold, 1)
	_, err := tb.players[0].xp.ListTreasuresToConvert(t.Context(), connect.NewRequest(&progressionv1.ListTreasuresToConvertRequest{CampaignId: tb.campaign}))
	wantCode(t, "a player listing the treasures", err, connect.CodePermissionDenied)
}

// asJSON is a message as the app reads it.
func asJSON(t *testing.T, m proto.Message) string {
	t.Helper()
	b, err := protojson.Marshal(m)
	if err != nil {
		t.Fatalf("protojson.Marshal() error = %v", err)
	}
	return string(b)
}

// TestMR041_AConversionAndItsUndoTellTheMapsOnceAfterTheCommit: converting
// treasures of two maps publishes one change per map, only once the award is
// committed (the conversion that retries after a serialization conflict
// publishes once, and a refused one publishes nothing); the undo publishes for
// the maps it freed.
func TestMR041_AConversionAndItsUndoTellTheMapsOnceAfterTheCommit(t *testing.T) {
	t.Parallel()
	tb := newTable(t, gold, 2)
	first, second := tb.newMap(), tb.newMap()
	session := tb.startSession(t)
	a := tb.treasure(t, first, "Bolsa", 120, &session, tb.pcs[0].GetId())
	b := tb.treasure(t, first, "Ídolo", 80, &session, tb.pcs[1].GetId())
	c := tb.treasure(t, second, "Cálice", 50, &session, tb.pcs[0].GetId())
	unfound := tb.treasure(t, second, "Baú", 70, nil)

	// A refused award changes no map: nothing is published.
	if _, err := tb.backToTown("", tb.ids(2), a, unfound); err == nil {
		t.Fatal("AwardXP(voltar à cidade) with a treasure not found yet succeeded, want a refusal")
	}
	if got := tb.h.treasures.published(); len(got) != 0 {
		t.Fatalf("published after a refused award = %v, want nothing", got)
	}

	// The first attempt conflicts after writing; the retry commits, and one
	// publish per map follows, for the committed write only.
	tb.h.treasures.mu.Lock()
	tb.h.treasures.conflicts = 1
	tb.h.treasures.mu.Unlock()
	key := newKey()
	res, err := tb.backToTown(key, tb.ids(2), a, b, c)
	if err != nil {
		t.Fatalf("AwardXP(voltar à cidade) error = %v", err)
	}
	want := []published{{campaignID: tb.campaign, mapIDs: sortedCopy(first, second)}}
	if got := tb.h.treasures.published(); !publishedEqual(got, want) {
		t.Fatalf("published after the conversion = %v, want %v", got, want)
	}

	// The same request again is a repeat: no new publish.
	if _, err := tb.backToTown(key, tb.ids(2), a, b, c); err != nil {
		t.Fatalf("AwardXP(voltar à cidade) repeated error = %v", err)
	}
	if got := tb.h.treasures.published(); len(got) != 1 {
		t.Fatalf("published after a repeated request = %v, want the one call", got)
	}

	undoKey := newKey()
	if undone, err := tb.master.undo(tb.campaign, undoKey); err != nil || undone.GetId() != res.GetAward().GetId() {
		t.Fatalf("UndoLastXPAward() = %v, %v; want the conversion undone", undone, err)
	}
	want = append(want, want[0])
	if got := tb.h.treasures.published(); !publishedEqual(got, want) {
		t.Fatalf("published after the undo = %v, want %v", got, want)
	}
	if _, err := tb.master.undo(tb.campaign, undoKey); err != nil { // a retry of the same undo
		t.Fatalf("UndoLastXPAward() retried error = %v", err)
	}
	if got := tb.h.treasures.published(); len(got) != 2 {
		t.Fatalf("published after a repeated undo = %v, want two calls in all", got)
	}

	// An award without treasures publishes nothing about maps.
	tb.master.manual(t, tb.campaign, 10, tb.ids(2)...)
	if _, err := tb.master.undo(tb.campaign, newKey()); err != nil {
		t.Fatalf("UndoLastXPAward(manual) error = %v", err)
	}
	if got := tb.h.treasures.published(); len(got) != 2 {
		t.Fatalf("published after a manual award = %v, want two calls in all", got)
	}
}

func sortedCopy(ids ...string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}

func publishedEqual(got, want []published) bool {
	return slices.EqualFunc(got, want, func(g, w published) bool {
		return g.campaignID == w.campaignID && slices.Equal(sortedCopy(g.mapIDs...), sortedCopy(w.mapIDs...))
	})
}
