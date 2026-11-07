package live

import (
	"errors"
	"sync"
	"testing"
	"time"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

const (
	campaignA = "campaign-a"
	campaignB = "campaign-b"
)

var (
	master = Subscriber{UserID: "master", Master: true}
	ana    = Subscriber{UserID: "ana"}
	bruno  = Subscriber{UserID: "bruno"}
)

func heartbeat() Event {
	return Event{Audience: Audience{Everyone: true}, Message: &playv1.WatchGameSessionResponse{
		Event: &playv1.WatchGameSessionResponse_Heartbeat_{Heartbeat: &playv1.WatchGameSessionResponse_Heartbeat{}},
	}}
}

func subscribe(t *testing.T, h *Hub, campaignID string, who Subscriber) *Subscription {
	t.Helper()
	s, err := h.Subscribe(campaignID, who)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// received drains what s has buffered right now.
func received(s *Subscription) int {
	n := 0
	for {
		select {
		case _, ok := <-s.Events():
			if !ok {
				return n
			}
			n++
		default:
			return n
		}
	}
}

// TestAudience: each event reaches only the subscriptions in its audience,
// and only in its campaign. A player never receives what is only for the
// master or for another player (RN-10, RN-11).
func TestAudience(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		audience Audience
		// master, ana, bruno, and ana in another campaign
		want [4]int
	}{
		{"nobody", Audience{}, [4]int{0, 0, 0, 0}},
		{"everyone", Audience{Everyone: true}, [4]int{1, 1, 1, 0}},
		{"master only", Audience{Master: true}, [4]int{1, 0, 0, 0}},
		{"master and Ana's character", Audience{Master: true, UserID: ana.UserID}, [4]int{1, 1, 0, 0}},
		{"only Bruno", Audience{UserID: bruno.UserID}, [4]int{0, 0, 1, 0}},
		{"the players except Ana", Audience{Players: true, Except: []string{ana.UserID}}, [4]int{0, 0, 1, 0}},
		{"the master and the players except Bruno", Audience{Master: true, Players: true, Except: []string{bruno.UserID}}, [4]int{1, 1, 0, 0}},
		{"everyone is not narrowed by except", Audience{Everyone: true, Except: []string{ana.UserID}}, [4]int{1, 1, 1, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := New(0)
			subs := []*Subscription{
				subscribe(t, h, campaignA, master),
				subscribe(t, h, campaignA, ana),
				subscribe(t, h, campaignA, bruno),
				subscribe(t, h, campaignB, ana),
			}
			ev := heartbeat()
			ev.Audience = tt.audience
			delivered := h.Publish(campaignA, ev)
			total := 0
			for i, s := range subs {
				got := received(s)
				total += got
				if got != tt.want[i] {
					t.Errorf("subscription %d received %d events, want %d", i, got, tt.want[i])
				}
			}
			if delivered != total {
				t.Errorf("Publish() = %d, but %d were received", delivered, total)
			}
		})
	}
}

// TestSlowSubscriberIsDropped: a full buffer drops that subscription
// without blocking the publisher or the other subscribers.
func TestSlowSubscriberIsDropped(t *testing.T) {
	t.Parallel()
	h := New(2)
	slow := subscribe(t, h, campaignA, ana)
	fast := subscribe(t, h, campaignA, master)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 3 {
			h.Publish(campaignA, heartbeat())
			// The master reads each event as it comes.
			<-fast.Events()
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish() blocked on a slow subscriber")
	}

	// The slow one got its 2 buffered events, then its channel closed.
	if got := received(slow); got != 2 {
		t.Errorf("slow subscriber received %d events, want its 2 buffered ones", got)
	}
	if _, ok := <-slow.Events(); ok {
		t.Error("slow subscriber's channel is still open")
	}
	if !errors.Is(slow.Err(), ErrSlow) {
		t.Errorf("slow subscriber Err() = %v, want ErrSlow", slow.Err())
	}
	if got := h.Count(campaignA); got != 1 {
		t.Errorf("Count() = %d, want 1 (the master)", got)
	}
}

// TestCloseEndsEverySubscription: the server's shutdown ends every stream
// at once, and no new one starts.
func TestCloseEndsEverySubscription(t *testing.T) {
	t.Parallel()
	h := New(0)
	a := subscribe(t, h, campaignA, master)
	b := subscribe(t, h, campaignB, ana)
	h.Close()
	h.Close() // twice is fine
	for _, s := range []*Subscription{a, b} {
		if _, ok := <-s.Events(); ok {
			t.Error("a subscription is still open after Close")
		}
		if !errors.Is(s.Err(), ErrClosed) {
			t.Errorf("Err() = %v, want ErrClosed", s.Err())
		}
	}
	if _, err := h.Subscribe(campaignA, ana); !errors.Is(err, ErrClosed) {
		t.Errorf("Subscribe() after Close error = %v, want ErrClosed", err)
	}
	if got := h.Publish(campaignA, heartbeat()); got != 0 {
		t.Errorf("Publish() after Close = %d, want 0", got)
	}
}

// TestSubscriptionClose: closing a subscription stops its events; closing
// again is fine.
func TestSubscriptionClose(t *testing.T) {
	t.Parallel()
	h := New(0)
	s := subscribe(t, h, campaignA, ana)
	s.Close()
	s.Close()
	if _, ok := <-s.Events(); ok {
		t.Error("channel still open after Close")
	}
	if s.Err() != nil {
		t.Errorf("Err() after Close = %v, want nil", s.Err())
	}
	if got := h.Publish(campaignA, heartbeat()); got != 0 {
		t.Errorf("Publish() after the only subscriber left = %d, want 0", got)
	}
	if got := h.Count(campaignA); got != 0 {
		t.Errorf("Count() = %d, want 0", got)
	}
}

// TestConcurrentUse runs publishers, subscribers joining and leaving, and
// Close at the same time. It has no assertions of its own: go test -race
// is the check.
func TestConcurrentUse(t *testing.T) {
	t.Parallel()
	h := New(4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 200 {
				h.Publish(campaignA, heartbeat())
			}
		})
	}
	for _, userID := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		wg.Go(func() {
			for range 20 {
				s, err := h.Subscribe(campaignA, Subscriber{UserID: userID})
				if err != nil {
					return // closed
				}
				for range 3 {
					if _, ok := <-s.Events(); !ok {
						break
					}
				}
				s.Close()
				_ = s.Err()
			}
		})
	}
	wg.Go(func() {
		time.Sleep(10 * time.Millisecond)
		h.Close()
	})
	// Keep publishing until the subscribers are done, so none waits forever.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				h.Publish(campaignA, heartbeat())
			}
		}
	}()
	wg.Wait()
	close(stop)
}

// TestCoalescedHintsQueueOnce: while a stream still has a hint with the same
// Coalesce key waiting in its queue, another is not queued for it; once the
// stream took it (Taken), the next is. Events with no key, and the streams
// that are not waiting, are not affected (the `vision_changed` hint, D6).
func TestCoalescedHintsQueueOnce(t *testing.T) {
	t.Parallel()
	h := New(0)
	waiting, reading, other := subscribe(t, h, campaignA, ana), subscribe(t, h, campaignA, bruno), subscribe(t, h, campaignB, ana)
	hint := func(key string) Event {
		ev := heartbeat()
		ev.Coalesce = key
		return ev
	}
	for range 3 {
		h.Publish(campaignA, hint("vision:1"))
	}
	if got := received(waiting); got != 1 {
		t.Errorf("a stream that never read got %d hints for 3 with the same key, want 1", got)
	}
	// Reading takes it off the queue: the next change is queued again.
	ev := <-reading.Events()
	reading.Taken(ev)
	h.Publish(campaignA, hint("vision:1"))
	if got := received(reading); got != 1 {
		t.Errorf("a stream that read its hint got %d more, want 1", got)
	}
	// Another key, and an event with no key, always queue.
	h.Publish(campaignA, hint("vision:2"))
	h.Publish(campaignA, heartbeat())
	h.Publish(campaignA, heartbeat())
	if got := received(waiting); got != 1+2 {
		t.Errorf("a stream that never read got %d events for another key and two plain ones, want 3", got)
	}
	if got := received(other); got != 0 {
		t.Errorf("another campaign's stream got %d events, want none", got)
	}
}

// Past the cap, a user's new stream replaces their oldest one: behind the load
// balancer the server is not told that a page went away, so after a few reloads
// the old streams are usually dead, and the newest is the page in use.
func TestSubscribePastTheCapReplacesTheUsersOldestStream(t *testing.T) {
	t.Parallel()
	h := New(0)
	h.SetMaxPerUser(2)
	a1 := subscribe(t, h, campaignA, ana)
	a2 := subscribe(t, h, campaignA, ana)
	// The cap is per user and per campaign: these take nobody's place.
	b1 := subscribe(t, h, campaignA, bruno)
	other := subscribe(t, h, campaignB, ana)

	a3 := subscribe(t, h, campaignA, ana)
	if _, ok := <-a1.Events(); ok {
		t.Fatal("the oldest stream is still open after a third one of the same user")
	}
	if !errors.Is(a1.Err(), ErrReplaced) {
		t.Errorf("oldest stream Err() = %v, want ErrReplaced", a1.Err())
	}
	if got := h.Count(campaignA); got != 3 {
		t.Errorf("Count(a) = %d, want 3: ana's two newest and bruno's", got)
	}
	// The next one replaces the next oldest, never the newest.
	subscribe(t, h, campaignA, ana)
	if !errors.Is(a2.Err(), ErrReplaced) {
		t.Errorf("second stream Err() = %v, want ErrReplaced", a2.Err())
	}
	if a3.Err() != nil || b1.Err() != nil || other.Err() != nil {
		t.Errorf("a stream that was not the user's oldest here ended: a3 %v, bruno %v, other campaign %v", a3.Err(), b1.Err(), other.Err())
	}
	// The newest ones still receive events.
	h.Publish(campaignA, heartbeat())
	if got := len(a3.Events()); got != 1 {
		t.Errorf("the newest stream has %d events queued, want 1", got)
	}
}

func TestDefaultMaxPerUser(t *testing.T) {
	t.Parallel()
	h := New(0)
	first := subscribe(t, h, campaignA, ana)
	for range DefaultMaxPerUser - 1 {
		subscribe(t, h, campaignA, ana)
	}
	if first.Err() != nil {
		t.Fatalf("a stream ended before the cap: %v", first.Err())
	}
	subscribe(t, h, campaignA, ana)
	if !errors.Is(first.Err(), ErrReplaced) {
		t.Errorf("after %d streams, the first one's Err() = %v, want ErrReplaced", DefaultMaxPerUser+1, first.Err())
	}
	if got := h.Count(campaignA); got != DefaultMaxPerUser {
		t.Errorf("Count() = %d, want %d", got, DefaultMaxPerUser)
	}
}

// An event with NoScreens reaches the master's session streams and not the TV tab's, which hears the rest
// of what the master hears.
func TestAudienceNoScreensSkipsTheTVStream(t *testing.T) {
	t.Parallel()
	h := New(0)
	session, err := h.Subscribe("c", Subscriber{UserID: "m", Master: true})
	if err != nil {
		t.Fatal(err)
	}
	screen, err := h.Subscribe("c", Subscriber{UserID: "tv:m", Master: true, Screen: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Publish("c", Event{Audience: Audience{Master: true, NoScreens: true}}); got != 1 {
		t.Errorf("a NoScreens event reached %d streams, want 1", got)
	}
	if len(session.Events()) != 1 || len(screen.Events()) != 0 {
		t.Errorf("queued: session %d, screen %d; want 1 and 0", len(session.Events()), len(screen.Events()))
	}
	if got := h.Publish("c", Event{Audience: Audience{Master: true}}); got != 2 {
		t.Errorf("a master event reached %d streams, want both", got)
	}
}
