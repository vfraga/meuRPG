package campaigns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
)

// Tests of the campaign document (MR-018, document.go). The ones that use
// newHarness run against CockroachDB and skip without
// MEURPG_TEST_DATABASE_URL; the others need no database.

// The IDs inside the sample document's links. The server never checks
// them, so they need not exist.
const (
	sampleMapID       = "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0a1"
	sampleCharacterID = "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0b2"
	sampleImageID     = "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0c3"
)

// sampleLinks are the three link targets of the app's own (a map, a sheet
// and a gallery image), as the editor's pickers write them.
var sampleLinks = []string{
	"[Mirathel e arredores](map:" + sampleMapID + ")",
	"[Capitão Goblin](character:" + sampleCharacterID + ")",
	"![A Taverna do Javali, onde a Velha Odra espera o grupo.](image:" + sampleImageID + ")",
}

// sampleDocument has what a master writes: headings, bold, a list, a tab,
// accents and an emoji, an indented code block at the very start (kept:
// nothing is trimmed) and the three links.
var sampleDocument = "    bloco de código no começo\n\n" +
	"## Arco 1: a estrada de Mirathel\n\n" +
	"Na curva da estrada (o ponto Emboscada na estrada, no mapa " + sampleLinks[0] + "), três goblins atacam.\n" +
	"Quem passar num teste de **Sabedoria (Percepção) CD 13** não fica surpreso. 🐉\n\n" +
	sampleLinks[2] + "\n\n" +
	"## Personagens importantes\n\n" +
	"- **Velha Odra** sabe quem paga os goblins.\n" +
	"-\t" + sampleLinks[1] + " comanda o bando.\n" +
	"\n"

// getDocument calls GetCampaignDocument as c.
func getDocument(t *testing.T, c campaignsv1connect.CampaignDocumentServiceClient, campaignID string) (*campaignsv1.CampaignDocument, error) {
	t.Helper()
	res, err := c.GetCampaignDocument(t.Context(), connect.NewRequest(&campaignsv1.GetCampaignDocumentRequest{CampaignId: campaignID}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetDocument(), nil
}

// saveDocument calls UpdateCampaignDocument as c.
func saveDocument(t *testing.T, c campaignsv1connect.CampaignDocumentServiceClient, campaignID, body string, expectedRevision int32) (*campaignsv1.CampaignDocument, error) {
	t.Helper()
	res, err := c.UpdateCampaignDocument(t.Context(), connect.NewRequest(&campaignsv1.UpdateCampaignDocumentRequest{
		CampaignId:       campaignID,
		Body:             body,
		ExpectedRevision: expectedRevision,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetDocument(), nil
}

// mustGetDocument is getDocument, failing the test on an error.
func mustGetDocument(t *testing.T, c campaignsv1connect.CampaignDocumentServiceClient, campaignID string) *campaignsv1.CampaignDocument {
	t.Helper()
	doc, err := getDocument(t, c, campaignID)
	if err != nil {
		t.Fatalf("GetCampaignDocument() error = %v", err)
	}
	return doc
}

// mustSaveDocument is saveDocument, failing the test on an error.
func mustSaveDocument(t *testing.T, c campaignsv1connect.CampaignDocumentServiceClient, campaignID, body string, expectedRevision int32) *campaignsv1.CampaignDocument {
	t.Helper()
	doc, err := saveDocument(t, c, campaignID, body, expectedRevision)
	if err != nil {
		t.Fatalf("UpdateCampaignDocument(expected_revision %d) error = %v", expectedRevision, err)
	}
	return doc
}

// addMaster makes userID a second master of the campaign, straight in the
// database. The API cannot do that yet (RN-13, MR-023), but the schema
// allows it, and it is the only way an editor's account can be deleted
// without deleting the campaign with it.
func (h *harness) addMaster(campaignID, userID string) {
	h.t.Helper()
	if _, err := h.pool.Exec(h.t.Context(),
		"INSERT INTO campaign_members (campaign_id, user_id, role, status) VALUES ($1, $2, 'master', 'active')",
		campaignID, userID); err != nil {
		h.t.Fatalf("add a second master: %v", err)
	}
}

// storedDocument is a campaign_documents row, read straight from the
// database.
type storedDocument struct {
	body      string
	revision  int32
	updatedBy *string
}

// documentRow reads the campaign's campaign_documents row directly. ok is
// false when there is none.
func (h *harness) documentRow(campaignID string) (row storedDocument, ok bool) {
	h.t.Helper()
	err := h.pool.QueryRow(h.t.Context(),
		"SELECT body, revision, updated_by FROM campaign_documents WHERE campaign_id = $1", campaignID).
		Scan(&row.body, &row.revision, &row.updatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false
	}
	if err != nil {
		h.t.Fatalf("read campaign_documents: %v", err)
	}
	return row, true
}

// TestMR018_MasterWritesTheCampaignDocument: the document starts empty; the
// master saves it and reads it back with its revision, time and editor;
// saves again with the right revision; and the text, the app's own links
// and image syntax included, comes back byte for byte.
func TestMR018_MasterWritesTheCampaignDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	id := master.createCampaign(t, "Mirathel").GetId()

	// Never saved: empty, at revision 0, with no time and no editor.
	res, err := master.doc.GetCampaignDocument(t.Context(), connect.NewRequest(&campaignsv1.GetCampaignDocumentRequest{CampaignId: id}))
	if err != nil {
		t.Fatalf("GetCampaignDocument() error = %v", err)
	}
	if want := (&campaignsv1.CampaignDocument{CampaignId: id}); !proto.Equal(res.Msg.GetDocument(), want) {
		t.Errorf("new document = %v, want %v", res.Msg.GetDocument(), want)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("GetCampaignDocument Cache-Control = %q, want no-store", got)
	}
	if _, ok := h.documentRow(id); ok {
		t.Error("reading a new document stored a row")
	}

	// The first save.
	h.clock.Advance(time.Hour)
	saved := mustSaveDocument(t, master.doc, id, sampleDocument, 0)
	if saved.GetRevision() != 1 || saved.GetBody() != sampleDocument || saved.GetCampaignId() != id {
		t.Errorf("first save = revision %d, campaign %q, body %q; want revision 1, the campaign and the body as sent",
			saved.GetRevision(), saved.GetCampaignId(), saved.GetBody())
	}
	if got := saved.GetUpdatedAt().AsTime(); !got.Equal(h.clock.Now()) {
		t.Errorf("updated_at = %v, want the save's time %v", got, h.clock.Now())
	}
	if got := saved.GetUpdatedByDisplayName(); got != "Samuel" {
		t.Errorf("updated_by_display_name = %q, want Samuel", got)
	}

	// Read back: the same document, links and image syntax byte for byte.
	got := mustGetDocument(t, master.doc, id)
	if !proto.Equal(got, saved) {
		t.Errorf("read back %v, want what the save returned %v", got, saved)
	}
	if got.GetBody() != sampleDocument {
		t.Errorf("body read back = %q, want %q", got.GetBody(), sampleDocument)
	}
	for _, link := range sampleLinks {
		if !strings.Contains(got.GetBody(), link) {
			t.Errorf("body read back lost %q", link)
		}
	}

	// A second save, with the revision just read.
	h.clock.Advance(time.Minute)
	second := mustSaveDocument(t, master.doc, id, sampleDocument+"## Segredos\n", got.GetRevision())
	if second.GetRevision() != 2 || !second.GetUpdatedAt().AsTime().Equal(h.clock.Now()) {
		t.Errorf("second save = revision %d at %v, want revision 2 at %v", second.GetRevision(), second.GetUpdatedAt().AsTime(), h.clock.Now())
	}

	// An empty body is saved too: the row stays, one revision up.
	empty := mustSaveDocument(t, master.doc, id, "", 2)
	if empty.GetRevision() != 3 || empty.GetBody() != "" || empty.GetUpdatedAt() == nil {
		t.Errorf("empty save = %v, want revision 3, an empty body and a time", empty)
	}
	if row, ok := h.documentRow(id); !ok || row.revision != 3 || row.body != "" {
		t.Errorf("row after the empty save = %+v (exists %v), want revision 3 and an empty body", row, ok)
	}
}

// TestMR018_PlayersCannotReadTheDocument: while the master-only default holds,
// only the master sees the document. A player gets permission_denied on
// both calls; a non-member and a pending member get not_found, as for a
// campaign that does not exist. Nothing they try changes it.
func TestMR018_PlayersCannotReadTheDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	player := h.newUser("Jogador")
	pending := h.newUser("Pendente")
	outsider := h.newUser("Outra pessoa")
	id := master.createCampaign(t, "Mirathel").GetId()
	_, token := master.createInvite(t, id, 0, 0)
	player.join(t, token)
	_, approvalToken := master.createApprovalInvite(t, id)
	pending.join(t, approvalToken)
	secret := mustSaveDocument(t, master.doc, id, "## Segredos\n\nO capitão trabalha para um contrabandista.", 0)

	for _, c := range []struct {
		name string
		user *user
		want connect.Code
	}{
		{"player", player, connect.CodePermissionDenied},
		{"non-member", outsider, connect.CodeNotFound},
		{"pending member", pending, connect.CodeNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc, err := getDocument(t, c.user.doc, id)
			wantCode(t, "GetCampaignDocument()", err, c.want)
			if doc != nil {
				t.Errorf("GetCampaignDocument() returned %v along with the error", doc)
			}
			_, err = saveDocument(t, c.user.doc, id, "Apagado.", secret.GetRevision())
			wantCode(t, "UpdateCampaignDocument()", err, c.want)
		})
	}

	if got := mustGetDocument(t, master.doc, id); !proto.Equal(got, secret) {
		t.Errorf("document after the others tried = %v, want it unchanged %v", got, secret)
	}
}

// TestUpdateCampaignDocumentRefusesAStaleRevision: two tabs read revision 1;
// the first save wins, and the second gets aborted without changing
// anything. A revision that never existed is stale too.
func TestUpdateCampaignDocumentRefusesAStaleRevision(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	id := master.createCampaign(t, "Mirathel").GetId()
	other := master.createCampaign(t, "Outra").GetId()
	mustSaveDocument(t, master.doc, id, "Primeira versão.", 0)

	read := mustGetDocument(t, master.doc, id) // both tabs read revision 1
	tabA := mustSaveDocument(t, master.doc, id, "Versão da aba A.", read.GetRevision())

	for name, expected := range map[string]int32{
		"the revision both tabs read":    read.GetRevision(),
		"0 once the document was saved":  0,
		"a revision that does not exist": 7,
	} {
		_, err := saveDocument(t, master.doc, id, "Versão da aba B.", expected)
		wantCode(t, "UpdateCampaignDocument("+name+")", err, connect.CodeAborted)
	}
	if got := mustGetDocument(t, master.doc, id); !proto.Equal(got, tabA) {
		t.Errorf("document after the stale saves = %v, want tab A's %v", got, tabA)
	}

	// A never-saved document is at revision 0, so 1 is stale there, and
	// the refused save stores nothing.
	_, err := saveDocument(t, master.doc, other, "Texto.", 1)
	wantCode(t, "UpdateCampaignDocument(never saved, expected 1)", err, connect.CodeAborted)
	if _, ok := h.documentRow(other); ok {
		t.Error("a refused first save stored a row")
	}
}

// TestSavingTheSameDocumentTwiceIsNotAConflict: when the response to a save
// gets lost, or the button is clicked twice, the same save arrives again
// with the old revision. It gets the saved document back, not aborted.
// Only the same text by the same person counts as the same save.
func TestSavingTheSameDocumentTwiceIsNotAConflict(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	coMaster := h.newUser("Coautora")
	id := master.createCampaign(t, "Mirathel").GetId()
	h.addMaster(id, coMaster.id)

	first := mustSaveDocument(t, master.doc, id, "Versão 1.", 0)
	h.clock.Advance(time.Minute)
	if again := mustSaveDocument(t, master.doc, id, "Versão 1.", 0); !proto.Equal(again, first) {
		t.Errorf("the first save again = %v, want the saved document %v", again, first)
	}

	second := mustSaveDocument(t, master.doc, id, "Versão 2.", 1)
	if again := mustSaveDocument(t, master.doc, id, "Versão 2.", 1); !proto.Equal(again, second) {
		t.Errorf("the second save again = %v, want the saved document %v", again, second)
	}

	// Another text on the old revision, or the same text by someone else,
	// is a real conflict.
	_, err := saveDocument(t, master.doc, id, "Outra versão.", 1)
	wantCode(t, "UpdateCampaignDocument(another text, old revision)", err, connect.CodeAborted)
	_, err = saveDocument(t, coMaster.doc, id, "Versão 2.", 1)
	wantCode(t, "UpdateCampaignDocument(same text by another master, old revision)", err, connect.CodeAborted)

	if got := mustGetDocument(t, master.doc, id); got.GetRevision() != 2 {
		t.Errorf("revision after the repeated saves = %d, want 2", got.GetRevision())
	}
}

// TestUpdateCampaignDocumentFirstSavesRace: several tabs save a new
// document at once, all from revision 0. Exactly one wins; the others get
// aborted, and the stored document is the winner's.
func TestUpdateCampaignDocumentFirstSavesRace(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 12) // the racers must overlap: one connection would run them one by one
	h := newHarness(t)
	master := h.newUser("Mestre")
	id := master.createCampaign(t, "Mirathel").GetId()

	const tabs = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners []string
	)
	start := dbtest.NewBarrier(tabs)
	for i := range tabs {
		wg.Go(func() {
			start.Wait()
			body := "Versão da aba " + strconv.Itoa(i+1) + "."
			_, err := saveDocument(t, master.doc, id, body, 0)
			switch {
			case err == nil:
				mu.Lock()
				winners = append(winners, body)
				mu.Unlock()
			case connect.CodeOf(err) == connect.CodeAborted:
			default:
				t.Errorf("UpdateCampaignDocument(%q) error = %v, want success or aborted", body, err)
			}
		})
	}
	wg.Wait()

	if len(winners) != 1 {
		t.Fatalf("%d saves from revision 0 won (%q), want exactly 1", len(winners), winners)
	}
	if got := mustGetDocument(t, master.doc, id); got.GetRevision() != 1 || got.GetBody() != winners[0] {
		t.Errorf("stored document = revision %d, body %q; want revision 1 and the winner's %q", got.GetRevision(), got.GetBody(), winners[0])
	}
}

// TestUpdateCampaignDocumentValidates: the server refuses a body over
// 200 KiB (in bytes), invalid UTF-8 and control characters with
// invalid_argument, never repeating the text; it turns \r\n into \n; and
// the database's CHECK refuses an oversized body even without the API.
func TestUpdateCampaignDocumentValidates(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	id := master.createCampaign(t, "Mirathel").GetId()
	saved := mustSaveDocument(t, master.doc, id, "Texto salvo.", 0)

	for name, body := range map[string]string{
		"one byte over 200 KiB":                    strings.Repeat("a", MaxDocumentBytes+1),
		"under the limit in characters, not bytes": strings.Repeat("é", MaxDocumentBytes/2+1),
		"NUL":                                  "segredo\x00",
		"escape":                               "segredo\x1b[31m",
		"bell":                                 "segredo\a",
		"right-to-left override":               "segredo\u202e",
		"C1 control character":                 "segredo\u0085",
		"the limit plus an old Mac line break": strings.Repeat("a", MaxDocumentBytes) + "\r",
	} {
		_, err := saveDocument(t, master.doc, id, body, saved.GetRevision())
		wantCode(t, "UpdateCampaignDocument("+name+")", err, connect.CodeInvalidArgument)
		if msg := connectMessage(err); !strings.HasPrefix(msg, "body ") || strings.Contains(msg, "segredo") {
			t.Errorf("%s: message %q should name the field and never repeat the text", name, msg)
		}
	}
	_, err := saveDocument(t, master.doc, id, "Texto.", -1)
	wantCode(t, "UpdateCampaignDocument(expected_revision -1)", err, connect.CodeInvalidArgument)

	// Invalid UTF-8 cannot even be encoded by a Go client, so this request
	// is built by hand: a binary UpdateCampaignDocumentRequest whose body
	// is not UTF-8.
	msg := protowire.AppendTag(nil, 1, protowire.BytesType)
	msg = protowire.AppendString(msg, id)
	msg = protowire.AppendTag(msg, 2, protowire.BytesType)
	msg = protowire.AppendBytes(msg, []byte("Mira\xffthel"))
	msg = protowire.AppendTag(msg, 3, protowire.VarintType)
	msg = protowire.AppendVarint(msg, 1) // expected_revision: saved once
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		h.server.URL+campaignsv1connect.CampaignDocumentServiceUpdateCampaignDocumentProcedure, bytes.NewReader(msg))
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set(testUserHeader, master.id)
	res, err := h.server.Client().Do(req)
	if err != nil {
		t.Fatalf("send the invalid UTF-8 request: %v", err)
	}
	var body struct{ Code string }
	err = json.NewDecoder(res.Body).Decode(&body)
	_ = res.Body.Close()
	if err != nil || body.Code != connect.CodeInvalidArgument.String() {
		t.Errorf("invalid UTF-8: status %d, code %q (%v); want invalid_argument", res.StatusCode, body.Code, err)
	}

	// Nothing above changed the document.
	if got := mustGetDocument(t, master.doc, id); !proto.Equal(got, saved) {
		t.Errorf("document after the refused saves = %v, want %v", got, saved)
	}

	// Line breaks: Windows and old Mac ones become \n; the limit is
	// inclusive.
	normalized := mustSaveDocument(t, master.doc, id, "linha 1\r\nlinha 2\rlinha 3\n", saved.GetRevision())
	if want := "linha 1\nlinha 2\nlinha 3\n"; normalized.GetBody() != want {
		t.Errorf("saved body = %q, want %q", normalized.GetBody(), want)
	}
	full := mustSaveDocument(t, master.doc, id, strings.Repeat("a", MaxDocumentBytes), normalized.GetRevision())
	if len(full.GetBody()) != MaxDocumentBytes {
		t.Errorf("a body of exactly %d bytes came back with %d", MaxDocumentBytes, len(full.GetBody()))
	}

	// The CHECK is the last line of defense.
	_, err = h.pool.Exec(t.Context(), "UPDATE campaign_documents SET body = $2 WHERE campaign_id = $1",
		id, strings.Repeat("a", MaxDocumentBytes+1))
	// 23514 is check_violation.
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); !ok || pgErr.Code != "23514" || pgErr.ConstraintName != "campaign_documents_body_size" {
		t.Errorf("storing %d bytes straight in the database: error = %v, want the campaign_documents_body_size CHECK", MaxDocumentBytes+1, err)
	}
}

// TestCampaignDocumentGoesWithTheCampaign checks the ON DELETE rules that
// docs/privacy.md promises: deleting the account of whoever saved the
// document last keeps the document, with no editor; deleting the campaign,
// or the account of the master who created it, deletes the document.
func TestCampaignDocumentGoesWithTheCampaign(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	coMaster := h.newUser("Coautora")
	id := master.createCampaign(t, "Mirathel").GetId()
	h.addMaster(id, coMaster.id)

	saved := mustSaveDocument(t, coMaster.doc, id, "Notas da coautora.", 0)
	if got := mustGetDocument(t, master.doc, id).GetUpdatedByDisplayName(); got != "Coautora" {
		t.Errorf("updated_by_display_name = %q, want Coautora", got)
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := h.pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	// The editor deletes their account: the document stays, with no editor.
	exec("DELETE FROM users WHERE id = $1", coMaster.id)
	if row, ok := h.documentRow(id); !ok || row.updatedBy != nil || row.body != saved.GetBody() {
		t.Errorf("row after the editor deleted their account = %+v (exists %v), want the same body and no editor", row, ok)
	}
	got := mustGetDocument(t, master.doc, id)
	if got.GetBody() != saved.GetBody() || got.GetRevision() != saved.GetRevision() || got.GetUpdatedByDisplayName() != "" {
		t.Errorf("document after the editor deleted their account = %v, want the same text and revision, with no editor's name", got)
	}

	// The campaign is deleted: so is its document.
	exec("DELETE FROM campaigns WHERE id = $1", id)
	if _, ok := h.documentRow(id); ok {
		t.Error("the document outlived its campaign")
	}

	// The master who created a campaign, and saved its document, deletes
	// their account: the campaign goes, and the document with it.
	other := master.createCampaign(t, "Outra").GetId()
	mustSaveDocument(t, master.doc, other, "Notas do mestre.", 0)
	exec("DELETE FROM users WHERE id = $1", master.id)
	if _, ok := h.documentRow(other); ok {
		t.Error("the document outlived the account of the master who created its campaign")
	}
}

// TestCampaignDocumentAuthorizationMatrix calls every
// CampaignDocumentService method as each kind of caller, like
// TestAuthorizationMatrix: only the master gets in (the
// default); a player gets permission_denied; a non-member and a pending
// member (RN-15) get not_found; a caller with no session, unauthenticated.
func TestCampaignDocumentAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	player := h.newUser("Jogador")
	pending := h.newUser("Pendente")
	id := master.createCampaign(t, "Mirathel").GetId()
	_, token := master.createInvite(t, id, 0, 0)
	player.join(t, token)
	_, approvalToken := master.createApprovalInvite(t, id)
	pending.join(t, approvalToken)

	type client = campaignsv1connect.CampaignDocumentServiceClient
	methods := []struct {
		name string
		call func(ctx context.Context, c client) error
		// The expected code for master, player, non-member, anonymous,
		// pending member.
		want [5]connect.Code
	}{
		{"GetCampaignDocument", func(ctx context.Context, c client) error {
			_, err := c.GetCampaignDocument(ctx, connect.NewRequest(&campaignsv1.GetCampaignDocumentRequest{CampaignId: id}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"UpdateCampaignDocument", func(ctx context.Context, c client) error {
			// The current revision each time, so the master's call really
			// saves.
			current := mustGetDocument(t, master.doc, id)
			_, err := c.UpdateCampaignDocument(ctx, connect.NewRequest(&campaignsv1.UpdateCampaignDocumentRequest{
				CampaignId: id, Body: "Texto.", ExpectedRevision: current.GetRevision(),
			}))
			return err
		}, [5]connect.Code{allowed, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
	}

	// Every method of the service must be in the table, so a new RPC cannot
	// ship without its authorization test.
	service := campaignsv1.File_meurpg_campaigns_v1_campaign_document_proto.Services().ByName("CampaignDocumentService")
	covered := map[string]bool{}
	for _, m := range methods {
		covered[m.name] = true
	}
	for i := range service.Methods().Len() {
		if name := string(service.Methods().Get(i).Name()); !covered[name] {
			t.Errorf("CampaignDocumentService.%s is missing from the authorization matrix", name)
		}
	}

	callers := []struct {
		name   string
		client client
	}{
		{"master", master.doc},
		{"player", player.doc},
		{"non-member", h.newUser("Outra pessoa").doc},
		{"anonymous", h.documentClient("")},
		{"pending", pending.doc},
	}
	for _, m := range methods {
		for i, caller := range callers {
			t.Run(m.name+"/"+caller.name, func(t *testing.T) {
				err := m.call(t.Context(), caller.client)
				want := m.want[i]
				switch {
				case want == allowed && err != nil:
					t.Errorf("error = %v, want allowed", err)
				case want != allowed && connect.CodeOf(err) != want:
					t.Errorf("error = %v, want %v", err, want)
				}
			})
		}
	}
}

// TestEveryDocumentMethodNeedsASession calls every CampaignDocumentService
// method signed out. The session check comes first, so no database is
// needed to see it refuse, and the error is not cacheable.
func TestEveryDocumentMethodNeedsASession(t *testing.T) {
	t.Parallel()
	svc, err := New(Config{Pool: lazyPool(t), Profiles: noProfiles{}, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	mux := http.NewServeMux()
	svc.Mount(mux.Handle, testSessions, reqlimit.HandlerOption())
	server := httptest.NewServer(reqlimit.Middleware(mux))
	t.Cleanup(server.Close)
	c := campaignsv1connect.NewCampaignDocumentServiceClient(server.Client(), server.URL)
	ctx := t.Context()
	id := "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"

	calls := map[string]error{}
	_, calls["GetCampaignDocument"] = c.GetCampaignDocument(ctx, connect.NewRequest(&campaignsv1.GetCampaignDocumentRequest{CampaignId: id}))
	_, calls["UpdateCampaignDocument"] = c.UpdateCampaignDocument(ctx, connect.NewRequest(&campaignsv1.UpdateCampaignDocumentRequest{CampaignId: id, Body: "Texto."}))

	methods := campaignsv1.File_meurpg_campaigns_v1_campaign_document_proto.Services().ByName("CampaignDocumentService").Methods()
	if len(calls) != methods.Len() {
		t.Errorf("called %d methods, the service has %d", len(calls), methods.Len())
	}
	for name, err := range calls {
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: error = %v, want unauthenticated", name, err)
		}
		if ce, ok := errors.AsType[*connect.Error](err); !ok || !slices.Equal(ce.Meta().Values("Cache-Control"), []string{"no-store"}) {
			t.Errorf("%s: error response Cache-Control = %q, want no-store, once", name, ce.Meta().Values("Cache-Control"))
		}
	}
}

// TestGetCampaignDocumentIsPostOnly: the read carries a campaign ID, so it
// is IDEMPOTENT, not NO_SIDE_EFFECTS, and the server refuses it over GET,
// which would put the ID in the URL.
func TestGetCampaignDocumentIsPostOnly(t *testing.T) {
	t.Parallel()
	svc, err := New(Config{Pool: lazyPool(t), Profiles: noProfiles{}, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	mux := http.NewServeMux()
	svc.Mount(mux.Handle, testSessions, reqlimit.HandlerOption())

	target := campaignsv1connect.CampaignDocumentServiceGetCampaignDocumentProcedure +
		"?connect=v1&encoding=json&message=" + url.QueryEscape(`{"campaignId":"6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"}`)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET GetCampaignDocument: status = %d, want 405", rec.Code)
	}

	method := campaignsv1.File_meurpg_campaigns_v1_campaign_document_proto.Services().ByName("CampaignDocumentService").Methods().ByName("GetCampaignDocument")
	opts, _ := method.Options().(*descriptorpb.MethodOptions)
	if opts.GetIdempotencyLevel() != descriptorpb.MethodOptions_IDEMPOTENT {
		t.Errorf("GetCampaignDocument idempotency_level = %v, want IDEMPOTENT", opts.GetIdempotencyLevel())
	}
}

// TestCleanDocument covers the body's rules without a database.
func TestCleanDocument(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"empty is fine", "", "", false},
		{"keeps spaces and blank lines at both ends", "\n    código\n\n", "\n    código\n\n", false},
		{"keeps tabs, accents and emoji", "-\tCapitão 🐉", "-\tCapitão 🐉", false},
		{"keeps the app's links", sampleDocument, sampleDocument, false},
		{"Windows line breaks", "linha 1\r\nlinha 2", "linha 1\nlinha 2", false},
		{"old Mac line breaks", "linha 1\rlinha 2", "linha 1\nlinha 2", false},
		{"exactly the limit", strings.Repeat("a", MaxDocumentBytes), strings.Repeat("a", MaxDocumentBytes), false},
		{"shrinks under the limit once \\r\\n becomes \\n", strings.Repeat("a", MaxDocumentBytes-1) + "\r\n", strings.Repeat("a", MaxDocumentBytes-1) + "\n", false},
		{"one byte over", strings.Repeat("a", MaxDocumentBytes+1), "", true},
		{"counts bytes, not characters", strings.Repeat("é", MaxDocumentBytes/2+1), "", true},
		{"NUL", "Mira\x00thel", "", true},
		{"escape", "Mira\x1bthel", "", true},
		{"C1 control", "Mira\u0085thel", "", true},
		{"right-to-left override", "Mira\u202ethel", "", true},
		{"invalid UTF-8", "Mira\xffthel", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := cleanDocument(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("cleanDocument() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("cleanDocument() = %q, want %q", got, tt.want)
			}
		})
	}
}
