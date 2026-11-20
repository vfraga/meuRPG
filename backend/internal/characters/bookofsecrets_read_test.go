package characters

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// TestTheOwnerKeepsReadingARitualOfTheirBookAfterTheMasterSwitchesItOff: the ritual a warlock holds in the Book of
// Ancient Secrets is a key of the sheet (RN-23), so its owner reads it after the master switched it off, and another
// player does not.
func TestTheOwnerKeepsReadingARitualOfTheirBookAfterTheMasterSwitchesItOff(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	row := spellSwitchRows()[0]
	if _, err := tb.create(tb.ana, row.sheet(rules.ScopedChoice(row.choiceKey, row.held), rules.ScopedChoice(row.choiceKey, row.allowed))); err != nil {
		t.Fatalf("a warlock with %s in the book: %v", row.held, err)
	}
	tb.master.setOff(t, tb.campaign, true, row.held)
	read := func(u *user) error {
		_, err := u.content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: tb.campaign, SpellKey: row.held}))
		return err
	}
	if err := read(tb.ana); err != nil {
		t.Errorf("the owner of the sheet that holds %s in the book: error = %v, want it read", row.held, err)
	}
	if err := read(tb.caio); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another player: error = %v, want not_found", err)
	}
	if err := read(tb.master); err != nil {
		t.Errorf("the master: error = %v, want it read", err)
	}
}

// TestTheOwnerIsToldWhenARitualOfTheirBookStopsBeingARitual: the master edits a table ritual that a warlock holds in
// the Book of Ancient Secrets so that it is no longer a ritual, and the owner's sheet carries the notice of changed
// content for it.
func TestTheOwnerIsToldWhenARitualOfTheirBookStopsBeingARitual(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	row := spellSwitchRows()[0]
	ritual := testSpell("Selo de teste", "class:wizard")
	ritual.Ritual = true
	ritual.Duration = &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_TIMED, Amount: 10, Unit: rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_MINUTE}
	entry := tb.master.addEntry(t, tb.campaign, ritual)
	c, err := tb.create(tb.ana, row.sheet(rules.ScopedChoice(row.choiceKey, row.held), rules.ScopedChoice(row.choiceKey, entry.GetKey())))
	if err != nil {
		t.Fatalf("a warlock with a table ritual in the book: %v", err)
	}
	if n := len(c.GetDerived().GetChangedContent()); n != 0 {
		t.Fatalf("a fresh sheet has changed content: %v", c.GetDerived().GetChangedContent())
	}
	plain := proto.Clone(ritual).(*rulesv1.TableSpell)
	plain.Ritual = false
	if _, err := tb.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(tb.campaign, entry.GetKey(), entry.GetRevision(), plain))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	got := tb.ana.get(t, tb.campaign, c.GetId())
	ch := got.GetDerived().GetChangedContent()
	if len(ch) != 1 || ch[0].GetKey() != entry.GetKey() {
		t.Fatalf("changed content = %v, want the ritual that is not one anymore", ch)
	}
	for _, m := range ch[0].GetMessages() {
		if strings.Contains(strings.ToLower(m), "desligad") {
			t.Errorf("the notice says %q, want only that the spell changed", m)
		}
	}
}
