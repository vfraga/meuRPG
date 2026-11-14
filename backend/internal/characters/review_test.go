package characters

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
)

// "Pedir ajustes" (RN-15, MR-024): the master sends a pending character back with a reason, the
// owner edits it and sends it again, and he approves or rejects at any time. The reason is
// personal data: only the master and the owner read it, and it is deleted in every way the
// character leaves the pending state.

// hostSpy is a ReviewHost that records what the module tells the streams.
type hostSpy struct {
	mu     sync.Mutex
	events []string // "event:character_id"
}

func (h *hostSpy) PublishCharacterEvent(_ context.Context, _, _, characterID, event string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event+":"+characterID)
}

func (*hostSpy) ReviveInCombat(context.Context, pgx.Tx, string, string, string, time.Time) (string, error) {
	return "", nil
}

func (*hostSpy) GearChanged(context.Context, pgx.Tx, string, string, sessiontell.Tell) error {
	return nil
}

func (*hostSpy) ArmorWorn(context.Context, pgx.Tx, string, string, sessiontell.Tell) error {
	return nil
}

func (*hostSpy) EquipInCombat(context.Context, pgx.Tx, string, string, string, EquipChange, sessiontell.Tell) error {
	return nil
}

func (*hostSpy) PublishEncounterChanged(context.Context, string, string) {}

func (h *hostSpy) told() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.events...)
}

// reviewTable is a campaign with a master, a pending player with her character, another player and
// another pending member.
type reviewTable struct {
	h                       *harness
	master, lia, other, eve *user
	campaign                string
	pc                      *charactersv1.Character
	host                    *hostSpy
}

// logBuffer is a log handler's destination that tests can read while the server writes.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func newReviewTable(t *testing.T) (*reviewTable, *logBuffer) {
	t.Helper()
	logs := &logBuffer{}
	h := newHarnessWith(t, func(c *Config) {
		c.Logger = slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})
	tb := &reviewTable{h: h, host: &hostSpy{}}
	h.svc.SetReviewHost(tb.host)
	tb.master, tb.lia, tb.other, tb.eve = h.newUser("Mestre"), h.newUser("Lia"), h.newUser("Outro"), h.newUser("Eva")
	tb.campaign = h.newCampaign(tb.master, "Mirathel", tb.other)
	h.joinPending(tb.master, tb.campaign, tb.lia, tb.eve)
	tb.pc = tb.lia.createPensantus(t, tb.campaign)
	return tb, logs
}

// ask calls RequestCharacterChanges as u.
func (u *user) ask(c *charactersv1.Character, reason, key string) (*charactersv1.Character, error) {
	res, err := u.api.RequestCharacterChanges(context.Background(), connect.NewRequest(&charactersv1.RequestCharacterChangesRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(), Reason: reason, IdempotencyKey: key,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

// resubmit calls ResubmitCharacter as u.
func (u *user) resubmit(c *charactersv1.Character, key string) (*charactersv1.Character, error) {
	res, err := u.api.ResubmitCharacter(context.Background(), connect.NewRequest(&charactersv1.ResubmitCharacterRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(), IdempotencyKey: key,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

// invalidField returns the field of an invalid_argument's InvalidField detail.
func invalidField(t *testing.T, call string, err error) string {
	t.Helper()
	wantCode(t, call, err, connect.CodeInvalidArgument)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		if v, err := d.Value(); err == nil {
			if f, ok := v.(*charactersv1.InvalidField); ok {
				return f.GetField()
			}
		}
	}
	return ""
}

// reviewRows counts the rows of character_reviews of the campaign.
func (h *harness) reviewRows(campaignID string) int {
	h.t.Helper()
	var n int
	if err := h.pool.QueryRow(h.t.Context(), "SELECT count(*) FROM character_reviews WHERE campaign_id = $1", campaignID).Scan(&n); err != nil {
		h.t.Fatalf("count the reviews: %v", err)
	}
	return n
}

func key() string { return uuid.New().String() }

// RN-15: the master asks for changes: the character stays pending, the owner and the master read
// the reason, the invite and the pending membership stay, and the stream is told.
func TestRN15_MasterAsksForChangesAndTheCharacterStaysPending(t *testing.T) {
	t.Parallel()
	tb, _ := newReviewTable(t)
	if got := tb.master.get(t, tb.campaign, tb.pc.GetId()); got.GetReview().GetStatus() != charactersv1.Review_REVIEW_AWAITING || !got.GetCanRequestChanges() || got.GetCanResubmit() {
		t.Fatalf("master before asking: review %v, can_request_changes %v, can_resubmit %v; want AWAITING, true, false", got.GetReview(), got.GetCanRequestChanges(), got.GetCanResubmit())
	}
	asked, err := tb.master.ask(tb.pc, "  O antecedente é Acólito, mas a história fala de um guarda.\nAjuste um dos dois.  ", key())
	if err != nil {
		t.Fatalf("RequestCharacterChanges() error = %v", err)
	}
	want := "O antecedente é Acólito, mas a história fala de um guarda.\nAjuste um dos dois."
	r := asked.GetReview()
	if asked.GetState() != charactersv1.CharacterState_CHARACTER_STATE_PENDING || !asked.GetCanApprove() ||
		r.GetStatus() != charactersv1.Review_REVIEW_CHANGES_REQUESTED || r.GetReason() != want || r.GetRequestedAt() == nil || r.GetResubmittedAt() != nil {
		t.Fatalf("RequestCharacterChanges() = state %v review %v, want PENDING, approvable, CHANGES_REQUESTED with the trimmed reason", asked.GetState(), r)
	}
	if got := tb.h.memberStatus(tb.campaign, tb.lia.id); got != "pending" {
		t.Errorf("her membership after the request = %q, want pending: the invite stays", got)
	}

	// She reads it and may edit and resubmit; she is still pending and editable.
	hers := tb.lia.get(t, tb.campaign, tb.pc.GetId())
	if hers.GetReview().GetReason() != want || hers.GetReview().GetStatus() != charactersv1.Review_REVIEW_CHANGES_REQUESTED || !hers.GetCanResubmit() ||
		!hers.GetCanEdit() || hers.GetCanRequestChanges() || hers.GetCanApprove() {
		t.Errorf("owner's GetCharacter() = review %v, can_resubmit %v, can_edit %v, can_request_changes %v; want the reason and the resubmit button",
			hers.GetReview(), hers.GetCanResubmit(), hers.GetCanEdit(), hers.GetCanRequestChanges())
	}
	if _, err := tb.lia.update(t, hers, "Pensantus, o Ajustado", pensantusSheet()); err != nil {
		t.Errorf("owner's UpdateCharacter() after the request error = %v, want it to edit", err)
	}
	// The list says it too, without the reason.
	for who, u := range map[string]*user{"master": tb.master, "owner": tb.lia} {
		list := u.list(t, tb.campaign)
		var found bool
		for _, c := range list {
			if c.GetId() == tb.pc.GetId() {
				found = true
				if c.GetReviewStatus() != charactersv1.Review_REVIEW_CHANGES_REQUESTED {
					t.Errorf("%s's ListCharacters() review_status = %v, want CHANGES_REQUESTED", who, c.GetReviewStatus())
				}
			}
		}
		if !found {
			t.Errorf("%s's ListCharacters() does not hold the pending character", who)
		}
	}
	if got, wantEvents := tb.host.told(), []string{"changes_requested:" + tb.pc.GetId()}; len(got) != 1 || got[0] != wantEvents[0] {
		t.Errorf("the stream was told %v, want %v", got, wantEvents)
	}
}

// RN-15: the reason is 1 to 500 characters after trimming, and only the master asks.
func TestRN15_TheReasonIsRequiredAndAtMost500Characters(t *testing.T) {
	t.Parallel()
	tb, _ := newReviewTable(t)
	for name, reason := range map[string]string{"empty": "", "only spaces": " \n\t  ", "501 characters": strings.Repeat("ã", 501)} {
		_, err := tb.master.ask(tb.pc, reason, key())
		if got := invalidField(t, name, err); got != "reason" {
			t.Errorf("%s: InvalidField = %q, want reason", name, got)
		}
	}
	if got := tb.h.reviewRows(tb.campaign); got != 0 {
		t.Fatalf("%d review rows after refused requests, want 0", got)
	}
	if _, err := tb.master.ask(tb.pc, strings.Repeat("ã", 500), key()); err != nil {
		t.Errorf("a reason of 500 characters error = %v, want it accepted", err)
	}
	_, err := tb.master.ask(tb.pc, "  "+strings.Repeat("a", 500)+"  ", key())
	if err != nil {
		t.Errorf("a reason of 500 characters between spaces error = %v, want the spaces not to count", err)
	}
	_, err = tb.master.ask(tb.pc, "Falta algo.", "")
	if got := invalidField(t, "no key", err); got != "idempotency_key" {
		t.Errorf("no key: InvalidField = %q, want idempotency_key", got)
	}
	_, err = tb.other.ask(tb.pc, "Eu.", key())
	wantCode(t, "a player's RequestCharacterChanges", err, connect.CodePermissionDenied)
}

// RN-15: asking applies to a pending character of a player only.
func TestRN15_OnlyAPendingPlayerCharacterCanBeSentBack(t *testing.T) {
	t.Parallel()
	tb, _ := newReviewTable(t)
	npc := tb.master.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Strahd", enemySheet())
	_, err := tb.master.ask(npc, "Mais sangue.", key())
	wantCode(t, "RequestCharacterChanges(an NPC)", err, connect.CodeInvalidArgument)

	tb.master.approve(t, tb.pc)
	_, err = tb.master.ask(tb.pc, "Tarde demais.", key())
	if d := blocked(t, "RequestCharacterChanges(approved)", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_PENDING {
		t.Errorf("CharacterBlocked = %v, want NOT_PENDING", d)
	}
	_, err = tb.master.ask(&charactersv1.Character{CampaignId: tb.campaign, Id: uuid.New().String()}, "Quem?", key())
	wantCode(t, "RequestCharacterChanges(unknown)", err, connect.CodeNotFound)
}

// RN-15: asking twice replaces the reason and the time; the same key is not a second request.
func TestRN15_AskingAgainReplacesTheReasonAndTheKeyDeduplicates(t *testing.T) {
	t.Parallel()
	tb, _ := newReviewTable(t)
	first := key()
	a, err := tb.master.ask(tb.pc, "Primeiro pedido.", first)
	if err != nil {
		t.Fatalf("first request error = %v", err)
	}
	// The same key and the same request: the first answer, and the stream hears nothing new.
	again, err := tb.master.ask(tb.pc, "Primeiro pedido.", first)
	if err != nil || !again.GetReview().GetRequestedAt().AsTime().Equal(a.GetReview().GetRequestedAt().AsTime()) {
		t.Fatalf("retry = %v, %v; want the first request back", again.GetReview(), err)
	}
	if got := len(tb.host.told()); got != 1 {
		t.Errorf("the stream was told %d times after a retry, want 1", got)
	}
	// The same key for another reason is refused.
	_, err = tb.master.ask(tb.pc, "Outro motivo.", first)
	wantCode(t, "a key reused for another request", err, connect.CodeInvalidArgument)
	if got := tb.master.get(t, tb.campaign, tb.pc.GetId()).GetReview().GetReason(); got != "Primeiro pedido." {
		t.Errorf("reason after the refused reuse = %q, want the first one", got)
	}

	// A new key replaces the reason and the time.
	tb.h.clock.Advance(time.Minute)
	second, err := tb.master.ask(tb.pc, "Segundo pedido.", key())
	if err != nil || second.GetReview().GetReason() != "Segundo pedido." ||
		!second.GetReview().GetRequestedAt().AsTime().After(a.GetReview().GetRequestedAt().AsTime()) {
		t.Fatalf("second request = %v, %v; want the new reason and a later time", second.GetReview(), err)
	}
	if got := tb.h.reviewRows(tb.campaign); got != 1 {
		t.Errorf("%d review rows after asking twice, want 1", got)
	}
}

// RN-15: the owner sends the character again; the reason stays for the master; the master asks again and
// the review is open again.
func TestRN15_OwnerResubmitsAndTheMasterCanAskAgain(t *testing.T) {
	t.Parallel()
	tb, _ := newReviewTable(t)

	_, err := tb.lia.resubmit(tb.pc, key())
	if d := blocked(t, "ResubmitCharacter(nothing asked)", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NO_CHANGES_REQUESTED {
		t.Errorf("CharacterBlocked = %v, want NO_CHANGES_REQUESTED", d)
	}
	if _, err := tb.master.ask(tb.pc, "Falta o equipamento.", key()); err != nil {
		t.Fatalf("RequestCharacterChanges() error = %v", err)
	}

	k := key()
	sent, err := tb.lia.resubmit(tb.pc, k)
	if err != nil {
		t.Fatalf("ResubmitCharacter() error = %v", err)
	}
	if sent.GetState() != charactersv1.CharacterState_CHARACTER_STATE_PENDING || sent.GetReview().GetStatus() != charactersv1.Review_REVIEW_RESUBMITTED ||
		sent.GetReview().GetResubmittedAt() == nil || sent.GetCanResubmit() || sent.GetReview().GetReason() != "Falta o equipamento." {
		t.Fatalf("ResubmitCharacter() = state %v review %v can_resubmit %v; want PENDING, RESUBMITTED with the time, nothing more to send", sent.GetState(), sent.GetReview(), sent.GetCanResubmit())
	}
	master := tb.master.get(t, tb.campaign, tb.pc.GetId())
	if master.GetReview().GetStatus() != charactersv1.Review_REVIEW_RESUBMITTED || master.GetReview().GetReason() != "Falta o equipamento." || !master.GetCanApprove() {
		t.Errorf("master after the resubmit = %v, want RESUBMITTED with the reason kept, still approvable", master.GetReview())
	}
	// A retry with the same key is the same answer; another key finds nothing open.
	if again, err := tb.lia.resubmit(tb.pc, k); err != nil || again.GetReview().GetStatus() != charactersv1.Review_REVIEW_RESUBMITTED {
		t.Errorf("retry of ResubmitCharacter = %v, %v; want the first answer", again.GetReview(), err)
	}
	_, err = tb.lia.resubmit(tb.pc, key())
	if d := blocked(t, "ResubmitCharacter(again)", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NO_CHANGES_REQUESTED {
		t.Errorf("CharacterBlocked = %v, want NO_CHANGES_REQUESTED", d)
	}
	wantTold := []string{"changes_requested:" + tb.pc.GetId(), "resubmitted:" + tb.pc.GetId()}
	if got := tb.host.told(); strings.Join(got, ",") != strings.Join(wantTold, ",") {
		t.Errorf("the stream was told %v, want %v", got, wantTold)
	}

	// He is not bound: he asks again, and she has something to send again.
	tb.h.clock.Advance(time.Minute)
	if asked, err := tb.master.ask(tb.pc, "Ainda falta a história.", key()); err != nil || asked.GetReview().GetStatus() != charactersv1.Review_REVIEW_CHANGES_REQUESTED ||
		asked.GetReview().GetResubmittedAt() != nil || asked.GetReview().GetReason() != "Ainda falta a história." {
		t.Fatalf("asking after the resubmit = %v, %v; want CHANGES_REQUESTED again, with no resubmit time", asked.GetReview(), err)
	}
	if _, err := tb.lia.resubmit(tb.pc, key()); err != nil {
		t.Errorf("ResubmitCharacter() after the second request error = %v", err)
	}
	// The master may approve at any review state.
	if approved := tb.master.approve(t, tb.pc); approved.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DRAFT || approved.GetReview() != nil {
		t.Errorf("ApproveCharacter() after a resubmit = state %v review %v; want a draft with no review", approved.GetState(), approved.GetReview())
	}
}

// RN-15: a character that was approved cannot be resubmitted.
func TestRN15_ResubmitAfterApprovalIsRefused(t *testing.T) {
	t.Parallel()
	tb, _ := newReviewTable(t)
	if _, err := tb.master.ask(tb.pc, "Falta o equipamento.", key()); err != nil {
		t.Fatalf("RequestCharacterChanges() error = %v", err)
	}
	tb.master.approve(t, tb.pc)
	_, err := tb.lia.resubmit(tb.pc, key())
	if d := blocked(t, "ResubmitCharacter(approved)", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_PENDING {
		t.Errorf("CharacterBlocked = %v, want NOT_PENDING", d)
	}
}

// RN-15, docs/privacy.md: the reason goes when the character is approved, rejected or deleted, when
// the player deletes the account, when the campaign is deleted, and when an ordinary invite
// approves the player.
func TestRN15_TheReasonIsDeletedInEveryWayTheCharacterLeavesThePendingState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		do   func(tb *reviewTable)
		want int // the review rows left in the campaign (the other pending member's stays)
	}{
		{"approved", func(tb *reviewTable) { tb.master.approve(t, tb.pc) }, 1},
		{"rejected", func(tb *reviewTable) {
			if err := tb.master.reject(t, tb.pc); err != nil {
				t.Fatalf("RejectCharacter() error = %v", err)
			}
		}, 1},
		{"the player deleted their account", func(tb *reviewTable) { tb.h.deleteUser(tb.lia.id) }, 1},
		{"the campaign was deleted", func(tb *reviewTable) {
			if _, err := tb.h.pool.Exec(t.Context(), "DELETE FROM campaigns WHERE id = $1", tb.campaign); err != nil {
				t.Fatalf("delete the campaign: %v", err)
			}
		}, 0},
		{"the character was deleted", func(tb *reviewTable) {
			if _, err := tb.h.pool.Exec(t.Context(), "DELETE FROM characters WHERE id = $1", tb.pc.GetId()); err != nil {
				t.Fatalf("delete the character: %v", err)
			}
		}, 1},
		{"an ordinary invite approved her", func(tb *reviewTable) {
			inv, err := tb.master.campaigns.CreateInvite(t.Context(), connect.NewRequest(&campaignsv1.CreateInviteRequest{CampaignId: tb.campaign, MaxUses: 5}))
			if err != nil {
				t.Fatalf("CreateInvite() error = %v", err)
			}
			if _, err := tb.lia.campaigns.AcceptInvite(t.Context(), connect.NewRequest(&campaignsv1.AcceptInviteRequest{Token: inv.Msg.GetToken()})); err != nil {
				t.Fatalf("AcceptInvite() error = %v", err)
			}
		}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tb, _ := newReviewTable(t)
			evePC := tb.eve.createPensantus(t, tb.campaign)
			for _, c := range []*charactersv1.Character{tb.pc, evePC} {
				if _, err := tb.master.ask(c, "Falta o equipamento.", key()); err != nil {
					t.Fatalf("RequestCharacterChanges() error = %v", err)
				}
			}
			if got := tb.h.reviewRows(tb.campaign); got != 2 {
				t.Fatalf("%d review rows before, want 2", got)
			}
			tc.do(tb)
			if got := tb.h.reviewRows(tb.campaign); got != tc.want {
				t.Errorf("%d review rows after %s, want %d", got, tc.name, tc.want)
			}
			var reasons int
			if err := tb.h.pool.QueryRow(t.Context(), "SELECT count(*) FROM character_reviews WHERE character_id = $1", tb.pc.GetId()).Scan(&reasons); err != nil || reasons != 0 {
				t.Errorf("her review rows after %s = %d, %v; want 0", tc.name, reasons, err)
			}
		})
	}
}

// RN-10: only the master and the owner read the reason. Not another player, not another pending
// member, not a stranger; not the list, not the logs.
func TestRN10_TheReasonReachesOnlyTheMasterAndTheOwner(t *testing.T) {
	t.Parallel()
	tb, logs := newReviewTable(t)
	const needle = "LEAKCANARY-review-1"
	if _, err := tb.master.ask(tb.pc, "Falta o equipamento. "+needle, key()); err != nil {
		t.Fatalf("RequestCharacterChanges() error = %v", err)
	}
	// The positive control: the two who may read it, do.
	for who, u := range map[string]*user{"master": tb.master, "owner": tb.lia} {
		if got := u.get(t, tb.campaign, tb.pc.GetId()); !strings.Contains(got.GetReview().GetReason(), needle) {
			t.Fatalf("%s does not read the reason: %v", who, got.GetReview())
		}
	}
	// Nobody else reads the character at all, so nothing of its review.
	for who, u := range map[string]*user{"another player": tb.other, "another pending member": tb.eve, "a stranger": tb.h.newUser("Estranho")} {
		_, err := u.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: tb.campaign, CharacterId: tb.pc.GetId()}))
		wantCode(t, who+"'s GetCharacter", err, connect.CodeNotFound)
		_, err = u.resubmit(tb.pc, key())
		wantCode(t, who+"'s ResubmitCharacter", err, connect.CodeNotFound)
		_, err = u.ask(tb.pc, "Eu também.", key())
		if connect.CodeOf(err) != connect.CodeNotFound && connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("%s's RequestCharacterChanges error = %v, want not_found or permission_denied", who, err)
		}
		list, err := u.api.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: tb.campaign}))
		if err == nil && strings.Contains(list.Msg.String(), needle) {
			t.Errorf("%s's ListCharacters carries the reason", who)
		}
	}
	// The master's list says the status but not the reason.
	list, err := tb.master.api.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: tb.campaign}))
	if err != nil || strings.Contains(list.Msg.String(), needle) {
		t.Errorf("master's ListCharacters = %v, %v; want no reason in a list", list, err)
	}
	// Other players learn nothing from the answer's errors either: it is the same for the owner of
	// another character and for a stranger.
	_, errOther := tb.other.resubmit(tb.pc, key())
	_, errStranger := tb.h.newUser("Outro Estranho").resubmit(tb.pc, key())
	if connect.CodeOf(errOther) != connect.CodeNotFound || connect.CodeOf(errStranger) != connect.CodeNotFound {
		t.Errorf("ResubmitCharacter errors differ: %v / %v; want not_found for both", errOther, errStranger)
	}
	// The server's log keeps the size, not the text.
	if strings.Contains(logs.String(), needle) {
		t.Errorf("the server log carries the reason")
	}
	if !strings.Contains(logs.String(), "reason_length") {
		t.Errorf("the server log does not carry the reason's length")
	}
}
