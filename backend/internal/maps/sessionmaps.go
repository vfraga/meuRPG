package maps

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/wiring"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// SessionMaps is what package play needs from this one for what the
// session shows (it implements play.MapKeeper):
//   - when the master makes a map the session's current one
//     (PlayService.SetCurrentMap), the map must be the campaign's, and it is
//     revealed, since the players see the current map (RN-10);
//   - when the master shows a gallery image (PlayService.SetShownImage,
//     MR-028), the image must be the campaign's, and the session needs its
//     name and size;
//   - when the master leaves the image with the players ("Deixar com os
//     jogadores", MR-028), the image goes to the campaign's left list, in
//     the session's transaction, and the master takes it back later. The
//     list is a table of this module (campaign_left_images) because the
//     image route reads it for the players (RN-10), next to the maps.
//   - when a combat starts (CombatService.StartEncounter, MR-013), the
//     session needs the map's grid, the battle point's map and where the
//     tokens stand, and when it ends it moves the player characters'
//     tokens to where they ended.
//
// It is apart from Service because the two modules need each other: this
// package needs play (LiveSession), and play needs this. SessionMaps needs
// nothing but the database, so cmd/api builds it first and hands it to
// play, then builds Service with play as its LiveSession.
type SessionMaps struct {
	queries *mapsdb.Queries
	// svc is the maps service, for what needs more than the database: the fog's
	// first view of a map that becomes visible (MapShown) and the copy of an image
	// a fog map owns (see fogimage.go). Connected by SetService; nil until then.
	svc *Service
}

// SetService connects the maps service, which is made after this (it needs play,
// and play needs this). cmd/api calls it once both exist.
func (sm *SessionMaps) SetService(s *Service) { sm.svc = s }

// MapShown tells the fog that the players can see the map from now on (it became
// the session's current map, or the combat's): what their characters see there is
// their first view, and is remembered. The play module calls it after the commit
// that made the map current. Nothing happens for a map without fog.
func (sm *SessionMaps) MapShown(ctx context.Context, campaignID, mapID string) {
	if sm.svc != nil {
		sm.svc.refreshVision(ctx, campaignID, mapID)
	}
}

// VisionChanged tells the fog that what the players of the map see changed without
// a token moving: a druid took a beast's senses or left them, or a player started
// or stopped looking through their familiar's eyes (MR-036, MR-037). The play
// module calls it after the commit. Nothing happens for a map without fog.
func (sm *SessionMaps) VisionChanged(ctx context.Context, campaignID, mapID string) {
	if sm.svc != nil && mapID != "" {
		sm.svc.refreshVision(ctx, campaignID, mapID)
	}
}

// NewSessionMaps returns the SessionMaps for play.
func NewSessionMaps(pool *pgxpool.Pool) *SessionMaps {
	return &SessionMaps{queries: mapsdb.New(pool)}
}

// RevealMap reveals the campaign's map inside tx; a revealed map stays as
// it is. It returns a `not_found` Connect error when mapID is not a map of
// the campaign. The caller checked that the caller is the campaign's
// master and that mapID is a UUID.
func (sm *SessionMaps) RevealMap(ctx context.Context, tx pgx.Tx, campaignID, mapID string, at time.Time) error {
	_, err := sm.queries.WithTx(tx).SetMapRevealed(ctx, mapsdb.SetMapRevealedParams{
		CampaignID: campaignID, ID: mapID, Revealed: true, Now: at,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return errMapNotFound()
	}
	if err != nil {
		return fmt.Errorf("reveal the current map: %w", err)
	}
	return nil
}

// ShownImage returns the campaign's gallery image as the session shows it
// to the players, or a `not_found` Connect error when imageID is not an
// image of the campaign's gallery. Its name goes along as the caption: the
// players see it while the image is shown.
func (sm *SessionMaps) ShownImage(ctx context.Context, campaignID, imageID string) (*playv1.ShownImage, error) {
	img, err := sm.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errImageNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("find the shown image: %w", err)
	}
	return shownImage(img), nil
}

// ShownCopy is what PrepareShow hands back for an image that is a fog map's
// background: the same method set as play.ShownCopy, spelled out because modules do
// not import each other.
type ShownCopy = interface {
	Insert(ctx context.Context, tx pgx.Tx) (id string, created bool, err error)
	Discard(ctx context.Context)
}

// PrepareShow is ShownImage for the master who is about to show the image. An image
// that is a fog map's background is not shown as it is (RN-10, MR-036): the copy
// already made of it is shown, or, when there is none, a new one, whose files are
// made now and whose gallery row is left to the caller's transaction (Insert,
// after it locked the session), so a show that does not happen leaves nothing but
// the files, which Discard deletes. The caller stores the returned ID. The copy is
// nil when the image is shown as it is.
func (sm *SessionMaps) PrepareShow(ctx context.Context, campaignID, imageID string) (*playv1.ShownImage, ShownCopy, error) {
	img, err := sm.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, errImageNotFound()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("find the image to show: %w", err)
	}
	if sm.svc == nil {
		return shownImage(img), nil, nil
	}
	use, c, err := sm.svc.prepareUseCopy(ctx, campaignID, img)
	if err != nil {
		return nil, nil, err
	}
	if c == nil {
		return shownImage(use), nil, nil
	}
	return shownImage(use), &pendingCopy{c: c, s: sm.svc, campaignID: campaignID}, nil
}

func shownImage(img mapsdb.GalleryImage) *playv1.ShownImage {
	return &playv1.ShownImage{
		Id:           img.ID,
		Name:         img.Name,
		Width:        img.Width,
		Height:       img.Height,
		Url:          imageURL(img.ID),
		ThumbnailUrl: thumbnailURL(img.ID),
	}
}

// LeaveImage leaves the campaign's gallery image with the players inside
// tx; an image already left stays as it is, and one that is not the
// campaign's anymore (deleted meanwhile) is skipped, since a left image
// that is gone has nothing to leave. The caller checked that the caller is
// the campaign's master.
func (sm *SessionMaps) LeaveImage(ctx context.Context, tx pgx.Tx, campaignID, imageID string, at time.Time) error {
	_, err := sm.queries.WithTx(tx).LeaveImage(ctx, mapsdb.LeaveImageParams{CampaignID: campaignID, ImageID: imageID, Now: at})
	if err != nil {
		return fmt.Errorf("leave the image with the players: %w", err)
	}
	return nil
}

// ListLeftImages returns the images left with the players, in the order
// they were left, with their names as captions.
func (sm *SessionMaps) ListLeftImages(ctx context.Context, campaignID string) ([]*playv1.ShownImage, error) {
	rows, err := sm.queries.ListLeftImages(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list the left images: %w", err)
	}
	out := make([]*playv1.ShownImage, 0, len(rows))
	for _, r := range rows {
		out = append(out, shownImage(r))
	}
	return out, nil
}

// TakeBackImage takes the image off the left list, or returns a
// `not_found` Connect error when it is not on it. The caller checked that
// the caller is the campaign's master.
func (sm *SessionMaps) TakeBackImage(ctx context.Context, campaignID, imageID string) error {
	n, err := sm.queries.TakeBackLeftImage(ctx, mapsdb.TakeBackLeftImageParams{CampaignID: campaignID, ImageID: imageID})
	if err != nil {
		return fmt.Errorf("take the left image back: %w", err)
	}
	if n == 0 {
		return errImageNotFound()
	}
	return nil
}

// gridRows is how many rows of squares a grid of columns across has on an
// image of this size (MR-013): the squares are square, so the rows follow the
// image's proportions, rounded, and kept between 1 and 400 (the
// encounters_grid_valid CHECK) for an extremely long image. Package rules/grid
// has the rule.
func gridRows(columns, factor, width, height int32) int32 {
	return int32(gridOf(&columns, factor, width, height).Rows) //nolint:gosec // G115: at most 400
}

// MapGrid returns the battle grid of the campaign's map (MR-013), or the
// zero Grid when it has none. It returns a `not_found` Connect error when
// mapID is not a map of the campaign. Like the other reads below, it reads inside
// tx when the caller has one (nil: the pool).
func (sm *SessionMaps) MapGrid(ctx context.Context, tx pgx.Tx, campaignID, mapID string) (link.Grid, error) {
	row, err := queriesIn(sm.queries, tx).GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: mapID})
	if errors.Is(err, pgx.ErrNoRows) {
		return link.Grid{}, errMapNotFound()
	}
	if err != nil {
		return link.Grid{}, fmt.Errorf("read the map's grid: %w", err)
	}
	if row.GridColumns == nil {
		return link.Grid{}, nil
	}
	return link.Grid{Columns: *row.GridColumns, Rows: gridRows(*row.GridColumns, row.GridFactor, row.ImageWidth, row.ImageHeight)}, nil
}

// BattlePoint returns a battle point of the campaign: its map and the map of
// its fight, if the master chose one. It returns a `not_found` Connect error
// when pointID is not a battle point of the campaign.
func (sm *SessionMaps) BattlePoint(ctx context.Context, campaignID, pointID string) (link.BattlePoint, error) {
	p, err := sm.queries.GetMapPointInCampaign(ctx, mapsdb.GetMapPointInCampaignParams{CampaignID: campaignID, ID: pointID})
	if err == nil && p.Kind != kindToDB[mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE] {
		err = pgx.ErrNoRows // a point of another kind starts no combat
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return link.BattlePoint{}, errPointNotFound()
	}
	if err != nil {
		return link.BattlePoint{}, fmt.Errorf("find the battle point: %w", err)
	}
	out := link.BattlePoint{MapID: p.MapID}
	if p.TargetMapID != nil {
		out.TargetMapID = *p.TargetMapID
	}
	return out, nil
}

// MapTokens returns where every token of the map stands, hidden ones
// included: the combat places the combatants on the squares where their
// characters' tokens are. The caller is the play module, after its own
// authorization check, and sends nothing of it to a player.
func (sm *SessionMaps) MapTokens(ctx context.Context, tx pgx.Tx, mapID string) ([]link.TokenPosition, error) {
	q := queriesIn(sm.queries, tx)
	rows, err := q.ListMapTokens(ctx, mapID)
	if err != nil {
		return nil, fmt.Errorf("list the map's tokens: %w", err)
	}
	out := make([]link.TokenPosition, 0, len(rows))
	for _, t := range rows {
		out = append(out, link.TokenPosition{CharacterID: t.CharacterID, XBP: t.XBp, YBP: t.YBp})
	}
	// The creatures' tokens too (MR-037): the familiar's eyes need to know where it
	// stands. They have no character of their own: CreatureID says whose they are.
	creatures, err := q.ListMapCreatureTokens(ctx, mapID)
	if err != nil {
		return nil, fmt.Errorf("list the map's creature tokens: %w", err)
	}
	for _, t := range creatures {
		out = append(out, link.TokenPosition{CreatureID: t.CreatureID, XBP: t.XBp, YBP: t.YBp})
	}
	return out, nil
}

// SetTokenPositions moves the characters' tokens on the map inside tx, or
// puts them there when they have none: where a combat left its player
// characters. A token that exists keeps its hidden flag; a new one starts
// visible. The map must exist: its foreign key says so.
func (sm *SessionMaps) SetTokenPositions(ctx context.Context, tx pgx.Tx, mapID string, positions []link.TokenPosition, at time.Time) error {
	q := sm.queries.WithTx(tx)
	for _, p := range positions {
		if p.CreatureID != "" {
			// A creature's token only moves if it has one: the master places them.
			if err := q.MoveMapCreatureToken(ctx, mapsdb.MoveMapCreatureTokenParams{MapID: mapID, CreatureID: p.CreatureID, XBp: p.XBP, YBp: p.YBP, UpdatedAt: at}); err != nil {
				return fmt.Errorf("move a creature's token: %w", err)
			}
			continue
		}
		if err := q.UpsertMapTokenPosition(ctx, mapsdb.UpsertMapTokenPositionParams{
			MapID: mapID, CharacterID: p.CharacterID, XBp: p.XBP, YBp: p.YBP, UpdatedAt: at,
		}); err != nil {
			return fmt.Errorf("move a token: %w", err)
		}
	}
	return nil
}

// PortraitCopy is what PreparePortrait hands back: the same method set as
// characters.PortraitCopy, spelled out because modules do not import each other.
type PortraitCopy = interface {
	Insert(ctx context.Context, tx pgx.Tx) (id string, created bool, err error)
	Discard(ctx context.Context)
}

// PreparePortrait is PortraitImage for a save that has a transaction (it
// implements characters.Gallery): the copy of a fog map's image has its files made
// now, and its gallery row is left to the caller's transaction (Insert), so the
// copy is never committed on its own and a save that fails leaves nothing but the
// files, which Discard deletes.
func (sm *SessionMaps) PreparePortrait(ctx context.Context, campaignID, imageID string) (string, bool, PortraitCopy, error) {
	img, err := sm.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil, nil
	}
	if err != nil {
		return "", false, nil, fmt.Errorf("find the portrait's image: %w", err)
	}
	if sm.svc == nil {
		return img.ID, true, nil, nil
	}
	use, c, err := sm.svc.prepareUseCopy(ctx, campaignID, img)
	if err != nil {
		return "", false, nil, err
	}
	if c == nil {
		return use.ID, true, nil, nil
	}
	return use.ID, true, &pendingCopy{c: c, s: sm.svc, campaignID: campaignID}, nil
}

// pendingCopy is a fogCopy whose gallery row is left to the transaction of the module that uses it (a portrait, the shown image).
type pendingCopy struct {
	c          *fogCopy
	s          *Service
	campaignID string
}

// Insert adds the copy's gallery row inside tx and returns the image to use. When
// another use of the same image committed a copy meanwhile, that copy is the one to
// use and created is false: the caller deletes these files (Discard) once the
// transaction is over, so two uses at once leave one copy, not two.
func (p *pendingCopy) Insert(ctx context.Context, tx pgx.Tx) (string, bool, error) {
	q := queriesIn(p.s.queries, tx)
	if p.c.remember {
		existing, err := q.FindImageCopy(ctx, mapsdb.FindImageCopyParams{CampaignID: p.campaignID, CopyOfImageID: &p.c.source.ID})
		if err == nil {
			return existing.ID, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", false, fmt.Errorf("find the image's copy: %w", err)
		}
	}
	row, err := p.c.insertRow(ctx, q, p.s, p.campaignID, nil)
	if err != nil {
		return "", false, err
	}
	return row.ID, true, nil
}

func (p *pendingCopy) Discard(ctx context.Context) {
	p.s.deleteFiles(context.WithoutCancel(ctx), p.campaignID, p.c.id)
}

// PortraitImage reports whether imageID is an image of the campaign's gallery and
// returns the image the portrait should be. The characters module asks it before
// it keeps an NPC's portrait (MR-031): an image of another campaign, or one that
// does not exist, is not accepted, and one that is the background of a map with
// the fog of war on is replaced by a copy of its own (RN-10). It implements
// characters.Gallery.
func (sm *SessionMaps) PortraitImage(ctx context.Context, campaignID, imageID string) (string, bool, error) {
	img, err := sm.queries.GetGalleryImageInCampaign(ctx, mapsdb.GetGalleryImageInCampaignParams{CampaignID: campaignID, ID: imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find the portrait's image: %w", err)
	}
	if sm.svc != nil {
		// A fog map's own image is never a portrait: the portrait gets a copy.
		if img, err = sm.svc.ownImage(ctx, campaignID, img); err != nil {
			return "", false, err
		}
	}
	return img.ID, true, nil
}

// CheckWired fails when the maps service was never connected (SetService): the
// fog's first view of a map would then never be computed (see platform/wiring).
func (sm *SessionMaps) CheckWired() error {
	return wiring.Check("maps.SessionMaps", wiring.Dep{Setter: "SetService", Missing: sm.svc == nil})
}
