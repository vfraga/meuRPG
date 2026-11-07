package leaktest

import (
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The roll minimums of the derived sheet (#757) follow the feature list of the sheet: a minimum is sent only with a
// feature that is in that list, with its key and name. The sheet is its owner's and the master's: another player gets no
// answer for it, so nothing of the feat, by name or by key. The owner and the master are the positive controls
// (checkCanariesAreReachable finds each canary in their answers).
const (
	// The names of the two feats carry a canary with letters outside a-f, so it cannot be found by chance in an id.
	toreFeatName = "Piso Zorvak4321"
	pensFeatName = "Piso Quimlo8765"
)

func TestRollMinimumsOnTheSheetReachOnlyWhoMayReadTheFeature(t *testing.T) {
	var torenFeat, pensFeat string
	rt := newRollsTableOf(t, false, false, func(rt *rollsTable) {
		torenFeat = rt.gift(&rt.toren, toreFeatName, "attack", "12")
		pensFeat = rt.gift(&rt.pens, pensFeatName, "check", "9")
	})
	w, m, caio, ana := rt.world, rt.master, rt.caio, rt.ana
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	w.secrets.add(&canary{needle: toreFeatName, kind: "minimum-sheet-name-toren", readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: torenFeat, kind: "minimum-sheet-key-toren", readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: pensFeatName, kind: "minimum-sheet-name-pens", readers: names([]*person{ana})})
	w.secrets.add(&canary{needle: pensFeat, kind: "minimum-sheet-key-pens", readers: names([]*person{ana})})

	rows := []read{
		{
			procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "Toren's sheet", allow: onlyCaio,
			req: func(w *world) proto.Message {
				return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}
			},
		},
		{
			procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "Pensantus's sheet", allow: onlyAna,
			req: func(w *world) proto.Message {
				return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
			},
		},
		{
			procedure: charactersv1connect.CharacterServiceListCharactersProcedure, allow: membersAndPending,
			req: func(w *world) proto.Message { return &charactersv1.ListCharactersRequest{CampaignId: w.campaign} },
		},
	}
	run := func(step string) {
		for _, r := range rows {
			t.Run(step+": "+r.name(), func(t *testing.T) { w.runRead(t, r, rt.got) })
		}
	}
	sheetOf := func(p *person, id string) *charactersv1.Character {
		return must(p.characters.GetCharacter(t.Context(), rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: id}))).GetCharacter()
	}
	// shown says that every minimum of the sheet has its feature in the feature list of the same answer.
	shown := func(who string, d *rulesv1.DerivedSheet) {
		for _, rm := range d.GetRollMinimums() {
			if !slices.ContainsFunc(d.GetFeatures(), func(f *rulesv1.Feature) bool { return f.GetKey() == rm.GetFeatureKey() }) {
				t.Errorf("%s reads the minimum %v of a feature that is not in their feature list", who, rm)
			}
		}
	}

	run("the feats are on the sheets")
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		d := sheetOf(p, w.toren.GetId()).GetDerived()
		got := d.GetRollMinimums()
		if len(got) != 1 || got[0].GetFeatureKey() != torenFeat || got[0].GetFeatureName() != toreFeatName || got[0].GetValue() != 12 || !slices.Equal(got[0].GetApplies(), []string{"attack"}) {
			t.Errorf("%s reads Toren's minimums %v, want the attack 12 of the feat (the positive control)", who, got)
		}
		shown(who, d)
	}
	for who, p := range map[string]*person{"the master": m, "Ana": ana} {
		d := sheetOf(p, w.pens.GetId()).GetDerived()
		got := d.GetRollMinimums()
		if len(got) != 1 || got[0].GetFeatureKey() != pensFeat || got[0].GetFeatureName() != pensFeatName || got[0].GetValue() != 9 {
			t.Errorf("%s reads Pensantus's minimums %v, want the check 9 of the feat (the positive control)", who, got)
		}
		shown(who, d)
	}
	// Another player's sheet is not found: nothing of it reaches them.
	for _, c := range []struct {
		p  *person
		id string
	}{{ana, w.toren.GetId()}, {caio, w.pens.GetId()}} {
		if res := c.p.call(charactersv1connect.CharacterServiceGetCharacterProcedure, &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: c.id}); res.ok() {
			t.Errorf("%s got another player's sheet: %s", c.p.name, shorten(res.body))
		}
	}
	// The master retires Toren's feat: the sheet keeps what the feature list keeps, and a minimum never outlives its feature.
	must(m.table.ArchiveTableEntry(t.Context(), rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: torenFeat})))
	run("the feat was retired")
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		d := sheetOf(p, w.toren.GetId()).GetDerived()
		shown(who, d)
		if !slices.ContainsFunc(d.GetFeatures(), func(f *rulesv1.Feature) bool { return f.GetKey() == torenFeat }) && len(d.GetRollMinimums()) != 0 {
			t.Errorf("%s reads the minimums %v of a retired feat that their feature list no longer has", who, d.GetRollMinimums())
		}
	}
	// The positive control: each canary of this test is in the answers of the master and of the one who may read it. (The
	// fixture's other canaries belong to the rolls' tests and are not read here.)
	t.Run("canaries are reachable", func(t *testing.T) {
		for _, c := range w.secrets.list {
			if !strings.HasPrefix(c.kind, "minimum-sheet-") {
				continue
			}
			if !rt.got.holds(m, c) {
				t.Errorf("the master reads %s in no answer", c)
			}
			for _, p := range []*person{ana, caio} {
				if c.readers[p.name] && !rt.got.holds(p, c) {
					t.Errorf("%s may read %s but finds it in no answer", p.name, c)
				}
			}
		}
	})
}
