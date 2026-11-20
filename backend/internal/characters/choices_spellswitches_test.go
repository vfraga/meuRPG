package characters

import (
	"errors"
	"slices"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The table's switches apply to every spell choice: the book's rituals, the Pact of the Tome's cantrips, a
// Mystic Arcanum and a bonus cantrip. A sheet that holds the pick keeps it, and the master picks it all the same.

type spellSwitchRow struct {
	name, choiceKey string
	// sheet is a complete sheet of a player with the picks of the pool.
	sheet func(picks ...string) *charactersv1.CharacterSheet
	// held is the spell the sheet holds before the switch, fresh the spell a pick adds after it, and allowed one
	// that stays on. The three are on the choice's list.
	held, fresh, allowed string
	others               []string // the picks of the pool that stay
	// table is a spell of the table that the choice's list has.
	table func() *rulesv1.TableSpell
}

func warlockPicksSheet(level int32, cantrips int, picks ...string) *charactersv1.CharacterSheet {
	s := choiceSheet("race:human", "", "class:warlock", "subclass:fiend", level, picks...)
	s.GetFull().CantripKeys = []string{"spell:mage-hand", "spell:chill-touch", "spell:prestidigitation", "spell:minor-illusion"}[:cantrips]
	return s
}

func spellSwitchRows() []spellSwitchRow {
	const (
		tome = "feature:pact-of-the-tome"
		book = "feature:eldritch-invocation-book-of-ancient-secrets"
		inv  = "feature:eldritch-invocation-"
	)
	cantrips := func(picks ...string) []string {
		var out []string
		for _, p := range picks {
			out = append(out, rules.ScopedChoice(tome+"#cantrips", p))
		}
		return out
	}
	return []spellSwitchRow{
		{
			name: "the book's rituals", table: func() *rulesv1.TableSpell { return choiceTableSpell(1, true, "class:wizard") }, choiceKey: book + "#rituals", held: "spell:detect-magic", fresh: "spell:alarm", allowed: "spell:find-familiar",
			others: []string{rules.ScopedChoice(book+"#rituals", "spell:comprehend-languages")},
			sheet: func(picks ...string) *charactersv1.CharacterSheet {
				return warlockPicksSheet(5, 3, slices.Concat([]string{tome, book, inv + "devils-sight", inv + "beast-speech"},
					cantrips("spell:guidance", "spell:shillelagh", "spell:eldritch-blast"), picks)...)
			},
		},
		{
			name: "the Tome's cantrips", table: func() *rulesv1.TableSpell { return choiceTableSpell(0, false, "class:wizard") }, choiceKey: tome + "#cantrips", held: "spell:guidance", fresh: "spell:resistance", allowed: "spell:shillelagh",
			others: cantrips("spell:eldritch-blast", "spell:druidcraft"),
			sheet: func(picks ...string) *charactersv1.CharacterSheet {
				return warlockPicksSheet(3, 2, slices.Concat([]string{tome, inv + "devils-sight", inv + "beast-speech"}, picks)...)
			},
		},
		{
			name: "a Mystic Arcanum", table: func() *rulesv1.TableSpell { return choiceTableSpell(6, false, "class:warlock") }, choiceKey: "feature:mystic-arcanum-6th-level", held: "spell:eyebite", fresh: "spell:circle-of-death", allowed: "spell:true-seeing",
			sheet: func(picks ...string) *charactersv1.CharacterSheet {
				return warlockPicksSheet(11, 4, slices.Concat([]string{"feature:pact-of-the-chain", inv + "devils-sight", inv + "beast-speech", inv + "armor-of-shadows", inv + "fiendish-vigor", inv + "mask-of-many-faces"}, picks)...)
			},
		},
		{
			name: "a bonus cantrip", table: func() *rulesv1.TableSpell { return choiceTableSpell(0, false, "class:wizard") }, choiceKey: "trait:high-elf-cantrip", held: "spell:fire-bolt", fresh: "spell:ray-of-frost", allowed: "spell:light",
			sheet: func(picks ...string) *charactersv1.CharacterSheet {
				s := choiceSheet("race:elf", "subrace:high-elf", "class:wizard", "", 1, picks...)
				s.GetFull().CantripKeys = []string{"spell:mage-hand", "spell:prestidigitation", "spell:minor-illusion"}
				return s
			},
		},
	}
}

func TestTheSwitchesApplyToEverySpellChoicePreviewsAndCompletions(t *testing.T) {
	t.Parallel()
	for _, row := range spellSwitchRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tb := newChoiceTable(t)
			scoped := func(spell string) string { return rules.ScopedChoice(row.choiceKey, spell) }
			holds := append(slices.Clone(row.others), scoped(row.held))
			pc, err := tb.create(tb.ana, row.sheet(holds...))
			if err != nil {
				t.Fatalf("a sheet that holds %s: %v", row.held, err)
			}
			tb.master.setOff(t, tb.campaign, true, row.held, row.fresh)

			// The sheet that holds the pick keeps it, with the sheet open to edits and locked.
			pickedBy := func(u *user, sheet *charactersv1.CharacterSheet) (picked []string, err error) {
				res, err := tb.preview(u, pc.GetId(), sheet)
				if err != nil {
					return nil, err
				}
				return choiceByKey(res, row.choiceKey).GetPicked(), nil
			}
			for _, state := range []string{"open to edits", "locked"} {
				if state == "locked" {
					tb.h.lockSheets(tb.campaign)
				}
				if picked, err := pickedBy(tb.ana, row.sheet(holds...)); err != nil || !slices.Contains(picked, row.held) {
					t.Errorf("the sheet that holds %s, %s: picked %v, error %v; want the pick kept", row.held, state, picked, err)
				}
			}

			// The master takes the pick out; now a pick of it is new.
			pc, err = tb.master.update(t, pc, pc.GetName(), row.sheet(row.others...))
			if err != nil {
				t.Fatalf("the master takes the pick out: %v", err)
			}
			for _, spell := range []string{row.held, row.fresh} {
				draft := row.sheet(append(slices.Clone(row.others), scoped(spell))...)
				_, err := pickedBy(tb.ana, draft)
				if !refusedAsSwitchedOff(err, spell) {
					t.Errorf("a locked preview that picks %s: error %v, want SWITCHED_OFF_CONTENT", spell, err)
				}
				if _, err := tb.complete(tb.ana, pc, pick(row.choiceKey, spell)); !refusedAsSwitchedOff(err, spell) {
					t.Errorf("completing the choice with %s: error %v, want SWITCHED_OFF_CONTENT", spell, err)
				}
				master := row.sheet(append(slices.Clone(row.others), scoped(spell))...)
				if picked, err := pickedBy(tb.master, master); err != nil || !slices.Contains(picked, spell) {
					t.Errorf("the master's preview that picks %s: picked %v, error %v; want it", spell, picked, err)
				}
			}
			if picked, err := pickedBy(tb.ana, row.sheet(append(slices.Clone(row.others), scoped(row.allowed))...)); err != nil || !slices.Contains(picked, row.allowed) {
				t.Errorf("a locked preview that picks %s: picked %v, error %v; want it", row.allowed, picked, err)
			}
			if _, err := tb.complete(tb.ana, pc, pick(row.choiceKey, row.allowed)); err != nil {
				t.Errorf("completing the choice with %s: %v", row.allowed, err)
			}
		})
	}
}

// tableSpellOf is a spell of the table of the level, with the ritual tag or not, on the class's list.
func choiceTableSpell(level int32, ritual bool, class string) *rulesv1.TableSpell {
	sp := testSpell("Magia da mesa", class)
	sp.Level, sp.Ritual = level, ritual
	if level == 0 {
		sp.Damage[0].PerSlotLevel = ""
	}
	if ritual {
		sp.Duration = &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_TIMED, Amount: 10, Unit: rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_MINUTE}
	}
	return sp
}

// refusedAsSwitchedOff says err is the switched-off-content refusal for the content key.
func refusedAsSwitchedOff(err error, key string) bool {
	return refusedAs(err, charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT, key)
}

// refusedAsArchived says err is the retired-content refusal for the content key.
func refusedAsArchived(err error, key string) bool {
	return refusedAs(err, charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT, key)
}

func refusedAs(err error, reason charactersv1.CharacterBlockedReason, key string) bool {
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Code() != connect.CodeFailedPrecondition {
		return false
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*charactersv1.CharacterBlocked); ok {
				return b.GetReason() == reason && b.GetContentKey() == key
			}
		}
	}
	return false
}
