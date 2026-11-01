package campaigns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tablerules"
)

// cmpProto returns "" when the messages are equal, and both otherwise.
func cmpProto(got, want proto.Message) string {
	if proto.Equal(got, want) {
		return ""
	}
	return fmt.Sprintf("got %v, want %v", got, want)
}

func TestStyleOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		dice    DiceMode
		withMap bool
		fog     bool
		want    campaignsv1.TableStyle
	}{
		{"Tudo no app", DiceModeApp, true, true, campaignsv1.TableStyle_TABLE_STYLE_TUDO_NO_APP},
		{"Mesa física", DiceModePhysical, false, false, campaignsv1.TableStyle_TABLE_STYLE_MESA_FISICA},
		{"Teatro da mente", DiceModePlayersChoose, false, false, campaignsv1.TableStyle_TABLE_STYLE_TEATRO_DA_MENTE},
		{"the defaults match none", DiceModePlayersChoose, true, false, campaignsv1.TableStyle_TABLE_STYLE_PERSONALIZADO},
		{"the drawing's Mirathel", DiceModePlayersChoose, true, true, campaignsv1.TableStyle_TABLE_STYLE_PERSONALIZADO},
		{"Tudo no app with the fog off", DiceModeApp, true, false, campaignsv1.TableStyle_TABLE_STYLE_PERSONALIZADO},
		{"Mesa física with a map", DiceModePhysical, true, false, campaignsv1.TableStyle_TABLE_STYLE_PERSONALIZADO},
		{"Teatro da mente with fog", DiceModePlayersChoose, false, true, campaignsv1.TableStyle_TABLE_STYLE_PERSONALIZADO},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := StyleOf(tt.dice, tt.withMap, tt.fog); got != tt.want {
				t.Errorf("StyleOf() = %v, want %v", got, tt.want)
			}
		})
	}
}

// validRules is a TableRules the master may save: the defaults.
func validRules() *campaignsv1.TableRules {
	return &campaignsv1.TableRules{
		DiceMode:            campaignsv1.DiceMode_DICE_MODE_PLAYERS_CHOOSE,
		CombatStartsWithMap: true,
		HitPoints:           campaignsv1.HitPointsRule_HIT_POINTS_RULE_PLAYER_CHOOSES,
		AbilityMethods:      &campaignsv1.AbilityMethods{StandardArray: true, PointBuy: true, Rolled_4D6: true, Typed: true},
		Critical:            campaignsv1.CriticalRule_CRITICAL_RULE_DOUBLED_DICE,
		DeathSaves:          campaignsv1.DeathSaveVisibility_DEATH_SAVE_VISIBILITY_VISIBLE_TO_ALL,
		HiddenAreaHits:      campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_REVEAL,
		EnemyReactions:      campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ONLY_WHEN_POSSIBLE,
	}
}

func TestTableRulesParamsRefuseWhatBreaksTheLimits(t *testing.T) {
	t.Parallel()
	tooMany := make([]string, MaxHouseRules+1)
	for i := range tooMany {
		tooMany[i] = "lembrete"
	}
	tests := []struct {
		name   string
		change func(*campaignsv1.TableRules)
		ok     bool
	}{
		{"the defaults", func(*campaignsv1.TableRules) {}, true},
		{"20 reminders of 200 characters", func(r *campaignsv1.TableRules) {
			for range MaxHouseRules {
				r.HouseRules = append(r.HouseRules, strings.Repeat("ã", MaxHouseRuleLength))
			}
		}, true},
		{"21 reminders", func(r *campaignsv1.TableRules) { r.HouseRules = tooMany }, false},
		{"a reminder of 201 characters", func(r *campaignsv1.TableRules) { r.HouseRules = []string{strings.Repeat("a", MaxHouseRuleLength+1)} }, false},
		{"an empty reminder", func(r *campaignsv1.TableRules) { r.HouseRules = []string{"   "} }, false},
		{"a reminder with a line break", func(r *campaignsv1.TableRules) { r.HouseRules = []string{"a\nb"} }, false},
		{"no ability method", func(r *campaignsv1.TableRules) { r.AbilityMethods = &campaignsv1.AbilityMethods{} }, false},
		{"no ability methods message", func(r *campaignsv1.TableRules) { r.AbilityMethods = nil }, false},
		{"one ability method", func(r *campaignsv1.TableRules) { r.AbilityMethods = &campaignsv1.AbilityMethods{Typed: true} }, true},
		{"no dice mode", func(r *campaignsv1.TableRules) { r.DiceMode = 0 }, false},
		{"no hit points rule", func(r *campaignsv1.TableRules) { r.HitPoints = 0 }, false},
		{"no critical rule", func(r *campaignsv1.TableRules) { r.Critical = 0 }, false},
		{"no death saves rule", func(r *campaignsv1.TableRules) { r.DeathSaves = 0 }, false},
		{"no hidden area hits rule", func(r *campaignsv1.TableRules) { r.HiddenAreaHits = 0 }, false},
		{"hidden creatures a spell hits stay hidden", func(r *campaignsv1.TableRules) {
			r.HiddenAreaHits = campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_KEEP_HIDDEN
		}, true},
		{"the master is asked each time", func(r *campaignsv1.TableRules) {
			r.HiddenAreaHits = campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_ASK
		}, true},
		{"a hidden area hits rule that does not exist", func(r *campaignsv1.TableRules) { r.HiddenAreaHits = 9 }, false},
		{"no enemy reactions rule", func(r *campaignsv1.TableRules) { r.EnemyReactions = 0 }, false},
		{"the enemy reactions rule Sempre", func(r *campaignsv1.TableRules) {
			r.EnemyReactions = campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ALWAYS
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := validRules()
			tt.change(r)
			_, _, err := tableRulesParams(r)
			if tt.ok != (err == nil) || (err != nil && connect.CodeOf(err) != connect.CodeInvalidArgument) {
				t.Errorf("tableRulesParams() error = %v, want ok = %v (invalid_argument otherwise)", err, tt.ok)
			}
		})
	}
	if _, _, err := tableRulesParams(nil); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("tableRulesParams(nil) error = %v, want invalid_argument", err)
	}
}

// tableRules reads the table's rules as u.
func (u *user) tableRules(t *testing.T, campaignID string) *campaignsv1.GetTableRulesResponse {
	t.Helper()
	res, err := u.api.GetTableRules(t.Context(), connect.NewRequest(&campaignsv1.GetTableRulesRequest{CampaignId: campaignID}))
	if err != nil {
		t.Fatalf("GetTableRules() error = %v", err)
	}
	return res.Msg
}

func (u *user) setTableRules(campaignID string, r *campaignsv1.TableRules) (*campaignsv1.SetTableRulesResponse, error) {
	res, err := u.api.SetTableRules(context.Background(), connect.NewRequest(&campaignsv1.SetTableRulesRequest{CampaignId: campaignID, Rules: r}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// TestRN24_TableRulesRoundTrip: a campaign that never saved its rules has the
// defaults (what the app did before); each setting is saved and read back, by the
// master and by a member; the dice mode is the campaign's own.
func TestRN24_TableRulesRoundTrip(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogador")
	id := master.createCampaign(t, "Mirathel").GetId()
	_, token := master.createInvite(t, id, MaxInviteUses, 0)
	player.join(t, token)

	// The defaults, and the three presets in order.
	got := player.tableRules(t, id)
	want := validRules()
	if diff := cmpProto(got.GetRules(), want); diff != "" {
		t.Errorf("default rules differ: %s", diff)
	}
	if got.GetStyle() != campaignsv1.TableStyle_TABLE_STYLE_PERSONALIZADO || len(got.GetPresets()) != 3 {
		t.Errorf("default style = %v with %d presets, want PERSONALIZADO and 3", got.GetStyle(), len(got.GetPresets()))
	}
	for i, style := range []campaignsv1.TableStyle{campaignsv1.TableStyle_TABLE_STYLE_TUDO_NO_APP, campaignsv1.TableStyle_TABLE_STYLE_MESA_FISICA, campaignsv1.TableStyle_TABLE_STYLE_TEATRO_DA_MENTE} {
		if got.GetPresets()[i].GetStyle() != style {
			t.Errorf("preset %d = %v, want %v", i, got.GetPresets()[i].GetStyle(), style)
		}
	}
	if p := got.GetPresets()[0]; p.GetDiceMode() != campaignsv1.DiceMode_DICE_MODE_APP || !p.GetCombatStartsWithMap() || !p.GetFogOnNewMaps() {
		t.Errorf("Tudo no app preset = %v", p)
	}
	if rules, err := h.service.StoredTableRules(t.Context(), nil, id); err != nil || rules.HitPoints != "" || rules.CriticalMaxPlusRoll || rules.DeathSavesHidden ||
		rules.CombatWithoutMap || rules.FogOnNewMaps || rules.HiddenAreaHits != tablerules.HiddenAreaHitsReveal || rules.EnemyReactionsAlways || rules.ShowSaveDC || rules.AbilityMethodsOff != (tablerules.AbilityMethods{}) || rules.Reminders != nil {
		t.Errorf("StoredTableRules() of a campaign with no row = %+v, %v; want the zero value", rules, err)
	}

	// Every setting changes and comes back.
	saved := &campaignsv1.TableRules{
		DiceMode:            campaignsv1.DiceMode_DICE_MODE_PHYSICAL,
		CombatStartsWithMap: false,
		FogOnNewMaps:        true,
		HitPoints:           campaignsv1.HitPointsRule_HIT_POINTS_RULE_ROLL,
		AbilityMethods:      &campaignsv1.AbilityMethods{StandardArray: false, PointBuy: true, Rolled_4D6: false, Typed: true},
		Critical:            campaignsv1.CriticalRule_CRITICAL_RULE_MAX_PLUS_ROLL,
		DeathSaves:          campaignsv1.DeathSaveVisibility_DEATH_SAVE_VISIBILITY_OWNER_AND_MASTER,
		HiddenAreaHits:      campaignsv1.HiddenAreaHitRule_HIDDEN_AREA_HIT_RULE_ASK,
		HouseRules:          []string{"Beber uma poção é uma ação bônus", "Sem ressurreição no primeiro ato"},
		EnemyReactions:      campaignsv1.EnemyReactionsRule_ENEMY_REACTIONS_RULE_ALWAYS,
		ShowSaveDc:          true,
	}
	res, err := master.setTableRules(id, saved)
	if err != nil {
		t.Fatalf("SetTableRules() error = %v", err)
	}
	if diff := cmpProto(res.GetRules(), saved); diff != "" || res.GetStyle() != campaignsv1.TableStyle_TABLE_STYLE_PERSONALIZADO {
		t.Errorf("SetTableRules() = %v (style %v), diff %q", res.GetRules(), res.GetStyle(), diff)
	}
	for _, u := range []*user{master, player} {
		if diff := cmpProto(u.tableRules(t, id).GetRules(), saved); diff != "" {
			t.Errorf("a member reads other rules than the saved ones: %s", diff)
		}
	}
	// The dice mode is the campaign's own setting.
	camp, err := master.api.GetCampaign(t.Context(), connect.NewRequest(&campaignsv1.GetCampaignRequest{CampaignId: id}))
	if err != nil || camp.Msg.GetCampaign().GetDiceMode() != campaignsv1.DiceMode_DICE_MODE_PHYSICAL {
		t.Errorf("campaign dice mode = %v, %v; want physical (the rules write it)", camp.Msg.GetCampaign().GetDiceMode(), err)
	}
	stored, err := h.service.StoredTableRules(t.Context(), nil, id)
	if err != nil || stored.HitPoints != "roll" || !stored.CriticalMaxPlusRoll || !stored.DeathSavesHidden || !stored.CombatWithoutMap || !stored.FogOnNewMaps || stored.HiddenAreaHits != tablerules.HiddenAreaHitsAsk ||
		!stored.AbilityMethodsOff.StandardArray || stored.AbilityMethodsOff.PointBuy || !stored.AbilityMethodsOff.Roll || stored.AbilityMethodsOff.Typed || len(stored.Reminders) != 2 || !stored.EnemyReactionsAlways || !stored.ShowSaveDC {
		t.Errorf("StoredTableRules() = %+v, %v", stored, err)
	}
	if on, err := h.service.FogOnNewMaps(t.Context(), nil, id); err != nil || !on {
		t.Errorf("FogOnNewMaps() = %v, %v, want true", on, err)
	}

	// Saving again replaces everything, the house rules included.
	if _, err := master.setTableRules(id, validRules()); err != nil {
		t.Fatalf("SetTableRules(defaults) error = %v", err)
	}
	if diff := cmpProto(master.tableRules(t, id).GetRules(), validRules()); diff != "" {
		t.Errorf("rules after saving the defaults differ: %s", diff)
	}
}

// TestRN24_TheStyleIsWorkedOutNotStored: a preset's values give its style, and
// an edit of one value afterwards gives "Personalizado".
func TestRN24_TheStyleIsWorkedOutNotStored(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	id := master.createCampaign(t, "Mirathel").GetId()

	for _, p := range master.tableRules(t, id).GetPresets() {
		r := validRules()
		r.DiceMode, r.CombatStartsWithMap, r.FogOnNewMaps = p.GetDiceMode(), p.GetCombatStartsWithMap(), p.GetFogOnNewMaps()
		// A preset only fills these three: the house rules (hit points...) stay as chosen.
		r.HitPoints = campaignsv1.HitPointsRule_HIT_POINTS_RULE_AVERAGE
		res, err := master.setTableRules(id, r)
		if err != nil || res.GetStyle() != p.GetStyle() {
			t.Fatalf("SetTableRules(%v's values) = style %v, %v", p.GetStyle(), res.GetStyle(), err)
		}
		if got := master.tableRules(t, id); got.GetStyle() != p.GetStyle() || got.GetRules().GetHitPoints() != campaignsv1.HitPointsRule_HIT_POINTS_RULE_AVERAGE {
			t.Errorf("after %v: style %v, hit points %v", p.GetStyle(), got.GetStyle(), got.GetRules().GetHitPoints())
		}
		// One edit by hand.
		r.FogOnNewMaps = !r.FogOnNewMaps
		if res, err := master.setTableRules(id, r); err != nil || res.GetStyle() != campaignsv1.TableStyle_TABLE_STYLE_PERSONALIZADO {
			t.Errorf("after editing %v by hand: style %v, %v; want PERSONALIZADO", p.GetStyle(), res.GetStyle(), err)
		}
	}
}

// TestRN24_OnlyTheMasterWritesTheRules: members read, a player's write is
// permission_denied, a non-member and a pending member's writes get not_found, and a pending member reads.
func TestRN24_OnlyTheMasterWritesTheRules(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, stranger, pending := h.newUser("Mestre"), h.newUser("Jogador"), h.newUser("Outra pessoa"), h.newUser("Pendente")
	id := master.createCampaign(t, "Mirathel").GetId()
	_, token := master.createInvite(t, id, MaxInviteUses, 0)
	player.join(t, token)
	_, approval := master.createApprovalInvite(t, id)
	if res, err := pending.accept(t, approval); err != nil || !res.GetCampaign().GetAwaitingApproval() {
		t.Fatalf("a pending member = %v, %v", res, err)
	}
	ctx := t.Context()

	_, err := player.setTableRules(id, validRules())
	wantCode(t, "a player's SetTableRules()", err, connect.CodePermissionDenied)
	for name, u := range map[string]*user{"non-member": stranger, "pending member": pending} {
		_, err := u.setTableRules(id, validRules())
		wantCode(t, name+"'s SetTableRules()", err, connect.CodeNotFound)
	}
	_, err = stranger.api.GetTableRules(ctx, connect.NewRequest(&campaignsv1.GetTableRulesRequest{CampaignId: id}))
	wantCode(t, "a non-member's GetTableRules()", err, connect.CodeNotFound)
	// A pending member reads the rules, to make their character's scores (RN-15).
	for name, u := range map[string]*user{"player": player, "pending member": pending} {
		if _, err := u.api.GetTableRules(ctx, connect.NewRequest(&campaignsv1.GetTableRulesRequest{CampaignId: id})); err != nil {
			t.Errorf("a %s's GetTableRules() error = %v", name, err)
		}
	}
	// Nothing a player or a stranger sent was saved.
	if diff := cmpProto(master.tableRules(t, id).GetRules(), validRules()); diff != "" {
		t.Errorf("rules changed by a refused call: %s", diff)
	}
}

// TestRN24_TheRulesRefuseWhatBreaksTheLimits runs the limits against the server.
func TestRN24_TheRulesRefuseWhatBreaksTheLimits(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	id := master.createCampaign(t, "Mirathel").GetId()

	r := validRules()
	r.HouseRules = []string{"Beber uma poção é uma ação bônus"}
	r.AbilityMethods = &campaignsv1.AbilityMethods{}
	_, err := master.setTableRules(id, r)
	wantCode(t, "SetTableRules() with no ability method", err, connect.CodeInvalidArgument)
	r.AbilityMethods = &campaignsv1.AbilityMethods{Typed: true}
	r.HouseRules = make([]string, MaxHouseRules+1)
	for i := range r.HouseRules {
		r.HouseRules[i] = "lembrete"
	}
	_, err = master.setTableRules(id, r)
	wantCode(t, "SetTableRules() with 21 reminders", err, connect.CodeInvalidArgument)
	r.HouseRules = []string{strings.Repeat("a", MaxHouseRuleLength+1)}
	_, err = master.setTableRules(id, r)
	wantCode(t, "SetTableRules() with a long reminder", err, connect.CodeInvalidArgument)
	if diff := cmpProto(master.tableRules(t, id).GetRules(), validRules()); diff != "" {
		t.Errorf("a refused save changed the rules: %s", diff)
	}
}

// fakeAwards is the XP awarded, which a test sets.
type fakeAwards struct {
	awards int32
	xp     int64
	asks   int
}

func (f *fakeAwards) AwardedXP(context.Context, pgx.Tx, string) (int32, int64, error) {
	f.asks++
	return f.awards, f.xp, nil
}

func (u *user) setXPMode(campaignID string, mode campaignsv1.XpMode, confirm bool) (*campaignsv1.SetCampaignXpModeResponse, error) {
	res, err := u.api.SetCampaignXpMode(context.Background(), connect.NewRequest(&campaignsv1.SetCampaignXpModeRequest{CampaignId: campaignID, XpMode: mode, Confirm: confirm}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// TestRN09_TheXPModeChangesAfterCreation: with no XP awarded the mode changes at
// once; with XP awarded it needs `confirm`, and the refusal says how much was
// awarded; nothing is converted.
func TestRN09_TheXPModeChangesAfterCreation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	awards := &fakeAwards{}
	h.service.SetXPAwards(awards)
	master, player := h.newUser("Mestre"), h.newUser("Jogador")
	camp := master.createCampaign(t, "Mirathel")
	id := camp.GetId()
	_, token := master.createInvite(t, id, MaxInviteUses, 0)
	player.join(t, token)
	mode := func() campaignsv1.XpMode {
		t.Helper()
		res, err := master.api.GetCampaign(t.Context(), connect.NewRequest(&campaignsv1.GetCampaignRequest{CampaignId: id}))
		if err != nil {
			t.Fatalf("GetCampaign() error = %v", err)
		}
		return res.Msg.GetCampaign().GetXpMode()
	}
	start := mode()
	other := campaignsv1.XpMode_XP_MODE_MILESTONES
	if start == other {
		other = campaignsv1.XpMode_XP_MODE_ENEMIES
	}

	// Nothing awarded: it changes at once, and the time comes back.
	res, err := master.setXPMode(id, other, false)
	if err != nil || res.GetXpMode() != other || res.GetChangedAt() == nil || mode() != other {
		t.Fatalf("SetCampaignXpMode() with no XP awarded = %v, %v; mode %v", res, err, mode())
	}

	// XP awarded: refused without confirm, with how much was awarded.
	awards.awards, awards.xp = 3, 450
	_, err = master.setXPMode(id, start, false)
	wantCode(t, "SetCampaignXpMode() without confirm", err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	var blocked *campaignsv1.XpModeChangeBlocked
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			blocked, _ = v.(*campaignsv1.XpModeChangeBlocked)
		}
	}
	if blocked == nil || blocked.GetAwards() != 3 || blocked.GetTotalXp() != 450 {
		t.Errorf("detail = %v, want 3 awards and 450 XP", blocked)
	}
	if mode() != other {
		t.Errorf("a refused change moved the mode to %v", mode())
	}
	// With confirm it changes, and going back asks the same way.
	if res, err := master.setXPMode(id, start, true); err != nil || res.GetXpMode() != start || mode() != start {
		t.Fatalf("SetCampaignXpMode(confirm) = %v, %v; mode %v", res, err, mode())
	}
	_, err = master.setXPMode(id, other, false)
	wantCode(t, "going back without confirm", err, connect.CodeFailedPrecondition)

	// The mode it already has asks for nothing.
	asks := awards.asks
	if res, err := master.setXPMode(id, start, false); err != nil || res.GetXpMode() != start || awards.asks != asks {
		t.Errorf("setting the same mode = %v, %v (asked %d more times)", res, err, awards.asks-asks)
	}

	// A call that changes nothing returns the time of the real change, stored.
	first, err1 := master.setXPMode(id, start, false)
	h.clock.Advance(time.Hour)
	second, err2 := master.setXPMode(id, start, false)
	if err1 != nil || err2 != nil || first.GetChangedAt() == nil || !first.GetChangedAt().AsTime().Equal(second.GetChangedAt().AsTime()) {
		t.Errorf("calls that change nothing: changed_at %v then %v (%v, %v); want the same stored time", first.GetChangedAt(), second.GetChangedAt(), err1, err2)
	}

	// Only the master, and a real mode.
	_, err = player.setXPMode(id, other, true)
	wantCode(t, "a player's SetCampaignXpMode()", err, connect.CodePermissionDenied)
	_, err = master.setXPMode(id, campaignsv1.XpMode_XP_MODE_UNSPECIFIED, true)
	wantCode(t, "SetCampaignXpMode(unspecified)", err, connect.CodeInvalidArgument)
}

// TestRN24_TheRulesCarryTheNumbersOfTheMethods: the web does no rules math, so the
// answer has the standard array, the point buy's costs and budget, and the typed range.
func TestRN24_TheRulesCarryTheNumbersOfTheMethods(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	got := master.tableRules(t, master.createCampaign(t, "Mirathel").GetId())
	if !slices.Equal(got.GetStandardArray(), []int32{15, 14, 13, 12, 10, 8}) ||
		!slices.Equal(got.GetPointBuyCosts(), []int32{0, 1, 2, 3, 4, 5, 7, 9}) || got.GetPointBuyMinScore() != 8 ||
		got.GetPointBuyBudget() != 27 || got.GetTypedMinScore() != 3 || got.GetTypedMaxScore() != 18 {
		t.Errorf("the numbers = %v", got)
	}
}

// TestRN24_APendingMemberRollsByTheCampaignsDiceMode: a pending member (RN-15) creates a
// character, so CampaignDiceMode answers for them through the real service, and
// the table's rules are theirs to read.
func TestRN24_APendingMemberRollsByTheCampaignsDiceMode(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, pending, stranger := h.newUser("Mestre"), h.newUser("Pendente"), h.newUser("Outra pessoa")
	id := master.createCampaign(t, "Mirathel").GetId()
	_, approval := master.createApprovalInvite(t, id)
	if res, err := pending.accept(t, approval); err != nil || !res.GetCampaign().GetAwaitingApproval() {
		t.Fatalf("a pending member = %v, %v", res, err)
	}
	if _, err := master.setTableRules(id, func() *campaignsv1.TableRules {
		r := validRules()
		r.DiceMode = campaignsv1.DiceMode_DICE_MODE_PHYSICAL
		return r
	}()); err != nil {
		t.Fatal(err)
	}
	mode, err := h.service.CampaignDiceMode(t.Context(), nil, id, pending.id)
	if err != nil || mode != DiceModePhysical {
		t.Errorf("CampaignDiceMode(pending) = %q, %v; want physical", mode, err)
	}
	if _, err := h.service.CampaignDiceMode(t.Context(), nil, id, stranger.id); err == nil {
		t.Error("CampaignDiceMode(stranger) answered")
	}
	if got := pending.tableRules(t, id); got.GetRules().GetDiceMode() != campaignsv1.DiceMode_DICE_MODE_PHYSICAL {
		t.Errorf("the pending member reads %v", got.GetRules())
	}
}

// afterQuery is a pgx tracer that calls hook, once, when a query containing
// match has just finished: the test's way of saving between two statements.
type afterQuery struct {
	match string
	hook  func()
	once  sync.Once
}

type queryTextKey struct{}

func (*afterQuery) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryTextKey{}, data.SQL)
}

func (a *afterQuery) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if sql, _ := ctx.Value(queryTextKey{}).(string); strings.Contains(sql, a.match) {
		a.once.Do(a.hook)
	}
}

// GetTableRules answers from one snapshot: a master who saves the rules (and
// with them the dice mode) while the page is being read never makes it show
// the new dice mode with the old rules, or the other way round.
func TestGetTableRulesIsOneSnapshotWhileTheMasterSaves(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogadora := h.newUser("Mestre"), h.newUser("Jogadora")
	id := mestre.createCampaign(t, "Mirathel").GetId()
	_, token := mestre.createInvite(t, id, 0, 0)
	jogadora.join(t, token)

	before := validRules()
	before.DiceMode = campaignsv1.DiceMode_DICE_MODE_APP
	before.HitPoints = campaignsv1.HitPointsRule_HIT_POINTS_RULE_AVERAGE
	after := validRules()
	after.DiceMode = campaignsv1.DiceMode_DICE_MODE_PHYSICAL
	after.HitPoints = campaignsv1.HitPointsRule_HIT_POINTS_RULE_ROLL
	if _, err := mestre.setTableRules(id, before); err != nil {
		t.Fatalf("SetTableRules(before) error = %v", err)
	}

	// A second service on the same database, whose connections save the master's
	// new rules right after the campaign row is read: between the page's two
	// reads, if it makes two.
	tracer := &afterQuery{match: "FROM campaigns", hook: func() {
		if _, err := mestre.setTableRules(id, after); err != nil {
			t.Errorf("SetTableRules(after) from the hook error = %v", err)
		}
	}}
	pool, err := db.NewPoolWith(t.Context(), h.pool.Config().ConnString(), func(cfg *pgxpool.Config) {
		cfg.MaxConns = 2
		cfg.ConnConfig.Tracer = tracer
	})
	if err != nil {
		t.Fatalf("NewPoolWith() error = %v", err)
	}
	t.Cleanup(pool.Close)
	svc, err := New(Config{Pool: pool, Profiles: h.users, Logger: slog.New(slog.DiscardHandler), Now: h.clock.Now})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	mux := http.NewServeMux()
	svc.Mount(mux.Handle, testSessions, reqlimit.HandlerOption())
	server := httptest.NewServer(reqlimit.Middleware(mux))
	t.Cleanup(server.Close)

	client := campaignsv1connect.NewCampaignServiceClient(server.Client(), server.URL, clientOptions(jogadora.id)...)
	res, err := client.GetTableRules(t.Context(), connect.NewRequest(&campaignsv1.GetTableRulesRequest{CampaignId: id}))
	if err != nil {
		t.Fatalf("GetTableRules() error = %v", err)
	}
	got := res.Msg.GetRules()
	old := got.GetDiceMode() == before.GetDiceMode() && got.GetHitPoints() == before.GetHitPoints()
	now := got.GetDiceMode() == after.GetDiceMode() && got.GetHitPoints() == after.GetHitPoints()
	if !old && !now {
		t.Errorf("GetTableRules() = dice %v with hit points %v, want the rules as they were or as they became, not a mix", got.GetDiceMode(), got.GetHitPoints())
	}

	// Positive control: the hook did save, so the next read shows the new rules.
	res, err = client.GetTableRules(t.Context(), connect.NewRequest(&campaignsv1.GetTableRulesRequest{CampaignId: id}))
	if err != nil {
		t.Fatalf("GetTableRules() again error = %v", err)
	}
	if got := res.Msg.GetRules(); got.GetDiceMode() != after.GetDiceMode() || got.GetHitPoints() != after.GetHitPoints() {
		t.Errorf("GetTableRules() after the save = dice %v, hit points %v, want the saved rules", got.GetDiceMode(), got.GetHitPoints())
	}
}
