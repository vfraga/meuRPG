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

// firedTrapIsPublic lets the players read the guard-room trap's name and id: the master
// fired it, and a trap that fired is public.
func firedTrapIsPublic(w *world) {
	for _, c := range w.secrets.list {
		if c.kind == "trap-guardroom-name" || c.needle == w.pts["trap-guardroom"].GetId() {
			c.readers = names([]*person{w.ana, w.caio})
		}
	}
}

func trapLog(w *world, p *person) reply {
	w.t.Helper()
	r := p.call(playv1connect.CombatServiceListCombatLogProcedure, &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})
	if !r.ok() {
		w.t.Fatalf("%s ListCombatLog: status %d %s", p.name, r.status, r.body)
	}
	return r
}

// A trap the master fires by hand is public from then on: its name and point reach every
// player in the combat log, wherever it stands, while the map still hides its square from
// whoever does not see it. The firing is the master's act; the fog hides places, not the
// trap that went off.
func TestFiredTrapIsNamedInTheLogButItsSquareStaysHidden(t *testing.T) {
	w := newWorld(t)
	fireByHand(w, w.combatantOf(w.encounter, w.boss.GetId()).GetId())
	for _, p := range []*person{w.ana, w.caio} {
		// The map read keeps hiding that square from the same player.
		m := p.call("/meurpg.maps.v1.MapService/GetMap", &mapsv1.GetMapRequest{CampaignId: w.campaign, MapId: w.fogMap})
		for _, f := range w.inspect(p, m, nil) {
			t.Errorf("%s GetMap: %s", p.name, f)
		}
	}
	firedTrapIsPublic(w)
	for _, p := range []*person{w.ana, w.caio} {
		r := trapLog(w, p)
		if !strings.Contains(string(r.body), "COMBAT_LOG_KIND_TRAP_TRIGGERED") {
			t.Errorf("%s reads no TRAP_TRIGGERED entry", p.name)
		}
		for _, f := range w.inspect(p, r, nil) {
			t.Errorf("%s: %s", p.name, f)
		}
	}
}

// The characters a trap caught reach the players as the combat shows them: a player's
// character by id, an NPC's never (an NPC's character is the master's secret), and a
// hidden NPC not at all.
func TestFiredTrapLogHoldsNoNPCCharacterID(t *testing.T) {
	w := newWorld(t)
	fireByHand(w,
		w.combatantOf(w.encounter, w.seenNPC.GetId()).GetId(),
		w.combatantOf(w.encounter, w.hiddenNPC.GetId()).GetId(),
		w.combatantOf(w.encounter, w.pens.GetId()).GetId())
	firedTrapIsPublic(w)
	for _, p := range []*person{w.ana, w.caio} {
		r := trapLog(w, p)
		for _, f := range w.inspect(p, r, nil) {
			t.Errorf("%s: %s", p.name, f)
		}
	}
}
