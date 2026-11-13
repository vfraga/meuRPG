package maps

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// Who sees what on a map (RN-10, MR-009). The master sees everything. A
// player sees:
//
//	a map      when it is revealed, or it is the open session's current map
//	a point    when it is revealed, on a map they see, with three kinds
//	           the rule changes for:
//	             a LIGHT never, revealed or not: they see its light, not it;
//	             a TRAP when it is revealed, or was triggered, or was
//	             revealed to one of their characters (map_point_reveals), and
//	             then without its DCs, preset or effect;
//	             a TREASURE when it is revealed or found, and its description
//	             and value only once found
//	a token    when it is not hidden, on a map they see
//	a target   (where a submap point leads) when they see the target map
//
// Everything a player may not see is left out of the response, never
// blanked: its ID, name and description never leave the server. The same
// rules decide who hears about a change on the live stream: a player hears
// about it only when it touches something they see, before or after it
// (publish, below).

// viewer is who reads the maps: the caller, and the open session's current
// map, which a player sees even when it is hidden.
type viewer struct {
	master     bool
	userID     string
	currentMap string // "" when there is no open session or no current map
	// preview is the master reading as a player's character ("Ver como"): every
	// map is then theirs to read, and everything else follows the player's rules.
	preview bool
	// tv is the table screen: a player with no account and no character, who reads what the group
	// sees (tableScreenUser). It is never the master, knows no trap and owns nothing.
	tv bool
	// parentKnows, for a player, says whether a submap point of a parent map with
	// the fog on is on a square they see or remember (nil: not asked, the master).
	// "Submapa de ..." names the parent only through such a point.
	parentKnows func(mapsdb.ListSubmapLinksRow) bool
	// traps are the traps revealed to one of the player's characters, by point
	// ID (nil for the master, who sees all of them).
	traps map[string]knownTrap
}

// knownTrap is a trap one of a player's characters knows: the map it is on, and
// whether everyone sees it anyway (revealed to all, or triggered), which the
// map's point count already holds.
type knownTrap struct {
	mapID  string
	public bool
}

// newTVViewer is the table screen's viewer. It sees the maps the players see, as a player does.
func newTVViewer(currentMap string) viewer {
	return viewer{userID: tableScreenUser, currentMap: currentMap, tv: true}
}

func newViewer(m authz.Membership, currentMap string) viewer {
	return viewer{master: m.Role == authz.RoleMaster, userID: m.UserID, currentMap: currentMap}
}

// playersSee says whether the players see a map: revealed, or the current
// one.
func playersSee(id string, revealedAt *time.Time, currentMap string) bool {
	return revealedAt != nil || (currentMap != "" && id == currentMap)
}

// seesMap says whether the viewer sees a map.
func (v viewer) seesMap(id string, revealedAt *time.Time) bool {
	return v.master || v.preview || playersSee(id, revealedAt, v.currentMap)
}

// seesLinked says whether the viewer sees a map that something they read leads to
// or comes from (a submap's target, a parent): the master sees all, and a player, or
// the master "Ver como", only the maps the players see.
func (v viewer) seesLinked(id string, revealedAt *time.Time) bool {
	return v.master || playersSee(id, revealedAt, v.currentMap)
}

// seesPoint says whether the viewer sees a point of a map they see.
func (v viewer) seesPoint(p mapsdb.MapPoint) bool {
	if v.master {
		return true
	}
	if _, known := v.traps[p.ID]; known && p.Kind == kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TRAP] {
		return true // revealed to one of their characters
	}
	return everyoneSees(p)
}

// everyoneSees says whether every player who sees the point's map sees the
// point. It is also what decides whether a change to the point reaches the
// players' streams.
func everyoneSees(p mapsdb.MapPoint) bool {
	switch p.Kind {
	case kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT]:
		return false
	case kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TRAP]:
		return p.RevealedAt != nil || p.TrapTriggeredAt != nil // a trap that fired stays public, even disarmed
	case kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE]:
		return p.RevealedAt != nil || p.TreasureFoundAt != nil
	default:
		return p.RevealedAt != nil
	}
}

// seesToken says whether the viewer sees a token of a map they see.
func (v viewer) seesToken(t mapsdb.MapToken) bool {
	return v.master || !t.Hidden
}

// campaignMaps is what the responses need to know about a campaign's maps,
// read at once: each map with its image and point counts, the submap links
// between them, and the current map.
type campaignMaps struct {
	rows  []mapsdb.ListMapDetailsRow // oldest first
	byID  map[string]mapsdb.ListMapDetailsRow
	links []mapsdb.ListSubmapLinksRow
}

// loadMaps reads the campaign's maps. A campaign has at most 200, so
// reading them all is cheap, and it gives every response the names and
// states it needs (parents, submap targets) in two queries. The two run in one
// read-only transaction, so the links always match the rows.
func (s *Service) loadMaps(ctx context.Context, campaignID string) (campaignMaps, error) {
	var cm campaignMaps
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		cm, err = loadMapsWith(ctx, s.queries.WithTx(tx), campaignID)
		return err
	})
	return cm, err
}

// loadMapsWith is loadMaps on queries the caller already holds, for a read RPC
// that puts more statements in the same read-only transaction (GetMap).
func loadMapsWith(ctx context.Context, q *mapsdb.Queries, campaignID string) (campaignMaps, error) {
	rows, err := q.ListMapDetails(ctx, campaignID)
	if err != nil {
		return campaignMaps{}, fmt.Errorf("list the map details: %w", err)
	}
	links, err := q.ListSubmapLinks(ctx, campaignID)
	if err != nil {
		return campaignMaps{}, fmt.Errorf("list the submap links: %w", err)
	}
	cm := campaignMaps{rows: rows, byID: make(map[string]mapsdb.ListMapDetailsRow, len(rows)), links: links}
	for _, r := range rows {
		cm.byID[r.ID] = r
	}
	return cm, nil
}

// mapToProto builds the Map the viewer sees. The caller checked that the
// viewer sees it.
func (cm campaignMaps) mapToProto(r mapsdb.ListMapDetailsRow, v viewer) *mapsv1.Map {
	out := &mapsv1.Map{
		Id:         r.ID,
		CampaignId: r.CampaignID,
		Name:       r.Name,
		Image: &mapsv1.MapImage{
			Id:           r.ImageID,
			Url:          imageURL(r.ImageID),
			ThumbnailUrl: thumbnailURL(r.ImageID),
			Width:        r.ImageWidth,
			Height:       r.ImageHeight,
		},
		Revealed:   r.RevealedAt != nil,
		Current:    v.currentMap != "" && r.ID == v.currentMap,
		PointCount: r.RevealedPointCount,
		Revision:   r.Revision,
		CreatedAt:  timestamppb.New(r.CreatedAt),
		UpdatedAt:  timestamppb.New(r.UpdatedAt),
	}
	setMapGrid(out, r)
	out.FogEnabled, out.GroupVision, out.LayersRevision = r.FogEnabled, r.GroupVision, r.LayersRevision
	out.PlayersMoveTokens = r.PlayersMoveTokens
	if v.master {
		setMasterMapFields(out, r)
	} else {
		setPlayerMapFields(out, r, v)
	}
	out.ParentMaps = cm.parentRefs(r, v)
	return out
}

// setMapGrid fills the grid of the Map: the rules' squares and the drawn ones (MR-025).
func setMapGrid(out *mapsv1.Map, r mapsdb.ListMapDetailsRow) {
	if r.GridColumns == nil {
		out.SquareFactor = 1
		return
	}
	out.GridColumns = *r.GridColumns
	out.GridRows = gridRows(*r.GridColumns, r.GridFactor, r.ImageWidth, r.ImageHeight)
	out.SquareFactor = r.GridFactor
	out.DrawnColumns = *r.GridColumns / r.GridFactor
	out.DrawnRows = out.GridRows / r.GridFactor
}

// setMasterMapFields adds what only the master reads of a map.
func setMasterMapFields(out *mapsv1.Map, r mapsdb.ListMapDetailsRow) {
	out.LayersRevision += r.LightRevision // the light is the master's: its changes are theirs alone
	// The gallery is the master's preparation: its names stay with them.
	out.Image.Name = r.ImageName
	out.PointCount = r.PointCount
	out.BaseLight = baseLightFromDB[r.BaseLight]
	out.GeneratedDungeon = r.GeneratedDungeon
}

// setPlayerMapFields adjusts the Map for a player: the traps they know count for them,
// and on a fog map they never receive the image.
func setPlayerMapFields(out *mapsv1.Map, r mapsdb.ListMapDetailsRow, v viewer) {
	// A trap one of their characters knows counts for them, unless everyone
	// sees it (the revealed count has it already).
	for _, k := range v.traps {
		if k.mapID == r.ID && !k.public {
			out.PointCount++
		}
	}
	if r.FogEnabled && r.GridColumns != nil {
		// On a fog map a player never receives the image: only its size, to lay
		// the map out. What they see is drawn square by square (GetMapVision).
		// The counter of the layers would tell when the master paints, so it is
		// 0 too: the app reads the view's own revision.
		out.LayersRevision = 0
		out.Image = &mapsv1.MapImage{Width: r.ImageWidth, Height: r.ImageHeight}
		out.ImageWithheld = true
	}
}

// parentRefs lists the parents of the map, in the order of the maps list (oldest first),
// each once. A player sees a parent only through a revealed point on a map they see.
func (cm campaignMaps) parentRefs(r mapsdb.ListMapDetailsRow, v viewer) []*mapsv1.MapRef {
	parents := map[string]bool{}
	for _, l := range cm.links {
		parent, ok := cm.byID[l.MapID]
		if l.TargetMapID == r.ID && ok && (v.master || v.knowsParentLink(l, parent)) {
			parents[parent.ID] = true
		}
	}
	var refs []*mapsv1.MapRef
	for _, p := range cm.rows {
		if parents[p.ID] {
			refs = append(refs, &mapsv1.MapRef{Id: p.ID, Name: p.Name})
		}
	}
	return refs
}

// knowsParentLink says whether a player may name the parent through the link: it is
// revealed, they see the parent, and the point is one they know (parentKnows).
func (v viewer) knowsParentLink(l mapsdb.ListSubmapLinksRow, parent mapsdb.ListMapDetailsRow) bool {
	return l.Revealed && v.seesLinked(parent.ID, parent.RevealedAt) && (v.parentKnows == nil || v.parentKnows(l))
}

// pointToProto builds the MapPoint the viewer sees. The caller checked
// that the viewer sees the point. Its target is left out when the viewer
// does not see the target map. The trap's reveals and the treasure's finders are
// attached apart (attachPointDetails).
func (s *Service) pointToProto(cm campaignMaps, p mapsdb.MapPoint, v viewer) *mapsv1.MapPoint {
	out := &mapsv1.MapPoint{
		Id:          p.ID,
		MapId:       p.MapID,
		Kind:        kindFromDB[p.Kind],
		Name:        p.Name,
		Description: p.Description,
		XBp:         p.XBp,
		YBp:         p.YBp,
		Revealed:    p.RevealedAt != nil || !v.master, // a player only receives what they see
		ShowDc:      p.ShowDc,
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
		Stairs:      stairsFromDB(p.Stairs),
	}
	if v.master {
		out.Hooks = p.Hooks // the master's private text (RN-20): never a player's
		out.Trap = s.trapOf(p)
		out.Light = lightOf(p)
	} else {
		out.Trap = publicTrap(s.trapOf(p))
	}
	if p.Kind == kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE] {
		setTreasureFields(out, p, v)
	}
	if p.TargetMapID != nil {
		if target, ok := cm.byID[*p.TargetMapID]; ok && v.seesLinked(target.ID, target.RevealedAt) {
			out.TargetMap = &mapsv1.MapRef{Id: target.ID, Name: target.Name}
		}
	}
	return out
}

// setTreasureFields adds what the viewer reads of a treasure: the value and the
// description only once it is found, the master always.
func setTreasureFields(out *mapsv1.MapPoint, p mapsdb.MapPoint, v viewer) {
	found := p.TreasureFoundAt != nil
	if found {
		out.TreasureFoundAt = timestamppb.New(*p.TreasureFoundAt)
	}
	if p.TreasureValuePo != nil && (v.master || found) {
		out.TreasureValuePo = *p.TreasureValuePo
	}
	if !v.master && !found {
		out.Description = "" // what is inside is read once the treasure is found
	}
	out.TreasureConverted = v.master && p.TreasureConvertedAwardID != nil
}

// tokenToProto builds the MapToken the viewer sees, with its character.
func tokenToProto(t mapsdb.MapToken, c *charactersv1.CharacterSummary, v viewer) *mapsv1.MapToken {
	out := &mapsv1.MapToken{
		MapId:       t.MapID,
		CharacterId: t.CharacterID,
		Name:        c.GetName(),
		Kind:        c.GetKind(),
		Mine:        c.GetPlayerUserId() != "" && c.GetPlayerUserId() == v.userID,
		XBp:         t.XBp,
		YBp:         t.YBp,
		Hidden:      t.Hidden,
		UpdatedAt:   timestamppb.New(t.UpdatedAt),
	}
	if t.CarriedLight != nil && (v.master || out.Mine) {
		// What the other players see of a carried light is its light, from the
		// fog's read slice on; the key is for the master and the owner.
		out.CarriedLight = *t.CarriedLight
	}
	return out
}

// The live events (play.proto, WatchGameSessionResponse). Each one goes to
// the master, and to the players only when the change touches something
// they see before or after it: then their copy of the map is stale. A
// change to hidden things only reaches the master, so a player's stream
// never mentions them, not even by a map_changed on a map they see.

// publish is the one function in this package that calls the play module's
// Publish (master always, players when asked): every other call tells the
// streams through it (the archtest in internal/archtest/publish_test.go fails
// for a call to Publish anywhere else). It passes its arguments on unchanged.
func (s *Service) publish(campaignID string, players bool, ev *playv1.WatchGameSessionResponse) {
	s.live.Publish(campaignID, players, ev)
}

// publishMapChanged tells the watching members that a map changed: the
// master, and the players when players is true.
func (s *Service) publishMapChanged(campaignID, mapID string, players bool) {
	s.publish(campaignID, players, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_MapChanged_{
		MapChanged: &playv1.WatchGameSessionResponse_MapChanged{MapId: mapID},
	}})
}

// publishTokenMoved tells the watching members that a token moved.
func (s *Service) publishTokenMoved(campaignID string, t mapsdb.MapToken, players bool) {
	s.publish(campaignID, players, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_TokenMoved_{
		TokenMoved: &playv1.WatchGameSessionResponse_TokenMoved{MapId: t.MapID, CharacterId: t.CharacterID, XBp: t.XBp, YBp: t.YBp},
	}})
}

// publishCurrentMapCleared tells everyone that the session has no current
// map anymore: its map was deleted.
func (s *Service) publishCurrentMapCleared(campaignID string) {
	s.publish(campaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CurrentMapChanged_{
		CurrentMapChanged: &playv1.WatchGameSessionResponse_CurrentMapChanged{},
	}})
}

// publishParentsChanged tells the watching members about the maps whose
// submap points lead to target, after target was deleted, revealed or
// hidden: those points gained or lost their target. A player hears about a
// parent they see, with a revealed point, when the target was visible to
// them before or after (targetBefore, targetAfter) and that changed.
func (s *Service) publishParentsChanged(campaignID, target string, cm campaignMaps, currentMap string, targetBefore, targetAfter bool) {
	players := map[string]bool{}
	var order []string
	for _, l := range cm.links {
		if l.TargetMapID != target {
			continue
		}
		parent, ok := cm.byID[l.MapID]
		if !ok {
			continue
		}
		if _, seen := players[parent.ID]; !seen {
			order = append(order, parent.ID)
		}
		players[parent.ID] = players[parent.ID] ||
			(l.Revealed && playersSee(parent.ID, parent.RevealedAt, currentMap) && targetBefore != targetAfter)
	}
	for _, id := range order {
		s.publishMapChanged(campaignID, id, players[id])
	}
}

// stairsFromDB is a stair point's direction; every other point has none.
func stairsFromDB(s *string) mapsv1.StairDirection {
	switch {
	case s == nil:
		return mapsv1.StairDirection_STAIR_DIRECTION_UNSPECIFIED
	case *s == "up":
		return mapsv1.StairDirection_STAIR_DIRECTION_UP
	case *s == "down":
		return mapsv1.StairDirection_STAIR_DIRECTION_DOWN
	}
	return mapsv1.StairDirection_STAIR_DIRECTION_UNSPECIFIED
}
