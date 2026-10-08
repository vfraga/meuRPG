package rules

import "testing"

// Every creature gives the XP of its challenge rating (RN-09, encounter budgets), as the SRD's
// table in effects/advancement.json says: a value copied from upstream that disagrees with it
// understates the encounter. cmd/srdimport keeps its own copy of the table, and this checks both.
func TestCreatureXPMatchesChallengeRatingTable(t *testing.T) {
	c := loadForTest(t)
	list, _ := c.ListCreatures(CreatureFilter{})
	for _, e := range list {
		want, ok := c.XPForChallenge(e.ChallengeRating)
		if !ok {
			t.Errorf("%s: unknown challenge rating %q", e.Key, e.ChallengeRating)
			continue
		}
		// A few CR 0 creatures (frog, sea horse) legitimately give 0.
		if e.ChallengeRating == "0" && e.XP == 0 {
			continue
		}
		if e.XP != want {
			t.Errorf("%s: challenge rating %s gives %d XP, the table says %d", e.Key, e.ChallengeRating, e.XP, want)
		}
	}
}
