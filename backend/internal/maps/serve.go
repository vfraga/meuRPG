package maps

import (
	"context"
	"errors"
	"net/http"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/blob"
	"github.com/PuraFome/meuRPG/backend/internal/platform/slowclient"
)

// playersSeeImage says whether the campaign's players see the image now:
// it is the open session's shown image, an image the master left with them
// (MR-028), or the background of a map they see (revealed, or the
// session's current map), or the portrait of an NPC on the stage of the open
// scene (MR-031). Reads, one per module: what the session shows and the
// stage come from play (LiveSession), the maps from this module's table.
//
// The background of a map with the fog of war on is never one of them: a player
// receives such a map only as the squares their character sees (MR-036), so the
// raw image is refused them whatever else shows it (RN-10). Turning the fog on
// copies an image that is also used another way (fogimage.go), so the fog map's
// image is its own.
func (s *Service) playersSeeImage(ctx context.Context, campaignID, imageID string) (bool, error) {
	fog, err := s.queries.ImageIsOnAFogMap(ctx, mapsdb.ImageIsOnAFogMapParams{CampaignID: campaignID, ImageID: imageID})
	if err != nil || fog {
		return false, err
	}
	currentMap, shownImage, err := s.live.OnScreen(ctx, campaignID)
	if err != nil {
		return false, err
	}
	if shownImage == imageID {
		return true, nil
	}
	left, err := s.queries.ImageIsLeft(ctx, mapsdb.ImageIsLeftParams{CampaignID: campaignID, ImageID: imageID})
	if err != nil || left {
		return left, err
	}
	var current *string
	if currentMap != "" {
		current = &currentMap
	}
	onMap, err := s.queries.ImageIsOnAVisibleMap(ctx, mapsdb.ImageIsOnAVisibleMapParams{CampaignID: campaignID, ImageID: imageID, CurrentMapID: current})
	if err != nil || onMap {
		return onMap, err
	}
	// A portrait is fetchable only while its NPC is on the stage (RN-20): the
	// moment the master takes the NPC off, or closes the scene, it is 404.
	return s.live.ImageOnStage(ctx, campaignID, imageID)
}

// handleImage serves GET /images/{id}, the image.
func (s *Service) handleImage(w http.ResponseWriter, r *http.Request) {
	if err := s.serve(w, r, false); err != nil {
		s.writeError(w, r, err)
	}
}

// handleThumbnail serves GET /images/{id}/thumb, its thumbnail.
func (s *Service) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	if err := s.serve(w, r, true); err != nil {
		s.writeError(w, r, err)
	}
}

// serve sends an image, or its thumbnail, to a member of its campaign who
// may see it now (RN-10):
//   - the campaign's master, every image of the campaign;
//   - a player, only an image they see at this moment: the background of a
//     map they see (revealed, or the open session's current map), the
//     image the master shows in the open session, an image the master
//     left with the players (MR-028), or the portrait of an NPC that is on
//     the stage of the open scene (MR-031).
//
// Knowing an ID is not enough for a player: they keep the IDs of maps that
// were hidden again and of images no longer shown, so the rule is checked
// on every request. Anyone else, and a player asking for an image they may
// not see now, gets 404, exactly as for an image that does not exist,
// never 403: the answer must not say whether an image exists.
//
// The order of the checks keeps that true. Without a session the answer
// is 401 before the image is even looked up. The membership and visibility
// checks come before the ETag's 304, so nobody learns that an image exists,
// or keeps an image that is no longer theirs to see, by sending
// If-None-Match.
//
// Caching follows the same rule. The master's copy may be kept for a year
// (an ID's bytes never change). A player's copy must be checked again on
// every use (no-cache): the browser asks with If-None-Match, gets a cheap
// 304 while the image is still visible, and 404 once it is not. Both carry
// Vary: Cookie, so a browser shared by the master and a player (one signs
// out, the other signs in) never answers one's request from the other's
// cached copy: a new session cookie is a new cache entry.
func (s *Service) serve(w http.ResponseWriter, r *http.Request, thumbnail bool) error {
	ctx := r.Context()
	if _, err := authz.RequireSignedIn(ctx); err != nil {
		return err
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return errImageNotFound()
	}
	row, err := s.queries.GetGalleryImage(ctx, id.String())
	if errors.Is(err, pgx.ErrNoRows) {
		return errImageNotFound()
	}
	if err != nil {
		return s.dbError(ctx, "find an image", err)
	}
	m, err := authz.RequireCampaignMember(ctx, row.CampaignID)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeUnavailable {
			return err
		}
		return errImageNotFound()
	}
	master := m.Role == authz.RoleMaster
	if !master {
		visible, err := s.playersSeeImage(ctx, row.CampaignID, row.ID)
		if err != nil {
			return s.dbError(ctx, "check whether the players see an image", err)
		}
		if !visible {
			return errImageNotFound()
		}
	}

	imageKey, thumbnailKey := blobKeys(row.CampaignID, row.ID)
	key, etag := imageKey, `"`+row.ID+`"`
	if thumbnail {
		key, etag = thumbnailKey, `"`+row.ID+`.thumb"`
	}
	obj, err := s.blobs.Open(ctx, key)
	if err != nil {
		// The row is written after its files and deleted before them, so a
		// missing file means the store lost it.
		if errors.Is(err, blob.ErrNotFound) {
			s.logger.ErrorContext(ctx, "maps: an image's file is missing")
			return errImageNotFound()
		}
		s.logger.ErrorContext(ctx, "maps: cannot read an image file", "error", err)
		return errStorage()
	}
	defer func() { _ = obj.Close() }()

	header := w.Header()
	header.Set("Content-Type", obj.ContentType)
	if master {
		// The bytes behind an ID never change: a new upload gets a new ID.
		// So the master's browser may keep the image for a year without
		// asking again; private, because it is only for this member.
		header.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		// The player may lose sight of it any time: ask every time (the 304
		// keeps that cheap).
		header.Set("Cache-Control", "private, no-cache")
	}
	// The master's and a player's copies never stand in for each other.
	header.Set("Vary", "Cookie")
	header.Set("ETag", etag)
	// The browser must take the file for what Content-Type says, show it
	// in the page, and run nothing in it even when opened on its own.
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Disposition", "inline")
	header.Set("Content-Security-Policy", "default-src 'none'")
	// Only this site's pages may embed it.
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	// ServeContent sets Content-Length, answers If-None-Match with 304
	// (from the ETag above), HEAD and range requests.
	defer slowclient.WriteBody(w, downloadWriteTimeout)()
	http.ServeContent(w, r, "", time.Time{}, obj.Content)
	return nil
}
