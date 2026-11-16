package characters

import (
	"slices"
	"strconv"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Preflight #580, round 2: a lost option stored in the old wording (the sentence that named the entry and, for a
// condition on a class, the class), as a row saved before the sheet kept identities holds it.

// lostOptionTable is the hidden-knowledge table with a feat whose option asks for 3 levels of the hidden class, taken
// by the character: the option is lost and its sentence, for the master, names the class.
type lostOptionTable struct {
	*hiddenKnowledge
	feat   *rulesv1.TableEntry
	option string
	// at is the option's place in the sheet's picks.
	at int
}

func newLostOptionTable(t *testing.T) *lostOptionTable {
	t.Helper()
	lo := &lostOptionTable{hiddenKnowledge: newHiddenKnowledge(t)}
	m := lo.master
	lo.feat = m.addEntry(t, lo.campaign, &rulesv1.TableFeat{
		NamePt: "Posturas do vale", DescPt: []string{"Texto."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1}},
		Options: []*rulesv1.TableFeature{{NamePt: "Postura exigente", DescPt: []string{"Texto."}}},
	})
	lo.option = lo.feat.GetTableFeat().GetOptions()[0].GetKey()
	current := m.get(t, lo.campaign, lo.pc.GetId())
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().FeatKeys = []string{lo.feat.GetKey()}
	sheet.GetFull().FeatureChoiceKeys = append(sheet.GetFull().FeatureChoiceKeys, lo.option)
	saved, err := m.update(t, current, current.GetName(), sheet)
	if err != nil {
		t.Fatalf("the master's save of the option: %v", err)
	}
	lo.at = slices.Index(saved.GetSheet().GetFull().GetFeatureChoiceKeys(), lo.option)
	if lo.at < 0 {
		t.Fatalf("the pick was not kept: %v", saved.GetSheet().GetFull().GetFeatureChoiceKeys())
	}
	return lo
}

// edit applies the master's edit to the feat and returns the answer.
func (lo *lostOptionTable) edit(t *testing.T, edit func(*rulesv1.TableFeat)) *rulesv1.UpdateTableEntryResponse {
	t.Helper()
	entry := lo.master.entries(t, lo.campaign)[lo.feat.GetKey()]
	edited := proto.CloneOf(bodyMessage(entry).(*rulesv1.TableFeat))
	edit(edited)
	res, err := lo.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(lo.campaign, lo.feat.GetKey(), entry.GetRevision(), edited)))
	if err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	return res.Msg
}

// storeOld replaces the stored lost option by the row of the old wording with the sentence, the way the earlier code
// wrote it.
func (lo *lostOptionTable) storeOld(t *testing.T, name, sentence string) {
	t.Helper()
	row := "option_prerequisite|full.feature_choice_keys[" + strconv.Itoa(lo.at) + "]|O personagem não cumpre mais o pré-requisito de " + name + ": " + sentence
	setStoredIssues(t, lo.h, lo.pc.GetId(), withoutCode(storedIssues(t, lo.h, lo.pc.GetId()), "option_prerequisite", row))
}

func (lo *lostOptionTable) asksTheClass(f *rulesv1.TableFeat) {
	f.Options[0].Prerequisite = &rulesv1.OptionPrerequisite{ClassLevel: &rulesv1.OptionClassLevel{ClassKey: lo.class, Level: 3}}
}

// The old sentence of an option names the class it asks for. The row is the sheet's own bookkeeping: the owner reads
// no name of the hidden class anywhere in the character, and the lost option is no new issue.
func TestAStoredOptionIssueNamingAHiddenClassIsToldToNoPlayer(t *testing.T) {
	t.Parallel()
	lo := newLostOptionTable(t)
	lo.edit(t, lo.asksTheClass)
	lo.storeOld(t, "Postura exigente", "precisa de nível 3 de "+forgottenClass+".")
	read := lo.owner.get(t, lo.campaign, lo.pc.GetId())
	wantNames(t, "the owner's character", read, false, forgottenClass, lo.class)
	if keys := changedKeys(read); len(keys) != 0 {
		t.Errorf("the owner reads a changed entry %v for an issue the sheet already had", keys)
	}
	wantNames(t, "the master's character", lo.master.get(t, lo.campaign, lo.pc.GetId()), true, forgottenClass)
}

// The master renames the option before the next save: the row in the old wording still holds its issue.
func TestAStoredOptionIssueSurvivesTheRenameOfItsOption(t *testing.T) {
	t.Parallel()
	lo := newLostOptionTable(t)
	lo.edit(t, lo.asksTheClass)
	lo.storeOld(t, "Postura exigente", "precisa de nível 3 de "+forgottenClass+".")
	res := lo.edit(t, func(f *rulesv1.TableFeat) { f.Options[0].NamePt = "Postura rigorosa" })
	if a := res.GetAffectedCharacters(); len(a) != 0 {
		t.Errorf("\"Fichas com aviso\" counts %v for an issue the sheet already had", a)
	}
	if keys := changedKeys(lo.owner.get(t, lo.campaign, lo.pc.GetId())); len(keys) != 0 {
		t.Errorf("the owner reads a changed entry %v for an issue the sheet already had", keys)
	}
}

// A row in the old wording holds one issue: a second option of the feat that becomes lost afterwards is new to the sheet.
func TestAStoredOldRowHoldsOneOptionAndNotASecondOneLostLater(t *testing.T) {
	t.Parallel()
	lo := newLostOptionTable(t)
	m := lo.master
	// A second option, picked too, and the first one lost with its row in the old wording.
	lo.edit(t, func(f *rulesv1.TableFeat) {
		f.Options = append(f.Options, &rulesv1.TableFeature{NamePt: "Postura firme", DescPt: []string{"Texto."}})
	})
	second := lo.master.entries(t, lo.campaign)[lo.feat.GetKey()].GetTableFeat().GetOptions()[1].GetKey()
	current := m.get(t, lo.campaign, lo.pc.GetId())
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().FeatureChoiceKeys = append(sheet.GetFull().FeatureChoiceKeys, second)
	if _, err := m.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the second option: %v", err)
	}
	lo.edit(t, lo.asksTheClass)
	lo.storeOld(t, "Postura exigente", "precisa de nível 3 de "+forgottenClass+".")

	res := lo.edit(t, func(f *rulesv1.TableFeat) {
		f.Options[1].Prerequisite = &rulesv1.OptionPrerequisite{ClassLevel: &rulesv1.OptionClassLevel{ClassKey: lo.class, Level: 3}}
	})
	if a := res.GetAffectedCharacters(); len(a) != 1 {
		t.Errorf("\"Fichas com aviso\" counts %v, want the sheet whose second option became lost", a)
	}
	read := lo.owner.get(t, lo.campaign, lo.pc.GetId())
	var got []string
	for _, ch := range read.GetDerived().GetChangedContent() {
		if ch.GetKey() == lo.feat.GetKey() {
			got = ch.GetMessages()
		}
	}
	want := "Postura firme pede uma condição que a mesa não mostra: a opção só vale de novo quando " + read.GetName() + " cumprir."
	if len(got) != 1 || got[0] != want {
		t.Errorf("the owner reads the changed feat as %q, want only [%q]", got, want)
	}
}
