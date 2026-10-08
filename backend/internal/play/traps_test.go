package play

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
)

// Traps in play (MR-035, RN-10, RN-02, Etapa 9, slice 9.8). These tests need the
// database (MEURPG_TEST_DATABASE_URL). The fixture is the cave of the movement
// tests, with the real maps service next to the play one: Toren (6, 7), Pensantus
// (5, 8) and Brisa (4, 7) in the entrance cave, the goblins far away; a square is
// 5 ft. Without the fog every square is seen in bright light; passive Perception:
// Toren 10, Pensantus 10 (a gnome, darkvision 60 ft), Brisa 13 (Wisdom 16).

// trapRig is the cave with the maps module wired to play both ways.
type trapRig struct {
	*cave
	msvc   *maps.Service
	server *httptest.Server
}

func newTrapRig(t *testing.T) *trapRig {
	t.Helper()
	c := newCave(t)
	content, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	msvc, err := maps.New(maps.Config{Pool: c.h.pool, Characters: c.h.chars, Live: c.h.svc, Rules: content, Combats: c.h.svc, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("maps.New() error = %v", err)
	}
	c.h.svc.SetTraps(msvc)
	msvc.SetTrapFirer(c.h.svc)
	srv := httpserver.New(httpserver.Config{Logger: slog.New(slog.DiscardHandler)})
	msvc.Mount(srv.Handle, mapsSessions{testSessions}, c.h.camps, connect.WithRequireConnectProtocolHeader())
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	return &trapRig{cave: c, msvc: msvc, server: server}
}

// mc is the maps client of the user.
func (r *trapRig) mc(u *user) mapsv1connect.MapServiceClient {
	return mapsv1connect.NewMapServiceClient(&http.Client{Transport: userTransport{userID: u.id, next: r.server.Client().Transport}}, r.server.URL)
}

// sq is the position of the middle of a square of the cave's map (24 x 16 squares
// of 50 px on a 1200 x 800 image), in basis points.
func sq(col, row int) (x, y int32) {
	return int32((col*50 + 25) * 10000 / 1200), int32((row*50 + 25) * 10000 / 800) //nolint:gosec // small
}

// aTrap is a trap of the master's with no effect: notice DC 12, find DC 15, one
// square, "Ao entrar na área".
func aTrap() *mapsv1.TrapSpec {
	return &mapsv1.TrapSpec{NoticeDc: 12, FindDc: 15, AreaSize: 1, Trigger: rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER, Effect: &rulesv1.TrapEffect{}}
}

// trap creates a hidden trap point on the cave's map.
func (r *trapRig) trap(t *testing.T, name string, col, row int, edit ...func(*mapsv1.TrapSpec)) *mapsv1.MapPoint {
	t.Helper()
	spec := aTrap()
	for _, e := range edit {
		e(spec)
	}
	x, y := sq(col, row)
	res, err := r.mc(r.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: r.campaignID, MapId: r.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: name, Description: "descrição de " + name, XBp: x, YBp: y, Trap: spec,
	}))
	if err != nil {
		t.Fatalf("CreateMapPoint(%s) error = %v", name, err)
	}
	return res.Msg.GetPoint()
}

// place puts a character's token on a square, as the master.
func (r *trapRig) place(t *testing.T, characterID string, col, row int) {
	t.Helper()
	x, y := sq(col, row)
	if _, err := r.mc(r.master).PlaceMapToken(t.Context(), connect.NewRequest(&mapsv1.PlaceMapTokenRequest{
		CampaignId: r.campaignID, MapId: r.mapID, CharacterId: characterID, XBp: x, YBp: y,
	})); err != nil {
		t.Fatalf("PlaceMapToken() error = %v", err)
	}
}

// getMap reads the map as u.
func (r *trapRig) getMap(t *testing.T, u *user) *mapsv1.GetMapResponse {
	t.Helper()
	res, err := r.mc(u).GetMap(t.Context(), connect.NewRequest(&mapsv1.GetMapRequest{CampaignId: r.campaignID, MapId: r.mapID}))
	if err != nil {
		t.Fatalf("GetMap() error = %v", err)
	}
	return res.Msg
}

// point is the trap as u reads it, nil when u does not receive it.
func (r *trapRig) point(t *testing.T, u *user, id string) *mapsv1.MapPoint {
	t.Helper()
	for _, p := range r.getMap(t, u).GetPoints() {
		if p.GetId() == id {
			return p
		}
	}
	return nil
}

// wantKnows says whether u receives the trap.
func (r *trapRig) wantKnows(t *testing.T, who string, u *user, id string, want bool) {
	t.Helper()
	if got := r.point(t, u, id) != nil; got != want {
		t.Errorf("%s receives the trap = %v, want %v", who, got, want)
	}
}

// ownTrap is what a player's own reveal tells: the master's card says who knows it.
func (r *trapRig) revealedTo(t *testing.T, id string) []*mapsv1.TrapReveal {
	t.Helper()
	return r.point(t, r.master, id).GetTrapRevealedTo()
}

// eventCount counts the session's events of a kind.
func (r *trapRig) eventCount(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := r.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatalf("count the events: %v", err)
	}
	return n
}

// fireByHand calls FireTrap as the master.
func (r *trapRig) fireByHand(t *testing.T, p *mapsv1.MapPoint, targets ...string) (*playv1.FireTrapResponse, error) {
	t.Helper()
	res, err := r.master.play.FireTrap(t.Context(), connect.NewRequest(&playv1.FireTrapRequest{
		CampaignId: r.campaignID, MapId: r.mapID, PointId: p.GetId(), TargetIds: targets, IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// search calls SearchForTraps as u with a typed die.
func (r *trapRig) search(t *testing.T, u *user, skill playv1.TrapSearchSkill, face int32) (*playv1.SearchForTrapsResponse, error) {
	t.Helper()
	res, err := u.play.SearchForTraps(t.Context(), connect.NewRequest(&playv1.SearchForTrapsRequest{
		CampaignId: r.campaignID, IdempotencyKey: newKey(), Skill: skill, Roll: &playv1.SearchForTrapsRequest_D20Face{D20Face: face},
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// moveResult calls MoveCombatant as u and returns the whole answer.
func (r *trapRig) moveResult(t *testing.T, u *user, label string, col, row int32, edit ...func(*playv1.MoveCombatantRequest)) (*playv1.MoveCombatantResponse, error) {
	t.Helper()
	req := &playv1.MoveCombatantRequest{
		CampaignId: r.campaignID, EncounterId: r.get(t, r.master).GetId(), CombatantId: r.id(t, label), IdempotencyKey: newKey(), Col: col, Row: row,
	}
	for _, e := range edit {
		e(req)
	}
	res, err := u.combat.MoveCombatant(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (r *trapRig) trapDamages(t *testing.T) []*playv1.TrapDamage {
	t.Helper()
	res, err := r.master.play.ListTrapDamages(t.Context(), connect.NewRequest(&playv1.ListTrapDamagesRequest{CampaignId: r.campaignID}))
	if err != nil {
		t.Fatalf("ListTrapDamages() error = %v", err)
	}
	return res.Msg.GetDamages()
}

var (
	perception    = playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_PERCEPTION
	investigation = playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_INVESTIGATION
)

// TestMR035_ThePassiveNotice: a player character that ends a move within 3 m of an
// armed trap, whose passive Perception reaches the trap's DC to notice it, learns of
// the trap, and only its player does: the other player's map never has it. Out of
// range it notices nothing.
func TestMR035_ThePassiveNotice(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	pit := r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 10 })
	r.fight(t)
	r.wantKnows(t, "Toren's player", r.caio, pit.GetId(), false)

	// (7, 8) is 11,2 ft from (9, 7): out of range, nothing is noticed.
	r.mustMove(t, r.caio, "Toren", 7, 8)
	r.wantKnows(t, "Toren's player out of range", r.caio, pit.GetId(), false)
	if n := r.eventCount(t, "trap_noticed"); n != 0 {
		t.Fatalf("trap_noticed events out of range = %d, want 0", n)
	}

	// (7, 7) is 10 ft away: in range, and a passive 10 meets the DC 10.
	r.mustMove(t, r.caio, "Toren", 7, 7)
	r.wantKnows(t, "Toren's player", r.caio, pit.GetId(), true)
	r.wantKnows(t, "Pensantus's player", r.ana, pit.GetId(), false)
	r.wantKnows(t, "Brisa's player", r.bia, pit.GetId(), false)
	got := r.point(t, r.caio, pit.GetId())
	if got.GetTrap().GetNoticeDc() != 0 || got.GetTrap().GetFindDc() != 0 || got.GetTrap().GetEffect() != nil {
		t.Errorf("the trap Toren's player got = %v, want no DC and no effect", got.GetTrap())
	}
	if rev := r.revealedTo(t, pit.GetId()); len(rev) != 1 || rev[0].GetCharacterId() != r.toren.GetId() || rev[0].GetHow() != mapsv1.TrapRevealHow_TRAP_REVEAL_HOW_NOTICED {
		t.Errorf("the master's card says it is revealed to %v, want Toren, noticed", rev)
	}
	if n := r.eventCount(t, "trap_noticed"); n != 1 {
		t.Errorf("trap_noticed events = %d, want 1", n)
	}
}

// TestMR035_TheNoticeNeedsADCAndTheDCToBeMet: a trap with no DC to notice is never
// noticed passively, however close; a DC above the passive score is not met.
func TestMR035_TheNoticeNeedsADCAndTheDCToBeMet(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	needle := r.trap(t, "Agulha", 8, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 0 })
	hard := r.trap(t, "Difícil", 7, 8, func(s *mapsv1.TrapSpec) { s.NoticeDc = 11 })
	r.fight(t)
	r.mustMove(t, r.caio, "Toren", 7, 7) // next to both: Toren's passive is 10
	r.wantKnows(t, "Toren and the needle", r.caio, needle.GetId(), false)
	r.wantKnows(t, "Toren and the DC 11", r.caio, hard.GetId(), false)
}

// TestMR035_QuemNotaria: the master's card says, for each player character, its
// passive Perception and what the light at the trap's squares takes from it, as the
// character sees them now: dim light, or darkness seen through darkvision, takes 5;
// dim light seen through darkvision counts as bright; what a character does not see
// it cannot notice.
func TestMR035_QuemNotaria(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	pit := r.trap(t, "Fosso Dourado", 6, 8, func(s *mapsv1.TrapSpec) { s.NoticeDc = 8 })
	r.fight(t)
	noticers := func() map[string]*mapsv1.TrapNoticer {
		t.Helper()
		res, err := r.mc(r.master).GetTrapNoticers(t.Context(), connect.NewRequest(&mapsv1.GetTrapNoticersRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: pit.GetId()}))
		if err != nil {
			t.Fatalf("GetTrapNoticers() error = %v", err)
		}
		if res.Msg.GetNoticeDc() != 8 {
			t.Errorf("notice_dc = %d, want 8", res.Msg.GetNoticeDc())
		}
		out := map[string]*mapsv1.TrapNoticer{}
		for _, n := range res.Msg.GetNoticers() {
			out[n.GetCharacterName()] = n
		}
		return out
	}
	type want struct {
		passive, penalty           int32
		inRange, sees, wouldNotice bool
	}
	check := func(label string, got map[string]*mapsv1.TrapNoticer, wants map[string]want) {
		t.Helper()
		for name, w := range wants {
			n := got[name]
			if n == nil {
				t.Errorf("%s: %s is not on the card (card: %v)", label, name, got)
				continue
			}
			if !n.GetOnMap() || n.GetPassivePerception() != w.passive || n.GetLightPenalty() != w.penalty || n.GetInRange() != w.inRange ||
				n.GetSees() != w.sees || n.GetWouldNotice() != w.wouldNotice || n.GetKnows() {
				t.Errorf("%s: %s = %v, want passive %d, penalty %d, in range %v, sees %v, would notice %v, not knowing", label, name, n, w.passive, w.penalty, w.inRange, w.sees, w.wouldNotice)
			}
		}
	}
	// Toren (6, 7) and Pensantus (5, 8) are next to the trap, Brisa (4, 7) is 11,2 ft
	// from it. No fog: every square is seen in bright light.
	check("no fog", noticers(), map[string]want{
		"Toren": {10, 0, true, true, true}, "Pensantus": {10, 0, true, true, true}, "Brisa": {13, 0, false, true, false},
	})

	mc := r.mc(r.master)
	fog := func(light mapsv1.LightLevel) {
		t.Helper()
		if _, err := mc.SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{
			CampaignId: r.campaignID, MapId: r.mapID, FogEnabled: new(true), BaseLight: &light,
		})); err != nil {
			t.Fatalf("SetMapFog() error = %v", err)
		}
	}
	// Dim light: Toren sees it dim (-5, a passive 5); Pensantus's darkvision makes it
	// bright (no penalty).
	fog(mapsv1.LightLevel_LIGHT_LEVEL_DIM)
	check("dim light", noticers(), map[string]want{
		"Toren": {10, -5, true, true, false}, "Pensantus": {10, 0, true, true, true}, "Brisa": {13, -5, false, true, false},
	})
	// Darkness: Toren sees nothing of it; Pensantus sees it in grey, -5.
	fog(mapsv1.LightLevel_LIGHT_LEVEL_DARK)
	got := noticers()
	if n := got["Toren"]; n.GetSees() || n.GetWouldNotice() || n.GetLightPenalty() != 0 {
		t.Errorf("in the dark Toren = %v, want not seeing the trap", n)
	}
	if n := got["Pensantus"]; !n.GetSees() || n.GetLightPenalty() != -5 || n.GetWouldNotice() {
		t.Errorf("in the dark Pensantus (darkvision) = %v, want seeing it with -5 and a passive 5, short of the DC 8", n)
	}
}

// TestMR035_APassiveNoticeFollowsTheLight: with the fog on and dim light, Toren's
// passive 10 is 5 and misses a DC 8, but Pensantus's darkvision keeps hers: her move
// notices what his does not.
func TestMR035_APassiveNoticeFollowsTheLight(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	pit := r.trap(t, "Fosso Dourado", 6, 8, func(s *mapsv1.TrapSpec) { s.NoticeDc = 8 })
	light := mapsv1.LightLevel_LIGHT_LEVEL_DIM
	if _, err := r.mc(r.master).SetMapFog(t.Context(), connect.NewRequest(&mapsv1.SetMapFogRequest{
		CampaignId: r.campaignID, MapId: r.mapID, FogEnabled: new(true), BaseLight: &light,
	})); err != nil {
		t.Fatalf("SetMapFog() error = %v", err)
	}
	r.fight(t)
	r.mustMove(t, r.caio, "Toren", 7, 8) // next to the trap, in dim light
	r.wantKnows(t, "Toren in dim light", r.caio, pit.GetId(), false)
	r.mustMove(t, r.master, "Pensantus", 5, 9) // the master moves her: she ends a move next to it
	r.wantKnows(t, "Pensantus with darkvision", r.ana, pit.GetId(), true)
}

// TestMR035_Searching: "Procurar armadilhas" rolls Perception against the DC to
// notice or Investigation against the DC to find, for traps within 3 m that the
// character sees; a pass reveals the trap to that character alone. A trap with no
// DC to notice is found only with Investigation. The answer reads the same when the
// roll fell short and when nothing is there, and never carries a DC (RN-10).
func TestMR035_Searching(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	pit := r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 16, 17 })
	needle := r.trap(t, "Agulha Rubra", 8, 8, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 0, 13 })
	far := r.trap(t, "Lá longe", 20, 12)
	pens := r.pens.GetId()

	// Not on the map yet: the search is refused by the state.
	_, err := r.search(t, r.ana, investigation, 12)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_NOT_ON_MAP)

	r.place(t, pens, 8, 7) // next to both traps
	r.place(t, r.toren.GetId(), 3, 3)

	// Investigation 11 + 4 (Intelligence 18 with the gnome's bonus) = 15 misses the DC 17
	// of the pit; the needle's DC 13 is met.
	miss, err := r.search(t, r.ana, investigation, 11)
	if err != nil {
		t.Fatalf("SearchForTraps() error = %v", err)
	}
	if got := miss.GetRoll(); got.GetTotal() != 15 || got.GetModifier() != 4 || got.GetFaces()[0] != 11 {
		t.Errorf("the roll = %v, want 11 + 4 = 15", got)
	}
	if len(miss.GetFoundPointIds()) != 1 || miss.GetFoundPointIds()[0] != needle.GetId() {
		t.Fatalf("found = %v, want only the needle (DC 13)", miss.GetFoundPointIds())
	}
	r.wantKnows(t, "Pensantus's player (needle)", r.ana, needle.GetId(), true)
	r.wantKnows(t, "Pensantus's player (pit)", r.ana, pit.GetId(), false)
	r.wantKnows(t, "Toren's player (needle)", r.caio, needle.GetId(), false)

	// Perception 12 + 0 misses the pit's DC to notice (16); a natural 20 is 20.
	res, err := r.search(t, r.ana, perception, 12)
	if err != nil || len(res.GetFoundPointIds()) != 0 {
		t.Fatalf("a Perception 12 against a DC 16: found %v, error %v; want nothing", res.GetFoundPointIds(), err)
	}
	// The needle has no DC to notice: Perception never finds it (she knows it, so
	// look at the pit alone: a 20 reaches 16).
	res, err = r.search(t, r.ana, perception, 20)
	if err != nil || len(res.GetFoundPointIds()) != 1 || res.GetFoundPointIds()[0] != pit.GetId() {
		t.Fatalf("a Perception 20: found %v, error %v; want the pit", res.GetFoundPointIds(), err)
	}
	r.wantKnows(t, "Pensantus's player (pit)", r.ana, pit.GetId(), true)
	r.wantKnows(t, "Brisa's player (pit)", r.bia, pit.GetId(), false)
	r.wantKnows(t, "Pensantus's player (far trap)", r.ana, far.GetId(), false)

	// Nothing there reads as a failed roll: Toren searches far from any trap with a 20.
	nothing, err := r.search(t, r.caio, investigation, 20)
	if err != nil {
		t.Fatalf("SearchForTraps() error = %v", err)
	}
	failed, err := r.search(t, r.ana, investigation, 1) // nothing left for her to find either
	if err != nil {
		t.Fatalf("SearchForTraps() error = %v", err)
	}
	for name, got := range map[string]*playv1.SearchForTrapsResponse{"nothing there": nothing, "a failed roll": failed} {
		if len(got.GetFoundPointIds()) != 0 || got.GetSpentAction() {
			t.Errorf("%s: %v, want no trap found and no action spent", name, got)
		}
		// A JSON key that holds a DC ("noticeDc", "findDc", "dc"), never the letters "dc" anywhere in the text:
		// a UUID in the answer would contain them one time in 65 and fail this test by chance (testing audit T7).
		if js, _ := protojson.Marshal(got); dcKey.MatchString(string(js)) || strings.Contains(string(js), "Fosso") {
			t.Errorf("%s: the answer %s tells a DC or a name", name, js)
		}
	}

	// The master's history has the rolls and what they found, ids and numbers only.
	rows, err := r.h.pool.Query(t.Context(), `SELECT payload::text FROM session_events WHERE kind = 'trap_searched' ORDER BY seq`)
	if err != nil {
		t.Fatalf("read the history: %v", err)
	}
	var payloads []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		payloads = append(payloads, p)
	}
	rows.Close()
	if len(payloads) != 5 || !strings.Contains(payloads[0], needle.GetId()) || !strings.Contains(payloads[2], pit.GetId()) {
		t.Errorf("trap_searched events = %v, want 5, the first finding the needle and the third the pit", payloads)
	}
}

// TestMR035_APhysicalDieFollowsTheCampaign: a search takes the d20 in the app, or the
// face of a real die, as the campaign's dice setting allows (RN-18).
func TestMR035_APhysicalDieFollowsTheCampaign(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.place(t, r.pens.GetId(), 8, 7)
	r.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP) // everybody in the app
	_, err := r.search(t, r.ana, investigation, 12)
	wantSceneBlocked(t, err, playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_WRONG_DICE_MODE)
	r.h.roller.queue(15)
	res, err := r.ana.play.SearchForTraps(t.Context(), connect.NewRequest(&playv1.SearchForTrapsRequest{
		CampaignId: r.campaignID, IdempotencyKey: newKey(), Skill: investigation, Roll: &playv1.SearchForTrapsRequest_RollInApp{RollInApp: true},
	}))
	if err != nil || res.Msg.GetRoll().GetTotal() != 19 {
		t.Fatalf("a roll in the app = %v, %v; want 15 + 4 = 19", res, err)
	}
}

// TestMR035_SearchingInACombatCostsTheAction: while a combat runs, the search is the
// SRD's Search action: only on the character's own turn, and it spends the action.
func TestMR035_SearchingInACombatCostsTheAction(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	pit := r.trap(t, "Fosso Dourado", 7, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 20 }) // too high to be noticed by passing
	e := r.fight(t)
	_, err := r.search(t, r.ana, perception, 20) // Pensantus waits for her turn
	wantEncounterBlocked(t, err, reasonNotTurn)
	res, err := r.search(t, r.caio, perception, 20)
	if err != nil || !res.GetSpentAction() || len(res.GetFoundPointIds()) != 1 || res.GetFoundPointIds()[0] != pit.GetId() {
		t.Fatalf("Toren's search = %v, %v; want the pit found and the action spent", res, err)
	}
	if toren := r.mustOptions(t, r.caio, e, "Toren"); !toren.GetOptions().GetEconomy().GetAction().GetUsed() {
		t.Errorf("Toren's action after the search = %v, want used", toren.GetOptions().GetEconomy())
	}
	_, err = r.search(t, r.caio, investigation, 20)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED)

	// The master's log has the search; the table's does not.
	found := false
	for _, round := range r.log(t, r.master, r.get(t, r.master)).GetRounds() {
		for _, entry := range round.GetEntries() {
			found = found || (entry.GetKey() == "standard:search" && entry.GetHidden())
		}
	}
	if !found {
		t.Errorf("the master's log has no hidden Procurar line")
	}
	for _, round := range r.log(t, r.caio, r.get(t, r.caio)).GetRounds() {
		for _, entry := range round.GetEntries() {
			if entry.GetKey() == "standard:search" {
				t.Errorf("a player got the search in the combat log: %v", entry)
			}
		}
	}
}

// pit is a trap whose effect is a fall: damage that always lands.
func pit(dice string) func(*mapsv1.TrapSpec) {
	return func(s *mapsv1.TrapSpec) {
		s.Effect = &rulesv1.TrapEffect{Damage: []*rulesv1.TrapDamage{{Dice: dice, DamageTypeKey: "damage-type:bludgeoning"}}}
	}
}

// TestMR035_AMoveStopsAtTheFirstSquareOfTheArea: a player's straight move runs into
// an armed "Ao entrar na área" trap and stops on its first square, and the trap fires
// there: it is public from then on, its damage to the player's character waits for the
// master (RN-02, who may change it), and an undo takes the firing back first, then the
// move. A move that stops on a trap the player did not know reads as the trap firing.
func TestMR035_AMoveStopsAtTheFirstSquareOfTheArea(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 9, 7, pit("1d6"))
	e := r.fight(t)
	r.wantKnows(t, "Pensantus's player before", r.ana, hole.GetId(), false)

	// Toren asks for (12, 7), 30 ft away; the area is (9, 7): the move stops there,
	// after 15 ft.
	r.h.roller.queue(4)
	res, err := r.moveResult(t, r.caio, "Toren", 12, 7)
	if err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	toren := byLabel(t, res.GetEncounter(), "Toren")
	if toren.GetCol() != 9 || toren.GetRow() != 7 || !res.GetStoppedEarly() || toren.GetMovementUsedDft() != 150 {
		t.Fatalf("Toren after the move = (%d, %d), %d dft, stopped early %v; want (9, 7), 150 dft, stopped early", toren.GetCol(), toren.GetRow(), toren.GetMovementUsedDft(), res.GetStoppedEarly())
	}

	// Public now: everyone who sees the map gets it, "Disparada".
	for who, u := range map[string]*user{"Pensantus's player": r.ana, "Brisa's player": r.bia, "Toren's player": r.caio} {
		p := r.point(t, u, hole.GetId())
		if p == nil || p.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_TRIGGERED || p.GetName() != "Fosso Dourado" {
			t.Errorf("%s reads the fired trap as %v, want it public and triggered", who, p)
		}
	}
	if n := r.eventCount(t, "trap_triggered"); n != 1 {
		t.Fatalf("trap_triggered events = %d, want 1", n)
	}

	// The damage waits for the master: 1d6 rolled 4.
	damages := r.trapDamages(t)
	if len(damages) != 1 || damages[0].GetAmount() != 4 || damages[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED ||
		damages[0].GetTrapName() != "Fosso Dourado" || damages[0].GetEncounterId() != e.GetId() || damages[0].GetCharacterName() != "Toren" {
		t.Fatalf("the trap damages = %v, want 4 from the Fosso Dourado waiting for the master, on Toren", damages)
	}
	before := r.vitals(t, r.toren).GetHitPointsCurrent()

	// An undo takes the firing back first: the trap is armed and hidden again, the damage gone.
	r.undoLast(t)
	if p := r.point(t, r.ana, hole.GetId()); p != nil {
		t.Errorf("after the undo Pensantus's player still gets the trap: %v", p)
	}
	if got := r.point(t, r.master, hole.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_ARMED {
		t.Errorf("the trap after the undo = %v, want armed", got)
	}
	if d := r.trapDamages(t); len(d) != 0 {
		t.Errorf("trap damages after the undo = %v, want none", d)
	}
	if got := r.who(t, r.caio, "Toren"); got.GetCol() != 9 {
		t.Errorf("Toren after the first undo = (%d, %d), want still on (9, 7)", got.GetCol(), got.GetRow())
	}
	r.undoLast(t) // then the move
	if got := r.who(t, r.caio, "Toren"); got.GetCol() != 6 || got.GetRow() != 7 || got.GetMovementUsedDft() != 0 {
		t.Errorf("Toren after the second undo = %v, want back on (6, 7) with nothing walked", got)
	}

	// The same move again fires it for good; the master applies the damage, changed.
	r.h.roller.queue(6)
	r.mustMove(t, r.caio, "Toren", 12, 7)
	damages = r.trapDamages(t)
	if len(damages) != 1 || damages[0].GetAmount() != 6 {
		t.Fatalf("the trap damages = %v, want 6", damages)
	}
	if got := r.vitals(t, r.toren).GetHitPointsCurrent(); got != before {
		t.Fatalf("the damage landed before the master applied it: %d hit points, want %d", got, before)
	}
	if _, err := r.master.combat.ApplyPendingDamage(t.Context(), connect.NewRequest(&playv1.ApplyPendingDamageRequest{
		CampaignId: r.campaignID, EncounterId: e.GetId(), PendingDamageId: damages[0].GetId(), IdempotencyKey: newKey(), Amount: proto.Int32(3),
	})); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if got := r.vitals(t, r.toren).GetHitPointsCurrent(); got != before-3 {
		t.Errorf("Toren's hit points = %d, want %d (the master changed 6 to 3)", got, before-3)
	}
	if d := r.trapDamages(t); len(d) != 0 {
		t.Errorf("trap damages after the apply = %v, want none", d)
	}
	// The log tells the table what fired; a player never reads another character's dice.
	var line *playv1.CombatLogEntry
	for _, round := range r.log(t, r.ana, r.get(t, r.ana)).GetRounds() {
		for _, entry := range round.GetEntries() {
			if entry.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_TRAP_TRIGGERED {
				line = entry
			}
		}
	}
	if line == nil || line.GetTrap().GetName() != "Fosso Dourado" || len(line.GetTrap().GetCaught()) != 1 {
		t.Fatalf("Pensantus's player's log line = %v, want the Fosso Dourado that caught Toren", line)
	}
	if d := line.GetTrap().GetCaught()[0].GetDamages(); len(d) != 1 || d[0].GetRoll() != nil || d[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED || d[0].GetAmount() != 3 {
		t.Errorf("the damage line a stranger reads = %v, want 3 applied, with no dice", d)
	}
}

// TestMR035_AJumpFiresATrapOnlyWhereItLands: a jump that clears a trap's area fires
// nothing; one that lands in it fires it.
func TestMR035_AJumpFiresATrapOnlyWhereItLands(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		trapCol int
		want    bool
	}{
		"over the trap":     {trapCol: 10, want: false}, // from (8, 7) the jump goes to (11, 7)
		"landing in it":     {trapCol: 11, want: true},
		"short of the trap": {trapCol: 12, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newTrapRig(t)
			hole := r.trap(t, "Fosso Dourado", tc.trapCol, 7, pit("1d6"))
			r.fight(t)
			r.mustMove(t, r.caio, "Toren", 8, 7) // 10 ft: a running start
			res, err := r.moveResult(t, r.caio, "Toren", 11, 7, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
			if err != nil {
				t.Fatalf("a long jump to (11, 7) error = %v", err)
			}
			if got := byLabel(t, res.GetEncounter(), "Toren"); got.GetCol() != 11 {
				t.Fatalf("Toren after the jump = (%d, %d), want (11, 7)", got.GetCol(), got.GetRow())
			}
			fired := r.point(t, r.master, hole.GetId()).GetTrap().GetState() == mapsv1.TrapState_TRAP_STATE_TRIGGERED
			if fired != tc.want {
				t.Errorf("the trap fired = %v, want %v", fired, tc.want)
			}
		})
	}
}

// TestMR035_NPCsNeverFireATrapAndTheMasterPlacesOnlyWhereTheMoveEnds: an NPC walking
// into the area fires nothing; the master dragging a player's character onto it does.
func TestMR035_NPCsNeverFireATrapAndTheMasterPlacesOnlyWhereTheMoveEnds(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 12, 7, pit("1d6"))
	r.fight(t)
	r.mustMove(t, r.master, "Goblin 1", 12, 7)
	if got := r.point(t, r.master, hole.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_ARMED {
		t.Fatalf("the trap after a goblin walked into it = %v, want armed", got)
	}
	r.mustMove(t, r.master, "Toren", 12, 7)
	if got := r.point(t, r.master, hole.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_TRIGGERED {
		t.Errorf("the trap after the master put Toren on it = %v, want triggered", got)
	}
	// It fires once: moving on does nothing more.
	r.mustMove(t, r.master, "Toren", 6, 7)
	r.mustMove(t, r.master, "Toren", 12, 7)
	if n := r.eventCount(t, "trap_triggered"); n != 1 {
		t.Errorf("trap_triggered events = %d, want 1", n)
	}
}

// TestMR035_ACreatureNoticesWithItsOwnEyesAndFiresTheTrap: a creature of a player's
// character that ends a move near a trap notices it with its own passive Perception (a
// wolf's 13 meets a DC 13 that its owner's 10 does not), and the trap is revealed to
// the owner's character, whose player hears of it; walking into the area fires the
// trap, and the damage to the creature lands at once.
func TestMR035_ACreatureNoticesWithItsOwnEyesAndFiresTheTrap(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 9, 7, pit("1d6"), func(s *mapsv1.TrapSpec) { s.NoticeDc = 13 })
	r.wolfFight(t) // it is the wolf's turn: Presa is on (7, 7), 40 ft of speed
	r.wantKnows(t, "Toren's player", r.caio, hole.GetId(), false)
	r.mustMove(t, r.caio, "Presa", 8, 8) // 7,1 ft from the area: in range
	r.wantKnows(t, "Toren's player", r.caio, hole.GetId(), true)
	r.wantKnows(t, "Pensantus's player", r.ana, hole.GetId(), false)
	if rev := r.revealedTo(t, hole.GetId()); len(rev) != 1 || rev[0].GetCharacterId() != r.toren.GetId() {
		t.Errorf("revealed to %v, want Toren (the wolf's owner)", rev)
	}
	before := r.who(t, r.master, "Presa").GetHitPointsCurrent()
	r.h.roller.queue(3)
	res, err := r.moveResult(t, r.caio, "Presa", 9, 7) // into the area
	if err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	wolf := byLabel(t, res.GetEncounter(), "Presa")
	if wolf.GetCol() != 9 || wolf.GetRow() != 7 {
		t.Fatalf("the wolf after the move = (%d, %d), want it stopped on (9, 7)", wolf.GetCol(), wolf.GetRow())
	}
	if got := r.who(t, r.master, "Presa").GetHitPointsCurrent(); got != before-3 {
		t.Errorf("the wolf's hit points = %d, want %d: a creature takes the damage at once", got, before-3)
	}
	if d := r.trapDamages(t); len(d) != 0 {
		t.Errorf("trap damages = %v, want none waiting (a creature's lands at once)", d)
	}
}

// TestMR035_TheMasterFiresATrapByHand: the master fires any trap and picks who is
// caught; the server rolls each creature's saving throw with its bonus; the damage to
// an NPC lands at once and a condition goes on it, and the damage to a player's
// character waits for the master; a trap that fired cannot fire again, and an undo
// takes it all back.
func TestMR035_TheMasterFiresATrapByHand(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	statue := r.trap(t, "Estátua de Fogo", 15, 12, func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
		s.Effect = &rulesv1.TrapEffect{
			Save: &rulesv1.TrapSaveEffect{
				Ability: rulesv1.Ability_ABILITY_DEXTERITY, Dc: 12, AppliesTo: rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_CAUGHT,
				OnFail: &rulesv1.TrapOnFail{
					Damage:    []*rulesv1.TrapDamage{{Dice: "2d6", DamageTypeKey: "damage-type:fire"}},
					Condition: &rulesv1.TrapCondition{ConditionKey: "condition:poisoned"},
				},
				OnPass: rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_HALF,
			},
			Targets: rulesv1.TrapTargets_TRAP_TARGETS_MANUAL,
		}
	})
	e := r.fight(t)
	// Goblin 1 fails (d20 5 + 0 against 12) and takes 3 + 4 = 7, all of its hit points;
	// Toren passes (20 + 1) and takes half of 6 + 6 = 12.
	r.h.roller.queue(5, 3, 4, 20, 6, 6)
	res, err := r.fireByHand(t, statue, r.id(t, "Goblin 1"), r.id(t, "Toren"))
	if err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	caught := res.GetFiring().GetCaught()
	if res.GetFiring().GetName() != "Estátua de Fogo" || len(caught) != 2 {
		t.Fatalf("the firing = %v, want the statue and two creatures caught", res.GetFiring())
	}
	gob, tor := caught[0], caught[1]
	if gob.GetTargetLabel() != "Goblin 1" || gob.GetSave().GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED || gob.GetSave().GetDc() != 12 ||
		len(gob.GetDamages()) != 1 || gob.GetDamages()[0].GetAmount() != 7 || gob.GetDamages()[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED ||
		!gob.GetDamages()[0].GetTargetDefeated() || len(gob.GetConditionKeys()) != 1 {
		t.Errorf("the goblin's part = %v, want a failed save against DC 12, 7 fire damage applied at once (defeated) and poisoned", gob)
	}
	if tor.GetSave().GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED || tor.GetSave().GetRoll().GetTotal() != 22 || len(tor.GetDamages()) != 1 ||
		tor.GetDamages()[0].GetAmount() != 6 || !tor.GetDamages()[0].GetHalf() || tor.GetDamages()[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED ||
		len(tor.GetConditionKeys()) != 0 {
		t.Errorf("Toren's part = %v, want a save of 22, half of 12 = 6 waiting for the master, no condition", tor)
	}
	enc := r.get(t, r.master)
	goblin := byLabel(t, enc, "Goblin 1")
	if goblin.GetHitPointsCurrent() != 0 || !goblin.GetDefeated() || len(goblin.GetConditions()) != 1 || goblin.GetConditions()[0] != "condition:poisoned" {
		t.Errorf("Goblin 1 after the statue = %v, want 0 hit points, defeated, poisoned", goblin)
	}
	if got := r.point(t, r.ana, statue.GetId()); got == nil || got.GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_TRIGGERED {
		t.Errorf("Pensantus's player reads the fired statue as %v, want it public", got)
	}

	// It cannot fire twice.
	_, err = r.fireByHand(t, statue)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_NOT_ARMED)
	// A player never fires a trap.
	_, err = r.caio.play.FireTrap(t.Context(), connect.NewRequest(&playv1.FireTrapRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: statue.GetId(), IdempotencyKey: newKey()}))
	wantCode(t, "a player's FireTrap", err, connect.CodePermissionDenied)

	// One undo takes the whole firing back.
	r.undoLast(t)
	goblin = byLabel(t, r.get(t, r.master), "Goblin 1")
	if goblin.GetHitPointsCurrent() != 7 || goblin.GetDefeated() || len(goblin.GetConditions()) != 0 {
		t.Errorf("Goblin 1 after the undo = %v, want 7 hit points, up, no condition", goblin)
	}
	if d := r.trapDamages(t); len(d) != 0 {
		t.Errorf("trap damages after the undo = %v, want none", d)
	}
	if got := r.point(t, r.master, statue.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_ARMED {
		t.Errorf("the statue after the undo = %v, want armed", got)
	}
	_ = e
}

// TestMR035_TheAttack: the trap's attacks go round the creatures it caught, each
// against its armor class with the trap's bonus; a natural 20 doubles the damage
// dice and a natural 1 misses.
func TestMR035_TheAttack(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	darts := r.trap(t, "Dardos", 15, 12, func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
		s.Effect = &rulesv1.TrapEffect{
			Attack:  &rulesv1.TrapAttack{Bonus: 12, Count: 2, Damage: &rulesv1.TrapDamage{Dice: "1d6", DamageTypeKey: "damage-type:piercing"}},
			Targets: rulesv1.TrapTargets_TRAP_TARGETS_MANUAL,
		}
	})
	r.fight(t)
	r.h.roller.queue(20, 3, 3, 1) // the first dart is a natural 20 (2d6: 3 + 3), the second a natural 1
	res, err := r.fireByHand(t, darts, r.id(t, "Toren"), r.id(t, "Pensantus"))
	if err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	caught := res.GetFiring().GetCaught()
	if len(caught) != 2 || len(caught[0].GetAttacks()) != 1 || len(caught[1].GetAttacks()) != 1 {
		t.Fatalf("the firing = %v, want one dart at each", res.GetFiring())
	}
	if a := caught[0].GetAttacks()[0]; a.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT || a.GetRoll().GetTotal() != 32 || a.GetTargetArmorClass() == 0 {
		t.Errorf("the first dart = %v, want a critical hit (20 + 12) and the target's armor class for the master", a)
	}
	if d := caught[0].GetDamages(); len(d) != 1 || d[0].GetAmount() != 6 || d[0].GetRoll().GetDiceCount() != 2 || !d[0].GetCritical() {
		t.Errorf("the critical damage = %v, want 6 from 2d6, critical", d)
	}
	if a := caught[1].GetAttacks()[0]; a.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || len(caught[1].GetDamages()) != 0 {
		t.Errorf("the second dart = %v, damages %v, want a miss with no damage", a, caught[1].GetDamages())
	}
}

// TestMR035_OutsideACombat: with no combat on the map, a fired trap's damage to a
// player's character waits for the master as a trap damage, applied to the vitals
// (changed first, if he wants); an NPC's is only a line of the history; the
// conditions are a reminder.
func TestMR035_OutsideACombat(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 12, 7, pit("2d6"), func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
		s.Effect.Conditions = []*rulesv1.TrapCondition{{ConditionKey: "condition:prone"}}
	})
	r.place(t, r.pens.GetId(), 12, 7)
	r.place(t, r.goblins.GetId(), 12, 7)
	r.h.roller.queue(3, 4, 2, 2) // Pensantus 3 + 4, the goblin 2 + 2
	res, err := r.fireByHand(t, hole)
	if err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	if n := len(res.GetFiring().GetCaught()); n != 2 {
		t.Fatalf("caught = %d, want Pensantus and the goblin (the tokens in the area)", n)
	}
	pens, gob := res.GetFiring().GetCaught()[0], res.GetFiring().GetCaught()[1]
	if pens.GetTargetLabel() != "Pensantus" || pens.GetDamages()[0].GetAmount() != 7 || pens.GetDamages()[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED ||
		gob.GetDamages()[0].GetAmount() != 4 || gob.GetDamages()[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED ||
		len(pens.GetConditionKeys()) != 1 || len(gob.GetConditionKeys()) != 1 {
		t.Errorf("the firing = %v, want 7 waiting on Pensantus, 4 on the goblin, a condition each", res.GetFiring())
	}
	damages := r.trapDamages(t)
	if len(damages) != 1 || damages[0].GetAmount() != 7 || damages[0].GetEncounterId() != "" || damages[0].GetCharacterName() != "Pensantus" {
		t.Fatalf("the trap damages = %v, want 7 on Pensantus, outside a combat", damages)
	}
	before := r.vitals(t, r.pens).GetHitPointsCurrent()
	if got := r.vitals(t, r.pens).GetHitPointsCurrent(); got != before {
		t.Fatalf("the damage landed before the master applied it")
	}
	// Only the master settles it.
	_, err = r.ana.play.ApplyTrapDamage(t.Context(), connect.NewRequest(&playv1.ApplyTrapDamageRequest{CampaignId: r.campaignID, TrapDamageId: damages[0].GetId(), IdempotencyKey: newKey()}))
	wantCode(t, "a player's ApplyTrapDamage", err, connect.CodePermissionDenied)
	key := newKey()
	apply := func() (*playv1.ApplyTrapDamageResponse, error) {
		res, err := r.master.play.ApplyTrapDamage(t.Context(), connect.NewRequest(&playv1.ApplyTrapDamageRequest{
			CampaignId: r.campaignID, TrapDamageId: damages[0].GetId(), IdempotencyKey: key, Amount: proto.Int32(2),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	if done, err := apply(); err != nil || done.GetDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED || done.GetDamage().GetAppliedAmount() != 2 {
		t.Fatalf("ApplyTrapDamage() = %v, %v; want applied, changed to 2", done, err)
	}
	if got := r.vitals(t, r.pens).GetHitPointsCurrent(); got != before-2 {
		t.Errorf("Pensantus's hit points = %d, want %d", got, before-2)
	}
	if _, err := apply(); err != nil { // a retry with the same key changes nothing
		t.Errorf("the retry error = %v, want the first answer", err)
	}
	if got := r.vitals(t, r.pens).GetHitPointsCurrent(); got != before-2 {
		t.Errorf("after the retry Pensantus's hit points = %d, want %d", got, before-2)
	}
	_, err = r.master.play.ApplyTrapDamage(t.Context(), connect.NewRequest(&playv1.ApplyTrapDamageRequest{CampaignId: r.campaignID, TrapDamageId: damages[0].GetId(), IdempotencyKey: newKey()}))
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED)
	if d := r.trapDamages(t); len(d) != 0 {
		t.Errorf("trap damages after the apply = %v, want none", d)
	}
}

// TestMR035_ATokenDroppedInTheArea: with no combat on the map, the master dropping
// a player's character's token inside the area of an armed "Ao entrar na área" trap
// fires it; an NPC's token never does. The damage waits for the master.
func TestMR035_ATokenDroppedInTheArea(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 12, 7, pit("1d6"))
	r.place(t, r.goblins.GetId(), 12, 7)
	if got := r.point(t, r.master, hole.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_ARMED {
		t.Fatalf("the trap after an NPC token = %v, want armed", got)
	}
	r.h.roller.queue(5)
	r.place(t, r.pens.GetId(), 12, 7)
	if got := r.point(t, r.master, hole.GetId()).GetTrap().GetState(); got != mapsv1.TrapState_TRAP_STATE_TRIGGERED {
		t.Fatalf("the trap after Pensantus's token = %v, want triggered", got)
	}
	// The NPC's token was in the area too: it is caught with her (no damage kept for it).
	damages := r.trapDamages(t)
	if len(damages) != 1 || damages[0].GetCharacterName() != "Pensantus" || damages[0].GetAmount() != 5 {
		t.Errorf("the trap damages = %v, want 5 on Pensantus", damages)
	}
	r.wantKnows(t, "Brisa's player", r.bia, hole.GetId(), true) // it fired: public
}

// TestMR035_ADisarmedTrapNeverFires: the master marks a trap "Desarmada" (an event
// goes into the history) and it fires no more, by walking or by hand; a disarmed
// trap that nobody knew stays hidden.
func TestMR035_ADisarmedTrapNeverFires(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 9, 7, pit("1d6"))
	r.fight(t)
	res, err := r.mc(r.master).DisarmTrap(t.Context(), connect.NewRequest(&mapsv1.DisarmTrapRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: hole.GetId()}))
	if err != nil || res.Msg.GetPoint().GetTrap().GetState() != mapsv1.TrapState_TRAP_STATE_DISARMED {
		t.Fatalf("DisarmTrap() = %v, %v; want it disarmed", res, err)
	}
	if n := r.eventCount(t, "trap_disarmed"); n != 1 {
		t.Errorf("trap_disarmed events = %d, want 1", n)
	}
	r.wantKnows(t, "Pensantus's player", r.ana, hole.GetId(), false)
	got, err := r.moveResult(t, r.caio, "Toren", 11, 7)
	if err != nil || byLabel(t, got.GetEncounter(), "Toren").GetCol() != 11 || got.GetStoppedEarly() {
		t.Fatalf("a move through the disarmed trap = %v, %v; want it to go on", got, err)
	}
	_, err = r.fireByHand(t, hole)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_NOT_ARMED)
	if again, err := r.mc(r.master).DisarmTrap(t.Context(), connect.NewRequest(&mapsv1.DisarmTrapRequest{CampaignId: r.campaignID, MapId: r.mapID, PointId: hole.GetId()})); err != nil || again == nil {
		t.Errorf("disarming it again = %v, %v; want nothing to change", again, err)
	}
	if n := r.eventCount(t, "trap_disarmed"); n != 1 {
		t.Errorf("trap_disarmed events after disarming again = %d, want 1", n)
	}
}

// TestMR035_TheWarningBeforeSteppingIn: GetMoveOptions marks the squares whose move
// would run into an armed trap the character knows, and never a trap it does not.
func TestMR035_TheWarningBeforeSteppingIn(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc = 20 })
	r.fight(t)
	marked := func(u *user) map[[2]int32]string {
		t.Helper()
		opts, err := r.options(t, u, "Toren")
		if err != nil {
			t.Fatalf("GetMoveOptions() error = %v", err)
		}
		out := map[[2]int32]string{}
		for _, s := range opts.GetReachable() {
			if s.GetKnownTrapPointId() != "" {
				out[[2]int32{s.GetCol(), s.GetRow()}] = s.GetKnownTrapName()
				if s.GetKnownTrapPointId() != hole.GetId() {
					t.Errorf("a square marked with the trap %s, want %s", s.GetKnownTrapPointId(), hole.GetId())
				}
			}
		}
		return out
	}
	if m := marked(r.caio); len(m) != 0 {
		t.Fatalf("squares marked before the character knows the trap = %v, want none (RN-10)", m)
	}
	if js, _ := protojson.Marshal(mustOptionsOf(t, r, r.caio)); strings.Contains(string(js), "Fosso") {
		t.Fatalf("the move options of a player who does not know the trap name it: %s", js)
	}
	if _, err := r.mc(r.master).RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{
		CampaignId: r.campaignID, MapId: r.mapID, PointId: hole.GetId(), CharacterIds: []string{r.toren.GetId()},
	})); err != nil {
		t.Fatalf("RevealTrap() error = %v", err)
	}
	m := marked(r.caio)
	if m[[2]int32{9, 7}] != "Fosso Dourado" || m[[2]int32{11, 7}] != "Fosso Dourado" {
		t.Errorf("marked squares = %v, want (9, 7) and the ones past it on the line, named", m)
	}
	if _, ok := m[[2]int32{8, 7}]; ok {
		t.Errorf("the square before the trap is marked: %v", m)
	}
	if _, ok := m[[2]int32{9, 12}]; ok {
		t.Errorf("a square off the trap's line is marked: %v", m)
	}
}

func mustOptionsOf(t *testing.T, r *trapRig, u *user) *playv1.GetMoveOptionsResponse {
	t.Helper()
	opts, err := r.options(t, u, "Toren")
	if err != nil {
		t.Fatalf("GetMoveOptions() error = %v", err)
	}
	return opts
}

// TestRN10_NoPlayerResponseNamesAnUnrevealedTrap: through a whole scene (a search that
// finds nothing, another that finds the trap for one player, the combat, the master
// revealing it to a second player), nothing a player gets as the app's JSON (the map,
// the combat, the move options, the log, the live session, the stream) names a trap
// their characters do not know, carries its id, or a DC. Brisa's player never learns
// of it; Pensantus's player is held to it until the master reveals it to her.
func TestRN10_NoPlayerResponseNamesAnUnrevealedTrap(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	secret := r.trap(t, "Fosso Dourado Secreto", 9, 7, func(s *mapsv1.TrapSpec) {
		s.NoticeDc, s.FindDc = 12, 14
		s.Effect = &rulesv1.TrapEffect{Damage: []*rulesv1.TrapDamage{{Dice: "1d6", DamageTypeKey: "damage-type:bludgeoning"}}}
	})
	brisaStream := r.watch(t, r.bia, r.campaignID)
	pensStream := r.watch(t, r.ana, r.campaignID)

	// strict says who must not receive the trap now.
	strict := map[string]bool{"Brisa's player": true, "Pensantus's player": true}
	check := func(who, label string, v proto.Message, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s %s: %v", who, label, err)
		}
		js, err := protojson.Marshal(v)
		if err != nil {
			t.Fatalf("%s %s: %v", who, label, err)
		}
		if !strict[who] {
			return
		}
		for _, bad := range []string{"Fosso Dourado Secreto", secret.GetId(), "noticeDc", "findDc", "descrição de Fosso"} {
			if strings.Contains(string(js), bad) {
				t.Errorf("%s %s leaks %q: %.400s", who, label, bad, js)
			}
		}
	}
	readAll := func(who string, u *user) {
		t.Helper()
		m, err := r.mc(u).GetMap(t.Context(), connect.NewRequest(&mapsv1.GetMapRequest{CampaignId: r.campaignID, MapId: r.mapID}))
		check(who, "GetMap", msgOf(m), err)
		l, err := r.mc(u).ListMaps(t.Context(), connect.NewRequest(&mapsv1.ListMapsRequest{CampaignId: r.campaignID}))
		check(who, "ListMaps", msgOf(l), err)
		lay, err := r.mc(u).GetMapLayers(t.Context(), connect.NewRequest(&mapsv1.GetMapLayersRequest{CampaignId: r.campaignID, MapId: r.mapID}))
		check(who, "GetMapLayers", msgOf(lay), err)
		live, err := u.play.GetLiveSession(t.Context(), connect.NewRequest(&playv1.GetLiveSessionRequest{CampaignId: r.campaignID}))
		check(who, "GetLiveSession", msgOf(live), err)
	}
	readCombat := func(who string, u *user) {
		t.Helper()
		enc, err := u.combat.GetEncounter(t.Context(), connect.NewRequest(&playv1.GetEncounterRequest{CampaignId: r.campaignID}))
		check(who, "GetEncounter", msgOf(enc), err)
		if err != nil || enc.Msg.GetEncounter() == nil {
			return
		}
		lg, err := u.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: r.campaignID, EncounterId: enc.Msg.GetEncounter().GetId()}))
		check(who, "ListCombatLog", msgOf(lg), err)
	}
	readStream := func(who string, w *watcher) {
		t.Helper()
		time.Sleep(300 * time.Millisecond) // the stream's events are delivered in the background
		for _, ev := range r.drain(w) {
			check(who, "stream", ev, nil)
		}
	}
	both := func() {
		t.Helper()
		readAll("Pensantus's player", r.ana)
		readAll("Brisa's player", r.bia)
	}

	// Before the combat: a search that finds nothing, and one that reveals it to Toren only.
	r.place(t, r.toren.GetId(), 8, 7)
	r.place(t, r.pens.GetId(), 3, 3)
	r.place(t, r.bri.GetId(), 3, 4)
	miss, err := r.search(t, r.ana, investigation, 12) // far from the trap: nothing to find
	check("Pensantus's player", "search", miss, err)
	both()
	found, err := r.search(t, r.caio, perception, 14) // 14 meets the DC 12
	if err != nil || len(found.GetFoundPointIds()) != 1 {
		t.Fatalf("Toren's search = %v, %v; want the trap", found, err)
	}
	r.wantKnows(t, "Toren's player", r.caio, secret.GetId(), true)
	both()

	e := r.fight(t)
	both()
	readCombat("Pensantus's player", r.ana)
	readCombat("Brisa's player", r.bia)
	r.mustMove(t, r.caio, "Toren", 8, 7) // Toren knows it: he stays out of the area
	readCombat("Pensantus's player", r.ana)
	// It is Brisa's turn after Toren's and Pensantus's: she is the mover, so her move
	// options and her move are read, far from the pit.
	e = r.mustEndTurn(t, r.caio, e)
	r.mustEndTurn(t, r.ana, e)
	opts, err := r.options(t, r.bia, "Brisa")
	if err != nil {
		t.Fatalf("GetMoveOptions() as Brisa error = %v", err)
	}
	if len(opts.GetReachable()) == 0 {
		t.Fatalf("Brisa's move options are empty: the sweep read nothing")
	}
	check("Brisa's player", "GetMoveOptions", opts, nil)
	mv, err := r.moveResult(t, r.bia, "Brisa", 5, 7)
	check("Brisa's player", "MoveCombatant", mv, err)
	readCombat("Brisa's player", r.bia)
	readStream("Pensantus's player", pensStream)

	// The master reveals it to Pensantus: from now on only Brisa's player is held to it.
	if _, err := r.mc(r.master).RevealTrap(t.Context(), connect.NewRequest(&mapsv1.RevealTrapRequest{
		CampaignId: r.campaignID, MapId: r.mapID, PointId: secret.GetId(), CharacterIds: []string{r.pens.GetId()},
	})); err != nil {
		t.Fatalf("RevealTrap() error = %v", err)
	}
	strict["Pensantus's player"] = false
	r.wantKnows(t, "Pensantus's player after the reveal", r.ana, secret.GetId(), true)
	r.wantKnows(t, "Brisa's player after the others learned of it", r.bia, secret.GetId(), false)
	both()
	readCombat("Brisa's player", r.bia)
	readStream("Brisa's player", brisaStream)
}

// TestRN10_AMoveStoppedByAnUnknownTrapReadsAsTheTrapFiring: a player whose character
// did not know the trap walks into it: the answer says only that the move was cut
// short, and the trap is public from then on (it fired), so that is no leak.
func TestRN10_AMoveStoppedByAnUnknownTrapReadsAsTheTrapFiring(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 11, 7, pit("1d6"), func(s *mapsv1.TrapSpec) { s.NoticeDc = 25 })
	r.fight(t)
	res, err := r.moveResult(t, r.caio, "Toren", 12, 7)
	if err != nil || !res.GetStoppedEarly() || byLabel(t, res.GetEncounter(), "Toren").GetCol() != 11 {
		t.Fatalf("the move = %v, %v; want it cut short on (11, 7)", res, err)
	}
	if js, _ := protojson.Marshal(res); strings.Contains(string(js), "Fosso") || strings.Contains(string(js), hole.GetId()) {
		t.Errorf("the move's answer names the trap: %s", js)
	}
	r.wantKnows(t, "Brisa's player after it fired", r.bia, hole.GetId(), true)
}

// msgOf is the message of an answer, nil when the call failed.
func msgOf[T any, PT interface {
	*T
	proto.Message
}](res *connect.Response[T]) proto.Message {
	if res == nil {
		return nil
	}
	return PT(res.Msg)
}

// watch opens the stream of a player.
func (r *trapRig) watch(t *testing.T, u *user, campaignID string) *watcher {
	t.Helper()
	w := u.watch(t, campaignID)
	w.ready(t)
	return w
}

// drain returns what the stream has delivered so far.
func (r *trapRig) drain(w *watcher) []*playv1.WatchGameSessionResponse {
	var out []*playv1.WatchGameSessionResponse
	for {
		select {
		case ev, ok := <-w.events:
			if !ok {
				return out
			}
			if ev.GetHeartbeat() == nil {
				out = append(out, ev)
			}
		default:
			return out
		}
	}
}

// wantSceneBlocked checks the reason of a SceneBlocked failed_precondition.
func wantSceneBlocked(t *testing.T, err error, want playv1.SceneBlockedReason) {
	t.Helper()
	cerr, ok := errors.AsType[*connect.Error](err)
	if !ok || cerr.Code() != connect.CodeFailedPrecondition {
		t.Fatalf("error = %v, want failed_precondition", err)
	}
	for _, d := range cerr.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*playv1.SceneBlocked); ok && b.GetReason() == want {
				return
			}
		}
	}
	t.Fatalf("error %v has no SceneBlocked detail with the reason %v", err, want)
}

// Applying a trap damage to a combatant of the running combat changes its vitals, so the
// combat's revision goes up and the players are told, as a correction of the vitals does.
func TestApplyingTrapDamageInACombatRaisesItsRevision(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 12, 7, pit("2d6"), func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
	})
	r.place(t, r.pens.GetId(), 12, 7)
	r.h.roller.queue(3, 4)
	if _, err := r.fireByHand(t, hole); err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	damages := r.trapDamages(t)
	if len(damages) != 1 || damages[0].GetEncounterId() != "" {
		t.Fatalf("trap damages = %v, want one outside a combat", damages)
	}
	// A combat starts later with Pensantus as a combatant.
	r.fight(t)
	w := r.watch(t, r.ana, r.campaignID)
	r.drain(w)
	before := r.get(t, r.master).GetRevision()
	hp := r.vitals(t, r.pens).GetHitPointsCurrent()
	if _, err := r.master.play.ApplyTrapDamage(t.Context(), connect.NewRequest(&playv1.ApplyTrapDamageRequest{
		CampaignId: r.campaignID, TrapDamageId: damages[0].GetId(), IdempotencyKey: newKey(), Amount: proto.Int32(hp + 50),
	})); err != nil {
		t.Fatalf("ApplyTrapDamage() error = %v", err)
	}
	if got := r.vitals(t, r.pens).GetHitPointsCurrent(); got != 0 {
		t.Fatalf("Pensantus HP = %d, want 0", got)
	}
	e := r.get(t, r.master)
	if got := byLabel(t, e, "Pensantus").GetState(); got != playv1.CombatantState_COMBATANT_STATE_DOWN {
		t.Errorf("Pensantus state = %v, want DOWN", got)
	}
	if e.GetRevision() <= before {
		t.Errorf("encounter revision after = %d, before = %d; want it raised (as AdjustCharacterVitals does)", e.GetRevision(), before)
	}
	seen := false
	for _, ev := range r.drain(w) {
		if ev.GetEncounterChanged() != nil {
			seen = true
		}
	}
	if !seen {
		t.Errorf("no encounter_changed event reached the player's stream after ApplyTrapDamage dropped a combatant to 0")
	}
}
