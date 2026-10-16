package characters

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Characterization of the table content's RPCs (MR-025, RN-23): what each viewer reads of
// the table's entries once the master hid some of them (archived, switched off, or under a
// hidden parent, with references to hidden SRD keys), what each switch answers, and the exact
// shape of every refusal of the writes, each for who may call it. The answers are golden
// files (testdata/table_content_*.golden.json, ids replaced by names).

// viewersTable is the campaign of the viewers' golden: a master, Dona with a sheet that uses
// the table's race, subrace, background and spell, Davi without one, a pending member and an
// outsider.
type viewersTable struct {
	h                                     *harness
	live                                  *liveSpy
	master, dona, davi, pending, outsider *user
	campaign                              string
	keys                                  map[string]string // the entries' keys, by a short name
	steps                                 []tableContentGoldenStep
}

func newViewersTable(t *testing.T) *viewersTable {
	t.Helper()
	h := newHarness(t)
	v := &viewersTable{h: h, live: &liveSpy{}, keys: map[string]string{}}
	h.svc.SetLive(v.live)
	v.master, v.dona, v.davi, v.pending, v.outsider = h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Davi"), h.newUser("Pendente"), h.newUser("De fora")
	v.campaign = h.newCampaign(v.master, "Mirathel", v.dona, v.davi)
	h.joinPending(v.master, v.campaign, v.pending)
	return v
}

// add creates an entry as the master and names its key.
func (v *viewersTable) add(t *testing.T, name string, body proto.Message) string {
	t.Helper()
	v.keys[name] = v.master.addEntry(t, v.campaign, body).GetKey()
	return v.keys[name]
}

// fill writes every kind of entry, each with references to the SRD keys the master will
// switch off (class:druid, spell:cure-wounds), and Dona's sheet.
func (v *viewersTable) fill(t *testing.T) {
	t.Helper()
	d, menu := v.master.classDefaults(t, v.campaign), v.master.effectMenu(t, v.campaign)
	guardian := guardianClass(t, d, menu)
	guardian.Levels[0].Features = append(guardian.Levels[0].Features, &rulesv1.TableFeature{
		NamePt: "Dom do Vale", Effects: []*rulesv1.TableEffect{{Type: "note", TextPt: "Dois dons.", Spells: []string{"spell:cure-wounds", "spell:fire-bolt"}}},
	})
	class := v.add(t, "class", guardian)
	v.add(t, "subclass", valleyPath(class))
	blade := inkBlade(t, d)
	blade.Casting.ListFrom = "class:druid"
	v.add(t, "third caster", blade)
	race := v.add(t, "race", testRace("Anão das Brumas"))
	subrace := v.add(t, "subrace", testSubrace("Da Colina", race))
	secret := v.add(t, "secret race", testRace("Raça secreta"))
	v.add(t, "secret subrace", testSubrace("Sub-raça secreta", secret))
	bg := testBackground("Guarda de farol")
	bg.Feature.Effects[0].Spells = []string{"spell:cure-wounds", "spell:fire-bolt"}
	background := v.add(t, "background", bg)
	spell := v.add(t, "spell", testSpell("Raio de teste", "class:wizard", "class:druid", class))
	v.add(t, "archived spell", testSpell("Raio arquivado", "class:wizard"))
	feat := testFeat("Punho de ferro")
	feat.Prerequisite.RaceKey = race
	v.add(t, "feat", feat)
	v.add(t, "off feat", testFeat("Talento desligado"))

	sheet := sheetWithTable(race, background, spell)
	sheet.GetFull().SubraceKey = subrace
	v.dona.create(t, v.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Icaro", sheet)
}

// step records the answer of a call in the golden.
func (v *viewersTable) step(label string, res proto.Message, err error) {
	if err != nil {
		v.steps = append(v.steps, tableContentGoldenStep{label, err})
		return
	}
	v.steps = append(v.steps, tableContentGoldenStep{label, res})
}

// switches calls SetOptionSwitches as the master with each key's off.
func (v *viewersTable) switches(t *testing.T, label string, switches ...*rulesv1.OptionSwitch) {
	t.Helper()
	res, err := v.master.table.SetOptionSwitches(t.Context(), connect.NewRequest(&rulesv1.SetOptionSwitchesRequest{CampaignId: v.campaign, Switches: switches}))
	if err != nil {
		t.Fatalf("SetOptionSwitches(%s) error = %v", label, err)
	}
	v.step("switches: "+label, res.Msg, nil)
}

// archive retires an entry as the master.
func (v *viewersTable) archive(t *testing.T, name string) {
	t.Helper()
	res, err := v.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: v.campaign, Key: v.keys[name]}))
	if err != nil {
		t.Fatalf("ArchiveTableEntry(%s) error = %v", name, err)
	}
	v.step("archive "+name, res.Msg, nil)
}

// read records ListTableEntries as u.
func (v *viewersTable) read(t *testing.T, label string, u *user) {
	t.Helper()
	res, err := u.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: v.campaign}))
	if err != nil {
		v.step(label, nil, err)
		return
	}
	v.step(label, res.Msg, nil)
}

// names are the ids of the golden replaced by names.
func (v *viewersTable) names() map[string]string {
	return map[string]string{
		v.campaign: "<campaign>", v.master.id: "<samuel>", v.dona.id: "<dona>", v.davi.id: "<davi>",
		v.pending.id: "<pending>", v.outsider.id: "<outsider>",
	}
}

func optionSwitchOf(key string, isOff bool) *rulesv1.OptionSwitch {
	return &rulesv1.OptionSwitch{Key: key, Off: isOff}
}

// TestTableEntriesGoldenOfEveryViewer: what the master, a player whose sheet uses hidden
// entries, a player without a sheet, a pending member, an outsider and nobody read of the
// table's entries, before and after the master switches options off and archives some, and
// what each switch answers (only the options whose state changed for the players, the
// characters that use them, no change when nothing changes).
func TestTableEntriesGoldenOfEveryViewer(t *testing.T) {
	t.Parallel()
	v := newViewersTable(t)
	v.fill(t)
	v.read(t, "nothing hidden: master", v.master)
	v.read(t, "nothing hidden: dona", v.dona)
	hintsBefore := v.hintsOf()

	v.switches(t, "off the druid, cure wounds, the race and a feat",
		optionSwitchOf("class:druid", true), optionSwitchOf("spell:cure-wounds", true), optionSwitchOf(v.keys["race"], true), optionSwitchOf(v.keys["off feat"], true))
	v.switches(t, "the same again changes nothing", optionSwitchOf("class:druid", true), optionSwitchOf("spell:fire-bolt", false))
	v.switches(t, "on and off in one call", optionSwitchOf("class:druid", false), optionSwitchOf("spell:magic-missile", true))
	v.switches(t, "the druid off again, magic missile on", optionSwitchOf("class:druid", true), optionSwitchOf("spell:magic-missile", false))
	v.archive(t, "secret race")
	v.archive(t, "archived spell")
	v.switches(t, "an archived entry switched off", optionSwitchOf(v.keys["archived spell"], true))

	for _, viewer := range []struct {
		label string
		u     *user
	}{
		{"master", v.master},
		{"dona, whose sheet uses the race and the subrace", v.dona},
		{"davi, without a sheet", v.davi},
		{"pending", v.pending},
		{"outsider", v.outsider},
		{"nobody", v.h.anonymous()},
	} {
		v.read(t, "hidden: "+viewer.label, viewer.u)
	}
	if n := v.hintsOf() - hintsBefore; n != 6 {
		t.Errorf("content_changed hints = %d, want 6: four switches that changed something and two archives", n)
	}
	tableContentGolden(t, "viewers", v.names(), v.steps...)
}

// hintsOf counts the content_changed hints of the campaign.
func (v *viewersTable) hintsOf() int {
	v.live.mu.Lock()
	defer v.live.mu.Unlock()
	return strings.Count(strings.Join(v.live.content, " "), v.campaign)
}

// tableRefusalCall is one call of the refusals' golden.
type tableRefusalCall struct {
	label string
	call  func() error
}

// TestTableContentRefusalsGolden: the code, the message and the details of every refusal of
// the table content's writes and of the read, for each kind of caller, in the order the
// checks run (the first one that fails is the answer).
func TestTableContentRefusalsGolden(t *testing.T) {
	t.Parallel()
	v := newViewersTable(t)
	ctx := t.Context()
	m, c := v.master, v.campaign
	race := v.add(t, "race", testRace("Anão das Brumas"))
	sub := v.add(t, "subrace", testSubrace("Da Colina", race))
	elf := v.add(t, "second race", testRace("Elfo Pálido"))
	subclass := v.add(t, "subclass", testSubclass("Caminho do Vento", "class:fighter"))
	cantrip := testSpell("Truque", "class:wizard")
	cantrip.Level, cantrip.Damage[0].PerSlotLevel = 0, ""
	trick := v.add(t, "cantrip", cantrip)
	background := v.add(t, "background", testBackground("Guarda de farol"))
	entries := m.entries(t, c)
	revOf := func(key string) int32 { return entries[key].GetRevision() }
	nobody := v.h.anonymous()

	create := func(u *user, body proto.Message) func() error {
		return func() error {
			_, err := u.table.CreateTableEntry(ctx, connect.NewRequest(createReq(c, body)))
			return err
		}
	}
	createReqOf := func(r *rulesv1.CreateTableEntryRequest) func() error {
		return func() error { _, err := m.table.CreateTableEntry(ctx, connect.NewRequest(r)); return err }
	}
	update := func(u *user, key string, rev int32, body proto.Message) func() error {
		return func() error {
			_, err := u.table.UpdateTableEntry(ctx, connect.NewRequest(updateReq(c, key, rev, body)))
			return err
		}
	}
	archive := func(u *user, key string, archived bool) func() error {
		return func() error {
			if archived {
				_, err := u.table.ArchiveTableEntry(ctx, connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: c, Key: key}))
				return err
			}
			_, err := u.table.UnarchiveTableEntry(ctx, connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: c, Key: key}))
			return err
		}
	}
	switches := func(u *user, sw ...*rulesv1.OptionSwitch) func() error {
		return func() error {
			_, err := u.table.SetOptionSwitches(ctx, connect.NewRequest(&rulesv1.SetOptionSwitchesRequest{CampaignId: c, Switches: sw}))
			return err
		}
	}
	importOf := func(u *user, r *rulesv1.ImportTableContentRequest) func() error {
		return func() error {
			r.CampaignId = c
			_, err := u.table.ImportTableContent(ctx, connect.NewRequest(r))
			return err
		}
	}
	bigRace := testRace("Raça enorme")
	for range rules.MaxTraits + 1 {
		bigRace.Traits = append(bigRace.Traits, &rulesv1.TableFeature{NamePt: "x"})
	}
	foreignKey := testRace("Outra raça")
	foreignKey.Traits[0].Key = "trait:inventada@mesa"
	dupWithKey := testRace("Anão das Brumas")
	dupWithKey.Traits[0].Key = "trait:inventada@mesa"
	movedSub := testSubrace("Da Colina", "race:dwarf")
	movedSubclass := testSubclass("Caminho do Vento", "class:wizard")
	leveled := testSpell("Truque", "class:wizard")
	var manySwitches []*rulesv1.OptionSwitch
	for i := range MaxOptionSwitches + 1 {
		manySwitches = append(manySwitches, optionSwitchOf("spell:x"+strings.Repeat("y", i), true))
	}
	goodPack := &rulesv1.TableContentPack{Format: PackFormat, Version: PackVersion, Entries: []*rulesv1.TableEntry{frozenPackEntry("um")}}
	preview := rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW

	calls := []tableRefusalCall{
		{"create as a player", create(v.dona, testBackground("Intruso"))},
		{"create as a pending member", create(v.pending, testBackground("Intruso"))},
		{"create as an outsider", create(v.outsider, testBackground("Intruso"))},
		{"create as nobody", create(nobody, testBackground("Intruso"))},
		{"create without a body", createReqOf(&rulesv1.CreateTableEntryRequest{CampaignId: c})},
		{"create with too many features", create(m, bigRace)},
		{"create with a bad idempotency key", createReqOf(func() *rulesv1.CreateTableEntryRequest {
			r := createReq(c, testBackground("Chave"))
			r.IdempotencyKey = strings.Repeat("k", 65)
			return r
		}())},
		{"create with a duplicate name", create(m, testRace("anão das brumas"))},
		{"create with a duplicate name and a made-up feature key", create(m, dupWithKey)},
		{"create with a made-up feature key", create(m, foreignKey)},
		{"create a subrace of an unknown race", create(m, testSubrace("Órfã", "race:nada@mesa"))},
		{"create with a duplicate name the rules also refuse", create(m, testSubrace("da colina", "race:nada@mesa"))},

		{"update as a player", update(v.dona, race, revOf(race), testRace("Anão das Brumas"))},
		{"update as an outsider", update(v.outsider, race, revOf(race), testRace("Anão das Brumas"))},
		{"update an SRD key", update(m, "race:gnome", 1, testRace("Gnomo"))},
		{"update without a body", func() error {
			_, err := m.table.UpdateTableEntry(ctx, connect.NewRequest(&rulesv1.UpdateTableEntryRequest{CampaignId: c, Key: race, ExpectedRevision: revOf(race)}))
			return err
		}},
		{"update with another kind", update(m, race, revOf(race), testBackground("Anão das Brumas"))},
		{"update with too many features", update(m, race, revOf(race), bigRace)},
		{"update a key the table does not have", update(m, "race:nada@mesa", 1, testRace("Nada"))},
		{"update with a stale revision", update(m, race, revOf(race)+7, testRace("Anão das Brumas"))},
		{"update a cantrip to a leveled spell", update(m, trick, revOf(trick), leveled)},
		{"update a subrace's race", update(m, sub, revOf(sub), movedSub)},
		{"update a subclass's class", update(m, subclass, revOf(subclass), movedSubclass)},
		{"update with a duplicate name", update(m, elf, revOf(elf), testRace("ANÃO DAS BRUMAS"))},
		{"update to the name of an entry of another kind", update(m, sub, revOf(sub), testSubrace("Guarda de FAROL", race))},
		{"update with a made-up feature key", update(m, race, revOf(race), foreignKey)},

		{"archive as a player", archive(v.dona, race, true)},
		{"unarchive as a player", archive(v.dona, race, false)},
		{"archive an SRD key", archive(m, "race:gnome", true)},
		{"unarchive an SRD key", archive(m, "race:gnome", false)},
		{"archive a key the table does not have", archive(m, "race:nada@mesa", true)},
		{"unarchive a key the table does not have", archive(m, "race:nada@mesa", false)},
		{"unarchive an entry that is not archived", archive(m, race, false)},
		{"archive the race", archive(m, race, true)},
		{"archive the race again", archive(m, race, true)},
		{"archive the background", archive(m, background, true)},
		// Stored with a stray field, as an older version could have left it: bringing it back
		// checks the campaign's overlay again, strictly for the entry.
		{"unarchive an entry stored with a stray field", func() error {
			if _, err := v.h.pool.Exec(ctx, `UPDATE campaign_content SET data = jsonb_set(data, '{feature,effects,0,recharge}', '"long_rest"') WHERE campaign_id = $1 AND content_key = $2`, c, background); err != nil {
				t.Fatalf("store the stray field: %v", err)
			}
			return archive(m, background, false)()
		}},

		{"switches as a player", switches(v.dona, optionSwitchOf("class:druid", true))},
		{"switches as a pending member", switches(v.pending, optionSwitchOf("class:druid", true))},
		{"switches as an outsider", switches(v.outsider, optionSwitchOf("class:druid", true))},
		{"no switches", switches(m)},
		{"too many switches", switches(m, manySwitches...)},
		{"a switch twice", switches(m, optionSwitchOf("class:druid", true), optionSwitchOf("spell:fire-bolt", true), optionSwitchOf("class:druid", false))},
		{"a twice key before an unknown one", switches(m, optionSwitchOf("nada", true), optionSwitchOf("nada", false))},
		{"a key that is not an option", switches(m, optionSwitchOf("class:druid", true), optionSwitchOf("equipment:longsword", true))},
		{"a feature key", switches(m, optionSwitchOf("feature:fighter-fighting-style-defense", true))},

		{"import as a player", importOf(v.dona, &rulesv1.ImportTableContentRequest{Mode: preview, Pack: goodPack})},
		{"import without a mode", importOf(m, &rulesv1.ImportTableContentRequest{Pack: goodPack})},
		{"import with pack and pack_json", importOf(m, &rulesv1.ImportTableContentRequest{Mode: preview, Pack: goodPack, PackJson: []byte(`{}`)})},
		{"import a file with an unknown pack field", importOf(m, &rulesv1.ImportTableContentRequest{Mode: preview, PackJson: []byte(`{"format":"meurpg.table-content","version":1,"nome":"x"}`)})},
		{"import a file that is not a pack", importOf(m, &rulesv1.ImportTableContentRequest{Mode: preview, PackJson: []byte(`[]`)})},
		{"import without a pack", importOf(m, &rulesv1.ImportTableContentRequest{Mode: preview})},
		{"import with a bad idempotency key", importOf(m, &rulesv1.ImportTableContentRequest{Mode: preview, Pack: goodPack, IdempotencyKey: strings.Repeat("k", 65)})},
		{"import a pack of another format", importOf(m, &rulesv1.ImportTableContentRequest{Mode: preview, Pack: &rulesv1.TableContentPack{Format: "x", Version: PackVersion}})},
		{"import a file whose entry has an unknown field", importOf(m, &rulesv1.ImportTableContentRequest{
			Mode:     rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY,
			PackJson: []byte(`{"format":"meurpg.table-content","version":1,"entries":[{"key":"feat:um@mesa","kind":"TABLE_CONTENT_KIND_FEAT","tableFeat":{"namePt":"Um","bogus":1}}]}`),
		})},
	}
	for _, call := range calls {
		v.steps = append(v.steps, tableContentGoldenStep{call.label, tableRefusalShape(call.call())})
	}
	if got := len(m.entries(t, c)); got != len(entries) {
		t.Errorf("the table has %d entries after the refusals, want %d", got, len(entries))
	}
	tableContentGolden(t, "refusals", v.names(), v.steps...)
}

// TestCreateTableEntryWithAKeyIsFrozen: rule 9 for a create. The same key and request answer
// the first entry again, at the table's revision, with no write and no hint; the same key with
// another request is refused; the key is the campaign's own.
func TestCreateTableEntryWithAKeyIsFrozen(t *testing.T) {
	t.Parallel()
	v := newViewersTable(t)
	other := v.h.newCampaign(v.master, "Outra")
	create := func(campaign, key string, body proto.Message) (*rulesv1.CreateTableEntryResponse, error) {
		r := createReq(campaign, body)
		r.IdempotencyKey = key
		res, err := v.master.table.CreateTableEntry(t.Context(), connect.NewRequest(r))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	first, err := create(v.campaign, "k-1", testBackground("Guarda de farol"))
	if err != nil {
		t.Fatalf("CreateTableEntry(k-1) error = %v", err)
	}
	v.add(t, "later", testBackground("Outro"))
	again, err := create(v.campaign, "k-1", testBackground("Guarda de farol"))
	if err != nil {
		t.Fatalf("CreateTableEntry(k-1 again) error = %v", err)
	}
	if !proto.Equal(again.GetEntry(), first.GetEntry()) {
		t.Errorf("the replay's entry = %v, want the first one %v", again.GetEntry(), first.GetEntry())
	}
	if again.GetTableRevision() != 2 || v.h.contentRevision(v.campaign) != 2 {
		t.Errorf("the replay's table revision = %d, the campaign's %d; want 2 and 2 (the replay writes nothing)", again.GetTableRevision(), v.h.contentRevision(v.campaign))
	}
	_, err = create(v.campaign, "k-1", testBackground("Guarda de outro farol"))
	v.steps = append(v.steps,
		tableContentGoldenStep{"first", first},
		tableContentGoldenStep{"the same request again", again},
		tableContentGoldenStep{"the same key with another request", tableRefusalShape(err)},
	)
	elsewhere, err := create(other, "k-1", testBackground("Guarda de farol"))
	if err != nil {
		t.Fatalf("CreateTableEntry(k-1 in another campaign) error = %v", err)
	}
	v.steps = append(v.steps, tableContentGoldenStep{"the same key in another campaign", elsewhere})
	if n := v.hintsOf(); n != 2 {
		t.Errorf("content_changed hints = %d, want 2: the first create and the later one", n)
	}
	names := v.names()
	names[other] = "<other campaign>"
	tableContentGolden(t, "create_with_key", names, v.steps...)
}
