package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// The writes of the characters module that change what a live session shows run under play's funnel (Service.InSession, #735 part 3):
// the session's number moves once with the write, the hints go out after the commit with it, a refused call or one that changes nothing
// moves nothing and tells nothing, and a keyed repeat moves nothing.

const (
	unknownLink       = "not-a-link-of-this-table"
	kXPChanged        = "*playv1.WatchGameSessionResponse_XpChanged_"
	kCharacterRevived = "*playv1.WatchGameSessionResponse_CharacterRevived_"
	kVitalsChanged    = "*playv1.WatchGameSessionResponse_VitalsChanged_"
)

// partyHintRig is a rig with a stream for the master and each player of the party.
func (a *armed) partyHintRig(t *testing.T, quiet ...string) *hintRig {
	t.Helper()
	return newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Caio": a.caio, "Ana": a.ana, "Bia": a.bia}, quiet...)
}

func TestReviveCharacterHints(t *testing.T) {
	t.Parallel()
	race := reviveRace(t)
	a := race.a
	g := a.partyHintRig(t, "Ana", "Bia") // Toren is Caio's: the table's other players hear nothing of it
	revive := func(character string) func() error {
		return func() error {
			_, err := a.master.characters.ReviveCharacter(t.Context(), connect.NewRequest(&charactersv1.ReviveCharacterRequest{
				CampaignId: a.campaignID, CharacterId: character, IdempotencyKey: "7f0e1d2c-3b4a-4c5d-8e6f-0a1b2c3d4e5f",
			}))
			return err
		}
	}
	g.keyed(revive(a.toren.GetId()), revive(a.toren.GetId()), revive(a.bri.GetId()))
}

func TestReviveCharacterHeardByTheMasterAndTheOwner(t *testing.T) {
	t.Parallel()
	race := reviveRace(t)
	g := race.a.partyHintRig(t)
	heard := g.first(race.act, true)
	for _, name := range []string{"master", "Caio"} {
		if !slices.Contains(heard[name], kCharacterRevived) {
			t.Errorf("%s heard %v, want character_revived", name, heard[name])
		}
	}
	for _, name := range []string{"Ana", "Bia"} {
		if len(heard[name]) != 0 {
			t.Errorf("%s heard %v, want nothing", name, heard[name])
		}
	}
}

func TestReviveCharacterRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	race := reviveRace(t)
	a := race.a
	g := a.partyHintRig(t)
	g.refused(func() error { // Brisa is alive
		_, err := a.master.characters.ReviveCharacter(t.Context(), connect.NewRequest(&charactersv1.ReviveCharacterRequest{
			CampaignId: a.campaignID, CharacterId: a.bri.GetId(), IdempotencyKey: newKey(),
		}))
		return err
	})
}

func TestReviveCharacterWithNoOpenSessionMovesNothing(t *testing.T) {
	t.Parallel()
	race := reviveRace(t)
	a := race.a
	g := a.partyHintRig(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	before := g.seq()
	g.drain()
	if err := race.act(); err != nil {
		t.Fatalf("ReviveCharacter(no open session) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d", got, before)
	}
}

func TestSetEquippedInACombatHints(t *testing.T) {
	t.Parallel()
	race := equipRace(t)
	a := race.a
	g := a.partyHintRig(t)
	heard := g.first(race.act, true) // stowing the longsword spends the free object interaction
	if !slices.Contains(heard["master"], "*playv1.WatchGameSessionResponse_EncounterChanged_") || !slices.Contains(heard["Ana"], "*playv1.WatchGameSessionResponse_EncounterChanged_") {
		t.Errorf("heard %v, want the table told of the combat", heard)
	}
	// Stowing it again changes nothing: the number stays and nobody hears.
	before := g.seq()
	g.drain()
	if _, err := a.swap(a.ana, a.pens, longswordKey, false); err != nil {
		t.Fatalf("SetEquipped(again) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d after a swap that changed nothing, want %d", got, before)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of a swap that changed nothing", name, eventKinds(evs))
		}
	}
}

func TestSetEquippedRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	race := equipRace(t)
	a := race.a
	g := a.partyHintRig(t)
	g.refused(func() error { _, err := a.swap(a.ana, a.pens, "equipment:not-on-the-sheet", false); return err })
}

func TestSheetSaveThatChangesTheGearHints(t *testing.T) {
	t.Parallel()
	race := shieldRace(t) // Mage Armor is on Pensantus: the shield changes the armor class it gives
	a := race.a
	g := a.partyHintRig(t, "Bia")
	heard := g.first(race.act, true)
	if !slices.Contains(heard["master"], kVitalsChanged) || !slices.Contains(heard["Ana"], kVitalsChanged) {
		t.Errorf("heard %v, want the master and the owner told of the vitals", heard)
	}
}

// A save that changes no gear still moves the number once, and tells nobody: a correction only the master sees.
func TestSheetSaveThatChangesNothingOfTheGearMovesTheNumberOnce(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	g := a.partyHintRig(t)
	g.first(a.sheetEdit(t, a.pens, func(*charactersv1.FullSheet) {}), false)
}

func TestSheetSaveRefusedMovesNothing(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	save := a.sheetEdit(t, a.pens, func(f *charactersv1.FullSheet) { f.Shield = true })
	if err := a.sheetEdit(t, a.pens, func(*charactersv1.FullSheet) {})(); err != nil {
		t.Fatalf("the save that comes first: %v", err)
	}
	g := a.partyHintRig(t)
	g.refused(save) // its revision is stale now
}

func TestSheetSaveWithNoOpenSessionMovesNothing(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	g := a.partyHintRig(t)
	save := a.sheetEdit(t, a.pens, func(f *charactersv1.FullSheet) { f.Shield = true })
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	before := g.seq()
	g.drain()
	if err := save(); err != nil {
		t.Fatalf("UpdateCharacter(no open session) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d", got, before)
	}
}

func TestCompleteCharacterChoicesHints(t *testing.T) {
	t.Parallel()
	race := choicesRace(t)
	g := race.a.partyHintRig(t)
	heard := g.first(race.act, true)
	for _, name := range []string{"master", "Caio", "Ana", "Bia"} {
		if !slices.Contains(heard[name], kXPChanged) {
			t.Errorf("%s heard %v, want xp_changed", name, heard[name])
		}
	}
	// The choice is made now: asking again is refused and moves nothing.
	g.refused(race.act)
}

// A level-up has no key: a repeat of it is a stale revision, refused, and it moves nothing.
func TestLevelUpCharacterHints(t *testing.T) {
	t.Parallel()
	race := levelUpRace(t)
	g := race.a.partyHintRig(t)
	heard := g.first(race.act, true)
	for _, name := range []string{"master", "Caio", "Ana", "Bia"} {
		if !slices.Contains(heard[name], kXPChanged) {
			t.Errorf("%s heard %v, want xp_changed", name, heard[name])
		}
	}
	for _, name := range []string{"master", "Ana"} { // the master and the player who plays it get the vitals
		if !slices.Contains(heard[name], kVitalsChanged) {
			t.Errorf("%s heard %v, want vitals_changed", name, heard[name])
		}
	}
	for _, name := range []string{"Caio", "Bia"} {
		if slices.Contains(heard[name], kVitalsChanged) {
			t.Errorf("%s heard %v, want no vitals_changed", name, heard[name])
		}
	}
	g.refused(race.act)
}

// partyTable is a table whose master made a reserved character of the party (for a player who has not claimed it yet).
func partyTable(t *testing.T) (*armed, *charactersv1.Character) {
	t.Helper()
	var reserved *charactersv1.Character
	a := newArmedWith(t, func(a *armed) {
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: abilities(16, 13, 14, 10, 8), RaceKey: "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 3}},
		}}}
		res, err := a.master.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Reserva", Sheet: sheet, ForPlayer: true,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(for_player) error = %v", err)
		}
		reserved = res.Msg.GetCharacter()
	})
	return a, reserved
}

func (a *armed) claimToken(t *testing.T, c *charactersv1.Character) string {
	t.Helper()
	res, err := a.master.characters.CreateClaimLink(t.Context(), connect.NewRequest(&charactersv1.CreateClaimLinkRequest{CampaignId: a.campaignID, CharacterId: c.GetId()}))
	if err != nil {
		t.Fatalf("CreateClaimLink() error = %v", err)
	}
	return res.Msg.GetToken()
}

func TestClaimCharacterAndReturnItToTheReserveHints(t *testing.T) {
	t.Parallel()
	a, reserved := partyTable(t)
	token := a.claimToken(t, reserved)
	claim := func() error {
		_, err := a.bia.characters.ClaimCharacter(t.Context(), connect.NewRequest(&charactersv1.ClaimCharacterRequest{Token: token}))
		return err
	}
	g := a.partyHintRig(t)
	heard := g.first(claim, true) // joining the party moves the number once
	for _, name := range []string{"master", "Caio", "Ana", "Bia"} {
		if !slices.Contains(heard[name], kXPChanged) {
			t.Errorf("%s heard %v of the claim, want xp_changed", name, heard[name])
		}
	}
	g.refused(claim) // the link is spent

	back := func() error {
		_, err := a.master.characters.ReturnCharacterToReserve(t.Context(), connect.NewRequest(&charactersv1.ReturnCharacterToReserveRequest{CampaignId: a.campaignID, CharacterId: reserved.GetId()}))
		return err
	}
	heard = g.first(back, true) // leaving it moves the number once
	for _, name := range []string{"master", "Caio", "Ana", "Bia"} {
		if !slices.Contains(heard[name], kXPChanged) {
			t.Errorf("%s heard %v of the return to the reserve, want xp_changed", name, heard[name])
		}
	}
	g.refused(back) // it is in the reserve already
}

func TestClaimCharacterWithAnUnknownTokenTellsNothing(t *testing.T) {
	t.Parallel()
	a, _ := partyTable(t)
	g := a.partyHintRig(t)
	g.refused(func() error {
		_, err := a.bia.characters.ClaimCharacter(t.Context(), connect.NewRequest(&charactersv1.ClaimCharacterRequest{Token: unknownLink}))
		return err
	})
}
