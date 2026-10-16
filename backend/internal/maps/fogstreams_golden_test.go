package maps

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Characterization of who is told of each write to a token and of what each player sees
// afterwards (MR-036, RN-10, D6): the live events every viewer of the table receives after
// each step of a script, on the cave with the fog on and on a map without fog, and the views
// (GetMapVision) the script leaves each player with, memory included. Each answer is a golden
// file (testdata/fog_*.golden.*); run with -update-golden to write them again after a
// change of behavior that was meant.

// fogStreamScript runs the steps of a table and writes down, after each one, what every
// viewer's stream got.
type fogStreamScript struct {
	t        *testing.T
	master   *user
	campaign string
	mapID    string
	probe    string
	viewers  []string
	streams  []*watcher
	labels   map[string]string
	out      strings.Builder
}

func newFogStreamScript(t *testing.T, master *user, campaign, mapID, probe string, names []string, viewers []*user) *fogStreamScript {
	t.Helper()
	sc := &fogStreamScript{t: t, master: master, campaign: campaign, mapID: mapID, probe: probe, viewers: append([]string{"master"}, names...), labels: map[string]string{mapID: "map", probe: "probe"}}
	sc.streams = append(sc.streams, master.watch(campaign))
	for _, u := range viewers {
		sc.streams = append(sc.streams, u.watch(campaign))
	}
	sc.drain() // what the setup left in the streams
	return sc
}

func (sc *fogStreamScript) name(id, label string) { sc.labels[id] = label }

// line says an event in a few words.
func (sc *fogStreamScript) line(ev *playv1.WatchGameSessionResponse) string {
	label := func(id string) string {
		if l, ok := sc.labels[id]; ok {
			return l
		}
		return "other"
	}
	switch {
	case ev.GetMapChanged() != nil:
		return "map_changed " + label(ev.GetMapChanged().GetMapId())
	case ev.GetVisionChanged() != nil:
		return "vision_changed " + label(ev.GetVisionChanged().GetMapId())
	case ev.GetTokenMoved() != nil:
		tm := ev.GetTokenMoved()
		return fmt.Sprintf("token_moved %s %s (%d, %d)", label(tm.GetMapId()), label(tm.GetCharacterId()), tm.GetXBp(), tm.GetYBp())
	}
	return fmt.Sprintf("other event %T", ev.GetEvent())
}

// drain toggles the probe map to end every stream, and returns the lines of each.
func (sc *fogStreamScript) drain() [][]string {
	sc.t.Helper()
	sc.master.setMapRevealed(sc.campaign, sc.probe, !sc.master.mustGetMap(sc.campaign, sc.probe).GetMap().GetRevealed())
	out := make([][]string, len(sc.streams))
	for i, w := range sc.streams {
		for _, ev := range w.drain(sc.probe) {
			if ev.GetMapChanged().GetMapId() == sc.probe {
				continue // a probe's trailing event
			}
			out[i] = append(out[i], sc.line(ev))
		}
	}
	return out
}

// step runs a write and writes down who heard what.
func (sc *fogStreamScript) step(name string, do func()) {
	sc.t.Helper()
	do()
	fmt.Fprintf(&sc.out, "== %s\n", name)
	for i, lines := range sc.drain() {
		if len(lines) == 0 {
			fmt.Fprintf(&sc.out, "%s: -\n", sc.viewers[i])
			continue
		}
		fmt.Fprintf(&sc.out, "%s: %s\n", sc.viewers[i], strings.Join(lines, "; "))
	}
}

func (sc *fogStreamScript) golden(name string) { checkGolden(sc.t, name, []byte(sc.out.String())) }

func (u *user) removeToken(campaignID, mapID, characterID string) {
	u.h.t.Helper()
	if _, err := u.maps.RemoveMapToken(u.h.t.Context(), connect.NewRequest(&mapsv1.RemoveMapTokenRequest{CampaignId: campaignID, MapId: mapID, CharacterId: characterID})); err != nil {
		u.h.t.Fatalf("RemoveMapToken() error = %v", err)
	}
}

func (u *user) carry(campaignID, mapID, characterID, key string) {
	u.h.t.Helper()
	if _, err := u.maps.SetCarriedLight(u.h.t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{CampaignId: campaignID, MapId: mapID, CharacterId: characterID, LightKey: key})); err != nil {
		u.h.t.Fatalf("SetCarriedLight() error = %v", err)
	}
}

// TestFogStreams_WhoIsToldOfEachTokenWriteWithFog: every kind of token write on the cave, and
// the events each viewer gets for it; then the views the writes leave.
func TestFogStreams_WhoIsToldOfEachTokenWriteWithFog(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	m := c.master
	sc := newFogStreamScript(t, m, c.campaign, c.mapID, c.probeMap, []string{"ana", "caio", "bia"}, []*user{c.ana, c.caio, c.bia})
	for id, label := range map[string]string{
		c.pens.GetId(): "pens", c.toren.GetId(): "toren", c.brisa.GetId(): "brisa", c.salvia.GetId(): "salvia",
		c.goblin1.GetId(): "goblin1", c.goblin2.GetId(): "goblin2", c.captain.GetId(): "captain", c.hiddenGoblin.GetId(): "hiddenGoblin",
	} {
		sc.name(id, label)
	}
	place := func(ch string, col, row int) func() {
		return func() { m.placeAt(c.campaign, c.mapID, ch, grid.Square{Col: col, Row: row}) }
	}
	hide := func(ch string, hidden bool) func() {
		return func() { m.setTokenHidden(c.campaign, c.mapID, ch, hidden) }
	}
	remove := func(ch string) func() { return func() { m.removeToken(c.campaign, c.mapID, ch) } }
	torch := func(ch, key string) func() { return func() { m.carry(c.campaign, c.mapID, ch, key) } }
	g1, g2, hg, cpt := c.goblin1.GetId(), c.goblin2.GetId(), c.hiddenGoblin.GetId(), c.captain.GetId()

	sc.step("a goblin moves between two squares nobody sees", place(g1, 18, 6))
	sc.step("a goblin moves where the players see it", place(g2, 21, 7))
	sc.step("a goblin the master keeps hidden moves in plain view", place(hg, 21, 8))
	sc.step("a player's character moves", place(c.toren.GetId(), 7, 7))
	sc.step("a player's character moves where nobody else is", place(c.salvia.GetId(), 3, 9))
	sc.step("a visible goblin is hidden", hide(g2, true))
	sc.step("a hidden goblin is shown in plain view", hide(g2, false))
	sc.step("a hidden goblin is shown in the dark", hide(g1, false))
	sc.step("a goblin in the dark is hidden again", hide(g1, true))
	sc.step("a goblin in plain view is removed", remove(g2))
	sc.step("a hidden goblin is removed", remove(hg))
	sc.step("a goblin is placed again where the players see it", place(g2, 20, 7))
	sc.step("a player's character is taken off the map", remove(c.brisa.GetId()))
	sc.step("a player's character is placed again", place(c.brisa.GetId(), 4, 7))
	sc.step("a hidden goblin picks up a torch", torch(g1, "light:torch"))
	sc.step("a hidden goblin with a torch moves", place(g1, 17, 7))
	sc.step("a visible goblin picks up a torch", torch(cpt, "light:torch"))
	sc.step("a visible goblin with a torch moves in the dark", place(cpt, 21, 4))
	sc.step("a player's character picks up a torch", torch(c.pens.GetId(), "light:torch"))
	sc.step("a goblin puts the torch out", torch(g1, ""))

	// A combat runs on the map: the NPC tokens are out of a player's reads.
	sc.step("a combat starts", func() {
		if _, err := m.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
			CampaignId: c.campaign, IdempotencyKey: newKey(), Name: "A guarita",
			Participants: []*playv1.Participant{{CharacterId: c.toren.GetId()}, {CharacterId: g2}},
		})); err != nil {
			t.Fatalf("StartEncounter() error = %v", err)
		}
	})
	sc.step("a goblin moves during the combat", place(cpt, 20, 4))
	sc.step("a player's character moves during the combat", place(c.toren.GetId(), 8, 7))
	sc.golden("fog_streams_cave.golden.txt")

	// What each player is left with: the view now, the memory included, and the tokens.
	views := map[string]any{}
	read := func(label string, u *user, as ...string) {
		res := u.mustVision(c.campaign, c.mapID, as...)
		raw, err := protojson.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		// The revision and the tiles' path hash and name the map, which is new on every run.
		if tree, ok := v.(map[string]any); ok {
			delete(tree, "revision")
			delete(tree, "tilesPath")
			delete(tree, "tiles")
		}
		views[label] = v
		get := u.mustGetMapAs(c.campaign, c.mapID, firstOf(as))
		var names []string
		for _, tok := range get.GetTokens() {
			names = append(names, sc.labels[tok.GetCharacterId()])
		}
		views[label+"_tokens"] = names
	}
	read("master_as_pens", m, c.pens.GetId())
	read("master_as_toren", m, c.toren.GetId())
	read("ana", c.ana)
	read("caio", c.caio)
	read("bia", c.bia)
	read("dani", c.dani)
	raw, err := json.MarshalIndent(views, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "fog_views_cave.golden.json", append(fogScrub(raw), '\n'))
}

func firstOf(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

var fogTimePattern = regexp.MustCompile(`"\d{4}-\d\d-\d\dT[0-9:.]+Z"`)

// fogScrub drops the times, which change with every run.
func fogScrub(raw []byte) []byte { return fogTimePattern.ReplaceAll(raw, []byte(`"<time>"`)) }

// TestFogStreams_WhoIsToldOfEachTokenWriteWithoutFog: the same writes on a map without fog,
// where the existing rules say who hears (a token hidden from the players stays out of
// their stream), and on a map the players do not see.
func TestFogStreams_WhoIsToldOfEachTokenWriteWithoutFog(t *testing.T) {
	t.Parallel()
	s := newScenes(t, true)
	m := s.master
	npc := m.createCharacter(s.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	probe := m.createMap(s.campaign, "Sonda", m.newImage(s.campaign)).GetId()
	m.setMapRevealed(s.campaign, probe, true)
	sc := newFogStreamScript(t, m, s.campaign, s.mapID, probe, []string{"ana", "caio"}, []*user{s.ana, s.caio})
	sc.name(s.pens.GetId(), "pens")
	sc.name(s.other.GetId(), "toren")
	sc.name(npc.GetId(), "goblin")
	place := func(ch string, x, y int32) func() {
		return func() { m.placeToken(s.campaign, s.mapID, ch, x, y) }
	}
	hide := func(hidden bool) func() { return func() { m.setTokenHidden(s.campaign, s.mapID, npc.GetId(), hidden) } }
	remove := func(ch string) func() { return func() { m.removeToken(s.campaign, s.mapID, ch) } }
	pens, toren, g := s.pens.GetId(), s.other.GetId(), npc.GetId()

	sc.step("a player's character is placed", place(pens, 3000, 3000))
	sc.step("a player's character moves", place(pens, 3500, 3000))
	sc.step("a goblin is placed (hidden)", place(g, 5000, 5000))
	sc.step("a hidden goblin moves", place(g, 5500, 5000))
	sc.step("a hidden goblin is shown", hide(false))
	sc.step("a visible goblin moves", place(g, 6000, 5000))
	sc.step("a visible goblin is hidden", hide(true))
	sc.step("a hidden goblin is removed", remove(g))
	sc.step("a goblin is placed and shown", func() {
		place(g, 5000, 5000)()
		hide(false)()
	})
	sc.step("a visible goblin picks up a torch", func() { m.carry(s.campaign, s.mapID, g, "light:torch") })
	sc.step("a visible goblin is removed", remove(g))
	sc.step("a second player's character is placed", place(toren, 8000, 8000))
	sc.step("a player's character is removed", remove(pens))
	sc.step("the map is hidden from the players", func() { m.setMapRevealed(s.campaign, s.mapID, false) })
	sc.step("a token moves on a map the players do not see", place(toren, 8500, 8000))
	sc.step("a goblin is placed on a map the players do not see", func() { place(g, 1000, 1000)(); hide(false)() })
	sc.golden("fog_streams_mirathel.golden.txt")
}
