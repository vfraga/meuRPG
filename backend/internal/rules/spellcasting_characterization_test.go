package rules

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// The corpus of characterSpells: sheets of every casting shape, each with the
// spell lists right and then broken one way at a time. For each the golden
// keeps the spells the sheet ends up with (and which are prepared), the open
// choices and every issue about the cantrip, known and prepared lists, with
// the "A classe mudou" sentences.

// spellsAt lists the spells of a class list at a spell level, in key order.
func spellsAt(c *Content, class string, level int) []string {
	var out []string
	for _, k := range sortedKeys(c.c.spells) {
		if s := c.c.spells[k]; s.Level == level && c.c.onList(s, class) {
			out = append(out, k)
		}
	}
	return out
}

// spellsOff lists the spells of a level that are on none of the classes' lists.
func spellsOff(c *Content, level int, classes ...string) []string {
	var out []string
	for _, k := range sortedKeys(c.c.spells) {
		s := c.c.spells[k]
		if s.Level != level || len(s.Subclasses) > 0 {
			continue
		}
		on := false
		for _, cl := range classes {
			on = on || c.c.onList(s, cl)
		}
		if !on {
			out = append(out, k)
		}
	}
	return out
}

// take is up to n keys from index from; a content with fewer gives fewer.
func take(keys []string, from, n int) []string {
	from = min(from, len(keys))
	return slices.Clone(keys[from:min(from+n, len(keys))])
}

func cat(lists ...[]string) []string { return slices.Concat(lists...) }

// spellBuild is a sheet of one or more classes with its three spell lists.
type spellBuild struct {
	race, subrace string
	classes       []ClassLevel
	cantrips      []string
	known         []string
	prepared      []string
	picks         []string
}

func (s spellBuild) build() Build {
	return Build{
		BaseScores: map[Ability]int{STR: 10, DEX: 14, CON: 14, INT: 12, WIS: 14, CHA: 16},
		Race:       cmpOr(s.race, "race:human"), Subrace: s.subrace, Background: "background:acolyte",
		Classes: s.classes, Cantrips: s.cantrips, SpellsKnown: s.known, SpellsPrepared: s.prepared, FeatureChoices: s.picks,
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func one(class, sub string, level int) []ClassLevel {
	return []ClassLevel{{Class: class, Subclass: sub, Level: level}}
}

// srdSpellScenarios are the sheets run against the SRD content.
func srdSpellScenarios(c *Content) map[string]spellBuild {
	sc := map[string]spellBuild{}
	wiz := func(level int) []ClassLevel { return one("class:wizard", "subclass:evocation", level) }
	wc := spellsAt(c, "class:wizard", 0)
	w1, w2, w3, w4 := spellsAt(c, "class:wizard", 1), spellsAt(c, "class:wizard", 2), spellsAt(c, "class:wizard", 3), spellsAt(c, "class:wizard", 4)
	clericOnly1 := spellsOff(c, 1, "class:wizard")
	clericOnly0 := spellsOff(c, 0, "class:wizard")
	book := cat(take(w1, 0, 6), take(w2, 0, 4), take(w3, 0, 4))

	// Wizard: a spellbook, prepared from it.
	sc["wizard ok"] = spellBuild{classes: wiz(5), cantrips: take(wc, 0, 4), known: book, prepared: take(book, 0, 6)}
	sc["wizard no spells"] = spellBuild{classes: wiz(5)}
	sc["wizard prepared too many"] = spellBuild{classes: wiz(5), cantrips: take(wc, 0, 4), known: book, prepared: take(book, 0, 9)}
	sc["wizard prepared too few"] = spellBuild{classes: wiz(5), cantrips: take(wc, 0, 4), known: book, prepared: take(book, 0, 2)}
	sc["wizard prepared outside the spellbook"] = spellBuild{classes: wiz(5), cantrips: take(wc, 0, 4), known: take(book, 0, 8), prepared: cat(take(book, 0, 5), take(book, 9, 1))}
	sc["wizard prepared twice"] = spellBuild{classes: wiz(5), known: book, prepared: []string{book[0], book[0], book[1]}}
	sc["wizard cantrips too many"] = spellBuild{classes: wiz(5), cantrips: take(wc, 0, 6), known: book, prepared: take(book, 0, 6)}
	sc["wizard cantrips too few"] = spellBuild{classes: wiz(5), cantrips: take(wc, 0, 2), known: book, prepared: take(book, 0, 6)}
	sc["wizard cantrip twice"] = spellBuild{classes: wiz(5), cantrips: []string{wc[0], wc[0], wc[1]}}
	sc["wizard unknown cantrip"] = spellBuild{classes: wiz(5), cantrips: []string{wc[0], "spell:nope", wc[1]}}
	sc["wizard cantrip that is level 1"] = spellBuild{classes: wiz(5), cantrips: cat(take(wc, 0, 1), take(w1, 0, 1))}
	sc["wizard cantrip off the list"] = spellBuild{classes: wiz(5), cantrips: []string{wc[0], clericOnly0[0]}}
	sc["wizard known cantrip"] = spellBuild{classes: wiz(5), known: cat(take(wc, 0, 1), take(w1, 0, 1))}
	sc["wizard unknown spell"] = spellBuild{classes: wiz(5), known: []string{w1[0], "spell:nope"}, prepared: []string{"spell:nope"}}
	sc["wizard spell too high"] = spellBuild{classes: wiz(5), known: cat(take(w1, 0, 2), take(w4, 0, 1)), prepared: []string{w4[0]}}
	sc["wizard spell off the list"] = spellBuild{classes: wiz(5), known: []string{w1[0], clericOnly1[0]}, prepared: []string{clericOnly1[0]}}
	sc["wizard prepared cantrip"] = spellBuild{classes: wiz(5), known: take(book, 0, 3), prepared: []string{wc[0]}}
	sc["wizard level 1"] = spellBuild{classes: wiz(1), cantrips: take(wc, 0, 3), known: take(w1, 0, 6), prepared: take(w1, 0, 2)}
	sc["wizard level 20 high spells"] = spellBuild{classes: wiz(20), cantrips: take(wc, 0, 5), known: cat(take(spellsAt(c, "class:wizard", 9), 0, 1), take(spellsAt(c, "class:wizard", 8), 0, 1)), prepared: take(spellsAt(c, "class:wizard", 9), 0, 1)}

	// Cleric: prepares from the whole list, the domain's spells come for free.
	cl := func(level int) []ClassLevel { return one("class:cleric", "subclass:life", level) }
	cw := spellsAt(c, "class:cleric", 1)
	cw2 := spellsAt(c, "class:cleric", 2)
	cw3 := spellsAt(c, "class:cleric", 3)
	cc := spellsAt(c, "class:cleric", 0)
	domain := []string{"spell:bless", "spell:cure-wounds", "spell:lesser-restoration", "spell:spiritual-weapon", "spell:beacon-of-hope", "spell:revivify"}
	sc["cleric ok"] = spellBuild{classes: cl(5), cantrips: take(cc, 0, 4), prepared: cat(take(cw, 2, 3), take(cw2, 2, 2), take(cw3, 2, 2))}
	sc["cleric with the domain listed"] = spellBuild{classes: cl(5), cantrips: take(cc, 0, 4), prepared: cat(domain[:2], take(cw, 4, 2), take(cw2, 3, 2), domain[2:4])}
	sc["cleric domain at level 1"] = spellBuild{classes: cl(1), cantrips: take(cc, 0, 3), prepared: cat(domain[:2], take(cw, 3, 2))}
	sc["cleric domain beyond the level"] = spellBuild{classes: cl(2), cantrips: take(cc, 0, 3), prepared: cat(domain[:2], domain[4:5])}
	sc["cleric prepared too many"] = spellBuild{classes: cl(5), cantrips: take(cc, 0, 4), prepared: cat(take(cw, 2, 5), take(cw2, 2, 3), take(cw3, 2, 2))}
	sc["cleric prepared too few"] = spellBuild{classes: cl(5), cantrips: take(cc, 0, 4), prepared: take(cw, 2, 2)}
	sc["cleric no subclass"] = spellBuild{classes: one("class:cleric", "", 3), cantrips: take(cc, 0, 3), prepared: take(cw, 0, 4)}
	sc["cleric known list is ignored for the count"] = spellBuild{classes: cl(3), cantrips: take(cc, 0, 3), known: take(cw, 0, 3), prepared: take(cw, 0, 3)}

	// Sorcerer: knows spells, prepares none.
	so := func(level int) []ClassLevel { return one("class:sorcerer", "subclass:draconic", level) }
	s1, s2, s3, s0 := spellsAt(c, "class:sorcerer", 1), spellsAt(c, "class:sorcerer", 2), spellsAt(c, "class:sorcerer", 3), spellsAt(c, "class:sorcerer", 0)
	sc["sorcerer ok"] = spellBuild{classes: so(5), cantrips: take(s0, 0, 5), known: cat(take(s1, 0, 3), take(s2, 0, 2), take(s3, 0, 1))}
	sc["sorcerer too many known"] = spellBuild{classes: so(5), cantrips: take(s0, 0, 5), known: cat(take(s1, 0, 4), take(s2, 0, 3), take(s3, 0, 2))}
	sc["sorcerer too few known"] = spellBuild{classes: so(5), cantrips: take(s0, 0, 5), known: take(s1, 0, 2)}
	sc["sorcerer too many cantrips"] = spellBuild{classes: so(5), cantrips: take(s0, 0, 7), known: cat(take(s1, 0, 3), take(s2, 0, 2), take(s3, 0, 1))}
	sc["sorcerer known off the list"] = spellBuild{classes: so(5), cantrips: take(s0, 0, 5), known: cat(take(s1, 0, 3), []string{clericOnly1[0]}, take(s2, 0, 2))}
	sc["sorcerer prepared list on a known caster"] = spellBuild{classes: so(5), known: take(s1, 0, 6), prepared: take(s1, 0, 2)}

	// Bard: Magical Secrets let spells off the list in.
	bd := func(level int, sub string) []ClassLevel { return one("class:bard", sub, level) }
	b0, b1, b2, b3, b4, b5 := spellsAt(c, "class:bard", 0), spellsAt(c, "class:bard", 1), spellsAt(c, "class:bard", 2), spellsAt(c, "class:bard", 3), spellsAt(c, "class:bard", 4), spellsAt(c, "class:bard", 5)
	off2 := spellsOff(c, 2, "class:bard")
	off3 := spellsOff(c, 3, "class:bard")
	off1 := spellsOff(c, 1, "class:bard")
	core := cat(take(b1, 0, 4), take(b2, 0, 3), take(b3, 0, 3), take(b4, 0, 2), take(b5, 0, 1))
	sc["bard level 5"] = spellBuild{classes: bd(5, ""), cantrips: take(b0, 0, 3), known: cat(take(b1, 0, 4), take(b2, 0, 3), take(b3, 0, 2))}
	sc["bard level 10 secrets used"] = spellBuild{classes: bd(10, "subclass:lore"), cantrips: take(b0, 0, 4), known: cat(take(core, 0, 11), take(off2, 0, 1), take(off3, 0, 1))}
	sc["bard level 10 secrets over"] = spellBuild{classes: bd(10, ""), cantrips: take(b0, 0, 4), known: cat(take(core, 0, 10), take(off1, 0, 1), take(off2, 0, 1), take(off3, 0, 1))}
	sc["bard level 10 secrets unused off-list below the level"] = spellBuild{classes: bd(10, ""), cantrips: take(b0, 0, 4), known: cat(take(core, 0, 12), take(off1, 0, 2))}
	sc["bard level 6 off the list"] = spellBuild{classes: bd(6, ""), cantrips: take(b0, 0, 3), known: cat(take(core, 0, 9), take(off1, 0, 1))}
	sc["bard lore level 6 knows a wizard spell"] = spellBuild{classes: bd(6, "subclass:lore"), cantrips: take(b0, 0, 3), known: cat(take(core, 0, 9), []string{"spell:acid-arrow"})}
	sc["bard lore level 14 additional secrets"] = spellBuild{classes: bd(14, "subclass:lore"), cantrips: take(b0, 0, 4), known: cat(take(core, 0, 15), take(off2, 0, 1), take(off3, 0, 1), take(off1, 0, 1), take(off1, 1, 1))}
	sc["bard lore level 14 too many"] = spellBuild{classes: bd(14, "subclass:lore"), cantrips: take(b0, 0, 4), known: cat(take(core, 0, 16), take(off2, 0, 1), take(off3, 0, 1), take(off1, 0, 1), take(off1, 1, 1))}

	allBard := cat(take(b1, 0, 30), take(b2, 0, 30), take(b3, 0, 30), take(b4, 0, 30), take(b5, 0, 30))
	sc["bard lore level 14 at the limit with the additional secrets"] = spellBuild{classes: bd(14, "subclass:lore"), cantrips: take(b0, 0, 4), known: cat(take(allBard, 0, 18), take(off2, 0, 1), take(off3, 0, 1))}
	sc["bard lore level 14 one over the limit"] = spellBuild{classes: bd(14, "subclass:lore"), cantrips: take(b0, 0, 4), known: cat(take(allBard, 0, 18), take(off2, 0, 1), take(off3, 0, 1), take(allBard, 18, 1))}
	sc["bard level 14 at the limit without the college"] = spellBuild{classes: bd(14, ""), cantrips: take(b0, 0, 4), known: take(allBard, 0, 18)}

	// Warlock: pact magic, a patron, the Tome, an Arcanum.
	wl := func(level int, sub string) []ClassLevel { return one("class:warlock", sub, level) }
	l0, l1, l2, l3 := spellsAt(c, "class:warlock", 0), spellsAt(c, "class:warlock", 1), spellsAt(c, "class:warlock", 2), spellsAt(c, "class:warlock", 3)
	sc["warlock ok"] = spellBuild{classes: wl(5, "subclass:fiend"), cantrips: take(l0, 0, 3), known: cat(take(l1, 0, 2), take(l2, 0, 2), take(l3, 0, 2))}
	sc["warlock too many known"] = spellBuild{classes: wl(5, "subclass:fiend"), cantrips: take(l0, 0, 3), known: cat(take(l1, 0, 3), take(l2, 0, 3), take(l3, 0, 3))}
	sc["warlock level 1 spell too high"] = spellBuild{classes: wl(1, ""), cantrips: take(l0, 0, 2), known: cat(take(l1, 0, 1), take(l2, 0, 1))}
	sc["warlock fiend expanded list"] = spellBuild{classes: wl(5, "subclass:fiend"), cantrips: take(l0, 0, 3), known: []string{"spell:burning-hands", "spell:command", "spell:blindness-deafness", "spell:scorching-ray", l1[0], l1[1]}}
	sc["warlock other patron's spell"] = spellBuild{classes: wl(5, "subclass:fiend"), cantrips: take(l0, 0, 3), known: []string{"spell:bless", "spell:fireball", l1[0]}}
	tome := "feature:pact-of-the-tome#cantrips"
	sc["warlock tome cantrips"] = spellBuild{classes: wl(3, ""), cantrips: take(l0, 0, 2), known: take(l1, 0, 4), picks: []string{"feature:pact-of-the-tome", ScopedChoice(tome, "spell:guidance"), ScopedChoice(tome, "spell:shillelagh"), ScopedChoice(tome, "spell:eldritch-blast")}}
	sc["warlock tome pick that is not a spell"] = spellBuild{classes: wl(3, ""), cantrips: take(l0, 0, 2), picks: []string{"feature:pact-of-the-tome", ScopedChoice(tome, "nope"), ScopedChoice(tome, "spell:nope")}}
	sc["warlock pick without the feature"] = spellBuild{classes: wl(3, ""), cantrips: take(l0, 0, 2), picks: []string{ScopedChoice(tome, "spell:guidance")}}
	sc["warlock tome pick that is a level 1 spell"] = spellBuild{classes: wl(3, ""), cantrips: take(l0, 0, 2), picks: []string{"feature:pact-of-the-tome", ScopedChoice(tome, "spell:bless")}}
	sc["warlock arcanum"] = spellBuild{classes: wl(11, ""), cantrips: take(l0, 0, 4), known: cat(take(l1, 0, 3), take(l2, 0, 3), take(l3, 0, 3)), picks: []string{ScopedChoice("feature:mystic-arcanum-6th-level", "spell:eyebite")}}
	sc["warlock arcanum listed twice"] = spellBuild{classes: wl(11, ""), known: []string{"spell:eyebite"}, picks: []string{ScopedChoice("feature:mystic-arcanum-6th-level", "spell:eyebite")}}
	sc["wizard spell mastery pick"] = spellBuild{classes: wiz(18), known: take(w1, 0, 3), picks: []string{ScopedChoice("feature:spell-mastery#1st", w1[0])}}
	sc["wizard signature spell pick"] = spellBuild{classes: wiz(20), known: take(w3, 0, 3), picks: []string{ScopedChoice("feature:signature-spell#1", w3[0])}}

	// Races that give spells.
	sc["high elf wizard cantrip picked"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: wiz(1), cantrips: take(wc, 0, 3), picks: []string{ScopedChoice("trait:high-elf-cantrip", "spell:mage-hand")}}
	sc["high elf cantrip pick that is a level 1 spell"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: wiz(1), cantrips: take(wc, 0, 4), picks: []string{ScopedChoice("trait:high-elf-cantrip", "spell:bless")}}
	sc["high elf wizard cantrip in the list"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: wiz(1), cantrips: take(wc, 0, 4)}
	sc["high elf wizard cantrip missing"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: wiz(1), cantrips: take(wc, 0, 3)}
	sc["high elf wizard too many cantrips"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: wiz(1), cantrips: take(wc, 0, 6)}
	sc["high elf cantrip from the wizard list on a cleric"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: one("class:cleric", "", 1), cantrips: cat(take(cc, 0, 3), take(clericOnly0, 0, 0), []string{wc[3]})}
	sc["high elf cantrip off every list"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: one("class:cleric", "", 1), cantrips: cat(take(cc, 0, 3), []string{spellsOff(c, 0, "class:wizard", "class:cleric")[0]})}
	sc["tiefling warlock granted"] = spellBuild{race: "race:tiefling", classes: wl(5, ""), cantrips: cat(take(l0, 0, 2), []string{"spell:thaumaturgy"}), known: cat(take(l1, 0, 2), []string{"spell:hellish-rebuke", "spell:darkness"}, take(l2, 0, 2), take(l3, 0, 2))}
	sc["tiefling cleric prepares a granted spell"] = spellBuild{race: "race:tiefling", classes: one("class:cleric", "", 5), cantrips: cat(take(cc, 0, 3), []string{"spell:thaumaturgy"}), prepared: cat(take(cw, 0, 4), take(cw2, 0, 3), []string{"spell:hellish-rebuke"})}
	sc["tiefling cleric prepares one over"] = spellBuild{race: "race:tiefling", classes: one("class:cleric", "", 5), cantrips: take(cc, 0, 3), prepared: cat(take(cw, 0, 4), take(cw2, 0, 4), []string{"spell:hellish-rebuke"})}
	sc["tiefling granted spell above the slots"] = spellBuild{race: "race:tiefling", classes: wl(1, ""), cantrips: take(l0, 0, 2), known: []string{"spell:darkness", l1[0]}}

	// Other casting classes and no class at all.
	pal := func(level int) []ClassLevel { return one("class:paladin", "", level) }
	p1 := spellsAt(c, "class:paladin", 1)
	sc["paladin level 1"] = spellBuild{classes: pal(1), prepared: take(p1, 0, 1)}
	sc["paladin level 5"] = spellBuild{classes: pal(5), prepared: cat(take(p1, 0, 3), take(spellsAt(c, "class:paladin", 2), 0, 1))}
	sc["paladin level 5 too many"] = spellBuild{classes: pal(5), prepared: cat(take(p1, 0, 5), take(spellsAt(c, "class:paladin", 2), 0, 3))}
	rg := func(level int) []ClassLevel { return one("class:ranger", "", level) }
	r1 := spellsAt(c, "class:ranger", 1)
	sc["ranger level 5"] = spellBuild{classes: rg(5), known: cat(take(r1, 0, 3), take(spellsAt(c, "class:ranger", 2), 0, 1))}
	sc["ranger level 5 too many"] = spellBuild{classes: rg(5), known: cat(take(r1, 0, 6), take(spellsAt(c, "class:ranger", 2), 0, 3))}
	sc["ranger level 1 has no spells"] = spellBuild{classes: rg(1), known: take(r1, 0, 1)}
	dr := spellsAt(c, "class:druid", 1)
	sc["druid level 3 land"] = spellBuild{classes: one("class:druid", "subclass:land", 3), cantrips: take(spellsAt(c, "class:druid", 0), 0, 2), prepared: cat(take(dr, 0, 4), []string{"spell:acid-arrow"})}
	sc["fighter with spells"] = spellBuild{classes: one("class:fighter", "", 5), cantrips: take(wc, 0, 2), known: take(w1, 0, 2), prepared: take(w1, 0, 1)}
	sc["no class lists"] = spellBuild{classes: one("class:fighter", "", 5)}

	// More than one casting class.
	sc["wizard and cleric"] = spellBuild{classes: []ClassLevel{{Class: "class:wizard", Level: 3}, {Class: "class:cleric", Level: 2}}, cantrips: cat(take(wc, 0, 3), take(cc, 0, 3)), known: take(book, 0, 6), prepared: cat(take(book, 0, 3), take(cw, 0, 3))}
	sc["wizard and cleric too many"] = spellBuild{classes: []ClassLevel{{Class: "class:wizard", Level: 3}, {Class: "class:cleric", Level: 2}}, cantrips: cat(take(wc, 0, 4), take(cc, 0, 4)), known: take(book, 0, 6), prepared: cat(take(book, 0, 6), take(cw, 0, 6))}
	sc["wizard and sorcerer"] = spellBuild{classes: []ClassLevel{{Class: "class:wizard", Level: 3}, {Class: "class:sorcerer", Level: 3}}, cantrips: cat(take(wc, 0, 4), take(s0, 0, 4)), known: cat(take(book, 0, 8), take(s1, 0, 4)), prepared: take(book, 0, 4)}
	sc["sorcerer and bard"] = spellBuild{classes: []ClassLevel{{Class: "class:sorcerer", Level: 3}, {Class: "class:bard", Level: 3}}, cantrips: cat(take(s0, 0, 4), take(b0, 0, 2)), known: cat(take(s1, 0, 3), take(b1, 0, 4), take(b2, 0, 2))}
	sc["sorcerer and bard too many"] = spellBuild{classes: []ClassLevel{{Class: "class:sorcerer", Level: 3}, {Class: "class:bard", Level: 3}}, cantrips: cat(take(s0, 0, 6), take(b0, 0, 4)), known: cat(take(s1, 0, 5), take(b1, 0, 6), take(b2, 0, 3))}
	sc["warlock and wizard"] = spellBuild{classes: []ClassLevel{{Class: "class:warlock", Level: 2}, {Class: "class:wizard", Level: 3}}, cantrips: cat(take(l0, 0, 2), take(wc, 0, 3)), known: cat(take(l1, 0, 3), take(book, 0, 5)), prepared: take(book, 0, 3)}
	sc["paladin and ranger"] = spellBuild{classes: []ClassLevel{{Class: "class:paladin", Level: 2}, {Class: "class:ranger", Level: 2}}, known: take(r1, 0, 2), prepared: take(p1, 0, 2)}
	sc["cleric and fighter"] = spellBuild{classes: []ClassLevel{{Class: "class:cleric", Level: 2}, {Class: "class:fighter", Level: 2}}, cantrips: take(cc, 0, 3), prepared: take(cw, 0, 3)}
	return sc
}

// spellOutcome is what a scenario is recorded as.
func spellOutcome(c *Content, sb spellBuild) string {
	d := Derive(sb.build(), c)
	var spells []string
	for _, s := range d.Spells {
		k := s.Spell.Key
		if s.Prepared {
			k += "+"
		}
		spells = append(spells, k)
	}
	var issues []Issue
	for _, is := range d.Issues {
		if strings.HasPrefix(is.Field, "full.cantrip_keys") || strings.HasPrefix(is.Field, "full.known_spell_keys") || strings.HasPrefix(is.Field, "full.prepared_spell_keys") {
			issues = append(issues, is)
		}
	}
	var numbers []string
	for _, sp := range d.Spellcasting {
		numbers = append(numbers, fmt.Sprintf("%s c%d k%d p%d max%d", sp.Class, sp.CantripsKnown, sp.SpellsKnownMax, sp.PreparedMax, sp.MaxSpellLevel))
	}
	b, err := json.Marshal(struct {
		Spells  []string
		Open    []OpenChoice
		Issues  []Issue
		Numbers []string
	}{spells, d.OpenChoices, issues, numbers})
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func TestCharacterSpellsCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	got := map[string]string{}
	for name, sb := range srdSpellScenarios(srd) {
		got["srd: "+name] = spellOutcome(srd, sb)
	}
	table := withOverlayOn(t, srd, stamped(t, srd))
	tableScenarios := map[string]spellBuild{}
	w0 := spellsAt(table, "class:wizard", 0)
	w1 := spellsAt(table, "class:wizard", 1)
	w2 := spellsAt(table, "class:wizard", 2)
	tableScenarios["eldritch knight ok"] = spellBuild{classes: one("class:fighter", "subclass:cavaleiro-runico@mesa", 7), cantrips: take(w0, 0, 2), known: cat(take(w1, 0, 3), take(w2, 0, 2))}
	tableScenarios["eldritch knight too many"] = spellBuild{classes: one("class:fighter", "subclass:cavaleiro-runico@mesa", 7), cantrips: take(w0, 0, 4), known: cat(take(w1, 0, 5), take(w2, 0, 3))}
	tableScenarios["eldritch knight too few"] = spellBuild{classes: one("class:fighter", "subclass:cavaleiro-runico@mesa", 7), cantrips: take(w0, 0, 1), known: take(w1, 0, 1)}
	tableScenarios["eldritch knight level 3"] = spellBuild{classes: one("class:fighter", "subclass:cavaleiro-runico@mesa", 3), cantrips: take(w0, 0, 2), known: take(w1, 0, 3)}
	tableScenarios["eldritch knight level 2 casts nothing"] = spellBuild{classes: one("class:fighter", "subclass:cavaleiro-runico@mesa", 2), cantrips: take(w0, 0, 2), known: take(w1, 0, 3)}
	tableScenarios["trickster ok"] = spellBuild{classes: one("class:rogue", "subclass:trapaceiro-mistico@mesa", 7), cantrips: take(w0, 0, 2), known: cat(take(w1, 0, 3), take(w2, 0, 2)), prepared: take(w1, 0, 2)}
	tableScenarios["trickster too many prepared"] = spellBuild{classes: one("class:rogue", "subclass:trapaceiro-mistico@mesa", 7), cantrips: take(w0, 0, 2), known: take(w1, 0, 4), prepared: take(w1, 0, 4)}
	tableScenarios["knight and wizard"] = spellBuild{classes: []ClassLevel{{Class: "class:fighter", Subclass: "subclass:cavaleiro-runico@mesa", Level: 7}, {Class: "class:wizard", Level: 3}}, cantrips: take(w0, 0, 4), known: take(w1, 0, 6), prepared: take(w1, 0, 3)}
	tableScenarios["high elf with a table caster, cantrips over"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: one("class:gen-full-prepared@mesa", "subclass:gen-full-prepared-a@mesa", 5), cantrips: take(spellsAt(table, "class:cleric", 0), 0, 8)}
	tableScenarios["high elf with a table caster, cantrips at the limit"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: one("class:gen-full-prepared@mesa", "subclass:gen-full-prepared-a@mesa", 5), cantrips: take(spellsAt(table, "class:cleric", 0), 0, 4)}
	tableScenarios["high elf knight, cantrips over"] = spellBuild{race: "race:elf", subrace: "subrace:high-elf", classes: one("class:fighter", "subclass:cavaleiro-runico@mesa", 7), cantrips: take(w0, 0, 6), known: take(w1, 0, 3)}
	for _, gk := range genKinds {
		slug := "gen-" + gk.name
		class := "class:" + slug + tableSuffix
		sub := "subclass:" + slug + "-b" + tableSuffix
		if gk.casting.Kind == CastingNone {
			continue
		}
		list := gk.casting.ListFrom
		tableScenarios[slug+" ok"] = spellBuild{classes: one(class, sub, 5), cantrips: take(spellsAt(table, list, 0), 0, 3), known: take(spellsAt(table, list, 1), 0, 4), prepared: take(spellsAt(table, list, 1), 0, 3)}
		tableScenarios[slug+" always prepared listed"] = spellBuild{classes: one(class, sub, 5), prepared: []string{"spell:bless", "spell:fireball", spellsAt(table, list, 1)[2]}}
		tableScenarios[slug+" always prepared below the level"] = spellBuild{classes: one(class, sub, 3), prepared: []string{"spell:bless", "spell:fireball"}}
		tableScenarios[slug+" too many"] = spellBuild{classes: one(class, sub, 5), cantrips: take(spellsAt(table, list, 0), 0, 8), known: take(spellsAt(table, list, 1), 0, 12), prepared: take(spellsAt(table, list, 1), 0, 12)}
	}
	for name, sb := range tableScenarios {
		got["table: "+name] = spellOutcome(table, sb)
	}
	goldenText(t, "characterspells", got)
}
