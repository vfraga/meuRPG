package characters

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// RN-23 for the class level, the pact, the spell and the option a feat asks (#580): a player's level-up options name
// none of them, in a key, a name or a condition; the master reads them as the positive control.

const (
	forgottenClass  = "Ordem Esquecida"
	forgottenPact   = "Pacto do Eco"
	forgottenSpell  = "Raio Esquecido"
	forgottenOption = "Estilo Esquecido"
)

// hiddenKnowledge is the table of the hidden-names fixture plus a hidden class, pact, spell and option, and a feat that
// asks for each.
type hiddenKnowledge struct {
	*hiddenNamesTable
	class, pact, spell, style          string
	byClass, byPact, bySpell, byOption string
}

func newHiddenKnowledge(t *testing.T) *hiddenKnowledge {
	t.Helper()
	tb := newHiddenNamesTable(t)
	m := tb.master
	class := m.addEntry(t, tb.campaign, testClass(forgottenClass)).GetKey()
	pact := m.addEntry(t, tb.campaign, &rulesv1.TableOption{NamePt: forgottenPact, Set: "pact_boon", DescPt: []string{"Texto."}}).GetKey()
	spell := m.addEntry(t, tb.campaign, testSpell(forgottenSpell, "class:wizard")).GetKey()
	style := m.addEntry(t, tb.campaign, &rulesv1.TableOption{NamePt: forgottenOption, Set: "fighting_style", DescPt: []string{"Texto."}}).GetKey()
	feat := func(name string, p *rulesv1.FeatPrerequisite) string {
		return m.addEntry(t, tb.campaign, &rulesv1.TableFeat{NamePt: name, DescPt: []string{"Texto."}, Prerequisite: p}).GetKey()
	}
	hk := &hiddenKnowledge{hiddenNamesTable: tb, class: class, pact: pact, spell: spell, style: style}
	hk.byClass = feat("Pede a ordem", &rulesv1.FeatPrerequisite{ClassLevel: &rulesv1.FeatClassLevel{ClassKey: class, Level: 3}})
	hk.byPact = feat("Pede o pacto", &rulesv1.FeatPrerequisite{RequiresPact: pact})
	hk.bySpell = feat("Pede a magia", &rulesv1.FeatPrerequisite{RequiresSpellKey: spell})
	hk.byOption = feat("Pede o estilo", &rulesv1.FeatPrerequisite{RequiresOptionKeys: []string{style}})
	m.setOff(t, tb.campaign, true, class, pact, spell, style)
	return hk
}

func TestAPlayersLevelUpNamesNoHiddenClassPactSpellOrOption(t *testing.T) {
	t.Parallel()
	hk := newHiddenKnowledge(t)
	tb, m := hk.hiddenNamesTable, hk.master
	class, pact, spell, style := hk.class, hk.pact, hk.spell, hk.style
	byClass, byPact, bySpell, byOption := hk.byClass, hk.byPact, hk.bySpell, hk.byOption

	player, err := tb.options(tb.owner, tb.pc)
	if err != nil {
		t.Fatal(err)
	}
	master, err := tb.options(m, tb.pc)
	if err != nil {
		t.Fatal(err)
	}
	words := []string{forgottenClass, forgottenPact, forgottenSpell, forgottenOption, class, pact, spell, style}
	wantNames(t, "the master's options", master, true, forgottenClass, forgottenPact, forgottenSpell, forgottenOption, class, pact, spell, style)
	wantNames(t, "the player's options", player, false, words...)
	for _, key := range []string{byClass, byPact, bySpell, byOption} {
		got := optionOf(player, key)
		if got == nil {
			t.Fatalf("the feat %s is not offered to the player", key)
		}
		if got.GetQualifies() {
			t.Errorf("%s: the player qualifies for a feat asking for what they lack", key)
		}
		if len(got.GetUnmet()) != 1 || !got.GetUnmet()[0].GetHidden() || got.GetUnmet()[0].GetKey() != "" || got.GetUnmet()[0].GetNamePt() != "" {
			t.Errorf("%s: unmet = %v, want one hidden condition with no key or name", key, got.GetUnmet())
		}
		if got.GetUnmet()[0].GetValue() != 0 {
			t.Errorf("%s: the hidden condition still sends the number %d", key, got.GetUnmet()[0].GetValue())
		}
		if control := optionOf(master, key); len(control.GetUnmet()) != 1 || control.GetUnmet()[0].GetHidden() || control.GetUnmet()[0].GetKey() == "" {
			t.Errorf("%s: the master's unmet = %v, want the condition itself", key, control.GetUnmet())
		}
	}
	// The player's creation cards say the same, with the words of the "Pede:" line.
	sheet := pensantusSheet()
	res, err := tb.owner.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
		CampaignId: tb.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("PreviewChoices() error = %v", err)
	}
	wantNames(t, "the player's cards", res.Msg, false, words...)
}

// The choice list of a feat's options names no hidden entry its options ask for (option prerequisites name no hidden entry).
func TestAPlayersChoiceListNamesNoHiddenEntryAnOptionAsksFor(t *testing.T) {
	t.Parallel()
	tb := newHiddenNamesTable(t)
	m := tb.master
	class := m.addEntry(t, tb.campaign, testClass(forgottenClass)).GetKey()
	style := m.addEntry(t, tb.campaign, &rulesv1.TableOption{NamePt: forgottenOption, Set: "fighting_style", DescPt: []string{"Texto."}}).GetKey()
	spell := m.addEntry(t, tb.campaign, testSpell(forgottenSpell, "class:wizard")).GetKey()
	feat := m.addEntry(t, tb.campaign, &rulesv1.TableFeat{
		NamePt: "Posturas do vale", DescPt: []string{"Texto."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1}},
		Options: []*rulesv1.TableFeature{{NamePt: "Postura exigente", DescPt: []string{"Texto."}, Prerequisite: &rulesv1.OptionPrerequisite{
			ClassLevel: &rulesv1.OptionClassLevel{ClassKey: class, Level: 3}, RequiresOptionKeys: []string{style}, RequiresSpellKey: spell,
		}}},
	}).GetKey()
	m.setOff(t, tb.campaign, true, class, style, spell)

	sheet := pensantusSheet()
	sheet.GetFull().FeatKeys = []string{feat}
	preview := func(u *user, kind charactersv1.CharacterKind) *charactersv1.PreviewChoicesResponse {
		res, err := u.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{CampaignId: tb.campaign, Kind: kind, Sheet: sheet}))
		if err != nil {
			t.Fatalf("PreviewChoices() error = %v", err)
		}
		return res.Msg
	}
	master := preview(m, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY)
	player := preview(tb.owner, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER)
	wantNames(t, "the master's choice list", master, true, "Postura exigente", forgottenClass, forgottenOption, forgottenSpell)
	wantNames(t, "the player's choice list", player, true, "Postura exigente")
	wantNames(t, "the player's choice list", player, false, forgottenClass, forgottenOption, forgottenSpell, class, style, spell)
}

// The refusal a player gets for a feat that asks for a hidden entry names no hidden key and no hidden name.
func TestAPlayersLevelUpRefusalNamesNoHiddenEntry(t *testing.T) {
	t.Parallel()
	hk := newHiddenKnowledge(t)
	for name, feat := range map[string]string{"class": hk.byClass, "pact": hk.byPact, "spell": hk.bySpell, "option": hk.byOption} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ch := pensantusLevelUp()
			ch.FeatKey, ch.AbilityIncrease = feat, nil
			_, err := hk.levelUp(hk.owner, hk.pc, ch)
			if err == nil {
				t.Fatal("a player took a feat whose prerequisite they do not meet")
			}
			for _, w := range []string{forgottenClass, forgottenPact, forgottenSpell, forgottenOption, hk.class, hk.pact, hk.spell, hk.style} {
				if strings.Contains(err.Error(), w) {
					t.Errorf("the refusal names %q: %v", w, err)
				}
			}
		})
	}
}

// Board 580-05 with RN-23: the issue of a lost option or a lost feat names the character, and a missing condition on an
// entry the players may not read is "uma condição que a mesa não mostra" for the owner and for another player; the master
// reads it named. The sentence is built for the reader: the sheet's stored issues name no hidden entry either.
func TestALostPrerequisiteIsToldToEachReaderWithoutHiddenNames(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"option", "feat"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			hk := newHiddenKnowledge(t)
			m := hk.master
			current := m.get(t, hk.campaign, hk.pc.GetId())
			sheet := proto.CloneOf(current.GetSheet())
			code, entry := "feat_prerequisite", "Pede a ordem"
			if kind == "option" {
				feat := m.addEntry(t, hk.campaign, &rulesv1.TableFeat{
					NamePt: "Posturas do vale", DescPt: []string{"Texto."},
					Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1}},
					Options: []*rulesv1.TableFeature{{NamePt: "Postura exigente", DescPt: []string{"Texto."}, Prerequisite: &rulesv1.OptionPrerequisite{
						ClassLevel: &rulesv1.OptionClassLevel{ClassKey: hk.class, Level: 3},
					}}},
				})
				sheet.GetFull().FeatKeys = []string{feat.GetKey()}
				sheet.GetFull().FeatureChoiceKeys = []string{feat.GetTableFeat().GetOptions()[0].GetKey()}
				code, entry = "option_prerequisite", "Postura exigente"
			} else {
				sheet.GetFull().FeatKeys = []string{hk.byClass}
			}
			if _, err := m.update(t, current, current.GetName(), sheet); err != nil {
				t.Fatalf("the master's save of a sheet whose %s asks for a class it lacks: %v", kind, err)
			}
			name, ends := current.GetName(), map[string]string{"option": "a opção", "feat": "o talento"}[kind]
			hidden := entry + " pede uma condição que a mesa não mostra: " + ends + " só vale de novo quando " + name + " cumprir."
			named := entry + " pede " + forgottenClass + " 3, e " + name + " não tem nível de " + forgottenClass + ": " + ends + " só vale de novo quando " + name + " cumprir."
			for _, r := range []struct {
				who  string
				u    *user
				want string
			}{{"the owner", hk.owner, hidden}, {"the master", m, named}} {
				read := r.u.get(t, hk.campaign, hk.pc.GetId())
				var message string
				for _, i := range read.GetDerived().GetIssues() {
					if i.GetCode() == code {
						message = i.GetMessage()
					}
				}
				if message != r.want {
					t.Errorf("%s reads %q, want %q", r.who, message, r.want)
				}
				if r.u != m {
					wantNames(t, r.who+"'s sheet", read, false, forgottenClass, hk.class)
				}
			}
			// Another player does not read the sheet at all, and the party list tells them nothing of the condition.
			_, err := hk.other.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: hk.campaign, CharacterId: hk.pc.GetId()}))
			if connect.CodeOf(err) != connect.CodeNotFound {
				t.Errorf("another player's read = %v, want not found", err)
			}
			for _, sum := range hk.other.list(t, hk.campaign) {
				wantNames(t, "another player's list", sum, false, forgottenClass, hk.class)
			}
			stored := m.get(t, hk.campaign, hk.pc.GetId()).GetSheet().GetFull().GetKnownIssues()
			for _, id := range stored {
				if strings.Contains(id, forgottenClass) || strings.Contains(id, name) {
					t.Errorf("a stored issue names %q or the character: %q", forgottenClass, id)
				}
			}
		})
	}
}

// After the master's edit, the lost line is told in "A mudança na mesa" (changed content) the same way: masked for the
// owner, named for the master.
func TestALostPrerequisiteAfterAnEditIsToldInTheChangedEntryPerReader(t *testing.T) {
	t.Parallel()
	hk := newHiddenKnowledge(t)
	m := hk.master
	feat := m.addEntry(t, hk.campaign, &rulesv1.TableFeat{NamePt: "Pede depois", DescPt: []string{"Texto."}})
	current := m.get(t, hk.campaign, hk.pc.GetId())
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().FeatKeys = []string{feat.GetKey()}
	if _, err := m.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the feat: %v", err)
	}
	edited := proto.CloneOf(bodyMessage(feat).(*rulesv1.TableFeat))
	edited.Prerequisite = &rulesv1.FeatPrerequisite{ClassLevel: &rulesv1.FeatClassLevel{ClassKey: hk.class, Level: 3}}
	if _, err := m.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(hk.campaign, feat.GetKey(), feat.GetRevision(), edited))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	name := current.GetName()
	tail := ": o talento só vale de novo quando " + name + " cumprir."
	for _, r := range []struct {
		who  string
		u    *user
		want string
	}{
		{"the owner", hk.owner, "Pede depois pede uma condição que a mesa não mostra" + tail},
		{"the master", m, "Pede depois pede " + forgottenClass + " 3, e " + name + " não tem nível de " + forgottenClass + tail},
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
