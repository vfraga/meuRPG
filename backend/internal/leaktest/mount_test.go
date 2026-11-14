package leaktest

import (
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A rider on a mount the master hid reads as a plain combatant to the players: no id, no name of the mount in any copy they get,
// and the mark the master put on it stays the master's. The master's copy and a player's copy after the reveal are the positive controls.
func TestHiddenMountLeavesItsRiderPlain(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	e := w.encounter
	mount, rider := w.combatant(w.hiddenNPC), w.combatant(w.seenNPC)
	if !mount.GetHidden() {
		t.Fatalf("the fixture's hidden NPC combatant is not hidden")
	}
	must(w.master.combat.SetCombatantMount(ctx, rq(&playv1.SetCombatantMountRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: mount.GetId(), IdempotencyKey: newKey(), Kind: "controlled"})))
	if _, err := w.pool.Exec(ctx, `UPDATE combatants SET mounted_on_id = $1 WHERE id = $2`, mount.GetId(), rider.GetId()); err != nil {
		t.Fatalf("put the rider on the mount: %v", err)
	}
	read := func(p *person) string {
		return jsonOf(must(p.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))))
	}
	// The positive control: the master reads the link, the mark and the mount's name.
	masterCopy := read(w.master)
	for _, needle := range []string{mount.GetId(), mount.GetLabel(), "mountKind"} {
		if !strings.Contains(masterCopy, needle) {
			t.Fatalf("the positive control failed: the master's copy has no %s", needle)
		}
	}
	for _, p := range []*person{w.ana, w.caio} {
		got := read(p)
		for _, needle := range []string{mount.GetId(), mount.GetLabel(), "mountKind", "mountedOnId", "riderId"} {
			if strings.Contains(got, needle) {
				t.Errorf("%s reads %q while the mount is hidden", p.name, needle)
			}
		}
	}
	// The reveal is the second positive control: a player who sees both rows reads the link and the mount, and still not the mark.
	must(w.master.combat.SetCombatantHidden(ctx, rq(&playv1.SetCombatantHiddenRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: mount.GetId(), IdempotencyKey: newKey(), Hidden: false})))
	got := read(w.ana)
	for _, needle := range []string{mount.GetId(), mount.GetLabel(), "mountedOnId", "riderId"} {
		if !strings.Contains(got, needle) {
			t.Errorf("the positive control failed: Ana reads no %q after the mount is revealed", needle)
		}
	}
	if strings.Contains(got, "mountKind") {
		t.Errorf("Ana reads the master's mark of a mount that is not hers")
	}
}

// The master puts a rider on a mount he keeps hidden: the call, its line of the log and the hints of the stream never tell a player the
// mount is there, not even after the master reveals it, while the master reads all of it. The rider is the visible NPC of the fixture,
// brought on turn; the mount is the hidden one, made Large and put next to it.
func TestAHiddenMountsRideNeverReachesPlayers(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	e := w.encounter
	mount, rider := w.combatant(w.hiddenNPC), w.combatant(w.seenNPC)
	for range 60 {
		cur := must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == rider.GetId() {
			break
		}
		must(w.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	if _, err := w.pool.Exec(ctx, `UPDATE combatants SET size = 'large' WHERE id = $1`, mount.GetId()); err != nil {
		t.Fatalf("make the mount Large: %v", err)
	}
	w.placeForced(w.master, e.GetId(), placement{mount.GetId(), 14, 8}, placement{rider.GetId(), 13, 8})
	must(w.master.combat.SetCombatantMount(ctx, rq(&playv1.SetCombatantMountRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: mount.GetId(), IdempotencyKey: newKey(), Kind: "controlled"})))

	read := func(p *person) string {
		log := jsonOf(must(p.combat.ListCombatLog(ctx, rq(&playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: e.GetId()}))))
		return log + jsonOf(must(p.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))))
	}
	// The reads happen inside the act: the helper ends the game session after it, and the combat is read in a session.
	people := []*person{w.master, w.ana, w.caio}
	hiding, revealed := map[string]string{}, map[string]string{}
	got := w.hiddenOnlyEvents(func() {
		must(w.master.combat.MountCombatant(ctx, rq(&playv1.MountCombatantRequest{CampaignId: w.campaign, EncounterId: e.GetId(), RiderId: rider.GetId(), MountId: mount.GetId(), IdempotencyKey: newKey()})))
		for _, p := range people {
			hiding[p.name] = read(p)
		}
		must(w.master.combat.SetCombatantHidden(ctx, rq(&playv1.SetCombatantHiddenRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: mount.GetId(), IdempotencyKey: newKey(), Hidden: false})))
		for _, p := range people {
			revealed[p.name] = read(p)
		}
	})
	heard := func(name string) string { return strings.Join(got[name], "\n") }
	if len(got[w.master.name]) == 0 {
		t.Errorf("the positive control failed: the master heard nothing of the mount")
	}
	if masterRead := hiding[w.master.name]; !strings.Contains(masterRead, "COMBAT_LOG_KIND_MOUNTED") || !strings.Contains(masterRead, mount.GetLabel()) {
		t.Fatalf("the positive control failed: the master's log has no line of the mount")
	}
	for _, p := range []*person{w.ana, w.caio} {
		for when, body := range map[string]string{"while the mount is hidden": hiding[p.name] + heard(p.name), "after the mount was revealed": revealed[p.name]} {
			for _, needle := range []string{mount.GetId(), mount.GetLabel(), "COMBAT_LOG_KIND_MOUNTED"} {
				if when == "after the mount was revealed" && needle != "COMBAT_LOG_KIND_MOUNTED" {
					continue // the revealed mount is on the table: its name and id are theirs; the line of the ride stays the master's
				}
				if strings.Contains(body, needle) {
					t.Errorf("%s reads %q %s", p.name, needle, when)
				}
			}
		}
	}
}

// The options of a player's character that stands next to a mount the master hid, or next to a mount that a hidden creature rides,
// never name the hidden one: not in TurnOptions.mount, and not in the refusal of GetMoveOptions for a rider of a hidden mount. The
// master's copy of each is the positive control: it lists the mount, or the rider's mount, with what the player is not told.
func TestMountOptionsNeverNameAHiddenCreature(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	e := w.encounter
	hidden, seen, rider := w.combatant(w.hiddenNPC), w.combatant(w.seenNPC), w.combatant(w.toren)
	for _, id := range []string{hidden.GetId(), seen.GetId()} {
		if _, err := w.pool.Exec(ctx, `UPDATE combatants SET size = 'large', side = 'party' WHERE id = $1`, id); err != nil {
			t.Fatalf("make the mount Large and of the party: %v", err)
		}
	}
	w.placeForced(w.master, e.GetId(), placement{hidden.GetId(), 14, 8}, placement{seen.GetId(), 12, 8}, placement{rider.GetId(), 13, 8})
	for _, c := range []*playv1.Combatant{hidden, seen} {
		must(w.master.combat.SetCombatantMount(ctx, rq(&playv1.SetCombatantMountRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(), Kind: "controlled"})))
	}
	mountOf := func(p *person) *rulesv1.MountOptions {
		return must(p.combat.GetTurnOptions(ctx, rq(&playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: rider.GetId()}))).GetOptions().GetMount()
	}
	listed := func(o *rulesv1.MountOptions, id string) *rulesv1.MountChoice {
		for _, m := range o.GetMounts() {
			if m.GetCombatantId() == id {
				return m
			}
		}
		return nil
	}
	moveRefusal := func(p *person) string {
		_, err := p.combat.GetMoveOptions(ctx, rq(&playv1.GetMoveOptionsRequest{CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: rider.GetId()}))
		if err == nil {
			t.Fatalf("%s reads the move options of a rider", p.name)
		}
		return err.Error()
	}

	// The mount is hidden, on its own: the master lists it, Caio does not, and the visible one is listed to both.
	if listed(mountOf(w.master), hidden.GetId()) == nil || listed(mountOf(w.master), seen.GetId()) == nil {
		t.Fatalf("the positive control failed: the master's options do not list both mounts: %v", mountOf(w.master))
	}
	if listed(mountOf(w.caio), seen.GetId()) == nil {
		t.Fatalf("the positive control failed: Caio's options do not list the visible mount: %v", mountOf(w.caio))
	}
	if m := listed(mountOf(w.caio), hidden.GetId()); m != nil {
		t.Errorf("Caio's options list the hidden mount: %v", m)
	}

	// A hidden creature rides the visible mount: the master lists the mount as taken, Caio's options do not list it at all.
	if _, err := w.pool.Exec(ctx, `UPDATE combatants SET mounted_on_id = $1, grid_col = 12, grid_row = 8 WHERE id = $2`, seen.GetId(), hidden.GetId()); err != nil {
		t.Fatalf("put the hidden creature on the mount: %v", err)
	}
	if m := listed(mountOf(w.master), seen.GetId()); m == nil || m.GetRefusal() != rulesv1.MountRefusal_MOUNT_REFUSAL_TAKEN {
		t.Fatalf("the positive control failed: the master's options list the mount as %v, want it taken", m)
	}
	if m := listed(mountOf(w.caio), seen.GetId()); m != nil {
		t.Errorf("Caio's options list the mount a hidden creature rides: %v", m)
	}

	// Caio's character rides the hidden mount: his options say nothing of it and his move options are refused with no name.
	if _, err := w.pool.Exec(ctx, `UPDATE combatants SET mounted_on_id = NULL WHERE id = $1`, hidden.GetId()); err != nil {
		t.Fatalf("take the hidden creature off the mount: %v", err)
	}
	if _, err := w.pool.Exec(ctx, `UPDATE combatants SET mounted_on_id = $1, grid_col = 14, grid_row = 8 WHERE id = $2`, hidden.GetId(), rider.GetId()); err != nil {
		t.Fatalf("put Caio's character on the hidden mount: %v", err)
	}
	if got := mountOf(w.master).GetMountedOnId(); got != hidden.GetId() {
		t.Fatalf("the positive control failed: the master's options name %q as the rider's mount, want the hidden one", got)
	}
	if got := moveRefusal(w.master); !strings.Contains(got, "rider moves with its mount") {
		t.Fatalf("the positive control failed: the master's move options for a rider say %q", got)
	}
	if got := mountOf(w.caio); got.GetMountedOnId() != "" || len(got.GetLanding()) != 0 {
		t.Errorf("Caio's options of a rider on a hidden mount = %v, want nothing of it", got)
	}
	if got := moveRefusal(w.caio); strings.Contains(got, "rider") || strings.Contains(got, hidden.GetId()) || strings.Contains(got, hidden.GetLabel()) {
		t.Errorf("Caio's move options of a rider on a hidden mount say %q, want a refusal that names nothing", got)
	}
}
