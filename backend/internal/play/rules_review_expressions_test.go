package play

import (
	"testing"

	"connectrpc.com/connect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// TestRulesReviewExpressions_EldritchBlastBeams: SRD 5.1 Eldritch Blast makes
// two beams at 5th level, each with its own attack roll. A level 5 warlock
// must be able to make 2 eldritch blast attacks with the one Cast a Spell action.
func TestRulesReviewExpressions_EldritchBlastBeams(t *testing.T) {
	t.Parallel()
	const blast = "spell:eldritch-blast"
	a := newArmedWith(t, func(a *armed) {
		sc := func(str, dex, con, intl int32) *rulesv1.AbilityScores {
			return &rulesv1.AbilityScores{Strength: str, Dexterity: dex, Constitution: con, Intelligence: intl, Wisdom: 10, Charisma: 16}
		}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, sc(16, 13, 14, 10), []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:warlock", "race:human", 5, sc(10, 14, 12, 10), nil, []string{blast})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, sc(10, 16, 14, 10), []string{rapier}, nil)
	})
	e := a.threeAndAGoblin(t)
	a.mustEndTurn(t, a.caio, e) // Pensantus's turn
	e = a.get(t, a.ana)
	if _, err := a.attack(t, a.ana, e, "Pensantus", blast, "Goblin", inAppRoll); err != nil {
		t.Fatalf("first beam: RollAttack error = %v", err)
	}
	_, err := a.attack(t, a.ana, e, "Pensantus", blast, "Goblin", inAppRoll)
	if err != nil {
		t.Errorf("level 5 warlock, second Eldritch Blast beam: app refuses (%v, code %v); SRD: 2 beams at 5th level, a separate attack roll for each", err, connect.CodeOf(err))
	}
}
