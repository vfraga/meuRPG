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
	var g grant
	switch req.Msg.GetMode() {
	case progressionv1.XPAwardMode_XP_AWARD_MODE_ENEMIES:
		g.mode = modeEnemies
		id, err := uuid.Parse(req.Msg.GetEncounterId())
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("encounter not found"))
		}
		g.encounterID = id.String()
		if req.Msg.GetAmount() != 0 || req.Msg.GetGold() != 0 {
			return nil, invalidArgument("amount and gold", errors.New("must be empty for an ENEMIES award"))
		}
	case progressionv1.XPAwardMode_XP_AWARD_MODE_GOLD:
		g.mode = modeGold
		if req.Msg.GetEncounterId() != "" || req.Msg.GetAmount() != 0 {
			return nil, invalidArgument("encounter_id and amount", errors.New("must be empty for a GOLD award"))
		}
		if len(req.Msg.GetTreasurePointIds()) > 0 {
			// "Voltar à cidade": the treasures' PO is the gold.
			if req.Msg.GetGold() != 0 {
				return nil, invalidArgument("gold", errors.New("must be empty when treasure_point_ids is set"))
			}
			if g.treasureIDs, err = checkTreasureIDs(req.Msg.GetTreasurePointIds()); err != nil {
				return nil, err
			}
			break
		}
		if gold := req.Msg.GetGold(); gold < 1 || gold > maxXPAmount {
			return nil, invalidArgument("gold", fmt.Errorf("must be 1 to %d", maxXPAmount))
		}
		g.amount = req.Msg.GetGold()
	case progressionv1.XPAwardMode_XP_AWARD_MODE_MANUAL:
		g.mode = modeManual
		if req.Msg.GetEncounterId() != "" || req.Msg.GetGold() != 0 {
			return nil, invalidArgument("encounter_id and gold", errors.New("must be empty for a MANUAL award"))
		}
		if amount := req.Msg.GetAmount(); amount < 1 || amount > maxXPAmount {
			return nil, invalidArgument("amount", fmt.Errorf("must be 1 to %d", maxXPAmount))
		}
		g.amount = req.Msg.GetAmount()
	default:
		return nil, invalidArgument("mode", errors.New("must be ENEMIES, GOLD or MANUAL"))
	}
	if g.mode != modeGold && len(req.Msg.GetTreasurePointIds()) > 0 {
		return nil, invalidArgument("treasure_point_ids", errors.New("must be empty unless the mode is GOLD"))
	}
	if g.reason, g.key, g.characters, err = checkCommon(req.Msg.GetReason(), req.Msg.GetIdempotencyKey(), req.Msg.GetCharacterIds()); err != nil {
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
	if done, err := s.queries.GetXPAwardByKey(ctx, progressiondb.GetXPAwardByKeyParams{CampaignID: m.CampaignID, IdempotencyKey: g.key}); err == nil {
		return s.sameRequest(ctx, s.queries, done, g)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return progressiondb.XpAward{}, s.dbError(ctx, "find an award by its key", err)
	}

	campaignMode, err := s.campaigns.CampaignXPMode(ctx, nil, m.CampaignID)
	if err != nil {
		return progressiondb.XpAward{}, s.dbError(ctx, "read the campaign's XP mode", err)
	}
	if !fits(campaignMode, g.mode) {
		return progressiondb.XpAward{}, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED, "", campaignMode)
	}
	party, err := s.party.Party(ctx, m.CampaignID)
	if err != nil {
		return progressiondb.XpAward{}, s.dbError(ctx, "read the party", err)
	}
	for _, id := range g.characters {
		if !slices.ContainsFunc(party, func(p link.Member) bool { return p.ID == id }) {
			return progressiondb.XpAward{}, errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE, id, 0)
		}
	}

	var award progressiondb.XpAward
	var repeated, logged bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		award, repeated, logged = progressiondb.XpAward{}, false, false // a retry starts over
		q := s.queries.WithTx(tx)
		if done, err := q.GetXPAwardByKey(ctx, progressiondb.GetXPAwardByKeyParams{CampaignID: m.CampaignID, IdempotencyKey: g.key}); err == nil {
			repeated = true // a request that raced with its own retry
			award, err = s.sameRequest(ctx, q, done, g)
			return err
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find an award by its key: %w", err)
		}

		// The mode is read again here, in the transaction (RN-09): SetCampaignXpMode
		// reads the awards and writes the mode in a transaction of its own, so under
		// serializable isolation the two cannot both pass, and one retries.
		txMode, err := s.campaigns.CampaignXPMode(ctx, tx, m.CampaignID)
		if err != nil {
			return fmt.Errorf("read the campaign's XP mode: %w", err)
		}
		if !fits(txMode, g.mode) {
			return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_MODE_NOT_ALLOWED, "", txMode)
		}
		reason := g.reason
		if g.milestoneID != "" {
			if reason, err = s.checkMilestone(ctx, q, m.CampaignID, g); err != nil {
				return err
			}
		}
		// "Voltar à cidade": lock and check the treasures, and their PO is the
		// gold. The lock holds until the commit, so two awards for one treasure
		// take turns and the second finds it converted.
		var treasures []link.Treasure
		if len(g.treasureIDs) > 0 {
			if treasures, err = s.lockTreasures(ctx, tx, m.CampaignID, g.treasureIDs); err != nil {
				return err
			}
			g.amount = 0
			for _, tr := range treasures {
				g.amount += tr.ValuePO
			}
			if g.amount > maxXPAmount {
				return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_TREASURES_OVER_LIMIT, "", 0)
			}
		}
		total, err := s.total(ctx, tx, q, m.CampaignID, g)
		if err != nil {
			return err
		}
		each := int32(0)
		if g.mode != modeMilestone {
			if each, _ = xpEach(total, len(g.characters)); each < 1 {
				return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_NOTHING_TO_GIVE, "", 0)
			}
		}
		now := s.now()
		params := progressiondb.InsertXPAwardParams{
			CampaignID: m.CampaignID, GivenBy: m.UserID, CreatedAt: now, Mode: g.mode, Reason: reason,
			TotalXp: total, IdempotencyKey: g.key, IdempotencyHash: &g.hash,
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
		if award, err = q.InsertXPAward(ctx, params); err != nil {
			if isUniqueViolation(err, oneAwardPerEncounterIndex) {
				return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_ALREADY_AWARDED, "", 0)
			}
			if isUniqueViolation(err, idempotencyKeyIndex) {
				return connect.NewError(connect.CodeAborted, errors.New("the same request is being handled, please try again"))
			}
			return fmt.Errorf("insert an award: %w", err)
		}
		for _, id := range g.characters {
			share := progressiondb.InsertXPShareParams{AwardID: award.ID, CharacterID: id, Xp: each}
			if g.mode == modeMilestone {
				// The tag lasts until the sheet's level goes past this one.
				level := party[slices.IndexFunc(party, func(p link.Member) bool { return p.ID == id })].Level
				share.LevelAtMark = &level
			} else {
				// The sheet holds at most 1,000,000 XP: the share keeps what the
				// sheet gained, so an undo takes back exactly that.
				before, after, err := s.party.AddExperience(ctx, tx, m.CampaignID, id, each, now)
				if err != nil {
					if connect.CodeOf(err) == connect.CodeNotFound {
						return errBlocked(progressionv1.XPBlockedReason_XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE, id, 0)
					}
					return fmt.Errorf("add the XP to a sheet: %w", err)
				}
				share.Xp = after - before
			}
			if err := q.InsertXPShare(ctx, share); err != nil {
				return fmt.Errorf("insert a share: %w", err)
			}
		}
		for _, tr := range treasures {
			if err := q.InsertXPAwardTreasure(ctx, progressiondb.InsertXPAwardTreasureParams{AwardID: award.ID, PointID: tr.PointID, ValuePo: tr.ValuePO}); err != nil {
				return fmt.Errorf("insert a converted treasure: %w", err)
			}
		}
		if len(treasures) > 0 {
			if err := s.treasures.MarkConverted(ctx, tx, award.ID, g.treasureIDs); err != nil {
				return err
			}
		}
		payload := eventPayload{AwardID: award.ID, Mode: g.mode, TotalXP: total, XPEach: each, CharacterIDs: g.characters, EncounterID: g.encounterID, MilestoneID: g.milestoneID}
		_, payload.LostXP = xpEach(total, len(g.characters))
		if g.mode == modeGold {
			payload.Gold = g.amount
		}
		for _, tr := range treasures {
			payload.Treasures = append(payload.Treasures, eventTreasure{PointID: tr.PointID, ValuePO: tr.ValuePO})
		}
		kind := eventXPAwarded
		if g.mode == modeMilestone {
			kind, payload.LostXP = eventMilestoneMarked, 0
		}
		logged, err = s.appendEvent(ctx, tx, m, kind, payload, now)
		return err
	})
	if err != nil {
		return progressiondb.XpAward{}, s.dbError(ctx, "give XP", err)
	}
	if logged && !repeated {
		name := "xp.awarded"
		if g.mode == modeMilestone {
			name = "xp.milestone_marked"
		}
		logging.Event(ctx, s.logger, name, slog.String("award_id", award.ID), slog.String("mode", g.mode),
			slog.Int("characters", len(g.characters)), slog.Int64("total_xp", int64(award.TotalXp)))
		if len(g.treasureIDs) > 0 {
			logging.Event(ctx, s.logger, "treasure.converted", slog.String("award_id", award.ID), slog.Int("treasures", len(g.treasureIDs)))
		}
		s.log.PublishXPChanged(m.CampaignID)
	}
	return award, nil
}

// sameRequest checks that a retried key is for the same request, and returns
// the award it made. A key already used for another change is a bug in the
// app, not a retry: another mode, another milestone, "Dar a mais alguém" for a
// first mark (or the reverse), or other characters.
func (s *Service) sameRequest(ctx context.Context, q *progressiondb.Queries, done progressiondb.XpAward, g grant) (progressiondb.XpAward, error) {
	reused := invalidArgument("idempotency_key", errors.New("was already used for another change"))
	if done.IdempotencyHash != nil {
		if *done.IdempotencyHash != g.hash {
			return progressiondb.XpAward{}, reused
		}
		return done, nil
	}
	// An award from before the hash was kept: compared by what it left.
	if done.Mode != g.mode || deref(done.MilestoneID) != g.milestoneID || (g.milestoneID != "" && done.MilestoneAgain != g.again) {
		return progressiondb.XpAward{}, reused
	}
	if g.mode == modeGold {
		// "Voltar à cidade" and a typed amount are different requests.
		rows, err := q.ListXPAwardTreasures(ctx, []string{done.ID})
		if err != nil {
			return progressiondb.XpAward{}, s.dbError(ctx, "read the treasures of an award", err)
		}
		if len(rows) != len(g.treasureIDs) || slices.ContainsFunc(rows, func(r progressiondb.XpAwardTreasure) bool { return !slices.Contains(g.treasureIDs, r.PointID) }) {
			return progressiondb.XpAward{}, reused
		}
	}
	if g.milestoneID != "" {
		shares, err := q.ListXPSharesOfAwards(ctx, []string{done.ID})
		if err != nil {
			return progressiondb.XpAward{}, s.dbError(ctx, "read the shares of an award", err)
		}
		ids := make([]string, 0, len(shares))
		for _, sh := range shares {
			ids = append(ids, sh.CharacterID)
		}
		if len(ids) != len(g.characters) || slices.ContainsFunc(ids, func(id string) bool { return !slices.Contains(g.characters, id) }) {
			return progressiondb.XpAward{}, reused
		}
	}
	return done, nil
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
