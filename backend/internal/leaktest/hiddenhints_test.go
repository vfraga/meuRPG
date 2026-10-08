package leaktest

import (
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// hiddenOnlyEvents opens Ana's and Caio's streams, runs act (master actions on hidden things
// only), then a visible marker, and returns the encounter/log hints each person (the master
// included, as the positive control) got before it.
func (w *world) hiddenOnlyEvents(act func()) map[string][]string {
	watchers := map[*person]*streamWatcher{w.master: w.watchStream(w.master), w.ana: w.watchStream(w.ana), w.caio: w.watchStream(w.caio)}
	act()
	// a visible marker, so the end of the hidden part is known by the stream's own order
	must(w.master.maps.SetMapRevealed(w.t.Context(), rq(&mapsv1.SetMapRevealedRequest{CampaignId: w.campaign, MapId: w.map2, Revealed: true})))
	must(w.master.play.SetCurrentMap(w.t.Context(), rq(&playv1.SetCurrentMapRequest{CampaignId: w.campaign, MapId: w.map2})))
	time.Sleep(300 * time.Millisecond)
	must(w.master.play.EndGameSession(w.t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	out := map[string][]string{}
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			w.t.Fatalf("%s's stream did not end", p.name)
		}
		for _, ev := range sw.events {
			c := eventCase(ev)
			if c == "encounter_changed" || c == "combat_log_changed" {
				out[p.name] = append(out[p.name], c+" "+shorten(mustJSON(ev)))
				continue
			}
			if c != "ready" && c != "heartbeat" {
				break // the marker: what follows is not the hidden part
			}
		}
	}
	return out
}

func mustJSON(ev *playv1.WatchGameSessionResponse) []byte {
	b, _ := protojson.Marshal(ev)
	return b
}

// A master edit (HP, conditions, side, cover) of a hidden combatant sends the players no hint, not even a content-free one: it would let them count the edits.
func TestHiddenCombatantEditSendsPlayersNoHint(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	e := w.encounter
	hid := w.combatant(w.hiddenNPC)
	if !hid.GetHidden() {
		t.Fatalf("the fixture's hidden NPC combatant is not hidden")
	}
	got := w.hiddenOnlyEvents(func() {
		c := w.master.combat
		must(c.AdjustCombatantHitPoints(ctx, rq(&playv1.AdjustCombatantHitPointsRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: hid.GetId(), IdempotencyKey: newKey(), Change: &playv1.AdjustCombatantHitPointsRequest_Damage{Damage: 1}})))
		must(c.SetCombatantConditions(ctx, rq(&playv1.SetCombatantConditionsRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: hid.GetId(), IdempotencyKey: newKey(), Conditions: &playv1.ConditionList{Keys: []string{"condition:poisoned"}}})))
		must(c.SetCombatantSide(ctx, rq(&playv1.SetCombatantSideRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: hid.GetId(), IdempotencyKey: newKey(), Side: playv1.CombatantSide_COMBATANT_SIDE_PARTY})))
		must(c.SetCombatantCover(ctx, rq(&playv1.SetCombatantCoverRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: hid.GetId(), IdempotencyKey: newKey(), Cover: playv1.CoverDegree_COVER_DEGREE_HALF})))
	})
	requireOnlyMaster(t, w, got, "an edit made only to a hidden combatant")
}

// Adding hidden monsters sends the players no hint.
func TestHiddenMonstersAddedSendPlayersNoHint(t *testing.T) {
	w := newWorld(t)
	hidden := true
	got := w.hiddenOnlyEvents(func() {
		must(w.master.combat.AddMonsters(t.Context(), rq(&playv1.AddMonstersRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), CreatureKey: "monster:bandit", Count: 1, Name: w.secrets.marker("mon-hidden-hint"), Hidden: &hidden})))
	})
	requireOnlyMaster(t, w, got, "a hidden monster being added")
}

// requireOnlyMaster fails when a player heard of what the master did, and when the master did not.
func requireOnlyMaster(t *testing.T, w *world, got map[string][]string, what string) {
	t.Helper()
	if len(got[w.master.name]) == 0 {
		t.Errorf("the master did not hear of %s", what)
	}
	for name, evs := range got {
		if name == w.master.name {
			continue
		}
		for _, ev := range evs {
			t.Errorf("%s heard of %s: %s", name, what, ev)
		}
	}
}
