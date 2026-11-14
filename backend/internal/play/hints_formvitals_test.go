package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The writers that change the vitals through the characters module's keeper inside a combat write (#735, part 2): Wild Shape
// (assume and leave, in a combat and out of one), the exhaustion the master sets, and the exhaustion a frenzied rage leaves. The
// keeper writes in the caller's transaction, so the write is Service.write's, which is Service.tx's: the session's number moves
// once, the events carry it, a repeat of the key moves nothing and tells again, and the key used for another request tells nothing.

// The kinds a combat write's handler sends itself once the write is committed (the encounter, its log, its map and the vitals it
// publishes, in finish and publishVitals) go through no collector and carry no number, as the live session document says. The
// number of the write is the one they follow. Each row lists only the kinds its write sends.
const (
	kLogChanged = "*playv1.WatchGameSessionResponse_CombatLogChanged_"
)

func rigOf(t *testing.T, a *armed, people map[string]*user, unnumbered ...string) *hintRig {
	t.Helper()
	g := newHintRig(t, a.h, a.campaignID, a.master, people)
	g.unnumbered = unnumbered
	return g
}

func partyOf(a *armed) map[string]*user {
	return map[string]*user{"Caio": a.caio, "Ana": a.ana, "Bia": a.bia}
}

// shapeCall is the call of one Wild Shape writer with a fixed key; beast is the form the call asks for ("" leaves it).
func shapeCall(t *testing.T, a *armed, u *user, charID, beast, key string) func() error {
	t.Helper()
	return func() error {
		if beast == "" {
			_, err := u.play.LeaveWildShape(t.Context(), connect.NewRequest(&playv1.LeaveWildShapeRequest{CampaignId: a.campaignID, CharacterId: charID, IdempotencyKey: key}))
			return err
		}
		_, err := u.play.AssumeWildShape(t.Context(), connect.NewRequest(&playv1.AssumeWildShapeRequest{CampaignId: a.campaignID, CharacterId: charID, BeastKey: beast, IdempotencyKey: key}))
		return err
	}
}

// setupShape is the Wild Shape table (in the fight, or not), the druid already in her form when the test is about leaving it.
func setupShape(t *testing.T, inFight, leave bool) (g *hintRig, call, other func() error) {
	t.Helper()
	s := newShapers(t)
	if inFight {
		s.fight(t)
	}
	if leave {
		s.mustAssume(t, s.bia, s.bri, wolfKey)
	}
	people := partyOf(s.armed)
	people["Dani"] = s.dani
	if inFight {
		g = rigOf(t, s.armed, people, kEncounter, kLogChanged, kVitals)
	} else {
		g = rigOf(t, s.armed, people, kVitals)
	}
	key := newKey()
	if leave {
		// Another request under the key: the same call for another druid.
		return g, shapeCall(t, s.armed, s.bia, s.bri.GetId(), "", key), shapeCall(t, s.armed, s.bia, s.pens.GetId(), "", key)
	}
	return g, shapeCall(t, s.armed, s.bia, s.bri.GetId(), wolfKey, key), shapeCall(t, s.armed, s.bia, s.bri.GetId(), "monster:badger", key)
}

func TestAssumeWildShapeInACombatHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupShape(t, true, false)
	g.keyed(call, call, other)
}

func TestAssumeWildShapeOutsideACombatHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupShape(t, false, false)
	g.keyed(call, call, other)
}

func TestLeaveWildShapeInACombatHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupShape(t, true, true)
	g.keyed(call, call, other)
}

func TestLeaveWildShapeOutsideACombatHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupShape(t, false, true)
	g.keyed(call, call, other)
}

// The events of a Wild Shape whose sends were all lost are told again by the repeat, with the stored number.
func TestAssumeWildShapeLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupShape(t, true, false)
	g.lost(call)
}

// The same, for a form assumed or left outside a combat: the repeat tells the vitals again.
func TestAssumeWildShapeOutsideACombatLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupShape(t, false, false)
	g.lost(call)
}

func TestLeaveWildShapeOutsideACombatLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupShape(t, false, true)
	g.lost(call)
}

// setupSetExhaustion is the master's SetExhaustion on Toren, in a fight, with a fixed key.
func setupSetExhaustion(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	a := newEffectsParty(t)
	e := a.closeFight(t)
	g = rigOf(t, a, partyOf(a), kEncounter, kLogChanged, kVitals)
	key := newKey()
	set := func(level int32) func() error {
		return func() error {
			_, err := a.master.lasting.SetExhaustion(t.Context(), connect.NewRequest(&playv1.SetExhaustionRequest{
				CampaignId: a.campaignID, IdempotencyKey: key, EncounterId: e.GetId(), Level: level,
				Subject: &playv1.SetExhaustionRequest_CombatantId{CombatantId: a.id(t, "Toren")},
			}))
			return err
		}
	}
	return g, set(2), set(3)
}

func TestSetExhaustionHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupSetExhaustion(t)
	g.keyed(call, call, other)
}

func TestSetExhaustionLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupSetExhaustion(t)
	g.lost(call)
}

// setupFrenzyEnd is Brisa in a frenzied rage, and the call that ends it (the exhaustion is the level the frenzy leaves).
func setupFrenzyEnd(t *testing.T, endEncounter bool) (g *hintRig, call, other func() error) {
	t.Helper()
	a, e := berserkerTable(t, 3, "subclass:berserker")
	if _, err := a.rage(t, e, true); err != nil {
		t.Fatalf("TakeAction(Rage, frenzy) error = %v", err)
	}
	a.mustAttack(t, a.bia, e, "Brisa", battleaxe, "Goblin", d20(15))
	e = a.passTo(t, a.mustEndTurnDiscarding(t, e), "Brisa")
	if endEncounter {
		g = rigOf(t, a, partyOf(a), kEncounter, kLogChanged, kMapChanged, kVitals)
	} else {
		g = rigOf(t, a, partyOf(a), kEncounter, kLogChanged, kVitals)
	}
	key := newKey()
	if endEncounter {
		end := func(encounterID string) func() error {
			return func() error {
				_, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: encounterID, IdempotencyKey: key}))
				return err
			}
		}
		return g, end(e.GetId()), end("00000000-0000-4000-8000-000000000000")
	}
	endRage := func(combatant string) func() error {
		return func() error {
			_, err := a.bia.combat.EndRage(t.Context(), connect.NewRequest(&playv1.EndRageRequest{
				CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: combatant, IdempotencyKey: key,
			}))
			return err
		}
	}
	return g, endRage(a.id(t, "Brisa")), endRage(a.id(t, "Toren"))
}

func TestFrenzyEndOfTheRageExhaustionHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupFrenzyEnd(t, false)
	g.keyed(call, call, other)
}

// keyedExcept is keyed for a write whose repeat has a named limit: it does not tell again the kinds in limit, which the first call
// told. Everything else the first call told is asserted on the repeat, the number included.
func (g *hintRig) keyedExcept(call, other func() error, limit ...string) {
	g.t.Helper()
	first := g.first(call, true)
	for name, kinds := range first {
		first[name] = slices.DeleteFunc(slices.Clone(kinds), func(k string) bool { return slices.Contains(limit, k) })
	}
	g.replay(call, first)
	g.refused(other)
}

// The repeat of an EndEncounter does not tell MapChanged again, a limit of the replay that is not the frenzy's (the live session
// document lists it): the rest, the player's vitals included, is told again.
func TestFrenzyEndOfTheCombatExhaustionHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupFrenzyEnd(t, true)
	g.keyedExcept(call, other, kMapChanged)
}
