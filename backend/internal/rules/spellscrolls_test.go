package rules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

func scrollsContent() *content {
	family := &srd51.MagicItem{Key: "item:spell-scroll", Category: "scroll", Variants: []string{"item:spell-scroll-cantrip", "item:spell-scroll-1st", "item:spell-scroll-2nd"}}
	return &content{magicItems: map[string]*srd51.MagicItem{
		"item:spell-scroll":         family,
		"item:spell-scroll-cantrip": {Key: "item:spell-scroll-cantrip", Category: "scroll", VariantOf: "item:spell-scroll"},
		"item:spell-scroll-1st":     {Key: "item:spell-scroll-1st", Category: "scroll", VariantOf: "item:spell-scroll"},
		"item:spell-scroll-2nd":     {Key: "item:spell-scroll-2nd", Category: "scroll", VariantOf: "item:spell-scroll"},
		"item:wand":                 {Key: "item:wand", Category: "wand"},
		"item:spell-scroll-1rd":     {Key: "item:spell-scroll-1rd", Category: "scroll", VariantOf: "item:spell-scroll"},
		"item:spell-scroll-1sth":    {Key: "item:spell-scroll-1sth", Category: "scroll", VariantOf: "item:spell-scroll"},
		"item:spell-scroll-3st":     {Key: "item:spell-scroll-3st", Category: "scroll", VariantOf: "item:spell-scroll"},
	}}
}

const scrollsGood = `{"item:spell-scroll-cantrip":{"level":0,"save_dc":13,"attack_bonus":5},"item:spell-scroll-1st":{"level":1,"save_dc":13,"attack_bonus":5},"item:spell-scroll-2nd":{"level":2,"save_dc":13,"attack_bonus":5}}`

func loadScrolls(c *content, body string) error {
	return c.loadSpellScrolls(fstest.MapFS{"effects/spell_scrolls.json": {Data: []byte(body)}})
}

func TestLoadSpellScrollsAccepts(t *testing.T) {
	t.Parallel()
	c := scrollsContent()
	if err := loadScrolls(c, scrollsGood); err != nil {
		t.Fatal(err)
	}
	got, ok := (&Content{c: c}).SpellScrollNumbers("item:spell-scroll-2nd")
	if !ok || got != (SpellScrollNumbers{Level: 2, SaveDC: 13, AttackBonus: 5}) {
		t.Errorf("got %+v %v", got, ok)
	}
}

func TestSpellScrollNumbersRefusesWhatIsNoScroll(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	for _, key := range []string{"item:spell-scroll", "item:bag-of-holding", "item:potion-of-healing-common", "item:nope", ""} {
		if got, ok := c.SpellScrollNumbers(key); ok || got != (SpellScrollNumbers{}) {
			t.Errorf("%q: %+v %v", key, got, ok)
		}
	}
}

func TestLoadSpellScrollsCharacterization(t *testing.T) {
	t.Parallel()
	row := func(key string, level, dc, bonus int) string {
		return fmt.Sprintf(`%q:{"level":%d,"save_dc":%d,"attack_bonus":%d}`, key, level, dc, bonus)
	}
	rows := func(r ...string) string { return "{" + strings.Join(r, ",") + "}" }
	cantrip, first, second := row("item:spell-scroll-cantrip", 0, 13, 5), row("item:spell-scroll-1st", 1, 13, 5), row("item:spell-scroll-2nd", 2, 13, 5)
	cases := map[string]string{
		"bad: missing":                            "<missing>",
		"bad: not json":                           `{`,
		"bad: not an object":                      `[1]`,
		"bad: unknown field":                      `{"item:spell-scroll-1st":{"level":1,"save_dc":13,"attack_bonus":5,"x":1}}`,
		"ok: every variant":                       scrollsGood,
		"bad: nothing":                            `{}`,
		"bad: a variant with no row":              rows(cantrip, first),
		"bad: a row of an unknown item":           rows(cantrip, first, second, row("item:nope", 3, 15, 7)),
		"bad: a row of the family":                rows(cantrip, first, second, row("item:spell-scroll", 3, 15, 7)),
		"bad: a row of an item that is no scroll": rows(cantrip, first, second, row("item:wand", 3, 15, 7)),
		"bad: a level that is not the variant's":  rows(cantrip, first, row("item:spell-scroll-2nd", 3, 13, 5)),
		"bad: a suffix that is no ordinal (1rd)":  rows(cantrip, first, second, row("item:spell-scroll-1rd", 1, 13, 5)),
		"bad: a suffix that is no ordinal (1sth)": rows(cantrip, first, second, row("item:spell-scroll-1sth", 1, 13, 5)),
		"bad: the wrong ordinal word (3st)":       rows(cantrip, first, second, row("item:spell-scroll-3st", 3, 15, 7)),
		"bad: two variants of one level":          rows(cantrip, first, row("item:spell-scroll-2nd", 1, 13, 5)),
		"bad: level below 0":                      rows(row("item:spell-scroll-cantrip", -1, 13, 5), first, second),
		"bad: level above 9":                      rows(cantrip, first, row("item:spell-scroll-2nd", 10, 13, 5)),
		"bad: dc under 10":                        rows(row("item:spell-scroll-cantrip", 0, 9, 1), first, second),
		"bad: dc over 30":                         rows(row("item:spell-scroll-cantrip", 0, 31, 23), first, second),
		"bad: attack bonus negative":              rows(row("item:spell-scroll-cantrip", 0, 13, -1), first, second),
		"bad: attack bonus not dc minus 8":        rows(row("item:spell-scroll-cantrip", 0, 13, 6), first, second),
		"bad: no dc":                              rows(`"item:spell-scroll-cantrip":{"level":0,"attack_bonus":5}`, first, second),
		"order: key before level":                 rows(row("item:nope", 12, 0, 0)),
		"order: level before dc":                  rows(row("item:spell-scroll-cantrip", 12, 0, 0)),
		"order: dc before bonus":                  rows(row("item:spell-scroll-cantrip", 0, 40, 0)),
	}
	runEffectLoaderCases(t, "spellscrolls-refusals", "effects/spell_scrolls.json", cases, scrollsContent,
		func(c *content, fsys fstest.MapFS) error { return c.loadSpellScrolls(fsys) },
		func(c *content) any { return c.spellScrolls })
}

var (
	scrollTableRow = regexp.MustCompile(`^\| (Cantrip|\d+)(?:st|nd|rd|th)? \| [A-Za-z ]+ \| (\d+) \| \+(\d+) \|$`)
	scrollOwnText  = regexp.MustCompile(`saving throw is (\d+) and the attack bonus is \+(\d+)`)
	scrollOwnPT    = regexp.MustCompile(`A CD de resistência da magia é (\d+) e o bônus de ataque é \+(\d+)`)
)

// The file holds the numbers of the family table of the Spell Scroll entry, for each of the ten variants, and each
// variant's own Portuguese text says the same; the English text says the same too, except the 6th level one: the
// snapshot's text for it says 18 and +10, the table 17 and +9.
func TestSpellScrollNumbersMatchTheCatalog(t *testing.T) {
	t.Parallel()
	c := loadForTest(t)
	family, ok := c.MagicItem("item:spell-scroll")
	if !ok || len(family.Variants) != 10 {
		t.Fatalf("family: %v %v", family, ok)
	}
	table := map[int][2]int{}
	for _, line := range family.Desc {
		m := scrollTableRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		level := 0
		if m[1] != "Cantrip" {
			level, _ = strconv.Atoi(m[1])
		}
		dc, _ := strconv.Atoi(m[2])
		bonus, _ := strconv.Atoi(m[3])
		table[level] = [2]int{dc, bonus}
	}
	if len(table) != 10 {
		t.Fatalf("the family table has %d rows: %v", len(table), table)
	}
	const knownDifference = "item:spell-scroll-6th"
	for _, key := range family.Variants {
		got, ok := c.SpellScrollNumbers(key)
		if !ok {
			t.Errorf("%s has no numbers", key)
			continue
		}
		if want := table[got.Level]; got.SaveDC != want[0] || got.AttackBonus != want[1] {
			t.Errorf("%s (level %d): %d and +%d, the family table says %v", key, got.Level, got.SaveDC, got.AttackBonus, want)
		}
		v, _ := c.MagicItem(key)
		own := scrollOwnText.FindStringSubmatch(strings.Join(v.Desc, " "))
		if own == nil {
			t.Errorf("%s: no numbers in its text", key)
			continue
		}
		dc, _ := strconv.Atoi(own[1])
		bonus, _ := strconv.Atoi(own[2])
		ownPT := scrollOwnPT.FindStringSubmatch(strings.Join(v.DescPT, " "))
		if ownPT == nil {
			t.Errorf("%s: no numbers in its Portuguese text", key)
		} else if dcPT, bonusPT := atoi(ownPT[1]), atoi(ownPT[2]); dcPT != got.SaveDC || bonusPT != got.AttackBonus {
			t.Errorf("%s: its Portuguese text says %d and +%d, the file %d and +%d", key, dcPT, bonusPT, got.SaveDC, got.AttackBonus)
		}
		same := dc == got.SaveDC && bonus == got.AttackBonus
		if key == knownDifference {
			if same || dc != 18 || bonus != 10 {
				t.Errorf("%s: its text says %d and +%d, the snapshot slip is 18 and +10 (and the file says %d and +%d)", key, dc, bonus, got.SaveDC, got.AttackBonus)
			}
		} else if !same {
			t.Errorf("%s: its text says %d and +%d, the file %d and +%d", key, dc, bonus, got.SaveDC, got.AttackBonus)
		}
	}
	if got, _ := c.SpellScrollNumbers(knownDifference); got.SaveDC != 17 || got.AttackBonus != 9 || got.Level != 6 {
		t.Errorf("6th level: %+v, want 17 and +9", got)
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
