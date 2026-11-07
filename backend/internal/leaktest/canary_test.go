package leaktest

import (
	"fmt"
	"strconv"
	"strings"
)

// A canary is a string the master wrote into a hidden thing, or a secret the
// master's data holds. The leak tests look for every canary in every answer a
// player gets. Three kinds of needle, chosen so that none of them can turn up
// in an answer by chance:
//
//   - a marker, "LEAKCANARY-<kind>-<n>", written into every free-text field of a
//     hidden thing (a word that exists nowhere else, never a short word or hex);
//   - an id of a hidden entity (the exact UUID);
//   - a secret number, a number that the master's data holds and that a player must
//     never read in a numeric field (an NPC with 4,321 hit points, a trap DC of 29).
//     Numbers are compared in the decoded answer, field by field, never in the
//     JSON text, where "29" could be a part of anything.
//
// What a player may legitimately read is not a canary: a thing the master showed
// (an opened scene's name, a shown puzzle's clue) is registered as "public" so
// the test can prove, by a positive control, that the player's read works.

// markerPrefix starts every marker. It is not hex and not a word.
const markerPrefix = "LEAKCANARY-"

// canary is one needle and who may legitimately read it.
type canary struct {
	needle  string
	kind    string // "trap-name", "npc-hp"... what hides here, for the failure message
	numeric bool   // a secret number: compared in numeric fields only
	value   int64  // the number, when numeric
	// fields, for a small number (a DC from 1 to 30 turns up everywhere), limits the
	// search to numeric fields whose name has one of these words ("dc"). A big number
	// (4,321 hit points) is searched in every numeric field.
	fields []string
	// readers are the people who may read it besides the master (who reads
	// everything): a clue revealed to one player, a trap revealed to one character.
	readers map[string]bool
	// public marks what the master showed to everyone. It is checked in the other
	// direction: the matrix must find it in the players' answers somewhere.
	public bool
}

// secrets is the registry the fixture fills while it builds the master's data.
type secrets struct {
	list  []*canary
	count map[string]int
}

func newSecrets() *secrets { return &secrets{count: map[string]int{}} }

func (s *secrets) add(c *canary) *canary {
	s.list = append(s.list, c)
	return c
}

// marker returns a new marker of a kind: "LEAKCANARY-trap-name-3". readers are
// the people who may read it besides the master.
func (s *secrets) marker(kind string, readers ...*person) string {
	s.count[kind]++
	m := markerPrefix + kind + "-" + strconv.Itoa(s.count[kind])
	s.add(&canary{needle: m, kind: kind, readers: names(readers)})
	return m
}

// public returns a marker the master showed to everyone, so a player may read it.
func (s *secrets) public(kind string) string {
	s.count[kind]++
	m := markerPrefix + kind + "-" + strconv.Itoa(s.count[kind])
	s.add(&canary{needle: m, kind: kind, public: true})
	return m
}

// id registers the id of a hidden entity.
func (s *secrets) id(kind, id string, readers ...*person) string {
	s.add(&canary{needle: id, kind: kind + "-id", readers: names(readers)})
	return id
}

// number registers a secret number and returns it, so the fixture can use it. A
// number that is not unlikely by itself (a DC) is searched only in the fields
// whose name has one of the words in fields.
func (s *secrets) number(kind string, n int64, fields ...string) int64 {
	for _, c := range s.list {
		if c.numeric && c.value == n && c.kind == kind {
			return n
		}
	}
	s.add(&canary{needle: strconv.FormatInt(n, 10), kind: kind, numeric: true, value: n, fields: fields})
	return n
}

func names(ps []*person) map[string]bool {
	m := map[string]bool{}
	for _, p := range ps {
		m[p.name] = true
	}
	return m
}

// mayRead says whether p may read the canary.
func (c *canary) mayRead(p *person) bool {
	if p.reads != nil {
		return p.reads(c)
	}
	return c.public || c.readers[p.name]
}

func (c *canary) String() string {
	if c.numeric {
		return fmt.Sprintf("%s %s", c.kind, c.needle)
	}
	return fmt.Sprintf("%s %q", c.kind, strings.TrimSpace(c.needle))
}

// release makes the first canary of a kind and a number readable: the master
// let it out (a hint, a clue), and the kind has several such markers.
func (s *secrets) release(needle string, readers ...*person) {
	for _, c := range s.list {
		if strings.HasSuffix(c.needle, needle) || strings.HasSuffix(c.needle, "-"+needle) {
			c.readers = names(readers)
		}
	}
}

// allowNeedle lets readers read the canary whose needle is exactly this one.
func (s *secrets) allowNeedle(needle string, readers ...*person) {
	for _, c := range s.list {
		if c.needle == needle {
			c.readers = names(readers)
		}
	}
}
