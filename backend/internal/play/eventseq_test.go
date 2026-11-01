package play

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// TestAnEventNumberTakenByAConcurrentChangeIsASerializationFailure: the second event written
// with a number already taken answers 40001 (which the transaction loop retries), with the
// unique violation still in the error; an event with a free number is written as it was.
func TestAnEventNumberTakenByAConcurrentChangeIsASerializationFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")
	session := master.start(t, campaign).GetGameSession()
	q := h.svc.queries
	event := func(seq int32) playdb.InsertSessionEventParams {
		return playdb.InsertSessionEventParams{GameSessionID: session.GetId(), Seq: seq, Kind: eventCharacterVitalsAdjusted, Payload: []byte(`{}`), CreatedAt: time.Now()}
	}
	next, err := q.NextSessionEventSeq(t.Context(), session.GetId())
	if err != nil {
		t.Fatalf("NextSessionEventSeq() error = %v", err)
	}
	if _, err := insertSessionEvent(t.Context(), q, event(next)); err != nil {
		t.Fatalf("insertSessionEvent(a free number) error = %v", err)
	}
	_, err = insertSessionEvent(t.Context(), q, event(next))
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || pgErr.Code != "40001" {
		t.Fatalf("insertSessionEvent(a number already taken) error = %v, want SQLSTATE 40001", err)
	}
	if !strings.Contains(err.Error(), "SQLSTATE 23505") {
		t.Errorf("insertSessionEvent(a number already taken) error = %v, want the unique violation kept in it", err)
	}
}
