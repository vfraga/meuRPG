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
	var casters []caster
	for _, oc := range x.classes {
		if cs, ok := x.casterOf(oc); ok {
			casters = append(casters, cs)
		}
	}
	// casters[i] goes with Spellcasting[i]; point at it once the slice
	// stops growing.
	for i := range casters {
		casters[i].sc = &x.d.Spellcasting[i]
	}
	x.spellSlots(casters)
	x.characterSpells(casters)
}

// casterOf adds the Spellcasting of a class that casts at its level and
// returns its caster, or false when the class casts nothing yet (or its table
// has no row there).
func (x *deriver) casterOf(oc ownedClass) (caster, bool) {
	c := x.c
	cast, ok := c.castingFor(oc.key, oc.subclass)
	if !ok || oc.level < cast.level {
		return caster{}, false
	}
	e := cast.effect
	a := Ability(e.Ability)
	row := c.castingRow(oc.key, oc.level, cast)
	if row == nil {
		return caster{}, false
	}
	sc := Spellcasting{
		Class: oc.key, ClassNamePT: c.namePT(oc.key), Ability: a,
		SaveDC: 8 + x.prof + x.mods[a], AttackBonus: x.prof + x.mods[a],
		PreparesSpells: e.Prepares, Spellbook: e.Spellbook, Ritual: e.Ritual, SpellList: cast.list,
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
	return caster{oc: oc, e: e, row: row, list: cast.list}, true
}

// spellSlots fills the spell slots: pact magic apart, the class row of a single
// caster, or the multiclass spellcaster table for more.
func (x *deriver) spellSlots(casters []caster) {
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
		level += casterLevelOf(cs.e.Progression, cs.oc.level)
	}
	if level < 1 || x.c.multiclassTable == "" {
		return
	}
	if s := x.c.classLevels[x.c.multiclassTable][min(level, MaxLevel)-1].Spellcasting; s != nil {
		copy(x.d.SpellSlots, s.Slots[:])
	}
}

// The divisors of the multiclass caster level for half and third casters.
const (
	halfCasterDivisor  = 2
	thirdCasterDivisor = 3
)

// casterLevelOf is what a class's levels add to the caster level of the multiclass
// spellcaster table: all of a full caster's, half (rounded down) of a half
// caster's, a third (rounded down) of a third caster's, and none of the
// warlock's, whose Pact Magic is apart (SRD 5.1, "Multiclassing", "Spell Slots").
func casterLevelOf(progression string, classLevel int) int {
	switch progression {
	case "full":
		return classLevel
	case "half":
		return classLevel / halfCasterDivisor
	case "third":
		return classLevel / thirdCasterDivisor
	}
	return 0
}

// spellSheet is the state of building a character's spell list: what the
// effects and the picks grant, the limits of the casting classes, and the
// spells added so far.
type spellSheet struct {
	x       *deriver
	c       *content
	casters []caster

	// granted are the spells an effect or a pick gives (Infernal Legacy's
	// thaumaturgy, a Mystic Arcanum); they count against none of the class's
	// numbers.
	granted map[string]bool
	// extraCantrips are the cantrips effects let the character choose from
	// another list (the high elf's wizard cantrip), from extraCantripLists.
	extraCantrips     int
	extraCantripLists []string
	// grantedCantrips and grantedSpells are what the picks of the choice engine
	// grant (choicegroups.go): a bonus cantrip, the Pact of the Tome's cantrips
	// and a Mystic Arcanum. They are known on top of the class's numbers.
	grantedCantrips, grantedSpells []string
	// alwaysPrepared are the subclass's spells at its class level.
	alwaysPrepared map[string]bool

	// The limits of the casting classes.
	maxLevel                                    int
	hasKnownCaster, hasSpellbook, onlySpellbook bool
	knownMax, preparedMax, cantripsMax          int
	// offListLeft is how many spells off the list the Bard's Magical Secrets
	// still allow, and extraKnown the ones that do not count against the spells
	// known.
	offListLeft, extraKnown int

	seen map[string]bool
}

// characterSpells builds Derived.Spells and checks the spell choices.
func (x *deriver) characterSpells(casters []caster) {
	s := &spellSheet{
		x: x, c: x.c, casters: casters,
		granted: map[string]bool{}, alwaysPrepared: map[string]bool{}, seen: map[string]bool{},
		onlySpellbook: true,
	}
	s.collectGranted()
	s.collectPicks()
	s.collectAlwaysPrepared()
	s.collectLimits()
	s.addCantrips()
	s.checkCantripCount()
	s.countSecrets()
	s.addKnown()
	counted := s.addPrepared()
	s.addAlwaysPrepared()
	s.checkKnownCount()
	s.checkPreparedCount(counted)

	slices.SortFunc(x.d.Spells, func(a, b CharacterSpell) int {
		return cmp.Or(cmp.Compare(a.Spell.Level, b.Spell.Level), comparePT(a.Spell.NamePT, b.Spell.NamePT))
	})
}

// onClassList says the spell is on the list of one of the casting classes, or
// of the subclass of one.
func (s *spellSheet) onClassList(sp *srd51.Spell) bool {
	for _, cs := range s.casters {
		if s.c.onList(sp, cs.list) {
			return true
		}
		if cs.oc.subclass != nil && slices.Contains(sp.Subclasses, cs.oc.subclass.Key) {
			return true
		}
	}
	return false
}

// collectGranted reads what effects add: granted spells (Infernal Legacy) and
// cantrip choices from another list (the high elf's wizard cantrip).
func (s *spellSheet) collectGranted() {
	for _, a := range s.x.active {
		e := a.effect
		for _, sp := range e.Spells {
			s.granted[sp] = true
		}
		if e.Type == "choice" && e.Choice == "cantrip" {
			s.extraCantrips += e.Count
			s.extraCantripLists = append(s.extraCantripLists, e.From...)
		}
	}
}

// collectPicks reads what the picks of the choice engine grant (choicegroups.go).
// The bonus cantrips take the place of the extra cantrips a sheet that predates
// the picks keeps in its cantrip list.
func (s *spellSheet) collectPicks() {
	for _, k := range s.x.b.FeatureChoices {
		choiceKey, spell, ok := SplitScopedChoice(k)
		if !ok || !strings.HasPrefix(spell, "spell:") || !s.x.grantsSpells(choiceKey) {
			continue
		}
		s.granted[spell] = true
		if sp := s.c.spells[spell]; sp != nil && sp.Level == 0 {
			s.grantedCantrips = append(s.grantedCantrips, spell)
			if len(s.c.effectChoices(strings.SplitN(choiceKey, textSeparator, 2)[0], "cantrip")) > 0 {
				s.extraCantrips = max(s.extraCantrips-1, 0)
			}
		} else {
			s.grantedSpells = append(s.grantedSpells, spell)
		}
	}
}

// collectAlwaysPrepared reads the subclass's always-prepared spells at its
// class level.
func (s *spellSheet) collectAlwaysPrepared() {
	for _, cs := range s.casters {
		if cs.oc.subclass == nil || cs.oc.subclass.ExpandedList {
			continue // an expanded list is spells to choose from, not spells given
		}
		for _, ss := range cs.oc.subclass.Spells {
			if ss.ClassLevel <= cs.oc.level && s.hasChosen(ss.WithFeatures) {
				s.alwaysPrepared[ss.Spell] = true
			}
		}
	}
}

// hasChosen says the sheet chose every one of the features.
func (s *spellSheet) hasChosen(features []string) bool {
	for _, f := range features {
		if !slices.Contains(s.x.b.FeatureChoices, f) {
			return false
		}
	}
	return true
}

// collectLimits reads the highest spell level the sheet casts and the numbers
// of cantrips, spells known and spells prepared its classes allow.
func (s *spellSheet) collectLimits() {
	x := s.x
	for i, n := range x.d.SpellSlots {
		if n > 0 {
			s.maxLevel = i + 1
		}
	}
	if x.d.PactMagic != nil {
		s.maxLevel = max(s.maxLevel, x.d.PactMagic.SlotLevel)
	}
	s.cantripsMax = s.extraCantrips
	for _, cs := range s.casters {
		s.cantripsMax += cs.sc.CantripsKnown
		switch preparation(cs.e) {
		case PreparationKnown:
			s.hasKnownCaster = true
			s.knownMax += cs.sc.SpellsKnownMax
		case PreparationSpellbook:
			s.hasSpellbook = true
			s.preparedMax += cs.sc.PreparedMax
		case PreparationPrepared:
			s.onlySpellbook = false
			s.preparedMax += cs.sc.PreparedMax
		}
	}
}

// knownByCaster says the spell is known (not just on the sheet) because a
// caster that knows its spells has it on its list.
func (s *spellSheet) knownByCaster(sp *srd51.Spell) bool {
	// A spell off the class's list (the Bard's Magical Secrets) is as known
	// as the others when the sheet has no spellbook to tell them apart.
	if s.hasKnownCaster && !s.hasSpellbook {
		return true
	}
	for _, cs := range s.casters {
		if preparation(cs.e) == PreparationKnown && s.c.onList(sp, cs.list) {
			return true
		}
	}
	return false
}

// addSpell puts a spell on the sheet once; a second time it only marks it
// prepared.
func (s *spellSheet) addSpell(sp *srd51.Spell, prepared bool) {
	x := s.x
	if s.seen[sp.Key] {
		if prepared {
			for i := range x.d.Spells {
				if x.d.Spells[i].Spell.Key == sp.Key {
					x.d.Spells[i].Prepared = true
				}
			}
		}
		return
	}
	s.seen[sp.Key] = true
	x.d.Spells = append(x.d.Spells, CharacterSpell{Spell: s.c.spellEntries[sp.Key], Prepared: prepared})
}

// notGranted counts the keys that are not granted: a spell an effect grants (a
// race's cantrip) comes on top of the class's numbers and counts against none
// of them.
func (s *spellSheet) notGranted(keys []string) int {
	n := 0
	for _, k := range keys {
		if !s.granted[k] {
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
func (s *spellSheet) whoCasts(verb, what, total string, allowed, have int, otherSource bool) (subject, change string) {
	if len(s.casters) != 1 || otherSource {
		return "", fmt.Sprintf("o total de %s agora é %d; esta ficha tem %d.", total, allowed, have)
	}
	only := s.casters[0]
	subject = only.oc.key
	if cast, ok := s.c.castingFor(only.oc.key, only.oc.subclass); ok && cast.sub != "" {
		subject = cast.sub
	}
	return subject, fmt.Sprintf("agora %s %s; esta ficha tem %d.", verb, what, have)
}

// addCantrips adds the cantrips of the sheet, checking each, then the cantrips
// and spells the picks grant.
func (s *spellSheet) addCantrips() {
	for i, key := range s.x.b.Cantrips {
		s.addCantrip(i, key)
	}
	for _, spell := range s.grantedCantrips {
		if sp := s.c.spells[spell]; sp != nil {
			s.addSpell(sp, true)
		}
	}
	for _, spell := range s.grantedSpells {
		if sp := s.c.spells[spell]; sp != nil {
			s.addSpell(sp, true)
		}
	}
}

func (s *spellSheet) addCantrip(i int, key string) {
	x, c := s.x, s.c
	field := fmt.Sprintf("full.cantrip_keys[%d]", i)
	sp, ok := c.spells[key]
	if !ok {
		x.issue(IssueUnknownKey, field, "A magia escolhida não existe no conteúdo %s.", c.version)
		return
	}
	if sp.Level != 0 {
		x.issue(IssueSpellLevel, field, "%s não é um truque.", c.namePT(key))
	}
	fromExtraList := slices.ContainsFunc(s.extraCantripLists, func(cl string) bool { return c.onList(sp, cl) })
	if !s.onClassList(sp) && !s.granted[key] && !fromExtraList {
		x.issue(IssueSpellNotOnList, field, "%s não está na lista de magias do personagem.", c.namePT(key))
	}
	s.addSpell(sp, true)
}

// checkCantripCount compares the cantrips of the sheet with what the classes
// allow: too many is an issue, too few an open choice.
func (s *spellSheet) checkCantripCount() {
	x := s.x
	n := s.notGranted(x.b.Cantrips)
	if n > s.cantripsMax {
		subject, change := s.whoCasts("conhece", countPT(s.cantripsMax, "truque", "truques"), "truques conhecidos", s.cantripsMax, n, s.extraCantrips > 0)
		x.issueChange(IssueSpellCount, "full.cantrip_keys", subject, change,
			fmt.Sprintf("Há %d truques; o personagem conhece %d.", n, s.cantripsMax))
	}
	if n < s.cantripsMax {
		x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: OpenChoiceCantrips, Missing: s.cantripsMax - n})
	}
}

// checkSpell checks a spell of the known or prepared list: it exists, it is not
// a cantrip, the sheet casts its level and it is on a list the character reads.
func (s *spellSheet) checkSpell(field, key string, offList bool) (*srd51.Spell, bool) {
	x, c := s.x, s.c
	sp, ok := c.spells[key]
	if !ok {
		x.issue(IssueUnknownKey, field, "A magia escolhida não existe no conteúdo %s.", c.version)
		return nil, false
	}
	switch {
	case sp.Level == 0:
		x.issue(IssueSpellLevel, field, "%s é um truque: vai na lista de truques.", c.namePT(key))
	case sp.Level > s.maxLevel && !s.granted[key]:
		// A granted spell is cast with the feature's own uses, not with a slot.
		x.issue(IssueSpellLevel, field, "%s é de %dº nível; o personagem conjura até o %dº.", c.namePT(key), sp.Level, s.maxLevel)
	}
	if !s.onClassList(sp) && !s.granted[key] && !s.alwaysPrepared[key] && !offList {
		x.issue(IssueSpellNotOnList, field, "%s não está na lista de magias do personagem.", c.namePT(key))
	}
	return sp, true
}

// countSecrets reads the Bard's Magical Secrets (two spells from any class at
// levels 10, 14 and 18) and the College of Lore's Additional Magical Secrets
// (two more, which do not count against the spells known): they allow spells off
// the list.
func (s *spellSheet) countSecrets() {
	for _, f := range s.x.d.Features {
		switch {
		case strings.HasPrefix(f.Key, "feature:magical-secrets-") && !isTableKey(f.Key):
			s.offListLeft += secretsPerFeature
		case f.Key == "feature:additional-magical-secrets" && !isTableKey(f.Key):
			s.offListLeft += secretsPerFeature
			s.extraKnown += secretsPerFeature
		}
	}
}

// takeOffList spends one of the Magical Secrets on a spell off the class list.
func (s *spellSheet) takeOffList(key string) bool {
	sp := s.c.spells[key]
	if sp == nil || s.offListLeft <= 0 || s.onClassList(sp) {
		return false
	}
	s.offListLeft--
	return true
}

// addKnown adds the spells known (a wizard's spellbook, or a known caster's spells).
func (s *spellSheet) addKnown() {
	for i, key := range s.x.b.SpellsKnown {
		offList := s.takeOffList(key)
		if sp, ok := s.checkSpell(fmt.Sprintf("full.known_spell_keys[%d]", i), key, offList); ok {
			s.addSpell(sp, s.knownByCaster(sp))
		}
	}
}

// addPrepared adds the prepared spells and returns how many count against the
// number the classes prepare (the always-prepared and the granted do not).
func (s *spellSheet) addPrepared() int {
	x := s.x
	counted := 0
	for i, key := range x.b.SpellsPrepared {
		field := fmt.Sprintf("full.prepared_spell_keys[%d]", i)
		sp, ok := s.checkSpell(field, key, false)
		if !ok {
			continue
		}
		if !s.alwaysPrepared[key] && !s.granted[key] {
			counted++
		}
		if s.hasSpellbook && s.onlySpellbook && !slices.Contains(x.b.SpellsKnown, key) {
			x.issue(IssueSpellNotOnList, field, "%s está preparada mas não está no grimório.", s.c.namePT(key))
		}
		s.addSpell(sp, true)
	}
	return counted
}

func (s *spellSheet) addAlwaysPrepared() {
	for _, key := range sortedKeys(s.alwaysPrepared) {
		if sp, ok := s.c.spells[key]; ok {
			s.addSpell(sp, true)
		}
	}
}

// checkKnownCount compares the spells known with what a known caster allows.
func (s *spellSheet) checkKnownCount() {
	if !s.hasKnownCaster || s.hasSpellbook {
		return
	}
	x := s.x
	n := s.notGranted(x.b.SpellsKnown)
	limit := s.knownMax + s.extraKnown
	if n > limit {
		subject, change := s.whoCasts("conhece", countPT(limit, "magia", "magias"), "magias conhecidas", limit, n, s.extraKnown > 0)
		x.issueChange(IssueSpellCount, "full.known_spell_keys", subject, change,
			fmt.Sprintf("Há %d magias conhecidas; o personagem conhece %d.", n, limit))
	}
	if n < s.knownMax {
		x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: OpenChoiceSpellsKnown, Missing: s.knownMax - n})
	}
}

// checkPreparedCount compares the prepared spells that count with what the
// classes prepare.
func (s *spellSheet) checkPreparedCount(counted int) {
	x := s.x
	if counted < s.preparedMax {
		x.d.OpenChoices = append(x.d.OpenChoices, OpenChoice{Kind: OpenChoiceSpellsPrepared, Missing: s.preparedMax - counted})
	}
	if s.preparedMax > 0 && counted > s.preparedMax {
		subject, change := s.whoCasts("prepara", countPT(s.preparedMax, "magia", "magias"), "magias preparadas", s.preparedMax, counted, false)
		x.issueChange(IssueSpellCount, "full.prepared_spell_keys", subject, change,
			fmt.Sprintf("Há %d magias preparadas; o personagem prepara %d.", counted, s.preparedMax))
	}
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

// grantsSpells says whether the choice with this key (a feature, a trait, with its
// "#part") gives the spells picked for it, and the sheet has it. Spell Mastery and
// Signature Spells name spells of the spellbook, which it already has.
func (x *deriver) grantsSpells(choiceKey string) bool {
	base, _, _ := strings.Cut(choiceKey, textSeparator)
	if base == "feature:spell-mastery" || base == "feature:signature-spell" {
		return false
	}
	for _, f := range x.d.Features {
		if f.Key == base {
			return true
		}
	}
	return false
}
