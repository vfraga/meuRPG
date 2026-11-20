package leaktest

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
)

// A pick the sheet holds of an option the table closed afterwards stays with its holder (RN-23): she reads it, key and name, on
// her own sheet, her level-up and her choices, and nothing more of the options that are closed to her. Another player reads
// nothing that names any of them, and the issues about it reach the master alone. Once the holder lets go of it, it is a key that
// does not exist for her.
func TestAHeldPickOfAnOptionTheTableClosedAfterwardsIsReadByItsHolderAndTheMasterAlone(t *testing.T) {
	td := newTides(t, tideWays[0])
	p, w, ctx := td.p, td.p.w, t.Context()
	held, heldName := td.live, td.names[0]
	others, otherNames := td.fresh(), td.freshNames()
	holder := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantora", Sheet: td.sheetAt(2, held),
	}))).GetCharacter()
	td.close(t)

	reads := map[string]func(who reader) reply{
		"the character": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServiceGetCharacterProcedure, &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: holder.GetId()})
		},
		"the characters list": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServiceListCharactersProcedure, &charactersv1.ListCharactersRequest{CampaignId: w.campaign})
		},
		"the level-up options": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: holder.GetId()})
		},
		"the level-up preview": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, td.levelUp(holder))
		},
		"the choices of the sheet": func(who reader) reply {
			return who.person.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
				CampaignId: w.campaign, CharacterId: holder.GetId(), Kind: who.kind, Sheet: td.sheetAt(2, held),
			})
		},
	}
	for what, read := range reads {
		for _, who := range p.readers() {
			if !who.member {
				continue
			}
			r := read(who)
			body := string(r.body)
			holds := who.person == w.ana && what != "the characters list" || who.person == w.master
			if has := strings.Contains(body, held); has && !holds {
				t.Errorf("%s reads %s: it names the option %s that is not hers", who.name, what, held)
			}
			if strings.Contains(body, heldName) && !holds {
				t.Errorf("%s reads %s: it names the option %q that is not hers", who.name, what, heldName)
			}
			if who.person == w.master {
				continue
			}
			td.expectNone(t, who.name+" reads "+what+", where the closed options are", r.body, others, otherNames)
		}
	}
	// The holder reads the pick she holds on her sheet and in her level-up preview, and her issue sentences name none of it.
	for _, what := range []string{"the character", "the level-up preview", "the choices of the sheet"} {
		if r := reads[what](reader{person: w.ana, kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER}); !strings.Contains(string(r.body), held) {
			t.Errorf("Ana reads %s: it does not name her pick %s: %s", what, held, shorten(r.body))
		}
	}
	var got charactersv1.GetCharacterResponse
	if r := reads["the character"](reader{person: w.ana}); protojson.Unmarshal(r.body, &got) != nil {
		t.Fatalf("GetCharacter: %s", shorten(r.body))
	}
	for _, is := range got.GetCharacter().GetDerived().GetIssues() {
		if text := protojson.Format(is); strings.Contains(text, held) || strings.Contains(text, heldName) {
			t.Errorf("an issue Ana reads names the pick: %s", text)
		}
	}

	// Ana lets go of it: the key is then one that does not exist for her.
	saved := must(w.ana.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: holder.GetId(), Revision: holder.GetRevision(), Name: holder.GetName(), Sheet: td.sheetAt(2),
	}))).GetCharacter()
	for what, call := range map[string]func(key string) reply{
		"PreviewChoices": func(key string) reply {
			return p.preview(reader{person: w.ana, kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER}, td.sheetAt(2, key))
		},
		"UpdateCharacter": func(key string) reply {
			return w.ana.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: holder.GetId(), Revision: saved.GetRevision(), Name: holder.GetName(), Sheet: td.sheetAt(2, key),
			})
		},
	} {
		if named, absent := call(held), call(absentKey); !sameAnswer(named, held, absent, absentKey) {
			t.Errorf("%s: the new pick of %s: HTTP %d %s\nthe pick of a key that does not exist: HTTP %d %s", what, held, named.status, shorten(named.body), absent.status, shorten(absent.body))
		}
	}
}
