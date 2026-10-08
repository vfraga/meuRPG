package rules

import (
	"slices"
	"testing"
)

// These tests check the imported creatures against the SRD 5.1 stat blocks.

func reviewCreature(t *testing.T, key string) Creature {
	t.Helper()
	c := loadForTest(t)
	cr, ok := c.CreatureByKey(key)
	if !ok {
		t.Fatalf("%s: creature not found", key)
	}
	return cr
}

func reviewAllCreatures(t *testing.T) []Creature {
	t.Helper()
	c := loadForTest(t)
	list, err := c.ListCreatures(CreatureFilter{})
	if err != nil {
		t.Fatalf("ListCreatures: %v", err)
	}
	var out []Creature
	for _, e := range list {
		cr, ok := c.CreatureByKey(e.Key)
		if !ok {
			t.Fatalf("%s: listed but not found", e.Key)
		}
		out = append(out, cr)
	}
	return out
}

func reviewSense(cr Creature, key string) int {
	for _, s := range cr.Senses {
		if s.Key == key {
			return s.RangeFt
		}
	}
	return 0
}

func TestRulesReviewCreatures_BasiliskAC(t *testing.T) {
	t.Parallel()
	cr := reviewCreature(t, "monster:basilisk")
	if cr.ArmorClass != 15 {
		t.Errorf("Basilisk armor class is %d in the app, but the SRD 5.1 says 15 (natural armor)", cr.ArmorClass)
	}
}

func TestRulesReviewCreatures_CultFanaticHP(t *testing.T) {
	t.Parallel()
	cr := reviewCreature(t, "monster:cult-fanatic")
	if cr.HitPoints != 33 || cr.HitPointsRoll != "6d8+6" {
		t.Errorf("Cult Fanatic hit points are %d (%s) in the app, but the SRD 5.1 says 33 (6d8 + 6)", cr.HitPoints, cr.HitPointsRoll)
	}
}

func TestRulesReviewCreatures_HunterSharkSenses(t *testing.T) {
	t.Parallel()
	cr := reviewCreature(t, "monster:hunter-shark")
	if got := reviewSense(cr, "blindsight"); got != 30 {
		t.Errorf("Hunter Shark has blindsight %d ft in the app, but the SRD 5.1 says blindsight 30 ft", got)
	}
	if got := reviewSense(cr, "darkvision"); got != 0 {
		t.Errorf("Hunter Shark has darkvision %d ft in the app, but the SRD 5.1 gives it no darkvision", got)
	}
}

func TestRulesReviewCreatures_CrocodileSwim(t *testing.T) {
	t.Parallel()
	cr := reviewCreature(t, "monster:crocodile")
	if cr.SpeedWalkFt != 20 || cr.SpeedSwimFt != 30 {
		t.Errorf("Crocodile speed is walk %d, swim %d in the app, but the SRD 5.1 says walk 20, swim 30", cr.SpeedWalkFt, cr.SpeedSwimFt)
	}
}

func TestRulesReviewCreatures_GiantWaspSwim(t *testing.T) {
	t.Parallel()
	cr := reviewCreature(t, "monster:giant-wasp")
	if cr.SpeedSwimFt != 0 || cr.SpeedFlyFt != 50 || cr.SpeedWalkFt != 10 {
		t.Errorf("Giant Wasp speed is walk %d, fly %d, swim %d in the app, but the SRD 5.1 says walk 10, fly 50 and no swim speed", cr.SpeedWalkFt, cr.SpeedFlyFt, cr.SpeedSwimFt)
	}
}

func TestRulesReviewCreatures_AdultBrassDragonBurrow(t *testing.T) {
	t.Parallel()
	cr := reviewCreature(t, "monster:adult-brass-dragon")
	if cr.SpeedWalkFt != 40 || cr.SpeedBurrowFt != 30 || cr.SpeedFlyFt != 80 {
		t.Errorf("Adult Brass Dragon speed is walk %d, burrow %d, fly %d in the app, but the SRD 5.1 says walk 40, burrow 30, fly 80", cr.SpeedWalkFt, cr.SpeedBurrowFt, cr.SpeedFlyFt)
	}
}

func TestRulesReviewCreatures_ConditionImmunities(t *testing.T) {
	t.Parallel()
	want := map[string][]string{
		"monster:black-pudding":     {"blinded", "charmed", "deafened", "exhaustion", "frightened", "prone"},
		"monster:flying-sword":      {"blinded", "charmed", "deafened", "frightened", "paralyzed", "petrified", "poisoned"},
		"monster:rug-of-smothering": {"blinded", "charmed", "deafened", "frightened", "paralyzed", "petrified", "poisoned"},
		"monster:ochre-jelly":       {"blinded", "charmed", "deafened", "exhaustion", "frightened", "prone"},
		"monster:shambling-mound":   {"blinded", "deafened", "exhaustion"},
		"monster:shrieker":          {"blinded", "deafened", "frightened"},
		"monster:violet-fungus":     {"blinded", "deafened", "frightened"},
	}
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		cr := reviewCreature(t, k)
		var got []string
		for _, ci := range cr.ConditionImmunities {
			got = append(got, ci.Key[len("condition:"):])
		}
		g := slices.Clone(got)
		slices.Sort(g)
		w := slices.Clone(want[k])
		slices.Sort(w)
		if !slices.Equal(g, w) {
			t.Errorf("%s condition immunities are %v in the app, but the SRD 5.1 says %v", cr.Name, got, want[k])
		}
	}
}

func TestRulesReviewCreatures_IceDevilImmunities(t *testing.T) {
	t.Parallel()
	cr := reviewCreature(t, "monster:ice-devil")
	var got []string
	for _, m := range cr.Immunities {
		for _, ty := range m.Types {
			got = append(got, ty.Key)
		}
	}
	slices.Sort(got)
	want := []string{"damage-type:cold", "damage-type:fire", "damage-type:poison"}
	if !slices.Equal(got, want) {
		t.Errorf("Ice Devil damage immunities are %v in the app, but the SRD 5.1 says cold, fire, poison", got)
	}
}

var reviewXPByCR = map[string]int{
	"0": 0, "1/8": 25, "1/4": 50, "1/2": 100, "1": 200, "2": 450, "3": 700, "4": 1100, "5": 1800,
	"6": 2300, "7": 2900, "8": 3900, "9": 5000, "10": 5900, "11": 7200, "12": 8400, "13": 10000,
	"14": 11500, "15": 13000, "16": 15000, "17": 18000, "18": 20000, "19": 22000, "20": 25000,
	"21": 33000, "22": 41000, "23": 50000, "24": 62000, "25": 75000, "26": 90000, "27": 105000,
	"28": 120000, "29": 135000, "30": 155000,
}

// TestRulesReviewCreatures_PassivePerception: passive Perception is 10 plus
// the Perception bonus (the listed skill, or else the Wisdom modifier).
func TestRulesReviewCreatures_PassivePerception(t *testing.T) {
	t.Parallel()
	for _, cr := range reviewAllCreatures(t) {
		// The SRD's Spider says passive Perception 12 with Wisdom 10 and no
		// Perception skill (the other transcription says 10); left out as unverified.
		if cr.Key == "monster:spider" {
			continue
		}
		bonus, listed := 0, false
		for _, s := range cr.Skills {
			if s.Key == "skill:perception" {
				bonus, listed = s.Bonus, true
			}
		}
		if !listed {
			for _, a := range cr.Abilities {
				if a.Ability == WIS {
					bonus = a.Modifier
				}
			}
		}
		if want := 10 + bonus; cr.PassivePerception != want {
			t.Errorf("%s (%s): passive Perception is %d in the app, but 10 + its Perception bonus (%+d, skill listed: %v) is %d", cr.Name, cr.Key, cr.PassivePerception, bonus, listed, want)
		}
	}
}

func TestRulesReviewCreatures_PerceptionSkillsListed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key, skill string
		bonus      int
	}{
		{"monster:black-bear", "skill:perception", 3},
		{"monster:half-red-dragon-veteran", "skill:athletics", 5},
		{"monster:half-red-dragon-veteran", "skill:perception", 2},
		{"monster:swarm-of-ravens", "skill:perception", 5},
	}
	for _, tt := range tests {
		cr := reviewCreature(t, tt.key)
		got, found := 0, false
		for _, s := range cr.Skills {
			if s.Key == tt.skill {
				got, found = s.Bonus, true
			}
		}
		if !found || got != tt.bonus {
			t.Errorf("%s: skill %s is %d (listed: %v) in the app, but the SRD 5.1 says %+d", cr.Name, tt.skill, got, found, tt.bonus)
		}
	}
}

func TestRulesReviewCreatures_BlinkDogPassive(t *testing.T) {
	t.Parallel()
	cr := reviewCreature(t, "monster:blink-dog")
	if cr.PassivePerception != 13 {
		t.Errorf("Blink Dog passive Perception is %d in the app, but the SRD 5.1 says 13 (Perception +3)", cr.PassivePerception)
	}
}
