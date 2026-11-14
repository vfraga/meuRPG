package characters

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

const (
	staff  = "equipment:quarterstaff"
	dagger = "equipment:dagger"
	leath  = "equipment:leather-armor"
)

// equipHostSpy is a ReviewHost whose EquipInCombat records the changes it is asked about,
// the revision the character has in the transaction at that moment, and fails when told to.
type equipHostSpy struct {
	*gearHostSpy
	mu        sync.Mutex
	changes   []EquipChange
	revisions []int32
	actors    []string
	fail      error
}

func (e *equipHostSpy) EquipInCombat(ctx context.Context, tx pgx.Tx, _, characterID, actor string, change EquipChange, _ sessiontell.Tell) error {
	var revision int32
	if err := tx.QueryRow(ctx, `SELECT revision FROM characters WHERE id = $1`, characterID).Scan(&revision); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.changes, e.revisions, e.actors = append(e.changes, change), append(e.revisions, revision), append(e.actors, actor)
	return e.fail
}

// gearSheet is Pensantus with leather armor, a shield and two weapons.
func gearSheet() *charactersv1.CharacterSheet {
	return editorClone(pensantusSheet(), func(f *charactersv1.FullSheet) {
		f.ArmorKey, f.Shield, f.WeaponKeys = leath, true, []string{staff, dagger}
	})
}

type equipTable struct {
	*editorTable
	spy *equipHostSpy
	pc  *charactersv1.Character
}

func newEquipTable(t *testing.T) *equipTable {
	t.Helper()
	et := &equipTable{editorTable: newEditorTable(t, nil), spy: &equipHostSpy{gearHostSpy: &gearHostSpy{}}}
	et.h.svc.SetReviewHost(et.spy)
	et.pc = et.dona.create(t, et.campaign, editorPlayer, "Pensantus", gearSheet())
	return et
}

func setEquipped(u *user, c *charactersv1.Character, key string, equipped bool) (*charactersv1.SetEquippedResponse, error) {
	res, err := u.api.SetEquipped(context.Background(), connect.NewRequest(&charactersv1.SetEquippedRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(), Key: key, Equipped: equipped,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func mustSetEquipped(t *testing.T, u *user, c *charactersv1.Character, key string, equipped bool) *charactersv1.SetEquippedResponse {
	t.Helper()
	res, err := setEquipped(u, c, key, equipped)
	if err != nil {
		t.Fatalf("SetEquipped(%s, %v) error = %v", key, equipped, err)
	}
	return res
}

// withoutCarried is the sheet with carried_keys cleared, to compare what else changed.
func withoutCarried(c *charactersv1.Character) *charactersv1.CharacterSheet {
	out := proto.CloneOf(c.GetSheet())
	out.GetFull().CarriedKeys, out.GetFull().KnownIssues = nil, nil // the issues follow the gear in use
	return out
}

// The owner stows and equips on a draft and on a locked sheet, the master too; the
// revision goes up with each change and nothing else on the sheet moves.
func TestSetEquippedOwnerAndMaster(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	was := withoutCarried(et.pc)

	res := mustSetEquipped(t, et.dona, et.pc, staff, false)
	if !slices.Equal(res.GetCarriedKeys(), []string{staff}) || res.GetRevision() != et.pc.GetRevision()+1 {
		t.Fatalf("stow on a draft = %v, revision %d; want [%s], %d", res.GetCarriedKeys(), res.GetRevision(), staff, et.pc.GetRevision()+1)
	}
	et.h.lockSheets(et.campaign)
	res = mustSetEquipped(t, et.dona, et.pc, rules.ShieldKey, false)
	if !slices.Equal(res.GetCarriedKeys(), []string{staff, rules.ShieldKey}) {
		t.Errorf("stow on a locked sheet by the owner = %v", res.GetCarriedKeys())
	}
	res = mustSetEquipped(t, et.master, et.pc, leath, false)
	if !slices.Equal(res.GetCarriedKeys(), []string{staff, rules.ShieldKey, leath}) {
		t.Errorf("stow by the master = %v", res.GetCarriedKeys())
	}
	res = mustSetEquipped(t, et.master, et.pc, staff, true)
	if !slices.Equal(res.GetCarriedKeys(), []string{rules.ShieldKey, leath}) {
		t.Errorf("equip by the master = %v", res.GetCarriedKeys())
	}
	got := et.dona.get(t, et.campaign, et.pc.GetId())
	if !slices.Equal(got.GetSheet().GetFull().GetCarriedKeys(), res.GetCarriedKeys()) || got.GetRevision() != res.GetRevision() {
		t.Errorf("stored carried %v rev %d, want %v rev %d", got.GetSheet().GetFull().GetCarriedKeys(), got.GetRevision(), res.GetCarriedKeys(), res.GetRevision())
	}
	if !proto.Equal(withoutCarried(got), was) {
		t.Errorf("SetEquipped changed more than carried_keys:\n got  %v\n want %v", withoutCarried(got), was)
	}
	if got.GetRevision() != et.pc.GetRevision()+4 {
		t.Errorf("revision = %d after four changes, want %d", got.GetRevision(), et.pc.GetRevision()+4)
	}
}

// Stowing the armor changes the derived armor class the owner reads.
func TestSetEquippedChangesTheDerivedSheet(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	acOf := func() int32 { return et.dona.get(t, et.campaign, et.pc.GetId()).GetDerived().GetArmorClass() }
	worn := acOf()
	mustSetEquipped(t, et.dona, et.pc, leath, false)
	mustSetEquipped(t, et.dona, et.pc, rules.ShieldKey, false)
	if got := acOf(); got >= worn {
		t.Errorf("AC with armor and shield stowed = %d, want less than %d", got, worn)
	}
}

// Who may not: another player and a stranger get the answer of a character that is not
// there, a player never reaches an NPC, a dead character is refused (master included), and
// a key the sheet does not list, or a basic sheet, is an invalid argument.
func TestSetEquippedRefusals(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	npc := et.master.create(t, et.campaign, editorEnemy, "Bandido", enemySheet())
	minion := et.master.create(t, et.campaign, editorMinion, "Goblin", basicSheet())
	noShield := et.master.create(t, et.campaign, editorEnemy, "Sem escudo", editorClone(enemySheet(), func(f *charactersv1.FullSheet) { f.Shield = false }))
	refusal := func(want string) func(*testing.T, string, error) {
		return func(t *testing.T, name string, err error) { t.Helper(); wantEditorRefusal(t, name, err, want) }
	}
	code := func(want connect.Code) func(*testing.T, string, error) {
		return func(t *testing.T, name string, err error) {
			t.Helper()
			if ce, ok := errors.AsType[*connect.Error](err); !ok || ce.Code() != want {
				t.Errorf("%s: %v, want %v", name, err, want)
			}
		}
	}
	invalidKey := `invalid_argument: key is not the armor, the shield or a weapon of the sheet | InvalidField{field="key"}`
	for _, c := range []struct {
		name  string
		as    *user
		on    *charactersv1.Character
		key   string
		check func(*testing.T, string, error)
	}{
		{"another player", et.caio, et.pc, staff, refusal(`not_found: character not found`)},
		{"a stranger", et.visitor, et.pc, staff, code(connect.CodeNotFound)},
		{"a player on an NPC", et.dona, npc, "equipment:longsword", refusal(`not_found: character not found`)},
		{"an unlisted key", et.dona, et.pc, "equipment:longsword", refusal(invalidKey)},
		{"the shield of a sheet without one", et.master, noShield, rules.ShieldKey, refusal(invalidKey)},
		{"an empty key", et.dona, et.pc, "", refusal(`invalid_argument: key is required | InvalidField{field="key"}`)},
		{"a basic sheet", et.master, minion, "equipment:dagger", refusal(`invalid_argument: character_id is a character with a basic sheet, which has no gear to equip | InvalidField{field="character_id"}`)},
	} {
		_, err := setEquipped(c.as, c.on, c.key, false)
		c.check(t, c.name, err)
	}
	if len(et.spy.changes) != 0 {
		t.Errorf("play was asked about %v for refused calls", et.spy.changes)
	}

	if _, err := et.master.api.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: et.campaign, CharacterId: et.pc.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	for name, u := range map[string]*user{"owner": et.dona, "master": et.master} {
		_, err := setEquipped(u, et.pc, staff, false)
		wantEditorRefusal(t, "dead, by the "+name, err,
			`failed_precondition: the character is dead, so only the master edits its sheet | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_CHARACTER_DEAD character_id="`+et.pc.GetId()+`"}`)
	}
}

// Asking for the state the piece has changes nothing: no revision, no call to play.
func TestSetEquippedIsIdempotent(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	res := mustSetEquipped(t, et.dona, et.pc, staff, true) // already equipped
	if res.GetRevision() != et.pc.GetRevision() || len(res.GetCarriedKeys()) != 0 || len(et.spy.changes) != 0 {
		t.Errorf("equipping an equipped piece = %v rev %d, play asked %v; want a no-op", res.GetCarriedKeys(), res.GetRevision(), et.spy.changes)
	}
	first := mustSetEquipped(t, et.dona, et.pc, staff, false)
	again := mustSetEquipped(t, et.dona, et.pc, staff, false)
	if again.GetRevision() != first.GetRevision() || !slices.Equal(again.GetCarriedKeys(), first.GetCarriedKeys()) || len(et.spy.changes) != 1 {
		t.Errorf("stowing twice: rev %d then %d, play asked %d times; want the same revision and one ask", first.GetRevision(), again.GetRevision(), len(et.spy.changes))
	}
}

// The editor is no way around the lock: a player's save of a locked sheet is refused
// whatever it carries, and the master's save works. A draft save drops carried keys the
// sheet no longer lists.
func TestUpdateCharacterCarriedKeysAndTheLock(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	pc := et.dona.get(t, et.campaign, et.pc.GetId())
	req := editorSaveOf(pc)
	req.Sheet.GetFull().CarriedKeys = []string{dagger}
	saved, err := editorUpdate(et.dona, req)
	if err != nil || !slices.Equal(saved.GetSheet().GetFull().GetCarriedKeys(), []string{dagger}) {
		t.Fatalf("a draft save with a carried weapon = %v, %v", saved.GetSheet().GetFull().GetCarriedKeys(), err)
	}
	req = editorSaveOf(saved)
	req.Sheet.GetFull().WeaponKeys = []string{staff}
	if saved, err = editorUpdate(et.dona, req); err != nil || len(saved.GetSheet().GetFull().GetCarriedKeys()) != 0 {
		t.Fatalf("removing the carried weapon = %v, %v; want its mark gone", saved.GetSheet().GetFull().GetCarriedKeys(), err)
	}
	et.h.lockSheets(et.campaign)
	req = editorSaveOf(saved)
	req.Sheet.GetFull().CarriedKeys = []string{staff}
	_, err = editorUpdate(et.dona, req)
	wantEditorRefusal(t, "a player's save of a locked sheet", err,
		`failed_precondition: the sheet is locked: a game session has started, so only the master edits it | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_SHEET_LOCKED character_id="`+pc.GetId()+`"}`)
	if got, err := editorUpdate(et.master, req); err != nil || !slices.Equal(got.GetSheet().GetFull().GetCarriedKeys(), []string{staff}) {
		t.Errorf("the master's save = %v, %v", got.GetSheet().GetFull().GetCarriedKeys(), err)
	}
}

// RN-10: carried_keys lives in the sheet, which only the owner and the master read. Another
// player gets not_found for the character and does not list it; the sheet's other gear
// fields are behind the same gate.
func TestCarriedKeysFollowTheSheetsGate(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	mustSetEquipped(t, et.dona, et.pc, staff, false)
	for name, u := range map[string]*user{"owner": et.dona, "master": et.master} {
		full := u.get(t, et.campaign, et.pc.GetId()).GetSheet().GetFull()
		if !slices.Equal(full.GetCarriedKeys(), []string{staff}) || full.GetArmorKey() != leath || len(full.GetWeaponKeys()) != 2 {
			t.Errorf("%s reads carried %v armor %q weapons %v", name, full.GetCarriedKeys(), full.GetArmorKey(), full.GetWeaponKeys())
		}
	}
	_, err := et.caio.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: et.campaign, CharacterId: et.pc.GetId()}))
	if ce, ok := errors.AsType[*connect.Error](err); !ok || ce.Code() != connect.CodeNotFound {
		t.Errorf("another player's GetCharacter = %v, want not_found", err)
	}
	for _, c := range et.caio.list(t, et.campaign) {
		if c.GetId() == et.pc.GetId() {
			t.Errorf("another player lists the character")
		}
	}
}

// play is asked inside the transaction, before the write (the character still has its old
// revision there), with the piece, its kind and the direction, as the caller; its refusal
// aborts the write.
func TestSetEquippedAsksPlayInsideTheTransaction(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	for _, c := range []struct {
		key   string
		kind  EquipKind
		equip bool
	}{{staff, EquipWeapon, false}, {rules.ShieldKey, EquipShield, false}, {leath, EquipArmor, false}, {leath, EquipArmor, true}} {
		before := et.dona.get(t, et.campaign, et.pc.GetId()).GetRevision()
		mustSetEquipped(t, et.dona, et.pc, c.key, c.equip)
		last := len(et.spy.changes) - 1
		if et.spy.changes[last] != (EquipChange{Key: c.key, Kind: c.kind, Equip: c.equip}) || et.spy.revisions[last] != before || et.spy.actors[last] != et.dona.id {
			t.Errorf("play was told %+v at revision %d by %q, want %+v at %d by %q", et.spy.changes[last], et.spy.revisions[last], et.spy.actors[last], c, before, et.dona.id)
		}
	}
	et.spy.fail = connect.NewError(connect.CodeFailedPrecondition, errors.New("no action left this turn"))
	before := et.dona.get(t, et.campaign, et.pc.GetId())
	_, err := setEquipped(et.dona, before, dagger, false)
	wantEditorRefusal(t, "play refuses", err, `failed_precondition: no action left this turn`)
	after := et.dona.get(t, et.campaign, et.pc.GetId())
	if after.GetRevision() != before.GetRevision() || !proto.Equal(after.GetSheet(), before.GetSheet()) {
		t.Errorf("a refusal of play wrote the sheet: revision %d -> %d", before.GetRevision(), after.GetRevision())
	}
}

// After the commit, play hears about the equipped armor and shield (Mage Armor) as it does
// for an editor save; a weapon tells it nothing.
func TestSetEquippedTellsPlayAboutTheGear(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	steps := []struct {
		key   string
		equip bool
		want  string
	}{
		{staff, false, ""},
		{rules.ShieldKey, false, "GearChanged, after GearChanged"},
		{leath, false, "GearChanged, after GearChanged"},
		{leath, true, "ArmorWorn, after ArmorWorn"},
		{leath, true, ""}, // no change
	}
	for _, s := range steps {
		mustSetEquipped(t, et.dona, et.pc, s.key, s.equip)
		if got := et.spy.take(); got != s.want {
			t.Errorf("%s equip=%v: play was told %q, want %q", s.key, s.equip, got, s.want)
		}
	}
}

// known_issues follow the gear in use: Derive reads only what is equipped, so the
// proficiency issues of a wizard in leather armor and a shield end when they are stowed
// and come back when they are equipped.
func TestSetEquippedRefreshesKnownIssues(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	issues := func() []string {
		return et.dona.get(t, et.campaign, et.pc.GetId()).GetSheet().GetFull().GetKnownIssues()
	}
	if len(issues()) != 2 {
		t.Fatalf("a wizard in leather armor with a shield has issues %v, want two", issues())
	}
	mustSetEquipped(t, et.dona, et.pc, leath, false)
	if len(issues()) != 1 {
		t.Errorf("after stowing the armor: %v, want the shield's issue only", issues())
	}
	mustSetEquipped(t, et.dona, et.pc, rules.ShieldKey, false)
	if len(issues()) != 0 {
		t.Errorf("after stowing both: %v, want none", issues())
	}
	mustSetEquipped(t, et.dona, et.pc, leath, true)
	if len(issues()) != 1 {
		t.Errorf("after equipping the armor: %v, want its issue", issues())
	}
}

// The master equips a draft character's gear; the owner's sheet reads it.
func TestSetEquippedMasterOnADraft(t *testing.T) {
	t.Parallel()
	et := newEquipTable(t)
	if got := et.dona.get(t, et.campaign, et.pc.GetId()).GetState(); got != charactersv1.CharacterState_CHARACTER_STATE_DRAFT {
		t.Fatalf("state = %v, want a draft", got)
	}
	res := mustSetEquipped(t, et.master, et.pc, dagger, false)
	if !slices.Equal(res.GetCarriedKeys(), []string{dagger}) {
		t.Errorf("carried after the master's stow = %v", res.GetCarriedKeys())
	}
	if got := et.dona.get(t, et.campaign, et.pc.GetId()).GetSheet().GetFull().GetCarriedKeys(); !slices.Equal(got, []string{dagger}) {
		t.Errorf("the owner reads carried %v", got)
	}
}

// dropStaleCarried keeps the pieces the sheet lists (armor, shield, weapons) and drops
// the rest, the empty key included.
func TestDropStaleCarried(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		f       *charactersv1.FullSheet
		carried []string
		want    []string
	}{
		{"armor listed", &charactersv1.FullSheet{ArmorKey: leath}, []string{leath}, []string{leath}},
		{"armor removed", &charactersv1.FullSheet{}, []string{leath}, nil},
		{"shield listed", &charactersv1.FullSheet{Shield: true}, []string{rules.ShieldKey}, []string{rules.ShieldKey}},
		{"shield removed", &charactersv1.FullSheet{}, []string{rules.ShieldKey}, nil},
		{"weapon listed and one removed", &charactersv1.FullSheet{WeaponKeys: []string{staff}}, []string{staff, dagger}, []string{staff}},
		{"the empty key with no armor", &charactersv1.FullSheet{}, []string{""}, nil},
	} {
		tt.f.CarriedKeys = tt.carried
		dropStaleCarried(tt.f)
		if !slices.Equal(tt.f.CarriedKeys, tt.want) {
			t.Errorf("%s: carried = %v, want %v", tt.name, tt.f.CarriedKeys, tt.want)
		}
	}
}
