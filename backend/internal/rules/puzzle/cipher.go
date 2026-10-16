package puzzle

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ErrCipher means the message or the key of a cipher is not valid.
var ErrCipher = errors.New("puzzle: the cipher is not valid")

// The limits of a cipher.
const (
	// MaxCipherMessage is the longest message, in characters.
	MaxCipherMessage = 300
	// MinShift and MaxShift bound a shift key: 0 and 26 change nothing.
	MinShift = 1
	MaxShift = 25
	// MinKeyword and MaxKeyword bound the distinct letters of a keyword key.
	MinKeyword = 3
	MaxKeyword = 26
)

// CipherKey is how a message is ciphered: each letter is swapped for another, always
// the same. Exactly one of the two is set. A Shift moves every letter that many
// places on (3: A becomes D, X becomes A). A Keyword starts the cipher alphabet with
// its own letters (once each, in order) and goes on with the rest of the alphabet:
// "LUA" ciphers A to L, B to U, C to A, D to B, E to C, and so on.
type CipherKey struct {
	Shift   int
	Keyword string
}

// alphabet is the 26 letters the cipher swaps: a to z, folded.
const alphabet = "abcdefghijklmnopqrstuvwxyz"

// cipherAlphabet is the letter each plain letter becomes, a to z. It is refused when
// the key changes no letter, or has too few distinct letters in a keyword.
func cipherAlphabet(k CipherKey) ([26]byte, error) {
	switch {
	case k.Shift != 0 && k.Keyword != "":
		return [26]byte{}, fmt.Errorf("%w: a shift and a keyword together", ErrCipher)
	case k.Shift != 0:
		return shiftAlphabet(k.Shift)
	case k.Keyword != "":
		return keywordAlphabet(k.Keyword)
	default:
		return [26]byte{}, fmt.Errorf("%w: no key", ErrCipher)
	}
}

// shiftAlphabet is the alphabet moved shift places on.
func shiftAlphabet(shift int) ([26]byte, error) {
	var out [26]byte
	if shift < MinShift || shift > MaxShift {
		return out, fmt.Errorf("%w: the shift is %d, want %d to %d", ErrCipher, shift, MinShift, MaxShift)
	}
	for i := range out {
		out[i] = alphabet[(i+shift)%len(alphabet)]
	}
	return out, nil
}

// keywordAlphabet is the keyword's distinct letters, then the rest of the
// alphabet in order. On a refusal it still hands back what it built, as the
// alphabet of a keyword that changes no letter.
func keywordAlphabet(keyword string) ([26]byte, error) {
	var out [26]byte
	order, used := keywordLetters(keyword)
	if len(order) < MinKeyword || len(order) > MaxKeyword {
		return out, fmt.Errorf("%w: the keyword has %d distinct letters, want %d to %d", ErrCipher, len(order), MinKeyword, MaxKeyword)
	}
	for i := range len(alphabet) {
		if c := alphabet[i]; !used[c] {
			order = append(order, c)
		}
	}
	copy(out[:], order)
	if string(out[:]) == alphabet {
		return out, fmt.Errorf("%w: the keyword changes no letter", ErrCipher)
	}
	return out, nil
}

// keywordLetters is the letters a to z of the folded keyword, once each, in
// order, and the set of them.
func keywordLetters(keyword string) (order []byte, used map[byte]bool) {
	used = map[byte]bool{}
	for _, r := range Fold(keyword) {
		if r >= 'a' && r <= 'z' && !used[byte(r)] {
			used[byte(r)] = true
			order = append(order, byte(r))
		}
	}
	return order, used
}

// ValidateCipher checks the key and the message: 1 to MaxCipherMessage characters
// with at least one letter A to Z once folded (the cipher has nothing to hide
// otherwise).
func ValidateCipher(message string, k CipherKey) error {
	if _, err := cipherAlphabet(k); err != nil {
		return err
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(message)); n < 1 || n > MaxCipherMessage {
		return fmt.Errorf("%w: the message has %d characters, want 1 to %d", ErrCipher, n, MaxCipherMessage)
	}
	if !strings.ContainsFunc(Fold(message), func(r rune) bool { return r >= 'a' && r <= 'z' }) {
		return fmt.Errorf("%w: the message has no letter to cipher", ErrCipher)
	}
	return nil
}

// Encipher ciphers the message: the letters A to Z (accents taken off, upper case)
// are swapped, and everything else (spaces, digits, punctuation) stays where it is.
// "O tesouro está sob o altar" with a shift of 3 is "R WHVRXUR HVWD VRE R DOWDU".
func Encipher(message string, k CipherKey) (string, error) {
	if err := ValidateCipher(message, k); err != nil {
		return "", err
	}
	cipher, _ := cipherAlphabet(k) // checked above
	var b strings.Builder
	for _, r := range accentFolder.Replace(strings.ToLower(strings.TrimSpace(message))) {
		if r >= 'a' && r <= 'z' {
			b.WriteByte(cipher[r-'a'] - 'a' + 'A')
			continue
		}
		b.WriteRune(r)
	}
	return b.String(), nil
}

// Decipher is Encipher backwards: it turns a ciphered text into the plain one
// (upper case, no accents). The service never calls it for a player; the tests use it
// to prove the round trip.
func Decipher(ciphertext string, k CipherKey) (string, error) {
	cipher, err := cipherAlphabet(k)
	if err != nil {
		return "", err
	}
	var back [26]byte
	for i, c := range cipher {
		back[c-'a'] = alphabet[i]
	}
	var b strings.Builder
	for _, r := range ciphertext {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte(back[r-'A'] - 'a' + 'A')
			continue
		}
		b.WriteRune(r)
	}
	return b.String(), nil
}
