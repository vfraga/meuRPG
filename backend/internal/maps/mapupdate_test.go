package maps

import (
	"slices"
	"testing"
	"uuid"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// UpdateMap is frozen here before its phases are split (MR-008, AIP-154, RN-10,
// MR-025, D2): the order of its refusals, what a rename and an image swap do, the
// copy of the image a fog map takes, and who hears about it.

// updateRefusal is what a refused UpdateMap says.
type updateRefusal struct {
	code    connect.Code
	message string
}

func wantUpdateRefusal(t *testing.T, call string, err error, want updateRefusal) {
	t.Helper()
	wantRefusal(t, call, err, want.code, want.message)
}

func (u *user) updateMapWith(req *mapsv1.UpdateMapRequest) (*mapsv1.Map, error) {
	res, err := u.maps.UpdateMap(u.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetMap(), nil
}

// Who may call it and what each bad request is told, in the order the checks run:
// the role, the map's id, whether anything is set, the revision, the name, the
// image's id, and only then (inside the transaction) the map, its revision and
// the image.
func TestUpdateMap_Refusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	outsider := s.h.newUser("De fora")
	other := s.h.newCampaign(outsider)
	foreignMap := outsider.createMap(other, "De outra mesa", outsider.newImage(other)).GetId()
	foreignImage := outsider.newImage(other)
	cur := m.mustGetMap(s.campaign, s.mapID).GetMap()
	rev := cur.GetRevision()
	unknown := uuid.New().String()

	const (
		nothingMsg  = "set name, image_id or both"
		revisionMsg = "revision must be at least 1"
		emptyMsg    = "name must not be empty"
		imageMsg    = "image_id is not an image of the campaign's gallery"
		staleMsg    = "the map changed since you opened it; reload it and try again"
	)
	invalid := func(msg string) updateRefusal { return updateRefusal{connect.CodeInvalidArgument, msg} }
	notFound := updateRefusal{connect.CodeNotFound, "map not found"}
	stale := updateRefusal{connect.CodeAborted, staleMsg}
	name := func(s string) *string { return &s }

	for _, tt := range []struct {
		name string
		user *user
		req  *mapsv1.UpdateMapRequest
		code connect.Code // when want is empty: only the code is frozen
		want updateRefusal
	}{
		{"a player", s.ana, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev, Name: name("X")}, connect.CodePermissionDenied, updateRefusal{}},
		{"a player with nothing valid", s.ana, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: "x"}, connect.CodePermissionDenied, updateRefusal{}},
		{"a stranger to the campaign", outsider, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev, Name: name("X")}, connect.CodeNotFound, updateRefusal{}},
		{"an id that is not a uuid", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: "nope", Revision: rev, Name: name("X")}, 0, notFound},
		{"an id that is not a uuid, with nothing to change", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: "nope"}, 0, notFound},
		{"nothing to change", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev}, 0, invalid(nothingMsg)},
		{"nothing to change with revision 0", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID}, 0, invalid(nothingMsg)},
		{"revision 0", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Name: name("X")}, 0, invalid(revisionMsg)},
		{"a negative revision", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: -2, Name: name("X")}, 0, invalid(revisionMsg)},
		{"revision 0 before an empty name", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Name: name("")}, 0, invalid(revisionMsg)},
		{"an empty name", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev, Name: name("")}, 0, invalid(emptyMsg)},
		{"a name of spaces", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev, Name: name("   ")}, 0, invalid(emptyMsg)},
		{"an empty name before a bad image id", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev, Name: name(""), ImageId: name("x")}, 0, invalid(emptyMsg)},
		{"an image id that is not a uuid", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev, ImageId: name("imagem")}, 0, invalid(imageMsg)},
		{"an image id that is not a uuid, on a map that does not exist", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: unknown, Revision: rev, ImageId: name("imagem")}, 0, invalid(imageMsg)},
		{"a map that does not exist", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: unknown, Revision: rev, Name: name("X")}, 0, notFound},
		{"a map of another campaign", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: foreignMap, Revision: rev, Name: name("X")}, 0, notFound},
		{"a map that does not exist, with a stale revision", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: unknown, Revision: rev + 5, Name: name("X")}, 0, notFound},
		{"a map that does not exist, with another campaign's image", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: unknown, Revision: rev, ImageId: &foreignImage}, 0, notFound},
		{"a stale revision", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev + 1, Name: name("X")}, 0, stale},
		{"a revision from the future", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev + 1000, Name: name("X")}, 0, stale},
		{"a stale revision before another campaign's image", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev + 1, ImageId: &foreignImage}, 0, stale},
		{"another campaign's image", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev, ImageId: &foreignImage}, 0, invalid(imageMsg)},
		{"an image that does not exist", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev, ImageId: &unknown}, 0, invalid(imageMsg)},
		{"a good name with another campaign's image", m, &mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: rev, Name: name("Novo"), ImageId: &foreignImage}, 0, invalid(imageMsg)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.user.updateMapWith(tt.req)
			if tt.want.message == "" {
				wantCode(t, tt.name, err, tt.code)
				return
			}
			wantUpdateRefusal(t, tt.name, err, tt.want)
		})
	}
	got := m.mustGetMap(s.campaign, s.mapID).GetMap()
	if got.GetName() != cur.GetName() || got.GetRevision() != rev || got.GetImage().GetId() != cur.GetImage().GetId() {
		t.Errorf("the refused calls left the map as %q, revision %d, image %q; want %q, %d, %q",
			got.GetName(), got.GetRevision(), got.GetImage().GetId(), cur.GetName(), rev, cur.GetImage().GetId())
	}
}

// What a successful call writes: the name is trimmed (inner spaces stay), the revision goes up by one
// whatever changed, the other field stays, and the answer is what a read says.
func TestUpdateMap_WhatItWrites(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	cur := m.mustGetMap(s.campaign, s.mapID).GetMap()
	second := m.newImage(s.campaign)

	renamed, err := m.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: cur.GetRevision(), Name: new("  Estrada   velha ")})
	if err != nil {
		t.Fatalf("UpdateMap(name) error = %v", err)
	}
	if renamed.GetName() != "Estrada   velha" || renamed.GetRevision() != cur.GetRevision()+1 || renamed.GetImage().GetId() != cur.GetImage().GetId() {
		t.Errorf("a rename: %q, revision %d, image %q; want %q, %d, %q",
			renamed.GetName(), renamed.GetRevision(), renamed.GetImage().GetId(), "Estrada   velha", cur.GetRevision()+1, cur.GetImage().GetId())
	}
	again, err := m.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: renamed.GetRevision(), Name: new("Estrada velha")})
	if err != nil || again.GetRevision() != renamed.GetRevision()+1 {
		t.Errorf("the same name again = %v, %v; want it accepted and the revision up by one", again, err)
	}
	both, err := m.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: again.GetRevision(), Name: new("Estrada nova"), ImageId: &second})
	if err != nil || both.GetName() != "Estrada nova" || both.GetImage().GetId() != second || both.GetRevision() != again.GetRevision()+1 {
		t.Errorf("name and image together = %v, %v", both, err)
	}
	if read := m.mustGetMap(s.campaign, s.mapID).GetMap(); read.GetName() != both.GetName() || read.GetImage().GetId() != second || read.GetRevision() != both.GetRevision() {
		t.Errorf("a read says %q, image %q, revision %d; the answer said %q, %q, %d",
			read.GetName(), read.GetImage().GetId(), read.GetRevision(), both.GetName(), both.GetImage().GetId(), both.GetRevision())
	}
}

// A new image clears the layers and the players' map memory; the same image sent
// again, with or without a rename, clears nothing and is not refused during a combat.
func TestUpdateMap_OnlyANewImageClearsTheLayers(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	m := s.master
	m.mustSetGrid(s.campaign, s.mapID, 20)
	rev := m.mustPaint(s.campaign, s.mapID, mapsv1.MapLayer_MAP_LAYER_WALL, 1, [2]int32{1, 1}).GetLayersRevision()
	cur := m.mustGetMap(s.campaign, s.mapID).GetMap()

	same := cur.GetImage().GetId()
	if _, err := m.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: cur.GetRevision(), ImageId: &same}); err != nil {
		t.Fatalf("UpdateMap(the same image) error = %v", err)
	}
	if res := m.mustLayers(s.campaign, s.mapID); res.GetLayersRevision() != rev || len(res.GetWall()) == 0 {
		t.Fatalf("the same image cleared the layers: %v (revision was %d)", res, rev)
	}
	fresh := m.mustGetMap(s.campaign, s.mapID).GetMap()
	other := m.newImage(s.campaign)
	moved, err := m.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: fresh.GetRevision(), ImageId: &other})
	if err != nil {
		t.Fatalf("UpdateMap(a new image) error = %v", err)
	}
	if res := m.mustLayers(s.campaign, s.mapID); res.GetLayersRevision() != rev+1 || len(res.GetWall()) != 0 {
		t.Errorf("a new image left layers %v, want them empty at revision %d", res, rev+1)
	}
	if moved.GetGridColumns() != 20 {
		t.Errorf("a new image changed the grid to %d columns, want 20", moved.GetGridColumns())
	}
}

// RN-10: a map with the fog on never has an image a player can fetch, and a map
// without fog never takes the image of a fog map. The image is copied for the map
// when another use needs it, and left alone when nothing else does.
func TestUpdateMap_TheImageOfAFogMap(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	update := func(mapID string, imageID string) *mapsv1.Map {
		t.Helper()
		cur := master.mustGetMap(campaign, mapID).GetMap()
		got, err := master.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: campaign, MapId: mapID, Revision: cur.GetRevision(), ImageId: &imageID})
		if err != nil {
			t.Fatalf("UpdateMap(image) error = %v", err)
		}
		return got
	}

	master.setNewMapFog(campaign, true)
	fogImage := master.newImage(campaign)
	fog := master.createMap(campaign, "Com névoa", fogImage)
	master.mustSetGrid(campaign, fog.GetId(), 20)
	if !master.mustGetMap(campaign, fog.GetId()).GetMap().GetFogEnabled() {
		t.Fatal("the map did not get the fog")
	}
	master.setNewMapFog(campaign, false)
	// check runs an update and says how many files it added: 2 for a copy (the image and its
	// thumbnail), 0 when the map takes the image as it is.
	check := func(what string, mapID, imageID string, copied bool) {
		t.Helper()
		before := len(h.storedFiles())
		got := update(mapID, imageID)
		added := len(h.storedFiles()) - before
		if copied && (got.GetImage().GetId() == imageID || added != 2) {
			t.Errorf("%s: image %q (the original is %q), %d files added; want a copy and 2 files", what, got.GetImage().GetId(), imageID, added)
		}
		if !copied && (got.GetImage().GetId() != imageID || added != 0) {
			t.Errorf("%s: image %q (want %q), %d files added; want it as it is and none", what, got.GetImage().GetId(), imageID, added)
		}
	}

	shared := master.newImage(campaign)
	master.createMap(campaign, "Usa a imagem", shared)
	check("a fog map taking an image another map uses", fog.GetId(), shared, true)
	check("a fog map taking an image nothing else uses", fog.GetId(), master.newImage(campaign), false)

	plain := master.createMap(campaign, "Sem névoa", master.newImage(campaign))
	fogNow := master.mustGetMap(campaign, fog.GetId()).GetMap().GetImage().GetId()
	check("a plain map taking a fog map's image", plain.GetId(), fogNow, true)
	check("a plain map taking an image no fog map has", plain.GetId(), shared, false)
	check("the same image again", plain.GetId(), shared, false)
}

// The copy is made before the transaction. When the call is refused inside it, the
// copy's files go, and the map keeps its image.
func TestUpdateMap_ACopyOfNoUseIsDeleted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	master.setNewMapFog(campaign, true)
	fog := master.createMap(campaign, "Com névoa", master.newImage(campaign))
	master.mustSetGrid(campaign, fog.GetId(), 20)
	shared := master.newImage(campaign)
	master.createMap(campaign, "Usa a imagem", shared)
	files := h.storedFiles()
	cur := master.mustGetMap(campaign, fog.GetId()).GetMap()

	_, err := master.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: campaign, MapId: fog.GetId(), Revision: cur.GetRevision() + 1, ImageId: &shared})
	wantUpdateRefusal(t, "a stale revision with a copy made", err, updateRefusal{connect.CodeAborted, "the map changed since you opened it; reload it and try again"})
	if now := h.storedFiles(); !slices.Equal(now, files) {
		t.Errorf("the copy made for nothing stayed in the store: %v, want %v", now, files)
	}
	if got := master.mustGetMap(campaign, fog.GetId()).GetMap(); got.GetImage().GetId() != cur.GetImage().GetId() {
		t.Errorf("the refused call left image %q, want %q", got.GetImage().GetId(), cur.GetImage().GetId())
	}
}

// Who hears about it: a map the players see sends map_changed to them and to the
// master; a hidden map only to the master.
func TestUpdateMap_WhoHearsAboutIt(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	hidden := m.createMap(s.campaign, "Escondido", m.newImage(s.campaign))
	marker := m.createMap(s.campaign, "Marcador", m.newImage(s.campaign))
	masterWatch, anaWatch := m.watch(s.campaign), s.ana.watch(s.campaign)

	cur := m.mustGetMap(s.campaign, s.mapID).GetMap()
	if _, err := m.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: s.mapID, Revision: cur.GetRevision(), Name: new("Visível")}); err != nil {
		t.Fatalf("UpdateMap(shown map) error = %v", err)
	}
	masterWatch.mapChanged(s.mapID)
	anaWatch.mapChanged(s.mapID)

	if _, err := m.updateMapWith(&mapsv1.UpdateMapRequest{CampaignId: s.campaign, MapId: hidden.GetId(), Revision: hidden.GetRevision(), Name: new("Ainda escondido")}); err != nil {
		t.Fatalf("UpdateMap(hidden map) error = %v", err)
	}
	masterWatch.mapChanged(hidden.GetId())
	// A later change the player does hear proves the hidden one was not queued for them.
	m.setMapRevealed(s.campaign, marker.GetId(), true)
	masterWatch.mapChanged(marker.GetId())
	anaWatch.mapChanged(marker.GetId())
}
