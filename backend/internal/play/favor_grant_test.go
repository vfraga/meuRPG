package play

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The Inspiration the master grants (favor_*.go): who may grant it, what a grant writes and tells, who reads
// who has it, and the refusals.

// favorParty is the party of newEffectsParty (Toren is Caio's, Pensantus is Ana's, Brisa is Bia's, two NPCs) in an
// open session.
type favorParty struct {
	*armed
	sessionID string
}

func newFavorParty(t *testing.T) *favorParty {
	t.Helper()
	a := newEffectsParty(t)
	open, err := a.h.svc.queries.GetOpenGameSession(t.Context(), a.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	return &favorParty{armed: a, sessionID: open.ID}
}

// reservedCharacter is a player character the master made for a player to claim: nobody owns it.
func (f *favorParty) reservedCharacter(t *testing.T) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
		RaceKey:    "race:human",
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 1}},
	}}}
	res, err := f.master.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: f.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Reservada", Sheet: sheet, ForPlayer: true,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(for a player) error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// seq is the session's hint number.
func (f *favorParty) seq(t *testing.T) int64 {
	t.Helper()
	n, err := f.h.svc.queries.GetHintSeq(t.Context(), f.sessionID)
	if err != nil {
		t.Fatalf("GetHintSeq() error = %v", err)
	}
	return n
}

// count is how many rows the query counts, in the campaign.
func (f *favorParty) count(t *testing.T, query string) int {
	t.Helper()
	var n int
	if err := f.h.pool.QueryRow(t.Context(), query, f.campaignID).Scan(&n); err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

func (f *favorParty) rows(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM character_favor WHERE campaign_id = $1`)
}

func (f *favorParty) grantedEvents(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM session_events WHERE kind = 'favor_granted' AND game_session_id IN (SELECT id FROM game_sessions WHERE campaign_id = $1)`)
}

// grantAs sends GrantFavor as u.
func (f *favorParty) grantAs(u *user, characterID, key string) error {
	_, err := u.favor.GrantFavor(f.h.t.Context(), connect.NewRequest(&playv1.GrantFavorRequest{CampaignId: f.campaignID, CharacterId: characterID, IdempotencyKey: key}))
	return err
}

// grant is the master's GrantFavor with a new key.
func (f *favorParty) grant(characterID string) error {
	return f.grantAs(f.master, characterID, newKey())
}

// holders is what u reads of GetFavor.
func (f *favorParty) holders(t *testing.T, u *user) []*playv1.FavorHolder {
	t.Helper()
	res, err := u.favor.GetFavor(t.Context(), connect.NewRequest(&playv1.GetFavorRequest{CampaignId: f.campaignID}))
	if err != nil {
		t.Fatalf("GetFavor() error = %v", err)
	}
	return res.Msg.GetHolders()
}

// wantFavorBlocked checks that err is a failed_precondition with the FavorBlocked detail of the reason.
func wantFavorBlocked(t *testing.T, call string, err error, reason playv1.FavorBlockedReason) {
	t.Helper()
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("%s error = %v, want failed_precondition", call, err)
	}
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		t.Fatalf("%s error = %v, want a Connect error", call, err)
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*playv1.FavorBlocked); ok {
				if b.GetReason() != reason {
					t.Fatalf("%s blocked by %v, want %v", call, b.GetReason(), reason)
				}
				return
			}
		}
	}
	t.Fatalf("%s error = %v, want a FavorBlocked detail (%v)", call, err, reason)
}

func TestGrantFavorGivesTheInspirationAndMovesTheNumberOnce(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	master, caio, ana := f.master.watch(t, f.campaignID), f.caio.watch(t, f.campaignID), f.ana.watch(t, f.campaignID)
	for _, w := range []*watcher{master, caio, ana} {
		w.ready(t)
	}
	before := f.seq(t)

	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	if got := f.seq(t); got != before+1 {
		t.Errorf("hint_seq = %d after a grant, want %d (one step)", got, before+1)
	}
	if got := f.rows(t); got != 1 {
		t.Errorf("character_favor has %d rows, want 1", got)
	}
	var payload []byte
	err := f.h.pool.QueryRow(t.Context(), `SELECT payload FROM session_events WHERE kind = 'favor_granted' AND game_session_id = $1`, f.sessionID).Scan(&payload)
	if err != nil {
		t.Fatalf("read the event: %v", err)
	}
	var got favorGrantedEvent
	if err := json.Unmarshal(payload, &got); err != nil || got.CharacterID != f.toren.GetId() {
		t.Errorf("favor_granted payload = %s (%v), want only the character's id %s", payload, err, f.toren.GetId())
	}

	f.master.markCurrentMap(t, f.campaignID, f.mapID, master, caio, ana)
	for who, w := range map[string]*watcher{"the master": master, "Caio (the owner)": caio} {
		heard := w.beforeMarker(t)
		if len(heard) != 1 || heard[0].GetVitalsChanged().GetVitals().GetCharacterId() != f.toren.GetId() || heard[0].GetSessionSeq() != before+1 {
			t.Errorf("%s heard %v, want one hint for Toren stamped %d", who, heard, before+1)
		}
	}
	if heard := ana.beforeMarker(t); len(heard) != 0 {
		t.Errorf("Ana (another player) heard %v, want nothing", heard)
	}
}

func TestGrantFavorWorksInACombatAndLeavesItAlone(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	e := f.threeAndAGoblin(t)
	if err := f.grant(f.pens.GetId()); err != nil {
		t.Fatalf("GrantFavor() in a combat error = %v", err)
	}
	after := f.get(t, f.master)
	if after.GetRevision() != e.GetRevision() || len(after.GetCombatants()) != len(e.GetCombatants()) {
		t.Errorf("the combat after a grant = revision %d with %d combatants, want %d with %d", after.GetRevision(), len(after.GetCombatants()), e.GetRevision(), len(e.GetCombatants()))
	}
	if h := f.holders(t, f.master); len(h) != 1 || h[0].GetCharacterId() != f.pens.GetId() {
		t.Errorf("GetFavor() = %v, want Pensantus", h)
	}
}

func TestGrantFavorRefusals(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	other := f.h.newUser("Outra mestra")
	elsewhere := f.h.newCampaign(other, "Outra", f.ana)
	foreign := f.ana.createCharacter(t, elsewhere, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "De outra mesa")
	reserved := f.reservedCharacter(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("the first grant error = %v", err)
	}
	seq, events := f.seq(t), f.grantedEvents(t)

	for name, id := range map[string]string{
		"an NPC":                      f.goblin.GetId(),
		"the captain, an NPC too":     f.capitao.GetId(),
		"a reserved character":        reserved.GetId(),
		"a character of another game": foreign.GetId(),
		"an id nobody has":            newKey(),
	} {
		wantFavorBlocked(t, name, f.grant(id), playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_A_PLAYER_CHARACTER)
	}
	wantFavorBlocked(t, "a character that has it", f.grant(f.toren.GetId()), playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ALREADY_HAS)

	if got := f.seq(t); got != seq {
		t.Errorf("hint_seq = %d after the refusals, want %d: a refusal moves no number", got, seq)
	}
	if got := f.grantedEvents(t); got != events {
		t.Errorf("%d favor_granted events after the refusals, want %d", got, events)
	}
	if got := f.rows(t); got != 1 {
		t.Errorf("character_favor has %d rows after the refusals, want 1", got)
	}
}

func TestGrantFavorAnswersTheSameForEveryoneThatIsNotAPlayerCharacter(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	var texts []string
	for _, id := range []string{f.goblin.GetId(), newKey()} {
		err := f.grant(id)
		wantFavorBlocked(t, "GrantFavor", err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_A_PLAYER_CHARACTER)
		texts = append(texts, err.Error())
	}
	if texts[0] != texts[1] {
		t.Errorf("the refusal for an NPC says %q and for a stranger's id %q: the answer must not tell them apart", texts[0], texts[1])
	}
}

// A combat names its fighters by combatant ids, and an NPC's copies share the NPC's character id: none of them is a player
// character, and the refusal is the one an id nobody has gets.
func TestGrantFavorRefusesTheCombatantsOfACombat(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	e := f.threeAndAGoblin(t)
	stranger := f.grant(newKey())
	wantFavorBlocked(t, "an id nobody has", stranger, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_A_PLAYER_CHARACTER)
	seq := f.seq(t)
	ids := map[string]string{}
	for _, c := range e.GetCombatants() {
		ids["the combatant "+c.GetLabel()] = c.GetId()
		if c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_NPC {
			ids["the character of "+c.GetLabel()] = c.GetCharacterId()
		}
	}
	if len(ids) <= len(e.GetCombatants()) {
		t.Fatalf("the fixture has no NPC combatant: %v", ids)
	}
	for name, id := range ids {
		err := f.grant(id)
		wantFavorBlocked(t, name, err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_A_PLAYER_CHARACTER)
		if err.Error() != stranger.Error() {
			t.Errorf("the refusal for %s says %q, and for an id nobody has %q", name, err, stranger)
		}
	}
	if got, rows := f.seq(t), f.rows(t); got != seq || rows != 0 {
		t.Errorf("hint_seq = %d (want %d) and %d rows (want 0) after the refusals", got, seq, rows)
	}
}

// A grant reaches every player character of the campaign that is out of the reserve, whatever its status: a dead one is
// granted too. Its vitals cannot be read, so nothing is told, and the number moves once all the same.
func TestGrantFavorReachesADeadCharacterAndMovesTheNumberOnce(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	f.execSQL(t, `UPDATE characters SET status = 'dead', died_at = now() WHERE id = $1`, f.toren.GetId())
	caio := f.caio.watch(t, f.campaignID)
	caio.ready(t)
	before := f.seq(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() to a dead character error = %v", err)
	}
	if got := f.seq(t); got != before+1 {
		t.Errorf("hint_seq = %d, want %d: one step for a grant whose hint cannot be told", got, before+1)
	}
	if got := f.grantedEvents(t); got != 1 {
		t.Errorf("%d favor_granted events, want 1", got)
	}
	if h := f.holders(t, f.master); len(h) != 1 || h[0].GetCharacterId() != f.toren.GetId() {
		t.Errorf("GetFavor() for the master = %v, want the dead character", h)
	}
	f.master.markCurrentMap(t, f.campaignID, f.mapID, caio)
	if heard := caio.beforeMarker(t); len(heard) != 0 {
		t.Errorf("the owner heard %v, want nothing: a dead character's vitals cannot be read", heard)
	}
}

func TestGrantFavorInputsAndRoles(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	outsider := f.h.newUser("Fora")
	for _, c := range []struct {
		name string
		by   *user
		id   string
		key  string
		code connect.Code
	}{
		{"a player", f.caio, f.toren.GetId(), newKey(), connect.CodePermissionDenied},
		{"someone outside the campaign", outsider, f.toren.GetId(), newKey(), connect.CodeNotFound},
		{"a key that is not a UUID", f.master, f.toren.GetId(), "nope", connect.CodeInvalidArgument},
		{"a character id that is not a UUID", f.master, "toren", newKey(), connect.CodeInvalidArgument},
	} {
		wantCode(t, c.name, f.grantAs(c.by, c.id, c.key), c.code)
	}
	if got := f.rows(t); got != 0 {
		t.Errorf("character_favor has %d rows after refused calls, want none", got)
	}
}

func TestGrantFavorNeedsAnOpenSession(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	f.master.end(t, f.master.liveSession(t, f.campaignID).GetGameSession())
	err := f.grant(f.toren.GetId())
	wantCode(t, "GrantFavor with no session", err, connect.CodeFailedPrecondition)
	if got := f.rows(t); got != 0 {
		t.Errorf("character_favor has %d rows, want none", got)
	}
}

func TestGrantFavorRetries(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	master := f.master.watch(t, f.campaignID)
	master.ready(t)
	key := newKey()
	if err := f.grantAs(f.master, f.toren.GetId(), key); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	seq := f.seq(t)

	if err := f.grantAs(f.master, f.toren.GetId(), key); err != nil {
		t.Errorf("the same request again error = %v, want it answered as the first call was", err)
	}
	if got := f.seq(t); got != seq {
		t.Errorf("hint_seq = %d after a retry, want %d: a replay moves no number", got, seq)
	}
	if rows, events := f.rows(t), f.grantedEvents(t); rows != 1 || events != 1 {
		t.Errorf("%d rows and %d events after a retry, want 1 and 1", rows, events)
	}
	// A retry tells what the first call may have died before telling, with the number as it stands.
	f.master.markCurrentMap(t, f.campaignID, f.mapID, master)
	heard := master.beforeMarker(t)
	if len(heard) != 2 || heard[1].GetSessionSeq() != seq {
		t.Errorf("the master heard %v, want the grant's hint and its replay, both on number %d", heard, seq)
	}

	wantCode(t, "the key for another character", f.grantAs(f.master, f.pens.GetId(), key), connect.CodeInvalidArgument)
	if got := f.rows(t); got != 1 {
		t.Errorf("character_favor has %d rows, want 1", got)
	}
}

// N masters' calls at once on the same character: one gets it, the rest are told it has one.
func TestGrantFavorRacesOnTheSameCharacter(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8) // the racers must overlap
	f := newFavorParty(t)
	const racers = 20
	before := f.seq(t)
	errs := make([]error, racers)
	var start, wg sync.WaitGroup
	start.Add(1)
	for i := range racers {
		wg.Go(func() {
			start.Wait()
			errs[i] = f.grant(f.toren.GetId())
		})
	}
	start.Done()
	wg.Wait()
	granted := 0
	for _, err := range errs {
		if err == nil {
			granted++
			continue
		}
		wantFavorBlocked(t, "a racing grant", err, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_ALREADY_HAS)
	}
	if granted != 1 {
		t.Errorf("%d of %d racing grants succeeded, want exactly 1", granted, racers)
	}
	if got := f.rows(t); got != 1 {
		t.Errorf("character_favor has %d rows, want 1", got)
	}
	if got := f.seq(t); got != before+1 {
		t.Errorf("hint_seq = %d, want %d: one step for the one grant", got, before+1)
	}
	if got := f.grantedEvents(t); got != 1 {
		t.Errorf("%d favor_granted events, want 1", got)
	}
}

func TestGrantFavorToTwoCharactersAtOnceGivesBoth(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8)
	f := newFavorParty(t)
	before := f.seq(t)
	ids := []string{f.toren.GetId(), f.pens.GetId()}
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() { errs[i] = f.grant(id) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("the grant to %s error = %v, want both to succeed", ids[i], err)
		}
	}
	if got := f.rows(t); got != 2 {
		t.Errorf("character_favor has %d rows, want 2", got)
	}
	if got := f.seq(t); got != before+2 {
		t.Errorf("hint_seq = %d, want %d: one step for each grant", got, before+2)
	}
}

func TestGetFavorShowsEachReaderTheirOwn(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	pending := f.h.newUser("Pendente")
	f.h.joinPending(f.master, f.campaignID, pending)
	outsider := f.h.newUser("Fora")
	if got := f.holders(t, f.master); len(got) != 0 {
		t.Fatalf("GetFavor() before any grant = %v, want none", got)
	}
	for _, c := range []*charactersv1.Character{f.toren, f.pens} {
		if err := f.grant(c.GetId()); err != nil {
			t.Fatalf("GrantFavor(%s) error = %v", c.GetName(), err)
		}
	}
	names := func(h []*playv1.FavorHolder) []string {
		var out []string
		for _, x := range h {
			out = append(out, x.GetCharacterName())
		}
		slices.Sort(out)
		return out
	}
	for who, c := range map[string]struct {
		u    *user
		want []string
	}{
		"the master":        {f.master, []string{"Pensantus", "Toren"}},
		"Caio (Toren's)":    {f.caio, []string{"Toren"}},
		"Ana (Pensantus's)": {f.ana, []string{"Pensantus"}},
		"Bia (has none)":    {f.bia, nil},
	} {
		if got := names(f.holders(t, c.u)); !slices.Equal(got, c.want) {
			t.Errorf("%s reads %v, want %v", who, got, c.want)
		}
	}
	for who, u := range map[string]*user{"someone outside": outsider, "a pending member": pending} {
		_, err := u.favor.GetFavor(t.Context(), connect.NewRequest(&playv1.GetFavorRequest{CampaignId: f.campaignID}))
		wantCode(t, "GetFavor as "+who, err, connect.CodeNotFound)
	}
}

func TestGetFavorNeedsNoSessionAndKeepsAnEndedOne(t *testing.T) {
	t.Parallel()
	f := newFavorParty(t)
	if err := f.grant(f.toren.GetId()); err != nil {
		t.Fatalf("GrantFavor() error = %v", err)
	}
	f.master.end(t, f.master.liveSession(t, f.campaignID).GetGameSession())
	if got := f.holders(t, f.caio); len(got) != 1 || got[0].GetCharacterName() != "Toren" {
		t.Errorf("GetFavor() after the session = %v, want Toren still holding it", got)
	}
}

// favorTable is who calls FavorService in the authorization matrix: the master, the owner of the character, another player of
// the campaign and someone outside it.
type favorTable struct {
	campaign, character                string
	master, owner, bystander, outsider *user
}

// checkFavorAuthorization is the rows of FavorService in TestAuthorizationMatrix: every method, asked of each kind of caller.
// The campaign has an open session. The calls name an id that is no pass and a giver that is its own receiver: PassFavor refuses SELF to a member, AnswerFavorPass and CancelFavorPass find no pass (a pass is never told to a caller who may not read it), and a stranger gets not_found.
func checkFavorAuthorization(t *testing.T, f favorTable) {
	t.Helper()
	type client = playv1connect.FavorServiceClient
	rows := []struct {
		name string
		call func(ctx context.Context, c client) error
		// master, owner, another player, someone outside the campaign
		want [4]connect.Code
	}{
		{"GrantFavor", func(ctx context.Context, c client) error {
			_, err := c.GrantFavor(ctx, connect.NewRequest(&playv1.GrantFavorRequest{CampaignId: f.campaign, CharacterId: f.character, IdempotencyKey: newKey()}))
			return err
		}, [4]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound}},
		{"GetFavor", func(ctx context.Context, c client) error {
			_, err := c.GetFavor(ctx, connect.NewRequest(&playv1.GetFavorRequest{CampaignId: f.campaign}))
			return err
		}, [4]connect.Code{allowed, allowed, allowed, connect.CodeNotFound}},
		{"PassFavor", func(ctx context.Context, c client) error {
			_, err := c.PassFavor(ctx, connect.NewRequest(&playv1.PassFavorRequest{CampaignId: f.campaign, FromCharacterId: f.character, ToCharacterId: f.character, IdempotencyKey: newKey()}))
			return err
		}, [4]connect.Code{connect.CodeFailedPrecondition, connect.CodeFailedPrecondition, connect.CodeFailedPrecondition, connect.CodeNotFound}},
		{"AnswerFavorPass", func(ctx context.Context, c client) error {
			_, err := c.AnswerFavorPass(ctx, connect.NewRequest(&playv1.AnswerFavorPassRequest{CampaignId: f.campaign, PassId: f.character, Accept: true, IdempotencyKey: newKey()}))
			return err
		}, [4]connect.Code{connect.CodeNotFound, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound}},
		{"CancelFavorPass", func(ctx context.Context, c client) error {
			_, err := c.CancelFavorPass(ctx, connect.NewRequest(&playv1.CancelFavorPassRequest{CampaignId: f.campaign, PassId: f.character, IdempotencyKey: newKey()}))
			return err
		}, [4]connect.Code{connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound}},
		{"AnswerFavorRefund", func(ctx context.Context, c client) error {
			_, err := c.AnswerFavorRefund(ctx, connect.NewRequest(&playv1.AnswerFavorRefundRequest{CampaignId: f.campaign, PromptId: f.character, GiveBack: true, IdempotencyKey: newKey()}))
			return err
		}, [4]connect.Code{connect.CodeNotFound, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound}},
	}
	covered := map[string]bool{}
	for _, r := range rows {
		covered[r.name] = true
	}
	methods := playv1.File_meurpg_play_v1_favor_proto.Services().ByName("FavorService").Methods()
	for i := range methods.Len() {
		if name := string(methods.Get(i).Name()); !covered[name] {
			t.Errorf("FavorService.%s is missing from the authorization matrix", name)
		}
	}
	callers := []struct {
		name   string
		client client
	}{
		{"master", f.master.favor},
		{"owner", f.owner.favor},
		{"another player", f.bystander.favor},
		{"non-member", f.outsider.favor},
	}
	for _, r := range rows {
		for i, caller := range callers {
			t.Run("FavorService "+r.name+"/"+caller.name, func(t *testing.T) {
				err := r.call(t.Context(), caller.client)
				switch want := r.want[i]; {
				case want == allowed && err != nil:
					t.Errorf("error = %v, want allowed", err)
				case want != allowed && connect.CodeOf(err) != want:
					t.Errorf("error = %v, want %v", err, want)
				}
			})
		}
	}
}
