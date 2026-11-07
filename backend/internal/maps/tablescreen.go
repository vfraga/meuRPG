package maps

import (
	"context"

	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// The map on the table screen (the TV at the table): a player with no account and no character, who reads
// what the group sees. The play module calls TableScreenMap for the map it shows; the tile route answers
// the master's `as=tv` for the same viewer (tiles.go).

// tvViewer is the table screen's viewer: the maps the players see now, no trap known, nothing owned.
func (s *Service) tvViewer(ctx context.Context, campaignID string) (viewer, error) {
	current, err := s.currentMap(ctx, campaignID)
	if err != nil {
		return viewer{}, err
	}
	return newTVViewer(current), nil
}

// TableScreenMap reads a map as the table screen shows it: the map, its points and its tokens from the
// code of the player's GetMap, and the view of its squares from the code of the player's GetMapVision, both
// for the table screen's viewer. The caller is the play module, which has checked that the caller is the
// master.
//
// may decides, from the map's fog and "Visão do grupo", whether the map goes to the TV at all; when it does
// not, the answer is Hidden and nothing of the map goes out (the snapshot is read first, to know the fog). It returns a `not_found` Connect error for a map
// that is not the campaign's or that no player sees now (RN-10).
func (sm *SessionMaps) TableScreenMap(ctx context.Context, campaignID, mapID string, may link.MapGate) (link.TableScreenMap, error) {
	id, ok := uid.Canonical(mapID)
	if !ok || sm.svc == nil {
		return link.TableScreenMap{}, errMapNotFound()
	}
	return sm.svc.tableScreenMap(ctx, campaignID, id, may)
}

func (s *Service) tableScreenMap(ctx context.Context, campaignID, mapID string, may link.MapGate) (link.TableScreenMap, error) {
	v, err := s.tvViewer(ctx, campaignID)
	if err != nil {
		return link.TableScreenMap{}, err
	}
	// The generation is read before the snapshot, as GetMapVision does: a change that commits between the
	// read and the cache put drops the views, and the views built from the older read must not be kept.
	gen := s.tiles.views.generation(mapID)
	snap, err := s.readMapSnapshot(ctx, campaignID, mapID, v) // the map, its points and its tokens, one snapshot
	if err != nil {
		return link.TableScreenMap{}, s.dbError(ctx, "get a map", err)
	}
	if !may(foggedFor(v, snap.row), snap.row.GroupVision) {
		return link.TableScreenMap{Hidden: true}, nil
	}
	res, pv, err := s.mapOfSnapshot(ctx, snap, v)
	if err != nil {
		return link.TableScreenMap{}, err
	}
	// The TV paints no layer: the counter would move for every stroke of the master's, and the squares
	// the TV draws are in the vision.
	res.Map.LayersRevision = 0
	out := link.TableScreenMap{Map: res}
	if g := gridOf(snap.row.GridColumns, snap.row.GridFactor, snap.row.ImageWidth, snap.row.ImageHeight); g.Valid() { // no grid, no squares to see
		if pv == nil {
			pv = everything(g)
		}
		out.Vision = s.visionResponse(mapID, gen, tileViewerOf(v, campaignID, ""), v, snap.row, pv)
	}
	out.Revision = tableScreenRevision(out.Map, out.Vision)
	return out, nil
}

// tableScreenRevision is the TV's own number for a map: a hash of what the TV reads (the map, its points
// and its tokens, and the view of the squares). It moves when that moves. The map's revision and update
// time are left out: they move for any change of the map, including the ones only the master sees (a hidden
// point, the light), and a number that moved for those would tell the table something happened.
func tableScreenRevision(m *mapsv1.GetMapResponse, vision *mapsv1.GetMapVisionResponse) int32 {
	body, _ := proto.Clone(m).(*mapsv1.GetMapResponse)
	body.Map.Revision, body.Map.UpdatedAt = 0, nil
	opts := proto.MarshalOptions{Deterministic: true}
	a, _ := opts.Marshal(body)
	var b []byte
	if vision != nil {
		b, _ = opts.Marshal(vision)
	}
	return hashOf(a, b)
}
