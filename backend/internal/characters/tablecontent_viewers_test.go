package characters

import (
	"fmt"
	"slices"
	"strconv"
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
// shape of every refusal of the writes, each for who may call it. The viewers' answers are
// stated inline; the refusals and the create with a key are golden files
// (testdata/table_content_*.golden.json, ids replaced by names).

// viewersTable is the campaign of the viewers' test: a master, Dona with a sheet that uses
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

// switches calls SetOptionSwitches as the master with each key's off and says what it
// answered (see optionsSummary).
func (v *viewersTable) switches(t *testing.T, switches ...*rulesv1.OptionSwitch) string {
	t.Helper()
	res, err := v.master.table.SetOptionSwitches(t.Context(), connect.NewRequest(&rulesv1.SetOptionSwitchesRequest{CampaignId: v.campaign, Switches: switches}))
	if err != nil {
		t.Fatalf("SetOptionSwitches() error = %v", err)
	}
	return optionsSummary(res.Msg)
}

// optionsSummary is "changed=N rev=R:" and each option that answered, with its flags.
func optionsSummary(res *rulesv1.SetOptionSwitchesResponse) string {
	parts := make([]string, 0, len(res.GetOptions()))
	for _, o := range res.GetOptions() {
		var flags []string
		for _, f := range []struct {
			on   bool
			name string
		}{{o.GetOff(), "off"}, {o.GetHidden(), "hidden"}, {o.GetArchived(), "archived"}, {o.GetTable(), "table"}} {
			if f.on {
				flags = append(flags, f.name)
			}
		}
		if n := o.GetCharactersUsing(); n > 0 {
			flags = append(flags, fmt.Sprintf("users=%d", n))
		}
		parts = append(parts, o.GetKey()+"("+strings.Join(flags, ",")+")")
	}
	return fmt.Sprintf("changed=%d rev=%d: %s", res.GetChanged(), res.GetTableRevision(), strings.Join(parts, " "))
}

// archive retires an entry as the master and says what it answered.
func (v *viewersTable) archive(t *testing.T, name string) string {
	t.Helper()
	res, err := v.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: v.campaign, Key: v.keys[name]}))
	if err != nil {
		t.Fatalf("ArchiveTableEntry(%s) error = %v", name, err)
	}
	return fmt.Sprintf("%s archived=%v rev=%d", res.Msg.GetEntry().GetKey(), res.Msg.GetEntry().GetArchived(), res.Msg.GetTableRevision())
}

// list reads the table's entries as u.
func (v *viewersTable) list(t *testing.T, u *user) (*rulesv1.ListTableEntriesResponse, error) {
	t.Helper()
	res, err := u.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: v.campaign}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// mustList is list for a viewer that may read.
func (v *viewersTable) mustList(t *testing.T, u *user) *rulesv1.ListTableEntriesResponse {
	t.Helper()
	res, err := v.list(t, u)
	if err != nil {
		t.Fatalf("ListTableEntries() error = %v", err)
	}
	return res
}

// viewerDiffs says, for each of the master's entries, how a viewer's copy differs: absent,
// or the fields moved (path=value for a changed field, -path for a removed one). An entry
// that reads the same is not listed.
func viewerDiffs(t *testing.T, master, viewer []*rulesv1.TableEntry) []string {
	t.Helper()
	byKey := map[string]*rulesv1.TableEntry{}
	for _, e := range viewer {
		byKey[e.GetKey()] = e
	}
	var out []string
	for _, e := range master {
		seen, ok := byKey[e.GetKey()]
		if !ok {
			out = append(out, e.GetKey()+": absent")
			continue
		}
		if moved := fieldsMoved(protoFlat(t, e), protoFlat(t, seen)); moved != "unchanged" {
			out = append(out, e.GetKey()+": "+moved)
		}
	}
	return out
}

// entryLines lists the entries with the master's marks: (off) for a switched-off one,
// (archived) for a retired one.
func entryLines(entries []*rulesv1.TableEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		var marks []string
		if e.GetOff() {
			marks = append(marks, "off")
		}
		if e.GetArchived() {
			marks = append(marks, "archived")
		}
		line := e.GetKey()
		if len(marks) > 0 {
			line += "(" + strings.Join(marks, ",") + ")"
		}
		out = append(out, line)
	}
	return out
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

// TestTableEntriesOfEveryViewer: what the master, a player whose sheet uses hidden entries,
// a player without a sheet, a pending member, an outsider and nobody read of the table's
// entries, before and after the master switches options off and archives some, and what each
// switch answers (only the options whose state changed for the players, the characters that
// use them, no change when nothing changes). What the master hid or switched off never
// reaches a player (RN-10, RN-23): a hidden entry is absent from their list, and an entry
// they keep has no reference to a hidden SRD key, no hidden race requirement and no count of
// the characters that use it.
func TestTableEntriesOfEveryViewer(t *testing.T) {
	t.Parallel()
	v := newViewersTable(t)
	v.fill(t)
	master, dona := v.mustList(t, v.master), v.mustList(t, v.dona)
	if n := len(master.GetEntries()); n != 12 || master.GetTableRevision() != 12 {
		t.Errorf("master reads %d entries at revision %d, want 12 at 12", n, master.GetTableRevision())
	}
	// Nothing is hidden yet: Dona reads what the master reads, without who uses it.
	wantDiffs(t, "dona before", viewerDiffs(t, master.GetEntries(), dona.GetEntries()), []string{
		`race:anao-das-brumas@mesa: -charactersUsing`,
		`subrace:da-colina@mesa: -charactersUsing`,
		`background:guarda-de-farol@mesa: -charactersUsing`,
		`spell:raio-de-teste@mesa: -charactersUsing`,
	})
	hintsBefore := v.hintsOf()

	wantString(t, "switches: off the druid, cure wounds, the race and a feat", v.switches(t,
		optionSwitchOf("class:druid", true), optionSwitchOf("spell:cure-wounds", true), optionSwitchOf(v.keys["race"], true), optionSwitchOf(v.keys["off feat"], true)),
		`changed=4 rev=13: class:druid(off,hidden) subclass:land(hidden) race:anao-das-brumas@mesa(off,hidden,table,users=1) subrace:da-colina@mesa(hidden,table,users=1) spell:cure-wounds(off,hidden) feat:talento-desligado@mesa(off,hidden,table)`)
	wantString(t, "switches: the same again changes nothing", v.switches(t, optionSwitchOf("class:druid", true), optionSwitchOf("spell:fire-bolt", false)),
		`changed=0 rev=13: `)
	wantString(t, "switches: on and off in one call", v.switches(t, optionSwitchOf("class:druid", false), optionSwitchOf("spell:magic-missile", true)),
		`changed=2 rev=14: class:druid() subclass:land() spell:magic-missile(off,hidden,users=1)`)
	wantString(t, "switches: the druid off again, magic missile on", v.switches(t, optionSwitchOf("class:druid", true), optionSwitchOf("spell:magic-missile", false)),
		`changed=2 rev=15: class:druid(off,hidden) subclass:land(hidden) spell:magic-missile(users=1)`)
	wantString(t, "archive secret race", v.archive(t, "secret race"), `race:raca-secreta@mesa archived=true rev=16`)
	wantString(t, "archive archived spell", v.archive(t, "archived spell"), `spell:raio-arquivado@mesa archived=true rev=17`)
	wantString(t, "switches: an archived entry switched off", v.switches(t, optionSwitchOf(v.keys["archived spell"], true)),
		`changed=1 rev=18: spell:raio-arquivado@mesa(off,hidden,archived,table)`)

	master, dona = v.mustList(t, v.master), v.mustList(t, v.dona)
	davi := v.mustList(t, v.davi)
	// The master reads every entry, marked when it is off or archived.
	wantDiffs(t, "master", entryLines(master.GetEntries()), []string{
		`class:guardiao-do-vale@mesa`,
		`subclass:caminho-do-vale@mesa`,
		`subclass:lamina-de-tinta@mesa`,
		`race:anao-das-brumas@mesa(off)`,
		`race:raca-secreta@mesa(archived)`,
		`subrace:da-colina@mesa`,
		`subrace:sub-raca-secreta@mesa`,
		`background:guarda-de-farol@mesa`,
		`spell:raio-arquivado@mesa(off,archived)`,
		`spell:raio-de-teste@mesa`,
		`feat:punho-de-ferro@mesa`,
		`feat:talento-desligado@mesa(off)`,
	})
	// Dona's sheet uses the race and the subrace, so she still reads the subrace; Davi, with no
	// sheet, does not.
	wantDiffs(t, "dona, whose sheet uses the race and the subrace", viewerDiffs(t, master.GetEntries(), dona.GetEntries()), []string{
		`class:guardiao-do-vale@mesa: -tableClass.casting.listFrom -tableClass.levels[0].features[1].effects[0].spells[1] tableClass.levels[0].features[1].effects[0].spells[0]="spell:fire-bolt"`,
		`subclass:caminho-do-vale@mesa: -tableSubclass.alwaysPrepared[1].classLevel -tableSubclass.alwaysPrepared[1].spellKey tableSubclass.alwaysPrepared[0].classLevel=5 tableSubclass.alwaysPrepared[0].spellKey="spell:lesser-restoration"`,
		`subclass:lamina-de-tinta@mesa: -tableSubclass.casting.listFrom`,
		`race:anao-das-brumas@mesa: absent`,
		`race:raca-secreta@mesa: absent`,
		`subrace:da-colina@mesa: -charactersUsing`,
		`subrace:sub-raca-secreta@mesa: absent`,
		`background:guarda-de-farol@mesa: -charactersUsing -tableBackground.feature.effects[0].spells[1] tableBackground.feature.effects[0].spells[0]="spell:fire-bolt"`,
		`spell:raio-arquivado@mesa: absent`,
		`spell:raio-de-teste@mesa: -charactersUsing -tableSpell.classKeys[2] tableSpell.classKeys[1]="class:guardiao-do-vale@mesa"`,
		`feat:punho-de-ferro@mesa: -tableFeat.prerequisite.raceKey`,
		`feat:talento-desligado@mesa: absent`,
	})
	wantDiffs(t, "davi, without a sheet", viewerDiffs(t, master.GetEntries(), davi.GetEntries()), []string{
		`class:guardiao-do-vale@mesa: -tableClass.casting.listFrom -tableClass.levels[0].features[1].effects[0].spells[1] tableClass.levels[0].features[1].effects[0].spells[0]="spell:fire-bolt"`,
		`subclass:caminho-do-vale@mesa: -tableSubclass.alwaysPrepared[1].classLevel -tableSubclass.alwaysPrepared[1].spellKey tableSubclass.alwaysPrepared[0].classLevel=5 tableSubclass.alwaysPrepared[0].spellKey="spell:lesser-restoration"`,
		`subclass:lamina-de-tinta@mesa: -tableSubclass.casting.listFrom`,
		`race:anao-das-brumas@mesa: absent`,
		`race:raca-secreta@mesa: absent`,
		`subrace:da-colina@mesa: absent`,
		`subrace:sub-raca-secreta@mesa: absent`,
		`background:guarda-de-farol@mesa: -charactersUsing -tableBackground.feature.effects[0].spells[1] tableBackground.feature.effects[0].spells[0]="spell:fire-bolt"`,
		`spell:raio-arquivado@mesa: absent`,
		`spell:raio-de-teste@mesa: -charactersUsing -tableSpell.classKeys[2] tableSpell.classKeys[1]="class:guardiao-do-vale@mesa"`,
		`feat:punho-de-ferro@mesa: -tableFeat.prerequisite.raceKey`,
		`feat:talento-desligado@mesa: absent`,
	})
	for _, res := range []*rulesv1.ListTableEntriesResponse{master, dona, davi} {
		if res.GetTableRevision() != 18 {
			t.Errorf("table revision read = %d, want 18", res.GetTableRevision())
		}
	}
	for _, viewer := range []struct {
		label string
		u     *user
		want  string
	}{
		{"pending", v.pending, "not_found: campaign not found"},
		{"outsider", v.outsider, "not_found: campaign not found"},
		{"nobody", v.h.anonymous(), "unauthenticated: sign in to continue"},
	} {
		if _, err := v.list(t, viewer.u); tableRefusalShape(err) != viewer.want {
			t.Errorf("%s reads: %s, want %s", viewer.label, tableRefusalShape(err), viewer.want)
		}
	}
	if n := v.hintsOf() - hintsBefore; n != 6 {
		t.Errorf("content_changed hints = %d, want 6: four switches that changed something and two archives", n)
	}
}

// wantString fails when got is not want.
func wantString(t *testing.T, label, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s:\n got  %s\n want %s", label, got, want)
	}
}

// wantDiffs fails when got is not the lines of want.
func wantDiffs(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s:\n got  %s\n want %s", label, strings.Join(got, "\n      "), strings.Join(want, "\n      "))
	}
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
		manySwitches = append(manySwitches, optionSwitchOf("spell:x"+strconv.Itoa(i), true))
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
