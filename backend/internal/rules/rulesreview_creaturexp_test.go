package rules

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/encounter"
)

func TestRulesReviewProgression_CreatureXPFollowsChallengeRating(t *testing.T) {
	c := loadForTest(t)
	list, err := c.ListCreatures(CreatureFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("no creatures")
	}
	for _, e := range list {
		want, ok := c.XPForChallenge(e.ChallengeRating)
		if !ok {
			t.Errorf("%s: CR %q not in the XP table", e.Key, e.ChallengeRating)
			continue
		}
		if e.XP == want || (e.ChallengeRating == "0" && e.XP == 0) {
			continue
		}
		t.Errorf("%s: CR %s app XP %d, table XP %d", e.Key, e.ChallengeRating, e.XP, want)
	}
}

func TestRulesReviewProgression_EncounterUsesWrongCreatureXP(t *testing.T) {
	c := loadForTest(t)
	ev, err := c.EvaluateEncounter([]int{1}, []encounter.Entry{{Key: "monster:brass-dragon-wyrmling", Count: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if ev.TotalXP != 200 {
		t.Errorf("brass-dragon-wyrmling (CR 1) TotalXP = %d, want 200", ev.TotalXP)
	}
}
