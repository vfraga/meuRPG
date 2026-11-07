package rules

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The expected options are written out by hand from the SRD 5.1 stat blocks
// ("Name:cost:kind"), so a wrong classification shows as a diff.
var dragonOptions = []string{"Detect:1:text", "Tail Attack:1:attack", "Wing Attack:2:text"}

var legendaryGolden = map[string]struct {
	resistance int
	options    []string
}{
	"monster:aboleth":         {0, []string{"Detect:1:text", "Tail Swipe:1:attack", "Psychic Drain:2:text"}},
	"monster:androsphinx":     {0, []string{"Claw Attack:1:attack", "Teleport:2:text", "Cast a Spell:3:text"}},
	"monster:gynosphinx":      {0, []string{"Claw Attack:1:attack", "Teleport:2:text", "Cast a Spell:3:text"}},
	"monster:kraken":          {0, []string{"Tentacle Attack or Fling:1:text", "Lightning Storm:2:text", "Ink Cloud:3:text"}},
	"monster:lich":            {3, []string{"Cantrip:1:text", "Paralyzing Touch:2:attack", "Frightening Gaze:2:text", "Disrupt Life:3:text"}},
	"monster:mummy-lord":      {0, []string{"Attack:1:text", "Blinding Dust:1:text", "Blasphemous Word:2:text", "Channel Negative Energy:2:text", "Whirlwind of Sand:2:text"}},
	"monster:solar":           {0, []string{"Teleport:1:text", "Searing Burst:2:text", "Blinding Gaze:3:text"}},
	"monster:tarrasque":       {3, []string{"Attack:1:text", "Move:1:move", "Chomp:2:text"}},
	"monster:unicorn":         {0, []string{"Hooves:1:attack", "Shimmering Shield:2:text", "Heal Self:3:text"}},
	"monster:vampire-bat":     {3, []string{"Move:1:move", "Unarmed Strike:1:text", "Bite:2:attack"}},
	"monster:vampire-mist":    {3, []string{"Move:1:move", "Unarmed Strike:1:text", "Bite:2:text"}},
	"monster:vampire-vampire": {3, []string{"Move:1:move", "Unarmed Strike:1:attack", "Bite:2:attack"}},
}

func init() {
	for _, color := range []string{"black", "blue", "brass", "bronze", "copper", "gold", "green", "red", "silver", "white"} {
		for _, age := range []string{"adult", "ancient"} {
			legendaryGolden["monster:"+age+"-"+color+"-dragon"] = struct {
				resistance int
				options    []string
			}{3, dragonOptions}
		}
	}
}

var reactionGolden = map[string]string{
	"monster:bandit-captain":  "Parry:parry:2",
	"monster:black-pudding":   "Split:text:0",
	"monster:chain-devil":     "Unnerving Mask:text:0",
	"monster:erinyes":         "Parry:parry:4",
	"monster:gladiator":       "Parry:parry:3",
	"monster:knight":          "Parry:parry:2",
	"monster:marilith":        "Parry:parry:5",
	"monster:noble":           "Parry:parry:2",
	"monster:ochre-jelly":     "Split:text:0",
	"monster:shield-guardian": "Shield:text:0",
	"monster:shrieker":        "Shriek:text:0",
	"monster:stone-giant":     "Rock Catching:text:0",
}

func optionLine(o LegendaryOption) string {
	return o.Name + ":" + strconv.Itoa(o.Cost) + ":" + o.Kind
}

// TestLegendaryProfileOfEverySRDStatBlock: every SRD stat block with legendary
// actions reads to the options, costs and kinds written out above, and no other
// stat block reads as legendary.
func TestLegendaryProfileOfEverySRDStatBlock(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	seen := 0
	for _, e := range c.c.monsters {
		p, _ := c.LegendaryProfile(e.Key)
		want, legendary := legendaryGolden[e.Key]
		if !legendary {
			if p.PerRound != 0 || len(p.Options) != 0 || p.ResistancePerDay != 0 {
				t.Errorf("%s reads as legendary: %+v", e.Key, p)
			}
			continue
		}
		seen++
		var got []string
		for _, o := range p.Options {
			got = append(got, optionLine(o))
			if o.NamePT == "" || o.NamePT == o.Name {
				t.Errorf("%s: option %q has no Portuguese name", e.Key, o.Name)
			}
		}
		if !slices.Equal(got, want.options) {
			t.Errorf("%s options = %v, want %v", e.Key, got, want.options)
		}
		if p.PerRound != LegendaryActionsPerRound || p.ResistancePerDay != want.resistance {
			t.Errorf("%s per round %d, resistance %d; want %d, %d", e.Key, p.PerRound, p.ResistancePerDay, LegendaryActionsPerRound, want.resistance)
		}
	}
	if seen != len(legendaryGolden) {
		t.Errorf("read %d legendary stat blocks, the table has %d", seen, len(legendaryGolden))
	}
}

// TestLegendaryOptionKeysAndAttackActions: an option key is unique in its stat
// block, and an attack option names an action of the stat block that rolls an attack.
func TestLegendaryOptionKeysAndAttackActions(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for key := range legendaryGolden {
		cr, _ := c.CreatureByKey(key)
		p := ReadLegendaryProfile(cr)
		keys := map[string]bool{}
		for _, o := range p.Options {
			if o.Key == "" || keys[o.Key] {
				t.Errorf("%s option %q has key %q (empty or repeated)", key, o.Name, o.Key)
			}
			keys[o.Key] = true
			if o.Cost < 1 || o.Cost > LegendaryActionsPerRound || strings.Contains(o.Name, "(Costs") {
				t.Errorf("%s option %q has cost %d or a cost in its name", key, o.Name, o.Cost)
			}
			if (o.Kind == LegendaryKindAttack) != (o.ActionKey != "") {
				t.Errorf("%s option %q: kind %s with action %q", key, o.Name, o.Kind, o.ActionKey)
			}
		}
	}
}

// TestMonsterReactionsOfEverySRDStatBlock: Parry reads its armor class bonus
// ("adds N to its AC"), every other reaction is text, and only these stat blocks
// have reactions.
func TestMonsterReactionsOfEverySRDStatBlock(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, e := range c.c.monsters {
		p, _ := c.LegendaryProfile(e.Key)
		want, has := reactionGolden[e.Key]
		if !has {
			if len(p.Reactions) != 0 {
				t.Errorf("%s has unexpected reactions %+v", e.Key, p.Reactions)
			}
			continue
		}
		if len(p.Reactions) != 1 {
			t.Errorf("%s reactions = %+v, want one", e.Key, p.Reactions)
			continue
		}
		r := p.Reactions[0]
		if got := r.Name + ":" + r.Kind + ":" + strconv.Itoa(r.ACBonus); got != want {
			t.Errorf("%s reaction = %s, want %s", e.Key, got, want)
		}
	}
}

// TestLegendaryProfileOfOrdinaryCreatures: a goblin has none, an unknown key is not a creature.
func TestLegendaryProfileOfOrdinaryCreatures(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	if p, ok := c.LegendaryProfile("monster:goblin"); !ok || p.IsLegendary() {
		t.Errorf("goblin = %+v, %v", p, ok)
	}
	if _, ok := c.LegendaryProfile("monster:nope"); ok {
		t.Error("an unknown key read as a creature")
	}
	p, _ := c.LegendaryProfile("monster:adult-red-dragon")
	if !p.IsLegendary() {
		t.Error("adult red dragon is not legendary")
	}
}

// TestLegendaryMoveSpeedPercent: the tarrasque's Move is half its speed, the vampires' is all of it.
func TestLegendaryMoveSpeedPercent(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for key, want := range map[string]int{"monster:tarrasque": 50, "monster:vampire-vampire": 100} {
		p, ok := c.LegendaryProfile(key)
		if !ok {
			t.Fatalf("%s: no profile", key)
		}
		var got int
		for _, o := range p.Options {
			if o.Kind == LegendaryKindMove {
				got = o.MoveSpeedPercent()
			}
		}
		if got != want {
			t.Errorf("%s: the move walks %d%% of the speed, want %d%%", key, got, want)
		}
	}
}

// The option's Portuguese name is the content's; with none (homebrew content) it is the option's Name,
// without the cost suffix.
func TestLegendaryOptionNamePT(t *testing.T) {
	t.Parallel()
	p := ReadLegendaryProfile(Creature{LegendaryActions: []CreatureAbility{
		{Name: "Tail Attack", NamePT: "Ataque de cauda"},
		{Name: "Wing Attack (Costs 2 Actions)"},
	}})
	if got := p.Options[0].NamePT; got != "Ataque de cauda" {
		t.Errorf("NamePT = %q, want the content's name", got)
	}
	if got := p.Options[1].NamePT; got != "Wing Attack" {
		t.Errorf("NamePT = %q, want the name without the cost when the content has none", got)
	}
}
