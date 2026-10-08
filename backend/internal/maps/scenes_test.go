package maps

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The RP scenes (MR-015, RN-18, RN-20; questions 51 to 55 with their
// defaults): the master's actions on a scene point (this package), and the
// scene open in the session, the rolls and the log (package play). The tests
// live here because this harness has the maps, the characters and play
// together. Pensantus's numbers are the golden file's (rules/testdata/golden):
// Investigação +6 (passive 16), Percepção +1 (passive 11), a Teste de Força +1
// and the Teste de resistência de Sabedoria +3.

func newKey() string { return uuid.New().String() }

// scenes is the table of these tests: Mirathel's master, Pensantus's player
// (Ana) and a second player (Caio) whose gnome wizard has other numbers, a
// map with the hidden SCENE point "A carroça tombada", and an open session.
type scenes struct {
	h                 *harness
	master, ana, caio *user
	campaign, mapID   string
	point             *mapsv1.MapPoint
	pens, other       *charactersv1.Character
}

const sceneDescription = "Uma carroça tombada bloqueia a estrada. Há marcas de garras na madeira."

// pensantus is the gnome wizard of the golden file.
func (u *user) pensantus(campaignID string) *charactersv1.Character {
	u.h.t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 12, Dexterity: 16, Constitution: 15, Intelligence: 16, Wisdom: 13, Charisma: 12},
		RaceKey:    "race:gnome", SubraceKey: "subrace:rock-gnome",
		Classes:              []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:evocation"}, Level: 3}},
		Background:           &charactersv1.FullSheet_CustomBackground{CustomBackground: &charactersv1.CustomBackground{Name: "Sábio", SkillKeys: []string{"skill:arcana", "skill:history"}}},
		SkillProficiencyKeys: []string{"skill:investigation", "skill:insight"},
	}}}
	res, err := u.characters.CreateCharacter(u.h.t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: sheet,
	}))
	if err != nil {
		u.h.t.Fatalf("CreateCharacter(Pensantus) error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// newScenes sets the table, with a session open when session is true.
func newScenes(t *testing.T, session bool) *scenes {
	t.Helper()
	h := newHarness(t)
	s := &scenes{h: h, master: h.newUser("Mestre"), ana: h.newUser("Ana"), caio: h.newUser("Caio")}
	s.campaign = h.newCampaign(s.master, s.ana, s.caio)
	s.pens = s.ana.pensantus(s.campaign)
	s.other = s.caio.createCharacter(s.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren")
	m := s.master.createMap(s.campaign, "Estrada de Mirathel", s.master.newImage(s.campaign))
	s.mapID = m.GetId()
	s.master.setMapRevealed(s.campaign, s.mapID, true)
	s.point = s.master.createPoint(&mapsv1.CreateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE,
		Name: "A carroça tombada", Description: sceneDescription, XBp: 3000, YBp: 4000,
	})
	if session {
		s.master.start(s.campaign)
	}
	return s
}

// setup puts the master's three actions on the point: the numbers of MR-015's
// first criterion.
func (s *scenes) setup() (invest, force, wis *mapsv1.SceneAction) {
	s.h.t.Helper()
	invest = s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "skill:investigation", "Procurar pistas", 15)
	force = s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "ability:str", "", 0)
	wis = s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "save:wis", "", 12)
	return invest, force, wis
}

func (u *user) tryAddAction(campaignID, mapID, pointID, key, name string, dc int32) (*mapsv1.AddSceneActionResponse, error) {
	res, err := u.maps.AddSceneAction(u.h.t.Context(), connect.NewRequest(&mapsv1.AddSceneActionRequest{
		CampaignId: campaignID, MapId: mapID, PointId: pointID, Key: key, Name: name, Dc: dc,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (u *user) addAction(campaignID, mapID, pointID, key, name string, dc int32) *mapsv1.SceneAction {
	u.h.t.Helper()
	res, err := u.tryAddAction(campaignID, mapID, pointID, key, name, dc)
	if err != nil {
		u.h.t.Fatalf("AddSceneAction(%s) error = %v", key, err)
	}
	return res.GetAction()
}

func (s *scenes) update(u *user, a *mapsv1.SceneAction, edit func(*mapsv1.UpdateSceneActionRequest)) (*mapsv1.UpdateSceneActionResponse, error) {
	req := &mapsv1.UpdateSceneActionRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId(), ActionId: a.GetId()}
	edit(req)
	res, err := u.maps.UpdateSceneAction(s.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (s *scenes) move(a *mapsv1.SceneAction, dir mapsv1.SceneActionDirection) []*mapsv1.SceneAction {
	s.h.t.Helper()
	res, err := s.master.maps.MoveSceneAction(s.h.t.Context(), connect.NewRequest(&mapsv1.MoveSceneActionRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId(), ActionId: a.GetId(), Direction: dir,
	}))
	if err != nil {
		s.h.t.Fatalf("MoveSceneAction() error = %v", err)
	}
	return res.Msg.GetActions()
}

func (u *user) openScene(campaignID, pointID string) (*playv1.OpenSceneInfo, error) {
	res, err := u.play.OpenScene(u.h.t.Context(), connect.NewRequest(&playv1.OpenSceneRequest{CampaignId: campaignID, PointId: pointID}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetScene(), nil
}

func (u *user) mustOpenScene(campaignID, pointID string) *playv1.OpenSceneInfo {
	u.h.t.Helper()
	scene, err := u.openScene(campaignID, pointID)
	if err != nil {
		u.h.t.Fatalf("OpenScene() error = %v", err)
	}
	return scene
}

func (u *user) closeScene(campaignID string) {
	u.h.t.Helper()
	if _, err := u.play.CloseScene(u.h.t.Context(), connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: campaignID})); err != nil {
		u.h.t.Fatalf("CloseScene() error = %v", err)
	}
}

func (u *user) getScene(campaignID string) *playv1.OpenSceneInfo {
	u.h.t.Helper()
	res, err := u.play.GetOpenScene(u.h.t.Context(), connect.NewRequest(&playv1.GetOpenSceneRequest{CampaignId: campaignID}))
	if err != nil {
		u.h.t.Fatalf("GetOpenScene() error = %v", err)
	}
	return res.Msg.GetScene()
}

// rollWith calls RollSceneCheck: a typed d20 face, or in the app when face is 0.
func (u *user) rollWith(campaignID, actionID string, face int32, key string) (*playv1.SceneRoll, error) {
	req := &playv1.RollSceneCheckRequest{CampaignId: campaignID, ActionId: actionID, IdempotencyKey: key}
	if face == 0 {
		req.Roll = &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}
	} else {
		req.Roll = &playv1.RollSceneCheckRequest_D20Face{D20Face: face}
	}
	res, err := u.play.RollSceneCheck(u.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetRoll(), nil
}

func (u *user) mustRoll(campaignID, actionID string, face int32) *playv1.SceneRoll {
	u.h.t.Helper()
	roll, err := u.rollWith(campaignID, actionID, face, newKey())
	if err != nil {
		u.h.t.Fatalf("RollSceneCheck() error = %v", err)
	}
	return roll
}

func wantSceneBlocked(t *testing.T, call string, err error, reason playv1.SceneBlockedReason) {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		if msg, derr := d.Value(); derr == nil {
			if b, ok := msg.(*playv1.SceneBlocked); ok && b.GetReason() == reason {
				return
			}
		}
	}
	t.Errorf("%s error = %v, want a SceneBlocked detail with %v", call, err, reason)
}

// sceneEvents reads the session_events the scenes wrote, oldest first, as
// "kind payload" lines.
func (h *harness) sceneEvents() []string {
	h.t.Helper()
	rows, err := h.pool.Query(h.t.Context(), `SELECT kind, payload::TEXT FROM session_events WHERE kind LIKE 'scene_%' ORDER BY seq`)
	if err != nil {
		h.t.Fatalf("read the scene events: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var kind, payload string
		if err := rows.Scan(&kind, &payload); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, kind+" "+payload)
	}
	return out
}

func (s *scenes) count(kind string) int {
	n := 0
	for _, e := range s.h.sceneEvents() {
		if strings.HasPrefix(e, kind+" ") {
			n++
		}
	}
	return n
}

func actionByID(scene *playv1.OpenSceneInfo, id string) *playv1.SceneActionView {
	for _, a := range scene.GetActions() {
		if a.GetId() == id {
			return a
		}
	}
	return nil
}

// MR-015, first criterion: the master picks Investigação, a Teste de Força
// and a "Teste de resistência de Sabedoria"; Pensantus's player reads the open scene and
// gets his own bonuses, the golden numbers, and no DC. Another player gets
// their own character's numbers.
func TestMR015_PlayerSeesTheMastersActionsWithTheirBonus(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	invest, force, wis := s.setup()
	opened := s.master.mustOpenScene(s.campaign, s.point.GetId())

	// The master: the DCs, and no character's numbers.
	if opened.GetPointId() != s.point.GetId() || opened.GetName() != "A carroça tombada" || opened.GetDescription() != sceneDescription {
		t.Errorf("the master's scene = %v, want the point's name and description", opened)
	}
	for _, want := range []struct {
		id    string
		name  string
		dc    int32
		check string
	}{
		{invest.GetId(), "Procurar pistas", 15, "Investigação"},
		{force.GetId(), "", 0, "Teste de Força"},
		{wis.GetId(), "", 12, "Teste de resistência de Sabedoria"},
	} {
		a := actionByID(opened, want.id)
		if a == nil || a.GetName() != want.name || a.GetDc() != want.dc || a.GetCheckName() != want.check || a.Bonus != nil || a.Passive != nil {
			t.Errorf("the master's action = %v, want %q %q DC %d and no bonus", a, want.name, want.check, want.dc)
		}
	}

	// Pensantus's player: his own numbers, no DC. The description is the
	// point's, the players read it in the open scene.
	ana := s.ana.getScene(s.campaign)
	if ana.GetDescription() != sceneDescription || len(ana.GetActions()) != 3 {
		t.Fatalf("Ana's scene = %v, want the description and 3 actions", ana)
	}
	for _, want := range []struct {
		id      string
		bonus   int32
		passive *int32
	}{
		{invest.GetId(), 6, proto.Int32(16)}, // Investigação +6, passive 16
		{force.GetId(), 1, nil},              // Força 12
		{wis.GetId(), 3, nil},                // Sabedoria 13 and the wizard's proficiency
	} {
		a := actionByID(ana, want.id)
		if a == nil || a.Bonus == nil || a.GetBonus() != want.bonus || a.GetDc() != 0 || !sameInt(a.Passive, want.passive) {
			t.Errorf("Ana's action = %v, want bonus %d, passive %v and no DC", a, want.bonus, want.passive)
		}
	}
	if a := actionByID(ana, invest.GetId()); a.GetName() != "Procurar pistas" || a.GetCheckName() != "Investigação" {
		t.Errorf("Ana's Investigação = %v, want the master's name and the check's", a)
	}

	// Another player has their own character's numbers: the default wizard,
	// Intelligence 18 with the gnome's bonus, and no proficiency.
	caio := s.caio.getScene(s.campaign)
	if a := actionByID(caio, invest.GetId()); a.GetBonus() != 4 || a.GetPassive() != 14 || a.GetDc() != 0 {
		t.Errorf("Caio's Investigação = %v, want +4, passive 14, no DC", a)
	}

	// The actions of a revealed point reach a player's map without a DC.
	s.master.setPointRevealed(s.campaign, s.point, true)
	for _, p := range s.ana.mustGetMap(s.campaign, s.mapID).GetPoints() {
		if len(p.GetSceneActions()) != 3 {
			t.Fatalf("Ana's point = %v, want 3 scene actions", p)
		}
		for _, a := range p.GetSceneActions() {
			if a.GetDc() != 0 {
				t.Errorf("a player's map action %v carries a DC", a)
			}
		}
	}
	var dcs []int32
	for _, a := range s.master.mustGetMap(s.campaign, s.mapID).GetPoints()[0].GetSceneActions() {
		dcs = append(dcs, a.GetDc())
	}
	if !slices.Equal(dcs, []int32{15, 0, 12}) {
		t.Errorf("the master's map DCs = %v, want 15, 0, 12 in order", dcs)
	}
}

// MR-015, second criterion: nothing that belongs to the combat is a scene
// action: an attack, a spell or a feature is refused when the master adds it
// or changes an action into it.
func TestMR015_NothingCombatOnlyInAScene(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	good := s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "skill:perception", "", 0)
	for _, key := range []string{
		"attack:longsword", "weapon:equipment:longsword", "spell:fire-bolt", "feature:second-wind", "feature:action-surge",
		"standard:attack", "standard:dash", "", "skill:", "skill:flying", "ability:luck", "save:xyz", "investigation", "SKILL:arcana",
	} {
		_, err := s.master.tryAddAction(s.campaign, s.mapID, s.point.GetId(), key, "", 0)
		wantCode(t, "adding "+key, err, connect.CodeInvalidArgument)
		_, err = s.update(s.master, good, func(r *mapsv1.UpdateSceneActionRequest) { r.Key = new(key) })
		wantCode(t, "changing an action to "+key, err, connect.CodeInvalidArgument)
	}
	if got := s.master.mustGetMap(s.campaign, s.mapID).GetPoints()[0].GetSceneActions(); len(got) != 1 || got[0].GetKey() != "skill:perception" {
		t.Errorf("the point's actions = %v, want only the Percepção check", got)
	}
	// The whole catalog is accepted: 18 skills, 6 abilities, 6 saves.
	p2 := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Tudo"})
	for _, k := range []string{"skill:acrobatics", "skill:animal-handling", "skill:sleight-of-hand", "ability:str", "ability:cha", "save:con", "save:dex"} {
		s.master.addAction(s.campaign, s.mapID, p2.GetId(), k, "", 0)
	}
}

// RN-20, question 52: a player never gets a DC, a pass or fail, or another
// player's roll, in any answer or any event of the stream, and the session
// history keeps ids and numbers only.
func TestRN20_APlayerNeverGetsADCOrAnotherPlayersRoll(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	invest, force, _ := s.setup()
	s.master.setPointRevealed(s.campaign, s.point, true) // so their map shows the point too
	masterWatch, anaWatch := s.master.watch(s.campaign), s.ana.watch(s.campaign)
	caioWatch := s.caio.watch(s.campaign)

	var seen []string // everything Ana was sent, as JSON
	see := func(m proto.Message) { seen = append(seen, protojson.Format(m)) }

	s.master.mustOpenScene(s.campaign, s.point.GetId())
	for _, w := range []*watcher{masterWatch, anaWatch, caioWatch} {
		if w.next().GetSceneChanged() == nil {
			t.Fatal("opening the scene sent no scene_changed")
		}
	}

	anaRoll := s.ana.mustRoll(s.campaign, invest.GetId(), 11) // 11 + 6 = 17, reaches DC 15
	see(anaRoll)
	caioRoll := s.caio.mustRoll(s.campaign, invest.GetId(), 3) // 3 + 4 = 7, misses
	s.caio.mustRoll(s.campaign, force.GetId(), 20)
	if anaRoll.GetRoll().GetTotal() != 17 || anaRoll.GetRoll().GetModifier() != 6 || anaRoll.GetRoll().GetFaces()[0] != 11 || !anaRoll.GetRoll().GetPhysical() {
		t.Errorf("Ana's roll = %v, want 1d20 (11) + 6 = 17, typed", anaRoll)
	}
	if anaRoll.Passed != nil || caioRoll.Passed != nil {
		t.Errorf("a roll's answer to its player carries a pass or fail: %v, %v", anaRoll, caioRoll)
	}

	see(s.ana.getScene(s.campaign))
	see(s.ana.mustGetMap(s.campaign, s.mapID))
	for _, text := range seen {
		for _, banned := range []string{"\"dc\"", "passed", s.other.GetId(), "Toren"} {
			if strings.Contains(text, banned) {
				t.Errorf("Ana was sent %q:\n%s", banned, text)
			}
		}
	}
	if scene := s.ana.getScene(s.campaign); len(scene.GetRolls()) != 1 || scene.GetRolls()[0].GetCharacterId() != s.pens.GetId() {
		t.Errorf("Ana's rolls = %v, want only her own", scene.GetRolls())
	}

	// The master: every roll, newest first, with the names and what passed.
	log := s.master.getScene(s.campaign).GetRolls()
	if len(log) != 3 {
		t.Fatalf("the master's log = %v, want 3 rolls", log)
	}
	type row struct {
		name   string
		action string
		total  int32
		passed *bool
	}
	var got []row
	for _, r := range log {
		got = append(got, row{r.GetCharacterName(), r.GetActionId(), r.GetRoll().GetTotal(), r.Passed})
	}
	want := []row{
		{"Toren", force.GetId(), 20, nil}, // Força 10: +0; the check has no DC: no pass or fail
		{"Toren", invest.GetId(), 7, new(false)},
		{"Pensantus", invest.GetId(), 17, new(true)},
	}
	for i := range want {
		if i >= len(got) || got[i].name != want[i].name || got[i].action != want[i].action || got[i].total != want[i].total || !sameBool(got[i].passed, want[i].passed) {
			t.Errorf("master's log[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// The stream: the master hears of every roll; Ana, of hers only.
	for range 3 {
		if ev := masterWatch.next(); ev.GetSceneCheckRolled() == nil {
			t.Fatalf("the master's event = %v, want scene_check_rolled", ev)
		}
	}
	if ev := anaWatch.next(); ev.GetSceneCheckRolled().GetActionId() != invest.GetId() {
		t.Fatalf("Ana's event = %v, want her own scene_check_rolled", ev)
	}
	s.master.closeScene(s.campaign)
	if ev := anaWatch.next(); ev.GetSceneChanged() == nil {
		t.Fatalf("Ana's next event = %v, want the close's scene_changed, after hers: she heard of no one else's roll", ev)
	}

	// The history: scene_opened, three rolls and scene_closed, ids and numbers
	// only: no name of a person or of an action, no DC.
	events := s.h.sceneEvents()
	if len(events) != 5 || !strings.HasPrefix(events[0], "scene_opened ") || !strings.HasPrefix(events[4], "scene_closed ") {
		t.Fatalf("the history = %v, want opened, three rolls, closed", events)
	}
	for _, e := range events {
		for _, banned := range []string{"Pensantus", "Toren", "Procurar pistas", "carroça", "\"dc\""} {
			if strings.Contains(e, banned) {
				t.Errorf("the history %q holds %q", e, banned)
			}
		}
	}
}

// sameBool compares two optional bools: nil is "unset".
func sameBool(a, b *bool) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// Question 55: a character rolls each action once while the scene is open. A
// second roll is refused with a typed reason; a retry with the same key
// returns the first roll and changes nothing; another action may be rolled;
// closing the scene and opening it again starts afresh.
func TestMR015_OneRollPerActionWhileTheSceneIsOpen(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	invest, force, _ := s.setup()
	s.master.mustOpenScene(s.campaign, s.point.GetId())

	key := newKey()
	first, err := s.ana.rollWith(s.campaign, invest.GetId(), 14, key)
	if err != nil {
		t.Fatalf("the first roll error = %v", err)
	}
	_, err = s.ana.rollWith(s.campaign, invest.GetId(), 19, newKey())
	wantSceneBlocked(t, "the second roll of the same action", err, playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_ALREADY_ROLLED)

	// A retry of the first call (the answer was lost) is answered with the
	// first roll, the same numbers, and writes nothing.
	again, err := s.ana.rollWith(s.campaign, invest.GetId(), 14, key)
	if err != nil || !proto.Equal(again, first) {
		t.Errorf("the retry = %v, %v; want the first roll %v", again, err, first)
	}
	if n := s.count("scene_check_rolled"); n != 1 {
		t.Errorf("%d scene_check_rolled events after the retry, want 1", n)
	}
	// The key belongs to that roll: another change with it is refused.
	_, err = s.ana.rollWith(s.campaign, force.GetId(), 10, key)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a roll with a used key and another action = %v, want invalid_argument", err)
	}

	// Another action, and another character, are free.
	if _, err := s.ana.rollWith(s.campaign, force.GetId(), 8, newKey()); err != nil {
		t.Errorf("rolling another action error = %v", err)
	}
	if _, err := s.caio.rollWith(s.campaign, invest.GetId(), 8, newKey()); err != nil {
		t.Errorf("another character rolling the same action error = %v", err)
	}
	if n := len(s.master.getScene(s.campaign).GetRolls()); n != 3 {
		t.Errorf("the log has %d rolls, want 3", n)
	}

	// Opening the same scene again changes nothing, the rolls stay...
	s.master.mustOpenScene(s.campaign, s.point.GetId())
	if _, err := s.ana.rollWith(s.campaign, invest.GetId(), 5, newKey()); err == nil {
		t.Error("rolling again after the master opened the same scene again succeeded")
	}
	if n := s.count("scene_opened"); n != 1 {
		t.Errorf("%d scene_opened events, want 1", n)
	}
	// ...closing it and opening it again starts afresh: the log is empty, and
	// the roll is allowed.
	s.master.closeScene(s.campaign)
	_, err = s.ana.rollWith(s.campaign, invest.GetId(), 5, newKey())
	wantSceneBlocked(t, "rolling with the scene closed", err, playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_OPEN_SCENE)
	s.master.mustOpenScene(s.campaign, s.point.GetId())
	if n := len(s.master.getScene(s.campaign).GetRolls()); n != 0 {
		t.Errorf("the log of the new opening has %d rolls, want none", n)
	}
	roll, err := s.ana.rollWith(s.campaign, invest.GetId(), 5, newKey())
	if err != nil || roll.GetRoll().GetTotal() != 11 {
		t.Errorf("rolling after reopening = %v, %v; want 5 + 6 = 11", roll, err)
	}
}

// RN-18, per roll: a typed d20 is 1 to 20; the campaign's setting decides
// whether the app's roll or a typed one is allowed; with "players choose" both
// are. The app's roll is a d20 plus the bonus.
func TestRN18_SceneRollsFollowTheDiceMode(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	invest, force, wis := s.setup()
	s.master.mustOpenScene(s.campaign, s.point.GetId())
	setMode := func(mode campaignsv1.DiceMode) {
		t.Helper()
		if _, err := s.master.campaigns.SetCampaignDiceMode(t.Context(), connect.NewRequest(&campaignsv1.SetCampaignDiceModeRequest{CampaignId: s.campaign, Mode: mode})); err != nil {
			t.Fatalf("SetCampaignDiceMode() error = %v", err)
		}
	}

	// A typed face out of 1 to 20, or no way of rolling, is invalid.
	for _, face := range []int32{-1, 21, 100} {
		_, err := s.ana.play.RollSceneCheck(t.Context(), connect.NewRequest(&playv1.RollSceneCheckRequest{
			CampaignId: s.campaign, ActionId: invest.GetId(), IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_D20Face{D20Face: face},
		}))
		wantCode(t, "a typed d20 of "+strconv.Itoa(int(face)), err, connect.CodeInvalidArgument)
	}
	_, err := s.ana.play.RollSceneCheck(t.Context(), connect.NewRequest(&playv1.RollSceneCheckRequest{CampaignId: s.campaign, ActionId: invest.GetId(), IdempotencyKey: newKey()}))
	wantCode(t, "no way of rolling", err, connect.CodeInvalidArgument)
	_, err = s.ana.play.RollSceneCheck(t.Context(), connect.NewRequest(&playv1.RollSceneCheckRequest{
		CampaignId: s.campaign, ActionId: invest.GetId(), IdempotencyKey: "not-a-uuid", Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true},
	}))
	wantCode(t, "a key that is not a UUID", err, connect.CodeInvalidArgument)
	// Nothing above rolled.
	if n := s.count("scene_check_rolled"); n != 0 {
		t.Fatalf("%d rolls after the refusals, want 0", n)
	}

	setMode(campaignsv1.DiceMode_DICE_MODE_APP)
	_, err = s.ana.rollWith(s.campaign, invest.GetId(), 12, newKey())
	wantSceneBlocked(t, "a typed roll when everybody rolls in the app", err, playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_WRONG_DICE_MODE)
	roll, err := s.ana.rollWith(s.campaign, invest.GetId(), 0, newKey())
	if err != nil {
		t.Fatalf("the app's roll error = %v", err)
	}
	d := roll.GetRoll()
	if len(d.GetFaces()) != 1 || d.GetFaces()[0] < 1 || d.GetFaces()[0] > 20 || d.GetModifier() != 6 || d.GetTotal() != d.GetFaces()[0]+6 || d.GetPhysical() || d.GetDiceCount() != 1 || d.GetDiceSides() != 20 {
		t.Errorf("the app's roll = %v, want a d20 (1 to 20) + 6, not physical", d)
	}

	setMode(campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	_, err = s.ana.rollWith(s.campaign, force.GetId(), 0, newKey())
	wantSceneBlocked(t, "the app's roll when everybody rolls real dice", err, playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_WRONG_DICE_MODE)
	typed, err := s.ana.rollWith(s.campaign, force.GetId(), 20, newKey())
	if err != nil || typed.GetRoll().GetTotal() != 21 || !typed.GetRoll().GetPhysical() {
		t.Errorf("the typed roll = %v, %v; want 20 + 1 = 21, physical", typed, err)
	}

	setMode(campaignsv1.DiceMode_DICE_MODE_PLAYERS_CHOOSE)
	if _, err := s.ana.rollWith(s.campaign, wis.GetId(), 0, newKey()); err != nil {
		t.Errorf("the app's roll when players choose error = %v", err)
	}
	if _, err := s.caio.rollWith(s.campaign, wis.GetId(), 7, newKey()); err != nil {
		t.Errorf("a typed roll when players choose error = %v", err)
	}
}

// RN-10, question 53: the master may open a hidden point, and it stays hidden
// on the map: the players read the scene, never the point.
func TestMR015_AHiddenPointCanBeOpenedAndStaysHidden(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	s.setup()
	if s.point.GetRevealed() {
		t.Fatal("a new point is revealed")
	}
	scene := s.master.mustOpenScene(s.campaign, s.point.GetId())
	if scene.GetName() != "A carroça tombada" {
		t.Errorf("the opened scene = %v", scene)
	}
	if got := s.ana.getScene(s.campaign); got.GetPointId() != s.point.GetId() || len(got.GetActions()) != 3 {
		t.Errorf("Ana's scene = %v, want the hidden point's scene with its 3 actions", got)
	}
	if pts := s.ana.mustGetMap(s.campaign, s.mapID).GetPoints(); len(pts) != 0 {
		t.Errorf("Ana's map has the points %v, want none: the point is still hidden", pts)
	}
	if got := s.master.mustGetMap(s.campaign, s.mapID).GetPoints()[0]; got.GetRevealed() {
		t.Errorf("the master's point = %v, want it still hidden", got)
	}
	// Ana rolls, as for any scene.
	if roll := s.ana.mustRoll(s.campaign, s.master.getScene(s.campaign).GetActions()[0].GetId(), 10); roll.GetRoll().GetTotal() != 16 {
		t.Errorf("Ana's roll = %v, want 10 + 6", roll)
	}
}

// Opening a scene: master only; the point must be a SCENE point of the
// campaign; during a session; one at a time;
// closing asks nothing; a point that stops being a scene, or is deleted,
// closes it; everyone hears.
func TestMR015_OpeningAndClosingAScene(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	invest, _, _ := s.setup()

	// No session open.
	_, err := s.master.openScene(s.campaign, s.point.GetId())
	wantBlocked(t, "OpenScene without a session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	s.master.start(s.campaign)

	// A point of another kind, another campaign's point, or one that does not
	// exist cannot be opened (a scene with no actions can: TestScenesWithNoActionsOpen).
	battle := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, Name: "Emboscada"})
	_, err = s.master.openScene(s.campaign, battle.GetId())
	wantCode(t, "opening a battle point", err, connect.CodeNotFound)
	otherCampaign := s.h.newCampaign(s.master)
	om := s.master.createMap(otherCampaign, "Outra mesa", s.master.newImage(otherCampaign))
	foreign := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: otherCampaign, MapId: om.GetId(), Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Alheia"})
	s.master.addAction(otherCampaign, om.GetId(), foreign.GetId(), "skill:arcana", "", 0)
	_, err = s.master.openScene(s.campaign, foreign.GetId())
	wantCode(t, "opening another campaign's scene", err, connect.CodeNotFound)
	_, err = s.master.openScene(s.campaign, newKey())
	wantCode(t, "opening a point that does not exist", err, connect.CodeNotFound)
	_, err = s.master.openScene(s.campaign, "not-a-uuid")
	wantCode(t, "opening a point that is not a UUID", err, connect.CodeNotFound)
	_, err = s.ana.openScene(s.campaign, s.point.GetId())
	wantCode(t, "a player opening a scene", err, connect.CodePermissionDenied)
	if _, err := s.ana.play.CloseScene(t.Context(), connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: s.campaign})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player closing a scene error = %v, want permission_denied", err)
	}
	if s.ana.getScene(s.campaign) != nil {
		t.Error("a scene is open after the refusals")
	}

	// Everyone hears, and a new session starts with no scene.
	masterWatch, anaWatch := s.master.watch(s.campaign), s.ana.watch(s.campaign)
	s.master.mustOpenScene(s.campaign, s.point.GetId())
	for name, w := range map[string]*watcher{"master": masterWatch, "Ana": anaWatch} {
		if !w.sceneChanged() {
			t.Errorf("%s heard nothing of the opening", name)
		}
	}

	// One at a time: another scene replaces the first.
	second := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "A ponte", Description: "Uma ponte de corda."})
	s.master.addAction(s.campaign, s.mapID, second.GetId(), "skill:athletics", "Atravessar", 10)
	if got := s.master.mustOpenScene(s.campaign, second.GetId()); got.GetName() != "A ponte" || len(got.GetActions()) != 1 {
		t.Errorf("the second scene = %v", got)
	}
	if got := s.ana.getScene(s.campaign); got.GetPointId() != second.GetId() {
		t.Errorf("Ana's scene = %v, want the second", got)
	}
	// Rolling the first scene's action is refused now: it is not the open scene's.
	_, err = s.ana.rollWith(s.campaign, invest.GetId(), 10, newKey())
	wantCode(t, "rolling an action of a scene that is not open", err, connect.CodeNotFound)

	// Changing an action of the open scene, or the point's text, reaches
	// everyone.
	for _, w := range []*watcher{masterWatch, anaWatch} {
		w.sceneChanged() // the replacement
	}
	s.master.addAction(s.campaign, s.mapID, second.GetId(), "ability:dex", "", 0)
	for name, w := range map[string]*watcher{"master": masterWatch, "Ana": anaWatch} {
		if !w.sceneChanged() {
			t.Errorf("%s heard nothing of the new action", name)
		}
	}
	if _, err := s.master.maps.UpdateMapPoint(t.Context(), connect.NewRequest(&mapsv1.UpdateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: second.GetId(), Description: new("A ponte caiu."),
	})); err != nil {
		t.Fatalf("UpdateMapPoint() error = %v", err)
	}
	for name, w := range map[string]*watcher{"master": masterWatch, "Ana": anaWatch} {
		if !w.sceneChanged() {
			t.Errorf("%s heard nothing of the new description", name)
		}
	}
	if got := s.ana.getScene(s.campaign); got.GetDescription() != "A ponte caiu." || len(got.GetActions()) != 2 {
		t.Errorf("Ana's scene after the changes = %v", got)
	}
	// Another point's actions are none of the open scene's business.
	s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "skill:medicine", "", 0)

	// Closing asks nothing, and with none open it changes nothing.
	s.master.closeScene(s.campaign)
	for name, w := range map[string]*watcher{"master": masterWatch, "Ana": anaWatch} {
		if !w.sceneChanged() {
			t.Errorf("%s heard nothing of the closing", name)
		}
	}
	if s.ana.getScene(s.campaign) != nil || s.master.getScene(s.campaign) != nil {
		t.Error("a scene is open after CloseScene")
	}
	s.master.closeScene(s.campaign)
	if n := s.count("scene_closed"); n != 1 {
		t.Errorf("%d scene_closed events, want 1", n)
	}

	// A point that stops being a scene, or is deleted, closes the scene.
	s.master.mustOpenScene(s.campaign, second.GetId())
	for _, w := range []*watcher{masterWatch, anaWatch} {
		w.sceneChanged()
	}
	if _, err := s.master.maps.UpdateMapPoint(t.Context(), connect.NewRequest(&mapsv1.UpdateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: second.GetId(), Kind: mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE.Enum(),
	})); err != nil {
		t.Fatalf("UpdateMapPoint(kind) error = %v", err)
	}
	if s.ana.getScene(s.campaign) != nil {
		t.Error("a point that is not a scene anymore is still the open scene")
	}
	anaWatch.sceneChanged()
	if got := s.master.mustGetMap(s.campaign, s.mapID); len(got.GetPoints()[len(got.GetPoints())-2].GetSceneActions()) != 0 {
		t.Errorf("a point that stopped being a scene still has actions: %v", got.GetPoints())
	}
	s.master.mustOpenScene(s.campaign, s.point.GetId())
	if _, err := s.master.maps.DeleteMapPoint(t.Context(), connect.NewRequest(&mapsv1.DeleteMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId()})); err != nil {
		t.Fatalf("DeleteMapPoint() error = %v", err)
	}
	if s.ana.getScene(s.campaign) != nil || s.master.getScene(s.campaign) != nil {
		t.Error("the scene of a deleted point is still open")
	}
}

// The actions of a scene point: at most 20, a name of up to 60 characters, a
// DC of 1 to 30 or none, only on a SCENE point; they change one by one, move
// up and down, and go away without asking.
func TestMR015_SceneActionRules(t *testing.T) {
	t.Parallel()
	s := newScenes(t, false)
	point := func() *mapsv1.MapPoint { return s.master.mustGetMap(s.campaign, s.mapID).GetPoints()[0] }

	// The 60-character name and DC 1 to 30.
	sixty := strings.Repeat("ã", 60)
	if a := s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "skill:stealth", sixty, 30); a.GetName() != sixty || a.GetDc() != 30 {
		t.Errorf("a 60-character name and DC 30 = %v, want both kept", a)
	}
	for name, c := range map[string]struct {
		n  string
		dc int32
	}{
		"a 61-character name":      {strings.Repeat("a", 61), 10},
		"a name with a line break": {"duas\nlinhas", 10},
		"a DC of 31":               {"ok", 31},
		"a negative DC":            {"ok", -1},
	} {
		_, err := s.master.tryAddAction(s.campaign, s.mapID, s.point.GetId(), "skill:arcana", c.n, c.dc)
		wantCode(t, name, err, connect.CodeInvalidArgument)
	}
	one := s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "skill:arcana", "  Ler a runa  ", 1)
	if one.GetName() != "Ler a runa" || one.GetDc() != 1 {
		t.Errorf("a trimmed name and DC 1 = %v", one)
	}

	// Update: one field at a time; "" and 0 remove the name and the DC.
	res, err := s.update(s.master, one, func(r *mapsv1.UpdateSceneActionRequest) { r.Dc = proto.Int32(0); r.Name = new("") })
	if err != nil || res.GetAction().GetDc() != 0 || res.GetAction().GetName() != "" || res.GetAction().GetKey() != "skill:arcana" {
		t.Errorf("clearing the name and the DC = %v, %v", res, err)
	}
	res, err = s.update(s.master, one, func(r *mapsv1.UpdateSceneActionRequest) { r.Key = new("save:con"); r.Dc = proto.Int32(14) })
	if err != nil || res.GetAction().GetKey() != "save:con" || res.GetAction().GetCheckName() != "Teste de resistência de Constituição" || res.GetAction().GetDc() != 14 {
		t.Errorf("changing the key and the DC = %v, %v", res, err)
	}
	_, err = s.update(s.master, one, func(*mapsv1.UpdateSceneActionRequest) {})
	wantCode(t, "an update with nothing to change", err, connect.CodeInvalidArgument)
	_, err = s.update(s.master, one, func(r *mapsv1.UpdateSceneActionRequest) { r.Dc = proto.Int32(31) })
	wantCode(t, "an update to DC 31", err, connect.CodeInvalidArgument)
	_, err = s.update(s.master, one, func(r *mapsv1.UpdateSceneActionRequest) { r.Name = new(strings.Repeat("a", 61)) })
	wantCode(t, "an update to a 61-character name", err, connect.CodeInvalidArgument)
	_, err = s.update(s.master, &mapsv1.SceneAction{Id: newKey()}, func(r *mapsv1.UpdateSceneActionRequest) { r.Dc = proto.Int32(5) })
	wantCode(t, "updating an action that does not exist", err, connect.CodeNotFound)

	// Order: add two more, move them, the ends stay.
	third := s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "ability:cha", "", 0)
	keys := func(as []*mapsv1.SceneAction) string {
		var out []string
		for _, a := range as {
			out = append(out, a.GetKey())
		}
		return strings.Join(out, " ")
	}
	if got := keys(point().GetSceneActions()); got != "skill:stealth save:con ability:cha" {
		t.Fatalf("the order = %s", got)
	}
	if got := keys(s.move(third, mapsv1.SceneActionDirection_SCENE_ACTION_DIRECTION_UP)); got != "skill:stealth ability:cha save:con" {
		t.Errorf("after moving the last up = %s", got)
	}
	if got := keys(s.move(third, mapsv1.SceneActionDirection_SCENE_ACTION_DIRECTION_UP)); got != "ability:cha skill:stealth save:con" {
		t.Errorf("after moving it up again = %s", got)
	}
	if got := keys(s.move(third, mapsv1.SceneActionDirection_SCENE_ACTION_DIRECTION_UP)); got != "ability:cha skill:stealth save:con" {
		t.Errorf("moving the first up changed the order: %s", got)
	}
	if got := keys(s.move(third, mapsv1.SceneActionDirection_SCENE_ACTION_DIRECTION_DOWN)); got != "skill:stealth ability:cha save:con" {
		t.Errorf("after moving it down = %s", got)
	}
	if got := keys(point().GetSceneActions()); got != "skill:stealth ability:cha save:con" {
		t.Errorf("the saved order = %s", got)
	}
	_, err = s.master.maps.MoveSceneAction(t.Context(), connect.NewRequest(&mapsv1.MoveSceneActionRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId(), ActionId: third.GetId()}))
	wantCode(t, "a move without a direction", err, connect.CodeInvalidArgument)

	// Remove, with no confirmation; a new action goes last, after the gaps.
	rm, err := s.master.maps.RemoveSceneAction(t.Context(), connect.NewRequest(&mapsv1.RemoveSceneActionRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId(), ActionId: third.GetId()}))
	if err != nil || keys(rm.Msg.GetActions()) != "skill:stealth save:con" {
		t.Errorf("RemoveSceneAction = %v, %v", rm, err)
	}
	_, err = s.master.maps.RemoveSceneAction(t.Context(), connect.NewRequest(&mapsv1.RemoveSceneActionRequest{CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId(), ActionId: third.GetId()}))
	wantCode(t, "removing an action twice", err, connect.CodeNotFound)
	s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "skill:history", "", 0)
	if got := keys(point().GetSceneActions()); got != "skill:stealth save:con skill:history" {
		t.Errorf("the order after a removal and an add = %s", got)
	}

	// The limit: 20 actions, a 21st is refused; a removal frees one.
	for range 17 {
		s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "skill:nature", "", 0)
	}
	if n := len(point().GetSceneActions()); n != 20 {
		t.Fatalf("the point has %d actions, want 20", n)
	}
	_, err = s.master.tryAddAction(s.campaign, s.mapID, s.point.GetId(), "skill:nature", "", 0)
	wantCode(t, "a 21st action", err, connect.CodeResourceExhausted)

	// Only a SCENE point has actions.
	battle := s.master.createPoint(&mapsv1.CreateMapPointRequest{CampaignId: s.campaign, MapId: s.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, Name: "Emboscada"})
	_, err = s.master.tryAddAction(s.campaign, s.mapID, battle.GetId(), "skill:nature", "", 0)
	wantCode(t, "an action on a battle point", err, connect.CodeInvalidArgument)
	_, err = s.master.tryAddAction(s.campaign, s.mapID, newKey(), "skill:nature", "", 0)
	wantCode(t, "an action on a point that does not exist", err, connect.CodeNotFound)
	_, err = s.master.tryAddAction(s.campaign, newKey(), s.point.GetId(), "skill:nature", "", 0)
	wantCode(t, "an action on a map that does not exist", err, connect.CodeNotFound)
	// The actions of another map's point are not reachable through this one.
	om := s.master.createMap(s.campaign, "Outro mapa", s.master.newImage(s.campaign))
	_, err = s.master.tryAddAction(s.campaign, om.GetId(), s.point.GetId(), "skill:nature", "", 0)
	wantCode(t, "an action on a point of another map", err, connect.CodeNotFound)

	// A point that stops being a scene loses its actions.
	if _, err := s.master.maps.UpdateMapPoint(t.Context(), connect.NewRequest(&mapsv1.UpdateMapPointRequest{
		CampaignId: s.campaign, MapId: s.mapID, PointId: s.point.GetId(), Kind: mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE.Enum(),
	})); err != nil {
		t.Fatalf("UpdateMapPoint(kind) error = %v", err)
	}
	if n := len(point().GetSceneActions()); n != 0 {
		t.Errorf("a battle point has %d scene actions, want 0", n)
	}
	var count int
	if err := s.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM scene_actions WHERE point_id = $1`, s.point.GetId()).Scan(&count); err != nil || count != 0 {
		t.Errorf("%d scene_actions rows left for the point, %v", count, err)
	}
}

// Rolling needs a living character of the caller's own; an outsider and a
// pending member are not_found (the full matrix is below).
func TestMR015_RollingNeedsALivingCharacter(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	invest, _, _ := s.setup()
	s.master.mustOpenScene(s.campaign, s.point.GetId())

	// A member with no character in the campaign.
	lone := s.h.newUser("Sem ficha")
	s.h.join(s.master, s.campaign, false, lone)
	_, err := lone.rollWith(s.campaign, invest.GetId(), 10, newKey())
	wantSceneBlocked(t, "a player with no character", err, playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER)
	if scene := lone.getScene(s.campaign); len(scene.GetActions()) != 3 || scene.GetActions()[0].Bonus != nil || len(scene.GetRolls()) != 0 {
		t.Errorf("the scene of a player with no character = %v, want the actions without bonuses", scene)
	}

	// A dead character cannot roll.
	if _, err := s.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: s.campaign, CharacterId: s.other.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	_, err = s.caio.rollWith(s.campaign, invest.GetId(), 10, newKey())
	wantSceneBlocked(t, "a dead character rolling", err, playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER)

	// An action that is not a UUID, or does not exist, is not found.
	_, err = s.ana.rollWith(s.campaign, "nope", 10, newKey())
	wantCode(t, "an action that is not a UUID", err, connect.CodeNotFound)
	_, err = s.ana.rollWith(s.campaign, newKey(), 10, newKey())
	wantCode(t, "an action that does not exist", err, connect.CodeNotFound)
	// No session: the usual blocked reason.
	end := s.master.play
	sessions, _ := end.ListGameSessions(t.Context(), connect.NewRequest(&playv1.ListGameSessionsRequest{CampaignId: s.campaign}))
	if _, err := end.EndGameSession(t.Context(), connect.NewRequest(&playv1.EndGameSessionRequest{CampaignId: s.campaign, GameSessionId: sessions.Msg.GetGameSessions()[0].GetId()})); err != nil {
		t.Fatalf("EndGameSession() error = %v", err)
	}
	_, err = s.ana.rollWith(s.campaign, invest.GetId(), 10, newKey())
	wantBlocked(t, "rolling with no session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	_, err = s.ana.play.GetOpenScene(t.Context(), connect.NewRequest(&playv1.GetOpenSceneRequest{CampaignId: s.campaign}))
	wantBlocked(t, "GetOpenScene with no session", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
}

// The authorization matrix of the scene RPCs of PlayService, with a real scene
// (play's own matrix has no maps): only the master opens and closes; any
// member reads; only a player rolls; an outsider and a pending member get
// not_found; a signed-out caller, unauthenticated.
func TestSceneAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	pending := s.h.newUser("Pendente")
	s.h.join(s.master, s.campaign, true, pending)
	s.setup()

	type call = func(ctx context.Context, u *user) error
	rows := []struct {
		name string
		call call
		// master, player, non-member, anonymous, pending
		want [5]connect.Code
	}{
		{"OpenScene", func(_ context.Context, u *user) error {
			_, err := u.openScene(s.campaign, s.point.GetId())
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"GetOpenScene", func(ctx context.Context, u *user) error {
			_, err := u.play.GetOpenScene(ctx, connect.NewRequest(&playv1.GetOpenSceneRequest{CampaignId: s.campaign}))
			return err
		}, [5]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// Each call rolls an action of its own, so the player's roll is allowed.
		{"RollSceneCheck", func(_ context.Context, u *user) error {
			a := s.master.addAction(s.campaign, s.mapID, s.point.GetId(), "skill:perception", "", 0)
			_, err := u.rollWith(s.campaign, a.GetId(), 10, newKey())
			return err
		}, [5]connect.Code{connect.CodePermissionDenied, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// The scene is open here (the OpenScene row opened it), so the master's
		// grant for an action of it is allowed.
		{"GrantSceneAttempt", func(_ context.Context, u *user) error {
			_, err := u.grantAttempt(s.campaign, s.master.getScene(s.campaign).GetActions()[0].GetId(), s.pens.GetId(), newKey())
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"CloseScene", func(ctx context.Context, u *user) error {
			_, err := u.play.CloseScene(ctx, connect.NewRequest(&playv1.CloseSceneRequest{CampaignId: s.campaign}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
	}
	covered := map[string]bool{}
	for _, r := range rows {
		covered[r.name] = true
	}
	methods := playv1.File_meurpg_play_v1_play_proto.Services().ByName("PlayService").Methods()
	for i := range methods.Len() {
		name := string(methods.Get(i).Name())
		if strings.Contains(name, "Scene") && !covered[name] {
			t.Errorf("PlayService.%s is missing from the scene authorization matrix", name)
		}
	}
	callers := []*user{s.master, s.ana, s.h.newUser("De fora"), s.h.anonymous(), pending}
	names := []string{"master", "player", "non-member", "anonymous", "pending"}
	for _, r := range rows {
		for i, u := range callers {
			wantCode(t, r.name+" as "+names[i], r.call(t.Context(), u), r.want[i])
		}
	}
}

// sceneChanged reads events until a scene_changed, skipping the map_changed
// the master also gets for a change to a point.
func (w *watcher) sceneChanged() bool {
	w.t.Helper()
	for {
		ev := w.next()
		if ev.GetSceneChanged() != nil {
			return true
		}
		if ev.GetMapChanged() == nil {
			w.t.Fatalf("event = %v, want scene_changed", ev)
		}
	}
}

func sameInt(a, b *int32) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
