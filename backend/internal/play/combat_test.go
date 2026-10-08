package play

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The combat (MR-013, Etapa 6, slice 6.3): the encounter, who fights, the
// initiative, the turns, the grid and what each person sees. These tests need
// the database (MEURPG_TEST_DATABASE_URL).

// fight is a campaign ready for a combat: a master, two players with a
// character each (Pensantus and Toren), a goblin NPC, and a map with a grid
// that is the session's current map. The session is open.
type fight struct {
	h          *harness
	campaignID string
	master     *user
	caio, ana  *user // Toren's and Pensantus's players
	pens, tor  *charactersv1.Character
	goblin     *charactersv1.Character
	mapID      string
	session    *playv1.GameSession
}

const (
	gridColumns = 20 // an image of 1000 x 500 px: 20 x 10 squares
	imageWidth  = 1000
	imageHeight = 500
)

// newMap inserts a gallery image and a map with the given grid (0: none) in
// the campaign, straight into the tables: the maps module's own tests cover
// uploading. It returns the map's ID.
func (h *harness) newMap(campaignID string, columns int) string {
	h.t.Helper()
	imageID, mapID := newKey(), newKey()
	var grid *int
	if columns > 0 {
		grid = &columns
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at)
		  VALUES ($1, $2, 'Mapa', 'image/png', $3, $4, 100, now())`, []any{imageID, campaignID, imageWidth, imageHeight}},
		{`INSERT INTO maps (id, campaign_id, name, image_id, grid_columns, created_at, updated_at)
		  VALUES ($1, $2, 'Emboscada', $3, $4, now(), now())`, []any{mapID, campaignID, imageID, grid}},
	} {
		if _, err := h.pool.Exec(h.t.Context(), q.sql, q.args...); err != nil {
			h.t.Fatalf("insert map: %v", err)
		}
	}
	return mapID
}

func (h *harness) placeToken(mapID, characterID string, xBP, yBP int) {
	h.t.Helper()
	if _, err := h.pool.Exec(h.t.Context(),
		`INSERT INTO map_tokens (map_id, character_id, x_bp, y_bp, hidden, updated_at) VALUES ($1, $2, $3, $4, false, now())`,
		mapID, characterID, xBP, yBP); err != nil {
		h.t.Fatalf("insert token: %v", err)
	}
}

func newFight(t *testing.T) *fight {
	t.Helper()
	h := newHarness(t)
	f := &fight{h: h, master: h.newUser("Samuel"), caio: h.newUser("Caio"), ana: h.newUser("Ana")}
	f.campaignID = h.newCampaign(f.master, "Mirathel", f.caio, f.ana)
	f.pens = f.ana.createCharacter(t, f.campaignID, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus")
	f.tor = f.caio.createCharacter(t, f.campaignID, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren")
	f.goblin = f.master.createCharacter(t, f.campaignID, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin")
	f.session = f.master.start(t, f.campaignID).GetGameSession()
	f.mapID = h.newMap(f.campaignID, gridColumns)
	if _, err := f.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: f.campaignID, MapId: f.mapID})); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	return f
}

// startFight starts a combat with the party and count goblins (hidden, the
// default, unless hidden says otherwise).
func (f *fight) startFight(t *testing.T, count int32, hidden ...bool) *playv1.Encounter {
	t.Helper()
	p := &playv1.Participant{CharacterId: f.goblin.GetId(), Count: count}
	if len(hidden) > 0 {
		p.Hidden = &hidden[0]
	}
	res, err := f.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: f.campaignID, IdempotencyKey: newKey(), Name: "Emboscada na estrada", Participants: []*playv1.Participant{p},
	}))
	if err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	return res.Msg.GetEncounter()
}

// get reads the combat as u.
func (f *fight) get(t *testing.T, u *user) *playv1.Encounter {
	t.Helper()
	res, err := u.combat.GetEncounter(t.Context(), connect.NewRequest(&playv1.GetEncounterRequest{CampaignId: f.campaignID}))
	if err != nil {
		t.Fatalf("GetEncounter() error = %v", err)
	}
	return res.Msg.GetEncounter()
}

func byLabel(t *testing.T, e *playv1.Encounter, label string) *playv1.Combatant {
	t.Helper()
	for _, c := range e.GetCombatants() {
		if c.GetLabel() == label {
			return c
		}
	}
	t.Fatalf("no combatant %q in %v", label, labels(e))
	return nil
}

func labels(e *playv1.Encounter) []string {
	var out []string
	for _, c := range e.GetCombatants() {
		out = append(out, c.GetLabel())
	}
	return out
}

func (f *fight) submit(t *testing.T, u *user, e *playv1.Encounter, label string, edit func(*playv1.SubmitInitiativeRequest)) (*playv1.Encounter, error) {
	t.Helper()
	req := &playv1.SubmitInitiativeRequest{CampaignId: f.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, e, label).GetId(), IdempotencyKey: newKey()}
	edit(req)
	res, err := u.combat.SubmitInitiative(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

func inApp(r *playv1.SubmitInitiativeRequest) {
	r.Roll = &playv1.SubmitInitiativeRequest_RollInApp{RollInApp: true}
}

func typed(face int32) func(*playv1.SubmitInitiativeRequest) {
	return func(r *playv1.SubmitInitiativeRequest) {
		r.Roll = &playv1.SubmitInitiativeRequest_D20Face{D20Face: face}
	}
}

// move calls MoveCombatant as u.
func (f *fight) move(t *testing.T, u *user, e *playv1.Encounter, label string, col, row int32) (*playv1.Encounter, error) {
	t.Helper()
	res, err := u.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, e, label).GetId(), IdempotencyKey: newKey(), Col: col, Row: row,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

func (f *fight) mustMove(t *testing.T, u *user, e *playv1.Encounter, label string, col, row int32) *playv1.Encounter {
	t.Helper()
	out, err := f.move(t, u, e, label, col, row)
	if err != nil {
		t.Fatalf("MoveCombatant(%s to %d,%d) error = %v", label, col, row, err)
	}
	return out
}

func (f *fight) endTurn(t *testing.T, u *user, e *playv1.Encounter, expectedID, key string) (*playv1.Encounter, error) {
	t.Helper()
	res, err := u.combat.EndTurn(t.Context(), connect.NewRequest(&playv1.EndTurnRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), IdempotencyKey: key, ExpectedCombatantId: expectedID,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

func (f *fight) begin(t *testing.T, e *playv1.Encounter) (*playv1.Encounter, error) {
	t.Helper()
	res, err := f.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

// blocked returns the EncounterBlocked detail of a failed_precondition error.
func encounterBlocked(t *testing.T, err error) *playv1.EncounterBlocked {
	t.Helper()
	wantCode(t, "call", err, connect.CodeFailedPrecondition)
	cerr, ok := err.(*connect.Error) //nolint:errorlint // the error comes straight from the client
	if !ok {
		t.Fatalf("error %v is not a connect error", err)
	}
	for _, d := range cerr.Details() {
		msg, derr := d.Value()
		if derr != nil {
			continue
		}
		if b, ok := msg.(*playv1.EncounterBlocked); ok {
			return b
		}
	}
	t.Fatalf("error %v has no EncounterBlocked detail", err)
	return nil
}

func wantEncounterBlocked(t *testing.T, err error, want playv1.EncounterBlockedReason) *playv1.EncounterBlocked {
	t.Helper()
	b := encounterBlocked(t, err)
	if b.GetReason() != want {
		t.Fatalf("blocked reason = %v, want %v", b.GetReason(), want)
	}
	return b
}

var (
	reasonTooFar   = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_FAR
	reasonNotTurn  = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN
	reasonOccupied = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SQUARE_OCCUPIED
)

// setPreference makes the player roll real dice (RN-18).
func (f *fight) setPhysical(t *testing.T, u *user) {
	t.Helper()
	if _, err := u.campaigns.SetMyDicePreference(t.Context(), connect.NewRequest(&campaignsv1.SetMyDicePreferenceRequest{
		CampaignId: f.campaignID, Preference: campaignsv1.DicePreference_DICE_PREFERENCE_PHYSICAL,
	})); err != nil {
		t.Fatalf("SetMyDicePreference() error = %v", err)
	}
}

func TestMR013_TurnOrderAndMovementLeft(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	ctx := t.Context()
	// The goblins roll 12 and 12 (a tie); Pensantus rolls 12 in the app.
	f.h.roller.queue(12, 12, 12)
	e := f.startFight(t, 2)
	if e.GetStatus() != playv1.EncounterStatus_ENCOUNTER_STATUS_SETUP || e.GetRound() != 0 || len(e.GetCombatants()) != 4 {
		t.Fatalf("started combat = %v, want SETUP, round 0, four combatants", e)
	}
	if e.GetGridColumns() != gridColumns || e.GetGridRows() != gridColumns*imageHeight/imageWidth || e.GetMapId() != f.mapID {
		t.Errorf("grid %dx%d on map %s, want %dx%d on %s", e.GetGridColumns(), e.GetGridRows(), e.GetMapId(), gridColumns, gridColumns/2, f.mapID)
	}
	g1, g2 := byLabel(t, e, "Goblin 1"), byLabel(t, e, "Goblin 2")
	if g1.GetInitiative() != 12 || g2.GetInitiative() != 12 || !g1.GetTieUnresolved() || !g2.GetTieUnresolved() {
		t.Fatalf("goblins' initiatives = %v, %v; want a tie at 12, unresolved", g1, g2)
	}
	if !g1.GetHidden() {
		t.Error("a new NPC starts hidden (question 31)")
	}

	// Beginning needs every initiative: the message names who is missing.
	_, err := f.begin(t, e)
	b := wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_INITIATIVE_MISSING)
	if len(b.GetCombatantIds()) != 2 || !strings.Contains(err.Error(), "Pensantus") || !strings.Contains(err.Error(), "Toren") {
		t.Fatalf("INITIATIVE_MISSING = %v (%v), want Pensantus and Toren", b.GetCombatantIds(), err)
	}

	// Pensantus rolls in the app: 12 + 2 (Dexterity 14). The answer is the
	// player's own view, which has no hidden goblin, so the test reads the
	// master's view again when it needs the whole order.
	mine, err := f.submit(t, f.ana, e, "Pensantus", inApp)
	if err != nil {
		t.Fatalf("SubmitInitiative(app) error = %v", err)
	}
	if got := byLabel(t, mine, "Pensantus"); got.GetInitiative() != 14 || got.GetInitiativeFace() != 12 || got.GetInitiativeBonus() != 2 {
		t.Fatalf("Pensantus = %v, want 12 + 2 = 14", got)
	}
	// A player sets theirs once: nobody rolls again until the result is good.
	_, err = f.submit(t, f.ana, e, "Pensantus", inApp)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_INITIATIVE_ALREADY_SET)

	// With "cada jogador escolhe" the player chooses on each roll (RN-18): Toren's
	// saved preference is real dice, and he may still type a face or roll in the
	// app. Only a forced mode binds: with real dice for everybody the app's
	// roll is refused, and with the app for everybody a typed face is.
	f.setPhysical(t, f.caio)
	for mode, roll := range map[campaignsv1.DiceMode]func(*playv1.SubmitInitiativeRequest){
		campaignsv1.DiceMode_DICE_MODE_PHYSICAL: inApp, campaignsv1.DiceMode_DICE_MODE_APP: typed(5),
	} {
		if _, err := f.master.campaigns.SetCampaignDiceMode(ctx, connect.NewRequest(&campaignsv1.SetCampaignDiceModeRequest{CampaignId: f.campaignID, Mode: mode})); err != nil {
			t.Fatalf("SetCampaignDiceMode() error = %v", err)
		}
		_, err = f.submit(t, f.caio, e, "Toren", roll)
		wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE)
	}
	if _, err := f.master.campaigns.SetCampaignDiceMode(ctx, connect.NewRequest(&campaignsv1.SetCampaignDiceModeRequest{CampaignId: f.campaignID, Mode: campaignsv1.DiceMode_DICE_MODE_PLAYERS_CHOOSE})); err != nil {
		t.Fatalf("SetCampaignDiceMode() error = %v", err)
	}
	_, err = f.submit(t, f.caio, e, "Toren", typed(21))
	wantCode(t, "SubmitInitiative(21)", err, connect.CodeInvalidArgument)
	_, err = f.submit(t, f.caio, e, "Toren", typed(5))
	if err != nil {
		t.Fatalf("SubmitInitiative(typed) error = %v", err)
	}
	e = f.get(t, f.master)
	if got := byLabel(t, e, "Toren"); got.GetInitiative() != 7 || got.GetInitiativeFace() != 5 {
		t.Fatalf("Toren = %v, want 5 + 2 = 7", got)
	}

	// The tie is the master's to order (RN-19): Goblin 2 before Goblin 1.
	res, err := f.master.combat.SetInitiativeOrder(ctx, connect.NewRequest(&playv1.SetInitiativeOrderRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), CombatantIds: []string{g2.GetId(), g1.GetId()},
	}))
	if err != nil {
		t.Fatalf("SetInitiativeOrder() error = %v", err)
	}
	e = res.Msg.GetEncounter()
	if got := labels(e); strings.Join(got, ",") != "Pensantus,Goblin 2,Goblin 1,Toren" {
		t.Fatalf("order = %v, want Pensantus, Goblin 2, Goblin 1, Toren", got)
	}
	if byLabel(t, e, "Goblin 1").GetTieUnresolved() {
		t.Error("the tie is still unresolved after the master ordered it")
	}
	// Combatants that are not tied cannot be ordered.
	_, err = f.master.combat.SetInitiativeOrder(ctx, connect.NewRequest(&playv1.SetInitiativeOrderRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), CombatantIds: []string{g2.GetId(), byLabel(t, e, "Toren").GetId()},
	}))
	wantCode(t, "SetInitiativeOrder(not tied)", err, connect.CodeInvalidArgument)

	// The turns begin: round 1, the first of the order on turn.
	e, err = f.begin(t, e)
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	pens := byLabel(t, e, "Pensantus")
	if e.GetStatus() != playv1.EncounterStatus_ENCOUNTER_STATUS_ACTIVE || e.GetRound() != 1 || e.GetCurrentCombatantId() != pens.GetId() || e.GetStartedAt() == nil {
		t.Fatalf("begun combat = %v, want ACTIVE, round 1, Pensantus on turn", e)
	}

	// Movement left (RN-21): Pensantus is a gnome with 25 ft. The master
	// places her, then she walks three squares (15 ft) and has 10 ft left.
	e = f.mustMove(t, f.master, e, "Pensantus", 2, 2)
	if got := byLabel(t, e, "Pensantus"); got.GetMovementLeftFt() != 25 || got.GetMovementUsedFt() != 0 {
		t.Fatalf("after the master's move = %v, want the full 25 ft left", got)
	}
	f.mustMove(t, f.ana, e, "Pensantus", 5, 2)
	e = f.get(t, f.master)
	if got := byLabel(t, e, "Pensantus"); got.GetMovementUsedFt() != 15 || got.GetMovementLeftFt() != 10 || got.GetCol() != 5 {
		t.Fatalf("after three squares = %v, want 15 ft used, 10 ft left", got)
	}
	_, err = f.move(t, f.ana, e, "Pensantus", 8, 2)
	if got := wantEncounterBlocked(t, err, reasonTooFar); got.GetMissingFt() != 5 {
		t.Errorf("missing_ft = %d, want 5 (3 squares, 10 ft left)", got.GetMissingFt())
	}
	// The Dash action doubles the speed: 50 - 15 = 35 ft left.
	if err := markDashed(ctx, f.h.svc.queries, pens.GetId()); err != nil {
		t.Fatalf("markDashed() error = %v", err)
	}
	f.mustMove(t, f.ana, e, "Pensantus", 8, 2)
	e = f.get(t, f.master)
	if got := byLabel(t, e, "Pensantus"); got.GetMovementLeftFt() != 20 || !got.GetDashed() {
		t.Fatalf("after the Dash and 3 more squares = %v, want dashed, 20 ft left", got)
	}

	// The turns go round (the goblins are the master's to end): after the last,
	// the round goes up and the first one's movement is new.
	for _, label := range []string{"Pensantus", "Goblin 2", "Goblin 1", "Toren"} {
		cur := byLabel(t, e, label)
		if e.GetCurrentCombatantId() != cur.GetId() {
			t.Fatalf("on turn = %s, want %s", e.GetCurrentCombatantId(), label)
		}
		who := f.master
		if label == "Pensantus" {
			who = f.ana
		}
		if _, err = f.endTurn(t, who, e, cur.GetId(), newKey()); err != nil {
			t.Fatalf("EndTurn(%s) error = %v", label, err)
		}
		e = f.get(t, f.master)
	}
	pens = byLabel(t, e, "Pensantus")
	if e.GetRound() != 2 || e.GetCurrentCombatantId() != pens.GetId() {
		t.Fatalf("after the last turn = round %d, current %s; want round 2, Pensantus", e.GetRound(), e.GetCurrentCombatantId())
	}
	if pens.GetMovementUsedFt() != 0 || pens.GetDashed() || pens.GetMovementLeftFt() != 25 {
		t.Errorf("Pensantus at the start of her turn = %v, want movement, Dash and economy reset", pens)
	}
}

func TestRN19_EachNPCRollsItsOwnInitiative(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	f.h.roller.queue(3, 17, 9) // three goblins, three rolls
	e := f.startFight(t, 3)
	var got []int32
	for _, label := range []string{"Goblin 1", "Goblin 2", "Goblin 3"} {
		got = append(got, byLabel(t, e, label).GetInitiative())
	}
	if got[0] != 3 || got[1] != 17 || got[2] != 9 {
		t.Fatalf("goblins' initiatives = %v, want 3, 17, 9: each copy rolls its own", got)
	}
	// The order comes from the rolls: Goblin 2 (17), Goblin 3 (9), Goblin 1 (3)
	// and the players, who have not rolled, last.
	if want := "Goblin 2,Goblin 3,Goblin 1,Pensantus,Toren"; strings.Join(labels(e), ",") != want {
		t.Errorf("order = %v, want %s", labels(e), want)
	}
	// A reinforcement rolls for itself and takes its place in the order.
	f.h.roller.queue(10)
	res, err := f.master.combat.AddCombatants(t.Context(), connect.NewRequest(&playv1.AddCombatantsRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(),
		Participants: []*playv1.Participant{{CharacterId: f.goblin.GetId()}},
	}))
	if err != nil {
		t.Fatalf("AddCombatants() error = %v", err)
	}
	if want := "Goblin 2,Goblin 4,Goblin 3,Goblin 1,Pensantus,Toren"; strings.Join(labels(res.Msg.GetEncounter()), ",") != want {
		t.Errorf("order with the reinforcement = %v, want %s", labels(res.Msg.GetEncounter()), want)
	}
}

func TestRN20_PlayersNeverReceiveHiddenCombatantsOrNPCNumbers(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	// Goblin 1 hides, Goblin 2 is revealed; the rolls are 17 and 13.
	f.h.roller.queue(17, 13)
	e := f.startFight(t, 2)
	g1, g2 := byLabel(t, e, "Goblin 1"), byLabel(t, e, "Goblin 2")
	if _, err := f.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), CombatantId: g2.GetId(), IdempotencyKey: newKey(), Hidden: false,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	f.mustMove(t, f.master, e, "Goblin 1", 4, 4)

	for _, p := range []*user{f.ana, f.caio} {
		seen := f.get(t, p)
		raw, err := protojson.Marshal(seen)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		text := string(raw)
		// The hidden goblin: no ID, no label, nothing of it.
		if strings.Contains(text, g1.GetId()) || strings.Contains(text, "Goblin 1") {
			t.Fatalf("a player's GetEncounter has the hidden goblin: %s", text)
		}
		if strings.Contains(text, f.goblin.GetId()) {
			t.Errorf("a player's GetEncounter has an NPC's character ID: %s", text)
		}
		// No hit point of an NPC (the goblin has 7), no initiative roll of one
		// (13 and 17), no bonus, no armor class: a state word only.
		for _, banned := range []string{"hitPoints", "armor", `"initiative":13`, `"initiative":17`, `"hidden"`} {
			if strings.Contains(text, banned) {
				t.Errorf("a player's GetEncounter has %q: %s", banned, text)
			}
		}
		if len(seen.GetCombatants()) != 3 { // Pensantus, Toren, Goblin 2
			t.Errorf("a player sees %v, want Pensantus, Toren and the revealed Goblin 2", labels(seen))
		}
		visible := byLabel(t, seen, "Goblin 2")
		if visible.GetState() != playv1.CombatantState_COMBATANT_STATE_UNHURT || visible.Initiative != nil || visible.InitiativeBonus != nil || visible.InitiativeFace != nil {
			t.Errorf("Goblin 2 for a player = %v, want the word Ileso and no initiative, bonus or d20", visible)
		}
	}
	// The master sees all of it.
	master := f.get(t, f.master)
	if len(master.GetCombatants()) != 4 {
		t.Fatalf("the master sees %v, want 4 combatants", labels(master))
	}
	if gm := byLabel(t, master, "Goblin 1"); gm.GetHitPointsCurrent() != 7 || gm.GetInitiative() != 17 || !gm.GetHidden() {
		t.Errorf("Goblin 1 for the master = %v, want 7 hit points, initiative 17, hidden", gm)
	}

	// A hidden combatant's turn is "Vez do mestre": nobody on turn, and a flag.
	f.submitAll(t, e)
	e = f.get(t, f.master)
	e, err := f.begin(t, e)
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	if e.GetCurrentCombatantId() != g1.GetId() {
		t.Fatalf("the master sees %s on turn, want the hidden Goblin 1", e.GetCurrentCombatantId())
	}
	player := f.get(t, f.ana)
	if player.GetCurrentCombatantId() != "" || !player.GetMasterTurn() {
		t.Errorf("a player's turn = %q master_turn %v, want empty and true", player.GetCurrentCombatantId(), player.GetMasterTurn())
	}
	// The player cannot even name the hidden combatant: not_found.
	_, err = f.move(t, f.master, e, "Goblin 1", 5, 5) // the master moves it
	if err != nil {
		t.Fatalf("the master's move error = %v", err)
	}
	_, err = f.ana.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), CombatantId: g1.GetId(), IdempotencyKey: newKey(), Col: 1, Row: 1,
	}))
	wantCode(t, "MoveCombatant(hidden)", err, connect.CodeNotFound)
}

// submitAll gives every player combatant without an initiative one, so the
// combat can begin.
func (f *fight) submitAll(t *testing.T, e *playv1.Encounter) {
	t.Helper()
	for _, c := range []struct {
		u     *user
		label string
	}{{f.ana, "Pensantus"}, {f.caio, "Toren"}} {
		// Typed faces, one apart, so the two never tie: a tie is a joint turn
		// (MR-013), and these tests are about one combatant on turn at a time.
		if _, err := f.submit(t, c.u, e, c.label, typed(map[string]int32{"Pensantus": 11, "Toren": 10}[c.label])); err != nil {
			t.Fatalf("SubmitInitiative(%s) error = %v", c.label, err)
		}
	}
}

func TestRN21_PlayerMovementIsLimitedTheMasterIsNot(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	f.h.roller.queue(1, 20, 12) // Goblin on top, then Pensantus 12 + 2, Toren 10 + 2
	e := f.startFight(t, 1, false)
	f.submitAll(t, e)
	e = f.get(t, f.master)
	e, err := f.begin(t, e)
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	// Order: Pensantus 14... (the goblin rolled 1): Pensantus, Toren (12), Goblin (1).
	if labels(e)[0] != "Pensantus" {
		t.Fatalf("order = %v, want Pensantus first", labels(e))
	}
	e = f.mustMove(t, f.master, e, "Pensantus", 3, 3)
	e = f.mustMove(t, f.master, e, "Toren", 4, 2) // not on Pensantus's row: passing a creature costs 5 ft more (RN-21)
	e = f.mustMove(t, f.master, e, "Goblin", 10, 8)

	// The master moves anyone anywhere, however far, and nothing is spent.
	e = f.mustMove(t, f.master, e, "Pensantus", 19, 9)
	if got := byLabel(t, e, "Pensantus"); got.GetMovementUsedFt() != 0 || got.GetCol() != 19 || got.GetRow() != 9 {
		t.Fatalf("after the master's move across the map = %v, want no feet spent", got)
	}
	e = f.mustMove(t, f.master, e, "Pensantus", 3, 3)

	// A player: not on turn, or someone else's character, or too far, or onto
	// a square taken, or off the grid.
	_, err = f.move(t, f.caio, e, "Toren", 4, 4)
	wantEncounterBlocked(t, err, reasonNotTurn)
	_, err = f.move(t, f.caio, e, "Pensantus", 4, 4)
	wantCode(t, "Toren's player moves Pensantus", err, connect.CodePermissionDenied)
	_, err = f.move(t, f.ana, e, "Pensantus", 9, 3) // 6 squares = 30 ft > 25 ft
	if b := wantEncounterBlocked(t, err, reasonTooFar); b.GetMissingFt() != 5 {
		t.Errorf("missing_ft = %d, want 5", b.GetMissingFt())
	}
	_, err = f.move(t, f.ana, e, "Pensantus", 4, 2) // Toren is there
	wantEncounterBlocked(t, err, reasonOccupied)
	_, err = f.move(t, f.ana, e, "Pensantus", 20, 3)
	wantCode(t, "Pensantus off the grid", err, connect.CodeInvalidArgument)
	// The master may also stack, but a visible combatant blocks a player; one
	// hidden from the player does not tell them anything.
	e = f.mustMove(t, f.ana, e, "Pensantus", 8, 3) // 5 squares = 25 ft: exactly the speed
	if got := byLabel(t, e, "Pensantus"); got.GetMovementLeftFt() != 0 {
		t.Errorf("movement left = %d, want 0", got.GetMovementLeftFt())
	}
	_, err = f.move(t, f.ana, e, "Pensantus", 8, 4)
	wantEncounterBlocked(t, err, reasonTooFar)

	// Before the turns begin a player moves nobody: only the master places.
	f2 := newFight(t)
	e2 := f2.startFight(t, 1)
	_, err = f2.move(t, f2.ana, e2, "Pensantus", 1, 1)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE)
}

func TestEndTurnIsIdempotent(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	f.h.roller.queue(1)
	e := f.startFight(t, 1, false)
	f.submitAll(t, e)
	e, err := f.begin(t, f.get(t, f.master))
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	first := e.GetCurrentCombatantId()
	key := newKey()
	e1, err := f.endTurn(t, f.master, e, first, key)
	if err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	second := e1.GetCurrentCombatantId()
	if second == first {
		t.Fatal("the first EndTurn did not pass the turn")
	}
	// A retry with the same key changes nothing and answers as it is now.
	e2, err := f.endTurn(t, f.master, e, first, key)
	if err != nil {
		t.Fatalf("EndTurn(retry) error = %v", err)
	}
	if e2.GetCurrentCombatantId() != second || e2.GetRevision() != e1.GetRevision() {
		t.Fatalf("the retry moved the turn: current %s revision %d, want %s revision %d", e2.GetCurrentCombatantId(), e2.GetRevision(), second, e1.GetRevision())
	}
	// A double tap (another key, the same expected combatant) is aborted: it
	// must not skip the second turn.
	_, err = f.endTurn(t, f.master, e, first, newKey())
	wantCode(t, "EndTurn(stale)", err, connect.CodeAborted)
	if got := f.get(t, f.master); got.GetCurrentCombatantId() != second {
		t.Errorf("after the stale tap, on turn = %s, want %s", got.GetCurrentCombatantId(), second)
	}
	// A key reused for another kind of change is refused.
	_, err = f.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), IdempotencyKey: key,
	}))
	wantCode(t, "BeginCombat(reused key)", err, connect.CodeInvalidArgument)

	// Each change is one session event, and the retry wrote none.
	var turns int
	if err := f.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = 'turn_ended'`).Scan(&turns); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if turns != 1 {
		t.Errorf("turn_ended events = %d, want 1", turns)
	}
}

func TestStartEncounterNeedsAGrid(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	start := func(point string) error {
		_, err := f.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
			CampaignId: f.campaignID, IdempotencyKey: newKey(), Name: "Emboscada", MapPointId: point,
			Participants: []*playv1.Participant{{CharacterId: f.goblin.GetId()}},
		}))
		return err
	}
	bare := f.h.newMap(f.campaignID, 0)
	setCurrent := func(id string) {
		t.Helper()
		if _, err := f.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: f.campaignID, MapId: id})); err != nil {
			t.Fatalf("SetCurrentMap() error = %v", err)
		}
	}

	// A map without a grid: the master sets one first (E6-02).
	setCurrent(bare)
	b := wantEncounterBlocked(t, start(""), playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MAP_HAS_NO_GRID)
	if b.GetMapId() != bare {
		t.Errorf("MAP_HAS_NO_GRID map_id = %q, want %q", b.GetMapId(), bare)
	}
	// No current map at all.
	setCurrent("")
	wantEncounterBlocked(t, start(""), playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_CURRENT_MAP)

	// A battle point that names a gridded map: it becomes the current map, and
	// it is revealed.
	hidden := f.h.newMap(f.campaignID, 10)
	var point string
	if err := f.h.pool.QueryRow(t.Context(),
		`INSERT INTO map_points (map_id, kind, name, x_bp, y_bp, target_map_id, created_at, updated_at)
		 VALUES ($1, 'battle', 'Emboscada', 100, 100, $2, now(), now()) RETURNING id`, bare, hidden).Scan(&point); err != nil {
		t.Fatalf("insert point: %v", err)
	}
	res, err := f.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: f.campaignID, IdempotencyKey: newKey(), Name: "Emboscada", MapPointId: point,
		Participants: []*playv1.Participant{{CharacterId: f.goblin.GetId()}},
	}))
	if err != nil {
		t.Fatalf("StartEncounter(from the point) error = %v", err)
	}
	if res.Msg.GetEncounter().GetMapId() != hidden || res.Msg.GetEncounter().GetMapPointId() != point || res.Msg.GetEncounter().GetGridColumns() != 10 {
		t.Errorf("combat = %v, want it on the point's map with its grid", res.Msg.GetEncounter())
	}
	if live := f.ana.liveSession(t, f.campaignID); live.GetCurrentMapId() != hidden {
		t.Errorf("current map = %q, want the point's map %q", live.GetCurrentMapId(), hidden)
	}
	var revealed bool
	if err := f.h.pool.QueryRow(t.Context(), `SELECT revealed_at IS NOT NULL FROM maps WHERE id = $1`, hidden).Scan(&revealed); err != nil || !revealed {
		t.Errorf("the point's map revealed = %v (%v), want true", revealed, err)
	}
	// The party joined with the goblin; and a second start is refused.
	if len(res.Msg.GetEncounter().GetCombatants()) != 3 {
		t.Errorf("combatants = %v, want the party and the goblin", labels(res.Msg.GetEncounter()))
	}
	wantEncounterBlocked(t, start(""), playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENCOUNTER_ALREADY_OPEN)
}

func TestMR013_CombatantsStartOnTheirTokensAndEndWhereTheyStand(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	f.h.placeToken(f.mapID, f.pens.GetId(), 2600, 5100) // square 5, 5 of the 20 x 10 grid
	f.h.roller.queue(1)
	e := f.startFight(t, 1, false)
	if got := byLabel(t, e, "Pensantus"); !got.GetPlaced() || got.GetCol() != 5 || got.GetRow() != 5 {
		t.Fatalf("Pensantus = %v, want her on the square of her token (5, 5)", got)
	}
	if byLabel(t, e, "Toren").GetPlaced() {
		t.Error("Toren has no token: he starts unplaced")
	}
	e = f.mustMove(t, f.master, e, "Pensantus", 10, 2)
	e = f.mustMove(t, f.master, e, "Toren", 0, 0)

	// Ending the session ends the combat and moves the tokens to the middle of
	// the squares where the player characters stand.
	f.master.end(t, f.session)
	if got := f.latest(t); got.GetStatus() != playv1.EncounterStatus_ENCOUNTER_STATUS_ENDED || got.GetEndedAt() == nil {
		t.Fatalf("encounter after the session ended = %v, want ENDED", got)
	}
	tokenAt := func(character string) (x, y int) {
		t.Helper()
		if err := f.h.pool.QueryRow(t.Context(), `SELECT x_bp, y_bp FROM map_tokens WHERE map_id = $1 AND character_id = $2`, f.mapID, character).Scan(&x, &y); err != nil {
			t.Fatalf("read token: %v", err)
		}
		return x, y
	}
	if x, y := tokenAt(f.pens.GetId()); x != 5250 || y != 2500 { // (10.5 / 20, 2.5 / 10)
		t.Errorf("Pensantus's token = %d, %d; want 5250, 2500", x, y)
	}
	if x, y := tokenAt(f.tor.GetId()); x != 250 || y != 500 {
		t.Errorf("Toren's token = %d, %d; want 250, 500 (created: he had none)", x, y)
	}
	_ = e
}

// latest reads the status of the session's latest combat straight from the
// table, for after the session ended (GetEncounter needs an open one).
func (f *fight) latest(t *testing.T) *playv1.Encounter {
	t.Helper()
	var status string
	var ended bool
	if err := f.h.pool.QueryRow(t.Context(),
		`SELECT status, ended_at IS NOT NULL FROM encounters WHERE game_session_id = $1 ORDER BY created_at DESC LIMIT 1`, f.session.GetId()).
		Scan(&status, &ended); err != nil {
		t.Fatalf("read the encounter: %v", err)
	}
	out := &playv1.Encounter{Status: statusToProto[status]}
	if ended {
		out.EndedAt = timestampOrNil(new(f.h.svc.now()))
	}
	return out
}

func TestMR013_RemoveAndEnd(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	f.h.roller.queue(18, 5)
	e := f.startFight(t, 2, false)
	f.submitAll(t, e)
	e, err := f.begin(t, f.get(t, f.master))
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	g1 := byLabel(t, e, "Goblin 1")
	if e.GetCurrentCombatantId() != g1.GetId() {
		t.Fatalf("on turn = %s, want Goblin 1 (18)", e.GetCurrentCombatantId())
	}
	// Removing the combatant on turn passes the turn to the next one.
	res, err := f.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), CombatantId: g1.GetId(), IdempotencyKey: newKey(),
	}))
	if err != nil {
		t.Fatalf("RemoveCombatant() error = %v", err)
	}
	e = res.Msg.GetEncounter()
	if len(e.GetCombatants()) != 3 || e.GetCurrentCombatantId() == "" || e.GetCurrentCombatantId() == g1.GetId() {
		t.Fatalf("after removing the combatant on turn: %v, current %s", labels(e), e.GetCurrentCombatantId())
	}
	// A player's combatant cannot leave a running combat.
	_, err = f.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: f.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, e, "Toren").GetId(), IdempotencyKey: newKey(),
	}))
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_PLAYER_IN_COMBAT)

	end := func(key string) *playv1.Encounter {
		res, err := f.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{
			CampaignId: f.campaignID, EncounterId: e.GetId(), IdempotencyKey: key,
		}))
		if err != nil {
			t.Fatalf("EndEncounter() error = %v", err)
		}
		return res.Msg.GetEncounter()
	}
	ended := end(newKey())
	if ended.GetStatus() != playv1.EncounterStatus_ENCOUNTER_STATUS_ENDED || ended.GetCurrentCombatantId() != "" {
		t.Fatalf("ended combat = %v", ended)
	}
	end(newKey()) // ending twice is not an error
	// The ended combat can be read, and a new one can start.
	if got := f.get(t, f.ana); got.GetStatus() != playv1.EncounterStatus_ENCOUNTER_STATUS_ENDED {
		t.Errorf("GetEncounter after the end = %v, want ENDED", got.GetStatus())
	}
	if n := f.startFight(t, 1); n.GetId() == e.GetId() || n.GetStatus() != playv1.EncounterStatus_ENCOUNTER_STATUS_SETUP {
		t.Errorf("new combat = %v, want another one in SETUP", n)
	}
}

// TestMR013_TheMasterRestartsAnOrphanedTurn: when the combatant on turn
// leaves the fight with nobody to pass the turn to, or its character is
// deleted mid-fight, nobody is on turn; the master starts the turns again
// from the top of the order, and a player cannot.
func TestMR013_TheMasterRestartsAnOrphanedTurn(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	f.h.roller.queue(18, 5)
	e := f.startFight(t, 2, false)
	f.submitAll(t, e)
	e, err := f.begin(t, f.get(t, f.master))
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	g1 := byLabel(t, e, "Goblin 1")
	if e.GetCurrentCombatantId() != g1.GetId() {
		t.Fatalf("on turn = %s, want Goblin 1 (18)", e.GetCurrentCombatantId())
	}

	// The goblins' character is deleted mid-fight: both combatants go with
	// it (ON DELETE CASCADE), the one on turn included.
	if _, err := f.h.pool.Exec(t.Context(), "DELETE FROM characters WHERE id = $1", f.goblin.GetId()); err != nil {
		t.Fatalf("delete the goblin: %v", err)
	}
	e = f.get(t, f.master)
	if e.GetCurrentCombatantId() != "" || len(e.GetCombatants()) != 2 {
		t.Fatalf("after the delete: current %q, %v; want nobody on turn and the two players", e.GetCurrentCombatantId(), labels(e))
	}

	// A player cannot restart the turns, even with the stale ID.
	_, err = f.endTurn(t, f.ana, e, g1.GetId(), newKey())
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("a player's EndTurn with nobody on turn: %v, want permission_denied", err)
	}
	// The master can, with the stale screen's ID or with none: the first in
	// the order takes the turn, in the same round.
	e, err = f.endTurn(t, f.master, e, g1.GetId(), newKey())
	if err != nil {
		t.Fatalf("the master's EndTurn with nobody on turn: %v", err)
	}
	if first := e.GetCombatants()[0]; e.GetCurrentCombatantId() != first.GetId() || e.GetRound() != 1 {
		t.Fatalf("after the restart: current %s round %d, want %s in round 1", e.GetCurrentCombatantId(), e.GetRound(), first.GetLabel())
	}
	// Once someone is on turn, an empty expected ID is a stale screen.
	if _, err := f.endTurn(t, f.master, e, "", newKey()); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("EndTurn with an empty expected ID while someone is on turn: %v, want aborted", err)
	}
}

func TestMR013_GetEncounterWithoutOneIsEmpty(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	if got := f.get(t, f.ana); got != nil {
		t.Errorf("GetEncounter() = %v, want none", got)
	}
	// And with no open session it is NO_OPEN_SESSION.
	f.master.end(t, f.session)
	_, err := f.ana.combat.GetEncounter(t.Context(), connect.NewRequest(&playv1.GetEncounterRequest{CampaignId: f.campaignID}))
	wantCode(t, "GetEncounter(no session)", err, connect.CodeFailedPrecondition)
}

// TestMR013_CombatAuthorizationMatrix: for every CombatService method, who may
// call it. The master: all. A player: only what is about their own character
// (SubmitInitiative, EndTurn, MoveCombatant) and the read. Someone outside
// the campaign, and a pending member, get not_found; signed out gets
// unauthenticated.
func TestMR013_CombatAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	h := f.h
	f.h.roller.queue(1)
	e := f.startFight(t, 1, false)
	// The combat runs, so every call gets past its state check to the one
	// about who is calling.
	f.submitAll(t, e)
	e, err := f.begin(t, f.get(t, f.master))
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	outsider := h.newUser("Intruso")
	pending := h.newUser("Pendente")
	h.joinPending(f.master, f.campaignID, pending)
	pending.createCharacter(t, f.campaignID, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Esperando")
	anonymous := h.anonymous()

	id := newKey()
	pens, tor, gob := byLabel(t, e, "Pensantus").GetId(), byLabel(t, e, "Toren").GetId(), byLabel(t, e, "Goblin").GetId()
	campaign, enc := f.campaignID, e.GetId()
	calls := map[string]func(u *user, ctx context.Context) error{
		"StartEncounter": func(u *user, ctx context.Context) error {
			_, err := u.combat.StartEncounter(ctx, connect.NewRequest(&playv1.StartEncounterRequest{CampaignId: campaign, IdempotencyKey: newKey(), Name: "x", Participants: []*playv1.Participant{{CharacterId: f.goblin.GetId()}}}))
			return err
		},
		"GetEncounter": func(u *user, ctx context.Context) error {
			_, err := u.combat.GetEncounter(ctx, connect.NewRequest(&playv1.GetEncounterRequest{CampaignId: campaign}))
			return err
		},
		"SetInitiativeOrder": func(u *user, ctx context.Context) error {
			_, err := u.combat.SetInitiativeOrder(ctx, connect.NewRequest(&playv1.SetInitiativeOrderRequest{CampaignId: campaign, EncounterId: enc, IdempotencyKey: newKey(), CombatantIds: []string{pens, tor}}))
			return err
		},
		"BeginCombat": func(u *user, ctx context.Context) error {
			_, err := u.combat.BeginCombat(ctx, connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: campaign, EncounterId: enc, IdempotencyKey: newKey()}))
			return err
		},
		"GetMoveOptions": func(u *user, ctx context.Context) error {
			_, err := u.combat.GetMoveOptions(ctx, connect.NewRequest(&playv1.GetMoveOptionsRequest{CampaignId: campaign, EncounterId: enc, CombatantId: pens}))
			return err
		},
		"SpendMovement": func(u *user, ctx context.Context) error {
			_, err := u.combat.SpendMovement(ctx, connect.NewRequest(&playv1.SpendMovementRequest{CampaignId: campaign, EncounterId: enc, CombatantId: pens, IdempotencyKey: newKey(), DistanceFt: 5}))
			return err
		},
		"OfferOpportunity": func(u *user, ctx context.Context) error {
			_, err := u.combat.OfferOpportunity(ctx, connect.NewRequest(&playv1.OfferOpportunityRequest{CampaignId: campaign, EncounterId: enc, IdempotencyKey: newKey(), MoverId: gob, ReactorId: pens}))
			return err
		},
		"WithdrawOpportunity": func(u *user, ctx context.Context) error {
			_, err := u.combat.WithdrawOpportunity(ctx, connect.NewRequest(&playv1.WithdrawOpportunityRequest{CampaignId: campaign, EncounterId: enc, OpportunityOfferId: id, IdempotencyKey: newKey()}))
			return err
		},
		"SetCombatantSide": func(u *user, ctx context.Context) error {
			_, err := u.combat.SetCombatantSide(ctx, connect.NewRequest(&playv1.SetCombatantSideRequest{CampaignId: campaign, EncounterId: enc, CombatantId: gob, IdempotencyKey: newKey(), Side: playv1.CombatantSide_COMBATANT_SIDE_PARTY}))
			return err
		},
		"SetCombatantCover": func(u *user, ctx context.Context) error {
			_, err := u.combat.SetCombatantCover(ctx, connect.NewRequest(&playv1.SetCombatantCoverRequest{CampaignId: campaign, EncounterId: enc, CombatantId: gob, IdempotencyKey: newKey(), Cover: playv1.CoverDegree_COVER_DEGREE_HALF}))
			return err
		},
		"SetCombatantHidden": func(u *user, ctx context.Context) error {
			_, err := u.combat.SetCombatantHidden(ctx, connect.NewRequest(&playv1.SetCombatantHiddenRequest{CampaignId: campaign, EncounterId: enc, CombatantId: gob, IdempotencyKey: newKey(), Hidden: false}))
			return err
		},
		"AddCombatants": func(u *user, ctx context.Context) error {
			_, err := u.combat.AddCombatants(ctx, connect.NewRequest(&playv1.AddCombatantsRequest{CampaignId: campaign, EncounterId: enc, IdempotencyKey: newKey(), Participants: []*playv1.Participant{{CharacterId: f.goblin.GetId()}}}))
			return err
		},
		"AddMonsters": func(u *user, ctx context.Context) error {
			_, err := u.combat.AddMonsters(ctx, connect.NewRequest(&playv1.AddMonstersRequest{CampaignId: campaign, EncounterId: enc, IdempotencyKey: newKey(), CreatureKey: bandit, Count: 1}))
			return err
		},
		"RemoveCombatant": func(u *user, ctx context.Context) error {
			_, err := u.combat.RemoveCombatant(ctx, connect.NewRequest(&playv1.RemoveCombatantRequest{CampaignId: campaign, EncounterId: enc, CombatantId: id, IdempotencyKey: newKey()}))
			return err
		},
		"EndEncounter": func(u *user, ctx context.Context) error {
			_, err := u.combat.EndEncounter(ctx, connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: campaign, EncounterId: id, IdempotencyKey: newKey()}))
			return err
		},
		"SubmitInitiative(Pensantus)": func(u *user, ctx context.Context) error {
			_, err := u.combat.SubmitInitiative(ctx, connect.NewRequest(&playv1.SubmitInitiativeRequest{CampaignId: campaign, EncounterId: enc, CombatantId: pens, IdempotencyKey: newKey(), Roll: &playv1.SubmitInitiativeRequest_RollInApp{RollInApp: true}}))
			return err
		},
		"EndTurn": func(u *user, ctx context.Context) error {
			_, err := u.combat.EndTurn(ctx, connect.NewRequest(&playv1.EndTurnRequest{CampaignId: campaign, EncounterId: enc, ExpectedCombatantId: pens, IdempotencyKey: newKey()}))
			return err
		},
		"MoveCombatant(Pensantus)": func(u *user, ctx context.Context) error {
			_, err := u.combat.MoveCombatant(ctx, connect.NewRequest(&playv1.MoveCombatantRequest{CampaignId: campaign, EncounterId: enc, CombatantId: pens, IdempotencyKey: newKey(), Col: 1, Row: 1}))
			return err
		},
	}
	methods := playv1.File_meurpg_play_v1_combat_proto.Services().ByName("CombatService").Methods()
	covered := map[string]bool{}
	for name := range calls {
		covered[strings.SplitN(name, "(", 2)[0]] = true
	}
	// The actions of a turn, the spells and the highlights have their own
	// matrices (combat_actions_test.go, combat_spells_test.go,
	// highlights_test.go).
	if want := methods.Len() - len(actionRPCs) - len(spellRPCs) - len(highlightRPCs) - len(opportunityRPCs); len(covered) != want {
		t.Errorf("the matrix covers %d methods, the service has %d besides the actions of a turn", len(covered), want)
	}

	masterOnly := map[string]bool{
		"StartEncounter": true, "SetInitiativeOrder": true, "BeginCombat": true, "SetCombatantHidden": true,
		"AddCombatants": true, "AddMonsters": true, "RemoveCombatant": true, "EndEncounter": true, "SetCombatantSide": true, "SetCombatantCover": true,
		"OfferOpportunity": true, "WithdrawOpportunity": true,
	}
	for name, call := range calls {
		if err := call(anonymous, t.Context()); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: %v, want unauthenticated", name, err)
		}
		for who, u := range map[string]*user{"outsider": outsider, "pending": pending} {
			if err := call(u, t.Context()); connect.CodeOf(err) != connect.CodeNotFound {
				t.Errorf("%s as %s: %v, want not_found", name, who, err)
			}
		}
		// Toren's player, on Pensantus's (or the master's) things.
		err := call(f.caio, t.Context())
		switch {
		case masterOnly[name]:
			if connect.CodeOf(err) != connect.CodePermissionDenied {
				t.Errorf("%s as a player: %v, want permission_denied", name, err)
			}
		case name == "GetEncounter":
			if err != nil {
				t.Errorf("%s as a player: %v, want ok", name, err)
			}
		default: // another player's combatant
			if connect.CodeOf(err) != connect.CodePermissionDenied && connect.CodeOf(err) != connect.CodeAborted {
				t.Errorf("%s as another player: %v, want permission_denied", name, err)
			}
		}
	}
	// The owner is never told "not yours"; only the rules may refuse. In this
	// running combat, the initiative is already set (failed_precondition).
	for _, name := range []string{"SubmitInitiative(Pensantus)", "MoveCombatant(Pensantus)", "EndTurn"} {
		if err := calls[name](f.ana, t.Context()); connect.CodeOf(err) == connect.CodePermissionDenied || connect.CodeOf(err) == connect.CodeNotFound {
			t.Errorf("%s as the character's player: %v, want it past authorization", name, err)
		}
	}
	// And the master is never turned away by authorization either: the call
	// is refused, if at all, by the state of the combat or its arguments.
	for name, call := range calls {
		if err := call(f.master, t.Context()); connect.CodeOf(err) == connect.CodePermissionDenied || connect.CodeOf(err) == connect.CodeUnauthenticated {
			t.Errorf("%s as the master: %v, want it past authorization", name, err)
		}
	}
}

// TestMR013_CombatEventsPerAudience: the stream tells each audience only what
// it may see (D9). A hidden goblin's move reaches the master only; its turn
// reaches the players as the master's.
func TestMR013_CombatEventsPerAudience(t *testing.T) {
	t.Parallel()
	f := newFight(t)
	f.h.roller.queue(18) // the hidden goblin plays first
	e := f.startFight(t, 1)
	f.submitAll(t, e)
	e = f.get(t, f.master)
	f.mustMove(t, f.master, e, "Pensantus", 3, 3)

	masterStream, playerStream := f.master.watch(t, f.campaignID), f.ana.watch(t, f.campaignID)
	masterStream.ready(t)
	playerStream.ready(t)

	// The hidden goblin moves: only the master hears.
	f.mustMove(t, f.master, e, "Goblin", 7, 7)
	ev := masterStream.nextChange(t)
	if m := ev.GetCombatantMoved(); m == nil || m.GetCol() != 7 || m.GetRow() != 7 {
		t.Fatalf("master's event = %v, want combatant_moved to 7, 7", ev)
	}
	// Then a visible one moves; the player's next event is that one.
	f.mustMove(t, f.master, e, "Pensantus", 4, 3)
	if m := playerStream.nextChange(t).GetCombatantMoved(); m == nil || m.GetCol() != 4 {
		t.Fatalf("the player's first combatant_moved is not Pensantus's: a hidden move leaked")
	}
	if m := masterStream.nextChange(t).GetCombatantMoved(); m == nil || m.GetCol() != 4 {
		t.Fatalf("the master's second event is not Pensantus's move")
	}

	// The turns begin with the hidden goblin on turn.
	if _, err := f.begin(t, e); err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	goblin := byLabel(t, e, "Goblin").GetId()
	var gotTurn, gotChanged bool
	for range 2 {
		ev := masterStream.nextChange(t)
		if tc := ev.GetTurnChanged(); tc != nil {
			gotTurn = tc.GetCurrentCombatantId() == goblin && !tc.GetMasterTurn() && tc.GetRound() == 1
		}
		if ev.GetEncounterChanged() != nil {
			gotChanged = true
		}
	}
	if !gotTurn || !gotChanged {
		t.Errorf("master got turn_changed %v, encounter_changed %v; want both, with the goblin on turn", gotTurn, gotChanged)
	}
	gotTurn, gotChanged = false, false
	for range 2 {
		ev := playerStream.nextChange(t)
		if tc := ev.GetTurnChanged(); tc != nil {
			gotTurn = tc.GetCurrentCombatantId() == "" && tc.GetMasterTurn()
		}
		if ev.GetEncounterChanged() != nil {
			gotChanged = true
		}
		if strings.Contains(protojson.Format(ev), goblin) {
			t.Fatalf("the player's stream has the hidden goblin's ID: %v", ev)
		}
	}
	if !gotTurn || !gotChanged {
		t.Errorf("player got turn_changed (master's turn) %v, encounter_changed %v; want both", gotTurn, gotChanged)
	}
}

// TestSessionEventKindsMatchTheTable: session_event_kinds, the table that
// session_events.kind points at, holds exactly the kinds the module writes. A
// new kind is an INSERT in a migration (it used to rewrite a CHECK); add it both
// here and there. (migrations.TestEveryEventKindOfTheCodeIsInTheTable also scans
// every package's `event...` constants.)
func TestSessionEventKindsMatchTheTable(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	want := []string{
		eventCharacterVitalsAdjusted,
		eventEncounterStarted, eventInitiativeSubmitted, eventInitiativeOrderSet, eventCombatBegun,
		eventTurnEnded, eventCombatantMoved, eventCombatantHiddenSet, eventCombatantsAdded,
		eventCombatantRemoved, eventEncounterEnded,
		eventAttackRolled, eventDamageRolled, eventDamageApplied, eventDamageDiscarded,
		eventActionTaken, eventHitPointsAdjusted, eventActionUndone,
		eventSpellCast, eventReactionUsed, eventReactionDeclined, eventDeathSaveRolled, eventDeathConfirmed, eventConditionsSet,
		eventXPAwarded, eventXPAwardUndone, eventMilestoneMarked, eventSceneOpened, eventSceneClosed, eventSceneCheckRolled,
		eventClueRevealed, eventStageChanged,
		eventSceneAttemptGranted, eventTurnPartEnded,
		eventTrapNoticed, eventTrapSearched, eventTrapTriggered, eventTrapDisarmed, eventTrapRevealed,
		eventTreasureFound, eventTreasureUnfound, eventCoverSet, eventSideSet, eventOpportunityOffered,
		eventCreatureSummoned, eventCreatureDismissed, eventWildShapeStarted, eventWildShapeEnded,
		eventFamiliarSight, eventDoorOpened,
		eventPuzzleShown, eventPuzzleSolved, eventPuzzleReset, eventPuzzleClosed,
	}
	rows, err := h.pool.Query(t.Context(), `SELECT kind FROM session_event_kinds`)
	if err != nil {
		t.Fatalf("read the kinds: %v", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("read the kinds: %v", err)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("session_event_kinds holds %v, the code writes %v", got, want)
	}
}

// RemoveCombatant refuses while a damage the combatant attacked with, or took, is still to roll
// or to apply: it would vanish with the combatant, leaving an attack whose damage never lands.
func TestRemoveCombatantRefusesWhileItsDamageWaits(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId(), Hidden: new(false)}},
		npcRolls: []int{19},
		players:  map[string]int32{"Toren": 12, "Pensantus": 8, "Brisa": 1},
		at:       map[string][2]int32{"Capitão Goblin": {6, 3}, "Toren": {5, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	a.h.roller.queue(15, 4)
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Toren", inAppRoll)
	pend := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	if pend.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED {
		t.Fatalf("the damage = %v, want rolled and waiting for the master", pend)
	}
	remove := func() error {
		_, err := a.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Capitão Goblin"), IdempotencyKey: newKey(),
		}))
		return err
	}
	err := remove()
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("RemoveCombatant(the attacker of a waiting damage) error = %v, want failed_precondition", err)
	}
	var left int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM pending_damages WHERE id = $1`, pend.GetId()).Scan(&left); err != nil {
		t.Fatalf("count pending damages: %v", err)
	}
	if left != 1 {
		t.Errorf("the refused removal left %d rows of the waiting damage, want 1", left)
	}
	// Once the master settles the damage, the combatant leaves.
	if _, err := a.settle(t, a.master, e, pend.GetId(), false); err != nil {
		t.Fatalf("discarding the damage error = %v", err)
	}
	if err := remove(); err != nil {
		t.Errorf("RemoveCombatant after the damage was settled error = %v, want nil", err)
	}
}

// TestTheLatestEncounterIsTheOpenOneWhateverTheClockSays: the session's latest combat is the
// open one even when the clock that stamped it was behind the one of the combat that ended
// (a clock that stepped back, or two combats within the same instant).
func TestTheLatestEncounterIsTheOpenOneWhateverTheClockSays(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	clock := &movableClock{t: time.Now().Truncate(time.Microsecond)}
	a.h.svc.now = clock.now
	session := a.master.liveSession(t, a.campaignID).GetGameSession().GetId()
	latest := func() string {
		t.Helper()
		enc, err := a.h.svc.queries.GetLatestEncounter(t.Context(), session)
		if err != nil {
			t.Fatalf("GetLatestEncounter() error = %v", err)
		}
		return enc.ID
	}
	first := a.threeAndAGoblin(t)
	if got := latest(); got != first.GetId() {
		t.Fatalf("latest encounter = %q, want the first, %q", got, first.GetId())
	}
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: first.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	clock.advance(-time.Hour)
	second := a.threeAndAGoblin(t)
	if got := latest(); got != second.GetId() {
		t.Errorf("latest encounter = %q, want the open one, %q", got, second.GetId())
	}
}
