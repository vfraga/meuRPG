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

// The table's switches apply to every spell choice, on every path that writes a sheet: the sheet editor's update, a
// preview and the completion of a choice, for a spell the SRD has (switched off) and for a spell of the table (retired).

func TestTheSwitchesApplyToEverySpellChoiceOnTheSheetEditor(t *testing.T) {
	t.Parallel()
	for _, row := range spellSwitchRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tb := newChoiceTable(t)
			scoped := func(spell string) string { return rules.ScopedChoice(row.choiceKey, spell) }
			with := func(spell string) *charactersv1.CharacterSheet {
				return row.sheet(append(slices.Clone(row.others), scoped(spell))...)
			}
			pc, err := tb.create(tb.ana, with(row.held))
			if err != nil {
				t.Fatalf("a sheet that holds %s: %v", row.held, err)
			}
			tb.master.setOff(t, tb.campaign, true, row.held, row.fresh)

			// The sheet that holds the pick is saved again, and a new pick is refused, on the editable sheet.
			if _, err := tb.ana.update(t, pc, "Renomeada", with(row.held)); err != nil {
				t.Errorf("saving the sheet that holds %s: %v", row.held, err)
			}
			pc = tb.ana.get(t, tb.campaign, pc.GetId())
			if pc, err = tb.master.update(t, pc, pc.GetName(), row.sheet(row.others...)); err != nil {
				t.Fatalf("the master takes the pick out: %v", err)
			}
			for _, spell := range []string{row.held, row.fresh} {
				if _, err := tb.ana.update(t, pc, pc.GetName(), with(spell)); !refusedAsSwitchedOff(err, spell) {
					t.Errorf("the player's update that picks %s: error %v, want SWITCHED_OFF_CONTENT", spell, err)
				}
			}
			if got, err := tb.ana.update(t, pc, pc.GetName(), with(row.allowed)); err != nil {
				t.Errorf("the player's update that picks %s: %v", row.allowed, err)
			} else {
				pc = got
			}
			if _, err := tb.master.update(t, pc, pc.GetName(), with(row.fresh)); err != nil {
				t.Errorf("the master's update that picks %s: %v", row.fresh, err)
			}
		})
	}
}

func TestTheSwitchesApplyToEverySpellChoiceOfARetiredTableSpell(t *testing.T) {
	t.Parallel()
	for _, row := range spellSwitchRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tb := newChoiceTable(t)
			spell := tb.master.addEntry(t, tb.campaign, row.table()).GetKey()
			scoped := func(spell string) string { return rules.ScopedChoice(row.choiceKey, spell) }
			with := func(spell string) *charactersv1.CharacterSheet {
				return row.sheet(append(slices.Clone(row.others), scoped(spell))...)
			}
			pc, err := tb.create(tb.ana, with(spell))
			if err != nil {
				t.Fatalf("a sheet that holds the table spell: %v", err)
			}
			if _, err := tb.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: spell})); err != nil {
				t.Fatalf("ArchiveTableEntry() error = %v", err)
			}

			// The sheet that holds the pick keeps it.
			res, err := tb.preview(tb.ana, pc.GetId(), with(spell))
			if err != nil || !slices.Contains(choiceByKey(res, row.choiceKey).GetPicked(), spell) {
				t.Errorf("the sheet that holds the retired spell: error %v, want the pick kept", err)
			}
			if pc, err = tb.ana.update(t, pc, "Renomeada", with(spell)); err != nil {
				t.Errorf("saving the sheet that holds the retired spell: %v", err)
			}

			// A new pick of it is refused for everyone, the master too.
			if pc, err = tb.master.update(t, pc, pc.GetName(), row.sheet(row.others...)); err != nil {
				t.Fatalf("the master takes the pick out: %v", err)
			}
			for name, u := range map[string]*user{"the player": tb.ana, "the master": tb.master} {
				if _, err := tb.preview(u, pc.GetId(), with(spell)); !refusedAsArchived(err, spell) {
					t.Errorf("%s's preview that picks the retired spell: error %v, want ARCHIVED_CONTENT", name, err)
				}
				if _, err := u.update(t, pc, pc.GetName(), with(spell)); !refusedAsArchived(err, spell) {
					t.Errorf("%s's update that picks the retired spell: error %v, want ARCHIVED_CONTENT", name, err)
				}
			}
			if _, err := tb.complete(tb.ana, pc, pick(row.choiceKey, spell)); connect.CodeOf(err) != connect.CodeFailedPrecondition {
				t.Errorf("completing the choice with the retired spell: error %v, want it refused", err)
			}
		})
	}
}

// levelUpRefusalOf is the refusal a failed level-up carries.
func levelUpRefusalOf(err error) *charactersv1.LevelUpRefusal {
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		return nil
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if r, ok := v.(*charactersv1.LevelUpRefusal); ok {
				return r
			}
		}
	}
	return nil
}

// TestTheSwitchesApplyToTheSpellPicksOfALevelUp: the rituals of the Book of Ancient Secrets are picked in the level that
// takes it, or at the next level as a late choice.
func TestTheSwitchesApplyToTheSpellPicksOfALevelUp(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 0)
	c := tomeWarlock(t, tb)
	row := spellSwitchRows()[0]
	retired := tb.master.addEntry(t, tb.campaign, row.table()).GetKey()
	if _, err := tb.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: retired})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	tb.master.setOff(t, tb.campaign, true, row.held)
	rituals := func(spells ...string) []string {
		out := []string{testBook}
		for _, s := range spells {
			out = append(out, rules.ScopedChoice(row.choiceKey, s))
		}
		return out
	}
	levelUp := func(c *charactersv1.Character, known string, picks []string, late []string) error {
		_, err := tb.levelUp(tb.other, c, &charactersv1.LevelUpChoices{
			ClassKey: "class:warlock", KnownSpellKeys: []string{known}, FeatureChoiceKeys: picks, LateChoiceKeys: late,
			HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
		})
		return err
	}
	refusal := func(name string, err error, want charactersv1.LevelUpRefusalReason) {
		t.Helper()
		if r := levelUpRefusalOf(err); r == nil || r.GetReason() != want {
			t.Errorf("%s: error %v, want the refusal %v", name, err, want)
		}
	}
	// A ritual the character never held and the table offers no player is answered as one that does not exist.
	bare := charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE
	refusal("the level that picks a switched-off ritual", levelUp(c, "spell:hold-person", rituals(row.held, row.allowed), nil), bare)
	refusal("the level that picks a retired ritual", levelUp(c, "spell:hold-person", rituals(retired, row.allowed), nil), bare)

	// The level that takes the book without its rituals, and then the late choice.
	if err := levelUp(c, "spell:hold-person", rituals(), nil); err != nil {
		t.Fatalf("the level that takes the book: %v", err)
	}
	c = tb.other.get(t, tb.campaign, c.GetId())
	late := func(spells ...string) []string { return rituals(spells...)[1:] }
	refusal("the late choice that picks a switched-off ritual", levelUp(c, "spell:fear", nil, late(row.held, row.allowed)), bare)
	refusal("the late choice that picks a retired ritual", levelUp(c, "spell:fear", nil, late(retired, row.allowed)), bare)
	if err := levelUp(c, "spell:fear", nil, late(row.allowed, "spell:comprehend-languages")); err != nil {
		t.Errorf("the late choice with rituals that stay on: %v", err)
	}
}
