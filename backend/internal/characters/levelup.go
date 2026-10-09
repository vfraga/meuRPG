package characters

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The guided level-up (MR-040, RN-01's exception, RN-12). Until the full
// level-up exists, a character that can level up (Character.can_level_up) may
// change its locked sheet, but only to add what the next level gives. Package
// rules says what that is (LevelUpOptions, ApplyLevelUp, CheckLevelUp); this
// file is the part that needs the database: who may call, the kept hit die
// roll, saving the sheet with the record of the level-up for the master, and
// the hint to the live session.

// DiceRules tells what the campaign's dice setting makes a player do with the
// hit die (RN-18). cmd/api wires it to the campaigns module, so this package
// never imports it. Without one, every player chooses.
type DiceRules interface {
	// LevelUpDice returns the rule for userID, an active member of the campaign. It
	// reads inside tx when the caller has one (nil: the pool).
	LevelUpDice(ctx context.Context, tx pgx.Tx, campaignID, userID string) (charactersv1.LevelUpDiceRule, error)
}

// Live is the session's live stream (package play implements it): a level-up
// changes the sheet everyone watches, so every stream of the campaign gets the
// same content-free hint as an XP award, `xp_changed`; a write to the table's
// content gets `content_changed`.
type Live interface {
	// PublishXPChanged tells every stream of the campaign that the XP or the
	// levels changed. Call it after the commit.
	PublishXPChanged(campaignID string)
	// PublishContentChanged tells every stream of the campaign that the table's
	// content changed (an entry written, archived or brought back, an option
	// switched on or off): `content_changed`, a hint with no content (RN-10) that
	// each app answers by reading the content as its own role. Call it after the
	// commit. The play module throttles it per campaign.
	PublishContentChanged(campaignID string)
	// PublishVitalsChanged sends the character's vitals as they are now, as the
	// master's correction of them does, to the streams that already receive that
	// character's hit points (the master and the player who plays it). A level-up
	// changes the maximum and the current hit points, so every open screen shows
	// them at once. Call it after the commit.
	PublishVitalsChanged(ctx context.Context, campaignID, characterID string)
}

// SetLive connects the live stream. Without it, a level-up still works and
// nothing is published.
func (s *Service) SetLive(l Live) { s.live = l }

// The page sizes of ListLevelUps.
const (
	defaultLevelUpsPage = 20
	maxLevelUpsListed   = 50
)

// levelUpsToken is the cursor of the list: the time and ID of the last
// level-up of a page, which is all the keyset query needs. It is opaque to
// the app.
func levelUpsToken(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMicro(), 10) + ":" + id))
}

func parseLevelUpsToken(token string) (time.Time, string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, "", false
	}
	micro, id, ok := strings.Cut(string(raw), ":")
	if !ok {
		return time.Time{}, "", false
	}
	n, err := strconv.ParseInt(micro, 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return time.Time{}, "", false
	}
	return time.UnixMicro(n).UTC(), u.String(), true
}

// The database's text values for the hit points method of a level-up
// (character_level_ups_hp_method_valid).
var (
	hpMethodToDB = map[charactersv1.LevelUpHitPointsMethod]string{
		charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE:         "average",
		charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP:   "rolled_in_app",
		charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_PHYSICAL: "rolled_physical",
	}
	refusalReasons = map[string]charactersv1.LevelUpRefusalReason{
		rules.LevelUpReasonClass:          charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_CLASS,
		rules.LevelUpReasonMaxLevel:       charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_MAX_LEVEL,
		rules.LevelUpReasonLocked:         charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_LOCKED_FIELD,
		rules.LevelUpReasonAbilityNotDue:  charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ABILITY_NOT_DUE,
		rules.LevelUpReasonAbilityShape:   charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ABILITY_SHAPE,
		rules.LevelUpReasonAbilityAbove20: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ABILITY_ABOVE_20,
		rules.LevelUpReasonHitPoints:      charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINTS,
		rules.LevelUpReasonSubclass:       charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SUBCLASS,
		rules.LevelUpReasonCantrips:       charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_CANTRIPS,
		rules.LevelUpReasonSpells:         charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SPELLS,
		rules.LevelUpReasonPrepared:       charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_PREPARED,
		rules.LevelUpReasonFeatureChoice:  charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE,
		rules.LevelUpReasonSkills:         charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SKILLS,
		rules.LevelUpReasonExpertise:      charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_EXPERTISE,
		rules.LevelUpReasonSheetIssue:     charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SHEET_ISSUE,
	}
)

// errRefused is the failed_precondition of a set of choices that break a rule
// of the level, with the LevelUpRefusal detail.
func errRefused(r *charactersv1.LevelUpRefusal) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New("the level up does not allow this choice: "+r.GetField()))
	if detail, detailErr := connect.NewErrorDetail(r); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// refusalOf turns a package rules refusal into the message.
func refusalOf(e *rules.LevelUpError) *charactersv1.LevelUpRefusal {
	return &charactersv1.LevelUpRefusal{Field: e.Field, Reason: refusalReasons[e.Reason], IssueCode: e.Code}
}

// levelUpTarget is the character a guided level-up call is about, checked
// before anything else: the caller may see it, it is a living player
// character, and it can level up now. owner is true for the owning player.
type levelUpTarget struct {
	row   charactersdb.Character
	full  *charactersv1.FullSheet
	build rules.Build
	// classKey is the class that gains the level; idx its place in the sheet.
	classKey string
	idx      int
	// content is the campaign's rules content, read in the caller's transaction.
	content *rules.Content
	// rules are the campaign's table rules, read with the content: the hit points
	// method the table allows (RN-24).
	rules TableRules
}

// newLevelUpTarget runs the checks every guided level-up call shares, on a
// row the caller already read (locked, in a write). It answers `not_found` for
// a character the caller may not see, the CharacterBlocked detail for one
// that is dead, pending or cannot level up, and `invalid_argument` for a class
// the character does not have. An empty classKey means the first class.
func (s *Service) newLevelUpTarget(ctx context.Context, tx pgx.Tx, m authz.Membership, row charactersdb.Character, classKey string) (levelUpTarget, error) {
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return levelUpTarget{}, errCharacterNotFound()
	}
	switch row.Status {
	case statusDead:
		return levelUpTarget{}, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD, row.ID)
	case statusPending:
		return levelUpTarget{}, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_AWAITING_APPROVAL, row.ID)
	}
	cannot := errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CANNOT_LEVEL_UP, row.ID)
	sheet, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return levelUpTarget{}, err
	}
	full := sheet.GetFull()
	if row.Kind != kindPlayer || full == nil || s.levelUps == nil {
		return levelUpTarget{}, cannot
	}
	content, tableRules, err := s.content.For(ctx, tx, m.CampaignID)
	if err != nil {
		return levelUpTarget{}, wrap("read rules content", err)
	}
	d := rules.Derive(buildOf(full), content)
	reason, err := s.levelUps.LevelUpReason(ctx, tx, deref(row.CampaignID), row.ID, i32(d.TotalLevel), full.GetExperiencePoints(), i32(d.NextLevelXP))
	if err != nil {
		return levelUpTarget{}, wrap("check the level up", err)
	}
	if reason == charactersv1.LevelUpReason_LEVEL_UP_REASON_UNSPECIFIED {
		return levelUpTarget{}, cannot
	}
	build := buildOf(full)
	// A sheet that does not pass today's rules by itself cannot be fixed by
	// the level's choices: it is the master's to correct.
	if err := rules.Validate(build, content); err != nil {
		field := ""
		if ve, ok := errors.AsType[*rules.ValidationError](err); ok {
			field = ve.Field
		}
		return levelUpTarget{}, errRefused(&charactersv1.LevelUpRefusal{
			Field: field, Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SHEET_NEEDS_MASTER,
		})
	}
	if classKey == "" && len(build.Classes) > 0 {
		classKey = build.Classes[0].Class
	}
	idx := slices.IndexFunc(build.Classes, func(c rules.ClassLevel) bool { return c.Class == classKey })
	if idx < 0 {
		return levelUpTarget{}, invalidArgument(fieldErr("class_key", "is not a class of the character"))
	}
	return levelUpTarget{row: row, full: full, build: build, classKey: classKey, idx: idx, content: content, rules: tableRules}, nil
}

// readLevelUpTarget is newLevelUpTarget for a read: the row is not locked. It
// reads through tx, a read transaction of the caller, so the row, the content and
// the level-up reason are one moment.
func (s *Service) readLevelUpTarget(ctx context.Context, tx pgx.Tx, m authz.Membership, characterID, classKey string) (levelUpTarget, error) {
	id, ok := parseUUID(characterID)
	if !ok {
		return levelUpTarget{}, errCharacterNotFound()
	}
	row, err := s.queriesIn(tx).GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
	if err != nil {
		return levelUpTarget{}, wrap("get a character", err)
	}
	return s.newLevelUpTarget(ctx, tx, m, row, classKey)
}

// diceRule is what the campaign's dice setting makes the caller do.
func (s *Service) diceRule(ctx context.Context, tx pgx.Tx, m authz.Membership) (charactersv1.LevelUpDiceRule, error) {
	if s.dice == nil {
		return charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_PLAYER_CHOOSES, nil
	}
	rule, err := s.dice.LevelUpDice(ctx, tx, m.CampaignID, m.UserID)
	if err != nil {
		return 0, wrap("read the dice setting", err)
	}
	return rule, nil
}

// GetLevelUpOptions implements charactersv1connect.CharacterServiceHandler.
func (s *Service) GetLevelUpOptions(
	ctx context.Context,
	req *connect.Request[charactersv1.GetLevelUpOptionsRequest],
) (*connect.Response[charactersv1.GetLevelUpOptionsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	// The character, the dice setting and the kept roll are one moment: a level-up
	// that commits between them would offer the level just taken, with its roll gone.
	var (
		t         levelUpTarget
		offer     rules.LevelUpOffer
		diceRule  charactersv1.LevelUpDiceRule
		roll      charactersdb.GetLevelUpRollRow
		rollFound bool
	)
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if t, err = s.readLevelUpTarget(ctx, tx, m, req.Msg.GetCharacterId(), req.Msg.GetClassKey()); err != nil {
			return err
		}
		if offer, err = rules.LevelUpOptions(t.build, t.classKey, t.content); err != nil {
			return levelUpRulesError(err)
		}
		if diceRule, err = s.diceRule(ctx, tx, m); err != nil {
			return err
		}
		roll, err = s.queries.WithTx(tx).GetLevelUpRoll(ctx, charactersdb.GetLevelUpRollParams{CharacterID: t.row.ID, ToLevel: i32(offer.TotalTo)})
		rollFound = err == nil
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return wrap("read the kept roll", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "read the level up options", err)
	}
	out := levelUpOptionsToProto(offer)
	if !isMaster(m) {
		// A subclass the master archived or switched off is not offered to a player (RN-23), unless
		// the character's own sheet already has it.
		out.Subclasses = slices.DeleteFunc(out.Subclasses, func(sub *charactersv1.LevelUpSubclass) bool {
			return (sub.GetArchived() || sub.GetOff()) && !slices.ContainsFunc(t.build.Classes, func(c rules.ClassLevel) bool { return c.Subclass == sub.GetKey() })
		})
		// Nor a reference to a hidden class the sheet does not use (a list a table
		// class or subclass reuses).
		hiddenList := func(k string) bool {
			return k != "" && t.content.Hidden(k) && !slices.ContainsFunc(t.build.Classes, func(c rules.ClassLevel) bool { return c.Class == k })
		}
		if hiddenList(out.GetSpellListClassKey()) {
			out.SpellListClassKey = ""
		}
		for _, sub := range out.Subclasses {
			if hiddenList(sub.GetSpellListClassKey()) {
				sub.SpellListClassKey = ""
			}
		}
	}
	out.DiceRule = diceRule
	out.HitPointsRule = hitPointsRuleToProto(t.rules.HitPoints)
	if rollFound {
		out.KeptHitPointRoll, out.KeptHitPointRollClassKey = roll.Value, roll.ClassKey
	}
	return connect.NewResponse(&charactersv1.GetLevelUpOptionsResponse{Options: out}), nil
}

// levelUpRulesError turns a refusal from package rules, in a call that
// has no choices to refuse, into the error the client gets.
func levelUpRulesError(err error) error {
	if le, ok := errors.AsType[*rules.LevelUpError](err); ok {
		return errRefused(refusalOf(le))
	}
	return err
}

// PreviewLevelUp implements charactersv1connect.CharacterServiceHandler.
func (s *Service) PreviewLevelUp(
	ctx context.Context,
	req *connect.Request[charactersv1.PreviewLevelUpRequest],
) (*connect.Response[charactersv1.PreviewLevelUpResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	choices := req.Msg.GetChoices()
	// The character and the roll the plan keeps are one moment: a level-up that
	// commits between them would refuse a roll that was just used.
	var (
		t    levelUpTarget
		plan levelUpPlan
	)
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if t, err = s.readLevelUpTarget(ctx, tx, m, req.Msg.GetCharacterId(), choices.GetClassKey()); err != nil {
			return err
		}
		plan, err = s.planLevelUpIn(ctx, tx, s.queries.WithTx(tx), m, t, choices)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "preview a level up", err)
	}
	// A retired or switched-off choice builds no sheet, whatever else is refused: a
	// player who guesses such a key must not read its features in `after` (RN-23).
	if plan.retiredChoice {
		return connect.NewResponse(&charactersv1.PreviewLevelUpResponse{Refusal: plan.refusal}), nil
	}
	return connect.NewResponse(&charactersv1.PreviewLevelUpResponse{
		After:   derivedToProto(rules.Derive(buildOf(plan.sheet), t.content)),
		Refusal: plan.refusal,
	}), nil
}

// RollLevelUpHitPoints implements charactersv1connect.CharacterServiceHandler.
func (s *Service) RollLevelUpHitPoints(
	ctx context.Context,
	req *connect.Request[charactersv1.RollLevelUpHitPointsRequest],
) (*connect.Response[charactersv1.RollLevelUpHitPointsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	// The master keeps the sheet editor: the guided level-up is the player's.
	if isMaster(m) {
		return nil, errPermission("only the owning player levels up a character with the guided level-up")
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	if _, ok := parseUUID(req.Msg.GetIdempotencyKey()); !ok {
		return nil, invalidArgument(fieldErr("idempotency_key", "must be a UUID"))
	}
	res := &charactersv1.RollLevelUpHitPointsResponse{}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		row, err := visibleForUpdate(ctx, q, m, id) // locked: two rolls of the same level wait for each other
		if err != nil {
			return err
		}
		t, err := s.newLevelUpTarget(ctx, tx, m, row, req.Msg.GetClassKey())
		if err != nil {
			return err
		}
		rule, err := s.diceRule(ctx, tx, m)
		if err != nil {
			return err
		}
		if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL {
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_PHYSICAL, row.ID)
		}
		// A table where everybody takes the average never rolls (RN-24).
		if t.rules.HitPoints == HitPointsAverage {
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_HIT_POINTS_AVERAGE_ONLY, row.ID)
		}
		offer, lerr := rules.LevelUpOptions(t.build, t.classKey, t.content)
		if lerr != nil {
			return levelUpRulesError(lerr)
		}
		key := charactersdb.GetLevelUpRollParams{CharacterID: row.ID, ToLevel: i32(offer.TotalTo)}
		kept, err := q.GetLevelUpRoll(ctx, key)
		if err == nil {
			// One roll per level, for the class that rolled it.
			if kept.ClassKey != t.classKey {
				return errRefused(&charactersv1.LevelUpRefusal{
					Field: "class_key", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINT_ROLL_OTHER_CLASS,
				})
			}
			res.Die, res.Value, res.AlreadyRolled = kept.Die, kept.Value, true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return wrap("read the kept roll", err)
		}
		rolled, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: offer.HitDie})
		if err != nil {
			return wrap("roll the hit die", err)
		}
		if _, err := q.InsertLevelUpRoll(ctx, charactersdb.InsertLevelUpRollParams{
			CharacterID: row.ID, ClassKey: t.classKey, ToLevel: key.ToLevel, Die: i32(offer.HitDie), Value: i32(rolled.Total), Now: s.now(),
		}); err != nil {
			return wrap("keep the roll", err)
		}
		res.Die, res.Value, res.AlreadyRolled = i32(offer.HitDie), i32(rolled.Total), false
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll the level up hit points", err)
	}
	return connect.NewResponse(res), nil
}

// LevelUpCharacter implements charactersv1connect.CharacterServiceHandler.
func (s *Service) LevelUpCharacter(
	ctx context.Context,
	req *connect.Request[charactersv1.LevelUpCharacterRequest],
) (*connect.Response[charactersv1.LevelUpCharacterResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if isMaster(m) {
		return nil, errPermission("only the owning player levels up a character with the guided level-up")
	}
	id, ok := parseUUID(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	revision := req.Msg.GetRevision()
	if revision < 1 {
		return nil, invalidArgument(fieldErr("revision", "must be at least 1"))
	}
	choices := req.Msg.GetChoices()
	if err := checkLevelUpChoices(choices); err != nil {
		return nil, invalidArgument(err)
	}
	if _, ok := hpMethodToDB[choices.GetHitPoints().GetMethod()]; !ok {
		return nil, invalidArgument(fieldErr("choices.hit_points.method", "is required"))
	}

	var row charactersdb.Character
	var leveled bool
	var leveledContent *rules.Content // the content read in the transaction
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		current, err := visibleForUpdate(ctx, q, m, id)
		if err != nil {
			return err
		}
		// The checks that stand in for the sheet's lock come in this order:
		// who the character is (dead, pending), then the revision (a retry of
		// a level-up that already went through is stale, not "cannot"), then
		// whether it can level up now.
		switch current.Status {
		case statusDead:
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD, current.ID)
		case statusPending:
			return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_AWAITING_APPROVAL, current.ID)
		}
		if current.Revision != revision {
			return errStaleRevision()
		}
		t, err := s.newLevelUpTarget(ctx, tx, m, current, choices.GetClassKey())
		if err != nil {
			return err
		}
		plan, err := s.planLevelUpIn(ctx, tx, q, m, t, choices)
		if err != nil {
			return err
		}
		if plan.refusal != nil {
			return errRefused(plan.refusal)
		}
		// A level-up never clears what a change of the table's content flagged.
		carryFlags(t.content, t.full, plan.sheet)
		leveledContent = t.content
		doc, err := storeJSON.Marshal(&charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: plan.sheet}})
		if err != nil {
			return wrap("encode a sheet", err)
		}
		row, err = q.UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
			CampaignID: m.CampaignID, ID: id, Revision: revision, Name: current.Name, Sheet: doc, Now: s.now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStaleRevision()
		}
		if err != nil {
			return wrap("update sheet", err)
		}
		if err := s.carryHitPoints(ctx, q, t.content, id, current.Sheet, doc); err != nil {
			return err
		}
		record, err := storeJSON.Marshal(plan.record)
		if err != nil {
			return wrap("encode the level up", err)
		}
		if _, err := q.InsertLevelUp(ctx, charactersdb.InsertLevelUpParams{
			CampaignID: m.CampaignID, CharacterID: id, ClassKey: t.classKey,
			FromLevel: i32(plan.toLevel - 1), ToLevel: i32(plan.toLevel),
			HpMethod: hpMethodToDB[plan.record.GetHitPoints().GetMethod()], HpValue: plan.record.GetHitPoints().GetValue(),
			Choices: record, CreatedAt: s.now(),
		}); err != nil {
			return wrap("record the level up", err)
		}
		// The roll was used: the next level rolls again.
		if err := q.DeleteLevelUpRoll(ctx, charactersdb.DeleteLevelUpRollParams{CharacterID: id, ToLevel: i32(plan.toLevel)}); err != nil {
			return wrap("delete the used roll", err)
		}
		leveled = true
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "level up a character", err)
	}
	if leveled {
		logging.Event(ctx, s.logger, "levelup.done", slog.String("character_id", id))
	}
	if leveled && s.live != nil {
		s.live.PublishXPChanged(m.CampaignID)
		s.live.PublishVitalsChanged(ctx, m.CampaignID, id)
	}
	// The response is derived from the content the level-up was checked with.
	c, err := s.character(ctx, leveledContent, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a leveled up character", err)
	}
	return connect.NewResponse(&charactersv1.LevelUpCharacterResponse{Character: c}), nil
}

// ListLevelUps implements charactersv1connect.CharacterServiceHandler.
func (s *Service) ListLevelUps(
	ctx context.Context,
	req *connect.Request[charactersv1.ListLevelUpsRequest],
) (*connect.Response[charactersv1.ListLevelUpsResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	size := req.Msg.GetPageSize()
	switch {
	case size == 0:
		size = defaultLevelUpsPage
	case size < 0 || size > maxLevelUpsListed:
		return nil, invalidArgument(fieldErr("page_size", "must be 1 to %d", maxLevelUpsListed))
	}
	params := charactersdb.ListLevelUpsParams{CampaignID: m.CampaignID, MaxRows: size + 1} // one more tells whether there is a next page
	if token := req.Msg.GetPageToken(); token != "" {
		at, id, ok := parseLevelUpsToken(token)
		if !ok {
			return nil, invalidArgument(fieldErr("page_token", "is not a token of this list"))
		}
		params.BeforeCreatedAt, params.BeforeID = &at, &id
	}
	if raw := req.Msg.GetCharacterId(); raw != "" {
		id, ok := parseUUID(raw)
		if !ok {
			return nil, errCharacterNotFound()
		}
		params.CharacterID = &id
	}
	rows, err := s.queries.ListLevelUps(ctx, params)
	if err != nil {
		return nil, s.dbError(ctx, "list level ups", err)
	}
	res := &charactersv1.ListLevelUpsResponse{}
	if int32(len(rows)) > size { //nolint:gosec // at most maxLevelUpsListed+1
		rows = rows[:size]
		last := rows[len(rows)-1]
		res.NextPageToken = levelUpsToken(last.CreatedAt, last.ID)
	}
	var playerIDs []string
	for _, row := range rows {
		if row.PlayerUserID != nil {
			playerIDs = append(playerIDs, *row.PlayerUserID)
		}
	}
	displayNames, err := s.displayNames(ctx, playerIDs)
	if err != nil {
		return nil, s.dbError(ctx, "read display names", err)
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	for _, row := range rows {
		choices := &charactersv1.LevelUpChoices{}
		if err := loadJSON.Unmarshal(row.Choices, choices); err != nil {
			return nil, s.dbError(ctx, "list level ups", errors.Join(errCorruptDocument, err))
		}
		res.LevelUps = append(res.LevelUps, &charactersv1.LevelUp{
			Id: row.ID, CharacterId: row.CharacterID, CharacterName: row.CharacterName,
			PlayerDisplayName: displayNames[deref(row.PlayerUserID)],
			ClassKey:          row.ClassKey, ClassNamePt: content.NamePT(row.ClassKey),
			FromLevel: row.FromLevel, ToLevel: row.ToLevel,
			Choices: choices, NamesPt: levelUpNames(content, row.ClassKey, choices),
			CreatedAt: timestamppb.New(row.CreatedAt),
		})
	}
	return connect.NewResponse(res), nil
}

// levelUpNames names every content key a level-up's choices hold.
func levelUpNames(content *rules.Content, classKey string, c *charactersv1.LevelUpChoices) map[string]string {
	names := map[string]string{}
	keys := slices.Concat([]string{classKey, c.GetSubclassKey()}, c.GetCantripKeys(), c.GetKnownSpellKeys(),
		c.GetPreparedSpellKeys(), c.GetFeatureChoiceKeys(), c.GetSkillProficiencyKeys(), c.GetExpertiseSkillKeys())
	for _, k := range keys {
		if name := content.NamePT(k); k != "" && name != "" {
			names[k] = name
		}
	}
	return names
}

// checkLevelUpChoices checks the shape of the choices: the lists' sizes (the
// same limits as the sheet's), before anything is built from them. Unknown
// keys are found by rules.Validate on the new sheet.
func checkLevelUpChoices(c *charactersv1.LevelUpChoices) error {
	if c == nil {
		return fieldErr("choices", "is required")
	}
	for _, l := range []struct {
		field string
		keys  []string
		limit int
	}{
		{"choices.cantrip_keys", c.GetCantripKeys(), rules.MaxCantrips},
		{"choices.known_spell_keys", c.GetKnownSpellKeys(), rules.MaxKnownSpells},
		{"choices.prepared_spell_keys", c.GetPreparedSpellKeys(), rules.MaxPreparedSpells},
		{"choices.feature_choice_keys", c.GetFeatureChoiceKeys(), rules.MaxListLength},
		{"choices.skill_proficiency_keys", c.GetSkillProficiencyKeys(), rules.MaxSkillKeys},
		{"choices.expertise_skill_keys", c.GetExpertiseSkillKeys(), rules.MaxSkillKeys},
	} {
		if len(l.keys) > l.limit {
			return fieldErr(l.field, "must have at most %d entries", l.limit)
		}
	}
	for _, l := range []struct {
		field string
		keys  []string
	}{
		{"choices.cantrip_keys", c.GetCantripKeys()},
		{"choices.known_spell_keys", c.GetKnownSpellKeys()},
		{"choices.prepared_spell_keys", c.GetPreparedSpellKeys()},
		{"choices.feature_choice_keys", c.GetFeatureChoiceKeys()},
		{"choices.skill_proficiency_keys", c.GetSkillProficiencyKeys()},
		{"choices.expertise_skill_keys", c.GetExpertiseSkillKeys()},
	} {
		if len(slices.Compact(slices.Sorted(slices.Values(l.keys)))) != len(l.keys) {
			return fieldErr(l.field, "repeats an entry")
		}
	}
	if m := c.GetHitPoints().GetMethod(); charactersv1.LevelUpHitPointsMethod_name[int32(m)] == "" {
		return fieldErr("choices.hit_points.method", "is not a known method")
	}
	if v := c.GetHitPoints().GetValue(); v < 0 || v > 1000 {
		return fieldErr("choices.hit_points.value", "must be 0 to 1000")
	}
	return nil
}

// levelUpPlan is what a set of choices makes of a character's sheet.
type levelUpPlan struct {
	// sheet is the new full sheet: the stored one with the choices added.
	sheet *charactersv1.FullSheet
	// refusal is the first rule the choices break, nil when they are allowed.
	refusal *charactersv1.LevelUpRefusal
	// retiredChoice says a new choice is one the master archived or, for a player,
	// switched off, whatever the refusal is: the sheet it builds is not theirs to read
	// (RN-23).
	retiredChoice bool
	// record is what ListLevelUps keeps: the choices, with the hit points
	// value the level took.
	record *charactersv1.LevelUpChoices
	// toLevel is the character's total level after the level-up.
	toLevel int
}

// planLevelUpIn builds the new sheet from the stored one and the choices, and
// checks it with the rules. The error is a Connect error for what the client
// got wrong (`invalid_argument`, the dice setting); a rule of the level that
// the choices break is plan.refusal. A choice that is not ready (a preview
// without a hit points method, an in-app roll that was never made) takes the
// average, and the plan still carries the sheet as far as the choices go.
func (s *Service) planLevelUpIn(ctx context.Context, tx pgx.Tx, q *charactersdb.Queries, m authz.Membership, t levelUpTarget, choices *charactersv1.LevelUpChoices) (levelUpPlan, error) {
	if err := checkLevelUpChoices(choices); err != nil {
		return levelUpPlan{}, invalidArgument(err)
	}
	offer, lerr := rules.LevelUpOptions(t.build, t.classKey, t.content)
	if lerr != nil {
		return levelUpPlan{}, levelUpRulesError(lerr)
	}
	plan := levelUpPlan{toLevel: offer.TotalTo, record: proto.CloneOf(choices)}
	plan.record.ClassKey = t.classKey

	// The hit points of the level.
	method := choices.GetHitPoints().GetMethod()
	if method == charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_UNSPECIFIED {
		method = charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE // a preview without a method
	}
	hp := rules.LevelUpHitPoints{Average: true}
	value := offer.HitPointAverage
	// The table's rule (RN-24) refuses the method it does not allow, with the
	// average standing in for the rest of the plan. A preview without a method
	// has not chosen one, so it is not refused.
	if chosen := choices.GetHitPoints().GetMethod(); chosen != charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_UNSPECIFIED &&
		!hitPointsMethodAllowed(t.rules.HitPoints, chosen) {
		plan.refusal = &charactersv1.LevelUpRefusal{
			Field: "choices.hit_points.method", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINTS_RULE,
		}
		method = charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE
	}
	switch method {
	case charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP:
		rule, err := s.diceRule(ctx, tx, m)
		if err != nil {
			return levelUpPlan{}, err
		}
		if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL {
			return levelUpPlan{}, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_PHYSICAL, t.row.ID)
		}
		kept, err := q.GetLevelUpRoll(ctx, charactersdb.GetLevelUpRollParams{CharacterID: t.row.ID, ToLevel: i32(offer.TotalTo)})
		switch {
		case err == nil && kept.ClassKey != t.classKey:
			plan.refusal = &charactersv1.LevelUpRefusal{
				Field: "choices.hit_points", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINT_ROLL_OTHER_CLASS,
			}
		case err == nil:
			hp, value = rules.LevelUpHitPoints{Roll: int(kept.Value)}, int(kept.Value)
		case errors.Is(err, pgx.ErrNoRows):
			plan.refusal = &charactersv1.LevelUpRefusal{
				Field: "choices.hit_points", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINT_ROLL_MISSING,
			}
		default:
			return levelUpPlan{}, wrap("read the kept roll", err)
		}
	case charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_PHYSICAL:
		rule, err := s.diceRule(ctx, tx, m)
		if err != nil {
			return levelUpPlan{}, err
		}
		if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_IN_APP {
			return levelUpPlan{}, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_IN_APP, t.row.ID)
		}
		if typed := int(choices.GetHitPoints().GetValue()); typed < 1 || typed > offer.HitDie {
			// Not a result of this die: refuse it, and build the rest with the average.
			plan.refusal = &charactersv1.LevelUpRefusal{
				Field: "choices.hit_points.value", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINTS,
			}
		} else {
			hp, value = rules.LevelUpHitPoints{Roll: typed}, typed
		}
	}
	plan.record.HitPoints = &charactersv1.LevelUpHitPoints{Method: method, Value: i32(value)}

	// The new Build, written onto a copy of the stored sheet.
	after, err := rules.ApplyLevelUp(t.build, levelUpChoicesFromProto(t.classKey, choices, hp), t.content)
	if err != nil {
		return levelUpPlan{}, levelUpRulesError(err)
	}
	plan.sheet = applyLevelUp(t.full, after, t.idx)
	checked, err := checkSheet(t.content, &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: plan.sheet}})
	if err != nil {
		return levelUpPlan{}, invalidArgument(err)
	}
	plan.sheet = checked.GetFull()
	// A table entry the master archived is not a new choice (RN-23).
	if _, field, found := newArchivedChoice(t.content, t.full, plan.sheet); found {
		plan.retiredChoice = true
		if plan.refusal == nil {
			plan.refusal = &charactersv1.LevelUpRefusal{
				Field: field, Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ARCHIVED_CHOICE,
			}
		}
	}
	// Nor an option the master switched off, for a player (RN-23).
	if _, field, found := newOffChoice(t.content, t.full, plan.sheet); found && !isMaster(m) {
		plan.retiredChoice = true
		if plan.refusal == nil {
			plan.refusal = &charactersv1.LevelUpRefusal{
				Field: field, Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWITCHED_OFF_CHOICE,
			}
		}
	}
	if plan.refusal == nil {
		if err := rules.CheckLevelUp(t.build, buildOf(plan.sheet), t.content); err != nil {
			le, ok := errors.AsType[*rules.LevelUpError](err)
			if !ok {
				return levelUpPlan{}, err
			}
			plan.refusal = refusalOf(le)
		}
	}
	return plan, nil
}

// levelUpChoicesFromProto turns the message into the rules' choices.
func levelUpChoicesFromProto(classKey string, c *charactersv1.LevelUpChoices, hp rules.LevelUpHitPoints) rules.LevelUpChoices {
	out := rules.LevelUpChoices{
		Class: classKey, Subclass: c.GetSubclassKey(),
		Cantrips: c.GetCantripKeys(), Spells: c.GetKnownSpellKeys(), Prepared: c.GetPreparedSpellKeys(),
		FeatureChoices: c.GetFeatureChoiceKeys(), SkillProficiencies: c.GetSkillProficiencyKeys(), Expertise: c.GetExpertiseSkillKeys(),
		HitPoints: hp,
	}
	for a, v := range abilityMap(c.GetAbilityIncrease()) {
		if v != 0 {
			if out.AbilityIncrease == nil {
				out.AbilityIncrease = map[rules.Ability]int{}
			}
			out.AbilityIncrease[a] = v
		}
	}
	return out
}

// applyLevelUp writes what a level-up changes of a Build onto a copy of the
// sheet: the class's level and subclass, the ability bonuses, the hit points
// and the lists. Nothing else of the sheet is touched, so everything the level
// does not change stays as it was stored.
func applyLevelUp(full *charactersv1.FullSheet, after rules.Build, idx int) *charactersv1.FullSheet {
	out := proto.CloneOf(full)
	cl := out.Classes[idx]
	cl.Level = i32(after.Classes[idx].Level)
	if sub := after.Classes[idx].Subclass; sub != "" {
		cl.Subclass = &charactersv1.ClassLevel_SubclassKey{SubclassKey: sub}
	}
	if !abilityMapsEqual(abilityMap(full.GetExtraAbilityBonuses()), after.ExtraAbilityBonuses) {
		out.ExtraAbilityBonuses = &rulesv1.AbilityScores{
			Strength: i32(after.ExtraAbilityBonuses[rules.STR]), Dexterity: i32(after.ExtraAbilityBonuses[rules.DEX]),
			Constitution: i32(after.ExtraAbilityBonuses[rules.CON]), Intelligence: i32(after.ExtraAbilityBonuses[rules.INT]),
			Wisdom: i32(after.ExtraAbilityBonuses[rules.WIS]), Charisma: i32(after.ExtraAbilityBonuses[rules.CHA]),
		}
	}
	if before := buildOf(full).HitPoints; before.Method != after.HitPoints.Method || !slices.Equal(before.Rolls, after.HitPoints.Rolls) {
		hp := &charactersv1.HitPoints{Method: charactersv1.HitPointsMethod_HIT_POINTS_METHOD_AVERAGE}
		if after.HitPoints.Method == rules.HitPointsRolled {
			hp.Method = charactersv1.HitPointsMethod_HIT_POINTS_METHOD_ROLLED
			for _, r := range after.HitPoints.Rolls {
				hp.Rolls = append(hp.Rolls, i32(r))
			}
		}
		out.HitPoints = hp
	}
	out.CantripKeys = slices.Clone(after.Cantrips)
	out.KnownSpellKeys = slices.Clone(after.SpellsKnown)
	out.PreparedSpellKeys = slices.Clone(after.SpellsPrepared)
	out.FeatureChoiceKeys = slices.Clone(after.FeatureChoices)
	out.SkillProficiencyKeys = slices.Clone(after.SkillProficiencies)
	out.ExpertiseSkillKeys = slices.Clone(after.Expertise)
	return out
}

// abilityMapsEqual says whether two ability maps hold the same numbers, a missing
// ability counting as 0.
func abilityMapsEqual(a, b map[rules.Ability]int) bool {
	for _, ab := range rules.AllAbilities() {
		if a[ab] != b[ab] {
			return false
		}
	}
	return true
}

// levelUpOptionsToProto copies rules.LevelUpOffer into the message.
func levelUpOptionsToProto(o rules.LevelUpOffer) *charactersv1.LevelUpOptions {
	named := func(keys []rules.NamedKey) []*charactersv1.LevelUpNamedKey {
		var out []*charactersv1.LevelUpNamedKey
		for _, k := range keys {
			out = append(out, &charactersv1.LevelUpNamedKey{Key: k.Key, NamePt: k.NamePT})
		}
		return out
	}
	choices := func(in []rules.LevelUpFeatureChoice) []*charactersv1.LevelUpFeatureChoice {
		var out []*charactersv1.LevelUpFeatureChoice
		for _, c := range in {
			out = append(out, &charactersv1.LevelUpFeatureChoice{
				Feature:     &charactersv1.LevelUpNamedKey{Key: c.Feature.Key, NamePt: c.Feature.NamePT},
				SubclassKey: c.Subclass, Choose: i32(c.Choose), Options: named(c.Options),
			})
		}
		return out
	}
	pact := func(p *rules.PactMagic) *rulesv1.PactMagic {
		if p == nil {
			return nil
		}
		return &rulesv1.PactMagic{SlotLevel: i32(p.SlotLevel), Count: i32(p.Slots)}
	}
	slots := func(s [9]int) []int32 {
		out := make([]int32, len(s))
		for i, n := range s {
			out[i] = i32(n)
		}
		return out
	}
	out := &charactersv1.LevelUpOptions{
		ClassKey: o.Class, ClassNamePt: o.ClassNamePT,
		FromLevel: i32(o.FromLevel), ToLevel: i32(o.ToLevel), TotalFromLevel: i32(o.TotalFrom), TotalToLevel: i32(o.TotalTo),
		HitDie: i32(o.HitDie), HitPointAverage: i32(o.HitPointAverage),
		AbilityScoreImprovement: o.AbilityScoreImprovement, SubclassDue: o.SubclassDue,
		Cantrips: i32(o.Cantrips), Spells: i32(o.Spells),
		SpellListClassKey: o.SpellList, MaxSpellLevel: i32(o.MaxSpellLevel),
		Prepares: o.Prepares, PreparedMax: i32(o.PreparedMax), PreparedMaxAfter: i32(o.PreparedMaxAfter),
		FeatureChoices: choices(o.FeatureChoices), SkillChoices: i32(o.SkillChoices), ExpertiseChoices: i32(o.ExpertiseChoices),
		ProficiencyBonusBefore: i32(o.ProficiencyBonusBefore), ProficiencyBonusAfter: i32(o.ProficiencyBonusAfter),
		SpellSlotsBefore: slots(o.SlotsBefore), SpellSlotsAfter: slots(o.SlotsAfter),
		PactMagicBefore: pact(o.PactBefore), PactMagicAfter: pact(o.PactAfter),
		NewFeatures: named(o.NewFeatures), MasterAdds: named(o.MasterAdds), AnyClassSpells: i32(o.AnyClassSpells),
	}
	out.SpellsKind = spellsKindOf(o.SpellsKind)
	for _, sub := range o.Subclasses {
		out.Subclasses = append(out.Subclasses, &charactersv1.LevelUpSubclass{
			Key: sub.Key, NamePt: sub.NamePT, FeatureChoices: choices(sub.FeatureChoices),
			Cantrips: i32(sub.Cantrips), SkillChoices: i32(sub.SkillChoices), ExpertiseChoices: i32(sub.ExpertiseChoices),
			Archived: sub.Archived, Off: sub.Off,
			Spells: i32(sub.Spells), SpellsKind: spellsKindOf(sub.SpellsKind), SpellListClassKey: sub.SpellList, MaxSpellLevel: i32(sub.MaxSpellLevel),
			Prepares: sub.Prepares, PreparedMaxAfter: i32(sub.PreparedMaxAfter),
		})
	}
	return out
}

// hitPointsMethodAllowed says whether the table's rule allows the method (RN-24):
// with "roll" only the rolled methods, with "average" only the average, and with
// the default every method.
func hitPointsMethodAllowed(rule HitPointsRule, m charactersv1.LevelUpHitPointsMethod) bool {
	average := m == charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE
	switch rule {
	case HitPointsRoll:
		return !average
	case HitPointsAverage:
		return average
	}
	return true
}

// hitPointsRuleToProto is the rule GetLevelUpOptions tells the app.
func hitPointsRuleToProto(rule HitPointsRule) charactersv1.LevelUpHitPointsRule {
	switch rule {
	case HitPointsRoll:
		return charactersv1.LevelUpHitPointsRule_LEVEL_UP_HIT_POINTS_RULE_ROLL_ONLY
	case HitPointsAverage:
		return charactersv1.LevelUpHitPointsRule_LEVEL_UP_HIT_POINTS_RULE_AVERAGE_ONLY
	}
	return charactersv1.LevelUpHitPointsRule_LEVEL_UP_HIT_POINTS_RULE_PLAYER_CHOOSES
}

// spellsKindOf is where the new spells of a level go: the wizard's spellbook or a
// known caster's list; unspecified for a class that adds none.
func spellsKindOf(kind string) charactersv1.LevelUpSpellsKind {
	switch kind {
	case rules.PreparationSpellbook:
		return charactersv1.LevelUpSpellsKind_LEVEL_UP_SPELLS_KIND_SPELLBOOK
	case rules.PreparationKnown:
		return charactersv1.LevelUpSpellsKind_LEVEL_UP_SPELLS_KIND_KNOWN
	}
	return charactersv1.LevelUpSpellsKind_LEVEL_UP_SPELLS_KIND_UNSPECIFIED
}
