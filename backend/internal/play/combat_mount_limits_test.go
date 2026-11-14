package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A controlled mount that carries a rider takes only Dash, Disengage and Dodge (SRD 5.1, "Mounted Combat"): it acts as its rider directs.
// Every other action is refused with MOUNT_ACTION_LIMITED, its reaction stays free, and the options of the mount show why.

var reasonMountActionLimited = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MOUNT_ACTION_LIMITED

// ridden is the cave's fight with Toren on the controlled Ogro, in Toren's turn, and Goblin 1 next to them.
func (c *cave) ridden(t *testing.T) {
	t.Helper()
	c.fight(t)
	c.steed(t)
	c.mustMove(t, c.master, "Goblin 1", 8, 7)
	c.mustMount(t, c.caio, "Toren", "Ogro")
}

// limitedCalls are the actions of the Ogro that its rider's mount refuses, by entry point, as the user makes them.
func (c *cave) limitedCalls() map[string]func(t *testing.T, u *user) error {
	enc := func(t *testing.T) string {
		t.Helper()
		return c.get(t, c.master).GetId()
	}
	return map[string]func(t *testing.T, u *user) error{
		"TakeAction search": func(t *testing.T, u *user) error {
			t.Helper()
			_, err := c.action(t, u, c.get(t, c.master), "Ogro", "standard:search")
			return err
		},
		"TakeAction ready": func(t *testing.T, u *user) error {
			t.Helper()
			_, err := c.action(t, u, c.get(t, c.master), "Ogro", "standard:ready")
			return err
		},
		"TakeAction use an object": func(t *testing.T, u *user) error {
			t.Helper()
			_, err := c.action(t, u, c.get(t, c.master), "Ogro", "standard:use-an-object")
			return err
		},
		"TakeAction help": func(t *testing.T, u *user) error {
			t.Helper()
			_, err := c.action(t, u, c.get(t, c.master), "Ogro", "standard:help")
			return err
		},
		"RollAttack": func(t *testing.T, u *user) error {
			t.Helper()
			_, err := c.attack(t, u, c.get(t, c.master), "Ogro", "basic:0", "Goblin 1", d20(15))
			return err
		},
		"StartContest grapple": func(t *testing.T, u *user) error {
			t.Helper()
			_, err := u.contests.StartContest(t.Context(), connect.NewRequest(&playv1.StartContestRequest{
				CampaignId: c.campaignID, EncounterId: enc(t), IdempotencyKey: newKey(), Roll: rollIn(),
				InitiatorId: c.id(t, "Ogro"), TargetId: c.id(t, "Goblin 1"), Purpose: playv1.ContestPurpose_CONTEST_PURPOSE_GRAPPLE,
			}))
			return err
		},
		"StartContest shove": func(t *testing.T, u *user) error {
			t.Helper()
			_, err := u.contests.StartContest(t.Context(), connect.NewRequest(&playv1.StartContestRequest{
				CampaignId: c.campaignID, EncounterId: enc(t), IdempotencyKey: newKey(), Roll: rollIn(),
				InitiatorId: c.id(t, "Ogro"), TargetId: c.id(t, "Goblin 1"), Purpose: playv1.ContestPurpose_CONTEST_PURPOSE_SHOVE,
			}))
			return err
		},
		"Help": func(t *testing.T, u *user) error {
			t.Helper()
			_, err := u.contests.Help(t.Context(), connect.NewRequest(&playv1.HelpRequest{
				CampaignId: c.campaignID, EncounterId: enc(t), IdempotencyKey: newKey(), CombatantId: c.id(t, "Ogro"),
				Kind: playv1.HelpKind_HELP_KIND_ATTACK, AllyId: c.id(t, "Toren"), TargetId: c.id(t, "Goblin 1"),
			}))
			return err
		},
		"Hide": func(t *testing.T, u *user) error {
			t.Helper()
			_, err := c.action(t, u, c.get(t, c.master), "Ogro", actionHide)
			return err
		},
	}
}

// TestMountedCombat_AControlledMountTakesOnlyDashDisengageAndDodge: the rider's player acts for the Ogro, which carries Toren, and the master
// too: the three actions work and the rest of the entry points answer MOUNT_ACTION_LIMITED, naming the mount.
func TestMountedCombat_AControlledMountTakesOnlyDashDisengageAndDodge(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"standard:dash", "standard:disengage", "standard:dodge"} {
		for who, user := range map[string]func(c *cave) *user{"the rider's player": func(c *cave) *user { return c.caio }, "the master": func(c *cave) *user { return c.master }} {
			t.Run(key+" by "+who, func(t *testing.T) {
				t.Parallel()
				c := newCave(t)
				c.ridden(t)
				if _, err := c.action(t, user(c), c.get(t, c.master), "Ogro", key); err != nil {
					t.Errorf("the mount's %s error = %v, want it taken", key, err)
				}
			})
		}
	}
	c := newCave(t)
	c.ridden(t)
	for name, call := range c.limitedCalls() {
		for who, u := range map[string]*user{"the rider's player": c.caio, "the master": c.master} {
			err := call(t, u)
			if name == "RollAttack" && u == c.caio {
				// The master answers for the mount's attacks (its reaction attacks too): its rider's player is not the mount's.
				wantCode(t, name+" by "+who, err, connect.CodePermissionDenied)
				continue
			}
			b := wantBlockedBy(t, name+" by "+who, err, reasonMountActionLimited)
			if ids := b.GetCombatantIds(); len(ids) != 1 || ids[0] != c.id(t, "Ogro") {
				t.Errorf("%s by %s names %v, want the Ogro", name, who, ids)
			}
		}
	}
	if ogre := c.joinRowOf(t, "Ogro"); ogre.ActionUsed {
		t.Error("a refused action spent the Ogro's action")
	}
}

// TestMountedCombat_AMountThatCarriesNobodyOrIsIndependentActsAsAnyCreature: without a rider the controlled Ogro attacks; an independent one
// carrying Toren attacks too, on its own turn.
func TestMountedCombat_AMountThatCarriesNobodyOrIsIndependentActsAsAnyCreature(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		kind  string
		mount bool
	}{"a controlled mount with no rider": {"controlled", false}, "an independent mount with a rider": {"independent", true}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newCave(t)
			c.fight(t)
			c.steed(t)
			c.mustMark(t, "Ogro", tc.kind)
			c.mustMove(t, c.master, "Goblin 1", 8, 7)
			if tc.mount {
				c.mustMount(t, c.caio, "Toren", "Ogro")
			}
			e := c.passTo(t, c.get(t, c.master), "Ogro")
			if _, err := c.attack(t, c.master, e, "Ogro", "basic:0", "Goblin 1", d20(15)); err != nil {
				t.Errorf("the Ogro's attack error = %v, want it made", err)
			}
		})
	}
}

// TestMountedCombat_AMountKeepsItsReaction: the Ogro carries Toren and is not limited in its reaction: the Goblin leaves its reach on its own
// turn and the Ogro's opportunity attack is made.
func TestMountedCombat_AMountKeepsItsReaction(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.ridden(t)
	c.passTo(t, c.get(t, c.master), "Goblin 1")
	c.moveOffering(t, "Goblin 1", 11, 7)
	var offerID string
	for _, o := range c.offersOf(t, c.master) {
		if o.GetReactorId() == c.id(t, "Ogro") {
			offerID = o.GetId()
		}
	}
	if offerID == "" {
		t.Fatalf("the Goblin left the Ogro's reach and the Ogro has no offer: %v", c.offersOf(t, c.master))
	}
	if _, err := c.offerAttack(t, c.master, "Ogro", "basic:0", "Goblin 1", offerID, d20(15)); err != nil {
		t.Errorf("the mount's opportunity attack error = %v, want it made: a reaction is not limited", err)
	}
}

// TestMountedCombat_TheMountsOptionsShowOnlyTheThreeActions: GetTurnOptions of the Ogro that carries Toren enables Dash, Disengage and Dodge and
// disables every other row with the reason in words (the master's and the owner's copy); the rider's player, who is not the Ogro's owner, reads
// the three actions and nothing of the Ogro's attacks and spells. A hidden mount's options are not found for a player.
func TestMountedCombat_TheMountsOptionsShowOnlyTheThreeActions(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.ridden(t)
	allowed := map[string]bool{"standard:dash": true, "standard:disengage": true, "standard:dodge": true}

	check := func(who string, u *user, listsTheRest bool) {
		t.Helper()
		res := c.mustOptions(t, u, c.get(t, c.master), "Ogro")
		o := res.GetOptions()
		rows := append(append([]*rulesv1.ActionOption{}, o.GetStandardActions()...), o.GetFeatureActions()...)
		seen := map[string]bool{}
		for _, a := range rows {
			key := a.GetAction().GetKey()
			seen[key] = true
			switch {
			case allowed[key] && !a.GetEnabled():
				t.Errorf("%s: %s is disabled (%v), want it enabled", who, key, a.GetReason())
			case !allowed[key] && a.GetEnabled():
				t.Errorf("%s: %s is enabled on a mount that carries a rider", who, key)
			case !allowed[key] && a.GetReason().GetTextPt() == "":
				t.Errorf("%s: %s is disabled with no words to say why", who, key)
			}
		}
		for key := range allowed {
			if !seen[key] {
				t.Errorf("%s: %s is not listed", who, key)
			}
		}
		for _, a := range o.GetAttacks() {
			if a.GetEnabled() || !listsTheRest {
				t.Errorf("%s: the attack %s is enabled or listed (%v)", who, a.GetAttack().GetKey(), a.GetEnabled())
			} else if a.GetReason().GetTextPt() == "" {
				t.Errorf("%s: the attack %s is disabled with no words to say why", who, a.GetAttack().GetKey())
			}
		}
		for _, co := range res.GetContestAttackOptions() {
			if co.GetEnabled() {
				t.Errorf("%s: the %v contest is enabled on a mount that carries a rider", who, co.GetKind())
			}
		}
		if !listsTheRest && (len(o.GetAttacks()) != 0 || len(o.GetSpells()) != 0 || len(res.GetAttackTargets()) != 0 || len(seen) != len(allowed)) {
			t.Errorf("%s: reads %d attacks, %d spells, %d attack targets and %d actions, want only the three actions", who,
				len(o.GetAttacks()), len(o.GetSpells()), len(res.GetAttackTargets()), len(seen))
		}
	}
	check("the master", c.master, true)
	check("the rider's player", c.caio, false)
}
