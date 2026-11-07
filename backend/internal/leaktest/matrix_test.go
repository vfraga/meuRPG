package leaktest

import (
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// read is one row of the table of player-facing reads: one call, asked of every
// person in the fixture. To add a read, add a row to `reads` (reads_test.go).
type read struct {
	// procedure is the generated constant, e.g. mapsv1connect.MapServiceGetMapProcedure.
	procedure string
	// label tells rows of the same procedure apart ("a hidden map", "as Ana's character").
	label string
	// req builds the request. A read whose request depends on who asks (a read of "my
	// character" asks for a different id for each) sets reqFor instead.
	req    func(w *world) proto.Message
	reqFor func(w *world, p *person) proto.Message
	// allow says who gets an answer besides the master, who always does (and whose
	// answer is the positive control). Everyone else must be refused: a pending member and
	// a stranger always are, and a player is when the thing is not theirs to read.
	allow func(w *world) []*person
	// ignore lists the canary kinds this answer may hold: public SRD data, which a
	// player reads whatever the master hid (a bestiary names every creature).
	ignore []string
	// why explains a row that is not the obvious one (an ignore, a master-only read).
	why string
	// notMaster marks a read the master may not make (a player's own ability rolls):
	// there is no positive control from the master, and the readers' answers are the control.
	notMaster bool
	// once marks a read that depends on whose turn it is: it is asked before the stream
	// test's changes (the turn passes then), not again after them.
	once bool
	// asTV marks a read whose answer to the master is what the TV at the table shows: it is inspected
	// as the reader of the TV (tvReader), who reads what every player reads and nothing of what only one
	// does, and the master's own reads are its positive control (checkCanariesAreReachable).
	asTV bool
	// tvLog marks an asTV row of a world with a combat whose log has a line by the time it is asked: the TV's answer
	// must carry at least one, so that the check for a name in the log cannot pass on an empty log.
	tvLog bool
}

// request is the request person p sends.
func (r read) request(w *world, p *person) proto.Message {
	if r.reqFor != nil {
		return r.reqFor(w, p)
	}
	return r.req(w)
}

func (r read) name() string {
	n := strings.TrimPrefix(r.procedure, "/meurpg.")
	if r.label != "" {
		n += " (" + r.label + ")"
	}
	return n
}

// who a row lets in.
func members(w *world) []*person { return []*person{w.ana, w.caio} }
func onlyAna(w *world) []*person { return []*person{w.ana} }
func masterOnlyRead(*world) []*person {
	return nil
}

// players are the people who must never read what the master hid.
func (w *world) players() []*person { return []*person{w.ana, w.caio, w.pending, w.stranger} }

// answers remembers what each person read in the matrix, so the canaries can be
// proved reachable: a needle nobody can find in the master's answers tests nothing.
type answers struct {
	bodies map[string][][]byte // by person name
	msgs   map[string][]proto.Message
}

func newAnswers() *answers {
	return &answers{bodies: map[string][][]byte{}, msgs: map[string][]proto.Message{}}
}

func (a *answers) keep(p *person, r reply) {
	a.bodies[p.name] = append(a.bodies[p.name], r.body)
	if r.msg != nil {
		a.msgs[p.name] = append(a.msgs[p.name], r.msg)
	}
}

// holds says whether anything p read holds the canary: the marker or the id in
// some body, or the number in some numeric field.
func (a *answers) holds(p *person, c *canary) bool {
	if !c.numeric {
		for _, b := range a.bodies[p.name] {
			if strings.Contains(string(b), c.needle) {
				return true
			}
		}
		return false
	}
	for _, m := range a.msgs[p.name] {
		found := false
		walk(m.ProtoReflect(), "", func(_ string, _ protoreflect.Message, fd protoreflect.FieldDescriptor, v protoreflect.Value) {
			if isNumber(fd) && c.matchesNumber(fd, asInt(fd, v)) {
				found = true
			}
		})
		if found {
			return true
		}
	}
	return false
}

// runRead asks one row of every person and checks the answers.
func (w *world) runRead(t *testing.T, r read, got *answers) {
	t.Helper()
	// The master's answer is the positive control: the call works and the thing is there.
	mr := w.master.call(r.procedure, r.request(w, w.master))
	switch {
	case r.notMaster && mr.ok():
		t.Errorf("the master's call worked, but the row says it is not the master's")
	case !r.notMaster && !mr.ok():
		t.Errorf("the master's call failed: HTTP %d %s", mr.status, mr.body)
	}
	got.keep(w.master, mr)
	if r.asTV && mr.ok() {
		tv := w.tvReader()
		got.keep(tv, mr)
		for _, f := range w.inspect(tv, mr, r.ignore) {
			t.Errorf("the TV: %s\n\tanswer: %s", f, shorten(mr.body))
		}
		if r.tvLog {
			screen := &playv1.GetTableScreenResponse{}
			if err := protojson.Unmarshal(mr.body, screen); err != nil || len(screen.GetStage().GetCombat().GetLog()) == 0 {
				t.Errorf("the TV's combat carries no log line: the check for a table name in it proves nothing\n\tanswer: %s", shorten(mr.body))
			}
		}
	}

	allowed := r.allow(w)
	for _, p := range w.players() {
		reply := p.call(r.procedure, r.request(w, p))
		got.keep(p, reply)
		switch want := slices.Contains(allowed, p); {
		case want && !reply.ok():
			t.Errorf("%s was refused (HTTP %d %s), but this read is theirs: the row proves nothing", p.name, reply.status, reply.body)
		case !want && reply.ok():
			t.Errorf("%s got an answer (%s), but this read is not theirs", p.name, shorten(reply.body))
		}
		for _, f := range w.inspect(p, reply, r.ignore) {
			t.Errorf("%s: %s\n\tanswer: %s", p.name, f, shorten(reply.body))
		}
	}
}

func shorten(b []byte) string {
	if len(b) > 400 {
		return string(b[:400]) + "..."
	}
	return string(b)
}

// membersAndPending are who a read documented for "a pending member too" lets in.
func membersAndPending(w *world) []*person { return []*person{w.ana, w.caio, w.pending} }

func everyone(w *world) []*person { return []*person{w.ana, w.caio, w.pending, w.stranger} }
func onlyCaio(w *world) []*person { return []*person{w.caio} }

// combatant finds the combatant of a character in the combat the fixture started.
func (w *world) combatant(ch interface{ GetId() string }) *playv1.Combatant {
	return w.combatantOf(w.encounter, ch.GetId())
}

// keyOf is the key the master's table content got (entries made by buildTableContent).
func (w *world) keyOf(kind string) string {
	for _, c := range w.secrets.list {
		if c.kind == kind+"-key-id" {
			return c.needle
		}
	}
	panic("no key registered for " + kind)
}

// TestLeakMatrix asks every read of every person against the fixture's state: a
// session open, a combat running, a scene open, puzzles shown (see world_test.go).
func TestLeakMatrix(t *testing.T) {
	w := newWorld(t)
	got := newAnswers()
	w.assertFogShape(t)
	// What the players do first, so the reads see the state it leaves.
	for _, r := range actions {
		t.Run("action "+r.name(), func(t *testing.T) { w.runRead(t, r, got) })
	}
	for _, r := range reads {
		t.Run(r.name(), func(t *testing.T) { w.runRead(t, r, got) })
	}
	t.Run("images and tiles", func(t *testing.T) { checkImagesAndTiles(t, w) })
	t.Run("the audio of the lines", func(t *testing.T) { checkLineAudio(t, w) })
	t.Run("calls that are not reads", func(t *testing.T) { checkNotReads(t, w, got) })
	t.Run("the table screen", func(t *testing.T) { checkTableScreen(t, w, got) })
	t.Run("stream", func(t *testing.T) { checkStream(t, w, got) })
	// The stream test ended the combat and the session: what only exists then.
	for _, r := range readsAfterTheSession {
		t.Run("after the session "+r.name(), func(t *testing.T) { w.runRead(t, r, got) })
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, got) })
	t.Run("the TV's controls", func(t *testing.T) { checkTVCarries(t, w, got) })
}

// ownCharacter is the character that belongs to p: Ana's, Caio's or the pending member's
// (the master and the stranger have none: Ana's character stands for "someone else's").
func (w *world) ownCharacter(p *person) *charactersv1.Character {
	switch p {
	case w.caio:
		return w.toren
	case w.pending:
		return w.pendingHero
	}
	return w.pens
}

// unreachable lists the canaries that the master himself can read in no answer of the matrix,
// by kind, and why: a needle nobody can find tests nothing, so every one needs a reason. (The
// readers are still held to their positive control.)
var unreachable = map[string]string{ //nolint:gosec // G101: kinds of canary and the reasons, no credential
	"invite-token":  "an invite's token is in the answer to CreateInvite only, once: no read returns it (ListInvites lists the invite, not the secret)",
	"claim-token":   "a claim link's token is in the answer to CreateClaimLink only, once: no read returns it (the master's list says only the link's state)",
	"treasure-seed": "the seed goes into PlaceTreasure and is not stored as it is: no read gives it back",
	"note-Ana":      "a player's private notes have no master's read (MR-030); the owner's ListNotes is the control",
	"note-Caio":     "a player's private notes have no master's read (MR-030); the owner's ListNotes is the control",
	"note-id":       "a player's private notes have no master's read (MR-030); the owner's ListNotes is the control",
	"creature-type": "a creature's type is a pseudo feature of its stat block (creature-type:undead) that only the rules read, for Divine Smite: no answer returns it to anyone, the master included; the needle proves that no player's answer ever does",
}

// checkCanariesAreReachable is the check of the check: a leak test passes when the secret is
// absent, and passes just as well when the secret never was in the answers. Every canary must
// be found in the master's answers (it is there to be found), and in the answers of everyone
// who may read it (the positive control of the player's read).
func checkCanariesAreReachable(t *testing.T, w *world, got *answers) {
	t.Helper()
	for _, c := range w.secrets.list {
		if _, ok := unreachable[c.kind]; !ok && !got.holds(w.master, c) {
			t.Errorf("the master reads %s in no answer of the matrix: add a read that shows it, or list its kind in unreachable with the reason", c)
		}
		readers := []*person{w.ana, w.caio}
		if !c.public {
			readers = nil
			for _, p := range []*person{w.ana, w.caio, w.pending} {
				if c.readers[p.name] {
					readers = append(readers, p)
				}
			}
		}
		for _, p := range readers {
			if !got.holds(p, c) {
				t.Errorf("%s may read %s but finds it in no answer: the control that the player's read works is missing", p.name, c)
			}
		}
	}
}
