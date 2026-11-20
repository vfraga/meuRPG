package play

import (
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The release of a readied spell is checked against the table's dice as the cast is: a player is refused the dice the table
// does not roll, and a mode the server did not suggest, and the refusal leaves everything as it was (#938, #947).

// readiedFor readies the spell for Pensantus and fires it: the READIED window it answers. Sleep lays an area on a map, so it is
// readied in a fight with no map.
func (a *armed) readiedFor(t *testing.T, spell string, slot int32, target string, theatre bool) (*playv1.Encounter, *playv1.ReactionWindow) {
	t.Helper()
	var e *playv1.Encounter
	if theatre {
		e = a.start(t, plan{
			npcs: []*playv1.Participant{{CharacterId: a.capitao.GetId()}}, npcRolls: []int{1}, reveal: []string{"Capitão Goblin"}, theatre: true,
			players: map[string]int32{"Pensantus": 20, "Toren": 15, "Brisa": 10},
		})
		a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", spell, slot, target))
		e = a.mustEndTurn(t, a.master, e)
	} else {
		e = a.readyForRelease(t, spell, slot, target)
	}
	return e, a.mustFire(t, e, "Pensantus")
}

func TestAPlayerAtAnAppRolledTableIsRefusedTypedDiceOnTheRelease(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name    string
		party   func(*testing.T) *armed
		spell   string
		slot    int32
		target  string
		theatre bool
		set     func(*testing.T, *armed, *playv1.AnswerReactionRequest)
	}{
		{"typed", newCasters, fireBolt, 0, "Goblin", false, func(_ *testing.T, _ *armed, r *playv1.AnswerReactionRequest) { useTyped(12)(r) }},
		{"d20_faces", newCasters, fireBolt, 0, "Goblin", false, func(_ *testing.T, _ *armed, r *playv1.AnswerReactionRequest) { useFaces(nil, 12, 3)(r) }},
		{"pool_sum", newCasters, sleepSpell, 1, "Capitão Goblin", true, func(_ *testing.T, _ *armed, r *playv1.AnswerReactionRequest) { withPool(8)(r) }},
		{"ray_rolls", newRayCasters, scorchingRay, 2, "Goblin", false, func(t *testing.T, a *armed, r *playv1.AnswerReactionRequest) {
			t.Helper()
			withRayRolls(rayHit, rayHit, rayHit)(r)
			r.Targets[0].CombatantId = a.id(t, "Goblin")
		}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			a := row.party(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
			e, w := a.readiedFor(t, row.spell, row.slot, row.target, row.theatre)
			before, status := a.stateSnapshot(t), a.readiedStatus(t, "Pensantus")
			_, err := a.answerReaction(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
				useSpellAnswer(r)
				row.set(t, a, r)
			})
			wantBlockedBy(t, "typed dice at an app-rolled table: "+row.name, err, blockedWrongDice)
			if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting || status != readiedWaiting {
				t.Errorf("the window is open %v and the row %q, want open and waiting", a.readiedOpen(t, "Pensantus"), a.readiedStatus(t, "Pensantus"))
			}
			if after := a.stateSnapshot(t); after != before {
				t.Errorf("the refusal changed the state (the slot, the reaction or a window):\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

type typedRelease struct {
	name  string
	ready func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user, string)
	// faces are the d20 the attack rolls: one for the weapon attack, a pair for the spell (the Capitão stands next to Pensantus).
	faces func(other bool) []int32
}

// releases are the readied attacks a release can take typed faces for: a weapon attack and a spell attack, each with the
// window the readier answers and who answers it.
func releases() []typedRelease {
	return []typedRelease{
		{"a weapon attack", func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user, string) {
			t.Helper()
			a := newArmed(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.readyThenPass(t, nil)
			w := a.mustFire(t, e, "Toren")
			return a, e, w, a.caio, "Toren"
		}, func(other bool) []int32 { return []int32{map[bool]int32{false: 15, true: 9}[other]} }},
		{"a spell attack", func(t *testing.T) (*armed, *playv1.Encounter, *playv1.ReactionWindow, *user, string) {
			t.Helper()
			a := newCasters(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.readyForRelease(t, fireBolt, 0, "Goblin")
			w := a.mustFire(t, e, "Pensantus")
			return a, e, w, a.ana, "Pensantus"
		}, func(other bool) []int32 { return []int32{15, map[bool]int32{false: 4, true: 9}[other]} }},
	}
}

// A player who asks for advantage the server did not suggest is refused on the release as on an ordinary attack, until the
// master answers: nothing is spent and the window stays open.
func TestAPlayerAsksAdvantageOnAReleaseAndIsRefusedAsOnAnAttack(t *testing.T) {
	t.Parallel()
	for _, rel := range releases() {
		t.Run(rel.name, func(t *testing.T) {
			t.Parallel()
			a, e, w, by, who := rel.ready(t)
			before := a.stateSnapshot(t)
			_, err := a.answerReaction(t, by, e, w.GetId(), withMode(playv1.RollMode_ROLL_MODE_ADVANTAGE, "flanco", 15, 4))
			wantBlockedBy(t, "advantage asked on a release", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ROLL_MODE_NEEDS_APPROVAL)
			if !a.readiedOpen(t, who) || a.readiedStatus(t, who) != readiedWaiting {
				t.Errorf("the window is open %v and the row %q, want open and waiting", a.readiedOpen(t, who), a.readiedStatus(t, who))
			}
			if after := a.stateSnapshot(t); after != before {
				t.Errorf("the refusal changed the state:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

// The key of a release used again with other dice is another request, refused as the key of a cast is: it lands nothing, and
// the release the key made stays as it was.
func TestAReleaseKeyUsedAgainWithOtherDiceIsRefusedAsReused(t *testing.T) {
	t.Parallel()
	for _, rel := range releases() {
		t.Run(rel.name, func(t *testing.T) {
			t.Parallel()
			a, e, w, by, who := rel.ready(t)
			key := newKey()
			with := func(faces ...int32) func(*playv1.AnswerReactionRequest) {
				return func(r *playv1.AnswerReactionRequest) {
					useFaces(nil, faces...)(r)
					r.IdempotencyKey = key
				}
			}
			a.mustAnswer(t, by, e, w.GetId(), with(rel.faces(false)...))
			if a.readiedStatus(t, who) != readiedFired {
				t.Fatalf("the first release left the row %q, want fired", a.readiedStatus(t, who))
			}
			before := a.stateSnapshot(t)
			_, err := a.answerReaction(t, by, e, w.GetId(), with(rel.faces(true)...))
			wantRefusal(t, "the key of a release used with other dice", err, connect.CodeInvalidArgument, "idempotency_key was already used for another change")
			if after := a.stateSnapshot(t); after != before {
				t.Errorf("the refused repeat changed the state:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}
