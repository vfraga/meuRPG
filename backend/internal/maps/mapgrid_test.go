package maps

import (
	"context"
	"slices"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// SetMapGrid is frozen here before its phases are split (MR-013, MR-025, RN-24):
// the order of its refusals, the fog rule that meets a map's first grid, and the
// copy of the image that rule makes for nothing when the map changed meanwhile.
// The calibration itself (same grid, scale, clear) is frozen by TestMR025_*.

// gridRefusal is what a refused SetMapGrid says.
type gridRefusal struct {
	code    connect.Code
	message string
}

func wantGridRefusal(t *testing.T, call string, err error, want gridRefusal) {
	t.Helper()
	wantRefusal(t, call, err, want.code, want.message)
}

func (u *user) setGridWith(campaignID, mapID string, columns, factor int32) error {
	_, err := u.maps.SetMapGrid(u.h.t.Context(), connect.NewRequest(&mapsv1.SetMapGridRequest{
		CampaignId: campaignID, MapId: mapID, Columns: columns, SquareFactor: factor,
	}))
	return err
}

// Who may call it and what each bad request is told, in the order the checks run:
// the role, the id, the columns, the factor, and only then the map itself.
func TestSetMapGrid_Refusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	outsider := s.h.newUser("De fora")
	const (
		columnsMsg = "columns must be 0 (no grid) or 4 to 200"
		factorMsg  = "square_factor must be 1 to 20 (0 reads as 1), and only with columns"
		limitMsg   = "11 columns of 20 squares each make a grid that passes the limit of 200 x 400 squares"
	)
	invalid := func(msg string) gridRefusal { return gridRefusal{connect.CodeInvalidArgument, msg} }
	notFound := gridRefusal{connect.CodeNotFound, "map not found"}
	unknownMap := uuid.New().String()
	other := s.h.newCampaign(outsider)
	foreign := outsider.createMap(other, "De outra mesa", outsider.newImage(other)).GetId()

	for _, tt := range []struct {
		name     string
		user     *user
		campaign string
		mapID    string
		columns  int32
		factor   int32
		code     connect.Code // when message is empty: only the code is frozen
		want     gridRefusal
	}{
		{"a player", s.ana, s.campaign, s.mapID, 20, 1, connect.CodePermissionDenied, gridRefusal{}},
		{"a player with a bad id and bad columns", s.ana, s.campaign, "x", 3, -1, connect.CodePermissionDenied, gridRefusal{}},
		{"a stranger to the campaign", outsider, s.campaign, s.mapID, 20, 1, connect.CodeNotFound, gridRefusal{}},
		{"an id that is not a uuid", m, s.campaign, "nope", 20, 1, 0, notFound},
		{"an empty id", m, s.campaign, "", 20, 1, 0, notFound},
		{"an id that is not a uuid, with bad columns", m, s.campaign, "nope", 3, 1, 0, notFound},
		{"a map that does not exist", m, s.campaign, unknownMap, 20, 1, 0, notFound},
		{"a map of another campaign", m, s.campaign, foreign, 20, 1, 0, notFound},
		{"3 columns", m, s.campaign, s.mapID, 3, 1, 0, invalid(columnsMsg)},
		{"201 columns", m, s.campaign, s.mapID, 201, 1, 0, invalid(columnsMsg)},
		{"negative columns", m, s.campaign, s.mapID, -4, 1, 0, invalid(columnsMsg)},
		{"bad columns before a bad factor", m, s.campaign, s.mapID, 3, -1, 0, invalid(columnsMsg)},
		{"bad columns on a map that does not exist", m, s.campaign, unknownMap, 3, 1, 0, invalid(columnsMsg)},
		{"a negative factor", m, s.campaign, s.mapID, 20, -1, 0, invalid(factorMsg)},
		{"a factor over the largest", m, s.campaign, s.mapID, 20, grid.MaxFactor + 1, 0, invalid(factorMsg)},
		{"a factor without columns", m, s.campaign, s.mapID, 0, 2, 0, invalid(factorMsg)},
		{"a bad factor on a map that does not exist", m, s.campaign, unknownMap, 20, -1, 0, invalid(factorMsg)},
		{"a grid past the limit", m, s.campaign, s.mapID, 11, 20, 0, invalid(limitMsg)},
		{"a grid past the limit, on a map that does not exist", m, s.campaign, unknownMap, 11, 20, 0, notFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.user.setGridWith(tt.campaign, tt.mapID, tt.columns, tt.factor)
			if tt.want.message == "" {
				wantCode(t, tt.name, err, tt.code)
				return
			}
			wantGridRefusal(t, tt.name, err, tt.want)
		})
	}
	if got := m.mustGetMap(s.campaign, s.mapID).GetMap(); got.GetGridColumns() != 0 {
		t.Errorf("a refused call left %d columns on the map", got.GetGridColumns())
	}
}

// Sending the factor 0 reads as 1, and the answer is the master's view of the map
// just written: what a later read says too.
func TestSetMapGrid_TheAnswerIsTheMastersMap(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	for _, tt := range []struct {
		columns, factor         int32
		wantColumns, wantFactor int32
		wantDrawn, wantRows     int32
	}{
		{20, 0, 20, 1, 20, 15},
		{20, 3, 60, 3, 20, 45},
		{200, 1, 200, 1, 200, 150},
		{4, 20, 80, 20, 4, 60},
		{0, 0, 0, 1, 0, 0},
		{0, 1, 0, 1, 0, 0},
	} {
		got := m.mustSetCalibration(s.campaign, s.mapID, tt.columns, tt.factor)
		if got.GetGridColumns() != tt.wantColumns || got.GetSquareFactor() != tt.wantFactor || got.GetDrawnColumns() != tt.wantDrawn || got.GetGridRows() != tt.wantRows {
			t.Errorf("SetMapGrid(%d, factor %d) = %d x %d, factor %d, drawn %d; want %d x %d, factor %d, drawn %d",
				tt.columns, tt.factor, got.GetGridColumns(), got.GetGridRows(), got.GetSquareFactor(), got.GetDrawnColumns(),
				tt.wantColumns, tt.wantRows, tt.wantFactor, tt.wantDrawn)
		}
		if read := m.mustGetMap(s.campaign, s.mapID).GetMap(); !proto.Equal(read, got) {
			t.Errorf("SetMapGrid(%d, factor %d) answered %v, a read says %v", tt.columns, tt.factor, got, read)
		}
	}
}

// RN-24, the fog rule at the first grid, on a map the table's rule marked: the
// image another use needs is copied (the players never get the raw one, RN-10);
// an image nothing else uses is kept as it is; both turn the fog on once.
func TestSetMapGrid_TheFogRuleAtTheFirstGrid(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Mestre"), h.newUser("Ana")
	campaign := h.newCampaign(master, ana)
	master.setNewMapFog(campaign, true)
	shared := master.newImage(campaign)
	master.createMap(campaign, "Outro mapa", shared) // makes the image shared
	own := master.newImage(campaign)

	files := len(h.storedFiles())
	withOwn := master.createMap(campaign, "Imagem própria", own)
	got := master.mustSetGrid(campaign, withOwn.GetId(), 20)
	if !got.GetFogEnabled() || got.GetImage().GetId() != own || len(h.storedFiles()) != files {
		t.Errorf("an image only the map uses: fog %v, image %q (want %q), %d files (want %d)",
			got.GetFogEnabled(), got.GetImage().GetId(), own, len(h.storedFiles()), files)
	}

	withShared := master.createMap(campaign, "Imagem comum", shared)
	got = master.mustSetGrid(campaign, withShared.GetId(), 20)
	if !got.GetFogEnabled() || got.GetImage().GetId() == shared || len(h.storedFiles()) != files+2 {
		t.Errorf("an image another map uses: fog %v, image %q (the shared one is %q), %d files (want %d: the copy and its thumbnail)",
			got.GetFogEnabled(), got.GetImage().GetId(), shared, len(h.storedFiles()), files+2)
	}
	if again := master.mustSetGrid(campaign, withShared.GetId(), 30); again.GetImage().GetId() != got.GetImage().GetId() {
		t.Error("a second grid changed the fog map's image again")
	}

	// A map the rule did not mark has no copy made, shared image or not.
	master.setNewMapFog(campaign, false)
	plain := master.createMap(campaign, "Sem névoa", shared)
	if got := master.mustSetGrid(campaign, plain.GetId(), 20); got.GetFogEnabled() || got.GetImage().GetId() != shared {
		t.Errorf("a map the rule did not mark: fog %v, image %q", got.GetFogEnabled(), got.GetImage().GetId())
	}
	// Clearing the grid of a map that has the fog turns it off and spends nothing else.
	if cleared := master.mustSetGrid(campaign, withShared.GetId(), 0); cleared.GetFogEnabled() {
		t.Error("a map with no grid keeps the fog on")
	}
}

// The copy is made before the transaction, from what the map was then. When the
// map's image changed meanwhile, the copy is of no use: it is deleted, the grid is
// set and the fog rule is applied all the same (finding, frozen: the map then has
// fog on with the image the other writer gave it, which this call did not copy).
func TestSetMapGrid_ACopyOfNoUseIsDeleted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	master.setNewMapFog(campaign, true)
	shared := master.newImage(campaign)
	master.createMap(campaign, "Outro mapa", shared)
	swapped := master.newImage(campaign)
	m := master.createMap(campaign, "Imagem comum", shared)
	files := h.storedFiles()

	h.hookAfterQuery("-- name: GetMap :one", func() {
		if _, err := h.pool.Exec(context.Background(), "UPDATE maps SET image_id = $1 WHERE id = $2", swapped, m.GetId()); err != nil {
			t.Errorf("swap the image from the hook: %v", err)
		}
	})
	got := master.mustSetGrid(campaign, m.GetId(), 20)
	if got.GetImage().GetId() != swapped || !got.GetFogEnabled() || got.GetGridColumns() != 20 {
		t.Errorf("after the swap: image %q (want %q), fog %v, %d columns", got.GetImage().GetId(), swapped, got.GetFogEnabled(), got.GetGridColumns())
	}
	if now := h.storedFiles(); !slices.Equal(now, files) {
		t.Errorf("the copy made for nothing stayed in the store: %v, want %v", now, files)
	}
}
