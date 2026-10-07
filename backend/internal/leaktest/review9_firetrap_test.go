package leaktest

import (
	"strings"
	"testing"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// fireByHand has the master fire the guard-room trap (never revealed) by hand in the running combat.
func fireByHand(w *world, targets ...string) {
	w.t.Helper()
	must(w.master.play.FireTrap(w.t.Context(), rq(&playv1.FireTrapRequest{
		CampaignId: w.campaign, MapId: w.fogMap, PointId: w.pts["trap-guardroom"].GetId(),
		TargetIds: targets, IdempotencyKey: newKey(),
	})))
}

func trapLog(w *world, p *person) reply {
	w.t.Helper()
	r := p.call(playv1connect.CombatServiceListCombatLogProcedure, &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})
	if !r.ok() {
		w.t.Fatalf("%s ListCombatLog: status %d %s", p.name, r.status, r.body)
	}
	return r
}

// Finding U9-16: a hand-fired, never revealed trap puts its point id and name in every player's combat log.
func TestReview9_FireTrapHiddenTrapNameInLog(t *testing.T) {
	w := newWorld(t)
	fireByHand(w, w.combatantOf(w.encounter, w.boss.GetId()).GetId())
	for _, p := range []*person{w.ana, w.caio} {
		r := trapLog(w, p)
		for _, f := range w.inspect(p, r, nil) {
			t.Errorf("%s: %s", p.name, f)
		}
		if strings.Contains(string(r.body), "COMBAT_LOG_KIND_TRAP_TRIGGERED") {
			t.Logf("%s reads a TRAP_TRIGGERED entry", p.name)
		}
		// The map read must keep hiding that square from the same player.
		m := p.call("/meurpg.maps.v1.MapService/GetMap", &mapsv1.GetMapRequest{CampaignId: w.campaign, MapId: w.fogMap})
		for _, f := range w.inspect(p, m, nil) {
			t.Errorf("%s GetMap: %s", p.name, f)
		}
	}
}

// Finding U9-17: trap.caught[].character_id of a seen NPC (a master-only id) reaches the players' combat log.
func TestReview9_FireTrapCaughtNPCCharacterId(t *testing.T) {
	w := newWorld(t)
	fireByHand(w,
		w.combatantOf(w.encounter, w.seenNPC.GetId()).GetId(),
		w.combatantOf(w.encounter, w.hiddenNPC.GetId()).GetId(),
		w.combatantOf(w.encounter, w.pens.GetId()).GetId())
	for _, p := range []*person{w.ana, w.caio} {
		r := trapLog(w, p)
		for _, f := range w.inspect(p, r, nil) {
			t.Errorf("%s: %s", p.name, f)
		}
	}
}
