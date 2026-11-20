package play

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The answer to a FEATURE window (feature_window.go). "Usar" runs the feature through UseFeature's own steps, as a reaction
// on the creature that triggered it (the target the request names, or that creature when it names none): the reaction, the
// use and the feature's resource are spent as a use spends them, and the window is closed with the use in one change. "Deixar
// passar" closes the window and leaves everything unspent. Either answer is one event, so the undo of the last action takes
// it back and the window waits again.

// answerFeature answers a FEATURE window, or says the window is not one (handled false).
func (s *Service) answerFeature(ctx context.Context, m authz.Membership, in answerInput, msg *playv1.AnswerReactionRequest) (*connect.Response[playv1.AnswerReactionResponse], bool, error) {
	w, err := s.queries.GetReactionWindow(ctx, playdb.GetReactionWindowParams{EncounterID: in.encID, ID: in.winID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil // the generic answer refuses it as it refuses any window it does not find
	}
	if err != nil {
		return nil, true, s.dbError(ctx, "find the reaction window", err)
	}
	if reaction.Kind(w.Kind) != reaction.Feature {
		return nil, false, nil
	}
	if err := s.mayAnswerFeature(ctx, m, in.encID, w); err != nil {
		return nil, true, err
	}
	if len(msg.GetTargets()) > 0 {
		return nil, true, badTargets(targetsNotUsed)
	}
	if err := diceNotUsed(msg); err != nil {
		return nil, true, err
	}
	out := &useFeatureOut{}
	kind := eventFeatureUsed
	do := func(c *combatTx) (any, error) { return s.useFeatureWindowTx(ctx, c, m, in, msg, out) }
	if in.choice == playv1.ReactionChoice_REACTION_CHOICE_PASS {
		kind = eventReactionDeclined
		do = func(c *combatTx) (any, error) {
			ev, err := s.passFeatureWindowTx(ctx, c, m, in)
			out.made = ev
			return ev, err
		}
	}
	res, err := s.write(ctx, combatWrite{m: m, key: in.key, hash: idem.Hash(msg), kind: kind, encounterID: in.encID}, do)
	if err != nil {
		return nil, true, s.dbError(ctx, "answer a feature window", err)
	}
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, true, s.dbError(ctx, "read the answer", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		s.publishVitalsOf(ctx, m.CampaignID, out.vitals)
	})
	if err != nil {
		return nil, true, err
	}
	return connect.NewResponse(&playv1.AnswerReactionResponse{Encounter: enc, Result: &playv1.ReactionResult{
		Kind: playv1.ReactionKind_REACTION_KIND_FEATURE, ByMaster: m.Role == authz.RoleMaster, Used: kind == eventFeatureUsed,
	}}), true, nil
}

// mayAnswerFeature refuses a player who does not play the window's reactor, and any player a hidden creature's window: it
// is the master's alone (RN-10). Both are refused as the generic answer refuses a window it does not find.
func (s *Service) mayAnswerFeature(ctx context.Context, m authz.Membership, encID string, w playdb.ReactionWindow) error {
	if m.Role != authz.RoleMaster && windowTriggerOf(w).Hidden {
		return errWindowDenied()
	}
	return s.mayAnswerReadied(ctx, m, encID, w)
}

// useFeatureWindowTx is "Usar": UseFeature's steps for the window's reactor and feature, then the window answered.
func (s *Service) useFeatureWindowTx(ctx context.Context, c *combatTx, m authz.Membership, in answerInput, msg *playv1.AnswerReactionRequest, out *useFeatureOut) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	w, reactor, err := s.windowToAnswer(ctx, c, m, in.winID)
	if err != nil {
		return nil, err
	}
	call, err := readUseFeatureCall(&playv1.UseFeatureRequest{
		CampaignId: m.CampaignID, EncounterId: in.encID, CombatantId: reactor.ID, ActionKey: windowTriggerOf(w).Key,
		TargetIds: msg.GetCreatureIds(), IdempotencyKey: in.key,
	})
	if err != nil {
		return nil, err
	}
	f := newFeatureFlow(s, c, m, call, out)
	f.win = &w
	return f.run(ctx)
}

// fromWindow checks the window is the one of this feature and aims the use at the creature that triggered it when the request
// names no target.
func (f *featureFlow) fromWindow(_ context.Context) error {
	t := windowTriggerOf(*f.win)
	if t.Key != f.call.actionKey {
		return errNotYourTurnToAnswer()
	}
	if aim := targetRules(*f.fa.Use); len(f.call.targets) == 0 && aim.Aimed() && aim.Who != rules.FeatureWhoSelf {
		f.call.targets = []string{t.Actor}
	}
	return nil
}

// answerWindow closes the window with the use: the event keeps it, for the undo to open it again.
func (f *featureFlow) answerWindow(ctx context.Context) error {
	if _, err := f.s.closeWindow(ctx, f.c, *f.win, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: f.c.master, Used: true}, reaction.Feature); err != nil {
		return err
	}
	f.made.Reaction = &reactionEvent{Kind: string(reaction.Feature), Window: f.win.ID, Used: true, ByMaster: f.c.master}
	return nil
}

// steps are the checks and the writes of the use, in order: the window's own go after the feature is found and at the end.
func (f *featureFlow) steps() []func(context.Context) error {
	steps := []func(context.Context) error{f.locate, f.findFeature, f.gate, f.mayUse, f.aim, f.pick, f.begin, f.spend, f.apply, f.remember}
	if f.win == nil {
		return steps
	}
	return append(slices.Insert(steps, 2, f.fromWindow), f.answerWindow)
}

// passFeatureWindowTx is "Deixar passar": the window closes and nothing is spent. A hidden creature's window is secret.
func (s *Service) passFeatureWindowTx(ctx context.Context, c *combatTx, m authz.Membership, in answerInput) (actionEvent, error) {
	if err := notEnded(c.enc); err != nil {
		return actionEvent{}, err
	}
	w, reactor, err := s.windowToAnswer(ctx, c, m, in.winID)
	if err != nil {
		return actionEvent{}, err
	}
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master}, ""); err != nil {
		return actionEvent{}, err
	}
	if err := touch(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &reactor.CharacterID
	t := windowTriggerOf(w)
	return actionEvent{
		Round: c.enc.Round, Secret: t.Hidden, Actor: reactor.ID, Target: t.Actor, Key: t.Key,
		Reaction: &reactionEvent{Kind: string(reaction.Feature), Window: w.ID, ByMaster: c.master},
	}, nil
}
