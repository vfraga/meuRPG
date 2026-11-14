package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The repeat of a key tells the combat that holds the character, and only that one (#735, 4b): encounter_changed goes to the master
// and to the players, in the form the first call sent (the whole combat, or blind where the fog hides it), when the character the
// write is about is a player's combatant of the open combat; another player's character in the fight does not make it tell.

const kEncounter = "*playv1.WatchGameSessionResponse_EncounterChanged_"

// wantEncounterTold checks that the master, Ana (Pensantus) and Caio (Toren) heard encounter_changed in the first call and in its
// repeat, or in neither.
func wantEncounterTold(t *testing.T, first, replay map[string][]string, want bool) {
	t.Helper()
	for _, who := range []string{"master", "Ana", "Caio"} {
		if got := slices.Contains(first[who], kEncounter); got != want {
			t.Errorf("%s heard encounter_changed in the first call = %v, want %v (heard %v)", who, got, want, first[who])
		}
		if got := slices.Contains(replay[who], kEncounter); got != want {
			t.Errorf("%s heard encounter_changed in the repeat = %v, want %v (heard %v)", who, got, want, replay[who])
		}
	}
}

// lateHero is a player's fighter made after the combat began: the combat has the party as it was, so it is not in the fight.
func lateHero(t *testing.T, x *coreFixture) *charactersv1.Character {
	t.Helper()
	u := x.h.newUser("Dani")
	x.h.join(x.master, x.campaignID, u)
	return u.hero(t, x.campaignID, "Dani", "class:fighter", "race:human", 2, abilities(16, 13, 14, 10, 10), []string{battleaxe}, nil)
}

func combatPlayers(x *coreFixture) map[string]*user {
	return map[string]*user{"Ana": x.ana, "Caio": x.caio, "Bia": x.bia}
}

// adjustInFight is the master's correction of c's hit points, repeated with its key.
func adjustInFight(t *testing.T, x *coreFixture, c *charactersv1.Character) func() error {
	t.Helper()
	req := &playv1.AdjustCharacterVitalsRequest{
		CampaignId: x.campaignID, CharacterId: c.GetId(), IdempotencyKey: newKey(),
		ExpectedRevision: x.master.revisionIfOpen(x.campaignID, c.GetId()), HitPointsCurrent: proto.Int32(3),
	}
	return func() error {
		_, err := x.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(proto.Clone(req).(*playv1.AdjustCharacterVitalsRequest)))
		return err
	}
}

func TestAdjustReplayTellsTheCombatOfAPlayerInIt(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t) // the party fights: Toren, Pensantus and Brisa
	call := adjustInFight(t, x, x.toren)
	g := newHintRig(t, x.h, x.campaignID, x.master, combatPlayers(x))
	first := g.first(call, true)
	wantEncounterTold(t, first, g.replay(call, first), true)
}

func TestAdjustReplayTellsNoCombatForACharacterOutOfIt(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t)
	call := adjustInFight(t, x, lateHero(t, x))
	g := newHintRig(t, x.h, x.campaignID, x.master, combatPlayers(x))
	first := g.first(call, true)
	wantEncounterTold(t, first, g.replay(call, first), false)
}

// restOf is the master's short rest after the character spent its Second Wind (a rest gives back what a character spent), repeated
// with its key.
func restAfterSecondWind(t *testing.T, x *coreFixture, c *charactersv1.Character) func() error {
	t.Helper()
	if _, err := x.master.adjust(t, x.campaignID, c.GetId(), func(q *playv1.AdjustCharacterVitalsRequest) {
		q.ResourcesUsed = []*playv1.ResourceUsed{{Key: "second_wind", Used: 1}}
	}); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	req := &playv1.TakeRestRequest{CampaignId: x.campaignID, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()}
	return func() error {
		_, err := x.master.resource.TakeRest(t.Context(), connect.NewRequest(proto.Clone(req).(*playv1.TakeRestRequest)))
		return err
	}
}

func TestRestReplayTellsTheCombatOfAPlayerItChanged(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t)
	call := restAfterSecondWind(t, x, x.toren)
	g := newHintRig(t, x.h, x.campaignID, x.master, combatPlayers(x))
	first := g.first(call, true)
	wantEncounterTold(t, first, g.replay(call, first), true)
}

func TestRestReplayTellsNoCombatWhenItChangedNobodyInIt(t *testing.T) {
	t.Parallel()
	x := newCoreFixture(t)
	call := restAfterSecondWind(t, x, lateHero(t, x)) // not in the fight
	g := newHintRig(t, x.h, x.campaignID, x.master, combatPlayers(x))
	first := g.first(call, true)
	wantEncounterTold(t, first, g.replay(call, first), false)
}

// answerOutsideTheFight is the master's "less than a minute" for a Revivify asked outside a combat on Toren, repeated with its key.
// With toren in the fight, the combat has Toren (dead there), Ilaria and Brisa; without, only Ilaria and Brisa: Toren stays dead out of it.
func answerOutsideTheFight(t *testing.T, torenFights bool) (*armed, func() error) {
	t.Helper()
	a := newVigil(t)
	asked, err := a.ask(a.ana, a.pens, a.toren, newKey())
	if err != nil {
		t.Fatalf("RequestRevivify() error = %v", err)
	}
	players := map[string]int32{"Ilaria": 12, "Brisa": 1}
	at := map[string][2]int32{"Ilaria": {3, 4}, "Goblin": {4, 3}, "Brisa": {8, 8}}
	if torenFights {
		if _, err := a.master.characters.ReviveCharacter(t.Context(), connect.NewRequest(&charactersv1.ReviveCharacterRequest{
			CampaignId: a.campaignID, CharacterId: a.toren.GetId(), IdempotencyKey: newKey(),
		})); err != nil {
			t.Fatalf("ReviveCharacter() error = %v", err)
		}
		players["Toren"], at["Toren"] = 18, [2]int32{3, 3}
	}
	e := a.start(t, plan{npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3}, players: players, reveal: []string{"Goblin"}, at: at})
	if torenFights {
		a.dies(t, e, "Toren", a.toren)
	}
	key := newKey()
	return a, func() error {
		_, err := a.answerRevivify(t, a.master, asked.GetId(), true, key)
		return err
	}
}

func TestConfirmReplayTellsTheCombatThatHadTheDeadCharacter(t *testing.T) {
	t.Parallel()
	a, call := answerOutsideTheFight(t, true)
	g := newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Ana": a.ana, "Caio": a.caio, "Bia": a.bia})
	first := g.first(call, true)
	wantEncounterTold(t, first, g.replay(call, first), true)
}

func TestConfirmReplayTellsNoCombatWhenTheDeadCharacterWasNotInIt(t *testing.T) {
	t.Parallel()
	a, call := answerOutsideTheFight(t, false) // Ilaria and Brisa fight: neither may make the filter match
	g := newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Ana": a.ana, "Caio": a.caio, "Bia": a.bia})
	first := g.first(call, true)
	wantEncounterTold(t, first, g.replay(call, first), false)
}
