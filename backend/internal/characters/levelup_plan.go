package characters

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The plan of a guided level-up (levelup.go): the shape check of the choices,
// then the phases that make the new sheet from them: the hit points of the
// level, the sheet itself, the table's retired choices and the rules of the level.

// maxLevelUpHitPointsValue is the highest hit points value a level's choices
// may carry: far above any die, it only bounds the stored number.
const maxLevelUpHitPointsValue = 1000

// levelUpList is one list of the choices, with its field and its most entries.
type levelUpList struct {
	field string
	keys  []string
	limit int
}

// levelUpLists is the choices' lists, in the order they are checked.
func levelUpLists(c *charactersv1.LevelUpChoices) []levelUpList {
	return []levelUpList{
		{"choices.cantrip_keys", c.GetCantripKeys(), rules.MaxCantrips},
		{"choices.known_spell_keys", c.GetKnownSpellKeys(), rules.MaxKnownSpells},
		{"choices.prepared_spell_keys", c.GetPreparedSpellKeys(), rules.MaxPreparedSpells},
		{"choices.feature_choice_keys", c.GetFeatureChoiceKeys(), rules.MaxListLength},
		{"choices.late_choice_keys", c.GetLateChoiceKeys(), rules.MaxListLength},
		{"choices.skill_proficiency_keys", c.GetSkillProficiencyKeys(), rules.MaxSkillKeys},
		{"choices.expertise_skill_keys", c.GetExpertiseSkillKeys(), rules.MaxSkillKeys},
	}
}

// checkLevelUpChoices checks the shape of the choices: the lists' sizes (the
// same limits as the sheet's), before anything is built from them. Unknown
// keys are found by rules.Validate on the new sheet. It cleans the texts of the
// features' choices in place.
func checkLevelUpChoices(c *charactersv1.LevelUpChoices) error {
	if c == nil {
		return fieldErr("choices", "is required")
	}
	if err := checkLevelUpLists(levelUpLists(c)); err != nil {
		return err
	}
	if err := cleanLevelUpTexts(c); err != nil {
		return err
	}
	return checkLevelUpKeys(c)
}

// checkLevelUpLists checks every list's size first, then that no list repeats an
// entry.
func checkLevelUpLists(lists []levelUpList) error {
	for _, l := range lists {
		if len(l.keys) > l.limit {
			return fieldErr(l.field, "must have at most %d entries", l.limit)
		}
	}
	for _, l := range lists {
		if len(slices.Compact(slices.Sorted(slices.Values(l.keys)))) != len(l.keys) {
			return fieldErr(l.field, "repeats an entry")
		}
	}
	return nil
}

// cleanLevelUpTexts checks the texts of the features' choices and cleans them in
// place: a blank one is dropped, the others trimmed and normalized.
func cleanLevelUpTexts(c *charactersv1.LevelUpChoices) error {
	if len(c.GetFeatureChoiceText()) > rules.MaxChoiceTexts {
		return fieldErr("choices.feature_choice_text", "must have at most %d entries", rules.MaxChoiceTexts)
	}
	for key, text := range c.GetFeatureChoiceText() {
		if strings.TrimSpace(text) == "" {
			delete(c.FeatureChoiceText, key)
			continue
		}
		clean, err := names.Clean(text, rules.MaxChoiceTextLength)
		if err != nil {
			return &fieldError{field: "choices.feature_choice_text", err: err}
		}
		c.FeatureChoiceText[key] = clean
	}
	return nil
}

// checkLevelUpKeys checks the single keys' lengths and the hit points' shape.
func checkLevelUpKeys(c *charactersv1.LevelUpChoices) error {
	if len(c.GetSwapInvocationKey()) > maxChoiceKeyLength {
		return fieldErr("choices.swap_invocation_key", "must be at most %d characters", maxChoiceKeyLength)
	}
	if sw := c.GetSwapPick(); len(sw.GetFromKey()) > maxChoiceKeyLength || len(sw.GetToKey()) > maxChoiceKeyLength || len(sw.GetGroup()) > maxChoiceKeyLength {
		return fieldErr("choices.swap_pick", "keys must be at most %d characters", maxChoiceKeyLength)
	}
	for _, sw := range c.GetSwapFeatureOptions() {
		if len(sw.GetFromKey()) > maxChoiceKeyLength || len(sw.GetToKey()) > maxChoiceKeyLength || len(sw.GetGroup()) > maxChoiceKeyLength {
			return fieldErr("choices.swap_feature_options", "keys must be at most %d characters", maxChoiceKeyLength)
		}
	}
	if len(c.GetFeatKey()) > maxContentKeyLength {
		return fieldErr("choices.feat_key", "must be at most %d characters", maxContentKeyLength)
	}
	if m := c.GetHitPoints().GetMethod(); charactersv1.LevelUpHitPointsMethod_name[int32(m)] == "" {
		return fieldErr("choices.hit_points.method", "is not a known method")
	}
	if v := c.GetHitPoints().GetValue(); v < 0 || v > maxLevelUpHitPointsValue {
		return fieldErr("choices.hit_points.value", "must be 0 to %d", maxLevelUpHitPointsValue)
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
	// bare says a choice is one the level does not offer this player: the answer is the refusal alone, no sheet is built.
	bare bool
	// record is what ListLevelUps keeps: the choices, with the hit points
	// value the level took.
	record *charactersv1.LevelUpChoices
	// applied are the choices the new sheet was made from.
	applied rules.LevelUpChoices
	// toLevel is the character's total level after the level-up.
	toLevel int
}

// planLevelUp builds the new sheet from the stored one and the choices, and
// checks it with the rules. The error is a Connect error for what the client
// got wrong (`invalid_argument`, the dice setting); a rule of the level that
// the choices break is plan.refusal. A choice that is not ready (a preview
// without a hit points method, an in-app roll that was never made) takes the
// average, and the plan still carries the sheet as far as the choices go.
func (s *Service) planLevelUp(ctx context.Context, tx pgx.Tx, q *charactersdb.Queries, m authz.Membership, t levelUpTarget, choices *charactersv1.LevelUpChoices) (levelUpPlan, error) {
	if err := checkLevelUpChoices(choices); err != nil {
		return levelUpPlan{}, invalidArgument(err)
	}
	offer, lerr := rules.LevelUpOptions(t.build, t.classKey, t.content)
	if lerr != nil {
		return levelUpPlan{}, levelUpRulesError(lerr)
	}
	if refusal, err := unofferedRefusal(t, m, choices); err != nil || refusal != nil {
		return levelUpPlan{refusal: refusal, bare: refusal != nil}, err
	}
	plan := newLevelUpPlan(t, choices, offer)
	hp, err := s.planHitPoints(ctx, tx, q, m, t, choices, offer, &plan)
	if err != nil {
		return levelUpPlan{}, err
	}
	if err := planLevelUpSheet(t, choices, hp, &plan); err != nil {
		return levelUpPlan{}, err
	}
	markRetiredChoices(t, m, &plan)
	if err := checkPlannedLevel(t, &plan); err != nil {
		return levelUpPlan{}, err
	}
	return plan, nil
}

// newLevelUpPlan starts the plan: the record of the choices and the feats rule.
func newLevelUpPlan(t levelUpTarget, choices *charactersv1.LevelUpChoices, offer rules.LevelUpOffer) levelUpPlan {
	plan := levelUpPlan{toLevel: offer.TotalTo, record: proto.CloneOf(choices)}
	plan.record.ClassKey = t.classKey
	// Feats are an optional rule of the game: only a table that plays with them lets the
	// level take one. The plan is still built (as with the other refusals) so the preview
	// can show what the rest of the choices make.
	if choices.GetFeatKey() != "" && !t.rules.FeatsAllowed {
		plan.refusal = &charactersv1.LevelUpRefusal{
			Field: "choices.feat_key", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_FEATS_NOT_ALLOWED,
		}
	}
	// Swapping a pick is a rule of the table too, and only for what the level offers.
	if swap := pickSwapFromProto(choices.GetSwapPick()); !swap.IsZero() && plan.refusal == nil && (!t.rules.PickSwapAllowed || !offer.Offers(swap)) {
		plan.refusal = &charactersv1.LevelUpRefusal{
			Field: "choices.swap_pick", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWAP_NOT_ALLOWED,
		}
	}
	// The swaps of a table feature's options are the feature's own data: only what the level offers, whatever the rule.
	if plan.refusal == nil && !offer.AllowsFeatureSwaps(featureSwapsFromProto(choices.GetSwapFeatureOptions())) {
		plan.refusal = &charactersv1.LevelUpRefusal{
			Field: "choices.swap_feature_options", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWAP_NOT_ALLOWED,
		}
	}
	return plan
}

// levelUpHitPoints is the hit points a plan gives the level: what the rules take
// and the value the record keeps, or the refusal of the chosen ones (the average
// then stands in).
type levelUpHitPoints struct {
	hp      rules.LevelUpHitPoints
	value   int
	refusal *charactersv1.LevelUpRefusal
}

// planHitPoints decides the hit points of the level and writes them into the
// record. A refusal of the hit points replaces the plan's refusal so far.
func (s *Service) planHitPoints(ctx context.Context, tx pgx.Tx, q *charactersdb.Queries, m authz.Membership, t levelUpTarget, choices *charactersv1.LevelUpChoices, offer rules.LevelUpOffer, plan *levelUpPlan) (rules.LevelUpHitPoints, error) {
	method, refusal := levelUpMethodOf(t, choices)
	if refusal != nil {
		plan.refusal = refusal
	}
	got := levelUpHitPoints{hp: rules.LevelUpHitPoints{Average: true}, value: offer.HitPointAverage}
	var err error
	switch method {
	case charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP:
		got, err = s.inAppHitPoints(ctx, tx, q, m, t, offer, got)
	case charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_PHYSICAL:
		got, err = s.typedHitPoints(ctx, tx, m, t, choices, offer, got)
	}
	if err != nil {
		return rules.LevelUpHitPoints{}, err
	}
	if got.refusal != nil {
		plan.refusal = got.refusal
	}
	plan.record.HitPoints = &charactersv1.LevelUpHitPoints{Method: method, Value: i32(got.value)}
	return got.hp, nil
}

// levelUpMethodOf is the hit points method the plan takes: the chosen one, the
// average for a preview without one, and the average in place of a method the
// table's rule (RN-24) does not allow, with its refusal. A preview without a
// method has not chosen one, so it is not refused.
func levelUpMethodOf(t levelUpTarget, choices *charactersv1.LevelUpChoices) (charactersv1.LevelUpHitPointsMethod, *charactersv1.LevelUpRefusal) {
	chosen := choices.GetHitPoints().GetMethod()
	if chosen == charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_UNSPECIFIED {
		return charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE, nil
	}
	if !hitPointsMethodAllowed(t.rules.HitPoints, chosen) {
		return charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE, &charactersv1.LevelUpRefusal{
			Field: "choices.hit_points.method", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINTS_RULE,
		}
	}
	return chosen, nil
}

// inAppHitPoints takes the roll the server kept for the level, unless the dice
// setting forces physical dice. A missing roll, or one of another class, is
// refused, with the average standing in.
func (s *Service) inAppHitPoints(ctx context.Context, tx pgx.Tx, q *charactersdb.Queries, m authz.Membership, t levelUpTarget, offer rules.LevelUpOffer, got levelUpHitPoints) (levelUpHitPoints, error) {
	rule, err := s.diceRule(ctx, tx, m)
	if err != nil {
		return got, err
	}
	if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL {
		return got, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_PHYSICAL, t.row.ID)
	}
	kept, err := q.GetLevelUpRoll(ctx, charactersdb.GetLevelUpRollParams{CharacterID: t.row.ID, ToLevel: i32(offer.TotalTo)})
	switch {
	case err == nil && kept.ClassKey != t.classKey:
		got.refusal = &charactersv1.LevelUpRefusal{
			Field: "choices.hit_points", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINT_ROLL_OTHER_CLASS,
		}
	case err == nil:
		got.hp, got.value = rules.LevelUpHitPoints{Roll: int(kept.Value)}, int(kept.Value)
	case errors.Is(err, pgx.ErrNoRows):
		got.refusal = &charactersv1.LevelUpRefusal{
			Field: "choices.hit_points", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINT_ROLL_MISSING,
		}
	default:
		return got, wrap("read the kept roll", err)
	}
	return got, nil
}

// typedHitPoints takes the result the player typed from a physical die, unless
// the dice setting forces the app's roll. A number the die cannot show is
// refused, with the average standing in.
func (s *Service) typedHitPoints(ctx context.Context, tx pgx.Tx, m authz.Membership, t levelUpTarget, choices *charactersv1.LevelUpChoices, offer rules.LevelUpOffer, got levelUpHitPoints) (levelUpHitPoints, error) {
	rule, err := s.diceRule(ctx, tx, m)
	if err != nil {
		return got, err
	}
	if rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_IN_APP {
		return got, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_DICE_FORCED_IN_APP, t.row.ID)
	}
	if typed := int(choices.GetHitPoints().GetValue()); typed < 1 || typed > offer.HitDie {
		got.refusal = &charactersv1.LevelUpRefusal{
			Field: "choices.hit_points.value", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINTS,
		}
	} else {
		got.hp, got.value = rules.LevelUpHitPoints{Roll: typed}, typed
	}
	return got, nil
}

// planLevelUpSheet writes the new Build onto a copy of the stored sheet and
// checks it as any sheet is checked.
func planLevelUpSheet(t levelUpTarget, choices *charactersv1.LevelUpChoices, hp rules.LevelUpHitPoints, plan *levelUpPlan) error {
	plan.applied = levelUpChoicesFromProto(t.classKey, choices, hp)
	after, err := rules.ApplyLevelUp(t.build, plan.applied, t.content)
	if err != nil {
		return levelUpRulesError(err)
	}
	checked, err := checkSheet(t.content, &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: applyLevelUp(t.full, after, t.idx)}})
	if err != nil {
		return invalidArgument(err)
	}
	plan.sheet = checked.GetFull()
	return nil
}

// markRetiredChoices marks a new choice the master archived, or switched off for
// a player (RN-23), and refuses it unless something else was refused first.
func markRetiredChoices(t levelUpTarget, m authz.Membership, plan *levelUpPlan) {
	if _, field, found := newArchivedChoice(t.content, t.full, plan.sheet); found {
		plan.markRetired(field, charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ARCHIVED_CHOICE)
	}
	if _, field, found := newOffChoice(t.content, t.full, plan.sheet); found && !isMaster(m) {
		plan.markRetired(field, charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWITCHED_OFF_CHOICE)
	}
}

// markRetired says the plan holds a retired choice, refused on field for reason
// when nothing else was.
func (p *levelUpPlan) markRetired(field string, reason charactersv1.LevelUpRefusalReason) {
	p.retiredChoice = true
	if p.refusal == nil {
		p.refusal = &charactersv1.LevelUpRefusal{Field: field, Reason: reason}
	}
}

// checkPlannedLevel runs the rules of the level on the new sheet, when nothing
// was refused before.
func checkPlannedLevel(t levelUpTarget, plan *levelUpPlan) error {
	if plan.refusal != nil {
		return nil
	}
	err := rules.CheckLevelUpOf(t.build, t.buildOf(plan.sheet), plan.applied, t.content)
	if err == nil {
		return nil
	}
	le, ok := errors.AsType[*rules.LevelUpError](err)
	if !ok {
		return err
	}
	plan.refusal = refusalOf(le)
	return nil
}
