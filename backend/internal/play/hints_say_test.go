package play

import (
	"testing"

	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// sayWith stamps what it sends, never the caller's message: a publisher may send one message to several audiences, and a stream
// may already be marshaling an earlier send of it (#735).
func TestSayWithStampsACopyAndLeavesTheCallersMessageAlone(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	var sent []live.Event
	svc.setDropSend(func(_ string, ev live.Event) bool { sent = append(sent, ev); return true })

	msg := &playv1.WatchGameSessionResponse{}
	say := svc.sayWith(5)
	say("c1", live.Event{Audience: live.Audience{UserID: "u1"}, Message: msg})
	say("c1", live.Event{Audience: live.Audience{UserID: "u2"}, Message: msg})

	if msg.GetSessionSeq() != 0 {
		t.Errorf("the caller's message carries number %d after sayWith, want 0 (it is shared)", msg.GetSessionSeq())
	}
	if len(sent) != 2 {
		t.Fatalf("sent %d events, want 2", len(sent))
	}
	for i, ev := range sent {
		if ev.Message.GetSessionSeq() != 5 {
			t.Errorf("event %d carries number %d, want 5", i, ev.Message.GetSessionSeq())
		}
	}
	if sent[0].Message == sent[1].Message || sent[0].Message == msg {
		t.Errorf("the sends share one message: each must carry its own copy")
	}
}

// A message sent to two audiences while the stream of the first is marshaling it: the race detector fails the test if sayWith stamps
// the message the stream holds.
func TestSayWithOfOneMessageToTwoAudiencesDoesNotRaceWithAStreamMarshallingIt(t *testing.T) {
	t.Parallel()
	svc := &Service{hub: live.New(0)}
	const rounds = 200
	subs := make([]*live.Subscription, 2)
	done := make(chan struct{}, 2)
	for i := range subs {
		sub, err := svc.hub.Subscribe("c1", live.Subscriber{UserID: string(rune('a' + i))})
		if err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
		subs[i] = sub
		go func() {
			defer func() { done <- struct{}{} }()
			for range rounds {
				ev := <-sub.Events()
				if _, err := proto.Marshal(ev.Message); err != nil { // what a stream does with it
					t.Errorf("Marshal() error = %v", err)
				}
			}
		}()
	}
	say := svc.sayWith(5)
	for range rounds {
		msg := &playv1.WatchGameSessionResponse{}
		say("c1", live.Event{Audience: live.Audience{UserID: "a"}, Message: msg})
		say("c1", live.Event{Audience: live.Audience{UserID: "b"}, Message: msg})
	}
	<-done
	<-done
}
