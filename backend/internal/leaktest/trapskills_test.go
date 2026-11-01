package leaktest

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The player's trap search is the same for every trap: Perception, Investigation and
// "Outra perícia…" with all the other skills. The server counts a skill only for the
// traps that allow it, so a search made with a skill nothing nearby allows answers exactly
// like a search that fell short, and an empty place answers the same. Otherwise the
// answer would say there is a trap.

// srdSkills are the 18 skills of the SRD, as the searches name them.
var srdSkills = []string{
	"acrobatics", "animal-handling", "arcana", "athletics", "deception", "history", "insight", "intimidation", "investigation",
	"medicine", "nature", "perception", "performance", "persuasion", "religion", "sleight-of-hand", "stealth", "survival",
}

const (
	arcanaOnlyDC = 20 // the DC to find of the trap only Arcana (besides Investigation) finds
	lowFace      = 5  // far below every DC: no skill finds anything with it
	highFace     = 20
)

// searchRequest is the player's search with one skill and the face of a real die (and a
// second one, which a Perception search in dim light asks for and nothing else reads).
func (w *world) searchRequest(skill string, face int32) *playv1.SearchForTrapsRequest {
	req := &playv1.SearchForTrapsRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), Roll: &playv1.SearchForTrapsRequest_D20Face{D20Face: face}, D20Face_2: proto.Int32(face),
	}
	switch skill {
	case "perception":
		req.Skill = playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_PERCEPTION
	case "investigation":
		req.Skill = playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_INVESTIGATION
	default:
		req.Skill, req.OtherSkillKey = playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_OTHER, "skill:"+skill
	}
	return req
}

// searchWorld is a world whose combat is over (a search costs no action then), with a
// trap only Arcana finds next to Caio's character, or, without it, Caio's character in
// a place with nothing within reach. Whatever a player may read of the trap is nothing:
// its name and description are markers of the master.
func searchWorld(t *testing.T, withTrap bool) *world {
	t.Helper()
	w := newWorld(t)
	if withTrap {
		w.pts["trap-arcana"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, w.secrets.marker("trap-arcana-name"), w.secrets.marker("trap-arcana-description"), 9, 13, func(r *mapsv1.CreateMapPointRequest) {
			r.Trap = &mapsv1.TrapSpec{
				FindDc: arcanaOnlyDC, AreaSize: 1, Trigger: rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER, Effect: &rulesv1.TrapEffect{},
				AlsoFindSkillKeys: []string{"skill:arcana"},
			}
		})
		w.secrets.id("trap", w.pts["trap-arcana"].GetId())
		w.secrets.add(&canary{needle: "skill:arcana", kind: "trap-skill-list"})
	} else {
		w.place(w.fogMap, w.toren.GetId(), 15, 9)
	}
	must(w.master.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	return w
}

// shapeOf is what a search answer says apart from the numbers of the dice (which differ
// with the character's bonus in each skill and are the player's own): the traps it found
// and whether there is a second roll.
func shapeOf(r reply) string {
	res := searchAnswer(r)
	second := res.GetSecondRoll() != nil
	res.Roll, res.SecondRoll = nil, nil
	if second {
		return protojson.Format(res) + " + a second roll"
	}
	return protojson.Format(res)
}

// searchAnswer is the search reply as the typed message.
func searchAnswer(r reply) *playv1.SearchForTrapsResponse {
	res := &playv1.SearchForTrapsResponse{}
	if err := protojson.Unmarshal(r.body, res); err != nil {
		panic(err)
	}
	return res
}

type searchRun struct {
	w       *world
	replies map[string]reply
	stream  map[string]int
}

// searchWithEverySkill makes Caio search with each of the 18 skills and reads the stream
// he was listening to until the session ended.
func searchWithEverySkill(t *testing.T, withTrap bool) searchRun {
	t.Helper()
	w := searchWorld(t, withTrap)
	// The setup's map edits and table-content write have hints still waiting in the gates (up to a
	// second): let them go before the stream opens, or they land on one side of it in one run only.
	w.hints.Settle(t)
	watcher := w.watchStream(w.caio)
	run := searchRun{w: w, replies: map[string]reply{}, stream: map[string]int{}}
	for _, skill := range srdSkills {
		r := w.caio.call(playv1connect.PlayServiceSearchForTrapsProcedure, w.searchRequest(skill, lowFace))
		if !r.ok() {
			t.Fatalf("a search with %s: status %d %s", skill, r.status, r.body)
		}
		run.replies[skill] = r
	}
	must(w.master.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	select {
	case <-watcher.done:
	case <-time.After(30 * time.Second):
		t.Fatal("Caio's stream did not end with the session")
	}
	for _, ev := range watcher.events {
		switch eventCase(ev) {
		case "heartbeat":
			continue // a timer, not a hint about the searches
		case "content_changed":
			// The fixture's own table content writes: the hint is throttled (one every
			// 250 ms, the rest merged and sent when the interval ends), so under load it
			// can land after the stream opens in one run and before it in the other. A
			// search never changes the table's content, so this hint says nothing about it.
			continue
		}
		run.stream[eventCase(ev)]++
	}
	return run
}

// TestASearchWithAnySkillReadsTheSameWhateverTheTrapsNearby: Caio searches next to a trap
// that only Arcana (besides Investigation) finds, with each of the 18 skills and a roll
// no skill passes with, and again in a place with no trap at all. Every answer has the
// same shape, nothing holds a marker of the trap, and the stream hears the same hints.
func TestASearchWithAnySkillReadsTheSameWhateverTheTrapsNearby(t *testing.T) {
	trapped, empty := searchWithEverySkill(t, true), searchWithEverySkill(t, false)
	for _, skill := range srdSkills {
		a, b := trapped.replies[skill], empty.replies[skill]
		if shapeOf(a) != shapeOf(b) {
			t.Errorf("%s: next to the trap the answer is %s, in an empty place %s; want the same", skill, shapeOf(a), shapeOf(b))
		}
		for _, f := range trapped.w.inspect(trapped.w.caio, a, nil) {
			t.Errorf("%s: %s", skill, f)
		}
	}
	// Every skill but Perception answers with one roll: the same shape for all of them.
	for _, skill := range srdSkills {
		if skill == "perception" {
			continue
		}
		if shapeOf(trapped.replies[skill]) != shapeOf(trapped.replies["arcana"]) {
			t.Errorf("%s answers %s, Arcana %s; want the same shape", skill, shapeOf(trapped.replies[skill]), shapeOf(trapped.replies["arcana"]))
		}
	}
	for kind, n := range trapped.stream {
		if empty.stream[kind] != n {
			t.Errorf("Caio's stream next to the trap got %d %q events, in an empty place %d; want the same", n, kind, empty.stream[kind])
		}
	}
	for kind, n := range empty.stream {
		if trapped.stream[kind] != n {
			t.Errorf("Caio's stream in an empty place got %d %q events, next to the trap %d; want the same", n, kind, trapped.stream[kind])
		}
	}
}

// TestOnlyTheAllowedSkillFindsTheTrap is the positive control of the test above: with a
// roll that clears the DC, Arcana finds the trap and tells which one, Perception (the trap
// has no DC to notice it) finds nothing, and an Acrobatics roll as high finds nothing.
func TestOnlyTheAllowedSkillFindsTheTrap(t *testing.T) {
	w := searchWorld(t, true)
	id := w.pts["trap-arcana"].GetId()
	for _, skill := range []string{"acrobatics", "perception"} {
		r := w.caio.call(playv1connect.PlayServiceSearchForTrapsProcedure, w.searchRequest(skill, highFace))
		if !r.ok() || len(searchAnswer(r).GetFoundPointIds()) != 0 {
			t.Errorf("%s with a roll of %d found %s (status %d), want nothing", skill, highFace, r.body, r.status)
		}
	}
	r := w.caio.call(playv1connect.PlayServiceSearchForTrapsProcedure, w.searchRequest("arcana", highFace))
	if found := searchAnswer(r).GetFoundPointIds(); !r.ok() || len(found) != 1 || found[0] != id {
		t.Fatalf("Arcana with a roll of %d found %s (status %d), want the trap", highFace, r.body, r.status)
	}
	// What the character found it may read; the checks below hold the rest back.
	for _, c := range w.secrets.list {
		if c.needle == id {
			c.readers = names([]*person{w.caio})
		}
	}
	for _, f := range w.inspect(w.caio, r, nil) {
		t.Errorf("the answer that found the trap: %s", f)
	}
}

// oneDieSearch is Caio's Perception search with a single real die from where his character
// stands in the dim light of the fog map (a human: dim light is lightly obscured to him,
// so the search needs a second die). With a trap, it sits next to him; without, nothing
// is within reach. The place and the light are the same.
func oneDieSearch(t *testing.T, withTrap bool) reply {
	t.Helper()
	w := newWorld(t)
	if withTrap {
		w.pts["trap-dim"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, w.secrets.marker("trap-dim-name"), w.secrets.marker("trap-dim-description"), 9, 13, func(r *mapsv1.CreateMapPointRequest) {
			r.Trap = &mapsv1.TrapSpec{NoticeDc: 12, FindDc: 15, AreaSize: 1, Trigger: rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER, Effect: &rulesv1.TrapEffect{}}
		})
	}
	must(w.master.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	req := w.searchRequest("perception", lowFace)
	req.D20Face_2 = nil
	return w.caio.call(playv1connect.PlayServiceSearchForTrapsProcedure, req)
}

// TestADimLightSearchOnOneDieAnswersTheSameWhateverTheTrapsOnTheMap: the refusal that asks
// for a second die depends on the light around the searcher, never on the map's traps, so
// a map with a trap and a map with none answer the same status and the same words.
func TestADimLightSearchOnOneDieAnswersTheSameWhateverTheTrapsOnTheMap(t *testing.T) {
	trapped, empty := oneDieSearch(t, true), oneDieSearch(t, false)
	if trapped.ok() {
		t.Fatalf("premise: a Perception search on one die in dim light was answered %d %s, want the refusal that asks for a second die", trapped.status, trapped.body)
	}
	if trapped.status != empty.status || !bytes.Equal(trapped.body, empty.body) {
		t.Errorf("with a trap on the map the answer is %d %s, with none %d %s; want the same", trapped.status, trapped.body, empty.status, empty.body)
	}
}
