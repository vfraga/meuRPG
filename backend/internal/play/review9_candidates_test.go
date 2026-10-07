package play

import (
	"encoding/json"
	"testing"
	"time"

	"connectrpc.com/connect"
	"uuid"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// sessionEventCount counts the events of a kind written into one game session.
func sessionEventCount(t *testing.T, h *harness, sessionID, kind string) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE game_session_id = $1 AND kind = $2`, sessionID, kind).Scan(&n); err != nil {
		t.Fatalf("count %s events: %v", kind, err)
	}
	return n
}

// Finding U9-08: settleTrapDamage passes the trap's (ended) session to changeVitals, so a Wild Shape ending at 0 HP logs wild_shape_ended there and leaves the open session's combatant with the beast's numbers.
func TestReview9_TrapDamageEndsFormInOpenSession(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	ctx := t.Context()
	s1 := s.master.liveSession(t, s.campaignID).GetGameSession()
	d, err := s.h.svc.queries.InsertTrapDamage(ctx, playdb.InsertTrapDamageParams{
		GameSessionID: s1.GetId(), TrapPointID: uuid.New().String(), FireID: uuid.New().String(), CharacterID: s.bri.GetId(),
		DiceCount: 0, DiceSides: 0, DiceBonus: 50, DamageType: "damage-type:piercing", Faces: []int32{}, RollTotal: 50, Amount: 50, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("InsertTrapDamage() error = %v", err)
	}
	s.master.end(t, s1)
	s2 := s.master.start(t, s.campaignID).GetGameSession()
	if s2.GetId() == s1.GetId() {
		t.Fatalf("session 2 has session 1's ID")
	}
	s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}},
		npcRolls: []int{1},
		players:  map[string]int32{"Sálvia": 20, "Irmã": 15, "Pensantus": 12, "Toren": 10},
		reveal:   []string{"Goblin"},
		theatre:  true,
	})
	s.mustAssume(t, s.bia, s.bri, wolfKey)
	if got := byLabel(t, s.get(t, s.master), "Sálvia"); got.GetSpeedFt() != 40 {
		t.Fatalf("Sálvia as a wolf = %d ft, want 40", got.GetSpeedFt())
	}
	if _, err := s.master.play.ApplyTrapDamage(ctx, connect.NewRequest(&playv1.ApplyTrapDamageRequest{CampaignId: s.campaignID, TrapDamageId: d.ID, IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("ApplyTrapDamage() error = %v", err)
	}
	if v := s.vitals(t, s.bri); v.GetWildShape() != nil {
		t.Fatalf("the beast is still on after 50 damage: %v", v)
	}
	if got := sessionEventCount(t, s.h, s2.GetId(), eventWildShapeEnded); got != 1 {
		t.Errorf("wild_shape_ended events in the open session 2 = %d, want 1 (in session 1: %d)", got, sessionEventCount(t, s.h, s1.GetId(), eventWildShapeEnded))
	}
	if got := byLabel(t, s.get(t, s.master), "Sálvia"); got.GetSpeedFt() != 30 || got.GetWildShapeBeastKey() != "" {
		t.Errorf("combatant in session 2's combat after the beast fell = %d ft, form %q, want her own 30 ft and no form", got.GetSpeedFt(), got.GetWildShapeBeastKey())
	}
}

// Finding U9-09: RemoveCombatant of an attacker cascade-deletes its rolled pending damage with no damage_discarded event and no refusal, unlike EndTurn.
func TestReview9_RemoveAttackerKeepsDamageTrail(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId(), Hidden: new(false)}},
		npcRolls: []int{19},
		players:  map[string]int32{"Toren": 12, "Pensantus": 8, "Brisa": 1},
		at:       map[string][2]int32{"Capitão Goblin": {6, 3}, "Toren": {5, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	a.h.roller.queue(15, 4)
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Toren", inAppRoll)
	pend := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	if pend.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED {
		t.Fatalf("the damage = %v, want rolled and waiting for the master", pend)
	}
	_, err := a.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Capitão Goblin"), IdempotencyKey: newKey(),
	}))
	if err != nil {
		return // refused while a damage waits: acceptable
	}
	var left, discarded int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM pending_damages WHERE id = $1`, pend.GetId()).Scan(&left); err != nil {
		t.Fatalf("count pending damages: %v", err)
	}
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1`, eventDamageDiscarded).Scan(&discarded); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if left == 0 && discarded == 0 {
		t.Errorf("RemoveCombatant deleted the rolled damage %s with no damage_discarded event and no refusal", pend.GetId())
	}
}

// Finding U9-10: ListTrapEventsOfSession is oldest-first LIMIT 500, so ListTrapActivity loses the newest trap events of a long session.
func TestReview9_TrapActivityKeepsNewest(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	ctx := t.Context()
	one := r.trap(t, "Um", 12, 7, func(s *mapsv1.TrapSpec) { s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL })
	sess := r.master.liveSession(t, r.campaignID).GetGameSession()
	payload, err := json.Marshal(map[string]any{"point_id": one.GetId(), "character_ids": []string{r.toren.GetId()}})
	if err != nil {
		t.Fatal(err)
	}
	for range 501 {
		seq, err := r.h.svc.queries.NextSessionEventSeq(ctx, sess.GetId())
		if err != nil {
			t.Fatalf("NextSessionEventSeq() error = %v", err)
		}
		if _, err := r.h.svc.queries.InsertSessionEvent(ctx, playdb.InsertSessionEventParams{
			GameSessionID: sess.GetId(), Seq: seq, Kind: eventTrapNoticed, CharacterID: new(r.toren.GetId()), Payload: payload, CreatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("InsertSessionEvent() error = %v", err)
		}
	}
	r.place(t, r.pens.GetId(), 12, 7)
	if _, err := r.fireByHand(t, one); err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	act := r.activity(t, r.master)
	if len(act) == 0 || act[len(act)-1].GetFiring() == nil {
		t.Errorf("the last of %d trap activity lines is not the newest firing: the 501 older notices pushed it out of the 500-row window", len(act))
	}
}

// Finding U9-07: SearchForTraps replays a stored search by key without checking the actor, so another player's same key returns the first player's roll and found traps.
func TestReview9_SearchReplayIsPerActor(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hidden := r.trap(t, "Fosso Oculto", 4, 4, func(s *mapsv1.TrapSpec) { s.FindDc = 10 })
	r.place(t, r.toren.GetId(), 3, 3)
	key := newKey()
	search := func(u *user) (*playv1.SearchForTrapsResponse, error) {
		res, err := u.play.SearchForTraps(t.Context(), connect.NewRequest(&playv1.SearchForTrapsRequest{
			CampaignId: r.campaignID, IdempotencyKey: key, Skill: investigation, Roll: &playv1.SearchForTrapsRequest_D20Face{D20Face: 12},
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	first, err := search(r.caio)
	if err != nil || len(first.GetFoundPointIds()) != 1 || first.GetFoundPointIds()[0] != hidden.GetId() {
		t.Fatalf("Toren's player's search = %v, %v; want the hidden pit", first, err)
	}
	second, err := search(r.ana)
	if err == nil {
		t.Errorf("another player's search with Toren's key got %v, want an error (not Toren's result)", second)
	}
}
