package characters

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Preflight #253, A5 and A6: what each reader gets of a lost prerequisite, by the kind of the condition the players
// may not read and by the call that tells it.

// giveFeats saves the feats on the table's character as the master and returns the character's name.
func (tb *hiddenNamesTable) giveFeats(t *testing.T, feats ...string) string {
	t.Helper()
	current := tb.master.get(t, tb.campaign, tb.pc.GetId())
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().FeatKeys = feats
	if _, err := tb.master.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the feats %v: %v", feats, err)
	}
	return current.GetName()
}

func lostMessages(d *rulesv1.DerivedSheet) []string {
	var out []string
	for _, i := range d.GetIssues() {
		if i.GetCode() == "feat_prerequisite" || i.GetCode() == "option_prerequisite" {
			out = append(out, i.GetMessage())
		}
	}
	return out
}

func wantOneLost(t *testing.T, who string, d *rulesv1.DerivedSheet, want string) {
	t.Helper()
	if got := lostMessages(d); len(got) != 1 || got[0] != want {
		t.Errorf("%s reads %q, want [%q]", who, got, want)
	}
}

func TestALostFeatIsToldToEachReaderByTheKindOfItsHiddenCondition(t *testing.T) {
	t.Parallel()
	const hidden = "uma condição que a mesa não mostra"
	cases := []struct {
		name string
		// feat is the lost feat's key and its name; owner and master what each reads after "<feat> pede ", with %s
		// for the character's name; words what the owner's whole character must not carry.
		pick func(*hiddenKnowledge) (feat, featName, owner, master string, words []string)
	}{
		{"a pact", func(hk *hiddenKnowledge) (string, string, string, string, []string) {
			return hk.byPact, "Pede o pacto", hidden, "a Dádiva do Pacto " + forgottenPact + ", que %s não tem mais", []string{forgottenPact, hk.pact}
		}},
		{"a spell", func(hk *hiddenKnowledge) (string, string, string, string, []string) {
			return hk.bySpell, "Pede a magia", hidden, "a magia " + forgottenSpell + ", que %s não conhece mais", []string{forgottenSpell, hk.spell}
		}},
		{"an option", func(hk *hiddenKnowledge) (string, string, string, string, []string) {
			return hk.byOption, "Pede o estilo", hidden, forgottenOption + ", que %s não tem mais", []string{forgottenOption, hk.style}
		}},
		{"a race", func(hk *hiddenKnowledge) (string, string, string, string, []string) {
			return hk.onlyHidden, onlyHiddenName, hidden, "ser " + hiddenRaceName + ", que %s não é", []string{hiddenRaceName, hk.race}
		}},
		{"a race among races the players read", func(hk *hiddenKnowledge) (string, string, string, string, []string) {
			return hk.mixed, mixedName, "ser " + visibleRaceName + ", que %s não é", "ser " + hiddenRaceName + " ou " + visibleRaceName + ", que %s não é", []string{hiddenRaceName, hk.race}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hk := newHiddenKnowledge(t)
			feat, featName, owner, master, words := tc.pick(hk)
			name := hk.giveFeats(t, feat)
			say := func(asked string) string {
				return featName + " pede " + strings.ReplaceAll(asked, "%s", name) + ": o talento só vale de novo quando " + name + " cumprir."
			}
			read := hk.owner.get(t, hk.campaign, hk.pc.GetId())
			wantOneLost(t, "the owner", read.GetDerived(), say(owner))
			wantNames(t, "the owner's character", read, false, words...)
			wantOneLost(t, "the master", hk.master.get(t, hk.campaign, hk.pc.GetId()).GetDerived(), say(master))
		})
	}
}

// A feat that asks not to have a feature is lost by a character who has it: the sheet holds the feature, so its
// owner reads it named even when the table hides it from the players who do not have it.
func TestALostFeatThatAsksNotToHaveAFeatureTheSheetHas(t *testing.T) {
	t.Parallel()
	tb := newHiddenNamesTable(t)
	name := tb.giveFeats(t, tb.feat, tb.lacksFeat)
	want := lacksHiddenFeat + " pede não ter " + hiddenFeatName + ", e " + name + " tem: o talento só vale de novo quando " + name + " cumprir."
	wantOneLost(t, "the owner", tb.owner.get(t, tb.campaign, tb.pc.GetId()).GetDerived(), want)
	wantOneLost(t, "the master", tb.master.get(t, tb.campaign, tb.pc.GetId()).GetDerived(), want)
	// A player who does not have it reads nothing of it.
	for _, sum := range tb.other.list(t, tb.campaign) {
		wantNames(t, "another player's list", sum, false, hiddenFeatName, tb.feat)
	}
}

// The previews tell the sentence as the character's own read does: with the character's name, and a hidden condition
// named for the master only.
func TestThePreviewsTellALostPrerequisiteToTheirReader(t *testing.T) {
	t.Parallel()
	hk := newHiddenKnowledge(t)
	name := hk.giveFeats(t, hk.byClass)
	tail := ": o talento só vale de novo quando " + name + " cumprir."
	masked := "Pede a ordem pede uma condição que a mesa não mostra" + tail
	named := "Pede a ordem pede " + forgottenClass + " 3, e " + name + " não tem nível de " + forgottenClass + tail

	levelUp := func(u *user) *rulesv1.DerivedSheet {
		res, err := u.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{CampaignId: hk.campaign, CharacterId: hk.pc.GetId(), Choices: pensantusLevelUp()}))
		if err != nil {
			t.Fatalf("PreviewLevelUp() error = %v", err)
		}
		if res.Msg.GetRefusal() != nil {
			t.Fatalf("PreviewLevelUp() refusal = %v: a lost prerequisite the sheet already had refuses no level", res.Msg.GetRefusal())
		}
		return res.Msg.GetAfter()
	}
	owner := levelUp(hk.owner)
	wantOneLost(t, "the owner's level-up preview", owner, masked)
	wantNames(t, "the owner's level-up preview", owner, false, forgottenClass, hk.class)
	wantOneLost(t, "the master's level-up preview", levelUp(hk.master), named)

	current := hk.master.get(t, hk.campaign, hk.pc.GetId())
	res, err := hk.master.preview(t, hk.campaign, hk.pc.GetId(), charactersv1.CharacterKind_CHARACTER_KIND_UNSPECIFIED, current.GetSheet())
	if err != nil {
		t.Fatalf("PreviewCharacter() error = %v", err)
	}
	wantOneLost(t, "the master's sheet preview", res.GetDerived(), named)
}

// The changed notice of an option says the lost line per reader, as a feat's does.
func TestALostOptionAfterAnEditIsToldInTheChangedEntryPerReader(t *testing.T) {
	t.Parallel()
	hk := newHiddenKnowledge(t)
	m := hk.master
	feat := m.addEntry(t, hk.campaign, &rulesv1.TableFeat{
		NamePt: "Posturas do vale", DescPt: []string{"Texto."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1}},
		Options: []*rulesv1.TableFeature{{NamePt: "Postura exigente", DescPt: []string{"Texto."}}},
	})
	current := m.get(t, hk.campaign, hk.pc.GetId())
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().FeatKeys = []string{feat.GetKey()}
	sheet.GetFull().FeatureChoiceKeys = append(sheet.GetFull().FeatureChoiceKeys, feat.GetTableFeat().GetOptions()[0].GetKey())
	if _, err := m.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the option: %v", err)
	}
	edited := proto.CloneOf(bodyMessage(feat).(*rulesv1.TableFeat))
	edited.Options[0].Prerequisite = &rulesv1.OptionPrerequisite{ClassLevel: &rulesv1.OptionClassLevel{ClassKey: hk.class, Level: 3}}
	if _, err := m.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(hk.campaign, feat.GetKey(), feat.GetRevision(), edited))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	name := current.GetName()
	tail := ": a opção só vale de novo quando " + name + " cumprir."
	for _, r := range []struct {
		who  string
		u    *user
		want string
	}{
		{"the owner", hk.owner, "Postura exigente pede uma condição que a mesa não mostra" + tail},
		{"the master", m, "Postura exigente pede " + forgottenClass + " 3, e " + name + " não tem nível de " + forgottenClass + tail},
	} {
		read := r.u.get(t, hk.campaign, hk.pc.GetId())
		var got []string
		for _, ch := range read.GetDerived().GetChangedContent() {
			if ch.GetKey() == feat.GetKey() {
				got = ch.GetMessages()
			}
		}
		if len(got) != 1 || got[0] != r.want {
			t.Errorf("%s reads the changed feat as %q, want [%q]", r.who, got, r.want)
		}
		if r.u != m {
			wantNames(t, r.who+"'s sheet", read, false, forgottenClass, hk.class)
		}
	}
}

// A player's own preview of a sheet they may still edit (a draft) tells the lost feat as their sheet read does: the
// character's name, and no name of the class the players may not read. The master's preview of the same sheet names it.
func TestAPlayersSheetPreviewTellsALostPrerequisiteWithoutAHiddenName(t *testing.T) {
	t.Parallel()
	hk := newHiddenKnowledge(t)
	// The fixture's character is locked (RN-01): a character made after that is a draft its player edits.
	draft := hk.other.create(t, hk.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Rascunho", pensantusSheet())
	if !draft.GetCanEdit() {
		t.Fatalf("the draft cannot be edited by its player: state %v", draft.GetState())
	}
	sheet := proto.CloneOf(draft.GetSheet())
	sheet.GetFull().FeatKeys = []string{hk.byClass}
	if _, err := hk.master.update(t, draft, draft.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the feat: %v", err)
	}
	read := hk.other.get(t, hk.campaign, draft.GetId())
	tail := ": o talento só vale de novo quando Rascunho cumprir."
	masked := "Pede a ordem pede uma condição que a mesa não mostra" + tail
	named := "Pede a ordem pede " + forgottenClass + " 3, e Rascunho não tem nível de " + forgottenClass + tail

	player, err := hk.other.preview(t, hk.campaign, draft.GetId(), charactersv1.CharacterKind_CHARACTER_KIND_UNSPECIFIED, read.GetSheet())
	if err != nil {
		t.Fatalf("the player's PreviewCharacter() error = %v", err)
	}
	wantOneLost(t, "the player's sheet preview", player.GetDerived(), masked)
	wantNames(t, "the player's sheet preview", player, false, forgottenClass, hk.class)

	master, err := hk.master.preview(t, hk.campaign, draft.GetId(), charactersv1.CharacterKind_CHARACTER_KIND_UNSPECIFIED, read.GetSheet())
	if err != nil {
		t.Fatalf("the master's PreviewCharacter() error = %v", err)
	}
	wantOneLost(t, "the master's sheet preview", master.GetDerived(), named)
}
