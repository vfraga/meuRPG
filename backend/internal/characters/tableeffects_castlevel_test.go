package characters

import (
	"testing"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The cast level of a granted_spell (MR-025): the stored field reaches the engine's effect, survives the stored JSON
// and a pack file, and the sheet's grant says it.

func castLevelFeat() *rulesv1.TableFeat {
	return &rulesv1.TableFeat{
		NamePt: "Graça de teste", DescPt: []string{"Um dom de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "granted_spell", Spell: "spell:bless", Ability: "cha", CastLevel: 3}},
	}
}

func TestACastLevelReachesTheEngineAndTheStoredJSONKeepsIt(t *testing.T) {
	t.Parallel()
	feat := castLevelFeat()
	if got := tableEffectsOf(feat.GetEffects())[0].CastLevel; got != 3 {
		t.Errorf("the engine's cast level = %d, want 3", got)
	}
	data, err := storedData(feat)
	if err != nil {
		t.Fatalf("storedData() error = %v", err)
	}
	pack, unknown, top := readPackJSON([]byte(`{"entries":[{"key":"feat:graca@mesa","table_feat":` + string(data) + `}]}`))
	if len(unknown) != 0 || len(top) != 0 {
		t.Fatalf("readPackJSON() found unknown fields %v and pack violations %v, want none", unknown, top)
	}
	if got := pack.GetEntries()[0].GetTableFeat().GetEffects()[0].GetCastLevel(); got != 3 {
		t.Errorf("the cast level after the stored JSON and a pack file = %d, want 3", got)
	}
}

func TestASheetGrantSaysItsCastLevel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ level, want int32 }{{0, 0}, {1, 1}, {3, 3}} {
		g := grantedSpellToProto(rules.GrantedSpell{Spell: rules.SpellEntry{Key: "spell:bless", Level: 1}, CastLevel: int(tc.level)})
		if g.GetCastLevel() != tc.want {
			t.Errorf("the sheet's cast level for %d = %d, want %d", tc.level, g.GetCastLevel(), tc.want)
		}
	}
}
