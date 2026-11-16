package characters

import (
	"testing"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// An entry stored before the list of a choice of tools was checked by kind still loads: the key of another kind leaves the
// pool, the master reads it on the entry (a write that holds it is refused: rules, Overlay.Strict).
func TestAStoredToolChoiceThatListsAKeyOfAnotherKindLoadsAndTheMasterReadsIt(t *testing.T) {
	t.Parallel()
	srd := loadRules(t)
	const key = "background:cais-do-farol@mesa"
	body := testBackground("Cais do farol")
	body.Feature = &rulesv1.TableFeature{NamePt: "Rede do cais", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{{
		Type: "choice", Choice: "tool", Count: 1, From: []string{"proficiency:smiths-tools", "skill:athletics"},
	}}}
	entries := []entryRow{storedEntry(rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND, key, "Cais do farol", body)}

	c, out, err := buildTolerant(srd, entries, 1, nil)
	if err != nil || len(out) != 0 || len(c.Refused()) != 0 {
		t.Fatalf("buildTolerant() = %v, %v, %v; want the entry loaded and nothing left out", c, out, err)
	}
	got := c.Stored()
	if len(got) != 1 || got[0].Key != key || got[0].Field != "table_background.feature.effects[0].from" {
		t.Fatalf("Stored() = %+v, want one issue at the list of the entry", got)
	}
	master := (&tableListing{master: true, content: c}).response()
	if len(master.GetLoadedWithIssues()) != 1 || master.GetLoadedWithIssues()[0].GetKey() != key || len(master.GetLeftOut()) != 0 {
		t.Errorf("the master reads %v left out %v, want the issue on the entry", master.GetLoadedWithIssues(), master.GetLeftOut())
	}
	if player := (&tableListing{content: c}).response(); len(player.GetLoadedWithIssues()) != 0 {
		t.Errorf("a player reads %v, want nothing", player.GetLoadedWithIssues())
	}
}
