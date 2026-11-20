package rules

import (
	"slices"
	"testing"
)

// Preflight #580, round 2: a row stored in the old wording ("O personagem não cumpre mais o pré-requisito de <nome>:
// ...", as main stored it) is the same issue whatever the entry is called now, and wherever its pick stands.
func TestARowInTheOldWordingIsTheLostIssueWhateverTheEntryIsCalledNow(t *testing.T) {
	t.Parallel()
	const start = "O personagem não cumpre mais o pré-requisito de "
	feat := Issue{Code: IssueFeatPrerequisite, Field: "full.feat_keys[2]", Lost: &LostPrerequisite{Key: "feat:x@mesa", EntryPT: "Novo nome"}}
	choiceFeat := Issue{Code: IssueFeatPrerequisite, Field: "full.feature_choice_keys[1]", Lost: &LostPrerequisite{Key: "feat:x@mesa", EntryPT: "Novo nome"}}
	option := Issue{Code: IssueOptionPrerequisite, Field: "full.feature_choice_keys[1]", Lost: &LostPrerequisite{Key: "feature:o@mesa", EntryPT: "Novo nome", Option: true}}
	cases := []struct {
		name   string
		is     Issue
		stored string
		want   bool
	}{
		{"a feat, as it was named", feat, "feat_prerequisite|full.feat_keys[2]|" + start + "Novo nome: o talento só vale de novo quando cumprir.", true},
		{"a feat, renamed", feat, "feat_prerequisite|full.feat_keys[2]|" + start + "Nome antigo: o talento só vale de novo quando cumprir.", true},
		{"a feat, its pick has moved", feat, "feat_prerequisite|full.feat_keys[0]|" + start + "Nome antigo: o talento só vale de novo quando cumprir.", true},
		{"a feat taken through a choice, stored without the index", choiceFeat, "feat_prerequisite|full.feature_choice_keys|" + start + "Nome antigo: o talento só vale de novo quando cumprir.", true},
		{"an option, renamed", option, "option_prerequisite|full.feature_choice_keys[1]|" + start + "Nome antigo: precisa de nível 5 de Guerreiro.", true},
		{"an option, its pick has moved", option, "option_prerequisite|full.feature_choice_keys[3]|" + start + "Nome antigo: precisa de nível 5 de Guerreiro.", true},
		{"another code", feat, "option_prerequisite|full.feat_keys[2]|" + start + "Nome antigo: o talento só vale de novo quando cumprir.", false},
		{"another list", feat, "feat_prerequisite|full.feature_choice_keys|" + start + "Nome antigo: o talento só vale de novo quando cumprir.", false},
		{"another sentence", feat, "feat_prerequisite|full.feat_keys[2]|O talento escolhido não existe no conteúdo 1.", false},
		{"the identity of an entry", feat, "feat_prerequisite|full.feat_keys|lost:feat:y@mesa", false},
	}
	for _, tc := range cases {
		if got := KnownIssues([]string{tc.stored}, []Issue{tc.is})[0]; got != tc.want {
			t.Errorf("%s: KnownIssues([%q]) = %v, want %v", tc.name, tc.stored, got, tc.want)
		}
	}
}

const oldStart = "O personagem não cumpre mais o pré-requisito de "

func lostOf(code, field, key, name string) Issue {
	return Issue{Code: code, Field: field, Lost: &LostPrerequisite{Key: key, EntryPT: name, Option: code == IssueOptionPrerequisite}}
}

// A stored row in the old wording holds one issue: it is taken by the entry it names, then by a renamed entry of the
// list, and never covers a second lost entry of the list.
func TestARowInTheOldWordingIsTakenOnce(t *testing.T) {
	t.Parallel()
	a := lostOf(IssueOptionPrerequisite, "full.feature_choice_keys[0]", "feature:a@mesa", "Alfa")
	b := lostOf(IssueOptionPrerequisite, "full.feature_choice_keys[1]", "feature:b@mesa", "Beta")
	rowOf := func(index, name string) string {
		return IssueOptionPrerequisite + "|full.feature_choice_keys[" + index + "]|" + oldStart + name + ": precisa de nível 5 de Guerreiro."
	}
	cases := []struct {
		name   string
		stored []string
		issues []Issue
		want   []bool
	}{
		{"the row of A and a second lost B", []string{rowOf("0", "Alfa")}, []Issue{a, b}, []bool{true, false}},
		{"the row of B, listed after A", []string{rowOf("1", "Beta")}, []Issue{a, b}, []bool{false, true}},
		{"the exact name wins over the order", []string{rowOf("0", "Beta")}, []Issue{a, b}, []bool{false, true}},
		{"A renamed: its row covers A and not B, by its place", []string{rowOf("0", "Nome antigo")}, []Issue{a, b}, []bool{true, false}},
		{"A renamed and its pick moved: the row covers the first one", []string{rowOf("5", "Nome antigo")}, []Issue{a, b}, []bool{true, false}},
		{"two rows, two lost entries", []string{rowOf("0", "Alfa"), rowOf("1", "Antigo")}, []Issue{a, b}, []bool{true, true}},
		{"one row of A, A twice-named by a stored identity too", []string{rowOf("0", "Alfa"), a.ID()}, []Issue{a, b}, []bool{true, false}},
	}
	for _, tc := range cases {
		got := KnownIssues(tc.stored, tc.issues)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: known = %v, want %v", tc.name, got, tc.want)
		}
	}
	// The same for feats, in the list of the sheet's own feats and in the picks of a choice (stored without a place).
	fa := lostOf(IssueFeatPrerequisite, "full.feat_keys[0]", "feat:a@mesa", "Alfa")
	fb := lostOf(IssueFeatPrerequisite, "full.feat_keys[1]", "feat:b@mesa", "Beta")
	row := IssueFeatPrerequisite + "|full.feat_keys[0]|" + oldStart + "Alfa: o talento só vale de novo quando cumprir."
	if got := KnownIssues([]string{row}, []Issue{fa, fb}); !slices.Equal(got, []bool{true, false}) {
		t.Errorf("feats: known = %v, want [true false]", got)
	}
	ca := lostOf(IssueFeatPrerequisite, "full.feature_choice_keys[1]", "feat:a@mesa", "Alfa")
	cb := lostOf(IssueFeatPrerequisite, "full.feature_choice_keys[3]", "feat:b@mesa", "Beta")
	row = IssueFeatPrerequisite + "|full.feature_choice_keys|" + oldStart + "Beta: o talento só vale de novo quando cumprir."
	if got := KnownIssues([]string{row}, []Issue{ca, cb}); !slices.Equal(got, []bool{false, true}) {
		t.Errorf("feats taken through a choice: known = %v, want [false true]", got)
	}
}

// A stored sentence is replaced by the identity of the issue it holds, once, and the rows that hold none stay.
func TestIdentitiesOfReplacesTheRowsThatHoldAnIssue(t *testing.T) {
	t.Parallel()
	a := lostOf(IssueOptionPrerequisite, "full.feature_choice_keys[0]", "feature:a@mesa", "Alfa")
	b := lostOf(IssueOptionPrerequisite, "full.feature_choice_keys[1]", "feature:b@mesa", "Beta")
	row := IssueOptionPrerequisite + "|full.feature_choice_keys[0]|" + oldStart + "Alfa: precisa de nível 5 de Guerreiro."
	other := IssueUnknownKey + "|full.feat_keys[0]|chave desconhecida"
	got := IdentitiesOf([]string{other, row}, []Issue{a, b})
	if want := []string{other, a.ID()}; !slices.Equal(got, want) {
		t.Errorf("identities = %v, want %v", got, want)
	}
}

// A known limit: a stored old-wording row cannot tell an entry that was renamed from one that was fixed. The row of an
// entry fixed with no sheet save (it is no longer lost, and was not renamed) is taken by another entry of the same code
// and list that becomes lost later, so that entry's notice does not show until the next save writes identities.
func TestARowOfAFixedEntryIsTakenByAnotherEntryLostLaterAKnownLimit(t *testing.T) {
	t.Parallel()
	b := lostOf(IssueOptionPrerequisite, "full.feature_choice_keys[1]", "feature:b@mesa", "Beta")
	rowOfFixedAlfa := IssueOptionPrerequisite + "|full.feature_choice_keys[0]|" + oldStart + "Alfa: precisa de nível 5 de Guerreiro."
	// Alfa is not lost any more and is not renamed, so only Beta is lost: Beta takes Alfa's row and is told as known.
	if got := KnownIssues([]string{rowOfFixedAlfa}, []Issue{b}); !slices.Equal(got, []bool{true}) {
		t.Errorf("known = %v, want [true] (the known limit: Beta took the row of the fixed Alfa)", got)
	}
}
