package progression

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/internal/campaigns"
	"github.com/PuraFome/meuRPG/backend/internal/characters"
	"github.com/PuraFome/meuRPG/backend/internal/characters/contenttest"
	"github.com/PuraFome/meuRPG/backend/internal/identity"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/play"
)

// flakyCampaigns fails the first read made inside a transaction with a
// serialization conflict (40001), wrapped with %w as the repo asks.
type flakyCampaigns struct {
	Campaigns
	failed  atomic.Bool
	txCalls atomic.Int32
}

func (f *flakyCampaigns) CampaignXPMode(ctx context.Context, tx pgx.Tx, id string) (campaignsv1.XpMode, error) {
	if tx != nil {
		f.txCalls.Add(1)
		if f.failed.CompareAndSwap(false, true) {
			return 0, fmt.Errorf("read the campaign's XP mode: %w", &pgconn.PgError{Code: "40001", Message: "restart transaction"})
		}
	}
	return f.Campaigns.CampaignXPMode(ctx, tx, id)
}

// Finding U9-01: a 40001 raised inside db.InTx by requireMilestonesMode is turned into connect Aborted by rpcerr.FromDB (no cause) and never retried.
func TestReview9_SerializationRetry(t *testing.T) {
	pool := dbtest.NewPool(t, "meurpg_progression_test")
	users := identity.NewPostgresStore(pool)
	clock := &fakeClock{now: time.Now().Truncate(time.Microsecond)}
	logger := slog.New(slog.DiscardHandler)
	content, err := testRules()
	if err != nil {
		t.Fatal(err)
	}
	camps, err := campaigns.New(campaigns.Config{Pool: pool, Profiles: users, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	chars, err := characters.New(characters.Config{Pool: pool, Profiles: users, Members: camps, Content: contenttest.NewSource(pool, content), SRD: content, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	pl, err := play.New(play.Config{Pool: pool, Sheets: chars, Vitals: chars, Campaigns: camps, Maps: maps.NewSessionMaps(pool), Roster: chars, Dice: testDice{camps}, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	flaky := &flakyCampaigns{Campaigns: camps}
	svc, err := New(Config{Pool: pool, Party: chars, Combats: pl, Log: pl, Treasures: maps.NewTreasures(pool), Campaigns: flaky, Profiles: users, Logger: logger, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	chars.SetLevelUps(svc)
	camps.SetXPAwards(svc)
	srv := httpserver.New(httpserver.Config{Logger: logger})
	opt := connect.WithRequireConnectProtocolHeader()
	var sessions Sessions = fakeSessions{}
	camps.Mount(srv.Handle, sessions.(campaigns.Sessions), opt)
	chars.Mount(srv.Handle, sessions.(characters.Sessions), camps, opt)
	pl.Mount(srv.Handle, sessions.(play.Sessions), camps, opt)
	svc.Mount(srv.Handle, sessions, camps, opt)
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	t.Cleanup(pl.Close)
	h := &harness{t: t, pool: pool, users: users, play: pl, server: server}

	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Marcos", campaignsv1.XpMode_XP_MODE_MILESTONES)

	flaky.failed.Store(false) // arm the failure only now: setup is done
	flaky.txCalls.Store(0)
	_, err = master.xp.AddMilestone(t.Context(), connect.NewRequest(&progressionv1.AddMilestoneRequest{CampaignId: campaign, Text: "Derrotar o dragao"}))
	if err != nil {
		t.Errorf("AddMilestone() after one 40001 error = %v (code %v), want success after a retry; tx reads = %d", err, connect.CodeOf(err), flaky.txCalls.Load())
	}
}
