package play

import (
	"context"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The held roll of the d20 rolls that are not a weapon attack (SRD 5.1, Bard 1: the creature
// "can wait until after it rolls the d20 before deciding to use the Bardic Inspiration die, but
// must decide before the GM says whether the roll succeeds"). They use the gate (held_gate.go) and
// inspiration_holds, whose die is the character's out of a combat and its combatant's in one
// (dieOf): the roll path asks the gate, which rolls the d20 and keeps it with the request before
// anything is written, the player is asked, and the answer runs the same request again with the d20
// already rolled. The paths that hold are the ones of a player's own
// character: the master's rolls, and the saves the app rolls for a target in a caster's call
// (a spell's saving throw, a hit rider's), do not wait for a die. The same holds ask the
// halfling's Lucky question first, die or not, when a d20 shows a 1 (inspiration_lucky.go).

// spellAttackPre is the d20 of a spell attack settled before the cast spent anything, when the
// answer to a Bardic Inspiration question brings it back: spellAttack takes the mode and the roll
// from here instead of rolling.
type spellAttackPre struct {
	target string
	truth  modeResult
	choice modeChoice
	step   gateResult
	// die is the combatant's die as it was before the answer used it, for the undo of the cast.
	die heldDie
}

// holdSpellAttack is asked by every cast before anything is spent, and has something to say about a cast
// with one spell attack. A caster that is a player's character and holds a die gets the d20 rolled and kept:
// it answers the question to put to the player (the question comes back), and the cast writes nothing. So
// does a lucky one (a halfling) whose d20 shows a 1, with the Lucky question; its d20 that shows no 1 is
// settled at once. The answer's run of the cast, and the settled d20, get the roll in modes.pre and answer
// no question; so does every cast that holds no die and is not lucky.
func (s *Service) holdSpellAttack(
	ctx context.Context, c *combatTx, m authz.Membership, req *playv1.CastSpellRequest, key string,
	caster playdb.Combatant, targs []playdb.Combatant, sp link.Spell, in rollInput, modes *spellModes,
	armor func(playdb.Combatant) (int, error),
) (*outsideAsk, error) {
	if !spellMayAsk(c, m, caster, sp, targs) {
		return nil, nil
	}
	target := targs[0]
	env := s.inspirationGate(c)
	spec := rollSpec{
		Kind: heldSpellAttack, Key: key, Request: req, Roller: heldRoller{CampaignID: m.CampaignID, CharacterID: caster.CharacterID, UserID: m.UserID},
		In: in, Modifier: sp.ToHit, Lucky: modes.traits.Lucky, MayAsk: true, // a halfling: a d20 of 1 may be rolled again
	}
	// Stroke of Luck (held_stroke.go): the rogue of the 20th level is asked about a spell attack that missed.
	blind := len(effectDiceFor(modes.in.states, caster.ID, rules.RollAppliesAttack)) > 0
	spec.Stroke = attackStrokeRule(func() (int, error) { return armor(target) }, 0, blind)
	_, forced := heldReplyOf(ctx)
	before, err := s.spellDieBefore(ctx, env, spec, forced)
	if err != nil || (!forced && !before.Holds && !spec.Lucky && !s.strokeAvailable(ctx, env, spec)) {
		return nil, err
	}
	// The mode is worked out as spellAttack does, without writing while the roll only waits and as the
	// roll was made with when it is settled.
	ask := modes.ask
	ask.dry, ask.trusted = !forced, forced
	truth := modes.in.facts(c.enc, c.sight).attackMode(caster, target, modes.traits, shapeOfSpell(sp), actsNow(c.enc, caster), modes.reader, modes.names)
	choice, err := s.chooseMode(ctx, c, modes.v, caster, target, sp.Key, truth.Mode, ask)
	if err != nil {
		return nil, err
	}
	spec.Mode = choice.Mode
	step, err := s.heldGate(ctx, env, spec)
	switch {
	case err != nil:
		return nil, err
	case step.Held != nil:
		return s.outsideAskOf(ctx, c.tx, step.Held), nil
	case step.Settled:
		if !forced { // a lucky caster's d20 that showed no 1 goes on: the reason of its mode is written now
			if choice, err = s.keepReason(ctx, c, choice); err != nil {
				return nil, err
			}
		}
		modes.pre = &spellAttackPre{target: target.ID, truth: truth, choice: choice, step: step, die: before.Die}
	}
	return nil, nil // settled, or no die after all: the cast goes on
}

// spellMayAsk says the cast has a spell attack that may wait: one against a single target, by a player's
// character (the NPCs roll without a die; the master's cast for a player's character asks, E4-10), and not a cast
// that waited for a reaction window.
func spellMayAsk(c *combatTx, m authz.Membership, caster playdb.Combatant, sp link.Spell, targs []playdb.Combatant) bool {
	return sp.AttackType != "" && len(targs) == 1 && c.replay == nil &&
		caster.Kind == kindPlayer && backedByCharacter(caster) && playedBy(caster, m)
}

// spellDieBefore is the die the caster holds before the answer uses it, for the undo of the cast. An answer
// leaves the error to the gate, which reads the die again and reports it.
func (s *Service) spellDieBefore(ctx context.Context, env gateEnv, spec rollSpec, forced bool) (rollerState, error) {
	st, err := s.rollerStateOf(ctx, env, spec.Roller)
	if forced {
		return st, nil
	}
	return st, err
}

// strokeUsed is the use of Stroke of Luck the cast's spell attack made, for the undo; nil when none.
func (m *spellModes) strokeUsed() *strokeUse {
	if m == nil || m.pre == nil {
		return nil
	}
	return m.pre.step.Stroke
}

// dieUsed is the Bardic Inspiration die the cast's spell attack used, for the undo; nil when none.
func (m *spellModes) dieUsed() *resourceEvent {
	if m == nil || m.pre == nil || m.pre.step.Bonus == nil || m.pre.die.Combatant == nil {
		return nil
	}
	b, who := m.pre.step.Bonus, m.pre.die.Combatant
	return &resourceEvent{Kind: resBardicUse, Sides: b.Sides, Face: b.Face, FromID: deref(who.InspirationFrom), ExpiresRound: derefInt32(who.InspirationExpiresRound)}
}
