package characters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	pkgv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// These tests freeze the characters' part of the campaign package (MR-050): what Export
// writes of a campaign with a bit of everything, what Stage records and prepares for each
// kind of entry a package can carry (every problem, in the order the checks run), and what
// Apply writes to the new campaign. The ids are replaced by <id-N>, in the order they first
// appear, and the times by their offset from the first of their kind.

// packagePartCap is the harness's cap on a campaign's characters, low so a package can
// pass it.
const packagePartCap = 12

// packagePartEntry is one entry of a package: its kind, its path and its JSON.
type packagePartEntry struct {
	kind pkgv1.PackageEntryKind
	name string
	data []byte
}

// packagePartSnapshotEntries reads what Export added to the snapshot. The snapshot keeps its
// entries to itself (only the package service writes them to a zip), so the test reads the
// unexported fields through reflect, read-only.
func packagePartSnapshotEntries(t *testing.T, snap *campaignpackage.Snapshot) []packagePartEntry {
	t.Helper()
	list := reflect.ValueOf(snap).Elem().FieldByName("entries")
	out := make([]packagePartEntry, 0, list.Len())
	for i := range list.Len() {
		e := list.Index(i)
		out = append(out, packagePartEntry{
			kind: pkgv1.PackageEntryKind(e.FieldByName("kind").Int()), //nolint:gosec // G115: an enum
			name: e.FieldByName("name").String(),
			data: slices.Clone(e.FieldByName("data").Bytes()),
		})
	}
	return out
}

// packagePartOpen writes the entries to a zip as an export does and opens it as an import
// does.
func packagePartOpen(t *testing.T, entries []packagePartEntry) *campaignpackage.Package {
	t.Helper()
	var buf bytes.Buffer
	w := campaignpackage.NewWriter(&buf, time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	for _, e := range entries {
		if err := w.AddBytes(e.kind, e.name, e.data, 0); err != nil {
			t.Fatalf("AddBytes(%s) error = %v", e.name, err)
		}
	}
	if err := w.Close("test", "Mirathel"); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	pkg, problems := campaignpackage.Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if len(problems) > 0 {
		t.Fatalf("Open() problems = %v", problems)
	}
	return pkg
}

// packagePartJSON is a message as the package writes it.
func packagePartJSON(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// packagePartAny is JSON bytes as a value the golden can hold.
func packagePartAny(t *testing.T, b []byte) any {
	t.Helper()
	if len(b) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	return v
}

// packagePartWith is base with the entries of kind dropped (when drop is set) and more added.
func packagePartWith(base []packagePartEntry, drop pkgv1.PackageEntryKind, more ...packagePartEntry) []packagePartEntry {
	out := slices.DeleteFunc(slices.Clone(base), func(e packagePartEntry) bool { return drop != 0 && e.kind == drop })
	return append(out, more...)
}

// packagePartContent is a content entry of the package, at a path of its own.
func packagePartContent(t *testing.T, name string, te *rulesv1.TableEntry) packagePartEntry {
	t.Helper()
	return packagePartEntry{kind: pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT, name: name, data: packagePartJSON(t, &pkgv1.PackageContent{Entry: te})}
}

// packagePartStaged is what Stage recorded and prepared, as the golden keeps it.
func packagePartStaged(t *testing.T, in *campaignpackage.Import, withData bool) map[string]any {
	t.Helper()
	out := map[string]any{"counts": packagePartAny(t, packagePartJSON(t, in.Counts))}
	var problems []any
	for _, p := range in.Problems.List {
		problems = append(problems, packagePartAny(t, packagePartJSON(t, p)))
	}
	out["problems"] = problems
	if sc, _ := in.Get(stashContent).(*stagedContent); sc != nil {
		var entries []any
		for _, e := range sc.entries {
			row := map[string]any{
				"key": e.row.ContentKey, "kind": e.row.Kind, "name_pt": e.row.NamePt, "revision": e.row.Revision,
				"archived": e.row.ArchivedAt != nil, "created_after": e.row.CreatedAt.Sub(sc.entries[0].row.CreatedAt).String(),
				"updated_is_created": e.row.UpdatedAt.Equal(e.row.CreatedAt),
			}
			if withData {
				row["data"] = packagePartAny(t, e.row.Data)
			}
			entries = append(entries, row)
		}
		out["content"] = map[string]any{"entries": entries, "off": sc.off}
	}
	if chars, _ := in.Get(stashCharacters).(*stagedCharacters); chars != nil {
		var first time.Time
		list := func(cs []*stagedCharacter) []any {
			var l []any
			for _, c := range cs {
				if first.IsZero() {
					first = c.createdAt
				}
				l = append(l, map[string]any{
					"id": c.id, "name": c.name, "kind": c.kind, "notes": c.notes, "created_after": c.createdAt.Sub(first).String(),
					"sheet": packagePartAny(t, c.sheet), "story": packagePartAny(t, c.story),
				})
			}
			return l
		}
		npcs := list(chars.npcs)
		out["characters"] = map[string]any{"npcs": npcs, "players": list(chars.players), "facts_players": len(in.Facts.Players)}
	}
	return out
}

// packagePartIDs replaces every UUID of the golden by <id-N>, in the order they appear.
var packagePartIDs = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func packagePartAliased(t *testing.T, v any) any {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	text := packagePartIDs.ReplaceAllStringFunc(string(b), func(id string) string {
		if a, ok := seen[id]; ok {
			return a
		}
		seen[id] = fmt.Sprintf("<id-%d>", len(seen)+1)
		return seen[id]
	})
	// The exported entries carry the times they were made at, which move from run to run.
	text = packagePartTimes.ReplaceAllString(text, `"<time>"`)
	var out any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// packagePartTimes matches a timestamp as protojson writes it.
var packagePartTimes = regexp.MustCompile(`"\d{4}-\d\d-\d\dT[0-9:.]+Z"`)

// packagePartRows reads what an import wrote to a campaign: its characters, the master's
// notes, the table content, the options switched off and the content revision.
func packagePartRows(t *testing.T, h *harness, campaignID, masterID string) map[string]any {
	t.Helper()
	ctx := t.Context()
	out := map[string]any{}
	rows, err := h.pool.Query(ctx, `SELECT c.id::TEXT, c.kind, c.status, c.name, c.sheet, c.story, c.reserved,
			COALESCE(c.master_user_id::TEXT = $2, false), c.player_user_id IS NULL, COALESCE(n.notes, '')
		FROM characters AS c LEFT JOIN character_master_notes AS n ON n.character_id = c.id
		WHERE c.campaign_id = $1 ORDER BY c.created_at, c.id`, campaignID, masterID)
	if err != nil {
		t.Fatal(err)
	}
	var chars []any
	for rows.Next() {
		var id, kind, status, name, notes string
		var sheet, story []byte
		var reserved, byMaster, noPlayer bool
		if err := rows.Scan(&id, &kind, &status, &name, &sheet, &story, &reserved, &byMaster, &noPlayer, &notes); err != nil {
			t.Fatal(err)
		}
		chars = append(chars, map[string]any{
			"id": id, "kind": kind, "status": status, "name": name, "sheet": packagePartAny(t, sheet), "story": packagePartAny(t, story),
			"reserved": reserved, "master_is_the_importer": byMaster, "no_player": noPlayer, "notes": notes,
		})
	}
	rows.Close()
	out["characters"] = chars
	rows, err = h.pool.Query(ctx, `SELECT content_key, kind, name_pt, revision, archived_at IS NOT NULL FROM campaign_content
		WHERE campaign_id = $1 ORDER BY created_at, content_key`, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	var content []any
	for rows.Next() {
		var key, kind, name string
		var revision int64
		var archived bool
		if err := rows.Scan(&key, &kind, &name, &revision, &archived); err != nil {
			t.Fatal(err)
		}
		content = append(content, map[string]any{"key": key, "kind": kind, "name_pt": name, "revision": revision, "archived": archived})
	}
	rows.Close()
	out["content"] = content
	var off []string
	if err := h.pool.QueryRow(ctx, `SELECT COALESCE(array_agg(content_key ORDER BY content_key), ARRAY[]::TEXT[]) FROM campaign_content_off WHERE campaign_id = $1`, campaignID).Scan(&off); err != nil {
		t.Fatal(err)
	}
	out["off"] = off
	var revision *int64
	if err := h.pool.QueryRow(ctx, `SELECT max(revision) FROM campaign_content_state WHERE campaign_id = $1`, campaignID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	out["content_revision"] = revision
	return out
}

// TestPackagePartGoldenOfExportStageAndApply exports a campaign with table content, an
// option switched off, an active, a pending and a dead player's character, NPCs of each
// kind (a full sheet, a basic one, one from the bestiary, the app's copy of a creature put
// in combat) and the master's notes; stages that package and every broken variant of it;
// and applies the good ones to new campaigns.
func TestPackagePartGoldenOfExportStageAndApply(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(c *Config) { c.MaxCharactersPerCampaign = packagePartCap })
	ctx := t.Context()
	master := h.newUser("Mestre")
	active, pending, dying := h.newUser("Ativa"), h.newUser("Pendente"), h.newUser("Morta")
	campaign := h.newCampaign(master, "Mirathel", active, dying)
	h.joinPending(master, campaign, pending)

	class := master.addEntry(t, campaign, testClass("Andarilho"))
	master.addEntry(t, campaign, testSpell("Raio de teste", class.GetKey()))
	master.addEntry(t, campaign, testBackground("Guarda de farol"))
	master.addEntry(t, campaign, testRace("Povo do vale"))
	master.setOff(t, campaign, true, "spell:fireball")

	pc := active.createPensantus(t, campaign)
	pending.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pedido", wizardAt(1))
	dead := dying.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Caído", wizardAt(1))
	master.markDead(t, dead)
	enemy := master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, "Capitão", enemySheet())
	master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin", basicSheet())
	if _, err := npcFromCreature(t, master, campaign, "monster:ogre", "Grak", charactersv1.CharacterKind_CHARACTER_KIND_MINION, "00000000-0000-4000-8000-0000000000b1"); err != nil {
		t.Fatalf("npcFromCreature() error = %v", err)
	}
	if err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		_, _, err := h.svc.MonsterNpc(ctx, tx, campaign, master.id, "monster:wolf", h.clock.Now())
		return err
	}); err != nil {
		t.Fatalf("MonsterNpc() error = %v", err)
	}
	for _, c := range []*charactersv1.Character{pc, enemy} {
		if _, err := master.api.UpdateMasterNotes(ctx, connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{
			CampaignId: campaign, CharacterId: c.GetId(), Notes: "Notas sobre " + c.GetName() + ".",
		})); err != nil {
			t.Fatalf("UpdateMasterNotes() error = %v", err)
		}
	}

	part := h.svc.PackagePart()
	snap := campaignpackage.NewSnapshot(campaign)
	if err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		*snap = *campaignpackage.NewSnapshot(campaign)
		return part.Export(ctx, tx, campaign, snap)
	}); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	base := packagePartSnapshotEntries(t, snap)
	var exported []any
	for _, e := range base {
		exported = append(exported, map[string]any{"kind": e.kind.String(), "name": e.name, "body": packagePartAny(t, e.data)})
	}

	// A stored sheet or story the app cannot read stops the export, with no entry for it.
	for _, column := range []string{"sheet", "story"} {
		broken := h.newCampaign(master, "Corrompida "+column)
		c := master.create(t, broken, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, "Ilegível", enemySheet())
		// A field of the wrong type: an object, as the column's check asks, that does not decode.
		if _, err := h.pool.Exec(ctx, `UPDATE characters SET `+column+` = '{"full": 5, "backstory": 5}' WHERE id = $1`, c.GetId()); err != nil {
			t.Fatal(err)
		}
		err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
			return part.Export(ctx, tx, broken, campaignpackage.NewSnapshot(broken))
		})
		exported = append(exported, map[string]any{"export of a campaign with an unreadable " + column: fmt.Sprint(err)})
	}

	stage := func(entries []packagePartEntry, campaignID string) *campaignpackage.Import {
		t.Helper()
		in := campaignpackage.NewImport(packagePartOpen(t, entries), campaignID, master.id, false, nil)
		if err := part.Stage(ctx, in); err != nil {
			t.Fatalf("Stage() error = %v", err)
		}
		return in
	}
	staged := map[string]any{}
	for name, entries := range packagePartVariants(t, base) {
		staged[name] = packagePartStaged(t, stage(entries, "00000000-0000-4000-8000-0000000000c1"), false)
	}

	applied := map[string]any{}
	apply := func(name string, entries []packagePartEntry) {
		t.Helper()
		target := h.newCampaign(master, "Destino "+name)
		in := stage(entries, target)
		staged["applied: "+name] = packagePartStaged(t, in, true)
		if err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error { return part.Apply(ctx, tx, in) }); err != nil {
			t.Fatalf("Apply(%s) error = %v", name, err)
		}
		applied[name] = packagePartRows(t, h, target, master.id)
	}
	apply("the exported campaign", base)
	options := slices.DeleteFunc(slices.Clone(base), func(e packagePartEntry) bool {
		return e.kind != pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT_OPTIONS
	})
	apply("only the options switched off", options)
	apply("nothing of the characters' part", nil)
	apply("a content problem, so nothing is staged", packagePartWith(base, 0, packagePartEntry{
		kind: pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT, name: "content/0099.json", data: []byte("{"),
	}))

	checkCombatReadsGolden(t, "package_part.golden.json", packagePartAliased(t, map[string]any{
		"export": exported, "stage": staged, "apply": applied,
	}))
}

// packagePartVariants are the exported package and every broken variant of it, by name.
func packagePartVariants(t *testing.T, base []packagePartEntry) map[string][]packagePartEntry {
	t.Helper()
	content := pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT
	optionsKind := pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CONTENT_OPTIONS
	npcKind := pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_NPC
	charKind := pkgv1.PackageEntryKind_PACKAGE_ENTRY_KIND_CHARACTER

	// The exported entries, decoded, to make the variants from.
	var entries []*rulesv1.TableEntry
	var npc *pkgv1.PackageNpc
	var player *pkgv1.PackageCharacter
	for _, e := range base {
		switch e.kind {
		case content:
			var pc pkgv1.PackageContent
			if err := protojson.Unmarshal(e.data, &pc); err != nil {
				t.Fatal(err)
			}
			entries = append(entries, pc.GetEntry())
		case npcKind:
			if npc == nil {
				npc = &pkgv1.PackageNpc{}
				if err := protojson.Unmarshal(e.data, npc); err != nil {
					t.Fatal(err)
				}
			}
		case charKind:
			player = &pkgv1.PackageCharacter{}
			if err := protojson.Unmarshal(e.data, player); err != nil {
				t.Fatal(err)
			}
		default:
		}
	}
	entryOfKind := func(kind rulesv1.TableContentKind) *rulesv1.TableEntry {
		for _, e := range entries {
			if e.GetKind() == kind {
				return proto.CloneOf(e)
			}
		}
		t.Fatalf("the export has no %v", kind)
		return nil
	}
	background := entryOfKind(rulesv1.TableContentKind_TABLE_CONTENT_KIND_BACKGROUND)
	spell := entryOfKind(rulesv1.TableContentKind_TABLE_CONTENT_KIND_SPELL)
	class := entryOfKind(rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS)

	out := map[string][]packagePartEntry{"the exported campaign": base}

	var many []packagePartEntry
	for i := range 1001 {
		many = append(many, packagePartContent(t, fmt.Sprintf("content/9%04d.json", i), background))
	}
	out["more table entries than the overlay takes"] = packagePartWith(base, content, many...)

	edit := func(te *rulesv1.TableEntry, change func(*rulesv1.TableEntry)) *rulesv1.TableEntry {
		c := proto.CloneOf(te)
		change(c)
		return c
	}
	// Entries 0104, 0107 and 0108 repeat the name of an entry the package already
	// has in their kind, so staging refuses them as duplicates (the repeated-name
	// finding) before the compile that would judge the rest of what they hold.
	out["every kind of broken table entry"] = packagePartWith(base, 0,
		packagePartEntry{kind: content, name: "content/0100.json", data: []byte("{")},
		packagePartContent(t, "content/0101.json", edit(background, func(e *rulesv1.TableEntry) { e.Key = "spell:guarda-de-farol@mesa" })),
		packagePartContent(t, "content/0102.json", edit(background, func(e *rulesv1.TableEntry) { e.Key = "background:" + strings.Repeat("a", 200) + "@mesa" })),
		packagePartContent(t, "content/0103.json", edit(background, func(e *rulesv1.TableEntry) { e.NamePt = "Outro nome" })),
		packagePartContent(t, "content/0104.json", edit(background, func(e *rulesv1.TableEntry) { e.Key = "background:sem-forma@mesa" })),
		packagePartContent(t, "content/0105.json", edit(background, func(e *rulesv1.TableEntry) { e.Kind = rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE })),
		packagePartContent(t, "content/0106.json", background),
		packagePartContent(t, "content/0107.json", edit(spell, func(e *rulesv1.TableEntry) {
			e.Key, e.GetTableSpell().Level = "spell:nivel-alto@mesa", 99
		})),
		packagePartContent(t, "content/0108.json", edit(background, func(e *rulesv1.TableEntry) {
			e.Key = "background:chave-alheia@mesa"
			e.GetTableBackground().GetFeature().Key = "background-feature:outra--0000--x"
		})),
		packagePartContent(t, "content/0109.json", edit(spell, func(e *rulesv1.TableEntry) {
			e.Key = "spell:enorme@mesa"
			e.GetTableSpell().DescPt = []string{strings.Repeat("Texto longo. ", 6000)}
		})),
		packagePartContent(t, "content/0110.json", &rulesv1.TableEntry{Key: "feat:vazio@mesa", Kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, NamePt: "Vazio"}),
		packagePartContent(t, "content/0111.json", edit(class, func(e *rulesv1.TableEntry) {
			e.Key = "class:espacos-errados@mesa"
			e.GetTableClass().GetLevels()[0].Slots = []int32{2, 1, 1}
		})),
		packagePartContent(t, "content/0112.json", edit(entryOfKind(rulesv1.TableContentKind_TABLE_CONTENT_KIND_RACE), func(e *rulesv1.TableEntry) {
			e.Key = "race:tracos-demais@mesa"
			r := e.GetTableRace()
			for i := range rules.MaxTraits + 1 {
				r.Traits = append(r.Traits, &rulesv1.TableFeature{NamePt: fmt.Sprintf("Traço %d", i), Effects: []*rulesv1.TableEffect{{Type: "note"}}})
			}
		})),
	)
	// By hand and in a package alike, a second entry of a kind with a name another has is
	// refused (duplicate_name).
	out["two entries of one name"] = packagePartWith(base, 0, packagePartContent(t, "content/0130.json", edit(background, func(e *rulesv1.TableEntry) {
		e.Key = "background:outro-farol@mesa"
		e.GetTableBackground().GetFeature().Key = ""
	})))
	out["an archived table entry"] = packagePartWith(base, 0, packagePartContent(t, "content/0120.json", edit(background, func(e *rulesv1.TableEntry) {
		e.Key, e.Archived = "background:arquivado@mesa", true
		e.NamePt, e.GetTableBackground().NamePt = "Arquivado", "Arquivado"
		e.GetTableBackground().GetFeature().Key = ""
	})))
	out["the class the spell needs left out"] = slices.DeleteFunc(slices.Clone(base), func(e packagePartEntry) bool {
		return e.kind == content && bytes.Contains(e.data, []byte(`"table_class"`))
	})
	optionsEntry := func(name string, keys ...string) packagePartEntry {
		return packagePartEntry{kind: optionsKind, name: name, data: packagePartJSON(t, &pkgv1.PackageContentOptions{Disabled: keys})}
	}
	out["two entries of options"] = packagePartWith(base, optionsKind, optionsEntry("content/options.json", "spell:fireball"), optionsEntry("content/options2.json"))
	out["the options under another path"] = packagePartWith(base, optionsKind, optionsEntry("content/outras.json", "spell:fireball"))
	out["unreadable options"] = packagePartWith(base, optionsKind, packagePartEntry{kind: optionsKind, name: "content/options.json", data: []byte("[")})
	out["options with an unknown key and a repeat"] = packagePartWith(base, optionsKind,
		optionsEntry("content/options.json", "spell:fireball", "spell:nao-existe", "spell:fireball", "class:wizard"))

	npcEntry := func(name string, change func(*pkgv1.PackageNpc)) packagePartEntry {
		n := proto.CloneOf(npc)
		change(n)
		return packagePartEntry{kind: npcKind, name: name, data: packagePartJSON(t, n)}
	}
	charEntry := func(name string, change func(*pkgv1.PackageCharacter)) packagePartEntry {
		c := proto.CloneOf(player)
		change(c)
		return packagePartEntry{kind: charKind, name: name, data: packagePartJSON(t, c)}
	}
	var crowd []packagePartEntry
	for i := range packagePartCap + 1 {
		crowd = append(crowd, npcEntry(fmt.Sprintf("npcs/9%04d.json", i), func(n *pkgv1.PackageNpc) { n.Id = fmt.Sprintf("00000000-0000-4000-8000-%012d", i) }))
	}
	out["more characters than the cap"] = packagePartWith(base, npcKind, crowd...)

	id := func(i int) string { return fmt.Sprintf("00000000-0000-4000-9000-%012d", i) }
	basic := func(b *charactersv1.BasicSheet) *charactersv1.CharacterSheet {
		return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: b}}
	}
	goblin := basicSheet().GetBasic()
	// Two lists, each under the cap once the package's own characters are left out.
	noCharacters := packagePartWith(packagePartWith(base, npcKind), charKind)
	out["broken characters, first half"] = packagePartWith(noCharacters, 0,
		packagePartEntry{kind: npcKind, name: "npcs/0100.json", data: []byte("{")},
		npcEntry("npcs/0101.json", func(n *pkgv1.PackageNpc) { n.Id, n.Name = id(1), "   " }),
		npcEntry("npcs/0102.json", func(n *pkgv1.PackageNpc) { n.Id = "nao-e-um-id" }),
		npcEntry("npcs/0103.json", func(n *pkgv1.PackageNpc) { n.Id = id(17) }),
		npcEntry("npcs/0104.json", func(n *pkgv1.PackageNpc) { n.Id, n.Sheet = id(4), nil }),
		npcEntry("npcs/0105.json", func(n *pkgv1.PackageNpc) { n.Id, n.MasterNotes = id(5), strings.Repeat("n", MaxMasterNotesLength+1) }),
		npcEntry("npcs/0106.json", func(n *pkgv1.PackageNpc) { n.Id, n.Kind = id(6), charactersv1.CharacterKind_CHARACTER_KIND_UNSPECIFIED }),
		npcEntry("npcs/0107.json", func(n *pkgv1.PackageNpc) { n.Id, n.Kind = id(7), charactersv1.CharacterKind_CHARACTER_KIND_PLAYER }),
		npcEntry("npcs/0108.json", func(n *pkgv1.PackageNpc) {
			b := proto.CloneOf(goblin)
			b.CombatOnly = true
			n.Id, n.Kind, n.Sheet = id(8), charactersv1.CharacterKind_CHARACTER_KIND_MINION, basic(b)
		}),
		npcEntry("npcs/0117.json", func(n *pkgv1.PackageNpc) { n.Id = id(17) }),
	)
	out["broken characters, second half"] = packagePartWith(noCharacters, 0,
		npcEntry("npcs/0109.json", func(n *pkgv1.PackageNpc) {
			n.Id, n.Name = id(9), "Retratado"
			n.GetSheet().GetFull().PortraitImageId = "00000000-0000-4000-a000-000000000009"
		}),
		npcEntry("npcs/0110.json", func(n *pkgv1.PackageNpc) {
			n.Id = id(10)
			n.GetSheet().GetFull().Classes[0].ClassKey = "class:nao-existe@mesa"
		}),
		npcEntry("npcs/0111.json", func(n *pkgv1.PackageNpc) {
			n.Id = id(11)
			n.GetSheet().GetFull().Classes[0].Level = 0
		}),
		npcEntry("npcs/0112.json", func(n *pkgv1.PackageNpc) {
			b := proto.CloneOf(goblin)
			b.MonsterKey = "monster:nao-existe"
			n.Id, n.Kind, n.Sheet = id(12), charactersv1.CharacterKind_CHARACTER_KIND_MINION, basic(b)
		}),
		npcEntry("npcs/0113.json", func(n *pkgv1.PackageNpc) {
			b := proto.CloneOf(goblin)
			b.MonsterKey, b.AbilityScores = "monster:goblin", &rulesv1.AbilityScores{Strength: 0, Dexterity: 14, Constitution: 10, Intelligence: 10, Wisdom: 8, Charisma: 8}
			n.Id, n.Kind, n.Sheet = id(13), charactersv1.CharacterKind_CHARACTER_KIND_MINION, basic(b)
		}),
		npcEntry("npcs/0114.json", func(n *pkgv1.PackageNpc) {
			b := proto.CloneOf(goblin)
			b.MonsterKey, b.AbilityScores = "monster:goblin", &rulesv1.AbilityScores{Strength: 8, Dexterity: 14, Constitution: 10, Intelligence: 10, Wisdom: 8, Charisma: 8}
			n.Id, n.Kind, n.Name, n.Sheet = id(14), charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin do bestiário", basic(b)
		}),
		npcEntry("npcs/0115.json", func(n *pkgv1.PackageNpc) {
			b := proto.CloneOf(goblin)
			b.MonsterKey = "monster:goblin"
			n.Id, n.Kind, n.Name, n.Sheet = id(15), charactersv1.CharacterKind_CHARACTER_KIND_STORY, "Goblin sem valores", basic(b)
		}),
		npcEntry("npcs/0116.json", func(n *pkgv1.PackageNpc) {
			n.Id, n.Story = id(16), &charactersv1.CharacterStory{Backstory: strings.Repeat("h", 100000)}
		}),
		charEntry("characters/0100.json", func(c *pkgv1.PackageCharacter) {
			c.Id, c.Name = id(30), "Andarilha"
			f := c.GetSheet().GetFull()
			f.Classes = []*charactersv1.ClassLevel{{ClassKey: class.GetKey(), Level: 1}}
			f.CantripKeys, f.KnownSpellKeys, f.PreparedSpellKeys = nil, nil, nil
		}),
		charEntry("characters/0101.json", func(c *pkgv1.PackageCharacter) { c.Id, c.Name = id(31), "" }),
		packagePartEntry{kind: charKind, name: "characters/0102.json", data: []byte("{")},
	)
	return out
}
