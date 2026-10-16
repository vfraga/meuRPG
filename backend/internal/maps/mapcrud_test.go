package maps

import (
	"slices"
	"strings"
	"testing"
	"uuid"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// GetMap, CreateMap, DeleteMap and SetMapRevealed are frozen here before their
// phases are split (MR-008, MR-009, RN-10, RN-24): the order of their refusals, what
// each viewer reads of a map, and who hears about each write, in which order.

// crudRefusal is what a refused call of these four says.
type crudRefusal struct {
	code    connect.Code
	message string
	reason  mapsv1.MapBlockedReason
}

func wantCrudRefusal(t *testing.T, call string, err error, want crudRefusal) {
	t.Helper()
	wantRefusal(t, call, err, want.code, want.message)
	if want.reason != mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_UNSPECIFIED {
		wantMapBlocked(t, call, err, want.reason)
	}
}

var crudMapNotFound = crudRefusal{code: connect.CodeNotFound, message: "map not found"}

func (u *user) tryCreateMap(req *mapsv1.CreateMapRequest) (*mapsv1.Map, error) {
	res, err := u.maps.CreateMap(u.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetMap(), nil
}

func (u *user) tryDeleteMap(campaignID, mapID string) error {
	_, err := u.maps.DeleteMap(u.h.t.Context(), connect.NewRequest(&mapsv1.DeleteMapRequest{CampaignId: campaignID, MapId: mapID}))
	return err
}

func (u *user) trySetMapRevealed(campaignID, mapID string, revealed bool) (*mapsv1.Map, error) {
	res, err := u.maps.SetMapRevealed(u.h.t.Context(), connect.NewRequest(&mapsv1.SetMapRevealedRequest{CampaignId: campaignID, MapId: mapID, Revealed: revealed}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetMap(), nil
}

// eventLog writes the events a watcher drained as "kind:what", naming the maps by
// the aliases given.
func eventLog(evs []*playv1.WatchGameSessionResponse, alias map[string]string) []string {
	name := func(id string) string {
		if a, ok := alias[id]; ok {
			return a
		}
		return id
	}
	var out []string
	for _, ev := range evs {
		switch {
		case ev.GetMapChanged() != nil:
			out = append(out, "map_changed:"+name(ev.GetMapChanged().GetMapId()))
		case ev.GetCurrentMapChanged() != nil:
			out = append(out, "current_map_changed:"+name(ev.GetCurrentMapChanged().GetMapId()))
		case ev.GetSceneChanged() != nil:
			out = append(out, "scene_changed")
		default:
			out = append(out, "other")
		}
	}
	return out
}

func wantEvents(t *testing.T, who string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s heard %v, want %v", who, got, want)
	}
}

// GetMap's refusals, in order: the member check, the map's id, the reader, and then
// the map itself: a map that is hidden from a player is "not found" exactly as one
// that does not exist (RN-10).
func TestGetMap_Refusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	hidden := s.master.createMap(s.campaign, "Escondido", s.master.newImage(s.campaign))
	outsider := s.h.newUser("De fora")
	other := s.h.newCampaign(outsider)
	foreign := outsider.createMap(other, "De outra mesa", outsider.newImage(other)).GetId()
	get := func(u *user, campaign, id string) error {
		_, err := u.getMap(campaign, id)
		return err
	}

	wantCrudRefusal(t, "a bad id", get(s.master, s.campaign, "nope"), crudMapNotFound)
	wantCrudRefusal(t, "an empty id", get(s.master, s.campaign, ""), crudMapNotFound)
	wantCrudRefusal(t, "an unknown map", get(s.master, s.campaign, uuid.New().String()), crudMapNotFound)
	wantCrudRefusal(t, "a map of another campaign", get(s.master, s.campaign, foreign), crudMapNotFound)
	wantCrudRefusal(t, "a hidden map as a player", get(s.ana, s.campaign, hidden.GetId()), crudMapNotFound)
	wantCode(t, "a stranger", get(outsider, s.campaign, s.mapID), connect.CodeNotFound)
	wantCode(t, "a bad id and a stranger", get(outsider, s.campaign, "nope"), connect.CodeNotFound)
	if err := get(s.master, s.campaign, hidden.GetId()); err != nil {
		t.Errorf("the master reading a hidden map: %v", err)
	}
	// The session's current map is seen by the players even while hidden.
	if _, err := s.master.setCurrentMap(s.campaign, hidden.GetId()); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	if err := get(s.ana, s.campaign, hidden.GetId()); err != nil {
		t.Errorf("a player reading the current map: %v", err)
	}
}

// What each viewer reads of one map (RN-10): the master everything; a player the
// revealed points and the tokens that are not hidden, with the map's point count
// agreeing with what they got; a player's character is listed before the others.
func TestGetMap_WhatEachViewerReads(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	revealedScene := m.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Cena à vista", XBp: 1000, YBp: 1000})
	m.setPointRevealed(s.campaign, revealedScene, true)
	s.newTrap("Armadilha escondida", 2000, 2000)
	s.newTreasure("Tesouro escondido", "", 100, 3000, 3000)
	s.newLight("Tocha", "light:torch")
	goblin := m.createCharacter(s.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	orc := m.createCharacter(s.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Orc")
	m.placeToken(s.campaign, s.mapID, s.pens.GetId(), 4000, 4000)
	m.placeToken(s.campaign, s.mapID, s.other.GetId(), 4100, 4000)
	m.placeToken(s.campaign, s.mapID, goblin.GetId(), 4200, 4000)
	m.placeToken(s.campaign, s.mapID, orc.GetId(), 4300, 4000)
	// A creature's token is placed hidden: the master shows the goblin and keeps the orc hidden.
	m.setTokenHidden(s.campaign, s.mapID, goblin.GetId(), false)
	m.setTokenHidden(s.campaign, s.mapID, orc.GetId(), true)

	pointNames := func(res *mapsv1.GetMapResponse) []string {
		var out []string
		for _, p := range res.GetPoints() {
			out = append(out, p.GetName())
		}
		slices.Sort(out)
		return out
	}
	tokenNames := func(res *mapsv1.GetMapResponse) []string {
		var out []string
		for _, tk := range res.GetTokens() {
			out = append(out, tk.GetName())
		}
		return out
	}
	master := m.mustGetMap(s.campaign, s.mapID)
	if got, want := pointNames(master), []string{"A carroça tombada", "Armadilha escondida", "Cena à vista", "Tesouro escondido", "Tocha"}; !slices.Equal(got, want) {
		t.Errorf("the master's points = %v, want %v", got, want)
	}
	if got := tokenNames(master); len(got) != 4 {
		t.Errorf("the master's tokens = %v, want the four", got)
	}
	if master.GetMap().GetPointCount() != 5 {
		t.Errorf("the master's point count = %d, want 5", master.GetMap().GetPointCount())
	}
	for _, who := range []struct {
		name string
		u    *user
	}{{"Ana", s.ana}, {"Caio", s.caio}} {
		res := who.u.mustGetMap(s.campaign, s.mapID)
		if got, want := pointNames(res), []string{"Cena à vista"}; !slices.Equal(got, want) {
			t.Errorf("%s's points = %v, want %v", who.name, got, want)
		}
		if res.GetMap().GetPointCount() != 1 {
			t.Errorf("%s's point count = %d, want 1", who.name, res.GetMap().GetPointCount())
		}
		got := tokenNames(res)
		if len(got) != 3 || slices.Contains(got, "Orc") {
			t.Errorf("%s's tokens = %v, want the three that are not hidden", who.name, got)
		}
		for _, tk := range res.GetTokens() {
			if tk.GetHidden() {
				t.Errorf("%s reads a hidden token %v", who.name, tk)
			}
			if tk.GetMine() != (tk.GetCharacterId() == s.pens.GetId() && who.u == s.ana || tk.GetCharacterId() == s.other.GetId() && who.u == s.caio) {
				t.Errorf("%s: token %s has mine = %v", who.name, tk.GetName(), tk.GetMine())
			}
		}
	}
	// Players' characters come first (the order the characters module gives).
	tokens := s.ana.mustGetMap(s.campaign, s.mapID).GetTokens()
	if len(tokens) != 3 || tokens[0].GetKind() != charactersv1.CharacterKind_CHARACTER_KIND_PLAYER || tokens[1].GetKind() != charactersv1.CharacterKind_CHARACTER_KIND_PLAYER {
		t.Errorf("Ana's tokens = %v, want the players' characters first", tokens)
	}
	// The master hides a point and a player's reading follows.
	m.setPointRevealed(s.campaign, revealedScene, false)
	if got := pointNames(s.ana.mustGetMap(s.campaign, s.mapID)); len(got) != 0 {
		t.Errorf("Ana's points after hiding the scene = %v, want none", got)
	}
}

// CreateMap's refusals, in order: the role, the name, the image's id, the idempotency
// key, and inside the transaction the cap and the image itself.
func TestCreateMap_Refusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *Config) { c.MaxMaps = 2 })
	master, player, outsider := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("De fora")
	campaign := h.newCampaign(master, player)
	other := h.newCampaign(outsider)
	image := master.newImage(campaign)
	foreignImage := outsider.newImage(other)
	invalid := func(msg string) crudRefusal { return crudRefusal{code: connect.CodeInvalidArgument, message: msg} }
	notAnImage := invalid("image_id is not an image of the campaign's gallery")
	create := func(u *user, req *mapsv1.CreateMapRequest) error {
		req.CampaignId = campaign
		_, err := u.tryCreateMap(req)
		return err
	}

	wantCode(t, "a player", create(player, &mapsv1.CreateMapRequest{Name: "X", ImageId: image}), connect.CodePermissionDenied)
	wantCode(t, "a stranger", create(outsider, &mapsv1.CreateMapRequest{Name: "X", ImageId: image}), connect.CodeNotFound)
	wantCrudRefusal(t, "an empty name", create(master, &mapsv1.CreateMapRequest{Name: " ", ImageId: image}), invalid("name must not be empty"))
	wantCrudRefusal(t, "an empty name before a bad image id", create(master, &mapsv1.CreateMapRequest{Name: "", ImageId: "x"}), invalid("name must not be empty"))
	wantCrudRefusal(t, "a name of 81 characters", create(master, &mapsv1.CreateMapRequest{Name: strings.Repeat("a", 81), ImageId: image}), invalid("name must be at most 80 characters, got 81"))
	wantCrudRefusal(t, "an image id that is not a uuid", create(master, &mapsv1.CreateMapRequest{Name: "X", ImageId: "imagem"}), notAnImage)
	wantCrudRefusal(t, "an empty image id", create(master, &mapsv1.CreateMapRequest{Name: "X"}), notAnImage)
	wantCrudRefusal(t, "an image that does not exist", create(master, &mapsv1.CreateMapRequest{Name: "X", ImageId: uuid.New().String()}), notAnImage)
	wantCrudRefusal(t, "another campaign's image", create(master, &mapsv1.CreateMapRequest{Name: "X", ImageId: foreignImage}), notAnImage)
	wantCrudRefusal(t, "a key of 65 characters", create(master, &mapsv1.CreateMapRequest{Name: "X", ImageId: image, IdempotencyKey: strings.Repeat("k", 65)}),
		invalid("idempotency_key must be 1 to 64 characters"))
	wantCrudRefusal(t, "a bad key before a bad image id", create(master, &mapsv1.CreateMapRequest{Name: "X", ImageId: "x", IdempotencyKey: strings.Repeat("k", 65)}), notAnImage)
	if n := len(master.listMaps(campaign)); n != 0 {
		t.Fatalf("the refused calls made %d maps", n)
	}

	first, err := master.tryCreateMap(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "  Mirathel  ", ImageId: image, IdempotencyKey: "k1"})
	if err != nil {
		t.Fatalf("CreateMap() error = %v", err)
	}
	if first.GetName() != "Mirathel" || first.GetRevision() != 1 || first.GetRevealed() || first.GetImage().GetId() != image || first.GetGridColumns() != 0 || first.GetFogEnabled() {
		t.Errorf("a new map = %v, want a hidden map named Mirathel at revision 1 with no grid and no fog", first)
	}
	// The same request again returns the first map; the same key for another request is refused.
	again, err := master.tryCreateMap(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "  Mirathel  ", ImageId: image, IdempotencyKey: "k1"})
	if err != nil || again.GetId() != first.GetId() {
		t.Errorf("the same request again = %v, %v; want the first map", again, err)
	}
	_, err = master.tryCreateMap(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "Outro", ImageId: image, IdempotencyKey: "k1"})
	wantCrudRefusal(t, "a key used for another request", err, invalid("idempotency_key was used for another change"))
	// The cap counts before the image is checked, and a replay does not count.
	second := master.createMap(campaign, "Segundo", image)
	_, err = master.tryCreateMap(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "Terceiro", ImageId: foreignImage})
	wantCrudRefusal(t, "the cap before the image", err, crudRefusal{code: connect.CodeResourceExhausted, message: "the campaign already has 2 maps"})
	_, err = master.tryCreateMap(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "Terceiro", ImageId: image})
	wantCrudRefusal(t, "the cap", err, crudRefusal{code: connect.CodeResourceExhausted, message: "the campaign already has 2 maps"})
	replay, err := master.tryCreateMap(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "  Mirathel  ", ImageId: image, IdempotencyKey: "k1"})
	if err != nil || replay.GetId() != first.GetId() {
		t.Errorf("a replay at the cap = %v, %v; want the first map", replay, err)
	}
	if n := len(master.listMaps(campaign)); n != 2 || second.GetId() == "" {
		t.Errorf("%d maps, want 2", n)
	}
}

// CreateMap and the fog rule (RN-24, RN-10): the new map is marked when the table
// wants the fog, and takes a copy of an image a fog map has; the copy made for
// nothing, because the call is refused, leaves no files.
func TestCreateMap_TheFogRuleAndTheImageOfAFogMap(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(c *Config) { c.MaxMaps = 2 })
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master)
	master.setNewMapFog(campaign, true)
	fogImage := master.newImage(campaign)
	fog := master.createMap(campaign, "Com névoa", fogImage)
	if fog.GetFogEnabled() {
		t.Error("a map with no grid says the fog is on")
	}
	master.mustSetGrid(campaign, fog.GetId(), 20)
	if !master.mustGetMap(campaign, fog.GetId()).GetMap().GetFogEnabled() {
		t.Fatal("the first grid did not turn the fog on")
	}
	files := h.storedFiles()

	// A map made from the image of a fog map takes a copy of it.
	_, err := master.tryCreateMap(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "Segundo", ImageId: master.mustGetMap(campaign, fog.GetId()).GetMap().GetImage().GetId()})
	if err != nil {
		t.Fatalf("CreateMap() with a fog map's image error = %v", err)
	}
	if now := h.storedFiles(); len(now) != len(files)+2 {
		t.Errorf("a map made from a fog map's image: %d files, want %d (the copy and its thumbnail)", len(now), len(files)+2)
	}
	// The cap refuses the call that made a copy: the copy's files go.
	files = h.storedFiles()
	fogNow := master.mustGetMap(campaign, fog.GetId()).GetMap().GetImage().GetId()
	_, err = master.tryCreateMap(&mapsv1.CreateMapRequest{CampaignId: campaign, Name: "Terceiro", ImageId: fogNow})
	wantCrudRefusal(t, "the cap with a copy made", err, crudRefusal{code: connect.CodeResourceExhausted, message: "the campaign already has 2 maps"})
	if now := h.storedFiles(); !slices.Equal(now, files) {
		t.Errorf("the copy made for nothing stayed in the store: %v, want %v", now, files)
	}
	var copies int
	for _, m := range master.listMaps(campaign) {
		if m.GetImage().GetId() != fogNow && m.GetId() != fog.GetId() {
			copies++
		}
	}
	if copies != 1 {
		t.Errorf("%d maps took a copy of the fog image, want 1", copies)
	}
}

// Who hears about a new map: nobody but the master, and a replay announces nothing.
func TestCreateMap_OnlyTheMasterHearsAboutIt(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	image := m.newImage(s.campaign)
	probe := m.createMap(s.campaign, "Sonda", image)
	masterWatch, anaWatch := m.watch(s.campaign), s.ana.watch(s.campaign)
	req := &mapsv1.CreateMapRequest{CampaignId: s.campaign, Name: "Nova", ImageId: image, IdempotencyKey: "k"}
	created, err := m.tryCreateMap(req)
	if err != nil {
		t.Fatalf("CreateMap() error = %v", err)
	}
	if _, err := m.tryCreateMap(req); err != nil { // the replay
		t.Fatalf("CreateMap() replay error = %v", err)
	}
	s.probe(probe.GetId())
	alias := map[string]string{created.GetId(): "nova", probe.GetId(): "sonda"}
	wantEvents(t, "the master", eventLog(masterWatch.drain(probe.GetId()), alias), []string{"map_changed:nova"})
	wantEvents(t, "Ana", eventLog(anaWatch.drain(probe.GetId()), alias), nil)
}

// DeleteMap's refusals, in order: the role, the map's id, the map, the combat, and
// the treasures that were found or turned into XP (converted first).
func TestDeleteMap_Refusals(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	outsider := s.h.newUser("De fora")
	other := s.h.newCampaign(outsider)
	foreign := outsider.createMap(other, "De outra mesa", outsider.newImage(other)).GetId()

	wantCode(t, "a player", s.ana.tryDeleteMap(s.campaign, s.mapID), connect.CodePermissionDenied)
	wantCode(t, "a stranger", outsider.tryDeleteMap(s.campaign, s.mapID), connect.CodeNotFound)
	wantCrudRefusal(t, "a bad id", m.tryDeleteMap(s.campaign, "nope"), crudMapNotFound)
	wantCrudRefusal(t, "an unknown map", m.tryDeleteMap(s.campaign, uuid.New().String()), crudMapNotFound)
	wantCrudRefusal(t, "a map of another campaign", m.tryDeleteMap(s.campaign, foreign), crudMapNotFound)

	found := s.newTreasure("Achado", "", 100, 2000, 2000)
	s.markFound(found, s.pens.GetId())
	wantCrudRefusal(t, "a map with a found treasure", m.tryDeleteMap(s.campaign, s.mapID), crudRefusal{
		connect.CodeFailedPrecondition, "the treasure was found: unmark it before deleting it or changing its kind", mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_FOUND,
	})
	converted := s.newTreasure("Convertido", "", 100, 2100, 2000)
	s.convertTreasureForCrud(converted)
	wantCrudRefusal(t, "a map with a converted treasure", m.tryDeleteMap(s.campaign, s.mapID), crudRefusal{
		connect.CodeFailedPrecondition, "the treasure was turned into XP: its value and its found mark cannot change", mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_TREASURE_CONVERTED,
	})

	// A combat on the map is refused before the treasures are looked at.
	if _, err := m.setCurrentMap(s.campaign, s.mapID); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	m.mustSetGrid(s.campaign, s.mapID, 20)
	if _, err := m.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: s.campaign, IdempotencyKey: newKey(), Name: "Emboscada",
		Participants: []*playv1.Participant{{CharacterId: s.pens.GetId()}},
	})); err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	wantCrudRefusal(t, "a map with a combat", m.tryDeleteMap(s.campaign, s.mapID), crudRefusal{
		connect.CodeFailedPrecondition, "a combat is running on this map: it cannot be changed or deleted", mapsv1.MapBlockedReason_MAP_BLOCKED_REASON_COMBAT_RUNNING,
	})
	if _, err := m.getMap(s.campaign, s.mapID); err != nil {
		t.Errorf("the refused deletes removed the map: %v", err)
	}
}

// convertTreasureForCrud turns the treasure into XP, as the party's return to town does.
func (s *scenes) convertTreasureForCrud(p *mapsv1.MapPoint) {
	s.h.t.Helper()
	s.markFound(p, s.pens.GetId()) // the table's CHECK: only a found treasure is converted
	var award string
	if err := s.h.pool.QueryRow(s.h.t.Context(), `
		INSERT INTO xp_awards (campaign_id, created_at, mode, reason, gold, total_xp, idempotency_key)
		VALUES ($1, now(), 'gold', 'Voltar à cidade', 100, 100, gen_random_uuid()) RETURNING id::TEXT`, s.campaign).Scan(&award); err != nil {
		s.h.t.Fatalf("insert an XP award: %v", err)
	}
	if _, err := s.h.pool.Exec(s.h.t.Context(), `UPDATE map_points SET treasure_converted_award_id = $1 WHERE id = $2`, award, p.GetId()); err != nil {
		s.h.t.Fatalf("convert the treasure: %v", err)
	}
}

// Who hears about a deleted map and in which order: the map, the maps that linked to
// it, the session if it was the current map, and the scene if one of its points was open.
func TestDeleteMap_WhoHearsAboutIt(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	image := m.newImage(s.campaign)
	// Two revealed maps the estrada leads to: the point to the tower is revealed, the one to the cave is not.
	tower, cave := m.createMap(s.campaign, "Torre", image), m.createMap(s.campaign, "Caverna", image)
	m.setMapRevealed(s.campaign, tower.GetId(), true)
	m.setMapRevealed(s.campaign, cave.GetId(), true)
	for i, target := range []*mapsv1.Map{tower, cave} {
		p := m.createPoint(&mapsv1.CreateMapPointRequest{
			CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP, Name: target.GetName(), TargetMapId: target.GetId(), XBp: int32(500 + 100*i), YBp: 500,
		})
		if i == 0 {
			m.setPointRevealed(s.campaign, p, true)
		}
	}
	probe := m.createMap(s.campaign, "Sonda", image)
	alias := map[string]string{s.mapID: "estrada", tower.GetId(): "torre", cave.GetId(): "caverna", probe.GetId(): "sonda"}

	// A revealed map that is not the current one: the players hear about the parent only when
	// its point to the map is revealed.
	masterWatch, anaWatch := m.watch(s.campaign), s.ana.watch(s.campaign)
	if err := m.tryDeleteMap(s.campaign, tower.GetId()); err != nil {
		t.Fatalf("DeleteMap(tower) error = %v", err)
	}
	if err := m.tryDeleteMap(s.campaign, cave.GetId()); err != nil {
		t.Fatalf("DeleteMap(cave) error = %v", err)
	}
	s.probe(probe.GetId())
	wantEvents(t, "the master", eventLog(masterWatch.drain(probe.GetId()), alias), []string{"map_changed:torre", "map_changed:estrada", "map_changed:caverna", "map_changed:estrada"})
	wantEvents(t, "Ana", eventLog(anaWatch.drain(probe.GetId()), alias), []string{"map_changed:torre", "map_changed:estrada", "map_changed:caverna"})

	// The current map, with the scene of one of its points open.
	if _, err := m.setCurrentMap(s.campaign, s.mapID); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	m.mustOpenScene(s.campaign, s.point.GetId())
	probe2 := m.createMap(s.campaign, "Sonda 2", image)
	alias[probe2.GetId()] = "sonda2"
	masterWatch, anaWatch = m.watch(s.campaign), s.ana.watch(s.campaign)
	if err := m.tryDeleteMap(s.campaign, s.mapID); err != nil {
		t.Fatalf("DeleteMap(current) error = %v", err)
	}
	s.probe(probe2.GetId())
	want := []string{"map_changed:estrada", "current_map_changed:", "scene_changed"}
	wantEvents(t, "the master", eventLog(masterWatch.drain(probe2.GetId()), alias), want)
	wantEvents(t, "Ana", eventLog(anaWatch.drain(probe2.GetId()), alias), want)

	// A hidden map that is not current: only the master hears.
	hidden := m.createMap(s.campaign, "Escondido", image)
	probe3 := m.createMap(s.campaign, "Sonda 3", image)
	alias[hidden.GetId()], alias[probe3.GetId()] = "escondido", "sonda3"
	masterWatch, anaWatch = m.watch(s.campaign), s.ana.watch(s.campaign)
	if err := m.tryDeleteMap(s.campaign, hidden.GetId()); err != nil {
		t.Fatalf("DeleteMap(hidden) error = %v", err)
	}
	s.probe(probe3.GetId())
	wantEvents(t, "the master", eventLog(masterWatch.drain(probe3.GetId()), alias), []string{"map_changed:escondido"})
	wantEvents(t, "Ana", eventLog(anaWatch.drain(probe3.GetId()), alias), nil)
}

// SetMapRevealed's refusals and its writes: only the master; a map that is not found;
// revealing twice changes nothing but the revision stays; hiding clears it.
func TestSetMapRevealed_RefusalsAndWrites(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	outsider := s.h.newUser("De fora")
	hidden := m.createMap(s.campaign, "Escondido", m.newImage(s.campaign))

	_, err := s.ana.trySetMapRevealed(s.campaign, hidden.GetId(), true)
	wantCode(t, "a player", err, connect.CodePermissionDenied)
	_, err = outsider.trySetMapRevealed(s.campaign, hidden.GetId(), true)
	wantCode(t, "a stranger", err, connect.CodeNotFound)
	_, err = m.trySetMapRevealed(s.campaign, "nope", true)
	wantCrudRefusal(t, "a bad id", err, crudMapNotFound)
	_, err = m.trySetMapRevealed(s.campaign, uuid.New().String(), true)
	wantCrudRefusal(t, "an unknown map", err, crudMapNotFound)

	revealed, err := m.trySetMapRevealed(s.campaign, hidden.GetId(), true)
	if err != nil || !revealed.GetRevealed() {
		t.Fatalf("revealing = %v, %v", revealed, err)
	}
	again, err := m.trySetMapRevealed(s.campaign, hidden.GetId(), true)
	if err != nil || !again.GetRevealed() || again.GetRevision() != revealed.GetRevision() {
		t.Errorf("revealing twice = %v, %v; want the same revision %d", again, err, revealed.GetRevision())
	}
	if got := s.ana.mustGetMap(s.campaign, hidden.GetId()).GetMap(); !got.GetRevealed() {
		t.Errorf("Ana reads the revealed map as %v", got)
	}
	gone, err := m.trySetMapRevealed(s.campaign, hidden.GetId(), false)
	if err != nil || gone.GetRevealed() {
		t.Fatalf("hiding = %v, %v", gone, err)
	}
	if _, err := s.ana.getMap(s.campaign, hidden.GetId()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("Ana reading the hidden map error = %v, want not_found", err)
	}
}

// Who hears about a revealed or hidden map, and in which order: the map to whoever
// sees it before or after, then the maps that link to it.
func TestSetMapRevealed_WhoHearsAboutIt(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	image := m.newImage(s.campaign)
	tower := m.createMap(s.campaign, "Torre", image)
	m.setPointRevealed(s.campaign, m.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP, Name: "Torre", TargetMapId: tower.GetId(), XBp: 500, YBp: 500,
	}), true)
	probe := m.createMap(s.campaign, "Sonda", image)
	alias := map[string]string{s.mapID: "estrada", tower.GetId(): "torre", probe.GetId(): "sonda"}
	masterWatch, anaWatch := m.watch(s.campaign), s.ana.watch(s.campaign)

	m.setMapRevealed(s.campaign, tower.GetId(), true)  // players start seeing it
	m.setMapRevealed(s.campaign, tower.GetId(), true)  // nothing changes
	m.setMapRevealed(s.campaign, tower.GetId(), false) // players stop seeing it
	s.probe(probe.GetId())
	wantEvents(t, "the master", eventLog(masterWatch.drain(probe.GetId()), alias), []string{
		"map_changed:torre", "map_changed:estrada", "map_changed:torre", "map_changed:estrada", "map_changed:torre", "map_changed:estrada",
	})
	// The parent is told to the players only when the players' view of the tower changed.
	wantEvents(t, "Ana", eventLog(anaWatch.drain(probe.GetId()), alias), []string{
		"map_changed:torre", "map_changed:estrada", "map_changed:torre", "map_changed:torre", "map_changed:estrada",
	})
}
