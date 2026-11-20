package characters

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The cast level of a granted_spell (MR-025) through the table's editor calls: refused at its field when the
// entry is written, and kept by the stored row, the content pack and the import. These tests need the
// database (MEURPG_TEST_DATABASE_URL).

func castLevelGrantFeat(name string, e *rulesv1.TableEffect) *rulesv1.TableFeat {
	e.Type, e.Ability = "granted_spell", "cha"
	return &rulesv1.TableFeat{NamePt: name, DescPt: []string{"Um dom de teste."}, Effects: []*rulesv1.TableEffect{e}}
}

func TestMR025_ACastLevelIsRefusedAtItsFieldWhenTheEntryIsWritten(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	const field = "table_feat.effects[0].cast_level"
	for _, tc := range []struct {
		name string
		body proto.Message
	}{
		{"below the spell's own level", castLevelGrantFeat("Abaixo", &rulesv1.TableEffect{Spell: "spell:misty-step", CastLevel: 1})},
		{"above 9", castLevelGrantFeat("Acima", &rulesv1.TableEffect{Spell: "spell:bless", CastLevel: 10})},
		{"negative", castLevelGrantFeat("Negativo", &rulesv1.TableEffect{Spell: "spell:bless", CastLevel: -1})},
		{"on a cantrip", castLevelGrantFeat("Truque", &rulesv1.TableEffect{Spell: "spell:light", CastLevel: 2})},
		{"with ritual_only", castLevelGrantFeat("Ritual", &rulesv1.TableEffect{Spell: "spell:detect-magic", RitualOnly: true, CastLevel: 2})},
		{"on another effect type", &rulesv1.TableFeat{NamePt: "Nota", DescPt: []string{"Um dom de teste."}, Effects: []*rulesv1.TableEffect{{Type: "note", TextPt: "texto", CastLevel: 2}}}},
	} {
		vs := master.refused(t, campaign, tc.body)
		if len(vs) != 1 || vs[0].GetField() != field || vs[0].GetReason() != "bad_value" {
			t.Errorf("%s: violations %v, want one at %s (bad_value)", tc.name, vs, field)
		}
	}
	if got := master.entries(t, campaign); len(got) != 0 {
		t.Errorf("the refused entries wrote %d entries, want none", len(got))
	}
}

// The cast level survives the stored row, the pack's export and its import into another campaign, as a message
// and as the pack's JSON file.
func TestMR025_ACastLevelRoundTripsThroughTheStoredRowAndThePack(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	made := pt.master.addEntry(t, pt.from, castLevelGrantFeat("Graça da cura", &rulesv1.TableEffect{Spell: "spell:cure-wounds", CastLevel: 4}))
	castLevelOf := func(e *rulesv1.TableEntry) int32 {
		return e.GetTableFeat().GetEffects()[0].GetCastLevel()
	}
	if got := castLevelOf(made); got != 4 {
		t.Fatalf("the created entry has cast level %d, want 4", got)
	}
	if got := castLevelOf(pt.master.entries(t, pt.from)[made.GetKey()]); got != 4 {
		t.Errorf("the stored row reads back with cast level %d, want 4", got)
	}
	pack := pt.export(t, pt.from)
	if len(pack.GetEntries()) != 1 || castLevelOf(pack.GetEntries()[0]) != 4 {
		t.Fatalf("the exported pack = %v, want one entry with cast level 4", pack.GetEntries())
	}
	pt.apply(t, pt.to, pack, "")
	if got := castLevelOf(pt.master.entries(t, pt.to)[made.GetKey()]); got != 4 {
		t.Errorf("the imported entry has cast level %d, want 4", got)
	}
}

// Editing a table spell to a level above the cast level of a feat's grant of it is refused, because that grant would be left
// with a cast level below the spell's own. The violation points at the grant's cast_level field in the feat, and the spell
// keeps its old level.
func TestMR025_RaisingATableSpellAboveAGrantsCastLevelIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	spell := master.addEntry(t, campaign, testSpell("Raio de Teste", "class:wizard"))
	master.addEntry(t, campaign, castLevelGrantFeat("Graça do raio", &rulesv1.TableEffect{Spell: spell.GetKey(), CastLevel: 2}))
	higher := proto.Clone(spell.GetTableSpell()).(*rulesv1.TableSpell)
	higher.Level = 3
	_, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, spell.GetKey(), spell.GetRevision(), cloneWithKeys(higher, bodyMessage(spell)))))
	vs := violationsOfErr(t, err)
	if len(vs) != 1 || vs[0].GetField() != "table_feat.effects[0].cast_level" || vs[0].GetReason() != "bad_value" {
		t.Fatalf("violations %v, want one at table_feat.effects[0].cast_level (bad_value)", vs)
	}
	if got := master.entries(t, campaign)[spell.GetKey()].GetTableSpell().GetLevel(); got != 1 {
		t.Errorf("the spell has level %d after the refused edit, want 1", got)
	}
}
