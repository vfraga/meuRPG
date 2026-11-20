package characters

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
)

// A pick of a feature group is given up through the swap of the feature's options and the record, not through the
// warlock's invocation field: a request that names the pick there and takes the new option as a plain pick is refused, with
// nothing written and nobody told.
func TestAPickCannotBeGivenUpAroundTheSwapRecord(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 6500, swapAt(5), nil)
	spy := &optionSwapSpy{}
	f.h.svc.SetOptionSwaps(spy)
	ch := f.choices(false)
	ch.SwapInvocationKey = f.heron
	ch.FeatureChoiceKeys = []string{f.crow}
	if r := f.swapRefusal(t, ch); r == nil {
		t.Error("preview: the request went around the swap record and was not refused")
	}
	_, err := f.levelUp(f.other, f.fighter, ch)
	if r := refusal(t, "LevelUpCharacter", err); r.GetReason() == charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_UNSPECIFIED {
		t.Errorf("level-up refusal = %v", r)
	}
	again := f.other.get(t, f.campaign, f.fighter.GetId())
	if again.GetRevision() != f.fighter.GetRevision() || again.GetSheet().GetFull().GetClasses()[0].GetLevel() != 4 {
		t.Errorf("a refused level-up changed the sheet: revision %d (was %d)", again.GetRevision(), f.fighter.GetRevision())
	}
	if records, _ := f.levelUps(f.master, f.fighter.GetId()); len(records) != 0 {
		t.Errorf("level ups = %d, want none", len(records))
	}
	if len(spy.calls) != 0 {
		t.Errorf("the module was called %d times", len(spy.calls))
	}
}

// useResource spends n uses of the resource and starts the state, through the vitals as the live session does.
func (f *swapFixture) useResource(t *testing.T, resource, state string, n int32) {
	t.Helper()
	ctx := t.Context()
	err := db.InTx(ctx, f.h.pool, func(tx pgx.Tx) error {
		_, _, err := f.h.svc.AdjustVitals(ctx, tx, f.campaign, f.fighter.GetId(), &playv1.AdjustCharacterVitalsRequest{ResourcesUsed: []*playv1.ResourceUsed{{Key: resource, Used: n}}})
		if err != nil {
			return err
		}
		_, _, err = f.h.svc.SetActiveStates(ctx, tx, f.campaign, f.fighter.GetId(), []*playv1.ActiveState{{Key: state}})
		return err
	})
	if err != nil {
		t.Fatalf("spend %s and start %s: %v", resource, state, err)
	}
}

// stored reads the stored used counts of the resources and the states that are on, as the columns hold them.
func (f *swapFixture) stored(t *testing.T) (resources, states string) {
	t.Helper()
	err := f.h.pool.QueryRow(t.Context(), `SELECT COALESCE((SELECT resources_used::TEXT FROM character_vitals WHERE character_id = c.id), '{}'), COALESCE(c.active_states::TEXT, '[]')
		FROM characters AS c WHERE c.id = $1`, f.fighter.GetId()).Scan(&resources, &states)
	if err != nil {
		t.Fatalf("read the stored uses and states: %v", err)
	}
	return resources, states
}

// What the given-up option had stored goes with it: its resources' used counts and the state it started. The option picked again
// later has its uses fresh.
func TestAnOptionGivenUpTakesItsUsedCountsAndStatesWithIt(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 14000, swapEvery(), nil)
	state := f.heron + "#garca"
	f.useResource(t, "garca", state, 1)
	if res, states := f.stored(t); !strings.Contains(res, `"garca": 1`) || !strings.Contains(states, state) {
		t.Fatalf("before the swap: resources %s, states %s, want the use and the state stored", res, states)
	}

	away := f.choices(false)
	away.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.heron, f.crow)}
	fighter, err := f.levelUp(f.other, f.fighter, away)
	if err != nil {
		t.Fatalf("level 5, swapping the Heron away: %v", err)
	}
	if res, states := f.stored(t); strings.Contains(res, "garca") || strings.Contains(states, state) {
		t.Errorf("after the swap: resources %s, states %s, want neither the use nor the state of the Heron", res, states)
	}

	back := f.choices(false)
	back.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.crow, f.heron)}
	if _, err := f.levelUp(f.other, fighter, back); err != nil {
		t.Fatalf("level 6, taking the Heron back: %v", err)
	}
	if res, states := f.stored(t); strings.Contains(res, "garca") || strings.Contains(states, state) {
		t.Errorf("after taking it back: resources %s, states %s, want it fresh", res, states)
	}
}

// writingSpy is a module that writes through the transaction it is given, then answers err.
type writingSpy struct{ err error }

func (s writingSpy) OptionsGivenUp(ctx context.Context, tx pgx.Tx, _, characterID string, _ []OptionSwap, _ sessiontell.Tell) error {
	if _, err := tx.Exec(ctx, `UPDATE characters SET name = 'Escrito pelo módulo' WHERE id = $1`, characterID); err != nil {
		return err
	}
	return s.err
}

// The transaction the module gets is the level-up's own: what it writes through it is saved with the level, and is gone when the
// module answers with an error.
func TestTheModuleWritesInTheLevelUpsOwnTransaction(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 6500, swapAt(5), nil)
	swapped := f.choices(false)
	swapped.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.heron, f.crow)}
	nameOf := func() string {
		var name string
		if err := f.h.pool.QueryRow(t.Context(), `SELECT name FROM characters WHERE id = $1`, f.fighter.GetId()).Scan(&name); err != nil {
			t.Fatalf("read the name: %v", err)
		}
		return name
	}

	f.h.svc.SetOptionSwaps(writingSpy{err: errors.New("the module is down")})
	if _, err := f.levelUp(f.other, f.fighter, swapped); err == nil {
		t.Fatal("the level-up went through though the module refused")
	}
	if got := nameOf(); got != "Ícaro" {
		t.Errorf("name = %q after the module's error, want the write rolled back with the level", got)
	}
	f.h.svc.SetOptionSwaps(writingSpy{})
	if _, err := f.levelUp(f.other, f.fighter, swapped); err != nil {
		t.Fatalf("level-up: %v", err)
	}
	if got := nameOf(); got != "Escrito pelo módulo" {
		t.Errorf("name = %q after the level, want the module's write saved with it", got)
	}
}

// The swap levels are the class's own: a fighter of class level 4 that has a barbarian level too (total level 5) is offered the
// swap of a feature that allows it at class level 5, not at total level 6, and the module hears the total level the record holds.
func TestTheSwapLevelsFollowTheClassLevelNotTheTotal(t *testing.T) {
	t.Parallel()
	barbarian := &charactersv1.ClassLevel{ClassKey: "class:barbarian", Level: 1}
	notYet := newSwapFixtureWith(t, 4, 14000, swapAt(6), nil, barbarian)
	if o := notYet.fighterOptions(t); len(o.GetSwapGroups()) != 0 {
		t.Errorf("fighter 5 of total level 6 offers %v with a swap at class level 6, want none", o.GetSwapGroups())
	}
	f := newSwapFixtureWith(t, 4, 14000, swapAt(5), nil, barbarian)
	spy := &optionSwapSpy{}
	f.h.svc.SetOptionSwaps(spy)
	if o := f.fighterOptions(t); groupOf(o, f.postures.GetKey()) == nil || o.GetTotalToLevel() != 6 {
		t.Fatalf("offer = %v (total to level %d), want the Postures group at class level 5", o.GetSwapGroups(), o.GetTotalToLevel())
	}
	ch := f.choices(false)
	ch.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.heron, f.crow)}
	if _, err := f.levelUp(f.other, f.fighter, ch); err != nil {
		t.Fatalf("level-up: %v", err)
	}
	if len(spy.calls) != 1 || len(spy.calls[0].swaps) != 1 || spy.calls[0].swaps[0].ToLevel != 6 || spy.calls[0].swaps[0].ClassKey != "class:fighter" {
		t.Errorf("heard %+v, want the fighter's swap at total level 6", spy.calls)
	}
}

// The pool of a swap holds the options of the feature the master left on: one the master archived, or switched off for the players,
// is never offered, and a swap that takes it is refused as one the level does not allow.
func TestTheSwapPoolHoldsNoArchivedOrSwitchedOffOption(t *testing.T) {
	t.Parallel()
	f := newSwapFixture(t, 4, 6500, swapAt(5), nil)
	option := func(name string) string {
		return f.master.addEntry(t, f.campaign, &rulesv1.TableOption{
			NamePt: name, Set: "table_feature", OfferedBy: f.postures.GetKey(), DescPt: []string{name + "."},
		}).GetKey()
	}
	live, archived, off := option("Postura da Garra"), option("Postura antiga"), option("Postura desligada")
	if _, err := f.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: f.campaign, Key: archived})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	f.master.setOff(t, f.campaign, true, off)

	pool := groupKeys(groupOf(f.fighterOptions(t), f.postures.GetKey()).GetPool())
	if !slices.Contains(pool, live) || slices.Contains(pool, archived) || slices.Contains(pool, off) {
		t.Errorf("pool = %v, want the live option and neither the archived nor the switched off one", pool)
	}
	for name, key := range map[string]string{"archived": archived, "switched off": off} {
		ch := f.choices(false)
		ch.SwapFeatureOptions = []*charactersv1.LevelUpPickSwap{featureSwap(f.postures.GetKey(), f.heron, key)}
		if r := f.swapRefusal(t, ch); r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWAP_NOT_ALLOWED {
			t.Errorf("a swap that takes the %s option: refusal = %v, want swap_not_allowed", name, r)
		}
	}
}
