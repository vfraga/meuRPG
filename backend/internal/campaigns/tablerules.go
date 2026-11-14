package campaigns

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns/campaignsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tablerules"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The table's rules (MR-025, RN-24): what the master chooses on "Regras da
// mesa", stored in campaign_table_rules (migration 00124). A campaign with no row
// has the defaults, which are what the app did before the rules existed.
//
// Who applies each rule: package characters reads them (through its
// ContentSource: the hit points of a level-up, the ability scores of a new
// sheet), package maps reads FogOnNewMaps when it creates a map, and
// package play reads the critical and the death saves. The house
// rules are only shown. This file owns the storage, the style the settings make
// and the three RPCs.

// Limits of the house rules.
const (
	// MaxHouseRules is how many reminders a table keeps (the table's CHECK says the
	// same).
	MaxHouseRules = 20
	// MaxHouseRuleLength is the longest reminder, in characters.
	MaxHouseRuleLength = 200
)

// rulesFromRow is the shared value of a stored row. The row says what is
// allowed; the value says what is off, so its zero value stays the SRD's.
func rulesFromRow(r campaignsdb.CampaignTableRule) tablerules.Rules {
	hp := tablerules.HitPointsRule(r.HitPointsRule)
	if r.HitPointsRule == hitPointsPlayerChooses {
		hp = tablerules.HitPointsPlayerChooses
	}
	return tablerules.Rules{
		HitPoints: hp,
		AbilityMethodsOff: tablerules.AbilityMethods{
			StandardArray: !r.AbilityStandardArray, PointBuy: !r.AbilityPointBuy,
			Roll: !r.AbilityRoll4d6, Typed: !r.AbilityTyped,
		},
		CriticalMaxPlusRoll: r.CriticalRule == criticalMaxPlusRoll,
		DeathSavesHidden:    r.DeathSaves == deathSavesOwnerAndMaster,
		CombatWithoutMap:    !r.CombatStartsWithMap,
		FogOnNewMaps:        r.FogOnNewMaps,
		HiddenAreaHits:      hiddenAreaHitsFromDB(r.HiddenAreaHits),
		FeatsAllowed:        r.FeatsAllowed,
		Reminders:           r.HouseRules,

		EnemyReactionsAlways: r.EnemyReactions == enemyReactionsAlways,
		ShowSaveDC:           r.ShowSaveDc,
		PickSwapAllowed:      r.PickSwapAllowed,
	}
}

// hiddenAreaHitsFromDB is the shared rule of a stored text: the default (reveal)
// is the zero value.
func hiddenAreaHitsFromDB(v string) tablerules.HiddenAreaHitRule {
	if v == hiddenAreaHitsReveal {
		return tablerules.HiddenAreaHitsReveal
	}
	return tablerules.HiddenAreaHitRule(v)
}

// The database's text values of the rules (the CHECK constraints of
// campaign_table_rules). The hit points rule is stored as "player_chooses" where
// tablerules has its empty zero value.
const (
	hitPointsPlayerChooses     = "player_chooses"
	criticalDoubledDice        = "doubled_dice"
	criticalMaxPlusRoll        = "max_plus_roll"
	deathSavesVisibleToAll     = "visible_to_all"
	deathSavesOwnerAndMaster   = "owner_and_master"
	hiddenAreaHitsReveal       = "reveal"
	enemyReactionsWhenPossible = "only_when_possible"
	enemyReactionsAlways       = "always"
)

// StoredTableRules returns the table's stored rules of a campaign: the
// defaults (the zero value) when it never saved any. Package characters reads
// them through its ContentSource, so it takes the caller's transaction (nil: the
// pool): a read made inside a transaction must not take a second connection
// (PR #121).
func (s *Service) StoredTableRules(ctx context.Context, tx pgx.Tx, campaignID string) (tablerules.Rules, error) {
	row, err := s.rowOf(ctx, tx, campaignID)
	if err != nil {
		return tablerules.Rules{}, err
	}
	return rulesFromRow(row), nil
}

// CampaignName is the campaign's name, read in tx (nil: the pool). Package characters labels
// an exported content pack with it, through its CampaignNames interface.
func (s *Service) CampaignName(ctx context.Context, tx pgx.Tx, campaignID string) (string, error) {
	c, err := s.queriesIn(tx).GetCampaign(ctx, campaignID)
	if err != nil {
		return "", fmt.Errorf("get the campaign: %w", err)
	}
	return c.Name, nil
}

// CombatWithoutMap says whether "Iniciar combate" starts without a map by
// default (RN-24, RN-25): the table's rule "combate com mapa". Package play asks
// it, in the transaction that starts the combat (tx; nil: the pool), when the
// request does not choose a mode.
func (s *Service) CombatWithoutMap(ctx context.Context, tx pgx.Tx, campaignID string) (bool, error) {
	r, err := s.StoredTableRules(ctx, tx, campaignID)
	return r.CombatWithoutMap, err
}

// FogOnNewMaps says whether a map created now starts with the fog of war on
// (RN-24). Package maps asks it when the master creates a map, through its own
// interface.
func (s *Service) FogOnNewMaps(ctx context.Context, tx pgx.Tx, campaignID string) (bool, error) {
	r, err := s.StoredTableRules(ctx, tx, campaignID)
	return r.FogOnNewMaps, err
}

// The three styles "Estilo da mesa" fills (RN-24). A style is not
// stored: StyleOf works it out from the settings it fills.
type stylePreset struct {
	style      campaignsv1.TableStyle
	dice       DiceMode
	withMap    bool
	foggedMaps bool
}

var stylePresets = []stylePreset{
	{campaignsv1.TableStyle_TABLE_STYLE_TUDO_NO_APP, DiceModeApp, true, true},
	{campaignsv1.TableStyle_TABLE_STYLE_MESA_FISICA, DiceModePhysical, false, false},
	{campaignsv1.TableStyle_TABLE_STYLE_TEATRO_DA_MENTE, DiceModePlayersChoose, false, false},
}

// StyleOf is the style the three settings make: a preset when they are exactly
// its values, PERSONALIZADO otherwise.
func StyleOf(dice DiceMode, combatWithMap, fogOnNewMaps bool) campaignsv1.TableStyle {
	for _, p := range stylePresets {
		if p.dice == dice && p.withMap == combatWithMap && p.foggedMaps == fogOnNewMaps {
			return p.style
		}
	}
	return campaignsv1.TableStyle_TABLE_STYLE_PERSONALIZADO
}

// Conversions between the database's text values and the API's enums.
var (
	hitPointsToDB = map[campaignsv1.HitPointsRule]string{
		campaignsv1.HitPointsRule_HIT_POINTS_RULE_ROLL:           string(tablerules.HitPointsRoll),
		campaignsv1.HitPointsRule_HIT_POINTS_RULE_AVERAGE:        string(tablerules.HitPointsAverage),
		campaignsv1.HitPointsRule_HIT_POINTS_RULE_PLAYER_CHOOSES: hitPointsPlayerChooses,
	}
	hitPointsFromDB = map[string]campaignsv1.HitPointsRule{
		string(tablerules.HitPointsRoll):    campaignsv1.HitPointsRule_HIT_POINTS_RULE_ROLL,
		string(tablerules.HitPointsAverage): campaignsv1.HitPointsRule_HIT_POINTS_RULE_AVERAGE,
		hitPointsPlayerChooses:              campaignsv1.HitPointsRule_HIT_POINTS_RULE_PLAYER_CHOOSES,
	}
	criticalToDB = map[campaignsv1.CriticalRule]string{
		campaignsv1.CriticalRule_CRITICAL_RULE_DOUBLED_DICE:  criticalDoubledDice,
		campaignsv1.CriticalRule_CRITICAL_RULE_MAX_PLUS_ROLL: criticalMaxPlusRoll,
	}
	criticalFromDB = map[string]campaignsv1.CriticalRule{
		criticalDoubledDice: campaignsv1.CriticalRule_CRITICAL_RULE_DOUBLED_DICE,
		criticalMaxPlusRoll: campaignsv1.CriticalRule_CRITICAL_RULE_MAX_PLUS_ROLL,
	}
	deathSavesToDB = map[campaignsv1.DeathSaveVisibility]string{
		campaignsv1.DeathSaveVisibility_DEATH_SAVE_VISIBILITY_VISIBLE_TO_ALL:   deathSavesVisibleToAll,
		campaignsv1.DeathSaveVisibility_DEATH_SAVE_VISIBILITY_OWNER_AND_MASTER: deathSavesOwnerAndMaster,
	}
	enemyReactionsToDB = map[campaignsv1.EnemyReactionsRule]string{
		campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ONLY_WHEN_POSSIBLE: enemyReactionsWhenPossible,
		campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ALWAYS:             enemyReactionsAlways,
	}
	enemyReactionsFromDB = map[string]campaignsv1.EnemyReactionsRule{
		enemyReactionsWhenPossible: campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ONLY_WHEN_POSSIBLE,
		enemyReactionsAlways:       campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ALWAYS,
	}
	hiddenAreaHitsToDB = map[campaignsv1.HiddenAreaHitRule]string{
		campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_REVEAL:      hiddenAreaHitsReveal,
		campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_KEEP_HIDDEN: string(tablerules.HiddenAreaHitsKeepHidden),
		campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_ASK:         string(tablerules.HiddenAreaHitsAsk),
	}
	hiddenAreaHitsFromDBEnum = map[string]campaignsv1.HiddenAreaHitRule{
		hiddenAreaHitsReveal:                        campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_REVEAL,
		string(tablerules.HiddenAreaHitsKeepHidden): campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_KEEP_HIDDEN,
		string(tablerules.HiddenAreaHitsAsk):        campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_ASK,
	}
	deathSavesFromDB = map[string]campaignsv1.DeathSaveVisibility{
		deathSavesVisibleToAll:   campaignsv1.DeathSaveVisibility_DEATH_SAVE_VISIBILITY_VISIBLE_TO_ALL,
		deathSavesOwnerAndMaster: campaignsv1.DeathSaveVisibility_DEATH_SAVE_VISIBILITY_OWNER_AND_MASTER,
	}
)

// defaultRow is the row of a campaign that never saved its rules: the SRD's
// defaults, the same as the table's column defaults.
func defaultRow(campaignID string) campaignsdb.CampaignTableRule {
	return campaignsdb.CampaignTableRule{
		CampaignID: campaignID, HitPointsRule: hitPointsPlayerChooses,
		AbilityStandardArray: true, AbilityPointBuy: true, AbilityRoll4d6: true, AbilityTyped: true,
		CriticalRule: criticalDoubledDice, DeathSaves: deathSavesVisibleToAll,
		CombatStartsWithMap: true, FogOnNewMaps: false, HiddenAreaHits: hiddenAreaHitsReveal, EnemyReactions: enemyReactionsWhenPossible,
	}
}

// rowOf is the table's row of a campaign: the saved one, or the defaults.
func (s *Service) rowOf(ctx context.Context, tx pgx.Tx, campaignID string) (campaignsdb.CampaignTableRule, error) {
	row, err := s.queriesIn(tx).GetTableRules(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultRow(campaignID), nil
	}
	if err != nil {
		return campaignsdb.CampaignTableRule{}, fmt.Errorf("get the table rules: %w", err)
	}
	return row, nil
}

// tableRulesToProto is the API's TableRules of a row and the campaign's dice mode.
func tableRulesToProto(r campaignsdb.CampaignTableRule, dice string) *campaignsv1.TableRules {
	return &campaignsv1.TableRules{
		DiceMode:            diceModeFromDB[dice],
		CombatStartsWithMap: r.CombatStartsWithMap,
		FogOnNewMaps:        r.FogOnNewMaps,
		HitPoints:           hitPointsFromDB[r.HitPointsRule],
		AbilityMethods: &campaignsv1.AbilityMethods{
			StandardArray: r.AbilityStandardArray, PointBuy: r.AbilityPointBuy,
			Rolled_4D6: r.AbilityRoll4d6, Typed: r.AbilityTyped,
		},
		Critical:        criticalFromDB[r.CriticalRule],
		DeathSaves:      deathSavesFromDB[r.DeathSaves],
		HouseRules:      append([]string{}, r.HouseRules...),
		HiddenAreaHits:  hiddenAreaHitsFromDBEnum[r.HiddenAreaHits],
		FeatsAllowed:    r.FeatsAllowed,
		EnemyReactions:  enemyReactionsFromDB[r.EnemyReactions],
		ShowSaveDc:      r.ShowSaveDc,
		PickSwapAllowed: r.PickSwapAllowed,
	}
}

// GetTableRules implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) GetTableRules(
	ctx context.Context,
	req *connect.Request[campaignsv1.GetTableRulesRequest],
) (*connect.Response[campaignsv1.GetTableRulesResponse], error) {
	// A pending member reads them too (RN-15, pendingMayCall): the creation step
	// needs the allowed ways of making the scores.
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	// The dice mode is the campaign's, the rest is in the rules' row, and
	// SetTableRules writes both in one transaction: one read transaction shows
	// them from the same moment, never the new dice mode with the old rules.
	var (
		campaign campaignsdb.Campaign
		row      campaignsdb.CampaignTableRule
	)
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if campaign, err = s.queries.WithTx(tx).GetCampaign(ctx, m.CampaignID); err != nil {
			return fmt.Errorf("get the campaign: %w", err)
		}
		if row, err = s.rowOf(ctx, tx, m.CampaignID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "get the table rules", err)
	}
	res := &campaignsv1.GetTableRulesResponse{
		Rules: tableRulesToProto(row, campaign.DiceMode),
		Style: StyleOf(DiceMode(campaign.DiceMode), row.CombatStartsWithMap, row.FogOnNewMaps),
		// The numbers of the ways of making scores, from package rules (the web does
		// no rules math).
		StandardArray: toInt32s(rules.StandardArray()), PointBuyMinScore: rules.PointBuyMinScore,
		PointBuyBudget: rules.PointBuyBudget, TypedMinScore: rules.TypedMinScore, TypedMaxScore: rules.TypedMaxScore,
	}
	for score := rules.PointBuyMinScore; score <= rules.PointBuyMaxScore; score++ {
		cost, _ := rules.PointBuyCost(score)
		res.PointBuyCosts = append(res.PointBuyCosts, int32(cost)) //nolint:gosec // 0 to 9
	}
	for _, p := range stylePresets {
		res.Presets = append(res.Presets, &campaignsv1.TableStylePreset{
			Style: p.style, DiceMode: diceModeFromDB[string(p.dice)], CombatStartsWithMap: p.withMap, FogOnNewMaps: p.foggedMaps,
		})
	}
	return connect.NewResponse(res), nil
}

// toInt32s converts small numbers (the standard array) for the API.
func toInt32s(in []int) []int32 {
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v) //nolint:gosec // 8 to 15
	}
	return out
}

// tableRulesParams checks the master's rules and returns them as the query's
// parameters, with the dice mode. The error names the field.
func tableRulesParams(msg *campaignsv1.TableRules) (campaignsdb.UpsertTableRulesParams, string, error) {
	var none campaignsdb.UpsertTableRulesParams
	if msg == nil {
		return none, "", invalidArgument("rules", errors.New("is required"))
	}
	dice, ok := diceModeToDB[msg.GetDiceMode()]
	if !ok {
		return none, "", invalidArgument("rules.dice_mode", errors.New("must be players_choose, app or physical"))
	}
	hp, ok := hitPointsToDB[msg.GetHitPoints()]
	if !ok {
		return none, "", invalidArgument("rules.hit_points", errors.New("must be roll, average or player_chooses"))
	}
	crit, ok := criticalToDB[msg.GetCritical()]
	if !ok {
		return none, "", invalidArgument("rules.critical", errors.New("must be doubled_dice or max_plus_roll"))
	}
	deaths, ok := deathSavesToDB[msg.GetDeathSaves()]
	if !ok {
		return none, "", invalidArgument("rules.death_saves", errors.New("must be visible_to_all or owner_and_master"))
	}
	hidden, ok := hiddenAreaHitsToDB[msg.GetHiddenAreaHits()]
	if !ok {
		return none, "", invalidArgument("rules.hidden_area_hits", errors.New("must be reveal, keep_hidden or ask"))
	}
	enemy, ok := enemyReactionsToDB[msg.GetEnemyReactions()]
	if !ok {
		return none, "", invalidArgument("rules.enemy_reactions", errors.New("must be only_when_possible or always"))
	}
	am := msg.GetAbilityMethods()
	if !am.GetStandardArray() && !am.GetPointBuy() && !am.GetRolled_4D6() && !am.GetTyped() {
		return none, "", invalidArgument("rules.ability_methods", errors.New("must allow at least one method"))
	}
	if len(msg.GetHouseRules()) > MaxHouseRules {
		return none, "", invalidArgument("rules.house_rules", fmt.Errorf("must have at most %d entries", MaxHouseRules))
	}
	houseRules := make([]string, 0, len(msg.GetHouseRules()))
	for i, h := range msg.GetHouseRules() {
		clean, err := names.Clean(h, MaxHouseRuleLength)
		if err != nil {
			return none, "", invalidArgument(fmt.Sprintf("rules.house_rules[%d]", i), err)
		}
		houseRules = append(houseRules, clean)
	}
	return campaignsdb.UpsertTableRulesParams{
		HitPointsRule:        hp,
		AbilityStandardArray: am.GetStandardArray(), AbilityPointBuy: am.GetPointBuy(),
		AbilityRoll4d6: am.GetRolled_4D6(), AbilityTyped: am.GetTyped(),
		CriticalRule: crit, DeathSaves: deaths,
		CombatStartsWithMap: msg.GetCombatStartsWithMap(), FogOnNewMaps: msg.GetFogOnNewMaps(),
		HouseRules: houseRules, HiddenAreaHits: hidden, FeatsAllowed: msg.GetFeatsAllowed(), EnemyReactions: enemy, ShowSaveDc: msg.GetShowSaveDc(), PickSwapAllowed: msg.GetPickSwapAllowed(),
	}, dice, nil
}

// SetSessionHost connects the play module's funnel. Without it, the table rules are saved in a transaction of their own and nothing
// is told.
func (s *Service) SetSessionHost(h sessiontell.Host) { s.sessionHost = h }

// inSession runs fn on a transaction of the campaign's session, through the funnel when it is connected.
func (s *Service) inSession(ctx context.Context, campaignID string, fn func(tx pgx.Tx, t sessiontell.Tell) error) error {
	if s.sessionHost != nil {
		return s.sessionHost.InSession(ctx, campaignID, fn)
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error { return fn(tx, sessiontell.None{}) })
}

// SetTableRules implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) SetTableRules(
	ctx context.Context,
	req *connect.Request[campaignsv1.SetTableRulesRequest],
) (*connect.Response[campaignsv1.SetTableRulesResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	params, dice, err := tableRulesParams(req.Msg.GetRules())
	if err != nil {
		return nil, err
	}
	params.CampaignID, params.Now = m.CampaignID, s.now()
	var saved campaignsdb.CampaignTableRule
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		q := s.queries.WithTx(tx)
		var err error
		if saved, err = q.UpsertTableRules(ctx, params); err != nil {
			return fmt.Errorf("save the table rules: %w", err)
		}
		// The dice mode is the campaign's own setting (RN-18): one transaction, so
		// the page's dice choice and the rest are saved together.
		if _, err := q.SetCampaignDiceMode(ctx, campaignsdb.SetCampaignDiceModeParams{ID: m.CampaignID, DiceMode: dice}); err != nil {
			return fmt.Errorf("set the dice mode: %w", err)
		}
		// The rules decide what the combat log shows (hideDeath, showSaveDC): the screens read again, and the master's log from the start.
		t.CombatLogChangedFull()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "save the table rules", err)
	}
	return connect.NewResponse(&campaignsv1.SetTableRulesResponse{
		Rules: tableRulesToProto(saved, dice),
		Style: StyleOf(DiceMode(dice), saved.CombatStartsWithMap, saved.FogOnNewMaps),
	}), nil
}

// XPAwards says how much XP a campaign already gave (RN-09). Package
// progression implements it (progression.Service); cmd/api connects the two
// with SetXPAwards, and neither package imports the other.
type XPAwards interface {
	// AwardedXP returns how many awards stand (not undone, a milestone mark
	// included) and the XP the characters got from them, inside tx (nil: the pool).
	AwardedXP(ctx context.Context, tx pgx.Tx, campaignID string) (awards int32, totalXP int64, err error)
}

// SetXPAwards says which service reports the XP already awarded. It is a setter
// because progression needs the campaigns service to exist first; call it once,
// before any call. Without it, a change of the XP mode never asks for a
// confirmation.
func (s *Service) SetXPAwards(x XPAwards) { s.xpAwards = x }

// errXPModeChangeBlocked is SetCampaignXpMode's answer when XP was awarded and
// the master did not confirm: the XpModeChangeBlocked detail says how much, so the
// app asks "Já houve XP dado nesta campanha".
func errXPModeChangeBlocked(awards int32, totalXP int64) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New("xp was already awarded in this campaign; confirm to change the mode"))
	if detail, detailErr := connect.NewErrorDetail(&campaignsv1.XpModeChangeBlocked{Awards: awards, TotalXp: totalXP}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// SetCampaignXpMode implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) SetCampaignXpMode(
	ctx context.Context,
	req *connect.Request[campaignsv1.SetCampaignXpModeRequest],
) (*connect.Response[campaignsv1.SetCampaignXpModeResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mode, ok := xpModeToDB[req.Msg.GetXpMode()]
	if !ok {
		return nil, invalidArgument("xp_mode", errors.New("must be enemies, gold or milestones"))
	}
	var changedAt *time.Time
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		changedAt, err = s.changeXPMode(ctx, tx, m.CampaignID, mode, req.Msg.GetConfirm())
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "change the xp mode", err)
	}
	res := &campaignsv1.SetCampaignXpModeResponse{XpMode: xpModeFromDB[mode]}
	if changedAt != nil {
		res.ChangedAt = timestamppb.New(*changedAt)
	}
	return connect.NewResponse(res), nil
}

// changeXPMode is SetCampaignXpMode's transaction. It returns when the mode
// last changed, nil when it never did.
func (s *Service) changeXPMode(ctx context.Context, tx pgx.Tx, campaignID, mode string, confirm bool) (*time.Time, error) {
	q := s.queries.WithTx(tx)
	// The campaign row is locked, so two changes of the mode wait for each other.
	// An award that lands right after the check is fine: the mode changes only from
	// now on, and nothing already awarded is converted.
	campaign, err := q.GetCampaignForUpdate(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("lock the campaign: %w", err)
	}
	if campaign.XpMode == mode {
		return campaign.XpModeChangedAt, nil // already the mode: nothing to ask, nothing to change
	}
	if s.xpAwards != nil && !confirm {
		if err := s.refuseIfXPAwarded(ctx, tx, campaignID); err != nil {
			return nil, err
		}
	}
	saved, err := q.SetCampaignXPMode(ctx, campaignsdb.SetCampaignXPModeParams{ID: campaignID, XpMode: mode, Now: s.now()})
	if err != nil {
		return nil, fmt.Errorf("set the xp mode: %w", err)
	}
	return saved.XpModeChangedAt, nil
}

// refuseIfXPAwarded is errXPModeChangeBlocked when the campaign already
// gave XP, so the master confirms the change first.
func (s *Service) refuseIfXPAwarded(ctx context.Context, tx pgx.Tx, campaignID string) error {
	awards, total, err := s.xpAwards.AwardedXP(ctx, tx, campaignID)
	if err != nil {
		return err
	}
	if awards > 0 {
		return errXPModeChangeBlocked(awards, total)
	}
	return nil
}
