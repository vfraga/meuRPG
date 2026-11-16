package leaktest

import (
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
)

// A pick the sheet holds of an option the table hid afterwards stays on the sheet (ADR-0018, section 7), and is a choice of
// its holder alone: another player's reads of the character name nothing of it, nor do the issues, the choices and the
// level-up that the holder reads once the option is no pick of anyone else. The master reads it.
func TestAHeldPickOfAnOptionTheTableHidAfterwardsIsNamedToItsHolderAndTheMasterAlone(t *testing.T) {
	td := newTides(t, tideWays[2])
	p, w, ctx := td.p, td.p.w, t.Context()
	option, name := td.out[0], td.names[0]
	holder := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantora", Sheet: td.sheetAt(2, option),
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
		"the choices of the sheet": func(who reader) reply { return p.preview(who, td.sheetAt(2, option)) },
	}
	for what, read := range reads {
		for _, who := range p.readers() {
			if !who.member {
				continue
			}
			r := read(who)
			mentions := strings.Contains(string(r.body), option) || strings.Contains(string(r.body), name)
			switch {
			case who.person == w.master:
				// The master reads what is switched off, the held pick among it.
			case who.person == w.ana:
				// The holder keeps reading the pick it made.
			case mentions:
				t.Errorf("%s reads %s: it names the option %s (%q) that another player holds: %s", who.name, what, option, name, shorten(r.body))
			}
		}
	}
}
