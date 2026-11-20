package characters

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The five calls of InfusionService, on the fixture pack's artificer: Faísca, level 6, three live infusions at most, with a
// mace, a leather armor and a shield, and the blade, the plate, the ash cloak, the false bottom and the house seal known.

const (
	infusedDagger = "equipment:dagger"
	damageFire    = "damage-type:fire"
)

func (it *infusedTable) infuseReq(infusion string) *charactersv1.InfuseItemRequest {
	return &charactersv1.InfuseItemRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionKey: infusion, HolderCharacterId: it.pc.GetId()}
}

func (it *infusedTable) infuseOn(t *testing.T, u *user, infusion, item string) (*charactersv1.InfuseItemResponse, error) {
	t.Helper()
	req := it.infuseReq(infusion)
	req.ItemKey = item
	res, err := u.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (it *infusedTable) mustInfuseOn(t *testing.T, infusion, item string) *charactersv1.InfuseItemResponse {
	t.Helper()
	res, err := it.infuseOn(t, it.dona, infusion, item)
	if err != nil {
		t.Fatalf("InfuseItem(%s on %s) error = %v", infusion, item, err)
	}
	return res
}

func (it *infusedTable) infuseObject(t *testing.T, infusion, name string) (*charactersv1.InfuseItemResponse, error) {
	t.Helper()
	req := it.infuseReq(infusion)
	req.ObjectNamePt = name
	res, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (it *infusedTable) list(t *testing.T, u *user) (*charactersv1.ListCharacterInfusionsResponse, error) {
	t.Helper()
	res, err := u.infusions.ListCharacterInfusions(t.Context(), connect.NewRequest(&charactersv1.ListCharacterInfusionsRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId()}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (it *infusedTable) targets(t *testing.T, u *user, infusion string) (map[string]*charactersv1.InfusionTarget, []*charactersv1.InfusionTarget) {
	t.Helper()
	res, err := u.infusions.ListInfusionTargets(t.Context(), connect.NewRequest(&charactersv1.ListInfusionTargetsRequest{
		CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionKey: infusion, HolderCharacterId: it.pc.GetId(),
	}))
	if err != nil {
		t.Fatalf("ListInfusionTargets(%s) error = %v", infusion, err)
	}
	by := map[string]*charactersv1.InfusionTarget{}
	for _, tg := range res.Msg.GetTargets() {
		by[tg.GetItemKey()] = tg
	}
	return by, res.Msg.GetTargets()
}

// infusionReason is the InfusionBlocked reason of a failed_precondition.
func infusionReason(t *testing.T, call string, err error) charactersv1.InfusionBlockedReason {
	t.Helper()
	wantCode(t, call, err, connect.CodeFailedPrecondition)
	ce, _ := errors.AsType[*connect.Error](err)
	for _, d := range ce.Details() {
		v, derr := d.Value()
		if derr != nil {
			t.Fatalf("decode error detail: %v", derr)
		}
		if detail, ok := v.(*charactersv1.InfusionBlocked); ok {
			return detail.GetReason()
		}
	}
	t.Fatalf("%s error %v has no InfusionBlocked detail", call, err)
	return 0
}

func wantInfusionRefusal(t *testing.T, call string, err error, want charactersv1.InfusionBlockedReason) {
	t.Helper()
	if got := infusionReason(t, call, err); got != want {
		t.Errorf("%s reason = %v, want %v", call, got, want)
	}
}

const (
	notKnown     = charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_INFUSION_NOT_KNOWN
	notAccepted  = charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_NOT_ACCEPTED
	alreadyHeld  = charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_ALREADY_INFUSED
	itemChangedR = charactersv1.InfusionBlockedReason_INFUSION_BLOCKED_REASON_ITEM_CHANGED
)

// withPicks sets the infusions the artificer picked (the fixture has five at level 6; the focus is not among them).
func withPicks(keys ...string) func(*charactersv1.FullSheet) {
	return func(f *charactersv1.FullSheet) { f.FeatureChoiceKeys = keys }
}

// twoDaggers gives the sheet two copies of the dagger: the editor refuses a repeated weapon, so an older sheet is imitated.
func (it *infusedTable) twoDaggers(t *testing.T) {
	t.Helper()
	if _, err := it.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,weapon_keys}', '["equipment:mace","equipment:dagger","equipment:dagger"]') WHERE id = $1`, it.pc.GetId()); err != nil {
		t.Fatal(err)
	}
}

func liveRecords(t *testing.T, it *infusedTable) []string {
	t.Helper()
	rows, err := it.h.svc.queries.ListLiveInfusionsOfHolder(t.Context(), it.pc.GetId())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.InfusionKey)
	}
	return out
}

func endedReason(t *testing.T, it *infusedTable, id string) string {
	t.Helper()
	r := it.record(t, id)
	if r.EndedReason == nil {
		return ""
	}
	return *r.EndedReason
}

// The owner and the master read what the artificer keeps and knows; another player and a stranger are refused.
func TestListCharacterInfusionsReadsWhatIsKeptAndKnown(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	for _, u := range []*user{it.dona, it.master} {
		got, err := it.list(t, u)
		if err != nil {
			t.Fatalf("ListCharacterInfusions() error = %v", err)
		}
		if len(got.GetActive()) != 0 || len(got.GetKnown()) != 5 || got.GetLimit() != 3 || got.GetUsed() != 0 {
			t.Errorf("a fresh artificer: active %d, known %d, limit %d, used %d; want 0, 5, 3, 0", len(got.GetActive()), len(got.GetKnown()), got.GetLimit(), got.GetUsed())
		}
	}
	it.infuse(t, infusionKeyBlade, infusionMace)
	got, err := it.list(t, it.dona)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetActive()) != 1 || got.GetActive()[0].GetBonus() != 1 || len(got.GetKnown()) != 4 || got.GetUsed() != 1 {
		t.Errorf("one infusion live: active %v, known %d, used %d; want it listed with +1, 4 known, 1 used", got.GetActive(), len(got.GetKnown()), got.GetUsed())
	}
	for _, k := range got.GetKnown() {
		if k.GetInfusionKey() == infusionKeyBlade {
			t.Errorf("the blade is live and still listed as known")
		}
	}
	var ash *charactersv1.KnownInfusion
	for _, k := range got.GetKnown() {
		if k.GetInfusionKey() == infusionKeyAsh {
			ash = k
		}
	}
	if ash == nil || ash.GetNamePt() != "Manto de Cinza" || ash.GetAccepts().String() != "INFUSION_ACCEPTS_ARMOR" || !ash.GetRequiresAttunement() || !ash.GetComputed() ||
		len(ash.GetParameters()) != 4 || ash.GetParameters()[0].GetKey() != damageFire || ash.GetParameters()[0].GetNamePt() == "" || ash.GetDescriptionPt() == "" {
		t.Errorf("the ash cloak = %v, want its name, armor, attunement, computed, four damage types with names and a text", ash)
	}
	for _, c := range []struct {
		who  string
		u    *user
		want connect.Code
	}{{"another player", it.caio, connect.CodeNotFound}, {"a stranger", it.visitor, connect.CodeNotFound}} {
		_, err := it.list(t, c.u)
		wantCode(t, "ListCharacterInfusions as "+c.who, err, c.want)
		_, err = c.u.infusions.ListInfusionTargets(t.Context(), connect.NewRequest(&charactersv1.ListInfusionTargetsRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionKey: infusionKeyBlade, HolderCharacterId: it.pc.GetId()}))
		wantCode(t, "ListInfusionTargets as "+c.who, err, c.want)
	}
}

// The pieces of another holder are not listed: in this version the holder is the infuser.
func TestListInfusionTargetsOfAnotherHolderIsRefused(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	_, err := it.dona.infusions.ListInfusionTargets(t.Context(), connect.NewRequest(&charactersv1.ListInfusionTargetsRequest{
		CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionKey: infusionKeyBlade, HolderCharacterId: uuid.New().String(),
	}))
	wantCode(t, "ListInfusionTargets(another holder)", err, connect.CodeInvalidArgument)
}

// The targets of an infusion the character does not know (the table has it and the sheet did not pick it) are the answer of
// a key that is no infusion: same code, same empty list.
func TestListInfusionTargetsOfAnInfusionTheCharacterDoesNotKnow(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	ask := func(key string) (*charactersv1.ListInfusionTargetsResponse, error) {
		res, err := it.dona.infusions.ListInfusionTargets(t.Context(), connect.NewRequest(&charactersv1.ListInfusionTargetsRequest{
			CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionKey: key, HolderCharacterId: it.pc.GetId(),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	if _, rows := it.targets(t, it.dona, infusionKeyBlade); len(rows) == 0 {
		t.Fatal("a known infusion lists no piece, want rows (the positive control)")
	}
	want, wantErr := ask("feature:no-such-infusion")
	got, err := ask(infusionKeyFocus)
	if (err == nil) != (wantErr == nil) || (err != nil && err.Error() != wantErr.Error()) {
		t.Fatalf("the infusion the character does not know answers error %v, want %v", err, wantErr)
	}
	if !proto.Equal(got, want) || len(got.GetTargets()) != 0 {
		t.Errorf("the infusion the character does not know answers %v, want %v", got, want)
	}
}

// The target list has a row for each gear key with the reason it cannot take the infusion, in the order wrong kind, already
// magical, taken; two equal pieces are one row with two copies; an infusion for objects gets the row of the object by name.
func TestListInfusionTargetsGivesEachPieceItsReason(t *testing.T) {
	t.Parallel()
	it := newInfusedTableWith(t, func(f *charactersv1.FullSheet) {
		f.WeaponKeys = []string{infusionMace, infusedDagger}
		f.WeaponMarks = []*charactersv1.WeaponMark{{WeaponKey: infusionMace, Magical: true}}
		f.ItemBonuses = []*charactersv1.ItemBonus{{Key: infusionShieldGear, Bonus: 2}}
	})
	it.twoDaggers(t)
	by, rows := it.targets(t, it.dona, infusionKeyBlade)
	if len(rows) != 4 {
		t.Fatalf("the blade lists %d rows, want armor, shield and two weapons = 4: %v", len(rows), rows)
	}
	want := map[string]struct {
		reason charactersv1.InfusionTargetReason
		text   string
		copies int32
	}{
		infusionMace:       {charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_ALREADY_MAGICAL, "Já é um item mágico: a infusão pede um item comum.", 1},
		infusedDagger:      {charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_UNSPECIFIED, "", 2},
		infusionLeather:    {charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_WRONG_KIND, "Pede uma arma.", 1},
		infusionShieldGear: {charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_WRONG_KIND, "Pede uma arma.", 1},
	}
	for k, w := range want {
		got := by[k]
		if got == nil || got.GetReason() != w.reason || got.GetReasonPt() != w.text || got.GetCopies() != w.copies || got.GetNamePt() == "" || got.GetKindPt() == "" {
			t.Errorf("row of %s = %v, want reason %v %q with %d copies", k, got, w.reason, w.text, w.copies)
		}
	}
	// A plate goes on armor or a shield: the shield has a manual +2 (already magical, with its number), the armor is free.
	by, _ = it.targets(t, it.dona, infusionKeyPlate)
	if r := by[infusionShieldGear]; r.GetReason() != charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_ALREADY_MAGICAL || r.GetReasonPt() != "Já é um item mágico (+2): a infusão pede um item comum." {
		t.Errorf("the shield with a +2 = %v, want already magical (+2)", r)
	}
	if r := by[infusionMace]; r.GetReason() != charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_WRONG_KIND || r.GetReasonPt() != "Pede uma armadura ou um escudo." {
		t.Errorf("the mace for the plate = %v", r)
	}
	// A piece that carries an infusion is taken, and the sentence names it.
	it.infuse(t, infusionKeyPlate, infusionLeather)
	by, _ = it.targets(t, it.dona, infusionKeyAsh)
	if r := by[infusionLeather]; r.GetReason() != charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_TAKEN || r.GetReasonPt() != "Já tem Couraça de Pétalas: um item guarda uma infusão só." {
		t.Errorf("the armor with an infusion = %v, want taken, naming the plate", r)
	}
	if r := by[infusionShieldGear]; r.GetReason() != charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_WRONG_KIND || r.GetReasonPt() != "Pede uma armadura." {
		t.Errorf("the shield for the cloak = %v", r)
	}
}

// An infusion that goes on any object gets, besides the gear rows (none of which it takes), one row with no item key and no
// reason: the object the player names.
func TestListInfusionTargetsOffersTheObjectByName(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	by, rows := it.targets(t, it.dona, infusionKeyHouse)
	last := rows[len(rows)-1]
	if last.GetItemKey() != "" || last.GetReason() != charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_UNSPECIFIED || last.GetKindPt() != "objeto" {
		t.Errorf("the last row = %v, want the object by name: no key, no reason", last)
	}
	if r := by[infusionMace]; r.GetReason() != charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_WRONG_KIND || r.GetReasonPt() == "" {
		t.Errorf("the mace for the house seal = %v, want a wrong kind with its sentence", r)
	}
	_, rows = it.targets(t, it.dona, infusionKeyBlade)
	for _, r := range rows {
		if r.GetItemKey() == "" {
			t.Errorf("the blade offers the row of an object: %v", r)
		}
	}
}

// InfuseItem puts a known infusion on a piece: the answer is the infusion as the sheet reads it, the sheet reads the +1, the
// revision goes up and the master may do it too.
func TestInfuseItemPutsTheInfusionOnThePiece(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	before := it.read(t)
	res := it.mustInfuseOn(t, infusionKeyBlade, infusionMace)
	in := res.GetInfusion()
	if in.GetInfusionKey() != infusionKeyBlade || in.GetItemKey() != infusionMace || in.GetBonus() != 1 || !in.GetValidNow() || in.GetHolderCharacterId() != it.pc.GetId() || res.GetEndedInfusionId() != "" {
		t.Errorf("InfuseItem() = %v, want the blade on the mace with +1 and nothing ended", res)
	}
	after := it.read(t)
	if a := attackOf(after, infusionMace); a == nil || a.GetMagicBonus() != 1 || a.GetInfusionNamePt() != "Gume Vivo" {
		t.Errorf("the mace after = %v, want +1 and the infusion's name", a)
	}
	if after.GetRevision() <= before.GetRevision() {
		t.Errorf("revision %d after %d: an infusion must make an open sheet read again", after.GetRevision(), before.GetRevision())
	}
	if _, err := it.infuseOn(t, it.master, infusionKeyFalse, infusionShieldGear); err != nil {
		t.Errorf("the master infuses: %v", err)
	}
	if _, err := it.infuseOn(t, it.caio, infusionKeyHouse, ""); err == nil || connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another player infuses: %v, want not_found", err)
	}
}

// Every refusal of InfuseItem, with its code and reason.
func TestInfuseItemRefusals(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t) // the picks of the fixture: the focus and the thrown weapon are not among them
	it.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
	cases := []struct {
		name   string
		mutate func(*charactersv1.InfuseItemRequest)
		reason charactersv1.InfusionBlockedReason
		code   connect.Code
	}{
		{"an infusion the character did not pick", func(r *charactersv1.InfuseItemRequest) { r.InfusionKey, r.ItemKey = infusionKeyFocus, "" }, notKnown, 0},
		{"a key the content never had", func(r *charactersv1.InfuseItemRequest) { r.InfusionKey, r.ItemKey = "feature:nothing", infusionMace }, notKnown, 0},
		{"a piece of the wrong kind", func(r *charactersv1.InfuseItemRequest) { r.InfusionKey, r.ItemKey = infusionKeyBlade, infusionLeather }, notAccepted, 0},
		{"a piece the sheet does not list", func(r *charactersv1.InfuseItemRequest) { r.InfusionKey, r.ItemKey = infusionKeyBlade, infusedDagger }, notAccepted, 0},
		{"an object for an infusion that takes none", func(r *charactersv1.InfuseItemRequest) { r.InfusionKey, r.ObjectNamePt = infusionKeyBlade, "Lanterna" }, notAccepted, 0},
		{"a piece for an infusion that takes objects", func(r *charactersv1.InfuseItemRequest) { r.InfusionKey, r.ItemKey = infusionKeyHouse, infusionMace }, notAccepted, 0},
		{"a piece that holds an infusion", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey = infusionKeyPlate, infusionShieldGear
		}, alreadyHeld, 0},
		{"an infusion that is live already", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey = infusionKeyFalse, infusionShieldGear
		}, itemChangedR, 0},
		{"an end that is not a record", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey, r.EndInfusionId = infusionKeyBlade, infusionMace, uuid.New().String()
		}, itemChangedR, 0},
		{"a damage type the cloak does not offer", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey, r.Parameter = infusionKeyAsh, infusionLeather, "damage-type:poison"
		}, 0, connect.CodeInvalidArgument},
		{"no damage type for the cloak", func(r *charactersv1.InfuseItemRequest) { r.InfusionKey, r.ItemKey = infusionKeyAsh, infusionLeather }, 0, connect.CodeInvalidArgument},
		{"a damage type for the blade", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey, r.Parameter = infusionKeyBlade, infusionMace, damageFire
		}, 0, connect.CodeInvalidArgument},
		{"an object without a name", func(r *charactersv1.InfuseItemRequest) { r.InfusionKey, r.ObjectNamePt = infusionKeyHouse, "  " }, 0, connect.CodeInvalidArgument},
		{"an object name of 41 characters", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ObjectNamePt = infusionKeyHouse, strings.Repeat("ã", 41)
		}, 0, connect.CodeInvalidArgument},
		{"a piece and an object name", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey, r.ObjectNamePt = infusionKeyBlade, infusionMace, "Lanterna"
		}, 0, connect.CodeInvalidArgument},
		{"another holder", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey, r.HolderCharacterId = infusionKeyBlade, infusionMace, uuid.New().String()
		}, 0, connect.CodeInvalidArgument},
		{"a key that is no UUID", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey, r.IdempotencyKey = infusionKeyBlade, infusionMace, "not-a-uuid"
		}, 0, connect.CodeInvalidArgument},
		{"attunement for an infusion that asks none", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey, r.AttuneNow = infusionKeyBlade, infusionMace, true
		}, 0, connect.CodeInvalidArgument},
		{"a stale revision", func(r *charactersv1.InfuseItemRequest) {
			r.InfusionKey, r.ItemKey, r.Revision = infusionKeyBlade, infusionMace, 1
		}, 0, connect.CodeAborted},
	}
	for _, c := range cases {
		req := it.infuseReq("")
		c.mutate(req)
		_, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
		if c.reason != 0 {
			wantInfusionRefusal(t, c.name, err, c.reason)
		} else {
			wantCode(t, c.name, err, c.code)
		}
	}
	if got := liveRecords(t, it); len(got) != 1 {
		t.Errorf("the refusals left %d live records, want only the false bottom: %v", len(got), got)
	}
	unknown := it.infuseReq(infusionKeyBlade)
	unknown.CharacterId = uuid.New().String()
	unknown.HolderCharacterId = unknown.CharacterId
	_, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(unknown))
	wantCode(t, "a character that does not exist", err, connect.CodeNotFound)
}

// A dead artificer infuses nothing, as a dead character equips nothing; a locked sheet does not stop an infusion.
func TestInfuseItemOnADeadAndOnALockedSheet(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	it.h.lockSheets(it.campaign)
	if _, err := it.infuseOn(t, it.dona, infusionKeyBlade, infusionMace); err != nil {
		t.Fatalf("a locked sheet stops the infusion: %v", err)
	}
	if _, err := it.master.api.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	for _, u := range []*user{it.dona, it.master} {
		_, err := it.infuseOn(t, u, infusionKeyFalse, infusionShieldGear)
		if d := blocked(t, "InfuseItem(dead)", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD {
			t.Errorf("InfuseItem(dead) reason = %v, want CHARACTER_DEAD", d.GetReason())
		}
	}
	if _, err := it.list(t, it.dona); err != nil {
		t.Errorf("a dead character's infusions are still read: %v", err)
	}
}

// At the limit a new infusion ends the one the caller names, or the oldest, with `replaced`, and the id comes back.
func TestInfuseItemOverTheLimitEndsOne(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	first := it.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion().GetId()
	it.h.clock.Advance(time.Minute)
	second := it.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear).GetInfusion().GetId()
	it.h.clock.Advance(time.Minute)
	third, err := it.infuseObject(t, infusionKeyHouse, "Lanterna")
	if err != nil || third.GetEndedInfusionId() != "" {
		t.Fatalf("the third infusion = %v, %v, want it under the limit and nothing ended", third, err)
	}
	it.h.clock.Advance(time.Minute)

	// Named: the second ends, not the oldest.
	req := it.infuseReq(infusionKeyPlate)
	req.ItemKey, req.EndInfusionId = infusionLeather, second
	res, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
	if err != nil || res.Msg.GetEndedInfusionId() != second {
		t.Fatalf("with the second named: %v, %v, want it ended", res, err)
	}
	if endedReason(t, it, second) != "replaced" || endedReason(t, it, first) != "" {
		t.Errorf("reasons: second %q, first %q; want replaced and live", endedReason(t, it, second), endedReason(t, it, first))
	}
	if c, _ := it.list(t, it.dona); c.GetUsed() != 3 || c.GetLimit() != 3 {
		t.Errorf("used %d of %d, want 3 of 3", c.GetUsed(), c.GetLimit())
	}

	// Not named: the oldest (the blade) ends.
	it.h.clock.Advance(time.Minute)
	last := it.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
	if last.GetEndedInfusionId() != first || endedReason(t, it, first) != "replaced" {
		t.Errorf("without a name %q ended (%q), want the oldest, %s, replaced", last.GetEndedInfusionId(), endedReason(t, it, first), first)
	}
	if live := liveRecords(t, it); len(live) != 3 {
		t.Errorf("live = %v, want 3", live)
	}
	// A record the infuser does not have live cannot be named, even over the limit.
	it.h.clock.Advance(time.Minute)
	req = it.infuseReq(infusionKeyBlade)
	req.ItemKey, req.EndInfusionId = infusionMace, second
	_, err = it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
	wantInfusionRefusal(t, "naming an ended record", err, itemChangedR)
}

func (it *infusedTable) mustEnd(t *testing.T, id string) {
	t.Helper()
	if _, err := it.dona.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: id})); err != nil {
		t.Fatalf("EndInfusion(%s) error = %v", id, err)
	}
}

func (it *infusedTable) move(u *user, id, item, object string) (*charactersv1.MoveInfusionResponse, error) {
	res, err := u.infusions.MoveInfusion(it.h.t.Context(), connect.NewRequest(&charactersv1.MoveInfusionRequest{
		CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: id, ItemKey: item, ObjectNamePt: object,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// attune_now attunes through the checks of SetAttunement: outside the window the whole infusion is refused and writes
// nothing, inside it the record is attuned and the window spent, over three items it is refused, and the master has no
// window to wait for.
func TestInfuseItemAttuneNow(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	ash := func(u *user) (*charactersv1.InfuseItemResponse, error) {
		req := it.infuseReq(infusionKeyAsh)
		req.ItemKey, req.Parameter, req.AttuneNow = infusionLeather, damageFire, true
		res, err := u.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	_, err := ash(it.dona)
	if d := blocked(t, "attune_now without a window", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ATTUNEMENT_NEEDS_REST {
		t.Errorf("reason = %v, want ATTUNEMENT_NEEDS_REST", d.GetReason())
	}
	if live := liveRecords(t, it); len(live) != 0 {
		t.Fatalf("a refused attunement left %v live: nothing may be written", live)
	}
	it.openWindow(t, it.pc.GetId(), true)
	res, err := ash(it.dona)
	if err != nil || !res.GetInfusion().GetAttuned() || !it.record(t, res.GetInfusion().GetId()).Attuned {
		t.Fatalf("attune_now in the window = %v, %v, want an attuned record", res, err)
	}
	if open, _ := it.h.svc.queries.GetAttunementOpen(t.Context(), it.pc.GetId()); open {
		t.Error("the window was not spent")
	}

	// Over three: the cloak is the third item here (two rings), a fourth is refused to the master as well.
	it2 := newInfusedTable(t)
	pc := it2.withMagicItems(t, ringA, ringB, ringC)
	var perr error
	for _, k := range []string{ringA, ringB, ringC} {
		if pc, perr = it2.attune(it2.master, pc, k, true); perr != nil {
			t.Fatalf("attune %s: %v", k, perr)
		}
	}
	req := it2.infuseReq(infusionKeyAsh)
	req.ItemKey, req.Parameter, req.AttuneNow = infusionLeather, damageFire, true
	_, err = it2.master.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
	if d := blocked(t, "attune_now over three", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ATTUNEMENT_LIMIT {
		t.Errorf("reason = %v, want ATTUNEMENT_LIMIT", d.GetReason())
	}
	if live := liveRecords(t, it2); len(live) != 0 {
		t.Errorf("a refused attunement left %v live", live)
	}
	// With room, the master needs no window.
	it3 := newInfusedTable(t)
	m, err := ash2(it3)
	if err != nil || !m.GetInfusion().GetAttuned() {
		t.Errorf("the master attunes with no window: %v, %v", m, err)
	}
}

func ash2(it *infusedTable) (*charactersv1.InfuseItemResponse, error) {
	req := it.infuseReq(infusionKeyAsh)
	req.ItemKey, req.Parameter, req.AttuneNow = infusionLeather, damageFire, true
	res, err := it.master.infusions.InfuseItem(it.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// An object is chosen by name: the record has its own key, and two objects of one name are allowed.
func TestInfuseItemOnObjectsByName(t *testing.T) {
	t.Parallel()
	it := newInfusedTableWith(t, withPicks(infusionKeyBlade, infusionKeyFocus, infusionKeyAsh, infusionKeyFalse, infusionKeyHouse))
	a, err := it.infuseObject(t, infusionKeyHouse, "Lanterna")
	if err != nil {
		t.Fatalf("the house seal on an object: %v", err)
	}
	b, err := it.infuseObject(t, infusionKeyFocus, "Lanterna")
	if err != nil {
		t.Fatalf("a second object with the same name: %v", err)
	}
	if a.GetInfusion().GetId() == b.GetInfusion().GetId() || a.GetInfusion().GetItemKind() != charactersv1.InfusionItemKind_INFUSION_ITEM_KIND_OBJECT ||
		a.GetInfusion().GetObjectNamePt() != "Lanterna" || a.GetInfusion().GetItemNamePt() != "Lanterna" || a.GetInfusion().GetItemKey() != "" {
		t.Errorf("the objects = %v and %v", a, b)
	}
	if ra, rb := it.record(t, a.GetInfusion().GetId()), it.record(t, b.GetInfusion().GetId()); ra.ItemKey == rb.ItemKey || ra.ItemKey != objectItemKey(ra.ID) {
		t.Errorf("object keys = %q and %q, want one made of each record's id", ra.ItemKey, rb.ItemKey)
	}
}

// MoveInfusion: the record goes to another piece with its time begun again and its attunement cleared; a record that ended
// is refused; the piece must be accepted; an infusion not valid now on the piece ends with `taken`; moving to where it is
// changes nothing.
func TestMoveInfusion(t *testing.T) {
	t.Parallel()
	it := newInfusedTableWith(t, func(f *charactersv1.FullSheet) { f.WeaponKeys = []string{infusionMace, infusedDagger} })
	blade := it.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion()
	oldSince := it.record(t, blade.GetId()).Since
	it.h.clock.Advance(time.Hour)
	before := it.read(t).GetRevision()

	_, err := it.move(it.dona, blade.GetId(), infusionLeather, "")
	wantInfusionRefusal(t, "move to the wrong kind", err, notAccepted)
	_, err = it.move(it.dona, blade.GetId(), "", "Lanterna")
	wantInfusionRefusal(t, "move to an object", err, notAccepted)
	_, err = it.move(it.dona, blade.GetId(), "", "")
	wantCode(t, "move to nothing", err, connect.CodeInvalidArgument)

	// A record that is not valid now (its infusion left the content) sits on the dagger: the move takes the piece and ends it.
	ghost, gerr := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: it.pc.GetId(), InfusionKey: "feature:no-longer-there", ItemKind: infusionItemGear, GearKey: infusedDagger,
	})
	if gerr != nil {
		t.Fatal(gerr)
	}
	res, err := it.move(it.dona, blade.GetId(), infusedDagger, "")
	if err != nil || res.GetInfusion().GetItemKey() != infusedDagger || !res.GetInfusion().GetValidNow() || !res.GetInfusion().GetSince().AsTime().After(oldSince) {
		t.Fatalf("MoveInfusion() = %v, %v, want the blade on the dagger, valid, with a later time", res, err)
	}
	if endedReason(t, it, ghost.ID) != "taken" {
		t.Errorf("the record that was not valid ended %q, want taken", endedReason(t, it, ghost.ID))
	}
	if it.read(t).GetRevision() <= before {
		t.Error("the revision did not go up")
	}
	if a := attackOf(it.read(t), infusedDagger); a == nil || a.GetMagicBonus() != 1 {
		t.Errorf("the dagger after the move = %v, want +1", a)
	}
	if a := attackOf(it.read(t), infusionMace); a == nil || a.GetMagicBonus() != 0 {
		t.Errorf("the mace after the move = %v, want no bonus", a)
	}
	// Where it is already: nothing changes.
	again, err := it.move(it.dona, blade.GetId(), infusedDagger, "")
	if err != nil || again.GetInfusion().GetSince().AsTime() != res.GetInfusion().GetSince().AsTime() {
		t.Errorf("moving to where it is = %v, %v, want the same record untouched", again, err)
	}
	// A piece that holds a valid infusion is not taken.
	it.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
	_, err = it.move(it.dona, blade.GetId(), infusionShieldGear, "")
	wantInfusionRefusal(t, "move to a piece with an infusion", err, notAccepted) // the shield is no weapon: wrong kind comes first
	// End it, then move it.
	it.mustEnd(t, blade.GetId())
	_, err = it.move(it.dona, blade.GetId(), infusionMace, "")
	wantInfusionRefusal(t, "move an ended record", err, itemChangedR)
	_, err = it.move(it.dona, uuid.New().String(), infusionMace, "")
	wantInfusionRefusal(t, "move what does not exist", err, itemChangedR)
	_, err = it.move(it.caio, blade.GetId(), infusionMace, "")
	wantCode(t, "another player moves", err, connect.CodeNotFound)
}

// A move clears the attunement of the record, and a move onto an object renames it.
func TestMoveInfusionClearsAttunementAndRenamesObjects(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	it.openWindow(t, it.pc.GetId(), true)
	req := it.infuseReq(infusionKeyAsh)
	req.ItemKey, req.Parameter, req.AttuneNow = infusionLeather, damageFire, true
	cloak, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	_ = cloak
	obj, err := it.infuseObject(t, infusionKeyHouse, "Lanterna")
	if err != nil {
		t.Fatal(err)
	}
	res, err := it.move(it.dona, obj.GetInfusion().GetId(), "", "Lampião")
	if err != nil || res.GetInfusion().GetObjectNamePt() != "Lampião" || res.GetInfusion().GetItemKind() != charactersv1.InfusionItemKind_INFUSION_ITEM_KIND_OBJECT {
		t.Fatalf("renaming the object = %v, %v", res, err)
	}
	// The cloak is armor: moved to the shield it is refused; it stays attuned.
	_, err = it.move(it.dona, cloak.Msg.GetInfusion().GetId(), infusionShieldGear, "")
	wantInfusionRefusal(t, "the cloak on a shield", err, notAccepted)
	if !it.record(t, cloak.Msg.GetInfusion().GetId()).Attuned {
		t.Error("a refused move cleared the attunement")
	}
}

// EndInfusion ends a live record with `ended`; a record that ended, one that is not there and one of another artificer are
// ITEM_CHANGED.
func TestEndInfusion(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	blade := it.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion()
	before := it.read(t).GetRevision()
	res, err := it.master.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: blade.GetId()}))
	if err != nil || res.Msg.GetEndedInfusionId() != blade.GetId() || endedReason(t, it, blade.GetId()) != "ended" {
		t.Fatalf("EndInfusion() = %v, %v, want the id and the reason ended", res, err)
	}
	if after := it.read(t); after.GetRevision() <= before || attackOf(after, infusionMace).GetMagicBonus() != 0 {
		t.Error("the sheet still has the bonus or the old revision")
	}
	_, err = it.dona.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: blade.GetId()}))
	wantInfusionRefusal(t, "end it twice", err, itemChangedR)
	// Another artificer's record.
	other := it.caio.create(t, it.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Brisa", pensantusSheet())
	foreign, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: other.GetId(), HolderID: other.GetId(), InfusionKey: infusionKeyBlade, ItemKind: infusionItemObject, ObjectName: "Tocha",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = it.dona.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: foreign.ID}))
	wantInfusionRefusal(t, "end the record of another artificer", err, itemChangedR)
	_, err = it.dona.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId()}))
	wantCode(t, "end with no id", err, connect.CodeInvalidArgument)
	_, err = it.caio.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: foreign.ID}))
	wantCode(t, "another player ends", err, connect.CodeNotFound)
	if endedReason(t, it, foreign.ID) != "" {
		t.Error("a refused end touched the record of another artificer")
	}
}

// The same key with the same request is the first answer and writes nothing twice, even after the record ended; the same key
// with another request is refused.
func TestInfuseItemIsIdempotent(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	key := uuid.New().String()
	req := it.infuseReq(infusionKeyBlade)
	req.ItemKey, req.IdempotencyKey = infusionMace, key
	call := func(r *charactersv1.InfuseItemRequest) (*charactersv1.InfuseItemResponse, error) {
		res, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(r))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	first, err := call(req)
	if err != nil {
		t.Fatal(err)
	}
	revision := it.read(t).GetRevision()
	again, err := call(req)
	if err != nil || again.GetInfusion().GetId() != first.GetInfusion().GetId() || again.GetEndedInfusionId() != first.GetEndedInfusionId() {
		t.Fatalf("the retry = %v, %v, want the first answer %v", again, err, first)
	}
	if n := len(liveRecords(t, it)); n != 1 || it.read(t).GetRevision() != revision {
		t.Errorf("the retry wrote: %d live records, revision %d (was %d)", n, it.read(t).GetRevision(), revision)
	}
	// After the record ended, the retry still answers the first call and makes no record.
	it.mustEnd(t, first.GetInfusion().GetId())
	late, err := call(req)
	if err != nil || late.GetInfusion().GetId() != first.GetInfusion().GetId() || late.GetInfusion().GetValidNow() {
		t.Errorf("a late retry = %v, %v, want the first record, no longer valid", late, err)
	}
	if n := len(liveRecords(t, it)); n != 0 {
		t.Errorf("a late retry made a record: %d live", n)
	}
	other := it.infuseReq(infusionKeyFalse)
	other.ItemKey, other.IdempotencyKey = infusionShieldGear, key
	_, err = call(other)
	wantCode(t, "the same key for another infusion", err, connect.CodeInvalidArgument)
}

// secondArtificer makes a second artificer of the table, Caio's, with the same picks as the first.
func (it *infusedTable) secondArtificer(t *testing.T) *charactersv1.Character {
	t.Helper()
	sheet := choiceSheet("race:human", "", infusionClassKey, "", 6,
		infusionKeyBlade, infusionKeyPlate, infusionKeyAsh, infusionKeyFalse, infusionKeyHouse)
	sheet.GetFull().WeaponKeys = []string{infusionMace}
	sheet.GetFull().ArmorKey = infusionLeather
	sheet.GetFull().SkillProficiencyKeys = []string{"skill:arcana", "skill:history"}
	return it.caio.create(t, it.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Brisa", sheet)
}

// The same key used at the same moment for two characters of one campaign: one call wins, the other is answered as a key used
// for another change, never as an internal error.
func TestInfuseItemTheSameKeyForTwoCharactersAtOnce(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 6)
	it := newInfusedTable(t)
	other := it.secondArtificer(t)
	key := uuid.New().String()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, c := range []struct {
		u  *user
		id string
	}{{it.dona, it.pc.GetId()}, {it.caio, other.GetId()}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := &charactersv1.InfuseItemRequest{CampaignId: it.campaign, CharacterId: c.id, HolderCharacterId: c.id, InfusionKey: infusionKeyBlade, ItemKey: infusionMace, IdempotencyKey: key}
			_, errs[i] = c.u.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case connect.CodeOf(err) != connect.CodeInvalidArgument || !strings.Contains(err.Error(), "idempotency_key was used for another change"):
			t.Errorf("the second call with the key failed with %v, want the refusal of a reused key", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d of two calls with one key succeeded (%v), want exactly one", ok, errs)
	}
}

// The answer kept under a key that another call kept first (the row is there when this one writes) is the refusal of a reused key.
func TestKeepInfusionRequestOfAKeyKeptMeanwhile(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	rec := it.infuse(t, infusionKeyBlade, infusionMace)
	key := uuid.New().String()
	hash := "first"
	if err := it.h.svc.keepInfusionRequest(t.Context(), it.h.svc.queries, it.campaign, key, &hash, rec.ID, ""); err != nil {
		t.Fatal(err)
	}
	hash = "second"
	err := it.h.svc.keepInfusionRequest(t.Context(), it.h.svc.queries, it.campaign, key, &hash, rec.ID, "")
	if connect.CodeOf(err) != connect.CodeInvalidArgument || !strings.Contains(err.Error(), "idempotency_key was used for another change") {
		t.Errorf("keepInfusionRequest() = %v, want the refusal of a reused key", err)
	}
}

// Two infusions that would both pass the limit cannot both succeed: each locks the artificer's row before it counts, so the
// second one reads the first's result. Two that name the same record to end: one wins, the other has nothing to end.
func TestInfuseItemTwoAtTheLimitCannotBothEndTheSameRecord(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 6)
	it := newInfusedTable(t)
	first := it.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion().GetId()
	it.h.clock.Advance(time.Minute)
	it.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
	it.h.clock.Advance(time.Minute)
	if _, err := it.infuseObject(t, infusionKeyHouse, "Lanterna"); err != nil {
		t.Fatal(err)
	}
	it.h.clock.Advance(time.Minute)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, c := range []struct{ infusion, item, parameter string }{{infusionKeyPlate, infusionLeather, ""}, {infusionKeyAsh, infusionLeather, damageFire}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := it.infuseReq(c.infusion)
			req.ItemKey, req.Parameter, req.EndInfusionId = c.item, c.parameter, first
			_, errs[i] = it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Errorf("%d of two concurrent infusions succeeded (%v), want exactly one", ok, errs)
	}
	if live := liveRecords(t, it); len(live) != 3 {
		t.Errorf("live = %v, want the limit of 3", live)
	}
}

// Two infusions on different free places at the limit, with one oldest record to end: each locks the artificer's row, so
// the second reads the first's result. Each one succeeds or is aborted by the database, and the count is the limit.
func TestInfuseItemTwoOnDifferentPlacesAtTheLimit(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 6)
	it := newInfusedTableWith(t, withPicks(infusionKeyFalse, infusionKeyHouse, infusionKeyFocus, infusionKeyBlade, infusionKeyPlate))
	it.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
	it.h.clock.Advance(time.Minute)
	if _, err := it.infuseObject(t, infusionKeyHouse, "Lanterna"); err != nil {
		t.Fatal(err)
	}
	it.h.clock.Advance(time.Minute)
	if _, err := it.infuseObject(t, infusionKeyFocus, "Cajado"); err != nil {
		t.Fatal(err)
	}
	it.h.clock.Advance(time.Minute)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, c := range []struct{ infusion, item string }{{infusionKeyBlade, infusionMace}, {infusionKeyPlate, infusionLeather}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = it.infuseOn(t, it.dona, c.infusion, c.item)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case connect.CodeOf(err) != connect.CodeAborted:
			t.Errorf("a concurrent infusion failed with %v, want success or aborted", err)
		}
	}
	if ok == 0 {
		t.Errorf("no concurrent infusion succeeded: %v", errs)
	}
	if live := liveRecords(t, it); len(live) != 3 {
		t.Errorf("live = %v after two concurrent infusions (%v), want exactly the limit of 3", live, errs)
	}
}

// The name of an object is cleaned as the app's names are: control, hidden and bidi characters are refused on
// object_name_pt, and a name typed with a combining mark is kept in its composed form.
func TestInfuseItemObjectNameIsCleaned(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	for name, text := range map[string]string{
		"a control character": "Lan\x07terna",
		"a NUL":               "Lan\x00terna",
		"a newline":           "Lanterna\nvelha",
		"a zero-width space":  "Lan\u200bterna",
		"a bidi override":     "Lan\u202eterna",
	} {
		_, err := it.infuseObject(t, infusionKeyHouse, text)
		wantCode(t, name, err, connect.CodeInvalidArgument)
		if !strings.Contains(err.Error(), "object_name_pt") {
			t.Errorf("%s: error %q does not name object_name_pt", name, err)
		}
	}
	if live := liveRecords(t, it); len(live) != 0 {
		t.Fatalf("a refused name left records: %v", live)
	}
	res, err := it.infuseObject(t, infusionKeyHouse, "  Lampiao\u0303 de Re\u0301  ")
	if err != nil {
		t.Fatalf("a name with combining marks = %v", err)
	}
	if got, want := res.GetInfusion().GetObjectNamePt(), "Lampiaõ de Ré"; got != want {
		t.Errorf("the name kept = %q, want %q (trimmed, NFC)", got, want)
	}
	_, err = it.move(it.dona, res.GetInfusion().GetId(), "", "Lan\u202eterna")
	wantCode(t, "a bidi override on a move", err, connect.CodeInvalidArgument)
	moved, err := it.move(it.dona, res.GetInfusion().GetId(), "", "Cafe\u0301")
	if err != nil || moved.GetInfusion().GetObjectNamePt() != "Café" {
		t.Errorf("a move with combining marks = %v, %v, want Café", moved, err)
	}
}

// Each write answers the sheet's revision after it, and a follow-up write that sends it passes; an older one is refused.
func TestInfusionWritesAnswerTheRevision(t *testing.T) {
	t.Parallel()
	it := newInfusedTableWith(t, func(f *charactersv1.FullSheet) { f.WeaponKeys = []string{infusionMace, infusedDagger} })
	infuse := it.infuseReq(infusionKeyBlade)
	infuse.ItemKey = infusionMace
	res, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(infuse))
	if err != nil {
		t.Fatal(err)
	}
	if got := it.read(t).GetRevision(); res.Msg.GetRevision() == 0 || res.Msg.GetRevision() != got {
		t.Fatalf("InfuseItem revision = %d, the sheet is at %d", res.Msg.GetRevision(), got)
	}
	stale := res.Msg.GetRevision()
	mv, err := it.dona.infusions.MoveInfusion(t.Context(), connect.NewRequest(&charactersv1.MoveInfusionRequest{
		CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: res.Msg.GetInfusion().GetId(), ItemKey: infusedDagger, Revision: res.Msg.GetRevision(),
	}))
	if err != nil {
		t.Fatalf("a move with the revision of the infusion: %v", err)
	}
	if got := it.read(t).GetRevision(); mv.Msg.GetRevision() <= stale || mv.Msg.GetRevision() != got {
		t.Fatalf("MoveInfusion revision = %d (was %d), the sheet is at %d", mv.Msg.GetRevision(), stale, got)
	}
	_, err = it.dona.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{
		CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: res.Msg.GetInfusion().GetId(), Revision: stale,
	}))
	wantCode(t, "an end with the old revision", err, connect.CodeAborted)
	end, err := it.dona.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{
		CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: res.Msg.GetInfusion().GetId(), Revision: mv.Msg.GetRevision(),
	}))
	if err != nil {
		t.Fatalf("an end with the revision of the move: %v", err)
	}
	if got := it.read(t).GetRevision(); end.Msg.GetRevision() <= mv.Msg.GetRevision() || end.Msg.GetRevision() != got {
		t.Errorf("EndInfusion revision = %d, the sheet is at %d", end.Msg.GetRevision(), got)
	}
	next := it.infuseReq(infusionKeyBlade)
	next.ItemKey, next.Revision = infusionMace, end.Msg.GetRevision()
	if _, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(next)); err != nil {
		t.Errorf("an infusion with the revision of the end: %v", err)
	}
}

// A live record of the same infusion that the sheet no longer counts does not make the infusion "in use": the list offers
// the infusion as known, and putting it on ends that record with `taken`.
func TestInfuseItemEndsAStaleRecordOfTheSameInfusion(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	stale := it.infuse(t, infusionKeyBlade, infusedDagger) // the dagger is not on the sheet: the record is not valid now
	known, err := it.list(t, it.dona)
	if err != nil {
		t.Fatal(err)
	}
	offered := false
	for _, k := range known.GetKnown() {
		offered = offered || k.GetInfusionKey() == infusionKeyBlade
	}
	if !offered {
		t.Fatalf("the blade is not offered as known: %v", known.GetKnown())
	}
	res, err := it.infuseOn(t, it.dona, infusionKeyBlade, infusionMace)
	if err != nil || !res.GetInfusion().GetValidNow() {
		t.Fatalf("InfuseItem over a record that is not valid = %v, %v", res, err)
	}
	if endedReason(t, it, stale.ID) != "taken" {
		t.Errorf("the stale record ended %q, want taken", endedReason(t, it, stale.ID))
	}
	_, err = it.infuseOn(t, it.dona, infusionKeyBlade, infusionMace)
	wantInfusionRefusal(t, "the blade while it is in use", err, itemChangedR)
}

// A limit that fell below what was kept leaves the oldest record live and over the limit; a new infusion ends one valid record
// (the oldest valid), not the one that gave way, and a host with no limit refuses the infusion as unknown.
func TestInfuseItemAfterTheLimitFellEndsOneValidRecord(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	first := it.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion().GetId()
	it.h.clock.Advance(time.Minute)
	second := it.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear).GetInfusion().GetId()
	it.h.clock.Advance(time.Minute)
	third, err := it.infuseObject(t, infusionKeyHouse, "Lanterna")
	if err != nil {
		t.Fatal(err)
	}
	it.h.clock.Advance(time.Minute)
	extra, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{ // a fourth one: the limit fell
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: it.pc.GetId(), InfusionKey: infusionKeyAsh,
		ItemKind: infusionItemObject, ObjectName: "Capa velha", Parameter: damageFire,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := it.list(t, it.dona); c.GetUsed() != 3 || len(c.GetActive()) != 4 {
		t.Fatalf("used = %d with %d live, want 3 used (the oldest gave way) and 4 live to start", c.GetUsed(), len(c.GetActive()))
	}
	it.h.clock.Advance(time.Minute)
	res, err := it.infuseOn(t, it.dona, infusionKeyPlate, infusionLeather)
	if err != nil || res.GetEndedInfusionId() != second {
		t.Fatalf("InfuseItem = %v, %v, want the oldest valid record, %s, as the one answered", res, err, second)
	}
	if endedReason(t, it, second) != "replaced" || endedReason(t, it, first) != "" {
		t.Errorf("the oldest valid ended %q and the one over the limit %q, want replaced and live", endedReason(t, it, second), endedReason(t, it, first))
	}
	if endedReason(t, it, third.GetInfusion().GetId()) != "" || endedReason(t, it, extra.ID) != "" {
		t.Error("a newer record ended")
	}
	if c, _ := it.list(t, it.dona); c.GetUsed() != 3 {
		t.Errorf("used = %d, want 3", c.GetUsed())
	}

	v := it.view(t)
	v.derived.InfusionHosts = nil
	_, err = v.replaced(infusionKeyBlade, "")
	wantInfusionRefusal(t, "a host with no limit", err, notKnown)
}

// view is the infuser's sheet as a write reads it.
func (it *infusedTable) view(t *testing.T) infusionView {
	t.Helper()
	content, err := it.h.svc.contentFor(t.Context(), nil, it.campaign)
	if err != nil {
		t.Fatal(err)
	}
	row, err := it.h.svc.queries.GetCharacter(t.Context(), charactersdb.GetCharacterParams{CampaignID: it.campaign, ID: it.pc.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	v, err := it.h.svc.infusionViewOf(t.Context(), it.h.svc.queries, content, row)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// A move with an idempotency key answers the retry with the first answer and writes nothing again, even after the infusion
// moved on; another request under the key is refused.
func TestMoveInfusionIsIdempotent(t *testing.T) {
	t.Parallel()
	it := newInfusedTableWith(t, func(f *charactersv1.FullSheet) { f.WeaponKeys = []string{infusionMace, infusedDagger} })
	blade := it.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion()
	req := &charactersv1.MoveInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: blade.GetId(), ItemKey: infusedDagger, IdempotencyKey: uuid.New().String()}
	first, err := it.dona.infusions.MoveInfusion(t.Context(), connect.NewRequest(req))
	if err != nil || first.Msg.GetInfusion().GetItemKey() != infusedDagger {
		t.Fatalf("MoveInfusion() = %v, %v", first, err)
	}
	revision := it.read(t).GetRevision()
	again, err := it.dona.infusions.MoveInfusion(t.Context(), connect.NewRequest(req))
	if err != nil || again.Msg.GetInfusion().GetId() != blade.GetId() || again.Msg.GetInfusion().GetItemKey() != infusedDagger {
		t.Fatalf("the retry = %v, %v, want the first answer", again, err)
	}
	if again.Msg.GetInfusion().GetSince().AsTime() != first.Msg.GetInfusion().GetSince().AsTime() || it.read(t).GetRevision() != revision {
		t.Error("the retry wrote: the time or the revision changed")
	}
	other := &charactersv1.MoveInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: blade.GetId(), ItemKey: infusionMace, IdempotencyKey: req.GetIdempotencyKey()}
	_, err = it.dona.infusions.MoveInfusion(t.Context(), connect.NewRequest(other))
	wantCode(t, "the same key for another move", err, connect.CodeInvalidArgument)
}
