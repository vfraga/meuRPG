package characters

import (
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The limit holds on every read: the derivation never applies more records than the artificer's limit at the class level, the
// oldest give way, and nothing ends by itself. The writes judge by the same reading.

// keepFour keeps four live records of the artificer, the oldest the attuned ash cloak, against a limit of three: the state a
// restored pick or a level taken off leaves.
func (it *infusedTable) keepFour(t *testing.T) (cloak, blade, shield, lantern charactersdb.CharacterInfusion) {
	t.Helper()
	add := func(n newInfusion) charactersdb.CharacterInfusion {
		n.CampaignID, n.ArtificerID, n.HolderID = it.campaign, it.pc.GetId(), it.pc.GetId()
		rec, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, n)
		if err != nil {
			t.Fatalf("insertInfusion(%s) error = %v", n.InfusionKey, err)
		}
		it.h.clock.Advance(time.Minute)
		return rec
	}
	cloak = add(newInfusion{InfusionKey: infusionKeyAsh, ItemKind: infusionItemGear, GearKey: infusionLeather, Parameter: damageFire, Attuned: true})
	blade = add(newInfusion{InfusionKey: infusionKeyBlade, ItemKind: infusionItemGear, GearKey: infusionMace})
	shield = add(newInfusion{InfusionKey: infusionKeyFalse, ItemKind: infusionItemGear, GearKey: infusionShieldGear})
	lantern = add(newInfusion{InfusionKey: infusionKeyHouse, ItemKind: infusionItemObject, ObjectName: "Lanterna"})
	return cloak, blade, shield, lantern
}

// reasonOf is what the sheet says of a record that is not valid now.
func reasonOf(t *testing.T, v infusionView, id string) string {
	t.Helper()
	for _, st := range v.derived.Infusions {
		if st.ID == id {
			return st.ReasonPT
		}
	}
	t.Fatalf("no status of the record %s", id)
	return ""
}

const overTheLimit = "acima do limite"

func (it *infusedTable) wantOnlyTheCloakGivesWay(t *testing.T, cloak charactersdb.CharacterInfusion, others ...charactersdb.CharacterInfusion) {
	t.Helper()
	list, err := it.list(t, it.dona)
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]bool{}
	for _, in := range list.GetActive() {
		valid[in.GetId()] = in.GetValidNow()
	}
	if valid[cloak.ID] {
		t.Errorf("the oldest record is valid now, want it over the limit: %v", valid)
	}
	for _, o := range others {
		if !valid[o.ID] {
			t.Errorf("the record %s is not valid now, want the newest to keep their place: %v", o.InfusionKey, valid)
		}
	}
	if list.GetUsed() != list.GetLimit() || list.GetLimit() < 1 {
		t.Errorf("used %d of %d, want the limit full and never beyond it", list.GetUsed(), list.GetLimit())
	}
	v := it.view(t)
	if got := reasonOf(t, v, cloak.ID); !strings.Contains(got, overTheLimit) {
		t.Errorf("the reason of the oldest = %q, want it to say %q", got, overTheLimit)
	}
	if got := rules.AttunedCount(v.build, v.content); got != 0 {
		t.Errorf("attuned = %d, want 0: the attuned cloak is over the limit and takes no place", got)
	}
	if a := attackOf(it.read(t), infusionMace); a.GetMagicBonus() != 1 {
		t.Errorf("the mace of a record that keeps its place = %v, want +1", a)
	}
}

// A record that turns valid again beyond the limit: the pick the master restored. The oldest gives way and stays live; the
// infusions the character knows are still listed.
func TestARestoredPickBeyondTheLimitMakesTheOldestGiveWay(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	cloak, blade, shield, lantern := it.keepFour(t)
	if _, err := it.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,feature_choice_keys}', $2::jsonb) WHERE id = $1`,
		it.pc.GetId(), `["`+infusionKeyBlade+`","`+infusionKeyPlate+`","`+infusionKeyFalse+`","`+infusionKeyHouse+`"]`); err != nil {
		t.Fatal(err)
	}
	if got := reasonOf(t, it.view(t), cloak.ID); strings.Contains(got, overTheLimit) {
		t.Fatalf("the cloak without its pick says %q, want not picked", got)
	}
	if got := rules.AttunedCount(it.view(t).build, it.view(t).content); got != 0 {
		t.Fatalf("attuned = %d without the pick, want 0", got)
	}
	if _, err := it.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,feature_choice_keys}', $2::jsonb) WHERE id = $1`,
		it.pc.GetId(), `["`+infusionKeyBlade+`","`+infusionKeyPlate+`","`+infusionKeyAsh+`","`+infusionKeyFalse+`","`+infusionKeyHouse+`"]`); err != nil {
		t.Fatal(err)
	}
	it.wantOnlyTheCloakGivesWay(t, cloak, blade, shield, lantern)
	if endedReason(t, it, cloak.ID) != "" {
		t.Error("the record that gave way ended by itself")
	}
	list, _ := it.list(t, it.dona)
	known := map[string]bool{}
	for _, k := range list.GetKnown() {
		known[k.GetInfusionKey()] = true
	}
	if !known[infusionKeyPlate] || !known[infusionKeyAsh] || len(known) != 2 {
		t.Errorf("known = %v, want the plate and the ash cloak (a limit does not make the character forget an infusion)", known)
	}
}

// A level taken off lowers the limit: the oldest give way, and the attuned one takes no place.
func TestALoweredLimitMakesTheOldestGiveWay(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	cloak, blade, shield, _ := it.keepFour(t)
	if _, _, err := it.h.svc.endInfusion(t.Context(), it.h.svc.queries, it.campaign, shield.ID, infusionEndedEnded); err != nil {
		t.Fatal(err)
	}
	if v := it.view(t); rules.AttunedCount(v.build, v.content) != 1 {
		t.Fatalf("attuned = %d at the level 6, want 1", rules.AttunedCount(v.build, v.content))
	}
	if _, err := it.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,classes,0,level}', '5') WHERE id = $1`, it.pc.GetId()); err != nil {
		t.Fatal(err)
	}
	list, err := it.list(t, it.dona)
	if err != nil || list.GetLimit() != 2 || list.GetUsed() != 2 {
		t.Fatalf("list = %v, %v, want 2 used of 2", list, err)
	}
	valid := map[string]bool{}
	for _, in := range list.GetActive() {
		valid[in.GetId()] = in.GetValidNow()
	}
	if valid[cloak.ID] || !valid[blade.ID] {
		t.Errorf("valid = %v, want the cloak (the oldest) to give way", valid)
	}
	if v := it.view(t); rules.AttunedCount(v.build, v.content) != 0 || !strings.Contains(reasonOf(t, v, cloak.ID), overTheLimit) {
		t.Errorf("attuned = %d, reason %q, want 0 and over the limit", rules.AttunedCount(v.build, v.content), reasonOf(t, v, cloak.ID))
	}
}

// A write at the limit judges by the same reading: the record over the limit is not one of the valid, so one valid record
// ends, the oldest, and the new infusion takes its place. A piece that carries the record over the limit is refused (ending
// that record frees no place): the player ends it first, with EndInfusion.
func TestAnInfusionAtTheLimitAgreesWithTheDerivation(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	cloak, blade, shield, lantern := it.keepFour(t)
	_, err := it.infuseOn(t, it.dona, infusionKeyPlate, infusionLeather)
	wantInfusionRefusal(t, "InfuseItem on the piece of the record over the limit", err, itemChangedR)
	it.mustEnd(t, cloak.ID) // the player ends the record over the limit: it frees no place
	if c, _ := it.list(t, it.dona); c.GetUsed() != 3 || c.GetLimit() != 3 {
		t.Fatalf("used %d of %d after ending the record over the limit, want 3 of 3", c.GetUsed(), c.GetLimit())
	}
	res, err := it.infuseOn(t, it.dona, infusionKeyPlate, infusionLeather)
	if err != nil || res.GetEndedInfusionId() != blade.ID {
		t.Fatalf("InfuseItem = %v, %v, want the oldest valid record, the blade, ended", res, err)
	}
	if endedReason(t, it, blade.ID) != "replaced" || endedReason(t, it, cloak.ID) != "ended" {
		t.Errorf("reasons: blade %q, cloak %q, want replaced and ended", endedReason(t, it, blade.ID), endedReason(t, it, cloak.ID))
	}
	if endedReason(t, it, shield.ID) != "" || endedReason(t, it, lantern.ID) != "" {
		t.Error("a newer record ended")
	}
	if c, _ := it.list(t, it.dona); c.GetUsed() != 3 || c.GetLimit() != 3 {
		t.Errorf("used %d of %d, want 3 of 3", c.GetUsed(), c.GetLimit())
	}
}

// A dead artificer's infusions stay until the master ends them: the master ends a record, the owner does not, and nobody
// infuses or moves.
func TestAMasterEndsTheInfusionOfADeadArtificer(t *testing.T) {
	t.Parallel()
	it := newInfusedTableWith(t, func(f *charactersv1.FullSheet) { f.WeaponKeys = []string{infusionMace, infusedDagger} })
	blade := it.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion()
	if _, err := it.master.api.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	end := func(u *user) error {
		_, err := u.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: blade.GetId()}))
		return err
	}
	dead := charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD
	for _, u := range []*user{it.dona, it.master} {
		_, err := it.infuseOn(t, u, infusionKeyFalse, infusionShieldGear)
		if d := blocked(t, "InfuseItem(dead)", err); d.GetReason() != dead {
			t.Errorf("InfuseItem(dead) reason = %v, want CHARACTER_DEAD", d.GetReason())
		}
		_, err = u.infusions.MoveInfusion(t.Context(), connect.NewRequest(&charactersv1.MoveInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: blade.GetId(), ItemKey: infusedDagger}))
		if d := blocked(t, "MoveInfusion(dead)", err); d.GetReason() != dead {
			t.Errorf("MoveInfusion(dead) reason = %v, want CHARACTER_DEAD", d.GetReason())
		}
	}
	if d := blocked(t, "EndInfusion(dead, owner)", end(it.dona)); d.GetReason() != dead {
		t.Errorf("EndInfusion(dead, owner) reason = %v, want CHARACTER_DEAD", d.GetReason())
	}
	if endedReason(t, it, blade.GetId()) != "" {
		t.Fatal("a refused call ended the record")
	}
	if err := end(it.master); err != nil {
		t.Fatalf("the master ends the record of a dead artificer: %v", err)
	}
	if endedReason(t, it, blade.GetId()) != "ended" {
		t.Errorf("reason = %q, want ended", endedReason(t, it, blade.GetId()))
	}
}

// Naming the record on the very piece is a replacement: that record ends `replaced` and the new one goes on the piece, with
// the limit full or not.
func TestInfusingAPieceNamingItsOwnRecordReplacesIt(t *testing.T) {
	t.Parallel()
	for _, full := range []bool{false, true} {
		it := newInfusedTable(t)
		plate := it.mustInfuseOn(t, infusionKeyPlate, infusionLeather).GetInfusion()
		it.h.clock.Advance(time.Minute)
		if full {
			it.mustInfuseOn(t, infusionKeyBlade, infusionMace)
			it.h.clock.Advance(time.Minute)
			it.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
			it.h.clock.Advance(time.Minute)
		}
		// Without the name the piece is taken.
		req := it.infuseReq(infusionKeyAsh)
		req.ItemKey, req.Parameter = infusionLeather, damageFire
		_, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
		wantInfusionRefusal(t, "without naming the record", err, alreadyHeld)
		req.EndInfusionId = plate.GetId()
		res, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
		if err != nil || res.Msg.GetEndedInfusionId() != plate.GetId() || res.Msg.GetInfusion().GetItemKey() != infusionLeather {
			t.Fatalf("full=%v: InfuseItem naming the record on the piece = %v, %v, want it ended and the new one on the piece", full, res, err)
		}
		if endedReason(t, it, plate.GetId()) != "replaced" {
			t.Errorf("full=%v: the plate ended %q, want replaced", full, endedReason(t, it, plate.GetId()))
		}
		wantLive := 1
		if full {
			wantLive = 3
		}
		if live := liveRecords(t, it); len(live) != wantLive {
			t.Errorf("full=%v: live = %v, want %d", full, live, wantLive)
		}
	}
}
