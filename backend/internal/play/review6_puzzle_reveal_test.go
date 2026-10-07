package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Finding U6-F: a puzzle whose "Ao resolver" reveals a LIGHT point (never visible
// to players, RN-10) writes "NAME apareceu no mapa." for every player to read, so
// the name of a point that stays hidden leaks.
func TestReview6_PuzzleRevealDoesNotNameAHiddenPoint(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	const secret = "TOCHA-SECRETA-U6F"
	hiddenMap := p.h.newMap(p.campaignID, gridColumns) // not current, never revealed
	res, err := p.mc(p.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: p.campaignID, MapId: hiddenMap, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: secret,
		XBp: 2500, YBp: 2500,
	}))
	if err != nil {
		t.Fatalf("CreateMapPoint() error = %v", err)
	}
	puz := p.oneMoveLock(t, &playv1.PuzzleOnSolve{
		Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT,
		Target: &playv1.PuzzleOnSolve_Point{Point: &playv1.PuzzlePointTarget{MapId: hiddenMap, PointId: res.Msg.GetPoint().GetId()}},
	})
	var rev *string
	if err := p.h.pool.QueryRow(t.Context(), `SELECT revealed_at::TEXT FROM maps WHERE id = $1`, hiddenMap).Scan(&rev); err != nil || rev != nil {
		t.Fatalf("the map must be hidden: %v %v", err, rev)
	}
	move := p.mustMove(t, p.caio, puz.GetId(), lockMove(0, 1))
	var seen []string
	seen = append(seen, "move: "+jsonOf(move))
	for _, u := range []*user{p.caio, p.ana, p.bia} {
		seen = append(seen, "read: "+jsonOf(p.read(t, u, puz.GetId())))
	}
	var sid string
	if err := p.h.pool.QueryRow(t.Context(), `SELECT id::STRING FROM game_sessions WHERE campaign_id = $1 LIMIT 1`, p.campaignID).Scan(&sid); err != nil {
		t.Fatalf("read the session: %v", err)
	}
	rows, err := p.h.pool.Query(t.Context(), `SELECT kind, payload::STRING FROM session_events WHERE game_session_id = $1 ORDER BY seq`, sid)
	if err != nil {
		t.Fatalf("read session_events: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind, payload string
		if err := rows.Scan(&kind, &payload); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, "event "+kind+": "+payload)
	}
	for _, s := range seen {
		if strings.Contains(s, secret) {
			t.Errorf("a player can read the hidden point's name: %s", s)
		}
	}
}
