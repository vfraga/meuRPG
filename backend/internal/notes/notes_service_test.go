package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"

	notesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1/notesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/notes/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
)

// CreateNote and UpdateNote: the order of their checks, every refusal with its
// code and message, what each failing query answers, and what a change keeps. The package's
// end-to-end tests (privacy, the scenes the group discovered, the clues) live
// with the maps module, which owns the scenes; these run the service alone,
// against CockroachDB, with fakes for the scenes and the memberships.
//
// A database error is made by dbtest.QueryFaults: a pgx tracer on the
// service's pool that hands one named sqlc query an already canceled context,
// so pgx fails it before sending it and the connection stays usable. rpcerr
// calls that error Canceled.

const notesUserHeader = "Test-User-Id"

type notesUserKey struct{}

// notesSessions trusts the test header for the caller.
type notesSessions struct{}

func (notesSessions) Interceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if id := req.Header().Get(notesUserHeader); id != "" {
				ctx = context.WithValue(ctx, notesUserKey{}, id)
			}
			return next(ctx, req)
		}
	})
}

func (notesSessions) UserID(ctx context.Context) (string, error) {
	if id, ok := ctx.Value(notesUserKey{}).(string); ok {
		return id, nil
	}
	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("sign in to continue"))
}

// notesMembers is the campaign's members: user ID -> role, all active.
type notesMembers map[string]authz.Role

func (m notesMembers) CampaignMembership(_ context.Context, _, userID string) (authz.Role, authz.Status, error) {
	if role, ok := m[userID]; ok {
		return role, authz.StatusActive, nil
	}
	return "", "", authz.ErrNotMember
}

// notesScenes is the maps module as the notes see it.
type notesScenes struct {
	mu         sync.Mutex
	discovered []link.Scene
	err        error
}

func (s *notesScenes) DiscoveredScenes(context.Context, string, string) ([]link.Scene, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.discovered, s.err
}

func (*notesScenes) ReceivedClues(context.Context, string, string) ([]link.Clue, error) {
	return nil, nil
}

// notesLogTape keeps the service's log records as JSON.
type notesLogTape struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *notesLogTape) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// errors counts the records logged at ERROR.
func (l *notesLogTape) errors(t *testing.T) int {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for line := range strings.SplitSeq(strings.TrimSpace(l.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		if rec["level"] == "ERROR" {
			n++
		}
	}
	return n
}

// notesRig is the notes service on a test database, served over HTTP, with a
// campaign, its master, two players and three scenes on a map.
type notesRig struct {
	pool     *pgxpool.Pool
	faults   *dbtest.QueryFaults
	logs     *notesLogTape
	scenes   *notesScenes
	server   *httptest.Server
	campaign string
	master   string
	ana, bia string
	// sceneIDs are the map's points; the group discovered the first two.
	sceneIDs [3]string
	clockMu  sync.Mutex
	now      time.Time
}

// clock is the service's Now.
func (rig *notesRig) clock() time.Time {
	rig.clockMu.Lock()
	defer rig.clockMu.Unlock()
	return rig.now
}

func newNotesRig(t *testing.T) *notesRig {
	t.Helper()
	ctx := t.Context()
	pool := dbtest.NewPool(t, "meurpg_notes_test")
	rig := &notesRig{pool: pool, logs: &notesLogTape{}, scenes: &notesScenes{}, now: time.Now().Truncate(time.Microsecond)}
	one := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return id
	}
	rig.master = one(`INSERT INTO users DEFAULT VALUES RETURNING id::TEXT`)
	rig.ana = one(`INSERT INTO users DEFAULT VALUES RETURNING id::TEXT`)
	rig.bia = one(`INSERT INTO users DEFAULT VALUES RETURNING id::TEXT`)
	rig.campaign = one(`INSERT INTO campaigns (name, xp_mode, created_by) VALUES ('Mirathel', 'milestones', $1) RETURNING id::TEXT`, rig.master)
	image := one(`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at)
		VALUES (gen_random_uuid(), $1, 'Vila', 'image/png', 10, 10, 100, now()) RETURNING id::TEXT`, rig.campaign)
	mapID := one(`INSERT INTO maps (campaign_id, name, image_id, created_at, updated_at) VALUES ($1, 'Vila', $2, now(), now()) RETURNING id::TEXT`, rig.campaign, image)
	for i, name := range []string{"A ponte", "A torre", "O poço"} {
		rig.sceneIDs[i] = one(`INSERT INTO map_points (map_id, kind, name, x_bp, y_bp, created_at, updated_at) VALUES ($1, 'scene', $2, 10, 10, now(), now()) RETURNING id::TEXT`, mapID, name)
	}
	rig.scenes.discovered = []link.Scene{{ID: rig.sceneIDs[0], Name: "A ponte"}, {ID: rig.sceneIDs[1], Name: "A torre"}}

	rig.faults = dbtest.NewQueryFaults(pool.Config().ConnConfig.Tracer)
	svcPool, err := db.NewPoolWith(ctx, pool.Config().ConnString(), func(pc *pgxpool.Config) {
		pc.MaxConns = 1
		pc.ConnConfig.Tracer = rig.faults
	})
	if err != nil {
		t.Fatalf("NewPoolWith() error = %v", err)
	}
	t.Cleanup(svcPool.Close)
	svc, err := New(Config{
		Pool: svcPool, Scenes: rig.scenes, Now: rig.clock,
		Logger: slog.New(slog.NewJSONHandler(rig.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	members := notesMembers{rig.master: authz.RoleMaster, rig.ana: authz.RolePlayer, rig.bia: authz.RolePlayer}
	mux := http.NewServeMux()
	svc.Mount(mux.Handle, notesSessions{}, members, reqlimit.HandlerOption())
	rig.server = httptest.NewServer(reqlimit.Middleware(mux))
	t.Cleanup(rig.server.Close)
	return rig
}

// client calls NotesService as userID ("" for no session).
func (rig *notesRig) client(userID string) notesv1connect.NotesServiceClient {
	var opts []connect.ClientOption
	if userID != "" {
		opts = append(opts, connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set(notesUserHeader, userID)
				return next(ctx, req)
			}
		})))
	}
	return notesv1connect.NewNotesServiceClient(rig.server.Client(), rig.server.URL, opts...)
}

func wantNotesRefusal(t *testing.T, call string, err error, code connect.Code, msg string) {
	t.Helper()
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Code() != code || ce.Message() != msg {
		t.Errorf("%s error = %v, want %v %q", call, err, code, msg)
	}
}

// wantNotesFailure checks a call that failed with code.
func wantNotesFailure(t *testing.T, call string, err error, code connect.Code) {
	t.Helper()
	if connect.CodeOf(err) != code {
		t.Errorf("%s error = %v, want %v", call, err, code)
	}
}

// wantNotesNote checks a note's text and tag.
func wantNotesNote(t *testing.T, call string, n *notesv1.Note, text, sceneID, sceneName string) {
	t.Helper()
	if n.GetText() != text || n.GetScenePointId() != sceneID || n.GetSceneName() != sceneName || n.GetKind() != notesv1.NoteKind_NOTE_KIND_NOTE {
		t.Errorf("%s = %v, want text %q, scene %q %q", call, n, text, sceneID, sceneName)
	}
}

func TestCreateNoteOrderAndFailures(t *testing.T) {
	t.Parallel()
	rig := newNotesRig(t)
	ctx := t.Context()
	ana := rig.client(rig.ana)
	create := func(c notesv1connect.NotesServiceClient, req *notesv1.CreateNoteRequest) (*notesv1.Note, error) {
		if req.CampaignId == "" {
			req.CampaignId = rig.campaign
		}
		res, err := c.CreateNote(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetNote(), nil
	}

	// The order of the checks: the caller, the text, the scenes, the tag,
	// the key.
	_, err := create(rig.client(""), &notesv1.CreateNoteRequest{})
	wantNotesRefusal(t, "CreateNote(anonymous)", err, connect.CodeUnauthenticated, "sign in to continue")
	_, err = create(rig.client(rig.master), &notesv1.CreateNoteRequest{})
	wantNotesRefusal(t, "CreateNote(master)", err, connect.CodeNotFound, "note not found")
	_, err = create(ana, &notesv1.CreateNoteRequest{Text: "  \n ", ScenePointId: "not a uuid", IdempotencyKey: "bad\x00key"})
	wantNotesRefusal(t, "CreateNote(blank)", err, connect.CodeInvalidArgument, "text must not be empty")
	_, err = create(ana, &notesv1.CreateNoteRequest{Text: strings.Repeat("a", maxNoteText+1)})
	if connect.CodeOf(err) != connect.CodeInvalidArgument || !strings.HasPrefix(connectMessage(err), "text ") {
		t.Errorf("CreateNote(too long) error = %v", err)
	}
	rig.scenes.mu.Lock()
	rig.scenes.err = errors.New("maps is down")
	rig.scenes.mu.Unlock()
	_, err = create(ana, &notesv1.CreateNoteRequest{Text: "texto", ScenePointId: "not a uuid"})
	wantNotesFailure(t, "CreateNote(scenes fail)", err, connect.CodeInternal)
	if rig.logs.errors(t) == 0 {
		t.Errorf("a failure the client cannot fix was not logged at ERROR")
	}
	rig.scenes.mu.Lock()
	rig.scenes.err = nil
	rig.scenes.mu.Unlock()
	for _, tag := range []string{"not a uuid", rig.sceneIDs[2], "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0a1"} {
		_, err = create(ana, &notesv1.CreateNoteRequest{Text: "texto", ScenePointId: tag, IdempotencyKey: "bad\x00key"})
		wantNotesRefusal(t, "CreateNote(tag "+tag+")", err, connect.CodeInvalidArgument, "scene_point_id must be a scene the group discovered")
	}
	_, err = create(ana, &notesv1.CreateNoteRequest{Text: "texto", IdempotencyKey: "bad\x00key"})
	wantNotesRefusal(t, "CreateNote(bad key)", err, connect.CodeInvalidArgument, "idempotency_key must be 1 to 64 characters")

	// What a note keeps: the text as written (trimmed), the tag in
	// canonical form with its scene's name, the times.
	n, err := create(ana, &notesv1.CreateNoteRequest{Text: "  Linha 1\nLinha 2  ", ScenePointId: strings.ToUpper(rig.sceneIDs[0])})
	if err != nil {
		t.Fatalf("CreateNote() error = %v", err)
	}
	wantNotesNote(t, "CreateNote(tagged)", n, "Linha 1\nLinha 2", rig.sceneIDs[0], "A ponte")
	if !n.GetCreatedAt().AsTime().Equal(rig.clock()) || !n.GetUpdatedAt().AsTime().Equal(rig.clock()) {
		t.Errorf("CreateNote() times = %v, %v, want %v", n.GetCreatedAt().AsTime(), n.GetUpdatedAt().AsTime(), rig.clock())
	}
	n, err = create(ana, &notesv1.CreateNoteRequest{Text: strings.Repeat("é", maxNoteText)})
	if err != nil {
		t.Fatalf("CreateNote(longest) error = %v", err)
	}
	wantNotesNote(t, "CreateNote(longest)", n, strings.Repeat("é", maxNoteText), "", "")

	// Each failing query.
	for _, tc := range []struct{ query, key string }{
		{"GetPlayerNoteByCreateKey", "k1"},
		{"CountPlayerNotes", "k2"},
		{"InsertPlayerNote", "k3"},
		{"CountPlayerNotes", ""},
	} {
		rig.faults.Arm(tc.query)
		_, err := create(ana, &notesv1.CreateNoteRequest{Text: "texto", IdempotencyKey: tc.key})
		rig.faults.Disarm()
		if rig.faults.Hits(tc.query) == 0 {
			t.Fatalf("%s never ran", tc.query)
		}
		wantNotesFailure(t, "CreateNote("+tc.query+")", err, connect.CodeCanceled)
	}

	// The cap: a retry of a note already made is answered at the cap, a new
	// note is not; another player has their own cap.
	first, err := create(ana, &notesv1.CreateNoteRequest{Text: "primeira", IdempotencyKey: "first"})
	if err != nil {
		t.Fatalf("CreateNote(first) error = %v", err)
	}
	if _, err := rig.pool.Exec(ctx, `INSERT INTO player_notes (campaign_id, author_user_id, text, created_at, updated_at)
		SELECT $1, $2, 'x', now(), now() FROM generate_series(1, $3)`, rig.campaign, rig.ana, MaxNotes-3); err != nil {
		t.Fatalf("fill the notes: %v", err)
	}
	again, err := create(ana, &notesv1.CreateNoteRequest{Text: "primeira", IdempotencyKey: "first"})
	if err != nil || again.GetId() != first.GetId() {
		t.Errorf("CreateNote(retry at the cap) = %v, %v, want %s", again, err, first.GetId())
	}
	_, err = create(ana, &notesv1.CreateNoteRequest{Text: "mais uma", IdempotencyKey: "second"})
	wantNotesRefusal(t, "CreateNote(over the cap)", err, connect.CodeResourceExhausted, "you already have 300 notes in this campaign")
	_, err = create(ana, &notesv1.CreateNoteRequest{Text: "primeira, outra", IdempotencyKey: "first"})
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("CreateNote(the key with another text) error = %v, want invalid_argument", err)
	}
	if _, err := create(rig.client(rig.bia), &notesv1.CreateNoteRequest{Text: "da Bia", IdempotencyKey: "second"}); err != nil {
		t.Errorf("CreateNote(another player) error = %v", err)
	}
}

func TestUpdateNoteOrderAndFailures(t *testing.T) {
	t.Parallel()
	rig := newNotesRig(t)
	ctx := t.Context()
	ana := rig.client(rig.ana)
	n0 := mustNotesCreate(t, ana, rig.campaign, "original", rig.sceneIDs[0])
	update := func(c notesv1connect.NotesServiceClient, req *notesv1.UpdateNoteRequest) (*notesv1.Note, error) {
		if req.CampaignId == "" {
			req.CampaignId = rig.campaign
		}
		res, err := c.UpdateNote(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetNote(), nil
	}
	ptr := func(s string) *string { return &s }

	// The order of the checks: the caller, the note's ID, something to
	// change, the text, the scenes, the tag, then the note itself.
	_, err := update(rig.client(""), &notesv1.UpdateNoteRequest{})
	wantNotesRefusal(t, "UpdateNote(anonymous)", err, connect.CodeUnauthenticated, "sign in to continue")
	_, err = update(rig.client(rig.master), &notesv1.UpdateNoteRequest{NoteId: n0.GetId(), Text: ptr("x")})
	wantNotesRefusal(t, "UpdateNote(master)", err, connect.CodeNotFound, "note not found")
	_, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: "not a uuid"})
	wantNotesRefusal(t, "UpdateNote(bad id)", err, connect.CodeNotFound, "note not found")
	_, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: n0.GetId()})
	wantNotesRefusal(t, "UpdateNote(nothing)", err, connect.CodeInvalidArgument, "nothing to change")
	_, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: n0.GetId(), Text: ptr(" "), ScenePointId: ptr("not a uuid")})
	wantNotesRefusal(t, "UpdateNote(blank)", err, connect.CodeInvalidArgument, "text must not be empty")
	rig.scenes.mu.Lock()
	rig.scenes.err = errors.New("maps is down")
	rig.scenes.mu.Unlock()
	_, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: n0.GetId(), Text: ptr("x"), ScenePointId: ptr("not a uuid")})
	wantNotesFailure(t, "UpdateNote(scenes fail)", err, connect.CodeInternal)
	rig.scenes.mu.Lock()
	rig.scenes.err = nil
	rig.scenes.mu.Unlock()
	_, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0a1", ScenePointId: ptr(rig.sceneIDs[2])})
	wantNotesRefusal(t, "UpdateNote(undiscovered tag, unknown note)", err, connect.CodeInvalidArgument, "scene_point_id must be a scene the group discovered")
	_, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0a1", Text: ptr("x")})
	wantNotesRefusal(t, "UpdateNote(unknown note)", err, connect.CodeNotFound, "note not found")
	_, err = update(rig.client(rig.bia), &notesv1.UpdateNoteRequest{NoteId: n0.GetId(), Text: ptr("x")})
	wantNotesRefusal(t, "UpdateNote(another player's note)", err, connect.CodeNotFound, "note not found")

	// What each change keeps.
	rig.clockMu.Lock()
	rig.now = rig.now.Add(time.Minute)
	rig.clockMu.Unlock()
	got, err := update(ana, &notesv1.UpdateNoteRequest{NoteId: strings.ToUpper(n0.GetId()), Text: ptr(" novo texto ")})
	if err != nil {
		t.Fatalf("UpdateNote(text) error = %v", err)
	}
	wantNotesNote(t, "UpdateNote(text)", got, "novo texto", rig.sceneIDs[0], "A ponte")
	if !got.GetUpdatedAt().AsTime().Equal(rig.clock()) || !got.GetCreatedAt().AsTime().Equal(n0.GetCreatedAt().AsTime()) {
		t.Errorf("UpdateNote(text) times = %v, %v", got.GetCreatedAt().AsTime(), got.GetUpdatedAt().AsTime())
	}
	got, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: n0.GetId(), ScenePointId: ptr(rig.sceneIDs[1])})
	if err != nil {
		t.Fatalf("UpdateNote(tag) error = %v", err)
	}
	wantNotesNote(t, "UpdateNote(tag)", got, "novo texto", rig.sceneIDs[1], "A torre")
	got, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: n0.GetId(), ScenePointId: ptr("")})
	if err != nil {
		t.Fatalf("UpdateNote(untag) error = %v", err)
	}
	wantNotesNote(t, "UpdateNote(untag)", got, "novo texto", "", "")
	got, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: n0.GetId(), Text: ptr("os dois"), ScenePointId: ptr(rig.sceneIDs[0])})
	if err != nil {
		t.Fatalf("UpdateNote(both) error = %v", err)
	}
	wantNotesNote(t, "UpdateNote(both)", got, "os dois", rig.sceneIDs[0], "A ponte")

	// A tag whose scene is no longer discovered is kept, and not shown.
	rig.scenes.mu.Lock()
	rig.scenes.discovered = rig.scenes.discovered[1:]
	rig.scenes.mu.Unlock()
	got, err = update(ana, &notesv1.UpdateNoteRequest{NoteId: n0.GetId(), Text: ptr("escondida")})
	if err != nil {
		t.Fatalf("UpdateNote(hidden tag) error = %v", err)
	}
	wantNotesNote(t, "UpdateNote(hidden tag)", got, "escondida", "", "")
	var stored *string
	if err := rig.pool.QueryRow(ctx, `SELECT scene_point_id::TEXT FROM player_notes WHERE id = $1`, n0.GetId()).Scan(&stored); err != nil || stored == nil || *stored != rig.sceneIDs[0] {
		t.Errorf("stored tag = %v, %v, want %s kept", stored, err, rig.sceneIDs[0])
	}

	// Each failing query.
	for _, tc := range []struct{ query string }{
		{"GetPlayerNoteForUpdate"},
		{"UpdatePlayerNote"},
	} {
		rig.faults.Arm(tc.query)
		_, err := update(ana, &notesv1.UpdateNoteRequest{NoteId: n0.GetId(), Text: ptr("falha")})
		rig.faults.Disarm()
		if rig.faults.Hits(tc.query) == 0 {
			t.Fatalf("%s never ran", tc.query)
		}
		wantNotesFailure(t, "UpdateNote("+tc.query+")", err, connect.CodeCanceled)
	}
	var text string
	if err := rig.pool.QueryRow(ctx, `SELECT text FROM player_notes WHERE id = $1`, n0.GetId()).Scan(&text); err != nil || text != "escondida" {
		t.Errorf("stored text = %q, %v, want the last good one", text, err)
	}
}

// mustNotesCreate creates a note as c, or fails the test.
func mustNotesCreate(t *testing.T, c notesv1connect.NotesServiceClient, campaignID, text, scene string) *notesv1.Note {
	t.Helper()
	res, err := c.CreateNote(t.Context(), connect.NewRequest(&notesv1.CreateNoteRequest{CampaignId: campaignID, Text: text, ScenePointId: scene}))
	if err != nil {
		t.Fatalf("CreateNote() error = %v", err)
	}
	return res.Msg.GetNote()
}

// connectMessage returns the message of a Connect error, without the code.
func connectMessage(err error) string {
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		return ce.Message()
	}
	return ""
}
