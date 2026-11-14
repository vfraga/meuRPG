package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// mountMark is SetCombatantMount as u, by label.
func (c *cave) mountMark(t *testing.T, u *user, label, kind string) (*playv1.Encounter, error) {
	t.Helper()
	res, err := u.combat.SetCombatantMount(t.Context(), connect.NewRequest(&playv1.SetCombatantMountRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label), IdempotencyKey: newKey(), Kind: kind,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

// mustMark is a mark of the master's that must work: setup.
func (c *cave) mustMark(t *testing.T, label, kind string) {
	t.Helper()
	if _, err := c.mountMark(t, c.master, label, kind); err != nil {
		t.Fatalf("SetCombatantMount(%s, %q) error = %v", label, kind, err)
	}
}

// ride puts the rider on the mount in the table, as the mount call of a later slice will: the rows only.
func (c *cave) ride(t *testing.T, rider, mount string) {
	t.Helper()
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE combatants SET mounted_on_id = $1 WHERE id = $2`, c.id(t, mount), c.id(t, rider)); err != nil {
		t.Fatalf("put %s on %s: %v", rider, mount, err)
	}
}

// TestMountedCombat_SetCombatantMount: only the master marks a willing mount; the kind is one of the three words; a player's
// character is never a mount; a rider is not marked and a mount that carries one keeps its mark; the call is one session event
// and replays by its key.
func TestMountedCombat_SetCombatantMount(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)

	if _, err := c.mountMark(t, c.caio, "Ogro", "controlled"); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player marking a mount = %v, want permission_denied", err)
	}
	if _, err := c.mountMark(t, c.master, "Ogro", "wild"); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a kind that is not one of the three = %v, want invalid_argument", err)
	}
	if _, err := c.mountMark(t, c.master, "Toren", "controlled"); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a player's character as a mount = %v, want invalid_argument", err)
	}
	if got := c.who(t, c.master, "Ogro").GetMountKind(); got != "" {
		t.Errorf("the ogre's mount kind before any mark = %q, want empty", got)
	}
	key := newKey()
	req := &playv1.SetCombatantMountRequest{CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Ogro"), IdempotencyKey: key, Kind: "controlled"}
	for range 2 { // the second call is the replay of the first
		if _, err := c.master.combat.SetCombatantMount(t.Context(), connect.NewRequest(req)); err != nil {
			t.Fatalf("SetCombatantMount(controlled) error = %v", err)
		}
	}
	if got := c.who(t, c.master, "Ogro").GetMountKind(); got != "controlled" {
		t.Errorf("the ogre's mount kind after the mark = %q, want controlled", got)
	}
	if _, err := c.mountMark(t, c.master, "Ogro", "independent"); err != nil {
		t.Errorf("a mount changing from controlled to independent = %v, want it allowed", err)
	}

	// A rider cannot be marked, and the mark of a mount that carries a rider stays until the rider gets off.
	c.ride(t, "Goblin 1", "Ogro")
	if b := wantEncounterBlocked(t, dropEncounter(c.mountMark(t, c.master, "Goblin 1", "controlled")), playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_MOUNTED); len(b.GetCombatantIds()) != 1 || b.GetCombatantIds()[0] != c.id(t, "Goblin 1") {
		t.Errorf("a rider marked as a mount: combatant_ids = %v, want the rider", b.GetCombatantIds())
	}
	wantEncounterBlocked(t, dropEncounter(c.mountMark(t, c.master, "Goblin 1", "")), playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_MOUNTED)
	wantEncounterBlocked(t, dropEncounter(c.mountMark(t, c.master, "Ogro", "")), playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_TAKEN)
	if got := c.who(t, c.master, "Ogro").GetMountKind(); got != "independent" {
		t.Errorf("the carrying ogre's mount kind after the refused unmark = %q, want independent", got)
	}
	if _, err := c.mountMark(t, c.master, "Ogro", "controlled"); err != nil {
		t.Errorf("a carrying mount changing its kind = %v, want it allowed", err)
	}
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE combatants SET mounted_on_id = NULL WHERE id = $1`, c.id(t, "Goblin 1")); err != nil {
		t.Fatalf("take the rider off: %v", err)
	}
	if _, err := c.mountMark(t, c.master, "Ogro", ""); err != nil {
		t.Errorf("taking the mark off a mount with no rider = %v, want it allowed", err)
	}

	// Three marks that worked (controlled twice counts once by the replay, independent, controlled, and the unmark) wrote one event each.
	var marks int
	if err := c.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = 'mount_kind_set'`).Scan(&marks); err != nil {
		t.Fatalf("count the events: %v", err)
	}
	if marks != 4 {
		t.Errorf("mount_kind_set events = %d, want 4 (controlled once for its replay, independent, controlled, unmarked)", marks)
	}
}

func dropEncounter(_ *playv1.Encounter, err error) error { return err }

// TestMountedCombat_ViewLinks: the links and the mark are read by who may read them (RN-10, RN-20). A visible pair shows its links to
// every player; the mark and the used move stay with the master and the owner; a hidden mount leaves its rider plain, with nothing
// of the mount in the player's copy, and the same copy of the master and of the player before the mount hid are the positive controls.
func TestMountedCombat_ViewLinks(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.mustMark(t, "Escudeiro", "independent")
	c.ride(t, "Toren", "Escudeiro")
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE combatants SET mount_move_used = true WHERE id = $1`, c.id(t, "Toren")); err != nil {
		t.Fatalf("mark the move used: %v", err)
	}
	squire, toren := c.id(t, "Escudeiro"), c.id(t, "Toren")

	for who, u := range map[string]*user{"the master": c.master, "Toren's player": c.caio, "another player": c.ana} {
		if got := c.who(t, u, "Toren").GetMountedOnId(); got != squire {
			t.Errorf("Toren's mounted_on_id for %s = %q, want the squire's", who, got)
		}
		if got := c.who(t, u, "Escudeiro").GetRiderId(); got != toren {
			t.Errorf("the squire's rider_id for %s = %q, want Toren's", who, got)
		}
		if got := c.who(t, u, "Toren").GetRiderId(); got != "" {
			t.Errorf("Toren's rider_id for %s = %q, want empty (he carries nobody)", who, got)
		}
	}
	wantMark := map[string]string{"the master": "independent", "Toren's player": "", "another player": ""}
	wantUsed := map[string]bool{"the master": true, "Toren's player": true, "another player": false}
	for who, u := range map[string]*user{"the master": c.master, "Toren's player": c.caio, "another player": c.ana} {
		if got := c.who(t, u, "Escudeiro").GetMountKind(); got != wantMark[who] {
			t.Errorf("the squire's mount_kind for %s = %q, want %q", who, got, wantMark[who])
		}
		if got := c.who(t, u, "Toren").GetMountMoveUsed(); got != wantUsed[who] {
			t.Errorf("Toren's mount_move_used for %s = %v, want %v", who, got, wantUsed[who])
		}
	}

	// A mount the master hid: the rider shows as plain, and nothing of the mount is in any player's copy.
	const canary = "Zorvak4321"
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE combatants SET label = $1 WHERE id = $2`, canary, c.id(t, "Ogro")); err != nil {
		t.Fatalf("name the ogre: %v", err)
	}
	c.mustMark(t, canary, "controlled")
	c.ride(t, "Goblin 1", canary)
	ogre, goblin := c.id(t, canary), c.id(t, "Goblin 1")
	copyOf := func(u *user) string { return protojson.Format(c.get(t, u)) }
	for who, u := range map[string]*user{"the master": c.master, "Toren's player": c.caio, "another player": c.ana} {
		if !strings.Contains(copyOf(u), canary) {
			t.Fatalf("the positive control failed: %s does not read %s while the ogre is shown", who, canary)
		}
		if got := c.who(t, u, "Goblin 1").GetMountedOnId(); got != ogre {
			t.Errorf("the goblin's mounted_on_id for %s with the mount shown = %q, want the ogre's", who, got)
		}
	}
	if _, err := c.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: ogre, IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	masterCopy := c.get(t, c.master)
	if got := byLabel(t, masterCopy, canary); got.GetRiderId() != goblin || got.GetMountKind() != "controlled" {
		t.Errorf("the hidden ogre for the master: rider %q, kind %q; want the goblin and controlled", got.GetRiderId(), got.GetMountKind())
	}
	if got := byLabel(t, masterCopy, "Goblin 1").GetMountedOnId(); got != ogre {
		t.Errorf("the goblin's mounted_on_id for the master = %q, want the hidden ogre's", got)
	}
	for who, u := range map[string]*user{"Toren's player": c.caio, "another player": c.ana} {
		json := copyOf(u)
		if strings.Contains(json, canary) || strings.Contains(json, ogre) {
			t.Errorf("%s reads the hidden mount in their copy", who)
		}
		if got := c.who(t, u, "Goblin 1").GetMountedOnId(); got != "" {
			t.Errorf("the goblin's mounted_on_id for %s with the mount hidden = %q, want empty", who, got)
		}
	}
}

// TestMountedCombat_ViewOfAnOwnedSteed: the player who controls the mount reads its mark and its link, and a rider that is theirs reads
// its own used move; another player reads the link and none of the rest. The view functions are the ones every copy is built with.
func TestMountedCombat_ViewOfAnOwnedSteed(t *testing.T) {
	t.Parallel()
	owner, other := "user-owner", "user-other"
	steed := playdb.Combatant{ID: "steed", Kind: kindCreature, UserID: &owner, MountKind: "controlled"}
	rider := playdb.Combatant{ID: "rider", Kind: kindNPC, MountedOnID: new("steed"), MountMoveUsed: true}
	hiddenSteed := steed
	hiddenSteed.Hidden = true
	cs := []playdb.Combatant{steed, rider}

	read := func(v combatViewer, c playdb.Combatant, all []playdb.Combatant) *playv1.Combatant {
		out := &playv1.Combatant{}
		mountFor(out, c, all, v)
		return out
	}
	if got := read(combatViewer{userID: owner}, steed, cs); got.GetMountKind() != "controlled" || got.GetRiderId() != "rider" {
		t.Errorf("the steed's owner reads kind %q and rider %q, want controlled and rider", got.GetMountKind(), got.GetRiderId())
	}
	if got := read(combatViewer{userID: other}, steed, cs); got.GetMountKind() != "" || got.GetRiderId() != "rider" {
		t.Errorf("another player reads kind %q and rider %q, want empty and rider", got.GetMountKind(), got.GetRiderId())
	}
	if got := read(combatViewer{userID: other}, rider, cs); got.GetMountedOnId() != "steed" {
		t.Errorf("another player reads the rider's mounted_on_id %q, want steed", got.GetMountedOnId())
	}
	hidden := []playdb.Combatant{hiddenSteed, rider}
	if got := read(combatViewer{userID: owner}, rider, hidden); got.GetMountedOnId() != "" {
		t.Errorf("a player reads a hidden mount's id %q, want empty", got.GetMountedOnId())
	}
	hiddenRider := rider
	hiddenRider.Hidden = true
	if got := read(combatViewer{userID: owner}, steed, []playdb.Combatant{steed, hiddenRider}); got.GetRiderId() != "" {
		t.Errorf("a player reads a hidden rider's id %q on the mount, want empty", got.GetRiderId())
	}
	if got := read(combatViewer{master: true}, rider, hidden); got.GetMountedOnId() != "steed" {
		t.Errorf("the master reads %q, want steed", got.GetMountedOnId())
	}
	if got := read(combatViewer{master: true}, hiddenSteed, hidden); got.GetRiderId() != "rider" || got.GetMountKind() != "controlled" {
		t.Errorf("the master reads the hidden steed's rider %q and kind %q, want rider and controlled", got.GetRiderId(), got.GetMountKind())
	}
}

// TestMountedCombat_TurnStartClearsTheMountMove: the once-per-move mark of mounting ends with the movement counters, when the
// rider's next turn starts.
func TestMountedCombat_TurnStartClearsTheMountMove(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	if _, err := c.h.pool.Exec(t.Context(), `UPDATE combatants SET mount_move_used = true WHERE id = $1`, c.id(t, "Toren")); err != nil {
		t.Fatalf("mark the move used: %v", err)
	}
	if !c.who(t, c.master, "Toren").GetMountMoveUsed() {
		t.Fatalf("the positive control failed: the mark is not read before the turn ends")
	}
	c.nextTurnOf(t, "Toren")
	if c.who(t, c.master, "Toren").GetMountMoveUsed() {
		t.Errorf("mount_move_used is still set at the start of the rider's next turn")
	}
}
