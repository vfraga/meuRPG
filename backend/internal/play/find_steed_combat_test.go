package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Find Steed in a combat: the steed joins as a controlled mount with the caster on it, on the caster's square.
// At 0 hit points it goes through the dismount window of the rider and then disappears.
// Its bond stays for the next cast.

// steedRow is what the table keeps of a combatant in the ride columns.
type steedRow struct {
	MountKind, MountedOn string
	Col, Row             int32
	Initiative           int32
	Conditions           string
}

func (a *armed) steedRowOf(t *testing.T, label string) steedRow {
	t.Helper()
	var r steedRow
	var col, row, init *int32
	var on *string
	err := a.h.pool.QueryRow(t.Context(),
		`SELECT mount_kind, mounted_on_id::text, grid_col, grid_row, initiative, COALESCE(array_to_string(conditions, ','), '') FROM combatants WHERE id = $1`,
		a.id(t, label)).Scan(&r.MountKind, &on, &col, &row, &init, &r.Conditions)
	if err != nil {
		t.Fatalf("read the ride columns of %s: %v", label, err)
	}
	if on != nil {
		r.MountedOn = *on
	}
	if col != nil && row != nil {
		r.Col, r.Row = *col, *row
	}
	if init != nil {
		r.Initiative = *init
	}
	return r
}

// riderInFight casts Find Steed outside the combat (the spell takes ten minutes), starts a combat with the steed in it, and has Aurélio mount
// it: the combat, after the mount.
func riderInFight(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	return steedFight(t, true)
}

// steedFight is riderInFight with the choice of mounting.
func steedFight(t *testing.T, mount bool) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newRiders(t)
	a.mustCastSteed(t, warhorse, "celestial", "Trovão")
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Aurélio": 18, "Toren": 10, "Trovão": 12},
		reveal:  []string{"Goblin"},
		at:      map[string][2]int32{"Aurélio": {3, 3}, "Toren": {6, 3}, "Goblin": {12, 3}, "Trovão": {4, 3}},
	})
	if mount {
		if _, err := a.ana.combat.MountCombatant(t.Context(), connect.NewRequest(&playv1.MountCombatantRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), RiderId: a.id(t, "Aurélio"), MountId: a.id(t, "Trovão"), IdempotencyKey: newKey(),
		})); err != nil {
			t.Fatalf("MountCombatant(Aurélio, Trovão) error = %v", err)
		}
	}
	after := a.get(t, a.master)
	return a, after
}

// TestFindSteedJoinsTheCombatAsAControlledMount: the steed enters the combat marked as a controlled mount, so the rider mounts it with no
// mark from the master, and the pair then shares the steed's square and its initiative.
func TestFindSteedJoinsTheCombatAsAControlledMount(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	a.mustCastSteed(t, warhorse, "celestial", "Trovão")
	a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Aurélio": 18, "Toren": 10, "Trovão": 12},
		reveal:  []string{"Goblin"},
		at:      map[string][2]int32{"Aurélio": {3, 3}, "Toren": {6, 3}, "Goblin": {12, 3}, "Trovão": {4, 3}},
	})
	if got := a.steedRowOf(t, "Trovão").MountKind; got != "controlled" {
		t.Errorf("the steed's mount kind = %q, want controlled", got)
	}
}

// TestFindSteedPairSharesTheSquareAndTheInitiative: once Aurélio sits on the steed the two share the steed's square and the steed takes his
// initiative.
func TestFindSteedPairSharesTheSquareAndTheInitiative(t *testing.T) {
	t.Parallel()
	a, _ := riderInFight(t)
	steed, rider := a.steedRowOf(t, "Trovão"), a.steedRowOf(t, "Aurélio")
	if steed.MountKind != "controlled" {
		t.Errorf("the steed's mount kind = %q, want controlled", steed.MountKind)
	}
	if rider.MountedOn != a.id(t, "Trovão") {
		t.Errorf("Aurélio rides %q, want the steed %s", rider.MountedOn, a.id(t, "Trovão"))
	}
	if steed.Col != 4 || steed.Row != 3 || rider.Col != 4 || rider.Row != 3 {
		t.Errorf("the pair stands on (%d,%d) and (%d,%d), want (4,3) both", rider.Col, rider.Row, steed.Col, steed.Row)
	}
	if steed.Initiative != rider.Initiative {
		t.Errorf("the steed's initiative = %d, want the rider's %d", steed.Initiative, rider.Initiative)
	}
}

// TestFindSteedAtZeroHitPointsGoesThroughTheDismountWindow: a steed brought to 0 opens the rider's dismount window; the rider's answer lands
// it, and the steed then disappears: its creature is dismissed as 'disappeared', the bond stays, and a new cast brings it back.
func TestFindSteedAtZeroHitPointsGoesThroughTheDismountWindow(t *testing.T) {
	t.Parallel()
	a, e := riderInFight(t)
	if _, err := a.adjustHP(t, e, "Trovão", damageHP(500)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints(Trovão) error = %v", err)
	}
	w := a.windowOf(t, a.ana, reactionDismount)
	if w == nil {
		t.Fatal("the rider's player reads no dismount window")
	}
	if got := a.steedRowOf(t, "Aurélio"); got.MountedOn != a.id(t, "Trovão") {
		t.Errorf("before the answer Aurélio rides %q, want the steed still under him", got.MountedOn)
	}
	var present int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM combatants WHERE label = 'Trovão' AND NOT dismissed`).Scan(&present); err != nil || present != 1 {
		t.Errorf("before the answer the steed's combatants = %d (%v), want 1", present, err)
	}
	a.mustAnswer(t, a.ana, e, w.GetId(), useAnswer(0))
	if got := a.steedRowOf(t, "Aurélio"); got.MountedOn != "" || strings.Contains(got.Conditions, "prone") {
		t.Errorf("Aurélio after the landing = %+v, want on foot and on his feet", got)
	}
	var in int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM combatants WHERE creature_id IS NOT NULL AND label = 'Trovão' AND NOT dismissed`).Scan(&in); err != nil {
		t.Fatalf("count the steed's combatants: %v", err)
	}
	if in != 0 {
		t.Errorf("the steed is still in the combat after the landing")
	}
	var reason *string
	var bonded bool
	var hp int32
	if err := a.h.pool.QueryRow(t.Context(), `SELECT dismissed_reason, bonded, hp_current FROM character_creatures WHERE source = 'find_steed'`).Scan(&reason, &bonded, &hp); err != nil {
		t.Fatalf("read the steed's creature: %v", err)
	}
	if hp != 0 {
		t.Errorf("the steed that fell has %d hit points stored, want 0", hp)
	}
	if got := ptrOr(reason, "(none)"); got != "disappeared" || !bonded {
		t.Errorf("the steed's creature: reason = %q, bonded = %v, want disappeared and bonded", got, bonded)
	}
}

func ptrOr(s *string, none string) string {
	if s == nil {
		return none
	}
	return *s
}

// TestFindSteedDismissedUnderItsRiderLandsTheRiderOnItsFeet: the master sending the steed away puts the rider on foot where it stands, with no
// dismount window and no fall, and the bond stays for the next cast.
func TestFindSteedDismissedUnderItsRiderLandsTheRiderOnItsFeet(t *testing.T) {
	t.Parallel()
	a, _ := riderInFight(t)
	var id string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT id::text FROM character_creatures WHERE source = 'find_steed'`).Scan(&id); err != nil {
		t.Fatalf("read the steed's creature: %v", err)
	}
	a.dismissCreatureAs(t, a.master, id)
	got := a.steedRowOf(t, "Aurélio")
	if got.MountedOn != "" || strings.Contains(got.Conditions, "prone") {
		t.Errorf("Aurélio after the dismissal = %+v, want on foot and on his feet", got)
	}
	if a.windowOf(t, a.ana, reactionDismount) != nil {
		t.Error("the dismissal opened a dismount window")
	}
	var bonded bool
	if err := a.h.pool.QueryRow(t.Context(), `SELECT bonded FROM character_creatures WHERE id = $1`, id).Scan(&bonded); err != nil || !bonded {
		t.Errorf("the dismissed steed: bonded = %v, err = %v, want the bond kept", bonded, err)
	}
}

// TestFindSteedHealedBeforeTheAnswerIsBackWithItsHitPoints: the steed at 0 is dismissed as 'disappeared' at once, and a heal in the combat
// brings the same creature back with the hit points of its combatant, still mounted.
func TestFindSteedHealedBeforeTheAnswerIsBackWithItsHitPoints(t *testing.T) {
	t.Parallel()
	a, e := riderInFight(t)
	if _, err := a.adjustHP(t, e, "Trovão", damageHP(500)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints(damage) error = %v", err)
	}
	if _, err := a.adjustHP(t, e, "Trovão", healHP(7)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints(heal) error = %v", err)
	}
	var reason *string
	var hp int32
	if err := a.h.pool.QueryRow(t.Context(), `SELECT dismissed_reason, hp_current FROM character_creatures WHERE source = 'find_steed'`).Scan(&reason, &hp); err != nil {
		t.Fatalf("read the steed's creature: %v", err)
	}
	if reason != nil || hp != 7 {
		t.Errorf("the healed steed: reason = %q, hit points = %d, want live with 7", ptrOr(reason, "(none)"), hp)
	}
	if got := a.steedRowOf(t, "Aurélio"); got.MountedOn != a.id(t, "Trovão") {
		t.Errorf("after the heal Aurélio rides %q, want the steed", got.MountedOn)
	}
}

// TestFindSteedPonyAndMastiffAreTooSmallForAMediumRider: the pony and the mastiff are Small steeds, so a Medium caster cannot mount them
// (SRD 5.1 asks a mount one size larger); the combat refuses with MOUNT_TOO_SMALL and leaves the pair apart.
func TestFindSteedPonyAndMastiffAreTooSmallForAMediumRider(t *testing.T) {
	t.Parallel()
	for _, form := range []string{"monster:pony", "monster:mastiff"} {
		a := newRiders(t)
		a.mustCastSteed(t, form, "celestial", "Trovão")
		e := a.start(t, plan{
			npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
			players: map[string]int32{"Aurélio": 18, "Toren": 10, "Trovão": 12},
			reveal:  []string{"Goblin"},
			at:      map[string][2]int32{"Aurélio": {3, 3}, "Toren": {6, 3}, "Goblin": {12, 3}, "Trovão": {4, 3}},
		})
		_, err := a.ana.combat.MountCombatant(t.Context(), connect.NewRequest(&playv1.MountCombatantRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), RiderId: a.id(t, "Aurélio"), MountId: a.id(t, "Trovão"), IdempotencyKey: newKey(),
		}))
		b := wantBlockedBy(t, "a Medium paladin on the "+form, err, reasonMountTooSmall)
		if len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != a.id(t, "Trovão") {
			t.Errorf("%s: combatant_ids = %v, want the steed", form, b.GetCombatantIds())
		}
	}
}
