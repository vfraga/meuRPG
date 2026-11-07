package leaktest

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// MR-015, MR-028, RN-10: a scene open to only some players, and an image shown to some players only.
// Ana is in, Caio is out (the scene's players are Ana alone). Every thing that belongs to the scene
// carries a needle of its own that Ana reads (the positive control, with the master) and Caio, the
// pending member, the stranger and the signed-out caller never do: in a read, on the stream, on the
// image route. The images have one needle each (the gallery name, which only a viewer reads, and the id).

const (
	kindWhole  = playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_WHOLE_TABLE
	kindScenes = playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_SCENE_PLAYERS
	kindPicked = playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_CHOSEN_PLAYERS
)

// partialScene is a scene point with its own needles, all of them Ana's to read, and an NPC on its stage.
type partialScene struct {
	point   *mapsv1.MapPoint
	action  string
	npc     string // the NPC to put on its stage once the scene is open (opening a scene empties the stage)
	needles []string
}

// buildPartialScene prepares a scene with a name, a description, an action and an NPC to put on its stage,
// each with a marker nobody else has, and the ids of the point and of the action.
func (w *world) buildPartialScene() partialScene {
	w.t.Helper()
	ps := partialScene{}
	name := w.secrets.marker("partial-scene-name", w.ana)
	description := w.secrets.marker("partial-scene-description", w.ana)
	ps.point = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, name, description, 12, 12, nil)
	w.secrets.id("partial-scene-point", ps.point.GetId(), w.ana)
	action := w.secrets.marker("partial-scene-action", w.ana)
	res := must(w.master.maps.AddSceneAction(w.t.Context(), rq(&mapsv1.AddSceneActionRequest{
		CampaignId: w.campaign, MapId: ps.point.GetMapId(), PointId: ps.point.GetId(), Key: "skill:history", Name: action, Dc: 11, MaxAttempts: new(int32(2)),
	})))
	ps.action = res.GetAction().GetId()
	w.secrets.id("partial-action", ps.action, w.ana)
	portrait := w.image("partial-portrait", 60, 40, w.ana)
	w.secrets.id("partial-portrait-id", portrait, w.ana)
	npc, _ := w.npc("partial-stage", &charactersv1.BasicSheet{HitPointsMax: 9, ArmorClass: 11, SpeedFt: 30, PortraitImageId: portrait})
	w.allow("partial-stage-name", w.ana)
	w.allow("partial-stage-description")
	w.secrets.id("partial-stage-npc", npc.GetId())
	ps.npc = npc.GetId()
	ps.needles = append(w.needlesOf("partial-scene-name", "partial-scene-description", "partial-scene-action", "partial-stage-name"),
		ps.point.GetId(), ps.action, portrait)
	return ps
}

func (w *world) openSceneTo(point string, who ...*person) {
	w.t.Helper()
	aud := &playv1.SceneAudience{}
	for _, p := range who {
		aud.UserIds = append(aud.UserIds, p.id)
	}
	must(w.master.play.OpenScene(w.t.Context(), rq(&playv1.OpenSceneRequest{CampaignId: w.campaign, PointId: point, Audience: aud})))
}

func (w *world) setSceneTo(who ...*person) {
	w.t.Helper()
	aud := &playv1.SceneAudience{}
	for _, p := range who {
		aud.UserIds = append(aud.UserIds, p.id)
	}
	must(w.master.play.SetSceneAudience(w.t.Context(), rq(&playv1.SetSceneAudienceRequest{CampaignId: w.campaign, Audience: aud})))
}

// settle has the master show the map the table already shows: every stream gets a marker, in order, so when
// all of ws have it, whatever the steps before sent them has arrived.
//
// The hint gates (content_changed, map_changed, puzzle_changed) send their last change on a timer, up to a
// second after the write that made it, and that send goes to everyone it concerns, in no order with the
// marker. The world's setup ends with such writes: left alone, the send lands in a later step and passes for
// what that step sent. So the gates' scheduled sends go out first (the stack's tracker, not a sleep): then
// they are before the marker, and every step's own events are what comes after it.
func (w *world) settle(ws ...*streamWatcher) {
	w.t.Helper()
	w.hints.Settle(w.t)
	count := func(sw *streamWatcher) int {
		n := 0
		for _, ev := range sw.since(0) {
			if ev.GetCurrentMapChanged() != nil {
				n++
			}
		}
		return n
	}
	before := make([]int, len(ws))
	for i, sw := range ws {
		before[i] = count(sw)
	}
	must(w.master.play.SetCurrentMap(w.t.Context(), rq(&playv1.SetCurrentMapRequest{CampaignId: w.campaign, MapId: w.fogMap})))
	for i, sw := range ws {
		for count(sw) == before[i] {
			select {
			case <-sw.more:
			case <-time.After(20 * time.Second):
				w.t.Fatalf("%s's stream never got the marker", sw.p.name)
			}
		}
	}
}

// eventsAfter are the events of the stream after the first n, as JSON, one string per event.
func eventsAfter(sw *streamWatcher, n int) []string {
	var out []string
	for _, ev := range sw.since(n) {
		if ev.GetCurrentMapChanged() != nil || ev.GetHeartbeat() != nil {
			continue // the marker, and the heartbeat of an idle stream
		}
		body, err := protojson.Marshal(ev)
		if err != nil {
			panic(err)
		}
		out = append(out, string(body))
	}
	return out
}

func holdsNeedle(text string, needles []string) (string, bool) {
	for _, n := range needles {
		if strings.Contains(text, n) {
			return n, true
		}
	}
	return "", false
}

func TestRN10_APlayerOutsideAnOpenSceneReadsNoScene(t *testing.T) {
	w := newWorld(t)
	ps := w.buildPartialScene()
	master, ana, caio := w.master, w.ana, w.caio
	sm, sa, sc := w.watchStream(master), w.watchStream(ana), w.watchStream(caio)
	w.openSceneTo(ps.point.GetId(), ana)
	must(master.play.PutOnStage(t.Context(), rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: ps.npc})))
	w.settle(sm, sa, sc)

	// The reads: the whole table of reads, asked of everyone, finds none of the scene's needles in Caio's,
	// the pending member's, the stranger's or the signed-out caller's answers (the needles are Ana's alone).
	got := newAnswers()
	for _, r := range reads {
		w.runRead(t, r, got)
	}
	// The positive controls: the master and Ana read every needle of the scene in the open scene.
	for _, p := range []*person{master, ana} {
		r := w.openSceneAs(p)
		for _, n := range ps.needles {
			if !bytes.Contains(r.body, []byte(n)) {
				t.Errorf("%s's open scene lacks %q, so the check proves nothing: %s", p.name, n, shorten(r.body))
			}
		}
	}
	for _, p := range append(w.players()[1:], w.anonymous()) { // Caio, the pending member, the stranger, the signed-out caller
		if n, found := holdsAny(w.openSceneAs(p).body, ps.needles); found {
			t.Errorf("%s reads the scene (%s) though they are not in it", p.name, n)
		}
	}
	// Caio reads exactly what he reads when no scene is open.
	outsider := w.openSceneAs(caio)
	if !outsider.ok() || strings.Contains(string(outsider.body), "pointId") {
		t.Errorf("Caio's open scene = HTTP %d %s, want an answer with no scene", outsider.status, outsider.body)
	}

	// Rolling in it is refused as when none is open: the refusal of the same call once the scene is closed.
	roll := func(p *person) reply {
		return p.call(playv1connect.PlayServiceRollSceneCheckProcedure, &playv1.RollSceneCheckRequest{
			CampaignId: w.campaign, ActionId: ps.action, IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_D20Face{D20Face: 12},
		})
	}
	refusedOut := roll(caio)
	if refusedOut.ok() {
		t.Fatalf("Caio rolled an action of a scene he is not in: %s", refusedOut.body)
	}
	if r := roll(ana); !r.ok() {
		t.Errorf("Ana (in the scene) was refused a roll: HTTP %d %s", r.status, r.body)
	}
	must(master.play.CloseScene(t.Context(), rq(&playv1.CloseSceneRequest{CampaignId: w.campaign})))
	if closed := roll(caio); closed.status != refusedOut.status || !bytes.Equal(closed.body, refusedOut.body) {
		t.Errorf("a roll by someone out of the scene = HTTP %d %s, with no scene open = HTTP %d %s; want the same refusal", refusedOut.status, refusedOut.body, closed.status, closed.body)
	}
	w.openSceneTo(ps.point.GetId(), ana)
	must(master.play.PutOnStage(t.Context(), rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: ps.npc})))
	w.settle(sm, sa, sc)

	// The portrait of the NPC on the stage is Ana's to fetch, and 404 for the players out of the scene.
	portrait := ps.needles[len(ps.needles)-1]
	if status, _ := master.get("/images/" + portrait); status != 200 {
		t.Errorf("the master gets HTTP %d for the portrait on the scene's stage, want 200", status)
	}
	if status, _ := ana.get("/images/" + portrait); status != 200 {
		t.Errorf("Ana gets HTTP %d for the portrait on the scene's stage, want 200", status)
	}
	for _, p := range append(w.players()[1:], w.anonymous()) {
		if status, _ := p.get("/images/" + portrait); status == 200 {
			t.Errorf("%s gets the portrait of an NPC on the stage of a scene they are not in", p.name)
		}
	}
	// The group check asked in it goes to Ana's character alone (the table's own is closed first).
	if open := must(master.contests.GetGroupCheck(t.Context(), rq(&playv1.GetGroupCheckRequest{CampaignId: w.campaign}))).GetGroupCheck(); open.GetOpen() {
		must(master.contests.CloseGroupCheck(t.Context(), rq(&playv1.CloseGroupCheckRequest{CampaignId: w.campaign, IdempotencyKey: newKey(), GroupCheckId: open.GetId()})))
	}
	check := must(master.contests.RequestGroupCheck(t.Context(), rq(&playv1.RequestGroupCheckRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), SkillKey: "skill:stealth", Dc: 17, ShowDc: true,
	}))).GetGroupCheck()
	if check.GetAskedCount() != 1 {
		t.Errorf("the group check asked %d characters, want Ana's alone", check.GetAskedCount())
	}
	if r := caio.call(playv1connect.ContestServiceGetGroupCheckProcedure, &playv1.GetGroupCheckRequest{CampaignId: w.campaign}); strings.Contains(string(r.body), check.GetId()) {
		t.Errorf("Caio reads the group check asked in a scene he is not in: %s", r.body)
	}

	// Caio goes in, then out again: he reads the scene while he is in, and not a trace of it after.
	mark := sc.count()
	w.setSceneTo(ana, caio)
	if _, found := holdsAny(w.openSceneAs(caio).body, ps.needles); !found {
		t.Error("Caio, brought into the scene, reads nothing of it: the positive control of the change is missing")
	}
	w.setSceneTo(ana)
	w.settle(sm, sa, sc)
	if n, found := holdsAny(w.openSceneAs(caio).body, ps.needles); found {
		t.Errorf("Caio still reads the scene (%s) after the master took him out", n)
	}

	// The stream: while he is out, Caio hears of nothing the scene does but being taken out; Ana hears each change.
	mark2 := sc.count()
	markA := sa.count()
	must(master.maps.UpdateSceneAction(t.Context(), rq(&mapsv1.UpdateSceneActionRequest{CampaignId: w.campaign, MapId: ps.point.GetMapId(), PointId: ps.point.GetId(), ActionId: ps.action, Dc: new(int32(14))})))
	other, _ := w.npc("partial-stage-2", &charactersv1.BasicSheet{HitPointsMax: 9, ArmorClass: 11, SpeedFt: 30})
	must(master.play.PutOnStage(t.Context(), rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: other.GetId()})))
	w.settle(sm, sa, sc)
	if got := eventsAfter(sc, mark2); len(got) != 0 {
		t.Errorf("Caio's stream got %d events about a scene he is not in: %v", len(got), got)
	}
	var sceneHints int
	for _, ev := range eventsAfter(sa, markA) {
		if strings.Contains(ev, "sceneChanged") || strings.Contains(ev, "stageChanged") {
			sceneHints++
		}
	}
	if sceneHints < 2 {
		t.Errorf("Ana's stream got %d scene hints for the two changes, want both", sceneHints)
	}
	// What Caio was sent when he was brought in and taken out is a hint with nothing in it.
	for _, ev := range eventsAfter(sc, mark) {
		if n, found := holdsNeedle(ev, ps.needles); found {
			t.Errorf("Caio's stream carries %q: %s", n, ev)
		}
	}
}

// shownImage is a gallery image with a needle, the gallery name only a viewer reads, and its id.
type shownImage struct {
	id      string
	needles []string
}

func (w *world) newShown(kind string, viewers ...*person) shownImage {
	w.t.Helper()
	id := w.image(kind, 70, 50, viewers...)
	w.secrets.id(kind, id, viewers...)
	return shownImage{id: id, needles: append(w.needlesOf(kind+"-gallery-name"), id)}
}

func (w *world) showImage(im shownImage, keep bool, aud *playv1.ImageAudience) {
	w.t.Helper()
	must(w.master.play.SetShownImage(w.t.Context(), rq(&playv1.SetShownImageRequest{CampaignId: w.campaign, ImageId: im.id, Keep: keep, Audience: aud})))
}

func (w *world) setImageTo(aud *playv1.ImageAudience) {
	w.t.Helper()
	must(w.master.play.SetShownImageAudience(w.t.Context(), rq(&playv1.SetShownImageAudienceRequest{CampaignId: w.campaign, Audience: aud})))
}

func chosen(who ...*person) *playv1.ImageAudience {
	a := &playv1.ImageAudience{Kind: kindPicked}
	for _, p := range who {
		a.UserIds = append(a.UserIds, p.id)
	}
	return a
}

// seen is what a person gets of the shown image: in the live session, and the image route.
func (w *world) seen(p *person, im shownImage) (read bool, route int) {
	r := p.call(playv1connect.PlayServiceGetLiveSessionProcedure, &playv1.GetLiveSessionRequest{CampaignId: w.campaign})
	_, read = holdsAny(r.body, im.needles)
	route, _ = p.get("/images/" + im.id)
	return read, route
}

// wantImage checks the image for Ana and Caio: read in the live session, image route, and the stream since
// the marks. The pending member, the stranger and the signed-out caller never get it.
//
// fresh says, for Ana and for Caio, that the step sent the image to a viewer: their stream must carry it.
func (w *world) wantImage(t *testing.T, step string, im shownImage, sa, sc *streamWatcher, markA, markC int, ana, caio bool, fresh ...bool) {
	t.Helper()
	for i, c := range []struct {
		p    *person
		sw   *streamWatcher
		mark int
		want bool
	}{{w.ana, sa, markA, ana}, {w.caio, sc, markC, caio}} {
		read, route := w.seen(c.p, im)
		if read != c.want {
			t.Errorf("%s: %s reads the shown image in the live session = %v, want %v", step, c.p.name, read, c.want)
		}
		if (route == 200) != c.want {
			t.Errorf("%s: %s gets HTTP %d for the image, want it %v", step, c.p.name, route, c.want)
		}
		sentTheImage := false
		for _, ev := range eventsAfter(c.sw, c.mark) {
			if _, found := holdsNeedle(ev, im.needles); found {
				sentTheImage = true
			}
		}
		if sentTheImage && !c.want {
			t.Errorf("%s: %s's stream carried the image", step, c.p.name)
		}
		if wantSent := i < len(fresh) && fresh[i]; wantSent && !sentTheImage {
			t.Errorf("%s: %s's stream did not carry the image: the positive control of the stream is missing", step, c.p.name)
		}
	}
	if status, _ := w.master.get("/images/" + im.id); status != 200 {
		t.Errorf("%s: the master gets HTTP %d for the image, want 200", step, status)
	}
	for _, p := range []*person{w.pending, w.stranger, w.anonymous()} {
		if read, route := w.seen(p, im); read || route == 200 {
			t.Errorf("%s: %s reads the image (%v) or gets it (HTTP %d)", step, p.name, read, route)
		}
	}
}

func TestRN10_AShownImageReachesOnlyItsAudience(t *testing.T) {
	w := newWorld(t)
	ps := w.buildPartialScene()
	sm, sa, sc := w.watchStream(w.master), w.watchStream(w.ana), w.watchStream(w.caio)
	w.openSceneTo(ps.point.GetId(), w.ana) // Ana is in the scene, Caio is out
	w.settle(sm, sa, sc)

	// The scene's players (the default while the scene is open to some): Ana and the master, not Caio.
	scene := w.newShown("shown-scene", w.ana)
	markA, markC := sa.count(), sc.count()
	w.showImage(scene, false, nil)
	w.settle(sm, sa, sc)
	w.wantImage(t, "the scene's players", scene, sa, sc, markA, markC, true, false, true)
	if !bytes.Contains(w.master.call(playv1connect.PlayServiceGetLiveSessionProcedure, &playv1.GetLiveSessionRequest{CampaignId: w.campaign}).body, []byte(scene.id)) {
		t.Error("the master does not read the image shown to the scene's players")
	}

	// Chosen players: Caio alone, though he is out of the scene.
	picked := w.newShown("shown-chosen", w.caio)
	markA, markC = sa.count(), sc.count()
	w.showImage(picked, false, chosen(w.caio))
	w.settle(sm, sa, sc)
	w.wantImage(t, "chosen players", picked, sa, sc, markA, markC, false, true, false, true)

	// The whole table: everyone, though the scene is open to Ana alone (the positive control for the table).
	table := w.newShown("shown-table", w.ana, w.caio)
	markA, markC = sa.count(), sc.count()
	w.showImage(table, false, &playv1.ImageAudience{Kind: kindWhole})
	w.settle(sm, sa, sc)
	w.wantImage(t, "the whole table", table, sa, sc, markA, markC, true, true, true, true)

	// Narrowing takes the image away from Caio at once: the read, the route, the stream.
	markA, markC = sa.count(), sc.count()
	w.setImageTo(chosen(w.ana))
	w.settle(sm, sa, sc)
	w.wantImage(t, "narrowed to Ana", table, sa, sc, markA, markC, true, false)
	if got := eventsAfter(sa, markA); len(got) != 0 {
		t.Errorf("Ana, who stays, got %v when the audience narrowed", got)
	}
	// Caio is told the update he gets when an image stops being shown: an event with no image in it.
	told := false
	for _, ev := range sc.since(markC) {
		if ev.GetShownImageChanged() != nil && ev.GetShownImageChanged().GetImage() == nil {
			told = true
		}
	}
	if !told {
		t.Error("Caio, taken out of the audience, was not sent the update that the image stopped being shown")
	}

	// Widening brings him back, with the image, at once.
	markA, markC = sa.count(), sc.count()
	w.setImageTo(&playv1.ImageAudience{Kind: kindWhole})
	w.settle(sm, sa, sc)
	w.wantImage(t, "widened to the whole table", table, sa, sc, markA, markC, true, true, false, true)
	gotImage := false
	for _, ev := range sc.since(markC) {
		gotImage = gotImage || ev.GetShownImageChanged().GetImage().GetId() == table.id
	}
	if !gotImage {
		t.Error("Caio, brought into the audience, was not sent the image")
	}

	// The scene closes: an image shown to its players stays with them, and goes to nobody else.
	snap := w.newShown("shown-snapshot", w.ana)
	w.showImage(snap, false, nil)
	markA, markC = sa.count(), sc.count()
	must(w.master.play.CloseScene(t.Context(), rq(&playv1.CloseSceneRequest{CampaignId: w.campaign})))
	w.settle(sm, sa, sc)
	w.wantImage(t, "after the scene closed", snap, sa, sc, markA, markC, true, false)
	w.openSceneTo(ps.point.GetId(), w.caio) // another opening, for Caio: the image keeps its players
	w.settle(sm, sa, sc)
	w.wantImage(t, "after another opening", snap, sa, sc, markA, markC, true, false)
}

func TestRN10_AnImageLeftWithThePlayersKeepsItsAudience(t *testing.T) {
	w := newWorld(t)
	ps := w.buildPartialScene()
	sm, sa, sc := w.watchStream(w.master), w.watchStream(w.ana), w.watchStream(w.caio)
	w.openSceneTo(ps.point.GetId(), w.ana)
	w.settle(sm, sa, sc)

	left := w.newShown("left-scene", w.ana)
	w.showImage(left, true, nil) // kept, for the scene's players
	replacement := w.newShown("left-replacement", w.ana, w.caio)
	markA, markC := sa.count(), sc.count()
	w.showImage(replacement, false, &playv1.ImageAudience{Kind: kindWhole})
	w.settle(sm, sa, sc)

	listed := func(p *person) reply {
		return p.call(playv1connect.PlayServiceListLeftImagesProcedure, &playv1.ListLeftImagesRequest{CampaignId: w.campaign})
	}
	// The positive controls: the master and Ana list it and fetch it.
	for _, p := range []*person{w.master, w.ana} {
		if _, found := holdsAny(listed(p).body, left.needles); !found {
			t.Errorf("%s does not list the image left with the scene's players", p.name)
		}
		if status, _ := p.get("/images/" + left.id); status != 200 {
			t.Errorf("%s gets HTTP %d for the image left with the scene's players, want 200", p.name, status)
		}
	}
	// Caio, out of the scene, never does: not the list, not the route, not the stream.
	for _, p := range append(w.players()[1:], w.anonymous()) {
		if n, found := holdsAny(listed(p).body, left.needles); found {
			t.Errorf("%s lists the image left with the scene's players (%s)", p.name, n)
		}
		if status, _ := p.get("/images/" + left.id); status == 200 {
			t.Errorf("%s fetches the image left with the scene's players", p.name)
		}
	}
	for _, ev := range eventsAfter(sc, markC) {
		if strings.Contains(ev, "leftImagesChanged") {
			t.Errorf("Caio's stream carries the change of an image left with players he is not among: %s", ev)
		}
		if n, found := holdsNeedle(ev, left.needles); found {
			t.Errorf("Caio's stream carries %q", n)
		}
	}
	heard := false
	for _, ev := range eventsAfter(sa, markA) {
		heard = heard || strings.Contains(ev, "leftImagesChanged")
	}
	if !heard {
		t.Error("Ana's stream got no left_images_changed: the positive control of the stream is missing")
	}

	// Taken back, it is nobody's.
	must(w.master.play.TakeBackLeftImage(t.Context(), rq(&playv1.TakeBackLeftImageRequest{CampaignId: w.campaign, ImageId: left.id})))
	if status, _ := w.ana.get("/images/" + left.id); status == 200 {
		t.Error("Ana still fetches the image the master took back")
	}

	// Left with the whole table (shown to it, kept): Caio gets it too.
	table := w.newShown("left-table", w.ana, w.caio)
	w.showImage(table, true, &playv1.ImageAudience{Kind: kindWhole})
	w.showImage(replacement, false, &playv1.ImageAudience{Kind: kindWhole})
	if _, found := holdsAny(listed(w.caio).body, table.needles); !found {
		t.Error("Caio does not list the image left with the whole table")
	}
	if status, _ := w.caio.get("/images/" + table.id); status != 200 {
		t.Errorf("Caio gets HTTP %d for the image left with the whole table, want 200", status)
	}
}

// MR-015, RN-10: with "Os jogadores podem entrar" on, a player out of the scene reads its name and that they may join,
// and no other needle of it (its point id, description, action, stage, portrait); after JoinScene they read the stage.
func TestRN10_APlayerOutOfAJoinableSceneReadsOnlyItsName(t *testing.T) {
	w := newWorld(t)
	ps := w.buildPartialScene()
	master, ana, caio := w.master, w.ana, w.caio
	sm, sa, sc := w.watchStream(master), w.watchStream(ana), w.watchStream(caio)
	w.openSceneTo(ps.point.GetId(), ana)
	must(master.play.PutOnStage(t.Context(), rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: ps.npc})))
	name := w.needlesOf("partial-scene-name")[0]
	rest := slices.DeleteFunc(slices.Clone(ps.needles), func(n string) bool { return n == name })

	// Switch off: nothing, not even the name (the other test holds the needles).
	if _, found := holdsAny(w.openSceneAs(caio).body, ps.needles); found {
		t.Fatal("Caio reads the scene with the switch off")
	}
	must(master.play.SetScenePlayersMayJoin(t.Context(), rq(&playv1.SetScenePlayersMayJoinRequest{CampaignId: w.campaign, PlayersMayJoin: true})))
	w.settle(sm, sa, sc)

	// Switch on: the name reaches the outsiders; the master and Ana read all of it (the positive control).
	for _, p := range []*person{caio, w.pending, w.stranger, w.anonymous()} {
		r := w.openSceneAs(p)
		holdsName := bytes.Contains(r.body, []byte(name))
		isMember := p == caio
		if holdsName != isMember {
			t.Errorf("%s reads the name = %v, want %v: %s", p.name, holdsName, isMember, shorten(r.body))
		}
		if n, found := holdsAny(r.body, rest); found {
			t.Errorf("%s reads %q of a scene they may only join", p.name, n)
		}
	}
	for _, p := range []*person{master, ana} {
		r := w.openSceneAs(p)
		for _, n := range ps.needles {
			if !bytes.Contains(r.body, []byte(n)) {
				t.Errorf("%s's scene lacks %q: %s", p.name, n, shorten(r.body))
			}
		}
	}
	// No image of the scene, no portrait, no roll.
	portrait := ps.needles[len(ps.needles)-1]
	if status, _ := caio.get("/images/" + portrait); status == 200 {
		t.Error("Caio fetches the portrait of a scene he has not joined")
	}
	// Strangers, pending members and the master cannot join; the refusal of a stranger says nothing.
	for _, p := range []*person{master, w.pending, w.stranger, w.anonymous()} {
		if r := p.call(playv1connect.PlayServiceJoinSceneProcedure, &playv1.JoinSceneRequest{CampaignId: w.campaign}); r.ok() {
			t.Errorf("%s joined the scene", p.name)
		}
	}
	// Joining brings the stage.
	mark := sc.count()
	r := caio.call(playv1connect.PlayServiceJoinSceneProcedure, &playv1.JoinSceneRequest{CampaignId: w.campaign})
	if !r.ok() {
		t.Fatalf("Caio could not join: %s", r.body)
	}
	w.settle(sm, sa, sc)
	if _, found := holdsAny(w.openSceneAs(caio).body, ps.needles); !found || !bytes.Contains(w.openSceneAs(caio).body, []byte(w.needlesOf("partial-stage-name")[0])) {
		t.Error("Caio, who joined, does not read the stage")
	}
	if status, _ := caio.get("/images/" + portrait); status != 200 {
		t.Errorf("Caio gets HTTP %d for the portrait after joining, want 200", status)
	}
	if len(eventsAfter(sc, mark)) == 0 {
		t.Error("Caio's stream got no hint when he joined")
	}
}
