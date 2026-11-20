package characters

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// infusionTable is a campaign with two player characters: an artificer and a friend.
type infusionTable struct {
	h        *harness
	campaign string
	artif    string
	friend   string
}

func newInfusionTable(t *testing.T) *infusionTable {
	t.Helper()
	h := newHarness(t)
	master, a, b := h.newUser("Mestre"), h.newUser("Artifice"), h.newUser("Amigo")
	campaign := h.newCampaign(master, "Mirathel", a, b)
	return &infusionTable{h: h, campaign: campaign, artif: a.createPensantus(t, campaign).GetId(), friend: b.createPensantus(t, campaign).GetId()}
}

func (tb *infusionTable) insert(t *testing.T, holder, itemKey string) charactersdb.CharacterInfusion {
	t.Helper()
	rec, err := tb.h.svc.insertInfusion(t.Context(), tb.h.svc.queries, newInfusion{
		CampaignID: tb.campaign, ArtificerID: tb.artif, HolderID: holder,
		InfusionKey: "infusion:enhanced_defense", ItemKind: infusionItemGear, GearKey: itemKey,
	})
	if err != nil {
		t.Fatalf("insertInfusion(%q) error = %v", itemKey, err)
	}
	return rec
}

// get reads a record of the campaign, live or ended.
func (tb *infusionTable) get(t *testing.T, id string) charactersdb.CharacterInfusion {
	t.Helper()
	rec, err := tb.h.svc.queries.GetCharacterInfusionForUpdate(t.Context(), charactersdb.GetCharacterInfusionForUpdateParams{CampaignID: tb.campaign, ID: id})
	if err != nil {
		t.Fatalf("read the record %s: %v", id, err)
	}
	return rec
}

func TestInfusionStoreInsertAndLists(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	first := tb.insert(t, tb.artif, "equipment:shield")
	second := tb.insert(t, tb.artif, "equipment:chain-mail")
	if first.EndedAt != nil || first.EndedReason != nil || first.Attuned || first.ObjectName != "" || first.Parameter != "" {
		t.Errorf("a new record = %+v, want a live one with the defaults", first)
	}
	live, err := tb.h.svc.queries.ListLiveInfusionsOfHolder(t.Context(), tb.artif)
	if err != nil || len(live) != 2 || live[0].ID != first.ID || live[1].ID != second.ID {
		t.Fatalf("live of the holder = %v, %v; want the first then the second", live, err)
	}
	// An ended record leaves the live list.
	if _, ok, err := tb.h.svc.endInfusion(t.Context(), tb.h.svc.queries, tb.campaign, first.ID, infusionEndedReplaced); err != nil || !ok {
		t.Fatalf("endInfusion() = %v, %v; want true", ok, err)
	}
	held, err := tb.h.svc.queries.ListLiveInfusionsOfHolder(t.Context(), tb.artif)
	if err != nil || len(held) != 1 || held[0].ID != second.ID {
		t.Errorf("live of the holder = %v, %v; want only the second", held, err)
	}
	if other, _ := tb.h.svc.queries.ListLiveInfusionsOfHolder(t.Context(), tb.friend); len(other) != 0 {
		t.Errorf("live of another holder = %v, want none", other)
	}
}

func TestInfusionStoreListOrdersBySinceThenID(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	at := tb.h.clock.Now()
	ids := []string{"00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000001"}
	for i, id := range ids {
		if _, err := tb.h.pool.Exec(t.Context(), `INSERT INTO character_infusions (id, campaign_id, artificer_character_id, holder_character_id, infusion_key, item_kind, item_key, since)
			VALUES ($1, $2, $3, $3, 'infusion:x', 'gear', $4, $5)`, id, tb.campaign, tb.artif, "equipment:k"+string(rune('a'+i)), at); err != nil {
			t.Fatal(err)
		}
	}
	live, err := tb.h.svc.queries.ListLiveInfusionsOfHolder(t.Context(), tb.artif)
	if err != nil || len(live) != 2 || live[0].ID != ids[1] || live[1].ID != ids[0] {
		t.Errorf("same since: order = %v, %v; want the lower id first", live, err)
	}
}

func TestInfusionStoreReadsTheLiveRowInATransaction(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	rec := tb.insert(t, tb.artif, "equipment:shield")
	err := db.InTx(t.Context(), tb.h.pool, func(tx pgx.Tx) error {
		got, err := tb.h.svc.queries.WithTx(tx).GetCharacterInfusionForUpdate(t.Context(), charactersdb.GetCharacterInfusionForUpdateParams{CampaignID: tb.campaign, ID: rec.ID})
		if err != nil || got.ID != rec.ID {
			t.Errorf("GetCharacterInfusionForUpdate() = %v, %v", got.ID, err)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tb.h.svc.queries.GetCharacterInfusionForUpdate(t.Context(), charactersdb.GetCharacterInfusionForUpdateParams{CampaignID: tb.campaign, ID: "00000000-0000-4000-8000-0000000000ff"}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("an unknown id: error = %v, want no rows", err)
	}
}

func TestInfusionStoreEndOnlyALiveRecord(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	rec := tb.insert(t, tb.artif, "equipment:shield")
	_, ok, err := tb.h.svc.endInfusion(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, infusionEndedEnded)
	if err != nil || !ok {
		t.Fatalf("first end = %v, %v; want true", ok, err)
	}
	ended, _ := tb.h.svc.queries.GetCharacterInfusionForUpdate(t.Context(), charactersdb.GetCharacterInfusionForUpdateParams{CampaignID: tb.campaign, ID: rec.ID})
	if ended.EndedAt == nil || ended.EndedReason == nil || *ended.EndedReason != infusionEndedEnded {
		t.Fatalf("the ended record = %+v, want an end time and the reason", ended)
	}
	tb.h.clock.Advance(time.Hour)
	_, ok, err = tb.h.svc.endInfusion(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, infusionEndedItemGone)
	if err != nil || ok {
		t.Errorf("second end = %v, %v; want false, nil", ok, err)
	}
	again, _ := tb.h.svc.queries.GetCharacterInfusionForUpdate(t.Context(), charactersdb.GetCharacterInfusionForUpdateParams{CampaignID: tb.campaign, ID: rec.ID})
	if !again.EndedAt.Equal(*ended.EndedAt) || *again.EndedReason != infusionEndedEnded {
		t.Errorf("ending an ended record changed it: %v %q, was %v %q", again.EndedAt, *again.EndedReason, ended.EndedAt, *ended.EndedReason)
	}
}

func TestInfusionStoreMoveOnlyALiveRecord(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	rec := tb.insert(t, tb.artif, "equipment:shield")
	if ok, err := tb.h.svc.setInfusionAttuned(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, true); err != nil || !ok {
		t.Fatalf("attune = %v, %v", ok, err)
	}
	tb.h.clock.Advance(time.Hour)
	ok, err := tb.h.svc.moveInfusion(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, infusionItemObject, "ignored", "Bengala do Vovô")
	if err != nil || !ok {
		t.Fatalf("move to an object = %v, %v; want true", ok, err)
	}
	moved := tb.get(t, rec.ID)
	if moved.ItemKind != infusionItemObject || moved.ItemKey != "object:"+rec.ID || moved.ObjectName != "Bengala do Vovô" || moved.Attuned || !moved.Since.After(rec.Since.Add(time.Hour-time.Second)) {
		t.Errorf("the record moved to an object = %+v, want its own object key, the name, no attunement and a new since", moved)
	}
	if ok, err := tb.h.svc.moveInfusion(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, infusionItemGear, "equipment:chain-mail", ""); err != nil || !ok {
		t.Fatalf("move to gear = %v, %v; want true", ok, err)
	}
	if back := tb.get(t, rec.ID); back.ItemKind != infusionItemGear || back.ItemKey != "equipment:chain-mail" || back.ObjectName != "" {
		t.Errorf("the record moved back to gear = %+v", back)
	}
	if _, _, err := tb.h.svc.endInfusion(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, infusionEndedEnded); err != nil {
		t.Fatal(err)
	}
	ok, err = tb.h.svc.moveInfusion(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, infusionItemGear, "equipment:shield", "")
	if err != nil || ok {
		t.Errorf("moving an ended record = %v, %v; want false, nil", ok, err)
	}
}

func TestInfusionObjectRecordKeepsItsOwnKey(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	one, err := tb.h.svc.insertInfusion(t.Context(), tb.h.svc.queries, newInfusion{CampaignID: tb.campaign, ArtificerID: tb.artif, HolderID: tb.artif, InfusionKey: "infusion:x", ItemKind: infusionItemObject, ObjectName: "Bengala"})
	if err != nil {
		t.Fatalf("insert an object: %v", err)
	}
	two, err := tb.h.svc.insertInfusion(t.Context(), tb.h.svc.queries, newInfusion{CampaignID: tb.campaign, ArtificerID: tb.artif, HolderID: tb.artif, InfusionKey: "infusion:x", ItemKind: infusionItemObject, ObjectName: "Bengala"})
	if err != nil {
		t.Fatalf("insert a second object of the same name: %v", err)
	}
	if one.ItemKey != "object:"+one.ID || two.ItemKey != "object:"+two.ID || one.ItemKey == two.ItemKey {
		t.Errorf("object keys = %q, %q; want each its own id", one.ItemKey, two.ItemKey)
	}
}

func TestInfusionOtherCampaignTouchesNothing(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	rec := tb.insert(t, tb.artif, "equipment:shield")
	other := "00000000-0000-4000-8000-0000000000aa"
	q := tb.h.svc.queries
	if _, ok, err := tb.h.svc.endInfusion(t.Context(), q, other, rec.ID, infusionEndedEnded); err != nil || ok {
		t.Errorf("end from another campaign = %v, %v; want false, nil", ok, err)
	}
	if ok, err := tb.h.svc.moveInfusion(t.Context(), q, other, rec.ID, infusionItemGear, "equipment:chain-mail", ""); err != nil || ok {
		t.Errorf("move from another campaign = %v, %v; want false, nil", ok, err)
	}
	if ok, err := tb.h.svc.setInfusionAttuned(t.Context(), q, other, rec.ID, true); err != nil || ok {
		t.Errorf("attune from another campaign = %v, %v; want false, nil", ok, err)
	}
	if _, err := q.GetCharacterInfusionForUpdate(t.Context(), charactersdb.GetCharacterInfusionForUpdateParams{CampaignID: other, ID: rec.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("read from another campaign: error = %v, want no rows", err)
	}
	if got := tb.get(t, rec.ID); got.EndedAt != nil || got.ItemKey != "equipment:shield" || got.Attuned {
		t.Errorf("the record changed: %+v", got)
	}
}

func TestInfusionStoreSetAttuned(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	rec := tb.insert(t, tb.artif, "equipment:shield")
	if ok, err := tb.h.svc.setInfusionAttuned(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, true); err != nil || !ok {
		t.Fatalf("set attuned = %v, %v; want true", ok, err)
	}
	got, _ := tb.h.svc.queries.GetCharacterInfusionForUpdate(t.Context(), charactersdb.GetCharacterInfusionForUpdateParams{CampaignID: tb.campaign, ID: rec.ID})
	if !got.Attuned {
		t.Error("the record is not attuned")
	}
	if _, _, err := tb.h.svc.endInfusion(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, infusionEndedEnded); err != nil {
		t.Fatal(err)
	}
	if ok, err := tb.h.svc.setInfusionAttuned(t.Context(), tb.h.svc.queries, tb.campaign, rec.ID, false); err != nil || ok {
		t.Errorf("set attuned on an ended record = %v, %v; want false, nil", ok, err)
	}
}

func TestInfusionOneLiveRecordForAnItem(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	first := tb.insert(t, tb.artif, "equipment:shield")
	_, err := tb.h.svc.insertInfusion(t.Context(), tb.h.svc.queries, newInfusion{
		CampaignID: tb.campaign, ArtificerID: tb.artif, HolderID: tb.artif,
		InfusionKey: "infusion:enhanced_weapon", ItemKind: infusionItemGear, GearKey: "equipment:shield",
	})
	if !db.IsUniqueViolation(err, "character_infusions_one_live_item_idx") {
		t.Fatalf("a second live record on the same item: error = %v, want the unique index", err)
	}
	// The same item of another holder is another record.
	tb.insert(t, tb.friend, "equipment:shield")
	if _, _, err := tb.h.svc.endInfusion(t.Context(), tb.h.svc.queries, tb.campaign, first.ID, infusionEndedTaken); err != nil {
		t.Fatal(err)
	}
	tb.insert(t, tb.artif, "equipment:shield")
}

func TestInfusionRecordsGoWithTheCharacter(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	tb.insert(t, tb.friend, "equipment:shield")
	tb.insert(t, tb.artif, "equipment:chain-mail")
	// Deleting the holder deletes the records on it, the artificer's own stay.
	if _, err := tb.h.pool.Exec(t.Context(), `DELETE FROM characters WHERE id = $1`, tb.friend); err != nil {
		t.Fatal(err)
	}
	if left, _ := tb.h.svc.queries.ListLiveInfusionsOfHolder(t.Context(), tb.artif); len(left) != 1 {
		t.Errorf("after deleting the holder: %d records, want 1", len(left))
	}
	if _, err := tb.h.pool.Exec(t.Context(), `DELETE FROM characters WHERE id = $1`, tb.artif); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := tb.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM character_infusions WHERE campaign_id = $1`, tb.campaign).Scan(&left); err != nil || left != 0 {
		t.Errorf("after deleting the artificer: %d records, %v; want none", left, err)
	}
}

func TestInfusionEndReasonAndEndTimeConstraints(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	rec := tb.insert(t, tb.artif, "equipment:shield")
	cases := []struct {
		name, set, constraint string
	}{
		{"an unknown reason", `ended_at = now(), ended_reason = 'forgotten'`, "character_infusions_ended_valid"},
		{"a reason with no time", `ended_reason = 'ended'`, "character_infusions_ended_valid"},
		{"a time with no reason", `ended_at = now()`, "character_infusions_ended_valid"},
		{"an unknown item kind", `item_kind = 'spell'`, "character_infusions_item_kind_valid"},
		{"an object with a forged key", `item_kind = 'object', item_key = 'object:00000000-0000-4000-8000-000000000001'`, "character_infusions_object_key_valid"},
		{"an object with a gear key", `item_kind = 'object', item_key = 'equipment:shield'`, "character_infusions_object_key_valid"},
		{"a gear key of an object of another id", `item_key = 'object:00000000-0000-4000-8000-000000000001'`, "character_infusions_gear_key_valid"},
		{"an object name over 40 characters", `object_name = repeat('a', 41)`, "character_infusions_object_name_length"},
	}
	for _, c := range cases {
		_, err := tb.h.pool.Exec(t.Context(), `UPDATE character_infusions SET `+c.set+` WHERE id = $1`, rec.ID)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "23514" || pg.ConstraintName != c.constraint {
			t.Errorf("%s: error = %v, want a check violation of %s", c.name, err, c.constraint)
		}
	}
	for _, reason := range []string{infusionEndedEnded, infusionEndedReplaced, infusionEndedSwapped, infusionEndedItemGone, infusionEndedTaken} {
		row := tb.insert(t, tb.friend, "equipment:"+reason)
		if _, err := tb.h.pool.Exec(t.Context(), `UPDATE character_infusions SET ended_at = now(), ended_reason = $2 WHERE id = $1`, row.ID, reason); err != nil {
			t.Errorf("the reason %q is refused: %v", reason, err)
		}
	}
}

func TestInfusionObjectNameOf40CharactersIsAllowed(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	rec := tb.insert(t, tb.artif, "equipment:shield")
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE character_infusions SET object_name = repeat('é', 40) WHERE id = $1`, rec.ID); err != nil {
		t.Errorf("a name of 40 characters is refused: %v", err)
	}
}

func TestInfusionRecordsGoWithTheCampaign(t *testing.T) {
	t.Parallel()
	tb := newInfusionTable(t)
	tb.insert(t, tb.friend, "equipment:shield")
	tb.insert(t, tb.artif, "equipment:chain-mail")
	if _, err := tb.h.pool.Exec(t.Context(), `DELETE FROM campaigns WHERE id = $1`, tb.campaign); err != nil {
		t.Fatal(err)
	}
	var mine int
	if err := tb.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM character_infusions WHERE campaign_id = $1`, tb.campaign).Scan(&mine); err != nil || mine != 0 {
		t.Errorf("after deleting the campaign: %d records, %v; want none", mine, err)
	}
}
