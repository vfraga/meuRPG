package characters

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Preflight #253, A2 and A3: the sheet's stored issues hold identities, not sentences. A lost prerequisite is the same
// issue whatever its sentence says now (the level the character has, what the table shows), and a reader who is not the
// master does not receive the stored issues.

// lostFeatTable is the level-up table with a table class the players read, a feat that asks for 3 levels of it, and
// the character holding the feat (lost: Pensantus is a wizard).
type lostFeatTable struct {
	*hiddenNamesTable
	class, feat string
}

func newLostFeatTable(t *testing.T) *lostFeatTable {
	t.Helper()
	tb := newHiddenNamesTable(t)
	m := tb.master
	lt := &lostFeatTable{hiddenNamesTable: tb}
	lt.class = m.addEntry(t, tb.campaign, testClass(forgottenClass)).GetKey()
	lt.feat = m.addEntry(t, tb.campaign, &rulesv1.TableFeat{NamePt: "Pede a ordem", DescPt: []string{"Texto."}, Prerequisite: &rulesv1.FeatPrerequisite{
		ClassLevel: &rulesv1.FeatClassLevel{ClassKey: lt.class, Level: 3},
	}}).GetKey()
	lt.give(t, tb.pc)
	return lt
}

// give saves the feat on the character c as the master.
func (lt *lostFeatTable) give(t *testing.T, c *charactersv1.Character) {
	t.Helper()
	// A save first, so the sheet is known to fit the table as it is now: an entry made after the sheet's last save
	// would read as one that changed under it.
	current := lt.master.get(t, lt.campaign, c.GetId())
	current, err := lt.master.update(t, current, current.GetName(), current.GetSheet())
	if err != nil {
		t.Fatalf("the master's save before the feat: %v", err)
	}
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().FeatKeys = []string{lt.feat}
	if _, err := lt.master.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the feat: %v", err)
	}
}

func (lt *lostFeatTable) archive(t *testing.T, key string) {
	t.Helper()
	if _, err := lt.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: lt.campaign, Key: key})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
}

// storedIssues reads known_issues straight from the row.
func storedIssues(t *testing.T, h *harness, id string) []string {
	t.Helper()
	var doc string
	if err := h.pool.QueryRow(t.Context(), "SELECT sheet::text FROM characters WHERE id = $1", id).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	var sheet struct {
		Full struct {
			KnownIssues []string `json:"known_issues"`
		} `json:"full"`
	}
	if err := json.Unmarshal([]byte(doc), &sheet); err != nil {
		t.Fatal(err)
	}
	return sheet.Full.KnownIssues
}

func TestTheStoredIssuesHoldIdentitiesNotSentences(t *testing.T) {
	t.Parallel()
	lt := newLostFeatTable(t)
	stored := storedIssues(t, lt.h, lt.pc.GetId())
	want := "feat_prerequisite|full.feat_keys|lost:" + lt.feat
	found := false
	for _, id := range stored {
		found = found || id == want
		if strings.Contains(id, forgottenClass) || strings.Contains(id, " pede ") {
			t.Errorf("a stored issue holds a sentence: %q", id)
		}
	}
	if !found {
		t.Errorf("stored issues = %q, want %q among them", stored, want)
	}
	if got := lt.owner.get(t, lt.campaign, lt.pc.GetId()).GetSheet().GetFull().GetKnownIssues(); len(got) != 0 {
		t.Errorf("the owner receives the stored issues: %q", got)
	}
	if got := lt.master.get(t, lt.campaign, lt.pc.GetId()).GetSheet().GetFull().GetKnownIssues(); len(got) == 0 {
		t.Error("the master does not receive the stored issues")
	}
}

// Saved while the players read the class, then the master hides or archives it: no reader who is not the master gets
// its name or key, the owner and a player who takes the character from the reserve afterwards alike.
func TestAnEntryHiddenAfterTheSaveIsNamedToNoPlayer(t *testing.T) {
	t.Parallel()
	for _, how := range []string{"switched off", "archived"} {
		t.Run(how, func(t *testing.T) {
			t.Parallel()
			lt := newLostFeatTable(t)
			reserved := lt.master.reservedByMaster(t, lt.campaign, "Reservado")
			lt.give(t, reserved)
			if how == "archived" {
				lt.archive(t, lt.class)
			} else {
				lt.master.setOff(t, lt.campaign, true, lt.class)
			}
			wantNames(t, "the owner's character", lt.owner.get(t, lt.campaign, lt.pc.GetId()), false, forgottenClass, lt.class)
			wantNames(t, "the master's character", lt.master.get(t, lt.campaign, lt.pc.GetId()), true, forgottenClass)

			res, err := lt.other.claim(t, lt.master.link(t, reserved, 1))
			if err != nil {
				t.Fatalf("ClaimCharacter() error = %v", err)
			}
			wantNames(t, "the claim's answer", res, false, forgottenClass, lt.class)
			wantNames(t, "the character taken from the reserve", lt.other.get(t, lt.campaign, reserved.GetId()), false, forgottenClass, lt.class)
		})
	}
}

func changedKeys(c *charactersv1.Character) []string {
	var keys []string
	for _, ch := range c.GetDerived().GetChangedContent() {
		keys = append(keys, ch.GetKey())
	}
	return keys
}

func (lt *lostFeatTable) editFeatText(t *testing.T) {
	t.Helper()
	entry := lt.master.entries(t, lt.campaign)[lt.feat]
	edited := proto.CloneOf(bodyMessage(entry).(*rulesv1.TableFeat))
	edited.DescPt = []string{"Texto novo."}
	if _, err := lt.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(lt.campaign, lt.feat, entry.GetRevision(), edited))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
}

// An edit of the feat's text makes no notice; and none appears when the lost issue's sentence changes afterwards,
// because the asked class was hidden or the master's save changed what the character has.
func TestALostIssueWhoseSentenceChangesIsNotANewIssue(t *testing.T) {
	t.Parallel()
	t.Run("the asked entry is hidden after the edit", func(t *testing.T) {
		t.Parallel()
		lt := newLostFeatTable(t)
		lt.editFeatText(t)
		lt.master.setOff(t, lt.campaign, true, lt.class)
		for who, u := range map[string]*user{"the owner": lt.owner, "the master": lt.master} {
			if keys := changedKeys(u.get(t, lt.campaign, lt.pc.GetId())); len(keys) != 0 {
				t.Errorf("%s reads a changed entry %v for an issue the sheet already had", who, keys)
			}
		}
	})
	t.Run("the master's save changes what the character has", func(t *testing.T) {
		t.Parallel()
		tb := newHiddenNamesTable(t)
		m := tb.master
		lt := &lostFeatTable{hiddenNamesTable: tb}
		lt.feat = m.addEntry(t, tb.campaign, &rulesv1.TableFeat{NamePt: "Pede força", DescPt: []string{"Texto."}, Prerequisite: &rulesv1.FeatPrerequisite{
			Minimums: &rulesv1.AbilityScores{Strength: 18},
		}}).GetKey()
		lt.give(t, tb.pc)
		lt.editFeatText(t)
		current := m.get(t, tb.campaign, tb.pc.GetId())
		before := issueMessage(current, "feat_prerequisite")
		sheet := proto.CloneOf(current.GetSheet())
		sheet.GetFull().GetBaseScores().Strength++
		if _, err := m.update(t, current, current.GetName(), sheet); err != nil {
			t.Fatalf("the master's save of the score: %v", err)
		}
		read := tb.owner.get(t, tb.campaign, tb.pc.GetId())
		if after := issueMessage(read, "feat_prerequisite"); after == "" || after == before {
			t.Fatalf("the sentence did not change with the score (%q): the case tests nothing", after)
		}
		if keys := changedKeys(read); len(keys) != 0 {
			t.Errorf("the owner reads a changed entry %v for an issue the sheet already had", keys)
		}
	})
}

func issueMessage(c *charactersv1.Character, code string) string {
	for _, i := range c.GetDerived().GetIssues() {
		if i.GetCode() == code {
			return i.GetMessage()
		}
	}
	return ""
}

// setStoredIssues writes the character's stored known_issues straight into the row, as a sheet saved by an earlier
// version of the code holds them.
func setStoredIssues(t *testing.T, h *harness, id string, issues []string) {
	t.Helper()
	raw, err := json.Marshal(issues)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(t.Context(), "UPDATE characters SET sheet = jsonb_set(sheet, '{full,known_issues}', $2::jsonb) WHERE id = $1", id, string(raw)); err != nil {
		t.Fatal(err)
	}
}

// withoutCode is the stored issues without the ones of a code, and with the rows after them.
func withoutCode(stored []string, code string, rows ...string) []string {
	out := slices.Clone(rows)
	for _, id := range stored {
		if !strings.HasPrefix(id, code+"|") {
			out = append(out, id)
		}
	}
	return out
}

// A row saved by the code before this one holds the lost issue in its old wording: it is the same issue, so a change
// of the entry afterwards makes no notice and the master's "Fichas com aviso" does not count it.
func TestAStoredIssueInTheOldWordingIsTheSameIssue(t *testing.T) {
	t.Parallel()
	lt := newLostFeatTable(t)
	setStoredIssues(t, lt.h, lt.pc.GetId(), withoutCode(storedIssues(t, lt.h, lt.pc.GetId()), "feat_prerequisite",
		"feat_prerequisite|full.feat_keys[0]|O personagem não cumpre mais o pré-requisito de Pede a ordem: o talento só vale de novo quando cumprir."))
	lt.wantNoNotice(t, func(f *rulesv1.TableFeat) { f.DescPt = []string{"Texto novo."} })
}

// wantNoNotice applies the master's edit to the feat and checks nothing is flagged for the issue the sheet already had.
func (lt *lostFeatTable) wantNoNotice(t *testing.T, edit func(*rulesv1.TableFeat)) {
	t.Helper()
	entry := lt.master.entries(t, lt.campaign)[lt.feat]
	edited := proto.CloneOf(bodyMessage(entry).(*rulesv1.TableFeat))
	edit(edited)
	res, err := lt.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(lt.campaign, lt.feat, entry.GetRevision(), edited)))
	if err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	if a := res.Msg.GetAffectedCharacters(); len(a) != 0 {
		t.Errorf("\"Fichas com aviso\" counts %v for an issue the sheet already had", a)
	}
	if keys := changedKeys(lt.owner.get(t, lt.campaign, lt.pc.GetId())); len(keys) != 0 {
		t.Errorf("the owner reads a changed entry %v for an issue the sheet already had", keys)
	}
}

// The old wording names the entry, and the master may rename it before the next save: the row is still the same issue.
func TestAStoredIssueInTheOldWordingSurvivesTheRenameOfItsEntry(t *testing.T) {
	t.Parallel()
	t.Run("a feat", func(t *testing.T) {
		t.Parallel()
		lt := newLostFeatTable(t)
		setStoredIssues(t, lt.h, lt.pc.GetId(), withoutCode(storedIssues(t, lt.h, lt.pc.GetId()), "feat_prerequisite",
			"feat_prerequisite|full.feat_keys[0]|O personagem não cumpre mais o pré-requisito de Pede a ordem: o talento só vale de novo quando cumprir."))
		lt.wantNoNotice(t, func(f *rulesv1.TableFeat) { f.NamePt = "Pede a hierarquia" })
	})
	t.Run("a feat taken through a choice, stored without the pick's index", func(t *testing.T) {
		t.Parallel()
		lt := newLostFeatTable(t)
		lt.giveThroughAChoice(t)
		setStoredIssues(t, lt.h, lt.pc.GetId(), withoutCode(storedIssues(t, lt.h, lt.pc.GetId()), "feat_prerequisite",
			"feat_prerequisite|full.feature_choice_keys|O personagem não cumpre mais o pré-requisito de Pede a ordem: o talento só vale de novo quando cumprir."))
		lt.wantNoNotice(t, func(f *rulesv1.TableFeat) { f.NamePt = "Pede a hierarquia" })
	})
}

// giveThroughAChoice has the character take the feat through the background's choice, in place of the sheet's own
// list, and returns the stored issues of the character after the save.
func (lt *lostFeatTable) giveThroughAChoice(t *testing.T) {
	t.Helper()
	feature := lt.master.entries(t, lt.campaign)[lt.background].GetTableBackground().GetFeature().GetKey()
	current := lt.master.get(t, lt.campaign, lt.pc.GetId())
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: lt.background}
	sheet.GetFull().FeatKeys = nil
	sheet.GetFull().FeatureChoiceKeys = append(sheet.GetFull().FeatureChoiceKeys, feature+"#feat="+lt.feat)
	if _, err := lt.master.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the pick: %v", err)
	}
}

// A row in the old wording holds one issue. When another entry of the same list becomes lost after the row was
// written, that one is new to the sheet: the owner reads its notice and the master's "Fichas com aviso" counts the sheet.
func TestAStoredOldRowHoldsOneFeatAndNotASecondOneLostLater(t *testing.T) {
	t.Parallel()
	lt := newLostFeatTable(t)
	second := lt.master.addEntry(t, lt.campaign, &rulesv1.TableFeat{NamePt: "Pede a ordem de novo", DescPt: []string{"Texto."}}).GetKey()
	current := lt.master.get(t, lt.campaign, lt.pc.GetId())
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().FeatKeys = []string{lt.feat, second}
	if _, err := lt.master.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the feats: %v", err)
	}
	setStoredIssues(t, lt.h, lt.pc.GetId(), withoutCode(storedIssues(t, lt.h, lt.pc.GetId()), "feat_prerequisite",
		"feat_prerequisite|full.feat_keys[0]|O personagem não cumpre mais o pré-requisito de Pede a ordem: o talento só vale de novo quando cumprir."))

	entry := lt.master.entries(t, lt.campaign)[second]
	edited := proto.CloneOf(bodyMessage(entry).(*rulesv1.TableFeat))
	edited.Prerequisite = &rulesv1.FeatPrerequisite{ClassLevel: &rulesv1.FeatClassLevel{ClassKey: lt.class, Level: 3}}
	res, err := lt.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(lt.campaign, second, entry.GetRevision(), edited)))
	if err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	if a := res.Msg.GetAffectedCharacters(); len(a) != 1 {
		t.Errorf("\"Fichas com aviso\" counts %v, want the sheet whose second feat became lost", a)
	}
	if keys := changedKeys(lt.owner.get(t, lt.campaign, lt.pc.GetId())); !slices.Contains(keys, second) || slices.Contains(keys, lt.feat) {
		t.Errorf("the owner's changed entries = %v, want only the feat that became lost (%s)", keys, second)
	}
}
