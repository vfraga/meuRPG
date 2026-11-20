package characters

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// #580, board 580-16 state 8 ("A arma mudou."): the master edits a table weapon to take both hands, and the sheet that
// carries it with an equipped shield is told the shield stands idle. A notice, not an issue: it is not a "Pendência", it
// refuses no save and no level-up, and it goes away when the owner stows the shield or the weapon.

const (
	twoHandedProp = "weapon-property:two-handed"
	versatileProp = "weapon-property:versatile"
)

// idleTable is the level-up table (Pensantus, the owner's, locked sheet) whose sheet carries a table weapon and a shield.
type idleTable struct {
	*levelUpTable
	weapon *rulesv1.TableEntry
}

// newIdleTable gives the table the weapon "Machado de Lenhador" with the properties given, and saves the master's copy of
// the sheet with that weapon and a shield (so the sheet fits the table as it is now).
func newIdleTable(t *testing.T, edit func(*rulesv1.TableWeapon)) *idleTable {
	t.Helper()
	tb := &idleTable{levelUpTable: newLevelUpTable(t, 2700)}
	w := testWeapon("Machado de Lenhador")
	edit(w)
	tb.weapon = tb.master.addEntry(t, tb.campaign, w)
	// A save first, so the sheet is known to fit the table as it is now: an entry made after the sheet's last save would
	// read as one that changed under it.
	current := tb.master.get(t, tb.campaign, tb.pc.GetId())
	current, err := tb.master.update(t, current, current.GetName(), current.GetSheet())
	if err != nil {
		t.Fatalf("the master's save before the weapon: %v", err)
	}
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().Shield = true
	sheet.GetFull().WeaponKeys = append(sheet.GetFull().WeaponKeys, tb.weapon.GetKey())
	if _, err := tb.master.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatalf("the master's save of the weapon and the shield: %v", err)
	}
	tb.pc = tb.owner.get(t, tb.campaign, tb.pc.GetId())
	return tb
}

func versatile(*rulesv1.TableWeapon) {}

func twoHanded(w *rulesv1.TableWeapon) {
	w.Properties, w.TwoHandedDamage = []string{twoHandedProp}, ""
}

// edit is the master's edit of the weapon.
func (tb *idleTable) edit(t *testing.T, change func(*rulesv1.TableWeapon)) {
	t.Helper()
	entry := tb.master.entries(t, tb.campaign)[tb.weapon.GetKey()]
	edited := proto.CloneOf(bodyMessage(entry).(*rulesv1.TableWeapon))
	change(edited)
	if _, err := tb.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(tb.campaign, tb.weapon.GetKey(), entry.GetRevision(), edited))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
}

// banner is what the reader's changed entry for the weapon says, or nil when there is none.
func (tb *idleTable) banner(c *charactersv1.Character) []string {
	for _, ch := range c.GetDerived().GetChangedContent() {
		if ch.GetKey() == tb.weapon.GetKey() {
			return ch.GetMessages()
		}
	}
	return nil
}

func (tb *idleTable) sentence(name string, with, without int32) string {
	return fmt.Sprintf("A arma Machado de Lenhador passou a exigir as duas mãos. Com ela em uso, o escudo de %s não conta na CA (CA %d → %d).", name, with, without)
}

func issueCodes(c *charactersv1.Character) []string {
	var out []string
	for _, is := range c.GetDerived().GetIssues() {
		if is.GetCode() != IssueTableContentChanged {
			out = append(out, is.GetCode())
		}
	}
	slices.Sort(out)
	return out
}

// The board's sentence, with the AC numbers of the sheet, reaches the owner and the master; no other player reads it.
func TestTheMastersTwoHandedEditTellsTheShieldIdleToTheOwnerAndTheMaster(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, versatile)
	before := tb.owner.get(t, tb.campaign, tb.pc.GetId())
	if got := tb.banner(before); got != nil {
		t.Fatalf("the banner before the edit = %v", got)
	}
	with := before.GetDerived().GetArmorClass()
	tb.edit(t, twoHanded)

	want := tb.sentence("Pensantus", with, with-2)
	for who, u := range map[string]*user{"the owner": tb.owner, "the master": tb.master} {
		got := u.get(t, tb.campaign, tb.pc.GetId())
		if msgs := tb.banner(got); !slices.Equal(msgs, []string{want}) {
			t.Errorf("%s reads %q, want %q", who, msgs, want)
		}
		// The only pending issue that may go is the one about a shield the sheet does not use now (a wizard has no
		// shield training), never one that comes.
		if extra := slices.DeleteFunc(issueCodes(got), func(c string) bool { return slices.Contains(issueCodes(before), c) }); len(extra) != 0 {
			t.Errorf("%s: new pending issues %v after the edit", who, extra)
		}
		for _, is := range got.GetDerived().GetIssues() {
			if is.GetCode() == rules.IssueShieldIdle {
				t.Errorf("%s reads the notice as an issue: %v", who, is)
			}
		}
		if !got.GetDerived().GetShieldIdle() {
			t.Errorf("%s: the sheet does not say the shield is idle", who)
		}
	}

	_, err := tb.other.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: tb.campaign, CharacterId: tb.pc.GetId()}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another player's GetCharacter error = %v, want not found", err)
	}
	for _, s := range tb.other.list(t, tb.campaign) {
		if s.GetId() == tb.pc.GetId() {
			t.Errorf("another player's list holds the character: %v", s)
		}
	}
}

// Nothing stored names a sentence: the identity is the code, the list and the weapon's key.
func TestTheShieldIdleIdentityIsStoredAndNamesNoSentence(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, twoHanded)
	stored := storedIssues(t, tb.h, tb.pc.GetId())
	want := "shield_idle|full.weapon_keys|idle:" + tb.weapon.GetKey()
	if !slices.Contains(stored, want) {
		t.Errorf("stored issues = %q, want %q among them", stored, want)
	}
	for _, id := range stored {
		if strings.Contains(id, "mãos") || strings.Contains(id, "Machado") || strings.Contains(id, "CA ") {
			t.Errorf("a stored identity holds a sentence: %q", id)
		}
	}
}

// A sheet that already had the two-handed weapon and the shield is told nothing when the weapon is edited again (its
// identity is stored), and a sheet whose stored issues predate the notice is told nothing while the weapon is not edited.
func TestASheetThatAlreadyHadTheShieldIdleIsToldNothing(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, twoHanded)
	if got := tb.banner(tb.owner.get(t, tb.campaign, tb.pc.GetId())); got != nil {
		t.Errorf("the banner with no edit = %v, want none", got)
	}
	tb.edit(t, func(w *rulesv1.TableWeapon) { w.Damage = "1d12" })
	for who, u := range map[string]*user{"the owner": tb.owner, "the master": tb.master} {
		if got := tb.banner(u.get(t, tb.campaign, tb.pc.GetId())); got != nil {
			t.Errorf("%s reads %q after a damage edit of a weapon that already took both hands, want none", who, got)
		}
	}

	old := newIdleTable(t, twoHanded)
	if _, err := old.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,known_issues}', COALESCE((SELECT jsonb_agg(e) FROM jsonb_array_elements(sheet->'full'->'known_issues') e WHERE e::text NOT LIKE '%shield_idle%'), '[]'::jsonb)) WHERE id = $1`, old.pc.GetId()); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(storedIssues(t, old.h, old.pc.GetId()), func(id string) bool { return strings.HasPrefix(id, "shield_idle|") }) {
		t.Fatal("the stored identity is still there: the case tests nothing")
	}
	if got := old.banner(old.owner.get(t, old.campaign, old.pc.GetId())); got != nil {
		t.Errorf("a sheet saved before the notice, no edit: banner %v, want none", got)
	}
}

// What is not the edit that puts "Duas mãos" on the weapon tells nothing.
func TestOnlyTheTwoHandedEditRaisesTheNotice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit func(*rulesv1.TableWeapon)
	}{
		{"damage only", func(w *rulesv1.TableWeapon) { w.Damage = "1d12" }},
		{"versatile removed", func(w *rulesv1.TableWeapon) { w.Properties, w.TwoHandedDamage = nil, "" }},
		{"finesse added", func(w *rulesv1.TableWeapon) { w.Properties = append(w.Properties, "weapon-property:finesse") }},
		{"a range", func(w *rulesv1.TableWeapon) {
			w.Properties, w.TwoHandedDamage = []string{versatileProp, "weapon-property:thrown"}, "1d10"
			w.NormalRangeFt, w.LongRangeFt = 20, 60
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tb := newIdleTable(t, versatile)
			tb.edit(t, tc.edit)
			for who, u := range map[string]*user{"the owner": tb.owner, "the master": tb.master} {
				if got := tb.banner(u.get(t, tb.campaign, tb.pc.GetId())); got != nil {
					t.Errorf("%s reads %q, want no banner", who, got)
				}
			}
		})
	}
}

// The notice refuses nothing (a master's save, a level-up), a plain save does not clear it, and it goes away when the
// owner stows the shield or the weapon.
func TestTheShieldIdleNoticeRefusesNothingAndGoesWhenTheGearMoves(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, versatile)
	with := tb.owner.get(t, tb.campaign, tb.pc.GetId()).GetDerived().GetArmorClass()
	tb.edit(t, twoHanded)
	want := []string{tb.sentence("Pensantus", with, with-2)}
	read := func() *charactersv1.Character { return tb.owner.get(t, tb.campaign, tb.pc.GetId()) }

	current := tb.master.get(t, tb.campaign, tb.pc.GetId())
	if _, err := tb.master.update(t, current, current.GetName(), current.GetSheet()); err != nil {
		t.Fatalf("the master's save was refused: %v", err)
	}
	if got := tb.banner(read()); !slices.Equal(got, want) {
		t.Errorf("after a plain save: %q, want the notice to stay: %q", got, want)
	}

	leveled, err := tb.levelUp(tb.owner, read(), pensantusLevelUp())
	if err != nil {
		t.Fatalf("the level-up was refused: %v", err)
	}
	if leveled.GetSheet().GetFull().GetClasses()[0].GetLevel() != 4 {
		t.Fatalf("the level-up left the sheet at %v", leveled.GetSheet().GetFull().GetClasses())
	}
	if got := tb.banner(read()); len(got) != 1 || !strings.HasPrefix(got[0], "A arma Machado de Lenhador passou a exigir as duas mãos. Com ela em uso, o escudo de Pensantus não conta na CA (CA ") {
		t.Errorf("after the level-up: %q, want the notice to stay", got)
	}

	for _, key := range []string{rules.ShieldKey, tb.weapon.GetKey()} {
		mustSetEquipped(t, tb.owner, read(), key, false)
		if got := tb.banner(read()); got != nil {
			t.Errorf("after stowing %s: %q, want the notice gone", key, got)
		}
		mustSetEquipped(t, tb.owner, read(), key, true)
	}
}

// A notice that ended does not come back when the owner takes the shield or the weapon up again: it told of a change and
// the owner has seen it.
func TestAnEndedShieldIdleNoticeDoesNotComeBackWhenTheGearIsTakenUpAgain(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, versatile)
	with := tb.owner.get(t, tb.campaign, tb.pc.GetId()).GetDerived().GetArmorClass()
	tb.edit(t, twoHanded)
	read := func() *charactersv1.Character { return tb.owner.get(t, tb.campaign, tb.pc.GetId()) }
	if got, want := tb.banner(read()), []string{tb.sentence("Pensantus", with, with-2)}; !slices.Equal(got, want) {
		t.Fatalf("the banner before the owner moves anything = %q, want %q", got, want)
	}
	for _, key := range []string{rules.ShieldKey, tb.weapon.GetKey()} {
		mustSetEquipped(t, tb.owner, read(), key, false)
		if got := tb.banner(read()); got != nil {
			t.Errorf("after stowing %s: %q, want the notice gone", key, got)
		}
		mustSetEquipped(t, tb.owner, read(), key, true)
		if got := tb.banner(read()); got != nil {
			t.Errorf("after taking %s up again: %q, want the notice to stay gone", key, got)
		}
	}
	// The same with a save between the stowing and the taking up.
	mustSetEquipped(t, tb.owner, read(), rules.ShieldKey, false)
	current := tb.master.get(t, tb.campaign, tb.pc.GetId())
	if _, err := tb.master.update(t, current, current.GetName(), current.GetSheet()); err != nil {
		t.Fatalf("the master's save: %v", err)
	}
	mustSetEquipped(t, tb.owner, read(), rules.ShieldKey, true)
	if got := tb.banner(read()); got != nil {
		t.Errorf("after a save and taking the shield up again: %q, want the notice to stay gone", got)
	}
}

// A weapon the master archived after the edit is still named to the owner: the sheet has it.
func TestTheShieldIdleNoticeNamesAnArchivedWeaponToItsOwner(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, versatile)
	with := tb.owner.get(t, tb.campaign, tb.pc.GetId()).GetDerived().GetArmorClass()
	tb.edit(t, twoHanded)
	if _, err := tb.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: tb.weapon.GetKey()})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	if got, want := tb.banner(tb.owner.get(t, tb.campaign, tb.pc.GetId())), []string{tb.sentence("Pensantus", with, with-2)}; !slices.Equal(got, want) {
		t.Errorf("the owner reads %q, want %q", got, want)
	}
}

// stripIdleIdentity makes the stored sheet one saved before the notice existed: its known issues hold no shield_idle.
func (tb *idleTable) stripIdleIdentity(t *testing.T) {
	t.Helper()
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,known_issues}', COALESCE((SELECT jsonb_agg(e) FROM jsonb_array_elements(sheet->'full'->'known_issues') e WHERE e::text NOT LIKE '%shield_idle%'), '[]'::jsonb)) WHERE id = $1`, tb.pc.GetId()); err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(storedIssues(t, tb.h, tb.pc.GetId()), func(id string) bool { return strings.HasPrefix(id, "shield_idle|") }) {
		t.Fatal("the stored identity is still there: the case tests nothing")
	}
}

func bannerOf(c *charactersv1.Character, key string) []string {
	for _, ch := range c.GetDerived().GetChangedContent() {
		if ch.GetKey() == key {
			return ch.GetMessages()
		}
	}
	return nil
}

// weaponSince is the revision the table says the weapon last became two-handed at, as the master reads the entry.
func (tb *idleTable) weaponSince(t *testing.T, key string) int32 {
	t.Helper()
	return bodyMessage(tb.master.entries(t, tb.campaign)[key]).(*rulesv1.TableWeapon).GetTwoHandedSinceRevision()
}

// A sheet that already had a two-handed table weapon and a shield, saved before the notice existed (no stored identity),
// is told nothing when that weapon is edited for anything but "Duas mãos"; the edit that puts "Duas mãos" on another weapon
// it has tells nothing either while the first weapon keeps the shield idle: stowing the second alone moves no number.
func TestASheetSavedBeforeTheNoticeIsToldNothingWhileAnotherTwoHandedWeaponKeepsTheShieldIdle(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, twoHanded)
	second := tb.master.addEntry(t, tb.campaign, testWeapon("Martelo de Ferreiro"))
	current := tb.master.get(t, tb.campaign, tb.pc.GetId())
	current, err := tb.master.update(t, current, current.GetName(), current.GetSheet()) // fits the table with the second weapon
	if err != nil {
		t.Fatal(err)
	}
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().WeaponKeys = append(sheet.GetFull().WeaponKeys, second.GetKey())
	if _, err := tb.master.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatal(err)
	}
	tb.stripIdleIdentity(t)

	tb.edit(t, func(w *rulesv1.TableWeapon) { w.Damage = "1d12" })
	for who, u := range map[string]*user{"the owner": tb.owner, "the master": tb.master} {
		if got := tb.banner(u.get(t, tb.campaign, tb.pc.GetId())); got != nil {
			t.Errorf("%s reads %q after a damage edit of a weapon that was already two-handed, want none", who, got)
		}
	}

	entry := tb.master.entries(t, tb.campaign)[second.GetKey()]
	edited := proto.CloneOf(bodyMessage(entry).(*rulesv1.TableWeapon))
	edited.Properties, edited.TwoHandedDamage = []string{twoHandedProp}, ""
	if _, err := tb.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(tb.campaign, second.GetKey(), entry.GetRevision(), edited))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	for who, u := range map[string]*user{"the owner": tb.owner, "the master": tb.master} {
		got := u.get(t, tb.campaign, tb.pc.GetId())
		if msgs := bannerOf(got, second.GetKey()); msgs != nil {
			t.Errorf("%s reads %q for the second weapon, want none: the first still keeps the shield idle", who, msgs)
		}
		if msgs := tb.banner(got); msgs != nil {
			t.Errorf("%s reads %q for the first weapon, which was already two-handed, want none", who, msgs)
		}
	}
}

// The server keeps, on a weapon, the revision it last became two-handed at: set when it is made so or edited so, kept by any
// other edit, cleared when the property goes, and never taken from the request.
func TestAWeaponKeepsTheRevisionItBecameTwoHandedAt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	tb := &idleTable{levelUpTable: &levelUpTable{h: h, master: master, campaign: campaign}}
	w := testWeapon("Machado de Lenhador")
	twoHanded(w)
	w.TwoHandedSinceRevision = 99 // a client's value is ignored
	tb.weapon = master.addEntry(t, campaign, w)
	key := tb.weapon.GetKey()
	made := tb.weaponSince(t, key)
	if made != tb.weapon.GetRevision() || made == 99 {
		t.Fatalf("made two-handed at revision %d, since %d; want the entry's own revision", tb.weapon.GetRevision(), made)
	}
	tb.edit(t, func(w *rulesv1.TableWeapon) { w.Damage = "1d12"; w.TwoHandedSinceRevision = 99 })
	if got := tb.weaponSince(t, key); got != made {
		t.Errorf("after a damage edit since = %d, want %d kept", got, made)
	}
	tb.edit(t, func(w *rulesv1.TableWeapon) { w.Properties, w.TwoHandedDamage = nil, "" })
	if got := tb.weaponSince(t, key); got != 0 {
		t.Errorf("after the property went since = %d, want 0", got)
	}
	tb.edit(t, twoHanded)
	if got, rev := tb.weaponSince(t, key), master.entries(t, campaign)[key].GetRevision(); got != rev || got <= made {
		t.Errorf("after the property came back since = %d, want the new revision %d", got, rev)
	}
}

// A pack carries no revision of another table: what it holds in the field is ignored and the server's own is set.
func TestAnImportedTwoHandedWeaponTakesTheRevisionOfTheImport(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	w := testWeapon("Machado de Lenhador")
	twoHanded(w)
	made := pt.master.addEntry(t, pt.from, w)
	pack := pt.export(t, pt.from)
	for _, e := range pack.GetEntries() {
		if e.GetTableWeapon() != nil {
			e.GetTableWeapon().TwoHandedSinceRevision = 99
		}
	}
	res := pt.apply(t, pt.to, pack, "weapon-import")
	rev := pt.revisionOf(t, pt.to)
	got := bodyMessage(pt.entries(t, pt.to)[made.GetKey()]).(*rulesv1.TableWeapon).GetTwoHandedSinceRevision()
	if got != rev {
		t.Errorf("imported weapon since = %d (response %v), want the import's revision %d", got, res.GetTotals(), rev)
	}
}

// A character without shield training has no pending issue for the shield while it stands idle: the issue is raised only for
// a shield in use. This is the behavior today, written down.
func TestTheShieldTrainingIssueGoesWhileTheShieldIsIdle(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, versatile)
	before := tb.owner.get(t, tb.campaign, tb.pc.GetId())
	if !slices.Contains(issueCodes(before), "armor_proficiency") {
		t.Fatalf("pending issues before = %v, want the shield training issue (a wizard has none)", issueCodes(before))
	}
	tb.edit(t, twoHanded)
	after := tb.owner.get(t, tb.campaign, tb.pc.GetId())
	if slices.Contains(issueCodes(after), "armor_proficiency") {
		t.Errorf("pending issues after = %v, want no shield training issue while the shield is idle", issueCodes(after))
	}
}

// withSecondWeapon puts another table weapon (one-handed, no two-handed property) on the sheet next to the first one and
// the shield, and saves it as the master.
func (tb *idleTable) withSecondWeapon(t *testing.T, name string) string {
	t.Helper()
	second := tb.master.addEntry(t, tb.campaign, testWeapon(name))
	current := tb.master.get(t, tb.campaign, tb.pc.GetId())
	current, err := tb.master.update(t, current, current.GetName(), current.GetSheet()) // fits the table with the second weapon
	if err != nil {
		t.Fatal(err)
	}
	sheet := proto.CloneOf(current.GetSheet())
	sheet.GetFull().WeaponKeys = append(sheet.GetFull().WeaponKeys, second.GetKey())
	if _, err := tb.master.update(t, current, current.GetName(), sheet); err != nil {
		t.Fatal(err)
	}
	return second.GetKey()
}

// The notice that was flagged stays until the owner stows the shield or the weapon it names: stowing or taking up another
// weapon changes nothing it says.
func TestAnUnrelatedEquipKeepsTheShieldIdleBanner(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, versatile)
	other := tb.withSecondWeapon(t, "Adaga de Mesa")
	with := tb.owner.get(t, tb.campaign, tb.pc.GetId()).GetDerived().GetArmorClass()
	tb.edit(t, twoHanded)
	read := func() *charactersv1.Character { return tb.owner.get(t, tb.campaign, tb.pc.GetId()) }
	want := []string{tb.sentence("Pensantus", with, with-2)}
	if got := tb.banner(read()); !slices.Equal(got, want) {
		t.Fatalf("the banner before any equip = %q, want %q", got, want)
	}

	mustSetEquipped(t, tb.owner, read(), other, false)
	if got := tb.banner(read()); !slices.Equal(got, want) {
		t.Errorf("after stowing another weapon: %q, want the notice to stay: %q", got, want)
	}
	mustSetEquipped(t, tb.owner, read(), other, true)
	if got := tb.banner(read()); !slices.Equal(got, want) {
		t.Errorf("after taking it up again: %q, want the notice to stay: %q", got, want)
	}
	for _, key := range []string{rules.ShieldKey, tb.weapon.GetKey()} {
		mustSetEquipped(t, tb.owner, read(), other, false)
		mustSetEquipped(t, tb.owner, read(), key, false)
		if got := tb.banner(read()); got != nil {
			t.Errorf("after stowing %s: %q, want the notice gone", key, got)
		}
		mustSetEquipped(t, tb.owner, read(), key, true)
		mustSetEquipped(t, tb.owner, read(), other, true)
	}
}

// A weapon the master switched off after the edit is still named to the owner, as an archived one is: the sheet has it.
func TestTheShieldIdleNoticeNamesASwitchedOffWeaponToItsOwner(t *testing.T) {
	t.Parallel()
	tb := newIdleTable(t, versatile)
	with := tb.owner.get(t, tb.campaign, tb.pc.GetId()).GetDerived().GetArmorClass()
	tb.edit(t, twoHanded)
	tb.master.setOff(t, tb.campaign, true, tb.weapon.GetKey())
	want := []string{tb.sentence("Pensantus", with, with-2)}
	for who, u := range map[string]*user{"the owner": tb.owner, "the master": tb.master} {
		if got := tb.banner(u.get(t, tb.campaign, tb.pc.GetId())); !slices.Equal(got, want) {
			t.Errorf("%s reads %q, want %q", who, got, want)
		}
	}
}
