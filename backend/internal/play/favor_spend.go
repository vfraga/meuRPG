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
	"github.com/PuraFome/meuRPG/backend/internal/play/advantage"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The spend of the Inspiration (RN-43): the owner of a player character that has it takes it for advantage on one of the
// character's own d20 rolls, chosen before the d20 is rolled. useFavor is the one place that decides it; the paths that roll
// a d20 call it with the mode they worked out and roll with the mode it gives back.

// favorSource is the circumstance a spend adds to a roll's sources: advantage, the roller's own, never hidden.
var favorSource = combat.Source{Kind: combat.SourceFavor, Effect: combat.ModeAdvantage}

// favorAsk is what a path tells useFavor about its roll.
type favorAsk struct {
	// Want is the request's spend_favor.
	Want                    bool
	CampaignID, CharacterID string
	// Own says the caller is the owner of the character that rolls: the player, never the master, and a player character.
	Own bool
	// Key and Kind find the held roll the request made, when it is a retry of one.
	Key  string
	Kind heldKind
	// Sources are the circumstances of the roll before the Inspiration (after Protection and the like): the mode with it is
	// what they and the Inspiration resolve to (SRD 5.1: any advantages and disadvantages together cancel to a straight roll).
	Sources []combat.Source
	// Hide says which of the sources the roller may not read (an effect the master hides, the sun: they read "Outra fonte" in the roll,
	// RN-10); nil when they read all of them. ADDS_NOTHING is judged on the sources they read alone, so that a refusal never tells them
	// what they cannot read.
	Hide func(combat.Source) bool
	// Rolls is false when the call rolls no d20 (a save that fails by itself, an area spell), so there is nothing to spend on.
	Rolls bool
	// Replay says the roll is the replay of one a reaction window held: an Inspiration that can no longer be spent then (gone, or adding
	// nothing) does not refuse the roll, which would drop it with no reason; the roll is made without it and says so (favorUse.Lost).
	Replay bool
	// Asked and AskedRequest are the request's roll_mode and roll_mode_request_id, for the paths that ask a mode (an attack, a spell attack): a
	// replay judges them before it spends (favorAgainstBeforeSpend), because its refusal drops the held roll and keeps the transaction.
	Asked        playv1.RollMode
	AskedRequest string
	// Check only validates: a roll a reaction window holds is refused as the spend would refuse it, and nothing is taken (the
	// replay is the roll that spends). The Inspiration must be there at that time, and the replay answers for it later.
	Check bool
}

// favorUse is what useFavor decided.
type favorUse struct {
	// Mode is the mode to roll with.
	Mode combat.RollMode
	// Seen is the mode the roller reads the spend giving, from the sources they may read alone: favorAgainst judges the mode they ask on it,
	// so that a refusal never tells them of a source they cannot read (RN-10).
	Seen combat.RollMode
	// Used says the Inspiration counts on this roll: it was spent now, or by the held roll a retry or an answer belongs to.
	Used bool
	// Prompt is the id of the refund prompt the spend opened (empty when it opened none): the one a spend in this call opened, or the one of
	// the hold a retry or an answer belongs to (the hold remembers it). The roll's event stamps it (actionEvent.FavorPrompt), so its undo
	// finds that prompt and no other: not by a key, which two spends of one character can share, and not by the key the event is written under.
	Prompt string
	// Spent says it was spent in this call, so a hold the roll makes must remember it (favor_spent).
	Spent bool
	// Lost says why the spend the replay of a held roll asked for was not made (a favorLost* value): the roll is made without it, nothing
	// was taken, and the roll tells the master and the roller's player.
	Lost string
	// Refund lists the sources the roller may not read when the Inspiration added something over the ones they read but nothing over every
	// source (the hidden ones already gave the advantage): the roll resolves at once and the master is asked whether to give it back.
	Refund *refundAsk
	// Checked says the call only validated a spend a reaction window will make at its replay: nothing was taken.
	Checked bool
}

// refundAsk is what the master's refund prompt keeps of a spend that added nothing: the sources the roller could not read.
type refundAsk struct {
	Hidden []combat.Source
}

// Why a replay could not spend the Inspiration (favorUse.Lost).
const (
	favorLostGone    = "gone"
	favorLostNothing = "adds_nothing"
	// favorLostPending: the Inspiração waits in a pass the master has not answered (RN-43), so the replay cannot spend it.
	favorLostPending = "pass_pending"
)

// favorLost is the answer of a replay that cannot spend: the roll runs with the mode its real sources make, and says why.
func favorLost(a favorAsk, why string) favorUse {
	return favorUse{Mode: combat.Resolve(a.Sources), Lost: why}
}

// favorMode is the mode a roll has once the Inspiration counts.
func favorMode(sources []combat.Source) combat.RollMode {
	return combat.Resolve(append(append([]combat.Source(nil), sources...), favorSource))
}

// notesSources are the sources a check's notes make, for the helper: a bonus says nothing of advantage, and a note that is
// not an advantage is a disadvantage (as notesMode reads them). hide says which of them the roller may not read: the ones whose note is
// hidden (the master hides the source, RN-10), which spendFavor leaves out of its judgement and nowhere else.
func notesSources(notes []rollNote) (sources []combat.Source, hide func(combat.Source) bool) {
	var secret []combat.Source
	for _, n := range notes {
		if !n.givesMode() {
			continue
		}
		effect := combat.ModeDisadvantage
		if n.Adv {
			effect = combat.ModeAdvantage
		}
		src := combat.Source{Kind: n.Kind, Effect: effect}
		sources = append(sources, src)
		if n.hiddenFromOwner() {
			secret = append(secret, src)
		}
	}
	return sources, func(src combat.Source) bool { return slices.Contains(secret, src) }
}

// useFavor decides the spend of a roll, in the caller's transaction (so the row is deleted only if the roll is written).
// The refusals spend nothing. The hold is read first, whatever the request says: a retry of a held roll and the answer to one
// count the Inspiration the hold remembers (even when the retry no longer asks for it) and never spend again, and an answer
// run never spends or refuses. The hold is judged before the caller on purpose: the roll it belongs to is already made, so the
// first call's checks (the holder, the roll, the row) stand and the one thing a request that adds the spend now can be told is
// that the d20 is rolled; the row is gone after the first call, so HAS_NONE would be a lie.
func (s *Service) useFavor(ctx context.Context, env gateEnv, a favorAsk) (favorUse, error) {
	h, found, answer, err := favorHoldOf(ctx, env, a)
	if err != nil {
		return favorUse{}, err
	}
	if found {
		return heldFavor(a, h, answer)
	}
	if !a.Want {
		return favorUse{Mode: combat.Resolve(a.Sources)}, nil
	}
	a.Replay = env.c != nil && env.c.replay != nil
	if err := s.owedBeforeTheSpend(ctx, env, a); err != nil {
		return favorUse{}, err
	}
	use, err := spendFavor(ctx, env.q, a)
	if err != nil || use.Refund == nil {
		return use, err
	}
	use.Prompt, err = s.openRefundPrompt(ctx, env, a, use.Refund)
	return use, err
}

// owedBeforeTheSpend gives the roller the Inspiration its stale holds still owe, before the roll tries to spend it. A hold whose question
// went stale (the round turned, the die ran out) is forgotten only when the roller's next held roll meets it (gateOpen), which is after
// the spend: a player whose Inspiração waits in such a hold would be told HAS_NONE for it. Only when the character has no row: with
// one (a grant came in), the spend takes it and the gate's later forget gives the refund back, so none is lost to the one-row rule
// (a give-back never stacks). Only the stale holds go (heldRoll.stale), never one that still waits. A refusal that follows rolls the
// transaction back, so a spend that is refused still changes nothing.
func (s *Service) owedBeforeTheSpend(ctx context.Context, env gateEnv, a favorAsk) error {
	if !a.Own || !a.Rolls {
		return nil // the refusal is the answer (favorRefusal): it needs no Inspiration
	}
	has, err := env.q.HasCharacterFavor(ctx, playdb.HasCharacterFavorParams{CharacterID: a.CharacterID, CampaignID: a.CampaignID})
	if err != nil {
		return fmt.Errorf("read the character's inspiration: %w", err)
	}
	if has {
		return nil
	}
	return s.forgetStaleHoldsOf(ctx, env, heldRoller{CampaignID: a.CampaignID, CharacterID: a.CharacterID})
}

// favorHoldOf finds the held roll the request belongs to: the one an answer is being run for, or the one a retry made under its key.
func favorHoldOf(ctx context.Context, env gateEnv, a favorAsk) (h heldRoll, found, answer bool, err error) {
	stores := newHeldStores(env.q)
	if reply, ok := heldReplyOf(ctx); ok {
		h, err = stores.byIDOf(ctx, holdRef{Campaign: a.CampaignID, ID: reply.holdID})
		if err != nil {
			return heldRoll{}, false, false, fmt.Errorf("find the held roll: %w", err)
		}
		return h, true, true, nil
	}
	h, found, err = stores.byKeyOf(ctx, a.Kind, heldRoller{CampaignID: a.CampaignID, CharacterID: a.CharacterID}, a.Key)
	return h, found, false, err
}

// heldFavor is the Inspiration of a roll a hold keeps: it counts when the roll was made with it, and nothing is spent. A request
// that asks for it on a hold made without it comes after the d20 (an answer run never refuses: it only reads the hold).
func heldFavor(a favorAsk, h heldRoll, answer bool) (favorUse, error) {
	switch {
	case h.FavorSpent:
		return favorUse{Mode: favorMode(a.Sources), Used: true, Prompt: h.FavorPrompt}, nil
	case a.Want && !answer:
		return favorUse{}, favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_AFTER_THE_ROLL, "the d20 was already rolled")
	}
	return favorUse{Mode: combat.Resolve(a.Sources)}, nil
}

// spendFavor takes the character's Inspiration for a roll that is being made now.
func spendFavor(ctx context.Context, q *playdb.Queries, a favorAsk) (favorUse, error) {
	if err := favorRefusal(a); err != nil {
		return favorUse{}, err
	}
	if use, stop, err := favorInPass(ctx, q, a); stop {
		return use, err
	}
	// The judgement is the roller's: what a source they may not read does to the roll is no reason to refuse (the spend goes, as at a
	// table where the player does not know), and the mode the roll is made with counts every source.
	seen := visibleSources(a.Sources, a.Hide)
	if favorMode(seen) == combat.Resolve(seen) {
		if a.Replay {
			return favorLost(a, favorLostNothing), nil
		}
		return favorUse{}, favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ADDS_NOTHING, "the inspiration would not change the roll")
	}
	if a.Check {
		return checkFavor(ctx, q, a, seen)
	}
	if a.Replay {
		if err := favorAgainstBeforeSpend(ctx, q, a, seen); err != nil {
			return favorUse{}, err
		}
	}
	_, err := q.SpendCharacterFavor(ctx, playdb.SpendCharacterFavorParams{CharacterID: a.CharacterID, CampaignID: a.CampaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		if a.Replay {
			return favorLost(a, favorLostGone), nil
		}
		return favorUse{}, favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_HAS_NONE, "the character has no inspiration")
	}
	if err != nil {
		return favorUse{}, fmt.Errorf("spend the character's inspiration: %w", err)
	}
	use := favorUse{Mode: favorMode(a.Sources), Seen: favorMode(seen), Used: true, Spent: true}
	if favorMode(a.Sources) == combat.Resolve(a.Sources) { // visibly it added something, but not over every source
		use.Refund = &refundAsk{Hidden: slices.DeleteFunc(slices.Clone(a.Sources), func(src combat.Source) bool { return a.Hide == nil || !a.Hide(src) })}
	}
	return use, nil
}

// favorInPass is the refusal of a spend of an Inspiration that waits in a pass the master has not answered (RN-43): it is the first look
// at the character's own state, after the refusals that need none (favorRefusal), and before the judgement of the roll, so the box
// that spends it is closed whatever the roll is; the ask is the player's own, so the refusal tells them nothing they do not know. The
// replay of a held roll does not refuse (the roll would be dropped with no reason): it is made without the Inspiração, and says so.
func favorInPass(ctx context.Context, q *playdb.Queries, a favorAsk) (use favorUse, stop bool, err error) {
	if a.CharacterID == "" {
		return favorUse{}, false, nil
	}
	_, err = q.GetPendingFavorPassOfGiver(ctx, a.CharacterID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return favorUse{}, false, nil
	case err != nil:
		return favorUse{}, true, fmt.Errorf("read the character's pending pass: %w", err)
	case a.Replay:
		return favorLost(a, favorLostPending), true, nil
	}
	return favorUse{}, true, favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_PASS_PENDING, "the inspiration waits in an ask the master has not answered")
}

// visibleSources are the sources the roller may read: hide says which they may not.
func visibleSources(sources []combat.Source, hide func(combat.Source) bool) []combat.Source {
	if hide == nil {
		return sources
	}
	return slices.DeleteFunc(slices.Clone(sources), hide)
}

// favorRefusal is the refusal that needs no look at the roll's mode or at the row: the roller is not the owner, or the call
// rolls no d20 (a path that finds the second out before its mode is known asks it alone).
func favorRefusal(a favorAsk) error {
	switch {
	case !a.Own:
		return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_THE_HOLDER, "only the holder of the inspiration may spend it")
	case !a.Rolls:
		return favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOTHING_TO_ROLL, "this call rolls no d20")
	}
	return nil
}

// checkFavor is the validation of a spend that is made later: the character has the Inspiration now, and the roll takes it.
func checkFavor(ctx context.Context, q *playdb.Queries, a favorAsk, seen []combat.Source) (favorUse, error) {
	has, err := q.HasCharacterFavor(ctx, playdb.HasCharacterFavorParams{CharacterID: a.CharacterID, CampaignID: a.CampaignID})
	if err != nil {
		return favorUse{}, fmt.Errorf("read the character's inspiration: %w", err)
	}
	if !has {
		return favorUse{}, favorBlocked(playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_HAS_NONE, "the character has no inspiration")
	}
	return favorUse{Mode: favorMode(a.Sources), Seen: favorMode(seen), Checked: true}, nil
}

// favorAgainst refuses a request that spends the Inspiration and asks for a mode worse than the one it gives (the roll
// would waste it): the mode the caller asked is not a mode the Inspiration allows. It is judged on the mode the roller reads (fv.Seen,
// the sources they may read alone), never on the one the roll is made with, so a hidden source cannot change the answer. A request that answers a master's
// decision (requestID) is the master's to settle.
func favorAgainst(fv favorUse, want playv1.RollMode, requestID string) error {
	asked, ok := advantage.ModeFromProto(want)
	if (!fv.Spent && !fv.Checked) || !ok || requestID != "" || !fv.Seen.Better(asked) {
		return nil
	}
	return connect.NewError(connect.CodeInvalidArgument, errors.New("spend_favor gives a better mode than the roll_mode asked: leave the Inspiration for another roll"))
}

// favorAgainstBeforeSpend is favorAgainst for the replay of a held roll, judged before the Inspiration is taken. The refusal of a replay drops
// the held roll and the transaction keeps what the replay did (settleReplay): a spend made first would be lost with the roll, so the mode the
// player asked is refused while the row is still there. A character that has none now is not refused: the spend below says so, and the
// roll is made without it.
func favorAgainstBeforeSpend(ctx context.Context, q *playdb.Queries, a favorAsk, seen []combat.Source) error {
	has, err := q.HasCharacterFavor(ctx, playdb.HasCharacterFavorParams{CharacterID: a.CharacterID, CampaignID: a.CampaignID})
	if err != nil {
		return fmt.Errorf("read the character's inspiration: %w", err)
	}
	if !has {
		return nil
	}
	return favorAgainst(favorUse{Spent: true, Seen: favorMode(seen)}, a.Asked, a.AskedRequest)
}

// favorGiveBack puts the Inspiration a roll spent back, for the undo of that roll. It is idempotent: a character that has it
// again keeps the one row.
func favorGiveBack(ctx context.Context, q *playdb.Queries, campaignID, characterID string) error {
	_, err := q.InsertCharacterFavor(ctx, playdb.InsertCharacterFavorParams{CharacterID: characterID, CampaignID: campaignID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("give the character's inspiration back: %w", err)
	}
	return nil
}

// favorBack gives the Inspiration the undone roll spent back to its character. It is idempotent: a character that holds another
// one by now (a grant after the spend) keeps the one row, so the master's undo never stacks two.
func (u *undoer) favorBack(ctx context.Context, who playdb.Combatant, ev actionEvent) error {
	if !ev.Favor || who.CharacterID == "" {
		return nil
	}
	return favorBackOfRoll(ctx, u.c.q, u.c.session.CampaignID, who.CharacterID, ev.FavorPrompt)
}

// withFavor is the attack roll's mode with the Inspiration counted: the source joins the list the roller reads, and the mode
// is what the sources resolve to with it. fv.Mode is not read: it is the same resolution of the same sources (the caller gave
// useFavor the sources this result holds), so a second copy of it could only disagree. A roll without it is unchanged.
func (m modeResult) withFavor(fv favorUse, names func(string) string) modeResult {
	if !fv.Used {
		return m
	}
	m.Sources = append(append([]combat.Source(nil), m.Sources...), favorSource)
	m.Shown = append(append([]*playv1.AdvantageSource(nil), m.Shown...), advantage.SourceProto(favorSource, advantage.TextContext{Self: true, Names: names}, nil))
	m.Mode = combat.Resolve(m.Sources)
	return m
}

// withFavor is the check's mode with the Inspiration counted.
func (cm checkMode) withFavor(fv favorUse) checkMode {
	if !fv.Used {
		return cm
	}
	cm.Sources = append(append([]combat.Source(nil), cm.Sources...), favorSource)
	cm.Mode = combat.Resolve(cm.Sources)
	return cm
}

// withFavor is the combatant's check with the Inspiration counted: a note of it, and the mode their notes make.
func (p preparedCheck) withFavor(fv favorUse) preparedCheck {
	if fv.Lost != "" { // the replay could not spend it: a note says so, which the mode never reads
		p.notes = append(append([]rollNote(nil), p.notes...), rollNote{Kind: noteFavorLost, Label: favorLostNote(fv.Lost)})
		p.favor = fv
		return p
	}
	if !fv.Used {
		return p
	}
	p.notes = append(append([]rollNote(nil), p.notes...), rollNote{Kind: noteFavor, Label: labelFavor, Adv: true})
	p.mode, p.favor = notesMode(p.notes), fv
	return p
}

// favorOnCheck decides the Inspiration of a combatant's check in a combat (a contest's side, a Hide) over the circumstances its
// notes make, and counts it on the prepared check. The caller says the request, the roller and whether it owns the character.
func (s *Service) favorOnCheck(ctx context.Context, c *combatTx, p preparedCheck, a favorAsk) (preparedCheck, error) {
	a.CampaignID = c.session.CampaignID
	a.Sources, a.Hide = notesSources(p.notes)
	fv, err := s.useFavor(ctx, s.inspirationGate(c), a)
	if err != nil {
		return p, err
	}
	return p.withFavor(fv), nil
}

// checkFavorOfCheck validates the spend of a check a reaction window holds before any d20: it is refused as the roll would refuse
// it, and nothing is taken (the replay is the roll that spends, and answers for the Inspiration then).
func (s *Service) checkFavorOfCheck(ctx context.Context, c *combatTx, who playdb.Combatant, skill string, cs []playdb.Combatant, a favorAsk) error {
	if !a.Want {
		return nil
	}
	p, err := s.prepareCheck(ctx, c, who, skill, cs)
	if err != nil {
		return err
	}
	a.Check = true
	_, err = s.favorOnCheck(ctx, c, p, a)
	return err
}

// favorOnNoRoll is the answer to a spend on a call that rolls no d20 (a grapple that came from an attack, a contest answer left to
// the master): the caller is told they are not the holder, or that there is nothing to roll.
func favorOnNoRoll(want, own bool) error {
	if !want {
		return nil
	}
	return favorRefusal(favorAsk{Own: own})
}

// ownsCharacter says the viewer is the owner of the combatant's character: the player of a player's character that a character
// backs, never the master (who may roll for it, but never spends what is the player's).
func ownsCharacter(v combatViewer, c playdb.Combatant) bool {
	return v.owns(c) && c.Kind == kindPlayer && backedByCharacter(c)
}

// favorLine is the line of a roll made with the Inspiration ("Toren usou a Inspiração."), or of one that was made without the Inspiration
// its replay could no longer spend. It has the audience of the roll's dice: the master and the roller's own player, never another player.
func favorLine(ev actionEvent, v combatViewer, actor playdb.Combatant) string {
	if v.master || v.owns(actor) {
		switch {
		case ev.Favor:
			return actor.Label + " usou a Inspiração."
		case ev.FavorLost == favorLostGone:
			return actor.Label + " não tinha mais a Inspiração: a rolagem foi feita sem ela."
		case ev.FavorLost == favorLostNothing:
			return "A Inspiração de " + actor.Label + " não mudaria mais a rolagem: ela foi guardada e a rolagem foi feita sem ela."
		case ev.FavorLost == favorLostPending:
			return "A Inspiração de " + actor.Label + " está num pedido de passagem que o mestre não respondeu: a rolagem foi feita sem ela."
		}
	}
	return ""
}

// favorLostNote is the note of a check rolled without the Inspiration its replay could no longer spend.
func favorLostNote(why string) string {
	switch why {
	case favorLostGone:
		return "Inspiração não usada: não estava mais com você"
	case favorLostPending:
		return "Inspiração não usada: está num pedido de passagem sem resposta"
	}
	return "Inspiração guardada: não mudaria mais a rolagem"
}

// ownsSave says the member who rolls a saving throw is the owner of the player's character that makes it: the player, never the
// master, whose roll for an absent player is not the player's to spend an Inspiration on.
func ownsSave(m authz.Membership, reactor playdb.Combatant) bool {
	return m.Role != authz.RoleMaster && reactor.Kind == kindPlayer && backedByCharacter(reactor) && reactor.UserID != nil && *reactor.UserID == m.UserID
}

// favorOfPlayerCheck decides the Inspiration of a check that only a player rolls for their own character (a scene check, a group
// check, a trap search, a hint try): the roller is the player and the master is refused before, so Own is always true.
func (s *Service) favorOfPlayerCheck(ctx context.Context, tx pgx.Tx, q *playdb.Queries, h *hints, want bool, campaignID, characterID, key string, kind heldKind, cm checkMode) (favorUse, error) {
	return s.useFavor(ctx, s.outsideGate(tx, q, h), favorAsk{
		Want: want, CampaignID: campaignID, CharacterID: characterID, Own: true, Key: key, Kind: kind, Sources: cm.Sources, Hide: cm.hide, Rolls: true,
	})
}

// favorOnSave decides the Inspiration of a saving throw of a combatant: the mode to roll with, and whether it counts. rolls is false
// for a save that fails by itself. states are the combat's, which say what the master hides from the roller.
func (s *Service) favorOnSave(ctx context.Context, env gateEnv, m authz.Membership, reactor playdb.Combatant, kind heldKind, key string, want bool, states map[string][]playdb.CombatantState, sources []combat.Source, rolls bool) (favorUse, error) {
	return s.useFavor(ctx, env, favorAsk{
		Want: want, CampaignID: m.CampaignID, CharacterID: reactor.CharacterID, Own: ownsSave(m, reactor), Key: key, Kind: kind, Sources: sources,
		Hide: hideFor(hiddenConditionsOf(states, reactor), hasHiddenEffect(states, reactor.ID)), Rolls: rolls,
	})
}
