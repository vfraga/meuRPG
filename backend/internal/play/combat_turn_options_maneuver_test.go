package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// GetTurnOptions lists the weapon-attack table dice of a player's character (maneuver_dice), with the uses left of
// the pool. Pensantus has a d8 "Golpe Preciso" of attack, and a check, a save and an initiative die that are not listed.

// attackDice reads the maneuver dice of a combatant's turn options as u.
func (g *rollDiceTable) attackDice(t *testing.T, u *user, e *playv1.Encounter, label string) []*playv1.ManeuverDieOption {
	t.Helper()
	return g.mustOptions(t, u, e, label).GetManeuverDice()
}

func TestTheTurnOptionsListTheAttackDieWithTheUsesLeft(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	dice := g.attackDice(t, g.ana, e, "Pensantus")
	if len(dice) != 1 {
		t.Fatalf("maneuver dice = %v, want the attack die alone (a check, save or initiative die is not listed)", dice)
	}
	d := dice[0]
	if d.GetDie().GetKey() != g.strikeKey || d.GetDie().GetApplies() != "attack" || d.GetDie().GetSides() != 8 ||
		d.GetDie().GetNamePt() != "Golpe Preciso" || d.GetDie().GetResource() != rollDiceUses {
		t.Errorf("die = %v, want Golpe Preciso, a d8 of attack from %s", d.GetDie(), rollDiceUses)
	}
	if want := g.usesLeft(t); d.GetUsesLeft() != want || want != 2 {
		t.Errorf("uses_left = %d, want the pool's %d (2)", d.GetUsesLeft(), want)
	}
	if d.GetResourceNamePt() != "Reserva de Olhar" {
		t.Errorf("resource_name_pt = %q, want the pool's name", d.GetResourceNamePt())
	}
}

func TestTheAttackDieStaysListedAtZeroUses(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	if _, err := g.strike(t, e, g.strikeKey, 6, 4); err != nil {
		t.Fatalf("RollAttack(with the die) error = %v", err)
	}
	if dice := g.attackDice(t, g.ana, e, "Pensantus"); len(dice) != 1 || dice[0].GetUsesLeft() != 1 {
		t.Fatalf("maneuver dice after one use = %v, want the die with 1 use left", dice)
	}
	if _, err := g.strike(t, e, g.strikeKey, 2, 1); err != nil {
		t.Fatalf("RollAttack(with the die) error = %v", err)
	}
	if dice := g.attackDice(t, g.ana, e, "Pensantus"); len(dice) != 1 || dice[0].GetUsesLeft() != 0 {
		t.Errorf("maneuver dice with the pool spent = %v, want the die listed with 0 uses left", dice)
	}
}

func TestTheMasterReadsThePlayersAttackDieToo(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	dice := g.attackDice(t, g.master, e, "Pensantus")
	if len(dice) != 1 || dice[0].GetDie().GetKey() != g.strikeKey || dice[0].GetUsesLeft() != 2 {
		t.Errorf("the master's maneuver dice = %v, want the attack die with 2 uses", dice)
	}
}

func TestAnNPCAndACharacterWithNoManeuverGetNoAttackDie(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.castersFightNPCFirst(t)
	if dice := g.attackDice(t, g.master, e, "Capitão Goblin"); len(dice) != 0 {
		t.Errorf("an NPC's maneuver dice = %v, want none", dice)
	}
	if dice := g.attackDice(t, g.caio, e, "Toren"); len(dice) != 0 {
		t.Errorf("a character with no maneuver has the dice %v, want none", dice)
	}
}
