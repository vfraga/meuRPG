// Package sessiontell is the little a module that does not own the live session may tell its streams through the play module's
// InSession: a few hints, and nothing of the hub or the session's number. The play module implements it; the modules that call
// InSession declare the host interface themselves, with this type as the argument of their closure.
package sessiontell

import (
	"context"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// Tell collects what a write tells the live streams once its transaction committed. The session's number moves once if the write
// called any of them.
type Tell interface {
	// Touch marks a write that sends no event but changes what the screens show.
	Touch()
	// CreaturesChanged tells the master and the owner's player that their creature lists changed.
	CreaturesChanged(ownerUserID string)
	// EncounterChanged tells the streams that a combat changed (an empty id tells nothing).
	EncounterChanged(encounterID string)
	// CombatLogChangedFull tells the master, and only the master, that the log of the combat in progress must be read again from
	// the start: the table's rules decide what its lines show.
	CombatLogChangedFull()
	// After runs fn once the transaction committed, after the events told: what fn sends through out carries the session's
	// number, and out reaches only the audiences Audience names. It is for a write that works out its hints from what it
	// committed (who sees which change), which a read inside the transaction cannot tell. After moves nothing by itself: the write
	// calls Touch. fn's context has the request's values and no cancellation.
	After(fn func(ctx context.Context, out Out))
	// XPChanged tells every stream of the campaign that the party, its levels or its experience changed (a content-free hint).
	XPChanged()
	// VitalsChanged tells the master and the character's player the character's vitals as they stand once the transaction committed.
	VitalsChanged(characterID string)
	// CharacterEvent tells the master and the owner (ownerUserID, empty for none) that a review changed or a dead character lives again
	// (event "changes_requested", "resubmitted" or "revived"); a content-free hint.
	CharacterEvent(ownerUserID, characterID, event string)
	// Replay marks the call as the repeat of a write already made (an idempotency key stored): nothing was written, so the number
	// does not move, and what the steps of After send carries the number as it stands.
	Replay()
}

// Audience says who hears an event. The zero Audience reaches nobody.
type Audience struct {
	// Master reaches the campaign's master, Players every player.
	Master, Players bool
	// Users reaches exactly these accounts' streams, and nobody else.
	Users []string
	// Scene reaches the master and the players of the open scene (everyone when it is open to the whole table or none is open).
	Scene bool
	// Coalesce, with Users, names a hint that says "read it again": while a stream still has one with the same name waiting, no
	// other is queued for it.
	Coalesce string
}

// Out is where the steps of After send: an event stamped with the session's number, to the audience named.
type Out interface {
	// Send tells ev (a play.v1 WatchGameSessionResponse) to the audience. ev is not shared: Send may stamp it.
	Send(ctx context.Context, to Audience, ev proto.Message)
}

// Host runs a write of another module that changes what a live session shows, under the play module's funnel: the open session's row
// is taken first, fn writes on that transaction and tells through t, and once it committed the session's number moves once if fn told
// and what fn told is sent with it. With no open session fn runs in a plain transaction and t tells nothing. play.Service implements it.
type Host interface {
	InSession(ctx context.Context, campaignID string, fn func(tx pgx.Tx, t Tell) error) error
}

// None is the Tell of a write that runs with no play module: it tells nothing. A type that keeps some of the hints embeds it.
type None struct{}

// Touch tells nothing.
func (None) Touch() {}

// CreaturesChanged tells nothing.
func (None) CreaturesChanged(string) {}

// EncounterChanged tells nothing.
func (None) EncounterChanged(string) {}

// CombatLogChangedFull tells nothing.
func (None) CombatLogChangedFull() {}

// XPChanged tells nothing.
func (None) XPChanged() {}

// VitalsChanged tells nothing.
func (None) VitalsChanged(string) {}

// CharacterEvent tells nothing.
func (None) CharacterEvent(string, string, string) {}

// After drops the step.
func (None) After(func(context.Context, Out)) {}

// Replay does nothing: there is no number to keep.
func (None) Replay() {}

// Direct is the Host of a module that runs with no play module (a tool, a test with no session): fn runs in a plain transaction,
// the number never moves, and what the steps of After send goes out through Out(ctx, campaignID) once the transaction committed, with no
// number.
type Direct struct {
	Pool db.TxStarter
	Out  func(ctx context.Context, campaignID string) Out
}

type direct struct {
	None
	steps []func(ctx context.Context, out Out)
}

func (d *direct) After(fn func(ctx context.Context, out Out)) { d.steps = append(d.steps, fn) }

// InSession runs fn in a transaction of its own and, once it committed, the steps fn collected.
func (d Direct) InSession(ctx context.Context, campaignID string, fn func(tx pgx.Tx, t Tell) error) error {
	var t *direct
	if err := db.InTx(ctx, d.Pool, func(tx pgx.Tx) error {
		t = &direct{}
		return fn(tx, t)
	}); err != nil {
		return err
	}
	if len(t.steps) > 0 && d.Out != nil {
		after := context.WithoutCancel(ctx)
		out := d.Out(after, campaignID)
		for _, step := range t.steps {
			step(after, out)
		}
	}
	return nil
}

type outKey struct{}

// WithOut is ctx for a step that sends through out: a module's publish helpers read it with OutFrom, so what the step tells carries
// the session's number.
func WithOut(ctx context.Context, out Out) context.Context {
	return context.WithValue(ctx, outKey{}, out)
}

// OutFrom is the Out a step carries in its context (WithOut), if any.
func OutFrom(ctx context.Context) (Out, bool) {
	out, ok := ctx.Value(outKey{}).(Out)
	return out, ok
}
