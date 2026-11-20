package characters

import (
	"errors"
	"slices"
	"testing"
	"time"
	"uuid"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// Every write that changes a record tells play once, in the transaction of the write, with the list of what changed (one call
// for the whole write, so play writes the events in order and tells the streams once), and tells the master and the owner
// that the infusions of the character changed. A retry and a refusal tell nothing.

// infusedPlay is the infused table with a ReviewHost that records what the module tells play.
type infusedPlay struct {
	*infusedTable
	spy *hostSpy
}

func newInfusedPlay(t *testing.T) *infusedPlay {
	t.Helper()
	ip := &infusedPlay{infusedTable: newInfusedTableWith(t, func(f *charactersv1.FullSheet) { f.WeaponKeys = []string{infusionMace, infusedDagger} }), spy: &hostSpy{}}
	ip.h.svc.SetReviewHost(ip.spy)
	return ip
}

// calls takes what play was told since the last call, one list for each write.
func (ip *infusedPlay) calls() [][]link.InfusionChange {
	ip.spy.mu.Lock()
	defer ip.spy.mu.Unlock()
	out := ip.spy.infusions
	ip.spy.infusions = nil
	return out
}

// hints takes the hints the streams were told since the last call.
func (ip *infusedPlay) hints() []string {
	out := ip.spy.told()
	ip.spy.mu.Lock()
	ip.spy.events = nil
	ip.spy.mu.Unlock()
	return out
}

func (ip *infusedPlay) wantHint(t *testing.T, what string) {
	t.Helper()
	want := []string{"infusions_changed:" + ip.pc.GetId()}
	if got := ip.hints(); !slices.Equal(got, want) {
		t.Errorf("%s told the streams %v, want %v", what, got, want)
	}
}

func TestInfusionWritesTellPlayOnceWithWhatChanged(t *testing.T) {
	t.Parallel()
	ip := newInfusedPlay(t)
	id := ip.pc.GetId()

	first := ip.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion().GetId()
	got := ip.calls()
	want := []link.InfusionChange{{
		Kind: link.InfusionInfused, RecordID: first, ArtificerID: id, HolderID: id, InfusionKey: infusionKeyBlade,
		ItemKind: infusionItemGear, ItemKey: infusionMace,
	}}
	if len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("infusing told play %v, want one call with %v", got, want)
	}
	ip.wantHint(t, "infusing")

	// Over a new object: the key of the object is its own.
	ip.h.clock.Advance(time.Minute)
	obj, err := ip.infuseObject(t, infusionKeyHouse, "Lanterna")
	if err != nil {
		t.Fatal(err)
	}
	got = ip.calls()
	if len(got) != 1 || len(got[0]) != 1 || got[0][0].ItemKind != infusionItemObject || got[0][0].ItemKey != "object:"+obj.GetInfusion().GetId() {
		t.Errorf("infusing an object told play %v, want its object key", got)
	}
	ip.hints()

	// Moved: one change, with the item it went to.
	ip.h.clock.Advance(time.Minute)
	if _, err := ip.move(ip.dona, first, infusedDagger, ""); err != nil {
		t.Fatal(err)
	}
	got = ip.calls()
	want = []link.InfusionChange{{
		Kind: link.InfusionMoved, RecordID: first, ArtificerID: id, HolderID: id, InfusionKey: infusionKeyBlade,
		ItemKind: infusionItemGear, ItemKey: infusedDagger,
	}}
	if len(got) != 1 || !slices.Equal(got[0], want) {
		t.Errorf("moving told play %v, want one call with %v", got, want)
	}
	ip.wantHint(t, "moving")

	// Ended by the owner.
	ip.mustEnd(t, first)
	got = ip.calls()
	want = []link.InfusionChange{{
		Kind: link.InfusionEnded, RecordID: first, ArtificerID: id, HolderID: id, InfusionKey: infusionKeyBlade,
		ItemKind: infusionItemGear, ItemKey: infusedDagger, EndedReason: infusionEndedEnded,
	}}
	if len(got) != 1 || !slices.Equal(got[0], want) {
		t.Errorf("ending told play %v, want one call with %v", got, want)
	}
	ip.wantHint(t, "ending")
}

// Over the limit the record that ends and the new one are one call: the ending first, then the infusion.
func TestInfusionOverTheLimitTellsPlayOneCallWithBothChanges(t *testing.T) {
	t.Parallel()
	ip := newInfusedPlay(t)
	oldest := ip.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion().GetId()
	ip.h.clock.Advance(time.Minute)
	ip.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
	ip.h.clock.Advance(time.Minute)
	if _, err := ip.infuseObject(t, infusionKeyHouse, "Lanterna"); err != nil {
		t.Fatal(err)
	}
	ip.calls()
	ip.hints()

	ip.h.clock.Advance(time.Minute)
	res := ip.mustInfuseOn(t, infusionKeyPlate, infusionLeather)
	if res.GetEndedInfusionId() != oldest {
		t.Fatalf("the infusion ended %q, want the oldest %q", res.GetEndedInfusionId(), oldest)
	}
	got := ip.calls()
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("the write told play %v, want one call with two changes", got)
	}
	if c := got[0][0]; c.Kind != link.InfusionEnded || c.RecordID != oldest || c.EndedReason != infusionEndedReplaced {
		t.Errorf("first change = %+v, want the oldest ended as replaced", c)
	}
	if c := got[0][1]; c.Kind != link.InfusionInfused || c.RecordID != res.GetInfusion().GetId() {
		t.Errorf("second change = %+v, want the new record infused", c)
	}
	ip.wantHint(t, "an infusion over the limit")
}

// An infusion that takes the item of a record that is not valid now ends that record with `taken`, in the same call.
func TestInfusionOverAStaleRecordTellsPlayItEnded(t *testing.T) {
	t.Parallel()
	ip := newInfusedPlay(t)
	stale := ip.infuse(t, infusionKeyBlade, "equipment:club") // the club is not on the sheet
	ip.h.clock.Advance(time.Minute)
	res := ip.mustInfuseOn(t, infusionKeyBlade, infusionMace)
	got := ip.calls()
	if len(got) != 1 || len(got[0]) != 2 || got[0][0].Kind != link.InfusionEnded || got[0][0].RecordID != stale.ID || got[0][0].EndedReason != infusionEndedTaken ||
		got[0][1].Kind != link.InfusionInfused || got[0][1].RecordID != res.GetInfusion().GetId() {
		t.Errorf("the write told play %+v, want the stale record ended as taken, then the new one infused", got)
	}
}

// Moving onto the item of a record that is not valid now ends that record, in the same call as the move.
func TestMoveOverAStaleRecordTellsPlayItEnded(t *testing.T) {
	t.Parallel()
	ip := newInfusedPlay(t)
	moving := ip.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion().GetId()
	stale, err := ip.h.svc.insertInfusion(t.Context(), ip.h.svc.queries, newInfusion{
		CampaignID: ip.campaign, ArtificerID: ip.pc.GetId(), HolderID: ip.pc.GetId(), InfusionKey: "feature:no-longer-there", ItemKind: infusionItemGear, GearKey: infusedDagger,
	})
	if err != nil {
		t.Fatal(err)
	}
	ip.calls()
	ip.h.clock.Advance(time.Minute)
	if _, err := ip.move(ip.dona, moving, infusedDagger, ""); err != nil {
		t.Fatal(err)
	}
	got := ip.calls()
	if len(got) != 1 || len(got[0]) != 2 || got[0][0].Kind != link.InfusionEnded || got[0][0].RecordID != stale.ID || got[0][0].EndedReason != infusionEndedTaken ||
		got[0][1].Kind != link.InfusionMoved || got[0][1].RecordID != moving {
		t.Errorf("the move told play %+v, want the stale record ended as taken, then the move", got)
	}
}

// A move onto an object tells play the object's own key, the one the record keeps.
// An infusion that takes an object takes no piece, so a move from a piece onto an object cannot happen.
func TestMoveOntoAnObjectTellsPlayTheObjectKey(t *testing.T) {
	t.Parallel()
	ip := newInfusedPlay(t)
	first, err := ip.infuseObject(t, infusionKeyHouse, "Lampião")
	if err != nil {
		t.Fatal(err)
	}
	rec := first.GetInfusion().GetId()
	ip.calls()
	ip.h.clock.Advance(time.Minute)
	if _, err := ip.move(ip.dona, rec, "", "Lanterna"); err != nil {
		t.Fatal(err)
	}
	got := ip.calls()
	if len(got) != 1 || len(got[0]) != 1 || got[0][0].Kind != link.InfusionMoved || got[0][0].ItemKind != infusionItemObject || got[0][0].ItemKey != "object:"+rec {
		t.Fatalf("the move told play %+v, want one move to the key object:%s", got, rec)
	}
	if kept := ip.record(t, rec).ItemKey; kept != got[0][0].ItemKey {
		t.Errorf("play was told %q, the record keeps %q", got[0][0].ItemKey, kept)
	}
}

// A retry, and a write play or the checks refused, tell nothing: no call, no hint.
func TestInfusionRetryAndRefusalTellPlayNothing(t *testing.T) {
	t.Parallel()
	ip := newInfusedPlay(t)
	req := ip.infuseReq(infusionKeyBlade)
	req.ItemKey, req.IdempotencyKey = infusionMace, uuid.New().String()
	for i := range 2 {
		if _, err := ip.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req)); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := ip.calls(); len(got) != 1 {
		t.Errorf("a call and its retry told play %d times, want 1", len(got))
	}
	if got := ip.hints(); len(got) != 1 {
		t.Errorf("a call and its retry told the streams %v, want one hint", got)
	}
	// Refused: the mace already has an infusion.
	if _, err := ip.infuseOn(t, ip.dona, infusionKeyFalse, infusionMace); err == nil {
		t.Fatal("the second infusion on the mace was not refused")
	}
	if got, hints := ip.calls(), ip.hints(); len(got) != 0 || len(hints) != 0 {
		t.Errorf("a refused write told play %v and the streams %v", got, hints)
	}
}

// A refusal from play rolls the whole write back: the record is not written.
func TestInfusionWriteFailsWhenPlayRefuses(t *testing.T) {
	t.Parallel()
	ip := newInfusedPlay(t)
	boom := errors.New("play is down")
	ip.spy.failWith = boom
	if _, err := ip.infuseOn(t, ip.dona, infusionKeyBlade, infusionMace); err == nil {
		t.Fatal("InfuseItem succeeded although play failed")
	}
	if n := len(liveRecords(t, ip.infusedTable)); n != 0 {
		t.Errorf("%d live records after a write that failed", n)
	}
	if got := ip.hints(); len(got) != 0 {
		t.Errorf("a write that failed told the streams %v", got)
	}
}

// A sheet save that takes a piece off the sheet ends its record with item_gone and tells play, once, in the save's
// transaction; a save that ends nothing tells play nothing.
func TestTheSaveTellsPlayWhatItEnded(t *testing.T) {
	t.Parallel()
	ip := newInfusedPlay(t)
	id := ip.pc.GetId()
	blade := ip.infuse(t, infusionKeyBlade, infusionMace)
	plate := ip.infuse(t, infusionKeyPlate, infusionLeather)

	pc, err := editorUpdate(ip.dona, editorSaveOf(ip.read(t)))
	if err != nil {
		t.Fatal(err)
	}
	if got, hints := ip.calls(), ip.hints(); len(got) != 0 || len(hints) != 0 {
		t.Fatalf("a save that kept the gear told play %v and the streams %v", got, hints)
	}

	req := editorSaveOf(pc)
	req.Sheet.GetFull().WeaponKeys = []string{"equipment:club"}
	req.Sheet.GetFull().ArmorKey = "equipment:studded-leather-armor"
	if _, err := editorUpdate(ip.dona, req); err != nil {
		t.Fatal(err)
	}
	got := ip.calls()
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("the save told play %+v, want one call with two endings", got)
	}
	ended := map[string]link.InfusionChange{}
	for _, c := range got[0] {
		ended[c.RecordID] = c
	}
	for _, rec := range []string{blade.ID, plate.ID} {
		c, ok := ended[rec]
		if !ok || c.Kind != link.InfusionEnded || c.EndedReason != infusionEndedItemGone || c.ArtificerID != id || c.HolderID != id {
			t.Errorf("change of %s = %+v, want it ended as item_gone", rec, c)
		}
	}
	ip.wantHint(t, "the save")
}
