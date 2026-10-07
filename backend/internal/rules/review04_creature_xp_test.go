package rules

import "testing"

// Finding U4-13: four creatures carry an xp that disagrees with the standard
// challenge-rating table (riding-horse and dretch CR 1/4 have 25, table 50;
// brass-dragon-wyrmling CR 1 has 100, table 200; deep-gnome-svirfneblin CR 1/2
// has 50, table 100). The values were copied unchanged from the pinned upstream
// 5e-database, and nothing overrides them, so encounter XP totals and the XP
// awarded for defeated monsters are understated for these creatures.
func TestReview4_CreatureXPMatchesChallengeRatingTable(t *testing.T) {
	c := loadForTest(t)
	table := map[string]int{"0": 10, "1/8": 25, "1/4": 50, "1/2": 100, "1": 200, "2": 450, "3": 700, "4": 1100, "5": 1800, "6": 2300, "7": 2900,
		"8": 3900, "9": 5000, "10": 5900, "11": 7200, "12": 8400, "13": 10000, "14": 11500, "15": 13000, "16": 15000, "17": 18000, "18": 20000,
		"19": 22000, "20": 25000, "21": 33000, "22": 41000, "23": 50000, "24": 62000, "25": 75000, "26": 90000, "27": 105000, "28": 120000,
		"29": 135000, "30": 155000}
	list, _ := c.ListCreatures(CreatureFilter{})
	for _, e := range list {
		want, ok := table[e.ChallengeRating]
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
