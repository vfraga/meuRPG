package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The d20 of an ability check a combatant makes in a combat (a contest's two sides, a Hide) may wait for a question like
// any other (SRD 5.1): a Bardic Inspiration die the combatant holds may be added to it, and a halfling's 1 may be rolled
// again. The path prepares the check (its number and the circumstances behind the mode), puts the d20 through the gate
// before it spends anything, and finishes the check with the roll the gate gave back.

// preparedCheck is a combatant's check before its d20: the number it adds, the circumstances and the mode they make.
type preparedCheck struct {
	skill string
	n     checkNumbers
	notes []rollNote
	mode  combat.CheckMode
}

// checkHold is what a path tells the gate about its check: the roll it is, the request an answer runs again, and who
// rolls it.
type checkHold struct {
	kind heldKind
	key  string
	req  proto.Message
	m    authz.Membership
	who  playdb.Combatant
	in   checkInput
}

// prepareCheck reads a combatant's number for a check and the circumstances behind its mode. It writes nothing.
func (s *Service) prepareCheck(ctx context.Context, c *combatTx, who playdb.Combatant, skill string, cs []playdb.Combatant) (preparedCheck, error) {
	n, err := s.numbersOf(ctx, c.tx, c.session.CampaignID, who, skill)
	if err != nil {
		return preparedCheck{}, err
	}
	notes, err := s.checkSources(ctx, c.tx, c.session.CampaignID, c.q, c.enc, who, skill, cs)
	if err != nil {
		return preparedCheck{}, err
	}
	return preparedCheck{skill: skill, n: n, notes: notes, mode: notesMode(notes)}, nil
}

// cutOf is what the Cutting Words of the windows that were answered take off the roll of a replayed check, 0 for any
// other.
func cutOf(c *combatTx) int {
	if c.replay == nil {
		return 0
	}
	return int(c.replay.reduction)
}

// gateCheck puts the check's d20 through the held-roll gate. A player's own check, or the master's for a player's
// character, may wait; any other rolls at once and the gate gives nothing back (finishCheck rolls it). The die Cutting
// Words took off is part of the modifier the roll is kept with, so an answer settles it with the die already off.
func (s *Service) gateCheck(ctx context.Context, c *combatTx, p preparedCheck, h checkHold) (gateResult, error) {
	if !h.in.inApp && len(h.in.faces) != p.mode.Dice() {
		return gateResult{}, connect.NewError(connect.CodeAborted, errors.New("the roll's mode changed: read the options again"))
	}
	mayAsk := h.who.Kind == kindPlayer && backedByCharacter(h.who) && playedBy(h.who, h.m)
	lucky := false
	if mayAsk {
		sheet, err := s.sheetOf(ctx, c.tx, c.session.CampaignID, h.who)
		if err != nil {
			return gateResult{}, err
		}
		lucky = sheet.Traits.Lucky
	}
	if !mayAsk {
		return gateResult{}, nil // rolled by finishCheck, as it always was
	}
	return s.heldGate(ctx, s.inspirationGate(c), rollSpec{
		Kind: h.kind, Key: h.key, Request: h.req, Window: c.replay != nil, Roller: attackRoller(h.m, h.who, c.enc.Round),
		In: rollInput{inApp: h.in.inApp, typedFaces: h.in.faces}, Modifier: p.n.Bonus - cutOf(c), Mode: rollModeOfCheck(p.mode), Reliable: p.n.Reliable,
		Lucky: lucky, MayAsk: mayAsk, Stroke: opposedStrokeRule(),
	})
}

// finishCheck is the roll of a prepared check: the gate's, when it settled the d20, else rolled here as it always was. It
// uses the Help that covered the check and takes the die of Cutting Words off a replayed one.
func (s *Service) finishCheck(ctx context.Context, c *combatTx, who playdb.Combatant, cs []playdb.Combatant, p preparedCheck, in checkInput, gate gateResult) (contestRoll, error) {
	var roll contestRoll
	if gate.Settled {
		roll = contestRoll{
			Skill: p.skill, Faces: faces32(gate.Roll.Faces), Modifier: conv.Int32(p.n.Bonus),
			Total: conv.Int32(gate.Roll.Total), Physical: gate.Roll.Physical, Mode: checkModeKey(p.mode), Notes: p.notes, Unknown: !p.n.Known, Lucky: gate.Lucky, Stroke: gate.Stroke,
			CountedAs: countedAs(gate.Roll.Face(), countedFace(gate.Roll.Face(), p.n.Reliable)),
		}
		if gate.Bonus != nil {
			roll.BonusSides, roll.BonusFace = gate.Bonus.Sides, gate.Bonus.Face
		}
		roll = markedCut(c, roll)
	} else {
		var err error
		if roll, err = s.rollCheck(in, p.mode, p.n.Bonus, p.skill, p.notes, !p.n.Known, p.n.Reliable); err != nil {
			return contestRoll{}, err
		}
		roll = cutRoll(c, roll)
	}
	if err := s.consumeCheckHelp(ctx, c, who, p.skill, cs); err != nil {
		return contestRoll{}, err
	}
	return roll, nil
}

// markedCut says in the roll's notes what Cutting Words took off a roll the gate counted with the die already off.
func markedCut(c *combatTx, roll contestRoll) contestRoll {
	if c.replay == nil {
		return roll
	}
	if cut := c.replay.reduction; cut > 0 {
		roll.Notes = append(slices.Clone(roll.Notes), rollNote{Kind: noteCutting, Label: fmt.Sprintf("%s: −%d", labelCutting, cut)})
	}
	c.replay.after = roll.Total
	return roll
}

// rollModeOfCheck is the gate's mode for a check's.
func rollModeOfCheck(m combat.CheckMode) combat.RollMode {
	switch m {
	case combat.CheckAdvantage:
		return combat.ModeAdvantage
	case combat.CheckDisadvantage:
		return combat.ModeDisadvantage
	case combat.CheckNormal:
	}
	return combat.ModeNormal
}

// keptAs is a request as a hold of a kind that two paths share keeps it: with its type, so that the answer runs the path
// that asked.
func keptAs(req proto.Message) proto.Message {
	kept, err := anypb.New(req)
	if err != nil {
		return req
	}
	return kept
}

// heldCheckOffer is the question a check's kept d20 asks, as the combat's callers read it.
func (s *Service) heldCheckOffer(ctx context.Context, c *combatTx, gate gateResult) *playv1.HeldRollOffer {
	_, _, offer := s.outsideOffers(ctx, c.tx, gate.Held)
	return offer
}
