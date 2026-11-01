package characters

import (
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A table creature (issue #578): an SRD wolf with 40 hit points, armor class 17, +2 damage and one
// trait, "Lobo da Mesa". These tests need the database (MEURPG_TEST_DATABASE_URL).

func tableWolf() *rulesv1.TableCreature {
	return &rulesv1.TableCreature{
		NamePt: "Lobo da Mesa", BaseKey: "monster:wolf", HitPoints: 40, ArmorClass: 17, DamageBonus: 2,
		Traits: []*rulesv1.TableCreatureTrait{{NamePt: "Pelo de Prata", TextPt: "Brilha ao luar."}},
	}
}

const tableWolfKey = "creature:lobo-da-mesa@mesa"

// bigCreature is a creature whose entry is over 64 KiB as JSON: ten traits of 4000 characters of two bytes each.
func bigCreature() *rulesv1.TableCreature {
	c := tableWolf()
	c.Traits = nil
	for i := range 10 {
		c.Traits = append(c.Traits, &rulesv1.TableCreatureTrait{
			NamePt: "Traço " + string(rune('A'+i)), TextPt: strings.Repeat("ã", 4000),
		})
	}
	return c
}

func TestMR578_ATableCreatureHasItsLifeCycleAndItsRefusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)

	created := master.addEntry(t, campaign, tableWolf())
	if created.GetKey() != tableWolfKey || created.GetKind() != rulesv1.TableContentKind_TABLE_CONTENT_KIND_CREATURE {
		t.Fatalf("created = %q kind %v, want %q of the creature kind", created.GetKey(), created.GetKind(), tableWolfKey)
	}
	// The creature is a creature of the campaign's content, with the adjustments; the SRD's wolf is not touched.
	got, err := master.content.GetCreature(t.Context(), connect.NewRequest(&rulesv1.GetCreatureRequest{CampaignId: campaign, Key: tableWolfKey}))
	if err != nil || got.Msg.GetCreature().GetHitPoints() != 40 || got.Msg.GetCreature().GetArmorClass() != 17 {
		t.Fatalf("GetCreature(table creature) = %v, %v; want 40 hit points and armor class 17", got, err)
	}
	base, err := player.content.GetCreature(t.Context(), connect.NewRequest(&rulesv1.GetCreatureRequest{CampaignId: campaign, Key: "monster:wolf"}))
	if err != nil || base.Msg.GetCreature().GetHitPoints() != 11 {
		t.Fatalf("GetCreature(wolf) = %v, %v; want the SRD's 11 hit points", base, err)
	}

	// The base never changes.
	next := tableWolf()
	next.BaseKey = "monster:dire-wolf"
	_, err = master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, created.GetKey(), created.GetRevision(), next)))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetField() != "table_creature.base_key" || v[0].GetReason() != reasonImmutable {
		t.Errorf("changing the base: violations = %v, want immutable at table_creature.base_key", v)
	}

	// A player holding none never reads it, live or archived (the holder's reads are in the test of the holders);
	// the master does, archived or not.
	if player.entries(t, campaign)[tableWolfKey] != nil {
		t.Error("a player who holds none reads a live creature")
	}
	if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: tableWolfKey})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	if player.entries(t, campaign)[tableWolfKey] != nil {
		t.Error("a player reads an archived creature")
	}
	if e := master.entries(t, campaign)[tableWolfKey]; e == nil || !e.GetArchived() {
		t.Errorf("the master reads %v, want the archived creature", e)
	}
	if _, err := master.table.UnarchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: campaign, Key: tableWolfKey})); err != nil {
		t.Fatalf("UnarchiveTableEntry() error = %v", err)
	}
	if e := master.entries(t, campaign)[tableWolfKey]; e == nil || e.GetArchived() {
		t.Errorf("the master reads %v once the creature is back, want it live", e)
	}

	// A base the SRD does not have, and an entry over 64 KiB, are refused with the field of the creature.
	dangling := tableWolf()
	dangling.NamePt, dangling.BaseKey = "Sem Base", "monster:nao-existe"
	_, err = master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, dangling)))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetField() != "table_creature.base_key" || v[0].GetReason() != rules.ReasonReference {
		t.Errorf("a base that does not exist: violations = %v, want a dangling reference at table_creature.base_key", v)
	}
	big := bigCreature()
	big.NamePt = "Gigante"
	_, err = master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, big)))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetReason() != "size_limit" || v[0].GetField() != "table_creature" {
		t.Errorf("a creature over 64 KiB: violations = %v, want size_limit", v)
	}
	bad := tableWolf()
	bad.NamePt, bad.HitPoints = "Fraco", 10000
	_, err = master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, bad)))
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetField() != "table_creature.hit_points" {
		t.Errorf("hit points over the limit: violations = %v, want table_creature.hit_points", v)
	}
}

func TestMR578_ATableCreatureHasItsOwnSwitchForThePlayers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	master.addEntry(t, campaign, tableWolf())

	e := master.options(t, campaign)[tableWolfKey]
	if e == nil || e.GetKind() != rulesv1.TableContentKind_TABLE_CONTENT_KIND_CREATURE || !e.GetTable() || e.GetOff() || e.GetNamePt() != "Lobo da Mesa" {
		t.Fatalf("switch entry = %v, want the table creature, on", e)
	}
	master.setOff(t, campaign, true, tableWolfKey)
	if e := master.options(t, campaign)[tableWolfKey]; e == nil || !e.GetOff() || !e.GetHidden() {
		t.Errorf("switch entry after switching off = %v, want off and hidden", e)
	}
	if e := master.entries(t, campaign)[tableWolfKey]; e == nil || !e.GetOff() {
		t.Errorf("the master reads %v, want the creature marked off", e)
	}
	master.setOff(t, campaign, false, tableWolfKey)
	if e := master.options(t, campaign)[tableWolfKey]; e == nil || e.GetOff() {
		t.Errorf("switch entry after switching on = %v, want on", e)
	}
}

func TestMR578_ATableCreatureTravelsInAPack(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	made := pt.master.addEntry(t, pt.from, tableWolf())
	pack := pt.export(t, pt.from)
	if len(pack.GetEntries()) != 1 || pack.GetEntries()[0].GetKind() != rulesv1.TableContentKind_TABLE_CONTENT_KIND_CREATURE {
		t.Fatalf("pack entries = %v, want the creature", pack.GetEntries())
	}
	file, err := storeJSON.Marshal(pack)
	if err != nil || !strings.Contains(string(file), `"table_creature"`) || !strings.Contains(string(file), `"base_key"`) {
		t.Fatalf("the pack file = %s, %v; want proto JSON with the creature", file, err)
	}
	loaded := &rulesv1.TableContentPack{}
	if err := loadJSON.Unmarshal(file, loaded); err != nil {
		t.Fatal(err)
	}
	before := pt.revisionOf(t, pt.to)
	if pv := pt.preview(t, pt.to, loaded); pv.GetTotals().GetNew() != 1 || pt.revisionOf(t, pt.to) != before {
		t.Fatalf("preview totals = %v, want one new and nothing written", pv.GetTotals())
	}
	pt.apply(t, pt.to, loaded, "creature-1")
	in := pt.entries(t, pt.to)[tableWolfKey]
	if in == nil || !proto.Equal(bodyMessage(in), bodyMessage(made)) {
		t.Fatalf("the imported creature = %v, want the exported one", in)
	}
	if again := pt.apply(t, pt.to, loaded, "creature-2"); again.GetTotals().GetUnchanged() != 1 {
		t.Errorf("importing again: %v, want one unchanged", again.GetTotals())
	}

	// A base the SRD does not have is refused, in the preview and in the apply, as any dangling reference.
	broken := proto.CloneOf(loaded)
	bodyMessage(broken.GetEntries()[0]).(*rulesv1.TableCreature).BaseKey = "monster:nao-existe"
	pv := pt.preview(t, pt.from, broken)
	e := pv.GetEntries()[0]
	if e.GetStatus() != rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED || len(e.GetViolations()) != 1 ||
		e.GetViolations()[0].GetField() != "table_creature.base_key" || e.GetViolations()[0].GetReason() != rules.ReasonReference {
		t.Errorf("a dangling base in the preview: %v, want refused with a reference at table_creature.base_key", e)
	}
	_, err = pt.importPack(pt.master, pt.from, broken, rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY, "creature-3")
	if v := violationsOfErr(t, err); len(v) != 1 || v[0].GetField() != "table_creature.base_key" {
		t.Errorf("a dangling base in the apply: violations = %v", v)
	}

	// An entry over 64 KiB is refused with the size limit.
	huge := proto.CloneOf(loaded)
	huge.Entries[0].Key, huge.Entries[0].NamePt = "creature:gigante@mesa", "Gigante"
	big := bigCreature()
	big.NamePt = "Gigante"
	huge.Entries[0].Body = &rulesv1.TableEntry_TableCreature{TableCreature: big}
	pv = pt.preview(t, pt.from, huge)
	if e := pv.GetEntries()[0]; e.GetStatus() != rulesv1.TableImportStatus_TABLE_IMPORT_STATUS_REFUSED || e.GetViolations()[0].GetReason() != "size_limit" {
		t.Errorf("a creature over 64 KiB in the preview: %v, want size_limit", e)
	}
}

func TestMR578_ATableCreatureGivenToACharacterHasTheAdjustedNumbers(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	tb.master.addEntry(t, tb.campaign, tableWolf())

	given, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: tableWolfKey, Name: "Presa de Prata"})
	if err != nil {
		t.Fatalf("GiveCreature(table creature) error = %v", err)
	}
	if given.GetMonsterKey() != tableWolfKey || given.GetMonsterNamePt() != "Lobo da Mesa" || given.GetHitPointsMax() != 40 || given.GetHitPointsCurrent() != 40 || given.GetArmorClass() != 17 {
		t.Fatalf("given = %v, want the table wolf with 40 hit points and armor class 17", given)
	}
	// A creature the content does not have is still refused.
	if _, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: "creature:nada@mesa"}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("GiveCreature(no such table creature) error = %v, want invalid_argument", err)
	}

	list := func(u *user) (*charactersv1.ListCharacterCreaturesResponse, error) {
		res, err := u.api.ListCharacterCreatures(t.Context(), connect.NewRequest(&charactersv1.ListCharacterCreaturesRequest{CampaignId: tb.campaign, CharacterId: tb.pc.GetId()}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	for name, u := range map[string]*user{"master": tb.master, "owner": tb.owner} {
		res, err := list(u)
		if err != nil || len(res.GetCreatures()) != 1 {
			t.Fatalf("%s: ListCharacterCreatures() = %v, %v; want the creature", name, res, err)
		}
		if c := res.GetCreatures()[0]; c.GetArmorClass() != 17 || c.GetHitPointsMax() != 40 || c.GetMonsterNamePt() != "Lobo da Mesa" {
			t.Errorf("%s: creature = %v, want armor class 17 and 40 hit points", name, c)
		}
	}
	if _, err := list(tb.other); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another player: ListCharacterCreatures() error = %v, want not_found", err)
	}

	// A creature of the SRD shows the SRD's armor class.
	wolf := tb.wolf(t, "Comum")
	if wolf.GetArmorClass() != 13 {
		t.Errorf("the SRD wolf's armor class = %d, want 13", wolf.GetArmorClass())
	}

	// The entry is edited and archived: the creature already given keeps the hit points it was given, and
	// an archived entry still resolves, so the creature stays whole.
	table := tb.master.entries(t, tb.campaign)[tableWolfKey]
	next := tableWolf()
	next.HitPoints, next.ArmorClass = 55, 19
	if _, err := tb.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(tb.campaign, tableWolfKey, table.GetRevision(), next))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	if _, err := tb.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: tableWolfKey})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	res, err := list(tb.owner)
	if err != nil || len(res.GetCreatures()) != 2 {
		t.Fatalf("after the edit: ListCharacterCreatures() = %v, %v; want both creatures", res, err)
	}
	for _, c := range res.GetCreatures() {
		if c.GetMonsterKey() != tableWolfKey {
			continue
		}
		if c.GetHitPointsMax() != 40 || c.GetArmorClass() != 19 {
			t.Errorf("after the edit: creature = hp max %d, armor class %d; want the 40 it was given and the entry's new 19", c.GetHitPointsMax(), c.GetArmorClass())
		}
	}
}

func TestMR578_ATableCreatureIsReadOnlyByTheMasterAndByWhoHoldsIt(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	tb.master.addEntry(t, tb.campaign, tableWolf())
	inBestiary := func(u *user) bool {
		res, err := u.content.ListCreatures(t.Context(), connect.NewRequest(&rulesv1.ListCreaturesRequest{CampaignId: tb.campaign, Query: "Lobo da Mesa", PageSize: 400}))
		if err != nil {
			t.Fatalf("ListCreatures() error = %v", err)
		}
		for _, c := range res.Msg.GetCreatures() {
			if c.GetKey() == tableWolfKey {
				return true
			}
		}
		return false
	}
	get := func(u *user) error {
		_, err := u.content.GetCreature(t.Context(), connect.NewRequest(&rulesv1.GetCreatureRequest{CampaignId: tb.campaign, Key: tableWolfKey}))
		return err
	}
	// reads is what each of the three reads of the creature answers: the list, the sheet and the table's entries.
	reads := func(u *user) (listed bool, got error, entry bool) {
		return inBestiary(u), get(u), u.entries(t, tb.campaign)[tableWolfKey] != nil
	}
	check := func(when string, u *user, who string, want bool) {
		t.Helper()
		listed, got, entry := reads(u)
		if listed != want || (got == nil) != want || entry != want {
			t.Errorf("%s, %s: listed = %v, GetCreature error = %v, in the table's entries = %v; want all %v", when, who, listed, got, entry, want)
		}
		if !want && connect.CodeOf(got) != connect.CodeNotFound {
			t.Errorf("%s, %s: GetCreature error = %v, want not_found", when, who, got)
		}
	}
	// A live creature nobody holds is the master's alone: a player does not browse the table's creatures.
	check("live, held by no one", tb.master, "the master", true)
	check("live, held by no one", tb.owner, "the player", false)
	check("live, held by no one", tb.other, "another player", false)

	given, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: tableWolfKey})
	if err != nil || given.GetHitPointsMax() != 40 {
		t.Fatalf("GiveCreature() = %v, %v; want the table wolf", given, err)
	}
	check("live, held", tb.master, "the master", true)
	check("live, held", tb.owner, "the holder", true)
	check("live, held", tb.other, "a player who holds none", false)

	if _, err := tb.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: tableWolfKey})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	check("archived, held", tb.master, "the master", true)
	check("archived, held", tb.owner, "the holder", true)
	check("archived, held", tb.other, "a player who holds none", false)

	// Dismissed, the creature is no longer held: it stops resolving for its player.
	if err := tb.dismiss(tb.owner, given.GetId()); err != nil {
		t.Fatalf("dismiss error = %v", err)
	}
	check("archived, dismissed", tb.master, "the master", true)
	check("archived, dismissed", tb.owner, "the former holder", false)
}

func TestMR578_TheMasterCanGiveAnArchivedTableCreature(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	tb.master.addEntry(t, tb.campaign, tableWolf())
	if _, err := tb.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: tableWolfKey})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	given, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: tb.pc.GetId(), MonsterKey: tableWolfKey})
	if err != nil || given.GetMonsterKey() != tableWolfKey || given.GetHitPointsMax() != 40 {
		t.Fatalf("GiveCreature(archived table creature) = %v, %v; want the table wolf with 40 hit points", given, err)
	}
}

// MonsterNpc keys the NPC of a table creature by the numbers of its sheet: an edit that changes them makes
// another NPC, one that changes them back finds the first, and a rename changes none.
func TestMR578_TheNpcOfATableCreatureFollowsItsNumbers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	entry := master.addEntry(t, campaign, tableWolf())
	revision := entry.GetRevision()
	edit := func(c *rulesv1.TableCreature) {
		t.Helper()
		res, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, tableWolfKey, revision, c)))
		if err != nil {
			t.Fatalf("UpdateTableEntry() error = %v", err)
		}
		revision = res.Msg.GetEntry().GetRevision()
	}
	npc := func() string {
		t.Helper()
		var id string
		err := db.InTx(t.Context(), h.pool, func(tx pgx.Tx) error {
			got, ok, err := h.svc.MonsterNpc(t.Context(), tx, campaign, master.id, tableWolfKey, h.clock.Now())
			id = got.ID
			if err == nil && !ok {
				err = errors.New("no NPC for the table creature")
			}
			return err
		})
		if err != nil {
			t.Fatalf("MonsterNpc() error = %v", err)
		}
		return id
	}
	rows := func() int {
		t.Helper()
		var n int
		if err := h.pool.QueryRow(t.Context(), "SELECT count(*) FROM characters WHERE campaign_id = $1", campaign).Scan(&n); err != nil {
			t.Fatalf("count the NPCs: %v", err)
		}
		return n
	}

	first := npc()
	if again := npc(); again != first || rows() != 1 {
		t.Fatalf("the same entry again = %s with %d rows, want %s and 1", again, rows(), first)
	}

	stronger := tableWolf()
	stronger.HitPoints = 55
	edit(stronger)
	second := npc()
	if second == first || rows() != 2 {
		t.Fatalf("an edit of the hit points = %s with %d rows, want another NPC and 2 rows", second, rows())
	}

	renamed := tableWolf()
	renamed.HitPoints, renamed.NamePt = 55, "Lobo de Prata"
	edit(renamed)
	if got := npc(); got != second || rows() != 2 {
		t.Fatalf("a rename = %s with %d rows, want the same NPC %s and 2 rows", got, rows(), second)
	}

	edit(tableWolf()) // back to the earlier numbers
	if got := npc(); got != first || rows() != 2 {
		t.Fatalf("the numbers back = %s with %d rows, want the first NPC %s and 2 rows", got, rows(), first)
	}

	// The damage bonus is part of the numbers: an edit of it makes another NPC.
	sharper := tableWolf()
	sharper.DamageBonus = 5
	edit(sharper)
	third := npc()
	if third == first || third == second || rows() != 3 {
		t.Fatalf("an edit of the damage bonus = %s with %d rows, want a third NPC and 3 rows", third, rows())
	}

	// A trait only adds text: it finds the NPC it had.
	withTrait := tableWolf()
	withTrait.DamageBonus = 5
	withTrait.Traits = append(withTrait.Traits, &rulesv1.TableCreatureTrait{NamePt: "Uivo", TextPt: "Ecoa na floresta."})
	edit(withTrait)
	if got := npc(); got != third || rows() != 3 {
		t.Fatalf("a trait added = %s with %d rows, want the same NPC %s and 3 rows", got, rows(), third)
	}
}
