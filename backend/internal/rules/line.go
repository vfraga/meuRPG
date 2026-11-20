package rules

import (
	"slices"
	"strings"
)

// The line filter of weapon_rule and extra_damage (their `line`): the one attack line the effect goes
// with, on top of the weapon filter. The filter names the line by one of
//   - "pact": the weapon line the sheet marks as the pact weapon (WeaponMark.PactWeapon);
//   - the key of a weapon of the content, or of the unarmed strike;
//   - the attack_name, or the line key (PseudoWeaponKey, NaturalAttackKey), of a pseudo_weapon or
//     natural_attack of the same entry.
//
// The table's content replaces the value with the key of the line on the effect it compiles, so the sheet
// and the combat compare keys alone (LineMatches).

// LinePact is the line value that names the character's pact weapon.
const LinePact = "pact"

// LineMatches says whether an effect whose resolved line is line goes with the attack line of the key
// that the sheet does (or does not) mark as the pact weapon. An empty line goes with every line.
func LineMatches(line, key string, pact bool) bool {
	switch line {
	case "":
		return true
	case LinePact:
		return pact
	}
	return line == key
}

// simpleLine resolves the values of a line that need no entry: "pact", the unarmed strike and a weapon of
// the content.
func (c *content) simpleLine(line string) (string, bool) {
	if line == LinePact || line == UnarmedStrikeKey {
		return line, true
	}
	if eq, ok := c.equipment[line]; ok && eq.Weapon != nil {
		return line, true
	}
	return "", false
}

// line is the compile-time check of a line: the table's content resolved it already (to a line key), the
// SRD's names a weapon or the pact weapon.
func (x effectCompiler) line() error {
	e := x.e
	if e.Line == "" {
		return nil
	}
	if _, ok := x.c.simpleLine(e.Line); ok || strings.HasPrefix(e.Line, "pseudo:") || strings.HasPrefix(e.Line, "natural:") {
		return nil
	}
	return x.fail("line %q names no weapon of the content", e.Line)
}

// checkLine checks the line of a weapon_rule or extra_damage of the table's content at its field and
// resolves it: the effect then carries the line's key. The effect's own entry is the only one whose pseudo_weapon and
// natural_attack lines it may name.
func (b *overlayBuilder) checkLine(site effectSite, e *Effect) error {
	if e.Line == "" {
		return nil
	}
	if key, ok := b.n.simpleLine(e.Line); ok {
		e.Line = key
		return nil
	}
	keys := b.entryLines(site.owner, e.Line)
	switch len(keys) {
	case 0:
		return site.fail(".line", ReasonReference, "line %q names no weapon, no pseudo_weapon or natural_attack of this entry and not %q", e.Line, LinePact)
	case 1:
		e.Line = keys[0]
		return nil
	}
	return site.fail(".line", ReasonValue, "line %q names %d attack lines of this entry: give them different names", e.Line, len(keys))
}

// entryLines are the keys of the pseudo_weapon and natural_attack lines of the owner's entry that the
// text names, by attack_name (ignoring case and accents) or by the line's key.
func (b *overlayBuilder) entryLines(owner, text string) []string {
	entry, ok := b.featureEntry[owner]
	if !ok {
		entry = owner
	}
	want := foldPT(strings.TrimSpace(text))
	var keys []string
	for _, p := range b.pending {
		if p.entry != entry {
			continue
		}
		pseudo, natural := 0, 0
		for i := range p.effects {
			var key string
			switch p.effects[i].Type {
			case "pseudo_weapon":
				pseudo++
				key = PseudoWeaponKey(p.owner, pseudo)
			case "natural_attack":
				natural++
				key = NaturalAttackKey(p.owner, natural)
			default:
				continue
			}
			if (text == key || foldPT(strings.TrimSpace(p.effects[i].AttackName)) == want) && !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

// checkPactMark is the part of checkWeaponMarks about one mark's pact_weapon: at most one weapon of the
// sheet is the pact weapon.
func checkPactMark(m WeaponMark, field string, seenPact *bool) error {
	if !m.PactWeapon {
		return nil
	}
	if *seenPact {
		return invalid(field+".pact_weapon", "at most one weapon is the pact weapon")
	}
	*seenPact = true
	return nil
}
