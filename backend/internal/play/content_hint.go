package play

import (
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// PublishContentChanged tells every stream of the campaign that the table's
// content changed (MR-025, RN-23): the master wrote or archived an entry, or
// switched options on or off. The hint carries nothing (RN-10): each app reads the
// content again as its own role. At most one goes out per campaign every 250 ms
// (the puzzles' gate), the last change never left unannounced, and one that waits
// in a stream's queue is not queued twice. Call it after the commit.
func (s *Service) PublishContentChanged(campaignID string) {
	every := s.puzzles.hintEvery
	if every <= 0 {
		every = defaultHintEvery
	}
	s.puzzles.hints.fire("content:"+campaignID, every, func() {
		s.send(campaignID, live.Event{
			Audience: live.Audience{Everyone: true},
			Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_ContentChanged_{
				ContentChanged: &playv1.WatchGameSessionResponse_ContentChanged{},
			}},
			Coalesce: "content",
		})
	})
}
