package play

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// failingRules is a roster whose rules cannot be read, and counts how many times they were asked for.
type failingRules struct {
	noRoster
	asked *int
}

func (r failingRules) RulesContent(context.Context, pgx.Tx, string) (*rules.Content, error) {
	*r.asked++
	return nil, errors.New("down")
}

func TestTheLogReadsTheRulesOnceEvenWhenTheReadFails(t *testing.T) {
	asked := 0
	names := &keyNames{s: &Service{roster: failingRules{asked: &asked}, logger: slog.New(slog.DiscardHandler)}, campaignID: "camp"}
	for range 3 {
		if got := names.rulesContent(context.Background()); got != nil {
			t.Fatalf("rulesContent = %v, want nil when the rules cannot be read", got)
		}
	}
	if asked != 1 {
		t.Fatalf("the rules were asked for %d times, want 1 for the whole call", asked)
	}
}
