package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

var reasonCombatantDown = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_COMBATANT_DOWN

// downToren puts Toren's character at 0 hit points, as a hit would.
func (c *cave) downToren(t *testing.T) {
	t.Helper()
	if _, err := c.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: c.campaignID, CharacterId: c.toren.GetId(), IdempotencyKey: newKey(), ExpectedRevision: c.master.revisionOf(t, c.campaignID, c.toren.GetId()), HitPointsCurrent: proto.Int32(0),
	})); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
}

// TestMountedCombat_ADownRiderKeepsItsMountsTurnAndLimit pins what happens today: a rider at 0 hit points does not take the mount out of
// the pair. The Ogro keeps the rider's initiative and its Dash, Disengage and Dodge limit, its rider's player does not move it
// (COMBATANT_DOWN: a down rider directs nothing) and the master does. The slice that makes a rider that falls to 0 get off its mount closes this.
func TestMountedCombat_ADownRiderKeepsItsMountsTurnAndLimit(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.ridden(t)
	before := c.orderNow(t)
	c.downToren(t)
	if got := c.orderNow(t); !slices.Equal(got, before) {
		t.Errorf("order with the rider down = %v, want %v", got, before)
	}
	if got := group(c.get(t, c.master)); got != "Toren,Ogro" {
		t.Errorf("group with the rider down = %q, want Toren,Ogro", got)
	}
	_, err := c.move(t, c.caio, "Ogro", 8, 8)
	wantBlockedBy(t, "the down rider's player moving the mount", err, reasonCombatantDown)
	_, err = c.attack(t, c.master, c.get(t, c.master), "Ogro", "basic:0", "Goblin 1", d20(15))
	wantBlockedBy(t, "an attack of the mount of a down rider", err, reasonMountActionLimited)
	if _, err := c.move(t, c.master, "Ogro", 8, 8); err != nil {
		t.Errorf("the master moving the mount of a down rider = %v, want it moved", err)
	}
	if got := c.rideRowOf(t, "Toren"); got.Col != 8 || got.Row != 8 || got.MountedOn == "" {
		t.Errorf("the down rider after its mount moved = %+v, want carried to (8,8) and still on it", got)
	}
}
