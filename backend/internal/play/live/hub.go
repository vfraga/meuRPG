// Package live fans the live session's events out to the members watching
// it (PlayService.WatchGameSession, ADR-0005).
//
// A Hub keeps, per campaign, the subscriptions of the members who have the
// session open. Whoever changes something at the table (package play)
// publishes an Event with its Audience: who may receive it. The hub hands
// each event only to the subscriptions in its audience, so a player never
// receives what only the master may see (RN-10, RN-11): the filter runs on
// the server, before anything is sent.
//
// Publishing never waits for a subscriber. Each subscription has a small
// buffer; one whose buffer is full (the app stopped reading, or the
// network stalled) is dropped: its channel closes, its stream ends, and the
// app reconnects and reads the snapshot again. A slow phone never holds the
// master's change back.
//
// The hub lives in the server's memory. With more than one server
// instance, a change made on one instance would never reach the streams
// open on another, so the service runs with Cloud Run max-instances = 1
// while fan-out stays in memory (docs/operations/README.md). A shared channel, such
// as CockroachDB changefeeds or Pub/Sub, replaces it when one instance is no
// longer enough.
package live

import (
	"errors"
	"slices"
	"sync"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// DefaultBuffer is how many events a subscription holds before it is
// dropped as too slow. Changes at a table come a few per minute; 16 is
// plenty for a burst, and small enough to notice a stream nobody reads.
const DefaultBuffer = 16

// DefaultMaxPerUser is how many streams one user may hold open for one
// campaign: a phone, a laptop and a spare tab or two. Each stream is a
// goroutine, a buffer and one of the instance's 50 concurrent requests, and any
// member may open them, so without a cap one account could take the instance.
//
// A new stream past the cap replaces the user's oldest one (ErrReplaced) rather
// than being refused. Behind the load balancer the server does not learn that a
// page went away: the request is not canceled and the heartbeats keep being
// accepted, so the stream lives until its maximum life (measured on production
// on 10/10/2026: of 85 streams, none ended between 1 and 30 minutes). Each
// reload or return to the session page leaves one behind; the newest call is
// the page the person is looking at, and the oldest is the likeliest dead.
const DefaultMaxPerUser = 4

// Why a subscription ended. Err returns one of these once Events is closed.
var (
	// ErrSlow: the subscriber did not keep up and was dropped.
	ErrSlow = errors.New("live: the subscriber fell behind and was dropped")
	// ErrClosed: the hub closed, because the server is shutting down.
	ErrClosed = errors.New("live: the hub is closed")
	// ErrReplaced: the user opened one stream more than the cap on this
	// campaign, and this one, their oldest, made room for it.
	ErrReplaced = errors.New("live: replaced by a newer stream of the same user")
)

// Subscriber is who watches: their account, and whether they are the
// campaign's master.
type Subscriber struct {
	UserID string
	Master bool
	// Screen marks the stream of the master's TV tab. It hears what the master hears, and an event
	// with Audience.NoScreens skips it. Its UserID is its own, so its streams are capped apart from
	// the master's own.
	Screen bool
}

// Audience says who may receive an event. The zero Audience reaches nobody.
type Audience struct {
	// Everyone watching the campaign: the master and every player.
	Everyone bool
	// The campaign's master.
	Master bool
	// Every player of the campaign, not the master: for an event the master
	// gets in another form (a combat's turn, where a hidden combatant's name
	// is the master's alone).
	Players bool
	// One more user, such as the player of the character the event is
	// about. Empty for none.
	UserID string
	// Except lists players Players does not reach: "the players but these". It narrows Players only.
	Except []string
	// NoScreens leaves out the streams of the master's TV tab (Subscriber.Screen), which hear every
	// event of the master's audience otherwise. An event about the TV tabs themselves sets it, so that a
	// tab connecting does not tell the tabs.
	NoScreens bool
}

// includes reports whether s may receive an event for this audience.
func (a Audience) includes(s Subscriber) bool {
	if a.NoScreens && s.Screen {
		return false
	}
	return a.Everyone || (a.Master && s.Master) || (a.Players && !s.Master && !slices.Contains(a.Except, s.UserID)) || (a.UserID != "" && a.UserID == s.UserID)
}

// Event is one live change, and who may receive it.
type Event struct {
	Audience Audience
	// Message is what the stream sends. It must not be changed after
	// Publish: several streams send the same message.
	Message *playv1.WatchGameSessionResponse
	// Coalesce, when not empty, names a hint that says "read it again": while a
	// stream still has an event with the same Coalesce waiting in its queue, a
	// new one for that stream is not queued, since the app will read the latest
	// state when it handles the one that waits. The stream calls
	// Subscription.Taken when it takes the event off the queue, so a change that
	// follows is queued again. The vision hints use it (`vision_changed`).
	Coalesce string
}

// Hub is the in-memory fan-out. The zero Hub is not usable: call New.
type Hub struct {
	buffer     int
	maxPerUser int

	mu     sync.Mutex
	subs   map[string]map[*Subscription]struct{} // by campaign ID
	seq    uint64                                // the order subscriptions were made in, under mu
	closed bool
}

// New returns a Hub whose subscriptions hold buffer events each (0 means
// DefaultBuffer).
func New(buffer int) *Hub {
	if buffer <= 0 {
		buffer = DefaultBuffer
	}
	return &Hub{buffer: buffer, maxPerUser: DefaultMaxPerUser, subs: map[string]map[*Subscription]struct{}{}}
}

// Subscription is one member watching one campaign. Read Events until it
// closes, then Err says why; call Close when done watching.
type Subscription struct {
	hub        *Hub
	campaignID string
	who        Subscriber
	events     chan Event
	seq        uint64              // when it was made, among the hub's subscriptions
	pending    map[string]struct{} // the Coalesce keys queued in events, under hub.mu
	err        error               // set, under hub.mu, before events closes
}

// SetMaxPerUser changes how many streams a user may hold per campaign (tests
// use it; the default is DefaultMaxPerUser). Call it before any Subscribe.
func (h *Hub) SetMaxPerUser(n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.maxPerUser = n
}

// Subscribe starts delivering the campaign's events for who. It fails with
// ErrClosed once the hub is closed. When who already has the most streams
// allowed on this campaign, their oldest one ends with ErrReplaced to make
// room for this one.
func (h *Hub) Subscribe(campaignID string, who Subscriber) (*Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, ErrClosed
	}
	held := 0
	var oldest *Subscription
	for other := range h.subs[campaignID] {
		if other.who.UserID == who.UserID {
			held++
			if oldest == nil || other.seq < oldest.seq {
				oldest = other
			}
		}
	}
	if held >= h.maxPerUser {
		h.remove(oldest, ErrReplaced)
	}
	h.seq++
	s := &Subscription{hub: h, campaignID: campaignID, who: who, events: make(chan Event, h.buffer), seq: h.seq}
	if h.subs[campaignID] == nil {
		h.subs[campaignID] = map[*Subscription]struct{}{}
	}
	h.subs[campaignID][s] = struct{}{}
	return s, nil
}

// Publish hands ev to every subscription of the campaign in its audience,
// without waiting: a subscription whose buffer is full is dropped
// (ErrSlow). It reports how many subscriptions received the event.
func (h *Hub) Publish(campaignID string, ev Event) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	delivered := 0
	for s := range h.subs[campaignID] {
		if ev.Audience.includes(s.who) && h.deliver(s, ev) {
			delivered++
		}
	}
	return delivered
}

// deliver queues ev for s without waiting, and reports whether it did: not when the one
// that waits under the same Coalesce key will do, and a full buffer drops s (ErrSlow). The
// caller holds h.mu.
func (h *Hub) deliver(s *Subscription, ev Event) bool {
	if ev.Coalesce != "" {
		if _, queued := s.pending[ev.Coalesce]; queued {
			return false // the one that waits will make the app read the latest state
		}
	}
	select {
	case s.events <- ev:
		if ev.Coalesce != "" {
			if s.pending == nil {
				s.pending = map[string]struct{}{}
			}
			s.pending[ev.Coalesce] = struct{}{}
		}
		return true
	default:
		h.remove(s, ErrSlow)
		return false
	}
}

// Close ends every subscription (ErrClosed) and refuses new ones. The
// server calls it when its graceful shutdown starts, so open streams end
// right away instead of holding the shutdown until its deadline. Closing
// twice is fine.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for _, subs := range h.subs {
		for s := range subs {
			h.remove(s, ErrClosed)
		}
	}
}

// Count returns how many subscriptions the campaign has, for tests and
// logs.
func (h *Hub) Count(campaignID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[campaignID])
}

// remove drops s and closes its channel, with err as the reason. The
// caller holds h.mu; sending and closing both happen under it, so nothing
// ever sends on a closed channel.
func (h *Hub) remove(s *Subscription, err error) {
	subs := h.subs[s.campaignID]
	if _, ok := subs[s]; !ok {
		return // already removed
	}
	delete(subs, s)
	if len(subs) == 0 {
		delete(h.subs, s.campaignID)
	}
	s.err = err
	close(s.events)
}

// Events delivers the subscription's events. It closes when the
// subscription ends: Close, too slow, replaced by a newer stream of the same
// user, or the hub closed.
func (s *Subscription) Events() <-chan Event { return s.events }

// Taken tells the hub that the stream took ev off the queue, so another event
// with the same Coalesce key may be queued. A stream calls it for each event it
// reads from Events, before it sends it.
func (s *Subscription) Taken(ev Event) {
	if ev.Coalesce == "" {
		return
	}
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	delete(s.pending, ev.Coalesce)
}

// Err says why Events closed: ErrSlow, ErrReplaced, ErrClosed, or nil after Close. Call
// it only after Events has closed.
func (s *Subscription) Err() error {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	return s.err
}

// Close ends the subscription. Closing twice, or after the hub dropped it,
// is fine.
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.hub.remove(s, nil)
}
