package characters

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"maps"
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
		rules.LevelUpReasonClass:                         charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_CLASS,
		rules.LevelUpReasonMaxLevel:                      charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_MAX_LEVEL,
		rules.LevelUpReasonLocked:                        charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_LOCKED_FIELD,
		rules.LevelUpReasonAbilityNotDue:                 charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ABILITY_NOT_DUE,
		rules.LevelUpReasonAbilityShape:                  charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ABILITY_SHAPE,
		rules.LevelUpReasonAbilityAbove20:                charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ABILITY_ABOVE_20,
		rules.LevelUpReasonHitPoints:                     charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINTS,
		rules.LevelUpReasonSubclass:                      charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SUBCLASS,
		rules.LevelUpReasonCantrips:                      charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_CANTRIPS,
		rules.LevelUpReasonSpells:                        charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SPELLS,
		rules.LevelUpReasonPrepared:                      charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_PREPARED,
		rules.LevelUpReasonFeatureChoice:                 charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEATURE_CHOICE,
		rules.LevelUpReasonSkills:                        charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SKILLS,
		rules.LevelUpReasonExpertise:                     charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_EXPERTISE,
		rules.LevelUpReasonSheetIssue:                    charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SHEET_ISSUE,
		rules.LevelUpReasonFeat:                          charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEAT,
		rules.LevelUpReasonFeatPrerequisite:              charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEAT_PREREQUISITE,
		rules.LevelUpReasonLateChoice:                    charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_LATE_CHOICE_MISSING,
		rules.LevelUpReasonMulticlassPrerequisite:        charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_MULTICLASS_PREREQUISITE,
		rules.LevelUpReasonMulticlassPrerequisiteCurrent: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_MULTICLASS_PREREQUISITE_CURRENT,
		rules.LevelUpReasonProficiencyChoice:             charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_PROFICIENCY_CHOICE,
		rules.LevelUpReasonInstrumentChoice:              charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_INSTRUMENT_CHOICE,
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
	r := &charactersv1.LevelUpRefusal{Field: e.Field, Reason: refusalReasons[e.Reason], IssueCode: e.Code}
	if e.ClassKey != "" {
		r.ClassKey, r.Ability, r.Minimum, r.Have = e.ClassKey, abilityToProto[e.Ability], i32(e.Minimum), i32(e.Have)
	}
	return r
}

// levelUpTarget is the character a guided level-up call is about, checked
// before anything else: the caller may see it, it is a living player
// character, and it can level up now. owner is true for the owning player.
type levelUpTarget struct {
	row   charactersdb.Character
	full  *charactersv1.FullSheet
	build rules.Build
	// classKey is the class that gains the level; idx its place in the sheet. A
	// class the character does not have (multiclassing) has idx past the last
	// class: it joins the sheet at level 1.
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
	if err := levelUpStatusBlocked(row); err != nil {
		return levelUpTarget{}, err
	}
	full, err := s.levelUpSheet(row)
	if err != nil {
		return levelUpTarget{}, err
	}
	content, tableRules, err := s.content.For(ctx, tx, m.CampaignID)
	if err != nil {
		return levelUpTarget{}, wrap("read rules content", err)
	}
	if err := s.checkLevelUpDue(ctx, tx, row, full, content); err != nil {
		return levelUpTarget{}, err
	}
	build := buildOf(full)
	if err := sheetNeedsMaster(build, content); err != nil {
		return levelUpTarget{}, err
	}
	classKey, idx, err := levelUpClassOf(content, m, build, classKey)
	if err != nil {
		return levelUpTarget{}, err
	}
	return levelUpTarget{row: row, full: full, build: build, classKey: classKey, idx: idx, content: content, rules: tableRules}, nil
}

// levelUpStatusBlocked is the CharacterBlocked refusal of a dead or pending
// character, nil for any other.
func levelUpStatusBlocked(row charactersdb.Character) error {
	switch row.Status {
	case statusDead:
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD, row.ID)
	case statusPending:
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_AWAITING_APPROVAL, row.ID)
	}
	return nil
}

// errCannotLevelUp is the refusal of a character that has no level to take now.
func errCannotLevelUp(characterID string) error {
	return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CANNOT_LEVEL_UP, characterID)
}

// levelUpSheet is the stored full sheet of a player character, the only kind
// the guided level-up takes. Without the progression module's rule nothing can
// level up.
func (s *Service) levelUpSheet(row charactersdb.Character) (*charactersv1.FullSheet, error) {
	sheet, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return nil, err
	}
	full := sheet.GetFull()
	if row.Kind != kindPlayer || full == nil || s.levelUps == nil {
		return nil, errCannotLevelUp(row.ID)
	}
	return full, nil
}

// checkLevelUpDue asks the progression module whether the character has a
// level to take now (RN-12).
func (s *Service) checkLevelUpDue(ctx context.Context, tx pgx.Tx, row charactersdb.Character, full *charactersv1.FullSheet, content *rules.Content) error {
	d := rules.Derive(buildOf(full), content)
	reason, err := s.levelUps.LevelUpReason(ctx, tx, deref(row.CampaignID), row.ID, i32(d.TotalLevel), full.GetExperiencePoints(), i32(d.NextLevelXP))
	if err != nil {
		return wrap("check the level up", err)
	}
	if reason == charactersv1.LevelUpReason_LEVEL_UP_REASON_UNSPECIFIED {
		return errCannotLevelUp(row.ID)
	}
	return nil
}

// sheetNeedsMaster refuses a sheet that does not pass today's rules by itself:
// the level's choices cannot fix it, it is the master's to correct.
func sheetNeedsMaster(build rules.Build, content *rules.Content) error {
	err := rules.Validate(build, content)
	if err == nil {
		return nil
	}
	field := ""
	if ve, ok := errors.AsType[*rules.ValidationError](err); ok {
		field = ve.Field
	}
	return errRefused(&charactersv1.LevelUpRefusal{
		Field: field, Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SHEET_NEEDS_MASTER,
	})
}

// levelUpClassOf resolves the class that gains the level (empty: the first
// one) and its place in the sheet: past the last class for a class the
// character does not have yet.
func levelUpClassOf(content *rules.Content, m authz.Membership, build rules.Build, classKey string) (string, int, error) {
	if classKey == "" && len(build.Classes) > 0 {
		classKey = build.Classes[0].Class
	}
	if idx := slices.IndexFunc(build.Classes, func(c rules.ClassLevel) bool { return c.Class == classKey }); idx >= 0 {
		return classKey, idx, nil
	}
	// A class of the content the character does not have is multiclassing: the
	// rules judge the prerequisites. The table's retired and switched-off classes
	// are not a new choice (RN-23); the master's switch does not bind the master.
	if !isClassKey(content, classKey) {
		return "", 0, invalidArgument(fieldErr("class_key", "is not a class"))
	}
	if reason, key := newClassRefusal(content, m, classKey); reason != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_UNSPECIFIED {
		return "", 0, errRefused(&charactersv1.LevelUpRefusal{Field: "class_key", Reason: reason, ClassKey: key})
	}
	return classKey, len(build.Classes), nil
}

// isClassKey says the content has a class with this key.
func isClassKey(content *rules.Content, key string) bool {
	return slices.ContainsFunc(content.Catalog().Classes, func(c rules.ClassEntry) bool { return c.Key == key })
}

// newClassRefusal says why the caller may not take a class as a new class
// because of the table's switches (RN-23): the master retired it, or switched it
// off for the players (the master is not bound by the switch). The zero reason
// says none.
func newClassRefusal(content *rules.Content, m authz.Membership, classKey string) (charactersv1.LevelUpRefusalReason, string) {
	switch {
	case content.Archived(classKey):
		return charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ARCHIVED_CHOICE, classKey
	case !isMaster(m) && content.Hidden(classKey):
		return charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWITCHED_OFF_CHOICE, classKey
	}
	return charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_UNSPECIFIED, ""
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
	var read levelUpOptionsRead
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		read, err = s.readLevelUpOptions(ctx, tx, m, req.Msg)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "read the level up options", err)
	}
	return connect.NewResponse(&charactersv1.GetLevelUpOptionsResponse{Options: levelUpOptionsFor(m, read)}), nil
}

// levelUpOptionsRead is what GetLevelUpOptions reads in its transaction.
type levelUpOptionsRead struct {
	t        levelUpTarget
	offer    rules.LevelUpOffer
	diceRule charactersv1.LevelUpDiceRule
	// roll is the hit die kept for the level, when rollFound.
	roll      charactersdb.GetLevelUpRollRow
	rollFound bool
}

// readLevelUpOptions reads the character, the level's offer, the dice setting
// and the kept roll, through tx.
func (s *Service) readLevelUpOptions(ctx context.Context, tx pgx.Tx, m authz.Membership, req *charactersv1.GetLevelUpOptionsRequest) (levelUpOptionsRead, error) {
	var (
		r   levelUpOptionsRead
		err error
	)
	if r.t, err = s.readLevelUpTarget(ctx, tx, m, req.GetCharacterId(), req.GetClassKey()); err != nil {
		return r, err
	}
	if r.offer, err = rules.LevelUpOptions(r.t.build, r.t.classKey, r.t.content); err != nil {
		return r, levelUpRulesError(err)
	}
	if r.diceRule, err = s.diceRule(ctx, tx, m); err != nil {
		return r, err
	}
	r.roll, err = s.queries.WithTx(tx).GetLevelUpRoll(ctx, charactersdb.GetLevelUpRollParams{CharacterID: r.t.row.ID, ToLevel: i32(r.offer.TotalTo)})
	r.rollFound = err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return r, wrap("read the kept roll", err)
	}
	return r, nil
}

// levelUpOptionsFor is the options message the caller reads.
func levelUpOptionsFor(m authz.Membership, r levelUpOptionsRead) *charactersv1.LevelUpOptions {
	t := r.t
	out := levelUpOptionsToProto(r.offer)
	out.ClassChoices = classChoicesToProto(t.content, m, rules.LevelUpClasses(t.build, t.content))
	out.Feats = featOptionsToProto(r.offer.Feats, t.rules.FeatsAllowed, isMaster(m))
	if !isMaster(m) {
		hideFromPlayer(out, t)
	}
	out.DiceRule = r.diceRule
	out.HitPointsRule = hitPointsRuleToProto(t.rules.HitPoints)
	if r.rollFound {
		out.KeptHitPointRoll, out.KeptHitPointRollClassKey = r.roll.Value, r.roll.ClassKey
	}
	return out
}

// hideFromPlayer takes out of a player's options what the master retired or
// switched off (RN-23), unless the character's own sheet already uses it.
func hideFromPlayer(out *charactersv1.LevelUpOptions, t levelUpTarget) {
	// A subclass the master archived or switched off is not offered.
	out.Subclasses = slices.DeleteFunc(out.Subclasses, func(sub *charactersv1.LevelUpSubclass) bool {
		return (sub.GetArchived() || sub.GetOff()) && !slices.ContainsFunc(t.build.Classes, func(c rules.ClassLevel) bool { return c.Subclass == sub.GetKey() })
	})
	// Nor a reference to a hidden class the sheet does not use (a list a table
	// class or subclass reuses).
	if hiddenListOf(t, out.GetSpellListClassKey()) {
		out.SpellListClassKey = ""
	}
	// Nor the name of a hidden race or subrace a feat asks for.
	for _, f := range out.Feats {
		hideFeatRace(f, t)
	}
	for _, sub := range out.Subclasses {
		if hiddenListOf(t, sub.GetSpellListClassKey()) {
			sub.SpellListClassKey = ""
		}
	}
}

// hiddenListOf says k is a hidden class the character's sheet does not have.
func hiddenListOf(t levelUpTarget, k string) bool {
	return k != "" && t.content.Hidden(k) && !slices.ContainsFunc(t.build.Classes, func(c rules.ClassLevel) bool { return c.Class == k })
}

// hiddenRaceOf says k is a hidden race or subrace that is not the character's.
func hiddenRaceOf(t levelUpTarget, k string) bool {
	return k != "" && t.content.Hidden(k) && t.build.Race != k && t.build.Subrace != k
}

// hideFeatRace blanks the hidden race a feat asks for, in its prerequisite and
// in what the character does not meet.
func hideFeatRace(f *rulesv1.FeatOption, t levelUpTarget) {
	if hiddenRaceOf(t, f.GetPrerequisite().GetRaceKey()) {
		f.Prerequisite.RaceKey = ""
	}
	for _, u := range f.GetUnmet() {
		if hiddenRaceOf(t, u.GetKey()) && u.GetKind() == rulesv1.FeatUnmetKind_FEAT_UNMET_KIND_RACE {
			u.Key = ""
		}
	}
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
	res := &charactersv1.PreviewLevelUpResponse{
		After:   derivedToProto(rules.Derive(buildOf(plan.sheet), t.content)),
		Refusal: plan.refusal,
	}
	newClass := ""
	if t.idx == len(t.build.Classes) {
		newClass = t.classKey
	}
	if sum, ok := rules.MulticlassSummaryOf(t.build, buildOf(plan.sheet), newClass, t.content); ok {
		res.MulticlassSummary = multiclassSummaryToProto(sum, rules.Derive(buildOf(plan.sheet), t.content))
	}
	return connect.NewResponse(res), nil
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
	id, err := checkRollRequest(m, req.Msg)
	if err != nil {
		return nil, err
	}
	var res *charactersv1.RollLevelUpHitPointsResponse
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		res, err = s.rollLevelUpIn(ctx, tx, m, id, req.Msg.GetClassKey())
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll the level up hit points", err)
	}
	return connect.NewResponse(res), nil
}

// errOnlyThePlayer refuses the master a call of the guided level-up: the master
// keeps the sheet editor, the guided level-up is the player's.
func errOnlyThePlayer() error {
	return errPermission("only the owning player levels up a character with the guided level-up")
}

// checkRollRequest checks who calls and the request's IDs, and returns the
// character's ID. The idempotency key is only checked: the kept roll is what
// makes a second call answer the same.
func checkRollRequest(m authz.Membership, req *charactersv1.RollLevelUpHitPointsRequest) (string, error) {
	if isMaster(m) {
		return "", errOnlyThePlayer()
	}
	id, ok := parseUUID(req.GetCharacterId())
	if !ok {
		return "", errCharacterNotFound()
	}
	if _, ok := parseUUID(req.GetIdempotencyKey()); !ok {
		return "", invalidArgument(fieldErr("idempotency_key", "must be a UUID"))
	}
	return id, nil
}

// rollLevelUpIn rolls the hit die of the character's next level in tx, or
// answers the roll already kept for it.
func (s *Service) rollLevelUpIn(ctx context.Context, tx pgx.Tx, m authz.Membership, id, classKey string) (*charactersv1.RollLevelUpHitPointsResponse, error) {
	q := s.queries.WithTx(tx)
	row, err := visibleForUpdate(ctx, q, m, id) // locked: two rolls of the same level wait for each other
	if err != nil {
		return nil, err
	}
	t, err := s.newLevelUpTarget(ctx, tx, m, row, classKey)
	if err != nil {
		return nil, err
	}
	if err := s.checkRollAllowed(ctx, tx, m, t); err != nil {
		return nil, err
	}
	offer, lerr := rules.LevelUpOptions(t.build, t.classKey, t.content)
	if lerr != nil {
		return nil, levelUpRulesError(lerr)
	}
	key := charactersdb.GetLevelUpRollParams{CharacterID: row.ID, ToLevel: i32(offer.TotalTo)}
	kept, err := q.GetLevelUpRoll(ctx, key)
	if err == nil {
		return keptRollAnswer(kept, t.classKey)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, wrap("read the kept roll", err)
	}
	return s.keepNewRoll(ctx, q, t, key, offer.HitDie)
}

// checkRollAllowed refuses a roll in the app that the campaign's dice setting
// or the table's rule does not allow.
func (s *Service) checkRollAllowed(ctx context.Context, tx pgx.Tx, m authz.Membership, t levelUpTarget) error {
	rule, err := s.diceRule(ctx, tx, m)
	if err != nil {
		return err
	}
	if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_PHYSICAL, t.row.ID)
	}
	// A table where everybody takes the average never rolls (RN-24).
	if t.rules.HitPoints == HitPointsAverage {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_HIT_POINTS_AVERAGE_ONLY, t.row.ID)
	}
	return nil
}

// keptRollAnswer answers the roll kept for the level: one roll per level, for
// the class that rolled it.
func keptRollAnswer(kept charactersdb.GetLevelUpRollRow, classKey string) (*charactersv1.RollLevelUpHitPointsResponse, error) {
	if kept.ClassKey != classKey {
		return nil, errRefused(&charactersv1.LevelUpRefusal{
			Field: "class_key", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINT_ROLL_OTHER_CLASS,
		})
	}
	return &charactersv1.RollLevelUpHitPointsResponse{Die: kept.Die, Value: kept.Value, AlreadyRolled: true}, nil
}

// keepNewRoll rolls the hit die and keeps the result for the level.
func (s *Service) keepNewRoll(ctx context.Context, q *charactersdb.Queries, t levelUpTarget, key charactersdb.GetLevelUpRollParams, hitDie int) (*charactersv1.RollLevelUpHitPointsResponse, error) {
	rolled, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: hitDie})
	if err != nil {
		return nil, wrap("roll the hit die", err)
	}
	if _, err := q.InsertLevelUpRoll(ctx, charactersdb.InsertLevelUpRollParams{
		CharacterID: t.row.ID, ClassKey: t.classKey, ToLevel: key.ToLevel, Die: i32(hitDie), Value: i32(rolled.Total), Now: s.now(),
	}); err != nil {
		return nil, wrap("keep the roll", err)
	}
	return &charactersv1.RollLevelUpHitPointsResponse{Die: i32(hitDie), Value: i32(rolled.Total), AlreadyRolled: false}, nil
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
	id, err := checkLevelUpRequest(m, req.Msg)
	if err != nil {
		return nil, err
	}
	var done leveledUp
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		done, err = s.levelUpIn(ctx, tx, m, id, req.Msg.GetRevision(), req.Msg.GetChoices())
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "level up a character", err)
	}
	s.announceLevelUp(ctx, m.CampaignID, id)
	// The response is derived from the content the level-up was checked with.
	c, err := s.character(ctx, done.content, done.row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a leveled up character", err)
	}
	return connect.NewResponse(&charactersv1.LevelUpCharacterResponse{Character: c}), nil
}

// leveledUp is what a committed level-up leaves: the character's new row and the
// content the level-up was checked with.
type leveledUp struct {
	row     charactersdb.Character
	content *rules.Content
}

// checkLevelUpRequest checks who calls and the request's shape, before anything
// is read, and returns the character's ID.
func checkLevelUpRequest(m authz.Membership, req *charactersv1.LevelUpCharacterRequest) (string, error) {
	if isMaster(m) {
		return "", errOnlyThePlayer()
	}
	id, ok := parseUUID(req.GetCharacterId())
	if !ok {
		return "", errCharacterNotFound()
	}
	if req.GetRevision() < 1 {
		return "", invalidArgument(fieldErr("revision", "must be at least 1"))
	}
	choices := req.GetChoices()
	if err := checkLevelUpChoices(choices); err != nil {
		return "", invalidArgument(err)
	}
	if _, ok := hpMethodToDB[choices.GetHitPoints().GetMethod()]; !ok {
		return "", invalidArgument(fieldErr("choices.hit_points.method", "is required"))
	}
	return id, nil
}

// levelUpIn checks and plans the level-up on the locked row, then writes it, in tx.
func (s *Service) levelUpIn(ctx context.Context, tx pgx.Tx, m authz.Membership, id string, revision int32, choices *charactersv1.LevelUpChoices) (leveledUp, error) {
	q := s.queries.WithTx(tx)
	current, err := visibleForUpdate(ctx, q, m, id)
	if err != nil {
		return leveledUp{}, err
	}
	// The checks that stand in for the sheet's lock come in this order:
	// who the character is (dead, pending), then the revision (a retry of
	// a level-up that already went through is stale, not "cannot"), then
	// whether it can level up now.
	if err := levelUpStatusBlocked(current); err != nil {
		return leveledUp{}, err
	}
	if current.Revision != revision {
		return leveledUp{}, errStaleRevision()
	}
	t, err := s.newLevelUpTarget(ctx, tx, m, current, choices.GetClassKey())
	if err != nil {
		return leveledUp{}, err
	}
	plan, err := s.planLevelUpIn(ctx, tx, q, m, t, choices)
	if err != nil {
		return leveledUp{}, err
	}
	if plan.refusal != nil {
		return leveledUp{}, errRefused(plan.refusal)
	}
	// A level-up never clears what a change of the table's content flagged.
	carryFlags(t.content, t.full, plan.sheet)
	row, err := s.writeLevelUp(ctx, q, m, current, revision, t, plan)
	if err != nil {
		return leveledUp{}, err
	}
	return leveledUp{row: row, content: t.content}, nil
}

// writeLevelUp saves the new sheet, moves the current hit points with the
// maximum, records the level-up for the master and deletes the used roll.
func (s *Service) writeLevelUp(ctx context.Context, q *charactersdb.Queries, m authz.Membership, current charactersdb.Character, revision int32, t levelUpTarget, plan levelUpPlan) (charactersdb.Character, error) {
	id := current.ID
	doc, err := storeJSON.Marshal(&charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: plan.sheet}})
	if err != nil {
		return charactersdb.Character{}, wrap("encode a sheet", err)
	}
	row, err := q.UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
		CampaignID: m.CampaignID, ID: id, Revision: revision, Name: current.Name, Sheet: doc, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return charactersdb.Character{}, errStaleRevision()
	}
	if err != nil {
		return charactersdb.Character{}, wrap("update sheet", err)
	}
	if err := s.carryHitPoints(ctx, q, t.content, id, current.Sheet, doc); err != nil {
		return charactersdb.Character{}, err
	}
	if err := s.recordLevelUp(ctx, q, m, id, t.classKey, plan); err != nil {
		return charactersdb.Character{}, err
	}
	return row, nil
}

// recordLevelUp keeps what ListLevelUps shows and deletes the used roll: the
// next level rolls again.
func (s *Service) recordLevelUp(ctx context.Context, q *charactersdb.Queries, m authz.Membership, id, classKey string, plan levelUpPlan) error {
	record, err := storeJSON.Marshal(plan.record)
	if err != nil {
		return wrap("encode the level up", err)
	}
	if _, err := q.InsertLevelUp(ctx, charactersdb.InsertLevelUpParams{
		CampaignID: m.CampaignID, CharacterID: id, ClassKey: classKey,
		FromLevel: i32(plan.toLevel - 1), ToLevel: i32(plan.toLevel),
		HpMethod: hpMethodToDB[plan.record.GetHitPoints().GetMethod()], HpValue: plan.record.GetHitPoints().GetValue(),
		Choices: record, CreatedAt: s.now(),
	}); err != nil {
		return wrap("record the level up", err)
	}
	if err := q.DeleteLevelUpRoll(ctx, charactersdb.DeleteLevelUpRollParams{CharacterID: id, ToLevel: i32(plan.toLevel)}); err != nil {
		return wrap("delete the used roll", err)
	}
	return nil
}

// announceLevelUp logs the committed level-up and tells the live session.
func (s *Service) announceLevelUp(ctx context.Context, campaignID, id string) {
	logging.Event(ctx, s.logger, "levelup.done", slog.String("character_id", id))
	if s.live != nil {
		s.live.PublishXPChanged(campaignID)
		s.live.PublishVitalsChanged(ctx, campaignID, id)
	}
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
	params, err := levelUpsParams(m, req.Msg)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListLevelUps(ctx, params)
	if err != nil {
		return nil, s.dbError(ctx, "list level ups", err)
	}
	res := &charactersv1.ListLevelUpsResponse{}
	rows, res.NextPageToken = levelUpsPage(rows, params.MaxRows-1)
	displayNames, err := s.displayNames(ctx, levelUpPlayerIDs(rows))
	if err != nil {
		return nil, s.dbError(ctx, "read display names", err)
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	for _, row := range rows {
		l, err := levelUpToProto(content, displayNames, row)
		if err != nil {
			return nil, s.dbError(ctx, "list level ups", err)
		}
		res.LevelUps = append(res.LevelUps, l)
	}
	return connect.NewResponse(res), nil
}

// levelUpsParams turns the request into the query's parameters: the page size,
// the cursor and the character.
func levelUpsParams(m authz.Membership, req *charactersv1.ListLevelUpsRequest) (charactersdb.ListLevelUpsParams, error) {
	size := req.GetPageSize()
	switch {
	case size == 0:
		size = defaultLevelUpsPage
	case size < 0 || size > maxLevelUpsListed:
		return charactersdb.ListLevelUpsParams{}, invalidArgument(fieldErr("page_size", "must be 1 to %d", maxLevelUpsListed))
	}
	params := charactersdb.ListLevelUpsParams{CampaignID: m.CampaignID, MaxRows: size + 1} // one more tells whether there is a next page
	if token := req.GetPageToken(); token != "" {
		at, id, ok := parseLevelUpsToken(token)
		if !ok {
			return charactersdb.ListLevelUpsParams{}, invalidArgument(fieldErr("page_token", "is not a token of this list"))
		}
		params.BeforeCreatedAt, params.BeforeID = &at, &id
	}
	if raw := req.GetCharacterId(); raw != "" {
		id, ok := parseUUID(raw)
		if !ok {
			return charactersdb.ListLevelUpsParams{}, errCharacterNotFound()
		}
		params.CharacterID = &id
	}
	return params, nil
}

// levelUpsPage cuts the rows to the page and returns the next page's token, empty
// on the last page.
func levelUpsPage(rows []charactersdb.ListLevelUpsRow, size int32) ([]charactersdb.ListLevelUpsRow, string) {
	if int32(len(rows)) <= size { //nolint:gosec // at most maxLevelUpsListed+1
		return rows, ""
	}
	rows = rows[:size]
	last := rows[len(rows)-1]
	return rows, levelUpsToken(last.CreatedAt, last.ID)
}

// levelUpPlayerIDs is the players of the rows' characters.
func levelUpPlayerIDs(rows []charactersdb.ListLevelUpsRow) []string {
	var playerIDs []string
	for _, row := range rows {
		if row.PlayerUserID != nil {
			playerIDs = append(playerIDs, *row.PlayerUserID)
		}
	}
	return playerIDs
}

// levelUpToProto is one row of the list, with the names of what it chose.
func levelUpToProto(content *rules.Content, displayNames map[string]string, row charactersdb.ListLevelUpsRow) (*charactersv1.LevelUp, error) {
	choices := &charactersv1.LevelUpChoices{}
	if err := loadJSON.Unmarshal(row.Choices, choices); err != nil {
		return nil, errors.Join(errCorruptDocument, err)
	}
	return &charactersv1.LevelUp{
		Id: row.ID, CharacterId: row.CharacterID, CharacterName: row.CharacterName,
		PlayerDisplayName: displayNames[deref(row.PlayerUserID)],
		ClassKey:          row.ClassKey, ClassNamePt: content.NamePT(row.ClassKey),
		FromLevel: row.FromLevel, ToLevel: row.ToLevel,
		Choices: choices, NamesPt: levelUpNames(content, row.ClassKey, choices),
		CreatedAt: timestamppb.New(row.CreatedAt),
	}, nil
}

// levelUpNames names every content key a level-up's choices hold.
func levelUpNames(content *rules.Content, classKey string, c *charactersv1.LevelUpChoices) map[string]string {
	names := map[string]string{}
	keys := slices.Concat([]string{classKey, c.GetSubclassKey(), c.GetFeatKey()}, c.GetCantripKeys(), c.GetKnownSpellKeys(),
		c.GetPreparedSpellKeys(), c.GetFeatureChoiceKeys(), c.GetSkillProficiencyKeys(), c.GetExpertiseSkillKeys())
	for _, k := range keys {
		if name := content.NamePT(k); k != "" && name != "" {
			names[k] = name
		}
	}
	if k := c.GetInstrumentKey(); k != "" {
		if name := content.ProficiencyNamePT(k); name != "" {
			names[k] = name
		}
	}
	return names
}

// levelUpChoicesFromProto turns the message into the rules' choices.
func levelUpChoicesFromProto(classKey string, c *charactersv1.LevelUpChoices, hp rules.LevelUpHitPoints) rules.LevelUpChoices {
	out := rules.LevelUpChoices{
		Class: classKey, Subclass: c.GetSubclassKey(),
		Cantrips: c.GetCantripKeys(), Spells: c.GetKnownSpellKeys(), Prepared: c.GetPreparedSpellKeys(),
		FeatureChoices: c.GetFeatureChoiceKeys(), SkillProficiencies: c.GetSkillProficiencyKeys(), Expertise: c.GetExpertiseSkillKeys(),
		LateChoices: c.GetLateChoiceKeys(), FeatureChoiceText: c.GetFeatureChoiceText(), SwapInvocation: c.GetSwapInvocationKey(),
		Feat: c.GetFeatKey(), HitPoints: hp, Instrument: c.GetInstrumentKey(),
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
	if idx == len(out.Classes) {
		// A class the character did not have joins the sheet, last.
		out.Classes = append(out.Classes, &charactersv1.ClassLevel{ClassKey: after.Classes[idx].Class})
	}
	cl := out.Classes[idx]
	cl.Level = i32(after.Classes[idx].Level)
	if sub := after.Classes[idx].Subclass; sub != "" {
		cl.Subclass = &charactersv1.ClassLevel_SubclassKey{SubclassKey: sub}
	}
	out.ToolProficiencies = slices.Clone(after.ToolProficiencies)
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
	out.FeatureChoiceText = maps.Clone(after.FeatureChoiceText)
	out.FeatKeys = slices.Clone(after.Feats)
	out.FeatSlots = maps.Clone(after.FeatSlots)
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
	out := &charactersv1.LevelUpOptions{
		ClassKey: o.Class, ClassNamePt: o.ClassNamePT,
		FromLevel: i32(o.FromLevel), ToLevel: i32(o.ToLevel), TotalFromLevel: i32(o.TotalFrom), TotalToLevel: i32(o.TotalTo),
		HitDie: i32(o.HitDie), HitPointAverage: i32(o.HitPointAverage),
		AbilityScoreImprovement: o.AbilityScoreImprovement, SubclassDue: o.SubclassDue,
		Cantrips: i32(o.Cantrips), Spells: i32(o.Spells),
		SpellListClassKey: o.SpellList, MaxSpellLevel: i32(o.MaxSpellLevel),
		Prepares: o.Prepares, PreparedMax: i32(o.PreparedMax), PreparedMaxAfter: i32(o.PreparedMaxAfter),
		FeatureChoices: levelUpFeatureChoicesToProto(o.FeatureChoices), SkillChoices: i32(o.SkillChoices), ExpertiseChoices: i32(o.ExpertiseChoices),
		ProficiencyBonusBefore: i32(o.ProficiencyBonusBefore), ProficiencyBonusAfter: i32(o.ProficiencyBonusAfter),
		SpellSlotsBefore: levelUpSlotsToProto(o.SlotsBefore), SpellSlotsAfter: levelUpSlotsToProto(o.SlotsAfter),
		PactMagicBefore: pactMagicToProto(o.PactBefore), PactMagicAfter: pactMagicToProto(o.PactAfter),
		NewFeatures: levelUpNamedKeysToProto(o.NewFeatures), MasterAdds: levelUpNamedKeysToProto(o.MasterAdds), AnyClassSpells: i32(o.AnyClassSpells),
		LateChoices: choiceGroupsToProto(o.LateChoices), NewChoices: choiceGroupsToProto(o.NewChoices),
		CanSwapInvocation: o.CanSwapInvocation,
	}
	out.SpellsKind = spellsKindOf(o.SpellsKind)
	out.IsNewClass = o.NewClass
	if mc := o.Multiclass; mc != nil {
		out.ProficiencyGains, out.ProficiencyChoices = multiclassOfferToProto(mc)
	}
	out.Subclasses = levelUpSubclassesToProto(o.Subclasses)
	return out
}

// levelUpNamedKeysToProto copies named keys into the message; none gives nil.
func levelUpNamedKeysToProto(keys []rules.NamedKey) []*charactersv1.LevelUpNamedKey {
	var out []*charactersv1.LevelUpNamedKey
	for _, k := range keys {
		out = append(out, &charactersv1.LevelUpNamedKey{Key: k.Key, NamePt: k.NamePT})
	}
	return out
}

// levelUpFeatureChoicesToProto copies the features' choices, with the options
// they block; none gives nil.
func levelUpFeatureChoicesToProto(in []rules.LevelUpFeatureChoice) []*charactersv1.LevelUpFeatureChoice {
	var out []*charactersv1.LevelUpFeatureChoice
	for _, c := range in {
		fc := &charactersv1.LevelUpFeatureChoice{
			Feature:     &charactersv1.LevelUpNamedKey{Key: c.Feature.Key, NamePt: c.Feature.NamePT},
			SubclassKey: c.Subclass, Choose: i32(c.Choose), Options: levelUpNamedKeysToProto(c.Options),
		}
		for _, b := range c.Blocked {
			fc.Blocked = append(fc.Blocked, &charactersv1.LevelUpBlockedOption{
				Option: &charactersv1.LevelUpNamedKey{Key: b.Key, NamePt: b.NamePT}, ReasonPt: b.ReasonPT,
			})
		}
		out = append(out, fc)
	}
	return out
}

// pactMagicToProto copies the pact slots; nil for a character without them.
func pactMagicToProto(p *rules.PactMagic) *rulesv1.PactMagic {
	if p == nil {
		return nil
	}
	return &rulesv1.PactMagic{SlotLevel: i32(p.SlotLevel), Count: i32(p.Slots)}
}

// levelUpSlotsToProto copies the slots per circle, all nine of them.
func levelUpSlotsToProto(s [9]int) []int32 {
	out := make([]int32, len(s))
	for i, n := range s {
		out[i] = i32(n)
	}
	return out
}

// multiclassOfferToProto copies what a new class brings: the proficiencies it
// gains, and the skills and instruments to pick, skills first.
func multiclassOfferToProto(mc *rules.MulticlassOffer) ([]*charactersv1.LevelUpProficiencyGain, []*charactersv1.LevelUpProficiencyChoice) {
	var gains []*charactersv1.LevelUpProficiencyGain
	for _, p := range mc.Proficiencies {
		gains = append(gains, &charactersv1.LevelUpProficiencyGain{Key: p.Key, NamePt: p.NamePT, Kind: p.Kind, AlreadyHave: p.AlreadyHave})
	}
	var picks []*charactersv1.LevelUpProficiencyChoice
	for _, p := range []struct {
		kind charactersv1.LevelUpProficiencyKind
		pick *rules.MulticlassPick
	}{
		{charactersv1.LevelUpProficiencyKind_LEVEL_UP_PROFICIENCY_KIND_SKILL, mc.Skills},
		{charactersv1.LevelUpProficiencyKind_LEVEL_UP_PROFICIENCY_KIND_INSTRUMENT, mc.Instruments},
	} {
		if p.pick != nil {
			picks = append(picks, multiclassPickToProto(p.kind, p.pick))
		}
	}
	return gains, picks
}

// multiclassPickToProto copies one pick of a new class.
func multiclassPickToProto(kind charactersv1.LevelUpProficiencyKind, p *rules.MulticlassPick) *charactersv1.LevelUpProficiencyChoice {
	ch := &charactersv1.LevelUpProficiencyChoice{Kind: kind, Count: i32(p.Choose)}
	for _, o := range p.From {
		ch.From = append(ch.From, &charactersv1.LevelUpProficiencyOption{Key: o.Key, NamePt: o.NamePT, AlreadyHave: o.AlreadyHave})
	}
	return ch
}

// levelUpSubclassesToProto copies the subclasses the level offers; none gives nil.
func levelUpSubclassesToProto(in []rules.LevelUpSubclass) []*charactersv1.LevelUpSubclass {
	var out []*charactersv1.LevelUpSubclass
	for _, sub := range in {
		out = append(out, &charactersv1.LevelUpSubclass{
			Key: sub.Key, NamePt: sub.NamePT, FeatureChoices: levelUpFeatureChoicesToProto(sub.FeatureChoices),
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

// unavailableToProto turns the reason a class is closed into the message's.
var unavailableToProto = map[string]charactersv1.LevelUpClassUnavailable{
	rules.UnavailableMaxLevel:            charactersv1.LevelUpClassUnavailable_LEVEL_UP_CLASS_UNAVAILABLE_MAX_LEVEL,
	rules.UnavailablePrerequisite:        charactersv1.LevelUpClassUnavailable_LEVEL_UP_CLASS_UNAVAILABLE_PREREQUISITE,
	rules.UnavailablePrerequisiteCurrent: charactersv1.LevelUpClassUnavailable_LEVEL_UP_CLASS_UNAVAILABLE_PREREQUISITE_CURRENT,
}

// classChoicesToProto is the class step: the classes the character has and the
// others, leaving out the table's classes the caller may not take (RN-23).
func classChoicesToProto(content *rules.Content, m authz.Membership, in []rules.LevelUpClassChoice) []*charactersv1.LevelUpClassChoice {
	var out []*charactersv1.LevelUpClassChoice
	for _, ch := range in {
		if ch.New {
			if reason, _ := newClassRefusal(content, m, ch.Class); reason != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_UNSPECIFIED {
				continue
			}
		}
		pc := &charactersv1.LevelUpClassChoice{
			ClassKey: ch.Class, NamePt: ch.ClassNamePT, IsNew: ch.New, FromLevel: i32(ch.FromLevel), ToLevel: i32(ch.ToLevel),
			SubclassNamePt: ch.SubclassNamePT, SubclassDue: ch.SubclassDue,
			PrerequisiteAnyOf: ch.Prerequisite.AnyOf, PrerequisiteMet: ch.Prerequisite.Met,
			Available: ch.Available, Unavailable: unavailableToProto[ch.Unavailable],
		}
		for _, r := range ch.Prerequisite.Requirements {
			pc.Prerequisites = append(pc.Prerequisites, &charactersv1.LevelUpPrerequisite{
				Ability: abilityToProto[r.Ability], Minimum: i32(r.Minimum), Have: i32(r.Have), Met: r.Met,
			})
		}
		out = append(out, pc)
	}
	return out
}

// exceptionToProto turns a multiclass exception into the message's.
var exceptionToProto = map[string]charactersv1.LevelUpMulticlassException{
	rules.ExceptionExtraAttack:      charactersv1.LevelUpMulticlassException_LEVEL_UP_MULTICLASS_EXCEPTION_EXTRA_ATTACK,
	rules.ExceptionChannelDivinity:  charactersv1.LevelUpMulticlassException_LEVEL_UP_MULTICLASS_EXCEPTION_CHANNEL_DIVINITY,
	rules.ExceptionUnarmoredDefense: charactersv1.LevelUpMulticlassException_LEVEL_UP_MULTICLASS_EXCEPTION_UNARMORED_DEFENSE,
}

// multiclassSummaryToProto is what the summary says about a multiclass level.
func multiclassSummaryToProto(sum rules.MulticlassSummary, after rules.Derived) *charactersv1.LevelUpMulticlassSummary {
	out := &charactersv1.LevelUpMulticlassSummary{
		NewClassKey: sum.NewClass, CasterLevelBefore: i32(sum.CasterLevelBefore), CasterLevelAfter: i32(sum.CasterLevelAfter),
		SlotsByTable: sum.SlotsByTable,
	}
	for _, n := range after.SpellSlots {
		out.SlotsAfter = append(out.SlotsAfter, i32(n))
	}
	for _, h := range after.HitDice {
		out.HitDiceAfter = append(out.HitDiceAfter, &rulesv1.HitDice{Faces: i32(h.Die), Count: i32(h.Count)})
	}
	for _, p := range sum.ProficienciesGained {
		out.ProficienciesGained = append(out.ProficienciesGained, &charactersv1.LevelUpNamedKey{Key: p.Key, NamePt: p.NamePT})
	}
	for _, e := range sum.Exceptions {
		out.Exceptions = append(out.Exceptions, exceptionToProto[e])
	}
	return out
}
