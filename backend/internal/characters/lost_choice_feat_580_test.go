package characters

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Preflight #253, A4: a feat taken through a choice that loses its prerequisite on the master's edit. The issue names
// the pick ("full.feature_choice_keys[n]"), which is what the sheet marks the feat's row by, and the owner reads
// "O talento mudou." for it as for a feat of the sheet's own list.
func TestALostFeatTakenThroughAChoiceReachesTheChangedNotice(t *testing.T) {
	t.Parallel()
	tb := newHiddenNamesTable(t)
	m := tb.master
	feat := m.addEntry(t, tb.campaign, &rulesv1.TableFeat{NamePt: "Braço firme", DescPt: []string{"Texto."}}).GetKey()
	feature := m.entries(t, tb.campaign)[tb.background].GetTableBackground().GetFeature().GetKey()

	current := m.get(t, tb.campaign, tb.pc.GetId())
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: tb.background}
	pick := feature + "#feat=" + feat
	sheet.GetFull().FeatureChoiceKeys = append(sheet.GetFull().FeatureChoiceKeys, pick)
	saved, err := m.update(t, current, current.GetName(), sheet)
	if err != nil {
		t.Fatalf("the master's save of the pick: %v", err)
	}
	at := slices.Index(saved.GetSheet().GetFull().GetFeatureChoiceKeys(), pick)
	if at < 0 {
		t.Fatalf("the pick was not kept: %v", saved.GetSheet().GetFull().GetFeatureChoiceKeys())
	}
	if msg := issueMessage(saved, "feat_prerequisite"); msg != "" {
		t.Fatalf("the feat is lost before the edit: %q", msg)
	}

	entry := m.entries(t, tb.campaign)[feat]
	edited := proto.CloneOf(bodyMessage(entry).(*rulesv1.TableFeat))
	edited.Prerequisite = &rulesv1.FeatPrerequisite{Minimums: &rulesv1.AbilityScores{Strength: 20}}
	if _, err := m.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(tb.campaign, feat, entry.GetRevision(), edited))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}

	read := tb.owner.get(t, tb.campaign, tb.pc.GetId())
	var field string
	for _, is := range read.GetDerived().GetIssues() {
		if is.GetCode() == "feat_prerequisite" {
			field = is.GetField()
		}
	}
	if want := "full.feature_choice_keys[" + strconv.Itoa(at) + "]"; field != want {
		t.Errorf("the issue's field = %q, want %q", field, want)
	}
	if keys := changedKeys(read); !slices.Contains(keys, feat) {
		t.Errorf("the owner's changed entries = %v, want the feat %q", keys, feat)
	}
	for _, ch := range read.GetDerived().GetChangedContent() {
		if ch.GetKey() == feat && !strings.Contains(strings.Join(ch.GetMessages(), " "), "Braço firme pede") {
			t.Errorf("the notice does not say what the feat asks now: %v", ch.GetMessages())
		}
	}
}
