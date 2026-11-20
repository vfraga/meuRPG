package characters

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// A write never pushes a valid record out silently: after InfuseItem or MoveInfusion every record that was valid before is
// still valid, except the ones the call ended (each with its event). With the limit full, a record that is over the limit
// frees no place, so the calls that would make it count again, or end it to make room, are refused with ITEM_CHANGED and
// change nothing.

// allRecords reads every record the character holds, live or ended, in id order.
func (it *infusedTable) allRecords(t *testing.T) []charactersdb.CharacterInfusion {
	t.Helper()
	rows, err := it.h.pool.Query(t.Context(), `SELECT id FROM character_infusions WHERE holder_character_id = $1 ORDER BY id`, it.pc.GetId())
	if err != nil {
		t.Fatal(err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	out := make([]charactersdb.CharacterInfusion, 0, len(ids))
	for _, id := range ids {
		out = append(out, it.record(t, id))
	}
	return out
}

// validNow is the ids of the records that are valid now, as the sheet reads them.
func (it *infusedTable) validNow(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, st := range it.view(t).derived.Infusions {
		if st.Valid {
			out = append(out, st.ID)
		}
	}
	slices.Sort(out)
	return out
}

// overLimitOf fills the table with three valid records and a fourth, the oldest, that gave way: the blade on the mace is over
// the limit, the false bottom on the shield, the house seal on the lantern and the ash cloak on an old cape are valid. It
// returns the blade's id.
func overLimitOf(t *testing.T, it *infusedTable) string {
	t.Helper()
	first := it.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion().GetId()
	it.h.clock.Advance(time.Minute)
	it.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
	it.h.clock.Advance(time.Minute)
	if _, err := it.infuseObject(t, infusionKeyHouse, "Lanterna"); err != nil {
		t.Fatal(err)
	}
	it.h.clock.Advance(time.Minute)
	if _, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: it.pc.GetId(), HolderID: it.pc.GetId(), InfusionKey: infusionKeyAsh,
		ItemKind: infusionItemObject, ObjectName: "Capa velha", Parameter: damageFire,
	}); err != nil {
		t.Fatal(err)
	}
	it.h.clock.Advance(time.Minute)
	if c, _ := it.list(t, it.dona); c.GetUsed() != 3 || len(c.GetActive()) != 4 {
		t.Fatalf("used = %d with %d live, want 3 used and 4 live to start", c.GetUsed(), len(c.GetActive()))
	}
	return first
}

// untouched is what a refusal leaves: the records, the sheet's revision, what play was told and what the streams were told.
type untouched struct {
	records  []charactersdb.CharacterInfusion
	revision int32
}

func (ip *infusedPlay) before(t *testing.T) untouched {
	t.Helper()
	ip.calls()
	ip.hints()
	return untouched{records: ip.allRecords(t), revision: ip.read(t).GetRevision()}
}

// wantUntouched checks that nothing was written after a refusal: no record, no ending, no revision, no play call, no hint,
// and no answer kept under the idempotency key.
func (ip *infusedPlay) wantUntouched(t *testing.T, was untouched, key string) {
	t.Helper()
	if got := ip.allRecords(t); !reflect.DeepEqual(got, was.records) {
		t.Errorf("the records changed on a refusal: %+v, want %+v", got, was.records)
	}
	if got := ip.read(t).GetRevision(); got != was.revision {
		t.Errorf("revision = %d, want %d: a refusal wrote the sheet", got, was.revision)
	}
	if got := ip.calls(); len(got) != 0 {
		t.Errorf("a refusal told play %v", got)
	}
	if got := ip.hints(); len(got) != 0 {
		t.Errorf("a refusal told the streams %v", got)
	}
	if key != "" {
		if _, err := ip.h.svc.queries.GetCharacterInfusionRequest(t.Context(), charactersdb.GetCharacterInfusionRequestParams{CampaignID: ip.campaign, IdempotencyKey: key}); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("an answer was kept under the key of a refusal: %v", err)
		}
	}
}

// A record over the limit that the caller names is refused at a full host: ending it frees no place, so the oldest valid
// record would go over the limit with no event. The player ends such a record with EndInfusion.
func TestInfuseItemNamingARecordOverTheLimitIsRefusedAtAFullLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, infusion, item string }{
		{"another piece", infusionKeyPlate, infusionLeather},
		{"the same piece", infusionKeyBlade, infusionMace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ip := newInfusedPlay(t)
			first := overLimitOf(t, ip.infusedTable)
			was := ip.before(t)
			req := ip.infuseReq(tc.infusion)
			req.ItemKey, req.EndInfusionId, req.IdempotencyKey = tc.item, first, uuid.New().String()
			_, err := ip.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
			wantInfusionRefusal(t, "InfuseItem naming the record over the limit", err, itemChangedR)
			ip.wantUntouched(t, was, req.IdempotencyKey)
		})
	}
}

// Ending a valid record at a full host frees a place and the record over the limit takes it again: a record is over the limit
// only while the host is full, so a free place and a record over the limit never meet.
func TestARecordOverTheLimitExistsOnlyWhileTheHostIsFull(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	cloak, _, shield, _ := it.keepFour(t)
	if valid := it.validNow(t); slices.Contains(valid, cloak.ID) || len(valid) != 3 {
		t.Fatalf("valid = %v, want three and not the oldest", valid)
	}
	it.mustEnd(t, shield.ID)
	if valid := it.validNow(t); !slices.Contains(valid, cloak.ID) || len(valid) != 3 {
		t.Errorf("valid = %v after a valid record ended, want the record that gave way back and three valid", valid)
	}
	for _, st := range it.view(t).derived.Infusions {
		if st.OverLimit {
			t.Errorf("the record %s is over the limit with a place free", st.ID)
		}
	}
}

// A piece that carries a record over the limit, infused again at a full host: refused, named or not, since ending that record
// frees no place.
func TestInfuseItemOnAPieceWithARecordOverTheLimitIsRefusedAtAFullLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		named func(cloak, blade string) string
	}{
		{"not named", func(string, string) string { return "" }},
		{"naming the record over the limit", func(cloak, _ string) string { return cloak }},
		{"naming a valid record", func(_, blade string) string { return blade }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ip := newInfusedPlay(t)
			cloak, blade, _, _ := ip.keepFour(t)
			was := ip.before(t)
			req := ip.infuseReq(infusionKeyPlate)
			req.ItemKey, req.EndInfusionId, req.IdempotencyKey = infusionLeather, tc.named(cloak.ID, blade.ID), uuid.New().String()
			_, err := ip.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
			wantInfusionRefusal(t, "InfuseItem on the piece of a record over the limit", err, itemChangedR)
			ip.wantUntouched(t, was, req.IdempotencyKey)
		})
	}
}

// Moving a record that is not valid now makes it the newest, and it may count: at a full host the oldest valid record would go
// over the limit with no event, so the move is refused. Under the limit it moves.
func TestMoveInfusionOfARecordThatWouldCountIsRefusedAtAFullLimit(t *testing.T) {
	t.Parallel()
	t.Run("over the limit", func(t *testing.T) {
		t.Parallel()
		ip := newInfusedPlay(t)
		first := overLimitOf(t, ip.infusedTable)
		was := ip.before(t)
		_, err := ip.move(ip.dona, first, infusedDagger, "")
		wantInfusionRefusal(t, "MoveInfusion of the record over the limit", err, itemChangedR)
		ip.wantUntouched(t, was, "")
		if got := ip.validNow(t); len(got) != 3 {
			t.Errorf("valid = %v, want the three that were", got)
		}
	})
	for _, full := range []bool{true, false} {
		t.Run(map[bool]string{true: "a piece that left the sheet, full", false: "a piece that left the sheet, a place free"}[full], func(t *testing.T) {
			t.Parallel()
			ip := newInfusedPlay(t)
			// The blade is on a quarterstaff the sheet does not list (the piece left), so it is not valid and takes no place.
			gone := ip.infuse(t, infusionKeyBlade, "equipment:quarterstaff")
			ip.h.clock.Advance(time.Minute)
			ip.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
			ip.h.clock.Advance(time.Minute)
			if _, err := ip.infuseObject(t, infusionKeyHouse, "Lanterna"); err != nil {
				t.Fatal(err)
			}
			if full {
				ip.h.clock.Advance(time.Minute)
				ip.mustInfuseOn(t, infusionKeyPlate, infusionLeather)
			}
			wantValid := 2
			if full {
				wantValid = 3
			}
			if got := ip.validNow(t); len(got) != wantValid || slices.Contains(got, gone.ID) {
				t.Fatalf("valid = %v, want %d and not the record of the piece that left", got, wantValid)
			}
			was := ip.before(t)
			ip.h.clock.Advance(time.Minute)
			_, err := ip.move(ip.dona, gone.ID, infusionMace, "")
			if full {
				wantInfusionRefusal(t, "MoveInfusion that would make the record count at a full limit", err, itemChangedR)
				ip.wantUntouched(t, was, "")
				return
			}
			if err != nil {
				t.Fatalf("MoveInfusion with a place free error = %v", err)
			}
			if got := ip.validNow(t); len(got) != 3 || !slices.Contains(got, gone.ID) {
				t.Errorf("valid = %v, want the moved record counting with the two that were", got)
			}
		})
	}
}

// lostPieceTable is a full host with a record of the ash cloak on the leather armor that is not valid because the pick was taken
// off (the prerequisite of the infusion is lost), and the blade, the false bottom and the house seal valid.
func lostPieceTable(t *testing.T) (*infusedPlay, charactersdb.CharacterInfusion) {
	t.Helper()
	ip := newInfusedPlay(t)
	cloak, _, _, _ := ip.keepFour(t)
	if _, err := ip.h.pool.Exec(t.Context(), `UPDATE characters SET sheet = jsonb_set(sheet, '{full,feature_choice_keys}', $2::jsonb) WHERE id = $1`,
		ip.pc.GetId(), `["`+infusionKeyBlade+`","`+infusionKeyPlate+`","`+infusionKeyFalse+`","`+infusionKeyHouse+`"]`); err != nil {
		t.Fatal(err)
	}
	if got := ip.validNow(t); len(got) != 3 || slices.Contains(got, cloak.ID) {
		t.Fatalf("valid = %v, want the three that are not the cloak", got)
	}
	ip.h.clock.Advance(time.Minute)
	return ip, cloak
}

// idsToKeys makes an answer comparable across two tables: the ids that differ by construction become the infusion keys.
func idsToKeys(t *testing.T, it *infusedTable, res *charactersv1.InfuseItemResponse) *charactersv1.InfuseItemResponse {
	t.Helper()
	out := proto.Clone(res).(*charactersv1.InfuseItemResponse)
	if out.GetEndedInfusionId() != "" {
		out.EndedInfusionId = it.record(t, out.GetEndedInfusionId()).InfusionKey
	}
	if out.GetInfusion() != nil {
		out.Infusion.Id, out.Infusion.Since, out.Infusion.HolderCharacterId = "", nil, ""
	}
	return out
}

// endedByKey is, for every record of the table, the reason it ended with, by infusion key and item.
func (it *infusedTable) endedByKey(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range it.allRecords(t) {
		reason := ""
		if r.EndedReason != nil {
			reason = *r.EndedReason
		}
		item := r.ItemKey
		if r.ItemKind == infusionItemObject {
			item = r.ObjectName
		}
		out[r.InfusionKey+"@"+item] = reason
	}
	return out
}

// The request names the record on the piece, which the write ends anyway: naming it and not naming it give the same answer
// and end the same records with the same reasons, with the limit full. The record that is not valid for another reason (a
// lost prerequisite, another artificer's) ends `taken`, and the oldest valid record gives way as `replaced`.
func TestInfuseItemNamingTheRecordOnThePieceThatIsNotValidAnswersAsNotNamingIt(t *testing.T) {
	t.Parallel()
	other := func(t *testing.T, ip *infusedPlay) charactersdb.CharacterInfusion {
		t.Helper()
		stranger := ip.caio.create(t, ip.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Outro", ip.read(t).GetSheet())
		rec, err := ip.h.svc.insertInfusion(t.Context(), ip.h.svc.queries, newInfusion{
			CampaignID: ip.campaign, ArtificerID: stranger.GetId(), HolderID: ip.pc.GetId(), InfusionKey: infusionKeyAsh, ItemKind: infusionItemGear, GearKey: infusionLeather,
		})
		if err != nil {
			t.Fatal(err)
		}
		return rec
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) (*infusedPlay, string)
	}{
		{"a lost prerequisite", func(t *testing.T) (*infusedPlay, string) {
			t.Helper()
			ip, cloak := lostPieceTable(t)
			return ip, cloak.ID
		}},
		{"another artificer's record", func(t *testing.T) (*infusedPlay, string) {
			t.Helper()
			ip := newInfusedPlay(t)
			ip.mustInfuseOn(t, infusionKeyBlade, infusionMace)
			ip.h.clock.Advance(time.Minute)
			ip.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
			ip.h.clock.Advance(time.Minute)
			if _, err := ip.infuseObject(t, infusionKeyHouse, "Lanterna"); err != nil {
				t.Fatal(err)
			}
			ip.h.clock.Advance(time.Minute)
			return ip, other(t, ip).ID
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			unnamed, _ := tc.setup(t)
			named, namedID := tc.setup(t)
			if got := named.view(t).derived; len(got.Infusions) < 4 {
				t.Fatalf("infusions = %v, want the record on the piece among them", got.Infusions)
			}
			bare := unnamed.infuseReq(infusionKeyPlate)
			bare.ItemKey = infusionLeather
			a, errA := unnamed.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(bare))
			byName := named.infuseReq(infusionKeyPlate)
			byName.ItemKey, byName.EndInfusionId = infusionLeather, namedID
			b, errB := named.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(byName))
			if errA != nil || errB != nil {
				t.Fatalf("InfuseItem not named = %v, named = %v, want both to go through", errA, errB)
			}
			if x, y := idsToKeys(t, unnamed.infusedTable, a.Msg), idsToKeys(t, named.infusedTable, b.Msg); !proto.Equal(x, y) {
				t.Errorf("the answers differ: not named %v, named %v", x, y)
			}
			if x, y := unnamed.endedByKey(t), named.endedByKey(t); !reflect.DeepEqual(x, y) {
				t.Errorf("the records ended differently: not named %v, named %v", x, y)
			}
			if got := named.endedByKey(t)[infusionKeyAsh+"@"+infusionLeather]; got != "taken" {
				t.Errorf("the record on the piece ended %q, want taken", got)
			}
			if got := named.endedByKey(t)[infusionKeyBlade+"@"+infusionMace]; got != "replaced" {
				t.Errorf("the oldest valid record ended %q, want replaced", got)
			}
			calls := named.calls()
			if len(calls) == 0 {
				t.Fatal("play was told nothing")
			}
			var ended []string
			for _, c := range calls[len(calls)-1] {
				if c.Kind == link.InfusionEnded {
					ended = append(ended, c.EndedReason)
				}
			}
			if len(ended) < 2 {
				t.Errorf("the write told play %v ended, want one ending for each record that gave way", ended)
			}
		})
	}
}

// Every write keeps every record that was valid before valid, except the ones the call ended with an event: one table for the
// plain cases and for each refusal.
func TestAnInfusionWriteNeverPushesAValidRecordOutSilently(t *testing.T) {
	t.Parallel()
	type call func(t *testing.T, ip *infusedPlay) error
	infuse := func(infusion, item, named string) call {
		return func(t *testing.T, ip *infusedPlay) error {
			t.Helper()
			req := ip.infuseReq(infusion)
			req.ItemKey, req.EndInfusionId = item, named
			_, err := ip.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
			return err
		}
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) (*infusedPlay, call)
		// refused says the call must be refused with ITEM_CHANGED (and so end nothing).
		refused bool
	}{
		{"under the limit", func(t *testing.T) (*infusedPlay, call) {
			t.Helper()
			ip := newInfusedPlay(t)
			ip.mustInfuseOn(t, infusionKeyBlade, infusionMace)
			ip.h.clock.Advance(time.Minute)
			return ip, infuse(infusionKeyPlate, infusionLeather, "")
		}, false},
		{"at the limit naming a record that counts", func(t *testing.T) (*infusedPlay, call) {
			t.Helper()
			ip := newInfusedPlay(t)
			blade := ip.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion().GetId()
			ip.h.clock.Advance(time.Minute)
			ip.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
			ip.h.clock.Advance(time.Minute)
			if _, err := ip.infuseObject(t, infusionKeyHouse, "Lanterna"); err != nil {
				t.Fatal(err)
			}
			ip.h.clock.Advance(time.Minute)
			return ip, infuse(infusionKeyPlate, infusionLeather, blade)
		}, false},
		{"at the limit not naming one", func(t *testing.T) (*infusedPlay, call) {
			t.Helper()
			ip := newInfusedPlay(t)
			ip.mustInfuseOn(t, infusionKeyBlade, infusionMace)
			ip.h.clock.Advance(time.Minute)
			ip.mustInfuseOn(t, infusionKeyFalse, infusionShieldGear)
			ip.h.clock.Advance(time.Minute)
			if _, err := ip.infuseObject(t, infusionKeyHouse, "Lanterna"); err != nil {
				t.Fatal(err)
			}
			ip.h.clock.Advance(time.Minute)
			return ip, infuse(infusionKeyPlate, infusionLeather, "")
		}, false},
		{"naming a record over the limit", func(t *testing.T) (*infusedPlay, call) {
			t.Helper()
			ip := newInfusedPlay(t)
			first := overLimitOf(t, ip.infusedTable)
			return ip, infuse(infusionKeyPlate, infusionLeather, first)
		}, true},
		{"a piece with a record over the limit, not named", func(t *testing.T) (*infusedPlay, call) {
			t.Helper()
			ip := newInfusedPlay(t)
			ip.keepFour(t)
			return ip, infuse(infusionKeyPlate, infusionLeather, "")
		}, true},
		{"moving a record over the limit", func(t *testing.T) (*infusedPlay, call) {
			t.Helper()
			ip := newInfusedPlay(t)
			first := overLimitOf(t, ip.infusedTable)
			return ip, func(_ *testing.T, ip *infusedPlay) error {
				_, err := ip.move(ip.dona, first, infusedDagger, "")
				return err
			}
		}, true},
		{"moving a valid record under the limit", func(t *testing.T) (*infusedPlay, call) {
			t.Helper()
			ip := newInfusedPlay(t)
			blade := ip.mustInfuseOn(t, infusionKeyBlade, infusionMace).GetInfusion().GetId()
			ip.h.clock.Advance(time.Minute)
			return ip, func(_ *testing.T, ip *infusedPlay) error {
				_, err := ip.move(ip.dona, blade, infusedDagger, "")
				return err
			}
		}, false},
		{"naming the record on the piece that is not valid", func(t *testing.T) (*infusedPlay, call) {
			t.Helper()
			ip, cloak := lostPieceTable(t)
			return ip, infuse(infusionKeyPlate, infusionLeather, cloak.ID)
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ip, run := tc.setup(t)
			valid := ip.validNow(t)
			was := ip.before(t)
			err := run(t, ip)
			endedWithEvent := map[string]bool{}
			for _, c := range ip.calls() {
				for _, ch := range c {
					if ch.Kind == link.InfusionEnded {
						endedWithEvent[ch.RecordID] = true
					}
				}
			}
			if tc.refused {
				wantInfusionRefusal(t, tc.name, err, itemChangedR)
				if len(endedWithEvent) != 0 {
					t.Errorf("a refusal ended %v", endedWithEvent)
				}
			} else if err != nil {
				t.Fatalf("%s error = %v", tc.name, err)
			}
			after := ip.validNow(t)
			for _, id := range valid {
				if !slices.Contains(after, id) && !endedWithEvent[id] {
					t.Errorf("the record %s was valid and is not, and no ending was told: valid before %v, after %v", id, valid, after)
				}
			}
			if tc.refused && !reflect.DeepEqual(ip.allRecords(t), was.records) {
				t.Error("a refusal changed the records")
			}
		})
	}
}

// The ids of another campaign's records and of another character's are answered as a random id is: ITEM_CHANGED, saying nothing
// of the record.
func TestInfuseItemAndMoveAndEndAnswerAnIdThatIsNotTheArtificersAsARandomOne(t *testing.T) {
	t.Parallel()
	it := newInfusedTable(t)
	it.mustInfuseOn(t, infusionKeyBlade, infusionMace)
	otherCampaign := it.h.newCampaign(it.master, "Outra campanha", it.dona)
	elsewhere := it.dona.createPensantus(t, otherCampaign)
	foreign, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: otherCampaign, ArtificerID: elsewhere.GetId(), HolderID: elsewhere.GetId(), InfusionKey: infusionKeyBlade, ItemKind: infusionItemGear, GearKey: infusionMace,
	})
	if err != nil {
		t.Fatal(err)
	}
	stranger := it.caio.create(t, it.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Outro", it.read(t).GetSheet())
	sibling, err := it.h.svc.insertInfusion(t.Context(), it.h.svc.queries, newInfusion{
		CampaignID: it.campaign, ArtificerID: stranger.GetId(), HolderID: stranger.GetId(), InfusionKey: infusionKeyBlade, ItemKind: infusionItemGear, GearKey: infusionMace,
	})
	if err != nil {
		t.Fatal(err)
	}
	random := uuid.New().String()
	calls := map[string]func(id string) error{
		"InfuseItem": func(id string) error {
			req := it.infuseReq(infusionKeyPlate)
			req.ItemKey, req.EndInfusionId = infusionLeather, id
			_, err := it.dona.infusions.InfuseItem(t.Context(), connect.NewRequest(req))
			return err
		},
		"MoveInfusion": func(id string) error {
			_, err := it.move(it.dona, id, infusedDagger, "")
			return err
		},
		"EndInfusion": func(id string) error {
			_, err := it.dona.infusions.EndInfusion(t.Context(), connect.NewRequest(&charactersv1.EndInfusionRequest{CampaignId: it.campaign, CharacterId: it.pc.GetId(), InfusionId: id}))
			return err
		},
	}
	for name, call := range calls {
		want := call(random)
		wantInfusionRefusal(t, name+"(a random id)", want, itemChangedR)
		for label, id := range map[string]string{"another campaign": foreign.ID, "another character": sibling.ID} {
			got := call(id)
			wantInfusionRefusal(t, name+"("+label+")", got, itemChangedR)
			if got.Error() != want.Error() {
				t.Errorf("%s(%s) = %q, want the answer of a random id, %q", name, label, got, want)
			}
		}
	}
	for _, r := range []charactersdb.CharacterInfusion{foreign, sibling} {
		var live bool
		if err := it.h.pool.QueryRow(t.Context(), `SELECT ended_at IS NULL FROM character_infusions WHERE id = $1`, r.ID).Scan(&live); err != nil || !live {
			t.Errorf("the record %s of another character or campaign is live = %v, %v, want it untouched", r.ID, live, err)
		}
	}
}
