package characters

import (
	"slices"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// #580, board 580-05 and RN-23: the option a character picked from the choice of a feat that is lost reads "pede o talento X,
// que <nome> não aplica agora". When the master then archives that feat or switches it off, the sentence is told for each
// reader without naming what the reader can no longer read.

const lentStyle = "feature:ranger-fighting-style-defense"

// lostFeatWithAnOption is a table whose character holds the feat "Estilo emprestado" (it offers the SRD style as its choice)
// and the style picked from it, then loses the feat's prerequisite on the master's edit.
func lostFeatWithAnOption(t *testing.T) (*hiddenNamesTable, string, string) {
	t.Helper()
	tb := newHiddenNamesTable(t)
	m := tb.master
	feat := m.addEntry(t, tb.campaign, &rulesv1.TableFeat{
		NamePt: "Estilo emprestado", DescPt: []string{"Texto."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1, From: []string{lentStyle}}},
	}).GetKey()
	current := m.get(t, tb.campaign, tb.pc.GetId())
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().FeatKeys = []string{feat}
	sheet.GetFull().FeatureChoiceKeys = append(sheet.GetFull().FeatureChoiceKeys, lentStyle)
	if _, err := m.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the feat and its style: %v", err)
	}
	entry := m.entries(t, tb.campaign)[feat]
	edited := proto.CloneOf(bodyMessage(entry).(*rulesv1.TableFeat))
	edited.Prerequisite = &rulesv1.FeatPrerequisite{Level: 20}
	if _, err := m.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(tb.campaign, feat, entry.GetRevision(), edited))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	return tb, feat, current.GetName()
}

func optionLost(c *charactersv1.Character) []string {
	var out []string
	for _, i := range c.GetDerived().GetIssues() {
		if i.GetCode() == "option_prerequisite" {
			out = append(out, i.GetMessage())
		}
	}
	return out
}

func TestTheOptionOfALostFeatTheMasterHidAfterwardsIsToldToEachReader(t *testing.T) {
	t.Parallel()
	for _, how := range []string{"archived", "switched off"} {
		t.Run(how, func(t *testing.T) {
			t.Parallel()
			tb, feat, name := lostFeatWithAnOption(t)
			m := tb.master
			plain := optionLost(tb.owner.get(t, tb.campaign, tb.pc.GetId()))
			if len(plain) != 1 {
				t.Fatalf("the owner reads %q before the feat is hidden, want the one option_prerequisite", plain)
			}
			if how == "archived" {
				if _, err := m.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: feat})); err != nil {
					t.Fatalf("ArchiveTableEntry() error = %v", err)
				}
			} else {
				m.setOff(t, tb.campaign, true, feat)
			}
			want := []string{"Estilo de Luta: Defesa pede o talento Estilo emprestado, que " + name + " não aplica agora: a opção só vale de novo quando " + name + " cumprir."}
			for who, u := range map[string]*user{"the owner": tb.owner, "the master": m} {
				got := optionLost(u.get(t, tb.campaign, tb.pc.GetId()))
				if !slices.Equal(got, want) {
					t.Errorf("%s reads %q, want %q", who, got, want)
				}
			}
			// The owner may read the name: the sheet holds the feat. Another player, who does not, reads nothing of it.
			if own := tb.owner.get(t, tb.campaign, tb.pc.GetId()); !slices.Contains(own.GetSheet().GetFull().GetFeatKeys(), feat) {
				t.Errorf("the owner's sheet lists the feats %v, want %q among them", own.GetSheet().GetFull().GetFeatKeys(), feat)
			}
			for _, sum := range tb.other.list(t, tb.campaign) {
				wantNames(t, "another player's list", sum, false, "Estilo emprestado", feat)
			}
		})
	}
}
