package characters

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// refuseStored makes the engine refuse the stored entries (a class with a die it does not
// have, a spell above level 9) and moves the revision, as an engine change would leave them.
func (h *harness) refuseStored(campaign string, keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		set := `jsonb_set(data, '{level}', '12')`
		if strings.HasPrefix(k, "class:") {
			set = `jsonb_set(data, '{hit_die}', '7')`
		}
		if _, err := h.pool.Exec(h.t.Context(), `UPDATE campaign_content SET data = `+set+` WHERE campaign_id = $1 AND content_key = $2`, campaign, k); err != nil {
			h.t.Fatalf("refuse %s: %v", k, err)
		}
	}
	if _, err := h.pool.Exec(h.t.Context(), `UPDATE campaign_content_state SET revision = revision + 1 WHERE campaign_id = $1`, campaign); err != nil {
		h.t.Fatalf("bump the revision: %v", err)
	}
}

// tolerantTable is a table with a class, its subclass, a race and a spell, and a sheet
// that knows the spell.
type tolerantTable struct {
	h                    *harness
	master, owner        *user
	campaign             string
	class, sub, race, sp *rulesv1.TableEntry
	pc                   *charactersv1.Character
}

func newTolerantTable(t *testing.T) *tolerantTable {
	t.Helper()
	h := newHarness(t)
	master, owner := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", owner)
	f := &tolerantTable{h: h, master: master, owner: owner, campaign: campaign}
	f.class = master.addEntry(t, campaign, testClass("Guardião"))
	f.sub = master.addEntry(t, campaign, testSubclass("Caminho", f.class.GetKey()))
	f.race = master.addEntry(t, campaign, testRace("Gnomo de teste"))
	f.sp = master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard"))
	f.pc = owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", pensantusWith(f.sp.GetKey()))
	return f
}

// TestTableSourceReadsWithoutTheEntriesTheRulesRefuse: a stored entry the engine refuses
// is left out with what depends on it; the rest of the campaign reads and plays.
func TestTableSourceReadsWithoutTheEntriesTheRulesRefuse(t *testing.T) {
	t.Parallel()
	f := newTolerantTable(t)
	f.h.refuseStored(f.campaign, f.class.GetKey(), f.sp.GetKey())
	src := f.h.svc.content.(*TableSource)

	c, err := src.ContentFor(t.Context(), nil, f.campaign)
	if err != nil {
		t.Fatalf("ContentFor() error = %v, want the content without the refused entries", err)
	}
	if c.NamePT(f.race.GetKey()) != "Gnomo de teste" {
		t.Errorf("the healthy race is not in the content")
	}
	for _, e := range []*rulesv1.TableEntry{f.class, f.sub, f.sp} {
		if c.NamePT(e.GetKey()) != "" {
			t.Errorf("%s is in the content, want it left out", e.GetKey())
		}
	}
	got := map[string]string{}
	for _, r := range c.Refused() {
		got[r.Key] = r.Reason
	}
	if len(got) != 3 || got[f.class.GetKey()] != rules.ReasonValue || got[f.sub.GetKey()] != rules.ReasonReference || got[f.sp.GetKey()] != rules.ReasonValue {
		t.Errorf("Refused() = %v, want the class and the spell (bad_value) and the subclass (dangling_reference)", c.Refused())
	}
	if again, err := src.ContentFor(t.Context(), nil, f.campaign); err != nil || again != c {
		t.Errorf("a second read = %p, %v; want the cached content %p", again, err, c)
	}
	if listed := f.master.entries(t, f.campaign); listed[f.class.GetKey()] == nil || listed[f.sp.GetKey()] == nil {
		t.Errorf("ListTableEntries lost a refused row: %d entries", len(listed))
	}
	sheet := f.owner.get(t, f.campaign, f.pc.GetId())
	if !slices.ContainsFunc(sheet.GetDerived().GetIssues(), func(is *rulesv1.Issue) bool { return is.GetCode() == rules.IssueUnknownKey }) {
		t.Errorf("the sheet that knew the spell has issues %v, want %s", sheet.GetDerived().GetIssues(), rules.IssueUnknownKey)
	}
}

// refusedKeys are the keys the campaign's content reports as left out.
func (f *tolerantTable) refusedKeys(t *testing.T) []string {
	t.Helper()
	c, err := f.h.svc.content.ContentFor(t.Context(), nil, f.campaign)
	if err != nil {
		t.Fatalf("ContentFor() error = %v", err)
	}
	var keys []string
	for _, r := range c.Refused() {
		keys = append(keys, r.Key)
	}
	return keys
}

// update writes body over the stored entry of key, with the revision the master reads.
func (f *tolerantTable) update(key string, body proto.Message) error {
	rev := tableRevisionOf(f.h.t, f.master, f.campaign, key)
	_, err := f.master.table.UpdateTableEntry(f.h.t.Context(), connect.NewRequest(updateReq(f.campaign, key, rev, body)))
	return err
}

// TestTableContentWritesWorkWithARefusedEntryStored: with two entries refused, other entries
// are written, archived and switched, and repairing one leaves the other out.
func TestTableContentWritesWorkWithARefusedEntryStored(t *testing.T) {
	t.Parallel()
	f := newTolerantTable(t)
	f.h.refuseStored(f.campaign, f.sp.GetKey(), f.class.GetKey())
	other := f.master.addEntry(t, f.campaign, testBackground("Guarda de farol"))
	if _, _, err := f.h.svc.setArchived(t.Context(), f.membership(), other.GetKey(), true); err != nil {
		t.Fatalf("archive with refused entries stored: %v", err)
	}
	if _, _, err := f.h.svc.setArchived(t.Context(), f.membership(), other.GetKey(), false); err != nil {
		t.Fatalf("unarchive with refused entries stored: %v", err)
	}
	options := slices.Sorted(maps.Keys(f.master.options(t, f.campaign)))
	if len(options) == 0 {
		t.Fatal("the campaign lists no option to switch")
	}
	f.master.setOff(t, f.campaign, true, options[0])
	if !f.master.options(t, f.campaign)[options[0]].GetOff() {
		t.Errorf("option %s is not off after the switch", options[0])
	}

	spell := proto.Clone(bodyMessage(f.master.entries(t, f.campaign)[f.sp.GetKey()])).(*rulesv1.TableSpell)
	spell.Level = 1
	if err := f.update(f.sp.GetKey(), spell); err != nil {
		t.Fatalf("repairing the spell while the class is refused: %v", err)
	}
	if got, want := f.refusedKeys(t), []string{f.class.GetKey(), f.sub.GetKey()}; !slices.Equal(got, want) {
		t.Errorf("left out after repairing the spell = %v, want %v", got, want)
	}
	class := proto.Clone(bodyMessage(f.master.entries(t, f.campaign)[f.class.GetKey()])).(*rulesv1.TableClass)
	class.HitDie = 8
	if err := f.update(f.class.GetKey(), class); err != nil {
		t.Fatalf("repairing the class: %v", err)
	}
	if got := f.refusedKeys(t); len(got) != 0 {
		t.Errorf("left out after repairing both = %v, want none", got)
	}
}

// TestTableContentWriteThatBreaksAStoredEntryIsRefused: a write is refused for its own
// violations and for the healthy stored entry it breaks, with or without a refused entry stored.
func TestTableContentWriteThatBreaksAStoredEntryIsRefused(t *testing.T) {
	t.Parallel()
	f := newTolerantTable(t)
	broken := proto.Clone(bodyMessage(f.master.entries(t, f.campaign)[f.class.GetKey()])).(*rulesv1.TableClass)
	broken.SubclassLevel = 9
	wantSub := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: the write that breaks the subclass was accepted", label)
		}
		if v := violationsOfErr(t, err); !slices.ContainsFunc(v, func(x *rulesv1.TableContentViolation) bool {
			return x.GetKey() == f.sub.GetKey() && strings.HasPrefix(x.GetField(), "table_subclass")
		}) {
			t.Errorf("%s: violations %v, want one at the subclass's field with its key", label, v)
		}
	}
	wantSub("healthy table", f.update(f.class.GetKey(), broken))
	f.h.refuseStored(f.campaign, f.sp.GetKey())
	wantSub("a refused spell stored", f.update(f.class.GetKey(), broken))

	// The refused spell written again with the same fault: refused on its own field, no key.
	err := f.update(f.sp.GetKey(), bodyMessage(f.master.entries(t, f.campaign)[f.sp.GetKey()]))
	v := violationsOfErr(t, err)
	if len(v) == 0 || v[0].GetKey() != "" || v[0].GetField() != "table_spell.level" || v[0].GetReason() != rules.ReasonValue {
		t.Errorf("violations of the unrepaired spell = %v, want table_spell.level (bad_value) with no key", v)
	}
}

func (f *tolerantTable) membership() authz.Membership {
	return authz.Membership{CampaignID: f.campaign, Role: authz.RoleMaster}
}

// TestTableImportIgnoresAStoredEntryTheRulesRefuse: a stored entry outside the pack that the
// rules refuse does not make the import refuse; the pack's entries land.
func TestTableImportIgnoresAStoredEntryTheRulesRefuse(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	pt.master.addEntry(t, pt.from, testRace("Anão das Brumas"))
	pack := pt.export(t, pt.from)
	spell := pt.master.addEntry(t, pt.to, testSpell("Raio de teste", "class:wizard"))
	pt.h.refuseStored(pt.to, spell.GetKey())

	pv := pt.preview(t, pt.to, pack)
	if pv.GetTotals().GetRefused() != 0 || pv.GetTotals().GetNew() != 1 {
		t.Fatalf("preview = %v, want one new entry and no refusal", pv)
	}
	pt.apply(t, pt.to, pack, "tolerant-1")
	c, err := pt.h.svc.content.ContentFor(t.Context(), nil, pt.to)
	if err != nil || c.NamePT(pack.GetEntries()[0].GetKey()) != "Anão das Brumas" || len(c.Refused()) != 1 {
		t.Errorf("after the import: refused %v, %v; want the race in and the spell still left out", c.Refused(), err)
	}
}

// TestTableImportThatBreaksAStoredEntryIsRefused: a pack that updates a class so that a stored
// subclass breaks is refused, with a stored refused entry outside the pack present too.
func TestTableImportThatBreaksAStoredEntryIsRefused(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	class := testClass("Guardião")
	class.SubclassLevel = 9
	pt.master.addEntry(t, pt.from, class)
	pack := pt.export(t, pt.from)
	stored := pt.master.addEntry(t, pt.to, testClass("Guardião"))
	sub := pt.master.addEntry(t, pt.to, testSubclass("Caminho", stored.GetKey()))
	spell := pt.master.addEntry(t, pt.to, testSpell("Raio de teste", "class:wizard"))
	pt.h.refuseStored(pt.to, spell.GetKey())

	_, err := pt.importPack(pt.master, pt.to, pack, rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW, "")
	if err == nil {
		t.Fatal("the import that breaks the stored subclass was accepted")
	}
	if v := violationsOfErr(t, err); !slices.ContainsFunc(v, func(x *rulesv1.TableContentViolation) bool { return x.GetKey() == sub.GetKey() }) {
		t.Errorf("violations %v, want one that names the subclass", v)
	}
}

// TestTableSourceFailsWhenTheContentAsAWholeIsRefused: a violation that names no entry fails the read.
func TestTableSourceFailsWhenTheContentAsAWholeIsRefused(t *testing.T) {
	t.Parallel()
	f := newTolerantTable(t)
	_, err := f.h.pool.Exec(t.Context(), `INSERT INTO campaign_content (campaign_id, content_key, kind, name_pt, data, revision, created_at, updated_at)
		SELECT campaign_id, 'race:copia-' || i || '@mesa', kind, 'Copia ' || i, data, revision, created_at, updated_at
		FROM campaign_content, generate_series(1, 1000) AS i WHERE campaign_id = $1 AND content_key = $2`, f.campaign, f.race.GetKey())
	if err != nil {
		t.Fatalf("store 1000 more entries: %v", err)
	}
	f.h.refuseStored(f.campaign, f.sp.GetKey())
	if c, err := f.h.svc.content.ContentFor(t.Context(), nil, f.campaign); err == nil {
		t.Errorf("ContentFor() = %v, nil; want the read to fail over the limit", c)
	}
}

// listTable lists the campaign's entries as u, with the response whole.
func listTable(t *testing.T, u *user, campaign string) (*rulesv1.ListTableEntriesResponse, error) {
	t.Helper()
	res, err := u.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: campaign}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// TestListTableEntriesTellsOnlyTheMasterWhatIsLeftOut: the master reads the refused entries
// as left_out (key, field, reason) and still lists them; a player, the master of another
// campaign and the text of the entries read none of it.
func TestListTableEntriesTellsOnlyTheMasterWhatIsLeftOut(t *testing.T) {
	t.Parallel()
	f := newTolerantTable(t)
	other := f.h.newUser("Outro Mestre")
	otherCampaign := f.h.newCampaign(other, "Outra Mesa")
	f.h.refuseStored(f.campaign, f.class.GetKey(), f.sp.GetKey())

	// The positive control: the master reads each refused entry, once, with the engine's field and reason.
	got, err := listTable(t, f.master, f.campaign)
	if err != nil {
		t.Fatalf("master ListTableEntries() error = %v", err)
	}
	byKey := map[string]*rulesv1.TableContentViolation{}
	for _, v := range got.GetLeftOut() {
		if byKey[v.GetKey()] != nil {
			t.Errorf("left_out names %s twice", v.GetKey())
		}
		byKey[v.GetKey()] = v
	}
	want := map[string]struct{ field, reason string }{
		f.class.GetKey(): {"table_class.hit_die", rules.ReasonValue},
		f.sp.GetKey():    {"table_spell.level", rules.ReasonValue},
		f.sub.GetKey():   {"", rules.ReasonReference},
	}
	if len(byKey) != len(want) {
		t.Errorf("left_out = %v, want the keys %v", got.GetLeftOut(), slices.Sorted(maps.Keys(want)))
	}
	for k, w := range want {
		v := byKey[k]
		if v == nil {
			t.Errorf("left_out has no %s", k)
			continue
		}
		if v.GetReason() != w.reason || (w.field != "" && v.GetField() != w.field) || v.GetMessage() == "" {
			t.Errorf("left_out[%s] = %v, want reason %s, field %q, a message", k, v, w.reason, w.field)
		}
	}
	for _, e := range []*rulesv1.TableEntry{f.class, f.sp, f.sub} {
		if !slices.ContainsFunc(got.GetEntries(), func(x *rulesv1.TableEntry) bool { return x.GetKey() == e.GetKey() }) {
			t.Errorf("the master's entries lost %s: it must stay to edit or archive", e.GetKey())
		}
	}

	// Never the entries' text: not in the message, the field, the reason or the key.
	for _, v := range got.GetLeftOut() {
		seen := strings.Join([]string{v.GetKey(), v.GetField(), v.GetReason(), v.GetMessage()}, "\n")
		for _, e := range []*rulesv1.TableEntry{f.class, f.sub, f.sp} {
			for _, s := range entryTexts(e) {
				if strings.Contains(seen, s) {
					t.Errorf("left_out of %s holds the entry text %q: %s", v.GetKey(), s, seen)
				}
			}
		}
	}

	// A player of the campaign reads no left_out, and the entries they read are what they were.
	player, err := listTable(t, f.owner, f.campaign)
	if err != nil {
		t.Fatalf("player ListTableEntries() error = %v", err)
	}
	if len(player.GetLeftOut()) != 0 {
		t.Errorf("a player reads left_out %v, want none", player.GetLeftOut())
	}
	if _, ok := f.owner.entries(t, f.campaign)[f.race.GetKey()]; !ok {
		t.Errorf("the player no longer reads the healthy race")
	}
	if _, _, err := f.h.svc.setArchived(t.Context(), f.membership(), f.race.GetKey(), true); err != nil {
		t.Fatalf("archive the race: %v", err)
	}
	if _, ok := f.owner.entries(t, f.campaign)[f.race.GetKey()]; ok {
		t.Errorf("the player reads an archived entry")
	}
	if _, ok := f.master.entries(t, f.campaign)[f.race.GetKey()]; !ok {
		t.Errorf("the master lost the archived entry")
	}

	// The master of another campaign cannot read this one, and reads nothing of it in their own.
	if _, err := listTable(t, other, f.campaign); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another campaign's master reading this table: %v, want not_found", err)
	}
	own, err := listTable(t, other, otherCampaign)
	if err != nil {
		t.Fatalf("own ListTableEntries() error = %v", err)
	}
	if len(own.GetLeftOut()) != 0 || len(own.GetEntries()) != 0 {
		t.Errorf("another campaign's master reads %v in their own list, want nothing of this one", own)
	}
}

// entryTexts are the names and descriptions the entry holds: its strings that are not keys.
func entryTexts(e *rulesv1.TableEntry) []string {
	var out []string
	var walk func(m protoreflect.Message)
	walk = func(m protoreflect.Message) {
		m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			switch {
			case fd.IsList() && fd.Message() != nil:
				for i := range v.List().Len() {
					walk(v.List().Get(i).Message())
				}
			case fd.IsList() && fd.Kind() == protoreflect.StringKind:
				for i := range v.List().Len() {
					out = append(out, v.List().Get(i).String())
				}
			case fd.Message() != nil && !fd.IsMap():
				walk(v.Message())
			case fd.Kind() == protoreflect.StringKind && !fd.IsMap():
				out = append(out, v.String())
			}
			return true
		})
	}
	walk(e.ProtoReflect())
	return slices.DeleteFunc(out, func(s string) bool {
		return len(s) < 6 || strings.Contains(s, ":") || strings.Contains(s, "@mesa")
	})
}

// TestListTableEntriesLeftOutClearsWhenTheMasterRetiresTheEntry: "Remover" archives a refused entry, which
// clears it from left_out together with what is out only because of it; bringing it back lists it again.
func TestListTableEntriesLeftOutClearsWhenTheMasterRetiresTheEntry(t *testing.T) {
	t.Parallel()
	f := newTolerantTable(t)
	f.h.refuseStored(f.campaign, f.class.GetKey(), f.sp.GetKey())
	keys := func() []string {
		res, err := listTable(t, f.master, f.campaign)
		if err != nil {
			t.Fatalf("ListTableEntries() error = %v", err)
		}
		var out []string
		for _, v := range res.GetLeftOut() {
			out = append(out, v.GetKey())
		}
		slices.Sort(out)
		return out
	}
	if got, want := keys(), slices.Sorted(slices.Values([]string{f.class.GetKey(), f.sub.GetKey(), f.sp.GetKey()})); !slices.Equal(got, want) {
		t.Fatalf("left_out = %v, want %v", got, want)
	}
	if _, _, err := f.h.svc.setArchived(t.Context(), f.membership(), f.class.GetKey(), true); err != nil {
		t.Fatalf("archive the class: %v", err)
	}
	if got := keys(); !slices.Equal(got, []string{f.sp.GetKey()}) {
		t.Errorf("after archiving the class left_out = %v, want only the spell", got)
	}
	if _, ok := f.master.entries(t, f.campaign)[f.class.GetKey()]; !ok {
		t.Errorf("the master lost the archived class")
	}
	if c, err := f.h.svc.content.ContentFor(t.Context(), nil, f.campaign); err != nil || c.NamePT(f.class.GetKey()) != "" {
		t.Errorf("the archived refused class is in the content: %v", err)
	}
	// Bringing it back while it is still refused is refused; fixing it in place (it stays archived) brings the subclass back.
	if _, _, err := f.h.svc.setArchived(t.Context(), f.membership(), f.class.GetKey(), false); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("bring back a refused class: %v, want invalid_argument", err)
	}
	if err := f.update(f.class.GetKey(), testClass("Guardião")); err != nil {
		t.Fatalf("fix the archived class: %v", err)
	}
	if got := keys(); !slices.Equal(got, []string{f.sp.GetKey()}) {
		t.Errorf("after fixing the class left_out = %v, want only the spell", got)
	}
	if c, err := f.h.svc.content.ContentFor(t.Context(), nil, f.campaign); err != nil || c.NamePT(f.sub.GetKey()) == "" {
		t.Errorf("the subclass did not come back with its fixed class: %v", err)
	}
}
