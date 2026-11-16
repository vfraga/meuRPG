package rules

import (
	"fmt"
	"slices"
	"strings"
)

// LostPrerequisite is what a feat or an option the sheet keeps asks and the character lacks now (board 580-05): the
// issue's sentence is built from it for each reader (SentencePT), as the character's name is not in the Build and a
// condition the players may not read is named for the master alone (RN-23).
type LostPrerequisite struct {
	// Key is the feat's or the option's key: with the issue's code and the list its pick is in, what the issue is
	// told apart by (Issue.ID), whatever its sentence says now.
	Key string
	// EntryPT is the name of the feat or the option.
	EntryPT string
	// Option says it is an option; otherwise it is a feat.
	Option bool
	// Conditions are the missing conditions, in the order the prerequisite lists them.
	Conditions []LostCondition
	// unmet is what the conditions were made from, to say them again over the sheet as it ends up.
	unmet []FeatUnmet
}

// LostCondition is one missing condition: what is asked ("Guerreiro 7") and what the character has, told after the
// character's name ("e Tor está no 5", "que Tor não conhece mais").
type LostCondition struct {
	AskPT  string
	LinkPT string
	HasPT  string
	// Hidden says the condition is on an entry the players may not read and the sheet does not have: a player reads
	// hiddenAskPT for it. In a player's view of the content AskPT is empty.
	Hidden bool
	// ShownPT is what a player reads of a hidden condition that also asks for something they may read: the races
	// they may read of a list with a hidden one, as the feat's picker names them. Empty for any other.
	ShownPT string
}

// lostNobody stands for the character in an issue's own Message, which names no reader and is the same for all.
const lostNobody = "o personagem"

// SentencePT is the sentence a reader gets: "Asa Partida pede Guerreiro 7, e Tor está no 5: a opção só vale de novo
// quando Tor cumprir.". name is the character's (empty reads "o personagem"); master says the reader is the
// campaign's master, who reads a hidden condition named.
func (l LostPrerequisite) SentencePT(name string, master bool) string {
	if name == "" {
		name = lostNobody
	}
	what := "o talento"
	if l.Option {
		what = "a opção"
	}
	asked := listPT(l.asked(name, master))
	if asked == "" { // no condition to name: the prerequisite as a whole
		asked = "um pré-requisito que " + name + " não cumpre"
	}
	return fmt.Sprintf("%s pede %s: %s só vale de novo quando %s cumprir.", l.EntryPT, asked, what, name)
}

// asked are the missing conditions as the reader gets them, each once. The conditions a player may not read are
// one phrase for all of them, and the last: its place in the list would say which kind of condition it stands for.
func (l LostPrerequisite) asked(name string, master bool) []string {
	var parts []string
	hidden := false
	for _, c := range l.Conditions {
		ask := c.AskPT
		if c.Hidden && (!master || ask == "") {
			ask = c.ShownPT
		}
		if ask == "" {
			hidden = true
			continue
		}
		if part := ask + ", " + c.LinkPT + " " + name + " " + c.HasPT; !slices.Contains(parts, part) {
			parts = append(parts, part)
		}
	}
	if hidden {
		parts = append(parts, hiddenAskPT)
	}
	return parts
}

// listPT joins phrases as a list: "A", "A e B", "A, B e C".
func listPT(parts []string) string {
	last := len(parts) - 1
	if last < 1 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:last], ", ") + " e " + parts[last]
}

// lost is the LostPrerequisite of the entry key for the missing conditions unmet, judged on the sheet d.
func (c *content) lost(key string, option bool, unmet []FeatUnmet, b Build, d Derived) *LostPrerequisite {
	l := &LostPrerequisite{Key: key, EntryPT: c.namePT(key), Option: option, unmet: unmet}
	for _, u := range unmet {
		l.Conditions = append(l.Conditions, c.lostCondition(u, b, d))
	}
	return l
}

// lostIDPrefix marks, in an issue's identity, the key of a lost feat or option where another issue has its sentence.
const lostIDPrefix = "lost:"

// ID is what an issue is told apart by, among the issues of a sheet and over time: its code, its field and its
// sentence. A lost prerequisite is told apart by the entry's key in the sentence's place, and by the name of the
// list its pick is in without the pick's place: the sentence says what the character has now and what the reader may
// see, and the place moves when a swap or a rule takes an earlier pick out, while the issue is the same one. The sheet
// stores these (known issues), so a stored lost prerequisite holds no sentence and no name.
func (is Issue) ID() string {
	if is.Lost != nil {
		return is.Code + "|" + listOf(is.Field) + "|" + lostIDPrefix + is.Lost.Key
	}
	return is.Code + "|" + is.Field + "|" + is.Message
}

// listOf is the name of the sheet's list a field is in, without the place in it: "full.feat_keys[2]" is in
// "full.feat_keys", and a field with no place is its own.
func listOf(field string) string {
	list, _, _ := strings.Cut(field, "[")
	return list
}

// sentenceStart is how the sentence of a lost feat or option begins: it names the entry after it, and the rest is
// the sentence's own.
const sentenceStart = "O personagem não cumpre mais o pré-requisito de "

// sentenceRow is a stored identity that holds a lost feat or option by its sentence: the issue's code, the field it is
// in and the sentence, which begins the same for every reader and names the entry. The place in the field is not read:
// a feat taken through a choice is stored with none, and a swap may have moved a pick.
type sentenceRow struct {
	index    int
	field    string
	sentence string
}

// sentenceRows are the stored identities that hold, by their sentence, a lost prerequisite of this code and list.
func (is Issue) sentenceRows(stored []string) []sentenceRow {
	var rows []sentenceRow
	for i, id := range stored {
		code, rest, ok := strings.Cut(id, "|")
		if !ok || code != is.Code {
			continue
		}
		field, sentence, ok := strings.Cut(rest, "|")
		if ok && listOf(field) == listOf(is.Field) && strings.HasPrefix(sentence, sentenceStart) {
			rows = append(rows, sentenceRow{index: i, field: field, sentence: sentence})
		}
	}
	return rows
}

// named says the row's sentence names this issue's entry.
func (r sentenceRow) named(is Issue) bool {
	return strings.HasPrefix(r.sentence, sentenceStart+is.Lost.EntryPT+":")
}

// knownIssues matches the stored identities to the issues of a sheet. An issue is known when its identity is stored.
// A stored sentence is the same issue as the lost prerequisite it names, and each such row holds one issue: the rows
// that name an entry go to that entry first, then each row left goes to one lost prerequisite of its code and list that
// no row names (the entry may have been renamed, its pick moved), the one at the row's own place first, in the
// order of the issues. held[i] is the issue a stored row went to, and -1 for a row that holds no issue by its sentence.
func knownIssues(stored []string, issues []Issue) (known []bool, held []int) {
	m := rowMatch{stored: stored, issues: issues, known: make([]bool, len(issues)), held: make([]int, len(stored))}
	for i := range m.held {
		m.held[i] = -1
	}
	for i, is := range issues {
		m.known[i] = slices.Contains(stored, is.ID())
	}
	m.assign(func(is Issue, r sentenceRow) bool { return r.named(is) }, false)
	m.assign(func(is Issue, r sentenceRow) bool { return r.field == is.Field }, true)
	m.assign(func(Issue, sentenceRow) bool { return true }, true)
	return m.known, m.held
}

// rowMatch is knownIssues' state: which issues are known, and the issue each stored row went to.
type rowMatch struct {
	stored []string
	issues []Issue
	known  []bool
	held   []int
}

// assign gives, to each lost prerequisite in turn (the known ones too unless skipKnown), the first row left that take
// accepts.
func (m *rowMatch) assign(take func(Issue, sentenceRow) bool, skipKnown bool) {
	for i, is := range m.issues {
		if is.Lost != nil && (!skipKnown || !m.known[i]) {
			m.takeRow(i, take)
		}
	}
}

// takeRow gives issue i the first row left of its code and list that take accepts.
func (m *rowMatch) takeRow(i int, take func(Issue, sentenceRow) bool) {
	is := m.issues[i]
	for _, r := range is.sentenceRows(m.stored) {
		if m.held[r.index] < 0 && take(is, r) {
			m.held[r.index], m.known[i] = i, true
			return
		}
	}
}

// KnownIssues says, for each of issues, whether the stored identities hold it: the sheet's known issues, which are
// what an issue is told "new" against. It is the one place that reads them.
func KnownIssues(stored []string, issues []Issue) []bool {
	known, _ := knownIssues(stored, issues)
	return known
}

// IdentitiesOf is stored with each row that holds an issue by its sentence replaced by the issue's identity.
func IdentitiesOf(stored []string, issues []Issue) []string {
	_, held := knownIssues(stored, issues)
	out := make([]string, 0, len(stored))
	for i, id := range stored {
		if held[i] < 0 {
			out = append(out, id)
		} else if next := issues[held[i]].ID(); !slices.Contains(out, next) {
			out = append(out, next)
		}
	}
	return out
}

// retold is the lost prerequisite said over the sheet d: the same conditions, with what the character has there.
func (c *content) retold(l *LostPrerequisite, b Build, d Derived) *LostPrerequisite {
	return c.lost(l.Key, l.Option, l.unmet, b, d)
}

// lostCondition says one missing condition and what the character has of it.
func (c *content) lostCondition(u FeatUnmet, b Build, d Derived) LostCondition {
	lc := c.lostAsk(u, b, d)
	switch u.Kind {
	case FeatUnmetClassLevel, FeatUnmetPact, FeatUnmetSpell, FeatUnmetOption, FeatUnmetLacksFeature:
		lc.Hidden = c.hiddenOn(b, u.Key)
	}
	if lc.Hidden && c.player {
		lc.AskPT = "" // a player's view holds no name of it, whoever reads the sentence
	}
	return lc
}

// lostAsk is the condition's wording by its kind; a race's says by itself whether it is hidden.
func (c *content) lostAsk(u FeatUnmet, b Build, d Derived) LostCondition {
	switch u.Kind {
	case FeatUnmetClassLevel:
		return c.lostClassLevel(u, b)
	case FeatUnmetAbilityMinimum, FeatUnmetAbilityAnyOf:
		return c.lostAbilities(u, d)
	case FeatUnmetRace:
		return c.lostRace(u, b)
	case FeatUnmetLevel:
		return LostCondition{AskPT: fmt.Sprintf("nível %d", u.Value), LinkPT: "e", HasPT: fmt.Sprintf("está no %d", d.TotalLevel)}
	case FeatUnmetLacksFeature:
		return LostCondition{AskPT: "não ter " + c.namePT(u.Key), LinkPT: "e", HasPT: "tem"}
	}
	return LostCondition{AskPT: c.lostAskPT(u), LinkPT: "que", HasPT: lostHasPT[u.Kind]}
}

// lostHasPT is what the character has of a condition that is plainly missing, told after "que <name>".
var lostHasPT = map[string]string{
	FeatUnmetPact:         "não tem mais",
	FeatUnmetSpell:        "não conhece mais",
	FeatUnmetOption:       "não tem mais",
	FeatUnmetProficiency:  "não tem",
	FeatUnmetSpellcasting: "não consegue mais",
	FeatUnmetSize:         "não tem",
}

// lostAskPT names what a plainly missing condition asks.
func (c *content) lostAskPT(u FeatUnmet) string {
	switch u.Kind {
	case FeatUnmetPact:
		return "a Dádiva do Pacto " + optionLabel(c.namePT(u.Key))
	case FeatUnmetSpell:
		return "a magia " + c.namePT(u.Key)
	case FeatUnmetOption:
		return optionLabel(c.namePT(u.Key))
	case FeatUnmetProficiency:
		return "proficiência em " + c.namePT(u.Key)
	case FeatUnmetSpellcasting:
		return "conjurar magias"
	case FeatUnmetSize:
		return "tamanho " + strings.ToLower(sizeNamePT[u.Key])
	}
	return ""
}

// lostClassLevel says the class level asked and the one the character is at: "Guerreiro 7, e Tor está no 5".
func (c *content) lostClassLevel(u FeatUnmet, b Build) LostCondition {
	class := c.namePT(u.Key)
	lc := LostCondition{AskPT: fmt.Sprintf("%s %d", class, u.Value), LinkPT: "e", HasPT: "não tem nível de " + class}
	if have := classLevelIn(b.Classes, u.Key); have > 0 {
		lc.HasPT = fmt.Sprintf("está no %d", have)
	}
	return lc
}

// lostRace says the races asked: "ser Anão ou Elfo, que Tor não é".
func (c *content) lostRace(u FeatUnmet, b Build) LostCondition {
	asked := u.Keys
	if u.Key != "" {
		asked = []string{u.Key}
	}
	lc := LostCondition{AskPT: "ser " + c.racesPT(asked), LinkPT: "que", HasPT: "não é"}
	shown := slices.DeleteFunc(slices.Clone(asked), func(k string) bool { return c.hiddenOn(b, k) })
	if lc.Hidden = len(shown) < len(asked); lc.Hidden && len(shown) > 0 {
		lc.ShownPT = "ser " + c.racesPT(shown)
	}
	return lc
}

// lostAbilities says the scores asked and the ones the character has: "Força 13, e Tor tem 11".
func (c *content) lostAbilities(u FeatUnmet, d Derived) LostCondition {
	score := map[Ability]int{}
	for _, s := range d.Abilities {
		score[s.Ability] = s.Score
	}
	var asked, has []string
	for _, am := range u.Abilities {
		name := c.namePT(string(am.Ability))
		asked = append(asked, fmt.Sprintf("%s %d", name, am.Minimum))
		has = append(has, fmt.Sprintf("%s %d", name, score[am.Ability]))
	}
	join := " e "
	if u.Kind == FeatUnmetAbilityAnyOf {
		join = " ou "
	}
	lc := LostCondition{AskPT: strings.Join(asked, join), LinkPT: "e", HasPT: "tem " + strings.Join(has, " e ")}
	if len(u.Abilities) == 1 {
		lc.HasPT = fmt.Sprintf("tem %d", score[u.Abilities[0].Ability])
	}
	return lc
}

// hiddenOn says key is an entry the players may not read and the sheet b does not have, whoever the view is for.
func (c *content) hiddenOn(b Build, key string) bool {
	return c.hidden(key) && !c.sheetHas(b, key)
}
