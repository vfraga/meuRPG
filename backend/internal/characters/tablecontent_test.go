package characters

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The acceptance criteria of MR-025 (docs/product/stories.md) for the table's
// own content: storage, API and liveness (RN-23, ADR-0018). Each test starts its
// own database.

// The bodies the tests write, one per kind. They follow the fixtures of package
// rules' overlay tests, in the messages' own shape.

func testNote(slug, name, text string) *rulesv1.TableFeature {
	return &rulesv1.TableFeature{NamePt: name, DescPt: []string{"Texto de " + slug + "."}, Effects: []*rulesv1.TableEffect{{Type: "note", TextPt: text}}}
}

func testBackground(name string) *rulesv1.TableBackground {
	return &rulesv1.TableBackground{
		NamePt: name, Skills: []string{"skill:insight", "skill:religion"}, Tools: []string{"proficiency:thieves-tools"},
		LanguageChoices: 1, EquipmentPt: "Uma lanterna e um apito.",
		Feature: testNote("luz", "Luz-guia", "Conhece a rota dos navios."),
	}
}

func testRace(name string) *rulesv1.TableRace {
	return &rulesv1.TableRace{
		NamePt: name, Size: "Medium", SpeedFt: 25, AbilityBonuses: &rulesv1.AbilityScores{Constitution: 1}, ChoiceBonuses: []int32{2, 1},
		DarkvisionFt: 60, Languages: []string{"language:common"}, LanguageChoices: 1,
		Traits: []*rulesv1.TableFeature{{NamePt: "Resistente", Effects: []*rulesv1.TableEffect{{
			Type: "roll_mode", Roll: "advantage", Targets: []string{"save.con"}, Tags: []string{"against:poison"},
		}}}},
	}
}

func testSubrace(name, race string) *rulesv1.TableSubrace {
	return &rulesv1.TableSubrace{
		NamePt: name, RaceKey: race, AbilityBonuses: &rulesv1.AbilityScores{Wisdom: 1},
		Traits: []*rulesv1.TableFeature{{NamePt: "Olhar firme", Effects: []*rulesv1.TableEffect{{Type: "proficiency", Proficiency: "skill:perception"}}}},
	}
}

func testSpell(name string, classes ...string) *rulesv1.TableSpell {
	return &rulesv1.TableSpell{
		NamePt: name, Level: 1, SchoolKey: "school:evocation",
		CastingTime: &rulesv1.TableSpellCastingTime{Unit: rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_ACTION, Amount: 1},
		Range:       &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_RANGED, DistanceFt: 60},
		Duration:    &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS},
		Components:  &rulesv1.TableSpellComponents{Verbal: true, Somatic: true},
		ClassKeys:   classes, DescPt: []string{"Um raio de teste."},
		Target: &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_CREATURE},
		Attack: "ranged",
		Damage: []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:force", Dice: "3d6", PerSlotLevel: "1d6"}},
	}
}

func testClass(name string) *rulesv1.TableClass {
	c := &rulesv1.TableClass{
		NamePt: name, HitDie: 8,
		SavingThrows: []rulesv1.Ability{rulesv1.Ability_ABILITY_CONSTITUTION, rulesv1.Ability_ABILITY_WISDOM},
		SkillChoose:  2, SkillFrom: []string{"skill:arcana", "skill:history", "skill:medicine", "skill:nature", "skill:survival"},
		Proficiencies: []string{"proficiency:light-armor", "proficiency:simple-weapons"},
		Minimums:      &rulesv1.AbilityScores{Constitution: 11},
		SubclassLevel: 3,
	}
	for lvl := 1; lvl <= 20; lvl++ {
		row := &rulesv1.TableClassLevel{}
		switch lvl {
		case 1:
			row.Features = []*rulesv1.TableFeature{
				{NamePt: "Vigor", DescPt: []string{"Mais iniciativa."}, Effects: []*rulesv1.TableEffect{{Type: "modifier", Target: "initiative", Mode: "add", Value: "1"}}},
				{NamePt: "Treino", Effects: []*rulesv1.TableEffect{{Type: "proficiency", Proficiency: "skill:survival"}}},
			}
		case 2:
			row.Features = []*rulesv1.TableFeature{{NamePt: "Surto", Effects: []*rulesv1.TableEffect{
				{Type: "resource", Resource: "surto_teste", Max: "prof()", Recharge: "short_rest"},
			}}}
		}
		c.Levels = append(c.Levels, row)
	}
	return c
}

func testSubclass(name, class string) *rulesv1.TableSubclass {
	return &rulesv1.TableSubclass{
		NamePt: name, ClassKey: class, DescPt: []string{"Um caminho de teste."},
		Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{testNote("g", "Golpe do caminho", "Um golpe especial.")}}},
	}
}

// createReq and updateReq wrap a body in the request's oneof.
func createReq(campaign string, body proto.Message) *rulesv1.CreateTableEntryRequest {
	r := &rulesv1.CreateTableEntryRequest{CampaignId: campaign}
	switch b := body.(type) {
	case *rulesv1.TableClass:
		r.Body = &rulesv1.CreateTableEntryRequest_TableClass{TableClass: b}
	case *rulesv1.TableSubclass:
		r.Body = &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: b}
	case *rulesv1.TableRace:
		r.Body = &rulesv1.CreateTableEntryRequest_TableRace{TableRace: b}
	case *rulesv1.TableSubrace:
		r.Body = &rulesv1.CreateTableEntryRequest_TableSubrace{TableSubrace: b}
	case *rulesv1.TableBackground:
		r.Body = &rulesv1.CreateTableEntryRequest_TableBackground{TableBackground: b}
	case *rulesv1.TableSpell:
		r.Body = &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: b}
	}
	return r
}

func updateReq(campaign, key string, revision int32, body proto.Message) *rulesv1.UpdateTableEntryRequest {
	r := &rulesv1.UpdateTableEntryRequest{CampaignId: campaign, Key: key, ExpectedRevision: revision}
	switch b := body.(type) {
	case *rulesv1.TableClass:
		r.Body = &rulesv1.UpdateTableEntryRequest_TableClass{TableClass: b}
	case *rulesv1.TableSubclass:
		r.Body = &rulesv1.UpdateTableEntryRequest_TableSubclass{TableSubclass: b}
	case *rulesv1.TableRace:
		r.Body = &rulesv1.UpdateTableEntryRequest_TableRace{TableRace: b}
	case *rulesv1.TableSubrace:
		r.Body = &rulesv1.UpdateTableEntryRequest_TableSubrace{TableSubrace: b}
	case *rulesv1.TableBackground:
		r.Body = &rulesv1.UpdateTableEntryRequest_TableBackground{TableBackground: b}
	case *rulesv1.TableSpell:
		r.Body = &rulesv1.UpdateTableEntryRequest_TableSpell{TableSpell: b}
	}
	return r
}

// bodyMessage is the body of an entry, whatever its kind.
func bodyMessage(e *rulesv1.TableEntry) proto.Message {
	switch b := e.GetBody().(type) {
	case *rulesv1.TableEntry_TableClass:
		return b.TableClass
	case *rulesv1.TableEntry_TableSubclass:
		return b.TableSubclass
	case *rulesv1.TableEntry_TableRace:
		return b.TableRace
	case *rulesv1.TableEntry_TableSubrace:
		return b.TableSubrace
	case *rulesv1.TableEntry_TableBackground:
		return b.TableBackground
	case *rulesv1.TableEntry_TableSpell:
		return b.TableSpell
	}
	return nil
}

// addEntry creates an entry as u, or fails the test.
func (u *user) addEntry(t *testing.T, campaign string, body proto.Message) *rulesv1.TableEntry {
	t.Helper()
	res, err := u.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, body)))
	if err != nil {
		t.Fatalf("CreateTableEntry(%T) error = %v", body, err)
	}
	return res.Msg.GetEntry()
}

// entries lists the table's entries as u, by key.
func (u *user) entries(t *testing.T, campaign string) map[string]*rulesv1.TableEntry {
	t.Helper()
	res, err := u.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ListTableEntries() error = %v", err)
	}
	out := map[string]*rulesv1.TableEntry{}
	for _, e := range res.Msg.GetEntries() {
		out[e.GetKey()] = e
	}
	return out
}

// tableRevisionOf is the entry's revision as the master reads it, for a write
// that must not fail as stale.
func tableRevisionOf(t *testing.T, master *user, campaign, key string) int32 {
	t.Helper()
	e := master.entries(t, campaign)[key]
	if e == nil {
		return 1
	}
	return e.GetRevision()
}

// violationsOfErr reads the refusal detail of an invalid_argument.
func violationsOfErr(t *testing.T, err error) []*rulesv1.TableContentViolation {
	t.Helper()
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Code() != connect.CodeInvalidArgument {
		t.Fatalf("error = %v, want invalid_argument", err)
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if r, ok := v.(*rulesv1.TableContentRefusal); ok {
				return r.GetViolations()
			}
		}
	}
	t.Fatalf("no TableContentRefusal detail in %v", err)
	return nil
}

// blockedOf reads the TableContentBlocked detail.
func blockedOf(t *testing.T, err error) *rulesv1.TableContentBlocked {
	t.Helper()
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		t.Fatalf("error = %v, want a connect error", err)
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*rulesv1.TableContentBlocked); ok {
				return b
			}
		}
	}
	t.Fatalf("no TableContentBlocked detail in %v", err)
	return nil
}

// charBlocked reads the CharacterBlocked detail.
func charBlocked(t *testing.T, err error) *charactersv1.CharacterBlocked {
	t.Helper()
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Code() != connect.CodeFailedPrecondition {
		t.Fatalf("error = %v, want failed_precondition", err)
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if b, ok := v.(*charactersv1.CharacterBlocked); ok {
				return b
			}
		}
	}
	t.Fatalf("no CharacterBlocked detail in %v", err)
	return nil
}

// TestTableContentEveryKindIsCreatedUpdatedArchivedAndBroughtBack walks the four
// writes for each of the six kinds, and checks that the key and the feature keys
// stay through a rename.
func TestTableContentEveryKindIsCreatedUpdatedArchivedAndBroughtBack(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)

	// The kinds that need a parent come after it.
	class := master.addEntry(t, campaign, testClass("Guardião do Vale"))
	race := master.addEntry(t, campaign, testRace("Anão das Brumas"))
	tests := []struct {
		kind    string
		body    proto.Message
		renamed proto.Message
		prefix  string
	}{
		{"class", testClass("Guardião do Vale 2"), testClass("Guardião Renomeado"), "class:guardiao-do-vale-2@mesa"},
		{"subclass", testSubclass("Caminho do Vento", "class:fighter"), testSubclass("Caminho Renomeado", "class:fighter"), "subclass:caminho-do-vento@mesa"},
		{"race", testRace("Elfo Pálido"), testRace("Elfo Renomeado"), "race:elfo-palido@mesa"},
		{"subrace", testSubrace("Da Colina", race.GetKey()), testSubrace("Da Colina Renomeada", race.GetKey()), "subrace:da-colina@mesa"},
		{"background", testBackground("Guarda de farol"), testBackground("Guarda Renomeado"), "background:guarda-de-farol@mesa"},
		{"spell", testSpell("Raio de teste", "class:wizard"), testSpell("Raio Renomeado", "class:wizard"), "spell:raio-de-teste@mesa"},
	}
	if class.GetKey() != "class:guardiao-do-vale@mesa" {
		t.Fatalf("the class key = %q, want one made from the name", class.GetKey())
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			created := master.addEntry(t, campaign, tc.body)
			if created.GetKey() != tc.prefix || created.GetNamePt() == "" || created.GetArchived() || created.GetRevision() < 1 {
				t.Fatalf("created = %v, want key %q, a name, not archived, a revision", created, tc.prefix)
			}
			if !proto.Equal(stripKeys(bodyMessage(created)), stripKeys(tc.body)) {
				t.Errorf("the stored body differs from what was sent (but the feature keys)")
			}
			keysBefore := featureKeysOf(bodyMessage(created))
			update, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, created.GetKey(), created.GetRevision(), cloneWithKeys(tc.renamed, bodyMessage(created)))))
			if err != nil {
				t.Fatalf("UpdateTableEntry() error = %v", err)
			}
			updated := update.Msg.GetEntry()
			if updated.GetKey() != created.GetKey() {
				t.Errorf("the key changed from %q to %q with the name", created.GetKey(), updated.GetKey())
			}
			if !strings.Contains(updated.GetNamePt(), "Renomead") || updated.GetRevision() <= created.GetRevision() {
				t.Errorf("updated = %q at revision %d, want the new name and a newer revision", updated.GetNamePt(), updated.GetRevision())
			}
			if got := featureKeysOf(bodyMessage(updated)); !slices.Equal(got, keysBefore) {
				t.Errorf("feature keys after the update = %v, want the same %v", got, keysBefore)
			}
			archived, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: created.GetKey()}))
			if err != nil || !archived.Msg.GetEntry().GetArchived() || !archived.Msg.GetEntry().GetArchivedAt().IsValid() {
				t.Fatalf("ArchiveTableEntry() = %v, %v; want an archived entry with its date", archived, err)
			}
			if archived.Msg.GetEntry().GetRevision() != updated.GetRevision() || !archived.Msg.GetEntry().GetUpdatedAt().AsTime().Equal(updated.GetUpdatedAt().AsTime()) ||
				archived.Msg.GetTableRevision() <= updated.GetRevision() {
				t.Errorf("an archive moved the entry's revision or date (%d, %v), or not the campaign's (%d): it is not a change of the entry",
					archived.Msg.GetEntry().GetRevision(), archived.Msg.GetEntry().GetUpdatedAt(), archived.Msg.GetTableRevision())
			}
			if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: created.GetKey()})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
				t.Errorf("archiving twice: error = %v, want failed_precondition", err)
			}
			// An archived entry can still be edited, and stays archived.
			edited, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, created.GetKey(), archived.Msg.GetEntry().GetRevision(), cloneWithKeys(tc.renamed, bodyMessage(updated)))))
			if err != nil || !edited.Msg.GetEntry().GetArchived() || !edited.Msg.GetEntry().GetArchivedAt().IsValid() {
				t.Errorf("updating an archived entry: %v, %v; want it saved, still archived", edited, err)
			}
			back, err := master.table.UnarchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: campaign, Key: created.GetKey()}))
			if err != nil || back.Msg.GetEntry().GetArchived() {
				t.Fatalf("UnarchiveTableEntry() = %v, %v; want the entry back", back, err)
			}
			if _, err := master.table.UnarchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: campaign, Key: created.GetKey()})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
				t.Errorf("unarchiving twice: error = %v, want failed_precondition", err)
			}
		})
	}
}

// stripKeys is a body without the feature keys the server made, to compare with
// what the editor sent.
func stripKeys(m proto.Message) proto.Message {
	c := proto.Clone(m)
	walkFeatures(c, func(f *rulesv1.TableFeature) { f.Key = "" })
	return c
}

// cloneWithKeys is next with the feature keys of the stored body, as an editor
// sends them back (by position).
func cloneWithKeys(next, stored proto.Message) proto.Message {
	c := proto.Clone(next)
	var keys []string
	walkFeatures(stored, func(f *rulesv1.TableFeature) { keys = append(keys, f.GetKey()) })
	i := 0
	walkFeatures(c, func(f *rulesv1.TableFeature) {
		if i < len(keys) {
			f.Key = keys[i]
		}
		i++
	})
	return c
}

func featureKeysOf(m proto.Message) []string {
	var keys []string
	walkFeatures(m, func(f *rulesv1.TableFeature) { keys = append(keys, f.GetKey()) })
	return keys
}

func walkFeatures(m proto.Message, fn func(*rulesv1.TableFeature)) {
	each := func(fs []*rulesv1.TableFeature) {
		for _, f := range fs {
			fn(f)
		}
	}
	switch b := m.(type) {
	case *rulesv1.TableClass:
		for _, l := range b.GetLevels() {
			each(l.GetFeatures())
		}
	case *rulesv1.TableSubclass:
		for _, l := range b.GetLevels() {
			each(l.GetFeatures())
		}
	case *rulesv1.TableRace:
		each(b.GetTraits())
	case *rulesv1.TableSubrace:
		each(b.GetTraits())
	case *rulesv1.TableBackground:
		if b.GetFeature() != nil {
			fn(b.GetFeature())
		}
	}
}

// TestTableContentFeatureKeysAreStable: a feature keeps its key while it exists,
// through a rename and a move to another level of the editor's list; a new one
// gets its own, and a key the entry never had is refused.
func TestTableContentFeatureKeysAreStable(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	created := master.addEntry(t, campaign, testClass("Guardião"))
	first := featureKeysOf(bodyMessage(created))
	if len(first) != 3 || !strings.HasPrefix(first[0], "feature:class-guardiao-") || !strings.HasSuffix(first[0], "--vigor@mesa") || !strings.HasSuffix(first[2], "--surto@mesa") {
		t.Fatalf("feature keys = %v, want ones made from the class and the feature names", first)
	}
	stem := strings.TrimSuffix(first[0], "vigor@mesa")
	// Rename "Vigor" and send it back with its key; add a new "Vigor" and leave it keyless.
	next := proto.Clone(bodyMessage(created)).(*rulesv1.TableClass)
	next.Levels[0].Features[0].NamePt = "Vigor de Ferro"
	next.Levels[0].Features = append(next.Levels[0].Features, &rulesv1.TableFeature{NamePt: "Vigor"})
	res, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, created.GetKey(), created.GetRevision(), next)))
	if err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	after := featureKeysOf(bodyMessage(res.Msg.GetEntry()))
	if after[0] != first[0] || after[1] != first[1] || after[3] != first[2] {
		t.Errorf("keys after the rename = %v, want the first ones kept (%v)", after, first)
	}
	if after[2] != stem+"vigor@mesa" && after[2] != stem+"vigor-2@mesa" {
		t.Errorf("the new feature's key = %q", after[2])
	}
	if after[2] == after[0] {
		t.Errorf("the new feature took the key of an existing one: %q", after[2])
	}
	// An editor that did not send the keys back: the same level and name take them.
	again := proto.Clone(bodyMessage(res.Msg.GetEntry())).(*rulesv1.TableClass)
	walkFeatures(again, func(f *rulesv1.TableFeature) { f.Key = "" })
	res2, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, created.GetKey(), res.Msg.GetEntry().GetRevision(), again)))
	if err != nil {
		t.Fatalf("UpdateTableEntry() without the keys error = %v", err)
	}
	if got := featureKeysOf(bodyMessage(res2.Msg.GetEntry())); !slices.Equal(got, after) {
		t.Errorf("keys after an update with none = %v, want %v", got, after)
	}
	// A key the entry never had.
	bad := proto.Clone(bodyMessage(res2.Msg.GetEntry())).(*rulesv1.TableClass)
	bad.Levels[0].Features[0].Key = "feature:class-outra--vigor@mesa"
	_, err = master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, created.GetKey(), res2.Msg.GetEntry().GetRevision(), bad)))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetReason() != "immutable" || v[0].GetField() != "table_class.levels[0].features[0].key" {
		t.Errorf("violations = %v, want one immutable at the feature's key", v)
	}
}

// TestTableContentKeysNeverChangeAndNamesAreUnique: the key comes from the name
// once; two entries with the same name in one kind are refused, and a name
// differing only in accents makes a key with a number.
func TestTableContentKeysNeverChangeAndNamesAreUnique(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	a := master.addEntry(t, campaign, testBackground("Guarda de Farol"))
	_, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, testBackground("guarda de farol"))))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetReason() != reasonDuplicateName || v[0].GetField() != "table_background.name_pt" {
		t.Errorf("a repeated name: violations = %v, want duplicate_name at the name", v)
	}
	b := master.addEntry(t, campaign, testBackground("Guarda de Fárol"))
	if a.GetKey() == b.GetKey() || b.GetKey() != "background:guarda-de-farol-2@mesa" {
		t.Errorf("keys = %q and %q, want a number to tell them apart", a.GetKey(), b.GetKey())
	}
	// The kind and the parent are fixed.
	_, err = master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, a.GetKey(), a.GetRevision(), testRace("Outra"))))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetReason() != "immutable" || v[0].GetField() != "body" {
		t.Errorf("another kind: violations = %v, want immutable at the body", v)
	}
	sc := master.addEntry(t, campaign, testSubclass("Caminho", "class:fighter"))
	_, err = master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, sc.GetKey(), sc.GetRevision(), testSubclass("Caminho", "class:rogue"))))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetReason() != "immutable" || v[0].GetField() != "table_subclass.class_key" {
		t.Errorf("another parent: violations = %v, want immutable at class_key", v)
	}
	// Not a table key, and a table key that is not there.
	for _, key := range []string{"class:wizard", "spell:nao-existe@mesa"} {
		if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: key})); connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("Archive(%q) error = %v, want not_found", key, err)
		}
	}
}

// TestTableContentRefusalsAreFieldViolations: what the rules engine refuses comes
// back as a violation per error, at the field of the body, and nothing changes.
func TestTableContentRefusalsAreFieldViolations(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	revisionBefore := h.contentRevision(campaign)

	badSpell := testSpell("Feitiço ruim", "class:wizard")
	badSpell.Target = &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_AREA, Shape: rulesv1.TableAreaShape_TABLE_AREA_SHAPE_CONE, SizeFt: 7}
	tests := []struct {
		name   string
		body   proto.Message
		field  string
		reason string
	}{
		{"hit die", func() proto.Message { c := testClass("Dado"); c.HitDie = 7; return c }(), "table_class.hit_die", rules.ReasonValue},
		{"a handler", func() proto.Message {
			c := testClass("Handler")
			c.Levels[0].Features[0].Effects[0] = &rulesv1.TableEffect{Type: "handler", Target: "x"}
			return c
		}(), "table_class.levels[0].features[0].effects[0].type", rules.ReasonEffect},
		{"a formula", func() proto.Message {
			c := testClass("Formula")
			c.Levels[1].Features[0].Effects[0].Max = "prof("
			return c
		}(), "table_class.levels[1].features[0].effects[0]", rules.ReasonFormula},
		{"slots", func() proto.Message { c := testClass("Espacos"); c.Levels[2].Slots = []int32{1, 2}; return c }(), "table_class.levels[2].slots", rules.ReasonTable},
		{"an area", badSpell, "table_spell.target.size_ft", rules.ReasonLimit},
		{"a missing class", testSubclass("Sem mae", "class:nao-existe@mesa"), "table_subclass", ""},
		{"a name", testBackground(" "), "table_background.name_pt", rules.ReasonName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, tc.body)))
			v := violationsOfErr(t, err)
			if len(v) == 0 || !strings.HasPrefix(v[0].GetField(), tc.field) || (tc.reason != "" && v[0].GetReason() != tc.reason) {
				t.Errorf("violations = %v, want %q (%s)", v, tc.field, tc.reason)
			}
		})
	}
	if got := h.contentRevision(campaign); got != revisionBefore {
		t.Errorf("revision after refusals = %d, want %d: a refused write changes nothing", got, revisionBefore)
	}
	if n := len(master.entries(t, campaign)); n != 0 {
		t.Errorf("%d entries after refusals, want none", n)
	}
}

// contentRevision reads the campaign's content revision straight from the table.
func (h *harness) contentRevision(campaign string) int32 {
	h.t.Helper()
	var rev int32
	err := h.pool.QueryRow(h.t.Context(), "SELECT COALESCE((SELECT revision FROM campaign_content_state WHERE campaign_id = $1), 0)", campaign).Scan(&rev)
	if err != nil {
		h.t.Fatalf("read the content revision: %v", err)
	}
	return rev
}

// TestTableContentLimits: 64 KiB of data per entry, 300 entries per campaign.
func TestTableContentLimits(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")

	big := testBackground("Gigante")
	big.Feature.DescPt = nil
	for range 40 {
		big.Feature.DescPt = append(big.Feature.DescPt, strings.Repeat("a", 1900)) // under the 50 paragraphs of the engine, over 64 KiB
	}
	_, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, big)))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetReason() != "size_limit" || v[0].GetField() != "table_background" {
		t.Errorf("an entry over 64 KiB: violations = %v, want size_limit", v)
	}

	// 300 entries, written straight into the table: each through the API would take minutes.
	ctx := t.Context()
	for i := range 300 {
		name := "Antecedente " + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26))
		data, err := storedData(testBackground(name))
		if err != nil {
			t.Fatal(err)
		}
		slug := slugify(name) + "-" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		if _, err := h.pool.Exec(ctx, `INSERT INTO campaign_content (campaign_id, content_key, kind, name_pt, data, revision, created_at, updated_at)
			VALUES ($1, $2, 'background', $3, $4, 1, now(), now())`, campaign, "background:"+slug+"@mesa", name+" "+slug, data); err != nil {
			t.Fatalf("insert entry %d: %v", i, err)
		}
	}
	if _, err := h.pool.Exec(ctx, "INSERT INTO campaign_content_state (campaign_id, revision, updated_at) VALUES ($1, 1, now())", campaign); err != nil {
		t.Fatalf("insert the revision: %v", err)
	}
	_, err = master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, testBackground("A trecentésima primeira"))))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetReason() != rules.ReasonLimit {
		t.Errorf("the 301st entry: violations = %v, want limit", v)
	}
	if got := h.contentRevision(campaign); got != 1 {
		t.Errorf("revision = %d after a refused 301st entry, want 1", got)
	}
}

// TestTableContentAccess: the master writes; players and the master read; a
// player gets the playable entries in full and never an archived one, nor a
// count; a pending member and an outsider get not_found.
func TestTableContentAccess(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, pending, outsider := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Pendente"), h.newUser("De fora")
	campaign := h.newCampaign(master, "Mirathel", player)
	h.joinPending(master, campaign, pending)
	live := master.addEntry(t, campaign, testClass("Guardião do Vale"))
	draft := master.addEntry(t, campaign, testRace("Rascunho"))
	if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: draft.GetKey()})); err != nil {
		t.Fatal(err)
	}

	m, p := master.entries(t, campaign), player.entries(t, campaign)
	if len(m) != 2 || !m[draft.GetKey()].GetArchived() {
		t.Errorf("the master's entries = %d, want both, the draft archived", len(m))
	}
	if len(p) != 1 || p[live.GetKey()] == nil || p[draft.GetKey()] != nil {
		t.Errorf("the player's entries = %v, want only the one that is not archived", p)
	}
	if got := p[live.GetKey()].GetTableClass(); got.GetHitDie() != 8 || len(got.GetLevels()) != 20 || len(got.GetLevels()[0].GetFeatures()[0].GetEffects()) != 1 {
		t.Errorf("the player reads %v, want the class in full: the hit die, 20 levels and the effects", got)
	}
	if p[live.GetKey()].GetCharactersUsing() != 0 {
		t.Errorf("the player's count = %d, want none", p[live.GetKey()].GetCharactersUsing())
	}
	// The catalog: the player's has no archived race, the master's has it marked.
	pc, err := player.content.ListContent(t.Context(), connect.NewRequest(&rulesv1.ListContentRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatal(err)
	}
	mc, err := master.content.ListContent(t.Context(), connect.NewRequest(&rulesv1.ListContentRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatal(err)
	}
	if !hasRace(pc.Msg.GetContent(), "race:gnome") || hasRace(pc.Msg.GetContent(), draft.GetKey()) || !hasClass(pc.Msg.GetContent(), live.GetKey()) {
		t.Errorf("the player's catalog has the archived race or lacks the class")
	}
	if r := findRace(mc.Msg.GetContent(), draft.GetKey()); r == nil || !r.GetArchived() || len(r.GetChoiceBonuses()) != 2 {
		t.Errorf("the master's catalog race = %v, want the archived draft with its choice bonuses", r)
	}
	if pc.Msg.GetTableRevision() != 3 || !strings.HasSuffix(pc.Msg.GetContent().GetContentVersion(), "+mesa.3") {
		t.Errorf("table revision %d, version %q, want 3 and +mesa.3", pc.Msg.GetTableRevision(), pc.Msg.GetContent().GetContentVersion())
	}
	// The pending member reads the catalog, not the entries; the outsider nothing.
	if _, err := pending.content.ListContent(t.Context(), connect.NewRequest(&rulesv1.ListContentRequest{CampaignId: campaign})); err != nil {
		t.Errorf("a pending member's ListContent error = %v, want allowed", err)
	}
	for name, u := range map[string]*user{"pending": pending, "outsider": outsider} {
		_, err := u.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: campaign}))
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("%s: ListTableEntries error = %v, want not_found", name, err)
		}
		_, err = u.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, testBackground("Intruso"))))
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("%s: CreateTableEntry error = %v, want not_found", name, err)
		}
	}
	// Another campaign has none of it.
	other := h.newCampaign(master, "Outra")
	if n := len(master.entries(t, other)); n != 0 {
		t.Errorf("another campaign has %d entries, want none", n)
	}
}

func findRace(c *rulesv1.Content, key string) *rulesv1.Race {
	for _, r := range c.GetRaces() {
		if r.GetKey() == key {
			return r
		}
	}
	return nil
}
func hasRace(c *rulesv1.Content, key string) bool { return findRace(c, key) != nil }
func hasClass(c *rulesv1.Content, key string) bool {
	return slices.ContainsFunc(c.GetClasses(), func(cl *rulesv1.CharacterClass) bool { return cl.GetKey() == key })
}

// TestTableContentStaleAndRevision: a write bumps the campaign's content revision
// once; an editor with an old revision is told, as aborted with a STALE detail.
func TestTableContentStaleAndRevision(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	if got := h.contentRevision(campaign); got != 0 {
		t.Fatalf("revision of a campaign with no content = %d, want 0", got)
	}
	a := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	if a.GetRevision() != 1 || h.contentRevision(campaign) != 1 {
		t.Errorf("after one write: entry revision %d, campaign %d, want 1 and 1", a.GetRevision(), h.contentRevision(campaign))
	}
	b := master.addEntry(t, campaign, testBackground("Outro"))
	if b.GetRevision() != 2 {
		t.Errorf("the second entry's revision = %d, want 2", b.GetRevision())
	}
	// Another master's edit of a: the first editor's revision 1 is stale now.
	if _, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, a.GetKey(), 1, testBackground("Guarda de farol")))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	_, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, a.GetKey(), 1, testBackground("Guarda de farol"))))
	if connect.CodeOf(err) != connect.CodeAborted || blockedOf(t, err).GetReason() != rulesv1.TableContentBlockedReason_TABLE_CONTENT_BLOCKED_REASON_STALE {
		t.Errorf("a stale update: error = %v, want aborted STALE", err)
	}
	if got := h.contentRevision(campaign); got != 3 {
		t.Errorf("revision = %d, want 3: the stale write changed nothing", got)
	}
}

// TestTableContentIsLive: a write makes the next read see it, in a request and
// inside a transaction (the one-connection test pool fails a read through the
// pool); the content is cached by revision, at most 8 of them.
func TestTableContentIsLive(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	src := h.svc.content.(*TableSource)
	srd := loadRules(t)

	c0, _, err := src.For(t.Context(), nil, campaign)
	if err != nil || c0 != srd {
		t.Fatalf("For() with no content = %p, %v; want the SRD content itself", c0, err)
	}
	e := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	read := func(tx pgx.Tx) *rules.Content {
		c, _, err := src.For(t.Context(), tx, campaign)
		if err != nil {
			t.Fatalf("For() error = %v", err)
		}
		return c
	}
	c1 := read(nil)
	if c1.NamePT(e.GetKey()) != "Guarda de farol" || !strings.HasSuffix(c1.Version(), "+mesa.1") || c1.TableRevision() != 1 {
		t.Errorf("after a write: name %q, version %q, want the entry and +mesa.1", c1.NamePT(e.GetKey()), c1.Version())
	}
	if again := read(nil); again != c1 {
		t.Errorf("a second read built the content again: the cache holds one per (campaign, revision)")
	}
	// A write, then a read in a transaction: the new revision, with no pool read.
	e2 := master.addEntry(t, campaign, testBackground("Outro"))
	err = db.InTx(t.Context(), h.pool, func(tx pgx.Tx) error {
		c := read(tx)
		if c.NamePT(e2.GetKey()) != "Outro" || c.TableRevision() != 2 {
			t.Errorf("in a transaction: name %q, revision %d, want the new entry and 2", c.NamePT(e2.GetKey()), c.TableRevision())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A refused write is not seen: the revision did not move.
	if _, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, testBackground(" ")))); err == nil {
		t.Fatal("a nameless entry was accepted")
	}
	if c := read(nil); c.TableRevision() != 2 {
		t.Errorf("revision after a refused write = %d, want 2", c.TableRevision())
	}
	// More revisions than the cache holds: it never holds more than 8.
	for i := range 10 {
		master.addEntry(t, campaign, testBackground("Mais "+strings.Repeat("a", i+1)))
		read(nil)
	}
	if n := src.size(); n > maxLiveContents {
		t.Errorf("the source holds %d contents, want at most %d", n, maxLiveContents)
	}
	// The campaign's own: another campaign still has the SRD.
	other := h.newCampaign(master, "Outra")
	if c, _, err := src.For(t.Context(), nil, other); err != nil || c != srd {
		t.Errorf("another campaign's content = %p, %v; want the SRD content", c, err)
	}
	if c, _, err := src.For(t.Context(), nil, "not-a-uuid"); err != nil || c != srd {
		t.Errorf("For(not a UUID) = %p, %v; want the SRD content", c, err)
	}
}

// TestTableContentConcurrentWrites: writes of one campaign at once all land, one
// after the other: the revision ends at the number of writes, and no two entries
// share one.
func TestTableContentConcurrentWrites(t *testing.T) {
	t.Parallel()
	const n = 8
	dbtest.PoolSize(t, n) // before the harness: its pool is the one that must hold n connections
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			_, errs[i] = master.table.CreateTableEntry(context.Background(), connect.NewRequest(createReq(campaign, testBackground("Fundo "+strings.Repeat("z", i+1)))))
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("write %d error = %v", i, err)
		}
	}
	if got := h.contentRevision(campaign); got != n {
		t.Errorf("revision = %d, want %d, one per write", got, n)
	}
	seen := map[int32]bool{}
	for _, e := range master.entries(t, campaign) {
		if seen[e.GetRevision()] {
			t.Errorf("two entries have revision %d", e.GetRevision())
		}
		seen[e.GetRevision()] = true
	}
	if len(seen) != n {
		t.Errorf("%d entries, want %d", len(seen), n)
	}
}

// pensantusWith is Pensantus's sheet with a table spell added to his book.
func pensantusWith(spell string) *charactersv1.CharacterSheet {
	s := pensantusSheet()
	s.GetFull().KnownSpellKeys = append(s.GetFull().KnownSpellKeys, spell)
	return s
}

// TestTableContentChangeShowsOnTheSheet (question 80): a change applies at once,
// also on a locked sheet; the owner sees "A classe mudou" (an issue and a
// ChangedContent) until the sheet is saved again; the master is told which sheets
// ended with issues; nobody else's sheet is touched.
func TestTableContentChangeShowsOnTheSheet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	spell := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard"))

	pc := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", pensantusWith(spell.GetKey()))
	bystander := other.createPensantus(t, campaign)
	if len(pc.GetDerived().GetChangedContent()) != 0 {
		t.Fatalf("a fresh sheet has changed content: %v", pc.GetDerived().GetChangedContent())
	}
	if h.lockSheets(campaign) != 2 {
		t.Fatal("expected two sheets to lock")
	}
	// Dona's spell is a book spell of the master's table; the master moves it off the wizard list.
	moved := proto.Clone(bodyMessage(spell)).(*rulesv1.TableSpell)
	moved.ClassKeys = []string{"class:cleric"}
	res, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, spell.GetKey(), spell.GetRevision(), moved)))
	if err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	if a := res.Msg.GetAffectedCharacters(); len(a) != 1 || a[0].GetCharacterId() != pc.GetId() || a[0].GetPlayerDisplayName() != "Dona" || a[0].GetIssues() < 1 {
		t.Errorf("affected = %v, want Pensantus of Dona with issues, and not the other sheet", a)
	}
	got := owner.get(t, campaign, pc.GetId())
	if !got.GetSheetLockedAt().IsValid() {
		t.Errorf("the sheet was supposed to stay locked")
	}
	ch := got.GetDerived().GetChangedContent()
	if len(ch) != 1 || ch[0].GetKey() != spell.GetKey() || ch[0].GetNamePt() != "Raio de teste" || ch[0].GetRevision() != res.Msg.GetTableRevision() || ch[0].GetSavedRevision() != 1 ||
		ch[0].GetField() != "full.known_spell_keys[10]" {
		t.Errorf("changed content = %v, want the spell, at its field, saved at revision 1", ch)
	}
	var banner *rulesv1.Issue
	for _, is := range got.GetDerived().GetIssues() {
		if is.GetCode() == IssueTableContentChanged {
			banner = is
		}
	}
	if banner == nil || !strings.HasPrefix(banner.GetMessage(), "A magia Raio de teste mudou") || banner.GetField() != ch[0].GetField() {
		t.Errorf("banner = %v, want the issue 'A magia ... mudou' at the field", banner)
	}
	if len(ch[0].GetMessages()) == 0 || !ch[0].GetChangedAt().IsValid() {
		t.Errorf("changed content = %v, want the sentences of what no longer fits and the date of the change", ch[0])
	}
	for _, msg := range ch[0].GetMessages() {
		if !slices.ContainsFunc(got.GetDerived().GetIssues(), func(is *rulesv1.Issue) bool { return is.GetMessage() == msg }) {
			t.Errorf("message %q is not one of the sheet's issues", msg)
		}
	}
	// It goes away by itself when nothing tied to it remains: the master puts the
	// spell back on the wizard list, and the sheet, read again, shows nothing.
	back, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, spell.GetKey(), res.Msg.GetEntry().GetRevision(), bodyMessage(spell))))
	if err != nil {
		t.Fatalf("UpdateTableEntry() back error = %v", err)
	}
	if n := len(owner.get(t, campaign, pc.GetId()).GetDerived().GetChangedContent()); n != 0 {
		t.Errorf("after the entry fits again: %d changed entries, want none (no save needed)", n)
	}
	if _, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, spell.GetKey(), back.Msg.GetEntry().GetRevision(), moved))); err != nil {
		t.Fatalf("UpdateTableEntry() again error = %v", err)
	}
	got = owner.get(t, campaign, pc.GetId())
	if n := len(other.get(t, campaign, bystander.GetId()).GetDerived().GetChangedContent()); n != 0 {
		t.Errorf("another player's sheet has %d changed entries, want none", n)
	}
	// The master reads the same notice on the sheet; the sheet's numbers use the new rules at once.
	if n := len(master.get(t, campaign, pc.GetId()).GetDerived().GetChangedContent()); n != 1 {
		t.Errorf("the master sees %d changed entries, want 1", n)
	}
	// A save that does not fix the mismatch keeps the notice (E10-02 state 8): the
	// master renames the sheet, and the spell is still off the list.
	same, err := master.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: pc.GetId(), Revision: got.GetRevision(), Name: "Pensantus renomeado", Sheet: got.GetSheet(),
	}))
	if err != nil {
		t.Fatalf("UpdateCharacter() rename error = %v", err)
	}
	if n := len(same.Msg.GetCharacter().GetDerived().GetChangedContent()); n != 1 {
		t.Errorf("after a rename that fixes nothing: %d changed entries, want the notice to stay (1)", n)
	}
	got = same.Msg.GetCharacter()
	// Fixing what it says (the master takes the spell out of the book) clears it.
	fixed := proto.Clone(got.GetSheet()).(*charactersv1.CharacterSheet)
	fixed.GetFull().KnownSpellKeys = fixed.GetFull().KnownSpellKeys[:10]
	up, err := master.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: pc.GetId(), Revision: got.GetRevision(), Name: got.GetName(), Sheet: fixed,
	}))
	if err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	if n := len(up.Msg.GetCharacter().GetDerived().GetChangedContent()); n != 0 {
		t.Errorf("after fixing the sheet: %d changed entries, want none", n)
	}
}

// TestTableContentAnUnrelatedEditIsNeverRefusedForAChange: a mechanical change of
// an entry a sheet uses (here a background's feature) only shows the notice; the
// sheet's other edits still save.
func TestTableContentAnUnrelatedEditIsNeverRefusedForAChange(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", owner)
	bg := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	sheet := pensantusSheet()
	sheet.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: bg.GetKey()}
	sheet.GetFull().SkillProficiencyKeys = []string{"skill:investigation", "skill:medicine"}
	pc := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", sheet)
	if got := pc.GetDerived().GetBackgroundNamePt(); got != "Guarda de farol" {
		t.Fatalf("the sheet's background = %q, want the table's", got)
	}
	changed := testBackground("Guarda de farol")
	changed.Feature = testNote("luz", "Luz-guia", "Agora ilumina o caminho todo.")
	if _, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, bg.GetKey(), bg.GetRevision(), changed))); err != nil {
		t.Fatal(err)
	}
	got := owner.get(t, campaign, pc.GetId())
	if len(got.GetDerived().GetChangedContent()) != 0 {
		t.Fatalf("changed content = %v, want none: a change with no new issue on the sheet shows nothing", got.GetDerived().GetChangedContent())
	}
	renamed, err := owner.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: pc.GetId(), Revision: got.GetRevision(), Name: "Pensantus, o Sábio", Sheet: got.GetSheet(),
	}))
	if err != nil {
		t.Fatalf("an unrelated edit after the change: error = %v, want it to save", err)
	}
	if n := len(renamed.Msg.GetCharacter().GetDerived().GetChangedContent()); n != 0 {
		t.Errorf("after the save: %d changed entries, want none", n)
	}
}

// TestTableContentArchivedIsNeverANewChoice: a sheet that picks an archived entry
// is refused with ARCHIVED_CONTENT (create, update); one that already has it keeps
// working; the level-up refuses an archived subclass.
func TestTableContentArchivedIsNeverANewChoice(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, nil)
	h.svc.SetLevelUps(xpLevelUps{})
	master, owner, other := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	bg := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	sub := master.addEntry(t, campaign, testSubclass("Caminho do Vento", "class:fighter"))

	withBackground := func(key string) *charactersv1.CharacterSheet {
		s := pensantusSheet()
		s.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: key}
		s.GetFull().SkillProficiencyKeys = []string{"skill:investigation", "skill:medicine"}
		return s
	}
	// Dona's sheet uses the background before it is archived.
	pc := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", withBackground(bg.GetKey()))
	for _, key := range []string{bg.GetKey(), sub.GetKey()} {
		if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: key})); err != nil {
			t.Fatal(err)
		}
	}
	// A new sheet picking it is refused, with the key.
	_, err := other.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Novo", Sheet: withBackground(bg.GetKey()),
	}))
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT || b.GetContentKey() != bg.GetKey() {
		t.Errorf("CreateCharacter with an archived background: %v, want ARCHIVED_CONTENT with the key", b)
	}
	// An edit that picks it is refused; one that keeps it works.
	third := other.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Terceiro", pensantusSheet())
	_, err = other.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: third.GetId(), Revision: third.GetRevision(), Name: third.GetName(), Sheet: withBackground(bg.GetKey()),
	}))
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT || b.GetCharacterId() != third.GetId() {
		t.Errorf("UpdateCharacter picking an archived background: %v, want ARCHIVED_CONTENT", b)
	}
	got := owner.get(t, campaign, pc.GetId())
	if _, err := owner.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: pc.GetId(), Revision: got.GetRevision(), Name: "Pensantus, o Sábio", Sheet: got.GetSheet(),
	})); err != nil {
		t.Errorf("a sheet that already has the archived background: error = %v, want it to keep working", err)
	}
	if got.GetDerived().GetBackgroundNamePt() != "Guarda de farol" {
		t.Errorf("the archived background's name on the sheet = %q", got.GetDerived().GetBackgroundNamePt())
	}

	// The level-up: Toren, a Fighter 2 with 900 XP, may not pick the archived subclass.
	toren := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Classes:           []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
		ExperiencePoints:  900,
	}}}
	player := h.newUser("Quarta")
	h.join(master, campaign, player)
	pcT := player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", toren)
	h.lockSheets(campaign)
	pcT = player.get(t, campaign, pcT.GetId())
	level := func(subclass string) error {
		_, err := player.api.LevelUpCharacter(t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
			CampaignId: campaign, CharacterId: pcT.GetId(), Revision: pcT.GetRevision(),
			Choices: &charactersv1.LevelUpChoices{
				ClassKey: "class:fighter", SubclassKey: subclass,
				HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
			},
		}))
		return err
	}
	err = level(sub.GetKey())
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Code() != connect.CodeFailedPrecondition {
		t.Fatalf("LevelUpCharacter with an archived subclass: error = %v, want failed_precondition", err)
	}
	var refused *charactersv1.LevelUpRefusal
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			refused, _ = v.(*charactersv1.LevelUpRefusal)
		}
	}
	if refused.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ARCHIVED_CHOICE || refused.GetField() != "full.classes[0].subclass_key" {
		t.Errorf("refusal = %v, want ARCHIVED_CHOICE at the subclass", refused)
	}
	// Brought back, it is a choice again.
	if _, err := master.table.UnarchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: campaign, Key: sub.GetKey()})); err != nil {
		t.Fatal(err)
	}
	if err := level(sub.GetKey()); err != nil {
		t.Errorf("LevelUpCharacter with the subclass back: error = %v", err)
	}
}

// TestSheetsThatUseTableContentStayInTheirCampaign: the helper copy and move paths
// call (MR-021, MR-022).
func TestSheetsThatUseTableContentStayInTheirCampaign(t *testing.T) {
	t.Parallel()
	srdSheet := pensantusSheet()
	if err := CheckSheetCanMove(srdSheet); err != nil {
		t.Errorf("an SRD sheet: error = %v, want it to move", err)
	}
	if err := CheckSheetCanMove(basicSheet()); err != nil {
		t.Errorf("a basic sheet: error = %v, want it to move", err)
	}
	table := pensantusSheet()
	table.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:guarda-de-farol@mesa"}
	err := CheckSheetCanMove(table)
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_TABLE_CONTENT_STAYS || b.GetContentKey() != "background:guarda-de-farol@mesa" {
		t.Errorf("a table sheet: %v, want TABLE_CONTENT_STAYS with the key", b)
	}
}

// TestSlugsAndFeatureKeys covers what makes the keys: no accents, the reserved
// words of the engine, the numbers.
func TestSlugsAndFeatureKeys(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"Guardião do Vale":            "guardiao-do-vale",
		"  Mão  de Ferro!! ":          "mao-de-ferro",
		"Raio de Gelo 2":              "raio-de-gelo-2",
		"---":                         "",
		"Çñü":                         "cnu",
		strings.Repeat("a", 100):      strings.Repeat("a", 60),
		"Forma Selvagem (Wild Shape)": "forma-selvagem-wild-shape",
	} {
		if got := slugify(name); got != want {
			t.Errorf("slugify(%q) = %q, want %q", name, got, want)
		}
	}
	for slug, want := range map[string]string{
		"wild-shape-do-lobo":          "wildshape-do-lobo",
		"ability-score-improvement-x": "abilityscoreimprovement-x",
		"conjuracao-spellcasting":     "conjuracao-spellcasting-x",
		"spellcasting":                "spellcasting-x",
		"":                            "class",
	} {
		if got := safeSlug(slug, "class"); got != want {
			t.Errorf("safeSlug(%q) = %q, want %q", slug, got, want)
		}
	}
	taken := map[string]bool{"a": true, "a-2": true}
	if got := uniqueSlug("a", taken); got != "a-3" || !taken["a-3"] {
		t.Errorf("uniqueSlug() = %q, want a-3, and taken", got)
	}
	long := strings.Repeat("b", 60)
	if got := uniqueSlug(long, map[string]bool{long: true}); len(got) > 60 || !strings.HasSuffix(got, "-2") {
		t.Errorf("uniqueSlug(60 characters) = %q, want at most 60 with -2", got)
	}
}

// TestPlayerCatalogWithoutArchived: the player's catalog is the master's without
// the retired entries, sharing the rest.
func TestPlayerCatalogWithoutArchived(t *testing.T) {
	t.Parallel()
	c := &rulesv1.Content{
		ContentVersion: "v", Races: []*rulesv1.Race{{Key: "a"}, {Key: "b", Archived: true}},
		Spells: []*rulesv1.Spell{{Key: "s", Archived: true}}, Classes: []*rulesv1.CharacterClass{{Key: "c"}},
	}
	p := withoutArchived(c, nil)
	if len(p.GetRaces()) != 1 || p.GetRaces()[0].GetKey() != "a" || len(p.GetSpells()) != 0 || len(p.GetClasses()) != 1 || p.GetContentVersion() != "v" {
		t.Errorf("withoutArchived() = %v", p)
	}
	if len(c.GetRaces()) != 2 {
		t.Errorf("the master's catalog lost entries: %v", c.GetRaces())
	}
}

// sheetWithTable is a player's sheet that uses the table's race, background and
// a known spell, and, for a fighter, the table's subclass.
func sheetWithTable(race, background, spell string) *charactersv1.CharacterSheet {
	s := pensantusSheet()
	f := s.GetFull()
	f.RaceKey, f.SubraceKey = race, ""
	f.Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: background}
	f.SkillProficiencyKeys = []string{"skill:investigation", "skill:medicine"}
	f.KnownSpellKeys = append(f.KnownSpellKeys, spell)
	return s
}

// TestTableContentEveryMechanicalChangeLetsTheSheetSave (question 80, the review's
// finding): the master makes every mechanical change an entry allows, and the
// player can still rename the sheet that uses it.
func TestTableContentEveryMechanicalChangeLetsTheSheetSave(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	race := master.addEntry(t, campaign, testRace("Anão das Brumas"))
	bg := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	spell := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard"))
	sub := master.addEntry(t, campaign, testSubclass("Caminho do Vento", "class:fighter"))

	pc := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", sheetWithTable(race.GetKey(), bg.GetKey(), spell.GetKey()))
	toren := other.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Classes:           []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 3, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: sub.GetKey()}}},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
	}}})

	update := func(key string, e *rulesv1.TableEntry, body proto.Message) *rulesv1.TableEntry {
		t.Helper()
		res, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, key, e.GetRevision(), cloneWithKeys(body, bodyMessage(e)))))
		if err != nil {
			t.Fatalf("UpdateTableEntry(%s) error = %v", key, err)
		}
		return res.Msg.GetEntry()
	}
	// The race: size, speed, the bonuses to place, languages, a new trait.
	r := proto.Clone(bodyMessage(race)).(*rulesv1.TableRace)
	r.Size, r.SpeedFt, r.ChoiceBonuses, r.LanguageChoices, r.DarkvisionFt = "Large", 40, []int32{1, 1, 1}, 0, 0
	r.Traits = append(r.Traits, &rulesv1.TableFeature{NamePt: "Pele de pedra", Effects: []*rulesv1.TableEffect{{Type: "modifier", Target: "ac.base", Mode: "set", Value: "14"}}})
	update(race.GetKey(), race, r)
	// The background: other skills (one the sheet also chose), no tools, a new feature.
	b := testBackground("Guarda de farol")
	b.Skills, b.Tools, b.LanguageChoices = []string{"skill:investigation", "skill:medicine"}, nil, 0
	b.Feature = testNote("outra", "Outra luz", "Texto novo.")
	update(bg.GetKey(), bg, b)
	// The spell: another circle (still leveled), school, classes, damage, concentration.
	sp := testSpell("Raio de teste", "class:cleric")
	sp.Level, sp.SchoolKey, sp.Concentration = 3, "school:necromancy", true
	sp.Duration = &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_TIMED, Amount: 1, Unit: rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_MINUTE}
	sp.Damage = []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:cold", Dice: "8d6", PerSlotLevel: "1d6"}}
	update(spell.GetKey(), spell, sp)
	// The subclass: other features.
	su := testSubclass("Caminho do Vento", "class:fighter")
	su.Levels = []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{
		{NamePt: "Golpe novo", Effects: []*rulesv1.TableEffect{{Type: "proficiency", Proficiency: "skill:athletics"}}},
		{NamePt: "Outro golpe", Effects: []*rulesv1.TableEffect{{Type: "note", TextPt: "Nota."}}},
	}}}
	update(sub.GetKey(), sub, su)

	for name, c := range map[string]struct {
		u  *user
		id string
	}{"Pensantus": {owner, pc.GetId()}, "Toren": {other, toren.GetId()}} {
		got := c.u.get(t, campaign, c.id)
		renamed, err := c.u.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
			CampaignId: campaign, CharacterId: c.id, Revision: got.GetRevision(), Name: name + " renomeado", Sheet: got.GetSheet(),
		}))
		if err != nil || renamed.Msg.GetCharacter().GetName() != name+" renomeado" {
			t.Errorf("%s: the player's rename after every mechanical change: %v, %v; want it saved", name, renamed, err)
		}
	}
	// And the master edits a locked sheet the same way.
	h.lockSheets(campaign)
	for name, id := range map[string]string{"Pensantus": pc.GetId(), "Toren": toren.GetId()} {
		got := master.get(t, campaign, id)
		if _, err := master.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
			CampaignId: campaign, CharacterId: id, Revision: got.GetRevision(), Name: name + " de novo", Sheet: got.GetSheet(),
		})); err != nil {
			t.Errorf("%s: the master's rename of the locked sheet: error = %v, want it saved", name, err)
		}
	}
}

// TestTableContentASpellNeverCrossesCantripAndLeveled: a sheet keeps cantrips and
// leveled spells in different lists, and Validate refuses a spell in the wrong
// one, so the edit is refused on the entry.
func TestTableContentASpellNeverCrossesCantripAndLeveled(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	leveled := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard"))
	cantrip := testSpell("Faísca de teste", "class:wizard")
	cantrip.Level, cantrip.Damage = 0, []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:lightning", Dice: "1d8", PerTier: "1d8"}}
	cantripEntry := master.addEntry(t, campaign, cantrip)

	toCantrip := proto.Clone(bodyMessage(leveled)).(*rulesv1.TableSpell)
	toCantrip.Level, toCantrip.Damage = 0, cantrip.Damage
	_, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, leveled.GetKey(), leveled.GetRevision(), toCantrip)))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetReason() != "immutable" || v[0].GetField() != "table_spell.level" {
		t.Errorf("leveled to cantrip: violations = %v, want immutable at level", v)
	}
	toLeveled := proto.Clone(bodyMessage(cantripEntry)).(*rulesv1.TableSpell)
	toLeveled.Level, toLeveled.Damage = 2, []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:lightning", Dice: "2d8", PerSlotLevel: "1d8"}}
	_, err = master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, cantripEntry.GetKey(), cantripEntry.GetRevision(), toLeveled)))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetReason() != "immutable" || v[0].GetField() != "table_spell.level" {
		t.Errorf("cantrip to leveled: violations = %v, want immutable at level", v)
	}
	// Leveled to another circle is fine.
	other := proto.Clone(bodyMessage(leveled)).(*rulesv1.TableSpell)
	other.Level = 3
	if _, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, leveled.GetKey(), leveled.GetRevision(), other))); err != nil {
		t.Errorf("leveled to another circle: error = %v", err)
	}
}

// TestTableContentArchivedSubclassesAreNotOffered: a player's level-up options never
// list a subclass the master archived (unless their sheet has it); the master's do,
// marked.
func TestTableContentArchivedSubclassesAreNotOffered(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 0)
	sub := tb.master.addEntry(t, tb.campaign, testSubclass("Caminho do Vento", "class:fighter"))
	live := tb.master.addEntry(t, tb.campaign, testSubclass("Caminho da Chuva", "class:fighter"))
	toren := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Classes:           []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
		ExperiencePoints:  900,
	}}}
	player := tb.h.newUser("Quarta")
	tb.h.join(tb.master, tb.campaign, player)
	pc := player.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", toren)
	tb.h.lockSheets(tb.campaign)
	pc = player.get(t, tb.campaign, pc.GetId())
	if _, err := tb.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: sub.GetKey()})); err != nil {
		t.Fatal(err)
	}
	keys := func(o *charactersv1.LevelUpOptions) map[string]bool {
		out := map[string]bool{}
		for _, s := range o.GetSubclasses() {
			out[s.GetKey()] = s.GetArchived()
		}
		return out
	}
	po, err := tb.options(player, pc)
	if err != nil {
		t.Fatalf("GetLevelUpOptions() error = %v", err)
	}
	if k := keys(po); k[sub.GetKey()] || len(k) == 0 || !hasKey(k, live.GetKey()) || hasKey(k, sub.GetKey()) {
		t.Errorf("the player's subclasses = %v, want the live one and not the archived one", k)
	}
	mo, err := tb.options(tb.master, pc)
	if err != nil {
		t.Fatalf("GetLevelUpOptions() as the master error = %v", err)
	}
	if k := keys(mo); !hasKey(k, sub.GetKey()) || !k[sub.GetKey()] || k[live.GetKey()] {
		t.Errorf("the master's subclasses = %v, want the archived one marked and the live one not", k)
	}
}

func hasKey(m map[string]bool, k string) bool { _, ok := m[k]; return ok }

// TestTableContentArchivedSpellsAreNotShown: GetSpellDetails answers not_found for
// an archived table spell, for a player or a pending member, unless one of their
// own sheets uses it; the master reads it.
func TestTableContentArchivedSpellsAreNotShown(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other, pending := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra"), h.newUser("Pendente")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	h.joinPending(master, campaign, pending)
	spell := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard"))
	owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", pensantusWith(spell.GetKey()))
	pending.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pendente", pensantusWith(spell.GetKey()))
	if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: spell.GetKey()})); err != nil {
		t.Fatal(err)
	}
	read := func(u *user) error {
		_, err := u.content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: campaign, SpellKey: spell.GetKey()}))
		return err
	}
	if err := read(master); err != nil {
		t.Errorf("the master: error = %v, want it read", err)
	}
	if err := read(other); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a player without it: error = %v, want not_found", err)
	}
	if err := read(owner); err != nil {
		t.Errorf("a player whose sheet uses it: error = %v, want it read", err)
	}
	if err := read(pending); err != nil {
		t.Errorf("a pending member whose sheet uses it: error = %v, want it read", err)
	}
	stranger := h.newUser("Pendente 2")
	h.joinPending(master, campaign, stranger)
	if err := read(stranger); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a pending member without it: error = %v, want not_found", err)
	}
}

// afterReadSource runs hook once, right after the first content read made outside a
// transaction: the moment a write of the table's content can commit between a
// handler's read and its transaction.
type afterReadSource struct {
	ContentSource
	mu   sync.Mutex
	hook func()
}

func (a *afterReadSource) ContentFor(ctx context.Context, tx pgx.Tx, campaignID string) (*rules.Content, error) {
	c, err := a.ContentSource.ContentFor(ctx, tx, campaignID)
	a.mu.Lock()
	hook := a.hook
	if tx == nil && hook != nil {
		a.hook = nil
	} else {
		hook = nil
	}
	a.mu.Unlock()
	if hook != nil {
		hook()
	}
	return c, err
}

// TestTableContentWritesAreOrderedAgainstContentWrites: an archive that commits
// between CreateCharacter's read of the content and its transaction is seen by the
// transaction, which checks the sheet again (ADR-0018, section 5).
func TestTableContentWritesAreOrderedAgainstContentWrites(t *testing.T) {
	t.Parallel()
	var src *afterReadSource
	h := newHarnessWith(t, func(c *Config) {
		src = &afterReadSource{ContentSource: c.Content}
		c.Content = src
	})
	master, owner := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", owner)
	bg := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	sheet := pensantusSheet()
	sheet.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: bg.GetKey()}
	sheet.GetFull().SkillProficiencyKeys = []string{"skill:investigation", "skill:medicine"}

	src.hook = func() {
		if _, err := master.table.ArchiveTableEntry(context.Background(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: bg.GetKey()})); err != nil {
			t.Errorf("the archive in between: %v", err)
		}
	}
	_, err := owner.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: sheet,
	}))
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT || b.GetContentKey() != bg.GetKey() {
		t.Errorf("CreateCharacter with an archive in between: %v, want ARCHIVED_CONTENT (the transaction saw the archive)", b)
	}
	// The same for an update that takes a background the master archives in between.
	pc := owner.createPensantus(t, campaign)
	bg2 := master.addEntry(t, campaign, testBackground("Outro fundo"))
	src.hook = func() {
		if _, err := master.table.ArchiveTableEntry(context.Background(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: bg2.GetKey()})); err != nil {
			t.Errorf("the archive in between: %v", err)
		}
	}
	sheet2 := pensantusSheet()
	sheet2.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: bg2.GetKey()}
	sheet2.GetFull().SkillProficiencyKeys = []string{"skill:investigation", "skill:medicine"}
	_, err = owner.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: pc.GetId(), Revision: pc.GetRevision(), Name: pc.GetName(), Sheet: sheet2,
	}))
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT || b.GetContentKey() != bg2.GetKey() {
		t.Errorf("UpdateCharacter with an archive in between: %v, want ARCHIVED_CONTENT", b)
	}
}

// TestTableContentFeatureKeysOfLongNames: two entries whose long names share a
// beginning never make the same feature keys.
func TestTableContentFeatureKeysOfLongNames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	stem := strings.Repeat("a", 40)
	a := master.addEntry(t, campaign, testClass(stem+" um"))
	b := master.addEntry(t, campaign, testClass(stem+" dois"))
	ka, kb := featureKeysOf(bodyMessage(a)), featureKeysOf(bodyMessage(b))
	if len(ka) != 3 || len(kb) != 3 {
		t.Fatalf("keys = %v and %v, want three each", ka, kb)
	}
	for _, k := range ka {
		if slices.Contains(kb, k) || len(k) > len("feature:")+rules.MaxSlugLength+len("@mesa") {
			t.Errorf("feature key %q is shared by the two entries, or too long", k)
		}
	}
}

// TestTableContentTheClientNeverSetsTheContentRevision: what a client sends in
// FullSheet.content_revision and known_issues is ignored on create, update and
// level-up: the server writes the revision it checked the sheet against.
func TestTableContentTheClientNeverSetsTheContentRevision(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.svc.SetLevelUps(xpLevelUps{})
	master, owner, player := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Quarta")
	campaign := h.newCampaign(master, "Mirathel", owner, player)
	master.addEntry(t, campaign, testBackground("Guarda de farol")) // the campaign's revision is 1

	forged := pensantusSheet()
	forged.GetFull().ContentRevision, forged.GetFull().KnownIssues = 999, []string{"unknown_key|full.race_key"}
	pc := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", forged)
	if got := pc.GetSheet().GetFull(); got.GetContentRevision() != 1 || len(got.GetKnownIssues()) != 0 {
		t.Errorf("after create: revision %d, known issues %v; want 1 and none", got.GetContentRevision(), got.GetKnownIssues())
	}
	again := proto.Clone(pc.GetSheet()).(*charactersv1.CharacterSheet)
	again.GetFull().ContentRevision, again.GetFull().KnownIssues = 777, []string{"x|y"}
	up, err := master.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: pc.GetId(), Revision: pc.GetRevision(), Name: pc.GetName(), Sheet: again,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := up.Msg.GetCharacter().GetSheet().GetFull(); got.GetContentRevision() != 1 || len(got.GetKnownIssues()) != 0 {
		t.Errorf("after update: revision %d, known issues %v; want 1 and none", got.GetContentRevision(), got.GetKnownIssues())
	}
	// Level-up: Toren, a Fighter 3 with 2,700 XP (level 4 is his), after a second table write.
	master.addEntry(t, campaign, testBackground("Outro fundo"))
	toren := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Classes:           []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 4, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: "subclass:champion"}}},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
		ExperiencePoints:  6500,
		ContentRevision:   999,
		KnownIssues:       []string{"z|z"},
	}}}
	tc := player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", toren)
	if got := tc.GetSheet().GetFull().GetContentRevision(); got != 2 {
		t.Fatalf("Toren's revision = %d, want 2", got)
	}
	master.addEntry(t, campaign, testBackground("Terceiro fundo"))
	h.lockSheets(campaign)
	tc = player.get(t, campaign, tc.GetId())
	lv, err := player.api.LevelUpCharacter(t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
		CampaignId: campaign, CharacterId: tc.GetId(), Revision: tc.GetRevision(),
		Choices: &charactersv1.LevelUpChoices{
			ClassKey:  "class:fighter",
			HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
		},
	}))
	if err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	if got := lv.Msg.GetCharacter().GetSheet().GetFull(); got.GetContentRevision() != 3 || slices.Contains(got.GetKnownIssues(), "z|z") {
		t.Errorf("after the level-up: revision %d, known issues %v; want 3 and not the forged one", got.GetContentRevision(), got.GetKnownIssues())
	}
}

// TestTableContentAnIssueIsBlamedOnItsOwnEntry (the review's probe): a spell leaves
// the wizard list, and later the text of the sheet's background changes. The sheet
// shows one banner, on the spell; the background's update affects no sheet.
func TestTableContentAnIssueIsBlamedOnItsOwnEntry(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", owner)
	bg := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	spell := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard"))
	sheet := sheetWithTable("race:gnome", bg.GetKey(), spell.GetKey())
	sheet.GetFull().SubraceKey = "subrace:rock-gnome"
	pc := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", sheet)

	moved := proto.Clone(bodyMessage(spell)).(*rulesv1.TableSpell)
	moved.ClassKeys = []string{"class:cleric"}
	if _, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, spell.GetKey(), spell.GetRevision(), moved))); err != nil {
		t.Fatal(err)
	}
	text := testBackground("Guarda de farol")
	text.Feature = testNote("luz", "Luz-guia", "Agora o texto é outro.")
	res, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, bg.GetKey(), bg.GetRevision(), cloneWithKeys(text, bodyMessage(bg)))))
	if err != nil {
		t.Fatal(err)
	}
	if a := res.Msg.GetAffectedCharacters(); len(a) != 0 {
		t.Errorf("a text edit of the background affects %v, want no sheet", a)
	}
	ch := owner.get(t, campaign, pc.GetId()).GetDerived().GetChangedContent()
	if len(ch) != 1 || ch[0].GetKey() != spell.GetKey() {
		t.Errorf("changed content = %v, want one banner, on the spell", ch)
	}
}

// TestIssuesAreToldApartByTheirWholeIdentity: the code, the field and the sentence.
func TestIssuesAreToldApartByTheirWholeIdentity(t *testing.T) {
	t.Parallel()
	a := rules.Issue{Code: "skill_count", Field: "full.skill_proficiency_keys", Message: "Faltam 1 perícias para escolher."}
	b := rules.Issue{Code: "skill_count", Field: "full.skill_proficiency_keys", Message: "Há 3 perícias escolhidas; o personagem escolhe 2."}
	f1 := rules.Issue{Code: "formula", Message: "Um efeito de A foi ignorado."}
	f2 := rules.Issue{Code: "formula", Message: "Um efeito de B foi ignorado."}
	if got := issueIDs([]rules.Issue{a, b, f1, f2, a}); len(got) != 4 {
		t.Errorf("issueIDs() = %v, want four different issues", got)
	}
}

// TestTableContentNeverNamesAnArchivedKeyToAPlayer: through references too (the
// classes of a spell, the parent of a subclass or a subrace), in the catalog, the
// entries and the spell's details, read as the player's JSON.
func TestTableContentNeverNamesAnArchivedKeyToAPlayer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	class := master.addEntry(t, campaign, testClass("Guardião secreto"))
	race := master.addEntry(t, campaign, testRace("Raça secreta"))
	sub := master.addEntry(t, campaign, testSubclass("Caminho secreto", class.GetKey()))
	subrace := master.addEntry(t, campaign, testSubrace("Sub-raça secreta", race.GetKey()))
	spell := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard", class.GetKey()))
	for _, key := range []string{class.GetKey(), race.GetKey()} {
		if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: key})); err != nil {
			t.Fatal(err)
		}
	}
	secret := func(what string, m proto.Message) {
		t.Helper()
		b, err := protojson.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{class.GetKey(), race.GetKey(), sub.GetKey(), subrace.GetKey()} {
			if strings.Contains(string(b), k) {
				t.Errorf("%s names the archived (or parentless) key %q to a player", what, k)
			}
		}
	}
	entries, err := player.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatal(err)
	}
	secret("ListTableEntries", entries.Msg)
	if e := entries.Msg.GetEntries(); len(e) != 1 || e[0].GetKey() != spell.GetKey() {
		t.Errorf("the player's entries = %v, want only the spell", e)
	}
	catalog, err := player.content.ListContent(t.Context(), connect.NewRequest(&rulesv1.ListContentRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatal(err)
	}
	secret("ListContent", catalog.Msg)
	details, err := player.content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: campaign, SpellKey: spell.GetKey()}))
	if err != nil {
		t.Fatal(err)
	}
	secret("GetSpellDetails", details.Msg)
	// The master still sees them all.
	m := master.entries(t, campaign)
	if len(m) != 5 {
		t.Errorf("the master's entries = %d, want 5", len(m))
	}
}

// An entry with more features than the engine allows is refused with
// invalid_argument before its keys are made: a body inside the request limit can
// hold thousands of them.
func TestTooManyFeaturesAreRefusedBeforeTheyAreKeyed(t *testing.T) {
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")

	raceOf := func(n int) *rulesv1.TableRace {
		race := testRace("Raça enorme")
		race.Traits = make([]*rulesv1.TableFeature, n)
		for i := range race.Traits {
			race.Traits[i] = &rulesv1.TableFeature{NamePt: "x"}
		}
		return race
	}

	// Control: a race at the limit is made, and its sixty traits named alike get sixty keys.
	created, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, raceOf(rules.MaxTraits))))
	if err != nil {
		t.Fatalf("CreateTableEntry(%d traits) error = %v", rules.MaxTraits, err)
	}
	keys := map[string]bool{}
	for _, f := range created.Msg.GetEntry().GetTableRace().GetTraits() {
		keys[f.GetKey()] = true
	}
	if len(keys) != rules.MaxTraits {
		t.Errorf("%d distinct trait keys, want %d", len(keys), rules.MaxTraits)
	}

	start := time.Now()
	_, err = master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, raceOf(5000))))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("CreateTableEntry(5000 traits) code = %v (%v), want invalid_argument", got, err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("the refusal took %v, want under 2s", elapsed)
	}

	_, err = master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, created.Msg.GetEntry().GetKey(), created.Msg.GetEntry().GetRevision(), raceOf(5000))))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("UpdateTableEntry(5000 traits) code = %v (%v), want invalid_argument", got, err)
	}
}
