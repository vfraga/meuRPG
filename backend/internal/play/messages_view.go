package play

import (
	"context"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// AudioPath is where the audio of a line is served: AudioPath + id + "/audio".
const AudioPath = "/session-messages/"

// viewer is who reads a line: the master, or a player (userID).
type viewer struct {
	master bool
	userID string
}

// allowMessage applies the per-user limit of lines.
func (s *Service) allowMessage(ctx context.Context, userID string) error {
	if s.msgs.send == nil {
		return nil
	}
	if ok, wait := s.msgs.send.Allow(userID); !ok {
		s.msgs.sendHit.Hit(ctx)
		return ratelimit.RPCError(wait)
	}
	return nil
}

// messageView is the line as the viewer reads it. The viewer has been checked: this only shapes the
// line, it never decides who may read it. images, when not nil, holds the images of a page already
// read.
func (s *Service) messageView(ctx context.Context, row playdb.SessionMessage, v viewer, images map[string]*playv1.SessionMessageImage) (*playv1.SessionMessage, error) {
	views, err := s.messageViews(ctx, []playdb.SessionMessage{row}, v, images)
	if err != nil {
		return nil, err
	}
	return views[0], nil
}

// messageViews shapes a page of lines for one viewer, reading what they need in one go: the readers
// of the restricted lines (the master's), the display names of out-of-character authors, and the
// gallery images the lines carry.
func (s *Service) messageViews(ctx context.Context, rows []playdb.SessionMessage, v viewer, images map[string]*playv1.SessionMessageImage) ([]*playv1.SessionMessage, error) {
	readers, err := s.readersOf(ctx, rows, v)
	if err != nil {
		return nil, err
	}
	names, err := s.authorNames(ctx, rows)
	if err != nil {
		return nil, err
	}
	if images == nil {
		images = map[string]*playv1.SessionMessageImage{}
	}
	out := make([]*playv1.SessionMessage, 0, len(rows))
	for _, r := range rows {
		image, err := s.lineImage(ctx, r, images)
		if err != nil {
			return nil, err
		}
		out = append(out, messageToProto(r, image, readers[r.ID], names, s.msgs.voiceOn()))
	}
	return out, nil
}

// readersOf is, for the master, the readers of each restricted line. A player never gets them.
func (s *Service) readersOf(ctx context.Context, rows []playdb.SessionMessage, v viewer) (map[string][]string, error) {
	out := map[string][]string{}
	if !v.master {
		return out, nil
	}
	var ids []string
	for _, r := range rows {
		if !r.ForEveryone {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	found, err := s.queries.ListSessionMessageReaders(ctx, ids)
	if err != nil {
		return nil, s.dbError(ctx, "list the readers of the messages", err)
	}
	for _, f := range found {
		out[f.MessageID] = append(out[f.MessageID], f.UserID)
	}
	return out, nil
}

// authorNames reads the display names of the authors of out-of-character lines. An account that never chose
// a name has none, and an erased account has no author id at all: only the second says author_removed.
func (s *Service) authorNames(ctx context.Context, rows []playdb.SessionMessage) (map[string]string, error) {
	if s.msgs.profiles == nil {
		return nil, nil
	}
	var ids []string
	for _, r := range rows {
		if r.SpeakerKind == speakerOutOfCharacter && r.AuthorUserID != nil && !slices.Contains(ids, *r.AuthorUserID) {
			ids = append(ids, *r.AuthorUserID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	names, err := s.msgs.profiles.DisplayNames(ctx, ids)
	if err != nil {
		return nil, s.dbError(ctx, "read the display names", err)
	}
	return names, nil
}

// lineImage is the gallery image a line carries, nil when it has none or the master deleted it.
func (s *Service) lineImage(ctx context.Context, r playdb.SessionMessage, cache map[string]*playv1.SessionMessageImage) (*playv1.SessionMessageImage, error) {
	if r.ImageID == nil {
		return nil, nil
	}
	if image, ok := cache[*r.ImageID]; ok {
		return image, nil
	}
	shown, err := s.maps.ShownImage(ctx, r.CampaignID, *r.ImageID)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			cache[*r.ImageID] = nil
			return nil, nil
		}
		return nil, err
	}
	cache[*r.ImageID] = &playv1.SessionMessageImage{
		Id: shown.GetId(), Name: shown.GetName(), Width: shown.GetWidth(), Height: shown.GetHeight(), Url: shown.GetUrl(), ThumbnailUrl: shown.GetThumbnailUrl(),
	}
	return cache[*r.ImageID], nil
}

var speakerKindsOfRow = map[string]playv1.SessionMessageSpeakerKind{
	speakerNarrator:       playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_NARRATOR,
	speakerNPC:            playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_NPC,
	speakerCharacter:      playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_CHARACTER,
	speakerOutOfCharacter: playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_OUT_OF_CHARACTER,
}

// messageToProto shapes a row. readers is what the viewer may know of the readers (empty for a player);
// voiceOn says the server can voice a line, and only then does a narrator or NPC line offer its audio.
func messageToProto(r playdb.SessionMessage, image *playv1.SessionMessageImage, readers []string, names map[string]string, voiceOn bool) *playv1.SessionMessage {
	out := &playv1.SessionMessage{
		Id: r.ID, GameSessionId: r.GameSessionID, SpeakerKind: speakerKindsOfRow[r.SpeakerKind], SpeakerId: deref(r.SpeakerRef),
		SpeakerName: deref(r.SpeakerName), AuthorUserId: deref(r.AuthorUserID), AuthorRemoved: r.AuthorUserID == nil,
		Text: r.Text, Image: image, ForEveryone: r.ForEveryone, ReaderUserIds: readers, CreatedAt: timestamppb.New(r.CreatedAt),
	}
	if r.SpeakerKind == speakerOutOfCharacter && r.AuthorUserID != nil {
		// A present author who never chose a display name gets an empty name, and is still not "removed": that
		// flag is for an erased account, which has no author id left.
		out.SpeakerName = names[*r.AuthorUserID]
	}
	if r.SpeakerImageID != nil {
		out.SpeakerPortrait = &playv1.SessionMessageImage{Id: *r.SpeakerImageID, Url: imagesPath + *r.SpeakerImageID, ThumbnailUrl: imagesPath + *r.SpeakerImageID + "/thumb"}
	}
	if voiceOn && (r.SpeakerKind == speakerNarrator || r.SpeakerKind == speakerNPC) { // with the voice off the app shows no "Ouvir"
		out.CanHear, out.AudioUrl = true, AudioPath+r.ID+"/audio"
	}
	return out
}

// publishMessage tells the streams of a line that was added, to the readers and to nobody else
// (RN-10): everyone for a line for the table; for a line for chosen readers, the master (who also gets the
// readers), each chosen reader and the author. It runs after the commit.
func (s *Service) publishMessage(ctx context.Context, row playdb.SessionMessage, readers []string) {
	forPlayers, err := s.messageView(ctx, row, viewer{}, nil)
	if err != nil {
		s.logger.WarnContext(ctx, "play: cannot shape a message to publish it", "error", err)
		return
	}
	event := func(m *playv1.SessionMessage) *playv1.WatchGameSessionResponse {
		return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SessionMessageAdded_{
			SessionMessageAdded: &playv1.WatchGameSessionResponse_SessionMessageAdded{Message: m},
		}}
	}
	if row.ForEveryone {
		s.send(row.CampaignID, live.Event{Audience: live.Audience{Everyone: true}, Message: event(forPlayers)})
		return
	}
	forMaster, err := s.messageView(ctx, row, viewer{master: true}, nil)
	if err != nil {
		s.logger.WarnContext(ctx, "play: cannot shape a message to publish it", "error", err)
		return
	}
	s.send(row.CampaignID, live.Event{Audience: live.Audience{Master: true}, Message: event(forMaster)})
	chosen := forPlayers // the readers get the line without the list of readers
	targets := slices.Clone(readers)
	playerLine := row.SpeakerKind == speakerCharacter || row.SpeakerKind == speakerOutOfCharacter // the master's lines reach the master above
	if playerLine && row.AuthorUserID != nil && !slices.Contains(targets, *row.AuthorUserID) {
		targets = append(targets, *row.AuthorUserID)
	}
	for _, id := range targets {
		s.send(row.CampaignID, live.Event{Audience: live.Audience{UserID: id}, Message: event(chosen)})
	}
}

// ImageInConversation says whether the user may read a line that carries the gallery image, or that an
// NPC with it as its portrait speaks. It implements maps.LiveSession: the image route lets a player open
// that image (RN-36), and nothing else a line mentions, the same way "mostrar imagem" does.
func (s *Service) ImageInConversation(ctx context.Context, campaignID, userID, imageID string) (bool, error) {
	ok, err := s.queries.ImageInReadableMessage(ctx, playdb.ImageInReadableMessageParams{CampaignID: campaignID, ImageID: &imageID, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("check the image of a line: %w", err)
	}
	return ok, nil
}
