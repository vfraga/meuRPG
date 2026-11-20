package leaktest

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
)

// A choice is complete when it holds as many picks as a player may make today: the same for every call that asks whether a
// choice is complete, and for the master as for the player.
type completeCase struct {
	way   string
	count int32
	lean  bool
	picks int // the picks a player may make once the table closes the options
}

func TestAChoiceIsCompleteWithAsManyPicksAsAPlayerMayMakeToday(t *testing.T) {
	cases := map[string]completeCase{
		"the source is switched off and the pool is empty": {tideWays[0], 1, false, 0},
		"the pool is cut from two to one":                  {tideWays[2], 2, true, 1},
	}
	for name, tc := range cases {
		for _, step := range []string{"level-up", "create", "save"} {
			t.Run(name+", the "+step, func(t *testing.T) { tc.run(t, step) })
		}
	}
}

func (tc completeCase) run(t *testing.T, step string) {
	t.Helper()
	td := newTidesAsking(t, tc.way, tc.count, tc.lean)
	w, ctx := td.p.w, t.Context()
	made := func(who *person) *charactersv1.Character {
		return must(who.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
			CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Marés", Sheet: td.sheetAt(1),
		}))).GetCharacter()
	}
	var level, save *charactersv1.Character
	switch step {
	case "level-up":
		level = made(w.caio)
	case "save":
		save = made(w.ana)
	}
	td.close(t)
	var picks []string
	if tc.picks > 0 {
		picks = []string{td.live}
	}
	switch step {
	case "level-up":
		if r := w.caio.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, td.levelUp(level, picks...)); !r.ok() {
			t.Errorf("PreviewLevelUp with %d picks: HTTP %d %s", len(picks), r.status, shorten(r.body))
		}
		if _, err := w.caio.characters.LevelUpCharacter(ctx, rq(&charactersv1.LevelUpCharacterRequest{
			CampaignId: w.campaign, CharacterId: level.GetId(), Revision: level.GetRevision(), Choices: td.choices(picks...),
		})); err != nil {
			t.Errorf("LevelUpCharacter with %d picks: %v", len(picks), err)
		}
	case "create":
		if _, err := w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
			CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Marés 2", Sheet: td.sheetAt(2, picks...),
		})); err != nil {
			t.Errorf("CreateCharacter with %d picks: %v", len(picks), err)
		}
	default:
		if _, err := w.ana.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
			CampaignId: w.campaign, CharacterId: save.GetId(), Revision: save.GetRevision(), Name: save.GetName(), Sheet: td.sheetAt(2, picks...),
		})); err != nil {
			t.Errorf("UpdateCharacter with %d picks: %v", len(picks), err)
		}
	}
	// Nothing is pending for the master or the player.
	for _, who := range []*person{w.master, w.ana, w.caio} {
		for _, c := range must(who.characters.ListCharacters(ctx, rq(&charactersv1.ListCharactersRequest{CampaignId: w.campaign}))).GetCharacters() {
			if n := c.GetPendingChoiceCount(); n != 0 {
				t.Errorf("%s reads %d choices pending for %s, want none", who.name, n, c.GetName())
			}
		}
	}
}
