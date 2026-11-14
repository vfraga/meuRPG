package play

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The initiative of a pair (SRD 5.1, "Mounted Combat"): a controlled mount acts as its rider directs, so while it carries one it takes the
// rider's initiative total and sits right after the rider in the order, which makes the two one joint group; an independent mount keeps its
// place. The cave's order is Toren 19, Pensantus 12, ... and the Ogro, Toren's steed, at the end with the lowest total.

// joinRow is what the table keeps of a combatant's place in the order and of its turn.
type joinRow struct {
	Initiative, Face *int32
	Bonus            int32
	Tie              bool
	Index            int32
	State            string
	Prior            string
	ActionUsed       bool
	BonusUsed        bool
	ReactionUsed     bool
	MovementDft      int32
}

// String is the row with its numbers, not the addresses of the optional ones.
func (r joinRow) String() string {
	return fmt.Sprintf("{init %d face %d bonus %d tie %v index %d state %s prior %q action %v bonus %v reaction %v moved %d}",
		optInt(r.Initiative), optInt(r.Face), r.Bonus, r.Tie, r.Index, r.State, r.Prior, r.ActionUsed, r.BonusUsed, r.ReactionUsed, r.MovementDft)
}

func (c *cave) joinRowOf(t *testing.T, label string) joinRow {
	t.Helper()
	var r joinRow
	err := c.h.pool.QueryRow(t.Context(),
		`SELECT initiative, initiative_face, initiative_bonus, tie_ordered, order_index, turn_state, COALESCE(mount_prior_init::text, ''),
		        action_used, bonus_action_used, reaction_used, movement_used_dft
		   FROM combatants WHERE id = $1`, c.id(t, label),
	).Scan(&r.Initiative, &r.Face, &r.Bonus, &r.Tie, &r.Index, &r.State, &r.Prior, &r.ActionUsed, &r.BonusUsed, &r.ReactionUsed, &r.MovementDft)
	if err != nil {
		t.Fatalf("read the order columns of %s: %v", label, err)
	}
	return r
}

// priorOf is the mount_prior_init of a combatant, decoded: nil when the column is empty.
func (c *cave) priorOf(t *testing.T, label string) *struct {
	Initiative *int32 `json:"initiative"`
	Face       *int32 `json:"face"`
	Tie        bool   `json:"tie"`
} {
	t.Helper()
	raw := c.joinRowOf(t, label).Prior
	if raw == "" {
		return nil
	}
	var p struct {
		Initiative *int32 `json:"initiative"`
		Face       *int32 `json:"face"`
		Tie        bool   `json:"tie"`
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("mount_prior_init of %s = %q: %v", label, raw, err)
	}
	return &p
}

// orderNow is the master's order, by label.
func (c *cave) orderNow(t *testing.T) []string {
	t.Helper()
	return labels(c.get(t, c.master))
}

// orderWith is base with the mount taken out and put right after the rider.
func orderWith(base []string, rider, mount string) []string {
	out := slices.DeleteFunc(slices.Clone(base), func(l string) bool { return l == mount })
	return slices.Insert(out, slices.Index(out, rider)+1, mount)
}

// pairState is everything the table keeps of the order and the turn of the pair and of Pensantus, as one string to compare.
func (c *cave) pairState(t *testing.T) string {
	t.Helper()
	out := []string{strings.Join(c.orderNow(t), ",")}
	for _, l := range []string{"Toren", "Ogro", "Pensantus", "Escudeiro"} {
		out = append(out, fmt.Sprintf("%s=%v", l, c.joinRowOf(t, l)))
	}
	return strings.Join(out, "\n")
}

// spent marks the Ogro's turn as used: its action, bonus action and reaction and 5 ft of walking.
func (c *cave) spendTheOgre(t *testing.T) {
	t.Helper()
	c.execSQL(t, `UPDATE combatants SET action_used = true, bonus_action_used = true, reaction_used = true, movement_used_dft = 50 WHERE id = $1`, c.id(t, "Ogro"))
}

// freeTheRide gives Toren's mount-or-dismount of the move back, for a test that gets on and off in one turn.
func (c *cave) freeTheRide(t *testing.T, rider string) {
	t.Helper()
	c.execSQL(t, `UPDATE combatants SET mount_move_used = false WHERE id = $1`, c.id(t, rider))
}

// rolledFight is the cave's fight with the NPCs' d20 and the players' faces given: npcRolls in the order Capitão, Goblin 1-3, Ogro, Escudeiro.
func (c *cave) rolledFight(t *testing.T, npcRolls []int, players map[string]int32) *playv1.Encounter {
	t.Helper()
	return c.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: c.capitao.GetId()}, {CharacterId: c.goblins.GetId(), Count: 3}, {CharacterId: c.ogre.GetId()}, {CharacterId: c.squire.GetId()},
		},
		npcRolls: npcRolls,
		players:  players,
		reveal:   []string{"Capitão Goblin", "Goblin 1", "Goblin 2", "Goblin 3", "Ogro", "Escudeiro"},
		at: map[string][2]int32{
			"Toren": {6, 7}, "Pensantus": {5, 8}, "Brisa": {4, 7}, "Escudeiro": {3, 8},
			"Goblin 1": {18, 5}, "Goblin 2": {20, 7}, "Goblin 3": {21, 3}, "Capitão Goblin": {12, 13}, "Ogro": {22, 9},
		},
	})
}

// ogreFirstFight is the cave's fight with the Ogro rolling 20 and Toren low: the order is the Ogro, Pensantus, Toren, ...: the Ogro has its
// turn before Toren's.
func (c *cave) ogreFirstFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	return c.rolledFight(t, []int{2, 2, 2, 2, 20, 2}, map[string]int32{"Toren": 5, "Pensantus": 10, "Brisa": 1})
}

// TestMountedCombat_AControlledMountTakesItsRidersInitiativeAndSitsRightAfterIt: Toren gets on the Ogro, the lowest in the order, mid-turn.
// The Ogro takes his total, face and bonus and sits right after him, its own total and face kept for the dismount, and its turn is not
// over: it takes his turn's part, with its economy fresh. They are one joint group. When Toren gets off, the Ogro is back at the end
// with what it had, and out of the turn.
func TestMountedCombat_AControlledMountTakesItsRidersInitiativeAndSitsRightAfterIt(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	base := c.orderNow(t)
	ogre, toren := c.joinRowOf(t, "Ogro"), c.joinRowOf(t, "Toren")
	c.spendTheOgre(t) // what a turn that is not here yet leaves on it: the part it takes is a fresh one

	c.mustMount(t, c.caio, "Toren", "Ogro")

	want := orderWith(base, "Toren", "Ogro")
	if got := c.orderNow(t); !slices.Equal(got, want) {
		t.Errorf("order after the mount = %v, want %v", got, want)
	}
	joined := c.joinRowOf(t, "Ogro")
	if *joined.Initiative != *toren.Initiative || *joined.Face != *toren.Face || joined.Bonus != toren.Bonus {
		t.Errorf("the Ogro's initiative = %d (face %d, bonus %d), want the rider's %d (face %d, bonus %d)",
			*joined.Initiative, *joined.Face, joined.Bonus, *toren.Initiative, *toren.Face, toren.Bonus)
	}
	prior := c.priorOf(t, "Ogro")
	if prior == nil || *prior.Initiative != *ogre.Initiative || *prior.Face != *ogre.Face || prior.Tie != ogre.Tie {
		t.Errorf("mount_prior_init = %+v, want the Ogro's own initiative %d, face %d, tie %v", prior, *ogre.Initiative, *ogre.Face, ogre.Tie)
	}
	if joined.State != "acting" || joined.ActionUsed || joined.BonusUsed || joined.MovementDft != 0 || !joined.ReactionUsed {
		t.Errorf("the Ogro's turn after it joined Toren's = %+v, want acting with a fresh action, bonus action and movement and the reaction it spent still spent", joined)
	}
	e := c.get(t, c.master)
	if got := group(e); got != "Toren,Ogro" {
		t.Errorf("the group on turn = %q, want Toren,Ogro", got)
	}
	if got := group(c.get(t, c.caio)); got != "Toren,Ogro" {
		t.Errorf("the group Toren's player sees = %q, want Toren,Ogro", got)
	}

	c.freeTheRide(t, "Toren")
	if _, err := c.dismountAs(t, c.caio, "Toren", 6, 7); err != nil {
		t.Fatalf("DismountCombatant() error = %v", err)
	}
	if got := c.orderNow(t); !slices.Equal(got, base) {
		t.Errorf("order after the dismount = %v, want the order before the mount %v", got, base)
	}
	back := c.joinRowOf(t, "Ogro")
	if *back.Initiative != *ogre.Initiative || *back.Face != *ogre.Face || back.Bonus != ogre.Bonus || back.Tie != ogre.Tie || back.Index != ogre.Index {
		t.Errorf("the Ogro after the dismount = %+v, want its own place back: %+v", back, ogre)
	}
	if back.Prior != "" {
		t.Errorf("mount_prior_init after the dismount = %q, want it cleared", back.Prior)
	}
	if back.State != "idle" {
		t.Errorf("the Ogro's turn after the dismount = %q, want idle: it is out of Toren's turn and has not its own yet", back.State)
	}
	if got := group(c.get(t, c.master)); got != "Toren" {
		t.Errorf("the group on turn after the dismount = %q, want Toren alone", got)
	}
}

// TestMountedCombat_AnIndependentMountKeepsItsPlace: the Ogro is an independent mount: Toren gets on and nothing of the order changes.
func TestMountedCombat_AnIndependentMountKeepsItsPlace(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMark(t, "Ogro", "independent")
	before := c.pairState(t)

	c.mustMount(t, c.caio, "Toren", "Ogro")

	after := c.pairState(t)
	// What the ride changes of Toren is the link and the movement: the order and the turn columns are the point here.
	beforeOrder, afterOrder := strings.SplitN(before, "\n", 2)[0], strings.SplitN(after, "\n", 2)[0]
	if beforeOrder != afterOrder {
		t.Errorf("order after mounting an independent mount = %s, want %s", afterOrder, beforeOrder)
	}
	if ogre := c.joinRowOf(t, "Ogro"); ogre.State != "idle" || ogre.Prior != "" {
		t.Errorf("the independent Ogro after the mount = %+v, want idle with no prior initiative", ogre)
	}
	if got := group(c.get(t, c.master)); got != "Toren" {
		t.Errorf("the group on turn = %q, want Toren alone: an independent mount has its own turn", got)
	}
}

// TestMountedCombat_AMountWhoseTurnPassedKeepsItsEconomySpent: the Ogro has the highest total and had its turn this round. Toren, whose turn
// is on now, gets on it: the Ogro joins Toren's part with the economy it had spent, no second turn, and the turn passes when both end.
func TestMountedCombat_AMountWhoseTurnPassedKeepsItsEconomySpent(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.ogreFirstFight(t)
	c.steed(t)
	base := c.orderNow(t)
	if i, j := slices.Index(base, "Ogro"), slices.Index(base, "Toren"); i != 0 || j != 2 {
		t.Fatalf("order = %v, want the Ogro first and Toren third", base)
	}
	c.spendTheOgre(t)
	ogre := c.joinRowOf(t, "Ogro")
	_ = c.passTo(t, c.get(t, c.master), "Toren") // the Ogro's turn and Pensantus's are over

	c.mustMount(t, c.caio, "Toren", "Ogro")

	if got, want := c.orderNow(t), orderWith(base, "Toren", "Ogro"); !slices.Equal(got, want) {
		t.Errorf("order after the mount = %v, want %v", got, want)
	}
	joined := c.joinRowOf(t, "Ogro")
	if joined.State != "acting" || !joined.ActionUsed || !joined.BonusUsed || !joined.ReactionUsed || joined.MovementDft != ogre.MovementDft {
		t.Errorf("the Ogro that joined after its turn = %+v, want acting with its action, bonus action and reaction still spent and 5 ft walked", joined)
	}
	if got := group(c.get(t, c.master)); got != "Toren,Ogro" {
		t.Fatalf("the group on turn = %q, want Toren,Ogro", got)
	}
	e := c.mustEndPart(t, c.caio, c.get(t, c.caio), "Toren")
	if group(e) != "Toren,Ogro" || current(e) != "Ogro" {
		t.Errorf("after Toren's part: group %q, current %q, want Toren,Ogro with the Ogro's part running", group(e), current(e))
	}
	e = c.mustEndPart(t, c.caio, e, "Ogro")
	order := c.orderNow(t)
	if next := order[slices.Index(order, "Ogro")+1]; current(e) != next {
		t.Errorf("after both parts the turn is %q's, want %s's, next in the order %v", current(e), next, order)
	}
}

// TestMountedCombat_TheMountIsInTheGroupAtTheStartOfTheNextTurn: Toren stays on the Ogro as the round goes by. When his turn starts again
// the pair is one group from the start, both with the turn's economy fresh, and the order is the one the mount made.
func TestMountedCombat_TheMountIsInTheGroupAtTheStartOfTheNextTurn(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	c.fight(t)
	c.steed(t)
	c.mustMount(t, c.caio, "Toren", "Ogro")
	c.execSQL(t, `UPDATE combatants SET action_used = true, movement_used_dft = 77 WHERE id = ANY($1)`, []string{c.id(t, "Toren"), c.id(t, "Ogro")})
	order := c.orderNow(t)

	e := c.passTo(t, c.get(t, c.master), "Pensantus")
	if group(e) != "Pensantus" {
		t.Fatalf("group on Pensantus's turn = %q", group(e))
	}
	if ogre := c.joinRowOf(t, "Ogro"); ogre.State != "idle" {
		t.Errorf("the Ogro's turn state while Pensantus plays = %q, want idle", ogre.State)
	}
	e = c.passTo(t, e, "Toren")
	if got := group(e); got != "Toren,Ogro" {
		t.Errorf("the group on Toren's next turn = %q, want Toren,Ogro", got)
	}
	for _, l := range []string{"Toren", "Ogro"} {
		if r := c.joinRowOf(t, l); r.State != "acting" || r.ActionUsed || r.MovementDft != 0 {
			t.Errorf("%s at the start of the turn = %+v, want acting with the economy reset", l, r)
		}
	}
	if got := c.orderNow(t); !slices.Equal(got, order) {
		t.Errorf("order a round later = %v, want %v", got, order)
	}
}

// TestMountedCombat_TheJoinIsUndone: undoing the mount gives back the Ogro's own initiative, the order, and the turn state and economy it
// had; undoing the dismount puts the pair together again as it was. The case of a mount whose turn passed and of one whose turn is to come.
func TestMountedCombat_TheJoinIsUndone(t *testing.T) {
	t.Parallel()
	for name, passed := range map[string]bool{"its turn is to come": false, "its turn passed": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newCave(t)
			if passed {
				c.ogreFirstFight(t)
			} else {
				c.fight(t)
			}
			c.steed(t)
			c.spendTheOgre(t)
			if passed {
				c.passTo(t, c.get(t, c.master), "Toren")
			}
			alone := c.pairState(t)

			c.mustMount(t, c.caio, "Toren", "Ogro")
			together := c.pairState(t)
			if together == alone {
				t.Fatal("the mount changed nothing of the order or the turn")
			}
			c.undoLast(t)
			if got := c.pairState(t); got != alone {
				t.Errorf("after the undo of the mount:\n%s\nwant what it was:\n%s", got, alone)
			}

			c.mustMount(t, c.caio, "Toren", "Ogro")
			c.freeTheRide(t, "Toren")
			if _, err := c.dismountAs(t, c.caio, "Toren", 6, 7); err != nil {
				t.Fatalf("DismountCombatant() error = %v", err)
			}
			c.undoLast(t)
			if got, want := strings.Join(c.orderNow(t), ","), strings.SplitN(together, "\n", 2)[0]; got != want {
				t.Errorf("order after the undo of the dismount = %s, want the pair's order of the mount %s", got, want)
			}
			if ogre := c.joinRowOf(t, "Ogro"); ogre.State != "acting" || ogre.Prior == "" {
				t.Errorf("the Ogro after the undo of the dismount = %+v, want it acting in the pair with its prior initiative kept", ogre)
			}
			if got := group(c.get(t, c.master)); got != "Toren,Ogro" {
				t.Errorf("the group after the undo of the dismount = %q, want Toren,Ogro", got)
			}
		})
	}
}
