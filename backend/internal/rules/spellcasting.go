package rules

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// preparation turns a spellcasting effect into PreparationKnown,
// PreparationPrepared or PreparationSpellbook.
func preparation(e *Effect) string {
	switch {
	case e.Spellbook:
		return PreparationSpellbook
	case e.Prepares:
		return PreparationPrepared
	}
	return PreparationKnown
}

// caster is one casting class of the character.
type caster struct {
	oc  ownedClass
	e   *Effect
	row *srd51.Level
	sc  *Spellcasting
	// list is the class whose spell list the caster reads: its own class, or a
	// third caster's list (the class a table subclass casts from).
	list string
}

// spellcasting computes each casting class's numbers, the spell slots
// (with the multiclass spellcaster table when more than one class casts,
// pact magic apart) and the list of the character's spells, then checks
// the spell choices.
func (x *deriver) spellcasting() {
	c := x.c
	var casters []caster
	for _, oc := range x.classes {
		cast, ok := c.castingFor(oc.key, oc.subclass)
		if !ok || oc.level < cast.level {
			continue
		}
		e := cast.effect
		a := Ability(e.Ability)
		row := c.castingRow(oc.key, oc.level, cast)
		if row == nil {
			continue
		}
		sc := Spellcasting{
			Class: oc.key, ClassNamePT: c.namePT(oc.key), Ability: a,
			SaveDC: 8 + x.prof + x.mods[a], AttackBonus: x.prof + x.mods[a],
			PreparesSpells: e.Prepares, Ritual: e.Ritual, SpellList: cast.list,
		}
		if s := row.Spellcasting; s != nil {
			sc.CantripsKnown = s.CantripsKnown
			if !e.Prepares {
				sc.SpellsKnownMax = s.SpellsKnown
			}
			sc.MaxSpellLevel = MaxSpellLevelFromSlots(s.Slots)
		}
		if e.Prepares {
			n, err := e.preparedMax.Int(x.env)
			if err != nil {
				x.issue(IssueFormula, "", "O número de magias preparadas de %s não pôde ser calculado.", sc.ClassNamePT)
			}
			sc.PreparedMax = max(n, 0)
		}
		x.d.Spellcasting = append(x.d.Spellcasting, sc)
		casters = append(casters, caster{oc: oc, e: e, row: row, list: cast.list})
	}
	// casters[i] goes with Spellcasting[i]; point at it once the slice
	// stops growing.
	for i := range casters {
		casters[i].sc = &x.d.Spellcasting[i]
	}

	x.d.SpellSlots = make([]int, 9)
	var slotCasters []caster
	for _, cs := range casters {
		if cs.e.Progression == "pact" {
			x.pactMagic(cs)
			continue
		}
		slotCasters = append(slotCasters, cs)
	}
	switch len(slotCasters) {
	case 0:
	case 1:
		if s := slotCasters[0].row.Spellcasting; s != nil {
			copy(x.d.SpellSlots, s.Slots[:])
		}
	default:
		x.multiclassSlots(slotCasters)
	}

	x.characterSpells(casters)
}

// pactMagic reads the warlock row's slots: all of one level.
func (x *deriver) pactMagic(cs caster) {
	s := cs.row.Spellcasting
	if s == nil {
		return
	}
	for i, n := range s.Slots {
		if n > 0 {
			x.d.PactMagic = &PactMagic{SlotLevel: i + 1, Slots: n}
		}
	}
}

// multiclassSlots applies the multiclass spellcaster table: add the levels
// of full casters, half the levels (rounded down) of half casters and a third
// of the levels (rounded down) of third casters, and read the slots of that
// caster level. The table is the same as a full caster's class table, so the
// slots come from the first full-caster class of the SRD (never a table class,
// whose table is its own).
func (x *deriver) multiclassSlots(casters []caster) {
	level := 0
	for _, cs := range casters {
		switch cs.e.Progression {
		case "full":
			level += cs.oc.level
		case "half":
			level += cs.oc.level / 2
		case "third":
			level += cs.oc.level / 3
		}
	}
	if level < 1 || x.c.multiclassTable == "" {
		return
	}
	if s := x.c.classLevels[x.c.multiclassTable][min(level, MaxLevel)-1].Spellcasting; s != nil {
		copy(x.d.SpellSlots, s.Slots[:])
	}
}

// characterSpells builds Derived.Spells and checks the spell choices.
func (x *deriver) characterSpells(casters []caster) {
	c := x.c
	onClassList := func(s *srd51.Spell) bool {
		for _, cs := range casters {
			if c.onList(s, cs.list) {
				return true
			}
			if cs.oc.subclass != nil && slices.Contains(s.Subclasses, cs.oc.subclass.Key) {
				return true
			}
		}
		return false
	}

	// What effects add: granted spells (Infernal Legacy) and cantrip
	// choices from another list (the high elf's wizard cantrip).
	granted := map[string]bool{}
	extraCantrips := 0
	var extraCantripLists []string
	for _, a := range x.active {
		e := a.effect
		for _, s := range e.Spells {
			granted[s] = true
		}
		if e.Type == "choice" && e.Choice == "cantrip" {
			extraCantrips += e.Count
			extraCantripLists = append(extraCantripLists, e.From...)
		}
	}

	// The subclass's always-prepared spells at its class level.
	alwaysPrepared := map[string]bool{}
	for _, cs := range casters {
		if cs.oc.subclass == nil || cs.oc.subclass.ExpandedList {
			continue // an expanded list is spells to choose from, not spells given
		}
		for _, ss := range cs.oc.subclass.Spells {
			if ss.ClassLevel > cs.oc.level {
				continue
			}
			ok := true
			for _, f := range ss.WithFeatures {
				ok = ok && slices.Contains(x.b.FeatureChoices, f)
			}
			if ok {
				alwaysPrepared[ss.Spell] = true
			}
		}
	}

	maxLevel := 0
	for i, n := range x.d.SpellSlots {
		if n > 0 {
			maxLevel = i + 1
		}
	}
	if x.d.PactMagic != nil {
		maxLevel = max(maxLevel, x.d.PactMagic.SlotLevel)
	}
	hasKnownCaster := false
	hasSpellbook := false
	onlySpellbook := true
	knownMax, preparedMax, cantripsMax := 0, 0, extraCantrips
	for _, cs := range casters {
		cantripsMax += cs.sc.CantripsKnown
		switch preparation(cs.e) {
		case PreparationKnown:
			hasKnownCaster = true
			knownMax += cs.sc.SpellsKnownMax
		case PreparationSpellbook:
			hasSpellbook = true
			preparedMax += cs.sc.PreparedMax
		case PreparationPrepared:
			onlySpellbook = false
			preparedMax += cs.sc.PreparedMax
		}
	}
	knownByCaster := func(s *srd51.Spell) bool {
		// A spell off the class's list (the Bard's Magical Secrets) is as known
		// as the others when the sheet has no spellbook to tell them apart.
		if hasKnownCaster && !hasSpellbook {
			return true
		}
		for _, cs := range casters {
			if preparation(cs.e) == PreparationKnown && c.onList(s, cs.list) {
				return true
			}
		}
		return false
	}

	seen := map[string]bool{}
	addSpell := func(s *srd51.Spell, prepared bool) {
		if seen[s.Key] {
			if prepared {
				for i := range x.d.Spells {
					if x.d.Spells[i].Spell.Key == s.Key {
						x.d.Spells[i].Prepared = true
					}
				}
			}
			return
		}
		seen[s.Key] = true
		x.d.Spells = append(x.d.Spells, CharacterSpell{Spell: c.spellEntries[s.Key], Prepared: prepared})
	}

	// Cantrips.
	for i, key := range x.b.Cantrips {
		field := fmt.Sprintf("full.cantrip_keys[%d]", i)
		s, ok := c.spells[key]
		if !ok {
			x.issue(IssueUnknownKey, field, "A magia escolhida não existe no conteúdo %s.", c.version)
			continue
		}
		if s.Level != 0 {
			x.issue(IssueSpellLevel, field, "%s não é um truque.", c.namePT(key))
		}
		fromExtraList := slices.ContainsFunc(extraCantripLists, func(cl string) bool { return c.onList(s, cl) })
		if !onClassList(s) && !granted[key] && !fromExtraList {
			x.issue(IssueSpellNotOnList, field, "%s não está na lista de magias do personagem.", c.namePT(key))
		}
		addSpell(s, true)
	}
	// A spell an effect grants (a race's cantrip) comes on top of the class's
	// numbers: it counts against none of them.
	notGranted := func(keys []string) int {
		n := 0
		for _, k := range keys {
			if !granted[k] {
				n++
			}
		}
		return n
	}
	// whoCasts is the sentence "A classe mudou" tells a spell count that changed,
	// and the entry it is about: with one caster and no other source of the spells,
	// "agora conhece 3 truques; esta ficha tem 4." about the class (the subclass,
	// for a third caster, whose numbers are the subclass's); otherwise only the
	// total, with no entry named.
	whoCasts := func(verb, what, total string, allowed, have int, otherSource bool) (subject, change string) {
		if len(casters) != 1 || otherSource {
			return "", fmt.Sprintf("o total de %s agora é %d; esta ficha tem %d.", total, allowed, have)
		}
		subject = casters[0].oc.key
		if cast, ok := c.castingFor(casters[0].oc.key, casters[0].oc.subclass); ok && cast.sub != "" {
			subject = cast.sub
		}
		return subject, fmt.Sprintf("agora %s %s; esta ficha tem %d.", verb, what, have)
	}
	if n := notGranted(x.b.Cantrips); n > cantripsMax {
		subject, change := whoCasts("conhece", countPT(cantripsMax, "truque", "truques"), "truques conhecidos", cantripsMax, n, extraCantrips > 0)
		x.issueChange(IssueSpellCount, "full.cantrip_keys", subject, change,
			fmt.Sprintf("Há %d truques; o personagem conhece %d.", n, cantripsMax))
	}

	if n := notGranted(x.b.Cantrips); n < cantripsMax {
		x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: OpenChoiceCantrips, Missing: cantripsMax - n})
	}

	// Spells known (a wizard's spellbook, or a known caster's spells) and
	// prepared.
	checkSpell := func(field, key string, offList bool) (*srd51.Spell, bool) {
		s, ok := c.spells[key]
		if !ok {
			x.issue(IssueUnknownKey, field, "A magia escolhida não existe no conteúdo %s.", c.version)
			return nil, false
		}
		switch {
		case s.Level == 0:
			x.issue(IssueSpellLevel, field, "%s é um truque: vai na lista de truques.", c.namePT(key))
		case s.Level > maxLevel && !granted[key]:
			// A granted spell is cast with the feature's own uses, not with a slot.
			x.issue(IssueSpellLevel, field, "%s é de %dº nível; o personagem conjura até o %dº.", c.namePT(key), s.Level, maxLevel)
		}
		if !onClassList(s) && !granted[key] && !alwaysPrepared[key] && !offList {
			x.issue(IssueSpellNotOnList, field, "%s não está na lista de magias do personagem.", c.namePT(key))
		}
		return s, true
	}
	// The Bard's Magical Secrets (two spells from any class at levels 10, 14
	// and 18) and the College of Lore's Additional Magical Secrets (two more,
	// which do not count against the spells known) allow spells off the list.
	offListLeft, extraKnown := 0, 0
	for _, f := range x.d.Features {
		switch {
		case strings.HasPrefix(f.Key, "feature:magical-secrets-") && !isTableKey(f.Key):
			offListLeft += 2
		case f.Key == "feature:additional-magical-secrets" && !isTableKey(f.Key):
			offListLeft += 2
			extraKnown += 2
		}
	}
	for i, key := range x.b.SpellsKnown {
		offList := false
		if s := c.spells[key]; s != nil && offListLeft > 0 && !onClassList(s) {
			offList = true
			offListLeft--
		}
		if s, ok := checkSpell(fmt.Sprintf("full.known_spell_keys[%d]", i), key, offList); ok {
			addSpell(s, knownByCaster(s))
		}
	}
	counted := 0
	for i, key := range x.b.SpellsPrepared {
		field := fmt.Sprintf("full.prepared_spell_keys[%d]", i)
		s, ok := checkSpell(field, key, false)
		if !ok {
			continue
		}
		if !alwaysPrepared[key] && !granted[key] {
			counted++
		}
		if hasSpellbook && onlySpellbook && !slices.Contains(x.b.SpellsKnown, key) {
			x.issue(IssueSpellNotOnList, field, "%s está preparada mas não está no grimório.", c.namePT(key))
		}
		addSpell(s, true)
	}
	for _, key := range sortedKeys(alwaysPrepared) {
		if s, ok := c.spells[key]; ok {
			addSpell(s, true)
		}
	}
	if n := notGranted(x.b.SpellsKnown); hasKnownCaster && !hasSpellbook && n > knownMax+extraKnown {
		subject, change := whoCasts("conhece", countPT(knownMax+extraKnown, "magia", "magias"), "magias conhecidas", knownMax+extraKnown, n, extraKnown > 0)
		x.issueChange(IssueSpellCount, "full.known_spell_keys", subject, change,
			fmt.Sprintf("Há %d magias conhecidas; o personagem conhece %d.", n, knownMax+extraKnown))
	}
	if n := notGranted(x.b.SpellsKnown); hasKnownCaster && !hasSpellbook && n < knownMax {
		x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: OpenChoiceSpellsKnown, Missing: knownMax - n})
	}
	if counted < preparedMax {
		x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: OpenChoiceSpellsPrepared, Missing: preparedMax - counted})
	}
	if preparedMax > 0 && counted > preparedMax {
		subject, change := whoCasts("prepara", countPT(preparedMax, "magia", "magias"), "magias preparadas", preparedMax, counted, false)
		x.issueChange(IssueSpellCount, "full.prepared_spell_keys", subject, change,
			fmt.Sprintf("Há %d magias preparadas; o personagem prepara %d.", counted, preparedMax))
	}

	slices.SortFunc(x.d.Spells, func(a, b CharacterSpell) int {
		return cmp.Or(cmp.Compare(a.Spell.Level, b.Spell.Level), comparePT(a.Spell.NamePT, b.Spell.NamePT))
	})
}

// MaxSpellLevelFromSlots is the highest spell level a class table row can
// cast: the last circle with at least one slot, or 0 when the row has none
// (Paladin and Ranger at level 1). It also works for the Warlock, whose row
// holds all its pact slots at one circle, so that circle is the highest.
func MaxSpellLevelFromSlots(slots [9]int) int {
	highest := 0
	for i, n := range slots {
		if n > 0 {
			highest = i + 1
		}
	}
	return highest
}
