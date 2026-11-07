package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The master's table screen. These tests need the database (MEURPG_TEST_DATABASE_URL).

func (a *armed) getScreen(t *testing.T, u *user) (*playv1.GetTableScreenResponse, error) {
	t.Helper()
	res, err := u.screen.GetTableScreen(t.Context(), connect.NewRequest(&playv1.GetTableScreenRequest{CampaignId: a.campaignID}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustGetScreen(t *testing.T) *playv1.GetTableScreenResponse {
	t.Helper()
	got, err := a.getScreen(t, a.master)
	if err != nil {
		t.Fatalf("GetTableScreen() error = %v", err)
	}
	return got
}

func (a *armed) setScreen(u *user, revision int32, mirror bool, pin playv1.TableScreenKind) (*playv1.SetTableScreenResponse, error) {
	res, err := u.screen.SetTableScreen(a.h.t.Context(), connect.NewRequest(&playv1.SetTableScreenRequest{
		CampaignId: a.campaignID, ChoiceRevision: revision, Choice: &playv1.TableScreenChoice{Mirror: mirror, Pin: pin},
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func TestGetTableScreenReadsTheDefaultChoice(t *testing.T) {
	a := newArmed(t)
	msvc, _ := a.h.mapsService(t)
	a.h.vision.SetService(msvc)
	got := a.mustGetScreen(t)
	if got.GetChoiceRevision() != 0 || !got.GetChoice().GetMirror() {
		t.Errorf("choice = %v at revision %d, want mirror at 0", got.GetChoice(), got.GetChoiceRevision())
	}
	if got.GetStage().GetKind() != kMap || got.GetNotice() != nNone {
		t.Errorf("stage = %v/%v, want the current map", got.GetStage().GetKind(), got.GetNotice())
	}
	if m := got.GetStage().GetMap(); m.GetHiddenByFog() || m.GetMap().GetMap().GetId() != a.mapID || m.GetName() != "Emboscada" || m.GetVision() == nil {
		t.Errorf("map body = %v, want the current map with its vision", m)
	}
	if got.GetCampaignName() != "Mirathel" || got.GetSessionNumber() != 1 {
		t.Errorf("header = %q, session %d, want Mirathel, 1", got.GetCampaignName(), got.GetSessionNumber())
	}
	for _, name := range []string{"Toren", "Pensantus", "Brisa"} {
		if !slices.Contains(got.GetParty(), name) {
			t.Errorf("party = %v, want %s in it", got.GetParty(), name)
		}
	}
	for _, name := range got.GetParty() {
		if name == "Goblin" || name == "Capitão Goblin" {
			t.Errorf("party = %v, want player characters only", got.GetParty())
		}
	}
	if len(got.GetOptions()) != 5 {
		t.Errorf("options = %d, want 5", len(got.GetOptions()))
	}
}

func TestGetTableScreenIsTheMastersAndNeedsASession(t *testing.T) {
	a := newArmed(t)
	_, err := a.getScreen(t, a.caio)
	wantCode(t, "GetTableScreen as a player", err, connect.CodePermissionDenied)
	_, err = a.getScreen(t, a.h.newUser("Intruso"))
	wantCode(t, "GetTableScreen as a stranger", err, connect.CodeNotFound)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	_, err = a.getScreen(t, a.master)
	wantBlocked(t, "GetTableScreen", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
}

func TestSetTableScreenStoresThePinAndBumpsTheRevision(t *testing.T) {
	a := newArmed(t)
	res, err := a.setScreen(a.master, 0, false, kCombat)
	if err != nil {
		t.Fatalf("SetTableScreen() error = %v", err)
	}
	if res.GetChoiceRevision() != 1 || res.GetChoice().GetMirror() || res.GetChoice().GetPin() != kCombat {
		t.Errorf("response = %v, want the pin at revision 1", res)
	}
	got := a.mustGetScreen(t)
	// The combat is not on: the pin stays, the stage falls to idle.
	if got.GetChoiceRevision() != 1 || got.GetChoice().GetPin() != kCombat || got.GetStage().GetKind() != kIdle || got.GetNotice() != nPinEnded {
		t.Errorf("after the pin: revision %d, choice %v, stage %v/%v; want the pin kept and idle/PIN_ENDED", got.GetChoiceRevision(), got.GetChoice(), got.GetStage().GetKind(), got.GetNotice())
	}
	// Mirroring again keeps the pin it had.
	if _, err := a.setScreen(a.master, 1, true, playv1.TableScreenKind_TABLE_SCREEN_KIND_UNSPECIFIED); err != nil {
		t.Fatalf("SetTableScreen(mirror) error = %v", err)
	}
	got = a.mustGetScreen(t)
	if !got.GetChoice().GetMirror() || got.GetChoice().GetPin() != kCombat || got.GetChoiceRevision() != 2 {
		t.Errorf("after mirroring: %v at %d, want mirror, the old pin, revision 2", got.GetChoice(), got.GetChoiceRevision())
	}
}

func TestSetTableScreenRefusesAStaleRevision(t *testing.T) {
	a := newArmed(t)
	if _, err := a.setScreen(a.master, 0, false, kMap); err != nil {
		t.Fatalf("SetTableScreen() error = %v", err)
	}
	_, err := a.setScreen(a.master, 0, false, kIdle)
	wantCode(t, "SetTableScreen over a stale revision", err, connect.CodeAborted)
	if got := a.mustGetScreen(t); got.GetChoice().GetPin() != kMap || got.GetChoiceRevision() != 1 {
		t.Errorf("choice = %v at %d, want the first write kept", got.GetChoice(), got.GetChoiceRevision())
	}
}

func TestSetTableScreenRefusesWhoIsNotTheMaster(t *testing.T) {
	a := newArmed(t)
	_, err := a.setScreen(a.caio, 0, false, kMap)
	wantCode(t, "SetTableScreen as a player", err, connect.CodePermissionDenied)
	_, err = a.setScreen(a.h.newUser("Intruso"), 0, false, kMap)
	wantCode(t, "SetTableScreen as a stranger", err, connect.CodeNotFound)
	if got := a.mustGetScreen(t); got.GetChoiceRevision() != 0 {
		t.Errorf("revision = %d after the refusals, want 0", got.GetChoiceRevision())
	}
}

func TestSetTableScreenRefusesAnInvalidPinAndNoSession(t *testing.T) {
	a := newArmed(t)
	_, err := a.setScreen(a.master, 0, false, playv1.TableScreenKind_TABLE_SCREEN_KIND_UNSPECIFIED)
	wantCode(t, "a pin of UNSPECIFIED without mirror", err, connect.CodeInvalidArgument)
	_, err = a.setScreen(a.master, 0, true, 99)
	wantCode(t, "an unknown pin", err, connect.CodeInvalidArgument)
	_, err = a.master.screen.SetTableScreen(t.Context(), connect.NewRequest(&playv1.SetTableScreenRequest{CampaignId: a.campaignID}))
	wantCode(t, "no choice", err, connect.CodeInvalidArgument)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	_, err = a.setScreen(a.master, 0, true, kIdle)
	wantBlocked(t, "SetTableScreen", err, playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
}

func TestSetTableScreenWritesOnlyTheChoiceAndTellsTheMasterAlone(t *testing.T) {
	a := newArmed(t)
	ana, master := a.ana.watch(t, a.campaignID), a.master.watch(t, a.campaignID)
	ana.ready(t)
	master.ready(t)
	before := a.master.liveSession(t, a.campaignID)
	if _, err := a.setScreen(a.master, 0, false, kMap); err != nil {
		t.Fatalf("SetTableScreen() error = %v", err)
	}
	a.master.markCurrentMap(t, a.campaignID, a.mapID, ana, master)
	var heard []int32
	for _, ev := range master.beforeMarker(t) {
		if c := ev.GetTableScreenChanged(); c != nil {
			heard = append(heard, c.GetChoiceRevision())
		}
	}
	if !slices.Equal(heard, []int32{1}) {
		t.Errorf("the master's stream heard table_screen_changed at revisions %v, want [1]", heard)
	}
	for _, ev := range ana.beforeMarker(t) {
		if ev.GetTableScreenChanged() != nil {
			t.Errorf("a player's stream got table_screen_changed")
		}
	}
	after := a.master.liveSession(t, a.campaignID)
	if after.GetCurrentMapId() != before.GetCurrentMapId() || after.GetShownImage() != nil {
		t.Errorf("the session shows %v, then %v; the choice must change nothing else", before, after)
	}
}

func TestGetTableScreenFollowsTheTable(t *testing.T) {
	a := newArmed(t)
	if _, err := a.setScreen(a.master, 0, false, kCombat); err != nil {
		t.Fatalf("SetTableScreen() error = %v", err)
	}
	a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{2},
		players: map[string]int32{"Toren": 12, "Brisa": 11, "Pensantus": 5}, reveal: []string{"Goblin"},
		at: map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Brisa": {6, 5}},
	})
	got := a.mustGetScreen(t)
	if got.GetStage().GetKind() != kCombat || got.GetNotice() != nNone {
		t.Errorf("with the combat on: stage = %v/%v, want the pinned combat", got.GetStage().GetKind(), got.GetNotice())
	}
	for _, o := range got.GetOptions() {
		if o.GetKind() == kCombat && !o.GetAvailable() {
			t.Errorf("the combat option is unavailable with a combat on: %v", o)
		}
	}
	if _, err := a.setScreen(a.master, 1, true, kCombat); err != nil {
		t.Fatalf("SetTableScreen(mirror) error = %v", err)
	}
	if got := a.mustGetScreen(t); got.GetStage().GetKind() != kCombat {
		t.Errorf("mirroring with the combat on: stage = %v, want the combat", got.GetStage().GetKind())
	}
}

func TestSetTableScreenAtTheSameRevisionHasOneWinner(t *testing.T) {
	a := newArmed(t)
	const writers = 4
	errs := make(chan error, writers)
	for i := range writers {
		pin := tvPinnable[i%len(tvPinnable)]
		go func() {
			_, err := a.setScreen(a.master, 0, false, pin)
			errs <- err
		}()
	}
	wins, stale := 0, 0
	for range writers {
		switch err := <-errs; connect.CodeOf(err) {
		case connect.CodeUnknown: // no error
			if err == nil {
				wins++
			}
		case connect.CodeAborted:
			stale++
		default:
			t.Errorf("SetTableScreen error = %v, want none or aborted", err)
		}
	}
	if wins != 1 || stale != writers-1 {
		t.Errorf("%d won and %d were refused as stale, want 1 and %d", wins, stale, writers-1)
	}
	if got := a.mustGetScreen(t); got.GetChoiceRevision() != 1 {
		t.Errorf("revision = %d, want 1", got.GetChoiceRevision())
	}
}
