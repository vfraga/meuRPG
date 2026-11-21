package leaktest

import (
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"uuid"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// probeRequest builds the probe's own request for any procedure, by the names of its
// fields: every id field gets the id of a hidden thing of the fixture (the boss NPC, the
// hidden trap, the puzzle never shown...), a fresh idempotency key and the like, and every
// other field stays empty. A call that only the master may make must be refused for a
// player whatever else is in the request; the others are asked to answer without a word
// of what is hidden, so the target of the probe is always the hidden one. The request is
// rarely valid: what the probe proves is the refusal, and that no refusal says too much.
// The other requests of the probe (callerVariants) are made from this one.
func (w *world) probeRequest(p *person, method protoreflect.MethodDescriptor) *dynamicpb.Message {
	m := dynamicpb.NewMessage(method.Input())
	byName := map[protoreflect.Name]string{
		"campaign_id":         w.campaign,
		"map_id":              w.fogMap,
		"point_id":            w.pts["trap-hidden"].GetId(),
		"character_id":        w.boss.GetId(),
		"caster_character_id": w.casterNPC.GetId(),
		"cast_id":             w.hiddenCast,
		"encounter_id":        w.encounter.GetId(),
		"combatant_id":        w.combatant(w.boss).GetId(),
		"attacker_id":         w.combatant(w.pens).GetId(),
		"target_id":           w.combatant(w.boss).GetId(),
		"initiator_id":        w.combatant(w.boss).GetId(),
		"ally_id":             w.combatant(w.boss).GetId(),
		"grappled_id":         w.combatant(w.boss).GetId(),
		"puzzle_id":           w.puzzles["riddle-hidden"].GetId(),
		"image_id":            w.imgUnshown,
		"generation_id":       w.generation,
		"game_session_id":     w.session,
		"clue_id":             w.clueUnrevealed,
		"action_id":           w.hiddenAction,
		"map_point_id":        w.pts["battle"].GetId(),
		"idempotency_key":     uuid.New().String(),
		"token":               "x",
		"as_character_id":     w.boss.GetId(),
		"creature_id":         w.creatureNotOf(p),
		"note_id":             w.notes[otherName(p)], // a note that is not the caller's
		"milestone_id":        w.milestoneUnreached,
		"key":                 w.keyOf("race-archived"),
		"scene_point_id":      w.pts["scene-closed"].GetId(),
		"portrait_image_id":   w.imgPortrait,
	}
	for i := range method.Input().Fields().Len() {
		fd := method.Input().Fields().Get(i)
		if v, ok := byName[fd.Name()]; ok && fd.Kind() == protoreflect.StringKind && !fd.IsList() {
			m.Set(fd, protoreflect.ValueOfString(v))
		}
	}
	return m
}

func otherName(p *person) string {
	if p.name == "Ana" {
		return "Caio"
	}
	return "Ana"
}

// creatureNotOf is a creature that is not p's: Ana's, for anyone but Ana (who gets her
// character's id, which is no creature).
func (w *world) creatureNotOf(p *person) string {
	if p == w.ana {
		return w.boss.GetId()
	}
	return w.creature
}

// checkNotReads aims every call that is not a read at what the master hid, as each person
// who is not the master, and checks, for each of the five requests of callerVariants:
//
//   - a masterWrite or a masterWriteHidden never succeeds for a player, a pending member, a
//     stranger or someone with no session, whatever else the request says;
//   - no call fails the server (5xx) or answers with a word of what the request aimed at:
//     the refusal of a hidden thing says what it says for a thing that does not exist;
//   - the code of the refusal is the one of the class (wantCode): unauthenticated without a
//     session; not_found for a stranger and for a pending member, who gets the stranger's
//     code and message; for a player, permission_denied on a masterWrite and not_found on a
//     masterWriteHidden. The calls that anyone signed in may make (signedInCall) are checked
//     only for the signed-out caller.
//
// The player's own actions (playerAction) get no code requirement as Ana or Caio: the
// requests do not reach the owner check, so actions_test.go and the per-service matrices
// cover the owner.
//
// The probe does not reach the path that a held roll runs again: that path takes its caller
// from the hold (play/held_caller.go).
//
// The requests are built from the names of the fields and are rarely valid in the rest, so
// a call that is a player's own usually fails its validation: that is why the player's own
// actions are also scripted (actions_test.go), where the answers are real.
func checkNotReads(t *testing.T, w *world, got *answers) {
	t.Helper()
	other := w.secondCampaign(t)
	anon := w.anonymous()
	for _, procedure := range probedProcedures() {
		c := probedClass(procedure)
		// A call that names no campaign and a stream are not probed.
		if c.class == notACampaignCall || c.class == streamed {
			continue
		}
		method := methodOf(t, procedure)
		for _, v := range callerVariants {
			replies := map[*person]reply{}
			for _, p := range append(w.players(), anon) {
				r := p.call(procedure, v.build(w, p, method, other))
				replies[p] = r
				got.keep(p, r)
				checkReply(t, w, procedure, c, p, v, r)
			}
			checkCodes(t, w, anon, procedure, c, v, replies)
		}
	}
}

// probedClass is the class of a procedure that is not a read, the scripted actions
// (actions_test.go) included as the player's own calls.
func probedClass(procedure string) classified {
	for _, a := range actions {
		if a.procedure == procedure {
			return classified{playerAction, a.why}
		}
	}
	return notReads[procedure]
}

// probedProcedures lists, in order, every procedure of notReads and of the scripted actions.
func probedProcedures() []string {
	all := map[string]bool{}
	for p := range notReads {
		all[p] = true
	}
	for _, a := range actions {
		all[a.procedure] = true
	}
	return slices.Sorted(maps.Keys(all))
}

// checkReply holds what every answer must satisfy, whoever asks and whatever the code:
// no success of a master-only call, no server failure, and nothing said of the hidden.
func checkReply(t *testing.T, w *world, procedure string, c classified, p *person, v callerVariant, r reply) {
	t.Helper()
	name := strings.TrimPrefix(procedure, "/meurpg.")
	if (c.class == masterWrite || c.class == masterWriteHidden) && r.ok() {
		t.Errorf("%s (%s): %s got an answer from a master-only call (%s): %s", name, v.name, p.name, c.why, shorten(r.body))
	}
	if r.status >= 500 && (!unbuilt[procedure] || r.status != http.StatusNotImplemented) {
		t.Errorf("%s (%s): %s made the server fail (HTTP %d): %s", name, v.name, p.name, r.status, shorten(r.body))
	}
	for _, f := range w.inspect(p, r, nil) {
		t.Errorf("%s (%s): %s: %s\n\tanswer: %s", name, v.name, p.name, f, shorten(r.body))
	}
}

// A procedure in unbuilt really answers `unimplemented` (HTTP 501) to a member: once it is built it has to leave the list, so
// the probe goes back to taking every 5xx for a failure.
func TestUnbuiltProceduresAnswerUnimplemented(t *testing.T) {
	w := newWorld(t)
	for procedure := range unbuilt {
		r := w.caio.call(procedure, w.probeRequest(w.caio, methodOf(t, procedure)))
		if r.status != http.StatusNotImplemented {
			t.Errorf("%s answered HTTP %d to a member, want 501: take it out of unbuilt (%s)", strings.TrimPrefix(procedure, "/meurpg."), r.status, shorten(r.body))
		}
	}
}
