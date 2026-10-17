package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// Revivify, "Revivificar" (SRD 5.1, Revivify; RN-03): 3rd level, 1 action, touch, diamonds
// worth 300 gp that the spell consumes. It touches a creature that died in the last minute
// and the creature returns to life with 1 hit point. The SRD adds that it cannot bring back
// a creature that died of old age or restore a missing body part: the app cannot know either,
// so the master has a switch for the death where it does not work (revivify_blocked).
//
// In a combat the app counts the minute: 10 rounds (SRD 5.1, "The Order of Combat": a round is
// about 6 seconds), from the round the creature died in; in the tenth round only up to the
// place in the order where it died (the app's reading of "in the last minute"). Out of combat it
// cannot count, and the master answers (revivify_requests.go).
//
// Who sees what (RN-10): a player's list holds only the creatures the spell can reach now, and
// nothing says why another is missing. The reasons are the master's.

// revivifyKey is the spell.
const revivifyKey = "spell:revivify"

// errRevivifyTarget is the one answer a player gets for a target that cannot be revived: it does
// not exist, is not dead, is far, too long dead, hidden, not seen or marked by the master. No
// reason, so none can be learned from it (RN-10).
func errRevivifyTarget() error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("that creature cannot be revived now"))
}

// reviveSpecOf is what the campaign's content says Revivify does.
func (s *Service) reviveSpecOf(ctx context.Context, tx pgx.Tx, campaignID string) (rules.ReviveSpec, error) {
	content, err := s.roster.RulesContent(ctx, tx, campaignID)
	if err != nil {
		return rules.ReviveSpec{}, err
	}
	spec, ok := content.Revive(revivifyKey)
	if !ok {
		return rules.ReviveSpec{}, fmt.Errorf("the content has no %s", revivifyKey)
	}
	return spec, nil
}

// canBeDead says whether a combatant is something Revivify can bring back once it is dead: a
// player's character the master confirmed dead, or an NPC taken to 0. A creature of a
// character (a familiar, a summon) is not a target.
func canBeDead(t playdb.Combatant) bool {
	return t.Defeated && (t.Kind == kindPlayer || t.Kind == kindNPC)
}

// currentOrderIndex is the place in the order of the combatant on turn.
func currentOrderIndex(enc playdb.Encounter, cs []playdb.Combatant) (int32, bool) {
	if enc.CurrentCombatantID == nil {
		return 0, false
	}
	for _, c := range cs {
		if c.ID == *enc.CurrentCombatantID {
			return c.OrderIndex, true
		}
	}
	return 0, false
}

// windowOf says how many rounds ago the creature died and whether the spell still reaches it:
// within the spec's rounds, and in the last of them only until the place in the order where it
// died. A death with no round (from before the round was kept) is always in time.
func windowOf(spec rules.ReviveSpec, enc playdb.Encounter, cs []playdb.Combatant, t playdb.Combatant) (since int32, fits bool) {
	if t.DeathRound == nil {
		return 0, true
	}
	since = max(enc.Round-*t.DeathRound, 0)
	window := clamp32(spec.WindowRounds, 0, 1000)
	switch {
	case since < window:
		return since, true
	case since > window:
		return since, false
	case t.DeathOrderIndex == nil:
		return since, true
	}
	here, ok := currentOrderIndex(enc, cs)
	return since, !ok || here <= *t.DeathOrderIndex
}

// revivifyVerdict says why the caster cannot revive t now, or REVIVIFY_UNAVAILABLE_REASON_UNSPECIFIED
// when it can, and how many rounds ago t died. sees is the caster's player's viewer (what they
// see); the master casting for them is held to the spell's window and to his own switch only: he
// is never held to the range or to what is seen.
func revivifyVerdict(spec rules.ReviveSpec, enc playdb.Encounter, cs []playdb.Combatant, caster, t playdb.Combatant, sees combatViewer, master bool) (playv1.RevivifyUnavailableReason, int32) {
	since, fits := windowOf(spec, enc, cs, t)
	switch {
	case !master && t.Hidden:
		return playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_HIDDEN, since
	case !master && sees.unseen[t.ID]:
		return playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_NOT_SEEN, since
	case t.RevivifyBlocked:
		return playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_MASTER_BLOCKED, since
	case !fits:
		return playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_TOO_LONG_AGO, since
	case master || isTheatre(enc): // without a grid the master judges the range (RN-25)
		return playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_UNSPECIFIED, since
	}
	if d, ok := distanceFt(caster, t); !ok || d > meleeReachFt {
		return playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_TOO_FAR, since
	}
	return playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_UNSPECIFIED, since
}

// revivifySlot is the lowest free slot of at least the 3rd level that casts Revivify, and how
// many of that level are free; nil when there is none.
func revivifySlot(c castable) (*playv1.SpellSlot, int32) {
	var best *rulesv1.SlotChoice
	for _, sl := range c.slots {
		if sl.GetLevel() >= c.level && sl.GetFree() > 0 && (best == nil || sl.GetLevel() < best.GetLevel()) {
			best = sl
		}
	}
	if best == nil {
		return nil, 0
	}
	return &playv1.SpellSlot{Level: best.GetLevel(), Pact: best.GetPact()}, best.GetFree()
}

// revivifyScene is a combat as the caller reads it for the preview: its data, what the caller
// sees and the caster.
type revivifyScene struct {
	d      *encounterData
	v      combatViewer
	caster playdb.Combatant
}

// PreviewRevivify implements playv1connect.RevivifyServiceHandler.
func (s *Service) PreviewRevivify(
	ctx context.Context,
	req *connect.Request[playv1.PreviewRevivifyRequest],
) (*connect.Response[playv1.PreviewRevivifyResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetEncounterId() == "" {
		return s.previewOutside(ctx, m, req.Msg)
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	casterID, err := parseCombatID(req.Msg.GetCasterId(), "combatant")
	if err != nil {
		return nil, err
	}
	sc, err := s.previewScene(ctx, m, encID, casterID)
	if err != nil {
		return nil, err
	}
	return s.previewInside(ctx, m, sc)
}

// previewScene reads the combat and finds the caster, who must be the caller's to act for.
func (s *Service) previewScene(ctx context.Context, m authz.Membership, encID, casterID string) (revivifyScene, error) {
	_, d, err := s.readEncounter(ctx, m.CampaignID, encID)
	if err != nil {
		return revivifyScene{}, err
	}
	if err := notEnded(d.enc); err != nil {
		return revivifyScene{}, err
	}
	v, _, err := s.viewerWith(ctx, m, d.enc, d.cs)
	if err != nil {
		return revivifyScene{}, s.dbError(ctx, "work out what the player sees", err)
	}
	caster, err := findCombatant(d.cs, casterID, v)
	if err != nil {
		return revivifyScene{}, err
	}
	if err := v.mayAct(caster); err != nil {
		return revivifyScene{}, err
	}
	return revivifyScene{d: d, v: v, caster: caster}, nil
}

// previewInside is the preview of a combat: the caster's slot and who the spell reaches.
func (s *Service) previewInside(ctx context.Context, m authz.Membership, sc revivifyScene) (*connect.Response[playv1.PreviewRevivifyResponse], error) {
	opts, err := s.optionsOf(ctx, nil, m.CampaignID, sc.caster)
	if err != nil {
		return nil, s.dbError(ctx, "work out the turn options", err)
	}
	cast, err := castableOf(opts, revivifyKey)
	if err != nil {
		return nil, err
	}
	spec, err := s.reviveSpecOf(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the spell", err)
	}
	sees, err := s.previewSees(ctx, m, sc)
	if err != nil {
		return nil, err
	}
	res := &playv1.PreviewRevivifyResponse{CountsTime: true}
	res.Slot, res.SlotsFree = revivifySlot(cast)
	listRevivifyTargets(res, spec, sc, sees)
	return connect.NewResponse(res), nil
}

// previewSees is what the caster sees: for the master asking about a player's character, what
// that player sees, so the reasons he reads are the ones that keep the creature off their list.
func (s *Service) previewSees(ctx context.Context, m authz.Membership, sc revivifyScene) (combatViewer, error) {
	if !sc.v.master || sc.caster.UserID == nil {
		return sc.v, nil
	}
	player := authz.Membership{CampaignID: m.CampaignID, UserID: *sc.caster.UserID, Role: authz.RolePlayer}
	sees, _, err := s.viewerWith(ctx, player, sc.d.enc, sc.d.cs)
	if err != nil {
		return sc.v, s.dbError(ctx, "work out what the player sees", err)
	}
	return sees, nil
}

// listRevivifyTargets fills the preview: a player's list holds only the creatures the spell
// reaches and they see; the master also gets the reason for each one it does not reach.
func listRevivifyTargets(res *playv1.PreviewRevivifyResponse, spec rules.ReviveSpec, sc revivifyScene, sees combatViewer) {
	for _, t := range sc.d.cs {
		if !canBeDead(t) || t.ID == sc.caster.ID {
			continue
		}
		reason, since := revivifyVerdict(spec, sc.d.enc, sc.d.cs, sc.caster, t, sees, false)
		switch {
		case reason != playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_UNSPECIFIED:
			if sc.v.master {
				res.Unavailable = append(res.Unavailable, &playv1.RevivifyUnavailable{
					TargetId: t.ID, Name: t.Label, Reason: reason, DeathRound: t.DeathRound, RoundsSinceDeath: new(since),
				})
			}
		case sc.v.sees(t): // a player never lists what they do not see
			res.Targets = append(res.Targets, &playv1.RevivifyTarget{TargetId: t.ID, Name: t.Label, DeathRound: t.DeathRound, RoundsSinceDeath: new(since)})
		}
	}
}

// revivifyShape checks the request of a Revivify cast and returns the dead creature it names:
// one target, as dead_target_id, the diamonds ticked, and nothing the spell does not use.
func revivifyShape(in *playv1.CastSpellRequest) (string, error) {
	bad := func(msg string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(msg)) }
	targets := in.GetTargets()
	if len(targets) != 1 || targets[0].GetDeadTargetId() == "" || targets[0].GetCombatantId() != "" || targets[0].GetDarts() != 0 {
		return "", bad("Revivify takes one target: the dead creature, as dead_target_id")
	}
	targetID, err := parseCombatID(targets[0].GetDeadTargetId(), "combatant")
	if err != nil {
		return "", errRevivifyTarget() // the same answer for an id that is not a combatant
	}
	if !in.GetMaterialConfirmed() {
		return "", bad("material_confirmed is required: the spell consumes diamonds worth 300 gp")
	}
	if in.GetRoll() != nil || in.GetSummon() != nil || in.GetDamageTypeKey() != "" {
		return "", bad("Revivify rolls nothing and takes no summon or damage type")
	}
	return targetID, nil
}

// revivifyInput is what a Revivify cast names, read before anything is opened.
type revivifyInput struct {
	key, encounterID, casterID, targetID string
}

// revivifyInputOf reads the key, the combat, the caster and the dead creature of a cast.
func revivifyInputOf(msg *playv1.CastSpellRequest) (revivifyInput, error) {
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return revivifyInput{}, err
	}
	encID, err := parseCombatID(msg.GetEncounterId(), "encounter")
	if err != nil {
		return revivifyInput{}, err
	}
	casterID, err := parseCombatID(msg.GetCasterId(), "combatant")
	if err != nil {
		return revivifyInput{}, err
	}
	targetID, err := revivifyShape(msg)
	if err != nil {
		return revivifyInput{}, err
	}
	return revivifyInput{key: key, encounterID: encID, casterID: casterID, targetID: targetID}, nil
}

// revivifyOutcome is what the transaction of a cast leaves for what happens after the commit.
type revivifyOutcome struct {
	made   actionEvent
	vitals []*playv1.CharacterVitals
	told   *revivedCharacter
	v      combatViewer
}

// castRevivify is CastSpell for Revivify inside a combat: the cast spends the slot and the action,
// touches a creature that died within the minute, and the creature returns with 1 hit point. The
// caster confirmed the diamonds (material_confirmed); the app only notes that they were spent.
// Everything the preview offered is checked again, and a target that fails any check, for any
// reason, is the same failed_precondition for a player: nothing is spent.
func (s *Service) castRevivify(
	ctx context.Context,
	req *connect.Request[playv1.CastSpellRequest],
) (*connect.Response[playv1.CastSpellResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	in, err := revivifyInputOf(req.Msg)
	if err != nil {
		return nil, err
	}
	out := &revivifyOutcome{v: viewerOf(m)}
	res, err := s.write(ctx, combatWrite{m: m, key: in.key, hash: idem.Hash(req.Msg), kind: eventSpellCast, encounterID: in.encounterID}, func(c *combatTx) (any, error) {
		return s.revivifyWrite(ctx, c, m, req.Msg, in, out)
	})
	if err != nil {
		return nil, s.dbError(ctx, "cast Revivify", err)
	}
	return s.revivifyReply(ctx, m, res, out)
}

// revivifyWrite is the transaction of a cast: check the caster and the target, let a Counterspell
// answer, spend the slot and the action, and bring the creature back.
func (s *Service) revivifyWrite(ctx context.Context, c *combatTx, m authz.Membership, msg *playv1.CastSpellRequest, in revivifyInput, out *revivifyOutcome) (any, error) {
	out.vitals, out.told = nil, nil
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	out.v = c.viewer(m, cs)
	caster, slot, err := s.revivifyCaster(ctx, c, m, out.v, cs, in.casterID, msg.GetSlot())
	if err != nil {
		return nil, err
	}
	target, err := s.revivifyTarget(ctx, c, m, out.v, cs, caster, in.targetID)
	if err != nil {
		return nil, err
	}
	// A Counterspell can answer this cast like any other: it waits for the windows and
	// happens when they are answered (PM-04, combat_reaction_hold.go).
	if held, ok, err := s.holdCast(ctx, c, m, msg, cs, caster, revivifyKey, slot.Level, []playdb.Combatant{target}); err != nil {
		return nil, err
	} else if ok {
		out.made = held
		return out.made, nil
	}
	if out.made, out.vitals, err = s.spendForRevivify(ctx, c, caster, slot); err != nil {
		return nil, err
	}
	if c.replay != nil && c.replay.countered {
		return s.revivifyCountered(ctx, c, caster, slot, out)
	}
	return s.revivifyRaise(ctx, c, m, caster, target, out)
}

// revivifyCountered ends a cast a Counterspell stopped. The slot and the action are spent, as
// casting expended them, and nothing else happens: the creature stays dead (SRD, Counterspell).
func (s *Service) revivifyCountered(ctx context.Context, c *combatTx, caster playdb.Combatant, slot *slotRef, out *revivifyOutcome) (any, error) {
	out.made.Secret = caster.Hidden
	out.made.Reaction = &reactionEvent{Kind: string(reaction.CounterspellKind), Countered: true, Spell: revivifyKey, Level: slot.Level}
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &caster.CharacterID
	return out.made, nil
}

// revivifyRaise brings the target back and notes it in the event.
func (s *Service) revivifyRaise(ctx context.Context, c *combatTx, m authz.Membership, caster, target playdb.Combatant, out *revivifyOutcome) (any, error) {
	hit, more, back, err := s.bringBack(ctx, c, m, out.v, target)
	if err != nil {
		return nil, err
	}
	out.vitals = append(out.vitals, more...)
	out.told = back
	out.made.Hits = []castHit{hit}
	out.made.Secret = caster.Hidden || target.Hidden
	c.characterID = &caster.CharacterID
	return out.made, nil
}

// revivifyReply is what happens after the commit: the viewer, the event, the hints and the answer.
func (s *Service) revivifyReply(ctx context.Context, m authz.Membership, res combatResult, out *revivifyOutcome) (*connect.Response[playv1.CastSpellResponse], error) {
	v, err := s.viewerAfter(ctx, m, res, out.v)
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the spell", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishRevivified(ctx, m, d, ev, out)
	})
	if err != nil {
		return nil, err
	}
	if ev.Reaction != nil && ev.Reaction.Hold { // a reaction window holds the cast: nothing of it happened yet
		return connect.NewResponse(&playv1.CastSpellResponse{Encounter: enc}), nil
	}
	spell, err := s.castProto(ctx, res, ev, v)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.CastSpellResponse{Encounter: enc, Cast: spell}), nil
}

// publishRevivified tells the table what the cast changed: the combat, the log, the slot's vitals
// and the character that came back.
func (s *Service) publishRevivified(ctx context.Context, m authz.Membership, d *encounterData, ev actionEvent, out *revivifyOutcome) {
	s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
	s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	for _, vit := range out.vitals {
		s.publishVitals(m.CampaignID, vit)
	}
	if out.told != nil {
		s.PublishCharacterEvent(ctx, m.CampaignID, out.told.owner, out.told.characterID, characterRevived)
	}
}

// bringBack makes the creature live again. A player's character comes back through the
// characters module, as the master's Reviver does it (RN-03 may refuse: nothing is spent then,
// the transaction rolls back).
func (s *Service) bringBack(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, target playdb.Combatant) (castHit, []*playv1.CharacterVitals, *revivedCharacter, error) {
	hit := castHit{Target: target.ID, Fx: fxAffected, RevivedAfter: 1, DeathRound: target.DeathRound}
	var vitals []*playv1.CharacterVitals
	var told *revivedCharacter
	if target.Kind == kindPlayer {
		if _, err := s.roster.ReviveDead(ctx, c.tx, m.CampaignID, target.CharacterID, c.now); err != nil {
			if !v.master && connect.CodeOf(err) == connect.CodeFailedPrecondition {
				return hit, nil, nil, errRevivifyTarget() // a player never learns why (RN-10)
			}
			return hit, nil, nil, err
		}
		hit.Revived = target.CharacterID
		after, err := s.vitals.GetVitalsTx(ctx, c.tx, m.CampaignID, target.CharacterID)
		if err != nil {
			return hit, nil, nil, err
		}
		vitals = append(vitals, after)
		told = &revivedCharacter{characterID: target.CharacterID, owner: deref(target.UserID)}
	}
	if err := s.reviveCombatant(ctx, c, target); err != nil {
		return hit, nil, nil, err
	}
	return hit, vitals, told, nil
}

// revivifyCaster finds the caster and checks the cast is theirs to make now: the combatant is
// visible and the caller's, it is its turn, it has the spell with a slot of the 3rd level or more.
func (s *Service) revivifyCaster(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, cs []playdb.Combatant, casterID string, in *playv1.SpellSlot) (playdb.Combatant, *slotRef, error) {
	caster, err := findCombatant(cs, casterID, v)
	if err != nil {
		return caster, nil, err
	}
	if err := v.mayAct(caster); err != nil {
		return caster, nil, err
	}
	if err := s.mustActNow(ctx, c, caster); err != nil {
		return caster, nil, err
	}
	if err := s.refuseInShape(ctx, c, caster); err != nil {
		return caster, nil, err
	}
	opts, err := s.optionsOf(ctx, c.tx, m.CampaignID, caster)
	if err != nil {
		return caster, nil, err
	}
	cast, err := castableOf(opts, revivifyKey)
	if err != nil {
		return caster, nil, err
	}
	if !cast.enabled {
		if err := castError(cast.reason, v.master); err != nil {
			return caster, nil, err
		}
	}
	slot, err := slotOf(in, cast)
	if err != nil {
		return caster, nil, err
	}
	if slot == nil {
		return caster, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("slot must be a spell slot of the 3rd level or more"))
	}
	return caster, slot, nil
}

// revivifyTarget finds the dead creature the caster touches. Whatever is wrong with it, a
// player reads the same answer (RN-10); the master is held to the window and to his switch only.
func (s *Service) revivifyTarget(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, cs []playdb.Combatant, caster playdb.Combatant, targetID string) (playdb.Combatant, error) {
	i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == targetID })
	if i < 0 || !canBeDead(cs[i]) || cs[i].ID == caster.ID || !v.sees(cs[i]) {
		return playdb.Combatant{}, errRevivifyTarget()
	}
	spec, err := s.reviveSpecOf(ctx, c.tx, m.CampaignID)
	if err != nil {
		return playdb.Combatant{}, err
	}
	if reason, _ := revivifyVerdict(spec, c.enc, cs, caster, cs[i], v, v.master); reason != playv1.RevivifyUnavailableReason_REVIVIFY_UNAVAILABLE_REASON_UNSPECIFIED {
		return playdb.Combatant{}, errRevivifyTarget()
	}
	return cs[i], nil
}

// spendForRevivify spends the slot and the action at once, as every spell does (MR-014), and
// returns the event so far and the vitals the slot changed.
func (s *Service) spendForRevivify(ctx context.Context, c *combatTx, caster playdb.Combatant, slot *slotRef) (actionEvent, []*playv1.CharacterVitals, error) {
	run, err := breakRun(ctx, c, caster)
	if err != nil {
		return actionEvent{}, nil, err
	}
	made := actionEvent{
		RunBefore: run, Round: c.enc.Round, Actor: caster.ID, Key: revivifyKey, CastID: uuid.New().String(), Slot: slot,
		ActionBefore: caster.ActionUsed, BonusBefore: caster.BonusActionUsed, ReactionBefore: caster.ReactionUsed, DashedBefore: caster.Dashed,
		SpellCastBefore: caster.SpellCast, BonusSpellBefore: caster.BonusSpellCast,
		FxKind: rules.SpellKindRevive, Material: true,
	}
	var vitals []*playv1.CharacterVitals
	if caster.Kind == kindPlayer {
		slotVitals, err := s.spendSlot(ctx, c, caster.CharacterID, *slot, 1)
		if err != nil {
			return actionEvent{}, nil, err
		}
		vitals = append(vitals, slotVitals)
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: caster.ID, ActionUsed: true, BonusActionUsed: caster.BonusActionUsed, ReactionUsed: caster.ReactionUsed, Dashed: caster.Dashed,
	}); err != nil {
		return actionEvent{}, nil, fmt.Errorf("spend the economy: %w", err)
	}
	if err := c.q.SetCombatantSpellsCast(ctx, playdb.SetCombatantSpellsCastParams{ID: caster.ID, SpellCast: true, BonusSpellCast: caster.BonusSpellCast}); err != nil {
		return actionEvent{}, nil, fmt.Errorf("note the spell cast: %w", err)
	}
	return made, vitals, nil
}

// revivedCharacter is a player's character a cast brought back, for the hint after the commit.
type revivedCharacter struct {
	characterID string
	owner       string
}

// revivifyBlockTarget is who the master's switch names: a combatant of the open combat or a
// character, one of the two.
type revivifyBlockTarget struct {
	combatantID, characterID string
}

// revivifyBlockTargetOf reads the target of the switch.
func revivifyBlockTargetOf(msg *playv1.SetRevivifyBlockedRequest) (revivifyBlockTarget, error) {
	switch t := msg.GetTarget().(type) {
	case *playv1.SetRevivifyBlockedRequest_CombatantId:
		id, err := parseCombatID(t.CombatantId, "combatant")
		return revivifyBlockTarget{combatantID: id}, err
	case *playv1.SetRevivifyBlockedRequest_CharacterId:
		id, err := uuid.Parse(t.CharacterId)
		if err != nil {
			return revivifyBlockTarget{}, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
		}
		return revivifyBlockTarget{characterID: id.String()}, nil
	}
	return revivifyBlockTarget{}, connect.NewError(connect.CodeInvalidArgument, errors.New("set combatant_id or character_id"))
}

// SetRevivifyBlocked implements playv1connect.RevivifyServiceHandler: the master's switch
// "Revivificar não funciona nesta morte" on a creature that died of old age or lost a body part
// (SRD 5.1, Revivify). Only the master. Setting what it already is changes nothing.
func (s *Service) SetRevivifyBlocked(
	ctx context.Context,
	req *connect.Request[playv1.SetRevivifyBlockedRequest],
) (*connect.Response[playv1.SetRevivifyBlockedResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	target, err := revivifyBlockTargetOf(req.Msg)
	if err != nil {
		return nil, err
	}
	blocked := req.Msg.GetBlocked()

	var encounterID string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		encounterID, err = s.setBlockedIn(ctx, tx, m, target, blocked)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the Revivify switch", err)
	}
	if encounterID != "" {
		s.PublishEncounterChanged(ctx, m.CampaignID, encounterID)
	}
	return connect.NewResponse(&playv1.SetRevivifyBlockedResponse{}), nil
}

// setBlockedIn is the transaction of the switch. It returns the combat that changed, empty when
// none did.
func (s *Service) setBlockedIn(ctx context.Context, tx pgx.Tx, m authz.Membership, target revivifyBlockTarget, blocked bool) (string, error) {
	q := s.queries.WithTx(tx)
	session, err := q.GetOpenGameSessionForUpdate(ctx, m.CampaignID)
	hasSession := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lock the open session: %w", err)
	}
	if target.combatantID != "" {
		if !hasSession {
			return "", errNoOpenSession()
		}
		return s.blockCombatant(ctx, tx, q, session.ID, m.CampaignID, target.combatantID, blocked)
	}
	if err := s.roster.SetRevivifyBlocked(ctx, tx, m.CampaignID, target.characterID, blocked); err != nil {
		return "", err
	}
	if !hasSession {
		return "", nil
	}
	return s.blockDeadInCombat(ctx, q, session.ID, target.characterID, blocked)
}

// blockCombatant sets the switch on a dead combatant of the open combat, and on its character
// for a player's character (the cast outside a combat reads that one). It returns the combat.
func (s *Service) blockCombatant(ctx context.Context, tx pgx.Tx, q *playdb.Queries, sessionID, campaignID, combatantID string, blocked bool) (string, error) {
	who, err := s.combatantInOpenSession(ctx, q, sessionID, combatantID)
	if err != nil {
		return "", err
	}
	if !canBeDead(who) {
		return "", connect.NewError(connect.CodeFailedPrecondition, errors.New("the creature is not dead"))
	}
	if err := q.SetCombatantRevivifyBlocked(ctx, playdb.SetCombatantRevivifyBlockedParams{ID: who.ID, Blocked: blocked}); err != nil {
		return "", fmt.Errorf("set the switch: %w", err)
	}
	if who.Kind == kindPlayer {
		if err := s.roster.SetRevivifyBlocked(ctx, tx, campaignID, who.CharacterID, blocked); err != nil {
			return "", err
		}
	}
	enc, err := q.TouchEncounter(ctx, who.EncounterID)
	if err != nil {
		return "", fmt.Errorf("touch the encounter: %w", err)
	}
	return enc.ID, nil
}

// blockDeadInCombat carries the switch of a dead character to its combatant in the open combat,
// if it died there. It returns the combat that changed, empty when none did.
func (s *Service) blockDeadInCombat(ctx context.Context, q *playdb.Queries, sessionID, characterID string, blocked bool) (string, error) {
	enc, cs, found, err := openCombatants(ctx, q, sessionID)
	if err != nil || !found {
		return "", err
	}
	i := slices.IndexFunc(cs, defeatedPlayerRowOf(characterID))
	if i < 0 {
		return "", nil
	}
	if err := q.SetCombatantRevivifyBlocked(ctx, playdb.SetCombatantRevivifyBlockedParams{ID: cs[i].ID, Blocked: blocked}); err != nil {
		return "", fmt.Errorf("set the switch: %w", err)
	}
	touched, err := q.TouchEncounter(ctx, enc.ID)
	if err != nil {
		return "", fmt.Errorf("touch the encounter: %w", err)
	}
	return touched.ID, nil
}

// combatantInOpenSession finds a combatant of the combat of the open session that is not ended,
// or not_found.
func (s *Service) combatantInOpenSession(ctx context.Context, q *playdb.Queries, sessionID, combatantID string) (playdb.Combatant, error) {
	_, cs, found, err := openCombatants(ctx, q, sessionID)
	if err != nil {
		return playdb.Combatant{}, err
	}
	if !found {
		return playdb.Combatant{}, errCombatantNotFound()
	}
	for _, c := range cs {
		if c.ID == combatantID {
			return c, nil
		}
	}
	return playdb.Combatant{}, errCombatantNotFound()
}

// deathsOf lists the creatures that died in the combat (Encounter.deaths), in the order they
// died: the master gets all of them, with the window and his switch; a player gets only their own
// character, with the round and no more; for the others the list does not exist (RN-10).
func (s *Service) deathsOf(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer) []*playv1.CombatDeath {
	dead := deadInOrder(d.cs, v)
	if len(dead) == 0 {
		return nil
	}
	var spec rules.ReviveSpec
	if v.master {
		var err error
		if spec, err = s.reviveSpecOf(ctx, nil, m.CampaignID); err != nil {
			s.logger.WarnContext(ctx, "play: the deaths list has no Revivify window", "error", err)
			return nil
		}
	}
	out := make([]*playv1.CombatDeath, 0, len(dead))
	for _, c := range dead {
		out = append(out, combatDeathOf(c, spec, d, v))
	}
	return out
}

// deadInOrder is the dead the viewer may read about, by the round and the place in the order they
// died in.
func deadInOrder(cs []playdb.Combatant, v combatViewer) []playdb.Combatant {
	var dead []playdb.Combatant
	for _, c := range cs {
		if canBeDead(c) && c.DeathRound != nil && (v.master || v.owns(c)) {
			dead = append(dead, c)
		}
	}
	slices.SortStableFunc(dead, func(a, b playdb.Combatant) int {
		if c := int(*a.DeathRound) - int(*b.DeathRound); c != 0 {
			return c
		}
		return int(deref32(a.DeathOrderIndex)) - int(deref32(b.DeathOrderIndex))
	})
	return dead
}

// combatDeathOf is one entry of the list of deaths; the window and the switch are the master's.
func combatDeathOf(c playdb.Combatant, spec rules.ReviveSpec, d *encounterData, v combatViewer) *playv1.CombatDeath {
	death := &playv1.CombatDeath{
		CombatantId: c.ID, Name: c.Label, DeathRound: *c.DeathRound, IsPlayerCharacter: c.Kind == kindPlayer,
	}
	if c.Kind == kindPlayer {
		death.CharacterId = c.CharacterID
	}
	if v.master {
		_, fits := windowOf(spec, d.enc, d.cs, c)
		death.FitsRevivify, death.RevivifyBlocked = new(fits), new(c.RevivifyBlocked)
	}
	return death
}

func deref32(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}
