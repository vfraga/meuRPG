package characters

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The characterization of the sheet editor's calls (CreateCharacter, UpdateCharacter,
// UpdateCharacterStory, RollAbilityScores, ListCharacters and the Character a member
// reads): the web reads each refusal's code, message and detail to put the message under
// the right input, so these tests freeze them byte for byte, with the order of the checks,
// what is written and what is logged. A refactor of the handlers must leave every one as it
// is.

// editorRefusal renders an error as the web reads it: the Connect code, the message and
// each detail, in a stable text ("ok" for no error).
func editorRefusal(err error) string {
	if err == nil {
		return "ok"
	}
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		return "not a connect error: " + err.Error()
	}
	out := ce.Code().String() + ": " + ce.Message()
	for _, d := range ce.Details() {
		v, derr := d.Value()
		if derr != nil {
			out += " | undecodable " + d.Type()
			continue
		}
		out += " | " + editorMessage(v.ProtoReflect())
	}
	return out
}

// editorMessage renders a message with its set fields in declaration order, so the text
// never depends on the protobuf library's randomized formatting. A timestamp reads
// "<time>": the tests' clocks move.
func editorMessage(m protoreflect.Message) string {
	if m.Descriptor().FullName() == "google.protobuf.Timestamp" {
		return "<time>"
	}
	var parts []string
	fields := m.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if !m.Has(fd) {
			continue
		}
		parts = append(parts, string(fd.Name())+"="+editorField(fd, m.Get(fd)))
	}
	return string(m.Descriptor().Name()) + "{" + strings.Join(parts, " ") + "}"
}

func editorField(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	switch {
	case fd.IsList():
		l := v.List()
		items := make([]string, l.Len())
		for i := range l.Len() {
			items[i] = editorScalar(fd, l.Get(i))
		}
		return "[" + strings.Join(items, ", ") + "]"
	case fd.IsMap():
		var items []string
		v.Map().Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
			items = append(items, k.String()+":"+editorScalar(fd.MapValue(), mv))
			return true
		})
		slices.Sort(items)
		return "{" + strings.Join(items, ", ") + "}"
	}
	return editorScalar(fd, v)
}

func editorScalar(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	switch fd.Kind() {
	case protoreflect.EnumKind:
		if ev := fd.Enum().Values().ByNumber(v.Enum()); ev != nil {
			return string(ev.Name())
		}
		return strconv.Itoa(int(v.Enum()))
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return editorMessage(v.Message())
	case protoreflect.StringKind:
		return strconv.Quote(v.String())
	default:
		return v.String()
	}
}

// wantEditorRefusal fails the test unless err renders as want.
func wantEditorRefusal(t *testing.T, call string, err error, want string) {
	t.Helper()
	if got := editorRefusal(err); got != want {
		t.Errorf("%s:\n got  %s\n want %s", call, got, want)
	}
}

// errEditorContentDown is the failure editorSource answers with.
var errEditorContentDown = errors.New("the content source is down")

// editorSource is the campaign's content source with the knobs the editor's tests need:
// a read outside a transaction (failOutside) or inside one (failInside) that fails; a
// hook run once right after the next read outside a transaction (what it commits lands
// between a handler's first read and its transaction); and, inside a transaction, the
// content of another campaign (swapInside[from] = to), a content that moved since the
// handler read it.
type editorSource struct {
	inner                   ContentSource
	failOutside, failInside atomic.Bool
	mu                      sync.Mutex
	hook                    func()
	swapInside              map[string]string
}

func (e *editorSource) campaign(tx pgx.Tx, campaignID string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if to, ok := e.swapInside[campaignID]; ok && tx != nil {
		return to
	}
	return campaignID
}

func (e *editorSource) fail(tx pgx.Tx) bool {
	if tx == nil {
		return e.failOutside.Load()
	}
	return e.failInside.Load()
}

func (e *editorSource) after(tx pgx.Tx) {
	if tx != nil {
		return
	}
	e.mu.Lock()
	hook := e.hook
	e.hook = nil
	e.mu.Unlock()
	if hook != nil {
		hook()
	}
}

func (e *editorSource) ContentFor(ctx context.Context, tx pgx.Tx, campaignID string) (*rules.Content, error) {
	if e.fail(tx) {
		return nil, errEditorContentDown
	}
	c, err := e.inner.ContentFor(ctx, tx, e.campaign(tx, campaignID))
	e.after(tx)
	return c, err
}

func (e *editorSource) For(ctx context.Context, tx pgx.Tx, campaignID string) (*rules.Content, TableRules, error) {
	if e.fail(tx) {
		return nil, TableRules{}, errEditorContentDown
	}
	c, tr, err := e.inner.For(ctx, tx, e.campaign(tx, campaignID))
	e.after(tx)
	return c, tr, err
}

// editorTable is a campaign with a master, two players (Dona and Caio), a pending member
// (Pia) and a stranger, served by a service whose content source is an editorSource and
// whose log is kept.
type editorTable struct {
	h                                *harness
	logs                             *logBuffer
	src                              *editorSource
	dice                             *testDiceRules
	master, dona, caio, pia, visitor *user
	campaign                         string
}

// newEditorTable builds the table; tweak changes the service's Config further.
func newEditorTable(t *testing.T, tweak func(*Config)) *editorTable {
	t.Helper()
	et := &editorTable{logs: &logBuffer{}, dice: &testDiceRules{}}
	et.dice.rule.Store(int32(charactersv1.LevelUpDiceRule_LEVEL_UP_DICE_RULE_PLAYER_CHOOSES))
	et.h = newHarnessWith(t, func(c *Config) {
		c.Logger = slog.New(slog.NewJSONHandler(et.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		et.src = &editorSource{inner: c.Content}
		c.Content, c.Dice = et.src, et.dice
		if tweak != nil {
			tweak(c)
		}
	})
	et.master, et.dona, et.caio = et.h.newUser("Mestre"), et.h.newUser("Dona"), et.h.newUser("Caio")
	et.pia, et.visitor = et.h.newUser("Pia"), et.h.newUser("Visitante")
	et.campaign = et.h.newCampaign(et.master, "Mirathel", et.dona, et.caio)
	et.h.joinPending(et.master, et.campaign, et.pia)
	return et
}

// editorLogged fails the test unless the log has a line with every fragment.
func (et *editorTable) editorLogged(t *testing.T, fragments ...string) {
	t.Helper()
	for line := range strings.Lines(et.logs.String()) {
		all := true
		for _, f := range fragments {
			all = all && strings.Contains(line, f)
		}
		if all {
			return
		}
	}
	t.Errorf("no log line has %q; the log:\n%s", fragments, et.logs.String())
}

// editorRow is the owner and the state columns of a stored character.
func (et *editorTable) editorRow(t *testing.T, id string) string {
	t.Helper()
	var player, master *string
	var status string
	var reserved bool
	err := et.h.pool.QueryRow(t.Context(),
		"SELECT player_user_id, master_user_id, status, reserved FROM characters WHERE id = $1", id).Scan(&player, &master, &status, &reserved)
	if err != nil {
		t.Fatalf("read character %s: %v", id, err)
	}
	name := func(p *string) string {
		switch {
		case p == nil:
			return "none"
		case *p == et.master.id:
			return "Mestre"
		case *p == et.dona.id:
			return "Dona"
		case *p == et.caio.id:
			return "Caio"
		case *p == et.pia.id:
			return "Pia"
		}
		return "someone else"
	}
	return "player=" + name(player) + " master=" + name(master) + " status=" + status + " reserved=" + strconv.FormatBool(reserved)
}

// editorCount counts the campaign's characters.
func (et *editorTable) editorCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := et.h.pool.QueryRow(t.Context(), "SELECT count(*) FROM characters WHERE campaign_id = $1", et.campaign).Scan(&n); err != nil {
		t.Fatalf("count characters: %v", err)
	}
	return n
}

// editorCorrupt overwrites a stored column of a character with a document no version
// can read.
func (et *editorTable) editorCorrupt(t *testing.T, id, column string) {
	t.Helper()
	if _, err := et.h.pool.Exec(t.Context(), "UPDATE characters SET "+column+` = '{"full": 1, "backstory": 2}' WHERE id = $1`, id); err != nil {
		t.Fatalf("corrupt the %s of %s: %v", column, id, err)
	}
}

// editorGallery is a gallery that knows images by campaign, counts the calls it gets,
// and can fail: failCheck makes every lookup fail, failInsert makes the copy of a fog
// map's background (fog) fail inside the transaction, and reused makes the copy's gallery
// row one that was there already.
type editorGallery struct {
	images                                map[string][]string
	fog, copyID                           string
	failCheck, failInsert, reused         atomic.Bool
	looked, prepared, inserted, discarded atomic.Int32
}

var errEditorGalleryDown = errors.New("the gallery is down")

func (g *editorGallery) PortraitImage(_ context.Context, campaignID, imageID string) (string, bool, error) {
	g.looked.Add(1)
	if g.failCheck.Load() {
		return "", false, errEditorGalleryDown
	}
	return imageID, slices.Contains(g.images[campaignID], imageID), nil
}

func (g *editorGallery) PreparePortrait(ctx context.Context, campaignID, imageID string) (string, bool, PortraitCopy, error) {
	use, found, err := g.PortraitImage(ctx, campaignID, imageID)
	if err != nil || !found || imageID != g.fog {
		return use, found, nil, err
	}
	g.prepared.Add(1)
	return g.copyID, true, editorCopy{g}, nil
}

// editorCopy is the copy of a fog map's background.
type editorCopy struct{ g *editorGallery }

func (c editorCopy) Insert(context.Context, pgx.Tx) (string, bool, error) {
	if c.g.failInsert.Load() {
		return "", false, errEditorGalleryDown
	}
	c.g.inserted.Add(1)
	return c.g.copyID, !c.g.reused.Load(), nil
}

func (c editorCopy) Discard(context.Context) { c.g.discarded.Add(1) }

// gearHostSpy is package play as the sheet editor sees it: it records which of the
// armor calls a save made, and the calls made after the commit.
type gearHostSpy struct {
	mu    sync.Mutex
	calls []string
}

func (g *gearHostSpy) record(call string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, call)
}

func (g *gearHostSpy) take() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := strings.Join(g.calls, ", ")
	g.calls = nil
	return out
}

func (*gearHostSpy) PublishCharacterEvent(context.Context, string, string, string, string) {}

func (*gearHostSpy) ReviveInCombat(context.Context, pgx.Tx, string, string, string, time.Time) (string, error) {
	return "", nil
}

func (g *gearHostSpy) ArmorWorn(context.Context, pgx.Tx, string, string) (func(context.Context), error) {
	g.record("ArmorWorn")
	return func(context.Context) { g.record("after ArmorWorn") }, nil
}

func (g *gearHostSpy) GearChanged(context.Context, pgx.Tx, string, string) (func(context.Context), error) {
	g.record("GearChanged")
	return func(context.Context) { g.record("after GearChanged") }, nil
}

func (*gearHostSpy) PublishEncounterChanged(context.Context, string, string) {}

// editorClone is a copy of a sheet changed by edit.
func editorClone(sheet *charactersv1.CharacterSheet, edit func(f *charactersv1.FullSheet)) *charactersv1.CharacterSheet {
	out := proto.CloneOf(sheet)
	edit(out.GetFull())
	return out
}
