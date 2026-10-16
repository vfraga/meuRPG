package progression

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/progression/link"
	"github.com/PuraFome/meuRPG/backend/internal/progression/progressiondb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The values of xp_awards.mode (xp_awards_mode_valid).
const (
	modeEnemies   = "enemies"
	modeGold      = "gold"
	modeManual    = "manual"
	modeMilestone = "milestone"
)

// The kinds of session_events rows this module writes
// (session_event_kinds; package play lists them too).
const (
	eventXPAwarded       = "xp_awarded"
	eventXPAwardUndone   = "xp_award_undone"
	eventMilestoneMarked = "milestone_marked"
)

// The limits of an award, as progression.proto documents them.
const (
	maxReasonLength = 120
	maxXPAmount     = 1_000_000 // the most XP a sheet holds
	maxAwardTargets = 40
)

// The names of the unique indexes that make the rules true when two calls
// race (migration 00062).
const (
	oneAwardPerEncounterIndex = "xp_awards_one_per_encounter"
	idempotencyKeyIndex       = "xp_awards_campaign_id_idempotency_key_idx"
)

// eventPayload is what the session events of an award keep: IDs and numbers
// only, never the reason or a name (docs/privacy.md).
type eventPayload struct {
	AwardID      string   `json:"award_id"`
	Mode         string   `json:"mode"`
	TotalXP      int32    `json:"total_xp,omitempty"`
	XPEach       int32    `json:"xp_each,omitempty"`
	LostXP       int32    `json:"lost_xp,omitempty"`
	Gold         int32    `json:"gold,omitempty"`
	EncounterID  string   `json:"encounter_id,omitempty"`
	CharacterIDs []string `json:"character_ids"`
	MilestoneID  string   `json:"milestone_id,omitempty"`
	// Treasures are the ones a "Voltar à cidade" award converted: IDs and PO.
	Treasures []eventTreasure `json:"treasures,omitempty"`
}

// eventTreasure is a converted treasure in an event's payload.
type eventTreasure struct {
	PointID string `json:"point_id"`
	ValuePO int32  `json:"value_po"`
}

// grant is one award to give, checked: what AwardXP and MarkMilestone hand to
// give.
type grant struct {
	mode        string
	reason      string
	key         string
	encounterID string // enemies
	amount      int32  // manual's amount, or gold's
	characters  []string
	// milestoneID is the planned milestone a milestone award marks, empty for
	// the ad hoc ones. again says it is "Dar a mais alguém": the milestone must
	// be reached already, and the reason is its text.
	milestoneID string
	again       bool
	// treasureIDs are the found treasures a "Voltar à cidade" award converts
	// (gold only): its amount is their PO, summed inside the transaction.
	treasureIDs []string
	// hash is requestHash, set when the award is given.
	hash string
}

// requestHash is the hash of the whole request but its key, kept with the award: a retry of
// the key has the very same one, and the key reused for another change does not. The amount
// is the one the request carried (the sum of the treasures is worked out inside the
// transaction, later), and the sets do not depend on the order they were sent in.
func (g grant) requestHash() string {
	parts := []string{
		g.mode, g.reason, g.encounterID, strconv.Itoa(int(g.amount)), g.milestoneID, strconv.FormatBool(g.again),
		strings.Join(slices.Sorted(slices.Values(g.characters)), ","),
		strings.Join(slices.Sorted(slices.Values(g.treasureIDs)), ","),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// AwardXP implements progressionv1connect.ProgressionServiceHandler.
func (s *Service) AwardXP(
	ctx context.Context,
	req *connect.Request[progressionv1.AwardXPRequest],
) (*connect.Response[progressionv1.AwardXPResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	g, err := awardGrant(req.Msg)
	if err != nil {
		return nil, err
	}
	award, err := s.give(ctx, m, g)
	if err != nil {
		return nil, err
	}
	views, err := s.awardViews(ctx, m, award)
	if err != nil {
		return nil, s.dbError(ctx, "read an award", err)
	}
	each, lost := xpEach(award.TotalXp, len(views[0].GetShares()))
	return connect.NewResponse(&progressionv1.AwardXPResponse{Award: views[0], XpEach: each, LostXp: lost, Treasures: views[0].GetTreasures()}), nil
}

// awardGrant checks an AwardXP request and returns the grant it asks for: the
// mode's own fields first, then that only GOLD takes treasures, then what
// every award has (checkCommon).
func awardGrant(msg *progressionv1.AwardXPRequest) (grant, error) {
	byMode, ok := grantByMode[msg.GetMode()]
	if !ok {
		return grant{}, invalidArgument("mode", errors.New("must be ENEMIES, GOLD or MANUAL"))
	}
	g, err := byMode(msg)
	if err != nil {
		return grant{}, err
	}
	if g.mode != modeGold && len(msg.GetTreasurePointIds()) > 0 {
		return grant{}, invalidArgument("treasure_point_ids", errors.New("must be empty unless the mode is GOLD"))
	}
	if g.reason, g.key, g.characters, err = checkCommon(msg.GetReason(), msg.GetIdempotencyKey(), msg.GetCharacterIds()); err != nil {
		return grant{}, err
	}
	return g, nil
}

// grantByMode checks the fields of each mode AwardXP takes (a milestone is
// MarkMilestone's).
var grantByMode = map[progressionv1.XPAwardMode]func(*progressionv1.AwardXPRequest) (grant, error){
	progressionv1.XPAwardMode_XP_AWARD_MODE_ENEMIES: enemiesGrant,
	progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD:    goldGrant,
	progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL:  manualGrant,
}

// enemiesGrant is an ENEMIES award: a combat, and neither an amount nor gold.
// An encounter ID that is not a UUID is a combat that does not exist.
func enemiesGrant(msg *progressionv1.AwardXPRequest) (grant, error) {
	id, err := uuid.Parse(msg.GetEncounterId())
	if err != nil {
		return grant{}, connect.NewError(connect.CodeNotFound, errors.New("encounter not found"))
	}
	if msg.GetAmount() != 0 || msg.GetGold() != 0 {
		return grant{}, invalidArgument("amount and gold", errors.New("must be empty for an ENEMIES award"))
	}
	return grant{mode: modeEnemies, encounterID: id.String()}, nil
}

// goldGrant is a GOLD award: the gold typed, or the treasures of a "Voltar à
// cidade" (their PO is the gold), and neither a combat nor an amount.
func goldGrant(msg *progressionv1.AwardXPRequest) (grant, error) {
	if msg.GetEncounterId() != "" || msg.GetAmount() != 0 {
		return grant{}, invalidArgument("encounter_id and amount", errors.New("must be empty for a GOLD award"))
	}
	if len(msg.GetTreasurePointIds()) > 0 {
		if msg.GetGold() != 0 {
			return grant{}, invalidArgument("gold", errors.New("must be empty when treasure_point_ids is set"))
		}
		ids, err := checkTreasureIDs(msg.GetTreasurePointIds())
		if err != nil {
			return grant{}, err
		}
		return grant{mode: modeGold, treasureIDs: ids}, nil
	}
	gold, err := checkXPAmount("gold", msg.GetGold())
	return grant{mode: modeGold, amount: gold}, err
}

// manualGrant is a MANUAL award: an amount, and neither a combat nor gold.
func manualGrant(msg *progressionv1.AwardXPRequest) (grant, error) {
	if msg.GetEncounterId() != "" || msg.GetGold() != 0 {
		return grant{}, invalidArgument("encounter_id and gold", errors.New("must be empty for a MANUAL award"))
	}
	amount, err := checkXPAmount("amount", msg.GetAmount())
	return grant{mode: modeManual, amount: amount}, err
}

// checkXPAmount checks that the field's amount is 1 to what a sheet holds.
func checkXPAmount(field string, amount int32) (int32, error) {
	if amount < 1 || amount > maxXPAmount {
		return 0, invalidArgument(field, fmt.Errorf("must be 1 to %d", maxXPAmount))
	}
	return amount, nil
}

// MarkMilestone implements progressionv1connect.ProgressionServiceHandler.
func (s *Service) MarkMilestone(
	ctx context.Context,
	req *connect.Request[progressionv1.MarkMilestoneRequest],
) (*connect.Response[progressionv1.MarkMilestoneResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	g := grant{mode: modeMilestone}
	if g.reason, g.key, g.characters, err = checkCommon(req.Msg.GetReason(), req.Msg.GetIdempotencyKey(), req.Msg.GetCharacterIds()); err != nil {
		return nil, err
	}
	award, err := s.give(ctx, m, g)
	if err != nil {
		return nil, err
	}
	views, err := s.awardViews(ctx, m, award)
	if err != nil {
		return nil, s.dbError(ctx, "read a milestone", err)
	}
	return connect.NewResponse(&progressionv1.MarkMilestoneResponse{Award: views[0]}), nil
}

// checkCommon checks what every award has: the reason, the idempotency key and
// the characters (1 to 40 UUIDs, no repeat). It returns them cleaned, the IDs
// in canonical form. The planned milestones' calls have no reason of their
// own (it is the milestone's text) and call checkTargets.
func checkCommon(reason, key string, characterIDs []string) (string, string, []string, error) {
	reason, err := names.Clean(reason, maxReasonLength)
	if err != nil {
		return "", "", nil, invalidArgument("reason", err)
	}
	key, ids, err := checkTargets(key, characterIDs)
	return reason, key, ids, err
}

// checkTargets checks the idempotency key and the characters of an award, and
// returns them in canonical form.
func checkTargets(key string, characterIDs []string) (string, []string, error) {
	k, err := uuid.Parse(key)
	if err != nil {
		return "", nil, invalidArgument("idempotency_key", errors.New("must be a UUID"))
	}
	if len(characterIDs) < 1 || len(characterIDs) > maxAwardTargets {
		return "", nil, invalidArgument("character_ids", fmt.Errorf("must have 1 to %d characters", maxAwardTargets))
	}
	ids := make([]string, 0, len(characterIDs))
	for _, raw := range characterIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return "", nil, invalidArgument("character_ids", errors.New("must be UUIDs"))
		}
		if slices.Contains(ids, id.String()) {
			return "", nil, invalidArgument("character_ids", errors.New("must not repeat a character"))
		}
		ids = append(ids, id.String())
	}
	return k.String(), ids, nil
}

// xpEach is each character's share of total among n, and the XP that does not
// split (question 46).
func xpEach(total int32, n int) (each, lost int32) {
	each = int32(rules.SplitXP(int(total), n)) //nolint:gosec // at most total
	return each, total - each*int32(n)         //nolint:gosec // n is at most maxAwardTargets
}

// fits says whether an award of this mode suits the campaign's XP mode
// (RN-09): enemies campaigns take enemies and manual awards, gold campaigns
// gold and manual, milestones campaigns only milestones.
func fits(campaignMode campaignsv1.XpMode, mode string) bool {
	switch campaignMode {
	case campaignsv1.XpMode_XP_MODE_ENEMIES:
		return mode == modeEnemies || mode == modeManual
	case campaignsv1.XpMode_XP_MODE_GOLD:
		return mode == modeGold || mode == modeManual
	case campaignsv1.XpMode_XP_MODE_MILESTONES:
		return mode == modeMilestone
	}
	return false
}

// give gives an award: it checks it against the campaign's XP mode, the party
// and the combat, writes the award, each share and the XP on the sheets in one
// transaction (with the session's event, when a session is open), and, after
// the commit, tells the streams. A retry of the same key changes and sends
// nothing and returns the award as it was.
func (s *Service) give(ctx context.Context, m authz.Membership, g grant) (progressiondb.XpAward, error) {
	g.hash = g.requestHash()
	// A retry comes first: whatever changed since (the mode, a character that
	// died) must not turn the answer to a request already done into an error.
	if done, found, err := s.retriedAward(ctx, m, g); found || err != nil {
		return done, err
	}
	campaignMode, err := s.campaigns.CampaignXPMode(ctx, nil, m.CampaignID)
	if err != nil {
		return progressiondb.XpAward{}, s.dbError(ctx, "read the campaign's XP mode", err)
	}
	if !fits(campaignMode, g.mode) {
		return progressiondb.XpAward{}, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED, "", campaignMode)
	}
	var given givenAward
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		given, err = s.giveInTx(ctx, tx, m, g) // a retry starts over
		return err
	})
	if err != nil {
		return progressiondb.XpAward{}, s.dbError(ctx, "give XP", err)
	}
	s.announceAward(ctx, m, g, given)
	return given.award, nil
}

// givenAward is what an award's transaction did: the award, whether it was a
// retry that found it (repeated), whether the open session logged it, and
// the maps whose treasures it converted.
type givenAward struct {
	award       progressiondb.XpAward
	repeated    bool
	logged      bool
	changedMaps []string
}

// awardPlan is an award checked inside its transaction, ready to write: the
// grant (with the treasures' gold), the party as it is now, the reason (a
// planned milestone's text), the locked treasures, the XP to split and each
// character's share of it (0 for a milestone).
type awardPlan struct {
	g         grant
	party     []link.Member
	reason    string
	treasures []link.Treasure
	total     int32
	each      int32
}

// retriedAward finds, outside a transaction, the award a retried key already
// made, and checks it is for the same request (found says there was one).
func (s *Service) retriedAward(ctx context.Context, m authz.Membership, g grant) (progressiondb.XpAward, bool, error) {
	done, err := s.queries.GetXPAwardByKey(ctx, progressiondb.GetXPAwardByKeyParams{CampaignID: m.CampaignID, IdempotencyKey: g.key})
	if errors.Is(err, pgx.ErrNoRows) {
		return progressiondb.XpAward{}, false, nil
	}
	if err != nil {
		return progressiondb.XpAward{}, false, s.dbError(ctx, "find an award by its key", err)
	}
	award, err := s.sameRequest(ctx, s.queries, done, g)
	return award, true, err
}

// giveInTx is the award's transaction: the key again, the checks against what
// the campaign is now, then the writes and the session's event.
func (s *Service) giveInTx(ctx context.Context, tx pgx.Tx, m authz.Membership, g grant) (givenAward, error) {
	q := s.queries.WithTx(tx)
	done, err := q.GetXPAwardByKey(ctx, progressiondb.GetXPAwardByKeyParams{CampaignID: m.CampaignID, IdempotencyKey: g.key})
	if err == nil {
		// A request that raced with its own retry.
		award, err := s.sameRequest(ctx, q, done, g)
		return givenAward{award: award, repeated: true}, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return givenAward{}, fmt.Errorf("find an award by its key: %w", err)
	}
	p, err := s.planAward(ctx, tx, q, m, g)
	if err != nil {
		return givenAward{}, err
	}
	now := s.now()
	award, err := insertAward(ctx, q, m, p, now)
	if err != nil {
		return givenAward{}, err
	}
	if err := s.insertShares(ctx, tx, q, m, p, award.ID, now); err != nil {
		return givenAward{}, err
	}
	changedMaps, err := s.convertTreasures(ctx, tx, q, p, award.ID)
	if err != nil {
		return givenAward{}, err
	}
	kind, payload := awardEvent(p, award.ID)
	logged, err := s.appendEvent(ctx, tx, m, kind, payload, now)
	return givenAward{award: award, logged: logged, changedMaps: changedMaps}, err
}

// planAward checks the award inside its transaction, in this order: the XP
// mode, the party, the planned milestone, the treasures, the XP to give.
func (s *Service) planAward(ctx context.Context, tx pgx.Tx, q *progressiondb.Queries, m authz.Membership, g grant) (awardPlan, error) {
	// The mode is read again here, in the transaction (RN-09): SetCampaignXpMode
	// reads the awards and writes the mode in a transaction of its own, so under
	// serializable isolation the two cannot both pass, and one retries.
	txMode, err := s.campaigns.CampaignXPMode(ctx, tx, m.CampaignID)
	if err != nil {
		return awardPlan{}, fmt.Errorf("read the campaign's XP mode: %w", err)
	}
	if !fits(txMode, g.mode) {
		return awardPlan{}, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED, "", txMode)
	}
	p := awardPlan{g: g, reason: g.reason}
	if p.party, err = s.eligibleParty(ctx, tx, m.CampaignID, g.characters); err != nil {
		return awardPlan{}, err
	}
	if g.milestoneID != "" {
		if p.reason, err = s.checkMilestone(ctx, q, m.CampaignID, g); err != nil {
			return awardPlan{}, err
		}
	}
	if err := s.treasureGold(ctx, tx, m.CampaignID, &p); err != nil {
		return awardPlan{}, err
	}
	if p.total, err = s.total(ctx, tx, q, m.CampaignID, p.g); err != nil {
		return awardPlan{}, err
	}
	if p.g.mode != modeMilestone {
		if p.each, _ = xpEach(p.total, len(g.characters)); p.each < 1 {
			return awardPlan{}, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_NOTHING_TO_GIVE, "", 0)
		}
	}
	return p, nil
}

// eligibleParty reads the party in the transaction, so that a character that
// died, or went up a level, since the request began is judged as it is now,
// and refuses the first character that is not in it.
func (s *Service) eligibleParty(ctx context.Context, tx pgx.Tx, campaignID string, characters []string) ([]link.Member, error) {
	party, err := s.party.Party(ctx, tx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("read the party: %w", err)
	}
	for _, id := range characters {
		if !slices.ContainsFunc(party, func(p link.Member) bool { return p.ID == id }) {
			return nil, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE, id, 0)
		}
	}
	return party, nil
}

// treasureGold locks and checks the treasures of a "Voltar à cidade", and
// makes their PO the plan's gold. The lock holds until the commit, so two
// awards for one treasure take turns and the second finds it converted.
func (s *Service) treasureGold(ctx context.Context, tx pgx.Tx, campaignID string, p *awardPlan) error {
	if len(p.g.treasureIDs) == 0 {
		return nil
	}
	treasures, err := s.lockTreasures(ctx, tx, campaignID, p.g.treasureIDs)
	if err != nil {
		return err
	}
	p.treasures, p.g.amount = treasures, 0
	for _, tr := range treasures {
		p.g.amount += tr.ValuePO
	}
	if p.g.amount > maxXPAmount {
		return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_TREASURES_OVER_LIMIT, "", 0)
	}
	return nil
}

// insertAward writes the award's row. The unique indexes refuse what two
// calls that raced both checked: a second award for the combat, or the key
// of a request still being handled.
func insertAward(ctx context.Context, q *progressiondb.Queries, m authz.Membership, p awardPlan, now time.Time) (progressiondb.XpAward, error) {
	g := p.g
	params := progressiondb.InsertXPAwardParams{
		CampaignID: m.CampaignID, GivenBy: m.UserID, CreatedAt: now, Mode: g.mode, Reason: p.reason,
		TotalXp: p.total, IdempotencyKey: g.key, IdempotencyHash: &g.hash,
	}
	if g.milestoneID != "" {
		params.MilestoneID, params.MilestoneAgain = &g.milestoneID, g.again
	}
	switch g.mode {
	case modeEnemies:
		params.EncounterID = &g.encounterID
	case modeGold:
		params.Gold = &g.amount
	}
	award, err := q.InsertXPAward(ctx, params)
	switch {
	case err == nil:
		return award, nil
	case isUniqueViolation(err, oneAwardPerEncounterIndex):
		return award, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_ALREADY_AWARDED, "", 0)
	case isUniqueViolation(err, idempotencyKeyIndex):
		return award, connect.NewError(connect.CodeAborted, errors.New("the same request is being handled, please try again"))
	}
	return award, fmt.Errorf("insert an award: %w", err)
}

// insertShares writes each character's share, in the order the request named
// them, with its XP on the sheet.
func (s *Service) insertShares(ctx context.Context, tx pgx.Tx, q *progressiondb.Queries, m authz.Membership, p awardPlan, awardID string, now time.Time) error {
	for _, id := range p.g.characters {
		share, err := s.share(ctx, tx, m, p, awardID, id, now)
		if err != nil {
			return err
		}
		if err := q.InsertXPShare(ctx, share); err != nil {
			return fmt.Errorf("insert a share: %w", err)
		}
	}
	return nil
}

// share is one character's share of the award. A milestone's keeps the
// sheet's level, as the tag lasts until the level goes past it; any other's
// adds the XP to the sheet and keeps what the sheet gained (it holds at most
// 1,000,000 XP), so an undo takes back exactly that.
func (s *Service) share(ctx context.Context, tx pgx.Tx, m authz.Membership, p awardPlan, awardID, id string, now time.Time) (progressiondb.InsertXPShareParams, error) {
	share := progressiondb.InsertXPShareParams{AwardID: awardID, CharacterID: id, Xp: p.each}
	if p.g.mode == modeMilestone {
		level := p.party[slices.IndexFunc(p.party, func(member link.Member) bool { return member.ID == id })].Level
		share.LevelAtMark = &level
		return share, nil
	}
	before, after, err := s.party.AddExperience(ctx, tx, m.CampaignID, id, p.each, now)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return share, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE, id, 0)
		}
		return share, fmt.Errorf("add the XP to a sheet: %w", err)
	}
	share.Xp = after - before
	return share, nil
}

// convertTreasures links the award to the treasures it converted, and returns
// the maps they are on, each once.
func (s *Service) convertTreasures(ctx context.Context, tx pgx.Tx, q *progressiondb.Queries, p awardPlan, awardID string) ([]string, error) {
	for _, tr := range p.treasures {
		if err := q.InsertXPAwardTreasure(ctx, progressiondb.InsertXPAwardTreasureParams{AwardID: awardID, PointID: tr.PointID, ValuePo: tr.ValuePO}); err != nil {
			return nil, fmt.Errorf("insert a converted treasure: %w", err)
		}
	}
	if len(p.treasures) == 0 {
		return nil, nil
	}
	if err := s.treasures.MarkConverted(ctx, tx, awardID, p.g.treasureIDs); err != nil {
		return nil, err
	}
	var maps []string
	for _, tr := range p.treasures {
		if !slices.Contains(maps, tr.MapID) {
			maps = append(maps, tr.MapID)
		}
	}
	return maps, nil
}

// awardEvent is the session event of an award: milestone_marked, with no XP
// lost, for a milestone, and xp_awarded for the others.
func awardEvent(p awardPlan, awardID string) (string, eventPayload) {
	g := p.g
	payload := eventPayload{AwardID: awardID, Mode: g.mode, TotalXP: p.total, XPEach: p.each, CharacterIDs: g.characters, EncounterID: g.encounterID, MilestoneID: g.milestoneID}
	if g.mode == modeMilestone {
		return eventMilestoneMarked, payload
	}
	_, payload.LostXP = xpEach(p.total, len(g.characters))
	if g.mode == modeGold {
		payload.Gold = g.amount
	}
	for _, tr := range p.treasures {
		payload.Treasures = append(payload.Treasures, eventTreasure{PointID: tr.PointID, ValuePO: tr.ValuePO})
	}
	return eventXPAwarded, payload
}

// announceAward runs after the commit of an award that was not a retry: it
// tells each map whose treasures changed, and, once the open session logged
// the award, writes the log lines and tells the streams the XP changed.
func (s *Service) announceAward(ctx context.Context, m authz.Membership, g grant, given givenAward) {
	if given.repeated {
		return
	}
	if len(given.changedMaps) > 0 {
		s.treasures.PublishChanged(m.CampaignID, given.changedMaps) // once per map
	}
	if !given.logged {
		return
	}
	name := "xp.awarded"
	if g.mode == modeMilestone {
		name = "xp.milestone_marked"
	}
	logging.Event(ctx, s.logger, name, slog.String("award_id", given.award.ID), slog.String("mode", g.mode),
		slog.Int("characters", len(g.characters)), slog.Int64("total_xp", int64(given.award.TotalXp)))
	if len(g.treasureIDs) > 0 {
		logging.Event(ctx, s.logger, "treasure.converted", slog.String("award_id", given.award.ID), slog.Int("treasures", len(g.treasureIDs)))
	}
	s.log.PublishXPChanged(m.CampaignID)
}

// sameRequest checks that a retried key is for the same request, and returns
// the award it made. A key already used for another change is a bug in the
// app, not a retry: another mode, another milestone, "Dar a mais alguém" for a
// first mark (or the reverse), or other characters.
func (s *Service) sameRequest(ctx context.Context, q *progressiondb.Queries, done progressiondb.XpAward, g grant) (progressiondb.XpAward, error) {
	same := done.IdempotencyHash != nil && *done.IdempotencyHash == g.hash
	if done.IdempotencyHash == nil {
		var err error
		if same, err = s.leftTheSame(ctx, q, done, g); err != nil {
			return progressiondb.XpAward{}, err
		}
	}
	if !same {
		return progressiondb.XpAward{}, invalidArgument("idempotency_key", errors.New("was already used for another change"))
	}
	return done, nil
}

// leftTheSame compares a retry with an award from before the request's hash
// was kept, by what the award left: its mode, its milestone and "Dar a mais
// alguém", its treasures (gold) and its characters (a planned milestone).
func (s *Service) leftTheSame(ctx context.Context, q *progressiondb.Queries, done progressiondb.XpAward, g grant) (bool, error) {
	if done.Mode != g.mode || deref(done.MilestoneID) != g.milestoneID || (g.milestoneID != "" && done.MilestoneAgain != g.again) {
		return false, nil
	}
	if g.mode == modeGold {
		// "Voltar à cidade" and a typed amount are different requests.
		rows, err := q.ListXPAwardTreasures(ctx, []string{done.ID})
		if err != nil {
			return false, s.dbError(ctx, "read the treasures of an award", err)
		}
		points := make([]string, 0, len(rows))
		for _, r := range rows {
			points = append(points, r.PointID)
		}
		if !sameIDSet(points, g.treasureIDs) {
			return false, nil
		}
	}
	if g.milestoneID == "" {
		return true, nil
	}
	shares, err := q.ListXPSharesOfAwards(ctx, []string{done.ID})
	if err != nil {
		return false, s.dbError(ctx, "read the shares of an award", err)
	}
	ids := make([]string, 0, len(shares))
	for _, sh := range shares {
		ids = append(ids, sh.CharacterID)
	}
	return sameIDSet(ids, g.characters), nil
}

// sameIDSet says whether got and want hold the same IDs (neither repeats one).
func sameIDSet(got, want []string) bool {
	return len(got) == len(want) && !slices.ContainsFunc(got, func(id string) bool { return !slices.Contains(want, id) })
}

// total is the XP an award splits: the defeated NPCs' XP of an ended combat
// that was not awarded yet, the gold (1 XP per gold piece, RN-09), or the
// amount; 0 for a milestone.
func (s *Service) total(ctx context.Context, tx pgx.Tx, q *progressiondb.Queries, campaignID string, g grant) (int32, error) {
	switch g.mode {
	case modeEnemies:
		enc, err := s.combats.CampaignEncounter(ctx, tx, campaignID, g.encounterID)
		if err != nil {
			return 0, err // not_found for a combat of another campaign
		}
		if !enc.Ended {
			return 0, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_ENCOUNTER_NOT_ENDED, "", 0)
		}
		given, err := q.HasLiveEnemiesAward(ctx, progressiondb.HasLiveEnemiesAwardParams{CampaignID: campaignID, EncounterID: g.encounterID})
		if err != nil {
			return 0, fmt.Errorf("check the combat's award: %w", err)
		}
		if given {
			return 0, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_ALREADY_AWARDED, "", 0)
		}
		if enc.XP < 1 {
			return 0, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_NOTHING_TO_GIVE, "", 0)
		}
		return enc.XP, nil
	case modeGold, modeManual:
		return g.amount, nil
	}
	return 0, nil
}

// appendEvent writes the award's event in the open session's history, if there
// is one.
func (s *Service) appendEvent(ctx context.Context, tx pgx.Tx, m authz.Membership, kind string, payload eventPayload, at time.Time) (bool, error) {
	doc, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("encode the event payload: %w", err)
	}
	logged, err := s.log.AppendEvent(ctx, tx, m.CampaignID, kind, m.UserID, doc, at)
	if err != nil {
		return false, fmt.Errorf("append the session event: %w", err)
	}
	return logged, nil
}

// maxAwardTreasures is the most treasures one "Voltar à cidade" converts.
const maxAwardTreasures = 100

// checkTreasureIDs checks the treasures of a "Voltar à cidade": 1 to 100 UUIDs,
// no repeat. It returns them in canonical form.
func checkTreasureIDs(raw []string) ([]string, error) {
	if len(raw) > maxAwardTreasures {
		return nil, invalidArgument("treasure_point_ids", fmt.Errorf("must have at most %d treasures", maxAwardTreasures))
	}
	ids := make([]string, 0, len(raw))
	for _, r := range raw {
		id, err := uuid.Parse(r)
		if err != nil {
			return nil, invalidArgument("treasure_point_ids", errors.New("must be UUIDs"))
		}
		if slices.Contains(ids, id.String()) {
			return nil, invalidArgument("treasure_point_ids", errors.New("must not repeat a treasure"))
		}
		ids = append(ids, id.String())
	}
	return ids, nil
}

// lockTreasures locks the treasures inside tx and checks that each is a found
// treasure of the campaign that no award converted. A point that is not a
// treasure of the campaign is `not_found`, like any ID of another campaign.
func (s *Service) lockTreasures(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) ([]link.Treasure, error) {
	locked, err := s.treasures.LockForConversion(ctx, tx, campaignID, ids)
	if err != nil {
		return nil, fmt.Errorf("lock the treasures: %w", err)
	}
	// In the order the master sent them (the history lists them by ID).
	out := make([]link.Treasure, 0, len(ids))
	for _, id := range ids {
		i := slices.IndexFunc(locked, func(t link.Treasure) bool { return t.PointID == id })
		if i < 0 {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("treasure not found"))
		}
		tr := locked[i]
		switch {
		case tr.ConvertedAwardID != "":
			return nil, errTreasure(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_TREASURE_ALREADY_CONVERTED, id)
		case !tr.Found:
			return nil, errTreasure(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_TREASURE_NOT_FOUND_YET, id)
		}
		out = append(out, tr)
	}
	return out, nil
}
