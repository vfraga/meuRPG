package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The writers of the lasting effects on characters outside a combat and of game time (#735, part 2, 4c) go through Service.tx:
// the session's number moves once with the write, the events carry it, a repeat of the key moves nothing and tells again, and a
// refused call tells nothing and moves nothing.

// partyHints is the effects party with a stream open for each person.
func partyHints(t *testing.T) (*armed, *hintRig) {
	t.Helper()
	a := newEffectsParty(t)
	g := newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Caio": a.caio, "Ana": a.ana, "Bia": a.bia})
	return a, g
}

// setupAddEffect is the table, and the writer's call (repeatable: one key) and its key used for another request.
func setupAddEffect(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	a, g := partyHints(t)
	key := newKey()
	give := func(catalog string) func() error {
		return func() error {
			_, err := a.giveRaw(t, a.master, func(r *playv1.AddCharacterEffectRequest) {
				r.IdempotencyKey, r.CatalogKey, r.CharacterIds = key, catalog, []string{a.pens.GetId(), a.bri.GetId()}
			})
			return err
		}
	}
	return g, give(blessKey), give("condition:blinded")
}

func TestAddCharacterEffectHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupAddEffect(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestAddCharacterEffectLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupAddEffect(t)
	g.lost(call)
}

// A refusal after the write began (Mage Armor on a creature that wears armor is not held) rolls everything back and tells nobody.
func TestAddCharacterEffectRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a, g := partyHints(t)
	g.refused(func() error {
		_, err := a.giveRaw(t, a.master, func(r *playv1.AddCharacterEffectRequest) {
			r.CatalogKey, r.CharacterIds = mageArmorKey, []string{a.toren.GetId()}
		})
		return err
	})
}

// setupEndEffect is the table with an effect on Pensantus, and the writer's call (repeatable: one key) and its key used for another
// request.
func setupEndEffect(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	a, g := partyHints(t)
	made, err := a.giveRaw(t, a.master, onCharacters(a.pens.GetId()))
	if err != nil {
		t.Fatalf("AddCharacterEffect() error = %v", err)
	}
	g.drain()
	key := newKey()
	end := func(scope playv1.EffectEndScope) func() error {
		return func() error {
			_, err := a.master.lasting.EndLastingEffect(t.Context(), connect.NewRequest(&playv1.EndLastingEffectRequest{
				CampaignId: a.campaignID, EffectId: made.GetEffectIds()[0], Scope: scope, IdempotencyKey: key,
			}))
			return err
		}
	}
	return g, end(playv1.EffectEndScope_EFFECT_END_SCOPE_THIS), end(playv1.EffectEndScope_EFFECT_END_SCOPE_CONCENTRATION_GROUP)
}

func TestEndCharacterEffectHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupEndEffect(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestEndCharacterEffectLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupEndEffect(t)
	g.lost(call)
}

// An effect that is not there is refused after the session was locked: nothing moves, nobody hears.
func TestEndCharacterEffectRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a, g := partyHints(t)
	g.refused(func() error {
		_, err := a.master.lasting.EndLastingEffect(t.Context(), connect.NewRequest(&playv1.EndLastingEffectRequest{
			CampaignId: a.campaignID, EffectId: newKey(), Scope: playv1.EffectEndScope_EFFECT_END_SCOPE_THIS, IdempotencyKey: newKey(),
		}))
		return err
	})
}

// setupAdvance is the table with an effect that ends in the minute, and the writer's call (repeatable: one key) and its key used
// for another request.
func setupAdvance(t *testing.T) (g *hintRig, call, other func() error) {
	t.Helper()
	a, g := partyHints(t)
	if _, err := a.giveRaw(t, a.master, func(r *playv1.AddCharacterEffectRequest) {
		r.CharacterIds, r.DurationKind, r.Seconds = []string{a.pens.GetId()}, playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS, 30
	}); err != nil {
		t.Fatalf("AddCharacterEffect() error = %v", err)
	}
	g.drain()
	key := newKey()
	advance := func(seconds int32) func() error {
		return func() error {
			_, err := a.master.lasting.AdvanceGameTime(t.Context(), connect.NewRequest(&playv1.AdvanceGameTimeRequest{CampaignId: a.campaignID, IdempotencyKey: key, Seconds: seconds}))
			return err
		}
	}
	return g, advance(60), advance(61)
}

func TestAdvanceGameTimeHints(t *testing.T) {
	t.Parallel()
	g, call, other := setupAdvance(t)
	g.keyed(call, call, other)
}

// The first call's events are all lost: its repeat tells them again, with the stored number.
func TestAdvanceGameTimeLostHints(t *testing.T) {
	t.Parallel()
	g, call, _ := setupAdvance(t)
	g.lost(call)
}

// A combat that runs refuses the clock: nothing moves, nobody hears.
func TestAdvanceGameTimeRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a, _ := partyHints(t)
	a.closeFight(t)
	g := newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Caio": a.caio, "Ana": a.ana, "Bia": a.bia})
	g.refused(func() error {
		_, err := a.master.lasting.AdvanceGameTime(t.Context(), connect.NewRequest(&playv1.AdvanceGameTimeRequest{CampaignId: a.campaignID, IdempotencyKey: newKey(), Seconds: 60}))
		return err
	})
}
