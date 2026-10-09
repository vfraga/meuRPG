package rules

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Reading the free-text equipment of a sheet written before the inventory ("Corda de
// cânhamo (15 m), 10 rações, tochas") into items. It matches the pieces against the
// Portuguese names of the SRD equipment and never invents: what matches nothing stays
// free text, and a number the text lacks is a guess the player corrects.

// MaxTextPieces is how many pieces one line becomes at most.
const MaxTextPieces = 20

// guessedPlural is the quantity proposed for a plural noun with no number ("tochas").
const guessedPlural = 10

// TextPiece is one piece of a free-text line as an item.
type TextPiece struct {
	// Source is the words it came from.
	Source string
	// Key is the SRD equipment it matched, "" for free text.
	Key string
	// Name is the equipment's Portuguese name, or the words kept as free text.
	Name     string
	Quantity int
	// Guessed says the text had no number and Quantity is the app's guess.
	Guessed bool
}

var (
	leadingNumber  = regexp.MustCompile(`^(\d{1,4})\s*(?:[x×]\s*|\s+)(.+)$`)
	trailingNumber = regexp.MustCompile(`^(.+?)\s*(?:[x×]\s*(\d{1,4})|\((\d{1,4})\))$`)
)

// ParseEquipmentText splits the text at commas (outside parentheses), semicolons and line
// breaks, and reads each piece.
func (c *Content) ParseEquipmentText(text string) []TextPiece {
	index := c.c.equipmentByName()
	var out []TextPiece
	for _, src := range splitPieces(text) {
		if len(out) == MaxTextPieces {
			break
		}
		name, qty, given := readQuantity(src)
		piece := TextPiece{Source: src, Name: name, Quantity: qty}
		if key, ok := index[normalizeName(name)]; ok {
			piece.Key, piece.Name = key, c.c.namePT(key)
		}
		if !given {
			piece.Quantity = 1
			if isPluralName(name) {
				piece.Quantity, piece.Guessed = guessedPlural, true
			}
		}
		out = append(out, piece)
	}
	return out
}

// splitPieces cuts a text at commas outside parentheses, semicolons and line breaks.
func splitPieces(text string) []string {
	var out []string
	depth, start := 0, 0
	flush := func(end int) {
		if p := strings.TrimSpace(text[start:end]); p != "" {
			out = append(out, p)
		}
		start = end + 1
	}
	for i, r := range text {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth = max(depth-1, 0)
		case (r == ',' && depth == 0) || r == ';' || r == '\n':
			flush(i)
		}
	}
	flush(len(text))
	return out
}

// readQuantity separates a leading "10 " or "10x", or a trailing "x10" or "(20)", from the name.
func readQuantity(piece string) (name string, qty int, given bool) {
	if m := leadingNumber.FindStringSubmatch(piece); m != nil {
		n, _ := strconv.Atoi(m[1])
		return strings.TrimSpace(m[2]), max(n, 1), true
	}
	if m := trailingNumber.FindStringSubmatch(piece); m != nil {
		digits := cmpFirst(m[2], m[3])
		n, _ := strconv.Atoi(digits)
		return strings.TrimSpace(m[1]), max(n, 1), true
	}
	return piece, 1, false
}

func cmpFirst(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// isPluralName says the name looks plural: its last word ends in s.
func isPluralName(name string) bool {
	words := strings.Fields(strings.ToLower(name))
	if len(words) == 0 {
		return false
	}
	// The head noun is the first word ("rações", "tochas"); a name like "corda de cânhamo"
	// has a singular head.
	return strings.HasSuffix(words[0], "s") && !strings.HasSuffix(words[0], "ss")
}

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// normalizeName makes names comparable: no accents, lower case, each word singular.
func normalizeName(s string) string {
	plain, _, err := transform.String(stripMarks, strings.ToLower(s))
	if err != nil {
		plain = strings.ToLower(s)
	}
	words := strings.Fields(plain)
	for i, w := range words {
		words[i] = singularWord(w)
	}
	return strings.Join(words, " ")
}

func singularWord(w string) string {
	switch {
	case len(w) > 4 && (strings.HasSuffix(w, "oes") || strings.HasSuffix(w, "aes")):
		return w[:len(w)-3] + "ao"
	case len(w) > 4 && strings.HasSuffix(w, "ais"):
		return w[:len(w)-3] + "al"
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	}
	return w
}

// equipmentByName indexes the SRD equipment by its normalized Portuguese name, and by that
// name without its parenthesis ("Rações (1 dia)" is also "ração") when that is not another
// item's name.
func (c *content) equipmentByName() map[string]string {
	out := make(map[string]string, 2*len(c.equipment))
	short := map[string]string{}
	clash := map[string]bool{}
	for _, k := range sortedKeys(c.equipment) {
		name := c.namePT(k)
		out[normalizeName(name)] = k
		if i := strings.Index(name, "("); i > 0 {
			base := normalizeName(name[:i])
			if _, seen := short[base]; seen {
				clash[base] = true
			}
			short[base] = k
		}
	}
	for base, k := range short {
		if _, taken := out[base]; !taken && !clash[base] {
			out[base] = k
		}
	}
	return out
}
