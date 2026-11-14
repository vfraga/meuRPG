package play

import (
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
)

// The free dice roller (MR-052, RN-10, RN-18). These tests need the database (MEURPG_TEST_DATABASE_URL);
// the rules of a roll that need none are in free_roll_test.go.

// rollFreeAs asks u for a roll of 2d6+3 and lets edit shape the request.
func (a *armed) rollFreeAs(u *user, edit ...func(*playv1.RollFreeDiceRequest)) (*playv1.FreeRoll, error) {
	a.h.t.Helper()
	req := &playv1.RollFreeDiceRequest{CampaignId: a.campaignID, Sides: 6, Count: 2, Modifier: 3, IdempotencyKey: newKey()}
	for _, e := range edit {
		e(req)
	}
	res, err := u.play.RollFreeDice(a.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetRoll(), nil
}

func (a *armed) mustRollFree(u *user, edit ...func(*playv1.RollFreeDiceRequest)) *playv1.FreeRoll {
	a.h.t.Helper()
	roll, err := a.rollFreeAs(u, edit...)
	if err != nil {
		a.h.t.Fatalf("RollFreeDice() error = %v", err)
	}
	return roll
}

func secretRoll(r *playv1.RollFreeDiceRequest) { r.Secret = true }

func (a *armed) freeRolls(u *user) *playv1.ListFreeRollsResponse {
	a.h.t.Helper()
	res, err := u.play.ListFreeRolls(a.h.t.Context(), connect.NewRequest(&playv1.ListFreeRollsRequest{CampaignId: a.campaignID}))
	if err != nil {
		a.h.t.Fatalf("ListFreeRolls() error = %v", err)
	}
	return res.Msg
}

func rollIDs(rolls []*playv1.FreeRoll) []string {
	var out []string
	for _, r := range rolls {
		out = append(out, r.GetId())
	}
	return out
}

func TestFreeRollOfAPlayerReachesTheWholeTable(t *testing.T) {
	a := newArmed(t)
	ana, caio, master := a.ana.watch(t, a.campaignID), a.caio.watch(t, a.campaignID), a.master.watch(t, a.campaignID)
	for _, w := range []*watcher{ana, caio, master} {
		w.ready(t)
	}
	a.h.roller.queue(4, 4)
	roll := a.mustRollFree(a.ana)
	if roll.GetTotal() != 11 || roll.GetRollerName() != "Ana" || roll.GetByMaster() || roll.GetSecret() || roll.GetFormula() != "2d6 + 3" ||
		roll.GetLogText() != "Rolagem livre · Ana: 2d6 + 3 = 11 (4, 4)" || len(roll.GetDice()) != 2 {
		t.Errorf("roll = %v, want Ana's 2d6 + 3 = 11 (4, 4)", roll)
	}
	a.master.markCurrentMap(t, a.campaignID, a.mapID, ana, caio, master)
	for name, w := range map[string]*watcher{"Ana": ana, "Caio": caio, "the master": master} {
		got := w.beforeMarker(t)
		if len(got) != 1 || got[0].GetFreeRollRolled().GetRoll().GetId() != roll.GetId() || got[0].GetFreeRollRolled().GetRoll().GetLogText() != roll.GetLogText() {
			t.Errorf("%s's stream got %v, want the roll once, whole", name, got)
		}
	}
	for name, u := range map[string]*user{"Ana": a.ana, "Caio": a.caio, "the master": a.master, "Bia": a.bia} {
		if got := rollIDs(a.freeRolls(u).GetTable()); len(got) != 1 || got[0] != roll.GetId() {
			t.Errorf("%s reads the table's rolls %v, want %s", name, got, roll.GetId())
		}
	}
}

func TestSecretFreeRollReachesNoPlayer(t *testing.T) {
	a := newArmed(t)
	ana, caio, master := a.ana.watch(t, a.campaignID), a.caio.watch(t, a.campaignID), a.master.watch(t, a.campaignID)
	for _, w := range []*watcher{ana, caio, master} {
		w.ready(t)
	}
	secret := a.mustRollFree(a.master, secretRoll)
	if !secret.GetSecret() || !secret.GetByMaster() {
		t.Errorf("roll = %v, want a secret roll of the master's", secret)
	}
	a.master.markCurrentMap(t, a.campaignID, a.mapID, ana, caio, master)
	for name, w := range map[string]*watcher{"Ana": ana, "Caio": caio, "the master": master} {
		if got := w.beforeMarker(t); len(got) != 0 {
			t.Errorf("%s's stream got %v, want nothing: a secret roll is told to no stream (RN-10)", name, got)
		}
	}
	for name, u := range map[string]*user{"Ana": a.ana, "Caio": a.caio, "Bia": a.bia} {
		res := a.freeRolls(u)
		if len(res.GetTable()) != 0 || len(res.GetMine()) != 0 {
			t.Errorf("%s reads %v, want nothing, not even a count", name, res)
		}
	}
	res := a.freeRolls(a.master)
	if got := rollIDs(res.GetTable()); len(got) != 1 || got[0] != secret.GetId() {
		t.Errorf("the master reads the table's rolls %v, want the secret one (the control)", got)
	}
	if got := rollIDs(res.GetMine()); len(got) != 1 || got[0] != secret.GetId() {
		t.Errorf("the master reads their own rolls %v, want the secret one", got)
	}
}

func TestSecretFreeRollIsNotInAnyPlayersSummary(t *testing.T) {
	a := newArmed(t)
	secret := a.mustRollFree(a.master, secretRoll)
	table := a.mustRollFree(a.ana)
	id := a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession()).GetId()
	for name, u := range map[string]*user{"Ana": a.ana, "Caio": a.caio, "Bia": a.bia} {
		res, err := u.play.GetSessionSummary(t.Context(), connect.NewRequest(&playv1.GetSessionSummaryRequest{CampaignId: a.campaignID, GameSessionId: id}))
		if err != nil {
			t.Fatalf("GetSessionSummary() as %s error = %v", name, err)
		}
		body, _ := protojson.Marshal(res.Msg)
		for _, needle := range []string{secret.GetId(), table.GetId(), "Rolagem livre"} {
			if strings.Contains(string(body), needle) {
				t.Errorf("%s's summary holds %q: free rolls are not part of the summary", name, needle)
			}
		}
	}
}

func TestOnlyTheMasterRollsInSecret(t *testing.T) {
	a := newArmed(t)
	wantCode(t, "a player's secret roll", errOfRoll(a.rollFreeAs(a.ana, secretRoll)), connect.CodePermissionDenied)
	if res := a.freeRolls(a.master); len(res.GetTable()) != 0 {
		t.Errorf("the master reads %v after a refused roll, want nothing written", res.GetTable())
	}
}

func errOfRoll(_ *playv1.FreeRoll, err error) error { return err }

func TestFreeRollIsRolledByTheServerWhateverTheTableDiceMode(t *testing.T) {
	a := newArmed(t)
	for _, mode := range []campaignsv1.DiceMode{campaignsv1.DiceMode_DICE_MODE_PHYSICAL, campaignsv1.DiceMode_DICE_MODE_APP, campaignsv1.DiceMode_DICE_MODE_PLAYERS_CHOOSE} {
		a.forceDice(t, mode)
		a.h.roller.queue(6, 5)
		if roll := a.mustRollFree(a.caio); roll.GetTotal() != 14 {
			t.Errorf("%v: total = %d, want 14: the app rolls, RN-18's mode is for the game's rolls", mode, roll.GetTotal())
		}
	}
}

func TestFreeRollKeyReturnsTheFirstRollAndRollsNothingAgain(t *testing.T) {
	a := newArmed(t)
	ana, caio := a.ana.watch(t, a.campaignID), a.caio.watch(t, a.campaignID)
	ana.ready(t)
	caio.ready(t)
	key := newKey()
	withKey := func(r *playv1.RollFreeDiceRequest) { r.IdempotencyKey = key }
	a.h.roller.queue(2, 3, 6, 6)
	first := a.mustRollFree(a.ana, withKey)
	again := a.mustRollFree(a.ana, withKey)
	if again.GetId() != first.GetId() || again.GetTotal() != first.GetTotal() || again.GetTotal() != 8 {
		t.Errorf("the retry = %v, want the first roll %v", again, first)
	}
	if next := a.mustRollFree(a.ana); next.GetTotal() != 15 {
		t.Errorf("the next roll = %d, want 15 (6 + 6 + 3): the retry took no dice", next.GetTotal())
	}
	wantCode(t, "the same key for another request", errOfRoll(a.rollFreeAs(a.ana, withKey, func(r *playv1.RollFreeDiceRequest) { r.Modifier = 4 })), connect.CodeInvalidArgument)
	a.master.markCurrentMap(t, a.campaignID, a.mapID, ana, caio)
	if got := caio.beforeMarker(t); len(got) != 3 {
		t.Errorf("Caio's stream got %d events, want 3: the first roll, its retelling by the retry and the next roll", len(got))
	}
	if got := a.freeRolls(a.ana).GetMine(); len(got) != 2 {
		t.Errorf("Ana's rolls = %v, want 2: the retry wrote nothing", rollIDs(got))
	}
}

func TestFreeRollKeyIsRequiredAndBounded(t *testing.T) {
	a := newArmed(t)
	for name, key := range map[string]string{"no key": "", "blank key": "   ", "a 65-character key": strings.Repeat("k", 65)} {
		wantCode(t, name, errOfRoll(a.rollFreeAs(a.ana, func(r *playv1.RollFreeDiceRequest) { r.IdempotencyKey = key })), connect.CodeInvalidArgument)
	}
	a.mustRollFree(a.ana, func(r *playv1.RollFreeDiceRequest) { r.IdempotencyKey = strings.Repeat("k", 64) })
}

func TestTwoMembersMayUseTheSameKey(t *testing.T) {
	a := newArmed(t)
	key := newKey()
	withKey := func(r *playv1.RollFreeDiceRequest) { r.IdempotencyKey = key }
	one, two := a.mustRollFree(a.ana, withKey), a.mustRollFree(a.caio, withKey)
	if one.GetId() == two.GetId() {
		t.Errorf("Ana and Caio got the same roll %s: a key is each member's own", one.GetId())
	}
}

func TestFreeRollRefusesWhatIsOutOfRangeAndWritesNothing(t *testing.T) {
	a := newArmed(t)
	for name, edit := range map[string]func(*playv1.RollFreeDiceRequest){
		"a d7":                  func(r *playv1.RollFreeDiceRequest) { r.Sides = 7 },
		"21 dice":               func(r *playv1.RollFreeDiceRequest) { r.Count = 21 },
		"no dice":               func(r *playv1.RollFreeDiceRequest) { r.Count = 0 },
		"modifier 21":           func(r *playv1.RollFreeDiceRequest) { r.Modifier = 21 },
		"modifier -21":          func(r *playv1.RollFreeDiceRequest) { r.Modifier = -21 },
		"advantage on two dice": func(r *playv1.RollFreeDiceRequest) { r.Sides, r.Count, r.Mode = 20, 2, modeAdvantage },
		"advantage on a d6":     func(r *playv1.RollFreeDiceRequest) { r.Count, r.Mode = 1, modeAdvantage },
	} {
		wantCode(t, name, errOfRoll(a.rollFreeAs(a.ana, edit)), connect.CodeInvalidArgument)
	}
	if res := a.freeRolls(a.ana); len(res.GetTable()) != 0 {
		t.Errorf("Ana reads %v after refused rolls, want nothing", res.GetTable())
	}
}

func TestFreeRollAdvantageIsStoredAndRead(t *testing.T) {
	a := newArmed(t)
	a.h.roller.queue(9, 14)
	roll := a.mustRollFree(a.bia, func(r *playv1.RollFreeDiceRequest) { r.Sides, r.Count, r.Modifier, r.Mode = 20, 1, 5, modeAdvantage })
	if roll.GetTotal() != 19 || roll.GetMode() != modeAdvantage || len(roll.GetDice()) != 2 || !roll.GetDice()[0].GetDiscarded() || roll.GetDice()[1].GetDiscarded() {
		t.Errorf("roll = %v, want 19 with the 9 discarded", roll)
	}
	read := a.freeRolls(a.caio).GetTable()
	if len(read) != 1 || read[0].GetLogText() != "Rolagem livre · Bia: 1d20 + 5 com vantagem = 19 (9 descartado, 14)" {
		t.Errorf("Caio reads %v, want the advantage line", read)
	}
}

func TestFreeRollNeedsMembershipAndAnOpenSession(t *testing.T) {
	a := newArmed(t)
	stranger := a.h.newUser("Intruso")
	wantCode(t, "a stranger rolling", errOfRoll(a.rollFreeAs(stranger)), connect.CodeNotFound)
	_, err := stranger.play.ListFreeRolls(t.Context(), connect.NewRequest(&playv1.ListFreeRollsRequest{CampaignId: a.campaignID}))
	wantCode(t, "a stranger listing", err, connect.CodeNotFound)
	_, err = a.h.anonymous().play.RollFreeDice(t.Context(), connect.NewRequest(&playv1.RollFreeDiceRequest{CampaignId: a.campaignID, Sides: 6, Count: 1, IdempotencyKey: newKey()}))
	wantCode(t, "no sign-in", err, connect.CodeUnauthenticated)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	wantBlocked(t, "RollFreeDice", errOfRoll(a.rollFreeAs(a.ana)), playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
	_, err = a.ana.play.ListFreeRolls(t.Context(), connect.NewRequest(&playv1.ListFreeRollsRequest{CampaignId: a.campaignID}))
	wantBlocked(t, "ListFreeRolls", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
}

func TestFreeRollRateLimitIsPerUserAndAnswersHowLongToWait(t *testing.T) {
	a := newArmed(t)
	a.h.svc.free.limit = ratelimit.New(ratelimit.Config{
		PerClient: ratelimit.Rate{Burst: 2, Every: time.Hour}, Global: ratelimit.Rate{Burst: 100, Every: time.Millisecond}, MaxClients: 100,
	})
	a.mustRollFree(a.ana)
	a.mustRollFree(a.ana)
	_, err := a.rollFreeAs(a.ana)
	wantCode(t, "the third roll", err, connect.CodeResourceExhausted)
	if ce, ok := errors.AsType[*connect.Error](err); !ok || ce.Meta().Get("Retry-After") == "" {
		t.Errorf("the refusal = %v, want a Retry-After header: the app says how many seconds to wait", err)
	}
	a.mustRollFree(a.caio)
}

func TestListFreeRollsIsNewestFirstAndBounded(t *testing.T) {
	a := newArmed(t)
	var ids []string
	for range 12 {
		ids = append(ids, a.mustRollFree(a.ana).GetId())
	}
	other := a.mustRollFree(a.caio)
	mine := a.freeRolls(a.ana).GetMine()
	if len(mine) != 10 || mine[0].GetId() != ids[11] || mine[9].GetId() != ids[2] {
		t.Errorf("Ana's own rolls = %v, want her 10 newest, newest first, none of Caio's", rollIDs(mine))
	}
	table := a.freeRolls(a.ana).GetTable()
	if len(table) != 13 || table[0].GetId() != other.GetId() {
		t.Errorf("the table's rolls = %v, want all 13, Caio's newest first", rollIDs(table))
	}
	for range 40 {
		a.mustRollFree(a.bia)
	}
	if got := a.freeRolls(a.ana).GetTable(); len(got) != 50 {
		t.Errorf("the table's rolls = %d, want the 50 newest", len(got))
	}
}

func TestFreeRollsOfAnEarlierSessionAreNotListed(t *testing.T) {
	a := newArmed(t)
	a.mustRollFree(a.ana)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	a.master.start(t, a.campaignID)
	if res := a.freeRolls(a.ana); len(res.GetTable()) != 0 || len(res.GetMine()) != 0 {
		t.Errorf("Ana reads %v in a new session, want nothing: the list is the open session's", res)
	}
}
