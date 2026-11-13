package leaktest

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// The table screen (the TV at the table, RN-10). The master asks GetTableScreen and the TV shows the
// answer, so the answer is a player's: it is inspected as the TV's reader (tvReader), with the three layers of every
// read (inspect), and the master's own reads of the same things are the positive controls.
//
// The TV's reader is a player with no account and no character. It reads what the whole table reads: a canary that
// is public, or that both players may read (a stage NPC, an image the table was shown). It reads nothing that only
// Ana or only Caio does, a hidden thing, or a master's number. What the group sees (a map with "Visão do grupo")
// is added by the test that turns it on.
//
// Which test proves which leak row (tvFieldRows below):
//
//   - TestLeakMatrix, the read GetTableScreen (reads_test.go, asTV): the answer in the state of the fixture, asked
//     again after the stream's changes, and checkTableScreen: every pin, inspected;
//   - the stream (stream_test.go): the TV's stream carries nothing, a hidden move shows nothing, and the master alone
//     hears table_screen_changed;
//   - the rows asTV in the worlds with a combat of their own (attack rolls, table creatures, summons): the log and
//     the order after each attack, with the hidden attackers and the table content the master hid (RN-23);
//   - TestRN10_TheTVReadsNoSceneNorImageMeantForChosenPlayers: a scene and an image for chosen players;
//   - TestRN10_TheTVOfAFogMapReadsOnlyWhatTheGroupSees: the map and its tiles (`as=tv`) with "Visão do grupo";
//   - the tests of package play (tablescreen_project_test.go) prove the same rows one by one against the
//     projector: a private roll ("Só eu"), a log line only one player reads, the log line that names a table entry
//     the master hid (TestTableScreenLogNamesNoTableEntryTheMasterHid, TestTvLogLineNamesNoTableContent).

// tvScreenRead is the row of the TV's answer: the master asks, and the answer is inspected as the TV's.
var tvScreenRead = read{
	procedure: playv1connect.TableScreenServiceGetTableScreenProcedure, allow: masterOnlyRead, asTV: true,
	why: "the TV's screen is opened by the master alone; what it carries is what a player with no character reads, so the master's answer is inspected as the TV's (tablescreen_test.go)",
	req: func(w *world) proto.Message { return &playv1.GetTableScreenRequest{CampaignId: w.campaign} },
}

// tvScreenLogRead is tvScreenRead for a world whose combat has a log line when the row is asked.
var tvScreenLogRead = func() read { r := tvScreenRead; r.tvLog = true; return r }()

// tvReaderName is the name the TV's answers are kept under in the answers of the matrix.
const tvReaderName = "Tela da mesa"

// tvReader is the reader the TV's answers are inspected as. sees lists the kinds of canary the TV may read besides
// the ones the whole table does (what the group sees on a map).
func (w *world) tvReader(sees ...string) *person {
	both := []string{w.ana.name, w.caio.name}
	return &person{name: tvReaderName, reads: func(c *canary) bool {
		return c.public || (c.readers[both[0]] && c.readers[both[1]]) || slices.Contains(sees, c.kind)
	}}
}

// tvMustCarry are the kinds of canary the TV has to read in the matrix, and where: the positive control of each
// thing the TV may carry. (The things it may never carry are the canaries the TV's answers are inspected for.)
var tvMustCarry = map[string]string{
	"shown-image-gallery-name": "the image shown to the whole table (TvImage.name)",
	"scene-open-name":          "the scene open to the whole table (TvScene.name)",
	"scene-open-description":   "the scene open to the whole table (TvScene.description)",
	"merchant-name":            "an NPC the master put on the stage (TvStageNpc.name)",
}

// tvNever are kinds of canary that both players read, or that are public, and that the TV has no field for: a
// scene's actions, the name of the combat, the puzzles, the milestones. They are searched in every answer of the
// TV. (The image left with the players is searched in the pins of checkTableScreen only: the stream's changes show it.)
var tvNever = []string{"scene-open-action", "encounter-name", "puzzle-shown-name", "puzzle-shown-clue", "milestone-reached"}

// checkTVCarries finds the positive controls of the TV in the answers the matrix kept for it.
func checkTVCarries(t *testing.T, w *world, got *answers) {
	t.Helper()
	tv := w.tvReader()
	for kind, where := range tvMustCarry {
		found := false
		for _, c := range w.secrets.list {
			if c.kind != kind {
				continue
			}
			found = true
			if !got.holds(tv, c) {
				t.Errorf("the TV reads %s in no answer: %s; the positive control of the TV's rows is missing", c, where)
			}
		}
		if !found {
			t.Errorf("no canary of kind %q: the row for %s checks nothing", kind, where)
		}
	}
	for _, n := range w.needlesOf(tvNever...) {
		if !got.holdsNeedle(tv, n) {
			continue
		}
		t.Errorf("the TV reads %q, which it has no field for", n)
	}
}

// holdsNeedle says whether anything p read holds the text.
func (a *answers) holdsNeedle(p *person, needle string) bool {
	return slices.ContainsFunc(a.bodies[p.name], func(b []byte) bool { return bytes.Contains(b, []byte(needle)) })
}

// readScreen asks GetTableScreen as the master and inspects the answer as the TV's reader.
func (w *world) readScreen(t *testing.T, step string, tv *person, got *answers) *playv1.GetTableScreenResponse {
	t.Helper()
	res, err := w.master.screen.GetTableScreen(t.Context(), rq(&playv1.GetTableScreenRequest{CampaignId: w.campaign}))
	if err != nil {
		t.Fatalf("%s: GetTableScreen() error = %v", step, err)
	}
	body, err := protojson.Marshal(res.Msg)
	if err != nil {
		t.Fatalf("%s: marshal the answer: %v", step, err)
	}
	r := reply{status: http.StatusOK, body: body, msg: res.Msg}
	if got != nil {
		got.keep(tv, r)
	}
	for _, f := range w.inspect(tv, r, nil) {
		t.Errorf("%s: the TV: %s\n\tanswer: %s", step, f, shorten(body))
	}
	for _, n := range w.needlesOf(tvNever...) {
		if bytes.Contains(body, []byte(n)) {
			t.Errorf("%s: the TV reads %q, which it has no field for\n\tanswer: %s", step, n, shorten(body))
		}
	}
	return res.Msg
}

// chooseScreen has the master pick what the TV shows, and returns the revision of the choice it made.
func (w *world) chooseScreen(t *testing.T, revision int32, mirror bool, pin playv1.TableScreenKind) int32 {
	t.Helper()
	res, err := w.master.screen.SetTableScreen(t.Context(), rq(&playv1.SetTableScreenRequest{
		CampaignId: w.campaign, ChoiceRevision: revision, Choice: &playv1.TableScreenChoice{Mirror: mirror, Pin: pin},
	}))
	if err != nil {
		t.Fatalf("SetTableScreen(mirror=%v, pin=%v) error = %v", mirror, pin, err)
	}
	return res.Msg.GetChoiceRevision()
}

// checkTableScreen is the TV's turn in the matrix: the fixture has an image shown to the whole table, a combat with a
// hidden NPC, a boss of 4,321 hit points, a scene open to the whole table with a hidden NPC on its stage, a fog map
// with "Visão do grupo" off, a free roll "Só eu" and one for the table. The master picks each item in turn and the
// answer is inspected as the TV's: no hidden thing, no number, no field a player never reads.
func checkTableScreen(t *testing.T, w *world, got *answers) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table screen: %v", r)
		}
	}()
	tv := w.tvReader()
	const (
		idle   = playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE
		mapKd  = playv1.TableScreenKind_TABLE_SCREEN_KIND_MAP
		image  = playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE
		scene  = playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE
		combat = playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT
	)
	if len(w.needlesOf(tvNever...)) < len(tvNever) {
		t.Fatalf("a kind of tvNever has no canary: the check would search for nothing")
	}
	noLeft := func(name string, s proto.Message) {
		for _, n := range w.needlesOf("left-image-gallery-name") {
			if strings.Contains(protojsonText(s), n) {
				t.Errorf("%s: the TV reads %q, the name of an image left with the players, which is not the image shown", name, n)
			}
		}
	}
	// The master follows what he shows: the image shown to the whole table comes first.
	screen := w.readScreen(t, "mirror", tv, got)
	noLeft("mirror", screen)
	rev := screen.GetChoiceRevision()
	if screen.GetStage().GetKind() != image || screen.GetStage().GetImage().GetName() == "" {
		t.Errorf("the TV mirrors %v %v, want the image shown to the whole table", screen.GetStage().GetKind(), screen.GetStage().GetImage())
	}
	if !slices.Contains(screen.GetParty(), w.toren.GetName()) {
		t.Errorf("party = %v, want Caio's character in it", screen.GetParty())
	}

	pin := func(name string, kind playv1.TableScreenKind) *playv1.GetTableScreenResponse {
		rev = w.chooseScreen(t, rev, false, kind)
		s := w.readScreen(t, "pinned "+name, tv, got)
		noLeft("pinned "+name, s)
		return s
	}
	noMasterRoll := func(name string, s *playv1.GetTableScreenResponse) {
		if len(s.GetStage().GetRolls()) == 0 {
			t.Errorf("%s: the TV has no free roll, but Ana rolled for the table: the control of the \"Só eu\" row is missing", name)
		}
		for _, r := range s.GetStage().GetRolls() {
			if r.GetWho() == w.master.name {
				t.Errorf("%s: the TV carries a roll of the master's (%v), whose roll was \"Só eu\"", name, r)
			}
		}
	}

	s := pin("the scene", scene)
	if s.GetStage().GetKind() != scene || s.GetStage().GetScene().GetName() == "" {
		t.Errorf("the pinned scene = %v %v, want the scene open to the whole table", s.GetStage().GetKind(), s.GetStage().GetScene())
	}
	noMasterRoll("the scene", s)

	s = pin("the combat", combat)
	c := s.GetStage().GetCombat()
	if s.GetStage().GetKind() != combat || c == nil {
		t.Fatalf("the pinned combat = %v %v, want the combat", s.GetStage().GetKind(), c)
	}
	if m := c.GetMap(); m != nil && !m.GetHiddenByFog() {
		t.Errorf("the combat's map = %v, want hidden_by_fog and nothing else: the fog is on and \"Visão do grupo\" off", m)
	}
	noMasterRoll("the combat", s)
	// The positive control of the combat: the order holds the two player characters, Pensantus down at 0 hit points
	// (the word FALLEN) and Toren, both as players; and no master's NPC (a hidden one, nor the bandit unseen on the fog map).
	seen := map[string]*playv1.TvCombatant{}
	for _, o := range c.GetOrder() {
		seen[o.GetName()] = o
		if !o.GetIsPlayer() && (o.GetName() == w.hiddenNPC.GetName() || o.GetName() == w.bandit.GetName()) {
			t.Errorf("the TV's order holds %q, an NPC the table does not see", o.GetName())
		}
	}
	if o := seen[w.pens.GetName()]; o == nil || !o.GetIsPlayer() || o.GetState() != playv1.TvState_TV_STATE_FALLEN {
		t.Errorf("the TV's order holds Pensantus as %v, want a player character FALLEN (0 hit points): %v", o, c.GetOrder())
	}
	if o := seen[w.toren.GetName()]; o == nil || !o.GetIsPlayer() {
		t.Errorf("the TV's order holds Toren as %v, want a player character: %v", o, c.GetOrder())
	}

	// A fog map with "Visão do grupo" off is not the TV's: the pin falls to the idle screen and says why.
	s = pin("the map", mapKd)
	if s.GetStage().GetKind() != idle || s.GetNotice() != playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_MAP_NEEDS_GROUP_VISION || s.GetStage().GetMap() != nil {
		t.Errorf("the pinned fog map = %v, notice %v, map %v; want the idle screen, MAP_NEEDS_GROUP_VISION and no map", s.GetStage().GetKind(), s.GetNotice(), s.GetStage().GetMap())
	}
	s = pin("the idle screen", idle)
	if s.GetStage().GetKind() != idle {
		t.Errorf("the pinned idle screen = %v", s.GetStage().GetKind())
	}
	_ = pin("the image", image)
	w.chooseScreen(t, rev, true, image) // back to following the table
}

// checkTableScreenAfterTheScript asks the TV's screen again, for each pin, after the changes of the stream's script
// (hidden NPCs moved and hidden again, a scene's stage changed, an image taken back): the answer is inspected as before.
func checkTableScreenAfterTheScript(t *testing.T, w *world, got *answers) {
	t.Helper()
	tv := w.tvReader()
	rev := w.readScreen(t, "after the script, mirror", tv, got).GetChoiceRevision()
	for _, kind := range []playv1.TableScreenKind{
		playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE, playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE, playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT,
		playv1.TableScreenKind_TABLE_SCREEN_KIND_MAP, playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE,
	} {
		rev = w.chooseScreen(t, rev, false, kind)
		w.readScreen(t, "after the script, pinned "+kind.String(), tv, got)
	}
	w.chooseScreen(t, rev, true, playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE)
}

// ===== The same rows, by table: every field of the TV's messages has a leak row.

// tvFieldRows says, for every field of the messages the TV's answers are made of, who may read it and what the
// leak test searches there: the canaries the field must never hold, and the control. TestEveryTableScreenFieldHasALeakRow
// fails for a field that is not here, so a field added to the TV is a decision made here, with its row.
var tvFieldRows = map[protoreflect.FullName]string{
	"meurpg.play.v1.GetTableScreenResponse.choice_revision":         "the master's own number of his choice; a counter, no canary",
	"meurpg.play.v1.GetTableScreenResponse.choice":                  "the master's choice: an enum and a bool",
	"meurpg.play.v1.GetTableScreenResponse.stage":                   "the TV's screen: the fields of TableScreenStage",
	"meurpg.play.v1.GetTableScreenResponse.notice":                  "an enum: it names no hidden thing",
	"meurpg.play.v1.GetTableScreenResponse.campaign_name":           "the campaign's name, which every member reads",
	"meurpg.play.v1.GetTableScreenResponse.session_number":          "the number of the open session; the numbers of the fixture are never a session's",
	"meurpg.play.v1.GetTableScreenResponse.party":                   "living player characters: not a reserved character, an NPC nor a character waiting for approval (reserved-name, review-reason, NPC names); control: the master's GetLiveSession",
	"meurpg.play.v1.GetTableScreenResponse.options":                 "the master's panel: an enum and a bool for each item; the contract says the master's panel is its only reader",
	"meurpg.play.v1.GetTableScreenResponse.presence":                "whether a TV tab is connected: a bool and a time",
	"meurpg.play.v1.TableScreenPresence.open":                       "a bool",
	"meurpg.play.v1.TableScreenPresence.lost_at":                    "a time",
	"meurpg.play.v1.TableScreenChoice.mirror":                       "a bool",
	"meurpg.play.v1.TableScreenChoice.pin":                          "an enum",
	"meurpg.play.v1.TableScreenOption.kind":                         "an enum",
	"meurpg.play.v1.TableScreenOption.available":                    "a bool",
	"meurpg.play.v1.TableScreenOption.unavailable":                  "an enum: the reason an item cannot be picked names no item",
	"meurpg.play.v1.TableScreenStage.kind":                          "an enum",
	"meurpg.play.v1.TableScreenStage.idle":                          "nothing",
	"meurpg.play.v1.TableScreenStage.map":                           "TvMap",
	"meurpg.play.v1.TableScreenStage.image":                         "TvImage",
	"meurpg.play.v1.TableScreenStage.scene":                         "TvScene",
	"meurpg.play.v1.TableScreenStage.combat":                        "TvCombat",
	"meurpg.play.v1.TableScreenStage.rolls":                         "TvRoll",
	"meurpg.play.v1.TvImage.name":                                   "only the image shown to the whole table: not an image for chosen players, one left with the players, the fog map's picture (left-image-gallery-name, scene-image-a-gallery-name); control: shown-image-gallery-name (tvMustCarry)",
	"meurpg.play.v1.TvImage.url":                                    "the route of the shown image (the image route answers 404 to who may not read it: images_test.go)",
	"meurpg.play.v1.TvImage.width":                                  "a size in pixels",
	"meurpg.play.v1.TvImage.height":                                 "a size in pixels",
	"meurpg.play.v1.TvScene.name":                                   "only a scene open to the whole table, not one for chosen players (partial-scene-name); control: scene-open-name (tvMustCarry)",
	"meurpg.play.v1.TvScene.description":                            "same as the name; control: scene-open-description (tvMustCarry)",
	"meurpg.play.v1.TvScene.stage":                                  "TvStageNpc",
	"meurpg.play.v1.TvStageNpc.name":                                "the NPCs the table sees on the stage, not a hidden one (hidden-stage-name); control: merchant-name (tvMustCarry)",
	"meurpg.play.v1.TvStageNpc.speaking":                            "a bool",
	"meurpg.play.v1.TvMap.name":                                     "the current map's name, only when the map is the TV's (mapOnTV); control: fog-map-name (TestRN10_TheTVOfAFogMapReadsOnlyWhatTheGroupSees)",
	"meurpg.play.v1.TvMap.hidden_by_fog":                            "true when the fog is on and \"Visão do grupo\" off, and then nothing else is set (checkTableScreen, the combat's map)",
	"meurpg.play.v1.TvMap.map":                                      "the player form of the map from the maps module: the masterOnly rules of maps (hidden point, trap, treasure, light, hidden token) and the canaries of the fog map; control: the master's GetMap",
	"meurpg.play.v1.TvMap.vision":                                   "the squares the group sees or remembers; a fogged cell has no tile (images_test.go and TestRN10_TheTVOfAFogMapReadsOnlyWhatTheGroupSees); control: the same test, with group vision on, reads the map and fetches a tile",
	"meurpg.play.v1.TvMap.revision":                                 "a hash of what the TV reads, never the map's revision; a number no canary has",
	"meurpg.play.v1.TvCombat.round":                                 "the round, a small number",
	"meurpg.play.v1.TvCombat.order":                                 "TvCombatant",
	"meurpg.play.v1.TvCombat.masters_turn":                          "a bool: the hidden combatant's name is not told on its turn",
	"meurpg.play.v1.TvCombat.log":                                   "the lines every player reads: no master-only line, no hidden combatant, no NPC dice, no table content the master hid (RN-23: proved in package play, and by the rows asTV of the worlds with a combat); control: the TV reads the combat in the worlds of table creatures and summons",
	"meurpg.play.v1.TvCombat.map":                                   "TvMap",
	"meurpg.play.v1.TvCombatant.name":                               "only the combatants the table sees: not a hidden NPC, not one unseen on a fog map (mon-group, dragon-name, bandit-name); control: checkTableScreen (Pensantus FALLEN, Toren) and the creature world (fight-creature-name in the order)",
	"meurpg.play.v1.TvCombatant.initial":                            "the first letter of the name above",
	"meurpg.play.v1.TvCombatant.state":                              "an enum, the RN-20 word: never the hit points, the armor class or the initiative (4,321 hit points; secret numbers are searched in every numeric field)",
	"meurpg.play.v1.TvCombatant.on_turn":                            "a bool",
	"meurpg.play.v1.TvCombatant.is_player":                          "a bool",
	"meurpg.play.v1.TvRoll.who":                                     "the roller of a roll for the whole table (Ana's), never the master's \"Só eu\" roll (checkTableScreen: noMasterRoll, free-roll-secret)",
	"meurpg.play.v1.TvRoll.expression":                              "the formula of a roll for the whole table",
	"meurpg.play.v1.TvRoll.total":                                   "the total of a roll for the whole table",
	"meurpg.play.v1.TvRoll.faces":                                   "the faces of a roll for the whole table",
	"meurpg.play.v1.SetTableScreenResponse.choice_revision":         "the master's own number of his choice",
	"meurpg.play.v1.SetTableScreenResponse.choice":                  "the master's choice",
	"meurpg.play.v1.WatchTableScreenResponse.ready":                 "the stream's events carry nothing: checkTVEvents (stream_test.go)",
	"meurpg.play.v1.WatchTableScreenResponse.heartbeat":             "same",
	"meurpg.play.v1.WatchTableScreenResponse.changed":               "same: not a revision, not a part, not a count",
	"meurpg.play.v1.WatchTableScreenResponse.session_ended":         "same",
	"meurpg.play.v1.WatchTableScreenResponse.Ready.session_seq":     "the session's hint number: a counter the master's tab reads to know it is behind; never derived from content, so a hidden write moves it like any other; no canary",
	"meurpg.play.v1.WatchTableScreenResponse.Heartbeat.session_seq": "same",
}

// tvMessages are the messages whose fields tvFieldRows lists, by name.
func tvMessages() []protoreflect.MessageDescriptor {
	file := (&playv1.GetTableScreenResponse{}).ProtoReflect().Descriptor().ParentFile()
	var out []protoreflect.MessageDescriptor
	var visit func(ms protoreflect.MessageDescriptors)
	visit = func(ms protoreflect.MessageDescriptors) {
		for i := range ms.Len() {
			m := ms.Get(i)
			if !strings.HasSuffix(string(m.Name()), "Request") {
				out = append(out, m)
			}
			visit(m.Messages())
		}
	}
	visit(file.Messages())
	return out
}

// TestEveryTableScreenFieldHasALeakRow needs no database. A field of a message the TV reads has to be in
// tvFieldRows, and a row has to name a field that exists.
func TestEveryTableScreenFieldHasALeakRow(t *testing.T) {
	seen := map[protoreflect.FullName]bool{}
	for _, m := range tvMessages() {
		for i := range m.Fields().Len() {
			f := m.Fields().Get(i)
			seen[f.FullName()] = true
			if strings.TrimSpace(tvFieldRows[f.FullName()]) == "" {
				t.Errorf("%s has no leak row: say who may read it and what the leak test searches there (tvFieldRows, tablescreen_test.go)", f.FullName())
			}
		}
	}
	for name := range tvFieldRows {
		if !seen[name] {
			t.Errorf("tvFieldRows names %s, which is not a field of the table screen any more", name)
		}
	}
}

// TestTheTVReaderReadsWhatTheWholeTableReads needs no database: the TV's reader is the players' common ground.
func TestTheTVReaderReadsWhatTheWholeTableReads(t *testing.T) {
	w, ana, caio := unitWorld()
	both := w.secrets.marker("stage-npc", ana, caio)
	onlyAna := w.secrets.marker("entrance", ana)
	nobody := w.secrets.marker("hidden-npc")
	public := w.secrets.public("scene-name")
	sight := w.secrets.marker("entrance-sight", ana)
	r := asReply(&playv1.TvScene{Name: both + public + onlyAna + nobody + sight})

	got := w.inspect(w.tvReader(), r, nil)
	if len(got) != 3 || !strings.Contains(strings.Join(got, "|"), "entrance ") || !strings.Contains(strings.Join(got, "|"), "hidden-npc") {
		t.Errorf("the TV: found %v, want what Ana alone reads and what nobody does (the whole table's and the public are the TV's)", got)
	}
	if got := w.inspect(w.tvReader("entrance-sight"), asReply(&playv1.TvScene{Name: sight}), nil); len(got) != 0 {
		t.Errorf("the TV that reads what the group sees found %v in a thing the group sees", got)
	}
}

// ===== The stream.

// tvWatcher reads one WatchTableScreen stream in the background.
type tvWatcher struct {
	mu     sync.Mutex
	events []*playv1.WatchTableScreenResponse
	more   chan struct{}
	done   chan struct{}
	err    error
}

func (tw *tvWatcher) count(kind string) int {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	n := 0
	for _, ev := range tw.events {
		if tvEventCase(ev) == kind {
			n++
		}
	}
	return n
}

func tvEventCase(ev *playv1.WatchTableScreenResponse) string {
	fd := ev.ProtoReflect().WhichOneof(ev.ProtoReflect().Descriptor().Oneofs().ByName("event"))
	if fd == nil {
		return ""
	}
	return string(fd.Name())
}

// watchTV opens the TV's stream as the master and waits for its first event (ready).
func (w *world) watchTV(p *person) *tvWatcher {
	ctx, cancel := context.WithCancel(w.t.Context())
	w.t.Cleanup(cancel)
	stream, err := p.screen.WatchTableScreen(ctx, connect.NewRequest(&playv1.WatchTableScreenRequest{CampaignId: w.campaign}))
	if err != nil {
		panic(err)
	}
	tw := &tvWatcher{done: make(chan struct{}), more: make(chan struct{}, 1)}
	first := make(chan struct{})
	go func() {
		defer close(tw.done)
		defer func() { _ = stream.Close() }()
		opened := false
		for stream.Receive() {
			tw.mu.Lock()
			tw.events = append(tw.events, proto.Clone(stream.Msg()).(*playv1.WatchTableScreenResponse))
			tw.mu.Unlock()
			select {
			case tw.more <- struct{}{}:
			default:
			}
			if !opened {
				opened = true
				close(first)
			}
		}
		tw.err = stream.Err()
		if !opened {
			close(first)
		}
	}()
	select {
	case <-first:
	case <-time.After(20 * time.Second):
		panic("the TV's stream never sent ready")
	}
	return tw
}

// checkTVStreamIsRefused: only the master opens the TV's stream. A player, a pending member, a stranger and someone
// with no session are refused, and what the refusal says names nothing.
func checkTVStreamIsRefused(t *testing.T, w *world) {
	t.Helper()
	for _, p := range []*person{w.ana, w.caio, w.pending, w.stranger, w.anonymous()} {
		stream, err := p.screen.WatchTableScreen(t.Context(), connect.NewRequest(&playv1.WatchTableScreenRequest{CampaignId: w.campaign}))
		if err == nil {
			// a Connect stream reports the refusal on the first Receive
			if stream.Receive() {
				t.Errorf("%s opened the TV's stream and got %v", p.name, stream.Msg())
			}
			err = stream.Err()
			_ = stream.Close()
		}
		if err == nil {
			t.Errorf("%s opened the TV's stream", p.name)
			continue
		}
		r := reply{status: http.StatusForbidden, body: []byte(err.Error())}
		for _, f := range w.inspect(p, r, nil) {
			t.Errorf("%s: the refusal of the TV's stream: %s\n\terror: %s", p.name, f, err)
		}
	}
}

// checkTVEvents: every event of the TV's stream is ready, heartbeat, changed or session_ended, with no field set (the ready
// and the heartbeat carry the session's number, a counter).
// A change the script made, the hidden NPC's moves included, produced only `changed`.
func checkTVEvents(t *testing.T, w *world, tw *tvWatcher, got *answers) {
	t.Helper()
	tv := w.tvReader()
	seen := map[string]int{}
	tw.mu.Lock()
	events := slices.Clone(tw.events)
	tw.mu.Unlock()
	for _, ev := range events {
		kind := tvEventCase(ev)
		seen[kind]++
		switch kind {
		case "ready", "heartbeat", "changed", "session_ended":
		default:
			t.Errorf("the TV's stream sent %q, which is not one of ready, heartbeat, changed and session_ended: %v", kind, ev)
			continue
		}
		fields := 0
		ev.ProtoReflect().Get(ev.ProtoReflect().Descriptor().Oneofs().ByName("event").Fields().ByName(protoreflect.Name(kind))).Message().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
			if fd.Name() != "session_seq" { // the session's hint number on ready and heartbeat: a counter (tvFieldRows)
				fields++
			}
			return true
		})
		if fields != 0 {
			t.Errorf("the TV's %s event has %d fields set besides session_seq, want none: %v", kind, fields, ev)
		}
		body, err := protojson.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal an event: %v", err)
		}
		r := reply{status: http.StatusOK, body: body, msg: ev}
		got.keep(tv, r)
		for _, f := range w.inspect(tv, r, nil) {
			t.Errorf("the TV's stream: event %s: %s\n\tevent: %s", kind, f, shorten(body))
		}
	}
	if seen["ready"] != 1 || seen["changed"] == 0 || seen["session_ended"] != 1 {
		t.Errorf("the TV's stream got %v, want one ready, a changed for what the script did and one session_ended", seen)
	}
}

// checkAHiddenMoveShowsNothing: a hidden NPC moves in the combat, the TV's answer is the same as before. The master's
// own stream knows of the move; the TV reads only GetTableScreen. The master has pinned the combat for the check.
func checkAHiddenMoveShowsNothing(t *testing.T, w *world, tw *tvWatcher, got *answers) {
	t.Helper()
	tv := w.tvReader()
	pinned := w.readScreen(t, "before the hidden move", tv, nil)
	heard := tw.count("changed")
	rev := w.chooseScreen(t, pinned.GetChoiceRevision(), false, playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT)
	before := w.readScreen(t, "the combat, before the hidden move", tv, got)
	if before.GetStage().GetKind() != playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT {
		t.Fatalf("the pinned combat = %v, want the combat", before.GetStage().GetKind())
	}
	bandit := w.combatant(w.bandit)
	if !bandit.GetHidden() {
		t.Fatalf("the bandit is not hidden: the check needs a hidden NPC")
	}
	waitChanged(t, tw, heard, "the choice of the combat") // the choice itself is heard first
	changed := tw.count("changed")
	must(w.master.combat.MoveCombatant(t.Context(), rq(&playv1.MoveCombatantRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: bandit.GetId(), IdempotencyKey: newKey(), Col: 22, Row: 3, Forced: true,
	})))
	waitChanged(t, tw, changed, "the hidden NPC's move")
	after := w.readScreen(t, "the combat, after the hidden move", tv, got)
	if !proto.Equal(before, after) {
		t.Errorf("a hidden NPC's move changed what the TV shows:\n\tbefore: %v\n\tafter:  %v", before, after)
	}
	w.chooseScreen(t, rev, true, playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT) // follow the table again
}

// waitChanged waits until the TV's stream has more than n `changed`, and fails if it does not within ten seconds.
func waitChanged(t *testing.T, tw *tvWatcher, n int, what string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for tw.count("changed") <= n {
		select {
		case <-tw.more:
		case <-deadline:
			t.Errorf("%s caused no `changed` on the TV's stream: the master's stream hears it, so the TV's must", what)
			return
		}
	}
}

// checkTableScreenChangedIsTheMasters: the master's stream hears table_screen_changed (a choice, a TV tab coming and
// going), and no player's does (RN-10).
func checkTableScreenChangedIsTheMasters(t *testing.T, w *world, watchers map[*person]*streamWatcher) {
	t.Helper()
	for p, sw := range watchers {
		n := 0
		for _, ev := range sw.since(0) {
			if ev.GetTableScreenChanged() != nil {
				n++
			}
		}
		switch {
		case p == w.master && n == 0:
			t.Errorf("the master's stream got no table_screen_changed, though he chose what the TV shows and a TV tab connected")
		case p != w.master && n != 0:
			t.Errorf("%s's stream got %d table_screen_changed: only the master hears of the TV (RN-10)", p.name, n)
		}
	}
}

// ===== The scene and the image for chosen players.

func TestRN10_TheTVReadsNoSceneNorImageMeantForChosenPlayers(t *testing.T) {
	w := newWorld(t)
	tv := w.tvReader()
	ps := w.buildPartialScene()
	w.openSceneTo(ps.point.GetId(), w.ana)
	must(w.master.play.PutOnStage(t.Context(), rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: ps.npc})))
	im := w.newShown("tv-chosen-image", w.ana)
	w.showImage(im, false, chosen(w.ana))

	// The positive controls: the master reads the scene's needles, and fetches the image.
	scene := w.openSceneAs(w.master)
	for _, n := range ps.needles {
		if !bytes.Contains(scene.body, []byte(n)) {
			t.Errorf("the master's open scene lacks %q, so the check proves nothing: %s", n, shorten(scene.body))
		}
	}
	if status, _ := w.master.get("/images/" + im.id); status != http.StatusOK {
		t.Errorf("the master gets HTTP %d for the image, want 200", status)
	}

	needles := append(slices.Clone(ps.needles), im.needles...)
	rev := w.readScreen(t, "mirror", tv, nil).GetChoiceRevision()
	steps := []struct {
		name   string
		mirror bool
		pin    playv1.TableScreenKind
		notice playv1.TableScreenNotice // wanted when the pin falls to the idle screen
	}{
		{"mirror", true, playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE, playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_UNSPECIFIED},
		{"the scene pinned", false, playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE, playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_SCENE_NOT_FOR_ALL},
		{"the image pinned", false, playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE, playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_PIN_ENDED},
	}
	for _, s := range steps {
		rev = w.chooseScreen(t, rev, s.mirror, s.pin)
		screen := w.readScreen(t, s.name, tv, nil)
		if n, found := holdsAny([]byte(protojsonText(screen)), needles); found {
			t.Errorf("%s: the TV reads %q, which is meant for chosen players", s.name, n)
		}
		if k := screen.GetStage().GetKind(); k == playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE || k == playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE {
			t.Errorf("%s: the TV shows %v, which is meant for chosen players", s.name, k)
		}
		if !s.mirror && screen.GetNotice() != s.notice {
			t.Errorf("%s: notice = %v, want %v", s.name, screen.GetNotice(), s.notice)
		}
	}
}

// protojsonText is the answer as the app receives it.
func protojsonText(m proto.Message) string {
	b, err := protojson.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// ===== The map and its tiles, with "Visão do grupo".

// groupSees are the kinds of canary of the fog map that the group sees: what Ana's character or Caio's sees, which
// the TV reads as soon as the group's vision is on. What a character knows (a trap revealed to it) or owns is not.
var groupSees = []string{
	"entrance-name", "entrance-description", "treasure-revealed-name", "south-treasure-name",
	"submap-name", "submap-description", "seen-npc-name",
}

// checkTVTiles asks the route of the TV's tiles (`as=tv`) as every person, and every answer but the master's with
// "Visão do grupo" on is the answer for a tile that does not exist. tilesPath and tile come from the vision.
func checkTVTiles(t *testing.T, w *world, step, tilesPath string, tile *mapsv1.MapTile, masterGets bool) {
	t.Helper()
	path := tilesPath + strconv.Itoa(int(tile.GetTx())) + "/" + strconv.Itoa(int(tile.GetTy())) + "?as=tv"
	missing := tilesPath + "99/99?as=tv"
	for _, p := range append(w.players(), w.master, w.anonymous()) {
		status, body := p.get(path)
		wantStatus, wantBody := p.get(missing)
		if p == w.master && masterGets {
			if status != http.StatusOK {
				t.Errorf("%s: the master got HTTP %d for the TV's tile %s, want 200 (the positive control)", step, status, path)
			}
			continue
		}
		if status == http.StatusOK {
			t.Errorf("%s: %s got the TV's tile %s (RN-10)", step, p.name, path)
		}
		if status != wantStatus || !bytes.Equal(body, wantBody) {
			t.Errorf("%s: %s got HTTP %d %q for the TV's tile, want what a tile that does not exist answers: HTTP %d %q", step, p.name, status, shorten(body), wantStatus, shorten(wantBody))
		}
		for _, f := range w.inspect(p, reply{status: status, body: body}, nil) {
			t.Errorf("%s: %s: %s", step, p.name, f)
		}
	}
}

func TestRN10_TheTVOfAFogMapReadsOnlyWhatTheGroupSees(t *testing.T) {
	w := newWorld(t)
	tv := w.tvReader(groupSees...)
	setGroupVision := func(on bool) {
		must(w.master.maps.SetMapFog(t.Context(), rq(&mapsv1.SetMapFogRequest{CampaignId: w.campaign, MapId: w.fogMap, GroupVision: new(on)})))
	}
	mapKind := playv1.TableScreenKind_TABLE_SCREEN_KIND_MAP

	// "Visão do grupo" off: the TV has nothing of the map.
	rev := w.readScreen(t, "mirror, group vision off", tv, nil).GetChoiceRevision()
	w.chooseScreen(t, rev, false, mapKind)
	off := w.readScreen(t, "the map, group vision off", tv, nil)
	if off.GetStage().GetKind() != playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE || off.GetStage().GetMap() != nil {
		t.Errorf("the fog map without group vision = %v %v, want the idle screen and no map", off.GetStage().GetKind(), off.GetStage().GetMap())
	}

	// On: the map as the group sees it.
	setGroupVision(true)
	on := w.readScreen(t, "the map, group vision on", tv, nil)
	m := on.GetStage().GetMap()
	if on.GetStage().GetKind() != mapKind || m == nil || m.GetHiddenByFog() {
		t.Fatalf("the fog map with group vision = %v %v, want the map, shown", on.GetStage().GetKind(), m)
	}
	if !strings.Contains(protojsonText(on), w.needlesOf("fog-map-name")[0]) {
		t.Errorf("the TV's map lacks the map's name: the positive control of the map's rows is missing")
	}
	if len(m.GetVision().GetTiles()) == 0 || m.GetVision().GetTilesPath() == "" {
		t.Fatalf("the TV's vision has no tile: %v", m.GetVision())
	}
	for _, tile := range m.GetVision().GetTiles() {
		checkTVTiles(t, w, "group vision on", m.GetVision().GetTilesPath(), tile, true)
	}

	// Off again: the master asks for the same tile and gets the answer of a tile that does not exist.
	setGroupVision(false)
	checkTVTiles(t, w, "group vision off", m.GetVision().GetTilesPath(), m.GetVision().GetTiles()[0], false)
}
