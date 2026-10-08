package characters

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The methods other modules call inside their own db.InTx closure map read
// errors with s.dbError. The error it returns must keep the pgconn 40001 within
// reach, so db.InTx (cockroach-go) still retries a conflict a second attempt
// survives, instead of the client getting `aborted`.

// conflictTx is a pgx.Tx whose statements fail with the error CockroachDB
// sends for a serialization conflict: a *pgconn.PgError with SQLSTATE 40001.
// (crdb_internal.force_retry is restricted on this server, so the error is
// built, not provoked; it is the same type and code the driver returns.)
// Every other method is the real transaction's.
type conflictTx struct {
	pgx.Tx
	err error
}

func newConflictTx(tx pgx.Tx) conflictTx {
	return conflictTx{Tx: tx, err: &pgconn.PgError{Severity: "ERROR", Code: "40001", Message: "restart transaction: TransactionRetryWithProtoRefreshError"}}
}

func (c conflictTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, c.err }
func (c conflictTx) QueryRow(context.Context, string, ...any) pgx.Row        { return errRow{c.err} }

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

type srdContent struct{ srd *rules.Content }

func (c srdContent) For(context.Context, pgx.Tx, string) (*rules.Content, TableRules, error) {
	return c.srd, TableRules{}, nil
}

func (c srdContent) ContentFor(context.Context, pgx.Tx, string) (*rules.Content, error) {
	return c.srd, nil
}

// failingContent returns a 40001 from the content read, as a conflict on the
// campaign's content revision would.
type failingContent struct{ srdContent }

func (failingContent) ContentFor(context.Context, pgx.Tx, string) (*rules.Content, error) {
	return nil, &pgconn.PgError{Code: "40001", Message: "restart transaction"}
}

func TestTxMethodsKeepRetryableErrors(t *testing.T) {
	pool := dbtest.NewPool(t, "meurpg_characters_test")
	srd := loadRules(t)
	const campaign = "00000000-0000-0000-0000-000000000001"
	const char = "00000000-0000-0000-0000-000000000002"

	cases := []struct {
		name string
		// failQuery: the method's own SQL statement fails with a real 40001;
		// otherwise the content read (failingContent) does.
		failQuery bool
		call      func(s *Service, ctx context.Context, tx pgx.Tx) error
	}{
		{"CombatParty/query", true, func(s *Service, ctx context.Context, tx pgx.Tx) error {
			_, err := s.CombatParty(ctx, tx, campaign)
			return err
		}},
		{"CombatParty/content", false, func(s *Service, ctx context.Context, tx pgx.Tx) error {
			_, err := s.CombatParty(ctx, tx, campaign)
			return err
		}},
		{"PartyLevels/query", true, func(s *Service, ctx context.Context, tx pgx.Tx) error {
			_, err := s.PartyLevels(ctx, tx, campaign)
			return err
		}},
		{"SessionCharacters/query", true, func(s *Service, ctx context.Context, tx pgx.Tx) error {
			_, err := s.SessionCharacters(ctx, tx, campaign, []string{char})
			return err
		}},
		{"CombatCharacters/query", true, func(s *Service, ctx context.Context, tx pgx.Tx) error {
			_, err := s.CombatCharacters(ctx, tx, campaign, []string{char})
			return err
		}},
		{"GetVitalsTx/queryrow", true, func(s *Service, ctx context.Context, tx pgx.Tx) error {
			_, err := s.GetVitalsTx(ctx, tx, campaign, char)
			return err
		}},
		// Control: RulesContent wraps with %w (wrap), so it keeps the 40001.
		{"RulesContent/content (wrap, control)", false, func(s *Service, ctx context.Context, tx pgx.Tx) error {
			_, err := s.RulesContent(ctx, tx, campaign)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cs ContentSource = srdContent{srd}
			if !tc.failQuery {
				cs = failingContent{srdContent{srd}}
			}
			s := &Service{pool: pool, queries: charactersdb.New(pool), content: cs, srd: srd, logger: slog.New(slog.DiscardHandler)}
			ctx := t.Context()
			attempts := 0
			err := db.InTx(ctx, pool, func(tx pgx.Tx) error {
				attempts++
				if attempts > 1 {
					return nil // the retry: the conflict is gone
				}
				if tc.failQuery {
					return tc.call(s, ctx, newConflictTx(tx))
				}
				return tc.call(s, ctx, tx)
			})
			if attempts < 2 {
				ce, isConnect := errors.AsType[*connect.Error](err)
				code := connect.Code(0)
				if isConnect {
					code = ce.Code()
				}
				t.Errorf("db.InTx ran the closure %d time(s), want a retry (2): the 40001 was hidden from InTx; err = %v (connect code %v)", attempts, err, code)
			}
		})
	}
}
