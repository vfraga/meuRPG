package campaigns

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns/campaignsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// How a campaign's players roll dice (RN-18). The values are the ones the
// database stores (campaigns.dice_mode, campaign_members.dice_preference).

// DiceMode is a campaign's setting: who decides how the players roll.
type DiceMode string

// DicePreference is one member's choice, used when the campaign lets
// players choose.
type DicePreference string

// RollsIn is where a player's dice are rolled, once the campaign's mode and
// the player's preference are put together.
type RollsIn string

const (
	// DiceModePlayersChoose lets each player pick (their preference counts).
	DiceModePlayersChoose DiceMode = "players_choose"
	// DiceModeApp makes everyone roll in the app.
	DiceModeApp DiceMode = "app"
	// DiceModePhysical makes everyone roll real dice and type the sum.
	DiceModePhysical DiceMode = "physical"

	// DicePreferenceApp is "No app", the default.
	DicePreferenceApp DicePreference = "app"
	// DicePreferencePhysical is "Meus próprios dados".
	DicePreferencePhysical DicePreference = "physical"

	// RollsInApp means the app rolls and records the dice.
	RollsInApp RollsIn = "app"
	// RollsPhysical means the player rolls real dice and types the sum.
	RollsPhysical RollsIn = "physical"
)

// EffectiveDiceMode says where a player rolls (RN-18): when the campaign
// forces a mode, that mode; when it lets players choose, their preference
// (anything but "physical" means the app, the default). A forced mode makes
// the preference irrelevant, which is why the preference is kept, not
// erased, when the master changes the mode. NPCs never go through this:
// they always roll in the app.
//
// It is pure. Package play will not import this package: it declares a small
// interface and cmd/api passes it Service.PlayerDiceMode.
func EffectiveDiceMode(mode DiceMode, preference DicePreference) RollsIn {
	switch mode {
	case DiceModeApp:
		return RollsInApp
	case DiceModePhysical:
		return RollsPhysical
	}
	if preference == DicePreferencePhysical {
		return RollsPhysical
	}
	return RollsInApp
}

// CampaignDiceMode returns the campaign's dice setting for a member:
// only a mode the master forced (app or physical) binds the player; with
// players_choose each roll is theirs to choose, and their preference is just
// the default a screen offers (RN-18, decided 03/10/2026). authz.ErrNotMember
// for anyone who is not a member (a pending one counts). It reads inside tx when the caller
// has one (nil: the pool): a request that holds a transaction must not take a
// second connection.
func (s *Service) CampaignDiceMode(ctx context.Context, tx pgx.Tx, campaignID, userID string) (DiceMode, error) {
	q := s.queriesIn(tx)
	campaign, err := q.GetCampaign(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", authz.ErrNotMember
	}
	if err != nil {
		return "", fmt.Errorf("get campaign: %w", err)
	}
	// A pending member (RN-15) makes a character too, and rolls its ability scores by
	// the campaign's dice setting, so any membership counts, not only an active one.
	if _, err := q.GetMembership(ctx, campaignsdb.GetMembershipParams{CampaignID: campaignID, UserID: userID, Now: s.now()}); errors.Is(err, pgx.ErrNoRows) {
		return "", authz.ErrNotMember
	} else if err != nil {
		return "", fmt.Errorf("get membership: %w", err)
	}
	return DiceMode(campaign.DiceMode), nil
}

// PlayerDiceMode tells where userID rolls dice in campaignID (EffectiveDiceMode
// of the campaign's mode and the member's preference). It reads the campaign and
// the membership, two primary-key reads; a user who is not an active
// member gets authz.ErrNotMember.
//
// Package play will call it when a player starts a roll, through its own
// interface, with the user ID from its authorization check.
func (s *Service) PlayerDiceMode(ctx context.Context, campaignID, userID string) (RollsIn, error) {
	campaign, err := s.queries.GetCampaign(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", authz.ErrNotMember
	}
	if err != nil {
		return "", fmt.Errorf("get campaign: %w", err)
	}
	pref, err := s.queries.GetMemberDicePreference(ctx, campaignsdb.GetMemberDicePreferenceParams{CampaignID: campaignID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", authz.ErrNotMember
	}
	if err != nil {
		return "", fmt.Errorf("get dice preference: %w", err)
	}
	return EffectiveDiceMode(DiceMode(campaign.DiceMode), DicePreference(pref)), nil
}

// Conversions between the database's text values and the API's enums.
var (
	diceModeToDB = map[campaignsv1.DiceMode]string{
		campaignsv1.DiceMode_DICE_MODE_PLAYERS_CHOOSE: string(DiceModePlayersChoose),
		campaignsv1.DiceMode_DICE_MODE_APP:            string(DiceModeApp),
		campaignsv1.DiceMode_DICE_MODE_PHYSICAL:       string(DiceModePhysical),
	}
	diceModeFromDB = map[string]campaignsv1.DiceMode{
		string(DiceModePlayersChoose): campaignsv1.DiceMode_DICE_MODE_PLAYERS_CHOOSE,
		string(DiceModeApp):           campaignsv1.DiceMode_DICE_MODE_APP,
		string(DiceModePhysical):      campaignsv1.DiceMode_DICE_MODE_PHYSICAL,
	}
	dicePreferenceToDB = map[campaignsv1.DicePreference]string{
		campaignsv1.DicePreference_DICE_PREFERENCE_APP:      string(DicePreferenceApp),
		campaignsv1.DicePreference_DICE_PREFERENCE_PHYSICAL: string(DicePreferencePhysical),
	}
	dicePreferenceFromDB = map[string]campaignsv1.DicePreference{
		string(DicePreferenceApp):      campaignsv1.DicePreference_DICE_PREFERENCE_APP,
		string(DicePreferencePhysical): campaignsv1.DicePreference_DICE_PREFERENCE_PHYSICAL,
	}
)

// SetCampaignDiceMode implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) SetCampaignDiceMode(
	ctx context.Context,
	req *connect.Request[campaignsv1.SetCampaignDiceModeRequest],
) (*connect.Response[campaignsv1.SetCampaignDiceModeResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	mode, ok := diceModeToDB[req.Msg.GetMode()]
	if !ok {
		return nil, invalidArgument("mode", errors.New("must be players_choose, app or physical"))
	}
	var saved string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		saved, err = s.queries.WithTx(tx).SetCampaignDiceMode(ctx, campaignsdb.SetCampaignDiceModeParams{ID: m.CampaignID, DiceMode: mode})
		if err != nil {
			return fmt.Errorf("set dice mode: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the dice mode", err)
	}
	return connect.NewResponse(&campaignsv1.SetCampaignDiceModeResponse{Mode: diceModeFromDB[saved]}), nil
}

// SetMyDicePreference implements campaignsv1connect.CampaignServiceHandler.
func (s *Service) SetMyDicePreference(
	ctx context.Context,
	req *connect.Request[campaignsv1.SetMyDicePreferenceRequest],
) (*connect.Response[campaignsv1.SetMyDicePreferenceResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	pref, ok := dicePreferenceToDB[req.Msg.GetPreference()]
	if !ok {
		return nil, invalidArgument("preference", errors.New("must be app or physical"))
	}
	var saved string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		saved, err = s.queries.WithTx(tx).SetMemberDicePreference(ctx, campaignsdb.SetMemberDicePreferenceParams{
			CampaignID: m.CampaignID, UserID: m.UserID, DicePreference: pref,
		})
		if err != nil {
			return fmt.Errorf("set dice preference: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the dice preference", err)
	}
	return connect.NewResponse(&campaignsv1.SetMyDicePreferenceResponse{Preference: dicePreferenceFromDB[saved]}), nil
}
