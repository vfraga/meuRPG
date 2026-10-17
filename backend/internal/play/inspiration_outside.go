package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Bardic Inspiration out of a combat (SRD 5.1, Bard 1). The die a character holds between
// fights is a row of character_inspiration with the game time it has left (10 minutes, moved
// by the master's clock like the effects that last). It is not an effect of character_effects:
// those are made by a spell or an action and are added to a roll by themselves, while this die
// is the player's to decide on after the d20, and in a combat it is the combatant's own die
// (combatants.inspiration_*). The row moves onto the combatant when a combat takes the
// character in and back when it ends, whole, so there is never a copy.

// eventInspirationGiven is the session event of a die given out of a combat.
const eventInspirationGiven = "inspiration_given"

// outsideDieSeconds is how long the die lasts: 10 minutes of game time (100 rounds).
const outsideDieSeconds = rules.BardicInspirationRounds * rules.SecondsPerRound

type inspirationGivenEvent struct {
	Target string `json:"target"`
	Sides  int32  `json:"sides"`
}

// nameOf is the name of a character of the table, "" when it is gone.
func (s *Service) nameOf(ctx context.Context, tx pgx.Tx, campaignID string, id *string) string {
	if id == nil {
		return ""
	}
	chars, err := s.roster.CombatCharacters(ctx, tx, campaignID, []string{*id})
	if err != nil || len(chars) != 1 {
		return ""
	}
	return chars[0].Name
}

// GiveBardicInspirationOutside implements playv1connect.ResourceServiceHandler.
func (s *Service) GiveBardicInspirationOutside(
	ctx context.Context,
	req *connect.Request[playv1.GiveBardicInspirationOutsideRequest],
) (*connect.Response[playv1.GiveBardicInspirationOutsideResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	target, err := parseCombatID(req.Msg.GetTargetCharacterId(), "character")
	if err != nil {
		return nil, err
	}
	given, err := s.giveOutsideTx(ctx, m, key, idem.Hash(req.Msg), target)
	if err != nil {
		return nil, s.dbError(ctx, "give bardic inspiration", err)
	}
	if given.told {
		s.publishCastsChanged(m.CampaignID, false) // the table's hint to read the dice held again
		s.publishVitals(m.CampaignID, given.after)
		if v, err := s.vitals.GetVitals(ctx, m.CampaignID, target); err == nil {
			s.publishVitals(m.CampaignID, v)
		}
	}
	return connect.NewResponse(s.giveOutsideResponse(ctx, m.CampaignID, target, given)), nil
}

// outsideGiven is what the transaction of GiveBardicInspirationOutside answers: the bard, its vitals
// after the use (nil on a retry) and whether the die was given now, so the table is told.
type outsideGiven struct {
	bard  link.Character
	after *playv1.CharacterVitals
	told  bool
}

// giveOutsideTx is the transaction of GiveBardicInspirationOutside: the open session locked, then the
// die given or, for a key already used by the same caller, nothing.
func (s *Service) giveOutsideTx(ctx context.Context, m authz.Membership, key string, hash *string, target string) (outsideGiven, error) {
	var given outsideGiven
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		given = outsideGiven{}
		q := s.queries.WithTx(tx)
		session, err := lockOpenSession(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		done, again, err := eventByKey(ctx, q, session.ID, key, eventInspirationGiven, hash)
		if err != nil {
			return err
		}
		if again {
			return s.replayGive(ctx, tx, m, done, &given)
		}
		if given.bard, given.after, err = s.giveDie(ctx, tx, q, m, session, key, hash, target); err != nil {
			return err
		}
		given.told = true
		return nil
	})
	return given, err
}

// replayGive answers a key the caller already used: the bard, and nothing written.
func (s *Service) replayGive(ctx context.Context, tx pgx.Tx, m authz.Membership, done playdb.GetSessionEventByIdempotencyKeyRow, given *outsideGiven) error {
	if done.ActorUserID == nil || *done.ActorUserID != m.UserID {
		return errKeyReused()
	}
	bard, _, err := s.myCharacter(ctx, tx, m)
	given.bard = bard
	return err
}

// giveOutsideResponse is the answer of GiveBardicInspirationOutside: the bard's vitals and the die the
// target holds now.
func (s *Service) giveOutsideResponse(ctx context.Context, campaignID, target string, given outsideGiven) *playv1.GiveBardicInspirationOutsideResponse {
	after := given.after
	if after == nil {
		if v, err := s.vitals.GetVitals(ctx, campaignID, given.bard.ID); err == nil {
			after = v
		}
	}
	out := &playv1.GiveBardicInspirationOutsideResponse{Vitals: after}
	if row, err := s.queries.GetCharacterInspiration(ctx, playdb.GetCharacterInspirationParams{CharacterID: target, CampaignID: campaignID}); err == nil {
		out.Die = s.dieViewOf(ctx, nil, row, "")
	}
	return out
}

// outsideGift is one die given out of a combat: who gives it, to whom and what it is.
type outsideGift struct {
	m       authz.Membership
	session playdb.GameSession
	key     string
	hash    *string
	who     link.Character
	target  string
	sides   int32
}

// giveDie is the write of GiveBardicInspirationOutside, in its transaction: the checks, the use spent, the die stored
// and the event under the key. It answers the bard and its vitals after the use.
func (s *Service) giveDie(
	ctx context.Context, tx pgx.Tx, q *playdb.Queries, m authz.Membership, session playdb.GameSession, key string, hash *string, target string,
) (link.Character, *playv1.CharacterVitals, error) {
	var bard link.Character
	if err := requireNoActiveCombat(ctx, q, session.ID); err != nil {
		return bard, nil, err
	}
	who, sheet, err := s.outsideBard(ctx, tx, m)
	if err != nil {
		return bard, nil, err
	}
	bard = who
	if err := s.checkDieTarget(ctx, tx, q, m, who, target); err != nil {
		return bard, nil, err
	}
	now, err := s.vitals.GetVitalsTx(ctx, tx, m.CampaignID, who.ID)
	if err != nil {
		return bard, nil, err
	}
	if r := resourceOf(now, rules.BardicInspirationKey); r == nil || r.GetUsed() >= r.GetTotal() {
		return bard, nil, resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_USES_LEFT, "no use of Bardic Inspiration is left")
	}
	gift := outsideGift{m: m, session: session, key: key, hash: hash, who: who, target: target, sides: clamp32(sheet.BardicDie, 6, 12)}
	after, err := s.storeDie(ctx, tx, q, gift)
	return bard, after, err
}

// requireNoActiveCombat refuses the die out of a combat while a combat runs: it is given in it.
func requireNoActiveCombat(ctx context.Context, q *playdb.Queries, sessionID string) error {
	enc, err := q.GetOpenEncounter(ctx, sessionID)
	if err == nil && enc.Status == statusActive {
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_COMBAT_OPEN, "a combat is running: give the die in it")
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find the open encounter: %w", err)
	}
	return nil
}

// outsideBard is the caller's character and its sheet, when it has the Bardic Inspiration feature.
func (s *Service) outsideBard(ctx context.Context, tx pgx.Tx, m authz.Membership) (link.Character, link.Sheet, error) {
	notAvailable := resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_AVAILABLE, "the character does not have this feature")
	who, has, err := s.myCharacter(ctx, tx, m)
	if err != nil {
		return who, link.Sheet{}, err
	}
	if !has {
		return who, link.Sheet{}, notAvailable
	}
	sheet, err := s.roster.CombatSheet(ctx, tx, m.CampaignID, who.ID)
	if err != nil {
		return who, sheet, err
	}
	if sheet.BardicDie == 0 {
		return who, sheet, notAvailable
	}
	return who, sheet, nil
}

// checkDieTarget refuses a target that is not a player's character of the table, the bard itself and
// a character that holds a die already.
func (s *Service) checkDieTarget(ctx context.Context, tx pgx.Tx, q *playdb.Queries, m authz.Membership, who link.Character, target string) error {
	party, err := s.roster.CombatParty(ctx, tx, m.CampaignID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(party, func(c link.Character) bool { return c.ID == target && c.Player && !c.Reserved }) {
		return connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	refused := resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_TARGET_REFUSED, "the creature cannot get a die")
	if target == who.ID {
		return refused
	}
	_, err = q.GetCharacterInspiration(ctx, playdb.GetCharacterInspirationParams{CharacterID: target, CampaignID: m.CampaignID})
	if err == nil {
		return refused
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find the die of the target: %w", err)
	}
	return nil
}

// storeDie spends the use, stores the die on the target and writes the event under the key. It answers the
// bard's vitals after the use.
func (s *Service) storeDie(ctx context.Context, tx pgx.Tx, q *playdb.Queries, g outsideGift) (*playv1.CharacterVitals, error) {
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: g.session, now: s.now(), characterID: &g.who.ID, hash: g.hash})
	if err != nil {
		return nil, err
	}
	after, err := s.spendResource(ctx, c, g.who.ID, rules.BardicInspirationKey, 1)
	if err != nil {
		return nil, err
	}
	if err := q.InsertCharacterInspiration(ctx, playdb.InsertCharacterInspirationParams{
		CharacterID: g.target, CampaignID: g.m.CampaignID, SourceCharacterID: &g.who.ID,
		Sides: g.sides, SecondsLeft: outsideDieSeconds, CreatedAt: c.now,
	}); err != nil {
		return nil, fmt.Errorf("give the die: %w", err)
	}
	if _, err := insertSceneEvent(ctx, c, eventInspirationGiven, &g.m.UserID, &g.key, inspirationGivenEvent{Target: g.target, Sides: g.sides}); err != nil {
		return nil, err
	}
	return after, nil
}

// resourceOf is a resource of the character's vitals, nil when it has none.
func resourceOf(v *playv1.CharacterVitals, key string) *playv1.ResourceUsage {
	i := slices.IndexFunc(v.GetResources(), func(r *playv1.ResourceUsage) bool { return r.GetKey() == key })
	if i < 0 {
		return nil
	}
	return v.GetResources()[i]
}

// dieViewOf is the die a character holds, as the table reads it.
func (s *Service) dieViewOf(ctx context.Context, tx pgx.Tx, row playdb.CharacterInspiration, name string) *playv1.OutsideInspirationDie {
	return &playv1.OutsideInspirationDie{
		CharacterId: row.CharacterID, CharacterName: name, Sides: row.Sides, FromName: s.nameOf(ctx, tx, row.CampaignID, row.SourceCharacterID),
		SecondsLeft: row.SecondsLeft,
	}
}

// GetOutsideInspiration implements playv1connect.ResourceServiceHandler.
func (s *Service) GetOutsideInspiration(
	ctx context.Context,
	req *connect.Request[playv1.GetOutsideInspirationRequest],
) (*connect.Response[playv1.GetOutsideInspirationResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	party, err := s.roster.CombatParty(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the party", err)
	}
	rows, err := s.queries.ListCharacterInspirationOfCampaign(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the dice", err)
	}
	out := &playv1.GetOutsideInspirationResponse{}
	if enc, err := s.openEncounterOf(ctx, m.CampaignID); err == nil && enc != nil && enc.Status == statusActive {
		out.CombatOpen = true
	}
	if m.Role == authz.RoleMaster {
		for _, r := range rows {
			out.Held = append(out.Held, s.dieViewOf(ctx, nil, r, partyNameOf(party, r.CharacterID)))
		}
		return connect.NewResponse(out), nil
	}
	i := slices.IndexFunc(party, func(c link.Character) bool { return c.PlayerUserID != "" && c.PlayerUserID == m.UserID })
	if i < 0 {
		return connect.NewResponse(out), nil
	}
	if err := s.outsideOfPlayer(ctx, out, m, party, party[i], rows); err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

// partyNameOf is the name of a character of the party, "" when it is not in it.
func partyNameOf(party []link.Character, id string) string {
	if i := slices.IndexFunc(party, func(c link.Character) bool { return c.ID == id }); i >= 0 {
		return party[i].Name
	}
	return ""
}

// outsideOfPlayer fills what a player reads: its die, the rolls that wait for it and, for a bard, its uses and
// the characters it may give to.
func (s *Service) outsideOfPlayer(
	ctx context.Context, out *playv1.GetOutsideInspirationResponse, m authz.Membership, party []link.Character, me link.Character, rows []playdb.CharacterInspiration,
) error {
	for _, r := range rows {
		if r.CharacterID == me.ID {
			out.Mine = s.dieViewOf(ctx, nil, r, me.Name)
		}
	}
	if err := s.outsideOffersOf(ctx, out, m.CampaignID, me.ID); err != nil {
		return err
	}
	sheet, err := s.roster.CombatSheet(ctx, nil, m.CampaignID, me.ID)
	if err != nil || sheet.BardicDie == 0 {
		return nil //nolint:nilerr // a sheet that cannot be read has no feature to show
	}
	out.IsBard, out.Sides = true, clamp32(sheet.BardicDie, 0, 12)
	if v, err := s.vitals.GetVitals(ctx, m.CampaignID, me.ID); err == nil {
		if r := resourceOf(v, rules.BardicInspirationKey); r != nil {
			out.UsesMax, out.UsesLeft = r.GetTotal(), max(r.GetTotal()-r.GetUsed(), 0)
		}
	}
	out.Targets = bardicTargets(party, me, rows)
	return nil
}

// outsideOffersOf lists the rolls that wait for the die, wherever the die is: on the character, or on its
// combatant in a combat; and the ones that wait for the halfling's Lucky question first, die or not.
func (s *Service) outsideOffersOf(ctx context.Context, out *playv1.GetOutsideInspirationResponse, campaignID, characterID string) error {
	die, found, holds, err := s.heldRollsOf(ctx, campaignID, characterID)
	if err != nil {
		return s.dbError(ctx, "list the held rolls", err)
	}
	state := rollerState{Die: die, Holds: found}
	for _, h := range holds {
		switch asked := askedBy(h, state); {
		case asked.Asks == askLucky:
			out.LuckyOffers = append(out.LuckyOffers, asked.luckyOffer())
		case found:
			out.Offers = append(out.Offers, s.outsideDieOffer(ctx, nil, asked))
		}
	}
	return nil
}

// bardicTargets are the characters a bard may give a die to, with why not for the ones that hold one.
func bardicTargets(party []link.Character, me link.Character, rows []playdb.CharacterInspiration) []*playv1.BardicInspirationTarget {
	var targets []*playv1.BardicInspirationTarget
	for _, c := range party {
		if !c.Player || c.Reserved || c.ID == me.ID {
			continue
		}
		t := &playv1.BardicInspirationTarget{CharacterId: c.ID, Name: c.Name}
		if slices.ContainsFunc(rows, func(r playdb.CharacterInspiration) bool { return r.CharacterID == c.ID }) {
			t.DisabledReasonPt = rules.BardicInspirationRefusal(rules.BardicInspirationTarget{CanHear: true, HasDie: true})
		}
		targets = append(targets, t)
	}
	return targets
}

// heldRollsOf lists the rolls of a character that wait for an answer, with the die it holds
// (found): about the die, or the halfling's Lucky question, which needs no die. A stale roll (its
// combat round is over, or its die is gone) is left out.
func (s *Service) heldRollsOf(ctx context.Context, campaignID, characterID string) (heldDie, bool, []heldRoll, error) {
	die, found, err := s.dieOf(ctx, s.queries, campaignID, characterID)
	if err != nil {
		return heldDie{}, false, nil, err
	}
	all, err := newHeldStores(s.queries).of(inspirationHolds).openOf(ctx, heldRoller{CharacterID: characterID})
	if err != nil {
		return heldDie{}, false, nil, err
	}
	state := rollerState{Die: die, Holds: found}
	return die, found, slices.DeleteFunc(all, func(h heldRoll) bool { return h.stale(state) }), nil
}

// openEncounterOf is the open combat of the campaign's open game session, nil when none.
func (s *Service) openEncounterOf(ctx context.Context, campaignID string) (*playdb.Encounter, error) {
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no session, no combat
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller wraps
	}
	enc, err := s.queries.GetOpenEncounter(ctx, session.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no combat
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller wraps
	}
	return &enc, nil
}

// outsideDieRollOf reads how the Bardic Inspiration die of AnswerOutsideInspiration comes, when the
// player uses it: rolled in the app or typed from a real die.
func outsideDieRollOf(msg *playv1.AnswerOutsideInspirationRequest) (rollInput, error) {
	if !msg.GetUse() {
		return rollInput{}, nil
	}
	var p rollPick
	switch roll := msg.GetRoll().(type) {
	case *playv1.AnswerOutsideInspirationRequest_RollInApp:
		p.app = &roll.RollInApp
	case *playv1.AnswerOutsideInspirationRequest_TypedFace:
		p.face = &roll.TypedFace
	}
	return rollOfAnswer(p, askDie)
}

// AnswerOutsideInspiration implements playv1connect.ResourceServiceHandler.
func (s *Service) AnswerOutsideInspiration(
	ctx context.Context,
	req *connect.Request[playv1.AnswerOutsideInspirationRequest],
) (*connect.Response[playv1.AnswerOutsideInspirationResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RolePlayer)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	holdID, err := parseCombatID(req.Msg.GetHoldId(), "held roll")
	if err != nil {
		return nil, err
	}
	in, err := outsideDieRollOf(req.Msg)
	if err != nil {
		return nil, err
	}
	out, err := s.answerHeld(ctx, m, answerRequest{
		ref: holdRef{Table: inspirationHolds, Campaign: m.CampaignID, ID: holdID}, key: key, question: askDie, yes: req.Msg.GetUse(), in: in,
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(outsideInspirationResultOf(out)), nil
}

// outsideInspirationResultOf is the response of the path that was run again, in the arm of the oneof it
// belongs to.
func outsideInspirationResultOf(out any) *playv1.AnswerOutsideInspirationResponse {
	res := &playv1.AnswerOutsideInspirationResponse{}
	switch o := out.(type) {
	case *playv1.RollSceneCheckResponse:
		res.Result = &playv1.AnswerOutsideInspirationResponse_SceneCheck{SceneCheck: o}
	case *playv1.RollGroupCheckResponse:
		res.Result = &playv1.AnswerOutsideInspirationResponse_GroupCheck{GroupCheck: o}
	case *playv1.SearchForTrapsResponse:
		res.Result = &playv1.AnswerOutsideInspirationResponse_TrapSearch{TrapSearch: o}
	case *playv1.TryPuzzleHintResponse:
		res.Result = &playv1.AnswerOutsideInspirationResponse_HintTry{HintTry: o}
	case *playv1.CastSpellResponse:
		res.Result = &playv1.AnswerOutsideInspirationResponse_SpellCast{SpellCast: o}
	case *playv1.RollEffectSaveResponse:
		res.Result = &playv1.AnswerOutsideInspirationResponse_EffectSave{EffectSave: o}
	case *playv1.ResolveConcentrationSaveResponse:
		res.Result = &playv1.AnswerOutsideInspirationResponse_ConcentrationSave{ConcentrationSave: o}
	}
	return res
}

// dieToCombat moves the die of a character that joined a combat onto its combatant, with the
// time it has left in whole rounds (59 seconds are 9 rounds); a die with less than a round
// left is gone.
func (s *Service) dieToCombat(ctx context.Context, c *combatTx, all, added []playdb.Combatant) error {
	for _, a := range added {
		if !backedByCharacter(a) {
			continue
		}
		if err := s.carryDieIn(ctx, c, all, a); err != nil {
			return err
		}
	}
	return nil
}

// carryDieIn moves the die of one character onto its combatant, when it holds one.
func (s *Service) carryDieIn(ctx context.Context, c *combatTx, all []playdb.Combatant, a playdb.Combatant) error {
	row, err := c.q.GetCharacterInspiration(ctx, playdb.GetCharacterInspirationParams{CharacterID: a.CharacterID, CampaignID: c.session.CampaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find the die of a character: %w", err)
	}
	if rounds := roundsOfSeconds(row.SecondsLeft); rounds > 0 {
		sides, expires := row.Sides, clamp32(int(max(c.enc.Round, 1)+rounds), 1, 1<<30)
		if err := c.q.SetCombatantInspirationDie(ctx, playdb.SetCombatantInspirationDieParams{ID: a.ID, Sides: &sides, FromID: dieGiverCombatant(all, row), ExpiresRound: &expires}); err != nil {
			return fmt.Errorf("bring the die into the combat: %w", err)
		}
	}
	if _, err := c.q.DeleteCharacterInspiration(ctx, a.CharacterID); err != nil {
		return fmt.Errorf("take the die off the character: %w", err)
	}
	if err := c.q.DeleteInspirationHoldsOfCharacter(ctx, a.CharacterID); err != nil {
		return fmt.Errorf("forget the held rolls: %w", err)
	}
	return nil
}

// dieGiverCombatant is the combatant of the character that gave the die, nil when it is not in the combat.
func dieGiverCombatant(all []playdb.Combatant, row playdb.CharacterInspiration) *string {
	if row.SourceCharacterID == nil {
		return nil
	}
	if j := slices.IndexFunc(all, func(o playdb.Combatant) bool { return o.CharacterID == *row.SourceCharacterID && backedByCharacter(o) }); j >= 0 {
		return &all[j].ID
	}
	return nil
}

// dieToCharacters gives back to the characters the die their combatants hold when a combat ends
// or a character leaves it (only limits it to those combatants), with the rounds it had left,
// 6 seconds each.
func (s *Service) dieToCharacters(ctx context.Context, c *combatTx, cs, only []playdb.Combatant) error {
	round := max(c.enc.Round, 1)
	for _, a := range cs {
		if !backedByCharacter(a) || !holdsDie(a, round) ||
			(only != nil && !slices.ContainsFunc(only, func(o playdb.Combatant) bool { return o.ID == a.ID })) {
			continue
		}
		if err := s.giveDieBack(ctx, c, cs, a, *a.InspirationExpiresRound-round); err != nil {
			return err
		}
	}
	return nil
}

// giveDieBack takes the die off a combatant and, with rounds left, gives it back to its character.
func (s *Service) giveDieBack(ctx context.Context, c *combatTx, cs []playdb.Combatant, a playdb.Combatant, left int32) error {
	if left > 0 {
		var from *string
		if a.InspirationFrom != nil {
			if j := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == *a.InspirationFrom }); j >= 0 && backedByCharacter(cs[j]) {
				from = &cs[j].CharacterID
			}
		}
		if err := c.q.InsertCharacterInspiration(ctx, playdb.InsertCharacterInspirationParams{
			CharacterID: a.CharacterID, CampaignID: c.session.CampaignID, SourceCharacterID: from, Sides: *a.InspirationSides,
			SecondsLeft: clamp32(int(left*rules.SecondsPerRound), 1, outsideDieSeconds), CreatedAt: c.now,
		}); err != nil {
			return fmt.Errorf("give the die back to the character: %w", err)
		}
	}
	if err := c.q.SetCombatantInspirationDie(ctx, playdb.SetCombatantInspirationDieParams{ID: a.ID}); err != nil {
		return fmt.Errorf("take the die off the combatant: %w", err)
	}
	// The rolls held in the combat have nothing to settle once it is over (or the character is out of it).
	if err := c.q.DeleteInspirationHoldsOfCharacter(ctx, a.CharacterID); err != nil {
		return fmt.Errorf("forget the rolls held in the combat: %w", err)
	}
	return nil
}

// dieAdvance moves game time on for the dice held out of a combat: the ones with no time left
// are gone.
func (s *Service) dieAdvance(ctx context.Context, c *combatTx, seconds int32) error {
	if err := c.q.DeleteExpiredCharacterInspiration(ctx, playdb.DeleteExpiredCharacterInspirationParams{CampaignID: c.session.CampaignID, Column2: seconds}); err != nil {
		return fmt.Errorf("end the dice whose time ran out: %w", err)
	}
	if err := c.q.DeleteOrphanInspirationHolds(ctx, c.session.CampaignID); err != nil {
		return fmt.Errorf("forget the rolls that waited for a die that ran out: %w", err)
	}
	if err := c.q.AdvanceCharacterInspiration(ctx, playdb.AdvanceCharacterInspirationParams{CampaignID: c.session.CampaignID, Column2: seconds}); err != nil {
		return fmt.Errorf("move the dice's time on: %w", err)
	}
	return nil
}
