package play

import (
	"testing"

	"connectrpc.com/connect"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
)

// The writes of the maps module that change a token, the party on a map, a carried light or a scene's actions run under play's
// funnel (Service.InSession, #735 part 2, 4d maps): the session's number moves once with the write, the events the maps module
// tells carry it, a repeat of the key moves nothing and tells again, a refused call tells nothing and moves nothing, and with no
// open session nothing moves.

// mapRigHints is the cave with the maps module wired both ways and a stream open for each person.
func mapRigHints(t *testing.T, quiet ...string) (*trapRig, *hintRig) {
	t.Helper()
	r := newTrapRig(t)
	return r, newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia}, quiet...)
}

func (r *trapRig) placeCall(t *testing.T, characterID string, col, row int) func() error {
	t.Helper()
	x, y := sq(col, row)
	return func() error {
		_, err := r.mc(r.master).PlaceMapToken(t.Context(), connect.NewRequest(&mapsv1.PlaceMapTokenRequest{
			CampaignId: r.campaignID, MapId: r.mapID, CharacterId: characterID, XBp: x, YBp: y,
		}))
		return err
	}
}

// noSession ends the table's session and says the number it stood at.
func (r *trapRig) noSession(t *testing.T, g *hintRig) {
	t.Helper()
	r.master.end(t, r.master.liveSession(t, r.campaignID).GetGameSession())
	g.drain()
}

// withNoOpenSession fails if the write moved the number once no session was open.
func (g *hintRig) withNoOpenSession(call func() error) {
	g.t.Helper()
	before := g.seq()
	if err := call(); err != nil {
		g.t.Fatalf("the write with no open session: error = %v", err)
	}
	if got := g.seq(); got != before {
		g.t.Errorf("hint_seq = %d with no open session, want %d", got, before)
	}
}

func TestPlaceMapTokenHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t)
	// A player's character that lands is one write: the token's, with the notice of what it sees from there, which moves nothing more
	// whether or not a trap was noticed.
	g.first(r.placeCall(t, r.toren.GetId(), 6, 7), true) // a new token
	g.first(r.placeCall(t, r.toren.GetId(), 7, 7), true) // a move
}

// A goblin the master places is hidden: only the master hears of it, and the players' number moves the same.
func TestPlaceHiddenMapTokenTellsOnlyTheMaster(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t, "Caio", "Ana", "Bia")
	g.first(r.placeCall(t, r.goblins.GetId(), 12, 7), true)
}

func TestPlaceMapTokenRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t)
	x, y := sq(6, 7)
	g.refused(func() error {
		_, err := r.mc(r.master).PlaceMapToken(t.Context(), connect.NewRequest(&mapsv1.PlaceMapTokenRequest{
			CampaignId: r.campaignID, MapId: newKey(), CharacterId: r.toren.GetId(), XBp: x, YBp: y,
		}))
		return err
	})
}

func TestPlaceMapTokenWithNoOpenSessionMovesNothing(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t)
	r.noSession(t, g)
	g.withNoOpenSession(r.placeCall(t, r.toren.GetId(), 6, 7))
}

func TestRemoveMapTokenHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.place(t, r.toren.GetId(), 6, 7)
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia})
	remove := func(mapID, characterID string) func() error {
		return func() error {
			_, err := r.mc(r.master).RemoveMapToken(t.Context(), connect.NewRequest(&mapsv1.RemoveMapTokenRequest{CampaignId: r.campaignID, MapId: mapID, CharacterId: characterID}))
			return err
		}
	}
	g.refused(remove(r.mapID, r.pens.GetId())) // no token of Pensantus
	g.first(remove(r.mapID, r.toren.GetId()), true)
}

func TestSetMapTokenHiddenHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.place(t, r.goblins.GetId(), 12, 7) // an NPC's token starts hidden
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia})
	set := func(characterID string, hidden bool) func() error {
		return func() error {
			_, err := r.mc(r.master).SetMapTokenHidden(t.Context(), connect.NewRequest(&mapsv1.SetMapTokenHiddenRequest{
				CampaignId: r.campaignID, MapId: r.mapID, CharacterId: characterID, Hidden: hidden,
			}))
			return err
		}
	}
	g.refused(set(r.toren.GetId(), true)) // no token of Toren
	g.first(set(r.goblins.GetId(), false), true)
}

func TestSetCarriedLightHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.place(t, r.toren.GetId(), 6, 7)
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia}, "Bia")
	light := func(who *user, characterID, key string) func() error {
		return func() error {
			_, err := r.mc(who).SetCarriedLight(t.Context(), connect.NewRequest(&mapsv1.SetCarriedLightRequest{
				CampaignId: r.campaignID, MapId: r.mapID, CharacterId: characterID, LightKey: key,
			}))
			return err
		}
	}
	g.refused(light(r.caio, r.toren.GetId(), "light:nope"))
	heard := g.first(light(r.caio, r.toren.GetId(), "light:torch"), true)
	heardOnly(t, heard, kMapChanged, "Caio") // the master and the carrier's player
}

func TestMoveMapTokensHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.place(t, r.toren.GetId(), 6, 7)
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia})
	x, y := sq(8, 7)
	move := func(characterID string) func() error {
		return func() error {
			_, err := r.mc(r.master).MoveMapTokens(t.Context(), connect.NewRequest(&mapsv1.MoveMapTokensRequest{
				CampaignId: r.campaignID, MapId: r.mapID, CharacterIds: []string{characterID}, XBp: x, YBp: y,
			}))
			return err
		}
	}
	g.refused(move(r.pens.GetId()))      // no token of Pensantus
	g.first(move(r.toren.GetId()), true) // the move, with the notice of where the token landed
}

func TestSetMapPlayersMoveHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t)
	set := func(enabled bool) func() error {
		return func() error {
			_, err := r.mc(r.master).SetMapPlayersMove(t.Context(), connect.NewRequest(&mapsv1.SetMapPlayersMoveRequest{CampaignId: r.campaignID, MapId: r.mapID, Enabled: enabled}))
			return err
		}
	}
	first := g.first(set(true), true)
	// The same setting again changes nothing: the number stays, and the notice goes out again with it.
	g.replay(set(true), first)
	g.refused(func() error {
		_, err := r.mc(r.master).SetMapPlayersMove(t.Context(), connect.NewRequest(&mapsv1.SetMapPlayersMoveRequest{CampaignId: r.campaignID, MapId: newKey(), Enabled: true}))
		return err
	})
}

func TestPutPartyOnMapHints(t *testing.T) {
	t.Parallel()
	r, g := mapRigHints(t)
	x, y := sq(6, 7)
	put := func(mapID string, ids ...string) func() error {
		return func() error {
			_, err := r.mc(r.master).PutPartyOnMap(t.Context(), connect.NewRequest(&mapsv1.PutPartyOnMapRequest{
				CampaignId: r.campaignID, MapId: mapID, CharacterIds: ids, XBp: x, YBp: y,
			}))
			return err
		}
	}
	g.refused(put(newKey(), r.toren.GetId()))
	g.first(put(r.mapID, r.toren.GetId(), r.pens.GetId(), r.bri.GetId()), true) // three tokens and their notice, one write
}

func TestCreatureTokenHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	wolf := r.give(t, r.toren, "monster:wolf", "Presa")
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia})
	x, y := sq(7, 7)
	place := func() error {
		_, err := r.mc(r.master).PlaceMapToken(t.Context(), connect.NewRequest(&mapsv1.PlaceMapTokenRequest{
			CampaignId: r.campaignID, MapId: r.mapID, CreatureId: wolf.GetId(), XBp: x, YBp: y,
		}))
		return err
	}
	remove := func(creatureID string) func() error {
		return func() error {
			_, err := r.mc(r.master).RemoveMapToken(t.Context(), connect.NewRequest(&mapsv1.RemoveMapTokenRequest{CampaignId: r.campaignID, MapId: r.mapID, CreatureId: creatureID}))
			return err
		}
	}
	g.first(place, true)
	g.refused(remove(newKey())) // no such token
	g.first(remove(wolf.GetId()), true)
}

// The actions of a scene point: the master's own write, keyed for the one that adds.
func TestSceneActionHints(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	x, y := sq(10, 4)
	res, err := r.mc(r.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
		CampaignId: r.campaignID, MapId: r.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, Name: "Sala", XBp: x, YBp: y,
	}))
	if err != nil {
		t.Fatalf("CreateMapPoint(scene) error = %v", err)
	}
	pointID := res.Msg.GetPoint().GetId()
	g := newHintRig(t, r.h, r.campaignID, r.master, map[string]*user{"Caio": r.caio, "Ana": r.ana, "Bia": r.bia}, "Caio", "Ana", "Bia")
	add := func(key, name string) func() error {
		return func() error {
			_, err := r.mc(r.master).AddSceneAction(t.Context(), connect.NewRequest(&mapsv1.AddSceneActionRequest{
				CampaignId: r.campaignID, MapId: r.mapID, PointId: pointID, Key: "skill:investigation", Name: name, Dc: 12, IdempotencyKey: key,
			}))
			return err
		}
	}
	key := newKey()
	g.keyed(add(key, "Procurar"), add(key, "Procurar"), add(key, "Outra")) // the point is not revealed: the master alone hears
	g.lost(add(newKey(), "Revistar"))
	g.refused(func() error {
		_, err := r.mc(r.master).RemoveSceneAction(t.Context(), connect.NewRequest(&mapsv1.RemoveSceneActionRequest{
			CampaignId: r.campaignID, MapId: r.mapID, PointId: pointID, ActionId: newKey(),
		}))
		return err
	})
}
