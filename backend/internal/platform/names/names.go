// Package names checks the text that users type the same way in every
// module: short, one-line names, such as a campaign's name or a display name
// (Clean), and longer texts that may span several lines, such as a
// character's backstory (CleanText).
package names

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// ErrEmpty means the name is empty, or only spaces.
var ErrEmpty = errors.New("must not be empty")

// ErrNoVisible means the name has no visible character: it is made only of
// spaces and invisible characters.
var ErrNoVisible = errors.New("must have at least one visible character")

// Characters that render as nothing or as blank space but are letters or
// marks to Unicode: the Hangul fillers and the Khmer inherent vowels.
const (
	hangulChoseongFiller  = '\u115F'
	hangulJungseongFiller = '\u1160'
	hangulFiller          = '\u3164'
	halfwidthHangulFiller = '\uFFA0'
	khmerInherentVowelAq  = '\u17B4'
	khmerInherentVowelAa  = '\u17B5'
	zeroWidthNonJoiner    = '\u200C'
	zeroWidthJoiner       = '\u200D'
)

// IsHidden reports whether r is a character a one-line text must not carry
// besides the control characters: the line and paragraph separators, the
// invisible format characters (text direction, zero width, word joiner, byte
// order mark) and the filler characters. The zero-width joiner and
// non-joiner are allowed, since emoji sequences and some scripts need them;
// a text of only those still has no visible character.
func IsHidden(r rune) bool {
	switch r {
	case zeroWidthNonJoiner, zeroWidthJoiner:
		return false
	case hangulChoseongFiller, hangulJungseongFiller, hangulFiller, halfwidthHangulFiller,
		khmerInherentVowelAq, khmerInherentVowelAa:
		return true
	}
	return unicode.Is(unicode.Bidi_Control, r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
}

// isVisible reports whether r draws something: a letter, mark, number,
// punctuation or symbol that is not a filler.
func isVisible(r rune) bool {
	return !IsHidden(r) && r != zeroWidthNonJoiner && r != zeroWidthJoiner &&
		unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S)
}

// Clean trims spaces at both ends of s and checks that the rest is a
// one-line name of 1 to maxLength characters (Unicode code points, the same
// unit as char_length in SQL, so the database CHECKs agree). It returns the
// trimmed name, or an error whose message can be shown to the user.
//
// It refuses control characters (line breaks, tabs, NUL...), the line and
// paragraph separators, and the invisible Unicode characters (text direction,
// zero-width and filler characters), which could make a name look like
// another one on screen or render blank; a name needs at least one visible
// character. The name is returned in Unicode normalization form C, so the
// same text typed two ways is one name. Accents, emoji and any script are
// fine.
func Clean(s string, maxLength int) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrEmpty
	}
	if !utf8.ValidString(s) {
		return "", errors.New("must be valid UTF-8")
	}
	s = norm.NFC.String(s)
	if n := utf8.RuneCountInString(s); n > maxLength {
		return "", fmt.Errorf("must be at most %d characters, got %d", maxLength, n)
	}
	visible := false
	for _, r := range s {
		if IsHidden(r) || unicode.IsControl(r) {
			return "", fmt.Errorf("must not contain the character %U", r)
		}
		visible = visible || isVisible(r)
	}
	if !visible {
		return "", ErrNoVisible
	}
	return s, nil
}

// CleanText trims spaces (and blank lines) at both ends of s and checks that
// the rest is a text of at most maxLength characters (Unicode code points,
// like Clean). Unlike Clean, it accepts an empty text, line breaks and tabs.
// Windows (\r\n) and old Mac (\r) line breaks become \n. It returns the
// cleaned text, or an error whose message can be shown to the user.
//
// It refuses every other control character and the invisible characters
// that change text direction, the line and paragraph separators and the
// zero-width and filler characters, for the same reasons as Clean, and
// normalizes to NFC like it.
func CleanText(s string, maxLength int) (string, error) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) {
		return "", errors.New("must be valid UTF-8")
	}
	s = norm.NFC.String(s)
	if n := utf8.RuneCountInString(s); n > maxLength {
		return "", fmt.Errorf("must be at most %d characters, got %d", maxLength, n)
	}
	for _, r := range s {
		if r == '\n' || r == '\t' {
			continue
		}
		if IsHidden(r) || unicode.IsControl(r) {
			return "", fmt.Errorf("must not contain the character %U", r)
		}
	}
	return s, nil
}
