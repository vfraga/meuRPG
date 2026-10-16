package characters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The ability scores of a new sheet (MR-025, RN-24). The table says which ways of
// making them are allowed (TableRules.AbilityMethodsOff); a player who creates
// a sheet says which one they used (CreateCharacterRequest.ability_method) and
// the server checks the base scores against it. The check is only for a player
// creating a sheet: the master's NPCs and the master's later edits are free.
//
// "4d6, drop the lowest" is rolled and stored by the server once per new
// character (character_ability_rolls), so nobody rerolls by reloading the page;
// with physical dice (RN-18) the player types the dice once instead. The pure
// checks are in package rules (abilitymethods.go).

// errAbilityScores is the failed_precondition of the ability scores, with the
// AbilityScoresRefusal detail that tells the app why.
func errAbilityScores(reason charactersv1.AbilityScoresRefusalReason, method charactersv1.AbilityMethod) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New("the ability scores are refused: "+reason.String()))
	if detail, detailErr := connect.NewErrorDetail(&charactersv1.AbilityScoresRefusal{Reason: reason, Method: method}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// methodAllowed says whether the table allows the method.
func methodAllowed(off AbilityMethods, m charactersv1.AbilityMethod) bool {
	switch m {
	case charactersv1.AbilityMethod_ABILITY_METHOD_STANDARD_ARRAY:
		return !off.StandardArray
	case charactersv1.AbilityMethod_ABILITY_METHOD_POINT_BUY:
		return !off.PointBuy
	case charactersv1.AbilityMethod_ABILITY_METHOD_ROLLED_4D6:
		return !off.Roll
	}
	return !off.Typed
}

// baseScoreList is the sheet's six base scores in the order of the official
// sheet.
func baseScoreList(full *charactersv1.FullSheet) []int {
	b := full.GetBaseScores()
	return []int{
		int(b.GetStrength()), int(b.GetDexterity()), int(b.GetConstitution()),
		int(b.GetIntelligence()), int(b.GetWisdom()), int(b.GetCharisma()),
	}
}

// storedSets is what character_ability_rolls.sets holds: the dice of the six sets.
func decodeSets(raw []byte) ([][]int, error) {
	var sets [][]int
	if err := json.Unmarshal(raw, &sets); err != nil {
		return nil, errors.Join(errCorruptDocument, err)
	}
	return sets, nil
}

// abilityRollsToProto is the stored sets as the API's AbilityRolls.
func abilityRollsToProto(row charactersdb.GetAbilityRollsRow) (*charactersv1.AbilityRolls, error) {
	sets, err := decodeSets(row.Sets)
	if err != nil {
		return nil, err
	}
	out := &charactersv1.AbilityRolls{Typed: row.Source == "typed", RolledAt: timestamppb.New(row.RolledAt)}
	for _, set := range sets {
		dice := make([]int32, len(set))
		for i, d := range set {
			dice[i] = int32(d) //nolint:gosec // 1 to 6, checked when stored
		}
		out.Sets = append(out.Sets, &charactersv1.AbilityRollSet{Dice: dice, Total: int32(rules.AbilityRollTotal(set))}) //nolint:gosec // 3 to 18
	}
	return out, nil
}

// scoresRefusal says why the base scores do not follow the method, "" when
// they do. sets are the stored 4d6 sets, only read for ROLLED_4D6.
func scoresRefusal(method charactersv1.AbilityMethod, scores []int, sets [][]int) charactersv1.AbilityScoresRefusalReason {
	const r = charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_UNSPECIFIED
	switch method {
	case charactersv1.AbilityMethod_ABILITY_METHOD_STANDARD_ARRAY:
		if rules.CheckStandardArray(scores) != nil {
			return charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_NOT_STANDARD_ARRAY
		}
	case charactersv1.AbilityMethod_ABILITY_METHOD_POINT_BUY:
		if rules.CheckPointBuy(scores) != nil {
			return charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_BAD_POINT_BUY
		}
	case charactersv1.AbilityMethod_ABILITY_METHOD_ROLLED_4D6:
		if rules.CheckRolled(scores, sets) != nil {
			return charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_NOT_THE_ROLLS
		}
	default: // TYPED
		if rules.CheckTyped(scores) != nil {
			return charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_TYPED_OUT_OF_RANGE
		}
	}
	return r
}

// checkAbilityScores checks a player's new sheet against the way they say they
// made its base scores, and returns what the server records on the sheet
// (FullSheet.ability_origin; nil for a client that did not say). When the origin
// is ROLLED_4D6 the caller deletes the stored rolls in the same transaction. It
// runs inside the transaction of CreateCharacter, with the table's rules read in it
// (PR #121).
func (s *Service) checkAbilityScores(ctx context.Context, q *charactersdb.Queries, m authz.Membership, content *rules.Content, tr TableRules, method charactersv1.AbilityMethod, full *charactersv1.FullSheet) (*charactersv1.AbilityOrigin, error) {
	refuse := func(r charactersv1.AbilityScoresRefusalReason) error { return errAbilityScores(r, method) }
	if method == charactersv1.AbilityMethod_ABILITY_METHOD_UNSPECIFIED {
		// A client that does not say its way is the one that shipped before the rules
		// (the web keeps working at every merge): on a table that allows every way it
		// is checked as before, by the sheet's own ranges. Once the master switches a
		// way off, the way must be said, because nothing could be checked against it.
		if tr.AbilityMethodsOff != (AbilityMethods{}) {
			return nil, refuse(charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_METHOD_NOT_ALLOWED)
		}
		return nil, nil
	}
	if !methodAllowed(tr.AbilityMethodsOff, method) {
		return nil, refuse(charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_METHOD_NOT_ALLOWED)
	}
	origin := &charactersv1.AbilityOrigin{Method: method}
	var sets [][]int
	if method == charactersv1.AbilityMethod_ABILITY_METHOD_ROLLED_4D6 {
		row, err := q.GetAbilityRolls(ctx, charactersdb.GetAbilityRollsParams{CampaignID: m.CampaignID, UserID: m.UserID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, refuse(charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_NO_ROLLS_STORED)
		}
		if err != nil {
			return nil, wrap("read the stored ability rolls", err)
		}
		stored, err := abilityRollsToProto(row)
		if err != nil {
			return nil, err
		}
		origin.Rolls, origin.Typed = stored.GetSets(), stored.GetTyped()
		sets = setsOf(origin)
	}
	if r := scoresRefusal(method, baseScoreList(full), sets); r != charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_UNSPECIFIED {
		return nil, refuse(r)
	}
	if extraBonusesOver(content, full) {
		return nil, refuse(charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_EXTRA_BONUSES)
	}
	return origin, nil
}

// extraBonusesOver says whether the positive manual ability bonuses of the sheet
// add up to more than the race and the classes' improvements let the player place
// (rules.Content.FreeAbilityPoints): otherwise they would get around the way the
// scores were made. Negative bonuses are free.
func extraBonusesOver(content *rules.Content, full *charactersv1.FullSheet) bool {
	b := buildOf(full)
	return rules.PositiveManualBonus(b) > content.FreeAbilityPoints(b)
}

// hitPointsRuleRefusal checks the hit points a player's new sheet stores per level
// against the table's rule (RN-24): with "roll", every level after the first is a
// roll; with "average", none is. A level-1 sheet has nothing to decide.
func hitPointsRuleRefusal(tr TableRules, full *charactersv1.FullSheet) error {
	levels := 0
	for _, c := range full.GetClasses() {
		levels += int(c.GetLevel())
	}
	if levels <= 1 {
		return nil
	}
	hp := full.GetHitPoints()
	rolled := hp.GetMethod() == charactersv1.HitPointsMethod_HIT_POINTS_METHOD_ROLLED
	if (tr.HitPoints == HitPointsRoll && (!rolled || len(hp.GetRolls()) < levels-1)) || (tr.HitPoints == HitPointsAverage && rolled) {
		return errRefused(&charactersv1.LevelUpRefusal{
			Field: "full.hit_points.method", Reason: charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINTS_RULE,
		})
	}
	return nil
}

// setsOf is the dice of an origin's stored sets.
func setsOf(o *charactersv1.AbilityOrigin) [][]int {
	sets := make([][]int, 0, len(o.GetRolls()))
	for _, r := range o.GetRolls() {
		d := make([]int, len(r.GetDice()))
		for i, v := range r.GetDice() {
			d[i] = int(v)
		}
		sets = append(sets, d)
	}
	return sets
}

// keepAbilityOrigin makes the server's record the only one on a sheet about to be
// saved: the origin of the stored sheet (nil for a new one) replaces whatever the
// client sent. For a player editing the draft, base scores that changed must still
// follow the recorded method; the master's edits are free, and a sheet with no
// recorded origin is not checked.
func keepAbilityOrigin(m authz.Membership, content *rules.Content, stored, next *charactersv1.CharacterSheet) error {
	if next.GetFull() == nil {
		return nil
	}
	origin := stored.GetFull().GetAbilityOrigin()
	next.GetFull().AbilityOrigin = origin
	if origin == nil || isMaster(m) || stored.GetFull() == nil {
		return nil
	}
	before, after := baseScoreList(stored.GetFull()), baseScoreList(next.GetFull())
	if extraBonusesOver(content, next.GetFull()) {
		return errAbilityScores(charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_EXTRA_BONUSES, origin.GetMethod())
	}
	if slices.Equal(before, after) {
		return nil
	}
	if r := scoresRefusal(origin.GetMethod(), after, setsOf(origin)); r != charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_UNSPECIFIED {
		return errAbilityScores(r, origin.GetMethod())
	}
	return nil
}

// GetAbilityRolls implements charactersv1connect.CharacterServiceHandler.
func (s *Service) GetAbilityRolls(
	ctx context.Context,
	req *connect.Request[charactersv1.GetAbilityRollsRequest],
) (*connect.Response[charactersv1.GetAbilityRollsResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if isMaster(m) {
		return nil, errPermission("only a player makes a sheet's ability scores; the master's NPCs are free")
	}
	row, err := s.queries.GetAbilityRolls(ctx, charactersdb.GetAbilityRollsParams{CampaignID: m.CampaignID, UserID: m.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewResponse(&charactersv1.GetAbilityRollsResponse{}), nil
	}
	if err != nil {
		return nil, s.dbError(ctx, "read the ability rolls", err)
	}
	rolls, err := abilityRollsToProto(row)
	if err != nil {
		return nil, s.dbError(ctx, "read the ability rolls", err)
	}
	return connect.NewResponse(&charactersv1.GetAbilityRollsResponse{Rolls: rolls}), nil
}

// RollAbilityScores implements charactersv1connect.CharacterServiceHandler.
func (s *Service) RollAbilityScores(
	ctx context.Context,
	req *connect.Request[charactersv1.RollAbilityScoresRequest],
) (*connect.Response[charactersv1.RollAbilityScoresResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	if isMaster(m) {
		return nil, errPermission("only a player makes a sheet's ability scores; the master's NPCs are free")
	}
	typed, err := typedAbilityDice(req.Msg.GetTypedDice())
	if err != nil {
		return nil, err
	}
	res := &charactersv1.RollAbilityScoresResponse{}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		return s.rollAbilityScoresIn(ctx, tx, m, typed, res)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll the ability scores", err)
	}
	return connect.NewResponse(res), nil
}

// rolledMethod is the way of making the scores that RollAbilityScores serves.
const rolledMethod = charactersv1.AbilityMethod_ABILITY_METHOD_ROLLED_4D6

// typedAbilityDice is the dice a player typed (RN-18), checked, or nil when the app is to
// roll.
func typedAbilityDice(sets []*charactersv1.AbilityRollSet) ([][]int, error) {
	var typed [][]int
	for _, set := range sets {
		d := make([]int, len(set.GetDice()))
		for i, v := range set.GetDice() {
			d[i] = int(v)
		}
		typed = append(typed, d)
	}
	if len(typed) > 0 {
		if err := rules.CheckAbilityRollDice(typed); err != nil {
			return nil, invalidArgument(fieldErr("typed_dice", "%s", err.Error()))
		}
	}
	return typed, nil
}

// rollAbilityScoresIn is RollAbilityScores' transaction: the table's rule, then the sets
// stored already or new ones, written into res.
func (s *Service) rollAbilityScoresIn(ctx context.Context, tx pgx.Tx, m authz.Membership, typed [][]int, res *charactersv1.RollAbilityScoresResponse) error {
	q := s.queries.WithTx(tx)
	_, tr, err := s.content.For(ctx, tx, m.CampaignID)
	if err != nil {
		return wrap("read the table rules", err)
	}
	if tr.AbilityMethodsOff.Roll {
		return errAbilityScores(charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_METHOD_NOT_ALLOWED, rolledMethod)
	}
	key := charactersdb.GetAbilityRollsParams{CampaignID: m.CampaignID, UserID: m.UserID}
	stored, err := q.GetAbilityRolls(ctx, key)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return wrap("read the stored ability rolls", err)
	}
	if err == nil {
		return answerStoredRolls(stored, typed, res)
	}
	sets, source, err := s.newAbilitySets(ctx, tx, m, typed)
	if err != nil {
		return err
	}
	return s.storeAbilityRolls(ctx, q, key, sets, source, res)
}

// answerStoredRolls answers the sets stored already: reloading never rerolls. Typed dice
// are accepted again only when they are the stored ones (a retried request).
func answerStoredRolls(stored charactersdb.GetAbilityRollsRow, typed [][]int, res *charactersv1.RollAbilityScoresResponse) error {
	if len(typed) > 0 {
		have, err := decodeSets(stored.Sets)
		if err != nil {
			return err
		}
		if !slices.EqualFunc(have, typed, slices.Equal[[]int]) {
			return errAbilityScores(charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_ROLLS_ALREADY_STORED, rolledMethod)
		}
	}
	var err error
	if res.Rolls, err = abilityRollsToProto(stored); err != nil {
		return err
	}
	res.AlreadyRolled = true
	return nil
}

// newAbilitySets is the sets of a first roll and who made them ("typed" or "app"), as
// the campaign's dice setting allows.
func (s *Service) newAbilitySets(ctx context.Context, tx pgx.Tx, m authz.Membership, typed [][]int) (sets [][]int, source string, err error) {
	rule, err := s.diceRule(ctx, tx, m)
	if err != nil {
		return nil, "", err
	}
	switch {
	case len(typed) > 0 && rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_IN_APP:
		return nil, "", errAbilityScores(charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_DICE_FORCED_IN_APP, rolledMethod)
	case len(typed) == 0 && rule == charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_FORCED_PHYSICAL:
		return nil, "", errAbilityScores(charactersv1.AbilityScoresRefusalReason_ABILITY_SCORES_REFUSAL_REASON_DICE_FORCED_PHYSICAL, rolledMethod)
	case len(typed) == 0:
		sets, err = s.rollAbilitySets()
		return sets, "app", err
	}
	return typed, "typed", nil
}

// storeAbilityRolls stores the sets of a first roll and answers them as they stand.
func (s *Service) storeAbilityRolls(ctx context.Context, q *charactersdb.Queries, key charactersdb.GetAbilityRollsParams, sets [][]int, source string, res *charactersv1.RollAbilityScoresResponse) error {
	raw, err := json.Marshal(sets)
	if err != nil {
		return wrap("encode the ability rolls", err)
	}
	now := s.now()
	if _, err := q.InsertAbilityRolls(ctx, charactersdb.InsertAbilityRollsParams{
		CampaignID: key.CampaignID, UserID: key.UserID, Sets: raw, Source: source, Now: now,
	}); err != nil {
		return wrap("store the ability rolls", err)
	}
	// Read back what stands, as GetAbilityRolls would answer.
	stored, err := q.GetAbilityRolls(ctx, key)
	if err != nil {
		return wrap("read the stored ability rolls", err)
	}
	res.Rolls, err = abilityRollsToProto(stored)
	return err
}

// rollAbilitySets rolls six sets of four six-sided dice with the server's source.
func (s *Service) rollAbilitySets() ([][]int, error) {
	sets := make([][]int, 0, rules.AbilityRollSets)
	for range rules.AbilityRollSets {
		r, err := dice.Roll(s.roller, dice.Expr{Count: rules.AbilityRollDice, Sides: 6})
		if err != nil {
			return nil, fmt.Errorf("roll the ability scores: %w", err)
		}
		sets = append(sets, r.Faces)
	}
	return sets, nil
}
