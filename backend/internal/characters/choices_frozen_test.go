package characters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Characterization of CompleteCharacterChoices (with cleanChoicePicks and
// applyChoicePicks) and GetCampaignOpenChoices: every refusal with its code, message and
// detail, in the order the checks run, what each viewer reads, the writes, the history
// line, the live hint and the log lines. The answers are golden files
// (testdata/choices_*.golden.json, see choicesGolden).

// choicesRefusalShape is what a refusal tells the app: the code, the message and each
// detail with its type. Nil for no error.
func choicesRefusalShape(t *testing.T, err error) map[string]any {
	t.Helper()
	if err == nil {
		return nil
	}
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		return map[string]any{"plain": err.Error()}
	}
	out := map[string]any{"code": ce.Code().String(), "message": ce.Message()}
	var details []any
	for _, d := range ce.Details() {
		v, derr := d.Value()
		if derr != nil {
			t.Fatalf("decode error detail: %v", derr)
		}
		raw, merr := protojson.Marshal(v)
		if merr != nil {
			t.Fatal(merr)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		details = append(details, map[string]any{"type": d.Type(), "value": value})
	}
	if details != nil {
		out["details"] = details
	}
	return out
}

// choicesProtoJSON is a message as a JSON value, for a golden list.
func choicesProtoJSON(t *testing.T, m proto.Message) any {
	t.Helper()
	raw, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestCleanChoicePicksFrozen freezes the shape checks of the picks: each refusal with its
// field and message, in order, and what a good pick becomes.
func TestCleanChoicePicksFrozen(t *testing.T) {
	t.Parallel()
	many := make([]*charactersv1.ChoicePick, maxChoicePicks+1)
	for i := range many {
		many[i] = pick("feature:x")
	}
	cases := []struct {
		name  string
		picks []*charactersv1.ChoicePick
	}{
		{"no picks", nil},
		{"an empty list", []*charactersv1.ChoicePick{}},
		{"51 picks", many},
		{"50 picks", many[:maxChoicePicks]},
		{"an empty key", []*charactersv1.ChoicePick{pick("")}},
		{"a key of 121", []*charactersv1.ChoicePick{pick(strings.Repeat("k", maxChoiceKeyLength+1))}},
		{"a key of 120", []*charactersv1.ChoicePick{pick(strings.Repeat("k", maxChoiceKeyLength))}},
		{"too many options", []*charactersv1.ChoicePick{pick("feature:x", make([]string, rules.MaxListLength+1)...)}},
		{"too many texts", []*charactersv1.ChoicePick{{ChoiceKey: "feature:x", Texts: make([]string, rules.MaxChoiceTexts+1)}}},
		{"a long text", []*charactersv1.ChoicePick{{ChoiceKey: "feature:x", Texts: []string{strings.Repeat("o", rules.MaxChoiceTextLength+1)}}}},
		{"a control character", []*charactersv1.ChoicePick{{ChoiceKey: "feature:x", Texts: []string{"orc\x07s"}}}},
		{"invalid UTF-8", []*charactersv1.ChoicePick{{ChoiceKey: "feature:x", Texts: []string{"orc\xffs"}}}},
		{"blank texts stay open, the others are trimmed", []*charactersv1.ChoicePick{{ChoiceKey: "feature:x", Texts: []string{"   ", "  gnolls ", ""}}}},
		{"the key is checked before the texts", []*charactersv1.ChoicePick{{ChoiceKey: "", Texts: []string{"\x07"}}}},
		{"the counts are checked before the texts", []*charactersv1.ChoicePick{{ChoiceKey: "feature:x", OptionKeys: make([]string, rules.MaxListLength+1), Texts: []string{"\x07"}}}},
		{"the first bad pick wins", []*charactersv1.ChoicePick{pick("feature:a", "o"), {ChoiceKey: "feature:b", Texts: []string{"\x07"}}, pick("")}},
		// Finding: slices.Compact only drops a repeated option next to its twin, so
		// ["a", "b", "a"] keeps both a's (applyChoicePicks then counts the repeat as a
		// second pick).
		{"repeated options", []*charactersv1.ChoicePick{pick("feature:x", "a", "a", "b", "a")}},
	}
	var got []any
	for _, tc := range cases {
		in := proto.Clone(&charactersv1.CompleteCharacterChoicesRequest{Picks: tc.picks})
		out, err := cleanChoicePicks(tc.picks)
		if !proto.Equal(in, &charactersv1.CompleteCharacterChoicesRequest{Picks: tc.picks}) {
			t.Errorf("%s: cleanChoicePicks changed its input", tc.name)
		}
		entry := map[string]any{"case": tc.name}
		if err != nil {
			entry["error"] = err.Error()
			if fe, ok := errors.AsType[*fieldError](err); ok {
				entry["field"] = fe.field
			}
			entry["refusal"] = choicesRefusalShape(t, invalidArgument(err))
		} else {
			entry["picks"] = choicesProtoJSON(t, &charactersv1.CompleteCharacterChoicesRequest{Picks: out})
			entry["count"] = len(out)
		}
		got = append(got, entry)
	}
	choicesGolden(t, "choices_clean_picks", got)
}

// TestApplyChoicePicksFrozen freezes how the picks are laid over a stored sheet: every
// refusal (with its detail) in the order of the checks, the sheet and the labels a good
// call gives, and that the stored sheet is never changed.
func TestApplyChoicePicksFrozen(t *testing.T) {
	t.Parallel()
	content := loadRules(t)
	key := rules.AbilityChoiceKey("race:half-elf")
	const (
		style       = "feature:fighter-fighting-style"
		invocations = "feature:eldritch-invocations"
		enemy       = "feature:favored-enemy-1-type"
		terrain     = "feature:natural-explorer-1-terrain-type"
	)
	halfElf := choiceSheet("race:half-elf", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense").GetFull()
	halfElfDex := choiceSheet("race:half-elf", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense", rules.ScopedChoice(key, "ability:dex")).GetFull()
	halfElfDex.FeatureChoiceText = map[string]string{"other#1": "kept"}
	warlock := choiceSheet("race:human", "", "class:warlock", "", 5, "feature:pact-of-the-chain",
		"feature:eldritch-invocation-armor-of-shadows", "feature:eldritch-invocation-devils-sight").GetFull()
	warlock.CantripKeys = []string{"spell:mage-hand", "spell:chill-touch"}
	ranger := choiceSheet("race:human", "", "class:ranger", "", 1, rules.ScopedChoice(terrain, "terrain:forest")).GetFull()
	humanoid := func(texts ...string) *charactersv1.ChoicePick {
		return &charactersv1.ChoicePick{ChoiceKey: enemy, OptionKeys: []string{"creature-type:humanoid"}, Texts: texts}
	}
	cases := []struct {
		name  string
		sheet *charactersv1.FullSheet
		picks []*charactersv1.ChoicePick
	}{
		{"both abilities", halfElf, []*charactersv1.ChoicePick{pick(key, "ability:dex", "ability:con")}},
		{"one ability of two", halfElf, []*charactersv1.ChoicePick{pick(key, "ability:dex")}},
		{"no option", halfElf, []*charactersv1.ChoicePick{pick(key)}},
		{"the same choice twice", halfElf, []*charactersv1.ChoicePick{pick(key, "ability:dex"), pick(key, "ability:con")}},
		{"a choice the sheet does not have", halfElf, []*charactersv1.ChoicePick{pick("feature:metamagic-1", "feature:metamagic-careful-spell")}},
		{"a choice already made", halfElf, []*charactersv1.ChoicePick{pick(style, "feature:fighter-fighting-style-archery")}},
		{"an option the choice does not list", halfElf, []*charactersv1.ChoicePick{pick(key, "ability:nope")}},
		{"three abilities for two", halfElf, []*charactersv1.ChoicePick{pick(key, "ability:dex", "ability:con", "ability:str")}},
		// Finding (with cleanChoicePicks'): a repeated option counts twice, and the answer
		// is CHOICE_NOT_OFFERED rather than a refusal of the repeat.
		{"a repeated option", halfElf, []*charactersv1.ChoicePick{pick(key, "ability:dex", "ability:con", "ability:dex")}},
		{"the option already made, alone", halfElfDex, []*charactersv1.ChoicePick{pick(key, "ability:dex")}},
		{"the option already made and a new one", halfElfDex, []*charactersv1.ChoicePick{pick(key, "ability:dex", "ability:con")}},
		{"a good pick, then a choice not open", halfElf, []*charactersv1.ChoicePick{pick(key, "ability:dex"), pick("feature:nope")}},
		{"a good pick, then the same again", halfElf, []*charactersv1.ChoicePick{pick(key, "ability:dex"), pick(key, "ability:dex")}},
		{"an unmet prerequisite", warlock, []*charactersv1.ChoicePick{pick(invocations, "feature:eldritch-invocation-agonizing-blast")}},
		{"an unmet prerequisite before an unknown option", warlock, []*charactersv1.ChoicePick{pick(invocations, "feature:eldritch-invocation-agonizing-blast", "feature:nope")}},
		{"an unknown option before an unmet prerequisite", warlock, []*charactersv1.ChoicePick{pick(invocations, "feature:nope", "feature:eldritch-invocation-agonizing-blast")}},
		{"the open invocation", warlock, []*charactersv1.ChoicePick{pick(invocations, "feature:eldritch-invocation-mask-of-many-faces")}},
		{"the humanoid with one race", ranger, []*charactersv1.ChoicePick{humanoid("gnolls")}},
		{"the humanoid with a race not written", ranger, []*charactersv1.ChoicePick{humanoid("gnolls", "")}},
		{"the humanoid with three races", ranger, []*charactersv1.ChoicePick{humanoid("gnolls", "orcs", "kobolds")}},
		{"the humanoid with two races", ranger, []*charactersv1.ChoicePick{humanoid("gnolls", "orcs")}},
		{"the humanoid with two races and no language", ranger, []*charactersv1.ChoicePick{humanoid("gnolls", "orcs"), pick(enemy+"#language", rules.LanguageNone)}},
		// Finding: every pick of a call is checked against what the stored sheet offers,
		// so a language that only the enemy picked in the same call would offer (Gnoll)
		// is refused as not offered.
		{"the humanoid with two races and their language", ranger, []*charactersv1.ChoicePick{humanoid("gnolls", "orcs"), pick(enemy+"#language", "language:gnoll")}},
		// A text sent with an option that takes none is dropped without a word.
		{"a beast with texts", ranger, []*charactersv1.ChoicePick{{ChoiceKey: enemy, OptionKeys: []string{"creature-type:beast"}, Texts: []string{"lobos", "ursos"}}}},
	}
	var got []any
	for _, tc := range cases {
		before := proto.CloneOf(tc.sheet)
		next, labels, err := applyChoicePicks(content, tc.sheet, tc.picks)
		if !proto.Equal(before, tc.sheet) {
			t.Errorf("%s: applyChoicePicks changed the stored sheet", tc.name)
		}
		entry := map[string]any{"case": tc.name}
		if err != nil {
			entry["refusal"] = choicesRefusalShape(t, err)
		} else {
			entry["feature_choice_keys"] = next.GetFeatureChoiceKeys()
			entry["feature_choice_text"] = next.GetFeatureChoiceText()
			entry["text_map_is_nil"] = next.FeatureChoiceText == nil
			entry["labels"] = labels
			rest := proto.CloneOf(next)
			rest.FeatureChoiceKeys, rest.FeatureChoiceText = tc.sheet.GetFeatureChoiceKeys(), tc.sheet.GetFeatureChoiceText()
			if !proto.Equal(rest, tc.sheet) {
				t.Errorf("%s: applyChoicePicks changed more than the picks and the texts", tc.name)
			}
		}
		got = append(got, entry)
	}
	choicesGolden(t, "choices_apply_picks", got)
}

// choicesLogBuffer keeps the service's log lines, without their time, for a golden.
type choicesLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *choicesLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// take returns the lines written since the last take, as JSON values.
func (b *choicesLogBuffer) take(t *testing.T) []any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []any
	for line := range strings.SplitSeq(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		out = append(out, v)
	}
	b.buf.Reset()
	return out
}

// choicesEventHost is the play module's side of a completed choice: it records the
// history lines, or fails them when fail is set.
type choicesEventHost struct {
	CreatureHost
	mu     sync.Mutex
	events []string
	fail   bool
}

func (e *choicesEventHost) AppendEvent(_ context.Context, _ pgx.Tx, campaignID, kind, userID string, payload []byte, _ time.Time) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fail {
		return false, errors.New("the history is down")
	}
	e.events = append(e.events, campaignID+" "+kind+" "+userID+" "+string(payload))
	return true, nil
}

func (e *choicesEventHost) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

// choicesFrozenTable is a campaign with a master, two players, a pending member and a
// stranger, the service's logs, history and live hint recorded.
type choicesFrozenTable struct {
	h                                 *harness
	master, ana, caio, pending, alien *user
	campaign                          string
	logs                              *choicesLogBuffer
	host                              *choicesEventHost
	live                              *liveSpy
	names                             []string // id -> name pairs for the golden files
}

func newChoicesFrozenTable(t *testing.T) *choicesFrozenTable {
	t.Helper()
	tb := &choicesFrozenTable{logs: &choicesLogBuffer{}, host: &choicesEventHost{}, live: &liveSpy{}}
	logger := slog.New(slog.NewJSONHandler(tb.logs, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
	tb.h = newHarnessWith(t, func(c *Config) { c.Logger = logger })
	tb.h.svc.SetCreatureHost(tb.host)
	tb.h.svc.SetLive(tb.live)
	tb.master, tb.ana, tb.caio = tb.h.newUser("Samuel"), tb.h.newUser("Ana"), tb.h.newUser("Caio")
	tb.pending, tb.alien = tb.h.newUser("Pendente"), tb.h.newUser("Estranho")
	tb.campaign = tb.h.newCampaign(tb.master, "Mirathel", tb.ana, tb.caio)
	tb.h.joinPending(tb.master, tb.campaign, tb.pending)
	tb.names = []string{tb.campaign, "<campaign>", tb.master.id, "<master>", tb.ana.id, "<ana>", tb.caio.id, "<caio>", tb.pending.id, "<pending>", tb.alien.id, "<alien>"}
	return tb
}

// name records the name a character's id takes in the golden files.
func (tb *choicesFrozenTable) name(c *charactersv1.Character, name string) *charactersv1.Character {
	tb.names = append(tb.names, c.GetId(), "<"+name+">")
	return c
}

func (tb *choicesFrozenTable) complete(u *user, id string, revision int32, picks ...*charactersv1.ChoicePick) (*charactersv1.Character, error) {
	res, err := u.api.CompleteCharacterChoices(context.Background(), connect.NewRequest(&charactersv1.CompleteCharacterChoicesRequest{
		CampaignId: tb.campaign, CharacterId: id, ExpectedRevision: revision, Picks: picks,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

// choicesCharacterView is a character for a golden file: no timestamps, and no rules
// text in the features of its derived sheet.
func choicesCharacterView(c *charactersv1.Character) *charactersv1.Character {
	out := proto.CloneOf(c)
	out.CreatedAt, out.UpdatedAt, out.SheetLockedAt, out.DiedAt = nil, nil, nil, nil
	for _, f := range out.GetDerived().GetFeatures() {
		f.Description, f.DescriptionPt = "", ""
	}
	return out
}

// TestCompleteCharacterChoicesFrozen freezes CompleteCharacterChoices: each refusal in the
// order of the checks (who calls, the id, the revision, the picks, the character, its
// sheet, the rules), that a refusal writes nothing (no revision, no history line, no live
// hint, no log line of success), and what the owner and the master read after a good
// call, with the history line, the live hint and the log line.
func TestCompleteCharacterChoicesFrozen(t *testing.T) {
	t.Parallel()
	tb := newChoicesFrozenTable(t)
	key := rules.AbilityChoiceKey("race:half-elf")
	open := func(u *user, name string, complete, open *charactersv1.CharacterSheet) *charactersv1.Character {
		t.Helper()
		c, err := u.createWith(tb.campaign, unspecifiedMethod, complete)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		c, err = tb.master.update(t, c, name, open)
		if err != nil {
			t.Fatalf("the master opens a choice of %s: %v", name, err)
		}
		return tb.name(c, name)
	}
	halfElf := open(tb.ana, "half-elf",
		choiceSheet("race:half-elf", "", "class:fighter", "", 1, rules.ScopedChoice(key, "ability:dex"), rules.ScopedChoice(key, "ability:con"), "feature:fighter-fighting-style-defense"),
		choiceSheet("race:half-elf", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense"))
	caios := open(tb.caio, "caios-fighter",
		choiceSheet("race:human", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense"),
		choiceSheet("race:human", "", "class:fighter", "", 1))
	story := tb.name(tb.master.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_STORY, "Taverneiro", basicSheet()), "story-npc")
	enemy := tb.name(tb.master.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, "Bandido", enemySheet()), "enemy")
	tb.h.lockSheets(tb.campaign)
	halfElf = tb.ana.get(t, tb.campaign, halfElf.GetId())
	caios = tb.caio.get(t, tb.campaign, caios.GetId())
	story = tb.master.get(t, tb.campaign, story.GetId())
	enemy = tb.master.get(t, tb.campaign, enemy.GetId())
	tb.logs.take(t)

	good := pick(key, "ability:dex", "ability:con")
	const unknownID = "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e002"
	type step struct {
		name     string
		u        *user
		id       string
		revision int32
		picks    []*charactersv1.ChoicePick
	}
	steps := []step{
		{"nobody signed in", tb.h.anonymous(), halfElf.GetId(), halfElf.GetRevision(), []*charactersv1.ChoicePick{good}},
		{"a stranger", tb.alien, halfElf.GetId(), halfElf.GetRevision(), []*charactersv1.ChoicePick{good}},
		{"a pending member", tb.pending, halfElf.GetId(), halfElf.GetRevision(), []*charactersv1.ChoicePick{good}},
		{"an id that is no uuid, before the revision and the picks", tb.ana, "not-an-id", 0, nil},
		{"revision 0, before the picks", tb.ana, halfElf.GetId(), 0, nil},
		{"a negative revision", tb.ana, halfElf.GetId(), -3, []*charactersv1.ChoicePick{good}},
		{"no picks", tb.ana, halfElf.GetId(), halfElf.GetRevision(), nil},
		{"a bad pick shape", tb.ana, halfElf.GetId(), halfElf.GetRevision(), []*charactersv1.ChoicePick{good, pick("")}},
		{"a character that does not exist", tb.ana, unknownID, halfElf.GetRevision(), []*charactersv1.ChoicePick{good}},
		{"another player's character", tb.ana, caios.GetId(), caios.GetRevision(), []*charactersv1.ChoicePick{pick("feature:fighter-fighting-style", "feature:fighter-fighting-style-archery")}},
		{"an NPC, for a player", tb.ana, enemy.GetId(), enemy.GetRevision(), []*charactersv1.ChoicePick{good}},
		{"a stale revision", tb.ana, halfElf.GetId(), halfElf.GetRevision() + 7, []*charactersv1.ChoicePick{good}},
		{"a stale revision with a choice not open", tb.ana, halfElf.GetId(), halfElf.GetRevision() + 7, []*charactersv1.ChoicePick{pick("feature:nope")}},
		{"a basic sheet, for the master", tb.master, story.GetId(), story.GetRevision(), []*charactersv1.ChoicePick{good}},
		{"a basic sheet with a stale revision", tb.master, story.GetId(), story.GetRevision() + 1, []*charactersv1.ChoicePick{good}},
		{"a choice not open", tb.ana, halfElf.GetId(), halfElf.GetRevision(), []*charactersv1.ChoicePick{pick("feature:nope")}},
		{"a choice already made", tb.ana, halfElf.GetId(), halfElf.GetRevision(), []*charactersv1.ChoicePick{pick("feature:fighter-fighting-style", "feature:fighter-fighting-style-archery")}},
		{"an option not listed", tb.ana, halfElf.GetId(), halfElf.GetRevision(), []*charactersv1.ChoicePick{pick(key, "ability:nope")}},
	}
	var got []any
	for _, s := range steps {
		_, err := tb.complete(s.u, s.id, s.revision, s.picks...)
		if err == nil {
			t.Fatalf("%s: want a refusal", s.name)
		}
		got = append(got, map[string]any{"case": s.name, "refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)})
	}
	if ev := tb.host.take(); len(ev) != 0 || tb.live.count() != 0 {
		t.Errorf("a refusal wrote history %v or sent %d live hints", ev, tb.live.count())
	}
	if now := tb.ana.get(t, tb.campaign, halfElf.GetId()); now.GetRevision() != halfElf.GetRevision() {
		t.Errorf("a refusal moved the revision to %d", now.GetRevision())
	}

	// A history that fails rolls the write back, and the caller gets the internal error.
	tb.host.fail = true
	_, err := tb.complete(tb.ana, halfElf.GetId(), halfElf.GetRevision(), good)
	got = append(got, map[string]any{"case": "the history fails", "refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)})
	tb.host.fail = false
	if now := tb.ana.get(t, tb.campaign, halfElf.GetId()); now.GetRevision() != halfElf.GetRevision() || tb.live.count() != 0 {
		t.Errorf("the failed history left revision %d and %d live hints", now.GetRevision(), tb.live.count())
	}
	tb.logs.take(t)

	// The owner completes one ability, then the other; the master completes Caio's style.
	first, err := tb.complete(tb.ana, halfElf.GetId(), halfElf.GetRevision(), pick(key, "ability:dex"))
	if err != nil {
		t.Fatalf("the owner's first ability: %v", err)
	}
	got = append(got, map[string]any{
		"case": "the owner's first ability", "character": choicesProtoJSON(t, choicesCharacterView(first)),
		"history": tb.host.take(), "live": tb.live.count(), "logs": tb.logs.take(t),
	})
	second, err := tb.complete(tb.ana, halfElf.GetId(), first.GetRevision(), pick(key, "ability:dex", "ability:con"))
	if err != nil {
		t.Fatalf("the owner's second ability: %v", err)
	}
	got = append(got, map[string]any{
		"case": "the owner's second ability, the first sent again", "character": choicesProtoJSON(t, choicesCharacterView(second)),
		"history": tb.host.take(), "live": tb.live.count(), "logs": tb.logs.take(t),
	})
	byMaster, err := tb.complete(tb.master, caios.GetId(), caios.GetRevision(), pick("feature:fighter-fighting-style", "feature:fighter-fighting-style-archery"))
	if err != nil {
		t.Fatalf("the master completes Caio's style: %v", err)
	}
	got = append(got, map[string]any{
		"case": "the master completes another's style", "character": choicesProtoJSON(t, choicesCharacterView(byMaster)),
		"history": tb.host.take(), "live": tb.live.count(), "logs": tb.logs.take(t),
	})
	// The master completes an NPC's open style too.
	npc, err := tb.complete(tb.master, enemy.GetId(), enemy.GetRevision(), pick("feature:fighter-fighting-style", "feature:fighter-fighting-style-archery"))
	if err != nil {
		t.Fatalf("the master completes the NPC's style: %v", err)
	}
	got = append(got, map[string]any{
		"case": "the master completes an NPC's style", "character": choicesProtoJSON(t, choicesCharacterView(npc)),
		"history": tb.host.take(), "live": tb.live.count(), "logs": tb.logs.take(t),
	})
	wrote := proto.CloneOf(choicesCharacterView(byMaster).GetSheet())
	wrote.GetFull().KnownIssues = nil // the server's bookkeeping: only the master receives it
	if read := tb.caio.get(t, tb.campaign, caios.GetId()); !proto.Equal(choicesCharacterView(read).GetSheet(), wrote) {
		t.Errorf("Caio reads another sheet than the master wrote")
	}

	// A dead character is blocked before its revision is read. Finding: the master is
	// refused too, with the message "only the master edits its sheet".
	tb.master.markDead(t, second)
	tb.logs.take(t)
	_, err = tb.complete(tb.ana, halfElf.GetId(), 1, pick(key, "ability:dex"))
	got = append(got, map[string]any{"case": "a dead character, as its owner", "refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)})
	dead := tb.master.get(t, tb.campaign, halfElf.GetId())
	_, err = tb.complete(tb.master, halfElf.GetId(), dead.GetRevision()+1, pick(key, "ability:dex"))
	got = append(got, map[string]any{"case": "a dead character with a stale revision, as the master", "refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)})
	choicesGolden(t, "choices_complete", got, tb.names...)
}

// TestCompleteCharacterChoicesFrozenRules freezes the checks after the picks are laid: a
// stored sheet the rules no longer take (checkSheet), two picks of one call that break a
// rule together (refuseNewChoiceProblems), and a service with no history (no line).
func TestCompleteCharacterChoicesFrozenRules(t *testing.T) {
	t.Parallel()
	tb := newChoicesFrozenTable(t)
	tb.h.svc.SetCreatureHost(nil)
	const style = "feature:fighter-fighting-style"
	mk := func(u *user, name string, open *charactersv1.CharacterSheet) *charactersv1.Character {
		t.Helper()
		c, err := u.createWith(tb.campaign, unspecifiedMethod, choiceSheet("race:human", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense"))
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if c, err = tb.master.update(t, c, name, open); err != nil {
			t.Fatalf("the master's sheet for %s: %v", name, err)
		}
		return tb.name(c, name)
	}
	gone := mk(tb.ana, "gone-race", choiceSheet("race:human", "", "class:fighter", "", 1))
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,race_key}', '"race:gone"') WHERE id = $1`, gone.GetId()); err != nil {
		t.Fatal(err)
	}
	championSheet := choiceSheet("race:human", "", "class:fighter", "subclass:champion", 10)
	championSheet.GetFull().ExperiencePoints = 64000
	champion := mk(tb.caio, "champion", championSheet)
	const enemy = "feature:favored-enemy-1-type"
	terrain := rules.ScopedChoice("feature:natural-explorer-1-terrain-type", "terrain:forest")
	dora := tb.h.newUser("Dora")
	tb.h.join(tb.master, tb.campaign, dora)
	tb.names = append(tb.names, dora.id, "<dora>")
	ranger, err := dora.createWith(tb.campaign, unspecifiedMethod, choiceSheet("race:human", "", "class:ranger", "", 1,
		rules.ScopedChoice(enemy, "creature-type:beast"), rules.ScopedChoice(enemy+"#language", rules.LanguageNone), terrain))
	if err != nil {
		t.Fatalf("create the ranger: %v", err)
	}
	if ranger, err = tb.master.update(t, ranger, "ranger", choiceSheet("race:human", "", "class:ranger", "", 1, terrain)); err != nil {
		t.Fatalf("the master opens the ranger's enemy: %v", err)
	}
	tb.name(ranger, "ranger")
	eva := tb.h.newUser("Eva")
	tb.h.join(tb.master, tb.campaign, eva)
	tb.names = append(tb.names, eva.id, "<eva>")
	multiSheet := choiceSheet("race:human", "", "class:fighter", "", 1)
	multiSheet.GetFull().Classes = append(multiSheet.GetFull().Classes, &charactersv1.ClassLevel{ClassKey: "class:paladin", Level: 2})
	multiSheet.GetFull().BaseScores.Charisma = 13
	multiSheet.GetFull().ExperiencePoints = 900
	multi := mk(eva, "fighter-paladin", multiSheet)
	tb.logs.take(t)

	var got []any
	_, err = tb.complete(dora, ranger.GetId(), ranger.GetRevision(), pick(enemy, "creature-type:beast"), pick(enemy+"#language", "language:abyssal"))
	// Finding: a beast, which speaks no language, is taken with Abyssal as the language
	// its ranger learns: the language is checked against the stored sheet's offer (no
	// enemy yet), and the sheet after has no problem the rules report.
	got = append(got, map[string]any{"case": "a beast that speaks Abyssal", "refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)})
	// One style picked for each class in one call: the second is taken by the other class
	// on the sheet after, a problem the stored sheet did not have.
	_, err = tb.complete(eva, multi.GetId(), multi.GetRevision(), pick(style, "feature:fighter-fighting-style-defense"), pick("feature:paladin-fighting-style", "feature:fighting-style-defense"))
	got = append(got, map[string]any{"case": "the same style for the fighter and the paladin", "refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)})
	_, err = tb.complete(tb.ana, gone.GetId(), gone.GetRevision(), pick(style, "feature:fighter-fighting-style-archery"))
	got = append(got, map[string]any{"case": "a stored race the rules do not know", "refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)})
	_, err = tb.complete(tb.caio, champion.GetId(), champion.GetRevision(),
		pick(style, "feature:fighter-fighting-style-defense"), pick("feature:additional-fighting-style", "feature:fighter-fighting-style-defense"))
	got = append(got, map[string]any{"case": "the same style in both of the champion's choices", "refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)})
	done, err := tb.complete(tb.caio, champion.GetId(), champion.GetRevision(),
		pick(style, "feature:fighter-fighting-style-defense"), pick("feature:additional-fighting-style", "feature:fighter-fighting-style-dueling"))
	got = append(got, map[string]any{"case": "two styles without a history", "refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t), "live": tb.live.count()})
	if err == nil {
		got = append(got, map[string]any{"case": "the champion after", "picks": done.GetSheet().GetFull().GetFeatureChoiceKeys(), "revision": done.GetRevision()})
	}
	choicesGolden(t, "choices_complete_rules", got, tb.names...)
}

// TestGetCampaignOpenChoicesFrozen freezes the master's list of open choices: who is in it
// (a living player character with a full sheet and something open) and how it reads, what
// every other viewer gets, and the answer for a sheet that cannot be read.
func TestGetCampaignOpenChoicesFrozen(t *testing.T) {
	t.Parallel()
	tb := newChoicesFrozenTable(t)
	key := rules.AbilityChoiceKey("race:half-elf")
	open := func(u *user, name string, complete, open *charactersv1.CharacterSheet) *charactersv1.Character {
		t.Helper()
		c, err := u.createWith(tb.campaign, unspecifiedMethod, complete)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if c, err = tb.master.update(t, c, name, open); err != nil {
			t.Fatalf("the master opens a choice of %s: %v", name, err)
		}
		return tb.name(c, name)
	}
	fighter := func(picks ...string) *charactersv1.CharacterSheet {
		return choiceSheet("race:human", "", "class:fighter", "", 1, picks...)
	}
	// Ana's half-elf has three selections open in two choices; Caio's fighter one.
	open(tb.ana, "Ana half-elf",
		choiceSheet("race:half-elf", "", "class:fighter", "", 1, rules.ScopedChoice(key, "ability:dex"), rules.ScopedChoice(key, "ability:con"), "feature:fighter-fighting-style-defense"),
		choiceSheet("race:half-elf", "", "class:fighter", "", 1))
	caios := open(tb.caio, "Caio fighter", fighter("feature:fighter-fighting-style-defense"), fighter())
	// Not in the list: a complete sheet, an NPC with a choice open, a story NPC, a dead
	// player character with a choice open, a player character with a basic sheet.
	dora, eva := tb.h.newUser("Dora"), tb.h.newUser("Eva")
	tb.h.join(tb.master, tb.campaign, dora, eva)
	tb.names = append(tb.names, dora.id, "<dora>", eva.id, "<eva>")
	tb.master.markDead(t, open(dora, "dead", fighter("feature:fighter-fighting-style-defense"), fighter()))
	complete, err := tb.pending.createWith(tb.campaign, unspecifiedMethod, fighter("feature:fighter-fighting-style-dueling"))
	if err != nil {
		t.Fatalf("the pending member's character: %v", err)
	}
	tb.name(complete, "pending's draft")
	npc := tb.master.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, "Bandido", enemySheet())
	tb.name(npc, "enemy")
	tb.name(tb.master.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_STORY, "Taverneiro", basicSheet()), "story")
	basic, err := eva.createWith(tb.campaign, unspecifiedMethod, fighter("feature:fighter-fighting-style-defense"))
	if err != nil {
		t.Fatalf("Eva's character: %v", err)
	}
	tb.name(basic, "basic player")
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = '{"basic":{"hit_points_max":7,"armor_class":15}}' WHERE id = $1`, basic.GetId()); err != nil {
		t.Fatal(err)
	}
	tb.logs.take(t)

	read := func(u *user, campaign string) map[string]any {
		res, err := u.api.GetCampaignOpenChoices(t.Context(), connect.NewRequest(&charactersv1.GetCampaignOpenChoicesRequest{CampaignId: campaign}))
		if err != nil {
			return map[string]any{"refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)}
		}
		return map[string]any{"answer": choicesProtoJSON(t, res.Msg), "logs": tb.logs.take(t)}
	}
	const absent = "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"
	viewers := []struct {
		name string
		u    *user
	}{{"master", tb.master}, {"owner", tb.ana}, {"another player", tb.caio}, {"pending member", tb.pending}, {"stranger", tb.alien}, {"nobody", tb.h.anonymous()}}
	var got []any
	for _, v := range viewers {
		got = append(got, map[string]any{"viewer": v.name, "this campaign": read(v.u, tb.campaign), "a campaign that does not exist": read(v.u, absent)})
	}
	// A sheet the server cannot read stops the whole list, with the internal error.
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = '{"full":"texto"}' WHERE id = $1`, caios.GetId()); err != nil {
		t.Fatal(err)
	}
	got = append(got, map[string]any{"viewer": "master", "a sheet that cannot be read": read(tb.master, tb.campaign)})
	// CompleteCharacterChoices of that sheet is the same internal error, logged once.
	_, err = tb.complete(tb.master, caios.GetId(), caios.GetRevision(), pick("feature:fighter-fighting-style", "feature:fighter-fighting-style-archery"))
	got = append(got, map[string]any{"viewer": "master", "completing a sheet that cannot be read": map[string]any{"refusal": choicesRefusalShape(t, err), "logs": tb.logs.take(t)}})
	choicesGolden(t, "choices_open_list", got, tb.names...)
}
