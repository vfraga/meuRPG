package characters

import (
	"slices"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// ChangedContent.codes (#580): the code of the issue each sentence of `messages` comes from, one per sentence, in the same
// order. Every ChangedContent of every answer is built by addChangedContent (tablechoice.go), which access.go calls once,
// in the one function that makes a Character for a read (get, list of a campaign's characters with their sheets, the
// previews and the level-up answers that return a Character, equip and every other write that answers with the
// character): so the invariant is pinned on the builder's output, read through the paths of the tests below (the sheet
// read of the owner and of the master, the shield notice, the lost-prerequisite sentence, a class with two issues).

// codesMatchMessages fails for every changed entry of c whose codes are not one per sentence, or hold an empty code.
func codesMatchMessages(t *testing.T, who string, c *charactersv1.Character) {
	t.Helper()
	for _, ch := range c.GetDerived().GetChangedContent() {
		if len(ch.GetCodes()) != len(ch.GetMessages()) {
			t.Errorf("%s: %s has %d codes for %d sentences: %q / %q", who, ch.GetKey(), len(ch.GetCodes()), len(ch.GetMessages()), ch.GetCodes(), ch.GetMessages())
		}
		if slices.Contains(ch.GetCodes(), "") {
			t.Errorf("%s: %s has an empty code: %q", who, ch.GetKey(), ch.GetCodes())
		}
	}
}

// changedOf is the changed entry of key in c, or nil.
func changedOf(c *charactersv1.Character, key string) *rulesv1.ChangedContent {
	for _, ch := range c.GetDerived().GetChangedContent() {
		if ch.GetKey() == key {
			return ch
		}
	}
	return nil
}

func TestChangedContentCodesTellTheShieldNoticeApart(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, versatile)
	tb.edit(t, twoHanded)
	for who, u := range map[string]*user{"the owner": tb.owner, "the master": tb.master} {
		got := u.get(t, tb.campaign, tb.pc.GetId())
		codesMatchMessages(t, who, got)
		ch := changedOf(got, tb.weapon.GetKey())
		if ch == nil || !slices.Equal(ch.GetCodes(), []string{rules.IssueShieldIdle}) {
			t.Errorf("%s reads codes %v, want [%s]", who, ch.GetCodes(), rules.IssueShieldIdle)
		}
	}
}

func TestChangedContentCodesFollowTheSentencesOfAnEntryWithTwoIssues(t *testing.T) {
	t.Parallel()
	f := newClassFlow(t)
	icaro := f.davi.create(t, f.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", f.icaroSheet())
	f.h.lockSheets(f.campaign)
	icaro = f.davi.get(t, f.campaign, icaro.GetId())
	for to := 2; to <= 3; to++ {
		icaro, _ = f.levelUp(t, f.davi, icaro, f.guardian.GetKey(), f.path.GetKey())
	}
	key := f.guardian.GetKey()
	f.edit(t, key, func(b proto.Message) {
		c := b.(*rulesv1.TableClass)
		c.SkillChoose = 1
		c.Casting.PreparedMax = "1"
	})
	for who, u := range map[string]*user{"the owner": f.davi, "the master": f.master} {
		got := u.get(t, f.campaign, icaro.GetId())
		codesMatchMessages(t, who, got)
		ch := changedOf(got, key)
		if ch == nil || len(ch.GetMessages()) < 2 {
			t.Fatalf("%s reads %v, want two sentences on the class", who, ch)
		}
		// Each sentence is told with the code of the issue it comes from: the skill sentence by skill_count, the
		// spell one by spell_count.
		for i, m := range ch.GetMessages() {
			if i >= len(ch.GetCodes()) {
				break
			}
			want := rules.IssueSpellCount
			if m == "Guardião do Vale agora dá 1 perícia no nível 1; esta ficha tem 2." {
				want = rules.IssueSkillCount
			}
			if ch.GetCodes()[i] != want {
				t.Errorf("%s: %q has code %q, want %q", who, m, ch.GetCodes()[i], want)
			}
		}
		if !slices.Contains(ch.GetCodes(), rules.IssueSkillCount) || !slices.Contains(ch.GetCodes(), rules.IssueSpellCount) {
			t.Errorf("%s reads codes %v, want a skill_count and a spell_count", who, ch.GetCodes())
		}
	}
}

func TestChangedContentCodesOfALostPrerequisiteAreTheSameForTheOwnerAndTheMaster(t *testing.T) {
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
	for who, u := range map[string]*user{"the owner": hk.owner, "the master": m} {
		got := u.get(t, hk.campaign, hk.pc.GetId())
		codesMatchMessages(t, who, got)
		ch := changedOf(got, feat.GetKey())
		if ch == nil || !slices.Equal(ch.GetCodes(), []string{rules.IssueFeatPrerequisite}) {
			t.Errorf("%s reads codes %v, want [%s]", who, ch.GetCodes(), rules.IssueFeatPrerequisite)
		}
	}
}
